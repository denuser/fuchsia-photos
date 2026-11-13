// Package transcode makes H.264 copies of videos that some browsers cannot
// play, such as HEVC from phones, on graphics that encode H.264 through VAAPI.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// Queue runs the transcoding. It has one worker, as the graphics make one
// copy at a time, and photo processing keeps its own workers.
const Queue = "transcode"

// When copies are made, as stored in video_settings.
const (
	ModeOff    = "off"
	ModeUpload = "upload"
	ModeView   = "view"
)

// The states of photos.video_copy.
const (
	CopyReady     = "ready"
	CopyNotNeeded = "not_needed"
	CopyFailed    = "failed"
)

// A copy of a long 4K video takes minutes; this leaves room for the longest
// upload of 4 GB.
const jobTimeout = 2 * time.Hour

// waitingOrRunning keeps a job from being queued twice.
var waitingOrRunning = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
	rivertype.JobStateRunning, rivertype.JobStateScheduled,
}

// Transcoder makes the copy of the video at in. Tests replace it, as they have
// no graphics.
type Transcoder func(ctx context.Context, in, out string, video media.Video, hw media.Hardware) error

// Prober reads what the copy depends on. Tests replace it, as their sample
// videos are H.264, which needs no copy.
type Prober func(ctx context.Context, path string) (media.Video, error)

// Service decides which items get a copy and queues their jobs.
type Service struct {
	Queries *store.Queries
	Layout  storage.Layout
	// Hardware is nil when the graphics cannot make copies, and Unavailable
	// then says why.
	Hardware    *media.Hardware
	Unavailable string
	// Jobs queues the copies. It is set once the job client exists.
	Jobs *river.Client[pgx.Tx]
	// Transcode defaults to media.Transcode, Probe to media.ProbeVideo.
	Transcode Transcoder
	Probe     Prober
}

// Mode returns when copies are made, or ModeOff when the graphics cannot make
// them.
func (s *Service) Mode(ctx context.Context) (string, error) {
	if s.Hardware == nil {
		return ModeOff, nil
	}
	settings, err := s.Queries.GetVideoSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("load video settings: %w", err)
	}
	return settings.Transcode, nil
}

// Args queues the copy of one item. Viewed marks an item someone has opened,
// which goes before the others; it is a separate job from the one queued
// after processing, and whichever runs second finds the copy made.
type Args struct {
	PhotoID int64 `json:"photo_id"`
	Viewed  bool  `json:"viewed,omitempty"`
}

// Kind implements river.JobArgs.
func (Args) Kind() string { return "transcode_video" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (a Args) InsertOpts() river.InsertOpts {
	priority := 4
	if a.Viewed {
		priority = 1
	}
	return river.InsertOpts{
		Queue:       Queue,
		MaxAttempts: 2,
		Priority:    priority,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: waitingOrRunning},
	}
}

// QueueMissingArgs queues copies of every item the transcode job has not
// looked at, when copies are made on upload. It runs at every start, once a
// day and when an administrator chooses copies on upload.
type QueueMissingArgs struct{}

// Kind implements river.JobArgs.
func (QueueMissingArgs) Kind() string { return "queue_transcodes" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (QueueMissingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:      Queue,
		Priority:   4,
		UniqueOpts: river.UniqueOpts{ByState: waitingOrRunning},
	}
}

// PeriodicJob runs QueueMissingArgs at every start and once a day.
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) { return QueueMissingArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

// Register adds the workers to a job client.
func (s *Service) Register(workers *river.Workers) {
	river.AddWorker(workers, &worker{service: s})
	river.AddWorker(workers, &queueMissingWorker{service: s})
}

// Processed queues the copy of an item processing has just made ready, when
// copies are made on upload. A failure only leaves the copy to the daily job.
func (s *Service) Processed(ctx context.Context, photoID int64, video, motion bool) {
	if !video && !motion {
		return
	}
	mode, err := s.Mode(ctx)
	if err == nil && mode == ModeUpload {
		_, err = s.Jobs.Insert(ctx, Args{PhotoID: photoID}, nil)
	}
	if err != nil {
		slog.Warn("queue video copy", "photo", photoID, "error", err)
	}
}

// Request queues the copy of an item someone has opened and tells whether a
// copy may come: false when copies are not made.
func (s *Service) Request(ctx context.Context, photoID int64) (bool, error) {
	mode, err := s.Mode(ctx)
	if err != nil || mode == ModeOff {
		return false, err
	}
	if _, err := s.Jobs.Insert(ctx, Args{PhotoID: photoID, Viewed: true}, nil); err != nil {
		return false, fmt.Errorf("queue video copy: %w", err)
	}
	return true, nil
}

// QueueMissing queues the copies of every item not looked at yet and returns
// how many it queued.
func (s *Service) QueueMissing(ctx context.Context) (int, error) {
	queued := 0
	var after int64
	for {
		ids, err := s.Queries.ListPhotosMissingCopy(ctx, store.ListPhotosMissingCopyParams{AfterID: after, MaxRows: 1000})
		if err != nil {
			return queued, fmt.Errorf("list items without a copy: %w", err)
		}
		if len(ids) == 0 {
			return queued, nil
		}
		params := make([]river.InsertManyParams, len(ids))
		for i, id := range ids {
			params[i] = river.InsertManyParams{Args: Args{PhotoID: id}}
		}
		if _, err := s.Jobs.InsertMany(ctx, params); err != nil {
			return queued, fmt.Errorf("queue video copies: %w", err)
		}
		queued += len(ids)
		after = ids[len(ids)-1]
	}
}

type queueMissingWorker struct {
	river.WorkerDefaults[QueueMissingArgs]
	service *Service
}

func (w *queueMissingWorker) Work(ctx context.Context, _ *river.Job[QueueMissingArgs]) error {
	mode, err := w.service.Mode(ctx)
	if err != nil || mode != ModeUpload {
		return err
	}
	queued, err := w.service.QueueMissing(ctx)
	if queued > 0 {
		slog.Info("queued video copies", "items", queued)
	}
	return err
}

type worker struct {
	river.WorkerDefaults[Args]
	service *Service
}

func (w *worker) Timeout(*river.Job[Args]) time.Duration { return jobTimeout }

func (w *worker) Work(ctx context.Context, job *river.Job[Args]) error {
	s := w.service
	mode, err := s.Mode(ctx)
	if err != nil || mode == ModeOff {
		// The item waits until copies are made again.
		return err
	}
	photo, err := s.Queries.GetPhoto(ctx, job.Args.PhotoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	// The video of a Live Photo plays as its photo's motion, which gets the
	// copy.
	if photo.LivePhotoID != nil {
		photo, err = s.Queries.GetPhoto(ctx, *photo.LivePhotoID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load photo: %w", err)
		}
	}
	if photo.Status != "ready" || photo.PurgedAt != nil || photo.VideoCopy != nil {
		return nil
	}
	source, err := s.source(ctx, photo)
	if err != nil || source == "" {
		return err
	}

	state, err := s.makeCopy(ctx, photo.ID, source)
	if err != nil {
		if ctx.Err() != nil || job.Attempt < job.MaxAttempts {
			return err
		}
		slog.Error("make video copy", "photo", photo.ID, "error", err)
		state = CopyFailed
	}
	if err := s.Queries.SetPhotoVideoCopy(ctx, store.SetPhotoVideoCopyParams{ID: photo.ID, VideoCopy: &state}); err != nil {
		return fmt.Errorf("update photo: %w", err)
	}
	return nil
}

// source returns the file an item plays, if any: the original of a video, or
// the motion of a photo.
func (s *Service) source(ctx context.Context, photo store.Photo) (string, error) {
	if storage.IsVideo(photo.Mime) {
		return s.Layout.Original(photo.OwnerID, photo.Sha256, photo.Mime), nil
	}
	if !photo.HasMotion {
		return "", nil
	}
	video, err := s.Queries.GetLivePhotoVideo(ctx, &photo.ID)
	switch {
	case err == nil:
		return s.Layout.Original(video.OwnerID, video.Sha256, video.Mime), nil
	case errors.Is(err, pgx.ErrNoRows):
		return s.Layout.Motion(photo.ID), nil
	default:
		return "", fmt.Errorf("look up live photo video: %w", err)
	}
}

// makeCopy writes the copy of the video at source, if it needs one, and
// returns the new state of the item.
func (s *Service) makeCopy(ctx context.Context, photoID int64, source string) (string, error) {
	probe := s.Probe
	if probe == nil {
		probe = media.ProbeVideo
	}
	video, err := probe(ctx, source)
	if err != nil {
		return "", err
	}
	if !media.NeedsCopy(video) {
		return CopyNotNeeded, nil
	}
	// Written next to the previews, which may be on another disk than the
	// originals, and moved into place when whole.
	path := s.Layout.Copy(photoID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	transcode := s.Transcode
	if transcode == nil {
		transcode = media.Transcode
	}
	start := time.Now()
	if err := transcode(ctx, source, tmp.Name(), video, *s.Hardware); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("move video copy: %w", err)
	}
	slog.Info("made video copy", "photo", photoID, "seconds", time.Since(start).Seconds(), "duration", video.Duration)
	return CopyReady, nil
}

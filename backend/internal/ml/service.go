package ml

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	// MaxAttempts is how often a photo may fail before the job skips it.
	MaxAttempts = 3
	batchSize   = 16
	// A run of the job ends after this long and the next one goes on, so that
	// a large library does not hold one job for hours.
	runLength  = 10 * time.Minute
	jobTimeout = runLength + 3*time.Minute
)

// ErrUnavailable means the ML service cannot be used now.
var ErrUnavailable = errors.New("the ML service is unavailable")

// QueueML runs the embedding job. It has one worker, as one job at a time
// works through all photos.
const QueueML = "ml"

// Service embeds photos and search texts with the ML service.
type Service struct {
	Monitor *Monitor
	Client  *Client
	// Model is the configured model, the one whose vectors are used.
	Model   string
	Layout  storage.Layout
	Queries *store.Queries
}

// EmbedText returns the vector of a search text, with the least similarity
// of a match if the service knows one.
func (s *Service) EmbedText(ctx context.Context, text string) (TextEmbedding, error) {
	if !s.Monitor.Available() {
		return TextEmbedding{}, ErrUnavailable
	}
	embedding, err := s.Client.EmbedText(ctx, text)
	if err != nil {
		if ctx.Err() == nil {
			s.Monitor.Failed(err)
		}
		return TextEmbedding{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if embedding.Model != s.Model {
		s.Monitor.Check(ctx)
		return TextEmbedding{}, ErrUnavailable
	}
	return embedding, nil
}

// VectorText writes a vector the way pgvector reads it, such as [0.1,0.2].
func VectorText(vector []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// EmbedPhotosArgs queues a run over the photos that have no vector yet.
type EmbedPhotosArgs struct{}

// Kind implements river.JobArgs.
func (EmbedPhotosArgs) Kind() string { return "embed_photos" }

// InsertOpts implements river.JobArgsWithInsertOpts. One run at a time: a run
// that is queued or running already covers every photo.
func (EmbedPhotosArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       QueueML,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
		}},
	}
}

// PeriodicJob starts a run every minute. Runs do nothing while the service is
// unavailable.
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return EmbedPhotosArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

// Register adds the embedding worker to a job client.
func (s *Service) Register(workers *river.Workers) {
	river.AddWorker(workers, &embedWorker{service: s})
}

type embedWorker struct {
	river.WorkerDefaults[EmbedPhotosArgs]
	service *Service
}

func (w *embedWorker) Timeout(*river.Job[EmbedPhotosArgs]) time.Duration { return jobTimeout }

func (w *embedWorker) Work(ctx context.Context, _ *river.Job[EmbedPhotosArgs]) error {
	_, err := w.service.EmbedPending(ctx, runLength)
	return err
}

// EmbedPending sends the photos without a vector to the service in batches,
// until none are left, the service goes away or the time is up. It returns how
// many photos got a vector. A missing service is not an error: the photos wait
// for the next run.
func (s *Service) EmbedPending(ctx context.Context, limit time.Duration) (int, error) {
	if s.Monitor == nil || !s.Monitor.Available() {
		return 0, nil
	}
	deadline := time.Now().Add(limit)
	done := 0
	// Photos that fail in this run wait for the next one.
	var afterID int64
	for time.Now().Before(deadline) {
		rows, err := s.Queries.ListPhotosToEmbed(ctx, store.ListPhotosToEmbedParams{
			Model: s.Model, MaxAttempts: MaxAttempts, AfterID: afterID, MaxRows: batchSize,
		})
		if err != nil {
			return done, fmt.Errorf("list photos to embed: %w", err)
		}
		if len(rows) == 0 {
			return done, nil
		}
		afterID = rows[len(rows)-1].ID

		n, err := s.embedBatch(ctx, rows)
		done += n
		var unavailable *serviceError
		switch {
		case errors.As(err, &unavailable) && ctx.Err() == nil:
			slog.Warn("embed photos", "error", err)
			if errors.Is(err, ErrModelMismatch) {
				s.Monitor.Check(ctx)
			} else {
				s.Monitor.Failed(err)
			}
			return done, nil
		case err != nil:
			return done, err
		}
	}
	return done, nil
}

func (s *Service) embedBatch(ctx context.Context, rows []store.ListPhotosToEmbedRow) (int, error) {
	images := make([]Image, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		data, err := os.ReadFile(s.Layout.Preview(row.ID, storage.PreviewViewer, row.PreviewFormat))
		if err != nil {
			s.recordFailure(ctx, row.ID, fmt.Sprintf("read preview: %v", err))
			continue
		}
		images = append(images, Image{Name: fmt.Sprintf("%d.%s", row.ID, row.PreviewFormat), Data: data})
		ids = append(ids, row.ID)
	}
	if len(images) == 0 {
		return 0, nil
	}

	model, results, err := s.Client.EmbedImages(ctx, images)
	if err != nil {
		return 0, &serviceError{err}
	}
	if model != s.Model {
		return 0, &serviceError{fmt.Errorf("%w: %s", ErrModelMismatch, model)}
	}
	done := 0
	for i, result := range results {
		if result.Error != "" || len(result.Embedding) == 0 {
			message := result.Error
			if message == "" {
				message = "no vector"
			}
			s.recordFailure(ctx, ids[i], message)
			continue
		}
		err := s.Queries.SaveEmbedding(ctx, store.SaveEmbeddingParams{
			PhotoID: ids[i], Model: s.Model, Embedding: VectorText(result.Embedding),
		})
		if err != nil {
			return done, fmt.Errorf("save embedding of photo %d: %w", ids[i], err)
		}
		done++
	}
	return done, nil
}

func (s *Service) recordFailure(ctx context.Context, photoID int64, message string) {
	slog.Warn("embed photo", "photo", photoID, "error", message)
	err := s.Queries.RecordEmbeddingFailure(ctx, store.RecordEmbeddingFailureParams{
		PhotoID: photoID, Model: s.Model, Error: message,
	})
	if err != nil {
		slog.Error("record embedding failure", "photo", photoID, "error", err)
	}
}

// serviceError is a failure of the service rather than of the application.
type serviceError struct{ err error }

func (e *serviceError) Error() string { return e.err.Error() }
func (e *serviceError) Unwrap() error { return e.err }

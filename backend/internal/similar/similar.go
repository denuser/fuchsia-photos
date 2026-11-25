// Package similar finds photos of one owner that look alike: copies, edits and
// nearly identical frames (duplicates), and other shots of the same scene
// (similar shots). It works from the vectors the ML service made and from a
// hash of each preview, so it runs on the vectors already stored even when the
// service is switched off.
package similar

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/magellanica-app/magellanica-photos/backend/internal/media"
	"github.com/magellanica-app/magellanica-photos/backend/internal/ml"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// The tiers of a pair.
const (
	TierDuplicate = "duplicate"
	TierSimilar   = "similar"
)

// The limits, found on labelled pairs of real libraries (improvement 25, step 0).
// A vector alone cannot tell a copy from another shot of the same place, so a
// duplicate needs a second reason: a hash a few bits away, or a capture time a
// few seconds away.
const (
	DuplicateDistance = 0.012
	DuplicateHashBits = 6
	DuplicateSeconds  = 3.0
	SimilarDistance   = 0.03
)

const (
	hashBatch      = 100
	checkBatch     = 50
	maxCandidates  = 40
	runLength      = 10 * time.Minute
	jobTimeout     = runLength + 3*time.Minute
	pairsPerPhoto  = maxCandidates
	checkTimeLimit = 30 * time.Second
)

// Classify tells which tier a candidate pair belongs to. ok is false when it
// belongs to none. bits and seconds are nil when the hash or a capture time of
// one of the photos is missing, and a missing one is never a reason.
func Classify(c store.FindSimilarCandidatesRow) (tier string, hashBits *int16, seconds *float64, ok bool) {
	if c.Phash != nil && c.OtherPhash != nil {
		n := int16(bits.OnesCount64(uint64(*c.Phash) ^ uint64(*c.OtherPhash)))
		hashBits = &n
	}
	if c.TakenAt != nil && c.OtherTakenAt != nil {
		s := math.Abs(c.TakenAt.Sub(*c.OtherTakenAt).Seconds())
		seconds = &s
	}
	if c.Distance > SimilarDistance {
		return "", hashBits, seconds, false
	}
	if c.Distance <= DuplicateDistance &&
		((hashBits != nil && *hashBits <= DuplicateHashBits) || (seconds != nil && *seconds <= DuplicateSeconds)) {
		return TierDuplicate, hashBits, seconds, true
	}
	return TierSimilar, hashBits, seconds, true
}

// Service looks for similar photos.
type Service struct {
	// Model is the configured model, the one whose vectors are used.
	Model  string
	Layout storage.Layout
	Pool   *pgxpool.Pool
}

// FindSimilarArgs queues a run over the photos that have not been looked at.
type FindSimilarArgs struct{}

// Kind implements river.JobArgs.
func (FindSimilarArgs) Kind() string { return "find_similar" }

// InsertOpts implements river.JobArgsWithInsertOpts. It shares the queue of
// the embedding job, which has one worker, and one run at a time covers every
// photo that waits.
func (FindSimilarArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       ml.QueueML,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{ByState: []rivertype.JobState{
			rivertype.JobStateAvailable,
			rivertype.JobStatePending,
			rivertype.JobStateRunning,
			rivertype.JobStateScheduled,
		}},
	}
}

// PeriodicJob starts a run every minute. A run with nothing waiting is two
// short queries.
func PeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(time.Minute),
		func() (river.JobArgs, *river.InsertOpts) { return FindSimilarArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

// Register adds the worker to a job client.
func (s *Service) Register(workers *river.Workers) {
	river.AddWorker(workers, &worker{service: s})
}

type worker struct {
	river.WorkerDefaults[FindSimilarArgs]
	service *Service
}

func (w *worker) Timeout(*river.Job[FindSimilarArgs]) time.Duration { return jobTimeout }

func (w *worker) Work(ctx context.Context, _ *river.Job[FindSimilarArgs]) error {
	return w.service.Run(ctx, runLength)
}

// Run hashes the previews that have no hash, then looks at the photos whose
// vector has not been looked at, until none are left or the time is up.
func (s *Service) Run(ctx context.Context, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	if err := s.hashPending(ctx, deadline); err != nil {
		return err
	}
	return s.checkPending(ctx, deadline)
}

func (s *Service) hashPending(ctx context.Context, deadline time.Time) error {
	queries := store.New(s.Pool)
	var afterID int64
	for time.Now().Before(deadline) {
		rows, err := queries.ListPhotosToHash(ctx, store.ListPhotosToHashParams{AfterID: afterID, MaxRows: hashBatch})
		if err != nil {
			return fmt.Errorf("list photos to hash: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		afterID = rows[len(rows)-1].ID
		for _, row := range rows {
			data, err := os.ReadFile(s.Layout.Preview(row.ID, storage.PreviewViewer, row.PreviewFormat))
			if err != nil {
				slog.Debug("hash photo: read preview", "photo", row.ID, "error", err)
				continue
			}
			hash, err := media.Hash(data)
			if err != nil {
				slog.Debug("hash photo", "photo", row.ID, "error", err)
				continue
			}
			if err := queries.SavePerceptualHash(ctx, store.SavePerceptualHashParams{ID: row.ID, Phash: int64Ptr(hash)}); err != nil {
				return fmt.Errorf("save hash of photo %d: %w", row.ID, err)
			}
		}
	}
	return nil
}

func (s *Service) checkPending(ctx context.Context, deadline time.Time) error {
	queries := store.New(s.Pool)
	var afterID int64
	for time.Now().Before(deadline) {
		rows, err := queries.ListPhotosToCheck(ctx, store.ListPhotosToCheckParams{Model: s.Model, AfterID: afterID, MaxRows: checkBatch})
		if err != nil {
			return fmt.Errorf("list photos to check: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		afterID = rows[len(rows)-1].PhotoID
		for _, row := range rows {
			if err := s.Check(ctx, row.PhotoID, row.EmbeddingAt); err != nil {
				return err
			}
		}
	}
	return nil
}

// Check looks at one photo: its open pairs are replaced by the ones found now,
// and the look is noted against the time of its vector.
func (s *Service) Check(ctx context.Context, photoID int64, embeddingAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeLimit)
	defer cancel()
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		queries := store.New(tx)
		candidates, err := queries.FindSimilarCandidates(ctx, store.FindSimilarCandidatesParams{
			PhotoID: photoID, Model: s.Model, MaxDistance: SimilarDistance, MaxRows: pairsPerPhoto,
		})
		if err != nil {
			return fmt.Errorf("find candidates: %w", err)
		}
		if err := queries.DeleteOpenSimilarPairsOf(ctx, photoID); err != nil {
			return fmt.Errorf("delete open pairs: %w", err)
		}
		for _, c := range candidates {
			tier, hashBits, seconds, ok := Classify(c)
			if !ok {
				continue
			}
			err := queries.UpsertSimilarPair(ctx, store.UpsertSimilarPairParams{
				PhotoID: photoID, OtherID: c.OtherID, Tier: tier, Distance: c.Distance,
				HashBits: hashBits, SecondsApart: seconds,
			})
			if err != nil {
				return fmt.Errorf("save pair: %w", err)
			}
		}
		return queries.SaveSimilarCheck(ctx, store.SaveSimilarCheckParams{PhotoID: photoID, Model: s.Model, EmbeddingAt: embeddingAt})
	})
	if err != nil {
		return fmt.Errorf("check photo %d: %w", photoID, err)
	}
	return nil
}

func int64Ptr(hash uint64) *int64 {
	v := int64(hash)
	return &v
}

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KindStats counts the jobs of one kind by what is happening to them. River
// keeps finished jobs for a day, so the counts of finished jobs cover a day at most.
type KindStats struct {
	Kind      string `json:"kind"`
	Waiting   int64  `json:"waiting"`
	Running   int64  `json:"running"`
	Retrying  int64  `json:"retrying"`
	DoneHour  int64  `json:"doneHour"`
	DoneDay   int64  `json:"doneDay"`
	FailedDay int64  `json:"failedDay"`
}

// JobError is the latest error of a job that failed or waits to be retried.
type JobError struct {
	JobID       int64     `json:"jobId"`
	Kind        string    `json:"kind"`
	State       string    `json:"state"`
	Attempt     int       `json:"attempt"`
	MaxAttempts int       `json:"maxAttempts"`
	PhotoID     *int64    `json:"photoId"`
	Error       string    `json:"error"`
	At          time.Time `json:"at"`
}

// QueueStats describes the job queue.
type QueueStats struct {
	Kinds  []KindStats `json:"kinds"`
	Errors []JobError  `json:"errors"`
}

const maxStatsErrors = 10

// ReadQueueStats counts jobs by kind and lists the latest errors.
func ReadQueueStats(ctx context.Context, pool *pgxpool.Pool) (QueueStats, error) {
	rows, err := pool.Query(ctx, `
		SELECT kind,
			count(*) FILTER (WHERE state IN ('available', 'pending', 'scheduled')),
			count(*) FILTER (WHERE state = 'running'),
			count(*) FILTER (WHERE state = 'retryable'),
			count(*) FILTER (WHERE state = 'completed' AND finalized_at > now() - interval '1 hour'),
			count(*) FILTER (WHERE state = 'completed' AND finalized_at > now() - interval '1 day'),
			count(*) FILTER (WHERE state IN ('cancelled', 'discarded') AND finalized_at > now() - interval '1 day')
		FROM river_job
		GROUP BY kind
		ORDER BY kind`)
	if err != nil {
		return QueueStats{}, fmt.Errorf("count jobs: %w", err)
	}
	kinds, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (KindStats, error) {
		var k KindStats
		err := row.Scan(&k.Kind, &k.Waiting, &k.Running, &k.Retrying, &k.DoneHour, &k.DoneDay, &k.FailedDay)
		return k, err
	})
	if err != nil {
		return QueueStats{}, fmt.Errorf("count jobs: %w", err)
	}

	// Jobs that failed for good, and those waiting for another attempt.
	rows, err = pool.Query(ctx, `
		SELECT id, kind, state::text, attempt, max_attempts, (args->>'photo_id')::bigint,
			errors[array_length(errors, 1)]
		FROM river_job
		WHERE state IN ('retryable', 'cancelled', 'discarded') AND errors IS NOT NULL
		ORDER BY coalesce(finalized_at, attempted_at) DESC
		LIMIT $1`, maxStatsErrors)
	if err != nil {
		return QueueStats{}, fmt.Errorf("list job errors: %w", err)
	}
	errs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (JobError, error) {
		var e JobError
		var last []byte
		if err := row.Scan(&e.JobID, &e.Kind, &e.State, &e.Attempt, &e.MaxAttempts, &e.PhotoID, &last); err != nil {
			return e, err
		}
		var detail struct {
			At    time.Time `json:"at"`
			Error string    `json:"error"`
		}
		if err := json.Unmarshal(last, &detail); err != nil {
			return e, fmt.Errorf("read error of job %d: %w", e.JobID, err)
		}
		e.Error, e.At = detail.Error, detail.At
		return e, nil
	})
	if err != nil {
		return QueueStats{}, fmt.Errorf("list job errors: %w", err)
	}
	return QueueStats{Kinds: kinds, Errors: errs}, nil
}

package library

import (
	"context"
	"testing"
)

// The periodic job is inserted with the same arguments every time. It must not be dropped
// because an earlier run finished, or folders would be scanned about once a day.
func TestDueRunsAreNotDroppedAfterTheFirstCompleted(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	client := e.service.Ingest.Jobs
	for i := range 2 {
		res, err := client.Insert(ctx, DueArgs{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.UniqueSkippedAsDuplicate {
			t.Fatalf("run %d was skipped as a duplicate", i+1)
		}
		// The run finishes before the next one is due.
		if _, err := e.service.Ingest.Pool.Exec(ctx, `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE id = $1`, res.Job.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A run that waits still blocks another, so that none piles up.
	if _, err := client.Insert(ctx, DueArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := client.Insert(ctx, DueArgs{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.UniqueSkippedAsDuplicate {
		t.Error("a second waiting run was queued")
	}
}

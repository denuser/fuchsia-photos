package ml

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// GroupLimit is the largest mean distance from a face to the three nearest
// faces of a group for the face to join it. Found on labelled pairs of one
// library (improvement 26, grouping calibration); it errs towards splitting
// a person, which one merge undoes, rather than joining two people.
const GroupLimit = 0.50

// groupNearest is how many of a group's nearest faces the mean covers.
const groupNearest = 3

// DefaultRetryMinScore is the least score a face found only by the contrast
// retry needs to join a group, until an administrator changes it.
const DefaultRetryMinScore = 0.75

// RetryMinScore reads the setting for faces found by the contrast retry.
func RetryMinScore(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (float64, error) {
	var score float32
	err := db.QueryRow(ctx, "SELECT retry_min_score FROM face_settings").Scan(&score)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultRetryMinScore, nil
	}
	return float64(score), err
}

// Groupable tells whether a face may join a group under the retry setting.
// A face scanned before scores were kept counts as found by the first pass.
func Groupable(retry bool, score *float32, minScore float64) bool {
	return !retry || score == nil || float64(*score) >= minScore
}

// assignFace finds the group a new face of a photo joins, by the same rule as
// Regroup, or reports false when it starts one. It runs in the owner's
// transaction, after the photo's earlier faces were inserted.
func assignFace(ctx context.Context, tx pgx.Tx, owner int64, model string, photo int64, vector string) (int64, bool, error) {
	var person int64
	err := tx.QueryRow(ctx, `WITH grouped AS (
   SELECT f.person_id, f.embedding <=> $3::halfvec AS distance FROM visible_photo_faces f
   WHERE f.owner_id=$1 AND f.model=$2 AND f.person_id IS NOT NULL
   AND f.person_id NOT IN (SELECT person_id FROM photo_faces WHERE photo_id=$4 AND person_id IS NOT NULL)
 ), ranked AS (
   SELECT person_id, distance, row_number() OVER (PARTITION BY person_id ORDER BY distance) AS n FROM grouped
 )
 SELECT person_id FROM ranked WHERE n<=$5 GROUP BY person_id
 HAVING avg(distance)<$6 ORDER BY avg(distance), person_id LIMIT 1`,
		owner, model, vector, photo, groupNearest, GroupLimit).Scan(&person)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return person, err == nil, err
}

// GroupFace is one face as the grouping rule sees it.
type GroupFace struct {
	ID     int64
	Photo  int64
	Vector []float32
	// Group is the face's group, 0 for none. Fixed faces keep theirs.
	Group int64
	Fixed bool
	// Eligible is false for a face the retry setting leaves out.
	Eligible bool
}

// Group assigns every face that is not fixed, in the order given, by the
// rule of assignFace: the group whose nearest faces have the smallest mean
// distance under GroupLimit, skipping groups that hold a face of the same
// photo, or else a new group. New groups get negative ids from -1 down.
func Group(faces []GroupFace) {
	members := map[int64][]int{}
	photos := map[int64]map[int64]bool{}
	add := func(i int) {
		g := faces[i].Group
		members[g] = append(members[g], i)
		if photos[g] == nil {
			photos[g] = map[int64]bool{}
		}
		photos[g][faces[i].Photo] = true
	}
	for i := range faces {
		if faces[i].Fixed && faces[i].Group != 0 {
			add(i)
		}
	}
	next := int64(-1)
	order := make([]int64, 0, len(members))
	for g := range members {
		order = append(order, g)
	}
	sort.Slice(order, func(a, b int) bool { return order[a] < order[b] })
	for i := range faces {
		face := &faces[i]
		if face.Fixed {
			continue
		}
		face.Group = 0
		if !face.Eligible {
			continue
		}
		best, bestMean := int64(0), math.Inf(1)
		for _, g := range order {
			if photos[g][face.Photo] {
				continue
			}
			mean := nearestMean(face.Vector, faces, members[g])
			if mean < GroupLimit && (mean < bestMean || mean == bestMean && g < best) {
				best, bestMean = g, mean
			}
		}
		if best == 0 {
			best = next
			next--
			order = append(order, best)
		}
		face.Group = best
		add(i)
	}
}

func nearestMean(vector []float32, faces []GroupFace, members []int) float64 {
	nearest := make([]float64, 0, groupNearest+1)
	for _, m := range members {
		d := cosineDistance(vector, faces[m].Vector)
		nearest = append(nearest, d)
		sort.Float64s(nearest)
		if len(nearest) > groupNearest {
			nearest = nearest[:groupNearest]
		}
	}
	var sum float64
	for _, d := range nearest {
		sum += d
	}
	return sum / float64(len(nearest))
}

func cosineDistance(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 1
	}
	return 1 - dot/math.Sqrt(na*nb)
}

func parseVector(text string) ([]float32, error) {
	text = strings.TrimSuffix(strings.TrimPrefix(text, "["), "]")
	parts := strings.Split(text, ",")
	vector := make([]float32, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseFloat(part, 32)
		if err != nil {
			return nil, fmt.Errorf("parse vector: %w", err)
		}
		vector[i] = float32(v)
	}
	return vector, nil
}

// RegroupArgs groups an owner's faces again.
type RegroupArgs struct {
	Owner int64 `json:"owner"`
}

// Kind implements river.JobArgs.
func (RegroupArgs) Kind() string { return "regroup_people" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (RegroupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "people",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending,
			rivertype.JobStateRetryable, rivertype.JobStateScheduled,
		}},
	}
}

// RegroupWorker runs RegroupArgs.
type RegroupWorker struct {
	river.WorkerDefaults[RegroupArgs]
	Pool *pgxpool.Pool
}

func (w *RegroupWorker) Timeout(*river.Job[RegroupArgs]) time.Duration { return 30 * time.Minute }

func (w *RegroupWorker) Work(ctx context.Context, job *river.Job[RegroupArgs]) error {
	return Regroup(ctx, w.Pool, job.Args.Owner)
}

// Regroup assigns an owner's faces again by the present rule and setting.
// Faces the owner placed or ignored, and faces of named groups, stay where
// they are and anchor the others. The groups are computed from a snapshot;
// a face that changed meanwhile keeps its new state.
func Regroup(ctx context.Context, pool *pgxpool.Pool, owner int64) error {
	minScore, err := RetryMinScore(ctx, pool)
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `SELECT f.id, f.photo_id, v.model, coalesce(f.person_id, 0), f.manual,
   coalesce(g.name, '') <> '', f.retry, f.score, f.embedding::text
 FROM visible_photo_faces v JOIN photo_faces f ON f.id = v.id LEFT JOIN people g ON g.id = f.person_id
 WHERE v.owner_id = $1 ORDER BY f.id`, owner)
	if err != nil {
		return fmt.Errorf("load faces: %w", err)
	}
	type loaded struct {
		model string
		old   int64
		face  GroupFace
	}
	var all []loaded
	for rows.Next() {
		var l loaded
		var manual, named, retry bool
		var score *float32
		var text string
		if err := rows.Scan(&l.face.ID, &l.face.Photo, &l.model, &l.old, &manual, &named, &retry, &score, &text); err != nil {
			rows.Close()
			return err
		}
		if l.face.Vector, err = parseVector(text); err != nil {
			rows.Close()
			return err
		}
		l.face.Group = l.old
		l.face.Fixed = manual || named
		l.face.Eligible = Groupable(retry, score, minScore)
		all = append(all, l)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Faces of different models are never compared.
	byModel := map[string][]int{}
	for i, l := range all {
		byModel[l.model] = append(byModel[l.model], i)
	}
	var ids, olds []int64
	var news []int64
	newGroups := 0
	for _, indexes := range byModel {
		faces := make([]GroupFace, len(indexes))
		for j, i := range indexes {
			faces[j] = all[i].face
		}
		Group(faces)
		offset := int64(newGroups)
		for j, i := range indexes {
			if faces[j].Fixed {
				continue
			}
			group := faces[j].Group
			if group < 0 {
				group -= offset
				if int(-group) > newGroups {
					newGroups = int(-group)
				}
			}
			ids = append(ids, all[i].face.ID)
			olds = append(olds, all[i].old)
			news = append(news, group)
		}
	}

	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", owner); err != nil {
			return err
		}
		created := make([]int64, newGroups)
		for i := range created {
			if err := tx.QueryRow(ctx, "INSERT INTO people(owner_id) VALUES($1) RETURNING id", owner).Scan(&created[i]); err != nil {
				return err
			}
		}
		targets := make([]*int64, len(news))
		for i, g := range news {
			switch {
			case g < 0:
				targets[i] = &created[-g-1]
			case g > 0:
				targets[i] = &news[i]
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE photo_faces f SET person_id = u.target
 FROM unnest($1::bigint[], $2::bigint[], $3::bigint[]) AS u(id, old, target)
 WHERE f.id = u.id AND coalesce(f.person_id, 0) = u.old AND NOT f.manual`, ids, olds, targets); err != nil {
			return fmt.Errorf("save groups: %w", err)
		}
		_, err := tx.Exec(ctx, `DELETE FROM people g WHERE g.owner_id = $1 AND g.name = ''
 AND NOT EXISTS (SELECT FROM photo_faces f WHERE f.person_id = g.id)`, owner)
		return err
	})
}

// RegroupAll queues a regrouping of every owner that has faces.
func RegroupAll(ctx context.Context, pool *pgxpool.Pool, jobs *river.Client[pgx.Tx]) error {
	rows, err := pool.Query(ctx, "SELECT DISTINCT owner_id FROM visible_photo_faces")
	if err != nil {
		return err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	for _, owner := range owners {
		if _, err := jobs.Insert(ctx, RegroupArgs{Owner: owner}, nil); err != nil {
			return err
		}
	}
	return nil
}

// RegroupPending queues a regrouping of every owner if a migration asked for
// one, and clears the request. It runs at start, once the job client exists.
func RegroupPending(ctx context.Context, pool *pgxpool.Pool, jobs *river.Client[pgx.Tx]) error {
	var pending bool
	err := pool.QueryRow(ctx, "SELECT regroup_pending FROM face_settings").Scan(&pending)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !pending {
		return nil
	}
	if err != nil {
		return err
	}
	if err := RegroupAll(ctx, pool, jobs); err != nil {
		return err
	}
	_, err = pool.Exec(ctx, "UPDATE face_settings SET regroup_pending = false")
	return err
}

package ml

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

// RescanOverlap is the least overlap (intersection over union) between a face
// found after a rotation and a stored face, turned with the photo, for the two
// to be the same face. The detector draws a turned face's box tighter or
// looser: on 57 faces found in both orientations the overlap ran from 0.21 to
// 0.90, median 0.69, and 0.3 keeps 55 (improvement 27).
const RescanOverlap = 0.3

// groupFor gives a new or rescanned face its group: by the grouping rule,
// a new group when none fits, or none when the retry setting leaves it out.
func groupFor(ctx context.Context, tx pgx.Tx, owner int64, model string, photo int64, vector string, face DetectedFace, minScore float64) (*int64, error) {
	if !Groupable(face.Retry, face.Score, minScore) {
		return nil, nil
	}
	group, found, err := assignFace(ctx, tx, owner, model, photo, vector)
	if err != nil {
		return nil, err
	}
	if !found {
		if err := tx.QueryRow(ctx, "INSERT INTO people(owner_id) VALUES($1) RETURNING id", owner).Scan(&group); err != nil {
			return nil, err
		}
	}
	return &group, nil
}

type storedFace struct {
	id    int64
	box   []float64
	fixed bool
}

// overlap is the intersection over union of two boxes of left, top, width
// and height.
func overlap(a, b []float64) float64 {
	ix := min(a[0]+a[2], b[0]+b[2]) - max(a[0], b[0])
	iy := min(a[1]+a[3], b[1]+b[3]) - max(a[1], b[1])
	if ix <= 0 || iy <= 0 {
		return 0
	}
	inter := ix * iy
	return inter / (a[2]*a[3] + b[2]*b[3] - inter)
}

// matchFaces pairs detections with stored faces, the closest overlaps first,
// each at most once. It returns, for each detection, the index of its stored
// face or -1.
func matchFaces(detected []DetectedFace, stored []storedFace) []int {
	type pair struct {
		d, s int
		iou  float64
	}
	var pairs []pair
	for d, face := range detected {
		for s, old := range stored {
			if iou := overlap(face.Box, old.box); iou >= RescanOverlap {
				pairs = append(pairs, pair{d, s, iou})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].iou > pairs[j].iou })
	match := make([]int, len(detected))
	for i := range match {
		match[i] = -1
	}
	taken := make([]bool, len(stored))
	for _, p := range pairs {
		if match[p.d] == -1 && !taken[p.s] {
			match[p.d], taken[p.s] = p.s, true
		}
	}
	return match
}

// rescanFaces applies a scan of a photo that was turned after its faces were
// found. A detection that matches a stored face takes its place and keeps its
// id; it keeps its group only if the owner placed it or named the group, and
// is otherwise grouped again. Stored faces without a match are deleted unless
// the owner placed or named them, which stay marked as unmatched. The scan
// row is updated, never deleted, as that would delete the faces with it.
func rescanFaces(ctx context.Context, tx pgx.Tx, owner int64, model string, photo int64, detected []DetectedFace, scanErr error) error {
	if scanErr != nil {
		// The photo keeps its faces; the error shows in its scan lookup.
		_, err := tx.Exec(ctx, "UPDATE face_scans SET rescan=false, error=$2, scanned_at=now() WHERE photo_id=$1", photo, scanErr.Error())
		return err
	}
	rows, err := tx.Query(ctx, `SELECT f.id, f.box, f.manual OR coalesce(g.name,'') <> ''
 FROM photo_faces f LEFT JOIN people g ON g.id = f.person_id WHERE f.photo_id=$1 ORDER BY f.id`, photo)
	if err != nil {
		return err
	}
	var stored []storedFace
	for rows.Next() {
		var f storedFace
		var box []float32
		if err := rows.Scan(&f.id, &box, &f.fixed); err != nil {
			rows.Close()
			return err
		}
		for _, v := range box {
			f.box = append(f.box, float64(v))
		}
		stored = append(stored, f)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	match := matchFaces(detected, stored)
	matched := make([]bool, len(stored))
	for _, s := range match {
		if s >= 0 {
			matched[s] = true
		}
	}
	for i, f := range stored {
		switch {
		case matched[i] && !f.fixed:
			// Grouped again below, so that its own group is not skipped as
			// one that already holds a face of this photo.
			_, err = tx.Exec(ctx, "UPDATE photo_faces SET person_id=NULL WHERE id=$1", f.id)
		case !matched[i] && f.fixed:
			_, err = tx.Exec(ctx, "UPDATE photo_faces SET unmatched=true WHERE id=$1", f.id)
		case !matched[i]:
			_, err = tx.Exec(ctx, "DELETE FROM photo_faces WHERE id=$1", f.id)
		}
		if err != nil {
			return err
		}
	}
	minScore, err := RetryMinScore(ctx, tx)
	if err != nil {
		return err
	}
	for d, face := range detected {
		vector := VectorText(face.Embedding)
		if s := match[d]; s >= 0 {
			old := stored[s]
			if old.fixed {
				_, err = tx.Exec(ctx, `UPDATE photo_faces SET box=$2, embedding=$3::halfvec, score=$4, retry=$5, unmatched=false WHERE id=$1`,
					old.id, face.Box, vector, face.Score, face.Retry)
			} else {
				person, gerr := groupFor(ctx, tx, owner, model, photo, vector, face, minScore)
				if gerr != nil {
					return gerr
				}
				_, err = tx.Exec(ctx, `UPDATE photo_faces SET box=$2, embedding=$3::halfvec, score=$4, retry=$5, unmatched=false, person_id=$6 WHERE id=$1`,
					old.id, face.Box, vector, face.Score, face.Retry, person)
			}
		} else {
			person, gerr := groupFor(ctx, tx, owner, model, photo, vector, face, minScore)
			if gerr != nil {
				return gerr
			}
			_, err = tx.Exec(ctx, "INSERT INTO photo_faces(photo_id,person_id,box,embedding,score,retry) VALUES($1,$2,$3,$4::halfvec,$5,$6)",
				photo, person, face.Box, vector, face.Score, face.Retry)
		}
		if err != nil {
			return fmt.Errorf("save rescanned face: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE face_scans SET rescan=false, model=$2, retry_at=NULL, error=NULL, scanned_at=now() WHERE photo_id=$1", photo, model); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM people g WHERE g.owner_id=$1 AND g.name='' AND NOT EXISTS (SELECT FROM photo_faces f WHERE f.person_id=g.id)`, owner)
	return err
}

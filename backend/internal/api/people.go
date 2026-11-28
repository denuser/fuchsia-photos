package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/magellanica-app/magellanica-photos/backend/internal/media"
	"github.com/magellanica-app/magellanica-photos/backend/internal/storage"
)

type personItem struct {
	Face    int64     `json:"face"`
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Photo   int64     `json:"photo"`
	Version int32     `json:"version"`
	Box     []float32 `json:"box"`
}

func (s *server) listPeople(w http.ResponseWriter, r *http.Request) {
	var person, after int64
	for key, target := range map[string]*int64{"person": &person, "after": &after} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 0 {
				writeError(w, 400, "invalid id")
				return
			}
			*target = n
		}
	}
	ctx := r.Context()
	const groups = `SELECT DISTINCT ON (g.id) f.id,g.id,g.name,f.photo_id,p.preview_version,f.box
 FROM people g JOIN visible_photo_faces f ON f.person_id=g.id AND f.owner_id=g.owner_id
 JOIN photos p ON p.id=f.photo_id WHERE g.owner_id=$1 AND g.id>$2 ORDER BY g.id,f.id LIMIT 100`
	const members = `SELECT f.id,f.id,g.name,p.id,p.preview_version,f.box FROM visible_photo_faces f
 JOIN people g ON g.id=f.person_id AND g.owner_id=f.owner_id JOIN photos p ON p.id=f.photo_id
 WHERE f.owner_id=$1 AND f.id>$2 AND g.id=$3 ORDER BY f.id LIMIT 100`
	args := []any{currentUser(r).ID, after}
	query := groups
	if person != 0 {
		query = members
		args = append(args, person)
	}
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		internalError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[personItem])
	if err != nil {
		internalError(w, r, err)
		return
	}
	if items == nil {
		items = []personItem{}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, 200, items)
}

func (s *server) editPeople(w http.ResponseWriter, r *http.Request) {
	var edit struct {
		Action string
		Person int64
		Face   int64
		Target int64
		Name   string
	}
	if !readJSON(w, r, &edit) {
		return
	}
	edit.Name = strings.TrimSpace(edit.Name)
	if edit.Person <= 0 || len(edit.Name) > 256 || edit.Target < 0 {
		writeError(w, 400, "invalid person")
		return
	}
	ctx := r.Context()
	owner := currentUser(r).ID
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", owner); err != nil {
			return err
		}
		var id int64
		for _, group := range []int64{edit.Person, edit.Target} {
			if group == 0 {
				continue
			}
			if err := tx.QueryRow(ctx, "SELECT id FROM people WHERE id=$1 AND owner_id=$2 FOR UPDATE", group, owner).Scan(&id); err != nil {
				return err
			}
		}
		switch edit.Action {
		case "rename":
			_, err := tx.Exec(ctx, "UPDATE people SET name=$1 WHERE id=$2", edit.Name, edit.Person)
			return err
		case "merge":
			if edit.Target == 0 || edit.Target == edit.Person {
				return errPeopleEdit
			}
			_, err := tx.Exec(ctx, "UPDATE photo_faces SET person_id=$1 WHERE person_id=$2", edit.Target, edit.Person)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "DELETE FROM people WHERE id=$1", edit.Person)
			return err
		case "move", "ignore":
			var target any
			if edit.Action == "move" {
				if edit.Target == 0 {
					if err := tx.QueryRow(ctx, "INSERT INTO people(owner_id) VALUES($1) RETURNING id", owner).Scan(&edit.Target); err != nil {
						return err
					}
				}
				target = edit.Target
			}
			return tx.QueryRow(ctx, `UPDATE photo_faces SET person_id=$1 WHERE id=$2 AND person_id=$3 AND EXISTS(SELECT FROM visible_photo_faces WHERE id=$2 AND owner_id=$4) RETURNING id`, target, edit.Face, edit.Person, owner).Scan(&id)
		default:
			return errPeopleEdit
		}
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, 404, "person or face not found")
	case errors.Is(err, errPeopleEdit):
		writeError(w, 400, "invalid operation")
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

var errPeopleEdit = errors.New("invalid people edit")

func (s *server) faceThumbnail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, 400, "invalid face id")
		return
	}
	var photo int64
	var format string
	var box []float32
	err = s.Pool.QueryRow(r.Context(), `SELECT f.photo_id,p.preview_format,f.box FROM visible_photo_faces f
 JOIN photos p ON p.id=f.photo_id WHERE f.id=$1 AND f.owner_id=$2 AND f.person_id IS NOT NULL`, id, currentUser(r).ID).Scan(&photo, &format, &box)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "face not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	data, err := media.FaceThumbnail(s.Layout.Preview(photo, storage.PreviewViewer, format), box)
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(data)
}

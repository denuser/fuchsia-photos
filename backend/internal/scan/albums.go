package scan

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/denuser/magellanica-photos/backend/internal/store"
)

// maxAlbumName is the longest album name the interface accepts.
const maxAlbumName = 200

// albumDir returns the folder of a file right under the product folder of an
// export, Takeout/Google Фото/<album>/, or "" for a file elsewhere. Album files
// lie there; the list of memory titles lies one level higher. An unpacked
// export keeps Takeout/ at the start of its paths too.
func albumDir(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[0] != takeoutRoot || parts[1] == "" || parts[2] == "" {
		return ""
	}
	return strings.Join(parts[:3], "/") + "/"
}

// albumName cleans a title as the interface cleans a name typed in it: control
// characters become spaces, spaces around it go, and it is cut to the longest
// name allowed.
func albumName(title string) string {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title))
	if utf8.RuneCountInString(name) > maxAlbumName {
		name = strings.TrimSpace(string([]rune(name)[:maxAlbumName]))
	}
	return name
}

// makeAlbums makes an album of the owner for each album of the run's exports
// and adds the photos of its folder. An album is the owner's album with the
// same name, so one in several exports, or imported again, is one album. It
// returns the number of albums filled, created or found, which stays the same
// when the matching runs again.
func (s *Service) makeAlbums(ctx context.Context, queries *store.Queries, run store.ImportRun) (int, error) {
	files, err := queries.ListTakeoutAlbumFiles(ctx, run.ID)
	if err != nil {
		return 0, fmt.Errorf("list album files: %w", err)
	}
	type folder struct{ export, path string }
	titles := map[folder][]string{}
	var folders []folder
	for _, f := range files {
		dir := albumDir(f.Path)
		name := albumName(f.Title)
		if dir == "" || name == "" {
			continue
		}
		k := folder{f.Export, dir}
		if titles[k] == nil {
			folders = append(folders, k)
		}
		titles[k] = append(titles[k], name)
	}

	albums := map[string]int64{}
	params := store.ListTakeoutAlbumPhotosParams{RunID: run.ID, OwnerID: run.OwnerID}
	for _, k := range folders {
		// A folder with more than one album file is not taken for an album.
		if len(titles[k]) != 1 {
			continue
		}
		name := titles[k][0]
		id, ok := albums[name]
		if !ok {
			id, err = queries.FindAlbumByName(ctx, store.FindAlbumByNameParams{OwnerID: run.OwnerID, Name: name})
			if errors.Is(err, pgx.ErrNoRows) {
				id, err = queries.CreateAlbum(ctx, store.CreateAlbumParams{OwnerID: run.OwnerID, Name: name})
			}
			if err != nil {
				return 0, fmt.Errorf("album %q: %w", name, err)
			}
			albums[name] = id
		}
		params.Exports = append(params.Exports, k.export)
		params.Folders = append(params.Folders, k.path)
		params.AlbumIds = append(params.AlbumIds, id)
	}
	if len(albums) == 0 {
		return 0, nil
	}

	rows, err := queries.ListTakeoutAlbumPhotos(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("list album photos: %w", err)
	}
	var add store.AddPhotosToAlbumsParams
	var restore []int64
	for _, row := range rows {
		if row.Trashed && run.AlbumTrash == AlbumTrashSkip {
			continue
		}
		add.AlbumIds = append(add.AlbumIds, row.AlbumID)
		add.PhotoIds = append(add.PhotoIds, row.PhotoID)
		// A Live Photo video comes back with its photo.
		if row.Trashed && row.Shown && run.AlbumTrash == AlbumTrashRestore {
			restore = append(restore, row.PhotoID)
		}
	}
	if err := queries.AddPhotosToAlbums(ctx, add); err != nil {
		return 0, fmt.Errorf("add album photos: %w", err)
	}
	if len(restore) > 0 {
		slices.Sort(restore)
		restore = slices.Compact(restore)
		if err := queries.RestorePhotos(ctx, store.RestorePhotosParams{OwnerID: run.OwnerID, Ids: restore}); err != nil {
			return 0, fmt.Errorf("restore album photos: %w", err)
		}
		if err := queries.CountRestoredPhotos(ctx, store.CountRestoredPhotosParams{ID: run.ID, Restored: int64(len(restore))}); err != nil {
			return 0, fmt.Errorf("count restored photos: %w", err)
		}
	}
	return len(albums), nil
}

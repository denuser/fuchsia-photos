package scan

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

func TestAlbumDir(t *testing.T) {
	for p, want := range map[string]string{
		albumFolder + "метаданные.json":                         albumFolder,
		"Takeout/Google Фото/названия-воспоминаний.json":        "",
		"Takeout/Google Фото/Поездка/Вложенная/метаданные.json": "",
		"Other/Google Фото/Поездка/метаданные.json":             "",
	} {
		if got := albumDir(p); got != want {
			t.Errorf("albumDir(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestAlbumName(t *testing.T) {
	long := strings.Repeat("я", 250)
	for title, want := range map[string]string{
		"  Поездка\n":  "Поездка",
		"Море\tи горы": "Море и горы",
		long:           strings.Repeat("я", 200),
		" \x00 ":       "",
	} {
		if got := albumName(title); got != want {
			t.Errorf("albumName(%q) = %q, want %q", title, got, want)
		}
	}
}

// albumPhotos returns the names of the owner's albums with the ids of their
// photos, in the order of the albums' ids.
func (e *env) albumPhotos() map[string][]int64 {
	e.t.Helper()
	rows, err := e.service.Ingest.Pool.Query(context.Background(), `
		SELECT a.name, coalesce(array_agg(ap.photo_id ORDER BY ap.photo_id) FILTER (WHERE ap.photo_id IS NOT NULL), '{}')
		FROM albums a LEFT JOIN album_photos ap ON ap.album_id = a.id
		WHERE a.owner_id = $1 GROUP BY a.id, a.name`, e.owner)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var name string
		var ids []int64
		if err := rows.Scan(&name, &ids); err != nil {
			e.t.Fatal(err)
		}
		if _, dup := out[name]; dup {
			e.t.Fatalf("two albums named %q", name)
		}
		out[name] = ids
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (e *env) photoID(data []byte) int64 {
	e.t.Helper()
	var id int64
	err := e.service.Ingest.Pool.QueryRow(context.Background(),
		"SELECT id FROM photos WHERE owner_id = $1 AND sha256 = sha256($2)", e.owner, data).Scan(&id)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestTakeoutMakesAlbums(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// The owner has an album of this name already, and one more made later.
	trip, err := e.queries.CreateAlbum(ctx, store.CreateAlbumParams{OwnerID: e.owner, Name: "Поездка"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.queries.CreateAlbum(ctx, store.CreateAlbumParams{OwnerID: e.owner, Name: "поездка"}); err != nil {
		t.Fatal(err)
	}

	files := append(e.export(),
		// Not albums: the list of memory titles and a JSON outside album folders.
		archiveFile{"Takeout/Google Фото/названия-воспоминаний-созданные-пользователем.json", []byte(`{"title": ["Лето", "Зима"]}`)},
		archiveFile{"Takeout/Google Фото/другое.json", []byte(`{"title": "Не альбом"}`)},
		// An album with nothing to add is made empty.
		archiveFile{"Takeout/Google Фото/Пустой/метаданные.json", []byte(`{"title": " Пустой ", "description": "x", "access": "protected"}`)},
		archiveFile{"Takeout/Google Фото/Пустой/notes.txt", []byte("not a photo")},
		// A folder with two album files is not an album.
		archiveFile{"Takeout/Google Фото/Два/метаданные.json", []byte(`{"title": "Два"}`)},
		archiveFile{"Takeout/Google Фото/Два/metadata.json", []byte(`{"title": "Два"}`)},
	)
	// The same album in a second export, with a photo of its own, and in an
	// unpacked folder.
	second := []archiveFile{
		{albumFolder + "метаданные.json", []byte(`{"title": "Поездка"}`)},
		{albumFolder + "IMG_2.jpg", e.fixture("rotated.jpg")},
	}
	e.write("takeout-20261115T073434Z-1-001.zip", makeZip(t, files))
	e.write("takeout-20261115T075702Z-1-001.tgz", makeTgz(t, second))
	for _, f := range second {
		e.write("unpacked/"+f.name, f.data)
	}

	run := e.finish(e.startKind("", KindTakeout).ID)
	if run.Status != "finished" || run.Albums != 2 {
		t.Fatalf("status %s, albums %d, want finished and 2", run.Status, run.Albums)
	}
	photo, edited := e.photoID(e.fixture("landscape.jpg")), e.photoID(e.fixture("rotated.jpg"))
	want := map[string][]int64{
		"Поездка": {photo, edited},
		"поездка": {},
		"Пустой":  {},
	}
	got := e.albumPhotos()
	for name, ids := range want {
		if !slices.Equal(got[name], ids) {
			t.Errorf("album %q has %v, want %v", name, got[name], ids)
		}
	}
	if len(got) != len(want) {
		t.Errorf("albums = %v", got)
	}
	var tripPhotos int
	if err := e.service.Ingest.Pool.QueryRow(ctx, "SELECT count(*) FROM album_photos WHERE album_id = $1", trip).Scan(&tripPhotos); err != nil || tripPhotos != 2 {
		t.Errorf("the older album has %d photos (%v)", tripPhotos, err)
	}

	t.Run("a second run adds nothing", func(t *testing.T) {
		run := e.finish(e.startKind("", KindTakeout).ID)
		if run.Albums != 2 {
			t.Errorf("albums = %d", run.Albums)
		}
		if got := e.albumPhotos(); len(got) != 3 || len(got["Поездка"]) != 2 {
			t.Errorf("albums = %v", got)
		}
	})
}

func TestTakeoutAlbumPhotosInTheTrash(t *testing.T) {
	for _, choice := range []string{AlbumTrashSkip, AlbumTrashAdd, AlbumTrashRestore} {
		t.Run(choice, func(t *testing.T) {
			e := setup(t)
			ctx := context.Background()
			photo := e.fixture("landscape.jpg")
			res, err := e.service.Ingest.Ingest(ctx, e.owner, bytes.NewReader(photo), "IMG_1.jpg", 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.queries.TrashPhotos(ctx, store.TrashPhotosParams{Ids: []int64{res.Photo.ID}, ActorID: e.owner}); err != nil {
				t.Fatal(err)
			}
			e.write("takeout-20261115T073434Z-1-001.zip", makeZip(t, []archiveFile{
				{albumFolder + "метаданные.json", []byte(`{"title": "Поездка"}`)},
				{albumFolder + "IMG_1.jpg", photo},
			}))

			run, err := e.service.Start(ctx, e.owner, e.owner, Options{Kind: KindTakeout, AlbumTrash: choice})
			if err != nil {
				t.Fatal(err)
			}
			run = e.finish(run.ID)

			album := e.albumPhotos()["Поездка"]
			if inAlbum := len(album) == 1; inAlbum != (choice != AlbumTrashSkip) {
				t.Errorf("album photos = %v", album)
			}
			var trashed bool
			if err := e.service.Ingest.Pool.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM photos WHERE id = $1", res.Photo.ID).Scan(&trashed); err != nil {
				t.Fatal(err)
			}
			restored := choice == AlbumTrashRestore
			if trashed == restored {
				t.Errorf("in the trash: %v", trashed)
			}
			if wantCount := map[bool]int64{true: 1}[restored]; run.Restored != wantCount {
				t.Errorf("restored = %d, want %d", run.Restored, wantCount)
			}
		})
	}

	t.Run("an unknown choice", func(t *testing.T) {
		e := setup(t)
		if _, err := e.service.Start(context.Background(), e.owner, e.owner, Options{Kind: KindTakeout, AlbumTrash: "keep"}); err != ErrNoAlbumTrash {
			t.Errorf("err = %v", err)
		}
	})
}

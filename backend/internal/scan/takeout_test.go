package scan

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// The names follow a real export of an account in Russian: year folders, the
// album file and the word for an edited copy are in the account's language,
// the sidecar suffix is not. Titles and contents are made up.
const (
	yearFolder  = "Takeout/Google Фото/Фото 2019 г./"
	albumFolder = "Takeout/Google Фото/Поездка/"
)

type archiveFile struct {
	name string
	data []byte
}

func (e *env) fixture(name string) []byte {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		e.t.Fatal(err)
	}
	return data
}

// export is a small Takeout export: a photo, its edited copy, the same photo in
// an album with its own JSON, the album's file and a page that is not a photo.
func (e *env) export() []archiveFile {
	photo := e.fixture("landscape.jpg")
	return []archiveFile{
		{yearFolder + "IMG_1.jpg", photo},
		{yearFolder + "IMG_1.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_1.jpg", "photoTakenTime": {"timestamp": "1569055192"}}`)},
		{yearFolder + "IMG_1-отредактировано.jpg", e.fixture("rotated.jpg")},
		{albumFolder + "IMG_1.jpg", photo},
		{albumFolder + "IMG_1.jpg.supplemental-metadata.json", []byte(`{"title": "IMG_1.jpg", "photoTakenTime": {"timestamp": "1569055192"}, "sharedAlbumComments": [{"liked": true}]}`)},
		{albumFolder + "метаданные.json", []byte(`{"title": "Поездка"}`)},
		{"Takeout/archive_browser.html", []byte("<html></html>")},
	}
}

func makeZip(t *testing.T, files []archiveFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := w.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTgz(t *testing.T, files []archiveFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	dirs := map[string]bool{}
	for _, f := range files {
		// Real parts list folders too; a reader must pass over them.
		if dir := filepath.Dir(f.name); !dirs[dir] {
			dirs[dir] = true
			if err := w.WriteHeader(&tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.WriteHeader(&tar.Header{Name: f.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(f.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type takeoutEntry struct {
	Export, Path, Source, Outcome string
	PhotoID                       *int64
	Sidecar                       []byte
	SidecarPath, OriginalPath     *string
}

func (e *env) entries(runID int64) []takeoutEntry {
	e.t.Helper()
	rows, err := e.service.Ingest.Pool.Query(context.Background(),
		"SELECT export, path, source, outcome, photo_id, sidecar, sidecar_path, original_path FROM takeout_entries WHERE run_id = $1 ORDER BY export, path", runID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []takeoutEntry
	for rows.Next() {
		var en takeoutEntry
		if err := rows.Scan(&en.Export, &en.Path, &en.Source, &en.Outcome, &en.PhotoID, &en.Sidecar, &en.SidecarPath, &en.OriginalPath); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, en)
	}
	if err := rows.Err(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func TestTakeoutReadsZipTgzAndFolder(t *testing.T) {
	e := setup(t)
	files := e.export()
	e.write("exports/takeout-20261115T073434Z-1-001.zip", makeZip(t, files))
	e.write("exports/takeout-20261115T073533Z-1-001.tgz", makeTgz(t, files))
	for _, f := range files {
		e.write("exports/"+f.name, f.data)
	}
	// Not part of an export: a loose photo, and an archive of something else.
	e.copyFixture("landscape.png", "exports/IMG_9.png")
	e.write("exports/other.zip", makeZip(t, []archiveFile{{"photo.png", e.fixture("landscape.png")}}))
	before := snapshot(t, e.dir)

	run := e.finish(e.startKind("exports", KindTakeout).ID)
	// Three exports of seven files: two photos added once, the other copies of
	// them duplicates, the page unsupported, three JSON files each.
	wantCounts(t, run, counts{found: 21, added: 2, duplicates: 7, unsupported: 3})
	if run.Sidecars != 9 || run.Kind != KindTakeout {
		t.Errorf("sidecars = %d, kind = %s", run.Sidecars, run.Kind)
	}
	if run.Edited != 3 || run.WithoutSidecar != 0 || run.UnmatchedSidecars != 0 {
		t.Errorf("edited = %d, without sidecar = %d, unmatched sidecars = %d", run.Edited, run.WithoutSidecar, run.UnmatchedSidecars)
	}
	if got := e.photoCount(); got != 2 {
		t.Errorf("photos = %d, want 2", got)
	}
	if snapshot(t, e.dir) != before {
		t.Error("the source folder changed")
	}
	if left, _ := os.ReadDir(e.service.Ingest.Layout.TempDir()); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}

	entries := e.entries(run.ID)
	exports := map[string]string{
		"exports/Takeout":                  "",
		"exports/takeout-20261115T073434Z": "exports/takeout-20261115T073434Z-1-001.zip",
		"exports/takeout-20261115T073533Z": "exports/takeout-20261115T073533Z-1-001.tgz",
	}
	byExport := map[string]map[string]takeoutEntry{}
	for _, en := range entries {
		source, ok := exports[en.Export]
		if !ok || en.Source != source {
			t.Fatalf("entry %+v: unknown export or source", en)
		}
		if byExport[en.Export] == nil {
			byExport[en.Export] = map[string]takeoutEntry{}
		}
		byExport[en.Export][en.Path] = en
	}
	for export, got := range byExport {
		if len(got) != len(files) {
			t.Errorf("%s: %d entries, want %d", export, len(got), len(files))
		}
		year, album := got[yearFolder+"IMG_1.jpg"], got[albumFolder+"IMG_1.jpg"]
		if year.PhotoID == nil || album.PhotoID == nil || *year.PhotoID != *album.PhotoID {
			t.Errorf("%s: the album's copy is not linked to the same photo: %v, %v", export, year.PhotoID, album.PhotoID)
		}
		if year.SidecarPath == nil || *year.SidecarPath != yearFolder+"IMG_1.jpg.supplemental-metadata.json" ||
			album.SidecarPath == nil || *album.SidecarPath != albumFolder+"IMG_1.jpg.supplemental-metadata.json" {
			t.Errorf("%s: sidecars = %v, %v", export, year.SidecarPath, album.SidecarPath)
		}
		edited := got[yearFolder+"IMG_1-отредактировано.jpg"]
		if edited.PhotoID == nil || *edited.PhotoID == *year.PhotoID {
			t.Errorf("%s: edited copy = %+v", export, edited)
		}
		if edited.SidecarPath != nil || edited.OriginalPath == nil || *edited.OriginalPath != yearFolder+"IMG_1.jpg" {
			t.Errorf("%s: edited copy matched to %v, %v", export, edited.SidecarPath, edited.OriginalPath)
		}
		if sidecar := got[albumFolder+"IMG_1.jpg.supplemental-metadata.json"]; sidecar.Outcome != "sidecar" || !bytes.Contains(sidecar.Sidecar, []byte("sharedAlbumComments")) {
			t.Errorf("%s: sidecar = %+v", export, sidecar)
		}
		if page := got["Takeout/archive_browser.html"]; page.Outcome != "unsupported" || page.PhotoID != nil {
			t.Errorf("%s: page = %+v", export, page)
		}
	}

	t.Run("a second run reads nothing new", func(t *testing.T) {
		again := e.finish(e.startKind("exports", KindTakeout).ID)
		wantCounts(t, again, counts{found: 21, duplicates: 9, unsupported: 3})
		if left, _ := os.ReadDir(e.service.Ingest.Layout.TempDir()); len(left) != 0 {
			t.Errorf("temporary files left: %v", left)
		}
	})
}

func TestTakeoutResumesInTheMiddleOfAPart(t *testing.T) {
	for _, kind := range []string{"zip", "tgz"} {
		t.Run(kind, func(t *testing.T) {
			e := setup(t)
			ctx := context.Background()
			// More files than one batch, so that a slice of zero stops partway.
			png := e.fixture("landscape.png")
			var files []archiveFile
			for i := range processBatch + 5 {
				files = append(files, archiveFile{fmt.Sprintf("%s%02d.png", yearFolder, i), append(png[:len(png):len(png)], byte(i))})
			}
			name := "takeout-20261115T073434Z-1-001." + kind
			if kind == "zip" {
				e.write(name, makeZip(t, files))
			} else {
				e.write(name, makeTgz(t, files))
			}

			run := e.startKind("", KindTakeout)
			if _, _, err := e.service.step(ctx, run.ID, 0); err != nil { // The listing.
				t.Fatal(err)
			}
			_, more, err := e.service.step(ctx, run.ID, 0) // One batch.
			if err != nil {
				t.Fatal(err)
			}
			if !more {
				t.Fatal("the scan finished in one slice")
			}
			if got := e.photoCount(); got != processBatch {
				t.Errorf("photos after one slice = %d, want %d", got, processBatch)
			}

			wantCounts(t, e.finish(run.ID), counts{found: processBatch + 5, added: processBatch + 5})
		})
	}
}

func TestTakeoutReportsProblems(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.write("takeout-20261115T073434Z-1-001.zip", makeZip(t, []archiveFile{
		{yearFolder + "IMG_1.jpg", e.fixture("landscape.jpg")},
		{yearFolder + "IMG_1.jpg.supplemental-metadata.json", []byte(`{"title": `)},
		{yearFolder + "IMG_2.jpg.supplemental-metadata.json", []byte(`{"title": "\u0000"}`)},
	}))
	e.write("takeout-20261115T073434Z-1-002.zip", []byte("not an archive"))
	e.write("takeout-20261115T073434Z-1-003.tgz", []byte("not gzip either"))

	run := e.finish(e.startKind("", KindTakeout).ID)
	wantCounts(t, run, counts{found: 3, added: 1, failed: 4})
	problems, err := e.queries.ListImportProblems(ctx, store.ListImportProblemsParams{RunID: run.ID, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"takeout-20261115T073434Z-1-001.zip/" + yearFolder + "IMG_1.jpg.supplemental-metadata.json",
		"takeout-20261115T073434Z-1-001.zip/" + yearFolder + "IMG_2.jpg.supplemental-metadata.json",
		"takeout-20261115T073434Z-1-002.zip",
		"takeout-20261115T073434Z-1-003.tgz",
	}
	if len(problems) != len(want) {
		t.Fatalf("problems = %+v", problems)
	}
	for i, p := range problems {
		if p.Path != want[i] || p.State != "failed" || p.Error == nil {
			t.Errorf("problem %d = %+v, want %s", i, p, want[i])
		}
	}
}

func TestTakeoutNames(t *testing.T) {
	exports := map[string]string{
		"a/takeout-20261115T073434Z-1-001.zip":     "a/takeout-20261115T073434Z",
		"a/takeout-20261115T073434Z-1-002.tgz":     "a/takeout-20261115T073434Z",
		"a/takeout-20261115T073434Z-1-001 (1).zip": "a/takeout-20261115T073434Z",
		"a/takeout-20200101T000000Z-003.tar.gz":    "a/takeout-20200101T000000Z",
		"Мой экспорт.ZIP":                          "Мой экспорт",
	}
	for p, want := range exports {
		if got := partExport(p); got != want {
			t.Errorf("partExport(%q) = %q, want %q", p, got, want)
		}
	}

	unpacked := []struct{ path, export, inner string }{
		{"a/Takeout/Google Фото/x.jpg", "a/Takeout", "Takeout/Google Фото/x.jpg"},
		{"Takeout/Google Фото/Takeout/x.jpg", "Takeout", "Takeout/Google Фото/Takeout/x.jpg"},
		{"a/x.jpg", "", ""},
		{"a/Takeout", "", ""},
	}
	for _, u := range unpacked {
		export, inner, ok := splitUnpacked(u.path)
		if export != u.export || inner != u.inner || ok != (u.export != "") {
			t.Errorf("splitUnpacked(%q) = %q, %q, %v", u.path, export, inner, ok)
		}
	}
}

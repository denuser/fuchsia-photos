package scan

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// wholeExport is a small export split the way Google splits one: photos in the
// first part and most of their JSON in the second, with names cut short and
// repeated, an edited copy, and an album whose file and copy of a photo are in
// the second part while the photo's year copy is in the first. The first part
// holds more entries than one batch, with a photo before the end of the batch
// and its JSON after it.
type wholeExport struct {
	parts [2][]archiveFile
	// takenAt is the photoTakenTime of each photo's JSON, by the photo's path;
	// an edited copy takes its original's.
	takenAt map[string]int64
	// sidecar is the JSON each photo must be matched to.
	sidecar map[string]string
	data    map[string][]byte
}

const (
	cutSuffix  = "Screenshot_20200101-123456_Some_Very_Long_App.png"
	cutName    = "Very long original name that Goo.png"
	editedName = "IMG_1-отредактировано.jpg"
	fillers    = 20
)

func (e *env) wholeExport() wholeExport {
	png := e.fixture("landscape.png")
	variant := func(i int) []byte { return append(slices.Clone(png), byte(i)) }
	w := wholeExport{takenAt: map[string]int64{}, sidecar: map[string]string{}, data: map[string][]byte{}}
	next := int64(1569055192)
	json := func(title string, taken int64) []byte {
		return fmt.Appendf(nil, `{"title": %q, "photoTakenTime": {"timestamp": "%d"}}`, title, taken)
	}
	// photo adds a photo to the first part and its JSON to the given part.
	photo := func(name string, data []byte, sidecarName, title string, part int) {
		next += 1000
		path := yearFolder + name
		w.parts[0] = append(w.parts[0], archiveFile{path, data})
		w.parts[part] = append(w.parts[part], archiveFile{yearFolder + sidecarName, json(title, next)})
		w.takenAt[path], w.sidecar[path], w.data[path] = next, yearFolder+sidecarName, data
	}

	photo("IMG_1.jpg", e.fixture("landscape.jpg"), "IMG_1.jpg.supplemental-metadata.json", "IMG_1.jpg", 1)
	w.parts[0] = append(w.parts[0], archiveFile{yearFolder + editedName, e.fixture("rotated.jpg")})
	w.takenAt[yearFolder+editedName], w.data[yearFolder+editedName] = w.takenAt[yearFolder+"IMG_1.jpg"], e.fixture("rotated.jpg")
	// Two files of one name in one folder, told apart only by the "(1)".
	photo("IMG_2.png", variant(1), "IMG_2.png.supplemental-metadata.json", "IMG_2.png", 1)
	photo("IMG_2(1).png", variant(2), "IMG_2.png.supplemental-metadata(1).json", "IMG_2.png", 1)
	// A suffix cut short, and a file name cut short.
	photo(cutSuffix, variant(3), cutSuffix+".supplemen.json", cutSuffix, 1)
	photo(cutName, variant(4), "Very long original name that Goo.json", "Very long original name that Google cut short.png", 1)
	// Photos that fill the first batch: the ones before its end have their JSON
	// after it, in the same part.
	for i := range fillers {
		name := fmt.Sprintf("F_%02d.png", i)
		photo(name, variant(10+i), name+".supplemental-metadata.json", name, 0)
	}
	// Move the fillers' JSON after all their photos, so that the first batch
	// ends between a photo and its JSON.
	slices.SortStableFunc(w.parts[0], func(a, b archiveFile) int {
		ja, jb := isJSON(a.name), isJSON(b.name)
		switch {
		case ja == jb:
			return 0
		case jb:
			return -1
		default:
			return 1
		}
	})

	// The album: its file and its copy of IMG_1 are in the second part.
	album := albumFolder + "IMG_1.jpg"
	w.parts[1] = append(w.parts[1],
		archiveFile{albumFolder + "метаданные.json", []byte(`{"title": "Поездка"}`)},
		archiveFile{album, e.fixture("landscape.jpg")},
		archiveFile{album + ".supplemental-metadata.json", json("IMG_1.jpg", w.takenAt[yearFolder+"IMG_1.jpg"])},
		archiveFile{"Takeout/archive_browser.html", []byte("<html></html>")},
	)
	w.takenAt[album], w.sidecar[album], w.data[album] = w.takenAt[yearFolder+"IMG_1.jpg"], album+".supplemental-metadata.json", e.fixture("landscape.jpg")
	return w
}

func isJSON(name string) bool { return strings.HasSuffix(name, ".json") }

// check tests what a run of the whole export recorded, whatever form it was
// read in.
func (e *env) check(w wholeExport, run int64, export string) {
	t := e.t
	t.Helper()
	ctx := context.Background()
	got, err := e.queries.GetImportRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	// 6 photos and 20 fillers once, the album's copy a duplicate, the page not
	// a photo; every JSON counted, the album's file included.
	wantCounts(t, got, counts{found: 55, added: 26, duplicates: 1, unsupported: 1})
	if got.Sidecars != 27 || got.Edited != 1 || got.WithoutSidecar != 0 || got.UnmatchedSidecars != 0 || got.Albums != 1 {
		t.Errorf("sidecars = %d, edited = %d, without sidecar = %d, unmatched sidecars = %d, albums = %d",
			got.Sidecars, got.Edited, got.WithoutSidecar, got.UnmatchedSidecars, got.Albums)
	}

	entries := map[string]takeoutEntry{}
	for _, en := range e.entries(run) {
		if en.Export != export {
			t.Errorf("entry %s in export %s, want %s", en.Path, en.Export, export)
		}
		entries[en.Path] = en
	}
	for path, want := range w.sidecar {
		if en := entries[path]; en.SidecarPath == nil || *en.SidecarPath != want {
			t.Errorf("%s matched to %v, want %s", path, en.SidecarPath, want)
		}
	}
	if en := entries[yearFolder+editedName]; en.SidecarPath != nil || en.OriginalPath == nil || *en.OriginalPath != yearFolder+"IMG_1.jpg" {
		t.Errorf("edited copy matched to %v, %v", en.SidecarPath, en.OriginalPath)
	}

	original := e.photoID(w.data[yearFolder+"IMG_1.jpg"])
	for path, taken := range w.takenAt {
		id := e.photoID(w.data[path])
		var metaExport string
		var takenAt time.Time
		var originalID *int64
		err := e.service.Ingest.Pool.QueryRow(ctx,
			"SELECT export, taken_at, original_id FROM imported_metadata WHERE photo_id = $1", id).Scan(&metaExport, &takenAt, &originalID)
		if err != nil {
			t.Errorf("%s: no imported metadata: %v", path, err)
			continue
		}
		if metaExport != export || !takenAt.Equal(time.Unix(taken, 0)) {
			t.Errorf("%s: metadata from %s taken at %v, want %s and %v", path, metaExport, takenAt, export, time.Unix(taken, 0))
		}
		if wantEdited := path == yearFolder+editedName; wantEdited != (originalID != nil) || (wantEdited && *originalID != original) {
			t.Errorf("%s: original = %v", path, originalID)
		}
	}

	albums := e.albumPhotos()
	if len(albums) != 1 || !slices.Equal(albums["Поездка"], []int64{original}) {
		t.Errorf("albums = %v, want Поездка with %d", albums, original)
	}
}

// TestTakeoutWholeExport reads one small export in each form a user may have
// it in: two .zip parts, two .tgz parts read with a restart in the middle of
// the first, and both parts unpacked into one folder.
func TestTakeoutWholeExport(t *testing.T) {
	const export = "takeout-20261115T073434Z"

	t.Run("zip", func(t *testing.T) {
		e := setup(t)
		w := e.wholeExport()
		e.write(export+"-1-001.zip", makeZip(t, w.parts[0]))
		e.write(export+"-1-002.zip", makeZip(t, w.parts[1]))
		e.check(w, e.finish(e.startKind("", KindTakeout).ID).ID, export)
	})

	t.Run("tgz with a restart", func(t *testing.T) {
		e := setup(t)
		ctx := context.Background()
		w := e.wholeExport()
		e.write(export+"-1-001.tgz", makeTgz(t, w.parts[0]))
		e.write(export+"-1-002.tgz", makeTgz(t, w.parts[1]))

		run := e.startKind("", KindTakeout)
		if _, _, err := e.service.step(ctx, run.ID, 0); err != nil { // The listing.
			t.Fatal(err)
		}
		if _, more, err := e.service.step(ctx, run.ID, 0); err != nil || !more {
			t.Fatalf("one slice: more = %v, err = %v", more, err)
		}
		if n := len(e.entries(run.ID)); n != processBatch {
			t.Fatalf("entries after one slice = %d, want %d", n, processBatch)
		}
		// A new service, as after a restart of the application, keeps nothing
		// in memory from the slice before.
		e.service = &Service{Ingest: e.service.Ingest, Dir: e.dir}
		e.check(w, e.finish(run.ID).ID, export)
	})

	t.Run("folder", func(t *testing.T) {
		e := setup(t)
		w := e.wholeExport()
		for _, part := range w.parts {
			for _, f := range part {
				e.write(f.name, f.data)
			}
		}
		e.check(w, e.finish(e.startKind("", KindTakeout).ID).ID, "Takeout")
	})
}

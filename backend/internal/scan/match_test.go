package scan

import (
	"maps"
	"testing"
)

func TestMatchExport(t *testing.T) {
	const year, album = "Takeout/Google Фото/Фото 2019 г./", "Takeout/Google Фото/Поездка/"
	media := func(p string) matchFile { return matchFile{path: p, media: true, imported: true} }
	sidecar := func(p, title string) matchFile { return matchFile{path: p, photoSidecar: true, title: title} }
	long := "Screenshot_20200101-123456_Some_Very_Long_App.jpg"
	files := []matchFile{
		// The usual pair, and the same name in an album with a JSON of its own.
		media(year + "IMG_1.jpg"),
		sidecar(year+"IMG_1.jpg.supplemental-metadata.json", "IMG_1.jpg"),
		media(album + "IMG_1.jpg"),
		sidecar(album+"IMG_1.jpg.supplemental-metadata.json", "IMG_1.jpg"),
		// Two files of one name in one folder.
		media(year + "IMG_2(1).jpg"),
		media(year + "IMG_2.jpg"),
		sidecar(year+"IMG_2.jpg.supplemental-metadata(1).json", "IMG_2.jpg"),
		sidecar(year+"IMG_2.jpg.supplemental-metadata.json", "IMG_2.jpg"),
		// A name whose own "(1)" is not Google's.
		media(year + "photo (1).jpg"),
		sidecar(year+"photo (1).jpg.supplemental-metadata.json", "photo (1).jpg"),
		// A suffix cut short, and a file name cut short.
		media(year + long),
		sidecar(year+long+".supplemen.json", long),
		media(year + "Very long original name that Goo.jpg"),
		sidecar(year+"Very long original name that Goo.json", "Very long original name that Google cut short.jpg"),
		// An older export without a title, and the suffix gone entirely.
		media(year + "IMG_3.png"),
		sidecar(year+"IMG_3.png.json", ""),
		media(year + "IMG_4.jpg"),
		sidecar(year+"IMG_4.jpg.supplem.json", ""),
		// An edited copy, in the language of the account.
		media(year + "20190921-_MG_5983.jpg"),
		sidecar(year+"20190921-_MG_5983.jpg.supplemental-metadata.json", "20190921-_MG_5983.jpg"),
		media(year + "20190921-_MG_5983-отредактировано.jpg"),
		// Edited copies of a repeated name, with the word before or after the "(n)".
		media(year + "IMG_2(1)-edited.jpg"),
		media(year + "IMG_2-edited(1).jpg"),
		// The album's own file, a JSON without a file, and a file without a JSON.
		{path: album + "метаданные.json", title: "Поездка"},
		sidecar(year+"IMG_9.jpg.supplemental-metadata.json", "IMG_9.jpg"),
		media(year + "IMG_8.png"),
		// A page that is not a photo is not counted as missing its JSON.
		{path: "Takeout/archive_browser.html", media: true},
	}

	got := matchExport(files)
	wantSidecar := map[string]string{
		year + "IMG_1.jpg":     year + "IMG_1.jpg.supplemental-metadata.json",
		album + "IMG_1.jpg":    album + "IMG_1.jpg.supplemental-metadata.json",
		year + "IMG_2(1).jpg":  year + "IMG_2.jpg.supplemental-metadata(1).json",
		year + "IMG_2.jpg":     year + "IMG_2.jpg.supplemental-metadata.json",
		year + "photo (1).jpg": year + "photo (1).jpg.supplemental-metadata.json",
		year + long:            year + long + ".supplemen.json",
		year + "Very long original name that Goo.jpg": year + "Very long original name that Goo.json",
		year + "IMG_3.png":             year + "IMG_3.png.json",
		year + "IMG_4.jpg":             year + "IMG_4.jpg.supplem.json",
		year + "20190921-_MG_5983.jpg": year + "20190921-_MG_5983.jpg.supplemental-metadata.json",
	}
	if !maps.Equal(got.sidecar, wantSidecar) {
		for k, v := range got.sidecar {
			if wantSidecar[k] != v {
				t.Errorf("sidecar of %s = %s, want %s", k, v, wantSidecar[k])
			}
		}
		for k, v := range wantSidecar {
			if _, ok := got.sidecar[k]; !ok {
				t.Errorf("no sidecar for %s, want %s", k, v)
			}
		}
	}
	wantOriginal := map[string]string{
		year + "20190921-_MG_5983-отредактировано.jpg": year + "20190921-_MG_5983.jpg",
		year + "IMG_2(1)-edited.jpg":                   year + "IMG_2(1).jpg",
		year + "IMG_2-edited(1).jpg":                   year + "IMG_2(1).jpg",
	}
	if !maps.Equal(got.original, wantOriginal) {
		t.Errorf("originals = %v, want %v", got.original, wantOriginal)
	}
	if got.withoutSidecar != 1 || got.unmatchedSidecars != 1 {
		t.Errorf("without sidecar = %d, unmatched sidecars = %d, want 1 and 1", got.withoutSidecar, got.unmatchedSidecars)
	}
}

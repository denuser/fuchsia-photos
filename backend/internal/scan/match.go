package scan

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// The suffix of a photo's JSON file before ".json". Long names cut it short,
// down to nothing.
const sidecarWord = "supplemental-metadata"

// copyCounter matches the "(1)" Google adds when one folder holds two files of
// the same name: IMG_1(1).jpg, whose JSON is IMG_1.jpg.supplemental-metadata(1).json.
var copyCounter = regexp.MustCompile(`^(.*)\((\d+)\)$`)

func splitCounter(s string) (rest, counter string) {
	if m := copyCounter.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return s, ""
}

// matchFile is a file of one export as the matching sees it.
type matchFile struct {
	path string
	// media is a photo, a video or another file that is not JSON.
	media bool
	// imported is a media file that is in the library, not unsupported.
	imported bool
	// photoSidecar is the JSON file of a photo, as opposed to that of an album
	// or a list of titles. title is the file name it describes.
	photoSidecar bool
	title        string
}

// matches is what the matching found in one export.
type matches struct {
	// sidecar maps a media file to its JSON file, and original maps an edited
	// copy to the file it was made from.
	sidecar, original map[string]string
	// withoutSidecar counts imported files with neither a JSON file nor an
	// original; unmatchedSidecars counts JSON files of photos without a file.
	withoutSidecar, unmatchedSidecars int
}

type mediaName struct {
	dir, name    string // name is the whole file name
	stem, ext    string // name split at its extension
	base, number string // stem without its "(n)", and n
}

func parseMedia(p string) mediaName {
	dir, name := path.Split(p)
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	base, number := splitCounter(stem)
	return mediaName{dir: dir, name: name, stem: stem, ext: ext, base: base, number: number}
}

// matchExport matches the files of one export. A JSON file is found for a file
// in the same folder in this order:
//  1. by its whole name: IMG_1.jpg.supplemental-metadata.json;
//  2. by the title inside the JSON and the "(n)" of both names, which covers
//     files of the same name and suffixes cut short;
//  3. by a title that the file's own name was cut from, as Google cuts long
//     file names;
//  4. by its name: the file's name followed by a cut suffix, for JSON without
//     a title.
//
// A file still without one, whose name less its last "-" part names another
// file of the folder with the same extension, is an edited copy of that file:
// IMG_1-edited.jpg, with the word in the language of the account.
func matchExport(files []matchFile) matches {
	result := matches{sidecar: map[string]string{}, original: map[string]string{}}

	byPath := map[string]matchFile{}
	sidecars := map[string][]matchFile{} // by folder
	var media []mediaName
	for _, f := range files {
		byPath[f.path] = f
		switch {
		case f.media:
			media = append(media, parseMedia(f.path))
		case f.photoSidecar:
			dir, _ := path.Split(f.path)
			sidecars[dir] = append(sidecars[dir], f)
		}
	}
	used := map[string]bool{}
	match := func(m mediaName, sidecar string) {
		result.sidecar[m.dir+m.name] = sidecar
		used[sidecar] = true
	}
	free := func(m mediaName) []matchFile {
		var out []matchFile
		for _, s := range sidecars[m.dir] {
			if !used[s.path] {
				out = append(out, s)
			}
		}
		return out
	}

	// 1. The whole name.
	for _, m := range media {
		p := m.dir + m.name + "." + sidecarWord + ".json"
		if f, ok := byPath[p]; ok && f.photoSidecar {
			match(m, p)
		}
	}

	// 2. The title and the "(n)".
	type key struct{ dir, name, number string }
	byTitle := map[key][]string{}
	for dir, list := range sidecars {
		for _, s := range list {
			_, name := path.Split(s.path)
			_, number := splitCounter(strings.TrimSuffix(name, ".json"))
			k := key{dir, s.title, number}
			byTitle[k] = append(byTitle[k], s.path)
		}
	}
	for _, m := range media {
		if _, done := result.sidecar[m.dir+m.name]; done {
			continue
		}
		for _, k := range []key{{m.dir, m.base + m.ext, m.number}, {m.dir, m.name, ""}} {
			if candidates := unused(byTitle[k], used); len(candidates) == 1 {
				match(m, candidates[0])
				break
			}
		}
	}

	// 3. A title the file's name was cut from, and 4. the file's name and a cut suffix.
	for _, rule := range []func(mediaName, matchFile) bool{cutFrom, nameAndCutSuffix} {
		for _, m := range media {
			if _, done := result.sidecar[m.dir+m.name]; done {
				continue
			}
			var found []string
			for _, s := range free(m) {
				if rule(m, s) {
					found = append(found, s.path)
				}
			}
			if len(found) == 1 {
				match(m, found[0])
			}
		}
	}

	// Edited copies.
	names := map[string]bool{}
	for _, m := range media {
		names[m.dir+m.name] = true
	}
	for _, m := range media {
		if _, done := result.sidecar[m.dir+m.name]; done {
			continue
		}
		if original := originalOf(m, names); original != "" {
			result.original[m.dir+m.name] = original
			continue
		}
		if byPath[m.dir+m.name].imported {
			result.withoutSidecar++
		}
	}

	for _, list := range sidecars {
		for _, s := range list {
			if !used[s.path] {
				result.unmatchedSidecars++
			}
		}
	}
	return result
}

func unused(paths []string, used map[string]bool) []string {
	var out []string
	for _, p := range paths {
		if !used[p] {
			out = append(out, p)
		}
	}
	return out
}

// cutFrom tells whether the file's name is the JSON's title cut short, with
// the same extension and "(n)".
func cutFrom(m mediaName, s matchFile) bool {
	if s.title == "" || path.Ext(s.title) != m.ext {
		return false
	}
	_, name := path.Split(s.path)
	_, number := splitCounter(strings.TrimSuffix(name, ".json"))
	titleStem := strings.TrimSuffix(s.title, m.ext)
	return number == m.number && len(m.base) < len(titleStem) && strings.HasPrefix(titleStem, m.base)
}

// nameAndCutSuffix tells whether the JSON's name is the file's name followed
// by the suffix cut short, or by nothing, as older exports named it.
func nameAndCutSuffix(m mediaName, s matchFile) bool {
	_, name := path.Split(s.path)
	rest, number := splitCounter(strings.TrimSuffix(name, ".json"))
	whole := m.base + m.ext
	if number != m.number {
		whole, number = m.name, ""
		rest = strings.TrimSuffix(name, ".json")
	}
	if !strings.HasPrefix(rest, whole) {
		return false
	}
	suffix := rest[len(whole):]
	return suffix == "" || (strings.HasPrefix(suffix, ".") && strings.HasPrefix(sidecarWord, suffix[1:]))
}

// originalOf returns the file an edited copy was made from, or "". The word
// that marks the copy is the last "-" part of its name, before or after its "(n)".
func originalOf(m mediaName, names map[string]bool) string {
	var candidates []string
	if i := strings.LastIndex(m.stem, "-"); i > 0 {
		candidates = append(candidates, m.dir+m.stem[:i]+m.ext)
	}
	if m.number != "" {
		if i := strings.LastIndex(m.base, "-"); i > 0 {
			candidates = append(candidates, m.dir+m.base[:i]+"("+m.number+")"+m.ext)
		}
	}
	for _, c := range candidates {
		if names[c] && c != m.dir+m.name {
			return c
		}
	}
	return ""
}

// match matches the entries of every export of a run and finishes it. It reads
// only the database, so it can run again after an interruption.
func (s *Service) match(ctx context.Context, run store.ImportRun) (bool, error) {
	queries := s.queries()
	rows, err := queries.ListTakeoutEntriesForMatching(ctx, run.ID)
	if err != nil {
		return false, fmt.Errorf("list entries: %w", err)
	}
	exports := map[string][]matchFile{}
	for _, row := range rows {
		f := matchFile{path: row.Path}
		if row.Outcome == "sidecar" {
			f.photoSidecar, f.title = row.Photo, row.Title
		} else {
			f.media, f.imported = true, row.Outcome != "unsupported"
		}
		exports[row.Export] = append(exports[row.Export], f)
	}

	params := store.SetTakeoutMatchesParams{RunID: run.ID}
	finish := store.FinishMatchingParams{ID: run.ID}
	for export, files := range exports {
		found := matchExport(files)
		for _, f := range files {
			sidecar, original := found.sidecar[f.path], found.original[f.path]
			if !f.media {
				continue
			}
			params.Exports = append(params.Exports, export)
			params.Paths = append(params.Paths, f.path)
			params.SidecarPaths = append(params.SidecarPaths, sidecar)
			params.OriginalPaths = append(params.OriginalPaths, original)
		}
		finish.WithoutSidecar += int64(found.withoutSidecar)
		finish.UnmatchedSidecars += int64(found.unmatchedSidecars)
		finish.Edited += int64(len(found.original))
	}

	tx, err := s.Ingest.Pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := store.New(tx).SetTakeoutMatches(ctx, params); err != nil {
		return false, fmt.Errorf("record matches: %w", err)
	}
	if err := store.New(tx).FinishMatching(ctx, finish); err != nil {
		return false, fmt.Errorf("finish run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return false, nil
}

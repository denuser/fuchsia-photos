package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckSource(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	layout := Layout{Folders: root}
	write := func(p string) string {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
		return full
	}
	link := func(target, name string) string {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, full); err != nil {
			t.Fatal(err)
		}
		return full
	}

	real := write("lightroom/a.jpg")
	secret := filepath.Join(outside, "secret.webp")
	if err := os.WriteFile(secret, []byte("private"), 0o640); err != nil {
		t.Fatal(err)
	}
	other := write("archive/b.jpg")

	tests := []struct {
		name string
		path string
		ok   bool
	}{
		{"a regular file", real, true},
		{"a link to a file outside the mount", link(secret, "lightroom/outside.jpg"), false},
		{"a link to a file of another folder", link(other, "lightroom/other.jpg"), false},
		{"a file behind a link to a folder", filepath.Join(link(outside, "lightroom/dir"), "secret.webp"), false},
		{"a folder", filepath.Join(root, "lightroom"), false},
		{"the mount", root, false},
		{"outside the mount", secret, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := layout.CheckSource(tt.path)
			if tt.ok && err != nil {
				t.Errorf("err = %v, want none", err)
			}
			if !tt.ok && err == nil {
				t.Error("a file that must not be read passed the check")
			}
			if _, err := layout.OpenSource(tt.path); (err == nil) != tt.ok {
				t.Errorf("OpenSource err = %v, want ok = %v", err, tt.ok)
			}
		})
	}
	if err := layout.CheckSource(filepath.Join(root, "missing.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: err = %v, want not exist", err)
	}
}

func TestUnavailableOnlyForLibraryFolderFiles(t *testing.T) {
	layout := Layout{Library: t.TempDir(), Folders: t.TempDir()}
	if layout.Unavailable(nil, filepath.Join(layout.Library, "gone")) {
		t.Error("a file of the library was reported unavailable: it is an error, not a wait")
	}
	source := "lightroom/a.jpg"
	if !layout.Unavailable(&source, filepath.Join(layout.Folders, source)) {
		t.Error("a missing file of a library folder was reported available")
	}
}

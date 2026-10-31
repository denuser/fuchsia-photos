// Package storage defines where files live on disk.
package storage

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Preview kinds.
const (
	PreviewGrid   = "grid"
	PreviewViewer = "viewer"
)

var extensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/heic": ".heic",
}

// Layout holds the root folders of the application's files.
type Layout struct {
	Library string
	Cache   string
}

// Ensure creates the folders that must exist before any file is written.
func (l Layout) Ensure() error {
	for _, dir := range []string{l.TempDir(), filepath.Join(l.Library, "originals"), filepath.Join(l.Cache, "previews")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// TempDir is on the same filesystem as the originals, so that a finished upload
// can be moved into place atomically.
func (l Layout) TempDir() string {
	return filepath.Join(l.Library, "tmp")
}

// Original returns the path of an original photo.
func (l Layout) Original(ownerID int64, sha256 []byte, mime string) string {
	sum := hex.EncodeToString(sha256)
	return filepath.Join(l.Library, "originals", fmt.Sprint(ownerID), sum[:2], sum+extensions[mime])
}

// Preview returns the path of a preview. The format is the file extension.
func (l Layout) Preview(photoID int64, kind, format string) string {
	return filepath.Join(l.Cache, "previews", fmt.Sprintf("%02x", photoID%256), fmt.Sprintf("%d-%s.%s", photoID, kind, format))
}

// WriteFileAtomic writes data so that path never holds a partial file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

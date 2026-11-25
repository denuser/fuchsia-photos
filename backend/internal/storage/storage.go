// Package storage defines where files live on disk.
package storage

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Preview kinds.
const (
	PreviewGrid   = "grid"
	PreviewViewer = "viewer"
)

var extensions = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"image/heic":      ".heic",
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
}

// Layout holds the root folders of the application's files.
type Layout struct {
	Library string
	Cache   string
	// Folders is the read-only mount of library folders. A photo whose
	// original is there has a source path relative to it.
	Folders string
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
	return filepath.Join(l.Library, "originals", fmt.Sprint(ownerID), sum[:2], sum+extension(mime))
}

// OriginalOf returns the path of a photo's original: its file in a library
// folder when it has one, else the copy in the library. sourcePath is the
// photo's source path, nil for a photo stored in the library.
func (l Layout) OriginalOf(ownerID int64, sha256 []byte, mime string, sourcePath *string) string {
	if sourcePath != nil {
		return filepath.Join(l.Folders, filepath.FromSlash(*sourcePath))
	}
	return l.Original(ownerID, sha256, mime)
}

// ErrUnsafeSource means a file of a library folder is, or lies behind, a symbolic link, or is not
// a regular file. The scan never indexes a link, so a source path through one means somebody
// changed the share after the scan, as a way to read files of the application or of others.
var ErrUnsafeSource = errors.New("the file in the library folder is not a regular file under its folder")

// CheckSource tells whether path, a file under the library folders mount, may be read: every part
// of it below the mount is a real folder and the file is a regular file. The mount itself is
// resolved once, as it may be a link, such as /var on macOS.
func (l Layout) CheckSource(path string) error {
	rel, err := filepath.Rel(l.Folders, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnsafeSource
	}
	cur, err := filepath.EvalSymlinks(l.Folders)
	if err != nil {
		return err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeSource
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return ErrUnsafeSource
		}
	}
	return nil
}

// OpenSource opens a file of a library folder after CheckSource, and without following a link at
// the last step, which closes most of the gap between the check and the open.
func (l Layout) OpenSource(path string) (*os.File, error) {
	if err := l.CheckSource(path); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

// IsSource tells whether path is under the library folders mount.
func (l Layout) IsSource(path string) bool {
	return l.Folders != "" && strings.HasPrefix(path, l.Folders+string(filepath.Separator))
}

// Unavailable tells whether an original that lives in a library folder cannot
// be read now, as when the share is not mounted, the file is away or it was
// replaced by a link. An original in the library is never reported: if it is
// gone, that is an error.
func (l Layout) Unavailable(sourcePath *string, path string) bool {
	if sourcePath == nil {
		return false
	}
	return l.CheckSource(path) != nil
}

func extension(mime string) string {
	if IsRAW(mime) {
		return rawExtensions[mime]
	}
	return extensions[mime]
}

// Preview returns the path of a preview. The format is the file extension.
func (l Layout) Preview(photoID int64, kind, format string) string {
	return filepath.Join(l.Cache, "previews", fmt.Sprintf("%02x", photoID%256), fmt.Sprintf("%d-%s.%s", photoID, kind, format))
}

// Motion returns the path of the video extracted from a motion photo. Like
// the previews, it can be made again from the original.
func (l Layout) Motion(photoID int64) string {
	return filepath.Join(l.Cache, "previews", fmt.Sprintf("%02x", photoID%256), fmt.Sprintf("%d-motion.mp4", photoID))
}

// Copy returns the path of the H.264 copy of what an item plays: a video, or
// the motion of a motion photo or a Live Photo. It is made from the original
// on graphics that can, and is not backed up.
func (l Layout) Copy(photoID int64) string {
	return filepath.Join(l.Cache, "previews", fmt.Sprintf("%02x", photoID%256), fmt.Sprintf("%d-copy.mp4", photoID))
}

// IsVideo tells whether a MIME type of the library is a video.
func IsVideo(mime string) bool {
	return strings.HasPrefix(mime, "video/")
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

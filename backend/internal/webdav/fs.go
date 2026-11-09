package webdav

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dav "golang.org/x/net/webdav"

	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const pgUniqueViolation = "23505"

// fileSystem is one user's tree, for one request. Files have no contents here:
// uploads and downloads are served apart from x/net/webdav, which uses this
// for listings, folders, moves and deletes.
type fileSystem struct {
	queries *store.Queries
	userID  int64
	// listed holds the entries of the folders listed in this request. A
	// PROPFIND opens and stats every entry it lists, and this spares a query each.
	listed map[string]store.WebdavEntry
}

var _ dav.FileSystem = (*fileSystem)(nil)

func (f *fileSystem) entry(ctx context.Context, name string) (store.WebdavEntry, error) {
	name = cleanPath(name)
	if name == "/" {
		return store.WebdavEntry{UserID: f.userID, Path: "/", IsDir: true, ModifiedAt: time.Unix(0, 0)}, nil
	}
	if entry, ok := f.listed[name]; ok {
		return entry, nil
	}
	entry, err := f.queries.GetWebDAVEntry(ctx, store.GetWebDAVEntryParams{UserID: f.userID, Path: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.WebdavEntry{}, os.ErrNotExist
	}
	return entry, err
}

// parentFolder checks that the folder that would hold name exists.
func (f *fileSystem) parentFolder(ctx context.Context, name string) error {
	parent, err := f.entry(ctx, path.Dir(name))
	if err != nil {
		return err
	}
	if !parent.IsDir {
		return os.ErrNotExist
	}
	return nil
}

func (f *fileSystem) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	name = cleanPath(name)
	if !validPath(name) {
		return os.ErrInvalid
	}
	if err := f.parentFolder(ctx, name); err != nil {
		return err
	}
	err := f.queries.CreateWebDAVDir(ctx, store.CreateWebDAVDirParams{UserID: f.userID, Path: name, Parent: path.Dir(name)})
	if isUniqueViolation(err) {
		return os.ErrExist
	}
	return err
}

// OpenFile opens an entry for reading its details or listing a folder. Writing
// goes through the PUT handler, so it is refused here.
func (f *fileSystem) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (dav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		return nil, os.ErrPermission
	}
	entry, err := f.entry(ctx, name)
	if err != nil {
		return nil, err
	}
	return &file{ctx: ctx, fs: f, entry: entry}, nil
}

// RemoveAll deletes an entry and, for a folder, everything in it. The photos
// stay in the library: clearing space on a phone must not delete its photos here.
func (f *fileSystem) RemoveAll(ctx context.Context, name string) error {
	name = cleanPath(name)
	if name == "/" {
		return os.ErrPermission
	}
	_, err := f.queries.DeleteWebDAVTree(ctx, store.DeleteWebDAVTreeParams{UserID: f.userID, Path: name})
	return err
}

// Rename moves an entry. Some clients upload under a temporary name and rename
// the file when it is complete.
func (f *fileSystem) Rename(ctx context.Context, oldName, newName string) error {
	oldName, newName = cleanPath(oldName), cleanPath(newName)
	if oldName == "/" || !validPath(newName) || strings.HasPrefix(newName, oldName+"/") {
		return os.ErrInvalid
	}
	if _, err := f.entry(ctx, oldName); err != nil {
		return err
	}
	if err := f.parentFolder(ctx, newName); err != nil {
		return err
	}
	_, err := f.queries.MoveWebDAVTree(ctx, store.MoveWebDAVTreeParams{
		UserID:    f.userID,
		OldPath:   oldName,
		NewPath:   newName,
		NewParent: path.Dir(newName),
	})
	if isUniqueViolation(err) {
		return os.ErrExist
	}
	return err
}

func (f *fileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	entry, err := f.entry(ctx, name)
	if err != nil {
		return nil, err
	}
	return fileInfo{entry}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// file is an entry opened for its details or its listing. It has no contents.
type file struct {
	ctx   context.Context
	fs    *fileSystem
	entry store.WebdavEntry
	read  bool
}

var errNoContents = errors.New("file contents are not served here")

func (f *file) Close() error                   { return nil }
func (f *file) Read([]byte) (int, error)       { return 0, errNoContents }
func (f *file) Seek(int64, int) (int64, error) { return 0, errNoContents }
func (f *file) Write([]byte) (int, error)      { return 0, os.ErrPermission }
func (f *file) Stat() (os.FileInfo, error)     { return fileInfo{f.entry}, nil }

// Readdir lists a folder. Its entries are kept for the rest of the request.
func (f *file) Readdir(count int) ([]os.FileInfo, error) {
	if !f.entry.IsDir {
		return nil, os.ErrInvalid
	}
	if f.read {
		if count > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	f.read = true
	entries, err := f.fs.queries.ListWebDAVChildren(f.ctx, store.ListWebDAVChildrenParams{UserID: f.fs.userID, Parent: f.entry.Path})
	if err != nil {
		return nil, err
	}
	if f.fs.listed == nil {
		f.fs.listed = make(map[string]store.WebdavEntry, len(entries))
	}
	infos := make([]os.FileInfo, len(entries))
	for i, entry := range entries {
		f.fs.listed[entry.Path] = entry
		infos[i] = fileInfo{entry}
	}
	return infos, nil
}

type fileInfo struct {
	entry store.WebdavEntry
}

var _ dav.ContentTyper = fileInfo{}

func (i fileInfo) Name() string       { return path.Base(i.entry.Path) }
func (i fileInfo) Size() int64        { return i.entry.Size }
func (i fileInfo) ModTime() time.Time { return i.entry.ModifiedAt }
func (i fileInfo) IsDir() bool        { return i.entry.IsDir }
func (i fileInfo) Sys() any           { return nil }

func (i fileInfo) Mode() fs.FileMode {
	if i.entry.IsDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

// ContentType saves x/net/webdav from opening the file to sniff its type.
func (i fileInfo) ContentType(context.Context) (string, error) {
	if t := mime.TypeByExtension(path.Ext(i.entry.Path)); t != "" {
		return t, nil
	}
	return "application/octet-stream", nil
}

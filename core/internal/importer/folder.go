package importer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mole/core/internal/library"
)

// FolderOptions say how ImportFolder treats a folder.
type FolderOptions struct {
	// Collections puts each book in a collection named after the folder
	// that holds it.
	Collections bool
	// Watch keeps the folder in sync: Sync imports what changes in it later.
	Watch bool
}

// FolderResult says what a pass over a folder did.
type FolderResult struct {
	Folder  int64     // the watched folder, or 0 when the folder is not watched
	Found   int       // supported files found
	Added   []int64   // books new to the library
	Updated []int64   // books whose file changed, or was renamed or moved
	Missing []int64   // books whose file left the watched folder
	Failed  []Failure // files that could not be imported
}

// Failure is a file that could not be imported, and why.
type Failure struct {
	Path string
	Err  error
}

// ImportFolder imports every supported file in dir and its subfolders,
// skipping hidden files and the library's own Books directory. With
// opts.Collections every book found, new or not, also goes into the
// collection of its folder. With opts.Watch the folder is kept in sync from
// then on (see Sync), and this first pass already marks the books whose
// file is gone.
//
// A file that cannot be imported is reported in the result and does not
// stop the others.
func ImportFolder(store *library.Store, dir string, opts FolderOptions, now time.Time) (FolderResult, error) {
	root, err := folderRoot(store, dir)
	if err != nil {
		return FolderResult{}, err
	}
	var folder library.Folder
	if opts.Watch {
		if folder, err = store.AddFolder(root, opts.Collections, now); err != nil {
			return FolderResult{}, err
		}
	}
	p := pass{store: store, root: root, now: now, collections: opts.Collections, everyBook: true, marks: opts.Watch}
	r, err := p.run()
	if err != nil {
		return FolderResult{}, err
	}
	if opts.Watch {
		r.Folder = folder.ID
		err = store.FolderSynced(folder.ID, now)
	}
	return r, err
}

// Sync brings the library up to date with a watched folder: it imports the
// files that are new or changed, puts those books in their folder's
// collection when the folder makes collections, and marks the books whose
// file is gone. Books are never removed; a book whose file comes back loses
// its mark.
//
// Collections are only filled with books the pass touched, so a book the
// user took out of a collection, or a collection the user deleted, stays
// that way until its file changes.
func Sync(store *library.Store, folderID int64, now time.Time) (FolderResult, error) {
	folder, err := store.Folder(folderID)
	if err != nil {
		return FolderResult{}, err
	}
	if info, err := os.Stat(folder.Path); err != nil || !info.IsDir() {
		// A folder that went away (an unplugged disk, say) is not a reason
		// to mark all of its books.
		return FolderResult{}, fmt.Errorf("sync %s: the folder is not there", folder.Path)
	}
	p := pass{store: store, root: folder.Path, now: now, collections: folder.Collections, marks: true}
	r, err := p.run()
	if err != nil {
		return FolderResult{}, err
	}
	r.Folder = folder.ID
	return r, store.FolderSynced(folder.ID, now)
}

// folderRoot returns dir as an absolute path without symbolic links, the
// way file system events name it, or an error if it is not a folder or lies
// inside the library's Books directory.
func folderRoot(store *library.Store, dir string) (string, error) {
	root, err := realPath(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("import %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("import %s: not a folder", root)
	}
	if books := realBooksDir(store); root == books || library.Within(root, books) {
		return "", fmt.Errorf("import %s: the folder is inside the library", root)
	}
	return root, nil
}

// realBooksDir returns the library's Books directory with symbolic links
// resolved, as a walk from a resolved root names it.
func realBooksDir(store *library.Store) string {
	if dir, err := realPath(store.BooksDir()); err == nil {
		return dir
	}
	return store.BooksDir()
}

// pass is one walk over a folder.
type pass struct {
	store       *library.Store
	root        string
	now         time.Time
	collections bool // put books in their folder's collection
	everyBook   bool // put every book found in a collection, not only new and changed ones
	marks       bool // mark the books whose file is gone
}

func (p pass) run() (FolderResult, error) {
	var r FolderResult
	files, failed, err := p.files()
	if err != nil {
		return FolderResult{}, err
	}
	r.Failed = failed
	r.Found = len(files)
	for _, file := range files {
		got, err := importFile(p.store, file, p.now)
		if err != nil {
			r.Failed = append(r.Failed, Failure{Path: file, Err: err})
			continue
		}
		switch got.change {
		case added:
			r.Added = append(r.Added, got.book.ID)
		case replaced, moved:
			r.Updated = append(r.Updated, got.book.ID)
		}
		if p.collections && (p.everyBook || got.change != unchanged) {
			if err := p.collect(got, file); err != nil {
				return FolderResult{}, err
			}
		}
	}
	if p.marks {
		missing, err := p.mark()
		if err != nil {
			return FolderResult{}, err
		}
		r.Missing = missing
	}
	return r, nil
}

// files lists the supported files under the root in lexical order, with the
// subfolders that could not be read.
func (p pass) files() ([]string, []Failure, error) {
	var files []string
	var failed []Failure
	books := realBooksDir(p.store)
	err := filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == p.root {
				return err
			}
			failed = append(failed, Failure{Path: path, Err: err})
			return nil
		}
		if path == p.root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || path == books {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if Supported(path) {
				// An unpacked book, which only a zipped one can replace.
				return fs.SkipDir
			}
			return nil
		}
		if !Supported(path) {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				return nil
			}
		} else if !d.Type().IsRegular() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", p.root, err)
	}
	return files, failed, nil
}

// collect puts the book imported from file in the collection of its folder.
// A book moved from another folder leaves the collection made from that one.
func (p pass) collect(got imported, file string) error {
	dir := filepath.Dir(file)
	c, err := p.store.MakeCollectionOfFolder(dir, p.now)
	if err != nil {
		return err
	}
	if err := p.store.AddToCollection(c.ID, got.book.ID, p.now); err != nil {
		return err
	}
	if got.change != moved || filepath.Dir(got.from) == dir {
		return nil
	}
	old, err := p.store.CollectionOfFolder(filepath.Dir(got.from))
	if errors.Is(err, library.ErrCollectionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return p.store.RemoveFromCollection(old.ID, got.book.ID)
}

// mark marks the books from under the root whose file is gone, clears the
// mark of those whose file is back, and records the stamp of files that had
// none. It returns every book under the root that carries the mark.
func (p pass) mark() ([]int64, error) {
	books, err := p.store.BooksUnder(p.root)
	if err != nil {
		return nil, err
	}
	var missing []int64
	for _, b := range books {
		info, statErr := os.Stat(b.OriginalPath)
		var err error
		switch {
		case errors.Is(statErr, fs.ErrNotExist):
			missing = append(missing, b.ID)
			if !b.Missing {
				err = p.store.SetMissing(b.ID, true)
			}
		case statErr != nil:
			// Unreadable is not gone; the book stays as it is.
		case !b.Source.Known():
			err = p.store.SetSource(b.ID, b.OriginalPath, stampOf(info))
		case b.Missing:
			err = p.store.SetMissing(b.ID, false)
		}
		if err != nil {
			return nil, err
		}
	}
	return missing, nil
}

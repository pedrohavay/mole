// Package importer adds books to the library: it copies them into the
// library's Books directory and reads their metadata and cover, falling back
// to the first page when no cover is declared. It takes the formats of
// package bookfile: EPUB, MOBI, PRC, FB2 and plain text. It also imports
// whole folders and keeps watched folders in sync.
package importer

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"mole/core/internal/bookfile"
	"mole/core/internal/library"
)

// maxNameLength is the longest file name, in bytes, kept from the original
// when naming the library's copy.
const maxNameLength = 100

// Import adds the book at path to store and returns the stored book. The
// library keeps a copy of the file, so the book survives the original being
// moved or deleted. A book protected by DRM is refused with an error that
// wraps bookfile.ErrDRM.
//
// The original's path, size and modification time identify it. A file
// imported before is returned without being read again; one that changed
// since replaces the book's copy and metadata, and one that was renamed or
// moved is recognized by its size and modification time when its old path
// is gone. Either way the book keeps its ID, place and annotations, so the
// library never holds two copies of one file.
//
// Import is safe to call from any goroutine.
func Import(store *library.Store, path string, now time.Time) (library.Book, error) {
	r, err := importFile(store, path, now)
	return r.book, err
}

// Supported reports whether Import reads the file at path, judging by its
// name: the extension of one of the formats of package bookfile. Folder
// imports skip the files it rejects.
func Supported(path string) bool {
	_, ok := bookfile.FormatOf(path)
	return ok
}

// change says what an import did to the library.
type change int

const (
	unchanged change = iota // the file was imported before, as it is now
	added                   // the book is new to the library
	replaced                // the file changed and replaced the book's copy
	moved                   // the file was renamed or moved
)

// imported is the outcome of importing one file.
type imported struct {
	book   library.Book
	change change
	from   string // the original path before a move
}

func importFile(store *library.Store, path string, now time.Time) (imported, error) {
	abs, err := realPath(path)
	if err != nil {
		return imported{}, err
	}
	info, statErr := os.Stat(abs)
	if statErr == nil && info.IsDir() {
		return imported{}, fmt.Errorf("import %s: is a folder", abs)
	}
	known, err := store.BookByOriginalPath(abs)
	switch {
	case err == nil:
		if statErr != nil {
			// The original is gone; the library's copy is still the book.
			return imported{book: known}, nil
		}
		return refresh(store, known, stampOf(info))
	case !errors.Is(err, library.ErrNotFound):
		return imported{}, err
	case statErr != nil:
		return imported{}, fmt.Errorf("import %s: %w", abs, statErr)
	}
	st := stampOf(info)
	if b, ok, err := movedBook(store, st); err != nil || ok {
		if err != nil {
			return imported{}, err
		}
		if err := store.SetSource(b.ID, abs, st); err != nil {
			return imported{}, err
		}
		from := b.OriginalPath
		b.OriginalPath, b.Source, b.Missing = abs, st, false
		return imported{book: b, change: moved, from: from}, nil
	}
	read, err := readBook(store, abs, st)
	if err != nil {
		return imported{}, err
	}
	b, err := store.Add(read, now)
	if err != nil || b.Path != read.Path {
		// The book was not added, or another import of the same file won.
		os.Remove(read.Path)
	}
	return imported{book: b, change: added}, err
}

// refresh brings book up to date with its original, which has the stamp st
// now: a changed file replaces the copy, and an unknown stamp is recorded.
func refresh(store *library.Store, book library.Book, st library.Stamp) (imported, error) {
	if book.Source.Equal(st) && !book.Missing {
		return imported{book: book}, nil
	}
	if !book.Source.Known() || book.Source.Equal(st) {
		// Imported before stamps were kept, or back where it was.
		if err := store.SetSource(book.ID, book.OriginalPath, st); err != nil {
			return imported{}, err
		}
		book.Source, book.Missing = st, false
		return imported{book: book}, nil
	}
	read, err := readBook(store, book.OriginalPath, st)
	if err != nil {
		return imported{}, err
	}
	// Replace deletes the new copy itself if the book was not updated.
	b, err := store.Replace(book.ID, read)
	if err != nil {
		return imported{}, err
	}
	return imported{book: b, change: replaced}, nil
}

// movedBook returns the book whose original had the stamp st and is no
// longer where it was, which makes a file with st that book's original,
// renamed or moved.
func movedBook(store *library.Store, st library.Stamp) (library.Book, bool, error) {
	candidates, err := store.BooksBySource(st)
	if err != nil {
		return library.Book{}, false, err
	}
	for _, b := range candidates {
		if _, err := os.Stat(b.OriginalPath); errors.Is(err, fs.ErrNotExist) {
			return b, true, nil
		}
	}
	return library.Book{}, false, nil
}

// readBook copies the book at abs into the library, under the extension of
// its format, and reads its metadata and cover from the copy. The copy is
// removed if the file cannot be read.
func readBook(store *library.Store, abs string, st library.Stamp) (library.Book, error) {
	format, err := bookfile.Detect(abs)
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	copied, err := copyInto(store.BooksDir(), abs, format.Extension(abs))
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	info, err := bookfile.Inspect(copied)
	if err != nil {
		os.Remove(copied)
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	if info.Title == "" {
		info.Title = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	}
	return library.Book{
		Path: copied, OriginalPath: abs, Title: info.Title, Author: info.Author, Cover: info.Cover, Source: st,
	}, nil
}

// realPath returns path as an absolute path without symbolic links, so one
// file has one name whichever way it is reached (/tmp is /private/tmp). A
// path that does not exist keeps its last element as it is.
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(dir, filepath.Base(abs)), nil
	}
	return abs, nil
}

func stampOf(info fs.FileInfo) library.Stamp {
	return library.Stamp{Size: info.Size(), ModTime: info.ModTime()}
}

// copyInto copies the file at path into dir under a new, unique name made
// from its own and ending in ext, and returns the path of the copy.
func copyInto(dir, path, ext string) (copied string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.CreateTemp(dir, copyName(path, ext))
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			dst.Close()
			os.Remove(dst.Name())
		}
	}()
	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy into the library: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("copy into the library: %w", err)
	}
	return dst.Name(), nil
}

// copyName returns the os.CreateTemp pattern for the copy of path: its name,
// shortened, then a random number and the extension ext.
func copyName(path, ext string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, "*", "")
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
		for len(name) > 0 && !utf8.ValidString(name) {
			name = name[:len(name)-1] // cut inside a character
		}
	}
	if name == "" {
		name = "book"
	}
	return name + "-*" + ext
}

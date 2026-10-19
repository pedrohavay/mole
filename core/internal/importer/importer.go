// Package importer adds books to the library: it copies them into the
// library's Books directory and reads their metadata and cover, falling back
// to the first page when no cover is declared. It takes the formats of
// package bookfile: EPUB, MOBI, PRC, FB2 and plain text.
package importer

import (
	"errors"
	"fmt"
	"io"
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
// moved or deleted. A file that was already imported is returned unchanged,
// without being read again. A book protected by DRM is refused with an
// error that wraps bookfile.ErrDRM.
//
// Import is safe to call from any goroutine.
func Import(store *library.Store, path string, now time.Time) (library.Book, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return library.Book{}, err
	}
	if b, err := store.BookByOriginalPath(abs); !errors.Is(err, library.ErrNotFound) {
		return b, err
	}
	info, err := bookfile.Inspect(abs)
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	if info.Title == "" {
		info.Title = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	}
	copied, err := copyInto(store.BooksDir(), abs, info.Format.Extension(abs))
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	b, err := store.Add(library.Book{
		Path: copied, OriginalPath: abs, Title: info.Title, Author: info.Author, Cover: info.Cover,
	}, now)
	if err != nil || b.Path != copied {
		// The book was not added, or another import of the same file won.
		os.Remove(copied)
	}
	return b, err
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

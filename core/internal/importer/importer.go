// Package importer adds EPUB files to the library: it copies them into the
// library's Books directory and reads their metadata and cover, falling back
// to the first page when no cover is declared.
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

	"mole/core/internal/epub"
	"mole/core/internal/library"
	"mole/core/internal/mupdf"
)

// maxNameLength is the longest file name, in bytes, kept from the original
// when naming the library's copy.
const maxNameLength = 100

// Import adds the EPUB at path to store and returns the stored book. The
// library keeps a copy of the file, so the book survives the original being
// moved or deleted. A file that was already imported is returned unchanged,
// without being read again.
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
	info, err := epub.Inspect(abs)
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	if info.Title == "" {
		info.Title = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	}
	copied, err := copyInto(store.BooksDir(), abs)
	if err != nil {
		return library.Book{}, fmt.Errorf("import %s: %w", abs, err)
	}
	if info.Cover == nil {
		// The cover is decorative; a book without one is still imported.
		if cover, err := firstPage(copied); err == nil {
			info.Cover = cover
		}
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
// from its own, and returns the path of the copy.
func copyInto(dir, path string) (copied string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.CreateTemp(dir, copyName(path))
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
// shortened, then a random number and the .epub extension.
func copyName(path string) string {
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
	return name + "-*.epub"
}

// firstPage renders the first page, laid out at a cover's proportions.
func firstPage(path string) ([]byte, error) {
	doc, err := mupdf.Open(path, "")
	if err != nil {
		return nil, err
	}
	defer doc.Close()
	if _, err := doc.Layout(600, 900, 16); err != nil {
		return nil, err
	}
	return doc.RenderPNG(0, 0.5)
}

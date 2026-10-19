// Package bookfile reads the kinds of book files Mole supports: EPUB, MOBI
// (with PRC, its PalmDOC ancestor), FictionBook 2 and plain text. It tells
// a file's format from its content and extension, reads its title, author
// and cover, and opens it with MuPDF for layout.
package bookfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mole/core/internal/epub"
	"mole/core/internal/mupdf"
)

// Format is the kind of a book file.
type Format int

// The formats Mole reads.
const (
	EPUB Format = iota + 1
	MOBI        // Mobipocket, and PalmDOC (PRC) books
	FB2         // FictionBook 2
	Text        // plain text
)

// extensions lists the file extensions of each format, the usual one first.
var extensions = map[Format][]string{
	EPUB: {".epub"},
	MOBI: {".mobi", ".prc"},
	FB2:  {".fb2"},
	Text: {".txt", ".text"},
}

// Extension returns the extension, with its dot, for a copy of the file at
// path in format f: path's own when it is one of f's, else f's usual one.
func (f Format) Extension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if slices.Contains(extensions[f], ext) {
		return ext
	}
	if len(extensions[f]) == 0 {
		return ""
	}
	return extensions[f][0]
}

var (
	// ErrUnsupported is returned for a file that is not a book in a
	// format Mole reads.
	ErrUnsupported = errors.New("not a book in a format Mole reads")
	// ErrDRM is returned for a book whose text is encrypted by DRM.
	ErrDRM = errors.New("this book is protected by DRM")
)

// sniffSize is how much of the start of a file Detect looks at.
const sniffSize = 4096

// Detect returns the format of the book at path. Content decides first: a
// ZIP archive is an EPUB, a Palm database of a book is a MOBI and an XML
// document with a FictionBook element is an FB2. Files with no such mark
// are an EPUB or plain text by their extension, and text must not look
// binary. It returns ErrUnsupported for anything else.
func Detect(path string) (Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	head := make([]byte, sniffSize)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	if format, ok := detect(head[:n], filepath.Ext(path)); ok {
		return format, nil
	}
	return 0, fmt.Errorf("%s: %w", filepath.Base(path), ErrUnsupported)
}

// detect returns the format of a file that starts with head and has the
// extension ext.
func detect(head []byte, ext string) (Format, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return EPUB, true
	case isPalmBook(head):
		return MOBI, true
	case isFictionBook(head):
		return FB2, true
	}
	switch strings.ToLower(ext) {
	case ".epub":
		return EPUB, true
	case ".txt", ".text":
		return Text, looksLikeText(head)
	}
	return 0, false
}

// isFictionBook reports whether head starts an XML document whose root is
// a FictionBook element.
func isFictionBook(head []byte) bool {
	head = bytes.TrimPrefix(head, utf8BOM)
	return bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n"), []byte("<")) &&
		bytes.Contains(head, []byte("<FictionBook"))
}

// looksLikeText reports whether head can start a text file: it has no NUL
// bytes, unless a byte order mark says it is UTF-16.
func looksLikeText(head []byte) bool {
	if bytes.HasPrefix(head, utf16LEBOM) || bytes.HasPrefix(head, utf16BEBOM) {
		return true
	}
	return bytes.IndexByte(head, 0) < 0
}

// Info is the metadata of a book.
type Info struct {
	Format Format
	Title  string // "" when the book has none
	Author string // "" when the book has none
	// Cover is an encoded image (JPEG, PNG, GIF, ...): the cover the book
	// declares or else its first page, or nil when neither can be had.
	Cover []byte
}

// Inspect reads the format, the metadata and the cover of the book at
// path. A book that is not an EPUB is also opened with MuPDF, which checks
// that it can be read and supplies a title the format's own metadata
// lacks. It returns ErrUnsupported for a file in no known format and ErrDRM
// for a protected book.
func Inspect(path string) (Info, error) {
	format, err := Detect(path)
	if err != nil {
		return Info{}, err
	}
	info := Info{Format: format}
	switch format {
	case EPUB:
		meta, err := epub.Inspect(path)
		if err != nil {
			return Info{}, err
		}
		info.Title, info.Author, info.Cover = meta.Title, meta.Author, meta.Cover
		if info.Cover == nil {
			// The cover is decorative: a book whose first page cannot be
			// drawn is still added, and fails when it is opened.
			info.Cover, _ = firstPage(path)
		}
		return info, nil
	case MOBI:
		err = inspectMOBI(path, &info)
	case FB2:
		err = inspectFB2(path, &info)
	case Text:
		err = inspectText(path, &info)
	}
	if err != nil {
		return Info{}, err
	}
	doc, err := Open(path, "")
	if err != nil {
		return Info{}, err
	}
	defer doc.Close()
	if info.Title == "" {
		info.Title = clean(doc.Metadata(mupdf.MetaTitle))
	}
	if info.Author == "" {
		info.Author = clean(doc.Metadata(mupdf.MetaAuthor))
	}
	if info.Cover == nil {
		info.Cover, _ = renderFirstPage(doc) // decorative, as above
	}
	return info, nil
}

// formatCSS holds styles added over the books of a format. MuPDF breaks
// the page before an element only when the element is below the top of
// the page, and margins above it put it there: a MOBI that starts with a
// page break, as many do, and every FB2, whose section titles break the
// page, would start with an empty page, and an FB2 with a cover would have
// empty pages after it. These styles remove the margins that cause it.
var formatCSS = map[Format]string{
	MOBI: "body{margin-top:0 !important}",
	FB2:  "FictionBook{margin-top:0 !important}title>p{margin-top:0 !important}coverpage>image{margin:0 !important}",
}

// Open opens the book at path with MuPDF. userCSS, when not empty, is
// applied over the book's own styles. Plain text is turned into HTML with
// paragraphs first (see textHTML). It returns ErrDRM for a protected book;
// a file of no known format is left to MuPDF.
func Open(path, userCSS string) (*mupdf.Document, error) {
	format, err := Detect(path)
	if err != nil && !errors.Is(err, ErrUnsupported) {
		return nil, err
	}
	userCSS = formatCSS[format] + userCSS
	switch format {
	case MOBI:
		if err := checkMOBI(path); err != nil {
			return nil, err
		}
	case Text:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		doc, err := mupdf.OpenBuffer(textHTML(decodeText(data)), "html", userCSS)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		return doc, nil
	}
	return mupdf.Open(path, userCSS)
}

// firstPage renders the first page of the book at path, as for a cover.
func firstPage(path string) ([]byte, error) {
	doc, err := Open(path, "")
	if err != nil {
		return nil, err
	}
	defer doc.Close()
	return renderFirstPage(doc)
}

// renderFirstPage renders the first page of doc, laid out at a cover's
// proportions, as PNG.
func renderFirstPage(doc *mupdf.Document) ([]byte, error) {
	if _, err := doc.Layout(600, 900, 16); err != nil {
		return nil, err
	}
	return doc.RenderPNG(0, 0.5)
}

// clean returns s with runs of whitespace collapsed into single spaces and
// none at either end.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

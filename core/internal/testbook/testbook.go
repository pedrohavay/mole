// Package testbook writes small EPUB files for tests, so that tests of the
// layout, import and library code run without a real book.
package testbook

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Book describes an EPUB to write.
type Book struct {
	Title    string
	Chapters []string          // the HTML body of each document of the spine
	Files    map[string][]byte // more files, such as images, by path under OEBPS/
}

// Write writes the book into a new temporary directory of t and returns the
// path of the EPUB.
func (b Book) Write(t testing.TB) string {
	t.Helper()
	return b.WriteTo(t, filepath.Join(t.TempDir(), "book.epub"))
}

// WriteTo writes the book at file and returns file.
func (b Book) WriteTo(t testing.TB, file string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name string, content []byte) {
		t.Helper()
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	add("mimetype", []byte("application/epub+zip"))
	add("META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`))
	var manifest, spine strings.Builder
	for i, body := range b.Chapters {
		name := fmt.Sprintf("c%d.xhtml", i)
		fmt.Fprintf(&manifest, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, name)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, i)
		add("OEBPS/"+name, []byte(`<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>c</title></head><body>`+body+`</body></html>`))
	}
	i := 0
	for name, content := range b.Files {
		media := "application/octet-stream"
		switch path.Ext(name) {
		case ".png":
			media = "image/png"
		case ".jpg", ".jpeg":
			media = "image/jpeg"
		case ".svg":
			media = "image/svg+xml"
		}
		fmt.Fprintf(&manifest, `<item id="f%d" href="%s" media-type="%s"/>`, i, name, media)
		add("OEBPS/"+name, content)
		i++
	}
	title := b.Title
	if title == "" {
		title = "Test"
	}
	add("OEBPS/content.opf", []byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="3.0">
  <metadata><dc:title>`+title+`</dc:title></metadata>
  <manifest>`+manifest.String()+`</manifest>
  <spine>`+spine.String()+`</spine>
</package>`))
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write epub: %v", err)
	}
	return file
}

// Chapters returns the path of a book with the given chapters.
func Chapters(t testing.TB, chapters ...string) string {
	t.Helper()
	return Book{Chapters: chapters}.Write(t)
}

// PNG encodes img.
func PNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

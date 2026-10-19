package mupdf

import (
	"bytes"
	"testing"

	"mole/core/internal/testbook"
)

// TestFixedPages checks that a PDF keeps its pages whatever the layout
// asks for, and that each page is a chapter of its own.
func TestFixedPages(t *testing.T) {
	sample := testbook.SamplePDF()
	doc, err := Open(sample.Write(t), "")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer doc.Close()
	if !doc.Fixed() || doc.NeedsPassword() {
		t.Errorf("Fixed, NeedsPassword = %v, %v; want true, false", doc.Fixed(), doc.NeedsPassword())
	}
	for _, size := range [][2]float64{{300, 400}, {900, 600}} {
		pages, err := doc.Layout(size[0], size[1], 20)
		if err != nil || pages != len(sample.Pages) {
			t.Fatalf("Layout(%v) = %d, %v; want %d pages", size, pages, err, len(sample.Pages))
		}
	}
	if n := doc.Chapters(); n != len(sample.Pages) {
		t.Errorf("Chapters() = %d, want a chapter per page, %d", n, len(sample.Pages))
	}
	for page := range len(sample.Pages) {
		chapter, index, ok := doc.Location(page)
		if !ok || chapter != page || index != 0 {
			t.Errorf("Location(%d) = %d, %d, %v; want chapter %d, index 0", page, chapter, index, ok, page)
		}
		if n, ok := doc.ChapterPages(page); !ok || n != 1 {
			t.Errorf("ChapterPages(%d) = %d, %v; want 1", page, n, ok)
		}
		if got, ok := doc.PageAt(page, 0.7); !ok || got != page {
			t.Errorf("PageAt(%d, 0.7) = %d, %v; want %d", page, got, ok, page)
		}
		if c, fraction, ok := doc.ChapterPosition(page); !ok || c != page || fraction != 0 {
			t.Errorf("ChapterPosition(%d) = %d, %v, %v; want %d, 0", page, c, fraction, ok, page)
		}
	}
	if w, h, err := doc.PageSize(1); err != nil || w != 612 || h != 792 {
		t.Errorf("PageSize(1) = %v x %v, %v; want 612 x 792", w, h, err)
	}
	if language := doc.Language(); language != "en" {
		t.Errorf("Language() = %q, want %q", language, "en")
	}
	outline, err := doc.Outline()
	if err != nil || len(outline) != len(sample.Outline) {
		t.Fatalf("Outline() = %+v, %v; want %d entries", outline, err, len(sample.Outline))
	}
	for i, item := range outline {
		want := sample.Outline[i]
		page, ok := doc.Resolve(item.URI)
		if item.Title != want.Title || item.Level != want.Level || !ok || page != want.Page {
			t.Errorf("outline entry %d = %+v at page %d (%v); want %+v", i, item, page, ok, want)
		}
	}
	if chars, err := doc.PageChars(1); err != nil || len(chars) < 100 || chars[0].Box.Y0 < 60 || chars[0].Box.Y0 > 90 {
		t.Errorf("PageChars(1) = %d characters, first at %+v, %v; want the text an inch from the top", len(chars), chars, err)
	}
}

func TestPasswordPDF(t *testing.T) {
	sample := testbook.SamplePDF()
	sample.UserPassword = "secret"
	doc, err := Open(sample.Write(t), "")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer doc.Close()
	if !doc.NeedsPassword() {
		t.Error("NeedsPassword() = false for a PDF with a user password")
	}
}

// TestPageCountLimit checks that a PDF that claims more pages than a book
// has fails to lay out instead of making room for all of them.
func TestPageCountLimit(t *testing.T) {
	data := bytes.Replace(testbook.SamplePDF().Bytes(), []byte("/Count 6"), []byte("/Count 900000000"), 1)
	doc, err := Open(testbook.File(t, "huge.pdf", data), "")
	if err != nil {
		return // refusing the file is as good
	}
	defer doc.Close()
	// MuPDF takes the count from the file.
	if pages, err := doc.Layout(600, 800, 16); err == nil {
		t.Errorf("Layout = %d pages, want an error past 100000", pages)
	}
}

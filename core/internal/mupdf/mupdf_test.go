package mupdf

import (
	"bytes"
	"os"
	"testing"
)

// TestDocument exercises the shim against a real book. Set MOLE_TEST_EPUB to
// the path of an EPUB file to run it.
func TestDocument(t *testing.T) {
	path := os.Getenv("MOLE_TEST_EPUB")
	if path == "" {
		t.Skip("MOLE_TEST_EPUB not set")
	}
	doc, err := Open(path, "body * { color: black !important; }")
	if err != nil {
		t.Fatalf("Open(%q) error: %v", path, err)
	}
	t.Cleanup(doc.Close)

	pages, err := doc.Layout(586, 940, 21)
	if err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if pages < 100 {
		t.Errorf("Layout(586, 940, 21) = %d pages, want at least 100", pages)
	}
	if title := doc.Metadata(MetaTitle); title == "" {
		t.Error("Metadata(MetaTitle) is empty")
	}
	outline, err := doc.Outline()
	if err != nil {
		t.Fatalf("Outline error: %v", err)
	}
	if len(outline) < 10 {
		t.Fatalf("Outline has %d items, want a full table of contents", len(outline))
	}
	previous := -1
	for _, item := range outline[:10] {
		page, ok := doc.Resolve(item.URI)
		if !ok || page < previous {
			t.Errorf("Resolve(%q) for %q = %d, %v; want a page after %d", item.URI, item.Title, page, ok, previous)
		}
		previous = page
	}
	t.Logf("title %q, %d pages, %d outline items, first %q", doc.Metadata(MetaTitle), pages, len(outline), outline[0].Title)

	const scale, stride = 2.0, 1280
	width, height := 1172, 1880
	mask := make([]byte, stride*height)
	images := make([]byte, stride*height)
	if _, err := doc.RenderLayers(80, scale, mask, images, 0, width, height, stride); err != nil {
		t.Fatalf("RenderLayers error: %v", err)
	}
	ink := 0
	for _, b := range mask {
		if b > 128 {
			ink++
		}
	}
	if ink == 0 || ink > len(mask)/2 {
		t.Errorf("RenderLayers(80) inked %d of %d pixels, want some ink on a mostly empty page", ink, len(mask))
	}
	if _, err := doc.RenderLayers(80, scale, mask, images, len(mask)-10, width, height, stride); err == nil {
		t.Error("RenderLayers with a region past the layers: got nil error, want error")
	}

	png, err := doc.RenderPNG(0, 0.3)
	if err != nil {
		t.Fatalf("RenderPNG error: %v", err)
	}
	if !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Errorf("RenderPNG(0) = %d bytes without a PNG signature", len(png))
	}

	// Locations and the text of a page.
	inChapter, index, ok := doc.Location(80)
	if !ok {
		t.Fatal("Location(80) failed")
	}
	if page, ok := doc.PageNumber(inChapter, index); !ok || page != 80 {
		t.Errorf("PageNumber(Location(80)) = %d, %v; want 80", page, ok)
	}
	if n, ok := doc.ChapterPages(inChapter); !ok || index >= n || doc.Chapters() <= inChapter {
		t.Errorf("chapter %d has %d pages (%v) of %d chapters; want page %d among them", inChapter, n, ok, doc.Chapters(), index)
	}
	if _, ok := doc.ChapterPages(doc.Chapters()); ok {
		t.Error("ChapterPages past the last chapter: got ok")
	}
	chars, err := doc.PageChars(80)
	if err != nil {
		t.Fatalf("PageChars(80) error: %v", err)
	}
	lines := 0
	for i, c := range chars {
		if c.Box.X1 < c.Box.X0 || c.Box.Y1 < c.Box.Y0 {
			t.Fatalf("PageChars(80)[%d] = %+v, want a box with positive size", i, c)
		}
		if i == 0 || c.Line != chars[i-1].Line {
			lines++
		}
	}
	if len(chars) < 200 || lines < 10 {
		t.Errorf("PageChars(80) = %d characters on %d lines, want a full page", len(chars), lines)
	}
	if _, err := doc.PageChars(pages + 5); err == nil {
		t.Error("PageChars past the last page: got nil error")
	}

	chapter, fraction, ok := doc.ChapterPosition(80)
	if !ok {
		t.Fatal("ChapterPosition(80) failed")
	}
	if _, err := doc.Layout(586, 940, 26); err != nil {
		t.Fatalf("Layout at 26pt error: %v", err)
	}
	page, ok := doc.PageAt(chapter, fraction)
	if !ok || page <= 80 {
		t.Errorf("PageAt(%d, %.3f) after enlarging the font = %d, %v; want a later page", chapter, fraction, page, ok)
	}

	// The position must also survive closing and reopening the book.
	reopened, err := Open(path, "body * { color: black !important; }")
	if err != nil {
		t.Fatalf("reopen error: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.Layout(586, 940, 26); err != nil {
		t.Fatalf("Layout of reopened book error: %v", err)
	}
	restored, ok := reopened.PageAt(chapter, fraction)
	if !ok {
		t.Fatalf("PageAt(%d, %.3f) failed", chapter, fraction)
	}
	if diff := restored - page; diff < -1 || diff > 1 {
		t.Errorf("PageAt(%d, %.3f) after reopening = page %d, want within one page of %d", chapter, fraction, restored, page)
	}
	t.Logf("page 80 at 21pt = chapter %d at %.0f%%; page %d at 26pt, %d after reopening", chapter, fraction*100, page, restored)
}

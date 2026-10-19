package mupdf

import (
	"bytes"
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

// pageText returns the text of page.
func pageText(t *testing.T, doc *Document, page int) string {
	t.Helper()
	chars, err := doc.PageChars(page)
	if err != nil {
		t.Fatalf("PageChars(%d) error: %v", page, err)
	}
	var b strings.Builder
	for _, c := range chars {
		b.WriteRune(c.Rune)
	}
	return b.String()
}

// TestLeadingBlankPages checks that a chapter whose first element breaks
// the page starts with its text instead of an empty page, and that every
// method numbers pages the same way.
func TestLeadingBlankPages(t *testing.T) {
	long := strings.Repeat("<p>Words that fill the page of the second chapter.</p>", 40)
	path := testbook.Book{Chapters: []string{
		`<h1 style="page-break-before:always">Start</h1><p>Text.</p>`,
		`<div style="page-break-before:always"><h1 id="two">Two</h1>` + long + `</div>`,
		`<p>Plain chapter.</p>`,
	}}.Write(t)
	doc, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer doc.Close()
	pages, err := doc.Layout(400, 600, 16)
	if err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if got := pageText(t, doc, 0); !strings.HasPrefix(got, "Start") {
		t.Errorf("page 0 = %q, want the first chapter's text", got)
	}
	total := 0
	for chapter := range doc.Chapters() {
		n, ok := doc.ChapterPages(chapter)
		if !ok || n == 0 {
			t.Fatalf("ChapterPages(%d) = %d, %v", chapter, n, ok)
		}
		first, ok := doc.PageNumber(chapter, 0)
		if !ok || first != total {
			t.Errorf("PageNumber(%d, 0) = %d, %v; want %d", chapter, first, ok, total)
		}
		if got := pageText(t, doc, first); got == "" {
			t.Errorf("first page of chapter %d (page %d) is empty", chapter, first)
		}
		total += n
	}
	if total != pages {
		t.Errorf("chapters hold %d pages, Layout returned %d", total, pages)
	}
	for page := range pages {
		chapter, index, ok := doc.Location(page)
		if back, _ := doc.PageNumber(chapter, index); !ok || back != page {
			t.Errorf("Location(%d) = %d, %d, %v; back to page %d", page, chapter, index, ok, back)
		}
		c, f, ok := doc.ChapterPosition(page)
		if back, _ := doc.PageAt(c, f); !ok || back != page {
			t.Errorf("ChapterPosition(%d) = %d, %v; back to page %d", page, c, f, back)
		}
	}
	if _, _, ok := doc.Location(pages); ok {
		t.Errorf("Location(%d) past the last page: got ok", pages)
	}
	second, _ := doc.PageNumber(1, 0)
	for _, uri := range []string{"OEBPS/c1.xhtml", "OEBPS/c1.xhtml#two"} {
		if page, ok := doc.Resolve(uri); !ok || page != second {
			t.Errorf("Resolve(%q) = %d, %v; want the second chapter's first page %d", uri, page, ok, second)
		}
	}
	if png, err := doc.RenderPNG(pages-1, 0.5); err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Errorf("RenderPNG of the last page = %d bytes, %v", len(png), err)
	}
	if _, err := doc.RenderPNG(pages, 0.5); err == nil {
		t.Error("RenderPNG past the last page: got nil error")
	}
}

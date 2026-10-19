package mupdf

import (
	"strings"
	"testing"
)

func TestOpenBuffer(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><head><title>In Memory</title></head><body><h1>One</h1><p>Ação em memória.</p></body></html>`)
	doc, err := OpenBuffer(html, "html", "p { color: black !important; }")
	if err != nil {
		t.Fatalf("OpenBuffer error: %v", err)
	}
	defer doc.Close()
	// The document has its own copy of the bytes.
	clear(html)
	pages, err := doc.Layout(400, 600, 12)
	if err != nil || pages != 1 {
		t.Fatalf("Layout = %d pages, %v; want 1", pages, err)
	}
	chars, err := doc.PageChars(0)
	if err != nil {
		t.Fatalf("PageChars error: %v", err)
	}
	var text strings.Builder
	for _, c := range chars {
		text.WriteRune(c.Rune)
	}
	if got := text.String(); !strings.Contains(got, "Ação em memória.") {
		t.Errorf("text = %q, want the paragraph", got)
	}
	if title := doc.Metadata(MetaTitle); title != "In Memory" {
		t.Errorf("Metadata(MetaTitle) = %q, want %q", title, "In Memory")
	}
	if outline, err := doc.Outline(); err != nil || len(outline) != 1 {
		t.Errorf("Outline = %v, %v; want the heading", outline, err)
	}

	if _, err := OpenBuffer(nil, "html", ""); err == nil {
		t.Error("OpenBuffer of nothing: got nil error")
	}
}

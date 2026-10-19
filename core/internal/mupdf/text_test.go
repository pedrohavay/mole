package mupdf

import (
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

// TestSetLineDistance checks that the lines of a paragraph set farther apart
// than MuPDF's 1.5 em stay one paragraph, while paragraphs that start with
// an indent or after a margin stay apart.
func TestSetLineDistance(t *testing.T) {
	long := strings.Repeat("words that wrap over several lines of the page ", 6)
	path := testbook.Chapters(t,
		`<p>`+long+`</p><p>`+long+`</p>`+
			`<p style="margin:0;text-indent:2em">`+long+`</p><p style="margin:0;text-indent:2em">`+long+`</p>`)
	doc, err := Open(path, "p { line-height: 1.7 !important; }")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(doc.Close)
	if _, err := doc.Layout(500, 1600, 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	paragraphs := func() (int, int) {
		t.Helper()
		chars, err := doc.PageChars(0)
		if err != nil || len(chars) == 0 {
			t.Fatalf("PageChars = %d characters, %v", len(chars), err)
		}
		last := chars[len(chars)-1]
		return last.Block + 1, last.Line + 1
	}
	if blocks, lines := paragraphs(); blocks != lines {
		t.Errorf("PageChars at MuPDF's distance: %d paragraphs of %d lines; want one per line", blocks, lines)
	}
	doc.SetLineDistance(1.8)
	if blocks, lines := paragraphs(); blocks != 4 {
		t.Errorf("PageChars at 1.8 em: %d paragraphs of %d lines; want 4", blocks, lines)
	}
}

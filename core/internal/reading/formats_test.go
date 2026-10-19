package reading

import (
	"fmt"
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

// formatBooks returns the same story as a MOBI, an FB2 and a plain text
// book, long enough for many pages: chapters of paragraphs that name their
// place, so every word is easy to find.
func formatBooks(t *testing.T) map[string]string {
	t.Helper()
	var html, text strings.Builder
	var sections []string
	for c := range 3 {
		fmt.Fprintf(&html, "<mbp:pagebreak/><h1>Chapter %d</h1>", c+1)
		fmt.Fprintf(&text, "CHAPTER %d\n\n", c+1)
		var section strings.Builder
		fmt.Fprintf(&section, "<title><p>Chapter %d</p></title>", c+1)
		for p := range 30 {
			para := fmt.Sprintf("Paragraph %d of chapter %d tells a plain story in words that wrap over a few lines of the page, ação.", p+1, c+1)
			fmt.Fprintf(&html, "<p>%s</p>", para)
			fmt.Fprintf(&section, "<p>%s</p>", para)
			// Plain text wraps its lines, as most text books do.
			words := strings.Fields(para)
			fmt.Fprintf(&text, "%s\n%s\n\n", strings.Join(words[:10], " "), strings.Join(words[10:], " "))
		}
		sections = append(sections, section.String())
	}
	return map[string]string{
		"MOBI": testbook.MOBI{Title: "Story", Author: "A", HTML: html.String()}.Write(t),
		"FB2":  testbook.FB2{Title: "Story", Sections: sections}.Write(t),
		"Text": testbook.File(t, "story.txt", []byte(text.String())),
	}
}

// TestFormats checks that books in the formats other than EPUB, which MuPDF
// lays out as a single chapter, read like an EPUB: text on the first page,
// a table of contents, anchors that survive a relayout, text ranges, going
// to an anchor and search.
func TestFormats(t *testing.T) {
	for name, path := range formatBooks(t) {
		t.Run(name, func(t *testing.T) {
			narrow := openSession(t, path, 300, 16)
			wide := openSession(t, path, 900, 22)
			if narrow.Pages() <= wide.Pages() || wide.Pages() < 4 {
				t.Fatalf("narrow layout has %d pages and wide %d, want more narrow pages and at least 4 wide", narrow.Pages(), wide.Pages())
			}
			if pt, err := narrow.pageText(0); err != nil || pt.count == 0 {
				t.Errorf("first page: %d characters, %v; want text", pt.count, err)
			}
			if got := len(narrow.Chapters()); got != 3 {
				t.Errorf("table of contents has %d entries, want 3", got)
			}

			r := find(t, narrow, 0, "Paragraph 20 of chapter 3")
			if got := find(t, wide, 0, "Paragraph 20 of chapter 3"); got != r {
				t.Errorf("the paragraph is at %+v in the wide layout and %+v in the narrow one", got, r)
			}
			if text, err := narrow.RangeText(r); err != nil || text != "Paragraph 20 of chapter 3" {
				t.Errorf("RangeText(%+v) = %q, %v", r, text, err)
			}
			if changed, err := wide.GoToAnchor(r.Start); err != nil || !changed {
				t.Fatalf("GoToAnchor(%+v) = %v, %v; want a page change", r.Start, changed, err)
			}
			visible, err := wide.VisibleRange()
			if err != nil || visible.Start.Compare(r.Start) > 0 || r.Start.Compare(visible.End) >= 0 {
				t.Errorf("after GoToAnchor(%+v) the visible range is %+v, %v", r.Start, visible, err)
			}
			if rects, err := wide.RangeRects(r); err != nil || len(rects) == 0 {
				t.Errorf("RangeRects(%+v) = %v, %v; want the rectangles of the visible range", r, rects, err)
			}
			// The place holds across a relayout.
			layout(t, wide, 600, 18)
			if visible, _ := wide.VisibleRange(); visible.Start.Compare(r.Start) > 0 || r.Start.Compare(visible.End) >= 0 {
				t.Errorf("after a relayout the visible range is %+v, want it to hold %+v", visible, r.Start)
			}

			res := search(t, narrow, "chapter 3 tells")
			if len(res.Hits) != 30 {
				t.Errorf("Search found %d hits, want 30", len(res.Hits))
			}
			if res := search(t, narrow, "acao"); len(res.Hits) != 90 {
				t.Errorf("Search(acao) found %d hits, want 90", len(res.Hits))
			}
		})
	}
}

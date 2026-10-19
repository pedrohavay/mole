package reading

import (
	"os"
	"strings"
	"testing"
	"time"

	"mole/core/internal/testbook"
)

// longChapters hold a paragraph that runs over several pages of a 260 by
// 700 point layout.
var longChapters = []string{
	`<p>Opening.</p><p>` + strings.Repeat("lorem ipsum dolor sit amet ", 90) + `end.</p><p>After.</p>`,
	`<p>Next chapter.</p>`,
}

// toEnd is past the end of any test book.
var toEnd = Anchor{Chapter: 1 << 20}

// joined returns the text of paragraphs, with the parts of a paragraph
// joined by a space and paragraphs on lines of their own.
func joined(paragraphs []Paragraph) []string {
	var out []string
	for _, p := range paragraphs {
		if p.Continued && len(out) > 0 {
			out[len(out)-1] += " " + p.Text
			continue
		}
		out = append(out, p.Text)
	}
	return out
}

// checkParts checks that each paragraph's text is the text of its range and
// that its range counts the characters of its text.
func checkParts(t *testing.T, s *Session, paragraphs []Paragraph) {
	t.Helper()
	for _, p := range paragraphs {
		text, err := s.RangeText(p.Range)
		if err != nil || text != p.Text {
			t.Errorf("RangeText(%+v) = %q, %v; want the paragraph's text %q", p.Range, text, err, p.Text)
		}
		n := 0
		for _, r := range p.Text {
			if counted(r) {
				n++
			}
		}
		if p.Range.Start.Chapter != p.Range.End.Chapter || p.Range.End.Offset-p.Range.Start.Offset != n {
			t.Errorf("paragraph %q has range %+v, want %d counted characters in one chapter", p.Text, p.Range, n)
		}
	}
}

func TestParagraphs(t *testing.T) {
	path := testbook.Chapters(t, sampleChapters...)
	s := openSession(t, path, 260, 16)
	tests := []struct {
		name  string
		give  Range
		limit int
		want  []string
	}{
		{
			name: "Book",
			give: Range{End: toEnd},
			want: []string{
				"First paragraph with several words that wrap around the narrow line of a page.",
				"Ação é coração; o fim chegou.",
				"Internationalization of wellknown supercalifragilistic words.",
				"Closing words.",
				"Two",
				"Second chapter text.",
			},
		},
		{
			name: "ClippedAtBothEnds",
			give: Range{Start: find(t, s, 0, "several").Start, End: find(t, s, 0, "Ação").End},
			want: []string{"several words that wrap around the narrow line of a page.", "Ação"},
		},
		{
			name:  "Limit",
			give:  Range{Start: find(t, s, 0, "Closing").Start, End: toEnd},
			limit: 5,
			want:  []string{"Closing words."},
		},
		{
			name:  "LimitAcrossChapters",
			give:  Range{Start: find(t, s, 0, "Closing").Start, End: toEnd},
			limit: 14,
			want:  []string{"Closing words.", "Two"},
		},
		{
			name: "Empty",
			give: Range{Start: find(t, s, 0, "Closing").Start, End: find(t, s, 0, "Closing").Start},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Paragraphs(tt.give, tt.limit)
			if err != nil {
				t.Fatalf("Paragraphs(%+v, %d) error: %v", tt.give, tt.limit, err)
			}
			checkParts(t, s, got)
			texts := joined(got)
			// Soft hyphens show where they break a line, and the real
			// hyphen of well-known is not counted either way.
			for i := range texts {
				texts[i] = strings.ReplaceAll(texts[i], "-", "")
			}
			if strings.Join(texts, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("Paragraphs(%+v, %d) = %q, want %q", tt.give, tt.limit, texts, tt.want)
			}
		})
	}
}

func TestParagraphsOverPageBreaks(t *testing.T) {
	path := testbook.Chapters(t, longChapters...)
	s := openSession(t, path, 260, 16)
	all, err := s.Paragraphs(Range{End: toEnd}, 0)
	if err != nil {
		t.Fatalf("Paragraphs error: %v", err)
	}
	checkParts(t, s, all)
	long := strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet ", 90)) + " end."
	want := []string{"Opening.", long, "After.", "Next chapter."}
	if got := joined(all); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Paragraphs of the book = %q, want %q", got, want)
	}
	parts := 0
	for _, p := range all {
		if p.Continued {
			parts++
		}
		first, err := s.AnchorPage(p.Range.Start)
		if err != nil {
			t.Fatalf("AnchorPage(%+v) error: %v", p.Range.Start, err)
		}
		last, err := s.AnchorPage(Anchor{Chapter: p.Range.End.Chapter, Offset: p.Range.End.Offset - 1})
		if err != nil || last != first {
			t.Errorf("paragraph part %+v runs from page %d to page %d, %v; want one page", p.Range, first, last, err)
		}
	}
	if parts < 2 {
		t.Errorf("the long paragraph came in %d continued parts, want it split over at least three pages", parts)
	}

	// A limit stops at the end of a paragraph, not at a page break.
	got, err := s.Paragraphs(Range{End: toEnd}, len("Opening.")+5)
	if err != nil {
		t.Fatalf("Paragraphs with a limit error: %v", err)
	}
	if texts := joined(got); len(texts) != 2 || texts[1] != long {
		t.Errorf("Paragraphs with a limit inside the long paragraph = %q, want it whole after Opening.", texts)
	}
}

// TestParagraphsInBook runs against a real book. Set MOLE_TEST_EPUB to an
// EPUB path.
func TestParagraphsInBook(t *testing.T) {
	path := os.Getenv("MOLE_TEST_EPUB")
	if path == "" {
		t.Skip("MOLE_TEST_EPUB not set")
	}
	s, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Layout(NewGeometry(1384, 874, 52, 2, 16, SpreadAuto, MarginsNormal), 16); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if _, err := s.GoTo(s.Pages() / 2); err != nil {
		t.Fatalf("GoTo error: %v", err)
	}
	visible, err := s.VisibleRange()
	if err != nil {
		t.Fatalf("VisibleRange error: %v", err)
	}
	start := time.Now()
	paragraphs, err := s.Paragraphs(visible, 0)
	t.Logf("Paragraphs of the visible pages: %d in %v", len(paragraphs), time.Since(start))
	if err != nil || len(paragraphs) == 0 {
		t.Fatalf("Paragraphs(%+v) = %d paragraphs, %v; want some", visible, len(paragraphs), err)
	}
	checkParts(t, s, paragraphs)
	if visible.Start.Chapter == visible.End.Chapter {
		n := 0
		for _, p := range paragraphs {
			n += p.Range.End.Offset - p.Range.Start.Offset
		}
		if n != visible.End.Offset-visible.Start.Offset {
			t.Errorf("paragraphs of the visible pages count %d characters, want %d", n, visible.End.Offset-visible.Start.Offset)
		}
	}
	for _, p := range paragraphs {
		rects, err := s.RangeRects(p.Range)
		if err != nil || len(rects) == 0 {
			t.Errorf("RangeRects of visible paragraph %+v = %d rects, %v; want some", p.Range, len(rects), err)
		}
	}

	// Reading aloud asks for a couple thousand characters at a time.
	s.forgetText()
	start = time.Now()
	ahead, err := s.Paragraphs(Range{Start: visible.Start, End: toEnd}, 2000)
	t.Logf("Paragraphs of 2000 characters ahead: %d in %v", len(ahead), time.Since(start))
	if err != nil || len(ahead) == 0 {
		t.Fatalf("Paragraphs ahead = %d paragraphs, %v; want some", len(ahead), err)
	}
	checkParts(t, s, ahead)
}

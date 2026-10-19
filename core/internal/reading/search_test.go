package reading

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"mole/core/internal/mupdf"
	"mole/core/internal/testbook"
)

func TestFold(t *testing.T) {
	tests := []struct{ give, want string }{
		{give: "Ação", want: "acao"},
		{give: "CORAÇÃO", want: "coracao"},
		{give: "\uFB01m", want: "fim"},
		{give: "Straße", want: "strasse"},
		{give: "well-known", want: "wellknown"},
		{give: "soft\u00ADhyphen", want: "softhyphen"},
		{give: "  Two \n\t words\u00A0here ", want: "two words here"},
		{give: "Ünïcödé", want: "unicode"},
	}
	for _, tt := range tests {
		if got := string(foldQuery(tt.give)); got != tt.want {
			t.Errorf("foldQuery(%q) = %q, want %q", tt.give, got, tt.want)
		}
	}
}

func TestExcerpt(t *testing.T) {
	text := []rune(strings.Repeat("word ", 20) + "ação\nfinal " + strings.Repeat("tail ", 20))
	from := strings.Index(string(text), "ação")
	from = len([]rune(string(text)[:from]))
	got, start, length := excerpt(text, from, from+4)
	if got[start:start+length] != "ação" {
		t.Errorf("excerpt match = %q, want %q", got[start:start+length], "ação")
	}
	if strings.Contains(got, "\n") || strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") {
		t.Errorf("excerpt = %q, want one trimmed line", got)
	}
	if n := len([]rune(got)); n < 60 || n > excerptLength {
		t.Errorf("excerpt has %d characters, want about %d", n, excerptLength)
	}
	for _, w := range strings.Fields(got) {
		if w != "word" && w != "ação" && w != "final" && w != "tail" {
			t.Errorf("excerpt %q cuts a word: %q", got, w)
		}
	}
}

// search runs a search to the end and returns its results.
func search(t *testing.T, s *Session, query string) Results {
	t.Helper()
	sr, err := s.Search(query)
	if err != nil {
		t.Fatalf("Search(%q) error: %v", query, err)
	}
	<-sr.exited
	res, err := sr.Results()
	if err != nil || !res.Done {
		t.Fatalf("Search(%q) results: done %v, error %v", query, res.Done, err)
	}
	return res
}

func TestSearch(t *testing.T) {
	words := make([]string, 300)
	for i := range words {
		words[i] = fmt.Sprintf("w%03d", i)
	}
	path := testbook.Chapters(t, append(slices.Clone(sampleChapters), `<p>`+strings.Join(words, " ")+`</p>`)...)
	s := openSession(t, path, 260, 16)
	tests := []struct {
		query string
		want  []string // range text of each hit
	}{
		{query: "acao", want: []string{"Ação", "ação"}},
		{query: "CORAÇÃO;", want: []string{"coração;"}},
		{query: "o fim", want: []string{"o fim"}},
		{query: "fim chegou", want: []string{"fim chegou"}}, // after a non-breaking space
		{query: "internationalization", want: []string{"Internationalization"}},
		{query: "supercalifragilistic", want: []string{"supercalifragilistic"}},
		{query: "well-known", want: []string{"well-known"}},
		{query: "wellknown", want: []string{"well-known"}},
		{query: "well known"},
		{query: "  paragraph \n with ", want: []string{"paragraph with"}},
		{query: "paragraphwith"},
		{query: "words", want: []string{"words", "words", "words"}},
		{query: "second chapter", want: []string{"Second chapter"}},
		{query: "nowhere"},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(strings.TrimSpace(tt.query), " ", "_"), func(t *testing.T) {
			res := search(t, s, tt.query)
			var got []string
			for _, h := range res.Hits {
				text, err := s.RangeText(h.Range)
				if err != nil {
					t.Fatalf("RangeText(%+v) error: %v", h.Range, err)
				}
				got = append(got, text)
				if m := h.Excerpt[h.MatchStart : h.MatchStart+h.MatchLength]; !strings.EqualFold(foldString(m), foldString(text)) {
					t.Errorf("hit %+v: excerpt %q marks %q, want %q", h.Range, h.Excerpt, m, text)
				}
			}
			// A soft hyphen shows only where it breaks a line.
			unhyphenate := func(s string) string { return strings.ReplaceAll(s, "-", "") }
			if !slices.EqualFunc(got, tt.want, func(a, b string) bool { return unhyphenate(a) == unhyphenate(b) }) {
				t.Errorf("Search(%q) found %q, want %q", tt.query, got, tt.want)
			}
			if res.Truncated {
				t.Errorf("Search(%q) truncated", tt.query)
			}
		})
	}
	if _, err := s.Search(" \u00AD- "); err == nil {
		t.Error("Search of whitespace and hyphens: got nil error")
	}

	// A match may cross a line break of the search layout.
	doc, err := mupdf.Open(path, Typography{}.css())
	if err != nil {
		t.Fatalf("mupdf.Open error: %v", err)
	}
	defer doc.Close()
	if _, err := doc.Layout(searchWidth, searchHeight, searchFontSize); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	page, _ := doc.PageNumber(2, 0)
	chars, err := doc.PageChars(page)
	if err != nil {
		t.Fatalf("PageChars error: %v", err)
	}
	pt := newPageText(2, 0, chars)
	if len(pt.lines) < 2 {
		t.Fatalf("the long paragraph has %d lines, want several", len(pt.lines))
	}
	lineText := func(l textLine) []string {
		var b strings.Builder
		for _, c := range chars[l.first:l.end] {
			b.WriteRune(c.Rune)
		}
		return strings.Fields(b.String())
	}
	first, second := lineText(pt.lines[0]), lineText(pt.lines[1])
	query := first[len(first)-1] + " " + second[0]
	if res := search(t, s, query); len(res.Hits) != 1 {
		t.Errorf("Search(%q) across a line break found %d hits, want 1", query, len(res.Hits))
	}
}

func foldString(s string) string {
	return string(foldQuery(s))
}

func TestSearchStopsAtMaxHits(t *testing.T) {
	path := testbook.Chapters(t, `<p>`+strings.Repeat("zz ", MaxHits+100)+`</p>`, `<p>zz</p>`)
	s := openSession(t, path, 400, 16)
	res := search(t, s, "zz")
	if len(res.Hits) != MaxHits || !res.Truncated {
		t.Errorf("Search(zz) = %d hits, truncated %v; want %d, truncated", len(res.Hits), res.Truncated, MaxHits)
	}
	for i := 1; i < len(res.Hits); i++ {
		if res.Hits[i-1].Range.End.Compare(res.Hits[i].Range.Start) > 0 {
			t.Fatalf("hits %d and %d overlap or are out of order: %+v, %+v", i-1, i, res.Hits[i-1].Range, res.Hits[i].Range)
		}
	}
}

// TestSearchInBook runs against a real book. Set MOLE_TEST_EPUB to an EPUB path.
func TestSearchInBook(t *testing.T) {
	path := os.Getenv("MOLE_TEST_EPUB")
	if path == "" {
		t.Skip("MOLE_TEST_EPUB not set")
	}
	s, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Layout(NewGeometry(1384, 874, 52, 2, 21, SpreadAuto, MarginsNormal), 21); err != nil {
		t.Fatalf("Layout error: %v", err)
	}

	// Cancelling returns at once, even while the text is being extracted.
	sr, err := s.Search("pipeline")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	start := time.Now()
	sr.Close()
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Close took %v, want it to return at once", d)
	}
	if res, err := sr.Results(); err != nil || !res.Done {
		t.Errorf("Results after Close: done %v, error %v; want done", res.Done, err)
	}

	// The first full search extracts the text; hits come in while it runs,
	// and the page keeps rendering and answering hit tests meanwhile.
	const query = "continuous   delivery"
	start = time.Now()
	sr, err = s.Search(query)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	var prev []Hit
	partial := false
	for {
		res, err := sr.Results()
		if err != nil {
			t.Fatalf("Results error: %v", err)
		}
		if len(res.Hits) < len(prev) || !slices.Equal(res.Hits[:len(prev)], prev) {
			t.Fatalf("Results went from %d to %d hits without keeping the earlier ones", len(prev), len(res.Hits))
		}
		prev = res.Hits
		if res.Done {
			break
		}
		partial = partial || len(res.Hits) > 0
		if _, _, err := s.Render(); err != nil {
			t.Fatalf("Render during search error: %v", err)
		}
		if _, _, err := s.HitTest(400, 400); err != nil {
			t.Fatalf("HitTest during search error: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	first := time.Since(start)
	if !partial {
		t.Error("Results never returned hits before the search was done")
	}
	if len(prev) < 20 {
		t.Fatalf("Search(%q) found %d hits, want many", query, len(prev))
	}

	start = time.Now()
	again := search(t, s, query)
	t.Logf("search for %q: %d hits, %v with text extraction, %v after", query, len(prev), first, time.Since(start))
	if len(again.Hits) != len(prev) {
		t.Errorf("second search found %d hits, the first %d", len(again.Hits), len(prev))
	}

	// Hits found in the search layout name the same words in the reader's.
	for _, h := range prev[:10] {
		text, err := s.RangeText(h.Range)
		if err != nil {
			t.Fatalf("RangeText(%+v) error: %v", h.Range, err)
		}
		if foldString(text) != "continuous delivery" {
			t.Errorf("hit %+v reads %q, want %q", h.Range, text, "Continuous delivery")
		}
		pt, _ := charAt(t, s, h.Range.Start)
		if !strings.Contains(strings.ToLower(string(chapterRunes(pt))), "continuous") {
			t.Errorf("AnchorPage(%+v) is a page without %q", h.Range.Start, "continuous")
		}
		if m := h.Excerpt[h.MatchStart : h.MatchStart+h.MatchLength]; foldString(m) != "continuous delivery" || strings.Contains(h.Excerpt, "\n") {
			t.Errorf("excerpt %q marks %q", h.Excerpt, m)
		}
	}

	// Closing the session ends a search that waits for text.
	other, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	sr, err = other.Search("delivery")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	other.Close()
	<-other.text.exited
	<-sr.exited
	if res, err := sr.Results(); err != nil || !res.Done {
		t.Errorf("Results after closing the session: done %v, error %v; want done", res.Done, err)
	}
}

func chapterRunes(t *pageText) []rune {
	out := make([]rune, len(t.chars))
	for i, c := range t.chars {
		out[i] = c.Rune
	}
	return out
}

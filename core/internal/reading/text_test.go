package reading

import (
	"os"
	"strings"
	"testing"
	"time"

	"mole/core/internal/mupdf"
	"mole/core/internal/testbook"
)

// sampleChapters exercise the counting rules: accents, a ligature, a
// non-breaking space, soft hyphens, real hyphens and paragraphs without
// margins.
var sampleChapters = []string{
	`<p>First paragraph with several words that wrap around the narrow line of a page.</p>` +
		`<p>A&#xE7;&#xE3;o &#xE9; cora&#xE7;&#xE3;o; o &#xFB01;m&#xA0;chegou.</p>` +
		`<p style="margin:0;text-indent:2em">Inter&#xAD;na&#xAD;tio&#xAD;nal&#xAD;iza&#xAD;tion of well-known super&#xAD;cali&#xAD;fragi&#xAD;listic words.</p>` +
		`<p style="margin:0;text-indent:2em">Closing words.</p>`,
	`<h1>Two</h1><p>Second chapter text.</p>`,
}

func openSession(t *testing.T, path string, width, fontSize float64) *Session {
	t.Helper()
	s, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	layout(t, s, width, fontSize)
	return s
}

func layout(t *testing.T, s *Session, width, fontSize float64) {
	t.Helper()
	if err := s.Layout(NewGeometry(width, 700, 0, 2, fontSize, SpreadSingle, MarginsNormal), fontSize); err != nil {
		t.Fatalf("Layout(%v, %v) error: %v", width, fontSize, err)
	}
}

// chapterText returns the counted characters of chapter in the current
// layout; the offset of a character is its index.
func chapterText(t *testing.T, s *Session, chapter int) []rune {
	t.Helper()
	var out []rune
	for page := range s.Pages() {
		pt, err := s.pageText(page)
		if err != nil {
			t.Fatalf("pageText(%d) error: %v", page, err)
		}
		if pt.chapter != chapter {
			continue
		}
		if pt.start != len(out) {
			t.Fatalf("page %d of chapter %d starts at offset %d, want %d", page, chapter, pt.start, len(out))
		}
		for _, c := range pt.chars {
			if counted(c.Rune) {
				out = append(out, c.Rune)
			}
		}
	}
	return out
}

// find returns the range of the first occurrence of word in chapter,
// comparing counted characters only.
func find(t *testing.T, s *Session, chapter int, word string) Range {
	t.Helper()
	var want []rune
	for _, r := range word {
		if counted(r) {
			want = append(want, r)
		}
	}
	i := strings.Index(string(chapterText(t, s, chapter)), string(want))
	if i < 0 {
		t.Fatalf("%q not found in chapter %d", word, chapter)
	}
	offset := len([]rune(string(chapterText(t, s, chapter))[:i]))
	return Range{Start: Anchor{Chapter: chapter, Offset: offset}, End: Anchor{Chapter: chapter, Offset: offset + len(want)}}
}

// charAt returns the page that holds a and the index of its character there.
func charAt(t *testing.T, s *Session, a Anchor) (*pageText, int) {
	t.Helper()
	page, err := s.AnchorPage(a)
	if err != nil {
		t.Fatalf("AnchorPage(%+v) error: %v", a, err)
	}
	pt, err := s.pageText(page)
	if err != nil {
		t.Fatalf("pageText(%d) error: %v", page, err)
	}
	for i, c := range pt.chars {
		if counted(c.Rune) && pt.next[i] == a.Offset {
			return pt, i
		}
	}
	t.Fatalf("AnchorPage(%+v) = page %d, which does not hold the anchor (offsets %d..%d)", a, page, pt.start, pt.start+pt.count)
	return nil, 0
}

func TestCounted(t *testing.T) {
	for _, tt := range []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'ç', true}, {'.', true}, {'\u2014', true},
		{' ', false}, {'\u00A0', false}, {'\n', false},
		{'-', false}, {'\u00AD', false}, {'\u2010', false}, {'\u200B', false},
	} {
		if got := counted(tt.r); got != tt.want {
			t.Errorf("counted(%U) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestAnchorsIgnoreLayout(t *testing.T) {
	path := testbook.Chapters(t, sampleChapters...)
	narrow := openSession(t, path, 260, 16)
	wide := openSession(t, path, 900, 22)
	if narrow.Pages() <= wide.Pages() {
		t.Fatalf("narrow layout has %d pages and wide %d, want more narrow pages", narrow.Pages(), wide.Pages())
	}
	for chapter := range 2 {
		if a, b := string(chapterText(t, narrow, chapter)), string(chapterText(t, wide, chapter)); a != b {
			t.Errorf("chapter %d counted text differs between layouts:\nnarrow %q\nwide   %q", chapter, a, b)
		}
	}
	// The narrow layout breaks the long words at their soft hyphens.
	for _, word := range []string{"Internationalization", "supercalifragilistic", "Closing", "chapter"} {
		chapter := 0
		if word == "chapter" {
			chapter = 1
		}
		r := find(t, narrow, chapter, word)
		if got := find(t, wide, chapter, word); got != r {
			t.Errorf("%q is at %+v in the wide layout and %+v in the narrow one", word, got, r)
		}
		for _, s := range []*Session{narrow, wide} {
			if pt, i := charAt(t, s, r.Start); pt.chars[i].Rune != []rune(word)[0] {
				t.Errorf("character at %+v = %q, want %q", r.Start, pt.chars[i].Rune, []rune(word)[0])
			}
		}
	}
}

func TestRangeText(t *testing.T) {
	path := testbook.Chapters(t, sampleChapters...)
	s := openSession(t, path, 260, 16)
	tests := []struct {
		name, from, to string
		chapter        int
		want           string
	}{
		{name: "AcrossLines", from: "First", to: "page.", want: "First paragraph with several words that wrap around the narrow line of a page."},
		{name: "AccentsAndLigature", from: "Ação", to: "chegou.", want: "Ação é coração; o fim chegou."},
		{name: "AcrossParagraphs", from: "words.", to: "Closing", want: "words.\nClosing"},
		{name: "HyphenatedWord", from: "well-known", to: "well-known", want: "well-known"},
		{name: "OtherChapter", from: "Second", to: "text.", chapter: 1, want: "Second chapter text."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Range{Start: find(t, s, tt.chapter, tt.from).Start, End: find(t, s, tt.chapter, tt.to).End}
			got, err := s.RangeText(r)
			if err != nil {
				t.Fatalf("RangeText(%+v) error: %v", r, err)
			}
			if got != tt.want {
				t.Errorf("RangeText(%q..%q) = %q, want %q", tt.from, tt.to, got, tt.want)
			}
		})
	}
	// Soft hyphens show only where they break a line, so the text of a
	// broken word may keep that hyphen; the letters are always there.
	r := find(t, s, 0, "Internationalization")
	got, err := s.RangeText(r)
	if err != nil {
		t.Fatalf("RangeText error: %v", err)
	}
	if strings.ReplaceAll(got, "-", "") != "Internationalization" {
		t.Errorf("RangeText(Internationalization) = %q, want the word with at most line-break hyphens", got)
	}
	if got, err := s.RangeText(Range{Start: r.End, End: r.Start}); err != nil || got != "" {
		t.Errorf("RangeText of a reversed range = %q, %v; want empty", got, err)
	}
	r = Range{Start: find(t, s, 0, "Closing").Start, End: find(t, s, 1, "Second").End}
	if got, err := s.RangeText(r); err != nil || got != "Closing words.\nTwo\nSecond" {
		t.Errorf("RangeText across chapters = %q, %v; want %q", got, err, "Closing words.\nTwo\nSecond")
	}
}

func TestHitTestAndRects(t *testing.T) {
	path := testbook.Chapters(t, sampleChapters...)
	s := openSession(t, path, 260, 16)
	pages, err := s.visiblePages()
	if err != nil {
		t.Fatalf("visiblePages error: %v", err)
	}
	p := pages[0]
	r := find(t, s, 0, "several")
	pt, i := charAt(t, s, r.Start)
	if pt != p.text {
		t.Fatalf("%q is not on the first page", "several")
	}
	box := pt.chars[i+2].Box // the second 'v'... any letter inside the word
	x, y := p.left+(box.X0+box.X1)/2, p.top+(box.Y0+box.Y1)/2
	got, ok, err := s.HitTest(x, y)
	if err != nil || !ok || got != (Anchor{Chapter: 0, Offset: r.Start.Offset + 2}) {
		t.Errorf("HitTest(%.1f, %.1f) = %+v, %v, %v; want offset %d", x, y, got, ok, err, r.Start.Offset+2)
	}
	line := pt.lines[0]
	got, ok, err = s.HitTest(p.left+s.geo.ColumnWidth-1, p.top+(line.box.Y0+line.box.Y1)/2)
	if last := pt.anchor(line.end - 1); err != nil || !ok || got != last {
		t.Errorf("HitTest right of the first line = %+v, %v, %v; want its last character %+v", got, ok, err, last)
	}
	if _, ok, err := s.HitTest(p.left+5, s.geo.Height-1); err != nil || ok {
		t.Errorf("HitTest below the pages = %v, %v; want no text", ok, err)
	}

	rects, err := s.RangeRects(r)
	if err != nil {
		t.Fatalf("RangeRects error: %v", err)
	}
	if len(rects) != 1 || x < rects[0].X || x > rects[0].X+rects[0].Width || y < rects[0].Y || y > rects[0].Y+rects[0].Height {
		t.Errorf("RangeRects(several) = %+v, want one rect around (%.1f, %.1f)", rects, x, y)
	}
	whole := Range{Start: find(t, s, 0, "First").Start, End: find(t, s, 0, "page.").End}
	lines := 0
	for _, l := range pt.lines {
		if pt.inside(l.first, whole) {
			lines++
		}
	}
	if rects, err := s.RangeRects(whole); err != nil || len(rects) != lines || lines < 3 {
		t.Errorf("RangeRects(first paragraph) = %d rects, %v; want one per line, %d", len(rects), err, lines)
	}
	if rects, err := s.RangeRects(find(t, s, 1, "Second")); err != nil || len(rects) != 0 {
		t.Errorf("RangeRects off screen = %+v, %v; want none", rects, err)
	}
}

func TestVisibleRangeAndGoToAnchor(t *testing.T) {
	path := testbook.Chapters(t, sampleChapters...)
	s := openSession(t, path, 260, 16)
	visible, err := s.VisibleRange()
	if err != nil {
		t.Fatalf("VisibleRange error: %v", err)
	}
	if visible.Start != (Anchor{}) || visible.End.Offset == 0 {
		t.Errorf("VisibleRange on the first page = %+v, want from the start of chapter 0", visible)
	}
	target := find(t, s, 1, "chapter").Start
	changed, err := s.GoToAnchor(target)
	if err != nil || !changed {
		t.Fatalf("GoToAnchor(%+v) = %v, %v; want a page change", target, changed, err)
	}
	visible, _ = s.VisibleRange()
	if visible.Start.Compare(target) > 0 || target.Compare(visible.End) >= 0 {
		t.Errorf("after GoToAnchor(%+v) the visible range is %+v", target, visible)
	}
	if changed, err := s.GoToAnchor(target); err != nil || changed {
		t.Errorf("GoToAnchor to a visible anchor = %v, %v; want no change", changed, err)
	}
	if _, err := s.AnchorPage(Anchor{Chapter: 9}); err == nil {
		t.Error("AnchorPage of a missing chapter: got nil error")
	}
	if page, err := s.AnchorPage(Anchor{Chapter: 0, Offset: 1 << 30}); err != nil || page != s.Page()-1 {
		t.Errorf("AnchorPage past the end of chapter 0 = %d, %v; want its last page %d", page, err, s.Page()-1)
	}
}

// TestAnchorsInBook runs against a real book. Set MOLE_TEST_EPUB to an EPUB path.
func TestAnchorsInBook(t *testing.T) {
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

	// Pick a word in the middle of the visible page and hit it.
	pages, err := s.visiblePages()
	if err != nil {
		t.Fatalf("visiblePages error: %v", err)
	}
	p := pages[0]
	i := len(p.text.chars) / 2
	for !counted(p.text.chars[i].Rune) {
		i++
	}
	word := wordAt(p.text, i)
	box := p.text.chars[i].Box
	x, y := p.left+(box.X0+box.X1)/2, p.top+(box.Y0+box.Y1)/2
	s.forgetText() // time a hit test right after a layout
	start := time.Now()
	a, ok, err := s.HitTest(x, y)
	t.Logf("first HitTest on page %d after a layout: %v", s.Page(), time.Since(start))
	if err != nil || !ok || a != p.text.anchor(i) {
		t.Fatalf("HitTest(%.1f, %.1f) = %+v, %v, %v; want %+v", x, y, a, ok, err, p.text.anchor(i))
	}
	start = time.Now()
	const runs = 100
	for range runs {
		s.HitTest(x, y)
	}
	t.Logf("HitTest: %v", time.Since(start)/runs)
	selection := Range{Start: p.text.anchor(i - 40), End: a}
	start = time.Now()
	for range runs {
		s.RangeRects(selection)
	}
	t.Logf("RangeRects of %d characters: %v", selection.End.Offset-selection.Start.Offset, time.Since(start)/runs)

	// A larger font and a narrower window put the word on another page;
	// its anchor still finds it.
	if err := s.Layout(NewGeometry(900, 700, 52, 2, 26, SpreadAuto, MarginsNormal), 26); err != nil {
		t.Fatalf("Layout at 26pt error: %v", err)
	}
	start = time.Now()
	page, err := s.AnchorPage(a)
	t.Logf("AnchorPage after relayout: %v", time.Since(start))
	if err != nil {
		t.Fatalf("AnchorPage(%+v) error: %v", a, err)
	}
	pt, j := charAt(t, s, a)
	if got := wordAt(pt, j); got != word {
		t.Errorf("word at %+v after relayout = %q on page %d, want %q", a, got, page, word)
	}
	s.GoTo(0)
	if changed, err := s.GoToAnchor(a); err != nil || !changed || s.Page() != s.spread(page) {
		t.Errorf("GoToAnchor(%+v) = %v, %v on page %d; want a change to the screen of page %d", a, changed, err, s.Page(), page)
	}
	if visible, err := s.VisibleRange(); err != nil || visible.Start.Compare(a) > 0 || a.Compare(visible.End) >= 0 {
		t.Errorf("VisibleRange after GoToAnchor(%+v) = %+v, %v", a, visible, err)
	}

	// Every chapter counts the same characters in any layout.
	doc, err := mupdf.Open(path, Style{}.css())
	if err != nil {
		t.Fatalf("mupdf.Open error: %v", err)
	}
	defer doc.Close()
	if _, err := doc.Layout(searchWidth, searchHeight, searchFontSize); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	for chapter := range doc.Chapters() {
		pages, _ := doc.ChapterPages(chapter)
		want := 0
		for index := range pages {
			page, _ := doc.PageNumber(chapter, index)
			chars, err := doc.PageChars(page)
			if err != nil {
				t.Fatalf("PageChars(%d) error: %v", page, err)
			}
			want += countChars(chars)
		}
		last, _ := s.doc.ChapterPages(chapter)
		starts, err := s.pageStarts(chapter, last)
		if err != nil {
			t.Fatalf("pageStarts(%d) error: %v", chapter, err)
		}
		if got := starts[len(starts)-1]; got != want {
			t.Errorf("chapter %d has %d characters at 26pt and %d in the search layout", chapter, got, want)
		}
	}

	// Text off screen.
	far := Anchor{Chapter: a.Chapter + 1, Offset: 10}
	text, err := s.RangeText(Range{Start: far, End: Anchor{Chapter: far.Chapter, Offset: 60}})
	if err != nil || len([]rune(strings.ReplaceAll(strings.ReplaceAll(text, " ", ""), "\n", ""))) < 50 {
		t.Errorf("RangeText off screen = %q, %v; want 50 counted characters", text, err)
	}
}

// wordAt returns the counted characters around character i of a page,
// between whitespace.
func wordAt(t *pageText, i int) string {
	from, to := i, i
	for from > 0 && t.chars[from-1].Line == t.chars[i].Line && !strings.ContainsRune(" \u00A0", t.chars[from-1].Rune) {
		from--
	}
	for to < len(t.chars) && t.chars[to].Line == t.chars[i].Line && !strings.ContainsRune(" \u00A0", t.chars[to].Rune) {
		to++
	}
	var b strings.Builder
	for _, c := range t.chars[from:to] {
		if counted(c.Rune) {
			b.WriteRune(c.Rune)
		}
	}
	return b.String()
}

package reading

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"mole/core/internal/testbook"
)

func TestTypographyCSS(t *testing.T) {
	tests := []struct {
		name     string
		give     Typography
		want     []string // in this order
		wantNone string
	}{
		{
			name:     "Zero",
			give:     Typography{},
			want:     []string{"monospace !important", "line-height: 1.45 !important; text-align: left !important"},
			wantNone: "font-family: \"",
		},
		{
			name: "SerifFont",
			give: Typography{Font: "Georgia", LineSpacing: LineSpacingRelaxed},
			want: []string{`body, body * { font-family: "Georgia", serif !important; }`, "pre, pre *", "line-height: 1.70"},
		},
		{
			name: "SansFontJustified",
			give: Typography{Font: "Helvetica Neue", LineSpacing: LineSpacingCompact, Justify: true},
			want: []string{`font-family: "Helvetica Neue", sans-serif !important`, "line-height: 1.25 !important; text-align: justify !important"},
		},
		{
			name:     "FontNotOffered",
			give:     Typography{Font: `Comic Sans", serif; } body { color: red`},
			wantNone: "Comic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			css := tt.give.css()
			at := 0
			for _, w := range tt.want {
				i := strings.Index(css[at:], w)
				if i < 0 {
					t.Fatalf("%+v.css() = %q, want %q after offset %d", tt.give, css, w, at)
				}
				at += i + len(w)
			}
			if tt.wantNone != "" && strings.Contains(css, tt.wantNone) {
				t.Errorf("%+v.css() = %q, want no %q", tt.give, css, tt.wantNone)
			}
		})
	}
}

func TestFonts(t *testing.T) {
	fonts := Fonts()
	// These come with every macOS 14 install.
	for _, want := range []string{"Charter", "Georgia", "Palatino", "Helvetica Neue"} {
		if !slices.Contains(fonts, want) {
			t.Errorf("Fonts() = %q, want %q among them", fonts, want)
		}
	}
	var order []string
	for _, f := range offeredFonts {
		if slices.Contains(fonts, f.family) {
			order = append(order, f.family)
		}
	}
	if !slices.Equal(fonts, order) {
		t.Errorf("Fonts() = %q, want them in the offered order %q", fonts, order)
	}
}

// TestSetTypography sets a book in other typographies and checks that each one takes
// effect while the reader stays at the same text.
func TestSetTypography(t *testing.T) {
	chapters := append(bookChapters(2, 120), "<pre>iiiiiiiiii\nMMMMMMMMMM</pre>")
	s, err := Open(testbook.Chapters(t, chapters...), Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.SetTypography(Typography{}); err != nil {
		t.Fatalf("SetStyle before a layout error: %v", err)
	}
	geo := NewGeometry(600, 700, 0, 1, 14, SpreadSingle, MarginsNormal)
	if err := s.Layout(geo, 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if _, err := s.GoTo(s.Pages() / 2); err != nil {
		t.Fatalf("GoTo error: %v", err)
	}
	page, at := s.Page(), s.Location()
	anchor := Anchor{Chapter: at.Chapter, Offset: at.Offset}
	passage := Range{Start: anchor, End: Anchor{Chapter: at.Chapter, Offset: at.Offset + 60}}
	text, err := s.RangeText(passage)
	if err != nil || len(text) < 40 {
		t.Fatalf("RangeText(%+v) = %q, %v; want a passage", passage, text, err)
	}

	pagesBy := map[LineSpacing]int{}
	for _, style := range []Typography{
		{Font: "Georgia"},
		{Font: "Avenir Next", LineSpacing: LineSpacingRelaxed, Justify: true},
		{Font: "Palatino", LineSpacing: LineSpacingCompact},
		{LineSpacing: LineSpacingRelaxed},
		{LineSpacing: LineSpacingCompact},
		{},
	} {
		if err := s.SetTypography(style); err != nil {
			t.Fatalf("SetTypography(%+v) error: %v", style, err)
		}
		if err := s.Layout(geo, 14); err != nil {
			t.Fatalf("Layout in %+v error: %v", style, err)
		}
		if s.Typography() != style {
			t.Errorf("Typography() = %+v after SetTypography(%+v)", s.Typography(), style)
		}
		if style.Font == "" {
			pagesBy[style.LineSpacing] = s.Pages()
		}
		visible, err := s.VisibleRange()
		if err != nil {
			t.Fatalf("VisibleRange in %+v error: %v", style, err)
		}
		if visible.Start.Compare(anchor) > 0 || anchor.Compare(visible.End) >= 0 {
			t.Errorf("in %+v the screen shows %+v, not the reader's anchor %+v", style, visible, anchor)
		}
		if got, err := s.RangeText(passage); err != nil || got != text {
			t.Errorf("RangeText(%+v) in %+v = %q, %v; want %q", passage, style, got, err, text)
		}
		if rects, err := s.RangeRects(passage); err != nil || len(rects) == 0 {
			t.Errorf("RangeRects(%+v) in %+v = %v, %v; want the lines of the passage", passage, style, rects, err)
		}
		pre, _ := s.doc.PageNumber(2, 0)
		pt, err := s.pageText(pre)
		if err != nil || len(pt.lines) < 2 {
			t.Fatalf("pre page text in %+v: %v, %v", style, pt, err)
		}
		w0, w1 := pt.lines[0].box.X1-pt.lines[0].box.X0, pt.lines[1].box.X1-pt.lines[1].box.X0
		if w0 < 0.9*w1 || w0 > 1.1*w1 {
			t.Errorf("in %+v, ten i in <pre> are %.1f pt wide and ten M %.1f pt; want a monospaced font", style, w0, w1)
		}
	}
	if !(pagesBy[LineSpacingCompact] < pagesBy[LineSpacingNormal] && pagesBy[LineSpacingNormal] < pagesBy[LineSpacingRelaxed]) {
		t.Errorf("pages by line spacing = %v, want fewer pages for tighter lines", pagesBy)
	}
	if s.Page() != page || s.Location() != at {
		t.Errorf("back in the first style: page %d at %+v, want page %d at %+v", s.Page(), s.Location(), page, at)
	}
}

// TestJustify checks that justified lines end at the same edge.
func TestJustify(t *testing.T) {
	words := strings.Fields("a reader sets long and short words in lines of text that wrap at different places on the page")
	var p strings.Builder
	for i := range 120 {
		p.WriteString(words[(i*7)%len(words)] + " ")
	}
	path := testbook.Chapters(t, "<p>"+p.String()+"</p>")
	full := func(style Typography) float64 {
		t.Helper()
		s, err := Open(path, Location{Offset: -1})
		if err != nil {
			t.Fatalf("Open error: %v", err)
		}
		defer s.Close()
		if err := s.SetTypography(style); err != nil {
			t.Fatalf("SetStyle error: %v", err)
		}
		if err := s.Layout(NewGeometry(600, 700, 0, 1, 14, SpreadSingle, MarginsNormal), 14); err != nil {
			t.Fatalf("Layout error: %v", err)
		}
		pt, err := s.pageText(0)
		if err != nil || len(pt.lines) < 10 {
			t.Fatalf("pageText = %d lines, %v; want a long paragraph", len(pt.lines), err)
		}
		lines := pt.lines[:len(pt.lines)-1] // the last line of a paragraph is never justified
		right := 0.0
		for _, l := range lines {
			right = max(right, l.box.X1)
		}
		n := 0
		for _, l := range lines {
			if right-l.box.X1 < 1 {
				n++
			}
		}
		return float64(n) / float64(len(lines))
	}
	left, justified := full(Typography{}), full(Typography{Justify: true})
	if justified < 0.9 || left > 0.5 {
		t.Errorf("lines reaching the right edge: %.0f%% justified, %.0f%% aligned left; want nearly all and few", justified*100, left*100)
	}
}

// TestTypographyInBook sets a real book in every offered font and in other
// spacings and margins, and checks that the reader stays at the same text
// and that a search hit and its highlight still fall on it. Set
// MOLE_TEST_EPUB to an EPUB path.
func TestTypographyInBook(t *testing.T) {
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
	// A search hit in the middle of the book, and the screen that shows it.
	sr, err := s.Search("continuous delivery")
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	t.Cleanup(sr.Close)
	res, err := sr.Results()
	for ; err == nil && !res.Done; res, err = sr.Results() {
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || len(res.Hits) < 3 {
		t.Fatalf("Search = %d hits, %v; want several", len(res.Hits), err)
	}
	hit := res.Hits[len(res.Hits)/2]
	if _, err := s.GoToAnchor(hit.Range.Start); err != nil {
		t.Fatalf("GoToAnchor error: %v", err)
	}
	visible, err := s.VisibleRange()
	if err != nil {
		t.Fatalf("VisibleRange error: %v", err)
	}
	anchor := visible.Start
	word, err := s.RangeText(hit.Range)
	if err != nil {
		t.Fatalf("RangeText error: %v", err)
	}

	type setting struct {
		style   Typography
		margins Margins
	}
	var settings []setting
	for _, f := range Fonts() {
		settings = append(settings, setting{style: Typography{Font: f}})
	}
	settings = append(settings,
		setting{style: Typography{Font: "Charter", LineSpacing: LineSpacingRelaxed, Justify: true}, margins: MarginsWide},
		setting{style: Typography{LineSpacing: LineSpacingCompact}, margins: MarginsNarrow},
		setting{})
	for _, st := range settings {
		start := time.Now()
		if err := s.SetTypography(st.style); err != nil {
			t.Fatalf("SetTypography(%+v) error: %v", st.style, err)
		}
		if err := s.Layout(NewGeometry(1384, 874, 52, 2, 21, SpreadAuto, st.margins), 21); err != nil {
			t.Fatalf("Layout with margins %d error: %v", st.margins, err)
		}
		t.Logf("%+v, margins %d: %d pages in %v", st.style, st.margins, s.Pages(), time.Since(start))
		shown, err := s.VisibleRange()
		if err != nil {
			t.Fatalf("VisibleRange error: %v", err)
		}
		if shown.Start.Compare(anchor) > 0 || anchor.Compare(shown.End) >= 0 {
			t.Errorf("in %+v the screen shows %+v, not the reader's anchor %+v", st.style, shown, anchor)
		}
		if got, err := s.RangeText(hit.Range); err != nil || got != word {
			t.Errorf("RangeText of the hit in %+v = %q, %v; want %q", st.style, got, err, word)
		}
		if page, err := s.AnchorPage(hit.Range.Start); err != nil {
			t.Errorf("AnchorPage of the hit in %+v error: %v", st.style, err)
		} else if _, err := s.GoTo(page); err != nil {
			t.Fatalf("GoTo error: %v", err)
		}
		rects, err := s.RangeRects(hit.Range)
		if err != nil || len(rects) == 0 {
			t.Fatalf("RangeRects of the hit in %+v = %v, %v; want its box", st.style, rects, err)
		}
		// The box holds the hit: a hit test at its center finds a letter of it.
		r := rects[0]
		a, ok, err := s.HitTest(r.X+r.Width/2, r.Y+r.Height/2)
		if err != nil || !ok || a.Compare(hit.Range.Start) < 0 || a.Compare(hit.Range.End) >= 0 {
			t.Errorf("HitTest at the center of the hit's box in %+v = %+v, %v, %v; want inside %+v", st.style, a, ok, err, hit.Range)
		}
		if _, err := s.GoToAnchor(anchor); err != nil {
			t.Fatalf("GoToAnchor error: %v", err)
		}
	}
}

// TestParagraphsInStyles checks that the paragraphs read aloud and read by
// VoiceOver stay whole in relaxed and justified text, whose lines MuPDF
// splits into blocks and pieces, and that their boxes cover their text.
func TestParagraphsInStyles(t *testing.T) {
	words := strings.Fields("a reader sets long and short words in lines of text that wrap at different places")
	var paragraph strings.Builder
	for i := range 60 {
		paragraph.WriteString(words[(i*5)%len(words)] + " ")
	}
	body := strings.Repeat("<p>"+paragraph.String()+"</p>", 3)
	s, err := Open(testbook.Chapters(t, body), Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	for _, style := range []Typography{
		{},
		{LineSpacing: LineSpacingRelaxed},
		{Justify: true},
		{Font: "Charter", LineSpacing: LineSpacingRelaxed, Justify: true},
	} {
		if err := s.SetTypography(style); err != nil {
			t.Fatalf("SetTypography(%+v) error: %v", style, err)
		}
		if err := s.Layout(NewGeometry(700, 2000, 0, 1, 14, SpreadSingle, MarginsNormal), 14); err != nil {
			t.Fatalf("Layout in %+v error: %v", style, err)
		}
		visible, err := s.VisibleRange()
		if err != nil {
			t.Fatalf("VisibleRange error: %v", err)
		}
		parts, err := s.Paragraphs(visible, 0)
		if err != nil || len(joined(parts)) != 3 {
			t.Fatalf("Paragraphs in %+v = %d paragraphs (%v), want 3", style, len(joined(parts)), err)
		}
		for _, p := range parts {
			rects, err := s.RangeRects(p.Range)
			if err != nil || len(rects) < 3 {
				t.Fatalf("RangeRects of a paragraph in %+v = %d boxes, %v; want one per line", style, len(rects), err)
			}
			for _, r := range rects {
				a, ok, err := s.HitTest(r.X+r.Width/2, r.Y+r.Height/2)
				if err != nil || !ok || a.Compare(p.Range.Start) < 0 || a.Compare(p.Range.End) >= 0 {
					t.Errorf("in %+v, the middle of a line box of %+v hits %+v, %v, %v; want inside", style, p.Range, a, ok, err)
				}
			}
		}
	}
}

// TestBookColorsInTypography checks that the book's text colors survive the
// reader's typography: a font of the reader's, relaxed lines and justified
// paragraphs change the style sheet, never the colors drawn at render time.
func TestBookColorsInTypography(t *testing.T) {
	s := openColors(t)
	geo := NewGeometry(600, 700, 0, 1, 14, SpreadSingle, MarginsNormal)
	for _, typography := range []Typography{{}, {Font: "Georgia", LineSpacing: LineSpacingRelaxed, Justify: true}} {
		if err := s.SetTypography(typography); err != nil {
			t.Fatalf("SetTypography(%+v) error: %v", typography, err)
		}
		if err := s.Layout(geo, 14); err != nil {
			t.Fatalf("Layout in %+v error: %v", typography, err)
		}
		renderChapter(t, s, 0, Style{BookColors: true})
		layer, drawn := s.ColorLayer()
		red := pixels(layer, func(r, g, b, a int) bool { return a > 200 && r > g+60 && r > b+60 })
		blue := pixels(layer, func(r, g, b, a int) bool { return a > 200 && b > r+60 })
		if !drawn || red == 0 || blue == 0 {
			t.Errorf("in %+v with book colors: drawn %v, %d red and %d blue pixels; want the colored text in its hues",
				typography, drawn, red, blue)
		}
	}
}

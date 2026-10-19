package reading

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"mole/core/internal/testbook"
)

func TestNewGeometry(t *testing.T) {
	tests := []struct {
		name          string
		width, height float64
		topInset      float64
		fontSize      float64
		spread        Spread
		wantColumns   int
	}{
		{name: "WideWindowTwoColumns", width: 1384, height: 874, topInset: 52, fontSize: 21, wantColumns: 2},
		{name: "NarrowWindowOneColumn", width: 800, height: 874, topInset: 52, fontSize: 21, wantColumns: 1},
		{name: "LargeFontOneColumn", width: 1384, height: 874, topInset: 52, fontSize: 30, wantColumns: 1},
		{name: "SingleOnWideWindow", width: 1384, height: 874, topInset: 52, fontSize: 21, spread: SpreadSingle, wantColumns: 1},
		{name: "DoubleOnNarrowWindow", width: 800, height: 874, topInset: 52, fontSize: 21, spread: SpreadDouble, wantColumns: 2},
		{name: "DoubleTooNarrow", width: 300, height: 874, topInset: 52, fontSize: 21, spread: SpreadDouble, wantColumns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGeometry(tt.width, tt.height, tt.topInset, 2, tt.fontSize, tt.spread)
			if g.Columns != tt.wantColumns {
				t.Errorf("NewGeometry(%v x %v, font %v).Columns = %d, want %d", tt.width, tt.height, tt.fontSize, g.Columns, tt.wantColumns)
			}
			if g.Top < tt.topInset {
				t.Errorf("Top = %v, want at least the toolbar inset %v", g.Top, tt.topInset)
			}
			right := g.Left + float64(g.Columns)*g.ColumnWidth + float64(g.Columns-1)*g.Gap
			if g.Left < 0 || right > tt.width+0.001 || g.Top+g.PageHeight > tt.height {
				t.Errorf("pages %v..%v x %v..%v fall outside the %v x %v view", g.Left, right, g.Top, g.Top+g.PageHeight, tt.width, tt.height)
			}
			if g.ColumnWidth > 32*tt.fontSize {
				t.Errorf("ColumnWidth = %v, want at most %v (32 em)", g.ColumnWidth, 32*tt.fontSize)
			}
		})
	}
}

// bookText returns chapters of numbered paragraphs, long enough for many
// pages.
func bookChapters(chapters, paragraphs int) []string {
	out := make([]string, chapters)
	for c := range out {
		var b strings.Builder
		fmt.Fprintf(&b, "<h1>Chapter %d</h1>", c+1)
		for p := range paragraphs {
			fmt.Fprintf(&b, "<p>Paragraph %d of chapter %d tells a plain story in words that wrap over a few lines of the page.</p>", p+1, c+1)
		}
		out[c] = b.String()
	}
	return out
}

func TestSpreads(t *testing.T) {
	s, err := Open(testbook.Chapters(t, bookChapters(3, 40)...), Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Layout(NewGeometry(800, 600, 0, 1, 14, SpreadDouble), 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if s.Geometry().Columns != 2 || s.Pages() < 10 {
		t.Fatalf("Layout: %d columns, %d pages; want 2 columns and at least 10 pages", s.Geometry().Columns, s.Pages())
	}
	if changed, err := s.Turn(1); err != nil || !changed || s.Page() != 2 || s.LastVisible() != 3 {
		t.Errorf("Turn(1) = %v, %v: pages %d..%d, want 2..3", changed, err, s.Page(), s.LastVisible())
	}
	for {
		changed, err := s.Turn(1)
		if err != nil {
			t.Fatalf("Turn(1) error: %v", err)
		}
		if !changed {
			break
		}
		if s.Page()%2 != 0 {
			t.Fatalf("Turn(1) showed page %d first, want an even page", s.Page())
		}
	}
	if s.LastVisible() != s.Pages()-1 || s.Progress() != 1 {
		t.Errorf("last screen: pages %d..%d of %d, progress %v; want the last page and progress 1", s.Page(), s.LastVisible(), s.Pages(), s.Progress())
	}
	last := s.Page()
	if changed, _ := s.Turn(-1); !changed || s.Page() != last-2 {
		t.Errorf("Turn(-1) from the last screen: page %d, want %d", s.Page(), last-2)
	}
	if changed, _ := s.GoTo(5); !changed || s.Page() != 4 {
		t.Errorf("GoTo(5): first page %d, want the screen 4..5", s.Page())
	}
	if changed, _ := s.GoTo(4); changed {
		t.Error("GoTo(4) on screen 4..5 reported a change")
	}
}

func TestLayoutKeepsPlace(t *testing.T) {
	path := testbook.Chapters(t, bookChapters(3, 40)...)
	s, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	narrow := NewGeometry(500, 600, 0, 1, 14, SpreadSingle)
	wide := NewGeometry(1300, 700, 0, 1, 14, SpreadDouble)
	if err := s.Layout(narrow, 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if _, err := s.GoTo(s.Pages() / 2); err != nil {
		t.Fatalf("GoTo error: %v", err)
	}
	page, at := s.Page(), s.Location()
	// Round trips through other layouts come back to the same page.
	for range 3 {
		for _, l := range []struct {
			geo  Geometry
			font float64
		}{{wide, 14}, {narrow, 22}, {wide, 11}, {narrow, 14}} {
			if err := s.Layout(l.geo, l.font); err != nil {
				t.Fatalf("Layout error: %v", err)
			}
			visible, err := s.VisibleRange()
			if err != nil {
				t.Fatalf("VisibleRange error: %v", err)
			}
			if a := (Anchor{Chapter: at.Chapter, Offset: at.Offset}); visible.Start.Compare(a) > 0 || a.Compare(visible.End) >= 0 {
				t.Fatalf("at %v pt on %d columns the screen shows %+v, not the anchor %+v", l.font, l.geo.Columns, visible, a)
			}
		}
	}
	if s.Page() != page || s.Location() != at {
		t.Errorf("after round trips: page %d at %+v, want page %d at %+v", s.Page(), s.Location(), page, at)
	}

	// Reopening at the saved location shows the same page.
	reopened, err := Open(path, at)
	if err != nil {
		t.Fatalf("reopen error: %v", err)
	}
	t.Cleanup(reopened.Close)
	if err := reopened.Layout(narrow, 14); err != nil {
		t.Fatalf("reopened Layout error: %v", err)
	}
	if reopened.Page() != page || reopened.Location() != at {
		t.Errorf("reopened at %+v: page %d at %+v, want page %d", at, reopened.Page(), reopened.Location(), page)
	}

	// A place saved before anchors existed is found by its chapter fraction.
	legacy, err := Open(path, Location{Chapter: at.Chapter, Fraction: at.Fraction, Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(legacy.Close)
	if err := legacy.Layout(narrow, 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	if diff := legacy.Page() - page; diff < -1 || diff > 1 {
		t.Errorf("opened at chapter %d fraction %.3f: page %d, want within one of %d", at.Chapter, at.Fraction, legacy.Page(), page)
	}
	if loc := legacy.Location(); loc.Offset < 0 {
		t.Errorf("Location after opening at a fraction = %+v, want an anchor", loc)
	}
}

func TestRenderLayers(t *testing.T) {
	line := image.NewGray(image.Rect(0, 0, 200, 200))
	dark := image.NewGray(image.Rect(0, 0, 200, 200))
	for y := range 200 {
		for x := range 200 {
			line.SetGray(x, y, color.Gray{Y: 255})
			if (x >= 95 && x < 105) || (y >= 95 && y < 105) {
				line.SetGray(x, y, color.Gray{Y: 0}) // a black cross on white
			}
			dark.SetGray(x, y, color.Gray{Y: 30})
			if x >= 50 && x < 150 && y >= 50 && y < 150 {
				dark.SetGray(x, y, color.Gray{Y: 220}) // a light square on dark gray
			}
		}
	}
	img := `<img src="%s" style="width:150pt;height:150pt"/>`
	path := testbook.Book{
		Chapters: []string{
			`<p>Only text on this page.</p>`,
			fmt.Sprintf(img, "line.png"),
			fmt.Sprintf(img, "dark.png"),
			`<div style="background-color:#000000;padding:3em"><p style="color:#ffffff">Text on a dark box.</p></div>`,
			`<pre>iiiiiiiiii
MMMMMMMMMM</pre>`,
		},
		Files: map[string][]byte{"line.png": testbook.PNG(t, line), "dark.png": testbook.PNG(t, dark)},
	}.Write(t)
	s, err := Open(path, Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Layout(NewGeometry(500, 600, 0, 1, 14, SpreadSingle), 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	render := func(chapter int) (text, images []byte, drawn bool) {
		t.Helper()
		page, ok := s.doc.PageNumber(chapter, 0)
		if !ok {
			t.Fatalf("no page in chapter %d", chapter)
		}
		s.GoTo(page)
		mask, _, err := s.Render()
		if err != nil {
			t.Fatalf("Render error: %v", err)
		}
		layer, drawn := s.ImageLayer()
		return mask, luma(layer), drawn
	}
	count := func(layer []byte, lo, hi byte) int {
		n := 0
		for _, v := range layer {
			if v >= lo && v <= hi {
				n++
			}
		}
		return n
	}

	text, images, drawn := render(0)
	if drawn || count(images, 1, 255) != 0 || count(text, 129, 255) == 0 {
		t.Errorf("text page: drawn %v, %d image pixels, %d ink pixels; want ink and no image", drawn, count(images, 1, 255), count(text, 129, 255))
	}

	// A dark picture keeps its tones: the area is mostly dark gray with a
	// light square, and nothing turns white.
	text, images, drawn = render(2)
	grays, lights := count(images, 20, 45), count(images, 205, 235)
	area := grays + lights
	if !drawn || count(text, 129, 255) != 0 || area < 1000 {
		t.Fatalf("dark image page: drawn %v, %d ink pixels, %d image pixels; want only an image", drawn, count(text, 129, 255), area)
	}
	if share := float64(lights) / float64(area); share < 0.15 || share > 0.35 || count(images, 240, 255) != 0 {
		t.Errorf("dark image: %.0f%% light, %d white pixels; want about 25%% light and no white (shown as it is)", share*100, count(images, 240, 255))
	}

	// Line art is inverted: the white paper turns black and the cross glows.
	text, images, drawn = render(1)
	cross := count(images, 200, 255)
	if !drawn || count(text, 129, 255) != 0 {
		t.Fatalf("line art page: drawn %v, %d ink pixels; want only an image", drawn, count(text, 129, 255))
	}
	if share := float64(cross) / float64(area); share < 0.03 || share > 0.2 {
		t.Errorf("line art: %d light pixels, %.0f%% of the image; want only the cross (about 10%%), inverted", cross, share*100)
	}

	// Backgrounds are not drawn: only the glyphs of the text on the box ink.
	text, _, _ = render(3)
	if ink := count(text, 129, 255); ink == 0 || ink > len(text)/50 {
		t.Errorf("dark box page: %d of %d pixels inked, want just the text", ink, len(text))
	}

	// Code keeps a monospaced font.
	page, _ := s.doc.PageNumber(4, 0)
	pt, err := s.pageText(page)
	if err != nil || len(pt.lines) < 2 {
		t.Fatalf("pre page text: %v, %v", pt, err)
	}
	w0, w1 := pt.lines[0].box.X1-pt.lines[0].box.X0, pt.lines[1].box.X1-pt.lines[1].box.X0
	if w0 < 0.9*w1 || w0 > 1.1*w1 {
		t.Errorf("in <pre>, ten i are %.1f pt wide and ten M %.1f pt; want a monospaced font", w0, w1)
	}
}

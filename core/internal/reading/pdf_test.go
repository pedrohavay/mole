package reading

import (
	"math"
	"testing"

	"mole/core/internal/mupdf"
	"mole/core/internal/testbook"
)

// The pages of testbook.SamplePDF.
const (
	pdfCover   = 0
	pdfText    = 1 // the first of three pages of text
	pdfFigures = 4
	pdfScan    = 5
)

// openPDF opens the sample PDF of testbook in a view of width x height
// points at one pixel per point, showing pages side by side as spread asks.
func openPDF(t *testing.T, width, height float64, spread Spread) *Session {
	t.Helper()
	s, err := Open(testbook.SamplePDF().Write(t), Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	layoutPDF(t, s, width, height, spread)
	return s
}

func layoutPDF(t *testing.T, s *Session, width, height float64, spread Spread) {
	t.Helper()
	if err := s.Layout(NewGeometry(width, height, 0, 1, 18, spread, MarginsNormal), 18); err != nil {
		t.Fatalf("Layout(%v x %v) error: %v", width, height, err)
	}
}

// renderPage shows page and renders it with style.
func renderPage(t *testing.T, s *Session, page int, style Style) (mask, images []byte) {
	t.Helper()
	if _, err := s.GoTo(page); err != nil {
		t.Fatalf("GoTo(%d) error: %v", page, err)
	}
	if _, err := s.SetStyle(style); err != nil {
		t.Fatalf("SetStyle error: %v", err)
	}
	mask, _, err := s.Render()
	if err != nil {
		t.Fatalf("Render page %d error: %v", page, err)
	}
	images, _ = s.ImageLayer()
	return mask, images
}

func inked(mask []byte) int {
	n := 0
	for _, c := range mask {
		if c > 128 {
			n++
		}
	}
	return n
}

func TestPDFLayout(t *testing.T) {
	s := openPDF(t, 800, 700, SpreadSingle)
	if !s.FixedLayout() || s.Pages() != 6 {
		t.Fatalf("FixedLayout, Pages = %v, %d; want true, 6", s.FixedLayout(), s.Pages())
	}
	want := []Chapter{
		{Title: "Cover", Page: 0}, {Title: "Chapter One: Tunnels", Page: 1}, {Title: "Chapter Two: Lanterns", Page: 2},
		{Title: "Chapter Three: Maps", Page: 3}, {Title: "Figures", Level: 1, Page: 4}, {Title: "Scanned Page", Page: 5},
	}
	got := s.Chapters()
	if len(got) != len(want) {
		t.Fatalf("Chapters() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Chapters()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if language := s.Language(); language != "en" {
		t.Errorf("Language() = %q, want %q", language, "en")
	}

	// A whole page fits the view, centered, with a pad around it.
	frames, err := s.frames()
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames() = %+v, %v; want one page", frames, err)
	}
	f := frames[0]
	if want := (700.0 - 2*fixedPad) / 792; math.Abs(f.zoom-want) > 1e-9 {
		t.Errorf("zoom = %v, want %v, the page's height filling the view", f.zoom, want)
	}
	if right := f.x + f.width*f.zoom; math.Abs(f.x-(800-right)) > 1 || f.y < fixedPad-1 {
		t.Errorf("page at x %v..%v, y %v; want it centered in the 800 point view", f.x, right, f.y)
	}

	tests := []struct {
		name          string
		width, height float64
		spread        Spread
		want          int
	}{
		{name: "WideAuto", width: 1400, height: 800, spread: SpreadAuto, want: 2},
		{name: "TallAuto", width: 800, height: 900, spread: SpreadAuto, want: 1},
		{name: "WideSingle", width: 1400, height: 800, spread: SpreadSingle, want: 1},
		{name: "TallDouble", width: 800, height: 900, spread: SpreadDouble, want: 2},
		{name: "NarrowDouble", width: 260, height: 900, spread: SpreadDouble, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layoutPDF(t, s, tt.width, tt.height, tt.spread)
			if got := s.Geometry().Columns; got != tt.want {
				t.Errorf("columns in a %v x %v view = %d, want %d", tt.width, tt.height, got, tt.want)
			}
			frames, err := s.frames()
			if err != nil {
				t.Fatalf("frames() error: %v", err)
			}
			for _, f := range frames {
				if f.x < 0 || f.x+f.width*f.zoom > tt.width+0.5 || f.y < 0 || f.y+f.height*f.zoom > tt.height+0.5 {
					t.Errorf("page %d at %+v falls outside the %v x %v view", f.page, f, tt.width, tt.height)
				}
			}
		})
	}
}

func TestPDFAnchors(t *testing.T) {
	s := openPDF(t, 800, 700, SpreadSingle)
	if _, err := s.GoTo(pdfText + 1); err != nil {
		t.Fatalf("GoTo error: %v", err)
	}
	r, err := s.VisibleRange()
	if err != nil {
		t.Fatalf("VisibleRange error: %v", err)
	}
	// Each page is a chapter, and offsets count within it.
	if r.Start != (Anchor{Chapter: pdfText + 1}) || r.End.Chapter != pdfText+1 || r.End.Offset < 500 {
		t.Errorf("VisibleRange() = %+v, want all of page %d", r, pdfText+1)
	}
	hit := find(t, s, pdfText+2, "number 8")
	if changed, err := s.GoToAnchor(hit.Start); err != nil || !changed || s.Page() != pdfText+2 {
		t.Errorf("GoToAnchor(%+v) = %v, %v at page %d; want page %d", hit.Start, changed, err, s.Page(), pdfText+2)
	}
	if text, err := s.RangeText(hit); err != nil || text != "number 8" {
		t.Errorf("RangeText(%+v) = %q, %v; want %q", hit, text, err, "number 8")
	}
	loc := s.Location()
	if loc.Chapter != pdfText+2 || loc.Offset != hit.Start.Offset {
		t.Errorf("Location() = %+v, want the anchor %+v", loc, hit.Start)
	}

	// The place holds in another view and after reopening.
	layoutPDF(t, s, 1400, 800, SpreadAuto)
	if s.Page() != pdfText+1 || s.LastVisible() != pdfText+2 {
		t.Errorf("two pages side by side show %d-%d, want %d-%d", s.Page(), s.LastVisible(), pdfText+1, pdfText+2)
	}
	again, err := Open(s.path, loc)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer again.Close()
	layoutPDF(t, again, 800, 700, SpreadSingle)
	if again.Page() != pdfText+2 {
		t.Errorf("reopened at page %d, want %d", again.Page(), pdfText+2)
	}

	// Typography does not apply to fixed pages, and leaves them alone.
	doc := s.doc
	if err := s.SetTypography(Typography{Font: "Georgia", LineSpacing: LineSpacingRelaxed}); err != nil || s.doc != doc {
		t.Errorf("SetTypography reopened the PDF or failed: %v", err)
	}

	if n, err := s.VisibleCharacters(); err != nil || n < 1000 {
		t.Errorf("VisibleCharacters() = %d, %v; want the text of two pages", n, err)
	}
}

func TestPDFSearch(t *testing.T) {
	s := openPDF(t, 800, 700, SpreadSingle)
	res := search(t, s, "lantern number 5")
	if len(res.Hits) != 1 {
		t.Fatalf("Search(lantern number 5) = %+v, want one hit", res.Hits)
	}
	hit := res.Hits[0]
	if hit.Range.Start.Chapter != pdfText+1 {
		t.Errorf("hit in chapter %d, want page %d", hit.Range.Start.Chapter, pdfText+1)
	}
	if text, err := s.RangeText(hit.Range); err != nil || text != "lantern number 5" {
		t.Errorf("RangeText(hit) = %q, %v; want the match", text, err)
	}
	if res := search(t, s, "Mole reads fixed pages"); len(res.Hits) != 72 {
		t.Errorf("Search(Mole reads fixed pages) found %d hits, want 72, eight lines of nine parts", len(res.Hits))
	}
	paragraphs, err := s.Paragraphs(Range{Start: Anchor{Chapter: pdfText}, End: Anchor{Chapter: pdfText + 1}}, 0)
	if err != nil || len(paragraphs) == 0 || paragraphs[0].Text != "Chapter One: Tunnels" {
		t.Errorf("Paragraphs(page %d) = %+v, %v; want the title first", pdfText, paragraphs, err)
	}
}

func TestPDFRender(t *testing.T) {
	s := openPDF(t, 700, 900, SpreadSingle)
	// Text lights the mask; a page of text alone draws no picture.
	mask, _ := renderPage(t, s, pdfText, Style{})
	if _, drawn := s.ImageLayer(); inked(mask) < 1000 || drawn {
		t.Errorf("page of text: %d inked pixels, pictures drawn %v; want text and no picture", inked(mask), drawn)
	}

	frames, err := s.frames()
	if err != nil {
		t.Fatalf("frames() error: %v", err)
	}
	f := frames[0]
	// at returns the offset of the pixel at the point x, y of the page,
	// counted from its top left corner.
	at := func(x, y float64) int {
		return int(f.y+y*f.zoom)*s.Stride() + int(f.x+x*f.zoom)
	}

	// The figure page: its drawing goes to the pictures, not the text,
	// and its white paper is left out.
	mask, images := renderPage(t, s, pdfFigures, Style{})
	if _, drawn := s.ImageLayer(); !drawn {
		t.Fatal("the figure page drew no picture")
	}
	below := 0
	for y := int(f.y + 120*f.zoom); y < int(f.y+f.height*f.zoom); y++ {
		for x := 0; x < s.Stride(); x++ {
			if mask[y*s.Stride()+x] > 0 {
				below++
			}
		}
	}
	if below > 0 {
		t.Errorf("%d pixels of the mask are lit below the caption, where only drawings and a picture are", below)
	}
	lit := pixels(images, func(r, g, b, a int) bool { return a > 0 && r+g+b > 600 })
	if total := len(images) / 4; lit > total/20 {
		t.Errorf("%d of %d pixels of the picture layer are near white, want the paper left out", lit, total)
	}
	// A black rule of the table glows at night (inverted), and the gray
	// shading of its row goes dark.
	rule := images[4*at(300, 792-348):]
	if rule[0] < 150 || rule[3] < 150 {
		t.Errorf("table rule at night = %v, want light", rule[:4])
	}
	shade := images[4*at(300, 792-312):]
	if shade[3] < 200 || shade[0] > 60 {
		t.Errorf("table shading at night = %v, want an opaque dark gray", shade[:4])
	}

	// The cover's blue bar: lighter at night, as printed in original
	// colors, and not drawn when pictures are hidden.
	_, images = renderPage(t, s, pdfCover, Style{})
	night := append([]byte(nil), images[4*at(300, 792-524):][:4]...)
	_, images = renderPage(t, s, pdfCover, Style{Pictures: mupdf.PicturesOriginal})
	original := images[4*at(300, 792-524):][:4]
	if !(night[2] > original[2] && night[0] > original[0]) || original[2] < 150 || original[2] > 200 {
		t.Errorf("blue bar at night %v and in original colors %v; want the night one lighter, the original as printed", night, original)
	}
	if _, images := renderPage(t, s, pdfCover, Style{Pictures: mupdf.PicturesHidden}); images != nil {
		if _, drawn := s.ImageLayer(); drawn {
			t.Error("hidden pictures still drew the cover's bar")
		}
	}
}

func TestPDFScannedPage(t *testing.T) {
	s := openPDF(t, 700, 900, SpreadSingle)
	mask, images := renderPage(t, s, pdfScan, Style{})
	if inked(mask) != 0 {
		t.Errorf("the scanned page lit %d pixels of the mask, want none: it has no text", inked(mask))
	}
	if _, drawn := s.ImageLayer(); !drawn {
		t.Fatal("the scanned page drew no picture")
	}
	// At night the scan is line art: its paper goes dark and its words glow.
	dark := pixels(images, func(r, g, b, a int) bool { return a > 0 && r < 40 })
	light := pixels(images, func(r, g, b, a int) bool { return a > 0 && r > 180 })
	if dark < 2*light || light == 0 {
		t.Errorf("scan at night: %d dark and %d light pixels; want dark paper with light words", dark, light)
	}
	r, err := s.VisibleRange()
	if err != nil || r.Start != (Anchor{Chapter: pdfScan}) || r.End != (Anchor{Chapter: pdfScan, Offset: 39}) {
		t.Errorf("VisibleRange() = %+v, %v; want the 39 letters of the OCR text of page %d", r, err, pdfScan)
	}
	if loc := s.Location(); loc.Chapter != pdfScan || loc.Offset != 0 {
		t.Errorf("Location() = %+v, want page %d", loc, pdfScan)
	}
	if p, ok, err := s.PictureAt(350, 450); err != nil || !ok || p.Page != pdfScan {
		t.Errorf("PictureAt(middle) = %+v, %v, %v; want the scan", p, ok, err)
	}
	// Its invisible OCR text is not drawn, yet it can be found and selected.
	res := search(t, s, "OCR found")
	if len(res.Hits) != 1 || res.Hits[0].Range.Start.Chapter != pdfScan {
		t.Fatalf("Search(OCR found) = %+v, want a hit on the scan", res.Hits)
	}
	if rects, err := s.RangeRects(res.Hits[0].Range); err != nil || len(rects) != 1 {
		t.Errorf("RangeRects(OCR hit) = %+v, %v; want a box over the scan", rects, err)
	}
}

func TestPDFZoom(t *testing.T) {
	s := openPDF(t, 800, 500, SpreadSingle)
	if _, err := s.GoTo(pdfText); err != nil {
		t.Fatalf("GoTo error: %v", err)
	}
	vp, err := s.Viewport()
	if err != nil {
		t.Fatalf("Viewport error: %v", err)
	}
	if vp.Zoom != (Zoom{}) || vp.Scale != vp.FitPage || vp.MaxX != 0 || vp.MaxY != 0 || vp.FitWidth <= vp.FitPage {
		t.Errorf("Viewport() = %+v; want whole pages that fit, wider than they are", vp)
	}

	if changed, err := s.SetZoom(Zoom{Fit: FitWidth}); err != nil || !changed {
		t.Fatalf("SetZoom(FitWidth) = %v, %v", changed, err)
	}
	// The middle of the page stays in the middle of the view.
	vp, _ = s.Viewport()
	if vp.Scale != vp.FitWidth || vp.MaxX != 0 || vp.MaxY <= 0 || math.Abs(vp.Y-vp.MaxY/2) > 1 {
		t.Errorf("at the width of the view, Viewport() = %+v; want a page to scroll, halfway down", vp)
	}
	if _, err := s.Pan(0, -1e9); err != nil {
		t.Fatalf("Pan error: %v", err)
	}
	if moved, err := s.Pan(50, 100); err != nil || !moved {
		t.Errorf("Pan(50, 100) = %v, %v; want it to move", moved, err)
	}
	if vp, _ = s.Viewport(); vp.X != 0 || vp.Y != 100 {
		t.Errorf("after Pan(50, 100), scrolled to %v, %v; want 0, 100", vp.X, vp.Y)
	}
	if _, err := s.Pan(0, 1e9); err != nil {
		t.Fatalf("Pan error: %v", err)
	}
	if vp, _ = s.Viewport(); vp.Y != vp.MaxY {
		t.Errorf("after a long Pan, scrolled to %v, want the bottom %v", vp.Y, vp.MaxY)
	}
	mask, _, err := s.Render()
	if err != nil || inked(mask) < 100 {
		t.Errorf("Render at the width of the view: %d inked pixels, %v", inked(mask), err)
	}

	// Hit tests and boxes follow the zoom and the scroll.
	hit := find(t, s, s.Page(), "number 3")
	rects, err := s.RangeRects(hit)
	if err != nil || len(rects) != 1 {
		t.Fatalf("RangeRects(number 3) = %+v, %v; want one box in view", rects, err)
	}
	box := rects[0]
	if got, ok, err := s.HitTest(box.X+box.Width-1, box.Y+box.Height/2); err != nil || !ok || got.Offset != hit.End.Offset-1 {
		t.Errorf("HitTest(end of number 3) = %+v, %v, %v; want offset %d", got, ok, err, hit.End.Offset-1)
	}
	if mask[int(box.Y+box.Height/2)*s.Stride()+int(box.X+box.Width/2)] == 0 &&
		mask[int(box.Y+box.Height/2)*s.Stride()+int(box.X+2)] == 0 {
		t.Errorf("no ink under the box %+v of number 3", box)
	}
	title := find(t, s, s.Page(), "Chapter One")
	if rects, err := s.RangeRects(title); err != nil || len(rects) != 0 {
		t.Errorf("RangeRects(title scrolled out of view) = %+v, %v; want none", rects, err)
	}

	// Going to an anchor scrolls it into view; turning the page shows its top.
	if changed, err := s.GoToAnchor(title.Start); err != nil || !changed {
		t.Errorf("GoToAnchor(title) = %v, %v; want the view to scroll", changed, err)
	}
	if rects, _ := s.RangeRects(title); len(rects) != 1 {
		t.Errorf("after GoToAnchor, the title has boxes %+v, want one in view", rects)
	}
	if _, err := s.Pan(0, 200); err != nil {
		t.Fatalf("Pan error: %v", err)
	}
	if _, err := s.Turn(1); err != nil {
		t.Fatalf("Turn error: %v", err)
	}
	if vp, _ = s.Viewport(); vp.Y != 0 {
		t.Errorf("after a page turn, scrolled to %v, want the top", vp.Y)
	}

	// Steps go up past the width and back down to whole pages, no further.
	if _, err := s.StepZoom(1); err != nil {
		t.Fatalf("StepZoom error: %v", err)
	}
	if vp, _ = s.Viewport(); vp.Zoom.Fit != FitNone || vp.Scale <= vp.FitWidth {
		t.Errorf("one step past the width: %+v", vp)
	}
	if _, err := s.StepZoom(-1); err != nil {
		t.Fatalf("StepZoom error: %v", err)
	}
	if vp, _ = s.Viewport(); vp.Zoom.Fit != FitWidth {
		t.Errorf("one step back: %+v, want the width", vp.Zoom)
	}
	for range 20 {
		if _, err := s.StepZoom(-1); err != nil {
			t.Fatalf("StepZoom error: %v", err)
		}
	}
	if vp, _ = s.Viewport(); vp.Zoom != (Zoom{}) {
		t.Errorf("after many steps back: %+v, want whole pages", vp.Zoom)
	}
	if changed, err := s.StepZoom(-1); err != nil || changed {
		t.Errorf("StepZoom(-1) at whole pages = %v, %v; want no change", changed, err)
	}
	for range 30 {
		if _, err := s.StepZoom(1); err != nil {
			t.Fatalf("StepZoom error: %v", err)
		}
	}
	if vp, _ = s.Viewport(); vp.Scale != maxZoom {
		t.Errorf("after many steps in, scale %v, want %v", vp.Scale, maxZoom)
	}
	if _, err := s.SetZoom(Zoom{Fit: FitNone, Scale: 0.01}); err != nil {
		t.Fatalf("SetZoom error: %v", err)
	}
	if vp, _ = s.Viewport(); vp.Zoom != (Zoom{}) {
		t.Errorf("a scale below whole pages gave %+v, want whole pages", vp.Zoom)
	}
	if _, err := s.SetZoom(Zoom{Fit: FitNone}); err == nil {
		t.Error("SetZoom(FitNone without a scale) succeeded")
	}
}

func TestZoomReflowable(t *testing.T) {
	s := openSession(t, testbook.Chapters(t, "<p>Words.</p>"), 400, 16)
	if s.FixedLayout() {
		t.Fatal("an EPUB has fixed pages")
	}
	if changed, err := s.StepZoom(1); err != nil || changed {
		t.Errorf("StepZoom on an EPUB = %v, %v; want nothing", changed, err)
	}
	if moved, err := s.Pan(0, 10); err != nil || moved {
		t.Errorf("Pan on an EPUB = %v, %v; want nothing", moved, err)
	}
	if _, err := s.Viewport(); err == nil {
		t.Error("Viewport on an EPUB succeeded")
	}
}

package reading

import (
	"testing"

	"mole/core/internal/mupdf"
	"mole/core/internal/testbook"
)

// luma returns the luma of each pixel of an RGBA layer, which is the
// picture on black since its alpha is premultiplied.
func luma(rgba []byte) []byte {
	out := make([]byte, len(rgba)/4)
	for i := range out {
		p := rgba[4*i:]
		out[i] = byte((77*int(p[0]) + 150*int(p[1]) + 29*int(p[2])) >> 8)
	}
	return out
}

// pixels counts the pixels of an RGBA layer for which keep is true.
func pixels(rgba []byte, keep func(r, g, b, a int) bool) int {
	n := 0
	for i := 0; i+3 < len(rgba); i += 4 {
		if keep(int(rgba[i]), int(rgba[i+1]), int(rgba[i+2]), int(rgba[i+3])) {
			n++
		}
	}
	return n
}

// openColors opens the color book of testbook laid out on one page.
func openColors(t *testing.T) *Session {
	t.Helper()
	return openColorsAt(t, 1)
}

// openColorsAt opens the color book of testbook laid out on one page at
// scale pixels per point.
func openColorsAt(t *testing.T, scale float64) *Session {
	t.Helper()
	s, err := Open(testbook.Colors(t).Write(t), Location{Offset: -1})
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(s.Close)
	if err := s.Layout(NewGeometry(600, 700, 0, scale, 14, SpreadSingle), 14); err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	return s
}

// renderChapter shows and renders the first page of chapter with style.
func renderChapter(t *testing.T, s *Session, chapter int, style Style) []byte {
	t.Helper()
	page, ok := s.doc.PageNumber(chapter, 0)
	if !ok {
		t.Fatalf("no page in chapter %d", chapter)
	}
	if _, err := s.GoTo(page); err != nil {
		t.Fatalf("GoTo(%d) error: %v", page, err)
	}
	if _, err := s.SetStyle(style); err != nil {
		t.Fatalf("SetStyle(%+v) error: %v", style, err)
	}
	mask, _, err := s.Render()
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	return mask
}

func TestColorLayer(t *testing.T) {
	s := openColors(t)

	mask := renderChapter(t, s, 0, Style{BookColors: true})
	layer, drawn := s.ColorLayer()
	if !drawn || len(layer) != 4*len(mask) {
		t.Fatalf("ColorLayer with book colors = %d bytes, drawn %v; want %d bytes, drawn", len(layer), drawn, 4*len(mask))
	}
	ink := 0
	for _, v := range mask {
		if v > 128 {
			ink++
		}
	}
	solid := func(keep func(r, g, b int) bool) int {
		return pixels(layer, func(r, g, b, a int) bool { return a > 200 && keep(r, g, b) })
	}
	red := solid(func(r, g, b int) bool { return r > g+60 && r > b+60 })
	blue := solid(func(r, g, b int) bool { return b > r+60 })
	green := solid(func(r, g, b int) bool { return g > r+40 && g > b+40 })
	colored := pixels(layer, func(_, _, _, a int) bool { return a > 128 })
	if red == 0 || blue == 0 || green == 0 {
		t.Errorf("colored text: %d red, %d blue, %d green pixels; want the heading, words and code in their hues", red, blue, green)
	}
	// Black, gray, near-black and white text keep the tint, so they are
	// left out, and every hue is drawn where the mask has ink.
	if colored == 0 || colored > ink*6/10 {
		t.Errorf("%d colored pixels for %d inked ones; want the colored text only", colored, ink)
	}
	if dark := solid(func(r, g, b int) bool { return max(r, g, b) < 128 }); dark != 0 {
		t.Errorf("%d dark colored pixels; want dark hues lightened for a black page", dark)
	}

	renderChapter(t, s, 0, Style{})
	if layer, drawn := s.ColorLayer(); drawn || layer != nil {
		t.Errorf("ColorLayer without book colors = %d bytes, drawn %v; want none", len(layer), drawn)
	}
}

func TestPictures(t *testing.T) {
	s := openColors(t)
	type stats struct {
		area, dark, white, red, sky int
	}
	measure := func(chapter int, pictures mupdf.Pictures) (stats, bool) {
		t.Helper()
		renderChapter(t, s, chapter, Style{Pictures: pictures})
		layer, drawn := s.ImageLayer()
		return stats{
			area:  pixels(layer, func(_, _, _, a int) bool { return a > 250 }),
			dark:  pixels(layer, func(r, g, b, a int) bool { return a > 250 && max(r, g, b) < 60 }),
			white: pixels(layer, func(r, g, b, a int) bool { return a > 250 && min(r, g, b) > 240 }),
			red:   pixels(layer, func(r, g, b, a int) bool { return a > 250 && r > 150 && g < 90 && b < 90 }),
			sky:   pixels(layer, func(r, g, b, a int) bool { return a > 250 && b > r+40 && b > 150 }),
		}, drawn
	}

	// A bright photograph keeps its colors at night: it is light, but not
	// line art.
	for _, pictures := range []mupdf.Pictures{mupdf.PicturesNight, mupdf.PicturesOriginal} {
		photo, drawn := measure(1, pictures)
		if !drawn || photo.area < 1000 || photo.sky < photo.area/3 || photo.red == 0 {
			t.Errorf("photograph with pictures %d: drawn %v, %+v; want blue sky and a red house", pictures, drawn, photo)
		}
	}

	// At night line art turns light on dark: the paper goes dark and the
	// red frame stays red. In original colors the paper stays white, also
	// behind the transparent drawing.
	night, _ := measure(2, mupdf.PicturesNight)
	original, _ := measure(2, mupdf.PicturesOriginal)
	if night.dark < night.area/2 || night.red == 0 || night.white > night.area/5 {
		t.Errorf("diagrams at night: %+v; want mostly dark paper, light lines and the red frame", night)
	}
	if original.white < original.area/2 || original.red == 0 {
		t.Errorf("diagrams in original colors: %+v; want white paper and the red frame", original)
	}

	renderChapter(t, s, 1, Style{Pictures: mupdf.PicturesHidden})
	if layer, drawn := s.ImageLayer(); drawn || layer != nil {
		t.Errorf("ImageLayer with hidden pictures = %d bytes, drawn %v; want none", len(layer), drawn)
	}
	if _, err := s.SetStyle(Style{Pictures: 7}); err == nil {
		t.Error("SetStyle with unknown pictures: got nil error")
	}
}

func TestPictureAt(t *testing.T) {
	s := openColors(t)
	renderChapter(t, s, 2, Style{})
	boxes, err := s.doc.PageImages(s.Page())
	if err != nil || len(boxes) != 2 {
		t.Fatalf("PageImages = %v, %v; want the diagram and the line art", boxes, err)
	}
	left, top := s.geo.pageOrigin(0)
	for i, b := range boxes {
		x, y := left+(b.X0+b.X1)/2, top+(b.Y0+b.Y1)/2
		p, ok, err := s.PictureAt(x, y)
		if err != nil || !ok || p.Page != s.Page() || p.Index != i {
			t.Errorf("PictureAt(%.0f, %.0f) = %+v, %v, %v; want image %d of page %d", x, y, p, ok, err, i, s.Page())
			continue
		}
		if p.Rect.X > x || p.Rect.X+p.Rect.Width < x || p.Rect.Width < 100 {
			t.Errorf("PictureAt(%.0f, %.0f).Rect = %+v, want the box around the point", x, y, p.Rect)
		}
	}
	// Above the first picture is a line of text.
	if p, ok, err := s.PictureAt(left+boxes[0].X0+5, top+boxes[0].Y0-8); ok || err != nil {
		t.Errorf("PictureAt over text = %+v, %v, %v; want none", p, ok, err)
	}

	diagram, err := s.PicturePixels(s.Page(), 0)
	if err != nil {
		t.Fatalf("PicturePixels(diagram) error: %v", err)
	}
	if b := diagram.Bounds(); b.Dx() != 400 || b.Dy() != 240 {
		t.Errorf("diagram is %v, want its own 400 x 240 pixels", b)
	}
	lines, err := s.PicturePixels(s.Page(), 1)
	if err != nil {
		t.Fatalf("PicturePixels(line art) error: %v", err)
	}
	// The transparent background shows as white paper.
	if paper, line := lines.RGBAAt(2, 2), lines.RGBAAt(12, 12); paper.R < 250 || paper.A != 255 || line.R > 10 {
		t.Errorf("line art pixels: paper %v, line %v; want opaque white paper and black lines", paper, line)
	}
	if _, err := s.PicturePixels(s.Page(), 2); err == nil {
		t.Error("PicturePixels past the last image of the page: got nil error")
	}
}

// TestDrawing checks a transparent SVG drawing, an image that MuPDF scales
// rather than decodes.
func TestDrawing(t *testing.T) {
	s := openColors(t)
	frame := func(r, g, b, a int) bool { return a > 250 && min(r, g, b) > 240 }
	dark := func(r, g, b, a int) bool { return a > 250 && max(r, g, b) < 20 }
	// At night the black frame glows on dark paper; in original colors it
	// is black on white. The session reuses its layers, so each is
	// measured before the next render.
	renderChapter(t, s, 3, Style{})
	night, _ := s.ImageLayer()
	if lit, paper := pixels(night, frame), pixels(night, dark); lit == 0 || paper < 4*lit {
		t.Errorf("drawing at night: %d light and %d dark pixels; want a light frame on dark paper", lit, paper)
	}
	renderChapter(t, s, 3, Style{Pictures: mupdf.PicturesOriginal})
	original, _ := s.ImageLayer()
	if paper, ink := pixels(original, frame), pixels(original, dark); ink == 0 || paper < 4*ink {
		t.Errorf("drawing in original colors: %d white and %d black pixels; want a black frame on white paper", paper, ink)
	}

	img, err := s.PicturePixels(s.Page(), 0)
	if err != nil {
		t.Fatalf("PicturePixels(drawing) error: %v", err)
	}
	if b := img.Bounds(); max(b.Dx(), b.Dy()) != MaxPictureSide {
		t.Errorf("drawing is %v, want %d pixels on its longer side", b, MaxPictureSide)
	}
	if paper := img.RGBAAt(2, 2); paper.R < 250 || paper.G < 250 {
		t.Errorf("drawing corner = %v, want white paper behind the transparent background", paper)
	}
	// Drawn at that size, not scaled up from a small bitmap, the frame's
	// edges are sharp: few gray pixels between black and white.
	gray := 0
	for x := range img.Bounds().Dx() / 4 {
		if v := img.RGBAAt(x, img.Bounds().Dy()/2).R; v > 30 && v < 225 {
			gray++
		}
	}
	if gray > 6 {
		t.Errorf("drawing: %d gray pixels across the frame's edges, want sharp edges", gray)
	}
}

// TestDrawingOnPage checks that an SVG drawing is drawn at the size it has
// on screen, as on a Retina display, rather than scaled up from its own
// small size: its frame has sharp edges in every style.
func TestDrawingOnPage(t *testing.T) {
	const scale = 2
	s := openColorsAt(t, scale)
	for _, style := range []Style{{Pictures: mupdf.PicturesNight}, {Pictures: mupdf.PicturesOriginal}} {
		renderChapter(t, s, 3, style)
		boxes, err := s.doc.PageImages(s.Page())
		if err != nil || len(boxes) != 1 {
			t.Fatalf("PageImages = %v, %v; want the drawing", boxes, err)
		}
		layer, _ := s.ImageLayer()
		left, top := s.geo.pageOrigin(0)
		b := boxes[0]
		x0, x1 := int((left+b.X0)*scale), int((left+b.X0+(b.X1-b.X0)/4)*scale)
		y := int((top + (b.Y0+b.Y1)/2) * scale)
		gray := 0
		for x := x0; x < x1; x++ {
			p := layer[4*(y*s.Stride()+x):]
			if v := (77*int(p[0]) + 150*int(p[1]) + 29*int(p[2])) >> 8; v > 30 && v < 225 {
				gray++
			}
		}
		if gray > 4 {
			t.Errorf("drawing with pictures %d: %d gray pixels across the frame's edges, want sharp edges", style.Pictures, gray)
		}
	}
}

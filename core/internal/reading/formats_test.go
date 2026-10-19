package reading

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"mole/core/internal/mupdf"
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

// TestFormatPictures checks that the pictures of a MOBI and an FB2 take the
// same path as an EPUB's: in color in the image layer, a photograph as it
// is and a diagram inverted at night, and found under the pointer with
// their own pixels for the viewer.
func TestFormatPictures(t *testing.T) {
	photo, diagram := testbook.JPEG(t, testbook.Photo()), testbook.PNG(t, testbook.Diagram())
	photoSize, diagramSize := testbook.Photo().Bounds().Size(), testbook.Diagram().Bounds().Size()
	cover := testbook.PNG(t, image.NewGray(image.Rect(0, 0, 60, 90)))
	mobi := testbook.MOBI{Title: "Pictures", Cover: cover, Images: [][]byte{photo, diagram}}
	mobi.HTML = `<p style="color:#B22222">Red words.</p><p>A photograph:</p><img recindex="` + mobi.ImageIndex(0) +
		`" style="width:300pt"/><mbp:pagebreak/><p>A diagram:</p><img recindex="` + mobi.ImageIndex(1) + `" style="width:300pt"/>`
	books := map[string]string{
		"MOBI": mobi.Write(t),
		"FB2": testbook.FB2{Title: "Pictures", Images: map[string][]byte{"photo.jpg": photo, "diagram.png": diagram},
			Sections: []string{
				`<title><p>One</p></title><p>A photograph:</p><image l:href="#photo.jpg"/>`,
				`<title><p>Two</p></title><p>A diagram:</p><image l:href="#diagram.png"/>`,
			}}.Write(t),
	}
	for name, path := range books {
		t.Run(name, func(t *testing.T) {
			s, err := Open(path, Location{Offset: -1})
			if err != nil {
				t.Fatalf("Open error: %v", err)
			}
			t.Cleanup(s.Close)
			if err := s.Layout(NewGeometry(600, 700, 0, 1, 14, SpreadSingle), 14); err != nil {
				t.Fatalf("Layout error: %v", err)
			}
			var sawPhoto, sawDiagram bool
			for page := range s.Pages() {
				if _, err := s.GoTo(page); err != nil {
					t.Fatalf("GoTo(%d) error: %v", page, err)
				}
				if _, err := s.SetStyle(Style{Pictures: mupdf.PicturesNight}); err != nil {
					t.Fatalf("SetStyle error: %v", err)
				}
				if _, _, err := s.Render(); err != nil {
					t.Fatalf("Render error: %v", err)
				}
				layer, drawn := s.ImageLayer()
				if !drawn {
					continue
				}
				area := pixels(layer, func(_, _, _, a int) bool { return a > 250 })
				sky := pixels(layer, func(r, g, b, a int) bool { return a > 250 && b > r+40 && b > 150 })
				dark := pixels(layer, func(r, g, b, a int) bool { return a > 250 && max(r, g, b) < 60 })
				switch {
				case sky > area/3:
					sawPhoto = true
				case dark > area/2:
					sawDiagram = true
				}
				boxes, err := s.doc.PageImages(page)
				if err != nil || len(boxes) == 0 {
					t.Fatalf("PageImages(%d) = %v, %v; want the picture drawn there", page, boxes, err)
				}
				left, top := s.geo.pageOrigin(0)
				b := boxes[0]
				p, ok, err := s.PictureAt(left+(b.X0+b.X1)/2, top+(b.Y0+b.Y1)/2)
				if err != nil || !ok || p.Page != page {
					t.Errorf("PictureAt the middle of page %d's picture = %+v, %v, %v", page, p, ok, err)
					continue
				}
				img, err := s.PicturePixels(p.Page, p.Index)
				if err != nil {
					t.Fatalf("PicturePixels(%d, %d) error: %v", p.Page, p.Index, err)
				}
				if got := img.Bounds().Size(); got != photoSize && got != diagramSize {
					t.Errorf("PicturePixels(%d, %d) is %v, want a picture's own size, %v or %v", p.Page, p.Index, got, photoSize, diagramSize)
				}
			}
			if !sawPhoto || !sawDiagram {
				t.Errorf("night pictures: photograph in color %v, diagram inverted %v; want both", sawPhoto, sawDiagram)
			}
			if name != "MOBI" {
				return
			}
			// The MOBI's first paragraph is red.
			if _, err := s.GoTo(0); err != nil {
				t.Fatalf("GoTo(0) error: %v", err)
			}
			if _, err := s.SetStyle(Style{BookColors: true}); err != nil {
				t.Fatalf("SetStyle error: %v", err)
			}
			if _, _, err := s.Render(); err != nil {
				t.Fatalf("Render error: %v", err)
			}
			layer, drawn := s.ColorLayer()
			if red := pixels(layer, func(r, g, b, a int) bool { return a > 0 && r > g+40 && r > b+40 }); !drawn || red == 0 {
				t.Errorf("ColorLayer of the first page: drawn %v, %d red pixels; want the red words", drawn, red)
			}
		})
	}
}

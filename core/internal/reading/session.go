// Package reading lays out a book with MuPDF and draws the visible pages into
// a coverage mask for an EDR renderer. It also locates text independently of
// the layout (Anchor), for selection, highlights and search. It has no user
// interface code.
package reading

import (
	"errors"
	"fmt"
	"math"

	"mole/core/internal/bookfile"
	"mole/core/internal/mupdf"
)

// userCSS keeps the book's own styles and only overrides what hurts on an
// EDR page: colored text and lines (dark red turns dim once inverted to
// light) and justification, because MuPDF does not hyphenate. Code keeps a
// monospaced font so that listings stay aligned. Backgrounds are left out
// when rendering instead (see mupdf.Document.RenderLayers): MuPDF lets inline
// styles win over these rules.
const userCSS = `
body, body * { font-family: serif !important; color: black !important; }
pre, pre *, code, kbd, samp, tt { font-family: monospace !important; }
p, li { line-height: 1.45 !important; text-align: left !important; }
`

// Location is where the reader is in a book, independently of the layout:
// the first character shown, or, for places saved before anchors existed, a
// chapter and how far into it.
type Location struct {
	Chapter  int
	Fraction float64 // how far into the chapter the page is, from 0 to 1
	Offset   int     // offset of the first character shown in Chapter, or -1 when unknown
}

// Chapter is a table of contents entry resolved to a page of the current layout.
type Chapter struct {
	Title string
	Level int // 0 for chapters, 1 for their sections, ...
	Page  int
}

// Session is an open book: its layout, the current page and the mask the
// visible pages are drawn into. It is not safe for concurrent use, except
// that searches run on goroutines of their own.
type Session struct {
	path     string
	doc      *mupdf.Document
	outline  []mupdf.OutlineItem
	chapters []Chapter
	start    Location // where to go on the first layout
	geo      Geometry
	fontSize float64
	pages    int
	page     int // first visible page, a multiple of the number of columns
	mask     []byte
	images   []byte // image layer of the last render, laid out like mask
	drawn    bool   // whether the last render drew any image
	stride   int

	// at is the anchor the reader is at. Only moving (Turn, GoTo and
	// GoToAnchor) changes it, and every layout shows it, so going back and
	// forth between layouts does not drift. atPage is the page that holds it.
	at       Anchor
	anchored bool // at is set; false until the first layout of a place without an offset
	atPage   int

	// starts holds, for each chapter, the offset of the first counted
	// character of each of its pages measured so far in the current layout,
	// followed by the offset after the last measured page.
	starts map[int][]int
	texts  map[int]*pageText // text of recently used pages
	recent []int             // pages in texts, oldest first
	bottom float64           // lowest line bottom seen in the current layout
	text   *bookText         // text for searching, extracted on first use
}

// Open opens the book at path; the first Layout shows the page at start.
func Open(path string, start Location) (*Session, error) {
	doc, err := bookfile.Open(path, userCSS)
	if err != nil {
		return nil, err
	}
	outline, err := doc.Outline()
	if err != nil {
		doc.Close()
		return nil, err
	}
	s := &Session{path: path, doc: doc, outline: outline, start: start}
	if start.Offset >= 0 {
		s.at, s.anchored = Anchor{Chapter: start.Chapter, Offset: start.Offset}, true
	}
	s.forgetText()
	return s, nil
}

// Close releases the book and stops extracting its text for searches.
// Searches that are still running end with the chapters they have.
func (s *Session) Close() {
	if s.text != nil {
		s.text.close()
	}
	s.doc.Close()
}

// Layout reflows the book for geo and fontSize and shows the screen that
// holds the reader's anchor, so that the same text stays in view. It does
// nothing when neither changed.
func (s *Session) Layout(geo Geometry, fontSize float64) error {
	if s.pages > 0 && geo == s.geo && fontSize == s.fontSize {
		return nil
	}
	if !geo.usable() {
		return errors.New("reading: view too small for a page")
	}
	pages, err := s.doc.Layout(geo.ColumnWidth, geo.PageHeight, fontSize)
	if err != nil {
		return err
	}
	if pages == 0 {
		return errors.New("reading: the book has no pages")
	}
	s.geo, s.fontSize, s.pages = geo, fontSize, pages
	s.forgetText()
	s.resolveChapters()
	width, height := geo.PixelSize()
	s.stride = (width + 63) &^ 63
	s.mask = make([]byte, s.stride*height)
	s.images = make([]byte, s.stride*height)
	s.drawn = false

	if s.anchored {
		if page, err := s.AnchorPage(s.at); err == nil {
			s.page, s.atPage = s.spread(page), page
			return nil
		}
		// The anchor is not in this book (it changed since the place was
		// saved): fall back to the chapter fraction.
		fraction := 0.0
		if s.start.Chapter == s.at.Chapter {
			fraction = s.start.Fraction
		}
		s.anchored = false
		s.start = Location{Chapter: s.at.Chapter, Fraction: fraction, Offset: -1}
	}
	page := 0
	if p, ok := s.doc.PageAt(s.start.Chapter, s.start.Fraction); ok {
		page = p
	}
	s.page = s.spread(page)
	return s.anchorAt(page)
}

// spread returns the first page of the screen that holds page: screens of
// two pages always start on an even page, so they pair the same pages
// wherever the reader comes from.
func (s *Session) spread(page int) int {
	page = max(0, min(page, s.pages-1))
	return page - page%s.geo.Columns
}

// anchorAt moves the reader's anchor to the start of the text of page. A
// page without text, such as a full-page figure, is anchored to the end of
// the text before it, so that a new layout shows the figure rather than the
// page after it.
func (s *Session) anchorAt(page int) error {
	t, err := s.pageText(page)
	if err != nil {
		return err
	}
	s.at = t.first()
	if t.count == 0 && t.start > 0 {
		s.at.Offset--
	}
	s.anchored, s.atPage = true, page
	return nil
}

func (s *Session) resolveChapters() {
	s.chapters = s.chapters[:0]
	for _, item := range s.outline {
		if page, ok := s.doc.Resolve(item.URI); ok {
			s.chapters = append(s.chapters, Chapter{Title: item.Title, Level: item.Level, Page: page})
		}
	}
}

// Geometry returns the current layout.
func (s *Session) Geometry() Geometry { return s.geo }

// Stride returns the row length, in bytes, of the mask and the image layer.
func (s *Session) Stride() int { return s.stride }

// Pages returns the number of pages in the current layout.
func (s *Session) Pages() int { return s.pages }

// Page returns the first visible page, counting from 0.
func (s *Session) Page() int { return s.page }

// LastVisible returns the last visible page, counting from 0.
func (s *Session) LastVisible() int {
	return min(s.page+s.geo.Columns, s.pages) - 1
}

// Location returns where the reader is, independently of the layout.
func (s *Session) Location() Location {
	if !s.anchored {
		return s.start
	}
	loc := Location{Chapter: s.at.Chapter, Offset: s.at.Offset}
	if _, fraction, ok := s.doc.ChapterPosition(s.atPage); ok {
		loc.Fraction = fraction
	}
	return loc
}

// Progress returns how far through the book the visible pages reach, from
// 0 on the first page to 1 on the last screen.
func (s *Session) Progress() float64 {
	if s.pages <= 1 {
		return 0
	}
	return float64(s.LastVisible()) / float64(s.pages-1)
}

// Turn moves by delta screens of pages and reports whether the page changed.
// It stops at the first and the last screen.
func (s *Session) Turn(delta int) (bool, error) {
	return s.GoTo(s.page + delta*s.geo.Columns)
}

// GoTo shows the screen that holds page and reports whether the page
// changed. The reader's anchor moves to the start of page.
func (s *Session) GoTo(page int) (bool, error) {
	page = max(0, min(page, s.pages-1))
	first := s.spread(page)
	if first == s.page {
		return false, nil
	}
	s.page = first
	return true, s.anchorAt(page)
}

// Chapters returns the table of contents for the current layout.
func (s *Session) Chapters() []Chapter { return s.chapters }

// ChapterTitle returns the title of the top-level chapter that holds the first
// visible page, or "" before the first one.
func (s *Session) ChapterTitle() string {
	title := ""
	for _, c := range s.chapters {
		if c.Page > s.page {
			break
		}
		if c.Level == 0 {
			title = c.Title
		}
	}
	return title
}

// Render draws the visible pages and returns the text mask with its row
// stride in bytes: one byte of ink coverage per pixel of the view, for text
// and vector paths. Images go to a second layer, ImageLayer.
func (s *Session) Render() (mask []byte, stride int, err error) {
	if s.pages == 0 {
		return nil, 0, errors.New("reading: render before layout")
	}
	clear(s.mask)
	clear(s.images)
	s.drawn = false
	g := s.geo
	width, height := g.PixelSize()
	y := int(math.Round(g.Top * g.Scale))
	for column := range g.Columns {
		page := s.page + column
		if page >= s.pages {
			break
		}
		x := int(math.Round((g.Left + float64(column)*(g.ColumnWidth+g.Gap)) * g.Scale))
		w := min(int(math.Ceil(g.ColumnWidth*g.Scale)), width-x)
		h := min(int(math.Ceil(g.PageHeight*g.Scale)), height-y)
		drawn, err := s.doc.RenderLayers(page, g.Scale, s.mask, s.images, y*s.stride+x, w, h, s.stride)
		if err != nil {
			return nil, 0, fmt.Errorf("render page %d: %w", page, err)
		}
		s.drawn = s.drawn || drawn
	}
	return s.mask, s.stride, nil
}

// ImageLayer returns the images of the last Render, laid out like its mask:
// one byte of luminance per pixel, on black. Photographs and dark pictures
// are as they are; line art is inverted so that its lines are light. It
// reports whether any image is on screen; the layer is all zero when not.
func (s *Session) ImageLayer() (layer []byte, drawn bool) {
	return s.images, s.drawn
}

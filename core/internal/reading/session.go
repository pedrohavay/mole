// Package reading lays out a book with MuPDF and draws the visible pages into
// a coverage mask, with layers for the hues of the text and for the
// pictures, for an EDR renderer. It also locates text independently of the
// layout (Anchor), for selection, highlights and search, and finds the
// pictures on screen. It has no user interface code.
package reading

import (
	"errors"
	"fmt"
	"math"

	"mole/core/internal/mupdf"
)

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

// Style is how Render draws the pages.
type Style struct {
	Pictures   mupdf.Pictures
	BookColors bool // keep the hues of colored text and lines
}

// Session is an open book: its layout, the current page and the layers the
// visible pages are drawn into. It is not safe for concurrent use, except
// that searches run on goroutines of their own.
type Session struct {
	path       string
	typography Typography
	doc        *mupdf.Document
	outline    []mupdf.OutlineItem
	chapters   []Chapter
	start      Location // where to go on the first layout
	geo        Geometry
	fontSize   float64
	pages      int
	page       int // first visible page, a multiple of the number of columns
	style      Style
	mask       []byte
	colors     []byte      // hues of the text of the last render, 4 bytes per pixel of mask; nil without BookColors
	images     []byte      // image layer of the last render, 4 bytes per pixel of mask; nil with PicturesHidden
	drawn      mupdf.Drawn // which of colors and images the last render drew into
	stride     int
	pictures   map[int][]mupdf.Rect // where the pages measured in this layout draw images

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

// Open opens the book at path in the zero Typography; the first Layout
// shows the page at start.
func Open(path string, start Location) (*Session, error) {
	doc, outline, err := openDocument(path, Typography{})
	if err != nil {
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

// openDocument opens the book at path with the user style sheet of t and
// reads its table of contents.
func openDocument(path string, t Typography) (*mupdf.Document, []mupdf.OutlineItem, error) {
	doc, err := t.open(path)
	if err != nil {
		return nil, nil, err
	}
	outline, err := doc.Outline()
	if err != nil {
		doc.Close()
		return nil, nil, err
	}
	return doc, outline, nil
}

// SetTypography sets the text in t. The book has no layout until the next
// Layout, which lays it out again showing the same text, also when the
// geometry and the font size stay the same.
func (s *Session) SetTypography(t Typography) error {
	if t.css() == s.typography.css() {
		s.typography = t
		return nil
	}
	// MuPDF keeps the chapters it parsed with the old style sheet, so the
	// book is opened again rather than given a new one.
	doc, outline, err := openDocument(s.path, t)
	if err != nil {
		return err
	}
	s.doc.Close()
	s.doc, s.outline, s.typography = doc, outline, t
	s.pages = 0
	return nil
}

// Typography returns how the text is set.
func (s *Session) Typography() Typography { return s.typography }

// Layout reflows the book for geo and fontSize and shows the screen that
// holds the reader's anchor, so that the same text stays in view. It does
// nothing when neither changed.
func (s *Session) Layout(geo Geometry, fontSize float64) error {
	if s.pages > 0 && geo == s.geo && fontSize == s.fontSize {
		return nil
	}
	return s.layout(geo, fontSize)
}

// layout reflows the book for geo and fontSize; see Layout. A view too small
// for a page leaves the layout as it was; any other failure leaves the book
// without one.
func (s *Session) layout(geo Geometry, fontSize float64) error {
	if !geo.usable() {
		return errors.New("reading: view too small for a page")
	}
	s.pages = 0
	pages, err := s.doc.Layout(geo.ColumnWidth, geo.PageHeight, fontSize)
	if err != nil {
		return err
	}
	if pages == 0 {
		return errors.New("reading: the book has no pages")
	}
	s.geo, s.fontSize, s.pages = geo, fontSize, pages
	s.forgetText()
	s.pictures = map[int][]mupdf.Rect{}
	s.resolveChapters()
	width, height := geo.PixelSize()
	s.stride = (width + 63) &^ 63
	s.mask = make([]byte, s.stride*height)
	s.colors, s.images, s.drawn = nil, nil, mupdf.Drawn{}

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

// SetStyle sets how the next Render draws the pages and reports whether it
// changed.
func (s *Session) SetStyle(style Style) (bool, error) {
	if style.Pictures < mupdf.PicturesNight || style.Pictures > mupdf.PicturesHidden {
		return false, fmt.Errorf("reading: unknown pictures %d", style.Pictures)
	}
	if style == s.style {
		return false, nil
	}
	s.style = style
	return true, nil
}

// Style returns how Render draws the pages.
func (s *Session) Style() Style { return s.style }

// Render draws the visible pages and returns the text mask with its row
// stride in bytes: one byte of ink coverage per pixel of the view, for text
// and vector paths. The hues of colored text go to ColorLayer and images to
// ImageLayer, as the style says.
func (s *Session) Render() (mask []byte, stride int, err error) {
	if s.pages == 0 {
		return nil, 0, errors.New("reading: render before layout")
	}
	clear(s.mask)
	s.colors = blankLayer(s.colors, 4*len(s.mask), s.style.BookColors, s.drawn.Colors)
	s.images = blankLayer(s.images, 4*len(s.mask), s.style.Pictures != mupdf.PicturesHidden, s.drawn.Images)
	s.drawn = mupdf.Drawn{}
	g := s.geo
	width, height := g.PixelSize()
	y := int(math.Round(g.Top * g.Scale))
	for column := range g.Columns {
		page := s.page + column
		if page >= s.pages {
			break
		}
		x := int(math.Round((g.Left + float64(column)*(g.ColumnWidth+g.Gap)) * g.Scale))
		layers := mupdf.Layers{
			Text: s.mask, Colors: s.colors, Images: s.images,
			Offset: y*s.stride + x, Stride: s.stride,
			Width:  min(int(math.Ceil(g.ColumnWidth*g.Scale)), width-x),
			Height: min(int(math.Ceil(g.PageHeight*g.Scale)), height-y),
		}
		drawn, err := s.doc.RenderLayers(page, g.Scale, layers, s.style.Pictures)
		if err != nil {
			// A page may have drawn part of itself before failing.
			s.drawn = mupdf.Drawn{Colors: true, Images: true}
			return nil, 0, fmt.Errorf("render page %d: %w", page, err)
		}
		s.drawn.Colors = s.drawn.Colors || drawn.Colors
		s.drawn.Images = s.drawn.Images || drawn.Images
	}
	return s.mask, s.stride, nil
}

// blankLayer returns an all-zero layer of size bytes when wanted, reusing
// layer if it has that size (dirty says whether something was drawn into
// it), and nil when not wanted.
func blankLayer(layer []byte, size int, wanted, dirty bool) []byte {
	switch {
	case !wanted:
		return nil
	case len(layer) != size:
		return make([]byte, size)
	case dirty:
		clear(layer)
	}
	return layer
}

// ColorLayer returns the hues of the text of the last Render, laid out like
// its mask with four bytes per pixel: RGBA in sRGB with premultiplied
// alpha, where alpha is the coverage of the text drawn in a hue. Black,
// white and gray text is left out, for the reader's tint, and dark hues are
// lightened to show on black. It reports whether any colored text is on
// screen; the layer is nil or all zero when not.
func (s *Session) ColorLayer() (layer []byte, drawn bool) {
	return s.colors, s.drawn.Colors
}

// ImageLayer returns the images of the last Render, laid out like its mask
// with four bytes per pixel: RGBA in sRGB with premultiplied alpha, which
// makes the color channels the picture on black. The style says whether
// line art is inverted. It reports whether any image is on screen; the
// layer is nil or all zero when not.
func (s *Session) ImageLayer() (layer []byte, drawn bool) {
	return s.images, s.drawn.Images
}

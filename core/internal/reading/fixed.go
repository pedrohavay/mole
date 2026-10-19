package reading

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
)

// A book of fixed pages, such as a PDF, is not reflowed: its pages keep
// their size and are scaled to the view, one or two side by side, and a
// reader who zooms in past the view scrolls within them. Each page is a
// chapter of its own (see mupdf.Document.Layout), so an anchor is a page and
// a count of characters on it, and the typography does not apply.

// Fit is how the pages of a book of fixed pages fit the view.
type Fit int

// Fits.
const (
	FitPage  Fit = iota // whole pages in view
	FitWidth            // pages as wide as the view
	FitNone             // pages at the scale of Zoom.Scale
)

// Zoom is how large the pages of a book of fixed pages are drawn. The zero
// value fits whole pages in view.
type Zoom struct {
	Fit   Fit
	Scale float64 // view points per point of the page, with FitNone
}

// Viewport is how the visible pages of a book of fixed pages sit in the
// view.
type Viewport struct {
	Zoom     Zoom
	Scale    float64 // view points per point of the page, as the pages are drawn
	FitPage  float64 // the scale at which whole pages fit the view
	FitWidth float64 // the scale at which the pages are as wide as the view
	// Width and Height are the size of the visible pages side by side, in
	// view points.
	Width, Height float64
	// X and Y are how far the view is scrolled into pages larger than it,
	// in view points, from 0 up to MaxX and MaxY. A maximum is 0 along a
	// side where the pages fit.
	X, Y       float64
	MaxX, MaxY float64
}

// Room around fixed pages, in view points: fixedPad is left between pages
// that fit the view and its edges, fixedGap between two pages side by side.
const (
	fixedPad = 12
	fixedGap = 16
)

// maxZoom is the largest scale pages are drawn at, unless whole pages need
// more to fit the view.
const maxZoom = 8

// zoomSteps are the scales StepZoom stops at besides those that fit whole
// pages or their width.
var zoomSteps = []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8}

// pageSize is the size of a fixed page in points.
type pageSize struct {
	width, height float64
}

// pageFrame is a visible page and where it is drawn: the top left corner
// in view points, rounded to whole pixels, the size in points of the page
// and the view points per point of the page it is drawn at.
type pageFrame struct {
	page          int
	x, y          float64
	width, height float64
	zoom          float64
}

// contains reports whether the point x, y of the view lies on the page,
// and where on the page, in its points.
func (f pageFrame) contains(x, y float64) (px, py float64, ok bool) {
	px, py = (x-f.x)/f.zoom, (y-f.y)/f.zoom
	return px, py, px >= 0 && px <= f.width && py >= 0 && py <= f.height
}

// FixedLayout reports whether the book has fixed pages, as a PDF does. Its
// pages are not reflowed: the font size and the typography do not apply,
// and a Zoom scales them instead.
func (s *Session) FixedLayout() bool { return s.fixed }

// frames returns the visible pages and where they are drawn.
func (s *Session) frames() ([]pageFrame, error) {
	if s.pages == 0 {
		return nil, errNoLayout
	}
	if s.fixed {
		frames, _, err := s.placeFixed()
		return frames, err
	}
	var out []pageFrame
	for column := range s.geo.Columns {
		page := s.page + column
		if page >= s.pages {
			break
		}
		x, y := s.geo.pageOrigin(column)
		out = append(out, pageFrame{page: page, x: x, y: y, width: s.geo.ColumnWidth, height: s.geo.PageHeight, zoom: 1})
	}
	return out, nil
}

// size returns the size of page in points.
func (s *Session) size(page int) (pageSize, error) {
	if size, ok := s.sizes[page]; ok {
		return size, nil
	}
	if page < 0 || page >= s.pages {
		return pageSize{}, fmt.Errorf("reading: no page %d", page)
	}
	w, h, err := s.doc.PageSize(page)
	if err != nil {
		return pageSize{}, err
	}
	if w <= 0 || h <= 0 {
		return pageSize{}, fmt.Errorf("reading: page %d has no area", page)
	}
	s.sizes[page] = pageSize{width: w, height: h}
	return s.sizes[page], nil
}

// screenSizes returns the sizes of the pages of the screen that starts at
// first.
func (s *Session) screenSizes(first int) ([]pageSize, error) {
	var sizes []pageSize
	for page := first; page < min(first+s.geo.Columns, s.pages); page++ {
		size, err := s.size(page)
		if err != nil {
			return nil, err
		}
		sizes = append(sizes, size)
	}
	return sizes, nil
}

// fixedArea returns the room for fixed pages that fit the view, in view
// points: the view less the toolbar's inset and a pad on every side.
func (g Geometry) fixedArea() (width, height float64) {
	return g.Width - 2*fixedPad, g.Height - g.Inset - 2*fixedPad
}

// fitScales returns the scales at which pages of sizes side by side fit
// the room of g: whole, and by their width.
func (g Geometry) fitScales(sizes []pageSize) (page, width float64) {
	areaWidth, areaHeight := g.fixedArea()
	sum, tallest := 0.0, 0.0
	for _, size := range sizes {
		sum += size.width
		tallest = max(tallest, size.height)
	}
	width = (areaWidth - fixedGap*float64(len(sizes)-1)) / sum
	return min(width, areaHeight/tallest), width
}

// fixedColumns returns how many fixed pages the view shows side by side.
// SpreadAuto shows two when each is nearly as large as one alone would be,
// which takes a view wider than the pages are; SpreadDouble shows two
// whenever each can still be as wide as a column. The second page stands
// for the book, since the first is often a cover of another size.
func (s *Session) fixedColumns() (int, error) {
	if s.geo.Spread == SpreadSingle {
		return 1, nil
	}
	size, err := s.size(min(1, s.pages-1))
	if err != nil {
		return 0, err
	}
	one, _ := s.geo.fitScales([]pageSize{size})
	two, _ := s.geo.fitScales([]pageSize{size, size})
	if s.geo.Spread == SpreadDouble && two*size.width >= minColumn ||
		s.geo.Spread == SpreadAuto && two >= 0.8*one {
		return 2, nil
	}
	return 1, nil
}

// zoomScale returns the scale the zoom draws pages at, given the scales
// that fit them.
func (s *Session) zoomScale(fitPage, fitWidth float64) float64 {
	switch s.zoom.Fit {
	case FitWidth:
		return fitWidth
	case FitNone:
		return min(max(s.zoom.Scale, fitPage), max(maxZoom, fitPage))
	}
	return fitPage
}

// placeFixed returns where the visible fixed pages are drawn and the
// viewport they make. Pages that fit the view along a side are centered
// along it; pages larger than the view are scrolled by the session's pan,
// held within what they allow.
func (s *Session) placeFixed() ([]pageFrame, Viewport, error) {
	sizes, err := s.screenSizes(s.page)
	if err != nil {
		return nil, Viewport{}, err
	}
	g := s.geo
	areaWidth, areaHeight := g.fixedArea()
	fitPage, fitWidth := g.fitScales(sizes)
	scale := s.zoomScale(fitPage, fitWidth)
	width, height := fixedGap*float64(len(sizes)-1), 0.0
	for _, size := range sizes {
		width += size.width * scale
		height = max(height, size.height*scale)
	}
	vp := Viewport{
		Zoom: s.zoom, Scale: scale, FitPage: fitPage, FitWidth: fitWidth, Width: width, Height: height,
		MaxX: max(0, width-areaWidth), MaxY: max(0, height-areaHeight),
	}
	vp.X, vp.Y = min(max(s.panX, 0), vp.MaxX), min(max(s.panY, 0), vp.MaxY)
	x := fixedPad + max(0, (areaWidth-width)/2) - vp.X
	top := g.Inset + fixedPad + max(0, (areaHeight-height)/2) - vp.Y
	snap := func(v float64) float64 { return math.Round(v*g.Scale) / g.Scale }
	frames := make([]pageFrame, len(sizes))
	for i, size := range sizes {
		frames[i] = pageFrame{
			page: s.page + i, x: snap(x), y: snap(top + (height-size.height*scale)/2),
			width: size.width, height: size.height, zoom: scale,
		}
		x += size.width*scale + fixedGap
	}
	return frames, vp, nil
}

// Viewport returns how the visible pages of a book of fixed pages sit in
// the view. It fails for a reflowable book and before the first layout.
func (s *Session) Viewport() (Viewport, error) {
	if !s.fixed {
		return Viewport{}, errors.New("reading: the book has no fixed pages")
	}
	if s.pages == 0 {
		return Viewport{}, errNoLayout
	}
	_, vp, err := s.placeFixed()
	return vp, err
}

// SetZoom draws the pages of a book of fixed pages at z and reports
// whether the view changed. The point at the center of the view stays
// there. Pages are never drawn smaller than they fit the view, nor larger
// than 8 times their size unless fitting them takes more. It does nothing
// for a reflowable book.
func (s *Session) SetZoom(z Zoom) (bool, error) {
	if z.Fit < FitPage || z.Fit > FitNone || z.Fit == FitNone && !(z.Scale > 0) {
		return false, fmt.Errorf("reading: no zoom %+v", z)
	}
	if !s.fixed {
		return false, nil
	}
	if s.pages == 0 {
		s.zoom = z
		return false, nil
	}
	before, err := s.Viewport()
	if err != nil {
		return false, err
	}
	if z.Fit != FitNone {
		z.Scale = 0
	} else if z.Scale <= before.FitPage {
		z = Zoom{Fit: FitPage}
	}
	s.zoom = z
	after, err := s.Viewport()
	if err != nil {
		return false, err
	}
	// Scroll to the point that was at the center of the view: center
	// returns the scroll that puts it there along a side where the view is
	// area long and the pages were size long, scrolled by scroll.
	areaWidth, areaHeight := s.geo.fixedArea()
	center := func(scroll, size, area float64) float64 {
		at := area/2 - max(0, (area-size)/2) + scroll
		return at/before.Scale*after.Scale - area/2
	}
	s.panX = center(before.X, before.Width, areaWidth)
	s.panY = center(before.Y, before.Height, areaHeight)
	after, err = s.Viewport()
	if err != nil {
		return false, err
	}
	s.panX, s.panY = after.X, after.Y
	return after != before, nil
}

// StepZoom zooms the pages of a book of fixed pages in by steps (out when
// negative) and reports whether the view changed. The steps go through
// the scales that fit whole pages and their width and fixed scales from
// half size to 8 times, never below whole pages. It does nothing for a
// reflowable book.
func (s *Session) StepZoom(steps int) (bool, error) {
	if !s.fixed || steps == 0 {
		return false, nil
	}
	vp, err := s.Viewport()
	if err != nil {
		return false, err
	}
	type stop struct {
		zoom  Zoom
		scale float64
	}
	stops := []stop{{Zoom{Fit: FitPage}, vp.FitPage}}
	if vp.FitWidth > vp.FitPage*1.02 {
		stops = append(stops, stop{Zoom{Fit: FitWidth}, vp.FitWidth})
	}
	for _, scale := range zoomSteps {
		near := func(st stop) bool { return math.Abs(st.scale-scale) <= 0.04*scale }
		if scale > vp.FitPage && scale <= max(maxZoom, vp.FitPage) && !slices.ContainsFunc(stops, near) {
			stops = append(stops, stop{Zoom{Fit: FitNone, Scale: scale}, scale})
		}
	}
	slices.SortFunc(stops, func(a, b stop) int { return cmp.Compare(a.scale, b.scale) })
	// From a scale between two stops, the first step lands on the stop
	// beyond it.
	next := slices.IndexFunc(stops, func(st stop) bool { return st.zoom == vp.Zoom })
	switch {
	case next >= 0:
		next += steps
	case steps > 0:
		next = len(stops)
		if i := slices.IndexFunc(stops, func(st stop) bool { return st.scale > vp.Scale*1.001 }); i >= 0 {
			next = i
		}
		next += steps - 1
	default:
		next = -1
		for i, st := range stops {
			if st.scale < vp.Scale*0.999 {
				next = i
			}
		}
		next += steps + 1
	}
	next = min(max(next, 0), len(stops)-1)
	if stops[next].zoom == vp.Zoom {
		return false, nil
	}
	return s.SetZoom(stops[next].zoom)
}

// Pan scrolls the view by dx, dy view points within pages of a book of
// fixed pages that are larger than it, and reports whether it moved: a
// positive dy shows what is further down. It stops at the edges of the
// pages. It does nothing for a reflowable book.
func (s *Session) Pan(dx, dy float64) (bool, error) {
	if !s.fixed {
		return false, nil
	}
	before, err := s.Viewport()
	if err != nil {
		return false, err
	}
	s.panX = min(max(before.X+dx, 0), before.MaxX)
	s.panY = min(max(before.Y+dy, 0), before.MaxY)
	return s.panX != before.X || s.panY != before.Y, nil
}

// reveal scrolls the view of a book of fixed pages so that the character at
// a, which is on a visible page, is in view, a quarter of the way down when
// the view must move. It reports whether the view moved.
func (s *Session) reveal(a Anchor) (bool, error) {
	frames, vp, err := s.placeFixed()
	if err != nil {
		return false, err
	}
	for _, f := range frames {
		if f.page != a.Chapter {
			continue
		}
		t, err := s.pageText(f.page)
		if err != nil {
			return false, err
		}
		box, ok := t.box(a.Offset)
		if !ok {
			return false, nil
		}
		x0, y0 := f.x+box.X0*f.zoom, f.y+box.Y0*f.zoom
		x1, y1 := f.x+box.X1*f.zoom, f.y+box.Y1*f.zoom
		areaWidth, areaHeight := s.geo.fixedArea()
		top, left := s.geo.Inset+fixedPad, float64(fixedPad)
		dx, dy := 0.0, 0.0
		if x0 < left || x1 > left+areaWidth {
			dx = x0 - left - areaWidth/4
		}
		if y0 < top || y1 > top+areaHeight {
			dy = y0 - top - areaHeight/4
		}
		if dx == 0 && dy == 0 {
			return false, nil
		}
		s.panX, s.panY = vp.X+dx, vp.Y+dy
		after, err := s.Viewport()
		if err != nil {
			return false, err
		}
		s.panX, s.panY = after.X, after.Y
		return after.X != vp.X || after.Y != vp.Y, nil
	}
	return false, nil
}

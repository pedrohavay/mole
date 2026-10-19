package reading

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
)

var errNoLayout = errors.New("reading: the book is not laid out")

// recentPages is how many pages keep their text between calls: the visible
// pages and a few around them.
const recentPages = 8

// forgetText drops what is known about the text of the current layout.
func (s *Session) forgetText() {
	s.starts = map[int][]int{}
	s.texts = map[int]*pageText{}
	s.recent = nil
	s.bottom = 0
}

// pageStarts measures the pages of chapter up to index n and returns the
// offset each page starts at, followed by the offset after the last page
// measured. It returns fewer than n+1 starts when the chapter is shorter.
func (s *Session) pageStarts(chapter, n int) ([]int, error) {
	starts := s.starts[chapter]
	if len(starts) == 0 {
		starts = []int{0}
	}
	defer func() { s.starts[chapter] = starts }()
	for len(starts) <= n {
		index := len(starts) - 1
		page, ok := s.doc.PageNumber(chapter, index)
		if !ok {
			break
		}
		if t, ok := s.texts[page]; ok {
			starts = append(starts, starts[index]+t.count)
			continue
		}
		chars, err := s.doc.PageChars(page)
		if err != nil {
			return nil, err
		}
		s.bottom = max(s.bottom, lowest(chars))
		starts = append(starts, starts[index]+countChars(chars))
	}
	return starts, nil
}

// pageText returns the text of page in the current layout.
func (s *Session) pageText(page int) (*pageText, error) {
	if t, ok := s.texts[page]; ok {
		return t, nil
	}
	chapter, index, ok := s.doc.Location(page)
	if !ok {
		return nil, fmt.Errorf("reading: no page %d", page)
	}
	starts, err := s.pageStarts(chapter, index)
	if err != nil {
		return nil, err
	}
	if len(starts) <= index {
		return nil, fmt.Errorf("reading: page %d is not in chapter %d", page, chapter)
	}
	chars, err := s.doc.PageChars(page)
	if err != nil {
		return nil, err
	}
	t := newPageText(chapter, starts[index], chars)
	s.bottom = max(s.bottom, lowest(chars))
	if len(starts) == index+1 {
		s.starts[chapter] = append(starts, t.start+t.count)
	}
	if len(s.recent) == recentPages {
		delete(s.texts, s.recent[0])
		s.recent = s.recent[1:]
	}
	s.texts[page] = t
	s.recent = append(s.recent, page)
	return t, nil
}

// AnchorPage returns the page of the current layout that holds the character
// at a. An offset past the end of its chapter maps to the chapter's last page.
func (s *Session) AnchorPage(a Anchor) (int, error) {
	if s.pages == 0 {
		return 0, errNoLayout
	}
	if s.fixed {
		// Each page is a chapter.
		if a.Chapter < 0 || a.Chapter >= s.pages || a.Offset < 0 {
			return 0, fmt.Errorf("reading: no offset %d in chapter %d", a.Offset, a.Chapter)
		}
		return a.Chapter, nil
	}
	pages, ok := s.doc.ChapterPages(a.Chapter)
	if !ok || pages <= 0 || a.Offset < 0 {
		return 0, fmt.Errorf("reading: no offset %d in chapter %d", a.Offset, a.Chapter)
	}
	starts, err := s.pageStarts(a.Chapter, 1)
	for err == nil && len(starts) <= pages && starts[len(starts)-1] <= a.Offset {
		n := len(starts)
		if starts, err = s.pageStarts(a.Chapter, n); err == nil && len(starts) == n {
			break // MuPDF has no more pages for the chapter
		}
	}
	if err != nil {
		return 0, err
	}
	// The page that holds the character is the last one starting at or
	// before it; pages without text start where the next page does.
	index := sort.Search(len(starts), func(i int) bool { return starts[i] > a.Offset }) - 1
	index = max(0, min(index, pages-1))
	page, ok := s.doc.PageNumber(a.Chapter, index)
	if !ok {
		return 0, fmt.Errorf("reading: no page %d in chapter %d", index, a.Chapter)
	}
	return page, nil
}

// GoToAnchor shows the screen that holds the character at a and makes a the
// reader's anchor. It reports whether the view changed; it does not when a
// is already in view. Fixed pages larger than the view scroll to show it.
func (s *Session) GoToAnchor(a Anchor) (bool, error) {
	page, err := s.AnchorPage(a)
	if err != nil {
		return false, err
	}
	s.at, s.anchored, s.atPage = a, true, page
	first := s.spread(page)
	changed := first != s.page
	if changed {
		s.page, s.panY = first, 0
	}
	if s.fixed {
		moved, err := s.reveal(a)
		return changed || moved, err
	}
	return changed, nil
}

// VisibleRange returns the text of the visible pages.
func (s *Session) VisibleRange() (Range, error) {
	if s.pages == 0 {
		return Range{}, errNoLayout
	}
	first, err := s.pageText(s.page)
	if err != nil {
		return Range{}, err
	}
	last, err := s.pageText(s.LastVisible())
	if err != nil {
		return Range{}, err
	}
	return Range{Start: first.first(), End: last.end()}, nil
}

// VisibleCharacters returns the number of counted characters on the
// visible pages.
func (s *Session) VisibleCharacters() (int, error) {
	pages, err := s.visiblePages()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pages {
		n += p.text.count
	}
	return n, nil
}

// visiblePage is a visible page and where it is drawn in the view.
type visiblePage struct {
	text *pageText
	pageFrame
}

func (s *Session) visiblePages() ([]visiblePage, error) {
	frames, err := s.frames()
	if err != nil {
		return nil, err
	}
	out := make([]visiblePage, 0, len(frames))
	for _, f := range frames {
		t, err := s.pageText(f.page)
		if err != nil {
			return nil, err
		}
		out = append(out, visiblePage{text: t, pageFrame: f})
	}
	return out, nil
}

// HitTest returns the anchor of the character at the point x, y of the view,
// or of the character nearest to it on the same line. It reports false when
// the point is not over a line of text.
func (s *Session) HitTest(x, y float64) (Anchor, bool, error) {
	pages, err := s.visiblePages()
	if err != nil {
		return Anchor{}, false, err
	}
	for _, p := range pages {
		px, py, ok := p.contains(x, y)
		if !ok {
			continue
		}
		i, ok := p.text.hit(px, py)
		if !ok {
			return Anchor{}, false, nil
		}
		return p.text.anchor(i), true, nil
	}
	return Anchor{}, false, nil
}

// hit returns the counted character under the point x, y of the page, or the
// one nearest to it on the line under the point.
func (t *pageText) hit(x, y float64) (int, bool) {
	// under returns how far y is from the middle of line l, and false when
	// y is not over it. Lines of a paragraph touch; a little slack covers
	// the gaps around smaller text and between paragraphs.
	under := func(l textLine) (float64, bool) {
		pad := (l.box.Y1 - l.box.Y0) / 4
		return math.Abs(y - (l.box.Y0+l.box.Y1)/2), y >= l.box.Y0-pad && y <= l.box.Y1+pad
	}
	nearest := math.Inf(1)
	for _, l := range t.lines {
		if d, ok := under(l); ok {
			nearest = min(nearest, d)
		}
	}
	// MuPDF cuts a justified line into pieces at its widest spaces; of the
	// pieces at the nearest height, the one closest to x wins.
	line, best := -1, math.Inf(1)
	for i, l := range t.lines {
		if d, ok := under(l); !ok || d > nearest+0.5 {
			continue
		}
		if d := max(l.box.X0-x, x-l.box.X1, 0); d < best {
			line, best = i, d
		}
	}
	if line < 0 {
		return 0, false
	}
	hit, best := -1, math.Inf(1)
	for i := t.lines[line].first; i < t.lines[line].end; i++ {
		c := t.chars[i]
		if !counted(c.Rune) {
			continue
		}
		d := max(c.Box.X0-x, x-c.Box.X1, 0)
		if d < best {
			hit, best = i, d
		}
	}
	return hit, hit >= 0
}

// RangeRects returns the boxes of the visible part of r in view points, one
// per line it covers. Lines of fixed pages scrolled out of the view are
// left out.
func (s *Session) RangeRects(r Range) ([]Rect, error) {
	pages, err := s.visiblePages()
	if err != nil {
		return nil, err
	}
	var rects []Rect
	for _, p := range pages {
		t := p.text
		if r.End.Compare(t.first()) <= 0 || r.Start.Compare(t.end()) >= 0 {
			continue
		}
		first := len(rects) // the first box of this page
		for _, l := range t.lines {
			x0, x1 := math.Inf(1), math.Inf(-1)
			for i := l.first; i < l.end; i++ {
				if t.inside(i, r) {
					x0, x1 = min(x0, t.chars[i].Box.X0), max(x1, t.chars[i].Box.X1)
				}
			}
			if x0 > x1 {
				continue
			}
			z := p.zoom
			box := Rect{X: p.x + x0*z, Y: p.y + l.box.Y0*z, Width: (x1 - x0) * z, Height: (l.box.Y1 - l.box.Y0) * z}
			// The pieces MuPDF cuts a justified line into make one box.
			if n := len(rects); n > first && sameLine(rects[n-1], box) {
				last := &rects[n-1]
				right := max(last.X+last.Width, box.X+box.Width)
				last.X = min(last.X, box.X)
				last.Width = right - last.X
				continue
			}
			rects = append(rects, box)
		}
	}
	return slices.DeleteFunc(rects, func(b Rect) bool {
		return b.X+b.Width < 0 || b.X > s.geo.Width || b.Y+b.Height < 0 || b.Y > s.geo.Height
	}), nil
}

// sameLine reports whether two boxes of a page are on the same line of text.
func sameLine(a, b Rect) bool {
	return math.Abs(a.Y-b.Y) < 0.5 && math.Abs(a.Height-b.Height) < 0.5
}

// RangeText returns the text of r, on screen or not, with whitespace
// collapsed to single spaces and a newline between paragraphs.
func (s *Session) RangeText(r Range) (string, error) {
	if r.End.Compare(r.Start) <= 0 {
		return "", nil
	}
	page, err := s.AnchorPage(r.Start)
	if err != nil {
		return "", err
	}
	b := newTextBuilder()
	var prev *pageText
	for ; page < s.pages; page++ {
		t, err := s.pageText(page)
		if err != nil {
			return "", err
		}
		if r.End.Compare(t.first()) <= 0 {
			break
		}
		if prev != nil {
			b.page(continues(prev, t, s.bottom))
		}
		for i, c := range t.chars {
			if t.inside(i, r) {
				b.add(c)
			}
		}
		prev = t
	}
	return b.String(), nil
}

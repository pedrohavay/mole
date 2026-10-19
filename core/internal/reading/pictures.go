package reading

import (
	"fmt"
	"image"

	"mole/core/internal/mupdf"
)

// Picture is an image on the visible pages.
type Picture struct {
	Page  int  // the page that draws it
	Index int  // its place among the images of the page, in drawing order
	Rect  Rect // where it is on screen, in view points
}

// minPicture is the shortest side, in points, of an image that counts as a
// picture. Smaller ones are icons and ornaments in the text.
const minPicture = 16

// MaxPictureSide is the longest side, in pixels, of a picture drawn by
// PicturePixels.
const MaxPictureSide = 4096

// PictureAt returns the picture under the point x, y of the view, the one
// drawn last where pictures overlap. It reports false when there is none.
func (s *Session) PictureAt(x, y float64) (Picture, bool, error) {
	if s.pages == 0 {
		return Picture{}, false, errNoLayout
	}
	for column := range s.geo.Columns {
		page := s.page + column
		if page >= s.pages {
			break
		}
		left, top := s.geo.pageOrigin(column)
		px, py := x-left, y-top
		if px < 0 || px > s.geo.ColumnWidth || py < 0 || py > s.geo.PageHeight {
			continue
		}
		boxes, err := s.pageImages(page)
		if err != nil {
			return Picture{}, false, err
		}
		for i := len(boxes) - 1; i >= 0; i-- {
			// MuPDF clips what spills off the page.
			b := boxes[i]
			b.X0, b.Y0 = max(b.X0, 0), max(b.Y0, 0)
			b.X1, b.Y1 = min(b.X1, s.geo.ColumnWidth), min(b.Y1, s.geo.PageHeight)
			if b.X1-b.X0 < minPicture || b.Y1-b.Y0 < minPicture {
				continue
			}
			if px >= b.X0 && px <= b.X1 && py >= b.Y0 && py <= b.Y1 {
				rect := Rect{X: left + b.X0, Y: top + b.Y0, Width: b.X1 - b.X0, Height: b.Y1 - b.Y0}
				return Picture{Page: page, Index: i, Rect: rect}, true, nil
			}
		}
		return Picture{}, false, nil
	}
	return Picture{}, false, nil
}

// pageImages returns where page draws its images in the current layout.
func (s *Session) pageImages(page int) ([]mupdf.Rect, error) {
	if boxes, ok := s.pictures[page]; ok {
		return boxes, nil
	}
	boxes, err := s.doc.PageImages(page)
	if err != nil {
		return nil, err
	}
	s.pictures[page] = boxes
	return boxes, nil
}

// PicturePixels draws the picture at index on page, as PictureAt reports
// it, as it looks on white paper: in its colors, at its own resolution (a
// drawing that scales at MaxPictureSide pixels on its longer side) and at
// most MaxPictureSide pixels on its longer side.
func (s *Session) PicturePixels(page, index int) (*image.RGBA, error) {
	if s.pages == 0 {
		return nil, errNoLayout
	}
	if page < 0 || page >= s.pages {
		return nil, fmt.Errorf("reading: no page %d", page)
	}
	return s.doc.ImagePixels(page, index, MaxPictureSide)
}

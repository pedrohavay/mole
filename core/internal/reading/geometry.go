package reading

import "math"

// Geometry is the page layout for one view, in points unless noted.
type Geometry struct {
	Width, Height float64
	Scale         float64 // pixels per point
	Columns       int
	ColumnWidth   float64
	Gap           float64
	Left          float64 // x of the first column
	Top           float64 // y of the top of the pages, from the top of the view
	PageHeight    float64
}

// Spread is how many pages the reader shows side by side.
type Spread int

// Spreads.
const (
	SpreadAuto   Spread = iota // two pages when both fit at a comfortable measure
	SpreadSingle               // always one page
	SpreadDouble               // two pages whenever each can still hold a page
)

// NewGeometry lays out pages in a view of width x height points whose top
// inset points are covered by the toolbar, with as many pages side by side as
// spread asks for and the view allows at fontSize.
func NewGeometry(width, height, topInset, scale, fontSize float64, spread Spread) Geometry {
	marginX := math.Max(40, width*0.05)
	top := topInset + math.Max(20, (height-topInset)*0.04)
	bottom := math.Max(36, height*0.06)
	gap := 3.5 * fontSize
	maxColumn := 32 * fontSize
	available := width - 2*marginX
	columns := 1
	switch spread {
	case SpreadAuto:
		if available >= 2*0.8*maxColumn+gap {
			columns = 2
		}
	case SpreadDouble:
		if (available-gap)/2 >= minColumn {
			columns = 2
		}
	}
	columnWidth := math.Min(maxColumn, (available-gap*float64(columns-1))/float64(columns))
	total := float64(columns)*columnWidth + gap*float64(columns-1)
	return Geometry{
		Width: width, Height: height, Scale: scale,
		Columns: columns, ColumnWidth: columnWidth, Gap: gap,
		Left: (width - total) / 2, Top: top, PageHeight: height - top - bottom,
	}
}

// PixelSize returns the size of the view in pixels.
func (g Geometry) PixelSize() (width, height int) {
	return int(math.Round(g.Width * g.Scale)), int(math.Round(g.Height * g.Scale))
}

// pageOrigin returns the top left corner of the page in column of the view,
// in view points rounded to whole pixels as Render draws it.
func (g Geometry) pageOrigin(column int) (x, y float64) {
	x = g.Left + float64(column)*(g.ColumnWidth+g.Gap)
	y = g.Top
	if g.Scale > 0 {
		x = math.Round(x*g.Scale) / g.Scale
		y = math.Round(y*g.Scale) / g.Scale
	}
	return x, y
}

// minColumn is the narrowest column, in points, that still holds a page.
const minColumn = 120

// usable reports whether the view is large enough to hold a page.
func (g Geometry) usable() bool {
	return g.ColumnWidth >= minColumn && g.PageHeight >= 120 && g.Scale > 0
}

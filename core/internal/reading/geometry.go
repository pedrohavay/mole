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
	// Inset is the top of the view the toolbar covers, and Spread how many
	// pages side by side were asked for, which a book of fixed pages lays
	// its pages out by instead of the columns.
	Inset  float64
	Spread Spread
}

// Spread is how many pages the reader shows side by side.
type Spread int

// Spreads.
const (
	SpreadAuto   Spread = iota // two pages when both fit at a comfortable measure
	SpreadSingle               // always one page
	SpreadDouble               // two pages whenever each can still hold a page
)

// Margins is how much room the reader leaves around the pages.
type Margins int

// Margins.
const (
	MarginsNormal Margins = iota
	MarginsNarrow
	MarginsWide
)

// marginSizes are the sizes behind a Margins setting: the least margin of
// each side, in points and as a share of the view, and the widest page.
type marginSizes struct {
	side, sideShare     float64
	top, topShare       float64
	bottom, bottomShare float64
	measure             float64 // widest page, in ems
}

func (m Margins) sizes() marginSizes {
	switch m {
	case MarginsNarrow:
		return marginSizes{side: 24, sideShare: 0.025, top: 12, topShare: 0.025, bottom: 24, bottomShare: 0.04, measure: 36}
	case MarginsWide:
		return marginSizes{side: 64, sideShare: 0.09, top: 32, topShare: 0.07, bottom: 52, bottomShare: 0.09, measure: 28}
	default:
		return marginSizes{side: 40, sideShare: 0.05, top: 20, topShare: 0.04, bottom: 36, bottomShare: 0.06, measure: 32}
	}
}

// comfortableMeasure is the page width, in ems, that SpreadAuto wants room
// for, at 80%, before it shows two pages. It does not change with the
// margins, which only take room away.
const comfortableMeasure = 32

// NewGeometry lays out pages in a view of width x height points whose top
// inset points are covered by the toolbar, with as many pages side by side as
// spread asks for and the view allows at fontSize, and margins around them.
// On a view wider than the pages need, the margins also bound how wide a
// page grows.
func NewGeometry(width, height, topInset, scale, fontSize float64, spread Spread, margins Margins) Geometry {
	m := margins.sizes()
	marginX := math.Max(m.side, width*m.sideShare)
	top := topInset + math.Max(m.top, (height-topInset)*m.topShare)
	bottom := math.Max(m.bottom, height*m.bottomShare)
	gap := 3.5 * fontSize
	maxColumn := m.measure * fontSize
	available := width - 2*marginX
	columns := 1
	switch spread {
	case SpreadAuto:
		if available >= 2*0.8*comfortableMeasure*fontSize+gap {
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
		Inset: topInset, Spread: spread,
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

// usable reports whether the view is large enough to hold a page, a fixed
// page when fixed is set.
func (g Geometry) usable(fixed bool) bool {
	if fixed {
		width, height := g.fixedArea()
		return width >= minColumn && height >= 120 && g.Scale > 0
	}
	return g.ColumnWidth >= minColumn && g.PageHeight >= 120 && g.Scale > 0
}

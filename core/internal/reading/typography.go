package reading

import (
	"fmt"
	"strings"

	"mole/core/internal/mupdf"
)

// Typography is how the reader sets a book's text over the book's own
// styles. The zero value keeps the book's fonts, spaces lines normally and
// aligns paragraphs to the left.
type Typography struct {
	// Font is the family of one of Fonts, or "" for the book's own fonts,
	// embedded ones included. Any other family keeps the book's fonts too.
	Font        string
	LineSpacing LineSpacing
	Justify     bool // justify paragraphs instead of aligning them to the left
}

// LineSpacing is how far apart the lines of a paragraph are.
type LineSpacing int

// Line spacings.
const (
	LineSpacingNormal LineSpacing = iota
	LineSpacingCompact
	LineSpacingRelaxed
)

// height returns the CSS line height of l, in ems.
func (l LineSpacing) height() float64 {
	switch l {
	case LineSpacingCompact:
		return 1.25
	case LineSpacingRelaxed:
		return 1.7
	default:
		return 1.45
	}
}

// lineDistance returns how far apart, in ems, the baselines of two lines of
// one paragraph may be in l, for mupdf.Document.SetLineDistance: a little
// more than the line height when it passes the 1.5 em MuPDF allows, and 0,
// MuPDF's own, otherwise.
func (l LineSpacing) lineDistance() float64 {
	if h := l.height(); h > 1.5 {
		return h + 0.1
	}
	return 0
}

// open opens the book at path with the style sheet of t.
func (t Typography) open(path string) (*mupdf.Document, error) {
	doc, err := mupdf.Open(path, t.css())
	if err != nil {
		return nil, err
	}
	doc.SetLineDistance(t.LineSpacing.lineDistance())
	return doc, nil
}

// offeredFont is a font the reader offers and the generic family that stands
// in for it should it not be installed after all.
type offeredFont struct {
	family, generic string
}

// offeredFonts are the fonts the reader offers, in the order to show them.
// They come with macOS, but Fonts lists only those installed. New York is
// left out: it is a variable font whose default instance is its display cut
// (an optical size of 256), and MuPDF 1.26 builds FreeType without the
// support for variations that would pick its text cut.
var offeredFonts = []offeredFont{
	{family: "Iowan Old Style", generic: "serif"},
	{family: "Charter", generic: "serif"},
	{family: "Georgia", generic: "serif"},
	{family: "Palatino", generic: "serif"},
	{family: "Baskerville", generic: "serif"},
	{family: "Avenir Next", generic: "sans-serif"},
	{family: "Helvetica Neue", generic: "sans-serif"},
}

// Fonts returns the families of the installed fonts the reader offers for
// Typography.Font, in the order to show them.
func Fonts() []string {
	var out []string
	for _, f := range offeredFonts {
		if _, _, ok := mupdf.FindFont(f.family, mupdf.FontStyle{}); ok {
			out = append(out, f.family)
		}
	}
	return out
}

func offered(family string) (offeredFont, bool) {
	for _, f := range offeredFonts {
		if f.family == family {
			return f, true
		}
	}
	return offeredFont{}, false
}

// css returns the user style sheet that sets the text in t. It keeps the
// book's own styles and only overrides what the reader chose. Code keeps a
// monospaced font whatever the font, so that listings stay aligned. MuPDF
// does not hyphenate (it breaks words only at the soft hyphens a book has),
// so justified text can space words widely. Colors and backgrounds are
// handled when rendering instead (see mupdf.Document.RenderLayers): MuPDF
// lets inline styles win over these rules.
func (t Typography) css() string {
	var b strings.Builder
	if f, ok := offered(t.Font); ok {
		fmt.Fprintf(&b, "body, body * { font-family: \"%s\", %s !important; }\n", f.family, f.generic)
	}
	// Later than the font rule, so that code wins over it at equal specificity.
	b.WriteString("pre, pre *, code, kbd, samp, tt { font-family: monospace !important; }\n")
	align := "left"
	if t.Justify {
		align = "justify"
	}
	fmt.Fprintf(&b, "p, li { line-height: %.2f !important; text-align: %s !important; }\n", t.LineSpacing.height(), align)
	return b.String()
}

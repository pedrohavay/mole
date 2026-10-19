package reading

import (
	"cmp"
	"unicode"

	"mole/core/internal/mupdf"
)

// Anchor is a place in the text of a book that does not depend on the
// layout: a chapter (a document of the EPUB spine) and the number of counted
// characters before the place in that chapter.
//
// Characters are those of MuPDF's structured text with ligatures expanded.
// Whitespace, invisible format characters and hyphens are not counted,
// because reflowing the text adds and removes them: spaces vanish at line
// ends, and a soft hyphen is drawn as a plain hyphen only where it breaks a
// line. Every other character takes one offset, so an anchor names the same
// character at any font size, page size or spread, and after reopening.
type Anchor struct {
	Chapter int
	Offset  int
}

// Compare returns -1, 0 or +1 as a is before, at or after b in reading order.
func (a Anchor) Compare(b Anchor) int {
	if c := cmp.Compare(a.Chapter, b.Chapter); c != 0 {
		return c
	}
	return cmp.Compare(a.Offset, b.Offset)
}

// Range is the text from Start up to, but not including, End.
type Range struct {
	Start, End Anchor
}

// Rect is a rectangle in view points, with the origin at the top left of the
// view the book is laid out for.
type Rect struct {
	X, Y, Width, Height float64
}

// counted reports whether r takes up an offset in a chapter.
func counted(r rune) bool {
	return !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r) && !isHyphen(r)
}

// isHyphen reports whether r is a hyphen. MuPDF breaks lines after hyphens
// and draws a soft hyphen as U+002D where it breaks a line.
func isHyphen(r rune) bool {
	switch r {
	case '-', '\u00AD', '\u2010', '\u2011':
		return true
	}
	return false
}

// joinsLines reports whether a line break between the characters before and
// after it is not a word break: a line that ends in a hyphen or an em dash,
// or a line that starts with an em dash, continues the word before it.
func joinsLines(before, after rune) bool {
	return isHyphen(before) || before == '\u2014' || after == '\u2014'
}

// pageText is the text of one page of a layout, with the offset of every
// character.
type pageText struct {
	chapter int
	start   int // offset of the first counted character on the page
	count   int // number of counted characters on the page
	chars   []mupdf.Char
	next    []int // for each character, the offset of the first counted character at or after it
	lines   []textLine
}

// textLine is a line of a page: chars[first:end] and the box around them.
type textLine struct {
	first, end int
	box        mupdf.Rect
}

func newPageText(chapter, start int, chars []mupdf.Char) *pageText {
	t := &pageText{chapter: chapter, start: start, chars: chars, next: make([]int, len(chars))}
	for _, c := range chars {
		if counted(c.Rune) {
			t.count++
		}
	}
	offset := start + t.count
	for i := len(chars) - 1; i >= 0; i-- {
		if counted(chars[i].Rune) {
			offset--
		}
		t.next[i] = offset
	}
	for i, c := range chars {
		if i == 0 || c.Line != chars[i-1].Line {
			t.lines = append(t.lines, textLine{first: i, end: i, box: c.Box})
		}
		line := &t.lines[len(t.lines)-1]
		line.end = i + 1
		line.box = union(line.box, c.Box)
	}
	return t
}

// countChars returns the number of counted characters in chars.
func countChars(chars []mupdf.Char) int {
	n := 0
	for _, c := range chars {
		if counted(c.Rune) {
			n++
		}
	}
	return n
}

// anchor returns the anchor of character i, or of the first counted
// character after it when it is not counted itself.
func (t *pageText) anchor(i int) Anchor {
	return Anchor{Chapter: t.chapter, Offset: t.next[i]}
}

// inside reports whether character i belongs to r. A character that is not
// counted belongs to r when counted characters of r surround it.
func (t *pageText) inside(i int, r Range) bool {
	a := t.anchor(i)
	if counted(t.chars[i].Rune) {
		return r.Start.Compare(a) <= 0 && a.Compare(r.End) < 0
	}
	return r.Start.Compare(a) < 0 && a.Compare(r.End) < 0
}

// first returns the anchor of the first counted character on the page, or
// the place after the page's text when it has none.
func (t *pageText) first() Anchor {
	return Anchor{Chapter: t.chapter, Offset: t.start}
}

// end returns the place after the last counted character on the page.
func (t *pageText) end() Anchor {
	return Anchor{Chapter: t.chapter, Offset: t.start + t.count}
}

// continues reports whether the last paragraph of prev goes on at the top
// of next, the page after it. MuPDF does not say, so it is guessed: the
// paragraph must reach bottom, the lowest line bottom seen in the layout, and
// then either the next page starts in lower case, the paragraph does not end
// a sentence, or its last line is full.
func continues(prev, next *pageText, bottom float64) bool {
	if len(prev.lines) == 0 || len(next.chars) == 0 || prev.chapter != next.chapter {
		return false
	}
	last := prev.lines[len(prev.lines)-1]
	pitch := last.box.Y1 - last.box.Y0
	if bottom-last.box.Y1 > pitch {
		return false // there was room for another line
	}
	if unicode.IsLower(firstRune(next.chars)) {
		return true
	}
	if !endsSentence(lastRune(prev.chars[last.first:last.end])) {
		return true
	}
	right := last.box.X1
	for i := len(prev.lines) - 2; i >= 0 && prev.chars[prev.lines[i].first].Block == prev.chars[last.first].Block; i-- {
		right = max(right, prev.lines[i].box.X1)
	}
	return last.box.X1 >= right-2*pitch
}

// lowest returns the bottom of the lowest character in chars, or 0.
func lowest(chars []mupdf.Char) float64 {
	y := 0.0
	for _, c := range chars {
		y = max(y, c.Box.Y1)
	}
	return y
}

func firstRune(chars []mupdf.Char) rune {
	for _, c := range chars {
		if counted(c.Rune) {
			return c.Rune
		}
	}
	return 0
}

func lastRune(chars []mupdf.Char) rune {
	for i := len(chars) - 1; i >= 0; i-- {
		if !unicode.IsSpace(chars[i].Rune) {
			return chars[i].Rune
		}
	}
	return 0
}

func endsSentence(r rune) bool {
	switch r {
	case '.', '!', '?', ':', ';', '"', '\'', '\u201D', '\u2019', '\u00BB', ')', '\u2026':
		return true
	}
	return false
}

func union(a, b mupdf.Rect) mupdf.Rect {
	return mupdf.Rect{X0: min(a.X0, b.X0), Y0: min(a.Y0, b.Y0), X1: max(a.X1, b.X1), Y1: max(a.Y1, b.Y1)}
}

// textBuilder assembles normalized text: whitespace between characters
// becomes one space and a paragraph break one newline, with nothing before
// the first character or after the last.
type textBuilder struct {
	runes   []rune
	pending rune // separator to write before the next character: 0, ' ' or '\n'
	line    int  // line and block of the last character seen, -1 before any
	block   int
	last    rune // last character written
}

func newTextBuilder() *textBuilder {
	return &textBuilder{line: -1, block: -1}
}

// separate asks for a separator before the next character; a paragraph
// break wins over a space.
func (b *textBuilder) separate(sep rune) {
	if b.pending != '\n' {
		b.pending = sep
	}
}

// page starts a new page: the next character starts its first line, after a
// space when the paragraph goes on and a newline otherwise.
func (b *textBuilder) page(continued bool) {
	b.line, b.block = -1, -1
	if continued {
		b.separate(' ')
	} else {
		b.separate('\n')
	}
}

// add writes character c of a page.
func (b *textBuilder) add(c mupdf.Char) {
	switch {
	case b.block >= 0 && c.Block != b.block:
		b.separate('\n')
	case b.line >= 0 && c.Line != b.line && !joinsLines(b.last, c.Rune):
		b.separate(' ')
	}
	b.line, b.block = c.Line, c.Block
	switch {
	case unicode.IsSpace(c.Rune):
		b.separate(' ')
	case unicode.Is(unicode.Cf, c.Rune):
	default:
		if b.pending != 0 && len(b.runes) > 0 {
			b.runes = append(b.runes, b.pending)
		}
		b.pending = 0
		b.runes = append(b.runes, c.Rune)
		b.last = c.Rune
	}
}

func (b *textBuilder) String() string {
	return string(b.runes)
}

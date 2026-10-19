package reading

// Paragraph is a paragraph of text, or the part of one that lies on a page.
type Paragraph struct {
	// Range runs from the paragraph's first counted character to the place
	// after its last one.
	Range Range
	// Text is the text of Range, with whitespace collapsed to single spaces.
	Text string
	// Continued is set when the paragraph goes on from the page before, so
	// that it is the rest of the Paragraph before it.
	Continued bool
}

// Paragraphs returns the text of r a paragraph at a time, in reading order,
// on screen or not. A paragraph that runs over a page break comes in two
// parts, the second one Continued, so each part lies on one page. When
// limit is positive, it stops at the end of the paragraph whose characters
// bring the count returned to limit, its parts on later pages included.
// Paragraphs without counted characters, such as a row of dashes, are left
// out.
func (s *Session) Paragraphs(r Range, limit int) ([]Paragraph, error) {
	if r.End.Compare(r.Start) <= 0 {
		return nil, nil
	}
	page, err := s.AnchorPage(r.Start)
	if err != nil {
		return nil, err
	}
	var (
		out   []Paragraph
		count int
		prev  *pageText
	)
	for ; page < s.pages; page++ {
		t, err := s.pageText(page)
		if err != nil {
			return nil, err
		}
		if r.End.Compare(t.first()) <= 0 {
			break
		}
		continued := prev != nil && continues(prev, t, s.bottom)
		for i, p := range t.paragraphs(r) {
			p.Continued = i == 0 && continued && p.Range.Start == t.first()
			if limit > 0 && count >= limit && !p.Continued {
				return out, nil
			}
			out = append(out, p)
			count += p.Range.End.Offset - p.Range.Start.Offset
		}
		prev = t
	}
	return out, nil
}

// paragraphs returns the parts of the paragraphs of the page that lie in r.
// MuPDF puts each paragraph of a page in a block of its own.
func (t *pageText) paragraphs(r Range) []Paragraph {
	var out []Paragraph
	for first := 0; first < len(t.chars); {
		end := first + 1
		for end < len(t.chars) && t.chars[end].Block == t.chars[first].Block {
			end++
		}
		if p, ok := t.paragraph(first, end, r); ok {
			out = append(out, p)
		}
		first = end
	}
	return out
}

// paragraph returns the part of chars[first:end], one block, that lies in r,
// and reports false when it holds no counted character.
func (t *pageText) paragraph(first, end int, r Range) (Paragraph, bool) {
	b := newTextBuilder()
	start, last := -1, -1
	for i := first; i < end; i++ {
		if !t.inside(i, r) {
			continue
		}
		b.add(t.chars[i])
		if counted(t.chars[i].Rune) {
			if start < 0 {
				start = i
			}
			last = i
		}
	}
	if start < 0 {
		return Paragraph{}, false
	}
	after := t.anchor(last)
	after.Offset++
	return Paragraph{Range: Range{Start: t.anchor(start), End: after}, Text: b.String()}, true
}

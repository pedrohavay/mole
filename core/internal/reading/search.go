package reading

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"mole/core/internal/bookfile"
	"mole/core/internal/mupdf"
)

// MaxHits is the number of hits after which a search stops.
const MaxHits = 500

// excerptLength is about how many characters of text a hit shows.
const excerptLength = 80

// The copy of a book that searches read is laid out once at this page size
// and font size, in points. Anchors do not depend on the layout, and tall
// pages make for few page breaks.
const (
	searchWidth    = 600
	searchHeight   = 2400
	searchFontSize = 12
)

// Hit is a place where a search query occurs.
type Hit struct {
	Range Range
	// Excerpt is the text around the match, on one line. The match is
	// Excerpt[MatchStart:MatchStart+MatchLength], in bytes.
	Excerpt     string
	MatchStart  int
	MatchLength int
}

// Results are the hits a search has found so far, in reading order.
type Results struct {
	Hits      []Hit
	Done      bool // the search is over
	Truncated bool // the search stopped after MaxHits hits
}

// Search finds the places where a query occurs in a book, chapter by
// chapter, on a goroutine of its own. Letters match regardless of case and
// accents, and any whitespace in the query matches any whitespace in the
// text, line and page breaks included. Its methods are safe for concurrent
// use.
type Search struct {
	query  []rune // folded
	stop   chan struct{}
	once   sync.Once
	exited chan struct{}

	mu        sync.Mutex
	hits      []Hit
	done      bool
	truncated bool
	err       error
}

// Search starts looking for query in the book and returns at once. The
// first search of a session also starts extracting the text of the book,
// which later searches reuse.
func (s *Session) Search(query string) (*Search, error) {
	folded := foldQuery(query)
	if len(folded) == 0 {
		return nil, errors.New("reading: nothing to search for")
	}
	if s.text == nil {
		s.text = startBookText(s.path)
	}
	sr := &Search{query: folded, stop: make(chan struct{}), exited: make(chan struct{})}
	go sr.run(s.text)
	return sr, nil
}

// Results returns a copy of what the search has found so far. The error is
// the one that ended the search early, if any.
func (sr *Search) Results() (Results, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return Results{Hits: slices.Clone(sr.hits), Done: sr.done, Truncated: sr.truncated}, sr.err
}

// Close stops the search if it is still running and waits for it to end,
// which takes at most the time to search one chapter. Results stay
// available.
func (sr *Search) Close() {
	sr.once.Do(func() { close(sr.stop) })
	<-sr.exited
}

func (sr *Search) run(text *bookText) {
	defer close(sr.exited)
	folds := map[rune][]rune{}
	for chapter := 0; ; chapter++ {
		runes, ok, err := text.chapter(chapter, sr.stop)
		if !ok {
			sr.finish(err)
			return
		}
		if !sr.searchChapter(chapter, runes, folds) {
			sr.finish(nil)
			return
		}
	}
}

func (sr *Search) finish(err error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.done, sr.err = true, err
}

// searchChapter adds the hits in the text of chapter and reports whether
// the search may go on.
func (sr *Search) searchChapter(chapter int, text []rune, folds map[rune][]rune) bool {
	sr.mu.Lock()
	room := MaxHits - len(sr.hits) // only this goroutine adds hits
	sr.mu.Unlock()
	folded, source := foldText(text, folds)
	q := sr.query
	// offset counts the counted characters in text[:pos]; matches come in
	// order, so it only moves forward.
	pos, offset := 0, 0
	offsetAt := func(k int) int {
		for ; pos < k; pos++ {
			if counted(text[pos]) {
				offset++
			}
		}
		return offset
	}
	var found []Hit
	for i := 0; i+len(q) <= len(folded) && len(found) <= room; i++ {
		if folded[i] != q[0] || !slices.Equal(folded[i:i+len(q)], q) {
			continue
		}
		first, last := source[i], source[i+len(q)-1]
		hit := Hit{Range: Range{
			Start: Anchor{Chapter: chapter, Offset: offsetAt(first)},
			End:   Anchor{Chapter: chapter, Offset: offsetAt(last + 1)},
		}}
		hit.Excerpt, hit.MatchStart, hit.MatchLength = excerpt(text, first, last+1)
		found = append(found, hit)
		i += len(q) - 1
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if len(found) > room {
		found, sr.truncated = found[:room], true
	}
	sr.hits = append(sr.hits, found...)
	return !sr.truncated
}

// excerpt returns the text around text[from:to] on one line, cut at word
// boundaries, and where the match is in it, in bytes.
func excerpt(text []rune, from, to int) (s string, matchStart, matchLength int) {
	room := max(0, excerptLength-(to-from)) / 2
	start, end := max(0, from-room), min(len(text), to+room)
	for start < from && start > 0 && !unicode.IsSpace(text[start-1]) {
		start++
	}
	for end > to && end < len(text) && !unicode.IsSpace(text[end]) {
		end--
	}
	line := func(r []rune) string {
		return strings.ReplaceAll(string(r), "\n", " ")
	}
	before := strings.TrimLeft(line(text[start:from]), " ")
	match := line(text[from:to])
	after := strings.TrimRight(line(text[to:end]), " ")
	return before + match + after, len(before), len(match)
}

// foldQuery returns the folded form of a query, with whitespace collapsed
// to single spaces and trimmed.
func foldQuery(query string) []rune {
	runes, _ := foldText([]rune(query), map[rune][]rune{})
	for len(runes) > 0 && runes[0] == ' ' {
		runes = runes[1:]
	}
	for len(runes) > 0 && runes[len(runes)-1] == ' ' {
		runes = runes[:len(runes)-1]
	}
	return runes
}

// foldText returns the folded form of text and, for each folded rune, the
// index of the rune of text it comes from. Runs of whitespace fold to one
// space.
func foldText(text []rune, folds map[rune][]rune) (folded []rune, source []int) {
	folded = make([]rune, 0, len(text))
	source = make([]int, 0, len(text))
	for i, r := range text {
		n := len(folded)
		folded = fold(folded, r, folds)
		if len(folded) == n+1 && folded[n] == ' ' && n > 0 && folded[n-1] == ' ' {
			folded = folded[:n]
		}
		for range len(folded) - n {
			source = append(source, i)
		}
	}
	return folded, source
}

// fold appends the searchable form of r to dst: lower case, without accents
// and other combining marks, with compatibility forms such as ligatures
// decomposed. Whitespace folds to a space; hyphens and invisible format
// characters fold to nothing, so that a word broken at a hyphen still
// matches. folds caches the forms of non-ASCII runes.
func fold(dst []rune, r rune, folds map[rune][]rune) []rune {
	if r < utf8.RuneSelf {
		switch {
		case unicode.IsSpace(r):
			return append(dst, ' ')
		case r == '-':
			return dst
		case 'A' <= r && r <= 'Z':
			return append(dst, r+'a'-'A')
		}
		return append(dst, r)
	}
	if unicode.IsSpace(r) {
		return append(dst, ' ')
	}
	if isHyphen(r) || unicode.Is(unicode.Cf, r) {
		return dst
	}
	f, ok := folds[r]
	if !ok {
		for _, d := range norm.NFKD.String(string(r)) {
			switch {
			case unicode.Is(unicode.Mn, d):
			case d == '\u00DF' || d == '\u1E9E': // sharp s
				f = append(f, 's', 's')
			default:
				f = append(f, unicode.ToLower(d))
			}
		}
		folds[r] = f
	}
	return append(dst, f...)
}

// bookText is the text of every chapter of a book, extracted once, from a
// copy of the book with its own MuPDF context, on a goroutine of its own,
// and shared by the searches of a session.
type bookText struct {
	stop   chan struct{}
	once   sync.Once
	exited chan struct{}

	mu       sync.Mutex
	chapters [][]rune      // normalized text of the chapters extracted so far
	changed  chan struct{} // closed when a chapter is added or the extraction ends
	done     bool
	err      error
}

func startBookText(path string) *bookText {
	b := &bookText{stop: make(chan struct{}), exited: make(chan struct{}), changed: make(chan struct{})}
	go b.run(path)
	return b
}

// close stops the extraction without waiting for it.
func (b *bookText) close() {
	b.once.Do(func() { close(b.stop) })
}

func (b *bookText) run(path string) {
	defer close(b.exited)
	err := b.extract(path)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.done, b.err = true, err
	close(b.changed)
}

func (b *bookText) extract(path string) error {
	doc, err := bookfile.Open(path, userCSS)
	if err != nil {
		return err
	}
	defer doc.Close()
	if _, err := doc.Layout(searchWidth, searchHeight, searchFontSize); err != nil {
		return err
	}
	bottom := 0.0 // lowest line bottom seen, for guessing paragraph breaks
	for chapter := range doc.Chapters() {
		text, ok, err := b.chapterText(doc, chapter, &bottom)
		if err != nil || !ok {
			return err
		}
		b.mu.Lock()
		b.chapters = append(b.chapters, text)
		close(b.changed)
		b.changed = make(chan struct{})
		b.mu.Unlock()
	}
	return nil
}

// chapterText returns the normalized text of chapter and lowers *bottom to
// the lowest line bottom on its pages. It reports false when the extraction
// is stopped.
func (b *bookText) chapterText(doc *mupdf.Document, chapter int, bottom *float64) ([]rune, bool, error) {
	pages, _ := doc.ChapterPages(chapter)
	out := newTextBuilder()
	var prev *pageText
	for index := range pages {
		select {
		case <-b.stop:
			return nil, false, nil
		default:
		}
		page, ok := doc.PageNumber(chapter, index)
		if !ok {
			break
		}
		chars, err := doc.PageChars(page)
		if err != nil {
			return nil, false, err
		}
		start := 0
		if prev != nil {
			start = prev.start + prev.count
		}
		t := newPageText(chapter, start, chars)
		*bottom = max(*bottom, lowest(chars))
		if prev != nil {
			out.page(continues(prev, t, *bottom))
		}
		for _, c := range chars {
			out.add(c)
		}
		prev = t
	}
	return out.runes, true, nil
}

// chapter returns the text of chapter i, waiting for it to be extracted. It
// reports false when the book has no such chapter, the extraction failed or
// stop is closed.
func (b *bookText) chapter(i int, stop <-chan struct{}) ([]rune, bool, error) {
	for {
		b.mu.Lock()
		if i < len(b.chapters) {
			text := b.chapters[i]
			b.mu.Unlock()
			return text, true, nil
		}
		if b.done {
			err := b.err
			b.mu.Unlock()
			return nil, false, err
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-changed:
		case <-stop:
			return nil, false, nil
		}
	}
}

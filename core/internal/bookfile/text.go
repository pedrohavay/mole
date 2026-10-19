package bookfile

import (
	"bytes"
	"html"
	"iter"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	encunicode "golang.org/x/text/encoding/unicode"
)

// Byte order marks.
var (
	utf8BOM    = []byte("\xEF\xBB\xBF")
	utf16LEBOM = []byte("\xFF\xFE")
	utf16BEBOM = []byte("\xFE\xFF")
)

// decodeText returns the text of a plain text file: UTF-16 when a byte order
// mark says so, UTF-8 when the bytes are valid UTF-8, and Windows-1252, the
// usual encoding of older text files, otherwise.
func decodeText(data []byte) string {
	var enc encoding.Encoding
	switch {
	case bytes.HasPrefix(data, utf16LEBOM):
		enc = encunicode.UTF16(encunicode.LittleEndian, encunicode.ExpectBOM)
	case bytes.HasPrefix(data, utf16BEBOM):
		enc = encunicode.UTF16(encunicode.BigEndian, encunicode.ExpectBOM)
	default:
		data = bytes.TrimPrefix(data, utf8BOM)
		if utf8.Valid(data) {
			return string(data)
		}
		enc = charmap.Windows1252
	}
	text, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		// Both decoders replace what they cannot decode instead of failing.
		return string(bytes.ToValidUTF8(data, []byte("�")))
	}
	return string(text)
}

// inspectText reads the title and the author from the header of a Project
// Gutenberg text, the most common source of books as plain text.
func inspectText(path string, info *Info) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info.Title, info.Author = textHeader(decodeText(data))
	return nil
}

// headerLines is how many lines at the start of a text textHeader reads.
const headerLines = 60

// textHeader returns the values of the "Title:" and "Author:" lines among
// the first lines of text, as Project Gutenberg writes them.
func textHeader(text string) (title, author string) {
	lines := strings.SplitN(text, "\n", headerLines+1)
	for _, line := range lines[:min(len(lines), headerLines)] {
		if v, ok := strings.CutPrefix(line, "Title:"); ok && title == "" {
			title = clean(v)
		} else if v, ok := strings.CutPrefix(line, "Author:"); ok && author == "" {
			author = clean(v)
		}
	}
	return title, author
}

// textStyle lays out the paragraphs of a text: spaced blocks, and headings
// that start a page unless they start the book. MuPDF has no :first-child,
// and a break before the first heading would leave the first page empty.
const textStyle = `p{margin:0 0 0.8em}` +
	`h2{font-size:1.3em;text-align:center;margin:1em 0 1.5em}` +
	`h2.next{page-break-before:always}`

// unwrappedLength is the line length, in characters, past which a text is
// taken to keep each paragraph on one line instead of wrapping its lines.
const unwrappedLength = 100

// textHTML turns plain text into an HTML document of paragraphs. Blank
// lines separate paragraphs. When the text wraps its lines at a fixed
// width, the lines of a paragraph are joined again, except after a line
// much shorter than the others (verse, an address), whose break is kept;
// an indented line after one that is not starts a paragraph. When it does
// not wrap, every line is a paragraph. A line on its own that names a
// chapter or a part becomes a heading, so the book has a table of contents.
func textHTML(text string) []byte {
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\f", "\n\n", "\t", "    ").Replace(text)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	width, wrapped := wrapWidth(lines)

	var b bytes.Buffer
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><style>` + textStyle + `</style></head><body>`)
	start := b.Len()
	for block := range blocks(lines) {
		if !wrapped || len(block) == 1 {
			for _, line := range block {
				writeLine(&b, line, b.Len() == start)
			}
			continue
		}
		b.WriteString("<p>")
		for i, line := range block {
			if i > 0 {
				prev := block[i-1]
				switch {
				case indent(line) > 0 && indent(prev) == 0:
					b.WriteString("</p>\n<p>")
				case utf8.RuneCountInString(prev) < width*6/10:
					b.WriteString("<br/>")
				case !strings.HasSuffix(prev, "-"):
					b.WriteByte(' ')
				}
			}
			b.WriteString(html.EscapeString(strings.TrimSpace(line)))
		}
		b.WriteString("</p>\n")
	}
	b.WriteString("</body></html>")
	return b.Bytes()
}

// writeLine writes a line that stands alone as a heading or a paragraph,
// the first of the book when first is set.
func writeLine(b *bytes.Buffer, line string, first bool) {
	line = strings.TrimSpace(line)
	switch {
	case isHeading(line) && first:
		b.WriteString("<h2>" + html.EscapeString(line) + "</h2>\n")
		return
	case isHeading(line):
		b.WriteString(`<h2 class="next">` + html.EscapeString(line) + "</h2>\n")
		return
	}
	b.WriteString("<p>" + html.EscapeString(line) + "</p>\n")
}

// blocks yields the runs of lines between blank lines.
func blocks(lines []string) iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		start := 0
		for i := 0; i <= len(lines); i++ {
			if i < len(lines) && lines[i] != "" {
				continue
			}
			if i > start && !yield(lines[start:i]) {
				return
			}
			start = i + 1
		}
	}
}

// wrapWidth returns the length of a full line of text and whether the text
// wraps its lines at about that length. A text wraps when hardly any line
// is longer than unwrappedLength; a full line is then as long as most of
// the longer lines.
func wrapWidth(lines []string) (width int, wrapped bool) {
	var lengths []int
	long := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		n := utf8.RuneCountInString(line)
		lengths = append(lengths, n)
		if n > unwrappedLength {
			long++
		}
	}
	if len(lengths) == 0 {
		return 0, false
	}
	slices.Sort(lengths)
	return lengths[len(lengths)*9/10], long*100 <= len(lengths)
}

// indent returns the number of leading spaces of line.
func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// Headings: a chapter or a part with its number, a section of the front or
// back matter, or a Roman numeral alone. English and Portuguese words.
var (
	numberedHeading = regexp.MustCompile(`(?i)^(chapter|part|book|cap[íi]tulo|parte|livro)\s+([0-9]+|[ivxlcdm]+)\b`)
	namedHeading    = regexp.MustCompile(`(?i)^(chapter|part|book|cap[íi]tulo|parte|livro)\b`)
	sectionHeading  = regexp.MustCompile(`(?i)^(prologue|epilogue|preface|introduction|afterword|pr[óo]logo|ep[íi]logo|pref[áa]cio|introdu[çc][ãa]o|posf[áa]cio)\s*[.:]?$`)
	romanHeading    = regexp.MustCompile(`^[IVXLC]+\.?$`)
)

// maxHeadingLength is the longest line, in characters, taken for a heading.
const maxHeadingLength = 60

// isHeading reports whether a line on its own is the heading of a chapter. A chapter word followed by a name instead of
// a number counts only in capitals ("CHAPTER ONE"), since a sentence may
// start with the same word.
func isHeading(line string) bool {
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) > maxHeadingLength {
		return false
	}
	switch {
	case numberedHeading.MatchString(line), sectionHeading.MatchString(line), romanHeading.MatchString(line):
		return true
	case namedHeading.MatchString(line):
		return strings.ToUpper(line) == line
	}
	return false
}

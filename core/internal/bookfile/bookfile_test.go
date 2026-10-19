package bookfile

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"mole/core/internal/mupdf"
	"mole/core/internal/testbook"
)

// coverPNG returns a small PNG image for the covers of test books.
func coverPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	return testbook.PNG(t, img)
}

// bookText returns the text of every page of doc laid out at 400 x 600
// points, with whitespace collapsed.
func bookText(t *testing.T, doc *mupdf.Document) string {
	t.Helper()
	pages, err := doc.Layout(400, 600, 12)
	if err != nil {
		t.Fatalf("Layout error: %v", err)
	}
	var b strings.Builder
	for page := range pages {
		chars, err := doc.PageChars(page)
		if err != nil {
			t.Fatalf("PageChars(%d) error: %v", page, err)
		}
		for i, c := range chars {
			if i > 0 && c.Line != chars[i-1].Line {
				b.WriteByte(' ')
			}
			b.WriteRune(c.Rune)
		}
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func TestDetect(t *testing.T) {
	cover := coverPNG(t)
	tests := []struct {
		name string
		path string
		want Format
	}{
		{name: "EPUB", path: testbook.Chapters(t, "<p>Hi.</p>"), want: EPUB},
		{name: "MOBI", path: testbook.MOBI{Title: "M", HTML: "<p>Hi.</p>"}.Write(t), want: MOBI},
		{name: "FB2", path: testbook.FB2{Title: "F", Sections: []string{"<p>Hi.</p>"}, Cover: cover}.Write(t), want: FB2},
		{name: "Text", path: testbook.File(t, "notes.txt", []byte("Hello.\n")), want: Text},
		{name: "UTF16Text", path: testbook.File(t, "notes.TXT", []byte("\xFF\xFEH\x00i\x00")), want: Text},
		// Content wins over the extension.
		{name: "FB2NamedText", path: testbook.File(t, "book.txt", []byte(`<?xml version="1.0"?><FictionBook><body/></FictionBook>`)), want: FB2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := Detect(tt.path); err != nil || got != tt.want {
				t.Errorf("Detect(%s) = %v, %v; want %v", tt.path, got, err, tt.want)
			}
		})
	}

	for _, tt := range []struct{ name, file, content string }{
		{name: "BinaryText", file: "data.txt", content: "\x00\x01\x02binary"},
		{name: "UnknownExtension", file: "notes.md", content: "# Notes\n"},
		{name: "MOBIOfText", file: "book.mobi", content: "plain words"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := testbook.File(t, tt.file, []byte(tt.content))
			if got, err := Detect(path); !errors.Is(err, ErrUnsupported) {
				t.Errorf("Detect(%s) = %v, %v; want ErrUnsupported", tt.file, got, err)
			}
		})
	}
}

func TestExtension(t *testing.T) {
	tests := []struct {
		format     Format
		path, want string
	}{
		{format: EPUB, path: "/d/a.epub", want: ".epub"},
		{format: MOBI, path: "/d/a.PRC", want: ".prc"},
		{format: MOBI, path: "/d/a.azw", want: ".mobi"},
		{format: FB2, path: "/d/a.txt", want: ".fb2"},
		{format: Text, path: "/d/a.text", want: ".text"},
		{format: Text, path: "/d/README", want: ".txt"},
	}
	for _, tt := range tests {
		if got := tt.format.Extension(tt.path); got != tt.want {
			t.Errorf("Format(%d).Extension(%q) = %q, want %q", tt.format, tt.path, got, tt.want)
		}
	}
}

func TestFormatOf(t *testing.T) {
	tests := []struct {
		name   string
		want   Format
		wantOK bool
	}{
		{name: "/d/a.EPUB", want: EPUB, wantOK: true},
		{name: "/d/a.prc", want: MOBI, wantOK: true},
		{name: "/d/a.fb2", want: FB2, wantOK: true},
		{name: "/d/a.text", want: Text, wantOK: true},
		{name: "/d/a.pdf"},
		{name: "/d/README"},
	}
	for _, tt := range tests {
		if got, ok := FormatOf(tt.name); got != tt.want || ok != tt.wantOK {
			t.Errorf("FormatOf(%q) = %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestInspect(t *testing.T) {
	cover := coverPNG(t)
	gutenberg := "The Project Gutenberg eBook of Plain Words\r\n\r\nTitle: Plain Words\r\n\r\nAuthor: Ana Lima\r\n\r\nSome text.\r\n"
	tests := []struct {
		name       string
		path       string
		want       Info
		firstPage  bool // the cover is the first page, as PNG
		wantInText string
	}{
		{
			name: "MOBI",
			path: testbook.MOBI{Title: "Mobi Title", Author: "Ana Lima", HTML: "<p>Hello from MOBI.</p>", Cover: cover}.Write(t),
			want: Info{Format: MOBI, Title: "Mobi Title", Author: "Ana Lima", Cover: cover},
		},
		{
			name:      "MOBIWithoutCover",
			path:      testbook.MOBI{Title: "Bare", HTML: "<p>Hello.</p>"}.Write(t),
			want:      Info{Format: MOBI, Title: "Bare"},
			firstPage: true,
		},
		{
			name:      "PalmDOC",
			path:      testbook.PalmDOC{Title: "Old Text", Text: "Plain old words.\n"}.Write(t),
			want:      Info{Format: MOBI, Title: "Old Text"},
			firstPage: true,
		},
		{
			name: "FB2",
			path: testbook.FB2{Title: "Fb Title", Author: [2]string{"João", "Silva"}, Sections: []string{"<p>Hello.</p>"}, Cover: cover}.Write(t),
			want: Info{Format: FB2, Title: "Fb Title", Author: "João Silva", Cover: cover},
		},
		{
			name:      "Text",
			path:      testbook.File(t, "plain.txt", []byte(gutenberg)),
			want:      Info{Format: Text, Title: "Plain Words", Author: "Ana Lima"},
			firstPage: true,
		},
		{
			name:      "TextWithoutHeader",
			path:      testbook.File(t, "plain.txt", []byte("Just words.\n")),
			want:      Info{Format: Text},
			firstPage: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Inspect(tt.path)
			if err != nil {
				t.Fatalf("Inspect error: %v", err)
			}
			if got.Format != tt.want.Format || got.Title != tt.want.Title || got.Author != tt.want.Author {
				t.Errorf("Inspect = %v %q by %q, want %v %q by %q", got.Format, got.Title, got.Author, tt.want.Format, tt.want.Title, tt.want.Author)
			}
			switch {
			case tt.firstPage && !bytes.HasPrefix(got.Cover, []byte("\x89PNG")):
				t.Errorf("cover = %d bytes, want the first page as PNG", len(got.Cover))
			case !tt.firstPage && !bytes.Equal(got.Cover, tt.want.Cover):
				t.Errorf("cover = %d bytes, want the book's %d", len(got.Cover), len(tt.want.Cover))
			}
		})
	}
}

func TestDRM(t *testing.T) {
	path := testbook.MOBI{Title: "Locked", HTML: "<p>Secret.</p>", Encrypted: true}.Write(t)
	if _, err := Inspect(path); !errors.Is(err, ErrDRM) {
		t.Errorf("Inspect of a protected MOBI = %v, want ErrDRM", err)
	}
	if doc, err := Open(path, ""); !errors.Is(err, ErrDRM) {
		if doc != nil {
			doc.Close()
		}
		t.Errorf("Open of a protected MOBI = %v, want ErrDRM", err)
	}
}

func TestOpen(t *testing.T) {
	long := strings.Repeat("Many words fill this book. ", 400) // over one 4 KB text record
	tests := []struct {
		name string
		path string
		want []string // pieces of the text, in order
	}{
		{
			name: "MOBI",
			path: testbook.MOBI{Title: "M", HTML: "<h1>Start</h1><p>Ação e coração.</p><p>" + long + "</p><p>The end.</p>"}.Write(t),
			want: []string{"Start", "Ação e coração.", "Many words fill", "The end."},
		},
		{
			name: "PalmDOC",
			path: testbook.PalmDOC{Title: "P", Text: "Plain old words.\nCaf\xe9 " + long + "\nThe end.\n"}.Write(t),
			want: []string{"Plain old words.", "Café", "Many words fill", "The end."},
		},
		{
			name: "FB2",
			path: testbook.FB2{Title: "F", Sections: []string{"<title><p>One</p></title><p>First section.</p>", "<title><p>Two</p></title><p>Second section.</p>"}}.Write(t),
			want: []string{"One", "First section.", "Two", "Second section."},
		},
		{
			name: "Text",
			path: testbook.File(t, "book.txt", []byte("CHAPTER I.\n\nIt was a dark\nand stormy night.\n\nCaf\xe9 \xe0 la carte.\n")),
			want: []string{"CHAPTER I.", "It was a dark and stormy night.", "Café à la carte."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Open(tt.path, "")
			if err != nil {
				t.Fatalf("Open error: %v", err)
			}
			defer doc.Close()
			text := bookText(t, doc)
			at := 0
			for _, piece := range tt.want {
				i := strings.Index(text[at:], piece)
				if i < 0 {
					t.Fatalf("text %q lacks %q after offset %d", text, piece, at)
				}
				at += i + len(piece)
			}
		})
	}
}

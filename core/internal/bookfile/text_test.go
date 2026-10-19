package bookfile

import (
	"strings"
	"testing"
)

func TestDecodeText(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "UTF8", give: "Ação\n", want: "Ação\n"},
		{name: "UTF8WithBOM", give: "\xEF\xBB\xBFAção", want: "Ação"},
		{name: "Windows1252", give: "A\xe7\xe3o \x93quoted\x94 \x80", want: "Ação “quoted” €"},
		{name: "UTF16LE", give: "\xFF\xFEA\x00\xe7\x00", want: "Aç"},
		{name: "UTF16BE", give: "\xFE\xFF\x00A\x00\xe7", want: "Aç"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeText([]byte(tt.give)); got != tt.want {
				t.Errorf("decodeText(%q) = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}

func TestTextHeader(t *testing.T) {
	text := "The Project Gutenberg eBook of Dom Casmurro\n\nTitle:  Dom  Casmurro\n\nAuthor: Machado de Assis\r\n\nTitle: Not this one\n"
	title, author := textHeader(text)
	if title != "Dom Casmurro" || author != "Machado de Assis" {
		t.Errorf("textHeader = %q, %q; want the first Title and Author lines", title, author)
	}
	if title, author := textHeader("No header here.\n"); title != "" || author != "" {
		t.Errorf("textHeader of a text without a header = %q, %q; want nothing", title, author)
	}
}

// body returns the body of the HTML textHTML makes of text, without the
// line breaks between its elements.
func body(text string) string {
	html := string(textHTML(text))
	_, html, _ = strings.Cut(html, "<body>")
	html, _, _ = strings.Cut(html, "</body>")
	return strings.ReplaceAll(html, "\n", "")
}

func TestTextHTML(t *testing.T) {
	wrapped := "Call me Ishmael. Some years ago, never mind how long precisely, having\n" +
		"little or no money in my purse, and nothing particular to interest me on\n" +
		"shore, I thought I would sail about a little.\n"
	tests := []struct {
		name string
		give string
		want string
	}{
		{
			name: "WrappedParagraphs",
			give: wrapped + "\n" + wrapped,
			want: "<p>Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little.</p>" +
				"<p>Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little.</p>",
		},
		{
			name: "WindowsLineEnds",
			give: strings.ReplaceAll(wrapped, "\n", "\r\n"),
			want: "<p>Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little.</p>",
		},
		{
			name: "VerseKeepsItsLines",
			give: wrapped + "\n    Tyger Tyger, burning bright,\n    In the forests of the night;\n",
			want: "<p>Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little.</p>" +
				"<p>Tyger Tyger, burning bright,<br/>In the forests of the night;</p>",
		},
		{
			name: "IndentedParagraphs",
			give: "  The first paragraph starts here and goes on for a while, until the\n" +
				"line is full and the next one continues the same thought for longer.\n" +
				"  The second paragraph starts with an indent like the first one did.\n",
			want: "<p>The first paragraph starts here and goes on for a while, until the line is full and the next one continues the same thought for longer.</p>" +
				"<p>The second paragraph starts with an indent like the first one did.</p>",
		},
		{
			name: "HyphenJoinsWithoutSpace",
			give: "A long line of words that ends in the middle of a compound well-\nknown word and keeps on going for long enough to be a full line too.\n",
			want: "<p>A long line of words that ends in the middle of a compound well-known word and keeps on going for long enough to be a full line too.</p>",
		},
		{
			name: "UnwrappedLines",
			give: strings.Repeat("x", 120) + "\n" + "Short reply.\n" + strings.Repeat("y", 130) + "\n",
			want: "<p>" + strings.Repeat("x", 120) + "</p><p>Short reply.</p><p>" + strings.Repeat("y", 130) + "</p>",
		},
		{
			name: "Headings",
			give: "CHAPTER I.\n\n" + wrapped + "\nChapter 2: The Sea\n\nPrologue\n\nPart of the reason was this.\n\nXIV\n",
			want: "<h2>CHAPTER I.</h2>" +
				"<p>Call me Ishmael. Some years ago, never mind how long precisely, having little or no money in my purse, and nothing particular to interest me on shore, I thought I would sail about a little.</p>" +
				`<h2 class="next">Chapter 2: The Sea</h2><h2 class="next">Prologue</h2><p>Part of the reason was this.</p><h2 class="next">XIV</h2>`,
		},
		{
			name: "Escapes",
			give: "<b> & \"quotes\"\f\fNext page.\n",
			want: "<p>&lt;b&gt; &amp; &#34;quotes&#34;</p><p>Next page.</p>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := body(tt.give); got != tt.want {
				t.Errorf("textHTML(%q) body =\n%s\nwant\n%s", tt.give, got, tt.want)
			}
		})
	}
}

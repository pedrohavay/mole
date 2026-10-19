package bookfile

import (
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/text/encoding/htmlindex"
)

// maxCoverSize is the largest cover image read from an FB2 book, encoded.
const maxCoverSize = 32 << 20

// titleInfo is the path of the element of an FB2 book that describes it.
const titleInfo = "FictionBook/description/title-info/"

// inspectFB2 reads the title, the first author and the cover image of the
// FictionBook 2 book at path.
func inspectFB2(path string, info *Info) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := readFB2(f, info); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// readFB2 reads the metadata of the FB2 document in r into info: the title
// and the authors from its description, which comes first, and the cover
// from the binary elements, which come last.
func readFB2(r io.Reader, info *Info) error {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		enc, err := htmlindex.Get(label)
		if err != nil {
			return nil, err
		}
		return enc.NewDecoder().Reader(input), nil
	}
	var (
		path      []string // local names of the open elements
		text      strings.Builder
		reading   bool              // collecting the text of the innermost element
		inAuthor  bool              // inside the first author
		names     map[string]string // parts of the first author's name
		coverID   string
		fromCover bool // the binary being read is the cover
	)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if info.Title != "" {
				return nil // MuPDF decides whether the rest can be read
			}
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			switch at := strings.Join(path, "/"); {
			case at == titleInfo+"book-title", inAuthor && len(path) == 5:
				reading = true
				text.Reset()
			case at == titleInfo+"author" && names == nil:
				inAuthor, names = true, map[string]string{}
			case at == titleInfo+"coverpage/image" && coverID == "":
				coverID = strings.TrimPrefix(attr(t, "href"), "#")
			case at == "FictionBook/binary" && coverID != "" && attr(t, "id") == coverID:
				fromCover, reading = true, true
				text.Reset()
			}
		case xml.CharData:
			if reading && text.Len() < maxCoverSize {
				text.Write(t)
			}
		case xml.EndElement:
			if len(path) == 0 {
				continue // the decoder keeps elements balanced
			}
			at, name := strings.Join(path, "/"), path[len(path)-1]
			path = path[:len(path)-1]
			switch {
			case at == titleInfo+"book-title":
				info.Title = clean(text.String())
			case inAuthor && len(path) == 4:
				names[name] = clean(text.String())
			case at == titleInfo+"author" && inAuthor:
				info.Author, inAuthor = authorName(names), false
			case fromCover:
				data := strings.Map(dropSpace, text.String())
				if img, err := base64.StdEncoding.DecodeString(data); err == nil && isImage(img) {
					info.Cover = img
				}
				return nil
			}
			reading = false
		}
	}
}

// attr returns the value of the attribute of el with the local name, or "".
func attr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// authorName returns the name of an FB2 author from its first, middle and
// last names, or its nickname when it has none of those.
func authorName(names map[string]string) string {
	if name := clean(names["first-name"] + " " + names["middle-name"] + " " + names["last-name"]); name != "" {
		return name
	}
	return names["nickname"]
}

// dropSpace maps whitespace to nothing, for strings.Map.
func dropSpace(r rune) rune {
	switch r {
	case ' ', '\t', '\r', '\n':
		return -1
	}
	return r
}

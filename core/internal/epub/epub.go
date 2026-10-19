// Package epub reads the package metadata of EPUB files: title, author and
// the cover image they declare. It does not lay out or render content.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Limits on what is read from an archive, which may come from anywhere.
const (
	maxPackageSize = 4 << 20
	maxCoverSize   = 32 << 20
)

// Info is the metadata of a book.
type Info struct {
	Title  string
	Author string
	// Cover is the encoded cover image (JPEG, PNG, ...), or nil when the
	// package declares none or it cannot be read.
	Cover []byte
}

type packageDoc struct {
	Titles   []string `xml:"metadata>title"`
	Creators []string `xml:"metadata>creator"`
	Metas    []struct {
		Name    string `xml:"name,attr"`
		Content string `xml:"content,attr"`
	} `xml:"metadata>meta"`
	Items []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Guide []struct {
		Type string `xml:"type,attr"`
		Href string `xml:"href,attr"`
	} `xml:"guide>reference"`
}

// Inspect reads the metadata of the EPUB at file.
func Inspect(file string) (Info, error) {
	archive, err := zip.OpenReader(file)
	if err != nil {
		return Info{}, err
	}
	defer archive.Close()

	var container struct {
		Rootfiles []struct {
			Path string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := readXML(&archive.Reader, "META-INF/container.xml", &container); err != nil {
		return Info{}, err
	}
	if len(container.Rootfiles) == 0 || container.Rootfiles[0].Path == "" {
		return Info{}, errors.New("epub has no package document")
	}
	opfPath := container.Rootfiles[0].Path
	var pkg packageDoc
	if err := readXML(&archive.Reader, opfPath, &pkg); err != nil {
		return Info{}, err
	}

	info := Info{Title: first(pkg.Titles), Author: first(pkg.Creators)}
	info.Cover = findCover(&archive.Reader, path.Dir(opfPath), pkg)
	return info, nil
}

// findCover returns the first cover that can be read, trying in turn: the
// item with the cover-image property (EPUB 3), the item the cover meta
// names by ID or by href (EPUB 2), and the cover in the guide. A candidate
// that is a page rather than an image gives the first image on it.
func findCover(archive *zip.Reader, dir string, pkg packageDoc) []byte {
	var hrefs []string
	for _, item := range pkg.Items {
		if strings.Contains(" "+item.Properties+" ", " cover-image ") {
			hrefs = append(hrefs, item.Href)
		}
	}
	for _, meta := range pkg.Metas {
		if meta.Name != "cover" || meta.Content == "" {
			continue
		}
		for _, item := range pkg.Items {
			if item.ID == meta.Content || item.Href == meta.Content {
				hrefs = append(hrefs, item.Href)
			}
		}
	}
	for _, ref := range pkg.Guide {
		if strings.EqualFold(ref.Type, "cover") {
			hrefs = append(hrefs, ref.Href)
		}
	}
	for _, href := range hrefs {
		name, ok := resolve(dir, href)
		if !ok {
			continue
		}
		if isPage(pkg, dir, name) {
			if name, ok = firstImage(archive, name); !ok {
				continue
			}
		}
		if cover, err := readFile(archive, name, maxCoverSize); err == nil {
			return cover
		}
	}
	return nil
}

// resolve returns the archive name of href, relative to dir, without its
// fragment.
func resolve(dir, href string) (string, bool) {
	href, _, _ = strings.Cut(href, "#")
	name, err := url.PathUnescape(href)
	if err != nil || name == "" {
		return "", false
	}
	return path.Join(dir, name), true
}

// isPage reports whether the archive file name is an (X)HTML page, by its
// media type in the manifest or else by its extension.
func isPage(pkg packageDoc, dir, name string) bool {
	for _, item := range pkg.Items {
		if n, ok := resolve(dir, item.Href); ok && n == name {
			return strings.Contains(item.MediaType, "html")
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".xhtml", ".html", ".htm":
		return true
	}
	return false
}

// firstImage returns the archive name of the first image of a page: an
// HTML img or an SVG image.
func firstImage(archive *zip.Reader, page string) (string, bool) {
	data, err := readFile(archive, page, maxPackageSize)
	if err != nil {
		return "", false
	}
	d := newDecoder(data)
	// Pages are often not well-formed XML.
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	for {
		tok, err := d.Token()
		if err != nil {
			return "", false
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range el.Attr {
			if (el.Name.Local == "img" && attr.Name.Local == "src") || (el.Name.Local == "image" && attr.Name.Local == "href") {
				return resolve(path.Dir(page), attr.Value)
			}
		}
	}
}

func first(values []string) string {
	for _, v := range values {
		if v = strings.Join(strings.Fields(v), " "); v != "" {
			return v
		}
	}
	return ""
}

func readXML(archive *zip.Reader, name string, v any) error {
	data, err := readFile(archive, name, maxPackageSize)
	if err != nil {
		return err
	}
	// encoding/xml does not resolve external entities, so untrusted package
	// documents cannot reach the file system or the network.
	if err := newDecoder(data).Decode(v); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}

// newDecoder returns an XML decoder of data that also reads the Latin-1 and
// Windows-1252 encodings some older books declare.
func newDecoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "iso-8859-1", "iso8859-1", "iso_8859-1", "latin1", "latin-1", "l1":
			return charmap.ISO8859_1.NewDecoder().Reader(input), nil
		case "windows-1252", "cp1252", "x-cp1252":
			return charmap.Windows1252.NewDecoder().Reader(input), nil
		case "us-ascii", "ascii":
			return input, nil
		}
		return nil, fmt.Errorf("unsupported encoding %q", label)
	}
	return d
}

func readFile(archive *zip.Reader, name string, limit int64) ([]byte, error) {
	f, err := archive.Open(path.Clean(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("read %s: larger than %d bytes", name, limit)
	}
	return data, nil
}

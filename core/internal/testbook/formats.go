package testbook

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// FB2 describes a FictionBook 2 book to write.
type FB2 struct {
	Title    string
	Author   [2]string // first and last name
	Sections []string  // the body XML of each section, such as "<title><p>One</p></title><p>Text.</p>"
	Cover    []byte    // a PNG image, or nil
	// Images are more binaries, PNG or JPEG, by ID; a section shows one
	// with <image l:href="#ID"/>.
	Images map[string][]byte
}

// Write writes the book into a new temporary directory of t and returns the
// path of the .fb2 file.
func (b FB2) Write(t testing.TB) string {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">
<description><title-info>`)
	fmt.Fprintf(&buf, `<author><first-name>%s</first-name><last-name>%s</last-name></author>`,
		html.EscapeString(b.Author[0]), html.EscapeString(b.Author[1]))
	fmt.Fprintf(&buf, `<book-title>%s</book-title><lang>en</lang>`, html.EscapeString(b.Title))
	if b.Cover != nil {
		buf.WriteString(`<coverpage><image l:href="#cover.png"/></coverpage>`)
	}
	buf.WriteString("</title-info></description>\n<body>")
	for _, s := range b.Sections {
		buf.WriteString("<section>" + s + "</section>\n")
	}
	buf.WriteString("</body>\n")
	if b.Cover != nil {
		fmt.Fprintf(&buf, `<binary id="cover.png" content-type="image/png">%s</binary>`, base64.StdEncoding.EncodeToString(b.Cover))
	}
	ids := make([]string, 0, len(b.Images))
	for id := range b.Images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		kind := "image/png"
		if bytes.HasPrefix(b.Images[id], []byte("\xFF\xD8")) {
			kind = "image/jpeg"
		}
		fmt.Fprintf(&buf, `<binary id="%s" content-type="%s">%s</binary>`, html.EscapeString(id), kind,
			base64.StdEncoding.EncodeToString(b.Images[id]))
	}
	buf.WriteString("</FictionBook>\n")
	return File(t, "book.fb2", buf.Bytes())
}

// File writes data into a new temporary directory of t under name and
// returns its path.
func File(t testing.TB, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// MOBI describes a Mobipocket book to write: a Palm database whose text is
// HTML compressed with PalmDOC, with the metadata in an EXTH block and the
// cover in an image record after the text.
type MOBI struct {
	Title  string
	Author string
	HTML   string // the body of the book
	Cover  []byte // an encoded image, or nil
	// Images are more image records, after the cover; the HTML shows one
	// with <img recindex="..."/>, the value ImageIndex returns.
	Images [][]byte
	// Encrypted marks the book as protected by DRM. Its text is left as
	// it is, as only the mark matters to a reader that refuses it.
	Encrypted bool
}

// MOBI record sizes and header values.
const (
	mobiTextRecord = 4096 // uncompressed bytes of text per record
	mobiHeaderLen  = 232
	mobiUTF8       = 65001
)

// Write writes the book into a new temporary directory of t and returns the
// path of the .mobi file.
func (b MOBI) Write(t testing.TB) string {
	t.Helper()
	text := []byte("<html><head></head><body>" + b.HTML + "</body></html>")
	var records [][]byte
	records = append(records, nil) // record 0, the headers, written last
	for start := 0; start < len(text); start += mobiTextRecord {
		records = append(records, palmDOC(text[start:min(len(text), start+mobiTextRecord)]))
	}
	textRecords := len(records) - 1
	firstImage := uint32(0xFFFFFFFF)
	if b.Cover != nil || len(b.Images) > 0 {
		firstImage = uint32(len(records))
	}
	if b.Cover != nil {
		records = append(records, b.Cover)
	}
	records = append(records, b.Images...)
	exth := exthBlock(map[uint32][]byte{100: []byte(b.Author), 503: []byte(b.Title)}, b.Cover != nil)

	var r0 bytes.Buffer
	be := func(v any) { binary.Write(&r0, binary.BigEndian, v) }
	encryption := uint16(0)
	if b.Encrypted {
		encryption = 2
	}
	// PalmDOC header: compression 2 (PalmDOC), text length, record count,
	// record size, encryption.
	be(uint16(2))
	be(uint16(0))
	be(uint32(len(text)))
	be(uint16(textRecords))
	be(uint16(mobiTextRecord))
	be(encryption)
	be(uint16(0))
	// MOBI header, with every field not set here unused (0xFFFFFFFF or 0).
	mobi := make([]byte, mobiHeaderLen)
	for i := 0; i < len(mobi); i += 4 {
		binary.BigEndian.PutUint32(mobi[i:], 0xFFFFFFFF)
	}
	put := func(offset int, v uint32) { binary.BigEndian.PutUint32(mobi[offset-16:], v) }
	copy(mobi, "MOBI")
	put(20, mobiHeaderLen)
	put(24, 2) // a book
	put(28, mobiUTF8)
	put(32, 1)                                  // unique ID
	put(36, 6)                                  // file version
	put(84, uint32(16+mobiHeaderLen+len(exth))) // full name offset
	put(88, uint32(len(b.Title)))               // full name length
	put(92, 9)                                  // English
	put(104, 6)                                 // minimum reader version
	put(108, firstImage)
	put(112, 0) // no HUFF/CDIC compression
	put(116, 0)
	put(128, 0x40) // an EXTH block follows
	put(240, 0)    // no trailing entries after the text of a record
	r0.Write(mobi)
	r0.Write(exth)
	r0.WriteString(b.Title)
	r0.Write(make([]byte, 4-r0.Len()%4))
	records[0] = r0.Bytes()

	return File(t, "book.mobi", palmDatabase(strings.ReplaceAll(b.Title, " ", "_"), "BOOKMOBI", records))
}

// PalmDOC describes a PalmDOC book to write: plain text in a Palm database,
// the format of older .prc files, with the title as the database name.
type PalmDOC struct {
	Title string
	Text  string // in Latin-1, as PalmDOC readers take it
}

// Write writes the book into a new temporary directory of t and returns the
// path of the .prc file.
func (b PalmDOC) Write(t testing.TB) string {
	t.Helper()
	records := [][]byte{nil}
	for start := 0; start < len(b.Text); start += mobiTextRecord {
		records = append(records, palmDOC([]byte(b.Text[start:min(len(b.Text), start+mobiTextRecord)])))
	}
	var r0 bytes.Buffer
	for _, v := range []any{uint16(2), uint16(0), uint32(len(b.Text)), uint16(len(records) - 1), uint16(mobiTextRecord), uint32(0)} {
		binary.Write(&r0, binary.BigEndian, v)
	}
	records[0] = r0.Bytes()
	return File(t, "book.prc", palmDatabase(b.Title, "TEXtREAd", records))
}

// palmDatabase returns a Palm database of the kind (type and creator) with
// the records.
func palmDatabase(name, kind string, records [][]byte) []byte {
	var out bytes.Buffer
	header := make([]byte, 78)
	copy(header[:31], name)
	copy(header[60:], kind)
	binary.BigEndian.PutUint16(header[76:], uint16(len(records)))
	out.Write(header)
	offset := uint32(len(header) + 8*len(records) + 2)
	for i, rec := range records {
		binary.Write(&out, binary.BigEndian, offset)
		binary.Write(&out, binary.BigEndian, uint32(2*i)) // attributes and unique ID
		offset += uint32(len(rec))
	}
	out.Write([]byte{0, 0})
	for _, rec := range records {
		out.Write(rec)
	}
	return out.Bytes()
}

// ImageIndex returns the recindex attribute of an img element that shows
// Images[i]: the place of its record among the image records, from 1.
func (b MOBI) ImageIndex(i int) string {
	if b.Cover != nil {
		i++
	}
	return fmt.Sprintf("%05d", i+1)
}

// palmDOC compresses text with PalmDOC compression, literals only: bytes 1
// to 8 and from 0x80 are escaped with a length byte of 1.
func palmDOC(text []byte) []byte {
	out := make([]byte, 0, len(text)+len(text)/4)
	for _, c := range text {
		if (c >= 1 && c <= 8) || c >= 0x80 {
			out = append(out, 1)
		}
		out = append(out, c)
	}
	return out
}

// exthBlock returns an EXTH block with the string records of values and,
// when cover is set, a cover record pointing at the first image.
func exthBlock(values map[uint32][]byte, cover bool) []byte {
	var recs bytes.Buffer
	count := 0
	add := func(kind uint32, data []byte) {
		binary.Write(&recs, binary.BigEndian, kind)
		binary.Write(&recs, binary.BigEndian, uint32(8+len(data)))
		recs.Write(data)
		count++
	}
	for _, kind := range []uint32{100, 503} {
		if len(values[kind]) > 0 {
			add(kind, values[kind])
		}
	}
	if cover {
		add(201, []byte{0, 0, 0, 0})
	}
	var out bytes.Buffer
	out.WriteString("EXTH")
	size := 12 + recs.Len()
	pad := (4 - size%4) % 4
	binary.Write(&out, binary.BigEndian, uint32(size))
	binary.Write(&out, binary.BigEndian, uint32(count))
	out.Write(recs.Bytes())
	out.Write(make([]byte, pad))
	return out.Bytes()
}

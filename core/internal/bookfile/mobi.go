package bookfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// A MOBI or PRC book is a Palm database: a header, a list of records and the
// records. Record 0 holds the PalmDOC header, which in a MOBI book is
// followed by the MOBI header and, optionally, the EXTH block of metadata.
// The layout is documented at https://wiki.mobileread.com/wiki/PDB and
// https://wiki.mobileread.com/wiki/MOBI.

// Database types, at offset 60 of the file.
const (
	typeMOBI    = "BOOKMOBI" // Mobipocket
	typePalmDOC = "TEXtREAd" // PalmDOC, usually a .prc or .pdb file
)

const (
	headerSize     = 78       // the database header, before the record list
	maxRecordSize  = 32 << 20 // largest record read, as the file may come from anywhere
	mobiHeaderBase = 16       // offset of the MOBI header in record 0
	exthFlag       = 0x40     // bit of the MOBI header's flags set when EXTH follows
	encodingUTF8   = 65001    // text encoding of UTF-8 books; others are 1252 or Latin-1
	noImage        = 0xFFFFFFFF
)

// EXTH record types read for metadata.
const (
	exthAuthor    = 100
	exthCover     = 201 // index of the cover among the image records
	exthThumbnail = 202 // index of a small cover among the image records
	exthTitle     = 503
)

// isPalmBook reports whether head starts a Palm database of a MOBI or
// PalmDOC book.
func isPalmBook(head []byte) bool {
	if len(head) < 68 {
		return false
	}
	kind := string(head[60:68])
	return kind == typeMOBI || kind == typePalmDOC
}

// palmDB is an open Palm database.
type palmDB struct {
	r       io.ReaderAt
	size    int64
	name    string   // the database name, up to 31 bytes
	kind    string   // typeMOBI or typePalmDOC
	offsets []uint32 // where each record starts
}

// readPalmDB reads the header and the record list of the database in r.
func readPalmDB(r io.ReaderAt, size int64) (*palmDB, error) {
	header := make([]byte, headerSize)
	if _, err := r.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("read palm database header: %w", err)
	}
	count := int(binary.BigEndian.Uint16(header[76:78]))
	if count == 0 {
		return nil, errors.New("palm database has no records")
	}
	list := make([]byte, 8*count)
	if _, err := r.ReadAt(list, headerSize); err != nil {
		return nil, fmt.Errorf("read palm database records: %w", err)
	}
	db := &palmDB{r: r, size: size, kind: string(header[60:68])}
	name, _, _ := bytes.Cut(header[:32], []byte{0})
	db.name = string(name)
	for i := range count {
		db.offsets = append(db.offsets, binary.BigEndian.Uint32(list[8*i:]))
	}
	return db, nil
}

// record returns record i, or false when it does not exist or is out of
// bounds.
func (db *palmDB) record(i int) ([]byte, bool) {
	if i < 0 || i >= len(db.offsets) {
		return nil, false
	}
	start, end := int64(db.offsets[i]), db.size
	if i+1 < len(db.offsets) {
		end = int64(db.offsets[i+1])
	}
	if start < headerSize || end > db.size || start >= end || end-start > maxRecordSize {
		return nil, false
	}
	data := make([]byte, end-start)
	if _, err := db.r.ReadAt(data, start); err != nil {
		return nil, false
	}
	return data, true
}

// mobiHeader is what is read from record 0 of a MOBI or PalmDOC book.
type mobiHeader struct {
	encrypted  bool
	encoding   uint32 // encodingUTF8, 1252, or else Latin-1
	fullName   []byte
	firstImage uint32 // record index of the first image, or noImage
	exth       map[uint32][][]byte
}

// readMOBIHeader reads record 0 of db.
func readMOBIHeader(db *palmDB) (mobiHeader, error) {
	h := mobiHeader{firstImage: noImage}
	rec, ok := db.record(0)
	if !ok || len(rec) < 16 {
		return h, errors.New("mobi: no PalmDOC header")
	}
	if db.kind != typeMOBI {
		// A PalmDOC header ends with the reading position where a MOBI
		// header has the encryption type.
		return h, nil
	}
	h.encrypted = binary.BigEndian.Uint16(rec[12:14]) != 0
	if len(rec) < mobiHeaderBase+8 || string(rec[16:20]) != "MOBI" {
		return h, nil
	}
	headerLen := int(binary.BigEndian.Uint32(rec[20:24]))
	field := func(offset int) (uint32, bool) {
		if offset+4 > len(rec) || offset+4 > mobiHeaderBase+headerLen {
			return 0, false
		}
		return binary.BigEndian.Uint32(rec[offset:]), true
	}
	h.encoding, _ = field(28)
	if off, ok := field(84); ok {
		if n, ok := field(88); ok && uint64(off)+uint64(n) <= uint64(len(rec)) {
			h.fullName = rec[off : off+n]
		}
	}
	if first, ok := field(108); ok {
		h.firstImage = first
	}
	if flags, ok := field(128); ok && flags&exthFlag != 0 {
		h.exth = readEXTH(rec[min(len(rec), mobiHeaderBase+headerLen):])
	}
	return h, nil
}

// readEXTH reads the records of the EXTH block at the start of data, by
// type. It returns what it could read of a damaged block.
func readEXTH(data []byte) map[uint32][][]byte {
	if len(data) < 12 || string(data[:4]) != "EXTH" {
		return nil
	}
	out := map[uint32][][]byte{}
	count := binary.BigEndian.Uint32(data[8:12])
	rest := data[12:]
	for range count {
		if len(rest) < 8 {
			break
		}
		kind, size := binary.BigEndian.Uint32(rest[:4]), binary.BigEndian.Uint32(rest[4:8])
		if size < 8 || uint64(size) > uint64(len(rest)) {
			break
		}
		out[kind] = append(out[kind], rest[8:size])
		rest = rest[size:]
	}
	return out
}

// text decodes a string of the book, which is in the book's text encoding.
func (h mobiHeader) text(data []byte) string {
	if h.encoding == encodingUTF8 && utf8.Valid(data) {
		return clean(string(data))
	}
	m := charmap.ISO8859_1
	if h.encoding == 1252 {
		m = charmap.Windows1252
	}
	s, err := m.NewDecoder().Bytes(data)
	if err != nil {
		return ""
	}
	return clean(string(s))
}

// firstEXTH returns the first EXTH record of type kind, decoded, or "".
func (h mobiHeader) firstEXTH(kind uint32) string {
	for _, v := range h.exth[kind] {
		if s := h.text(v); s != "" {
			return s
		}
	}
	return ""
}

// image returns the image record the EXTH record of type kind points to.
func (h mobiHeader) image(db *palmDB, kind uint32) []byte {
	if h.firstImage == noImage {
		return nil
	}
	for _, v := range h.exth[kind] {
		if len(v) != 4 {
			continue
		}
		index := uint64(h.firstImage) + uint64(binary.BigEndian.Uint32(v))
		if index >= uint64(len(db.offsets)) {
			continue
		}
		if rec, ok := db.record(int(index)); ok && isImage(rec) {
			return rec
		}
	}
	return nil
}

// isImage reports whether data starts like an image MuPDF can decode.
func isImage(data []byte) bool {
	for _, magic := range []string{"\xFF\xD8\xFF", "\x89PNG\r\n\x1A\n", "GIF87a", "GIF89a", "BM"} {
		if bytes.HasPrefix(data, []byte(magic)) {
			return true
		}
	}
	return false
}

// openMOBI opens the database at path and reads its header. It returns
// ErrDRM for an encrypted book.
func openMOBI(path string, use func(*palmDB, mobiHeader)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	db, err := readPalmDB(f, st.Size())
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	h, err := readMOBIHeader(db)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if h.encrypted {
		return fmt.Errorf("open %s: %w", path, ErrDRM)
	}
	if use != nil {
		use(db, h)
	}
	return nil
}

// checkMOBI checks that the MOBI or PalmDOC book at path is not protected
// by DRM, which MuPDF does not look at: it would show the encrypted text.
func checkMOBI(path string) error {
	return openMOBI(path, nil)
}

// inspectMOBI reads the title, the author and the cover of a MOBI book from
// its EXTH metadata, and the title of a PalmDOC book from its database name.
func inspectMOBI(path string, info *Info) error {
	return openMOBI(path, func(db *palmDB, h mobiHeader) {
		if db.kind == typePalmDOC {
			info.Title = h.text([]byte(db.name))
			return
		}
		info.Title = h.firstEXTH(exthTitle)
		if info.Title == "" {
			info.Title = h.text(h.fullName)
		}
		info.Author = h.firstEXTH(exthAuthor)
		if info.Cover = h.image(db, exthCover); info.Cover == nil {
			info.Cover = h.image(db, exthThumbnail)
		}
	})
}

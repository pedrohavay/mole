package testbook

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
)

// PDF describes a PDF to write, with pages of text set in Helvetica,
// pictures and vector drawings, an outline and, optionally, a password.
type PDF struct {
	Title, Author string
	Language      string // the /Lang of the catalog, left out when empty
	Pages         []PDFPage
	Outline       []PDFEntry
	// UserPassword and OwnerPassword encrypt the file with RC4 (revision
	// 3 of the standard security handler) when either is set. A file with
	// only an owner password opens without one.
	UserPassword, OwnerPassword string
}

// PDFPage is a page of a PDF.
type PDFPage struct {
	Width, Height float64 // in points; zero for US Letter, 612 x 792
	// Lines are lines of text, in 12 point Helvetica from the top left
	// corner, one inch in.
	Lines []string
	// Image is drawn at ImageRect: x and y of its lower left corner, from
	// the bottom left of the page, then its width and height, in points.
	Image     image.Image
	ImageRect [4]float64
	// Drawing holds content stream operators drawn before the text, such
	// as "0 0 1 RG 72 600 m 540 600 l S".
	Drawing string
}

// PDFEntry is an entry of the outline: a title that goes to a page.
type PDFEntry struct {
	Title string
	Page  int
	Level int // 0 for top-level entries, 1 for those under the entry before
}

// Write writes the PDF into a new temporary directory of t and returns its
// path.
func (p PDF) Write(t testing.TB) string {
	t.Helper()
	return File(t, "book.pdf", p.Bytes())
}

// Bytes returns the PDF file.
func (p PDF) Bytes() []byte {
	w := &pdfWriter{}
	w.id = md5.Sum([]byte(p.Title + p.Author + fmt.Sprint(len(p.Pages))))
	if p.UserPassword != "" || p.OwnerPassword != "" {
		w.encrypt(p.UserPassword, p.OwnerPassword)
	}
	const catalog, pages, font, info = 1, 2, 3, 4
	w.next = 5
	encryptObj := 0
	if w.key != nil {
		encryptObj = w.alloc()
	}
	pageObjs := make([]int, len(p.Pages))
	for i := range p.Pages {
		pageObjs[i] = w.alloc()
	}
	outlineObj := 0
	if len(p.Outline) > 0 {
		outlineObj = w.alloc()
	}

	root := "<< /Type /Catalog /Pages 2 0 R"
	if outlineObj != 0 {
		root += fmt.Sprintf(" /Outlines %d 0 R", outlineObj)
	}
	if p.Language != "" {
		root += " /Lang " + w.str(catalog, p.Language)
	}
	w.object(catalog, root+" >>")
	var kids strings.Builder
	for _, obj := range pageObjs {
		fmt.Fprintf(&kids, "%d 0 R ", obj)
	}
	w.object(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(p.Pages)))
	w.object(font, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	w.object(info, fmt.Sprintf("<< /Title %s /Author %s >>", w.str(info, p.Title), w.str(info, p.Author)))
	for i, page := range p.Pages {
		w.page(pageObjs[i], page)
	}
	if outlineObj != 0 {
		w.outline(outlineObj, p.Outline, pageObjs, p.Pages)
	}
	if encryptObj != 0 {
		w.object(encryptObj, fmt.Sprintf("<< /Filter /Standard /V 2 /R 3 /Length 128 /O <%x> /U <%x> /P %d >>",
			w.owner, w.user, pdfPermissions))
	}
	return w.finish(catalog, info, encryptObj)
}

// pdfWriter assembles the objects of a PDF and the table of their offsets.
type pdfWriter struct {
	buf     bytes.Buffer
	next    int         // number of the next object to allocate
	offsets map[int]int // where each object starts
	id      [16]byte    // the first part of the file's /ID
	key     []byte      // the file key, when encrypted
	owner   []byte      // /O and /U of the encryption dictionary
	user    []byte
	written bool
}

func (w *pdfWriter) alloc() int {
	w.next++
	return w.next - 1
}

func (w *pdfWriter) header() {
	if !w.written {
		w.buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
		w.offsets = map[int]int{}
		w.written = true
	}
}

// object writes object num with body.
func (w *pdfWriter) object(num int, body string) {
	w.header()
	w.offsets[num] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n%s\nendobj\n", num, body)
}

// stream writes object num as a stream of data, compressed, with more
// entries in its dictionary.
func (w *pdfWriter) stream(num int, dict string, data []byte) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(data)
	zw.Close()
	body := w.crypt(num, z.Bytes())
	w.header()
	w.offsets[num] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n<< %s /Filter /FlateDecode /Length %d >>\nstream\n", num, dict, len(body))
	w.buf.Write(body)
	w.buf.WriteString("\nendstream\nendobj\n")
}

// str returns s as a PDF string of object num, encrypted when the file is.
func (w *pdfWriter) str(num int, s string) string {
	return "<" + hex.EncodeToString(w.crypt(num, []byte(s))) + ">"
}

func (w *pdfWriter) page(num int, page PDFPage) {
	width, height := page.Width, page.Height
	if width == 0 || height == 0 {
		width, height = 612, 792
	}
	var content strings.Builder
	resources := "/Font << /F1 3 0 R >>"
	if page.Drawing != "" {
		content.WriteString("q\n" + page.Drawing + "\nQ\n")
	}
	if page.Image != nil {
		img := w.alloc()
		w.image(img, page.Image)
		resources += fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", img)
		r := page.ImageRect
		fmt.Fprintf(&content, "q %g 0 0 %g %g %g cm /Im1 Do Q\n", r[2], r[3], r[0], r[1])
	}
	if len(page.Lines) > 0 {
		fmt.Fprintf(&content, "BT /F1 12 Tf 14 TL 72 %g Td\n", height-72-12)
		for _, line := range page.Lines {
			fmt.Fprintf(&content, "(%s) Tj T*\n", escapePDF(line))
		}
		content.WriteString("ET\n")
	}
	contents := w.alloc()
	w.stream(contents, "", []byte(content.String()))
	w.object(num, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g] /Resources << %s >> /Contents %d 0 R >>",
		width, height, resources, contents))
}

// image writes object num as an image XObject of img, in gray when img is
// gray and in RGB otherwise.
func (w *pdfWriter) image(num int, img image.Image) {
	b := img.Bounds()
	_, gray := img.(*image.Gray)
	var data []byte
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if gray {
				data = append(data, c.R)
			} else {
				data = append(data, c.R, c.G, c.B)
			}
		}
	}
	space := "/DeviceRGB"
	if gray {
		space = "/DeviceGray"
	}
	w.stream(num, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8",
		b.Dx(), b.Dy(), space), data)
}

// outline writes the outline root as object root and an object for each
// entry, nested by level.
func (w *pdfWriter) outline(root int, entries []PDFEntry, pages []int, specs []PDFPage) {
	nums := make([]int, len(entries))
	for i := range entries {
		nums[i] = w.alloc()
	}
	parent := make([]int, len(entries))
	children := map[int][]int{} // by parent object, the entries under it
	var stack []int             // entries open at each level
	for i, e := range entries {
		level := min(e.Level, len(stack))
		stack = stack[:level]
		parent[i] = root
		if level > 0 {
			parent[i] = nums[stack[level-1]]
		}
		children[parent[i]] = append(children[parent[i]], i)
		stack = append(stack, i)
	}
	for i, e := range entries {
		siblings := children[parent[i]]
		body := fmt.Sprintf("<< /Title %s /Parent %d 0 R", w.str(nums[i], e.Title), parent[i])
		for k, s := range siblings {
			if s != i {
				continue
			}
			if k > 0 {
				body += fmt.Sprintf(" /Prev %d 0 R", nums[siblings[k-1]])
			}
			if k < len(siblings)-1 {
				body += fmt.Sprintf(" /Next %d 0 R", nums[siblings[k+1]])
			}
		}
		if kids := children[nums[i]]; len(kids) > 0 {
			body += fmt.Sprintf(" /First %d 0 R /Last %d 0 R /Count %d", nums[kids[0]], nums[kids[len(kids)-1]], len(kids))
		}
		height := 792.0
		if s := specs[e.Page]; s.Height > 0 {
			height = s.Height
		}
		body += fmt.Sprintf(" /Dest [%d 0 R /XYZ 0 %g 0] >>", pages[e.Page], height)
		w.object(nums[i], body)
	}
	top := children[root]
	w.object(root, fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		nums[top[0]], nums[top[len(top)-1]], len(top)))
}

// finish writes the cross-reference table and the trailer and returns the
// file.
func (w *pdfWriter) finish(root, info, encrypt int) []byte {
	start := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", w.next)
	for num := 1; num < w.next; num++ {
		fmt.Fprintf(&w.buf, "%010d 00000 n \n", w.offsets[num])
	}
	trailer := fmt.Sprintf("<< /Size %d /Root %d 0 R /Info %d 0 R /ID [<%x> <%x>]", w.next, root, info, w.id, w.id)
	if encrypt != 0 {
		trailer += fmt.Sprintf(" /Encrypt %d 0 R", encrypt)
	}
	fmt.Fprintf(&w.buf, "trailer\n%s >>\nstartxref\n%d\n%%%%EOF\n", trailer, start)
	return w.buf.Bytes()
}

// pdfPermissions allows everything but the reserved bits.
const pdfPermissions = -4

// pdfPadding pads passwords, as the standard security handler does.
var pdfPadding = []byte("\x28\xBF\x4E\x5E\x4E\x75\x8A\x41\x64\x00\x4E\x56\xFF\xFA\x01\x08" +
	"\x2E\x2E\x00\xB6\xD0\x68\x3E\x80\x2F\x0C\xA9\xFE\x64\x53\x69\x7A")

func padPassword(password string) []byte {
	return append([]byte(password), pdfPadding...)[:32]
}

// encrypt computes the file key and the /O and /U entries of the standard
// security handler, revision 3, with a 128-bit key (algorithms 2, 3 and 5
// of ISO 32000-1).
func (w *pdfWriter) encrypt(userPassword, ownerPassword string) {
	if ownerPassword == "" {
		ownerPassword = userPassword
	}
	h := md5.Sum(padPassword(ownerPassword))
	for range 50 {
		h = md5.Sum(h[:])
	}
	w.owner = rc4Rounds(h[:], padPassword(userPassword))

	var perms [4]byte
	permissions := int32(pdfPermissions)
	binary.LittleEndian.PutUint32(perms[:], uint32(permissions))
	k := md5.New()
	k.Write(padPassword(userPassword))
	k.Write(w.owner)
	k.Write(perms[:])
	k.Write(w.id[:])
	key := k.Sum(nil)
	for range 50 {
		sum := md5.Sum(key)
		key = sum[:]
	}
	w.key = key

	u := md5.New()
	u.Write(pdfPadding)
	u.Write(w.id[:])
	w.user = append(rc4Rounds(w.key, u.Sum(nil)), make([]byte, 16)...)
}

// rc4Rounds encrypts data with key and then with key XOR i, for i from 1 to
// 19.
func rc4Rounds(key, data []byte) []byte {
	out := append([]byte(nil), data...)
	k := make([]byte, len(key))
	for i := range 20 {
		for j := range key {
			k[j] = key[j] ^ byte(i)
		}
		c, _ := rc4.NewCipher(k)
		c.XORKeyStream(out, out)
	}
	return out
}

// crypt encrypts data of object num, or returns it as it is when the file
// is not encrypted.
func (w *pdfWriter) crypt(num int, data []byte) []byte {
	if w.key == nil {
		return data
	}
	k := append([]byte(nil), w.key...)
	k = append(k, byte(num), byte(num>>8), byte(num>>16), 0, 0)
	sum := md5.Sum(k)
	c, _ := rc4.NewCipher(sum[:])
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

// escapePDF escapes s for a literal string of a content stream.
func escapePDF(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(s)
}

// SamplePDF returns a book of fixed pages for tests and for trying the
// reader: a cover with a title, three pages of text under an outline, a
// page with a photograph-like picture, a diagram drawn with vector paths
// and a table, and a scanned page that is only a picture of text.
func SamplePDF() PDF {
	paragraph := func(n int) []string {
		var lines []string
		for i := range 8 {
			lines = append(lines, fmt.Sprintf("Mole reads fixed pages line %d of part %d, and the night stays dark.", i+1, n))
		}
		return append(lines, fmt.Sprintf("The burrow keeps the lantern number %d lit.", n), "")
	}
	text := func(title string, parts ...int) []string {
		lines := []string{title, ""}
		for _, n := range parts {
			lines = append(lines, paragraph(n)...)
		}
		return lines
	}
	return PDF{
		Title:    "The Mole in the PDF",
		Author:   "Ada Burrow",
		Language: "en",
		Pages: []PDFPage{
			{
				Lines:   []string{"", "", "", "", "The Mole in the PDF", "", "Ada Burrow"},
				Drawing: "0.1 0.3 0.7 rg 72 520 468 8 re f",
			},
			{Lines: text("Chapter One: Tunnels", 1, 2, 3)},
			{Lines: text("Chapter Two: Lanterns", 4, 5, 6)},
			{Lines: text("Chapter Three: Maps", 7, 8, 9)},
			{
				Lines:     []string{"Figure 1. A photograph, a diagram and a table."},
				Image:     samplePhoto(),
				ImageRect: [4]float64{72, 420, 220, 220},
				// The page's white paper, a diagram of boxes and arrows in
				// black and red, and a table with ruled and shaded rows.
				Drawing: "1 1 1 rg 0 0 612 792 re f " +
					"0 0 0 RG 2 w 330 600 100 40 re S 450 600 100 40 re S 430 620 m 450 620 l S " +
					"0.9 0.1 0.1 RG 380 600 m 380 520 l S 0.9 0.1 0.1 rg 375 525 m 385 525 l 380 515 l f " +
					"0.85 0.85 0.85 rg 72 300 468 24 re f 0 0 0 RG 1 w " +
					"72 348 m 540 348 l S 72 324 m 540 324 l S 72 276 m 540 276 l S 72 252 m 540 252 l S",
			},
			{Image: sampleScan(), ImageRect: [4]float64{0, 0, 612, 792}},
		},
		Outline: []PDFEntry{
			{Title: "Cover", Page: 0},
			{Title: "Chapter One: Tunnels", Page: 1},
			{Title: "Chapter Two: Lanterns", Page: 2},
			{Title: "Chapter Three: Maps", Page: 3},
			{Title: "Figures", Page: 4, Level: 1},
			{Title: "Scanned Page", Page: 5},
		},
	}
}

// samplePhoto returns a picture with the tones of a photograph: a gradient
// of colors with little white.
func samplePhoto() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: uint8(40 + 3*x), G: uint8(30 + 2*y), B: uint8(120 - y), A: 255})
		}
	}
	return img
}

// sampleScan returns a gray picture of a page of text, as a scanner makes
// it: white paper with dark bars for lines of words.
func sampleScan() image.Image {
	img := image.NewGray(image.Rect(0, 0, 306, 396))
	for i := range img.Pix {
		img.Pix[i] = 250
	}
	for line := range 24 {
		y0 := 40 + line*13
		for x := 36; x < 270; x++ {
			if (x/9)%5 == 4 { // the space between words
				continue
			}
			for y := y0; y < y0+6; y++ {
				img.Pix[y*img.Stride+x] = 30
			}
		}
	}
	return img
}

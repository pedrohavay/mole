// Package mupdf wraps the parts of MuPDF the reader needs: reflowing an EPUB
// to a page size, rendering pages as coverage masks with color and image
// layers or as PNG images, the images of a page, positions that survive a
// relayout and the text of a page.
//
// Every Document has its own MuPDF context, so different documents may be
// used on different goroutines at the same time.
//
// Documents find the fonts installed on the Mac that their styles name, which
// MuPDF alone does not (see FindFont).
//
// It links the static MuPDF libraries. Where they are comes from CGO_CFLAGS
// (-I) and CGO_LDFLAGS (-L), which the Makefile sets from MUPDF_PREFIX; run
// Go commands through make, or set those two variables yourself. A program
// that links the package as a C archive must also link CoreText.
package mupdf

/*
#cgo LDFLAGS: -lmupdf -lmupdf-third -lm -framework CoreText -framework CoreFoundation
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"strings"
	"unsafe"
)

const errLen = 512

// Document is an open MuPDF document. It is not safe for concurrent use, but
// separate documents, even of the same file, are independent.
type Document struct {
	d        *C.md_doc
	lineDist float64 // see SetLineDistance
}

// memoryLimit is what one document may allocate, so that a decompression
// bomb fails to open or lay out instead of exhausting memory.
const memoryLimit = 1 << 30

// Open opens the document at path. userCSS, when not empty, is applied on top
// of the document's own styles. The document may allocate at most 1 GiB;
// past that, its methods fail.
func Open(path, userCSS string) (*Document, error) {
	return open(path, userCSS, memoryLimit)
}

func open(path, userCSS string, limit int) (*Document, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var ccss *C.char
	if userCSS != "" {
		ccss = C.CString(userCSS)
		defer C.free(unsafe.Pointer(ccss))
	}
	msg := make([]byte, errLen)
	d := C.md_open(cpath, ccss, C.size_t(limit), cbuf(msg), errLen)
	if d == nil {
		return nil, fmt.Errorf("open %s: %s", path, cstr(msg))
	}
	return &Document{d: d}, nil
}

// Close releases the document.
func (doc *Document) Close() {
	C.md_close(doc.d)
	doc.d = nil
}

// Layout reflows the document to pages of width x height points with a base
// font size of em points and returns the number of pages.
func (doc *Document) Layout(width, height, em float64) (int, error) {
	msg := make([]byte, errLen)
	n := C.md_layout(doc.d, C.float(width), C.float(height), C.float(em), cbuf(msg), errLen)
	if n < 0 {
		return 0, fmt.Errorf("layout: %s", cstr(msg))
	}
	return int(n), nil
}

// Pictures says how RenderLayers draws images.
type Pictures int

// The ways RenderLayers draws images.
const (
	// PicturesNight draws photographs and dark pictures as they are and
	// line art (mostly light images, such as diagrams) with its lightness
	// inverted and its hues kept, so that its lines glow instead of its
	// paper.
	PicturesNight Pictures = iota
	// PicturesOriginal draws every image as on a white page.
	PicturesOriginal
	// PicturesHidden draws no image.
	PicturesHidden
)

// Layers are the buffers RenderLayers draws a page into: regions of Width x
// Height pixels that start at pixel Offset and have rows of Stride pixels.
// Text has one byte per pixel. Colors and Images have four, RGBA in sRGB
// with premultiplied alpha, and must be all zero where the page goes;
// either may be nil to draw nothing into it.
type Layers struct {
	Text, Colors, Images []byte
	Offset               int
	Width, Height        int
	Stride               int
}

// Drawn reports which of the optional layers a page drew into.
type Drawn struct {
	Colors bool // text or lines in a hue
	Images bool // images or shadings
}

// RenderLayers draws page into l. Each byte of Text is the coverage of
// text, in full ink whatever its color, and of vector lines: 0 for empty
// paper, 255 for full ink. Filled boxes (CSS backgrounds) are left out, so a
// dark box neither glows nor hides its text. Colors holds the hue of text
// and lines that have one, lightened where it is dark so that it shows on
// a black page; black, white and gray text is left out of it, for the
// reader's tint. Images holds the images and shadings on a black page, as
// pictures says.
func (doc *Document) RenderLayers(page int, scale float64, l Layers, pictures Pictures) (Drawn, error) {
	if l.Width <= 0 || l.Height <= 0 {
		return Drawn{}, nil
	}
	end := l.Offset + (l.Height-1)*l.Stride + l.Width
	if l.Offset < 0 || l.Width > l.Stride || end > len(l.Text) ||
		(l.Colors != nil && 4*end > len(l.Colors)) || (l.Images != nil && 4*end > len(l.Images)) {
		return Drawn{}, errors.New("render: region outside the layers")
	}
	if pictures == PicturesHidden {
		l.Images = nil
	}
	// The buffers are referred to from a C struct, so they must not move.
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pin := func(layer []byte, offset int) *C.uchar {
		if layer == nil {
			return nil
		}
		p := &layer[offset]
		pinner.Pin(p)
		return (*C.uchar)(unsafe.Pointer(p))
	}
	layers := C.md_layers{
		text:   pin(l.Text, l.Offset),
		colors: pin(l.Colors, 4*l.Offset),
		images: pin(l.Images, 4*l.Offset),
		width:  C.int(l.Width), height: C.int(l.Height), stride: C.int(l.Stride),
	}
	msg := make([]byte, errLen)
	var colors, images C.int
	status := C.md_render_layers(doc.d, C.int(page), C.float(scale), &layers, C.int(pictures),
		&colors, &images, cbuf(msg), errLen)
	if status != 0 {
		return Drawn{}, fmt.Errorf("render page %d: %s", page, cstr(msg))
	}
	return Drawn{Colors: colors != 0, Images: images != 0}, nil
}

// PageImages returns where page draws its images, in drawing order, as
// boxes in page points. Images inside masks and patterns are left out.
func (doc *Document) PageImages(page int) ([]Rect, error) {
	msg := make([]byte, errLen)
	var boxes *C.md_rect
	n := C.md_page_images(doc.d, C.int(page), &boxes, cbuf(msg), errLen)
	if n < 0 {
		return nil, fmt.Errorf("images of page %d: %s", page, cstr(msg))
	}
	defer C.md_free(unsafe.Pointer(boxes))
	var out []Rect
	for _, b := range unsafe.Slice(boxes, int(n)) {
		out = append(out, Rect{X0: float64(b.x0), Y0: float64(b.y0), X1: float64(b.x1), Y1: float64(b.y1)})
	}
	return out, nil
}

// ImagePixels draws an image of page, the one at index in PageImages, as
// it looks on white paper: at its own resolution, or maxSide pixels on its
// longer side for a drawing that scales, and never more than maxSide. The
// result is opaque.
func (doc *Document) ImagePixels(page, index, maxSide int) (*image.RGBA, error) {
	msg := make([]byte, errLen)
	var w, h C.int
	data := C.md_image_pixels(doc.d, C.int(page), C.int(index), C.int(maxSide), &w, &h, cbuf(msg), errLen)
	if data == nil {
		return nil, fmt.Errorf("image %d of page %d: %s", index, page, cstr(msg))
	}
	defer C.md_free(unsafe.Pointer(data))
	return &image.RGBA{
		Pix:    C.GoBytes(unsafe.Pointer(data), 4*w*h),
		Stride: 4 * int(w),
		Rect:   image.Rect(0, 0, int(w), int(h)),
	}, nil
}

// RenderPNG draws page in color and returns it encoded as PNG.
func (doc *Document) RenderPNG(page int, scale float64) ([]byte, error) {
	msg := make([]byte, errLen)
	var n C.size_t
	data := C.md_render_png(doc.d, C.int(page), C.float(scale), &n, cbuf(msg), errLen)
	if data == nil {
		return nil, fmt.Errorf("render page %d as png: %s", page, cstr(msg))
	}
	defer C.md_free(unsafe.Pointer(data))
	return C.GoBytes(unsafe.Pointer(data), C.int(n)), nil
}

// ChapterPosition returns the chapter that holds page and how far into that
// chapter the page is, from 0 to 1. It maps back to the same place (within a
// page) in any layout, also after the document is reopened.
func (doc *Document) ChapterPosition(page int) (chapter int, fraction float64, ok bool) {
	var c C.int
	var f C.float
	if C.md_chapter_position(doc.d, C.int(page), &c, &f) != 0 {
		return 0, 0, false
	}
	return int(c), float64(f), true
}

// PageAt returns the page at fraction of chapter in the current layout. It
// reports false when the chapter does not exist.
func (doc *Document) PageAt(chapter int, fraction float64) (int, bool) {
	page := int(C.md_page_at(doc.d, C.int(chapter), C.float(fraction)))
	return page, page >= 0
}

// Metadata keys understood by Metadata.
const (
	MetaTitle  = "info:Title"
	MetaAuthor = "info:Author"
)

// Metadata returns a metadata value of the document, or "" if there is none.
func (doc *Document) Metadata(key string) string {
	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))
	buf := make([]byte, 1024)
	if C.md_metadata(doc.d, ckey, cbuf(buf), C.int(len(buf))) == 0 {
		return ""
	}
	return cstr(buf)
}

// OutlineItem is one entry of the table of contents.
type OutlineItem struct {
	Title string
	URI   string // link target, resolved to a page with Resolve
	Level int    // 0 for top-level entries
}

// Outline returns the table of contents in reading order.
func (doc *Document) Outline() ([]OutlineItem, error) {
	msg := make([]byte, errLen)
	var items *C.md_outline_item
	n := C.md_outline(doc.d, &items, cbuf(msg), errLen)
	if n < 0 {
		return nil, fmt.Errorf("outline: %s", cstr(msg))
	}
	defer C.md_free_outline(items, n)
	out := make([]OutlineItem, 0, int(n))
	for _, item := range unsafe.Slice(items, int(n)) {
		out = append(out, OutlineItem{
			Title: strings.Join(strings.Fields(C.GoString(item.title)), " "),
			URI:   C.GoString(item.uri),
			Level: int(item.level),
		})
	}
	return out, nil
}

// Resolve returns the page a link URI points to in the current layout. It
// reports false when the link goes nowhere in the document.
func (doc *Document) Resolve(uri string) (int, bool) {
	curi := C.CString(uri)
	defer C.free(unsafe.Pointer(curi))
	page := int(C.md_resolve(doc.d, curi))
	return page, page >= 0
}

// Chapters returns the number of chapters (for an EPUB, the documents of its
// spine). It returns 0 on error.
func (doc *Document) Chapters() int {
	return max(0, int(C.md_count_chapters(doc.d)))
}

// ChapterPages returns the number of pages of chapter in the current layout.
// It reports false when the chapter does not exist.
func (doc *Document) ChapterPages(chapter int) (int, bool) {
	pages := int(C.md_chapter_pages(doc.d, C.int(chapter)))
	return pages, pages >= 0
}

// Location returns the chapter that holds page and the index of the page
// within that chapter, counting from 0.
func (doc *Document) Location(page int) (chapter, index int, ok bool) {
	var c, i C.int
	if C.md_location(doc.d, C.int(page), &c, &i) != 0 {
		return 0, 0, false
	}
	return int(c), int(i), true
}

// PageNumber returns the page at index within chapter in the current layout.
// It reports false when there is no such page.
func (doc *Document) PageNumber(chapter, index int) (int, bool) {
	page := int(C.md_page_number(doc.d, C.int(chapter), C.int(index)))
	return page, page >= 0
}

// Rect is a rectangle in page points, with y growing down from the top of
// the page.
type Rect struct {
	X0, Y0, X1, Y1 float64
}

// Char is a character of a page's text. Ligatures are expanded into their
// letters and horizontal whitespace is reported as spaces.
type Char struct {
	Rune  rune
	Box   Rect
	Line  int // index of the line on the page, counting from 0
	Block int // index of the paragraph on the page, counting from 0 (see SetLineDistance)
}

// SetLineDistance tells PageChars how far apart, in ems, the baselines of two
// lines of one paragraph may be. MuPDF starts a new paragraph past 1.5 em,
// which a generous line height passes; a larger distance keeps such lines
// together. Smaller values, the zero value included, keep MuPDF's.
func (doc *Document) SetLineDistance(ems float64) {
	doc.lineDist = ems
}

// PageChars returns the text of page in reading order.
func (doc *Document) PageChars(page int) ([]Char, error) {
	msg := make([]byte, errLen)
	var chars *C.md_char
	n := C.md_page_chars(doc.d, C.int(page), C.float(doc.lineDist), &chars, cbuf(msg), errLen)
	if n < 0 {
		return nil, fmt.Errorf("text of page %d: %s", page, cstr(msg))
	}
	defer C.md_free(unsafe.Pointer(chars))
	out := make([]Char, int(n))
	for i, c := range unsafe.Slice(chars, int(n)) {
		out[i] = Char{
			Rune:  rune(c.c),
			Box:   Rect{X0: float64(c.x0), Y0: float64(c.y0), X1: float64(c.x1), Y1: float64(c.y1)},
			Line:  int(c.line),
			Block: int(c.block),
		}
	}
	return out, nil
}

// FontStyle is a face of a font family: bold or not, italic or not.
type FontStyle struct {
	Bold, Italic bool
}

// FindFont returns the file that holds the face of an installed font family
// closest to style, as CoreText matches it, and the index of that face in the
// file: 0 unless the file is a collection (.ttc). It reports false when no
// family of that name is installed, and always for the CSS generic families
// (serif, sans-serif, monospace, ...), which MuPDF has fonts of its own for.
func FindFont(family string, style FontStyle) (path string, index int, ok bool) {
	cfamily := C.CString(family)
	defer C.free(unsafe.Pointer(cfamily))
	buf := make([]byte, 1024)
	var cindex C.int
	if C.md_find_font(cfamily, cbool(style.Bold), cbool(style.Italic), cbuf(buf), C.int(len(buf)), &cindex) != 0 {
		return "", 0, false
	}
	return cstr(buf), int(cindex), true
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func cbuf(b []byte) *C.char {
	return (*C.char)(unsafe.Pointer(&b[0]))
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

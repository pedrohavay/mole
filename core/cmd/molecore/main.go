// Molecore exposes the reader's core (library, import, layout and rendering of
// pages into an EDR coverage mask with layers for colors and pictures, text
// selection, search, bookmarks and highlights) as a C API, built as a static
// library for a native user interface written in another language:
//
//	go build -buildmode=c-archive -o libmolecore.a ./cmd/molecore
//
// Objects cross the boundary as int64 handles. Structured results are JSON
// strings, and every string or buffer returned to C must be released with
// mole_free, except the layers a session renders, which it owns. Functions
// that fail return 0 (or a null pointer) and leave a message for
// mole_last_error, which belongs to the calling thread.
//
// Threads: the library functions (mole_library_*, import included) and the
// search functions (mole_search_*) may be called from any thread, one call
// at a time per handle. A session (mole_session_*) must be used from one
// thread at a time, normally the main thread; searches it starts run on
// threads of their own. mole_last_error and mole_free work on any thread.
//
// Places in the text are JSON anchors, {"chapter":int,"offset":int}, which do
// not depend on the layout (see reading.Anchor), and ranges of text are
// {"start":anchor,"end":anchor} with the end exclusive. Rectangles are
// {"x":float,"y":float,"width":float,"height":float} in points of the view
// given to mole_session_layout, from its top left corner.
package main

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// Defined in lasterror.c.
void molecore_set_error(const char *msg);
char *molecore_copy_error(void);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"mole/core/internal/importer"
	"mole/core/internal/library"
	"mole/core/internal/mupdf"
	"mole/core/internal/reading"
	"mole/core/internal/session"
)

// main is required by -buildmode=c-archive and never runs.
func main() {}

// handles maps the int64 values given to C to Go objects; C cannot hold Go
// pointers.
type handles[T any] struct {
	mu   sync.Mutex
	next int64
	m    map[int64]T
}

func (h *handles[T]) add(v T) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = map[int64]T{}
	}
	h.next++
	h.m[h.next] = v
	return h.next
}

func (h *handles[T]) get(id int64) (T, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.m[id]
	return v, ok
}

func (h *handles[T]) remove(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.m, id)
}

// openBook is an open book together with the C buffers its layers are
// copied to, which stay valid until the next render, layout or close.
type openBook struct {
	book   *session.Book
	mask   cBuffer
	colors cBuffer
	images cBuffer
	gray   cBuffer // the image layer in gray, for mole_session_image_mask
}

// cBuffer is memory allocated with C, reused while its size fits.
type cBuffer struct {
	p     unsafe.Pointer
	size  int
	blank bool // all zero
}

// resize makes the buffer size bytes long, all zero if it had another size.
func (b *cBuffer) resize(size int) error {
	if size == 0 {
		return errors.New("molecore: nothing to copy; lay out the book first")
	}
	if b.size == size {
		return nil
	}
	C.free(b.p)
	b.p, b.size = C.calloc(C.size_t(size), 1), 0
	if b.p == nil {
		return errors.New("molecore: out of memory")
	}
	b.size, b.blank = size, true
	return nil
}

// fill copies data into the buffer, growing it as needed.
func (b *cBuffer) fill(data []byte) error {
	if err := b.resize(len(data)); err != nil {
		return err
	}
	C.memcpy(b.p, unsafe.Pointer(&data[0]), C.size_t(len(data)))
	b.blank = false
	return nil
}

// clear makes the buffer size bytes of zero.
func (b *cBuffer) clear(size int) error {
	if err := b.resize(size); err != nil {
		return err
	}
	if !b.blank {
		C.memset(b.p, 0, C.size_t(size))
		b.blank = true
	}
	return nil
}

// fillLayer copies a layer that may be empty (drawn false, layer nil or all
// zero) of size bytes into the buffer.
func (b *cBuffer) fillLayer(layer []byte, drawn bool, size int) error {
	if !drawn || len(layer) != size {
		return b.clear(size)
	}
	return b.fill(layer)
}

func (b *cBuffer) free() {
	C.free(b.p)
	*b = cBuffer{}
}

var (
	stores   handles[*library.Store]
	sessions handles[*openBook]
	searches handles[*reading.Search]
)

// fail records err as the last error of the calling thread. Exported
// functions run on the thread of their C caller, so each caller sees its own
// errors.
func fail(err error) {
	msg := C.CString(err.Error())
	defer C.free(unsafe.Pointer(msg))
	C.molecore_set_error(msg)
}

var errBadHandle = errors.New("molecore: unknown handle")

// mole_last_error returns the message of the last failure on the calling
// thread, or "" if there was none. The caller frees it with mole_free.
//
//export mole_last_error
func mole_last_error() *C.char {
	return C.molecore_copy_error()
}

//export mole_free
func mole_free(p unsafe.Pointer) {
	C.free(p)
}

// parseJSON decodes the JSON string arg, named what in errors, into v and
// reports whether it could.
func parseJSON(arg *C.char, what string, v any) bool {
	if arg == nil {
		fail(fmt.Errorf("molecore: missing %s", what))
		return false
	}
	if err := json.Unmarshal([]byte(C.GoString(arg)), v); err != nil {
		fail(fmt.Errorf("parse %s: %w", what, err))
		return false
	}
	return true
}

func jsonString(v any) *C.char {
	data, err := json.Marshal(v)
	if err != nil {
		fail(err)
		return nil
	}
	return C.CString(string(data))
}

// Library.

//export mole_library_open
func mole_library_open(path *C.char) C.int64_t {
	store, err := library.Open(C.GoString(path))
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(stores.add(store))
}

type bookJSON struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	OriginalPath string  `json:"originalPath"` // the file the book was imported from
	Title        string  `json:"title"`
	Author       string  `json:"author"`
	Fraction     float64 `json:"fraction"`
	OpenedAt     int64   `json:"openedAt"` // Unix seconds, 0 if never opened
	AddedAt      int64   `json:"addedAt"`
}

//export mole_library_books
func mole_library_books(lib C.int64_t) *C.char {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	books, err := store.Books()
	if err != nil {
		fail(err)
		return nil
	}
	out := make([]bookJSON, 0, len(books))
	for _, b := range books {
		item := bookJSON{ID: b.ID, Path: b.Path, OriginalPath: b.OriginalPath, Title: b.Title, Author: b.Author,
			Fraction: b.Position.Fraction, AddedAt: b.AddedAt.Unix()}
		if !b.OpenedAt.IsZero() {
			item.OpenedAt = b.OpenedAt.Unix()
		}
		out = append(out, item)
	}
	return jsonString(out)
}

// mole_library_cover copies a book's encoded cover image into a new buffer and
// stores its length in *length. It returns null when the book has no cover.
//
//export mole_library_cover
func mole_library_cover(lib, bookID C.int64_t, length *C.int64_t) unsafe.Pointer {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	b, err := store.Book(int64(bookID))
	if err != nil {
		fail(err)
		return nil
	}
	if len(b.Cover) == 0 {
		return nil
	}
	*length = C.int64_t(len(b.Cover))
	return C.CBytes(b.Cover)
}

// mole_library_import adds an EPUB and returns the book's ID, or 0. The
// library keeps a copy of the file in a Books directory next to its
// database; importing a file already imported returns the same book without
// reading the file again. It may run on a background thread.
//
//export mole_library_import
func mole_library_import(lib C.int64_t, path *C.char) C.int64_t {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	b, err := importer.Import(store, C.GoString(path), time.Now())
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(b.ID)
}

//export mole_library_remove
func mole_library_remove(lib, bookID C.int64_t) C.int {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	if err := store.Remove(int64(bookID)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// settingsJSON mirrors library.Settings field for field, so the two convert.
type settingsJSON struct {
	Level      float64 `json:"level"`
	EDR        bool    `json:"edr"`
	Tint       int     `json:"tint"`
	Gamma      float64 `json:"gamma"`
	FontSize   float64 `json:"fontSize"`
	Spread     int     `json:"spread"`
	Accent     string  `json:"accent"`
	Pictures   int     `json:"pictures"`   // 0 night, 1 original colors, 2 hidden
	BookColors bool    `json:"bookColors"` // keep the hues of colored text
}

//export mole_library_settings
func mole_library_settings(lib C.int64_t) *C.char {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	s, err := store.Settings()
	if err != nil {
		fail(err)
		return nil
	}
	return jsonString(settingsJSON(s))
}

//export mole_library_save_settings
func mole_library_save_settings(lib C.int64_t, settings *C.char) C.int {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	var s settingsJSON
	if err := json.Unmarshal([]byte(C.GoString(settings)), &s); err != nil {
		fail(fmt.Errorf("parse settings: %w", err))
		return 0
	}
	if err := store.SaveSettings(library.Settings(s)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// Annotations.

type anchorJSON struct {
	Chapter int `json:"chapter"`
	Offset  int `json:"offset"`
}

type rangeJSON struct {
	Start anchorJSON `json:"start"`
	End   anchorJSON `json:"end"`
}

type annotationJSON struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"` // "bookmark" or "highlight"
	Range     rangeJSON `json:"range"`
	Color     int       `json:"color"`
	Note      string    `json:"note"`
	Text      string    `json:"text"`
	CreatedAt int64     `json:"createdAt"` // Unix seconds
	UpdatedAt int64     `json:"updatedAt"`
}

// mole_library_annotations returns the bookmarks and highlights of a book in
// reading order, as a JSON array of
// {"id","kind","range","color","note","text","createdAt","updatedAt"}; kind
// is "bookmark" or "highlight" and the times are Unix seconds. The caller
// frees the result with mole_free.
//
//export mole_library_annotations
func mole_library_annotations(lib, bookID C.int64_t) *C.char {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	list, err := store.Annotations(int64(bookID))
	if err != nil {
		fail(err)
		return nil
	}
	out := make([]annotationJSON, 0, len(list))
	for _, a := range list {
		out = append(out, annotationJSON{
			ID: a.ID, Kind: string(a.Kind),
			Range: rangeJSON{Start: anchorJSON(a.Start), End: anchorJSON(a.End)},
			Color: a.Color, Note: a.Note, Text: a.Text,
			CreatedAt: a.CreatedAt.Unix(), UpdatedAt: a.UpdatedAt.Unix(),
		})
	}
	return jsonString(out)
}

// mole_library_add_annotation adds a bookmark or a highlight to a book and
// returns its ID, or 0. annotation is JSON with "kind", "range", "color",
// "note" and "text"; a bookmark's range starts and ends at the same anchor,
// the first visible character.
//
//export mole_library_add_annotation
func mole_library_add_annotation(lib, bookID C.int64_t, annotation *C.char) C.int64_t {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	var in annotationJSON
	if !parseJSON(annotation, "annotation", &in) {
		return 0
	}
	a, err := store.AddAnnotation(library.Annotation{
		BookID: int64(bookID), Kind: library.AnnotationKind(in.Kind),
		Start: library.Anchor(in.Range.Start), End: library.Anchor(in.Range.End),
		Color: in.Color, Note: in.Note, Text: in.Text,
	}, time.Now())
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(a.ID)
}

// mole_library_update_annotation replaces the color and the note of an
// annotation and returns 1, or 0 on error. changes is JSON
// {"color":int,"note":string}; both are required.
//
//export mole_library_update_annotation
func mole_library_update_annotation(lib, id C.int64_t, changes *C.char) C.int {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	var in struct {
		Color *int    `json:"color"`
		Note  *string `json:"note"`
	}
	if !parseJSON(changes, "annotation changes", &in) {
		return 0
	}
	if in.Color == nil || in.Note == nil {
		fail(errors.New("molecore: annotation changes need both color and note"))
		return 0
	}
	if err := store.UpdateAnnotation(int64(id), *in.Color, *in.Note, time.Now()); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// mole_library_remove_annotation deletes an annotation and returns 1, or 0
// on error.
//
//export mole_library_remove_annotation
func mole_library_remove_annotation(lib, id C.int64_t) C.int {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	if err := store.RemoveAnnotation(int64(id)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// Reading sessions.

// mole_session_open opens a book at the place it was left.
//
//export mole_session_open
func mole_session_open(lib, bookID C.int64_t) C.int64_t {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	book, err := session.Open(store, int64(bookID))
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(sessions.add(&openBook{book: book}))
}

// mole_session_close saves where the book was left and releases it.
//
//export mole_session_close
func mole_session_close(s C.int64_t) {
	ss, ok := sessions.get(int64(s))
	if !ok {
		return
	}
	if err := ss.book.Close(); err != nil {
		fail(err)
	}
	ss.mask.free()
	ss.colors.free()
	ss.images.free()
	ss.gray.free()
	sessions.remove(int64(s))
}

// mole_session_layout reflows the book for a view of width x height points
// with topInset points covered by the toolbar, scale pixels per point, a base
// font size and spread pages side by side (0 automatic, 1 single, 2 double),
// keeping the reader at the same place.
//
//export mole_session_layout
func mole_session_layout(s C.int64_t, width, height, topInset, scale, fontSize C.double, spread C.int) C.int {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	geo := reading.NewGeometry(float64(width), float64(height), float64(topInset), float64(scale), float64(fontSize),
		reading.Spread(spread))
	if err := ss.book.Reading().Layout(geo, float64(fontSize)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// moved returns 1 if the page changed. A failure to save the new place is
// left for mole_last_error without hiding the change.
func moved(changed bool, err error) C.int {
	if err != nil {
		fail(err)
	}
	if !changed {
		return 0
	}
	return 1
}

// mole_session_turn moves by delta screens of pages and returns 1 if the page
// changed. Screens of two pages always start on an even page. The new
// position is saved.
//
//export mole_session_turn
func mole_session_turn(s C.int64_t, delta C.int) C.int {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	return moved(ss.book.Turn(int(delta)))
}

// mole_session_go_to shows the screen that holds page and returns 1 if the
// page changed. The new position is saved.
//
//export mole_session_go_to
func mole_session_go_to(s C.int64_t, page C.int) C.int {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	return moved(ss.book.GoTo(int(page)))
}

type stateJSON struct {
	Page        int     `json:"page"`
	LastVisible int     `json:"lastVisible"`
	Pages       int     `json:"pages"`
	Columns     int     `json:"columns"`
	Chapter     string  `json:"chapter"`
	Progress    float64 `json:"progress"`
}

//export mole_session_state
func mole_session_state(s C.int64_t) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r := ss.book.Reading()
	return jsonString(stateJSON{
		Page: r.Page(), LastVisible: r.LastVisible(), Pages: r.Pages(),
		Columns: r.Geometry().Columns, Chapter: r.ChapterTitle(), Progress: r.Progress(),
	})
}

type chapterJSON struct {
	Title string `json:"title"`
	Level int    `json:"level"`
	Page  int    `json:"page"`
}

//export mole_session_chapters
func mole_session_chapters(s C.int64_t) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	chapters := ss.book.Reading().Chapters()
	out := make([]chapterJSON, len(chapters))
	for i, c := range chapters {
		out[i] = chapterJSON(c)
	}
	return jsonString(out)
}

// mole_session_set_style sets how the next renders draw the pages. pictures
// is 0 for night (photographs as they are, line art with its lightness
// inverted so that its lines glow), 1 for original colors (every image as
// on a white page) or 2 for hidden; bookColors is 1 to keep the hues of
// colored text. It returns 1 if the style changed, which calls for a new
// render, 0 if not, and -1 on error.
//
//export mole_session_set_style
func mole_session_set_style(s C.int64_t, pictures, bookColors C.int) C.int {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return -1
	}
	changed, err := ss.book.Reading().SetStyle(reading.Style{Pictures: mupdf.Pictures(pictures), BookColors: bookColors != 0})
	if err != nil {
		fail(err)
		return -1
	}
	if !changed {
		return 0
	}
	return 1
}

// mole_session_render draws the visible pages and returns the coverage mask
// of their text and vector lines: one byte per pixel, rows of *stride bytes,
// *width x *height pixels. The hues of colored text and the images are drawn
// into layers of their own, read with mole_session_text_colors and
// mole_session_image_layer. The buffer belongs to the session and stays
// valid until the next render, layout or close.
//
//export mole_session_render
func mole_session_render(s C.int64_t, stride, width, height *C.int64_t) unsafe.Pointer {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r := ss.book.Reading()
	mask, rowBytes, err := r.Render()
	if err != nil {
		fail(err)
		return nil
	}
	if err := ss.mask.fill(mask); err != nil {
		fail(err)
		return nil
	}
	w, h := r.Geometry().PixelSize()
	*stride, *width, *height = C.int64_t(rowBytes), C.int64_t(w), C.int64_t(h)
	return ss.mask.p
}

// maskSize returns the size in bytes of the text mask of r.
func maskSize(r *reading.Session) int {
	_, h := r.Geometry().PixelSize()
	return r.Stride() * h
}

// colorLayer copies a four-byte layer of the last render into buf and
// returns it, setting the sizes and *empty as the layer functions document.
func colorLayer(ss *openBook, buf *cBuffer, layer []byte, drawn bool, stride, width, height *C.int64_t,
	empty *C.int) unsafe.Pointer {
	r := ss.book.Reading()
	if err := buf.fillLayer(layer, drawn, 4*maskSize(r)); err != nil {
		fail(err)
		return nil
	}
	w, h := r.Geometry().PixelSize()
	*stride, *width, *height = C.int64_t(4*r.Stride()), C.int64_t(w), C.int64_t(h)
	*empty = 1
	if drawn {
		*empty = 0
	}
	return buf.p
}

// mole_session_text_colors returns the hues of the text of the last render,
// laid out like its mask (same size) with four bytes per pixel and rows of
// *stride bytes: RGBA in sRGB with premultiplied alpha, where alpha is the
// coverage of the text drawn in a hue. Black, white and gray text is left
// out, for the reader's tint, and dark hues are lightened to show on black;
// the mask still holds the coverage of all text. *empty is set to 1 when no
// colored text is on screen or the style does not keep the book's colors;
// the layer is then all zero. The buffer belongs to the session and stays
// valid until the next render, layout or close.
//
//export mole_session_text_colors
func mole_session_text_colors(s C.int64_t, stride, width, height *C.int64_t, empty *C.int) unsafe.Pointer {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	layer, drawn := ss.book.Reading().ColorLayer()
	return colorLayer(ss, &ss.colors, layer, drawn, stride, width, height, empty)
}

// mole_session_image_layer returns the images of the last render, laid out
// like its mask (same size) with four bytes per pixel and rows of *stride
// bytes: RGBA in sRGB with premultiplied alpha, which makes the color
// channels the picture on black. The style says how images are drawn.
// *empty is set to 1 when no image is on screen or pictures are hidden; the
// layer is then all zero. The buffer belongs to the session and stays valid
// until the next render, layout or close.
//
//export mole_session_image_layer
func mole_session_image_layer(s C.int64_t, stride, width, height *C.int64_t, empty *C.int) unsafe.Pointer {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	layer, drawn := ss.book.Reading().ImageLayer()
	return colorLayer(ss, &ss.images, layer, drawn, stride, width, height, empty)
}

// mole_session_image_mask returns the image layer of the last render in
// gray, laid out like its mask (same size and *stride): one byte of
// luminance per pixel on black. It predates mole_session_image_layer, which
// has the colors. *empty is set to 1 when no image is on screen; the layer
// is then all zero. The buffer belongs to the session and stays valid until
// the next render, layout or close.
//
//export mole_session_image_mask
func mole_session_image_mask(s C.int64_t, stride, width, height *C.int64_t, empty *C.int) unsafe.Pointer {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r := ss.book.Reading()
	layer, drawn := r.ImageLayer()
	size := maskSize(r)
	var err error
	if drawn && len(layer) == 4*size {
		err = ss.gray.fill(luminance(layer))
	} else {
		err = ss.gray.clear(size)
	}
	if err != nil {
		fail(err)
		return nil
	}
	w, h := r.Geometry().PixelSize()
	*stride, *width, *height = C.int64_t(r.Stride()), C.int64_t(w), C.int64_t(h)
	*empty = 1
	if drawn {
		*empty = 0
	}
	return ss.gray.p
}

// luminance returns the luma of each pixel of an RGBA layer.
func luminance(rgba []byte) []byte {
	out := make([]byte, len(rgba)/4)
	for i := range out {
		p := rgba[4*i : 4*i+3]
		out[i] = byte((77*int(p[0]) + 150*int(p[1]) + 29*int(p[2])) >> 8)
	}
	return out
}

type pictureJSON struct {
	Page  int      `json:"page"`
	Index int      `json:"index"`
	Rect  rectJSON `json:"rect"`
}

// mole_session_image_at returns the picture at the point x, y of the view as
// JSON {"page":int,"index":int,"rect":Rect}: the page that draws it, its
// place among the images of that page and where it is on screen. It returns
// the JSON literal null when no picture is there (images smaller than 16
// points do not count), and a null pointer only on error. The caller frees
// the result with mole_free.
//
//export mole_session_image_at
func mole_session_image_at(s C.int64_t, x, y C.double) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	p, ok, err := ss.book.Reading().PictureAt(float64(x), float64(y))
	if err != nil {
		fail(err)
		return nil
	}
	if !ok {
		return jsonString(nil)
	}
	return jsonString(pictureJSON{Page: p.Page, Index: p.Index, Rect: rectJSON(p.Rect)})
}

// mole_session_image_pixels draws a picture that mole_session_image_at
// reported, by its page and index, as it looks on a white page: in its
// colors, at its own resolution (a drawing that scales at the largest size)
// and at most 4096 pixels on its longer side. It returns opaque RGBA pixels,
// eight bits per channel, *width x *height with rows of 4 * *width bytes,
// in a buffer the caller frees with mole_free; or a null pointer on error.
//
//export mole_session_image_pixels
func mole_session_image_pixels(s, page, index C.int64_t, width, height *C.int64_t) unsafe.Pointer {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	img, err := ss.book.Reading().PicturePixels(int(page), int(index))
	if err != nil {
		fail(err)
		return nil
	}
	*width, *height = C.int64_t(img.Rect.Dx()), C.int64_t(img.Rect.Dy())
	return C.CBytes(img.Pix)
}

// Text: anchors, selection and search.

func toAnchorJSON(a reading.Anchor) anchorJSON {
	return anchorJSON{Chapter: a.Chapter, Offset: a.Offset}
}

func toRangeJSON(r reading.Range) rangeJSON {
	return rangeJSON{Start: toAnchorJSON(r.Start), End: toAnchorJSON(r.End)}
}

func parseAnchor(arg *C.char) (reading.Anchor, bool) {
	var a anchorJSON
	if !parseJSON(arg, "anchor", &a) {
		return reading.Anchor{}, false
	}
	return reading.Anchor(a), true
}

func parseRange(arg *C.char) (reading.Range, bool) {
	var r rangeJSON
	if !parseJSON(arg, "range", &r) {
		return reading.Range{}, false
	}
	return reading.Range{Start: reading.Anchor(r.Start), End: reading.Anchor(r.End)}, true
}

// mole_session_hit_test returns the anchor of the character at the point x,
// y of the view, or of the character nearest to it on the same line. It
// returns the JSON literal null when the point is not over a line of text,
// and a null pointer only on error. The caller frees the result with
// mole_free.
//
//export mole_session_hit_test
func mole_session_hit_test(s C.int64_t, x, y C.double) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	a, ok, err := ss.book.Reading().HitTest(float64(x), float64(y))
	if err != nil {
		fail(err)
		return nil
	}
	if !ok {
		return jsonString(nil)
	}
	return jsonString(toAnchorJSON(a))
}

// mole_session_visible_range returns the range of text on the visible pages.
// Its start, the first visible character, is where a bookmark of the screen
// goes. The caller frees the result with mole_free.
//
//export mole_session_visible_range
func mole_session_visible_range(s C.int64_t) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r, err := ss.book.Reading().VisibleRange()
	if err != nil {
		fail(err)
		return nil
	}
	return jsonString(toRangeJSON(r))
}

// mole_session_range_text returns the text of a range, on screen or not, as
// JSON {"text":string}: whitespace collapsed to single spaces and a "\n"
// between paragraphs. The caller frees the result with mole_free.
//
//export mole_session_range_text
func mole_session_range_text(s C.int64_t, textRange *C.char) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r, ok := parseRange(textRange)
	if !ok {
		return nil
	}
	text, err := ss.book.Reading().RangeText(r)
	if err != nil {
		fail(err)
		return nil
	}
	return jsonString(struct {
		Text string `json:"text"`
	}{text})
}

type rectJSON struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// mole_session_range_rects returns the boxes of the visible part of a range
// as a JSON array of rectangles, one per line of text it covers, and an
// empty array when none of it is visible. The caller frees the result with
// mole_free.
//
//export mole_session_range_rects
func mole_session_range_rects(s C.int64_t, textRange *C.char) *C.char {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	r, ok := parseRange(textRange)
	if !ok {
		return nil
	}
	rects, err := ss.book.Reading().RangeRects(r)
	if err != nil {
		fail(err)
		return nil
	}
	out := make([]rectJSON, len(rects))
	for i, rect := range rects {
		out[i] = rectJSON(rect)
	}
	return jsonString(out)
}

// mole_session_go_to_anchor shows the screen that holds an anchor and
// returns 1 if the page changed. It returns 0 when the anchor is already
// visible, and also on error. The new position is saved.
//
//export mole_session_go_to_anchor
func mole_session_go_to_anchor(s C.int64_t, anchor *C.char) C.int {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	a, ok := parseAnchor(anchor)
	if !ok {
		return 0
	}
	return moved(ss.book.GoToAnchor(a))
}

// mole_session_anchor_page returns the page, counting from 0, that holds an
// anchor in the current layout, or -1 on error.
//
//export mole_session_anchor_page
func mole_session_anchor_page(s C.int64_t, anchor *C.char) C.int64_t {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return -1
	}
	a, ok := parseAnchor(anchor)
	if !ok {
		return -1
	}
	page, err := ss.book.Reading().AnchorPage(a)
	if err != nil {
		fail(err)
		return -1
	}
	return C.int64_t(page)
}

// mole_session_search starts searching the book for query in the background
// and returns a search handle, or 0 on error (for example a query with
// nothing to search for). Case and accents are ignored, and whitespace in
// the query matches any whitespace in the text. Release the handle with
// mole_search_close; it stays valid after the session is closed.
//
//export mole_session_search
func mole_session_search(s C.int64_t, query *C.char) C.int64_t {
	ss, ok := sessions.get(int64(s))
	if !ok {
		fail(errBadHandle)
		return 0
	}
	sr, err := ss.book.Reading().Search(C.GoString(query))
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(searches.add(sr))
}

type hitJSON struct {
	Range       rangeJSON `json:"range"`
	Excerpt     string    `json:"excerpt"`
	MatchStart  int       `json:"matchStart"` // UTF-16 code units into excerpt
	MatchLength int       `json:"matchLength"`
}

type searchJSON struct {
	Done      bool      `json:"done"`
	Truncated bool      `json:"truncated"`
	Hits      []hitJSON `json:"hits"`
}

// mole_search_results returns every hit a search has found so far, as JSON
// {"done":bool,"truncated":bool,"hits":[{"range","excerpt","matchStart",
// "matchLength"}]}, in reading order. done is true once the search is over;
// truncated is true when it stopped at reading.MaxHits hits. Each excerpt is
// about 80 characters of text around the match on one line, and matchStart
// and matchLength locate the match in it in UTF-16 code units, the indexing
// of NSString. It returns a null pointer when the search failed. The caller
// frees the result with mole_free.
//
//export mole_search_results
func mole_search_results(h C.int64_t) *C.char {
	sr, ok := searches.get(int64(h))
	if !ok {
		fail(errBadHandle)
		return nil
	}
	res, err := sr.Results()
	if err != nil {
		fail(err)
		return nil
	}
	out := searchJSON{Done: res.Done, Truncated: res.Truncated, Hits: make([]hitJSON, len(res.Hits))}
	for i, hit := range res.Hits {
		before := hit.Excerpt[:hit.MatchStart]
		match := hit.Excerpt[hit.MatchStart : hit.MatchStart+hit.MatchLength]
		out.Hits[i] = hitJSON{
			Range: toRangeJSON(hit.Range), Excerpt: hit.Excerpt,
			MatchStart: utf16Len(before), MatchLength: utf16Len(match),
		}
	}
	return jsonString(out)
}

// mole_search_close stops a search if it is still running and releases it.
//
//export mole_search_close
func mole_search_close(h C.int64_t) {
	sr, ok := searches.get(int64(h))
	if !ok {
		return
	}
	sr.Close()
	searches.remove(int64(h))
}

// utf16Len returns the length of s in UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

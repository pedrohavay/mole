// shim.h is a small C API over MuPDF. MuPDF reports errors with setjmp/longjmp
// (fz_try/fz_catch), which must never unwind through Go frames, so every call
// that can throw is wrapped here and turned into a status code plus a message.

#ifndef MOLE_MUPDF_SHIM_H
#define MOLE_MUPDF_SHIM_H

#include <stddef.h>

typedef struct md_doc md_doc;

// md_open opens a document with its own MuPDF context, which may allocate
// at most limit bytes. user_css may be NULL. It returns NULL and writes a
// message into err on failure.
md_doc *md_open(const char *path, const char *user_css, size_t limit, char *err, int errlen);

// md_open_buffer is md_open for a document held in memory: it opens a copy
// of the len bytes at data, whose type magic names by extension or MIME
// type (for example "html").
md_doc *md_open_buffer(const unsigned char *data, size_t len, const char *magic, const char *user_css,
                       size_t limit, char *err, int errlen);

// md_close releases the document and its context.
void md_close(md_doc *d);

// md_layout reflows the document to pages of width x height points with a
// base font size of em points. It returns the page count, or -1 on error.
int md_layout(md_doc *d, float width, float height, float em, char *err, int errlen);

// md_render_layers draws a page into two 8-bit buffers owned by the caller,
// both width x height pixels with rows of stride bytes. text receives the
// coverage of text, in full ink whatever its color, and of vector lines: 0
// where the page is empty, 255 where ink fully covers the pixel; filled
// boxes such as backgrounds are left out. images receives the luminance of images and
// shadings on a black page, with line art inverted so that its lines are
// light; *has_images is set to 1 when the page draws any. It returns 0 on
// success.
int md_render_layers(md_doc *d, int page, float scale, unsigned char *text, unsigned char *images,
                     int width, int height, int stride, int *has_images, char *err, int errlen);

// md_render_png draws a page in color and returns it as PNG bytes that the
// caller must release with md_free. It returns NULL on error.
unsigned char *md_render_png(md_doc *d, int page, float scale, size_t *len, char *err, int errlen);

// md_chapter_position reports the chapter that holds a page and how far into
// the chapter the page is, from 0 to 1. It returns 0 on success.
int md_chapter_position(md_doc *d, int page, int *chapter, float *fraction);

// md_page_at returns the page at a fraction of a chapter in the current
// layout, or -1 if the chapter does not exist.
int md_page_at(md_doc *d, int chapter, float fraction);

// md_metadata copies a metadata value, such as "info:Title" or "info:Author",
// into buf; it returns 0 when the document has none.
int md_metadata(md_doc *d, const char *key, char *buf, int buflen);

// md_outline_item is one entry of a flattened table of contents.
typedef struct {
    char *title;
    char *uri;
    int level;
} md_outline_item;

// md_outline returns the table of contents in reading order, depth first, in
// *items (release it with md_free_outline). It returns the number of items,
// or -1 on error.
int md_outline(md_doc *d, md_outline_item **items, char *err, int errlen);

void md_free_outline(md_outline_item *items, int count);

// md_resolve returns the page a link URI points to in the current layout, or -1.
int md_resolve(md_doc *d, const char *uri);

// md_count_chapters returns the number of chapters, or -1 on error.
int md_count_chapters(md_doc *d);

// md_chapter_pages returns the number of pages of a chapter in the current
// layout, or -1 if the chapter does not exist.
int md_chapter_pages(md_doc *d, int chapter);

// md_location reports the chapter that holds a page and the index of the
// page within that chapter. It returns 0 on success.
int md_location(md_doc *d, int page, int *chapter, int *index);

// md_page_number returns the page at index within a chapter in the current
// layout, or -1 if there is none.
int md_page_number(md_doc *d, int chapter, int index);

// md_char is one character of a page's structured text, in reading order.
// Ligatures are expanded into their letters and horizontal whitespace is
// reported as spaces. The box is in page points, y down from the top.
typedef struct {
    int c;
    float x0, y0, x1, y1;
    int line;  // index of the line on the page, counting from 0
    int block; // index of the text block on the page, counting from 0
} md_char;

// md_page_chars extracts the text of a page into *chars (release it with
// md_free). It returns the number of characters, or -1 on error.
int md_page_chars(md_doc *d, int page, md_char **chars, char *err, int errlen);

void md_free(void *p);

#endif

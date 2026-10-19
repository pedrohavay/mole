// shim.h is a small C API over MuPDF. MuPDF reports errors with setjmp/longjmp
// (fz_try/fz_catch), which must never unwind through Go frames, so every call
// that can throw is wrapped here and turned into a status code plus a message.

#ifndef MOLE_MUPDF_SHIM_H
#define MOLE_MUPDF_SHIM_H

#include <stddef.h>

typedef struct md_doc md_doc;

// md_open opens a document with its own MuPDF context, which may allocate
// at most limit bytes and finds installed fonts that the document's styles
// name through md_find_font. user_css may be NULL. It returns NULL and writes a
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
// Pages are those shown, here and in every other function: MuPDF's pages
// less the blank pages that open a chapter of a reflowable document.
int md_layout(md_doc *d, float width, float height, float em, char *err, int errlen);

// MD_PICTURES_* say how md_render_layers draws images.
enum {
    MD_PICTURES_NIGHT = 0,    // photographs as they are, line art with its lightness inverted
    MD_PICTURES_ORIGINAL = 1, // every image as on a white page
    MD_PICTURES_HIDDEN = 2,   // no image
};

// md_layers are the buffers md_render_layers draws a page into, owned by the
// caller. All are width x height pixels with rows of stride pixels: text has
// one byte per pixel, colors and images four, RGBA in device RGB (sRGB) with
// premultiplied alpha. colors and images must be all zero (transparent) on
// entry; either may be NULL to draw nothing into it.
typedef struct {
    unsigned char *text;
    unsigned char *colors;
    unsigned char *images;
    int width, height, stride;
} md_layers;

// md_render_layers draws a page into layers. text receives the coverage of
// text, in full ink whatever its color, and of vector lines: 0 where the
// page is empty, 255 where ink fully covers the pixel; filled boxes such as
// backgrounds are left out. colors receives the color of text and lines
// drawn in a hue, lightened for a black page where it is dark; black, white
// and gray are left out, for the reader's tint. images receives the images
// and shadings on a black page as pictures (MD_PICTURES_*) says.
// *has_colors and *has_images are set to 1 when the page draws into colors
// or images. It returns 0 on success.
int md_render_layers(md_doc *d, int page, float scale, const md_layers *layers, int pictures, int *has_colors,
                     int *has_images, char *err, int errlen);

// md_rect is a rectangle in page points, y down from the top of the page.
typedef struct {
    float x0, y0, x1, y1;
} md_rect;

// md_page_images reports where a page draws its images, in drawing order,
// as boxes in *boxes (release it with md_free). Images inside masks and
// patterns are left out. It returns the number of images, or -1 on error.
int md_page_images(md_doc *d, int page, md_rect **boxes, char *err, int errlen);

// md_image_pixels draws an image of a page, the index-th that
// md_page_images reports, as it looks on white paper: at its own
// resolution, or max_side pixels on its longer side for a scalable image,
// and never more than max_side. It returns opaque RGBA pixels with rows of
// 4 * *width bytes, which the caller releases with md_free, or NULL on error.
unsigned char *md_image_pixels(md_doc *d, int page, int index, int max_side, int *width, int *height, char *err,
                               int errlen);

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
    int block; // index of the paragraph on the page, counting from 0; see md_page_chars
} md_char;

// md_page_chars extracts the text of a page into *chars (release it with
// md_free). Its paragraphs are MuPDF's text blocks, which MuPDF splits where
// two lines are more than 1.5 em apart. When line_dist is larger, a block
// whose first line goes on the block before it, no more than line_dist ems
// below, belongs to the same paragraph. It returns the number of characters,
// or -1 on error.
int md_page_chars(md_doc *d, int page, float line_dist, md_char **chars, char *err, int errlen);

// md_find_font looks up an installed font family through CoreText and copies
// the path of the file that holds its face closest to bold and italic into
// path, and the index of that face in the file (0 unless the file is a
// collection) into *index. It returns 0 on success and -1 when no family of
// that name is installed. The CSS generic families (serif, monospace, ...)
// are never looked up: MuPDF has fonts of its own for them.
int md_find_font(const char *family, int bold, int italic, char *path, int pathlen, int *index);

void md_free(void *p);

#endif

// systemfonts.h lets MuPDF load the fonts installed on the Mac. It is
// private to the shim; the Go side reaches it through shim.h.

#ifndef MOLE_MUPDF_SYSTEMFONTS_H
#define MOLE_MUPDF_SYSTEMFONTS_H

#include <mupdf/fitz.h>

typedef struct md_font_file md_font_file;
typedef struct md_font_miss md_font_miss;

// md_font_files holds the font files a document has read, so that the faces
// of one collection (bold, italic, ...) share a single copy of it, and the
// families it asked for that are not installed: MuPDF asks again for every
// element styled with them.
typedef struct {
    md_font_file *first;
    md_font_miss *misses;
} md_font_files;

// md_install_system_fonts makes ctx look up the font families that a book's
// styles name, and that MuPDF does not have built in, among the installed
// fonts. files keeps what was read and must outlive ctx's documents.
void md_install_system_fonts(fz_context *ctx, md_font_files *files);

// md_drop_font_files releases the files read for a document.
void md_drop_font_files(fz_context *ctx, md_font_files *files);

#endif

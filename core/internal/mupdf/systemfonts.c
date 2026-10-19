#include "shim.h"
#include "systemfonts.h"

#include <CoreText/CoreText.h>
#include <fcntl.h>
#include <limits.h>
#include <string.h>
#include <strings.h>
#include <sys/stat.h>
#include <unistd.h>

// MuPDF knows only the fonts it has built in. A book that names an installed
// font, and the reader's own choice of font, reach the Mac's fonts through a
// loader that asks CoreText which file holds the face of a family closest to
// the wanted weight and style, finds the face in that file and hands the file
// to MuPDF.
//
// MuPDF 1.26 builds FreeType without support for font variations, so a
// variable font loads at its default instance (for New York, its display
// cut) and MuPDF makes up its bold, as it does for a family that has no bold
// face.

// MD_MAX_FONT_FILE is the largest font file, in bytes, the loader reads.
// Larger ones (color emoji) are bitmaps MuPDF cannot draw anyway.
#define MD_MAX_FONT_FILE ((off_t)100 << 20)

// MD_MAX_FACES bounds how many faces of a collection are looked at.
#define MD_MAX_FACES 256

// CoreText's weight trait of a bold face, NSFontWeightBold in AppKit.
#define MD_WEIGHT_BOLD 0.4

static void release(CFTypeRef ref) {
    if (ref != NULL) {
        CFRelease(ref);
    }
}

// is_generic reports whether family is a CSS generic family. MuPDF maps
// serif, sans-serif and monospace to its built-in fonts, which the reader's
// styles rely on for code and as fallbacks.
static int is_generic(const char *family) {
    static const char *const generic[] = {"serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui"};
    for (size_t i = 0; i < sizeof generic / sizeof *generic; i++) {
        if (strcasecmp(family, generic[i]) == 0) {
            return 1;
        }
    }
    return 0;
}

// match_family returns the face of an installed family closest to bold and
// italic, or NULL when no family has that name. CoreText would rather
// substitute another family, which would hide the next family a style lists.
static CTFontDescriptorRef match_family(CFStringRef family, int bold, int italic) {
    CTFontSymbolicTraits symbolic = (bold ? kCTFontBoldTrait : 0) | (italic ? kCTFontItalicTrait : 0);
    double weight = bold ? MD_WEIGHT_BOLD : 0;
    CFNumberRef symbolic_number = CFNumberCreate(NULL, kCFNumberSInt32Type, &symbolic);
    CFNumberRef weight_number = CFNumberCreate(NULL, kCFNumberDoubleType, &weight);
    CFDictionaryRef traits = NULL;
    CFDictionaryRef attributes = NULL;
    CTFontDescriptorRef wanted = NULL;
    CTFontDescriptorRef match = NULL;
    if (symbolic_number != NULL && weight_number != NULL) {
        const void *trait_keys[] = {kCTFontSymbolicTrait, kCTFontWeightTrait};
        const void *trait_values[] = {symbolic_number, weight_number};
        traits = CFDictionaryCreate(NULL, trait_keys, trait_values, 2, &kCFTypeDictionaryKeyCallBacks,
                                    &kCFTypeDictionaryValueCallBacks);
    }
    if (traits != NULL) {
        const void *keys[] = {kCTFontFamilyNameAttribute, kCTFontTraitsAttribute};
        const void *values[] = {family, traits};
        attributes = CFDictionaryCreate(NULL, keys, values, 2, &kCFTypeDictionaryKeyCallBacks,
                                        &kCFTypeDictionaryValueCallBacks);
    }
    if (attributes != NULL) {
        wanted = CTFontDescriptorCreateWithAttributes(attributes);
    }
    if (wanted != NULL) {
        match = CTFontDescriptorCreateMatchingFontDescriptor(wanted, NULL);
    }
    if (match != NULL) {
        CFStringRef found = CTFontDescriptorCopyAttribute(match, kCTFontFamilyNameAttribute);
        if (found == NULL || CFStringCompare(found, family, kCFCompareCaseInsensitive) != kCFCompareEqualTo) {
            CFRelease(match);
            match = NULL;
        }
        release(found);
    }
    release(wanted);
    release(attributes);
    release(traits);
    release(weight_number);
    release(symbolic_number);
    return match;
}

static uint16_t be16(const unsigned char *p) {
    return (uint16_t)(p[0] << 8 | p[1]);
}

static uint32_t be32(const unsigned char *p) {
    return (uint32_t)p[0] << 24 | (uint32_t)p[1] << 16 | (uint32_t)p[2] << 8 | p[3];
}

// read_at reads n bytes at offset of fd and reports whether it could.
static int read_at(int fd, off_t offset, void *buf, size_t n) {
    return pread(fd, buf, n, offset) == (ssize_t)n;
}

// names_postscript reports whether the name table at offset of fd, length
// bytes long, gives the font the PostScript name psname.
static int names_postscript(int fd, off_t table, uint32_t length, const char *psname) {
    unsigned char head[6];
    if (length < sizeof head || !read_at(fd, table, head, sizeof head)) {
        return 0;
    }
    size_t want = strlen(psname);
    uint16_t count = be16(head + 2);
    off_t strings = table + be16(head + 4);
    for (uint16_t i = 0; i < count; i++) {
        unsigned char record[12];
        if (!read_at(fd, table + 6 + (off_t)12 * i, record, sizeof record)) {
            return 0;
        }
        uint16_t platform = be16(record), name_id = be16(record + 6), len = be16(record + 8);
        unsigned char name[512];
        if (name_id != 6 || len > sizeof name || !read_at(fd, strings + be16(record + 10), name, len)) {
            continue;
        }
        if (platform == 1 && len == want && memcmp(name, psname, want) == 0) {
            return 1;
        }
        // Unicode and Windows names are UTF-16BE; PostScript names are ASCII.
        if ((platform == 0 || platform == 3) && len == 2 * want) {
            size_t k = 0;
            while (k < want && name[2 * k] == 0 && name[2 * k + 1] == (unsigned char)psname[k]) {
                k++;
            }
            if (k == want) {
                return 1;
            }
        }
    }
    return 0;
}

// has_postscript_name reports whether the font whose table directory starts
// at offset of fd has the PostScript name psname.
static int has_postscript_name(int fd, off_t offset, const char *psname) {
    unsigned char head[12];
    if (!read_at(fd, offset, head, sizeof head)) {
        return 0;
    }
    uint16_t tables = be16(head + 4);
    for (uint16_t t = 0; t < tables; t++) {
        unsigned char record[16];
        if (!read_at(fd, offset + 12 + (off_t)16 * t, record, sizeof record)) {
            return 0;
        }
        if (memcmp(record, "name", 4) == 0) {
            return names_postscript(fd, be32(record + 8), be32(record + 12), psname);
        }
    }
    return 0;
}

// face_index returns the index of the face named psname in the font file at
// path: 0 for a file of one font, its place in a collection (.ttc), or -1
// when the file cannot be read or the collection has no such face.
static int face_index(const char *path, const char *psname) {
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) {
        return -1;
    }
    int index = -1;
    unsigned char head[12];
    struct stat st;
    if (fstat(fd, &st) == 0 && st.st_size <= MD_MAX_FONT_FILE && read_at(fd, 0, head, sizeof head)) {
        if (memcmp(head, "ttcf", 4) != 0) {
            index = 0;
        } else {
            uint32_t count = be32(head + 8);
            for (uint32_t i = 0; i < count && i < MD_MAX_FACES && index < 0; i++) {
                unsigned char offset[4];
                if (!read_at(fd, 12 + (off_t)4 * i, offset, sizeof offset)) {
                    break;
                }
                if (has_postscript_name(fd, be32(offset), psname)) {
                    index = (int)i;
                }
            }
        }
    }
    close(fd);
    return index;
}

int md_find_font(const char *family, int bold, int italic, char *path, int pathlen, int *index) {
    if (family == NULL || family[0] == '\0' || is_generic(family)) {
        return -1;
    }
    CFStringRef name = CFStringCreateWithCString(NULL, family, kCFStringEncodingUTF8);
    if (name == NULL) {
        return -1;
    }
    CTFontDescriptorRef match = match_family(name, bold, italic);
    if (match == NULL && family[0] != '.') {
        // macOS hides its own families, such as New York, behind a leading dot.
        CFStringRef hidden = CFStringCreateWithFormat(NULL, NULL, CFSTR(".%@"), name);
        if (hidden != NULL) {
            match = match_family(hidden, bold, italic);
            CFRelease(hidden);
        }
    }
    CFRelease(name);
    if (match == NULL) {
        return -1;
    }
    int status = -1;
    char psname[256];
    CFURLRef url = CTFontDescriptorCopyAttribute(match, kCTFontURLAttribute);
    CFStringRef ps = CTFontDescriptorCopyAttribute(match, kCTFontNameAttribute);
    if (url != NULL && ps != NULL && CFURLGetFileSystemRepresentation(url, true, (UInt8 *)path, pathlen) &&
        CFStringGetCString(ps, psname, sizeof psname, kCFStringEncodingUTF8)) {
        *index = face_index(path, psname);
        status = *index >= 0 ? 0 : -1;
    }
    release(ps);
    release(url);
    CFRelease(match);
    return status;
}

struct md_font_file {
    char *path;
    fz_buffer *buffer;
    md_font_file *next;
};

// font_file returns the contents of the font file at path, reading it the
// first time. The buffer belongs to files.
static fz_buffer *font_file(fz_context *ctx, md_font_files *files, const char *path) {
    for (md_font_file *f = files->first; f != NULL; f = f->next) {
        if (strcmp(f->path, path) == 0) {
            return f->buffer;
        }
    }
    fz_buffer *buffer = fz_read_file(ctx, path);
    md_font_file *f = NULL;
    fz_var(f);
    fz_try(ctx) {
        f = fz_malloc_struct(ctx, md_font_file);
        f->path = fz_strdup(ctx, path);
    }
    fz_catch(ctx) {
        fz_free(ctx, f);
        fz_drop_buffer(ctx, buffer);
        fz_rethrow(ctx);
    }
    f->buffer = buffer;
    f->next = files->first;
    files->first = f;
    return buffer;
}

struct md_font_miss {
    char *family;
    int bold, italic;
    md_font_miss *next;
};

static int missing(md_font_files *files, const char *family, int bold, int italic) {
    for (md_font_miss *m = files->misses; m != NULL; m = m->next) {
        if (m->bold == bold && m->italic == italic && strcmp(m->family, family) == 0) {
            return 1;
        }
    }
    return 0;
}

// remember_missing records that a face of family is not installed. Failing
// to record it only costs another lookup.
static void remember_missing(fz_context *ctx, md_font_files *files, const char *family, int bold, int italic) {
    md_font_miss *m = NULL;
    fz_var(m);
    fz_try(ctx) {
        m = fz_malloc_struct(ctx, md_font_miss);
        m->family = fz_strdup(ctx, family);
    }
    fz_catch(ctx) {
        fz_rethrow_if(ctx, FZ_ERROR_SYSTEM);
        fz_free(ctx, m);
        return;
    }
    m->bold = bold;
    m->italic = italic;
    m->next = files->misses;
    files->misses = m;
}

// load_font is MuPDF's hook for a family it does not have built in. It
// returns NULL when no such family is installed, so that MuPDF goes on to the
// next family a style lists. needs_exact_metrics only matters for PDF fonts
// that are substituted; an installed font is the real one.
static fz_font *load_font(fz_context *ctx, const char *name, int bold, int italic, int needs_exact_metrics) {
    (void)needs_exact_metrics;
    md_font_files *files = fz_user_context(ctx);
    if (missing(files, name, bold, italic)) {
        return NULL;
    }
    char path[PATH_MAX];
    int index = 0;
    if (md_find_font(name, bold, italic, path, sizeof path, &index) != 0) {
        remember_missing(ctx, files, name, bold, italic);
        return NULL;
    }
    fz_buffer *buffer = font_file(ctx, files, path);
    return fz_new_font_from_buffer(ctx, NULL, buffer, index, 0);
}

void md_install_system_fonts(fz_context *ctx, md_font_files *files) {
    fz_set_user_context(ctx, files);
    fz_install_load_system_font_funcs(ctx, load_font, NULL, NULL);
}

void md_drop_font_files(fz_context *ctx, md_font_files *files) {
    md_font_file *f = files->first;
    while (f != NULL) {
        md_font_file *next = f->next;
        fz_drop_buffer(ctx, f->buffer);
        fz_free(ctx, f->path);
        fz_free(ctx, f);
        f = next;
    }
    md_font_miss *m = files->misses;
    while (m != NULL) {
        md_font_miss *next = m->next;
        fz_free(ctx, m->family);
        fz_free(ctx, m);
        m = next;
    }
    *files = (md_font_files){0};
}

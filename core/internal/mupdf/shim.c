#include "shim.h"
#include "systemfonts.h"

#include <mupdf/fitz.h>
#include <math.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// md_budget caps what one document may allocate, so that a decompression
// bomb in a chapter fails cleanly instead of exhausting memory. A context
// is used by one thread at a time, so the count needs no lock.
typedef struct {
    size_t used;
    size_t limit;
} md_budget;

// Each allocation is preceded by its size, padded to the strictest alignment.
typedef union {
    size_t size;
    max_align_t align;
} md_header;

static void *budget_malloc(void *user, size_t size) {
    md_budget *b = user;
    if (size > b->limit - b->used) {
        return NULL;
    }
    md_header *h = malloc(sizeof *h + size);
    if (h == NULL) {
        return NULL;
    }
    h->size = size;
    b->used += size;
    return h + 1;
}

static void budget_free(void *user, void *p) {
    if (p == NULL) {
        return;
    }
    md_budget *b = user;
    md_header *h = (md_header *)p - 1;
    b->used -= h->size;
    free(h);
}

static void *budget_realloc(void *user, void *p, size_t size) {
    if (p == NULL) {
        return budget_malloc(user, size);
    }
    if (size == 0) {
        budget_free(user, p);
        return NULL;
    }
    md_budget *b = user;
    md_header *h = (md_header *)p - 1;
    size_t old = h->size;
    if (size > old && size - old > b->limit - b->used) {
        return NULL;
    }
    md_header *n = realloc(h, sizeof *n + size);
    if (n == NULL) {
        return NULL;
    }
    b->used = b->used - old + size;
    n->size = size;
    return n + 1;
}

// MD_STORE_SIZE is how much MuPDF may keep in its cache of decoded
// resources, out of what a document may allocate.
#define MD_STORE_SIZE ((size_t)64 << 20)

// md_image_style remembers how an image of the document is shown in the
// image layer: line art inverted, everything else as it is.
typedef struct {
    fz_image *image; // kept
    int line_art;
    fz_image *inverted; // kept, made the first time it is drawn
} md_image_style;

#define MD_IMAGE_STYLES 16

struct md_doc {
    fz_context *ctx;
    fz_document *doc;
    md_budget *budget;
    md_font_files fonts;
    md_image_style styles[MD_IMAGE_STYLES];
    int next_style; // slot to reuse when all are taken
};

// MuPDF prints warnings and errors to stderr by default; the Go side reports
// errors itself, so the library stays quiet.
static void quiet(void *user, const char *message) {
    (void)user;
    (void)message;
}

// Every document has its own context, used by one thread at a time, so most
// MuPDF locks have nothing to protect. FZ_LOCK_FREETYPE is the exception:
// MuPDF hands the current context to HarfBuzz and OpenJPEG through
// process-wide variables guarded by that lock, so it must be one mutex shared
// by all contexts for two documents to be used on different threads.
static pthread_mutex_t freetype_mutex = PTHREAD_MUTEX_INITIALIZER;

static void lock(void *user, int which) {
    (void)user;
    if (which == FZ_LOCK_FREETYPE) {
        pthread_mutex_lock(&freetype_mutex);
    }
}

static void unlock(void *user, int which) {
    (void)user;
    if (which == FZ_LOCK_FREETYPE) {
        pthread_mutex_unlock(&freetype_mutex);
    }
}

static const fz_locks_context locks = {NULL, lock, unlock};

static void set_err(fz_context *ctx, char *err, int errlen) {
    if (err != NULL && errlen > 0) {
        snprintf(err, (size_t)errlen, "%s", fz_caught_message(ctx));
    }
}

md_doc *md_open(const char *path, const char *user_css, size_t limit, char *err, int errlen) {
    md_doc *d = calloc(1, sizeof *d);
    md_budget *budget = malloc(sizeof *budget);
    if (d == NULL || budget == NULL) {
        free(budget);
        free(d);
        snprintf(err, (size_t)errlen, "out of memory");
        return NULL;
    }
    *budget = (md_budget){0, limit};
    fz_alloc_context alloc = {budget, budget_malloc, budget_realloc, budget_free};
    fz_context *ctx = fz_new_context(&alloc, &locks, MD_STORE_SIZE);
    if (ctx == NULL) {
        free(budget);
        free(d);
        snprintf(err, (size_t)errlen, "cannot create MuPDF context");
        return NULL;
    }
    fz_set_warning_callback(ctx, quiet, NULL);
    fz_set_error_callback(ctx, quiet, NULL);
    md_install_system_fonts(ctx, &d->fonts);
    fz_document *doc = NULL;
    fz_var(doc);
    fz_try(ctx) {
        fz_register_document_handlers(ctx);
        if (user_css != NULL) {
            fz_set_user_css(ctx, user_css);
        }
        doc = fz_open_document(ctx, path);
    }
    fz_catch(ctx) {
        set_err(ctx, err, errlen);
        md_drop_font_files(ctx, &d->fonts);
        fz_drop_context(ctx);
        free(budget);
        free(d);
        return NULL;
    }
    d->ctx = ctx;
    d->doc = doc;
    d->budget = budget;
    return d;
}

static void drop_style(fz_context *ctx, md_image_style *style) {
    fz_drop_image(ctx, style->image);
    fz_drop_image(ctx, style->inverted);
    *style = (md_image_style){0};
}

void md_close(md_doc *d) {
    if (d == NULL) {
        return;
    }
    for (int i = 0; i < MD_IMAGE_STYLES; i++) {
        drop_style(d->ctx, &d->styles[i]);
    }
    fz_drop_document(d->ctx, d->doc);
    md_drop_font_files(d->ctx, &d->fonts);
    fz_drop_context(d->ctx);
    free(d->budget);
    free(d);
}

int md_layout(md_doc *d, float width, float height, float em, char *err, int errlen) {
    int count = -1;
    fz_var(count);
    fz_try(d->ctx) {
        fz_layout_document(d->ctx, d->doc, width, height, em);
        count = fz_count_pages(d->ctx, d->doc);
    }
    fz_catch(d->ctx) {
        set_err(d->ctx, err, errlen);
        count = -1;
    }
    return count;
}

// The image layer shows images and shadings on a black page. A photograph or
// a dark picture is drawn as it is, so a dark cover stays dark; line art (a
// diagram, black lines on white) would be a sheet of light, so it is drawn
// inverted and its lines glow instead.

// gray_pixmap converts pix to gray with alpha, keeping it.
static fz_pixmap *gray_pixmap(fz_context *ctx, fz_pixmap *pix) {
    return fz_convert_pixmap(ctx, pix, fz_device_gray(ctx), NULL, NULL, fz_default_color_params, 1);
}

// is_line_art reports whether an image is mostly light, judged from a small
// copy of it composited on white, as it would appear on a white page.
static int is_line_art(fz_context *ctx, fz_image *image) {
    fz_pixmap *pix = NULL;
    fz_pixmap *gray = NULL;
    int light = 0;
    fz_var(pix);
    fz_var(gray);
    fz_var(light);
    fz_try(ctx) {
        // Asking for a 64 x 64 drawing lets MuPDF decode at a reduced size.
        fz_matrix ctm = fz_scale(64, 64);
        pix = fz_get_pixmap_from_image(ctx, image, NULL, &ctm, NULL, NULL);
        if (fz_pixmap_colorspace(ctx, pix) != NULL) {
            gray = gray_pixmap(ctx, pix);
            int w = fz_pixmap_width(ctx, gray), h = fz_pixmap_height(ctx, gray);
            int n = fz_pixmap_components(ctx, gray), alpha = fz_pixmap_alpha(ctx, gray);
            ptrdiff_t stride = fz_pixmap_stride(ctx, gray);
            const unsigned char *samples = fz_pixmap_samples(ctx, gray);
            int dx = w > 64 ? w / 64 : 1, dy = h > 64 ? h / 64 : 1;
            double sum = 0;
            long count = 0;
            for (int y = 0; y < h; y += dy) {
                const unsigned char *row = samples + y * stride;
                for (int x = 0; x < w; x += dx) {
                    const unsigned char *p = row + x * n;
                    // Premultiplied gray over white.
                    sum += alpha ? p[0] + (255 - p[1]) : p[0];
                    count++;
                }
            }
            light = count > 0 && sum / count > 0.5 * 255;
        }
    }
    fz_always(ctx) {
        fz_drop_pixmap(ctx, gray);
        fz_drop_pixmap(ctx, pix);
    }
    fz_catch(ctx) {
        fz_rethrow_if(ctx, FZ_ERROR_SYSTEM);
        fz_report_error(ctx); // an image that cannot be decoded is shown as it is
        light = 0;
    }
    return light;
}

// MD_MAX_INVERTED is the largest side, in pixels, of an inverted copy.
#define MD_MAX_INVERTED 2048

// inverted_image returns a copy of image with its luminance inverted, in
// gray, keeping its transparency.
static fz_image *inverted_image(fz_context *ctx, fz_image *image) {
    fz_pixmap *pix = NULL;
    fz_pixmap *gray = NULL;
    fz_image *out = NULL;
    fz_var(pix);
    fz_var(gray);
    fz_try(ctx) {
        fz_matrix ctm = fz_scale(image->w, image->h);
        int side = image->w > image->h ? image->w : image->h;
        if (side > MD_MAX_INVERTED) {
            float shrink = (float)MD_MAX_INVERTED / (float)side;
            ctm = fz_scale(image->w * shrink, image->h * shrink);
        }
        pix = fz_get_pixmap_from_image(ctx, image, NULL, &ctm, NULL, NULL);
        gray = gray_pixmap(ctx, pix);
        int w = fz_pixmap_width(ctx, gray), h = fz_pixmap_height(ctx, gray);
        int n = fz_pixmap_components(ctx, gray), alpha = fz_pixmap_alpha(ctx, gray);
        ptrdiff_t stride = fz_pixmap_stride(ctx, gray);
        unsigned char *samples = fz_pixmap_samples(ctx, gray);
        for (int y = 0; y < h; y++) {
            unsigned char *p = samples + y * stride;
            for (int x = 0; x < w; x++, p += n) {
                // Premultiplied: the inverse of gray g under alpha a is a - g.
                p[0] = (unsigned char)((alpha ? p[1] : 255) - p[0]);
            }
        }
        out = fz_new_image_from_pixmap(ctx, gray, NULL);
    }
    fz_always(ctx) {
        fz_drop_pixmap(ctx, gray);
        fz_drop_pixmap(ctx, pix);
    }
    fz_catch(ctx) {
        fz_rethrow(ctx);
    }
    return out;
}

// image_to_show returns the image to draw in the image layer for image,
// remembering the decision, and the inverted copy, for later renders.
static fz_image *image_to_show(fz_context *ctx, md_doc *d, fz_image *image) {
    md_image_style *style = NULL;
    for (int i = 0; i < MD_IMAGE_STYLES && style == NULL; i++) {
        if (d->styles[i].image == image) {
            style = &d->styles[i];
        }
    }
    if (style == NULL) {
        style = &d->styles[d->next_style];
        d->next_style = (d->next_style + 1) % MD_IMAGE_STYLES;
        drop_style(ctx, style);
        style->line_art = is_line_art(ctx, image);
        style->image = fz_keep_image(ctx, image);
    }
    if (!style->line_art) {
        return image;
    }
    if (style->inverted == NULL) {
        fz_try(ctx) {
            style->inverted = inverted_image(ctx, image);
        }
        fz_catch(ctx) {
            fz_rethrow_if(ctx, FZ_ERROR_SYSTEM);
            fz_report_error(ctx);
            style->line_art = 0;
            return image;
        }
    }
    return style->inverted;
}

// split_device sends text and vector paths to one device and images and
// shadings to another. Clips, groups and masks go to both, and so does
// everything inside a mask definition, as it is, so both see the same
// clipping.
typedef struct {
    fz_device super;
    md_doc *doc;
    fz_device *text;
    fz_device *images;
    int masking; // depth of mask definitions being drawn
    int drawn;   // images and shadings sent to the image device
} split_device;

#define SPLIT(dev) ((split_device *)(dev))

// MD_MAX_RULE is the thickness, in points, above which a filled path is a
// box (a CSS background) rather than a line (a border, a rule, an underline).
#define MD_MAX_RULE 6.0f

// is_box reports whether a filled path is wider and taller than a rule.
// Boxes are not drawn: a dark one would glow as a solid block and hide the
// text on it, and the book's styles cannot be relied on to drop them,
// because MuPDF lets inline styles win over the reader's.
static int is_box(fz_context *ctx, const fz_path *path, fz_matrix ctm) {
    fz_rect r = fz_bound_path(ctx, path, NULL, ctm);
    return r.x1 - r.x0 > MD_MAX_RULE && r.y1 - r.y0 > MD_MAX_RULE;
}

// Text is drawn in full ink whatever its color, so that light or white text
// (white on a dark box, gray captions) still shows as light.
static const float black[FZ_MAX_COLORS] = {0};

static void split_fill_path(fz_context *ctx, fz_device *dev, const fz_path *path, int even_odd, fz_matrix ctm,
                            fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    if (SPLIT(dev)->masking) {
        fz_fill_path(ctx, SPLIT(dev)->text, path, even_odd, ctm, cs, color, alpha, cp);
        fz_fill_path(ctx, SPLIT(dev)->images, path, even_odd, ctm, cs, color, alpha, cp);
    } else if (!is_box(ctx, path, ctm)) {
        fz_fill_path(ctx, SPLIT(dev)->text, path, even_odd, ctm, cs, color, alpha, cp);
    }
}

static void split_stroke_path(fz_context *ctx, fz_device *dev, const fz_path *path, const fz_stroke_state *stroke,
                              fz_matrix ctm, fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    fz_stroke_path(ctx, SPLIT(dev)->text, path, stroke, ctm, cs, color, alpha, cp);
    if (SPLIT(dev)->masking) {
        fz_stroke_path(ctx, SPLIT(dev)->images, path, stroke, ctm, cs, color, alpha, cp);
    }
}

static void split_clip_path(fz_context *ctx, fz_device *dev, const fz_path *path, int even_odd, fz_matrix ctm,
                            fz_rect scissor) {
    fz_clip_path(ctx, SPLIT(dev)->text, path, even_odd, ctm, scissor);
    fz_clip_path(ctx, SPLIT(dev)->images, path, even_odd, ctm, scissor);
}

static void split_clip_stroke_path(fz_context *ctx, fz_device *dev, const fz_path *path,
                                   const fz_stroke_state *stroke, fz_matrix ctm, fz_rect scissor) {
    fz_clip_stroke_path(ctx, SPLIT(dev)->text, path, stroke, ctm, scissor);
    fz_clip_stroke_path(ctx, SPLIT(dev)->images, path, stroke, ctm, scissor);
}

static void split_fill_text(fz_context *ctx, fz_device *dev, const fz_text *text, fz_matrix ctm, fz_colorspace *cs,
                            const float *color, float alpha, fz_color_params cp) {
    if (SPLIT(dev)->masking) {
        fz_fill_text(ctx, SPLIT(dev)->text, text, ctm, cs, color, alpha, cp);
        fz_fill_text(ctx, SPLIT(dev)->images, text, ctm, cs, color, alpha, cp);
    } else {
        fz_fill_text(ctx, SPLIT(dev)->text, text, ctm, fz_device_gray(ctx), black, alpha, cp);
    }
}

static void split_stroke_text(fz_context *ctx, fz_device *dev, const fz_text *text, const fz_stroke_state *stroke,
                              fz_matrix ctm, fz_colorspace *cs, const float *color, float alpha,
                              fz_color_params cp) {
    if (SPLIT(dev)->masking) {
        fz_stroke_text(ctx, SPLIT(dev)->text, text, stroke, ctm, cs, color, alpha, cp);
        fz_stroke_text(ctx, SPLIT(dev)->images, text, stroke, ctm, cs, color, alpha, cp);
    } else {
        fz_stroke_text(ctx, SPLIT(dev)->text, text, stroke, ctm, fz_device_gray(ctx), black, alpha, cp);
    }
}

static void split_clip_text(fz_context *ctx, fz_device *dev, const fz_text *text, fz_matrix ctm, fz_rect scissor) {
    fz_clip_text(ctx, SPLIT(dev)->text, text, ctm, scissor);
    fz_clip_text(ctx, SPLIT(dev)->images, text, ctm, scissor);
}

static void split_clip_stroke_text(fz_context *ctx, fz_device *dev, const fz_text *text,
                                   const fz_stroke_state *stroke, fz_matrix ctm, fz_rect scissor) {
    fz_clip_stroke_text(ctx, SPLIT(dev)->text, text, stroke, ctm, scissor);
    fz_clip_stroke_text(ctx, SPLIT(dev)->images, text, stroke, ctm, scissor);
}

static void split_fill_shade(fz_context *ctx, fz_device *dev, fz_shade *shade, fz_matrix ctm, float alpha,
                             fz_color_params cp) {
    if (SPLIT(dev)->masking) {
        fz_fill_shade(ctx, SPLIT(dev)->text, shade, ctm, alpha, cp);
    } else {
        SPLIT(dev)->drawn++;
    }
    fz_fill_shade(ctx, SPLIT(dev)->images, shade, ctm, alpha, cp);
}

static void split_fill_image(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm, float alpha,
                             fz_color_params cp) {
    if (SPLIT(dev)->masking) {
        fz_fill_image(ctx, SPLIT(dev)->text, image, ctm, alpha, cp);
        fz_fill_image(ctx, SPLIT(dev)->images, image, ctm, alpha, cp);
        return;
    }
    SPLIT(dev)->drawn++;
    fz_fill_image(ctx, SPLIT(dev)->images, image_to_show(ctx, SPLIT(dev)->doc, image), ctm, alpha, cp);
}

// An image mask is a stencil painted in one color, like a glyph, so it
// belongs with the text.
static void split_fill_image_mask(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm,
                                  fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    fz_fill_image_mask(ctx, SPLIT(dev)->text, image, ctm, cs, color, alpha, cp);
    if (SPLIT(dev)->masking) {
        fz_fill_image_mask(ctx, SPLIT(dev)->images, image, ctm, cs, color, alpha, cp);
    }
}

static void split_clip_image_mask(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm, fz_rect scissor) {
    fz_clip_image_mask(ctx, SPLIT(dev)->text, image, ctm, scissor);
    fz_clip_image_mask(ctx, SPLIT(dev)->images, image, ctm, scissor);
}

static void split_pop_clip(fz_context *ctx, fz_device *dev) {
    fz_pop_clip(ctx, SPLIT(dev)->text);
    fz_pop_clip(ctx, SPLIT(dev)->images);
}

static void split_begin_mask(fz_context *ctx, fz_device *dev, fz_rect area, int luminosity, fz_colorspace *cs,
                             const float *bc, fz_color_params cp) {
    fz_begin_mask(ctx, SPLIT(dev)->text, area, luminosity, cs, bc, cp);
    fz_begin_mask(ctx, SPLIT(dev)->images, area, luminosity, cs, bc, cp);
    SPLIT(dev)->masking++;
}

static void split_end_mask(fz_context *ctx, fz_device *dev, fz_function *fn) {
    SPLIT(dev)->masking--;
    fz_end_mask_tr(ctx, SPLIT(dev)->text, fn);
    fz_end_mask_tr(ctx, SPLIT(dev)->images, fn);
}

static void split_begin_group(fz_context *ctx, fz_device *dev, fz_rect area, fz_colorspace *cs, int isolated,
                              int knockout, int blendmode, float alpha) {
    fz_begin_group(ctx, SPLIT(dev)->text, area, cs, isolated, knockout, blendmode, alpha);
    fz_begin_group(ctx, SPLIT(dev)->images, area, cs, isolated, knockout, blendmode, alpha);
}

static void split_end_group(fz_context *ctx, fz_device *dev) {
    fz_end_group(ctx, SPLIT(dev)->text);
    fz_end_group(ctx, SPLIT(dev)->images);
}

// Tiles are never cached (id 0), so both devices always want the content.
static int split_begin_tile(fz_context *ctx, fz_device *dev, fz_rect area, fz_rect view, float xstep, float ystep,
                            fz_matrix ctm, int id) {
    (void)id;
    fz_begin_tile_id(ctx, SPLIT(dev)->text, area, view, xstep, ystep, ctm, 0);
    fz_begin_tile_id(ctx, SPLIT(dev)->images, area, view, xstep, ystep, ctm, 0);
    return 0;
}

static void split_end_tile(fz_context *ctx, fz_device *dev) {
    fz_end_tile(ctx, SPLIT(dev)->text);
    fz_end_tile(ctx, SPLIT(dev)->images);
}

static void split_render_flags(fz_context *ctx, fz_device *dev, int set, int clear) {
    fz_render_flags(ctx, SPLIT(dev)->text, set, clear);
    fz_render_flags(ctx, SPLIT(dev)->images, set, clear);
}

static void split_set_default_colorspaces(fz_context *ctx, fz_device *dev, fz_default_colorspaces *cs) {
    fz_set_default_colorspaces(ctx, SPLIT(dev)->text, cs);
    fz_set_default_colorspaces(ctx, SPLIT(dev)->images, cs);
}

static fz_device *new_split_device(fz_context *ctx, md_doc *d, fz_device *text, fz_device *images) {
    split_device *dev = fz_new_derived_device(ctx, split_device);
    dev->super.fill_path = split_fill_path;
    dev->super.stroke_path = split_stroke_path;
    dev->super.clip_path = split_clip_path;
    dev->super.clip_stroke_path = split_clip_stroke_path;
    dev->super.fill_text = split_fill_text;
    dev->super.stroke_text = split_stroke_text;
    dev->super.clip_text = split_clip_text;
    dev->super.clip_stroke_text = split_clip_stroke_text;
    dev->super.fill_shade = split_fill_shade;
    dev->super.fill_image = split_fill_image;
    dev->super.fill_image_mask = split_fill_image_mask;
    dev->super.clip_image_mask = split_clip_image_mask;
    dev->super.pop_clip = split_pop_clip;
    dev->super.begin_mask = split_begin_mask;
    dev->super.end_mask = split_end_mask;
    dev->super.begin_group = split_begin_group;
    dev->super.end_group = split_end_group;
    dev->super.begin_tile = split_begin_tile;
    dev->super.end_tile = split_end_tile;
    dev->super.render_flags = split_render_flags;
    dev->super.set_default_colorspaces = split_set_default_colorspaces;
    dev->doc = d;
    dev->text = text;
    dev->images = images;
    return &dev->super;
}

int md_render_layers(md_doc *d, int page_number, float scale, unsigned char *text, unsigned char *images,
                     int width, int height, int stride, int *has_images, char *err, int errlen) {
    fz_context *ctx = d->ctx;
    fz_page *page = NULL;
    fz_pixmap *text_pix = NULL;
    fz_pixmap *image_pix = NULL;
    fz_device *text_dev = NULL;
    fz_device *image_dev = NULL;
    fz_device *split = NULL;
    int status = 0;
    fz_var(page);
    fz_var(text_pix);
    fz_var(image_pix);
    fz_var(text_dev);
    fz_var(image_dev);
    fz_var(split);
    *has_images = 0;
    fz_try(ctx) {
        page = fz_load_page(ctx, d->doc, page_number);
        fz_matrix ctm = fz_scale(scale, scale);
        // The pixmaps borrow the caller's buffers, so dropping them frees nothing.
        text_pix = fz_new_pixmap_with_data(ctx, fz_device_gray(ctx), width, height, NULL, 0, stride, text);
        image_pix = fz_new_pixmap_with_data(ctx, fz_device_gray(ctx), width, height, NULL, 0, stride, images);
        fz_clear_pixmap_with_value(ctx, text_pix, 255);
        fz_clear_pixmap_with_value(ctx, image_pix, 0);
        text_dev = fz_new_draw_device(ctx, ctm, text_pix);
        image_dev = fz_new_draw_device(ctx, ctm, image_pix);
        split = new_split_device(ctx, d, text_dev, image_dev);
        fz_run_page(ctx, page, split, fz_identity, NULL);
        fz_close_device(ctx, split);
        fz_close_device(ctx, text_dev);
        fz_close_device(ctx, image_dev);
        *has_images = SPLIT(split)->drawn > 0;
        // Black ink on white paper becomes coverage: 0 = paper, 255 = ink.
        fz_invert_pixmap(ctx, text_pix);
    }
    fz_always(ctx) {
        fz_drop_device(ctx, split);
        fz_drop_device(ctx, image_dev);
        fz_drop_device(ctx, text_dev);
        fz_drop_pixmap(ctx, image_pix);
        fz_drop_pixmap(ctx, text_pix);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        set_err(ctx, err, errlen);
        status = -1;
    }
    return status;
}

unsigned char *md_render_png(md_doc *d, int page_number, float scale, size_t *len, char *err, int errlen) {
    fz_context *ctx = d->ctx;
    fz_pixmap *pix = NULL;
    fz_buffer *buf = NULL;
    unsigned char *out = NULL;
    fz_var(pix);
    fz_var(buf);
    fz_var(out);
    fz_try(ctx) {
        pix = fz_new_pixmap_from_page_number(ctx, d->doc, page_number, fz_scale(scale, scale),
                                             fz_device_rgb(ctx), 0);
        buf = fz_new_buffer_from_pixmap_as_png(ctx, pix, fz_default_color_params);
        unsigned char *data = NULL;
        size_t n = fz_buffer_storage(ctx, buf, &data);
        out = malloc(n);
        if (out == NULL) {
            fz_throw(ctx, FZ_ERROR_GENERIC, "out of memory");
        }
        memcpy(out, data, n);
        *len = n;
    }
    fz_always(ctx) {
        fz_drop_buffer(ctx, buf);
        fz_drop_pixmap(ctx, pix);
    }
    fz_catch(ctx) {
        free(out);
        out = NULL;
        set_err(ctx, err, errlen);
    }
    return out;
}

int md_chapter_position(md_doc *d, int page, int *chapter, float *fraction) {
    int status = 0;
    fz_var(status);
    fz_try(d->ctx) {
        fz_location loc = fz_location_from_page_number(d->ctx, d->doc, page);
        int pages = fz_count_chapter_pages(d->ctx, d->doc, loc.chapter);
        *chapter = loc.chapter;
        *fraction = pages > 1 ? (float)loc.page / (float)(pages - 1) : 0.0f;
    }
    fz_catch(d->ctx) {
        status = -1;
    }
    return status;
}

int md_page_at(md_doc *d, int chapter, float fraction) {
    int page = -1;
    fz_var(page);
    fz_try(d->ctx) {
        if (chapter >= 0 && chapter < fz_count_chapters(d->ctx, d->doc)) {
            int pages = fz_count_chapter_pages(d->ctx, d->doc, chapter);
            int within = (int)(fraction * (float)(pages - 1) + 0.5f);
            within = within < 0 ? 0 : (within >= pages ? pages - 1 : within);
            page = fz_page_number_from_location(d->ctx, d->doc, fz_make_location(chapter, within));
        }
    }
    fz_catch(d->ctx) {
        page = -1;
    }
    return page;
}

int md_metadata(md_doc *d, const char *key, char *buf, int buflen) {
    int found = 0;
    fz_var(found);
    fz_try(d->ctx) {
        found = fz_lookup_metadata(d->ctx, d->doc, key, buf, (size_t)buflen) > 0;
    }
    fz_catch(d->ctx) {
        found = 0;
    }
    return found;
}

static int count_outline(fz_outline *node) {
    int n = 0;
    for (; node != NULL; node = node->next) {
        n += 1 + count_outline(node->down);
    }
    return n;
}

static char *copy_string(const char *s) {
    if (s == NULL) {
        s = "";
    }
    size_t n = strlen(s) + 1;
    char *out = malloc(n);
    if (out != NULL) {
        memcpy(out, s, n);
    }
    return out;
}

static void flatten_outline(fz_outline *node, int level, md_outline_item *items, int *index) {
    for (; node != NULL; node = node->next) {
        md_outline_item *item = &items[(*index)++];
        item->title = copy_string(node->title);
        item->uri = copy_string(node->uri);
        item->level = level;
        flatten_outline(node->down, level + 1, items, index);
    }
}

int md_outline(md_doc *d, md_outline_item **items, char *err, int errlen) {
    fz_outline *outline = NULL;
    int count = -1;
    fz_var(outline);
    fz_var(count);
    *items = NULL;
    fz_try(d->ctx) {
        outline = fz_load_outline(d->ctx, d->doc);
        count = count_outline(outline);
        if (count > 0) {
            *items = calloc((size_t)count, sizeof **items);
            if (*items == NULL) {
                fz_throw(d->ctx, FZ_ERROR_GENERIC, "out of memory");
            }
            int index = 0;
            flatten_outline(outline, 0, *items, &index);
        }
    }
    fz_always(d->ctx) {
        fz_drop_outline(d->ctx, outline);
    }
    fz_catch(d->ctx) {
        set_err(d->ctx, err, errlen);
        count = -1;
    }
    return count;
}

void md_free_outline(md_outline_item *items, int count) {
    if (items == NULL) {
        return;
    }
    for (int i = 0; i < count; i++) {
        free(items[i].title);
        free(items[i].uri);
    }
    free(items);
}

int md_resolve(md_doc *d, const char *uri) {
    int page = -1;
    fz_var(page);
    fz_try(d->ctx) {
        float x = 0, y = 0;
        fz_location loc = fz_resolve_link(d->ctx, d->doc, uri, &x, &y);
        page = loc.chapter < 0 ? -1 : fz_page_number_from_location(d->ctx, d->doc, loc);
    }
    fz_catch(d->ctx) {
        page = -1;
    }
    return page;
}

int md_count_chapters(md_doc *d) {
    int count = -1;
    fz_var(count);
    fz_try(d->ctx) {
        count = fz_count_chapters(d->ctx, d->doc);
    }
    fz_catch(d->ctx) {
        count = -1;
    }
    return count;
}

int md_chapter_pages(md_doc *d, int chapter) {
    int pages = -1;
    fz_var(pages);
    fz_try(d->ctx) {
        if (chapter >= 0 && chapter < fz_count_chapters(d->ctx, d->doc)) {
            pages = fz_count_chapter_pages(d->ctx, d->doc, chapter);
        }
    }
    fz_catch(d->ctx) {
        pages = -1;
    }
    return pages;
}

int md_location(md_doc *d, int page, int *chapter, int *index) {
    int status = 0;
    fz_var(status);
    fz_try(d->ctx) {
        fz_location loc = fz_location_from_page_number(d->ctx, d->doc, page);
        *chapter = loc.chapter;
        *index = loc.page;
        status = loc.chapter < 0 ? -1 : 0;
    }
    fz_catch(d->ctx) {
        status = -1;
    }
    return status;
}

int md_page_number(md_doc *d, int chapter, int index) {
    int page = -1;
    fz_var(page);
    fz_try(d->ctx) {
        if (chapter >= 0 && chapter < fz_count_chapters(d->ctx, d->doc) && index >= 0 &&
            index < fz_count_chapter_pages(d->ctx, d->doc, chapter)) {
            page = fz_page_number_from_location(d->ctx, d->doc, fz_make_location(chapter, index));
        }
    }
    fz_catch(d->ctx) {
        page = -1;
    }
    return page;
}

// MD_MUPDF_PARAGRAPH_DIST is how far apart, in ems, MuPDF lets the baselines
// of two lines of a text block be.
#define MD_MUPDF_PARAGRAPH_DIST 1.5f

// continues_block reports whether line, the first of a text block, goes on
// the paragraph whose last line is prev, the last of the block before it, as
// MuPDF would judge with a paragraph distance of line_dist ems instead of its
// own: it is horizontal text of the same size, below prev, farther than
// MuPDF allows but no farther than line_dist, and not indented from where
// prev starts.
static int continues_block(fz_stext_line *prev, fz_stext_line *line, float line_dist) {
    if (line_dist <= MD_MUPDF_PARAGRAPH_DIST || prev == NULL || prev->first_char == NULL || line->first_char == NULL ||
        prev->wmode || line->wmode) {
        return 0;
    }
    fz_stext_char *a = prev->first_char, *b = line->first_char;
    float size = a->size;
    if (size <= 0 || fabsf(b->size - size) > 0.01f * size) {
        return 0;
    }
    float down = b->origin.y - a->origin.y;
    return down > MD_MUPDF_PARAGRAPH_DIST * size && down <= line_dist * size && b->origin.x <= a->origin.x + 0.5f;
}

int md_page_chars(md_doc *d, int page_number, float line_dist, md_char **chars, char *err, int errlen) {
    fz_context *ctx = d->ctx;
    fz_stext_page *text = NULL;
    int count = -1;
    fz_var(text);
    fz_var(count);
    *chars = NULL;
    fz_try(ctx) {
        // Zero flags: ligatures expanded, whitespace as spaces.
        fz_stext_options options = {0};
        text = fz_new_stext_page_from_page_number(ctx, d->doc, page_number, &options);
        int n = 0;
        for (fz_stext_block *b = text->first_block; b != NULL; b = b->next) {
            if (b->type != FZ_STEXT_BLOCK_TEXT) {
                continue;
            }
            for (fz_stext_line *l = b->u.t.first_line; l != NULL; l = l->next) {
                for (fz_stext_char *c = l->first_char; c != NULL; c = c->next) {
                    n++;
                }
            }
        }
        if (n > 0) {
            *chars = malloc((size_t)n * sizeof **chars);
            if (*chars == NULL) {
                fz_throw(ctx, FZ_ERROR_GENERIC, "out of memory");
            }
        }
        int i = 0, line = 0, paragraph = -1;
        fz_stext_line *prev = NULL;
        for (fz_stext_block *b = text->first_block; b != NULL; b = b->next) {
            if (b->type != FZ_STEXT_BLOCK_TEXT || b->u.t.first_line == NULL) {
                continue;
            }
            if (paragraph < 0 || !continues_block(prev, b->u.t.first_line, line_dist)) {
                paragraph++;
            }
            for (fz_stext_line *l = b->u.t.first_line; l != NULL; l = l->next, line++) {
                for (fz_stext_char *c = l->first_char; c != NULL; c = c->next) {
                    fz_rect r = fz_rect_from_quad(c->quad);
                    (*chars)[i++] = (md_char){c->c, r.x0, r.y0, r.x1, r.y1, line, paragraph};
                }
                prev = l;
            }
        }
        count = n;
    }
    fz_always(ctx) {
        fz_drop_stext_page(ctx, text);
    }
    fz_catch(ctx) {
        free(*chars);
        *chars = NULL;
        set_err(ctx, err, errlen);
        count = -1;
    }
    return count;
}

void md_free(void *p) {
    free(p);
}

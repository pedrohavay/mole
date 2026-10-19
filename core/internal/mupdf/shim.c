#include "shim.h"

#include <math.h>
#include <mupdf/fitz.h>
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

// md_image_style remembers how an image of the document is shown at night:
// line art inverted, everything else as it is.
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
    md_budget *budget = malloc(sizeof *budget);
    if (budget == NULL) {
        snprintf(err, (size_t)errlen, "out of memory");
        return NULL;
    }
    *budget = (md_budget){0, limit};
    fz_alloc_context alloc = {budget, budget_malloc, budget_realloc, budget_free};
    fz_context *ctx = fz_new_context(&alloc, &locks, MD_STORE_SIZE);
    if (ctx == NULL) {
        free(budget);
        snprintf(err, (size_t)errlen, "cannot create MuPDF context");
        return NULL;
    }
    fz_set_warning_callback(ctx, quiet, NULL);
    fz_set_error_callback(ctx, quiet, NULL);
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
        fz_drop_context(ctx);
        free(budget);
        return NULL;
    }
    md_doc *d = calloc(1, sizeof *d);
    if (d == NULL) {
        fz_drop_document(ctx, doc);
        fz_drop_context(ctx);
        free(budget);
        snprintf(err, (size_t)errlen, "out of memory");
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

// The image layer shows images and shadings on a black page, in color. At
// night a photograph or a dark picture is drawn as it is, so a dark cover
// stays dark, while line art (a diagram, dark lines on white) would be a
// sheet of light, so its lightness is inverted and its lines glow instead.
// In the original style every image is drawn as on a white page.

// gray_pixmap converts pix to gray with alpha, keeping it.
static fz_pixmap *gray_pixmap(fz_context *ctx, fz_pixmap *pix) {
    return fz_convert_pixmap(ctx, pix, fz_device_gray(ctx), NULL, NULL, fz_default_color_params, 1);
}

// MD_PAPER is the gray, out of 255, from which a pixel counts as bare
// paper, and MD_MIN_PAPER the share of bare paper that line art has at least.
#define MD_PAPER 217
#define MD_MIN_PAPER 0.3

// is_line_art reports whether an image looks like line art, judged from a
// small copy of it composited on white, as it would appear on a white page:
// mostly light, with a good share of bare paper. A bright photograph is
// light too, but has little pure white, and keeps its tones.
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
            long count = 0, paper = 0;
            for (int y = 0; y < h; y += dy) {
                const unsigned char *row = samples + y * stride;
                for (int x = 0; x < w; x += dx) {
                    const unsigned char *p = row + x * n;
                    // Premultiplied gray over white.
                    int g = alpha ? p[0] + (255 - p[1]) : p[0];
                    sum += g;
                    paper += g >= MD_PAPER;
                    count++;
                }
            }
            light = count > 0 && sum / count > 0.5 * 255 && (double)paper / count >= MD_MIN_PAPER;
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

// invert_lightness inverts the lightness of every pixel of pix, a gray or RGB
// pixmap with premultiplied alpha, and keeps its hue and chroma: black turns
// white and white black, a red stays red and navy turns light blue. Each
// colorant moves by alpha - max - min, which takes the lightest colorant to
// where the darkest was and the other way around.
static void invert_lightness(fz_context *ctx, fz_pixmap *pix) {
    int w = fz_pixmap_width(ctx, pix), h = fz_pixmap_height(ctx, pix);
    int n = fz_pixmap_components(ctx, pix), alpha = fz_pixmap_alpha(ctx, pix);
    int colorants = fz_pixmap_colorants(ctx, pix);
    ptrdiff_t stride = fz_pixmap_stride(ctx, pix);
    unsigned char *samples = fz_pixmap_samples(ctx, pix);
    for (int y = 0; y < h; y++) {
        unsigned char *p = samples + y * stride;
        for (int x = 0; x < w; x++, p += n) {
            int a = alpha ? p[n - 1] : 255;
            int hi = p[0], lo = p[0];
            for (int k = 1; k < colorants; k++) {
                hi = p[k] > hi ? p[k] : hi;
                lo = p[k] < lo ? p[k] : lo;
            }
            int shift = a - hi - lo;
            for (int k = 0; k < colorants; k++) {
                int v = p[k] + shift;
                p[k] = (unsigned char)(v < 0 ? 0 : (v > a ? a : v));
            }
        }
    }
}

// MD_MAX_INVERTED is the largest side, in pixels, of an inverted copy.
#define MD_MAX_INVERTED 2048

// inverted_image returns a copy of image with its lightness inverted (see
// invert_lightness), keeping its transparency.
static fz_image *inverted_image(fz_context *ctx, fz_image *image) {
    fz_pixmap *pix = NULL;
    fz_pixmap *copy = NULL;
    fz_image *out = NULL;
    fz_var(pix);
    fz_var(copy);
    fz_try(ctx) {
        fz_matrix ctm = fz_scale(image->w, image->h);
        int side = image->w > image->h ? image->w : image->h;
        if (side > MD_MAX_INVERTED) {
            float shrink = (float)MD_MAX_INVERTED / (float)side;
            ctm = fz_scale(image->w * shrink, image->h * shrink);
        }
        pix = fz_get_pixmap_from_image(ctx, image, NULL, &ctm, NULL, NULL);
        // MuPDF may share the decoded pixmap with its cache, so the copy is
        // inverted, in RGB unless the image is gray.
        fz_colorspace *cs = fz_pixmap_colorspace(ctx, pix);
        if (fz_colorspace_is_gray(ctx, cs) || fz_colorspace_is_rgb(ctx, cs)) {
            copy = fz_clone_pixmap(ctx, pix);
        } else {
            copy = fz_convert_pixmap(ctx, pix, fz_device_rgb(ctx), NULL, NULL, fz_default_color_params, 1);
        }
        invert_lightness(ctx, copy);
        out = fz_new_image_from_pixmap(ctx, copy, NULL);
    }
    fz_always(ctx) {
        fz_drop_pixmap(ctx, copy);
        fz_drop_pixmap(ctx, pix);
    }
    fz_catch(ctx) {
        fz_rethrow(ctx);
    }
    return out;
}

// image_to_show returns the image to draw at night for image, remembering
// the decision, and the inverted copy, for later renders.
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

// fill_paper fills the unit square under ctm, where an image goes, with
// white, so that a transparent image shows as on a white page.
static void fill_paper(fz_context *ctx, fz_device *dev, fz_matrix ctm, float alpha, fz_color_params cp) {
    static const float white[FZ_MAX_COLORS] = {1, 1, 1};
    fz_path *path = fz_new_path(ctx);
    fz_try(ctx) {
        fz_rectto(ctx, path, 0, 0, 1, 1);
        fz_fill_path(ctx, dev, path, 0, ctm, fz_device_rgb(ctx), white, alpha, cp);
    }
    fz_always(ctx) {
        fz_drop_path(ctx, path);
    }
    fz_catch(ctx) {
        fz_rethrow(ctx);
    }
}

// MD_MIN_CHROMA is how far apart, from 0 to 1, the strongest and the
// weakest channel of a color are at least for it to count as a hue rather
// than a gray. MD_DARKEST and MD_LIGHTEST bound the lightness of the hues
// kept: darker colors count as black and lighter ones as white.
#define MD_MIN_CHROMA 0.12f
#define MD_DARKEST 0.12f
#define MD_LIGHTEST 0.92f

// book_color converts the color of text or a line to the RGB it glows in on
// a black page into rgb, and reports whether it is a hue to keep. Black,
// white, grays and colors too dark or too light to tell are left to the
// reader's tint. A dark color, chosen to stand out on white paper, has its
// lightness mirrored so that it stands out on black as much, with its hue
// and chroma kept.
static int book_color(fz_context *ctx, fz_colorspace *cs, const float *color, fz_color_params cp, float *rgb) {
    if (cs == NULL || color == NULL) {
        return 0;
    }
    fz_convert_color(ctx, cs, color, fz_device_rgb(ctx), rgb, NULL, cp);
    float hi = fmaxf(rgb[0], fmaxf(rgb[1], rgb[2]));
    float lo = fminf(rgb[0], fminf(rgb[1], rgb[2]));
    float lightness = (hi + lo) / 2;
    if (hi - lo < MD_MIN_CHROMA || lightness < MD_DARKEST || lightness > MD_LIGHTEST) {
        return 0;
    }
    if (lightness < 0.5f) {
        float shift = 1 - hi - lo;
        for (int i = 0; i < 3; i++) {
            rgb[i] += shift;
        }
    }
    return 1;
}

// split_device sends text and vector paths to the text device, and those in
// a hue also to the colors device; images and shadings go to the images
// device. Clips, groups and masks go to all three, and so does everything
// inside a mask definition, as it is, so that all see the same clipping.
typedef struct {
    fz_device super;
    md_doc *doc;
    fz_device *text;
    fz_device *colors;
    fz_device *images;
    int keep_colors; // send text and lines in a hue to colors
    int pictures;    // MD_PICTURES_*
    int masking;     // depth of mask definitions being drawn
    int colored;     // text and lines sent to the colors device
    int drawn;       // images and shadings sent to the images device
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
// (white on a dark box, gray captions) still shows as light. Its hue, if it
// has one, goes to the colors layer.
static const float black[FZ_MAX_COLORS] = {0};

static void split_fill_path(fz_context *ctx, fz_device *dev, const fz_path *path, int even_odd, fz_matrix ctm,
                            fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    split_device *s = SPLIT(dev);
    if (s->masking) {
        fz_fill_path(ctx, s->text, path, even_odd, ctm, cs, color, alpha, cp);
        fz_fill_path(ctx, s->colors, path, even_odd, ctm, cs, color, alpha, cp);
        fz_fill_path(ctx, s->images, path, even_odd, ctm, cs, color, alpha, cp);
        return;
    }
    if (is_box(ctx, path, ctm)) {
        return;
    }
    fz_fill_path(ctx, s->text, path, even_odd, ctm, cs, color, alpha, cp);
    float rgb[FZ_MAX_COLORS] = {0};
    if (s->keep_colors && book_color(ctx, cs, color, cp, rgb)) {
        fz_fill_path(ctx, s->colors, path, even_odd, ctm, fz_device_rgb(ctx), rgb, alpha, cp);
        s->colored++;
    }
}

static void split_stroke_path(fz_context *ctx, fz_device *dev, const fz_path *path, const fz_stroke_state *stroke,
                              fz_matrix ctm, fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    split_device *s = SPLIT(dev);
    fz_stroke_path(ctx, s->text, path, stroke, ctm, cs, color, alpha, cp);
    if (s->masking) {
        fz_stroke_path(ctx, s->colors, path, stroke, ctm, cs, color, alpha, cp);
        fz_stroke_path(ctx, s->images, path, stroke, ctm, cs, color, alpha, cp);
        return;
    }
    float rgb[FZ_MAX_COLORS] = {0};
    if (s->keep_colors && book_color(ctx, cs, color, cp, rgb)) {
        fz_stroke_path(ctx, s->colors, path, stroke, ctm, fz_device_rgb(ctx), rgb, alpha, cp);
        s->colored++;
    }
}

static void split_clip_path(fz_context *ctx, fz_device *dev, const fz_path *path, int even_odd, fz_matrix ctm,
                            fz_rect scissor) {
    fz_clip_path(ctx, SPLIT(dev)->text, path, even_odd, ctm, scissor);
    fz_clip_path(ctx, SPLIT(dev)->colors, path, even_odd, ctm, scissor);
    fz_clip_path(ctx, SPLIT(dev)->images, path, even_odd, ctm, scissor);
}

static void split_clip_stroke_path(fz_context *ctx, fz_device *dev, const fz_path *path,
                                   const fz_stroke_state *stroke, fz_matrix ctm, fz_rect scissor) {
    fz_clip_stroke_path(ctx, SPLIT(dev)->text, path, stroke, ctm, scissor);
    fz_clip_stroke_path(ctx, SPLIT(dev)->colors, path, stroke, ctm, scissor);
    fz_clip_stroke_path(ctx, SPLIT(dev)->images, path, stroke, ctm, scissor);
}

static void split_fill_text(fz_context *ctx, fz_device *dev, const fz_text *text, fz_matrix ctm, fz_colorspace *cs,
                            const float *color, float alpha, fz_color_params cp) {
    split_device *s = SPLIT(dev);
    if (s->masking) {
        fz_fill_text(ctx, s->text, text, ctm, cs, color, alpha, cp);
        fz_fill_text(ctx, s->colors, text, ctm, cs, color, alpha, cp);
        fz_fill_text(ctx, s->images, text, ctm, cs, color, alpha, cp);
        return;
    }
    fz_fill_text(ctx, s->text, text, ctm, fz_device_gray(ctx), black, alpha, cp);
    float rgb[FZ_MAX_COLORS] = {0};
    if (s->keep_colors && book_color(ctx, cs, color, cp, rgb)) {
        fz_fill_text(ctx, s->colors, text, ctm, fz_device_rgb(ctx), rgb, alpha, cp);
        s->colored++;
    }
}

static void split_stroke_text(fz_context *ctx, fz_device *dev, const fz_text *text, const fz_stroke_state *stroke,
                              fz_matrix ctm, fz_colorspace *cs, const float *color, float alpha,
                              fz_color_params cp) {
    split_device *s = SPLIT(dev);
    if (s->masking) {
        fz_stroke_text(ctx, s->text, text, stroke, ctm, cs, color, alpha, cp);
        fz_stroke_text(ctx, s->colors, text, stroke, ctm, cs, color, alpha, cp);
        fz_stroke_text(ctx, s->images, text, stroke, ctm, cs, color, alpha, cp);
        return;
    }
    fz_stroke_text(ctx, s->text, text, stroke, ctm, fz_device_gray(ctx), black, alpha, cp);
    float rgb[FZ_MAX_COLORS] = {0};
    if (s->keep_colors && book_color(ctx, cs, color, cp, rgb)) {
        fz_stroke_text(ctx, s->colors, text, stroke, ctm, fz_device_rgb(ctx), rgb, alpha, cp);
        s->colored++;
    }
}

static void split_clip_text(fz_context *ctx, fz_device *dev, const fz_text *text, fz_matrix ctm, fz_rect scissor) {
    fz_clip_text(ctx, SPLIT(dev)->text, text, ctm, scissor);
    fz_clip_text(ctx, SPLIT(dev)->colors, text, ctm, scissor);
    fz_clip_text(ctx, SPLIT(dev)->images, text, ctm, scissor);
}

static void split_clip_stroke_text(fz_context *ctx, fz_device *dev, const fz_text *text,
                                   const fz_stroke_state *stroke, fz_matrix ctm, fz_rect scissor) {
    fz_clip_stroke_text(ctx, SPLIT(dev)->text, text, stroke, ctm, scissor);
    fz_clip_stroke_text(ctx, SPLIT(dev)->colors, text, stroke, ctm, scissor);
    fz_clip_stroke_text(ctx, SPLIT(dev)->images, text, stroke, ctm, scissor);
}

static void split_fill_shade(fz_context *ctx, fz_device *dev, fz_shade *shade, fz_matrix ctm, float alpha,
                             fz_color_params cp) {
    split_device *s = SPLIT(dev);
    if (s->masking) {
        fz_fill_shade(ctx, s->text, shade, ctm, alpha, cp);
        fz_fill_shade(ctx, s->colors, shade, ctm, alpha, cp);
    } else if (s->pictures == MD_PICTURES_HIDDEN) {
        return;
    } else {
        s->drawn++;
    }
    fz_fill_shade(ctx, s->images, shade, ctm, alpha, cp);
}

static void split_fill_image(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm, float alpha,
                             fz_color_params cp) {
    split_device *s = SPLIT(dev);
    if (s->masking) {
        fz_fill_image(ctx, s->text, image, ctm, alpha, cp);
        fz_fill_image(ctx, s->colors, image, ctm, alpha, cp);
        fz_fill_image(ctx, s->images, image, ctm, alpha, cp);
        return;
    }
    switch (s->pictures) {
    case MD_PICTURES_HIDDEN:
        return;
    case MD_PICTURES_ORIGINAL:
        fill_paper(ctx, s->images, ctm, alpha, cp);
        fz_fill_image(ctx, s->images, image, ctm, alpha, cp);
        break;
    default:
        fz_fill_image(ctx, s->images, image_to_show(ctx, s->doc, image), ctm, alpha, cp);
    }
    s->drawn++;
}

// An image mask is a stencil painted in one color, like a glyph, so it
// belongs with the text.
static void split_fill_image_mask(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm,
                                  fz_colorspace *cs, const float *color, float alpha, fz_color_params cp) {
    fz_fill_image_mask(ctx, SPLIT(dev)->text, image, ctm, cs, color, alpha, cp);
    if (SPLIT(dev)->masking) {
        fz_fill_image_mask(ctx, SPLIT(dev)->colors, image, ctm, cs, color, alpha, cp);
        fz_fill_image_mask(ctx, SPLIT(dev)->images, image, ctm, cs, color, alpha, cp);
    }
}

static void split_clip_image_mask(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm, fz_rect scissor) {
    fz_clip_image_mask(ctx, SPLIT(dev)->text, image, ctm, scissor);
    fz_clip_image_mask(ctx, SPLIT(dev)->colors, image, ctm, scissor);
    fz_clip_image_mask(ctx, SPLIT(dev)->images, image, ctm, scissor);
}

static void split_pop_clip(fz_context *ctx, fz_device *dev) {
    fz_pop_clip(ctx, SPLIT(dev)->text);
    fz_pop_clip(ctx, SPLIT(dev)->colors);
    fz_pop_clip(ctx, SPLIT(dev)->images);
}

static void split_begin_mask(fz_context *ctx, fz_device *dev, fz_rect area, int luminosity, fz_colorspace *cs,
                             const float *bc, fz_color_params cp) {
    fz_begin_mask(ctx, SPLIT(dev)->text, area, luminosity, cs, bc, cp);
    fz_begin_mask(ctx, SPLIT(dev)->colors, area, luminosity, cs, bc, cp);
    fz_begin_mask(ctx, SPLIT(dev)->images, area, luminosity, cs, bc, cp);
    SPLIT(dev)->masking++;
}

static void split_end_mask(fz_context *ctx, fz_device *dev, fz_function *fn) {
    SPLIT(dev)->masking--;
    fz_end_mask_tr(ctx, SPLIT(dev)->text, fn);
    fz_end_mask_tr(ctx, SPLIT(dev)->colors, fn);
    fz_end_mask_tr(ctx, SPLIT(dev)->images, fn);
}

static void split_begin_group(fz_context *ctx, fz_device *dev, fz_rect area, fz_colorspace *cs, int isolated,
                              int knockout, int blendmode, float alpha) {
    fz_begin_group(ctx, SPLIT(dev)->text, area, cs, isolated, knockout, blendmode, alpha);
    fz_begin_group(ctx, SPLIT(dev)->colors, area, cs, isolated, knockout, blendmode, alpha);
    fz_begin_group(ctx, SPLIT(dev)->images, area, cs, isolated, knockout, blendmode, alpha);
}

static void split_end_group(fz_context *ctx, fz_device *dev) {
    fz_end_group(ctx, SPLIT(dev)->text);
    fz_end_group(ctx, SPLIT(dev)->colors);
    fz_end_group(ctx, SPLIT(dev)->images);
}

// Tiles are never cached (id 0), so every device always wants the content.
static int split_begin_tile(fz_context *ctx, fz_device *dev, fz_rect area, fz_rect view, float xstep, float ystep,
                            fz_matrix ctm, int id) {
    (void)id;
    fz_begin_tile_id(ctx, SPLIT(dev)->text, area, view, xstep, ystep, ctm, 0);
    fz_begin_tile_id(ctx, SPLIT(dev)->colors, area, view, xstep, ystep, ctm, 0);
    fz_begin_tile_id(ctx, SPLIT(dev)->images, area, view, xstep, ystep, ctm, 0);
    return 0;
}

static void split_end_tile(fz_context *ctx, fz_device *dev) {
    fz_end_tile(ctx, SPLIT(dev)->text);
    fz_end_tile(ctx, SPLIT(dev)->colors);
    fz_end_tile(ctx, SPLIT(dev)->images);
}

static void split_render_flags(fz_context *ctx, fz_device *dev, int set, int clear) {
    fz_render_flags(ctx, SPLIT(dev)->text, set, clear);
    fz_render_flags(ctx, SPLIT(dev)->colors, set, clear);
    fz_render_flags(ctx, SPLIT(dev)->images, set, clear);
}

static void split_set_default_colorspaces(fz_context *ctx, fz_device *dev, fz_default_colorspaces *cs) {
    fz_set_default_colorspaces(ctx, SPLIT(dev)->text, cs);
    fz_set_default_colorspaces(ctx, SPLIT(dev)->colors, cs);
    fz_set_default_colorspaces(ctx, SPLIT(dev)->images, cs);
}

static fz_device *new_split_device(fz_context *ctx, md_doc *d, fz_device *text, fz_device *colors,
                                   fz_device *images) {
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
    dev->colors = colors;
    dev->images = images;
    return &dev->super;
}

// layer_device returns a draw device for an RGBA layer, borrowing the
// caller's buffer, or a device that draws nothing when there is no buffer.
static fz_device *layer_device(fz_context *ctx, fz_matrix ctm, const md_layers *layers, unsigned char *samples,
                               fz_pixmap **pix) {
    if (samples == NULL) {
        return fz_new_derived_device(ctx, fz_device);
    }
    *pix = fz_new_pixmap_with_data(ctx, fz_device_rgb(ctx), layers->width, layers->height, NULL, 1,
                                   4 * layers->stride, samples);
    return fz_new_draw_device(ctx, ctm, *pix);
}

int md_render_layers(md_doc *d, int page_number, float scale, const md_layers *layers, int pictures, int *has_colors,
                     int *has_images, char *err, int errlen) {
    fz_context *ctx = d->ctx;
    fz_page *page = NULL;
    fz_pixmap *text_pix = NULL;
    fz_pixmap *color_pix = NULL;
    fz_pixmap *image_pix = NULL;
    fz_device *text_dev = NULL;
    fz_device *color_dev = NULL;
    fz_device *image_dev = NULL;
    fz_device *split = NULL;
    int status = 0;
    fz_var(page);
    fz_var(text_pix);
    fz_var(color_pix);
    fz_var(image_pix);
    fz_var(text_dev);
    fz_var(color_dev);
    fz_var(image_dev);
    fz_var(split);
    *has_colors = 0;
    *has_images = 0;
    if (layers->images == NULL) {
        pictures = MD_PICTURES_HIDDEN;
    }
    fz_try(ctx) {
        page = fz_load_page(ctx, d->doc, page_number);
        fz_matrix ctm = fz_scale(scale, scale);
        // The pixmaps borrow the caller's buffers, so dropping them frees nothing.
        text_pix = fz_new_pixmap_with_data(ctx, fz_device_gray(ctx), layers->width, layers->height, NULL, 0,
                                           layers->stride, layers->text);
        fz_clear_pixmap_with_value(ctx, text_pix, 255);
        text_dev = fz_new_draw_device(ctx, ctm, text_pix);
        color_dev = layer_device(ctx, ctm, layers, layers->colors, &color_pix);
        image_dev = layer_device(ctx, ctm, layers, layers->images, &image_pix);
        split = new_split_device(ctx, d, text_dev, color_dev, image_dev);
        SPLIT(split)->keep_colors = layers->colors != NULL;
        SPLIT(split)->pictures = pictures;
        fz_run_page(ctx, page, split, fz_identity, NULL);
        fz_close_device(ctx, split);
        fz_close_device(ctx, text_dev);
        fz_close_device(ctx, color_dev);
        fz_close_device(ctx, image_dev);
        *has_colors = SPLIT(split)->colored > 0;
        *has_images = SPLIT(split)->drawn > 0;
        // Black ink on white paper becomes coverage: 0 = paper, 255 = ink.
        fz_invert_pixmap(ctx, text_pix);
    }
    fz_always(ctx) {
        fz_drop_device(ctx, split);
        fz_drop_device(ctx, image_dev);
        fz_drop_device(ctx, color_dev);
        fz_drop_device(ctx, text_dev);
        fz_drop_pixmap(ctx, image_pix);
        fz_drop_pixmap(ctx, color_pix);
        fz_drop_pixmap(ctx, text_pix);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        set_err(ctx, err, errlen);
        status = -1;
    }
    return status;
}

// image_finder is a device that only looks at the images of a page, outside
// masks and patterns: it lists where they are, or keeps the one at index want.
typedef struct {
    fz_device super;
    int hidden;      // depth of masks and tiles being drawn
    int count;       // images seen so far
    md_rect *boxes;  // where the images are, when listing (want < 0)
    int cap;         // room in boxes
    int want;        // index of the image to keep, or -1 to list them
    fz_image *image; // the image kept
    fz_matrix ctm;   // where the image kept is drawn
} image_finder;

#define FINDER(dev) ((image_finder *)(dev))

static void find_fill_image(fz_context *ctx, fz_device *dev, fz_image *image, fz_matrix ctm, float alpha,
                            fz_color_params cp) {
    (void)alpha;
    (void)cp;
    image_finder *f = FINDER(dev);
    if (f->hidden) {
        return;
    }
    if (f->want < 0) {
        if (f->count == f->cap) {
            int cap = f->cap > 0 ? 2 * f->cap : 8;
            md_rect *boxes = realloc(f->boxes, (size_t)cap * sizeof *boxes);
            if (boxes == NULL) {
                fz_throw(ctx, FZ_ERROR_SYSTEM, "out of memory");
            }
            f->boxes = boxes;
            f->cap = cap;
        }
        fz_rect r = fz_transform_rect(fz_unit_rect, ctm);
        f->boxes[f->count] = (md_rect){r.x0, r.y0, r.x1, r.y1};
    } else if (f->count == f->want) {
        f->image = fz_keep_image(ctx, image);
        f->ctm = ctm;
    }
    f->count++;
}

static void find_begin_mask(fz_context *ctx, fz_device *dev, fz_rect area, int luminosity, fz_colorspace *cs,
                            const float *bc, fz_color_params cp) {
    (void)ctx, (void)area, (void)luminosity, (void)cs, (void)bc, (void)cp;
    FINDER(dev)->hidden++;
}

static void find_end_mask(fz_context *ctx, fz_device *dev, fz_function *fn) {
    (void)ctx, (void)fn;
    FINDER(dev)->hidden--;
}

static int find_begin_tile(fz_context *ctx, fz_device *dev, fz_rect area, fz_rect view, float xstep, float ystep,
                           fz_matrix ctm, int id) {
    (void)ctx, (void)area, (void)view, (void)xstep, (void)ystep, (void)ctm, (void)id;
    FINDER(dev)->hidden++;
    return 0;
}

static void find_end_tile(fz_context *ctx, fz_device *dev) {
    (void)ctx;
    FINDER(dev)->hidden--;
}

static void find_drop(fz_context *ctx, fz_device *dev) {
    fz_drop_image(ctx, FINDER(dev)->image);
}

// find_images runs a page through an image finder that keeps the image at
// index want, or lists them all when want is negative. The caller drops
// the finder and frees its boxes.
static image_finder *find_images(fz_context *ctx, fz_page *page, int want) {
    image_finder *f = fz_new_derived_device(ctx, image_finder);
    f->super.fill_image = find_fill_image;
    f->super.begin_mask = find_begin_mask;
    f->super.end_mask = find_end_mask;
    f->super.begin_tile = find_begin_tile;
    f->super.end_tile = find_end_tile;
    f->super.drop_device = find_drop;
    f->want = want;
    fz_try(ctx) {
        fz_run_page(ctx, page, &f->super, fz_identity, NULL);
        fz_close_device(ctx, &f->super);
    }
    fz_catch(ctx) {
        free(f->boxes);
        fz_drop_device(ctx, &f->super);
        fz_rethrow(ctx);
    }
    return f;
}

int md_page_images(md_doc *d, int page_number, md_rect **boxes, char *err, int errlen) {
    fz_context *ctx = d->ctx;
    fz_page *page = NULL;
    image_finder *f = NULL;
    int count = -1;
    fz_var(page);
    fz_var(f);
    fz_var(count);
    *boxes = NULL;
    fz_try(ctx) {
        page = fz_load_page(ctx, d->doc, page_number);
        f = find_images(ctx, page, -1);
        *boxes = f->boxes;
        count = f->count;
    }
    fz_always(ctx) {
        fz_drop_device(ctx, (fz_device *)f);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        set_err(ctx, err, errlen);
        count = -1;
    }
    return count;
}

unsigned char *md_image_pixels(md_doc *d, int page_number, int index, int max_side, int *width, int *height, char *err,
                               int errlen) {
    fz_context *ctx = d->ctx;
    fz_page *page = NULL;
    image_finder *f = NULL;
    fz_pixmap *pix = NULL;
    fz_device *dev = NULL;
    unsigned char *out = NULL;
    fz_var(page);
    fz_var(f);
    fz_var(pix);
    fz_var(dev);
    fz_var(out);
    fz_try(ctx) {
        page = fz_load_page(ctx, d->doc, page_number);
        f = find_images(ctx, page, index);
        if (f->image == NULL) {
            fz_throw(ctx, FZ_ERROR_ARGUMENT, "no image %d on page %d", index, page_number);
        }
        fz_rect box = fz_transform_rect(fz_unit_rect, f->ctm);
        float bw = box.x1 - box.x0, bh = box.y1 - box.y0;
        if (bw <= 0 || bh <= 0 || max_side <= 0) {
            fz_throw(ctx, FZ_ERROR_ARGUMENT, "image %d on page %d has no area", index, page_number);
        }
        // Pixels per point: the image's own, or what fills max_side for a
        // drawing that scales, and never more than max_side pixels.
        float longer = bw > bh ? bw : bh;
        float density = sqrtf((float)f->image->w * (float)f->image->h / (bw * bh));
        if (f->image->scalable || longer * density > max_side) {
            density = (float)max_side / longer;
        }
        int w = (int)(bw * density + 0.5f), h = (int)(bh * density + 0.5f);
        w = w < 1 ? 1 : w;
        h = h < 1 ? 1 : h;
        pix = fz_new_pixmap(ctx, fz_device_rgb(ctx), w, h, NULL, 1);
        fz_clear_pixmap_with_value(ctx, pix, 255); // opaque white paper
        fz_matrix ctm = fz_concat(f->ctm, fz_translate(-box.x0, -box.y0));
        ctm = fz_concat(ctm, fz_scale((float)w / bw, (float)h / bh));
        dev = fz_new_draw_device(ctx, fz_identity, pix);
        fz_fill_image(ctx, dev, f->image, ctm, 1, fz_default_color_params);
        fz_close_device(ctx, dev);
        size_t row = (size_t)w * 4;
        out = malloc(row * (size_t)h);
        if (out == NULL) {
            fz_throw(ctx, FZ_ERROR_SYSTEM, "out of memory");
        }
        const unsigned char *samples = fz_pixmap_samples(ctx, pix);
        ptrdiff_t stride = fz_pixmap_stride(ctx, pix);
        for (int y = 0; y < h; y++) {
            memcpy(out + (size_t)y * row, samples + y * stride, row);
        }
        *width = w;
        *height = h;
    }
    fz_always(ctx) {
        fz_drop_device(ctx, dev);
        fz_drop_pixmap(ctx, pix);
        if (f != NULL) {
            free(f->boxes);
        }
        fz_drop_device(ctx, (fz_device *)f);
        fz_drop_page(ctx, page);
    }
    fz_catch(ctx) {
        free(out);
        out = NULL;
        set_err(ctx, err, errlen);
    }
    return out;
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

int md_page_chars(md_doc *d, int page_number, md_char **chars, char *err, int errlen) {
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
        int i = 0, line = 0, block = 0;
        for (fz_stext_block *b = text->first_block; b != NULL; b = b->next) {
            if (b->type != FZ_STEXT_BLOCK_TEXT) {
                continue;
            }
            for (fz_stext_line *l = b->u.t.first_line; l != NULL; l = l->next, line++) {
                for (fz_stext_char *c = l->first_char; c != NULL; c = c->next) {
                    fz_rect r = fz_rect_from_quad(c->quad);
                    (*chars)[i++] = (md_char){c->c, r.x0, r.y0, r.x1, r.y1, line, block};
                }
            }
            block++;
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

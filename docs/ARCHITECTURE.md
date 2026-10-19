# Architecture

Mole is two programs linked into one binary. The Go core owns the data and
the book: the SQLite library, EPUB import, layout with MuPDF and the
rasterization of pages into a coverage mask and color layers. The Swift app
owns everything the user sees: the window, the library grid, the tabs and the
Metal view that turns the layers into light. They meet at a small C API.

```
SwiftUI views ── AppModel ── Bridge (Library, Session) ── C API ── Go core ── MuPDF
                                                                     └─────── SQLite
EDRRenderer (Metal) ◀── coverage mask (one byte per pixel) ◀──────────┤
                    ◀── text colors and pictures (RGBA) ◀─────────────┘
```

## The C API

`core/cmd/molecore` is built with `go build -buildmode=c-archive`, which
produces a static library and `libmolecore.h`. A c-archive does not carry
the C libraries its cgo code links, so the Makefile merges MuPDF's two
static libraries into it with `libtool`, giving `libmolecore.a`: the app
links that one library and never needs to know where MuPDF is. The header
is copied into `app/Sources/MoleCore`, a system library module the app
imports.

MuPDF itself is a pinned source release (`scripts/build-mupdf.sh`) built
with a deployment target of macOS 14. A Homebrew bottle would target the
macOS of the machine that built it and change with every upgrade.

- Objects are int64 handles (0 means failure). A library handle owns the
  database; a session handle is an open book.
- Structured results are JSON strings. Swift decodes them into `Codable`
  types (`Book`, `ReaderSettings`, `SessionState`, `Chapter`).
- Every string or buffer the core returns belongs to the caller and is
  released with `mole_free`. The exception is the layers a session renders
  (the coverage mask, the text colors and the pictures), which the session
  owns and reuses until its next render, layout or close.
- On failure a function returns 0 or NULL and `mole_last_error` explains why;
  `mole_last_error_kind` tells a book protected by DRM from other failures.
- Library calls (`mole_library_*`, import included) may come from any
  thread, one at a time per handle; a session is used from one thread at a
  time. The last error belongs to the calling thread. The app uses its main
  library handle on the main thread and opens a second one for importing on
  a background queue (`Importer`).

`app/Sources/Mole/Bridge` wraps these conventions: `CoreCall` holds the error
and memory rules, `Library` and `Session` are the handle classes, and a
`Session` closes (saving the position) when it is released.

## Book formats

Package `bookfile` knows the formats. It tells a file's format from its
content first (a ZIP archive, a Palm database of type `BOOKMOBI` or
`TEXtREAd`, an XML document with a `FictionBook` root, a `%PDF-` header)
and from its extension only when the content has no such mark, which is
the case of EPUB files that MuPDF would reject anyway, of PDFs with bytes
before their header and of plain text. The library's
copy keeps the extension, so MuPDF picks the same handler when the book is
opened.

MuPDF reads MOBI, PRC and FB2 itself, as one HTML document whose pictures
(a MOBI's image records, an FB2's `binary` elements) are images like an
EPUB's, so they take the same color and picture layers. It makes them
books of a single chapter: anchors all have chapter 0, and a relayout
measures every page up to the reader's place (about 0.3 s for a book of a
thousand pages). Plain text goes through Go first: it is decoded as UTF-16
when a byte order mark says so, as UTF-8 when it is valid UTF-8, and as
Windows-1252 otherwise, then turned into HTML paragraphs (wrapped lines are
joined, short lines such as verse keep their breaks, and lines like
"CHAPTER IV" become headings for the table of contents) and opened from
memory.

Metadata comes from each format's own fields: the OPF package of an EPUB,
the EXTH records of a MOBI (title, author and cover image), the
`description` of an FB2 with its `binary` cover, and the `Title:` and
`Author:` lines of a Project Gutenberg text. MuPDF's title fills a gap, and
the file name is the last resort; a PDF's title and author come from MuPDF
alone, out of its Info dictionary. A book without a cover gets its first
page, which for a PDF is drawn at its own proportions to fit where a laid
out page would.

MuPDF ignores the encryption field of a MOBI header and would show the
encrypted text, so `bookfile` checks it on import and on open and fails
with `ErrDRM`. A PDF that MuPDF cannot read without a password (an owner
password alone does not stop it) fails with `ErrPassword`. The C API
reports both kinds of failure through `mole_last_error_kind`, and the app
explains them in the user's language.

An FB2 with a cover would also show an empty page between the cover and
the text (see the next section), in the middle of its single chapter, so
`bookfile` adds styles to FB2 books that remove the margins causing it.

## Empty pages at the start of a chapter

MuPDF breaks the page before an element only when the element is below the
top of the page, and it moves the top margins of a chapter's first elements
up to the chapter's root. A chapter whose first element breaks the page, a
common style for chapter titles, therefore starts with an empty page. After
each layout the shim checks the first pages of every chapter of a
reflowable book and leaves out those that draw nothing; every page number
in the core counts only the pages shown. CSS cannot do it, because the
collapsed margin comes from the book's own elements.

## Fixed pages

A PDF has fixed pages: MuPDF does not reflow it, and the font size and the
typography do not apply. `reading.Session.SetTypography` does nothing to
such a book, and the C API's state says `fixed`, so the app shows the zoom
in place of the text settings and turns the text size keys into zoom keys.

Anchors need a scheme that stays cheap however long the PDF is. The shim
makes each page a chapter of its own (`md_layout`), so an anchor is a page
and a count of characters on that page, and placing one never measures the
pages before it. A position is saved as the page, an offset of 0 or more
and a chapter fraction of 0. Search extracts the text a page at a time, as
it does chapters; a phrase cut by a page break is not found, and a
paragraph that runs over one is read aloud as two. A file that claims more
than 100000 pages is refused, since MuPDF takes the count from the file.

`reading/fixed.go` lays the pages out in the view. Each page keeps its size
in points, measured the first time it is shown. Two pages go side by side
when the spread setting allows it and, for the automatic spread, when each
would be at least 80 percent of the size of one alone; the second page of
the book stands for the rest, since the first is often a cover of its own
size. A `Zoom` fits whole pages (the default), fits their width, or draws
them at a scale, never smaller than whole pages and never larger than 8
times their size unless fitting them takes more. `StepZoom` goes through
those two fits and fixed scales from half size to 8 times. Pages larger
than the view are scrolled by a pan in view points, held within the pages;
a new zoom keeps the point at the center of the view in place, a page
turn shows the top of the new page, and going to an anchor scrolls it into
view a quarter of the way down. The `Viewport` in the state tells the app
the scale, both fits and how far the view can scroll.

Rendering draws only the part of a page that is in the view: the layers
start at the page's visible corner, and `md_render_layers` takes the
position of the page's top left corner, which is negative when the page is
scrolled. Hit tests, the boxes of a range and the pictures under the
pointer convert between view points and page points with each page's
frame (its corner and scale), and boxes scrolled out of the view are left
out, which is how reading aloud knows to scroll to the next word.

Text goes to the coverage mask as in any book, so it glows with EDR. The
vector paths of a fixed page are its drawings (rules, tables, diagrams,
fills), and they go to the picture layer instead, so the picture modes
apply to them. A fill that covers the page is its paper and is never
drawn. At night a drawing is turned around as line art is: its lightness
is inverted with its hue kept, so black lines glow and white or light
fills go dark. In original colors only its grays are inverted and its hues
keep their colors, and hidden pictures leave drawings out. A page whose
paper is dark, as a slide's can be, already suits a black page, so its
drawings keep their colors. A scanned page is one image, which the night
mode inverts like any picture of light paper; its text, if it has an
invisible OCR layer, can still be searched and selected.

## Collections and synced folders

Collections live in two tables, `collections` and `collection_books`, so a
book can be in any number of them; the book list carries the ids of the
collections that hold each book, and the app filters the grid with them. A
collection made from a folder remembers the folder's path, so a book added
to that folder later finds its collection even after the user renames it.

The library knows a book's original file by its path, its size and its
modification time. Importing a known path whose size or time changed
replaces the library's copy and the book's metadata but keeps its id, place,
annotations and collections. A file at an unknown path whose size and time
match a book whose original is gone is that book, renamed or moved. Paths
are stored with symbolic links resolved, the way FSEvents reports them, so
`/tmp/a.epub` and `/private/tmp/a.epub` are one file.

`importer.ImportFolder` walks a folder, skipping hidden files and the
library's own `Books` folder, imports each book in a format of
`bookfile` (by its extension) and, when asked, puts it in the collection of
its folder. A book protected by DRM is reported as such among the failures. A watched folder is stored in
`watched_folders`; `importer.Sync` imports what is new or changed in it and
marks the books whose file is gone (`books.missing`) instead of removing
them. Collections are only filled with books a sync touched, so a book the
user took out of a collection stays out. A folder that is not there, such as
an unplugged disk, is not synced at all.

In the app, `FolderWatcher` follows the watched folders with FSEvents and
waits for a second and a half of quiet before it reports a change, since a
copy writes a file many times. It ignores the library's own folder, so the
database's writes do not trigger syncs of a folder that holds it. Folder
imports and syncs run on the `Importer` queue, like file imports; every
watched folder is synced once at launch to catch changes made while Mole was
closed.

## Light: how the text glows

The reader view is backed by a `CAMetalLayer` in `rgba16Float` with
`wantsExtendedDynamicRangeContent` and the extended linear Display P3 color
space. In that format 1.0 is SDR white and larger values are brighter, up to
the screen's current headroom (`NSScreen.maximumExtendedDynamicRangeColorComponentValue`).

The core renders the visible pages in grayscale and hands over the ink
coverage of every pixel. A fragment shader writes

```
color = tint * gain * coverage ^ gamma
```

where `gain = level * headroom` (capped at 1 when EDR is off) and `gamma`
sets the stroke weight: a larger exponent thins the antialiased edges.
Background pixels have no coverage and stay at zero, so the screen is dark
everywhere but under the letters.

Facts that shaped this design, found while prototyping:

- **The light must come from one EDR layer.** An SDR view (a web view, for
  example) with an EDR overlay in the same window is composited in SDR; the
  overlay has no effect. Rendering text natively into the EDR layer avoids
  the problem, which is why the page is rasterized by MuPDF rather than shown
  in a WKWebView.
- **Headroom is granted, not owned.** macOS gives EDR headroom only to the
  frontmost app and raises it over three to four seconds once EDR content is
  on screen. `EDRSentinel`, a transparent one-point EDR layer in the window,
  keeps the headroom raised in the library, and `ReaderCoordinator` holds the
  page black until the headroom settles (`WarmUp`), so text never fades in.
- **macOS posts no headroom notification**, so `AppModel` samples it ten
  times a second.

## Colors and pictures

The core splits each page among three layers (`md_render_layers` in
`core/internal/mupdf/shim.c`). Text always goes to the coverage mask in full
ink, whatever its color, so that gray captions and white text on a dark box
still glow. When the reader keeps the book's colors, text and lines drawn in
a hue also go to an RGBA layer with their coverage as alpha. Black, white and
grays are left out and keep the tint. A dark hue, chosen to stand out on
white paper, has its lightness mirrored so that it stands out on black, and
a hue still dimmer than a set luminance (pure blue, for example) is mixed
with white. The shader takes the hue in place of the tint in proportion to
that alpha, so a colored heading keeps the stroke weight and the EDR gain of
the rest of the text.

Pictures go to the third layer, RGBA on black with premultiplied alpha, in
one of three styles that `mole_session_set_style` picks:

- Night draws photographs as they are and inverts line art, so that a
  diagram's lines glow instead of its paper. An image counts as line art
  when, composited on white, it is light on average and at least a third of
  it is bare paper; a bright photograph has little pure white. The inversion
  keeps hue and chroma (each channel moves by alpha minus the largest minus
  the smallest), so a red box stays red.
- Original colors draws every image on a white rectangle, as on a printed
  page, so a transparent drawing does not vanish into the black.
- Hidden draws no picture.

MuPDF draws an image that scales, an SVG drawing, at its own size at 96 dpi
and scales the pixels, which blurs it on a Retina screen. The core draws
such an image itself at its size in pixels on the page and keeps that copy
until the size changes, so a diagram in SVG is as sharp as the text.

The shader converts both color layers from sRGB to linear Display P3 and
draws pictures under the text at `min(gain, 1)`, so they follow the
brightness level but never go past SDR white. MuPDF lets inline styles win
over the reader's style sheet, which is why colors are decided while
rendering rather than in CSS.

Clicking a picture asks the core what is under the pointer
(`mole_session_image_at`, which lists the images of the visible pages once
per layout) and for its pixels as on white paper, at its own resolution
(`mole_session_image_pixels`). `PictureViewer` shows them in an SDR
`NSScrollView` over the page, which handles the pinch and the keys.

## Fonts and typography

The core sets a book's text through a user style sheet
(`reading.Typography`) that MuPDF applies over the book's own styles. It
always keeps code in a monospaced font; colors are not CSS (see Colors and
pictures). The reader's choices add a font family for the body, a line
height for paragraphs and list items, and left or justified alignment.
Margins are not CSS either: `reading.NewGeometry` turns them into the room
around the pages and the widest a page may grow.

MuPDF knows only the fonts it has built in, so `systemfonts.c` installs a
loader with `fz_install_load_system_font_funcs`. When a style names a
family MuPDF does not have, the loader asks CoreText for the face of that
family closest to the weight and style wanted, gets the file it lives in,
finds the face's index in a collection (`.ttc`) by its PostScript name, and
hands the file to MuPDF. The faces of one collection share one copy of the
file, and families that are not installed are remembered, since MuPDF asks
again for every element styled with them. macOS hides some of its own
families behind a leading dot (`.New York`); the loader tries that name
too. A book that names an installed font, Georgia or Garamond for example,
now gets it, where it used to fall back to MuPDF's serif.

MuPDF 1.26 builds FreeType without support for font variations, so a
variable font loads at its default instance and MuPDF makes up its bold.
For New York that instance is the display cut, which is why the reader does
not offer it. MuPDF does not hyphenate either: it breaks words only at the
soft hyphens a book has, and has no CSS `hyphens`, so justified text can
space words widely.

A new typography opens the book again, through `bookfile` so that every
format opens as it did first, before it is laid out. MuPDF keeps the
chapters it parsed with the old style sheet and its page counts, so giving
the open document a new style sheet would show stale styles. Anchors
do not depend on the typography, so the reader stays at the same text, and
the background copy that searches read stays valid.

Two things in MuPDF's text extraction needed care. It starts a new text
block, which the core reads as a paragraph, wherever two lines are more
than 1.5 em apart, which the relaxed spacing passes, so
`mupdf.Document.SetLineDistance` joins such blocks back when their lines
follow at the line height chosen; the paragraphs VoiceOver and reading
aloud go by stay whole. And it cuts a justified line into pieces at its
widest spaces, so hit tests pick the piece nearest the pointer and the
boxes of a range, which highlights, the spoken sentence and VoiceOver's
paragraph frames are drawn from, join the pieces of a line into one.

## Marks under the text

Highlights, the selection, search hits and the sentence being read aloud
are rectangles the core returns for a text range
(`mole_session_range_rects`), one per line, in view points. `ReaderTab`
gathers those on screen as `PageMark`s; `ReaderCoordinator`
turns them into rectangles of light in drawable pixels, and `EDRRenderer`
paints them first and the text over them, blended as
`text + mark * (1 - coverage)`. Marks are dim (a fifth of SDR white at most)
and dim further with the text, so a highlight reads as a tint behind the
letters instead of a block of light.

Selection, highlights, notes and bookmarks are all addressed by text
anchors (a chapter and a count of non-whitespace characters), never by page,
so they stay put when the book is laid out again.

## Accessibility

The page is drawn into a Metal layer, so AppKit knows nothing of its text.
`EDRMetalView` describes it to assistive apps itself (`PageAccessibility`):
it is a group labeled with the book and the page, and its children are
static text elements, one per paragraph on screen. The core gives the
paragraphs of a range with `mole_session_paragraphs`, each with its anchors
and text, split at page breaks so that each part lies on one page; its frame
is the union of the part's line boxes from `mole_session_range_rects`. The
elements are made only when an assistive app asks for them and are dropped
when the page changes, so the page costs nothing while VoiceOver is off.

A page turn posts a layout change and an announcement of the new page and
chapter, except while the book is read aloud. Custom actions (Next Page,
Previous Page) and the scroll-by-page actions VoiceOver's scroll commands
send turn the page; AppKit has no constants for the latter, so the view
answers them through the older string-based actions.

The rest of the window is SwiftUI, labeled where the default reading falls
short: the sidebar, the tab strip, the side panel and the status bar are
labeled groups, rows give a title as the label and counts or progress as
the value, and context menu commands are also accessibility actions.
`Motion` leaves out the sidebar and panel animations when Reduce Motion is
on, and the cover no longer lifts under the pointer. `Theme.contrasting`
makes the hairlines and the secondary text tokens stronger while Increase
Contrast is on.

`make verify` walks the accessibility tree through the AX API, as VoiceOver
sees it, and writes it to `build/verify/ax-*.txt`. The AX API answers only a
trusted process; started from a terminal that has the Accessibility
permission, the app is.

## Reading aloud

`ReadAloud` speaks with `AVSpeechSynthesizer`, one utterance per sentence,
keeping a few in the synthesizer's queue. It asks the core for the
paragraphs after the place it reached, about 1,500 characters at a time,
joins the parts of a paragraph that runs over a page break, and splits them
into sentences with `NLTokenizer`. A sentence's characters are mapped back
to anchors by counting them the way the core does (whitespace, invisible
format characters and hyphens take no offset), so the sentence being spoken
becomes a mark under the text, and each word the synthesizer reaches turns
the page when it is not on screen. Reading goes on across chapters until
the end of the book.

The voice speaks the language the book declares, which the core reads from
the EPUB's `dc:language` (`mole_session_language`; MuPDF keeps no EPUB
language of its own, and other documents fall back to MuPDF's metadata).
Without a declaration, `NLLanguageRecognizer` decides only on a long sample
it is sure of; a page of names, such as a copyright page, is not enough, and
the user's preferred languages are used instead. The voice is the one picked
in Settings if it speaks that language, or else the best installed voice for
it, from the region the language names, the user's region or the usual one.
Reading stops when the book closes or another tab is shown.

## Positions that survive relayout

MuPDF reflows an EPUB whenever the font size, the page size or the
typography changes, so page numbers are not stable. (A PDF's pages are
stable, and its anchors are pages; see Fixed pages.) MuPDF's own bookmarks
for EPUB are not stable across runs either. The core therefore stores a position as a chapter and a
fraction of that chapter (`library.Position`) and maps it back to a page
after every layout.

## The window

The window has a hidden, transparent title bar and the tab strip is drawn in
its place. The tab strip is taller than a standard title bar, so
`TrafficLights` moves the window buttons to its vertical center and moves
them back whenever AppKit lays them out again. A toolbar would center them
without help, but its view sits above the content and takes the clicks meant
for the tabs.

The sidebar and the contents panel are plain columns rather than
`NavigationSplitView` and `.inspector`: both of those install a toolbar
area, which pushed the tab strip down.

## The side panel

Next to an open book, `SidePanel` has four pages: contents, bookmarks, notes
and search. Search runs in the core on its own MuPDF context, in the
background; `ReaderTab` polls its results ten times a second while it runs,
so hits appear as they are found.

## Menus

`MoleCommands` builds the menu bar. SwiftUI does not rebuild menus when an
`@Observable` model changes, but it does when a focused value changes, so the
window publishes a `MenuState` with `focusedSceneValue` and the menus read
that.

## Updates

`Updater` wraps Sparkle's `SPUStandardUpdaterController`. It starts only
when Info.plist names an HTTPS feed (`SUFeedURL`) and holds the EdDSA public
key that verifies updates (`SUPublicEDKey`). The Makefile writes both through
`scripts/bundle.sh`, from `SPARKLE_FEED_URL` and `SPARKLE_PUBLIC_KEY`; only
releases set the key, so development and CI builds never start the updater,
and the app menu's Check for Updates item and the Updates section of
Settings are hidden. The self-checks (`--smoke-test`, `--verify-dir`) never
start it either, so they stay off the network and out of the user defaults.
`SUVerifyUpdateBeforeExtraction` makes Sparkle check the signature before it
unpacks an update.

Sparkle comes from SwiftPM as a binary framework. `bundle.sh` copies it into
`Contents/Frameworks`, where the executable's rpath points, without its XPC
services, which only sandboxed apps use. `scripts/release.sh` signs its
nested code inside out, then the app, all with the hardened runtime. Library
validation then loads Sparkle because both carry the same team ID; an ad hoc
dry run has no team ID, so only that build turns library validation off.

Each release publishes `appcast.xml` next to the zip, and the app reads the
one of the latest GitHub release. `release.sh` writes the appcast from the
app's Info.plist (version, build number, minimum macOS), signs the zip with
Sparkle's `sign_update`, and checks the signature against the app's public
key with CryptoKit (`scripts/ed25519-verify.swift`) before anything reaches
`build/release`.

## Localization

`L10n` reads `Localizable.strings` from the SwiftPM resource bundle, which
`scripts/bundle.sh` copies into `Mole.app/Contents/Resources`. The main
bundle lists the same languages in `CFBundleLocalizations` and has an
`.lproj` folder for each; without them AppKit would show its own menus in
English whatever the system language.

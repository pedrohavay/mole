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
- On failure a function returns 0 or NULL and `mole_last_error` explains why.
- Library calls (`mole_library_*`, import included) may come from any
  thread, one at a time per handle; a session is used from one thread at a
  time. The last error belongs to the calling thread. The app uses its main
  library handle on the main thread and opens a second one for importing on
  a background queue (`Importer`).

`app/Sources/Mole/Bridge` wraps these conventions: `CoreCall` holds the error
and memory rules, `Library` and `Session` are the handle classes, and a
`Session` closes (saving the position) when it is released.

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

## Marks under the text

Highlights, the selection and search hits are rectangles the core returns
for a text range (`mole_session_range_rects`), one per line, in view points.
`ReaderTab` gathers those on screen as `PageMark`s; `ReaderCoordinator`
turns them into rectangles of light in drawable pixels, and `EDRRenderer`
paints them first and the text over them, blended as
`text + mark * (1 - coverage)`. Marks are dim (a fifth of SDR white at most)
and dim further with the text, so a highlight reads as a tint behind the
letters instead of a block of light.

Selection, highlights, notes and bookmarks are all addressed by text
anchors (a chapter and a count of non-whitespace characters), never by page,
so they stay put when the book is laid out again.

## Positions that survive relayout

MuPDF reflows an EPUB whenever the font size or the page size changes, so
page numbers are not stable. MuPDF's own bookmarks for EPUB are not stable
across runs either. The core therefore stores a position as a chapter and a
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

## Localization

`L10n` reads `Localizable.strings` from the SwiftPM resource bundle, which
`scripts/bundle.sh` copies into `Mole.app/Contents/Resources`. The main
bundle lists the same languages in `CFBundleLocalizations` and has an
`.lproj` folder for each; without them AppKit would show its own menus in
English whatever the system language.

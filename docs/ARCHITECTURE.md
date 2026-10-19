# Architecture

Mole is two programs linked into one binary. The Go core owns the data and
the book: the SQLite library, EPUB import, layout with MuPDF and the
rasterization of pages into a coverage mask. The Swift app owns everything
the user sees: the window, the library grid, the tabs and the Metal view that
turns the mask into light. They meet at a small C API.

```
SwiftUI views ── AppModel ── Bridge (Library, Session) ── C API ── Go core ── MuPDF
                                                                     └─────── SQLite
EDRRenderer (Metal) ◀── coverage mask (one byte per pixel) ◀──────────┘
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
  released with `mole_free`. The one exception is the coverage mask, which the
  session owns and reuses until its next render, layout or close.
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

## Fonts and the text style

The core sets a book's text through a user style sheet (`reading.Style`)
that MuPDF applies over the book's own styles. It always keeps the text
black, so it can be turned into light, and code in a monospaced font. The
reader's choices add a font family for the body, a line height for
paragraphs and list items, and left or justified alignment. Margins are not
CSS: `reading.NewGeometry` turns them into the room around the pages and
the widest a page may grow.

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

A new style opens the book again before it is laid out. MuPDF keeps the
chapters it parsed with the old style sheet and its page counts, so giving
the open document a new style sheet would show stale styles. Anchors do not
depend on the style, so the reader stays at the same text, and the
background copy that searches read stays valid.

Two things in MuPDF's text extraction needed care. It starts a new text
block, which the core reads as a paragraph, wherever two lines are more
than 1.5 em apart, which the relaxed spacing passes, so
`mupdf.Document.SetLineDistance` joins such blocks back when their lines
follow at the style's line height. And it cuts a justified line into pieces
at its widest spaces, so hit tests pick the piece nearest the pointer and
the boxes of a range join the pieces of a line into one.

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

MuPDF reflows an EPUB whenever the font size, the page size or the text
style changes, so page numbers are not stable. MuPDF's own bookmarks for EPUB are not stable
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

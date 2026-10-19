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
library's own `Books` folder, imports each EPUB and, when asked, puts it in
the collection of its folder. A watched folder is stored in
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

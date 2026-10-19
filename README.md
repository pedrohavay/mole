<p align="center">
  <img src="assets/AppIcon.appiconset/icon_256x256.png" width="128" height="128" alt="Mole icon">
</p>

<h1 align="center">Mole</h1>

<p align="center">An EPUB reader for the Mac where only the letters give off light.</p>

<p align="center">
  <a href="LICENSE"><img alt="License: AGPL-3.0-or-later" src="https://img.shields.io/badge/license-AGPL--3.0--or--later-blue"></a>
  <img alt="macOS 14 or later" src="https://img.shields.io/badge/macOS-14%2B-lightgrey">
  <img alt="Apple silicon" src="https://img.shields.io/badge/Apple%20silicon-arm64-lightgrey">
</p>

Mole keeps the screen black and lights only the text. It uses the Extended
Dynamic Range (EDR) of Apple displays to draw glyphs brighter than SDR white
while every other pixel stays at zero, so on a mini-LED or OLED screen each
letter glows on its own and the rest of the panel stays off. The name comes
from the animal, which lives in the dark and needs very little light to find
its way.

Mole is in early development and has no release builds yet.

## Features

- Text light that follows the screen's EDR headroom, from a faint glow to
  many times SDR white, in white, warm, amber or red, with three stroke
  weights.
- Books open at their final brightness. A one-point EDR layer keeps the
  headroom raised while Mole is open, so the text does not fade in.
- EPUB layout and rendering by MuPDF, one or two pages side by side.
  Diagrams are inverted so their lines glow; photos and covers keep their
  tones, and pictures stay dim by default.
- The book's own fonts or one of the Mac's: Iowan Old Style, Charter,
  Georgia, Palatino, Baskerville, Avenir Next or Helvetica Neue. Code keeps
  a monospaced font. Lines can be compact, normal or relaxed, margins
  narrow, normal or wide, and paragraphs aligned left or justified.
- A library with covers, shelves for books in progress, not started and
  finished, search by title or author, drag and drop, and tabs that reopen
  on the next launch.
- Full-text search that runs in the background, highlights in five colors,
  notes, bookmarks, and a way to copy all of a book's highlights as
  Markdown.
- Reading positions tied to the text, so the page you were on survives a
  new font, font size, spacing, margin or window size.
- A keyboard shortcut for every reading control, and arrow-key navigation in
  the library.
- English and Brazilian Portuguese, picked from the system language.

## Installation

Build Mole from source. With Xcode 15.4 or later and Go 1.26 or later
installed:

```bash
git clone https://github.com/pedrohavay/mole.git
```

```bash
cd mole && make run
```

`make run` builds `build/Mole.app` and opens it; copy it to `/Applications`
to keep it. The first build also downloads MuPDF (about 65 MB) and compiles
it, which takes a minute or two. The build is signed ad hoc, which is enough on the Mac that
built it. Gatekeeper refuses a copy downloaded from elsewhere until Mole has
notarized releases.

## Usage

Add books with File > Add Books (⌘O), by dropping EPUB files on the library,
or with Open With > Mole in Finder. Mole copies each book into its library,
so moving or deleting the original file does not break anything. Double-click
a cover to open the book.

Click the right side of the page, press →, or swipe left to turn the page;
click the left third or press ← to go back. The sun button in the status bar
sets the brightness, the EDR switch and the text color. The Aa button sets
the text size, the font, the line spacing and alignment, the margins, the
stroke weight, one or two pages, and how pictures are shown. Settings >
Reading has the same options.

Drag across the text to select it. The bar above the selection highlights it
in a color, adds a note, copies it, or searches the book for it. The side
panel (⌥⌘C) holds the table of contents, the bookmarks, the notes and the
search.

### Keyboard shortcuts

While reading:

| Keys | Action |
|------|--------|
| → or Space or Page Down | Next page |
| ← or ⇧Space or Page Up | Previous page |
| Home, End | Beginning or end of the book |
| ↑, ↓ | Brighter or dimmer text |
| +, − | Bigger or smaller text |
| E | Turn EDR on or off |
| C | Next text color |
| T or ⌥⌘C | Show or hide the side panel |
| F or ⌃⌘F | Full screen |
| Esc | Clear the selection, or leave full screen |

Anywhere in the window:

| Keys | Action |
|------|--------|
| ⌘O | Add books |
| ⌘F, ⌘G, ⇧⌘G | Find in the book, next and previous result |
| ⌘D | Bookmark the page |
| ⇧⌘H | Highlight the selection |
| ⌘C | Copy the selection |
| ⌘W | Close the book |
| ⌘1, ⌘2 to ⌘9 | Library, then the open books in tab order |
| ⌃⇥, ⌃⇧⇥ | Next or previous tab |
| ⌃⌘S | Show or hide the sidebar |
| ⌥⌘↑, ⌥⌘↓ | Brighter or dimmer text |
| ⌘, | Settings |

Help > Keyboard Shortcuts shows the same list in the app.

## Building from source

Mole runs on macOS 14 Sonoma or later on Apple silicon. EDR needs a display
that supports it, such as the Liquid Retina XDR of the MacBook Pro or the Pro
Display XDR; on other screens the text stops at SDR white. To build it you
need:

- Xcode 15.4 or later. The Command Line Tools alone lack XCTest.
- Go 1.26 or later.
- librsvg (`brew install librsvg`), only to render the icon again after
  editing `assets/Mole.svg`.

`make` builds MuPDF 1.26.1 from its source release into `build/mupdf` the
first time, then the Go core as a static library with MuPDF inside it, then
the SwiftUI app, and assembles `build/Mole.app`. The other targets:

| Target | What it does |
|--------|--------------|
| `make run` | Builds and opens the app. |
| `make test` | Runs the Go and Swift tests. Set `MOLE_TEST_EPUB` to an EPUB file to add the tests that open a real book. |
| `make verify` | Runs the app's self-check, which captures the library, a book and Settings into `build/verify`. Set `BOOK` to add an EPUB first. |
| `make lint` | Runs `gofmt` and `go vet` on the core. |
| `make release` | Builds a signed and notarized `build/release/Mole-<version>.zip`; see below. |
| `make notices` | Regenerates `THIRD_PARTY_NOTICES` after a dependency changes. |
| `make clean` | Removes the build products and keeps the MuPDF build. |
| `make distclean` | Removes everything, MuPDF included. |

If `go version` is older than 1.26 and `GOTOOLCHAIN` is `local`, point `GO`
at a newer toolchain, for example `make GO=$HOME/go/bin/go1.26.0`. The
Makefile uses the Xcode in `/Applications/Xcode.app`; set `DEVELOPER_DIR` to
use another one. Run `make core` once before opening `app/Package.swift` in
Xcode, because the package needs the header the core build generates.

To build against another MuPDF, such as Homebrew's, set `MUPDF_PREFIX` to
the folder that holds its `include` and `lib`, for example
`make MUPDF_PREFIX=$(brew --prefix mupdf-tools)`. The Makefile passes the
prefix to cgo through `CGO_CFLAGS` and `CGO_LDFLAGS`, so Go commands run
outside make need those two variables set the same way.

The library lives in `~/Library/Application Support/Mole/library.db`, with
the copies of the books in the `Books` folder next to it. Set `MOLE_LIBRARY`
to use another database file.

## Making a release

A release needs a "Developer ID Application" certificate in the keychain
and a notarytool profile with the credentials of the same Apple Developer
team. Store the profile once:

```bash
xcrun notarytool store-credentials mole --apple-id you@example.com --team-id TEAMID
```

Then build, sign, notarize and pack the release:

```bash
make release SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" NOTARY_PROFILE=mole VERSION=0.1.0
```

The release is built against the pinned MuPDF, so it targets macOS 14 no
matter which macOS builds it. The script signs the app with the hardened
runtime, starts the signed app once to check it, waits for notarization,
staples the ticket, and writes `build/release/Mole-0.1.0.zip` with its
SHA-256. Publish both on a GitHub release made from the tagged commit, so
the release links to its exact source as the AGPL asks. A dry run without an
Apple account is `make release SIGN_IDENTITY=- NOTARIZE=no`, which is what
CI does on every push.

## How it works

Mole is two programs linked into one binary. The Go core owns the books and
the data: it lays out EPUBs with MuPDF, renders the visible pages into a
one-byte coverage mask and a separate image layer, and keeps the library,
positions and annotations in SQLite. The SwiftUI app draws the window and
hands the masks to a Metal shader, which writes `tint * gain * coverage^gamma`
into an extended-range `rgba16Float` layer. The gain is the brightness level
times the screen's current EDR headroom, so a fully covered pixel can go
well past 1.0, the value of SDR white.

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) covers the C API between the two
halves, how the light is produced, and the findings behind those choices.

## Project layout

```
core/                 Go: the library, layout and rendering
  cmd/molecore/       the C API the app links against (c-archive)
  internal/epub/      EPUB metadata and covers
  internal/importer/  adding books to the library
  internal/library/   the SQLite library: books, settings, annotations
  internal/mupdf/     cgo binding to MuPDF
  internal/reading/   page geometry, text anchors, search and rendering
  internal/session/   an open book and how its position is saved
  internal/testbook/  EPUB fixtures for the tests
app/                  Swift: the macOS app (SwiftPM)
  Sources/MoleCore/   module map for the core's generated header
  Sources/Mole/
    App/              entry point, menus, launch arguments, self-check
    Bridge/           Swift wrappers of the C API
    Model/            app state and domain types
    DesignSystem/     colors, type, spacing and shared controls
    Features/         Shell, Sidebar, Library, Reader, Contents, Settings
    Resources/        string tables (en, pt-BR)
    Support/          localization, formatting, window chrome
  Tests/MoleTests/
assets/               the app icon (Mole.svg is its source) and the About credits
scripts/              building MuPDF, bundling, releases, notices and the icon
docs/                 architecture notes
.github/workflows/    CI: lint, tests and an unsigned release build
```

## Contributing

Issues and pull requests are welcome. Run `make lint test` before sending a
change. Code comments are in English, and Go code follows `gofmt` and the
usual godoc conventions.

User-facing text lives in
`app/Sources/Mole/Resources/<language>.lproj/Localizable.strings` and is
looked up with `L10n.text("key")`. To add a language, copy the English
table, translate the values, and add the language to `LANGUAGES` in
`scripts/bundle.sh`. The tests fail when a table misses a key, when a
translation changes a format specifier, or when the code uses a key that no
table has.

## License

Copyright (C) 2026 Pedro Havay

Mole is free software: you can redistribute it and/or modify it under the
terms of the GNU Affero General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option) any
later version. See [LICENSE](LICENSE) for the full text.

Mole links [MuPDF](https://mupdf.com), which Artifex distributes under the
GNU AGPL version 3 or a commercial license, along with the libraries and
fonts MuPDF builds in and a few Go modules. Their licenses are in
[THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES), which the app also opens from
Help > Third-Party Notices.

## Acknowledgements

[MuPDF](https://mupdf.com) lays out and renders the books, and
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite) gives the core SQLite
without a C dependency of its own. This software is based in part on the
work of the Independent JPEG Group.

<p align="center">
  <img src="assets/AppIcon.appiconset/icon_256x256.png" width="128" height="128" alt="Mole">
</p>

<h1 align="center">Mole</h1>

<p align="center">An EPUB reader for the Mac where only the letters give off light.</p>

Mole keeps the screen black and lights the text alone, using the Extended
Dynamic Range (EDR) of Apple displays: glyphs are drawn brighter than SDR
white while everything around them stays at zero. On a mini-LED or OLED
screen the result reads like a lamp behind each letter, much closer to an
e-ink night light than to a dark theme. The name comes from the animal: a
mole lives in the dark and needs very little light to find its way.

## Features

- **Dynamic light.** Text brightness follows the screen's EDR headroom, with
  a level from a faint glow to many times SDR white, four text colors (white,
  warm, amber and red) and three stroke weights.
- **Instant brightness.** A one-point EDR layer keeps the headroom raised
  while Mole is up, so a book opens at its final brightness instead of
  fading in.
- **Faithful EPUB layout.** Books are laid out and rasterized by MuPDF, with
  one or two pages side by side.
- **A real library.** Covers, shelves (reading, not started, finished),
  search, recent books, drag and drop, and tabs for several open books that
  come back on the next launch. Reading positions survive font and window
  changes.
- **Search, highlights, notes and bookmarks.** Search the whole book in the
  background, select text to highlight it in five colors or attach a note,
  bookmark pages, and copy every highlight of a book as Markdown. Highlights
  are painted under the letters, dim, so they never outshine the text.
- **Keyboard first.** Every reading control has a key, the library grid
  moves with the arrow keys, and *Help > Keyboard Shortcuts* lists them all.
- **English and Brazilian Portuguese**, picked from the system language.

## Requirements

- macOS 14 Sonoma or later on Apple silicon. EDR needs a display that
  supports it, such as the Liquid Retina XDR of the MacBook Pro or the Pro
  Display XDR; elsewhere the text stops at SDR white.
- To build: Xcode 15.4 or later, Go 1.26 or later and MuPDF from Homebrew.

```bash
brew install mupdf-tools
```

## Building

```bash
make
```

This builds the Go core as a static library, the SwiftUI app against it, and
assembles `build/Mole.app`. Other targets:

| Target        | What it does                                                        |
|---------------|---------------------------------------------------------------------|
| `make run`    | Builds and opens the app.                                           |
| `make test`   | Runs the Go and Swift tests. Set `MOLE_TEST_EPUB` to an EPUB file to include the tests that open a real book. |
| `make verify` | Runs the app's self-check, which captures the library, a book and Settings into `build/verify`. Set `BOOK` to add an EPUB first. |
| `make lint`   | Runs `gofmt` and `go vet` on the core.                              |
| `make clean`  | Removes every build product.                                        |

Things that trip up a first build:

- **Go version.** The core needs Go 1.26. If `go version` is older and
  `GOTOOLCHAIN` is `local`, point `GO` at a newer toolchain, for example
  `make GO=$HOME/go/bin/go1.26.0`.
- **Xcode.** The Makefile builds with the Xcode at
  `/Applications/Xcode.app`; set `DEVELOPER_DIR` to use another one. The
  Command Line Tools alone lack XCTest and are not enough.
- **Opening the package in Xcode.** `app/Package.swift` needs the core's
  generated header, so run `make core` once before opening it.
- **MuPDF location.** Homebrew on Apple silicon installs to `/opt/homebrew`;
  set `MUPDF_LIB_DIR` for a MuPDF elsewhere.

The library lives in `~/Library/Application Support/Mole/library.db`, and
Mole keeps a copy of every book it adds in the `Books` folder next to it, so
moving or deleting the original does not break the library. Set
`MOLE_LIBRARY` to use another database file.

## Repository layout

```
core/                 Go: everything that is not user interface
  cmd/molecore/       the C API the app links against (c-archive)
  internal/epub/      EPUB metadata and covers
  internal/importer/  adding books to the library
  internal/library/   the SQLite library: books, positions, settings
  internal/mupdf/     cgo binding to MuPDF
  internal/reading/   page geometry, layout and coverage rendering
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
assets/               app icon
scripts/              bundling
docs/                 architecture notes
```

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains how the halves fit,
how the EDR light is produced and the choices behind them.

## Localization

User-facing text lives in `app/Sources/Mole/Resources/<language>.lproj/Localizable.strings`
and is looked up with `L10n.text("key")`. To add a language, copy the English
table, translate the values, and add the language to `LANGUAGES` in
`scripts/bundle.sh`. The tests fail when a table misses a key, has a
mismatched format specifier, or when the code uses a key no table has.

## License

Mole links MuPDF, which is licensed under the GNU AGPL v3 (or a commercial
license from Artifex). A distributed build of Mole must therefore be offered
under the AGPL, or Artifex's commercial license is needed. The license of
Mole's own code has not been chosen yet.

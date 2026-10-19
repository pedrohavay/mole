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
- A library with covers, shelves for books in progress, not started and
  finished, search by title or author, drag and drop, and tabs that reopen
  on the next launch.
- Collections, made by hand or from the folders books come in, and folders
  kept in sync: new and changed books are added as they appear, and a book
  whose file leaves the folder is marked rather than deleted.
- Full-text search that runs in the background, highlights in five colors,
  notes, bookmarks, and a way to copy all of a book's highlights as
  Markdown.
- Reading positions tied to the text, so the page you were on survives a
  new font size or window size.
- The system dictionary for any word (force click it, or select it and
  press ⌃⌘D) and Apple's translation of a selection on macOS 14.4 and later.
- An estimate of the reading time left in the chapter and in the book,
  learned from your own pace as you turn pages.
- Reading aloud with the system voices, from the top of the screen or the
  selection, with the sentence being read lit under the text and the pages
  turning on their own.
- VoiceOver support: the page is read a paragraph at a time, each new page
  and chapter is announced, and VoiceOver's actions and scroll commands turn
  the pages. Mole also follows Reduce Motion and Increase Contrast.
- A keyboard shortcut for every reading control, and arrow-key navigation in
  the library.
- English and Brazilian Portuguese, picked from the system language.
- Updates through Sparkle in the builds published on GitHub, checked once a
  day if you allow it.

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
so moving or deleting the original file does not break anything. A file
added again after it changed, or after it was renamed, updates its book
instead of adding a second one. Double-click a cover to open the book.

Choose or drop a folder to add every EPUB in it and in its subfolders. Mole
then asks whether each folder that holds books should become a collection,
and whether to keep the folder in sync. A synced folder is watched while
Mole runs and checked again at launch; Settings > General lists the synced
folders, with Sync Now and a button to stop syncing one. When a file leaves
a synced folder, its book stays in the library with a "Not in folder" mark
until you remove it.

Collections are listed in the sidebar under the shelves. Make one with the +
button or File > New Collection (⌘N), double-click it to rename it, and drag
covers onto it, or use Add to Collection in a book's menu. A book can be in
several collections.

Click the right side of the page, press →, or swipe left to turn the page;
click the left third or press ← to go back. The sun button in the status bar
sets the brightness, the EDR switch and the text color. The Aa button sets
the text size, the stroke weight, one or two pages, and how pictures are
shown.

Drag across the text to select it. The bar above the selection highlights it
in a color, adds a note, copies it, or searches the book for it. The side
panel (⌥⌘C) holds the table of contents, the bookmarks, the notes and the
search.

The speaker button in the status bar, or Book > Read Aloud (⇧⌘R), reads the
book aloud from the top of the screen, or from the selection if there is
one, and goes on into the next chapters. The same button and shortcut pause
and resume; Esc or the stop button ends it. Settings > Reading picks the
voice and the speed. The automatic voice speaks the language of the book;
more voices can be added in System Settings > Accessibility > Spoken
Content.

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
| Esc | Clear the selection, stop reading aloud, or leave full screen |
| ⌃⌘D or force click | Look up the selection or the word in the dictionary |

Anywhere in the window:

| Keys | Action |
|------|--------|
| ⌘O | Add books |
| ⌘N | New collection |
| ⌘F, ⌘G, ⇧⌘G | Find in the book, next and previous result |
| ⌘D | Bookmark the page |
| ⇧⌘H | Highlight the selection |
| ⌘C | Copy the selection |
| ⌘W | Close the book |
| ⌘1, ⌘2 to ⌘9 | Library, then the open books in tab order |
| ⌃⇥, ⌃⇧⇥ | Next or previous tab |
| ⌃⌘S | Show or hide the sidebar |
| ⌥⌘↑, ⌥⌘↓ | Brighter or dimmer text |
| ⇧⌘R | Read aloud, pause or resume |
| ⌘. | Stop reading aloud |
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
| `make verify` | Runs the app's self-check, which captures the library, a book and Settings into `build/verify`, reads a page aloud and writes the accessibility tree there. Set `BOOK` to add an EPUB first. |
| `make lint` | Runs `gofmt` and `go vet` on the core. |
| `make release` | Builds a signed and notarized `build/release/Mole-<version>.zip` and, with the updater set up, its `appcast.xml`; see below. |
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

A version tag starts the release workflow
(`.github/workflows/release.yml`). It checks that the tag matches `VERSION`
in the Makefile, builds Mole against the pinned MuPDF, signs the app and the
Sparkle framework inside it with a Developer ID and the hardened runtime,
starts the signed app once to check it, notarizes it, staples the ticket,
signs the zip for Sparkle, and publishes a GitHub release with
`Mole-<version>.zip`, its SHA-256 and `appcast.xml`. The release notes link
the tagged source code, as the AGPL asks. Installed copies read the appcast
of the latest release
(`https://github.com/pedrohavay/mole/releases/latest/download/appcast.xml`)
to find new versions.

### Setting up the keys

This is done once, and the first three steps need an Apple Developer Program
membership.

1. Create the "Developer ID Application" certificate. In Xcode, open
   Settings > Accounts, select the team, click Manage Certificates, then +
   and Developer ID Application. Only the account holder can create it.

2. Export the certificate with its private key. In Keychain Access, under
   the login keychain and My Certificates, select "Developer ID Application:
   Your Name (TEAMID)", check that it has a private key under it, choose
   File > Export Items, save it as a Personal Information Exchange (.p12)
   file, and set a password. Then encode it for GitHub:

   ```bash
   base64 -i DeveloperID.p12 -o DeveloperID.p12.base64
   ```

3. Create an App Store Connect API key for notarytool. In App Store Connect,
   open Users and Access > Integrations > App Store Connect API (the account
   holder requests access the first time), and under Team Keys generate a
   key with the Developer role. Download `AuthKey_<key ID>.p8`, which App
   Store Connect offers only once, and note the key ID and the issuer ID
   shown above the list of keys.

4. Create the update keys. `make app` downloads Sparkle, whose tools are in
   `app/.build/artifacts/sparkle/Sparkle/bin`. `generate_keys` makes an
   EdDSA key pair, keeps the private key in the login keychain and prints
   the public key; `generate_keys -p` prints it again later. Export the
   private key to a file for GitHub:

   ```bash
   app/.build/artifacts/sparkle/Sparkle/bin/generate_keys
   ```

   ```bash
   app/.build/artifacts/sparkle/Sparkle/bin/generate_keys -x sparkle_private_key
   ```

   Keep the private key safe in the keychain and in a backup. Installed
   copies accept only updates signed with it, so losing it means users have
   to download the next version by hand.

5. Add the secrets and the variable to the repository, under Settings >
   Secrets and variables > Actions, or with `gh`:

   | Name | Kind | Value |
   |------|------|-------|
   | `DEVELOPER_ID_P12` | Secret | The contents of `DeveloperID.p12.base64` |
   | `DEVELOPER_ID_P12_PASSWORD` | Secret | The password of the .p12 file |
   | `NOTARY_KEY_P8` | Secret | The contents of `AuthKey_<key ID>.p8` |
   | `NOTARY_KEY_ID` | Secret | The key ID |
   | `NOTARY_ISSUER` | Secret | The issuer ID |
   | `SPARKLE_PRIVATE_KEY` | Secret | The contents of `sparkle_private_key` |
   | `SPARKLE_PUBLIC_KEY` | Variable | The public key from `generate_keys -p` |

   ```bash
   gh secret set DEVELOPER_ID_P12 < DeveloperID.p12.base64
   ```

   ```bash
   gh variable set SPARKLE_PUBLIC_KEY --body "$(app/.build/artifacts/sparkle/Sparkle/bin/generate_keys -p)"
   ```

   Then delete the exported files: `DeveloperID.p12`, its base64 copy,
   `sparkle_private_key`, and the .p8 file once it is stored somewhere safe.

### Cutting a release

Set `VERSION` in the Makefile to the new version, commit it on main, then
tag that commit and push the tag:

```bash
git tag -a v0.2.0 -m "Mole 0.2.0"
```

```bash
git push origin v0.2.0
```

The workflow refuses a tag that does not match `VERSION`. Sparkle compares
build numbers, and the build number is the count of commits, so each release
must come from a later commit on main than the one before.

### Releasing from a Mac

`make release` does the same on a Mac with the certificate in the keychain,
without publishing anything. For notarization it takes either a notarytool
keychain profile in `NOTARY_PROFILE`, or the API key itself in `NOTARY_KEY`
(the path of the .p8 file), `NOTARY_KEY_ID` and `NOTARY_ISSUER`. To store a
profile once and release with it:

```bash
xcrun notarytool store-credentials mole --key AuthKey_KEYID.p8 --key-id KEYID --issuer ISSUER_ID
```

```bash
make release SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" NOTARY_PROFILE=mole SPARKLE_PUBLIC_KEY=PUBLIC_KEY SPARKLE_KEY_FILE=sparkle_private_key
```

`SPARKLE_KEY_FILE` is a private key exported with `generate_keys -x`; the
`SPARKLE_PRIVATE_KEY` environment variable can hold the key instead. Leave
out both Sparkle keys to build a release whose updater is off. The script
checks the credentials before it builds anything, and checks that the zip's
signature matches the public key inside the app. Upload the zip, its
`.sha256` and `appcast.xml` to a GitHub release of the tag `v<version>`.

A dry run without an Apple account is
`make release SIGN_IDENTITY=- NOTARIZE=no`, which is what CI does on every
push. An ad hoc signature has no team, so the hardened runtime would refuse
to load Sparkle; only in this case the app gets the
`com.apple.security.cs.disable-library-validation` entitlement.

Development and CI builds have no `SPARKLE_PUBLIC_KEY`, so their updater
stays off and the Check for Updates item is hidden. To try the updater, pass
a public key on the command line, for example
`make run SPARKLE_PUBLIC_KEY=PUBLIC_KEY`, and `SPARKLE_FEED_URL` to read
another appcast.

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
.github/workflows/    CI (lint, tests, an unsigned release build) and releases
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
fonts MuPDF builds in, a few Go modules, and Sparkle. Their licenses are in
[THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES), which the app also opens from
Help > Third-Party Notices.

## Acknowledgements

[MuPDF](https://mupdf.com) lays out and renders the books,
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite) gives the core SQLite
without a C dependency of its own, and [Sparkle](https://sparkle-project.org)
keeps the app up to date. This software is based in part on the
work of the Independent JPEG Group.

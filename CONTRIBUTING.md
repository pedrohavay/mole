# Contributing to Mole

Thanks for your interest in Mole. Bug reports, ideas and pull requests are
all welcome. For a large change, open an issue first so we can agree on the
approach before you write the code.

## Setting up

You need:

- A Mac with Apple silicon and macOS 14 Sonoma or later.
- Xcode 15.4 or later. The Command Line Tools alone lack XCTest, which the
  tests need.
- Go 1.26 or later.
- librsvg (`brew install librsvg`), but only if you edit the icon in
  `assets/Mole.svg`.

Then build and open the app:

```bash
make run
```

The first `make` downloads MuPDF 1.26.1 and builds it into `build/mupdf`.
After that it builds the Go core as a static library with MuPDF inside it,
then the SwiftUI app, and puts them together in `build/Mole.app`.

| Target | What it does |
|--------|--------------|
| `make run` | Builds and opens the app. |
| `make test` | Runs the Go and Swift tests. |
| `make lint` | Runs `gofmt` and `go vet` on the core. |
| `make verify` | Runs the app's self-check, which saves screenshots and a report in `build/verify`. |
| `make notices` | Rebuilds `THIRD_PARTY_NOTICES` after a dependency changes. |
| `make clean` | Removes the build products but keeps the MuPDF build. |
| `make distclean` | Removes everything, MuPDF included. |
| `make release` | Builds a signed release. See [docs/RELEASING.md](docs/RELEASING.md). |

To work on the app in Xcode, run `make core` once and then open
`app/Package.swift`. The package needs the header that the core build makes.

## Running the tests

Run this before you send a change:

```bash
make lint test
```

Some tests open a real book. Set `MOLE_TEST_EPUB` to the path of an EPUB to
run them too. `make verify` opens the app, captures the library, a book and
Settings, and reads a page aloud. Set `BOOK` to an EPUB to add it first.

> [!TIP]
> Set `MOLE_LIBRARY` to a scratch database, such as
> `MOLE_LIBRARY=/tmp/mole-dev/library.db`, so test runs never touch your own
> library in `~/Library/Application Support/Mole`.

## Things to know about the build

Mole only runs on macOS, and several parts of the build depend on that.

- The text is drawn with Metal and Extended Dynamic Range (EDR), which
  only exist on Apple platforms.
- The build makes a binary for the Mac it runs on. Only Apple silicon is
  tested; Intel Macs are not.
- You need an HDR screen to see the effect. On other screens the text stops
  at normal white, so you cannot check brightness changes. Screenshots are
  8-bit, so they clip anything brighter than white.
- MuPDF is pinned. `scripts/build-mupdf.sh` builds a fixed release for
  macOS 14, while a Homebrew build targets the macOS that built it and
  changes with every upgrade. To try another MuPDF anyway, set
  `MUPDF_PREFIX` to the folder that holds its `include` and `lib`, for
  example `make MUPDF_PREFIX=$(brew --prefix mupdf-tools)`.
- The Makefile passes the MuPDF folder to cgo through `CGO_CFLAGS` and
  `CGO_LDFLAGS`. Set them the same way to run `go test` by hand.
- If `go version` is older than 1.26 and `GOTOOLCHAIN` is `local`, point
  `GO` at a newer toolchain, for example `make GO=$HOME/go/bin/go1.26.0`.
- The Makefile uses `/Applications/Xcode.app`. Set `DEVELOPER_DIR` to use
  another Xcode.
- Development builds are signed ad hoc, so macOS sees each build as a new
  app. It may ask again for access to Downloads, Documents or Desktop. Keep
  test books somewhere else, such as `/tmp`.
- Do not strip the app's binary. Stripping breaks its code signature, and
  macOS then kills the app at launch.

> [!WARNING]
> `make distclean` also deletes the MuPDF build, and the next `make` has to
> download and compile it again.

## Code style

- Code comments are in English.
- Go code follows `gofmt`, `go vet` and the usual godoc conventions: a
  comment on an exported name starts with that name.
- Swift code follows the style of the files around it.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org),
  for example `fix(core): keep the page when the font changes`.
- Keep a pull request to one change, and add a screenshot when it changes
  the interface.

## Adding a language

Text that people see lives in
`app/Sources/Mole/Resources/<language>.lproj/Localizable.strings`, and the
code looks it up with `L10n.text("key")`. To add a language:

1. Copy the English table into a new `<language>.lproj` folder.
2. Translate the values, not the keys.
3. Add the language to `LANGUAGES` in `scripts/bundle.sh`.

The tests fail when a table is missing a key, when a translation changes a
format specifier such as `%@`, or when the code uses a key that no table has.

## License

Mole is licensed under the GNU Affero General Public License, version 3 or
later. Contributions are accepted under the same license.

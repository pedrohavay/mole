# Mole builds as two halves: the Go core (core/) as a static C library, and
# the SwiftUI app (app/) linked against it. `make` produces build/Mole.app.
# Run `make help` for the other targets.

GO ?= go
SWIFT ?= swift
CONFIG ?= release
VERSION ?= 0.1.0
# BUILD_NUMBER only grows: the commit count, or 0 outside a repository.
BUILD_NUMBER ?= $(shell git rev-list --count HEAD 2>/dev/null || echo 0)
BUNDLE_ID ?= io.github.pedrohavay.mole
# BOOK is an EPUB that `make verify` adds to the library before capturing.
BOOK ?=

# SwiftPM needs the full Xcode toolchain (XCTest, the macOS SDK); an SDKROOT
# left over from another toolchain would point it elsewhere.
DEVELOPER_DIR ?= /Applications/Xcode.app/Contents/Developer
export DEVELOPER_DIR
unexport SDKROOT

BUILD := build
APP := $(BUILD)/Mole.app
RELEASE := $(BUILD)/release
CORE_LIB := core/build/libmolecore.a
CORE_HEADER := app/Sources/MoleCore/libmolecore.h
CORE_SOURCES := $(shell find core/cmd core/internal -type f \( -name '*.go' -o -name '*.c' -o -name '*.h' \) ! -name '*_test.go')

# MUPDF_PREFIX holds MuPDF's headers and static libraries. By default it is
# the pinned build of scripts/build-mupdf.sh, made on first use; point it at
# another install, for example MUPDF_PREFIX=$(brew --prefix mupdf-tools), to
# use that instead. Releases always use the pinned build.
MUPDF_PINNED := $(CURDIR)/$(BUILD)/mupdf
MUPDF_PREFIX ?= $(MUPDF_PINNED)
MUPDF_VERSION := $(shell sed -n 's/^VERSION=//p' scripts/build-mupdf.sh)
MUPDF_SOURCE := $(BUILD)/mupdf-src/mupdf-$(MUPDF_VERSION)-source
MUPDF_LIBS := $(MUPDF_PREFIX)/lib/libmupdf.a $(MUPDF_PREFIX)/lib/libmupdf-third.a

# cgo finds MuPDF through these. Setting CGO_CFLAGS replaces Go's default
# of -O2 -g, so the default is kept.
CGO_CFLAGS ?= -O2 -g
CGO_CFLAGS += -I$(MUPDF_PREFIX)/include
CGO_LDFLAGS += -L$(MUPDF_PREFIX)/lib
export CGO_CFLAGS CGO_LDFLAGS

# Release signing. SIGN_IDENTITY is a "Developer ID Application" identity
# and NOTARY_PROFILE a notarytool keychain profile (see the README).
# SIGN_IDENTITY=- NOTARIZE=no makes an ad hoc, unnotarized release that
# exercises every other step.
SIGN_IDENTITY ?=
NOTARY_PROFILE ?=
NOTARIZE ?= yes
# Instead of NOTARY_PROFILE, an App Store Connect API key: the path of its
# .p8 file, its key ID and its issuer ID. CI has no keychain profile.
NOTARY_KEY ?=
NOTARY_KEY_ID ?=
NOTARY_ISSUER ?=

# Updates (Sparkle). Releases publish appcast.xml next to the zip, and the
# app reads the latest one. SPARKLE_PUBLIC_KEY turns the updater on; dev and
# CI builds leave it empty, so their updater stays off. A release that sets
# it signs the zip with SPARKLE_PRIVATE_KEY (the key itself) or
# SPARKLE_KEY_FILE (a file holding it) and writes the appcast.
REPOSITORY_URL ?= https://github.com/pedrohavay/mole
SPARKLE_FEED_URL ?= $(REPOSITORY_URL)/releases/latest/download/appcast.xml
SPARKLE_PUBLIC_KEY ?=
SPARKLE_KEY_FILE ?=
SPARKLE_BIN := $(CURDIR)/app/.build/artifacts/sparkle/Sparkle/bin

.PHONY: all core app bundle run verify test test-core test-app lint notices release mupdf clean distclean help FORCE

all: bundle

help:
	@echo "make              build $(APP)"
	@echo "make run          build and open the app"
	@echo "make verify       build, run the capture check (BOOK=path.epub adds a book)"
	@echo "make test         run the Go and Swift tests (MOLE_TEST_EPUB=path.epub enables the book tests)"
	@echo "make lint         gofmt and go vet"
	@echo "make release      signed, notarized zip in $(RELEASE) (SIGN_IDENTITY, NOTARY_PROFILE or NOTARY_KEY)"
	@echo "make notices      regenerate THIRD_PARTY_NOTICES"
	@echo "make mupdf        build the pinned MuPDF $(MUPDF_VERSION) into $(BUILD)/mupdf"
	@echo "make clean        remove the build products, keeping the MuPDF build"
	@echo "make distclean    remove everything, MuPDF included"

core: $(CORE_LIB)

mupdf: $(MUPDF_PINNED)/lib/libmupdf.a

$(MUPDF_PINNED)/lib/libmupdf.a $(MUPDF_PINNED)/lib/libmupdf-third.a:
	scripts/build-mupdf.sh $(MUPDF_PINNED)

# The stamp records which MuPDF the core was built with, so switching
# MUPDF_PREFIX rebuilds it. It is rewritten only when the prefix changes.
core/build/mupdf.prefix: FORCE
	@mkdir -p core/build
	@echo "$(MUPDF_PREFIX)" | cmp -s - $@ || echo "$(MUPDF_PREFIX)" > $@

# The core archive is the Go c-archive with MuPDF's libraries merged in, so
# the app links one library. It is built under temporary names and moved
# into place, so a failed build never leaves a half-written library.
$(CORE_LIB): $(CORE_SOURCES) core/go.mod core/go.sum $(MUPDF_LIBS) core/build/mupdf.prefix
	@mkdir -p core/build
	cd core && $(GO) build -trimpath -buildmode=c-archive -o build/libmolecore.go.a ./cmd/molecore
	rm -f core/build/libmolecore.tmp.a
	libtool -static -no_warning_for_no_symbols -o core/build/libmolecore.tmp.a \
		core/build/libmolecore.go.a $(MUPDF_LIBS) 2>&1 | grep -v "duplicate member name" || true
	test -s core/build/libmolecore.tmp.a
	mv core/build/libmolecore.go.h core/build/libmolecore.h
	mv core/build/libmolecore.tmp.a $(CORE_LIB)
	@cmp -s core/build/libmolecore.h $(CORE_HEADER) || cp core/build/libmolecore.h $(CORE_HEADER)

SWIFT_FLAGS := -c $(CONFIG)

app: core
	cd app && $(SWIFT) build $(SWIFT_FLAGS)

bundle: app
	SPARKLE_FEED_URL="$(SPARKLE_FEED_URL)" SPARKLE_PUBLIC_KEY="$(SPARKLE_PUBLIC_KEY)" \
		scripts/bundle.sh "$$(cd app && $(SWIFT) build $(SWIFT_FLAGS) --show-bin-path)" $(APP) \
		$(VERSION) $(BUILD_NUMBER) $(BUNDLE_ID)

run: bundle
	open $(APP)

verify: bundle
	@mkdir -p $(BUILD)/verify
	$(APP)/Contents/MacOS/Mole --verify-dir "$(abspath $(BUILD)/verify)" $(if $(BOOK),"$(BOOK)")

test: test-core test-app

test-core: $(MUPDF_LIBS)
	cd core && $(GO) test ./...

test-app: core
	cd app && $(SWIFT) test

lint: $(MUPDF_LIBS)
	@test -z "$$(gofmt -l core)" || { gofmt -l core; echo "gofmt: the files above need formatting"; exit 1; }
	cd core && $(GO) vet ./...

# THIRD_PARTY_NOTICES collects the licenses of everything linked into Mole:
# MuPDF's components and fonts from its source, Sparkle, and the Go modules.
notices: $(MUPDF_PINNED)/lib/libmupdf.a
	cd app && $(SWIFT) package resolve
	GO=$(GO) scripts/notices.sh "$(MUPDF_SOURCE)" > THIRD_PARTY_NOTICES.tmp
	mv THIRD_PARTY_NOTICES.tmp THIRD_PARTY_NOTICES

# A release is always built against the pinned MuPDF, which targets the
# oldest macOS Mole supports.
# SPARKLE_PRIVATE_KEY is a secret, so it stays in the environment instead
# of on a command line that make would print.
RELEASE_ENV = SIGN_IDENTITY="$(SIGN_IDENTITY)" NOTARIZE="$(NOTARIZE)" NOTARY_PROFILE="$(NOTARY_PROFILE)" \
	NOTARY_KEY="$(NOTARY_KEY)" NOTARY_KEY_ID="$(NOTARY_KEY_ID)" NOTARY_ISSUER="$(NOTARY_ISSUER)" \
	SPARKLE_PUBLIC_KEY="$(SPARKLE_PUBLIC_KEY)" SPARKLE_KEY_FILE="$(SPARKLE_KEY_FILE)" \
	SPARKLE_BIN="$(SPARKLE_BIN)" REPOSITORY_URL="$(REPOSITORY_URL)"

release:
	@$(RELEASE_ENV) scripts/release.sh --check
	$(MAKE) bundle MUPDF_PREFIX=$(MUPDF_PINNED)
	$(RELEASE_ENV) scripts/release.sh $(APP) $(RELEASE) $(VERSION)

clean:
	rm -rf $(APP) $(BUILD)/verify $(RELEASE) core/build app/.build $(CORE_HEADER)

distclean: clean
	rm -rf $(BUILD)

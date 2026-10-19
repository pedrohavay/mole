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
CORE_LIB := core/build/libmolecore.a
CORE_HEADER := app/Sources/MoleCore/libmolecore.h
CORE_SOURCES := $(shell find core/cmd core/internal -type f \( -name '*.go' -o -name '*.c' -o -name '*.h' \) ! -name '*_test.go')

.PHONY: all core app bundle run verify test test-core test-app lint clean help

all: bundle

help:
	@echo "make            build $(APP)"
	@echo "make run        build and open the app"
	@echo "make verify     build, run the capture check (BOOK=path.epub adds a book)"
	@echo "make test       run the Go and Swift tests (MOLE_TEST_EPUB=path.epub enables the book tests)"
	@echo "make lint       gofmt and go vet"
	@echo "make clean      remove every build product"

core: $(CORE_LIB)

# The archive is built under a temporary name and moved into place, so a
# failed build never leaves a half-written library for the Swift linker.
$(CORE_LIB): $(CORE_SOURCES) core/go.mod core/go.sum
	@mkdir -p core/build
	cd core && $(GO) build -trimpath -buildmode=c-archive -o build/libmolecore.tmp.a ./cmd/molecore
	mv core/build/libmolecore.tmp.h core/build/libmolecore.h
	mv core/build/libmolecore.tmp.a $(CORE_LIB)
	@cmp -s core/build/libmolecore.h $(CORE_HEADER) || cp core/build/libmolecore.h $(CORE_HEADER)

SWIFT_FLAGS := -c $(CONFIG)

app: core
	cd app && $(SWIFT) build $(SWIFT_FLAGS)

bundle: app
	scripts/bundle.sh "$$(cd app && $(SWIFT) build $(SWIFT_FLAGS) --show-bin-path)" $(APP) \
		$(VERSION) $(BUILD_NUMBER) $(BUNDLE_ID)

run: bundle
	open $(APP)

verify: bundle
	@mkdir -p $(BUILD)/verify
	$(APP)/Contents/MacOS/Mole --verify-dir "$(abspath $(BUILD)/verify)" $(if $(BOOK),"$(BOOK)")

test: test-core test-app

test-core:
	cd core && $(GO) test ./...

test-app: core
	cd app && $(SWIFT) test

lint:
	@test -z "$$(gofmt -l core)" || { gofmt -l core; echo "gofmt: the files above need formatting"; exit 1; }
	cd core && $(GO) vet ./...

clean:
	rm -rf $(BUILD) core/build app/.build $(CORE_HEADER)

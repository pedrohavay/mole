#!/bin/sh
# build-mupdf.sh builds the pinned MuPDF release from source into a local
# prefix with its headers and static libraries. Mole links this build by
# default (see MUPDF_PREFIX in the Makefile): it targets macOS 14, Mole's
# minimum, where a Homebrew bottle targets the macOS of the machine that
# built it, and it does not change when Homebrew upgrades.
#
# Usage: scripts/build-mupdf.sh [PREFIX]
#
# PREFIX defaults to build/mupdf. The source is kept in build/mupdf-src,
# where scripts/notices.sh reads the licenses of MuPDF's components.
set -eu

VERSION=1.26.1
SHA256=bdce017c776744c288b02102977ee0378cb436c78df8127a23f281f1360406fd
URL="https://mupdf.com/downloads/archive/mupdf-$VERSION-source.tar.gz"

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PREFIX=${1:-$ROOT/build/mupdf}
SRC="$ROOT/build/mupdf-src"
TARBALL="$SRC/mupdf-$VERSION-source.tar.gz"
TREE="$SRC/mupdf-$VERSION-source"

mkdir -p "$SRC"
if [ ! -f "$TARBALL" ]; then
	echo "build-mupdf.sh: downloading MuPDF $VERSION"
	curl --fail --location --retry 3 --output "$TARBALL.part" "$URL"
	mv "$TARBALL.part" "$TARBALL"
fi
echo "$SHA256  $TARBALL" | shasum -a 256 -c - >/dev/null || {
	echo "build-mupdf.sh: $TARBALL does not match the pinned checksum; delete it and run again" >&2
	exit 1
}

rm -rf "$TREE"
tar -xzf "$TARBALL" -C "$SRC"

# Only the libraries Mole uses: no viewers, no OCR, no barcodes, and no
# OpenSSL, which MuPDF would otherwise pick up through pkg-config and which
# Mole does not link.
export MACOSX_DEPLOYMENT_TARGET=14.0
make -C "$TREE" -j"$(sysctl -n hw.ncpu)" \
	build=release HAVE_X11=no HAVE_GLUT=no HAVE_LIBCRYPTO=no tesseract=no barcode=no \
	prefix="$PREFIX" install-libs >"$SRC/build.log" 2>&1 || {
	echo "build-mupdf.sh: the build failed; see $SRC/build.log" >&2
	exit 1
}
echo "$PREFIX"

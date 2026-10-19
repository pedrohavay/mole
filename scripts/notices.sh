#!/bin/sh
# notices.sh writes, to standard output, the notices of the third-party
# software linked into Mole: MuPDF, the libraries and fonts it builds in,
# Sparkle, the Go runtime and the Go modules. `make notices` runs it to
# regenerate THIRD_PARTY_NOTICES; run that after changing MuPDF's version,
# Sparkle's version or the Go dependencies.
#
# Usage: scripts/notices.sh MUPDF_SOURCE_DIR
#
# GO selects the Go toolchain, as in the Makefile. Sparkle's license comes
# from the package SwiftPM resolved into app/.build (swift package resolve).
set -eu

if [ $# -ne 1 ] || [ ! -f "$1/COPYING" ]; then
	echo "usage: $0 MUPDF_SOURCE_DIR (run make mupdf first)" >&2
	exit 2
fi
MUPDF=$1
GO=${GO:-go}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION=$(sed -n 's/^VERSION=//p' "$ROOT/scripts/build-mupdf.sh")

rule() {
	echo
	echo "================================================================================"
}

# section TITLE URL FILE... prints a heading and the files' contents.
section() {
	title=$1
	url=$2
	shift 2
	rule
	echo "$title"
	echo "$url"
	echo "================================================================================"
	for file in "$@"; do
		echo
		cat "$file"
	done
}

cat <<EOF
Third-party notices for Mole

Mole is free software under the GNU Affero General Public License, version 3
or later; LICENSE holds its text, and the source code is at
https://github.com/pedrohavay/mole.

Mole includes the software below. Each part is used under the license that
follows its name. Parts under the GNU AGPL refer to the copy in LICENSE.
EOF

section "MuPDF $VERSION" "https://mupdf.com"
cat <<EOF

MuPDF is Copyright (c) 2006-2023 Artifex Software, Inc., and is licensed
under the GNU Affero General Public License, version 3 or later (see
LICENSE). The libraries and fonts below are part of the MuPDF distribution.
EOF

T="$MUPDF/thirdparty"
section "Brotli" "https://github.com/google/brotli" "$T/brotli/LICENSE"
section "Extract (Artifex), GNU AGPL 3.0" "https://github.com/ArtifexSoftware/extract"
echo
echo "Copyright (C) Artifex Software, Inc. Licensed under the GNU AGPL 3.0 (see LICENSE)."
section "FreeType, under the FreeType License" "https://freetype.org" "$T/freetype/docs/FTL.TXT"
section "Gumbo HTML parser" "https://github.com/google/gumbo-parser" "$T/gumbo-parser/COPYING"
section "HarfBuzz" "https://harfbuzz.github.io" "$T/harfbuzz/COPYING"
section "jbig2dec, GNU AGPL 3.0 or later" "https://jbig2dec.com" "$T/jbig2dec/LICENSE"
section "Little CMS (lcms2mt)" "https://www.littlecms.com" "$T/lcms2/LICENSE"
section "libjpeg (Independent JPEG Group)" "https://ijg.org"
echo
awk '/^LEGAL ISSUES$/ { on = 1 } /^REFERENCES$/ { on = 0 } on' "$T/libjpeg/README"
section "MuJS" "https://mujs.com" "$T/mujs/COPYING"
section "OpenJPEG" "https://www.openjpeg.org" "$T/openjpeg/LICENSE"
section "zlib" "https://zlib.net" "$T/zlib/LICENSE"

F="$MUPDF/resources/fonts"
section "Fonts: URW base 35 (Nimbus, Standard Symbols, Dingbats)" "https://github.com/ArtifexSoftware/urw-base35-fonts" "$F/urw/OFL.txt"
section "Fonts: Noto" "https://notofonts.github.io" "$F/noto/COPYING"
section "Fonts: Charis SIL" "https://software.sil.org/charis" "$F/sil/OFL.txt"
section "Fonts: Source Han Serif" "https://github.com/adobe-fonts/source-han-serif" "$F/han/LICENSE.txt"
section "Fonts: Droid Sans Fallback" "https://android.googlesource.com/platform/frameworks/base" "$F/droid/NOTICE"

# Sparkle's LICENSE also holds the licenses of the code it includes.
SPARKLE="$ROOT/app/.build/artifacts/sparkle/Sparkle/LICENSE"
if [ ! -f "$SPARKLE" ]; then
	echo "notices.sh: no $SPARKLE; run swift package resolve in app first" >&2
	exit 1
fi
SPARKLE_VERSION=$(awk '/"identity" *: *"sparkle"/ { on = 1 } on && /"version"/ { gsub(/[",]/, "", $3); print $3; exit }' "$ROOT/app/Package.resolved")
section "Sparkle $SPARKLE_VERSION" "https://sparkle-project.org" "$SPARKLE"

GOROOT=$(cd "$ROOT/core" && "$GO" env GOROOT)
section "The Go runtime and standard library" "https://go.dev" "$GOROOT/LICENSE"

# The modules that end up in the binary, with the license file of each.
cd "$ROOT/core"
"$GO" list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/molecore |
	sort -u |
	while read -r path version dir; do
		license=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | sort | head -n 1)
		if [ -z "$license" ]; then
			echo "notices.sh: no license file in $dir" >&2
			exit 1
		fi
		section "Go module $path $version" "https://pkg.go.dev/$path" "$license"
	done

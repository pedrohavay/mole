#!/bin/sh
# bundle.sh assembles Mole.app from a SwiftPM build directory and signs it
# ad hoc. The Makefile calls it; run it directly only to repackage a build.
#
# Usage: scripts/bundle.sh BIN_DIR APP VERSION BUILD BUNDLE_ID
#
# VERSION is the marketing version (0.1.0); BUILD is a build number that
# only grows, which macOS uses to tell builds of one version apart.
set -eu

if [ $# -ne 5 ]; then
	echo "usage: $0 BIN_DIR APP VERSION BUILD BUNDLE_ID" >&2
	exit 2
fi
BIN_DIR=$1
APP=$2
VERSION=$3
BUILD=$4
BUNDLE_ID=$5
ROOT=$(cd "$(dirname "$0")/.." && pwd)
LANGUAGES="en pt-BR"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN_DIR/Mole" "$APP/Contents/MacOS/Mole"
# SwiftPM's resource bundle holds the string tables (see L10n.bundle).
cp -R "$BIN_DIR/Mole_Mole.bundle" "$APP/Contents/Resources/"
cp "$ROOT/assets/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
# The AGPL and the licenses of the parts Mole includes travel with the app.
# They get a .txt extension so that Help > Third-Party Notices opens them in
# the user's text editor.
cp "$ROOT/LICENSE" "$APP/Contents/Resources/LICENSE.txt"
cp "$ROOT/THIRD_PARTY_NOTICES" "$APP/Contents/Resources/THIRD_PARTY_NOTICES.txt"

# The main bundle must list its languages too, or AppKit shows the system
# menus (Edit, Window, Help) in English whatever the user's language.
# InfoPlist.strings also translates the document type name Finder shows.
LOCALIZATIONS=""
for language in $LANGUAGES; do
	case $language in
	pt-BR) EPUB_NAME="Livro EPUB" ;;
	*) EPUB_NAME="EPUB Book" ;;
	esac
	mkdir -p "$APP/Contents/Resources/$language.lproj"
	printf '"CFBundleName" = "Mole";\n"EPUB Book" = "%s";\n' "$EPUB_NAME" \
		> "$APP/Contents/Resources/$language.lproj/InfoPlist.strings"
	# The About panel shows Credits.rtf in the user's language.
	cp "$ROOT/assets/credits/$language.rtf" "$APP/Contents/Resources/$language.lproj/Credits.rtf"
	LOCALIZATIONS="$LOCALIZATIONS		<string>$language</string>
"
done

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Mole</string>
	<key>CFBundleDisplayName</key>
	<string>Mole</string>
	<key>CFBundleIdentifier</key>
	<string>$BUNDLE_ID</string>
	<key>CFBundleExecutable</key>
	<string>Mole</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$BUILD</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright © $(date +%Y) Pedro Havay. GNU AGPL 3.0 or later.</string>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleLocalizations</key>
	<array>
$LOCALIZATIONS	</array>
	<key>CFBundleDocumentTypes</key>
	<array>
		<dict>
			<key>CFBundleTypeName</key>
			<string>EPUB Book</string>
			<key>CFBundleTypeRole</key>
			<string>Viewer</string>
			<key>LSHandlerRank</key>
			<string>Alternate</string>
			<key>LSItemContentTypes</key>
			<array>
				<string>org.idpf.epub-container</string>
			</array>
		</dict>
	</array>
	<!-- A cover dragged onto a collection inside Mole (BookDrag). -->
	<key>UTExportedTypeDeclarations</key>
	<array>
		<dict>
			<key>UTTypeIdentifier</key>
			<string>io.github.pedrohavay.mole.book</string>
			<key>UTTypeDescription</key>
			<string>Mole Library Book</string>
			<key>UTTypeConformsTo</key>
			<array>
				<string>public.data</string>
			</array>
		</dict>
	</array>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.books</string>
	<key>LSMinimumSystemVersion</key>
	<string>14.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

plutil -lint "$APP/Contents/Info.plist" >/dev/null
codesign --force --sign - "$APP" >/dev/null 2>&1
echo "$APP"

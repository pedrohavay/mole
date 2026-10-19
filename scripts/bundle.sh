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
# InfoPlist.strings also translates the names of the document types and of
# the types Mole declares, which Finder shows.
LOCALIZATIONS=""
for language in $LANGUAGES; do
	case $language in
	pt-BR) EPUB_NAME="Livro EPUB" MOBI_NAME="Livro MOBI" FB2_NAME="Livro FB2" TEXT_NAME="Texto simples" ;;
	*) EPUB_NAME="EPUB Book" MOBI_NAME="MOBI Book" FB2_NAME="FB2 Book" TEXT_NAME="Plain Text" ;;
	esac
	mkdir -p "$APP/Contents/Resources/$language.lproj"
	{
		printf '"CFBundleName" = "Mole";\n'
		printf '"EPUB Book" = "%s";\n"MOBI Book" = "%s";\n' "$EPUB_NAME" "$MOBI_NAME"
		printf '"FB2 Book" = "%s";\n"Plain Text" = "%s";\n' "$FB2_NAME" "$TEXT_NAME"
	} > "$APP/Contents/Resources/$language.lproj/InfoPlist.strings"
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
		<dict>
			<key>CFBundleTypeName</key>
			<string>MOBI Book</string>
			<key>CFBundleTypeRole</key>
			<string>Viewer</string>
			<key>LSHandlerRank</key>
			<string>Alternate</string>
			<key>LSItemContentTypes</key>
			<array>
				<string>com.mobipocket.mobi</string>
				<!-- The Kindle app exports this type for .mobi and .prc, and it
				     wins over Mole's declaration where Kindle is installed. -->
				<string>com.amazon.kindle-document</string>
			</array>
		</dict>
		<dict>
			<key>CFBundleTypeName</key>
			<string>FB2 Book</string>
			<key>CFBundleTypeRole</key>
			<string>Viewer</string>
			<key>LSHandlerRank</key>
			<string>Alternate</string>
			<key>LSItemContentTypes</key>
			<array>
				<string>org.fictionbook.fb2</string>
			</array>
		</dict>
		<dict>
			<key>CFBundleTypeName</key>
			<string>Plain Text</string>
			<key>CFBundleTypeRole</key>
			<string>Viewer</string>
			<key>LSHandlerRank</key>
			<string>Alternate</string>
			<key>LSItemContentTypes</key>
			<array>
				<string>public.plain-text</string>
			</array>
		</dict>
	</array>
	<!-- macOS knows no type for MOBI, PRC or FB2 files. Imported
	     declarations name them unless another app exports its own. -->
	<key>UTImportedTypeDeclarations</key>
	<array>
		<dict>
			<key>UTTypeIdentifier</key>
			<string>com.mobipocket.mobi</string>
			<key>UTTypeDescription</key>
			<string>MOBI Book</string>
			<key>UTTypeConformsTo</key>
			<array>
				<string>public.data</string>
				<string>public.content</string>
			</array>
			<key>UTTypeTagSpecification</key>
			<dict>
				<key>public.filename-extension</key>
				<array>
					<string>mobi</string>
					<string>prc</string>
				</array>
				<key>public.mime-type</key>
				<array>
					<string>application/x-mobipocket-ebook</string>
				</array>
			</dict>
		</dict>
		<dict>
			<key>UTTypeIdentifier</key>
			<string>org.fictionbook.fb2</string>
			<key>UTTypeDescription</key>
			<string>FB2 Book</string>
			<key>UTTypeConformsTo</key>
			<array>
				<string>public.xml</string>
				<string>public.content</string>
			</array>
			<key>UTTypeTagSpecification</key>
			<dict>
				<key>public.filename-extension</key>
				<array>
					<string>fb2</string>
				</array>
				<key>public.mime-type</key>
				<array>
					<string>application/x-fictionbook+xml</string>
				</array>
			</dict>
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

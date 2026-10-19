#!/bin/sh
# release.sh turns build/Mole.app into a release: it signs the app and the
# Sparkle framework inside it with a Developer ID and the hardened runtime,
# checks that the signed app starts, notarizes it, staples the ticket, and
# packs OUT_DIR/Mole-VERSION.zip with its SHA-256. When the app checks for
# updates, it also signs the zip for Sparkle and writes OUT_DIR/appcast.xml.
# `make release` builds the app and calls it.
#
# Usage: scripts/release.sh APP OUT_DIR VERSION
#        scripts/release.sh --check     only check the environment below
#
# Environment:
#   SIGN_IDENTITY        a "Developer ID Application: Name (TEAMID)" identity
#                        from the keychain (or its SHA-1 hash), or - for an
#                        ad hoc signature
#   NOTARIZE             yes (the default) or no; an ad hoc signature cannot
#                        be notarized, so SIGN_IDENTITY=- needs NOTARIZE=no
#   NOTARY_PROFILE       a keychain profile made with xcrun notarytool
#                        store-credentials; or else these three:
#   NOTARY_KEY           the .p8 file of an App Store Connect API key
#   NOTARY_KEY_ID        the key's ID
#   NOTARY_ISSUER        the issuer ID of the team's API keys
#   SPARKLE_PUBLIC_KEY   for --check, the EdDSA public key the app will hold;
#                        otherwise the app's SUPublicEDKey is used
#   SPARKLE_PRIVATE_KEY  the matching private key, as generate_keys -x
#                        exports it; or
#   SPARKLE_KEY_FILE     a file holding that private key
#   SPARKLE_BIN          the folder with Sparkle's sign_update
#   REPOSITORY_URL       the GitHub repository the release is published in
set -eu

fail() {
	echo "release.sh: $*" >&2
	exit 1
}

SIGN_IDENTITY=${SIGN_IDENTITY:-}
NOTARIZE=${NOTARIZE:-yes}
NOTARY_PROFILE=${NOTARY_PROFILE:-}
NOTARY_KEY=${NOTARY_KEY:-}
NOTARY_KEY_ID=${NOTARY_KEY_ID:-}
NOTARY_ISSUER=${NOTARY_ISSUER:-}
SPARKLE_PUBLIC_KEY=${SPARKLE_PUBLIC_KEY:-}
SPARKLE_PRIVATE_KEY=${SPARKLE_PRIVATE_KEY:-}
SPARKLE_KEY_FILE=${SPARKLE_KEY_FILE:-}
SPARKLE_BIN=${SPARKLE_BIN:-}
REPOSITORY_URL=${REPOSITORY_URL:-https://github.com/pedrohavay/mole}
ROOT=$(cd "$(dirname "$0")/.." && pwd)

# check_notary checks that there is one complete set of notarytool
# credentials: a keychain profile or an App Store Connect API key.
check_notary() {
	if [ -n "$NOTARY_PROFILE" ]; then
		[ -z "$NOTARY_KEY$NOTARY_KEY_ID$NOTARY_ISSUER" ] ||
			fail "set either NOTARY_PROFILE or NOTARY_KEY, NOTARY_KEY_ID and NOTARY_ISSUER, not both"
		return
	fi
	[ -n "$NOTARY_KEY$NOTARY_KEY_ID$NOTARY_ISSUER" ] ||
		fail "set NOTARY_PROFILE to a notarytool keychain profile (xcrun notarytool store-credentials), or NOTARY_KEY, NOTARY_KEY_ID and NOTARY_ISSUER to an App Store Connect API key"
	missing=""
	[ -n "$NOTARY_KEY" ] || missing="$missing NOTARY_KEY"
	[ -n "$NOTARY_KEY_ID" ] || missing="$missing NOTARY_KEY_ID"
	[ -n "$NOTARY_ISSUER" ] || missing="$missing NOTARY_ISSUER"
	[ -z "$missing" ] || fail "an App Store Connect API key needs NOTARY_KEY, NOTARY_KEY_ID and NOTARY_ISSUER; missing:$missing"
	[ -f "$NOTARY_KEY" ] && [ -r "$NOTARY_KEY" ] || fail "NOTARY_KEY: $NOTARY_KEY is not a readable file"
	grep -q -- '-----BEGIN PRIVATE KEY-----' "$NOTARY_KEY" ||
		fail "NOTARY_KEY: $NOTARY_KEY is not an App Store Connect key; the AuthKey_<key ID>.p8 file starts with -----BEGIN PRIVATE KEY-----"
	echo "$NOTARY_KEY_ID" | grep -Eq '^[A-Za-z0-9]+$' ||
		fail "NOTARY_KEY_ID: $NOTARY_KEY_ID is not a key ID, which is letters and digits, usually 10 of them, such as 2X9R4HXF34"
	echo "$NOTARY_ISSUER" | grep -Eiq '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' ||
		fail "NOTARY_ISSUER: $NOTARY_ISSUER is not an issuer ID, which is a UUID such as 57246542-96fe-1a63-e053-0824d011072a"
}

# check_sparkle PUBLIC_KEY checks that the release can sign updates exactly
# when the app checks for them: an app with a public key needs its private
# key here, or the release would have no appcast, and a private key with no
# public key in the app would sign updates nobody verifies.
check_sparkle() {
	[ -z "$SPARKLE_PRIVATE_KEY" ] || [ -z "$SPARKLE_KEY_FILE" ] ||
		fail "set either SPARKLE_PRIVATE_KEY or SPARKLE_KEY_FILE, not both"
	if [ -n "$SPARKLE_KEY_FILE" ]; then
		[ -f "$SPARKLE_KEY_FILE" ] && [ -r "$SPARKLE_KEY_FILE" ] ||
			fail "SPARKLE_KEY_FILE: $SPARKLE_KEY_FILE is not a readable file"
	fi
	if [ -n "$1" ]; then
		[ "$(printf '%s' "$1" | base64 -D 2>/dev/null | wc -c | tr -d ' ')" = 32 ] ||
			fail "SPARKLE_PUBLIC_KEY: $1 is not an EdDSA public key, which is 44 characters of base64 (generate_keys -p prints it)"
		[ -n "$SPARKLE_PRIVATE_KEY$SPARKLE_KEY_FILE" ] ||
			fail "the app checks for updates, so the release must sign them: set SPARKLE_PRIVATE_KEY or SPARKLE_KEY_FILE to the private key of SPARKLE_PUBLIC_KEY"
	else
		[ -z "$SPARKLE_PRIVATE_KEY$SPARKLE_KEY_FILE" ] ||
			fail "a Sparkle private key is set but the app has no public key, so it would not check for updates; set SPARKLE_PUBLIC_KEY too"
	fi
}

[ -n "$SIGN_IDENTITY" ] || fail 'set SIGN_IDENTITY to a "Developer ID Application" identity (security find-identity -v -p codesigning lists them)'
case $NOTARIZE in
yes)
	[ "$SIGN_IDENTITY" != - ] || fail "an ad hoc signature cannot be notarized; add NOTARIZE=no"
	check_notary
	;;
no) ;;
*) fail "NOTARIZE must be yes or no" ;;
esac
if [ "${1:-}" = --check ]; then
	check_sparkle "$SPARKLE_PUBLIC_KEY"
	exit 0
fi

[ $# -eq 3 ] || fail "usage: $0 APP OUT_DIR VERSION"
APP=$1
OUT=$2
VERSION=$3
[ -d "$APP" ] || fail "$APP does not exist; run make bundle first"
FRAMEWORK="$APP/Contents/Frameworks/Sparkle.framework"
[ -d "$FRAMEWORK" ] || fail "$APP has no Sparkle.framework; run make bundle again"

INFO="$APP/Contents/Info.plist"
plist() {
	/usr/libexec/PlistBuddy -c "Print :$1" "$INFO" 2>/dev/null || true
}
[ "$(plist CFBundleShortVersionString)" = "$VERSION" ] ||
	fail "$APP is version $(plist CFBundleShortVersionString), not $VERSION"
PUBLIC_KEY=$(plist SUPublicEDKey)
check_sparkle "$PUBLIC_KEY"
if [ -n "$PUBLIC_KEY" ]; then
	[ -x "$SPARKLE_BIN/sign_update" ] || fail "no sign_update in SPARKLE_BIN ($SPARKLE_BIN); make app downloads it"
fi

mkdir -p "$OUT"
ZIP="$OUT/Mole-$VERSION.zip"
APPCAST="$OUT/appcast.xml"
rm -f "$ZIP" "$ZIP.sha256" "$APPCAST"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# The hardened runtime is required for notarization. A secure timestamp is
# too, but an ad hoc signature cannot have one.
TIMESTAMP=--timestamp
[ "$SIGN_IDENTITY" != - ] || TIMESTAMP=--timestamp=none
sign() {
	codesign --force --options runtime $TIMESTAMP --sign "$SIGN_IDENTITY" "$@"
}

# The hardened runtime's library validation loads only code signed by the
# app's own team. An ad hoc signature has no team, so an ad hoc app turns
# validation off to load Sparkle; a Developer ID release keeps it on.
APP_ENTITLEMENTS=""
if [ "$SIGN_IDENTITY" = - ]; then
	APP_ENTITLEMENTS="$WORK/adhoc.entitlements"
	cat > "$APP_ENTITLEMENTS" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.cs.disable-library-validation</key>
	<true/>
</dict>
</plist>
PLIST
fi

# Sparkle's nested code is signed inside out, as Sparkle documents for apps
# signed outside Xcode, and never with --deep, which would give every part
# the entitlements of one. bundle.sh leaves the XPC services out; if they are
# there, the Downloader keeps its own entitlements.
echo "release.sh: signing $APP"
VERSIONED=$(cd "$FRAMEWORK/Versions/Current" && pwd -P)
for service in "$VERSIONED"/XPCServices/*.xpc; do
	[ ! -e "$service" ] || sign --preserve-metadata=entitlements "$service"
done
sign "$VERSIONED/Autoupdate"
sign "$VERSIONED/Updater.app"
sign "$FRAMEWORK"
if [ -n "$APP_ENTITLEMENTS" ]; then
	sign --entitlements "$APP_ENTITLEMENTS" "$APP"
else
	sign "$APP"
fi
codesign --verify --strict --deep "$APP"

echo "release.sh: starting the signed app"
MOLE_LIBRARY="$WORK/library.db" "$APP/Contents/MacOS/Mole" --smoke-test || fail "the signed app does not start"

# notarytool runs xcrun notarytool with the credentials set above.
notarytool() {
	if [ -n "$NOTARY_PROFILE" ]; then
		xcrun notarytool "$@" --keychain-profile "$NOTARY_PROFILE"
	else
		xcrun notarytool "$@" --key "$NOTARY_KEY" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER"
	fi
}

if [ "$NOTARIZE" = yes ]; then
	echo "release.sh: notarizing (this takes a few minutes)"
	ditto -c -k --keepParent "$APP" "$WORK/Mole.zip"
	RESULT=$(notarytool submit "$WORK/Mole.zip" --wait --output-format json) || true
	echo "$RESULT"
	if ! echo "$RESULT" | grep -q '"status" *: *"Accepted"'; then
		ID=$(echo "$RESULT" | sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p')
		[ -z "$ID" ] || notarytool log "$ID" >&2 || true
		fail "notarization failed; notarytool's output above, and its log when there is one, say why"
	fi
	xcrun stapler staple "$APP"
	xcrun stapler validate "$APP"
	spctl --assess --type execute --verbose "$APP"
fi

# The zip and the appcast are made in WORK and reach OUT_DIR only once
# every check has passed.
NAME=$(basename "$ZIP")
ditto -c -k --keepParent "$APP" "$WORK/$NAME"

# The appcast is what installed copies read to find this release. Its item
# names the zip by its download address on the GitHub release of tag
# vVERSION, with the EdDSA signature Sparkle checks before it installs it.
if [ -n "$PUBLIC_KEY" ]; then
	echo "release.sh: signing the update"
	if [ -n "$SPARKLE_PRIVATE_KEY" ]; then
		SIGNATURE=$(printf '%s\n' "$SPARKLE_PRIVATE_KEY" | "$SPARKLE_BIN/sign_update" --ed-key-file - -p "$WORK/$NAME")
	else
		SIGNATURE=$("$SPARKLE_BIN/sign_update" --ed-key-file "$SPARKLE_KEY_FILE" -p "$WORK/$NAME")
	fi
	[ -n "$SIGNATURE" ] || fail "sign_update gave no signature"
	# A key pair that does not match would ship an update no copy installs.
	xcrun swift "$ROOT/scripts/ed25519-verify.swift" "$PUBLIC_KEY" "$SIGNATURE" "$WORK/$NAME" ||
		fail "the update's signature does not match the app's SUPublicEDKey; SPARKLE_PRIVATE_KEY or SPARKLE_KEY_FILE holds another key"
	cat > "$WORK/appcast.xml" <<XML
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
	<channel>
		<title>Mole</title>
		<link>$REPOSITORY_URL</link>
		<item>
			<title>Mole $VERSION</title>
			<pubDate>$(LC_ALL=C date -u '+%a, %d %b %Y %H:%M:%S +0000')</pubDate>
			<sparkle:version>$(plist CFBundleVersion)</sparkle:version>
			<sparkle:shortVersionString>$VERSION</sparkle:shortVersionString>
			<sparkle:minimumSystemVersion>$(plist LSMinimumSystemVersion)</sparkle:minimumSystemVersion>
			<sparkle:releaseNotesLink>$REPOSITORY_URL/releases/tag/v$VERSION</sparkle:releaseNotesLink>
			<enclosure url="$REPOSITORY_URL/releases/download/v$VERSION/$NAME" length="$(stat -f %z "$WORK/$NAME")" type="application/octet-stream" sparkle:edSignature="$SIGNATURE"/>
		</item>
	</channel>
</rss>
XML
	xmllint --noout "$WORK/appcast.xml"
	mv "$WORK/appcast.xml" "$APPCAST"
	echo "$APPCAST"
else
	echo "release.sh: the app has no SUPublicEDKey, so there is no appcast"
fi
mv "$WORK/$NAME" "$ZIP"
(cd "$OUT" && shasum -a 256 "$NAME" > "$NAME.sha256")
echo "$ZIP"

#!/bin/sh
# release.sh turns build/Mole.app into a release: it signs the app with a
# Developer ID and the hardened runtime, checks that the signed app starts,
# notarizes it, staples the ticket, and packs OUT_DIR/Mole-VERSION.zip with
# its SHA-256. `make release` builds the app and calls it.
#
# Usage: scripts/release.sh APP OUT_DIR VERSION
#        scripts/release.sh --check     only check the environment below
#
# Environment:
#   SIGN_IDENTITY   a "Developer ID Application: Name (TEAMID)" identity from
#                   the keychain, or - for an ad hoc signature
#   NOTARY_PROFILE  a keychain profile made with xcrun notarytool
#                   store-credentials
#   NOTARIZE        yes (the default) or no; an ad hoc signature cannot be
#                   notarized, so SIGN_IDENTITY=- needs NOTARIZE=no
set -eu

fail() {
	echo "release.sh: $*" >&2
	exit 1
}

SIGN_IDENTITY=${SIGN_IDENTITY:-}
NOTARY_PROFILE=${NOTARY_PROFILE:-}
NOTARIZE=${NOTARIZE:-yes}

[ -n "$SIGN_IDENTITY" ] || fail 'set SIGN_IDENTITY to a "Developer ID Application" identity (security find-identity -v -p codesigning lists them)'
case $NOTARIZE in
yes)
	[ "$SIGN_IDENTITY" != - ] || fail "an ad hoc signature cannot be notarized; add NOTARIZE=no"
	[ -n "$NOTARY_PROFILE" ] || fail "set NOTARY_PROFILE to a notarytool keychain profile (xcrun notarytool store-credentials)"
	;;
no) ;;
*) fail "NOTARIZE must be yes or no" ;;
esac
if [ "${1:-}" = --check ]; then
	exit 0
fi

[ $# -eq 3 ] || fail "usage: $0 APP OUT_DIR VERSION"
APP=$1
OUT=$2
VERSION=$3
[ -d "$APP" ] || fail "$APP does not exist; run make bundle first"

mkdir -p "$OUT"
ZIP="$OUT/Mole-$VERSION.zip"

# The hardened runtime is required for notarization. A secure timestamp is
# too, but an ad hoc signature cannot have one.
TIMESTAMP=--timestamp
[ "$SIGN_IDENTITY" != - ] || TIMESTAMP=--timestamp=none
echo "release.sh: signing $APP"
codesign --force --options runtime $TIMESTAMP --sign "$SIGN_IDENTITY" "$APP"
codesign --verify --strict --deep "$APP"

echo "release.sh: starting the signed app"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
MOLE_LIBRARY="$WORK/library.db" "$APP/Contents/MacOS/Mole" --smoke-test || fail "the signed app does not start"

if [ "$NOTARIZE" = yes ]; then
	echo "release.sh: notarizing (this takes a few minutes)"
	ditto -c -k --keepParent "$APP" "$WORK/Mole.zip"
	RESULT=$(xcrun notarytool submit "$WORK/Mole.zip" --keychain-profile "$NOTARY_PROFILE" --wait --output-format json)
	echo "$RESULT"
	echo "$RESULT" | grep -q '"status" *: *"Accepted"' ||
		fail "notarization was not accepted; xcrun notarytool log <id> --keychain-profile $NOTARY_PROFILE explains why"
	xcrun stapler staple "$APP"
	xcrun stapler validate "$APP"
	spctl --assess --type execute --verbose "$APP"
fi

rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"
(cd "$OUT" && shasum -a 256 "$(basename "$ZIP")" > "$(basename "$ZIP").sha256")
echo "$ZIP"

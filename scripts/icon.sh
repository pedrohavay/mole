#!/bin/sh
# icon.sh renders the app icon from assets/Mole.svg: the PNGs of
# assets/AppIcon.appiconset and assets/AppIcon.icns. Run it after changing
# the SVG. It needs rsvg-convert (brew install librsvg).
#
# The SVG follows Apple's macOS icon grid: an 824 px body on the 1024 px
# canvas, with the shadow in the margin. Apple draws its 16 and 32 px icons
# with a larger body, 28 of 32 px, so those sizes are rendered from a tighter
# view of the same drawing.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SVG="$ROOT/assets/Mole.svg"
SET="$ROOT/assets/AppIcon.appiconset"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

command -v rsvg-convert >/dev/null || { echo "icon.sh: rsvg-convert not found (brew install librsvg)" >&2; exit 1; }

# The small-size view: the 824 px body fills 28/32 of it, centered.
sed 's|viewBox="0 0 1024 1024"|viewBox="41.143 41.143 941.714 941.714"|' "$SVG" > "$WORK/small.svg"

render() { # render SOURCE PIXELS NAME
	rsvg-convert --width "$2" --height "$2" "$1" --output "$SET/$3"
}

render "$SVG" 1024 icon_512x512@2x.png
render "$SVG" 512 icon_512x512.png
render "$SVG" 512 icon_256x256@2x.png
render "$SVG" 256 icon_256x256.png
render "$SVG" 256 icon_128x128@2x.png
render "$SVG" 128 icon_128x128.png
render "$SVG" 64 icon_32x32@2x.png
render "$WORK/small.svg" 32 icon_32x32.png
render "$WORK/small.svg" 32 icon_16x16@2x.png
render "$WORK/small.svg" 16 icon_16x16.png

# iconutil wants a folder named .iconset holding only the PNGs.
mkdir "$WORK/AppIcon.iconset"
cp "$SET"/icon_*.png "$WORK/AppIcon.iconset/"
iconutil --convert icns --output "$ROOT/assets/AppIcon.icns" "$WORK/AppIcon.iconset"
echo "$ROOT/assets/AppIcon.icns"

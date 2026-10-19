# dmg-settings.py lays out Mole's disk image for dmgbuild, which release.sh
# runs as: dmgbuild -s scripts/dmg-settings.py -D app=build/Mole.app Mole OUT.
#
# The window shows the app on the left and a link to Applications on the
# right, to drag one onto the other. It keeps Finder's own background, so
# the icon labels stay readable in both light and dark mode.
import os.path

# dmgbuild runs this file with `defines` set from its -D options.
app = defines["app"]  # noqa: F821
app_name = os.path.basename(app)

# Contents.
files = [app]
symlinks = {"Applications": "/Applications"}
hide_extensions = [app_name]

# The volume's own icon, as Finder shows it on the desktop and in the sidebar.
icon = os.path.join(app, "Contents", "Resources", "AppIcon.icns")

# The image, compressed with LZFSE, which macOS 10.11 and later read.
format = "ULFO"
filesystem = "HFS+"

# The window: no toolbar, sidebar, path bar or status bar, and big icons.
window_rect = ((200, 200), (560, 340))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
icon_size = 128
text_size = 13
icon_locations = {
    app_name: (150, 150),
    "Applications": (410, 150),
}

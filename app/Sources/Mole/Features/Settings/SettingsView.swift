import SwiftUI

/// SettingsView is the Settings window: appearance, updates, reading and the
/// list of keyboard shortcuts.
@MainActor
struct SettingsView: View {
    @AppStorage(SettingsTab.storageKey) private var selection = SettingsTab.general

    var body: some View {
        TabView(selection: $selection) {
            GeneralSettings()
                .tabItem { Label(L10n.text("settings.general"), systemImage: "gearshape") }
                .tag(SettingsTab.general)
            ReadingSettings()
                .tabItem { Label(L10n.text("settings.reading"), systemImage: "book") }
                .tag(SettingsTab.reading)
            ShortcutsSettings()
                .tabItem { Label(L10n.text("settings.shortcuts"), systemImage: "keyboard") }
                .tag(SettingsTab.shortcuts)
        }
        .frame(width: 520)
    }
}

/// SettingsTab names the tabs of the Settings window.
enum SettingsTab: String {
    case general, reading, shortcuts

    /// StorageKey is the user default that remembers the open tab.
    static let storageKey = "settingsTab"

    /// Opens the Settings window, at the tab last shown.
    @MainActor
    static func openWindow() {
        // Views can use the openSettings environment action, but menu
        // commands and the verifier have no environment; the app menu's
        // Settings item works from anywhere.
        guard let menu = NSApp.mainMenu?.items.first?.submenu,
              let index = menu.items.firstIndex(where: { $0.keyEquivalent == "," }) else { return }
        menu.performActionForItem(at: index)
    }
}

@MainActor
private struct GeneralSettings: View {
    @Environment(AppModel.self) private var model
    @Environment(Updater.self) private var updater

    var body: some View {
        @Bindable var updater = updater
        Form {
            Section(L10n.text("settings.appearance")) {
                LabeledContent(L10n.text("settings.accent")) {
                    HStack(spacing: Theme.Space.s) {
                        ForEach(Accent.presets) { preset in
                            swatch(preset)
                        }
                        ColorPicker(L10n.text("settings.accent.custom"), selection: customAccent, supportsOpacity: false)
                            .labelsHidden()
                            .help(L10n.text("settings.accent.custom"))
                    }
                }
                Text(L10n.text("settings.accent.note"))
                    .font(Theme.caption)
                    .foregroundStyle(Theme.secondary)
            }
            Section(L10n.text("settings.language")) {
                Text(L10n.text("settings.language.note"))
                    .font(Theme.caption)
                    .foregroundStyle(Theme.secondary)
            }
            // Builds without an updater (development and CI) have nothing to set.
            if updater.isEnabled {
                Section(L10n.text("settings.updates")) {
                    Toggle(L10n.text("settings.updates.automatic"), isOn: $updater.checksAutomatically)
                    Text(L10n.text("settings.updates.note"))
                        .font(Theme.caption)
                        .foregroundStyle(Theme.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .frame(height: updater.isEnabled ? 370 : 260)
    }

    private func swatch(_ preset: Accent) -> some View {
        let selected = (model.settings.accent.isEmpty ? Accent.presets[0].hex : model.settings.accent)
            .caseInsensitiveCompare(preset.hex) == .orderedSame
        return Button {
            // The default is stored as empty, so a future default applies.
            model.settings.accent = preset.key == Accent.presets[0].key ? "" : preset.hex
        } label: {
            Circle()
                .fill(preset.color)
                .frame(width: 18, height: 18)
                .overlay(Circle().strokeBorder(Color.white.opacity(selected ? 0.9 : 0), lineWidth: 2))
                .padding(2)
                .overlay(Circle().strokeBorder(selected ? preset.color : .clear, lineWidth: 1))
        }
        .buttonStyle(.plain)
        .help(preset.name)
        .accessibilityLabel(preset.name)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private var customAccent: Binding<Color> {
        Binding {
            model.accent
        } set: { color in
            if let hex = color.hexString { model.settings.accent = hex }
        }
    }
}

@MainActor
private struct ReadingSettings: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        Form {
            Section(L10n.text("settings.text")) {
                LabeledContent(L10n.text("text.size")) {
                    Stepper(value: $model.settings.fontSize, in: ReaderSettings.fontSizes, step: 1) {
                        Text(L10n.format("text.points", Int(model.settings.fontSize))).monospacedDigit()
                    }
                }
                Picker(L10n.text("text.stroke"), selection: $model.settings.gamma) {
                    ForEach(Stroke.all, id: \.gamma) { Text($0.name).tag($0.gamma) }
                }
                Picker(L10n.text("text.spread"), selection: $model.settings.spread) {
                    ForEach(Spread.allCases) { Text($0.title).tag($0) }
                }
                Picker(L10n.text("images.title"), selection: $model.settings.images) {
                    ForEach(ImageLight.allCases) { Text($0.title).tag($0) }
                }
            }
            Section(L10n.text("settings.light")) {
                LabeledContent(L10n.text("light.level")) {
                    Slider(value: $model.settings.level, in: ReaderSettings.levels)
                        .tint(model.accent)
                        .frame(width: 220)
                }
                Toggle(L10n.text("light.edr"), isOn: $model.settings.edr)
                Picker(L10n.text("light.tint"), selection: $model.settings.tint) {
                    ForEach(Tint.all.indices, id: \.self) { Text(Tint.all[$0].name).tag($0) }
                }
                Text(L10n.text("settings.light.note"))
                    .font(Theme.caption)
                    .foregroundStyle(Theme.secondary)
            }
        }
        .formStyle(.grouped)
        .frame(height: 400)
    }
}

/// Shortcut is a key and what it does.
struct Shortcut: Identifiable {
    let keys: String
    /// Action is the localization key of what the keys do.
    let action: String

    var id: String { action }

    /// The keys the reader handles while a book has focus. Keys are shown
    /// with the symbols macOS menus use: ⇞ Page Up, ⇟ Page Down, ↖ Home,
    /// ↘ End, ⎋ Escape.
    static let reading = [
        Shortcut(keys: "→  \(L10n.text("key.space"))  ⇟", action: "shortcut.nextPage"),
        Shortcut(keys: "←  ⇧\(L10n.text("key.space"))  ⇞", action: "shortcut.previousPage"),
        Shortcut(keys: "↖  ↘", action: "shortcut.startEnd"),
        Shortcut(keys: "↑  ↓", action: "shortcut.light"),
        Shortcut(keys: "+  −", action: "shortcut.textSize"),
        Shortcut(keys: "E", action: "shortcut.edr"),
        Shortcut(keys: "C", action: "shortcut.tint"),
        Shortcut(keys: "T  ⌥⌘C", action: "shortcut.contents"),
        Shortcut(keys: "F  ⌃⌘F", action: "shortcut.fullScreen"),
        Shortcut(keys: "⎋", action: "shortcut.exitFullScreen"),
    ]

    /// The menu commands, which work anywhere in the window.
    static let window = [
        Shortcut(keys: "⌘O", action: "shortcut.addBooks"),
        Shortcut(keys: "⌘W", action: "shortcut.closeBook"),
        Shortcut(keys: "⌘1", action: "shortcut.library"),
        Shortcut(keys: "⌘2 … ⌘9", action: "shortcut.tabs"),
        Shortcut(keys: "⌃⇥  ⌃⇧⇥", action: "shortcut.cycleTabs"),
        Shortcut(keys: "⌃⌘S", action: "shortcut.sidebar"),
        Shortcut(keys: "⌥⌘↑  ⌥⌘↓", action: "shortcut.light"),
        Shortcut(keys: "⌘,", action: "shortcut.settings"),
    ]
}

@MainActor
private struct ShortcutsSettings: View {
    var body: some View {
        Form {
            group(L10n.text("settings.shortcuts.reading"), Shortcut.reading)
            group(L10n.text("settings.shortcuts.window"), Shortcut.window)
        }
        .formStyle(.grouped)
        .frame(height: 540)
    }

    private func group(_ title: String, _ shortcuts: [Shortcut]) -> some View {
        Section(title) {
            ForEach(shortcuts) { shortcut in
                LabeledContent(L10n.text(shortcut.action)) {
                    Text(shortcut.keys).font(.system(size: 12, design: .rounded)).foregroundStyle(Theme.secondary)
                }
            }
        }
    }
}

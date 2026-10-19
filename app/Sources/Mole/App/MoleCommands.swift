import SwiftUI

/// MenuState is what the menus show of the main window. The window publishes
/// it as a focused scene value: SwiftUI rebuilds the menus when a focused
/// value changes, which it does not do for the model's own changes.
struct MenuState: Equatable {
    var hasBook = false
    /// BookTabs are the titles of the open books, in tab order.
    var bookTabs: [String] = []
    var showSidebar = true
    var showContents = true
    var isFullScreen = false
    var spread = Spread.automatic
    var edr = true
    var tint = 0
    var hasSelection = false
    var isBookmarked = false
    var hasHits = false

    @MainActor
    init(_ model: AppModel) {
        hasBook = model.activeTab != nil
        bookTabs = model.tabs.map(\.book.title)
        showSidebar = model.showSidebar
        showContents = model.showContents
        isFullScreen = model.isFullScreen
        spread = model.settings.spread
        edr = model.settings.edr
        tint = model.settings.tint
        hasSelection = model.activeTab?.selection != nil
        isBookmarked = model.activeTab?.isBookmarked ?? false
        hasHits = !(model.activeTab?.hits.isEmpty ?? true)
    }
}

private struct MenuStateKey: FocusedValueKey {
    typealias Value = MenuState
}

extension FocusedValues {
    var menuState: MenuState? {
        get { self[MenuStateKey.self] }
        set { self[MenuStateKey.self] = newValue }
    }
}

/// MoleCommands are the menu bar commands. The ones that only make sense
/// while reading are disabled elsewhere, so their keys reach the focused
/// control instead.
///
/// The Commands protocol of the macOS 14 SDK is not isolated to the main
/// actor, while the model is; menu actions run on the main thread, so they
/// reach the model through perform.
struct MoleCommands: Commands {
    let model: AppModel?
    @FocusedValue(\.menuState) private var state

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button(L10n.text("menu.addBooks"), action: perform { $0.chooseBooks() })
                .keyboardShortcut("o")
                .disabled(model == nil)
            Button(L10n.text("menu.closeBook"), action: perform { $0.closeActiveTab() })
                .keyboardShortcut("w")
                .disabled(state?.hasBook != true)
        }
        CommandGroup(replacing: .sidebar) {
            Button(state?.showSidebar == false ? L10n.text("sidebar.show") : L10n.text("sidebar.hide"),
                   action: perform { model in withAnimation(.easeInOut(duration: 0.2)) { model.showSidebar.toggle() } })
                .keyboardShortcut("s", modifiers: [.control, .command])
                .disabled(state == nil)
            Button(state?.showContents == true ? L10n.text("contents.hide") : L10n.text("contents.show"),
                   action: perform { model in withAnimation(.easeInOut(duration: 0.2)) { model.showContents.toggle() } })
                .keyboardShortcut("c", modifiers: [.option, .command])
                .disabled(state?.hasBook != true)
            Button(state?.isFullScreen == true ? L10n.text("menu.exitFullScreen") : L10n.text("menu.enterFullScreen"),
                   action: perform { $0.toggleFullScreen() })
                .keyboardShortcut("f", modifiers: [.control, .command])
                .disabled(state == nil)
            Divider()
            Button(L10n.text("text.bigger"), action: perform { $0.adjustFontSize(by: 1) })
                .keyboardShortcut("+")
                .disabled(state == nil)
            Button(L10n.text("text.smaller"), action: perform { $0.adjustFontSize(by: -1) })
                .keyboardShortcut("-")
                .disabled(state == nil)
            Picker(L10n.text("text.spread"), selection: Binding {
                state?.spread ?? .automatic
            } set: { spread in
                perform { $0.settings.spread = spread }()
            }) {
                ForEach(Spread.allCases) { Text($0.title).tag($0) }
            }
            .disabled(state == nil)
            Divider()
        }
        CommandGroup(after: .textEditing) {
            Divider()
            Button(L10n.text("menu.find"), action: perform { $0.beginSearch() })
                .keyboardShortcut("f")
                .disabled(state?.hasBook != true)
            Button(L10n.text("menu.findNext"), action: perform { $0.activeTab?.showHit(step: 1) })
                .keyboardShortcut("g")
                .disabled(state?.hasHits != true)
            Button(L10n.text("menu.findPrevious"), action: perform { $0.activeTab?.showHit(step: -1) })
                .keyboardShortcut("g", modifiers: [.command, .shift])
                .disabled(state?.hasHits != true)
        }
        CommandMenu(L10n.text("menu.book")) {
            Button(state?.isBookmarked == true ? L10n.text("bookmark.remove") : L10n.text("bookmark.add"),
                   action: perform { $0.toggleBookmark() })
                .keyboardShortcut("d")
                .disabled(state?.hasBook != true)
            Button(L10n.text("highlight.selection"), action: perform { $0.highlightSelection() })
                .keyboardShortcut("h", modifiers: [.command, .shift])
                .disabled(state?.hasSelection != true)
            Button(L10n.text("lookup.action"), action: perform { $0.activeTab?.lookUpSelection() })
                .keyboardShortcut("d", modifiers: [.control, .command])
                .disabled(state?.hasSelection != true)
            Button(L10n.text("translate.action"), action: perform { $0.activeTab?.translateSelection() })
                .disabled(state?.hasSelection != true || !Self.canTranslate)
            Divider()
            ForEach(PanelPage.allCases) { page in
                Button(page.title, action: perform { $0.showPanel(page) })
                    .disabled(state?.hasBook != true)
            }
        }
        CommandMenu(L10n.text("menu.go")) {
            Button(L10n.text("tab.library"), action: perform { $0.showTab(0) })
                .keyboardShortcut("1")
                .disabled(state == nil)
            ForEach(Array((state?.bookTabs ?? []).prefix(8).enumerated()), id: \.offset) { index, title in
                Button(title, action: perform { $0.showTab(index + 1) })
                    .keyboardShortcut(KeyEquivalent(Character("\(index + 2)")))
            }
            Divider()
            Button(L10n.text("menu.nextTab"), action: perform { $0.cycleTabs(1) })
                .keyboardShortcut(.tab, modifiers: .control)
                .disabled(state?.bookTabs.isEmpty ?? true)
            Button(L10n.text("menu.previousTab"), action: perform { $0.cycleTabs(-1) })
                .keyboardShortcut(.tab, modifiers: [.control, .shift])
                .disabled(state?.bookTabs.isEmpty ?? true)
            Divider()
            Button(L10n.text("menu.start"), action: perform { $0.activeTab?.goToStart() })
                .disabled(state?.hasBook != true)
            Button(L10n.text("menu.end"), action: perform { $0.activeTab?.goToEnd() })
                .disabled(state?.hasBook != true)
        }
        CommandMenu(L10n.text("menu.light")) {
            Button(L10n.text("menu.brighter"), action: perform { $0.adjustLevel(by: 1.25) })
                .keyboardShortcut(.upArrow, modifiers: [.option, .command])
                .disabled(state == nil)
            Button(L10n.text("menu.dimmer"), action: perform { $0.adjustLevel(by: 0.8) })
                .keyboardShortcut(.downArrow, modifiers: [.option, .command])
                .disabled(state == nil)
            Divider()
            Toggle(L10n.text("light.edr"), isOn: Binding {
                state?.edr ?? true
            } set: { edr in
                perform { $0.settings.edr = edr }()
            })
            .disabled(state == nil)
            Picker(L10n.text("light.tint"), selection: Binding {
                state?.tint ?? 0
            } set: { tint in
                perform { $0.settings.tint = tint }()
            }) {
                ForEach(Tint.all.indices, id: \.self) { Text(Tint.all[$0].name).tag($0) }
            }
            .disabled(state == nil)
        }
        CommandGroup(replacing: .help) {
            Button(L10n.text("menu.shortcuts"), action: perform { _ in
                UserDefaults.standard.set(SettingsTab.shortcuts.rawValue, forKey: SettingsTab.storageKey)
                SettingsTab.openWindow()
            })
            Divider()
            Button(L10n.text("menu.notices")) {
                // The bundle carries the notices; a bare build (swift run) has none.
                if let url = Bundle.main.url(forResource: "THIRD_PARTY_NOTICES", withExtension: "txt") {
                    NSWorkspace.shared.open(url)
                }
            }
            Button(L10n.text("menu.source")) {
                if let url = URL(string: "https://github.com/pedrohavay/mole") { NSWorkspace.shared.open(url) }
            }
        }
    }

    /// CanTranslate reports whether this macOS has the Translation framework.
    private static var canTranslate: Bool {
        if #available(macOS 14.4, *) { return true }
        return false
    }

    /// Returns a menu action that runs body with the model.
    private func perform(_ body: @escaping @MainActor (AppModel) -> Void) -> () -> Void {
        { [model] in
            MainActor.assumeIsolated {
                if let model { body(model) }
            }
        }
    }
}

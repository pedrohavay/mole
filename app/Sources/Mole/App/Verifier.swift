import AppKit
import ApplicationServices
import SwiftUI

/// Verifier is a development aid, run with `--verify-dir dir`: it captures
/// the library, checks that the title bar row takes clicks, opens the first
/// book, turns some pages, walks the accessibility tree, reads the end of a
/// screen aloud, captures the reader, the book's colors, its pictures in
/// each mode, the picture viewer and the Settings window, and quits. With
/// `--verify-folder tree` it checks folder imports and collections instead
/// (see verifyFolders). It reports to standard error. Captures are 8-bit,
/// so EDR values above SDR white come out clipped.
@MainActor
struct Verifier {
    let model: AppModel
    let directory: String
    var folderTree: String?

    func start() {
        NSApp.activate(ignoringOtherApps: true)
        if let folderTree {
            return after(2.0) { verifyFolders(folderTree) }
        }
        after(2.5) {
            model.selectedBook = model.books.first?.id
            after(0.4) {
                capture("library.png")
                reportChrome()
                report("headroom with the library open: \(decimal(model.headroom.current, 2))x of \(decimal(model.headroom.potential, 0))x, active \(NSApp.isActive)")
                walkTree("ax-library.txt") { nodes in
                    let title = model.books.first?.title ?? "-"
                    for node in nodes where node.label.hasPrefix(title) || node.role == kAXScrollAreaRole
                        || node.selected {
                        report("a11y library: \(node.line.trimmingCharacters(in: .whitespaces))")
                    }
                    guard let book = model.books.first else { return NSApp.terminate(nil) }
                    model.open(book)
                    after(2.0) { readBook() }
                }
            }
        }
    }

    private func readBook() {
        if let tab = model.activeTab {
            report("opened at page \(tab.state.page + 1) of \(tab.state.pages)")
        }
        reportButtons("with a book open")
        if let window = mainWindow {
            window.setFrame(window.frame.insetBy(dx: 30, dy: 20), display: true)
            after(0.3) { reportButtons("after a resize") }
            after(0.6) {
                window.zoom(nil)
                after(0.5) {
                    reportButtons("after zoom")
                    window.zoom(nil)
                    after(0.5) { reportButtons("after unzoom") }
                }
            }
        }
        model.activeTab?.turn(10)
        after(0.8) {
            capture("reader.png")
            if let tab = model.activeTab {
                report("reading \(tab.state.page + 1)-\(tab.state.lastVisible + 1) of \(tab.state.pages), chapter \(tab.currentChapter?.title ?? "-"), gain \(decimal(model.gain, 2))")
            }
            // The window stops resizing first.
            after(1.2) { checkAccessibility { checkReadAloud { annotate() } } }
        }
    }

    // MARK: Accessibility

    /// Walks the app's accessibility tree through the AX API, as VoiceOver
    /// does, from a background thread so that the main thread can answer.
    /// It writes the tree to name in the directory, reports the controls
    /// without a label, and hands the elements to next on the main thread.
    private func walkTree(_ name: String, then next: @escaping @MainActor ([AXProbe.Node]) -> Void) {
        guard AXIsProcessTrusted() else {
            report("a11y: not trusted for the AX API, so \(name) was not written")
            return next([])
        }
        let pid = getpid()
        DispatchQueue.global().async {
            let app = AXUIElementCreateApplication(pid)
            // SwiftUI makes its elements as a client first asks for them.
            _ = AXProbe.walk(app)
            Thread.sleep(forTimeInterval: 0.5)
            let nodes = AXProbe.walk(app)
            DispatchQueue.main.async {
                MainActor.assumeIsolated {
                    let path = (directory as NSString).appendingPathComponent(name)
                    try? nodes.map(\.line).joined(separator: "\n").write(toFile: path, atomically: true, encoding: .utf8)
                    var roles: [String: Int] = [:]
                    for node in nodes { roles[node.role, default: 0] += 1 }
                    report("a11y: \(nodes.count) elements in \(path): \(roles.sorted { $0.value > $1.value }.prefix(8).map { "\($0.key) \($0.value)" }.joined(separator: ", "))")
                    // Window buttons and scroll bar parts have a subrole
                    // that VoiceOver reads instead of a label.
                    let controls: Set<String> = [kAXButtonRole, kAXRadioButtonRole, kAXCheckBoxRole, kAXPopUpButtonRole,
                                                 kAXMenuButtonRole, kAXSliderRole, kAXTextFieldRole, kAXImageRole]
                    let unlabeled = nodes.filter {
                        controls.contains($0.role) && $0.subrole.isEmpty && $0.label.isEmpty && $0.title.isEmpty
                    }
                    report("a11y: controls without a label: \(unlabeled.isEmpty ? "none" : unlabeled.map(\.line).joined(separator: "; "))")
                    next(nodes)
                }
            }
        }
    }

    /// Reports whether node is in the subtree of panel.
    private func isUnder(_ node: AXProbe.Node, panel: AXProbe.Node, in nodes: [AXProbe.Node]) -> Bool {
        var parent = node.parent
        while let id = parent {
            if id == panel.id { return true }
            parent = nodes[id].parent
        }
        return false
    }

    /// Walks the tree with a book open, reports the page's paragraphs and
    /// the reader's controls, then turns the page with the scroll action
    /// through the AX API and back with the custom Previous Page action.
    private func checkAccessibility(then next: @escaping @MainActor () -> Void) {
        guard let tab = model.activeTab, let view = mainWindow?.contentView?.firstDescendant(of: EDRMetalView.self)
        else { return next() }
        walkTree("ax-reader.txt") { nodes in
            let groups = nodes.filter { $0.role == kAXGroupRole && !$0.label.isEmpty }
            report("a11y groups: \(groups.map { "\($0.label) (\($0.roleDescription))" }.joined(separator: "; "))")
            for node in nodes where node.role == kAXButtonRole && node.subrole.isEmpty && node.depth <= 4 {
                report("a11y control: \(node.line.trimmingCharacters(in: .whitespaces))")
            }
            guard let page = nodes.first(where: { $0.role == kAXGroupRole && $0.roleDescription == L10n.text("reader.role") })
            else {
                report("a11y: no page in the tree")
                return next()
            }
            let paragraphs = nodes.filter { $0.parent == page.id }
            report("a11y page: \(page.line.trimmingCharacters(in: .whitespaces))")
            report("a11y page: \(paragraphs.count) paragraphs on \(tab.pageDescription)")
            for (index, paragraph) in paragraphs.enumerated() where index < 3 || index == paragraphs.count - 1 {
                report("a11y paragraph \(index + 1): \(paragraph.line.trimmingCharacters(in: .whitespaces))")
            }
            report("a11y page actions: \(page.actions.joined(separator: ", "))")
            let before = tab.state.page
            DispatchQueue.global().async {
                let error = AXUIElementPerformAction(page.element, "AXScrollDownByPage" as CFString)
                DispatchQueue.main.async {
                    MainActor.assumeIsolated {
                        after(0.4) {
                            report("a11y scroll down by page: \(error == .success ? "done" : "error \(error.rawValue)"), page \(before + 1) -> \(tab.state.page + 1), announced \"\(tab.pageDescription)\", \(view.textElements().count) paragraphs")
                            let previous = view.accessibilityCustomActions()?.first { $0.name == L10n.text("reader.previousPage") }
                            _ = previous?.handler?()
                            after(0.4) {
                                report("a11y custom action \(previous?.name ?? "-"): page \(tab.state.page + 1)")
                                next()
                            }
                        }
                    }
                }
            }
        }
    }

    // MARK: Reading aloud

    /// Reads aloud from the paragraph before last on the screen, silently and
    /// fast, until the speech turns the page and goes on for two sentences;
    /// then Esc stops it. It reports each sentence and captures the mark.
    private func checkReadAloud(then next: @escaping @MainActor () -> Void) {
        guard let tab = model.activeTab, let view = mainWindow?.contentView?.firstDescendant(of: EDRMetalView.self)
        else { return next() }
        let reader = model.readAloud
        let paragraphs = tab.visibleParagraphs()
        guard !paragraphs.isEmpty else { return next() }
        let rate = reader.rate
        reader.volume = 0
        reader.rate = ReadAloud.rates.upperBound
        let startPage = tab.state.page
        let started = Date()
        reader.start(tab, from: paragraphs[max(0, paragraphs.count - 2)].range.start)
        let declared = ((try? tab.session.language()) ?? nil) ?? "none"
        report("read aloud: \(reader.state) from page \(startPage + 1), language \(tab.language ?? "-") (declared \(declared)), voice \(ReadAloud.voice(for: tab.language, picked: reader.voiceIdentifier)?.name ?? "system")")
        var last: ReadAloud.Sentence?
        var turnedAt: Int?
        var sentences = 0
        func poll() {
            if let sentence = reader.sentence, sentence != last {
                last = sentence
                sentences += 1
                report("read aloud: said \"\(ReaderTab.excerpt(sentence.text, limit: 70))\" on page \(tab.state.page + 1), marks \(tab.marks.filter { $0.kind == .speech }.count)")
                if sentences == 1 { after(0.3) { capture("readaloud.png") } }
            }
            if turnedAt == nil, tab.state.page != startPage {
                turnedAt = sentences
                report("read aloud: turned to page \(tab.state.page + 1) after \(decimal(Date().timeIntervalSince(started), 1)) s")
                after(0.2) { capture("readaloud-turned.png") }
            }
            let done = turnedAt.map { sentences >= $0 + 2 } ?? false
            guard done || !reader.isActive || Date().timeIntervalSince(started) > 60 else { return after(0.1, poll) }
            if let escape = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0,
                                             windowNumber: view.window?.windowNumber ?? 0, context: nil,
                                             characters: "\u{1B}", charactersIgnoringModifiers: "\u{1B}",
                                             isARepeat: false, keyCode: KeyCode.escape) {
                view.keyDown(with: escape)
            }
            report("read aloud: \(sentences) sentences, page turned \(turnedAt != nil); after Esc \(reader.state), speech marks \(tab.marks.filter { $0.kind == .speech }.count)")
            reader.rate = rate
            reader.volume = 1
            next()
        }
        poll()
    }

    /// Selects a passage, highlights it, bookmarks the page and searches the
    /// book, reporting each step and capturing the result.
    private func annotate() {
        guard let tab = model.activeTab, let window = mainWindow,
              let view = window.contentView?.firstDescendant(of: EDRMetalView.self) else { return finish() }
        let size = view.bounds.size
        for point in [CGPoint(x: size.width * 0.3, y: size.height * 0.35), CGPoint(x: size.width / 2, y: size.height / 2)] {
            do {
                let anchor = try tab.session.anchor(at: point)
                report("hit test at \(Int(point.x)),\(Int(point.y)) of \(Int(size.width))x\(Int(size.height)): \(anchor.map { "\($0.chapter):\($0.offset)" } ?? "no text")")
            } catch {
                report("hit test failed: \(error.localizedDescription)")
            }
        }
        // A drag from a third of the way in, across a few lines.
        tab.beginSelection(at: CGPoint(x: size.width * 0.3, y: size.height * 0.35))
        for step in 1...8 {
            tab.extendSelection(to: CGPoint(x: size.width * 0.6, y: size.height * (0.35 + Double(step) * 0.01)))
        }
        report("selected \(tab.selectedText()?.count ?? 0) characters: \(ReaderTab.excerpt(tab.selectedText() ?? "", limit: 60))")
        after(0.5) {
            capture("selection.png")
            model.highlightSelection(color: 2)
            model.toggleBookmark()
            report("highlights \(tab.highlights.count), bookmarks \(tab.bookmarks.count), bookmarked \(tab.isBookmarked), marks \(tab.marks.count)")
            lookUp(tab, in: view)
        }
    }

    /// Selects a word by its position, opens the dictionary and the
    /// translation for it, and reports the reading time left.
    private func lookUp(_ tab: ReaderTab, in view: EDRMetalView) {
        let size = view.bounds.size
        let found = tab.selectWord(at: CGPoint(x: size.width * 0.45, y: size.height * 0.5))
        report("word at the middle of the page: \(found ? tab.selectedText() ?? "" : "none")")
        if let left = tab.timeLeft {
            report("time left: chapter \(left.chapter.map { TimeLeft.format($0) } ?? "-"), book \(TimeLeft.format(left.book))")
        }
        let before = Set(NSApp.windows.map(ObjectIdentifier.init))
        tab.lookUpSelection()
        after(1.5) {
            let shown = NSApp.windows.filter { $0.isVisible && !before.contains(ObjectIdentifier($0)) }
            report("look up opened \(shown.map { String(describing: type(of: $0)) })")
            if let panel = shown.first(where: { String(describing: type(of: $0)).contains("Popover") }) ?? shown.last {
                capture("lookup.png", window: panel)
            }
            shown.forEach { $0.orderOut(nil) }
            tab.translateSelection()
            after(1.5) {
                let translation = NSApp.windows.filter { $0.isVisible && !before.contains(ObjectIdentifier($0)) }
                report("translation opened \(translation.map { String(describing: type(of: $0)) })")
                if let window = translation.first { capture("translation.png", window: window) }
                tab.translationText = nil
                tab.clearSelection()
                model.showPanel(.search)
                tab.find("pipeline")
                waitForSearch(tab, started: Date())
            }
        }
    }

    private func waitForSearch(_ tab: ReaderTab, started: Date) {
        guard tab.searchDone || Date().timeIntervalSince(started) > 20 else {
            return after(0.2) { waitForSearch(tab, started: started) }
        }
        report("search for \"pipeline\": \(tab.hits.count) hits\(tab.searchTruncated ? " (truncated)" : "") in \(decimal(Date().timeIntervalSince(started), 2)) s")
        tab.showHit(step: 1)
        after(0.8) {
            capture("search.png")
            report("showing hit \((tab.currentHit ?? -1) + 1) at page \(tab.state.page + 1), marks \(tab.marks.count)")
            model.showPanel(.notes)
            after(0.5) {
                capture("notes.png")
                walkTree("ax-notes.txt") { nodes in
                    let panel = nodes.first { $0.label == PanelPage.notes.title && $0.role == kAXGroupRole }
                    for node in nodes where node.parent != nil && panel.map({ isUnder(node, panel: $0, in: nodes) }) == true {
                        report("a11y notes: \(node.line.trimmingCharacters(in: .whitespaces))")
                    }
                    pictures()
                }
            }
        }
    }

    /// Captures the first screen with colored text, with and without the
    /// book's colors, then screens with pictures, the last one in every
    /// picture mode, and opens a picture in the viewer.
    private func pictures() {
        guard let tab = model.activeTab else { return finish() }
        model.showContents = false
        model.settings.bookColors = true
        tab.go(to: 0)
        var screens = 0
        while (try? tab.session.setStyle(pictures: .hidden, bookColors: true)) != nil,
              (try? tab.session.render()) != nil, (try? tab.session.textColors()) == nil, screens < 300 {
            tab.turn(1)
            screens += 1
        }
        report("colored text on page \(tab.state.page + 1)")
        after(0.8) {
            capture("colors.png")
            model.settings.bookColors = false
            after(0.6) {
                capture("colors-off.png")
                seekPicture(tab, from: 0, name: "pictures.png") {
                    seekPicture(tab, from: 40, name: "pictures-2.png") {
                        seekPicture(tab, from: 150, name: "pictures-3.png") { pictureModes(tab) }
                    }
                }
            }
        }
    }

    /// Shows the first screen from page from that has a picture, captures
    /// it into name and goes on with next.
    private func seekPicture(_ tab: ReaderTab, from page: Int, name: String, next: @escaping @MainActor () -> Void) {
        tab.go(to: page)
        var screens = 0
        while (try? tab.session.render()) != nil, (try? tab.session.imageLayer()) == nil, screens < 300 {
            tab.turn(1)
            screens += 1
        }
        report("picture on page \(tab.state.page + 1) for \(name)")
        after(0.8) {
            capture(name)
            next()
        }
    }

    private func pictureModes(_ tab: ReaderTab) {
        model.settings.pictures = .original
        after(0.6) {
            capture("pictures-original.png")
            model.settings.bookColors = true
            after(0.6) {
                capture("pictures-original-colors.png")
                model.settings.bookColors = false
                model.settings.pictures = .hidden
                after(0.6) {
                    capture("pictures-hidden.png")
                    model.settings.pictures = .night
                    after(0.6) { viewPicture(tab) }
                }
            }
        }
    }

    /// Clicks the picture on screen, zooms it with ⌘= and closes it with Esc,
    /// reporting each step and capturing the viewer.
    private func viewPicture(_ tab: ReaderTab) {
        guard let window = mainWindow, let view = window.contentView?.firstDescendant(of: EDRMetalView.self) else {
            return finish()
        }
        let size = view.bounds.size
        var points: [CGPoint] = []
        for x in 1..<16 {
            for y in 1..<16 { points.append(CGPoint(x: size.width * CGFloat(x) / 16, y: size.height * CGFloat(y) / 16)) }
        }
        guard let point = points.first(where: { tab.picture(at: $0) != nil }), let picture = tab.picture(at: point) else {
            report("no picture to click on page \(tab.state.page + 1)")
            return finish()
        }
        report("picture at \(Int(point.x)),\(Int(point.y)): page \(picture.page + 1), index \(picture.index), \(Int(picture.rect.width))x\(Int(picture.rect.height)) pt")
        // The keys below need the window to be key, which another app may
        // have taken since launch.
        NSApp.activate(ignoringOtherApps: true)
        window.makeKeyAndOrderFront(nil)
        view.onHover?(point)
        report("cursor over the picture: \(NSCursor.current === NSCursor.pointingHand ? "pointing hand" : "other")")
        click(at: point, in: view)
        after(1.0) {
            guard let viewer = window.contentView?.firstDescendant(of: PictureScrollView.self),
                  let image = (viewer.documentView as? NSImageView)?.image else {
                report("the picture viewer did not open")
                return finish()
            }
            let fontSize = model.settings.fontSize
            report("viewer open: \(Int(image.size.width))x\(Int(image.size.height)) px at \(decimal(viewer.magnification, 2))x, first responder \(window.firstResponder.map { String(describing: type(of: $0)) } ?? "none"), key \(window.isKeyWindow)")
            capture("viewer.png")
            press("=", keyCode: 24, modifiers: .command, in: window)
            press("=", keyCode: 24, modifiers: .command, in: window)
            after(0.6) {
                report("after ⌘= twice: \(decimal(viewer.magnification, 2))x, text size \(Int(model.settings.fontSize)) (was \(Int(fontSize)))")
                capture("viewer-zoom.png")
                press("0", keyCode: 29, modifiers: .command, in: window)
                after(0.6) {
                    report("after ⌘0: \(decimal(viewer.magnification, 2))x")
                    press("\u{1b}", keyCode: KeyCode.escape, modifiers: [], in: window)
                    after(0.6) {
                        let open = window.contentView?.firstDescendant(of: PictureScrollView.self) != nil
                        report("after Esc: viewer open \(open), page has the keys \(window.firstResponder === view)")
                        capture("text-panel.png", view: TextPanel()) {
                            capture("reading-settings.png", view: ReadingSettings().frame(width: 520)) { finish() }
                        }
                    }
                }
            }
        }
    }

    /// Captures view into name, in a window of its own with the app's
    /// environment, then goes on with next.
    private func capture<Content: View>(_ name: String, view: Content, next: @escaping @MainActor () -> Void) {
        let host = NSHostingView(rootView: view.environment(model).environment(\.accent, model.accent)
            .preferredColorScheme(.dark))
        let window = NSWindow(contentRect: NSRect(origin: .zero, size: host.fittingSize), styleMask: [.titled],
                              backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = host
        window.center()
        window.orderFront(nil)
        after(0.6) {
            capture(name, window: window)
            window.close()
            next()
        }
    }

    /// Clicks point of the page, in view points from the top left, as the
    /// page's own mouse handling sees it.
    private func click(at point: CGPoint, in view: EDRMetalView) {
        guard let window = view.window else { return }
        let location = view.convert(NSPoint(x: point.x, y: view.bounds.height - point.y), to: nil)
        for (type, step) in [(NSEvent.EventType.leftMouseDown, EDRMetalView.MouseStep.down), (.leftMouseUp, .up)] {
            if let event = NSEvent.mouseEvent(with: type, location: location, modifierFlags: [], timestamp: 0,
                                              windowNumber: window.windowNumber, context: nil, eventNumber: 0,
                                              clickCount: 1, pressure: 1) {
                view.onMouse?(step, point, event)
            }
        }
    }

    /// Presses a key in window through the app, menus included.
    private func press(_ key: String, keyCode: UInt16, modifiers: NSEvent.ModifierFlags, in window: NSWindow) {
        if let event = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: modifiers, timestamp: 0,
                                        windowNumber: window.windowNumber, context: nil, characters: key,
                                        charactersIgnoringModifiers: key, isARepeat: false, keyCode: keyCode) {
            NSApp.sendEvent(event)
        }
    }

    private func finish() {
        let main = mainWindow
        reportMenus()
        NSApp.orderFrontStandardAboutPanel(nil)
        after(0.8) {
            if let about = NSApp.windows.first(where: { $0.isVisible && $0 !== main && $0.contentView != nil }) {
                capture("about.png", window: about)
                about.close()
            }
            openSettings(main: main)
        }
    }

    private func openSettings(main: NSWindow?) {
        SettingsTab.openWindow()
        after(1.0) {
            let settings = NSApp.windows.first { $0.isVisible && $0 !== main && $0.contentView != nil }
            guard let settings else {
                report("the Settings window did not open")
                return NSApp.terminate(nil)
            }
            capture("settings.png", window: settings)
            // The Reading tab holds the voice and speed of reading aloud.
            let defaults = UserDefaults.standard
            let shown = defaults.string(forKey: SettingsTab.storageKey)
            defaults.set(SettingsTab.reading.rawValue, forKey: SettingsTab.storageKey)
            after(0.8) {
                capture("settings-reading.png", window: settings)
                defaults.set(shown, forKey: SettingsTab.storageKey)
                NSApp.terminate(nil)
            }
        }
    }

    /// Reports where the traffic lights are and which view takes a click in
    /// the tab strip, which must be the content, not the title bar.
    private func reportChrome() {
        guard let window = mainWindow, let frameView = window.contentView?.superview else { return }
        let height = window.frame.height
        reportButtons("at launch")
        report("title bar \(decimal(height - window.contentLayoutRect.maxY, 1)) pt")
        for y in [6.0, Theme.Size.bar / 2, Theme.Size.bar - 4] {
            let point = NSPoint(x: (model.showSidebar ? 252 : 120) + 40, y: height - y)
            let hit = frameView.hitTest(frameView.convert(point, from: nil))
            report("click \(decimal(y, 0)) pt down the tab strip goes to \(hit.map { String(describing: type(of: $0)) } ?? "nothing")")
        }
    }

    /// Reports where each window button is, from the top left of the window,
    /// and which view holds it; all three should share a center line.
    private func reportButtons(_ moment: String) {
        guard let window = mainWindow else { return }
        let height = window.frame.height
        let lines: [String] = [NSWindow.ButtonType.closeButton, .miniaturizeButton, .zoomButton].compactMap { kind in
            guard let button = window.standardWindowButton(kind), let superview = button.superview else { return nil }
            let frame = superview.convert(button.frame, to: nil)
            return "\(decimal(frame.midX, 1)),\(decimal(height - frame.midY, 1)) in \(type(of: superview))"
        }
        report("buttons \(moment): \(lines.joined(separator: " | "))")
    }

    /// Reports the menu bar, to check its language and key equivalents.
    private func reportMenus() {
        for item in NSApp.mainMenu?.items ?? [] {
            // Menus are brought up to date when they open; do what opening does.
            if let submenu = item.submenu {
                submenu.delegate?.menuNeedsUpdate?(submenu)
                submenu.update()
            }
            let entries = (item.submenu?.items ?? []).filter { !$0.isSeparatorItem }.map { entry in
                let action = entry.action.map { NSStringFromSelector($0) } ?? "-"
                let state = entry.isEnabled ? "" : " (off)"
                return entry.keyEquivalent.isEmpty ? "\(entry.title)\(state) <\(action)>"
                    : "\(entry.title) [\(entry.keyEquivalent)]\(state) <\(action)>"
            }
            report("menu \(item.title): \(entries.joined(separator: ", "))")
        }
    }

    var mainWindow: NSWindow? {
        NSApp.windows.first { $0.isVisible && $0.contentView != nil && $0.identifier?.rawValue.contains("main") == true }
            ?? NSApp.windows.first { $0.isVisible && $0.contentView != nil }
    }

    func after(_ seconds: Double, _ work: @escaping @MainActor () -> Void) {
        DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { MainActor.assumeIsolated(work) }
    }

    func report(_ message: String) {
        FileHandle.standardError.write(Data("verify: \(message)\n".utf8))
    }

    func capture(_ name: String, window: NSWindow? = nil) {
        guard let window = window ?? mainWindow,
              let image = (WindowListCapture() as WindowCapture).image(of: CGWindowID(window.windowNumber)),
              let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:]) else {
            report("cannot capture \(name)")
            return
        }
        let path = (directory as NSString).appendingPathComponent(name)
        do {
            try png.write(to: URL(fileURLWithPath: path))
            report(path)
        } catch {
            report("cannot write \(path): \(error.localizedDescription)")
        }
    }
}

/// AXProbe walks a process's accessibility tree through the AX API, as
/// assistive apps see it. It must not run on the main thread of the process
/// it walks.
private enum AXProbe {
    /// Node is an element of the tree and what VoiceOver would read of it.
    struct Node: @unchecked Sendable {
        let element: AXUIElement
        let id: Int
        let parent: Int?
        let depth: Int
        let role: String
        let subrole: String
        let roleDescription: String
        /// Label is AXDescription, which NSAccessibility calls the label.
        let label: String
        let title: String
        let value: String
        let selected: Bool
        let frame: CGRect
        let actions: [String]

        var line: String {
            var parts = [String(repeating: "  ", count: depth) + role]
            if !label.isEmpty { parts.append("label \"\(label)\"") }
            if !title.isEmpty { parts.append("title \"\(title)\"") }
            if !value.isEmpty { parts.append("value \"\(value.count > 60 ? value.prefix(60) + "…" : value)\"") }
            if selected { parts.append("selected") }
            parts.append("(\(roleDescription)) at \(Int(frame.minX)),\(Int(frame.minY)) \(Int(frame.width))x\(Int(frame.height))")
            let extra = actions.filter { !["AXPress", "AXShowMenu", "AXScrollToVisible"].contains($0) }
            if !extra.isEmpty { parts.append("actions \(extra.map { $0.replacingOccurrences(of: "\n", with: " ") })") }
            return parts.joined(separator: " ")
        }
    }

    /// Returns the elements under root, depth first, up to a limit.
    static func walk(_ root: AXUIElement, limit: Int = 4000) -> [Node] {
        var nodes: [Node] = []
        var stack: [(element: AXUIElement, parent: Int?, depth: Int)] = [(root, nil, 0)]
        while let (element, parent, depth) = stack.popLast(), nodes.count < limit {
            let id = nodes.count
            var actions: CFArray?
            AXUIElementCopyActionNames(element, &actions)
            nodes.append(Node(element: element, id: id, parent: parent, depth: depth,
                              role: string(kAXRoleAttribute, of: element),
                              subrole: string(kAXSubroleAttribute, of: element),
                              roleDescription: string(kAXRoleDescriptionAttribute, of: element),
                              label: string(kAXDescriptionAttribute, of: element),
                              title: string(kAXTitleAttribute, of: element),
                              value: string(kAXValueAttribute, of: element),
                              selected: attribute(kAXSelectedAttribute, of: element) as? Bool ?? false,
                              frame: frame(of: element), actions: actions as? [String] ?? []))
            let children = attribute(kAXChildrenAttribute, of: element) as? [AXUIElement] ?? []
            stack += children.reversed().map { ($0, id, depth + 1) }
        }
        return nodes
    }

    private static func attribute(_ name: String, of element: AXUIElement) -> AnyObject? {
        var value: AnyObject?
        return AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success ? value : nil
    }

    private static func string(_ name: String, of element: AXUIElement) -> String {
        guard let value = attribute(name, of: element) else { return "" }
        return value as? String ?? (value as? NSNumber)?.stringValue ?? ""
    }

    private static func frame(of element: AXUIElement) -> CGRect {
        var origin = CGPoint.zero
        var size = CGSize.zero
        if let value = attribute(kAXPositionAttribute, of: element), CFGetTypeID(value) == AXValueGetTypeID() {
            AXValueGetValue(value as! AXValue, .cgPoint, &origin)
        }
        if let value = attribute(kAXSizeAttribute, of: element), CFGetTypeID(value) == AXValueGetTypeID() {
            AXValueGetValue(value as! AXValue, .cgSize, &size)
        }
        return CGRect(origin: origin, size: size)
    }
}

/// WindowCapture takes an image of a window, Metal layers included.
private protocol WindowCapture {
    func image(of window: CGWindowID) -> CGImage?
}

/// WindowListCapture uses CGWindowListCreateImage, deprecated in favor of
/// ScreenCaptureKit, which needs the screen recording permission even for
/// the app's own windows. It is called through WindowCapture so the
/// deprecation stays confined here.
private struct WindowListCapture: WindowCapture {
    @available(macOS, deprecated: 14.0)
    func image(of window: CGWindowID) -> CGImage? {
        CGWindowListCreateImage(.null, .optionIncludingWindow, window, [.boundsIgnoreFraming, .bestResolution])
    }
}

extension NSView {
    /// Returns the first view of type T under this one, depth first.
    func firstDescendant<T: NSView>(of type: T.Type) -> T? {
        for subview in subviews {
            if let match = subview as? T ?? subview.firstDescendant(of: type) { return match }
        }
        return nil
    }
}

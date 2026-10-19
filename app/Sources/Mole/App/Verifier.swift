import AppKit
import SwiftUI

/// Verifier is a development aid, run with `--verify-dir dir`: it captures
/// the library, checks that the title bar row takes clicks, opens the first
/// book, turns some pages, captures the reader in several fonts, margins and
/// spacings, the Text popover and the Settings window, and quits. It reports
/// to standard error. Captures are 8-bit, so EDR values above SDR white come
/// out clipped.
@MainActor
struct Verifier {
    let model: AppModel
    let directory: String

    func start() {
        NSApp.activate(ignoringOtherApps: true)
        after(2.5) {
            model.selectedBook = model.books.first?.id
            after(0.4) {
                capture("library.png")
                reportChrome()
                report("headroom with the library open: \(decimal(model.headroom.current, 2))x of \(decimal(model.headroom.potential, 0))x, active \(NSApp.isActive)")
                guard let book = model.books.first else { return NSApp.terminate(nil) }
                model.open(book)
                after(2.0) { readBook() }
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
            annotate()
        }
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
            model.showPanel(.search)
            tab.find("pipeline")
            waitForSearch(tab, started: Date())
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
                typography()
            }
        }
    }

    /// Look is a way to set the text that the typography check captures.
    private struct Look {
        let name: String
        let change: (inout ReaderSettings) -> Void
    }

    /// Shows a search hit well into the book and highlights a passage near
    /// the top of its screen, then sets the text in other fonts, margins and
    /// spacings,
    /// capturing each look of the page. For each it reports whether the
    /// reader stayed at the same text and whether the highlight and the hit
    /// still lie over their text. The settings are put back afterwards.
    private func typography() {
        guard let tab = model.activeTab, let window = mainWindow,
              let view = window.contentView?.firstDescendant(of: EDRMetalView.self) else { return finish() }
        let original = model.settings
        let showedContents = model.showContents
        // A page of running text: a hit well into the book, past the
        // table of contents.
        tab.showHit(tab.hits.count / 2)
        model.showContents = false
        after(1.0) {
            capture("typography-start.png")
            // The highlight starts the screen, so it stays on the screen
            // that holds the reader's place in every look.
            guard let start = tab.visibleRange?.start,
                  let first = try? tab.session.rects(of: TextRange(start: start, end: Anchor(chapter: start.chapter, offset: start.offset + 1))).first
            else { return finish() }
            // Showing the hit left the reader's place at the hit; the place
            // is the start of the screen instead, as after turning a page.
            tab.go(to: start)
            tab.beginSelection(at: CGPoint(x: first.midX, y: first.midY))
            tab.extendSelection(to: CGPoint(x: first.midX + 180, y: first.midY))
            let highlight = try? tab.highlightSelection(color: 3)
            report("typography: highlighted \(highlight.map { ReaderTab.excerpt($0.text, limit: 40) } ?? "nothing")")
            let looks = [
                Look(name: "font-book") { $0.font = "" },
                Look(name: "font-georgia") { $0.font = "Georgia" },
                Look(name: "font-avenir-next") { $0.font = "Avenir Next" },
                Look(name: "font-palatino") { $0.font = "Palatino" },
                Look(name: "font-iowan") { $0.font = "Iowan Old Style" },
                Look(name: "margins-narrow") { $0.font = "Georgia"; $0.margins = .narrow },
                Look(name: "margins-wide-relaxed") { $0.font = "Georgia"; $0.margins = .wide; $0.lineSpacing = .relaxed },
                Look(name: "justified") { $0.font = "Charter"; $0.justify = true },
            ]
            showLooks(looks[...], tab: tab, start: start, highlight: highlight) {
                showTextPanel(over: view) {
                    model.settings = original
                    model.showContents = showedContents
                    after(0.8) { finish() }
                }
            }
        }
    }

    private func showLooks(_ looks: ArraySlice<Look>, tab: ReaderTab, start: Anchor, highlight: Annotation?,
                           then done: @escaping @MainActor () -> Void) {
        guard let look = looks.first else { return done() }
        var settings = model.settings
        settings.font = ""
        settings.lineSpacing = .normal
        settings.margins = .normal
        settings.justify = false
        look.change(&settings)
        model.settings = settings
        after(2.0) {
            capture("\(look.name).png")
            let hit = tab.currentHit.map { tab.hits[$0].range }
            let place = tab.visibleRange?.contains(start) == true ? "stays at the same text" : "LOST its place"
            report("\(look.name): pages \(tab.state.page + 1)-\(tab.state.lastVisible + 1) of \(tab.state.pages), \(place); "
                + "highlight \(linesUp(highlight?.range, in: tab)); search hit \(linesUp(hit, in: tab))")
            showLooks(looks.dropFirst(), tab: tab, start: start, highlight: highlight, then: done)
        }
    }

    /// Reports whether a range on screen is drawn over its text: a hit test
    /// at the middle of each of its boxes must land inside the range.
    private func linesUp(_ range: TextRange?, in tab: ReaderTab) -> String {
        guard let range else { return "missing" }
        guard let rects = try? tab.session.rects(of: range), !rects.isEmpty else { return "off screen" }
        let inside = rects.filter { rect in
            guard let anchor = (try? tab.session.anchor(at: CGPoint(x: rect.midX, y: rect.midY))) ?? nil else { return false }
            return range.contains(anchor)
        }
        return inside.count == rects.count ? "lines up (\(rects.count) boxes)" : "MISALIGNED (\(inside.count) of \(rects.count) boxes)"
    }

    /// Shows the Text popover over the bottom right of the page, as the
    /// status bar button does, and captures it.
    private func showTextPanel(over view: NSView, then done: @escaping @MainActor () -> Void) {
        let popover = NSPopover()
        popover.contentViewController = NSHostingController(rootView: TextPanel()
            .environment(model)
            .environment(\.accent, model.accent)
            .preferredColorScheme(.dark))
        popover.behavior = .applicationDefined
        let corner = NSRect(x: view.bounds.maxX - 30, y: view.bounds.minY, width: 1, height: 1)
        popover.show(relativeTo: corner, of: view, preferredEdge: view.isFlipped ? .maxY : .minY)
        after(0.8) {
            if let window = popover.contentViewController?.view.window {
                capture("text-panel.png", window: window)
            } else {
                report("the Text popover did not open")
            }
            popover.close()
            done()
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
        // The Reading tab holds the text settings; the tab the user last
        // chose is put back.
        let defaults = UserDefaults.standard
        let shownTab = defaults.string(forKey: SettingsTab.storageKey)
        defaults.set(SettingsTab.reading.rawValue, forKey: SettingsTab.storageKey)
        SettingsTab.openWindow()
        after(1.0) {
            let settings = NSApp.windows.first { $0.isVisible && $0 !== main && $0.contentView != nil }
            if let settings {
                capture("settings.png", window: settings)
            } else {
                report("the Settings window did not open")
            }
            defaults.set(shownTab, forKey: SettingsTab.storageKey)
            NSApp.terminate(nil)
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

    private var mainWindow: NSWindow? {
        NSApp.windows.first { $0.isVisible && $0.contentView != nil && $0.identifier?.rawValue.contains("main") == true }
            ?? NSApp.windows.first { $0.isVisible && $0.contentView != nil }
    }

    private func after(_ seconds: Double, _ work: @escaping @MainActor () -> Void) {
        DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { MainActor.assumeIsolated(work) }
    }

    private func report(_ message: String) {
        FileHandle.standardError.write(Data("verify: \(message)\n".utf8))
    }

    private func capture(_ name: String, window: NSWindow? = nil) {
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

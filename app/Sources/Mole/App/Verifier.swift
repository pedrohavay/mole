import AppKit

/// Verifier is a development aid, run with `--verify-dir dir`: it captures
/// the library, checks that the title bar row takes clicks, opens the first
/// book, turns some pages, captures the reader and the Settings window, and
/// quits. It reports to standard error. Captures are 8-bit, so EDR values
/// above SDR white come out clipped.
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
                finish()
            }
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
            if let settings {
                capture("settings.png", window: settings)
            } else {
                report("the Settings window did not open")
            }
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

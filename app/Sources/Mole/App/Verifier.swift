import AppKit

/// Verifier is a development aid, run with `--verify-dir dir`: it captures
/// the library, checks that the title bar row takes clicks, opens the first
/// book, turns some pages, captures the reader, the book's colors, its
/// pictures in each mode, the picture viewer and the Settings window, and
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
                pictures()
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
                        finish()
                    }
                }
            }
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

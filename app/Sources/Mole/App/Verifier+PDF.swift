import AppKit

/// The check of a book of fixed pages, run when the first book of the
/// library is one, such as the sample PDF of the core's testbook package.
/// It turns pages, zooms in with the + key and the menu, fits the width,
/// scrolls with Space and a trackpad scroll, searches, selects and
/// highlights a passage, reads aloud past the bottom of the view, shows the
/// pictures and drawings in each mode, and captures each step. A locked.pdf
/// and a long.pdf next to the book's original, as testbook writes them, are
/// imported too: the first must fail for its password, and the second is
/// opened deep into its pages to time the layout and a search.
extension Verifier {
    func verifyPDF() {
        guard let tab = model.activeTab, let view else { return finish() }
        NSApp.activate(ignoringOtherApps: true)
        mainWindow?.makeKeyAndOrderFront(nil)
        report("pdf: \(tab.book.title) by \(tab.book.author), \(tab.state.pages) pages, fixed \(tab.isFixedLayout), "
            + "columns \(tab.state.columns), \(viewport(tab)), cover \(model.library.cover(of: tab.book.id)?.count ?? 0) bytes")
        report("pdf: contents \(tab.chapters.map { "\($0.title)@\($0.page + 1)/\($0.level)" })")
        capture("pdf-open.png")
        tab.turn(1)
        tab.turn(1)
        after(0.8) {
            report("pdf: turned to page \(tab.state.page + 1)-\(tab.state.lastVisible + 1), chapter \(tab.currentChapter?.title ?? "-"), characters \(tab.state.characters)")
            capture("pdf-turned.png")
            // One page at a time, so that zooming fills the view.
            model.settings.spread = .single
            after(0.8) { zoomIn(tab, in: view) }
        }
    }

    private var view: EDRMetalView? { mainWindow?.contentView?.firstDescendant(of: EDRMetalView.self) }

    private func viewport(_ tab: ReaderTab) -> String {
        guard let v = tab.state.viewport else { return "no viewport" }
        return "zoom \(v.percent)% (\(v.fit)), fit page \(Int(v.fitPage * 100))%, fit width \(Int(v.fitWidth * 100))%, "
            + "scrolled \(Int(v.x)),\(Int(v.y)) of \(Int(v.maxX)),\(Int(v.maxY))"
    }

    /// Presses + in the page and ⌘+ through the menu, then fits the width
    /// and scrolls down with Space and a trackpad scroll.
    private func zoomIn(_ tab: ReaderTab, in view: EDRMetalView) {
        guard let window = view.window else { return finish() }
        let fontSize = model.settings.fontSize
        report("pdf: single page, \(viewport(tab))")
        capture("pdf-single.png")
        key("+", in: view)
        report("pdf: after +, \(viewport(tab)), text size \(Int(model.settings.fontSize)) (was \(Int(fontSize)))")
        report("pdf: window key \(window.isKeyWindow), app active \(NSApp.isActive)")
        menuItem(withKey: "+")
        after(0.8) {
            report("pdf: after the menu's ⌘+ item, \(viewport(tab))")
            capture("pdf-zoom.png")
            tab.zoom(to: .width)
            tab.pan(dx: 0, dy: -.greatestFiniteMagnitude)
            after(0.6) {
                report("pdf: fit width, \(viewport(tab))")
                capture("pdf-fit-width.png")
                scroll(by: -80, in: view)
                report("pdf: after a scroll of 80 points, page \(tab.state.page + 1), \(viewport(tab))")
                key(" ", keyCode: KeyCode.space, in: view)
                after(0.6) {
                    report("pdf: after Space, page \(tab.state.page + 1), \(viewport(tab))")
                    capture("pdf-scrolled.png")
                    let page = tab.state.page
                    for _ in 0..<4 { key(" ", keyCode: KeyCode.space, in: view) }
                    report("pdf: after Space four times more, page \(tab.state.page + 1) (was \(page + 1)), \(viewport(tab))")
                    tab.go(to: page)
                    after(0.4) { search(tab, in: view) }
                }
            }
        }
    }

    /// Searches the book at the width of the view and shows a hit low on its
    /// page, which must scroll into view.
    private func search(_ tab: ReaderTab, in view: EDRMetalView) {
        let query = "lantern number 3"
        model.showPanel(.search)
        tab.find(query)
        let started = Date()
        @MainActor func wait() {
            guard tab.searchDone || Date().timeIntervalSince(started) > 10 else { return after(0.2, wait) }
            report("pdf: search for \"\(query)\": \(tab.hits.count) hits in \(decimal(Date().timeIntervalSince(started), 2)) s")
            tab.pan(dx: 0, dy: -.greatestFiniteMagnitude)
            tab.showHit(step: 1)
            after(0.8) {
                let hits = tab.marks.filter { if case .hit = $0.kind { true } else { false } }
                report("pdf: hit \((tab.currentHit ?? -1) + 1) at page \(tab.state.page + 1), \(viewport(tab)), hit marks \(hits.map { "\(Int($0.rect.minX)),\(Int($0.rect.minY))" }) in a view \(Int(view.bounds.height)) high")
                capture("pdf-search.png")
                tab.find("")
                model.showContents = false
                after(0.6) { annotate(tab, in: view) }
            }
        }
        wait()
    }

    /// Drags over two lines of the text in view and highlights them.
    private func annotate(_ tab: ReaderTab, in view: EDRMetalView) {
        let paragraphs = tab.visibleParagraphs().filter { paragraph in
            paragraph.lines.contains { view.bounds.contains($0) && $0.minY > 60 }
        }
        guard let paragraph = paragraphs.dropFirst().first, let first = paragraph.lines.first(where: { view.bounds.contains($0) })
        else {
            report("pdf: no paragraph in view to select")
            return pictures(tab, in: view)
        }
        let start = CGPoint(x: first.minX + 2, y: first.midY)
        let end = CGPoint(x: first.midX, y: first.midY + first.height * 1.2)
        report("pdf: hit test at \(Int(start.x)),\(Int(start.y)): \((try? tab.session.anchor(at: start)).flatMap { $0 }.map { "\($0.chapter):\($0.offset)" } ?? "no text")")
        tab.beginSelection(at: start)
        tab.extendSelection(to: CGPoint(x: (start.x + end.x) / 2, y: start.y))
        tab.extendSelection(to: end)
        report("pdf: selected \"\(ReaderTab.excerpt(tab.selectedText() ?? "", limit: 90))\"")
        after(0.5) {
            capture("pdf-selection.png")
            model.highlightSelection(color: 1)
            model.toggleBookmark()
            report("pdf: highlights \(tab.highlights.count), bookmarks \(tab.bookmarks.count), bookmarked \(tab.isBookmarked), marks \(tab.marks.count)")
            after(0.5) {
                capture("pdf-highlight.png")
                readAloud(tab, in: view)
            }
        }
    }

    /// Reads aloud, silently and fast, from the last paragraph in view at
    /// the top of a page as wide as the view, until the speech has scrolled
    /// the page down.
    private func readAloud(_ tab: ReaderTab, in view: EDRMetalView) {
        let reader = model.readAloud
        tab.pan(dx: 0, dy: -.greatestFiniteMagnitude)
        let inView = tab.visibleParagraphs().filter { $0.lines.contains { view.bounds.contains($0) } }
        guard let from = inView.last else {
            report("pdf: nothing in view to read aloud")
            return pictures(tab, in: view)
        }
        let rate = reader.rate
        reader.volume = 0
        reader.rate = ReadAloud.rates.upperBound
        let startY = tab.state.viewport?.y ?? 0
        let startPage = tab.state.page
        let started = Date()
        reader.start(tab, from: from.range.start)
        report("pdf: read aloud from \"\(ReaderTab.excerpt(from.text, limit: 50))\", language \(tab.language ?? "-"), \(viewport(tab))")
        var captured = false
        @MainActor func poll() {
            let moved = tab.state.page != startPage || (tab.state.viewport?.y ?? 0) != startY
            if reader.sentence != nil, !captured {
                captured = true
                after(0.2) { capture("pdf-readaloud.png") }
            }
            guard moved || !reader.isActive || Date().timeIntervalSince(started) > 40 else { return after(0.1, poll) }
            let speech = tab.marks.filter { $0.kind == .speech }
            report("pdf: read aloud \"\(ReaderTab.excerpt(reader.sentence?.text ?? "", limit: 60))\", page \(tab.state.page + 1), \(viewport(tab)), speech marks \(speech.map { "\(Int($0.rect.minY))" }) after \(decimal(Date().timeIntervalSince(started), 1)) s")
            after(0.3) {
                capture("pdf-readaloud-scrolled.png")
                reader.stop()
                reader.rate = rate
                reader.volume = 1
                pictures(tab, in: view)
            }
        }
        poll()
    }

    /// Shows the page of figures and the scanned page in each picture mode.
    private func pictures(_ tab: ReaderTab, in view: EDRMetalView) {
        tab.zoom(to: .page)
        guard let figures = tab.chapters.first(where: { $0.title == "Figures" })?.page else {
            report("pdf: no Figures entry in the contents")
            return finish()
        }
        tab.go(to: figures)
        model.settings.spread = .automatic
        let modes: [PictureMode] = [.night, .original, .hidden]
        @MainActor func show(_ index: Int) {
            guard index < modes.count else {
                model.settings.pictures = .night
                return after(0.4) { viewPicture(tab, in: view) }
            }
            model.settings.pictures = modes[index]
            after(0.8) {
                report("pdf: pages \(tab.state.page + 1)-\(tab.state.lastVisible + 1) with pictures \(modes[index])")
                capture("pdf-pictures-\(modes[index]).png")
                show(index + 1)
            }
        }
        show(0)
    }

    /// Clicks the photograph of the figures page and closes the viewer.
    private func viewPicture(_ tab: ReaderTab, in view: EDRMetalView) {
        let size = view.bounds.size
        var points: [CGPoint] = []
        for x in 1..<24 {
            for y in 1..<24 { points.append(CGPoint(x: size.width * CGFloat(x) / 24, y: size.height * CGFloat(y) / 24)) }
        }
        guard let point = points.first(where: { tab.picture(at: $0) != nil }), let picture = tab.picture(at: point) else {
            report("pdf: no picture to click")
            return panels(tab)
        }
        click(at: point, in: view)
        after(1.0) {
            let viewer = view.window?.contentView?.firstDescendant(of: PictureScrollView.self)
            let image = (viewer?.documentView as? NSImageView)?.image
            report("pdf: picture on page \(picture.page + 1), viewer open \(viewer != nil), \(Int(image?.size.width ?? 0))x\(Int(image?.size.height ?? 0)) px")
            capture("pdf-viewer.png")
            if let window = view.window { press("\u{1b}", keyCode: KeyCode.escape, modifiers: [], in: window) }
            after(0.6) { panels(tab) }
        }
    }

    /// Captures the Text popover and the Reading settings as they are for a
    /// book of fixed pages, then imports the locked and the long PDF.
    private func panels(_ tab: ReaderTab) {
        capture("pdf-text-panel.png", view: TextPanel()) {
            capture("pdf-reading-settings.png", view: ReadingSettings().frame(width: 520)) {
                otherPDFs(tab)
            }
        }
    }

    /// Imports locked.pdf, which must fail for its password, and opens
    /// long.pdf deep into its pages.
    private func otherPDFs(_ tab: ReaderTab) {
        let folder = ((tab.book.originalPath ?? tab.book.path) as NSString).deletingLastPathComponent
        let locked = (folder as NSString).appendingPathComponent("locked.pdf")
        if FileManager.default.fileExists(atPath: locked) {
            do {
                _ = try model.library.importBook(at: locked)
                report("pdf: locked.pdf was imported, it should have failed")
            } catch let error as CoreError {
                report("pdf: locked.pdf failed with kind \(error.kind): \"\(error.localizedDescription)\"; import alert: \"\(model.importProblems(failed: [], protected: [], locked: ["locked.pdf"]) ?? "")\"")
            } catch {
                report("pdf: locked.pdf failed: \(error.localizedDescription)")
            }
        }
        let long = (folder as NSString).appendingPathComponent("long.pdf")
        guard FileManager.default.fileExists(atPath: long), let id = try? model.library.importBook(at: long) else {
            return finish()
        }
        model.reload()
        guard let book = model.books.first(where: { $0.id == id }) else { return finish() }
        let opened = Date()
        model.open(book)
        after(1.0) {
            guard let longTab = model.activeTab, longTab.book.id == id else { return finish() }
            report("pdf: long.pdf has \(longTab.state.pages) pages, laid out in \(decimal(Date().timeIntervalSince(opened) - 1, 2)) s after the tab opened")
            let jump = Date()
            longTab.go(to: 349)
            report("pdf: went to page \(longTab.state.page + 1) in \(decimal(Date().timeIntervalSince(jump) * 1000, 1)) ms, chapter \(longTab.currentChapter?.title ?? "-")")
            longTab.find("page 399")
            let started = Date()
            @MainActor func wait() {
                guard longTab.searchDone || Date().timeIntervalSince(started) > 20 else { return after(0.1, wait) }
                report("pdf: search of long.pdf for \"page 399\": \(longTab.hits.count) hits in \(decimal(Date().timeIntervalSince(started), 2)) s")
                longTab.showHit(step: 1)
                after(0.8) {
                    report("pdf: showing hit at page \(longTab.state.page + 1)")
                    capture("pdf-long.png")
                    finish()
                }
            }
            wait()
        }
    }

    /// Runs the item of the View menu whose key equivalent is key, as ⌘ and
    /// the key would, and reports its title.
    private func menuItem(withKey key: String) {
        for menu in NSApp.mainMenu?.items.compactMap(\.submenu) ?? [] {
            menu.update()
            guard let index = menu.items.firstIndex(where: { $0.keyEquivalent == key }) else { continue }
            report("pdf: menu item \"\(menu.items[index].title)\" [⌘\(key)], enabled \(menu.items[index].isEnabled)")
            menu.performActionForItem(at: index)
            return
        }
        report("pdf: no menu item for ⌘\(key)")
    }

    /// Types key into the page, as a key press without modifiers.
    private func key(_ key: String, keyCode: UInt16 = 0, in view: EDRMetalView) {
        guard let event = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0,
                                           windowNumber: view.window?.windowNumber ?? 0, context: nil, characters: key,
                                           charactersIgnoringModifiers: key, isARepeat: false, keyCode: keyCode)
        else { return }
        view.keyDown(with: event)
    }

    /// Scrolls the page by dy points, as a trackpad does: a negative dy moves
    /// the content up, showing what is further down.
    private func scroll(by dy: Int32, in view: EDRMetalView) {
        guard let cgEvent = CGEvent(scrollWheelEvent2Source: nil, units: .pixel, wheelCount: 1, wheel1: dy, wheel2: 0,
                                    wheel3: 0),
              let event = NSEvent(cgEvent: cgEvent) else { return }
        view.scrollWheel(with: event)
    }
}

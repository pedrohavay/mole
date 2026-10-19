import AppKit

extension Verifier {
    /// Screenshots captures the library for the README, after the books given
    /// on the command line are added, and quits. Pride and Prejudice, Alice's
    /// Adventures in Wonderland and Frankenstein are read part of the way
    /// first, so they show on the library's shelves as books being read.
    /// `make screenshots` runs it. The window gets a fixed size, so every
    /// capture has the same dimensions.
    func screenshots(expecting count: Int) {
        NSApp.activate(ignoringOtherApps: true)
        model.showSidebar = true
        model.showContents = false
        waitForBooks(count, tries: 60) {
            guard let window = mainWindow else { return NSApp.terminate(nil) }
            window.setContentSize(NSSize(width: 1280, height: 800))
            window.center()
            openBook(titled: "Pride and Prejudice", atShare: 0.08) {
                openBook(titled: "Alice", atShare: 0.63) {
                    openBook(titled: "Frankenstein", atShare: 0.42) {
                        // Closing the tabs saves the places read, which the
                        // shelves then count.
                        model.tabs.forEach { model.close($0) }
                        model.pane = .library
                        model.selectedBook = nil
                        after(3.0) {
                            // The window looks inactive, with gray buttons,
                            // if another app took the focus meanwhile.
                            NSApp.activate(ignoringOtherApps: true)
                            window.makeKeyAndOrderFront(nil)
                            after(1.0) {
                                capture("library.png")
                                NSApp.terminate(nil)
                            }
                        }
                    }
                }
            }
        }
    }

    /// Calls next once the library holds count books and their covers, or
    /// after tries half seconds.
    private func waitForBooks(_ count: Int, tries: Int, next: @escaping @MainActor () -> Void) {
        if model.books.count >= count || tries == 0 {
            report("screenshots: \(model.books.count) of \(count) books")
            return after(2.0, next)
        }
        after(0.5) { waitForBooks(count, tries: tries - 1, next: next) }
    }

    /// Opens the first book whose title contains title, share of the way
    /// in, and calls next once the page is drawn.
    private func openBook(titled title: String, atShare share: Double, next: @escaping @MainActor () -> Void) {
        open(titled: title, next: next) { tab in
            tab.go(to: Int(Double(tab.state.pages) * share))
            report("screenshots: \(title) at page \(tab.state.page + 1) of \(tab.state.pages)")
        }
    }

    /// Opens the first book whose title contains title, lets place move in
    /// it once it is laid out, and calls next once the page is drawn.
    private func open(titled title: String, next: @escaping @MainActor () -> Void,
                      place: @escaping @MainActor (ReaderTab) -> Void) {
        guard let book = model.books.first(where: { $0.title.localizedCaseInsensitiveContains(title) }) else {
            report("screenshots: no book titled \(title)")
            return next()
        }
        model.open(book)
        after(2.5) {
            if let tab = model.activeTab { place(tab) }
            after(2.0, next)
        }
    }
}

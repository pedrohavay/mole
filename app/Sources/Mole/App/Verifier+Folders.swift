import AppKit

/// The folder check of the Verifier, run with `--verify-folder tree`. The
/// tree holds two folders of EPUB fixtures: Library, with at least three
/// books in subfolders, which is imported with collections and kept in
/// sync; and Spare, with at least two books used to change Library while
/// the app runs. The check moves, renames, rewrites and deletes files in
/// the tree, so the tree must be a scratch copy.
extension Verifier {
    func verifyFolders(_ tree: String) {
        let root = FolderWatcher.realPath((tree as NSString).appendingPathComponent("Library"))
        let spare = Self.epubs(in: (tree as NSString).appendingPathComponent("Spare"))
        let originals = Self.epubs(in: root)
        guard originals.count >= 3, spare.count >= 2 else {
            report("the folder check needs three books in Library and two in Spare")
            return NSApp.terminate(nil)
        }
        model.folderImport = FolderImport(paths: [root])
        after(1.0) {
            report("import sheet shown: \(mainWindow?.attachedSheet != nil)")
            capture("import-sheet.png", window: mainWindow?.attachedSheet)
            model.folderImport = nil
            model.importFolders([root], collections: true, watch: true)
            waitFor("the first import", { model.importingFolders == 0 && !model.books.isEmpty }) {
                reportLibrary("after the first import")
                showCollection { addFile(spare[0], into: root) { changeFiles(originals, spare) } }
            }
        }
    }

    /// Shows the largest collection, captures the sidebar with it, then
    /// makes a collection, captures its name being edited, and renames it.
    private func showCollection(then next: @escaping @MainActor () -> Void) {
        guard let largest = model.collections.max(by: { model.count(of: $0) < model.count(of: $1) }) else {
            report("no collection was made")
            return next()
        }
        model.show(largest)
        after(0.8) {
            report("showing \"\(model.libraryTitle)\": \(model.shelfBooks.map(\.title).joined(separator: ", "))")
            capture("collections.png")
            guard let first = model.shelfBooks.first else { return next() }
            model.createCollection(with: [first.id])
            after(0.8) {
                let editing = model.collections.first { $0.id == model.renamingCollection }
                report("new collection \"\(editing?.name ?? "-")\" editing its name: \(editing != nil)")
                capture("collection-rename.png")
                if let editing { model.rename(editing, to: "Favorites") }
                after(0.4) {
                    reportLibrary("after making Favorites")
                    next()
                }
            }
        }
    }

    /// Moves a spare book into a new subfolder of root and waits for the
    /// watcher to pick it up.
    private func addFile(_ file: String, into root: String, then next: @escaping @MainActor () -> Void) {
        let count = model.books.count
        let folder = (root as NSString).appendingPathComponent("Later")
        do {
            try FileManager.default.createDirectory(atPath: folder, withIntermediateDirectories: true)
            try FileManager.default.moveItem(atPath: file,
                                             toPath: (folder as NSString).appendingPathComponent((file as NSString).lastPathComponent))
        } catch {
            report("cannot add \(file): \(error.localizedDescription)")
            return next()
        }
        waitFor("a new file to be picked up", { model.books.count > count }) {
            reportLibrary("after adding a file while running")
            next()
        }
    }

    /// Renames, rewrites and deletes three books of the library, checking
    /// after each that no book is added and the right one changes.
    private func changeFiles(_ originals: [String], _ spare: [String]) {
        let renamed = originals[0].replacingOccurrences(of: ".epub", with: " (renamed).epub")
        guard let moved = book(at: originals[0]), let changed = book(at: originals[1]),
              let removed = book(at: originals[2]) else {
            report("the books of the fixtures are not in the library")
            return NSApp.terminate(nil)
        }
        let count = model.books.count
        do {
            try FileManager.default.moveItem(atPath: originals[0], toPath: renamed)
        } catch {
            report("cannot rename: \(error.localizedDescription)")
        }
        waitFor("the rename to be seen", { book(id: moved.id)?.originalPath == FolderWatcher.realPath(renamed) }) {
            report("renamed: book \(moved.id) now from \(book(id: moved.id)?.originalPath ?? "-"), \(model.books.count) books (was \(count))")
            do {
                try Data(contentsOf: URL(fileURLWithPath: spare[1])).write(to: URL(fileURLWithPath: originals[1]))
            } catch {
                report("cannot rewrite: \(error.localizedDescription)")
            }
            waitFor("the change to be seen", { book(id: changed.id)?.title != changed.title }) {
                report("changed: book \(changed.id) was \"\(changed.title)\", now \"\(book(id: changed.id)?.title ?? "-")\", \(model.books.count) books, \(copies()) copies in Books")
                do {
                    try FileManager.default.removeItem(atPath: originals[2])
                } catch {
                    report("cannot delete: \(error.localizedDescription)")
                }
                waitFor("the removal to be seen", { book(id: removed.id)?.missing == true }) {
                    reportLibrary("after deleting a file")
                    model.shelf = .all
                    model.selectedBook = removed.id
                    after(0.8) {
                        capture("missing.png")
                        openFolderSettings()
                    }
                }
            }
        }
    }

    private func openFolderSettings() {
        let main = mainWindow
        UserDefaults.standard.set(SettingsTab.general.rawValue, forKey: SettingsTab.storageKey)
        SettingsTab.openWindow()
        after(1.0) {
            if let settings = NSApp.windows.first(where: { $0.isVisible && $0 !== main && $0.contentView != nil }) {
                capture("settings-folders.png", window: settings)
            } else {
                report("the Settings window did not open")
            }
            NSApp.terminate(nil)
        }
    }

    private func reportLibrary(_ moment: String) {
        let books = model.books.sorted { $0.title < $1.title }
        report("\(moment): \(books.count) books, \(copies()) copies in Books")
        for book in books {
            let names = book.collections.compactMap { id in model.collections.first { $0.id == id }?.name }
            report("  \(book.title)\(book.missing ? " [missing]" : "") in [\(names.joined(separator: ", "))]")
        }
        let collections = model.collections.map { "\($0.name) (\(model.count(of: $0)))" }
        report("  collections: \(collections.joined(separator: ", "))")
        for folder in model.folders {
            report("  watching \(folder.path), collections \(folder.collections), synced \(folder.syncedAt != 0)")
        }
    }

    private func book(at path: String) -> Book? {
        let real = FolderWatcher.realPath(path)
        return model.books.first { $0.originalPath == real }
    }

    private func book(id: Int64) -> Book? {
        model.books.first { $0.id == id }
    }

    /// Returns how many files the library's Books folder holds.
    private func copies() -> Int {
        let books = ((model.library.path as NSString).deletingLastPathComponent as NSString).appendingPathComponent("Books")
        return (try? FileManager.default.contentsOfDirectory(atPath: books).count) ?? 0
    }

    /// Runs next once condition holds, or after timeout seconds, reporting
    /// how long it took.
    private func waitFor(_ what: String, timeout: Double = 20, started: Date = Date(),
                         _ condition: @escaping @MainActor () -> Bool, then next: @escaping @MainActor () -> Void) {
        if condition() {
            report("\(what): \(decimal(Date().timeIntervalSince(started), 1)) s")
            return next()
        }
        guard Date().timeIntervalSince(started) < timeout else {
            report("timed out waiting for \(what)")
            return next()
        }
        after(0.2) { waitFor(what, timeout: timeout, started: started, condition, then: next) }
    }

    /// Returns the EPUB files under folder, sorted by path.
    private static func epubs(in folder: String) -> [String] {
        let files = FileManager.default.enumerator(atPath: folder)?.allObjects as? [String] ?? []
        return files.filter { $0.lowercased().hasSuffix(".epub") && !$0.hasPrefix(".") }
            .map { (folder as NSString).appendingPathComponent($0) }
            .sorted()
    }
}

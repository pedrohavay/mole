import AppKit

// MARK: Collections

extension AppModel {
    /// CurrentCollection is the collection the library shows, if any.
    var currentCollection: BookCollection? {
        collection.flatMap { id in collections.first { $0.id == id } }
    }

    /// LibraryTitle is the heading of the library: the collection's name, or
    /// the shelf's.
    var libraryTitle: String { currentCollection?.name ?? shelf.title }

    /// Returns how many books collection holds.
    func count(of collection: BookCollection) -> Int {
        books.reduce(0) { $0 + ($1.collections.contains(collection.id) ? 1 : 0) }
    }

    /// Reads the collections and the watched folders again, and watches the
    /// folders.
    func reloadCollections() {
        do {
            collections = try library.collections()
                .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
            folders = try library.folders()
        } catch {
            NSLog("Mole: cannot list collections: %@", error.localizedDescription)
        }
        if let id = collection, !collections.contains(where: { $0.id == id }) { collection = nil }
        watcher.watch(folders.map(\.path))
    }

    /// Shows collection in the library.
    func show(_ collection: BookCollection) {
        self.collection = collection.id
        pane = .library
    }

    /// Makes a collection with a free name, holding books, shows it and
    /// starts editing its name.
    func createCollection(with books: [Int64] = []) {
        let id: Int64
        do {
            id = try library.createCollection(named: freeCollectionName())
        } catch {
            return alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        collection = id
        pane = .library
        renamingCollection = id
        do {
            for book in books { try library.add(book: book, to: id) }
        } catch {
            alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        reload()
    }

    /// Returns "New Collection", or "New Collection 2" and so on when that
    /// name is taken.
    private func freeCollectionName() -> String {
        let taken = Set(collections.map { $0.name.lowercased() })
        let base = L10n.text("collection.defaultName")
        if !taken.contains(base.lowercased()) { return base }
        var number = 2
        while taken.contains(L10n.format("collection.defaultName.numbered", number).lowercased()) { number += 1 }
        return L10n.format("collection.defaultName.numbered", number)
    }

    /// Gives collection a new name and ends editing it. A blank name keeps
    /// the old one; a name another collection has is refused.
    func rename(_ collection: BookCollection, to name: String) {
        renamingCollection = nil
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, name != collection.name else { return }
        if collections.contains(where: { $0.id != collection.id && $0.name.caseInsensitiveCompare(name) == .orderedSame }) {
            alert(L10n.format("collection.exists", name), L10n.text("collection.exists.detail"))
            return
        }
        do {
            try library.rename(collection: collection.id, to: name)
        } catch {
            alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        reloadCollections()
    }

    /// Deletes collection after asking. Its books stay in the library.
    func delete(_ collection: BookCollection) {
        let confirm = NSAlert()
        confirm.alertStyle = .warning
        confirm.messageText = L10n.format("collection.delete.title", collection.name)
        confirm.informativeText = L10n.text("collection.delete.detail")
        confirm.addButton(withTitle: L10n.text("collection.delete.action"))
        confirm.addButton(withTitle: L10n.text("action.cancel"))
        guard confirm.runModal() == .alertFirstButtonReturn else { return }
        do {
            try library.delete(collection: collection.id)
        } catch {
            alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        reload()
    }

    /// Puts books in collection.
    func add(_ books: [Int64], to collection: BookCollection) {
        do {
            for book in books { try library.add(book: book, to: collection.id) }
        } catch {
            alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        reload()
    }

    /// Puts book in collection or takes it out.
    func set(_ book: Book, in collection: BookCollection, _ member: Bool) {
        do {
            if member {
                try library.add(book: book.id, to: collection.id)
            } else {
                try library.remove(book: book.id, from: collection.id)
            }
        } catch {
            alert(L10n.text("collection.failed"), error.localizedDescription)
        }
        reload()
    }
}

// MARK: Folders

extension AppModel {
    /// Adds the books in folders, in the background. With collections each
    /// folder that holds books becomes a collection; with watch the folders
    /// are kept in sync.
    func importFolders(_ folders: [String], collections: Bool, watch: Bool) {
        guard !folders.isEmpty else { return }
        importingFolders += folders.count
        Task {
            var found = 0
            var failed: [String] = []
            var protected: [String] = []
            var locked: [String] = []
            for path in folders {
                do {
                    let result = try await importer.run { try $0.importFolder(at: path, collections: collections, watch: watch) }
                    found += result.found
                    let names = Self.failedNames(result.failed)
                    failed += names.failed
                    protected += names.protected
                    locked += names.locked
                    forgetCovers(of: result.updated)
                } catch {
                    NSLog("Mole: cannot import the folder %@: %@", path, error.localizedDescription)
                    failed.append("\((path as NSString).lastPathComponent) (\(error.localizedDescription))")
                }
            }
            importingFolders -= folders.count
            reload()
            if let problems = importProblems(failed: failed, protected: protected, locked: locked) {
                alert(L10n.text("import.failed.title"), problems)
            } else if found == 0 && !watch {
                alert(L10n.text("import.folder.none.title"), L10n.text("import.folder.none.detail"))
            }
        }
    }

    /// Syncs folder in the background. Unless quietly, failures are shown.
    func sync(_ folder: WatchedFolder, quietly: Bool = false) {
        guard !syncing.contains(folder.id) else {
            // A sync is running; it may have read the folder before this
            // change, so the folder is synced again once it ends.
            resync.insert(folder.id)
            return
        }
        syncing.insert(folder.id)
        Task {
            var failure: String?
            do {
                let id = folder.id
                let result = try await importer.run { try $0.sync(folder: id) }
                forgetCovers(of: result.updated)
                let names = Self.failedNames(result.failed)
                failure = importProblems(failed: names.failed, protected: names.protected, locked: names.locked)
            } catch {
                NSLog("Mole: cannot sync %@: %@", folder.path, error.localizedDescription)
                failure = error.localizedDescription
            }
            syncing.remove(folder.id)
            reload()
            if let failure, !quietly {
                alert(L10n.format("sync.failed", folder.name), failure)
            }
            if resync.remove(folder.id) != nil, let again = folders.first(where: { $0.id == folder.id }) {
                sync(again, quietly: true)
            }
        }
    }

    /// Syncs every watched folder.
    func syncFolders(quietly: Bool = false) {
        for folder in folders { sync(folder, quietly: quietly) }
    }

    /// Stops watching folder. Its books stay in the library. It runs after
    /// any sync of the folder, so a sync cannot mark books again once the
    /// folder is gone.
    func remove(_ folder: WatchedFolder) {
        Task {
            do {
                let id = folder.id
                try await importer.run { try $0.remove(folder: id) }
            } catch {
                alert(L10n.text("folder.remove.failed"), error.localizedDescription)
            }
            reload()
        }
    }

    /// Syncs the watched folders at paths, which FolderWatcher saw change.
    func foldersChanged(_ paths: Set<String>) {
        for folder in folders where paths.contains(folder.path) {
            sync(folder, quietly: true)
        }
    }

    /// Returns the names of the files a folder import could not add: the
    /// books protected by DRM and the PDFs that need a password apart from
    /// the rest, which are logged.
    nonisolated static func failedNames(_ failures: [FolderImportResult.Failure])
        -> (failed: [String], protected: [String], locked: [String]) {
        var failed: [String] = []
        var protected: [String] = []
        var locked: [String] = []
        for failure in failures {
            let name = (failure.path as NSString).lastPathComponent
            if failure.drm {
                protected.append(name)
            } else if failure.password {
                locked.append(name)
            } else {
                NSLog("Mole: cannot import %@: %@", failure.path, failure.error)
                failed.append(name)
            }
        }
        return (failed, protected, locked)
    }
}

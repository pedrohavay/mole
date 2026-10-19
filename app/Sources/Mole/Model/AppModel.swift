import AppKit
import Observation
import SwiftUI
import UniformTypeIdentifiers

/// Pane is what the main area shows.
enum Pane: Hashable {
    case library
    case book(Int64)
}

/// PanelPage is a page of the side panel next to a book.
enum PanelPage: String, CaseIterable, Identifiable {
    case contents, bookmarks, notes, search

    var id: String { rawValue }

    var title: String { L10n.text("panel.\(rawValue)") }

    var symbol: String {
        switch self {
        case .contents: "list.bullet.indent"
        case .bookmarks: "bookmark"
        case .notes: "highlighter"
        case .search: "magnifyingglass"
        }
    }
}

/// AppModel is the state of the whole app. It runs on the main thread and
/// hands imports to Importer, which runs off it.
@Observable @MainActor
final class AppModel {
    let library: Library
    private(set) var books: [Book] = []
    private(set) var covers: [Int64: NSImage] = [:]
    private(set) var tabs: [ReaderTab] = []
    var pane: Pane = .library {
        didSet {
            // Reading aloud belongs to the book on screen.
            if readAloud.bookID != nil, readAloud.bookID != activeTab?.id { readAloud.stop() }
            saveTabs()
        }
    }
    /// ReadAloud speaks the open book.
    let readAloud = ReadAloud()
    /// Shelf filters the library unless a collection is shown; picking a
    /// shelf leaves the collection.
    var shelf: Shelf = .all {
        didSet { collection = nil }
    }
    /// Collection, when set, is the collection the library shows.
    var collection: Int64?
    /// Collections and folders are the library's collections and watched
    /// folders, read again with the books.
    var collections: [BookCollection] = []
    var folders: [WatchedFolder] = []
    /// RenamingCollection is the collection whose name is being edited in
    /// the sidebar.
    var renamingCollection: Int64?
    /// FolderImport, while set, asks how to add the books in some folders.
    var folderImport: FolderImport?
    /// ImportingFolders counts the folders being added in the background.
    var importingFolders = 0
    /// Syncing holds the watched folders being synced now.
    var syncing: Set<Int64> = []
    var query = ""
    var selectedBook: Int64?
    private(set) var headroom = Headroom()
    /// Importing counts the files still being added in the background.
    private(set) var importing = 0
    /// IsFullScreen mirrors the window; while reading in full screen the
    /// interface steps aside and only the page is left.
    var isFullScreen = false

    var showSidebar: Bool {
        didSet { UserDefaults.standard.set(showSidebar, forKey: Keys.sidebar) }
    }

    var showContents: Bool {
        didSet { UserDefaults.standard.set(showContents, forKey: Keys.contents) }
    }

    /// PanelPage is the page of the side panel next to a book.
    var panelPage = PanelPage.contents
    /// SearchFocus changes whenever the search field should take the keys.
    private(set) var searchFocus = 0

    /// Settings are saved shortly after they stop changing, so dragging a
    /// slider does not write the database on every step.
    var settings: ReaderSettings {
        didSet {
            if settings != oldValue { scheduleSave() }
        }
    }

    @ObservationIgnored private var poll: Timer?
    /// Importer adds books off the main thread, on a library handle of its own.
    @ObservationIgnored lazy var importer = Importer(path: library.path)
    /// Watcher syncs the watched folders when files change in them.
    @ObservationIgnored lazy var watcher = FolderWatcher(ignoring: URL(fileURLWithPath: library.path).deletingLastPathComponent().path) {
        [weak self] paths in self?.foldersChanged(paths)
    }
    /// Resync holds the folders that changed while they were being synced.
    @ObservationIgnored var resync: Set<Int64> = []
    @ObservationIgnored private var pendingSave: DispatchWorkItem?
    @ObservationIgnored private var observers: [NSObjectProtocol] = []

    /// Keys are the user defaults the model keeps between launches.
    private enum Keys {
        static let sidebar = "sidebarVisible"
        static let contents = "contentsVisible"
        static let openBooks = "openBooks"
        static let activeBook = "activeBook"
        static let highlightColor = "highlightColor"
    }

    init(library: Library) throws {
        self.library = library
        settings = try library.settings()
        let defaults = UserDefaults.standard
        defaults.register(defaults: [Keys.sidebar: true, Keys.contents: true])
        showSidebar = defaults.bool(forKey: Keys.sidebar)
        showContents = defaults.bool(forKey: Keys.contents)
        reload()
        restoreTabs()
        // Folders may have changed while Mole was closed.
        syncFolders(quietly: true)
        // macOS posts no notification when the headroom changes (brightness
        // keys, the EDR ramp), so it is polled.
        poll = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.sampleHeadroom() }
        }
        observers.append(NotificationCenter.default.addObserver(
            forName: NSApplication.willTerminateNotification, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.prepareToQuit() }
        })
    }

    private func sampleHeadroom() {
        guard let screen = NSApp.keyWindow?.screen ?? NSScreen.main else { return }
        let now = Headroom(screen: screen)
        if abs(now.current - headroom.current) > 0.005 || now.potential != headroom.potential {
            headroom = now
        }
    }

    // MARK: Derived state

    /// Gain is the multiplier for fully covered glyph pixels (1 = SDR white).
    var gain: Double {
        let gain = settings.level * headroom.current
        return settings.edr ? gain : min(gain, 1)
    }

    /// Accent is the interface accent color.
    var accent: Color { Color(hexString: settings.accent) ?? Theme.defaultAccent }

    /// AccentLight is the accent as light for the page, in linear Display P3.
    var accentLight: SIMD3<Float> {
        guard let p3 = NSColor(accent).usingColorSpace(.displayP3) else { return [1, 0.3, 0.3] }
        func linear(_ value: CGFloat) -> Float { Float(pow(max(0, value), 2.2)) }
        return [linear(p3.redComponent), linear(p3.greenComponent), linear(p3.blueComponent)]
    }

    var shelfBooks: [Book] {
        let query = query.trimmingCharacters(in: .whitespaces)
        return books.filter { book in
            let kept = collection.map { book.collections.contains($0) } ?? shelf.keeps(book)
            return kept && (query.isEmpty || "\(book.title) \(book.author)".localizedStandardContains(query))
        }
    }

    var activeTab: ReaderTab? {
        guard case .book(let id) = pane else { return nil }
        return tabs.first { $0.id == id }
    }

    /// Reports whether only the page is on screen.
    var isImmersive: Bool { isFullScreen && activeTab != nil }

    // MARK: Library

    /// Reads the book list again and loads the covers it does not have yet.
    func reload() {
        do {
            books = try library.books()
        } catch {
            NSLog("Mole: cannot list books: %@", error.localizedDescription)
        }
        reloadCollections()
        for book in books where covers[book.id] == nil {
            guard let data = library.cover(of: book.id) else { continue }
            // Covers can be large; they are decoded at display size, off
            // the main thread.
            let id = book.id
            Task.detached(priority: .userInitiated) { [weak self] in
                guard let image = CoverThumbnail.image(from: data) else { return }
                await self?.setCover(image, for: id)
            }
        }
    }

    /// Forgets the covers of books whose file changed, so reload reads them
    /// again.
    func forgetCovers(of books: [Int64]) {
        for id in books { covers[id] = nil }
    }

    private func setCover(_ image: CGImage, for book: Int64) {
        // The book may have been removed while its cover was decoding.
        guard books.contains(where: { $0.id == book }) else { return }
        covers[book] = NSImage(cgImage: image, size: NSSize(width: image.width, height: image.height))
    }

    func chooseBooks() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = true
        panel.allowedContentTypes = BookFiles.contentTypes + [.folder]
        panel.prompt = L10n.text("import.prompt")
        panel.message = L10n.text("import.panel.message")
        guard panel.runModal() == .OK else { return }
        add(panel.urls.map(\.path))
    }

    /// Adds the books at paths: files at once, and folders after asking how
    /// (see FolderImportSheet).
    func add(_ paths: [String]) {
        let (files, folders) = BookFiles.sort(paths)
        importBooks(files)
        if !folders.isEmpty { folderImport = FolderImport(paths: folders) }
    }

    /// Adds the books at paths in the background; done runs on the main
    /// thread with the ids of the books added, once they are listed.
    func importBooks(_ paths: [String], done: @escaping @MainActor ([Int64]) -> Void = { _ in }) {
        guard !paths.isEmpty else { return }
        importing += paths.count
        Task {
            let result = await importer.add(paths)
            importing -= paths.count
            // A file imported again may have changed, cover included.
            forgetCovers(of: result.added)
            reload()
            if let problems = importProblems(failed: result.failed, protected: result.protected, locked: result.locked) {
                alert(L10n.text("import.failed.title"), problems)
            }
            done(result.added)
        }
    }

    /// Returns what went wrong adding books, a line for the files that could
    /// not be read, one for the books protected by DRM and one for the PDFs
    /// that need a password, or nil when nothing did.
    func importProblems(failed: [String], protected: [String], locked: [String] = []) -> String? {
        var lines: [String] = []
        if !failed.isEmpty {
            lines.append(L10n.format("import.failed.detail", failed.joined(separator: ", ")))
        }
        if !protected.isEmpty {
            lines.append(L10n.format("import.drm.detail", protected.joined(separator: ", ")))
        }
        if !locked.isEmpty {
            lines.append(L10n.format("import.password.detail", locked.joined(separator: ", ")))
        }
        return lines.isEmpty ? nil : lines.joined(separator: "\n")
    }

    /// Adds the books at paths, as opened from Finder, and opens the first
    /// one. Folders are added as from File > Add Books.
    func openFiles(_ paths: [String]) {
        let (files, folders) = BookFiles.sort(paths)
        if !folders.isEmpty { folderImport = FolderImport(paths: folders) }
        importBooks(files) { [weak self] added in
            guard let self, let id = added.first, let book = self.books.first(where: { $0.id == id }) else { return }
            self.open(book)
        }
    }

    func remove(_ book: Book) {
        let confirm = NSAlert()
        confirm.alertStyle = .warning
        confirm.messageText = L10n.format("remove.confirm.title", book.title)
        confirm.informativeText = L10n.text("remove.confirm.detail")
        confirm.addButton(withTitle: L10n.text("remove.confirm.action"))
        confirm.addButton(withTitle: L10n.text("action.cancel"))
        guard confirm.runModal() == .alertFirstButtonReturn else { return }
        forget(book)
    }

    /// Removes book from the library without asking.
    private func forget(_ book: Book) {
        if let open = tabs.first(where: { $0.id == book.id }) { close(open) }
        do {
            try library.remove(book.id)
        } catch {
            alert(L10n.text("remove.failed"), error.localizedDescription)
        }
        if selectedBook == book.id { selectedBook = nil }
        covers[book.id] = nil
        reload()
    }

    /// Shows the file the book was imported from in Finder, or Mole's own
    /// copy when the original is gone.
    func revealInFinder(_ book: Book) {
        let original = book.originalPath.flatMap { FileManager.default.fileExists(atPath: $0) ? $0 : nil }
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: original ?? book.path)])
    }

    // MARK: Tabs

    func open(_ book: Book) {
        if !tabs.contains(where: { $0.id == book.id }) {
            guard FileManager.default.fileExists(atPath: book.path) else {
                missing(book)
                return
            }
            do {
                tabs.append(ReaderTab(book: book, session: try Session(library: library, book: book.id),
                                      library: library))
            } catch {
                alert(L10n.text("open.failed"), L10n.format("open.failed.detail", error.localizedDescription))
                return
            }
        }
        selectedBook = book.id
        pane = .book(book.id)
    }

    /// Explains that the file of book is gone and offers to remove it.
    private func missing(_ book: Book) {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = L10n.format("missing.title", book.title)
        alert.informativeText = L10n.format("missing.detail", book.path)
        alert.addButton(withTitle: L10n.text("missing.remove"))
        alert.addButton(withTitle: L10n.text("action.cancel"))
        if alert.runModal() == .alertFirstButtonReturn { forget(book) }
    }

    func close(_ readerTab: ReaderTab) {
        guard let index = tabs.firstIndex(where: { $0.id == readerTab.id }) else { return }
        if readAloud.bookID == readerTab.id { readAloud.stop() }
        // Closing saves the position, which the reload below reads back.
        readerTab.close()
        tabs.remove(at: index)
        if pane == .book(readerTab.id) {
            let next = tabs.indices.contains(index) ? tabs[index] : tabs.last
            pane = next.map { .book($0.id) } ?? .library
        }
        saveTabs()
        reload()
    }

    func closeActiveTab() {
        if let activeTab { close(activeTab) }
    }

    /// Shows the library for index 0 and the book tab index for 1 and up.
    func showTab(_ index: Int) {
        if index == 0 {
            pane = .library
        } else if tabs.indices.contains(index - 1) {
            pane = .book(tabs[index - 1].id)
        }
    }

    /// Moves offset tabs to the right (or left), wrapping around the library.
    func cycleTabs(_ offset: Int) {
        let current = activeTab.flatMap { tab in tabs.firstIndex { $0.id == tab.id } }.map { $0 + 1 } ?? 0
        let count = tabs.count + 1
        showTab(((current + offset) % count + count) % count)
    }

    /// Remembers the open books and the shown one for the next launch.
    private func saveTabs() {
        let defaults = UserDefaults.standard
        defaults.set(tabs.map { Int($0.id) }, forKey: Keys.openBooks)
        if case .book(let id) = pane {
            defaults.set(Int(id), forKey: Keys.activeBook)
        } else {
            defaults.removeObject(forKey: Keys.activeBook)
        }
    }

    /// Reopens the books that were open when the app last quit. Books that
    /// are gone or cannot be opened are skipped quietly.
    private func restoreTabs() {
        let defaults = UserDefaults.standard
        let wanted = (defaults.array(forKey: Keys.openBooks) as? [Int]) ?? []
        let active = defaults.integer(forKey: Keys.activeBook)
        for id in wanted {
            guard let book = books.first(where: { $0.id == Int64(id) }),
                  FileManager.default.fileExists(atPath: book.path),
                  let session = try? Session(library: library, book: book.id) else { continue }
            tabs.append(ReaderTab(book: book, session: session, library: library))
        }
        if tabs.contains(where: { $0.id == Int64(active) }) {
            pane = .book(Int64(active))
        }
    }

    // MARK: Reading controls

    func adjustLevel(by factor: Double) {
        settings.level = (settings.level * factor).clamped(to: ReaderSettings.levels)
    }

    /// Makes the text bigger or smaller by step points; in a book of fixed
    /// pages, whose text keeps its size, it zooms in or out a step instead.
    func adjustFontSize(by step: Double) {
        if let tab = activeTab, tab.isFixedLayout {
            tab.zoom(by: step > 0 ? 1 : -1)
            return
        }
        settings.fontSize = (settings.fontSize + step).clamped(to: ReaderSettings.fontSizes)
    }

    func cycleTint() {
        settings.tint = (settings.tint + 1) % Tint.all.count
    }

    /// Shows page of the side panel of the open book.
    func showPanel(_ page: PanelPage) {
        panelPage = page
        Motion.animate { showContents = true }
    }

    /// Opens the search of the open book with its field focused.
    func beginSearch() {
        guard activeTab != nil else { return }
        showPanel(.search)
        searchFocus += 1
    }

    /// Bookmarks the visible pages of the open book, or removes the bookmark.
    func toggleBookmark() {
        guard let tab = activeTab else { return }
        do { try tab.toggleBookmark() } catch { alert(L10n.text("annotation.failed"), error.localizedDescription) }
    }

    /// Highlights the selection of the open book in color.
    func highlightSelection(color: Int? = nil) {
        guard let tab = activeTab else { return }
        do {
            try tab.highlightSelection(color: color ?? lastHighlightColor)
            if let color { lastHighlightColor = color }
        } catch {
            alert(L10n.text("annotation.failed"), error.localizedDescription)
        }
    }

    /// LastHighlightColor is the color the next highlight takes by default.
    var lastHighlightColor: Int {
        get { UserDefaults.standard.integer(forKey: Keys.highlightColor) }
        set { UserDefaults.standard.set(newValue, forKey: Keys.highlightColor) }
    }

    /// Starts reading the open book aloud from its selection or the top of
    /// the screen, or pauses or resumes the reading.
    func toggleReadAloud() {
        guard let tab = activeTab else { return }
        readAloud.toggle(tab)
    }

    /// Copies the selection of the open book to the pasteboard.
    func copySelection() {
        guard let text = activeTab?.selectedText(), !text.isEmpty else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }

    /// Searches the open book for its selection.
    func searchSelection() {
        guard let tab = activeTab, let text = tab.selectedText() else { return }
        let query = ReaderTab.excerpt(text, limit: 80)
        tab.clearSelection()
        tab.find(query)
        showPanel(.search)
    }

    func toggleFullScreen() {
        (NSApp.keyWindow ?? NSApp.mainWindow)?.toggleFullScreen(nil)
    }

    // MARK: Persistence

    private func scheduleSave() {
        pendingSave?.cancel()
        let work = DispatchWorkItem { [weak self] in
            MainActor.assumeIsolated { self?.saveSettings() }
        }
        pendingSave = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3, execute: work)
    }

    /// Writes the settings now, cancelling any delayed save.
    func saveSettings() {
        pendingSave?.cancel()
        pendingSave = nil
        do {
            try library.save(settings)
        } catch {
            NSLog("Mole: cannot save settings: %@", error.localizedDescription)
        }
    }

    private func prepareToQuit() {
        readAloud.stop()
        if pendingSave != nil { saveSettings() }
        // Open tabs stay listed for the next launch; closing the sessions
        // only saves their positions.
        tabs.forEach { $0.close() }
    }

    func alert(_ message: String, _ detail: String) {
        let alert = NSAlert()
        alert.messageText = message
        alert.informativeText = detail
        alert.runModal()
    }
}

/// Importer adds books to the library on a background queue. Imports run
/// one at a time on a library handle used by nothing else.
final class Importer: @unchecked Sendable {
    /// Result lists the books added and the names of the files that failed:
    /// those protected by DRM and the PDFs that need a password apart from
    /// the rest.
    struct Result: Sendable {
        var added: [Int64] = []
        var failed: [String] = []
        var protected: [String] = []
        var locked: [String] = []
    }

    private let path: String
    private let queue = DispatchQueue(label: "Mole.import", qos: .userInitiated)
    // Opened on the queue and used only there.
    private var library: Library?

    init(path: String) {
        self.path = path
    }

    func add(_ paths: [String]) async -> Result {
        await withCheckedContinuation { continuation in
            queue.async { continuation.resume(returning: self.addNow(paths)) }
        }
    }

    /// Runs body on the import queue with the importer's library, after the
    /// imports queued before it.
    func run<T: Sendable>(_ body: @escaping @Sendable (Library) throws -> T) async throws -> T {
        try await withCheckedThrowingContinuation { continuation in
            queue.async {
                continuation.resume(with: Swift.Result {
                    let library = try self.library ?? Library(path: self.path)
                    self.library = library
                    return try body(library)
                })
            }
        }
    }

    private func addNow(_ paths: [String]) -> Result {
        var result = Result()
        let library: Library
        do {
            library = try self.library ?? Library(path: path)
            self.library = library
        } catch {
            NSLog("Mole: cannot open the library to import: %@", error.localizedDescription)
            result.failed = paths.map { ($0 as NSString).lastPathComponent }
            return result
        }
        for path in paths {
            do {
                result.added.append(try library.importBook(at: path))
            } catch let error as CoreError where error.kind == .drm {
                result.protected.append((path as NSString).lastPathComponent)
            } catch let error as CoreError where error.kind == .password {
                result.locked.append((path as NSString).lastPathComponent)
            } catch {
                result.failed.append((path as NSString).lastPathComponent)
                NSLog("Mole: cannot import %@: %@", path, error.localizedDescription)
            }
        }
        return result
    }
}

extension Comparable {
    /// Returns the value limited to range.
    func clamped(to range: ClosedRange<Self>) -> Self {
        min(max(self, range.lowerBound), range.upperBound)
    }
}

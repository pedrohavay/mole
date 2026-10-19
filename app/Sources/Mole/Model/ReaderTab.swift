import AppKit
import Foundation
import Observation

/// PageMark is something painted under the text of the visible pages, in
/// view points from the top left.
struct PageMark: Equatable {
    enum Kind: Equatable {
        case highlight(color: Int)
        case selection
        case hit(current: Bool)
        /// Speech marks the sentence being read aloud.
        case speech
    }

    var rect: CGRect
    var kind: Kind
}

/// PageParagraph is a paragraph on the visible pages, or the part of one on
/// a page, with the boxes of its lines in view points from the top left.
struct PageParagraph: Equatable {
    let range: TextRange
    let text: String
    let lines: [CGRect]

    /// Frame is the box around the lines.
    var frame: CGRect { lines.dropFirst().reduce(lines.first ?? .zero) { $0.union($1) } }
}

/// ReaderTab is an open book: its session with the core, what the reader
/// shows of it, and the reader's selection, annotations and search.
@Observable @MainActor
final class ReaderTab: Identifiable {
    let book: Book
    let session: Session
    private let library: Library

    private(set) var state = SessionState()
    private(set) var chapters: [Chapter] = []
    /// Revision changes whenever the visible pages change and must be redrawn.
    private(set) var revision = 0
    /// VisibleRange is the text on the visible pages.
    private(set) var visibleRange: TextRange?
    /// Marks are the highlights, selection, search hits and the sentence
    /// read aloud on screen.
    private(set) var marks: [PageMark] = []
    /// Spoken is the sentence being read aloud, if any.
    private(set) var spoken: TextRange?
    /// Paragraphs caches the paragraphs of the visible pages for the
    /// revision they were read at; they are read only when asked for.
    @ObservationIgnored private var paragraphs: (revision: Int, list: [PageParagraph])?
    @ObservationIgnored private var bookLanguage: String??
    /// TimeLeft estimates the reading time to the end of the chapter and of
    /// the book, at the reader's pace; nil until a screen of text is known.
    private(set) var timeLeft: TimeLeft?
    @ObservationIgnored private var screenShownAt = Date()
    /// ScreenCharacters is the text on the visible pages, in characters, when
    /// it lies in one chapter; averageScreen smooths it over screens.
    @ObservationIgnored private var screenCharacters: Int?
    @ObservationIgnored private var averageScreen: Double?

    private(set) var annotations: [Annotation] = []
    /// Selection is the text the reader dragged over, if any.
    private(set) var selection: TextRange?
    /// IsSelecting is set while the pointer drags out a selection.
    var isSelecting = false
    @ObservationIgnored private var selectionStart: Anchor?
    /// TranslationText is the text the translation popover shows, while it
    /// is open.
    var translationText: String?
    /// LookUpHandler shows the system dictionary for a text at its box in
    /// the view; the reader canvas sets it.
    @ObservationIgnored var lookUpHandler: ((String, CGRect) -> Void)?

    /// The search of the book, if one was asked for.
    private(set) var query = ""
    private(set) var hits: [Search.Hit] = []
    private(set) var searchDone = true
    private(set) var searchTruncated = false
    /// CurrentHit indexes hits: the match last shown.
    private(set) var currentHit: Int?
    @ObservationIgnored private var search: Search?
    @ObservationIgnored private var searchPoll: Timer?

    nonisolated var id: Int64 { book.id }

    init(book: Book, session: Session, library: Library) {
        self.book = book
        self.session = session
        self.library = library
        reloadAnnotations()
    }

    // MARK: Layout and pages

    /// Lays the book out for a view; see Session.layout.
    func layout(width: Double, height: Double, scale: Double, fontSize: Double, spread: Spread) throws {
        try session.layout(width: width, height: height, scale: scale, fontSize: fontSize, spread: spread)
        chapters = (try? session.chapters()) ?? []
        refresh()
    }

    func turn(_ screens: Int) {
        // Reading one screen and moving on is how the reading pace is
        // learned; jumps and turning back teach nothing.
        let readFor = Date().timeIntervalSince(screenShownAt)
        let characters = screenCharacters
        guard session.turn(screens) else { return }
        if screens == 1, let characters {
            PaceStore.shared.record(characters: characters, seconds: readFor)
        }
        refresh()
    }

    func go(to page: Int) {
        if session.go(to: page) { refresh() }
    }

    func go(to anchor: Anchor) {
        if (try? session.go(to: anchor)) == true { refresh() }
    }

    func goToStart() { go(to: 0) }

    func goToEnd() { go(to: max(0, state.pages - 1)) }

    /// Returns the page, from 1, that holds anchor in the current layout.
    func pageNumber(of anchor: Anchor) -> Int? {
        (try? session.page(of: anchor)).map { $0 + 1 }
    }

    /// Returns the top-level chapter that holds anchor.
    func chapter(of anchor: Anchor) -> Chapter? {
        guard let page = try? session.page(of: anchor) else { return nil }
        return chapters.last { $0.level == 0 && $0.page <= page }
    }

    private func refresh() {
        if let state = try? session.state() { self.state = state }
        visibleRange = try? session.visibleRange()
        screenShownAt = Date()
        screenCharacters = visibleRange.flatMap(Self.characters(in:))
        if let screenCharacters, screenCharacters >= 200 {
            averageScreen = averageScreen.map { $0 + 0.2 * (Double(screenCharacters) - $0) } ?? Double(screenCharacters)
        }
        refreshTimeLeft()
        refreshMarks()
        revision += 1
    }

    /// Returns the characters in range when it lies in one chapter. Offsets
    /// count non-whitespace characters, the unit of the reading pace.
    private static func characters(in range: TextRange) -> Int? {
        guard range.start.chapter == range.end.chapter else { return nil }
        return range.end.offset - range.start.offset
    }

    /// Estimates the time left from the pages left and the average text on
    /// a screen.
    func refreshTimeLeft() {
        guard let averageScreen, state.pages > 0 else {
            timeLeft = nil
            return
        }
        let pace = PaceStore.shared.pace
        let perPage = averageScreen / Double(max(1, state.columns))
        let pagesLeft = max(0, state.pages - state.lastVisible - 1)
        let nextChapter = chapters.first { $0.level == 0 && $0.page > state.lastVisible }?.page
        let chapterLeft = nextChapter.map { max(0, $0 - state.lastVisible - 1) }
        timeLeft = TimeLeft(chapter: chapterLeft.map { pace.time(for: Int(Double($0) * perPage)) },
                            book: pace.time(for: Int(Double(pagesLeft) * perPage)))
    }

    /// The top-level chapter that holds the first visible page.
    var currentChapter: Chapter? {
        chapters.last { $0.level == 0 && $0.page <= state.page }
    }

    /// PageDescription says which pages are shown and in which chapter, as
    /// VoiceOver announces a new screen.
    var pageDescription: String {
        guard state.pages > 0 else { return book.title }
        let pages = state.lastVisible > state.page
            ? L10n.format("reader.pages", state.page + 1, state.lastVisible + 1, state.pages)
            : L10n.format("reader.page", state.page + 1, state.pages)
        guard let chapter = currentChapter?.title, !chapter.isEmpty else { return pages }
        return "\(pages), \(chapter)"
    }

    /// Returns the paragraphs of the visible pages with their lines, in
    /// reading order. They are read from the core the first time they are
    /// asked for after the pages change.
    func visibleParagraphs() -> [PageParagraph] {
        if let paragraphs, paragraphs.revision == revision { return paragraphs.list }
        var list: [PageParagraph] = []
        if let visibleRange, let parts = try? session.paragraphs(of: visibleRange) {
            for part in parts {
                guard let lines = try? session.rects(of: part.range), !lines.isEmpty else { continue }
                list.append(PageParagraph(range: part.range, text: part.text, lines: lines))
            }
        }
        paragraphs = (revision, list)
        return list
    }

    /// Language is the language to read the book aloud in, as a BCP 47 tag
    /// such as "en" or "pt-BR": the one the book declares, or else one worked
    /// out from its text and the user's languages (see
    /// ReadAloud.readingLanguage). It is worked out once, when first asked.
    var language: String? {
        if let bookLanguage { return bookLanguage }
        let declared = (try? session.language()) ?? nil
        let language = ReadAloud.readingLanguage(declared: declared, text: sampleText(),
                                                 preferred: Locale.preferredLanguages)
        // Before the first layout there is no text to look at yet.
        if visibleRange != nil { bookLanguage = .some(language) }
        return language
    }

    /// Returns a few pages of text from the top of the screen, for a book
    /// that declares no language.
    private func sampleText() -> String {
        guard let start = visibleRange?.start,
              let ahead = try? session.paragraphs(of: TextRange(start: start, end: .endOfBook), limit: 6000)
        else { return "" }
        return ahead.map(\.text).joined(separator: "\n")
    }

    // MARK: Marks

    private func refreshMarks() {
        guard let visible = visibleRange else {
            marks = []
            return
        }
        var marks: [PageMark] = []
        func add(_ range: TextRange, _ kind: PageMark.Kind) {
            guard range.overlaps(visible), let rects = try? session.rects(of: range) else { return }
            marks += rects.map { PageMark(rect: $0, kind: kind) }
        }
        for annotation in annotations where annotation.kind == .highlight {
            add(annotation.range, .highlight(color: annotation.color))
        }
        for (index, hit) in hits.enumerated() where hit.range.overlaps(visible) {
            add(hit.range, .hit(current: index == currentHit))
        }
        if let spoken { add(spoken, .speech) }
        if let selection { add(selection, .selection) }
        if marks != self.marks { self.marks = marks }
    }

    /// Marks range as the sentence being read aloud, or clears the mark.
    func markSpoken(_ range: TextRange?) {
        guard range != spoken else { return }
        spoken = range
        refreshMarks()
    }

    // MARK: Selection

    /// Starts a selection at a point of the view, as pressing the mouse
    /// before a drag does. It reports whether the point is over text.
    @discardableResult
    func beginSelection(at point: CGPoint) -> Bool {
        selectionStart = (try? session.anchor(at: point)) ?? nil
        return selectionStart != nil
    }

    /// Extends the selection to a point of the view, as a drag does. Over
    /// the gap between paragraphs the selection keeps its last end.
    func extendSelection(to point: CGPoint) {
        guard let start = selectionStart, let end = (try? session.anchor(at: point)) ?? nil else { return }
        let range = TextRange.spanning(start, end)
        guard range != selection else { return }
        selection = range
        refreshMarks()
    }

    func clearSelection() {
        guard selection != nil else { return }
        selection = nil
        refreshMarks()
    }

    /// Selects the word under a point of the view, as a force click does,
    /// and reports whether there was one. The core counts offsets in
    /// non-whitespace characters, so the word is found in the text around
    /// the point and its ends are counted back into offsets.
    @discardableResult
    func selectWord(at point: CGPoint) -> Bool {
        guard let anchor = (try? session.anchor(at: point)) ?? nil else { return false }
        let before = min(anchor.offset, 40)
        let around = TextRange(start: Anchor(chapter: anchor.chapter, offset: anchor.offset - before),
                               end: Anchor(chapter: anchor.chapter, offset: anchor.offset + 40))
        guard let text = try? session.text(of: around),
              let word = Self.word(in: text, atCharacter: before) else { return false }
        let start = anchor.offset - before + word.lowerBound
        selection = TextRange(start: Anchor(chapter: anchor.chapter, offset: start),
                              end: Anchor(chapter: anchor.chapter, offset: anchor.offset - before + word.upperBound))
        refreshMarks()
        return true
    }

    /// Returns the word holding the character at index, counted in
    /// non-whitespace characters of text, as a range of such counts.
    static func word(in text: String, atCharacter index: Int) -> Range<Int>? {
        let characters = text.filter { !$0.isWhitespace }.map { $0 }
        // Positions of the non-whitespace characters, with the gaps between
        // words, so a word ends at whitespace as well as at punctuation.
        var gaps = Set<Int>()
        var count = 0
        for character in text {
            if character.isWhitespace { gaps.insert(count) } else { count += 1 }
        }
        func isWord(_ i: Int) -> Bool {
            let c = characters[i]
            return c.isLetter || c.isNumber || c == "'" || c == "\u{2019}" || c == "-"
        }
        guard characters.indices.contains(index), isWord(index) else { return nil }
        var low = index
        while low > 0, !gaps.contains(low), isWord(low - 1) { low -= 1 }
        var high = index + 1
        while high < characters.count, !gaps.contains(high), isWord(high) { high += 1 }
        return low..<high
    }

    /// Shows the system dictionary for the selection.
    func lookUpSelection() {
        guard let text = selectedText(), !text.isEmpty, let box = selectionRects().first else { return }
        lookUpHandler?(text, box)
    }

    /// Opens the translation popover for the selection.
    func translateSelection() {
        guard let text = selectedText(), !text.isEmpty else { return }
        translationText = text
    }

    /// Returns the selected text, or nil when nothing is selected.
    func selectedText() -> String? {
        guard let selection else { return nil }
        return try? session.text(of: selection)
    }

    /// Returns the boxes of the selection on screen, in view points.
    func selectionRects() -> [CGRect] {
        marks.filter { $0.kind == .selection }.map(\.rect)
    }

    // MARK: Annotations

    func reloadAnnotations() {
        do {
            annotations = try library.annotations(of: book.id)
        } catch {
            NSLog("Mole: cannot read annotations of %@: %@", book.title, error.localizedDescription)
        }
        refreshMarks()
    }

    var bookmarks: [Annotation] { annotations.filter { $0.kind == .bookmark } }

    /// Highlights are the highlighted passages, with or without a note.
    var highlights: [Annotation] { annotations.filter { $0.kind == .highlight } }

    /// Reports whether a bookmark falls on the visible pages.
    var isBookmarked: Bool {
        guard let visibleRange else { return false }
        return bookmarks.contains { visibleRange.contains($0.range.start) }
    }

    /// Bookmarks the visible pages, or removes their bookmarks.
    func toggleBookmark() throws {
        guard let visibleRange else { return }
        let here = bookmarks.filter { visibleRange.contains($0.range.start) }
        if here.isEmpty {
            let text = (try? session.text(of: visibleRange)) ?? ""
            let bookmark = Annotation(kind: .bookmark,
                                      range: TextRange(start: visibleRange.start, end: visibleRange.start),
                                      color: 0, note: "", text: Self.excerpt(text, limit: 160))
            _ = try library.add(bookmark, to: book.id)
        } else {
            for bookmark in here { try library.remove(bookmark) }
        }
        reloadAnnotations()
    }

    /// Highlights the selection in color and returns the new highlight.
    @discardableResult
    func highlightSelection(color: Int, note: String = "") throws -> Annotation? {
        guard let selection else { return nil }
        // A selection over an existing highlight recolors it instead.
        if var existing = highlights.first(where: { $0.range == selection }) {
            existing.color = color
            if !note.isEmpty { existing.note = note }
            try library.update(existing)
            clearSelection()
            reloadAnnotations()
            return existing
        }
        let text = (try? session.text(of: selection)) ?? ""
        let highlight = try library.add(Annotation(kind: .highlight, range: selection, color: color, note: note,
                                                   text: Self.excerpt(text, limit: 1000)), to: book.id)
        clearSelection()
        reloadAnnotations()
        return highlight
    }

    /// Saves a changed color or note.
    func update(_ annotation: Annotation) throws {
        try library.update(annotation)
        reloadAnnotations()
    }

    func remove(_ annotation: Annotation) throws {
        try library.remove(annotation)
        reloadAnnotations()
    }

    /// Returns the highlight under a point of the view, if any.
    func highlight(at point: CGPoint) -> Annotation? {
        guard let anchor = (try? session.anchor(at: point)) ?? nil else { return nil }
        return highlights.last { $0.range.contains(anchor) }
    }

    /// Returns the picture under a point of the view, if any.
    func picture(at point: CGPoint) -> PagePicture? {
        (try? session.picture(at: point)) ?? nil
    }

    /// Returns text shortened to about limit characters at a word boundary.
    static func excerpt(_ text: String, limit: Int) -> String {
        let flat = text.split(whereSeparator: \.isNewline).joined(separator: " ")
        guard flat.count > limit else { return flat }
        let cut = flat.prefix(limit)
        let end = cut.lastIndex(of: " ") ?? cut.endIndex
        return String(cut[..<end]) + "…"
    }

    // MARK: Search

    /// Searches the book for query, replacing any search before it. An empty
    /// query clears the search.
    func find(_ query: String) {
        let query = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard query != self.query else { return }
        stopSearch()
        self.query = query
        hits = []
        currentHit = nil
        searchTruncated = false
        guard !query.isEmpty else {
            searchDone = true
            refreshMarks()
            return
        }
        do {
            search = try session.search(query)
            searchDone = false
            searchPoll = Timer.scheduledTimer(withTimeInterval: 0.1, repeats: true) { [weak self] _ in
                MainActor.assumeIsolated { self?.pollSearch() }
            }
            pollSearch()
        } catch {
            searchDone = true
            NSLog("Mole: cannot search %@: %@", book.title, error.localizedDescription)
        }
    }

    private func pollSearch() {
        guard let search else { return }
        do {
            let results = try search.results()
            if results.hits.count != hits.count {
                hits = results.hits
                refreshMarks()
            }
            searchTruncated = results.truncated
            if results.done {
                searchDone = true
                stopPolling()
            }
        } catch {
            searchDone = true
            stopPolling()
        }
    }

    private func stopPolling() {
        searchPoll?.invalidate()
        searchPoll = nil
    }

    private func stopSearch() {
        stopPolling()
        search = nil
    }

    /// Shows hit index and marks it as the current one.
    func showHit(_ index: Int) {
        guard hits.indices.contains(index) else { return }
        currentHit = index
        go(to: hits[index].range.start)
        refreshMarks()
    }

    /// Shows the next (or, with step -1, previous) hit after the current one,
    /// or after the visible pages when none is current.
    func showHit(step: Int) {
        guard !hits.isEmpty else { return }
        let next: Int
        if let currentHit {
            next = (currentHit + step + hits.count) % hits.count
        } else if let start = visibleRange?.start {
            let after = hits.firstIndex { $0.range.start >= start } ?? 0
            next = step > 0 ? after : (after - 1 + hits.count) % hits.count
        } else {
            next = 0
        }
        showHit(next)
    }

    /// Ends the session's work before the tab goes away.
    func close() {
        stopSearch()
        session.close()
    }
}

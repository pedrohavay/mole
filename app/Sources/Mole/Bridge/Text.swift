import Foundation
import MoleCore

/// Anchor is a place in a book's text that no relayout moves: a chapter and
/// a count of the non-whitespace characters before it in that chapter.
struct Anchor: Codable, Hashable, Comparable {
    let chapter: Int
    let offset: Int

    static func < (a: Anchor, b: Anchor) -> Bool {
        (a.chapter, a.offset) < (b.chapter, b.offset)
    }

    /// EndOfBook is a place past the end of any book.
    static let endOfBook = Anchor(chapter: Int(Int32.max), offset: 0)

    /// Reports whether scalar takes up an offset, as the core counts them:
    /// whitespace, invisible format characters and hyphens do not.
    static func counts(_ scalar: Unicode.Scalar) -> Bool {
        if scalar.properties.isWhitespace || scalar.properties.generalCategory == .format { return false }
        switch scalar {
        case "-", "\u{00AD}", "\u{2010}", "\u{2011}": return false
        default: return true
        }
    }

    /// Returns the number of characters of text that take up an offset.
    static func count<Text: StringProtocol>(in text: Text) -> Int {
        text.unicodeScalars.reduce(0) { $0 + (counts($1) ? 1 : 0) }
    }
}

/// TextRange is a stretch of text from start up to, not including, end.
struct TextRange: Codable, Hashable {
    let start: Anchor
    let end: Anchor

    /// Returns the range between two anchors in either order, end included,
    /// as a drag from one character to another selects it.
    static func spanning(_ a: Anchor, _ b: Anchor) -> TextRange {
        let (first, last) = a <= b ? (a, b) : (b, a)
        return TextRange(start: first, end: Anchor(chapter: last.chapter, offset: last.offset + 1))
    }

    var isEmpty: Bool { start >= end }

    /// Reports whether the two ranges share any text.
    func overlaps(_ other: TextRange) -> Bool {
        start < other.end && other.start < end
    }

    /// Reports whether anchor lies in the range; an empty range holds only
    /// its start.
    func contains(_ anchor: Anchor) -> Bool {
        isEmpty ? anchor == start : (start <= anchor && anchor < end)
    }
}

/// TextRect is a box of text on screen, in view points from the top left.
private struct TextRect: Decodable {
    let x, y, width, height: Double
}

/// Paragraph is a paragraph of a book, or the part of one on a page.
struct Paragraph: Decodable, Hashable {
    let range: TextRange
    /// Text has whitespace collapsed to single spaces. Counting its
    /// characters as Anchor does gives the offsets of range.
    let text: String
    /// Continued is set when the paragraph goes on from the page before, so
    /// that it is the rest of the Paragraph before it.
    let continued: Bool
}

extension Session {
    /// Returns the anchor of the character at point, in view points from the
    /// top left, or nil when the point is not over text.
    func anchor(at point: CGPoint) throws -> Anchor? {
        try checkOpen()
        return try CoreCall.decode(Anchor?.self, mole_session_hit_test(handle, point.x, point.y))
    }

    /// Returns the range of text on the visible pages.
    func visibleRange() throws -> TextRange {
        try checkOpen()
        return try CoreCall.decode(TextRange.self, mole_session_visible_range(handle))
    }

    /// Returns the text of range, on screen or not, with paragraphs on
    /// their own lines.
    func text(of range: TextRange) throws -> String {
        try checkOpen()
        let json = try CoreCall.encode(range)
        struct Text: Decodable { let text: String }
        return try CoreCall.withCString(json) { try CoreCall.decode(Text.self, mole_session_range_text(handle, $0)) }.text
    }

    /// Returns the boxes of the visible part of range, one per line, in view
    /// points from the top left.
    func rects(of range: TextRange) throws -> [CGRect] {
        try checkOpen()
        let json = try CoreCall.encode(range)
        let rects = try CoreCall.withCString(json) {
            try CoreCall.decode([TextRect].self, mole_session_range_rects(handle, $0))
        }
        return rects.map { CGRect(x: $0.x, y: $0.y, width: $0.width, height: $0.height) }
    }

    /// Returns the text of range a paragraph at a time, on screen or not.
    /// A positive limit stops at the end of the paragraph that brings the
    /// characters returned to limit.
    func paragraphs(of range: TextRange, limit: Int = 0) throws -> [Paragraph] {
        try checkOpen()
        let json = try CoreCall.encode(range)
        return try CoreCall.withCString(json) {
            try CoreCall.decode([Paragraph].self, mole_session_paragraphs(handle, $0, Int64(limit)))
        }
    }

    /// Shows the screen that holds anchor; it reports whether the page changed.
    @discardableResult
    func go(to anchor: Anchor) throws -> Bool {
        try checkOpen()
        let json = try CoreCall.encode(anchor)
        return CoreCall.withCString(json) { mole_session_go_to_anchor(handle, $0) } == 1
    }

    /// Returns the page, from 0, that holds anchor in the current layout.
    func page(of anchor: Anchor) throws -> Int {
        try checkOpen()
        let json = try CoreCall.encode(anchor)
        let page = CoreCall.withCString(json) { mole_session_anchor_page(handle, $0) }
        guard page >= 0 else { throw CoreCall.lastError() }
        return Int(page)
    }

    /// Starts searching the whole book for query in the background.
    func search(_ query: String) throws -> Search {
        try checkOpen()
        let handle = CoreCall.withCString(query) { mole_session_search(self.handle, $0) }
        guard handle != 0 else { throw CoreCall.lastError() }
        return Search(handle: handle)
    }
}

/// Search is a search of a book running in the background. Read its hits
/// with results(); releasing it stops the search.
@MainActor
final class Search {
    private let handle: Int64

    /// Hit is a match: where it is and a line of text around it.
    struct Hit: Decodable, Hashable {
        let range: TextRange
        let excerpt: String
        /// MatchStart and matchLength locate the match in excerpt, in UTF-16
        /// code units.
        let matchStart: Int
        let matchLength: Int
    }

    /// Results are the hits found so far, in reading order.
    struct Results: Decodable {
        let done: Bool
        /// Truncated is set when the search stopped at its maximum of hits.
        let truncated: Bool
        let hits: [Hit]
    }

    fileprivate init(handle: Int64) {
        self.handle = handle
    }

    deinit {
        mole_search_close(handle)
    }

    func results() throws -> Results {
        try CoreCall.decode(Results.self, mole_search_results(handle))
    }
}

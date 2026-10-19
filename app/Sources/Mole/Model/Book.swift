import Foundation

/// Book is a book of the library, as the core lists it.
struct Book: Decodable, Identifiable, Hashable {
    let id: Int64
    let path: String
    /// OriginalPath is where the file was imported from, when the core keeps
    /// its own copy at path.
    let originalPath: String?
    let title: String
    let author: String
    /// Fraction is the share of the book read, from 0 to 1.
    let fraction: Double
    /// OpenedAt and addedAt are Unix times in seconds; openedAt is 0 for a
    /// book never opened.
    let openedAt: Int64
    let addedAt: Int64

    /// A book counts as finished from this fraction on, so that a few pages
    /// of index or ads do not keep it on the reading shelf.
    static let finishedAt = 0.98

    var lastOpened: Date? { openedAt == 0 ? nil : Date(timeIntervalSince1970: TimeInterval(openedAt)) }
    var isFinished: Bool { fraction >= Self.finishedAt }
}

/// Shelf filters the library.
enum Shelf: String, CaseIterable, Identifiable {
    case all, reading, unread, finished

    var id: String { rawValue }

    var title: String { L10n.text("shelf.\(rawValue)") }

    var symbol: String {
        switch self {
        case .all: "books.vertical"
        case .reading: "book"
        case .unread: "book.closed"
        case .finished: "checkmark.circle"
        }
    }

    /// Reports whether book belongs on the shelf.
    func keeps(_ book: Book) -> Bool {
        switch self {
        case .all: true
        case .reading: book.lastOpened != nil && !book.isFinished
        case .unread: book.lastOpened == nil
        case .finished: book.isFinished
        }
    }
}

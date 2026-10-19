import SwiftUI

/// Annotation is a bookmark or a highlight of a book; a highlight with a
/// note is what the interface calls a note.
struct Annotation: Codable, Identifiable, Hashable {
    enum Kind: String, Codable {
        case bookmark, highlight
    }

    var id: Int64 = 0
    var kind: Kind
    var range: TextRange
    /// Color indexes HighlightColor.all; it is 0 for bookmarks.
    var color: Int
    var note: String
    /// Text is the excerpt saved with the annotation, so lists show it
    /// without opening the book.
    var text: String
    var createdAt: Int64 = 0
    var updatedAt: Int64 = 0

    var hasNote: Bool { !note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
}

/// HighlightColor is a color for highlighted text: a swatch for the
/// interface and a dim light for the page, where a full color would outshine
/// the letters above it.
struct HighlightColor {
    let key: String
    /// Swatch is the color in the interface, in sRGB.
    let swatch: Color
    /// Light is the mark painted under the text, in linear Display P3.
    let light: SIMD3<Float>

    var name: String { L10n.text("highlight.\(key)") }

    static let all = [
        HighlightColor(key: "yellow", swatch: Color(hex: 0xF5C451), light: [0.16, 0.12, 0.02]),
        HighlightColor(key: "green", swatch: Color(hex: 0x3DD68C), light: [0.03, 0.13, 0.06]),
        HighlightColor(key: "blue", swatch: Color(hex: 0x5B8DEF), light: [0.03, 0.07, 0.17]),
        HighlightColor(key: "pink", swatch: Color(hex: 0xE5689B), light: [0.16, 0.04, 0.09]),
        HighlightColor(key: "violet", swatch: Color(hex: 0x9D7BEA), light: [0.09, 0.05, 0.17]),
    ]

    /// Returns the color at index, wrapping out-of-range values.
    static func at(_ index: Int) -> HighlightColor {
        all[((index % all.count) + all.count) % all.count]
    }
}

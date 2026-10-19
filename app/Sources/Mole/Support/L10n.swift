import Foundation

/// L10n looks up user-facing text in the app's string tables,
/// Resources/<language>.lproj/Localizable.strings. Foundation picks the table
/// from the user's preferred languages and falls back to English.
enum L10n {
    /// Bundle holds the string tables. Inside Mole.app the SwiftPM resource
    /// bundle lives in Contents/Resources; under `swift run` and `swift test`
    /// SwiftPM's own accessor finds it next to the executable.
    static let bundle: Bundle = {
        if let url = Bundle.main.url(forResource: "Mole_Mole", withExtension: "bundle"),
           let bundle = Bundle(url: url) {
            return bundle
        }
        return .module
    }()

    /// Returns the text for key.
    static func text(_ key: String) -> String {
        bundle.localizedString(forKey: key, value: nil, table: nil)
    }

    /// Returns the text for key with printf-style arguments filled in.
    static func format(_ key: String, _ arguments: CVarArg...) -> String {
        String(format: text(key), locale: .current, arguments: arguments)
    }

    /// Returns "1 book" or "3 books" in the current language.
    static func bookCount(_ count: Int) -> String {
        count == 1 ? text("count.books.one") : format("count.books.other", count)
    }
}

import Foundation
import XCTest
@testable import Mole

/// LocalizationTests keep the string tables complete: every language has the
/// same keys, the same format specifiers, and every key the code uses.
final class LocalizationTests: XCTestCase {
    private static let languages = ["en", "pt-BR"]

    /// Returns the string table of language. SwiftPM lowercases the folder
    /// names (pt-br.lproj), which Foundation matches regardless of case.
    private func table(_ language: String) throws -> [String: String] {
        let folders = try FileManager.default.contentsOfDirectory(at: L10n.bundle.resourceURL ?? L10n.bundle.bundleURL,
                                                                  includingPropertiesForKeys: nil)
        let folder = try XCTUnwrap(folders.first { $0.lastPathComponent.lowercased() == "\(language.lowercased()).lproj" },
                                   "no \(language).lproj in \(L10n.bundle.bundlePath)")
        let url = folder.appendingPathComponent("Localizable.strings")
        return try XCTUnwrap(NSDictionary(contentsOf: url) as? [String: String])
    }

    func testLanguagesHaveTheSameKeys() throws {
        let english = try table("en")
        XCTAssertFalse(english.isEmpty)
        for language in Self.languages.dropFirst() {
            let other = try table(language)
            XCTAssertEqual(Set(english.keys).subtracting(other.keys).sorted(), [], "missing in \(language)")
            XCTAssertEqual(Set(other.keys).subtracting(english.keys).sorted(), [], "only in \(language)")
        }
    }

    func testFormatSpecifiersMatch() throws {
        let english = try table("en")
        for language in Self.languages.dropFirst() {
            let other = try table(language)
            for (key, value) in english {
                guard let translated = other[key] else { continue }
                XCTAssertEqual(specifiers(translated), specifiers(value), "\(language): \(key)")
            }
        }
    }

    func testEveryKeyInTheCodeExists() throws {
        let english = try table("en")
        var keys = Set(try literalKeys())
        keys.formUnion(Shelf.allCases.flatMap { ["shelf.\($0.rawValue)", "shelf.empty.\($0.rawValue)"] })
        keys.formUnion(Tint.all.map { "tint.\($0.key)" })
        keys.formUnion(Stroke.all.map { "stroke.\($0.key)" })
        keys.formUnion(Accent.presets.map { "accent.\($0.key)" })
        keys.formUnion(HighlightColor.all.map { "highlight.\($0.key)" })
        keys.formUnion(PanelPage.allCases.map { "panel.\($0.rawValue)" })
        keys.formUnion(PictureMode.allCases.map { "pictures.\(String(describing: $0))" })
        keys.formUnion(LineSpacing.allCases.map { "spacing.\(String(describing: $0))" })
        keys.formUnion(PageMargins.allCases.map { "margins.\(String(describing: $0))" })
        keys.formUnion((Shortcut.reading + Shortcut.window).map(\.action))
        XCTAssertGreaterThan(keys.count, 50)
        XCTAssertEqual(keys.subtracting(english.keys).sorted(), [])
    }

    /// Returns the keys passed as literals to L10n in the app's sources.
    private func literalKeys() throws -> [String] {
        let sources = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/Mole")
        let pattern = try NSRegularExpression(pattern: #"L10n\.(?:text|format)\("([A-Za-z0-9.]+)""#)
        var keys: [String] = []
        let files = FileManager.default.enumerator(at: sources, includingPropertiesForKeys: nil)
        while let file = files?.nextObject() as? URL {
            guard file.pathExtension == "swift" else { continue }
            let text = try String(contentsOf: file, encoding: .utf8)
            for match in pattern.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
                if let range = Range(match.range(at: 1), in: text) { keys.append(String(text[range])) }
            }
        }
        return keys
    }

    private func specifiers(_ format: String) -> [String] {
        let pattern = try! NSRegularExpression(pattern: #"%(?:%|l?[d@f])"#)
        return pattern.matches(in: format, range: NSRange(format.startIndex..., in: format)).compactMap {
            Range($0.range, in: format).map { String(format[$0]) }
        }
    }
}

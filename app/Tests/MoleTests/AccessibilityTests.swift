import AppKit
import SwiftUI
import XCTest
@testable import Mole

final class AnchorCountTests: XCTestCase {
    func testCountsAsTheCore() {
        // Whitespace, invisible format characters and hyphens take no
        // offset; letters, punctuation and dashes do.
        XCTAssertEqual(Anchor.count(in: "Ação é coração;"), 13)
        XCTAssertEqual(Anchor.count(in: "well-known\u{00A0}x\u{200B}"), 10)
        XCTAssertEqual(Anchor.count(in: "a\u{00AD}b \u{2014} c\n"), 4)
        XCTAssertEqual(Anchor.count(in: ""), 0)
    }
}

@MainActor
final class ReadAloudTests: XCTestCase {
    /// Returns a paragraph of chapter starting at offset.
    private func paragraph(_ chapter: Int, _ offset: Int, _ text: String, continued: Bool = false) -> Paragraph {
        Paragraph(range: TextRange(start: Anchor(chapter: chapter, offset: offset),
                                   end: Anchor(chapter: chapter, offset: offset + Anchor.count(in: text))),
                  text: text, continued: continued)
    }

    func testSplitsParagraphsIntoSentencesOfTheBook() {
        // The second paragraph part goes on from the page before.
        let first = paragraph(0, 10, "First sentence here. Second one")
        let paragraphs = [first, paragraph(0, first.range.end.offset, "goes on. Third!", continued: true),
                          paragraph(1, 0, "Chapter two.")]
        let sentences = ReadAloud.sentences(in: paragraphs)
        XCTAssertEqual(sentences.map(\.text), ["First sentence here.", "Second one goes on.", "Third!", "Chapter two."])
        XCTAssertEqual(sentences.map(\.range), [
            TextRange(start: Anchor(chapter: 0, offset: 10), end: Anchor(chapter: 0, offset: 28)),
            TextRange(start: Anchor(chapter: 0, offset: 28), end: Anchor(chapter: 0, offset: 44)),
            TextRange(start: Anchor(chapter: 0, offset: 44), end: Anchor(chapter: 0, offset: 50)),
            TextRange(start: Anchor(chapter: 1, offset: 0), end: Anchor(chapter: 1, offset: 11)),
        ])
        XCTAssertEqual(sentences.map(\.endsParagraph), [false, false, true, true])

        // Words map back to the book, across the page break too.
        let second = sentences[1]
        let utf16 = Array(second.text.utf16)
        func offset(of word: String) -> Int {
            let target = Array(word.utf16)
            return (0...(utf16.count - target.count)).first { Array(utf16[$0..<$0 + target.count]) == target } ?? -1
        }
        XCTAssertEqual(second.anchor(at: offset(of: "one")), Anchor(chapter: 0, offset: 34))
        XCTAssertEqual(second.anchor(at: offset(of: "goes")), Anchor(chapter: 0, offset: 37))
        XCTAssertEqual(second.anchor(at: offset(of: "on.")), Anchor(chapter: 0, offset: 41))
    }

    func testSkipsTextWithoutCountedCharacters() {
        XCTAssertEqual(ReadAloud.sentences(in: []).count, 0)
        XCTAssertEqual(ReadAloud.sentences(in: [paragraph(0, 0, "Only this.")]).map(\.text), ["Only this."])
    }

    func testLanguageOfACode() {
        XCTAssertEqual(ReadAloud.language(of: "pt-BR"), "pt")
        XCTAssertEqual(ReadAloud.language(of: "en_US"), "en")
        XCTAssertEqual(ReadAloud.language(of: "zh-Hans"), "zh")
    }

    func testPicksAVoiceInTheBooksLanguage() throws {
        guard !ReadAloud.voices(in: ["en"]).isEmpty else { throw XCTSkip("no English voice is installed") }
        XCTAssertEqual(ReadAloud.voice(for: "en", picked: "").map { ReadAloud.language(of: $0.language) }, "en")
        // A voice picked in Settings reads the books in its language only.
        guard let portuguese = ReadAloud.voices(in: ["pt"]).first else { return }
        XCTAssertEqual(ReadAloud.voice(for: "pt", picked: portuguese.identifier)?.identifier, portuguese.identifier)
        let english = ReadAloud.voice(for: "en", picked: portuguese.identifier)
        XCTAssertEqual(english.map { ReadAloud.language(of: $0.language) }, "en")
    }
}

final class PageParagraphTests: XCTestCase {
    func testFrameHoldsTheLines() {
        let range = TextRange(start: Anchor(chapter: 0, offset: 0), end: Anchor(chapter: 0, offset: 9))
        let paragraph = PageParagraph(range: range, text: "Two lines",
                                      lines: [CGRect(x: 40, y: 10, width: 200, height: 20),
                                              CGRect(x: 20, y: 30, width: 100, height: 20)])
        XCTAssertEqual(paragraph.frame, CGRect(x: 20, y: 10, width: 220, height: 40))
    }
}

final class ContrastTests: XCTestCase {
    /// Returns the sRGB components of color drawn with appearance.
    private func components(_ color: Color, _ appearance: NSAppearance) -> [CGFloat] {
        var result: [CGFloat] = []
        appearance.performAsCurrentDrawingAppearance {
            if let rgb = NSColor(color).usingColorSpace(.sRGB) {
                result = [rgb.redComponent, rgb.greenComponent, rgb.blueComponent, rgb.alphaComponent]
            }
        }
        return result
    }

    func testTokensFollowIncreaseContrast() throws {
        let dark = try XCTUnwrap(NSAppearance(named: .darkAqua))
        let normal = Color(hex: 0x808080)
        let increased = Color(hex: 0xC0C0C0)
        let off = Theme.contrasting(normal, increased: increased) { _ in false }
        let on = Theme.contrasting(normal, increased: increased) { _ in true }
        XCTAssertEqual(components(off, dark), components(normal, dark))
        XCTAssertEqual(components(on, dark), components(increased, dark))
        // Without the setting the dark appearance keeps the normal tokens.
        if !NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast {
            XCTAssertFalse(Theme.isContrastIncreased(for: dark))
            XCTAssertEqual(components(Theme.secondary, dark), components(Color(hex: 0x8E8E95), dark))
        }
    }
}

@MainActor
final class MotionTests: XCTestCase {
    func testRunsTheChangeEitherWay() {
        var changed = false
        Motion.animate { changed = true }
        XCTAssertTrue(changed)
    }
}

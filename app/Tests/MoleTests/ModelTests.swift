import AppKit
import SwiftUI
import XCTest
@testable import Mole

final class ShelfTests: XCTestCase {
    private func book(fraction: Double, opened: Bool) -> Book {
        Book(id: 1, path: "/b.epub", originalPath: nil, title: "B", author: "", fraction: fraction,
             openedAt: opened ? 1_700_000_000 : 0, addedAt: 1_600_000_000)
    }

    func testShelves() {
        let unread = book(fraction: 0, opened: false)
        let reading = book(fraction: 0.4, opened: true)
        let finished = book(fraction: 0.99, opened: true)
        XCTAssertEqual(Shelf.allCases.filter { $0.keeps(unread) }, [.all, .unread])
        XCTAssertEqual(Shelf.allCases.filter { $0.keeps(reading) }, [.all, .reading])
        XCTAssertEqual(Shelf.allCases.filter { $0.keeps(finished) }, [.all, .finished])
    }
}

final class LaunchArgumentsTests: XCTestCase {
    func testParse() {
        let parsed = LaunchArguments.parse(["Mole", "/a/One.EPUB", "-NSDocumentRevisionsDebugMode", "YES",
                                            "--verify-dir", "/tmp/out", "/b/two.epub"])
        XCTAssertEqual(parsed, LaunchArguments(books: ["/a/One.EPUB", "/b/two.epub"], verifyDirectory: "/tmp/out"))
    }

    func testParseFormats() {
        let parsed = LaunchArguments.parse(["Mole", "/a/b.mobi", "/a/c.PRC", "/a/d.fb2", "/a/e.txt", "/a/f.text",
                                            "/a/g.PDF", "/a/h.azw", "/a/notes"])
        XCTAssertEqual(parsed.books, ["/a/b.mobi", "/a/c.PRC", "/a/d.fb2", "/a/e.txt", "/a/f.text", "/a/g.PDF"])
    }

    func testSmokeTest() {
        XCTAssertTrue(LaunchArguments.parse(["Mole", "--smoke-test"]).smokeTest)
        XCTAssertFalse(LaunchArguments.parse(["Mole"]).smokeTest)
    }

    func testVerifyDirWithoutValue() {
        XCTAssertNil(LaunchArguments.parse(["Mole", "--verify-dir"]).verifyDirectory)
    }
}

final class ColorTests: XCTestCase {
    func testHexRoundTrip() throws {
        for preset in Accent.presets {
            let color = try XCTUnwrap(Color(hexString: preset.hex))
            XCTAssertEqual(color.hexString, preset.hex)
        }
    }

    func testRejectsMalformedHex() {
        for text in ["", "#", "#12345", "#1234567", "#GGGGGG", "red"] {
            XCTAssertNil(Color(hexString: text), text)
        }
    }
}

final class ReaderSettingsTests: XCTestCase {
    func testDecodesTheCoreJSON() throws {
        let json = ##"{"level":0.5,"edr":true,"tint":1,"gamma":2.2,"fontSize":21,"spread":2,"accent":"#3E63DD"}"##
        let settings = try JSONDecoder().decode(ReaderSettings.self, from: Data(json.utf8))
        XCTAssertEqual(settings.spread, .double)
        XCTAssertEqual(settings.accent, "#3E63DD")
        // A library that predates the typography gets the defaults.
        XCTAssertEqual(settings.typography, Typography(font: "", lineSpacing: .normal, margins: .normal, justify: false))
        XCTAssertEqual(try JSONDecoder().decode(ReaderSettings.self, from: JSONEncoder().encode(settings)), settings)
    }

    func testDecodesTheTypography() throws {
        let json = ##"{"level":0.5,"edr":true,"tint":1,"gamma":2.2,"fontSize":21,"spread":0,"accent":"","pictures":0,"bookColors":false,"font":"Georgia","lineSpacing":2,"margins":1,"justify":true}"##
        let settings = try JSONDecoder().decode(ReaderSettings.self, from: Data(json.utf8))
        XCTAssertEqual(settings.typography, Typography(font: "Georgia", lineSpacing: .relaxed, margins: .narrow, justify: true))
        let style = try JSONSerialization.jsonObject(with: JSONEncoder().encode(settings.typography)) as? [String: Any]
        XCTAssertEqual(style?["font"] as? String, "Georgia")
        XCTAssertEqual(style?["lineSpacing"] as? Int, 2)
        XCTAssertEqual(style?["margins"] as? Int, 1)
        XCTAssertEqual(style?["justify"] as? Bool, true)
    }

    func testOffersInstalledFonts() {
        // These come with every macOS 14 install.
        XCTAssertTrue(ReaderFont.installed.contains("Georgia"), "\(ReaderFont.installed)")
        XCTAssertTrue(ReaderFont.installed.contains("Charter"), "\(ReaderFont.installed)")
    }

    func testDecodesPicturesAndBookColors() throws {
        let old = ##"{"level":0.5,"edr":true,"tint":1,"gamma":2.2,"fontSize":21,"spread":2,"accent":"","images":1}"##
        let defaults = try JSONDecoder().decode(ReaderSettings.self, from: Data(old.utf8))
        XCTAssertEqual(defaults.pictures, .night)
        XCTAssertFalse(defaults.bookColors)

        let json = ##"{"level":0.5,"edr":true,"tint":1,"gamma":2.2,"fontSize":21,"spread":2,"accent":"","pictures":1,"bookColors":true}"##
        let settings = try JSONDecoder().decode(ReaderSettings.self, from: Data(json.utf8))
        XCTAssertEqual(settings.pictures, .original)
        XCTAssertTrue(settings.bookColors)
        let encoded = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(settings)) as? [String: Any])
        XCTAssertEqual(encoded["pictures"] as? Int, 1)
        XCTAssertEqual(encoded["bookColors"] as? Bool, true)
    }

    func testPictureModes() {
        // The raw values are the core's; the cases are in the order of the controls.
        XCTAssertEqual(PictureMode.allCases, [.original, .night, .hidden])
        XCTAssertEqual(PictureMode.allCases.map(\.rawValue), [1, 0, 2])
        XCTAssertEqual(PictureMode.night.gain(for: 6), 1)
        XCTAssertEqual(PictureMode.original.gain(for: 0.4), 0.4)
        XCTAssertEqual(PictureMode.hidden.gain(for: 6), 0)
    }

    func testDecodesPicture() throws {
        let json = ##"{"page":4,"index":1,"rect":{"x":10,"y":20,"width":300,"height":180}}"##
        let picture = try JSONDecoder().decode(PagePicture.self, from: Data(json.utf8))
        XCTAssertEqual(picture, PagePicture(page: 4, index: 1, rect: CGRect(x: 10, y: 20, width: 300, height: 180)))
        let none = try JSONDecoder().decode(Optional<PagePicture>.self, from: Data("null".utf8))
        XCTAssertNil(none)
    }

    func testTintWraps() {
        XCTAssertEqual(Tint.at(Tint.all.count).key, Tint.all[0].key)
        XCTAssertEqual(Tint.at(-1).key, Tint.all.last?.key)
    }
}

final class WarmUpTests: XCTestCase {
    func testWarmUpEndsWhenHeadroomPeaks() {
        var warmUp = WarmUp()
        let start = Date()
        XCTAssertFalse(warmUp.sample(Headroom(current: 2, potential: 16), now: start))
        XCTAssertTrue(warmUp.sample(Headroom(current: 15.9, potential: 16), now: start.addingTimeInterval(0.1)))
        XCTAssertFalse(warmUp.isWarming)
    }

    func testWarmUpEndsWhenHeadroomStopsRising() {
        var warmUp = WarmUp()
        let start = Date()
        XCTAssertFalse(warmUp.sample(Headroom(current: 4, potential: 16), now: start))
        XCTAssertTrue(warmUp.sample(Headroom(current: 4, potential: 16), now: start.addingTimeInterval(0.2)))
    }
}

final class ReadingPaceTests: XCTestCase {
    func testLearnsFromPlausibleScreens() {
        var pace = ReadingPace()
        pace.record(characters: 1000, seconds: 30)
        XCTAssertEqual(pace.samples, 1)
        XCTAssertEqual(pace.secondsPerCharacter, 0.03, accuracy: 1e-9)
        pace.record(characters: 1000, seconds: 60)
        XCTAssertEqual(pace.secondsPerCharacter, 0.03 + 0.15 * 0.03, accuracy: 1e-9)
    }

    func testIgnoresSkimsPausesAndShortScreens() {
        var pace = ReadingPace()
        pace.record(characters: 1000, seconds: 1)
        pace.record(characters: 1000, seconds: 3600)
        pace.record(characters: 50, seconds: 20)
        XCTAssertEqual(pace.samples, 0)
        XCTAssertEqual(pace.secondsPerCharacter, ReadingPace.defaultSecondsPerCharacter)
    }

    func testFormatsWholeMinutes() {
        XCTAssertFalse(TimeLeft.format(10).isEmpty)
        XCTAssertNotEqual(TimeLeft.format(12 * 60), TimeLeft.format(4 * 3600 + 30 * 60))
    }
}

@MainActor
final class WordTests: XCTestCase {
    func testFindsTheWordAroundACharacter() {
        // Offsets count non-whitespace characters: "Look" is 0..<4, "up," 4..<7.
        let text = "Look up, then read on."
        XCTAssertEqual(ReaderTab.word(in: text, atCharacter: 1), 0..<4)
        XCTAssertEqual(ReaderTab.word(in: text, atCharacter: 5), 4..<6)
        XCTAssertNil(ReaderTab.word(in: text, atCharacter: 6))
        XCTAssertEqual(ReaderTab.word(in: "don\u{2019}t stop", atCharacter: 2), 0..<5)
    }
}

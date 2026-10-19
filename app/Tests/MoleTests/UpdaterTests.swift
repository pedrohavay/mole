import XCTest
@testable import Mole

/// UpdaterTests check when the updater turns itself on. The tests never
/// start Sparkle, which would reach the network.
final class UpdaterTests: XCTestCase {
    private let feed = "https://github.com/pedrohavay/mole/releases/latest/download/appcast.xml"
    private let key = "8w1NZg90/BkCRWSxJBC94wi5rtGvKFMucJz9cnLgHlU="

    func testNeedsAnHTTPSFeedAndAKey() {
        XCTAssertTrue(Updater.isConfigured(["SUFeedURL": feed, "SUPublicEDKey": key]))
        // Development and CI builds: a feed and no key.
        XCTAssertFalse(Updater.isConfigured(["SUFeedURL": feed]))
        XCTAssertFalse(Updater.isConfigured(["SUFeedURL": feed, "SUPublicEDKey": ""]))
        XCTAssertFalse(Updater.isConfigured(["SUPublicEDKey": key]))
        XCTAssertFalse(Updater.isConfigured(["SUFeedURL": "http://example.com/appcast.xml", "SUPublicEDKey": key]))
        // A key must be 32 bytes of base64.
        XCTAssertFalse(Updater.isConfigured(["SUFeedURL": feed, "SUPublicEDKey": "not a key"]))
        XCTAssertFalse(Updater.isConfigured(["SUFeedURL": feed, "SUPublicEDKey": "AAAA"]))
    }

    @MainActor
    func testStaysOffWithoutAKey() {
        let updater = Updater(info: ["SUFeedURL": feed], start: true)
        XCTAssertFalse(updater.isEnabled)
        XCTAssertFalse(updater.canCheck)
        updater.checkForUpdates()
    }

    @MainActor
    func testDoesNotStartWhenAsked() {
        let updater = Updater(info: ["SUFeedURL": feed, "SUPublicEDKey": key], start: false)
        XCTAssertTrue(updater.isEnabled)
        XCTAssertFalse(updater.canCheck)
        updater.checksAutomatically = true
        updater.checkForUpdates()
    }
}

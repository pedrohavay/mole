import Foundation
import XCTest
@testable import Mole

/// FixedPagesTests read a PDF through a ReaderTab: its pages zoom and
/// scroll instead of reflowing.
@MainActor
final class FixedPagesTests: XCTestCase {
    private var directory: URL!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("MoleTests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try FileManager.default.removeItem(at: directory)
    }

    /// Returns a tab on a PDF of three pages, laid out one page at a time in
    /// a view of 800 x 500 points.
    private func openPDF() throws -> ReaderTab {
        let library = try Library(path: directory.appendingPathComponent("library.db").path)
        let file = directory.appendingPathComponent("pages.pdf")
        try PDFFixture.data(pages: 3).write(to: file)
        let id = try library.importBook(at: file.path)
        let book = try XCTUnwrap(try library.books().first { $0.id == id })
        let tab = ReaderTab(book: book, session: try Session(library: library, book: id), library: library)
        try tab.layout(width: 800, height: 500, scale: 1, fontSize: 18, spread: .single,
                       typography: Typography(font: "", lineSpacing: .normal, margins: .normal, justify: false))
        addTeardownBlock { @MainActor in tab.close() }
        return tab
    }

    func testZoomAndScroll() throws {
        let tab = try openPDF()
        XCTAssertTrue(tab.isFixedLayout)
        XCTAssertEqual(tab.state.pages, 3)
        var viewport = try XCTUnwrap(tab.state.viewport)
        XCTAssertEqual(viewport.fit, .page)
        XCTAssertFalse(viewport.isScrollable)
        XCTAssertGreaterThan(tab.state.characters, 1000)

        tab.zoom(to: .width)
        viewport = try XCTUnwrap(tab.state.viewport)
        XCTAssertEqual(viewport.fit, .width)
        XCTAssertGreaterThan(viewport.maxY, 0)
        tab.pan(dx: 0, dy: -.greatestFiniteMagnitude)

        // A screen at a time to the bottom of the page, then the next page,
        // from its top.
        let revision = tab.revision
        tab.page(1, screen: 400)
        XCTAssertEqual(tab.state.page, 0)
        XCTAssertEqual(tab.state.viewport?.y ?? 0, 400, accuracy: 0.5)
        XCTAssertGreaterThan(tab.revision, revision)
        while tab.state.page == 0 { tab.page(1, screen: 400) }
        XCTAssertEqual(tab.state.page, 1)
        XCTAssertEqual(tab.state.viewport?.y ?? -1, 0, accuracy: 0.5)

        // Back from the top of a page shows the bottom of the one before.
        tab.page(-1, screen: 400)
        XCTAssertEqual(tab.state.page, 0)
        viewport = try XCTUnwrap(tab.state.viewport)
        XCTAssertEqual(viewport.y, viewport.maxY, accuracy: 0.5)
        let top = try XCTUnwrap(tab.visibleRange?.start)
        XCTAssertFalse(tab.isOnScreen(top), "the first line is scrolled out of view")
        let shown = try XCTUnwrap(tab.topOfScreen)
        XCTAssertGreaterThan(shown, top)
        XCTAssertTrue(tab.isOnScreen(shown), "reading aloud starts from text in view")
        tab.go(to: top)
        XCTAssertTrue(tab.isOnScreen(top))

        tab.zoom(by: 1)
        XCTAssertEqual(tab.state.viewport?.fit, .scale)
        XCTAssertGreaterThan(tab.state.viewport?.scale ?? 0, viewport.scale)
        for _ in 0..<20 { tab.zoom(by: -1) }
        XCTAssertEqual(tab.state.viewport?.fit, .page)
    }
}

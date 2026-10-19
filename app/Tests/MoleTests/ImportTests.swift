import Foundation
import XCTest
@testable import Mole

/// ImportTests add books through the bridge to the core, into a library in a
/// temporary directory.
final class ImportTests: XCTestCase {
    private var directory: URL!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("MoleTests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try FileManager.default.removeItem(at: directory)
    }

    /// Returns a MOBI marked as encrypted: a Palm database with one record,
    /// whose PalmDOC header names an encryption, which is all the core reads
    /// before refusing the book.
    private func protectedMOBI() -> Data {
        var data = Data(count: 78)
        data.replaceSubrange(60..<68, with: Data("BOOKMOBI".utf8))
        data[77] = 1 // one record, which starts at byte 88
        data.append(contentsOf: [0, 0, 0, 88, 0, 0, 0, 0, 0, 0])
        var record = Data(count: 16)
        record[1] = 2 // PalmDOC compression
        record[13] = 2 // Mobipocket encryption
        data.append(record)
        return data
    }

    func testProtectedBookFailsWithDRM() throws {
        let library = try Library(path: directory.appendingPathComponent("library.db").path)
        let book = directory.appendingPathComponent("Locked.mobi")
        try protectedMOBI().write(to: book)
        XCTAssertThrowsError(try library.importBook(at: book.path)) { error in
            let core = error as? CoreError
            XCTAssertEqual(core?.kind, .drm)
            XCTAssertEqual(core?.errorDescription, L10n.text("error.book.drm"))
        }
    }

    func testTextBook() throws {
        let library = try Library(path: directory.appendingPathComponent("library.db").path)
        let book = directory.appendingPathComponent("notes.txt")
        try Data("Title: Plain Words\nAuthor: Ana Lima\n\nHello.\n".utf8).write(to: book)
        let id = try library.importBook(at: book.path)
        let added = try XCTUnwrap(try library.books().first { $0.id == id })
        XCTAssertEqual(added.title, "Plain Words")
        XCTAssertEqual(added.author, "Ana Lima")
        XCTAssertEqual((added.path as NSString).pathExtension, "txt")
        XCTAssertNotNil(library.cover(of: id))
    }
}

import Foundation
import XCTest
@testable import Mole

final class CollectionBridgeTests: XCTestCase {
    private var directory: URL!

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("MoleTests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: directory)
    }

    func testCollections() throws {
        let library = try Library(path: directory.appendingPathComponent("library.db").path)
        let essays = try library.createCollection(named: "Essays")
        let art = try library.createCollection(named: "Art")
        XCTAssertThrowsError(try library.createCollection(named: "essays"))
        try library.rename(collection: art, to: "Pictures")
        XCTAssertEqual(try library.collections().map(\.name), ["Essays", "Pictures"])
        XCTAssertThrowsError(try library.add(book: 999, to: essays))
        try library.delete(collection: essays)
        XCTAssertEqual(try library.collections().map(\.id), [art])
        XCTAssertEqual(try library.collections().first?.folder, "")
    }

    func testWatchedFolders() throws {
        let library = try Library(path: directory.appendingPathComponent("library.db").path)
        let shelf = directory.appendingPathComponent("Shelf")
        try FileManager.default.createDirectory(at: shelf, withIntermediateDirectories: true)
        let first = try library.importFolder(at: shelf.path, collections: true, watch: true)
        XCTAssertEqual(first.found, 0)
        XCTAssertNotEqual(first.folder, 0)
        let folders = try library.folders()
        XCTAssertEqual(folders.map(\.id), [first.folder])
        XCTAssertEqual(folders.first?.path, FolderWatcher.realPath(shelf.path))
        XCTAssertEqual(folders.first?.collections, true)
        XCTAssertNotNil(folders.first?.lastSynced)
        XCTAssertEqual(try library.sync(folder: first.folder).folder, first.folder)
        try library.remove(folder: first.folder)
        XCTAssertEqual(try library.folders().count, 0)
        XCTAssertThrowsError(try library.sync(folder: first.folder))
        XCTAssertThrowsError(try library.importFolder(at: shelf.appendingPathComponent("gone").path,
                                                      collections: false, watch: false))
    }
}

final class BookFilesTests: XCTestCase {
    func testSort() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("MoleTests-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: directory) }
        let folder = directory.appendingPathComponent("Shelf")
        let unpacked = directory.appendingPathComponent("Unpacked.epub")
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: unpacked, withIntermediateDirectories: true)
        let book = directory.appendingPathComponent("Book.EPUB")
        let notes = directory.appendingPathComponent("notes.txt")
        for file in [book, notes] { try Data().write(to: file) }
        let gone = directory.appendingPathComponent("gone.epub").path

        let sorted = BookFiles.sort([book.path, notes.path, folder.path, unpacked.path, gone])
        XCTAssertEqual(sorted.files, [book.path, unpacked.path])
        XCTAssertEqual(sorted.folders, [folder.path])
    }
}

final class FolderWatcherTests: XCTestCase {
    func testPathIsIn() {
        XCTAssertTrue(FolderWatcher.path("/books/a.epub", isIn: "/books"))
        XCTAssertTrue(FolderWatcher.path("/books", isIn: "/books"))
        XCTAssertTrue(FolderWatcher.path("/books/x/a.epub", isIn: "/books/"))
        XCTAssertFalse(FolderWatcher.path("/books2/a.epub", isIn: "/books"))
    }

    func testRealPathKeepsPrivate() {
        XCTAssertEqual(FolderWatcher.realPath("/tmp"), "/private/tmp")
        XCTAssertEqual(FolderWatcher.realPath("/no/such/folder"), "/no/such/folder")
    }
}

final class CollectionDecodingTests: XCTestCase {
    func testBookWithCollections() throws {
        let json = #"{"id":2,"path":"/b","originalPath":"/o","title":"B","author":"","fraction":0,"openedAt":0,"addedAt":2,"collections":[4,7],"missing":true}"#
        let book = try JSONDecoder().decode(Book.self, from: Data(json.utf8))
        XCTAssertEqual(book.collections, [4, 7])
        XCTAssertTrue(book.missing)
    }

    func testFolderImportResult() throws {
        let json = #"{"folder":1,"found":3,"added":[9],"updated":[],"missing":[2],"failed":[{"path":"/x.epub","error":"not a zip"}]}"#
        let result = try JSONDecoder().decode(FolderImportResult.self, from: Data(json.utf8))
        XCTAssertEqual(result.added, [9])
        XCTAssertEqual(result.missing, [2])
        XCTAssertEqual(result.failed.first?.path, "/x.epub")
    }

    func testVerifyFolderArgument() {
        let parsed = LaunchArguments.parse(["Mole", "--verify-dir", "/out", "--verify-folder", "/tree"])
        XCTAssertEqual(parsed.verifyFolder, "/tree")
        XCTAssertEqual(parsed.verifyDirectory, "/out")
    }
}

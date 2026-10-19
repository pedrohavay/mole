import Foundation
import MoleCore

/// Library is the book database: books, covers, reading positions,
/// settings and annotations. The core takes library calls from any thread,
/// one at a time per handle, so a Library is used from one thread at a time;
/// background work opens a Library of its own on the same database.
final class Library: @unchecked Sendable {
    let handle: Int64
    let path: String

    /// Opens or creates the library database at path.
    init(path: String) throws {
        let handle = CoreCall.withCString(path) { mole_library_open($0) }
        guard handle != 0 else { throw CoreCall.lastError() }
        self.handle = handle
        self.path = path
    }

    /// Returns the books, the most recently opened first.
    func books() throws -> [Book] {
        try CoreCall.decode([Book].self, mole_library_books(handle))
    }

    /// Returns the cover image data of a book, or nil if it has none.
    func cover(of book: Int64) -> Data? {
        var length: Int64 = 0
        guard let bytes = mole_library_cover(handle, book, &length) else { return nil }
        defer { mole_free(bytes) }
        return Data(bytes: bytes, count: Int(length))
    }

    /// Adds the book at path and returns its book id. Adding a file that is
    /// already in the library returns the existing book unchanged. A book
    /// protected by DRM fails with a CoreError of kind drm.
    @discardableResult
    func importBook(at path: String) throws -> Int64 {
        let id = CoreCall.withCString(path) { mole_library_import(handle, $0) }
        guard id != 0 else { throw CoreCall.lastError() }
        return id
    }

    /// Removes a book and its annotations from the library, and deletes the
    /// library's copy of its file. The file it was imported from is kept.
    func remove(_ book: Int64) throws {
        guard mole_library_remove(handle, book) == 1 else { throw CoreCall.lastError() }
    }

    /// Returns the saved settings, or the defaults.
    func settings() throws -> ReaderSettings {
        try CoreCall.decode(ReaderSettings.self, mole_library_settings(handle))
    }

    /// Saves the settings.
    func save(_ settings: ReaderSettings) throws {
        let json = try CoreCall.encode(settings)
        guard CoreCall.withCString(json, { mole_library_save_settings(handle, $0) }) == 1 else {
            throw CoreCall.lastError()
        }
    }
}

// MARK: Annotations

extension Library {
    /// Returns the bookmarks and highlights of book, in reading order.
    func annotations(of book: Int64) throws -> [Annotation] {
        try CoreCall.decode([Annotation].self, mole_library_annotations(handle, book))
    }

    /// Adds annotation to book and returns it with its id.
    func add(_ annotation: Annotation, to book: Int64) throws -> Annotation {
        let json = try CoreCall.encode(annotation)
        let id = CoreCall.withCString(json) { mole_library_add_annotation(handle, book, $0) }
        guard id != 0 else { throw CoreCall.lastError() }
        var added = annotation
        added.id = id
        return added
    }

    /// Saves the color and the note of annotation.
    func update(_ annotation: Annotation) throws {
        struct Changes: Encodable { let color: Int; let note: String }
        let json = try CoreCall.encode(Changes(color: annotation.color, note: annotation.note))
        guard CoreCall.withCString(json, { mole_library_update_annotation(handle, annotation.id, $0) }) == 1 else {
            throw CoreCall.lastError()
        }
    }

    func remove(_ annotation: Annotation) throws {
        guard mole_library_remove_annotation(handle, annotation.id) == 1 else { throw CoreCall.lastError() }
    }
}

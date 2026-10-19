import Foundation
import MoleCore

// MARK: Collections

extension Library {
    /// Returns the collections, by name.
    func collections() throws -> [BookCollection] {
        try CoreCall.decode([BookCollection].self, mole_library_collections(handle))
    }

    /// Makes an empty collection named name and returns its id.
    func createCollection(named name: String) throws -> Int64 {
        let id = CoreCall.withCString(name) { mole_library_create_collection(handle, $0) }
        guard id != 0 else { throw CoreCall.lastError() }
        return id
    }

    func rename(collection: Int64, to name: String) throws {
        guard CoreCall.withCString(name, { mole_library_rename_collection(handle, collection, $0) }) == 1 else {
            throw CoreCall.lastError()
        }
    }

    /// Deletes a collection. Its books stay in the library.
    func delete(collection: Int64) throws {
        guard mole_library_delete_collection(handle, collection) == 1 else { throw CoreCall.lastError() }
    }

    func add(book: Int64, to collection: Int64) throws {
        guard mole_library_add_to_collection(handle, collection, book) == 1 else { throw CoreCall.lastError() }
    }

    func remove(book: Int64, from collection: Int64) throws {
        guard mole_library_remove_from_collection(handle, collection, book) == 1 else { throw CoreCall.lastError() }
    }
}

// MARK: Watched folders

extension Library {
    /// Returns the watched folders, by path.
    func folders() throws -> [WatchedFolder] {
        try CoreCall.decode([WatchedFolder].self, mole_library_folders(handle))
    }

    /// Imports every book in the folder at path and its subfolders. With
    /// collections each book also goes into a collection named after its
    /// folder; with watch the folder is kept in sync from then on.
    func importFolder(at path: String, collections: Bool, watch: Bool) throws -> FolderImportResult {
        struct Options: Encodable { let collections: Bool; let watch: Bool }
        let options = try CoreCall.encode(Options(collections: collections, watch: watch))
        return try CoreCall.withCString(path) { path in
            try CoreCall.withCString(options) { options in
                try CoreCall.decode(FolderImportResult.self, mole_library_import_folder(handle, path, options))
            }
        }
    }

    /// Imports what is new or changed in a watched folder and marks the
    /// books whose file left it.
    func sync(folder: Int64) throws -> FolderImportResult {
        try CoreCall.decode(FolderImportResult.self, mole_library_sync_folder(handle, folder))
    }

    /// Stops watching a folder. Its books stay in the library.
    func remove(folder: Int64) throws {
        guard mole_library_remove_folder(handle, folder) == 1 else { throw CoreCall.lastError() }
    }
}

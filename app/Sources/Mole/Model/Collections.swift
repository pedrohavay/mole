import CoreTransferable
import Foundation
import UniformTypeIdentifiers

/// BookCollection is a named group of books, made by the user or from a
/// folder of books. A book may be in several.
struct BookCollection: Decodable, Identifiable, Hashable {
    let id: Int64
    let name: String
    /// Folder is the folder the collection was made from, or empty.
    let folder: String

    var symbol: String { folder.isEmpty ? "rectangle.stack" : "folder" }
}

/// WatchedFolder is a folder the library keeps in sync.
struct WatchedFolder: Decodable, Identifiable, Hashable {
    let id: Int64
    let path: String
    /// Collections is set when each folder in it that holds books is a
    /// collection.
    let collections: Bool
    /// AddedAt and syncedAt are Unix times in seconds; syncedAt is 0 before
    /// the first sync.
    let addedAt: Int64
    let syncedAt: Int64

    var name: String { (path as NSString).lastPathComponent }

    /// DisplayPath is the path with the home folder written as ~.
    var displayPath: String { (path as NSString).abbreviatingWithTildeInPath }

    var lastSynced: Date? { syncedAt == 0 ? nil : Date(timeIntervalSince1970: TimeInterval(syncedAt)) }
}

/// FolderImportResult is what a pass over a folder did, as the core reports
/// it.
struct FolderImportResult: Decodable, Sendable {
    struct Failure: Decodable, Sendable {
        let path: String
        let error: String
        /// DRM is true when the file is a book protected by DRM.
        let drm: Bool
    }

    /// Folder is the id of the watched folder, or 0 when it is not watched.
    let folder: Int64
    /// Found counts the books found in the folder.
    let found: Int
    let added: [Int64]
    /// Updated lists the books whose file changed, or was renamed or moved.
    let updated: [Int64]
    /// Missing lists the books whose file left the watched folder.
    let missing: [Int64]
    let failed: [Failure]
}

/// FolderImport asks how to add the books in folders the user chose. While
/// it is set, the window shows FolderImportSheet.
struct FolderImport: Identifiable {
    let id = UUID()
    let paths: [String]
}

/// BookDrag is a cover dragged from the library grid, to drop on a
/// collection.
struct BookDrag: Codable, Transferable {
    let id: Int64

    static var transferRepresentation: some TransferRepresentation {
        CodableRepresentation(contentType: .moleBook)
    }
}

extension UTType {
    /// MoleBook is a book of the library, dragged inside Mole.
    static let moleBook = UTType(exportedAs: "io.github.pedrohavay.mole.book")
}

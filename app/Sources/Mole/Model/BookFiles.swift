import Foundation
import UniformTypeIdentifiers

/// BookFiles names the files Mole reads, EPUB, MOBI and PRC, FB2, plain text
/// and PDF, and sorts paths the user hands to Mole into book files and
/// folders.
/// The core tells a book's format from its content; the extensions choose
/// which files the open panel, a drop and the command line take, so this is
/// the one place in the app that says which files are books.
enum BookFiles {
    /// Extensions are the file name extensions of books, in lowercase.
    static let extensions: Set<String> = ["epub", "mobi", "prc", "fb2", "txt", "text", "pdf"]

    /// Returns whether path names a book by its extension.
    static func accepts(_ path: String) -> Bool {
        extensions.contains((path as NSString).pathExtension.lowercased())
    }

    /// ContentTypes are the types the open panel offers: the ones the system
    /// knows for the extensions, which Mole's Info.plist declares when no
    /// other app does.
    static var contentTypes: [UTType] {
        extensions.sorted().compactMap { UTType(filenameExtension: $0) }
    }

    /// Returns the readable files and the folders among paths.
    static func sort(_ paths: [String]) -> (files: [String], folders: [String]) {
        var files: [String] = []
        var folders: [String] = []
        for path in paths {
            var isFolder: ObjCBool = false
            guard FileManager.default.fileExists(atPath: path, isDirectory: &isFolder) else { continue }
            let isBook = accepts(path)
            if isFolder.boolValue && !isBook {
                folders.append(path)
            } else if isBook {
                files.append(path)
            }
        }
        return (files, folders)
    }
}

import Foundation
import UniformTypeIdentifiers

/// BookFiles names the files Mole reads: EPUB, MOBI and PRC, FB2 and plain
/// text. The core tells a book's format from its content; the extensions
/// only choose which files the open panel, a drop and the command line take.
enum BookFiles {
    /// Extensions are the file name extensions of books, in lowercase.
    static let extensions = ["epub", "mobi", "prc", "fb2", "txt", "text"]

    /// Returns whether path names a book by its extension.
    static func accepts(_ path: String) -> Bool {
        extensions.contains((path as NSString).pathExtension.lowercased())
    }

    /// ContentTypes are the types the open panel offers: the ones the system
    /// knows for the extensions, which Mole's Info.plist declares when no
    /// other app does.
    static var contentTypes: [UTType] {
        extensions.compactMap { UTType(filenameExtension: $0) }
    }
}

import Foundation
import MoleCore

/// ReaderFont is a font the core can set the text of a book in, besides the
/// book's own fonts.
enum ReaderFont {
    /// Installed are the families of the fonts the core offers that this Mac
    /// has, in the order to show them. The core looks them up once.
    static let installed: [String] = {
        do {
            return try CoreCall.decode([String].self, mole_fonts())
        } catch {
            NSLog("Mole: cannot list the fonts: %@", error.localizedDescription)
            return []
        }
    }()
}

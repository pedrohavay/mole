import Foundation
import MoleCore

/// CoreError is a failure reported by the Go core.
struct CoreError: LocalizedError {
    /// Kind is what went wrong, for the failures the app explains in the
    /// user's language (mole_last_error_kind).
    enum Kind: Int32 {
        case other = 0
        case drm = 1
    }

    let message: String
    var kind = Kind.other

    var errorDescription: String? {
        switch kind {
        case .drm: L10n.text("error.book.drm")
        case .other: message
        }
    }
}

/// CoreCall holds the conventions of the Go core's C API (core/cmd/molecore):
/// a nil or zero result means failure and mole_last_error explains it; every
/// string or buffer the core returns is owned by the caller and released with
/// mole_free.
enum CoreCall {
    /// Returns the core's last error.
    static func lastError() -> CoreError {
        guard let message = mole_last_error() else { return CoreError(message: L10n.text("error.core.unknown")) }
        defer { mole_free(message) }
        return CoreError(message: String(cString: message), kind: CoreError.Kind(rawValue: mole_last_error_kind()) ?? .other)
    }

    /// Takes ownership of a string returned by the core.
    static func take(_ string: UnsafeMutablePointer<CChar>?) throws -> String {
        guard let string else { throw lastError() }
        defer { mole_free(string) }
        return String(cString: string)
    }

    /// Decodes a JSON string returned by the core.
    static func decode<T: Decodable>(_ type: T.Type, _ json: UnsafeMutablePointer<CChar>?) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(try take(json).utf8))
    }

    /// Encodes value as a JSON string for the core.
    static func encode<T: Encodable>(_ value: T) throws -> String {
        String(decoding: try JSONEncoder().encode(value), as: UTF8.self)
    }

    /// Calls body with string as a C string. Go declares every string
    /// parameter as `char *`, though the core never writes to one.
    static func withCString<R>(_ string: String, _ body: (UnsafeMutablePointer<CChar>) throws -> R) rethrows -> R {
        try string.withCString { try body(UnsafeMutablePointer(mutating: $0)) }
    }
}

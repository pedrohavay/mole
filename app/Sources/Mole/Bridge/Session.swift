import Foundation
import MoleCore

/// SessionState is where the reader is in an open book.
struct SessionState: Decodable, Equatable {
    var page = 0
    var lastVisible = 0
    var pages = 0
    var columns = 1
    var chapter = ""
    var progress = 0.0
}

/// Chapter is an entry of a book's table of contents, resolved to the page
/// where it starts in the current layout.
struct Chapter: Decodable, Hashable {
    let title: String
    let level: Int
    let page: Int
}

/// PixelLayer is a layer of the visible pages that the session renders: the
/// text mask, one byte of ink coverage per pixel, or a layer of four bytes
/// per pixel, RGBA in sRGB with premultiplied alpha. The session owns the
/// bytes, which stay valid until its next render, layout or close.
struct PixelLayer {
    let bytes: UnsafeRawPointer
    /// Stride is the length of a row in bytes.
    let stride: Int
    let width: Int
    let height: Int
}

/// Session is an open book. The core saves the reading position on every
/// page turn and again on close. Close it explicitly when the reader is done
/// with it; releasing an open session closes it as a last resort.
@MainActor
final class Session {
    let handle: Int64
    private(set) var isClosed = false

    /// Opens a book of library at its saved position.
    init(library: Library, book: Int64) throws {
        let handle = mole_session_open(library.handle, book)
        guard handle != 0 else { throw CoreCall.lastError() }
        self.handle = handle
    }

    deinit {
        if !isClosed { mole_session_close(handle) }
    }

    /// Saves the position and releases the book. Calling it again does
    /// nothing; no other method may be called after it.
    func close() {
        guard !isClosed else { return }
        isClosed = true
        mole_session_close(handle)
    }

    /// Reflows the book for a view of width x height points at scale pixels
    /// per point with the text set in typography, keeping the reader at the
    /// same place in the text.
    func layout(width: Double, height: Double, scale: Double, fontSize: Double, spread: Spread,
                typography: Typography) throws {
        try checkOpen()
        let json = try CoreCall.encode(typography)
        let done = CoreCall.withCString(json) {
            mole_session_layout(handle, width, height, 0, scale, fontSize, Int32(spread.rawValue), $0)
        }
        guard done == 1 else { throw CoreCall.lastError() }
    }

    /// Moves by screens screens of pages; it reports whether the page changed.
    @discardableResult
    func turn(_ screens: Int) -> Bool { !isClosed && mole_session_turn(handle, Int32(screens)) == 1 }

    /// Shows page first; it reports whether the page changed.
    @discardableResult
    func go(to page: Int) -> Bool { !isClosed && mole_session_go_to(handle, Int32(page)) == 1 }

    func state() throws -> SessionState {
        try checkOpen()
        return try CoreCall.decode(SessionState.self, mole_session_state(handle))
    }

    func chapters() throws -> [Chapter] {
        try checkOpen()
        return try CoreCall.decode([Chapter].self, mole_session_chapters(handle))
    }

    /// Sets how the next renders draw the pages; it reports whether that
    /// changed, which calls for a new render.
    func setStyle(pictures: PictureMode, bookColors: Bool) throws -> Bool {
        try checkOpen()
        let changed = mole_session_set_style(handle, Int32(pictures.rawValue), bookColors ? 1 : 0)
        guard changed >= 0 else { throw CoreCall.lastError() }
        return changed == 1
    }

    /// Renders the visible pages into the session's coverage mask.
    func render() throws -> PixelLayer {
        try checkOpen()
        var stride: Int64 = 0, width: Int64 = 0, height: Int64 = 0
        guard let bytes = mole_session_render(handle, &stride, &width, &height) else { throw CoreCall.lastError() }
        return PixelLayer(bytes: UnsafeRawPointer(bytes), stride: Int(stride), width: Int(width), height: Int(height))
    }

    /// Returns the pictures of the last render, RGBA on black, or nil when
    /// none is on screen.
    func imageLayer() throws -> PixelLayer? {
        try layer(mole_session_image_layer)
    }

    /// Returns the hues of the colored text of the last render, RGBA with
    /// the coverage of that text as alpha, or nil when there is none or the
    /// style does not keep the book's colors.
    func textColors() throws -> PixelLayer? {
        try layer(mole_session_text_colors)
    }

    private typealias LayerCall = (Int64, UnsafeMutablePointer<Int64>?, UnsafeMutablePointer<Int64>?,
                                   UnsafeMutablePointer<Int64>?, UnsafeMutablePointer<Int32>?) -> UnsafeMutableRawPointer?

    private func layer(_ call: LayerCall) throws -> PixelLayer? {
        try checkOpen()
        var stride: Int64 = 0, width: Int64 = 0, height: Int64 = 0
        var empty: Int32 = 0
        guard let bytes = call(handle, &stride, &width, &height, &empty) else { throw CoreCall.lastError() }
        guard empty == 0 else { return nil }
        return PixelLayer(bytes: UnsafeRawPointer(bytes), stride: Int(stride), width: Int(width), height: Int(height))
    }

    /// SwiftUI can still hold a closed session's tab for a moment after it
    /// closes; calls in that window fail instead of reaching the core.
    func checkOpen() throws {
        if isClosed { throw CoreError(message: L10n.text("error.session.closed")) }
    }
}

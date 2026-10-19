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
    /// Characters counts the non-whitespace characters on the visible pages.
    var characters = 0
    /// Fixed is set for a book of fixed pages, such as a PDF: its pages are
    /// scaled and scrolled rather than reflowed, and the typography does not
    /// apply.
    var fixed = false
    /// Viewport is how the pages of a book of fixed pages sit in the view,
    /// once it is laid out.
    var viewport: Viewport?

    init() {}

    // The counts and the fixed pages arrived after the first release of the
    // core's state.
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        page = try values.decode(Int.self, forKey: .page)
        lastVisible = try values.decode(Int.self, forKey: .lastVisible)
        pages = try values.decode(Int.self, forKey: .pages)
        columns = try values.decode(Int.self, forKey: .columns)
        chapter = try values.decode(String.self, forKey: .chapter)
        progress = try values.decode(Double.self, forKey: .progress)
        characters = try values.decodeIfPresent(Int.self, forKey: .characters) ?? 0
        fixed = try values.decodeIfPresent(Bool.self, forKey: .fixed) ?? false
        viewport = try values.decodeIfPresent(Viewport.self, forKey: .viewport)
    }

    private enum CodingKeys: String, CodingKey {
        case page, lastVisible, pages, columns, chapter, progress, characters, fixed, viewport
    }
}

/// Fit is how the pages of a book of fixed pages fit the view. The raw
/// values are the core's.
enum Fit: Int, Decodable {
    /// Whole pages in view.
    case page = 0
    /// Pages as wide as the view.
    case width = 1
    /// Pages at a scale of their own.
    case scale = 2
}

/// Viewport is how the visible pages of a book of fixed pages sit in the
/// view, in view points.
struct Viewport: Decodable, Equatable {
    var fit: Fit
    /// Scale is the view points per point of the page the pages are drawn at.
    var scale: Double
    var fitPage: Double
    var fitWidth: Double
    /// Width and height are the size of the pages side by side.
    var width: Double
    var height: Double
    /// X and y are how far the view is scrolled into pages larger than it,
    /// up to maxX and maxY.
    var x: Double
    var y: Double
    var maxX: Double
    var maxY: Double

    /// Percent is the scale as a percentage of the pages' printed size.
    var percent: Int { Int((scale * 100).rounded()) }

    /// IsScrollable reports whether the pages are larger than the view.
    var isScrollable: Bool { maxX > 0.5 || maxY > 0.5 }
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

    /// Sets how large the pages of a book of fixed pages are drawn, at fit
    /// or, with .scale, at scale view points per point of the page; it
    /// reports whether the view changed.
    @discardableResult
    func setZoom(_ fit: Fit, scale: Double = 0) throws -> Bool {
        try checkOpen()
        let changed = mole_session_set_zoom(handle, Int32(fit.rawValue), scale)
        guard changed >= 0 else { throw CoreCall.lastError() }
        return changed == 1
    }

    /// Zooms the pages of a book of fixed pages in by steps, or out when
    /// steps is negative; it reports whether the view changed.
    @discardableResult
    func zoom(by steps: Int) throws -> Bool {
        try checkOpen()
        let changed = mole_session_zoom_step(handle, Int32(steps))
        guard changed >= 0 else { throw CoreCall.lastError() }
        return changed == 1
    }

    /// Scrolls the view within pages of a book of fixed pages larger than
    /// it, by dx, dy points; a positive dy shows what is further down. It
    /// reports whether the view moved.
    @discardableResult
    func pan(dx: Double, dy: Double) throws -> Bool {
        try checkOpen()
        let moved = mole_session_pan(handle, dx, dy)
        guard moved >= 0 else { throw CoreCall.lastError() }
        return moved == 1
    }

    func state() throws -> SessionState {
        try checkOpen()
        return try CoreCall.decode(SessionState.self, mole_session_state(handle))
    }

    /// Returns the language the book declares, normally a BCP 47 tag such as
    /// "en" or "pt-BR", or nil when it declares none.
    func language() throws -> String? {
        try checkOpen()
        struct Language: Decodable { let language: String }
        let language = try CoreCall.decode(Language.self, mole_session_language(handle)).language
        return language.isEmpty ? nil : language
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

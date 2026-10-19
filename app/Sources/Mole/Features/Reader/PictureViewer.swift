import AppKit
import Observation
import SwiftUI

/// PictureViewer shows a picture of the book full size over the page, in its
/// colors and at SDR brightness: pinch or ⌘+ and ⌘− to zoom, ⌘0 to fit,
/// Esc to close.
@MainActor
struct PictureViewer: View {
    let image: CGImage
    let close: () -> Void
    @State private var zoom = PictureZoom()

    var body: some View {
        ZStack(alignment: .top) {
            ZoomingImage(image: image, zoom: zoom, close: close)
                .accessibilityElement()
                .accessibilityLabel(L10n.text("picture.title"))
                .accessibilityAddTraits(.isImage)
            controls
                .padding(.top, Theme.Space.m)
        }
        .background(Theme.page)
    }

    private var controls: some View {
        HStack(spacing: Theme.Space.xxs) {
            IconButton(symbol: "minus.magnifyingglass", help: L10n.text("picture.zoomOut")) { zoom.zoom(by: 1 / PictureZoom.step) }
            Text(L10n.format("picture.scale", Int((zoom.scale * 100).rounded())))
                .font(Theme.digits)
                .foregroundStyle(Theme.secondary)
                .frame(minWidth: 44)
            IconButton(symbol: "plus.magnifyingglass", help: L10n.text("picture.zoomIn")) { zoom.zoom(by: PictureZoom.step) }
            IconButton(symbol: "arrow.up.left.and.down.right.magnifyingglass", help: L10n.text("picture.fit"),
                       active: zoom.isFitted) { zoom.fit() }
            Divider().frame(height: 16).padding(.horizontal, Theme.Space.xs)
            IconButton(symbol: "xmark", help: L10n.text("picture.close"), action: close)
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(height: 32)
        .background(Capsule().fill(Theme.raised.opacity(0.92)))
        .overlay(Capsule().strokeBorder(Theme.border))
    }
}

/// PictureZoom connects the viewer's controls to its scroll view.
@Observable @MainActor
final class PictureZoom {
    /// Scale is the magnification against the picture's own pixels.
    fileprivate(set) var scale = 1.0
    /// IsFitted is set while the whole picture fits the view, as it opens.
    fileprivate(set) var isFitted = true
    @ObservationIgnored fileprivate weak var view: PictureScrollView?

    /// Step is how much one ⌘+ or ⌘− zooms.
    static let step = 1.25

    func zoom(by factor: Double) { view?.zoom(by: factor) }

    func fit() { view?.fit() }
}

/// ZoomingImage hosts the scroll view that shows and zooms the picture.
private struct ZoomingImage: NSViewRepresentable {
    let image: CGImage
    let zoom: PictureZoom
    let close: () -> Void

    func makeNSView(context: Context) -> PictureScrollView {
        let view = PictureScrollView(image: image)
        view.onClose = close
        view.onZoom = { [zoom] scale, fitted in
            zoom.scale = scale
            zoom.isFitted = fitted
        }
        zoom.view = view
        return view
    }

    func updateNSView(_ view: PictureScrollView, context: Context) {
        view.onClose = close
    }
}

/// PictureScrollView shows a picture centered on black and zooms it with a
/// pinch or the keys; it keeps the picture fitted to the view until the user
/// zooms. A magnification of 1 shows the picture's own pixels.
final class PictureScrollView: NSScrollView {
    var onClose: (() -> Void)?
    /// OnZoom receives the scale against the picture's pixels and whether
    /// the picture is fitted to the view.
    var onZoom: ((Double, Bool) -> Void)?
    private let imageView = NSImageView()
    private var isFitted = true
    private var magnifyObserver: NSObjectProtocol?

    /// MaxFit is how far fitting may enlarge a small picture.
    private static let maxFit = 4.0

    init(image: CGImage) {
        super.init(frame: .zero)
        contentView = CenteringClipView()
        backgroundColor = .black
        drawsBackground = true
        hasHorizontalScroller = true
        hasVerticalScroller = true
        autohidesScrollers = true
        scrollerStyle = .overlay
        allowsMagnification = true
        maxMagnification = 16
        imageView.image = NSImage(cgImage: image, size: NSSize(width: image.width, height: image.height))
        imageView.imageScaling = .scaleAxesIndependently
        imageView.frame = NSRect(x: 0, y: 0, width: image.width, height: image.height)
        documentView = imageView
        magnifyObserver = NotificationCenter.default.addObserver(
            forName: NSScrollView.didEndLiveMagnifyNotification, object: self, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated {
                self?.isFitted = false
                self?.reportZoom()
            }
        }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    deinit {
        if let magnifyObserver { NotificationCenter.default.removeObserver(magnifyObserver) }
    }

    override var acceptsFirstResponder: Bool { true }

    // NSScrollView passes the keys to its document, which takes none; the
    // viewer takes them itself.
    override func becomeFirstResponder() -> Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        // SwiftUI settles the responder chain after inserting the view, so
        // the keys are taken once it has.
        DispatchQueue.main.async { [weak self] in
            guard let self, let window = self.window else { return }
            window.makeFirstResponder(self)
        }
    }

    /// Pixel is the size of a picture's pixel in points at magnification 1.
    private var pixel: CGFloat { 1 / (window?.backingScaleFactor ?? 2) }

    override func layout() {
        super.layout()
        // The document is sized in points so that magnification 1 shows the
        // picture's own pixels on this screen.
        if let size = imageView.image?.size {
            let points = NSSize(width: size.width * pixel, height: size.height * pixel)
            if imageView.frame.size != points { imageView.setFrameSize(points) }
        }
        if isFitted { applyFit() }
    }

    /// FitMagnification is the magnification that shows the whole picture,
    /// enlarging a small one at most maxFit times.
    private var fitMagnification: CGFloat {
        let size = imageView.frame.size, view = contentSize
        guard size.width > 0, size.height > 0, view.width > 0, view.height > 0 else { return 1 }
        return min(view.width / size.width, view.height / size.height, Self.maxFit)
    }

    private func applyFit() {
        let fit = fitMagnification
        minMagnification = min(fit, 1) / 2
        if abs(magnification - fit) > 0.001 { magnification = fit }
        reportZoom()
    }

    func fit() {
        isFitted = true
        animator().magnification = fitMagnification
        minMagnification = min(fitMagnification, 1) / 2
        onZoom?(fitMagnification, true)
    }

    func zoom(by factor: Double) {
        isFitted = false
        let target = (magnification * CGFloat(factor)).clamped(to: minMagnification...maxMagnification)
        let center = NSPoint(x: documentVisibleRect.midX, y: documentVisibleRect.midY)
        setMagnification(target, centeredAt: center)
        reportZoom()
    }

    private func reportZoom() {
        onZoom?(magnification, isFitted)
    }

    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask).subtracting([.shift, .numericPad])
        guard flags == .command else { return super.performKeyEquivalent(with: event) }
        switch event.charactersIgnoringModifiers {
        case "=", "+": zoom(by: PictureZoom.step)
        case "-", "_": zoom(by: 1 / PictureZoom.step)
        case "0": fit()
        default: return super.performKeyEquivalent(with: event)
        }
        return true
    }

    override func keyDown(with event: NSEvent) {
        if event.keyCode == KeyCode.escape {
            onClose?()
        } else {
            super.keyDown(with: event)
        }
    }

    override func cancelOperation(_ sender: Any?) {
        onClose?()
    }
}

/// CenteringClipView keeps a document smaller than the view in its center.
private final class CenteringClipView: NSClipView {
    override func constrainBoundsRect(_ proposedBounds: NSRect) -> NSRect {
        var rect = super.constrainBoundsRect(proposedBounds)
        guard let document = documentView else { return rect }
        if rect.width > document.frame.width { rect.origin.x = (document.frame.width - rect.width) / 2 }
        if rect.height > document.frame.height { rect.origin.y = (document.frame.height - rect.height) / 2 }
        return rect
    }
}

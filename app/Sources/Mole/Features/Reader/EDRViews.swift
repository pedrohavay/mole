import AppKit
import SwiftUI

/// EDRMetalView is a view backed by an EDR CAMetalLayer. It forwards size
/// changes, keys, mouse input and the Copy command to its owner. Points are
/// in view coordinates from the top left, like the core's.
final class EDRMetalView: NSView, NSMenuItemValidation {
    var onResize: (() -> Void)?
    var onKey: ((NSEvent) -> Bool)?
    /// OnMouse receives a press, each drag step and the release.
    var onMouse: ((MouseStep, CGPoint, NSEvent) -> Void)?
    var onScroll: ((NSEvent) -> Void)?
    /// OnMenu returns the context menu for a point, if any.
    var onMenu: ((CGPoint) -> NSMenu?)?
    var onCopy: (() -> Void)?
    var canCopy: (() -> Bool)?

    enum MouseStep {
        case down, dragged, up
    }

    override init(frame: NSRect) {
        super.init(frame: frame)
        wantsLayer = true
        layerContentsRedrawPolicy = .never
        // Until the relayout after a resize, keep the last frame unscaled
        // instead of stretching the text.
        layerContentsPlacement = .topLeft
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.makeFirstResponder(self)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    override func makeBackingLayer() -> CALayer { EDRRenderer.makeLayer() }

    // makeBackingLayer always returns a CAMetalLayer.
    var metalLayer: CAMetalLayer { layer as! CAMetalLayer }

    override var acceptsFirstResponder: Bool { true }

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        onResize?()
    }

    override func viewDidChangeBackingProperties() {
        super.viewDidChangeBackingProperties()
        onResize?()
    }

    override func keyDown(with event: NSEvent) {
        if onKey?(event) != true { super.keyDown(with: event) }
    }

    override func mouseDown(with event: NSEvent) {
        window?.makeFirstResponder(self)
        onMouse?(.down, point(of: event), event)
    }

    override func mouseDragged(with event: NSEvent) {
        onMouse?(.dragged, point(of: event), event)
    }

    override func mouseUp(with event: NSEvent) {
        onMouse?(.up, point(of: event), event)
    }

    override func menu(for event: NSEvent) -> NSMenu? {
        onMenu?(point(of: event))
    }

    /// Returns where event happened, from the top left of the view. The view
    /// is not flipped, so that its layer shows the drawable upright.
    private func point(of event: NSEvent) -> CGPoint {
        let point = convert(event.locationInWindow, from: nil)
        return CGPoint(x: point.x, y: bounds.height - point.y)
    }

    @objc func copy(_ sender: Any?) {
        onCopy?()
    }

    func validateMenuItem(_ item: NSMenuItem) -> Bool {
        item.action == #selector(copy(_:)) ? canCopy?() ?? false : true
    }

    override func scrollWheel(with event: NSEvent) {
        onScroll?(event)
    }
}

/// ActionMenuItem is a menu item that runs a closure.
final class ActionMenuItem: NSMenuItem {
    private let run: () -> Void

    init(_ title: String, image: String? = nil, run: @escaping () -> Void) {
        self.run = run
        super.init(title: title, action: #selector(runAction), keyEquivalent: "")
        target = self
        if let image { self.image = NSImage(systemSymbolName: image, accessibilityDescription: nil) }
    }

    @available(*, unavailable)
    required init(coder: NSCoder) { fatalError("init(coder:) is not used") }

    @objc private func runAction() { run() }
}

/// EDRSentinel is a 1-point, fully transparent EDR layer. While it is on
/// screen macOS keeps the EDR headroom raised.
struct EDRSentinel: NSViewRepresentable {
    func makeNSView(context: Context) -> NSView {
        let layer = EDRRenderer.makeLayer(opaque: false)
        layer.drawableSize = CGSize(width: 2, height: 2)
        let view = NSView()
        // Setting the layer before wantsLayer makes a layer-hosting view,
        // which AppKit never draws into.
        view.layer = layer
        view.wantsLayer = true
        EDRRenderer.presentClear(on: layer)
        return view
    }

    func updateNSView(_ view: NSView, context: Context) {}
}

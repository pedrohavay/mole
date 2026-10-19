import AppKit
import SwiftUI

/// WindowChrome gives the main window its look: no title and a transparent
/// title bar the content draws under, so the tab strip is the title bar.
@MainActor
enum WindowChrome {
    static func apply(to window: NSWindow) {
        window.titleVisibility = .hidden
        window.titlebarAppearsTransparent = true
        window.styleMask.insert(.fullSizeContentView)
        window.tabbingMode = .disallowed
        window.collectionBehavior.insert(.fullScreenPrimary)
    }
}

/// TrafficLights keeps the window buttons centered in the top bar, which is
/// taller than a standard title bar. AppKit lays the buttons out again on
/// resizes and state changes, sometimes only some of them, so they are moved
/// back after each of those. The move waits for the end of the current
/// layout pass: moving a button inside it is undone by the rest of the pass.
///
/// A toolbar would make the title bar taller with no repositioning, but its
/// view sits above the content and takes the clicks meant for the tabs.
@MainActor
final class TrafficLights {
    private weak var window: NSWindow?
    private var observers: [NSObjectProtocol] = []
    private var placeScheduled = false
    private static let kinds: [NSWindow.ButtonType] = [.closeButton, .miniaturizeButton, .zoomButton]
    /// Inset is the distance from the window's left edge to the close button.
    private static let inset: CGFloat = 12
    private static let spacing: CGFloat = 20

    init(window: NSWindow) {
        self.window = window
        let center = NotificationCenter.default
        for kind in Self.kinds {
            guard let button = window.standardWindowButton(kind) else { continue }
            button.postsFrameChangedNotifications = true
            observers.append(center.addObserver(forName: NSView.frameDidChangeNotification, object: button,
                                                queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.schedulePlace() }
            })
        }
        for name in [NSWindow.didResizeNotification, NSWindow.didEndLiveResizeNotification,
                     NSWindow.didBecomeKeyNotification, NSWindow.didResignKeyNotification] {
            observers.append(center.addObserver(forName: name, object: window, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.schedulePlace() }
            })
        }
        place()
    }

    deinit {
        observers.forEach(NotificationCenter.default.removeObserver)
    }

    /// Places the buttons once the current layout pass is over; several
    /// requests in one pass place them once.
    func schedulePlace() {
        guard !placeScheduled else { return }
        placeScheduled = true
        DispatchQueue.main.async { [weak self] in
            MainActor.assumeIsolated {
                self?.placeScheduled = false
                self?.place()
            }
        }
    }

    /// Moves the buttons to the center of the bar. In full screen the
    /// buttons live in a separate title bar and are left alone.
    func place() {
        guard let window, !window.styleMask.contains(.fullScreen) else { return }
        for (index, kind) in Self.kinds.enumerated() {
            guard let button = window.standardWindowButton(kind), let superview = button.superview else { continue }
            let size = button.frame.size
            let target = NSRect(x: Self.inset + CGFloat(index) * Self.spacing,
                                y: window.frame.height - Theme.Size.bar / 2 - size.height / 2,
                                width: size.width, height: size.height)
            let origin = superview.convert(target, from: nil).origin
            if button.frame.origin != origin { button.setFrameOrigin(origin) }
        }
    }
}

/// WindowObserver configures the window it is placed in and reports whether
/// that window is in full screen.
struct WindowObserver: NSViewRepresentable {
    let onFullScreen: (Bool) -> Void

    func makeNSView(context: Context) -> ObserverView {
        ObserverView(onFullScreen: onFullScreen)
    }

    func updateNSView(_ view: ObserverView, context: Context) {}

    final class ObserverView: NSView {
        private let onFullScreen: (Bool) -> Void
        private var observers: [NSObjectProtocol] = []
        private var trafficLights: TrafficLights?

        init(onFullScreen: @escaping (Bool) -> Void) {
            self.onFullScreen = onFullScreen
            super.init(frame: .zero)
        }

        @available(*, unavailable)
        required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            observers.forEach(NotificationCenter.default.removeObserver)
            observers = []
            guard let window else {
                trafficLights = nil
                return
            }
            WindowChrome.apply(to: window)
            trafficLights = TrafficLights(window: window)
            // A window restored in full screen sends no notification.
            onFullScreen(window.styleMask.contains(.fullScreen))
            let center = NotificationCenter.default
            for (name, value) in [(NSWindow.didEnterFullScreenNotification, true),
                                  (NSWindow.didExitFullScreenNotification, false)] {
                observers.append(center.addObserver(forName: name, object: window, queue: .main) { [weak self] _ in
                    MainActor.assumeIsolated {
                        self?.onFullScreen(value)
                        self?.trafficLights?.schedulePlace()
                    }
                })
            }
        }
    }
}

/// WindowDragArea moves the window when dragged and, on a double click, does
/// what System Settings asks of a title bar: zoom, minimize or nothing.
struct WindowDragArea: NSViewRepresentable {
    final class DragView: NSView {
        override var mouseDownCanMoveWindow: Bool { true }

        override func mouseDown(with event: NSEvent) {
            guard event.clickCount == 2 else {
                window?.performDrag(with: event)
                return
            }
            switch UserDefaults.standard.string(forKey: "AppleActionOnDoubleClick") {
            case "Minimize": window?.performMiniaturize(nil)
            case "None": break
            default: window?.performZoom(nil)
            }
        }
    }

    func makeNSView(context: Context) -> NSView { DragView() }
    func updateNSView(_ view: NSView, context: Context) {}
}

import AppKit

/// ReaderCoordinator lays the book out for the view, holds the text back
/// until the EDR headroom stops rising, draws, and turns input into reading
/// commands.
@MainActor
final class ReaderCoordinator {
    let model: AppModel
    var tab: ReaderTab?
    private let renderer = try? EDRRenderer()
    private var laidOut: Layout?
    private var drawnRevision = -1
    private var pendingLayout: DispatchWorkItem?
    private var warmUp = WarmUp()
    /// WarmUpScheduled is set while a warm-up redraw is queued, so the draws
    /// that settings and headroom changes trigger do not queue more.
    private var warmUpScheduled = false
    private var swipe = Swipe()
    private var observers: [NSObjectProtocol] = []
    /// DragStart is where the pointer went down, until it goes up; dragged
    /// is set once it moved far enough to be a drag rather than a click.
    private var dragStart: CGPoint?
    private var dragged = false
    /// OnEdit opens the editor of a highlight that was clicked.
    var onEdit: ((Annotation, CGPoint) -> Void)?

    /// Layout is what the book was last laid out for.
    private struct Layout: Equatable {
        var size: CGSize
        var scale: CGFloat
        var fontSize: Double
        var spread: Spread
        var style: TextStyle
    }

    init(model: AppModel) {
        self.model = model
        // macOS lowers the headroom while the app is in the background and
        // raises it again on return; hold the text back until it settles.
        observers.append(NotificationCenter.default.addObserver(
            forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.warmUp = WarmUp() }
        })
    }

    deinit {
        observers.forEach(NotificationCenter.default.removeObserver)
    }

    /// Forgets the layout for a newly shown book. The text is held back only
    /// if the headroom is still rising.
    func reset() {
        laidOut = nil
        drawnRevision = -1
        warmUp = WarmUp(settled: model.headroom.isSettled)
    }

    // MARK: Layout and drawing

    /// Brings the view up to date after SwiftUI changed one of its inputs:
    /// settings apply at once, size changes once a live resize settles.
    func update(_ view: EDRMetalView) {
        if let laidOut, laidOut.size != view.bounds.size {
            scheduleLayout(view)
        } else {
            layoutIfNeeded(view)
        }
        draw(view)
    }

    /// Live resizes and column animations send many size changes; relayout
    /// once they settle.
    func scheduleLayout(_ view: EDRMetalView) {
        guard laidOut != nil else { return layoutIfNeeded(view) }
        pendingLayout?.cancel()
        let work = DispatchWorkItem { [weak self, weak view] in
            MainActor.assumeIsolated {
                guard let self, let view else { return }
                self.layoutIfNeeded(view)
                self.draw(view)
            }
        }
        pendingLayout = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.15, execute: work)
    }

    func layoutIfNeeded(_ view: EDRMetalView) {
        guard let tab, let window = view.window, view.bounds.width > 200, view.bounds.height > 200 else { return }
        let settings = model.settings
        let wanted = Layout(size: view.bounds.size, scale: window.backingScaleFactor,
                            fontSize: settings.fontSize, spread: settings.spread, style: settings.textStyle)
        guard wanted != laidOut else { return }
        do {
            try tab.layout(width: wanted.size.width, height: wanted.size.height, scale: wanted.scale,
                           fontSize: wanted.fontSize, spread: wanted.spread, style: wanted.style)
        } catch {
            NSLog("Mole: cannot lay out %@: %@", tab.book.title, error.localizedDescription)
            return
        }
        laidOut = wanted
        view.metalLayer.contentsScale = wanted.scale
        view.metalLayer.drawableSize = CGSize(width: (wanted.size.width * wanted.scale).rounded(),
                                              height: (wanted.size.height * wanted.scale).rounded())
        drawnRevision = -1
    }

    func draw(_ view: EDRMetalView) {
        guard let tab, let renderer, laidOut != nil else { return }
        if tab.revision != drawnRevision {
            do {
                renderer.upload(text: try tab.session.render())
                renderer.upload(image: try tab.session.imageMask())
                drawnRevision = tab.revision
            } catch {
                NSLog("Mole: cannot render %@: %@", tab.book.title, error.localizedDescription)
            }
        }
        if warmUp.isWarming {
            warm(view)
            return
        }
        let settings = model.settings
        renderer.present(on: view.metalLayer, rgb: Tint.at(settings.tint).rgb, gain: model.gain, gamma: settings.gamma,
                         imageGain: settings.images.gain(for: model.gain),
                         marks: lights(for: tab.marks, scale: laidOut?.scale ?? 1))
    }

    /// Turns the marks of the page into rectangles of light, in drawable
    /// pixels. Marks stay well below the text's brightness and dim with it.
    private func lights(for marks: [PageMark], scale: CGFloat) -> [Mark] {
        let dim = Float(min(model.gain, 1))
        let accent = model.accentLight
        return marks.map { mark in
            let color: SIMD3<Float>
            switch mark.kind {
            case .highlight(let index): color = HighlightColor.at(index).light
            case .selection: color = accent * 0.22
            case .hit(let current): color = current ? accent * 0.3 : SIMD3(repeating: 0.06)
            }
            let rect = CGRect(x: mark.rect.minX * scale, y: mark.rect.minY * scale,
                              width: mark.rect.width * scale, height: mark.rect.height * scale)
            return Mark(rect: rect.integral, color: color * dim)
        }
    }

    /// Keeps the page black until the headroom settles, then draws the text,
    /// so it appears at its final brightness instead of fading in.
    private func warm(_ view: EDRMetalView) {
        if warmUp.sample(model.headroom) {
            draw(view)
            return
        }
        guard !warmUpScheduled else { return }
        renderer?.present(on: view.metalLayer, rgb: .zero, gain: 0, gamma: 1)
        warmUpScheduled = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) { [weak self, weak view] in
            MainActor.assumeIsolated {
                guard let self else { return }
                self.warmUpScheduled = false
                if let view { self.draw(view) }
            }
        }
    }

    // MARK: Input

    /// Handles a key press in the page; it reports whether the key was used.
    func handle(_ event: NSEvent) -> Bool {
        guard let tab, event.modifierFlags.isDisjoint(with: [.command, .control, .option]) else { return false }
        let shift = event.modifierFlags.contains(.shift)
        switch (event.keyCode, event.charactersIgnoringModifiers?.lowercased() ?? "") {
        case (KeyCode.right, _), (KeyCode.pageDown, _): tab.turn(1)
        case (KeyCode.space, _): tab.turn(shift ? -1 : 1)
        case (KeyCode.left, _), (KeyCode.pageUp, _): tab.turn(-1)
        case (KeyCode.home, _): tab.goToStart()
        case (KeyCode.end, _): tab.goToEnd()
        case (KeyCode.up, _): model.adjustLevel(by: 1.25)
        case (KeyCode.down, _): model.adjustLevel(by: 0.8)
        case (KeyCode.escape, _) where tab.selection != nil: tab.clearSelection()
        case (KeyCode.escape, _) where model.isFullScreen: model.toggleFullScreen()
        case (_, "+"), (_, "="): model.adjustFontSize(by: 1)
        case (_, "-"), (_, "_"): model.adjustFontSize(by: -1)
        case (_, "e"): model.settings.edr.toggle()
        case (_, "c"): model.cycleTint()
        case (_, "f"): model.toggleFullScreen()
        case (_, "t"): model.showContents.toggle()
        default: return false
        }
        NSCursor.setHiddenUntilMouseMoves(true)
        return true
    }

    /// Handles the mouse in the page. A drag selects text; a click clears
    /// the selection, opens a highlight, or turns the page (back on the left
    /// third, forward elsewhere).
    func mouse(_ step: EDRMetalView.MouseStep, at point: CGPoint, event: NSEvent, in view: EDRMetalView) {
        guard let tab else { return }
        switch step {
        case .down:
            dragStart = point
            dragged = false
        case .dragged:
            guard let start = dragStart, dragged || hypot(point.x - start.x, point.y - start.y) > 3 else { return }
            dragged = true
            if !tab.isSelecting {
                // A drag that starts off the text selects nothing.
                guard tab.beginSelection(at: start) else { return }
                tab.isSelecting = true
            }
            tab.extendSelection(to: point)
        case .up:
            defer {
                dragStart = nil
                dragged = false
            }
            if tab.isSelecting {
                tab.isSelecting = false
            } else if dragged {
                // A drag off the text does nothing.
            } else if tab.selection != nil {
                tab.clearSelection()
            } else if let highlight = tab.highlight(at: point) {
                onEdit?(highlight, point)
            } else {
                tab.turn(point.x < view.bounds.width * 0.3 ? -1 : 1)
            }
        }
    }

    /// Returns the context menu for a point of the page.
    func menu(at point: CGPoint) -> NSMenu? {
        guard let tab else { return nil }
        let menu = NSMenu()
        if tab.selection != nil {
            menu.addItem(ActionMenuItem(L10n.text("action.copy"), image: "doc.on.doc") { [model] in
                model.copySelection()
            })
            let colors = NSMenu()
            for (index, color) in HighlightColor.all.enumerated() {
                colors.addItem(ActionMenuItem(color.name) { [model] in model.highlightSelection(color: index) })
            }
            let highlight = NSMenuItem(title: L10n.text("highlight.title"), action: nil, keyEquivalent: "")
            highlight.image = NSImage(systemSymbolName: "highlighter", accessibilityDescription: nil)
            highlight.submenu = colors
            menu.addItem(highlight)
            menu.addItem(ActionMenuItem(L10n.text("search.selection"), image: "magnifyingglass") { [model] in
                model.searchSelection()
            })
            menu.addItem(.separator())
        } else if let highlight = tab.highlight(at: point) {
            menu.addItem(ActionMenuItem(L10n.text("note.edit"), image: "note.text") { [weak self] in
                self?.onEdit?(highlight, point)
            })
            menu.addItem(ActionMenuItem(L10n.text("highlight.remove"), image: "trash") { [model] in
                do { try tab.remove(highlight) } catch {
                    model.alert(L10n.text("annotation.failed"), error.localizedDescription)
                }
            })
            menu.addItem(.separator())
        }
        let bookmark = tab.isBookmarked ? L10n.text("bookmark.remove") : L10n.text("bookmark.add")
        menu.addItem(ActionMenuItem(bookmark, image: tab.isBookmarked ? "bookmark.slash" : "bookmark") { [model] in
            model.toggleBookmark()
        })
        return menu
    }

    /// Turns a page per trackpad swipe or mouse wheel notch. Content follows
    /// the scroll: moving it left or up shows the next page.
    func scroll(_ event: NSEvent) {
        guard let tab else { return }
        guard event.hasPreciseScrollingDeltas else {
            let delta = event.scrollingDeltaY != 0 ? event.scrollingDeltaY : event.scrollingDeltaX
            if delta != 0 { tab.turn(delta < 0 ? 1 : -1) }
            return
        }
        if let screens = swipe.feed(event) { tab.turn(screens) }
    }
}

/// WarmUp decides when the EDR headroom has settled after a book is shown:
/// when it reaches the screen's maximum, when it stops rising for a moment,
/// or after at most 1.5 seconds.
struct WarmUp {
    private(set) var isWarming: Bool
    private let started = Date()
    private var peak = 0.0
    private var stableSince = Date()

    /// Creates a warm-up; a settled one draws the text at once.
    init(settled: Bool = false) {
        isWarming = !settled
    }

    /// Records a headroom sample; it reports whether warming just ended.
    mutating func sample(_ headroom: Headroom, now: Date = Date()) -> Bool {
        guard isWarming else { return false }
        if headroom.current > peak + 0.02 {
            peak = headroom.current
            stableSince = now
        }
        let settled = headroom.isActive && now.timeIntervalSince(stableSince) >= 0.15
        if headroom.isSettled || settled || now.timeIntervalSince(started) >= 1.5 {
            isWarming = false
            return true
        }
        return false
    }
}

/// Swipe turns the scrolling of one trackpad gesture into at most one page
/// turn, ignoring the momentum that follows it.
struct Swipe {
    private var distance = CGSize.zero
    private var turned = false

    /// The scroll distance, in points, that turns a page.
    static let threshold: CGFloat = 50

    /// Feeds a scroll event; it returns the screens to turn, if any.
    mutating func feed(_ event: NSEvent) -> Int? {
        feed(phase: event.phase, momentum: !event.momentumPhase.isEmpty,
             dx: event.scrollingDeltaX, dy: event.scrollingDeltaY)
    }

    /// Feeds one scroll step of a gesture in the given phase.
    mutating func feed(phase: NSEvent.Phase, momentum: Bool, dx: CGFloat, dy: CGFloat) -> Int? {
        if phase.contains(.began) {
            distance = .zero
            turned = false
        }
        guard !momentum, !turned else { return nil }
        distance.width += dx
        distance.height += dy
        let delta = abs(distance.width) > abs(distance.height) ? distance.width : distance.height
        guard abs(delta) >= Self.threshold else { return nil }
        turned = true
        return delta < 0 ? 1 : -1
    }
}

/// KeyCode names the virtual key codes the reader handles.
enum KeyCode {
    static let space: UInt16 = 49
    static let escape: UInt16 = 53
    static let pageUp: UInt16 = 116
    static let pageDown: UInt16 = 121
    static let home: UInt16 = 115
    static let end: UInt16 = 119
    static let left: UInt16 = 123
    static let right: UInt16 = 124
    static let down: UInt16 = 125
    static let up: UInt16 = 126
}

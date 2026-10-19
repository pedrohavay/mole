import AppKit

/// PageAccessibility is what VoiceOver learns of the page an EDRMetalView
/// draws: a label, the paragraphs on screen as static text, and actions that
/// turn the page. The paragraphs are read from the core only when an
/// assistive app asks for them, so the page costs nothing while none runs.
@MainActor
final class PageAccessibility {
    /// Paragraphs returns the paragraphs of the visible pages.
    var paragraphs: (() -> [PageParagraph])?
    /// Label describes the page: the book and where it is.
    var label: (() -> String?)?
    /// OnTurn turns the given number of screens.
    var onTurn: ((Int) -> Void)?
    fileprivate var elements: [PageTextElement]?

    /// ScrollActions are the actions VoiceOver's scroll commands perform.
    /// AppKit has no constants for them.
    fileprivate static let scrollActions: [NSAccessibility.Action: Int] = [
        NSAccessibility.Action(rawValue: "AXScrollDownByPage"): 1,
        NSAccessibility.Action(rawValue: "AXScrollRightByPage"): 1,
        NSAccessibility.Action(rawValue: "AXScrollUpByPage"): -1,
        NSAccessibility.Action(rawValue: "AXScrollLeftByPage"): -1,
    ]
}

/// PageTextElement is a paragraph of the visible pages, or the part of one
/// on a page, as VoiceOver sees it: static text over the paragraph's lines.
final class PageTextElement: NSAccessibilityElement {
    let paragraph: PageParagraph
    private weak var view: NSView?

    init(_ paragraph: PageParagraph, in view: NSView) {
        self.paragraph = paragraph
        self.view = view
        super.init()
        setAccessibilityElement(true)
        setAccessibilityRole(.staticText)
        setAccessibilityValue(paragraph.text)
        setAccessibilityParent(view)
    }

    /// The frame is worked out from the view on every call, so it follows
    /// the window as it moves.
    override func accessibilityFrame() -> NSRect {
        guard let view else { return .zero }
        let box = paragraph.frame
        // The paragraph's box is from the top left; the view is not flipped.
        let local = NSRect(x: box.minX, y: view.bounds.height - box.maxY, width: box.width, height: box.height)
        return NSAccessibility.screenRect(fromView: view, rect: local)
    }
}

extension EDRMetalView {
    /// Drops the paragraphs of the pages shown before and tells assistive
    /// apps that the page changed, announcing announcement if given.
    func pageChanged(announcement: String?) {
        pageAccessibility.elements = nil
        NSAccessibility.post(element: self, notification: .layoutChanged)
        guard let announcement else { return }
        NSAccessibility.post(element: self, notification: .announcementRequested, userInfo: [
            .announcement: announcement,
            .priority: NSAccessibilityPriorityLevel.high.rawValue,
        ])
    }

    /// Returns the elements of the paragraphs on screen, making them the
    /// first time they are asked for after the page changed.
    func textElements() -> [PageTextElement] {
        if let elements = pageAccessibility.elements { return elements }
        let elements = (pageAccessibility.paragraphs?() ?? []).map { PageTextElement($0, in: self) }
        pageAccessibility.elements = elements
        return elements
    }

    override func isAccessibilityElement() -> Bool { true }

    override func accessibilityRole() -> NSAccessibility.Role? { .group }

    override func accessibilityRoleDescription() -> String? { L10n.text("reader.role") }

    override func accessibilityLabel() -> String? { pageAccessibility.label?() }

    override func accessibilityChildren() -> [Any]? { textElements() }

    override func accessibilityHitTest(_ point: NSPoint) -> Any? {
        textElements().first { $0.accessibilityFrame().contains(point) } ?? self
    }

    override func accessibilityCustomActions() -> [NSAccessibilityCustomAction]? {
        [
            NSAccessibilityCustomAction(name: L10n.text("reader.nextPage")) { [weak self] in
                self?.pageAccessibility.onTurn?(1)
                return true
            },
            NSAccessibilityCustomAction(name: L10n.text("reader.previousPage")) { [weak self] in
                self?.pageAccessibility.onTurn?(-1)
                return true
            },
        ]
    }

    // VoiceOver scrolls through the older, string-based actions, which the
    // NSAccessibility protocol does not cover; scrolling turns the page.

    @available(macOS, deprecated: 10.10)
    override func accessibilityActionNames() -> [NSAccessibility.Action] {
        super.accessibilityActionNames() + PageAccessibility.scrollActions.keys.sorted { $0.rawValue < $1.rawValue }
    }

    @available(macOS, deprecated: 10.10)
    override func accessibilityPerformAction(_ action: NSAccessibility.Action) {
        guard let screens = PageAccessibility.scrollActions[action] else {
            return super.accessibilityPerformAction(action)
        }
        pageAccessibility.onTurn?(screens)
    }
}

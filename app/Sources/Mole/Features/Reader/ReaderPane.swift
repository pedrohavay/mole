import AppKit
import SwiftUI
import Translation

/// ReaderPane shows an open book on the EDR canvas, with the controls that
/// float over it: the selection bar, the highlight editor, the bookmark
/// ribbon and the picture viewer.
@MainActor
struct ReaderPane: View {
    @Environment(AppModel.self) private var model
    let tab: ReaderTab
    @State private var editing: HighlightEdit?
    /// Viewing is the picture open full size, if any.
    @State private var viewing: CGImage?

    /// HighlightEdit is a highlight being edited and where its popover points.
    struct HighlightEdit: Identifiable {
        let annotation: Annotation
        let anchor: CGRect
        var id: Int64 { annotation.id }
    }

    var body: some View {
        if case .failure(let error) = EDRPipeline.shared {
            EmptyState(symbol: "exclamationmark.triangle", title: L10n.text("error.reader.title"),
                       message: error.localizedDescription)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        } else {
            GeometryReader { geometry in
                ZStack(alignment: .topLeading) {
                    ReaderCanvas(model: model, tab: tab, revision: tab.revision, gain: model.gain,
                                 tint: model.settings.tint, gamma: model.settings.gamma,
                                 fontSize: model.settings.fontSize, spread: model.settings.spread,
                                 pictures: model.settings.pictures, bookColors: model.settings.bookColors,
                                 marks: tab.marks, accentLight: model.accentLight, showsPicture: viewing != nil,
                                 onEdit: { annotation, point in
                                     editing = HighlightEdit(annotation: annotation,
                                                             anchor: CGRect(x: point.x - 1, y: point.y - 1, width: 2, height: 2))
                                 },
                                 onPicture: open)
                    if tab.isBookmarked {
                        bookmarkRibbon(width: geometry.size.width)
                    }
                    if let bar = selectionBarPosition(in: geometry.size) {
                        SelectionBar(tab: tab) { annotation in
                            editing = HighlightEdit(annotation: annotation, anchor: CGRect(origin: bar, size: .zero))
                        }
                        .position(bar)
                        .transition(.opacity)
                    }
                    // The anchor stays while there is a selection, so the
                    // popover sees its binding turn on.
                    if !tab.isSelecting, let box = tab.selectionRects().first {
                        Color.clear
                            .frame(width: max(2, box.width), height: max(2, box.height))
                            .offset(x: box.minX, y: box.minY)
                            .modifier(TranslationPopover(tab: tab))
                    }
                    if let editing {
                        Color.clear
                            .frame(width: max(2, editing.anchor.width), height: max(2, editing.anchor.height))
                            .offset(x: editing.anchor.minX, y: editing.anchor.minY)
                            .popover(isPresented: Binding { self.editing != nil } set: { if !$0 { self.editing = nil } },
                                     arrowEdge: .top) {
                                HighlightEditor(tab: tab, annotation: editing.annotation) { self.editing = nil }
                            }
                    }
                    if let viewing {
                        PictureViewer(image: viewing) { self.viewing = nil }
                            .transition(.opacity)
                    }
                }
            }
            .background(Theme.page)
            .animation(.easeOut(duration: 0.12), value: tab.selection == nil || tab.isSelecting)
            .animation(.easeOut(duration: 0.15), value: viewing != nil)
            .onChange(of: tab.id) { viewing = nil }
        }
    }

    /// Opens picture full size.
    private func open(_ picture: PagePicture) {
        do {
            viewing = try tab.session.image(of: picture)
        } catch {
            model.alert(L10n.text("picture.failed"), error.localizedDescription)
        }
    }

    /// Returns where the selection bar goes: centered above the selection,
    /// or below it when the selection starts near the top.
    private func selectionBarPosition(in size: CGSize) -> CGPoint? {
        guard tab.selection != nil, !tab.isSelecting else { return nil }
        let rects = tab.selectionRects()
        guard let first = rects.first, let last = rects.last else { return nil }
        let union = rects.dropFirst().reduce(first) { $0.union($1) }
        let x = union.midX.clamped(to: 140...max(140, size.width - 140))
        let above = first.minY - 26
        return CGPoint(x: x, y: above > 24 ? above : last.maxY + 26)
    }

    private func bookmarkRibbon(width: CGFloat) -> some View {
        Button {
            model.toggleBookmark()
        } label: {
            Image(systemName: "bookmark.fill")
                .font(.system(size: 18))
                .foregroundStyle(model.accent.opacity(0.85))
        }
        .buttonStyle(.plain)
        .help(L10n.text("bookmark.remove"))
        .accessibilityLabel(L10n.text("bookmark.remove"))
        .position(x: width - 34, y: 18)
    }
}

/// ReaderCanvas hosts the Metal view. Its parameters are the values that
/// require a redraw, so SwiftUI calls updateNSView whenever one changes.
@MainActor
struct ReaderCanvas: NSViewRepresentable {
    let model: AppModel
    let tab: ReaderTab
    let revision: Int
    let gain: Double
    let tint: Int
    let gamma: Double
    let fontSize: Double
    let spread: Spread
    let pictures: PictureMode
    let bookColors: Bool
    let marks: [PageMark]
    let accentLight: SIMD3<Float>
    /// ShowsPicture is set while the picture viewer covers the page.
    let showsPicture: Bool
    /// OnEdit runs when a click lands on a highlight.
    let onEdit: (Annotation, CGPoint) -> Void
    /// OnPicture runs when a click lands on a picture.
    let onPicture: (PagePicture) -> Void

    func makeCoordinator() -> ReaderCoordinator { ReaderCoordinator(model: model) }

    func makeNSView(context: Context) -> EDRMetalView {
        let view = EDRMetalView()
        let coordinator = context.coordinator
        view.onResize = { [weak view] in if let view { coordinator.scheduleLayout(view) } }
        view.onKey = { coordinator.handle($0) }
        view.onMouse = { [weak view] step, point, event in
            if let view { coordinator.mouse(step, at: point, event: event, in: view) }
        }
        view.onHover = { coordinator.hover(at: $0) }
        view.onScroll = { coordinator.scroll($0) }
        view.onMenu = { coordinator.menu(at: $0) }
        view.onCopy = { coordinator.model.copySelection() }
        view.onLookUp = { coordinator.lookUp(at: $0) }
        view.canCopy = { coordinator.tab?.selection != nil }
        view.pageAccessibility.paragraphs = { coordinator.tab?.visibleParagraphs() ?? [] }
        view.pageAccessibility.label = { coordinator.tab.map { "\($0.book.title), \($0.pageDescription)" } }
        view.pageAccessibility.onTurn = { coordinator.tab?.turn($0) }
        return view
    }

    func updateNSView(_ view: EDRMetalView, context: Context) {
        let coordinator = context.coordinator
        coordinator.onEdit = onEdit
        coordinator.onPicture = onPicture
        // The viewer takes the keys while it is open; give them back after.
        if coordinator.showsPicture, !showsPicture { view.window?.makeFirstResponder(view) }
        coordinator.showsPicture = showsPicture
        if coordinator.tab !== tab {
            coordinator.tab = tab
            tab.lookUpHandler = { [weak view] text, box in view?.showDefinition(of: text, at: box) }
            coordinator.reset()
            // Take the keys for the newly shown book; later updates leave
            // focus where the user put it.
            view.window?.makeFirstResponder(view)
        }
        coordinator.update(view)
    }
}

/// TranslationPopover shows Apple's translation of the selection, on macOS
/// 14.4 and later, where the Translation framework has it.
@MainActor
private struct TranslationPopover: ViewModifier {
    let tab: ReaderTab

    func body(content: Content) -> some View {
        if #available(macOS 14.4, *) {
            content.translationPresentation(
                isPresented: Binding { tab.translationText != nil } set: { if !$0 { tab.translationText = nil } },
                text: tab.translationText ?? "")
        } else {
            content
        }
    }
}

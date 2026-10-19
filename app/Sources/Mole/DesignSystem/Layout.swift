import AppKit
import SwiftUI

/// ColumnResizer is the hairline between a side column and the main area.
/// Dragging it resizes the column within range.
struct ColumnResizer: View {
    @Binding var width: Double
    let range: ClosedRange<CGFloat>
    /// Edge is the side of the main area the column is on.
    let edge: HorizontalEdge
    @State private var start: Double?

    var body: some View {
        Rectangle()
            .fill(Theme.border)
            .frame(width: 1)
            .overlay {
                Color.clear
                    .frame(width: 9)
                    .contentShape(Rectangle())
                    .background(ResizeCursor())
                    .gesture(drag)
            }
            .zIndex(1)
    }

    private var drag: some Gesture {
        DragGesture(minimumDistance: 1, coordinateSpace: .global)
            .onChanged { value in
                let base = start ?? width
                start = base
                let delta = edge == .leading ? value.translation.width : -value.translation.width
                width = Double(CGFloat(base + delta).clamped(to: range))
            }
            .onEnded { _ in start = nil }
    }
}

/// ResizeCursor shows the left-right resize cursor over its frame. A cursor
/// rect, unlike pushing the cursor on hover, cannot get out of balance when
/// the handle disappears under the pointer.
private struct ResizeCursor: NSViewRepresentable {
    final class CursorView: NSView {
        override func resetCursorRects() {
            addCursorRect(bounds, cursor: .resizeLeftRight)
        }

        // Clicks go to the SwiftUI drag gesture above.
        override func hitTest(_ point: NSPoint) -> NSView? { nil }
    }

    func makeNSView(context: Context) -> NSView { CursorView() }

    func updateNSView(_ view: NSView, context: Context) {
        view.window?.invalidateCursorRects(for: view)
    }
}

/// EmptyState explains why a list is empty and what to do about it. Unlike
/// ContentUnavailableView it sits at the top of the space it is given.
struct EmptyState<Actions: View>: View {
    let symbol: String
    let title: String
    let message: String
    @ViewBuilder var actions: Actions

    var body: some View {
        VStack(spacing: Theme.Space.m) {
            Image(systemName: symbol)
                .font(.system(size: 34, weight: .light))
                .foregroundStyle(Theme.tertiary)
                .padding(.bottom, Theme.Space.xs)
            Text(title)
                .font(Theme.font(15, .semibold))
                .foregroundStyle(Theme.text)
            Text(message)
                .font(Theme.label)
                .foregroundStyle(Theme.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 320)
            actions.padding(.top, Theme.Space.xs)
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 72)
    }
}

extension EmptyState where Actions == EmptyView {
    init(symbol: String, title: String, message: String) {
        self.init(symbol: symbol, title: title, message: message) { EmptyView() }
    }
}

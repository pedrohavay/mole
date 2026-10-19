import SwiftUI

/// Hoverable paints a rounded highlight under content while the pointer is
/// over it, and a stronger one while it is selected.
struct Hoverable: ViewModifier {
    var selected = false
    var radius = Theme.Radius.m
    @State private var hovering = false

    func body(content: Content) -> some View {
        content
            .background(
                RoundedRectangle(cornerRadius: radius, style: .continuous)
                    .fill(selected ? Theme.selected : (hovering ? Theme.hover : .clear))
            )
            .onHover { hovering = $0 }
            .animation(Theme.hoverAnimation, value: hovering)
    }
}

extension View {
    func hoverable(selected: Bool = false, radius: CGFloat = Theme.Radius.m) -> some View {
        modifier(Hoverable(selected: selected, radius: radius))
    }

    /// Card draws content on a raised surface with a hairline border.
    func card(radius: CGFloat = Theme.Radius.l) -> some View {
        background(
            RoundedRectangle(cornerRadius: radius, style: .continuous)
                .fill(Theme.raised.opacity(0.6))
                .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).strokeBorder(Theme.border))
        )
    }

    /// Hairline draws a one-point separator along edge.
    func hairline(_ edge: Alignment) -> some View {
        overlay(alignment: edge) {
            if edge == .top || edge == .bottom {
                Rectangle().fill(Theme.border).frame(height: 1)
            } else {
                Rectangle().fill(Theme.border).frame(width: 1)
            }
        }
    }
}

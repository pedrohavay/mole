import SwiftUI

/// SectionHeader labels a group of sidebar rows.
struct SectionHeader<Trailing: View>: View {
    let title: String
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack {
            Text(title)
                .font(Theme.font(11, .semibold))
                .foregroundStyle(Theme.tertiary)
            Spacer()
            trailing
        }
        .padding(.horizontal, Theme.Space.s)
        .padding(.top, Theme.Space.l)
        .padding(.bottom, Theme.Space.xs)
    }
}

extension SectionHeader where Trailing == EmptyView {
    init(_ title: String) {
        self.init(title: title) { EmptyView() }
    }
}

/// SidebarRow is one entry of the sidebar: icon or status, a title, an
/// optional second line and trailing metadata.
struct SidebarRow: View {
    @Environment(\.accent) private var accent
    var symbol: String?
    var dot: Color?
    let title: String
    var subtitle: String?
    var meta: String?
    var selected = false

    var body: some View {
        HStack(alignment: subtitle == nil ? .center : .top, spacing: Theme.Space.s) {
            Group {
                if let dot {
                    StatusDot(color: dot).padding(.top, subtitle == nil ? 0 : 5)
                } else if let symbol {
                    Image(systemName: symbol)
                        .font(.system(size: 13))
                        .foregroundStyle(selected ? accent : Theme.secondary)
                }
            }
            .frame(width: 18)
            VStack(alignment: .leading, spacing: 1) {
                Text(title)
                    .font(Theme.body)
                    .foregroundStyle(selected ? Theme.text : Theme.text.opacity(0.88))
                    .lineLimit(1)
                if let subtitle {
                    Text(subtitle)
                        .font(Theme.caption)
                        .foregroundStyle(Theme.secondary)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: Theme.Space.xs)
            if let meta {
                Text(meta)
                    .font(Theme.digits)
                    .foregroundStyle(Theme.tertiary)
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .padding(.vertical, subtitle == nil ? 5 : 6)
        .contentShape(Rectangle())
        .hoverable(selected: selected)
    }
}

/// TabButton is a tab of the tab strip, with a close button on books.
struct TabButton: View {
    @Environment(\.accent) private var accent
    let symbol: String
    let title: String
    let active: Bool
    var onClose: (() -> Void)?
    let onSelect: () -> Void
    @State private var hovering = false

    var body: some View {
        HStack(spacing: Theme.Space.s) {
            Image(systemName: symbol)
                .font(.system(size: 12))
                .foregroundStyle(active ? accent : Theme.secondary)
            Text(title)
                .font(Theme.label)
                .foregroundStyle(active ? Theme.text : Theme.secondary)
                .lineLimit(1)
                .frame(minWidth: 0, maxWidth: 190, alignment: .leading)
            if let onClose {
                Button(action: onClose) {
                    Image(systemName: "xmark")
                        .font(.system(size: 9, weight: .semibold))
                        .foregroundStyle(Theme.secondary)
                        .frame(width: 16, height: 16)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .hoverable(radius: 4)
                .opacity(active || hovering ? 1 : 0)
                .help(L10n.text("tab.close"))
                .accessibilityLabel(L10n.text("tab.close"))
            }
        }
        .padding(.leading, Theme.Space.m)
        .padding(.trailing, onClose == nil ? Theme.Space.m : Theme.Space.xs)
        .frame(height: Theme.Size.bar)
        .background(active ? Theme.canvas : (hovering ? Theme.hover : .clear))
        .overlay(alignment: .top) {
            Rectangle().fill(active ? accent : .clear).frame(height: 2)
        }
        .hairline(.trailing)
        .contentShape(Rectangle())
        .onTapGesture(perform: onSelect)
        .onHover { hovering = $0 }
        .animation(Theme.hoverAnimation, value: hovering)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(active ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction { onSelect() }
    }
}

import SwiftUI

/// IconButton is a borderless SF Symbols button with a hover highlight. When
/// active it takes the accent color, as a toggle that is on.
struct IconButton: View {
    @Environment(\.accent) private var accent
    let symbol: String
    let help: String
    var size: CGFloat = 13
    var active = false
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: size, weight: .regular))
                .foregroundStyle(active ? accent : Theme.secondary)
                .frame(width: 26, height: 24)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .hoverable(radius: Theme.Radius.s)
        .help(help)
        .accessibilityLabel(help)
    }
}

/// StatusDot is the small colored circle that marks a row's state.
struct StatusDot: View {
    let color: Color

    var body: some View {
        Circle().fill(color).frame(width: 7, height: 7)
    }
}

/// Meter is a thin capsule gauge.
struct Meter: View {
    @Environment(\.accent) private var accent
    let value: Double
    var tint: Color?
    var width: CGFloat = 46

    var body: some View {
        ZStack(alignment: .leading) {
            Capsule().fill(Color.white.opacity(0.1))
            Capsule().fill(tint ?? accent).frame(width: max(2, width * value.clamped(to: 0...1)))
        }
        .frame(width: width, height: 4)
    }
}

/// StatusItem is a group of the status bar: icon, an optional meter and text.
struct StatusItem: View {
    @Environment(\.accent) private var accent
    var symbol: String?
    let text: String
    var meter: Double?
    var tint: Color?

    var body: some View {
        HStack(spacing: Theme.Space.xs + 2) {
            if let symbol {
                Image(systemName: symbol).font(.system(size: 11)).foregroundStyle(tint ?? accent)
            }
            if let meter {
                Meter(value: meter, tint: tint)
            }
            Text(text).font(Theme.digits).foregroundStyle(Theme.secondary)
        }
    }
}

/// SearchField is a compact rounded text field with a magnifier and a clear
/// button.
struct SearchField: View {
    let prompt: String
    @Binding var text: String
    var width: CGFloat = 240
    @FocusState private var focused: Bool

    var body: some View {
        HStack(spacing: Theme.Space.xs + 2) {
            Image(systemName: "magnifyingglass").font(.system(size: 11)).foregroundStyle(Theme.tertiary)
            TextField(prompt, text: $text)
                .textFieldStyle(.plain)
                .font(Theme.label)
                .foregroundStyle(Theme.text)
                .focused($focused)
                .onExitCommand { text = ""; focused = false }
            if !text.isEmpty {
                Button { text = "" } label: {
                    Image(systemName: "xmark.circle.fill").font(.system(size: 11)).foregroundStyle(Theme.tertiary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(L10n.text("action.clear"))
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(width: width, height: 26)
        .background(RoundedRectangle(cornerRadius: Theme.Radius.m).fill(Theme.raised))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.m)
            .strokeBorder(focused ? Theme.secondary.opacity(0.5) : Theme.border))
    }
}

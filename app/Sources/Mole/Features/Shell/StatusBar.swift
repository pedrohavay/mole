import SwiftUI

/// StatusBar holds the light control, the chapter being read and where the
/// reader is in the book. The EDR numbers live in the light popover, so the
/// bar stays quiet.
@MainActor
struct StatusBar: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    @State private var showLight = false
    @State private var showText = false

    var body: some View {
        HStack(spacing: Theme.Space.l) {
            lightButton
            Spacer()
            if let tab = model.activeTab {
                Text(tab.currentChapter?.title ?? "")
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(1)
                Spacer()
                StatusItem(text: pages(tab.state), meter: tab.state.progress, tint: Theme.secondary)
            } else {
                Text(L10n.bookCount(model.books.count))
                    .font(Theme.digits)
                    .foregroundStyle(Theme.secondary)
            }
            IconButton(symbol: "textformat.size", help: L10n.text("text.title"), size: 12, active: showText) {
                showText.toggle()
            }
            .popover(isPresented: $showText, arrowEdge: .top) { TextPanel() }
        }
        .padding(.horizontal, Theme.Space.m)
        .frame(height: Theme.Size.statusBar)
        .background(Theme.surface)
        .hairline(.top)
    }

    /// LightButton shows the text brightness and opens the light popover.
    private var lightButton: some View {
        Button {
            showLight.toggle()
        } label: {
            HStack(spacing: Theme.Space.xs + 2) {
                Image(systemName: model.settings.edr ? "sun.max.fill" : "sun.max")
                    .font(.system(size: 11))
                    .foregroundStyle(model.settings.edr ? accent : Theme.secondary)
                Meter(value: model.settings.level, tint: model.settings.edr ? nil : Theme.secondary)
                Text(L10n.format("status.level", Int((model.settings.level * 100).rounded())))
                    .font(Theme.digits)
                    .foregroundStyle(Theme.secondary)
            }
            .padding(.horizontal, Theme.Space.xs + 2)
            .frame(height: 22)
            .contentShape(Rectangle())
            .hoverable(selected: showLight, radius: Theme.Radius.s)
        }
        .buttonStyle(.plain)
        .help(L10n.text("light.title"))
        .accessibilityLabel(L10n.text("light.title"))
        .accessibilityValue(L10n.format("status.level", Int((model.settings.level * 100).rounded())))
        .popover(isPresented: $showLight, arrowEdge: .top) { LightPanel() }
    }

    private func pages(_ state: SessionState) -> String {
        guard state.pages > 0 else { return "" }
        let percent = Int(state.progress * 100)
        if state.lastVisible > state.page {
            return L10n.format("status.pages.range", state.page + 1, state.lastVisible + 1, state.pages, percent)
        }
        return L10n.format("status.pages.single", state.page + 1, state.pages, percent)
    }
}

import SwiftUI

/// TabStrip holds the library tab and one tab per open book. It is the title
/// bar of the window: its empty space moves the window, and while the sidebar
/// is hidden it leaves room for the traffic lights.
@MainActor
struct TabStrip: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        HStack(spacing: 0) {
            if !model.showSidebar {
                if !model.isFullScreen {
                    Spacer().frame(width: Theme.Size.trafficLights)
                }
                IconButton(symbol: "sidebar.left", help: L10n.text("sidebar.show")) {
                    Motion.animate { model.showSidebar = true }
                }
                .padding(.horizontal, Theme.Space.xs)
            }
            TabButton(symbol: "books.vertical", title: L10n.text("tab.library"), active: model.pane == .library) {
                model.pane = .library
            }
            .fixedSize()
            ForEach(model.tabs) { tab in
                TabButton(symbol: "book", title: tab.book.title, active: model.pane == .book(tab.id),
                          onClose: { model.close(tab) }) {
                    model.pane = .book(tab.id)
                }
                // Tabs shrink to their icon before the strip overflows.
                .frame(minWidth: 56, maxWidth: 220)
            }
            IconButton(symbol: "plus", help: L10n.text("library.add.help")) { model.chooseBooks() }
                .padding(.horizontal, Theme.Space.xs)
            WindowDragArea()
                .frame(minWidth: Theme.Space.xxl)
            if model.activeTab != nil {
                IconButton(symbol: "list.bullet.indent", help: L10n.text("contents.toggle.help"),
                           active: model.showContents) {
                    Motion.animate { model.showContents.toggle() }
                }
                .padding(.trailing, Theme.Space.s)
            }
        }
        .frame(height: Theme.Size.bar)
        .background(Theme.surface)
        .hairline(.bottom)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(L10n.text("tabs.label"))
    }
}

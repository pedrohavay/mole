import SwiftUI

/// ShellView is the window: the sidebar, the tab strip, the library or a
/// book with its contents panel, and the status bar. While a book is read in
/// full screen only the page is left.
@MainActor
struct ShellView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("sidebarWidth") private var sidebarWidth = 252.0
    @AppStorage("contentsWidth") private var contentsWidth = 280.0

    var body: some View {
        GeometryReader { window in
            columns(width: window.size.width)
        }
        .background {
            // Keeps the EDR headroom raised while the window is up, so books
            // open at their final brightness at once.
            EDRSentinel().frame(width: 1, height: 1)
        }
        .background(WindowObserver { model.isFullScreen = $0 })
        .ignoresSafeArea()
        .focusedSceneValue(\.menuState, MenuState(model))
        .sheet(item: Bindable(model).folderImport) { FolderImportSheet(request: $0) }
    }

    /// Lays out the columns for a window width. The contents panel steps
    /// aside rather than squeeze the page below Theme.Size.minReader.
    private func columns(width: CGFloat) -> some View {
        let sidebar = model.showSidebar && !model.isImmersive ? CGFloat(sidebarWidth) + 1 : 0
        let room = width - sidebar - Theme.Size.minReader
        let contentsRange = Theme.Size.contents.lowerBound...max(Theme.Size.contents.lowerBound,
                                                                 min(Theme.Size.contents.upperBound, room))
        let showsContents = model.showContents && !model.isImmersive && room >= Theme.Size.contents.lowerBound
        return HStack(spacing: 0) {
            if model.showSidebar && !model.isImmersive {
                Sidebar()
                    .frame(width: sidebarWidth)
                    .transition(.move(edge: .leading))
                ColumnResizer(width: $sidebarWidth, range: Theme.Size.sidebar, edge: .leading)
            }
            VStack(spacing: 0) {
                if !model.isImmersive {
                    TabStrip()
                }
                HStack(spacing: 0) {
                    main.frame(maxWidth: .infinity, maxHeight: .infinity)
                    if let tab = model.activeTab, showsContents {
                        ColumnResizer(width: $contentsWidth, range: contentsRange, edge: .trailing)
                        SidePanel(tab: tab)
                            .frame(width: CGFloat(contentsWidth).clamped(to: contentsRange))
                            .transition(.move(edge: .trailing))
                    }
                }
                if !model.isImmersive {
                    StatusBar()
                }
            }
            .background(Theme.canvas)
        }
    }

    @ViewBuilder private var main: some View {
        if let tab = model.activeTab {
            ReaderPane(tab: tab)
        } else {
            LibraryView()
        }
    }
}

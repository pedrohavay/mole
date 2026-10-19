import SwiftUI

/// Sidebar lists the shelves, the collections, the open books and the
/// recently read ones.
@MainActor
struct Sidebar: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            ScrollView {
                VStack(alignment: .leading, spacing: 1) {
                    shelves
                    CollectionsSection()
                    openBooks
                    recentBooks
                }
                .padding(.horizontal, Theme.Space.s)
                .padding(.top, Theme.Space.xs)
            }
            .scrollIndicators(.never)
            footer
        }
        .background(Theme.sidebar)
    }

    private var header: some View {
        HStack(spacing: Theme.Space.s) {
            if !model.isFullScreen {
                Spacer().frame(width: Theme.Size.trafficLights - Theme.Space.s)
            }
            Text(L10n.text("app.name"))
                .font(Theme.font(13, .semibold))
                .foregroundStyle(Theme.secondary)
            Spacer()
            IconButton(symbol: "sidebar.left", help: L10n.text("sidebar.hide")) {
                withAnimation(.easeInOut(duration: 0.2)) { model.showSidebar = false }
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(height: Theme.Size.bar)
        .background(WindowDragArea())
    }

    @ViewBuilder private var shelves: some View {
        ForEach(Shelf.allCases) { shelf in
            let selected = model.pane == .library && model.collection == nil && model.shelf == shelf
            Button {
                model.shelf = shelf
                model.pane = .library
            } label: {
                SidebarRow(symbol: shelf.symbol, title: shelf.title,
                           meta: "\(model.books.filter(shelf.keeps).count)", selected: selected)
            }
            .buttonStyle(.plain)
            .accessibilityAddTraits(selected ? .isSelected : [])
        }
    }

    @ViewBuilder private var openBooks: some View {
        SectionHeader(title: L10n.text("sidebar.open")) {
            Text("\(model.tabs.count)").font(Theme.digits).foregroundStyle(Theme.tertiary)
        }
        if model.tabs.isEmpty {
            Text(L10n.text("sidebar.open.empty"))
                .font(Theme.caption)
                .foregroundStyle(Theme.tertiary)
                .padding(.horizontal, Theme.Space.s)
                .padding(.vertical, Theme.Space.xs)
        } else {
            VStack(spacing: 1) {
                ForEach(model.tabs) { tab in
                    let active = model.pane == .book(tab.id)
                    Button {
                        model.pane = .book(tab.id)
                    } label: {
                        SidebarRow(dot: active ? Theme.positive : Theme.tertiary,
                                   title: tab.book.title,
                                   subtitle: tab.currentChapter?.title ?? tab.book.author,
                                   meta: "\(Int(tab.state.progress * 100))%",
                                   selected: active)
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(active ? .isSelected : [])
                }
            }
            .padding(Theme.Space.xs)
            .card()
        }
    }

    @ViewBuilder private var recentBooks: some View {
        let open = Set(model.tabs.map(\.id))
        let recent = model.books.filter { $0.lastOpened != nil && !open.contains($0.id) }.prefix(6)
        if !recent.isEmpty {
            SectionHeader(L10n.text("sidebar.recent"))
            ForEach(Array(recent)) { book in
                Button {
                    model.selectedBook = book.id
                    model.shelf = .all
                    model.pane = .library
                } label: {
                    SidebarRow(dot: book.isFinished ? Theme.positive : Theme.caution,
                               title: book.title,
                               subtitle: book.author.isEmpty ? nil : book.author,
                               meta: book.lastOpened.map { compactAge($0) })
                }
                .buttonStyle(.plain)
                .simultaneousGesture(TapGesture(count: 2).onEnded { model.open(book) })
                .help(L10n.text("sidebar.recent.help"))
            }
        }
    }

    private var footer: some View {
        HStack(spacing: Theme.Space.s) {
            StatusDot(color: model.headroom.isActive ? Theme.positive : Theme.tertiary)
            Text(model.headroom.isActive ? L10n.text("edr.active") : L10n.text("edr.idle"))
                .font(Theme.caption)
                .foregroundStyle(Theme.secondary)
                .help(L10n.format("light.headroom", decimal(model.headroom.current, 1),
                                  decimal(model.headroom.potential, 0)))
            Spacer()
            IconButton(symbol: "plus", help: L10n.text("library.add.help")) { model.chooseBooks() }
        }
        .padding(.horizontal, Theme.Space.m)
        .frame(height: Theme.Size.statusBar + 4)
        .hairline(.top)
    }
}

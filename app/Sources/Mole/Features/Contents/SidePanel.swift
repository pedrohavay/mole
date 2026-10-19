import SwiftUI

/// SidePanel is the panel next to an open book, with four pages: contents,
/// bookmarks, notes and search. It only exists while a book is shown.
@MainActor
struct SidePanel: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let tab: ReaderTab

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            switch model.panelPage {
            case .contents: ContentsList(tab: tab)
            case .bookmarks: BookmarksList(tab: tab)
            case .notes: NotesList(tab: tab)
            case .search: SearchList(tab: tab)
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .background(Theme.sidebar)
    }

    private var header: some View {
        HStack(spacing: Theme.Space.xxs) {
            ForEach(PanelPage.allCases) { page in
                IconButton(symbol: page.symbol, help: page.title, size: 12, active: model.panelPage == page) {
                    model.panelPage = page
                }
                .accessibilityAddTraits(model.panelPage == page ? .isSelected : [])
            }
            Spacer()
            IconButton(symbol: "xmark", help: L10n.text("contents.hide"), size: 11) {
                withAnimation(.easeInOut(duration: 0.2)) { model.showContents = false }
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(height: 40)
        .hairline(.bottom)
    }
}

/// ContentsList is the table of contents, with the entry being read marked.
@MainActor
struct ContentsList: View {
    @Environment(\.accent) private var accent
    let tab: ReaderTab

    var body: some View {
        let entries = self.entries
        let current = entries.lastIndex { $0.page <= tab.state.page }
        VStack(alignment: .leading, spacing: 0) {
            PanelTitle(title: L10n.text("contents.title"))
            if entries.isEmpty {
                EmptyState(symbol: "list.bullet.indent", title: L10n.text("contents.empty.title"),
                           message: L10n.text("contents.empty.message"))
                Spacer()
            } else {
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 1) {
                            // Entries are identified by their place in the
                            // outline, which no relayout changes.
                            ForEach(Array(entries.enumerated()), id: \.offset) { index, chapter in
                                row(chapter, current: index == current)
                            }
                        }
                        .padding(Theme.Space.s)
                    }
                    // The ids are Ints; an Int? would match none of them.
                    .onAppear {
                        if let current { proxy.scrollTo(current, anchor: .center) }
                    }
                    .onChange(of: current) { _, index in
                        guard let index else { return }
                        withAnimation { proxy.scrollTo(index, anchor: .center) }
                    }
                }
            }
        }
    }

    /// Entries are the first two outline levels; deeper ones would crowd
    /// the panel.
    private var entries: [Chapter] { tab.chapters.filter { $0.level <= 1 } }

    private func row(_ chapter: Chapter, current: Bool) -> some View {
        Button {
            tab.go(to: chapter.page)
        } label: {
            HStack(alignment: .firstTextBaseline) {
                Text(chapter.title)
                    .font(chapter.level == 0 ? Theme.font(12, .medium) : Theme.label)
                    .foregroundStyle(current ? accent : (chapter.level == 0 ? Theme.text : Theme.secondary))
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                Spacer(minLength: Theme.Space.xs)
                Text("\(chapter.page + 1)").font(Theme.digits).foregroundStyle(Theme.tertiary)
            }
            .padding(.leading, CGFloat(chapter.level) * Theme.Space.m + Theme.Space.s)
            .padding(.trailing, Theme.Space.s)
            .padding(.vertical, 5)
            .contentShape(Rectangle())
            .hoverable(selected: current)
        }
        .buttonStyle(.plain)
        .accessibilityValue(L10n.format("contents.page", chapter.page + 1))
        .accessibilityAddTraits(current ? .isSelected : [])
    }
}

/// PanelTitle is the heading of a side panel page.
struct PanelTitle<Trailing: View>: View {
    let title: String
    @ViewBuilder var trailing: Trailing

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(title)
                .font(Theme.font(13, .semibold))
                .foregroundStyle(Theme.text)
            Spacer()
            trailing
        }
        .padding(.horizontal, Theme.Space.l)
        .padding(.top, Theme.Space.m)
        .padding(.bottom, Theme.Space.xs)
    }
}

extension PanelTitle where Trailing == EmptyView {
    init(title: String) {
        self.init(title: title) { EmptyView() }
    }
}

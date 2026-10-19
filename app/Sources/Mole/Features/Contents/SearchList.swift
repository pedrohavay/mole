import SwiftUI

/// SearchList searches the open book. Hits arrive while the search runs;
/// Return shows the next one, and a click shows any of them.
@MainActor
struct SearchList: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let tab: ReaderTab
    @State private var text = ""
    @FocusState private var focused: Bool
    @State private var pending: DispatchWorkItem?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: Theme.Space.s) {
                field
                status
            }
            .padding(.horizontal, Theme.Space.m)
            .padding(.vertical, Theme.Space.m)
            .hairline(.bottom)
            if !tab.query.isEmpty && tab.hits.isEmpty && tab.searchDone {
                EmptyState(symbol: "magnifyingglass", title: L10n.text("search.empty.title"),
                           message: L10n.format("search.empty.message", tab.query))
                Spacer()
            } else {
                results
            }
        }
        .onAppear {
            text = tab.query
            focused = true
        }
        .onChange(of: model.searchFocus) { _, _ in focused = true }
        // A search started elsewhere (Search for Selection) shows its query;
        // one typed here already matches, so typing is never overwritten.
        .onChange(of: tab.query) { _, query in
            if text.trimmingCharacters(in: .whitespacesAndNewlines) != query { text = query }
        }
    }

    private var field: some View {
        HStack(spacing: Theme.Space.xs + 2) {
            Image(systemName: "magnifyingglass").font(.system(size: 11)).foregroundStyle(Theme.tertiary)
            TextField(L10n.text("search.prompt"), text: $text)
                .textFieldStyle(.plain)
                .font(Theme.label)
                .foregroundStyle(Theme.text)
                .focused($focused)
                .onSubmit {
                    // Return searches at once, then walks the hits.
                    if tab.query == text.trimmingCharacters(in: .whitespacesAndNewlines) {
                        tab.showHit(step: 1)
                    } else {
                        pending?.cancel()
                        tab.find(text)
                    }
                }
                .onChange(of: text) { _, value in schedule(value) }
                .onExitCommand {
                    text = ""
                    tab.find("")
                }
            if !text.isEmpty {
                Button {
                    text = ""
                    tab.find("")
                } label: {
                    Image(systemName: "xmark.circle.fill").font(.system(size: 11)).foregroundStyle(Theme.tertiary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(L10n.text("action.clear"))
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(height: 28)
        .background(RoundedRectangle(cornerRadius: Theme.Radius.m).fill(Theme.raised))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.m)
            .strokeBorder(focused ? Theme.secondary.opacity(0.5) : Theme.border))
    }

    @ViewBuilder private var status: some View {
        if !tab.query.isEmpty {
            HStack(spacing: Theme.Space.s) {
                if !tab.searchDone {
                    ProgressView().controlSize(.small).scaleEffect(0.7).frame(width: 12, height: 12)
                }
                Text(statusText)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                Spacer()
                if !tab.hits.isEmpty {
                    IconButton(symbol: "chevron.up", help: L10n.text("menu.findPrevious"), size: 10) {
                        tab.showHit(step: -1)
                    }
                    IconButton(symbol: "chevron.down", help: L10n.text("menu.findNext"), size: 10) {
                        tab.showHit(step: 1)
                    }
                }
            }
        }
    }

    private var statusText: String {
        let count = tab.hits.count
        if !tab.searchDone { return L10n.format("search.running", count) }
        if tab.searchTruncated { return L10n.format("search.truncated", count) }
        if let current = tab.currentHit { return L10n.format("search.position", current + 1, count) }
        return count == 1 ? L10n.text("search.count.one") : L10n.format("search.count.other", count)
    }

    private var results: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 1) {
                    ForEach(Array(tab.hits.enumerated()), id: \.offset) { index, hit in
                        row(hit, index: index)
                    }
                }
                .padding(Theme.Space.s)
            }
            .onChange(of: tab.currentHit) { _, index in
                guard let index else { return }
                withAnimation { proxy.scrollTo(index, anchor: .center) }
            }
        }
    }

    private func row(_ hit: Search.Hit, index: Int) -> some View {
        let current = index == tab.currentHit
        return Button {
            tab.showHit(index)
        } label: {
            VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                excerpt(hit)
                    .font(Theme.label)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                Text(AnnotationPlace(tab: tab, anchor: hit.range.start).text)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, Theme.Space.s)
            .padding(.vertical, 6)
            .contentShape(Rectangle())
            .hoverable(selected: current)
        }
        .buttonStyle(.plain)
        .id(index)
    }

    /// Returns the excerpt of hit with the match picked out. The match is
    /// located in UTF-16 code units, as the core reports it.
    private func excerpt(_ hit: Search.Hit) -> Text {
        let excerpt = hit.excerpt as NSString
        let start = min(max(0, hit.matchStart), excerpt.length)
        let length = min(max(0, hit.matchLength), excerpt.length - start)
        let before = excerpt.substring(to: start)
        let match = excerpt.substring(with: NSRange(location: start, length: length))
        let after = excerpt.substring(from: start + length)
        return Text(before).foregroundColor(Theme.secondary)
            + Text(match).foregroundColor(Theme.text).bold()
            + Text(after).foregroundColor(Theme.secondary)
    }

    /// Searches shortly after typing stops, so each keystroke does not
    /// restart a search of the whole book.
    private func schedule(_ value: String) {
        pending?.cancel()
        let work = DispatchWorkItem { [tab] in
            MainActor.assumeIsolated { tab.find(value) }
        }
        pending = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.35, execute: work)
    }
}

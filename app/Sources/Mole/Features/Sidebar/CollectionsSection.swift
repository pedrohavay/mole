import SwiftUI

/// CollectionsSection lists the collections under the shelves. A click shows
/// a collection, a double click or Rename edits its name in place, and
/// covers dropped on a row join that collection.
@MainActor
struct CollectionsSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        SectionHeader(title: L10n.text("sidebar.collections")) {
            IconButton(symbol: "plus", help: L10n.text("collections.add.help"), size: 11) {
                model.createCollection()
            }
            .padding(.vertical, -6)
        }
        if model.collections.isEmpty {
            Text(L10n.text("sidebar.collections.empty"))
                .font(Theme.caption)
                .foregroundStyle(Theme.tertiary)
                .padding(.horizontal, Theme.Space.s)
                .padding(.vertical, Theme.Space.xs)
        } else {
            ForEach(model.collections) { collection in
                CollectionRow(collection: collection)
            }
        }
    }
}

/// CollectionRow is one collection of the sidebar.
@MainActor
private struct CollectionRow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let collection: BookCollection
    @State private var dropTargeted = false

    var body: some View {
        let selected = model.pane == .library && model.collection == collection.id
        Group {
            if model.renamingCollection == collection.id {
                NameField(collection: collection)
            } else {
                Button {
                    model.show(collection)
                } label: {
                    SidebarRow(symbol: collection.symbol, title: collection.name,
                               meta: "\(model.count(of: collection))", selected: selected)
                }
                .buttonStyle(.plain)
                .simultaneousGesture(TapGesture(count: 2).onEnded { model.renamingCollection = collection.id })
                .accessibilityAddTraits(selected ? .isSelected : [])
                .help(collection.folder.isEmpty ? "" : L10n.format("collection.folder.help", collection.folder))
            }
        }
        .overlay {
            RoundedRectangle(cornerRadius: Theme.Radius.m, style: .continuous)
                .strokeBorder(dropTargeted ? accent : .clear, lineWidth: 1.5)
                .allowsHitTesting(false)
        }
        .dropDestination(for: BookDrag.self) { drags, _ in
            guard !drags.isEmpty else { return false }
            model.add(drags.map(\.id), to: collection)
            return true
        } isTargeted: { dropTargeted = $0 }
        .contextMenu {
            Button(L10n.text("collection.rename")) { model.renamingCollection = collection.id }
            Divider()
            Button(L10n.text("collection.delete"), role: .destructive) { model.delete(collection) }
        }
    }
}

/// NameField edits a collection's name in place: Return or a click elsewhere
/// keeps the new name, Escape keeps the old one.
@MainActor
private struct NameField: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let collection: BookCollection
    @State private var name = ""
    @FocusState private var focused: Bool

    var body: some View {
        HStack(spacing: Theme.Space.s) {
            Image(systemName: collection.symbol)
                .font(.system(size: 13))
                .foregroundStyle(accent)
                .frame(width: 18)
            TextField(L10n.text("collection.name"), text: $name)
                .textFieldStyle(.plain)
                .font(Theme.body)
                .foregroundStyle(Theme.text)
                .focused($focused)
                .onSubmit(commit)
                .onExitCommand { model.renamingCollection = nil }
        }
        .padding(.horizontal, Theme.Space.s)
        .padding(.vertical, 4)
        .background(RoundedRectangle(cornerRadius: Theme.Radius.m, style: .continuous).fill(Theme.raised))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.m, style: .continuous).strokeBorder(accent.opacity(0.7)))
        .onAppear {
            name = collection.name
            // The field takes focus once it is in the window.
            DispatchQueue.main.async { focused = true }
        }
        .onChange(of: focused) { _, focused in
            if !focused { commit() }
        }
    }

    private func commit() {
        // Return and the loss of focus both end up here; only the first
        // one counts.
        guard model.renamingCollection == collection.id else { return }
        model.rename(collection, to: name)
    }
}

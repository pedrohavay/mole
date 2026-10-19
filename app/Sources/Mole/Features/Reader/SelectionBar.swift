import SwiftUI

/// SelectionBar floats over a selection with what can be done to it:
/// highlight in a color, add a note, copy, or search the book for it.
@MainActor
struct SelectionBar: View {
    @Environment(AppModel.self) private var model
    let tab: ReaderTab
    /// OnNote runs with the new highlight when the reader asks for a note.
    let onNote: (Annotation) -> Void

    var body: some View {
        HStack(spacing: Theme.Space.xs) {
            ForEach(HighlightColor.all.indices, id: \.self) { index in
                let color = HighlightColor.all[index]
                Button { model.highlightSelection(color: index) } label: {
                    Circle()
                        .fill(color.swatch)
                        .frame(width: 14, height: 14)
                        .padding(4)
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .help(L10n.format("highlight.in", color.name))
                .accessibilityLabel(L10n.format("highlight.in", color.name))
            }
            divider
            IconButton(symbol: "note.text.badge.plus", help: L10n.text("note.add"), size: 12) {
                addNote()
            }
            IconButton(symbol: "doc.on.doc", help: L10n.text("action.copy"), size: 12) {
                model.copySelection()
                tab.clearSelection()
            }
            IconButton(symbol: "magnifyingglass", help: L10n.text("search.selection"), size: 12) {
                model.searchSelection()
            }
        }
        .padding(.horizontal, Theme.Space.s)
        .frame(height: 34)
        .background(Capsule().fill(Theme.raised))
        .overlay(Capsule().strokeBorder(Theme.border))
        .shadow(color: .black.opacity(0.6), radius: 10, y: 4)
        .fixedSize()
    }

    private var divider: some View {
        Rectangle().fill(Theme.border).frame(width: 1, height: 18).padding(.horizontal, Theme.Space.xxs)
    }

    private func addNote() {
        do {
            if let highlight = try tab.highlightSelection(color: model.lastHighlightColor) {
                onNote(highlight)
            }
        } catch {
            model.alert(L10n.text("annotation.failed"), error.localizedDescription)
        }
    }
}

/// HighlightEditor is the popover of a highlight: its color, its note and a
/// way to remove it. Changes are saved as the popover closes.
@MainActor
struct HighlightEditor: View {
    @Environment(AppModel.self) private var model
    let tab: ReaderTab
    let done: () -> Void
    @State private var draft: Annotation
    @FocusState private var noteFocused: Bool
    private let original: Annotation

    init(tab: ReaderTab, annotation: Annotation, done: @escaping () -> Void) {
        self.tab = tab
        self.done = done
        original = annotation
        _draft = State(initialValue: annotation)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Text(draft.text)
                .font(.system(size: 12, design: .serif))
                .foregroundStyle(Theme.secondary)
                .lineLimit(4)
                .padding(.leading, Theme.Space.s)
                .overlay(alignment: .leading) {
                    Rectangle().fill(HighlightColor.at(draft.color).swatch).frame(width: 3)
                }
            HStack(spacing: Theme.Space.xs) {
                ForEach(HighlightColor.all.indices, id: \.self) { index in
                    let color = HighlightColor.all[index]
                    Button { draft.color = index } label: {
                        Circle()
                            .fill(color.swatch)
                            .frame(width: 16, height: 16)
                            .overlay(Circle().strokeBorder(Color.white.opacity(draft.color == index ? 0.9 : 0),
                                                           lineWidth: 2))
                            .padding(2)
                    }
                    .buttonStyle(.plain)
                    .help(color.name)
                    .accessibilityLabel(color.name)
                    .accessibilityAddTraits(draft.color == index ? .isSelected : [])
                }
            }
            TextEditor(text: $draft.note)
                .font(Theme.body)
                .scrollContentBackground(.hidden)
                .padding(Theme.Space.xs)
                .frame(height: 96)
                .background(RoundedRectangle(cornerRadius: Theme.Radius.m).fill(Theme.canvas))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.m).strokeBorder(Theme.border))
                .overlay(alignment: .topLeading) {
                    if draft.note.isEmpty {
                        Text(L10n.text("note.placeholder"))
                            .font(Theme.body)
                            .foregroundStyle(Theme.tertiary)
                            .padding(.horizontal, Theme.Space.s + 1)
                            .padding(.vertical, Theme.Space.xs + 4)
                            .allowsHitTesting(false)
                    }
                }
                .focused($noteFocused)
            HStack {
                Button(role: .destructive) {
                    remove()
                } label: {
                    Label(L10n.text("highlight.remove"), systemImage: "trash")
                }
                Spacer()
                Button(L10n.text("action.done")) { done() }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(Theme.Space.l)
        .frame(width: 320)
        .onAppear { noteFocused = original.note.isEmpty }
        .onDisappear(perform: save)
    }

    private func save() {
        guard draft.color != original.color || draft.note != original.note,
              tab.annotations.contains(where: { $0.id == draft.id }) else { return }
        do { try tab.update(draft) } catch { model.alert(L10n.text("annotation.failed"), error.localizedDescription) }
    }

    private func remove() {
        do { try tab.remove(original) } catch { model.alert(L10n.text("annotation.failed"), error.localizedDescription) }
        done()
    }
}

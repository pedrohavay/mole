import AppKit
import SwiftUI

/// BookmarksList lists the bookmarks of the open book; a click goes there.
@MainActor
struct BookmarksList: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let tab: ReaderTab

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            PanelTitle(title: L10n.text("panel.bookmarks")) {
                Button(tab.isBookmarked ? L10n.text("bookmark.remove") : L10n.text("bookmark.add")) {
                    model.toggleBookmark()
                }
                .buttonStyle(.link)
                .font(Theme.caption)
                .foregroundStyle(accent)
            }
            if tab.bookmarks.isEmpty {
                EmptyState(symbol: "bookmark", title: L10n.text("bookmarks.empty.title"),
                           message: L10n.text("bookmarks.empty.message"))
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 1) {
                        ForEach(tab.bookmarks) { bookmark in
                            row(bookmark)
                        }
                    }
                    .padding(Theme.Space.s)
                }
            }
        }
    }

    private func row(_ bookmark: Annotation) -> some View {
        let here = tab.visibleRange?.contains(bookmark.range.start) ?? false
        return Button {
            tab.go(to: bookmark.range.start)
        } label: {
            HStack(alignment: .top, spacing: Theme.Space.s) {
                Image(systemName: "bookmark.fill")
                    .font(.system(size: 11))
                    .foregroundStyle(here ? accent : Theme.tertiary)
                    .padding(.top, 2)
                VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                    Text(bookmark.text.isEmpty ? L10n.text("bookmark.untitled") : bookmark.text)
                        .font(Theme.label)
                        .foregroundStyle(Theme.text.opacity(0.9))
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)
                    Text(AnnotationPlace(tab: tab, anchor: bookmark.range.start).text)
                        .font(Theme.caption)
                        .foregroundStyle(Theme.tertiary)
                        .lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, Theme.Space.s)
            .padding(.vertical, 6)
            .contentShape(Rectangle())
            .hoverable(selected: here)
        }
        .buttonStyle(.plain)
        .contextMenu {
            Button(role: .destructive) {
                remove(bookmark)
            } label: { Label(L10n.text("bookmark.delete"), systemImage: "trash") }
        }
    }

    private func remove(_ bookmark: Annotation) {
        do { try tab.remove(bookmark) } catch { model.alert(L10n.text("annotation.failed"), error.localizedDescription) }
    }
}

/// NotesList lists the highlights of the open book, with their notes. A
/// click goes to the passage; the menu edits, recolors or deletes it.
@MainActor
struct NotesList: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let tab: ReaderTab
    @State private var editing: Annotation?
    @State private var notesOnly = false

    var body: some View {
        let highlights = tab.highlights.filter { !notesOnly || $0.hasNote }
        VStack(alignment: .leading, spacing: 0) {
            PanelTitle(title: L10n.text("panel.notes")) {
                if !tab.highlights.isEmpty {
                    Menu {
                        Toggle(L10n.text("notes.onlyWithNotes"), isOn: $notesOnly)
                        Divider()
                        Button(L10n.text("notes.copyAll")) { copyAll() }
                    } label: {
                        Image(systemName: "ellipsis.circle").font(.system(size: 12)).foregroundStyle(Theme.secondary)
                    }
                    .menuStyle(.borderlessButton)
                    .menuIndicator(.hidden)
                    .fixedSize()
                    .help(L10n.text("library.options"))
                }
            }
            if highlights.isEmpty {
                EmptyState(symbol: "highlighter", title: L10n.text("notes.empty.title"),
                           message: L10n.text("notes.empty.message"))
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: Theme.Space.xs) {
                        ForEach(highlights) { highlight in
                            row(highlight)
                        }
                    }
                    .padding(Theme.Space.s)
                }
            }
        }
    }

    private func row(_ highlight: Annotation) -> some View {
        Button {
            tab.go(to: highlight.range.start)
        } label: {
            VStack(alignment: .leading, spacing: Theme.Space.xs) {
                Text(highlight.text)
                    .font(.system(size: 12, design: .serif))
                    .foregroundStyle(Theme.text.opacity(0.9))
                    .lineLimit(4)
                    .multilineTextAlignment(.leading)
                    .padding(.leading, Theme.Space.s)
                    .overlay(alignment: .leading) {
                        Rectangle().fill(HighlightColor.at(highlight.color).swatch).frame(width: 3)
                    }
                if highlight.hasNote {
                    Text(highlight.note)
                        .font(Theme.label)
                        .foregroundStyle(Theme.secondary)
                        .lineLimit(6)
                        .multilineTextAlignment(.leading)
                }
                Text(AnnotationPlace(tab: tab, anchor: highlight.range.start).text)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(Theme.Space.s)
            .contentShape(Rectangle())
            .hoverable()
        }
        .buttonStyle(.plain)
        .popover(isPresented: Binding { editing?.id == highlight.id } set: { if !$0 { editing = nil } },
                 arrowEdge: .leading) {
            HighlightEditor(tab: tab, annotation: highlight) { editing = nil }
        }
        .contextMenu {
            Button { editing = highlight } label: {
                Label(highlight.hasNote ? L10n.text("note.edit") : L10n.text("note.add"), systemImage: "note.text")
            }
            Menu(L10n.text("highlight.color")) {
                ForEach(HighlightColor.all.indices, id: \.self) { index in
                    Button(HighlightColor.all[index].name) { recolor(highlight, index) }
                }
            }
            Button {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(highlight.text, forType: .string)
            } label: { Label(L10n.text("action.copy"), systemImage: "doc.on.doc") }
            Divider()
            Button(role: .destructive) { remove(highlight) } label: {
                Label(L10n.text("highlight.remove"), systemImage: "trash")
            }
        }
    }

    private func recolor(_ highlight: Annotation, _ color: Int) {
        var changed = highlight
        changed.color = color
        do { try tab.update(changed) } catch { model.alert(L10n.text("annotation.failed"), error.localizedDescription) }
    }

    private func remove(_ highlight: Annotation) {
        do { try tab.remove(highlight) } catch { model.alert(L10n.text("annotation.failed"), error.localizedDescription) }
    }

    /// Copies every highlight and note of the book as Markdown.
    private func copyAll() {
        var lines = ["# \(tab.book.title)"]
        if !tab.book.author.isEmpty { lines.append("_\(tab.book.author)_") }
        for highlight in tab.highlights {
            lines.append("")
            lines.append("> " + highlight.text.replacingOccurrences(of: "\n", with: "\n> "))
            if highlight.hasNote {
                lines += ["", highlight.note]
            }
            lines += ["", "— " + AnnotationPlace(tab: tab, anchor: highlight.range.start).text]
        }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(lines.joined(separator: "\n"), forType: .string)
    }
}

/// AnnotationPlace describes where an anchor is: its page in the current
/// layout and its chapter.
@MainActor
struct AnnotationPlace {
    let tab: ReaderTab
    let anchor: Anchor

    var text: String {
        let chapter = tab.chapter(of: anchor)?.title
        guard let page = tab.pageNumber(of: anchor) else { return chapter ?? "" }
        let pageText = L10n.format("place.page", page)
        return chapter.map { "\(pageText) · \($0)" } ?? pageText
    }
}

import SwiftUI

/// CoverCell is one book of the grid: a click selects it, a double click
/// opens it, and the options button or a secondary click shows its menu. The
/// cover lifts while the pointer is over it.
@MainActor
struct CoverCell: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let book: Book
    let selected: Bool
    /// OnSelect runs after a click selects the book.
    var onSelect: () -> Void = {}
    @State private var hovering = false
    @State private var showInfo = false

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Space.s) {
            cover
                .frame(height: 225, alignment: .bottom)
                .frame(maxWidth: .infinity)
            VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                Text(book.title)
                    .font(Theme.font(12, .medium))
                    .foregroundStyle(Theme.text.opacity(hovering || selected ? 1 : 0.88))
                    .lineLimit(1)
                HStack(spacing: Theme.Space.xs) {
                    Text(caption)
                        .font(Theme.font(11, book.lastOpened == nil ? .semibold : .regular))
                        .foregroundStyle(book.lastOpened == nil ? accent : Theme.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 0)
                    options
                }
            }
        }
        .contentShape(Rectangle())
        .onHover { hovering = $0 }
        .animation(Theme.hoverAnimation, value: hovering)
        .onTapGesture(count: 2) { model.open(book) }
        // Selecting on the first click, alongside the double click, keeps
        // selection from waiting out the double-click interval.
        .simultaneousGesture(TapGesture().onEnded {
            model.selectedBook = book.id
            onSelect()
        })
        .contextMenu { menuItems }
        .popover(isPresented: $showInfo, arrowEdge: .trailing) { BookInfo(book: book) }
        .help(book.author.isEmpty ? book.title : "\(book.title) · \(book.author)")
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(book.author.isEmpty ? book.title : "\(book.title), \(book.author)")
        .accessibilityValue(caption)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction(named: L10n.text("action.open")) { model.open(book) }
    }

    @ViewBuilder private var cover: some View {
        let shape = RoundedRectangle(cornerRadius: 3, style: .continuous)
        Group {
            if let image = model.covers[book.id] {
                Image(nsImage: image).resizable().aspectRatio(contentMode: .fit)
            } else {
                shape.fill(Theme.raised)
                    .overlay(Text(book.title).font(Theme.label).foregroundStyle(Theme.secondary).padding())
                    .aspectRatio(2.0 / 3.0, contentMode: .fit)
            }
        }
        .clipShape(shape)
        .overlay(shape.strokeBorder(Color.white.opacity(hovering ? 0.14 : 0.06)))
        .shadow(color: .black.opacity(hovering ? 0.75 : 0.55), radius: hovering ? 14 : 8, y: hovering ? 7 : 3)
        .overlay(shape.strokeBorder(selected ? accent : .clear, lineWidth: 2.5).padding(-4))
        .scaleEffect(hovering ? 1.03 : 1, anchor: .bottom)
        .offset(y: hovering ? -2 : 0)
    }

    private var options: some View {
        Menu { menuItems } label: {
            Image(systemName: "ellipsis")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Theme.secondary)
                .frame(width: 22, height: 16)
                .contentShape(Rectangle())
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
        .opacity(hovering || selected ? 1 : 0)
        .help(L10n.text("library.options"))
        .accessibilityLabel(L10n.text("library.options"))
    }

    private var caption: String {
        if book.lastOpened == nil { return L10n.text("book.new") }
        if book.isFinished { return L10n.text("book.finished") }
        return L10n.format("book.percentRead", Int(book.fraction * 100))
    }

    @ViewBuilder private var menuItems: some View {
        Button { model.open(book) } label: { Label(L10n.text("action.open"), systemImage: "book") }
        Button { showInfo = true } label: { Label(L10n.text("book.info"), systemImage: "info.circle") }
        Button { model.revealInFinder(book) } label: { Label(L10n.text("action.revealInFinder"), systemImage: "folder") }
        Divider()
        Button(role: .destructive) { model.remove(book) } label: {
            Label(L10n.text("library.remove"), systemImage: "trash")
        }
    }
}

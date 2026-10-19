import SwiftUI

/// BookInfo is the popover with a book's details.
@MainActor
struct BookInfo: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    let book: Book

    var body: some View {
        HStack(alignment: .top, spacing: Theme.Space.l) {
            if let cover = model.covers[book.id] {
                Image(nsImage: cover)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 96)
                    .clipShape(RoundedRectangle(cornerRadius: 3, style: .continuous))
                    .shadow(color: .black.opacity(0.5), radius: 6, y: 2)
            }
            VStack(alignment: .leading, spacing: Theme.Space.s) {
                Text(book.title).font(Theme.font(14, .semibold)).foregroundStyle(Theme.text)
                    .fixedSize(horizontal: false, vertical: true)
                if !book.author.isEmpty {
                    Text(book.author).font(Theme.label).foregroundStyle(Theme.secondary)
                }
                HStack(spacing: Theme.Space.s) {
                    Meter(value: book.fraction, width: 110)
                    Text(book.lastOpened == nil
                        ? L10n.text("book.unread")
                        : L10n.format("book.percentRead", Int(book.fraction * 100)))
                        .font(Theme.digits).foregroundStyle(Theme.secondary)
                }
                .padding(.top, Theme.Space.xxs)
                Grid(alignment: .leading, horizontalSpacing: Theme.Space.s, verticalSpacing: Theme.Space.xxs) {
                    GridRow {
                        Text(L10n.text("book.added")).foregroundStyle(Theme.tertiary)
                        Text(date(book.addedAt)).foregroundStyle(Theme.secondary)
                    }
                    if let opened = book.lastOpened {
                        GridRow {
                            Text(L10n.text("book.lastOpened")).foregroundStyle(Theme.tertiary)
                            Text(opened.formatted(date: .abbreviated, time: .shortened)).foregroundStyle(Theme.secondary)
                        }
                    }
                }
                .font(Theme.caption)
                Text(book.originalPath ?? book.path)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(2)
                    .truncationMode(.middle)
                    .textSelection(.enabled)
                HStack {
                    Button(L10n.text("action.open")) { model.open(book) }
                        .buttonStyle(.borderedProminent)
                        .tint(accent)
                    Button(L10n.text("action.revealInFinder")) { model.revealInFinder(book) }
                }
                .padding(.top, Theme.Space.xs)
            }
        }
        .padding(Theme.Space.l)
        .frame(width: 400)
    }

    private func date(_ seconds: Int64) -> String {
        Date(timeIntervalSince1970: TimeInterval(seconds)).formatted(date: .abbreviated, time: .omitted)
    }
}

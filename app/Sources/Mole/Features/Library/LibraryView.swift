import SwiftUI

/// LibraryView is the grid of covers of the selected shelf. It takes the
/// keyboard: arrows move the selection, Return opens and Delete removes.
/// EPUB files dropped on it are added to the library.
@MainActor
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    @FocusState private var gridFocused: Bool
    @State private var gridWidth: CGFloat = 0
    @State private var dropTargeted = false

    private static let cellWidth: ClosedRange<CGFloat> = 150...176
    private static let spacing: CGFloat = 28

    var body: some View {
        @Bindable var model = model
        let books = model.shelfBooks
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Space.m) {
                Text(model.shelf.title)
                    .font(Theme.heading)
                    .foregroundStyle(Theme.text)
                Text(L10n.bookCount(books.count))
                    .font(Theme.label)
                    .foregroundStyle(Theme.tertiary)
                if model.importing > 0 {
                    HStack(spacing: Theme.Space.xs) {
                        ProgressView().controlSize(.small).scaleEffect(0.7).frame(width: 12, height: 12)
                        Text(L10n.format("import.progress", model.importing))
                    }
                    .font(Theme.label)
                    .foregroundStyle(Theme.secondary)
                }
                Spacer()
                SearchField(prompt: L10n.text("library.search"), text: $model.query)
            }
            .padding(.horizontal, Theme.Space.xl + 4)
            .padding(.top, Theme.Space.m + 2)
            .padding(.bottom, Theme.Space.m)
            .hairline(.bottom)

            if books.isEmpty {
                emptyState
                Spacer(minLength: 0)
            } else {
                grid(books)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .overlay {
            if dropTargeted {
                RoundedRectangle(cornerRadius: Theme.Radius.l)
                    .strokeBorder(accent, lineWidth: 2)
                    .padding(Theme.Space.s)
                    .allowsHitTesting(false)
            }
        }
        .dropDestination(for: URL.self) { urls, _ in
            let paths = urls.filter { $0.isFileURL && $0.pathExtension.lowercased() == "epub" }.map(\.path)
            guard !paths.isEmpty else { return false }
            model.importBooks(paths)
            return true
        } isTargeted: { dropTargeted = $0 }
    }

    private func grid(_ books: [Book]) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: Self.cellWidth.lowerBound,
                                                       maximum: Self.cellWidth.upperBound),
                                             spacing: Self.spacing)],
                          alignment: .leading, spacing: 32) {
                    ForEach(books) { book in
                        CoverCell(book: book, selected: model.selectedBook == book.id) {
                            gridFocused = true
                        }
                        .id(book.id)
                    }
                }
                .background {
                    GeometryReader { geometry in
                        Color.clear
                            .onAppear { gridWidth = geometry.size.width }
                            .onChange(of: geometry.size.width) { _, width in gridWidth = width }
                    }
                }
                .padding(.horizontal, Theme.Space.xl + 4)
                .padding(.top, Theme.Space.xl)
                .padding(.bottom, Theme.Space.xxl)
            }
            .background {
                // A click between covers clears the selection.
                Color.clear
                    .contentShape(Rectangle())
                    .onTapGesture {
                        model.selectedBook = nil
                        gridFocused = true
                    }
            }
            .onAppear {
                gridFocused = true
                if let id = model.selectedBook { proxy.scrollTo(id, anchor: .center) }
            }
            .onChange(of: model.selectedBook) { _, id in
                guard let id else { return }
                Motion.animate(.easeOut(duration: 0.2)) { proxy.scrollTo(id) }
            }
        }
        .focusable()
        .focused($gridFocused)
        .focusEffectDisabled()
        .onMoveCommand { direction in move(direction, in: books) }
        .onKeyPress(.return) {
            guard let book = selected(in: books) else { return .ignored }
            model.open(book)
            return .handled
        }
        .onKeyPress(keys: [.delete, .deleteForward]) { _ in
            guard let book = selected(in: books) else { return .ignored }
            model.remove(book)
            return .handled
        }
    }

    /// Columns is how many covers the adaptive grid fits in a row.
    private var columns: Int {
        max(1, Int((gridWidth + Self.spacing) / (Self.cellWidth.lowerBound + Self.spacing)))
    }

    private func selected(in books: [Book]) -> Book? {
        books.first { $0.id == model.selectedBook }
    }

    /// Moves the selection one cover sideways or one row up or down.
    private func move(_ direction: MoveCommandDirection, in books: [Book]) {
        guard !books.isEmpty else { return }
        guard let index = books.firstIndex(where: { $0.id == model.selectedBook }) else {
            model.selectedBook = books[0].id
            return
        }
        let step: Int
        switch direction {
        case .left: step = -1
        case .right: step = 1
        case .up: step = -columns
        case .down: step = columns
        @unknown default: step = 0
        }
        let target = index + step
        if books.indices.contains(target) { model.selectedBook = books[target].id }
    }

    @ViewBuilder private var emptyState: some View {
        if model.books.isEmpty {
            EmptyState(symbol: "books.vertical", title: L10n.text("library.empty.title"),
                       message: L10n.text("library.empty.message")) {
                Button(L10n.text("library.add")) { model.chooseBooks() }
                    .buttonStyle(.borderedProminent)
                    .tint(accent)
                    .controlSize(.large)
            }
        } else if !model.query.trimmingCharacters(in: .whitespaces).isEmpty {
            EmptyState(symbol: "magnifyingglass", title: L10n.text("library.noResults.title"),
                       message: L10n.format("library.noResults.message", model.query))
        } else {
            EmptyState(symbol: model.shelf.symbol, title: L10n.text("shelf.empty.title"),
                       message: L10n.text("shelf.empty.\(model.shelf.rawValue)"))
        }
    }
}

import SwiftUI

/// FolderImportSheet asks how to add the books in folders the user chose or
/// dropped: whether each folder that holds books becomes a collection, and
/// whether Mole keeps the folders in sync. The choices are remembered for
/// the next time.
@MainActor
struct FolderImportSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent
    @AppStorage(Keys.collections) private var collections = true
    @AppStorage(Keys.watch) private var watch = true
    let request: FolderImport

    /// Keys are the user defaults that remember the choices.
    enum Keys {
        static let collections = "folderImportCollections"
        static let watch = "folderImportWatch"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Space.l) {
            HStack(alignment: .top, spacing: Theme.Space.m) {
                Image(systemName: "folder")
                    .font(.system(size: 28, weight: .light))
                    .foregroundStyle(accent)
                    .frame(width: 36)
                VStack(alignment: .leading, spacing: Theme.Space.xs) {
                    Text(title)
                        .font(Theme.font(15, .semibold))
                        .foregroundStyle(Theme.text)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(request.paths.map { ($0 as NSString).abbreviatingWithTildeInPath }.joined(separator: "\n"))
                        .font(Theme.caption)
                        .foregroundStyle(Theme.tertiary)
                        .lineLimit(3)
                        .truncationMode(.middle)
                    Text(L10n.text("import.folder.detail"))
                        .font(Theme.label)
                        .foregroundStyle(Theme.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, Theme.Space.xxs)
                }
            }
            VStack(alignment: .leading, spacing: Theme.Space.m) {
                option(L10n.text("import.folder.collections"), note: L10n.text("import.folder.collections.note"),
                       isOn: $collections)
                option(request.paths.count == 1 ? L10n.text("import.folder.sync") : L10n.text("import.folders.sync"),
                       note: L10n.text("import.folder.sync.note"), isOn: $watch)
            }
            .padding(.leading, 36 + Theme.Space.m)
            HStack {
                Spacer()
                Button(L10n.text("action.cancel")) { model.folderImport = nil }
                    .keyboardShortcut(.cancelAction)
                Button(L10n.text("import.folder.action")) { confirm() }
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                    .tint(accent)
            }
        }
        .padding(Theme.Space.xl)
        .frame(width: 460)
        .background(Theme.surface)
    }

    private var title: String {
        request.paths.count == 1
            ? L10n.format("import.folder.title", (request.paths[0] as NSString).lastPathComponent)
            : L10n.format("import.folders.title", request.paths.count)
    }

    private func option(_ title: String, note: String, isOn: Binding<Bool>) -> some View {
        Toggle(isOn: isOn) {
            VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                Text(title).font(Theme.body).foregroundStyle(Theme.text)
                Text(note)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .toggleStyle(.checkbox)
    }

    /// Adds the books with the options chosen and closes the sheet.
    func confirm() {
        model.folderImport = nil
        model.importFolders(request.paths, collections: collections, watch: watch)
    }
}

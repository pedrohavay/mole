import SwiftUI

/// FoldersSection is the part of Settings > General that lists the folders
/// Mole keeps in sync, each with Sync Now and a button to stop syncing it.
@MainActor
struct FoldersSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Section(L10n.text("settings.folders")) {
            if model.folders.isEmpty {
                Text(L10n.text("settings.folders.empty"))
                    .font(Theme.caption)
                    .foregroundStyle(Theme.secondary)
            }
            ForEach(model.folders) { folder in
                row(folder)
            }
            Text(L10n.text("settings.folders.note"))
                .font(Theme.caption)
                .foregroundStyle(Theme.secondary)
        }
    }

    private func row(_ folder: WatchedFolder) -> some View {
        HStack(spacing: Theme.Space.s) {
            Image(systemName: "folder")
                .foregroundStyle(Theme.secondary)
                .frame(width: 18)
            VStack(alignment: .leading, spacing: 1) {
                Text(folder.name)
                    .font(Theme.body)
                    .foregroundStyle(Theme.text)
                    .lineLimit(1)
                Text(folder.displayPath)
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(folder.path)
                Text(status(folder))
                    .font(Theme.caption)
                    .foregroundStyle(Theme.tertiary)
                    .lineLimit(1)
            }
            Spacer(minLength: Theme.Space.s)
            if model.syncing.contains(folder.id) {
                ProgressView().controlSize(.small)
            }
            Button(L10n.text("settings.folders.syncNow")) { model.sync(folder) }
                .disabled(model.syncing.contains(folder.id))
            Button {
                model.remove(folder)
            } label: {
                Image(systemName: "minus.circle")
            }
            .buttonStyle(.borderless)
            .help(L10n.text("settings.folders.remove.help"))
            .accessibilityLabel(L10n.text("settings.folders.remove"))
        }
    }

    /// Returns when the folder was last synced and whether it makes
    /// collections.
    private func status(_ folder: WatchedFolder) -> String {
        var parts: [String] = []
        if let synced = folder.lastSynced {
            parts.append(L10n.format("settings.folders.synced",
                                     synced.formatted(.relative(presentation: .named, unitsStyle: .wide))))
        } else {
            parts.append(L10n.text("settings.folders.notSynced"))
        }
        if folder.collections { parts.append(L10n.text("settings.folders.collections")) }
        return parts.joined(separator: " · ")
    }
}

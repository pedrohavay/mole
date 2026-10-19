import SwiftUI

/// LightPanel holds the light controls: text brightness, the extended range
/// switch, the text color and a live readout of the gain and headroom.
@MainActor
struct LightPanel: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent

    var body: some View {
        @Bindable var model = model
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Text(L10n.text("light.title")).font(Theme.font(13, .semibold))
            HStack(spacing: Theme.Space.s) {
                Image(systemName: "sun.min").foregroundStyle(Theme.secondary)
                Slider(value: $model.settings.level, in: ReaderSettings.levels)
                    .tint(accent)
                    .accessibilityLabel(L10n.text("light.level"))
                Image(systemName: "sun.max.fill").foregroundStyle(accent)
            }
            VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                Text(L10n.format("light.readout", decimal(model.gain, 1)))
                Text(L10n.format("light.headroom", decimal(model.headroom.current, 1),
                                 decimal(model.headroom.potential, 0)))
            }
            .font(Theme.digits)
            .foregroundStyle(Theme.secondary)
            Divider()
            Toggle(L10n.text("light.edr"), isOn: $model.settings.edr)
                .toggleStyle(.switch)
                .tint(accent)
            Text(L10n.text("light.tint")).font(Theme.label).foregroundStyle(Theme.secondary)
            Picker(L10n.text("light.tint"), selection: $model.settings.tint) {
                ForEach(Tint.all.indices, id: \.self) { Text(Tint.all[$0].name).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
        }
        .padding(Theme.Space.l)
        .frame(width: 320)
    }
}

/// TextPanel holds the text controls: the size, the font, the paragraphs,
/// the margins, the stroke weight, the page spread, and how the book's
/// pictures and colors show. For a book of fixed pages, whose text keeps its
/// own size, font and spacing, the zoom takes the place of the first four.
@MainActor
struct TextPanel: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent

    var body: some View {
        @Bindable var model = model
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Text(L10n.text("text.title")).font(Theme.font(13, .semibold))
            if let tab = model.activeTab, tab.isFixedLayout {
                ZoomControls(tab: tab)
            } else {
                typography
            }
            label("text.stroke")
            Picker(L10n.text("text.stroke"), selection: $model.settings.gamma) {
                ForEach(Stroke.all, id: \.gamma) { Text($0.name).tag($0.gamma) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            label("text.spread")
            Picker(L10n.text("text.spread"), selection: $model.settings.spread) {
                ForEach(Spread.allCases) { Text($0.title).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            label("pictures.title")
            Picker(L10n.text("pictures.title"), selection: $model.settings.pictures) {
                ForEach(PictureMode.allCases) { Text($0.title).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            Divider()
            Toggle(L10n.text("text.bookColors"), isOn: $model.settings.bookColors)
                .toggleStyle(.switch)
                .tint(accent)
        }
        .padding(Theme.Space.l)
        .frame(width: 320)
    }

    /// Typography holds the controls that set the text of a reflowable book.
    @ViewBuilder private var typography: some View {
        @Bindable var model = model
        Group {
            HStack(spacing: Theme.Space.m) {
                ControlGroup {
                    Button { model.adjustFontSize(by: -1) } label: {
                        Image(systemName: "textformat.size.smaller")
                    }
                    .accessibilityLabel(L10n.text("text.smaller"))
                    Button { model.adjustFontSize(by: 1) } label: {
                        Image(systemName: "textformat.size.larger")
                    }
                    .accessibilityLabel(L10n.text("text.bigger"))
                }
                .frame(width: 96)
                Text(L10n.format("text.points", Int(model.settings.fontSize)))
                    .font(Theme.digits)
                    .foregroundStyle(Theme.secondary)
            }
            label("text.font")
            FontGrid(selection: $model.settings.font)
            label("text.paragraphs")
            HStack(spacing: Theme.Space.s) {
                Picker(L10n.text("text.lineSpacing"), selection: $model.settings.lineSpacing) {
                    ForEach(LineSpacing.allCases) { Text($0.title).tag($0) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                Picker(L10n.text("text.alignment"), selection: $model.settings.justify) {
                    Label(L10n.text("alignment.left"), systemImage: "text.alignleft").tag(false)
                    Label(L10n.text("alignment.justified"), systemImage: "text.justify").tag(true)
                }
                .pickerStyle(.segmented)
                .labelStyle(.iconOnly)
                .labelsHidden()
                .fixedSize()
                .help(L10n.text("text.alignment"))
            }
            label("text.margins")
            Picker(L10n.text("text.margins"), selection: $model.settings.margins) {
                ForEach(PageMargins.allCases) { Text($0.title).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
        }
    }

    private func label(_ key: String) -> some View {
        Text(L10n.text(key)).font(Theme.label).foregroundStyle(Theme.secondary)
    }
}

/// ZoomControls set how large the pages of a book of fixed pages are drawn:
/// a step in or out, or fitted whole or by their width. A note says why the
/// text controls are not there.
@MainActor
struct ZoomControls: View {
    let tab: ReaderTab

    var body: some View {
        let viewport = tab.state.viewport
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Text(L10n.text("zoom.title")).font(Theme.label).foregroundStyle(Theme.secondary)
            HStack(spacing: Theme.Space.m) {
                ControlGroup {
                    Button { tab.zoom(by: -1) } label: { Image(systemName: "minus.magnifyingglass") }
                        .accessibilityLabel(L10n.text("zoom.out"))
                    Button { tab.zoom(by: 1) } label: { Image(systemName: "plus.magnifyingglass") }
                        .accessibilityLabel(L10n.text("zoom.in"))
                }
                .frame(width: 96)
                Text(viewport.map { L10n.format("zoom.percent", $0.percent) } ?? "")
                    .font(Theme.digits)
                    .foregroundStyle(Theme.secondary)
            }
            Picker(L10n.text("zoom.title"), selection: Binding<Fit?> {
                viewport.flatMap { $0.fit == .scale ? nil : $0.fit }
            } set: { fit in
                if let fit { tab.zoom(to: fit) }
            }) {
                Text(L10n.text("zoom.fitPage")).tag(Fit?.some(.page))
                Text(L10n.text("zoom.fitWidth")).tag(Fit?.some(.width))
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            Text(L10n.text("text.fixed.note"))
                .font(Theme.caption)
                .foregroundStyle(Theme.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// ZoomMenu is the status bar's item for a book of fixed pages: how large
/// the pages are drawn, as a percentage of their printed size, and a menu of
/// the zooms.
@MainActor
struct ZoomMenu: View {
    let tab: ReaderTab
    let viewport: Viewport

    var body: some View {
        Menu {
            Button(L10n.text("zoom.fitPage")) { tab.zoom(to: .page) }
            Button(L10n.text("zoom.fitWidth")) { tab.zoom(to: .width) }
            Divider()
            Button(L10n.text("zoom.in")) { tab.zoom(by: 1) }
            Button(L10n.text("zoom.out")) { tab.zoom(by: -1) }
        } label: {
            Text(L10n.format("zoom.percent", viewport.percent))
                .font(Theme.digits)
                .foregroundStyle(Theme.secondary)
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
        .help(L10n.text("zoom.title"))
        .accessibilityLabel(L10n.text("zoom.title"))
        .accessibilityValue(L10n.format("zoom.percent", viewport.percent))
    }
}

/// FontGrid offers the book's own fonts and the installed fonts the core can
/// set the text in, each name shown in its own font.
@MainActor
struct FontGrid: View {
    @Environment(\.accent) private var accent
    @Binding var selection: String

    private let columns = [GridItem(.flexible(), spacing: Theme.Space.xs + 2),
                           GridItem(.flexible(), spacing: Theme.Space.xs + 2)]

    var body: some View {
        LazyVGrid(columns: columns, spacing: Theme.Space.xs + 2) {
            ForEach([""] + ReaderFont.installed, id: \.self) { family in
                tile(family)
            }
        }
    }

    private func tile(_ family: String) -> some View {
        let selected = family == selection
        return Button {
            selection = family
        } label: {
            Text(ReaderFont.name(family))
                .font(family.isEmpty ? Theme.body : ReaderFont.sample(family, size: 14))
                .foregroundStyle(selected ? .primary : .secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
                .padding(.horizontal, Theme.Space.xs)
                .frame(maxWidth: .infinity)
                .frame(height: 28)
                .background(RoundedRectangle(cornerRadius: Theme.Radius.s)
                    .fill(selected ? accent.opacity(0.22) : Theme.hover))
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.s)
                    .strokeBorder(selected ? accent : Theme.border))
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ReaderFont.name(family))
        .accessibilityAddTraits(selected ? .isSelected : [])
    }
}

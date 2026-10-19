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

/// TextPanel holds the text size, the stroke weight, the page spread and
/// how the book's pictures and colors show.
@MainActor
struct TextPanel: View {
    @Environment(AppModel.self) private var model
    @Environment(\.accent) private var accent

    var body: some View {
        @Bindable var model = model
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Text(L10n.text("text.title")).font(Theme.font(13, .semibold))
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
            Text(L10n.text("text.stroke")).font(Theme.label).foregroundStyle(Theme.secondary)
            Picker(L10n.text("text.stroke"), selection: $model.settings.gamma) {
                ForEach(Stroke.all, id: \.gamma) { Text($0.name).tag($0.gamma) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            Text(L10n.text("text.spread")).font(Theme.label).foregroundStyle(Theme.secondary)
            Picker(L10n.text("text.spread"), selection: $model.settings.spread) {
                ForEach(Spread.allCases) { Text($0.title).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            Text(L10n.text("pictures.title")).font(Theme.label).foregroundStyle(Theme.secondary)
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
        .frame(width: 300)
    }
}

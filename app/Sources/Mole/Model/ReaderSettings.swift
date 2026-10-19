import SwiftUI

/// ReaderSettings are the settings shared by every book. The core stores
/// them in the library database.
struct ReaderSettings: Codable, Equatable {
    /// Level is the text brightness as a fraction of the EDR headroom.
    var level: Double
    /// EDR lets the text go brighter than SDR white.
    var edr: Bool
    /// Tint indexes Tint.all.
    var tint: Int
    /// Gamma is the exponent applied to glyph coverage; see Stroke.
    var gamma: Double
    var fontSize: Double
    var spread: Spread
    /// Accent is the interface accent as "#RRGGBB"; empty means the default.
    var accent: String
    var images: ImageLight

    static let fontSizes = 12.0...40.0
    static let levels = 0.02...1.0

    init(level: Double, edr: Bool, tint: Int, gamma: Double, fontSize: Double, spread: Spread, accent: String,
         images: ImageLight = .dim) {
        self.level = level
        self.edr = edr
        self.tint = tint
        self.gamma = gamma
        self.fontSize = fontSize
        self.spread = spread
        self.accent = accent
        self.images = images
    }

    // Images arrived after the first release of the core, so a library that
    // predates it has no value and gets the default.
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        level = try values.decode(Double.self, forKey: .level)
        edr = try values.decode(Bool.self, forKey: .edr)
        tint = try values.decode(Int.self, forKey: .tint)
        gamma = try values.decode(Double.self, forKey: .gamma)
        fontSize = try values.decode(Double.self, forKey: .fontSize)
        spread = try values.decode(Spread.self, forKey: .spread)
        accent = try values.decode(String.self, forKey: .accent)
        images = try values.decodeIfPresent(ImageLight.self, forKey: .images) ?? .dim
    }
}

/// ImageLight is how bright pictures are on the page. Line art glows like
/// the text; photographs keep their tones. Neither goes above SDR white.
enum ImageLight: Int, Codable, CaseIterable, Identifiable {
    /// Pictures at about a third of the text's light, capped at SDR white.
    case dim
    case off
    /// Pictures as bright as the text, capped at SDR white.
    case full

    var id: Int { rawValue }

    var title: String { L10n.text("images.\(String(describing: self))") }

    /// Returns the light of the image layer for a text gain.
    func gain(for textGain: Double) -> Double {
        switch self {
        case .dim: min(textGain, 1) * 0.35
        case .off: 0
        case .full: min(textGain, 1)
        }
    }
}

/// Spread is how many pages the reader shows side by side.
enum Spread: Int, Codable, CaseIterable, Identifiable {
    /// Two pages when the window is wide enough for both, else one.
    case automatic
    case single
    case double

    var id: Int { rawValue }

    var title: String {
        switch self {
        case .automatic: L10n.text("spread.automatic")
        case .single: L10n.text("spread.single")
        case .double: L10n.text("spread.double")
        }
    }
}

/// Tint is a color for the lit text, in linear Display P3.
struct Tint {
    let key: String
    let rgb: SIMD3<Float>

    var name: String { L10n.text("tint.\(key)") }

    static let all = [
        Tint(key: "white", rgb: [1, 1, 1]),
        Tint(key: "warm", rgb: [1, 0.72, 0.45]),
        Tint(key: "amber", rgb: [1, 0.42, 0.06]),
        Tint(key: "red", rgb: [1, 0.05, 0.02]),
    ]

    /// Returns the tint at index, wrapping out-of-range values.
    static func at(_ index: Int) -> Tint {
        all[((index % all.count) + all.count) % all.count]
    }
}

/// Stroke is a text weight, made by raising glyph coverage to a power: a
/// larger exponent thins the antialiased edges.
struct Stroke {
    let key: String
    let gamma: Double

    var name: String { L10n.text("stroke.\(key)") }

    static let all = [
        Stroke(key: "thin", gamma: 3),
        Stroke(key: "regular", gamma: 2.2),
        Stroke(key: "bold", gamma: 1.5),
    ]
}

/// Accent is a preset interface accent color.
struct Accent: Identifiable {
    let key: String
    let hex: String

    var id: String { key }
    var name: String { L10n.text("accent.\(key)") }
    var color: Color { Color(hexString: hex) ?? Theme.defaultAccent }

    /// The presets; the first is the default.
    static let presets = [
        Accent(key: "red", hex: "#E5484D"),
        Accent(key: "orange", hex: "#F76B15"),
        Accent(key: "amber", hex: "#FFB224"),
        Accent(key: "green", hex: "#30A46C"),
        Accent(key: "teal", hex: "#12A594"),
        Accent(key: "blue", hex: "#3E63DD"),
        Accent(key: "violet", hex: "#8E4EC6"),
        Accent(key: "pink", hex: "#D6409F"),
    ]
}

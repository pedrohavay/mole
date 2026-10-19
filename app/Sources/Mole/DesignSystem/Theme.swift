import SwiftUI

/// Theme holds the design tokens. Views take colors, spacing, radii and type
/// from here, so the look can be tuned in one place. The accent is the one
/// token the user picks; views read it from the environment (\.accent).
enum Theme {
    // Surfaces, from the window background up. The page itself is pure
    // black so the EDR text is the only light there.
    static let canvas = Color(hex: 0x0B0B0D)
    static let sidebar = Color(hex: 0x111114)
    static let surface = Color(hex: 0x151518)
    static let raised = Color(hex: 0x1D1D21)
    static let page = Color.black

    // Overlays for interaction states and hairlines.
    static let hover = Color.white.opacity(0.05)
    static let pressed = Color.white.opacity(0.08)
    static let selected = Color.white.opacity(0.09)
    static let border = Color.white.opacity(0.07)

    // Text.
    static let text = Color(hex: 0xE8E8EB)
    static let secondary = Color(hex: 0x8E8E95)
    // Tertiary still meets WCAG AA (4.5:1) on the canvas and the sidebar.
    static let tertiary = Color(hex: 0x7C7C84)

    // Accent: a flat red, warm like a reading lamp. Users can change it.
    static let defaultAccent = Color(hex: 0xE5484D)
    static let positive = Color(hex: 0x3DD68C)
    static let caution = Color(hex: 0xF5C451)

    enum Space {
        static let xxs: CGFloat = 2
        static let xs: CGFloat = 4
        static let s: CGFloat = 8
        static let m: CGFloat = 12
        static let l: CGFloat = 16
        static let xl: CGFloat = 24
        static let xxl: CGFloat = 32
    }

    enum Radius {
        static let s: CGFloat = 5
        static let m: CGFloat = 7
        static let l: CGFloat = 10
    }

    enum Size {
        /// Bar is the height of the top bars; TrafficLights centers the
        /// window buttons in it.
        static let bar: CGFloat = 38
        static let statusBar: CGFloat = 28
        /// TrafficLights is the room the window buttons take on the left.
        static let trafficLights: CGFloat = 78
        static let sidebar: ClosedRange<CGFloat> = 220...360
        static let contents: ClosedRange<CGFloat> = 220...420
        /// MinReader is the narrowest the page may get before side panels
        /// make way for it.
        static let minReader: CGFloat = 440
    }

    static func font(_ size: CGFloat, _ weight: Font.Weight = .regular) -> Font {
        .system(size: size, weight: weight)
    }

    static let body = font(13)
    static let label = font(12)
    static let caption = font(11)
    static let digits = Font.system(size: 11).monospacedDigit()
    static let heading = Font.system(size: 22, weight: .bold, design: .serif)

    static let hoverAnimation = Animation.easeOut(duration: 0.14)
}

extension Color {
    /// Creates a color from 0xRRGGBB in sRGB.
    init(hex: UInt32, opacity: Double = 1) {
        self.init(.sRGB,
                  red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255,
                  opacity: opacity)
    }

    /// Creates a color from "#RRGGBB", or returns nil for anything else.
    init?(hexString: String) {
        let digits = hexString.hasPrefix("#") ? hexString.dropFirst() : Substring(hexString)
        guard digits.count == 6, let value = UInt32(digits, radix: 16) else { return nil }
        self.init(hex: value)
    }

    /// Returns the color as "#RRGGBB" in sRGB, or nil if it has no sRGB form.
    var hexString: String? {
        guard let rgb = NSColor(self).usingColorSpace(.sRGB) else { return nil }
        let channels = [rgb.redComponent, rgb.greenComponent, rgb.blueComponent]
            .map { Int(($0.clamped(to: 0...1) * 255).rounded()) }
        return String(format: "#%02X%02X%02X", channels[0], channels[1], channels[2])
    }
}

private struct AccentKey: EnvironmentKey {
    static let defaultValue = Theme.defaultAccent
}

extension EnvironmentValues {
    /// Accent is the interface accent color the user picked.
    var accent: Color {
        get { self[AccentKey.self] }
        set { self[AccentKey.self] = newValue }
    }
}

import AppKit

/// Headroom is the EDR headroom of a screen: how many times brighter than SDR
/// white it can show right now, and at most. macOS raises the current value
/// over a few seconds once EDR content is on screen and the app is frontmost.
struct Headroom: Equatable {
    var current = 1.0
    var potential = 1.0

    /// Reports whether the screen is showing values above SDR white.
    var isActive: Bool { current > 1.05 }

    /// Reports whether the headroom has reached what the screen can give.
    /// At a high system brightness the potential is lower than the screen's
    /// maximum, so this is relative to it.
    var isSettled: Bool { current >= potential * 0.98 }

    /// Fraction is how far the current headroom is from 1 to the potential.
    var fraction: Double { potential > 1 ? (current - 1) / (potential - 1) : 0 }

    init(current: Double = 1, potential: Double = 1) {
        self.current = current
        self.potential = potential
    }

    init(screen: NSScreen) {
        self.init(current: max(1, screen.maximumExtendedDynamicRangeColorComponentValue),
                  potential: max(1, screen.maximumPotentialExtendedDynamicRangeColorComponentValue))
    }
}

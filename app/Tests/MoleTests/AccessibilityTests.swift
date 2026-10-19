import AppKit
import SwiftUI
import XCTest
@testable import Mole

final class ContrastTests: XCTestCase {
    /// Returns the sRGB components of color drawn with appearance.
    private func components(_ color: Color, _ appearance: NSAppearance) -> [CGFloat] {
        var result: [CGFloat] = []
        appearance.performAsCurrentDrawingAppearance {
            if let rgb = NSColor(color).usingColorSpace(.sRGB) {
                result = [rgb.redComponent, rgb.greenComponent, rgb.blueComponent, rgb.alphaComponent]
            }
        }
        return result
    }

    func testTokensFollowIncreaseContrast() throws {
        let dark = try XCTUnwrap(NSAppearance(named: .darkAqua))
        let normal = Color(hex: 0x808080)
        let increased = Color(hex: 0xC0C0C0)
        let off = Theme.contrasting(normal, increased: increased) { _ in false }
        let on = Theme.contrasting(normal, increased: increased) { _ in true }
        XCTAssertEqual(components(off, dark), components(normal, dark))
        XCTAssertEqual(components(on, dark), components(increased, dark))
        // Without the setting the dark appearance keeps the normal tokens.
        if !NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast {
            XCTAssertFalse(Theme.isContrastIncreased(for: dark))
            XCTAssertEqual(components(Theme.secondary, dark), components(Color(hex: 0x8E8E95), dark))
        }
    }
}

@MainActor
final class MotionTests: XCTestCase {
    func testRunsTheChangeEitherWay() {
        var changed = false
        Motion.animate { changed = true }
        XCTAssertTrue(changed)
    }
}

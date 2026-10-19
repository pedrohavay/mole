import AppKit
import SwiftUI

/// Motion runs the interface's animations, or leaves them out when Reduce
/// Motion is on in the Accessibility settings. Views read the setting from
/// the environment (\.accessibilityReduceMotion); code that changes the model
/// outside a view, such as menu commands, goes through Motion.
enum Motion {
    /// IsReduced reports whether the user asked for less motion.
    static var isReduced: Bool { NSWorkspace.shared.accessibilityDisplayShouldReduceMotion }

    /// Runs body inside animation, or without one when motion is reduced.
    @MainActor
    static func animate(_ animation: Animation = .easeInOut(duration: 0.2), _ body: () -> Void) {
        if isReduced {
            body()
        } else {
            withAnimation(animation, body)
        }
    }
}

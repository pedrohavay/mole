import Foundation
import Observation
import Sparkle

/// Updater keeps Mole up to date with Sparkle. Releases publish an appcast
/// next to the app's zip, and Info.plist names it (SUFeedURL) along with the
/// public key that verifies each update (SUPublicEDKey). Development and CI
/// builds have no key, so their updater never starts and the menus and
/// Settings hide its controls.
@Observable @MainActor
final class Updater {
    /// IsEnabled is true when the bundle is set up for updates.
    let isEnabled: Bool
    /// CanCheck is true when a check can start now, that is, when the
    /// updater runs and no check is in progress.
    private(set) var canCheck = false
    /// ChecksAutomatically mirrors Sparkle's own setting, which Sparkle keeps
    /// in the user defaults and may change itself when it asks the user.
    var checksAutomatically = false {
        didSet {
            guard let updater = controller?.updater,
                  updater.automaticallyChecksForUpdates != checksAutomatically else { return }
            updater.automaticallyChecksForUpdates = checksAutomatically
        }
    }

    @ObservationIgnored private let controller: SPUStandardUpdaterController?
    @ObservationIgnored private var observations: [NSKeyValueObservation] = []

    /// Creates the updater for the bundle described by info. With start
    /// false, as in the app's self-checks, it never starts, so nothing
    /// reaches the network or the user defaults.
    init(info: [String: Any] = Bundle.main.infoDictionary ?? [:], start: Bool = true) {
        isEnabled = Self.isConfigured(info)
        guard isEnabled, start else {
            controller = nil
            return
        }
        let controller = SPUStandardUpdaterController(startingUpdater: true, updaterDelegate: nil, userDriverDelegate: nil)
        self.controller = controller
        let updater = controller.updater
        canCheck = updater.canCheckForUpdates
        checksAutomatically = updater.automaticallyChecksForUpdates
        // Sparkle changes both on the main thread.
        observations = [
            updater.observe(\.canCheckForUpdates, options: [.new]) { [weak self] updater, _ in
                MainActor.assumeIsolated { self?.canCheck = updater.canCheckForUpdates }
            },
            updater.observe(\.automaticallyChecksForUpdates, options: [.new]) { [weak self] updater, _ in
                MainActor.assumeIsolated { self?.checksAutomatically = updater.automaticallyChecksForUpdates }
            },
        ]
    }

    /// Checks for updates now and shows Sparkle's progress and result.
    func checkForUpdates() {
        controller?.checkForUpdates(nil)
    }

    /// Returns whether info names an HTTPS feed and an EdDSA public key,
    /// which Sparkle needs to start.
    nonisolated static func isConfigured(_ info: [String: Any]) -> Bool {
        guard let feed = info["SUFeedURL"] as? String, URL(string: feed)?.scheme == "https",
              let key = info["SUPublicEDKey"] as? String else { return false }
        return Data(base64Encoded: key)?.count == 32
    }
}

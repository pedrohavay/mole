import AppKit
import Sparkle

/// SmokeTest checks, with `--smoke-test`, that a built app starts: that it
/// links and opens a library through the Go core, finds its string tables,
/// compiles its Metal pipeline, and loads Sparkle from its own bundle with
/// a feed and key Sparkle accepts. It reports to standard error and quits
/// with status 0, or 1 on the first failure. Releases run it on the signed
/// app, because the hardened runtime can stop an app that runs unsigned.
@MainActor
enum SmokeTest {
    static func run() {
        guard L10n.text("app.name") == "Mole" else { fail("the string tables are missing") }
        if case .failure(let error) = EDRPipeline.shared { fail("Metal: \(error.localizedDescription)") }
        checkUpdater()
        report("ok")
        exit(0)
    }

    /// Checks that Sparkle comes from the app's Frameworks folder, where
    /// scripts/bundle.sh puts it, and that a bundle with a public key also
    /// has a feed and a key the updater can use. It does not start the
    /// updater, which would reach the network and the user defaults.
    private static func checkUpdater() {
        let main = Bundle.main
        if main.bundleIdentifier != nil, let frameworks = main.privateFrameworksURL?.resolvingSymlinksInPath() {
            let sparkle = Bundle(for: SPUUpdater.self).bundleURL.resolvingSymlinksInPath()
            guard sparkle.path.hasPrefix(frameworks.path + "/") else { fail("Sparkle loads from \(sparkle.path)") }
        }
        let info = main.infoDictionary ?? [:]
        guard let key = info["SUPublicEDKey"] as? String, !key.isEmpty else { return report("updates off, no public key") }
        guard Updater.isConfigured(info) else { fail("SUPublicEDKey or SUFeedURL is not valid") }
        report("updates from \(info["SUFeedURL"] as? String ?? "")")
    }

    /// Reports a failure and quits with status 1.
    static func fail(_ message: String) -> Never {
        report("failed: \(message)")
        exit(1)
    }

    private static func report(_ message: String) {
        FileHandle.standardError.write(Data("smoke test: \(message)\n".utf8))
    }
}

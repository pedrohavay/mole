import AppKit

/// SmokeTest checks, with `--smoke-test`, that a built app starts: that it
/// links and opens a library through the Go core, finds its string tables,
/// and compiles its Metal pipeline. It reports to standard error and quits
/// with status 0, or 1 on the first failure. Releases run it on the signed
/// app, because the hardened runtime can stop an app that runs unsigned.
@MainActor
enum SmokeTest {
    static func run() {
        guard L10n.text("app.name") == "Mole" else { fail("the string tables are missing") }
        if case .failure(let error) = EDRPipeline.shared { fail("Metal: \(error.localizedDescription)") }
        report("ok")
        exit(0)
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

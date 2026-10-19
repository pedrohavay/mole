import AppKit
import SwiftUI

/// MoleApp is the macOS app: one window with the library and the open books,
/// and the Settings window.
@main
struct MoleApp: App {
    @NSApplicationDelegateAdaptor private var delegate: AppDelegate
    @State private var launch: Launch
    @State private var updater: Updater

    init() {
        // The library opens before the first scene is built, so the menus
        // are made with the model in place.
        let launch = Launch()
        launch.start()
        _launch = State(initialValue: launch)
        // The self-checks quit on their own and leave updates alone.
        let arguments = LaunchArguments.parse()
        _updater = State(initialValue: Updater(start: !arguments.smokeTest && arguments.verifyDirectory == nil))
    }

    var body: some Scene {
        Window(L10n.text("app.name"), id: "main") {
            Group {
                switch launch.state {
                case .ready(let model):
                    ShellView()
                        .environment(model)
                        .environment(\.accent, model.accent)
                        .tint(model.accent)
                case .failed(let message):
                    LaunchFailure(message: message)
                }
            }
            .frame(minWidth: 900, minHeight: 560)
            .preferredColorScheme(.dark)
        }
        .windowStyle(.hiddenTitleBar)
        .defaultSize(width: 1420, height: 900)
        .commands { MoleCommands(model: launch.model, updater: updater) }

        Settings {
            if let model = launch.model {
                SettingsView()
                    .environment(model)
                    .environment(updater)
                    .environment(\.accent, model.accent)
                    .tint(model.accent)
                    .preferredColorScheme(.dark)
            }
        }
    }
}

/// Launch opens the library as the app starts.
@Observable @MainActor
final class Launch {
    enum State {
        case ready(AppModel)
        case failed(String)
    }

    private(set) var state = State.failed(L10n.text("error.library.notOpened"))

    var model: AppModel? {
        if case .ready(let model) = state { return model }
        return nil
    }

    func start() {
        let arguments = LaunchArguments.parse()
        do {
            let model = try AppModel(library: Library(path: try Self.libraryPath()))
            if arguments.smokeTest {
                // NSApp does not exist yet while the App is initialized.
                DispatchQueue.main.async { SmokeTest.run() }
                state = .ready(model)
                return
            }
            model.importBooks(arguments.books)
            state = .ready(model)
            AppDelegate.openFiles = { model.openFiles($0) }
            if let directory = arguments.verifyDirectory {
                // NSApp does not exist yet while the App is initialized.
                DispatchQueue.main.async {
                    Verifier(model: model, directory: directory, folderTree: arguments.verifyFolder).start()
                }
            }
        } catch {
            if arguments.smokeTest { SmokeTest.fail("cannot open the library: \(error.localizedDescription)") }
            state = .failed(error.localizedDescription)
        }
    }

    /// Returns the path of the library database, in Application Support,
    /// creating its folder if needed. MOLE_LIBRARY overrides it.
    static func libraryPath() throws -> String {
        if let path = ProcessInfo.processInfo.environment["MOLE_LIBRARY"], !path.isEmpty { return path }
        let support = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask,
                                                  appropriateFor: nil, create: true)
            .appendingPathComponent("Mole", isDirectory: true)
        try FileManager.default.createDirectory(at: support, withIntermediateDirectories: true)
        return support.appendingPathComponent("library.db").path
    }
}

/// AppDelegate receives the books opened from Finder and makes a bare
/// executable (swift run) behave like a bundled app.
final class AppDelegate: NSObject, NSApplicationDelegate {
    /// OpenFiles handles files opened from Finder. Files that arrive before
    /// the library is ready wait until it is set.
    @MainActor static var openFiles: (([String]) -> Void)? {
        didSet { deliver() }
    }

    @MainActor private static var pending: [String] = []

    @MainActor private static func deliver() {
        guard let openFiles, !pending.isEmpty else { return }
        let paths = pending
        pending = []
        openFiles(paths)
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // A bundled app is regular already; only a bare executable needs it.
        guard Bundle.main.bundleIdentifier == nil else { return }
        NSApp.setActivationPolicy(.regular)
        NSApp.activate()
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        let paths = urls.filter(\.isFileURL).map(\.path)
        MainActor.assumeIsolated {
            Self.pending += paths
            Self.deliver()
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

/// LaunchFailure is the window when the library cannot be opened: what went
/// wrong and a way to look at the database or quit.
@MainActor
private struct LaunchFailure: View {
    let message: String

    var body: some View {
        EmptyState(symbol: "exclamationmark.triangle", title: L10n.text("error.library.title"), message: message) {
            HStack {
                Button(L10n.text("error.library.reveal")) {
                    if let path = try? Launch.libraryPath() {
                        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
                    }
                }
                Button(L10n.text("error.library.quit")) { NSApp.terminate(nil) }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(Theme.canvas)
    }
}

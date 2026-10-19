import CoreServices
import Foundation

/// FolderWatcher tells when files change inside a set of folders, through
/// FSEvents. Changes come in bursts (a copy writes a file many times), so
/// they are gathered until the folders have been quiet for a moment, and
/// onChange then runs on the main thread with the folders that changed.
/// The stream refers to the watcher without retaining it, so a watcher must
/// be stopped before it goes away; AppModel keeps its own for the life of
/// the app.
@MainActor
final class FolderWatcher {
    /// Quiet is how long the folders must go without a change before
    /// onChange runs.
    static let quiet: TimeInterval = 1.5

    private let ignored: String
    private let onChange: @MainActor (Set<String>) -> Void
    private var folders: [String] = []
    /// Real holds the folders with symbolic links resolved, as FSEvents
    /// reports paths (/private/tmp rather than /tmp), by the path watched.
    private var real: [String: String] = [:]
    private var stream: FSEventStreamRef?
    private var changed: Set<String> = []
    private var pending: DispatchWorkItem?

    /// Creates a watcher that reports nothing under ignored, the library's
    /// own folder: writing the database there must not look like a change
    /// to a folder that holds it.
    init(ignoring ignored: String, onChange: @escaping @MainActor (Set<String>) -> Void) {
        self.ignored = Self.realPath(ignored)
        self.onChange = onChange
    }

    /// Watches folders from now on, in place of the folders watched before.
    func watch(_ folders: [String]) {
        let folders = folders.sorted()
        guard folders != self.folders else { return }
        stopStream()
        self.folders = folders
        real = Dictionary(folders.map { ($0, Self.realPath($0)) }, uniquingKeysWith: { first, _ in first })
        // Changes still waiting out the quiet period stay, for the folders
        // still watched.
        changed.formIntersection(folders)
        guard !folders.isEmpty else { return }
        var context = FSEventStreamContext(version: 0, info: Unmanaged.passUnretained(self).toOpaque(),
                                           retain: nil, release: nil, copyDescription: nil)
        let flags = FSEventStreamCreateFlags(kFSEventStreamCreateFlagUseCFTypes | kFSEventStreamCreateFlagFileEvents)
        guard let stream = FSEventStreamCreate(nil, { _, info, count, paths, _, _ in
            guard let info, let paths = unsafeBitCast(paths, to: NSArray.self) as? [String] else { return }
            let watcher = Unmanaged<FolderWatcher>.fromOpaque(info).takeUnretainedValue()
            // The stream runs on the main queue.
            MainActor.assumeIsolated { watcher.received(Array(paths.prefix(count))) }
        }, &context, folders as CFArray, FSEventStreamEventId(kFSEventStreamEventIdSinceNow), 0.5, flags) else {
            NSLog("Mole: cannot watch %@", folders.joined(separator: ", "))
            return
        }
        FSEventStreamSetDispatchQueue(stream, .main)
        FSEventStreamStart(stream)
        self.stream = stream
    }

    /// Stops watching every folder and drops the changes not reported yet.
    func stop() {
        pending?.cancel()
        pending = nil
        changed = []
        folders = []
        real = [:]
        stopStream()
    }

    private func stopStream() {
        guard let stream else { return }
        FSEventStreamStop(stream)
        FSEventStreamInvalidate(stream)
        FSEventStreamRelease(stream)
        self.stream = nil
    }

    private func received(_ paths: [String]) {
        // FSEvents reports real paths. NSString's standardizingPath would
        // turn /private/tmp/x into /tmp/x whenever x exists.
        for path in paths {
            if Self.path(path, isIn: ignored) { continue }
            for folder in folders where Self.path(path, isIn: real[folder] ?? folder) {
                changed.insert(folder)
            }
        }
        guard !changed.isEmpty else { return }
        pending?.cancel()
        let work = DispatchWorkItem { [weak self] in
            MainActor.assumeIsolated { self?.deliver() }
        }
        pending = work
        DispatchQueue.main.asyncAfter(deadline: .now() + Self.quiet, execute: work)
    }

    /// Reports whether path is folder or lies inside it.
    nonisolated static func path(_ path: String, isIn folder: String) -> Bool {
        guard !folder.isEmpty else { return false }
        return path == folder || path.hasPrefix(folder.hasSuffix("/") ? folder : folder + "/")
    }

    /// Returns path with every symbolic link resolved, or path itself when
    /// it does not exist. Unlike NSString's resolvingSymlinksInPath it keeps
    /// /private in front of /private/tmp, as FSEvents does.
    nonisolated static func realPath(_ path: String) -> String {
        guard let resolved = realpath(path, nil) else { return path }
        defer { free(resolved) }
        return String(cString: resolved)
    }

    private func deliver() {
        let folders = changed
        changed = []
        pending = nil
        if !folders.isEmpty { onChange(folders) }
    }
}

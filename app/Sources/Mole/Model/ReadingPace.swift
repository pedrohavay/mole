import Foundation

/// ReadingPace estimates how fast the reader reads, in seconds per
/// non-whitespace character, the unit the core's text offsets count. It
/// learns from the time spent on each screen before turning forward, and
/// keeps what it learned between launches.
struct ReadingPace: Equatable {
    /// SecondsPerCharacter starts at about 200 words a minute, a typical
    /// pace for reading on a screen, until the reader's own pace is known.
    private(set) var secondsPerCharacter = ReadingPace.defaultSecondsPerCharacter
    /// Samples counts the screens learned from.
    private(set) var samples = 0

    static let defaultSecondsPerCharacter = 60.0 / 1000
    /// Weight of a new sample in the moving average.
    private static let weight = 0.15
    /// A screen read faster than this was skimmed or skipped, and one read
    /// slower was left open while the reader was away.
    static let plausible: ClosedRange<Double> = 0.004...0.4

    /// Learns from a screen of characters that took seconds to read. It
    /// ignores screens with too little text and implausible paces.
    mutating func record(characters: Int, seconds: TimeInterval) {
        guard characters >= 200 else { return }
        let pace = seconds / Double(characters)
        guard Self.plausible.contains(pace) else { return }
        secondsPerCharacter = samples == 0 ? pace : secondsPerCharacter + Self.weight * (pace - secondsPerCharacter)
        samples += 1
    }

    /// Returns how long characters take to read at this pace.
    func time(for characters: Int) -> TimeInterval {
        Double(max(0, characters)) * secondsPerCharacter
    }

    // MARK: Persistence

    private static let key = "readingPace"

    /// Reads the pace saved by an earlier launch.
    static func load(from defaults: UserDefaults = .standard) -> ReadingPace {
        var pace = ReadingPace()
        if let saved = defaults.dictionary(forKey: key),
           let seconds = saved["secondsPerCharacter"] as? Double, plausible.contains(seconds),
           let samples = saved["samples"] as? Int {
            pace.secondsPerCharacter = seconds
            pace.samples = samples
        }
        return pace
    }

    func save(to defaults: UserDefaults = .standard) {
        defaults.set(["secondsPerCharacter": secondsPerCharacter, "samples": samples], forKey: Self.key)
    }
}

/// PaceStore keeps the reading pace that every open book learns from and
/// estimates with, saved in user defaults.
@MainActor
final class PaceStore {
    static let shared = PaceStore()

    private(set) var pace = ReadingPace.load()

    func record(characters: Int, seconds: TimeInterval) {
        let before = pace
        pace.record(characters: characters, seconds: seconds)
        if pace != before { pace.save() }
    }
}

/// TimeLeft is how long the rest of a chapter and of a book should take.
struct TimeLeft: Equatable {
    var chapter: TimeInterval?
    var book: TimeInterval

    /// Formats a duration the way the status bar shows it: "12 min",
    /// "4 h 30 min", rounded to whole minutes.
    static func format(_ seconds: TimeInterval) -> String {
        let formatter = DateComponentsFormatter()
        formatter.unitsStyle = .abbreviated
        formatter.allowedUnits = seconds >= 3600 ? [.hour, .minute] : [.minute]
        formatter.zeroFormattingBehavior = .dropAll
        let minutes = max(60, (seconds / 60).rounded() * 60)
        return formatter.string(from: minutes) ?? ""
    }
}

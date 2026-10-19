import AVFoundation
import Foundation
import NaturalLanguage
import Observation

/// ReadAloud speaks a book with the system's voices, a sentence at a time,
/// from a place in it to the end of the book or until it is stopped. It
/// marks the sentence being spoken on the page and turns the pages as the
/// speech moves past the screen, across chapters.
@Observable @MainActor
final class ReadAloud {
    enum State: Equatable {
        case idle, speaking, paused
    }

    /// Sentence is a sentence to speak and where it is in the book.
    struct Sentence: Equatable {
        let text: String
        let range: TextRange
        /// Map locates the characters of text in the book.
        let map: TextMap
        /// EndsParagraph is set on the last sentence of a paragraph, which
        /// is followed by a short pause.
        let endsParagraph: Bool

        /// Returns the anchor of the character at a UTF-16 offset of text.
        func anchor(at index: Int) -> Anchor {
            map.anchor(at: index, in: text)
        }
    }

    private(set) var state = State.idle
    /// BookID is the book being read aloud, while it is.
    private(set) var bookID: Int64?
    /// Sentence is the sentence being spoken.
    private(set) var sentence: Sentence?

    /// VoiceIdentifier is the voice picked in Settings, or empty for the
    /// best voice in the language of each book.
    var voiceIdentifier: String {
        didSet { UserDefaults.standard.set(voiceIdentifier, forKey: Keys.voice) }
    }

    /// Rate is the speed of the speech, from AVSpeechUtteranceMinimumSpeechRate
    /// to AVSpeechUtteranceMaximumSpeechRate. It applies from the next sentence.
    var rate: Float {
        didSet { UserDefaults.standard.set(rate, forKey: Keys.rate) }
    }

    /// Volume of the voice, from 0 to 1. The verifier speaks silently.
    @ObservationIgnored var volume: Float = 1

    @ObservationIgnored private weak var tab: ReaderTab?
    @ObservationIgnored private let synthesizer = AVSpeechSynthesizer()
    @ObservationIgnored private let delegate = SpeechDelegate()
    @ObservationIgnored private var voice: AVSpeechSynthesisVoice?
    /// Queued holds the sentences read from the book and not yet handed to
    /// the synthesizer; spoken holds those handed to it, until they end.
    @ObservationIgnored private var queued: [Sentence] = []
    @ObservationIgnored private var spoken: [ObjectIdentifier: (utterance: AVSpeechUtterance, sentence: Sentence)] = [:]
    /// Next is where the next paragraphs are read from; nil once the end of
    /// the book is reached.
    @ObservationIgnored private var next: Anchor?

    /// Ahead is how many sentences the synthesizer holds at a time, so it
    /// goes from one to the next without a gap.
    private static let ahead = 3
    /// Batch is about how many characters are read from the book at a time.
    private static let batch = 1500
    /// Rates are the speeds offered in Settings.
    static let rates: ClosedRange<Float> = 0.3...0.7

    private enum Keys {
        static let voice = "readAloudVoice"
        static let rate = "readAloudRate"
    }

    init() {
        let defaults = UserDefaults.standard
        voiceIdentifier = defaults.string(forKey: Keys.voice) ?? ""
        rate = defaults.object(forKey: Keys.rate) as? Float ?? AVSpeechUtteranceDefaultSpeechRate
        synthesizer.delegate = delegate
        delegate.onEvent = { [weak self] id, event in self?.handle(event, of: id) }
    }

    var isActive: Bool { state != .idle }

    // MARK: Control

    /// Starts reading tab aloud at anchor, stopping whatever was being read.
    func start(_ tab: ReaderTab, from anchor: Anchor) {
        stop()
        self.tab = tab
        bookID = tab.id
        next = anchor
        voice = Self.voice(for: tab.language, picked: voiceIdentifier)
        fill()
        guard !spoken.isEmpty else { return stop() }
        state = .speaking
    }

    /// Starts reading tab aloud from its selection or the top of the
    /// screen, or pauses or resumes the reading under way.
    func toggle(_ tab: ReaderTab) {
        switch state {
        case .speaking where bookID == tab.id: pause()
        case .paused where bookID == tab.id: resume()
        default:
            guard let anchor = tab.selection?.start ?? tab.topOfScreen else { return }
            tab.clearSelection()
            start(tab, from: anchor)
        }
    }

    func pause() {
        guard state == .speaking else { return }
        synthesizer.pauseSpeaking(at: .word)
        state = .paused
    }

    func resume() {
        guard state == .paused else { return }
        synthesizer.continueSpeaking()
        state = .speaking
    }

    /// Stops speaking and clears the mark on the page.
    func stop() {
        state = .idle
        queued = []
        next = nil
        spoken = [:]
        if synthesizer.isSpeaking || synthesizer.isPaused { synthesizer.stopSpeaking(at: .immediate) }
        tab?.markSpoken(nil)
        tab = nil
        bookID = nil
        sentence = nil
    }

    // MARK: Speaking

    /// Hands sentences to the synthesizer until it holds a few, reading
    /// more of the book as needed.
    private func fill() {
        while spoken.count < Self.ahead {
            if queued.isEmpty {
                guard next != nil else { return }
                readAhead()
                continue
            }
            speak(queued.removeFirst())
        }
    }

    /// Reads the next paragraphs of the book into the queue.
    private func readAhead() {
        guard let tab, let from = next else { return }
        let paragraphs: [Paragraph]
        do {
            paragraphs = try tab.session.paragraphs(of: TextRange(start: from, end: .endOfBook), limit: Self.batch)
        } catch {
            NSLog("Mole: cannot read %@ aloud: %@", tab.book.title, error.localizedDescription)
            paragraphs = []
        }
        next = paragraphs.last?.range.end
        queued += Self.sentences(in: paragraphs)
    }

    private func speak(_ sentence: Sentence) {
        let utterance = AVSpeechUtterance(string: sentence.text)
        utterance.voice = voice
        utterance.rate = rate
        utterance.volume = volume
        utterance.postUtteranceDelay = sentence.endsParagraph ? 0.2 : 0
        spoken[ObjectIdentifier(utterance)] = (utterance, sentence)
        synthesizer.speak(utterance)
    }

    private func handle(_ event: SpeechDelegate.Event, of id: ObjectIdentifier) {
        guard let (_, sentence) = spoken[id] else { return }
        switch event {
        case .started:
            self.sentence = sentence
            tab?.markSpoken(sentence.range)
            show(sentence.range.start)
        case .word(let range):
            show(sentence.anchor(at: range.location))
        case .finished:
            spoken[id] = nil
            fill()
            if spoken.isEmpty { stop() }
        case .cancelled:
            spoken[id] = nil
        }
    }

    /// Turns to the screen that holds anchor, or scrolls fixed pages larger
    /// than the view to it, when it is not on screen.
    private func show(_ anchor: Anchor) {
        guard let tab, !tab.isOnScreen(anchor) else { return }
        tab.go(to: anchor)
    }

    // MARK: Text

    /// Returns the sentences of paragraphs, with the parts of a paragraph
    /// that runs over a page break joined into one.
    static func sentences(in paragraphs: [Paragraph]) -> [Sentence] {
        var sentences: [Sentence] = []
        var text = ""
        var pieces: [TextMap.Piece] = []
        func flush() {
            sentences += split(text, map: TextMap(pieces: pieces))
            text = ""
            pieces = []
        }
        for paragraph in paragraphs {
            if !paragraph.continued {
                flush()
            } else if !text.isEmpty {
                text += " "
            }
            pieces.append(TextMap.Piece(start: text.utf16.count, anchor: paragraph.range.start))
            text += paragraph.text
        }
        flush()
        return sentences
    }

    /// Splits the text of a paragraph into sentences.
    private static func split(_ text: String, map: TextMap) -> [Sentence] {
        guard !map.pieces.isEmpty else { return [] }
        let tokenizer = NLTokenizer(unit: .sentence)
        tokenizer.string = text
        var ranges: [Range<String.Index>] = []
        tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
            var range = range
            while range.lowerBound < range.upperBound, text[range.lowerBound].isWhitespace {
                range = text.index(after: range.lowerBound)..<range.upperBound
            }
            while range.lowerBound < range.upperBound, text[text.index(before: range.upperBound)].isWhitespace {
                range = range.lowerBound..<text.index(before: range.upperBound)
            }
            if Anchor.count(in: text[range]) > 0 { ranges.append(range) }
            return true
        }
        return ranges.enumerated().map { index, range in
            let start = range.lowerBound.utf16Offset(in: text)
            let end = range.upperBound.utf16Offset(in: text)
            let first = map.anchor(at: start, in: text)
            // The sentence's own map starts at its first character and
            // takes every later piece of the paragraph that it reaches.
            var pieces = [TextMap.Piece(start: 0, anchor: first)]
            pieces += map.pieces.filter { $0.start > start && $0.start < end }
                .map { TextMap.Piece(start: $0.start - start, anchor: $0.anchor) }
            return Sentence(text: String(text[range]),
                            range: TextRange(start: first, end: map.anchor(at: end, in: text)),
                            map: TextMap(pieces: pieces), endsParagraph: index == ranges.count - 1)
        }
    }

    // MARK: Languages and voices

    /// MinimumSample is how many characters of text a guess of a book's
    /// language needs; a page of names or a table of contents is too little.
    static let minimumSample = 2000
    /// MinimumConfidence is how sure the guess must be to be taken.
    static let minimumConfidence = 0.9

    /// Returns the language to read a book in, as a BCP 47 tag: the language
    /// the book declares when it names a real one; else the language of its
    /// text, when a long sample leaves little doubt; else whichever of the
    /// user's preferred languages the text is closest to, or the first of
    /// them. Text is read only when the book declares no language.
    static func readingLanguage(declared: String?, text: @autoclosure () -> String, preferred: [String]) -> String? {
        if let declared, let tag = languageTag(declared) { return tag }
        let sample = text()
        if sample.count >= minimumSample {
            let recognizer = NLLanguageRecognizer()
            recognizer.processString(sample)
            if let best = recognizer.languageHypotheses(withMaximum: 1).max(by: { $0.value < $1.value }),
               best.value >= minimumConfidence {
                return best.key.rawValue
            }
        }
        let languages = preferred.compactMap(languageTag)
        guard !sample.isEmpty, languages.count > 1 else { return languages.first }
        let recognizer = NLLanguageRecognizer()
        recognizer.languageConstraints = languages.map { NLLanguage(rawValue: language(of: $0)) }
        recognizer.processString(sample)
        let closest = recognizer.dominantLanguage?.rawValue
        return languages.first { language(of: $0) == closest } ?? languages.first
    }

    /// Returns tag with its language written as a two-letter ISO 639 code
    /// when there is one ("eng-GB" becomes "en-GB", "pt_BR" "pt-BR"), or nil
    /// when tag names no language, as "und", "mul" or "English" do.
    static func languageTag(_ tag: String) -> String? {
        var parts = tag.replacingOccurrences(of: "_", with: "-").split(separator: "-").map(String.init)
        guard let first = parts.first?.lowercased(), !["und", "mul", "mis", "zxx"].contains(first) else { return nil }
        let code = Locale.LanguageCode(first)
        guard code.isISOLanguage else { return nil }
        parts[0] = code.identifier(.alpha2) ?? first
        return parts.joined(separator: "-")
    }

    /// Returns the voice for a book in language: the voice picked in
    /// Settings when it speaks that language, else the best installed voice
    /// for it, from the region the language names, the user's region or the
    /// language's usual one, in that order. Nil leaves the choice to the
    /// system.
    static func voice(for language: String?, picked: String) -> AVSpeechSynthesisVoice? {
        let chosen = picked.isEmpty ? nil : AVSpeechSynthesisVoice(identifier: picked)
        guard let language else { return chosen }
        if let chosen, Self.language(of: chosen.language) == Self.language(of: language) { return chosen }
        // The usual region is en-US for English, pt-BR for Portuguese.
        let named = Locale.Language(identifier: language).region?.identifier
        let user = Locale.current.region?.identifier
        let likely = Locale.Language(identifier: language).maximalIdentifier
        let usual = Locale.Language(identifier: likely).region?.identifier
        func score(_ voice: AVSpeechSynthesisVoice) -> (Int, Int) {
            let region = Locale(identifier: voice.language).region?.identifier
            let rank = region == named ? 3 : region == user ? 2 : region == usual ? 1 : 0
            return (rank, voice.quality.rawValue)
        }
        return voices(in: [language]).max { score($0) < score($1) } ?? AVSpeechSynthesisVoice(language: language)
    }

    /// Returns the installed voices for languages, without the novelty
    /// voices, sorted by language and name.
    static func voices(in languages: [String]) -> [AVSpeechSynthesisVoice] {
        let wanted = Set(languages.map(language(of:)))
        return AVSpeechSynthesisVoice.speechVoices()
            .filter { wanted.contains(language(of: $0.language)) && !$0.voiceTraits.contains(.isNoveltyVoice) }
            .sorted { ($0.language, $0.name) < ($1.language, $1.name) }
    }

    /// Returns the language part of a BCP 47 code: "pt" for "pt-BR".
    static func language(of code: String) -> String {
        String(code.prefix { $0 != "-" && $0 != "_" }).lowercased()
    }
}

/// TextMap locates the characters of a text in a book. The text is made of
/// parts of the book's text laid end to end; each piece is where a part
/// starts in the text, in UTF-16 code units, with the anchor of its first
/// counted character.
struct TextMap: Equatable {
    struct Piece: Equatable {
        let start: Int
        let anchor: Anchor
    }

    let pieces: [Piece]

    /// Returns the anchor of the first counted character at or after a
    /// UTF-16 offset of text, or the place after the last one.
    func anchor(at index: Int, in text: String) -> Anchor {
        guard let piece = pieces.last(where: { $0.start <= index }) ?? pieces.first else {
            return Anchor(chapter: 0, offset: 0)
        }
        let utf16 = text.utf16
        let from = utf16.index(utf16.startIndex, offsetBy: min(piece.start, utf16.count))
        let to = utf16.index(utf16.startIndex, offsetBy: min(max(index, piece.start), utf16.count))
        return Anchor(chapter: piece.anchor.chapter, offset: piece.anchor.offset + Anchor.count(in: text[from..<to]))
    }
}

/// SpeechDelegate hands the synthesizer's progress to ReadAloud on the main
/// thread.
private final class SpeechDelegate: NSObject, AVSpeechSynthesizerDelegate {
    enum Event {
        case started
        /// Word is about to speak the characters of range, in UTF-16 code
        /// units of the utterance.
        case word(NSRange)
        case finished
        case cancelled
    }

    /// OnEvent receives each event with the utterance it is about.
    var onEvent: (@MainActor (ObjectIdentifier, Event) -> Void)?

    private func send(_ event: Event, _ utterance: AVSpeechUtterance) {
        let id = ObjectIdentifier(utterance)
        DispatchQueue.main.async { [weak self] in
            MainActor.assumeIsolated { self?.onEvent?(id, event) }
        }
    }

    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didStart utterance: AVSpeechUtterance) {
        send(.started, utterance)
    }

    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, willSpeakRangeOfSpeechString characterRange: NSRange,
                           utterance: AVSpeechUtterance) {
        send(.word(characterRange), utterance)
    }

    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        send(.finished, utterance)
    }

    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        send(.cancelled, utterance)
    }
}

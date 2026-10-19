import Foundation

/// Formats a number with the given fraction digits and the user's decimal
/// separator ("8.0" in English, "8,0" in Portuguese).
func decimal(_ value: Double, _ digits: Int) -> String {
    let formatter = NumberFormatter()
    formatter.locale = .current
    formatter.minimumFractionDigits = digits
    formatter.maximumFractionDigits = digits
    return formatter.string(from: NSNumber(value: value)) ?? String(format: "%.\(digits)f", value)
}

/// Writes how long ago a date was, compactly: "49m", "3h", "2d".
func compactAge(_ date: Date, now: Date = Date()) -> String {
    let seconds = max(0, now.timeIntervalSince(date))
    switch seconds {
    case ..<60: return L10n.text("age.now")
    case ..<3600: return L10n.format("age.minutes", Int(seconds / 60))
    case ..<86_400: return L10n.format("age.hours", Int(seconds / 3600))
    default: return L10n.format("age.days", Int(seconds / 86_400))
    }
}

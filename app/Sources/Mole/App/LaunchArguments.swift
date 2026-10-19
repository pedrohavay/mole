import Foundation

/// LaunchArguments are EPUB files to add to the library and, for
/// development, `--verify-dir dir` to capture the window into dir and quit,
/// or `--smoke-test` to check that the app starts and quit (see SmokeTest).
struct LaunchArguments: Equatable {
    var books: [String] = []
    var verifyDirectory: String?
    var smokeTest = false

    static func parse(_ arguments: [String] = CommandLine.arguments) -> LaunchArguments {
        var result = LaunchArguments()
        var rest = arguments.dropFirst()
        while let argument = rest.popFirst() {
            if argument == "--verify-dir" {
                result.verifyDirectory = rest.popFirst()
            } else if argument == "--smoke-test" {
                result.smokeTest = true
            } else if argument.lowercased().hasSuffix(".epub") {
                result.books.append(argument)
            }
        }
        return result
    }
}

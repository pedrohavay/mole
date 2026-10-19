import Foundation

/// LaunchArguments are books to add to the library (see BookFiles) and, for
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
            } else if BookFiles.accepts(argument) {
                result.books.append(argument)
            }
        }
        return result
    }
}

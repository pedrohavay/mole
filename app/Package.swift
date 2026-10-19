// swift-tools-version:5.10
// Mole's macOS app. The user interface is SwiftUI; the reading core (library,
// import, layout and rendering of the EDR mask) is the Go static library
// built from ../core/cmd/molecore. `make core` builds it and copies its header
// into Sources/MoleCore.
import Foundation
import PackageDescription

let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().path
let coreLibrary = "\(root)/../core/build"
let homebrewLibrary = ProcessInfo.processInfo.environment["MUPDF_LIB_DIR"] ?? "/opt/homebrew/lib"

let coreLinking: [LinkerSetting] = [
    .unsafeFlags(["-L", coreLibrary, "-L", homebrewLibrary]),
    .linkedLibrary("molecore"),
    // A Go c-archive does not carry the C libraries its cgo code needs, so
    // MuPDF is linked here.
    .linkedLibrary("mupdf"),
    .linkedLibrary("mupdf-third"),
    .linkedLibrary("resolv"),
    .linkedFramework("CoreFoundation"),
]

let package = Package(
    name: "Mole",
    defaultLocalization: "en",
    platforms: [.macOS(.v14)],
    targets: [
        .systemLibrary(name: "MoleCore", path: "Sources/MoleCore"),
        .executableTarget(
            name: "Mole",
            dependencies: ["MoleCore"],
            resources: [.process("Resources")],
            linkerSettings: coreLinking
        ),
        .testTarget(
            name: "MoleTests",
            dependencies: ["Mole"],
            linkerSettings: coreLinking
        ),
    ]
)

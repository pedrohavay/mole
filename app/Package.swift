// swift-tools-version:5.10
// Mole's macOS app. The user interface is SwiftUI; the reading core (library,
// import, layout and rendering of the EDR mask) is the Go static library
// built from ../core/cmd/molecore. `make core` builds it and copies its header
// into Sources/MoleCore.
import Foundation
import PackageDescription

let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().path
let coreLibrary = "\(root)/../core/build"

// libmolecore.a already holds MuPDF: `make core` merges MuPDF's static
// libraries into it, so the app does not need to know where MuPDF is. The
// core finds installed fonts through CoreText.
let coreLinking: [LinkerSetting] = [
    .unsafeFlags(["-L", coreLibrary]),
    .linkedLibrary("molecore"),
    .linkedLibrary("resolv"),
    .linkedFramework("CoreFoundation"),
    .linkedFramework("CoreText"),
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

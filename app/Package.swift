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
// libraries into it, so the app does not need to know where MuPDF is.
let coreLinking: [LinkerSetting] = [
    .unsafeFlags(["-L", coreLibrary]),
    .linkedLibrary("molecore"),
    .linkedLibrary("resolv"),
    .linkedFramework("CoreFoundation"),
]

let package = Package(
    name: "Mole",
    defaultLocalization: "en",
    platforms: [.macOS(.v14)],
    dependencies: [
        // Sparkle checks for and installs updates. It is pinned to one
        // release; scripts/bundle.sh embeds its framework in Mole.app.
        .package(url: "https://github.com/sparkle-project/Sparkle", exact: "2.10.0"),
    ],
    targets: [
        .systemLibrary(name: "MoleCore", path: "Sources/MoleCore"),
        .executableTarget(
            name: "Mole",
            dependencies: ["MoleCore", .product(name: "Sparkle", package: "Sparkle")],
            resources: [.process("Resources")],
            // Inside Mole.app the executable finds Sparkle.framework in
            // Contents/Frameworks.
            linkerSettings: coreLinking + [
                .unsafeFlags(["-Xlinker", "-rpath", "-Xlinker", "@executable_path/../Frameworks"]),
            ]
        ),
        .testTarget(
            name: "MoleTests",
            dependencies: ["Mole"],
            linkerSettings: coreLinking
        ),
    ]
)

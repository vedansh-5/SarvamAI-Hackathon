// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "GuideApp",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(
            name: "GuideApp",
            path: "Sources/GuideApp",
            linkerSettings: [
                .linkedFramework("AppKit"),
                .linkedFramework("AVFoundation"),
                .linkedFramework("Carbon"),
                .linkedFramework("SwiftUI"),
            ]
        )
    ]
)

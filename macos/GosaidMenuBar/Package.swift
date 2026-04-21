// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "GosaidMenuBar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "GosaidMenuBar",
            path: "Sources/GosaidMenuBar"
        )
    ]
)

// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "DetectRadarBar",
    platforms: [
        .macOS(.v13),
    ],
    products: [
        .library(name: "DetectRadarCore", targets: ["DetectRadarCore"]),
        .executable(name: "DetectRadarBar", targets: ["DetectRadarBar"]),
    ],
    targets: [
        .target(
            name: "DetectRadarCore",
            linkerSettings: [
                .linkedFramework("CFNetwork"),
                .linkedFramework("Network"),
                .linkedFramework("SystemConfiguration"),
            ]
        ),
        .executableTarget(
            name: "DetectRadarBar",
            dependencies: ["DetectRadarCore"],
            linkerSettings: [
                .linkedFramework("AppKit"),
                .linkedFramework("ServiceManagement"),
            ]
        ),
        .testTarget(
            name: "DetectRadarCoreTests",
            dependencies: ["DetectRadarCore"]
        ),
    ]
)

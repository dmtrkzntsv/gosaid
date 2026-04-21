import Foundation

// Mirrors internal/platform/paths.go.  macOS uses
// ~/Library/Application Support/gosaid for config and
// ~/Library/Logs/gosaid for logs.
enum Paths {
    private static let appName = "gosaid"

    static var configDir: URL {
        FileManager.default
            .urls(for: .applicationSupportDirectory, in: .userDomainMask)
            .first!
            .appendingPathComponent(appName)
    }

    static var configFile: URL {
        configDir.appendingPathComponent("config.json")
    }

    static var logDir: URL {
        FileManager.default
            .urls(for: .libraryDirectory, in: .userDomainMask)
            .first!
            .appendingPathComponent("Logs")
            .appendingPathComponent(appName)
    }

    static var logFile: URL {
        logDir.appendingPathComponent("gosaid.log")
    }

    /// Path to the bundled Go daemon.  When running inside the `.app`
    /// bundle this resolves to `Contents/MacOS/gosaid`.  When running a
    /// `swift run` dev build we fall back to `$PATH`.
    static var daemonExe: URL {
        let bundled = Bundle.main.bundleURL
            .appendingPathComponent("Contents/MacOS/gosaid")
        if FileManager.default.isExecutableFile(atPath: bundled.path) {
            return bundled
        }
        return URL(fileURLWithPath: "/usr/local/bin/gosaid")
    }
}

import Foundation

/// Manages the lifecycle of the gosaid child process.  stderr is appended
/// to ~/Library/Logs/gosaid/gosaid.log; stdout is swallowed.
final class DaemonProcess {
    private let queue = DispatchQueue(label: "dev.gosaid.ui.daemon")
    private var process: Process?
    private var logHandle: FileHandle?

    var onExit: (() -> Void)?

    var isRunning: Bool {
        queue.sync {
            guard let p = process else { return false }
            return p.isRunning
        }
    }

    func start() throws {
        try queue.sync {
            if let p = process, p.isRunning { return }

            let exe = Paths.daemonExe
            guard FileManager.default.isExecutableFile(atPath: exe.path) else {
                throw NSError(
                    domain: "GosaidMenuBar",
                    code: 1,
                    userInfo: [NSLocalizedDescriptionKey:
                        "gosaid binary not found at \(exe.path)"]
                )
            }

            try FileManager.default.createDirectory(
                at: Paths.logDir,
                withIntermediateDirectories: true
            )
            if !FileManager.default.fileExists(atPath: Paths.logFile.path) {
                FileManager.default.createFile(atPath: Paths.logFile.path, contents: nil)
            }
            let handle = try FileHandle(forWritingTo: Paths.logFile)
            handle.seekToEndOfFile()
            let banner = "--- gosaid started \(ISO8601DateFormatter().string(from: Date())) ---\n"
            handle.write(Data(banner.utf8))

            let p = Process()
            p.executableURL = exe
            p.standardError = handle
            p.standardOutput = handle
            p.terminationHandler = { [weak self] proc in
                self?.queue.async {
                    let tail = "--- gosaid exited status=\(proc.terminationStatus) ---\n"
                    try? self?.logHandle?.write(contentsOf: Data(tail.utf8))
                    self?.logHandle?.closeFile()
                    self?.logHandle = nil
                    self?.process = nil
                }
                self?.onExit?()
            }

            try p.run()
            self.process = p
            self.logHandle = handle
        }
    }

    func stop() {
        queue.sync {
            guard let p = process, p.isRunning else {
                logHandle?.closeFile()
                logHandle = nil
                return
            }
            p.interrupt()  // SIGINT — the daemon's signal handler catches this.
        }
        // Wait briefly for graceful exit; escalate to SIGTERM if needed.
        let deadline = Date().addingTimeInterval(2.0)
        while Date() < deadline {
            if !isRunning { return }
            Thread.sleep(forTimeInterval: 0.05)
        }
        queue.sync {
            process?.terminate()
        }
        let hardDeadline = Date().addingTimeInterval(1.0)
        while Date() < hardDeadline {
            if !isRunning { return }
            Thread.sleep(forTimeInterval: 0.05)
        }
    }

    func restart() throws {
        stop()
        // Small pause so audio device and pidfile release.
        Thread.sleep(forTimeInterval: 0.2)
        try start()
    }
}

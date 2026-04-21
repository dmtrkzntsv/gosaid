import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    private let daemon = DaemonProcess()
    private var settingsWindowController: SettingsWindowController?
    private var statusMenuItem: NSMenuItem!

    func applicationDidFinishLaunching(_ notification: Notification) {
        buildStatusItem()
        daemon.onExit = { [weak self] in
            DispatchQueue.main.async {
                self?.setStatus(self?.daemon.isRunning == true ? "Running" : "Stopped")
            }
        }
        startDaemon()
    }

    func applicationWillTerminate(_ notification: Notification) {
        daemon.stop()
    }

    private func buildStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            // SF Symbol "mic" — template icon adapts to light/dark menu bar.
            let symbol = NSImage(
                systemSymbolName: "mic",
                accessibilityDescription: "Gosaid"
            )
            symbol?.isTemplate = true
            button.image = symbol
            button.toolTip = "Gosaid"
        }

        let menu = NSMenu()

        statusMenuItem = NSMenuItem(title: "Status: starting…", action: nil, keyEquivalent: "")
        statusMenuItem.isEnabled = false
        menu.addItem(statusMenuItem)
        menu.addItem(.separator())

        addItem(to: menu, title: "Settings…", action: #selector(openSettings), key: ",")
        addItem(to: menu, title: "Restart daemon", action: #selector(restartDaemon), key: "r")

        menu.addItem(.separator())

        addItem(to: menu, title: "Open config file", action: #selector(openConfigFile), key: "")
        addItem(to: menu, title: "Open logs folder", action: #selector(openLogsFolder), key: "")

        menu.addItem(.separator())
        addItem(to: menu, title: "Quit", action: #selector(quit), key: "q")

        statusItem.menu = menu
    }

    @discardableResult
    private func addItem(
        to menu: NSMenu,
        title: String,
        action: Selector,
        key: String
    ) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: key)
        item.target = self
        menu.addItem(item)
        return item
    }

    private func setStatus(_ text: String) {
        statusMenuItem.title = "Status: \(text)"
    }

    private func startDaemon() {
        do {
            try daemon.start()
            setStatus("Running")
        } catch {
            setStatus("Not started")
            presentError("Failed to start gosaid", error: error)
        }
    }

    @objc private func openSettings() {
        if settingsWindowController == nil {
            settingsWindowController = SettingsWindowController(onSave: { [weak self] in
                self?.saveAndRestart()
            })
        }
        NSApp.activate(ignoringOtherApps: true)
        settingsWindowController?.showWindow(nil)
    }

    private func saveAndRestart() {
        setStatus("Restarting…")
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            guard let self else { return }
            do {
                try self.daemon.restart()
                DispatchQueue.main.async {
                    self.setStatus(self.daemon.isRunning ? "Running" : "Stopped")
                }
            } catch {
                DispatchQueue.main.async {
                    self.setStatus("Error")
                    self.presentError("Failed to restart gosaid", error: error)
                }
            }
        }
    }

    @objc private func restartDaemon() {
        saveAndRestart()
    }

    @objc private func openConfigFile() {
        _ = try? ConfigStore.load()
        NSWorkspace.shared.open(Paths.configFile)
    }

    @objc private func openLogsFolder() {
        try? FileManager.default.createDirectory(
            at: Paths.logDir,
            withIntermediateDirectories: true
        )
        NSWorkspace.shared.open(Paths.logDir)
    }

    @objc private func quit() {
        NSApp.terminate(nil)
    }

    private func presentError(_ summary: String, error: Error) {
        let alert = NSAlert()
        alert.messageText = summary
        alert.informativeText = error.localizedDescription
        alert.alertStyle = .warning
        alert.addButton(withTitle: "OK")
        alert.runModal()
    }
}

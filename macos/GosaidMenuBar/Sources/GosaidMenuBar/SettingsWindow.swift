import AppKit
import SwiftUI

final class SettingsWindowController: NSWindowController, NSWindowDelegate {
    private let viewModel: SettingsViewModel

    init(onSave: @escaping () -> Void) {
        let vm = SettingsViewModel(onSave: onSave)
        self.viewModel = vm

        let hosting = NSHostingController(rootView: SettingsView(viewModel: vm))
        hosting.view.frame = NSRect(x: 0, y: 0, width: 640, height: 480)

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 640, height: 480),
            styleMask: [.titled, .closable, .resizable, .miniaturizable],
            backing: .buffered,
            defer: false
        )
        window.title = "Gosaid — Settings"
        window.contentViewController = hosting
        window.minSize = NSSize(width: 520, height: 380)
        window.center()
        window.isReleasedWhenClosed = false

        super.init(window: window)
        window.delegate = self
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) not supported") }

    override func showWindow(_ sender: Any?) {
        viewModel.reload()
        super.showWindow(sender)
    }
}

@MainActor
final class SettingsViewModel: ObservableObject {
    @Published var config: Config = .makeDefault()
    @Published var errorMessage: String?
    let onSave: () -> Void

    init(onSave: @escaping () -> Void) {
        self.onSave = onSave
        reload()
    }

    func reload() {
        do {
            config = try ConfigStore.load()
            errorMessage = nil
        } catch {
            errorMessage = "Failed to read config.json: \(error.localizedDescription). Loaded defaults."
            config = .makeDefault()
        }
    }

    func save() -> Bool {
        do {
            // Basic structural validation mirroring internal/config/validate.go.
            if config.drivers.isEmpty {
                throw Self.validationError("at least one driver is required")
            }
            let endpoints = config.drivers.flatMap { $0.endpoints }
            if endpoints.isEmpty {
                throw Self.validationError("at least one endpoint is required")
            }
            for ep in endpoints {
                if ep.id.trimmingCharacters(in: .whitespaces).isEmpty {
                    throw Self.validationError("every endpoint needs an id")
                }
                if ep.config.apiBase.trimmingCharacters(in: .whitespaces).isEmpty {
                    throw Self.validationError("endpoint \"\(ep.id)\": api_base is required")
                }
            }
            if config.toggleMaxSeconds <= 0 {
                throw Self.validationError("toggle_max_seconds must be positive")
            }
            try ConfigStore.save(config)
            errorMessage = nil
            onSave()
            return true
        } catch {
            errorMessage = error.localizedDescription
            return false
        }
    }

    private static func validationError(_ msg: String) -> NSError {
        NSError(
            domain: "GosaidMenuBar",
            code: 2,
            userInfo: [NSLocalizedDescriptionKey: msg]
        )
    }

    func addEndpoint() {
        if config.drivers.isEmpty {
            config.drivers.append(
                Driver(driver: "openai_compatible", endpoints: [])
            )
        }
        config.drivers[0].endpoints.append(
            Endpoint(
                id: "",
                config: OpenAICompatibleConfig(apiBase: "", apiKey: "")
            )
        )
    }

    func removeEndpoint(at offsets: IndexSet) {
        guard !config.drivers.isEmpty else { return }
        config.drivers[0].endpoints.remove(atOffsets: offsets)
    }
}

struct SettingsView: View {
    @ObservedObject var viewModel: SettingsViewModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            TabView {
                PreferencesTab(viewModel: viewModel)
                    .tabItem { Text("Preferences") }
                EndpointsTab(viewModel: viewModel)
                    .tabItem { Text("Endpoints") }
            }
            .padding(12)

            if let err = viewModel.errorMessage {
                Text(err)
                    .foregroundStyle(.red)
                    .font(.footnote)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 16)
                    .padding(.bottom, 4)
            }

            Divider()
            HStack {
                Button("Edit config.json in default editor…") {
                    NSWorkspace.shared.open(Paths.configFile)
                }
                Spacer()
                Button("Cancel") {
                    viewModel.reload()
                    NSApp.keyWindow?.close()
                }
                .keyboardShortcut(.cancelAction)
                Button("Save & Restart") {
                    if viewModel.save() {
                        NSApp.keyWindow?.close()
                    }
                }
                .keyboardShortcut(.defaultAction)
            }
            .padding(12)
        }
        .frame(minWidth: 520, minHeight: 380)
    }
}

private struct PreferencesTab: View {
    @ObservedObject var viewModel: SettingsViewModel

    var body: some View {
        Form {
            Toggle("Sound feedback (start/stop chimes)",
                   isOn: $viewModel.config.soundFeedback)
            Stepper(value: $viewModel.config.toggleMaxSeconds, in: 1...3600) {
                Text("Toggle max seconds: \(viewModel.config.toggleMaxSeconds)")
            }
            Picker("Log level", selection: $viewModel.config.logLevel) {
                Text("debug").tag("debug")
                Text("info").tag("info")
                Text("warn").tag("warn")
                Text("error").tag("error")
            }
            .pickerStyle(.menu)

            Text("Hotkeys, vocabulary, and replacements live in config.json — use \"Edit config.json in default editor…\" below to modify them.")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .padding(.top, 8)
        }
        .formStyle(.grouped)
    }
}

private struct EndpointsTab: View {
    @ObservedObject var viewModel: SettingsViewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Each row is one OpenAI-compatible endpoint. Reference it in hotkey models as \"<id>:<model>\".")
                .font(.footnote)
                .foregroundStyle(.secondary)

            if viewModel.config.drivers.isEmpty {
                Text("No drivers defined. Add one to begin.")
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollView {
                    VStack(spacing: 8) {
                        ForEach(viewModel.config.drivers[0].endpoints.indices, id: \.self) { i in
                            EndpointRow(
                                endpoint: $viewModel.config.drivers[0].endpoints[i],
                                onDelete: { viewModel.removeEndpoint(at: IndexSet(integer: i)) }
                            )
                        }
                    }
                }
            }

            HStack {
                Button {
                    viewModel.addEndpoint()
                } label: {
                    Label("Add endpoint", systemImage: "plus")
                }
                Spacer()
            }
        }
        .padding(.vertical, 4)
    }
}

private struct EndpointRow: View {
    @Binding var endpoint: Endpoint
    var onDelete: () -> Void

    var body: some View {
        GroupBox {
            VStack(alignment: .leading, spacing: 6) {
                HStack {
                    Text("id").frame(width: 80, alignment: .leading)
                    TextField("openai", text: $endpoint.id)
                        .textFieldStyle(.roundedBorder)
                    Button(role: .destructive) {
                        onDelete()
                    } label: {
                        Image(systemName: "trash")
                    }
                    .buttonStyle(.borderless)
                    .help("Remove endpoint")
                }
                HStack {
                    Text("api_base").frame(width: 80, alignment: .leading)
                    TextField("https://api.openai.com/v1", text: $endpoint.config.apiBase)
                        .textFieldStyle(.roundedBorder)
                }
                HStack {
                    Text("api_key").frame(width: 80, alignment: .leading)
                    SecureField("sk-…", text: $endpoint.config.apiKey)
                        .textFieldStyle(.roundedBorder)
                }
            }
            .padding(.vertical, 2)
        }
    }
}

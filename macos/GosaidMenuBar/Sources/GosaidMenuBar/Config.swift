import Foundation

// Mirrors internal/config/config.go::Config.  JSON round-trips via
// JSONEncoder/JSONDecoder with snake_case key coding; optional fields use
// `Optional` so encoding preserves the "omit when nil" behaviour that the
// Go side expects.
struct Config: Codable {
    var version: Int
    var drivers: [Driver]
    var vocabulary: [String: [String]]?
    var replacements: [String: [String: String]]?
    var hotkeys: [String: Hotkey]
    var toggleMaxSeconds: Int
    var injectionMode: String
    var soundFeedback: Bool
    var logLevel: String

    enum CodingKeys: String, CodingKey {
        case version
        case drivers
        case vocabulary
        case replacements
        case hotkeys
        case toggleMaxSeconds = "toggle_max_seconds"
        case injectionMode = "injection_mode"
        case soundFeedback = "sound_feedback"
        case logLevel = "log_level"
    }

    static func makeDefault() -> Config {
        Config(
            version: 1,
            drivers: [
                Driver(
                    driver: "openai_compatible",
                    endpoints: [
                        Endpoint(
                            id: "groq",
                            config: OpenAICompatibleConfig(
                                apiBase: "https://api.groq.com/openai/v1",
                                apiKey: ""
                            )
                        )
                    ]
                )
            ],
            vocabulary: nil,
            replacements: nil,
            hotkeys: [
                "ctrl+alt+space": Hotkey(
                    mode: "hold",
                    transcribe: TranscribeStage(model: "groq:whisper-large-v3"),
                    translate: nil,
                    enhance: nil
                )
            ],
            toggleMaxSeconds: 60,
            injectionMode: "paste",
            soundFeedback: true,
            logLevel: "info"
        )
    }
}

struct Driver: Codable {
    var driver: String
    var endpoints: [Endpoint]
}

struct Endpoint: Codable {
    var id: String
    var config: OpenAICompatibleConfig
}

struct OpenAICompatibleConfig: Codable {
    var apiBase: String
    var apiKey: String

    enum CodingKeys: String, CodingKey {
        case apiBase = "api_base"
        case apiKey = "api_key"
    }
}

struct Hotkey: Codable {
    var mode: String?
    var transcribe: TranscribeStage
    var translate: TranslateStage?
    var enhance: EnhanceStage?
}

struct TranscribeStage: Codable {
    var model: String
    var inputLanguage: String?
    var outputLanguage: String?

    enum CodingKeys: String, CodingKey {
        case model
        case inputLanguage = "input_language"
        case outputLanguage = "output_language"
    }
}

struct TranslateStage: Codable {
    var outputLanguage: String
    var model: String
    var prompt: String?

    enum CodingKeys: String, CodingKey {
        case outputLanguage = "output_language"
        case model
        case prompt
    }
}

struct EnhanceStage: Codable {
    var prompt: String?
    var model: String
}

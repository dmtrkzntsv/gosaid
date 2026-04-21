using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace GosaidUI;

// Mirrors internal/config/config.go::Config so we can read and write the
// same config.json file as the daemon. Property names follow JSON snake_case
// via JsonPropertyName attributes.
internal sealed class Config
{
    [JsonPropertyName("version")] public int Version { get; set; } = 1;
    [JsonPropertyName("drivers")] public List<Driver> Drivers { get; set; } = new();
    [JsonPropertyName("vocabulary")] public Dictionary<string, List<string>>? Vocabulary { get; set; }
    [JsonPropertyName("replacements")] public Dictionary<string, Dictionary<string, string>>? Replacements { get; set; }
    [JsonPropertyName("hotkeys")] public Dictionary<string, Hotkey> Hotkeys { get; set; } = new();
    [JsonPropertyName("toggle_max_seconds")] public int ToggleMaxSeconds { get; set; } = 60;
    [JsonPropertyName("injection_mode")] public string InjectionMode { get; set; } = "paste";
    [JsonPropertyName("sound_feedback")] public bool SoundFeedback { get; set; } = true;
    [JsonPropertyName("log_level")] public string LogLevel { get; set; } = "info";
}

internal sealed class Driver
{
    [JsonPropertyName("driver")] public string Name { get; set; } = "openai_compatible";
    [JsonPropertyName("endpoints")] public List<Endpoint> Endpoints { get; set; } = new();
}

internal sealed class Endpoint
{
    [JsonPropertyName("id")] public string Id { get; set; } = "";
    [JsonPropertyName("config")] public OpenAICompatibleConfig Config { get; set; } = new();
}

internal sealed class OpenAICompatibleConfig
{
    [JsonPropertyName("api_base")] public string ApiBase { get; set; } = "";
    [JsonPropertyName("api_key")] public string ApiKey { get; set; } = "";
}

internal sealed class Hotkey
{
    [JsonPropertyName("mode"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Mode { get; set; }

    [JsonPropertyName("transcribe")] public TranscribeStage Transcribe { get; set; } = new();

    [JsonPropertyName("translate"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public TranslateStage? Translate { get; set; }

    [JsonPropertyName("enhance"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public EnhanceStage? Enhance { get; set; }
}

internal sealed class TranscribeStage
{
    [JsonPropertyName("model")] public string Model { get; set; } = "";
    [JsonPropertyName("input_language"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? InputLanguage { get; set; }
    [JsonPropertyName("output_language"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? OutputLanguage { get; set; }
}

internal sealed class TranslateStage
{
    [JsonPropertyName("output_language")] public string OutputLanguage { get; set; } = "";
    [JsonPropertyName("model")] public string Model { get; set; } = "";
    [JsonPropertyName("prompt"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Prompt { get; set; }
}

internal sealed class EnhanceStage
{
    [JsonPropertyName("prompt"), JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
    public string? Prompt { get; set; }
    [JsonPropertyName("model")] public string Model { get; set; } = "";
}

// Source-generated JSON context — required for NativeAOT. Keeps reflection-
// free serialization and trims unused types.
[JsonSourceGenerationOptions(
    WriteIndented = true,
    DefaultIgnoreCondition = JsonIgnoreCondition.Never)]
[JsonSerializable(typeof(Config))]
internal sealed partial class ConfigJsonContext : JsonSerializerContext
{
}

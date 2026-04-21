using System;
using System.IO;
using System.Text.Json;

namespace GosaidUI;

internal static class ConfigStore
{
    public static Config Load()
    {
        var path = Paths.ConfigFile;
        if (!File.Exists(path))
        {
            var def = Defaults.NewConfig();
            Save(def);
            return def;
        }
        using var stream = File.OpenRead(path);
        var cfg = JsonSerializer.Deserialize(stream, ConfigJsonContext.Default.Config);
        return cfg ?? Defaults.NewConfig();
    }

    // Atomic write via temp + File.Replace, mirroring writeAtomic in
    // internal/config/store.go so the daemon never sees a half-written file.
    public static void Save(Config cfg)
    {
        var path = Paths.ConfigFile;
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);

        var tmp = Path.Combine(
            Path.GetDirectoryName(path)!,
            $"config-{Guid.NewGuid():N}.json.tmp");

        using (var stream = File.Create(tmp))
        {
            JsonSerializer.Serialize(stream, cfg, ConfigJsonContext.Default.Config);
        }

        if (File.Exists(path))
        {
            File.Replace(tmp, path, null);
        }
        else
        {
            File.Move(tmp, path);
        }
    }
}

internal static class Defaults
{
    public static Config NewConfig() => new()
    {
        Version = 1,
        Drivers =
        [
            new Driver
            {
                Name = "openai_compatible",
                Endpoints =
                [
                    new Endpoint
                    {
                        Id = "groq",
                        Config = new OpenAICompatibleConfig
                        {
                            ApiBase = "https://api.groq.com/openai/v1",
                            ApiKey = "",
                        },
                    },
                ],
            },
        ],
        Hotkeys = new()
        {
            ["ctrl+alt+space"] = new Hotkey
            {
                Mode = "hold",
                Transcribe = new TranscribeStage { Model = "groq:whisper-large-v3" },
            },
        },
        ToggleMaxSeconds = 60,
        InjectionMode = "paste",
        SoundFeedback = true,
        LogLevel = "info",
    };
}

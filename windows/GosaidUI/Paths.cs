using System;
using System.IO;

namespace GosaidUI;

// Mirrors internal/platform/paths.go so the UI reads/writes the same files
// the daemon does: %AppData%\gosaid\config.json and %LocalAppData%\gosaid\.
internal static class Paths
{
    private const string AppName = "gosaid";

    public static string ConfigDir =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), AppName);

    public static string ConfigFile => Path.Combine(ConfigDir, "config.json");

    public static string StateDir =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), AppName);

    public static string LogFile => Path.Combine(StateDir, "gosaid.log");

    public static string DaemonExe
    {
        get
        {
            var dir = AppContext.BaseDirectory;
            return Path.Combine(dir, "gosaid.exe");
        }
    }
}

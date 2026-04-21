using System;
using System.Diagnostics;
using System.IO;
using System.Threading.Tasks;

namespace GosaidUI;

// Manages the lifecycle of the gosaid.exe child process. Its stderr is
// appended to %LocalAppData%\gosaid\gosaid.log; stdout is swallowed.
//
// Shutdown is done with Process.Kill(). The daemon on Windows has no
// critical cleanup — the PID file is overwritten on next start and the OS
// reclaims audio handles. If we later need graceful shutdown, we'll spawn
// the daemon with CREATE_NEW_PROCESS_GROUP + GenerateConsoleCtrlEvent.
internal sealed class DaemonProcess : IDisposable
{
    public event EventHandler? Exited;

    private readonly object _lock = new();
    private Process? _proc;
    private StreamWriter? _logWriter;

    public bool IsRunning
    {
        get
        {
            lock (_lock)
            {
                return _proc is { HasExited: false };
            }
        }
    }

    public void Start()
    {
        lock (_lock)
        {
            if (_proc is { HasExited: false }) return;

            var exe = Paths.DaemonExe;
            if (!File.Exists(exe))
            {
                throw new FileNotFoundException(
                    $"gosaid.exe not found next to GosaidUI.exe (expected {exe})", exe);
            }

            Directory.CreateDirectory(Paths.StateDir);
            _logWriter = new StreamWriter(
                new FileStream(Paths.LogFile, FileMode.Append, FileAccess.Write, FileShare.Read))
            {
                AutoFlush = true,
            };
            _logWriter.WriteLine($"--- gosaid started {DateTime.Now:u} ---");

            var psi = new ProcessStartInfo
            {
                FileName = exe,
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                WorkingDirectory = Path.GetDirectoryName(exe) ?? Environment.CurrentDirectory,
            };

            var p = new Process { StartInfo = psi, EnableRaisingEvents = true };
            p.OutputDataReceived += (_, e) =>
            {
                if (e.Data is null) return;
                lock (_lock) { _logWriter?.WriteLine(e.Data); }
            };
            p.ErrorDataReceived += (_, e) =>
            {
                if (e.Data is null) return;
                lock (_lock) { _logWriter?.WriteLine(e.Data); }
            };
            p.Exited += (_, _) =>
            {
                lock (_lock) { _logWriter?.WriteLine($"--- gosaid exited code={p.ExitCode} ---"); }
                Exited?.Invoke(this, EventArgs.Empty);
            };

            p.Start();
            p.BeginOutputReadLine();
            p.BeginErrorReadLine();
            _proc = p;
        }
    }

    public void Stop()
    {
        Process? p;
        lock (_lock)
        {
            p = _proc;
            _proc = null;
        }
        if (p is null || p.HasExited) { DisposeLog(); return; }

        try { p.Kill(entireProcessTree: true); } catch { /* best-effort */ }
        p.WaitForExit(2_000);
        p.Dispose();
        DisposeLog();
    }

    public async Task RestartAsync()
    {
        Stop();
        await Task.Delay(200).ConfigureAwait(false);
        Start();
    }

    private void DisposeLog()
    {
        lock (_lock)
        {
            _logWriter?.Dispose();
            _logWriter = null;
        }
    }

    public void Dispose() => Stop();
}

using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Reflection;
using System.Windows.Forms;

namespace GosaidUI;

internal sealed class TrayApplicationContext : ApplicationContext
{
    private readonly NotifyIcon _tray;
    private readonly DaemonProcess _daemon;
    private readonly ToolStripMenuItem _statusItem;
    private SettingsForm? _settingsForm;

    public TrayApplicationContext()
    {
        _daemon = new DaemonProcess();
        _daemon.Exited += (_, _) =>
            BeginInvokeOnTray(() => SetStatus(_daemon.IsRunning ? "Running" : "Stopped"));

        _statusItem = new ToolStripMenuItem("Status: starting…") { Enabled = false };

        var menu = new ContextMenuStrip();
        menu.Items.Add(_statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Settings…", null, (_, _) => ShowSettings());
        menu.Items.Add("Restart daemon", null, async (_, _) =>
        {
            SetStatus("Restarting…");
            try
            {
                await _daemon.RestartAsync();
                SetStatus(_daemon.IsRunning ? "Running" : "Stopped");
            }
            catch (Exception ex)
            {
                SetStatus("Error");
                ShowError("Failed to restart gosaid.exe", ex);
            }
        });
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Open config file", null, (_, _) => OpenInShell(Paths.ConfigFile));
        menu.Items.Add("Open logs folder", null, (_, _) => OpenInShell(Paths.StateDir));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Quit", null, (_, _) => ExitThread());

        _tray = new NotifyIcon
        {
            Icon = LoadTrayIcon(),
            Text = "Gosaid",
            Visible = true,
            ContextMenuStrip = menu,
        };
        _tray.DoubleClick += (_, _) => ShowSettings();

        try
        {
            _daemon.Start();
            SetStatus("Running");
        }
        catch (Exception ex)
        {
            SetStatus("Not started");
            ShowError("Failed to start gosaid.exe", ex);
        }
    }

    private void SetStatus(string text)
    {
        _statusItem.Text = $"Status: {text}";
        _tray.Text = text == "Running" ? "Gosaid — running" : $"Gosaid — {text.ToLower()}";
    }

    private void ShowSettings()
    {
        if (_settingsForm is { IsDisposed: false })
        {
            _settingsForm.Activate();
            return;
        }
        _settingsForm = new SettingsForm();
        _settingsForm.Saved += async (_, _) =>
        {
            SetStatus("Restarting…");
            try
            {
                await _daemon.RestartAsync();
                SetStatus(_daemon.IsRunning ? "Running" : "Stopped");
            }
            catch (Exception ex)
            {
                SetStatus("Error");
                ShowError("Failed to restart gosaid.exe after save", ex);
            }
        };
        _settingsForm.FormClosed += (_, _) => _settingsForm = null;
        _settingsForm.Show();
        _settingsForm.Activate();
    }

    private void OpenInShell(string path)
    {
        try
        {
            if (!File.Exists(path) && !Directory.Exists(path))
            {
                // Create the parent directory so Explorer has something to open.
                var dir = Path.GetDirectoryName(path);
                if (dir is not null) Directory.CreateDirectory(dir);
            }
            Process.Start(new ProcessStartInfo { FileName = path, UseShellExecute = true });
        }
        catch (Exception ex)
        {
            ShowError($"Failed to open {path}", ex);
        }
    }

    private static void ShowError(string summary, Exception ex)
    {
        MessageBox.Show(
            $"{summary}\n\n{ex.Message}",
            "Gosaid",
            MessageBoxButtons.OK,
            MessageBoxIcon.Error);
    }

    private void BeginInvokeOnTray(Action action)
    {
        if (_tray.ContextMenuStrip is { IsHandleCreated: true } strip)
        {
            strip.BeginInvoke(action);
        }
        else
        {
            action();
        }
    }

    private static Icon LoadTrayIcon()
    {
        var asm = Assembly.GetExecutingAssembly();
        foreach (var name in asm.GetManifestResourceNames())
        {
            if (!name.EndsWith("tray.ico", StringComparison.OrdinalIgnoreCase)) continue;
            using var stream = asm.GetManifestResourceStream(name);
            if (stream is null) continue;
            return new Icon(stream);
        }
        return SystemIcons.Application;
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _tray.Visible = false;
            _tray.Dispose();
            _daemon.Dispose();
            _settingsForm?.Dispose();
        }
        base.Dispose(disposing);
    }

    protected override void ExitThreadCore()
    {
        _tray.Visible = false;
        _daemon.Stop();
        base.ExitThreadCore();
    }
}

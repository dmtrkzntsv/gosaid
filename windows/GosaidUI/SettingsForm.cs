using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Drawing;
using System.Windows.Forms;

namespace GosaidUI;

internal sealed class SettingsForm : Form
{
    public event EventHandler? Saved;

    private Config _config = null!;

    private CheckBox _soundFeedback = null!;
    private NumericUpDown _toggleMaxSeconds = null!;
    private ComboBox _logLevel = null!;
    private DataGridView _endpointsGrid = null!;
    private Button _saveBtn = null!;
    private Button _cancelBtn = null!;
    private Button _editAdvancedBtn = null!;

    public SettingsForm()
    {
        Text = "Gosaid — Settings";
        MinimumSize = new Size(640, 480);
        Size = new Size(720, 540);
        StartPosition = FormStartPosition.CenterScreen;
        Font = new Font("Segoe UI", 9f);
        FormBorderStyle = FormBorderStyle.Sizable;
        ShowIcon = false;
        ShowInTaskbar = true;

        BuildLayout();
        LoadFromDisk();
    }

    private void BuildLayout()
    {
        var root = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 1,
            RowCount = 2,
            Padding = new Padding(12),
        };
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        Controls.Add(root);

        var tabs = new TabControl { Dock = DockStyle.Fill };
        tabs.TabPages.Add(BuildPreferencesTab());
        tabs.TabPages.Add(BuildEndpointsTab());
        root.Controls.Add(tabs, 0, 0);

        var buttons = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill,
            FlowDirection = FlowDirection.RightToLeft,
            AutoSize = true,
            Margin = new Padding(0, 8, 0, 0),
        };
        _saveBtn = new Button
        {
            Text = "Save && restart",
            AutoSize = true,
            MinimumSize = new Size(110, 28),
        };
        _saveBtn.Click += OnSave;
        _cancelBtn = new Button
        {
            Text = "Cancel",
            AutoSize = true,
            MinimumSize = new Size(90, 28),
        };
        _cancelBtn.Click += (_, _) => Close();
        _editAdvancedBtn = new Button
        {
            Text = "Edit config.json in Notepad…",
            AutoSize = true,
            MinimumSize = new Size(210, 28),
        };
        _editAdvancedBtn.Click += (_, _) => OpenInNotepad();

        buttons.Controls.Add(_saveBtn);
        buttons.Controls.Add(_cancelBtn);
        buttons.Controls.Add(_editAdvancedBtn);
        root.Controls.Add(buttons, 0, 1);

        AcceptButton = _saveBtn;
        CancelButton = _cancelBtn;
    }

    private TabPage BuildPreferencesTab()
    {
        var page = new TabPage("Preferences") { Padding = new Padding(12), UseVisualStyleBackColor = true };
        var grid = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 2,
            RowCount = 4,
        };
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        for (int i = 0; i < 4; i++) grid.RowStyles.Add(new RowStyle(SizeType.AutoSize));

        grid.Controls.Add(new Label { Text = "Sound feedback", AutoSize = true, Margin = new Padding(0, 6, 12, 6) }, 0, 0);
        _soundFeedback = new CheckBox { Text = "Play start/stop chimes", AutoSize = true, Margin = new Padding(0, 4, 0, 4) };
        grid.Controls.Add(_soundFeedback, 1, 0);

        grid.Controls.Add(new Label { Text = "Toggle max seconds", AutoSize = true, Margin = new Padding(0, 6, 12, 6) }, 0, 1);
        _toggleMaxSeconds = new NumericUpDown { Minimum = 1, Maximum = 3600, Width = 120 };
        grid.Controls.Add(_toggleMaxSeconds, 1, 1);

        grid.Controls.Add(new Label { Text = "Log level", AutoSize = true, Margin = new Padding(0, 6, 12, 6) }, 0, 2);
        _logLevel = new ComboBox { DropDownStyle = ComboBoxStyle.DropDownList, Width = 140 };
        _logLevel.Items.AddRange(new object[] { "debug", "info", "warn", "error" });
        grid.Controls.Add(_logLevel, 1, 2);

        var note = new Label
        {
            Text = "Hotkeys, vocabulary, and replacements live in config.json — click \"Edit config.json in Notepad…\" below to modify them.",
            AutoSize = false,
            Dock = DockStyle.Fill,
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 24, 0, 0),
            MinimumSize = new Size(0, 48),
        };
        grid.Controls.Add(note, 1, 3);

        page.Controls.Add(grid);
        return page;
    }

    private TabPage BuildEndpointsTab()
    {
        var page = new TabPage("Endpoints") { Padding = new Padding(12), UseVisualStyleBackColor = true };

        _endpointsGrid = new DataGridView
        {
            Dock = DockStyle.Fill,
            AutoGenerateColumns = false,
            AllowUserToAddRows = true,
            AllowUserToDeleteRows = true,
            RowHeadersWidth = 30,
            SelectionMode = DataGridViewSelectionMode.FullRowSelect,
            AutoSizeColumnsMode = DataGridViewAutoSizeColumnsMode.Fill,
        };
        _endpointsGrid.Columns.Add(new DataGridViewTextBoxColumn
        {
            HeaderText = "ID",
            Name = "id",
            FillWeight = 15,
            MinimumWidth = 80,
        });
        _endpointsGrid.Columns.Add(new DataGridViewTextBoxColumn
        {
            HeaderText = "API Base URL",
            Name = "api_base",
            FillWeight = 45,
            MinimumWidth = 200,
        });
        _endpointsGrid.Columns.Add(new DataGridViewTextBoxColumn
        {
            HeaderText = "API Key",
            Name = "api_key",
            FillWeight = 40,
            MinimumWidth = 200,
        });

        var hint = new Label
        {
            Text = "Each row is one OpenAI-compatible endpoint. Reference it in hotkey models as \"<id>:<model>\".",
            Dock = DockStyle.Bottom,
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 6, 0, 0),
        };

        page.Controls.Add(_endpointsGrid);
        page.Controls.Add(hint);
        return page;
    }

    private void LoadFromDisk()
    {
        try
        {
            _config = ConfigStore.Load();
        }
        catch (Exception ex)
        {
            MessageBox.Show(
                $"Failed to read config.json:\n\n{ex.Message}\n\nStarting with defaults.",
                "Gosaid",
                MessageBoxButtons.OK,
                MessageBoxIcon.Warning);
            _config = Defaults.NewConfig();
        }

        _soundFeedback.Checked = _config.SoundFeedback;
        _toggleMaxSeconds.Value = Math.Max(1, _config.ToggleMaxSeconds);
        var idx = _logLevel.Items.IndexOf(_config.LogLevel ?? "info");
        _logLevel.SelectedIndex = idx >= 0 ? idx : _logLevel.Items.IndexOf("info");

        _endpointsGrid.Rows.Clear();
        foreach (var drv in _config.Drivers)
        {
            foreach (var ep in drv.Endpoints)
            {
                _endpointsGrid.Rows.Add(ep.Id, ep.Config.ApiBase, ep.Config.ApiKey);
            }
        }
    }

    private void OnSave(object? sender, EventArgs e)
    {
        try
        {
            ApplyFormToConfig();
            ConfigStore.Save(_config);
            Saved?.Invoke(this, EventArgs.Empty);
            Close();
        }
        catch (Exception ex)
        {
            MessageBox.Show(
                $"Failed to save:\n\n{ex.Message}",
                "Gosaid",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
        }
    }

    private void ApplyFormToConfig()
    {
        _config.SoundFeedback = _soundFeedback.Checked;
        _config.ToggleMaxSeconds = (int)_toggleMaxSeconds.Value;
        _config.LogLevel = _logLevel.SelectedItem as string ?? "info";

        var endpoints = new List<Endpoint>();
        foreach (DataGridViewRow row in _endpointsGrid.Rows)
        {
            if (row.IsNewRow) continue;
            var id = (row.Cells["id"].Value as string ?? "").Trim();
            var apiBase = (row.Cells["api_base"].Value as string ?? "").Trim();
            var apiKey = (row.Cells["api_key"].Value as string ?? "").Trim();
            if (id.Length == 0 && apiBase.Length == 0 && apiKey.Length == 0) continue;
            if (id.Length == 0) throw new InvalidOperationException("Endpoint id is required on every row.");
            if (apiBase.Length == 0) throw new InvalidOperationException($"Endpoint \"{id}\": api_base is required.");
            endpoints.Add(new Endpoint
            {
                Id = id,
                Config = new OpenAICompatibleConfig { ApiBase = apiBase, ApiKey = apiKey },
            });
        }
        if (endpoints.Count == 0)
        {
            throw new InvalidOperationException("At least one endpoint is required.");
        }

        if (_config.Drivers.Count == 0)
        {
            _config.Drivers.Add(new Driver { Name = "openai_compatible" });
        }
        _config.Drivers[0].Name = "openai_compatible";
        _config.Drivers[0].Endpoints = endpoints;
        // Drop any additional driver blocks the user may have added by hand — we
        // only manage the first. They can use the JSON editor for multi-driver setups.
        while (_config.Drivers.Count > 1) _config.Drivers.RemoveAt(1);
    }

    private void OpenInNotepad()
    {
        try
        {
            // Ensure the file exists first (creates defaults if missing).
            _ = ConfigStore.Load();
            Process.Start(new ProcessStartInfo
            {
                FileName = "notepad.exe",
                Arguments = $"\"{Paths.ConfigFile}\"",
                UseShellExecute = true,
            });
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to open Notepad: {ex.Message}", "Gosaid", MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        base.OnFormClosing(e);
    }
}

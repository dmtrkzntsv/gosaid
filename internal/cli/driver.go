package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"golang.org/x/term"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

const driverUsage = `usage: gosaid driver

Opens an interactive manager that lists configured drivers, lets you
configure hosted drivers, and adds OpenAI, OpenRouter, or another
OpenAI-compatible API.`

type hostedDriverPreset struct {
	Key     string
	Label   string
	ID      string
	APIBase string
}

var hostedDriverPresets = []hostedDriverPreset{
	{
		Key:     "openai",
		Label:   "OpenAI",
		ID:      "openai",
		APIBase: "https://api.openai.com/v1",
	},
	{
		Key:     "openrouter",
		Label:   "OpenRouter",
		ID:      "openrouter",
		APIBase: "https://openrouter.ai/api/v1",
	},
	{
		Key:   "openai-compat",
		Label: "OpenAI-compatible",
	},
}

// RunDriver opens the single interactive `gosaid driver` manager.
func RunDriver(args []string) int {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
			fmt.Println(driverUsage)
			return 0
		}
		fmt.Fprintf(os.Stderr, "gosaid driver takes no arguments\n%s\n", driverUsage)
		return 2
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, "error: gosaid driver requires an interactive terminal")
		return 1
	}

	path, err := config.Path()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if err := manageDrivers(path, cfg); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func findHostedDriverPreset(name string) (hostedDriverPreset, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "openai-compatible", "openai_compatible":
		name = "openai-compat"
	}
	for _, p := range hostedDriverPresets {
		if p.Key == name {
			return p, true
		}
	}
	return hostedDriverPreset{}, false
}

func addHostedEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	id = strings.TrimSpace(id)
	apiBase = strings.TrimSpace(apiBase)
	apiKey = strings.TrimSpace(apiKey)
	if err := validateHostedEndpoint(cfg, id, apiBase, apiKey, ""); err != nil {
		return err
	}

	for i := range cfg.Drivers {
		if cfg.Drivers[i].Driver == config.DriverOpenAICompatible {
			cfg.Drivers[i].Endpoints = append(cfg.Drivers[i].Endpoints, config.Endpoint{
				ID: id,
				Config: config.EndpointConfig{
					APIBase: apiBase,
					APIKey:  apiKey,
				},
			})
			return validateDriverTopology(cfg)
		}
	}
	cfg.Drivers = append(cfg.Drivers, config.Driver{
		Driver: config.DriverOpenAICompatible,
		Endpoints: []config.Endpoint{{
			ID: id,
			Config: config.EndpointConfig{
				APIBase: apiBase,
				APIKey:  apiKey,
			},
		}},
	})
	return validateDriverTopology(cfg)
}

func configureHostedEndpoint(cfg *config.Config, id, apiBase, apiKey string) error {
	endpoint, driverType := findEndpoint(cfg, id)
	if endpoint == nil {
		return fmt.Errorf("driver %q is not configured", id)
	}
	if driverType != config.DriverOpenAICompatible {
		return fmt.Errorf("driver %q is not OpenAI-compatible", id)
	}
	apiBase = strings.TrimSpace(apiBase)
	apiKey = strings.TrimSpace(apiKey)
	if err := validateHostedEndpoint(cfg, id, apiBase, apiKey, id); err != nil {
		return err
	}
	endpoint.Config.APIBase = apiBase
	endpoint.Config.APIKey = apiKey
	return validateDriverTopology(cfg)
}

func validateHostedEndpoint(cfg *config.Config, id, apiBase, apiKey, ownID string) error {
	if id == "" {
		return errors.New("endpoint id is required")
	}
	if strings.Contains(id, ":") {
		return errors.New("endpoint id must not contain ':' (model references use endpoint:model)")
	}
	if endpoint, _ := findEndpoint(cfg, id); endpoint != nil && id != ownID {
		return fmt.Errorf("driver %q already exists", id)
	}
	if apiBase == "" {
		return errors.New("api base URL is required")
	}
	if apiKey == "" {
		return errors.New("api key is required")
	}
	return nil
}

// validateDriverTopology checks identities and grouping without requiring
// unrelated endpoints, hotkeys, or local model files to be runnable. The
// endpoint being added or configured is validated separately. This lets a
// new user configure providers incrementally from a placeholder config.
func validateDriverTopology(cfg *config.Config) error {
	ids := map[string]struct{}{}
	for di, d := range cfg.Drivers {
		switch d.Driver {
		case config.DriverOpenAICompatible, config.DriverWhisperCPP, config.DriverLlamaCPP:
		default:
			return fmt.Errorf("drivers[%d]: unsupported driver type %q", di, d.Driver)
		}
		if len(d.Endpoints) == 0 {
			return fmt.Errorf("drivers[%d]: at least one endpoint is required", di)
		}
		for _, e := range d.Endpoints {
			if e.ID == "" {
				return fmt.Errorf("drivers[%d]: endpoint id is required", di)
			}
			if strings.Contains(e.ID, ":") {
				return fmt.Errorf("endpoint id %q must not contain ':'", e.ID)
			}
			if _, exists := ids[e.ID]; exists {
				return fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			ids[e.ID] = struct{}{}
		}
	}
	return nil
}

func findEndpoint(cfg *config.Config, id string) (*config.Endpoint, string) {
	for di := range cfg.Drivers {
		for ei := range cfg.Drivers[di].Endpoints {
			e := &cfg.Drivers[di].Endpoints[ei]
			if e.ID == id {
				return e, cfg.Drivers[di].Driver
			}
		}
	}
	return nil, ""
}

type listedDriver struct {
	ID, Provider, Driver, Configuration string
}

func configuredDrivers(cfg *config.Config) []listedDriver {
	var rows []listedDriver
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			row := listedDriver{ID: e.ID, Driver: d.Driver}
			switch d.Driver {
			case config.DriverOpenAICompatible:
				row.Provider = hostedProviderLabel(e.Config.APIBase)
				keyState := "API key configured"
				if e.Config.APIKey == "" || e.Config.APIKey == "REPLACE_ME" {
					keyState = "API key missing"
				}
				row.Configuration = fmt.Sprintf("%s (%s)", e.Config.APIBase, keyState)
			case config.DriverWhisperCPP:
				row.Provider = "Local Whisper"
				row.Configuration = modelCountLabel(len(e.Config.Models))
			case config.DriverLlamaCPP:
				row.Provider = "Local Llama"
				row.Configuration = modelCountLabel(len(e.Config.Models))
			default:
				row.Provider = "Unknown"
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func hostedProviderLabel(apiBase string) string {
	normalized := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	switch normalized {
	case "https://api.openai.com/v1":
		return "OpenAI"
	case "https://openrouter.ai/api/v1":
		return "OpenRouter"
	default:
		return "OpenAI-compatible"
	}
}

func modelCountLabel(n int) string {
	if n == 1 {
		return "1 model"
	}
	return fmt.Sprintf("%d models", n)
}

func saveDriverConfig(path string, cfg *config.Config) error {
	if err := validateDriverTopology(cfg); err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: driver settings saved, but the full config is not yet runnable: %v\n", err)
	}
	return nil
}

const (
	driverPickAdd  = "\x00add"
	driverPickBack = "\x00back"
	driverPickDone = "\x00done"
)

func manageDrivers(path string, cfg *config.Config) error {
	for {
		var options []huh.Option[string]
		for _, row := range configuredDrivers(cfg) {
			options = append(options, huh.NewOption(
				fmt.Sprintf("%s · %s · %s", row.ID, row.Provider, row.Configuration),
				row.ID,
			))
		}
		options = append(options,
			huh.NewOption("+ Add a new driver", driverPickAdd),
			huh.NewOption("Done", driverPickDone),
		)
		choice := ""
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Configured drivers").
				Description("Choose a hosted driver to configure, or add a new driver.").
				Options(options...).
				Value(&choice),
		)).Run(); err != nil {
			return err
		}
		switch choice {
		case driverPickDone:
			return nil
		case driverPickAdd:
			if err := interactiveAddDriver(cfg); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}
			if err := saveDriverConfig(path, cfg); err != nil {
				return err
			}
			fmt.Println("Driver added.")
		default:
			endpoint, driverType := findEndpoint(cfg, choice)
			if endpoint == nil {
				continue
			}
			action, err := interactiveDriverAction(choice, driverType)
			if err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}
			switch action {
			case "configure":
				if driverType != config.DriverOpenAICompatible {
					if err := showLocalDriver(choice, driverType, len(endpoint.Config.Models)); err != nil &&
						!errors.Is(err, huh.ErrUserAborted) {
						return err
					}
					continue
				}
				if err := interactiveConfigureDriver(cfg, choice); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						continue
					}
					return err
				}
				if err := saveDriverConfig(path, cfg); err != nil {
					return err
				}
				fmt.Println("Driver configured.")
			case "delete":
				deleted, err := interactiveDeleteDriver(cfg, choice, driverType)
				if err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						continue
					}
					return err
				}
				if !deleted {
					continue
				}
				if err := saveDriverConfig(path, cfg); err != nil {
					return err
				}
				fmt.Println("Driver deleted.")
			}
		}
	}
}

func interactiveDriverAction(id, driverType string) (string, error) {
	configureLabel := "Configure"
	if driverType != config.DriverOpenAICompatible {
		configureLabel = "Details"
	}
	action := ""
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(id).
			Options(
				huh.NewOption(configureLabel, "configure"),
				huh.NewOption("Delete", "delete"),
				huh.NewOption("Back", "back"),
			).
			Value(&action),
	)).Run()
	return action, err
}

func interactiveDeleteDriver(cfg *config.Config, id, driverType string) (bool, error) {
	description := "This removes the driver from config.json."
	if driverType == config.DriverWhisperCPP || driverType == config.DriverLlamaCPP {
		description += " Downloaded model files will stay on disk."
	}
	if refs := hotkeysUsingEndpoint(cfg, id); len(refs) > 0 {
		description = fmt.Sprintf(
			"Used by hotkeys: %s. Deleting it will leave those hotkeys invalid.",
			strings.Join(refs, ", "),
		)
	}
	choice := "cancel"
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(fmt.Sprintf("Delete driver %q?", id)).
			Description(description).
			Options(
				huh.NewOption("Delete", "delete"),
				huh.NewOption("Cancel", "cancel"),
			).
			Value(&choice),
	)).Run(); err != nil {
		return false, err
	}
	if choice != "delete" {
		return false, nil
	}
	if err := deleteEndpoint(cfg, id); err != nil {
		return false, err
	}
	return true, nil
}

func deleteEndpoint(cfg *config.Config, id string) error {
	for di := range cfg.Drivers {
		driver := &cfg.Drivers[di]
		for ei := range driver.Endpoints {
			if driver.Endpoints[ei].ID != id {
				continue
			}
			driver.Endpoints = append(driver.Endpoints[:ei], driver.Endpoints[ei+1:]...)
			if len(driver.Endpoints) == 0 {
				cfg.Drivers = append(cfg.Drivers[:di], cfg.Drivers[di+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("driver %q is not configured", id)
}

func hotkeysUsingEndpoint(cfg *config.Config, id string) []string {
	var combos []string
	for combo, hotkey := range cfg.Hotkeys {
		refs := []string{hotkey.Transcribe.Model}
		if hotkey.Translate != nil {
			refs = append(refs, hotkey.Translate.Model)
		}
		if hotkey.Enhance != nil {
			refs = append(refs, hotkey.Enhance.Model)
		}
		if hotkey.Compose != nil {
			refs = append(refs, hotkey.Compose.Model)
		}
		for _, ref := range refs {
			if modelRefUsesEndpoint(ref, id) {
				combos = append(combos, combo)
				break
			}
		}
	}
	sort.Strings(combos)
	return combos
}

func modelRefUsesEndpoint(ref, id string) bool {
	colon := strings.IndexByte(ref, ':')
	return colon > 0 && ref[:colon] == id
}

func interactiveAddDriver(cfg *config.Config) error {
	presetKey := hostedDriverPresets[0].Key
	var options []huh.Option[string]
	for _, p := range hostedDriverPresets {
		options = append(options, huh.NewOption(p.Label, p.Key))
	}
	options = append(options, huh.NewOption("Back", driverPickBack))
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Add a new driver").
			Options(options...).
			Value(&presetKey),
	)).Run(); err != nil {
		return err
	}
	if presetKey == driverPickBack {
		return huh.ErrUserAborted
	}
	preset, _ := findHostedDriverPreset(presetKey)
	id, apiBase, apiKey := preset.ID, preset.APIBase, ""
	submitted := false
	backChoice := "back"
	if preset.Key != "openai-compat" {
		validate := func(string) error {
			return validateHostedEndpoint(cfg, id, apiBase, strings.TrimSpace(apiKey), "")
		}
		apiKeyInput := &submitDriverInput{
			Input: huh.NewInput().
				Title(preset.Label + " API key").
				EchoMode(huh.EchoModePassword).
				Value(&apiKey).
				Validate(validate),
			submitted: &submitted,
			validate:  validate,
		}
		form := huh.NewForm(huh.NewGroup(
			apiKeyInput,
			newDriverBackSelect(&backChoice),
		)).
			WithKeyMap(driverFormKeyMap()).
			WithTheme(huh.ThemeFunc(driverCredentialTheme))
		if err := form.Run(); err != nil {
			return err
		}
		if !submitted {
			return huh.ErrUserAborted
		}
		return addHostedEndpoint(cfg, id, apiBase, apiKey)
	}

	validate := func(string) error {
		return validateHostedEndpoint(
			cfg,
			strings.TrimSpace(id),
			strings.TrimSpace(apiBase),
			strings.TrimSpace(apiKey),
			"",
		)
	}
	apiKeyInput := &submitDriverInput{
		Input: huh.NewInput().
			Title("API key").
			EchoMode(huh.EchoModePassword).
			Value(&apiKey).
			Validate(validate),
		submitted: &submitted,
		validate:  validate,
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Endpoint id").
			Description(`Short name used in model references, such as "openai:gpt-5.4-nano".`).
			Value(&id),
		huh.NewInput().
			Title("API base URL").
			Placeholder("https://api.example.com/v1").
			Value(&apiBase),
		apiKeyInput,
		newDriverBackSelect(&backChoice),
	)).
		WithKeyMap(driverFormKeyMap()).
		WithTheme(huh.ThemeFunc(driverCredentialTheme))
	if err := form.Run(); err != nil {
		return err
	}
	if !submitted {
		return huh.ErrUserAborted
	}
	return addHostedEndpoint(cfg, id, apiBase, apiKey)
}

// submitDriverInput distinguishes Enter from Down on the final credential
// field. Enter validates and submits; Down moves to the visible Back option
// without running required-field validation.
type submitDriverInput struct {
	*huh.Input
	submitted            *bool
	validate             func(string) error
	skipValidationOnBlur bool
}

func (i *submitDriverInput) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Key().Code {
		case tea.KeyDown:
			*i.submitted = false
			i.skipValidationOnBlur = true
			return i, huh.NextField
		case tea.KeyEnter:
			model, _ := i.Input.Update(msg)
			if updated, ok := model.(*huh.Input); ok {
				i.Input = updated
			}
			if i.Input.Error() != nil {
				*i.submitted = false
				return i, nil
			}
			*i.submitted = true
			return i, tea.Quit
		}
	}
	model, cmd := i.Input.Update(msg)
	if updated, ok := model.(*huh.Input); ok {
		i.Input = updated
	}
	if i.Input.Error() != nil {
		*i.submitted = false
	}
	return i, cmd
}

func (i *submitDriverInput) Blur() tea.Cmd {
	if !i.skipValidationOnBlur {
		return i.Input.Blur()
	}
	i.Input.Validate(func(string) error { return nil })
	cmd := i.Input.Blur()
	if i.validate != nil {
		i.Input.Validate(i.validate)
	}
	i.skipValidationOnBlur = false
	return cmd
}

func newDriverBackSelect(choice *string) *huh.Select[string] {
	return huh.NewSelect[string]().
		Options(huh.NewOption("Back", "back")).
		Value(choice)
}

func driverCredentialTheme(isDark bool) *huh.Styles {
	styles := huh.ThemeCharm(isDark)
	styles.Blurred.SelectedOption = styles.Blurred.UnselectedOption
	return styles
}

func driverFormKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Input.Next.SetKeys("enter", "tab", "down")
	keymap.Input.Prev.SetKeys("shift+tab", "up")
	keymap.Select.Up.SetKeys("k", "ctrl+k", "ctrl+p")
	keymap.Select.Prev.SetKeys("shift+tab", "up")
	return keymap
}

func interactiveConfigureDriver(cfg *config.Config, id string) error {
	endpoint, _ := findEndpoint(cfg, id)
	apiBase, apiKey := endpoint.Config.APIBase, endpoint.Config.APIKey
	if isPredefinedHostedAPIBase(apiBase) {
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title(hostedProviderLabel(apiBase)+" API key").
				EchoMode(huh.EchoModePassword).
				Value(&apiKey).
				Validate(requireDriverValue("api key")),
			huh.NewInput().
				Title("API base URL").
				Value(&apiBase).
				Validate(requireDriverValue("api base URL")),
		)).Run(); err != nil {
			return err
		}
		return configureHostedEndpoint(cfg, id, apiBase, apiKey)
	}

	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("API base URL").
			Value(&apiBase).
			Validate(requireDriverValue("api base URL")),
		huh.NewInput().
			Title("API key").
			EchoMode(huh.EchoModePassword).
			Value(&apiKey).
			Validate(requireDriverValue("api key")),
	)).Run(); err != nil {
		return err
	}
	return configureHostedEndpoint(cfg, id, apiBase, apiKey)
}

func isPredefinedHostedAPIBase(apiBase string) bool {
	return hostedProviderLabel(apiBase) != "OpenAI-compatible"
}

func showLocalDriver(id, driverType string, modelCount int) error {
	choice := ""
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(id + " · " + driverType).
			Description(fmt.Sprintf("%s. Manage local models with gosaid setup or gosaid model download.", modelCountLabel(modelCount))).
			Options(huh.NewOption("Back", "back")).
			Value(&choice),
	)).Run()
}

func requireDriverValue(name string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
}

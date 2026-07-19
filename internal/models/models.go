// Package models handles downloading and registering local models — Whisper
// transcription models under the whisper_cpp driver, and GGUF chat models
// under llama_cpp — so both the CLI and the setup wizard can share the same
// logic without an import cycle.
package models

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// Default endpoint ids per driver, used when the caller doesn't name one.
const (
	DefaultWhisperEndpoint = "local"
	DefaultLlamaEndpoint   = "local-llm"
)

// DownloadOpts configures Download. Driver defaults to whisper_cpp when
// empty, matching the original transcription-only behavior.
type DownloadOpts struct {
	Repo, File, Name, EndpointID string
	CfgPath, ModelsDir, BaseURL  string
	Driver                       string
	Force                        bool
}

// DownloadDefaults maps a model file name to its driver and default endpoint
// id: .gguf files are chat models for llama_cpp, everything else is a Whisper
// GGML model for whisper_cpp.
func DownloadDefaults(file string) (driver, endpointID string) {
	if strings.EqualFold(filepath.Ext(file), ".gguf") {
		return config.DriverLlamaCPP, DefaultLlamaEndpoint
	}
	return config.DriverWhisperCPP, DefaultWhisperEndpoint
}

// Download performs the full model-download operation: load config, run
// collision/duplicate checks, fetch the file, register it, and save config.
func Download(o DownloadOpts) error {
	if o.Driver == "" {
		o.Driver = config.DriverWhisperCPP
	}
	cfg, err := config.Load(o.CfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if driver := otherDriverForEndpoint(cfg, o.EndpointID, o.Driver); driver != "" {
		return fmt.Errorf("endpoint %q already exists with driver %q; choose a different --endpoint id",
			o.EndpointID, driver)
	}
	if existing := findModel(cfg, o.Driver, o.EndpointID, o.Name); existing != "" && !o.Force {
		return fmt.Errorf("model %q is already registered on endpoint %q → %s (use --force to overwrite)",
			o.Name, o.EndpointID, existing)
	}
	if err := os.MkdirAll(o.ModelsDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(o.ModelsDir, filepath.Base(o.File))
	if _, err := os.Stat(dest); err == nil && !o.Force {
		return fmt.Errorf("file already exists: %s (use --force to overwrite)", dest)
	}
	url := fmt.Sprintf("%s/%s/resolve/main/%s", o.BaseURL, o.Repo, o.File)
	size, err := fetchToFile(url, dest)
	if err != nil {
		return err
	}
	RegisterFor(cfg, o.Driver, o.EndpointID, o.Name, dest)
	if err := config.Save(o.CfgPath, cfg); err != nil {
		return fmt.Errorf("update config: %w", err)
	}
	stage := "transcribe"
	if o.Driver == config.DriverLlamaCPP {
		stage = "enhance"
	}
	fmt.Printf("downloaded %s (%.1f MB)\nregistered model %q on endpoint %q\n\nuse it in a hotkey:\n  \"%s\": { \"model\": \"%s:%s\" }\n",
		dest, float64(size)/(1<<20), o.Name, o.EndpointID, stage, o.EndpointID, o.Name)
	return nil
}

// FetchModelFile downloads repo/file from baseURL into modelsDir, returning
// the destination path. When the file already exists and force is false it
// returns the existing path with size 0 and no download.
func FetchModelFile(baseURL, repo, file, modelsDir string, force bool) (string, int64, error) {
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return "", 0, err
	}
	dest := filepath.Join(modelsDir, filepath.Base(file))
	if _, err := os.Stat(dest); err == nil && !force {
		return dest, 0, nil
	}
	url := fmt.Sprintf("%s/%s/resolve/main/%s", baseURL, repo, file)
	size, err := fetchToFile(url, dest)
	if err != nil {
		return "", 0, err
	}
	return dest, size, nil
}

// quantSuffixRe matches a trailing GGUF quantization label such as
// -Q4_K_M, -q4_0, -IQ2_XS, -F16, or -BF16.
var quantSuffixRe = regexp.MustCompile(`(?i)-(i?q[0-9][a-z0-9_]*|f16|f32|bf16)$`)

// DeriveName turns a model file name into a short registered name.
// Whisper GGML files drop the "ggml-" prefix ("ggml-base.bin" → "base");
// GGUF files drop a quantization suffix ("gemma-3-4b-it-Q4_K_M.gguf" →
// "gemma-3-4b-it").
func DeriveName(file string) string {
	base := filepath.Base(file)
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)
	if strings.EqualFold(ext, ".gguf") {
		return quantSuffixRe.ReplaceAllString(base, "")
	}
	return strings.TrimPrefix(base, "ggml-")
}

// RegisteredModels returns the models map for a whisper_cpp endpoint, or nil.
func RegisteredModels(cfg *config.Config, endpointID string) map[string]string {
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverWhisperCPP {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == endpointID {
				return e.Config.Models
			}
		}
	}
	return nil
}

// Register adds models[name]=path to a whisper_cpp endpoint, creating the
// driver block and/or endpoint if missing.
func Register(cfg *config.Config, endpointID, name, path string) {
	RegisterFor(cfg, config.DriverWhisperCPP, endpointID, name, path)
}

// RegisterFor adds models[name]=path to the endpoint under the given driver,
// creating the driver block and/or endpoint if missing.
func RegisterFor(cfg *config.Config, driver, endpointID, name, path string) {
	for di := range cfg.Drivers {
		d := &cfg.Drivers[di]
		if d.Driver != driver {
			continue
		}
		for ei := range d.Endpoints {
			e := &d.Endpoints[ei]
			if e.ID != endpointID {
				continue
			}
			if e.Config.Models == nil {
				e.Config.Models = map[string]string{}
			}
			e.Config.Models[name] = path
			return
		}
		d.Endpoints = append(d.Endpoints, config.Endpoint{
			ID:     endpointID,
			Config: config.EndpointConfig{Models: map[string]string{name: path}},
		})
		return
	}
	cfg.Drivers = append(cfg.Drivers, config.Driver{
		Driver: driver,
		Endpoints: []config.Endpoint{{
			ID:     endpointID,
			Config: config.EndpointConfig{Models: map[string]string{name: path}},
		}},
	})
}

// Unregister removes a model from a whisper_cpp endpoint, returning the
// removed file path. Endpoints left with no models are removed (an empty
// models map fails validation), as are driver blocks left with no endpoints.
func Unregister(cfg *config.Config, endpointID, name string) (string, bool) {
	for di := range cfg.Drivers {
		d := &cfg.Drivers[di]
		if d.Driver != config.DriverWhisperCPP {
			continue
		}
		for ei := range d.Endpoints {
			e := &d.Endpoints[ei]
			if e.ID != endpointID {
				continue
			}
			path, ok := e.Config.Models[name]
			if !ok {
				return "", false
			}
			delete(e.Config.Models, name)
			if len(e.Config.Models) == 0 {
				d.Endpoints = append(d.Endpoints[:ei], d.Endpoints[ei+1:]...)
			}
			if len(d.Endpoints) == 0 {
				cfg.Drivers = append(cfg.Drivers[:di], cfg.Drivers[di+1:]...)
			}
			return path, true
		}
	}
	return "", false
}

// findModel returns the registered path for name on the endpoint under the
// given driver, or "".
func findModel(cfg *config.Config, driver, endpointID, name string) string {
	for _, d := range cfg.Drivers {
		if d.Driver != driver {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == endpointID {
				return e.Config.Models[name]
			}
		}
	}
	return ""
}

// otherDriverForEndpoint scans all drivers for an endpoint with the given id
// belonging to a driver other than ownDriver, returning that driver's type
// (or "" if the id is unused or already owned by ownDriver).
func otherDriverForEndpoint(cfg *config.Config, endpointID, ownDriver string) string {
	for _, d := range cfg.Drivers {
		if d.Driver == ownDriver {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == endpointID {
				return d.Driver
			}
		}
	}
	return ""
}

// fetchToFile streams url to dest via a .part temp file with a progress
// line on stderr, renaming atomically on success. Returns bytes written.
func fetchToFile(url, dest string) (int64, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: server returned %s", url, resp.Status)
	}

	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return 0, err
	}
	defer os.Remove(part) // no-op after successful rename

	n, err := io.Copy(f, &progressReader{r: resp.Body, total: resp.ContentLength})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	fmt.Fprintln(os.Stderr) // finish the progress line
	return n, os.Rename(part, dest)
}

// progressReader prints download progress to stderr as percent (or MB when
// the server sends no Content-Length).
type progressReader struct {
	r          io.Reader
	total      int64
	done       int64
	lastNotice int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	// Update roughly every 5 MB to keep output quiet.
	if p.done-p.lastNotice >= 5<<20 || (err == io.EOF && p.done > 0) {
		p.lastNotice = p.done
		if p.total > 0 {
			fmt.Fprintf(os.Stderr, "\rdownloading... %d%%", p.done*100/p.total)
		} else {
			fmt.Fprintf(os.Stderr, "\rdownloading... %.1f MB", float64(p.done)/(1<<20))
		}
	}
	return n, err
}

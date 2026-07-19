package cli

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

const modelUsage = "usage: gosaid model download <hf-repo> <file> [--name <name>] [--endpoint <id>] [--force]"

// RunModel handles `gosaid model ...` subcommands.
func RunModel(args []string) int {
	if len(args) == 0 || args[0] != "download" {
		fmt.Fprintln(os.Stderr, modelUsage)
		return 2
	}
	fs := flag.NewFlagSet("model download", flag.ContinueOnError)
	name := fs.String("name", "", "model name to register (default: derived from file name)")
	endpoint := fs.String("endpoint", "", "endpoint id to register under (default: local for whisper models, local-llm for .gguf chat models)")
	force := fs.Bool("force", false, "overwrite an existing file and config entry")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	rest := fs.Args()
	// Allow flags after the positionals too (flag stops at the first non-flag).
	if len(rest) > 2 {
		if err := fs.Parse(rest[2:]); err != nil {
			return 2
		}
		rest = rest[:2]
	}
	if len(rest) != 2 || rest[0] == "" || rest[1] == "" || strings.HasPrefix(rest[0], "-") || strings.HasPrefix(rest[1], "-") {
		fmt.Fprintln(os.Stderr, modelUsage)
		return 2
	}
	if *name == "" {
		*name = deriveModelName(rest[1])
	}
	driver, defaultEndpoint := downloadDefaults(rest[1])
	if *endpoint == "" {
		*endpoint = defaultEndpoint
	}

	cfgPath, err := config.Path()
	if err == nil {
		var modelsDir string
		modelsDir, err = platform.ModelsDir()
		if err == nil {
			err = modelDownload(modelDownloadOpts{
				repo: rest[0], file: rest[1], name: *name, endpointID: *endpoint,
				cfgPath: cfgPath, modelsDir: modelsDir,
				baseURL: "https://huggingface.co", force: *force,
				driver: driver,
			})
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// quantSuffixRe matches a trailing GGUF quantization label such as
// -Q4_K_M, -q4_0, -IQ2_XS, -F16, or -BF16.
var quantSuffixRe = regexp.MustCompile(`(?i)-(i?q[0-9][a-z0-9_]*|f16|f32|bf16)$`)

// deriveModelName turns a model file name into a short registered name.
// Whisper GGML files drop the "ggml-" prefix ("ggml-base.bin" → "base");
// GGUF files drop a quantization suffix ("gemma-3-4b-it-Q4_K_M.gguf" →
// "gemma-3-4b-it").
func deriveModelName(file string) string {
	base := filepath.Base(file)
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)
	if strings.EqualFold(ext, ".gguf") {
		return quantSuffixRe.ReplaceAllString(base, "")
	}
	return strings.TrimPrefix(base, "ggml-")
}

// downloadDefaults maps a model file name to its driver type and default
// endpoint id: .gguf files are chat models for llama_cpp, everything else
// is a whisper GGML model.
func downloadDefaults(file string) (driver, endpointID string) {
	if strings.EqualFold(filepath.Ext(file), ".gguf") {
		return config.DriverLlamaCPP, "local-llm"
	}
	return config.DriverWhisperCPP, "local"
}

type modelDownloadOpts struct {
	repo, file, name, endpointID string
	cfgPath, modelsDir, baseURL  string
	driver                       string
	force                        bool
}

func modelDownload(o modelDownloadOpts) error {
	cfg, err := config.Load(o.cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if driver := otherDriverForEndpoint(cfg, o.endpointID, o.driver); driver != "" {
		return fmt.Errorf("endpoint %q already exists with driver %q; choose a different --endpoint id",
			o.endpointID, driver)
	}
	if existing := findLocalModel(cfg, o.driver, o.endpointID, o.name); existing != "" && !o.force {
		return fmt.Errorf("model %q is already registered on endpoint %q → %s (use --force to overwrite)",
			o.name, o.endpointID, existing)
	}

	if err := os.MkdirAll(o.modelsDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(o.modelsDir, filepath.Base(o.file))
	if _, err := os.Stat(dest); err == nil && !o.force {
		return fmt.Errorf("file already exists: %s (use --force to overwrite)", dest)
	}

	url := fmt.Sprintf("%s/%s/resolve/main/%s", o.baseURL, o.repo, o.file)
	size, err := fetchToFile(url, dest)
	if err != nil {
		return err
	}

	registerModel(cfg, o.driver, o.endpointID, o.name, dest)
	if err := config.Save(o.cfgPath, cfg); err != nil {
		return fmt.Errorf("update config: %w", err)
	}

	stage := "transcribe"
	if o.driver == config.DriverLlamaCPP {
		stage = "enhance"
	}
	fmt.Printf("downloaded %s (%.1f MB)\nregistered model %q on endpoint %q\n\nuse it in a hotkey:\n  \"%s\": { \"model\": \"%s:%s\" }\n",
		dest, float64(size)/(1<<20), o.name, o.endpointID, stage, o.endpointID, o.name)
	return nil
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

// findLocalModel returns the registered path for name on the endpoint under
// driver, or "".
func findLocalModel(cfg *config.Config, driver, endpointID, name string) string {
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

// registerModel adds models[name]=path to the endpoint, creating the
// driver block and/or endpoint if missing.
func registerModel(cfg *config.Config, driver, endpointID, name, path string) {
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

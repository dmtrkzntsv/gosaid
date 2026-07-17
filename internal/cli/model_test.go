package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestDeriveModelName(t *testing.T) {
	cases := map[string]string{
		"ggml-base.bin":           "base",
		"ggml-large-v3-turbo.bin": "large-v3-turbo",
		"model.gguf":              "model",
		"ggml-tiny.en.bin":        "tiny.en",
	}
	for in, want := range cases {
		if got := deriveModelName(in); got != want {
			t.Errorf("deriveModelName(%q) = %q, want %q", in, got, want)
		}
	}
}

func downloadEnv(t *testing.T, handler http.Handler) (opts modelDownloadOpts, cfgPath string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.json")
	if err := config.Save(cfgPath, config.Default()); err != nil {
		t.Fatal(err)
	}
	return modelDownloadOpts{
		repo: "ggerganov/whisper.cpp", file: "ggml-base.bin",
		name: "base", endpointID: "local",
		cfgPath: cfgPath, modelsDir: filepath.Join(dir, "models"),
		baseURL: srv.URL,
	}, cfgPath
}

func TestModelDownloadHappyPath(t *testing.T) {
	opts, cfgPath := downloadEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggerganov/whisper.cpp/resolve/main/ggml-base.bin" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("FAKE-GGML-BYTES"))
	}))
	if err := modelDownload(opts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(opts.modelsDir, "ggml-base.bin"))
	if err != nil || string(data) != "FAKE-GGML-BYTES" {
		t.Fatalf("model file wrong: %v %q", err, data)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverWhisperCPP {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == "local" {
				found = e.Config.Models["base"]
			}
		}
	}
	if found == "" || !strings.HasSuffix(found, "ggml-base.bin") {
		t.Fatalf("config not updated, got path %q", found)
	}
}

func TestModelDownloadHTTPError(t *testing.T) {
	opts, cfgPath := downloadEnv(t, http.NotFoundHandler())
	before, _ := os.ReadFile(cfgPath)
	if err := modelDownload(opts); err == nil {
		t.Fatal("expected error on 404")
	}
	if entries, _ := os.ReadDir(opts.modelsDir); len(entries) != 0 {
		t.Fatalf("expected no files left behind, got %v", entries)
	}
	after, _ := os.ReadFile(cfgPath)
	if string(before) != string(after) {
		t.Fatal("config must be untouched on failed download")
	}
}

func TestModelDownloadDuplicateWithoutForce(t *testing.T) {
	opts, _ := downloadEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("V1"))
	}))
	if err := modelDownload(opts); err != nil {
		t.Fatal(err)
	}
	err := modelDownload(opts)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected already-registered error, got: %v", err)
	}
	opts.force = true
	if err := modelDownload(opts); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
}

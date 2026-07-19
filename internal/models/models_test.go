package models

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
		if got := DeriveName(in); got != want {
			t.Errorf("DeriveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func downloadEnv(t *testing.T, handler http.Handler) (opts DownloadOpts, cfgPath string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.json")
	if err := config.Save(cfgPath, config.Default()); err != nil {
		t.Fatal(err)
	}
	return DownloadOpts{
		Repo: "ggerganov/whisper.cpp", File: "ggml-base.bin",
		Name: "base", EndpointID: "local",
		CfgPath: cfgPath, ModelsDir: filepath.Join(dir, "models"),
		BaseURL: srv.URL,
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
	if err := Download(opts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(opts.ModelsDir, "ggml-base.bin"))
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
	if err := Download(opts); err == nil {
		t.Fatal("expected error on 404")
	}
	if entries, _ := os.ReadDir(opts.ModelsDir); len(entries) != 0 {
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
	if err := Download(opts); err != nil {
		t.Fatal(err)
	}
	err := Download(opts)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected already-registered error, got: %v", err)
	}
	opts.Force = true
	if err := Download(opts); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
}

func TestModelDownloadEndpointCollisionWithOtherDriver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("V1"))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.Save(cfgPath, config.Default()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := DownloadOpts{
		Repo: "ggerganov/whisper.cpp", File: "ggml-base.bin",
		Name: "base", EndpointID: "groq", // pre-existing openai_compatible endpoint id
		CfgPath: cfgPath, ModelsDir: filepath.Join(dir, "models"),
		BaseURL: srv.URL,
	}
	err = Download(opts)
	if err == nil {
		t.Fatal("expected error registering under an endpoint id owned by another driver")
	}
	if !strings.Contains(err.Error(), "groq") || !strings.Contains(err.Error(), config.DriverOpenAICompatible) {
		t.Fatalf("expected error mentioning endpoint id and driver type, got: %v", err)
	}

	if entries, _ := os.ReadDir(opts.ModelsDir); len(entries) != 0 {
		t.Fatalf("expected no files downloaded, got %v", entries)
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("config must be untouched when endpoint id collides with another driver")
	}
}

func TestUnregisterPrunesEmptyEndpointAndDriver(t *testing.T) {
	cfg := config.Default()
	Register(cfg, "local", "base", "/models/ggml-base.bin")
	Register(cfg, "local", "tiny", "/models/ggml-tiny.bin")

	path, ok := Unregister(cfg, "local", "base")
	if !ok || path != "/models/ggml-base.bin" {
		t.Fatalf("Unregister = %q, %v", path, ok)
	}
	if got := RegisteredModels(cfg, "local"); len(got) != 1 || got["tiny"] == "" {
		t.Fatalf("RegisteredModels after first removal = %v", got)
	}

	if _, ok := Unregister(cfg, "local", "tiny"); !ok {
		t.Fatal("second Unregister failed")
	}
	if got := RegisteredModels(cfg, "local"); got != nil {
		t.Fatalf("endpoint should be pruned, RegisteredModels = %v", got)
	}
	for _, d := range cfg.Drivers {
		if d.Driver == config.DriverWhisperCPP {
			t.Fatal("empty whisper_cpp driver block should be pruned")
		}
	}

	if _, ok := Unregister(cfg, "local", "missing"); ok {
		t.Fatal("Unregister of unknown model must return ok=false")
	}
}

func TestFetchModelFileSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(existing, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Server that fails the test if contacted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be contacted for an existing file")
	}))
	t.Cleanup(srv.Close)
	dest, size, err := FetchModelFile(srv.URL, "ggerganov/whisper.cpp", "ggml-base.bin", dir, false)
	if err != nil || dest != existing || size != 0 {
		t.Fatalf("FetchModelFile = %q, %d, %v", dest, size, err)
	}
}

func TestParseHuggingFaceURL(t *testing.T) {
	const wantRepo, wantFile = "ggerganov/whisper.cpp", "ggml-small.bin"
	ok := map[string]struct{ repo, file string }{
		"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin":               {wantRepo, wantFile},
		"https://huggingface.co/ggerganov/whisper.cpp/blob/main/ggml-small.bin":                  {wantRepo, wantFile},
		"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin?download=true": {wantRepo, wantFile},
		"  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin  ":           {wantRepo, wantFile},
		"https://www.huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin":           {wantRepo, wantFile},
		"ggerganov/whisper.cpp/ggml-small.bin":                                                   {wantRepo, wantFile},
		// Nested paths and non-main revisions must survive intact.
		"https://huggingface.co/owner/repo/resolve/v2/subdir/ggml-x.bin": {"owner/repo", "subdir/ggml-x.bin"},
	}
	for in, want := range ok {
		repo, file, err := ParseHuggingFaceURL(in)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", in, err)
			continue
		}
		if repo != want.repo || file != want.file {
			t.Errorf("%q → repo %q, file %q; want %q, %q", in, repo, file, want.repo, want.file)
		}
	}

	bad := []string{
		"",
		"   ",
		"https://example.com/owner/repo/resolve/main/ggml-small.bin", // wrong host
		"https://huggingface.co/ggerganov",                           // no file
		"ggerganov/whisper.cpp",                                      // repo only
		"https://huggingface.co/owner/repo/resolve/main/README.md",   // not a .bin
	}
	for _, in := range bad {
		if repo, file, err := ParseHuggingFaceURL(in); err == nil {
			t.Errorf("%q should be rejected, got repo %q file %q", in, repo, file)
		}
	}
}

func TestCatalog(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("catalog must offer at least one model")
	}
	seenNames := map[string]bool{}
	seenFiles := map[string]bool{}
	for _, e := range Catalog {
		if e.Name == "" || e.File == "" || e.Size == "" || e.Note == "" {
			t.Errorf("incomplete entry: %+v", e)
		}
		// Names become model refs ("local:turbo"), so they must be unique and
		// colon-free. They need NOT equal DeriveName(File): the curated names
		// are deliberately friendlier than some filenames (turbo, not
		// large-v3-turbo-q5_0).
		if seenNames[e.Name] {
			t.Errorf("duplicate model name %q — refs would collide", e.Name)
		}
		seenNames[e.Name] = true
		if strings.Contains(e.Name, ":") {
			t.Errorf("model name %q must not contain ':'", e.Name)
		}
		// Two entries sharing a file would download to the same path and
		// register two names for one file.
		if seenFiles[e.File] {
			t.Errorf("duplicate model file %q", e.File)
		}
		seenFiles[e.File] = true
		if !strings.HasSuffix(e.File, ".bin") {
			t.Errorf("entry %q: file %q should be a GGML .bin", e.Name, e.File)
		}
	}
}

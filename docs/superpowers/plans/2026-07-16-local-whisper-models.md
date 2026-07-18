# Local Whisper Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed whisper.cpp via cgo so the transcribe stage can run fully locally, with a `gosaid model download` CLI command that fetches GGML models from Hugging Face and registers them in config.json.

**Architecture:** A new `whisper_cpp` driver type routes `<endpoint>:<model>` refs to locally-loaded whisper.cpp models. whisper.cpp/ggml sources are vendored under `internal/whisper/vendor/` as per-directory cgo packages (the Ollama pattern — cgo only compiles C files that live inside a Go package directory, which is why the spec's `third_party/` path is refined to this layout). `internal/whisper` is the single public cgo wrapper; `internal/drivers/whisper_cpp.go` adapts it to the existing `Driver` interface with lazy model loading.

**Tech Stack:** Go 1.25, cgo, whisper.cpp (pinned tag), Metal+Accelerate on darwin, plain CPU on linux/windows.

**Spec:** `docs/superpowers/specs/2026-07-16-local-whisper-models-design.md`

## Global Constraints

- All five release targets must build: darwin/{arm64,amd64}, linux/{amd64,arm64}, windows/amd64 (`CGO_ENABLED=1`, already the norm in release.yml).
- No `-march=native`; CPU builds rely on ggml's runtime SIMD dispatch so release binaries stay portable.
- `git clone && go build` must keep working with no extra build steps (vendored sources are committed).
- Local support covers the transcribe stage only; chat stages (`enhance`/`compose`/`translate`) must reject `whisper_cpp` endpoints at validation time.
- Model ref syntax `<endpoint_id>:<model>` is unchanged; `routing.ParseModelRef` is not modified.
- Downloads stream to `<dest>.part` and rename atomically; config.json is written only after the file is fully in place.
- No checksum verification, no HF revision pinning (downloads from `main` over TLS), no idle model eviction, no `model list` command in v1.
- Working branch: `local-whisper-models` (already created; spec is committed on it).
- This machine is linux — darwin/windows compile verification comes from the release workflow (`gh workflow run release.yml --ref local-whisper-models -f publish=false`).
- First cgo build of the vendored tree takes several minutes — use generous Bash timeouts (600000 ms) for build/test steps after Task 2.

---

### Task 1: Config schema & validation for `whisper_cpp`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Test: `internal/config/config_test.go` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `config.DriverWhisperCPP = "whisper_cpp"` (const)
  - `config.EndpointConfig` (renamed from `OpenAICompatibleConfig`) with new field `Models map[string]string \`json:"models,omitempty"\`` and `omitempty` added to `api_base`/`api_key`
  - `config.ExpandPath(p string) (string, error)` — expands a leading `~/`
  - Validation guarantees later tasks rely on: whisper endpoints have a non-empty `models` map with existing files; transcribe refs to whisper endpoints name a key in that map; chat-stage refs never point at a whisper endpoint.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func whisperTestConfig(t *testing.T, modelPath string) *Config {
	t.Helper()
	return &Config{
		Version: CurrentVersion,
		Drivers: []Driver{
			{Driver: DriverOpenAICompatible, Endpoints: []Endpoint{{
				ID: "openai", Config: EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"},
			}}},
			{Driver: DriverWhisperCPP, Endpoints: []Endpoint{{
				ID: "local", Config: EndpointConfig{Models: map[string]string{"base": modelPath}},
			}}},
		},
		Hotkeys: map[string]Hotkey{
			"ctrl+alt+space": {Transcribe: TranscribeStage{Model: "local:base"}},
		},
		ToggleMaxSeconds: 60,
	}
}

func tempModelFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(p, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidateWhisperCPPValid(t *testing.T) {
	cfg := whisperTestConfig(t, tempModelFile(t))
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateWhisperCPPEmptyModels(t *testing.T) {
	cfg := whisperTestConfig(t, tempModelFile(t))
	cfg.Drivers[1].Endpoints[0].Config.Models = nil
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "models") {
		t.Fatalf("expected models-required error, got: %v", err)
	}
}

func TestValidateWhisperCPPMissingFile(t *testing.T) {
	cfg := whisperTestConfig(t, filepath.Join(t.TempDir(), "nope.bin"))
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected file-not-found error, got: %v", err)
	}
}

func TestValidateWhisperCPPUnknownModelName(t *testing.T) {
	cfg := whisperTestConfig(t, tempModelFile(t))
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Transcribe.Model = "local:huge"
	cfg.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "huge") {
		t.Fatalf("expected unknown-model error, got: %v", err)
	}
}

func TestValidateChatStageRejectsWhisperEndpoint(t *testing.T) {
	cfg := whisperTestConfig(t, tempModelFile(t))
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Enhance = &EnhanceStage{Model: "local:base"}
	cfg.Hotkeys["ctrl+alt+space"] = hk
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "transcription only") {
		t.Fatalf("expected chat-stage rejection, got: %v", err)
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got, err := ExpandPath("~/models/x.bin")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "models", "x.bin"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, _ := ExpandPath("/abs/path.bin"); got != "/abs/path.bin" {
		t.Fatalf("absolute path must pass through, got %q", got)
	}
}
```

Add `"os"`, `"path/filepath"`, `"strings"` to the test file's imports if not present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'Whisper|ExpandPath|ChatStage' -v`
Expected: FAIL — `EndpointConfig`, `DriverWhisperCPP`, `ExpandPath` undefined.

- [ ] **Step 3: Implement config schema**

In `internal/config/config.go`:

1. Rename `OpenAICompatibleConfig` → `EndpointConfig` (update the `Endpoint.Config` field type and the literal in `Default()`), and change its body to:

```go
// EndpointConfig is the per-endpoint configuration. Which fields are required
// depends on the driver type: openai_compatible needs api_base/api_key;
// whisper_cpp needs models (name → GGML model file path).
type EndpointConfig struct {
	APIBase string            `json:"api_base,omitempty"`
	APIKey  string            `json:"api_key,omitempty"`
	Models  map[string]string `json:"models,omitempty"`
}
```

2. Add the driver const next to `DriverOpenAICompatible`:

```go
DriverWhisperCPP = "whisper_cpp"
```

3. Check for other references to the old type name: `grep -rn OpenAICompatibleConfig internal/ cmd/` — update any hits (expected: only `config.go` and possibly `config_test.go`).

- [ ] **Step 4: Implement validation**

In `internal/config/validate.go`, add imports `"os"` and `"path/filepath"`, then replace the driver loop, `validateHotkey`, and `checkModelRef` with:

```go
// endpointInfo carries what stage validation needs to know about an endpoint.
type endpointInfo struct {
	driver string
	models map[string]string // whisper_cpp only
}
```

Driver loop inside `Validate` (replacing the current one):

```go
	endpoints := map[string]endpointInfo{}
	for di, d := range cfg.Drivers {
		switch d.Driver {
		case DriverOpenAICompatible, DriverWhisperCPP:
		default:
			return fmt.Errorf("drivers[%d]: unknown driver type %q (expected %q or %q)",
				di, d.Driver, DriverOpenAICompatible, DriverWhisperCPP)
		}
		if len(d.Endpoints) == 0 {
			return fmt.Errorf("drivers[%d]: at least one endpoint is required", di)
		}
		for ei, e := range d.Endpoints {
			if e.ID == "" {
				return fmt.Errorf("drivers[%d].endpoints[%d]: id is required", di, ei)
			}
			if _, dup := endpoints[e.ID]; dup {
				return fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			switch d.Driver {
			case DriverOpenAICompatible:
				if e.Config.APIBase == "" {
					return fmt.Errorf("endpoint %q: api_base is required", e.ID)
				}
				if e.Config.APIKey == "" {
					return fmt.Errorf("endpoint %q: api_key is required", e.ID)
				}
			case DriverWhisperCPP:
				if len(e.Config.Models) == 0 {
					return fmt.Errorf("endpoint %q: a non-empty models map is required for whisper_cpp", e.ID)
				}
				for name, p := range e.Config.Models {
					abs, err := ExpandPath(p)
					if err != nil {
						return fmt.Errorf("endpoint %q: model %q: %w", e.ID, name, err)
					}
					if _, err := os.Stat(abs); err != nil {
						return fmt.Errorf("endpoint %q: model %q: file not found: %s", e.ID, name, abs)
					}
				}
			}
			endpoints[e.ID] = endpointInfo{driver: d.Driver, models: e.Config.Models}
		}
	}
```

New `checkModelRef` (the `chatStage` flag rejects whisper endpoints for chat stages; for transcribe refs it verifies the model name exists in the endpoint's map):

```go
func checkModelRef(field, ref string, endpoints map[string]endpointInfo, chatStage bool) error {
	m, err := routing.ParseModelRef(ref)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	info, ok := endpoints[m.Endpoint]
	if !ok {
		return fmt.Errorf("%s: unknown endpoint %q", field, m.Endpoint)
	}
	if info.driver == DriverWhisperCPP {
		if chatStage {
			return fmt.Errorf("%s: endpoint %q is whisper_cpp, which supports transcription only", field, m.Endpoint)
		}
		if _, ok := info.models[m.Model]; !ok {
			return fmt.Errorf("%s: endpoint %q has no model named %q", field, m.Endpoint, m.Model)
		}
	}
	return nil
}
```

Update `validateHotkey` to take `endpoints map[string]endpointInfo` and call `checkModelRef(..., false)` for `transcribe.model`, and `checkModelRef(..., true)` for `translate.model`, `enhance.model`, `compose.model`.

Add `ExpandPath` at the bottom of `validate.go`:

```go
// ExpandPath expands a leading "~/" (or bare "~") to the user's home
// directory. Other paths pass through unchanged.
func ExpandPath(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %q: %w", p, err)
	}
	return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/")), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: all PASS (including pre-existing tests — fix any compile fallout from the rename).

- [ ] **Step 6: Run the full suite and commit**

Run: `go test ./... && go vet ./...`
Expected: PASS (registry still rejects `whisper_cpp` at build time — that branch lands in Task 4; no existing test exercises it).

```bash
git add internal/config/
git commit -m "feat(config): add whisper_cpp driver type with model map validation"
```

---

### Task 2: Vendor whisper.cpp as cgo packages

**Files:**
- Create: `scripts/vendor-whisper.sh`
- Create: `internal/whisper/vendor/ggml/src/build.go`
- Create: `internal/whisper/vendor/ggml/src/ggml-cpu/build.go`
- Create: `internal/whisper/vendor/ggml/src/ggml-metal/build.go` (darwin-tagged)
- Create: `internal/whisper/vendor/src/build.go`
- Create: `internal/whisper/link.go`, `internal/whisper/link_darwin.go`
- Modify: `Makefile` (add `vendor-whisper` target)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: importable packages `internal/whisper/vendor/ggml/src` (pkg `ggmlsrc`), `.../ggml-cpu` (pkg `ggmlcpu`), `.../ggml-metal` (pkg `ggmlmetal`, darwin only), `internal/whisper/vendor/src` (pkg `whispersrc`); header include dirs `internal/whisper/vendor/include` and `internal/whisper/vendor/ggml/include`; `internal/whisper/testdata/jfk.wav`; `internal/whisper` package (currently just blank links) that compiles on linux.

**Note:** File lists below match whisper.cpp v1.7.x layout. If the pinned tag's tree differs (e.g. an `arch/` split under `ggml-cpu`, or extra subdirs like `amx/`, `llamafile/`), mirror the actual tree: every directory containing C/C++ sources gets its own tiny `build.go` following the same template, arch-specific directories get `//go:build amd64` / `arm64` tags, and the parent package's `build.go` gains nothing (linking flows through the blank imports in `internal/whisper/link.go` — add new packages there). The acceptance criterion is the build being green, not the exact file list.

- [ ] **Step 1: Write the vendor script**

Create `scripts/vendor-whisper.sh` (mode 0755):

```bash
#!/usr/bin/env bash
# Vendors whisper.cpp C/C++ sources into internal/whisper/vendor/.
# Go shim files (*.go) and VERSION are preserved; everything else is synced
# from the upstream tag. Usage: scripts/vendor-whisper.sh [tag]
set -euo pipefail

TAG="${1:-v1.7.6}"
DEST="internal/whisper/vendor"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 --branch "$TAG" https://github.com/ggml-org/whisper.cpp "$TMP/w"

# Remove previously vendored C sources, keep our Go shims.
if [ -d "$DEST" ]; then
  find "$DEST" -type f ! -name '*.go' -delete
  find "$DEST" -type d -empty -delete
fi

mkdir -p "$DEST/include" "$DEST/src" "$DEST/ggml/include" "$DEST/ggml/src"

cp "$TMP/w/include/whisper.h" "$DEST/include/"
find "$TMP/w/src" -maxdepth 1 -type f \( -name '*.cpp' -o -name '*.h' \) \
  -exec cp {} "$DEST/src/" \;
cp "$TMP/w/ggml/include/"*.h "$DEST/ggml/include/"
find "$TMP/w/ggml/src" -maxdepth 1 -type f \
  \( -name '*.c' -o -name '*.cpp' -o -name '*.h' \) \
  -exec cp {} "$DEST/ggml/src/" \;

# CPU backend (and any subdirectories it has at this tag).
cp -r "$TMP/w/ggml/src/ggml-cpu" "$DEST/ggml/src/"

# Metal backend (darwin). Layout moved over time: directory or flat files.
if [ -d "$TMP/w/ggml/src/ggml-metal" ]; then
  cp -r "$TMP/w/ggml/src/ggml-metal" "$DEST/ggml/src/"
else
  mkdir -p "$DEST/ggml/src/ggml-metal"
  cp "$TMP/w/ggml/src/ggml-metal."* "$DEST/ggml/src/ggml-metal/" 2>/dev/null || true
fi

# Assembly stub that embeds the Metal shader source into the binary
# (GGML_METAL_EMBED_LIBRARY expects these symbols).
METAL_SRC="$(find "$DEST/ggml/src/ggml-metal" -name '*.metal' | head -1)"
if [ -n "$METAL_SRC" ]; then
  cat > "$DEST/ggml/src/ggml-metal/ggml-metal-embed.s" <<EOF
.section __DATA,__ggml_metallib
.globl _ggml_metallib_start
_ggml_metallib_start:
.incbin "$(basename "$METAL_SRC")"
.globl _ggml_metallib_end
_ggml_metallib_end:
EOF
fi

# Strip build-system files that cgo must not see.
find "$DEST" -name 'CMakeLists.txt' -delete
find "$DEST" -name '*.cmake' -delete

# Speech sample for the gated integration test.
mkdir -p internal/whisper/testdata
cp "$TMP/w/samples/jfk.wav" internal/whisper/testdata/jfk.wav

echo "$TAG" > "$DEST/VERSION"
echo "vendored whisper.cpp $TAG into $DEST"
```

Add to `Makefile` (after the `clean` target):

```makefile
# Re-vendor whisper.cpp sources at a pinned tag (see internal/whisper/vendor/VERSION).
vendor-whisper:
	scripts/vendor-whisper.sh $(WHISPER_TAG)
```

and add `vendor-whisper` to the `.PHONY` line.

- [ ] **Step 2: Run the vendor script**

Check the latest stable tag at https://github.com/ggml-org/whisper.cpp/releases (use `gh release view --repo ggml-org/whisper.cpp --json tagName -q .tagName`); pass it explicitly:

Run: `chmod +x scripts/vendor-whisper.sh && scripts/vendor-whisper.sh <latest-tag>`
Expected: `vendored whisper.cpp <tag> into internal/whisper/vendor`, tree populated, `internal/whisper/testdata/jfk.wav` present.

- [ ] **Step 3: Write the cgo shim packages**

`internal/whisper/vendor/ggml/src/build.go`:

```go
// Package ggmlsrc compiles the vendored ggml core sources via cgo.
// C sources in this directory are synced by scripts/vendor-whisper.sh;
// this file is hand-maintained and survives re-vendoring.
package ggmlsrc

// #cgo CPPFLAGS: -I${SRCDIR}/../include -I${SRCDIR} -DNDEBUG -DGGML_USE_CPU
// #cgo CFLAGS: -O3 -std=c11 -fPIC -pthread
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo darwin CPPFLAGS: -DGGML_USE_METAL -DGGML_METAL_EMBED_LIBRARY -DGGML_USE_ACCELERATE -DACCELERATE_NEW_LAPACK -DACCELERATE_LAPACK_ILP64
// #cgo darwin LDFLAGS: -framework Accelerate -framework Metal -framework MetalKit -framework Foundation
// #cgo linux LDFLAGS: -lm -lstdc++ -pthread
// #cgo windows CXXFLAGS: -Wa,-mbig-obj
// #cgo windows LDFLAGS: -lm -lstdc++ -pthread -static-libgcc -static-libstdc++
import "C"
```

`internal/whisper/vendor/ggml/src/ggml-cpu/build.go`:

```go
// Package ggmlcpu compiles the vendored ggml CPU backend via cgo.
package ggmlcpu

// #cgo CPPFLAGS: -I${SRCDIR}/../../include -I${SRCDIR}/.. -I${SRCDIR} -DNDEBUG -DGGML_USE_CPU
// #cgo CFLAGS: -O3 -std=c11 -fPIC -pthread
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo darwin CPPFLAGS: -DGGML_USE_ACCELERATE -DACCELERATE_NEW_LAPACK -DACCELERATE_LAPACK_ILP64
// #cgo windows CXXFLAGS: -Wa,-mbig-obj
import "C"
```

`internal/whisper/vendor/ggml/src/ggml-metal/build.go`:

```go
//go:build darwin

// Package ggmlmetal compiles the vendored ggml Metal backend (darwin only).
package ggmlmetal

// #cgo CPPFLAGS: -I${SRCDIR}/../../include -I${SRCDIR}/.. -I${SRCDIR} -DNDEBUG -DGGML_USE_METAL -DGGML_METAL_EMBED_LIBRARY
// #cgo CFLAGS: -O3 -fPIC
// #cgo LDFLAGS: -framework Metal -framework MetalKit -framework Foundation
import "C"
```

`internal/whisper/vendor/src/build.go`:

```go
// Package whispersrc compiles the vendored whisper.cpp source via cgo.
package whispersrc

// #cgo CPPFLAGS: -I${SRCDIR}/../include -I${SRCDIR}/../ggml/include -I${SRCDIR} -DNDEBUG
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo windows CXXFLAGS: -Wa,-mbig-obj
import "C"
```

`internal/whisper/link.go`:

```go
// Package whisper wraps the vendored whisper.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package.
package whisper

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/vendor/ggml/src"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/vendor/ggml/src/ggml-cpu"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/vendor/src"
)
```

`internal/whisper/link_darwin.go`:

```go
package whisper

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/vendor/ggml/src/ggml-metal"
)
```

If the vendored tree has additional source subdirectories (e.g. `ggml-cpu/amx/`, `ggml-cpu/llamafile/`, `ggml-cpu/arch/x86/`), give each a `build.go` from the `ggmlcpu` template (adjust `-I` depth; arch dirs get `//go:build amd64` or `//go:build arm64`) and add blank imports to `link.go`.

- [ ] **Step 4: Iterate until the linux build is green**

Run: `go build ./internal/whisper/... && go build ./...` (timeout 600000)
Expected: success. Likely first-pass failures and fixes:
- missing include dir → adjust `-I` in the failing package's `build.go`
- unknown symbols at link → a source directory lacks a `build.go`/blank import
- duplicate symbols → the same source got vendored into two package dirs; delete the copy and re-check the vendor script's `find -maxdepth 1` boundaries

- [ ] **Step 5: Verify tests still pass, then commit**

Run: `go test ./... ` (timeout 600000)
Expected: PASS.

```bash
git add scripts/vendor-whisper.sh Makefile internal/whisper/
git commit -m "feat(whisper): vendor whisper.cpp sources as cgo packages"
```

(If the vendored tree is large this is expected — it's a one-time cost; do not add it to .gitignore.)

- [ ] **Step 6: Kick off a cross-platform build check**

Run: `git push -u origin local-whisper-models && gh workflow run release.yml --ref local-whisper-models -f publish=false`
Then: `gh run watch $(gh run list --workflow=release.yml --branch local-whisper-models -L1 --json databaseId -q '.[0].databaseId')` (or poll `gh run list`).
Expected: all five build legs green. Fix per-platform flag issues in the `build.go` files if not (windows/mingw is the likely offender — see the `-Wa,-mbig-obj` flags already in place). Do not proceed to Task 3 until this is green.

---

### Task 3: `internal/whisper` Go wrapper API

**Files:**
- Create: `internal/whisper/whisper.go`
- Test: `internal/whisper/whisper_test.go`
- Modify: `internal/audio/capture.go` (export `ResampleLinear`)
- Modify: `internal/audio/capture_test.go` (rename references)

**Interfaces:**
- Consumes: vendored packages from Task 2 (via the existing blank imports in `link.go`).
- Produces:
  - `whisper.Load(path string) (*whisper.Model, error)`
  - `(*whisper.Model).Close()`
  - `(*whisper.Model).Transcribe(samples []float32, opts whisper.Options) (whisper.Result, error)` — samples must be 16 kHz mono
  - `whisper.Options{Language string; Translate bool; InitialPrompt string}` (`Language: ""` = auto-detect)
  - `whisper.Result{Text string; DetectedLanguage string}`
  - `audio.ResampleLinear(in []float32, inRate, outRate int) []float32` (exported rename of `resampleLinear`)

- [ ] **Step 1: Export the resampler**

In `internal/audio/capture.go` rename `resampleLinear` → `ResampleLinear` (keep the doc comment, adjust its first word). Update the call site in `Stop()` and any references in `capture_test.go` (`grep -n resampleLinear internal/audio/`).

Run: `go test ./internal/audio/`
Expected: PASS.

- [ ] **Step 2: Write the wrapper**

Create `internal/whisper/whisper.go`:

```go
package whisper

/*
#cgo CPPFLAGS: -I${SRCDIR}/vendor/include -I${SRCDIR}/vendor/ggml/include
#include <stdlib.h>
#include "whisper.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

// Model is a loaded whisper.cpp model. Safe for concurrent use; inference
// calls are serialized internally (whisper contexts are not thread-safe).
type Model struct {
	mu  sync.Mutex
	ctx *C.struct_whisper_context
}

// Options controls a single transcription run.
type Options struct {
	Language      string // ISO 639-1 hint; "" = auto-detect
	Translate     bool   // use whisper's native translate-to-English task
	InitialPrompt string // vocabulary hint
}

// Result is the transcription output.
type Result struct {
	Text             string
	DetectedLanguage string // ISO 639-1; set on auto-detect
}

// Load reads a GGML model file into memory. Callers keep the model resident
// and call Close only on shutdown.
func Load(path string) (*Model, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	params := C.whisper_context_default_params()
	ctx := C.whisper_init_from_file_with_params(cpath, params)
	if ctx == nil {
		return nil, fmt.Errorf("whisper: failed to load model %s", path)
	}
	return &Model{ctx: ctx}, nil
}

// Close frees the underlying whisper context.
func (m *Model) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx != nil {
		C.whisper_free(m.ctx)
		m.ctx = nil
	}
}

// Transcribe runs whisper_full over 16 kHz mono samples and returns the
// concatenated segment text.
func (m *Model) Transcribe(samples []float32, opts Options) (Result, error) {
	if len(samples) == 0 {
		return Result{}, errors.New("whisper: no audio samples")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return Result{}, errors.New("whisper: model is closed")
	}

	params := C.whisper_full_default_params(C.WHISPER_SAMPLING_GREEDY)
	params.n_threads = C.int(min(4, runtime.NumCPU()))
	params.translate = C.bool(opts.Translate)
	params.no_timestamps = C.bool(true)
	params.print_progress = C.bool(false)
	params.print_realtime = C.bool(false)
	params.print_special = C.bool(false)

	lang := opts.Language
	if lang == "" {
		lang = "auto"
	}
	clang := C.CString(lang)
	defer C.free(unsafe.Pointer(clang))
	params.language = clang

	var cprompt *C.char
	if opts.InitialPrompt != "" {
		cprompt = C.CString(opts.InitialPrompt)
		defer C.free(unsafe.Pointer(cprompt))
		params.initial_prompt = cprompt
	}

	if rc := C.whisper_full(m.ctx, params, (*C.float)(unsafe.Pointer(&samples[0])), C.int(len(samples))); rc != 0 {
		return Result{}, fmt.Errorf("whisper: inference failed (code %d)", int(rc))
	}

	var text string
	n := int(C.whisper_full_n_segments(m.ctx))
	for i := 0; i < n; i++ {
		text += C.GoString(C.whisper_full_get_segment_text(m.ctx, C.int(i)))
	}

	detected := opts.Language
	if opts.Language == "" {
		detected = C.GoString(C.whisper_lang_str(C.whisper_full_lang_id(m.ctx)))
	}
	return Result{Text: strings.TrimSpace(text), DetectedLanguage: detected}, nil
}
```

- [ ] **Step 3: Write the gated integration test**

Create `internal/whisper/whisper_test.go`:

```go
package whisper

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// readWAV16 reads a 16-bit PCM mono WAV file and returns float32 samples.
// Minimal parser sufficient for testdata/jfk.wav (16 kHz mono PCM16).
func readWAV16(t *testing.T, path string) []float32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Find the "data" chunk after the 12-byte RIFF header.
	i := 12
	for i+8 <= len(data) {
		id := string(data[i : i+4])
		size := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		if id == "data" {
			pcm := data[i+8 : i+8+size]
			out := make([]float32, len(pcm)/2)
			for j := range out {
				out[j] = float32(int16(binary.LittleEndian.Uint16(pcm[j*2:]))) / 32768
			}
			return out
		}
		i += 8 + size
	}
	t.Fatal("no data chunk in WAV")
	return nil
}

// TestTranscribeJFK exercises the real model end-to-end. Gated: set
// GOSAID_WHISPER_MODEL to a GGML model path (e.g. ggml-tiny.en.bin) to run.
func TestTranscribeJFK(t *testing.T) {
	modelPath := os.Getenv("GOSAID_WHISPER_MODEL")
	if modelPath == "" {
		t.Skip("GOSAID_WHISPER_MODEL not set")
	}
	m, err := Load(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	samples := readWAV16(t, "testdata/jfk.wav")
	res, err := m.Transcribe(samples, Options{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(res.Text), "country") {
		t.Fatalf("unexpected transcript: %q", res.Text)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/model.bin"); err == nil {
		t.Fatal("expected error for missing model file")
	}
}

func TestTranscribeEmptySamples(t *testing.T) {
	m := &Model{} // no ctx needed: empty-samples check fires first
	if _, err := m.Transcribe(nil, Options{}); err == nil {
		t.Fatal("expected error for empty samples")
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/whisper/ -v` (timeout 600000)
Expected: `TestTranscribeJFK` SKIP, `TestLoadMissingFile` and `TestTranscribeEmptySamples` PASS (whisper.cpp prints C-side error logs for the missing file — that's expected noise).

Optionally run the gated test for real:

```bash
curl -L -o /tmp/claude-1000/-home-dmitry-dev-gosaid-gosaid/32396fb5-b604-4793-ae34-ce115153c4b4/scratchpad/ggml-tiny.en.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin
GOSAID_WHISPER_MODEL=/tmp/claude-1000/-home-dmitry-dev-gosaid-gosaid/32396fb5-b604-4793-ae34-ce115153c4b4/scratchpad/ggml-tiny.en.bin go test ./internal/whisper/ -run TranscribeJFK -v
```

Expected: PASS with a transcript mentioning "ask not what your country can do for you".

- [ ] **Step 5: Commit**

```bash
git add internal/whisper/ internal/audio/
git commit -m "feat(whisper): add Go wrapper API over vendored whisper.cpp"
```

---

### Task 4: `whisper_cpp` driver and registry wiring

**Files:**
- Create: `internal/drivers/whisper_cpp.go`
- Modify: `internal/drivers/registry.go`
- Test: `internal/drivers/whisper_cpp_test.go`

**Interfaces:**
- Consumes: `whisper.Load/Model/Options/Result` (Task 3), `audio.ResampleLinear`, `audio.CaptureSampleRate` (== 16000), `config.DriverWhisperCPP`, `config.ExpandPath`, `config.EndpointConfig.Models` (Task 1).
- Produces: `drivers.NewWhisperCPP(models map[string]string) *WhisperCPP` implementing `drivers.Driver`; `BuildRegistry` accepts `whisper_cpp` driver blocks.

- [ ] **Step 1: Write the failing tests**

Create `internal/drivers/whisper_cpp_test.go`:

```go
package drivers

import (
	"context"
	"strings"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

func TestWhisperCPPChatUnsupported(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"})
	_, err := d.Chat(context.Background(), "base", "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "do not support chat") {
		t.Fatalf("expected chat-unsupported error, got: %v", err)
	}
}

func TestWhisperCPPUnknownModel(t *testing.T) {
	d := NewWhisperCPP(map[string]string{"base": "/tmp/x.bin"})
	_, err := d.Transcribe(context.Background(), []float32{0}, 16000, "huge", TranscribeOptions{})
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected unknown-model error, got: %v", err)
	}
}

func TestBuildRegistryWhisperCPP(t *testing.T) {
	cfg := &config.Config{Drivers: []config.Driver{{
		Driver: config.DriverWhisperCPP,
		Endpoints: []config.Endpoint{{
			ID:     "local",
			Config: config.EndpointConfig{Models: map[string]string{"base": "/tmp/x.bin"}},
		}},
	}}}
	r, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Endpoint("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.(*WhisperCPP); !ok {
		t.Fatalf("expected *WhisperCPP, got %T", d)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/drivers/ -run WhisperCPP -v`
Expected: FAIL — `NewWhisperCPP` undefined.

- [ ] **Step 3: Implement the driver**

Create `internal/drivers/whisper_cpp.go`:

```go
package drivers

import (
	"context"
	"fmt"
	"sync"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/whisper"
)

// WhisperCPP implements Driver over locally-loaded whisper.cpp models.
// Models load lazily on first use and stay resident; a failed load is not
// cached, so the next press retries.
type WhisperCPP struct {
	mu     sync.Mutex
	paths  map[string]string // model name → GGML file path (from config)
	loaded map[string]*whisper.Model
}

func NewWhisperCPP(models map[string]string) *WhisperCPP {
	return &WhisperCPP{paths: models, loaded: map[string]*whisper.Model{}}
}

func (w *WhisperCPP) model(name string) (*whisper.Model, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if m, ok := w.loaded[name]; ok {
		return m, nil
	}
	p, ok := w.paths[name]
	if !ok {
		return nil, fmt.Errorf("whisper_cpp: unknown model %q", name)
	}
	abs, err := config.ExpandPath(p)
	if err != nil {
		return nil, err
	}
	m, err := whisper.Load(abs)
	if err != nil {
		return nil, err
	}
	w.loaded[name] = m
	return m, nil
}

func (w *WhisperCPP) run(ctx context.Context, samples []float32, sampleRate int,
	model string, opts whisper.Options) (whisper.Result, error) {
	if err := ctx.Err(); err != nil {
		return whisper.Result{}, err
	}
	m, err := w.model(model)
	if err != nil {
		return whisper.Result{}, err
	}
	if sampleRate != audio.CaptureSampleRate {
		samples = audio.ResampleLinear(samples, sampleRate, audio.CaptureSampleRate)
	}
	return m.Transcribe(samples, opts)
}

func (w *WhisperCPP) Transcribe(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranscribeOptions) (TranscribeResult, error) {
	res, err := w.run(ctx, samples, sampleRate, model, whisper.Options{
		Language:      opts.Language,
		InitialPrompt: opts.InitialPrompt,
	})
	if err != nil {
		return TranscribeResult{}, err
	}
	return TranscribeResult{Text: res.Text, DetectedLanguage: res.DetectedLanguage}, nil
}

func (w *WhisperCPP) TranslateSpeech(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranslateSpeechOptions) (string, error) {
	res, err := w.run(ctx, samples, sampleRate, model, whisper.Options{
		Language:      opts.SourceLanguage,
		Translate:     true,
		InitialPrompt: opts.InitialPrompt,
	})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// Chat is a backstop: config validation already rejects chat-stage refs to
// whisper_cpp endpoints.
func (w *WhisperCPP) Chat(ctx context.Context, model, system, user string) (string, error) {
	return "", fmt.Errorf("whisper_cpp endpoints do not support chat stages")
}
```

- [ ] **Step 4: Wire the registry**

In `internal/drivers/registry.go`, replace the driver-type check and endpoint construction inside `BuildRegistry`'s loop with:

```go
	for _, d := range cfg.Drivers {
		for _, e := range d.Endpoints {
			if _, dup := r.endpoints[e.ID]; dup {
				return nil, fmt.Errorf("duplicate endpoint id %q", e.ID)
			}
			switch d.Driver {
			case config.DriverOpenAICompatible:
				r.endpoints[e.ID] = NewOpenAICompatible(e.Config.APIBase, e.Config.APIKey)
			case config.DriverWhisperCPP:
				r.endpoints[e.ID] = NewWhisperCPP(e.Config.Models)
			default:
				return nil, fmt.Errorf("unsupported driver type %q", d.Driver)
			}
		}
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/drivers/ -v` (timeout 600000)
Expected: all PASS (new and pre-existing).

- [ ] **Step 6: Full suite and commit**

Run: `go test ./... && go vet ./...` (timeout 600000)
Expected: PASS.

```bash
git add internal/drivers/
git commit -m "feat(drivers): add whisper_cpp driver with lazy model loading"
```

---

### Task 5: `gosaid model download` CLI command

**Files:**
- Create: `internal/cli/model.go`
- Test: `internal/cli/model_test.go`
- Modify: `internal/cli/cli.go` (dispatch + usage)
- Modify: `internal/platform/paths.go` (add `ModelsDir`)

**Interfaces:**
- Consumes: `config.Load/Save/Path` (tolerant: `Load` does not validate — exactly what the spec requires), `config.DriverWhisperCPP`, `config.EndpointConfig.Models` (Task 1).
- Produces: `cli.RunModel(args []string) int` wired into `Dispatch` under `model`; `platform.ModelsDir() (string, error)` (= `<ConfigDir>/models`); internal `modelDownload(opts modelDownloadOpts) error` used by tests.

- [ ] **Step 1: Add `platform.ModelsDir`**

Append to `internal/platform/paths.go`:

```go
// ModelsDir returns the directory for downloaded GGML model files,
// alongside the config file.
func ModelsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models"), nil
}
```

- [ ] **Step 2: Write the failing tests**

Create `internal/cli/model_test.go`:

```go
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
		"ggml-base.bin":          "base",
		"ggml-large-v3-turbo.bin": "large-v3-turbo",
		"model.gguf":             "model",
		"ggml-tiny.en.bin":       "tiny.en",
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run Model -v`
Expected: FAIL — `deriveModelName`, `modelDownload`, `modelDownloadOpts` undefined.

- [ ] **Step 4: Implement the command**

Create `internal/cli/model.go`:

```go
package cli

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	endpoint := fs.String("endpoint", "local", "whisper_cpp endpoint id to register under")
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
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, modelUsage)
		return 2
	}
	if *name == "" {
		*name = deriveModelName(rest[1])
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
			})
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// deriveModelName turns a model file name into a short registered name:
// strip the "ggml-" prefix and the final extension. "ggml-base.bin" → "base".
func deriveModelName(file string) string {
	base := filepath.Base(file)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return strings.TrimPrefix(base, "ggml-")
}

type modelDownloadOpts struct {
	repo, file, name, endpointID string
	cfgPath, modelsDir, baseURL  string
	force                        bool
}

func modelDownload(o modelDownloadOpts) error {
	cfg, err := config.Load(o.cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if existing := findWhisperModel(cfg, o.endpointID, o.name); existing != "" && !o.force {
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

	registerModel(cfg, o.endpointID, o.name, dest)
	if err := config.Save(o.cfgPath, cfg); err != nil {
		return fmt.Errorf("update config: %w", err)
	}

	fmt.Printf("downloaded %s (%.1f MB)\nregistered model %q on endpoint %q\n\nuse it in a hotkey:\n  \"transcribe\": { \"model\": \"%s:%s\" }\n",
		dest, float64(size)/(1<<20), o.name, o.endpointID, o.endpointID, o.name)
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

// findWhisperModel returns the registered path for name on the endpoint, or "".
func findWhisperModel(cfg *config.Config, endpointID, name string) string {
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverWhisperCPP {
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
// whisper_cpp driver block and/or endpoint if missing.
func registerModel(cfg *config.Config, endpointID, name, path string) {
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
		Driver: config.DriverWhisperCPP,
		Endpoints: []config.Endpoint{{
			ID:     endpointID,
			Config: config.EndpointConfig{Models: map[string]string{name: path}},
		}},
	})
}
```

- [ ] **Step 5: Wire dispatch and usage**

In `internal/cli/cli.go` add a case to the switch in `Dispatch`:

```go
	case "model":
		return RunModel(args[1:])
```

and add to the `Usage()` text after the `config` line:

```
  gosaid model download <hf-repo> <file>
                   download a GGML model from Hugging Face and register it
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: all PASS.

- [ ] **Step 7: End-to-end smoke test, full suite, commit**

```bash
go build -o /tmp/claude-1000/-home-dmitry-dev-gosaid-gosaid/32396fb5-b604-4793-ae34-ce115153c4b4/scratchpad/gosaid ./cmd/gosaid
/tmp/claude-1000/-home-dmitry-dev-gosaid-gosaid/32396fb5-b604-4793-ae34-ce115153c4b4/scratchpad/gosaid model download ggerganov/whisper.cpp ggml-tiny.en.bin
```

Expected: progress output, then `registered model "tiny.en" on endpoint "local"` and a hotkey snippet; `models.tiny.en` present in the real config.json. Then remove the added block from config.json (or leave it — it validates, the file exists) and run:

Run: `go test ./... && go vet ./...` (timeout 600000)
Expected: PASS.

```bash
git add internal/cli/ internal/platform/
git commit -m "feat(cli): add model download command for local whisper models"
```

---

### Task 6: README and full-matrix CI verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: everything shipped in Tasks 1–5.
- Produces: user-facing docs; verified green five-target build.

- [ ] **Step 1: Add the README section**

Insert after the **Transcribe** stage block (before **Enhance**) in `README.md`:

```markdown
### Local transcription (no cloud)

The transcribe stage can run fully locally via embedded whisper.cpp — no API
key, no network. Download a GGML model from Hugging Face and it is registered
in your config automatically:

```
gosaid model download ggerganov/whisper.cpp ggml-base.bin
```

This creates a `whisper_cpp` driver block:

```json
{
  "driver": "whisper_cpp",
  "endpoints": [
    {
      "id": "local",
      "config": {
        "models": { "base": "/path/to/models/ggml-base.bin" }
      }
    }
  ]
}
```

Reference it from a hotkey like any other endpoint:

```json
"transcribe": { "model": "local:base" }
```

Notes:

- Pick a model by RAM/speed trade-off: `ggml-tiny.bin` (~75 MB), `ggml-base.bin`
  (~140 MB), `ggml-small.bin` (~460 MB), `ggml-large-v3-turbo.bin` (~1.6 GB).
  The model loads on first use and stays in memory.
- On macOS inference runs on the GPU (Metal); elsewhere on CPU.
- Local models cover **transcription only** — `enhance`, `compose`, and
  `translate` still need an OpenAI-compatible endpoint (cloud, or a local
  server like Ollama via `api_base`).
- `--name`, `--endpoint`, and `--force` flags customize registration; any
  Hugging Face repo/file that hosts GGML whisper models works.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document local transcription via embedded whisper.cpp"
```

- [ ] **Step 3: Full-matrix CI verification**

```bash
git push origin local-whisper-models
gh workflow run release.yml --ref local-whisper-models -f publish=false
```

Watch: `gh run watch $(gh run list --workflow=release.yml --branch local-whisper-models -L1 --json databaseId -q '.[0].databaseId')`
Expected: **all five build legs green** (this is the all-platforms-day-one requirement). If windows fails on mingw, iterate on `#cgo windows` flags in the vendored `build.go` files; if darwin fails on Metal embedding, check the `.s` stub path and `GGML_METAL_EMBED_LIBRARY` define. Re-push and re-run until green.

- [ ] **Step 4: Final verification**

Run: `go test ./... && go vet ./... && go build ./...` (timeout 600000)
Expected: PASS. The branch is ready for review/merge (use superpowers:finishing-a-development-branch).

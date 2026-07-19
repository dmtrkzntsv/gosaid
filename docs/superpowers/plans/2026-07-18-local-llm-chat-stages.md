# Local LLM for Chat Stages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the chat stages (enhance/compose/translate) fully locally by embedding llama.cpp via cgo, sharing a single vendored ggml with the existing whisper.cpp embedding.

**Architecture:** ggml is promoted from `internal/whisper/cvendor/ggml` to a shared `internal/ggml/cvendor` (one copy of ggml's C symbols per binary — two copies collide at link time). llama.cpp core sources are vendored under `internal/llama/cvendor`, wrapped by a thin cgo package `internal/llama`, and exposed as a new `llama_cpp` driver that reuses a generic model cache extracted from the whisper driver. `gosaid model download` learns that `.gguf` files register under `llama_cpp`.

**Tech Stack:** Go 1.25+, cgo, llama.cpp (core lib only), ggml (shared with whisper.cpp), Metal on darwin / portable CPU elsewhere.

**Spec:** `docs/superpowers/specs/2026-07-18-local-llm-chat-stages-design.md`

## Global Constraints

- Single static binary; no external runtime (Ollama/llama-server) required or managed.
- Vendor only llama.cpp's core library (`src/`, `include/llama.h`) — not `common/`, server, or multimodal.
- The vendored (whisper.cpp tag, llama.cpp tag) pair must share one ggml version; ggml is vendored **from the llama.cpp tag**. Currently vendored ggml: `0.15.1` (from whisper.cpp `v1.9.1`).
- All five release targets: darwin/linux/windows × amd64/arm64 (windows amd64 only). CPU builds stay portable — runtime SIMD dispatch, no `-march=native`.
- Chat calls are stateless single-turn: fresh `llama_context` per call, no KV reuse, no streaming.
- Models without an embedded chat template are rejected with an error naming the model.
- Sampling: near-greedy (temperature 0.2 + min-p 0.05). `MaxTokens` default 1024; hitting it returns the truncated text, not an error. Context: model's trained context capped at 8192.
- New driver type string: `llama_cpp`. Default download endpoint id for `.gguf`: `local-llm`.
- `go test ./...` stays fast and hermetic: cgo inference tests are gated behind `GOSAID_LLAMA_MODEL` (mirroring `GOSAID_WHISPER_MODEL`).
- Commit after every task. Run `make fmt vet` before each commit.

## File Structure

| Path | Responsibility |
|---|---|
| `internal/ggml/ggml.go` + `link_*.go` | NEW — links vendored ggml (core, CPU, Metal-on-darwin, per-arch) into any importer |
| `internal/ggml/cvendor/**` | MOVED from `internal/whisper/cvendor/ggml/**` — the single vendored ggml tree |
| `internal/whisper/link.go` | MODIFIED — imports `internal/ggml` + whisper src only |
| `internal/whisper/whisper.go` | MODIFIED — ggml include path only |
| `internal/whisper/cvendor/src/build.go` | MODIFIED — ggml include path only |
| `internal/llama/link.go` | NEW — links vendored llama.cpp + `internal/ggml` |
| `internal/llama/llama.go` | NEW — cgo wrapper: `Load`, `Model.Chat`, `Model.Close` |
| `internal/llama/llama_test.go` | NEW — gated integration test |
| `internal/llama/cvendor/**` | NEW — vendored llama.cpp core sources + build shims |
| `scripts/vendor-ggml.sh` | NEW — vendors ggml from the llama.cpp tag (incl. Metal embed stub, version defs) |
| `scripts/vendor-llama.sh` | NEW — vendors llama.cpp core sources; gates on ggml version match |
| `scripts/vendor-whisper.sh` | MODIFIED — whisper.cpp sources only (ggml parts removed) |
| `scripts/vendor-versions.env` | NEW — single manifest pinning `WHISPER_TAG` / `LLAMA_TAG` |
| `internal/drivers/modelcache.go` | NEW — generic lazy-load/idle-unload cache extracted from WhisperCPP |
| `internal/drivers/modelcache_test.go` | NEW — cache tests (moved/adapted from whisper_cpp_test.go) |
| `internal/drivers/whisper_cpp.go` | MODIFIED — refactored onto modelCache |
| `internal/drivers/llama_cpp.go` + `_test.go` | NEW — LlamaCPP driver |
| `internal/drivers/registry.go` | MODIFIED — `llama_cpp` case |
| `internal/config/config.go` | MODIFIED — `DriverLlamaCPP` constant, comment updates |
| `internal/config/validate.go` + `_test.go` | MODIFIED — llama_cpp rules, transcribe-ref rejection |
| `internal/cli/model.go` + `_test.go` | MODIFIED — `.gguf` branch, name derivation, snippets |
| `internal/config/config.example.json` | MODIFIED — llama_cpp example block |
| `Makefile` | MODIFIED — `vendor-ggml`, `vendor-llama`, `vendor` targets |
| `README.md` | MODIFIED — "Local models" section |

---

### Task 1: Promote ggml to `internal/ggml` (mechanical move)

No upstream version changes in this task — pure restructure of the existing vendored tree. `make build && make test` green at the end is the acceptance gate.

**Files:**
- Move: `internal/whisper/cvendor/ggml/` → `internal/ggml/cvendor/`
- Create: `internal/ggml/ggml.go`, `internal/ggml/link_amd64.go`, `internal/ggml/link_arm64.go`, `internal/ggml/link_darwin.go`
- Modify: `internal/whisper/link.go`, `internal/whisper/whisper.go`, `internal/whisper/cvendor/src/build.go`, `scripts/vendor-whisper.sh`, `Makefile`
- Delete: `internal/whisper/link_amd64.go`, `internal/whisper/link_arm64.go`, `internal/whisper/link_darwin.go`

**Interfaces:**
- Consumes: existing vendored ggml tree and Go build shims (unchanged content).
- Produces: package `github.com/dmtrkzntsv/gosaid/internal/ggml` — blank-import it to link ggml. Later tasks (llama vendoring, wrapper) depend on this import path and on `internal/ggml/cvendor/include` as the ggml header dir.

- [ ] **Step 1: Move the vendored ggml tree**

```bash
mkdir -p internal/ggml
git mv internal/whisper/cvendor/ggml internal/ggml/cvendor
```

The build shims inside (`cvendor/src/build.go`, `cvendor/src/ggml-cpu/build.go`, `cvendor/src/ggml-metal/build.go`, `cvendor/src/ggml-cpu/arch/{x86,arm}/build.go`) keep their package names and relative include paths (`-I${SRCDIR}/../include` etc. still resolve within the moved tree) — do not edit them, except the two header-comment references to `scripts/vendor-whisper.sh` in `cvendor/src/build.go`, which change to `scripts/vendor-ggml.sh` (created in Task 2).

- [ ] **Step 2: Create the ggml link package**

`internal/ggml/ggml.go`:

```go
// Package ggml links the vendored ggml library (core, CPU backend, and on
// darwin the Metal backend) into any binary that imports it. whisper.cpp and
// llama.cpp both compile against this single copy — a second vendored ggml
// would collide at link time (duplicate C symbols).
//
// The vendored C/C++ sources live under internal/ggml/cvendor/ (not
// "vendor/": the Go toolchain reserves that name for module vendoring).
// They are synced by scripts/vendor-ggml.sh from the pinned llama.cpp tag
// (see scripts/vendor-versions.env).
package ggml

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml/cvendor/src"
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml/cvendor/src/ggml-cpu"
)
```

`internal/ggml/link_amd64.go`:

```go
package ggml

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml/cvendor/src/ggml-cpu/arch/x86"
)
```

`internal/ggml/link_arm64.go`:

```go
package ggml

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml/cvendor/src/ggml-cpu/arch/arm"
)
```

`internal/ggml/link_darwin.go`:

```go
//go:build darwin

package ggml

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml/cvendor/src/ggml-metal"
)
```

- [ ] **Step 3: Rewire the whisper package**

Replace the entire contents of `internal/whisper/link.go` with:

```go
// Package whisper wraps the vendored whisper.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package;
// ggml itself is linked via the shared internal/ggml package.
package whisper

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/cvendor/src"
)
```

Delete `internal/whisper/link_amd64.go`, `internal/whisper/link_arm64.go`, `internal/whisper/link_darwin.go`:

```bash
git rm internal/whisper/link_amd64.go internal/whisper/link_arm64.go internal/whisper/link_darwin.go
```

In `internal/whisper/whisper.go`, change the cgo preamble line

```
#cgo CPPFLAGS: -I${SRCDIR}/cvendor/include -I${SRCDIR}/cvendor/ggml/include
```

to

```
#cgo CPPFLAGS: -I${SRCDIR}/cvendor/include -I${SRCDIR}/../ggml/cvendor/include
```

In `internal/whisper/cvendor/src/build.go`, change

```
// #cgo CPPFLAGS: -I${SRCDIR}/../include -I${SRCDIR}/../ggml/include -I${SRCDIR} -DNDEBUG -include ${SRCDIR}/whisper-version-defs.h
```

to

```
// #cgo CPPFLAGS: -I${SRCDIR}/../include -I${SRCDIR}/../../../ggml/cvendor/include -I${SRCDIR} -DNDEBUG -include ${SRCDIR}/whisper-version-defs.h
```

(`${SRCDIR}` is `internal/whisper/cvendor/src`, so `../../..` is `internal/`.)

- [ ] **Step 4: Strip ggml handling from `scripts/vendor-whisper.sh`**

Remove from the script (all move to `vendor-ggml.sh` in Task 2):
- `mkdir` of `$DEST/ggml/include` and `$DEST/ggml/src`
- the `cp` of `$TMP/w/ggml/include/*.h` and the `find $TMP/w/ggml/src ...` copy
- the `cp -r` of `ggml-cpu` and the entire Metal-backend copy + embed-stub block (from `# CPU backend` through the `EOF` of `ggml-metal-embed.s`)
- the `GGML_MAJOR`/`GGML_MINOR`/`GGML_PATCH`/`GGML_VER` extraction and the `ggml-version-defs.h` heredoc

Keep: whisper src/include copy, `WHISPER_VER` extraction and `whisper-version-defs.h` generation, CMake-file stripping, `jfk.wav`, `VERSION` stamp. Update the script's header comment to note ggml is vendored separately by `vendor-ggml.sh`.

- [ ] **Step 5: Build and test**

Run: `make build && make test`
Expected: build succeeds (proves include paths and link shims are right); all tests pass. If a header isn't found, re-check the two edited `CPPFLAGS` lines against the paths above.

- [ ] **Step 6: Commit**

```bash
make fmt vet
git add -A
git commit -m "refactor: promote vendored ggml to shared internal/ggml"
```

---

### Task 2: Vendor llama.cpp with matching ggml

Introduces the version manifest, the two new vendor scripts, the vendored llama.cpp tree with its cgo build shims, and the `internal/llama` link file. The acceptance gate is `go build ./...` compiling the whole vendored trio together.

**Files:**
- Create: `scripts/vendor-versions.env`, `scripts/vendor-ggml.sh`, `scripts/vendor-llama.sh`, `internal/llama/link.go`, `internal/llama/cvendor/src/build.go` (+ one `build.go` per `src/` subdirectory that upstream ships, e.g. `src/models/`)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `internal/ggml` (Task 1) — import path and `internal/ggml/cvendor/include`.
- Produces: vendored `internal/llama/cvendor/include/llama.h` + compiled objects, linked by blank-importing `github.com/dmtrkzntsv/gosaid/internal/llama` packages. Task 3's cgo wrapper includes `llama.h` against these.

- [ ] **Step 1: Pick the llama.cpp tag**

The vendored ggml is `0.15.1` (see `internal/ggml/cvendor/src/ggml-version-defs.h`). Find the newest llama.cpp release tag whose ggml matches:

```bash
git clone --filter=blob:none --no-checkout https://github.com/ggml-org/llama.cpp /tmp/llama-probe
cd /tmp/llama-probe
for t in $(git tag --sort=-creatordate | head -60); do
  v=$(git show "$t:ggml/CMakeLists.txt" 2>/dev/null | sed -n \
    -e 's/.*set(GGML_VERSION_MAJOR \([0-9]*\)).*/\1/p' \
    -e 's/.*set(GGML_VERSION_MINOR \([0-9]*\)).*/\1/p' \
    -e 's/.*set(GGML_VERSION_PATCH \([0-9]*\)).*/\1/p' | paste -sd. -)
  echo "$t ggml=$v"
done | grep 'ggml=0\.15\.1' | head -5
```

Take the newest matching tag (a `b####` build tag). **If no tag matches 0.15.1:** pick the newest whisper.cpp tag ≥ v1.9.1 and llama.cpp tag that DO share a ggml version (run the same probe loop against `https://github.com/ggml-org/whisper.cpp`), set both tags in the manifest below, and re-run all three vendor scripts — the build in Step 7 plus the whisper integration test are the gate.

- [ ] **Step 2: Write the version manifest**

`scripts/vendor-versions.env` (substitute the discovered tag):

```bash
# Pinned upstream tags for the vendored C libraries. whisper.cpp and
# llama.cpp must agree on one ggml version — ggml itself is vendored from
# LLAMA_TAG (its ggml sync is the most current). Change these together and
# re-run `make vendor`.
WHISPER_TAG=v1.9.1
LLAMA_TAG=<discovered-tag>
```

- [ ] **Step 3: Write `scripts/vendor-ggml.sh`**

This is the ggml portion removed from `vendor-whisper.sh` in Task 1, retargeted at the llama.cpp repo and `internal/ggml/cvendor`:

```bash
#!/usr/bin/env bash
# Vendors ggml C/C++ sources (core, CPU backend, Metal backend) into
# internal/ggml/cvendor/, from the pinned llama.cpp tag — llama.cpp's ggml
# sync is the most current, and whisper.cpp must build against the same
# version (see scripts/vendor-versions.env). Go shim files (*.go) and
# VERSION are preserved; everything else is synced.
# Usage: scripts/vendor-ggml.sh [llama.cpp tag]
set -euo pipefail

cd "$(dirname "$0")/.."
source scripts/vendor-versions.env
TAG="${1:-$LLAMA_TAG}"
DEST="internal/ggml/cvendor"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 --branch "$TAG" https://github.com/ggml-org/llama.cpp "$TMP/l"
SRC="$TMP/l/ggml"

if [ -d "$DEST" ]; then
  find "$DEST" -type f ! -name '*.go' -delete
  find "$DEST" -type d -empty -delete
fi
mkdir -p "$DEST/include" "$DEST/src"

cp "$SRC/include/"*.h "$DEST/include/"
find "$SRC/src" -maxdepth 1 -type f \
  \( -name '*.c' -o -name '*.cpp' -o -name '*.h' \) \
  -exec cp {} "$DEST/src/" \;
cp -r "$SRC/src/ggml-cpu" "$DEST/src/"

# Metal backend (darwin). Layout moved over time: directory or flat files.
if [ -d "$SRC/src/ggml-metal" ]; then
  cp -r "$SRC/src/ggml-metal" "$DEST/src/"
else
  mkdir -p "$DEST/src/ggml-metal"
  cp "$SRC/src/ggml-metal."* "$DEST/src/ggml-metal/" 2>/dev/null || true
fi

# Assembly stub embedding the Metal shader source into the binary
# (GGML_METAL_EMBED_LIBRARY expects these symbols). Mirror upstream CMake:
# inline ggml-common.h and ggml-metal-impl.h to make the shader source
# self-contained (it is compiled by Metal at runtime with no include paths).
METAL_SRC="$(find "$DEST/src/ggml-metal" -name 'ggml-metal.metal' | head -1)"
if [ -n "$METAL_SRC" ]; then
  METAL_DIR="$(dirname "$METAL_SRC")"
  sed -e "/__embed_ggml-common.h__/r $DEST/src/ggml-common.h" \
      -e '/__embed_ggml-common.h__/d' "$METAL_SRC" \
    | sed -e "/#include \"ggml-metal-impl.h\"/r $METAL_DIR/ggml-metal-impl.h" \
          -e '/#include "ggml-metal-impl.h"/d' \
    > "$METAL_DIR/ggml-metal-embed.metal"
  cat > "$METAL_DIR/ggml-metal-embed.s" <<EOF
.section __DATA,__ggml_metallib
.globl _ggml_metallib_start
_ggml_metallib_start:
.incbin "ggml-metal-embed.metal"
.globl _ggml_metallib_end
_ggml_metallib_end:
EOF
fi

# Version macros normally injected by CMake; cgo forbids '"' in #cgo flags,
# so generate a force-included header instead.
GGML_MAJOR="$(sed -n 's/.*set(GGML_VERSION_MAJOR \([0-9][0-9]*\)).*/\1/p' "$SRC/CMakeLists.txt")"
GGML_MINOR="$(sed -n 's/.*set(GGML_VERSION_MINOR \([0-9][0-9]*\)).*/\1/p' "$SRC/CMakeLists.txt")"
GGML_PATCH="$(sed -n 's/.*set(GGML_VERSION_PATCH \([0-9][0-9]*\)).*/\1/p' "$SRC/CMakeLists.txt")"
cat > "$DEST/src/ggml-version-defs.h" <<EOF
// Generated by scripts/vendor-ggml.sh. Do not edit.
#pragma once
#ifndef GGML_VERSION
#define GGML_VERSION "${GGML_MAJOR}.${GGML_MINOR}.${GGML_PATCH}"
#endif
#ifndef GGML_COMMIT
#define GGML_COMMIT "unknown"
#endif
EOF

find "$DEST" -name 'CMakeLists.txt' -delete
find "$DEST" -name '*.cmake' -delete
find "$DEST" -type d -empty -delete

echo "$TAG" > "$DEST/VERSION"
echo "vendored ggml ${GGML_MAJOR}.${GGML_MINOR}.${GGML_PATCH} (llama.cpp $TAG) into $DEST"
```

```bash
chmod +x scripts/vendor-ggml.sh
```

- [ ] **Step 4: Write `scripts/vendor-llama.sh`**

```bash
#!/usr/bin/env bash
# Vendors llama.cpp core library sources into internal/llama/cvendor/.
# Only src/ and include/llama.h — no common/, server, or multimodal.
# Go shim files (*.go) are preserved. Fails if the tag's ggml version does
# not match the vendored internal/ggml tree (re-run vendor-ggml.sh first).
# Usage: scripts/vendor-llama.sh [tag]
set -euo pipefail

cd "$(dirname "$0")/.."
source scripts/vendor-versions.env
TAG="${1:-$LLAMA_TAG}"
DEST="internal/llama/cvendor"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

git clone --depth 1 --branch "$TAG" https://github.com/ggml-org/llama.cpp "$TMP/l"

# ggml compatibility gate.
VENDORED="$(sed -n 's/#define GGML_VERSION "\(.*\)"/\1/p' internal/ggml/cvendor/src/ggml-version-defs.h)"
UPSTREAM="$(sed -n \
  -e 's/.*set(GGML_VERSION_MAJOR \([0-9]*\)).*/\1/p' \
  -e 's/.*set(GGML_VERSION_MINOR \([0-9]*\)).*/\1/p' \
  -e 's/.*set(GGML_VERSION_PATCH \([0-9]*\)).*/\1/p' \
  "$TMP/l/ggml/CMakeLists.txt" | paste -sd. -)"
if [ "$VENDORED" != "$UPSTREAM" ]; then
  echo "error: vendored ggml is $VENDORED but llama.cpp $TAG ships ggml $UPSTREAM" >&2
  echo "       run scripts/vendor-ggml.sh $TAG first (and re-verify whisper builds)" >&2
  exit 1
fi

if [ -d "$DEST" ]; then
  find "$DEST" -type f ! -name '*.go' -delete
  find "$DEST" -type d -empty -delete
fi
mkdir -p "$DEST/include" "$DEST/src"

cp "$TMP/l/include/llama.h" "$DEST/include/"
find "$TMP/l/src" -maxdepth 1 -type f \( -name '*.cpp' -o -name '*.h' \) \
  -exec cp {} "$DEST/src/" \;

# Each src/ subdirectory needs its own cgo shim package (cgo compiles only
# the sources in a package's own directory). Copy them and list what was
# found so a new upstream subdirectory is noticed, not silently dropped.
for d in "$TMP/l/src"/*/; do
  name="$(basename "$d")"
  cp -r "$d" "$DEST/src/$name"
  if [ ! -f "$DEST/src/$name/build.go" ]; then
    echo "warning: $DEST/src/$name has no build.go shim — add one or the sources will not compile" >&2
  fi
done

find "$DEST" -name 'CMakeLists.txt' -delete
find "$DEST" -name '*.cmake' -delete
find "$DEST" -type d -empty -delete

echo "$TAG" > "$DEST/VERSION"
echo "vendored llama.cpp $TAG into $DEST"
```

```bash
chmod +x scripts/vendor-llama.sh
```

- [ ] **Step 5: Update the Makefile vendor targets**

Replace the `vendor-whisper` block with:

```make
# Re-vendor the C libraries at the tags pinned in scripts/vendor-versions.env.
# ggml is shared: whisper.cpp and llama.cpp must agree on its version, so
# change tags together and re-run `make vendor`.
vendor: vendor-ggml vendor-whisper vendor-llama

vendor-ggml:
	scripts/vendor-ggml.sh $(LLAMA_TAG)

vendor-whisper:
	scripts/vendor-whisper.sh $(WHISPER_TAG)

vendor-llama:
	scripts/vendor-llama.sh $(LLAMA_TAG)
```

(`$(LLAMA_TAG)`/`$(WHISPER_TAG)` empty means the scripts read the manifest.) Add `vendor vendor-ggml vendor-llama` to `.PHONY`.

Also point `scripts/vendor-whisper.sh` at the manifest so its default tag can't drift from it — replace its `TAG="${1:-v1.9.1}"` line with:

```bash
cd "$(dirname "$0")/.."
source scripts/vendor-versions.env
TAG="${1:-$WHISPER_TAG}"
```

- [ ] **Step 6: Run the vendor scripts and write the build shims**

```bash
scripts/vendor-ggml.sh && scripts/vendor-whisper.sh && scripts/vendor-llama.sh
```

(Re-vendoring ggml from the llama.cpp tag may produce a byte-identical tree if the versions match exactly; either way the build is the gate.)

`internal/llama/cvendor/src/build.go`:

```go
// Package llamasrc compiles the vendored llama.cpp core sources via cgo.
// C sources in this directory are synced by scripts/vendor-llama.sh; this
// file is hand-maintained and survives re-vendoring.
package llamasrc

// #cgo CPPFLAGS: -I${SRCDIR}/../include -I${SRCDIR}/../../../ggml/cvendor/include -I${SRCDIR} -DNDEBUG
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo linux CPPFLAGS: -D_XOPEN_SOURCE=600 -D_GNU_SOURCE
// #cgo darwin CPPFLAGS: -D_XOPEN_SOURCE=600 -D_DARWIN_C_SOURCE
// #cgo windows CXXFLAGS: -Wa,-mbig-obj
import "C"
```

For each subdirectory the vendor script reported (recent llama.cpp ships `src/models/`), add the analogous shim — e.g. `internal/llama/cvendor/src/models/build.go`:

```go
// Package llamamodels compiles the vendored llama.cpp per-architecture
// model sources via cgo.
package llamamodels

// #cgo CPPFLAGS: -I${SRCDIR}/../../include -I${SRCDIR}/../../../../ggml/cvendor/include -I${SRCDIR}/.. -I${SRCDIR} -DNDEBUG
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo linux CPPFLAGS: -D_XOPEN_SOURCE=600 -D_GNU_SOURCE
// #cgo darwin CPPFLAGS: -D_XOPEN_SOURCE=600 -D_DARWIN_C_SOURCE
// #cgo windows CXXFLAGS: -Wa,-mbig-obj
import "C"
```

`internal/llama/link.go` (blank-import every shim package created above):

```go
// Package llama wraps the vendored llama.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package;
// ggml itself is linked via the shared internal/ggml package.
package llama

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml"
	_ "github.com/dmtrkzntsv/gosaid/internal/llama/cvendor/src"
	_ "github.com/dmtrkzntsv/gosaid/internal/llama/cvendor/src/models"
)
```

(Drop the `models` import if upstream has no such subdirectory at the pinned tag; add one line per additional subdirectory shim.)

- [ ] **Step 7: Build everything**

Run: `go build ./... && make build && make test`
Expected: full compile and link (several minutes for the first llama.cpp build). Compile errors in llama sources referencing missing ggml symbols mean the ggml versions do NOT actually match — go back to Step 1's fallback. A missing-header error in a `src/` subdirectory means a shim's include paths need the subdirectory's parent added.

- [ ] **Step 8: Commit**

```bash
make fmt vet
git add -A
git commit -m "feat: vendor llama.cpp core with shared ggml"
```

---

### Task 3: `internal/llama` cgo wrapper

**Files:**
- Create: `internal/llama/llama.go`
- Test: `internal/llama/llama_test.go`

**Interfaces:**
- Consumes: vendored `llama.h` (Task 2). **Note:** the code below targets the llama.h API current as of early 2026 (`llama_model_load_from_file`, `llama_model_get_vocab`, `llama_init_from_model`, `llama_sampler_*`, `llama_vocab_is_eog`). If the vendored tag has drifted, adapt names to `internal/llama/cvendor/include/llama.h` — the structure stands.
- Produces (Task 6 depends on these exact signatures):
  - `llama.Load(path string) (*llama.Model, error)`
  - `(*llama.Model).Chat(ctx context.Context, system, user string, opts llama.Options) (string, error)`
  - `(*llama.Model).Close()`
  - `llama.Options{ MaxTokens int }`

- [ ] **Step 1: Write the gated integration test**

`internal/llama/llama_test.go`:

```go
package llama

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Integration test against a real GGUF model. Set GOSAID_LLAMA_MODEL to a
// small instruct model path to run (e.g. a 0.5-1B Q4 GGUF); skipped
// otherwise so `go test ./...` stays fast and hermetic.
func TestChatIntegration(t *testing.T) {
	path := os.Getenv("GOSAID_LLAMA_MODEL")
	if path == "" {
		t.Skip("GOSAID_LLAMA_MODEL not set")
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	out, err := m.Chat(context.Background(),
		"You answer with exactly one word.",
		"What is the capital of France? Answer with one word only.",
		Options{MaxTokens: 32})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(out), "paris") {
		t.Errorf("expected answer containing 'paris', got %q", out)
	}
}

func TestChatCancelled(t *testing.T) {
	path := os.Getenv("GOSAID_LLAMA_MODEL")
	if path == "" {
		t.Skip("GOSAID_LLAMA_MODEL not set")
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Chat(ctx, "", "Count to one thousand.", Options{}); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/model.gguf"); err == nil {
		t.Fatal("expected error loading nonexistent file")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/llama/ -run TestLoadMissingFile -v`
Expected: FAIL — `undefined: Load` (package has only link.go so far).

- [ ] **Step 3: Write the wrapper**

`internal/llama/llama.go`:

```go
package llama

/*
#cgo CPPFLAGS: -I${SRCDIR}/cvendor/include -I${SRCDIR}/../ggml/cvendor/include
#include <stdlib.h>
#include "llama.h"
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// Model is a loaded llama.cpp model. Safe for concurrent use; inference
// calls are serialized internally.
type Model struct {
	mu    sync.Mutex
	model *C.struct_llama_model
	tmpl  *C.char // built-in chat template; memory owned by the model
	path  string
}

// Options controls a single chat completion.
type Options struct {
	// MaxTokens caps generated tokens; 0 uses the default. Hitting the cap
	// returns the truncated text, not an error.
	MaxTokens int
}

const (
	defaultMaxTokens = 1024
	// maxContextTokens caps n_ctx below huge trained contexts: dictation
	// inputs are short, and KV-cache memory scales with n_ctx.
	maxContextTokens = 8192
)

var backendOnce sync.Once

// Load reads a GGUF model file into memory (fully GPU-offloaded on Metal
// builds). Models without an embedded chat template are rejected: chat
// formatting depends on it, and a wrong guess yields garbage output.
func Load(path string) (*Model, error) {
	backendOnce.Do(func() { C.llama_backend_init() })

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	params := C.llama_model_default_params()
	params.n_gpu_layers = 999 // ignored on CPU-only builds
	model := C.llama_model_load_from_file(cpath, params)
	if model == nil {
		return nil, fmt.Errorf("llama: failed to load model %s", path)
	}
	tmpl := C.llama_model_chat_template(model, nil)
	if tmpl == nil {
		C.llama_model_free(model)
		return nil, fmt.Errorf("llama: model %s has no embedded chat template; use an instruct-tuned GGUF", path)
	}
	return &Model{model: model, tmpl: tmpl, path: path}, nil
}

// Close frees the underlying model.
func (m *Model) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.model != nil {
		C.llama_model_free(m.model)
		m.model = nil
	}
}

// Chat runs a single-turn system+user completion and returns the
// assistant's text. Each call uses a fresh context; nothing is retained
// between calls. ctx is checked between decode steps, so cancellation
// stops a long generation promptly.
func (m *Model) Chat(ctx context.Context, system, user string, opts Options) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.model == nil {
		return "", fmt.Errorf("llama: model is closed")
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	prompt, err := m.applyTemplate(system, user)
	if err != nil {
		return "", err
	}
	vocab := C.llama_model_get_vocab(m.model)
	tokens, err := tokenize(vocab, prompt)
	if err != nil {
		return "", err
	}

	nCtx := min(int(C.llama_model_n_ctx_train(m.model)), maxContextTokens)
	if len(tokens) >= nCtx {
		return "", fmt.Errorf("llama: prompt (%d tokens) exceeds the context window (%d)", len(tokens), nCtx)
	}
	if len(tokens)+maxTokens > nCtx {
		maxTokens = nCtx - len(tokens)
	}

	cparams := C.llama_context_default_params()
	cparams.n_ctx = C.uint32_t(nCtx)
	cparams.n_batch = C.uint32_t(nCtx) // whole prompt in one decode call
	lctx := C.llama_init_from_model(m.model, cparams)
	if lctx == nil {
		return "", fmt.Errorf("llama: failed to create inference context")
	}
	defer C.llama_free(lctx)

	// Near-greedy sampling: enhance/translate want faithfulness; the small
	// temperature keeps compose phrasing natural without drifting.
	smpl := C.llama_sampler_chain_init(C.llama_sampler_chain_default_params())
	defer C.llama_sampler_free(smpl)
	C.llama_sampler_chain_add(smpl, C.llama_sampler_init_min_p(0.05, 1))
	C.llama_sampler_chain_add(smpl, C.llama_sampler_init_temp(0.2))
	C.llama_sampler_chain_add(smpl, C.llama_sampler_init_dist(C.LLAMA_DEFAULT_SEED))

	var sb strings.Builder
	batch := C.llama_batch_get_one(&tokens[0], C.int32_t(len(tokens)))
	for i := 0; i < maxTokens; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if rc := C.llama_decode(lctx, batch); rc != 0 {
			return "", fmt.Errorf("llama: decode failed (code %d)", int(rc))
		}
		tok := C.llama_sampler_sample(smpl, lctx, -1)
		if C.llama_vocab_is_eog(vocab, tok) {
			break
		}
		piece, err := tokenToPiece(vocab, tok)
		if err != nil {
			return "", err
		}
		sb.WriteString(piece)
		tokens[0] = tok
		batch = C.llama_batch_get_one(&tokens[0], 1)
	}
	return strings.TrimSpace(sb.String()), nil
}

// applyTemplate renders system+user through the model's chat template,
// with the assistant turn opened.
func (m *Model) applyTemplate(system, user string) (string, error) {
	cstrs := []*C.char{
		C.CString("system"), C.CString(system),
		C.CString("user"), C.CString(user),
	}
	defer func() {
		for _, p := range cstrs {
			C.free(unsafe.Pointer(p))
		}
	}()
	msgs := []C.struct_llama_chat_message{
		{role: cstrs[0], content: cstrs[1]},
		{role: cstrs[2], content: cstrs[3]},
	}
	// First call sizes the output; the second fills it.
	need := C.llama_chat_apply_template(m.tmpl, &msgs[0], C.size_t(len(msgs)), C.bool(true), nil, 0)
	if need <= 0 {
		return "", fmt.Errorf("llama: chat template of model %s failed to render", m.path)
	}
	buf := make([]byte, int(need))
	n := C.llama_chat_apply_template(m.tmpl, &msgs[0], C.size_t(len(msgs)), C.bool(true),
		(*C.char)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)))
	if n <= 0 || int(n) > len(buf) {
		return "", fmt.Errorf("llama: chat template of model %s failed to render", m.path)
	}
	return string(buf[:n]), nil
}

func tokenize(vocab *C.struct_llama_vocab, text string) ([]C.llama_token, error) {
	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	// With a nil buffer the call returns the negated required count.
	n := C.llama_tokenize(vocab, ctext, C.int32_t(len(text)), nil, 0, true, true)
	if n >= 0 {
		return nil, fmt.Errorf("llama: tokenization produced no tokens")
	}
	toks := make([]C.llama_token, -int(n))
	if rc := C.llama_tokenize(vocab, ctext, C.int32_t(len(text)), &toks[0], C.int32_t(len(toks)), true, true); rc < 0 {
		return nil, fmt.Errorf("llama: tokenization failed")
	}
	return toks, nil
}

func tokenToPiece(vocab *C.struct_llama_vocab, tok C.llama_token) (string, error) {
	var buf [256]C.char
	n := C.llama_token_to_piece(vocab, tok, &buf[0], C.int32_t(len(buf)), 0, false)
	if n < 0 {
		return "", fmt.Errorf("llama: detokenization failed")
	}
	return C.GoStringN(&buf[0], int(n)), nil
}
```

- [ ] **Step 4: Compile and run hermetic tests**

Run: `go test ./internal/llama/ -v`
Expected: `TestLoadMissingFile` PASS; the two integration tests SKIP. Fix any cgo compile errors against the vendored `llama.h` (see the API-drift note above).

- [ ] **Step 5: Run the integration test with a real model**

Download any small instruct GGUF, e.g.:

```bash
curl -L -o /tmp/qwen-0.6b.gguf \
  "https://huggingface.co/ggml-org/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"
GOSAID_LLAMA_MODEL=/tmp/qwen-0.6b.gguf go test ./internal/llama/ -v -run TestChat
```

Expected: both tests PASS (on an Apple Silicon Mac, generation runs on Metal — the load log lines mention Metal). If the answer check flakes on the chosen model, adjust the prompt to be more constraining, not the assertion to be looser.

- [ ] **Step 6: Commit**

```bash
make fmt vet
git add internal/llama/llama.go internal/llama/llama_test.go
git commit -m "feat: cgo wrapper for llama.cpp single-turn chat"
```

---

### Task 4: Extract the generic model cache

Pure refactor: `WhisperCPP` behavior is pinned by its existing tests, which must pass unchanged in substance (they are updated only to reach fields through `d.cache`).

**Files:**
- Create: `internal/drivers/modelcache.go`
- Modify: `internal/drivers/whisper_cpp.go`, `internal/drivers/whisper_cpp_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (Task 6 depends on these):
  - `newModelCache[M any](paths map[string]string, unloadAfter time.Duration, load func(string) (M, error), close func(M), kind string) *modelCache[M]`
  - `(*modelCache[M]).acquire(name string) (*cacheEntry[M], error)` — entry's model is field `m`
  - `(*modelCache[M]).release(name string, e *cacheEntry[M])`
  - Exported-to-tests fields: `cache.mu`, `cache.loaded`, `cache.load`

- [ ] **Step 1: Write `internal/drivers/modelcache.go`**

This is the cache logic moved out of `whisper_cpp.go` and parameterized; behavior is identical.

```go
package drivers

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// cacheEntry is a cached loaded model with idle-unload bookkeeping. All
// fields are guarded by modelCache.mu; an entry is only removed from the
// cache (and its model closed) when inflight is zero, so a holder returned
// by acquire can never have the model freed under it.
type cacheEntry[M any] struct {
	m        M
	inflight int
	lastUse  time.Time
	timer    *time.Timer // armed while idle and unloading is enabled
}

// modelCache lazily loads models by name and keeps them resident. A failed
// load is not cached, so the next use retries. If unloadAfter is positive,
// a model idle for that long is freed and reloaded lazily on next use.
// Shared by the whisper_cpp and llama_cpp drivers.
type modelCache[M any] struct {
	mu          sync.Mutex
	paths       map[string]string // model name → file path (from config)
	loaded      map[string]*cacheEntry[M]
	unloadAfter time.Duration
	kind        string // driver type, for errors and logs
	load        func(path string) (M, error)
	close       func(M)
}

func newModelCache[M any](paths map[string]string, unloadAfter time.Duration,
	load func(string) (M, error), close func(M), kind string) *modelCache[M] {
	return &modelCache[M]{
		paths:       paths,
		loaded:      map[string]*cacheEntry[M]{},
		unloadAfter: unloadAfter,
		kind:        kind,
		load:        load,
		close:       close,
	}
}

// acquire returns the cached model for name, loading it if needed, with its
// in-flight count incremented. Callers must pair it with release.
func (c *modelCache[M]) acquire(name string) (*cacheEntry[M], error) {
	c.mu.Lock()
	if e, ok := c.loaded[name]; ok {
		e.inflight++
		c.mu.Unlock()
		return e, nil
	}
	p, ok := c.paths[name]
	c.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s: unknown model %q", c.kind, name)
	}

	abs, err := config.ExpandPath(p)
	if err != nil {
		return nil, err
	}
	// The load runs a potentially multi-second cgo call; it must not hold
	// c.mu, or a concurrent request for a different, already-cached model
	// would block on it for no reason.
	m, err := c.load(abs)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.loaded[name]; ok {
		// Another goroutine won the race and cached its instance first;
		// close our redundant one and use theirs.
		c.close(m)
		existing.inflight++
		return existing, nil
	}
	e := &cacheEntry[M]{m: m, inflight: 1}
	c.loaded[name] = e
	return e, nil
}

// release ends a use begun by acquire and, once the model is idle, arms the
// unload timer.
func (c *modelCache[M]) release(name string, e *cacheEntry[M]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.inflight--
	e.lastUse = time.Now()
	if c.unloadAfter <= 0 || e.inflight > 0 {
		return
	}
	if e.timer == nil {
		e.timer = time.AfterFunc(c.unloadAfter, func() { c.maybeUnload(name) })
	} else {
		e.timer.Reset(c.unloadAfter)
	}
}

// maybeUnload frees the named model if it has been idle for the configured
// duration. Fired by the entry's timer; if a use is in flight the unload is
// skipped and the next release re-arms the timer.
func (c *modelCache[M]) maybeUnload(name string) {
	c.mu.Lock()
	e, ok := c.loaded[name]
	if !ok || e.inflight > 0 {
		c.mu.Unlock()
		return
	}
	if idle := time.Since(e.lastUse); idle < c.unloadAfter {
		// Used again after this timer was armed; try again later.
		e.timer.Reset(c.unloadAfter - idle)
		c.mu.Unlock()
		return
	}
	delete(c.loaded, name)
	c.mu.Unlock()
	c.close(e.m)
	slog.Info("model unloaded after idle timeout", "driver", c.kind,
		"model", name, "unload_after", c.unloadAfter)
}
```

- [ ] **Step 2: Refactor `WhisperCPP` onto the cache**

In `internal/drivers/whisper_cpp.go`, delete `modelEntry`, `acquire`, `release`, `maybeUnload`, and the cache fields, and rewire (the `whisperModel` interface, `Transcribe`, `TranslateSpeech`, and `Chat` bodies are unchanged):

```go
// WhisperCPP implements Driver over locally-loaded whisper.cpp models.
type WhisperCPP struct {
	cache *modelCache[whisperModel]
}

func NewWhisperCPP(models map[string]string, unloadAfter time.Duration) *WhisperCPP {
	return &WhisperCPP{cache: newModelCache(models, unloadAfter,
		func(path string) (whisperModel, error) { return whisper.Load(path) },
		func(m whisperModel) { m.Close() },
		config.DriverWhisperCPP)}
}

func (w *WhisperCPP) run(ctx context.Context, samples []float32, sampleRate int,
	model string, opts whisper.Options) (whisper.Result, error) {
	if err := ctx.Err(); err != nil {
		return whisper.Result{}, err
	}
	e, err := w.cache.acquire(model)
	if err != nil {
		return whisper.Result{}, err
	}
	defer w.cache.release(model, e)
	if sampleRate != audio.CaptureSampleRate {
		samples = audio.ResampleLinear(samples, sampleRate, audio.CaptureSampleRate)
	}
	return e.m.Transcribe(samples, opts)
}
```

- [ ] **Step 3: Update the tests' field access**

In `internal/drivers/whisper_cpp_test.go`: replace `d.load = l.load` with `d.cache.load = l.load` (three occurrences), and in `TestWhisperCPPConcurrentModelAccess` replace `d.mu.Lock()` / `d.mu.Unlock()` / `len(d.loaded)` with `d.cache.mu.Lock()` / `d.cache.mu.Unlock()` / `len(d.cache.loaded)`. No assertion changes.

- [ ] **Step 4: Run the driver tests (with race detector)**

Run: `go test ./internal/drivers/ -race -v`
Expected: all PASS — same behavior, new plumbing.

- [ ] **Step 5: Commit**

```bash
make fmt vet
git add internal/drivers/
git commit -m "refactor: extract generic model cache from whisper driver"
```

---

### Task 5: Config — `llama_cpp` driver type and validation

**Files:**
- Modify: `internal/config/config.go`, `internal/config/validate.go`
- Test: `internal/config/validate_llama_test.go` (new file; leaves existing config tests untouched)

**Interfaces:**
- Consumes: existing `Config`/`EndpointConfig` structs.
- Produces: `config.DriverLlamaCPP = "llama_cpp"` (Tasks 6-7 reference it); validation rules per spec.

- [ ] **Step 1: Write the failing validation tests**

`internal/config/validate_llama_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// llamaTestConfig returns a valid config with an openai endpoint, a
// llama_cpp endpoint holding one real temp model file, and one hotkey
// using the llama model for enhance.
func llamaTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	model := filepath.Join(dir, "gemma.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Config{
		Version: CurrentVersion,
		Drivers: []Driver{
			{Driver: DriverOpenAICompatible, Endpoints: []Endpoint{{
				ID:     "openai",
				Config: EndpointConfig{APIBase: "https://api.openai.com/v1", APIKey: "sk-x"},
			}}},
			{Driver: DriverLlamaCPP, Endpoints: []Endpoint{{
				ID:     "local-llm",
				Config: EndpointConfig{Models: map[string]string{"gemma": model}, UnloadAfterSeconds: 300},
			}}},
		},
		Hotkeys: map[string]Hotkey{"ctrl+alt+space": {
			Transcribe: TranscribeStage{Model: "openai:whisper-1"},
			Enhance:    &EnhanceStage{Model: "local-llm:gemma"},
		}},
		ToggleMaxSeconds: 60,
	}
}

func TestValidateLlamaCPPHappyPath(t *testing.T) {
	if err := Validate(llamaTestConfig(t)); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidateLlamaCPPRequiresModels(t *testing.T) {
	cfg := llamaTestConfig(t)
	cfg.Drivers[1].Endpoints[0].Config.Models = nil
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "models") {
		t.Fatalf("expected missing-models error, got: %v", err)
	}
}

func TestValidateLlamaCPPModelFileMustExist(t *testing.T) {
	cfg := llamaTestConfig(t)
	cfg.Drivers[1].Endpoints[0].Config.Models["gemma"] = "/nonexistent/gemma.gguf"
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("expected file-not-found error, got: %v", err)
	}
}

func TestValidateLlamaCPPRejectsTranscribeRef(t *testing.T) {
	cfg := llamaTestConfig(t)
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Transcribe.Model = "local-llm:gemma"
	cfg.Hotkeys["ctrl+alt+space"] = hk
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "chat stages only") {
		t.Fatalf("expected chat-stages-only error, got: %v", err)
	}
}

func TestValidateLlamaCPPUnknownModelName(t *testing.T) {
	cfg := llamaTestConfig(t)
	hk := cfg.Hotkeys["ctrl+alt+space"]
	hk.Enhance = &EnhanceStage{Model: "local-llm:nope"}
	cfg.Hotkeys["ctrl+alt+space"] = hk
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "no model named") {
		t.Fatalf("expected no-model-named error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/config/ -run TestValidateLlama -v`
Expected: FAIL — `undefined: DriverLlamaCPP`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`:
- Add to the consts block: `DriverLlamaCPP = "llama_cpp"`.
- Update the `EndpointConfig` comments: the struct doc gains `llama_cpp needs models (name → GGUF model file path)`, and the `UnloadAfterSeconds` comment changes `(whisper_cpp only)` to `(whisper_cpp / llama_cpp only)`.

In `internal/config/validate.go`:

1. `endpointInfo` comment: `models map[string]string // whisper_cpp / llama_cpp only`.
2. Driver-type switch:

```go
switch d.Driver {
case DriverOpenAICompatible, DriverWhisperCPP, DriverLlamaCPP:
default:
	return fmt.Errorf("drivers[%d]: unknown driver type %q (expected %q, %q, or %q)",
		di, d.Driver, DriverOpenAICompatible, DriverWhisperCPP, DriverLlamaCPP)
}
```

3. Endpoint-config switch: change `case DriverWhisperCPP:` to `case DriverWhisperCPP, DriverLlamaCPP:` and inside it use the driver type in the message: `fmt.Errorf("endpoint %q: a non-empty models map is required for %s", e.ID, d.Driver)`. In the openai branch, the unload error becomes `"endpoint %q: unload_after_seconds only applies to whisper_cpp and llama_cpp endpoints"`.
4. `checkModelRef` — replace the `if info.driver == DriverWhisperCPP { ... }` block with:

```go
switch info.driver {
case DriverWhisperCPP:
	if chatStage {
		return fmt.Errorf("%s: endpoint %q is whisper_cpp, which supports transcription only", field, m.Endpoint)
	}
case DriverLlamaCPP:
	if !chatStage {
		return fmt.Errorf("%s: endpoint %q is llama_cpp, which supports chat stages only", field, m.Endpoint)
	}
}
if info.driver == DriverWhisperCPP || info.driver == DriverLlamaCPP {
	if _, ok := info.models[m.Model]; !ok {
		return fmt.Errorf("%s: endpoint %q has no model named %q", field, m.Endpoint, m.Model)
	}
}
```

- [ ] **Step 4: Run the config tests**

Run: `go test ./internal/config/ -v`
Expected: new tests PASS, all existing tests still PASS (the whisper messages they assert are unchanged except the openai unload message — if an existing test asserts its exact text, update that assertion to the new wording).

- [ ] **Step 5: Commit**

```bash
make fmt vet
git add internal/config/
git commit -m "feat: llama_cpp driver type in config and validation"
```

---

### Task 6: `LlamaCPP` driver and registry wiring

**Files:**
- Create: `internal/drivers/llama_cpp.go`
- Modify: `internal/drivers/registry.go`
- Test: `internal/drivers/llama_cpp_test.go`

**Interfaces:**
- Consumes: `llama.Load` / `Model.Chat` / `Model.Close` / `llama.Options` (Task 3); `modelCache` (Task 4); `config.DriverLlamaCPP` (Task 5).
- Produces: `NewLlamaCPP(models map[string]string, unloadAfter time.Duration) *LlamaCPP` implementing `Driver`; registry constructs it for `llama_cpp` endpoints.

- [ ] **Step 1: Write the failing tests**

`internal/drivers/llama_cpp_test.go`:

```go
package drivers

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/llama"
)

func TestLlamaCPPTranscribeUnsupported(t *testing.T) {
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	_, err := d.Transcribe(context.Background(), []float32{0}, 16000, "gemma", TranscribeOptions{})
	if err == nil || !strings.Contains(err.Error(), "do not support transcription") {
		t.Fatalf("expected transcription-unsupported error, got: %v", err)
	}
	_, err = d.TranslateSpeech(context.Background(), []float32{0}, 16000, "gemma", TranslateSpeechOptions{})
	if err == nil || !strings.Contains(err.Error(), "do not support transcription") {
		t.Fatalf("expected transcription-unsupported error, got: %v", err)
	}
}

func TestLlamaCPPChatUnknownModel(t *testing.T) {
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	_, err := d.Chat(context.Background(), "other", "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("expected unknown-model error, got: %v", err)
	}
}

// fakeLlamaModel stands in for a loaded llama.Model.
type fakeLlamaModel struct {
	mu     sync.Mutex
	closed bool
	calls  int
}

func (f *fakeLlamaModel) Chat(ctx context.Context, system, user string, opts llama.Options) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return "cleaned: " + user, nil
}

func (f *fakeLlamaModel) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func TestLlamaCPPChatUsesLoadedModel(t *testing.T) {
	fake := &fakeLlamaModel{}
	loads := 0
	d := NewLlamaCPP(map[string]string{"gemma": "/tmp/x.gguf"}, 0)
	d.cache.load = func(path string) (llamaModel, error) { loads++; return fake, nil }

	out, err := d.Chat(context.Background(), "gemma", "sys", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "cleaned: hello" {
		t.Fatalf("unexpected chat output %q", out)
	}
	if _, err := d.Chat(context.Background(), "gemma", "sys", "again"); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatalf("expected model to load once and stay resident, got %d loads", loads)
	}
}

func TestBuildRegistryLlamaCPP(t *testing.T) {
	cfg := &config.Config{Drivers: []config.Driver{{
		Driver: config.DriverLlamaCPP,
		Endpoints: []config.Endpoint{{
			ID:     "local-llm",
			Config: config.EndpointConfig{Models: map[string]string{"gemma": "/tmp/x.gguf"}},
		}},
	}}}
	r, err := BuildRegistry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Endpoint("local-llm")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.(*LlamaCPP); !ok {
		t.Fatalf("expected *LlamaCPP, got %T", d)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/drivers/ -run TestLlama -v`
Expected: FAIL — `undefined: NewLlamaCPP`.

- [ ] **Step 3: Implement the driver**

`internal/drivers/llama_cpp.go`:

```go
package drivers

import (
	"context"
	"fmt"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/llama"
)

// llamaModel is the loaded-model surface LlamaCPP needs; *llama.Model
// satisfies it, tests substitute fakes via the cache's load hook.
type llamaModel interface {
	Chat(ctx context.Context, system, user string, opts llama.Options) (string, error)
	Close()
}

// LlamaCPP implements Driver over locally-loaded llama.cpp GGUF models.
// It serves chat stages only; transcription refs are rejected at config
// validation time.
type LlamaCPP struct {
	cache *modelCache[llamaModel]
}

func NewLlamaCPP(models map[string]string, unloadAfter time.Duration) *LlamaCPP {
	return &LlamaCPP{cache: newModelCache(models, unloadAfter,
		func(path string) (llamaModel, error) { return llama.Load(path) },
		func(m llamaModel) { m.Close() },
		config.DriverLlamaCPP)}
}

func (l *LlamaCPP) Chat(ctx context.Context, model, system, user string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	e, err := l.cache.acquire(model)
	if err != nil {
		return "", err
	}
	defer l.cache.release(model, e)
	return e.m.Chat(ctx, system, user, llama.Options{})
}

// Transcribe is a backstop: config validation already rejects
// transcribe-stage refs to llama_cpp endpoints.
func (l *LlamaCPP) Transcribe(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranscribeOptions) (TranscribeResult, error) {
	return TranscribeResult{}, fmt.Errorf("llama_cpp endpoints do not support transcription")
}

// TranslateSpeech is a backstop, as Transcribe.
func (l *LlamaCPP) TranslateSpeech(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranslateSpeechOptions) (string, error) {
	return "", fmt.Errorf("llama_cpp endpoints do not support transcription")
}
```

In `internal/drivers/registry.go`, add to the switch (after the `DriverWhisperCPP` case):

```go
case config.DriverLlamaCPP:
	r.endpoints[e.ID] = NewLlamaCPP(e.Config.Models,
		time.Duration(e.Config.UnloadAfterSeconds)*time.Second)
```

Also update the `Driver` interface comment in `internal/drivers/interfaces.go`: the sentence "Only `openai_compatible` is implemented" is stale — replace with "Implemented by openai_compatible (all methods), whisper_cpp (speech only), and llama_cpp (chat only)."

- [ ] **Step 4: Run the driver tests**

Run: `go test ./internal/drivers/ -race -v`
Expected: all PASS (new llama tests and existing whisper/cache tests).

- [ ] **Step 5: Commit**

```bash
make fmt vet
git add internal/drivers/
git commit -m "feat: llama_cpp driver for local chat stages"
```

---

### Task 7: `gosaid model download` learns `.gguf`

**Files:**
- Modify: `internal/cli/model.go`
- Test: `internal/cli/model_test.go`

**Interfaces:**
- Consumes: `config.DriverLlamaCPP` (Task 5).
- Produces: user-facing behavior only. Internal helpers (exact signatures used below): `downloadDefaults(file string) (driver, endpointID string)`, `deriveModelName(file string) string`, `modelDownloadOpts` gains field `driver string`, `otherDriverForEndpoint(cfg, endpointID, ownDriver string) string`, `findLocalModel(cfg, driver, endpointID, name string) string`, `registerModel(cfg, driver, endpointID, name, path string)`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/model_test.go`:

```go
func TestDeriveModelNameGGUF(t *testing.T) {
	cases := map[string]string{
		"gemma-3-4b-it-Q4_K_M.gguf":       "gemma-3-4b-it",
		"qwen2.5-0.5b-instruct-q4_0.gguf": "qwen2.5-0.5b-instruct",
		"Llama-3.2-1B-Instruct-F16.gguf":  "Llama-3.2-1B-Instruct",
		"plain.gguf":                      "plain",
	}
	for in, want := range cases {
		if got := deriveModelName(in); got != want {
			t.Errorf("deriveModelName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadDefaults(t *testing.T) {
	if d, e := downloadDefaults("ggml-base.bin"); d != config.DriverWhisperCPP || e != "local" {
		t.Fatalf("bin defaults = %q/%q", d, e)
	}
	if d, e := downloadDefaults("gemma-3-4b-it-Q4_K_M.gguf"); d != config.DriverLlamaCPP || e != "local-llm" {
		t.Fatalf("gguf defaults = %q/%q", d, e)
	}
}

func TestModelDownloadGGUF(t *testing.T) {
	opts, cfgPath := downloadEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggml-org/gemma-3-4b-it-GGUF/resolve/main/gemma-3-4b-it-Q4_K_M.gguf" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("FAKE-GGUF-BYTES"))
	}))
	opts.repo, opts.file = "ggml-org/gemma-3-4b-it-GGUF", "gemma-3-4b-it-Q4_K_M.gguf"
	opts.name, opts.endpointID = "gemma", "local-llm"
	opts.driver = config.DriverLlamaCPP
	if err := modelDownload(opts); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, d := range cfg.Drivers {
		if d.Driver != config.DriverLlamaCPP {
			continue
		}
		for _, e := range d.Endpoints {
			if e.ID == "local-llm" {
				found = e.Config.Models["gemma"]
			}
		}
	}
	if found == "" || !strings.HasSuffix(found, "gemma-3-4b-it-Q4_K_M.gguf") {
		t.Fatalf("llama_cpp endpoint not registered, got path %q", found)
	}
}
```

And in `downloadEnv`, set the new field on the returned opts: `driver: config.DriverWhisperCPP`.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/cli/ -run 'TestDeriveModelNameGGUF|TestDownloadDefaults|TestModelDownloadGGUF' -v`
Expected: FAIL — `undefined: downloadDefaults` (and missing `driver` field).

- [ ] **Step 3: Implement**

In `internal/cli/model.go`:

1. Name derivation (replace `deriveModelName`; add the regexp import):

```go
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
```

2. In `RunModel`: change the flag default to `endpoint := fs.String("endpoint", "", "endpoint id to register under (default: local for whisper models, local-llm for .gguf chat models)")`. After the positionals are settled, resolve driver and endpoint and pass them through:

```go
driver, defaultEndpoint := downloadDefaults(rest[1])
if *endpoint == "" {
	*endpoint = defaultEndpoint
}
```

and add `driver: driver` to the `modelDownloadOpts` literal.

3. `modelDownloadOpts` gains `driver string` (a `config.Driver*` constant).

4. Generalize the helpers — `otherDriverForEndpoint(cfg, o.endpointID, o.driver)` skips endpoints whose driver IS `o.driver` instead of hardcoding whisper; `findWhisperModel` is renamed `findLocalModel(cfg, o.driver, o.endpointID, o.name)` and matches `d.Driver == driver`; `registerModel(cfg, o.driver, o.endpointID, o.name, dest)` creates/extends the driver block for `driver` instead of hardcoded `DriverWhisperCPP`. Update `modelUsage` is not needed (flags unchanged).

5. Success message: pick the snippet stage by driver —

```go
stage := "transcribe"
if o.driver == config.DriverLlamaCPP {
	stage = "enhance"
}
fmt.Printf("downloaded %s (%.1f MB)\nregistered model %q on endpoint %q\n\nuse it in a hotkey:\n  \"%s\": { \"model\": \"%s:%s\" }\n",
	dest, float64(size)/(1<<20), o.name, o.endpointID, stage, o.endpointID, o.name)
```

- [ ] **Step 4: Run the CLI tests**

Run: `go test ./internal/cli/ -v`
Expected: all PASS, including the pre-existing whisper download tests (they now go through the generalized helpers with `driver: config.DriverWhisperCPP`).

- [ ] **Step 5: Commit**

```bash
make fmt vet
git add internal/cli/
git commit -m "feat: model download registers .gguf chat models under llama_cpp"
```

---

### Task 8: Example config, README, and end-to-end verification

**Files:**
- Modify: `internal/config/config.example.json`, `README.md`

**Interfaces:**
- Consumes: everything above.
- Produces: user-facing docs; final verified build.

- [ ] **Step 1: Extend `config.example.json`**

Add a `llama_cpp` driver block after the `openai_compatible` one:

```json
{
  "driver": "llama_cpp",
  "endpoints": [
    {
      "id": "local-llm",
      "config": {
        "models": {
          "gemma": "~/Library/Application Support/gosaid/models/gemma-3-4b-it-Q4_K_M.gguf"
        },
        "unload_after_seconds": 300
      }
    }
  ]
}
```

Verify the example still parses and validates structurally: `go test ./internal/config/ -v` (if a test loads the example file it must stay green; the model path not existing is fine for parse-only tests — if a validation test trips on it, keep the block but note that in the test's fixture instead).

- [ ] **Step 2: Update the README**

Rename the section "Local transcription (no cloud)" to "Local models (no cloud)". Keep the existing whisper content, then replace the paragraph "Local models cover **transcription only** — `enhance`, `compose`, and `translate` need an OpenAI-compatible endpoint (cloud, or a local server like Ollama)." with a new subsection:

```markdown
#### Local chat models (enhance / compose / translate)

The text stages can also run fully locally via embedded llama.cpp. Download
any instruct-tuned GGUF model — it registers as the `local-llm` endpoint:

    gosaid model download ggml-org/gemma-3-4b-it-GGUF gemma-3-4b-it-Q4_K_M.gguf --name gemma

Then use it in a hotkey's chat stages:

    "cmd+shift+r": {
      "mode": "hold",
      "transcribe": { "model": "local:turbo" },
      "enhance":    { "model": "local-llm:gemma" },
      "translate":  { "model": "local-llm:gemma", "output_language": "en" }
    }

| Model | Size | When to use |
|---|---|---|
| `gemma-3-4b-it-Q4_K_M.gguf` (repo `ggml-org/gemma-3-4b-it-GGUF`) | ~2.5 GB | Recommended default — strong cleanup and translation quality |
| `Qwen3-1.7B-Q4_K_M.gguf` (repo `ggml-org/Qwen3-1.7B-GGUF`) | ~1.1 GB | Budget option: less RAM, faster loads, weaker compose quality |

Chat models follow the same residency rules as whisper models: loaded
lazily on first use, kept in memory for instant dictation, and — with
`unload_after_seconds` set on the endpoint — freed after idling. Budget
RAM for every model that can be resident at once (e.g. whisper turbo
~550 MB + a 4B chat model ~2.5 GB); mixing a small enhance model with a
larger compose model is fine, but each loads separately.

Models must ship an embedded chat template (instruct builds do); base
models are rejected at load time.
```

Also update the models table caption if the surrounding text still says local support is transcription-only anywhere else in the README (search for "transcription only").

- [ ] **Step 3: End-to-end verification (macOS)**

```bash
make build && make test
```

Expected: clean build, all tests pass. Then the real-flow check (uses the small model from Task 3 Step 5, or download a listed one):

1. Register a chat model: `./gosaid model download ggml-org/Qwen3-0.6B-GGUF Qwen3-0.6B-Q8_0.gguf --name tiny` — expect the success message with an `"enhance"` snippet and a `llama_cpp` block in `config.json`.
2. Add `"enhance": { "model": "local-llm:tiny" }` to a hotkey, run `./gosaid` in the foreground, dictate a sentence with a few "um"s into a text field.
3. Expect: cleaned text typed at the cursor; daemon log shows the model load on first press and (if `unload_after_seconds` is set low for the test) an idle-unload line afterwards.

- [ ] **Step 4: Commit**

```bash
make fmt vet
git add internal/config/config.example.json README.md
git commit -m "docs: local chat models via embedded llama.cpp"
```

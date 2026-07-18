// Package whisper wraps the vendored whisper.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package.
//
// The vendored C/C++ sources live under internal/whisper/cvendor/ (not
// "vendor/": the Go toolchain reserves that name for module vendoring and
// refuses to import packages whose path contains a "vendor" element).
// Architecture-specific CPU backend sources are linked via link_amd64.go /
// link_arm64.go; the Metal backend via link_darwin.go.
package whisper

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/cvendor/ggml/src"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/cvendor/ggml/src/ggml-cpu"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/cvendor/src"
)

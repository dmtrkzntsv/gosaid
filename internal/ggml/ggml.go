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

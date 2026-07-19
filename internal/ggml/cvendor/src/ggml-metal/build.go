//go:build darwin

// Package ggmlmetal compiles the vendored ggml Metal backend (darwin only).
package ggmlmetal

// #cgo CPPFLAGS: -I${SRCDIR}/../../include -I${SRCDIR}/.. -I${SRCDIR} -DNDEBUG -DGGML_USE_METAL -DGGML_METAL_EMBED_LIBRARY
// #cgo CFLAGS: -O3 -fPIC
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC
// #cgo LDFLAGS: -framework Metal -framework MetalKit -framework Foundation
import "C"

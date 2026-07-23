//go:build arm64

// Package ggmlcpuarm compiles the vendored ggml CPU backend arm arch sources.
package ggmlcpuarm

// #cgo CPPFLAGS: -I${SRCDIR}/../../../../include -I${SRCDIR}/../../.. -I${SRCDIR}/../.. -I${SRCDIR} -DNDEBUG -DGGML_USE_CPU
// #cgo CFLAGS: -O3 -std=c11 -fPIC -pthread
// #cgo CXXFLAGS: -O3 -std=c++17 -fPIC -pthread
// #cgo linux CPPFLAGS: -D_XOPEN_SOURCE=600 -D_GNU_SOURCE
// #cgo darwin CPPFLAGS: -D_XOPEN_SOURCE=600 -D_DARWIN_C_SOURCE -DGGML_USE_ACCELERATE -DACCELERATE_NEW_LAPACK -DACCELERATE_LAPACK_ILP64
import "C"

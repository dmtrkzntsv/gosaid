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

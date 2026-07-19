package whisper

/*
#cgo CPPFLAGS: -I${SRCDIR}/cvendor/include -I${SRCDIR}/../ggml/cvendor/include
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

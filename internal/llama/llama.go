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
// between calls. ctx is checked between decode steps, so cancellation stops
// the in-flight generation promptly. Chat holds the model's mutex for the
// whole generation, so a call queued behind another generation on the same
// model waits for the mutex first and cannot observe ctx cancellation until
// then.
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
	// temperature keeps compose phrasing natural without drifting. temp is
	// applied first to sharpen the distribution, then min_p truncates the
	// low-probability tail of that sharpened distribution, and dist samples
	// from what remains — matching upstream's documented chain convention.
	smpl := C.llama_sampler_chain_init(C.llama_sampler_chain_default_params())
	defer C.llama_sampler_free(smpl)
	C.llama_sampler_chain_add(smpl, C.llama_sampler_init_temp(0.2))
	C.llama_sampler_chain_add(smpl, C.llama_sampler_init_min_p(0.05, 1))
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
	return C.GoStringN(&buf[0], C.int(n)), nil
}

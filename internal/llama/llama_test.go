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

	// The prompt is deliberately constraining: some small instruct models
	// (e.g. Qwen3) emit a <think>...</think> reasoning block by default,
	// which can burn the token budget before the answer appears. /no_think
	// suppresses that, and phrasing the ask as an imperative rather than a
	// question keeps the one-word answer reliable across samples.
	out, err := m.Chat(context.Background(),
		"You are a geography quiz bot. Always answer with exactly one word: the correct answer. /no_think",
		"Name the capital city of France. /no_think",
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

package inject

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type concurrentClipboard struct {
	mu   sync.Mutex
	text []byte
}

func (c *concurrentClipboard) ReadText() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.text...)
}

func (c *concurrentClipboard) ReadImage() []byte { return nil }

func (c *concurrentClipboard) WriteText(text []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.text = append([]byte(nil), text...)
}

func (c *concurrentClipboard) WriteImage([]byte) {}

func TestInjectPasteKeepsTextAvailableForAsyncConsumer(t *testing.T) {
	clip := &concurrentClipboard{text: []byte("previous clipboard")}
	consumed := make(chan string, 1)

	err := injectPaste(context.Background(), clip, func() error {
		go func() {
			time.Sleep(20 * time.Millisecond)
			consumed <- string(clip.ReadText())
		}()
		return nil
	}, "generated result", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("injectPaste: %v", err)
	}

	if got := <-consumed; got != "generated result" {
		t.Fatalf("async paste consumed %q, want generated result", got)
	}
	if got := string(clip.ReadText()); got != "previous clipboard" {
		t.Fatalf("clipboard = %q after paste, want previous contents restored", got)
	}
}

func TestInjectPasteDoesNotOverwriteNewClipboardContents(t *testing.T) {
	clip := &concurrentClipboard{text: []byte("previous clipboard")}

	err := injectPaste(context.Background(), clip, func() error {
		go func() {
			time.Sleep(10 * time.Millisecond)
			clip.WriteText([]byte("new user copy"))
		}()
		return nil
	}, "generated result", 30*time.Millisecond)
	if err != nil {
		t.Fatalf("injectPaste: %v", err)
	}

	if got := string(clip.ReadText()); got != "new user copy" {
		t.Fatalf("clipboard = %q, want newer contents preserved", got)
	}
}

func TestInjectPasteCancellationLeavesGeneratedText(t *testing.T) {
	clip := &concurrentClipboard{text: []byte("previous clipboard")}
	ctx, cancel := context.WithCancel(context.Background())

	err := injectPaste(ctx, clip, func() error {
		cancel()
		return nil
	}, "generated result", time.Second)
	if err != nil {
		t.Fatalf("injectPaste: %v", err)
	}

	if got := string(clip.ReadText()); got != "generated result" {
		t.Fatalf("clipboard = %q, want generated result for late paste", got)
	}
}

func TestInjectPasteSynthesisFailureLeavesGeneratedText(t *testing.T) {
	clip := &concurrentClipboard{text: []byte("previous clipboard")}
	synthErr := errors.New("no accessibility permission")

	err := injectPaste(context.Background(), clip, func() error {
		return synthErr
	}, "generated result", time.Millisecond)

	var injectionErr *InjectionFailedError
	if !errors.As(err, &injectionErr) {
		t.Fatalf("error = %v, want InjectionFailedError", err)
	}
	if !errors.Is(err, synthErr) {
		t.Fatalf("error = %v, want wrapped synthesis error", err)
	}
	if got := string(clip.ReadText()); got != "generated result" {
		t.Fatalf("clipboard = %q, want generated result for manual paste", got)
	}
}

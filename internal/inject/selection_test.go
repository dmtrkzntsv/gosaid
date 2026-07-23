package inject

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClipboard mimics an OS clipboard that holds either text or an image.
type fakeClipboard struct {
	text  []byte
	image []byte
}

func (f *fakeClipboard) ReadText() []byte    { return f.text }
func (f *fakeClipboard) ReadImage() []byte   { return f.image }
func (f *fakeClipboard) WriteText(b []byte)  { f.text = b; f.image = nil }
func (f *fakeClipboard) WriteImage(b []byte) { f.image = b; f.text = nil }

const (
	testTimeout  = 50 * time.Millisecond
	testInterval = 5 * time.Millisecond
)

func TestCaptureSelectionSuccess(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	synth := func() error {
		clip.WriteText([]byte("hello world")) // app copies the selection
		return nil
	}
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if !res.OK || res.Text != "hello world" {
		t.Fatalf("want OK with %q, got ok=%v text=%q", "hello world", res.OK, res.Text)
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionNoSelection(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	synth := func() error { return nil } // app ignores the copy
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.Err != nil || res.OK {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionSynthError(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	boom := errors.New("no accessibility permission")
	res := captureSelection(context.Background(), clip, func() error { return boom }, testTimeout, testInterval)
	if !errors.Is(res.Err, boom) {
		t.Fatalf("want synth error, got %v", res.Err)
	}
	if res.OK {
		t.Fatal("OK must be false on error")
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

func TestCaptureSelectionRestoresImage(t *testing.T) {
	clip := &fakeClipboard{image: []byte{0x89, 0x50, 0x4E, 0x47}} // png-ish bytes
	synth := func() error { return nil }
	res := captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if res.OK || res.Err != nil {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if !bytes.Equal(clip.image, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Fatalf("image clipboard not restored: %v", clip.image)
	}
}

func TestCaptureSelectionEmptyClipboardRestoredEmpty(t *testing.T) {
	clip := &fakeClipboard{} // nothing on the clipboard
	synth := func() error { return nil }
	_ = captureSelection(context.Background(), clip, synth, testTimeout, testInterval)
	if len(clip.text) != 0 || len(clip.image) != 0 {
		t.Fatalf("sentinel leaked: text=%q image=%v", clip.text, clip.image)
	}
}

func TestCaptureSelectionContextCancel(t *testing.T) {
	clip := &fakeClipboard{text: []byte("before")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	res := captureSelection(ctx, clip, func() error { return nil }, 10*time.Second, testInterval)
	if res.OK || res.Err != nil {
		t.Fatalf("want ok=false err=nil, got ok=%v err=%v", res.OK, res.Err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancelled capture did not return promptly")
	}
	if !bytes.Equal(clip.text, []byte("before")) {
		t.Fatalf("clipboard not restored: %q", clip.text)
	}
}

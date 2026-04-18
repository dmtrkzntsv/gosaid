package inject

import (
	"context"
	"fmt"
)

// Injector places text at the current input focus. The concrete
// implementation is platform-specific — this package only defines the
// contract and the shared error type.
type Injector interface {
	Inject(ctx context.Context, text string) error
}

// InjectionFailedError is returned when the OS-level paste synthesis fails.
// TextInClipboard signals that the user can still recover the text by
// pressing Cmd/Ctrl+V manually.
type InjectionFailedError struct {
	TextInClipboard bool
	Underlying      error
}

func (e *InjectionFailedError) Error() string {
	if e.Underlying == nil {
		return "injection failed"
	}
	return fmt.Sprintf("injection failed: %v", e.Underlying)
}

func (e *InjectionFailedError) Unwrap() error { return e.Underlying }

// Stub is a no-op injector used by early build steps and tests. It prints
// the text to stdout and succeeds. Replaced by platform injectors in Step 8.
type Stub struct{ Writer func(string) }

func (s Stub) Inject(_ context.Context, text string) error {
	if s.Writer != nil {
		s.Writer(text)
	}
	return nil
}

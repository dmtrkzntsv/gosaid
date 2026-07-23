// Package whisper wraps the vendored whisper.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package;
// ggml itself is linked via the shared internal/ggml package.
package whisper

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml"
	_ "github.com/dmtrkzntsv/gosaid/internal/whisper/cvendor/src"
)

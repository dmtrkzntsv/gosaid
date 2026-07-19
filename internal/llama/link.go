// Package llama wraps the vendored llama.cpp library. The blank imports
// pull the vendored C objects into any binary that imports this package;
// ggml itself is linked via the shared internal/ggml package.
package llama

import (
	_ "github.com/dmtrkzntsv/gosaid/internal/ggml"
	_ "github.com/dmtrkzntsv/gosaid/internal/llama/cvendor/src"
	_ "github.com/dmtrkzntsv/gosaid/internal/llama/cvendor/src/models"
)

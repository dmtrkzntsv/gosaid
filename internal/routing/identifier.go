package routing

import (
	"errors"
	"strings"
)

// ModelRef is an "endpoint_id:model_name" pair.
type ModelRef struct {
	Endpoint string
	Model    string
}

var ErrInvalidModelRef = errors.New("model reference must be in the form 'endpoint:model'")

// ParseModelRef splits on the first colon. Both halves must be non-empty.
func ParseModelRef(s string) (ModelRef, error) {
	i := strings.IndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return ModelRef{}, ErrInvalidModelRef
	}
	return ModelRef{Endpoint: s[:i], Model: s[i+1:]}, nil
}

package models

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	// CatalogRepo is the official whisper.cpp GGML model repository on
	// Hugging Face; every curated catalog entry downloads from it.
	CatalogRepo     = "ggerganov/whisper.cpp"
	HuggingFaceBase = "https://huggingface.co"
)

// CatalogEntry is one curated local Whisper model. Size and Note are human
// labels shown in the picker, not used programmatically.
type CatalogEntry struct {
	Name string // registered model name, referenced as "local:<name>"
	File string // file inside CatalogRepo ("ggml-base.bin")
	Size string // approximate download size ("~148 MB")
	Note string // one-phrase guidance ("fast on plain CPU")
}

// Catalog is the curated short list offered by `gosaid setup model`. It is
// deliberately small — three models that cover the useful range — with
// anything else reachable through the custom-model prompt.
//
// The turbo entry is the quantized q5_0 build: the README's default
// recommendation, roughly a third the size of the un-quantized file at
// comparable quality.
var Catalog = []CatalogEntry{
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB", Note: "fast on plain CPU"},
	{Name: "turbo", File: "ggml-large-v3-turbo-q5_0.bin", Size: "~550 MB", Note: "recommended — best accuracy/latency balance"},
	{Name: "large-v3", File: "ggml-large-v3.bin", Size: "~3.1 GB", Note: "highest accuracy, slowest"},
}

// ParseHuggingFaceURL extracts the repo and file from a Hugging Face model
// link, so users can paste what they see in the browser instead of splitting
// it themselves. It accepts the "resolve" and "blob" URL shapes:
//
//	https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin
//	https://huggingface.co/ggerganov/whisper.cpp/blob/main/ggml-small.bin
//
// and the bare "owner/repo/file" form. Query strings (?download=true) are
// ignored.
func ParseHuggingFaceURL(s string) (repo, file string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("a Hugging Face link is required")
	}
	path := s
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil {
			return "", "", fmt.Errorf("not a valid URL: %w", perr)
		}
		if !strings.EqualFold(u.Host, "huggingface.co") && !strings.EqualFold(u.Host, "www.huggingface.co") {
			return "", "", fmt.Errorf("only huggingface.co links are supported, got %q", u.Host)
		}
		path = u.Path
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// owner/repo/{resolve,blob}/<revision>/<file...> → drop the middle two.
	if len(parts) >= 5 && (parts[2] == "resolve" || parts[2] == "blob") {
		repo = parts[0] + "/" + parts[1]
		file = strings.Join(parts[4:], "/")
	} else if len(parts) >= 3 {
		repo = parts[0] + "/" + parts[1]
		file = strings.Join(parts[2:], "/")
	} else {
		return "", "", fmt.Errorf("expected a link like https://huggingface.co/owner/repo/resolve/main/model.bin")
	}
	if repo == "" || file == "" {
		return "", "", fmt.Errorf("could not find both a repository and a file in %q", s)
	}
	if !strings.HasSuffix(file, ".bin") {
		return "", "", fmt.Errorf("%q is not a GGML .bin model file", file)
	}
	return repo, file, nil
}

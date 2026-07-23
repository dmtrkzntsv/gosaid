package models

const (
	// CatalogRepo is the official whisper.cpp GGML model repository on
	// Hugging Face; every curated catalog entry downloads from it.
	CatalogRepo     = "ggerganov/whisper.cpp"
	HuggingFaceBase = "https://huggingface.co"
)

// CatalogEntry is one curated local model. Size and Note are human labels
// shown in the picker. Repo is the Hugging Face repo the file lives in; an
// empty Repo falls back to CatalogRepo (the shared whisper.cpp repo). Kind is
// "whisper" (transcription, .bin) or "chat" (llama_cpp, .gguf).
type CatalogEntry struct {
	Name string // registered model name, referenced as "endpoint:name"
	Repo string // HF repo; "" → CatalogRepo
	File string // file inside the repo
	Size string // approximate download size ("~148 MB")
	Note string // one-phrase guidance ("fast on plain CPU")
	Kind string // "whisper" or "chat"
}

// CatalogRepoFor returns the Hugging Face repo an entry downloads from,
// defaulting to the shared whisper.cpp repo.
func CatalogRepoFor(e CatalogEntry) string {
	if e.Repo != "" {
		return e.Repo
	}
	return CatalogRepo
}

// Catalog is the curated set offered by `gosaid setup`: two Whisper
// transcription models and two llama.cpp chat models, all from ggml-org (or
// the shared whisper.cpp repo), so quantizations track llama.cpp's format.
var Catalog = []CatalogEntry{
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB", Note: "fast on plain CPU", Kind: "whisper"},
	{Name: "turbo", File: "ggml-large-v3-turbo-q5_0.bin", Size: "~550 MB", Note: "recommended — best accuracy/latency balance", Kind: "whisper"},
	{Name: "qwen3.5-0.8b", Repo: "ggml-org/Qwen3.5-0.8B-GGUF", File: "Qwen3.5-0.8B-Q4_0.gguf", Size: "~563 MB", Note: "fast, light on RAM", Kind: "chat"},
	{Name: "gemma-4-e2b", Repo: "ggml-org/gemma-4-E2B-it-GGUF", File: "gemma-4-E2B-it-Q4_0.gguf", Size: "~2.8 GB", Note: "better quality", Kind: "chat"},
}

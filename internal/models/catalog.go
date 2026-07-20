package models

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
// deliberately small — the two models that suit most people — with everything
// else (large-v3, the .en variants, other quantizations) reachable through
// the custom-model prompt.
//
// The turbo entry is the quantized q5_0 build: the README's default
// recommendation, roughly a third the size of the un-quantized file at
// comparable quality.
var Catalog = []CatalogEntry{
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB", Note: "fast on plain CPU"},
	{Name: "turbo", File: "ggml-large-v3-turbo-q5_0.bin", Size: "~550 MB", Note: "recommended — best accuracy/latency balance"},
}

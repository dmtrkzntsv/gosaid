package models

const (
	// CatalogRepo is the official whisper.cpp GGML model repository on
	// Hugging Face; every curated catalog entry downloads from it.
	CatalogRepo     = "ggerganov/whisper.cpp"
	HuggingFaceBase = "https://huggingface.co"
)

// CatalogEntry is one curated local Whisper model. Size is a human label
// shown in the picker, not used programmatically.
type CatalogEntry struct {
	Name string // registered model name ("base") and DeriveName(File)
	File string // file inside CatalogRepo ("ggml-base.bin")
	Size string // approximate download size ("~148 MB")
}

var Catalog = []CatalogEntry{
	{Name: "tiny", File: "ggml-tiny.bin", Size: "~78 MB"},
	{Name: "base", File: "ggml-base.bin", Size: "~148 MB"},
	{Name: "small", File: "ggml-small.bin", Size: "~488 MB"},
	{Name: "medium", File: "ggml-medium.bin", Size: "~1.5 GB"},
	{Name: "large-v3", File: "ggml-large-v3.bin", Size: "~3.1 GB"},
	{Name: "large-v3-turbo", File: "ggml-large-v3-turbo.bin", Size: "~1.6 GB"},
}

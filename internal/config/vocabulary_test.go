package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVocabularyAddRemoveContains(t *testing.T) {
	v := &Vocabulary{}
	if !v.Add("Kubernetes") {
		t.Fatal("Add returned false for a new word")
	}
	if !v.Contains("kubernetes") {
		t.Fatal("Contains should match case-insensitively")
	}
	if v.Add("KUBERNETES") {
		t.Fatal("Add should reject a case-insensitive duplicate")
	}
	if len(v.Words) != 1 {
		t.Fatalf("Words = %v, want one entry", v.Words)
	}
	if !v.Remove("kubernetes") {
		t.Fatal("Remove should match case-insensitively")
	}
	if len(v.Words) != 0 {
		t.Fatalf("Words = %v, want empty", v.Words)
	}
	if v.Remove("kubernetes") {
		t.Fatal("Remove should report no change when absent")
	}
}

func TestVocabularyAddTrimsAndSkipsBlank(t *testing.T) {
	v := &Vocabulary{}
	if !v.Add("  gosaid  ") {
		t.Fatal("Add should accept a padded word")
	}
	if v.Words[0] != "gosaid" {
		t.Fatalf("stored %q, want trimmed", v.Words[0])
	}
	if v.Add("   ") {
		t.Fatal("Add should skip a blank word")
	}
}

func TestVocabularyPrompt(t *testing.T) {
	v := &Vocabulary{Words: []string{"PostHog", "gosaid"}}
	if got, want := v.Prompt(), "PostHog, gosaid"; got != want {
		t.Fatalf("Prompt() = %q, want %q", got, want)
	}
	if got := (&Vocabulary{}).Prompt(); got != "" {
		t.Fatalf("empty Prompt() = %q, want empty", got)
	}
}

func TestLoadVocabularyMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary.json")
	v, err := LoadVocabulary(path)
	if err != nil {
		t.Fatalf("LoadVocabulary on missing file: %v", err)
	}
	if len(v.Words) != 0 {
		t.Fatalf("Words = %v, want empty", v.Words)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary.json")
	in := &Vocabulary{Words: []string{"Anthropic", "Claude"}}
	if err := SaveVocabulary(path, in); err != nil {
		t.Fatalf("SaveVocabulary: %v", err)
	}
	out, err := LoadVocabulary(path)
	if err != nil {
		t.Fatalf("LoadVocabulary: %v", err)
	}
	if len(out.Words) != 2 || out.Words[0] != "Anthropic" || out.Words[1] != "Claude" {
		t.Fatalf("round trip = %v", out.Words)
	}
}

func TestLoadVocabularyEmptyFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := LoadVocabulary(path)
	if err != nil {
		t.Fatalf("LoadVocabulary on empty file: %v", err)
	}
	if len(v.Words) != 0 {
		t.Fatalf("Words = %v, want empty", v.Words)
	}
}

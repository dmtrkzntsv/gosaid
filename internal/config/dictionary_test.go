package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDictionaryAddRemoveContains(t *testing.T) {
	d := &Dictionary{}
	if !d.Add("Kubernetes") {
		t.Fatal("Add returned false for a new word")
	}
	if !d.Contains("kubernetes") {
		t.Fatal("Contains should match case-insensitively")
	}
	if d.Add("KUBERNETES") {
		t.Fatal("Add should reject a case-insensitive duplicate")
	}
	if len(d.Words) != 1 {
		t.Fatalf("Words = %v, want one entry", d.Words)
	}
	if !d.Remove("kubernetes") {
		t.Fatal("Remove should match case-insensitively")
	}
	if len(d.Words) != 0 {
		t.Fatalf("Words = %v, want empty", d.Words)
	}
	if d.Remove("kubernetes") {
		t.Fatal("Remove should report no change when absent")
	}
}

func TestDictionaryAddTrimsAndSkipsBlank(t *testing.T) {
	d := &Dictionary{}
	if !d.Add("  gosaid  ") {
		t.Fatal("Add should accept a padded word")
	}
	if d.Words[0] != "gosaid" {
		t.Fatalf("stored %q, want trimmed", d.Words[0])
	}
	if d.Add("   ") {
		t.Fatal("Add should skip a blank word")
	}
}

func TestDictionaryPrompt(t *testing.T) {
	d := &Dictionary{Words: []string{"PostHog", "gosaid"}}
	if got, want := d.Prompt(), "PostHog, gosaid"; got != want {
		t.Fatalf("Prompt() = %q, want %q", got, want)
	}
	if got := (&Dictionary{}).Prompt(); got != "" {
		t.Fatalf("empty Prompt() = %q, want empty", got)
	}
}

func TestLoadDictionaryMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.json")
	d, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("LoadDictionary on missing file: %v", err)
	}
	if len(d.Words) != 0 {
		t.Fatalf("Words = %v, want empty", d.Words)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.json")
	in := &Dictionary{Words: []string{"Anthropic", "Claude"}}
	if err := SaveDictionary(path, in); err != nil {
		t.Fatalf("SaveDictionary: %v", err)
	}
	out, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("LoadDictionary: %v", err)
	}
	if len(out.Words) != 2 || out.Words[0] != "Anthropic" || out.Words[1] != "Claude" {
		t.Fatalf("round trip = %v", out.Words)
	}
}

func TestLoadDictionaryEmptyFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := LoadDictionary(path)
	if err != nil {
		t.Fatalf("LoadDictionary on empty file: %v", err)
	}
	if len(d.Words) != 0 {
		t.Fatalf("Words = %v, want empty", d.Words)
	}
}

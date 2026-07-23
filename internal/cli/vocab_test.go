package cli

import (
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// vocabFixture points the vocabulary at a temp dir via XDG_CONFIG_HOME and
// returns the resolved path.
func vocabFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "gosaid", "vocabulary.json")
}

func TestRunVocabAddThenDelete(t *testing.T) {
	path := vocabFixture(t)

	if code := RunVocab([]string{"Kubernetes"}); code != 0 {
		t.Fatalf("add exit = %d, want 0", code)
	}
	v, err := config.LoadVocabulary(path)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Contains("Kubernetes") {
		t.Fatalf("word not persisted: %v", v.Words)
	}

	// Duplicate add is a no-op success and does not grow the file.
	if code := RunVocab([]string{"kubernetes"}); code != 0 {
		t.Fatalf("duplicate add exit = %d, want 0", code)
	}
	if v, _ = config.LoadVocabulary(path); len(v.Words) != 1 {
		t.Fatalf("duplicate add changed words: %v", v.Words)
	}

	if code := RunVocab([]string{"Kubernetes", "--delete"}); code != 0 {
		t.Fatalf("delete exit = %d, want 0", code)
	}
	if v, _ = config.LoadVocabulary(path); len(v.Words) != 0 {
		t.Fatalf("word not removed: %v", v.Words)
	}
}

func TestRunVocabMultiWordTerm(t *testing.T) {
	path := vocabFixture(t)
	if code := RunVocab([]string{"New", "York"}); code != 0 {
		t.Fatalf("add exit = %d, want 0", code)
	}
	v, _ := config.LoadVocabulary(path)
	if !v.Contains("New York") {
		t.Fatalf("multi-word term not joined: %v", v.Words)
	}
}

func TestRunVocabLoneDeleteIsUsageError(t *testing.T) {
	vocabFixture(t)
	if code := RunVocab([]string{"--delete"}); code != 2 {
		t.Fatalf("lone --delete exit = %d, want 2", code)
	}
}

func TestRunVocabUnknownFlag(t *testing.T) {
	vocabFixture(t)
	if code := RunVocab([]string{"--bogus"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
}

func TestRunVocabListEmpty(t *testing.T) {
	vocabFixture(t)
	if code := RunVocab(nil); code != 0 {
		t.Fatalf("list empty exit = %d, want 0", code)
	}
}

package cli

import (
	"path/filepath"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// dictFixture points the vocabulary at a temp dir via XDG_CONFIG_HOME and
// returns the resolved path.
func dictFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "gosaid", "vocabulary.json")
}

func TestRunDictAddThenDelete(t *testing.T) {
	path := dictFixture(t)

	if code := RunDict([]string{"Kubernetes"}); code != 0 {
		t.Fatalf("add exit = %d, want 0", code)
	}
	d, err := config.LoadVocabulary(path)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Contains("Kubernetes") {
		t.Fatalf("word not persisted: %v", d.Words)
	}

	// Duplicate add is a no-op success and does not grow the file.
	if code := RunDict([]string{"kubernetes"}); code != 0 {
		t.Fatalf("duplicate add exit = %d, want 0", code)
	}
	if d, _ = config.LoadVocabulary(path); len(d.Words) != 1 {
		t.Fatalf("duplicate add changed words: %v", d.Words)
	}

	if code := RunDict([]string{"Kubernetes", "--delete"}); code != 0 {
		t.Fatalf("delete exit = %d, want 0", code)
	}
	if d, _ = config.LoadVocabulary(path); len(d.Words) != 0 {
		t.Fatalf("word not removed: %v", d.Words)
	}
}

func TestRunDictMultiWordTerm(t *testing.T) {
	path := dictFixture(t)
	if code := RunDict([]string{"New", "York"}); code != 0 {
		t.Fatalf("add exit = %d, want 0", code)
	}
	d, _ := config.LoadVocabulary(path)
	if !d.Contains("New York") {
		t.Fatalf("multi-word term not joined: %v", d.Words)
	}
}

func TestRunDictLoneDeleteIsUsageError(t *testing.T) {
	dictFixture(t)
	if code := RunDict([]string{"--delete"}); code != 2 {
		t.Fatalf("lone --delete exit = %d, want 2", code)
	}
}

func TestRunDictUnknownFlag(t *testing.T) {
	dictFixture(t)
	if code := RunDict([]string{"--bogus"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
}

func TestRunDictListEmpty(t *testing.T) {
	dictFixture(t)
	if code := RunDict(nil); code != 0 {
		t.Fatalf("list empty exit = %d, want 0", code)
	}
}

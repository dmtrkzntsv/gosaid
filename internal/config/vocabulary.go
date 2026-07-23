package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// Vocabulary is the user's personal dictionary: proper nouns, product names,
// jargon, and other custom words that transcription and the text stages should
// spell correctly. It is stored in vocabulary.json next to config.json.
type Vocabulary struct {
	Words []string `json:"words"`
}

// VocabularyPath returns the resolved vocabulary.json path.
func VocabularyPath() (string, error) {
	return platform.VocabularyFile()
}

// LoadVocabulary reads the vocabulary from disk. A missing file is not an
// error — it yields an empty vocabulary, so callers can always use the result.
func LoadVocabulary(path string) (*Vocabulary, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Vocabulary{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("read vocabulary: %w", err)
	}
	var v Vocabulary
	if len(data) > 0 {
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("parse vocabulary: %w", err)
		}
	}
	return &v, nil
}

// SaveVocabulary writes the vocabulary to disk atomically (tmp + rename).
func SaveVocabulary(path string, v *Vocabulary) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

// Add inserts a word, trimming surrounding whitespace and skipping
// case-insensitive duplicates. It reports whether the vocabulary changed.
func (v *Vocabulary) Add(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	if v.Contains(word) {
		return false
	}
	v.Words = append(v.Words, word)
	return true
}

// Remove deletes the first case-insensitive match of word. It reports whether
// the vocabulary changed.
func (v *Vocabulary) Remove(word string) bool {
	word = strings.TrimSpace(word)
	for i, w := range v.Words {
		if strings.EqualFold(w, word) {
			v.Words = append(v.Words[:i], v.Words[i+1:]...)
			return true
		}
	}
	return false
}

// Contains reports whether word is already present, case-insensitively.
func (v *Vocabulary) Contains(word string) bool {
	word = strings.TrimSpace(word)
	for _, w := range v.Words {
		if strings.EqualFold(w, word) {
			return true
		}
	}
	return false
}

// Prompt renders the vocabulary as a single comma-separated string, suitable
// both as a Whisper initial prompt and as a hint injected into the text-stage
// system prompts. It returns "" when the vocabulary is empty.
func (v *Vocabulary) Prompt() string {
	return strings.Join(v.Words, ", ")
}

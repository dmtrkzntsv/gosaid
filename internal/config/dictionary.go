package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// Dictionary is the user's personal vocabulary: proper nouns, product names,
// jargon, and other custom words that transcription and the text stages should
// spell correctly. It is stored in dictionary.json next to config.json.
type Dictionary struct {
	Words []string `json:"words"`
}

// DictionaryPath returns the resolved dictionary.json path.
func DictionaryPath() (string, error) {
	return platform.DictionaryFile()
}

// LoadDictionary reads the dictionary from disk. A missing file is not an
// error — it yields an empty dictionary, so callers can always use the result.
func LoadDictionary(path string) (*Dictionary, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Dictionary{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("read dictionary: %w", err)
	}
	var d Dictionary
	if len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("parse dictionary: %w", err)
		}
	}
	return &d, nil
}

// SaveDictionary writes the dictionary to disk atomically (tmp + rename).
func SaveDictionary(path string, d *Dictionary) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

// Add inserts a word, trimming surrounding whitespace and skipping
// case-insensitive duplicates. It reports whether the dictionary changed.
func (d *Dictionary) Add(word string) bool {
	word = strings.TrimSpace(word)
	if word == "" {
		return false
	}
	if d.Contains(word) {
		return false
	}
	d.Words = append(d.Words, word)
	return true
}

// Remove deletes the first case-insensitive match of word. It reports whether
// the dictionary changed.
func (d *Dictionary) Remove(word string) bool {
	word = strings.TrimSpace(word)
	for i, w := range d.Words {
		if strings.EqualFold(w, word) {
			d.Words = append(d.Words[:i], d.Words[i+1:]...)
			return true
		}
	}
	return false
}

// Contains reports whether word is already present, case-insensitively.
func (d *Dictionary) Contains(word string) bool {
	word = strings.TrimSpace(word)
	for _, w := range d.Words {
		if strings.EqualFold(w, word) {
			return true
		}
	}
	return false
}

// Prompt renders the vocabulary as a single comma-separated string, suitable
// both as a Whisper initial prompt and as a hint injected into the text-stage
// system prompts. It returns "" when the dictionary is empty.
func (d *Dictionary) Prompt() string {
	return strings.Join(d.Words, ", ")
}

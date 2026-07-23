package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

const vocabUsage = `usage:
  gosaid vocab <word>            add a word to your custom vocabulary
  gosaid vocab <word> --delete   remove a word
  gosaid vocab                   list the current words`

// RunVocab handles `gosaid vocab ...`: add, delete, or list custom-vocabulary
// words. The list is a comma-separated hint fed to transcription and the text
// stages so custom vocabulary is spelled correctly.
func RunVocab(args []string) int {
	var del bool
	var words []string
	for _, a := range args {
		switch a {
		case "--delete", "-delete", "-d":
			del = true
		case "-h", "--help", "help":
			fmt.Println(vocabUsage)
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\n%s\n", a, vocabUsage)
				return 2
			}
			words = append(words, a)
		}
	}
	word := strings.TrimSpace(strings.Join(words, " "))

	path, err := config.VocabularyPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	vocab, err := config.LoadVocabulary(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// No word given → list (a lone --delete is a usage error).
	if word == "" {
		if del {
			fmt.Fprintln(os.Stderr, vocabUsage)
			return 2
		}
		if len(vocab.Words) == 0 {
			fmt.Fprintln(os.Stderr, "vocabulary is empty")
			return 0
		}
		for _, w := range vocab.Words {
			fmt.Println(w)
		}
		return 0
	}

	if del {
		if !vocab.Remove(word) {
			fmt.Fprintf(os.Stderr, "%q is not in your vocabulary\n", word)
			return 0
		}
		if err := config.SaveVocabulary(path, vocab); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "removed %q\n", word)
		return 0
	}

	if !vocab.Add(word) {
		fmt.Fprintf(os.Stderr, "%q is already in your vocabulary\n", word)
		return 0
	}
	if err := config.SaveVocabulary(path, vocab); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "added %q\n", word)
	return 0
}

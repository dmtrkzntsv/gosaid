package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

const dictUsage = `usage:
  gosaid dict <word>            add a word to the personal dictionary
  gosaid dict <word> --delete   remove a word
  gosaid dict                   list the current words`

// RunDict handles `gosaid dict ...`: add, delete, or list personal-dictionary
// words. The list is a comma-separated hint fed to transcription and the text
// stages so custom vocabulary is spelled correctly.
func RunDict(args []string) int {
	var del bool
	var words []string
	for _, a := range args {
		switch a {
		case "--delete", "-delete", "-d":
			del = true
		case "-h", "--help", "help":
			fmt.Println(dictUsage)
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n\n%s\n", a, dictUsage)
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
			fmt.Fprintln(os.Stderr, dictUsage)
			return 2
		}
		if len(vocab.Words) == 0 {
			fmt.Fprintln(os.Stderr, "dictionary is empty")
			return 0
		}
		for _, w := range vocab.Words {
			fmt.Println(w)
		}
		return 0
	}

	if del {
		if !vocab.Remove(word) {
			fmt.Fprintf(os.Stderr, "%q is not in the dictionary\n", word)
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
		fmt.Fprintf(os.Stderr, "%q is already in the dictionary\n", word)
		return 0
	}
	if err := config.SaveVocabulary(path, vocab); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "added %q\n", word)
	return 0
}

package text

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Apply executes case-insensitive whole-word find-and-replace. Triggers are
// matched longest-first so multi-word triggers beat single-word prefixes.
// A "word boundary" here means: start/end of string, or the adjacent rune is
// not a letter or digit in Unicode terms (handles Cyrillic, CJK, etc.).
func Apply(text string, rules map[string]string) string {
	if len(rules) == 0 || text == "" {
		return text
	}
	triggers := make([]string, 0, len(rules))
	for k := range rules {
		if k != "" {
			triggers = append(triggers, k)
		}
	}
	sort.Slice(triggers, func(i, j int) bool {
		return len(triggers[i]) > len(triggers[j])
	})

	alts := make([]string, len(triggers))
	for i, t := range triggers {
		alts[i] = regexp.QuoteMeta(t)
	}
	re := regexp.MustCompile(`(?i)(?:` + strings.Join(alts, "|") + `)`)

	runes := []rune(text)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Locate the match's rune boundaries and check that adjacent runes are non-word.
		idx := strings.Index(text, match)
		if idx < 0 {
			return match
		}
		leftOK := idx == 0 || !isWordRune(runeAt(runes, prevRuneIndex(text, idx)))
		endByte := idx + len(match)
		rightOK := endByte == len(text) || !isWordRune(runeAt(runes, runeIndexAt(text, endByte)))
		if !leftOK || !rightOK {
			return match
		}
		// Find the rule that produced this match (case-insensitive).
		low := strings.ToLower(match)
		for _, t := range triggers {
			if strings.ToLower(t) == low {
				return rules[t]
			}
		}
		return match
	})
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// runeIndexAt returns the rune index corresponding to a byte offset.
func runeIndexAt(s string, byteIdx int) int {
	return len([]rune(s[:byteIdx]))
}

func prevRuneIndex(s string, byteIdx int) int {
	r := runeIndexAt(s, byteIdx)
	if r == 0 {
		return 0
	}
	return r - 1
}

func runeAt(rs []rune, i int) rune {
	if i < 0 || i >= len(rs) {
		return 0
	}
	return rs[i]
}

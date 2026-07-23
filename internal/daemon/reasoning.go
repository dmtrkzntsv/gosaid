package daemon

import (
	"regexp"
	"strings"
)

// Reasoning models emit their chain of thought in a <think>…</think> block
// before the answer. It is private scratch work — dictation must inject only
// what follows it.
//
// The tags are matched case-insensitively and tolerate stray whitespace
// (`< think >`), because different builds format them differently. `(?s)`
// lets `.` span the newlines a reasoning block always contains.
var (
	thinkBlockRe = regexp.MustCompile(`(?is)<\s*think\s*>.*?<\s*/\s*think\s*>`)
	thinkOpenRe  = regexp.MustCompile(`(?is)<\s*think\s*>`)
	thinkCloseRe = regexp.MustCompile(`(?is)<\s*/\s*think\s*>`)
)

// stripReasoning removes a reasoning block from a chat model's reply,
// returning just the answer.
//
// Three shapes occur in the wild:
//   - a complete <think>…</think> block (possibly several) — all are dropped;
//   - a bare closing tag, when the opener was consumed as a prompt prefix —
//     everything before it is reasoning;
//   - an unterminated opener, meaning the answer never arrived — the result is
//     empty, since injecting the model's private reasoning is worse than
//     injecting nothing.
func stripReasoning(s string) string {
	if !thinkOpenRe.MatchString(s) && !thinkCloseRe.MatchString(s) {
		return strings.TrimSpace(s)
	}

	s = thinkBlockRe.ReplaceAllString(s, "")

	// A closing tag surviving the block pass had no opener: drop everything
	// up to and including the last one.
	if loc := lastIndex(thinkCloseRe, s); loc != nil {
		s = s[loc[1]:]
	}

	// An opener surviving both passes was never closed — the answer is missing.
	if loc := thinkOpenRe.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	return strings.TrimSpace(s)
}

// lastIndex returns the location of the final match of re in s, or nil.
func lastIndex(re *regexp.Regexp, s string) []int {
	all := re.FindAllStringIndex(s, -1)
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

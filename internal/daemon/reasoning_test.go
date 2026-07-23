package daemon

import "testing"

// Reasoning models (DeepSeek-R1, Qwen thinking variants, some local GGUF
// builds) emit a chain-of-thought block before the answer. It must never
// reach the injected text.
func TestStripReasoning(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"filled think block": {
			in:   "<think>\nThe user greeted me. I should reply.\n</think>\nПривет! Как ты?",
			want: "Привет! Как ты?",
		},
		"empty think block": {
			in:   "<think>\n\n</think>\n\nWhat new, what good.",
			want: "What new, what good.",
		},
		"no block passes through": {
			in:   "Привет! Как дела?",
			want: "Привет! Как дела?",
		},
		"inline on one line": {
			in:   "<think>brief</think>Answer here.",
			want: "Answer here.",
		},
		"leading whitespace before the tag": {
			in:   "  \n<think>x</think>\nAnswer.",
			want: "Answer.",
		},
		"multiline reasoning with blank lines": {
			in:   "<think>\nStep one.\n\nStep two.\n</think>\nFinal answer.",
			want: "Final answer.",
		},
		"case-insensitive tag": {
			in:   "<THINK>x</THINK>\nAnswer.",
			want: "Answer.",
		},
		"whitespace inside the tag": {
			in:   "< think >x< / think >\nAnswer.",
			want: "Answer.",
		},
		// Some models emit only the closing tag (the opener is consumed as a
		// prompt prefix). Everything up to it is reasoning.
		"unpaired closing tag": {
			in:   "I should greet them.\n</think>\nПривет!",
			want: "Привет!",
		},
		// An unterminated block means the answer never arrived — better to
		// return nothing than to inject the model's private reasoning.
		"unterminated block yields empty": {
			in:   "<think>\nI am still thinking and never finished.",
			want: "",
		},
		"answer containing the word think": {
			in:   "I think we should ship it.",
			want: "I think we should ship it.",
		},
		// Every block is removed, but text between them is answer text and is
		// kept — dropping it could swallow real output.
		"multiple blocks all removed": {
			in:   "<think>a</think>mid<think>b</think>Answer.",
			want: "midAnswer.",
		},
		"empty input": {in: "", want: ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := stripReasoning(c.in); got != c.want {
				t.Errorf("stripReasoning(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dmtrkzntsv/gosaid/internal/llama"
)

// Integration test for the selection-transform prompt against a real local
// model. It is skipped unless GOSAID_LLAMA_MODEL is set.
func TestTransformPromptIntegrationDoesNotLeakReferenceMetadata(t *testing.T) {
	path := os.Getenv("GOSAID_LLAMA_MODEL")
	if path == "" {
		t.Skip("GOSAID_LLAMA_MODEL not set")
	}
	model, err := llama.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()

	system, err := RenderTransform(TransformData{
		UserContext:  "My name is Dmitry (Дмитрий на русском).",
		Instructions: "write short and precise sentences",
		Vocabulary:   "Econumo, Эконумо, gosaid, winterflow, Kubernetes, New York",
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		selection   string
		instruction string
		want        []string
	}{
		{
			name:        "English to Spanish",
			selection:   "Hi guys! How are you doing?",
			instruction: "translate to Spanish",
			want:        []string{"hola"},
		},
		{
			name:        "Russian to English",
			selection:   "Привет мир",
			instruction: "translate to English",
			want:        []string{"hello", "world"},
		},
	}
	leakMarkers := []string{
		"dmitry", "дмитрий", "econumo", "эконумо", "gosaid", "winterflow",
		"kubernetes", "new york", "about the user", "additional instructions",
		"custom vocabulary", "rules:", "<user_context>", "<hotkey_instructions>",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, err := RenderTransformRequest(TransformRequestData{
				Selection:   tc.selection,
				Instruction: tc.instruction,
			})
			if err != nil {
				t.Fatal(err)
			}

			out, err := model.Chat(context.Background(), system, request, llama.Options{MaxTokens: 128})
			if err != nil {
				t.Fatal(err)
			}
			out = stripReasoning(out)
			t.Logf("model output: %q", out)
			lower := strings.ToLower(out)
			for _, leaked := range leakMarkers {
				if strings.Contains(lower, leaked) {
					t.Errorf("output leaked prompt metadata %q: %q", leaked, out)
				}
			}
			for _, want := range tc.want {
				if !strings.Contains(lower, want) {
					t.Errorf("output missing expected translation marker %q: %q", want, out)
				}
			}
		})
	}
}

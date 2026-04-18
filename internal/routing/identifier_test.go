package routing

import "testing"

func TestParseModelRef(t *testing.T) {
	cases := []struct {
		in      string
		want    ModelRef
		wantErr bool
	}{
		{"groq:whisper-large-v3", ModelRef{"groq", "whisper-large-v3"}, false},
		{"openrouter:anthropic/claude-3-5-haiku", ModelRef{"openrouter", "anthropic/claude-3-5-haiku"}, false},
		{"x:y:z", ModelRef{"x", "y:z"}, false}, // split on first colon only
		{":model", ModelRef{}, true},
		{"endpoint:", ModelRef{}, true},
		{"noColon", ModelRef{}, true},
		{"", ModelRef{}, true},
	}
	for _, tc := range cases {
		got, err := ParseModelRef(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

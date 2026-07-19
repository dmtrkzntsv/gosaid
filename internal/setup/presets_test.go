package setup

import "testing"

func TestProviderPresetsOrderAndShape(t *testing.T) {
	if ProviderPresets[0].Key != "local" || !ProviderPresets[0].Local {
		t.Fatalf("first preset must be local whisper, got %+v", ProviderPresets[0])
	}
	last := ProviderPresets[len(ProviderPresets)-1]
	if last.Key != "custom" || !last.Custom {
		t.Fatalf("last preset must be custom, got %+v", last)
	}
	for _, p := range ProviderPresets {
		if p.Local || p.Custom {
			continue
		}
		if p.APIBase == "" || p.ChatModel == "" {
			t.Errorf("cloud preset %q must have APIBase and ChatModel: %+v", p.Key, p)
		}
	}
}

func TestPresetForAPIBase(t *testing.T) {
	if p := PresetForAPIBase("https://api.openai.com/v1"); p == nil || p.Key != "openai" {
		t.Fatalf("openai base not matched: %+v", p)
	}
	if p := PresetForAPIBase("https://example.com/v1"); p != nil {
		t.Fatalf("unknown base must return nil, got %+v", p)
	}
}

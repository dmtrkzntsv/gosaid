package daemon

import (
	"strings"
	"testing"
)

func TestRenderTransformIncludesSelection(t *testing.T) {
	system, err := RenderTransform(TransformData{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	request, err := RenderTransformRequest(TransformRequestData{
		Selection:   "hey buddy, fix the thing",
		Instruction: "make it formal",
	})
	if err != nil {
		t.Fatalf("render request: %v", err)
	}
	if !strings.Contains(request, "<selected_text>\nhey buddy, fix the thing\n</selected_text>") {
		t.Fatalf("selection missing from request:\n%s", request)
	}
	if !strings.Contains(request, "<instruction>\nmake it formal\n</instruction>") {
		t.Fatalf("instruction missing from request:\n%s", request)
	}
	if strings.Contains(system, "hey buddy, fix the thing") {
		t.Fatalf("selection must not be embedded in the system prompt:\n%s", system)
	}
	if !strings.Contains(system, "same language as the selected text") {
		t.Fatalf("language rule missing from prompt:\n%s", system)
	}
	if strings.Contains(system, "<user_context>") {
		t.Fatalf("user-context block must be absent when empty:\n%s", system)
	}
	if strings.Contains(system, "<hotkey_instructions>") {
		t.Fatalf("instructions block must be absent when empty:\n%s", system)
	}
}

func TestRenderTransformOptionalBlocks(t *testing.T) {
	out, err := RenderTransform(TransformData{
		UserContext:  "  Dmitry, staff engineer  ",
		Instructions: "  prefer plain words  ",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "Dmitry, staff engineer") {
		t.Fatalf("user context missing:\n%s", out)
	}
	if !strings.Contains(out, "prefer plain words") {
		t.Fatalf("instructions missing:\n%s", out)
	}
}

func TestRenderTransformTranslationSeparatesSourceFromReferenceMetadata(t *testing.T) {
	system, err := RenderTransform(TransformData{
		UserContext: "El nombre es Dmitry (Дмитрий на русском).",
		Vocabulary:  "Econumo, Эконумо, gosaid, winterflow, Kubernetes, New York",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	request, err := RenderTransformRequest(TransformRequestData{
		Selection:   "Hi guys! How are you doing?",
		Instruction: "translate to Spanish",
	})
	if err != nil {
		t.Fatalf("render request: %v", err)
	}

	for _, want := range []string{
		"complete and only source text",
		"Never copy, translate, summarize, mention",
		"when the source addresses a group, use plural forms",
		"Never describe what language the output is in",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt missing %q:\n%s", want, system)
		}
	}
	for _, unwanted := range []string{"Dmitry", "Econumo", "New York"} {
		if strings.Contains(request, unwanted) {
			t.Errorf("request leaked reference metadata %q:\n%s", unwanted, request)
		}
	}
	for _, want := range []string{"Hi guys! How are you doing?", "translate to Spanish"} {
		if !strings.Contains(request, want) {
			t.Errorf("request missing %q:\n%s", want, request)
		}
	}
}

func TestRenderTransformRequestPreservesSelectionWhitespace(t *testing.T) {
	request, err := RenderTransformRequest(TransformRequestData{
		Selection:   "  indented text  \n",
		Instruction: "  make it formal  ",
	})
	if err != nil {
		t.Fatalf("render request: %v", err)
	}
	if !strings.Contains(request, "<selected_text>\n  indented text  \n\n</selected_text>") {
		t.Fatalf("selection whitespace changed:\n%q", request)
	}
	if !strings.Contains(request, "<instruction>\nmake it formal\n</instruction>") {
		t.Fatalf("instruction was not trimmed:\n%q", request)
	}
}

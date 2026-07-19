package daemon

import (
	"strings"
	"testing"
)

func TestRenderTransformIncludesSelection(t *testing.T) {
	out, err := RenderTransform(TransformData{Selection: "hey buddy, fix the thing"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "hey buddy, fix the thing") {
		t.Fatalf("selection missing from prompt:\n%s", out)
	}
	if !strings.Contains(out, "same language as the selected text") {
		t.Fatalf("language rule missing from prompt:\n%s", out)
	}
	if strings.Contains(out, "About the user") {
		t.Fatalf("user-context block must be absent when empty:\n%s", out)
	}
	if strings.Contains(out, "Additional instructions") {
		t.Fatalf("instructions block must be absent when empty:\n%s", out)
	}
}

func TestRenderTransformOptionalBlocks(t *testing.T) {
	out, err := RenderTransform(TransformData{
		Selection:    "text",
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

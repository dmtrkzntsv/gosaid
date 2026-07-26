package config

import "testing"

func TestDetectHostedProvider(t *testing.T) {
	openAI, ok := DetectHostedProvider(OpenAIAPIBase + "/")
	if !ok {
		t.Fatal("OpenAI API base was not detected")
	}
	if openAI.Label != "OpenAI" ||
		openAI.TranscribeModel != "whisper-1" ||
		openAI.ChatModel != "gpt-5.4-nano" {
		t.Fatalf("OpenAI defaults = %+v", openAI)
	}

	openRouter, ok := DetectHostedProvider(OpenRouterAPIBase)
	if !ok {
		t.Fatal("OpenRouter API base was not detected")
	}
	if openRouter.TranscribeModel != "" ||
		openRouter.ChatModel != "openai/gpt-5.4-nano" {
		t.Fatalf("OpenRouter defaults = %+v", openRouter)
	}

	if _, ok := DetectHostedProvider("http://localhost:11434/v1"); ok {
		t.Fatal("custom compatible API was detected as a preset")
	}
}

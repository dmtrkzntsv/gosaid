package config

import "strings"

const (
	OpenAIAPIBase     = "https://api.openai.com/v1"
	OpenRouterAPIBase = "https://openrouter.ai/api/v1"
)

// HostedProviderDefaults describes a known OpenAI-compatible provider and the
// model names setup can use without asking the user to type one.
type HostedProviderDefaults struct {
	Label           string
	TranscribeModel string
	ChatModel       string
}

// DetectHostedProvider recognizes a hosted-provider preset by API base URL.
// The trailing slash is ignored.
func DetectHostedProvider(apiBase string) (HostedProviderDefaults, bool) {
	switch strings.TrimRight(strings.TrimSpace(apiBase), "/") {
	case OpenAIAPIBase:
		return HostedProviderDefaults{
			Label:           "OpenAI",
			TranscribeModel: "whisper-1",
			ChatModel:       "gpt-5.4-nano",
		}, true
	case OpenRouterAPIBase:
		return HostedProviderDefaults{
			Label:     "OpenRouter",
			ChatModel: "openai/gpt-5.4-nano",
		}, true
	default:
		return HostedProviderDefaults{}, false
	}
}

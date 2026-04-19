package daemon

import (
	"embed"
	"strings"
	"text/template"
)

// Bumped whenever the contents of prompts/*.tmpl change in a way that
// influences model output. Logged at startup for debugging.
const (
	TranslateTemplateVersion = 2
	EnhanceTemplateVersion   = 2
)

const defaultTranslateInstruction = "Match the exact wording of the source as closely as possible and keep the original style, register, and tone. Do not paraphrase, summarize, or rephrase."

const defaultEnhanceInstruction = "Clean up the text by removing speech disfluencies such as filler words (\"um\", \"uh\", \"mm\", \"hmm\", \"er\", \"ah\", \"like\", \"you know\"), false starts, repeated words, and self-corrections. Do not rephrase, summarize, or change the meaning, wording, or style beyond removing those disfluencies."

//go:embed prompts/translate.tmpl prompts/enhance.tmpl
var promptFS embed.FS

var (
	translateTmpl = template.Must(template.ParseFS(promptFS, "prompts/translate.tmpl"))
	enhanceTmpl   = template.Must(template.ParseFS(promptFS, "prompts/enhance.tmpl"))
)

// TranslateData fills the translate template. Vocabulary and Replacements
// are formatted elsewhere into `[]string` lines (one term per line).
type TranslateData struct {
	SourceLanguage   string
	TargetLanguage   string
	UserInstructions string
	Vocabulary       []string
	Replacements     []string
}

type EnhanceData struct {
	UserInstructions string
	Vocabulary       []string
	Replacements     []string
}

func RenderTranslate(d TranslateData) (string, error) {
	if strings.TrimSpace(d.UserInstructions) == "" {
		d.UserInstructions = defaultTranslateInstruction
	}
	var b strings.Builder
	if err := translateTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderEnhance(d EnhanceData) (string, error) {
	if strings.TrimSpace(d.UserInstructions) == "" {
		d.UserInstructions = defaultEnhanceInstruction
	}
	var b strings.Builder
	if err := enhanceTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

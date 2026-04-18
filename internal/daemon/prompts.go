package daemon

import (
	"embed"
	"strings"
	"text/template"
)

// Bumped whenever the contents of prompts/*.tmpl change in a way that
// influences model output. Logged at startup for debugging.
const (
	TranslateTemplateVersion = 1
	EnhanceTemplateVersion   = 1
)

const defaultTranslateInstruction = "Translate faithfully, preserving the original meaning and tone."

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
	var b strings.Builder
	if err := enhanceTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

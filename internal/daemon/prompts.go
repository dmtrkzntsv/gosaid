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
	d.UserInstructions = strings.TrimSpace(d.UserInstructions)
	var b strings.Builder
	if err := translateTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderEnhance(d EnhanceData) (string, error) {
	d.UserInstructions = strings.TrimSpace(d.UserInstructions)
	var b strings.Builder
	if err := enhanceTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

package daemon

import (
	"embed"
	"strings"
	"text/template"
)

// Bumped whenever the contents of prompts/*.tmpl change in a way that
// influences model output. Logged at startup for debugging.
const (
	TranslateTemplateVersion = 3
	EnhanceTemplateVersion   = 3
	ComposeTemplateVersion   = 1
)

//go:embed prompts/translate.tmpl prompts/enhance.tmpl prompts/compose.tmpl
var promptFS embed.FS

var (
	translateTmpl = template.Must(template.ParseFS(promptFS, "prompts/translate.tmpl"))
	enhanceTmpl   = template.Must(template.ParseFS(promptFS, "prompts/enhance.tmpl"))
	composeTmpl   = template.Must(template.ParseFS(promptFS, "prompts/compose.tmpl"))
)

type TranslateData struct {
	SourceLanguage string
	TargetLanguage string
}

type EnhanceData struct{}

type ComposeData struct{}

func RenderTranslate(d TranslateData) (string, error) {
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

func RenderCompose(d ComposeData) (string, error) {
	var b strings.Builder
	if err := composeTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

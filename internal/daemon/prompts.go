package daemon

import (
	"embed"
	"strings"
	"text/template"
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

type ComposeData struct {
	UserContext string
}

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
	d.UserContext = strings.TrimSpace(d.UserContext)
	var b strings.Builder
	if err := composeTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

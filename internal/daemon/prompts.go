package daemon

import (
	"embed"
	"strings"
	"text/template"
)

//go:embed prompts/translate.tmpl prompts/enhance.tmpl prompts/compose.tmpl prompts/transform.tmpl prompts/transform_request.tmpl
var promptFS embed.FS

var (
	translateTmpl        = template.Must(template.ParseFS(promptFS, "prompts/translate.tmpl"))
	enhanceTmpl          = template.Must(template.ParseFS(promptFS, "prompts/enhance.tmpl"))
	composeTmpl          = template.Must(template.ParseFS(promptFS, "prompts/compose.tmpl"))
	transformTmpl        = template.Must(template.ParseFS(promptFS, "prompts/transform.tmpl"))
	transformRequestTmpl = template.Must(template.ParseFS(promptFS, "prompts/transform_request.tmpl"))
)

type TranslateData struct {
	SourceLanguage string
	TargetLanguage string
	// Vocabulary is the user's personal dictionary as a comma-separated hint.
	Vocabulary string
}

type EnhanceData struct {
	// Vocabulary is the user's personal dictionary as a comma-separated hint.
	Vocabulary string
}

type ComposeData struct {
	UserContext  string
	Instructions string
	// Vocabulary is the user's personal dictionary as a comma-separated hint.
	Vocabulary string
}

type TransformData struct {
	UserContext  string
	Instructions string
	// Vocabulary is the user's personal dictionary as a comma-separated hint.
	Vocabulary string
}

type TransformRequestData struct {
	Selection   string
	Instruction string
}

func RenderTranslate(d TranslateData) (string, error) {
	d.Vocabulary = strings.TrimSpace(d.Vocabulary)
	var b strings.Builder
	if err := translateTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderEnhance(d EnhanceData) (string, error) {
	d.Vocabulary = strings.TrimSpace(d.Vocabulary)
	var b strings.Builder
	if err := enhanceTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderCompose(d ComposeData) (string, error) {
	d.UserContext = strings.TrimSpace(d.UserContext)
	d.Instructions = strings.TrimSpace(d.Instructions)
	d.Vocabulary = strings.TrimSpace(d.Vocabulary)
	var b strings.Builder
	if err := composeTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderTransform(d TransformData) (string, error) {
	d.UserContext = strings.TrimSpace(d.UserContext)
	d.Instructions = strings.TrimSpace(d.Instructions)
	d.Vocabulary = strings.TrimSpace(d.Vocabulary)
	var b strings.Builder
	if err := transformTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

func RenderTransformRequest(d TransformRequestData) (string, error) {
	d.Instruction = strings.TrimSpace(d.Instruction)
	var b strings.Builder
	if err := transformRequestTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

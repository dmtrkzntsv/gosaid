package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/drivers"
	"github.com/dmtrkzntsv/gosaid/internal/inject"
	"github.com/dmtrkzntsv/gosaid/internal/routing"
	"github.com/dmtrkzntsv/gosaid/internal/text"
)

// captureStopper is the minimum audio-capture surface the pipeline needs.
// Allows tests to inject fake audio without pulling in malgo.
type captureStopper interface {
	Stop() ([]float32, error)
}

type Pipeline struct {
	Core       *Core
	Capture    captureStopper
	Registry   *drivers.Registry
	Injector   inject.Injector
	Config     *config.Config
	SampleRate int
	Log        *slog.Logger
}

// Run executes the full pipeline for one hotkey trigger. Called after the
// user releases the hotkey (or the toggle-mode cap fires).
func (p *Pipeline) Run(ctx context.Context, hk config.Hotkey) error {
	samples, err := p.Capture.Stop()
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}
	p.Core.Transition(StateTranscribing, nil)

	text1, detectedLang, err := p.transcribe(ctx, samples, hk.Transcribe)
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}
	p.Log.Info("transcription processed", "chars", len(text1), "lang", detectedLang)
	p.Log.Debug("transcription", "text", text1, "lang", detectedLang)

	p.Core.Transition(StateProcessing, nil)

	text2, err := p.translate(ctx, text1, detectedLang, hk.Translate)
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}

	text3, err := p.enhance(ctx, text2, hk.Enhance)
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}

	outputLang := outputLanguage(hk)
	final := p.applyReplacements(text3, outputLang)

	if final == "" {
		// Empty transcription — skip injection but still transition cleanly.
		p.Core.Transition(StateIdle, nil)
		return nil
	}

	p.Core.Transition(StateInjecting, nil)
	if err := p.Injector.Inject(ctx, final); err != nil {
		var iferr *inject.InjectionFailedError
		if errors.As(err, &iferr) && iferr.TextInClipboard {
			p.Core.Transition(StateError, fmt.Errorf("paste failed — use Cmd/Ctrl+V to paste from clipboard: %w", err))
			return err
		}
		p.Core.Transition(StateError, err)
		return err
	}
	p.Core.Transition(StateIdle, nil)
	return nil
}

func (p *Pipeline) transcribe(ctx context.Context, samples []float32, stage config.TranscribeStage) (string, string, error) {
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", "", err
	}
	vocab := p.vocabularyFor(stage.OutputLanguage)
	prompt := joinVocab(vocab)

	// English fast path via Whisper's native translate task.
	if stage.OutputLanguage == "en" {
		out, err := drv.TranslateSpeech(ctx, samples, p.SampleRate, model, drivers.TranslateSpeechOptions{
			InitialPrompt: prompt,
		})
		if err != nil {
			return "", "", err
		}
		return out, "en", nil
	}

	res, err := drv.Transcribe(ctx, samples, p.SampleRate, model, drivers.TranscribeOptions{
		Language:      stage.InputLanguage,
		InitialPrompt: prompt,
	})
	if err != nil {
		return "", "", err
	}
	return res.Text, res.DetectedLanguage, nil
}

func (p *Pipeline) translate(ctx context.Context, input, detected string, stage *config.TranslateStage) (string, error) {
	if stage == nil {
		return input, nil
	}
	if normalizeLang(detected) == stage.OutputLanguage {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderTranslate(TranslateData{
		SourceLanguage:   config.LanguageName(normalizeLang(detected)),
		TargetLanguage:   config.LanguageName(stage.OutputLanguage),
		UserInstructions: stage.Prompt,
		Vocabulary:       p.vocabularyFor(stage.OutputLanguage),
		Replacements:     replacementKeys(p.Config.Replacements[stage.OutputLanguage]),
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	p.Log.Debug("translation", "text", out, "source", normalizeLang(detected), "target", stage.OutputLanguage)
	return out, nil
}

func (p *Pipeline) enhance(ctx context.Context, input string, stage *config.EnhanceStage) (string, error) {
	if stage == nil {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	// Enhance uses the output language for vocab/replacements. We don't know
	// it without re-reading the hotkey; the caller set replacements before
	// reaching here, so use the original input's language heuristically.
	system, err := RenderEnhance(EnhanceData{UserInstructions: stage.Prompt})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	p.Log.Debug("enhancement", "text", out)
	return out, nil
}

func (p *Pipeline) applyReplacements(s, lang string) string {
	if rules, ok := p.Config.Replacements[lang]; ok {
		return text.Apply(s, rules)
	}
	return s
}

func (p *Pipeline) resolve(modelRef string) (drivers.Driver, string, error) {
	m, err := routing.ParseModelRef(modelRef)
	if err != nil {
		return nil, "", err
	}
	drv, err := p.Registry.Endpoint(m.Endpoint)
	if err != nil {
		return nil, "", err
	}
	return drv, m.Model, nil
}

func (p *Pipeline) vocabularyFor(lang string) []string {
	if lang == "" {
		return nil
	}
	return p.Config.Vocabulary[lang]
}

// outputLanguage returns the effective output language for a hotkey, used to
// pick the replacement table. Priority: translate.output_language > transcribe.output_language > "".
func outputLanguage(hk config.Hotkey) string {
	if hk.Translate != nil {
		return hk.Translate.OutputLanguage
	}
	if hk.Transcribe.OutputLanguage != "" {
		return hk.Transcribe.OutputLanguage
	}
	return ""
}

// normalizeLang maps the human-readable language names Whisper sometimes
// returns (e.g. "english") to the ISO 639-1 codes we use internally.
func normalizeLang(s string) string {
	switch s {
	case "english":
		return "en"
	case "russian":
		return "ru"
	case "french":
		return "fr"
	case "german":
		return "de"
	case "spanish":
		return "es"
	case "italian":
		return "it"
	case "portuguese":
		return "pt"
	case "dutch":
		return "nl"
	case "japanese":
		return "ja"
	case "korean":
		return "ko"
	case "chinese":
		return "zh"
	}
	return s
}

func joinVocab(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	out := xs[0]
	for _, x := range xs[1:] {
		out += ", " + x
	}
	return out
}

func replacementKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

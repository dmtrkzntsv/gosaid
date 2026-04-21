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
	p.Log.Debug("transcription processed", "chars", len(text1), "text", text1, "lang", detectedLang)

	p.Core.Transition(StateProcessing, nil)

	var reshaped string
	translateLang := detectedLang
	switch {
	case hk.Compose != nil:
		if hk.Enhance != nil {
			p.Log.Debug("compose set: enhance stage skipped")
		}
		reshaped, err = p.compose(ctx, text1, hk.Compose)
		// Compose may produce output in a different language than the transcript
		// (e.g. Russian instruction "write this in English"). Drop the stale
		// language hint so translate neither skips incorrectly nor fills the
		// prompt with a wrong source.
		translateLang = ""
	case hk.Enhance != nil:
		reshaped, err = p.enhance(ctx, text1, hk.Enhance)
	default:
		reshaped = text1
	}
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}

	final, err := p.translate(ctx, reshaped, translateLang, hk.Translate)
	if err != nil {
		p.Core.Transition(StateError, err)
		return err
	}

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

	// English fast path via Whisper's native translate task.
	if stage.OutputLanguage == "en" {
		out, err := drv.TranslateSpeech(ctx, samples, p.SampleRate, model, drivers.TranslateSpeechOptions{})
		if err != nil {
			return "", "", err
		}
		return out, "en", nil
	}

	res, err := drv.Transcribe(ctx, samples, p.SampleRate, model, drivers.TranscribeOptions{
		Language: stage.InputLanguage,
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
	detectedCode := normalizeLang(detected)
	if detectedCode != "" && detectedCode == stage.OutputLanguage {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	sourceName := ""
	if detectedCode != "" {
		sourceName = config.LanguageName(detectedCode)
	}
	system, err := RenderTranslate(TranslateData{
		SourceLanguage: sourceName,
		TargetLanguage: config.LanguageName(stage.OutputLanguage),
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	p.Log.Debug("translation", "text", out, "source", detectedCode, "target", stage.OutputLanguage)
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
	system, err := RenderEnhance(EnhanceData{})
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

func (p *Pipeline) compose(ctx context.Context, input string, stage *config.ComposeStage) (string, error) {
	if stage == nil {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderCompose(ComposeData{
		UserContext:  p.Config.UserContext,
		Instructions: stage.Instructions,
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	p.Log.Debug("compose", "text", out)
	return out, nil
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

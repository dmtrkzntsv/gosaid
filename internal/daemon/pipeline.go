package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
	// Vocabulary is the user's personal dictionary rendered as a hint string
	// (comma-separated). It is passed to Whisper as an initial prompt and
	// injected into the text-stage system prompts so custom words are spelled
	// correctly. Empty when the dictionary is empty or absent.
	Vocabulary string
}

// Run executes the full pipeline for one hotkey trigger. Called after the
// user releases the hotkey (or the toggle-mode cap fires). sel, when
// non-nil, delivers the result of the selection capture started at hotkey
// press; it is only set for compose-enabled hotkeys.
func (p *Pipeline) Run(ctx context.Context, hk config.Hotkey, sel <-chan inject.SelectionResult) error {
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

	if strings.TrimSpace(text1) == "" {
		// Silence or an accidental tap — never run LLM stages on an empty
		// instruction; with a selection held that would overwrite it.
		p.Core.Transition(StateIdle, nil)
		return nil
	}

	p.Core.Transition(StateProcessing, nil)

	var reshaped string
	translateLang := detectedLang
	switch {
	case hk.Compose.IsEnabled():
		if hk.Enhance.IsEnabled() {
			p.Log.Debug("compose set: enhance stage skipped")
		}
		var selRes inject.SelectionResult
		if sel != nil {
			select {
			case selRes = <-sel:
			case <-ctx.Done():
				p.Core.Transition(StateError, ctx.Err())
				return ctx.Err()
			}
		}
		if selRes.Err != nil {
			p.Core.Transition(StateError, selRes.Err)
			return selRes.Err
		}
		if selRes.OK {
			p.Log.Debug("selection captured", "chars", len(selRes.Text))
			reshaped, err = p.transform(ctx, text1, selRes.Text, hk.Compose)
		} else {
			reshaped, err = p.compose(ctx, text1, hk.Compose)
		}
		// Compose/transform may produce output in a different language than
		// the transcript (e.g. Russian instruction about English text). Drop
		// the stale language hint so translate neither skips incorrectly nor
		// fills the prompt with a wrong source.
		translateLang = ""
	case hk.Enhance.IsEnabled():
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
	// Record the attempt up-front so the UI can offer "Copy Last Text"
	// even when the inject itself fails (or crashes the daemon outright).
	// Success/failure is flipped after Inject returns.
	if err := RecordInjection(final, false, ""); err != nil {
		p.Log.Warn("state file", "err", err)
	}
	if err := p.Injector.Inject(ctx, final); err != nil {
		var iferr *inject.InjectionFailedError
		if errors.As(err, &iferr) && iferr.TextInClipboard {
			wrapped := fmt.Errorf("paste failed — use Cmd/Ctrl+V to paste from clipboard: %w", err)
			if rerr := RecordInjection(final, false, wrapped.Error()); rerr != nil {
				p.Log.Warn("state file", "err", rerr)
			}
			p.Core.Transition(StateError, wrapped)
			return err
		}
		if rerr := RecordInjection(final, false, err.Error()); rerr != nil {
			p.Log.Warn("state file", "err", rerr)
		}
		p.Core.Transition(StateError, err)
		return err
	}
	if err := RecordInjection(final, true, ""); err != nil {
		p.Log.Warn("state file", "err", err)
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
		out, err := drv.TranslateSpeech(ctx, samples, p.SampleRate, model, drivers.TranslateSpeechOptions{
			InitialPrompt: p.Vocabulary,
		})
		if err != nil {
			return "", "", err
		}
		return out, "en", nil
	}

	res, err := drv.Transcribe(ctx, samples, p.SampleRate, model, drivers.TranscribeOptions{
		Language:      stage.InputLanguage,
		InitialPrompt: p.Vocabulary,
	})
	if err != nil {
		return "", "", err
	}
	return res.Text, res.DetectedLanguage, nil
}

func (p *Pipeline) translate(ctx context.Context, input, detected string, stage *config.TranslateStage) (string, error) {
	if !stage.IsEnabled() {
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
		Vocabulary:     p.Vocabulary,
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	out = stripReasoning(out)
	p.Log.Debug("translation", "text", out, "source", detectedCode, "target", stage.OutputLanguage)
	return out, nil
}

func (p *Pipeline) enhance(ctx context.Context, input string, stage *config.EnhanceStage) (string, error) {
	if !stage.IsEnabled() {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderEnhance(EnhanceData{Vocabulary: p.Vocabulary})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	out = stripReasoning(out)
	p.Log.Debug("enhancement", "text", out)
	return out, nil
}

func (p *Pipeline) compose(ctx context.Context, input string, stage *config.ComposeStage) (string, error) {
	if !stage.IsEnabled() {
		return input, nil
	}
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderCompose(ComposeData{
		UserContext:  p.Config.UserContext,
		Instructions: stage.Instructions,
		Vocabulary:   p.Vocabulary,
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, input)
	if err != nil {
		return "", err
	}
	out = stripReasoning(out)
	p.Log.Debug("compose", "text", out)
	return out, nil
}

// transform rewrites captured selection text according to the dictated
// instruction, reusing the compose stage's model and instructions.
func (p *Pipeline) transform(ctx context.Context, instruction, selection string, stage *config.ComposeStage) (string, error) {
	drv, model, err := p.resolve(stage.Model)
	if err != nil {
		return "", err
	}
	system, err := RenderTransform(TransformData{
		UserContext:  p.Config.UserContext,
		Instructions: stage.Instructions,
		Vocabulary:   p.Vocabulary,
	})
	if err != nil {
		return "", err
	}
	request, err := RenderTransformRequest(TransformRequestData{
		Selection:   selection,
		Instruction: instruction,
	})
	if err != nil {
		return "", err
	}
	out, err := drv.Chat(ctx, model, system, request)
	if err != nil {
		return "", err
	}
	out = stripReasoning(out)
	p.Log.Debug("transform", "text", out)
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

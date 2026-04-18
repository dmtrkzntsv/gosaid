package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
)

// OpenAICompatible implements Driver against any endpoint that speaks the
// OpenAI API subset we rely on (transcriptions, translations, chat completions).
type OpenAICompatible struct {
	apiBase string
	apiKey  string
	http    *http.Client
}

func NewOpenAICompatible(apiBase, apiKey string) *OpenAICompatible {
	return &OpenAICompatible{
		apiBase: strings.TrimRight(apiBase, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *OpenAICompatible) Transcribe(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranscribeOptions) (TranscribeResult, error) {
	body, contentType, err := buildAudioForm(samples, sampleRate, model, opts.Language, opts.InitialPrompt, true)
	if err != nil {
		return TranscribeResult{}, err
	}
	var resp struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := c.postMultipart(ctx, "/audio/transcriptions", contentType, body, &resp); err != nil {
		return TranscribeResult{}, err
	}
	return TranscribeResult{Text: strings.TrimSpace(resp.Text), DetectedLanguage: resp.Language}, nil
}

func (c *OpenAICompatible) TranslateSpeech(ctx context.Context, samples []float32, sampleRate int,
	model string, opts TranslateSpeechOptions) (string, error) {
	body, contentType, err := buildAudioForm(samples, sampleRate, model, opts.SourceLanguage, opts.InitialPrompt, false)
	if err != nil {
		return "", err
	}
	var resp struct {
		Text string `json:"text"`
	}
	if err := c.postMultipart(ctx, "/audio/translations", contentType, body, &resp); err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

func (c *OpenAICompatible) Chat(ctx context.Context, model, system, user string) (string, error) {
	payload := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat: network: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", apiError("chat", resp.StatusCode, raw)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("chat: parse: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("chat: empty choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

// buildAudioForm builds a multipart form for /audio/transcriptions or
// /audio/translations. verboseJSON enables the extra detected-language field.
func buildAudioForm(samples []float32, sampleRate int, model, language, prompt string, verboseJSON bool) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	// file field with audio/wav content type
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="audio.wav"`)
	h.Set("Content-Type", "audio/wav")
	fw, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := fw.Write(audio.EncodeWAV(samples, sampleRate)); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("model", model); err != nil {
		return nil, "", err
	}
	if verboseJSON {
		if err := w.WriteField("response_format", "verbose_json"); err != nil {
			return nil, "", err
		}
	}
	if language != "" {
		if err := w.WriteField("language", language); err != nil {
			return nil, "", err
		}
	}
	if prompt != "" {
		if err := w.WriteField("prompt", prompt); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return body, w.FormDataContentType(), nil
}

func (c *OpenAICompatible) postMultipart(ctx context.Context, path, contentType string, body *bytes.Buffer, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: network: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return apiError(path, resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}

func apiError(path string, status int, body []byte) error {
	snippet := string(body)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}
	return fmt.Errorf("%s: api returned %d: %s", path, status, snippet)
}

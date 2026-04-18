package drivers

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) (*OpenAICompatible, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOpenAICompatible(srv.URL, "test-key"), srv
}

func TestTranscribe_ParsesResponse(t *testing.T) {
	var gotAuth, gotModel, gotLang, gotFormat string
	var gotFilePrefix []byte

	drv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch part.FormName() {
			case "file":
				buf, _ := io.ReadAll(part)
				gotFilePrefix = buf[:min(4, len(buf))]
			case "model":
				b, _ := io.ReadAll(part)
				gotModel = string(b)
			case "language":
				b, _ := io.ReadAll(part)
				gotLang = string(b)
			case "response_format":
				b, _ := io.ReadAll(part)
				gotFormat = string(b)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"text":     "hello world",
			"language": "english",
		})
	})

	res, err := drv.Transcribe(context.Background(), []float32{0, 0.1, -0.1}, 16000,
		"whisper-large-v3", TranscribeOptions{Language: "en"})
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if res.Text != "hello world" {
		t.Errorf("text = %q", res.Text)
	}
	if res.DetectedLanguage != "english" {
		t.Errorf("lang = %q", res.DetectedLanguage)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotModel != "whisper-large-v3" {
		t.Errorf("model = %q", gotModel)
	}
	if gotLang != "en" {
		t.Errorf("language = %q", gotLang)
	}
	if gotFormat != "verbose_json" {
		t.Errorf("response_format = %q", gotFormat)
	}
	if string(gotFilePrefix) != "RIFF" {
		t.Errorf("file did not start with RIFF header: %q", gotFilePrefix)
	}
}

func TestTranslateSpeech_HitsTranslationsPath(t *testing.T) {
	drv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/translations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	})
	out, err := drv.TranslateSpeech(context.Background(), []float32{0}, 16000, "whisper-large-v3", TranslateSpeechOptions{})
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if out != "hello" {
		t.Errorf("text = %q", out)
	}
}

func TestChat_PostsJSON(t *testing.T) {
	var gotBody map[string]any
	drv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("content-type = %s", r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  pong  "}}]}`))
	})
	out, err := drv.Chat(context.Background(), "gpt-4", "you are terse", "ping")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if out != "pong" {
		t.Errorf("trimmed output = %q", out)
	}
	if gotBody["model"] != "gpt-4" {
		t.Errorf("model = %v", gotBody["model"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d", len(msgs))
	}
}

func TestAPIError(t *testing.T) {
	drv, _ := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	})
	_, err := drv.Chat(context.Background(), "x", "", "")
	if err == nil {
		t.Fatal("expected error from 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error does not mention status code: %v", err)
	}
}

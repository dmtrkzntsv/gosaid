package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/drivers"
	"github.com/dmtrkzntsv/gosaid/internal/routing"
)

// RunDebug dispatches the internal --debug subcommands used during development.
// Returns true if args[0] was a debug command (regardless of result).
func RunDebug(args []string) (handled bool, code int) {
	if len(args) == 0 || args[0] != "--debug" {
		return false, 0
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "--debug requires a sub-command")
		return true, 2
	}
	switch args[1] {
	case "record-test":
		return true, debugRecordTest(args[2:])
	case "play-test":
		return true, debugPlayTest()
	case "transcribe":
		return true, debugTranscribe(args[2:], false)
	case "translate-speech":
		return true, debugTranscribe(args[2:], true)
	case "chat":
		return true, debugChat(args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown --debug command: %s\n", args[1])
		return true, 2
	}
}

// loadRegistry reads config and builds a driver registry. Validates first.
func loadRegistry() (*drivers.Registry, error) {
	path, err := config.Path()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("config invalid: %w", err)
	}
	return drivers.BuildRegistry(cfg)
}

func debugTranscribe(args []string, translateSpeech bool) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: --debug transcribe <wav_path> <endpoint:model>")
		return 2
	}
	wavPath, modelRef := args[0], args[1]

	data, err := os.ReadFile(wavPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read wav: %v\n", err)
		return 1
	}
	pcm, err := audio.ParseWAV(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse wav: %v\n", err)
		return 1
	}
	// Decode 16-bit PCM back into float32 for the driver, which re-encodes.
	samples := make([]float32, len(pcm.Data)/2)
	for i := range samples {
		v := int16(uint16(pcm.Data[i*2]) | uint16(pcm.Data[i*2+1])<<8)
		samples[i] = float32(v) / 32768.0
	}

	m, err := routing.ParseModelRef(modelRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "model ref: %v\n", err)
		return 2
	}
	reg, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "registry: %v\n", err)
		return 1
	}
	drv, err := reg.Endpoint(m.Endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if translateSpeech {
		out, err := drv.TranslateSpeech(ctx, samples, pcm.SampleRate, m.Model, drivers.TranslateSpeechOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "translate-speech: %v\n", err)
			return 1
		}
		fmt.Println(out)
		return 0
	}
	res, err := drv.Transcribe(ctx, samples, pcm.SampleRate, m.Model, drivers.TranscribeOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "detected: %s\n", res.DetectedLanguage)
	fmt.Println(res.Text)
	return 0
}

func debugChat(args []string) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: --debug chat <endpoint:model> <system> <user>")
		return 2
	}
	modelRef, system, user := args[0], args[1], args[2]
	m, err := routing.ParseModelRef(modelRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "model ref: %v\n", err)
		return 2
	}
	reg, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "registry: %v\n", err)
		return 1
	}
	drv, err := reg.Endpoint(m.Endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := drv.Chat(ctx, m.Model, system, user)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat: %v\n", err)
		return 1
	}
	fmt.Println(out)
	return 0
}

func debugRecordTest(args []string) int {
	dur := 3 * time.Second
	out := "/tmp/gosaid-test.wav"
	if len(args) > 0 {
		out = args[0]
	}

	cap, err := audio.NewCapturer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	defer cap.Close()

	fmt.Fprintf(os.Stderr, "recording %s...\n", dur)
	if err := cap.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		return 1
	}
	time.Sleep(dur)
	samples, err := cap.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stop: %v\n", err)
		return 1
	}

	wav := audio.EncodeWAV(samples, audio.CaptureSampleRate)
	if err := os.WriteFile(out, wav, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "wrote %d samples to %s\n", len(samples), out)
	return 0
}

func debugPlayTest() int {
	fb, err := audio.NewFeedback(true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		return 1
	}
	defer fb.Close()

	fmt.Fprintln(os.Stderr, "start cue...")
	fb.PlayStart()
	time.Sleep(200 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "stop cue...")
	fb.PlayStop()
	time.Sleep(200 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "error cue...")
	fb.PlayError()
	time.Sleep(200 * time.Millisecond)
	return 0
}

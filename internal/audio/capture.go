package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

// CaptureSampleRate is the rate Whisper expects. We resample to this in Go
// after capture, so the device can run at its native rate (avoids format
// negotiation failures on USB devices that resist 16kHz directly).
const CaptureSampleRate = 16000

// Capturer records from the default input device. Single-use per Start/Stop cycle.
type Capturer struct {
	mu         sync.Mutex
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	samples    []float32
	deviceRate int
	active     bool
}

// NewCapturer initializes the audio backend. Close() frees it.
func NewCapturer() (*Capturer, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	return &Capturer{ctx: ctx}, nil
}

// Close releases the audio backend.
func (c *Capturer) Close() {
	c.mu.Lock()
	if c.device != nil {
		c.device.Uninit()
		c.device = nil
	}
	c.mu.Unlock()
	if c.ctx != nil {
		_ = c.ctx.Uninit()
		c.ctx.Free()
		c.ctx = nil
	}
}

// Start begins capture from whichever device is currently the system default.
// Re-resolves the default each call so the daemon survives a hotplug
// (e.g. USB headset disconnected, default reverts to built-in mic).
//
// On macOS, Core Audio can keep a disconnected USB device flagged as default
// until something forces a refresh — so if opening the OS-flagged default
// fails, this falls through to the remaining devices in enumeration order.
func (c *Capturer) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return errors.New("capture already active")
	}

	devices, err := c.ctx.Devices(malgo.Capture)
	if err != nil {
		return fmt.Errorf("enumerate capture devices: %w%s", err, malgoCodeSuffix(err))
	}
	if len(devices) == 0 {
		return errors.New("no capture devices available")
	}

	// Order: OS-flagged default first, then everything else. This keeps the
	// happy path identical to before while letting us recover when the
	// "default" is a stale phantom (disconnected USB device on macOS).
	ordered := make([]malgo.DeviceInfo, 0, len(devices))
	for i := range devices {
		if devices[i].IsDefault != 0 {
			ordered = append(ordered, devices[i])
		}
	}
	for i := range devices {
		if devices[i].IsDefault == 0 {
			ordered = append(ordered, devices[i])
		}
	}

	c.samples = c.samples[:0]
	onRecv := func(_, pSample []byte, framecount uint32) {
		frames := int(framecount)
		c.mu.Lock()
		for i := range frames {
			s := int16(binary.LittleEndian.Uint16(pSample[i*2:]))
			c.samples = append(c.samples, float32(s)/32768.0)
		}
		c.mu.Unlock()
	}

	var attemptErrs []string
	for i := range ordered {
		dev, rate, err := tryOpenDevice(c.ctx.Context, &ordered[i], onRecv)
		if err != nil {
			attemptErrs = append(attemptErrs, fmt.Sprintf("%q: %v%s", ordered[i].Name(), err, malgoCodeSuffix(err)))
			continue
		}
		c.device = dev
		c.deviceRate = rate
		c.active = true
		return nil
	}
	return fmt.Errorf("no capture device could be opened: %s", strings.Join(attemptErrs, "; "))
}

// tryOpenDevice attempts to init and start capture on a specific device.
// Returns the live device and its negotiated sample rate, or an error.
// Cleans up the device on partial failure (Init succeeded but Start failed).
func tryOpenDevice(ctx malgo.Context, info *malgo.DeviceInfo, onRecv func([]byte, []byte, uint32)) (*malgo.Device, int, error) {
	devCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	devCfg.Capture.Format = malgo.FormatS16
	devCfg.Capture.Channels = 1
	devCfg.Capture.DeviceID = info.ID.Pointer()
	// SampleRate = 0 lets miniaudio use the device's native rate; the
	// caller resamples to CaptureSampleRate in Stop().
	devCfg.SampleRate = 0
	devCfg.Alsa.NoMMap = 1

	dev, err := malgo.InitDevice(ctx, devCfg, malgo.DeviceCallbacks{Data: onRecv})
	if err != nil {
		return nil, 0, err
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return nil, 0, err
	}
	return dev, int(dev.SampleRate()), nil
}

// malgoCodeSuffix renders " (code=N)" when err wraps a malgo.Result.
// The string description from miniaudio can be "Unknown error" for codes
// the C side doesn't recognize (e.g. Core Audio HAL errors on macOS),
// so the raw integer is the only diagnostic signal in those cases.
func malgoCodeSuffix(err error) string {
	var r malgo.Result
	if errors.As(err, &r) {
		return fmt.Sprintf(" (code=%d)", int32(r))
	}
	return ""
}

// Stop halts capture and returns samples resampled to CaptureSampleRate.
func (c *Capturer) Stop() ([]float32, error) {
	c.mu.Lock()
	if !c.active {
		c.mu.Unlock()
		return nil, errors.New("capture not active")
	}
	dev := c.device
	c.device = nil
	c.active = false
	raw := c.samples
	rate := c.deviceRate
	c.samples = nil
	c.deviceRate = 0
	c.mu.Unlock()

	if dev != nil {
		dev.Uninit()
	}
	return resampleLinear(raw, rate, CaptureSampleRate), nil
}

// resampleLinear converts mono float32 samples from inRate to outRate using
// linear interpolation. Speech transcription is forgiving enough that
// linear is fine — Whisper does its own internal resampling and is robust
// to mild aliasing in the input.
func resampleLinear(in []float32, inRate, outRate int) []float32 {
	if len(in) == 0 || inRate <= 0 || outRate <= 0 || inRate == outRate {
		return in
	}
	ratio := float64(inRate) / float64(outRate)
	outLen := int(float64(len(in)) / ratio)
	if outLen <= 0 {
		return nil
	}
	out := make([]float32, outLen)
	for i := range outLen {
		srcPos := float64(i) * ratio
		idx := int(srcPos)
		frac := float32(srcPos - float64(idx))
		if idx+1 >= len(in) {
			out[i] = in[len(in)-1]
			continue
		}
		out[i] = in[idx]*(1-frac) + in[idx+1]*frac
	}
	return out
}

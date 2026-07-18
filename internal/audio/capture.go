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

// Start begins capture from the device matching preferredDevice, or the
// system default when preferredDevice is empty. Re-resolves devices each
// call so the daemon survives a hotplug (e.g. USB headset disconnected,
// default reverts to built-in mic).
//
// On macOS, Core Audio can keep a disconnected USB device flagged as default
// until something forces a refresh — so if opening the first-choice device
// fails, this falls through to the remaining devices. Returns the name of
// the device that was actually opened; callers can compare it against
// preferredDevice (via MatchesDevice) to detect a fallback.
func (c *Capturer) Start(preferredDevice string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return "", errors.New("capture already active")
	}

	devices, err := c.ctx.Devices(malgo.Capture)
	if err != nil {
		return "", fmt.Errorf("enumerate capture devices: %w%s", err, malgoCodeSuffix(err))
	}
	if len(devices) == 0 {
		return "", errors.New("no capture devices available")
	}

	names := make([]string, len(devices))
	defaults := make([]bool, len(devices))
	for i := range devices {
		names[i] = devices[i].Name()
		defaults[i] = devices[i].IsDefault != 0
	}
	ordered := make([]malgo.DeviceInfo, 0, len(devices))
	for _, i := range orderPreference(names, defaults, preferredDevice) {
		ordered = append(ordered, devices[i])
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
		return ordered[i].Name(), nil
	}
	return "", fmt.Errorf("no capture device could be opened: %s", strings.Join(attemptErrs, "; "))
}

// MatchesDevice reports whether a device name satisfies a configured
// microphone preference. Matching is a case-insensitive substring test, so
// "usb" selects "USB PnP Sound Device". An empty preference matches nothing.
func MatchesDevice(name, preferred string) bool {
	if preferred == "" {
		return false
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(preferred))
}

// orderPreference returns device indices in open-attempt order: devices
// matching the preferred name first, then the OS-flagged default, then the
// rest in enumeration order. The default-then-rest tail lets capture recover
// when the first choice is a stale phantom (disconnected USB device on
// macOS) or the preferred device is currently unplugged.
func orderPreference(names []string, defaults []bool, preferred string) []int {
	order := make([]int, 0, len(names))
	taken := make([]bool, len(names))
	pick := func(match func(i int) bool) {
		for i := range names {
			if !taken[i] && match(i) {
				order = append(order, i)
				taken[i] = true
			}
		}
	}
	pick(func(i int) bool { return MatchesDevice(names[i], preferred) })
	pick(func(i int) bool { return defaults[i] })
	pick(func(i int) bool { return true })
	return order
}

// CaptureDevice describes an available input device.
type CaptureDevice struct {
	Name    string
	Default bool
}

// ListCaptureDevices enumerates input devices using a short-lived audio
// context. Intended for CLI use; the daemon enumerates through its
// long-lived Capturer instead.
func ListCaptureDevices() ([]CaptureDevice, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	devices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("enumerate capture devices: %w%s", err, malgoCodeSuffix(err))
	}
	out := make([]CaptureDevice, 0, len(devices))
	for i := range devices {
		out = append(out, CaptureDevice{
			Name:    devices[i].Name(),
			Default: devices[i].IsDefault != 0,
		})
	}
	return out, nil
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
	return ResampleLinear(raw, rate, CaptureSampleRate), nil
}

// ResampleLinear converts mono float32 samples from inRate to outRate using
// linear interpolation. Speech transcription is forgiving enough that
// linear is fine — Whisper does its own internal resampling and is robust
// to mild aliasing in the input.
func ResampleLinear(in []float32, inRate, outRate int) []float32 {
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

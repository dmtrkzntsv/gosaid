package daemon

type State int

const (
	StateIdle State = iota
	StateRecording
	StateTranscribing
	StateProcessing
	StateInjecting
	StateError
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateRecording:
		return "Recording"
	case StateTranscribing:
		return "Transcribing"
	case StateProcessing:
		return "Processing"
	case StateInjecting:
		return "Injecting"
	case StateError:
		return "Error"
	}
	return "Unknown"
}

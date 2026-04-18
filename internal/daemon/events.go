package daemon

type StateEvent struct {
	State    State
	Previous State
	Err      error
}

type Subscriber func(StateEvent)

package event

type SubSettings struct {}
type SubOption func(*SubSettings)

type EmitterSettings struct {}
type EmitterOption func(*EmitterSettings)

type Bus interface {
	// Subscribe creates new subscription. Failing to drain the incoming channel
	// will cause publishers to get blocked
	//
	// evtTypes only accepts typed nil pointers, and uses the type information to
	// select output type
	//
	// Example:
	// sub, cancel, err := eventbus.Subscribe(new(os.Signal))
	// defer cancel()
	//
	// evt := (<-sub).(os.Signal) // guaranteed to be safe
	Subscribe(eventType interface{}, opts ...SubOption) (<-chan interface{}, CancelFunc, error)


	Emitter(eventType interface{}, opts ...EmitterOption) (EmitFunc, CancelFunc, error)
}

// EmitFunc emits events. If any channel subscribed to the topic is blocked,
// calls to EmitFunc will block
//
// Calling this function with wrong event type will cause a panic
type EmitFunc func(event interface{})

type CancelFunc func()


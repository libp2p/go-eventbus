package event

import (
	"errors"
	"reflect"
)

type SubSettings struct {
	forcedType reflect.Type
}
type SubOption func(*SubSettings) error

func ForceSubType(evtType interface{}) SubOption {
	return func(s *SubSettings) error {
		typ := reflect.TypeOf(evtType)
		if typ.Kind() != reflect.Ptr {
			return errors.New("ForceSubType called with non-pointer type")
		}
		s.forcedType = typ
		return nil
	}
}

type EmitterSettings struct{
	makeStateful bool
}
type EmitterOption func(*EmitterSettings)

func Stateful(s *EmitterSettings) {
	s.makeStateful = true
}

type Bus interface {
	// Subscribe creates new subscription. Failing to drain the channel will cause
	// publishers to get blocked
	//
	// Example:
	// ch := make(chan EventT, 10)
	// defer close(ch)
	// cancel, err := eventbus.Subscribe(ch)
	// defer cancel()
	Subscribe(typedChan interface{}, opts ...SubOption) (CancelFunc, error)

	// Emitter creates new emitter
	//
	// eventType accepts typed nil pointers, and uses the type information to
	// select output type
	//
	// Example:
	// emit, cancel, err := eventbus.Emitter(new(EventT))
	// defer cancel()
	//
	// emit(EventT{})
	Emitter(eventType interface{}, opts ...EmitterOption) (EmitFunc, CancelFunc, error)
}

// EmitFunc emits events. If any channel subscribed to the topic is blocked,
// calls to EmitFunc will block
//
// Calling this function with wrong event type will cause a panic
type EmitFunc func(event interface{})

type CancelFunc func()

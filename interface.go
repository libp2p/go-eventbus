package event

import (
	"errors"
	"reflect"
)

var closeEmit struct{}

type subSettings struct {
	forcedType reflect.Type
}

type SubOption func(interface{}) error

// ForceSubType is a Subscribe option which overrides the type to which
// the subscription will be done. Note that the evtType must be assignable
// to channel type.
//
// This also allows for subscribing to multiple eventbus channels with one
// Go channel to get better ordering guarantees.
//
// Example:
// type Event struct{}
// func (Event) String() string {
//    return "event"
// }
//
// eventCh := make(chan fmt.Stringer) // interface { String() string }
// cancel, err := eventbus.Subscribe(eventCh, event.ForceSubType(new(Event)))
// [...]
func ForceSubType(evtType interface{}) SubOption {
	return func(settings interface{}) error {
		s := settings.(*subSettings)
		typ := reflect.TypeOf(evtType)
		if typ.Kind() != reflect.Ptr {
			return errors.New("ForceSubType called with non-pointer type")
		}
		s.forcedType = typ
		return nil
	}
}

type emitterSettings struct {
	makeStateful bool
}
type EmitterOption func(interface{}) error

// Stateful is an Emitter option which makes makes the eventbus channel
// 'remember' last event sent, and when a new subscriber joins the
// bus, the remembered event is immediately sent to the subscription
// channel.
//
// This allows to provide state tracking for dynamic systems, and/or
// allows new subscribers to verify that there are Emitters on the channel
func Stateful(s *emitterSettings) {
	s.makeStateful = true
}

// Bus is an interface to type-based event delivery system
type Bus interface {
	// Subscribe creates new subscription. Failing to drain the channel will cause
	// publishers to get blocked. CancelFunc is guaranteed to return after last send
	// to the channel
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
	// emit, err := eventbus.Emitter(new(EventT))
	// defer emit.Close() // MUST call this after being done with the emitter
	//
	// emit(EventT{})
	Emitter(eventType interface{}, opts ...EmitterOption) (EmitFunc, error)
}

// EmitFunc emits events. If any channel subscribed to the topic is blocked,
// calls to EmitFunc will block
//
// Calling this function with wrong event type will cause a panic
type EmitFunc func(event interface{})

func (f EmitFunc) Close() {
	f(closeEmit)
}

type CancelFunc func()

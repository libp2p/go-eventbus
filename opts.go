package event

import (
	"errors"
	"reflect"

	"github.com/libp2p/go-libp2p-core/event"
)

type subSettings struct {
	forcedType reflect.Type
}

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
func ForceSubType(evtType interface{}) event.SubscriptionOpt {
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

// Stateful is an Emitter option which makes makes the eventbus channel
// 'remember' last event sent, and when a new subscriber joins the
// bus, the remembered event is immediately sent to the subscription
// channel.
//
// This allows to provide state tracking for dynamic systems, and/or
// allows new subscribers to verify that there are Emitters on the channel
func Stateful(s interface{}) error {
	s.(*emitterSettings).makeStateful = true
	return nil
}

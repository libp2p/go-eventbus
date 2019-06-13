package event

import (
	"io"
)

type Bus interface {
	//Sub(...Opt) func(evtType interface{}, error)
	Sub(evtTypes interface{}) (Subscription, error)
	Emitter(evtTypes interface{}) (Emitter, error)
}

type Subscription interface {
	io.Closer

	Events() <-chan interface{}
}

type Emitter interface {
	io.Closer

	Emit(event interface{})
}



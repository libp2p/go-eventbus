package event

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

///////////////////////
// BUS

type bus struct {
	lk    sync.Mutex
	nodes map[string]*node
}

func NewBus() Bus {
	return &bus{
		nodes: map[string]*node{},
	}
}

func (b *bus) withNode(typ reflect.Type, cb func(*node)) error {
	path := typePath(typ)

	b.lk.Lock()

	n, ok := b.nodes[path]
	if !ok {
		n = newNode(typ)
		b.nodes[path] = n
	}

	n.lk.Lock()
	b.lk.Unlock()
	defer n.lk.Unlock()
	cb(n)
	return nil
}

func (b *bus) tryDropNode(typ reflect.Type) {
	path := typePath(typ)

	b.lk.Lock()
	n, ok := b.nodes[path]
	if !ok { // already dropped
		b.lk.Unlock()
		return
	}

	n.lk.Lock()
	if n.nEmitters > 0 || len(n.sinks) > 0 {
		n.lk.Unlock()
		b.lk.Unlock()
		return // still in use
	}
	n.lk.Unlock()

	delete(b.nodes, path)
	b.lk.Unlock()
}

func (b *bus) Subscribe(typedChan interface{}, opts ...SubOption) (c CancelFunc, err error) {
	var settings SubSettings
	for _, opt := range opts {
		if err := opt(&settings); err != nil {
			return nil, err
		}
	}

	refCh := reflect.ValueOf(typedChan)
	typ := refCh.Type()
	if typ.Kind() != reflect.Chan {
		return nil, errors.New("expected a channel")
	}
	if typ.ChanDir()&reflect.SendDir == 0 {
		return nil, errors.New("channel doesn't allow send")
	}

	if settings.forcedType != nil {
		if settings.forcedType.Elem().AssignableTo(typ) {
			return nil, fmt.Errorf("forced type %s cannot be sent to chan %s", settings.forcedType, typ)
		}
		typ = settings.forcedType
	}

	err = b.withNode(typ.Elem(), func(n *node) {
		// when all subs are waiting on this channel, setting this to 1 doesn't
		// really affect benchmarks
		n.sub(refCh)

		c = func() {
			n.lk.Lock()
			for i := 0; i < len(n.sinks); i++ {
				if n.sinks[i] == refCh {
					n.sinks[i] = n.sinks[len(n.sinks)-1]
					n.sinks = n.sinks[:len(n.sinks)-1]
					break
				}
			}
			tryDrop := len(n.sinks) == 0 && n.nEmitters == 0
			n.lk.Unlock()
			if tryDrop {
				b.tryDropNode(typ.Elem())
			}
		}
	})
	return
}

func (b *bus) Emitter(evtType interface{}, _ ...EmitterOption) (e EmitFunc, c CancelFunc, err error) {
	typ := reflect.TypeOf(evtType)
	if typ.Kind() != reflect.Ptr {
		return nil, nil, errors.New("emitter called with non-pointer type")
	}
	typ = typ.Elem()

	err = b.withNode(typ, func(n *node) {
		atomic.AddInt32(&n.nEmitters, 1)
		closed := false

		e = func(event interface{}) {
			if closed {
				panic("emitter is closed")
			}
			n.emit(event)
		}

		c = func() {
			closed = true
			if atomic.AddInt32(&n.nEmitters, -1) == 0 {
				b.tryDropNode(typ)
			}
		}
	})
	return
}

///////////////////////
// NODE

type node struct {
	// Note: make sure to NEVER lock bus.lk when this lock is held
	lk sync.RWMutex

	typ reflect.Type

	// emitter ref count
	nEmitters int32

	keepLast bool
	last     reflect.Value

	sinks []reflect.Value
}

func newNode(typ reflect.Type) *node {
	return &node{
		typ: typ,
	}
}

func (n *node) sub(outChan reflect.Value) {
	n.sinks = append(n.sinks, outChan)
}

func (n *node) emit(event interface{}) {
	eval := reflect.ValueOf(event)
	if eval.Type() != n.typ {
		panic(fmt.Sprintf("Emit called with wrong type. expected: %s, got: %s", n.typ, eval.Type()))
	}

	n.lk.RLock()
	// TODO: try using reflect.Select
	for _, ch := range n.sinks {
		ch.Send(eval)
	}
	n.lk.RUnlock()
}

///////////////////////
// UTILS

func typePath(t reflect.Type) string {
	return t.PkgPath() + "/" + t.String()
}

var _ Bus = &bus{}

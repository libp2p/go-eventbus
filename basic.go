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
	nodes map[reflect.Type]*node
}

func NewBus() Bus {
	return &bus{
		nodes: map[reflect.Type]*node{},
	}
}

func (b *bus) withNode(typ reflect.Type, cb func(*node), async func(*node)) error {
	b.lk.Lock()

	n, ok := b.nodes[typ]
	if !ok {
		n = newNode(typ)
		b.nodes[typ] = n
	}

	n.lk.Lock()
	b.lk.Unlock()

	cb(n)

	go func() {
		defer n.lk.Unlock()
		async(n)
	}()

	return nil
}

func (b *bus) tryDropNode(typ reflect.Type) {
	b.lk.Lock()
	n, ok := b.nodes[typ]
	if !ok { // already dropped
		b.lk.Unlock()
		return
	}

	n.lk.Lock()
	if atomic.LoadInt32(&n.nEmitters) > 0 || len(n.sinks) > 0 {
		n.lk.Unlock()
		b.lk.Unlock()
		return // still in use
	}
	n.lk.Unlock()

	delete(b.nodes, typ)
	b.lk.Unlock()
}

func (b *bus) Subscribe(typedChan interface{}, opts ...SubOption) (c CancelFunc, err error) {
	var settings subSettings
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
		n.sinks = append(n.sinks, refCh)
		c = func() {
			n.lk.Lock()
			for i := 0; i < len(n.sinks); i++ {
				if n.sinks[i] == refCh {
					n.sinks[i], n.sinks[len(n.sinks)-1] = n.sinks[len(n.sinks)-1], reflect.Value{}
					n.sinks = n.sinks[:len(n.sinks)-1]
					break
				}
			}
			tryDrop := len(n.sinks) == 0 && atomic.LoadInt32(&n.nEmitters) == 0
			n.lk.Unlock()
			if tryDrop {
				b.tryDropNode(typ.Elem())
			}
		}
	}, func(n *node) {
		if n.keepLast {
			lastVal, ok := n.last.Load().(reflect.Value)
			if !ok {
				return
			}

			refCh.Send(lastVal)
		}
	})
	return
}

func (b *bus) Emitter(evtType interface{}, opts ...EmitterOption) (e EmitFunc, err error) {
	var settings emitterSettings
	for _, opt := range opts {
		opt(&settings)
	}

	typ := reflect.TypeOf(evtType)
	if typ.Kind() != reflect.Ptr {
		return nil, errors.New("emitter called with non-pointer type")
	}
	typ = typ.Elem()

	err = b.withNode(typ, func(n *node) {
		atomic.AddInt32(&n.nEmitters, 1)
		closed := false
		n.keepLast = n.keepLast || settings.makeStateful

		e = func(event interface{}) {
			if closed {
				panic("emitter is closed")
			}
			if event == closeEmit {
				closed = true
				if atomic.AddInt32(&n.nEmitters, -1) == 0 {
					b.tryDropNode(typ)
				}
				return
			}
			n.emit(event)
		}
	}, func(_ *node) {})
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
	last     atomic.Value

	sinks []reflect.Value
}

func newNode(typ reflect.Type) *node {
	return &node{
		typ: typ,
	}
}

func (n *node) emit(event interface{}) {
	eval := reflect.ValueOf(event)
	if eval.Type() != n.typ {
		panic(fmt.Sprintf("Emit called with wrong type. expected: %s, got: %s", n.typ, eval.Type()))
	}

	n.lk.RLock()
	if n.keepLast {
		n.last.Store(eval)
	}

	for _, ch := range n.sinks {
		ch.Send(eval)
	}
	n.lk.RUnlock()
}

///////////////////////
// UTILS

var _ Bus = &bus{}

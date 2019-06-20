package eventbus

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p-core/event"
)

///////////////////////
// BUS

// basicBus is a type-based event delivery system
type basicBus struct {
	lk    sync.Mutex
	nodes map[reflect.Type]*node
}

var _ event.Bus = (*basicBus)(nil)

type Emitter struct {
	n       *node
	typ     reflect.Type
	closed  int32
	dropper func(reflect.Type)
}

func (e *Emitter) Emit(evt interface{}) {
	if atomic.LoadInt32(&e.closed) != 0 {
		panic("emitter is closed")
	}
	e.n.emit(evt)
}

func (e *Emitter) Close() error {
	if !atomic.CompareAndSwapInt32(&e.closed, 0, 1) {
		panic("closed an emitter more than once")
	}
	if atomic.AddInt32(&e.n.nEmitters, -1) == 0 {
		e.dropper(e.typ)
	}
	return nil
}

func NewBus() event.Bus {
	return &basicBus{
		nodes: map[reflect.Type]*node{},
	}
}

func (b *basicBus) withNode(typ reflect.Type, cb func(*node), async func(*node)) error {
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

func (b *basicBus) tryDropNode(typ reflect.Type) {
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

// Subscribe creates new subscription. Failing to drain the channel will cause
// publishers to get blocked. CancelFunc is guaranteed to return after last send
// to the channel
//
// Example:
// ch := make(chan EventT, 10)
// defer close(ch)
// cancel, err := eventbus.Subscribe(ch)
// defer cancel()
func (b *basicBus) Subscribe(typedChan interface{}, opts ...event.SubscriptionOpt) (c event.CancelFunc, err error) {
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
func (b *basicBus) Emitter(evtType interface{}, opts ...event.EmitterOpt) (e event.Emitter, err error) {
	var settings emitterSettings
	for _, opt := range opts {
		if err := opt(&settings); err != nil {
			return nil, err
		}
	}

	typ := reflect.TypeOf(evtType)
	if typ.Kind() != reflect.Ptr {
		return nil, errors.New("emitter called with non-pointer type")
	}
	typ = typ.Elem()

	err = b.withNode(typ, func(n *node) {
		atomic.AddInt32(&n.nEmitters, 1)
		n.keepLast = n.keepLast || settings.makeStateful
		e = &Emitter{n: n, typ: typ, dropper: b.tryDropNode}
	}, func(_ *node) {})
	return
}

///////////////////////
// NODE

type node struct {
	// Note: make sure to NEVER lock basicBus.lk when this lock is held
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

	sinks := make([]reflect.Value, len(n.sinks))
	copy(sinks, n.sinks)

	n.lk.RUnlock()

	for _, ch := range sinks {
		ch.Send(eval)
	}
}

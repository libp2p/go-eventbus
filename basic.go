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
	lk sync.Mutex
	nodes map[string]*node
}

func NewBus() Bus {
	return &bus{
		nodes: map[string]*node{},
	}
}

func (b *bus) withNode(evtType interface{}, cb func(*node)) error {
	typ := reflect.TypeOf(evtType)
	if typ.Kind() != reflect.Ptr {
		return errors.New("subscribe called with non-pointer type")
	}
	typ = typ.Elem()
	path := typePath(typ)

	b.lk.Lock()

	n, ok := b.nodes[path]
	if !ok {
		n = newNode(typ)
		b.nodes[path] = n
	}

	n.lk.Lock()
	b.lk.Unlock()
	cb(n)
	n.lk.Unlock()
	return nil
}

func (b *bus) tryDropNode(evtType interface{}) {
	path := typePath(reflect.TypeOf(evtType).Elem())

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

func (b *bus) Subscribe(evtType interface{}) (s <-chan interface{}, c CancelFunc, err error) {
	err = b.withNode(evtType, func(n *node) {
		out, i := n.sub(0)
		s = out
		c = func() {
			n.lk.Lock()
			delete(n.sinks, i)
			close(out)
			tryDrop := len(n.sinks) == 0 && n.nEmitters == 0
			n.lk.Unlock()
			if tryDrop {
				b.tryDropNode(evtType)
			}
		}
	})
	return
}

func (b *bus) Emitter(evtType interface{}) (e EmitFunc, c CancelFunc, err error) {
	err = b.withNode(evtType, func(n *node) {
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
				b.tryDropNode(evtType)
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

	nEmitters int32
	nSinks int
	sinks  map[int]chan interface{}
}

func newNode(typ reflect.Type) *node {
	return &node{
		typ: typ,

		sinks: map[int]chan interface{}{},
	}
}

func (n *node) sub(buf int) (chan interface{}, int) {
	out := make(chan interface{}, buf)
	i := n.nSinks
	n.nSinks++
	n.sinks[i] = out
	return out, i
}

func (n *node) emit(event interface{}) {
	etype := reflect.TypeOf(event)
	if etype != n.typ {
		panic(fmt.Sprintf("Emit called with wrong type. expected: %s, got: %s", n.typ, etype))
	}

	n.lk.RLock()
	for _, ch := range n.sinks {
		ch <- event
	}
	n.lk.RUnlock()
}

///////////////////////
// UTILS

func typePath(t reflect.Type) string {
	return t.PkgPath() + "/" + t.String()
}

var _ Bus = &bus{}

package event

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
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

func (b *bus) Sub(evtType interface{}) (s Subscription, err error) {
	err = b.withNode(evtType, func(n *node) {
		s = n.sub(0)
	})
	return
}

func (b *bus) Emitter(evtType interface{}) (e Emitter, err error) {
	err = b.withNode(evtType, func(n *node) {
		e = &emitter{
			Closer: closer(func(){}), //TODO: actually do something here
			node: n,
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

	n     int
	sinks map[int]chan interface{}
}

func newNode(typ reflect.Type) *node {
	return &node{
		typ: typ,

		sinks: map[int]chan interface{}{},
	}
}

func (n *node) sub(buf int) Subscription {
	out := make(chan interface{}, buf)
	n.n++
	n.sinks[n.n] = out
	return &sub{
		out: out,
	}
}

func (n *node) Emit(event interface{}) {
	etype := reflect.TypeOf(event)
	if etype != n.typ {
		panic(fmt.Sprintf("Emit called with wrong type. expected: %s, got: %s", n.typ, etype))
	}

	n.lk.RLock()
	for _, ch := range n.sinks {
		ch <- event
	}
}

///////////////////////
// SUB

type sub struct {
	io.Closer
	out <-chan interface{}
}

func (s *sub) Events() <-chan interface{} {
	return s.out
}

///////////////////////
// EMITTERS

type emitter struct {
	io.Closer
	*node
}

///////////////////////
// UTILS

func typePath(t reflect.Type) string {
	return t.PkgPath() + "/" + t.String()
}

type closer func()

func (c closer) Close() error {
	c()
	return nil
}

var _ Bus = &bus{}
var _ Subscription = &sub{}
var _ Emitter = &emitter{}
var _ io.Closer = closer(nil)
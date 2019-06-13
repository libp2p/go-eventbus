package event

import (
	"sync"
	"testing"
	"time"
)

type EventA struct{}
type EventB int

func TestEmit(t *testing.T) {
	bus := NewBus()
	events, cancel, err := bus.Subscribe(new(EventA))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer cancel()
		<-events
	}()

	emit, cancel, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	emit(EventA{})
}

func TestSub(t *testing.T) {
	bus := NewBus()
	events, cancel, err := bus.Subscribe(new(EventB))
	if err != nil {
		t.Fatal(err)
	}

	var event EventB

	var wait sync.WaitGroup
	wait.Add(1)

	go func() {
		defer cancel()
		event = (<-events).(EventB)
		wait.Done()
	}()

	emit, cancel, err := bus.Emitter(new(EventB))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	emit(EventB(7))
	wait.Wait()

	if event != 7 {
		t.Error("got wrong event")
	}
}

func TestEmitNoSubNoBlock(t *testing.T) {
	bus := NewBus()

	emit, cancel, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	emit(EventA{})
}

func TestEmitOnClosed(t *testing.T) {
	bus := NewBus()

	emit, cancel, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("expected panic")
		}
		if r.(string) != "emitter is closed" {
			t.Error("unexpected message")
		}
	}()

	emit(EventA{})
}

func TestClosingRaces(t *testing.T) {
	subs := 50000
	emits := 50000

	var wg sync.WaitGroup
	var lk sync.RWMutex
	lk.Lock()

	wg.Add(subs + emits)

	bus := NewBus()

	for i := 0; i < subs; i++ {
		go func() {
			lk.RLock()
			defer lk.RUnlock()

			_, cancel, _ := bus.Subscribe(new(EventA))
			time.Sleep(10 * time.Millisecond)
			cancel()

			wg.Done()
		}()
	}
	for i := 0; i < emits; i++ {
		go func() {
			lk.RLock()
			defer lk.RUnlock()

			_, cancel, _ := bus.Emitter(new(EventA))
			time.Sleep(10 * time.Millisecond)
			cancel()

			wg.Done()
		}()
	}

	time.Sleep(10 * time.Millisecond)
	lk.Unlock() // start everything

	wg.Wait()
}

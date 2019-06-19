package event

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jbenet/go-detect-race"
)

type EventA struct{}
type EventB int

func getN() int {
	n := 50000
	if detectrace.WithRace() {
		n = 1000
	}
	return n
}

func (EventA) String() string {
	return "Oh, Hello"
}

func TestEmit(t *testing.T) {
	bus := NewBus()
	events := make(chan EventA)
	cancel, err := bus.Subscribe(events)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer cancel()
		<-events
	}()

	emit, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	emit(EventA{})
}

func TestSub(t *testing.T) {
	bus := NewBus()
	events := make(chan EventB)
	cancel, err := bus.Subscribe(events)
	if err != nil {
		t.Fatal(err)
	}

	var event EventB

	var wait sync.WaitGroup
	wait.Add(1)

	go func() {
		defer cancel()
		event = <-events
		wait.Done()
	}()

	emit, err := bus.Emitter(new(EventB))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	emit(EventB(7))
	wait.Wait()

	if event != 7 {
		t.Error("got wrong event")
	}
}

func TestEmitNoSubNoBlock(t *testing.T) {
	bus := NewBus()

	emit, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	emit(EventA{})
}

func TestEmitOnClosed(t *testing.T) {
	bus := NewBus()

	emit, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	emit.Close()

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
	subs := getN()
	emits := getN()

	var wg sync.WaitGroup
	var lk sync.RWMutex
	lk.Lock()

	wg.Add(subs + emits)

	b := NewBus()

	for i := 0; i < subs; i++ {
		go func() {
			lk.RLock()
			defer lk.RUnlock()

			cancel, _ := b.Subscribe(make(chan EventA))
			time.Sleep(10 * time.Millisecond)
			cancel()

			wg.Done()
		}()
	}
	for i := 0; i < emits; i++ {
		go func() {
			lk.RLock()
			defer lk.RUnlock()

			emit, _ := b.Emitter(new(EventA))
			time.Sleep(10 * time.Millisecond)
			emit.Close()

			wg.Done()
		}()
	}

	time.Sleep(10 * time.Millisecond)
	lk.Unlock() // start everything

	wg.Wait()

	if len(b.nodes) != 0 {
		t.Error("expected no nodes")
	}
}

func TestSubMany(t *testing.T) {
	bus := NewBus()

	var r int32

	n := getN()
	var wait sync.WaitGroup
	var ready sync.WaitGroup
	wait.Add(n)
	ready.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			events := make(chan EventB)
			cancel, err := bus.Subscribe(events)
			if err != nil {
				panic(err)
			}
			defer cancel()

			ready.Done()
			atomic.AddInt32(&r, int32(<-events))
			wait.Done()
		}()
	}

	emit, err := bus.Emitter(new(EventB))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	ready.Wait()

	emit(EventB(7))
	wait.Wait()

	if int(r) != 7*n {
		t.Error("got wrong result")
	}
}

func TestSubType(t *testing.T) {
	bus := NewBus()
	events := make(chan fmt.Stringer)
	cancel, err := bus.Subscribe(events, ForceSubType(new(EventA)))
	if err != nil {
		t.Fatal(err)
	}

	var event fmt.Stringer

	var wait sync.WaitGroup
	wait.Add(1)

	go func() {
		defer cancel()
		event = <-events
		wait.Done()
	}()

	emit, err := bus.Emitter(new(EventA))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	emit(EventA{})
	wait.Wait()

	if event.String() != "Oh, Hello" {
		t.Error("didn't get the correct message")
	}
}

func TestNonStateful(t *testing.T) {
	bus := NewBus()
	emit, err := bus.Emitter(new(EventB))
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	eventsA := make(chan EventB, 1)
	cancelS, err := bus.Subscribe(eventsA)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelS()

	select {
	case <-eventsA:
		t.Fatal("didn't expect to get an event")
	default:
	}

	emit(EventB(1))

	select {
	case e := <-eventsA:
		if e != 1 {
			t.Fatal("got wrong event")
		}
	default:
		t.Fatal("expected to get an event")
	}

	eventsB := make(chan EventB, 1)
	cancelS2, err := bus.Subscribe(eventsB)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelS2()

	select {
	case <-eventsA:
		t.Fatal("didn't expect to get an event")
	default:
	}
}

func TestStateful(t *testing.T) {
	bus := NewBus()
	emit, err := bus.Emitter(new(EventB), Stateful)
	if err != nil {
		t.Fatal(err)
	}
	defer emit.Close()

	emit(EventB(2))

	eventsA := make(chan EventB, 1)
	cancelS, err := bus.Subscribe(eventsA)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelS()

	if <-eventsA != 2 {
		t.Fatal("got wrong event")
	}
}

func testMany(t testing.TB, subs, emits, msgs int, stateful bool) {
	if detectrace.WithRace() && subs+emits > 5000 {
		t.SkipNow()
	}

	bus := NewBus()

	var r int64

	var wait sync.WaitGroup
	var ready sync.WaitGroup
	wait.Add(subs + emits)
	ready.Add(subs)

	for i := 0; i < subs; i++ {
		go func() {
			events := make(chan EventB)
			cancel, err := bus.Subscribe(events)
			if err != nil {
				panic(err)
			}
			defer cancel()

			ready.Done()
			for i := 0; i < emits*msgs; i++ {
				atomic.AddInt64(&r, int64(<-events))
			}
			wait.Done()
		}()
	}

	for i := 0; i < emits; i++ {
		go func() {
			emit, err := bus.Emitter(new(EventB), func(settings *emitterSettings) {
				settings.makeStateful = stateful
			})
			if err != nil {
				panic(err)
			}
			defer emit.Close()

			ready.Wait()

			for i := 0; i < msgs; i++ {
				emit(EventB(97))
			}

			wait.Done()
		}()
	}

	wait.Wait()

	if int(r) != 97*subs*emits*msgs {
		t.Fatal("got wrong result")
	}
}

func TestBothMany(t *testing.T) {
	testMany(t, 10000, 100, 10, false)
}

func BenchmarkSubs(b *testing.B) {
	b.ReportAllocs()
	testMany(b, b.N, 100, 100, false)
}

func BenchmarkEmits(b *testing.B) {
	b.ReportAllocs()
	testMany(b, 100, b.N, 100, false)
}

func BenchmarkMsgs(b *testing.B) {
	b.ReportAllocs()
	testMany(b, 100, 100, b.N, false)
}

func BenchmarkOneToMany(b *testing.B) {
	b.ReportAllocs()
	testMany(b, b.N, 1, 100, false)
}

func BenchmarkManyToOne(b *testing.B) {
	b.ReportAllocs()
	testMany(b, 1, b.N, 100, false)
}

func BenchmarkMs1e2m4(b *testing.B) {
	b.N = 1000000
	b.ReportAllocs()
	testMany(b, 10, 100, 10000, false)
}

func BenchmarkMs1e0m6(b *testing.B) {
	b.N = 10000000
	b.ReportAllocs()
	testMany(b, 10, 1, 1000000, false)
}

func BenchmarkMs0e0m6(b *testing.B) {
	b.N = 1000000
	b.ReportAllocs()
	testMany(b, 1, 1, 1000000, false)
}

func BenchmarkStatefulMs1e0m6(b *testing.B) {
	b.N = 10000000
	b.ReportAllocs()
	testMany(b, 10, 1, 1000000, true)
}

func BenchmarkStatefulMs0e0m6(b *testing.B) {
	b.N = 1000000
	b.ReportAllocs()
	testMany(b, 1, 1, 1000000, true)
}

func BenchmarkMs0e6m0(b *testing.B) {
	b.N = 1000000
	b.ReportAllocs()
	testMany(b, 1, 1000000, 1, false)
}

func BenchmarkMs6e0m0(b *testing.B) {
	b.N = 1000000
	b.ReportAllocs()
	testMany(b, 1000000, 1, 1, false)
}

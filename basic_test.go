package event

import (
	"testing"
)

type EventA struct{}

func TestSimple(t *testing.T) {
	bus := NewBus()
	_, _ = bus.Sub(new(EventA))

}

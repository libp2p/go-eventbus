// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/p2p/host/eventbus.
package eventbus

import (
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/p2p/host/eventbus"
)

// Deprecated: Use github.com/libp2p/go-libp2p/p2p/host/eventbus.NewBus instead.
func NewBus() event.Bus {
	return eventbus.NewBus()
}

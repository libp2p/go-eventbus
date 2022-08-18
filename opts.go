package eventbus

import "github.com/libp2p/go-libp2p/p2p/host/eventbus"

// Deprecated: Use github.com/libp2p/go-libp2p/p2p/host/eventbus.BufSize instead.
func BufSize(n int) func(interface{}) error {
	return eventbus.BufSize(n)
}

// Stateful is an Emitter option which makes the eventbus channel
// 'remember' last event sent, and when a new subscriber joins the
// bus, the remembered event is immediately sent to the subscription
// channel.
//
// This allows to provide state tracking for dynamic systems, and/or
// allows new subscribers to verify that there are Emitters on the channel
// Deprecated: Use github.com/libp2p/go-libp2p/p2p/host/eventbus.Stateful instead.
func Stateful(s interface{}) error {
	return eventbus.Stateful(s)
}

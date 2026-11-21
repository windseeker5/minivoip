package main

import "testing"

// The relay keeps the router's ports unless `task dev` moves them; a bad value
// changes nothing.
func TestRelayPorts(t *testing.T) {
	for _, c := range []struct {
		port, ports     string
		want, low, high uint16
	}{
		{"", "", 3478, 49160, 49200},
		{"3479", "49210-49250", 3479, 49210, 49250},
		{"x", "49250-49210", 3478, 49160, 49200},
		{"0", "nope", 3478, 49160, 49200},
	} {
		t.Setenv("MINIVOIP_RELAY_PORT", c.port)
		t.Setenv("MINIVOIP_RELAY_PORTS", c.ports)
		if p, l, h := relayPorts(); p != c.want || l != c.low || h != c.high {
			t.Errorf("%q %q: %d %d-%d", c.port, c.ports, p, l, h)
		}
	}
}

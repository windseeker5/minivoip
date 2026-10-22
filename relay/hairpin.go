package relay

import (
	"net"

	"github.com/pion/turn/v4"
)

// hairpin wraps the relay's port generator so that voice sent from one of
// our relay ports to another of our relay ports (both callers using the
// relay) is delivered on this machine, not sent out to the router and back.
// Routers that rewrite such "hairpin" packets (Ken's TELUS hub gives them
// its own LAN address) would make the relay drop them. The old coturn did
// the same thing with its external-ip setting.
type hairpin struct {
	turn.RelayAddressGenerator
	public           net.IP
	minPort, maxPort int
}

func (h *hairpin) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	conn, addr, err := h.RelayAddressGenerator.AllocatePacketConn(network, requestedPort)
	if err != nil {
		return nil, nil, err
	}
	return &hairpinConn{PacketConn: conn, h: h}, addr, nil
}

func (h *hairpin) ours(port int) bool { return port >= h.minPort && port <= h.maxPort }

type hairpinConn struct {
	net.PacketConn
	h *hairpin
}

// WriteTo sends to our own public address on this machine instead.
func (c *hairpinConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if u, ok := addr.(*net.UDPAddr); ok && u.IP.Equal(c.h.public) && c.h.ours(u.Port) {
		addr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: u.Port}
	}
	return c.PacketConn.WriteTo(p, addr)
}

// ReadFrom shows packets from our own relay ports as coming from the public
// address, which is what the callers asked permission for.
func (c *hairpinConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(p)
	if u, ok := addr.(*net.UDPAddr); ok && u.IP.IsLoopback() && c.h.ours(u.Port) {
		addr = &net.UDPAddr{IP: c.h.public, Port: u.Port}
	}
	return n, addr, err
}

package relay

import (
	"net"
	"sync"
	"time"

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
	live             *livePorts
	maxRate          int // bytes a second, each way, per relay connection
	total            *totalMeter
}

// totalMeter is the whole relay's share of the home line, each way.
type totalMeter struct {
	limit   int
	out, in meter
}

// livePorts are the relay's ports in use now. Only these can be reached
// through our own public address: anything else there would be this machine
// or the router, seen from inside.
type livePorts struct {
	mu    sync.Mutex
	ports map[int]bool
}

func (l *livePorts) set(port int, on bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if on {
		l.ports[port] = true
	} else {
		delete(l.ports, port)
	}
}

func (l *livePorts) has(port int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ports[port]
}

// meter counts bytes per second; over the limit they're dropped, as a full
// network link would. A relay connection is for one person's voice and video.
type meter struct {
	mu    sync.Mutex
	start time.Time
	n     int
}

func (m *meter) ok(n, limit int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if now.Sub(m.start) >= time.Second {
		m.start, m.n = now, 0
	}
	m.n += n
	return m.n <= limit
}

func (h *hairpin) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	conn, addr, err := h.RelayAddressGenerator.AllocatePacketConn(network, requestedPort)
	if err != nil {
		return nil, nil, err
	}
	port := 0
	if u, ok := addr.(*net.UDPAddr); ok {
		port = u.Port
	}
	h.live.set(port, true)
	return &hairpinConn{PacketConn: conn, h: h, port: port}, addr, nil
}

func (h *hairpin) ours(port int) bool { return port >= h.minPort && port <= h.maxPort }

type hairpinConn struct {
	net.PacketConn
	h       *hairpin
	port    int
	out, in meter
}

func (c *hairpinConn) Close() error {
	c.h.live.set(c.port, false)
	return c.PacketConn.Close()
}

// WriteTo sends to our own public address on this machine instead, and only
// to another live relay port; the rest of that address is refused. Over the
// rate limit, the packet is dropped.
func (c *hairpinConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if !c.out.ok(len(p), c.h.maxRate) || !c.h.total.out.ok(len(p), c.h.total.limit) {
		return len(p), nil
	}
	if u, ok := addr.(*net.UDPAddr); ok && u.IP.Equal(c.h.public) {
		if !c.h.ours(u.Port) || !c.h.live.has(u.Port) {
			return len(p), nil
		}
		addr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: u.Port}
	}
	return c.PacketConn.WriteTo(p, addr)
}

// ReadFrom shows packets from our own relay ports as coming from the public
// address, which is what the callers asked permission for.
func (c *hairpinConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(p)
		if err == nil && (!c.in.ok(n, c.h.maxRate) || !c.h.total.in.ok(n, c.h.total.limit)) {
			continue
		}
		if u, ok := addr.(*net.UDPAddr); ok && u.IP.IsLoopback() && c.h.ours(u.Port) && c.h.live.has(u.Port) {
			addr = &net.UDPAddr{IP: c.h.public, Port: u.Port}
		}
		return n, addr, err
	}
}

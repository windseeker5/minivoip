// Package relay is the built-in TURN relay (pion/turn). It carries a call's
// voice when the two browsers can't reach each other directly (phones on
// cellular data, strict Wi-Fi). It replaces the proof of concept's coturn and
// uses the same ports the router already forwards: 3478 UDP+TCP for the relay
// itself and 49160-49200 UDP for the voice.
//
// Logins are short-lived (the "TURN REST" scheme): the server hands each
// browser a username "<expiry>:<who>" and an HMAC password made with a
// secret only this server knows.
package relay

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

type Config struct {
	PublicIP net.IP // the address browsers reach the relay at (the router's)
	Port     int    // 3478
	MinPort  uint16 // voice ports, forwarded by the router
	MaxPort  uint16
	Realm    string
	Secret   string
	// Listen is the local address to bind ("" = all interfaces).
	Listen string
	// MaxPerAddress, MaxPerLogin and MaxTotal cap the relay connections open at
	// one time (MaxPerAddressDefault, MaxPerLoginDefault and MaxTotalDefault
	// when zero).
	MaxPerAddress, MaxPerLogin, MaxTotal int
	// MaxRate caps each relay connection and MaxTotalRate all of them together,
	// in bytes a second each way (MaxRateDefault and MaxTotalRateDefault when
	// zero).
	MaxRate, MaxTotalRate int
	// MaxPeers caps how many different addresses one client may reach at a
	// time (MaxPeersDefault when zero). Each one costs memory on the server.
	MaxPeers int
}

// A browser opens two relay connections for each person it talks to, so a
// full meeting room is 6 from one address. The voice ports are 41: the total
// stays under that.
const (
	MaxPerAddressDefault = 12
	MaxTotalDefault      = 36
	// One login is given for one person: a full room, plus a reconnect. So one
	// answered call can't take the whole relay from several addresses.
	MaxPerLoginDefault = 8
	// About 8 Mbit/s: a camera plus a screen, with room to spare.
	MaxRateDefault = 1 << 20
	// About 48 Mbit/s: what the whole relay may send or take from the home line.
	MaxTotalRateDefault = 6 << 20
	// A real call tries a handful of addresses.
	MaxPeersDefault = 200
	// Open TCP connections to the relay, and how long one may stay silent.
	maxTCPPerAddress = 20
	maxTCPTotal      = 500
	tcpIdleLimit     = 10 * time.Minute
	// How long a peer address counts: a TURN permission lives 5 minutes.
	peerLife = 5 * time.Minute
)

// peerLimit caps the different addresses each client can open the relay to.
type peerLimit struct {
	max int

	mu    sync.Mutex
	seen  map[string]map[string]time.Time
	swept time.Time
}

func (l *peerLimit) allow(src net.Addr, ip net.IP) bool {
	if !publicPeer(src, ip) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.swept) > peerLife {
		l.swept = now
		for client, peers := range l.seen {
			for p, at := range peers {
				if now.Sub(at) > peerLife {
					delete(peers, p)
				}
			}
			if len(peers) == 0 {
				delete(l.seen, client)
			}
		}
	}
	client := host(src)
	peers := l.seen[client]
	if peers == nil {
		peers = map[string]time.Time{}
		l.seen[client] = peers
	}
	key := ip.String()
	if at, ok := peers[key]; ok && now.Sub(at) <= peerLife {
		peers[key] = now
		return true
	}
	for p, at := range peers {
		if now.Sub(at) > peerLife {
			delete(peers, p)
		}
	}
	if len(peers) >= l.max {
		return false
	}
	peers[key] = now
	return true
}

// tcpListener keeps idle connections from piling up, and survives an accept
// error (too many open files, say) instead of ending TCP relaying.
type tcpListener struct {
	net.Listener

	mu   sync.Mutex
	open map[string]int
	all  int
}

func (l *tcpListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil, err
		}
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		key := host(c.RemoteAddr())
		l.mu.Lock()
		full := l.open[key] >= maxTCPPerAddress || l.all >= maxTCPTotal
		if !full {
			l.open[key]++
			l.all++
		}
		l.mu.Unlock()
		if full {
			c.Close()
			continue
		}
		return &tcpConn{Conn: c, l: l, key: key}, nil
	}
}

type tcpConn struct {
	net.Conn
	l    *tcpListener
	key  string
	once sync.Once
}

func (c *tcpConn) Read(p []byte) (int, error) {
	c.Conn.SetReadDeadline(time.Now().Add(tcpIdleLimit))
	return c.Conn.Read(p)
}

func (c *tcpConn) Close() error {
	c.once.Do(func() {
		c.l.mu.Lock()
		defer c.l.mu.Unlock()
		if c.l.open[c.key] > 1 {
			c.l.open[c.key]--
		} else {
			delete(c.l.open, c.key)
		}
		c.l.all--
	})
	return c.Conn.Close()
}

// quota counts the relay connections open, per address, per login and in total.
type quota struct {
	perAddress, perLogin, total int

	mu     sync.Mutex
	open   map[string]int
	logins map[string]int
	all    int
}

func host(addr net.Addr) string {
	h, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return h
}

func (q *quota) allow(login, _ string, src net.Addr) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.all < q.total && q.open[host(src)] < q.perAddress && q.logins[login] < q.perLogin
}

func (q *quota) created(src, _ net.Addr, _, login, _ string, _ net.Addr, _ int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.open[host(src)]++
	q.logins[login]++
	q.all++
}

func (q *quota) deleted(src, _ net.Addr, _, login, _ string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.open[host(src)] > 1 {
		q.open[host(src)]--
	} else {
		delete(q.open, host(src))
	}
	if q.logins[login] > 1 {
		q.logins[login]--
	} else {
		delete(q.logins, login)
	}
	q.all--
}

type Relay struct {
	cfg  Config
	srv  *turn.Server
	addr net.Addr
}

// Addr is the relay's UDP address (useful when Config.Port was 0).
func (r *Relay) Addr() net.Addr { return r.addr }

// Start opens the relay's UDP and TCP listeners.
func Start(cfg Config) (*Relay, error) {
	addr := net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.Port))
	udp, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("relay udp %s: %w", addr, err)
	}
	tcp, err := net.Listen("tcp4", addr)
	if err != nil {
		udp.Close()
		return nil, fmt.Errorf("relay tcp %s: %w", addr, err)
	}
	rate := cfg.MaxRate
	if rate == 0 {
		rate = MaxRateDefault
	}
	totalRate := cfg.MaxTotalRate
	if totalRate == 0 {
		totalRate = MaxTotalRateDefault
	}
	total := &totalMeter{limit: totalRate}
	live := &livePorts{ports: map[int]bool{}}
	gen := func() turn.RelayAddressGenerator {
		bind := cfg.Listen
		if bind == "" {
			bind = "0.0.0.0"
		}
		return &hairpin{
			RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{RelayAddress: cfg.PublicIP, Address: bind, MinPort: cfg.MinPort, MaxPort: cfg.MaxPort},
			public:                cfg.PublicIP, minPort: int(cfg.MinPort), maxPort: int(cfg.MaxPort), live: live, maxRate: rate, total: total,
		}
	}
	q := &quota{perAddress: cfg.MaxPerAddress, perLogin: cfg.MaxPerLogin, total: cfg.MaxTotal, open: map[string]int{}, logins: map[string]int{}}
	if q.perAddress == 0 {
		q.perAddress = MaxPerAddressDefault
	}
	if q.perLogin == 0 {
		q.perLogin = MaxPerLoginDefault
	}
	if q.total == 0 {
		q.total = MaxTotalDefault
	}
	peers := &peerLimit{max: cfg.MaxPeers, seen: map[string]map[string]time.Time{}}
	if peers.max == 0 {
		peers.max = MaxPeersDefault
	}
	// The relay's own log is off: strangers could fill the disk with bad logins.
	factory := logging.NewDefaultLoggerFactory()
	factory.DefaultLogLevel = logging.LogLevelDisabled
	logger := factory.NewLogger("relay")
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             cfg.Realm,
		LoggerFactory:     factory,
		AuthHandler:       turn.LongTermTURNRESTAuthHandler(cfg.Secret, logger),
		QuotaHandler:      q.allow,
		EventHandler:      turn.EventHandler{OnAllocationCreated: q.created, OnAllocationDeleted: q.deleted},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen(), PermissionHandler: peers.allow}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: &tcpListener{Listener: tcp, open: map[string]int{}}, RelayAddressGenerator: gen(), PermissionHandler: peers.allow}},
	})
	if err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	log.Printf("relay: listening on %s (udp+tcp), public %s, voice ports %d-%d", addr, cfg.PublicIP, cfg.MinPort, cfg.MaxPort)
	return &Relay{cfg: cfg, srv: srv, addr: udp.LocalAddr()}, nil
}

func (r *Relay) Close() error { return r.srv.Close() }

// publicPeer refuses to relay to private and local addresses, so the relay
// can't be used to reach the home network (same rule as the old coturn).
func publicPeer(_ net.Addr, ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, n := range notPublic {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// notPublic is what Go's checks above leave out: "this network", carrier NAT,
// benchmarking and reserved addresses.
var notPublic = []*net.IPNet{
	{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
	{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},
	{IP: net.IPv4(198, 18, 0, 0), Mask: net.CIDRMask(15, 32)},
	{IP: net.IPv4(240, 0, 0, 0), Mask: net.CIDRMask(4, 32)},
}

// ICEServer is one entry of RTCPeerConnection's iceServers.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// STUNServer is only the address helper, for a browser with no relay login.
func STUNServer(host string, port int) []ICEServer {
	return []ICEServer{{URLs: []string{"stun:" + host + ":" + strconv.Itoa(port)}}}
}

// ICEServers gives a browser STUN plus a relay login valid for two hours.
// host is the name browsers use to reach this server (voip.dresdell.com).
func ICEServers(secret, host string, port int, who string) []ICEServer {
	p := strconv.Itoa(port)
	// Each login is its own: the relay counts connections per login.
	b := make([]byte, 6)
	rand.Read(b)
	user, pass, err := turn.GenerateLongTermTURNRESTCredentials(secret, who+"-"+hex.EncodeToString(b), 2*time.Hour)
	stun := ICEServer{URLs: []string{"stun:" + host + ":" + p}}
	if err != nil {
		return []ICEServer{stun}
	}
	return []ICEServer{stun, {
		URLs:       []string{"turn:" + host + ":" + p + "?transport=udp", "turn:" + host + ":" + p + "?transport=tcp"},
		Username:   user,
		Credential: pass,
	}}
}

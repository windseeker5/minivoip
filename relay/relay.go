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
	"fmt"
	"log"
	"net"
	"strconv"
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
	gen := func() turn.RelayAddressGenerator {
		bind := cfg.Listen
		if bind == "" {
			bind = "0.0.0.0"
		}
		return &hairpin{
			RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{RelayAddress: cfg.PublicIP, Address: bind, MinPort: cfg.MinPort, MaxPort: cfg.MaxPort},
			public:                cfg.PublicIP, minPort: int(cfg.MinPort), maxPort: int(cfg.MaxPort),
		}
	}
	logger := logging.NewDefaultLoggerFactory().NewLogger("relay")
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             cfg.Realm,
		AuthHandler:       turn.LongTermTURNRESTAuthHandler(cfg.Secret, logger),
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen(), PermissionHandler: publicPeer}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: tcp, RelayAddressGenerator: gen(), PermissionHandler: publicPeer}},
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
	return !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() ||
		ip.IsMulticast() || cgnat.Contains(ip))
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// ICEServer is one entry of RTCPeerConnection's iceServers.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICEServers gives a browser STUN plus a relay login valid for two hours.
// host is the name browsers use to reach this server (voip.dresdell.com).
func ICEServers(secret, host string, port int, who string) []ICEServer {
	p := strconv.Itoa(port)
	user, pass, err := turn.GenerateLongTermTURNRESTCredentials(secret, who, 2*time.Hour)
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

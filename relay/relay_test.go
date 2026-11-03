package relay

import (
	"net"
	"testing"
	"time"

	"github.com/pion/turn/v4"
)

func allocate(t *testing.T, server, user, pass string) error {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: server, TURNServerAddr: server, Conn: conn,
		Username: user, Password: pass, Realm: "test", RTO: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	relayConn, err := c.Allocate()
	if err != nil {
		return err
	}
	relayConn.Close()
	return nil
}

func TestRelayNeedsAValidLogin(t *testing.T) {
	r, err := Start(Config{PublicIP: net.IPv4(127, 0, 0, 1), Listen: "127.0.0.1", Port: 0, MinPort: 50100, MaxPort: 50150, Realm: "test", Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	server := r.Addr().String()

	ice := ICEServers("s3cret", "voip.example", 3478, "visitor")
	if len(ice) != 2 || ice[1].Username == "" || ice[1].URLs[0] != "turn:voip.example:3478?transport=udp" {
		t.Fatalf("ice servers: %+v", ice)
	}
	if err := allocate(t, server, ice[1].Username, ice[1].Credential); err != nil {
		t.Fatalf("a valid login should get a relay: %v", err)
	}
	if err := allocate(t, server, ice[1].Username, "wrong"); err == nil {
		t.Fatal("a wrong password must be refused")
	}
}

// One packet used to crash the whole server (GO-2026-6163): a Send indication
// ending in an empty XOR-PEER-ADDRESS, from anyone with a free relay login. If
// it comes back, this test panics.
func TestBadPacketDoesNotCrash(t *testing.T) {
	r, err := Start(Config{PublicIP: net.IPv4(127, 0, 0, 1), Listen: "127.0.0.1", Port: 0, MinPort: 50200, MaxPort: 50210, Realm: "test", Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	server := r.Addr().(*net.UDPAddr)
	ice := ICEServers("s3cret", "voip.example", 3478, "visitor")

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c, err := turn.NewClient(&turn.ClientConfig{STUNServerAddr: server.String(), TURNServerAddr: server.String(), Conn: conn,
		Username: ice[1].Username, Password: ice[1].Credential, Realm: "test", RTO: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	relayConn, err := c.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	defer relayConn.Close()

	// Send indication (0x0016), 12 bytes of attributes, the magic cookie, a
	// transaction id, DATA, then XOR-PEER-ADDRESS (0x0012) with no value. 32
	// bytes in all fills the server's copy exactly, so reading the empty value
	// runs off its end.
	packet := append([]byte{0x00, 0x16, 0x00, 0x0c, 0x21, 0x12, 0xa4, 0x42}, make([]byte, 12)...)
	packet = append(packet, 0x00, 0x13, 0x00, 0x04, 1, 2, 3, 4, 0x00, 0x12, 0x00, 0x00)
	if _, err := conn.WriteTo(packet, server); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := allocate(t, server.String(), ice[1].Username, ice[1].Credential); err != nil {
		t.Fatalf("the relay stopped answering: %v", err)
	}
}

func TestRelayQuota(t *testing.T) {
	r, err := Start(Config{PublicIP: net.IPv4(127, 0, 0, 1), Listen: "127.0.0.1", Port: 0, MinPort: 50300, MaxPort: 50340, Realm: "test", Secret: "s3cret", MaxPerAddress: 2, MaxTotal: 40})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ice := ICEServers("s3cret", "voip.example", 3478, "visitor")

	hold := func() (net.PacketConn, error) {
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		c, err := turn.NewClient(&turn.ClientConfig{STUNServerAddr: r.Addr().String(), TURNServerAddr: r.Addr().String(), Conn: conn,
			Username: ice[1].Username, Password: ice[1].Credential, Realm: "test", RTO: 200 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		if err := c.Listen(); err != nil {
			t.Fatal(err)
		}
		return c.Allocate()
	}
	first, err := hold()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold(); err != nil {
		t.Fatal(err)
	}
	if _, err := hold(); err == nil {
		t.Fatal("a third relay from one address should be refused")
	}
	// Closing one makes room again.
	first.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := hold(); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no room after one closed")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestPrivatePeersRefused(t *testing.T) {
	for ip, want := range map[string]bool{"203.0.113.10": true, "192.168.0.20": false, "127.0.0.1": false, "100.64.1.1": false, "10.1.2.3": false,
		"0.1.2.3": false, "198.18.0.1": false, "240.0.0.1": false, "169.254.169.254": false} {
		if got := publicPeer(nil, net.ParseIP(ip)); got != want {
			t.Errorf("%s: %v", ip, got)
		}
	}
}

// Through our own public address a client reaches only live relay ports: not
// other programs on this machine, not the router.
// One login can't take the whole relay from many addresses, each login is its
// own, and a connection closing gives its place back.
func TestRelayQuotaPerLogin(t *testing.T) {
	q := &quota{perAddress: 100, perLogin: 2, total: 100, open: map[string]int{}, logins: map[string]int{}}
	addr := func(i int) net.Addr { return &net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(i)), Port: 1000} }
	open := func(login string, i int) bool {
		if !q.allow(login, "test", addr(i)) {
			return false
		}
		q.created(addr(i), addr(0), "udp", login, "test", addr(0), 0)
		return true
	}
	if !open("a", 1) || !open("a", 2) || open("a", 3) {
		t.Fatal("two for one login, from any address")
	}
	if !open("b", 3) {
		t.Fatal("another login has its own")
	}
	q.deleted(addr(1), addr(0), "udp", "a", "test")
	if !open("a", 3) {
		t.Fatal("no room after one closed")
	}
	one, two := ICEServers("s3cret", "voip.example", 3478, "call"), ICEServers("s3cret", "voip.example", 3478, "call")
	if one[1].Username == two[1].Username {
		t.Fatal("two logins share a name")
	}
}

// All connections together stop at the relay's share of the home line.
func TestRelayTotalRate(t *testing.T) {
	out, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	in, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	h := &hairpin{public: net.IPv4(198, 51, 100, 9), live: &livePorts{ports: map[int]bool{}}, maxRate: 1 << 20, total: &totalMeter{limit: 1000}}
	a, b := &hairpinConn{PacketConn: out, h: h}, &hairpinConn{PacketConn: out, h: h}
	a.WriteTo(make([]byte, 600), in.LocalAddr())
	b.WriteTo(make([]byte, 600), in.LocalAddr())
	in.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1000)
	if _, _, err := in.ReadFrom(buf); err != nil {
		t.Fatal("the first packet should pass")
	}
	in.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := in.ReadFrom(buf); err == nil {
		t.Fatal("the second is over the total")
	}
}

func TestHairpinOnlyReachesLiveRelayPorts(t *testing.T) {
	target, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	port := target.LocalAddr().(*net.UDPAddr).Port
	out, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	public := net.IPv4(198, 51, 100, 9)
	h := &hairpin{public: public, minPort: port - 1, maxPort: port + 1, live: &livePorts{ports: map[int]bool{}}, maxRate: 1 << 20, total: &totalMeter{limit: 1 << 20}}
	c := &hairpinConn{PacketConn: out, h: h}

	send := func() bool {
		if _, err := c.WriteTo([]byte("hi"), &net.UDPAddr{IP: public, Port: port}); err != nil {
			t.Fatal(err)
		}
		target.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := target.ReadFrom(make([]byte, 10))
		return err == nil
	}
	if send() {
		t.Fatal("a voice port with no relay on it was reached")
	}
	h.live.set(port, true)
	if !send() {
		t.Fatal("a live relay port wasn't reached")
	}
	h.live.set(port, false)
	if send() {
		t.Fatal("a closed relay port was reached")
	}
	h.minPort, h.maxPort = port+1, port+2
	if send() {
		t.Fatal("a port outside the voice range was reached")
	}
}

func TestRelayRateLimit(t *testing.T) {
	var m meter
	sent := 0
	for i := 0; i < 100; i++ {
		if m.ok(1000, 10000) {
			sent++
		}
	}
	if sent != 10 {
		t.Fatalf("%d went through", sent)
	}
}

// One client can only open the relay to so many addresses: each one costs the
// server memory, and a real call uses a handful.
func TestPeerLimit(t *testing.T) {
	l := &peerLimit{max: 3, seen: map[string]map[string]time.Time{}}
	client := &net.UDPAddr{IP: net.IPv4(203, 0, 113, 1), Port: 5000}
	other := &net.UDPAddr{IP: net.IPv4(203, 0, 113, 2), Port: 5000}
	for i := byte(1); i <= 3; i++ {
		if !l.allow(client, net.IPv4(198, 51, 100, i)) {
			t.Fatalf("peer %d refused", i)
		}
	}
	if l.allow(client, net.IPv4(198, 51, 100, 4)) {
		t.Fatal("a fourth peer")
	}
	if !l.allow(client, net.IPv4(198, 51, 100, 2)) {
		t.Fatal("renewing a peer it has")
	}
	if !l.allow(other, net.IPv4(198, 51, 100, 4)) {
		t.Fatal("another client counts on its own")
	}
	if l.allow(client, net.IPv4(192, 168, 1, 1)) {
		t.Fatal("a private address")
	}
}

func TestTCPConnectionLimit(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &tcpListener{Listener: ln, open: map[string]int{}}
	defer l.Close()
	accepted := make(chan net.Conn, 100)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	for i := 0; i < maxTCPPerAddress+5; i++ {
		c, err := net.Dial("tcp4", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(accepted); n != maxTCPPerAddress {
		t.Fatalf("%d accepted", n)
	}
	(<-accepted).Close()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(100 * time.Millisecond)
	if n := len(accepted); n != maxTCPPerAddress {
		t.Fatalf("closing one should make room: %d", n)
	}
}

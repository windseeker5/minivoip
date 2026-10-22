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

func TestPrivatePeersRefused(t *testing.T) {
	for ip, want := range map[string]bool{"75.158.253.160": true, "192.168.1.89": false, "127.0.0.1": false, "100.64.1.1": false, "10.1.2.3": false} {
		if got := publicPeer(nil, net.ParseIP(ip)); got != want {
			t.Errorf("%s: %v", ip, got)
		}
	}
}

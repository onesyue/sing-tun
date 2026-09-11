package tun

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestNATDoesNotOverwriteOccupiedPortAfterWrap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n := NewNat(ctx, time.Hour)
	destination := netip.MustParseAddrPort("203.0.113.1:443")
	first := netip.MustParseAddrPort("192.0.2.1:1000")
	port, err := n.Lookup(first, destination)
	if err != nil {
		t.Fatal(err)
	}
	n.portIndex = port
	second := netip.MustParseAddrPort("192.0.2.1:1001")
	other, err := n.Lookup(second, destination)
	if err != nil {
		t.Fatal(err)
	}
	if port == other || n.LookupBack(port).Source != first || n.LookupBack(other).Source != second {
		t.Fatal("NAT port wrap overwrote an active connection")
	}
}

func TestUnconfiguredAddressFamilyDropsTCP(t *testing.T) {
	s := &System{}
	if write, err := s.processIPv4TCP(nil, nil); write || err != nil {
		t.Fatalf("IPv4 = %t, %v", write, err)
	}
	if write, err := s.processIPv6TCP(nil, nil); write || err != nil {
		t.Fatalf("IPv6 = %t, %v", write, err)
	}
}

//go:build windows

package winipcfg

import (
	"fmt"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/metacubex/sing-tun/internal/wintun"
	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	os.Exit(runWithTestAdapter(m))
}

func runWithTestAdapter(m *testing.M) int {
	// Local tests retain the upstream explicit, expendable-adapter contract.
	// CI opts into a new adapter; never select or mutate its uplink.
	if os.Getenv("SING_TUN_TEST_ADAPTER") != "1" {
		return m.Run()
	}
	if !runningElevated() {
		fmt.Fprintln(os.Stderr, "Wintun test adapter requires elevation")
		return 1
	}
	adapter, err := wintun.CreateAdapter(testInterfaceMarker, "Wintun", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create Wintun test adapter:", err)
		return 1
	}
	defer adapter.Close()
	session, err := adapter.StartSession(wintun.RingCapacityMin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start Wintun test session:", err)
		return 1
	}
	defer session.End()
	luid := LUID(adapter.LUID())
	// Wait for the native interface rows to appear after driver creation.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err = luid.IPInterface(windows.AF_INET)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err == nil {
		err = luid.AddIPAddress(netip.MustParsePrefix("172.16.1.1/24"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "configure Wintun test interface:", err)
		return 1
	}
	return m.Run()
}

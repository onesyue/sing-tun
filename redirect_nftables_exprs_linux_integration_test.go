//go:build linux && integration

package tun

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/metacubex/nftables"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

// Run only in a disposable container's private network namespace with NET_ADMIN.
func requireIsolatedNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("SING_TUN_ISOLATED_NETNS") != "1" {
		t.Fatal("requires disposable private network namespace; set SING_TUN_ISOLATED_NETNS=1")
	}
}

func TestNFTablesIPSetIntervals(t *testing.T) {
	requireIsolatedNetwork(t)
	for _, tc := range []struct {
		name     string
		family   nftables.TableFamily
		prefixes []netip.Prefix
	}{
		{"ipv4_max", nftables.TableFamilyIPv4, []netip.Prefix{netip.MustParsePrefix("255.255.255.254/31")}},
		{"ipv6_max", nftables.TableFamilyIPv6, []netip.Prefix{netip.MustParsePrefix("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe/127")}},
		{"ipv4_ordinary", nftables.TableFamilyIPv4, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}},
		{"ipv6_ordinary", nftables.TableFamilyIPv6, []netip.Prefix{netip.MustParsePrefix("2001:db8::/64")}},
		{"ipv4_batches", nftables.TableFamilyIPv4, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prefixes == nil {
				for i := 0; i < 601; i++ {
					tc.prefixes = append(tc.prefixes, netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(i >> 8), byte(i), 1}), 32))
				}
				tc.prefixes = append(tc.prefixes, netip.MustParsePrefix("255.255.255.255/32"))
			}
			nft := &nftables.Conn{}
			table := nft.AddTable(&nftables.Table{Name: "sing_tun_test", Family: tc.family})
			t.Cleanup(func() {
				nft.DelTable(table)
				if err := nft.Flush(); err != nil {
					t.Errorf("delete table: %v", err)
				}
			})
			set, err := nftablesCreateIPSet(nft, table, 1, "addresses", tc.family, nil, tc.prefixes, false, false)
			if err != nil {
				t.Fatalf("create set: %v", err)
			}
			if err = nft.Flush(); err != nil {
				t.Fatal(err)
			}
			elements, err := nft.GetSetElements(set)
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]bool{}
			for _, p := range tc.prefixes {
				expected[hex.EncodeToString(p.Masked().Addr().AsSlice())] = false
				// The test ranges are /32, /31, /24, /127 or /64.
				last := p.Masked().Addr().As16()
				bits := p.Bits()
				if p.Addr().Is4() {
					bits += 96
				}
				for bit := bits; bit < 128; bit++ {
					last[bit/8] |= 1 << (7 - uint(bit%8))
				}
				end := netip.AddrFrom16(last).Unmap().Next()
				if end.IsValid() {
					expected[hex.EncodeToString(end.AsSlice())] = true
				}
			}
			if len(elements) != len(expected) {
				t.Fatalf("kernel returned %d boundaries, want %d", len(elements), len(expected))
			}
			for _, element := range elements {
				key := hex.EncodeToString(element.Key)
				want, ok := expected[key]
				if !ok || want != element.IntervalEnd {
					t.Errorf("unexpected kernel boundary %s end=%v", key, element.IntervalEnd)
				}
				delete(expected, key)
			}
			if len(expected) > 0 {
				t.Errorf("missing boundaries: %v", expected)
			}
		})
	}
}

func TestRulesKernelRoundTrip(t *testing.T) {
	requireIsolatedNetwork(t)
	for _, dual := range []bool{false, true} {
		t.Run(fmt.Sprintf("dual_%v", dual), func(t *testing.T) {
			tun := routeTestTun(dual)
			// Dummy links provide interface identities without opening /dev/net/tun.
			for _, name := range []string{"testtun", "testin"} {
				link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}
				if err := netlink.LinkAdd(link); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := netlink.LinkDel(link); err != nil {
						t.Error(err)
					}
				})
			}
			t.Cleanup(func() {
				if err := tun.unsetRules(); err != nil {
					t.Error(err)
				}
			})
			if err := tun.setRules(); err != nil {
				t.Fatal(err)
			}
			got, err := netlink.RuleList(unix.AF_INET6)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tun.rules() {
				if want.Family != unix.AF_INET6 {
					continue
				}
				found := false
				for _, rule := range got {
					if rule.Priority == want.Priority && rule.Goto == want.Goto && rule.IifName == want.IifName && rule.Src == want.Src && (rule.Table == want.Table || want.Table == -1 && rule.Table == 0) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing kernel IPv6 rule: %+v", want)
				}
			}
		})
	}
}

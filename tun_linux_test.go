package tun

import (
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"github.com/metacubex/sing/common/ranges"
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
)

func routeTestTun(dual bool) *NativeTun {
	opts := Options{Name: "testtun", AutoRoute: true, StrictRoute: true,
		IPRoute2RuleIndex: 30000, IPRoute2TableIndex: 30000,
		Inet6Address:     []netip.Prefix{netip.MustParsePrefix("fd00::1/64")},
		IncludeInterface: []string{"testin"},
		ExcludeSrcPort:   []ranges.Range[uint16]{{Start: 1234, End: 1234}},
	}
	if dual {
		opts.Inet4Address = []netip.Prefix{netip.MustParsePrefix("198.18.0.1/30")}
	}
	return &NativeTun{options: opts}
}

func TestRulesIPv6IncludeInterface(t *testing.T) {
	for _, tc := range []struct {
		name string
		dual bool
	}{{"ipv6_only", false}, {"dual_stack", true}} {
		t.Run(tc.name, func(t *testing.T) {
			tun := routeTestTun(tc.dual)
			rules := tun.rules()
			foundInclude, foundSource := false, false
			sourcePriority, bypassPriority := -1, -1
			for _, rule := range rules {
				if rule.Family != unix.AF_INET6 {
					continue
				}
				if rule.IifName == "testin" {
					foundInclude = true
					if rule.Goto != rule.Priority+2 {
						t.Errorf("include jump %d must reach match after skip at %d", rule.Goto, rule.Priority+2)
					}
					found := false
					for _, target := range rules {
						if target.Family == unix.AF_INET6 && target.Priority == rule.Goto && target.IifName == "" && target.Goto < 0 {
							found = true
						}
					}
					if !found {
						t.Errorf("include jump has no IPv6 match target: %+v", rule)
					}
				}
				if rule.IifName == "lo" && rule.Src == tun.options.Inet6Address[0].Masked() {
					foundSource = true
					sourcePriority = rule.Priority
					if rule.Table != tun.options.IPRoute2TableIndex {
						t.Errorf("local source table %d", rule.Table)
					}
				}
				if rule.IifName == "testtun" {
					bypassPriority = rule.Priority
				}
			}
			if !foundInclude || !foundSource || bypassPriority < 0 {
				t.Fatal("missing expected IPv6 rules")
			}
			if sourcePriority >= bypassPriority {
				t.Errorf("local IPv6 source priority %d must precede bypass %d", sourcePriority, bypassPriority)
			}
		})
	}
}

func TestAddRouteRuleUnsupportedFamily(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		ruleType                      uint8
		addErr, socketErr             error
		wantErr, wantProbe, wantClose bool
	}{
		{"success", unix.FR_ACT_UNREACHABLE, nil, nil, false, false, false},
		{"missing_family", unix.FR_ACT_UNREACHABLE, unix.EAFNOSUPPORT, unix.EAFNOSUPPORT, false, true, false},
		{"wrapped_missing_family", unix.FR_ACT_UNREACHABLE, fmt.Errorf("wrapped: %w", unix.EAFNOSUPPORT), fmt.Errorf("wrapped: %w", unix.EAFNOSUPPORT), false, true, false},
		{"family_available", unix.FR_ACT_UNREACHABLE, unix.EAFNOSUPPORT, nil, true, true, true},
		{"socket_permission_denied", unix.FR_ACT_UNREACHABLE, unix.EAFNOSUPPORT, unix.EPERM, true, true, false},
		{"rule_permission_denied", unix.FR_ACT_UNREACHABLE, unix.EPERM, unix.EAFNOSUPPORT, true, false, false},
		{"route_rule_missing_family", unix.FR_ACT_TO_TBL, unix.EAFNOSUPPORT, unix.EAFNOSUPPORT, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := netlink.NewRule()
			rule.Family = unix.AF_INET6
			rule.Type = tc.ruleType
			probes, closes, adds := 0, 0, 0
			err := addRouteRule(rule, func(got *netlink.Rule) error {
				adds++
				if got != rule {
					t.Error("rule changed")
				}
				return tc.addErr
			},
				func(family, kind, protocol int) (int, error) {
					probes++
					if family != unix.AF_INET6 || kind != unix.SOCK_DGRAM|unix.SOCK_CLOEXEC || protocol != 0 {
						t.Error("wrong socket probe")
					}
					return 42, tc.socketErr
				},
				func(fd int) error {
					closes++
					if fd != 42 {
						t.Error("wrong closed fd")
					}
					return nil
				})
			if tc.wantErr {
				if !errors.Is(err, tc.addErr) {
					t.Errorf("got %v want original %v", err, tc.addErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error %v", err)
			}
			if adds != 1 || (probes == 1) != tc.wantProbe || (closes == 1) != tc.wantClose {
				t.Errorf("calls add=%d probe=%d close=%d", adds, probes, closes)
			}
		})
	}
}

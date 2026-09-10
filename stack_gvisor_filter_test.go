//go:build with_gvisor

package tun

import (
	"testing"

	"github.com/metacubex/gvisor/pkg/tcpip/stack"
)

type detachProbe struct {
	stack.LinkEndpoint
	got stack.NetworkDispatcher
}

func (p *detachProbe) Attach(d stack.NetworkDispatcher) { p.got = d }

type dispatcherProbe struct{ stack.NetworkDispatcher }

func TestLinkEndpointFilterPreservesDetach(t *testing.T) {
	p := &detachProbe{}
	filter := &LinkEndpointFilter{LinkEndpoint: p}
	filter.Attach(&dispatcherProbe{})
	if p.got == nil {
		t.Fatal("lost attached dispatcher")
	}
	filter.Attach(nil)
	if p.got != nil {
		t.Fatal("nil detach was wrapped into a live dispatcher")
	}
}

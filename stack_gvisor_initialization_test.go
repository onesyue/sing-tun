//go:build with_gvisor

package tun

import (
	"testing"

	"github.com/metacubex/gvisor/pkg/tcpip/link/channel"
	"github.com/metacubex/gvisor/pkg/tcpip/stack"
	"github.com/metacubex/gvisor/pkg/tcpip/transport/tcp"
)

// A native endpoint can begin packet delivery inside Attach. Observe that
// publication boundary rather than sleeping until Start probably finishes.
type initializationEndpoint struct {
	*channel.Endpoint
	beforeAttach func()
	attached     bool
}

func (e *initializationEndpoint) Attach(dispatcher stack.NetworkDispatcher) {
	if dispatcher != nil {
		e.attached = true
		e.beforeAttach()
	}
	e.Endpoint.Attach(dispatcher)
}

func TestStackInitializerPrecedesEndpointPublication(t *testing.T) {
	initialized := false
	ep := &initializationEndpoint{Endpoint: channel.New(1, 1500, "")}
	defer ep.Close()
	ep.beforeAttach = func() {
		if !initialized {
			t.Error("packet dispatcher published before transport handlers")
		}
	}
	s, err := newGVisorStackWithInitializer(ep, stack.NICOptions{}, TCPBufferRange{}, func(s *stack.Stack) {
		if ep.attached {
			t.Error("handler initialization ran after packet delivery became possible")
		}
		s.SetTransportProtocolHandler(tcp.ProtocolNumber, func(stack.TransportEndpointID, *stack.PacketBuffer) bool { return true })
		initialized = true
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !initialized || !ep.attached {
		t.Fatal("test did not traverse both initialization and native attachment")
	}
}

func TestInvalidStackInputNeverPublishesEndpoint(t *testing.T) {
	ep := &initializationEndpoint{Endpoint: channel.New(1, 1500, "")}
	defer ep.Close()
	ep.beforeAttach = func() { t.Error("invalid input published a dispatcher") }
	s, err := newGVisorStackWithInitializer(ep, stack.NICOptions{}, TCPBufferRange{Min: -1}, func(*stack.Stack) {
		t.Error("invalid input reached handler initialization")
	})
	if err == nil || s != nil || ep.attached {
		t.Fatalf("invalid input: stack=%v error=%v attached=%v", s, err, ep.attached)
	}
}

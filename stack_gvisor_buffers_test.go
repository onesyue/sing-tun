//go:build with_gvisor

package tun

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/gvisor/pkg/tcpip"
	"github.com/metacubex/gvisor/pkg/tcpip/link/channel"
	"github.com/metacubex/gvisor/pkg/tcpip/network/ipv4"
	"github.com/metacubex/gvisor/pkg/tcpip/stack"
	"github.com/metacubex/gvisor/pkg/tcpip/transport/tcp"
	"github.com/metacubex/gvisor/pkg/waiter"
	"github.com/metacubex/sing/common/logger"
)

type bufferTestTun struct {
	endpoint *channel.Endpoint
	ctx      context.Context
}

func (t *bufferTestTun) Read([]byte) (int, error)                     { <-t.ctx.Done(); return 0, net.ErrClosed }
func (*bufferTestTun) Write(p []byte) (int, error)                    { return len(p), nil }
func (*bufferTestTun) Close() error                                   { return nil }
func (*bufferTestTun) WritePacket(p *stack.PacketBuffer) (int, error) { return p.Size(), nil }
func (t *bufferTestTun) NewEndpoint() (stack.LinkEndpoint, stack.NICOptions, error) {
	return t.endpoint, stack.NICOptions{}, nil
}

// Exercise NewStack -> the real GVisor/Mixed Start -> a real TCP endpoint.
// No private field injection or stand-in stack can satisfy these assertions.
func TestStackTCPBufferOptionsReachEndpoints(t *testing.T) {
	for _, kind := range []string{"gvisor", "mixed"} {
		for _, buffers := range []TCPBufferRange{
			{},
			{Min: 4096, Default: 128 << 10, Max: 256 << 10},
			{Min: 4096, Default: 256 << 10, Max: 1 << 20},
			{Min: 4096, Default: 512 << 10, Max: 4 << 20},
		} {
			t.Run(kind+"/"+time.Duration(buffers.Default).String(), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				tun := &bufferTestTun{endpoint: channel.New(16, 9000, ""), ctx: ctx}
				defer tun.endpoint.Close()
				s, err := NewStack(kind, StackOptions{
					Context: ctx, Tun: tun, Logger: logger.NOP(),
					TunOptions: Options{MTU: 9000, Inet4Address: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/30")}},
					UDPTimeout: time.Second, ICMPTimeout: time.Second,
					GVisorTCPBufferRange: buffers,
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Start(); err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				var ipStack *stack.Stack
				switch s := s.(type) {
				case *GVisor:
					ipStack = s.stack
				case *Mixed:
					ipStack = s.stack
				default:
					t.Fatalf("unexpected native stack %T", s)
				}
				want := buffers
				if want == (TCPBufferRange{}) {
					want = TCPBufferRange{Min: 1, Default: 20 << 10, Max: 20 << 10}
				}
				var receive tcpip.TCPReceiveBufferSizeRangeOption
				var send tcpip.TCPSendBufferSizeRangeOption
				if err := ipStack.TransportProtocolOption(tcp.ProtocolNumber, &receive); err != nil {
					t.Fatal(err)
				}
				if err := ipStack.TransportProtocolOption(tcp.ProtocolNumber, &send); err != nil {
					t.Fatal(err)
				}
				if receive.Min != want.Min || receive.Default != want.Default || receive.Max != want.Max || send.Min != want.Min || send.Default != want.Default || send.Max != want.Max {
					t.Fatalf("native ranges receive=%+v send=%+v want=%+v", receive, send, want)
				}
				var wq waiter.Queue
				endpoint, epErr := ipStack.NewEndpoint(tcp.ProtocolNumber, ipv4.ProtocolNumber, &wq)
				if epErr != nil {
					t.Fatal(epErr)
				}
				defer endpoint.Close()
				if got := endpoint.SocketOptions().GetReceiveBufferSize(); got != int64(want.Default) {
					t.Fatalf("receive endpoint buffer = %d, want %d", got, want.Default)
				}
				if got := endpoint.SocketOptions().GetSendBufferSize(); got != int64(want.Default) {
					t.Fatalf("send endpoint buffer = %d, want %d", got, want.Default)
				}
			})
		}
	}
}

func TestInvalidTCPBufferRangeRejectedBeforeTunAccess(t *testing.T) {
	for _, kind := range []string{"gvisor", "mixed"} {
		for _, r := range []TCPBufferRange{{Min: -1}, {Default: 1024}, {Min: 2, Default: 1, Max: 3}, {Min: 1, Default: 3, Max: 2}} {
			if _, err := NewStack(kind, StackOptions{GVisorTCPBufferRange: r}); err == nil {
				t.Fatalf("%s accepted invalid range %+v", kind, r)
			}
		}
	}
}

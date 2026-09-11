//go:build with_gvisor

package tun

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/metacubex/gvisor/pkg/tcpip"
	"github.com/metacubex/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/metacubex/gvisor/pkg/tcpip/link/veth"
	"github.com/metacubex/gvisor/pkg/tcpip/network/ipv4"
	"github.com/metacubex/gvisor/pkg/tcpip/stack"
	"github.com/metacubex/gvisor/pkg/tcpip/transport/tcp"
)

// This is a local gVisor TCP transfer, not a physical TUN or WAN benchmark.
// Each operation transfers and verifies 8 MiB over a real native veth pair.
// The legacy case applies identical limits after construction, reproducing
// the previous client's steady-state tuning without depending on reflection.
func BenchmarkGVisorTCPBufferTransfer(b *testing.B) {
	for _, profile := range []struct {
		name       string
		rangeBytes TCPBufferRange
	}{
		{"ios", TCPBufferRange{4096, 128 << 10, 256 << 10}},
		{"android", TCPBufferRange{4096, 256 << 10, 1 << 20}},
		{"desktop", TCPBufferRange{4096, 512 << 10, 4 << 20}},
	} {
		for _, native := range []bool{false, true} {
			mode := "legacy"
			if native {
				mode = "native"
			}
			b.Run(profile.name+"/"+mode, func(b *testing.B) {
				left, right := veth.NewPair(9000, 1024)
				defer left.Close()
				makeStack := func(ep stack.LinkEndpoint, address [4]byte) *stack.Stack {
					buffers := profile.rangeBytes
					if !native {
						buffers = TCPBufferRange{}
					}
					s, err := newGVisorStackWithTCPBuffers(ep, stack.NICOptions{}, buffers)
					if err != nil {
						b.Fatal(err)
					}
					b.Cleanup(s.Close)
					if !native {
						r := profile.rangeBytes
						if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpip.TCPReceiveBufferSizeRangeOption{Min: r.Min, Default: r.Default, Max: r.Max}); err != nil {
							b.Fatal(err)
						}
						if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpip.TCPSendBufferSizeRangeOption{Min: r.Min, Default: r.Default, Max: r.Max}); err != nil {
							b.Fatal(err)
						}
					}
					if err := s.AddProtocolAddress(DefaultNIC, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: tcpip.AddrFrom4(address).WithPrefix()}, stack.AddressProperties{}); err != nil {
						b.Fatal(err)
					}
					return s
				}
				clientStack := makeStack(left, [4]byte{10, 0, 0, 1})
				serverStack := makeStack(right, [4]byte{10, 0, 0, 2})
				serverAddress := tcpip.FullAddress{NIC: DefaultNIC, Addr: tcpip.AddrFrom4([4]byte{10, 0, 0, 2}), Port: 443}
				listener, err := gonet.ListenTCP(serverStack, serverAddress, ipv4.ProtocolNumber)
				if err != nil {
					b.Fatal(err)
				}
				defer listener.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				client, err := gonet.DialContextTCP(ctx, clientStack, serverAddress, ipv4.ProtocolNumber)
				if err != nil {
					b.Fatal(err)
				}
				defer client.Close()
				server, err := listener.Accept()
				if err != nil {
					b.Fatal(err)
				}
				defer server.Close()
				payload := bytes.Repeat([]byte{0x5a}, 8<<20)
				received := make([]byte, len(payload))
				done := make(chan error, 1)
				b.SetBytes(int64(len(payload)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					deadline := time.Now().Add(10 * time.Second)
					client.SetDeadline(deadline)
					server.SetDeadline(deadline)
					go func() { _, err := io.ReadFull(server, received); done <- err }()
					n, err := client.Write(payload)
					if err != nil || n != len(payload) {
						b.Fatalf("transfer: %d, %v", n, err)
					}
					if err := <-done; err != nil {
						b.Fatal(err)
					}
					if !bytes.Equal(received, payload) {
						b.Fatal("payload corruption")
					}
				}
				b.StopTimer()
			})
		}
	}
}

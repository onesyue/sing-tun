# Yue native sing-tun changes

The exact MetaCubeX baseline remains `dfc71de` (the version already consumed
by the YueLink mihomo fork). This branch preserves the descriptor ownership
and dispatcher detach fixes from `46f2bf37e4b4`.

`StackOptions.GVisorTCPBufferRange` is a typed optional range in bytes. Its
zero value preserves the upstream 20 KiB defaults. Nonzero values must satisfy
`0 < Min <= Default <= Max`; both GVisor and Mixed reject invalid ranges before
opening resources. A range bounds buffering and does not reserve memory.

Both constructors copy the range by value and apply it before the stack's NIC
attaches its packet dispatcher. This lets the embedder select a platform memory
budget without inspecting private fields, exposing a gVisor pointer, or racing
the first connection with post-start mutation. The existing public stack
constructors retain their signatures and defaults.

`TestStackTCPBufferOptionsReachEndpoints` starts actual GVisor and Mixed
instances using a channel-backed TUN. It checks both transport ranges and the
send/receive sizes inherited by real newly created TCP endpoints. It covers
the upstream default plus the iOS, Android, and desktop budgets. This does not
claim mobile device throughput or platform VPN acceptance.

The Yue native workflow runs all packages with and without gVisor under the
race detector on Linux, macOS, and Windows, using Go 1.26.8. The copied Windows
IP configuration files now declare their actual platform, and the missing
`Address.String` method is restored verbatim from the pinned gVisor dependency.
This repairs the inherited test discovery and vet failures without disabling vet.

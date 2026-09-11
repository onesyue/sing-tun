# Yue native sing-tun changes

The exact MetaCubeX baseline is `cd38aa821322738cc92a4f618dc8ebad830f179b`
(2026-09-09), merged with its original ancestry. Compared with the previous
`dfc71de` pin, this includes native batch-buffer, processor-option, default
interface update, NAT port ownership, and unconfigured address-family fixes.
This branch preserves the descriptor ownership and dispatcher detach fixes
from `46f2bf37e4b4`.

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

The upstream interface monitor retry path is additionally bounded by its owner:
Close stops the timer, and a failed in-flight enumeration cannot rearm it after
closure. The deterministic blocked-enumeration test covers that shutdown race.
NAT port-wrap and unsupported-family tests exercise the newly imported upstream
data-path fixes.

`BenchmarkGVisorTCPBufferTransfer` compares identical old post-construction and
new native constructor buffer limits using 8 MiB verified payloads through a
real gVisor TCP/veth pair (MTU 9000). It measures local stack throughput and
cumulative allocations, not physical TUN overhead, RSS, or encrypted WAN speed.

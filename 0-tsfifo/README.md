# tsfifo

A [Tailscale](https://tailscale.com) node for hosts with no `/dev/net/tun`, no
`CAP_NET_ADMIN` and no root — AWS Lambda, for one. It is built on
[tsnet](https://pkg.go.dev/tailscale.com/tsnet) and exchanges the node's
packets over a pair of FIFOs instead of a TUN device, so a user-space network
stack in another process (e.g. [LKL](https://github.com/lkl/linux)) can own
the tailnet interface.

The image is `FROM scratch`: one static binary, `/tsfifo`, plus a CA bundle
and `/tmp`. Tags track the Tailscale release it is built against (e.g.
`:1.104.1`) plus `:latest`, rebuilt nightly.

## Usage

Copy it into your image:

```dockerfile
COPY --from=ghcr.io/cnuss/tsfifo /tsfifo /usr/local/bin/tsfifo
```

and start it before the process that uses the tailnet:

```sh
TS_AUTHKEY=tskey-auth-... TS_HOSTNAME=my-function tsfifo &
```

Node state is kept in memory and the node is ephemeral, so every start is a
new node; on `SIGTERM` it logs out, which deletes the node at once.

### FIFOs and sockets

All are created mode `0600`, under `$TMPDIR` (else `/tmp`) unless set:

| Flag | Env | Default | |
|---|---|---|---|
| `-fifo-up` | `TS_FIFO_UP` | `ts.up` | FIFO, frames to tsfifo. |
| `-fifo-down` | `TS_FIFO_DOWN` | `ts.down` | FIFO, frames from tsfifo. |
| `-config` | `TS_CONFIG_SOCKET` | `ts-config.sock` | `SOCK_STREAM`. Each client gets the node config as one JSON line now and another on every change. |
| `-socket` | `TS_SOCKET` | `tailscaled.sock` | LocalAPI: `tailscale --socket=$TMPDIR/tailscaled.sock status` |

The FIFOs carry Ethernet frames, each prefixed with its 4-byte big-endian
length (QEMU's `-netdev stream` framing), MTU 1280. tsfifo creates them
before anything else, or reuses FIFOs already at those paths, and opens both
`O_RDWR`, so either side can restart without EOF or `SIGPIPE`; open them
`O_RDWR` on your side too. Each frame is one `write()` of at most
`PIPE_BUF` (4096 bytes), so it is atomic. Writes toward the peer don't
block: when the pipe is full, the frame is dropped.

tsfifo's MAC is `02:00:00:00:00:01`; frames to the peer go to the MAC its
own frames come from (`02:00:00:00:00:02` until the first one). It answers
ARP requests and IPv6 neighbor solicitations for any address with its own
MAC, so routes can be on-link (`default dev eth0`) with no gateway. ARP
probes and DAD solicitations go unanswered, so the peer's own addresses
don't look duplicated; turning DAD off (`accept_dad=0`) skips its delay.
IPv4 and IPv6 go to the tailnet; other ethertypes, neighbor discovery, and
IPv6 multicast and link-local traffic are dropped.

The config line, once `state` is `Running`:

```json
{"state":"Running","name":"my-function.tailnet.ts.net","mtu":1280,
 "addresses":["100.101.102.103/32","fd7a:115c:a1e0::1/128"],
 "routes":["0.0.0.0/0","100.64.0.0/10","::/0","fd7a:115c:a1e0::/48"],
 "exitNode":"nXXXXXXXXX",
 "dns":{"nameservers":["100.100.100.100","fd7a:115c:a1e0::53"],"search":["tailnet.ts.net"]}}
```

`routes` holds the tailnet ranges, plus peers' subnet routes with
`TS_ACCEPT_ROUTES=true`, plus the default routes while an exit node is in
use. When `dns.matchDomains` is present, MagicDNS answers only those suffixes
and returns `SERVFAIL` for others, so list a second nameserver after it.
Before `Running`, `state` is `NeedsLogin` or `Starting`, with `error` set if
login failed.

### Environment

| Env | Flag | |
|---|---|---|
| `TS_AUTHKEY` | | Auth key. Use a reusable, ephemeral, pre-approved, tagged one. |
| `TS_CLIENT_SECRET` | | Or an OAuth client secret (`tskey-client-...?preauthorized=true`); needs `TS_TAGS`. |
| `TS_TAGS` | `-tags` | Comma-separated tags to advertise. |
| `TS_CONTROL_URL` | | Control server, e.g. Headscale. Default: Tailscale. |
| `TS_HOSTNAME` | `-hostname` | Node name. Default: the OS hostname. |
| `TS_EXIT_NODE` | `-exit-node` | Exit node: IP, MagicDNS name, or `auto:any`. |
| `TS_ACCEPT_ROUTES` | `-accept-routes` | `true` to accept peers' subnet routes. |
| `TS_STATE_DIR` | `-statedir` | Log directory. Default: `$TMPDIR/tsfifo`. |
| `TS_NO_LOGS_NO_SUPPORT` | | `true` stops log upload to Tailscale. |
| `TS_VERBOSE` | `-verbose` | `true` logs tsnet internals. |

### Host requirements

Outbound TCP (and UDP, for direct WireGuard paths; without it traffic relays
over DERP on TCP 443), a `NETLINK_ROUTE` socket (Go's interface listing
needs it; startup fails without it), and a writable `$TMPDIR`. No TUN,
capabilities, iptables or root.

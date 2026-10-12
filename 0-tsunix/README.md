# tsunix

A [Tailscale](https://tailscale.com) node for hosts with no `/dev/net/tun`, no
`CAP_NET_ADMIN` and no root — AWS Lambda, for one. It is built on
[tsnet](https://pkg.go.dev/tailscale.com/tsnet) and exchanges the node's IP
packets over a `SOCK_SEQPACKET` unix socket instead of a TUN device, so a
user-space network stack in another process (e.g. [LKL](https://github.com/lkl/linux))
can own the tailnet interface.

The image is `FROM scratch`: one static binary, `/tsunix`, plus a CA bundle
and `/tmp`. Tags track the Tailscale release it is built against (e.g.
`:1.104.1`) plus `:latest`, rebuilt nightly.

## Usage

Copy it into your image:

```dockerfile
COPY --from=ghcr.io/cnuss/tsunix /tsunix /usr/local/bin/tsunix
```

and start it before the process that uses the tailnet:

```sh
TS_AUTHKEY=tskey-auth-... TS_HOSTNAME=my-function tsunix &
```

Node state is kept in memory and the node is ephemeral, so every start is a
new node; on `SIGTERM` it logs out, which deletes the node at once.

### Sockets

All are created mode `0600`, under `$TMPDIR` (else `/tmp`) unless set:

| Flag | Env | Default | |
|---|---|---|---|
| `-tun` | `TS_TUN` | `ts.sock` | `SOCK_SEQPACKET`, listening. One raw IPv4/IPv6 packet per message, no header, MTU 1280. One client at a time; a new connection replaces the old one. |
| `-config` | `TS_CONFIG_SOCKET` | `ts-config.sock` | `SOCK_STREAM`. Each client gets the node config as one JSON line now and another on every change. |
| `-socket` | `TS_SOCKET` | `tailscaled.sock` | LocalAPI: `tailscale --socket=$TMPDIR/tailscaled.sock status` |

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
| `TS_STATE_DIR` | `-statedir` | Log directory. Default: `$TMPDIR/tsunix`. |
| `TS_NO_LOGS_NO_SUPPORT` | | `true` stops log upload to Tailscale. |
| `TS_VERBOSE` | `-verbose` | `true` logs tsnet internals. |

### Host requirements

Outbound TCP (and UDP, for direct WireGuard paths; without it traffic relays
over DERP on TCP 443), a `NETLINK_ROUTE` socket (Go's interface listing
needs it; startup fails without it), and a writable `$TMPDIR`. No TUN,
capabilities, iptables or root.

# lkl

[LKL](https://github.com/lkl/linux), the Linux Kernel Library: a Linux kernel
built as a shared library, and its `LD_PRELOAD` "hijack" library, which moves
a program's sockets onto that kernel's network stack inside the program's own
process. The program gets Linux's own TCP/IP, routing, netfilter and
WireGuard without root, capabilities, `/dev/net/tun`, ptrace or seccomp — so
it works where all of those are denied, as in AWS Lambda.

The image is `FROM scratch` with everything under `/opt/lkl`, for copying
into other images:

- `/opt/lkl/liblkl.so` — the kernel
- `/opt/lkl/liblkl-hijack.so` — the preload library, which finds `liblkl.so`
  next to itself
- `/opt/lkl/include` — LKL's headers, for programs that use the kernel
  through LKL's API (`lkl_sys_socket`, `lkl_netdev_fifo_create`, ...) rather
  than through the preload library:
  `gcc -I/opt/lkl/include -o prog prog.c -L/opt/lkl -llkl -Wl,-rpath,/opt/lkl`

They need glibc 2.39 or newer and `linux/amd64`. Tags are the kernel version
of lkl/linux's master branch plus its commit (e.g. `:6.12.0-d0f76a77e8c2`),
and `:latest`, rebuilt nightly.

## Usage

```dockerfile
COPY --from=ghcr.io/cnuss/lkl /opt/lkl /opt/lkl
```

```sh
LKL_HIJACK_CONFIG_FILE=lkl.json LD_PRELOAD=/opt/lkl/liblkl-hijack.so my-program
```

The kernel boots inside `my-program` before its `main()` runs, and its
sockets — `socket`, `connect`, `read`/`write` on them, `poll`, `epoll` and the
rest — go to that kernel. Files stay the host's: only the network moves. The
config file is LKL's own JSON
([reference](https://github.com/lkl/linux/blob/master/README.md#lkl-hijack-library)):
interfaces, addresses, gateway, `sysctl` and so on.

## The `fifo` interface

The kernel's network has to reach something outside the process. Besides
LKL's own interface types, this build adds `fifo`: Ethernet frames over a pair
of FIFOs (named pipes), each frame after its length as a 4-byte big-endian
integer — the framing of QEMU's `-netdev stream` — since a pipe keeps no
message boundaries. `param` is `"receive-fifo|transmit-fifo"` (or
`lkl_netdev_fifo_create("receive|transmit")` through the API), and the MTU
is 1280 (`LKL_NETDEV_FIFO_DEFAULT_MTU`) unless the config sets one, which
leaves room for WireGuard's overhead on a 1500-byte path; the program can
change it like any MTU.

[tsfifo](../0-tsfifo) speaks the other end: a Tailscale node that puts the
tailnet on those FIFOs. With it running, a config like this gives the program
the node's tailnet address, with every route going through tsfifo — which
answers ARP for any address, so `100.100.100.100` serves as the gateway:

```json
{
  "interfaces": [{
    "type": "fifo",
    "param": "/tmp/ts.down|/tmp/ts.up",
    "ip": "100.69.146.59",
    "masklen": "10",
    "mac": "02:00:00:00:00:02"
  }],
  "gateway": "100.100.100.100",
  "sysctl": "net.ipv6.conf.all.accept_dad=0"
}
```

tsfifo reports the address, routes and DNS on its config socket once it is
`Running`.

## Caveats

- **Each process boots its own kernel.** Children inherit `LD_PRELOAD` and
  boot one each: same config, separate stacks. A FIFO pair belongs to one of
  them.
- **A kernel that fails to start falls back to the host's network, silently**
  — a missing FIFO or a bad config leaves the program on the host stack, not
  without one. Check before trusting it: a socket's local address
  (`getsockname`) is the kernel's address, and the process has threads of the
  kernel's (`/proc/self/status`).
- Statically linked programs and Go programs make system calls directly, so
  the preload library never sees them.

## How it's built

A stage on stock `ubuntu:24.04` builds lkl/linux at the tagged commit with
the `fifo` patch (`fifo.patch`), WireGuard, TUN, nftables with NAT and policy
routing. `liblkl-hijack.so` is built from LKL's hijack sources against
`liblkl.so` instead of taken from LKL's build, whose copy links the kernel in
statically and hangs at boot. The build fails unless the kernel boots in a
process under the preload library.

```sh
docker build -t ghcr.io/cnuss/lkl --build-arg VERSION="$(./0-lkl/version.sh)" 0-lkl/
```

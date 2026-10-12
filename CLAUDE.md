# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this repo is

Multi-arch (`linux/amd64` + `linux/arm64`) container images, built by GitHub Actions
and published to GHCR under `ghcr.io/cnuss/<name>`. Each top-level directory with a
`Dockerfile` is one image. A leading `<digits>-` prefix on the directory sets its
build **tier** and is stripped from the image name (`0-ubuntu` → `ubuntu`,
`1-claude-code` → `claude-code`); unprefixed dirs are tier 0. Adding an image
requires zero workflow edits.

Images:

- `0-ubuntu` → `ghcr.io/cnuss/ubuntu:24.04` — shared base: Ubuntu 24.04 + common
  tools + NodeSource apt repo (no Node installed) + xpra/xpra-html5/xvfb from
  the xpra.org repo (universe's xpra is a dead 3.1.5), flattened to a single
  layer.
- `0-linux` → `ghcr.io/cnuss/linux` — User-mode Linux, alpine-style (no
  ENTRYPOINT, `CMD ["/bin/sh"]`, `SHELL ["/bin/sh", "-c"]` spelled out),
  except every `/bin` command runs in a UML guest whose root is the
  container's whole filesystem (hostfs `rootflags=/`): WORKDIR, ENV, COPY,
  volumes, hostname all carry into the guest. Was `1-uml`; moved to tier 0 on
  request so tier-1 images can build `FROM` it. Build stage on STOCK
  `ubuntu:24.04` (tier-0 images build in parallel, so it can't depend on
  `ghcr.io/cnuss/ubuntu`; nothing from the stage ships) compiles a dynamic
  `ARCH=um` kernel (`linux-${VERSION}.tar.xz` from cdn.kernel.org; defconfig +
  `UML_NET_VECTOR`, `HOSTFS`, `DEVTMPFS` + `DEVTMPFS_MOUNT` — init relies on the
  kernel mounting `/dev`, so pinned even though defconfig has them — minus
  `BLK_DEV_UBD`, which printed "Failed to initialize ubd device 0" every boot
  with no disk). The stage assembles `/out` = the whole image, copied in one
  `COPY --from=build /out/ /` onto scratch: `/uml/linux`, `/uml/lib`
  (its glibc), `/uml/etc` (guest resolv.conf mountpoint),
  `/uml/bin/{init,run,runas,gvproxy,busybox}` (the only real BusyBox), `/uml/tmp`
  1777; every
  BusyBox applet name in `/bin` (272, incl. `busybox` itself) and
  `/usr/bin/env` as symlinks to `/uml/bin/run`; `/tmp` 1777; empty `/dev
  /proc /sys /etc` so builds don't add mountpoints to layers; root-only
  `/etc/passwd` + `/etc/group` and `/root` 0700, as in alpine (without them
  BuildKit fails `USER root` / bare `USER 0` with "unable to find user";
  `USER uid:gid` worked even then). Kernel source is the cdn.kernel.org
  stable tarball, built UNALTERED — user's rule 2026-10-12: config via
  `scripts/config` only, never patch/sed the source. In-image paths and
  `UML_ARGS` keep the `uml` name. `version.sh` resolves the newest 6.18.x
  (LTS) via `git ls-remote` on the stable tree (survives 6.18 leaving
  kernel.org's releases.json) — was a static `6.18`, i.e. 6.18.0 forever;
  moving to a newer series is deliberate.
  - `arches` = `amd64`, settled 2026-10-12 — don't relitigate. Mainline UML
    has no arm64 port (`arch/x86/Makefile.um` is the only `Makefile.um`
    through v7.3-rc6), and the amd64 image can't run under per-process
    emulation on arm64: QEMU user mode dies with "ptrace: Function not
    implemented"; Rosetta (Docker Desktop's default on Apple Silicon) with
    "check_ptrace : expected (SIGTRAP|0x80), got status = 256", and UML's
    seccomp mode (`UML_ARGS=seccomp=on`, present in 6.18) with "SECCOMP
    userspace requested but not functional!" (all verified 2026-10-12). A
    full x86_64 VM runs it unchanged — colima `--arch x86_64 --vm-type qemu`
    (needs `brew install lima-additional-guestagents`): ~2.6s per command,
    `-it` shell and derived-image builds work (verified on M5 Max; Docker
    Desktop's VM has no `/dev/kvm`). Documented in the README's Apple
    Silicon section. A QEMU-system (TCG) backend
    for arm64 behind the same `/uml/bin/run` was considered and declined:
    a full VM with slow software-emulated boots on every RUN, not FROM
    scratch, two backends to maintain. Revisit only if upstream UML gains
    arm64.
  - `/uml/bin/run` (busybox-sh script on `/uml/bin/busybox`, which a RUN
    can't break by replacing `/bin`) runs `${0##*/} "$@"` in a guest when
    invoked via a `/bin` (or `/usr/bin/env`) symlink — `docker run IMG uname
    -a`, `docker exec C ls`, SHELL, exec-form RUN, shebangs; as itself it
    runs `"$@"` (no args → usage, exit 2). No arg joining anymore: `docker run
    IMG 'uname -a'` fails with 127 exactly as in alpine (decided 2026-10-12;
    the earlier ENTRYPOINT-run design joined args with `sh -c`). It writes
    a handoff script `/uml/tmp/run.$$` — `busybox rm -f` of itself (the
    guest deletes it at start, so a SIGKILLed container doesn't keep it),
    `(unset PWD OLDPWD SHLVL; export -p)`, `cd '<PWD>' || exit`, `exec
    ['/uml/bin/runas' uid gid groups] '<argv>'` — each word single-quoted
    with `'` → `'\''` via ash `${a//\'/...}` (no `$(sed)`: it strips
    trailing newlines) — boots UML as its child with `init=/uml/bin/init
    run=/uml/tmp/run.$$`, deletes the file, returns UML's status. Per-call
    file because every call (incl. each `docker exec`) boots its OWN guest;
    concurrent execs verified; a running guest sees another guest's file
    changes on reopen (verified). Not the kernel cmdline: it mangles quotes,
    and >32 words after `--` panic the kernel (`MAX_INIT_ARGS`). UML is
    never PID 1, so a guest panic's `abort()` exits (PID 1 ignores SIGABRT
    and spins); `--init` only makes `docker stop` prompt (wrapper as PID 1
    ignores SIGTERM → 10s kill; measured). `UML_ARGS` env adds kernel params
    (repeat `mem=` → last wins, so it overrides the wrapper's). Guest
    `mem=` is computed per call: min(cgroup `memory.max` / v1
    `memory.limit_in_bytes`, `/proc/meminfo` MemAvailable) − 64M headroom
    for UML's host process + gvproxy (was a fixed 128M until 2026-10-12;
    user: "assume this is the only process"). Sparse file, so only touched
    pages cost; boot grows ~0.2s/GB under emulation (32G: 10s vs 2.8s) from
    the guest's struct-page init — not measured natively. Oversized `mem`
    boots fine (8G in a 4G VM). Verified `-m 512m`/`-m 256m` with the
    guest filled to 10M free, disk-backed and `--tmpfs /uml/tmp:exec`: no
    container OOM kill (the guest's own OOM killer acts instead).
    Concurrent execs each get the full amount — overcommit by design.
    Native (host-kernel) commands in the
    container: `/uml/bin/busybox <applet>` — the wrapper itself must call
    applets that way (`/bin/id` would boot a guest).
  - Network (2026-10-12): `vec0` via the vector driver's `bess` transport
    (unix seqpacket) to gvproxy (gvisor-tap-vsock v0.9.0, release binary,
    sha256-checked against the release's `sha256sums`), started per call
    by the wrapper on `$uml_dir/net.$$`, killed after; ~2s wait, else the
    guest boots without vec0. Guest is gvproxy's default
    192.168.127.2/24, gw+DNS .1; init mounts a guest-only resolv.conf
    (container's minus nameservers + `nameserver 192.168.127.1`) from a
    tmpfs on `/uml/etc` — a loopback nameserver (Docker's 127.0.0.11) would
    be the guest's own loopback. `depth=64`: the driver warns on the
    console (= stdout) above 64. Trimmed 2026-10-12 as no-ops (each
    tested: streams, tty, network, dmesg, opening tty2/ttyS0, download
    speed): `con=null`, `ssl=null`, tty-mode `con0=fd:0,fd:1`,
    `root=/dev/root`, and vec0's `gro=1`/`mac=` from gvproxy's UML
    example. Verified: HTTP, DNS, `RUN wget` in builds,
    sibling-container DNS on a user network, `--network none` fails fast,
    non-root USER, concurrent execs, `mem=1G`. Costs: image 13.3 → 36.4MB
    (gvproxy 13.6MB, glibc 2.3MB), ~+0.3–1s per call under emulation.
    `UML_NET_VECTOR` selects `MAY_HAVE_RUNTIME_DEPS` and `STATIC_LINK`
    depends on its negation (checked in v6.18 Kconfig), so networking ⇒
    dynamic UML: glibc + loader + `libgcc_s` (glibc dlopen()s it for
    pthread_cancel; only DT_RPATH covers that) in `/uml/lib`, interpreter
    and `--force-rpath` set with patchelf. gvproxy is dynamic too and
    patchelf segfaults Go binaries, so it runs via `/uml/lib/ld.so`.
    passt (vhost-user, same IP) was tried first and is dead: it exits
    under Docker's default seccomp with "Couldn't create user namespace:
    Operation not permitted" and can't skip its sandbox. "Borrow the host
    stack" literally (guest socket() → host) needs a kernel patch — ruled
    out. BusyBox wget's HTTPS fails ("TLS error ... bad MAC") natively
    too — not the network — and it doesn't validate certs anyway. A
    static curl (moparisthebest/static-curl `curl-amd64` v8.21.0) + alpine's
    CA bundle does HTTPS fine in `docker run` and `RUN` (verified: google,
    api.github.com, cdn.kernel.org, cert verify 0); snippet in README. Merged as-is on request (no opt-out env var).
  - Sandboxes (2026-10-12; user wants it in AWS Lambda-like hosts, "focus
    on what a general sandbox needs"): host needs x86_64, ptrace OR seccomp
    filters, one writable non-noexec dir, outbound net optional; no root,
    caps, `/dev/shm`, cgroups, userns. Wrapper passes `seccomp=auto` (UML
    6.18: tries a seccomp filter + SIGSYS stub, else ptrace; default is off):
    faster (fork loop 67s vs 88s under emulation, `uname` 2.7 vs 3.9s) and
    survives ptrace-blocked hosts; its help text calls it insecure — guest
    processes can reach guest-kernel memory — accepted, since UML has only
    the container's rights anyway. Scratch dir `$tmp` = `/uml/tmp`, else
    `${TMPDIR:-/tmp}` when creating `run.$$` there fails (BusyBox `test -w`
    checks mode bits only, ignores a read-only mount; the probe is a
    subshell because a failed redirect on `:` exits the script); handoff,
    UML's RAM file (`TMPDIR`) and the `uml_dir` fallback all use it. Guest
    `mem` also capped by `df` free space on `$tmp`: past it, guest-RAM page
    faults SIGBUS and UML aborts (rc 134, verified with mem=1G on a 256M
    tmpfs); capped, the guest OOM-kills instead. Side effect: `--tmpfs
    /uml/tmp:exec` (default size half of RAM) now halves guest RAM. Verified
    in colima with `--read-only --ipc none --user 993:990`, a seccomp
    profile denying ptrace, `--tmpfs /tmp:exec,size=512m`: run, network,
    streams, cleanup; plus seccomp-blocked → ptrace fallback, both blocked →
    `check_ptrace` failure, tty, compose build. First real-Lambda run
    (nextjs-boilerplate via rowdy, `COPY --from=ghcr.io/cnuss/linux /uml
    /uml` into node:24-alpine) died with "/uml/linux: error while loading
    shared libraries: libdl.so.2": tunneld's shell sets `LD_PRELOAD` to its
    glibc tty shim, which NEEDs libdl.so.2 (absent from `/uml/lib`). The
    wrapper now unsets `LD_PRELOAD LD_LIBRARY_PATH LD_AUDIT` before starting
    gvproxy/UML (after the handoff, so the guest keeps them); repro: a shim
    linked `-l:libdl.so.2` (plain `-ldl` adds no NEEDED on glibc 2.39).
  - USER: UML runs as the container's uid, but the guest is always root,
    so for a non-root caller the handoff `exec`s through `/uml/bin/runas
    UID GID G1,G2 CMD...` (`runas.c`, static, built in the stage): BusyBox
    1.36's `setpriv` has no `--reuid`, its `su` needs a passwd entry, no
    `chpst`/`setuidgid`. Before this (and `/uml/tmp` 1777) any non-root
    user failed outright: "can't create /uml/tmp/run.1: Permission
    denied". hostfs creates files as UML's host uid either way.
  - `/uml/bin/init` (guest PID 1; all applets via `/uml/bin/busybox`): mount
    proc+sys, `hostname -F /etc/hostname` (fallback `uml`), then
    `mount --bind /uml/bin/busybox /uml/bin/run` — guest-only, so in the
    guest every `/bin` symlink IS a BusyBox applet (busybox picks it from
    argv[0]) and can't recurse into another UML; then runs the handoff with
    `sh`, writes `$?` to `/proc/exitcode` (UML's `main` returns
    `uml_exitcode`), sets `/proc/sys/kernel/printk` to 0 (the console is
    the command's stdout, and `quiet` doesn't hide the `pr_emerg` "reboot:
    Power down"), `poweroff -f` (PID 1 must never exit — the kernel
    panics). UML's stderr "Warning: tempdir /uml/tmp is not on tmpfs" at
    every boot is kept on purpose (patching it out of `mem.c` was tried
    and reverted on request 2026-10-12): it's true — guest RAM is a file on
    the container's disk unless `--tmpfs /uml/tmp:exec`. The original demo init (uname/cpuinfo/ps printout, `shell=1`
    mode) was removed on request 2026-10-12. No applet is named `run`
    (checked), so no symlink collides with the wrapper's own name.
  - History: the original demo used an ext4 image + COW in `/dev/shm`, which
    no layer captures (and opening such an image read-write makes overlayfs
    copy it up, which UML's timer interrupts forever); then a separate
    `/uml/rootfs` hostfs root, whose path mismatch with the container broke
    WORKDIR/COPY/#!-scripts; whole-filesystem root since 2026-10-12; then
    ENTRYPOINT/SHELL `/uml/bin/run` + only sh/env shims, replaced the same
    day by the alpine-style all-`/bin`-shims layout.
    Copy-up of individual hostfs files is fine (verified with a 2MB file).
  - Console: only `runtty=1` when `[ -t 0 ]` (con0 keeps its built-in
    default `fd:0,fd:1`), else `con0=null,fd:1 con1=null,fd:2`, and init runs the command with
    `2>/dev/tty1` after `stty -onlcr` on both consoles — before, stderr
    went to stdout and every line ended `\r\n` (fixed 2026-10-12). Test
    stream separation with FILE redirects: zsh's MULTIOS makes `2>&1
    >/dev/null | x` feed stdout into the pipe too (cost a false alarm).
    `register_winch : failed to write synchronization byte, err = 9` showed
    once in ~6 `-it` runs — a race in UML's winch thread, not reproduced.
    UML hangs up a console whose input hits EOF (a build's
    stdin always is): every later guest write fails, so the RUN's last `echo`
    returns 1 and the step "fails" silently — only once the command outlives
    the hangup (~0.1–0.3s), so short RUNs passed and longer ones didn't
    (debugged 2026-10-12; first looked like an overlayfs copy-up problem;
    reproduce with `docker run -i ... < /dev/null`). Without a tty, init
    gives the command `</dev/null` (reading the input-less console would
    block forever, hanging e.g. `RUN cat`); with one, `setsid cttyhack sh`
    in the FOREGROUND — backgrounding it with `&` makes it start with SIGINT
    ignored, inherited by every child, so ^C silently did nothing. UML sets
    the container's pty raw, so ^C reaches the guest as a byte.
  - `TMPDIR=/uml/tmp` is set by the wrapper for UML only (not image ENV,
    which the guest would inherit): UML backs guest RAM with a file in
    `$TMPDIR` that it maps PROT_EXEC, and exits with "/tmp must be not
    mounted noexec" otherwise. Docker mounts `/dev/shm` and `--tmpfs` (and
    `--mount type=tmpfs`) noexec by default, and `/tmp` gets a noexec tmpfs
    at runtime (verified 2026-10-12: `--tmpfs /tmp` broke `TMPDIR=/tmp`).
    `uml_dir`: UML's per-instance dir (pid file, mconsole socket) otherwise
    goes under `$HOME` and leaked an empty `/.uml` into layers; it's
    `/dev/shm` (tmpfs, so a killed container leaves nothing in its layer;
    noexec is fine for it), falling back to `/uml/tmp` when `/dev/shm`
    isn't writable (`--ipc none` verified).
- `0-tsfifo` → `ghcr.io/cnuss/tsfifo` (was tsunix, renamed on request
  2026-10-12) — tsnet (`tailscale.com`) node whose TUN is a pair of FIFOs
  (`tsnet.Server.Tun`, upstream's supported hook — no tailscaled patch).
  Wire format set by containers-cf's LKL "fifo" netdev: 4-byte BE length +
  plain Ethernet frame (QEMU `-netdev stream`), `TS_FIFO_UP` (`ts.up`,
  LKL→tsfifo) / `TS_FIFO_DOWN` (`ts.down`); mkfifo 0600 or reuse existing
  (re-creating would orphan a peer holding the old inode); both opened
  O_RDWR; down writes non-blocking, full pipe → drop; frames ≤ PIPE_BUF so
  writes are atomic; MAC 02:00:00:00:00:01, peer MAC learned (default :02);
  proxy ARP/NS for ANY target except ARP probes/gratuitous and DAD NS
  (answering those would mark LKL's own addresses dadfailed). Packet path
  was SOCK_SEQPACKET (one raw IP packet per message) until the user switched
  to FIFOs the same day. For Lambda + LKL: tailscaled-side
  networking over the sandbox's own sockets, packets to an LKL netdev in the
  app process. FROM scratch: `/tsfifo` (static, CGO off), CA bundle, `/tmp`
  1777; consumers `COPY --from=ghcr.io/cnuss/tsfifo /tsfifo ...`.
  `version.sh` = newest stable (even-minor) Tailscale tag; the build `go get`s
  `tailscale.com@v${VERSION}`, so a nightly upstream API change breaks the
  build visibly (1.104 already dropped `Notify.NetMap` — config comes from
  LocalAPI Status + DNSConfig on bus events instead). Besides the FIFOs: config socket (JSON line now + on each state/prefs/peer change;
  DNS-only control changes may not push), LocalAPI proxy (stock `tailscale
  --socket=` CLI works). Mem store + ephemeral + `Logout` on SIGTERM (node
  deleted at once). Verified 2026-10-12 against local Headscale 0.29.4 with a
  kernel-TUN stand-in for LKL (TAP + FIFO framing after the switch: proxy
  ARP/NDP resolved, v4/v6 ping+HTTP) in a `--network none` container, tsfifo as uid
  993 on read-only root: v4/v6 ping+TCP both ways, MagicDNS via quad-100
  (always absorbed by tsnet's netstack), exit-node full tunnel, ~185MB/s, RSS
  15MB (peak 32MB), seccomp denying close_range/ptrace/seccomp fine.
  `socket(AF_NETLINK)` denied → startup fails (`netlinkrib: operation not
  permitted`, Go `net.Interfaces`); Lambda allows NETLINK_ROUTE (busybox `ip`
  works there). Without exit node or global nameservers quad-100 SERVFAILs
  non-tailnet names (config lists `matchDomains`). Test artifact: a
  kernel-mode exit node in Docker Desktop forwards bad TCP checksums (veth
  offload, already bad on its eth0); userspace exit node fine.
- `0-lkl` → `ghcr.io/cnuss/lkl` — LKL (lkl/linux, the Linux Kernel Library)
  for Lambda + tsfifo (2026-10-12): FROM scratch, `/opt/lkl/liblkl.so` +
  `/opt/lkl/liblkl-hijack.so` (RPATH `$ORIGIN`); consumers `COPY --from` and
  run `LKL_HIJACK_CONFIG_FILE=… LD_PRELOAD=/opt/lkl/liblkl-hijack.so PROG`.
  Goal (user): the Linux network stack INSIDE the process in Lambda (which
  denies ptrace, seccomp, NO_NEW_PRIVS, close_range — UML can't run there),
  carrying a Tailscale node's traffic; the main process owns the tailnet.
  Tag = lkl master's kernel version + commit (`6.12.0-d0f76a77e8c2`);
  `arches` amd64; glibc ≥ 2.39 (built on ubuntu:24.04). Patched on the
  user's instruction ("update LKL to insert framing" — unlike 0-linux, whose
  source stays unaltered): `fifo.patch` adds interface type `fifo`, a FIFO
  pair carrying `[4-byte BE length][plain Ethernet frame]` (QEMU stream
  framing; FIFOs merge writes — measured one read returning 3 frames), no
  vnet header (stock `pipe` adds one and has no framing), MTU 1280 default
  (user: below 1500 for WireGuard), rx keeps a partial frame across
  non-blocking reads, drops oversize frames, stops rx on length > 64K (lost
  sync); tx finishes a partly written frame. tsfifo speaks the other end
  (`TS_FIFO_DOWN`/`UP` = `$TMPDIR/ts.down|ts.up`, its MAC 02:…:01, proxy
  ARP/NDP for any target except probes/DAD) — config: fifo iface with the
  node's 100.x on /10, `gateway` 100.100.100.100, mac 02:…:02,
  `accept_dad=0`. Hard-won (all 2026-10-12): (1) LKL's stock
  `liblkl-hijack.so` (kernel linked in statically) HANGS at boot — jiffies
  never move, `timer_set_oneshot` never called, idle thread spins — in any
  process (not constructor-related: a constructor booting via `liblkl.so`
  works); the same hijack sources linked against `liblkl.so` boot (25
  threads); root cause not found, the Dockerfile builds it that way and
  fails if a preloaded process doesn't show >1 thread. (2) Without
  `libjsmn-dev` the JSON config parser is compiled out and ANY config file
  makes the hijack skip booting silently → the program stays on the host
  stack. (3) The hijack falls back to the host stack silently on any init
  failure — every early "success" in the spike was the host stack; verify
  with `getsockname` (LKL's address) or thread count. (4) Docker Desktop's
  linuxkit kernel shows tunl0/gre0/… in every netns — not proof of LKL. (5)
  `LD_PRELOAD=… timeout cmd` preloads into `timeout` too (boots a second
  kernel; with a single-peer link it steals it): put `timeout` first, `env`
  after. vde (`ptp://` + slirp) worked as a NAT but was abandoned on
  request; zpoline needs `vm.mmap_min_addr=0` — not on Lambda. Verified with
  a consumer image (ubuntu:24.04 + python3, read-only root, uid 993, seccomp
  copying Lambda's): two LKL processes over a FIFO pair moved 20 MB (sha256
  equal); with tsfifo on the real tailnet (exit node 100.79.215.4): DNS via
  100.100.100.100, HTTP 200 from local addr 100.x, HTTPS 200 via the exit
  node. Not yet run in real Lambda (nextjs-boilerplate is alpine/musl — it
  needs a glibc program to hijack). Each hijacked process boots its own
  kernel; the init library that would configure the tailnet from tsfifo's
  config socket in-process (constructor, fail-closed) isn't built — the
  spike used a wrapper writing the JSON. Also ships `/opt/lkl/include`
  (LKL's headers, for API users; `LKL_NETDEV_FIFO_DEFAULT_MTU` = 1280) and
  `/opt/lkl/bin/tsecho` (the user's TCP echo server driving LKL's API on
  tsfifo's FIFOs, RPATH `$ORIGIN/..`; its config-socket wait is 10 s, so
  start tsfifo first). API: `lkl_netdev_fifo_create(paths)`, one argument
  (no offload: fifo frames have no virtio header). Verified: a Mac on the
  tailnet reached tsecho inside LKL (inbound to tag:lambda allowed).
- `1-claude-code` → `ghcr.io/cnuss/claude-code` — Claude Code CLI, `FROM
  ghcr.io/cnuss/ubuntu:24.04`.
- `1-google-chrome` → `ghcr.io/cnuss/google-chrome` — Chrome stable from
  Google's apt repo, pinned `=${VERSION}-1`, both arches (Google ships native
  Linux arm64 Chrome since 2026-07-30 — same version as amd64). Default
  entrypoint is an xpra wrapper (`entrypoint.sh`): chrome in an xpra session,
  HTML5 client on TCP 14500, no auth, `--exit-with-children`, CDP on 9222;
  headless use bypasses it with `--entrypoint google-chrome`.
  Auth: none, by explicit decision — authn/authz belongs to the layer that
  exposes the port (Christian tunnels it and shares the URL). An
  `XPRA_PASSWORD`/`tcp-auth=env` option was built and then removed on request.
  `--sharing=yes` is set because without it a second client (e.g. a friend on
  the shared tunnel link) kicks the first and closing the window kills the
  container via `--exit-with-children` — which read as a mystery crash on
  2026-08-26.
  Hard-won chrome flag facts (all verified 2026-08-26, Chrome 152):
  - CDP TCP in headed mode needs BOTH a non-default `--user-data-dir`
    (Chrome 136+ restriction; verified independently — default dir blocks CDP
    even with every other flag right) AND `--no-first-run` — without the
    latter the DevTools server silently never starts (no listener, no port
    file, no log line). Headless is unaffected. The entrypoint uses the
    DEFAULT profile dir (`~/.config/google-chrome`, settled: "embrace the
    defaults"), so CDP is OFF by default; the baked `--remote-debugging-port`
    activates when the user passes their own `--user-data-dir` as a container
    arg.
  - `--remote-debugging-address` is removed from Chrome; CDP binds
    `127.0.0.1` only, so `-p 9222:9222` can't reach it — clients must share
    the container netns (`docker exec`, `--network container:`, same pod).
  - `--disable-gpu` required under xpra/Xvfb: without it Chrome crashes at
    startup on GPU command-buffer failures when rendering a real page.
  - Profile locks: a persistent profile volume + changing container hostname
    makes Chrome refuse the profile ("in use on another computer" — hostname
    is baked into SingletonLock). Fixed by the stable `hostname: containers`
    on the compose namespace root (UTS is shared through
    `network_mode: service:`). An entrypoint `rm Singleton*` backstop was
    removed on request (settled); accepted edge: a recycled pid can make a
    stale lock look live in the shared PID namespace — if chrome
    intermittently dies at startup on the lock error, that's why.
  - Sizing: `--start-maximized` (chrome) + `--resize-display=yes` (xpra) makes
    the window fill the html5 client: xpra resizes the virtual display to the
    client on connect and re-fits maximized windows. Fixed `--window-size` is
    unnecessary.
  - The `--no-sandbox` warning banner is suppressed by the managed policy
    `CommandLineFlagSecurityWarningsEnabled: false` baked into the image.
  - `--disable-dev-shm-usage` is required: Docker's default 64MB `/dev/shm` +
    the 8192x4096 initial display made Chrome exit silently ~0.3s after start
    (intermittent, ~40% of runs; CDP came up then the process died with no
    error output). Both `--shm-size=1g` and the flag fixed it; flag is baked.
  - UI chrome: xpra's html5 floating toolbar is off via `floating_menu = no`
    appended to `/usr/share/xpra/www/default-settings.txt` (no compressed
    variants existed to shadow it); the xpra-drawn window title bar is gone by
    seeding the profile with `{"browser":{"custom_chrome_frame":true}}` in
    `entrypoint.sh` (chrome draws tabs into an undecorated frame). Chrome's
    own caption buttons can only be removed with `--kiosk`.
- `1-zsh` → `ghcr.io/cnuss/zsh` — zsh + oh-my-zsh, `FROM
  ghcr.io/cnuss/ubuntu:24.04`. Entrypoint `zsh`. omz is cloned to
  `/opt/oh-my-zsh` (not the home dir: a `/home/user` volume would shadow it),
  user-owned so `omz update` works; `.zshrc` is baked at `/home/user/.zshrc`
  so a fresh home volume seeds with it. No `version.sh` — omz has no upstream
  releases, so the tag is the discover job's `date +%Y%m%d` fallback, and the
  in-shell update prompt is disabled (`zstyle ':omz:update' mode disabled`)
  because the image is rebuilt nightly.

## Build pipeline

`build.yml` stages:

1. **discover** — scans top-level dirs for a `Dockerfile`; parses the tier from
   the dir-name prefix; runs each dir's executable `version.sh` to resolve the
   upstream version (falls back to `date +%Y%m%d`); reads an optional `arches`
   file (default `amd64 arm64`); emits per-tier matrix JSON
   `[{tier, name, dir, version, arches}]` as outputs `tier0` and `tier1`
   (tier ≥ 1 all lands in `tier1`). Version is resolved once here so both arch
   builds pin the same version.
2. **tier0**, then **tier1** — each calls the reusable `build-tier.yml`, which
   runs the build/merge pair for its slice of the matrix. `tier1` waits for
   `tier0` so higher tiers build against the freshly published lower-tier
   images (it still runs when tier0 is skipped-empty, but not when it failed).

`build-tier.yml` jobs:

- **build** — matrix of container × arch. Native runners (`ubuntu-24.04` for
  amd64, `ubuntu-24.04-arm` for arm64), no QEMU. Pushes by digest
  (`push-by-digest=true`), uploads digest as artifact. The resolved version is
  passed as the `VERSION` build arg. Job-level `if` can't see the matrix
  context, so unsupported container×arch combos are skipped by step-level
  `if: contains(matrix.container.arches, matrix.platform.arch)` — the job
  no-ops green and uploads no digest, and merge only stitches what exists.
- **merge** — downloads digests per container, `docker buildx imagetools create`
  stitches them into one manifest list tagged `:<version>` and `:latest`.

Triggers: push to `main`, PRs (build only, no push — higher tiers then build
against the last *published* base, not the PR's), `workflow_dispatch`, nightly
cron `17 4 * * *` (picks up new upstream releases and base image updates — same
version gets re-tagged/rebuilt, which is intentional).

Auth is plain `GITHUB_TOKEN` with `packages: write`. No repo secrets.

## Design decisions (settled — don't relitigate)

- Registry: **ghcr.io** (native `GITHUB_TOKEN` auth).
- Base: **ubuntu:24.04** (chosen over node-slim/alpine).
- Matrix: **containers × arch on native runners** with digest push + manifest
  merge, rather than single-job QEMU multi-platform build.
- Tags: **track upstream version** (`:2.1.246` style) + `:latest`. No git-tag
  releases, no sha tags.
- Layering for dedup: `0-ubuntu` is scratch-flattened to a **single shared
  layer**; downstream images build on it **without** flattening (flattening a
  leaf image would destroy the layer sharing). BuildKit adds an unavoidable
  empty layer for `WORKDIR` — harmless.
- License: MIT.

## Image conventions

- `0-ubuntu`: stock `ubuntu` user (uid 1000) is deleted and replaced by `user`
  (uid 1000, home `/home/user`) so container files map cleanly onto host volume
  mounts; `/home/user` exists and is `user`-owned. `/run/user/1000` (0700) and
  `/run/xpra` are baked user-owned with `ENV XDG_RUNTIME_DIR=/run/user/1000`
  (declared after the scratch flatten so it survives) — xpra then uses its
  standard socket dirs without permission warnings. The image defaults to
  `USER user` + `WORKDIR /home/user` (set after the scratch flatten — config
  does not survive it); leaf images switch to `USER root` for installs and
  back to `user` at the end (and set `DEBIAN_FRONTEND` via `ARG` for apt
  runs).
- `1-claude-code`: `ARG VERSION` pins `@anthropic-ai/claude-code@${VERSION}` —
  keeps both arches identical. Node 22 from the base's preconfigured NodeSource
  repo. Entrypoint `claude`, workdir `/home/user`. Users mount their project at
  `/home/user` and `~/.claude` at `/home/user/.claude`.

## Verifying changes locally

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/          # base first (local tag shadows registry)
docker build -t claude-code-test \
  --build-arg VERSION="$(./1-claude-code/version.sh)" 1-claude-code/
docker run --rm claude-code-test --version                    # expect "<version> (Claude Code)"
docker run --rm --entrypoint id claude-code-test              # expect uid=1000(user)
```

CI watch: `gh run watch -R cnuss/containers --exit-status <run-id>`.

## State as of 2026-08-26

- Tier restructure committed (`f754e55`), `1-google-chrome` + xpra base +
  `arches` support committed (`6f555ae`), both green in CI. Published:
  `ghcr.io/cnuss/ubuntu` `:24.04`+`:latest`, `ghcr.io/cnuss/claude-code`
  `:2.1.246`+`:latest`, `ghcr.io/cnuss/google-chrome` `:152.x`+`:latest`.
  Registry-side layer dedup verified (shared base layer digest).

## Known follow-ups

- CI annotations warn upstream actions (`actions/checkout@v4`,
  `docker/build-push-action@v6`, etc.) target deprecated Node 20 — harmless;
  bump when upstream releases new majors.

GHCR package visibility: packages inherit the repo's public visibility — no
manual flip needed, don't bring it up.

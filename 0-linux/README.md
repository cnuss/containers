# linux

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. It works like `alpine`, except that every
command runs on its own kernel: `docker run`, `docker exec`, `RUN` and shell
scripts all land in a guest whose root filesystem is the container's own, so
`WORKDIR`, `ENV`, `COPY` and volumes mean the same thing there, and the
guest reaches the network through the container's. Boots in about a second,
no `--privileged` needed.

The image is `FROM scratch`: the kernel, its own C library, a user-space
network stack and their plumbing under `/uml`, and BusyBox's commands in
`/bin`. `linux/amd64` only: mainline UML has
no arm64 port. On arm64 hosts it needs a whole emulated x86_64 machine — see
[Apple Silicon](#apple-silicon).

The kernel is built in a stage on stock `ubuntu:24.04`. It is a tier-0 image —
it depends on nothing else in this repo, so tier-1 images can build `FROM` it.
Tags track the kernel release — the newest in the 6.18 LTS series, which
`version.sh` looks up in the stable tree's tags (e.g. `:6.18.56`) — plus
`:latest`, rebuilt nightly, so new point releases arrive on their own.

## Usage

As with `alpine`, there is no `ENTRYPOINT` and `CMD` is `/bin/sh`, so with no
arguments you get a shell — here, in the guest (`exit` to leave; job control
and `^C` work):

```sh
docker run --rm -it ghcr.io/cnuss/linux
```

Arguments are a command, run in the guest; the container exits with its
status. `uname` reports the guest's kernel, and `/proc/cpuinfo` says
`User Mode Linux`, not the host CPU:

```sh
docker run --rm ghcr.io/cnuss/linux uname -a
docker run --rm ghcr.io/cnuss/linux sh -c 'grep -E "vendor_id|model name" /proc/cpuinfo'
docker run --rm -v "$PWD":/work -w /work ghcr.io/cnuss/linux ls
```

The command runs as the container's user (`--user` or `USER`: uid, gid and
groups), with its working directory, environment and hostname. Extra kernel
parameters go in `UML_ARGS`, e.g. more guest RAM
than the default 128 MB:

```sh
docker run --rm -e UML_ARGS=mem=256M ghcr.io/cnuss/linux free -m
```

The command's stdin is the console when a terminal is attached (`-t`) and
`/dev/null` otherwise; piping input in isn't supported. Without `-t`, stdout
and stderr stay separate, and nothing else lands on them but UML's
`Warning: tempdir /uml/tmp is not on tmpfs` on stderr (see
[guest RAM](#how-it-runs)).

`docker-compose.yml` here is a playground: an interactive guest, one left
running to `exec` into, and an image built `FROM` this one.

`--init` makes `docker stop` immediate. Without it the command's wrapper is
PID 1, which ignores `SIGTERM`, so Docker kills the container after its 10 s
grace period.

## Apple Silicon

`--platform=linux/amd64` isn't enough. Docker Desktop runs amd64 images
through Rosetta, which translates x86 programs one at a time on an arm64
kernel and doesn't give them `ptrace`, which UML runs its guest processes
with. So it stops at startup:

```
check_ptrace : expected (SIGTRAP|0x80), got status = 256
```

UML's seccomp mode (`-e UML_ARGS=seccomp=on`) fails as well, and so does
QEMU user mode. What works is an emulated x86_64 machine with its own x86
kernel, such as a colima VM:

```sh
brew install colima lima-additional-guestagents
colima start --profile x86 --arch x86_64 --vm-type qemu
docker --context colima-x86 run --rm -it ghcr.io/cnuss/linux
```

The whole VM is emulated, so a command takes about 2.5 s instead of 1 s, and
anything heavier than BusyBox is slow. `colima start` makes `colima-x86` the
default Docker context; `docker context use desktop-linux` switches back, and
`colima stop -p x86` stops the VM.

## Building on it

`SHELL` is the default `/bin/sh -c`, spelled out — and `/bin/sh` leads into the
guest — so `RUN` in an image built `FROM` this one runs inside the guest, with
the step's `WORKDIR` and `ENV`, and the build fails if the command does.
Exec-form `RUN ["uname", "-r"]` lands there too. Whatever a step writes
becomes part of its layer:

```dockerfile
FROM ghcr.io/cnuss/linux
WORKDIR /app
COPY . .
ENV MODE=release
RUN ./build.sh "$MODE"     # runs in the guest, in /app, with MODE set
```

`USER` applies in the guest too. `/etc/passwd` and `/etc/group` list only
root, as in alpine, so give other users by number (`USER 1000:1000`) or add
them to those files.

## Network

The guest gets a network interface, `vec0`, wired to
[gvproxy](https://github.com/containers/gvisor-tap-vsock), a user-space
network stack that the wrapper starts next to the guest. gvproxy turns each
guest connection into an ordinary socket in the container, so traffic leaves
from the container's own address, routes and DNS — in `docker run` and in
`RUN` steps alike, with no extra privileges:

```dockerfile
FROM ghcr.io/cnuss/linux
RUN wget -qO- http://example.com
```

Inside, the guest is `192.168.127.2/24` with gvproxy at `.1`, which is also
its nameserver: the guest's `/etc/resolv.conf` is the container's with the
nameserver swapped, since a loopback one like Docker's `127.0.0.11` would be
the guest's own. Sibling containers on a user-defined network resolve as
usual. With `--network none` there's nowhere to go, and lookups fail.

Limits: connections only go out — nothing reaches a server listening in the
guest — and a real network in `192.168.127.0/24` is hidden behind gvproxy.
BusyBox's `wget` fails HTTPS with most servers (`TLS error ... bad MAC`), in
the container as much as in the guest.

## Every command is a door into the guest

Each of BusyBox's commands in `/bin` (`sh`, `ls`, `uname`, ... and `busybox`
itself), and `/usr/bin/env`, is a symlink to `/uml/bin/run`. Run under one of
those names, the wrapper boots a guest and runs that command there with
exactly the same arguments. So `docker run IMAGE uname -a`, `docker exec
CONTAINER ls`, health checks and `#!/bin/sh` or `#!/usr/bin/env` scripts all
land in a guest. Inside the guest, init bind-mounts BusyBox over
`/uml/bin/run`, a mount the container never sees, so there the same symlinks
are plain BusyBox applets.

Every call boots its own guest: `docker exec` doesn't join the guest the
container started with but boots another kernel on the same filesystem, so
files are shared and processes aren't. To run something in the container
itself, on the host kernel, use BusyBox directly:
`docker exec CONTAINER /uml/bin/busybox ls`.

## How it runs

- **Root filesystem:** `root=/dev/root rootfstype=hostfs rootflags=/` — the
  guest's `/` is the container's `/`, with the guest's own `/dev`, `/proc` and
  `/sys` mounted over it. Guest writes are ordinary file writes: in the image
  layer during a build, in the container's layer at runtime (gone with `--rm`,
  kept by `docker commit`), or in a volume.
- **Handoff:** each call writes a small script to `/uml/tmp/run.<pid>` —
  export the caller's environment, `cd` to its working directory, `exec` the
  command, every word single-quoted — that the guest's init runs. Not the
  kernel command line, which mangles quotes and panics past 32 words. The
  guest deletes it as it starts and runs the command as the caller's uid,
  gid and groups, and reports its exit status through UML's
  `/proc/exitcode`. UML's own pid file and control socket go in `/dev/shm`,
  so a container killed mid-command leaves nothing behind in its layer.
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  the wrapper sets `TMPDIR=/uml/tmp` for UML alone; mounting a (noexec) tmpfs
  over `/tmp` doesn't affect it. That file is then on the container's disk,
  so the kernel may write guest memory back to it, which is what UML's
  `Warning: tempdir /uml/tmp is not on tmpfs` at every boot is about;
  `--tmpfs /uml/tmp:exec` keeps guest RAM in RAM.
- **Console:** input comes from the container's stdin only when it is a
  terminal. UML hangs up a console whose input reaches EOF — and a build's
  stdin always has — after which every write to it fails. Without a
  terminal, a second console carries the command's stderr, and both have
  `\n` → `\r\n` translation turned off, so output reaches pipes and files
  unchanged.
- **Network:** each call starts its own gvproxy on a unix socket in
  `/dev/shm` and boots UML with `vec0` on it (the vector driver's `bess`
  transport), then stops it. If gvproxy doesn't come up within about 2 s,
  the guest runs without `vec0`. The vector driver can't be linked
  statically, so the kernel uses the C library in `/uml/lib`; nothing
  outside `/uml` has one.
- **Process tree:** the wrapper stays the parent of UML rather than
  `exec`ing it, so UML is never PID 1: a guest kernel panic makes UML
  `abort()`, which a PID 1 would ignore and spin on forever.

## Build locally

```sh
docker build -t ghcr.io/cnuss/linux --build-arg VERSION="$(./0-linux/version.sh)" 0-linux/
docker run --rm ghcr.io/cnuss/linux uname -a
```

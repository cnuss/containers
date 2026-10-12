# uml

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. The image is `FROM scratch` and holds no OS at
all — just a statically linked kernel at `/uml/linux` and a BusyBox root
filesystem at `/uml/rootfs`, which the guest mounts with hostfs. Boots in about
a second, no `--privileged` needed. `linux/amd64` only: UML has no arm64 port.

The guest's `/init` prints `uname -a`, `/proc/cpuinfo` (which reports
`User Mode Linux`, not the host CPU), memory, its root mount, and its own
process table, then powers off with exit code 0.

The kernel and root disk are built in a stage on
[`ghcr.io/cnuss/ubuntu:24.04`](../0-ubuntu); none of it ships in the final
image. Tags track the kernel release, pinned to the 6.18 LTS (`:6.18`) in
`version.sh`, plus `:latest`, rebuilt nightly.

## Usage

```sh
docker run --rm --init ghcr.io/cnuss/uml
```

Boot to an interactive shell instead (`poweroff -f` to exit):

```sh
docker run --rm -it --init ghcr.io/cnuss/uml shell=1
```

Anything after the image name is appended to the UML kernel command line, so
`shell=1` lands in the guest init's environment — and kernel options work too,
e.g. `mem=256M` to give the guest more RAM.

`--init` only matters when something goes wrong: on a guest kernel panic UML
calls `abort()`, and a process running as PID 1 ignores `SIGABRT`, so without
`--init` a panicked container spins forever instead of exiting.

## Building on it

The image sets `SHELL`, so `RUN` in an image built `FROM` it runs inside the
guest — with the guest's `/bin/sh -c`, as root, from `/` — and the build fails
if the command does. Because the guest's root is a directory in the image,
whatever a `RUN` writes there becomes part of that layer:

```dockerfile
FROM ghcr.io/cnuss/uml
RUN echo 'built inside UML' > /etc/motd && uname -r > /etc/kernel-release
# COPY, WORKDIR, and friends act on the container, not the guest:
# the guest's / is /uml/rootfs
COPY app.sh /uml/rootfs/bin/app.sh
```

The same wrapper runs one command in the guest of any container:

```sh
docker run --rm --entrypoint /uml/bin/run ghcr.io/cnuss/uml 'cat /proc/cpuinfo'
```

`RUN` steps don't see the build's `ENV` or `WORKDIR`; the guest has its own
environment. `/uml/bin/run` hands the command over in a file (`/.uml-run`,
removed afterwards) rather than on the kernel command line, which mangles
quotes and panics past 32 words, and the guest reports the exit status
through UML's `/proc/exitcode`.

## How it runs

- **Root filesystem:** `root=/dev/root rootfstype=hostfs rootflags=/uml/rootfs`
  — the guest's `/` is the container's `/uml/rootfs` directory, so guest
  writes are ordinary file writes: in the image layer during a build, in the
  container's layer at runtime (gone with `--rm`, kept by `docker commit`).
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  `TMPDIR=/uml/tmp`, a directory in the image's own filesystem; mounting a
  (noexec) tmpfs over `/tmp` doesn't affect it.

## Build locally

Build the base image first, then this one:

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/
docker build -t uml --build-arg VERSION="$(./1-uml/version.sh)" 1-uml/
docker run --rm --init uml
```

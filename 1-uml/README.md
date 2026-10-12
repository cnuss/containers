# uml

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. The image is `FROM scratch` and holds no OS at
all — just a statically linked kernel at `/uml/linux` and a 32 MB BusyBox ext4
root disk at `/uml/rootfs.img`. Boots in about 3 seconds, no `--privileged`
needed. `linux/amd64` only: UML has no arm64 port.

The guest's `/init` prints `uname -a`, `/proc/cpuinfo` (which reports
`User Mode Linux`, not the host CPU), memory, block devices, and its own
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

## How it runs

- **Root disk:** `ubd0=/dev/shm/rootfs.cow,/uml/rootfs.img` — UML opens the
  image read-only and writes changes to a copy-on-write file in `/dev/shm`.
  Opening it read-write would make overlayfs copy it up, and UML's timer
  interrupts that copy-up forever. Changes are lost when the container exits.
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  `TMPDIR=/uml/tmp`, a directory in the image's own filesystem; mounting a
  (noexec) tmpfs over `/tmp` doesn't affect it.
- **Paths:** the kernel and disk live in `/uml`, not `/` — UML's COW code
  resolves the backing file's directory, and a file directly in `/` makes it
  fail with `EINVAL`.

## Build locally

Build the base image first, then this one:

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/
docker build -t uml --build-arg VERSION="$(./1-uml/version.sh)" 1-uml/
docker run --rm --init uml
```

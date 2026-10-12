//go:build linux

// Command tapbridge stands in for LKL in tests: it moves Ethernet frames
// between a kernel TAP device and tsfifo's FIFOs, applies tsfifo's config to
// the device with on-link routes, and writes the config to a file once the
// node is Running.
//
//	tapbridge UP_FIFO DOWN_FIFO CONFIG_SOCKET CONFIG_OUT
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const dev = "lkl0"

type config struct {
	State     string
	MTU       int
	Addresses []string
	Routes    []string
}

func run(args ...string) {
	if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
		log.Printf("%v: %v %s", args, err, out)
	}
}

func openTAP() *os.File {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		log.Fatal(err)
	}
	var ifr [unix.IFNAMSIZ + 64]byte
	copy(ifr[:], dev)
	*(*uint16)(unsafe.Pointer(&ifr[unix.IFNAMSIZ])) = unix.IFF_TAP | unix.IFF_NO_PI
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TUNSETIFF, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		log.Fatal(e)
	}
	return os.NewFile(uintptr(fd), dev)
}

func waitOpen(path string) *os.File {
	for {
		if f, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
			return f
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func main() {
	upPath, downPath, cfgPath, outPath := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	tap := openTAP()
	up, down := waitOpen(upPath), waitOpen(downPath)

	go func() {
		b := make([]byte, 4+65536)
		for {
			n, err := tap.Read(b[4:])
			if err != nil {
				log.Fatal(err)
			}
			binary.BigEndian.PutUint32(b, uint32(n))
			up.Write(b[:4+n])
		}
	}()
	go func() {
		r := bufio.NewReaderSize(down, 1<<17)
		b := make([]byte, 65536)
		var h [4]byte
		for {
			if _, err := io.ReadFull(r, h[:]); err != nil {
				log.Fatal(err)
			}
			n := binary.BigEndian.Uint32(h[:])
			if _, err := io.ReadFull(r, b[:n]); err != nil {
				log.Fatal(err)
			}
			tap.Write(b[:n])
		}
	}()

	var cc net.Conn
	for {
		var err error
		if cc, err = net.Dial("unix", cfgPath); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	applied := map[string]bool{}
	sc := bufio.NewScanner(cc)
	for sc.Scan() {
		log.Printf("config: %s", sc.Text())
		var cfg config
		if err := json.Unmarshal(sc.Bytes(), &cfg); err != nil || cfg.State != "Running" {
			continue
		}
		run("ip", "link", "set", dev, "up", "mtu", "1280")
		for _, a := range cfg.Addresses {
			if !applied[a] {
				run("ip", "addr", "add", a, "dev", dev)
				applied[a] = true
			}
		}
		for _, r := range cfg.Routes {
			run("ip", "route", "replace", r, "dev", dev)
		}
		if err := os.WriteFile(outPath+".tmp", sc.Bytes(), 0644); err == nil {
			os.Rename(outPath+".tmp", outPath)
		}
	}
	log.Fatalf("config socket closed: %v", sc.Err())
}

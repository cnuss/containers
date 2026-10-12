package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tailscale/wireguard-go/tun"
	"golang.org/x/sys/unix"
)

// fifoTUN is a tun.Device backed by two FIFOs carrying Ethernet frames, each
// prefixed with its 4-byte big-endian length (QEMU -netdev stream framing).
// up carries frames from the peer, down carries frames to it. ARP requests
// and IPv6 neighbor solicitations for any address are answered with
// tsfifoMAC, so the peer can route on-link without a gateway.
type fifoTUN struct {
	up     *os.File
	upr    *bufio.Reader
	frame  []byte // Read is only ever called from one goroutine
	down   int    // non-blocking: a full pipe drops the frame
	mtu    int
	events chan tun.Event
	once   sync.Once
	closed atomic.Bool

	writeMu sync.Mutex
	peerMAC atomic.Pointer[net.HardwareAddr]
}

const (
	pipeBuf      = 4096 // writes up to this size are atomic
	ethHdrLen    = 14
	maxFrameLen  = 65535
	etherTypeIP4 = 0x0800
	etherTypeARP = 0x0806
	etherTypeIP6 = 0x86dd
)

var (
	tsfifoMAC      = net.HardwareAddr{0x02, 0, 0, 0, 0, 0x01}
	defaultPeerMAC = net.HardwareAddr{0x02, 0, 0, 0, 0, 0x02}
)

// mkfifo creates a FIFO at path, or reuses the one already there so a peer
// holding it open stays connected across restarts.
func mkfifo(path string) error {
	fi, err := os.Lstat(path)
	if err == nil {
		if fi.Mode()&os.ModeNamedPipe != 0 {
			return nil
		}
		return fmt.Errorf("%s exists and is not a FIFO", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := unix.Mkfifo(path, 0600); err != nil {
		return fmt.Errorf("mkfifo %s: %w", path, err)
	}
	return nil
}

func openFIFOTUN(upPath, downPath string, mtu int) (*fifoTUN, error) {
	for _, p := range []string{upPath, downPath} {
		if err := mkfifo(p); err != nil {
			return nil, err
		}
	}
	// O_RDWR on both ends: opening never blocks, and the peer coming and
	// going never yields EOF or SIGPIPE.
	up, err := os.OpenFile(upPath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	down, err := unix.Open(downPath, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		up.Close()
		return nil, fmt.Errorf("open %s: %w", downPath, err)
	}
	t := &fifoTUN{
		up:     up,
		upr:    bufio.NewReaderSize(up, 2*maxFrameLen),
		frame:  make([]byte, maxFrameLen),
		down:   down,
		mtu:    mtu,
		events: make(chan tun.Event, 1),
	}
	t.events <- tun.EventUp
	return t, nil
}

func (t *fifoTUN) Read(slab []byte, packets []tun.ReadPacket) (int, error) {
	buf := slab[tun.ReadPacketSpacing : len(slab)-tun.ReadPacketSpacing]
	var hdr [4]byte
	frame := t.frame
	for {
		if _, err := io.ReadFull(t.upr, hdr[:]); err != nil {
			return 0, t.readErr(err)
		}
		n := int(binary.BigEndian.Uint32(hdr[:]))
		if n < ethHdrLen || n > maxFrameLen {
			log.Printf("fifo: lost sync (frame length %d), discarding buffered input", n)
			t.upr.Discard(t.upr.Buffered())
			continue
		}
		if _, err := io.ReadFull(t.upr, frame[:n]); err != nil {
			return 0, t.readErr(err)
		}
		f := frame[:n]
		src := net.HardwareAddr(f[6:12])
		if cur := t.peerMAC.Load(); cur == nil || !macEqual(*cur, src) {
			mac := append(net.HardwareAddr(nil), src...)
			t.peerMAC.Store(&mac)
		}
		etype := binary.BigEndian.Uint16(f[12:14])
		payload := f[ethHdrLen:]
		switch etype {
		case etherTypeARP:
			t.answerARP(payload)
			continue
		case etherTypeIP4:
			if len(payload) < 20 {
				continue
			}
		case etherTypeIP6:
			if len(payload) < 40 {
				continue
			}
			if t.answerNS(payload) {
				continue
			}
			dst, _ := netip.AddrFromSlice(payload[24:40])
			if dst.IsMulticast() || dst.IsLinkLocalUnicast() {
				continue
			}
		default:
			continue
		}
		if len(payload) > len(buf) {
			continue
		}
		copy(buf, payload)
		packets[0] = tun.ReadPacket{Offset: tun.ReadPacketSpacing, Size: len(payload)}
		return 1, nil
	}
}

func (t *fifoTUN) readErr(err error) error {
	if t.closed.Load() {
		return os.ErrClosed
	}
	return err
}

func macEqual(a, b net.HardwareAddr) bool { return string(a) == string(b) }

// writeFrame sends one frame toward the peer in a single write. Frames that
// don't fit in PIPE_BUF, or don't fit in the pipe right now, are dropped.
func (t *fifoTUN) writeFrame(dst net.HardwareAddr, etype uint16, payload []byte) {
	n := ethHdrLen + len(payload)
	if 4+n > pipeBuf {
		return
	}
	var b [pipeBuf]byte
	binary.BigEndian.PutUint32(b[0:4], uint32(n))
	copy(b[4:10], dst)
	copy(b[10:16], tsfifoMAC)
	binary.BigEndian.PutUint16(b[16:18], etype)
	copy(b[18:], payload)
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := unix.Write(t.down, b[:4+n]); err != nil && err != unix.EAGAIN && !t.closed.Load() {
		log.Printf("fifo: write: %v", err)
	}
}

func (t *fifoTUN) dstMAC() net.HardwareAddr {
	if m := t.peerMAC.Load(); m != nil {
		return *m
	}
	return defaultPeerMAC
}

func (t *fifoTUN) Write(bufs [][]byte, offset int) (int, error) {
	if t.closed.Load() {
		return 0, os.ErrClosed
	}
	dst := t.dstMAC()
	for _, b := range bufs {
		p := b[offset:]
		if len(p) == 0 {
			continue
		}
		etype := uint16(etherTypeIP4)
		if p[0]>>4 == 6 {
			etype = etherTypeIP6
		}
		t.writeFrame(dst, etype, p)
	}
	return len(bufs), nil
}

// answerARP replies to an IPv4-over-Ethernet ARP request for any address,
// except probes and gratuitous ARP, which announce the peer's own address.
func (t *fifoTUN) answerARP(p []byte) {
	if len(p) < 28 ||
		binary.BigEndian.Uint16(p[0:2]) != 1 || // Ethernet
		binary.BigEndian.Uint16(p[2:4]) != etherTypeIP4 ||
		p[4] != 6 || p[5] != 4 ||
		binary.BigEndian.Uint16(p[6:8]) != 1 { // request
		return
	}
	sha, spa, tpa := p[8:14], p[14:18], p[24:28]
	if string(spa) == "\x00\x00\x00\x00" || string(spa) == string(tpa) {
		return
	}
	r := make([]byte, 28)
	copy(r, p[:6])
	binary.BigEndian.PutUint16(r[6:8], 2) // reply
	copy(r[8:14], tsfifoMAC)
	copy(r[14:18], tpa)
	copy(r[18:24], sha)
	copy(r[24:28], spa)
	t.writeFrame(net.HardwareAddr(sha), etherTypeARP, r)
}

// answerNS replies to an IPv6 neighbor solicitation for any target, except
// DAD probes (unspecified source). It reports whether p was ND traffic,
// which never goes to the tailnet.
func (t *fifoTUN) answerNS(p []byte) bool {
	if p[6] != 58 || len(p) < 40+4 { // ICMPv6, no extension headers
		return false
	}
	icmp := p[40:]
	switch icmp[0] {
	case 133, 134, 136, 137: // RS, RA, NA, redirect
		return true
	case 135:
	default:
		return false
	}
	if len(icmp) < 24 {
		return true
	}
	src, _ := netip.AddrFromSlice(p[8:24])
	if src.IsUnspecified() {
		return true
	}
	target := icmp[8:24]

	na := make([]byte, 40+32)
	na[0] = 0x60
	binary.BigEndian.PutUint16(na[4:6], 32)
	na[6] = 58
	na[7] = 255
	copy(na[8:24], target)
	copy(na[24:40], p[8:24])
	m := na[40:]
	m[0] = 136
	m[4] = 0x60 // solicited, override
	copy(m[8:24], target)
	m[24] = 2 // target link-layer address option
	m[25] = 1
	copy(m[26:32], tsfifoMAC)
	binary.BigEndian.PutUint16(m[2:4], icmp6Checksum(na[8:24], na[24:40], m))

	dst := t.dstMAC()
	if len(icmp) >= 32 && icmp[24] == 1 && icmp[25] == 1 {
		dst = net.HardwareAddr(icmp[26:32]) // source link-layer address option
	}
	t.writeFrame(dst, etherTypeIP6, na)
	return true
}

func icmp6Checksum(src, dst, msg []byte) uint16 {
	var sum uint32
	add := func(b []byte) {
		for i := 0; i+1 < len(b); i += 2 {
			sum += uint32(b[i])<<8 | uint32(b[i+1])
		}
		if len(b)%2 == 1 {
			sum += uint32(b[len(b)-1]) << 8
		}
	}
	add(src)
	add(dst)
	var ph [8]byte
	binary.BigEndian.PutUint32(ph[0:4], uint32(len(msg)))
	ph[7] = 58
	add(ph[:])
	add(msg)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func (t *fifoTUN) File() *os.File           { return nil }
func (t *fifoTUN) MTU() (int, error)        { return t.mtu, nil }
func (t *fifoTUN) Name() (string, error)    { return "tailscale0", nil }
func (t *fifoTUN) Events() <-chan tun.Event { return t.events }
func (t *fifoTUN) BatchSize() int           { return 1 }

func (t *fifoTUN) Close() error {
	t.once.Do(func() {
		t.closed.Store(true)
		t.up.Close()
		t.writeMu.Lock()
		unix.Close(t.down)
		t.writeMu.Unlock()
		close(t.events)
	})
	return nil
}

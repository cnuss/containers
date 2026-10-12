package main

import (
	"errors"
	"log"
	"net"
	"os"
	"sync"

	"github.com/tailscale/wireguard-go/tun"
)

// seqpacketTUN is a tun.Device whose packets travel over a SOCK_SEQPACKET
// unix socket, one raw IPv4/IPv6 packet per message. One client at a time;
// a new connection replaces the old one. With no client, writes are dropped
// and reads block.
type seqpacketTUN struct {
	ln     *net.UnixListener
	mtu    int
	events chan tun.Event
	closed chan struct{}
	once   sync.Once

	mu      sync.Mutex
	conn    *net.UnixConn
	newConn chan struct{} // closed and replaced when conn changes
}

func listenSeqpacketTUN(path string, mtu int) (*seqpacketTUN, error) {
	os.Remove(path)
	ln, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, err
	}
	t := &seqpacketTUN{
		ln:      ln,
		mtu:     mtu,
		events:  make(chan tun.Event, 1),
		closed:  make(chan struct{}),
		newConn: make(chan struct{}),
	}
	t.events <- tun.EventUp
	go t.accept()
	return t, nil
}

func (t *seqpacketTUN) accept() {
	for {
		c, err := t.ln.AcceptUnix()
		if err != nil {
			select {
			case <-t.closed:
				return
			default:
			}
			log.Printf("tun: accept: %v", err)
			continue
		}
		log.Printf("tun: client connected")
		t.mu.Lock()
		if t.conn != nil {
			t.conn.Close()
		}
		t.conn = c
		close(t.newConn)
		t.newConn = make(chan struct{})
		t.mu.Unlock()
	}
}

func (t *seqpacketTUN) current() (*net.UnixConn, chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn, t.newConn
}

func (t *seqpacketTUN) drop(c *net.UnixConn, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == c {
		log.Printf("tun: client gone: %v", err)
		c.Close()
		t.conn = nil
	}
}

func (t *seqpacketTUN) Read(slab []byte, packets []tun.ReadPacket) (int, error) {
	buf := slab[tun.ReadPacketSpacing : len(slab)-tun.ReadPacketSpacing]
	for {
		c, changed := t.current()
		if c == nil {
			select {
			case <-changed:
				continue
			case <-t.closed:
				return 0, os.ErrClosed
			}
		}
		n, err := c.Read(buf)
		if err != nil || n == 0 {
			select {
			case <-t.closed:
				return 0, os.ErrClosed
			default:
			}
			if err == nil {
				err = errors.New("EOF")
			}
			t.drop(c, err)
			continue
		}
		packets[0] = tun.ReadPacket{Offset: tun.ReadPacketSpacing, Size: n}
		return 1, nil
	}
}

func (t *seqpacketTUN) Write(bufs [][]byte, offset int) (int, error) {
	c, _ := t.current()
	if c == nil {
		return len(bufs), nil
	}
	for _, b := range bufs {
		if _, err := c.Write(b[offset:]); err != nil {
			t.drop(c, err)
			break
		}
	}
	return len(bufs), nil
}

func (t *seqpacketTUN) File() *os.File           { return nil }
func (t *seqpacketTUN) MTU() (int, error)        { return t.mtu, nil }
func (t *seqpacketTUN) Name() (string, error)    { return "tailscale0", nil }
func (t *seqpacketTUN) Events() <-chan tun.Event { return t.events }
func (t *seqpacketTUN) BatchSize() int           { return 1 }

func (t *seqpacketTUN) Close() error {
	t.once.Do(func() {
		close(t.closed)
		t.ln.Close()
		t.mu.Lock()
		if t.conn != nil {
			t.conn.Close()
		}
		t.mu.Unlock()
		close(t.events)
	})
	return nil
}

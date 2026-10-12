// Command tsunix joins a tailnet with tsnet and exchanges the node's IP
// packets over a SOCK_SEQPACKET unix socket instead of a TUN device.
//
// Sockets (all mode 0600):
//
//	-tun     SOCK_SEQPACKET, one raw IPv4/IPv6 packet per message.
//	-config  SOCK_STREAM; each client gets the node config as one JSON line
//	         now and another on every change.
//	-socket  SOCK_STREAM LocalAPI, for `tailscale --socket=...`.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"tailscale.com/client/local"
	_ "tailscale.com/feature/oauthkey"
	"tailscale.com/ipn"
	"tailscale.com/ipn/store/mem"
	"tailscale.com/tsnet"
)

const mtu = 1280

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func osHostname() string {
	h, _ := os.Hostname()
	return h
}

func main() {
	tmp := env("TMPDIR", "/tmp")
	var (
		tunPath    = flag.String("tun", env("TS_TUN", filepath.Join(tmp, "ts.sock")), "SOCK_SEQPACKET packet socket path")
		configPath = flag.String("config", env("TS_CONFIG_SOCKET", filepath.Join(tmp, "ts-config.sock")), "node config socket path")
		apiPath    = flag.String("socket", env("TS_SOCKET", filepath.Join(tmp, "tailscaled.sock")), "LocalAPI socket path")
		stateDir   = flag.String("statedir", env("TS_STATE_DIR", filepath.Join(tmp, "tsunix")), "directory for logs; node state stays in memory")
		hostname   = flag.String("hostname", env("TS_HOSTNAME", osHostname()), "node name")
		tags       = flag.String("tags", env("TS_TAGS", ""), "comma-separated tags to advertise (required with TS_CLIENT_SECRET)")
		exitNode   = flag.String("exit-node", env("TS_EXIT_NODE", ""), "exit node IP, MagicDNS name, or auto:any")
		acceptRts  = flag.Bool("accept-routes", env("TS_ACCEPT_ROUTES", "") == "true", "accept subnet routes advertised by peers")
		verbose    = flag.Bool("verbose", env("TS_VERBOSE", "") == "true", "log tsnet internals to stderr")
	)
	flag.Parse()

	tdev, err := listenSeqpacketTUN(*tunPath, mtu)
	if err != nil {
		log.Fatalf("tun: %v", err)
	}

	srv := &tsnet.Server{
		Dir:       *stateDir,
		Store:     new(mem.Store),
		Hostname:  *hostname,
		Ephemeral: true,
		Tun:       tdev,
		UserLogf:  log.Printf,
	}
	if *verbose {
		srv.Logf = log.Printf
	}
	if *tags != "" {
		srv.AdvertiseTags = strings.Split(*tags, ",")
	}
	if err := os.MkdirAll(*stateDir, 0700); err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatalf("tsnet: %v", err)
	}
	defer srv.Close()

	lc, err := srv.LocalClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mp := &ipn.MaskedPrefs{
		Prefs:       ipn.Prefs{RouteAll: *acceptRts},
		RouteAllSet: true,
	}
	exitIP, exitIsIP := netip.ParseAddr(*exitNode)
	switch {
	case *exitNode == "auto:any":
		mp.AutoExitNode = ipn.AnyExitNode
		mp.AutoExitNodeSet = true
	case exitIsIP == nil:
		mp.ExitNodeIP = exitIP
		mp.ExitNodeIPSet = true
	}
	if _, err := lc.EditPrefs(ctx, mp); err != nil {
		log.Fatalf("prefs: %v", err)
	}

	pub := newPublisher()
	go serveConfig(*configPath, pub)
	go serveLocalAPI(*apiPath, lc)

	if err := watch(ctx, lc, pub, *exitNode); err != nil && ctx.Err() == nil {
		log.Fatalf("watch: %v", err)
	}
	// Logging out deletes an ephemeral node now instead of after control's
	// inactivity timeout.
	lctx, lcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer lcancel()
	if err := lc.Logout(lctx); err != nil {
		log.Printf("logout: %v", err)
	}
}

type dnsConfig struct {
	Nameservers []netip.Addr `json:"nameservers"`
	Search      []string     `json:"search"`
	// MatchDomains, when non-empty, are the only suffixes the nameservers
	// answer; other names need the host's resolver. Empty means the
	// nameservers answer everything.
	MatchDomains []string `json:"matchDomains,omitempty"`
}

type nodeConfig struct {
	State     string         `json:"state"`
	Error     string         `json:"error,omitempty"`
	Name      string         `json:"name,omitempty"`
	MTU       int            `json:"mtu"`
	Addresses []netip.Prefix `json:"addresses"`
	Routes    []netip.Prefix `json:"routes"`
	ExitNode  string         `json:"exitNode,omitempty"`
	DNS       *dnsConfig     `json:"dns,omitempty"`
}

var tailnetRoutes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("fd7a:115c:a1e0::/48"),
}

var defaultRoutes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/0"),
	netip.MustParsePrefix("::/0"),
}

func buildConfig(ctx context.Context, lc *local.Client, state ipn.State, errMsg string, prefs ipn.PrefsView) (nodeConfig, error) {
	c := nodeConfig{State: state.String(), Error: errMsg, MTU: mtu}
	if state != ipn.Running {
		return c, nil
	}
	st, err := lc.Status(ctx)
	if err != nil {
		return c, err
	}
	if st.Self == nil {
		return c, nil
	}
	c.Name = strings.TrimSuffix(st.Self.DNSName, ".")
	for _, ip := range st.Self.TailscaleIPs {
		c.Addresses = append(c.Addresses, netip.PrefixFrom(ip, ip.BitLen()))
	}
	c.Routes = slices.Clone(tailnetRoutes)
	if prefs.Valid() && prefs.RouteAll() {
		for _, p := range st.Peer {
			if p.PrimaryRoutes == nil {
				continue
			}
			for _, r := range p.PrimaryRoutes.All() {
				if r.Bits() != 0 {
					c.Routes = append(c.Routes, r)
				}
			}
		}
	}
	exitActive := prefs.Valid() && prefs.ExitNodeID() != ""
	if exitActive {
		c.ExitNode = string(prefs.ExitNodeID())
		c.Routes = append(c.Routes, defaultRoutes...)
	}
	slices.SortFunc(c.Routes, func(a, b netip.Prefix) int { return strings.Compare(a.String(), b.String()) })
	c.Routes = slices.Compact(c.Routes)

	if prefs.Valid() && prefs.CorpDNS() {
		dc, err := lc.DNSConfig(ctx)
		if err != nil {
			return c, err
		}
		d := &dnsConfig{
			Nameservers: []netip.Addr{netip.MustParseAddr("100.100.100.100"), netip.MustParseAddr("fd7a:115c:a1e0::53")},
		}
		var suffix string
		if st.CurrentTailnet != nil && st.CurrentTailnet.MagicDNSEnabled {
			suffix = strings.TrimSuffix(st.CurrentTailnet.MagicDNSSuffix, ".")
		}
		if suffix != "" {
			d.Search = append(d.Search, suffix)
		}
		for _, dom := range dc.Domains {
			if dom = strings.TrimSuffix(dom, "."); !slices.Contains(d.Search, dom) {
				d.Search = append(d.Search, dom)
			}
		}
		if !exitActive && len(dc.Resolvers) == 0 {
			if suffix != "" {
				d.MatchDomains = append(d.MatchDomains, suffix)
			}
			for dom := range dc.Routes {
				d.MatchDomains = append(d.MatchDomains, strings.TrimSuffix(dom, "."))
			}
			slices.Sort(d.MatchDomains)
			d.MatchDomains = slices.Compact(d.MatchDomains)
		}
		c.DNS = d
	}
	return c, nil
}

func watch(ctx context.Context, lc *local.Client, pub *publisher, exitNode string) error {
	w, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState|ipn.NotifyInitialPrefs|ipn.NotifyPeerChanges)
	if err != nil {
		return err
	}
	defer w.Close()

	var (
		state   ipn.State
		errMsg  string
		prefs   ipn.PrefsView
		exitSet = exitNode == "" || exitNode == "auto:any"
	)
	if _, err := netip.ParseAddr(exitNode); err == nil {
		exitSet = true
	}
	for {
		n, err := w.Next()
		if err != nil {
			return err
		}
		if n.State != nil {
			state = *n.State
			log.Printf("state: %v", state)
		}
		if n.ErrMessage != nil {
			errMsg = *n.ErrMessage
			log.Printf("error: %s", errMsg)
		}
		if n.BrowseToURL != nil {
			log.Printf("login required (no usable auth key?): %s", *n.BrowseToURL)
		}
		if n.Prefs != nil {
			prefs = *n.Prefs
		}
		if n.State == nil && n.Prefs == nil && n.ErrMessage == nil && n.SelfChange == nil && n.PeersChanged == nil && n.PeersRemoved == nil {
			continue
		}
		if !exitSet && state == ipn.Running {
			st, err := lc.Status(ctx)
			if err != nil {
				return err
			}
			var p ipn.Prefs
			if err := p.SetExitNodeIP(exitNode, st); err != nil {
				log.Printf("exit node %q: %v", exitNode, err)
			} else if _, err := lc.EditPrefs(ctx, &ipn.MaskedPrefs{Prefs: p, ExitNodeIDSet: true, ExitNodeIPSet: true}); err != nil {
				log.Printf("exit node %q: %v", exitNode, err)
			}
			exitSet = true
		}
		c, err := buildConfig(ctx, lc, state, errMsg, prefs)
		if err != nil {
			log.Printf("config: %v", err)
			continue
		}
		pub.set(c)
	}
}

// publisher holds the latest config line and wakes waiters on change.
type publisher struct {
	mu      sync.Mutex
	line    []byte
	changed chan struct{}
}

func newPublisher() *publisher { return &publisher{changed: make(chan struct{})} }

func (p *publisher) set(c nodeConfig) {
	b, err := json.Marshal(c)
	if err != nil {
		log.Printf("config: %v", err)
		return
	}
	b = append(b, '\n')
	p.mu.Lock()
	defer p.mu.Unlock()
	if bytes.Equal(b, p.line) {
		return
	}
	log.Printf("config: %s", bytes.TrimSpace(b))
	p.line = b
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *publisher) get() ([]byte, chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.line, p.changed
}

func listenUnix(path string) (net.Listener, error) {
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func serveConfig(path string, pub *publisher) {
	ln, err := listenUnix(path)
	if err != nil {
		log.Fatalf("config socket: %v", err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("config socket: %v", err)
			continue
		}
		go func() {
			defer c.Close()
			gone := make(chan struct{})
			go func() { io.Copy(io.Discard, c); close(gone) }()
			var sent []byte
			for {
				line, changed := pub.get()
				if line != nil && !bytes.Equal(line, sent) {
					if _, err := c.Write(line); err != nil {
						return
					}
					sent = line
				}
				select {
				case <-changed:
				case <-gone:
					return
				}
			}
		}()
	}
}

func serveLocalAPI(path string, lc *local.Client) {
	ln, err := listenUnix(path)
	if err != nil {
		log.Fatalf("localapi socket: %v", err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("localapi socket: %v", err)
			continue
		}
		go func() {
			defer c.Close()
			up, err := lc.Dial(context.Background(), "tcp", "local-tailscaled.sock:80")
			if err != nil {
				log.Printf("localapi dial: %v", err)
				return
			}
			defer up.Close()
			done := make(chan struct{}, 2)
			go func() { io.Copy(up, c); done <- struct{}{} }()
			go func() { io.Copy(c, up); done <- struct{}{} }()
			<-done
		}()
	}
}

package dlna

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// maxPendingReplies caps the answers waiting out their random delay. A flood of
// searches (or one forged to look like it came from a victim) cannot make this
// server send an unbounded amount of traffic.
const maxPendingReplies = 32

// ResponderConfig is everything a Responder needs.
type ResponderConfig struct {
	// UUID is the device identity, without "uuid:".
	UUID string
	// HTTPPort is the port the description is served on.
	HTTPPort int
	// Server is the SERVER header.
	Server string
	// Locals are the addresses to announce and answer on.
	Locals []localAddr
	Log    *slog.Logger

	// The fields below are for tests; zero values mean the real thing.

	// ListenPort replaces 1900; -1 lets the system choose (see Port).
	ListenPort int
	// Group replaces the multicast group notifications are sent to.
	Group netip.AddrPort
	// NoMulticast skips joining the multicast group.
	NoMulticast bool
	// Delay replaces the random pause before answering a search.
	Delay func(mx int) time.Duration
	// Repeat is how often alive messages are repeated; default 15 minutes.
	Repeat time.Duration
}

// Responder is an SSDP presence: it announces the device, answers searches for
// it, and says goodbye when stopped.
type Responder struct {
	cfg  ResponderConfig
	tgts []target

	conn *ipv4.PacketConn
	raw  net.PacketConn

	sendMu     sync.Mutex // one write at a time, and SetMulticastInterface applies to the next one
	pending    chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup // the read loop and the delayed answers
	announceWG sync.WaitGroup // the announce loop, finished before goodbye is sent
	stopped    sync.Once
}

// NewResponder prepares a Responder; Start opens the socket.
func NewResponder(cfg ResponderConfig) *Responder {
	if cfg.ListenPort == 0 {
		cfg.ListenPort = ssdpPort
	}
	if !cfg.Group.IsValid() {
		cfg.Group = netip.AddrPortFrom(netip.MustParseAddr(ssdpGroup), ssdpPort)
	}
	if cfg.Delay == nil {
		cfg.Delay = randomDelay
	}
	if cfg.Repeat == 0 {
		cfg.Repeat = time.Duration(maxAgeSeconds/2) * time.Second
	}
	return &Responder{
		cfg:     cfg,
		tgts:    targets(cfg.UUID),
		pending: make(chan struct{}, maxPendingReplies),
		done:    make(chan struct{}),
	}
}

// randomDelay spreads answers over the time the searcher said it would wait, so
// a room full of devices does not answer in the same instant. Half of MX at most,
// to leave the answer time to arrive.
func randomDelay(mx int) time.Duration {
	return time.Duration(rand.Int64N(int64(mx) * int64(time.Second) / 2))
}

// Start opens the socket, joins the multicast group on each interface, announces
// the device, and begins answering. It returns once announced.
func (r *Responder) Start(ctx context.Context) error {
	if len(r.cfg.Locals) == 0 {
		return errors.New("no network address to announce on")
	}
	lc := net.ListenConfig{Control: shareAddress}
	listen := max(r.cfg.ListenPort, 0)
	pc, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf(":%d", listen))
	if err != nil {
		return fmt.Errorf("listen on UDP %d: %w", r.cfg.ListenPort, err)
	}
	r.raw = pc
	r.conn = ipv4.NewPacketConn(pc)

	if !r.cfg.NoMulticast {
		if joined := r.joinGroups(); joined == 0 {
			_ = pc.Close()
			return errors.New("could not join the SSDP multicast group on any network interface")
		}
	}

	r.wg.Add(1)
	go func() { defer r.wg.Done(); r.readLoop() }()
	r.announceWG.Add(1)
	go func() { defer r.announceWG.Done(); r.announceLoop() }()
	return nil
}

// Port is the UDP port the responder listens on.
func (r *Responder) Port() int {
	if a, ok := r.raw.LocalAddr().(*net.UDPAddr); ok {
		return a.Port
	}
	return 0
}

// joinGroups joins 239.255.255.250 on every distinct interface and reports how
// many worked. One failing (a virtual adapter without multicast) is not fatal.
func (r *Responder) joinGroups() int {
	group := &net.UDPAddr{IP: r.cfg.Group.Addr().AsSlice()}
	seen := map[int]bool{}
	joined := 0
	for _, l := range r.cfg.Locals {
		if seen[l.iface.Index] {
			continue
		}
		seen[l.iface.Index] = true
		iface := l.iface
		if err := r.conn.JoinGroup(&iface, group); err != nil {
			r.cfg.Log.Warn("could not listen for TV searches on a network interface",
				slog.String("interface", iface.Name), slog.String("error", err.Error()))
			continue
		}
		joined++
	}
	return joined
}

// Stop says goodbye on every network, stops answering, and closes the socket.
// It is safe to call more than once.
func (r *Responder) Stop() {
	r.stopped.Do(func() {
		close(r.done)
		r.announceWG.Wait() // no "alive" may follow the goodbye
		r.announce(ntsBye)  // before the socket closes
		if r.raw != nil {
			_ = r.raw.Close() // ends readLoop
		}
		r.wg.Wait()
	})
}

// announceLoop announces the device when it starts, a few times because UDP
// loses packets, and again before listeners' memory of it would expire.
func (r *Responder) announceLoop() {
	for range 3 {
		r.announce(ntsAlive)
		select {
		case <-r.done:
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
	ticker := time.NewTicker(r.cfg.Repeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.announce(ntsAlive)
		}
	}
}

// announce multicasts every target on every interface, each with the address
// that interface's network can reach.
func (r *Responder) announce(nts string) {
	for _, l := range r.cfg.Locals {
		location := locationURL(l.addr, r.cfg.HTTPPort)
		for _, t := range r.tgts {
			r.send(notify(t, nts, location, r.cfg.Server), r.cfg.Group, &l.iface)
		}
	}
}

func (r *Responder) readLoop() {
	buf := make([]byte, maxSearchBytes+1)
	for {
		n, _, src, err := r.conn.ReadFrom(buf)
		if err != nil {
			return // closed by Stop
		}
		udp, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}
		from, ok := netip.AddrFromSlice(udp.IP)
		if !ok {
			continue
		}
		r.handle(buf[:n], netip.AddrPortFrom(from.Unmap(), uint16(udp.Port)))
	}
}

// handle answers one packet if it is a search for something this device is.
func (r *Responder) handle(packet []byte, from netip.AddrPort) {
	s, ok := parseSearch(packet)
	if !ok {
		return
	}
	hits := matching(r.tgts, s.st)
	if len(hits) == 0 {
		return
	}
	location, ok := locationFor(from.Addr(), r.cfg.Locals, r.cfg.HTTPPort)
	if !ok {
		return
	}
	for _, t := range hits {
		select {
		case r.pending <- struct{}{}:
		default:
			return // too many answers already waiting; searchers retry
		}
		msg := searchResponse(t, location, r.cfg.Server)
		delay := r.cfg.Delay(s.mx)
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			defer func() { <-r.pending }()
			select {
			case <-time.After(delay):
				r.send(msg, from, nil)
			case <-r.done:
			}
		}()
	}
}

// send writes one packet. For a multicast destination it first selects the
// interface to leave by, which applies to the next write only, hence the lock.
func (r *Responder) send(msg []byte, to netip.AddrPort, via *net.Interface) {
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	if via != nil && to.Addr().IsMulticast() {
		if err := r.conn.SetMulticastInterface(via); err != nil {
			r.cfg.Log.Debug("cannot send on interface", slog.String("interface", via.Name), slog.String("error", err.Error()))
			return
		}
	}
	dst := &net.UDPAddr{IP: to.Addr().AsSlice(), Port: int(to.Port())}
	if _, err := r.conn.WriteTo(msg, nil, dst); err != nil {
		r.cfg.Log.Debug("could not send an SSDP packet", slog.String("to", to.String()), slog.String("error", err.Error()))
	}
}

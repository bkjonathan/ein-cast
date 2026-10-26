package remote

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

func TestWatch_AFollowsAnAddressThatChanges(t *testing.T) {
	// A restarted quick tunnel gets a new random address. If the service kept
	// the old one, the Host check would refuse every request to the new one.
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	if !e.svc.HostAllowed("abc.trycloudflare.com") {
		t.Fatal("the first address should be allowed")
	}

	tn.set(func(f *fakeTunnel) {
		f.live = app.TunnelStatus{State: server.Running, URL: "https://new-one.trycloudflare.com"}
	})

	waitFor(t, func() bool { return e.svc.Status().URL == "https://new-one.trycloudflare.com" })
	if !e.svc.HostAllowed("new-one.trycloudflare.com") {
		t.Error("the new address is not accepted")
	}
	if e.svc.HostAllowed("abc.trycloudflare.com") {
		t.Error("the old address is still accepted although that tunnel is gone")
	}
	if got := e.svc.Status(); got.State != server.Running || !got.Public {
		t.Errorf("status = %+v: a new address is not a new run", got)
	}
}

func TestWatch_ReportsReconnectingAndRecovery(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	// Wait on the events, not on Status(): Status reads the provider directly and
	// would be true before the watcher had told anyone.
	tn.set(func(f *fakeTunnel) { f.live = app.TunnelStatus{State: server.Starting} })
	waitFor(t, func() bool { return slices.Contains(e.events.liveStates(), server.Starting) })
	if got := e.svc.Status(); got.State != server.Running || !e.svc.PublicActive() {
		t.Errorf("status = %+v: reconnecting is still on, and still public", got)
	}

	tn.set(func(f *fakeTunnel) {
		f.live = app.TunnelStatus{State: server.Running, URL: "https://abc.trycloudflare.com"}
	})
	waitFor(t, func() bool { return containsInOrder(e.events.liveStates(), server.Starting, server.Running) })
}

func containsInOrder(got []server.State, want ...server.State) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestWatch_DoesNotPublishWhenNothingChanged(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	before := len(e.events.states())

	before2 := tn.polls()
	waitFor(t, func() bool { return tn.polls() >= before2+10 }) // many polls, no change
	if after := len(e.events.states()); after != before {
		t.Errorf("%d events during idle polling, want none", after-before)
	}
}

func TestWatch_ATunnelThatGivesUpTakesRemoteAccessDown(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	sess := e.login(t, "198.51.100.1")

	tn.set(func(f *fakeTunnel) {
		f.live = app.TunnelStatus{State: server.Failed, Error: "cloudflared stopped: connection refused"}
	})

	waitFor(t, func() bool { return e.svc.Status().State == server.Failed })
	st := e.svc.Status()
	if !strings.Contains(st.Error, "connection refused") {
		t.Errorf("error = %q, want the provider's reason", st.Error)
	}
	if _, stopped := tn.counts(); stopped != 1 {
		t.Errorf("provider stopped %d times, want 1: a dead provider is still told to stop, to be sure", stopped)
	}
	if e.svc.PublicActive() || e.svc.HostAllowed("abc.trycloudflare.com") || e.svc.ValidSession(sess.Token) {
		t.Error("a tunnel that is gone is still treated as up")
	}
	if st.Provider != "" {
		t.Errorf("provider = %q: a clean failure can be retried, which the UI decides by this", st.Provider)
	}

	tn.set(func(f *fakeTunnel) { f.live = app.TunnelStatus{} })
	e.enable(t, tn) // retry works
	if e.svc.Status().State != server.Running {
		t.Error("could not start again after the tunnel gave up")
	}
}

func TestWatch_ADeadTunnelThatCannotBeConfirmedStoppedStaysPublic(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	tn.set(func(f *fakeTunnel) {
		f.live = app.TunnelStatus{State: server.Failed, Error: "died"}
		f.stopErr = errors.New("cannot signal process")
	})

	waitFor(t, func() bool { return e.svc.Status().State == server.Failed })

	if !e.svc.PublicActive() {
		t.Fatal("stopped counting as public although the stop was never confirmed")
	}
	if e.svc.Status().Provider == "" {
		t.Error("the provider must stay recorded so the UI offers to turn it off again, not to retry")
	}

	tn.set(func(f *fakeTunnel) { f.stopErr = nil })
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if e.svc.PublicActive() || e.svc.Status().State != server.Idle {
		t.Errorf("status = %+v after a confirmed stop", e.svc.Status())
	}
}

func TestWatch_StopsWhenRemoteAccessIsTurnedOff(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	waitFor(t, func() bool { return tn.polls() > 3 })

	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond) // let a poll that was already underway finish
	at := tn.polls()
	time.Sleep(100 * time.Millisecond) // twenty poll intervals

	if now := tn.polls(); now != at {
		t.Errorf("the watcher polled %d more times after Disable: it leaked", now-at)
	}
}

func TestWatch_AnOldRunsWatcherCannotAffectTheNewRun(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	oldGen := e.svc.gen
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.enable(t, tn)

	// What a slow watcher from the first run would do on its last tick.
	e.svc.observe(oldGen, tn, app.TunnelStatus{State: server.Failed, Error: "stale"})

	if got := e.svc.Status(); got.State != server.Running || !e.svc.PublicActive() {
		t.Errorf("status = %+v: an old watcher took down the new run", got)
	}
}

func TestWatch_IgnoresAProviderThatHasNothingToReport(t *testing.T) {
	// A private provider with no live URL (or a zero status) changes nothing.
	tn := privateTunnel()
	e := newEnv(t, tn)
	e.enable(t, tn)
	before := e.svc.Status().URL

	waitFor(t, func() bool { return tn.polls() > 5 })

	if got := e.svc.Status(); got.URL != before || got.State != server.Running {
		t.Errorf("status = %+v changed with nothing to report", got)
	}
}

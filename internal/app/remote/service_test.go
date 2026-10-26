package remote

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

func TestEnable_PublicProviderIsRefusedWithoutAPIN(t *testing.T) {
	tn := publicTunnel()
	e := newEnv(t, tn)

	err := e.svc.Enable(context.Background(), tn.id, localServer())

	is(t, err, app.ErrPINRequired)
	if started, _ := tn.counts(); started != 0 {
		t.Fatal("the tunnel was started before the PIN rule was checked")
	}
	if e.svc.PublicActive() || e.svc.Status().State != server.Idle {
		t.Error("a refused Enable must leave nothing exposed and nothing running")
	}
}

func TestEnable_PublicProviderIsRefusedAfterThePINIsCleared(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	if err := e.svc.ClearPIN(); err != nil {
		t.Fatal(err)
	}
	is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrPINRequired)
}

func TestEnable_PrivateProviderNeedsNoPIN(t *testing.T) {
	tn := privateTunnel()
	e := newEnv(t, tn)
	e.enable(t, tn)

	st := e.svc.Status()
	if st.State != server.Running || st.Public || st.URL != tn.url || st.Provider != tn.id {
		t.Errorf("status = %+v", st)
	}
	if e.svc.PublicActive() {
		t.Error("a private tunnel must not switch on the public-access rules")
	}
	if !e.svc.HostAllowed("den.tail1234.ts.net:8765") {
		t.Error("the tailnet address should pass the Host check")
	}
}

func TestEnable_PublicProviderWithAPIN(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	st := e.svc.Status()
	if st.State != server.Running || !st.Public || st.URL != tn.url || !st.PINSet {
		t.Errorf("status = %+v", st)
	}
	if !e.svc.PublicActive() {
		t.Error("PublicActive = false while a public tunnel runs")
	}
	if addr, err := app.LoopbackAddr(tn.gotSrv); err != nil || addr != "127.0.0.1:8765" {
		t.Errorf("tunnel was given the server at %q (%v), want its loopback address", addr, err)
	}
	if tn.lifetime().Err() != nil {
		t.Error("the tunnel's lifetime context ended while it should be running")
	}
}

func TestHostAllowed(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)

	if e.svc.HostAllowed("abc.trycloudflare.com") {
		t.Error("a host was allowed before any tunnel was online")
	}
	e.enable(t, tn)

	cases := map[string]bool{
		"abc.trycloudflare.com":              true,
		"ABC.TryCloudflare.com":              true,
		"abc.trycloudflare.com.":             true,
		"abc.trycloudflare.com:443":          false, // the URL names no port
		"evil.example":                       false,
		"trycloudflare.com":                  false,
		"abc.trycloudflare.com.evil.example": false,
		"":                                   false,
	}
	for host, want := range cases {
		if got := e.svc.HostAllowed(host); got != want {
			t.Errorf("HostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestEnable_UnknownProvider(t *testing.T) {
	e := newEnv(t)
	is(t, e.svc.Enable(context.Background(), "nope", localServer()), app.ErrUnknownProvider)
}

func TestEnable_WhileAlreadyOnIsRefused(t *testing.T) {
	tn := privateTunnel()
	e := newEnv(t, tn)
	e.enable(t, tn)

	is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrAlreadyEnabled)
	if started, _ := tn.counts(); started != 1 {
		t.Errorf("tunnel started %d times, want 1", started)
	}
}

func TestEnable_FailedStartLeavesNothingExposedAndCanBeRetried(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	tn.set(func(f *fakeTunnel) { f.startErr = errors.New("cloudflared not found") })

	err := e.svc.Enable(context.Background(), tn.id, localServer())

	if err == nil || !strings.Contains(err.Error(), "cloudflared not found") {
		t.Fatalf("error = %v", err)
	}
	st := e.svc.Status()
	if st.State != server.Failed || !strings.Contains(st.Error, "cloudflared not found") {
		t.Errorf("status = %+v, want Failed with the reason", st)
	}
	if e.svc.PublicActive() || e.svc.HostAllowed("abc.trycloudflare.com") {
		t.Error("a failed start left the server believing a tunnel is up")
	}
	if _, stopped := tn.counts(); stopped != 1 {
		t.Error("a half-started provider must be told to stop")
	}

	tn.set(func(f *fakeTunnel) { f.startErr = nil })
	e.enable(t, tn) // Failed → Starting is legal
	if e.svc.Status().State != server.Running {
		t.Error("retry after failure did not reach Running")
	}
}

func TestEnable_UnusableAddressIsAFailure(t *testing.T) {
	for _, url := range []app.PublicURL{"", "%zz", "just-a-word"} {
		t.Run(string(url), func(t *testing.T) {
			tn := publicTunnel()
			tn.url = url
			e := withPIN(t, tn)

			if err := e.svc.Enable(context.Background(), tn.id, localServer()); err == nil {
				t.Fatal("Enable accepted an address no request can ever match")
			}
			if e.svc.Status().State != server.Failed || e.svc.PublicActive() {
				t.Errorf("status = %+v", e.svc.Status())
			}
			if _, stopped := tn.counts(); stopped != 1 {
				t.Error("the tunnel was not stopped")
			}
		})
	}
}

func TestEnable_GivesUpWhenTheTunnelNeverComesUp(t *testing.T) {
	tn := publicTunnel()
	tn.block = true
	e := newEnvWith(t, newSecrets(), []*fakeTunnel{tn}, WithStartTimeout(30*time.Millisecond))
	if err := e.svc.SetPIN(testPIN); err != nil {
		t.Fatal(err)
	}

	err := e.svc.Enable(context.Background(), tn.id, localServer())

	if err == nil || !strings.Contains(err.Error(), "did not come up") {
		t.Fatalf("error = %v, want a timeout", err)
	}
	if tn.lifetime().Err() == nil {
		t.Error("the tunnel's context was left running after the timeout")
	}
	if e.svc.Status().State != server.Failed || e.svc.PublicActive() {
		t.Errorf("status = %+v", e.svc.Status())
	}
}

func TestDisable_InterruptsAStartInProgress(t *testing.T) {
	tn := publicTunnel()
	tn.block = true
	e := withPIN(t, tn)

	enableErr := make(chan error, 1)
	go func() { enableErr <- e.svc.Enable(context.Background(), tn.id, localServer()) }()
	waitFor(t, func() bool { started, _ := tn.counts(); return started == 1 })

	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	select {
	case err := <-enableErr:
		if err == nil {
			t.Error("the interrupted Enable reported success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enable is still blocked after Disable")
	}
	if st := e.svc.Status(); st.State != server.Idle || e.svc.PublicActive() {
		t.Errorf("status = %+v, want Idle and nothing exposed", st)
	}
}

func TestDisable_StopsTheTunnelAndSignsEveryoneOut(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	sess := e.login(t, "198.51.100.1")
	link := e.svc.SignStream("film")

	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, stopped := tn.counts(); stopped != 1 {
		t.Errorf("tunnel stopped %d times, want 1", stopped)
	}
	if tn.lifetime().Err() == nil {
		t.Error("the tunnel's lifetime context was not cancelled")
	}
	if e.svc.PublicActive() || e.svc.HostAllowed("abc.trycloudflare.com") {
		t.Error("still exposed after Disable")
	}
	if e.svc.ValidSession(sess.Token) {
		t.Error("a session outlived the tunnel")
	}
	if e.svc.VerifyStream("film", link.Sig, link.Expires.Unix()) {
		t.Error("a signed link outlived the tunnel")
	}
	if st := e.svc.Status(); st.State != server.Idle || st.Sessions != 0 {
		t.Errorf("status = %+v", st)
	}
}

func TestDisable_WhenOffIsNotAnError(t *testing.T) {
	tn := privateTunnel()
	e := newEnv(t, tn)
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, stopped := tn.counts(); stopped != 0 {
		t.Error("Disable touched a tunnel that was never on")
	}
}

func TestDisable_ThatCannotStopTheTunnelKeepsFailingClosed(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	tn.set(func(f *fakeTunnel) { f.stopErr = errors.New("process will not die") })

	err := e.svc.Disable(context.Background())

	if err == nil || !strings.Contains(err.Error(), "process will not die") {
		t.Fatalf("error = %v", err)
	}
	if !e.svc.PublicActive() {
		t.Fatal("the server stopped treating tunnel traffic as public although the tunnel may still be up")
	}
	if e.svc.Status().State != server.Failed {
		t.Errorf("state = %v, want Failed", e.svc.Status().State)
	}

	tn.set(func(f *fakeTunnel) { f.stopErr = nil })
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if e.svc.PublicActive() || e.svc.Status().State != server.Idle {
		t.Errorf("status = %+v after a successful retry", e.svc.Status())
	}
	if _, stopped := tn.counts(); stopped != 2 {
		t.Errorf("Stop called %d times, want 2 (the retry must reach the provider)", stopped)
	}
}

func TestEachRunStartsWithNewKeys(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	sess := e.login(t, "198.51.100.1")
	link := e.svc.SignStream("film")
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}

	e.enable(t, tn)

	if e.svc.ValidSession(sess.Token) {
		t.Error("a session from the previous run works in the next one")
	}
	if e.svc.VerifyStream("film", link.Sig, link.Expires.Unix()) {
		t.Error("a signed link from the previous run works in the next one")
	}
}

func TestEnable_RotatesKeysEvenIfNothingElseDid(t *testing.T) {
	// Disable already rotates; this pins down that Enable does not rely on it.
	tn := publicTunnel()
	e := withPIN(t, tn)
	stale := e.svc.creds.issue("198.51.100.1")
	staleLink := e.svc.SignStream("film")

	e.enable(t, tn)

	if e.svc.ValidSession(stale.Token) || e.svc.VerifyStream("film", staleLink.Sig, staleLink.Expires.Unix()) {
		t.Error("credentials minted before Enable are valid in the new run")
	}
}

func TestStatus_FollowsTheLifecycleAndReportsTheProvidersOwnState(t *testing.T) {
	tn := privateTunnel()
	e := newEnv(t, tn)
	e.enable(t, tn)
	tn.set(func(f *fakeTunnel) { f.live = app.TunnelStatus{State: server.Starting} })

	if got := e.svc.Status().Live.State; got != server.Starting {
		t.Errorf("Live.State = %v, want the provider's own (reconnecting) state", got)
	}
	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []server.State{server.Starting, server.Running, server.Stopping, server.Idle}
	if got := e.events.states(); !slices.Equal(got, want) {
		t.Errorf("published states = %v, want %v", got, want)
	}
}

func TestProviders(t *testing.T) {
	e := newEnv(t, publicTunnel(), privateTunnel())
	got := e.svc.Providers()
	if len(got) != 2 {
		t.Fatalf("providers = %+v", got)
	}
	for _, p := range got {
		if p.Public != (p.ID == "fake-public") {
			t.Errorf("provider %+v has the wrong visibility", p)
		}
	}
}

func TestPIN_Validation(t *testing.T) {
	cases := map[string]struct {
		pin string
		ok  bool
	}{
		"too short":                   {"1234567", false},
		"shortest allowed":            {"12345678", true},
		"empty":                       {"", false},
		"longest allowed":             {strings.Repeat("x", 64), true},
		"too long":                    {strings.Repeat("x", 65), false},
		"counts characters not bytes": {"สวัสดีครับผม", true}, // 12 runes, 36 bytes
		"control character":           {"abcdefgh\n", false},
		"tab":                         {"abcd\tefgh", false},
		"spaces are fine":             {"correct horse", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			err := e.svc.SetPIN(c.pin)
			if c.ok && err != nil {
				t.Fatalf("SetPIN: %v", err)
			}
			if !c.ok {
				is(t, err, app.ErrInvalidPIN)
				if e.svc.Status().PINSet {
					t.Error("a rejected PIN was stored")
				}
			}
		})
	}
}

func TestPIN_IsStoredAsAHashNeverAsText(t *testing.T) {
	e := newEnv(t)
	if err := e.svc.SetPIN(testPIN); err != nil {
		t.Fatal(err)
	}
	stored := e.secrets.stored(pinKey)
	if stored == "" || strings.Contains(stored, testPIN) || !strings.HasPrefix(stored, "v1$") {
		t.Errorf("stored value = %q", stored)
	}
	if strings.Contains(e.logs.String(), testPIN) {
		t.Errorf("the PIN reached the log:\n%s", e.logs.String())
	}
}

func TestPIN_FailureToSaveKeepsTheOldPIN(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	e.secrets.setErr = errors.New("keychain locked")

	if err := e.svc.SetPIN("a different pin"); err == nil {
		t.Fatal("SetPIN reported success although saving failed")
	}
	if _, err := e.svc.Login("198.51.100.1", testPIN); err != nil {
		t.Errorf("the old PIN stopped working: %v", err)
	}
	if _, err := e.svc.Login("198.51.100.2", "a different pin"); !errors.Is(err, app.ErrWrongPIN) {
		t.Errorf("the PIN that failed to save works: %v", err)
	}
}

func TestPIN_SurvivesARestartButFailsClosedWhenUnreadable(t *testing.T) {
	secrets := newSecrets()
	tn := publicTunnel()
	first := newEnvWith(t, secrets, []*fakeTunnel{tn})
	if err := first.svc.SetPIN(testPIN); err != nil {
		t.Fatal(err)
	}

	t.Run("restored", func(t *testing.T) {
		again := newEnvWith(t, secrets, []*fakeTunnel{tn})
		if !again.svc.Status().PINSet {
			t.Fatal("the PIN was not restored")
		}
		again.enable(t, tn)
		again.login(t, "198.51.100.1")
	})
	t.Run("corrupt value", func(t *testing.T) {
		bad := newSecrets()
		bad.m[pinKey] = "v1$not-base64$nope"
		e := newEnvWith(t, bad, []*fakeTunnel{tn})
		if e.svc.Status().PINSet {
			t.Error("a corrupt value counted as a PIN")
		}
		is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrPINRequired)
	})
	t.Run("credential store unavailable", func(t *testing.T) {
		locked := newSecrets()
		locked.getErr = errors.New("keychain locked")
		e := newEnvWith(t, locked, []*fakeTunnel{tn})
		is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrPINRequired)
		if !strings.Contains(e.logs.String(), "keychain locked") {
			t.Error("the reason was not logged")
		}
	})
}

func TestPIN_CannotBeClearedWhileAPublicTunnelIsOn(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	is(t, e.svc.ClearPIN(), app.ErrPublicActive)
	if !e.svc.Status().PINSet {
		t.Fatal("the PIN was removed while the library was public")
	}

	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ClearPIN(); err != nil {
		t.Fatalf("ClearPIN with the tunnel off: %v", err)
	}
	if e.secrets.stored(pinKey) != "" {
		t.Error("the stored hash was not deleted")
	}
}

func TestPIN_ChangingItSignsEveryoneOut(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	sess := e.login(t, "198.51.100.1")
	link := e.svc.SignStream("film")

	if err := e.svc.SetPIN("a brand new pin"); err != nil {
		t.Fatal(err)
	}

	if e.svc.ValidSession(sess.Token) || e.svc.VerifyStream("film", link.Sig, link.Expires.Unix()) {
		t.Error("credentials issued under the old PIN still work")
	}
	if _, err := e.svc.Login("198.51.100.1", testPIN); !errors.Is(err, app.ErrWrongPIN) {
		t.Errorf("old PIN: %v, want ErrWrongPIN", err)
	}
	if _, err := e.svc.Login("198.51.100.1", "a brand new pin"); err != nil {
		t.Errorf("new PIN: %v", err)
	}
}

func TestService_IsSafeToUseFromManyGoroutines(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := range 4 {
		wg.Add(1)
		go func() { // what requests do, all the time
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				e.svc.PublicActive()
				e.svc.HostAllowed("abc.trycloudflare.com")
				e.svc.Status()
				_, _ = e.svc.Login("198.51.100."+string(rune('1'+i)), "wrong")
				e.svc.SignStream(media.ID("x"))
			}
		}()
	}
	for range 5 {
		if err := e.svc.Enable(context.Background(), tn.id, localServer()); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Disable(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// waitFor polls until cond is true, failing after a generous deadline.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

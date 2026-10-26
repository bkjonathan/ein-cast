package remote

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"eincast/internal/app"
)

func TestLogin_RightPINGivesASessionThatWorks(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	sess := e.login(t, "198.51.100.1")

	if !e.svc.ValidSession(sess.Token) {
		t.Error("the new session is not valid")
	}
	if want := e.clock.now().Add(sessionTTL); !sess.Expires.Equal(want) {
		t.Errorf("expires %v, want %v", sess.Expires, want)
	}
	if got := e.svc.Status().Sessions; got != 1 {
		t.Errorf("Sessions = %d, want 1", got)
	}
}

func TestLogin_WrongPINGivesNothing(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	for _, pin := range []string{"", "wrong", testPIN + "x", testPIN[:len(testPIN)-1], " " + testPIN} {
		sess, err := e.svc.Login("198.51.100.1", pin)
		is(t, err, app.ErrWrongPIN)
		if sess.Token != "" {
			t.Errorf("PIN %q: a refused sign-in returned a token", pin)
		}
		e.clock.advance(time.Hour) // keep the rate limiter out of this test
	}
	if got := e.svc.Status().Sessions; got != 0 {
		t.Errorf("Sessions = %d after only wrong PINs", got)
	}
}

func TestLogin_NeedsAPublicTunnel(t *testing.T) {
	tn := privateTunnel()
	e := newEnv(t, tn)
	if err := e.svc.SetPIN(testPIN); err != nil {
		t.Fatal(err)
	}

	_, err := e.svc.Login("198.51.100.1", testPIN)
	is(t, err, app.ErrNotPublic)

	e.enable(t, tn) // private: still no public sign-in
	_, err = e.svc.Login("198.51.100.1", testPIN)
	is(t, err, app.ErrNotPublic)
}

func TestLogin_NoPINSetNeverLetsAnyoneIn(t *testing.T) {
	// Not reachable through Enable (it refuses), but the guard stands alone.
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	e.svc.pins.mu.Lock()
	e.svc.pins.salt, e.svc.pins.hash = nil, nil
	e.svc.pins.mu.Unlock()

	for _, pin := range []string{"", "anything at all"} {
		if _, err := e.svc.Login("198.51.100.1", pin); err == nil {
			t.Errorf("signed in with PIN %q although none is set", pin)
		}
	}
}

func TestLogin_LocksOutAfterRepeatedWrongGuesses(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	const ip = "198.51.100.1"

	for i := range loginBurst {
		_, err := e.svc.Login(ip, "wrong")
		if !errors.Is(err, app.ErrWrongPIN) {
			t.Fatalf("guess %d: %v, want ErrWrongPIN", i+1, err)
		}
	}

	// Locked: the right PIN is refused too, or the lockout would leak the answer.
	_, err := e.svc.Login(ip, testPIN)
	var locked *app.LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("after %d wrong guesses: %v, want a lockout", loginBurst, err)
	}
	if locked.RetryAfter <= 0 || locked.RetryAfter > loginRefill {
		t.Errorf("RetryAfter = %v, want within (0, %v]", locked.RetryAfter, loginRefill)
	}

	e.clock.advance(locked.RetryAfter - time.Second)
	if _, err := e.svc.Login(ip, testPIN); !errors.As(err, &locked) {
		t.Errorf("one second early: %v, want still locked", err)
	}
	e.clock.advance(2 * time.Second)
	e.login(t, ip) // the lockout lifts by itself
}

func TestLogin_OneClientsLockoutDoesNotLockOthersOut(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	for range loginBurst + 1 {
		_, _ = e.svc.Login("198.51.100.1", "wrong")
	}
	e.login(t, "198.51.100.2")
}

func TestLogin_SuccessClearsTheFailureCount(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	const ip = "198.51.100.1"

	for range 3 {
		round(t, e, ip, loginBurst-1)
		e.login(t, ip)
	}
}

// round makes n wrong guesses, all of which must be plainly wrong (not locked).
func round(t *testing.T, e *env, ip string, n int) {
	t.Helper()
	for i := range n {
		if _, err := e.svc.Login(ip, "wrong"); !errors.Is(err, app.ErrWrongPIN) {
			t.Fatalf("guess %d: %v", i+1, err)
		}
	}
}

func TestLogin_LimitsGuessingAcrossManyAddresses(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)

	// A botnet: every address makes just one guess, so no per-address limit trips.
	for i := range globalBurst {
		ip := fmt.Sprintf("203.0.113.%d", i+1)
		if _, err := e.svc.Login(ip, "wrong"); !errors.Is(err, app.ErrWrongPIN) {
			t.Fatalf("guess from %s: %v", ip, err)
		}
	}
	_, err := e.svc.Login("203.0.113.200", testPIN)
	var locked *app.LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("guess %d from a new address: %v, want the global lockout", globalBurst+1, err)
	}

	// The refused address was not charged for it.
	e.clock.advance(locked.RetryAfter + time.Second)
	e.login(t, "203.0.113.200")
}

func TestLogin_RightPINsDoNotUseUpTheGlobalBudget(t *testing.T) {
	tn := publicTunnel()
	e := withPIN(t, tn)
	e.enable(t, tn)
	for i := range 3 * globalBurst {
		e.login(t, fmt.Sprintf("198.51.100.%d", i%200+1))
	}
}

func TestLimiter(t *testing.T) {
	clk := newClock()
	l := newLimiter(clk.now, 3, time.Minute)

	for i := range 3 {
		if ok, _ := l.take("a"); !ok {
			t.Fatalf("attempt %d refused within the burst", i+1)
		}
	}
	ok, wait := l.take("a")
	if ok || wait != time.Minute {
		t.Fatalf("4th attempt: ok=%v wait=%v, want refused for a minute", ok, wait)
	}

	clk.advance(30 * time.Second)
	if ok, wait := l.take("a"); ok || wait != 30*time.Second {
		t.Errorf("after 30s: ok=%v wait=%v, want 30s still to wait", ok, wait)
	}

	clk.advance(30 * time.Second)
	if ok, _ := l.take("a"); !ok {
		t.Error("a full refill period earns one attempt back")
	}

	clk.advance(24 * time.Hour)
	l.give("a")
	l.give("a") // refunds cannot push past the burst
	for i := range 3 {
		if ok, _ := l.take("a"); !ok {
			t.Fatalf("attempt %d after a long idle was refused", i+1)
		}
	}
	if ok, _ := l.take("a"); ok {
		t.Error("the bucket holds more than its burst")
	}

	l.reset("a")
	if ok, _ := l.take("a"); !ok {
		t.Error("reset did not clear the bucket")
	}
	if ok, _ := l.take("b"); !ok {
		t.Error("keys are not independent")
	}
}

func TestLimiter_ForgetsIdleKeysSoMemoryStaysBounded(t *testing.T) {
	clk := newClock()
	l := newLimiter(clk.now, 2, time.Minute)

	for i := range maxBuckets {
		l.take(fmt.Sprintf("ip-%d", i))
	}
	clk.advance(time.Hour) // every bucket has refilled, so none carries information
	l.take("one-more")

	l.mu.Lock()
	n := len(l.buckets)
	l.mu.Unlock()
	if n > 2 {
		t.Errorf("%d buckets kept after the sweep, want only the live one", n)
	}
}

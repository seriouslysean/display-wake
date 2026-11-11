package wake

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeSystem stands in for caffeinate, ioreg and pmset. It records what the display went through.
type fakeSystem struct {
	idle    time.Duration
	idleErr error
	wakes   int
	sleeps  int
}

func (f *fakeSystem) Wake(time.Duration) error     { f.wakes++; return nil }
func (f *fakeSystem) Idle() (time.Duration, error) { return f.idle, f.idleErr }
func (f *fakeSystem) SleepDisplay() error          { f.sleeps++; return nil }
func (f *fakeSystem) Wait(time.Duration)           {}
func (f *fakeSystem) result() (wakes, sleeps int)  { return f.wakes, f.sleeps }

func newWatcher(sys System, now *time.Time) *Watcher {
	return &Watcher{
		Sys:      sys,
		Now:      func() time.Time { return *now },
		Logf:     func(string, ...any) {},
		Cooldown: 10 * time.Minute,
		WakeFor:  3 * time.Second,
		Settle:   10 * time.Second,
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Real lines from a loop: reconnect every ~26 s while the display sleeps. One wake must end it,
// not one wake per cycle and not none.
func TestLoopWakesOnceThenSleeps(t *testing.T) {
	now := time.Date(2026, 1, 1, 21, 4, 0, 0, time.UTC)
	sys := &fakeSystem{idle: time.Hour}
	if err := newWatcher(sys, &now).Run(strings.NewReader(fixture(t, "loop.log"))); err != nil {
		t.Fatal(err)
	}
	if w, s := sys.result(); w != 1 || s != 1 {
		t.Errorf("wakes, sleeps = %d, %d; want 1, 1", w, s)
	}
}

// Switching the monitor away while the display sleeps is a deferred disconnect: nothing to fix.
func TestDeferredDisconnectDoesNothing(t *testing.T) {
	now := time.Now()
	sys := &fakeSystem{idle: time.Hour}
	if err := newWatcher(sys, &now).Run(strings.NewReader(fixture(t, "switch-away.log"))); err != nil {
		t.Fatal(err)
	}
	if w, s := sys.result(); w != 0 || s != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 0, 0", w, s)
	}
}

// With the display awake macOS handles reconnects itself.
func TestAwakeReconnectDoesNothing(t *testing.T) {
	now := time.Now()
	sys := &fakeSystem{idle: time.Hour}
	if err := newWatcher(sys, &now).Run(strings.NewReader(fixture(t, "awake.log"))); err != nil {
		t.Fatal(err)
	}
	if w, s := sys.result(); w != 0 || s != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 0, 0", w, s)
	}
}

// Someone who switched back to use the Mac must not have the display put to sleep on them.
func TestRecentInputLeavesDisplayOn(t *testing.T) {
	now := time.Now()
	sys := &fakeSystem{idle: 2 * time.Second}
	if err := newWatcher(sys, &now).Run(strings.NewReader(fixture(t, "loop.log"))); err != nil {
		t.Fatal(err)
	}
	if w, s := sys.result(); w != 1 || s != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 1, 0", w, s)
	}
}

// Without an idle reading it can't tell whether someone is there, so the display stays on.
func TestUnreadableIdleLeavesDisplayOn(t *testing.T) {
	now := time.Now()
	sys := &fakeSystem{idleErr: errors.New("ioreg failed")}
	if err := newWatcher(sys, &now).Run(strings.NewReader(fixture(t, "loop.log"))); err != nil {
		t.Fatal(err)
	}
	if _, s := sys.result(); s != 0 {
		t.Errorf("sleeps = %d; want 0", s)
	}
}

// A second loop after the cooldown gets its own wake; one inside it doesn't.
func TestCooldown(t *testing.T) {
	for _, tc := range []struct {
		gap  time.Duration
		want int
	}{
		{5 * time.Minute, 1},
		{11 * time.Minute, 2},
	} {
		now := time.Now()
		sys := &fakeSystem{idle: time.Hour}
		w := newWatcher(sys, &now)
		for range 2 {
			if err := w.Run(strings.NewReader(fixture(t, "loop.log"))); err != nil {
				t.Fatal(err)
			}
			now = now.Add(tc.gap)
		}
		if got, _ := sys.result(); got != tc.want {
			t.Errorf("gap %s: wakes = %d; want %d", tc.gap, got, tc.want)
		}
	}
}

func TestParseIdle(t *testing.T) {
	// A line from `ioreg -c IOHIDSystem`.
	out := `    | | |   "HIDIdleTime" = 71312552474041` + "\n"
	got, err := ParseIdle(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := 71312552474041 * time.Nanosecond; got != want {
		t.Errorf("ParseIdle = %s; want %s", got, want)
	}
	if _, err := ParseIdle("nothing here"); err == nil {
		t.Error("ParseIdle without HIDIdleTime: want an error")
	}
}

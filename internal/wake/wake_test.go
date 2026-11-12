package wake

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeSystem stands in for caffeinate, ioreg and pmset. Time passes only in Wait, so the
// clock moves the way it would on the Mac.
type fakeSystem struct {
	now     time.Time
	idle    time.Duration
	idleErr error
	waits   []time.Duration
	wakes   int
	sleeps  int
	held    bool // the display is held on
	unheld  int  // waits that passed without the display held on
}

func (f *fakeSystem) Wake(time.Duration) (func(), error) {
	f.wakes++
	f.held = true
	return func() { f.held = false }, nil
}
func (f *fakeSystem) Idle() (time.Duration, error) { return f.idle, f.idleErr }
func (f *fakeSystem) SleepDisplay() error          { f.sleeps++; return nil }
func (f *fakeSystem) Wait(d time.Duration) {
	if !f.held {
		f.unheld++
	}
	f.waits = append(f.waits, d)
	f.now = f.now.Add(d)
}

// newRun returns a fake Mac whose clock starts at the first line of fixture, and a Watcher
// with the command's default settings.
func newRun(t *testing.T, fixture string) (*fakeSystem, *Watcher) {
	t.Helper()
	start, ok := lineTime(fixture)
	if !ok {
		t.Fatal("fixture does not start with a timestamp")
	}
	sys := &fakeSystem{now: start, idle: time.Hour}
	return sys, &Watcher{
		Sys:      sys,
		Now:      func() time.Time { return sys.now },
		Logf:     func(string, ...any) {},
		Settle:   15 * time.Second,
		MaxWakes: 3,
		Window:   10 * time.Minute,
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

// lineTime reads the timestamp that starts a `log stream --style compact` line, in local time.
func lineTime(line string) (time.Time, bool) {
	const layout = "2006-01-02 15:04:05.000"
	if len(line) < len(layout) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(layout, line[:len(layout)], time.Local)
	return t, err == nil
}

func run(t *testing.T, w *Watcher, lines string) {
	t.Helper()
	if err := w.Run(strings.NewReader(lines)); err != nil {
		t.Fatal(err)
	}
}

// Real lines from the end of a loop: one deferred reconnect, then macOS handles it once the
// display is awake. One wake and one sleep, not one per line.
func TestOneWakeEndsLoop(t *testing.T) {
	lines := fixture(t, "wake-ends-loop.log")
	sys, w := newRun(t, lines)
	run(t, w, lines)
	if sys.wakes != 1 || sys.sleeps != 1 {
		t.Errorf("wakes, sleeps = %d, %d; want 1, 1", sys.wakes, sys.sleeps)
	}
}

// A recorded loop that keeps going (four deferred reconnects): the monitor reconnects again
// after each sleep. Each retry waits twice as long, and it stops at MaxWakes instead of
// waking the display forever.
func TestLoopThatKeepsGoingGetsLongerWaitsThenStops(t *testing.T) {
	lines := fixture(t, "loop.log")
	sys, w := newRun(t, lines)
	run(t, w, lines)
	want := []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second}
	if sys.wakes != 3 || len(sys.waits) != 3 || sys.waits[0] != want[0] || sys.waits[1] != want[1] || sys.waits[2] != want[2] {
		t.Errorf("wakes %d, waits %v; want 3 wakes, waits %v", sys.wakes, sys.waits, want)
	}
}

// After a quiet Window, a new loop gets wakes again.
func TestNewWindowAllowsWakesAgain(t *testing.T) {
	lines := fixture(t, "loop.log")
	sys, w := newRun(t, lines)
	run(t, w, lines)
	sys.now = sys.now.Add(11 * time.Minute)
	run(t, w, fixture(t, "wake-ends-loop.log"))
	if sys.wakes != 4 {
		t.Errorf("wakes = %d; want 4 (3, then 1 in the next window)", sys.wakes)
	}
}

// Switching the monitor away while the display sleeps is a deferred disconnect: nothing to fix.
func TestDeferredDisconnectDoesNothing(t *testing.T) {
	lines := fixture(t, "switch-away.log")
	sys, w := newRun(t, lines)
	run(t, w, lines)
	if sys.wakes != 0 || sys.sleeps != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 0, 0", sys.wakes, sys.sleeps)
	}
}

// With the display awake macOS handles reconnects itself.
func TestAwakeReconnectDoesNothing(t *testing.T) {
	lines := fixture(t, "awake.log")
	sys, w := newRun(t, lines)
	run(t, w, lines)
	if sys.wakes != 0 || sys.sleeps != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 0, 0", sys.wakes, sys.sleeps)
	}
}

// Someone who switched back to use the Mac must not have the display go dark on them: it is
// held on through the whole wait, then left on and handed back to macOS's own display sleep.
func TestRecentInputLeavesDisplayOn(t *testing.T) {
	lines := fixture(t, "wake-ends-loop.log")
	sys, w := newRun(t, lines)
	sys.idle = 2 * time.Second
	run(t, w, lines)
	if sys.wakes != 1 || sys.sleeps != 0 {
		t.Errorf("wakes, sleeps = %d, %d; want 1, 0", sys.wakes, sys.sleeps)
	}
	if sys.unheld != 0 || sys.held {
		t.Errorf("waits without the display held %d, still held after %v; want 0, false", sys.unheld, sys.held)
	}
}

// Without an idle reading it can't tell whether someone is there, so the display stays on.
func TestUnreadableIdleLeavesDisplayOn(t *testing.T) {
	lines := fixture(t, "wake-ends-loop.log")
	sys, w := newRun(t, lines)
	sys.idleErr = errors.New("ioreg failed")
	run(t, w, lines)
	if sys.sleeps != 0 {
		t.Errorf("sleeps = %d; want 0", sys.sleeps)
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

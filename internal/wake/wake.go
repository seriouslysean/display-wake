// Package wake ends the loop a monitor can fall into when it reconnects while the Mac's
// display is asleep.
//
// macOS defers that reconnect ("Deferred hotplugs cannot be processed in a non-wake state")
// and never brings the link up, so some monitors keep dropping and reconnecting. Waking the
// display once lets macOS handle the reconnect; the display can then go back to sleep.
package wake

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kind is what a log line says about the display connection.
type Kind int

// The kinds of line Watcher reacts to.
const (
	Other    Kind = iota // a line that changes nothing
	In                   // the display connected
	Out                  // the display disconnected
	Deferred             // macOS held the last hotplug event because the display is asleep
)

// Predicate is the log stream filter for the lines Parse reads.
const Predicate = `subsystem == "com.apple.SkyLight" AND category == "display" AND eventMessage CONTAINS[c] "hotplug"`

// Lines from WindowServer, for example:
//
//	Inserting hotplug event for state "out", display id: 4
//	Replacing existing hotplug event for state "out" with "in", display id: 4
//	Deferred hotplugs cannot be processed in a non-wake state
var (
	insertRe  = regexp.MustCompile(`hotplug event for state "(in|out)", display`)
	replaceRe = regexp.MustCompile(`hotplug event for state "(?:in|out)" with "(in|out)"`)
)

const deferredText = "Deferred hotplugs cannot be processed"

// Parse reports what one log line says.
func Parse(line string) Kind {
	if strings.Contains(line, deferredText) {
		return Deferred
	}
	m := replaceRe.FindStringSubmatch(line)
	if m == nil {
		m = insertRe.FindStringSubmatch(line)
	}
	switch {
	case m == nil:
		return Other
	case m[1] == "in":
		return In
	default:
		return Out
	}
}

// System is what Watcher needs from the Mac.
type System interface {
	// Wake turns the display on and keeps it from idle sleep for up to d, or until
	// release is called.
	Wake(d time.Duration) (release func(), err error)
	// Idle is the time since the last keyboard or mouse input.
	Idle() (time.Duration, error)
	// SleepDisplay puts the display to sleep now.
	SleepDisplay() error
	// Wait pauses for d.
	Wait(d time.Duration)
}

// Watcher reads hotplug log lines and wakes the display when a reconnect is deferred.
//
// A monitor can drop and reconnect a few seconds after the display goes to sleep, which
// starts the loop again. So a deferred reconnect after Watcher's own sleep gets another wake
// with twice the wait, up to MaxWakes in each Window.
type Watcher struct {
	Sys      System
	Now      func() time.Time
	Logf     func(format string, args ...any)
	Settle   time.Duration // first wait before sleeping; the display sleeps only if there was no input for the whole wait
	MaxWakes int           // wakes allowed in one Window
	Window   time.Duration

	last        Kind
	windowStart time.Time
	wakes       int
}

// Run reads log lines from r until it ends and returns any read error. A failed wake or
// sleep is logged and Run carries on.
func (w *Watcher) Run(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		switch k := Parse(sc.Text()); k {
		case In, Out:
			w.last = k
		case Deferred:
			if w.last == In {
				w.handle()
			}
		}
	}
	return sc.Err()
}

func (w *Watcher) handle() {
	now := w.Now()
	if w.windowStart.IsZero() || now.Sub(w.windowStart) >= w.Window {
		w.windowStart, w.wakes = now, 0
	}
	if w.wakes >= w.MaxWakes {
		w.Logf("deferred reconnect: %d wakes in %s already, skipped", w.wakes, w.Window)
		return
	}
	wait := w.Settle << w.wakes
	w.wakes++
	w.Logf("deferred reconnect: waking display (wake %d of %d, wait %s)", w.wakes, w.MaxWakes, wait)
	// The display stays held on for the whole wait. A wake that only declares user activity
	// lets macOS turn the display off again as soon as that activity ends, even while
	// someone who just switched back is starting to use the Mac.
	release, err := w.Sys.Wake(wait)
	if err != nil {
		w.Logf("wake failed: %v", err)
		return
	}
	w.Sys.Wait(wait)
	idle, err := w.Sys.Idle()
	release()
	if err != nil {
		w.Logf("idle time unreadable, display left on: %v", err)
		return
	}
	if idle < wait {
		w.Logf("input %s ago: display left on", idle.Round(time.Second))
		return
	}
	if err := w.Sys.SleepDisplay(); err != nil {
		w.Logf("display sleep failed: %v", err)
		return
	}
	w.Logf("no input for %s: display put to sleep", idle.Round(time.Second))
}

var idleRe = regexp.MustCompile(`"HIDIdleTime" = (\d+)`)

// ParseIdle reads HIDIdleTime (nanoseconds) from `ioreg -c IOHIDSystem` output.
func ParseIdle(ioreg string) (time.Duration, error) {
	m := idleRe.FindStringSubmatch(ioreg)
	if m == nil {
		return 0, fmt.Errorf("no HIDIdleTime in ioreg output")
	}
	ns, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("HIDIdleTime %q: %w", m[1], err)
	}
	return time.Duration(ns), nil
}

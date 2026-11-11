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
	// Wake declares user activity for d, which turns the display on.
	Wake(d time.Duration) error
	// Idle is the time since the last keyboard or mouse input.
	Idle() (time.Duration, error)
	// SleepDisplay puts the display to sleep now.
	SleepDisplay() error
	// Wait pauses for d.
	Wait(d time.Duration)
}

// Watcher reads hotplug log lines and wakes the display once for each deferred reconnect.
type Watcher struct {
	Sys      System
	Now      func() time.Time
	Logf     func(format string, args ...any)
	Cooldown time.Duration // minimum time between wakes
	WakeFor  time.Duration // how long the wake lasts
	Settle   time.Duration // wait before checking for input; also the idle time that counts as nobody there

	last  Kind
	woken time.Time
}

// Run reads log lines from r until it ends. It returns the first error from r or from the
// system; a failed wake or sleep is logged and Run carries on.
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
	if !w.woken.IsZero() && now.Sub(w.woken) < w.Cooldown {
		w.Logf("deferred reconnect: within cooldown, skipped")
		return
	}
	w.woken = now
	w.Logf("deferred reconnect: waking display for %s", w.WakeFor)
	if err := w.Sys.Wake(w.WakeFor); err != nil {
		w.Logf("wake failed: %v", err)
		return
	}
	w.Sys.Wait(w.Settle)
	idle, err := w.Sys.Idle()
	if err != nil {
		w.Logf("idle time unreadable, display left on: %v", err)
		return
	}
	if idle < w.Settle {
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

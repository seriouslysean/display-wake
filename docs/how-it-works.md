# How it works

## The loop

A KVM switch (or a dock) hands a DisplayPort monitor back to a Mac whose display is asleep. The Mac never sleeps here, only its display. macOS sees the monitor reconnect, but while the display is asleep it holds the event back. WindowServer logs:

```text
Replacing existing hotplug event for state "out" with "in", display id: 4
Deferred hotplugs cannot be processed in a non-wake state
```

The link never comes up, so the Mac never tells the monitor it is asleep. Some monitors then light up to look for a signal, give up, drop the connection and reconnect, every 26 seconds or so, until something wakes the display. A display that goes to sleep while the monitor is connected does not loop: macOS tells the monitor first.

## What display-wake does

It reads `log stream` for WindowServer's hotplug lines. When a reconnect ("in") is deferred:

1. It wakes the display for 3 seconds (`caffeinate -u`). macOS handles the reconnect.
2. It waits 10 seconds and reads the time since the last keyboard or mouse input (`HIDIdleTime` from `ioreg`).
3. If nobody touched the Mac, it puts the display back to sleep (`pmset displaysleepnow`), which tells the monitor to rest. Otherwise it leaves the display on.

It wakes at most once every 10 minutes. A deferred disconnect (switching the monitor away) is ignored. Flags change the timings: `-cooldown`, `-wake`, `-settle`.

Expect one short flash when the monitor comes back, then a dark screen.

## Limits

- It depends on WindowServer's log wording. A macOS update that changes it stops display-wake from acting; nothing else breaks. The test fixtures in `internal/wake/testdata/` are real lines to compare against.
- macOS has no public API to stream another process's log or to sleep only the display, so it runs `/usr/bin/log` and `/usr/bin/pmset`.
- It can't stop a monitor that loops for some other reason while the display is awake.

# Background

Why display-wake exists: the problem, what was tried or ruled out first, and why a small tool was still needed. Researched and tested in November 2026 on a Mac Studio (macOS 27.2) with a Samsung Odyssey G7 (LC32G7xT) on DisplayPort, shared with a second computer through a KVM switch.

## The problem

The Mac stays on all the time; only its display sleeps. When the KVM switch hands the monitor back to the Mac while the Mac's display is asleep, the monitor starts flashing: it lights up, finds no picture, goes dark, and does it again about every 26 seconds. It keeps going until something wakes the display. One loop ran for 14 hours overnight.

The Mac's own log shows why. WindowServer sees the monitor come back, then logs:

```text
Deferred hotplugs cannot be processed in a non-wake state
```

macOS holds the reconnect until the display wakes, so it never sends the monitor a picture or tells it to sleep. The monitor sees a connected computer with no signal, gives up after about 20 seconds, drops the connection for about 5 seconds, and reconnects. Every reconnect is deferred again.

What the logs showed over a week:

| | Count |
|---|---|
| Display sleeps | 33 |
| Display sleeps with no reconnect events after them | 30 |
| Loops | 4, each after the switch came back to the Mac while its display was asleep |

A loop can also start right after the display goes to sleep. In one test the display slept 6 seconds after a wake; the monitor dropped and came back 4 seconds later, after the display was already asleep, and the loop began again. When a wake lasted 10 seconds, the monitor's drop and return both happened while the display was on, and the night stayed quiet.

Both sides are needed for the loop: macOS deferring the reconnect, and a monitor that keeps reconnecting instead of waiting in standby. The KVM switch only supplies the disconnect and reconnect; a dock did the same before it.

## What was considered

| Option | Result | Why |
|---|---|---|
| The monitor's Auto Source Switch | Not applicable | Both computers share one monitor input through the switch. One user on the Apple thread below reports that turning on input auto-switching fixed a similar loop; it doesn't apply to a single shared input. |
| Press a key after switching back | Partly works | One wake ends the loop, but a sleep that comes too soon after it can start the loop again (tested, see above). It also relies on remembering. |
| Leave the switch on the other computer | Rejected | The Mac's link drops once and stays quiet, but it means not switching back. |
| Turn the monitor off by its power button | Rejected | It works around the monitor instead of fixing the cause, and needs a press every time. |
| Change Mac sleep settings | Can't fix it | System sleep is off on purpose here. A shorter display-sleep timer doesn't help, because the loop starts when the display is already asleep. Turning display sleep off keeps the monitor lit all the time. |
| Power the monitor off over DDC | Can't fix it | DDC needs the link that the loop is missing, and `m1ddc` 1.3.1 has no power command. |
| EDID emulator dongle | Unlikely to work | A dongle powered by the video port shows as disconnected when a KVM switches away from it. NTI says so for its own DP 1.4 emulator ("Not recommended for use in conjunction with KVM or video-only switches"). |
| DisplayPort hotplug maintainer | Kept as a fallback | NTI's DP-HP-MNTR-SRC (about $150) says the computer keeps seeing the monitor as connected when the switch moves away. 240 Hz is not on its spec sheet. Untested here. |
| A small watcher that wakes the display once | Built | It does what the successful test did by hand: a short wake when the reconnect is deferred, then a proper sleep. No hardware and no cost. |

## Existing tools

Nothing found that handles this case. Searched in November 2026:

| Tool | What it does | Fits? |
|---|---|---|
| [asmoker/mac-wake-display](https://github.com/asmoker/mac-wake-display) | Go library that wakes the display | No trigger; only the wake step |
| [pbkhrv/os-x-sleep-wake-display](https://github.com/pbkhrv/os-x-sleep-wake-display) | `displayctl sleep` and `wake` commands (last change 2013) | No trigger; `pmset` and `caffeinate` already do both |
| [Eamo5/DisplayWake](https://github.com/Eamo5/DisplayWake) | Flashes a white screen to stop an OLED dimming | A different problem |
| [toobuntu/blackoutd](https://github.com/toobuntu/blackoutd) | Blacks out a MacBook's built-in display when an external one is connected | A different problem; its debug logs show the same deferred-hotplug line, and it ignores display changes while asleep |
| [sleepwatcher](https://www.bernhard-baehr.de/) | Runs scripts on sleep, wake and idle | Doesn't see monitor reconnects |
| [Hammerspoon](https://www.hammerspoon.org/) | Automation with display-change watchers | Its watcher relies on macOS display-change events, which macOS holds back while the display sleeps (inferred from the log line, not tested); it could only run this same log check as a script |
| BetterDisplay, Lunar | Display managers | No feature for this found; their docs were not read in full |

GitHub searches for the log line and for related phrases ("deferred hotplug macos", "kvm switch mac display sleep", "mac external monitor keeps waking") turned up only blackoutd.

Related reports: Apple's developer forum thread [Ventura 13.4 External Display Sleep Loop Bug](https://developer.apple.com/forums/thread/730112) describes the same kind of loop on DisplayPort and USB-C, mostly LG monitors, on macOS 13.4 to 14.4. One poster says Apple's feedback reply was "Works as currently designed"; some say macOS 14.1 fixed it, others that it didn't. Samsung's community thread [Monitor won't stay off in sleep mode](https://community.samsung.com/us/conversations/Monitors-and-Memory/Monitor-won-t-stay-off-in-sleep-mode/us_2552750) reports Samsung monitors flashing on every few seconds; Samsung support reportedly said it "cannot be validated". Neither thread names a tool or a lasting fix.

## Why a tool was required

The cause is inside macOS: it won't handle a monitor reconnect while the display is asleep, and Apple's reply to one report treats that as designed. The monitor's side can't be changed, the switch is needed, and the settings above either don't reach the cause or give up something wanted. The one action that ends the loop, a short wake that lasts long enough, needs something to notice the deferred reconnect, and nothing found does that.

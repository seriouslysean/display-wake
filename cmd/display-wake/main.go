// Command display-wake wakes a sleeping Mac display once when macOS defers a monitor
// reconnect, so a monitor that keeps reconnecting can rest. Run it as a launchd user agent:
// `display-wake install` writes and loads one; `display-wake uninstall` removes it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/seriouslysean/display-wake/internal/wake"
)

// Label is the launchd label and the plist's file name.
const Label = "display-wake"

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return install()
		case "uninstall":
			return uninstall()
		case "version":
			fmt.Println(version)
			return nil
		}
	}
	fs := flag.NewFlagSet("display-wake", flag.ContinueOnError)
	settle := fs.Duration("settle", 15*time.Second, "first wait before the display goes back to sleep; doubles on each retry")
	maxWakes := fs.Int("max-wakes", 3, "wakes allowed in one window")
	window := fs.Duration("window", 10*time.Minute, "how long max-wakes covers")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cmd := exec.Command("/usr/bin/log", "stream", "--style", "compact", "--predicate", wake.Predicate)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("log stream: %w", err)
	}
	log.Printf("watching for deferred monitor reconnects (up to %d wakes per %s)", *maxWakes, *window)
	w := &wake.Watcher{
		Sys:      mac{},
		Now:      time.Now,
		Logf:     log.Printf,
		Settle:   *settle,
		MaxWakes: *maxWakes,
		Window:   *window,
	}
	if err := w.Run(out); err != nil {
		return err
	}
	// log stream never ends on its own; exit non-zero so launchd starts it again.
	return errors.Join(errors.New("log stream ended"), cmd.Wait())
}

// mac runs the macOS commands behind wake.System.
type mac struct{}

// Wake holds a display-sleep assertion (caffeinate -d), then declares user activity
// (caffeinate -u) to turn the display on. Without the hold, macOS turns the display off
// again when the user-activity assertion times out. The -t cap ends the hold if release
// is never called.
func (mac) Wake(d time.Duration) (func(), error) {
	secs := int((d + holdMargin).Round(time.Second) / time.Second)
	hold := exec.Command("/usr/bin/caffeinate", "-d", "-t", strconv.Itoa(secs))
	if err := hold.Start(); err != nil {
		return nil, fmt.Errorf("caffeinate -d: %w", err)
	}
	release := func() {
		_ = hold.Process.Kill()
		_ = hold.Wait()
	}
	if err := exec.Command("/usr/bin/caffeinate", "-u", "-t", "1").Run(); err != nil {
		release()
		return nil, fmt.Errorf("caffeinate -u: %w", err)
	}
	return release, nil
}

// holdMargin keeps the hold past the wait so it is still in place when the watcher decides.
const holdMargin = 5 * time.Second

func (mac) Idle() (time.Duration, error) {
	out, err := exec.Command("/usr/sbin/ioreg", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0, err
	}
	return wake.ParseIdle(string(out))
}

func (mac) SleepDisplay() error {
	return exec.Command("/usr/bin/pmset", "displaysleepnow").Run()
}

func (mac) Wait(d time.Duration) { time.Sleep(d) }

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func logPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Logs", Label+".log"), nil
}

func install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	p, err := plistPath()
	if err != nil {
		return err
	}
	lp, err := logPath()
	if err != nil {
		return err
	}
	_ = launchctl("bootout", target()) // replace a loaded copy; not loaded is fine
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(Plist(exe, lp)), 0o644); err != nil {
		return err
	}
	if err := launchctl("bootstrap", domain(), p); err != nil {
		return err
	}
	fmt.Printf("installed %s\nlog: %s\n", p, lp)
	return nil
}

func uninstall() error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	_ = launchctl("bootout", target()) // not loaded is fine
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf("removed %s\n", p)
	return nil
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }
func target() string { return domain() + "/" + Label }

func launchctl(args ...string) error {
	cmd := exec.Command("/bin/launchctl", args...)
	cmd.Stdout, cmd.Stderr = io.Discard, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launchctl %v: %w", args, err)
	}
	return nil
}

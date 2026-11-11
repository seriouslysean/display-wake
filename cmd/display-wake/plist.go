package main

import (
	"bytes"
	"encoding/xml"
)

var version = "dev"

// Plist is the launchd user agent that runs exe and appends its output to logFile.
func Plist(exe, logFile string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + Label + `</string>
  <key>ProgramArguments</key><array><string>` + esc(exe) + `</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>30</integer>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>` + esc(logFile) + `</string>
  <key>StandardErrorPath</key><string>` + esc(logFile) + `</string>
</dict>
</plist>
`
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

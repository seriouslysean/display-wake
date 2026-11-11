package main

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// launchd refuses a malformed plist, and a path with & or < would break the XML.
func TestPlistIsValidXMLWithOddPaths(t *testing.T) {
	exe := "/Users/test-user/bin & tools/display-wake"
	p := Plist(exe, "/Users/test-user/Library/Logs/display-wake.log")
	d := xml.NewDecoder(strings.NewReader(p))
	var strs []string
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("plist is not valid XML: %v", err)
		}
		if cd, ok := tok.(xml.CharData); ok {
			strs = append(strs, string(cd))
		}
	}
	if !strings.Contains(strings.Join(strs, "\n"), exe) {
		t.Errorf("plist does not carry the executable path %q", exe)
	}
}

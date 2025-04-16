// Fichier: tests/main_integration_test.go
package tests

import (
    "os"
    "os/exec"
    "strings"
    "testing"
)

// TestMainHelp runs "go run github.com/nil0x42/dnsanity -h"
// to verify the help message from main.go. Comments in English.
func TestMainHelp(t *testing.T) {
    cmd := exec.Command("go", "run", "github.com/nil0x42/dnsanity", "-h")
    cmd.Env = os.Environ()
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("Error running 'go run github.com/nil0x42/dnsanity -h': %v\nOutput:\n%s", err, string(out))
    }
    if !strings.Contains(string(out), "Usage:") {
        t.Errorf("Expected 'Usage:' in the help output.\nGot:\n%s", string(out))
    }
}

// TestMainVersion checks the version output.
func TestMainVersion(t *testing.T) {
    cmd := exec.Command("go", "run", "github.com/nil0x42/dnsanity", "-version")
    cmd.Env = os.Environ()
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("Error running 'go run github.com/nil0x42/dnsanity -version': %v\nOutput:\n%s", err, string(out))
    }
    if !strings.Contains(string(out), "DNSanity") {
        t.Errorf("Expected 'DNSanity' in version output.\nGot:\n%s", string(out))
    }
}

// Fichier: internal/runner/runner_test.go

package runner

import (
    "bytes"
    "os"
    "strings"
    "testing"

    "github.com/nil0x42/dnsanity/internal/dns"
)

// helper to capture os.Stderr
func captureStderrRunner(f func()) string {
    backupStderr := os.Stderr
    r, w, _ := os.Pipe()
    os.Stderr = w

    f()

    w.Close()
    var buf bytes.Buffer
    _, _ = buf.ReadFrom(r)
    os.Stderr = backupStderr
    return buf.String()
}

// mock DNSAnswer for testing
var mockTests = []dns.DNSAnswer{
    {Domain: "example.com", Status: "NOERROR"},
    {Domain: "invalid.tld", Status: "NXDOMAIN"},
}

func TestRunAndReport(t *testing.T) {
    output := captureStderrRunner(func() {
        // The function returns the server states
        srvStates := RunAndReport(
            "\033[0;32m",        // color
            "Testing servers",   // message
            false,              // verbose
            []string{"8.8.8.8", "1.1.1.1"}, // servers
            mockTests,          // tests
            50,                 // globRateLimit
            10,                 // maxThreads
            2,                  // rateLimit
            2,                  // timeout
            0,                  // maxFailures => drop on first fail
            2,                  // maxAttempts
        )
        if len(srvStates) != 2 {
            t.Errorf("Expected 2 server states, got %d", len(srvStates))
        }
    })
    // check that it prints the "Testing servers" message
    if !strings.Contains(output, "Testing servers") {
        t.Errorf("Expected 'Testing servers' in output, got:\n%s", output)
    }
}

func TestRunAndReportVerbose(t *testing.T) {
    output := captureStderrRunner(func() {
        _ = RunAndReport(
            "\033[0;34m",
            "Verbose test",
            true, // verbose
            []string{"8.8.8.8"},
            mockTests,
            10,
            5,
            2,
            2,
            -1, // never disable server
            1,
        )
    })
    // if verbose is true, we expect some lines describing config
    if !strings.Contains(output, "Run: 1 servers * 2 tests") {
        t.Errorf("Expected verbose output with '1 servers * 2 tests', got:\n%s", output)
    }
    if !strings.Contains(output, "Each server: max 2 req/s, never dropped.") {
        t.Errorf("Expected 'never dropped' message for maxFailures = -1, got:\n%s", output)
    }
}


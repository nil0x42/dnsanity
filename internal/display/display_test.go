package display

import (
    "bytes"
    "os"
    "strings"
    "testing"

    "github.com/nil0x42/dnsanity/internal/dns"
    "github.com/nil0x42/dnsanity/internal/dnsanitize"
)

// captureStdout redirige temporairement os.Stdout vers un buffer
func captureStdout(f func()) string {
    backup := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    f()

    w.Close()
    var buf bytes.Buffer
    _, _ = buf.ReadFrom(r)
    os.Stdout = backup
    return buf.String()
}

// captureStderr redirige temporairement os.Stderr vers un buffer
func captureStderr(f func()) string {
    backup := os.Stderr
    r, w, _ := os.Pipe()
    os.Stderr = w

    f()

    w.Close()
    var buf bytes.Buffer
    _, _ = buf.ReadFrom(r)
    os.Stderr = backup
    return buf.String()
}

// ========================
// Tests des progress bars
// ========================

func TestNoTTYProgressReporter(t *testing.T) {
    // NoTTYProgressReporter écrit sur Stderr
    output := captureStderr(func() {
        pr := NewNoTTYProgressReporter(5)
        pr.Update(1, 0, 1)
        pr.Update(2, 1, 0)
        pr.Finish()
    })
    if !strings.Contains(output, "Starting (5 reqs scheduled so far)") {
        t.Errorf("Missing 'Starting (5 reqs scheduled so far)' in:\n%s", output)
    }
    if !strings.Contains(output, "Finished (3/6 reqs done).") {
        t.Errorf("Missing 'Finished (3/6 reqs done).' in:\n%s", output)
    }
}

func TestTTYProgressReporter(t *testing.T) {
    // TTYProgressReporter écrit aussi sur Stderr
    output := captureStderr(func() {
        pr := NewTTYProgressReporter("\033[0;31m", 5)
        pr.Update(2, 0, 0)
        pr.Update(1, 1, 0)
        pr.Finish()
    })
    // On vérifie qu'on a bien obtenu quelque chose
    if len(output) == 0 {
        t.Errorf("Expected some output from TTYProgressReporter, got empty.")
    }
}

// ========================
// Tests des ReportX
// ========================

func TestReportValidResults(t *testing.T) {
    // ReportValidResults écrit sur Stdout
    servers := []dnsanitize.ServerContext{
        {IPAddress: "8.8.8.8", Disabled: false},
        {IPAddress: "1.1.1.1", Disabled: true},
        {IPAddress: "9.9.9.9", Disabled: false},
    }
    captured := captureStdout(func() {
        ReportValidResults(servers, "")
    })
    // On s'attend à y voir 8.8.8.8 et 9.9.9.9
    if !strings.Contains(captured, "8.8.8.8") {
        t.Errorf("Missing 8.8.8.8 in output:\n%s", captured)
    }
    if !strings.Contains(captured, "9.9.9.9") {
        t.Errorf("Missing 9.9.9.9 in output:\n%s", captured)
    }
    // 1.1.1.1 ne doit pas apparaître
    if strings.Contains(captured, "1.1.1.1") {
        t.Errorf("Disabled server 1.1.1.1 should not appear, but was found in:\n%s", captured)
    }
}

func TestReportAllResults(t *testing.T) {
    // ReportAllResults écrit aussi sur Stdout
    servers := []dnsanitize.ServerContext{
        {IPAddress: "8.8.8.8", Disabled: false},
        {IPAddress: "1.1.1.1", Disabled: true},
    }
    captured := captureStdout(func() {
        ReportAllResults(servers, "")
    })
    // On s'attend à "+ 8.8.8.8" et "- 1.1.1.1"
    if !strings.Contains(captured, "+ 8.8.8.8") {
        t.Errorf("Expected '+ 8.8.8.8', got:\n%s", captured)
    }
    if !strings.Contains(captured, "- 1.1.1.1") {
        t.Errorf("Expected '- 1.1.1.1', got:\n%s", captured)
    }
}

func TestReportDetails(t *testing.T) {
    // ReportDetails écrit sur Stderr
    template := []dns.DNSAnswer{
        {Domain: "example.org", Status: "NOERROR"},
        {Domain: "test.org", Status: "SERVFAIL"},
    }
    servers := []dnsanitize.ServerContext{
        {IPAddress: "8.8.8.8", FailedCount: 0}, // valid
        {IPAddress: "1.1.1.1", FailedCount: 2}, // invalid
    }

    captured := captureStderr(func() {
        ReportDetails(template, servers)
    })
    if !strings.Contains(captured, "[+] SERVER 8.8.8.8 (valid)") {
        t.Errorf("Expected 8.8.8.8 valid, missing in:\n%s", captured)
    }
    if !strings.Contains(captured, "[-] SERVER 1.1.1.1 (invalid)") {
        t.Errorf("Expected 1.1.1.1 invalid, missing in:\n%s", captured)
    }
}

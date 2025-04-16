package tty

import (
    "bytes"
    "os"
    "strings"
    "testing"
)

// Because we are likely not in a real TTY, IsTTY should return false for os.Stderr or a file.
func TestIsTTY(t *testing.T) {
    // We'll check if it doesn't panic and returns a bool.
    isTerm := IsTTY(os.Stderr)
    t.Logf("IsTTY(os.Stderr) returned %v (likely false in CI)", isTerm)
    // We won't do a strict assert on the result, just ensure no panic
}

func TestStripAnsi(t *testing.T) {
    // Some ANSI codes
    input := "\033[0;31mHello\033[0m World\n\033[1;32mGreen\033[0m"
    stripped := StripAnsi(input)
    if strings.Contains(stripped, "\033[") {
        t.Errorf("Expected no ANSI codes in stripped string, got: %q", stripped)
    }
    // Check that the text remains
    if !strings.Contains(stripped, "Hello") || !strings.Contains(stripped, "World") || !strings.Contains(stripped, "Green") {
        t.Errorf("Some text was incorrectly removed. Output: %q", stripped)
    }
}

func TestSmartFprintf(t *testing.T) {
    // We'll capture output to a pipe (which is not a TTY).
    backupStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    ansiText := "\033[0;31mRED\033[0m"
    _, _ = SmartFprintf(os.Stdout, ansiText)

    w.Close()
    var buf bytes.Buffer
    _, _ = buf.ReadFrom(r)
    os.Stdout = backupStdout

    result := buf.String()
    // Because the pipe is not TTY, ANSI codes must be stripped.
    if strings.Contains(result, "\033[0;31m") {
        t.Errorf("Expected stripped ANSI, got: %q", result)
    }
    if !strings.Contains(result, "RED") {
        t.Errorf("Expected 'RED' text in output, got: %q", result)
    }
}

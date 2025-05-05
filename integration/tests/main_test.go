// Fichier : integration/tests/main_integration_test.go
package tests

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "testing"
    "time"

    "github.com/nil0x42/dnsanity/internal/config"
)

// binaryPath holds the path of the test‑compiled dnsanity binary.
var binaryPath string

// TestMain compiles the dnsanity CLI once for the whole test‑suite.
// This avoids paying the “go run” build cost for every individual
// sub‑test and guarantees we always test the exact same binary.
func TestMain(m *testing.M) {
    tmpDir, err := os.MkdirTemp("", "dnsanity‑bin")
    if err != nil {
        panic(err)
    }
    defer os.RemoveAll(tmpDir)

    binName := "dnsanity_test_bin"
    if runtime.GOOS == "windows" {
        binName += ".exe"
    }
    binaryPath = filepath.Join(tmpDir, binName)

    build := exec.Command("go", "build", "-o", binaryPath, "github.com/nil0x42/dnsanity")
    build.Stdout, build.Stderr = os.Stdout, os.Stderr
    if err := build.Run(); err != nil {
        panic("failed to compile dnsanity test binary: " + err.Error())
    }

    os.Exit(m.Run())
}

// runCLI executes the pre‑built dnsanity binary with the supplied
// arguments and returns (stdout+stderr, exit‑code).
func runCLI(t *testing.T, args ...string) (string, int) {
    t.Helper()

    var out bytes.Buffer
    cmd := exec.Command(binaryPath, args...)
    cmd.Stdout = &out
    cmd.Stderr = &out

    // We never want a stray dnsanity instance to hang the test run.
    cmd.Env = os.Environ()
    err := cmd.Start()
    if err != nil {
        t.Fatalf("starting command: %v", err)
    }

    done := make(chan struct{})
    go func() {
        _ = cmd.Wait()
        close(done)
    }()

    select {
    case <-done:
        // no‑op
    case <-time.After(10 * time.Second):
        // Kill runaway processes so the test suite always terminates.
        _ = cmd.Process.Kill()
        t.Fatalf("command timed‑out: dnsanity %v", args)
    }

    exitCode := cmd.ProcessState.ExitCode()

    return out.String(), exitCode
}

/* --------------------------  ACTUAL TESTS  -------------------------- */

func TestHelpOutput(t *testing.T) {
    // Both short and long help flags should behave identically.
    for _, flag := range []string{"-h", "--help"} {
        flag := flag // capture
        t.Run(flag, func(t *testing.T) {
            out, code := runCLI(t, flag)
            if code != 0 {
                t.Fatalf("expected exit‑code 0, got %d\n%s", code, out)
            }
            // Sanity‑check the most important sections so that a future
            // maintainer cannot accidentally remove or rename them.
            for _, want := range []string{
                "Usage:",
                "GENERIC OPTIONS:",
                "SERVERS SANITIZATION:",
                "TEMPLATE VALIDATION:",
            } {
                if !bytes.Contains([]byte(out), []byte(want)) {
                    t.Errorf("help text missing %q section\nFull output:\n%s", want, out)
                }
            }
        })
    }
}

func TestVersionMatchesConstant(t *testing.T) {
    out, code := runCLI(t, "-version")
    if code != 0 {
        t.Fatalf("expected exit‑code 0, got %d\n%s", code, out)
    }
    want := config.VERSION
    if !bytes.Contains([]byte(out), []byte(want)) {
        t.Fatalf("version output %q does not contain constant %q", out, want)
    }
}

func TestUnknownFlagFails(t *testing.T) {
    out, code := runCLI(t, "--definitely‑unknown‑flag")
    if code == 0 {
        t.Fatalf("expected non‑zero exit‑code with unknown flag\n%s", out)
    }
    if !bytes.Contains([]byte(out), []byte("flag provided but not defined")) {
        t.Errorf("unexpected stderr for unknown flag:\n%s", out)
    }
}

func TestMissingServerListFailsFast(t *testing.T) {
    out, code := runCLI(t /* no ‑list flag */)
    if code == 0 {
        t.Fatalf("expected failure without -list flag\n%s", out)
    }
    // We check for a fragment rather than the exact phrasing to avoid
    // brittle tests if wording changes slightly.
    if !bytes.Contains([]byte(out), []byte("server list")) {
        t.Errorf("expected error mentioning 'server list', got:\n%s", out)
    }
}

//go:build !windows
// +build !windows

package tty

/*
   Comprehensive white‑box tests for the tty package.
   All code paths – including ANSI stripping, TTY detection cache,
   OpenTTY fallback, and SmartFprintf stripping logic – are exercised.
   The auxiliary openPTY() helper relies on github.com/google/goterm/term
   so the tests run unchanged on Linux and macOS where /dev/ptmx is
   available.  On CI systems lacking a PTY device the relevant tests are
   skipped gracefully.
*/

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/goterm/term"
)

// openPTY is a tiny wrapper around term.OpenPTY so we can re‑use the
// function name already referenced in the existing test suite.  It
// returns the master and slave *os.File handles exactly like the
// original helper did.
func openPTY() (*os.File, *os.File, error) {
	p, err := term.OpenPTY()
	if err != nil {
		return nil, nil, err
	}
	return p.Master, p.Slave, nil
}

// TestStripAnsi verifies both removal of ANSI sequences and idempotency
// when no sequences are present.
func TestStripAnsi(t *testing.T) {
	colored := "\033[31mRED\033[0m and \033[32mGREEN\033[0m"
	want := "RED and GREEN"
	if got := StripAnsi(colored); got != want {
		t.Fatalf("StripAnsi failed: want %q, got %q", want, got)
	}

	plain := "no-color text"
	if got := StripAnsi(plain); got != plain {
		t.Fatalf("StripAnsi modified clean string: got %q", got)
	}
}

// TestIsTTYExhaustive exercises every branch of IsTTY, including the
// caching fast‑path and concurrent access (both negative and positive
// cases).
func TestIsTTYExhaustive(t *testing.T) {
	// 1. Non-*os.File writer should never be a TTY.
	if IsTTY(&bytes.Buffer{}) {
		t.Fatal("IsTTY returned true for non-*os.File writer")
	}

	// 2. *os.File that is NOT a TTY: pipe writer.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})

	if IsTTY(w) {
		t.Fatal("IsTTY returned true for pipe writer")
	}
	// Cached path should return the same value.
	if IsTTY(w) {
		t.Fatal("IsTTY cached value changed unexpectedly (pipe)")
	}

	// 3. Positive path using a pseudo‑terminal when available.
	ptmx, pts, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo‑terminal available: %v", err)
	}
	t.Cleanup(func() {
		ptmx.Close()
		pts.Close()
	})

	if !IsTTY(pts) {
		t.Fatal("IsTTY returned false for pty slave")
	}
	// Cached path (second call) – still true.
	if !IsTTY(pts) {
		t.Fatal("IsTTY cached value changed unexpectedly (pty)")
	}

	// 4. Concurrency check on both negative (pipe) and positive (pty) paths.
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				if IsTTY(w) {
					t.Error("IsTTY concurrently returned true for pipe writer")
				}
			} else {
				if !IsTTY(pts) {
					t.Error("IsTTY concurrently returned false for pty slave")
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestOpenTTY ensures OpenTTY behaves correctly.  In CI environments
// /dev/tty is usually absent; the test is skipped in that case.
func TestOpenTTY(t *testing.T) {
	f := OpenTTY()
	if f == nil {
		t.Skip("/dev/tty not available on this runner")
	}
	t.Cleanup(func() { f.Close() })

	if !IsTTY(f) {
		t.Fatal("OpenTTY returned file considered non‑TTY by IsTTY")
	}
}

// TestSmartFprintf verifies ANSI stripping on non‑TTY and preservation on
// TTY when a pseudo‑terminal is available.
func TestSmartFprintf(t *testing.T) {
	const ansi = "\033[35mMAGENTA\033[0m"

	// ---------- Non‑TTY path ----------
	r, w, _ := os.Pipe()
	t.Cleanup(func() { r.Close(); w.Close() })

	n, err := SmartFprintf(w, "%s", ansi)
	if err != nil {
		t.Fatalf("SmartFprintf(pipe) error: %v", err)
	}
	w.Close() // EOF for reader

	data, _ := io.ReadAll(r)
	out := string(data)
	if strings.Contains(out, "\033[") {
		t.Fatalf("ANSI codes not stripped on non‑TTY: %q", out)
	}
	if out != "MAGENTA" || n != len(out) {
		t.Fatalf("Unexpected output/len mismatch: out=%q n=%d", out, n)
	}

	// ---------- TTY path ----------
	ptmx, pts, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo‑terminal available: %v", err)
	}
	t.Cleanup(func() { ptmx.Close(); pts.Close() })

	if !IsTTY(pts) {
		t.Fatal("IsTTY returned false for pty slave")
	}
	if n, err := SmartFprintf(pts, "%s", ansi); err != nil || n != len(ansi) {
		t.Fatalf("SmartFprintf(pty) error=%v n=%d", err, n)
	}

	// Allow the kernel to flush the write.
	time.Sleep(10 * time.Millisecond)

	buf := make([]byte, 64)
	m, _ := ptmx.Read(buf)
	got := string(buf[:m])
	if !strings.Contains(got, ansi) {
		t.Fatalf("ANSI codes unexpectedly stripped on TTY: %q", got)
	}
}

//go:build !windows

package tty

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

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
// caching fast-path and concurrent access.
func TestIsTTYExhaustive(t *testing.T) {
	// 1. Non-*os.File writer.
	if IsTTY(&bytes.Buffer{}) {
		t.Fatal("IsTTY returned true for non-*os.File writer")
	}

	// 2. *os.File that is NOT a TTY: pipe writer.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if IsTTY(w) {
		t.Fatal("IsTTY returned true for pipe writer")
	}
	// Cached path.
	if IsTTY(w) {
		t.Fatal("IsTTY cached value changed unexpectedly")
	}

	// 3. Concurrency check.
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if IsTTY(w) {
				t.Error("IsTTY returned true inside goroutine")
			}
		}()
	}
	wg.Wait()
}

// TestOpenTTY ensures OpenTTY behaves correctly.  In CI environments
// /dev/tty is usually absent; the test is skipped in that case.
func TestOpenTTY(t *testing.T) {
	f := OpenTTY()
	if f == nil {
		t.Skip("/dev/tty not available")
	}
	defer f.Close()

	if !IsTTY(f) {
		t.Fatal("OpenTTY returned file considered non-TTY by IsTTY")
	}
}

// TestSmartFprintf verifies ANSI stripping on non-TTY and preservation on
// TTY when a pseudo-terminal is available.
func TestSmartFprintf(t *testing.T) {
	const ansi = "\033[35mMAGENTA\033[0m"

	// ---------- Non-TTY path ----------
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()

	n, err := SmartFprintf(w, "%s", ansi)
	if err != nil {
		t.Fatalf("SmartFprintf(pipe) error: %v", err)
	}
	w.Close() // EOF for reader

	data, _ := io.ReadAll(r)
	out := string(data)
	if strings.Contains(out, "\033[") {
		t.Fatalf("ANSI codes not stripped on non-TTY: %q", out)
	}
	if out != "MAGENTA" || n != len(out) {
		t.Fatalf("Unexpected output/len mismatch: out=%q n=%d", out, n)
	}

	// ---------- TTY path ----------
	ptmx, pts, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	defer ptmx.Close()
	defer pts.Close()

	if !IsTTY(pts) {
		t.Fatal("IsTTY returned false for pty slave")
	}
	if _, err := SmartFprintf(pts, "%s", ansi); err != nil {
		t.Fatalf("SmartFprintf(pty) error: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // kernel flush
	buf := make([]byte, 64)
	n, _ = ptmx.Read(buf)
	if !strings.Contains(string(buf[:n]), ansi) {
		t.Fatalf("ANSI codes unexpectedly stripped on TTY: %q", string(buf[:n]))
	}
}


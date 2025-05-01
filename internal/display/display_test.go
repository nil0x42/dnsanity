//go:build !integration
// +build !integration

package display

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/tty"
)

/* --------------------------------------------------------------------- */
/* Helpers                                                               */
/* --------------------------------------------------------------------- */

// newStatusNoTTY returns a Status with all fancy output désactivé.
func newStatusNoTTY() *Status {
	return NewStatus(
		"unit-test",
		1, 1,            // 1 serveur, 1 test
		100, 10, 1, 1,   // ratelimits & timeout
		0, 1,            // maxFailures, maxAttempts
		io.Discard,      // OutFile
		&bytes.Buffer{}, // DebugFile
		nil,             // TTYFile (désactive barre/spinner)
		false, "",       // debug off
	)
}

// newStatusWithPipe active la barre de progression via un faux TTY.
func newStatusWithPipe() (*Status, *os.File, *os.File) {
	r, w, _ := os.Pipe() // r: test reader ; w: faux TTY
	st := NewStatus(
		"pbar-test",
		2, 1,
		50, 4, 1, 1,
		0, 1,
		io.Discard,
		nil,
		w,
		false, "",
	)
	return st, r, w
}

/* --------------------------------------------------------------------- */
/* Unit tests                                                            */
/* --------------------------------------------------------------------- */

func TestScaleValue(t *testing.T) {
	// total = 0 => 0
	if got := scaleValue(5, 0, 100); got != 0 {
		t.Fatalf("scaleValue expected 0, got %v", got)
	}
	// 50 / 100 sur échelle 100 => 50
	if got := scaleValue(50, 100, 100); got != 50 {
		t.Fatalf("scaleValue 50/100*100 should be 50, got %v", got)
	}
}

func TestMakeBarRuneCountAndNoPanic(t *testing.T) {
	s := &Status{TotalServers: 100, ValidServers: 30, InvalidServers: 20}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("makeBar panicked: %v", r)
		}
	}()
	bar := s.makeBar()
	cleanBar := tty.StripAnsi(bar)
	if n := utf8.RuneCountInString(cleanBar); n != 60 {
		t.Fatalf("progress bar len invalid: %d runes (max 60)", n)
	}
	// Cas « barre terminée » : tous les serveurs traités
	s.ValidServers, s.InvalidServers = 70, 30
	bar = s.makeBar()
	if !strings.Contains(bar, "⣿") {
		t.Error("completed bar should contain full blocks (⣿)")
	}
}

// TestAddPBarSpinnerCycling vérifie que le spinner avance vraiment
// d’un frame à l’autre et injecte les bons glyphes.
func TestAddPBarSpinnerCycling(t *testing.T) {
	// Two distinct frames so curSpinnerID can advance.
	s := &Status{
		Spinner: [][]string{
			{"A", "B"}, // frame 0
			{"C", "D"}, // frame 1
		},
	}
	bar := "h1\nh2\nline1\nline2\n" // ≥2 lignes utiles
	first  := s.addPBarSpinner(bar) // utilise frame 0
	second := s.addPBarSpinner(bar) // utilise frame 1

	if first == second {
		t.Fatal("spinner frame did not advance")
	}

	linesFirst  := strings.Split(first,  "\n")
	linesSecond := strings.Split(second, "\n")

	if !strings.HasPrefix(linesFirst[2],  "A ") ||
	   !strings.HasPrefix(linesFirst[3],  "B ") {
		t.Errorf("unexpected glyphs in first frame: %q / %q",
			linesFirst[2], linesFirst[3])
	}
	if !strings.HasPrefix(linesSecond[2], "C ") ||
	   !strings.HasPrefix(linesSecond[3], "D ") {
		t.Errorf("unexpected glyphs in second frame: %q / %q",
			linesSecond[2], linesSecond[3])
	}
}

// TestGetLastSecReqCountAndLogRequests s’assure que les lots vieux de plus
// d’une seconde sont purgés, et que le compteur reflète uniquement
// l’activité récente.
func TestGetLastSecReqCountAndLogRequests(t *testing.T) {
	st, r, w := newStatusWithPipe() // hasPBar == true
	defer func() {
		st.Stop()
		r.Close()
		w.Close()
	}()

	// 3 requêtes récentes (<1 s)
	st.LogRequests(time.Now().Add(-500*time.Millisecond), 3)
	// 5 requêtes plus anciennes (>1 s) – doivent être écartées
	st.LogRequests(time.Now().Add(-1500*time.Millisecond), 5)

	if n := st.getLastSecReqCount(); n != 3 {
		t.Fatalf("expected 3 recent requests, got %d", n)
	}
	if len(st.LastRequests) != 1 {
		t.Fatalf("old batches not purged, len=%d", len(st.LastRequests))
	}
}

func TestWithLockConcurrency(t *testing.T) {
	s := &Status{}
	const n = 100
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			s.WithLock(func() { s.TotalChecks++ })
			wg.Done()
		}()
	}
	wg.Wait()
	if s.TotalChecks != n {
		t.Fatalf("WithLock lost updates: %d != %d", s.TotalChecks, n)
	}
}

func TestReportFinishedServerCountersAndOutput(t *testing.T) {
	outBuf, dbgBuf := &bytes.Buffer{}, &bytes.Buffer{}
	st := NewStatus(
		"report-srv",
		1, 1, 10, 2, 1, 1,
		0, 1,
		outBuf, dbgBuf, nil,
		true, "", // debug activé
	)

	// --- serveur OK ------------------------------------------------------
	srv := &dns.ServerContext{
		IPAddress:      "1.2.3.4",
		FailedCount:    0,
		CompletedCount: 1,
		Checks: []dns.CheckContext{{
			Passed: true,
			Answer: dns.DNSAnswer{Domain: "x", Status: "NOERROR"},
		}},
	}
	st.ReportFinishedServer(srv)
	if st.ValidServers != 1 || st.InvalidServers != 0 {
		t.Fatalf("counters wrong after valid server: %+v", st)
	}
	if !strings.Contains(outBuf.String(), "1.2.3.4") {
		t.Error("OutFile does not contain server IP")
	}
	if !strings.Contains(dbgBuf.String(), "[+] SERVER") {
		t.Error("Debug dump missing for valid server")
	}

	// --- serveur KO ------------------------------------------------------
	srvKO := &dns.ServerContext{
		IPAddress:      "9.9.9.9",
		FailedCount:    2,
		Disabled:       true,
		CompletedCount: 1,
		Checks: []dns.CheckContext{{
			Passed: false,
			Answer: dns.DNSAnswer{Domain: "x", Status: "SERVFAIL"},
		}},
	}
	st.ReportFinishedServer(srvKO)
	if st.InvalidServers != 1 {
		t.Fatalf("InvalidServers counter not incremented, got %d", st.InvalidServers)
	}
}

func TestUpdateAndRedrawPBarWritesToTTY(t *testing.T) {
	st, r, w := newStatusWithPipe()
	defer func() {
		st.Stop()
		r.Close()
		w.Close()
	}()

	// On simule une progression
	st.WithLock(func() {
		st.DoneChecks = 1
		st.TotalChecks = 1
		st.NumServersInPool, st.PoolSize = 0, 2
	})

	st.UpdateAndRedrawPBar()
	// Lecture non bloquante (pipe may contain multiple frames)
	time.Sleep(10 * time.Millisecond)
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if n == 0 {
		t.Fatal("no data written to pseudo-TTY")
	}
	if !bytes.Contains(buf[:n], []byte("1/1")) {
		t.Fatal("progress bar output does not reflect updated counters")
	}
}


package report

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nil0x42/dnsanity/internal/config"
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/tty"
)

/* --------------------------------------------------------------------- */
/* Helpers                                                               */
/* --------------------------------------------------------------------- */

// newReporterNoTTY builds a StatusReporter without progress-bar.
// All writers point to io.Discard to keep the test output clean.
func newReporterNoTTY() *StatusReporter {
	tpl, _ := dns.NewTemplate("example.com A=1.1.1.1")
	set := &config.Settings{
		ServerIPs:            []string{"8.8.8.8"},
		Template:             tpl,
		MaxThreads:           1,
		GlobRateLimit:        10,
		PerSrvRateLimit:      1,
		PerSrvMaxFailures:    0,
		PerCheckMaxAttempts:  1,
		PerQueryTimeout:      1,
	}
	ioFiles := &IOFiles{
		OutputFile: io.Discard,
	}
	return NewStatusReporter("unit-test-no-tty", ioFiles, set)
}

// newReporterWithPipe activates the progress-bar using a fake TTY.
// The reader side of the pipe lets us inspect what was printed.
func newReporterWithPipe() (st *StatusReporter, r *os.File, w *os.File) {
	r, w, _ = os.Pipe()
	tpl, _ := dns.NewTemplate("example.com A=1.1.1.1\nexample.org NXDOMAIN")
	set := &config.Settings{
		ServerIPs:            []string{"8.8.8.8", "9.9.9.9", "1.1.1.1"},
		Template:             tpl,
		MaxThreads:           2,
		GlobRateLimit:        50,
		PerSrvRateLimit:      1,
		PerSrvMaxFailures:    0,
		PerCheckMaxAttempts:  1,
		PerQueryTimeout:      1,
	}
	ioFiles := &IOFiles{
		TTYFile:    w,             // fake TTY
		OutputFile: io.Discard,
	}
	st = NewStatusReporter("unit-test-tty", ioFiles, set)
	return
}

/* --------------------------------------------------------------------- */
/* scaleValue                                                            */
/* --------------------------------------------------------------------- */

func TestScaleValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		value, tot int
		want       float64
	}{
		{"total-zero", 42, 0, 0},
		{"fifty-pct", 50, 100, 50},
		{"ratio", 3, 4, 75},
	}
	for _, c := range cases {
		got := scaleValue(c.value, c.tot, 100)
		if got != c.want {
			t.Fatalf("%s: scaleValue=%v, want %v", c.name, got, c.want)
		}
	}
}

/* --------------------------------------------------------------------- */
/* renderBrailleBar                                                      */
/* --------------------------------------------------------------------- */

func TestRenderBrailleBarWidthAndCompletion(t *testing.T) {
	t.Parallel()
	// 1) Width must remain exactly 60 runes
	s := &StatusReporter{TotalServers: 200, ValidServers: 60, InvalidServers: 40}
	bar := tty.StripAnsi(s.renderBrailleBar())
	if runes := utf8.RuneCountInString(bar); runes != 60 {
		t.Fatalf("unexpected bar width %d runes (want 60)", runes)
	}

	// 2) When every server processed, only full blocks (⣿) must be used
	s.ValidServers = 160
	s.InvalidServers = 40 // 200/200 processed
	full := tty.StripAnsi(s.renderBrailleBar())
	if !strings.ContainsRune(full, '⣿') {
		t.Fatalf("completed bar should contain full blocks (⣿)")
	}
	if strings.ContainsRune(full, '⡀') || strings.ContainsRune(full, '⢹') {
		t.Fatalf("completed bar must not contain partial blocks")
	}

	// 3) Overflow handling must still keep bar at 60 runes
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("renderBrailleBar should panic on impossible scaling but did not")
			}
		}()
		s.TotalServers = 10
		s.ValidServers = 9
		s.InvalidServers = 9 // 18/10 -> impossible
		_ = s.renderBrailleBar() // panics
	}()
}

/* --------------------------------------------------------------------- */
/* renderRemainingTime                                                   */
/* --------------------------------------------------------------------- */

func TestRenderRemainingTimeFormats(t *testing.T) {
	t.Parallel()
	// 1) Unknown progress (<0.1%)
	s := &StatusReporter{TotalChecks: 1000, TotalServers: 100, StartTime: time.Now()}
	if got := s.renderRemainingTime(); got != "ETA: --" {
		t.Fatalf("early remainingTime=%q, want \"ETA: --\"", got)
	}

	// 2) <1 minute
	s.DoneChecks, s.TotalChecks = 1, 1
	s.ValidServers, s.InvalidServers, s.TotalServers = 100, 0, 100
	s.StartTime = time.Now().Add(-30 * time.Second)
	if got := s.renderRemainingTime(); got != "DONE" {
		t.Fatalf("remainingTime 'DONE' unexpected: %q", got)
	}

	// 3) Hours format
	s = &StatusReporter{TotalChecks: 100, TotalServers: 10, StartTime: time.Now().Add(-time.Hour)}
	s.DoneChecks = 10
	s.ValidServers = 1
	if !strings.Contains(s.renderRemainingTime(), "h") {
		t.Fatalf("remainingTime should include hours: %q", s.renderRemainingTime())
	}

	// 4) Days format
	s = &StatusReporter{TotalChecks: 100, TotalServers: 10, StartTime: time.Now().Add(-24 * time.Hour)}
	s.DoneChecks = 1
	if !strings.Contains(s.renderRemainingTime(), "d") {
		t.Fatalf("remainingTime should include days: %q", s.renderRemainingTime())
	}
}

/* --------------------------------------------------------------------- */
/* renderLastSecReqCount & LogRequests                                   */
/* --------------------------------------------------------------------- */

func TestRenderLastSecReqCountPurgesOldBatches(t *testing.T) {
	t.Parallel()
	st, r, w := newReporterWithPipe()
	defer func() {
		st.Stop()
		r.Close()
		w.Close()
	}()

	// 3 recent requests (<1 s)
	st.LogRequests(time.Now().Add(-300*time.Millisecond), 2, 1)
	// 5 stale requests (>1 s)
	st.LogRequests(time.Now().Add(-1500*time.Millisecond), 3, 2)

	if got := st.renderLastSecReqCount(); got != 3 {
		t.Fatalf("expected 3 recent reqs, got %d", got)
	}
	if len(st.requestsLog) != 1 {
		t.Fatalf("stale batches not purged: len=%d", len(st.requestsLog))
	}
}

func TestLogRequestsWithoutPBar(t *testing.T) {
	t.Parallel()
	st := newReporterNoTTY()
	st.LogRequests(time.Now(), 4, 3)
	if st.DoneRequests != 7 {
		t.Fatalf("DoneRequests not incremented")
	}
	if len(st.requestsLog) != 0 {
		t.Fatalf("requestsLog should remain empty when progress-bar disabled")
	}
	st.Stop()
}

/* --------------------------------------------------------------------- */
/* fWrite behaviour                                                      */
/* --------------------------------------------------------------------- */

func TestFWriteBehaviour(t *testing.T) {
	t.Parallel()
	const ansiMsg = "\033[31mred\033[0m"

	// 1) bytes.Buffer → ANSI must be preserved.
	buf := &bytes.Buffer{}
	st := &StatusReporter{io: &IOFiles{}} // io non-nil, pas de TTY
	st.fWrite(buf, ansiMsg)
	if got := buf.String(); got != ansiMsg+"\n" {
		t.Fatalf("bytes.Buffer: unexpected output %q", got)
	}

	// 2) *os.File (non-TTY) → ANSI must be stripped.
	r, w, _ := os.Pipe()
	defer func() { r.Close(); w.Close() }()
	st.fWrite(w, ansiMsg)
	w.Close()
	out, _ := io.ReadAll(r)
	if string(out) != "red\n" {
		t.Fatalf("os.File: ANSI not stripped: %q", string(out))
	}
}

/* --------------------------------------------------------------------- */
/* Debug                                                                 */
/* --------------------------------------------------------------------- */

func TestDebugWritesToFile(t *testing.T) {
	t.Parallel()
	dbg := &bytes.Buffer{}
	tpl, _ := dns.NewTemplate("example.com A=1.1.1.1")
	set := &config.Settings{
		ServerIPs:           []string{"8.8.8.8"},
		Template:            tpl,
		MaxThreads:          1,
		GlobRateLimit:       10,
		PerSrvRateLimit:     1,
		PerSrvMaxFailures:   0,
		PerCheckMaxAttempts: 1,
		PerQueryTimeout:     1,
	}
	ioFiles := &IOFiles{DebugFile: dbg}
	st := NewStatusReporter("debug-test", ioFiles, set)
	st.Debug("hello %s", "world")
	if !strings.Contains(dbg.String(), "hello world") {
		t.Fatalf("Debug did not write expected content, got %q", dbg.String())
	}
	st.Stop()
}

/* --------------------------------------------------------------------- */
/* UpdatePoolSize & AddDoneChecks concurrency                            */
/* --------------------------------------------------------------------- */

func TestConcurrentAddDoneChecks(t *testing.T) {
	t.Parallel()
	st := newReporterNoTTY()
	const n = 128
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			st.AddDoneChecks(1, 0)
			wg.Done()
		}()
	}
	wg.Wait()
	if st.DoneChecks != n {
		t.Fatalf("lost updates: have %d, want %d", st.DoneChecks, n)
	}
	st.Stop()
}

func TestUpdatePoolSize(t *testing.T) {
	t.Parallel()
	st := newReporterNoTTY()
	st.UpdatePoolSize(3)
	if st.NumServersInPool != 3 {
		t.Fatalf("UpdatePoolSize failed, got NumServersInPool=%d", st.NumServersInPool)
	}
	st.Stop()
}

/* --------------------------------------------------------------------- */
/* Stop final render & Eraser                                            */
/* --------------------------------------------------------------------- */

func TestStopWritesFinalRender(t *testing.T) {
	t.Parallel()
	st, r, w := newReporterWithPipe()
	defer r.Close()
	defer w.Close()

	// Simulate some progress so the bar changes
	st.AddDoneChecks(2, 0)
	st.UpdatePoolSize(0)

	// Call Stop and read what was flushed
	st.Stop()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if n == 0 {
		t.Fatal("Stop did not write anything to fake TTY")
	}
	if !bytes.Contains(buf[:n], []byte("\n\n")) {
		t.Fatalf("final output missing expected newline padding: %q", string(buf[:n]))
	}
}

/* ------------------------------------------------------------------ */
/* ReportFinishedServer variants                                      */
/* ------------------------------------------------------------------ */

func TestReportFinishedServerVariants(t *testing.T) {
	t.Parallel()
	outBuf, verboseBuf := &bytes.Buffer{}, &bytes.Buffer{}
	rep := newReporterNoTTY()
	rep.io.OutputFile = outBuf
	rep.io.VerboseFile = verboseBuf     // KO → écrit ici

	// --- serveur OK --------------------------------------------------
	ok := &dns.ServerContext{IPAddress: "10.0.0.1"}
	rep.ReportFinishedServer(ok)
	if rep.ValidServers != 1 ||
		!strings.Contains(outBuf.String(), "10.0.0.1") {
		t.Fatalf("valid server path incorrect")
	}

	// --- serveur KO --------------------------------------------------
	ko := &dns.ServerContext{
		IPAddress: "10.0.0.2", Disabled: true, FailedCount: 3,
	}
	rep.ReportFinishedServer(ko)
	if rep.InvalidServers != 1 || rep.ServersWithFailures != 1 {
		t.Fatalf("invalid server counters incorrect")
	}
	if !strings.Contains(verboseBuf.String(), "10.0.0.2") {
		t.Fatalf("VerboseFile missing KO server dump")
	}
}

/* ------------------------------------------------------------------ */
/* AddDoneChecks — simple total increment                             */
/* ------------------------------------------------------------------ */

func TestAddDoneChecksTotalUpdate(t *testing.T) {
	t.Parallel()
	rep := newReporterNoTTY()
	base := rep.TotalChecks
	rep.AddDoneChecks(0, 3)
	if rep.TotalChecks != base+3 {
		t.Fatalf("TotalChecks not updated, got %d want %d", rep.TotalChecks, base+3)
	}
	rep.Stop()
}

/* --------------------------------------------------------------------- */
/* fWrite when PBar active                                               */
/* --------------------------------------------------------------------- */

func TestFWriteWithPBar(t *testing.T) {
	t.Parallel()
	rep, r, w := newReporterWithPipe()
	defer func() { rep.Stop(); r.Close(); w.Close() }()

	const msg = "\x1b[32mgreen\x1b[0m"
	rep.fWrite(w, msg)                      // message mis en cache
	time.Sleep(10 * time.Millisecond)       // flush par une itération du loop

	raw := make([]byte, 512)
	n, _ := r.Read(raw)

	// 1) Le texte « green » est bien là.
	if !strings.Contains(string(raw[:n]), "green") {
		t.Fatalf("message not found in TTY output")
	}
	// 2) …mais sans sa séquence ANSI (strippée par fWrite)
	if strings.Contains(string(raw[:n]), "\x1b[32mgreen") {
		t.Fatalf("ANSI sequence leaked from fWrite (should be stripped)")
	}
}

/* --------------------------------------------------------------------- */
/* NewStatusReporter init                                                */
/* --------------------------------------------------------------------- */

func TestNewStatusReporterInit(t *testing.T) {
	t.Parallel()
	rep := newReporterNoTTY()
	if rep.TotalServers == 0 || rep.TotalChecks == 0 || rep.redrawTicker == nil {
		t.Fatalf("NewStatusReporter did not initialise mandatory fields")
	}
	rep.Stop()
}

/* --------------------------------------------------------------------- */
/* renderBrailleBar edge cases                                           */
/* --------------------------------------------------------------------- */

func TestRenderBrailleBarEmptyAndAlmostFull(t *testing.T) {
	t.Parallel()
	rep := &StatusReporter{TotalServers: 5}

	// Empty bar
	empty := tty.StripAnsi(rep.renderBrailleBar())
	if strings.ContainsRune(empty, '⣿') {
		t.Fatalf("Empty bar contains full glyphs")
	}

	// 99% complete (4/5)
	rep.ValidServers = 4
	almost := tty.StripAnsi(rep.renderBrailleBar())
	if strings.Count(almost, "⣿") != 48 { // 4/5 of 60 runes = 48
		t.Fatalf("Unexpected ⣿ count in almost-full bar")
	}
}

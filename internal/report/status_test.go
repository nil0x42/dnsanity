package report

import (
    "bytes"
    "io"
    "os"
    "strings"
    "sync"
    "testing"
    "time"
    "math"
    "unicode/utf8"

    "github.com/nil0x42/dnsanity/internal/config"
    "github.com/nil0x42/dnsanity/internal/dns"
    "github.com/nil0x42/dnsanity/internal/tty"
)

/* --------------------------------------------------------------------- */
/* Helpers                                                               */
/* --------------------------------------------------------------------- */

// newReporterNoTTY builds a StatusReporter without the progress‑bar.
// Every writer points to io.Discard so that tests stay silent.
func newReporterNoTTY() *StatusReporter {
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
        MaxPoolSize:         10,
    }
    ioFiles := &IOFiles{OutputFile: io.Discard}
    return NewStatusReporter("unit‑test‑no‑tty", ioFiles, set)
}

// newReporterWithPipe enables the progress‑bar using a fake TTY.
// The read side of the pipe lets us inspect what was printed.
func newReporterWithPipe() (st *StatusReporter, r *os.File, w *os.File) {
    r, w, _ = os.Pipe()
    tpl, _ := dns.NewTemplate("example.com A=1.1.1.1\nexample.org NXDOMAIN")
    set := &config.Settings{
        ServerIPs:           []string{"8.8.8.8", "9.9.9.9", "1.1.1.1"},
        Template:            tpl,
        MaxThreads:          2,
        GlobRateLimit:       50,
        PerSrvRateLimit:     1,
        PerSrvMaxFailures:   0,
        PerCheckMaxAttempts: 1,
        PerQueryTimeout:     1,
        MaxPoolSize:         20,
    }
    ioFiles := &IOFiles{
        TTYFile:    w, // fake TTY
        OutputFile: io.Discard,
    }
    st = NewStatusReporter("unit‑test‑tty", ioFiles, set)
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
    }{{"total‑zero", 42, 0, 0}, {"fifty‑pct", 50, 100, 50}, {"ratio", 3, 4, 75}}
    for _, c := range cases {
        if got := scaleValue(c.value, c.tot, 100); got != c.want {
            t.Fatalf("%s: scaleValue=%v, want %v", c.name, got, c.want)
        }
    }
}

/* --------------------------------------------------------------------- */
/* MetricGauge                                                           */
/* --------------------------------------------------------------------- */

func TestMetricGaugeLogAndAvg(t *testing.T) {
    t.Parallel()
    var g MetricGauge
    // No sample yet → Avg() must return 0.
    if g.Avg() != 0 {
        t.Fatalf("empty MetricGauge Avg() != 0")
    }
    // First sample.
    g.Log(3)
    if g.Current != 3 || g.Peak != 3 || g.Avg() != 3 {
        t.Fatalf("after first Log, got cur=%d peak=%d avg=%d", g.Current, g.Peak, g.Avg())
    }
    // Higher value raises Peak and shifts average.
    g.Log(7)
    if g.Current != 7 || g.Peak != 7 || g.Avg() != 5 { // (3+7)/2 == 5
        t.Fatalf("after second Log unexpected values cur=%d peak=%d avg=%d", g.Current, g.Peak, g.Avg())
    }
    // Lower value updates Current but not Peak.
    g.Log(1)
    if g.Current != 1 || g.Peak != 7 || g.Avg() != int(math.Round(11.0/3.0)) {
        t.Fatalf("after third Log incorrect state cur=%d peak=%d avg=%d", g.Current, g.Peak, g.Avg())
    }
}

/* --------------------------------------------------------------------- */
/* RequestsLogger                                                        */
/* --------------------------------------------------------------------- */

func TestRequestsLoggerWindowAndStats(t *testing.T) {
    t.Parallel()
    base := time.Now()
    r := RequestsLogger{StartTime: base.Add(-2 * time.Second)}

    // Old batch (>1 s)
    r.Log(base.Add(-1500*time.Millisecond), 3, 2) // total 5
    // Recent batch (<1 s)
    r.Log(base.Add(-300*time.Millisecond), 2, 1)  // total 3
    // Empty batch should not be recorded.
    r.Log(base, 0, 0)

    if tot := r.Total(); tot != 8 {
        t.Fatalf("Total()=%d, want 8", tot)
    }

    if cnt := r.LastSecCount(); cnt != 3 {
        t.Fatalf("LastSecCount()=%d, want 3 (only recent batch)", cnt)
    }
    if r.OneSecPeak != 3 {
        t.Fatalf("OneSecPeak=%d, want 3", r.OneSecPeak)
    }
    if r.OneSecAvg() <= 0 {
        t.Fatalf("OneSecAvg()=%d, must be >0", r.OneSecAvg())
    }
}

/* --------------------------------------------------------------------- */
/* renderBrailleBar                                                      */
/* --------------------------------------------------------------------- */

func TestRenderBrailleBarWidthAndCompletion(t *testing.T) {
    t.Parallel()
    s := &StatusReporter{TotalServers: 200, ValidServers: 60, InvalidServers: 40}
    // 1) Width must remain exactly 60 runes.
    if runes := utf8.RuneCountInString(tty.StripAnsi(s.renderBrailleBar())); runes != 60 {
        t.Fatalf("unexpected bar width %d runes (want 60)", runes)
    }
    // 2) Completed run uses only full blocks.
    s.ValidServers = 160
    full := tty.StripAnsi(s.renderBrailleBar())
    if !strings.ContainsRune(full, '⣿') {
        t.Fatalf("completed bar should contain full blocks (⣿)")
    }
    if strings.ContainsRune(full, '⡀') || strings.ContainsRune(full, '⢹') {
        t.Fatalf("completed bar must not contain partial blocks")
    }
    // 3) Impossible scaling must panic (overflow correction safeguard).
    func() {
        defer func() { if r := recover(); r == nil { t.Fatalf("renderBrailleBar should panic on impossible scaling but did not") } }()
        s.TotalServers, s.ValidServers, s.InvalidServers = 10, 9, 9 // 18/10 impossible
        _ = s.renderBrailleBar()
    }()
}

/* --------------------------------------------------------------------- */
/* renderRemainingTime                                                   */
/* --------------------------------------------------------------------- */

func TestRenderRemainingTimeFormats(t *testing.T) {
    t.Parallel()
    // 1) Unknown progress (<0.1 %).
    s := &StatusReporter{TotalChecks: 1000, TotalServers: 100, StartTime: time.Now()}
    if got := s.renderRemainingTime(); got != "ETA: --" {
        t.Fatalf("early remainingTime=%q, want \"ETA: --\"", got)
    }
    // 2) DONE (<1 m remaining).
    s.DoneChecks, s.TotalChecks = 1, 1
    s.ValidServers, s.InvalidServers, s.TotalServers = 100, 0, 100
    s.StartTime = time.Now().Add(-30 * time.Second)
    if got := s.renderRemainingTime(); got != "DONE" {
        t.Fatalf("remainingTime 'DONE' unexpected: %q", got)
    }
    // 3) Hours format.
    s = &StatusReporter{TotalChecks: 100, TotalServers: 10, StartTime: time.Now().Add(-time.Hour)}
    s.DoneChecks = 10
    s.ValidServers = 1
    if !strings.Contains(s.renderRemainingTime(), "h") {
        t.Fatalf("remainingTime should include hours: %q", s.renderRemainingTime())
    }
    // 4) Days format.
    s = &StatusReporter{TotalChecks: 100, TotalServers: 10, StartTime: time.Now().Add(-24 * time.Hour)}
    s.DoneChecks = 1
    if !strings.Contains(s.renderRemainingTime(), "d") {
        t.Fatalf("remainingTime should include days: %q", s.renderRemainingTime())
    }
}

/* --------------------------------------------------------------------- */
/* fWrite behaviour                                                      */
/* --------------------------------------------------------------------- */

func TestFWriteBehaviour(t *testing.T) {
    t.Parallel()
    const ansiMsg = "\x1b[31mred\x1b[0m"
    buf := &bytes.Buffer{}
    st := &StatusReporter{io: &IOFiles{}} // io non‑nil, no TTY.
    st.fWrite(buf, ansiMsg)
    if got := buf.String(); got != ansiMsg+"\n" {
        t.Fatalf("bytes.Buffer: unexpected output %q", got)
    }
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
        MaxPoolSize:         5,
    }
    ioFiles := &IOFiles{DebugFile: dbg}
    st := NewStatusReporter("debug‑test", ioFiles, set)
    st.Debug("hello %s", "world")
    if !strings.Contains(dbg.String(), "hello world") {
        t.Fatalf("Debug did not write expected content, got %q", dbg.String())
    }
    st.Stop()
}

/* --------------------------------------------------------------------- */
/* UpdatePoolSize & UpdateBusyJobs                                       */
/* --------------------------------------------------------------------- */

func TestUpdatePoolAndBusyJobs(t *testing.T) {
    t.Parallel()
    rep := newReporterNoTTY()
    rep.UpdatePoolSize(3)
    rep.UpdateBusyJobs(2)
    if rep.PoolSize.Current != 3 || rep.PoolSize.Peak != 3 {
        t.Fatalf("PoolSize gauge incorrect cur=%d peak=%d", rep.PoolSize.Current, rep.PoolSize.Peak)
    }
    if rep.BusyJobs.Current != 2 || rep.BusyJobs.Peak != 2 {
        t.Fatalf("BusyJobs gauge incorrect cur=%d peak=%d", rep.BusyJobs.Current, rep.BusyJobs.Peak)
    }
    // Lower values update Current but not Peak.
    rep.UpdatePoolSize(1)
    rep.UpdateBusyJobs(1)
    if rep.PoolSize.Peak != 3 || rep.BusyJobs.Peak != 2 {
        t.Fatalf("Peak values should stay unchanged, got poolPeak=%d jobsPeak=%d", rep.PoolSize.Peak, rep.BusyJobs.Peak)
    }
    rep.Stop()
}

/* --------------------------------------------------------------------- */
/* LogRequests integration (StatusReporter → RequestsLogger)             */
/* --------------------------------------------------------------------- */

func TestStatusReporterLogRequests(t *testing.T) {
    t.Parallel()
    st := newReporterNoTTY() // progress‑bar disabled but RequestsLogger still active.
    now := time.Now()
    st.LogRequests(now, 4, 3) // 7 total
    if st.Requests.Idle != 4 || st.Requests.Busy != 3 {
        t.Fatalf("Idle/Busy counters not updated: idle=%d busy=%d", st.Requests.Idle, st.Requests.Busy)
    }
    if st.Requests.Total() != 7 {
        t.Fatalf("Total()=%d, want 7", st.Requests.Total())
    }
    if st.Requests.LastSecCount() != 7 {
        t.Fatalf("LastSecCount() should see 7 recent reqs")
    }
    st.Stop()
}

/* --------------------------------------------------------------------- */
/* AddDoneChecks concurrency                                             */
/* --------------------------------------------------------------------- */

func TestConcurrentAddDoneChecks(t *testing.T) {
    t.Parallel()
    st := newReporterNoTTY()
    const n = 128
    wg := sync.WaitGroup{}
    wg.Add(n)
    for i := 0; i < n; i++ {
        go func() { st.AddDoneChecks(1, 0); wg.Done() }()
    }
    wg.Wait()
    if st.DoneChecks != n {
        t.Fatalf("lost updates: have %d, want %d", st.DoneChecks, n)
    }
    st.Stop()
}

/* --------------------------------------------------------------------- */
/* ReportFinishedServer variants                                         */
/* --------------------------------------------------------------------- */

func TestReportFinishedServerVariants(t *testing.T) {
    t.Parallel()
    outBuf, verboseBuf := &bytes.Buffer{}, &bytes.Buffer{}
    rep := newReporterNoTTY()
    rep.io.OutputFile = outBuf
    rep.io.VerboseFile = verboseBuf // KO servers will be dumped here.

    // Valid server.
    ok := &dns.ServerContext{IPAddress: "10.0.0.1"}
    rep.ReportFinishedServer(ok)
    if rep.ValidServers != 1 || !strings.Contains(outBuf.String(), "10.0.0.1") {
        t.Fatalf("valid server path incorrect")
    }

    // Invalid server.
    ko := &dns.ServerContext{IPAddress: "10.0.0.2", Disabled: true, FailedCount: 3}
    rep.ReportFinishedServer(ko)
    if rep.InvalidServers != 1 || rep.ServersWithFailures != 1 {
        t.Fatalf("invalid server counters incorrect")
    }
    if !strings.Contains(verboseBuf.String(), "10.0.0.2") {
        t.Fatalf("VerboseFile missing KO server dump")
    }
}

/* --------------------------------------------------------------------- */
/* Stop final render & Eraser                                            */
/* --------------------------------------------------------------------- */

func TestStopWritesFinalRender(t *testing.T) {
    t.Parallel()
    st, r, w := newReporterWithPipe()
    defer r.Close()
    defer w.Close()

    st.AddDoneChecks(2, 0) // Simulate progress.
    st.UpdatePoolSize(0)

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

package report

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/nil0x42/dnsanity/internal/config"
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/tty"
)

var SPINNER = [][]rune{
	{'█', '▏', '█', '▏', '▋'},
	{'▎', '▌', '█', '▊', '▉'},
	{'█', '▋', '▌', '█', '▋'},
	{'█', '▍', '▍', '▋', '▊'},
	{'█', '▊', '▎', '▋', '▊'},
	{'▊', '▍', '▍', '▉', '▎'},
	{'▏', '▎', '▊', '▎', '▍'},
	{'▊', '█', '▋', '▋', '█'},
	{'█', '▉', '▌', '▍', '▍'},
	{'█', '▌', '▍', '▌', '▌'},
	{'▊', '▌', '▍', '▊', '▋'},
	{'▍', '▊', '█', '▉', '▌'},
	{'▉', '▌', '▊', '▉', '▉'},
	{'▍', '▍', '▏', '▊', '▎'},
	{'▎', '█', '█', '▌', '▏'},
	{'▌', '▎', '▉', '▎', '▊'},
	{'▌', '▋', '▌', '▍', '▏'},
	{'▎', '▋', '▋', '▎', '▊'},
	{'█', '▏', '▉', '▌', '▎'},
	{'▋', '▋', '▏', '▋', '▏'},
	{'█', '▌', '▋', '▍', '▏'},
	{'▊', '▏', '▍', '▊', '▊'},
	{'▎', '▏', '▋', '▏', '▎'},
	{'▌', '▊', '▉', '▏', '▊'},
}

type StatusReporter struct {
	// Plumbing:
	mu           sync.Mutex
	io           *IOFiles
	quit         chan struct{}
	redrawTicker *time.Ticker
	// Display:
	pBarTemplate   string // progress bar fmt string template
	pBarEraser     string // ANSI sequence to 'erase' current pbar
	cacheStr       string // cached data to display @ next redraw
	spinnerFrame   int    // current spinner frame
	verboseFileHdr string // printed once before 1st debugFile write
	// Servers Status:
	TotalServers        int
	ValidServers        int
	InvalidServers      int
	ServersWithFailures int
	// Checks Status:
	TotalChecks int
	DoneChecks  int
	// MISC:
	StartTime time.Time
	Requests  RequestsLogger // requests tracking
	PoolSize  MetricGauge    // pool tracking
	BusyJobs  MetricGauge    // jobs tracking
}

/* ------------------------------------------------------------------ */
/* CONSTRUCTOR ------------------------------------------------------ */
/* ------------------------------------------------------------------ */

// NewStatusReporter sets up StatusReporter, spinner and initial progress bar.
func NewStatusReporter(
	title string, ioFiles *IOFiles, set *config.Settings,
) *StatusReporter {
	dropMsg := func(srvMaxFail int) string {
		if srvMaxFail == 0 {
			return "dropped if any test fails"
		} else if srvMaxFail >= len(set.Template) {
			return "never dropped"
		}
		return fmt.Sprintf("dropped if >%d tests fail", srvMaxFail)
	}
	srvRatelimitStr := func() string {
		s := fmt.Sprintf("max %.10f", set.PerSrvRateLimit)
		return strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	pBarTemplate := fmt.Sprintf(
		"\n"+
			"\033[1;97m* %-30s\033[2;37m%%10s - %%s\n"+
			"%%c Run: %d servers * %d tests, max %d req/s, %d jobs (%%d busy)\n"+
			"%%c Per server: %s req/s, %s (%%d in pool)\n"+
			"%%c Per test: %ds timeout, up to %d attempts -> %%d%%%% done (%%d/%%d)\n"+
			"%%c │\033[32m%%-22s\033[2;37m%%6d req/s\033[31m%%26s\033[2;37m│\n"+
			"%%c │%%s\033[2;37m│\033[0m",
		// line 0: title
		title,
		// line 1: Run: ? servers * ? tests ...
		len(set.ServerIPs), len(set.Template),
		set.GlobRateLimit, set.MaxThreads,
		// line 2: Per server: ...
		srvRatelimitStr(), dropMsg(set.PerSrvMaxFailures),
		// line 3: Per test: ...
		set.PerQueryTimeout, set.PerCheckMaxAttempts,
	)
	s := &StatusReporter{
		io:           ioFiles,
		quit:         make(chan struct{}),
		redrawTicker: time.NewTicker(time.Millisecond * 250),

		pBarTemplate:   pBarTemplate,
		verboseFileHdr: set.Template.PrettyDump(),

		TotalServers: len(set.ServerIPs),
		TotalChecks:  len(set.ServerIPs) * len(set.Template),
		StartTime:    time.Now(),
		Requests:     RequestsLogger{StartTime: time.Now()},
		PoolSize:     MetricGauge{Max: set.MaxPoolSize},
		BusyJobs:     MetricGauge{Max: set.MaxThreads},
	}
	pBarNLines := strings.Count(s.renderDebugBar()+pBarTemplate, "\n")
	s.pBarEraser = "\r\033[2K" + strings.Repeat("\033[1A\033[2K", pBarNLines)
	if s.hasPBar() {
		s.io.TTYFile.WriteString(s.renderPBar())
		go s.loop()
	}
	return s
}

/* ------------------------------------------------------------------ */
/* PUBLIC API ------------------------------------------------------- */
/* ------------------------------------------------------------------ */

// UpdatePoolSize logs the current worker-pool size.
func (s *StatusReporter) UpdatePoolSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PoolSize.Log(n)
}

// UpdateBusyJobs logs the number of goroutines currently busy.
func (s *StatusReporter) UpdateBusyJobs(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BusyJobs.Log(n)
}

// AddDoneChecks adds to done/total check counters.
func (s *StatusReporter) AddDoneChecks(addDoneChecks, addTotalChecks int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.DoneChecks += addDoneChecks
	if addTotalChecks != 0 {
		s.TotalChecks += addTotalChecks
	}
}

// LogRequests records one idle/busy requests batch.
func (s *StatusReporter) LogRequests(t time.Time, nIdle, nBusy int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests.Log(t, nIdle, nBusy)
}

// ReportFinishedServer updates stats and writes results for one server.
func (s *StatusReporter) ReportFinishedServer(srv *dns.ServerContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if srv.FailedCount > 0 {
		s.ServersWithFailures++
	}
	if srv.Disabled {
		s.InvalidServers++
	} else {
		s.ValidServers++
		s.fWrite(s.io.OutputFile, srv.IPAddress)
	}
	if s.io.VerboseFile != nil {
		if s.verboseFileHdr == "" {
			s.fWrite(s.io.VerboseFile, srv.PrettyDump())
		} else { // hdr is only printed once, before 1st write to verboseFile
			s.fWrite(s.io.VerboseFile, s.verboseFileHdr+srv.PrettyDump())
			s.verboseFileHdr = ""
		}
	}
}

// Debug prints a formatted debug line when -debug is active.
func (s *StatusReporter) Debug(format string, args ...interface{}) {
	if s.hasDebug() {
		s.mu.Lock()
		defer s.mu.Unlock()

		str := fmt.Sprintf(
			"\x1b[1;33m[DEBUG]\x1b[0;33m %s\x1b[0m\n",
			fmt.Sprintf(format, args...),
		)
		s.fWrite(s.io.DebugFile, str)
	}
}

// Stop stops ticker, renders final bar and cleans up.
func (s *StatusReporter) Stop() {
	close(s.quit)
	s.redrawTicker.Stop()
	if s.hasPBar() {
		s.io.TTYFile.WriteString(
			s.pBarEraser + s.cacheStr + s.renderPBar() + "\n\n")
		s.cacheStr = ""
	}
}

/* ------------------------------------------------------------------ */
/* INTERNAL UTILS --------------------------------------------------- */
/* ------------------------------------------------------------------ */

// loop drives spinner redraws until quit.
func (s *StatusReporter) loop() {
	for {
		select {
		case <-s.redrawTicker.C:
			s.mu.Lock()
			s.spinnerFrame = (s.spinnerFrame + 1) % len(SPINNER)
			s.io.TTYFile.WriteString(
				s.pBarEraser + s.cacheStr + s.renderPBar())
			s.cacheStr = ""
			s.mu.Unlock()
		case <-s.quit:
			return
		}
	}
}

// hasPBar returns true when a TTY progress-bar is active.
func (s *StatusReporter) hasPBar() bool {
	return s.io.TTYFile != nil
}

// hasDebug returns true when debug logging is enabled.
func (s *StatusReporter) hasDebug() bool {
	return s.io.DebugFile != nil
}

// isFinished reports whether all servers have been processed.
func (s *StatusReporter) isFinished() bool {
	return s.doneServers() == s.TotalServers
}

// doneServers returns the number of servers already processed.
func (s *StatusReporter) doneServers() int {
	return s.ValidServers + s.InvalidServers
}

// fWrite outputs str to file (or caches) while handling TTY/ANSI.
func (s *StatusReporter) fWrite(file io.Writer, str string) {
	if file == nil {
		return
	}
	if !strings.HasSuffix(str, "\n") {
		str += "\n"
	}
	if s.hasPBar() && tty.IsTTY(file) {
		s.cacheStr += str
	} else {
		// Only strip ANSI if file is NOT a bytes.Buffer:
		if _, ok := file.(*bytes.Buffer); !ok {
			str = tty.StripAnsi(str)
		}
		io.WriteString(file, str)
	}
}

// scaleValue returns value/total scaled to `scale`.
func scaleValue(value, total, scale int) float64 {
	if total == 0 {
		return 0
	}
	return float64(scale) * (float64(value) / float64(total))
}

/* ------------------------------------------------------------------ */
/* INTERNAL RENDERING ----------------------------------------------- */
/* ------------------------------------------------------------------ */

// renderElapsedTime returns elapsed-time string in d h / h m / m s / s.
func (s *StatusReporter) renderElapsedTime() string {
	sec := int(time.Since(s.StartTime).Seconds())
	const D, H, M = 86400, 3600, 60
	switch {
	case sec >= D:
		return fmt.Sprintf("⏳%dd %dh", sec/D, (sec%D)/H)
	case sec >= H:
		return fmt.Sprintf("⏳%dh %dm", sec/H, (sec%H)/M)
	case sec >= M:
		return fmt.Sprintf("⏳%dm %ds", sec/M, sec%M)
	default:
		return fmt.Sprintf("⏳%ds", sec)
	}
}

// renderRemainingTime returns "DONE" or a brief human-readable ETA.
func (s *StatusReporter) renderRemainingTime() string {
	if s.isFinished() {
		return "DONE"
	}
	// Weighted progress : 80 % servers, 20 % checks.
	progress := scaleValue(s.DoneChecks, s.TotalChecks, 1)
	if srvPct := scaleValue(s.doneServers(), s.TotalServers, 1); srvPct > 0 {
		progress = (srvPct*4 + progress) / 5
	}
	if progress < 0.001 {
		return "ETA: --"
	}
	const D, H, M = 86400, 3600, 60
	elapsed := time.Since(s.StartTime)
	remain := time.Duration(float64(elapsed) * (1/progress - 1))
	switch sec := int(remain.Seconds()); {
	case sec < M:
		return "ETA: <1m"
	case sec < H:
		return fmt.Sprintf("ETA: %dm", sec/M)
	case sec < D:
		return fmt.Sprintf("ETA: %dh %dm", sec/H, (sec%H)/M)
	default:
		return fmt.Sprintf("ETA: %dd %dh", sec/D, (sec%D)/H)
	}
}

// renderBrailleBar outputs a 60-rune Braille bar: green valid blocks left,
// red invalid blocks right, rune-aligned and auto-trimmed to fit.
func (s *StatusReporter) renderBrailleBar() string {
	const (
		ptsPerChr = 8  // full Braille block (⣿) = 8 pts
		totalChrs = 60 // fixed bar width in runes
	)
	totalPts := totalChrs * ptsPerChr
	validPts := int(math.Round(scaleValue(
		s.ValidServers, s.TotalServers, totalPts)))
	invalidPts := int(math.Round(scaleValue(
		s.InvalidServers, s.TotalServers, totalPts)))

	// --- helpers to build runes for each side -----------------------------
	buildLeft := func(pts int) []rune {
		full, extra := pts/ptsPerChr, pts%ptsPerChr
		bar := make([]rune, 0, full+1)
		for i := 0; i < full; i++ {
			bar = append(bar, '⣿')
		}
		if extra > 0 {
			bar = append(bar, []rune("⡀⡄⡆⡇⣇⣧⣷")[extra-1])
		}
		return bar
	}
	buildRight := func(pts int) []rune {
		full, extra := pts/ptsPerChr, pts%ptsPerChr
		bar := make([]rune, 0, full+1)
		if extra > 0 {
			bar = append(bar, []rune("⠈⠘⠸⢸⢹⢻⢿")[extra-1])
		}
		for i := 0; i < full; i++ {
			bar = append(bar, '⣿')
		}
		return bar
	}
	validRunes := buildLeft(validPts)
	invalidRunes := buildRight(invalidPts)
	extraValid := validPts % ptsPerChr
	extraInvalid := invalidPts % ptsPerChr

	// --- overflow correction (rune‑level) ---------------------------------
	if len(validRunes)+len(invalidRunes) > totalChrs {
		switch {
		case len(validRunes) == 1: // if valid is single rune, trim invalid
			invalidRunes = invalidRunes[1:]
		case len(invalidRunes) == 1: // if invalid is single rune, trim valid
			validRunes = validRunes[:len(validRunes)-1]
		case len(validRunes) >= 2 && len(invalidRunes) >= 2:
			if extraValid >= extraInvalid { // priorize valid, trim invalid
				invalidRunes = invalidRunes[1:]
			} else { // priorize invalid, trim valid
				validRunes = validRunes[:len(validRunes)-1]
			}
		}
	}
	if s.isFinished() {
		for i := range validRunes {
			validRunes[i] = '⣿'
		}
		for i := range invalidRunes {
			invalidRunes[i] = '⣿'
		}
	}
	return "\033[32m" + string(validRunes) +
		strings.Repeat(" ", totalChrs-(len(validRunes)+len(invalidRunes))) +
		"\033[31m" + string(invalidRunes)
}

// renderPBar builds the multi-line spinner/progress bar
// (prepends debug bar if enabled).
func (s *StatusReporter) renderPBar() string {
	renderSrvStr := func(r string, n int) string { // OK/KO server str
		percent := int(scaleValue(n, s.TotalServers, 100))
		return fmt.Sprintf("%s: %d (%d%%)", r, n, percent)
	}
	pBar := fmt.Sprintf(
		s.pBarTemplate,
		// line 0 (title):
		s.renderElapsedTime(),
		s.renderRemainingTime(),
		// line 1: Run: N servers ...
		SPINNER[s.spinnerFrame][0],
		s.BusyJobs.Current,
		// line 2: Each server: ...
		SPINNER[s.spinnerFrame][1],
		s.PoolSize.Current,
		// line 3: Each test: ...
		SPINNER[s.spinnerFrame][2],
		int(scaleValue(s.DoneChecks, s.TotalChecks, 100)),
		s.DoneChecks, s.TotalChecks,
		// line 4: |OK: N%     KO: N%| ...
		SPINNER[s.spinnerFrame][3],
		renderSrvStr("OK", s.ValidServers),
		s.Requests.LastSecCount(),
		renderSrvStr("KO", s.InvalidServers),
		// line 5: |⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿| ...
		SPINNER[s.spinnerFrame][4],
		s.renderBrailleBar(),
	)
	if s.hasDebug() {
		return s.renderDebugBar() + pBar
	}
	return pBar
}

// renderDebugBar formats the yellow metrics block shown only in -debug mode.
func (s *StatusReporter) renderDebugBar() string {
	if !s.hasDebug() {
		return ""
	}
	return fmt.Sprintf(
		"\n\033[33m"+
			"* [jobs] cur:%-7d peak:%-7d avg:%-7d max:%-7d\n"+
			"* [pool] cur:%-7d peak:%-7d avg:%-7d max:%-7d\n"+
			"* [reqs] cur:%-7d peak:%-7d avg:%-7d all:%-7d idle:%-7d busy:%-7d",
		// line 1: [jobs]
		s.BusyJobs.Current, s.BusyJobs.Peak,
		s.BusyJobs.Avg(), s.BusyJobs.Max,
		// line 2: [pool]
		s.PoolSize.Current, s.PoolSize.Peak,
		s.PoolSize.Avg(), s.PoolSize.Max,
		// line 2: [reqs]
		s.Requests.LastSecCount(), s.Requests.OneSecPeak,
		s.Requests.OneSecAvg(), s.Requests.Total(),
		s.Requests.Idle, s.Requests.Busy,
	)
}

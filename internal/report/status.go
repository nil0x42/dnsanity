package report

import (
	"sync"
	"fmt"
	"strings"
	"math"
	"time"
	"io"
	"bytes"

	"github.com/nil0x42/dnsanity/internal/tty"
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/config"
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
	// plumbing:
	mu						sync.Mutex
	io						*IOFiles
	quit					chan struct{}
	redrawTicker			*time.Ticker
	// display:
	pBarTemplate			string // progress bar fmt string template
	pBarEraser				string // ANSI sequence to 'erase' current pbar
	cacheStr				string // cached data to display @ next redraw
	spinnerFrame			int    // current spinner frame
	verboseFileHdr			string // printed once before 1st debugFile write
	// Servers Status:
	TotalServers			int
	ValidServers			int
	InvalidServers			int
	ServersWithFailures		int
	// Requests Status:
	DoneRequests			int
	requestsLog				RequestsLog
    // Checks Status:
	TotalChecks				int
	DoneChecks				int
	// Pool Status:
	NumServersInPool		int
	// Time Tracking:
	StartTime				time.Time
}


/* ------------------------------------------------------------------ */
/* CONSTRUCTOR ------------------------------------------------------ */
/* ------------------------------------------------------------------ */

func NewStatusReporter(
	title string, ioFiles *IOFiles, set *config.Settings,
) *StatusReporter {
	dropMsg := func(srvMaxFail int) string {
		if srvMaxFail == 0 {
			return "dropped if any test fails"
		} else if srvMaxFail < 0 {
			return "never dropped"
		}
		return fmt.Sprintf("dropped if >%d tests fail", srvMaxFail)
	}
	pBarTemplate := fmt.Sprintf(
		"\n" +
		"\033[1;97m* %-45s\033[2;37m⏳%%s\n" +
		"%%c Run: %d servers * %d tests (max %d req/s, %d threads)\n" +
		"%%c Each server: %s req/s, %s (%%d loaded)\n" +
		"%%c Each test: %ds timeout, up to %d attempts -> %%d%%%% done (%%d/%%d)\n" +
		"%%c │\033[32m%%-22s\033[2;37m%%6d req/s\033[31m%%26s\033[2;37m│\n" +
		"%%c │%%s\033[2;37m│\033[0m",
		title,
		len(set.ServerIPs), len(set.Template), set.GlobRateLimit, set.MaxThreads,
		rateLimitRepr(set.PerSrvRateLimit), dropMsg(set.PerSrvMaxFailures),
		set.PerQueryTimeout, set.PerCheckMaxAttempts,
	)
	s := &StatusReporter{
		io:				ioFiles,
		quit:			make(chan struct{}),
		redrawTicker:	time.NewTicker(time.Millisecond * 293),

		pBarTemplate:	pBarTemplate,
		pBarEraser:		"\r\033[2K" + strings.Repeat("\033[1A\033[2K", 6),
		verboseFileHdr:	set.Template.PrettyDump(),

		TotalServers:	len(set.ServerIPs),
		TotalChecks:	len(set.ServerIPs) * len(set.Template),
		StartTime:		time.Now(),
	}
	if (s.hasPBar()) {
		s.io.TTYFile.WriteString(s.renderPBar())
		go s.loop()
	}
	return s
}


/* ------------------------------------------------------------------ */
/* PUBLIC API ------------------------------------------------------- */
/* ------------------------------------------------------------------ */

func (s *StatusReporter) UpdatePoolSize(newLen int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.NumServersInPool = newLen
}

func (s *StatusReporter) AddDoneChecks(addDoneChecks, addTotalChecks int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.DoneChecks += addDoneChecks
	if addTotalChecks != 0 {
		s.TotalChecks += addTotalChecks
	}
}

func (s *StatusReporter) LogRequests(t time.Time, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.DoneRequests += n
	if s.hasPBar() {
		// only log reqBatches if pbar is active to avoid accumulating...
		s.requestsLog.LogRequests(t, n)
	}
}

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
			s.fWrite(s.io.VerboseFile, s.verboseFileHdr + srv.PrettyDump())
			s.verboseFileHdr = ""
		}
	}
}

func (s *StatusReporter) Debug(format string, args ...interface{}) {
	if s.io.DebugFile != nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		str := fmt.Sprintf(
			"\x1b[1;33m[DEBUG]\x1b[0;33m %s\x1b[0m\n",
			fmt.Sprintf(format, args...),
		)
		s.fWrite(s.io.DebugFile, str)
	}
}

func (s *StatusReporter) Stop() {
	close(s.quit)
	s.redrawTicker.Stop()
	if (s.hasPBar()) {
		s.io.TTYFile.WriteString(
			s.pBarEraser + s.cacheStr + s.renderPBar() + "\n\n")
		s.cacheStr = ""
	}
}


/* ------------------------------------------------------------------ */
/* INTERNAL UTILS --------------------------------------------------- */
/* ------------------------------------------------------------------ */

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

func (s *StatusReporter) hasPBar() bool {
	return s.io.TTYFile != nil
}

//floatRepr: 45.00 -> "45", 45.50 -> "45.5"
func rateLimitRepr(num float64) string {
	if (num <= 0 || num > float64(time.Second)) {
		return "unlimited"
	}
	s := fmt.Sprintf("max %.10f", num)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

// percent := ScaleValue(cur, total, 100)     // to get percentage
// ratio   := ScaleValue(cur, total, 1.0)     // to get ratio [0,1]
// bps     := ScaleValue(cur, total, 10000)   // per 10000...
func scaleValue(value, total, scale int) float64 {
	if total == 0 {
		return 0
	}
	return float64(scale) * (float64(value) / float64(total))
}

func (s *StatusReporter) fWrite(file io.Writer, str string){
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


/* ------------------------------------------------------------------ */
/* INTERNAL RENDERING ----------------------------------------------- */
/* ------------------------------------------------------------------ */

func (s *StatusReporter) renderRemainingTime() string {
	reqsRate := scaleValue(s.DoneChecks, s.TotalChecks, 1)
	srvsRate := scaleValue(s.ValidServers + s.InvalidServers, s.TotalServers, 1)
	rate := (reqsRate + srvsRate + srvsRate) / 3.0 // more weight for srvsRate
	if rate < 0.001 {
		return "--" // unknown before 0.1% progress
	}
	elapsed := time.Since(s.StartTime)
	totalExpected := time.Duration(float64(elapsed) / rate)
	remaining := totalExpected - elapsed
	if remaining < 0 {
		remaining = 0
	}
	// --- human‑friendly formatting ----------------------------------------
    secs := int(remaining.Seconds() + 0.5) // round half‑up
    if secs < 60 {
        return "<1m"
    }
    mins   := secs / 60
    days   := mins / (24 * 60)
    hours  := (mins / 60) % 24
    minute := mins % 60
    switch {
    case days > 0:
        return fmt.Sprintf("%dd %dh", days, hours)
    case hours > 0:
        return fmt.Sprintf("%dh %dm", hours, minute)
    default:
        return fmt.Sprintf("%dm", minute)
    }
}

// renderLastSecReqCount purges old batches from requestsLog
// and return total sum of requests made during last second.
func (s *StatusReporter) renderLastSecReqCount() int {
	return s.requestsLog.CountLastSecRequests()
}

// makeBar renders a 60‑rune progress bar composed of Braille blocks.
// The green (left) segment represents valid servers, the red (right)
// segment represents invalid ones.  The bar is built from rune slices so
// any trimming removes entire UTF‑8 glyphs, never partial bytes.  When the
// bar would overflow 60 runes, the side that is only one rune wide (if any)
// is preserved and the opposite side is shortened by exactly one rune.
func (s *StatusReporter) renderBrailleBar() string {
	const (
		ptsPerChr = 8   // full Braille block (⣿) = 8 pts
		totalChrs = 60  // fixed bar width in runes
	)
	totalPts   := totalChrs * ptsPerChr
	validPts   := int(math.Round(scaleValue(
		s.ValidServers, s.TotalServers, totalPts)))
	invalidPts := int(math.Round(scaleValue(
		s.InvalidServers, s.TotalServers, totalPts)))

	// --- helpers to build runes for each side -----------------------------
	buildLeft := func(pts int) []rune {
		full, extra := pts / ptsPerChr, pts % ptsPerChr
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
		full, extra := pts / ptsPerChr, pts % ptsPerChr
		bar := make([]rune, 0, full+1)
		if extra > 0 {
			bar = append(bar, []rune("⠈⠘⠸⢸⢹⢻⢿")[extra-1])
		}
		for i := 0; i < full; i++ {
			bar = append(bar, '⣿')
		}
		return bar
	}
	validRunes   := buildLeft(validPts)
	invalidRunes := buildRight(invalidPts)
	extraValid   := validPts   % ptsPerChr
	extraInvalid := invalidPts % ptsPerChr

	// --- overflow correction (rune‑level) ---------------------------------
	if len(validRunes) + len(invalidRunes) > totalChrs {
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
	if s.ValidServers + s.InvalidServers == s.TotalServers {
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

func (s *StatusReporter) renderPBar() string {
	renderSrvStr := func(r string, n int) string { // OK/KO server str
		percent := int(scaleValue(n, s.TotalServers, 100))
		return fmt.Sprintf("%s: %d (%d%%)", r, n, percent)
	}
	return fmt.Sprintf(
		s.pBarTemplate,
		// line 0 (title):
		s.renderRemainingTime(),
		// line 1: Run: N servers ...
		SPINNER[s.spinnerFrame][0],
		// line 2: Each server: ...
		SPINNER[s.spinnerFrame][1],
		s.NumServersInPool,
		// line 3: Each test: ...
		SPINNER[s.spinnerFrame][2],
		int(scaleValue(s.DoneChecks, s.TotalChecks, 100)),
		s.DoneChecks, s.TotalChecks,
		// line 4: |OK: N%     KO: N%| ...
		SPINNER[s.spinnerFrame][3],
		renderSrvStr("OK", s.ValidServers),
		s.renderLastSecReqCount(),
		renderSrvStr("KO", s.InvalidServers),
		// line 5: |⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿| ...
		SPINNER[s.spinnerFrame][4],
		s.renderBrailleBar(),
	)
}

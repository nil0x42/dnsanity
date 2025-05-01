package display

//             VALIDATION                                  SANITIZATION

// --verbose             no-verbose              --verbose               no-verbose
// DebugFile->stringIO   DebugFile->stringIO     DebugFile->STDERR       DebugFile->DEVNULL
// OutFile->DEVNULL      OutFile->DEVNULL        OutFile->OUTFILE        OutFile->OUTFILE

// * Servers sanitization (step 2/2):
//   Run: 300 servers * 21 tests (max 500 req/s, 1000 jobs)
//   Each server: max 2 req/s, dropped if any test fails
//   Each test: 4s timeout, max 2 attempts, 100% done (845550/850000)
//   │OK: 315 (15%)        500 req/s           KO: 85000 (95%)|
//   │⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣧  ⢹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿│

import (
	"sync"
	"fmt"
	"strings"
	"os"
	"math"
	"time"
	"io"
	"bytes"

	"github.com/nil0x42/dnsanity/internal/tty"
	"github.com/nil0x42/dnsanity/internal/dns"
)

var defaultSpinner = [][]string{
	{"█", "▏", "█", "▏", "▋"},
	{"▎", "▌", "█", "▊", "▉"},
	{"█", "▋", "▌", "█", "▋"},
	{"█", "▍", "▍", "▋", "▊"},
	{"█", "▊", "▎", "▋", "▊"},
	{"▊", "▍", "▍", "▉", "▎"},
	{"▏", "▎", "▊", "▎", "▍"},
	{"▊", "█", "▋", "▋", "█"},
	{"█", "▉", "▌", "▍", "▍"},
	{"█", "▌", "▍", "▌", "▌"},
	{"▊", "▌", "▍", "▊", "▋"},
	{"▍", "▊", "█", "▉", "▌"},
	{"▉", "▌", "▊", "▉", "▉"},
	{"▍", "▍", "▏", "▊", "▎"},
	{"▎", "█", "█", "▌", "▏"},
	{"▌", "▎", "▉", "▎", "▊"},
	{"▌", "▋", "▌", "▍", "▏"},
	{"▎", "▋", "▋", "▎", "▊"},
	{"█", "▏", "▉", "▌", "▎"},
	{"▋", "▋", "▏", "▋", "▏"},
	{"█", "▌", "▋", "▍", "▏"},
	{"▊", "▏", "▍", "▊", "▊"},
	{"▎", "▏", "▋", "▏", "▎"},
	{"▌", "▊", "▉", "▏", "▊"},
}

type ReqBatch struct {
	timestamp	time.Time
	count		int
}

type Status struct {
	// Plumbing
	mu				sync.Mutex
	redrawTicker	*time.Ticker
	quit			chan struct{}

	// IO
	OutFile			io.Writer
	DebugFile		io.Writer
	TTYFile			*os.File
	DebugActivated	bool
	DebugFilePrefixStr string

	// ProgressBar attributes
	hasPBar			bool   // ProgresssBar activated ?
	PBarPrefixData	string // static prefix data for ProgressBar
	PBarEraseData	string // ProgressBar erasure string
	PBarData		string // ProgressBar data

	// Status Update
	TotalRequests	int
	LastRequests	[]ReqBatch

	TotalChecks		int
	DoneChecks		int

	TotalServers	int
	ValidServers	int
	InvalidServers	int

	NServersWithFail	int // needed on validation to know if it failed

	// pool
	PoolSize			int
	NumServersInPool	int

	// --- spinner state --------------------------------------------------
    Spinner				[][]string // frames
    curSpinnerID		int        // current spinner frame
}

func NewStatus(
	msg				string,
	numServers		int,
	numTests		int,
	globRateLimit	int,
	maxThreads		int,
	rateLimit		int,
	timeout			int,
	maxFailures		int,
	maxAttempts		int,
	outFile			io.Writer,
	debugFile		io.Writer,
	ttyFile			*os.File,
	debugActivated	bool,
	debugFilePrefixStr string,
) *Status {
	prefix := fmt.Sprintf("\n\033[1;97m* %s:\033[2;37m\n", msg)
	prefix += fmt.Sprintf(
		"Run: %d servers * %d tests (max %d req/s, %d threads)\n",
		numServers, numTests, globRateLimit, maxThreads,
	)
	if maxFailures == 0 {
		prefix += fmt.Sprintf(
			"Each server: max %d req/s, dropped if any test fails (%%d/%%d in queue)\n",
			rateLimit,
		)
	} else if maxFailures <= -1 {
		prefix += fmt.Sprintf(
			"Each server: max %d req/s, never dropped (%%d/%%d in queue)\n",
			rateLimit,
		)
	} else {
		prefix += fmt.Sprintf(
			"Each server: max %d req/s, dropped if >%d tests fail (%%d/%%d in queue)\n",
			rateLimit, maxFailures,
		)
	}
	prefix += fmt.Sprintf(
		"Each test: %ds timeout, up to %d attempts",
		timeout, maxAttempts,
	)
	
	s := &Status{
		TotalChecks:		numServers * numTests,
		TotalServers:		numServers,
		redrawTicker:		time.NewTicker(time.Millisecond * 293), // <3 primes
		quit:				make(chan struct{}),
		OutFile:			outFile,
		DebugFile:			debugFile,
		TTYFile:			ttyFile,
		PBarPrefixData:		prefix,
		Spinner:			defaultSpinner,
		DebugActivated:		debugActivated,
		DebugFilePrefixStr:	debugFilePrefixStr, // to show template before 1st svr.PrettyDump()
	}
	if s.TTYFile != nil {
		s.hasPBar = true
		s.updatePBar()
		s.TTYFile.WriteString(s.PBarData) // write PBar
		go s.loop()
	}
	return s
}

func (s *Status) loop() {
	for {
		select {
		case <-s.redrawTicker.C:
			s.UpdateAndRedrawPBar()
		case <-s.quit:
			return
		}
	}
}

// WithLock runs fn under the struct’s mutex,
// to update any number of fields atomically.
func (s *Status) WithLock(
	fn func(),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// log requests
func (s *Status) LogRequests(t time.Time, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalRequests += n
	// only log LastRequests if pbar is active
	// to avoid accumulating...
	if s.hasPBar {
		s.LastRequests = append(s.LastRequests, ReqBatch{timestamp: t, count: n})
	}
}

func (s *Status) fWrite(
	file io.Writer,
	str string,
){
	if file == nil {
		return
	}
	if !strings.HasSuffix(str, "\n") {
		str += "\n"
	}
	if s.hasPBar && tty.IsTTY(file) {
		io.WriteString(s.TTYFile, s.PBarEraseData + str + s.PBarData)
	} else {
		// Only strip ANSI colors if file is NOT a bytes.Buffer:
		if _, ok := file.(*bytes.Buffer); !ok {
			str = tty.StripAnsi(str)
		}
		io.WriteString(file, str)
	}
}

// used by dnsanitize to report when ServerContext.Finished()
func (s *Status) ReportFinishedServer(
	srv *dns.ServerContext,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if srv.FailedCount > 0 {
		s.NServersWithFail++
	}

	if srv.Disabled {
		s.InvalidServers++
	} else {
		s.ValidServers++
		s.fWrite(s.OutFile, srv.IPAddress)
	}
	// conditional optimization to not call PrettyDump() if not used
	if s.DebugFile != nil {
		var out string
		if s.DebugFilePrefixStr != "" {
			out += s.DebugFilePrefixStr
			s.DebugFilePrefixStr = "" // only triggered once
		}
		out += srv.PrettyDump()
		s.fWrite(s.DebugFile, out)
	}
}

// Debug behaves like printf, but uses Status.fWrite internally
func (s *Status) Debug(format string, args ...interface{}) {
	if !s.DebugActivated {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	colored := fmt.Sprintf("\x1b[1;33m[DEBUG]\x1b[0;33m %s\x1b[0m\n", msg)
	s.fWrite(os.Stderr, colored)
}


// makeBar renders a 56‑rune progress bar composed of Braille blocks.
// The green (left) segment represents valid servers, the red (right)
// segment represents invalid ones.  The bar is built from rune slices so
// any trimming removes entire UTF‑8 glyphs, never partial bytes.  When the
// bar would overflow 56 runes, the side that is only one rune wide (if any)
// is preserved and the opposite side is shortened by exactly one rune.
func (s *Status) makeBar() string {
	const (
		ptsPerChr = 8   // full Braille block (⣿) = 8 points
		totalChrs = 60  // fixed bar width in runes
	)
	totalPts := totalChrs * ptsPerChr

	// Scale each side to points (0‑448) using helper.
	validPts   := int(math.Round(scaleValue(s.ValidServers,   s.TotalServers, totalPts)))
	invalidPts := int(math.Round(scaleValue(s.InvalidServers, s.TotalServers, totalPts)))

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
	validRunes   := buildLeft(validPts)
	invalidRunes := buildRight(invalidPts)
	extraValid   := validPts   % ptsPerChr
	extraInvalid := invalidPts % ptsPerChr

	// --- overflow correction (rune‑level) ---------------------------------
	if len(validRunes)+len(invalidRunes) > totalChrs {
		switch {
		case len(validRunes) >= 2 && len(invalidRunes) >= 2:
			if extraValid >= extraInvalid {
				// Favour valid side, trim one rune from invalid.
				invalidRunes = invalidRunes[1:]
			} else {
				// Trim one rune from valid side.
				validRunes = validRunes[:len(validRunes)-1]
			}
		case len(validRunes) == 1:
			// Preserve single‑rune valid side.
			invalidRunes = invalidRunes[1:]
		case len(invalidRunes) == 1:
			// Preserve single‑rune invalid side.
			validRunes = validRunes[:len(validRunes)-1]
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
	if len(validRunes)+len(invalidRunes) > totalChrs {
		panic("xxx")
	}
	// Assemble with ANSI colours.
	spaces := strings.Repeat(" ", totalChrs - (len(validRunes) + len(invalidRunes)))
	return "\033[32m" + string(validRunes) + spaces +
	"\033[31m" + string(invalidRunes)
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

func (s *Status) getLastSecReqCount() int {
	now := time.Now()
	cutoff := now.Add(-1 * time.Second)
	// Purge old batches and sum requests in the last second
	res := 0
	writeIdx := 0
	for _, b := range s.LastRequests {
		if b.timestamp.After(cutoff) {
			// keep this batch
			s.LastRequests[writeIdx] = b
			writeIdx++
			res += b.count
		}
	}
	// drop all expired entries in one slice re-slice
	s.LastRequests = s.LastRequests[:writeIdx]
	return res
}

// addPBarSpinner inserts the spinner column on every line *except* the first
// two header lines. Frame advances on each call.
func (s *Status) addPBarSpinner(bar string) string {
	frame := s.Spinner[s.curSpinnerID]
	ignoreLines := 2

	lines := strings.Split(bar, "\n")

	// Inject glyph + space on lines ≥ 2 (0-based indexing).
	for i := ignoreLines; i < len(lines); i++ {
		var glyph string
		frameIdx := i - ignoreLines
		if frameIdx < len(frame) {
			glyph = frame[frameIdx]
		} else {
			glyph = " "
		}
		lines[i] = glyph + " " + lines[i]
	}
	// Next frame.
	s.curSpinnerID = (s.curSpinnerID + 1) % len(s.Spinner)
	return strings.Join(lines, "\n")
}

func (s *Status) updatePBar() {
	okStr := fmt.Sprintf(
		"OK: %d (%d%%)",
		s.ValidServers,
		int(scaleValue(s.ValidServers, s.TotalServers, 100)),
	)
	koStr := fmt.Sprintf(
		"KO: %d (%d%%)",
		s.InvalidServers,
		int(scaleValue(s.InvalidServers, s.TotalServers, 100)),
	)
	s.PBarData = s.addPBarSpinner(fmt.Sprintf(
		s.PBarPrefixData +
		" -> " +
		"%d%% done (%d/%d)\n" +
		"│\033[32m%-22s\033[2;37m%6d req/s\033[31m%26s\033[2;37m│\n" +
		"│%s%s│\033[0m",
		s.NumServersInPool, s.PoolSize,
		int(scaleValue(s.DoneChecks, s.TotalChecks, 100)),
		s.DoneChecks, s.TotalChecks,
		okStr, s.getLastSecReqCount(), koStr,
		s.makeBar(), "\033[2;37m",
	))
	s.PBarEraseData = "\033[0m\r\033[2K" + strings.Repeat(
		"\033[1A\033[2K", strings.Count(s.PBarData, "\n"),
	)
}

func (s *Status) UpdateAndRedrawPBar() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updatePBar()
	s.TTYFile.WriteString(s.PBarEraseData + s.PBarData)
}

func (s *Status) Stop() {
	close(s.quit)
	s.redrawTicker.Stop()
	s.updatePBar()
	s.TTYFile.WriteString(s.PBarEraseData + s.PBarData + "\n\n")
}

package runner

import (
	// standard
	"os"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/tty"
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/dnsanitize"
	"github.com/nil0x42/dnsanity/internal/display"
)


func RunAndReport(
	color string,
	msg string, 
	verbose bool,
	servers []string,
	tests []dns.DNSAnswer,
	globRateLimit int,
	maxThreads int,
	rateLimit int,
	timeout int,
	maxFailures int,
	maxAttempts int,
) []dnsanitize.ServerState {

	tty.SmartFprintf(
		os.Stderr,
		"%s[*] %s:\n",
		color, msg,
	)
	if (verbose || true) {
		prefix := "\033[0m    \033[2;37m# "
		// prefix := "    # "
		tty.SmartFprintf(
			os.Stderr,
			"%sRun: %d servers * %d tests (max %d req/s, %d threads).\n",
			prefix, len(servers), len(tests), globRateLimit, maxThreads,
		)
		if maxFailures == 0 {
			tty.SmartFprintf(
				os.Stderr,
				"%sEach server: max %d req/s, dropped if any test fails.\n",
				prefix, rateLimit,
			)
		} else if maxFailures <= -1 {
			tty.SmartFprintf(
				os.Stderr,
				"%sEach server: max %d req/s, never dropped.\n",
				prefix, rateLimit,
			)
		} else {
			tty.SmartFprintf(
				os.Stderr,
				"%sEach server: max %d req/s, dropped if >%d tests fail.\n",
				prefix, rateLimit, maxFailures,
			)
		}
		tty.SmartFprintf(
			os.Stderr,
			"%sEach test: %ds timeout, up to %d attempts.\n",
			prefix, timeout, maxAttempts,
		)
		tty.SmartFprintf(os.Stderr, color)
	}

	totalTests := len(servers) * len(tests)
	var reporter display.ProgressReporter
	if tty.IsTTY(os.Stderr) {
		reporter = display.NewTTYProgressReporter(color, totalTests)
	} else {
		reporter = display.NewNoTTYProgressReporter(totalTests)
	}

	serverStates := dnsanitize.DNSanitize(
		servers,
		tests,
		globRateLimit,
		maxThreads,
		rateLimit,
		timeout,
		maxFailures,
		maxAttempts,
		reporter.Update, // callback
	)
	reporter.Finish()
	tty.SmartFprintf(os.Stderr, "\033[0m\n")
	return serverStates
}

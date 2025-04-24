package main

import (
	// standard
	"os"
	"strings"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/config"
	"github.com/nil0x42/dnsanity/internal/runner"
	"github.com/nil0x42/dnsanity/internal/display"
	"github.com/nil0x42/dnsanity/internal/tty"
)

var header = "    " + strings.TrimSpace(`
    ▗▄▄▄ ▗▖  ▗▖ ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖
    ▐▌  █▐▛▚▖▐▌▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █    █   ▝▚▞▘
    ▐▌  █▐▌ ▝▜▌ ▝▀▚▖▐▛▀▜▌▐▌ ▝▜▌  █    █    ▐▌
    ▐▙▄▄▀▐▌  ▐▌▗▄▄▞▘▐▌ ▐▌▐▌  ▐▌▗▄█▄▖  █    ▐▌
`)


func main() {
	conf := config.Init()
	tty.SmartFprintf(os.Stderr, "\033[0;90m" + header + "\033[0m\n\n")

	// TEMPLATE VALIDATION
	trustedRes := runner.RunAndReport(
		"\033[2;97m", // color
		"Template validation (step 1/2)", // message
		conf.Opts.Verbose, // verbosity
		conf.TrustedDnsList, // servers
		conf.Template, // tests
		conf.Opts.GlobRateLimit, // global ratelimit
		conf.Opts.Threads, // max threads
		conf.Opts.TrustedRatelimit, // rate limit per server
		conf.Opts.TrustedTimeout, // server timeout
		-1, // max failures (-1 to do every test for trusted servers)
		conf.Opts.TrustedAttempts, // max attempts
	)
	// abort if at least 1 trusted server has a mismatch:
	failedTrustedServers := 0
	for _, srv := range trustedRes {
		if srv.FailedCount > 0 {
			failedTrustedServers++
		}
	}
	if failedTrustedServers > 0 {
		display.ReportDetails(conf.Template, trustedRes)
		tty.SmartFprintf(
			os.Stderr,
			"\n\033[1;31m[-] Template validation error: (%d/%d trusted servers failed)\n",
			failedTrustedServers, len(conf.TrustedDnsList),
		)
		tty.SmartFprintf(os.Stderr, "[-] Possible reasons:\n",)
		tty.SmartFprintf(os.Stderr, "    - Bad internet connection\n",)
		tty.SmartFprintf(os.Stderr, "    - Outdated template entries\n",)
		tty.SmartFprintf(os.Stderr, "    - Trusted server not so trustworthy\n",)
		tty.SmartFprintf(os.Stderr, "\033[0m",)
		os.Exit(2)
	}

	// SERVERS SANITIZATION
	res := runner.RunAndReport(
		"\033[1;97m", // color
		"Servers sanitization (step 2/2)", // message
		conf.Opts.Verbose, // verbosity
		conf.UntrustedDnsList, // servers
		conf.Template, // tests
		conf.Opts.GlobRateLimit, // global ratelimit
		conf.Opts.Threads, // max threads
		conf.Opts.Ratelimit, // rate limit per server
		conf.Opts.Timeout, // server timeout
		conf.Opts.MaxMismatches, // max failures
		conf.Opts.Attempts, // max attempts
	)
	totalServers := len(conf.UntrustedDnsList)
	validServers := 0
	for _, srv := range res {
		if !srv.Disabled {
			validServers++
		}
	}
	if conf.Opts.Verbose {
		display.ReportDetails(conf.Template, res)
	}

	// display final report line:
	percentValid := float32(0.0)
	if validServers > 0 && totalServers > 0 {
		percentValid = (float32(validServers) / float32(totalServers)) * 100.0
	}
	display.ReportValidResults(res, conf.Opts.OutputFilePath)
	tty.SmartFprintf(
		os.Stderr,
		"\n\033[1;34m[*] Valid servers: %d/%d (%.1f%%)\033[0m\n",
		validServers, totalServers, percentValid,
	)

	os.Exit(0)
}

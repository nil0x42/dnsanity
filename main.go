package main

import (
	// standard
	"os"
	"strings"
	"bytes"
	"fmt"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/config"
	"github.com/nil0x42/dnsanity/internal/runner"
	"github.com/nil0x42/dnsanity/internal/tty"
)

var header = "  " + strings.TrimSpace(`
  ▗▄▄▄ ▗▖  ▗▖ ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖
  ▐▌  █▐▛▚▖▐▌▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █    █   ▝▚▞▘
  ▐▌  █▐▌ ▝▜▌ ▝▀▚▖▐▛▀▜▌▐▌ ▝▜▌  █    █    ▐▌
  ▐▙▄▄▀▐▌  ▐▌▗▄▄▞▘▐▌ ▐▌▐▌  ▐▌▗▄█▄▖  █    ▐▌
`)


func main() {
	conf := config.Init()
	ttyFile := tty.OpenTTY()

	if ttyFile != nil {
		fmt.Fprintf(ttyFile, "\033[0;90m" + header + "\033[0m\n\n")
	}

	// TEMPLATE VALIDATION
	buf := &bytes.Buffer{}
	trustedRes := runner.RunAndReport(
		"[step 1/2] Template validation", // message
		conf.TrustedDnsList, // server IPs
		conf.Template, // tests
		conf.Opts.GlobRateLimit, // global ratelimit
		conf.Opts.Threads, // max threads
		conf.Opts.TrustedRatelimit, // rate limit per server
		conf.Opts.TrustedTimeout, // server timeout
		-1, // max failures (-1 to do every test for trusted servers)
		conf.Opts.TrustedAttempts, // max attempts
		conf.Opts.Debug, // debug mode ?
		nil, // outfile (to write valid servers (null here)
		buf, // debugfile (using buffer here)
		ttyFile, // /dev/tty
	)
	// abort if at least 1 trusted server has a mismatch:
	if trustedRes.NServersWithFail > 0 {
		tty.SmartFprintf(os.Stderr, "%s", buf.String())
		tty.SmartFprintf(
			os.Stderr,
			"\n\033[1;31m[-] Template validation error: " +
			"(%d/%d trusted servers failed)\n",
			trustedRes.NServersWithFail, len(conf.TrustedDnsList),
		)
		tty.SmartFprintf(os.Stderr, "[-] Possible reasons:\n",)
		tty.SmartFprintf(os.Stderr, "    - Unreliable internet connection\n",)
		tty.SmartFprintf(os.Stderr, "    - Outdated template entries\n",)
		tty.SmartFprintf(os.Stderr, "    - Trusted server not so trustworthy\n",)
		tty.SmartFprintf(os.Stderr, "\033[0m",)
		os.Exit(3)
	}
	trustedRes = nil // free mem
	buf = nil // free mem

	// SERVERS SANITIZATION
	debugFile := os.Stderr
	if !conf.Opts.Verbose {
		debugFile = nil
	}
	res := runner.RunAndReport(
		"[step 2/2] Servers sanitization", // message
		conf.UntrustedDnsList, // servers
		conf.Template, // tests
		conf.Opts.GlobRateLimit, // global ratelimit
		conf.Opts.Threads, // max threads
		conf.Opts.Ratelimit, // rate limit per server
		conf.Opts.Timeout, // server timeout
		conf.Opts.MaxMismatches, // max failures
		conf.Opts.Attempts, // max attempts
		conf.Opts.Debug, // debug mode ?
		conf.OutputFile, // outfile (-o option)
		debugFile, // verbose ? stderr : nil
		ttyFile, // /dev/tty
	)
	// display final report line:
	percentValid := float32(0.0)
	if res.ValidServers > 0 && res.TotalServers > 0 {
		percentValid = (float32(res.ValidServers) / float32(res.TotalServers)) * 100.0
	}

	if ttyFile != nil {
		fmt.Fprintf(
			ttyFile,
			"\033[1;34m[*] Valid servers: %d/%d (%.1f%%)\033[0m\n",
			res.ValidServers, res.TotalServers, percentValid,
		)
	}
	if !tty.IsTTY(os.Stderr) {
		tty.SmartFprintf(
			os.Stderr,
			"\n\033[1;34m[*] Valid servers: %d/%d (%.1f%%)\033[0m\n",
			res.ValidServers, res.TotalServers, percentValid,
		)
	}
	os.Exit(0)
}

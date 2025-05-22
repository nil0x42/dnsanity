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
	"github.com/nil0x42/dnsanity/internal/tty"
	"github.com/nil0x42/dnsanity/internal/report"
	"github.com/nil0x42/dnsanity/internal/dnsanitize"
)


func validateTemplate(
	conf		*config.Config,
	ttyFile		*os.File,
) bool {
	settings := &config.Settings{
		// global
		ServerIPs:				conf.TrustedDNSList,
		Template:				conf.Template,
		MaxThreads:				conf.Opts.Threads,
		MaxPoolSize:			conf.Opts.MaxPoolSize,
		GlobRateLimit:			conf.Opts.GlobRateLimit,
		// per server
		PerSrvRateLimit:		conf.Opts.TrustedRateLimit,
		PerSrvMaxFailures:		-1, // never drop Trusted Servers
		// per check
		PerCheckMaxAttempts:	conf.Opts.TrustedAttempts,
		// per dns query
		PerQueryTimeout:		conf.Opts.TrustedTimeout,
	}
	buffer := &bytes.Buffer{}
	ioFiles := &report.IOFiles{
		TTYFile:				ttyFile,
		VerboseFile:			buffer, // write to buffer for later
	}
	status := report.NewStatusReporter(
		"[step 1/2] Template validation",
		ioFiles, settings,
	)
	dnsanitize.DNSanitize(settings, status)
	status.Stop()

	// Fails if at least 1 trusted server has a mismatch:
	if status.ServersWithFailures > 0 {
		errMsg := "Template validation error"
		tty.SmartFprintf(
			os.Stderr,
			"%s\n" +
			"\033[1;31m[-] %s: (%d/%d trusted servers failed)\n" +
			"[-] Possible reasons:\n" +
			"    - Unreliable internet connection\n" +
			"    - Outdated template entries\n" +
			"    - Trusted servers not so trustworthy\n" +
			"\033[0m",
			buffer.String(), errMsg,
			status.ServersWithFailures, len(settings.ServerIPs),
		)
		return false
	}
	return true
}

func sanitizeServers(
	conf		*config.Config,
	ttyFile		*os.File,
) {
	settings := &config.Settings{
		// global
		ServerIPs:				conf.UntrustedDNSList,
		Template:				conf.Template,
		MaxThreads:				conf.Opts.Threads,
		MaxPoolSize:			conf.Opts.MaxPoolSize,
		GlobRateLimit:			conf.Opts.GlobRateLimit,
		// per server
		PerSrvRateLimit:		conf.Opts.RateLimit,
		PerSrvMaxFailures:		conf.Opts.MaxMismatches,
		// per check
		PerCheckMaxAttempts:	conf.Opts.Attempts,
		// per dns query
		PerQueryTimeout:		conf.Opts.Timeout,
	}

	ioFiles := &report.IOFiles{
		TTYFile:				ttyFile,
		OutputFile:				conf.OutputFile,
	}
	if conf.Opts.Verbose {
		ioFiles.VerboseFile = os.Stderr
	}
	if conf.Opts.Debug {
		ioFiles.DebugFile = os.Stderr
	}

	status := report.NewStatusReporter(
		"[step 2/2] Servers sanitization",
		ioFiles, settings,
	)
	dnsanitize.DNSanitize(settings, status)
	status.Stop()

	// display final report line:
	successRate := float64(0.0)
	if status.TotalServers > 0 {
		successRate =
			float64(status.ValidServers) / float64(status.TotalServers)
	}
	reportStr := fmt.Sprintf(
		"[*] Valid servers: %d/%d (%.1f%%)",
		status.ValidServers, status.TotalServers, successRate * 100,
	)
	if ttyFile != nil {
		fmt.Fprintf(ttyFile, "\033[1;34m%s\033[0m\n", reportStr)
	}
	if !tty.IsTTY(os.Stderr) {
		fmt.Fprintf(os.Stderr, "\n%s\n", reportStr)
	}
}

func main() {
	conf := config.Init()
	ttyFile := tty.OpenTTY()

	// display header
	if ttyFile != nil {
		fmt.Fprintf(
			ttyFile,
			"\033[0;90m%s\033[0m\n\n",
			strings.Trim(config.HEADER, "\n"),
		)
	}
	// validate Template
	if !validateTemplate(conf, ttyFile) {
		os.Exit(3)
	}

	// sanitize servers
	sanitizeServers(conf, ttyFile)
	os.Exit(0)
}

package runner

import (
	// standard
	"io"
	"os"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/dnsanitize"
	"github.com/nil0x42/dnsanity/internal/display"
)


func RunAndReport(
	msg string,
	serverIPs []string,
	template dns.Template,
	globRateLimit int,
	maxThreads int,
	rateLimit int,
	timeout int,
	maxFailures int,
	maxAttempts int,
	debug bool,
	outFile io.Writer,
	debugFile io.Writer,
	ttyFile *os.File,
) *display.Status {

	status := display.NewStatus(
		msg,
		len(serverIPs),
		len(template),
		globRateLimit,
		maxThreads,
		rateLimit,
		timeout,
		maxFailures,
		maxAttempts,
		outFile,
		debugFile,
		ttyFile,
		debug,
		template.PrettyDump(),
	)

	dnsanitize.DNSanitize(
		serverIPs,
		template,
		globRateLimit,
		maxThreads,
		rateLimit,
		timeout,
		maxFailures,
		maxAttempts,
		status,
	)
	status.Stop()
	return status
}

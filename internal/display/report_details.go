package display

import (
	// standard
	"os"
	"fmt"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/dnsanitize"
	"github.com/nil0x42/dnsanity/internal/tty"
)

func ReportTemplate(template []dns.DNSAnswer) {
	tty.SmartFprintf(os.Stderr, "\033[1;34m[*] DNSANITY TEMPLATE:\033[m\n")
	for _, entry := range template {
		tty.SmartFprintf(
			os.Stderr, "    \033[34m* %s\033[m\n", entry.ToString())
	}
}

// detailed report
func ReportDetails(
	template []dns.DNSAnswer,
	servers []dnsanitize.ServerContext,
) {
	ReportTemplate(template)

	for _, srv := range servers {
		if srv.FailedCount == 0 {
			tty.SmartFprintf(
				os.Stderr, "\033[1;32m[+] SERVER %v (valid)\033[m\n", srv.IPAddress)
		} else {
			tty.SmartFprintf(
				os.Stderr, "\033[1;31m[-] SERVER %v (invalid)\033[m\n", srv.IPAddress)
		}
		for _, test := range srv.Checks {
			var prefix string
			if test.Passed {
				prefix = "\033[1;32m+\033[0;32m"
			} else if test.Answer.Status == "SKIPPED" {
				prefix = "\033[1;90m!\033[0;90m"
			} else {
				prefix = "\033[1;31m-\033[0;31m"
			}
			numTries := test.MaxAttempts - test.AttemptsLeft
			attemptsRepr := ""
			if numTries > 1 {
				suffix := "th"
				if numTries == 2 {
					suffix = "nd"
				} else if numTries == 3 {
					suffix = "rd"
				}
				attemptsRepr = fmt.Sprintf(
					" \033[33m(on %v%v attempt)\033[m", numTries, suffix)
			}
			tty.SmartFprintf(
				os.Stderr, "    %s %s\033[m%v\n",
				prefix, test.Answer.ToString(), attemptsRepr)
		}
	}
}

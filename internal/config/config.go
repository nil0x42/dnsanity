package config

import (
	// standard
	"os"
	"fmt"
	"flag"
	// external
	// local
	"github.com/nil0x42/dnsanity/internal/dns"
	"github.com/nil0x42/dnsanity/internal/tty"
	"github.com/nil0x42/dnsanity/internal/display"
)


type Config struct {
	Opts             *Options
	TrustedDnsList   []string
	UntrustedDnsList []string
	Template         []dns.DNSAnswer
}


func exitUsage(format string, a ...interface{}) {
	err := fmt.Errorf(format, a...)
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	flag.Usage()
	os.Exit(1)
}


func Init() *Config {
	conf := &Config{}

	opts, err := ParseOptions()
	if err != nil {
		exitUsage("%w", err)
	}

	conf.TrustedDnsList, err = ParseServerList(opts.TrustedDNS)
	if err != nil {
		exitUsage("-trusted-list: %w", err)
	}

	if opts.Template == "" {
		// use default template
		conf.Template, err = dns.DNSAnswerSliceFromString(DEFAULT_TEMPLATE)
	} else {
		conf.Template, err = dns.DNSAnswerSliceFromFile(opts.Template)
	}
	if err != nil {
		exitUsage("-template: %w", err)
	}

	if opts.UntrustedDNS == "" {
		if tty.IsTTY(os.Stdin) {
			if opts.Verbose || opts.Template != "" {
				display.ReportTemplate(conf.Template)
				fmt.Fprintf(os.Stderr, "\n")
			}
			exitUsage("-list: Required unless passed through STDIN")
		}
		opts.UntrustedDNS = "/dev/stdin"
	}

	conf.UntrustedDnsList, err = ParseServerList(opts.UntrustedDNS)
	if err != nil {
		exitUsage("-list: %w", err)
	}


	conf.Opts = opts
	return conf
}

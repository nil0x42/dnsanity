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
)


type Config struct {
	Opts             *Options
	TrustedDNSList   []string
	UntrustedDNSList []string
	Template         dns.Template
	OutputFile       *os.File
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

	// TEMPLATE VALIDATION --------------------------------------------
	// -template
	if opts.Template == "" {
		conf.Template, err = dns.NewTemplate(DEFAULT_TEMPLATE)
	} else {
		conf.Template, err = dns.NewTemplateFromFile(opts.Template)
	}
	if err != nil {
		exitUsage("-template: %w", err)
	}
	// -trusted-list
	conf.TrustedDNSList, err = ParseServerList(opts.TrustedDNS)
	if err != nil {
		exitUsage("-trusted-list: %w", err)
	}
	// -trusted-timeout
	if opts.TrustedTimeout < 1 {
		exitUsage("-trusted-timeout: must be >= 1")
	}
	// -ratelimit
	if opts.TrustedRateLimit < 0 {
		exitUsage("-trusted-ratelimit: must be >= 0")
	}
	// -max-attempts
	if opts.TrustedAttempts < 1 {
		exitUsage("-trusted-max-attempts: must be >= 1")
	}

	// SERVERS SANITIZATION -------------------------------------------
	// -list
	if opts.UntrustedDNS == "/dev/stdin" {
		if tty.IsTTY(os.Stdin) {
			if opts.Verbose { // show template if -verbose
				tty.SmartFprintf(os.Stderr, "%s\n", conf.Template.PrettyDump())
				fmt.Fprintf(os.Stderr, "Use `--help` to learn how to use DNSanity\n")
				os.Exit(1)
			} else {
				exitUsage("-list: Required unless passed through STDIN")
			}
		}
	}
	conf.UntrustedDNSList, err = ParseServerList(opts.UntrustedDNS)
	if err != nil {
		exitUsage("-list: %w", err)
	}
	// -timeout
	if opts.Timeout < 1 {
		exitUsage("-timeout: must be >= 1")
	}
	// -ratelimit
	if opts.RateLimit < 0 {
		exitUsage("-ratelimit: must be >= 0")
	}
	// -max-attempts
	if opts.Attempts < 1 {
		exitUsage("-max-attempts: must be >= 1")
	}
	// -max-mismatches
	if opts.MaxMismatches < 0 {
		exitUsage("-max-mismatches: must be >= 0")
	}

	// GENERIC OPTIONS ------------------------------------------------
	// -o
	conf.OutputFile, err = OpenFile(opts.OutputFilePath)
	if err != nil {
		exitUsage("-o: %w", err)
	}
	// -global-ratelimit
	if opts.GlobRateLimit < 1 {
		exitUsage("-global-ratelimit: must be >= 1")
	}
	// -threads
	if opts.Threads == -0xdead {
		opts.Threads = opts.GlobRateLimit * 20 // default
	} else if opts.Threads < 1 {
		exitUsage("-threads: must be >= 1")
	}
	// -max-poolsize
	if opts.MaxPoolSize == -0xdead {
		opts.MaxPoolSize = opts.GlobRateLimit * 20 // default
	} else if opts.MaxPoolSize < 1 {
		exitUsage("-max-poolsize: must be >= 1")
	}


	conf.Opts = opts
	return conf
}


func OpenFile(path string) (*os.File, error) {
    if path == "" || path == "-" || path == "/dev/stdout" {
        return os.Stdout, nil
    }
    return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

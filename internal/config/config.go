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

	conf.TrustedDNSList, err = ParseServerList(opts.TrustedDNS)
	if err != nil {
		exitUsage("-trusted-list: %w", err)
	}

	if opts.Template == "" {
		conf.Template, err = dns.NewTemplate(DEFAULT_TEMPLATE)
	} else {
		conf.Template, err = dns.NewTemplateFromFile(opts.Template)
	}
	if err != nil {
		exitUsage("-template: %w", err)
	}

	if opts.UntrustedDNS == "" {
		if tty.IsTTY(os.Stdin) {
			if opts.Verbose || opts.Template != "" {
				fmt.Fprintf(os.Stderr, "%s\n", conf.Template.PrettyDump())
			}
			exitUsage("-list: Required unless passed through STDIN")
		}
		opts.UntrustedDNS = "/dev/stdin"
	}

	conf.UntrustedDNSList, err = ParseServerList(opts.UntrustedDNS)
	if err != nil {
		exitUsage("-list: %w", err)
	}

	conf.OutputFile, err = OpenFile(opts.OutputFilePath)
	if err != nil {
		exitUsage("-o: %w", err)
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

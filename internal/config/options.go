package config

import (
	// standard
	"flag"
	"fmt"
	"os"

	// external
	// local
	"github.com/nil0x42/dnsanity/internal/tty"
)

type Options struct {
	UntrustedDNS     string
	TrustedDNS       string
	Template         string
	Threads          int
	MaxPoolSize      int
	Timeout          int
	TrustedTimeout   int
	GlobRateLimit    int
	RateLimit        float64
	TrustedRateLimit float64
	Attempts         int
	MaxMismatches    int
	TrustedAttempts  int
	OutputFilePath   string
	ShowHelp         bool
	ShowVersion      bool
	Verbose          bool
	Debug            bool
}

func ShowHelp() {
	var rst = "\033[0m"
	var bol = "\033[1m"
	var red = "\033[31m"
	// var grn = "\033[32m"
	var yel = "\033[33m"
	// var blu = "\033[34m"
	// var mag = "\033[35m"
	// var cya = "\033[36m"
	var gra = "\033[37m"
	// var dimgra = "\033[2;37m"
	var whi = "\033[97m"
	var s string

	s += fmt.Sprintf("\n")
	s += fmt.Sprintf(
		"%s%sDNSanity is a high-performance DNS validator using template-based verification%s\n",
		rst, bol, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"Usage:      %sdnsanity%s %s[OPTION]...%s\n",
		whi, rst, yel, rst)
	s += fmt.Sprintf(
		"Example:    %sdnsanity%s %s-list%s /tmp/untrusted-dns.txt %s-o%s /tmp/trusted-dns.txt\n",
		whi, rst, yel, rst, yel, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sGENERIC OPTIONS:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-o%s %s[FILE]%s                  file to write output (defaults to %sSTDOUT%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-global-ratelimit%s %sint%s      global max requests per second (default %s500%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-threads%s %sint%s               max concurrency (default: %sauto%s) %s[experts only]%s\n",
		yel, rst, gra, rst, yel, rst, red, rst)
	s += fmt.Sprintf(
		"   %s-max-poolsize%s %sint%s          limit servers loaded in memory (default: %sauto%s) %s[experts only]%s\n",
		yel, rst, gra, rst, yel, rst, red, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sSERVERS SANITIZATION:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-list%s %s[FILE||str]%s          list of DNS servers to sanitize (%sfile%s or %scomma separated%s or %sSTDIN%s)\n",
		yel, rst, gra, rst, yel, rst, yel, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-timeout%s %sint%s               timeout in seconds for DNS queries (default %s4%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-ratelimit%s %sfloat%s           max requests per second per DNS server (default %s2%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-max-attempts%s %sint%s          max attempts before marking a mismatching DNS test as failed (default %s2%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-max-mismatches%s %sint%s        max allowed mismatching DNS tests per server (default %s0%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sTEMPLATE VALIDATION:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-template%s %s[FILE]%s           use a custom validation template instead of default one\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-trusted-list%s %s[FILE||str]%s  list of TRUSTED servers (defaults to %s\"8.8.8.8, 1.1.1.1, 9.9.9.9\"%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-trusted-timeout%s %sint%s       timeout in seconds for TRUSTED servers (default %s2%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-trusted-ratelimit%s %sfloat%s   max requests per second per TRUSTED server (default %s10%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf(
		"   %s-trusted-max-attempts%s %sint%s  max attempts before marking a mismatching TRUSTED test as failed (default %s2%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sDEBUG:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-h, --help%s                 show help\n",
		yel, rst)
	s += fmt.Sprintf(
		"   %s-version%s                   display version of dnsanity\n",
		yel, rst)
	s += fmt.Sprintf(
		"   %s-verbose%s                   show template and servers status details (on STDERR)\n",
		yel, rst)
	s += fmt.Sprintf(
		"   %s-debug%s                     show debugging information (on STDERR)\n",
		yel, rst)
	s += fmt.Sprintf("\n")
	tty.SmartFprintf(os.Stdout, "%s", s)
}

func ShowVersion() {
	tty.SmartFprintf(
		os.Stdout,
		"DNSanity %s <http://github.com/nil0x42/dnsanity>\n",
		VERSION,
	)
	os.Exit(0)
}

func ParseOptions() (*Options, error) {
	opts := &Options{}
	// GENERIC OPTIONS
	flag.StringVar(&opts.OutputFilePath, "o", "/dev/stdout", "file to write output")
	flag.IntVar(&opts.GlobRateLimit, "global-ratelimit", 500, "global rate limit")
	flag.IntVar(&opts.Threads, "threads", -0xdead, "number of threads")
	flag.IntVar(&opts.MaxPoolSize, "max-poolsize", -0xdead, "limit servers loaded in memory")
	// SERVER SANITIZATION
	flag.StringVar(&opts.UntrustedDNS, "list", "/dev/stdin", "list of DNS servers to sanitize (file or comma separated or stdin)")
	flag.IntVar(&opts.Timeout, "timeout", 4, "timeout in seconds for DNS queries")
	flag.Float64Var(&opts.RateLimit, "ratelimit", 2.0, "max requests per second per DNS server")
	flag.IntVar(&opts.Attempts, "max-attempts", 2, "max attempts before marking a mismatching DNS test as failed")
	flag.IntVar(&opts.MaxMismatches, "max-mismatches", 0, "max allowed mismatching tests per DNS server")
	// TEMPLATE VALIDATION
	flag.StringVar(&opts.Template, "template", "", "path to the DNSanity validation template")
	flag.StringVar(&opts.TrustedDNS, "trusted-list", "8.8.8.8, 1.1.1.1, 9.9.9.9", "list of TRUSTED servers")
	flag.IntVar(&opts.TrustedTimeout, "trusted-timeout", 2, "timeout in seconds for TRUSTED servers")
	flag.Float64Var(&opts.TrustedRateLimit, "trusted-ratelimit", 10.0, "max requests per second per TRUSTED server")
	flag.IntVar(&opts.TrustedAttempts, "trusted-max-attempts", 2, "max attempts before marking a mismatching TRUSTED test as failed")
	// DEBUG
	flag.BoolVar(&opts.ShowHelp, "h", false, "show help")
	// flag.BoolVar(&opts.ShowFullHelp, "full-help", false, "show advanced help")
	flag.BoolVar(&opts.ShowVersion, "version", false, "display version of dnsanity")
	flag.BoolVar(&opts.Verbose, "verbose", false, "show configuration and template")
	flag.BoolVar(&opts.Debug, "debug", false, "enable debugging information")

	flag.Usage = ShowHelp
	flag.Parse()

	if opts.ShowHelp {
		flag.Usage()
		os.Exit(0)
	}
	if opts.ShowVersion {
		ShowVersion()
	}
	return opts, nil
}

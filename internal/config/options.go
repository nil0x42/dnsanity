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
	Timeout          int
	TrustedTimeout   int
	GlobRateLimit	 int
	Ratelimit        int
	TrustedRatelimit int
	Attempts         int
	MaxMismatches    int
	TrustedAttempts  int
	OutputFilePath   string
	ShowHelp         bool
	// ShowFullHelp     bool
	ShowVersion      bool
	Verbose          bool
}

func ShowHelp() {
	var rst = "\033[0m"
	var bol = "\033[1m"
	// var red = "\033[31m"
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
		"%sDNSanity is a high-performance DNS validator using template-based verification%s\n",
		bol, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"Usage:      %sdnsanity%s %s[flags]%s\n",
		whi, rst, yel, rst)
	s += fmt.Sprintf(
		"Example:    %sdnsanity%s %s-list%s /tmp/untrusted-dns.txt %s-o%s /tmp/trusted-dns.txt\n",
		whi, rst, yel, rst, yel, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sGENERIC OPTIONS:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-o%s %sstring%s                  file to write output (defaults to STDOUT)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-global-ratelimit%s %sint%s      global max requests per second (default 500)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-threads%s %sint%s               max concurrency (default: %s-global-ratelimit * 2%s)\n",
		yel, rst, gra, rst, yel, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sSERVERS SANITIZATION:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-list%s %sstring%s               list of DNS servers to sanitize (file or comma separated or STDIN)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-timeout%s %sint%s               timeout in seconds for DNS queries (default 4)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-ratelimit%s %sint%s             max requests per second per DNS server (default 2)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-max-attempts%s %sint%s          max attempts before marking a mismatching DNS test as failed (default 2)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-max-mismatches%s %sint%s        max allowed mismatching DNS tests per server (default 0)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sTEMPLATE VALIDATION:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-template%s %sstring%s           path to the DNSanity validation template (-verbose to show)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-trusted-list%s %sstring%s       list of TRUSTED servers (defaults to \"8.8.8.8, 1.1.1.1, 9.9.9.9\")\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-trusted-timeout%s %sint%s       timeout in seconds for TRUSTED servers (default 2)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-trusted-ratelimit%s %sint%s     max requests per second per TRUSTED server (default 10)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf(
		"   %s-trusted-max-attempts%s %sint%s  max attempts before marking a mismatching TRUSTED test as failed (default 2)\n",
		yel, rst, gra, rst)
	s += fmt.Sprintf("\n")

	s += fmt.Sprintf(
		"%sDEBUG:%s\n",
		bol, rst)
	s += fmt.Sprintf(
		"   %s-h, --help%s                 show help\n",
		yel, rst)
	// s += fmt.Sprintf(
	// 	"   %s-full-help%s                 show advanced help to get an overview of dnsanity's workflow\n",
	// 	yel, rst)
	s += fmt.Sprintf(
		"   %s-version%s                   display version of dnsanity\n",
		yel, rst)
	s += fmt.Sprintf(
		"   %s-verbose%s                   show configuration and template\n",
		yel, rst)
	s += fmt.Sprintf("\n")
	tty.SmartFprintf(os.Stdout, s)
	os.Exit(0)
}

func ShowVersion() {
	tty.SmartFprintf(
		os.Stdout,
		"DNSanity %s <http://github.com/nil0x42/dnsanity>",
		VERSION,
	)
	os.Exit(0)
}

func ParseOptions() (*Options, error) {
	opts := &Options{}
	// GENERIC OPTIONS
	flag.StringVar(&opts.OutputFilePath, "o", "/dev/stdout", "file to write output")
	flag.IntVar(&opts.GlobRateLimit, "global-ratelimit", 500, "global rate limit")
	flag.IntVar(&opts.Threads, "threads", -1, "number of threads")
	// SERVER SANITIZATION
	flag.StringVar(&opts.UntrustedDNS, "list", "", "list of DNS servers to sanitize (file or comma separated or stdin)")
	flag.IntVar(&opts.Timeout, "timeout", 4, "timeout in seconds for DNS queries")
	flag.IntVar(&opts.Ratelimit, "ratelimit", 2, "max requests per second per DNS server")
	flag.IntVar(&opts.Attempts, "max-attempts", 2, "max attempts before marking a mismatching DNS test as failed")
	flag.IntVar(&opts.MaxMismatches, "max-mismatches", 0, "max allowed mismatching tests per DNS server")
	// TEMPLATE VALIDATION
	flag.StringVar(&opts.Template, "template", "", "path to the DNSanity validation template")
	flag.StringVar(&opts.TrustedDNS, "trusted-list", "8.8.8.8, 1.1.1.1, 9.9.9.9", "list of TRUSTED servers")
	flag.IntVar(&opts.TrustedTimeout, "trusted-timeout", 2, "timeout in seconds for TRUSTED servers")
	flag.IntVar(&opts.TrustedRatelimit, "trusted-ratelimit", 10, "max requests per second per TRUSTED server")
	flag.IntVar(&opts.TrustedAttempts, "trusted-max-attempts", 2, "max attempts before marking a mismatching TRUSTED test as failed")
	// DEBUG
	flag.BoolVar(&opts.ShowHelp, "h", false, "show help")
	// flag.BoolVar(&opts.ShowFullHelp, "full-help", false, "show advanced help")
	flag.BoolVar(&opts.ShowVersion, "version", false, "display version of dnsanity")
	flag.BoolVar(&opts.Verbose, "verbose", false, "show configuration and template")
	flag.Usage = ShowHelp

	flag.Parse()

	// if opts.ShowHelp || opts.ShowFullHelp {
	if opts.ShowHelp {
		flag.Usage()
	}

	if opts.ShowVersion {
		ShowVersion()
	}

	if opts.GlobRateLimit < 1 {
		opts.GlobRateLimit = 9999
	}
	if opts.Threads < 1 {
		if opts.GlobRateLimit < 50 {
			opts.Threads = 100
		} else {
			opts.Threads = opts.GlobRateLimit * 2
		}
	}

	return opts, nil
}

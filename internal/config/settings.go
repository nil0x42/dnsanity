package config

import (
	"github.com/nil0x42/dnsanity/internal/dns"
)

type Settings struct {
	// global
	ServerIPs			[]string
	Template			dns.Template
	MaxThreads			int
	MaxPoolSize			int
	GlobRateLimit		int
	// per server
	PerSrvRateLimit		float64
	PerSrvMaxFailures	int
	// per check
	PerCheckMaxAttempts	int
	// per dns query
	PerQueryTimeout		int
}

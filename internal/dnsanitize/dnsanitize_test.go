// Fichier: internal/dnsanitize/dnsanitize_test.go

package dnsanitize

import (
	"testing"

	"github.com/nil0x42/dnsanity/internal/dns"
)

// mockDNSResolve can be replaced by an actual mock later for advanced testing
// For now, we'll rely on the existing dns.ResolveDNS plus small tests verifying behavior
func TestDNSanitizeSimple(t *testing.T) {
	// Prepare test servers
	servers := []string{"8.8.8.8", "1.1.1.1"}
	// Prepare test queries
	tests := []dns.DNSAnswer{
		{Domain: "dnssec-failed.org", Status: "SERVFAIL"},
		{Domain: "dn05jq2u.fr", Status: "NXDOMAIN"},
	}

	srvStates := DNSanitize(
		servers,
		tests,
		10,   // globRateLimit
		2,    // maxThreads
		2,    // rateLimit
		2,    // timeout in seconds
		0,    // maxFailures => server disabled on first mismatch
		1,    // maxAttempts
		nil,  // onTestDone callback
	)

	if len(srvStates) != 2 {
		t.Fatalf("Got %d server states, want 2", len(srvStates))
	}
}


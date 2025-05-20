// resolver_test.go
package dns

import (
    "context"
    "testing"
    "time"
	"strings"
)

// testCase models one ResolveDNS scenario
type testCase struct {
    name        string
    domain      string
    server      string
    timeout     time.Duration
    cancelCtx   bool
    wantStatus  string       // exact expected Status string
    wantA       bool         // expect at least one A record
    wantCNAME   bool         // expect at least one CNAME record
}

// TestResolveDNS exhaustively exercises ResolveDNS covering every branch.
func TestResolveDNS(t *testing.T) {
    t.Parallel()

    // helper to make a context (possibly already cancelled)
    mkCtx := func(cancel bool) context.Context {
        ctx, cancelFn := context.WithCancel(context.Background())
        if cancel {
            cancelFn()
        }
        return ctx
    }

    // NOTE: The public DNS 8.8.8.8 is used for successful lookups because it is
    // globally reachable in the vast majority of CI environments.  Reserved
    // documentation‑prefix addresses (RFC 5737) are leveraged to produce
    // deterministic network errors without needing privileged ports or external
    // dependencies.
    tests := []testCase{
        {
            name:       "SuccessARecord",
            domain:     "example.com",
            server:     "8.8.8.8",
            timeout:    4 * time.Second,
            wantStatus: "NOERROR",
            wantA:      true,
        },
        {
            name:       "SuccessCNAME",
            domain:     "www.apple.com",
            server:     "8.8.8.8",
            timeout:    4 * time.Second,
            wantStatus: "NOERROR",
            wantCNAME:  true,
            wantA:      true,
        },
        {
            name:       "NXDOMAIN",
            domain:     "this-domain-should-not-exist.invalid",
            server:     "8.8.8.8",
            timeout:    4 * time.Second,
            wantStatus: "NXDOMAIN",
        },
        {
            name:       "Timeout",
            domain:     "example.com",
            server:     "192.0.2.1", // TEST‑NET‑1 (no host responds)
            timeout:    500 * time.Millisecond,
            wantStatus: "TIMEOUT",
        },
        {
            name:       "ConnectionRefused",
            domain:     "example.com",
            server:     "127.0.0.1", // loopback, assuming nothing on port 53
            timeout:    500 * time.Millisecond,
            wantStatus: "ECONNREFUSED",
        },
        {
            name:       "InvalidServer",
            domain:     "example.com",
            server:     "1203.0.113.1", // TEST‑NET‑3, usually unroutable
            timeout:    500 * time.Millisecond,
            wantStatus: ": no such host",
        },
        {
            name:       "ContextCanceled",
            domain:     "example.com",
            server:     "8.8.8.8",
            timeout:    4 * time.Second,
            cancelCtx:  true,
            wantStatus: "ERROR - ",
        },
    }

    for _, tc := range tests {
        tc := tc // capture
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()

            ctx := mkCtx(tc.cancelCtx)
            ans := ResolveDNS(tc.domain, tc.server, tc.timeout, ctx)

            if !strings.Contains(ans.Status,tc.wantStatus) {
                t.Fatalf("Status mismatch for %s: got %q, want %q", tc.name, ans.Status, tc.wantStatus)
            }

            if tc.wantA && len(ans.A) == 0 {
                t.Fatalf("%s: expected at least one A record, got none", tc.name)
            }
            if tc.wantCNAME && len(ans.CNAME) == 0 {
                t.Fatalf("%s: expected at least one CNAME record, got none", tc.name)
            }

            // Generic sanity: Domain field should echo input.
            if ans.Domain != tc.domain {
                t.Fatalf("%s: Domain mismatch, got %q want %q", tc.name, ans.Domain, tc.domain)
            }
        })
    }
}

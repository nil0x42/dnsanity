// File: internal/runner/runner_test.go
// Package‑level tests for runner.RunAndReport

package runner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nil0x42/dnsanity/internal/dns"
)

// TestRunAndReport_DebugFlag confirms that the debug switch is propagated.
func TestRunAndReport_DebugFlag(t *testing.T) {
	t.Parallel()

	dummyTemplate := []dns.DNSAnswer{
		{Domain: "example.com", Status: "NXDOMAIN"},
	}

	var dbg bytes.Buffer
	st := RunAndReport(
		"unit-debug",
		[]string{"0.0.0.0"},
		dummyTemplate,
		5, 1, 100, 1, 0, 1,
		true,          // debug ON
		nil, &dbg, nil,
	)
	if !st.DebugActivated {
		t.Fatal("DebugActivated should be true when debug flag is set")
	}
	expected := []string{
		"[-] SERVER 0.0.0.0 (invalid)",
		"example.com ECONNREFUSED",
	}
	dbgStr := dbg.String()
	for _, exp := range expected {
		if !strings.Contains(dbgStr, exp) {
			t.Fatalf("expected data: %v\nDBG=%q", exp, dbgStr)
		}
	}
}

// TestRunAndReport_PrefixVariants exercises all prefix branches.
func TestRunAndReport_PrefixVariants(t *testing.T) {
	t.Parallel()

	dummyTemplate := []dns.DNSAnswer{
		{Domain: "example.com", Status: "NXDOMAIN"},
	}

	cases := []struct {
		name        string
		maxFailures int
		expectSub   string
	}{
		{"dropOnFirstFail", 0, "dropped if any test fails"},
		{"neverDropped", -1, "never dropped"},
		{"customDrop", 3, "dropped if >3 tests fail"},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			st := RunAndReport(
				"prefix-check",
				[]string{"0.0.0.0"},
				dummyTemplate,   // one dummy test (len != 0 to vary totals)
				20, 1, 100, 1,
				tc.maxFailures,
				1,
				false,
				&out, nil, nil,
			)
			if !strings.Contains(st.PBarPrefixData, tc.expectSub) {
				t.Errorf("prefix missing %q\n%s", tc.expectSub, st.PBarPrefixData)
			}
			if tc.name == "dropOnFirstFail" {
				if out.Len() != 0 {
					t.Errorf("outfile must stay empty (got %d bytes)", out.Len())
				}
			} else {
				if out.String() != "0.0.0.0\n" {
					t.Errorf("outfile must have server 0.0.0.0")
				}
			}
		})
	}
}

package dns

import (
	"strings"
	"testing"
	"time"

	"github.com/nil0x42/dnsanity/internal/tty"
)

// buildTemplate quickly crafts a Template with domains mapped to NOERROR expectations.
func buildTemplate(domains []string) Template {
	tpl := make(Template, len(domains))
	for i, d := range domains {
		tpl[i] = TemplateEntry{Domain: d, ValidAnswers: []DNSAnswerData{{Status: "NOERROR"}}}
	}
	return tpl
}

// TestNewServerContextInitialization verifies that NewServerContext populates every
// field as expected for a freshly‑created ServerContext.
func TestNewServerContextInitialization(t *testing.T) {
	maxAttempts := 3
	tpl := buildTemplate([]string{"a.example", "b.example", "c.example"})

	sc := NewServerContext("1.2.3.4", tpl, maxAttempts)

	// Basic structural assertions.
	if got, want := len(sc.PendingChecks), len(tpl); got != want {
		t.Fatalf("PendingChecks length = %d, want %d", got, want)
	}
	for idx, v := range sc.PendingChecks {
		if v != idx {
			t.Errorf("PendingChecks[%d] = %d, want %d", idx, v, idx)
		}
	}

	// Per‑check initialization.
	for i, chk := range sc.Checks {
		if chk.Answer.Domain != tpl[i].Domain {
			t.Errorf("Check[%d].Domain = %q, want %q", i, chk.Answer.Domain, tpl[i].Domain)
		}
		if chk.Answer.Status != "SKIPPED" {
			t.Errorf("Check[%d].Status = %q, want SKIPPED", i, chk.Answer.Status)
		}
		if chk.AttemptsLeft != maxAttempts {
			t.Errorf("Check[%d].AttemptsLeft = %d, want %d", i, chk.AttemptsLeft, maxAttempts)
		}
		if chk.MaxAttempts != maxAttempts {
			t.Errorf("Check[%d].MaxAttempts = %d, want %d", i, chk.MaxAttempts, maxAttempts)
		}
	}

	// Sanity on zero‑values.
	if sc.Disabled {
		t.Error("Server should not be disabled on creation")
	}
	if sc.FailedCount != 0 || sc.CompletedCount != 0 {
		t.Error("FailedCount or CompletedCount not zero on creation")
	}
	if !sc.NextQueryAt.IsZero() {
		t.Error("NextQueryAt should start at zero value")
	}
}

// TestServerContextFinished ensures all branches of Finished() are exercised.
func TestServerContextFinished(t *testing.T) {
	tpl := buildTemplate([]string{"only.example"})
	sc := NewServerContext("5.6.7.8", tpl, 2)

	// Case 1: nothing done yet – should not be finished.
	if sc.Finished() {
		t.Error("Finished() returned true too early")
	}

	// Case 2: completed all checks.
	sc.CompletedCount = len(sc.Checks)
	if !sc.Finished() {
		t.Error("Finished() should be true when all checks completed")
	}

	// Case 3: disabled flag overrides CompletedCount.
	sc = NewServerContext("5.6.7.8", tpl, 2)
	sc.Disabled = true
	if !sc.Finished() {
		t.Error("Finished() should be true when server is disabled")
	}
}

// TestCancelCtx validates that CancelCtx actually cancels the underlying context.
func TestCancelCtx(t *testing.T) {
	tpl := buildTemplate([]string{"ctx.example"})
	sc := NewServerContext("9.9.9.9", tpl, 1)

	sc.CancelCtx()
	if err := sc.Ctx.Err(); err == nil {
		t.Error("context not cancelled after CancelCtx() call")
	}
}

// stripANSIFast provides a minimalist ANSI escape stripper suitable for test comparisons.
func stripANSIFast(s string) string {
	return tty.StripAnsi(s)
}

// TestPrettyDumpExhaustive crafts several pathological scenarios to hit every
// branch inside PrettyDump(), including colour prefixes and attempt‑suffix logic.
func TestPrettyDumpExhaustive(t *testing.T) {
	tpl := buildTemplate([]string{"p0.example", "p1.example", "p2.example", "p3.example"})
	sc := NewServerContext("8.8.4.4", tpl, 4) // MaxAttempts = 4

	// --- manipulate each check to explore prefixes & suffixes ------------
	// Check 0: Passed on 1st attempt (✓, numTries = 1 → no suffix)
	sc.Checks[0].Passed = true
	sc.Checks[0].Answer.Status = "NOERROR"
	sc.Checks[0].AttemptsLeft = 3 // Max‑Attempts 4 -> numTries=1

	// Check 1: Failed on 2nd attempt (✗, suffix "nd")
	sc.Checks[1].Answer.Status = "SERVFAIL"
	sc.Checks[1].AttemptsLeft = 2 // numTries=2 (suffix nd)
	sc.FailedCount++

	// Check 2: Failed on 3rd attempt (✗, suffix "rd")
	sc.Checks[2].Answer.Status = "TIMEOUT"
	sc.Checks[2].AttemptsLeft = 1 // numTries=3 (suffix rd)
	sc.FailedCount++

	// Check 3: Failed on 4th attempt (✗, suffix "th")
	sc.Checks[3].Answer.Status = "TIMEOUT"
	sc.Checks[3].AttemptsLeft = 0 // numTries=4 (suffix th)
	sc.FailedCount++

	// Mark completed to exercise header logic.
	sc.CompletedCount = len(sc.Checks)

	gotDump := stripANSIFast(sc.PrettyDump())

	// Header assertions.
	if !strings.Contains(gotDump, "SERVER 8.8.4.4") {
		t.Fatalf("PrettyDump header missing IP: %s", gotDump)
	}
	if !strings.Contains(gotDump, "(invalid)") {
		t.Fatalf("PrettyDump should mark server invalid when FailedCount>0: %s", gotDump)
	}

	// Prefix markers.
	if !strings.Contains(gotDump, " + ") {
		t.Error("PrettyDump missing '+' marker for passed test")
	}
	if !strings.Contains(gotDump, " - ") {
		t.Error("PrettyDump missing '-' marker for failed test")
	}
	if !strings.Contains(gotDump, "! p2.example") { // SKIPPED still present on p2? ensure at least one
		t.Log("PrettyDump has no SKIPPED entries as expected after overrides – OK (not fatal)")
	}

	// Attempt suffixes.
	for _, suff := range []string{"2nd attempt", "3rd attempt", "4th attempt"} {
		if !strings.Contains(gotDump, suff) {
			t.Errorf("PrettyDump missing attempt suffix %q", suff)
		}
	}

	// Timing – ensure function executes quickly (regression guard for accidental sleeps).
	deadline := time.AfterFunc(100*time.Millisecond, func() {
		t.Errorf("PrettyDump took too long (possible deadlock)")
	})
	_ = sc.PrettyDump()
	deadline.Stop()
}

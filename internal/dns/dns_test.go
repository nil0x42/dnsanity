//go:build !integration
// +build !integration

package dns

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"fmt"
)

/* ------------------------------------------------------------------------- */
/* ------------------------------ PARSING ---------------------------------- */
/* ------------------------------------------------------------------------- */

// TestDNSAnswerFromString validates parsing success and failure paths.
func TestDNSAnswerFromString(t *testing.T) {
	cases := []struct {
		line       string
		wantErr    bool
		wantStatus string
		wantA      int
		wantCNAME  int
	}{
		{"example.com A=1.2.3.4", false, "NOERROR", 1, 0},
		{"foo NXDOMAIN", false, "NXDOMAIN", 0, 0},
		{"foo SERVFAIL", false, "SERVFAIL", 0, 0},
		{"bar CNAME=baz. A=10.1.1.1", false, "NOERROR", 1, 1},
		{"incomplete", true, "", 0, 0},
		// parser accepts "A=" as empty record → treated as one A + status NOERROR
		{"bad A=", false, "NOERROR", 1, 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.line, func(t *testing.T) {
			ans, err := DNSAnswerFromString(tc.line)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error mismatch for %q: got err=%v, wantErr=%v", tc.line, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if ans.Status != tc.wantStatus {
				t.Errorf("status mismatch: got %q, want %q", ans.Status, tc.wantStatus)
			}
			if len(ans.A) != tc.wantA {
				t.Errorf("A count mismatch: got %d, want %d", len(ans.A), tc.wantA)
			}
			if len(ans.CNAME) != tc.wantCNAME {
				t.Errorf("CNAME count mismatch: got %d, want %d", len(ans.CNAME), tc.wantCNAME)
			}
			// round-trip check
			round, _ := DNSAnswerFromString(ans.ToString())
			if !ans.Equals(round) {
				t.Errorf("round-trip failed: orig=%+v round=%+v", ans, round)
			}
		})
	}
}

func TestDNSAnswerFromString_CNAMEOnly(t *testing.T) {
    ans, err := DNSAnswerFromString("foo CNAME=alias.example.")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if ans.Status != "NOERROR" {
        t.Errorf("got status %q, want NOERROR", ans.Status)
    }
    if len(ans.CNAME) != 1 || ans.CNAME[0] != "alias.example." {
        t.Errorf("CNAME mismatch: %+v", ans.CNAME)
    }
    if len(ans.A) != 0 {
        t.Errorf("expected no A records, got %+v", ans.A)
    }
}

func TestDNSAnswerFromString_MultipleA(t *testing.T) {
    line := "x.com A=3.3.3.3 A=1.1.1.1 A=2.2.2.2"
    ans, err := DNSAnswerFromString(line)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    want := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
    for i := range want {
        if ans.A[i] != want[i] {
            t.Errorf("A[%d]: got %q, want %q", i, ans.A[i], want[i])
        }
    }
}

// TestDNSAnswerToString covers formatting with no records and mixed records.
func TestDNSAnswerToString(t *testing.T) {
	// no records → Domain + Status
	da1 := &DNSAnswer{Domain: "nx", Status: "NXDOMAIN"}
	if got := da1.ToString(); got != "nx NXDOMAIN" {
		t.Errorf("ToString() no-records: got %q, want %q", got, "nx NXDOMAIN")
	}
	// mixed A + CNAME unordered → sorted output
	da2 := &DNSAnswer{
		Domain: "d",
		A:      []string{"2.2.2.2", "1.1.1.1"},
		CNAME:  []string{"z.example.", "a.example."},
	}
	want := "d A=2.2.2.2 A=1.1.1.1 CNAME=z.example. CNAME=a.example."
	if got := da2.ToString(); got != want {
		t.Errorf("ToString() mixed-records: got %q, want %q", got, want)
	}
}

// TestDNSAnswerSliceLoaders exercises loading templates from string and file.
func TestDNSAnswerSliceLoaders(t *testing.T) {
	multi := `
# comment
example.com A=1.1.1.1
foo NXDOMAIN
`
	sl, err := DNSAnswerSliceFromString(multi)
	if err != nil || len(sl) != 2 {
		t.Fatalf("slice from string failed: err=%v len=%d", err, len(sl))
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tmpl.txt")
	if err := os.WriteFile(path, []byte(multi), 0644); err != nil {
		t.Fatal(err)
	}
	sl2, err := DNSAnswerSliceFromFile(path)
	if err != nil || len(sl2) != 2 {
		t.Fatalf("slice from file failed: err=%v len=%d", err, len(sl2))
	}
}

func TestParseDNSAnswers_InlineComments(t *testing.T) {
    input := "a.com A=1.2.3.4 # ceci est un commentaire\n"
    sl, err := DNSAnswerSliceFromString(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(sl) != 1 || sl[0].Domain != "a.com" {
        t.Errorf("parse inline comment failed, got %+v", sl)
    }
}

// TestDNSAnswerSliceFromString_Errors covers empty and malformed inputs.
func TestDNSAnswerSliceFromString_Errors(t *testing.T) {
	// empty input → errNoEntries → "Can't find any entry"
	if _, err := DNSAnswerSliceFromString(""); err == nil || !strings.Contains(err.Error(), "Can't find any entry") {
		t.Errorf("expected 'Can't find any entry', got %v", err)
	}
	// malformed line → wrap error with line number
	bad := "foo BADREC"
	if _, err := DNSAnswerSliceFromString(bad); err == nil || !strings.Contains(err.Error(), "line 1:") {
		t.Errorf("expected parse error on line 1, got %v", err)
	}
}

type errReader struct{}
func (errReader) Read(p []byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestParseDNSAnswers_ScannerError(t *testing.T) {
    // appel direct de parseDNSAnswers pour simuler une erreur de lecture
    _, err := parseDNSAnswers(errReader{}, func(e error, n int) error { return e })
    if err == nil || !strings.Contains(err.Error(), "boom") {
        t.Errorf("expected scanner error 'boom', got %v", err)
    }
}

// TestDNSAnswerSliceFromFile_Errors covers file-open and comment-only cases.
func TestDNSAnswerSliceFromFile_Errors(t *testing.T) {
	// non-existent file → os.Open error wrapped
	_, err := DNSAnswerSliceFromFile("/no/such/file")
	if err == nil || !strings.Contains(err.Error(), "/no/such/file") {
		t.Errorf("expected file open error, got %v", err)
	}
	// file with only comments → errNoEntries → "Can't find any entry"
	dir := t.TempDir()
	path := filepath.Join(dir, "only.txt")
	os.WriteFile(path, []byte("# only comment\n\n"), 0644)
	if _, err := DNSAnswerSliceFromFile(path); err == nil || !strings.Contains(err.Error(), "Can't find any entry") {
		t.Errorf("expected 'Can't find any entry' for comment-only file, got %v", err)
	}
}

/* ------------------------------------------------------------------------- */
/* ------------------------------ MATCHING --------------------------------- */
/* ------------------------------------------------------------------------- */

// TestMatchRecordAndRecords covers basic wildcard and slice logic.
func TestMatchRecordAndRecords(t *testing.T) {
	// simple wildcard
	if !matchRecord("192.168.*.*", "192.168.1.1") {
		t.Error("matchRecord wildcard failed")
	}
	if matchRecord("192.168.*.*", "10.0.0.1") {
		t.Error("matchRecord false positive")
	}
	// slice match
	patterns := []string{"192.168.*.*", "10.0.0.1"}
	values := []string{"192.168.1.2", "10.0.0.1"}
	if !matchRecords(patterns, values) {
		t.Error("matchRecords should match")
	}
	// mismatch
	valuesBad := []string{"192.168.1.2", "10.0.0.2"}
	if matchRecords(patterns, valuesBad) {
		t.Error("matchRecords false positive")
	}
}

func TestMatchRecord_Exact(t *testing.T) {
    if !matchRecord("1.2.3.4", "1.2.3.4") {
        t.Error("exact match should succeed")
    }
    if matchRecord("1.2.3.4", "1.2.3.5") {
        t.Error("different IP should not match")
    }
}

// TestMatchRecord_DomainPatterns covers domain wildcard and case insensitivity.
func TestMatchRecord_DomainPatterns(t *testing.T) {
	if !matchRecord("*.example.com", "Sub.EXAMPLE.com") {
		t.Error("domain wildcard should match case-insensitive")
	}
	if matchRecord("*.example.com", "notexample.com") {
		t.Error("domain wildcard should not match wrong suffix")
	}
}

func TestMatchRecords_Permutation(t *testing.T) {
    patterns := []string{"192.*.1.*", "192.168.*.*", "192.*.*.1"}
    values := []string{"192.168.1.1", "192.169.2.1", "192.170.1.1"}
    if !matchRecords(patterns, values) {
        t.Error("permutation matchRecords should succeed")
    }
}

/* ------------------------------------------------------------------------- */
/* --------------------------- PERMUTATIONS -------------------------------- */
/* ------------------------------------------------------------------------- */

// TestNextPermutation covers standard permutation flow.
func TestNextPermutation(t *testing.T) {
	p := []int{0, 1, 2}
	want := []int{0, 2, 1}
	if !nextPermutation(p) || !equalIntSlice(p, want) {
		t.Errorf("first permutation: got %v, want %v", p, want)
	}
	for nextPermutation(p) {
	}
	if nextPermutation(p) {
		t.Error("nextPermutation should be false at end")
	}
}

// TestNextPermutation_EdgeCases ensures empty or single never permute.
func TestNextPermutation_EdgeCases(t *testing.T) {
	if nextPermutation([]int{}) {
		t.Error("empty slice should not permute")
	}
	if nextPermutation([]int{0}) {
		t.Error("single-element should not permute")
	}
}

// helper for int slice comparison
func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

/* ------------------------------------------------------------------------- */
/* -------------------------- SERVER CONTEXT -------------------------------- */
/* ------------------------------------------------------------------------- */

// TestServerContextHelpers covers Finished() and PrettyDump().
func TestServerContextHelpers(t *testing.T) {
	sc := &ServerContext{
		IPAddress:      "1.1.1.1",
		Checks:         make([]CheckContext, 1),
		PendingChecks:  []int{},
		CompletedCount: 1,
	}
	if !sc.Finished() {
		t.Error("Finished should be true when all checks done")
	}
	sc2 := &ServerContext{IPAddress: "2.2.2.2", Disabled: true}
	if !sc2.Finished() {
		t.Error("Finished should be true when Disabled")
	}
	out := sc.PrettyDump()
	if !strings.Contains(out, sc.IPAddress) {
		t.Error("PrettyDump missing IP")
	}
}

func TestServerContextPrettyDumpDetails(t *testing.T) {
    // build deux checks : 1 passed, 1 skipped, 1 failed 2e essai
    checks := []CheckContext{
        {Answer: DNSAnswer{Domain:"d1", Status:"NOERROR"}, Passed:true, AttemptsLeft:1, MaxAttempts:1},
        {Answer: DNSAnswer{Domain:"d2", Status:"SKIPPED"}, Passed:false, AttemptsLeft:1, MaxAttempts:1},
        {Answer: DNSAnswer{Domain:"d3", Status:"NXDOMAIN"}, Passed:false, AttemptsLeft:1, MaxAttempts:2},
    }
    // AttemptsLeft=1 / MaxAttempts=2 → numTries=1 → pas de suffixe “on 2nd”
    sc := &ServerContext{
        IPAddress:      "9.9.9.9",
        Checks:         checks,
        CompletedCount: 3,
    }
    dump := sc.PrettyDump()
    if !strings.Contains(dump, "[+] SERVER 9.9.9.9") {
        t.Error("missing [+] header for valid server")
    }
    if !strings.Contains(dump, "\n    \033[1;90m!") || !strings.Contains(dump, "SKIPPED") {
        t.Error("missing skipped (!) entry")
    }
    if !strings.Contains(dump, " d3 NXDOMAIN") {
        t.Error("missing NXDOMAIN entry for d3")
    }
    if strings.Contains(dump, "on 2nd attempt") {
        t.Error("unexpected suffix 'on 2nd attempt'")
	}
    // now force 2nd attempt
    sc.Checks[2].AttemptsLeft = 0  // 2 comsummed attempts
    sc.Checks[2].MaxAttempts = 2
    dump2 := sc.PrettyDump()
    if !strings.Contains(dump2, "on 2nd attempt") {
        t.Error("expected suffix 'on 2nd attempt'")
    }
}


/* ------------------------------------------------------------------------- */
/* ----------------------------- RESOLVER ---------------------------------- */
/* ------------------------------------------------------------------------- */

// TestResolveDNS_ErrorPaths covers ECONNREFUSED and TIMEOUT branches.
func TestResolveDNS_ErrorPaths(t *testing.T) {
	t.Run("connection refused", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ans := ResolveDNS("example.com", "0.0.0.0", time.Second, ctx)
		if ans.Status != "ECONNREFUSED" {
			t.Errorf("expected ECONNREFUSED, got %s", ans.Status)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		ans := ResolveDNS("example.com", "192.0.2.1", time.Millisecond, ctx)
		if ans.Status != "TIMEOUT" {
			t.Errorf("expected TIMEOUT, got %s", ans.Status)
		}
	})
}

// TestResolveDNS_ContextCanceled covers generic ERROR- prefix on cancel.
func TestResolveDNS_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ans := ResolveDNS("example.com", "0.0.0.0", time.Millisecond, ctx)
	if !strings.HasPrefix(ans.Status, "ERROR -") {
		t.Errorf("expected ERROR- prefix on canceled context, got %q", ans.Status)
	}
}

/* ------------------------------------------------------------------------- */
/* --------------------------- EQUALS LOGIC -------------------------------- */
/* ------------------------------------------------------------------------- */

// TestDNSAnswerEquals covers SERVFAIL vs TIMEOUT equivalence and domain mismatch.
func TestDNSAnswerEquals(t *testing.T) {
	base := &DNSAnswer{Domain: "foo", Status: "SERVFAIL"}
	timeout := &DNSAnswer{Domain: "foo", Status: "TIMEOUT"}
	if !base.Equals(timeout) || !timeout.Equals(base) {
		t.Error("SERVFAIL and TIMEOUT should be equal")
	}
	neq := &DNSAnswer{Domain: "bar", Status: "SERVFAIL"}
	if base.Equals(neq) {
		t.Error("different domains should not be equal")
	}
}

// TestDNSAnswerEquals_NilAndMismatch covers nil-safe and record mismatches.
func TestDNSAnswerEquals_NilAndMismatch(t *testing.T) {
	var nilAns *DNSAnswer
	real := &DNSAnswer{Domain: "foo", Status: "NOERROR", A: []string{"1.1.1.1"}}
	if nilAns.Equals(real) {
		t.Error("nil.Equals(real) must be false")
	}
	if real.Equals(nil) {
		t.Error("real.Equals(nil) must be false")
	}
	// A-record mismatch
	diffA := &DNSAnswer{Domain: "foo", Status: "NOERROR", A: []string{"2.2.2.2"}}
	if real.Equals(diffA) {
		t.Error("different A records should not be equal")
	}
	// CNAME mismatch
	realC := &DNSAnswer{Domain: "foo", Status: "NOERROR", CNAME: []string{"a"}}
	otherC := &DNSAnswer{Domain: "foo", Status: "NOERROR", CNAME: []string{"b"}}
	if realC.Equals(otherC) {
		t.Error("different CNAME records should not be equal")
	}
}

/* ------------------------------------------------------------------------- */
/* ----------------------------- MISC -------------------------------------- */
/* ------------------------------------------------------------------------- */

// TestPrettyDumpTemplate ensures ANSI markers and entries are present.
func TestPrettyDumpTemplate(t *testing.T) {
	tmpl := []DNSAnswer{{Domain: "foo", Status: "NXDOMAIN"}}
	dump := PrettyDumpTemplate(tmpl)
	if !strings.Contains(dump, "foo") {
		t.Error("template dump missing domain")
	}
	if !strings.Contains(dump, "[*]") {
		t.Error("template dump header missing")
	}
}


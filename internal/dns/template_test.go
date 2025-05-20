package dns

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
    "errors"
    "path/filepath"
)

// TestNewTemplateEntry_Valid ensures that a well‑formed line is parsed correctly.
func TestNewTemplateEntry_Valid(t *testing.T) {
	line := "example.com A=1.1.1.1 || NXDOMAIN || CNAME=alias.example.com. A=2.2.2.2"
	te, err := NewTemplateEntry(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if te.Domain != "example.com" {
		t.Errorf("wrong domain: got %q", te.Domain)
	}
	if len(te.ValidAnswers) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(te.ValidAnswers))
	}
	// Round‑trip via ToString() must preserve semantic content.
	rebuilt, err := NewTemplateEntry(te.ToString())
	if err != nil {
		t.Fatalf("round‑trip failed: %v", err)
	}
	if !reflect.DeepEqual(te, rebuilt) {
		t.Errorf("round‑trip mismatch: %+v vs %+v", te, rebuilt)
	}
}

// TestNewTemplateEntry_Invalid covers all error branches inside NewTemplateEntry().
func TestNewTemplateEntry_Invalid(t *testing.T) {
	cases := []string{
		"",                                      // empty line ➜ <2 tokens
		"onlydomain",                            // single token ➜ <2 tokens
		"bad.com AAAA=::1",                      // unsupported record ➜ invalid record
		"bad.com A=1.1.1.1||BLAH",               // alternative with unsupported token
	}
	for _, c := range cases {
		if _, err := NewTemplateEntry(c); err == nil {
			t.Errorf("expected error for input %q", c)
		}
	}
}

// TestGlobMatch exercises the globMatch helper with tricky patterns.
func TestGlobMatch(t *testing.T) {
	positive := map[string]string{
		"192.168.*.*":           "192.168.10.20",
		"1.1.*.1":               "1.1.99.1",
		"*.example.com":         "sub.domain.EXAMPLE.com", // case‑insensitive
		"*.*.*.*":               "255.255.255.255",
		"foo**bar*baz":          "foobarbaz",            // multiple consecutive '*'
		"foo**bar*baz2":         "fooxbarxxbaz2",         // complex skip
	}
	for pattern, value := range positive {
		if !globMatch(pattern, value) {
			t.Errorf("globMatch(%q,%q) expected true", pattern, value)
		}
	}
	// Negative checks.
	negatives := [][2]string{
		{"192.168.*.*", "10.0.0.1"},
		{"*.example.com", "sub.test.org"},
		{"foo*bar", "foobaz"},
	}
	for _, nv := range negatives {
		if globMatch(nv[0], nv[1]) {
			t.Errorf("globMatch(%q,%q) expected false", nv[0], nv[1])
		}
	}
}

// TestMatchRecords verifies exhaustive permutations and size mismatch logic.
func TestMatchRecords(t *testing.T) {
	patterns := []string{"A", "B"}
	valuesAB := []string{"B", "A"}
	if !matchRecords(patterns, valuesAB) {
		t.Error("permutation should match")
	}
	// Size mismatch ➜ false.
	if matchRecords(patterns[:1], valuesAB) {
		t.Error("size mismatch must fail")
	}
	// Glob inside patterns.
	globPatterns := []string{"192.168.*.1", "10.*.0.1"}
	globValues := []string{"10.123.0.1", "192.168.99.1"}
	if !matchRecords(globPatterns, globValues) {
		t.Error("glob patterns should match values irrespective of order")
	}
}

// TestNextPermutation enumerates all permutations of a length‑3 slice.
func TestNextPermutation(t *testing.T) {
	p := []int{0, 1, 2}
	seen := map[[3]int]bool{}
	for {
		var key [3]int
		copy(key[:], p)
		if seen[key] {
			t.Fatalf("duplicate permutation %v", p)
		}
		seen[key] = true
		if !nextPermutation(p) {
			break
		}
	}
	if len(seen) != 6 {
		t.Fatalf("expected 6 permutations, got %d", len(seen))
	}
	// last permutation must be descending order.
	if got := [3]int{p[0], p[1], p[2]}; got != [3]int{2, 1, 0} {
		t.Errorf("unexpected last permutation %v", got)
	}
}

// TestTemplateEntry_Matches covers success and failure scenarios.
func TestTemplateEntry_Matches(t *testing.T) {
	entryLine := "service.local A=10.0.*.1 || NXDOMAIN"
	entry, _ := NewTemplateEntry(entryLine)

	// Successful A record match (glob).
	ans := &DNSAnswer{
		Domain: "service.local",
		DNSAnswerData: DNSAnswerData{
			Status: "NOERROR",
			A:      []string{"10.0.50.1"},
		},
	}
	if !entry.Matches(ans) {
		t.Error("expected A record to match")
	}
	// Successful NXDOMAIN alternative.
	ans2 := &DNSAnswer{Domain: "service.local", DNSAnswerData: DNSAnswerData{Status: "NXDOMAIN"}}
	if !entry.Matches(ans2) {
		t.Error("expected NXDOMAIN to match")
	}
	// Domain mismatch ➜ false.
	ans3 := &DNSAnswer{Domain: "other.local", DNSAnswerData: DNSAnswerData{Status: "NXDOMAIN"}}
	if entry.Matches(ans3) {
		t.Error("domain mismatch must fail")
	}
}

// TestLoadTemplate_StringInput hits the happy path and PrettyDump().
func TestLoadTemplate_StringInput(t *testing.T) {
	tmpl := `
	# comment line
	example.com A=1.1.1.1
	invalid.com NXDOMAIN || SERVFAIL
	`
	tpls, err := NewTemplate(tmpl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tpls) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(tpls))
	}
	// PrettyDump should include exactly two bullet lines.
	dump := tpls.PrettyDump()
	if cnt := strings.Count(dump, "* "); cnt != 2 {
		t.Errorf("PrettyDump should list 2 entries, got %d", cnt)
	}
}

// TestLoadTemplate_ErrorPaths exercises errNoEntries and invalid line handling.
func TestLoadTemplate_ErrorPaths(t *testing.T) {
	// 1) Only comments ➜ errNoEntries.
	if _, err := NewTemplate("# just a comment\n\n"); err == nil {
		t.Error("expected error for empty template")
	}
	// 2) Line with invalid record token.
	bad := "bad.com AAAA=::1"
	if _, err := NewTemplate(bad); err == nil {
		t.Error("expected error for invalid record token")
	}
}

// TestNewTemplateFromFile creates a temporary file to ensure the file path branch executes.
func TestNewTemplateFromFile(t *testing.T) {
	content := `
	# comment
	example.org NXDOMAIN
	cr.yp.to	A=131.193.32.108 A=131.193.32.109 || TIMEOUT||A=*
	`
	tmp, err := ioutil.TempFile("", "tpl*.txt")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	tmp.Close()

	tpl, err := NewTemplateFromFile(tmp.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tpl) != 2 || tpl[0].Domain != "example.org" || len(tpl[1].ValidAnswers) != 3 {
		t.Errorf("template not loaded correctly: %+v", tpl)
	}
}


// createTempFile is a helper that writes content to a fresh temporary file and returns the file path and a cleanup func.
func createTempFile(t *testing.T, content string) (string, func()) {
    t.Helper()
    tmp, err := ioutil.TempFile("", "tpl*.txt")
    if err != nil {
        t.Fatalf("tempfile: %v", err)
    }
    if _, err := tmp.WriteString(content); err != nil {
        t.Fatalf("write tmp: %v", err)
    }
    if err := tmp.Close(); err != nil {
        t.Fatalf("close tmp: %v", err)
    }
    return tmp.Name(), func() { os.Remove(tmp.Name()) }
}

// TestNewTemplateFromFile_FileNotFound ensures that the function correctly wraps os.Open errors.
func TestNewTemplateFromFile_FileNotFound(t *testing.T) {
    t.Parallel()
    fakePath := filepath.Join(os.TempDir(), "definitely‑not‑exist‑12345.txt")
    _, err := NewTemplateFromFile(fakePath)
    if err == nil {
        t.Fatal("expected error for missing file")
    }
    // The wrapped error must preserve os.ErrNotExist.
    if !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("expected not‑exist error, got: %v", err)
    }
    if !strings.Contains(err.Error(), fakePath) {
        t.Errorf("error message should contain the file path; got %v", err)
    }
}

// TestNewTemplateFromFile_EmptyFile checks the branch that reports "Can't find any entry".
func TestNewTemplateFromFile_EmptyFile(t *testing.T) {
    t.Parallel()
    path, cleanup := createTempFile(t, "# just a comment\n\n   \n")
    defer cleanup()

    _, err := NewTemplateFromFile(path)
    if err == nil {
        t.Fatal("expected error for file with no entries")
    }
    if !strings.Contains(err.Error(), "Can't find any entry") {
        t.Errorf("unexpected error message: %v", err)
    }
}

// TestNewTemplateFromFile_InvalidLineNumber validates error wrapping and accurate line numbers.
func TestNewTemplateFromFile_InvalidLineNumber(t *testing.T) {
    t.Parallel()
    content := "valid.com NXDOMAIN\ninvalid.com AAAA=::1\n"
    path, cleanup := createTempFile(t, content)
    defer cleanup()

    _, err := NewTemplateFromFile(path)
    if err == nil {
        t.Fatal("expected parsing error")
    }
    if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line 2") {
        t.Errorf("error should include path and line 2; got %v", err)
    }
}

// TestNewTemplateFromFile_ValidComplex explores a realistic file with comments, blank lines, globs and multiple alternatives.
func TestNewTemplateFromFile_ValidComplex(t *testing.T) {
    t.Parallel()
    complex := `
    # initial blank lines and comments should be ignored



    example.net A=1.1.1.1 || A=2.2.2.2
    test.invalid   NXDOMAIN
    wildcard.com  A=192.168.*.*  || SERVFAIL
    `

    path, cleanup := createTempFile(t, complex)
    defer cleanup()

    tpl, err := NewTemplateFromFile(path)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(tpl) != 3 {
        t.Fatalf("expected 3 parsed entries, got %d", len(tpl))
    }

    domains := []string{tpl[0].Domain, tpl[1].Domain, tpl[2].Domain}
    expected := []string{"example.net", "test.invalid", "wildcard.com"}
    for i, exp := range expected {
        if domains[i] != exp {
            t.Errorf("entry %d domain mismatch: want %q got %q", i, exp, domains[i])
        }
    }
    if l := len(tpl[0].ValidAnswers); l != 2 {
        t.Errorf("first entry expected 2 alternatives, got %d", l)
    }
    if l := len(tpl[2].ValidAnswers); l != 2 {
        t.Errorf("third entry expected 2 alternatives, got %d", l)
    }
}

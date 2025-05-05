//go:build !integration
// +build !integration

package config

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// backupAndResetFlags creates an isolated FlagSet and os.Args.
// It returns a restore closure that must be deferred.
func backupAndResetFlags(t *testing.T, args ...string) func() {
	t.Helper()

	// Backup global state.
	origCommandLine := flag.CommandLine
	origArgs := os.Args

	// Replace it with a fresh one.
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ExitOnError)
	os.Args = args

	// Make sure Help/Version do NOT kill the test process.
	flag.Usage = func() {} // no‑op

	return func() {
		flag.CommandLine = origCommandLine
		os.Args = origArgs
	}
}

/* ------------------------------------------------------------------------- */
/* -------------------------- ParseServerList ------------------------------ */

func TestParseServerList(t *testing.T) {
	tmp := t.TempDir()

	// Create a sample list file.
	path := filepath.Join(tmp, "dns.lst")
	content := "8.8.8.8 # google\n1.1.1.1, 9.9.9.9\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write tmp file: %v", err)
	}

	cases := []struct {
		name      string
		input     string
		want      []string
		wantError bool
	}{
		{
			name:  "comma + spaces",
			input: "8.8.8.8 , 1.1.1.1",
			want:  []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name:  "file path",
			input: path,
			want:  []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"},
		},
		{
			name:      "invalid IP",
			input:     "256.0.0.1",
			wantError: true,
		},
		{
			name:      "empty",
			input:     "",
			wantError: true,
		},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseServerList(tc.input)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

/* ------------------------------------------------------------------------- */
/* ----------------------------- OpenFile ---------------------------------- */

func TestOpenFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "out.txt")

	// Case 1: default to Stdout.
	if f, err := OpenFile(""); err != nil || f != os.Stdout {
		t.Fatalf("OpenFile(\"\") = %v, %v, want Stdout", f, err)
	}

	// Case 2: explicit stdout variants.
	for _, s := range []string{"-", "/dev/stdout"} {
		if f, err := OpenFile(s); err != nil || f != os.Stdout {
			t.Fatalf("OpenFile(%q) = %v, %v, want Stdout", s, f, err)
		}
	}

	// Case 3: real file creation.
	f, err := OpenFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()
	info, _ := os.Stat(p)
	if info.IsDir() {
		t.Fatalf("expected regular file, got directory")
	}
}

/* ------------------------------------------------------------------------- */
/* ----------------------------- ParseOptions ------------------------------ */

func TestParseOptions(t *testing.T) {
	restore := backupAndResetFlags(t,
		"dnsanity",
		"-list", "1.1.1.1",
		"-threads", "10",
		"-global-ratelimit", "50",
		"-template", "",
		"-o", "/dev/stdout",
	)
	defer restore()

	opts, err := ParseOptions()
	if err != nil {
		t.Fatalf("ParseOptions error: %v", err)
	}

	if opts.Threads != 10 {
		t.Errorf("Threads = %d, want 10", opts.Threads)
	}
	if opts.GlobRateLimit != 50 {
		t.Errorf("GlobRateLimit = %d, want 50", opts.GlobRateLimit)
	}
	if opts.UntrustedDNS != "1.1.1.1" {
		t.Errorf("UntrustedDNS = %s, want 1.1.1.1", opts.UntrustedDNS)
	}
	if opts.OutputFilePath != "/dev/stdout" {
		t.Errorf("OutputFilePath = %s, want /dev/stdout", opts.OutputFilePath)
	}
}

/* ------------------------------------------------------------------------- */
/* ------------------------------- Init ------------------------------------ */

func TestInitSuccessful(t *testing.T) {
	tmp := t.TempDir()

	// Prepare list & template files.
	listFile := filepath.Join(tmp, "list.txt")
	if err := os.WriteFile(listFile, []byte("8.8.8.8\n"), 0o644); err != nil {
		t.Fatalf("write list: %v", err)
	}
	tplFile := filepath.Join(tmp, "tpl.txt")
	if err := os.WriteFile(tplFile, []byte("dn05jq2u.fr NXDOMAIN\n"), 0o644); err != nil {
		t.Fatalf("write tpl: %v", err)
	}

	restore := backupAndResetFlags(t,
		"dnsanity",
		"-list", listFile,
		"-template", tplFile,
		"-o", "-",
	)
	defer restore()

	conf := Init()

	if got := strings.Join(conf.UntrustedDnsList, ","); got != "8.8.8.8" {
		t.Errorf("UntrustedDnsList = %q, want 8.8.8.8", got)
	}
	if len(conf.Template) != 1 || conf.Template[0].Domain != "dn05jq2u.fr" {
		t.Errorf("Template not loaded properly: %+v", conf.Template)
	}
	if conf.OutputFile != os.Stdout {
		t.Errorf("OutputFile should be Stdout")
	}
}

/* ------------------------------------------------------------------------- */
/* ---- Helper to test branches that exit (ShowHelp / ShowVersion) --------- */

// runInSubprocess lance le binaire de test dans un sous‑processus
// et renvoie le code de sortie.
func runInSubprocess(t *testing.T, fn string, args ...string) int {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestSubprocessHelper")
	cmd.Env = append(os.Environ(), "TEST_SUBPROC="+fn)
	cmd.Args = append(cmd.Args, "--")
	cmd.Args = append(cmd.Args, args...)

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()          // le programme a bien quitté
		}
		t.Fatalf("exec: %v", err)         // erreur de lancement
	}
	return 0 // sortie avec code 0
}

func TestSubprocessHelper(t *testing.T) {
	if fn := os.Getenv("TEST_SUBPROC"); fn != "" {
		// Re‑initialise flags with the payload after "--".
		idx := 0
		for i, a := range os.Args {
			if a == "--" {
				idx = i
				break
			}
		}
		os.Args = append([]string{os.Args[0]}, os.Args[idx+1:]...)
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

		switch fn {
		case "help":
			os.Args = []string{"dnsanity", "-h"}
			ParseOptions() // triggers ShowHelp → os.Exit(0)
		case "version":
			os.Args = []string{"dnsanity", "-version"}
			ParseOptions() // triggers ShowVersion → os.Exit(0)
		case "exitUsage":
			exitUsage("fatal")
		}
	}
}

func TestShowHelpShowVersionExitUsage(t *testing.T) {
	tests := []struct {
		name      string
		fn        string
		wantCode  int
	}{
		{"help",      "help",      0},
		{"version",   "version",   0},
		{"exitUsage", "exitUsage", 1},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := runInSubprocess(t, tc.fn)
			if got != tc.wantCode {
				t.Fatalf("exit code = %d, want %d", got, tc.wantCode)
			}
		})
	}
}

/* ------------------------------------------------------------------------- */
/* --------------------------- Regression guard ---------------------------- */

// Benchmark‑like safety net: any unexpected change in DEFAULT_TEMPLATE length
// will fail the build – a cheap way to detect accidental edits.
func TestDefaultTemplateUnchanged(t *testing.T) {
	const expectedLines = 26 // adjust if template legitimately changes
	lines := strings.Count(DEFAULT_TEMPLATE, "\n")
	if lines != expectedLines {
		t.Fatalf("DEFAULT_TEMPLATE lines = %d, want %d (template changed?)", lines, expectedLines)
	}
}

/* ------------------------------------------------------------------------- */
/* -------------------------- Documentation check -------------------------- */

func TestVersionConstant(t *testing.T) {
	if VERSION == "" {
		t.Fatal("VERSION must not be empty")
	}
}

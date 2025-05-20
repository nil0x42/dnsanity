package config

import (
    "flag"
    "io"
    "os"
    "regexp"
    "strings"
    "testing"
)

// stripANSI removes ANSI escape codes from a string – handy for comparing
// coloured terminal output with plain strings.
func stripANSI(s string) string {
    re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
    return re.ReplaceAllString(s, "")
}

// captureStdout runs fn while capturing everything written to Stdout and
// returns it as a string.
func captureStdout(fn func()) string {
    orig := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    fn()

    w.Close()
    os.Stdout = orig

    out, _ := io.ReadAll(r)
    r.Close()
    return string(out)
}

// resetFlags installs a fresh flag set and custom os.Args so each test starts
// from a clean slate. It returns a restore() callback that must be deferred.
func resetFlags(args []string) (restore func()) {
    oldCmd := flag.CommandLine
    oldArgs := os.Args

    fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
    fs.SetOutput(io.Discard) // silence parsing noise
    flag.CommandLine = fs
    os.Args = args

    return func() {
        flag.CommandLine = oldCmd
        os.Args = oldArgs
    }
}

// ---------------------------------------------------------------------------
// ShowHelp() dynamic coverage test ------------------------------------------
// ---------------------------------------------------------------------------

// TestShowHelpContainsEveryFlag enumerates all flags added by ParseOptions at
// runtime and asserts that ShowHelp() prints each one. This way the test stays
// up‑to‑date even when new CLI options are introduced – no manual list to
// maintain.
func TestShowHelpContainsEveryFlag(t *testing.T) {
    // Step 1: create a fresh FlagSet *without* DNSanity flags.
    restore := resetFlags([]string{"dnsanity"})
    defer restore()

    // Snapshot Go test framework flags so we can ignore them later.
    baseline := map[string]struct{}{}
    flag.CommandLine.VisitAll(func(f *flag.Flag) { baseline[f.Name] = struct{}{} })

    // Step 2: register DNSanity flags by calling ParseOptions.
    if _, err := ParseOptions(); err != nil {
        t.Fatalf("ParseOptions() unexpected error: %v", err)
    }

    // Collect the *new* flag names – i.e. those not present in baseline.
    var dnsanityFlags []string
    flag.CommandLine.VisitAll(func(f *flag.Flag) {
        if _, preExisting := baseline[f.Name]; !preExisting {
            dnsanityFlags = append(dnsanityFlags, f.Name)
        }
    })

    // Sanity check – there *must* be at least one custom flag.
    if len(dnsanityFlags) == 0 {
        t.Fatalf("no DNSanity‑specific flags detected – test invalid")
    }

    // Step 3: capture ShowHelp() output and make sure every flag appears.
    output := captureStdout(func() { ShowHelp() })
    plain := stripANSI(output)

    for _, name := range dnsanityFlags {
        want := "-" + name
        if !strings.Contains(plain, want) {
            t.Errorf("ShowHelp() is missing %q", want)
        }
    }
}

// ---------------------------------------------------------------------------
// ParseOptions() tests -------------------------------------------------------
// ---------------------------------------------------------------------------

type parseTest struct {
    name   string
    args   []string
    expect func(t *testing.T, o *Options)
}

func TestParseOptions(t *testing.T) {
    tests := []parseTest{
        {
            name: "defaults",
            args: []string{"dnsanity"},
            expect: func(t *testing.T, o *Options) {
                if o.GlobRateLimit != 500 {
                    t.Fatalf("default GlobRateLimit = %d, want 500", o.GlobRateLimit)
                }
                if o.Threads != 1000 { // 500 * 2
                    t.Fatalf("Threads autocalc = %d, want 1000", o.Threads)
                }
                if o.OutputFilePath != "/dev/stdout" {
                    t.Fatalf("OutputFilePath = %q, want /dev/stdout", o.OutputFilePath)
                }
            },
        },
        {
            name: "explicit threads kept",
            args: []string{"dnsanity", "-threads", "42"},
            expect: func(t *testing.T, o *Options) {
                if o.Threads != 42 {
                    t.Fatalf("Threads = %d, want 42", o.Threads)
                }
            },
        },
        {
            name: "low ratelimit threads fallback to 100",
            args: []string{"dnsanity", "-global-ratelimit", "30"},
            expect: func(t *testing.T, o *Options) {
                if o.GlobRateLimit != 30 {
                    t.Fatalf("GlobRateLimit = %d, want 30", o.GlobRateLimit)
                }
                if o.Threads != 100 {
                    t.Fatalf("Threads autocalc = %d, want 100", o.Threads)
                }
            },
        },
        {
            name: "high ratelimit threads doubles",
            args: []string{"dnsanity", "-global-ratelimit", "60"},
            expect: func(t *testing.T, o *Options) {
                if o.Threads != 120 {
                    t.Fatalf("Threads autocalc = %d, want 120", o.Threads)
                }
            },
        },
        {
            name: "zero ratelimit coerced to 9999",
            args: []string{"dnsanity", "-global-ratelimit", "0"},
            expect: func(t *testing.T, o *Options) {
                if o.GlobRateLimit != 9999 {
                    t.Fatalf("GlobRateLimit = %d, want 9999", o.GlobRateLimit)
                }
                if o.Threads != 19998 {
                    t.Fatalf("Threads autocalc = %d, want 19998", o.Threads)
                }
            },
        },
        {
            name: "all custom values",
            args: []string{
                "dnsanity",
                "-o", "out.txt",
                "-list", "8.8.8.8",
                "-template", "tpl.txt",
                "-threads", "16",
                "-timeout", "7",
                "-ratelimit", "9",
                "-max-attempts", "3",
                "-max-mismatches", "2",
            },
            expect: func(t *testing.T, o *Options) {
                if o.OutputFilePath != "out.txt" {
                    t.Fatalf("OutputFilePath = %q, want out.txt", o.OutputFilePath)
                }
                if o.UntrustedDNS != "8.8.8.8" {
                    t.Fatalf("UntrustedDNS = %q, want 8.8.8.8", o.UntrustedDNS)
                }
                if o.Template != "tpl.txt" {
                    t.Fatalf("Template = %q, want tpl.txt", o.Template)
                }
                if o.Threads != 16 {
                    t.Fatalf("Threads = %d, want 16", o.Threads)
                }
                if o.Timeout != 7 {
                    t.Fatalf("Timeout = %d, want 7", o.Timeout)
                }
                if o.RateLimit != 9 {
                    t.Fatalf("RateLimit = %d, want 9", o.RateLimit)
                }
                if o.Attempts != 3 {
                    t.Fatalf("Attempts = %d, want 3", o.Attempts)
                }
                if o.MaxMismatches != 2 {
                    t.Fatalf("MaxMismatches = %d, want 2", o.MaxMismatches)
                }
            },
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            restore := resetFlags(tc.args)
            defer restore()

            opts, err := ParseOptions()
            if err != nil {
                t.Fatalf("ParseOptions() error: %v", err)
            }
            tc.expect(t, opts)
        })
    }
}

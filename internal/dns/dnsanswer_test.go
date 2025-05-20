// dnsanswer_test.go
package dns

import (
    "reflect"
    "testing"
)

// TestDNSAnswerData_ToString_StatusOnly verifies stringification when only a status is set.
func TestDNSAnswerData_ToString_StatusOnly(t *testing.T) {
    t.Parallel()
    dad := &DNSAnswerData{Status: "TIMEOUT"}
    if got, want := dad.ToString(), "TIMEOUT"; got != want {
        t.Fatalf("ToString() = %q, want %q", got, want)
    }
}

// TestDNSAnswerData_ToString_WithRecords verifies stringification with multiple A and CNAME records.
func TestDNSAnswerData_ToString_WithRecords(t *testing.T) {
    t.Parallel()
    dad := &DNSAnswerData{
        Status: "NOERROR",
        A:      []string{"1.2.3.4", "4.3.2.1"},
        CNAME:  []string{"example.com", "foo.bar"},
    }
    got := dad.ToString()
    want := "A=1.2.3.4 A=4.3.2.1 CNAME=example.com CNAME=foo.bar"
    if got != want {
        t.Fatalf("ToString() = %q, want %q", got, want)
    }
}

// TestNewDNSAnswerData covers every parsing branch including edge cases and error paths.
func TestNewDNSAnswerData(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        input   string
        want    *DNSAnswerData
        wantErr bool
    }{
        {
            name:    "empty_input",
            input:   "",
            wantErr: true,
        },
        {
            name:  "single_known_status",
            input: "SERVFAIL",
            want:  &DNSAnswerData{Status: "SERVFAIL"},
        },
        {
            name:  "single_A_record",
            input: "A=8.8.8.8",
            want:  &DNSAnswerData{Status: "NOERROR", A: []string{"8.8.8.8"}},
        },
        {
            name:  "single_CNAME_record",
            input: "CNAME=WWW.Example.Org",
            want:  &DNSAnswerData{Status: "NOERROR", CNAME: []string{"www.example.org"}},
        },
        {
            name:  "mixed_records",
            input: "A=1.1.1.1 CNAME=Foo.COM A=9.9.9.9",
            want: &DNSAnswerData{
                Status: "NOERROR",
                A:      []string{"1.1.1.1", "9.9.9.9"},
                CNAME:  []string{"foo.com"},
            },
        },
        {
            name:    "single_invalid_token",
            input:   "TXT=hello",
            wantErr: true,
        },
        {
            name:    "mixed_with_invalid_token",
            input:   "A=1.1.1.1 BADTOKEN CNAME=x",
            wantErr: true,
        },
    }

    for _, tc := range tests {
        tc := tc // capture range variable
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got, err := NewDNSAnswerData(tc.input)

            if tc.wantErr {
                if err == nil {
                    t.Fatalf("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !reflect.DeepEqual(got, tc.want) {
                t.Fatalf("result mismatch\n got  %+v\n want %+v", got, tc.want)
            }
        })
    }
}

// TestDNSAnswer_ToString verifies the outer DNSAnswer stringification including TC flag.
func TestDNSAnswer_ToString(t *testing.T) {
    t.Parallel()

    // NXDOMAIN (no records)
    nx := &DNSAnswer{
        Domain: "example.com.",
        DNSAnswerData: DNSAnswerData{
            Status: "NXDOMAIN",
        },
    }
    if got, want := nx.ToString(), "example.com. NXDOMAIN"; got != want {
        t.Fatalf("ToString() = %q, want %q", got, want)
    }

    // NOERROR with records and truncated flag
    okTrunc := &DNSAnswer{
        Domain: "example.com.",
        DNSAnswerData: DNSAnswerData{
            Status: "NOERROR",
            A:      []string{"4.4.4.4"},
        },
        Truncated: true,
    }
    if got, want := okTrunc.ToString(), "example.com. A=4.4.4.4 [TC=1]"; got != want {
        t.Fatalf("ToString() (truncated) = %q, want %q", got, want)
    }
}

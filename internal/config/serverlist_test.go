package config

import (
    "os"
    "reflect"
    "testing"
)

// helper to create temp file with given content and return path
func createTempFile(t *testing.T, content string) string {
    t.Helper()
    file, err := os.CreateTemp(t.TempDir(), "srvlist-*.txt")
    if err != nil {
        t.Fatalf("unable to create temp file: %v", err)
    }
    if _, err := file.WriteString(content); err != nil {
        file.Close()
        t.Fatalf("unable to write temp file: %v", err)
    }
    if err := file.Close(); err != nil {
        t.Fatalf("unable to close temp file: %v", err)
    }
    return file.Name()
}

func TestParseServerList(t *testing.T) {
    // Build a real file to exercise the “open file” branch
    tempFilePath := createTempFile(t, `
        # comment line
        8.8.8.8 ,  1.1.1.1  # inline comment
        2001:4860:4860::8888
        9.9.9.9,
        ,,,,   # empty elems should be ignored
    `)

    tests := []struct {
        name    string
        input   string
        want    []string
        wantErr bool
    }{
        {
            name:  "inline string simple list",
            input: "8.8.8.8,1.1.1.1",
            want:  []string{"8.8.8.8", "1.1.1.1"},
        },
        {
            name:  "inline multi-line with comments and spaces",
            input: "   8.8.4.4  # comment\n1.0.0.1 , 9.9.9.9  ",
            want:  []string{"8.8.4.4", "1.0.0.1", "9.9.9.9"},
        },
        {
            name:  "inline IPv6 mixed with IPv4",
            input: "2001:4860:4860::8888, 8.8.8.8",
            want:  []string{"2001:4860:4860::8888", "8.8.8.8"},
        },
        {
            name:    "invalid IP in inline list",
            input:   "8.8.8.8,999.999.999.999",
            wantErr: true,
        },
        {
            name:    "empty list",
            input:   "   # just a comment",
            wantErr: true,
        },
        {
            name:    "treat directory path as string, expect error",
            input:   ".",
            wantErr: true,
        },
        {
            name:  "file path input",
            input: tempFilePath,
            want:  []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888", "9.9.9.9"},
        },
        {
            name:  "non‑existent path treated as inline string",
            input: "4.4.4.4",
            want:  []string{"4.4.4.4"},
        },
        {
            name:  "duplicates preserved",
            input: "8.8.8.8,8.8.8.8",
            want:  []string{"8.8.8.8", "8.8.8.8"},
        },
    }

    for _, tc := range tests {
        tc := tc // capture range variable
        t.Run(tc.name, func(t *testing.T) {
            got, err := ParseServerList(tc.input)
            if tc.wantErr {
                if err == nil {
                    t.Fatalf("expected error, got nil with output %v", got)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if !reflect.DeepEqual(got, tc.want) {
                t.Errorf("result mismatch; got %v, want %v", got, tc.want)
            }
        })
    }
}

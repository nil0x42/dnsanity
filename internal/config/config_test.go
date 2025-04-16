package config

import (
	"testing"
	"os"
)

// TestParseServerList checks the behavior of ParseServerList with various inputs.
func TestParseServerList(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "Valid comma-separated IPs",
			input:     "8.8.8.8,1.1.1.1",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "Empty string",
			input:     "",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "Invalid IP",
			input:     "999.999.999.999",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseServerList(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseServerList() error = %v, wantErr = %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && len(got) != tc.wantCount {
				t.Errorf("ParseServerList() got %d IPs, expected %d", len(got), tc.wantCount)
			}
		})
	}
}

// TestInit checks that Config.Init() performs as expected when flags are set or missing.
func TestInit(t *testing.T) {
	// Example: We set some os.Args to simulate command-line usage
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"dnsanity", "-list", "8.8.8.8", "-template", ""}
	conf := Init()
	if len(conf.UntrustedDnsList) != 1 {
		t.Errorf("Expected 1 DNS in untrusted list, got %d", len(conf.UntrustedDnsList))
	}
	if len(conf.Template) == 0 {
		t.Errorf("Expected default template when -template flag is empty")
	}
}

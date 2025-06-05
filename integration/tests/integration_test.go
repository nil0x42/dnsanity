package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationBasic runs dnsanity with a simple server list input and checks
// that the output contains those servers.
func TestIntegrationBasic(t *testing.T) {
	// 1) Create a temporary file with some DNS servers.
	tempDir := t.TempDir()
	testServersPath := filepath.Join(tempDir, "servers.txt")
	content := []byte("8.8.8.8\n1.1.1.1\n")
	if err := os.WriteFile(testServersPath, content, 0644); err != nil {
		t.Fatalf("Cannot write test server file: %v", err)
	}

	// 2) Execute the pre-built dnsanity binary.
	out, code := runCLI(
		t,
		"-list", testServersPath,
		"-o", "/dev/stdout",
		"-trusted-timeout", "4",
		"-trusted-ratelimit", "3",
		"-trusted-max-attempts", "3",
	)
	if code != 0 {
		t.Fatalf("dnsanity exited with code %d\n%s", code, out)
	}

	// 3) Analyze the output
	got := out
	if !strings.Contains(got, "8.8.8.8") {
		t.Errorf("Expected output to contain 8.8.8.8, got:\n%s", got)
	}
	if !strings.Contains(got, "1.1.1.1") {
		t.Errorf("Expected output to contain 1.1.1.1, got:\n%s", got)
	}
}

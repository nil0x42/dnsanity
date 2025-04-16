// Fichier: tests/integration_test.go
package tests

import (
    "testing"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// TestIntegrationBasic builds the main package of dnsanity and runs it with
// a simple server list input, checking output.
func TestIntegrationBasic(t *testing.T) {
    // 1) Build the main package explicitly using the module path or '.'
    // Adjust "github.com/nil0x42/dnsanity" to match your module path if needed.
    cmdBuild := exec.Command("go", "build", "-o", "dnsanity_bin", "github.com/nil0x42/dnsanity")
    cmdBuild.Env = os.Environ()

    // Optionally, ensure we build from the repo's root directory.
    // If your test runs from the root by default, you might not need this:
    // cmdBuild.Dir = "../.."

    out, err := cmdBuild.CombinedOutput()
    if err != nil {
        t.Fatalf("Failed to build dnsanity: %v\nOutput:\n%s", err, string(out))
    }

    // 2) Create a temporary file with some DNS servers
    tempDir := t.TempDir()
    testServersPath := filepath.Join(tempDir, "servers.txt")
    content := []byte("8.8.8.8\n1.1.1.1\n")
    if err := os.WriteFile(testServersPath, content, 0644); err != nil {
        t.Fatalf("Cannot write test server file: %v", err)
    }

    // 3) Run the newly built binary
    // We assume that 'main.go' uses the flags: -list, -o, etc.
    cmdRun := exec.Command("./dnsanity_bin", "-list", testServersPath, "-o", "/dev/stdout")
    cmdRun.Env = os.Environ()
    runOut, runErr := cmdRun.CombinedOutput()
    if runErr != nil {
        t.Fatalf("Failed to run dnsanity: %v\nOutput:\n%s", runErr, string(runOut))
    }

    // 4) Analyze the output
    got := string(runOut)
    if !strings.Contains(got, "8.8.8.8") {
        t.Errorf("Expected output to contain 8.8.8.8, got:\n%s", got)
    }
    if !strings.Contains(got, "1.1.1.1") {
        t.Errorf("Expected output to contain 1.1.1.1, got:\n%s", got)
    }
}

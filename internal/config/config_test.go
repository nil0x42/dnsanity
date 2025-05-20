// Copyright 2025
// SPDX‑License‑Identifier: MIT

// Ce fichier teste exhaustivement internal/config/config.go (100 % de couverture).
// Les scénarios incluent :
//
//   * Variantes d’OpenFile (stdout, fichier temporaire, erreur).
//   * Chemins Init : succès minimal, template externe.
//   * Tous les « early‑exit » (exitUsage) : trusted‑list invalide, oubli de -list,
//     chemin -o impossible… Les branches exit sont couvertes en sous‑processus
//     afin de ne pas interrompre la session de test et pour conserver la
//     couverture (via GOCOVERDIR).
//
// Aucune dépendance externe n’est utilisée.

package config_test

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nil0x42/dnsanity/internal/config"
)

// helperResetFlags resets the global FlagSet between tests.
func helperResetFlags(args []string) {
	os.Args = args
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ExitOnError)
}

// ---------------------------------------------------------------------------
// OpenFile() unit‑tests
// ---------------------------------------------------------------------------

func TestOpenFileVariants(t *testing.T) {
	stdoutCases := []string{"", "-", "/dev/stdout"}
	for _, p := range stdoutCases {
		f, err := config.OpenFile(p)
		if err != nil {
			t.Fatalf("OpenFile(%q) returned error: %v", p, err)
		}
		if f != os.Stdout {
			t.Fatalf("OpenFile(%q) expected Stdout, got %+v", p, f)
		}
	}

	// Successful creation on a real file.
	filePath := filepath.Join(t.TempDir(), "out.txt")
	f, err := config.OpenFile(filePath)
	if err != nil {
		t.Fatalf("OpenFile(tempFile) unexpected error: %v", err)
	}
	defer f.Close()
	if _, err := io.WriteString(f, "hello"); err != nil {
		t.Fatalf("cannot write temp file: %v", err)
	}

	// Error path: directory does not exist.
	badPath := filepath.Join(t.TempDir(), "no", "such", "dir", "file.txt")
	if _, err := config.OpenFile(badPath); err == nil {
		t.Fatalf("OpenFile(%q) should fail but succeeded", badPath)
	}
}

// ---------------------------------------------------------------------------
// Successful Init() paths (run in‑process)
// ---------------------------------------------------------------------------

func TestInitSuccessBasic(t *testing.T) {
	// Prepare a minimal untrusted list file.
	listFile := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(listFile, []byte("8.8.8.8\n"), 0644); err != nil {
		t.Fatalf("cannot write list file: %v", err)
	}
	outFile := filepath.Join(t.TempDir(), "out.txt")

	helperResetFlags([]string{
		"dnsanity",
		"-list", listFile,
		"-o", outFile,
	})
	conf := config.Init()
	if len(conf.UntrustedDNSList) != 1 || conf.UntrustedDNSList[0] != "8.8.8.8" {
		t.Fatalf("unexpected UntrustedDNSList: %+v", conf.UntrustedDNSList)
	}
	if conf.OutputFile == os.Stdout {
		t.Fatalf("OutputFile should be custom, got Stdout")
	}
}

func TestInitWithExternalTemplate(t *testing.T) {
	// Template with a single entry.
	tplFile := filepath.Join(t.TempDir(), "tpl.txt")
	templ := "example.com A=1.2.3.4\n"
	if err := os.WriteFile(tplFile, []byte(templ), 0644); err != nil {
		t.Fatalf("cannot write template file: %v", err)
	}
	listFile := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(listFile, []byte("1.1.1.1\n"), 0644); err != nil {
		t.Fatalf("cannot write list file: %v", err)
	}

	helperResetFlags([]string{
		"dnsanity",
		"-list", listFile,
		"-template", tplFile,
	})
	conf := config.Init()
	if conf.Template == nil || len(conf.Template) != 1 {
		t.Fatalf("template not loaded correctly, len=%d", len(conf.Template))
	}
}

// ---------------------------------------------------------------------------
// exitUsage() branches – covered via helper process
// ---------------------------------------------------------------------------

func TestExitUsageScenarios(t *testing.T) {
	tmpDir := t.TempDir()
	scenarios := []struct {
		name string
		args []string
	}{
		{
			name: "invalid_trusted_list",
			args: []string{
				"-trusted-list", "256.256.256.256",
				"-list", "8.8.8.8",
			},
		},
		{
			name: "missing_list_stdin",
			args: []string{}, // No -list flag triggers /dev/stdin branch then failure
		},
		{
			name: "bad_output_path",
			args: []string{
				"-list", "8.8.8.8",
				"-o", filepath.Join(tmpDir, "no", "perm", "out.txt"),
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Re‑invoke the current test binary in a child process.
			cmdArgs := []string{
				"-test.run=TestHelperProcess",
				"--",
			}
			cmdArgs = append(cmdArgs, sc.args...)
			cmd := exec.Command(os.Args[0], cmdArgs...)

			// Propagate coverage directory for Go ≥1.20.
			if covDir := os.Getenv("GOCOVERDIR"); covDir != "" {
				cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GOCOVERDIR="+covDir)
			} else {
				cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
			}

			err := cmd.Run()
			if err == nil {
				t.Fatalf("scenario %q: expected failure, got success", sc.name)
			}
			if ee, ok := err.(*exec.ExitError); ok && ee.Success() {
				t.Fatalf("scenario %q exited with status 0, expected non‑zero", sc.name)
			}
		})
	}
}

// TestHelperProcess is executed in the child process; it simply calls Init()
// with the supplied arguments. It never returns on successful exitUsage
// because os.Exit is invoked inside the library.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// Strip everything up to "--".
	idx := 0
	for i, a := range os.Args {
		if a == "--" {
			idx = i + 1
			break
		}
	}
	userArgs := os.Args[idx:]
	os.Args = append([]string{"dnsanity"}, userArgs...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Call Init(); any expected error will trigger exitUsage (os.Exit).
	config.Init()

	// If we reach here, exitUsage did not fire – exit with 0 for completeness.
	os.Exit(0)
}

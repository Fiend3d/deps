package main

import (
	"os"
	"regexp"
	"testing"
)

// MSVC images carry a Rich header, so they resolve to a full toolset version.
func TestDescribeToolsetMSVC(t *testing.T) {
	const path = `C:\Windows\System32\kernel32.dll`
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s not available: %v", path, err)
	}

	f, err := parseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if !f.HasRichHdr {
		t.Skip("system binary has no Rich header on this machine")
	}

	got := describeToolset(f)
	if !regexp.MustCompile(`^MSVC \d+\.\d{2}\.\d+$`).MatchString(got) {
		t.Errorf("describeToolset() = %q, want something like \"MSVC 14.38.33145\"", got)
	}
}

// Anything else has no Rich header, and its linker version is reported as such
// rather than being presented as an MSVC toolset. The Go-built test binary is a
// convenient example.
func TestDescribeToolsetNonMSVC(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	f, err := parseFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if f.HasRichHdr {
		t.Skip("the test binary unexpectedly carries a Rich header")
	}

	if _, ok := richLinkerBuild(f); ok {
		t.Error("richLinkerBuild reported a build for an image with no Rich header")
	}

	got := describeToolset(f)
	if !regexp.MustCompile(`^linker \d+\.\d{2}$`).MatchString(got) {
		t.Errorf("describeToolset() = %q, want something like \"linker 3.00\"", got)
	}
}

// The version must survive initModel unmapping the image, like every other
// field copied out of the PE.
func TestModelCarriesToolset(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	m := initModel(exe, nil)
	if m.loadErr != "" {
		t.Fatalf("could not load the test binary: %s", m.loadErr)
	}
	if m.toolset == "" {
		t.Error("model.toolset is empty")
	}
}

// A file that could not be parsed has no toolset to report.
func TestNoToolsetOnLoadFailure(t *testing.T) {
	path := t.TempDir() + `\bogus.dll`
	if err := os.WriteFile(path, []byte("not a PE"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := initModel(path, nil)
	if m.toolset != "" {
		t.Errorf("toolset = %q, want empty for an unparseable file", m.toolset)
	}
}

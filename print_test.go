package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// captureOut redirects printed output into a buffer for the duration of a test,
// with colour forced on so the stripping under test is the only thing acting.
func captureOut(t *testing.T, profile colorprofile.Profile) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	saved := out
	out = &colorprofile.Writer{Forward: &buf, Profile: profile}
	t.Cleanup(func() { out = saved })

	return &buf
}

// A NoTTY profile must remove every escape sequence, not merely the colours.
func TestPlainOutputStripsEscapes(t *testing.T) {
	buf := captureOut(t, colorprofile.NoTTY)

	style := lg.NewStyle()
	fmt.Fprintln(out, style.Foreground(lg.Green).Render("GDI32.dll"))
	fmt.Fprintln(out, style.Foreground(lg.Red).Render("missing.dll"))

	got := buf.String()
	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("plain output still contains escape sequences: %q", got)
	}
	if want := "GDI32.dll\nmissing.dll\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestColouredOutputKeepsEscapes(t *testing.T) {
	buf := captureOut(t, colorprofile.TrueColor)

	fmt.Fprintln(out, lg.NewStyle().Foreground(lg.Green).Render("GDI32.dll"))

	if !strings.ContainsRune(buf.String(), '\x1b') {
		t.Error("a colour-capable profile should keep the escape sequences")
	}
}

// NO_COLOR is honoured explicitly rather than left to the library, so pin the
// behaviour down: set and non-empty disables colour, empty does not.
func TestNoColorEnv(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		set       bool
		wantPlain bool
	}{
		{"set to 1", "1", true, true},
		{"set to any value", "yes", true, true},
		{"set but empty", "", true, false},
		{"unset", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Force colour on, so NO_COLOR is what decides the outcome.
			t.Setenv("CLICOLOR_FORCE", "1")
			t.Setenv("TERM", "xterm-256color")
			if tt.set {
				t.Setenv("NO_COLOR", tt.value)
			} else {
				os.Unsetenv("NO_COLOR")
			}

			got := newOutput().Profile == colorprofile.NoTTY
			if got != tt.wantPlain {
				t.Errorf("plain = %v, want %v", got, tt.wantPlain)
			}
		})
	}
}

func TestPrintTree(t *testing.T) {
	buf := captureOut(t, colorprofile.NoTTY)

	g := newFakeGraph(map[string][]string{
		"a.dll": {"c.dll"},
		"b.dll": {"c.dll"},
		"c.dll": {},
	})
	deps := []dependency{
		{name: "a.dll", path: "a.dll", found: true},
		{name: "b.dll", path: "b.dll", found: true},
	}

	printTree("root.exe", deps, g.resolve, lg.NewStyle())

	want := strings.Join([]string{
		"├─ a.dll",
		"│  └─ c.dll",
		"└─ b.dll",
		"   └─ c.dll (seen)",
		"",
	}, "\n")

	if got := buf.String(); got != want {
		t.Errorf("printTree wrote:\n%s\nwant:\n%s", got, want)
	}
}

// An unreadable module is reported in place and does not stop the walk.
func TestPrintTreeMarksUnreadable(t *testing.T) {
	buf := captureOut(t, colorprofile.NoTTY)

	g := newFakeGraph(map[string][]string{"a.dll": {}, "b.dll": {}})
	g.fail["a.dll"] = true

	printTree("root.exe", []dependency{
		{name: "a.dll", path: "a.dll", found: true},
		{name: "b.dll", path: "b.dll", found: true},
	}, g.resolve, lg.NewStyle())

	got := buf.String()
	if !strings.Contains(got, "a.dll (unreadable)") {
		t.Errorf("expected a.dll to be marked unreadable, got:\n%s", got)
	}
	if !strings.Contains(got, "b.dll") {
		t.Errorf("the walk should have continued past the failure, got:\n%s", got)
	}
}

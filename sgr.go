package main

import (
	"io"
	"os"

	"golang.org/x/term"
)

// ANSI SGR foreground codes. The printed reports only ever use the sixteen
// basic colours, so there is nothing to degrade between profiles: a terminal
// either understands them or gets no colour at all.
const (
	fgRed         = "31"
	fgGreen       = "32"
	fgYellow      = "33"
	fgCyan        = "36"
	fgBrightBlack = "90"
)

// out receives every printed line. Tests swap it for a buffer.
var out io.Writer = os.Stdout

// colourOn decides whether paint emits escape sequences at all. Deciding once,
// here, is why nothing downstream has to care whether it is writing to a
// terminal or a file.
var colourOn = detectColour()

// detectColour reports whether stdout should carry colour, honouring the two
// environment conventions before falling back to what stdout actually is.
func detectColour() bool {
	// https://no-color.org/ — set and non-empty means no colour, whatever the
	// value. It wins over CLICOLOR_FORCE.
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	if v, ok := os.LookupEnv("CLICOLOR_FORCE"); ok && v != "" && v != "0" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// plainOutput drops colour regardless of what stdout looks like.
func plainOutput() {
	colourOn = false
}

// paint wraps s in an SGR sequence, or returns it untouched when colour is off.
func paint(code, s string) string {
	if !colourOn {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

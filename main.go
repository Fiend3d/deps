package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Fiend3d/catatui"
	"github.com/Fiend3d/catatui/term"
)

var (
	// Set by build flags
	Version   = "dev"
	GitCommit = ""
)

func main() {
	version := flag.Bool("v", false, "print version")
	imports := flag.Bool("i", false, "print imports")
	exports := flag.Bool("e", false, "print exports")
	recursive := flag.Bool("r", false, "print recursive dependencies")
	unresolvedOnly := flag.Bool("u", false, "print only unresolved dependencies (recursive)")
	plain := flag.Bool("p", false, "print without color")
	hideDelayed := flag.Bool("d", false, "hide delay-loaded dependencies")

	flag.Parse()

	args := flag.Args()

	if *version {
		fmt.Printf("version:%s (%s)\n", Version, GitCommit)
		return
	}

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: deps [flags] <filepath>\n")
		fmt.Fprintf(os.Stderr, "  -v    print version\n")
		fmt.Fprintf(os.Stderr, "  -i    print imports\n")
		fmt.Fprintf(os.Stderr, "  -e    print exports\n")
		fmt.Fprintf(os.Stderr, "  -r    print recursive dependencies\n")
		fmt.Fprintf(os.Stderr, "  -u    print only unresolved dependencies (recursive)\n")
		fmt.Fprintf(os.Stderr, "  -p    print without color\n")
		fmt.Fprintf(os.Stderr, "  -d    hide delay-loaded dependencies\n")
		os.Exit(1)
	}

	if *plain {
		plainOutput()
	}

	filePath, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("failed to convert (%s) to absolute path: %v", args[0], err)
	}

	// -u is a check rather than a listing: it stands alone and its findings
	// decide the exit status.
	if *unresolvedOnly {
		os.Exit(printUnresolved(filePath, *hideDelayed))
	}

	if *imports || *exports || *recursive {
		printPE(filePath, *imports, *exports, *recursive, *hideDelayed)
		return
	}

	// Fail before the TUI takes over the terminal: once the alternate screen
	// is up, exiting without letting it be restored leaves the shell in raw
	// mode with mouse tracking on.
	m := initModel(filePath, []string{})
	if m.loadErr != "" {
		log.Fatal(m.loadErr)
	}
	m.setHideDelayed(*hideDelayed)

	if err := runTUI(&m); err != nil {
		log.Fatalf("failed to launch the program: %s", err)
	}
}

// modeTitle names the window for the mode on screen.
func modeTitle(md mode) string {
	switch md {
	case exportMode:
		return "Deps - Export"
	case treeMode:
		return "Deps - Tree"
	case unresolvedMode:
		return "Deps - Unresolved"
	default:
		return "Deps - Import"
	}
}

// setWindowTitle names the terminal window. catatui draws cells and has no
// opinion about the window around them, so this is written out directly.
func setWindowTitle(title string) {
	fmt.Fprintf(os.Stdout, "\x1b]2;%s\a", title)
}

// runTUI owns the terminal for as long as the program is on screen: set it up,
// draw, block for an event, and repeat until the model says to stop.
func runTUI(m *model) error {
	// RecoverAndRestore puts the terminal back if anything below panics, so a
	// crash leaves a usable shell and a readable stack trace.
	defer term.RecoverAndRestore()

	terminal, restore, err := term.Init(term.WithMouse())
	if err != nil {
		return err
	}
	defer restore()

	events := term.NewEventReader(os.Stdin, os.Stdout)
	defer events.Close()

	title := ""
	for !m.quit {
		if next := modeTitle(m.mode); next != title {
			title = next
			setWindowTitle(title)
		}

		if err := terminal.Draw(func(f *catatui.Frame) {
			f.RenderWidget(m, f.Area())
		}); err != nil {
			return err
		}

		// Block until something happens: an idle view costs nothing at all.
		ev, ok := <-events.Events()
		if !ok {
			break
		}
		m.handle(ev)

		// Drain whatever else is already queued before drawing again. A wheel
		// spin or a held arrow key produces a burst of events, and collapsing
		// them into one frame is what keeps the list feeling immediate.
		for drained := true; drained; {
			select {
			case ev, ok := <-events.Events():
				if !ok {
					m.quit = true
					drained = false
					break
				}
				m.handle(ev)
			default:
				drained = false
			}
		}
	}

	return events.Err()
}

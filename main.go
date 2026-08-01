package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
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
		os.Exit(printUnresolved(filePath))
	}

	if *imports || *exports || *recursive {
		printPE(filePath, *imports, *exports, *recursive)
		return
	}

	// Fail before the TUI takes over the terminal: once bubbletea owns the
	// screen, exiting without letting it restore leaves the shell in the alt
	// screen with mouse tracking on.
	m := initModel(filePath, []string{})
	if m.loadErr != "" {
		log.Fatal(m.loadErr)
	}

	p := tea.NewProgram(m)

	_, err = p.Run()
	if err != nil {
		log.Fatalf("failed to launch the program: %s", err)
	}
}

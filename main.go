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

	flag.Parse()

	args := flag.Args()

	if *version {
		fmt.Printf("version:%s (%s)\n", Version, GitCommit)
		return
	}

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: deps [flags] <filepath>\n")
		fmt.Fprintf(os.Stderr, "  -v    print version\n")
		os.Exit(1)
	}

	filePath, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("failed to convert (%s) to absolute path: %v", args[0], err)
	}

	p := tea.NewProgram(initModel(filePath, []string{}))

	_, err = p.Run()
	if err != nil {
		log.Fatalf("failed to launch the program: %s", err)
	}
}

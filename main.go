package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/saferwall/pe"
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
		log.Fatalf("failed to convert (%s) to absulte path: %v", args[0], err)
	}

	f, err := pe.New(filePath, &pe.Options{})
	if err != nil {
		log.Fatalf("failed to open PE file: %v", err)
	}

	err = f.Parse()
	if err != nil {
		log.Fatalf("failed to parse PE: %v", err)
	}

	p := tea.NewProgram(initModel(filePath, f))

	_, err = p.Run()
	if err != nil {
		log.Fatalf("failed to launch the program: %s", err)
	}
}

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/ansi"
	"github.com/saferwall/pe"
	"golang.design/x/clipboard"
)

func parseFile(filePath string) *pe.File {
	f, err := pe.New(filePath, &pe.Options{})
	if err != nil {
		log.Fatalf("failed to open PE file: %v", err)
	}
	err = f.Parse()
	if err != nil {
		log.Fatalf("failed to parse PE: %v", err)
	}
	return f
}

func truncate(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

func findDependency(dep, dllPath string) (string, bool) {
	// 1. Check next to the DLL
	dllDir := filepath.Dir(dllPath)
	local := filepath.Join(dllDir, dep)
	if _, err := os.Stat(local); err == nil {
		return local, true
	}

	// 2. Check PATH
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, dep)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}

	return "", false
}

func clipboardWrite(text string) error {
	err := clipboard.Init()
	if err != nil {
		return err
	}

	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}

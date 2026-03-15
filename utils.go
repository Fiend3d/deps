package main

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/ansi"
)

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

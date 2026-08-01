package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/saferwall/pe"
	"golang.design/x/clipboard"
)

// parseFile opens and parses a PE image. The caller owns the returned file and
// must Close it: pe.New memory maps the whole image, and on Windows a live
// mapping keeps a lock on the file.
func parseFile(filePath string) (*pe.File, error) {
	f, err := pe.New(filePath, &pe.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to open PE file: %w", err)
	}
	if err := f.Parse(); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to parse PE: %w", err)
	}
	return f, nil
}

func truncate(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}

// searchDirs lists the directories to probe for a dependency, in the order the
// Windows loader uses. rootPath is the image the walk started from: the loader
// resolves every dependency relative to the process image, not to the DLL that
// happens to name it.
func searchDirs(rootPath string, machine pe.ImageFileHeaderMachineType) []string {
	dirs := []string{filepath.Dir(rootPath)}

	winDir := os.Getenv("SystemRoot")
	if winDir == "" {
		winDir = `C:\Windows`
	}

	// deps is built for amd64, so it sees the real System32 and SysWOW64. A
	// 32-bit image loads its system DLLs from SysWOW64, which is what a 32-bit
	// process sees as System32 through the WOW64 redirector.
	if machine == pe.ImageFileMachineI386 {
		dirs = append(dirs, filepath.Join(winDir, "SysWOW64"))
	} else {
		dirs = append(dirs, filepath.Join(winDir, "System32"))
	}

	dirs = append(dirs, winDir)
	dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)

	return dirs
}

// isAPISet reports whether name is an API set contract rather than a real DLL.
// The loader resolves these through the API set schema, so most have no file of
// that name on disk even though the import is satisfied — reporting them as
// missing would be wrong.
func isAPISet(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "api-ms-") || strings.HasPrefix(lower, "ext-ms-")
}

func findDependency(dep string, dirs []string) (string, bool) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, dep)
		// A directory carrying the name of a DLL is not a match.
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fiend3d/catatui"
	"github.com/saferwall/pe"
	"golang.design/x/clipboard"
)

// parseFile opens and parses a PE image. The caller owns the returned file and
// must Close it: pe.New memory maps the whole image, and on Windows a live
// mapping keeps a lock on the file.
func parseFile(filePath string) (*pe.File, error) {
	return parseWith(filePath, &pe.Options{})
}

// parseImports parses only what a dependency walk reads. Resources, relocations
// and Authenticode signatures are the costly directories and none of them
// matter here — skipping them makes a recursive walk several times faster.
func parseImports(filePath string) (*pe.File, error) {
	return parseWith(filePath, &pe.Options{
		OmitExportDirectory:       true,
		OmitExceptionDirectory:    true,
		OmitResourceDirectory:     true,
		OmitSecurityDirectory:     true,
		OmitRelocDirectory:        true,
		OmitDebugDirectory:        true,
		OmitArchitectureDirectory: true,
		OmitGlobalPtrDirectory:    true,
		OmitTLSDirectory:          true,
		OmitLoadConfigDirectory:   true,
		OmitBoundImportDirectory:  true,
		OmitIATDirectory:          true,
		OmitCLRHeaderDirectory:    true,
	})
}

func parseWith(filePath string, opts *pe.Options) (*pe.File, error) {
	f, err := pe.New(filePath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open PE file: %w", err)
	}
	if err := f.Parse(); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to parse PE: %w", err)
	}
	return f, nil
}

// truncate shortens s to at most width columns, marking the cut with an
// ellipsis. It measures with catatui's own width function, so what it counts is
// exactly what the buffer will draw.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if catatui.StringWidth(s) <= width {
		return s
	}

	// Room has to be left for the ellipsis itself, and a wide cluster is
	// dropped whole rather than split down the middle.
	var b strings.Builder
	used := 0
	for g := range catatui.SegmentGraphemes(s) {
		if used+int(g.Width) > width-1 {
			break
		}
		b.WriteString(g.Symbol)
		used += int(g.Width)
	}
	return b.String() + "…"
}

// searchDirs lists the directories to probe for a dependency, in the order the
// Windows loader uses. rootPath is the image the walk started from: the loader
// resolves every dependency relative to the process image, not to the DLL that
// happens to name it.
//
// systemCount is how many leading entries are the image's own directory and the
// Windows system directories; everything after them came from PATH.
func searchDirs(rootPath string, machine pe.ImageFileHeaderMachineType) (dirs []string, systemCount int) {
	dirs = []string{filepath.Dir(rootPath)}

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
	systemCount = len(dirs)

	dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)

	return dirs, systemCount
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

// clipboardWrite puts text on the clipboard. The write is complete when it
// returns; the context only bounds how long a wedged clipboard backend may take
// to answer, so a stuck copy reports a failure instead of hanging the UI.
func clipboardWrite(text string) error {
	err := clipboard.Init()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := clipboard.Write(ctx, clipboard.FmtText, []byte(text)); err != nil {
		return err
	}
	return nil
}

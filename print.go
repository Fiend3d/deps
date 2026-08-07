package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	lg "charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// out receives every printed line. A NoTTY profile strips escape sequences
// outright, and NewWriter already selects it when stdout is not a terminal, so
// redirecting to a file gives plain text without asking.
var out = newOutput()

func newOutput() *colorprofile.Writer {
	w := colorprofile.NewWriter(os.Stdout, os.Environ())

	// Detect documents that NO_COLOR wins over CLICOLOR_FORCE, but it does not
	// actually apply it here, and the ASCII profile it aims for still emits
	// bare reset sequences. Honour the convention ourselves instead: set and
	// non-empty means no colour, whatever the value.
	// https://no-color.org/
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		w.Profile = colorprofile.NoTTY
	}

	return w
}

// plainOutput drops colour regardless of what stdout looks like.
func plainOutput() {
	out.Profile = colorprofile.NoTTY
}

// depStyle colours a dependency by how it resolved, matching the TUI.
func depStyle(dep dependency, style lg.Style) lg.Style {
	switch {
	case dep.found:
		return style.Foreground(lg.Green)
	case dep.virtual:
		return style.Foreground(lg.Cyan)
	default:
		return style.Foreground(lg.Red)
	}
}

func printPE(filePath string, imports, exports, recursive, hideDelayed bool) {
	f, err := parseFile(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	style := lg.NewStyle()
	fmt.Fprintln(out, filePath+style.Foreground(lg.BrightBlack).Render(" "+describeToolset(f)))

	// -r prints dependencies, so it implies -i.
	if imports || recursive {
		search := newSearchPath(searchDirs(filePath, f.NtHeader.FileHeader.Machine))

		deps := filterDelayed(depsFromFile(f, search), hideDelayed)

		switch {
		case len(deps) == 0:
			fmt.Fprintln(out, style.Foreground(lg.Green).Render("No imports"))
		case recursive:
			printTree(filePath, deps, func(path string) ([]dependency, error) {
				return resolveDeps(path, search)
			}, style, hideDelayed)
		default:
			for _, dep := range deps {
				fmt.Fprintln(out, depStyle(dep, style).Render(dep.label()))
			}
		}
	}

	if exports {
		if !f.HasExport {
			fmt.Fprintln(out, style.Foreground(lg.Green).Render("No exports"))
		} else {
			for _, fn := range f.Export.Functions {
				if fn.Name == "" {
					fmt.Fprintf(out, "(ordinal %d, RVA 0x%08X)\n", fn.Ordinal, fn.FunctionRVA)
				} else {
					fmt.Fprintln(out, style.Foreground(lg.Yellow).Render(fn.Name))
				}
			}
		}
	}
}

// printUnresolved reports every dependency that resolved to nothing, walking to
// full depth. It returns the process exit code: non-zero only when something is
// missing at load time, since a missing delay-load fails only if that code path
// is ever called, and on a healthy Windows install several always are.
func printUnresolved(filePath string, hideDelayed bool) int {
	f, err := parseFile(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	style := lg.NewStyle()
	fmt.Fprintln(out, filePath+style.Foreground(lg.BrightBlack).Render(" "+describeToolset(f)))

	search := newSearchPath(searchDirs(filePath, f.NtHeader.FileHeader.Machine))
	missing := unresolvedDeps(filePath, depsFromFile(f, search), func(path string) ([]dependency, error) {
		return resolveDeps(path, search)
	})

	missing = filterDelayedFindings(missing, hideDelayed)

	if len(missing) == 0 {
		fmt.Fprintln(out, style.Foreground(lg.Green).Render("No unresolved dependencies"))
		return 0
	}

	return writeUnresolvedReport(missing)
}

// filterDelayedFindings drops the findings no importer needs at load time —
// exactly the ones label() marks "(delay)". It filters the findings rather than
// the walk that produced them: hard is decided by seeing both kinds of edge to
// the same module, so dropping delayed imports any earlier would change how the
// survivors are classified.
func filterDelayedFindings(missing []unresolved, hide bool) []unresolved {
	if !hide {
		return missing
	}

	kept := make([]unresolved, 0, len(missing))
	for _, hit := range missing {
		if !hit.hard {
			continue
		}
		kept = append(kept, hit)
	}
	return kept
}

// writeUnresolvedReport renders the findings and returns the exit code: 1 when
// any of them is needed at load time.
func writeUnresolvedReport(missing []unresolved) int {
	style := lg.NewStyle()

	width := labelWidth(missing)

	exit := 0
	for _, hit := range missing {
		colour := lg.Yellow
		if hit.hard {
			// Missing at load time: the image will not start.
			colour = lg.Red
			exit = 1
		}

		fmt.Fprintln(out,
			style.Foreground(colour).Render(fmt.Sprintf("%-*s", width, hit.label()))+
				style.Foreground(lg.BrightBlack).Render("  <- "+hit.importerList()))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, style.Foreground(lg.BrightBlack).Render(
		fmt.Sprintf("%d unresolved", len(missing))))

	return exit
}

// printTree walks the dependency graph depth-first to full depth. Each module is
// expanded the first time it is reached and later occurrences are marked
// "(seen)"; the walk order is fixed, so that set is deterministic.
func printTree(rootPath string, deps []dependency, resolve resolver, style lg.Style, hideDelayed bool) {
	seen := map[string]bool{strings.ToLower(rootPath): true}

	var walk func(deps []dependency, prefix string)
	walk = func(deps []dependency, prefix string) {
		deps = filterDelayed(deps, hideDelayed)
		for i, dep := range deps {
			branch, indent := "├─ ", prefix+"│  "
			if i == len(deps)-1 {
				branch, indent = "└─ ", prefix+"   "
			}

			label := dep.label()
			var children []dependency

			if dep.found {
				key := strings.ToLower(dep.path)
				if seen[key] {
					label += " (seen)"
				} else {
					seen[key] = true
					resolved, err := resolve(dep.path)
					if err != nil {
						label += " (unreadable)"
					} else {
						children = resolved
					}
				}
			}

			fmt.Fprintln(out,
				style.Foreground(lg.BrightBlack).Render(prefix+branch)+
					depStyle(dep, style).Render(label))

			walk(children, indent)
		}
	}

	walk(deps, "")
}

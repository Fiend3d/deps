package main

import (
	"fmt"
	"log"

	lg "charm.land/lipgloss/v2"
)

func printPE(filePath string, imports, exports bool) {
	f, err := parseFile(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	style := lg.NewStyle()
	fmt.Println(filePath + style.Foreground(lg.BrightBlack).Render(" "+describeToolset(f)))

	if imports {
		if !f.HasImport && !f.HasDelayImp {
			fmt.Println(style.Foreground(lg.Green).Render("No imports"))
		} else {
			dirs := searchDirs(filePath, f.NtHeader.FileHeader.Machine)
			printImport := func(name string, delayed bool) {
				_, found := findDependency(name, dirs)
				line := name
				if delayed {
					line += " (delay)"
				}
				switch {
				case found:
					fmt.Println(style.Foreground(lg.Green).Render(line))
				case isAPISet(name):
					fmt.Println(style.Foreground(lg.Cyan).Render(line + " (api-set)"))
				default:
					fmt.Println(style.Foreground(lg.Red).Render(line))
				}
			}
			for _, imp := range f.Imports {
				printImport(imp.Name, false)
			}
			for _, imp := range f.DelayImports {
				printImport(imp.Name, true)
			}
		}
	}

	if exports {
		if !f.HasExport {
			fmt.Println(style.Foreground(lg.Green).Render("No exports"))
		} else {
			for _, fn := range f.Export.Functions {
				if fn.Name == "" {
					fmt.Printf("(ordinal %d, RVA 0x%08X)\n", fn.Ordinal, fn.FunctionRVA)
				} else {
					fmt.Println(style.Foreground(lg.Yellow).Render(fn.Name))
				}
			}
		}
	}
}

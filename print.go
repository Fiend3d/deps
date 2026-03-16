package main

import (
	"fmt"

	lg "charm.land/lipgloss/v2"
)

func printPE(filePath string, imports, exports bool) {
	f := parseFile(filePath)

	style := lg.NewStyle()
	fmt.Println(filePath)

	if imports {
		if !f.HasImport {
			fmt.Println(style.Foreground(lg.Green).Render("No imports"))
		} else {
			for _, imp := range f.Imports {
				_, found := findDependency(imp.Name, filePath)
				if found {
					fmt.Println(style.Foreground(lg.Green).Render(imp.Name))
				} else {
					fmt.Println(style.Foreground(lg.Red).Render(imp.Name))
				}
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

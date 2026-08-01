package main

import (
	"fmt"
	"strings"

	"github.com/saferwall/pe"
)

// linkerVersion reads the linker version out of whichever optional header
// shape the image uses.
func linkerVersion(f *pe.File) (major, minor uint8) {
	switch oh := f.NtHeader.OptionalHeader.(type) {
	case pe.ImageOptionalHeader64:
		return oh.MajorLinkerVersion, oh.MinorLinkerVersion
	case pe.ImageOptionalHeader32:
		return oh.MajorLinkerVersion, oh.MinorLinkerVersion
	}
	return 0, 0
}

// richLinkerBuild returns the compiler build number recorded by the linker
// entry of the Rich header, which MSVC writes and other toolchains do not.
func richLinkerBuild(f *pe.File) (uint16, bool) {
	if !f.HasRichHdr {
		return 0, false
	}

	var build uint16
	var found bool
	for _, id := range f.RichHeader.CompIDs {
		if strings.HasPrefix(pe.ProdIDtoStr(id.ProdID), "Linker") {
			if !found || id.MinorCV > build {
				build, found = id.MinorCV, true
			}
		}
	}
	return build, found
}

// describeToolset names the toolchain that produced the image.
//
// MSVC records the exact build in the Rich header's linker entry; combined with
// the optional header's linker version that gives the full toolset, e.g.
// "MSVC 14.38.33145". Every other toolchain writes no Rich header, and its
// linker version means something of its own (Go reports 3.00), so there the
// raw field is reported as what it is rather than dressed up as a toolset.
func describeToolset(f *pe.File) string {
	major, minor := linkerVersion(f)

	if build, ok := richLinkerBuild(f); ok {
		return fmt.Sprintf("MSVC %d.%02d.%d", major, minor, build)
	}
	return fmt.Sprintf("linker %d.%02d", major, minor)
}

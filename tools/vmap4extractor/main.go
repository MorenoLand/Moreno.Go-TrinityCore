// MorenoCore VMAP4 Model Extractor: extracts raw WMO and M2 model geometry from MPQs.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

func printBanner() {
	fmt.Println("==========================================================")
	fmt.Println(" MorenoCore VMAP4 Raw Model Extractor (Go Parity Version)")
	fmt.Println(" Extracts WMO & M2 models from client MPQ archives")
	fmt.Println("==========================================================")
}

func main() {
	input := flag.String("input", "", "WoW client installation root or directory containing MPQ archives (e.g. Data/)")
	dataDir := flag.String("d", "", "Path to the vector data source folder (C++ -d)")
	output := flag.String("output", "Buildings", "Output directory for extracted raw building models")
	verbose := flag.Bool("l", false, "Enable verbose logging")
	small := flag.Bool("s", false, "Small size (data size optimization), the default")
	precise := flag.Bool("precise", false, "Large size: keep all WMO triangles/vertices (C++ -l)")
	help := flag.Bool("?", false, "Show this message")
	flag.Parse()

	// C++ owner: vmapexport.cpp processArgv — -s is the default small
	// mode, -l selects large/precise vector data, -d sets the input
	// path, -? prints usage. The Go tool historically used -l for
	// verbose logging, so -l keeps that meaning here and -precise is
	// the large-mode switch; -s is accepted as a no-op for CLI parity.
	_ = small
	if *help {
		fmt.Println("Extract V4.00 2012_02.")
		fmt.Printf("%s [-?][-s][-l][-precise][-d <path>][-input <path>][-output <dir>]\n", os.Args[0])
		fmt.Println("   -s : (default) small size (data size optimization), ~500MB less vmap data.")
		fmt.Println("   -l : verbose logging (Go extension; C++ -l means large/precise, use -precise).")
		fmt.Println("   -precise : large size, ~500MB more vmap data. (might contain more details)")
		fmt.Println("   -d <path>: Path to the vector data source folder.")
		os.Exit(0)
	}

	if *input == "" {
		*input = *dataDir
	}
	if *input == "" {
		if len(flag.Args()) > 0 {
			*input = flag.Args()[0]
		} else {
			for _, candidate := range []string{"Data", ".", "bin/Data"} {
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					*input = candidate
					break
				}
			}
		}
	}

	if *input == "" {
		fmt.Fprintln(os.Stderr, "Error: -input flag or client Data directory is required.")
		fmt.Fprintln(os.Stderr, "Usage: vmap4extractor -input <path to WoW client or Data/> [-output Buildings] [-l]")
		os.Exit(1)
	}

	// C++ owner: vmapexport.cpp main — refuse to run when the output
	// directory looks polluted (a previous dir or dir_bin is present).
	for _, stale := range []string{"dir", "dir_bin"} {
		if _, err := os.Stat(filepath.Join(*output, stale)); err == nil {
			fmt.Fprintln(os.Stderr, "Your output directory seems to be polluted, please use an empty directory!")
			os.Exit(1)
		}
	}

	// vmapexport.cpp fillArchiveNameVector open order, reversed into lookup
	// precedence (highest first) for the first-hit-wins extraction loop.
	archives, err := mpq.OrderedVmapArchives(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to locate MPQ archives in %s: %v\n", *input, err)
		os.Exit(1)
	}
	if len(archives) == 0 {
		fmt.Fprintf(os.Stderr, "No MPQ archives found under %s\n", *input)
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory %s: %v\n", *output, err)
		os.Exit(1)
	}

	// C++ owner: vmapexport.cpp main — archives stay open for the whole
	// run (gOpenArchives); MPQFile resolves each name against them in
	// precedence order. The extractor keeps them open and indexes their
	// listfiles for first-hit-wins reads.
	ext := newExtractor(*output, *verbose, *precise)
	opened := 0
	for _, archivePath := range archives {
		if err := ext.addArchive(archivePath); err != nil {
			if *verbose {
				fmt.Printf("Skipping archive %s: %v\n", filepath.Base(archivePath), err)
			}
			continue
		}
		opened++
	}
	if opened == 0 {
		fmt.Fprintf(os.Stderr, "FATAL ERROR: None MPQ archive found by path '%s'. Use -d option with proper path.\n", *input)
		os.Exit(1)
	}
	defer ext.close()

	start := time.Now()
	fmt.Printf("Scanning %d MPQ archives in %s...\n", opened, *input)

	// Global model scan (Go extension of the C++ flow — C++ extracts only
	// models referenced by ADTs/WDTs/GameObjectDisplayInfo.dbc, but the
	// scan pre-populates Buildings so the dir_bin pass below finds every
	// referenced model on disk). .wdt files are NOT extracted: C++ parses
	// them in ParsMapFiles and never writes them out.
	extracted := 0
	seen := make(map[string]struct{})
	for slot, arc := range ext.archives {
		files := ext.archiveFiles[slot]
		_ = arc
		for _, name := range files {
			extName := strings.ToLower(filepath.Ext(name))
			if extName != ".wmo" && extName != ".m2" && extName != ".mdx" {
				continue
			}
			if extName == ".wmo" && isWMOGroupName(name) {
				continue
			}
			normalized := strings.ToLower(filepath.ToSlash(name))
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}

			var ok bool
			if extName == ".wmo" {
				ok = ext.extractSingleWmo(name)
			} else {
				ok = ext.extractSingleModel(name)
			}
			if !ok {
				if *verbose {
					fmt.Printf("Skipping %s\n", name)
				}
				continue
			}
			extracted++
			if *verbose && extracted%100 == 0 {
				fmt.Printf("Extracted %d model files...\n", extracted)
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("Extraction complete: %d models written to %s in %v\n", extracted, *output, elapsed.Round(time.Millisecond))

	// C++ owner: vmapexport.cpp main — Map.dbc drives ParsMapFiles
	// (WDT/ADT dir_bin pass), then ExtractGameobjectModels writes the
	// temp_gameobject_models index. A missing Map.dbc is fatal.
	if err := ext.openDirBin(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	maps, err := ext.loadMapDBC()
	if err != nil {
		ext.closeDirBin()
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	ext.parseMapFiles(maps)
	ext.closeDirBin()

	if err := ext.extractGameobjectModels(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Println("Extract V4.00 2012_02. Work complete. No errors.")
}

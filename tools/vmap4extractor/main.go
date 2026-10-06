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
	output := flag.String("output", "Buildings", "Output directory for extracted raw building models")
	verbose := flag.Bool("l", false, "Enable verbose logging")
	flag.Parse()

	printBanner()

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

	start := time.Now()
	fmt.Printf("Scanning %d MPQ archives in %s...\n", len(archives), *input)

	extracted := 0
	seen := make(map[string]struct{})

	for _, archivePath := range archives {
		arc, err := mpq.Open(archivePath)
		if err != nil {
			if *verbose {
				fmt.Printf("Skipping archive %s: %v\n", filepath.Base(archivePath), err)
			}
			continue
		}

		files, err := arc.ListFiles()
		if err != nil {
			arc.Close()
			continue
		}

		for _, name := range files {
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".wmo" && ext != ".m2" && ext != ".mdx" && ext != ".wdt" {
				continue
			}
			if ext == ".wmo" && isWMOGroupName(name) {
				continue
			}

			normalized := strings.ToLower(filepath.ToSlash(name))
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}

			var data []byte
			if ext == ".wmo" {
				data, err = extractWMO(arc, name, files)
			} else {
				data, err = arc.ReadFile(name)
				if err == nil && (ext == ".m2" || ext == ".mdx") {
					data, err = extractM2(data)
				}
			}
			if err != nil {
				if *verbose {
					fmt.Printf("Skipping %s: %v\n", name, err)
				}
				continue
			}

			destFile := filepath.Join(*output, filepath.Base(name))
			if err := os.WriteFile(destFile, data, 0o644); err != nil {
				continue
			}
			extracted++
			if *verbose && extracted%100 == 0 {
				fmt.Printf("Extracted %d model files...\n", extracted)
			}
		}
		arc.Close()
	}

	elapsed := time.Since(start)
	fmt.Printf("Extraction complete: %d models written to %s in %v\n", extracted, *output, elapsed.Round(time.Millisecond))
}

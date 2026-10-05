// MorenoCore Map & DBC Extractor: extracts client database and map data from MPQs.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/wowdata"
)

func main() {
	input := flag.String("input", "", "WoW client installation root or directory containing MPQ archives")
	output := flag.String("output", "data", "output data directory")
	wdtFile := flag.String("wdt-file", "", "parse one local WDT file and print its active tile count")
	adtFile := flag.String("adt-file", "", "parse one local ADT file and print its chunk inventory")
	dirBin := flag.String("dir-bin", "", "write VMAP dir_bin records from the local ADT file")
	modelDir := flag.String("model-dir", "", "directory containing VMAP raw WMO/M2 models for WMO doodad expansion")
	mapID := flag.Uint("map-id", 0, "map ID written to dir_bin records")
	tileX := flag.Uint("tile-x", 0, "ADT tile X coordinate written to dir_bin records")
	tileY := flag.Uint("tile-y", 0, "ADT tile Y coordinate written to dir_bin records")
	flag.Parse()
	if *dirBin != "" && *adtFile == "" {
		if *wdtFile == "" {
			fmt.Fprintln(os.Stderr, "-dir-bin requires -adt-file or -wdt-file")
			os.Exit(2)
		}
	}
	if *dirBin != "" && *adtFile != "" && *wdtFile != "" {
		fmt.Fprintln(os.Stderr, "-dir-bin accepts one source file at a time")
		os.Exit(2)
	}
	if *adtFile != "" {
		data, err := readClientAsset(*input, *adtFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ADT read failed: %v\n", err)
			os.Exit(1)
		}
		info, err := parseADT(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ADT parse failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Parsed %s: mcnk=%d mh2o=%d liquid_layers=%d mcvt=%d mcly=%d mcal=%d mddf=%d modf=%d\n", filepath.Base(*adtFile), info.MCNKCount, info.MH2OCount, info.LiquidLayers, info.MCVTCount, info.MCLYCount, info.MCALCount, len(info.Doodads), len(info.WorldModels))
		if *dirBin != "" {
			if uint64(*mapID) > uint64(^uint32(0)) || *tileX > 65 || *tileY > 65 || (*tileX == 64 || *tileY == 64) {
				fmt.Fprintln(os.Stderr, "dir_bin map/tile coordinates are invalid")
				os.Exit(2)
			}
			payload, err := buildADTDirBinWithModelDir(info, uint32(*mapID), uint32(*tileX), uint32(*tileY), *modelDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dir_bin conversion failed: %v\n", err)
				os.Exit(1)
			}
			if err := writeDirBin(*dirBin, payload); err != nil {
				fmt.Fprintf(os.Stderr, "dir_bin write failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Wrote %d dir_bin bytes to %s\n", len(payload), *dirBin)
		}
		if *input == "" && *wdtFile == "" {
			return
		}
	}
	if *wdtFile != "" {
		data, err := readClientAsset(*input, *wdtFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WDT read failed: %v\n", err)
			os.Exit(1)
		}
		info, err := parseWDT(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WDT parse failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Parsed %s: version=%d active_tiles=%d global_wmo=%t name=%s\n", filepath.Base(*wdtFile), info.Version, info.TileCount, info.HasGlobalWMO, info.GlobalWMO)
		if *dirBin != "" {
			if uint64(*mapID) > uint64(^uint32(0)) {
				fmt.Fprintln(os.Stderr, "dir_bin map ID is invalid")
				os.Exit(2)
			}
			payload, err := buildWDTDirBinWithModelDir(info, uint32(*mapID), *modelDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dir_bin conversion failed: %v\n", err)
				os.Exit(1)
			}
			if err := writeDirBin(*dirBin, payload); err != nil {
				fmt.Fprintf(os.Stderr, "dir_bin write failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Wrote %d dir_bin bytes to %s\n", len(payload), *dirBin)
		}
		if *input == "" {
			return
		}
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(2)
	}
	count, err := wowdata.ExtractDBC(*input, *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DBC extraction failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Extracted and validated %d DBC files into %s\n", count, *output)
}

func readClientAsset(input, name string) ([]byte, error) {
	data, localErr := os.ReadFile(name)
	if localErr == nil {
		return data, nil
	}
	if input == "" {
		return nil, localErr
	}
	archives, err := mpq.OrderedArchives(input)
	if err != nil {
		return nil, err
	}
	var lastErr error = localErr
	for _, path := range archives {
		archive, openErr := mpq.Open(path)
		if openErr != nil {
			lastErr = openErr
			continue
		}
		data, readErr := archive.ReadFile(name)
		closeErr := archive.Close()
		if readErr == nil {
			if closeErr != nil {
				return nil, closeErr
			}
			return data, nil
		}
		lastErr = readErr
	}
	return nil, fmt.Errorf("client asset %q was not found in %q: %w", name, input, lastErr)
}

func writeDirBin(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

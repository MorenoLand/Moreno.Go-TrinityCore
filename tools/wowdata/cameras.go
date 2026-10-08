package wowdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/dbc"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

// C++ owner: src/tools/map_extractor/System.cpp ExtractCameraFiles
// (System.cpp:1032-1080) + main() camera phase (System.cpp:1165-1177).
//   - Runs once for the FIRST detected locale (basicLocale=true): the
//     destination is the flat <output>/Cameras/, with no per-locale
//     subdirectory.
//   - Archive set is the locale MPQ set plus the common MPQs
//     (LoadLocaleMPQFiles + LoadCommonMPQFiles); mpq.OrderedArchives emits
//     them highest-precedence first, matching the C++ push_front lookup
//     order, and the first archive holding a file wins.
//   - The model list comes from DBFilesClient\CinematicCamera.dbc record
//     field 1; the first ".mdx" occurrence is replaced with ".m2".
//   - The output name strips the leading "Cameras\" prefix; existing files
//     are skipped, never overwritten.
//   - A missing model still creates the (empty) output file and counts it:
//     C++ ExtractFile writes zero bytes when MPQFile isEof but returns true.
//   - A missing CinematicCamera.dbc aborts the phase with the C++ message;
//     it is not a fatal error.
//   - DELTA: the C++ -e extract-select flag (MAP=1/DBC=2/Camera=4) has no Go
//     counterpart; the Go tool always extracts DBCs and now cameras, like
//     its existing always-extract-DBC behavior.
func ExtractCameras(input, output string) (int, error) {
	locales := mpq.DetectLocales(input)
	if len(locales) == 0 {
		return 0, nil
	}
	archives, err := mpq.OrderedArchives(input)
	if err != nil {
		return 0, err
	}
	open := make([]*mpq.Archive, 0, len(archives))
	defer func() {
		for _, a := range open {
			a.Close()
		}
	}()
	for _, path := range archives {
		a, err := mpq.Open(path)
		if err != nil {
			continue
		}
		open = append(open, a)
	}
	readFirst := func(name string) ([]byte, bool) {
		for _, a := range open {
			if data, err := a.ReadFile(name); err == nil {
				return data, true
			}
		}
		return nil, false
	}
	dbcData, ok := readFirst(`DBFilesClient\CinematicCamera.dbc`)
	if !ok {
		fmt.Println("Unable to open CinematicCamera.dbc. Camera extract aborted.")
		return 0, nil
	}
	db, err := dbc.Parse(dbcData)
	if err != nil {
		return 0, fmt.Errorf("invalid CinematicCamera.dbc: %w", err)
	}
	dest := filepath.Join(output, "Cameras")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	count := 0
	for i := 0; i < db.Records(); i++ {
		rec, err := db.Record(i)
		if err != nil {
			continue
		}
		model, err := rec.String(1)
		if err != nil || model == "" {
			continue
		}
		model = strings.Replace(model, ".mdx", ".m2", 1)
		rel := model
		if upper := strings.ToUpper(rel); strings.HasPrefix(upper, `CAMERAS\`) {
			rel = rel[len(`Cameras\`):]
		} else if strings.HasPrefix(upper, "CAMERAS/") {
			rel = rel[len("Cameras/"):]
		}
		target := filepath.Join(dest, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return count, err
		}
		data, _ := readFirst(model)
		if err := os.WriteFile(target, data, 0o644); err != nil {
			fmt.Printf("Can't create the output file '%s'\n", target)
			continue
		}
		count++
	}
	return count, nil
}

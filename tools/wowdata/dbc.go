package wowdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

// C++ owner: src/tools/map_extractor/System.cpp ExtractDBCFiles +
// main() locale loop.
//   - DBC extraction opens ONLY the locale MPQ set per locale
//     (LoadLocaleMPQFiles: locale base + locale patches); common MPQs are
//     never consulted for DBCs.
//   - The .dbc name set is collected across the open archives, then each
//     name is read via MPQFile (highest-precedence archive wins).
//   - Each DBC is extracted by RAW BYTE COPY, exactly like C++ ExtractFile
//     (System.cpp:966-980): content is never parsed or validated, so a
//     malformed DBC is still written and counted.
//   - First detected locale -> <output>/dbc/ (flat); further locales ->
//     <output>/dbc/<lang>/.
//   - Already-extracted files are SKIPPED (exists check), never overwritten.
//   - component.wow-<lang>.txt (build info): the FIRST locale goes through
//     C++ ReadBuild (System.cpp:170-196) — missing file or a version="..."
//     that does not parse to build > 0 is fatal (exit 1) before any DBC is
//     extracted. Later locales use the ExtractDBCFiles arm
//     (System.cpp:1008-1014), which always writes (truncates) the dest,
//     empty when no archive contains it — mirroring C++ ExtractFile's
//     unconditional fopen("wb").
func ExtractDBC(input, output string) (int, error) {
	locales := mpq.DetectLocales(input)
	if len(locales) == 0 {
		return extractDBCWalkAll(input, output)
	}
	total := 0
	for i, lang := range locales {
		destination := filepath.Join(output, "dbc")
		if i > 0 {
			destination = filepath.Join(destination, lang)
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return total, err
		}
		archives := mpq.LocaleArchives(input, lang)
		if err := extractBuildInfo(archives, destination, lang, i == 0); err != nil {
			return total, err
		}
		count, err := extractDBCFromArchives(archives, destination)
		total += count
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// extractDBCWalkAll is the fallback when input is not a client root with a
// detectable locale (bare MPQ directory or single archive): the old
// lexical-walk behavior, but with the C++ skip-if-exists arm — previously
// later archives silently overwrote earlier DBCs.
func extractDBCWalkAll(input, output string) (int, error) {
	archives, err := mpq.Archives(input)
	if err != nil {
		return 0, err
	}
	if len(archives) == 0 {
		return 0, fmt.Errorf("no MPQ archives found under %s", input)
	}
	destination := filepath.Join(output, "dbc")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return 0, err
	}
	return extractDBCFromArchives(archives, destination)
}

// extractBuildInfo mirrors the component.wow-<lang>.txt handling across the
// two C++ sites (System.cpp): ReadBuild (System.cpp:170-196) runs for the
// FIRST locale only and is FATAL — a missing file or a version="..." that
// does not parse to build > 0 prints "Fatal error" and exits(1) before any
// DBC is extracted; the ExtractDBCFiles arm (System.cpp:1008-1014) that runs
// for every locale always writes (truncates) the dest, empty when no archive
// contains it. strict=true (first locale) mirrors ReadBuild; strict=false
// (later locales) mirrors the ExtractDBCFiles arm. The name resolves through
// the locale archive set in lookup-precedence order (highest first), exactly
// like MPQFile.
func extractBuildInfo(archives []string, destination, lang string, strict bool) error {
	name := "component.wow-" + lang + ".txt"
	var data []byte
	found := false
	for _, path := range archives {
		archive, err := mpq.Open(path)
		if err != nil {
			continue
		}
		file, readErr := archive.ReadFile(name)
		archive.Close()
		if readErr == nil {
			data = file
			found = true
			break
		}
	}
	if strict {
		if !found {
			return fmt.Errorf("Fatal error: Not found %s file!", name)
		}
		if _, err := parseBuildNumber(data, lang); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(destination, name), data, 0o644)
}

// parseBuildNumber mirrors C++ ReadBuild's version="NNNN" parse
// (System.cpp:183-196): the marker must be present, a closing quote must
// follow it (an empty value fails the pos1 >= pos2 gate), and the number
// must parse to build > 0 — anything else is fatal. C++ atoi skips leading
// whitespace; TrimSpace keeps the Go parse aligned on real files.
func parseBuildNumber(data []byte, lang string) (uint32, error) {
	invalid := fmt.Errorf("Fatal error: Invalid component.wow-%s.txt file format!", lang)
	text := string(data)
	const marker = `version="`
	pos := strings.Index(text, marker)
	if pos < 0 {
		return 0, invalid
	}
	rest := text[pos+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return 0, invalid
	}
	build, err := strconv.Atoi(strings.TrimSpace(rest[:end]))
	if err != nil || build <= 0 {
		return 0, invalid
	}
	return uint32(build), nil
}

// extractDBCFromArchives reads every .dbc from archives, which must already
// be in lookup-precedence order (highest first): the first archive
// containing a name wins, mirroring C++ MPQFile iteration over
// gOpenArchives. Files already present in destination are skipped,
// mirroring the C++ exists() continue arm. The seen-set dedups names across
// archives exactly like C++'s dbcfiles set.
func extractDBCFromArchives(archives []string, destination string) (int, error) {
	if len(archives) == 0 {
		return 0, fmt.Errorf("no MPQ archives to extract DBCs from")
	}
	seen := make(map[string]struct{})
	written := 0
	for _, path := range archives {
		archive, err := mpq.Open(path)
		if err != nil {
			return written, err
		}
		files, listErr := archive.ListFiles()
		if listErr != nil {
			archive.Close()
			continue
		}
		for _, name := range files {
			if !strings.EqualFold(filepath.Ext(name), ".dbc") {
				continue
			}
			key := strings.ToLower(filepath.ToSlash(name))
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			base := filepath.Base(filepath.FromSlash(name))
			dest := filepath.Join(destination, base)
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			data, err := archive.ReadFile(name)
			if err != nil {
				archive.Close()
				return written, fmt.Errorf("read %s from %s: %w", name, path, err)
			}
			// C++ ExtractFile (System.cpp:966-980) is a raw byte copy: it
			// never parses or validates DBC content, so a malformed DBC is
			// still written and counted. No dbc.Parse gate here.
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				archive.Close()
				return written, err
			}
			written++
		}
		if err := archive.Close(); err != nil {
			return written, err
		}
	}
	return written, nil
}

package wowdata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/dbc"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

// C++ owner: src/tools/map_extractor/System.cpp ExtractDBCFiles +
// main() locale loop.
//   - DBC extraction opens ONLY the locale MPQ set per locale
//     (LoadLocaleMPQFiles: locale base + locale patches); common MPQs are
//     never consulted for DBCs.
//   - The .dbc name set is collected across the open archives, then each
//     name is read via MPQFile (highest-precedence archive wins).
//   - First detected locale -> <output>/dbc/ (flat); further locales ->
//     <output>/dbc/<lang>/.
//   - Already-extracted files are SKIPPED (exists check), never overwritten.
//   - component.wow-<lang>.txt (build info) is also extracted; Go has no
//     client-build detection in the extractor, so that arm is a documented
//     no-bridge delta.
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
		count, err := extractDBCFromArchives(mpq.LocaleArchives(input, lang), destination)
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
			if _, err := dbc.Parse(data); err != nil {
				archive.Close()
				return written, fmt.Errorf("invalid DBC %s: %w", name, err)
			}
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

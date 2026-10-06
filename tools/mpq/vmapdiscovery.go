package mpq

import (
	"os"
	"path/filepath"
	"strconv"
)

// C++ owner: src/tools/vmap4_extractor/vmapexport.cpp
//   - fillArchiveNameVector (:258), scan_patches (:228), getGamePath.
//   - MPQArchive's ctor does gOpenArchives.push_front(this) (mpq_libmpq.cpp:52)
//     and MPQFile lookups iterate from begin(), so the LAST opened archive has
//     the HIGHEST lookup precedence.
//
// C++ open order (fillArchiveNameVector), per detected locale (langs[] probe
// order, Data/<lang> directory must exist — a stat dir check, not a file
// check like the map_extractor's DetectLocales):
//  1. Data/<lang>/locale-<lang>.MPQ, expansion-locale-<lang>.MPQ,
//     lichking-locale-<lang>.MPQ
//  2. Data/common.MPQ, common-2.MPQ, expansion.MPQ, lichking.MPQ
//  3. scan_patches("Data/patch"): patch.MPQ, patch-2.MPQ .. patch-99.MPQ,
//     existing files only (fopen check), in that order
//  4. scan_patches("Data/<lang>/patch-<lang>") per locale: patch-<lang>.MPQ,
//     patch-<lang>-2.MPQ .. -99.MPQ, existing only
//
// Fixed names (steps 1-2) are pushed unconditionally — a missing file fails
// the MPQArchive ctor and the archive is dropped. Lookup precedence (highest
// first) is therefore the reverse of the open order. The Go vmap4extractor
// is first-hit-wins (its `seen` map skips later archives), so
// OrderedVmapArchives returns the list in lookup-precedence order while the
// C++-exact open order is built internally and reversed.

// vmapLocaleArchives appends the three fixed locale MPQs for one locale in
// C++ open order. Like C++, they are pushed unconditionally; a file that
// does not exist is filtered at open time by the caller (Go mirrors the
// MPQArchive ctor check by skipping mpq.Open failures).
func vmapLocaleArchives(dataDir, lang string, out []string) []string {
	out = append(out,
		filepath.Join(dataDir, lang, "locale-"+lang+".MPQ"),
		filepath.Join(dataDir, lang, "expansion-locale-"+lang+".MPQ"),
		filepath.Join(dataDir, lang, "lichking-locale-"+lang+".MPQ"),
	)
	return out
}

// scanPatches mirrors C++ scan_patches: prefix.MPQ, prefix-2.MPQ ..
// prefix-99.MPQ, existing files only, in that order.
func scanPatches(prefix string, out []string) []string {
	for i := 1; i <= 99; i++ {
		var candidate string
		if i == 1 {
			candidate = prefix + ".MPQ"
		} else {
			candidate = prefix + "-" + strconv.Itoa(i) + ".MPQ"
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			out = append(out, candidate)
		}
	}
	return out
}

// detectVmapLocales mirrors the fillArchiveNameVector locale probe: the
// fixed langs[] order, and a locale counts only when Data/<lang> exists as
// a directory (C++ stat + S_IFDIR check).
func detectVmapLocales(dataDir string) []string {
	var locales []string
	for _, lang := range clientLocales {
		if info, err := os.Stat(filepath.Join(dataDir, lang)); err == nil && info.IsDir() {
			locales = append(locales, lang)
		}
	}
	return locales
}

// OrderedVmapArchives returns MPQ paths for the vmap4extractor in C++
// vmapexport.cpp lookup precedence order (highest first), i.e. the reverse
// of the fillArchiveNameVector open order. When input names a client root
// (has a Data/ subdirectory) discovery follows vmapexport.cpp exactly;
// when input has no Data/ subdirectory it is treated as the data directory
// itself (the C++ -d flag points at Data/). Fixed-name archives are pushed
// unconditionally per C++ and skipped by the caller's open failure, exactly
// like the MPQArchive ctor drop. When no locale directory, fixed archive,
// or patch file is found at all, it falls back to Archives (lexical walk)
// so bare MPQ directories keep working. DELTA: C++ aborts the tool with
// "no locale found" when no locale directory exists; Go proceeds with
// whatever fixed/patch archives exist (C++ also pushes those
// unconditionally) and only falls back when the candidate list is empty.
func OrderedVmapArchives(input string) ([]string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return Archives(input)
	}
	dataDir := input
	if dirInfo, err := os.Stat(filepath.Join(input, "Data")); err == nil && dirInfo.IsDir() {
		dataDir = filepath.Join(input, "Data")
	}

	locales := detectVmapLocales(dataDir)

	// Build the C++ open order first.
	open := make([]string, 0, 3*len(locales)+4)
	for _, lang := range locales {
		open = vmapLocaleArchives(dataDir, lang, open)
	}
	open = append(open,
		filepath.Join(dataDir, "common.MPQ"),
		filepath.Join(dataDir, "common-2.MPQ"),
		filepath.Join(dataDir, "expansion.MPQ"),
		filepath.Join(dataDir, "lichking.MPQ"),
	)
	open = scanPatches(filepath.Join(dataDir, "patch"), open)
	for _, lang := range locales {
		open = scanPatches(filepath.Join(dataDir, lang, "patch-"+lang), open)
	}

	// Reverse into lookup-precedence order for the first-hit-wins consumer.
	ordered := make([]string, 0, len(open))
	for i := len(open) - 1; i >= 0; i-- {
		ordered = append(ordered, open[i])
	}

	any := false
	for _, p := range ordered {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			any = true
			break
		}
	}
	if !any {
		return Archives(input)
	}
	return ordered, nil
}

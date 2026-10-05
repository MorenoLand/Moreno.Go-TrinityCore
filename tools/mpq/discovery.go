package mpq

import (
	"os"
	"path/filepath"
)

// C++ owner: src/tools/map_extractor/System.cpp
//   - langs[] (fixed locale probe order), CONF_mpq_list (fixed common open
//     order), LoadLocaleMPQFiles, LoadCommonMPQFiles, main() locale loop.
//   - MPQArchive's ctor does gOpenArchives.push_front(this) and MPQFile
//     iterates from begin(), so the LAST opened archive has the HIGHEST
//     lookup precedence.
//
// C++ open order for the map/camera extraction phase:
//  1. Data/<lang>/locale-<lang>.MPQ
//  2. Data/<lang>/patch-<lang>.MPQ, patch-<lang>-2 .. -4 (existing only)
//  3. Data/common.MPQ, common-2, lichking, expansion, patch,
//     patch-2 .. patch-5 (existing only)
//
// Lookup precedence (highest first) is therefore the reverse:
//  Data/patch-5.MPQ .. Data/patch-2.MPQ, Data/patch.MPQ,
//  Data/expansion.MPQ, Data/lichking.MPQ, Data/common-2.MPQ,
//  Data/common.MPQ, Data/<lang>/patch-<lang>-4.MPQ ..
//  Data/<lang>/patch-<lang>.MPQ, Data/<lang>/locale-<lang>.MPQ
//
// The old Archives() lexical walk gets patch precedence exactly backwards
// (patch-2 beats patch-5) and puts patch.MPQ nearly last; readClientAsset
// first-hit-wins then reads stale files. OrderedArchives fixes that.

// clientLocales mirrors C++ langs[] probe order in System.cpp.
var clientLocales = []string{
	"enGB", "enUS", "deDE", "esES", "frFR", "koKR",
	"zhCN", "zhTW", "enCN", "enTW", "esMX", "ruRU",
}

// commonArchives mirrors C++ CONF_mpq_list open order in System.cpp.
var commonArchives = []string{
	"common.MPQ", "common-2.MPQ", "lichking.MPQ", "expansion.MPQ",
	"patch.MPQ", "patch-2.MPQ", "patch-3.MPQ", "patch-4.MPQ", "patch-5.MPQ",
}

// DetectLocales returns every locale (C++ langs[] order) whose
// Data/<lang>/locale-<lang>.MPQ exists under the client root.
func DetectLocales(input string) []string {
	dataDir := filepath.Join(input, "Data")
	var locales []string
	for _, lang := range clientLocales {
		candidate := filepath.Join(dataDir, lang, "locale-"+lang+".MPQ")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			locales = append(locales, lang)
		}
	}
	return locales
}

// LocaleArchives returns the locale MPQ set for one locale in C++ lookup
// precedence order (highest first): patch-<lang>-4 .. patch-<lang>,
// then locale-<lang>.MPQ. Patch files are included only when present,
// mirroring LoadLocaleMPQFiles; the locale base MPQ is always included
// (it was existence-probed by DetectLocales).
func LocaleArchives(input, lang string) []string {
	dir := filepath.Join(input, "Data", lang)
	ordered := make([]string, 0, 5)
	suffixes := []string{"-4", "-3", "-2", ""}
	for _, suffix := range suffixes {
		candidate := filepath.Join(dir, "patch-"+lang+suffix+".MPQ")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			ordered = append(ordered, candidate)
		}
	}
	ordered = append(ordered, filepath.Join(dir, "locale-"+lang+".MPQ"))
	return ordered
}

// CommonArchives returns the existing Data/<name> MPQs from the C++
// CONF_mpq_list in lookup precedence order (highest first), i.e. the
// reverse of the C++ open order.
func CommonArchives(input string) []string {
	dataDir := filepath.Join(input, "Data")
	ordered := make([]string, 0, len(commonArchives))
	for i := len(commonArchives) - 1; i >= 0; i-- {
		candidate := filepath.Join(dataDir, commonArchives[i])
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			ordered = append(ordered, candidate)
		}
	}
	return ordered
}

// OrderedArchives returns MPQ paths in C++ map_extractor lookup precedence
// order (highest first). When input is a client root (has a Data/
// subdirectory) discovery follows System.cpp exactly: locale detection via
// the fixed langs[] probe, then locale patches, then CONF_mpq_list —
// existing files only, archives outside those fixed lists ignored just like
// C++. Otherwise it falls back to Archives (lexical walk), which also covers
// the single-.mpq-file input. DELTA: C++ aborts with "No locales detected"
// when no locale MPQ exists; Go falls back to the lexical walk so bare MPQ
// directories keep working per the -input flag documentation.
func OrderedArchives(input string) ([]string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return Archives(input)
	}
	if dirInfo, err := os.Stat(filepath.Join(input, "Data")); err != nil || !dirInfo.IsDir() {
		return Archives(input)
	}
	locales := DetectLocales(input)
	if len(locales) == 0 {
		return Archives(input)
	}
	ordered := make([]string, 0, len(commonArchives)+5)
	ordered = append(ordered, CommonArchives(input)...)
	ordered = append(ordered, LocaleArchives(input, locales[0])...)
	return ordered, nil
}

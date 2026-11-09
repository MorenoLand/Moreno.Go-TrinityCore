// C++ owners: src/tools/vmap4_extractor/vmapexport.cpp (ExtractSingleWmo,
// GenerateUniqueObjectId, WmoDoodads, szWorkDirWmo) and
// gameobject_extract.cpp (ExtractSingleModel).
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

// wmoDoodadData mirrors C++ WMODoodadData (wmo.h): the doodad sets, the
// MODN path table, the MODD spawns, and the group-validated reference set.
type wmoDoodadData struct {
	Sets       []wmoDoodadSet
	Paths      map[uint32]string
	Spawns     []wmoDoodad
	References map[uint16]struct{}
}

// extractor is the shared state for one vmap4extractor run: the open MPQ
// archives in C++ lookup-precedence order (highest first), the first-hit
// file index, the output directory, and the per-run registries.
type extractor struct {
	archives        []*mpq.Archive
	archiveFiles    [][]string     // per-archive (listfile) names, scan order
	index           map[string]int // normalized MPQ name -> archive slot
	output          string
	verbose         bool
	precise         bool
	extractedModels map[string]struct{} // fixed plain names already written
	wmoDoodads      map[string]*wmoDoodadData
	uniqueObjectIDs map[[2]uint32]uint32
	dirBin          *os.File
}

func newExtractor(output string, verbose, precise bool) *extractor {
	return &extractor{
		output:          output,
		verbose:         verbose,
		precise:         precise,
		index:           make(map[string]int),
		extractedModels: make(map[string]struct{}),
		wmoDoodads:      make(map[string]*wmoDoodadData),
		uniqueObjectIDs: make(map[[2]uint32]uint32),
	}
}

// addArchive opens one MPQ and indexes its (listfile) names in
// first-hit-wins order, mirroring C++ MPQFile's walk over gOpenArchives.
func (e *extractor) addArchive(path string) error {
	arc, err := mpq.Open(path)
	if err != nil {
		return err
	}
	slot := len(e.archives)
	e.archives = append(e.archives, arc)
	files, err := arc.ListFiles()
	if err != nil {
		e.archiveFiles = append(e.archiveFiles, nil)
		return nil
	}
	e.archiveFiles = append(e.archiveFiles, files)
	for _, name := range files {
		key := strings.ToLower(filepath.ToSlash(name))
		if _, exists := e.index[key]; !exists {
			e.index[key] = slot
		}
	}
	return nil
}

func (e *extractor) close() {
	for _, arc := range e.archives {
		arc.Close()
	}
	e.archives = nil
}

// readMPQFile mirrors C++ MPQFile: first archive in precedence order that
// contains the file wins; a size<=1 dummy reads as missing (the Go mpq
// package already maps that to os.ErrNotExist).
func (e *extractor) readMPQFile(name string) ([]byte, error) {
	key := strings.ToLower(filepath.ToSlash(name))
	if slot, ok := e.index[key]; ok {
		data, err := e.archives[slot].ReadFile(name)
		if err == nil {
			return data, nil
		}
	}
	for _, arc := range e.archives {
		data, err := arc.ReadFile(name)
		if err == nil {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}

// generateUniqueObjectID mirrors C++ GenerateUniqueObjectId: first-seen
// (clientId, clientDoodadId) pairs get sequential ids starting at 1.
func (e *extractor) generateUniqueObjectID(clientID uint32, clientDoodadID uint16) uint32 {
	key := [2]uint32{clientID, uint32(clientDoodadID)}
	if id, ok := e.uniqueObjectIDs[key]; ok {
		return id
	}
	id := uint32(len(e.uniqueObjectIDs) + 1)
	e.uniqueObjectIDs[key] = id
	return id
}

// fileExists mirrors C++ FileExists (vmapexport.cpp).
func fileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// extractSingleModel mirrors C++ ExtractSingleModel
// (gameobject_extract.cpp): .mdx/.mdl names are rewritten to .m2, the
// output name is the fixed plain name under the work dir, an already
// extracted file short-circuits true, and a model with no bounding
// triangles fails (Model::open returns false).
func (e *extractor) extractSingleModel(fname string) bool {
	if len(fname) < 4 {
		return false
	}
	ext := fname[len(fname)-4:]
	if ext == ".mdx" || ext == ".MDX" || ext == ".mdl" || ext == ".MDL" {
		fname = fname[:len(fname)-2] + "2"
	}
	name := fixedPlainName(fname)
	output := filepath.Join(e.output, name)
	if fileExists(output) {
		e.extractedModels[name] = struct{}{}
		return true
	}
	data, err := e.readMPQFile(fname)
	if err != nil {
		return false
	}
	converted, err := extractM2(data)
	if err != nil {
		return false
	}
	if err := os.WriteFile(output, converted, 0o644); err != nil {
		return false
	}
	e.extractedModels[name] = struct{}{}
	return true
}

// extractSingleWmo mirrors C++ ExtractSingleWmo (vmapexport.cpp): root
// files ending in _NNN are skipped by the caller (isWMOGroupName), the
// output name is the fixed plain name, an existing file short-circuits
// true, the root's MODN doodad models are extracted (building
// ValidDoodadNames), each group is converted, the total triangle count is
// patched into the root header by writeRawWMO, and the doodad data is
// registered under the plain name for the ADT/WDT dir_bin pass. A group
// file that cannot be opened makes the extraction fail (C++ deletes the
// partial output and reports file_ok=false).
func (e *extractor) extractSingleWmo(fname string) bool {
	name := fixedPlainName(fname)
	output := filepath.Join(e.output, name)
	if fileExists(output) {
		return true
	}
	rootData, err := e.readMPQFile(fname)
	if err != nil {
		return false
	}
	root, err := parseWMORoot(rootData)
	if err != nil {
		return false
	}
	// C++ owner: wmo.cpp WMORoot::open MODN arm — every doodad path is
	// extracted as a model; only names whose model extracts land in
	// ValidDoodadNames, which gates the group reference set below.
	validDoodadNames := make(map[uint32]struct{})
	offsets := make([]uint32, 0, len(root.DoodadNames))
	for offset := range root.DoodadNames {
		offsets = append(offsets, offset)
	}
	sortUint32(offsets)
	for _, offset := range offsets {
		if e.extractSingleModel(root.DoodadNames[offset]) {
			validDoodadNames[offset] = struct{}{}
		}
	}
	groups := make([]wmoGroupInfo, 0, root.Groups)
	base := fname[:len(fname)-len(filepath.Ext(fname))]
	for index := uint32(0); index < root.Groups; index++ {
		wanted := fmt.Sprintf("%s_%03d.wmo", base, index)
		data, err := e.readMPQFile(wanted)
		if err != nil {
			if e.verbose {
				fmt.Printf("Could not open group file for: %s\n", name)
			}
			return false
		}
		group, err := parseWMOGroup(data, root.Flags, e.precise)
		if err != nil {
			return false
		}
		groups = append(groups, group)
	}
	converted, err := writeRawWMO(root, groups)
	if err != nil {
		return false
	}
	if err := os.WriteFile(output, converted, 0o644); err != nil {
		return false
	}
	// C++ owner: vmapexport.cpp ExtractSingleWmo — the doodad data moves
	// into WmoDoodads[plain_name] and the reference set is the union of
	// the groups' MODR refs whose spawn name extracted successfully.
	refs := make(map[uint16]struct{})
	for _, group := range groups {
		for _, ref := range group.DoodadRefs {
			if int(ref) >= len(root.Doodads) {
				continue
			}
			if _, ok := validDoodadNames[root.Doodads[ref].NameIndex]; !ok {
				continue
			}
			refs[ref] = struct{}{}
		}
	}
	e.wmoDoodads[name] = &wmoDoodadData{
		Sets:       root.DoodadSets,
		Paths:      root.DoodadNames,
		Spawns:     root.Doodads,
		References: refs,
	}
	return true
}

// openDirBin creates a fresh dir_bin under the output directory. C++
// opens it per ADT/WDT file in "ab" mode, but main() aborts when
// Buildings/dir or dir_bin already exist, so a fresh create is the same
// observable behavior.
func (e *extractor) openDirBin() error {
	path := filepath.Join(e.output, "dir_bin")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("can't open dirfile %q: %w", path, err)
	}
	e.dirBin = f
	return nil
}

func (e *extractor) closeDirBin() {
	if e.dirBin != nil {
		e.dirBin.Close()
		e.dirBin = nil
	}
}

func sortUint32(v []uint32) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func sortUint16(v []uint16) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// dirBinRecord is one dir_bin spawn record as read by the Go
// vmap4assembler (tools/vmap4assembler/mapassemble.go readModelSpawn):
// Flags u32, NameSet u16, UniqueId u32, Position, Rotation, Scale, then
// BoundsLow/BoundsHigh when Flags has MOD_HAS_BOUND, then name length +
// name. C++ writes exactly this layout from MapObject::Extract and
// Doodad::Extract / Doodad::ExtractSet.
func (e *extractor) writeDirBinRecord(mapID, tileX, tileY, flags uint32, nameSet uint16, uniqueID uint32, position, rotation vec3, scale float32, boundsLow, boundsHigh *vec3, name string) error {
	w := e.dirBin
	write := func(v any) error { return binary.Write(w, binary.LittleEndian, v) }
	for _, v := range []any{mapID, tileX, tileY, flags, nameSet, uniqueID, position, rotation, scale} {
		if err := write(v); err != nil {
			return err
		}
	}
	if boundsLow != nil && boundsHigh != nil {
		if err := write(*boundsLow); err != nil {
			return err
		}
		if err := write(*boundsHigh); err != nil {
			return err
		}
	}
	if err := write(uint32(len(name))); err != nil {
		return err
	}
	_, err := w.WriteString(name)
	return err
}

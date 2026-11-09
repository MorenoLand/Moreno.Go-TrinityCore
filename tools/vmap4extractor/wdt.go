// C++ owners: src/tools/vmap4_extractor/wdtfile.cpp (WDTFile::init,
// WDTFile::GetMap) and vmapexport.cpp (ParsMapFiles).
package main

import (
	"encoding/binary"
	"fmt"
)

// parseWDT mirrors C++ WDTFile::init (wdtfile.cpp): the MWMO chunk lists
// global WMO paths (each extracted, names recorded) and the MODF chunk
// writes a MapObject record plus the WMO's doodad set per placement, all
// on the global (65, 65) tile.
func (e *extractor) parseWDT(data []byte, mapID uint32) error {
	wmoNames := make([]string, 0)
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			break
		}
		name := flipChunkName(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if size < 0 || size > len(data)-offset {
			return fmt.Errorf("invalid WDT chunk %q size %d", name, size)
		}
		chunk := data[offset : offset+size]
		next := offset + size
		switch name {
		case "MWMO":
			for p := 0; p < len(chunk); {
				end := p
				for end < len(chunk) && chunk[end] != 0 {
					end++
				}
				path := string(chunk[p:end])
				plain := fixedPlainName(path)
				wmoNames = append(wmoNames, plain)
				e.extractSingleWmo(path)
				p = end + 1
			}
		case "MODF":
			if size%64 != 0 {
				return fmt.Errorf("invalid WDT MODF chunk size %d", size)
			}
			for i := 0; i < size/64; i++ {
				def, err := parseMODF(chunk[i*64:])
				if err != nil {
					return err
				}
				if int(def.ID) >= len(wmoNames) {
					continue
				}
				wmoName := wmoNames[def.ID]
				e.mapObjectExtract(def, wmoName, mapID, 65, 65)
				e.doodadExtractSet(e.wmoDoodads[wmoName], def, mapID, 65, 65)
			}
		}
		offset = next
	}
	return nil
}

// adtFileName mirrors C++ WDTFile::GetMap: the ADT for tile (x, z) of a
// map lives at World\Maps\<name>\<name>_<x>_<z>.adt.
func adtFileName(mapName string, x, z int) string {
	return fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, mapName, mapName, x, z)
}

// parseMapFiles mirrors C++ ParsMapFiles (vmapexport.cpp): for every map
// in Map.dbc, open World\Maps\<name>\<name>.wdt; when it exists, run the
// WDT pass and then the ADT pass for all 64x64 tiles whose ADT exists.
// A missing WDT skips the map silently (C++ prints nothing either when
// WDT::init fails); a missing ADT is skipped the same way (MPQFile eof).
func (e *extractor) parseMapFiles(maps []mapEntry) {
	for _, m := range maps {
		wdtName := fmt.Sprintf(`World\Maps\%s\%s.wdt`, m.name, m.name)
		wdtData, err := e.readMPQFile(wdtName)
		if err != nil {
			continue
		}
		fmt.Printf("Processing Map %d\n", m.id)
		if err := e.parseWDT(wdtData, m.id); err != nil {
			fmt.Printf("WDT pass failed for map %d: %v\n", m.id, err)
			continue
		}
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				adtData, err := e.readMPQFile(adtFileName(m.name, x, y))
				if err != nil {
					continue
				}
				_ = e.parseADT(adtData, m.id, uint32(x), uint32(y))
			}
		}
	}
}

// mapEntry is one Map.dbc row: field 0 is the map id, field 1 the map
// directory name.
type mapEntry struct {
	id   uint32
	name string
}

// loadMapDBC mirrors the Map.dbc arm of C++ main(): read
// DBFilesClient\Map.dbc through the archives (first hit wins) and collect
// (id, name) per record. A missing or malformed Map.dbc is fatal, exactly
// like C++ ("FATAL ERROR: Map.dbc not found in data file.").
func (e *extractor) loadMapDBC() ([]mapEntry, error) {
	data, err := e.readMPQFile(`DBFilesClient\Map.dbc`)
	if err != nil {
		return nil, fmt.Errorf("FATAL ERROR: Map.dbc not found in data file")
	}
	dbc, err := openDBC(data)
	if err != nil {
		return nil, fmt.Errorf("FATAL ERROR: Map.dbc is invalid: %w", err)
	}
	maps := make([]mapEntry, 0, dbc.records())
	for id := uint32(0); id < dbc.records(); id++ {
		mapID, err := dbc.getUInt(id, 0)
		if err != nil {
			return nil, err
		}
		name, err := dbc.getString(id, 1)
		if err != nil {
			return nil, err
		}
		if len(name) >= 64 {
			return nil, fmt.Errorf("FATAL ERROR: Map name too long")
		}
		fmt.Printf("Map - %s\n", name)
		maps = append(maps, mapEntry{id: mapID, name: name})
	}
	return maps, nil
}

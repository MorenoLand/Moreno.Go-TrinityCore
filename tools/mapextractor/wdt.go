package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const wdtMapSize = 64

type wdtTile struct {
	Exists uint32
	Data   uint32
}

type wdtInfo struct {
	Version             uint32
	MPHD                [8]uint32
	Tiles               [wdtMapSize][wdtMapSize]wdtTile
	TileCount           int
	HasMain             bool
	HasGlobalWMO        bool
	GlobalWMO           string
	GlobalWMOModels     []adtWorldModelInstance
	GlobalWMOModelNames []string
}

func parseWDT(data []byte) (wdtInfo, error) {
	var info wdtInfo
	seenMVER := false
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return wdtInfo{}, errors.New("truncated WDT chunk header")
		}
		name := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if size < 0 || size > len(data)-offset {
			return wdtInfo{}, fmt.Errorf("invalid WDT chunk %q size %d", name, size)
		}
		chunk := data[offset : offset+size]
		switch name {
		case "MVER":
			if size < 4 {
				return wdtInfo{}, errors.New("truncated WDT MVER chunk")
			}
			info.Version = binary.LittleEndian.Uint32(chunk)
			// C++ owner: FileLoader::prepareLoadedData
			// (map_extractor/loadlib/loadlib.cpp:65-74) — the load fails
			// unless the MVER version equals FILE_FORMAT_VERSION (18,
			// loadlib/loadlib.h:25); ExtractMapsFromMpq then skips the map.
			if info.Version != 18 {
				return wdtInfo{}, fmt.Errorf("unsupported WDT version %d, want 18", info.Version)
			}
			seenMVER = true
		case "MPHD":
			if size < len(info.MPHD)*4 {
				return wdtInfo{}, errors.New("truncated WDT MPHD chunk")
			}
			for index := range info.MPHD {
				info.MPHD[index] = binary.LittleEndian.Uint32(chunk[index*4:])
			}
		case "MAIN":
			const tileBytes = wdtMapSize * wdtMapSize * 8
			if size < tileBytes {
				return wdtInfo{}, fmt.Errorf("truncated WDT MAIN chunk: got %d, want %d", size, tileBytes)
			}
			for y := 0; y < wdtMapSize; y++ {
				for x := 0; x < wdtMapSize; x++ {
					base := (y*wdtMapSize + x) * 8
					info.Tiles[y][x] = wdtTile{Exists: binary.LittleEndian.Uint32(chunk[base:]), Data: binary.LittleEndian.Uint32(chunk[base+4:])}
					if info.Tiles[y][x].Exists != 0 {
						info.TileCount++
					}
				}
			}
			info.HasMain = true
		case "MWMO":
			info.HasGlobalWMO = size > 0
			if size > 0 {
				info.GlobalWMOModelNames = parseNameList(chunk)
				info.GlobalWMO = strings.TrimRight(string(chunk), "\x00")
			}
		case "MODF":
			instances, err := parseMODF(chunk)
			if err != nil {
				return wdtInfo{}, err
			}
			info.GlobalWMOModels = append(info.GlobalWMOModels, instances...)
		}
		offset += size
	}
	if !seenMVER {
		return wdtInfo{}, errors.New("WDT MVER chunk not found")
	}
	if !info.HasMain {
		return wdtInfo{}, errors.New("WDT MAIN chunk not found")
	}
	return info, nil
}

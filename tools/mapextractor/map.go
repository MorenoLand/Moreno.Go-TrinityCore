// MorenoCore Map Extractor (.map writer): converts one .adt tile into the
// v1.9 .map file the server reads, and drives the per-map extraction loop.
//
// C++ owner: src/tools/map_extractor/System.cpp — ConvertADT (System.cpp
// :359-917) + ExtractMapsFromMpq (:918-970) + ReadMapDBC (:208-238) +
// ReadLiquidTypeTableDBC (:240-260) + ReadBuild (:171-206) + main()
// (:1118-1195). Struct layouts and flags come from
// src/server/game/Maps/Map.h (map_fileheader :90, map_areaHeader :103,
// map_heightHeader :119, map_liquidHeader :130) and
// src/tools/map_extractor/adt.h (adt_MCNK/MCVT/MCLQ/MH2O/MFBO).
//
// Fidelity notes (deliberate deltas from the C++):
//   - V9/V8 and liquid_height are C++ file-scope statics that ConvertADT
//     never zeroes, so a tile inherits the previous tile's values in any
//     region its own cells do not overwrite. That is unobservable: the
//     area leg dereferences every cell (null -> crash) before heights run,
//     and liquid_height is only ever read for shown cells, which the leg
//     that shows them always writes first. Go uses fresh zeroed arrays
//     per tile instead of carrying stale state.
//   - A missing MCNK cell crashes C++ (null deref in the area leg); Go
//     returns an error naming the cell instead.
//   - A LiquidType id absent from LiquidType.dbc throws in C++
//     (unordered_map::at); Go returns an error naming the id.
//   - MH2O instance rectangles that run past the 128x128 grid would
//     write out of bounds in C++; Go skips out-of-range tiles (valid
//     data never triggers this: Offset+Width/Height stay inside the
//     8x8 cell).
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// .map file magics (System.cpp:264-268).
const (
	mapMagic        = "MAPS"
	mapVersionMagic = "v1.9"
	mapAreaMagic    = "AREA"
	mapHeightMagic  = "MHGT"
	mapLiquidMagic  = "MLIQ"
)

// Section flags (Map.h:105, :114-117, :127-128).
const (
	mapAreaNoArea            = 0x0001
	mapHeightNoHeight        = 0x0001
	mapHeightAsInt16         = 0x0002
	mapHeightAsInt8          = 0x0004
	mapHeightHasFlightBounds = 0x0008
	mapLiquidNoType          = 0x0001
	mapLiquidNoHeight        = 0x0002
)

// Liquid type flags (Map.h:156-163).
const (
	mapLiquidTypeWater     = 0x01
	mapLiquidTypeOcean     = 0x02
	mapLiquidTypeMagma     = 0x04
	mapLiquidTypeSlime     = 0x08
	mapLiquidTypeDarkWater = 0x10
)

// LiquidType.dbc SoundBank values (adt.h:28-33).
const (
	liquidSoundBankWater = 0
	liquidSoundBankOcean = 1
	liquidSoundBankMagma = 2
	liquidSoundBankSlime = 3
)

// Extractor CONF defaults (System.cpp:63-76).
const (
	mapUseMinHeight         = -500.0
	mapFloatToInt8Limit     = 2.0
	mapFloatToInt16Limit    = 2048.0
	mapFlatHeightDeltaLimit = 0.005
	mapFlatLiquidDeltaLimit = 0.001
)

// convertADT converts one parsed .adt tile into a v1.9 .map file, following
// System.cpp ConvertADT step for step: area, height (V9/V8), MCLQ liquid,
// MH2O liquid, liquid packing, holes, then the file write. adtPath is only
// used in diagnostics. liquidTypes maps LiquidType.dbc id -> SoundBank.
func convertADT(adtData []byte, adtPath string, build uint32, floatToInt bool, liquidTypes map[uint32]uint8) ([]byte, error) {
	info, err := parseADT(adtData)
	if err != nil {
		return nil, err
	}
	if !info.HasMCIN {
		return nil, fmt.Errorf("can't find cells in %q", adtPath)
	}

	// Get area flags data. C++ dereferences getMCNK(i, j) with no nil
	// check; a missing cell is an error here instead of a crash.
	var areaIDs [16][16]uint16
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			cell := info.Grid[i][j]
			if cell == nil {
				return nil, fmt.Errorf("missing MCNK cell %d,%d in %q", i, j, adtPath)
			}
			areaIDs[i][j] = uint16(cell.AreaID)
		}
	}
	fullAreaData := false
	areaID := areaIDs[0][0]
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if areaIDs[y][x] != areaID {
				fullAreaData = true
				break
			}
		}
	}

	// Get height map from grid: seed each cell's V9/V8 samples with its
	// ypos, then add MCVT deltas. Edge V9s overlap between cells; the
	// later cell (higher i/j) wins the shared edge, exactly as in C++.
	var v9 [129][129]float32
	var v8 [128][128]float32
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			cell := info.Grid[i][j]
			for y := 0; y <= 8; y++ {
				cy := i*8 + y
				for x := 0; x <= 8; x++ {
					v9[cy][j*8+x] = cell.YPos
				}
			}
			for y := 0; y < 8; y++ {
				cy := i*8 + y
				for x := 0; x < 8; x++ {
					v8[cy][j*8+x] = cell.YPos
				}
			}
			if !cell.HasMCVT {
				continue
			}
			for y := 0; y <= 8; y++ {
				cy := i*8 + y
				for x := 0; x <= 8; x++ {
					v9[cy][j*8+x] += cell.Heights[y*17+x]
				}
			}
			for y := 0; y < 8; y++ {
				cy := i*8 + y
				for x := 0; x < 8; x++ {
					v8[cy][j*8+x] += cell.Heights[y*17+9+x]
				}
			}
		}
	}

	minHeight := float32(20000)
	maxHeight := float32(-20000)
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			h := v8[y][x]
			if maxHeight < h {
				maxHeight = h
			}
			if minHeight > h {
				minHeight = h
			}
		}
	}
	for y := 0; y <= 128; y++ {
		for x := 0; x <= 128; x++ {
			h := v9[y][x]
			if maxHeight < h {
				maxHeight = h
			}
			if minHeight > h {
				minHeight = h
			}
		}
	}

	// Check for allow limit minimum height.
	if minHeight < mapUseMinHeight {
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				if v8[y][x] < mapUseMinHeight {
					v8[y][x] = mapUseMinHeight
				}
			}
		}
		for y := 0; y <= 128; y++ {
			for x := 0; x <= 128; x++ {
				if v9[y][x] < mapUseMinHeight {
					v9[y][x] = mapUseMinHeight
				}
			}
		}
		if minHeight < mapUseMinHeight {
			minHeight = mapUseMinHeight
		}
		if maxHeight < mapUseMinHeight {
			maxHeight = mapUseMinHeight
		}
	}

	hasFlightBox := info.HasMFBO && info.MHDRFlags&1 != 0

	areaMapOffset := uint32(44)
	areaMapSize := uint32(8)
	if fullAreaData {
		areaMapSize += 512
	}
	heightMapOffset := areaMapOffset + areaMapSize
	heightMapSize := uint32(16)
	if hasFlightBox {
		heightMapSize += 36
	}

	var heightFlags uint32
	if maxHeight == minHeight {
		heightFlags |= mapHeightNoHeight
	}
	// Not need store if flat surface.
	if floatToInt && maxHeight-minHeight < mapFlatHeightDeltaLimit {
		heightFlags |= mapHeightNoHeight
	}
	if hasFlightBox {
		heightFlags |= mapHeightHasFlightBounds
	}

	// Try store as packed in uint16 or uint8 values.
	var uint8V9 [129][129]uint8
	var uint8V8 [128][128]uint8
	var uint16V9 [129][129]uint16
	var uint16V8 [128][128]uint16
	if heightFlags&mapHeightNoHeight == 0 {
		if floatToInt {
			diff := maxHeight - minHeight
			if diff < mapFloatToInt8Limit {
				heightFlags |= mapHeightAsInt8
				step := float32(255) / diff
				for y := 0; y < 128; y++ {
					for x := 0; x < 128; x++ {
						uint8V8[y][x] = packUint8((v8[y][x] - minHeight) * step)
					}
				}
				for y := 0; y <= 128; y++ {
					for x := 0; x <= 128; x++ {
						uint8V9[y][x] = packUint8((v9[y][x] - minHeight) * step)
					}
				}
				heightMapSize += 129*129 + 128*128
			} else if diff < mapFloatToInt16Limit {
				heightFlags |= mapHeightAsInt16
				step := float32(65535) / diff
				for y := 0; y < 128; y++ {
					for x := 0; x < 128; x++ {
						uint16V8[y][x] = packUint16((v8[y][x] - minHeight) * step)
					}
				}
				for y := 0; y <= 128; y++ {
					for x := 0; x <= 128; x++ {
						uint16V9[y][x] = packUint16((v9[y][x] - minHeight) * step)
					}
				}
				heightMapSize += 2 * (129*129 + 128*128)
			}
		}
		if heightFlags&(mapHeightAsInt8|mapHeightAsInt16) == 0 {
			heightMapSize += 4 * (129*129 + 128*128)
		}
	}

	// Get from MCLQ chunk (old).
	var liquidEntry [16][16]uint16
	var liquidFlags [16][16]uint8
	var liquidShow [128][128]bool
	var liquidHeight [129][129]float32
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			cell := info.Grid[i][j]
			if !cell.HasMCLQ || cell.MCLQSize <= 8 {
				continue
			}
			count := 0
			for y := 0; y < 8; y++ {
				cy := i*8 + y
				for x := 0; x < 8; x++ {
					cx := j*8 + x
					if cell.MCLQFlags[y][x] != 0x0F {
						liquidShow[cy][cx] = true
						if cell.MCLQFlags[y][x]&(1<<7) != 0 {
							liquidFlags[i][j] |= mapLiquidTypeDarkWater
						}
						count++
					}
				}
			}
			cFlag := cell.Flags
			if cFlag&(1<<2) != 0 {
				liquidEntry[i][j] = 1
				liquidFlags[i][j] |= mapLiquidTypeWater
			}
			if cFlag&(1<<3) != 0 {
				liquidEntry[i][j] = 2
				liquidFlags[i][j] |= mapLiquidTypeOcean
			}
			if cFlag&(1<<4) != 0 {
				liquidEntry[i][j] = 3
				liquidFlags[i][j] |= mapLiquidTypeMagma
			}
			if count == 0 && liquidFlags[i][j] != 0 {
				fmt.Fprintln(os.Stderr, "Wrong liquid detect in MCLQ chunk")
			}
			for y := 0; y <= 8; y++ {
				cy := i*8 + y
				for x := 0; x <= 8; x++ {
					liquidHeight[cy][j*8+x] = cell.MCLQLiquid[y][x]
				}
			}
		}
	}

	// Get liquid map for grid (in WOTLK used MH2O chunk).
	if info.MH2OCount > 0 {
		for i := 0; i < 16; i++ {
			for j := 0; j < 16; j++ {
				inst := info.Liquid[i][j]
				if !inst.HasInstance {
					continue
				}
				count := 0
				existsMask := inst.ExistsBitmap
				for y := 0; y < int(inst.Height); y++ {
					cy := i*8 + y + int(inst.OffsetY)
					for x := 0; x < int(inst.Width); x++ {
						cx := j*8 + x + int(inst.OffsetX)
						if existsMask&1 != 0 {
							if cy >= 0 && cy < 128 && cx >= 0 && cx < 128 {
								liquidShow[cy][cx] = true
							}
							count++
						}
						existsMask >>= 1
					}
				}
				liquidEntry[i][j] = inst.LiquidType
				soundBank, ok := liquidTypes[uint32(inst.LiquidType)]
				if !ok {
					return nil, fmt.Errorf("unknown LiquidType %d for map %s chunk %d,%d", inst.LiquidType, adtPath, i, j)
				}
				switch soundBank {
				case liquidSoundBankWater:
					liquidFlags[i][j] |= mapLiquidTypeWater
				case liquidSoundBankOcean:
					liquidFlags[i][j] |= mapLiquidTypeOcean
					if inst.Deep {
						liquidFlags[i][j] |= mapLiquidTypeDarkWater
					}
				case liquidSoundBankMagma:
					liquidFlags[i][j] |= mapLiquidTypeMagma
				case liquidSoundBankSlime:
					liquidFlags[i][j] |= mapLiquidTypeSlime
				default:
					fmt.Printf("\nCan't find Liquid type %d for map %s\nchunk %d,%d\n", inst.LiquidType, adtPath, i, j)
				}
				if count == 0 && liquidFlags[i][j] != 0 {
					fmt.Print("Wrong liquid detect in MH2O chunk")
				}
				pos := 0
				for y := 0; y <= int(inst.Height); y++ {
					cy := i*8 + y + int(inst.OffsetY)
					for x := 0; x <= int(inst.Width); x++ {
						cx := j*8 + x + int(inst.OffsetX)
						if cy >= 0 && cy < 129 && cx >= 0 && cx < 129 {
							liquidHeight[cy][cx] = liquidHeightAt(&inst, pos)
						}
						pos++
					}
				}
			}
		}
	}

	// Pack liquid data.
	firstLiquidType := liquidEntry[0][0]
	firstLiquidFlag := liquidFlags[0][0]
	fullType := false
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if liquidEntry[y][x] != firstLiquidType || liquidFlags[y][x] != firstLiquidFlag {
				fullType = true
				y = 16
				break
			}
		}
	}

	var liquidMapOffset, liquidMapSize uint32
	var liquidHeaderFlags, liquidHeaderLiquidFlags uint8
	var liquidHeaderLiquidType uint16
	var liquidOffsetX, liquidOffsetY, liquidWidth, liquidHeightDim uint8
	var liquidLevel float32
	if firstLiquidFlag == 0 && !fullType {
		// No liquid data.
		liquidMapOffset = 0
		liquidMapSize = 0
	} else {
		minX, minY := 255, 255
		maxX, maxY := 0, 0
		lMaxHeight := float32(-20000)
		lMinHeight := float32(20000)
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				if liquidShow[y][x] {
					if minX > x {
						minX = x
					}
					if maxX < x {
						maxX = x
					}
					if minY > y {
						minY = y
					}
					if maxY < y {
						maxY = y
					}
					h := liquidHeight[y][x]
					if lMaxHeight < h {
						lMaxHeight = h
					}
					if lMinHeight > h {
						lMinHeight = h
					}
				} else {
					liquidHeight[y][x] = mapUseMinHeight
				}
			}
		}
		liquidMapOffset = heightMapOffset + heightMapSize
		liquidMapSize = 16
		liquidOffsetX = uint8(minX)
		liquidOffsetY = uint8(minY)
		liquidWidth = uint8(maxX - minX + 2)
		liquidHeightDim = uint8(maxY - minY + 2)
		liquidLevel = lMinHeight
		if lMaxHeight == lMinHeight {
			liquidHeaderFlags |= mapLiquidNoHeight
		}
		// Not need store if flat surface.
		if floatToInt && lMaxHeight-lMinHeight < mapFlatLiquidDeltaLimit {
			liquidHeaderFlags |= mapLiquidNoHeight
		}
		if !fullType {
			liquidHeaderFlags |= mapLiquidNoType
		}
		if liquidHeaderFlags&mapLiquidNoType != 0 {
			liquidHeaderLiquidFlags = firstLiquidFlag
			liquidHeaderLiquidType = firstLiquidType
		} else {
			liquidMapSize += 512 + 256
		}
		if liquidHeaderFlags&mapLiquidNoHeight == 0 {
			liquidMapSize += uint32(4) * uint32(liquidWidth) * uint32(liquidHeightDim)
		}
	}

	var holes [16][16]uint16
	hasHoles := false
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			cell := info.Grid[i][j]
			holes[i][j] = uint16(cell.Holes)
			if cell.Holes != 0 {
				hasHoles = true
			}
		}
	}
	var holesOffset, holesSize uint32
	if hasHoles {
		if liquidMapOffset != 0 {
			holesOffset = liquidMapOffset + liquidMapSize
		} else {
			holesOffset = heightMapOffset + heightMapSize
		}
		holesSize = 512
	}

	// Ok all data prepared - store it.
	var buf bytes.Buffer
	// map_fileheader (44 bytes).
	buf.WriteString(mapMagic)
	buf.WriteString(mapVersionMagic)
	binary.Write(&buf, binary.LittleEndian, build)
	binary.Write(&buf, binary.LittleEndian, areaMapOffset)
	binary.Write(&buf, binary.LittleEndian, areaMapSize)
	binary.Write(&buf, binary.LittleEndian, heightMapOffset)
	binary.Write(&buf, binary.LittleEndian, heightMapSize)
	binary.Write(&buf, binary.LittleEndian, liquidMapOffset)
	binary.Write(&buf, binary.LittleEndian, liquidMapSize)
	binary.Write(&buf, binary.LittleEndian, holesOffset)
	binary.Write(&buf, binary.LittleEndian, holesSize)
	// Store area data.
	buf.WriteString(mapAreaMagic)
	var areaFlags uint16
	var gridArea uint16
	if fullAreaData {
		gridArea = 0
	} else {
		areaFlags |= mapAreaNoArea
		gridArea = areaID
	}
	binary.Write(&buf, binary.LittleEndian, areaFlags)
	binary.Write(&buf, binary.LittleEndian, gridArea)
	if areaFlags&mapAreaNoArea == 0 {
		for i := 0; i < 16; i++ {
			for j := 0; j < 16; j++ {
				binary.Write(&buf, binary.LittleEndian, areaIDs[i][j])
			}
		}
	}
	// Store height data.
	buf.WriteString(mapHeightMagic)
	binary.Write(&buf, binary.LittleEndian, heightFlags)
	binary.Write(&buf, binary.LittleEndian, minHeight)
	binary.Write(&buf, binary.LittleEndian, maxHeight)
	if heightFlags&mapHeightNoHeight == 0 {
		switch {
		case heightFlags&mapHeightAsInt16 != 0:
			for y := 0; y <= 128; y++ {
				for x := 0; x <= 128; x++ {
					binary.Write(&buf, binary.LittleEndian, uint16V9[y][x])
				}
			}
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					binary.Write(&buf, binary.LittleEndian, uint16V8[y][x])
				}
			}
		case heightFlags&mapHeightAsInt8 != 0:
			for y := 0; y <= 128; y++ {
				for x := 0; x <= 128; x++ {
					buf.WriteByte(uint8V9[y][x])
				}
			}
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					buf.WriteByte(uint8V8[y][x])
				}
			}
		default:
			for y := 0; y <= 128; y++ {
				for x := 0; x <= 128; x++ {
					binary.Write(&buf, binary.LittleEndian, v9[y][x])
				}
			}
			for y := 0; y < 128; y++ {
				for x := 0; x < 128; x++ {
					binary.Write(&buf, binary.LittleEndian, v8[y][x])
				}
			}
		}
	}
	if heightFlags&mapHeightHasFlightBounds != 0 {
		for index := 0; index < 9; index++ {
			binary.Write(&buf, binary.LittleEndian, info.FlightMax[index])
		}
		for index := 0; index < 9; index++ {
			binary.Write(&buf, binary.LittleEndian, info.FlightMin[index])
		}
	}
	// Store liquid data if need.
	if liquidMapOffset != 0 {
		buf.WriteString(mapLiquidMagic)
		buf.WriteByte(liquidHeaderFlags)
		buf.WriteByte(liquidHeaderLiquidFlags)
		binary.Write(&buf, binary.LittleEndian, liquidHeaderLiquidType)
		buf.WriteByte(liquidOffsetX)
		buf.WriteByte(liquidOffsetY)
		buf.WriteByte(liquidWidth)
		buf.WriteByte(liquidHeightDim)
		binary.Write(&buf, binary.LittleEndian, liquidLevel)
		if liquidHeaderFlags&mapLiquidNoType == 0 {
			for i := 0; i < 16; i++ {
				for j := 0; j < 16; j++ {
					binary.Write(&buf, binary.LittleEndian, liquidEntry[i][j])
				}
			}
			for i := 0; i < 16; i++ {
				for j := 0; j < 16; j++ {
					buf.WriteByte(liquidFlags[i][j])
				}
			}
		}
		if liquidHeaderFlags&mapLiquidNoHeight == 0 {
			for y := 0; y < int(liquidHeightDim); y++ {
				for x := 0; x < int(liquidWidth); x++ {
					binary.Write(&buf, binary.LittleEndian, liquidHeight[y+int(liquidOffsetY)][int(liquidOffsetX)+x])
				}
			}
		}
	}
	// Store hole data.
	if hasHoles {
		for i := 0; i < 16; i++ {
			for j := 0; j < 16; j++ {
				binary.Write(&buf, binary.LittleEndian, holes[i][j])
			}
		}
	}
	return buf.Bytes(), nil
}

// packUint8 mirrors C++ uint8((v - minHeight) * step + 0.5f): round-half-up
// with an explicit clamp, since a float-to-uint conversion of a value
// above the target max is implementation-defined in Go.
func packUint8(v float32) uint8 {
	v += 0.5
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// packUint16 mirrors C++ uint16((v - minHeight) * step + 0.5f).
func packUint16(v float32) uint16 {
	v += 0.5
	if v <= 0 {
		return 0
	}
	if v >= 65535 {
		return 65535
	}
	return uint16(v)
}

// liquidHeightAt mirrors adt_MH2O::GetLiquidHeight (adt.h): the heights
// are a contiguous float array at OffsetVertexData for HeightDepth (0)
// and HeightTextureCoord (1) formats; Depth (2) and missing vertex data
// read 0.
func liquidHeightAt(inst *adtLiquidInstance, pos int) float32 {
	if (inst.Format == 0 || inst.Format == 1) && pos < len(inst.VertexHeights) {
		return inst.VertexHeights[pos]
	}
	return 0
}

// extractMaps drives the .map extraction over every map in Map.dbc,
// following System.cpp ExtractMapsFromMpq. It returns the number of .map
// files written into <output>/maps. A WDT that fails to load skips its
// map (C++ loadFile fail -> continue); a missing or unconvertible ADT
// warns on stderr and skips that tile.
func extractMaps(input, output string, floatToInt bool) (int, error) {
	fmt.Println("Extracting maps...")
	maps, err := readMapDBC(input)
	if err != nil {
		return 0, err
	}
	liquidTypes, err := readLiquidTypeDBC(input)
	if err != nil {
		return 0, err
	}
	build, err := readClientBuild(input)
	if err != nil {
		return 0, err
	}
	fmt.Printf("Detected client build: %d\n", build)
	mapsDir := filepath.Join(output, "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		return 0, err
	}
	fmt.Println("Convert map files")
	converted := 0
	for z, m := range maps {
		fmt.Printf("Extract %s (%d/%d)                  \n", m.Name, z+1, len(maps))
		wdtData, err := readClientAsset(input, fmt.Sprintf(`World\Maps\%s\%s.wdt`, m.Name, m.Name))
		if err != nil {
			continue
		}
		wdt, err := parseWDT(wdtData)
		if err != nil {
			continue
		}
		for y := 0; y < wdtMapSize; y++ {
			for x := 0; x < wdtMapSize; x++ {
				if wdt.Tiles[y][x].Exists == 0 {
					continue
				}
				adtPath := fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, m.Name, m.Name, x, y)
				adtData, err := readClientAsset(input, adtPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", adtPath, err)
					continue
				}
				out, err := convertADT(adtData, adtPath, build, floatToInt, liquidTypes)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", adtPath, err)
					continue
				}
				outPath := filepath.Join(mapsDir, fmt.Sprintf("%03d%02d%02d.map", m.ID, y, x))
				if err := os.WriteFile(outPath, out, 0644); err != nil {
					return converted, err
				}
				converted++
			}
			fmt.Printf("Processing........................%d%%\r", (100*(y+1))/wdtMapSize)
		}
	}
	fmt.Println()
	return converted, nil
}

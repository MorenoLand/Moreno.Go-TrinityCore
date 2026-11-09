package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type adtInfo struct {
	HasMHDR           bool
	MHDRFlags         uint32
	HasMCIN           bool
	HasMTEX           bool
	HasMMDX           bool
	HasMMID           bool
	HasMWMO           bool
	HasMWID           bool
	HasMDDF           bool
	HasMODF           bool
	MCNKCount         int
	MH2OCount         int
	MCLQCount         int
	MCVTCount         int
	MCLYCount         int
	MCALCount         int
	MCVTHeights       int
	HeightMin         float32
	HeightMax         float32
	MH2OHeaders       int
	LiquidLayers      int
	LiquidAttributes  int
	LiquidInstances   int
	LiquidTiles       int
	LiquidExistsBytes int
	LiquidVertexBytes int
	MCLQBytes         int
	Cells             []adtCellInfo
	// Grid is the per-cell view of Cells, indexed [iy][ix] (row, column)
	// — the MCNK ix/iy fields place each cell, matching C++
	// adt_MCNK::getMCNK(i, j) indexing in ConvertADT.
	Grid [16][16]*adtCellInfo
	// Liquid holds the first MH2O instance per cell, indexed [iy][ix];
	// a zero value means the cell has no liquid instance. ConvertADT
	// reads only the first instance (adt_MH2O::GetLiquidInstance).
	Liquid [16][16]adtLiquidInstance
	// FlightBounds is the MFBO data (adt_MFBO, adt.h), valid when
	// HasMFBO is set and MHDRFlags bit 0 is set (adt_MFBO::getMFBO
	// gate): Max/Min are the two int16[9] flight-bound planes, file
	// order.
	HasMFBO         bool
	FlightMax       [9]int16
	FlightMin       [9]int16
	Doodads         []adtDoodadInstance
	WorldModels     []adtWorldModelInstance
	DoodadNames     []string
	WorldModelNames []string
	InstanceOrder   []adtModelInstanceRef
}

type adtDoodadInstance struct {
	NameID, UniqueID uint32
	Position         [3]float32
	Rotation         [3]float32
	Scale            float32
	Flags            uint16
}

type adtWorldModelInstance struct {
	NameID, UniqueID uint32
	Position         [3]float32
	Rotation         [3]float32
	BoundsMin        [3]float32
	BoundsMax        [3]float32
	Flags            uint16
	DoodadSet        uint16
	NameSet          uint16
	Scale            float32
}

type adtModelInstanceRef struct {
	Doodad bool
	Index  int
}

type adtCellInfo struct {
	Flags      uint32
	X          uint32
	Y          uint32
	Layers     uint32
	DoodadRefs uint32
	AreaID     uint32
	Holes      uint32
	// YPos is the MCNK base height (adt_MCNK.ypos, map_extractor/adt.h):
	// ConvertADT seeds every V9/V8 sample of the cell with it before
	// adding MCVT deltas.
	YPos float32
	// Heights holds the 145 MCVT samples (adt_MCVT.height_map, adt.h),
	// valid when HasMCVT is set. MCVT must be exactly 145 floats.
	Heights [145]float32
	HasMCVT bool
	// MCLQ (old liquid) data, valid when HasMCLQ is set. MCLQSize is the
	// MCNK sizeMCLQ field; ConvertADT skips the cell's MCLQ leg when the
	// chunk is absent or sizeMCLQ <= 8.
	HasMCLQ    bool
	MCLQSize   uint32
	MCLQLiquid [9][9]float32
	MCLQFlags  [8][8]uint8
}

// adtLiquidInstance is one MH2O liquid instance (adt_liquid_instance,
// map_extractor/adt.h:188-200) — ConvertADT reads only the FIRST instance
// per cell (adt_MH2O::GetLiquidInstance). VertexHeights holds the
// (Width+1)*(Height+1) height samples in row-major order, present only
// for HeightDepth/HeightTextureCoord formats with a vertex-data offset;
// Depth-format instances have no heights (GetLiquidHeight returns 0).
type adtLiquidInstance struct {
	HasInstance   bool
	LiquidType    uint16
	Format        uint16
	OffsetX       uint8
	OffsetY       uint8
	Width         uint8
	Height        uint8
	ExistsBitmap  uint64
	VertexHeights []float32
	Deep          bool
}

func parseADT(data []byte) (adtInfo, error) {
	var info adtInfo
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return adtInfo{}, errors.New("truncated ADT chunk header")
		}
		name := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if size < 0 || size > len(data)-offset {
			return adtInfo{}, fmt.Errorf("invalid ADT chunk %q size %d", name, size)
		}
		chunk := data[offset : offset+size]
		switch name {
		case "MHDR":
			info.HasMHDR = true
			// C++ owner: adt_MHDR flags (map_extractor/adt.h) — bit 0
			// gates the MFBO flight-bounds leg in ConvertADT.
			if len(chunk) >= 4 {
				info.MHDRFlags = binary.LittleEndian.Uint32(chunk[0:])
			}
		case "MFBO":
			// C++ owner: adt_MFBO (map_extractor/adt.h) — max plane
			// then min plane, each int16[9]; 36 payload bytes.
			if len(chunk) < 36 {
				return adtInfo{}, fmt.Errorf("truncated ADT MFBO chunk: got %d, want %d", len(chunk), 36)
			}
			info.HasMFBO = true
			for index := 0; index < 9; index++ {
				info.FlightMax[index] = int16(binary.LittleEndian.Uint16(chunk[index*2:]))
				info.FlightMin[index] = int16(binary.LittleEndian.Uint16(chunk[18+index*2:]))
			}
		case "MCIN":
			info.HasMCIN = true
		case "MTEX":
			info.HasMTEX = true
		case "MMDX":
			info.HasMMDX = true
			info.DoodadNames = parseNameList(chunk)
		case "MMID":
			info.HasMMID = true
		case "MWMO":
			info.HasMWMO = true
			info.WorldModelNames = parseNameList(chunk)
		case "MWID":
			info.HasMWID = true
		case "MDDF":
			info.HasMDDF = true
			instances, err := parseMDDF(chunk)
			if err != nil {
				return adtInfo{}, err
			}
			start := len(info.Doodads)
			info.Doodads = append(info.Doodads, instances...)
			for index := range instances {
				info.InstanceOrder = append(info.InstanceOrder, adtModelInstanceRef{Doodad: true, Index: start + index})
			}
		case "MODF":
			info.HasMODF = true
			instances, err := parseMODF(chunk)
			if err != nil {
				return adtInfo{}, err
			}
			start := len(info.WorldModels)
			info.WorldModels = append(info.WorldModels, instances...)
			for index := range instances {
				info.InstanceOrder = append(info.InstanceOrder, adtModelInstanceRef{Index: start + index})
			}
		case "MH2O":
			info.MH2OCount++
			if err := parseMH2O(chunk, &info); err != nil {
				return adtInfo{}, err
			}
		case "MCNK":
			info.MCNKCount++
			if err := countADTSubchunks(chunk, &info); err != nil {
				return adtInfo{}, err
			}
		}
		offset += size
	}
	if info.MCNKCount == 0 {
		return adtInfo{}, errors.New("ADT MCNK chunks not found")
	}
	return info, nil
}

func parseNameList(data []byte) []string {
	result := make([]string, 0)
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != 0 {
			end++
		}
		// C++ owner: vmap4_extractor/adtfile.cpp:113-135 (MMDX) and :137-152
		// (MWMO) — the C++ pushes one vector entry per NUL-terminated
		// segment, including empty ones (p advances by strlen(p)+1 even when
		// strlen(p)==0). MDDF/MODF Id fields index that vector, so empty
		// segments must be preserved here to keep indices aligned; dropping
		// them would shift every later name lookup.
		result = append(result, string(data[start:end]))
		start = end + 1
	}
	return result
}

func parseMDDF(chunk []byte) ([]adtDoodadInstance, error) {
	const recordSize = 36
	if len(chunk)%recordSize != 0 {
		return nil, fmt.Errorf("invalid MDDF size %d", len(chunk))
	}
	instances := make([]adtDoodadInstance, len(chunk)/recordSize)
	for index := range instances {
		base := index * recordSize
		instance := &instances[index]
		instance.NameID = binary.LittleEndian.Uint32(chunk[base:])
		instance.UniqueID = binary.LittleEndian.Uint32(chunk[base+4:])
		for axis := 0; axis < 3; axis++ {
			instance.Position[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+8+axis*4:]))
			instance.Rotation[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+20+axis*4:]))
		}
		instance.Scale = float32(binary.LittleEndian.Uint16(chunk[base+32:])) / 1024.0
		instance.Flags = binary.LittleEndian.Uint16(chunk[base+34:])
	}
	return instances, nil
}

func parseMODF(chunk []byte) ([]adtWorldModelInstance, error) {
	const recordSize = 64
	if len(chunk)%recordSize != 0 {
		return nil, fmt.Errorf("invalid MODF size %d", len(chunk))
	}
	instances := make([]adtWorldModelInstance, len(chunk)/recordSize)
	for index := range instances {
		base := index * recordSize
		instance := &instances[index]
		instance.NameID = binary.LittleEndian.Uint32(chunk[base:])
		instance.UniqueID = binary.LittleEndian.Uint32(chunk[base+4:])
		for axis := 0; axis < 3; axis++ {
			instance.Position[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+8+axis*4:]))
			instance.Rotation[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+20+axis*4:]))
			instance.BoundsMin[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+32+axis*4:]))
			instance.BoundsMax[axis] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[base+44+axis*4:]))
		}
		instance.Flags = binary.LittleEndian.Uint16(chunk[base+56:])
		instance.DoodadSet = binary.LittleEndian.Uint16(chunk[base+58:])
		instance.NameSet = binary.LittleEndian.Uint16(chunk[base+60:])
		instance.Scale = float32(binary.LittleEndian.Uint16(chunk[base+62:])) / 1024.0
	}
	return instances, nil
}

func parseMH2O(chunk []byte, info *adtInfo) error {
	const headerBytes = 256 * 12
	if len(chunk) < headerBytes {
		return fmt.Errorf("truncated ADT MH2O chunk: got %d, want at least %d", len(chunk), headerBytes)
	}
	info.MH2OHeaders = 256
	for index := 0; index < 256; index++ {
		base := index * 12
		offsetInstances := binary.LittleEndian.Uint32(chunk[base:])
		layers := binary.LittleEndian.Uint32(chunk[base+4:])
		offsetAttributes := binary.LittleEndian.Uint32(chunk[base+8:])
		if layers == 0 {
			continue
		}
		info.LiquidLayers += int(layers)
		if offsetInstances == 0 || uint64(offsetInstances) >= uint64(len(chunk)) {
			return fmt.Errorf("invalid ADT MH2O instance offset %d", offsetInstances)
		}
		instancesEnd := uint64(offsetInstances) + uint64(layers)*24
		if instancesEnd > uint64(len(chunk)) {
			return fmt.Errorf("truncated ADT MH2O instances at offset %d", offsetInstances)
		}
		for layer := uint32(0); layer < layers; layer++ {
			base := int(offsetInstances + layer*24)
			lvf := binary.LittleEndian.Uint16(chunk[base+2:])
			width, height := chunk[base+14], chunk[base+15]
			// C++ performs no dimension validation here (adt.h MH2O
			// parsing); the old Go hard-error on 0 or >8 is dropped.
			info.LiquidInstances++
			info.LiquidTiles += int(width) * int(height)
			existsOffset := binary.LittleEndian.Uint32(chunk[base+16:])
			if existsOffset != 0 {
				bitmapBytes := (uint32(width)*uint32(height) + 7) / 8
				if uint64(existsOffset)+uint64(bitmapBytes) > uint64(len(chunk)) {
					return fmt.Errorf("invalid ADT MH2O exists bitmap offset %d", existsOffset)
				}
				info.LiquidExistsBytes += int(bitmapBytes)
			}
			vertexOffset := binary.LittleEndian.Uint32(chunk[base+20:])
			if vertexOffset != 0 {
				// C++ owner: LiquidVertexFormatType
				// (map_extractor/adt.h:176-181); per-vertex byte sizes
				// derived from the C++ accessors (adt.h:254-303):
				// HeightDepth(0)=4-byte height + 1-byte depth,
				// HeightTextureCoord(1)=4-byte height + 4-byte texcoord,
				// Depth(2)=1-byte depth; undefined formats default to
				// 0, they are not an error.
				bytesPerVertex := map[uint16]uint64{0: 5, 1: 8, 2: 1}[lvf]
				vertexBytes := uint64(width+1) * uint64(height+1) * bytesPerVertex
				if uint64(vertexOffset)+vertexBytes > uint64(len(chunk)) {
					return fmt.Errorf("invalid ADT MH2O vertex offset %d", vertexOffset)
				}
				info.LiquidVertexBytes += int(vertexBytes)
			}
			if layer == 0 {
				// ConvertADT reads only the FIRST instance per cell
				// (adt_MH2O::GetLiquidInstance, map_extractor/adt.h).
				// The MH2O header array is liquid[16][16] in file
				// order, so header index maps to cell (index/16,
				// index%16), the same (i, j) ConvertADT passes to
				// GetLiquidInstance.
				inst := &info.Liquid[index/16][index%16]
				inst.HasInstance = true
				inst.LiquidType = binary.LittleEndian.Uint16(chunk[base:])
				inst.Format = lvf
				inst.OffsetX = chunk[base+12]
				inst.OffsetY = chunk[base+13]
				inst.Width = width
				inst.Height = height
				existsOffset := binary.LittleEndian.Uint32(chunk[base+16:])
				if existsOffset == 0 {
					// adt_MH2O::GetLiquidExistsBitmap returns the
					// all-ones mask when OffsetExistsBitmap is 0.
					inst.ExistsBitmap = 0xFFFFFFFFFFFFFFFF
				} else {
					if uint64(existsOffset)+8 > uint64(len(chunk)) {
						return fmt.Errorf("invalid ADT MH2O exists bitmap offset %d", existsOffset)
					}
					inst.ExistsBitmap = binary.LittleEndian.Uint64(chunk[existsOffset:])
				}
				if vertexOffset != 0 && (lvf == 0 || lvf == 1) {
					// adt_MH2O::GetLiquidHeight reads the heights as
					// a contiguous float array at OffsetVertexData;
					// Depth format (2) has no heights (returns 0).
					count := int(width+1) * int(height+1)
					if uint64(vertexOffset)+uint64(count)*4 > uint64(len(chunk)) {
						return fmt.Errorf("invalid ADT MH2O vertex offset %d", vertexOffset)
					}
					inst.VertexHeights = make([]float32, count)
					for pos := range inst.VertexHeights {
						inst.VertexHeights[pos] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[int(vertexOffset)+pos*4:]))
					}
				}
				// adt_MH2O::GetLiquidAttributes: used cells with no
				// attribute offset get all-ones (Deep set); the Deep
				// flag is the second uint64 of the attributes block.
				if offsetAttributes != 0 {
					if uint64(offsetAttributes)+16 > uint64(len(chunk)) {
						return fmt.Errorf("invalid ADT MH2O attribute offset %d", offsetAttributes)
					}
					inst.Deep = binary.LittleEndian.Uint64(chunk[int(offsetAttributes)+8:]) != 0
				} else {
					inst.Deep = true
				}
			}
		}
		if offsetAttributes != 0 {
			if uint64(offsetAttributes)+16 > uint64(len(chunk)) {
				return fmt.Errorf("invalid ADT MH2O attribute offset %d", offsetAttributes)
			}
			info.LiquidAttributes++
		}
	}
	return nil
}

func countADTSubchunks(chunk []byte, info *adtInfo) error {
	const mcnkHeaderSize = 128
	// The cell fields read below run through ypos at payload offset 112
	// (C++ owner: adt_MCNK field layout in map_extractor/adt.h — ypos at
	// struct offset 120 = payload offset 112, sizeMCLQ at 108/100,
	// offsMCLQ at 104/96, holes at struct offset 68 = payload offset 60).
	if len(chunk) < 116 {
		return fmt.Errorf("truncated ADT MCNK chunk: got %d, want at least %d", len(chunk), 116)
	}
	cell := adtCellInfo{Flags: binary.LittleEndian.Uint32(chunk[0:]), X: binary.LittleEndian.Uint32(chunk[4:]), Y: binary.LittleEndian.Uint32(chunk[8:]), Layers: binary.LittleEndian.Uint32(chunk[12:]), DoodadRefs: binary.LittleEndian.Uint32(chunk[16:]), AreaID: binary.LittleEndian.Uint32(chunk[52:]), Holes: binary.LittleEndian.Uint32(chunk[60:]), MCLQSize: binary.LittleEndian.Uint32(chunk[100:]), YPos: math.Float32frombits(binary.LittleEndian.Uint32(chunk[112:]))}
	if cell.X >= 16 || cell.Y >= 16 {
		return fmt.Errorf("invalid ADT MCNK cell coordinates %d,%d", cell.X, cell.Y)
	}
	// offsMCLQ gates the MCLQ leg in ConvertADT (adt_MCNK::getMCLQ
	// returns nil when it is 0); keep it on the stack for the subchunk
	// loop below.
	offsMCLQ := binary.LittleEndian.Uint32(chunk[96:])
	info.Cells = append(info.Cells, cell)
	cellPtr := &info.Cells[len(info.Cells)-1]
	info.Grid[cell.Y][cell.X] = cellPtr
	if len(chunk) <= mcnkHeaderSize {
		return nil
	}
	for offset := mcnkHeaderSize; offset+8 <= len(chunk); {
		name := string(chunk[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(chunk[offset+4 : offset+8]))
		offset += 8
		if size < 0 || size > len(chunk)-offset {
			return fmt.Errorf("invalid ADT MCNK subchunk %q size %d", name, size)
		}
		switch name {
		case "MCVT":
			// C++ owner: map_extractor/adt.cpp:129-137 — MCVT must be
			// exactly 145 floats (580 bytes); oversized chunks are rejected.
			if size != 145*4 {
				return fmt.Errorf("invalid ADT MCVT subchunk: got %d, want %d", size, 145*4)
			}
			info.MCVTCount++
			previousHeights := info.MCVTHeights
			info.MCVTHeights += size / 4
			for index := 0; index < 145; index++ {
				value := math.Float32frombits(binary.LittleEndian.Uint32(chunk[offset+index*4:]))
				// ConvertADT adds MCVT deltas onto the ypos-seeded
				// V9/V8 grids; retain the samples on the cell.
				cellPtr.Heights[index] = value
				if previousHeights == 0 && index == 0 {
					info.HeightMin, info.HeightMax = value, value
				} else {
					if value < info.HeightMin {
						info.HeightMin = value
					}
					if value > info.HeightMax {
						info.HeightMax = value
					}
				}
			}
			cellPtr.HasMCVT = true
		case "MCLY":
			info.MCLYCount++
		case "MCAL":
			info.MCALCount++
		case "MCLQ":
			if size < 804 {
				return fmt.Errorf("truncated ADT MCLQ subchunk: got %d, want %d", size, 804)
			}
			info.MCLQCount++
			info.MCLQBytes += size
			if offsMCLQ != 0 {
				// C++ owner: adt_MCLQ (map_extractor/adt.h) — liquid
				// [9][9] of {light uint32, height float32} at payload
				// offset 8, flags [8][8] at offset 656. ConvertADT
				// reads only the heights and the flags.
				cellPtr.HasMCLQ = true
				for y := 0; y < 9; y++ {
					for x := 0; x < 9; x++ {
						cellPtr.MCLQLiquid[y][x] = math.Float32frombits(binary.LittleEndian.Uint32(chunk[offset+8+(y*9+x)*8+4:]))
					}
				}
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						cellPtr.MCLQFlags[y][x] = chunk[offset+656+y*8+x]
					}
				}
			}
		}
		offset += size
	}
	return nil
}

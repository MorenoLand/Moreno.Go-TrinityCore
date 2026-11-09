package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

type wmoRootInfo struct {
	Groups      uint32
	ID          uint32
	Flags       uint32
	DoodadNames map[uint32]string
	DoodadSets  []wmoDoodadSet
	Doodads     []wmoDoodad
}

// wmoLiquidHeader mirrors C++ WMOLiquidHeader (wmo.h, packed, 30 bytes).
type wmoLiquidHeader struct {
	Raw       []byte // 30 raw bytes as read, written back verbatim
	XVerts    int32
	YVerts    int32
	XTiles    int32
	YTiles    int32
	Heights   []float32 // LiquEx heights only (C++ writes height per vert)
	TileBytes []byte
}

type wmoDoodadSet struct {
	Name       string
	StartIndex uint32
	Count      uint32
}

type wmoDoodad struct {
	NameIndex uint32
	Position  [3]float32
	Rotation  [4]float32
	Scale     float32
	Color     uint32
}

type wmoGroupInfo struct {
	Flags      uint32
	ID         uint32
	Low        [3]float32
	High       [3]float32
	Branches   []uint32
	DoodadRefs []uint16
	Indices    []uint16
	Vertices   [][3]float32
	// C++ owner: wmo.cpp WMOGroup::open + ConvertToVMAPGroupWmo. Liquid is
	// the MOGP groupLiquid field after GetLiquidTypeId mapping; LiquFlags
	// bit 1 = MLIQ chunk present, bit 2 = Liquid != 0.
	Liquid     uint32
	LiquFlags  uint32
	LiquidData *wmoLiquidHeader
}

type wmoChunk struct {
	Name string
	Data []byte
}

func normalizeWMOName(name string) string {
	return strings.ToLower(filepath.ToSlash(name))
}

func isWMOGroupName(name string) bool {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if len(base) < 4 || base[len(base)-4] != '_' {
		return false
	}
	for _, value := range base[len(base)-3:] {
		if value < '0' || value > '9' {
			return false
		}
	}
	return strings.EqualFold(filepath.Ext(name), ".wmo")
}

func parseWMORoot(data []byte) (wmoRootInfo, error) {
	var root wmoRootInfo
	chunks, err := parseWMOChunks(data, false)
	if err != nil {
		return root, err
	}
	for _, chunk := range chunks {
		switch chunk.Name {
		case "MOHD":
			if len(chunk.Data) < 36 {
				return root, errors.New("truncated MOHD chunk")
			}
			root.Groups = binary.LittleEndian.Uint32(chunk.Data[4:8])
			root.ID = binary.LittleEndian.Uint32(chunk.Data[32:36])
			// C++ owner: wmo.cpp WMORoot::open reads the full 64-byte
			// MOHD (fields, bbcorn1/2, flags at offset 60). flags feeds
			// the WMOGroup::open groupLiquid rule.
			if len(chunk.Data) >= 64 {
				root.Flags = binary.LittleEndian.Uint32(chunk.Data[60:64])
			}
			if root.Groups > 65535 {
				return root, errors.New("WMO group count is unreasonable")
			}
		case "MODN":
			root.DoodadNames = parseWMONameOffsets(chunk.Data)
		case "MODS":
			sets, err := parseWMODoodadSets(chunk.Data)
			if err != nil {
				return root, err
			}
			root.DoodadSets = sets
		case "MODD":
			doodads, err := parseWMODoodads(chunk.Data)
			if err != nil {
				return root, err
			}
			root.Doodads = doodads
		}
	}
	if root.Groups == 0 && root.ID == 0 {
		return root, errors.New("MOHD chunk not found")
	}
	return root, nil
}

func parseWMONameOffsets(data []byte) map[uint32]string {
	names := make(map[uint32]string)
	for offset := 0; offset < len(data); {
		end := offset
		for end < len(data) && data[end] != 0 {
			end++
		}
		if end > offset {
			names[uint32(offset)] = string(data[offset:end])
		}
		offset = end + 1
	}
	return names
}

func parseWMODoodadSets(data []byte) ([]wmoDoodadSet, error) {
	const recordSize = 32
	if len(data)%recordSize != 0 {
		return nil, fmt.Errorf("invalid MODS size %d", len(data))
	}
	sets := make([]wmoDoodadSet, len(data)/recordSize)
	for index := range sets {
		base := index * recordSize
		name := data[base : base+20]
		if nul := bytes.IndexByte(name, 0); nul >= 0 {
			name = name[:nul]
		}
		sets[index] = wmoDoodadSet{Name: string(name), StartIndex: binary.LittleEndian.Uint32(data[base+20:]), Count: binary.LittleEndian.Uint32(data[base+24:])}
	}
	return sets, nil
}

func parseWMODoodads(data []byte) ([]wmoDoodad, error) {
	const recordSize = 40
	if len(data)%recordSize != 0 {
		return nil, fmt.Errorf("invalid MODD size %d", len(data))
	}
	doodads := make([]wmoDoodad, len(data)/recordSize)
	for index := range doodads {
		base := index * recordSize
		doodad := &doodads[index]
		doodad.NameIndex = binary.LittleEndian.Uint32(data[base:]) & 0x00FFFFFF
		for axis := 0; axis < 3; axis++ {
			doodad.Position[axis] = mathFloat32(data[base+4+axis*4:])
		}
		for axis := 0; axis < 4; axis++ {
			doodad.Rotation[axis] = mathFloat32(data[base+16+axis*4:])
		}
		doodad.Scale = mathFloat32(data[base+32:])
		doodad.Color = binary.LittleEndian.Uint32(data[base+36:])
	}
	return doodads, nil
}

// parseWMOGroup mirrors C++ WMOGroup::open (wmo.cpp). precise selects the
// C++ -l (large) conversion path: all triangles and vertices are kept
// verbatim. The default (false) path is C++ -s (small): only collision /
// render-face triangles survive, vertices are compacted and remapped.
func parseWMOGroup(data []byte, rootFlags uint32, precise bool) (wmoGroupInfo, error) {
	var group wmoGroupInfo
	chunks, err := parseWMOChunks(data, true)
	if err != nil {
		return group, err
	}
	var mopy []byte
	var movi []byte
	var movt []byte
	var moba []byte
	var rawLiquid uint32
	var liquid *wmoLiquidHeader
	for _, chunk := range chunks {
		switch chunk.Name {
		case "MOGP":
			if len(chunk.Data) < 60 {
				return group, errors.New("truncated MOGP chunk")
			}
			group.Flags = binary.LittleEndian.Uint32(chunk.Data[8:12])
			for i := 0; i < 3; i++ {
				group.Low[i] = mathFloat32(chunk.Data[12+i*4:])
				group.High[i] = mathFloat32(chunk.Data[24+i*4:])
			}
			group.ID = binary.LittleEndian.Uint32(chunk.Data[56:60])
			// C++ owner: wmo.cpp MOGP arm reads groupLiquid at offset 52.
			rawLiquid = binary.LittleEndian.Uint32(chunk.Data[52:56])
		case "MOPY":
			mopy = chunk.Data
		case "MOVI":
			movi = chunk.Data
		case "MOVT":
			movt = chunk.Data
		case "MOBA":
			moba = chunk.Data
		case "MODR":
			if len(chunk.Data)%2 != 0 {
				return group, errors.New("invalid MODR chunk size")
			}
			group.DoodadRefs = make([]uint16, len(chunk.Data)/2)
			for index := range group.DoodadRefs {
				group.DoodadRefs[index] = binary.LittleEndian.Uint16(chunk.Data[index*2:])
			}
		case "MLIQ":
			// C++ owner: wmo.cpp MLIQ arm. Packed header is 30 bytes
			// (4 int32 + 3 float32 + int16), then xverts*yverts 8-byte
			// verts (only heights are kept), then xtiles*ytiles bytes.
			if len(chunk.Data) < 30 {
				return group, errors.New("truncated MLIQ chunk")
			}
			liquid = &wmoLiquidHeader{Raw: chunk.Data[:30]}
			liquid.XVerts = int32(binary.LittleEndian.Uint32(chunk.Data[0:4]))
			liquid.YVerts = int32(binary.LittleEndian.Uint32(chunk.Data[4:8]))
			liquid.XTiles = int32(binary.LittleEndian.Uint32(chunk.Data[8:12]))
			liquid.YTiles = int32(binary.LittleEndian.Uint32(chunk.Data[12:16]))
			if liquid.XVerts < 0 || liquid.YVerts < 0 || liquid.XTiles < 0 || liquid.YTiles < 0 ||
				liquid.XVerts > 1024 || liquid.YVerts > 1024 || liquid.XTiles > 1024 || liquid.YTiles > 1024 {
				return group, errors.New("unreasonable MLIQ dimensions")
			}
			vertCount := int(liquid.XVerts) * int(liquid.YVerts)
			need := 30 + vertCount*8 + int(liquid.XTiles)*int(liquid.YTiles)
			if len(chunk.Data) < need {
				return group, errors.New("truncated MLIQ chunk")
			}
			liquid.Heights = make([]float32, vertCount)
			for i := range liquid.Heights {
				// WMOLiquidVert is 8 bytes: unk1 u16, unk2 u16, height f32.
				liquid.Heights[i] = mathFloat32(chunk.Data[30+i*8+4:])
			}
			liquid.TileBytes = chunk.Data[30+vertCount*8 : need]
		}
	}
	// C++ owner: wmo.cpp MOGP arm, applied after the chunk walk (MOGP is
	// first in practice, MLIQ may still refine it below).
	if rootFlags&4 != 0 {
		group.Liquid = getLiquidTypeID(rawLiquid, group.Flags)
	} else if rawLiquid == 15 {
		group.Liquid = 0
	} else {
		group.Liquid = getLiquidTypeID(rawLiquid+1, group.Flags)
	}
	if group.Liquid != 0 {
		group.LiquFlags |= 2
	}
	if liquid != nil {
		group.LiquFlags |= 1
		group.LiquidData = liquid
		// C++ owner: wmo.cpp MLIQ arm — only when the MOGP-derived
		// liquid is still 0, the first non-15 tile byte decides it.
		if group.Liquid == 0 {
			for _, b := range liquid.TileBytes {
				if b&0xF != 15 {
					group.Liquid = getLiquidTypeID(uint32(b&0xF)+1, group.Flags)
					break
				}
			}
			if group.Liquid != 0 {
				group.LiquFlags |= 2
			}
		}
	}
	if len(mopy) < 2 || len(movi) == 0 || len(movi)%2 != 0 || len(movt) == 0 || len(movt)%12 != 0 {
		return group, errors.New("WMO group geometry chunks are incomplete")
	}
	vertices := make([][3]float32, len(movt)/12)
	for i := range vertices {
		for axis := 0; axis < 3; axis++ {
			vertices[i][axis] = mathFloat32(movt[i*12+axis*4:])
		}
	}
	indices := make([]uint16, len(movi)/2)
	for i := range indices {
		indices[i] = binary.LittleEndian.Uint16(movi[i*2:])
	}
	if precise {
		// C++ owner: wmo.cpp ConvertToVMAPGroupWmo precise arm — every
		// triangle and vertex is written verbatim, no filtering.
		group.Vertices = vertices
		group.Indices = indices
	} else {
		used := make([]bool, len(vertices))
		filtered := make([]uint16, 0, len(indices))
		triangleCount := len(indices) / 3
		for triangle := 0; triangle < triangleCount && triangle*2+1 < len(mopy); triangle++ {
			flags := mopy[triangle*2]
			render := flags&0x20 != 0 && flags&0x04 == 0
			if flags&0x08 == 0 && !render {
				continue
			}
			for corner := 0; corner < 3; corner++ {
				index := indices[triangle*3+corner]
				if int(index) >= len(vertices) {
					return wmoGroupInfo{}, errors.New("WMO triangle index is out of range")
				}
				used[index] = true
				filtered = append(filtered, index)
			}
		}
		remap := make([]uint16, len(vertices))
		compact := make([][3]float32, 0)
		for index, present := range used {
			if present {
				remap[index] = uint16(len(compact))
				compact = append(compact, vertices[index])
			}
		}
		for index := range filtered {
			filtered[index] = remap[filtered[index]]
		}
		group.Vertices = compact
		group.Indices = filtered
	}
	if len(moba)%24 == 0 {
		group.Branches = make([]uint32, len(moba)/24)
		for index := range group.Branches {
			group.Branches[index] = uint32(binary.LittleEndian.Uint16(moba[index*24+16:]))
		}
	}
	return group, nil
}

// getLiquidTypeID mirrors C++ WMOGroup::GetLiquidTypeId (wmo.cpp). The
// (liquidTypeId-1)&3 switch maps legacy liquid ids onto the 13/14/19/20
// set; the 0x80000 mogp flag picks 14 over 13 for the first case.
func getLiquidTypeID(liquidTypeID, mogpFlags uint32) uint32 {
	if liquidTypeID < 21 && liquidTypeID != 0 {
		switch (uint8(liquidTypeID) - 1) & 3 {
		case 0:
			if mogpFlags&0x80000 != 0 {
				return 14
			}
			return 13
		case 1:
			return 14
		case 2:
			return 19
		case 3:
			return 20
		}
	}
	return liquidTypeID
}

// parseWMOChunks walks fourcc+size chunks. groupFile selects the C++
// WMOGroup::open quirk (wmo.cpp): a MOGP chunk's declared size is ignored
// and forced to 68 ("Fix sizeoff = Data size"), so the walk advances 68
// bytes past the MOGP header regardless of what the size field says.
func parseWMOChunks(data []byte, groupFile bool) ([]wmoChunk, error) {
	chunks := make([]wmoChunk, 0)
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, errors.New("truncated WMO chunk header")
		}
		name := canonicalWMOChunk(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4:]))
		if groupFile && name == "MOGP" {
			size = 68
		}
		offset += 8
		if size < 0 || size > len(data)-offset {
			return nil, fmt.Errorf("invalid WMO chunk %q size %d", name, size)
		}
		chunks = append(chunks, wmoChunk{Name: name, Data: data[offset : offset+size]})
		offset += size
	}
	return chunks, nil
}

func canonicalWMOChunk(raw []byte) string {
	direct := string(raw)
	reversed := string([]byte{raw[3], raw[2], raw[1], raw[0]})
	known := func(value string) bool {
		switch value {
		case "MOHD", "MOGP", "MOPY", "MOVI", "MOVT", "MOBA", "MLIQ", "MODN", "MODS", "MODD", "MODR":
			return true
		default:
			return false
		}
	}
	if known(direct) {
		return direct
	}
	return reversed
}

func mathFloat32(data []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data))
}

func writeRawWMO(root wmoRootInfo, groups []wmoGroupInfo) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("VMAP047")
	output.WriteByte(0)
	// C++ owner: vmapexport.cpp ExtractSingleWmo patches this slot (offset
	// 8) with the total triangle count across all groups once the groups
	// are written; ConvertToVMAPGroupWmo returns nColTriangles per group
	// (all triangles in precise mode). Writing 0 here starves the reader
	// of the count, so the patch is applied inline.
	totalTriangles := uint32(0)
	for _, group := range groups {
		totalTriangles += uint32(len(group.Indices) / 3)
	}
	_ = binary.Write(&output, binary.LittleEndian, totalTriangles)
	_ = binary.Write(&output, binary.LittleEndian, uint32(len(groups)))
	_ = binary.Write(&output, binary.LittleEndian, root.ID)
	for _, group := range groups {
		_ = binary.Write(&output, binary.LittleEndian, group.Flags)
		_ = binary.Write(&output, binary.LittleEndian, group.ID)
		_ = binary.Write(&output, binary.LittleEndian, group.Low)
		_ = binary.Write(&output, binary.LittleEndian, group.High)
		_ = binary.Write(&output, binary.LittleEndian, group.LiquFlags)
		writeRawChunk(&output, "GRP ", uint32(4+len(group.Branches)*4))
		_ = binary.Write(&output, binary.LittleEndian, uint32(len(group.Branches)))
		_ = binary.Write(&output, binary.LittleEndian, group.Branches)
		writeRawChunk(&output, "INDX", uint32(4+len(group.Indices)*2))
		_ = binary.Write(&output, binary.LittleEndian, uint32(len(group.Indices)))
		_ = binary.Write(&output, binary.LittleEndian, group.Indices)
		writeRawChunk(&output, "VERT", uint32(4+len(group.Vertices)*12))
		_ = binary.Write(&output, binary.LittleEndian, uint32(len(group.Vertices)))
		_ = binary.Write(&output, binary.LittleEndian, group.Vertices)
		// C++ owner: wmo.cpp ConvertToVMAPGroupWmo LIQU arm — written
		// whenever either liquflag bit is set. Total size is 4 (liquid
		// id) plus, when an MLIQ chunk was present, the 30-byte header,
		// one height float per liquid vert, and the tile bytes.
		if group.LiquFlags&3 != 0 {
			totalSize := uint32(4)
			if group.LiquFlags&1 != 0 && group.LiquidData != nil {
				totalSize += uint32(30 + len(group.LiquidData.Heights)*4 + len(group.LiquidData.TileBytes))
			}
			writeRawChunk(&output, "LIQU", totalSize)
			_ = binary.Write(&output, binary.LittleEndian, group.Liquid)
			if group.LiquFlags&1 != 0 && group.LiquidData != nil {
				_, _ = output.Write(group.LiquidData.Raw)
				for _, height := range group.LiquidData.Heights {
					_ = binary.Write(&output, binary.LittleEndian, height)
				}
				_, _ = output.Write(group.LiquidData.TileBytes)
			}
		}
	}
	var metadata bytes.Buffer
	metadata.WriteString("MCWM")
	_ = binary.Write(&metadata, binary.LittleEndian, uint32(1))
	offsets := make([]uint32, 0, len(root.DoodadNames))
	for offset := range root.DoodadNames {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	_ = binary.Write(&metadata, binary.LittleEndian, uint32(len(offsets)))
	for _, offset := range offsets {
		name := root.DoodadNames[offset]
		_ = binary.Write(&metadata, binary.LittleEndian, offset)
		_ = binary.Write(&metadata, binary.LittleEndian, uint32(len(name)))
		_, _ = metadata.WriteString(name)
	}
	_ = binary.Write(&metadata, binary.LittleEndian, uint32(len(root.DoodadSets)))
	for _, set := range root.DoodadSets {
		_ = binary.Write(&metadata, binary.LittleEndian, set.StartIndex)
		_ = binary.Write(&metadata, binary.LittleEndian, set.Count)
	}
	_ = binary.Write(&metadata, binary.LittleEndian, uint32(len(root.Doodads)))
	for _, doodad := range root.Doodads {
		_ = binary.Write(&metadata, binary.LittleEndian, doodad.NameIndex)
		_ = binary.Write(&metadata, binary.LittleEndian, doodad.Position)
		_ = binary.Write(&metadata, binary.LittleEndian, doodad.Rotation)
		_ = binary.Write(&metadata, binary.LittleEndian, doodad.Scale)
		_ = binary.Write(&metadata, binary.LittleEndian, doodad.Color)
	}
	refs := make(map[uint16]struct{})
	for _, group := range groups {
		for _, ref := range group.DoodadRefs {
			refs[ref] = struct{}{}
		}
	}
	orderedRefs := make([]uint16, 0, len(refs))
	for ref := range refs {
		orderedRefs = append(orderedRefs, ref)
	}
	sort.Slice(orderedRefs, func(i, j int) bool { return orderedRefs[i] < orderedRefs[j] })
	_ = binary.Write(&metadata, binary.LittleEndian, uint32(len(orderedRefs)))
	_ = binary.Write(&metadata, binary.LittleEndian, orderedRefs)
	writeRawChunk(&output, "DODM", uint32(metadata.Len()))
	_, _ = output.Write(metadata.Bytes())
	return output.Bytes(), nil
}

func writeRawChunk(output *bytes.Buffer, name string, size uint32) {
	output.WriteString(name)
	_ = binary.Write(output, binary.LittleEndian, size)
}

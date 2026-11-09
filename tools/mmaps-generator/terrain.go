// Terrain data loading port: TerrainBuilder (src/tools/mmaps_generator/
// TerrainBuilder.cpp/.h) — .map tile parsing into triangle soup, hole and
// liquid handling, vertex cleaning, off-mesh connections.
//
// loadVMap is NOT ported yet: it needs a Go StaticMapTree/.vmtree reader
// plus WorldModel/GroupModel parsing (VMapManager2, MapTree, ModelInstance),
// which belongs with the vmap4extractor parity work (checkpoint unit 3).
// LoadVMap currently reports no data; the pipeline proceeds with terrain
// only until that reader lands.
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Map grid constants (TerrainBuilder.h).
const (
	v9Size       = 129
	v9SizeSq     = v9Size * v9Size
	v8Size       = 128
	v8SizeSq     = v8Size * v8Size
	gridPartSize = moveMapGridSize / v8Size

	invalidMapLiqHeight    = -500.0
	invalidMapLiqHeightMax = 5000.0
)

// .map file section flags (TerrainBuilder.cpp).
const (
	mapHeightNoHeight = 0x0001
	mapHeightAsInt16  = 0x0002
	mapHeightAsInt8   = 0x0004

	mapLiquidNoType   = 0x0001
	mapLiquidNoHeight = 0x0002

	mapLiquidTypeNoWater   = 0x00
	mapLiquidTypeWater     = 0x01
	mapLiquidTypeOcean     = 0x02
	mapLiquidTypeMagma     = 0x04
	mapLiquidTypeSlime     = 0x08
	mapLiquidTypeDarkWater = 0x10
)

// MeshData mirrors MMAP::MeshData (TerrainBuilder.h). Vertices are stored
// Recast-order (x, height, y) triples.
type MeshData struct {
	SolidVerts  []float32
	SolidTris   []int32
	LiquidVerts []float32
	LiquidTris  []int32
	LiquidType  []uint8

	// Off-mesh connection data, per connection.
	OffMeshConnections      []float32 // [p0y,p0z,p0x,p1y,p1z,p1x]
	OffMeshConnectionRads   []float32
	OffMeshConnectionDirs   []uint8
	OffMeshConnectionsAreas []uint8
	OffMeshConnectionsFlags []uint16
}

type spot int

const (
	spotTop    spot = 1
	spotRight  spot = 2
	spotLeft   spot = 3
	spotBottom spot = 4
	spotEntire spot = 5
)

type gridKind int

const (
	gridV8 gridKind = iota
	gridV9
)

// TerrainBuilder mirrors MMAP::TerrainBuilder (TerrainBuilder.h).
type TerrainBuilder struct {
	skipLiquid bool
	mapsDir    string
	vmapsDir   string
}

func newTerrainBuilder(skipLiquid bool, mapsDir, vmapsDir string) *TerrainBuilder {
	return &TerrainBuilder{skipLiquid: skipLiquid, mapsDir: mapsDir, vmapsDir: vmapsDir}
}

// UsesLiquids mirrors TerrainBuilder::usesLiquids.
func (tb *TerrainBuilder) UsesLiquids() bool { return !tb.skipLiquid }

// LoadVMap is the vmap half of TerrainBuilder::loadVMap. NOT ported yet: it
// needs a Go StaticMapTree/.vmtree reader plus WorldModel/GroupModel parsing
// (VMapManager2, MapTree, ModelInstance in the C++ tree). Returns false so
// callers proceed with terrain-only data.
func (tb *TerrainBuilder) LoadVMap(mapID, tileX, tileY uint32, md *MeshData) bool {
	return false
}

// LoadMap mirrors TerrainBuilder::loadMap(mapID, tileX, tileY, meshData):
// the whole tile plus one-cell edge strips from the four neighbours.
func (tb *TerrainBuilder) LoadMap(mapID, tileX, tileY uint32, md *MeshData) {
	if tb.loadMap(mapID, tileX, tileY, md, spotEntire) {
		tb.loadMap(mapID, tileX+1, tileY, md, spotLeft)
		tb.loadMap(mapID, tileX-1, tileY, md, spotRight)
		tb.loadMap(mapID, tileX, tileY+1, md, spotTop)
		tb.loadMap(mapID, tileX, tileY-1, md, spotBottom)
	}
}

func (tb *TerrainBuilder) getLoopVars(portion spot) (loopStart, loopEnd, loopInc int) {
	switch portion {
	case spotEntire:
		return 0, v8SizeSq, 1
	case spotTop:
		return 0, v8Size, 1
	case spotLeft:
		return 0, v8SizeSq - v8Size + 1, v8Size
	case spotRight:
		return v8Size - 1, v8SizeSq, v8Size
	case spotBottom:
		return v8SizeSq - v8Size, v8SizeSq, 1
	}
	return 0, 0, 1
}

type mapFileHeader struct {
	MapMagic        uint32
	VersionMagic    uint32
	BuildMagic      uint32
	AreaMapOffset   uint32
	AreaMapSize     uint32
	HeightMapOffset uint32
	HeightMapSize   uint32
	LiquidMapOffset uint32
	LiquidMapSize   uint32
	HolesOffset     uint32
	HolesSize       uint32
}

type mapHeightHeader struct {
	Fourcc        uint32
	Flags         uint32
	GridHeight    float32
	GridMaxHeight float32
}

type mapLiquidHeader struct {
	Fourcc      uint32
	Flags       uint8
	LiquidFlags uint8
	LiquidType  uint16
	OffsetX     uint8
	OffsetY     uint8
	Width       uint8
	Height      uint8
	LiquidLevel float32
}

func (tb *TerrainBuilder) loadMap(mapID, tileX, tileY uint32, md *MeshData, portion spot) bool {
	mapFileName := filepath.Join(tb.mapsDir, fmt.Sprintf("%03d%02d%02d.map", mapID, tileY, tileX))

	mapFile, err := os.Open(mapFileName)
	if err != nil {
		return false
	}
	defer mapFile.Close()

	var fheader mapFileHeader
	if err := binary.Read(mapFile, binary.LittleEndian, &fheader); err != nil ||
		fheader.VersionMagic != binary.LittleEndian.Uint32([]byte("v1.9")) {
		fmt.Printf("%s is the wrong version, please extract new .map files\n", mapFileName)
		return false
	}

	var hheader mapHeightHeader
	if _, err := mapFile.Seek(int64(fheader.HeightMapOffset), 0); err != nil {
		return false
	}

	haveTerrain := false
	haveLiquid := false
	if err := binary.Read(mapFile, binary.LittleEndian, &hheader); err == nil {
		haveTerrain = hheader.Flags&mapHeightNoHeight == 0
		haveLiquid = fheader.LiquidMapOffset != 0 && !tb.skipLiquid
	}

	// No data in this map file.
	if !haveTerrain && !haveLiquid {
		return false
	}

	var holes [16][16]uint16
	var liquidEntry [16][16]uint16
	var liquidFlags [16][16]uint8
	var ltriangles []int32
	var ttriangles []int32

	// Terrain data.
	if haveTerrain {
		v9 := make([]float32, v9SizeSq)
		v8 := make([]float32, v8SizeSq)
		expected := v9SizeSq + v8SizeSq

		if hheader.Flags&mapHeightAsInt8 != 0 {
			v9b := make([]uint8, v9SizeSq)
			v8b := make([]uint8, v8SizeSq)
			count := 0
			n, _ := mapFile.Read(v9b)
			count += n
			n, _ = mapFile.Read(v8b)
			count += n
			if count != expected {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected %d, read %d\n", expected, count)
			}
			heightMultiplier := (hheader.GridMaxHeight - hheader.GridHeight) / 255
			for i := 0; i < v9SizeSq; i++ {
				v9[i] = float32(v9b[i])*heightMultiplier + hheader.GridHeight
			}
			for i := 0; i < v8SizeSq; i++ {
				v8[i] = float32(v8b[i])*heightMultiplier + hheader.GridHeight
			}
		} else if hheader.Flags&mapHeightAsInt16 != 0 {
			v9s := make([]uint16, v9SizeSq)
			v8s := make([]uint16, v8SizeSq)
			count := 0
			var tmp [2]byte
			readU16 := func(dst []uint16) int {
				n := 0
				for i := range dst {
					if _, err := mapFile.Read(tmp[:]); err != nil {
						break
					}
					dst[i] = binary.LittleEndian.Uint16(tmp[:])
					n++
				}
				return n
			}
			count += readU16(v9s)
			count += readU16(v8s)
			if count != expected {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected %d, read %d\n", expected, count)
			}
			heightMultiplier := (hheader.GridMaxHeight - hheader.GridHeight) / 65535
			for i := 0; i < v9SizeSq; i++ {
				v9[i] = float32(v9s[i])*heightMultiplier + hheader.GridHeight
			}
			for i := 0; i < v8SizeSq; i++ {
				v8[i] = float32(v8s[i])*heightMultiplier + hheader.GridHeight
			}
		} else {
			count := 0
			var tmp [4]byte
			readF32 := func(dst []float32) int {
				n := 0
				for i := range dst {
					if _, err := mapFile.Read(tmp[:]); err != nil {
						break
					}
					dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(tmp[:]))
					n++
				}
				return n
			}
			count += readF32(v9)
			count += readF32(v8)
			if count != expected {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected %d, read %d\n", expected, count)
			}
		}

		// Hole data.
		if fheader.HolesSize != 0 {
			if _, err := mapFile.Seek(int64(fheader.HolesOffset), 0); err == nil {
				toRead := int(fheader.HolesSize)
				if toRead > 512 {
					toRead = 512
				}
				buf := make([]byte, toRead)
				if n, _ := mapFile.Read(buf); n != toRead {
					fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected 1, read 0\n")
				}
				for i := 0; i < toRead/2; i++ {
					holes[i/16][i%16] = binary.LittleEndian.Uint16(buf[i*2:])
				}
			}
		}

		count := len(md.SolidVerts) / 3
		xoffset := (float32(tileX) - 32) * moveMapGridSize
		yoffset := (float32(tileY) - 32) * moveMapGridSize

		for i := 0; i < v9SizeSq; i++ {
			coord := tb.getHeightCoord(i, gridV9, xoffset, yoffset, v9)
			md.SolidVerts = append(md.SolidVerts, coord[0], coord[2], coord[1])
		}
		for i := 0; i < v8SizeSq; i++ {
			coord := tb.getHeightCoord(i, gridV8, xoffset, yoffset, v8)
			md.SolidVerts = append(md.SolidVerts, coord[0], coord[2], coord[1])
		}

		loopStart, loopEnd, loopInc := tb.getLoopVars(portion)
		for i := loopStart; i < loopEnd; i += loopInc {
			for j := spotTop; j <= spotBottom; j++ {
				indices := tb.getHeightTriangle(i, j, false)
				ttriangles = append(ttriangles, int32(indices[2]+count), int32(indices[1]+count), int32(indices[0]+count))
			}
		}
	}

	// Liquid data.
	if haveLiquid {
		var lheader mapLiquidHeader
		if _, err := mapFile.Seek(int64(fheader.LiquidMapOffset), 0); err != nil {
			return false
		}
		if err := binary.Read(mapFile, binary.LittleEndian, &lheader); err != nil {
			fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected 1, read 0\n")
		}

		var liquidMap []float32

		if lheader.Flags&mapLiquidNoType == 0 {
			buf := make([]byte, 512)
			if n, _ := mapFile.Read(buf); n != 512 {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected 1, read 0\n")
			}
			for i := 0; i < 256; i++ {
				liquidEntry[i/16][i%16] = binary.LittleEndian.Uint16(buf[i*2:])
			}
			buf2 := make([]byte, 256)
			if n, _ := mapFile.Read(buf2); n != 256 {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected 1, read 0\n")
			}
			for i := 0; i < 256; i++ {
				liquidFlags[i/16][i%16] = buf2[i]
			}
		} else {
			for r := 0; r < 16; r++ {
				for c := 0; c < 16; c++ {
					liquidEntry[r][c] = lheader.LiquidType
					liquidFlags[r][c] = lheader.LiquidFlags
				}
			}
		}

		if lheader.Flags&mapLiquidNoHeight == 0 {
			toRead := int(lheader.Width) * int(lheader.Height)
			liquidMap = make([]float32, toRead)
			var tmp [4]byte
			read := 0
			for i := 0; i < toRead; i++ {
				if _, err := mapFile.Read(tmp[:]); err != nil {
					break
				}
				liquidMap[i] = math.Float32frombits(binary.LittleEndian.Uint32(tmp[:]))
				read++
			}
			if read != toRead {
				fmt.Printf("TerrainBuilder::loadMap: Failed to read some data expected 1, read 0\n")
				liquidMap = nil
			}
		}

		count := len(md.LiquidVerts) / 3
		xoffset := (float32(tileX) - 32) * moveMapGridSize
		yoffset := (float32(tileY) - 32) * moveMapGridSize

		// Generate coordinates. A short liquid-height read falls back to the
		// flat liquid level instead of dereferencing a nil map.
		if lheader.Flags&mapLiquidNoHeight == 0 && liquidMap != nil {
			j := 0
			for i := 0; i < v9SizeSq; i++ {
				row := i / v9Size
				col := i % v9Size
				if row < int(lheader.OffsetY) || row >= int(lheader.OffsetY)+int(lheader.Height) ||
					col < int(lheader.OffsetX) || col >= int(lheader.OffsetX)+int(lheader.Width) {
					// Dummy vert using invalid height.
					md.LiquidVerts = append(md.LiquidVerts, (xoffset+float32(col)*gridPartSize)*-1, invalidMapLiqHeight, (yoffset+float32(row)*gridPartSize)*-1)
					continue
				}
				coord := tb.getLiquidCoord(i, j, xoffset, yoffset, liquidMap)
				md.LiquidVerts = append(md.LiquidVerts, coord[0], coord[2], coord[1])
				j++
			}
		} else {
			for i := 0; i < v9SizeSq; i++ {
				row := i / v9Size
				col := i % v9Size
				md.LiquidVerts = append(md.LiquidVerts, (xoffset+float32(col)*gridPartSize)*-1, lheader.LiquidLevel, (yoffset+float32(row)*gridPartSize)*-1)
			}
		}

		loopStart, loopEnd, loopInc := tb.getLoopVars(portion)
		triInc := int(spotBottom - spotTop)
		for i := loopStart; i < loopEnd; i += loopInc {
			for j := spotTop; j <= spotBottom; j += spot(triInc) {
				indices := tb.getHeightTriangle(i, j, true)
				ltriangles = append(ltriangles, int32(indices[2]+count), int32(indices[1]+count), int32(indices[0]+count))
			}
		}
	}

	// Now that we have gathered the data, we can figure out which parts to keep:
	// liquid above ground, ground above liquid.
	loopStart, loopEnd, loopInc := tb.getLoopVars(portion)
	const tTriCount = 4

	lverts := md.LiquidVerts
	ltris := ltriangles
	tverts := md.SolidVerts
	ttris := ttriangles

	if len(ltriangles)+len(ttriangles) == 0 {
		return false
	}

	// Make a copy of liquid vertices, used to pad right-bottom frame due to
	// lost vertex data at extraction.
	var lvertsCopy []float32
	if len(md.LiquidVerts) > 0 {
		lvertsCopy = make([]float32, len(md.LiquidVerts))
		copy(lvertsCopy, lverts)
	}

	lcur, tcur := 0, 0
	for i := loopStart; i < loopEnd; i += loopInc {
		for j := 0; j < 2; j++ {
			// Default is true, will change to false if needed.
			useTerrain := true
			useLiquid := true
			liquidType := uint8(mapLiquidTypeNoWater)

			// If there is no liquid, don't use liquid.
			if len(md.LiquidVerts) == 0 || len(ltriangles) == 0 {
				useLiquid = false
			} else {
				liquidType = tb.getLiquidType(i, liquidFlags)
				if liquidType&mapLiquidTypeDarkWater != 0 {
					// Players should not be here, so logically neither should creatures.
					useTerrain = false
					useLiquid = false
				} else if liquidType&(mapLiquidTypeWater|mapLiquidTypeOcean) != 0 {
					liquidType = navAreaWater
				} else if liquidType&(mapLiquidTypeMagma|mapLiquidTypeSlime) != 0 {
					liquidType = navAreaMagmaSlime
				} else {
					useLiquid = false
				}
			}

			// If there is no terrain, don't use terrain.
			if len(ttriangles) == 0 {
				useTerrain = false
			}

			// While extracting ADT data we are losing right-bottom vertices;
			// this code adds fair approximation of lost data.
			if useLiquid {
				quadHeight := float32(0)
				var validCount uint32
				for idx := 0; idx < 3; idx++ {
					h := lvertsCopy[ltris[lcur+idx]*3+1]
					if h != invalidMapLiqHeight && h < invalidMapLiqHeightMax {
						quadHeight += h
						validCount++
					}
				}

				// Update vertex height data.
				if validCount > 0 && validCount < 3 {
					quadHeight /= float32(validCount)
					for idx := 0; idx < 3; idx++ {
						h := lverts[ltris[lcur+idx]*3+1]
						if h == invalidMapLiqHeight || h > invalidMapLiqHeightMax {
							lverts[ltris[lcur+idx]*3+1] = quadHeight
						}
					}
				}

				// No valid vertexes - don't use this poly at all.
				if validCount == 0 {
					useLiquid = false
				}
			}

			// If there is a hole here, don't use the terrain.
			if useTerrain && fheader.HolesSize != 0 {
				useTerrain = !tb.isHole(i, holes)
			}

			// We use only one terrain kind per quad - pick higher one.
			if useTerrain && useLiquid {
				minLLevel := float32(invalidMapLiqHeightMax)
				maxLLevel := float32(invalidMapLiqHeight)
				for x := 0; x < 3; x++ {
					h := lverts[ltris[lcur+x]*3+1]
					if minLLevel > h {
						minLLevel = h
					}
					if maxLLevel < h {
						maxLLevel = h
					}
				}

				maxTLevel := float32(invalidMapLiqHeight)
				minTLevel := float32(invalidMapLiqHeightMax)
				for x := 0; x < 6; x++ {
					h := tverts[ttris[tcur+x]*3+1]
					if maxTLevel < h {
						maxTLevel = h
					}
					if minTLevel > h {
						minTLevel = h
					}
				}

				// Terrain under the liquid?
				if minLLevel > maxTLevel {
					useTerrain = false
				}
				// Liquid under the terrain?
				if minTLevel > maxLLevel {
					useLiquid = false
				}
			}

			// Store the result.
			if useLiquid {
				md.LiquidType = append(md.LiquidType, liquidType)
				for k := 0; k < 3; k++ {
					md.LiquidTris = append(md.LiquidTris, ltris[lcur+k])
				}
			}

			if useTerrain {
				for k := 0; k < 3*tTriCount/2; k++ {
					md.SolidTris = append(md.SolidTris, ttris[tcur+k])
				}
			}

			// Advance to next set of triangles.
			lcur += 3
			tcur += 3 * tTriCount / 2
		}
	}

	return len(md.SolidTris) > 0 || len(md.LiquidTris) > 0
}

func (tb *TerrainBuilder) getHeightCoord(index int, grid gridKind, xOffset, yOffset float32, v []float32) [3]float32 {
	// Wow coords: x, y, height. Coord is mirrored about the horizontal axes.
	var coord [3]float32
	switch grid {
	case gridV9:
		coord[0] = (xOffset + float32(index%v9Size)*gridPartSize) * -1
		coord[1] = (yOffset + float32(index/v9Size)*gridPartSize) * -1
		coord[2] = v[index]
	case gridV8:
		coord[0] = (xOffset + float32(index%v8Size)*gridPartSize + gridPartSize/2) * -1
		coord[1] = (yOffset + float32(index/v8Size)*gridPartSize + gridPartSize/2) * -1
		coord[2] = v[index]
	}
	return coord
}

func (tb *TerrainBuilder) getHeightTriangle(square int, triangle spot, liquid bool) [3]int {
	var indices [3]int
	rowOffset := square / v8Size
	if !liquid {
		switch triangle {
		case spotTop:
			indices[0] = square + rowOffset
			indices[1] = square + 1 + rowOffset
			indices[2] = v9SizeSq + square
		case spotLeft:
			indices[0] = square + rowOffset
			indices[1] = v9SizeSq + square
			indices[2] = square + v9Size + rowOffset
		case spotRight:
			indices[0] = square + 1 + rowOffset
			indices[1] = square + v9Size + 1 + rowOffset
			indices[2] = v9SizeSq + square
		case spotBottom:
			indices[0] = v9SizeSq + square
			indices[1] = square + v9Size + 1 + rowOffset
			indices[2] = square + v9Size + rowOffset
		}
	} else {
		switch triangle {
		case spotTop:
			indices[0] = square + rowOffset
			indices[1] = square + 1 + rowOffset
			indices[2] = square + v9Size + 1 + rowOffset
		case spotBottom:
			indices[0] = square + rowOffset
			indices[1] = square + v9Size + 1 + rowOffset
			indices[2] = square + v9Size + rowOffset
		}
	}
	return indices
}

func (tb *TerrainBuilder) getLiquidCoord(index, index2 int, xOffset, yOffset float32, v []float32) [3]float32 {
	var coord [3]float32
	// Wow coords: x, y, height. Coord is mirrored about the horizontal axes.
	coord[0] = (xOffset + float32(index%v9Size)*gridPartSize) * -1
	coord[1] = (yOffset + float32(index/v9Size)*gridPartSize) * -1
	coord[2] = v[index2]
	return coord
}

var holetabH = [4]uint16{0x1111, 0x2222, 0x4444, 0x8888}
var holetabV = [4]uint16{0x000F, 0x00F0, 0x0F00, 0xF000}

func (tb *TerrainBuilder) isHole(square int, holes [16][16]uint16) bool {
	row := square / 128
	col := square % 128
	cellRow := row / 8 // 8 squares per cell
	cellCol := col / 8
	holeRow := row % 8 / 2
	holeCol := (square - (row*128 + cellCol*8)) / 2

	hole := holes[cellRow][cellCol]
	return hole&holetabH[holeCol]&holetabV[holeRow] != 0
}

func (tb *TerrainBuilder) getLiquidType(square int, liquidType [16][16]uint8) uint8 {
	row := square / 128
	col := square % 128
	cellRow := row / 8 // 8 squares per cell
	cellCol := col / 8
	return liquidType[cellRow][cellCol]
}

// CleanVertices mirrors TerrainBuilder::cleanVertices: drops vertices no
// triangle references and remaps triangle indices.
func CleanVertices(verts *[]float32, tris *[]int32) {
	vertMap := make(map[int]int)
	v := *verts
	t := *tris

	var cleanVerts []float32
	count := 0
	for i := 0; i < len(t); i++ {
		if _, ok := vertMap[int(t[i])]; ok {
			continue
		}
		index := int(t[i])
		vertMap[index] = count
		cleanVerts = append(cleanVerts, v[index*3], v[index*3+1], v[index*3+2])
		count++
	}

	*verts = cleanVerts

	// Update triangles to use new indices.
	for i := 0; i < len(t); i++ {
		if idx, ok := vertMap[int(t[i])]; ok {
			t[i] = int32(idx)
		}
	}
}

// LoadOffMeshConnections mirrors TerrainBuilder::loadOffMeshConnections:
// parses "<mapID> <tileX>,<tileY> (x y z) (x y z) <radius>" lines.
func (tb *TerrainBuilder) LoadOffMeshConnections(mapID, tileX, tileY uint32, md *MeshData, offMeshFilePath string) {
	if offMeshFilePath == "" {
		return
	}
	fp, err := os.Open(offMeshFilePath)
	if err != nil {
		fmt.Printf(" loadOffMeshConnections:: input file %s not found!\n", offMeshFilePath)
		return
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	scanner.Buffer(make([]byte, 512), 512)
	for scanner.Scan() {
		var mid, tx, ty uint32
		var p0, p1 [3]float32
		var size float32
		n, err := fmt.Sscanf(scanner.Text(), "%d %d,%d (%g %g %g) (%g %g %g) %g",
			&mid, &tx, &ty, &p0[0], &p0[1], &p0[2], &p1[0], &p1[1], &p1[2], &size)
		if err != nil || n != 10 {
			continue
		}
		if mapID == mid && tileX == tx && tileY == ty {
			md.OffMeshConnections = append(md.OffMeshConnections, p0[1], p0[2], p0[0])
			md.OffMeshConnections = append(md.OffMeshConnections, p1[1], p1[2], p1[0])
			md.OffMeshConnectionDirs = append(md.OffMeshConnectionDirs, 1) // 1 - both directions
			md.OffMeshConnectionRads = append(md.OffMeshConnectionRads, size)
			md.OffMeshConnectionsAreas = append(md.OffMeshConnectionsAreas, 0xFF)
			md.OffMeshConnectionsFlags = append(md.OffMeshConnectionsFlags, 0xFF)
		}
	}
}

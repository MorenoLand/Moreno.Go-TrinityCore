// VMap (.vmtree/.vmtile/.vmo) file reading port.
//
// C++ owners: VMapManager2 (src/common/Collision/Management/VMapManager2.cpp),
// StaticMapTree (src/common/Collision/Maps/MapTree.cpp), ModelSpawn /
// ModelInstance (src/common/Collision/Models/ModelInstance.cpp), WorldModel /
// GroupModel / WmoLiquid (src/common/Collision/Models/WorldModel.cpp), BIH
// (src/common/Collision/BoundingIntervalHierarchy.cpp). The on-disk layout
// matches the vmap4_assembler writer (src/common/Collision/Maps/
// TileAssembler.cpp): .vmtree is VMAP_MAGIC + tiled byte + NODE BIH + GOBJ
// (+ one global spawn when untiled), .vmtile is VMAP_MAGIC + spawn count +
// (ModelSpawn + tree index) per spawn, .vmo is VMAP_MAGIC + WMOD + GMOD
// groups + GBIH.
//
// The reader covers exactly the path TerrainBuilder::loadVMap needs: the
// tile's model spawns and their group triangles. The BIH query machinery
// (ray/point intersection, isInLineOfSight, getHeight) is NOT ported —
// loadVMap never queries it, it iterates every loaded instance and appends
// all of its triangles. The BIH chunks are parsed only to skip them and to
// learn the tree's primitive count.
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// VMAP_MAGIC (VMapDefinitions.h).
const vmapMagic = "VMAP_4.7"

// ModelSpawn flags (ModelInstance.h).
const (
	modM2        = 1
	modHasBound  = 1 << 2
	spawnNameMax = 500
)

// vec3 mirrors G3D::Vector3: three consecutive little-endian floats.
type vec3 [3]float32

// matrix3 mirrors G3D::Matrix3 (dep/g3dlite): row-major elt[row][col].
type matrix3 [3][3]float32

func (m matrix3) mul(o matrix3) matrix3 {
	var out matrix3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			out[r][c] = m[r][0]*o[0][c] + m[r][1]*o[1][c] + m[r][2]*o[2][c]
		}
	}
	return out
}

// vecMul mirrors G3D's Vector3 * Matrix3: v * M == M.transpose() * v
// (Matrix3.h, trailing inline operator*).
func (v vec3) vecMul(m matrix3) vec3 {
	var out vec3
	for r := 0; r < 3; r++ {
		out[r] = v[0]*m[0][r] + v[1]*m[1][r] + v[2]*m[2][r]
	}
	return out
}

func rotX(a float32) matrix3 {
	c := float32(math.Cos(float64(a)))
	s := float32(math.Sin(float64(a)))
	return matrix3{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

func rotY(a float32) matrix3 {
	c := float32(math.Cos(float64(a)))
	s := float32(math.Sin(float64(a)))
	return matrix3{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
}

func rotZ(a float32) matrix3 {
	c := float32(math.Cos(float64(a)))
	s := float32(math.Sin(float64(a)))
	return matrix3{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
}

// matrix3FromEulerAnglesXYZ mirrors G3D::Matrix3::fromEulerAnglesXYZ
// (dep/g3dlite/source/Matrix3.cpp): kXMat * (kYMat * kZMat). Note the
// parameter naming in G3D is misleading: the first angle rotates about X,
// the second about Y, the third about Z.
func matrix3FromEulerAnglesXYZ(yAngle, pAngle, rAngle float32) matrix3 {
	return rotX(yAngle).mul(rotY(pAngle).mul(rotZ(rAngle)))
}

// transformVec mirrors TerrainBuilder::transform (TerrainBuilder.cpp):
// rotate, scale, translate, then mirror about the horizontal axes.
func transformVec(v vec3, rotation matrix3, scale float32, position vec3) vec3 {
	out := v.vecMul(rotation)
	out[0] = out[0]*scale + position[0]
	out[1] = out[1]*scale + position[1]
	out[2] = out[2]*scale + position[2]
	out[0] *= -1
	out[1] *= -1
	return out
}

// vmapReader is a little-endian sequential reader over a vmap file,
// mirroring the fread sequences in the C++ readers.
type vmapReader struct {
	f *os.File
}

func (r *vmapReader) bytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(r.f, buf)
	return buf, err
}

func (r *vmapReader) u32() (uint32, error) {
	buf, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (r *vmapReader) u16() (uint16, error) {
	buf, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func (r *vmapReader) f32() (float32, error) {
	bits, err := r.u32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(bits), nil
}

func (r *vmapReader) vec3() (vec3, error) {
	var v vec3
	for i := 0; i < 3; i++ {
		f, err := r.f32()
		if err != nil {
			return v, err
		}
		v[i] = f
	}
	return v, nil
}

func (r *vmapReader) u32s(n uint32) ([]uint32, error) {
	buf, err := r.bytes(int(n) * 4)
	if err != nil {
		return nil, err
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	return out, nil
}

func (r *vmapReader) f32s(n uint32) ([]float32, error) {
	buf, err := r.bytes(int(n) * 4)
	if err != nil {
		return nil, err
	}
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return out, nil
}

func (r *vmapReader) vec3s(n uint32) ([]vec3, error) {
	buf, err := r.bytes(int(n) * 12)
	if err != nil {
		return nil, err
	}
	out := make([]vec3, n)
	for i := range out {
		for j := 0; j < 3; j++ {
			out[i][j] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*12+j*4:]))
		}
	}
	return out, nil
}

// readChunk mirrors readChunk (TileAssembler.cpp): reads len bytes and
// reports whether they equal the expected chunk tag.
func (r *vmapReader) readChunk(name string) bool {
	buf, err := r.bytes(len(name))
	return err == nil && string(buf) == name
}

// bih mirrors BIH (BoundingIntervalHierarchy.h). Only the object list is
// kept: loadVMap needs primCount (== len(objects)) and the byte length of
// the chunk; the tree itself is never queried here.
type bih struct {
	lo      vec3
	hi      vec3
	tree    []uint32
	objects []uint32
}

func (b *bih) primCount() uint32 { return uint32(len(b.objects)) }

// readBIH mirrors BIH::readFromFile.
func (r *vmapReader) readBIH() (*bih, error) {
	b := &bih{}
	var err error
	if b.lo, err = r.vec3(); err != nil {
		return nil, err
	}
	if b.hi, err = r.vec3(); err != nil {
		return nil, err
	}
	treeSize, err := r.u32()
	if err != nil {
		return nil, err
	}
	if b.tree, err = r.u32s(treeSize); err != nil {
		return nil, err
	}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	if b.objects, err = r.u32s(count); err != nil {
		return nil, err
	}
	return b, nil
}

// modelSpawn mirrors VMAP::ModelSpawn (ModelInstance.h).
type modelSpawn struct {
	flags uint32
	adtID uint16
	id    uint32
	pos   vec3
	rot   vec3
	scale float32
	// Bound is present only when flags & MOD_HAS_BOUND.
	bound    [2]vec3
	hasBound bool
	name     string
}

// readModelSpawn mirrors ModelSpawn::readFromFile (ModelInstance.cpp).
func (r *vmapReader) readModelSpawn() (modelSpawn, error) {
	var s modelSpawn
	var err error
	if s.flags, err = r.u32(); err != nil {
		return s, err
	}
	if s.adtID, err = r.u16(); err != nil {
		return s, err
	}
	if s.id, err = r.u32(); err != nil {
		return s, err
	}
	if s.pos, err = r.vec3(); err != nil {
		return s, err
	}
	if s.rot, err = r.vec3(); err != nil {
		return s, err
	}
	if s.scale, err = r.f32(); err != nil {
		return s, err
	}
	if s.flags&modHasBound != 0 {
		s.hasBound = true
		if s.bound[0], err = r.vec3(); err != nil {
			return s, err
		}
		if s.bound[1], err = r.vec3(); err != nil {
			return s, err
		}
	}
	nameLen, err := r.u32()
	if err != nil {
		return s, err
	}
	if nameLen > spawnNameMax {
		return s, fmt.Errorf("ModelSpawn file name too long: %d", nameLen)
	}
	name, err := r.bytes(int(nameLen))
	if err != nil {
		return s, err
	}
	s.name = string(name)
	return s, nil
}

// wmoLiquid mirrors VMAP::WmoLiquid (WorldModel.h). flags is nil when the
// liquid has no tiled height/flag storage (tilesX or tilesY == 0); C++
// loadVMap skips the liquid arm in that case (GetFlagsStorage() == null).
type wmoLiquid struct {
	tilesX     uint32
	tilesY     uint32
	corner     vec3
	liquidType uint32
	heights    []float32
	flags      []uint8
}

// readWmoLiquid mirrors WmoLiquid::readFromFile (WorldModel.cpp).
func (r *vmapReader) readWmoLiquid() (*wmoLiquid, error) {
	l := &wmoLiquid{}
	var err error
	if l.tilesX, err = r.u32(); err != nil {
		return nil, err
	}
	if l.tilesY, err = r.u32(); err != nil {
		return nil, err
	}
	if l.corner, err = r.vec3(); err != nil {
		return nil, err
	}
	if l.liquidType, err = r.u32(); err != nil {
		return nil, err
	}
	if l.tilesX != 0 && l.tilesY != 0 {
		size := (l.tilesX + 1) * (l.tilesY + 1)
		if l.heights, err = r.f32s(size); err != nil {
			return nil, err
		}
		flags, err := r.bytes(int(l.tilesX * l.tilesY))
		if err != nil {
			return nil, err
		}
		l.flags = flags
	} else {
		if l.heights, err = r.f32s(1); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// groupModel mirrors VMAP::GroupModel (WorldModel.h).
type groupModel struct {
	bound      [2]vec3
	mogpFlags  uint32
	groupWMOID uint32
	vertices   []vec3
	triangles  [][3]uint32
	liquid     *wmoLiquid
}

// readGroupModel mirrors GroupModel::readFromFile (WorldModel.cpp). Models
// without (collision) geometry end after the VERT chunk, like in C++.
func (r *vmapReader) readGroupModel() (groupModel, error) {
	var g groupModel
	var err error
	if g.bound[0], err = r.vec3(); err != nil {
		return g, err
	}
	if g.bound[1], err = r.vec3(); err != nil {
		return g, err
	}
	if g.mogpFlags, err = r.u32(); err != nil {
		return g, err
	}
	if g.groupWMOID, err = r.u32(); err != nil {
		return g, err
	}

	if !r.readChunk("VERT") {
		return g, fmt.Errorf("GroupModel missing VERT chunk")
	}
	if _, err = r.u32(); err != nil { // chunkSize
		return g, err
	}
	count, err := r.u32()
	if err != nil {
		return g, err
	}
	if count == 0 {
		return g, nil
	}
	if g.vertices, err = r.vec3s(count); err != nil {
		return g, err
	}

	if !r.readChunk("TRIM") {
		return g, fmt.Errorf("GroupModel missing TRIM chunk")
	}
	if _, err = r.u32(); err != nil { // chunkSize
		return g, err
	}
	triCount, err := r.u32()
	if err != nil {
		return g, err
	}
	indices, err := r.u32s(triCount * 3)
	if err != nil {
		return g, err
	}
	g.triangles = make([][3]uint32, triCount)
	for i := range g.triangles {
		g.triangles[i] = [3]uint32{indices[i*3], indices[i*3+1], indices[i*3+2]}
	}

	if !r.readChunk("MBIH") {
		return g, fmt.Errorf("GroupModel missing MBIH chunk")
	}
	if _, err = r.readBIH(); err != nil {
		return g, err
	}

	if !r.readChunk("LIQU") {
		return g, fmt.Errorf("GroupModel missing LIQU chunk")
	}
	chunkSize, err := r.u32()
	if err != nil {
		return g, err
	}
	if chunkSize > 0 {
		if g.liquid, err = r.readWmoLiquid(); err != nil {
			return g, err
		}
	}
	return g, nil
}

// worldModel mirrors VMAP::WorldModel (WorldModel.h).
type worldModel struct {
	rootWMOID uint32
	groups    []groupModel
}

// readWorldModelFile mirrors WorldModel::readFile (WorldModel.cpp): the
// GMOD group list is optional (a model with no groups has no GMOD chunk).
func readWorldModelFile(path string) (*worldModel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &vmapReader{f: f}

	wm := &worldModel{}
	if !r.readChunk(vmapMagic) {
		return nil, fmt.Errorf("%s: bad VMAP magic", path)
	}
	if !r.readChunk("WMOD") {
		return nil, fmt.Errorf("%s: missing WMOD chunk", path)
	}
	if _, err = r.u32(); err != nil { // chunkSize
		return nil, err
	}
	if wm.rootWMOID, err = r.u32(); err != nil {
		return nil, err
	}
	if r.readChunk("GMOD") {
		count, err := r.u32()
		if err != nil {
			return nil, err
		}
		wm.groups = make([]groupModel, 0, count)
		for i := uint32(0); i < count; i++ {
			g, err := r.readGroupModel()
			if err != nil {
				return nil, err
			}
			wm.groups = append(wm.groups, g)
		}
		if !r.readChunk("GBIH") {
			return nil, fmt.Errorf("%s: missing GBIH chunk", path)
		}
		if _, err = r.readBIH(); err != nil {
			return nil, err
		}
	}
	return wm, nil
}

// staticMapTree mirrors the load-time state of VMAP::StaticMapTree
// (MapTree.h) that loadVMap needs: the tree's primitive count and, for
// untiled maps, the single global spawn.
type staticMapTree struct {
	tiled       bool
	primCount   uint32
	globalSpawn *modelSpawn
}

// loadStaticMapTree mirrors StaticMapTree::InitMap (MapTree.cpp): opens
// "<vmapsDir>/%03d.vmtree", validates the magic, reads the tiled flag, the
// NODE BIH and the GOBJ chunk, then the one global spawn for untiled maps.
func loadStaticMapTree(vmapsDir string, mapID uint32) (*staticMapTree, error) {
	path := filepath.Join(vmapsDir, fmt.Sprintf("%03d.vmtree", mapID))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &vmapReader{f: f}

	tree := &staticMapTree{}
	if !r.readChunk(vmapMagic) {
		return nil, fmt.Errorf("%s: bad VMAP magic", path)
	}
	tiled, err := r.bytes(1)
	if err != nil {
		return nil, err
	}
	tree.tiled = tiled[0] != 0
	if !r.readChunk("NODE") {
		return nil, fmt.Errorf("%s: missing NODE chunk", path)
	}
	b, err := r.readBIH()
	if err != nil {
		return nil, err
	}
	tree.primCount = b.primCount()
	if !r.readChunk("GOBJ") {
		return nil, fmt.Errorf("%s: missing GOBJ chunk", path)
	}
	if !tree.tiled {
		spawn, err := r.readModelSpawn()
		if err != nil {
			return nil, err
		}
		tree.globalSpawn = &spawn
	}
	return tree, nil
}

// loadVMapTileSpawns mirrors StaticMapTree::LoadMapTile (MapTree.cpp):
// reads "<vmapsDir>/%03d_%02d_%02d.vmtile" (mapID, tileY, tileX — the same
// Y-first order as StaticMapTree::getTileFileName) and returns the tile's
// spawns keyed by tree index. A missing tile file is not an error: C++
// marks the tile not-loaded and returns true, so the caller proceeds with
// no spawns. A present-but-unreadable file is an error, mirroring
// LoadMapTile returning false (which makes VMapManager2::loadMap report
// VMAP_LOAD_RESULT_ERROR and loadVMap bail out with no geometry).
// The first spawn wins for a repeated tree index, mirroring the
// refcounted iLoadedSpawns logic; out-of-range indices are skipped like
// the C++ bounds check (which uses >; >= is used here because == would
// index one past the tree values).
func loadVMapTileSpawns(vmapsDir string, mapID, tileX, tileY uint32, primCount uint32) (map[uint32]modelSpawn, error) {
	path := filepath.Join(vmapsDir, fmt.Sprintf("%03d_%02d_%02d.vmtile", mapID, tileY, tileX))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[uint32]modelSpawn{}, nil
		}
		return nil, err
	}
	defer f.Close()
	r := &vmapReader{f: f}

	spawns := make(map[uint32]modelSpawn)
	if !r.readChunk(vmapMagic) {
		return nil, fmt.Errorf("%s: bad VMAP magic", path)
	}
	numSpawns, err := r.u32()
	if err != nil {
		return nil, err
	}
	for i := uint32(0); i < numSpawns; i++ {
		spawn, err := r.readModelSpawn()
		if err != nil {
			return nil, err
		}
		referencedVal, err := r.u32()
		if err != nil {
			return nil, err
		}
		if referencedVal >= primCount {
			continue
		}
		if _, ok := spawns[referencedVal]; !ok {
			spawns[referencedVal] = spawn
		}
	}
	return spawns, nil
}

// LiquidType.dbc lookup (PathGenerator.cpp LoadLiquid + GetLiquidFlags):
// maps the LiquidType.dbc record ID (field 0) to its Type (field 3), and
// GetLiquidFlags returns 1 << Type, or 0 for an unknown ID. C++ loads the
// table once in main() and aborts the whole run when the DBC is missing;
// here it is loaded lazily on first use and a missing/unreadable DBC only
// warns once, leaving every WMO liquid typed NAV_AREA_EMPTY — the tool has
// no DBC directory flag and must not fail tile generation for it.
var (
	liquidTypesOnce sync.Once
	liquidTypesMap  map[uint32]uint8
)

func loadLiquidTypes() map[uint32]uint8 {
	liquidTypesOnce.Do(func() {
		liquidTypesMap = map[uint32]uint8{}
		data, err := os.ReadFile(filepath.Join("dbc", "LiquidType.dbc"))
		if err != nil {
			fmt.Printf("LoadVMap: could not load dbc/LiquidType.dbc (%v); WMO liquid will be typed as empty\n", err)
			return
		}
		// Minimal WDBC reader (same header layout as
		// vmap4extractor/dbc.go openDBC): magic, record/field counts,
		// record size, string block size; records follow the header.
		if len(data) < 20 || string(data[:4]) != "WDBC" {
			fmt.Printf("LoadVMap: dbc/LiquidType.dbc has a bad header; WMO liquid will be typed as empty\n")
			return
		}
		recordCount := binary.LittleEndian.Uint32(data[4:8])
		fieldCount := binary.LittleEndian.Uint32(data[8:12])
		recordSize := binary.LittleEndian.Uint32(data[12:16])
		stringSize := binary.LittleEndian.Uint32(data[16:20])
		if fieldCount < 4 || uint64(fieldCount)*4 < uint64(recordSize) ||
			uint64(len(data)) < 20+uint64(recordSize)*uint64(recordCount)+uint64(stringSize) {
			fmt.Printf("LoadVMap: dbc/LiquidType.dbc is truncated; WMO liquid will be typed as empty\n")
			return
		}
		for i := uint32(0); i < recordCount; i++ {
			base := 20 + uint64(i)*uint64(recordSize)
			id := binary.LittleEndian.Uint32(data[base:])
			typ := binary.LittleEndian.Uint32(data[base+12:])
			liquidTypesMap[id] = uint8(typ)
		}
	})
	return liquidTypesMap
}

func getLiquidFlags(liquidID uint32) uint32 {
	if typ, ok := loadLiquidTypes()[liquidID]; ok && typ < 32 {
		return 1 << typ
	}
	return 0
}

// appendVMapLiquid mirrors the WMO-liquid arm of TerrainBuilder::loadVMap:
// builds the liquid vertex grid from the WmoLiquid height/flag storage,
// transforms it like the collision mesh, and appends it to md's liquid
// arrays with the NavTerrain type derived from GetLiquidFlags.
func appendVMapLiquid(liquid *wmoLiquid, rotation matrix3, scale float32, position vec3, md *MeshData) {
	if liquid == nil || liquid.flags == nil {
		return
	}
	tilesX, tilesY := liquid.tilesX, liquid.tilesY
	vertsX, vertsY := tilesX+1, tilesY+1

	liqType := uint8(navAreaEmpty)
	liquidFlags := getLiquidFlags(liquid.liquidType)
	if liquidFlags&(mapLiquidTypeWater|mapLiquidTypeOcean) != 0 {
		liqType = navAreaWater
	} else if liquidFlags&(mapLiquidTypeMagma|mapLiquidTypeSlime) != 0 {
		liqType = navAreaMagmaSlime
	}

	// Indexing is weird (C++ comment): vertex = y*vertsX+x in the height
	// storage, but the generated grid is x-major: vertex index = x*vertsY+y.
	liqVerts := make([]vec3, 0, vertsX*vertsY)
	for x := uint32(0); x < vertsX; x++ {
		for y := uint32(0); y < vertsY; y++ {
			vert := vec3{
				liquid.corner[0] + float32(x)*gridPartSize,
				liquid.corner[1] + float32(y)*gridPartSize,
				liquid.heights[y*vertsX+x],
			}
			liqVerts = append(liqVerts, transformVec(vert, rotation, scale, position))
		}
	}

	var liqTris []int32
	for x := uint32(0); x < tilesX; x++ {
		for y := uint32(0); y < tilesY; y++ {
			if liquid.flags[x+y*tilesX]&0x0f == 0x0f {
				continue
			}
			square := x*tilesY + y
			idx1 := int32(square + x)
			idx2 := int32(square + 1 + x)
			idx3 := int32(square + tilesY + 1 + 1 + x)
			idx4 := int32(square + tilesY + 1 + x)
			// Top triangle, then bottom triangle.
			liqTris = append(liqTris, idx3, idx2, idx1, idx4, idx3, idx1)
		}
	}

	liqOffset := int32(len(md.LiquidVerts) / 3)
	for _, v := range liqVerts {
		md.LiquidVerts = append(md.LiquidVerts, v[1], v[2], v[0])
	}
	for j := 0; j < len(liqTris)/3; j++ {
		md.LiquidTris = append(md.LiquidTris, liqTris[j*3+1]+liqOffset, liqTris[j*3+2]+liqOffset, liqTris[j*3]+liqOffset)
		md.LiquidType = append(md.LiquidType, liqType)
	}
}

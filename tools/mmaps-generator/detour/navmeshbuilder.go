// Package detour is a Go port of the Detour navigation-mesh tile builder
// (dep/recastnavigation/Detour/Source/DetourNavMeshBuilder.cpp,
// DetourNavMesh.h, DetourNavMeshBuilder.h) as used by TrinityCore's
// mmaps_generator (src/tools/mmaps_generator/MapBuilder.cpp,
// TileBuilder::buildMoveMapTile).
//
// CreateNavMeshData serializes one tile (dtMeshHeader + verts + polys +
// detail meshes + BV tree + off-mesh connections) into the exact little-endian
// byte layout dtNavMesh::addTile expects. All writes are explicit
// little-endian field writes; no Go struct is binary-written, so no implicit
// padding ever leaks into the output.
package detour

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/recast"
)

// Tile serialization constants (DetourNavMesh.h).
const (
	// DT_NAVMESH_MAGIC ('D'<<24 | 'N'<<16 | 'A'<<8 | 'V').
	DT_NAVMESH_MAGIC = 'D'<<24 | 'N'<<16 | 'A'<<8 | 'V'
	// DT_NAVMESH_VERSION.
	DT_NAVMESH_VERSION = 7
	// DT_NAVMESH_STATE_MAGIC ('D'<<24 | 'N'<<16 | 'M'<<8 | 'S').
	DT_NAVMESH_STATE_MAGIC = 'D'<<24 | 'N'<<16 | 'M'<<8 | 'S'
	// DT_NAVMESH_STATE_VERSION.
	DT_NAVMESH_STATE_VERSION = 1
	// DT_VERTS_PER_POLYGON.
	DT_VERTS_PER_POLYGON = 6
	// DT_EXT_LINK.
	DT_EXT_LINK = 0x8000
	// DT_NULL_LINK.
	DT_NULL_LINK = 0xffffffff
	// DT_OFFMESH_CON_BIDIR.
	DT_OFFMESH_CON_BIDIR = 1
	// DT_MAX_AREAS.
	DT_MAX_AREAS = 64
	// DT_POLYTYPE_GROUND.
	DT_POLYTYPE_GROUND = 0
	// DT_POLYTYPE_OFFMESH_CONNECTION.
	DT_POLYTYPE_OFFMESH_CONNECTION = 1
	// DT_DETAIL_EDGE_BOUNDARY.
	DT_DETAIL_EDGE_BOUNDARY = 0x01
	// DT_TILE_FREE_DATA.
	DT_TILE_FREE_DATA = 0x01
	// DT_SALT_BITS / DT_TILE_BITS / DT_POLY_BITS (DT_POLYREF64).
	DT_SALT_BITS = 12
	DT_TILE_BITS = 21
	DT_POLY_BITS = 31
)

// Wire sizes of the serialized C++ structs (DetourNavMesh.h). dtLink is 16
// bytes because DT_POLYREF64 makes dtPolyRef a uint64; dtPolyDetail is 12
// bytes (10 bytes of fields + 2 bytes tail padding).
const (
	meshHeaderSize        = 100
	polySize              = 32
	polyDetailSize        = 12
	bvNodeSize            = 16
	linkSize              = 16
	offMeshConnectionSize = 36
)

const meshNullIdx = 0xffff

// NavMeshCreateParams mirrors dtNavMeshCreateParams
// (DetourNavMeshBuilder.h) in C++ field order. TileLayer is included: the
// C++ struct has it (MapBuilder sets tileLayer = 0) and the header serializes
// it. There is no DetailMeshCount field in C++; the detail mesh count is
// PolyCount.
type NavMeshCreateParams struct {
	Verts     []uint16
	VertCount int
	Polys     []uint16
	PolyFlags []uint16
	PolyAreas []uint8
	PolyCount int
	Nvp       int

	DetailMeshes     []uint32
	DetailVerts      []float32
	DetailVertsCount int
	DetailTris       []uint8
	DetailTriCount   int

	OffMeshConVerts  []float32
	OffMeshConRad    []float32
	OffMeshConFlags  []uint16
	OffMeshConAreas  []uint8
	OffMeshConDir    []uint8
	OffMeshConUserID []uint32
	OffMeshConCount  int

	UserID    uint32
	TileX     int
	TileY     int
	TileLayer int
	Bmin      [3]float32
	Bmax      [3]float32

	WalkableHeight float32
	WalkableRadius float32
	WalkableClimb  float32
	Cs             float32
	Ch             float32
	BuildBvTree    bool
}

// NewNavMeshCreateParams builds creation params from a Recast poly mesh and
// detail mesh, mirroring the dtNavMeshCreateParams fill in
// MapBuilder::buildMoveMapTile (MapBuilder.cpp). Tile placement, walkable
// agent values, off-mesh connections and BuildBvTree are left for the caller;
// MapBuilder sets TileX/TileY from the tile bounds, WalkableHeight/Radius/
// Climb from the config, and BuildBvTree = true. A nil detail mesh is
// allowed: CreateNavMeshData then builds dummy detail meshes by
// triangulating the polys, as the C++ does.
func NewNavMeshCreateParams(mesh *recast.PolyMesh, detail *recast.PolyMeshDetail) *NavMeshCreateParams {
	if mesh == nil {
		return nil
	}
	p := &NavMeshCreateParams{
		Verts:     mesh.Verts,
		VertCount: mesh.Nverts,
		Polys:     mesh.Polys,
		PolyFlags: mesh.Flags,
		PolyAreas: mesh.Areas,
		PolyCount: mesh.Npolys,
		Nvp:       mesh.Nvp,
		Bmin:      mesh.Bmin,
		Bmax:      mesh.Bmax,
		Cs:        mesh.Cs,
		Ch:        mesh.Ch,
	}
	if detail != nil {
		p.DetailMeshes = detail.Meshes
		p.DetailVerts = detail.Verts
		p.DetailVertsCount = detail.Nverts
		p.DetailTris = detail.Tris
		p.DetailTriCount = detail.Ntris
	}
	return p
}

func align4(x int) int { return (x + 3) &^ 3 }

func clampInt(v, mn, mx int) int {
	if v < mn {
		return mn
	}
	if v > mx {
		return mx
	}
	return v
}

func putUint16(data []byte, off int, v uint16) { binary.LittleEndian.PutUint16(data[off:], v) }
func putUint32(data []byte, off int, v uint32) { binary.LittleEndian.PutUint32(data[off:], v) }
func putInt32(data []byte, off int, v int32)   { binary.LittleEndian.PutUint32(data[off:], uint32(v)) }
func putFloat32(data []byte, off int, v float32) {
	binary.LittleEndian.PutUint32(data[off:], math.Float32bits(v))
}

// classifyOffMeshPoint mirrors classifyOffMeshPoint in
// DetourNavMeshBuilder.cpp. Returns 0-7 for the side the point lies outside
// the tile bounds on, or 0xff when the point is inside (xz).
func classifyOffMeshPoint(pt, bmin, bmax [3]float32) uint8 {
	const (
		xp = 1 << 0
		zp = 1 << 1
		xm = 1 << 2
		zm = 1 << 3
	)
	var outcode uint8
	if pt[0] >= bmax[0] {
		outcode |= xp
	}
	if pt[2] >= bmax[2] {
		outcode |= zp
	}
	if pt[0] < bmin[0] {
		outcode |= xm
	}
	if pt[2] < bmin[2] {
		outcode |= zm
	}
	switch outcode {
	case xp:
		return 0
	case xp | zp:
		return 1
	case zp:
		return 2
	case xm | zp:
		return 3
	case xm:
		return 4
	case xm | zm:
		return 5
	case zm:
		return 6
	case xp | zm:
		return 7
	}
	return 0xff
}

// bvItem mirrors BVItem in DetourNavMeshBuilder.cpp.
type bvItem struct {
	bmin [3]uint16
	bmax [3]uint16
	i    int
}

// bvNode mirrors dtBVNode (DetourNavMesh.h).
type bvNode struct {
	bmin [3]uint16
	bmax [3]uint16
	i    int32
}

func calcExtendsBV(items []bvItem, imin, imax int, bmin, bmax *[3]uint16) {
	*bmin = items[imin].bmin
	*bmax = items[imin].bmax
	for i := imin + 1; i < imax; i++ {
		it := items[i]
		if it.bmin[0] < bmin[0] {
			bmin[0] = it.bmin[0]
		}
		if it.bmin[1] < bmin[1] {
			bmin[1] = it.bmin[1]
		}
		if it.bmin[2] < bmin[2] {
			bmin[2] = it.bmin[2]
		}
		if it.bmax[0] > bmax[0] {
			bmax[0] = it.bmax[0]
		}
		if it.bmax[1] > bmax[1] {
			bmax[1] = it.bmax[1]
		}
		if it.bmax[2] > bmax[2] {
			bmax[2] = it.bmax[2]
		}
	}
}

func longestAxisBV(x, y, z uint16) int {
	axis := 0
	maxVal := x
	if y > maxVal {
		axis = 1
		maxVal = y
	}
	if z > maxVal {
		axis = 2
	}
	return axis
}

func subdivideBV(items []bvItem, imin, imax int, curNode *int, nodes []bvNode) {
	inum := imax - imin
	icur := *curNode
	node := &nodes[*curNode]
	*curNode++

	if inum == 1 {
		node.bmin = items[imin].bmin
		node.bmax = items[imin].bmax
		node.i = int32(items[imin].i)
		return
	}

	calcExtendsBV(items, imin, imax, &node.bmin, &node.bmax)

	axis := longestAxisBV(node.bmax[0]-node.bmin[0], node.bmax[1]-node.bmin[1], node.bmax[2]-node.bmin[2])

	// qsort in C++ is not stable; sort.Slice is not stable either.
	switch axis {
	case 0:
		sort.Slice(items[imin:imax], func(a, b int) bool {
			return items[imin+a].bmin[0] < items[imin+b].bmin[0]
		})
	case 1:
		sort.Slice(items[imin:imax], func(a, b int) bool {
			return items[imin+a].bmin[1] < items[imin+b].bmin[1]
		})
	default:
		sort.Slice(items[imin:imax], func(a, b int) bool {
			return items[imin+a].bmin[2] < items[imin+b].bmin[2]
		})
	}

	isplit := imin + inum/2
	subdivideBV(items, imin, isplit, curNode, nodes)
	subdivideBV(items, isplit, imax, curNode, nodes)

	// Negative index means escape.
	node.i = -int32(*curNode - icur)
}

// createBVTree mirrors createBVTree in DetourNavMeshBuilder.cpp. It fills
// nodes (sized 2*polyCount) and returns the number of nodes used. ok is
// false when the params index outside their own arrays; the C++ reads out of
// bounds there, the Go port fails the build instead.
func createBVTree(params *NavMeshCreateParams, nodes []bvNode) (int, bool) {
	quantFactor := float32(1) / params.Cs
	items := make([]bvItem, params.PolyCount)
	for i := 0; i < params.PolyCount; i++ {
		it := &items[i]
		it.i = i
		if params.DetailMeshes != nil {
			vb := int(params.DetailMeshes[i*4+0])
			ndv := int(params.DetailMeshes[i*4+1])
			if vb < 0 || ndv < 1 || vb+ndv > params.DetailVertsCount {
				return 0, false
			}
			dv := params.DetailVerts[vb*3:]
			var bmin, bmax [3]float32
			bmin = [3]float32{dv[0], dv[1], dv[2]}
			bmax = bmin
			for j := 1; j < ndv; j++ {
				v := dv[j*3:]
				if v[0] < bmin[0] {
					bmin[0] = v[0]
				}
				if v[1] < bmin[1] {
					bmin[1] = v[1]
				}
				if v[2] < bmin[2] {
					bmin[2] = v[2]
				}
				if v[0] > bmax[0] {
					bmax[0] = v[0]
				}
				if v[1] > bmax[1] {
					bmax[1] = v[1]
				}
				if v[2] > bmax[2] {
					bmax[2] = v[2]
				}
			}
			// BV-tree uses cs for all dimensions.
			it.bmin[0] = uint16(clampInt(int((bmin[0]-params.Bmin[0])*quantFactor), 0, 0xffff))
			it.bmin[1] = uint16(clampInt(int((bmin[1]-params.Bmin[1])*quantFactor), 0, 0xffff))
			it.bmin[2] = uint16(clampInt(int((bmin[2]-params.Bmin[2])*quantFactor), 0, 0xffff))
			it.bmax[0] = uint16(clampInt(int((bmax[0]-params.Bmin[0])*quantFactor), 0, 0xffff))
			it.bmax[1] = uint16(clampInt(int((bmax[1]-params.Bmin[1])*quantFactor), 0, 0xffff))
			it.bmax[2] = uint16(clampInt(int((bmax[2]-params.Bmin[2])*quantFactor), 0, 0xffff))
		} else {
			p := params.Polys[i*params.Nvp*2:]
			if p[0] == meshNullIdx || int(p[0]) >= params.VertCount {
				return 0, false
			}
			it.bmin[0] = params.Verts[int(p[0])*3+0]
			it.bmin[1] = params.Verts[int(p[0])*3+1]
			it.bmin[2] = params.Verts[int(p[0])*3+2]
			it.bmax = it.bmin
			for j := 1; j < params.Nvp; j++ {
				if p[j] == meshNullIdx {
					break
				}
				if int(p[j]) >= params.VertCount {
					return 0, false
				}
				x := params.Verts[int(p[j])*3+0]
				y := params.Verts[int(p[j])*3+1]
				z := params.Verts[int(p[j])*3+2]
				if x < it.bmin[0] {
					it.bmin[0] = x
				}
				if y < it.bmin[1] {
					it.bmin[1] = y
				}
				if z < it.bmin[2] {
					it.bmin[2] = z
				}
				if x > it.bmax[0] {
					it.bmax[0] = x
				}
				if y > it.bmax[1] {
					it.bmax[1] = y
				}
				if z > it.bmax[2] {
					it.bmax[2] = z
				}
			}
			// Remap y.
			it.bmin[1] = uint16(math.Floor(float64(float32(it.bmin[1]) * params.Ch / params.Cs)))
			it.bmax[1] = uint16(math.Ceil(float64(float32(it.bmax[1]) * params.Ch / params.Cs)))
		}
	}

	curNode := 0
	subdivideBV(items, 0, params.PolyCount, &curNode, nodes)
	return curNode, true
}

// CreateNavMeshData is a faithful port of dtCreateNavMeshData
// (DetourNavMeshBuilder.cpp). It validates params exactly as the C++ does
// (nvp > DT_VERTS_PER_POLYGON, vertCount >= 0xffff, missing verts/polys)
// and serializes the tile into the exact C++ byte layout: dtMeshHeader,
// verts, polys, links (zeroed space, links are rebuilt on load), detail
// meshes, detail verts, detail tris, BV tree, off-mesh connections.
// It returns the tile data and true on success, nil and false on validation
// failure.
//
// Bounds guards beyond the C++ checks return false instead of reading out
// of bounds: the C++ reads caller-owned arrays unchecked, the Go port
// cannot. Valid Recast output always satisfies them.
func CreateNavMeshData(params *NavMeshCreateParams) ([]byte, bool) {
	if params == nil {
		return nil, false
	}
	if params.Nvp > DT_VERTS_PER_POLYGON {
		return nil, false
	}
	if params.VertCount >= 0xffff {
		return nil, false
	}
	if params.VertCount == 0 || len(params.Verts) == 0 {
		return nil, false
	}
	if params.PolyCount == 0 || len(params.Polys) == 0 {
		return nil, false
	}

	nvp := params.Nvp

	// Go memory-safety guards (C++ caller contract, unchecked there).
	if len(params.Verts) < params.VertCount*3 {
		return nil, false
	}
	if len(params.Polys) < params.PolyCount*2*nvp {
		return nil, false
	}
	if len(params.PolyFlags) < params.PolyCount || len(params.PolyAreas) < params.PolyCount {
		return nil, false
	}
	if params.DetailMeshes != nil {
		if len(params.DetailMeshes) < params.PolyCount*4 ||
			len(params.DetailVerts) < params.DetailVertsCount*3 ||
			len(params.DetailTris) < params.DetailTriCount*4 {
			return nil, false
		}
		for i := 0; i < params.PolyCount; i++ {
			vb := int(params.DetailMeshes[i*4+0])
			ndv := int(params.DetailMeshes[i*4+1])
			if vb < 0 || ndv < 0 || vb+ndv > params.DetailVertsCount {
				return nil, false
			}
		}
	}
	if params.OffMeshConCount > 0 {
		if len(params.OffMeshConVerts) < params.OffMeshConCount*6 ||
			len(params.OffMeshConRad) < params.OffMeshConCount ||
			len(params.OffMeshConFlags) < params.OffMeshConCount ||
			len(params.OffMeshConAreas) < params.OffMeshConCount ||
			len(params.OffMeshConDir) < params.OffMeshConCount {
			return nil, false
		}
		if params.OffMeshConUserID != nil && len(params.OffMeshConUserID) < params.OffMeshConCount {
			return nil, false
		}
	}
	// Every polygon vertex index must name a real vertex (the BV-tree
	// branch and the detail-vert count both index verts through them).
	for i := 0; i < params.PolyCount; i++ {
		p := params.Polys[i*2*nvp:]
		for j := 0; j < nvp; j++ {
			if p[j] == meshNullIdx {
				break
			}
			if int(p[j]) >= params.VertCount {
				return nil, false
			}
		}
	}

	// Classify off-mesh connection points. Only connections whose start
	// point is inside the tile are stored.
	var offMeshConClass []uint8
	storedOffMeshConCount := 0
	offMeshConLinkCount := 0

	if params.OffMeshConCount > 0 {
		offMeshConClass = make([]uint8, params.OffMeshConCount*2)

		// Find tight height bounds, used for culling out off-mesh start
		// locations.
		hmin := float32(math.MaxFloat32)
		hmax := float32(-math.MaxFloat32)

		if params.DetailVerts != nil && params.DetailVertsCount > 0 {
			for i := 0; i < params.DetailVertsCount; i++ {
				h := params.DetailVerts[i*3+1]
				if h < hmin {
					hmin = h
				}
				if h > hmax {
					hmax = h
				}
			}
		} else {
			for i := 0; i < params.VertCount; i++ {
				iv := params.Verts[i*3:]
				h := params.Bmin[1] + float32(iv[1])*params.Ch
				if h < hmin {
					hmin = h
				}
				if h > hmax {
					hmax = h
				}
			}
		}
		hmin -= params.WalkableClimb
		hmax += params.WalkableClimb
		bmin := params.Bmin
		bmax := params.Bmax
		bmin[1] = hmin
		bmax[1] = hmax

		for i := 0; i < params.OffMeshConCount; i++ {
			p0 := params.OffMeshConVerts[(i*2+0)*3:]
			p1 := params.OffMeshConVerts[(i*2+1)*3:]
			offMeshConClass[i*2+0] = classifyOffMeshPoint([3]float32{p0[0], p0[1], p0[2]}, bmin, bmax)
			offMeshConClass[i*2+1] = classifyOffMeshPoint([3]float32{p1[0], p1[1], p1[2]}, bmin, bmax)

			// Zero out off-mesh start positions which are not even
			// potentially touching the mesh.
			if offMeshConClass[i*2+0] == 0xff {
				if p0[1] < bmin[1] || p0[1] > bmax[1] {
					offMeshConClass[i*2+0] = 0
				}
			}

			// Count how many links should be allocated for off-mesh
			// connections.
			if offMeshConClass[i*2+0] == 0xff {
				offMeshConLinkCount++
			}
			if offMeshConClass[i*2+1] == 0xff {
				offMeshConLinkCount++
			}

			if offMeshConClass[i*2+0] == 0xff {
				storedOffMeshConCount++
			}
		}
	}

	// Off-mesh connections are stored as polygons, adjust values.
	totPolyCount := params.PolyCount + storedOffMeshConCount
	totVertCount := params.VertCount + storedOffMeshConCount*2

	// Find portal edges which are at tile borders.
	edgeCount := 0
	portalCount := 0
	for i := 0; i < params.PolyCount; i++ {
		p := params.Polys[i*2*nvp:]
		for j := 0; j < nvp; j++ {
			if p[j] == meshNullIdx {
				break
			}
			edgeCount++
			if p[nvp+j]&0x8000 != 0 {
				dir := p[nvp+j] & 0xf
				if dir != 0xf {
					portalCount++
				}
			}
		}
	}

	maxLinkCount := edgeCount + portalCount*2 + offMeshConLinkCount*2

	// Find unique detail vertices.
	uniqueDetailVertCount := 0
	detailTriCount := 0
	if params.DetailMeshes != nil {
		// Has detail mesh, count unique detail vertex count and use
		// input detail tri count.
		detailTriCount = params.DetailTriCount
		for i := 0; i < params.PolyCount; i++ {
			p := params.Polys[i*nvp*2:]
			ndv := int(params.DetailMeshes[i*4+1])
			nv := 0
			for j := 0; j < nvp; j++ {
				if p[j] == meshNullIdx {
					break
				}
				nv++
			}
			ndv -= nv
			uniqueDetailVertCount += ndv
		}
	} else {
		// No input detail mesh, build detail mesh from nav polys.
		uniqueDetailVertCount = 0
		detailTriCount = 0
		for i := 0; i < params.PolyCount; i++ {
			p := params.Polys[i*nvp*2:]
			nv := 0
			for j := 0; j < nvp; j++ {
				if p[j] == meshNullIdx {
					break
				}
				nv++
			}
			detailTriCount += nv - 2
		}
	}

	// Degenerate polys (fewer than 2 verts, or a detail mesh with fewer
	// verts than the poly itself) drive these counts negative; the C++
	// sizes its allocation from the same values, the Go port fails the
	// build instead of allocating a negative size.
	if uniqueDetailVertCount < 0 || detailTriCount < 0 {
		return nil, false
	}

	// Calculate data size.
	headerSize := align4(meshHeaderSize)
	vertsSize := align4(4 * 3 * totVertCount)
	polysSize := align4(polySize * totPolyCount)
	linksSize := align4(linkSize * maxLinkCount)
	detailMeshesSize := align4(polyDetailSize * params.PolyCount)
	detailVertsSize := align4(4 * 3 * uniqueDetailVertCount)
	detailTrisSize := align4(4 * detailTriCount)
	bvTreeSize := 0
	if params.BuildBvTree {
		bvTreeSize = align4(bvNodeSize * params.PolyCount * 2)
	}
	offMeshConsSize := align4(offMeshConnectionSize * storedOffMeshConCount)

	dataSize := headerSize + vertsSize + polysSize + linksSize +
		detailMeshesSize + detailVertsSize + detailTrisSize +
		bvTreeSize + offMeshConsSize

	data := make([]byte, dataSize)

	// Section offsets (dtGetThenAdvanceBufferPointer order in C++).
	d := 0
	headerOff := d
	d += headerSize
	vertsOff := d
	d += vertsSize
	polysOff := d
	d += polysSize
	d += linksSize // Ignore links; just leave space for them. Rebuilt on load.
	detailMeshesOff := d
	d += detailMeshesSize
	detailVertsOff := d
	d += detailVertsSize
	detailTrisOff := d
	d += detailTrisSize
	bvTreeOff := d
	d += bvTreeSize
	offMeshConsOff := d
	d += offMeshConsSize

	bvNodeCount := 0
	if params.BuildBvTree {
		bvNodeCount = params.PolyCount * 2
	}

	// Store header.
	putInt32(data, headerOff+0, DT_NAVMESH_MAGIC)
	putInt32(data, headerOff+4, DT_NAVMESH_VERSION)
	putInt32(data, headerOff+8, int32(params.TileX))
	putInt32(data, headerOff+12, int32(params.TileY))
	putInt32(data, headerOff+16, int32(params.TileLayer))
	putUint32(data, headerOff+20, params.UserID)
	putInt32(data, headerOff+24, int32(totPolyCount))
	putInt32(data, headerOff+28, int32(totVertCount))
	putInt32(data, headerOff+32, int32(maxLinkCount))
	putInt32(data, headerOff+36, int32(params.PolyCount))
	putInt32(data, headerOff+40, int32(uniqueDetailVertCount))
	putInt32(data, headerOff+44, int32(detailTriCount))
	putInt32(data, headerOff+48, int32(bvNodeCount))
	putInt32(data, headerOff+52, int32(storedOffMeshConCount))
	putInt32(data, headerOff+56, int32(params.PolyCount))
	putFloat32(data, headerOff+60, params.WalkableHeight)
	putFloat32(data, headerOff+64, params.WalkableRadius)
	putFloat32(data, headerOff+68, params.WalkableClimb)
	putFloat32(data, headerOff+72, params.Bmin[0])
	putFloat32(data, headerOff+76, params.Bmin[1])
	putFloat32(data, headerOff+80, params.Bmin[2])
	putFloat32(data, headerOff+84, params.Bmax[0])
	putFloat32(data, headerOff+88, params.Bmax[1])
	putFloat32(data, headerOff+92, params.Bmax[2])
	putFloat32(data, headerOff+96, float32(1)/params.Cs)

	offMeshVertsBase := params.VertCount
	offMeshPolyBase := params.PolyCount

	// Store vertices.
	// Mesh vertices.
	for i := 0; i < params.VertCount; i++ {
		iv := params.Verts[i*3:]
		v := vertsOff + i*3*4
		putFloat32(data, v+0, params.Bmin[0]+float32(iv[0])*params.Cs)
		putFloat32(data, v+4, params.Bmin[1]+float32(iv[1])*params.Ch)
		putFloat32(data, v+8, params.Bmin[2]+float32(iv[2])*params.Cs)
	}
	// Off-mesh link vertices.
	n := 0
	for i := 0; i < params.OffMeshConCount; i++ {
		// Only store connections which start from this tile.
		if offMeshConClass[i*2+0] == 0xff {
			linkv := params.OffMeshConVerts[i*2*3:]
			v := vertsOff + (offMeshVertsBase+n*2)*3*4
			putFloat32(data, v+0, linkv[0])
			putFloat32(data, v+4, linkv[1])
			putFloat32(data, v+8, linkv[2])
			putFloat32(data, v+12, linkv[3])
			putFloat32(data, v+16, linkv[4])
			putFloat32(data, v+20, linkv[5])
			n++
		}
	}

	// Store polygons.
	// Mesh polys. Poly vert counts are kept for the detail-mesh pass below.
	polyVertCounts := make([]int, params.PolyCount)
	src := params.Polys
	for i := 0; i < params.PolyCount; i++ {
		// dtPoly: firstLink uint32 @0 (left 0; set on load), verts[6]
		// uint16 @4, neis[6] uint16 @16, flags uint16 @28, vertCount
		// uint8 @30, areaAndtype uint8 @31.
		p := polysOff + i*polySize
		vertCount := 0
		putUint16(data, p+28, params.PolyFlags[i])
		// setArea then setType(GROUND): areaAndtype = area & 0x3f.
		data[p+31] = params.PolyAreas[i] & 0x3f
		for j := 0; j < nvp; j++ {
			if src[j] == meshNullIdx {
				break
			}
			putUint16(data, p+4+j*2, src[j])
			if src[nvp+j]&0x8000 != 0 {
				// Border or portal edge.
				dir := src[nvp+j] & 0xf
				switch dir {
				case 0xf: // Border
					putUint16(data, p+16+j*2, 0)
				case 0: // Portal x-
					putUint16(data, p+16+j*2, DT_EXT_LINK|4)
				case 1: // Portal z+
					putUint16(data, p+16+j*2, DT_EXT_LINK|2)
				case 2: // Portal x+
					putUint16(data, p+16+j*2, DT_EXT_LINK|0)
				case 3: // Portal z-
					putUint16(data, p+16+j*2, DT_EXT_LINK|6)
				}
			} else {
				// Normal connection.
				putUint16(data, p+16+j*2, src[nvp+j]+1)
			}
			vertCount++
		}
		data[p+30] = uint8(vertCount)
		polyVertCounts[i] = vertCount
		src = src[nvp*2:]
	}
	// Off-mesh connection polys.
	n = 0
	for i := 0; i < params.OffMeshConCount; i++ {
		// Only store connections which start from this tile.
		if offMeshConClass[i*2+0] == 0xff {
			p := polysOff + (offMeshPolyBase+n)*polySize
			putUint16(data, p+4+0*2, uint16(offMeshVertsBase+n*2+0))
			putUint16(data, p+4+1*2, uint16(offMeshVertsBase+n*2+1))
			putUint16(data, p+28, params.OffMeshConFlags[i])
			// setArea then setType(OFFMESH_CONNECTION).
			data[p+31] = params.OffMeshConAreas[i]&0x3f | (DT_POLYTYPE_OFFMESH_CONNECTION << 6)
			data[p+30] = 2
			n++
		}
	}

	// Store detail meshes and vertices.
	// The nav polygon vertices are stored as the first vertices on each
	// mesh. The mesh data is compressed by skipping them and using the
	// navmesh coordinates.
	if params.DetailMeshes != nil {
		vbase := 0
		for i := 0; i < params.PolyCount; i++ {
			dtl := detailMeshesOff + i*polyDetailSize
			vb := int(params.DetailMeshes[i*4+0])
			ndv := int(params.DetailMeshes[i*4+1])
			nv := polyVertCounts[i]
			putUint32(data, dtl+0, uint32(vbase))
			data[dtl+8] = uint8(ndv - nv)
			putUint32(data, dtl+4, params.DetailMeshes[i*4+2])
			data[dtl+9] = uint8(params.DetailMeshes[i*4+3])
			// Copy vertices except the first 'nv' verts which are equal
			// to nav poly verts.
			if ndv-nv > 0 {
				for k := 0; k < (ndv-nv)*3; k++ {
					putFloat32(data, detailVertsOff+(vbase*3+k)*4, params.DetailVerts[(vb+nv)*3+k])
				}
				vbase += ndv - nv
			}
		}
		// Store triangles.
		copy(data[detailTrisOff:], params.DetailTris[:4*params.DetailTriCount])
	} else {
		// Create dummy detail mesh by triangulating polys.
		tbase := 0
		for i := 0; i < params.PolyCount; i++ {
			dtl := detailMeshesOff + i*polyDetailSize
			nv := polyVertCounts[i]
			putUint32(data, dtl+0, 0)
			data[dtl+8] = 0
			putUint32(data, dtl+4, uint32(tbase))
			data[dtl+9] = uint8(nv - 2)
			// Triangulate polygon (local indices).
			for j := 2; j < nv; j++ {
				t := detailTrisOff + tbase*4
				data[t+0] = 0
				data[t+1] = uint8(j - 1)
				data[t+2] = uint8(j)
				// Bit for each edge that belongs to poly boundary.
				data[t+3] = (1 << 2)
				if j == 2 {
					data[t+3] |= (1 << 0)
				}
				if j == nv-1 {
					data[t+3] |= (1 << 4)
				}
				tbase++
			}
		}
	}

	// Store and create BV tree.
	if params.BuildBvTree {
		nodes := make([]bvNode, params.PolyCount*2)
		if _, ok := createBVTree(params, nodes); !ok {
			return nil, false
		}
		for i, node := range nodes {
			n := bvTreeOff + i*bvNodeSize
			for k := 0; k < 3; k++ {
				putUint16(data, n+k*2, node.bmin[k])
				putUint16(data, n+6+k*2, node.bmax[k])
			}
			putInt32(data, n+12, node.i)
		}
	}

	// Store off-mesh connections.
	n = 0
	for i := 0; i < params.OffMeshConCount; i++ {
		// Only store connections which start from this tile.
		if offMeshConClass[i*2+0] == 0xff {
			con := offMeshConsOff + n*offMeshConnectionSize
			putUint16(data, con+28, uint16(offMeshPolyBase+n))
			// Copy connection end-points.
			endPts := params.OffMeshConVerts[i*2*3:]
			for k := 0; k < 6; k++ {
				putFloat32(data, con+k*4, endPts[k])
			}
			putFloat32(data, con+24, params.OffMeshConRad[i])
			if params.OffMeshConDir[i] != 0 {
				data[con+30] = DT_OFFMESH_CON_BIDIR
			} else {
				data[con+30] = 0
			}
			data[con+31] = offMeshConClass[i*2+1]
			if params.OffMeshConUserID != nil {
				putUint32(data, con+32, params.OffMeshConUserID[i])
			}
			n++
		}
	}

	return data, true
}

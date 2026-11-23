// Runtime navigation mesh: tile storage, link building and tile-local
// queries. A faithful port of dep/recastnavigation/Detour/Source/
// DetourNavMesh.cpp (with the types from Include/DetourNavMesh.h).
//
// The tile layout types (MeshHeader, Poly, Link, BVNode, PolyDetail,
// OffMeshConnection) live in navmesh.go; this file parses serialized tile
// bytes into them and implements the runtime mesh those types describe.
// Tile data is parsed once in AddTile; the raw bytes are retained in
// MeshTile.Data exactly as C++ keeps the tile data pointer, but the parsed
// slices are the working copies (C++ mutates verts in place when snapping
// off-mesh connections; the Go slices are mutated the same way, the raw
// bytes are not re-read).
package detour

import (
	"encoding/binary"
	"math"
)

// Poly area/type accessors, mirroring dtPoly::getArea/getType/setArea/
// setType (DetourNavMesh.h).
func (p *Poly) GetArea() uint8 { return p.AreaAndType & 0x3f }
func (p *Poly) GetType() uint8 { return p.AreaAndType >> 6 }
func (p *Poly) SetArea(a uint8) {
	p.AreaAndType = (p.AreaAndType & 0xc0) | (a & 0x3f)
}
func (p *Poly) SetType(t uint8) {
	p.AreaAndType = (p.AreaAndType & 0x3f) | (t << 6)
}

// PolyRef encoding (DetourNavMesh.h, DT_POLYREF64): salt 12 bits at the
// top, then tile 21 bits, then poly 31 bits.
func (m *NavMesh) EncodePolyID(salt, it, ip uint32) PolyRef {
	return PolyRef(salt)<<(DT_POLY_BITS+DT_TILE_BITS) | PolyRef(it)<<DT_POLY_BITS | PolyRef(ip)
}

// DecodePolyID mirrors dtNavMesh::decodePolyId (DetourNavMesh.h).
func DecodePolyID(ref PolyRef) (salt, it, ip uint32) {
	salt = uint32((ref >> (DT_POLY_BITS + DT_TILE_BITS)) & ((PolyRef(1) << DT_SALT_BITS) - 1))
	it = uint32((ref >> DT_POLY_BITS) & ((PolyRef(1) << DT_TILE_BITS) - 1))
	ip = uint32(ref & ((PolyRef(1) << DT_POLY_BITS) - 1))
	return
}

func decodePolyIdTile(ref PolyRef) uint32 {
	_, it, _ := DecodePolyID(ref)
	return it
}

func decodePolyIdSalt(ref PolyRef) uint32 {
	salt, _, _ := DecodePolyID(ref)
	return salt
}

func decodePolyIdPoly(ref PolyRef) uint32 {
	_, _, ip := DecodePolyID(ref)
	return ip
}

// NavMeshParams mirrors dtNavMeshParams (DetourNavMesh.h).
type NavMeshParams struct {
	Orig       Vec3
	TileWidth  float32
	TileHeight float32
	MaxTiles   int
	MaxPolys   int
}

// MeshTile mirrors dtMeshTile (DetourNavMesh.h). Header is nil while the
// tile slot is free or the tile has been removed. Next is shared by the
// free list and the position hash chains, exactly as in C++. index is
// the tile's slot in NavMesh.tiles (C++ derives it from pointer
// arithmetic, tile - m_tiles).
type MeshTile struct {
	Salt          uint32
	LinksFreeList uint32
	Header        *MeshHeader
	Polys         []Poly
	Verts         []float32
	Links         []Link
	DetailMeshes  []PolyDetail
	DetailVerts   []float32
	DetailTris    []byte
	BVTree        []BVNode
	OffMeshCons   []OffMeshConnection
	Data          []byte
	DataSize      int
	Flags         int
	next          *MeshTile
	index         int
}

// NavMesh mirrors dtNavMesh (DetourNavMesh.h). posLookup is the tile
// hash lookup; nextFree is the free-list head.
type NavMesh struct {
	params      NavMeshParams
	orig        Vec3
	tileWidth   float32
	tileHeight  float32
	maxTiles    int
	tileLutSize int
	tileLutMask int
	posLookup   []*MeshTile
	nextFree    *MeshTile
	tiles       []*MeshTile
}

// GetParams mirrors dtNavMesh::getParams.
func (m *NavMesh) GetParams() NavMeshParams { return m.params }

// Init mirrors dtNavMesh::init(const dtNavMeshParams*)
// (DetourNavMesh.cpp).
func (m *NavMesh) Init(params *NavMeshParams) Status {
	m.params = *params
	m.orig = params.Orig
	m.tileWidth = params.TileWidth
	m.tileHeight = params.TileHeight

	m.maxTiles = params.MaxTiles
	m.tileLutSize = int(nextPow2(uint32(params.MaxTiles / 4)))
	if m.tileLutSize == 0 {
		m.tileLutSize = 1
	}
	m.tileLutMask = m.tileLutSize - 1

	m.tiles = make([]*MeshTile, m.maxTiles)
	for i := range m.tiles {
		m.tiles[i] = &MeshTile{Salt: 1, index: i}
	}
	m.posLookup = make([]*MeshTile, m.tileLutSize)
	m.nextFree = nil
	for i := m.maxTiles - 1; i >= 0; i-- {
		m.tiles[i].next = m.nextFree
		m.nextFree = m.tiles[i]
	}

	return DT_SUCCESS
}

// InitSingleTile mirrors dtNavMesh::init(unsigned char*, int, int)
// (DetourNavMesh.cpp): single-tile initialization from serialized tile
// data.
func (m *NavMesh) InitSingleTile(data []byte, flags int) Status {
	if len(data) < meshHeaderSize {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	header := parseMeshHeader(data)
	if header.Magic != DT_NAVMESH_MAGIC {
		return DT_FAILURE | DT_WRONG_MAGIC
	}
	if header.Version != DT_NAVMESH_VERSION {
		return DT_FAILURE | DT_WRONG_VERSION
	}

	params := &NavMeshParams{
		Orig:       header.Bmin,
		TileWidth:  header.Bmax[0] - header.Bmin[0],
		TileHeight: header.Bmax[2] - header.Bmin[2],
		MaxTiles:   1,
		MaxPolys:   int(header.PolyCount),
	}
	if st := m.Init(params); StatusFailed(st) {
		return st
	}
	_, st := m.AddTile(data, flags, 0)
	return st
}

// computeTileHash mirrors computeTileHash (DetourNavMesh.cpp).
func computeTileHash(x, y, mask int) int {
	const h1 = uint32(0x8da6b343)
	const h2 = uint32(0xd8163841)
	n := h1*uint32(x) + h2*uint32(y)
	return int(n & uint32(mask))
}

func allocLink(tile *MeshTile) uint32 {
	if tile.LinksFreeList == DT_NULL_LINK {
		return DT_NULL_LINK
	}
	link := tile.LinksFreeList
	tile.LinksFreeList = tile.Links[link].Next
	return link
}

func freeLink(tile *MeshTile, link uint32) {
	tile.Links[link].Next = tile.LinksFreeList
	tile.LinksFreeList = link
}

func getSlabCoord(va *Vec3, side int) float32 {
	if side == 0 || side == 4 {
		return va[0]
	} else if side == 2 || side == 6 {
		return va[2]
	}
	return 0
}

func calcSlabEndPoints(va, vb *Vec3, bmin, bmax *[2]float32, side int) {
	if side == 0 || side == 4 {
		if va[2] < vb[2] {
			bmin[0] = va[2]
			bmin[1] = va[1]
			bmax[0] = vb[2]
			bmax[1] = vb[1]
		} else {
			bmin[0] = vb[2]
			bmin[1] = vb[1]
			bmax[0] = va[2]
			bmax[1] = va[1]
		}
	} else if side == 2 || side == 6 {
		if va[0] < vb[0] {
			bmin[0] = va[0]
			bmin[1] = va[1]
			bmax[0] = vb[0]
			bmax[1] = vb[1]
		} else {
			bmin[0] = vb[0]
			bmin[1] = vb[1]
			bmax[0] = va[0]
			bmax[1] = va[1]
		}
	}
}

func overlapSlabs(amin, amax, bmin, bmax *[2]float32, px, py float32) bool {
	minx := dtMaxF(amin[0]+px, bmin[0]+px)
	maxx := dtMinF(amax[0]-px, bmax[0]-px)
	if minx > maxx {
		return false
	}

	ad := (amax[1] - amin[1]) / (amax[0] - amin[0])
	ak := amin[1] - ad*amin[0]
	bd := (bmax[1] - bmin[1]) / (bmax[0] - bmin[0])
	bk := bmin[1] - bd*bmin[0]
	aminy := ad*minx + ak
	amaxy := ad*maxx + ak
	bminy := bd*minx + bk
	bmaxy := bd*maxx + bk
	dmin := bminy - aminy
	dmax := bmaxy - amaxy

	if dmin*dmax < 0 {
		return true
	}

	thr := dtSqrF(py * 2)
	if dmin*dmin <= thr || dmax*dmax <= thr {
		return true
	}
	return false
}

// findConnectingPolys mirrors dtNavMesh::findConnectingPolys
// (DetourNavMesh.cpp). It returns the neighbour refs and their portal
// overlap areas (in slab coordinates) for the segment va-vb on the given
// side of tile, up to maxcon entries.
func (m *NavMesh) findConnectingPolys(va, vb *Vec3, tile *MeshTile, side, maxcon int) ([]PolyRef, [][2]float32) {
	if tile == nil {
		return nil, nil
	}

	var amin, amax [2]float32
	calcSlabEndPoints(va, vb, &amin, &amax, side)
	apos := getSlabCoord(va, side)

	var bmin, bmax [2]float32
	extLinkSide := uint16(DT_EXT_LINK | uint16(side))
	var con []PolyRef
	var conarea [][2]float32

	base := m.GetPolyRefBase(tile)

	for i := 0; i < len(tile.Polys); i++ {
		poly := &tile.Polys[i]
		nv := int(poly.VertCount)
		for j := 0; j < nv; j++ {
			if poly.Neis[j] != extLinkSide {
				continue
			}

			vc := tile.vertAt(int(poly.Verts[j]))
			vd := tile.vertAt(int(poly.Verts[(j+1)%nv]))
			bpos := getSlabCoord(vc, side)

			if dtAbsF(apos-bpos) > 0.01 {
				continue
			}

			calcSlabEndPoints(vc, vd, &bmin, &bmax, side)

			if !overlapSlabs(&amin, &amax, &bmin, &bmax, 0.01, tile.Header.WalkableClimb) {
				continue
			}

			if len(con) < maxcon {
				conarea = append(conarea, [2]float32{
					dtMaxF(amin[0], bmin[0]),
					dtMinF(amax[0], bmax[0]),
				})
				con = append(con, base|PolyRef(i))
			}
			break
		}
	}
	return con, conarea
}

// vertAt returns the tile vertex at index i as a Vec3 pointer into the
// tile's flat vertex array.
func (t *MeshTile) vertAt(i int) *Vec3 {
	return (*Vec3)(t.Verts[i*3:])
}

// unconnectLinks mirrors dtNavMesh::unconnectLinks (DetourNavMesh.cpp).
func (m *NavMesh) unconnectLinks(tile, target *MeshTile) {
	if tile == nil || target == nil {
		return
	}

	targetNum := decodePolyIdTile(PolyRef(m.GetTileRef(target)))

	for i := range tile.Polys {
		poly := &tile.Polys[i]
		j := poly.FirstLink
		var pj uint32 = DT_NULL_LINK
		for j != DT_NULL_LINK {
			if decodePolyIdTile(tile.Links[j].Ref) == targetNum {
				nj := tile.Links[j].Next
				if pj == DT_NULL_LINK {
					poly.FirstLink = nj
				} else {
					tile.Links[pj].Next = nj
				}
				freeLink(tile, j)
				j = nj
			} else {
				pj = j
				j = tile.Links[j].Next
			}
		}
	}
}

// connectExtLinks mirrors dtNavMesh::connectExtLinks
// (DetourNavMesh.cpp).
func (m *NavMesh) connectExtLinks(tile, target *MeshTile, side int) {
	if tile == nil {
		return
	}

	for i := range tile.Polys {
		poly := &tile.Polys[i]
		nv := int(poly.VertCount)
		for j := 0; j < nv; j++ {
			if poly.Neis[j]&DT_EXT_LINK == 0 {
				continue
			}

			dir := int(poly.Neis[j] & 0xff)
			if side != -1 && dir != side {
				continue
			}

			va := tile.vertAt(int(poly.Verts[j]))
			vb := tile.vertAt(int(poly.Verts[(j+1)%nv]))
			nei, neia := m.findConnectingPolys(va, vb, target, oppositeTile(dir), 4)
			for k := range nei {
				idx := allocLink(tile)
				if idx != DT_NULL_LINK {
					link := &tile.Links[idx]
					link.Ref = nei[k]
					link.Edge = uint8(j)
					link.Side = uint8(dir)
					link.Next = poly.FirstLink
					poly.FirstLink = idx

					if dir == 0 || dir == 4 {
						tmin := (neia[k][0] - va[2]) / (vb[2] - va[2])
						tmax := (neia[k][1] - va[2]) / (vb[2] - va[2])
						if tmin > tmax {
							tmin, tmax = tmax, tmin
						}
						link.Bmin = uint8(dtClampF(tmin, 0.0, 1.0) * 255.0)
						link.Bmax = uint8(dtClampF(tmax, 0.0, 1.0) * 255.0)
					} else if dir == 2 || dir == 6 {
						tmin := (neia[k][0] - va[0]) / (vb[0] - va[0])
						tmax := (neia[k][1] - va[0]) / (vb[0] - va[0])
						if tmin > tmax {
							tmin, tmax = tmax, tmin
						}
						link.Bmin = uint8(dtClampF(tmin, 0.0, 1.0) * 255.0)
						link.Bmax = uint8(dtClampF(tmax, 0.0, 1.0) * 255.0)
					}
				}
			}
		}
	}
}

// connectExtOffMeshLinks mirrors dtNavMesh::connectExtOffMeshLinks
// (DetourNavMesh.cpp).
func (m *NavMesh) connectExtOffMeshLinks(tile, target *MeshTile, side int) {
	if tile == nil {
		return
	}

	var oppositeSide uint8 = 0xff
	if side != -1 {
		oppositeSide = uint8(oppositeTile(side))
	}

	for i := range target.OffMeshCons {
		targetCon := &target.OffMeshCons[i]
		if targetCon.Side != oppositeSide {
			continue
		}

		targetPoly := &target.Polys[targetCon.Poly]
		if targetPoly.FirstLink == DT_NULL_LINK {
			continue
		}

		halfExtents := Vec3{targetCon.Rad, target.Header.WalkableClimb, targetCon.Rad}

		p := Vec3{targetCon.Pos[3], targetCon.Pos[4], targetCon.Pos[5]}
		nearestPt := Vec3{}
		ref := m.findNearestPolyInTile(tile, &p, &halfExtents, &nearestPt)
		if ref == 0 {
			continue
		}
		if dtSqrF(nearestPt[0]-p[0])+dtSqrF(nearestPt[2]-p[2]) > dtSqrF(targetCon.Rad) {
			continue
		}
		v := target.vertAt(int(targetPoly.Verts[1]))
		*v = nearestPt

		idx := allocLink(target)
		if idx != DT_NULL_LINK {
			link := &target.Links[idx]
			link.Ref = ref
			link.Edge = 1
			link.Side = oppositeSide
			link.Bmin = 0
			link.Bmax = 0
			link.Next = targetPoly.FirstLink
			targetPoly.FirstLink = idx
		}

		if targetCon.Flags&DT_OFFMESH_CON_BIDIR != 0 {
			tidx := allocLink(tile)
			if tidx != DT_NULL_LINK {
				landPolyIdx := decodePolyIdPoly(ref)
				landPoly := &tile.Polys[landPolyIdx]
				link := &tile.Links[tidx]
				link.Ref = m.GetPolyRefBase(target) | PolyRef(targetCon.Poly)
				link.Edge = 0xff
				if side == -1 {
					link.Side = 0xff
				} else {
					link.Side = uint8(side)
				}
				link.Bmin = 0
				link.Bmax = 0
				link.Next = landPoly.FirstLink
				landPoly.FirstLink = tidx
			}
		}
	}
}

// connectIntLinks mirrors dtNavMesh::connectIntLinks
// (DetourNavMesh.cpp).
func (m *NavMesh) connectIntLinks(tile *MeshTile) {
	if tile == nil {
		return
	}

	base := m.GetPolyRefBase(tile)

	for i := range tile.Polys {
		poly := &tile.Polys[i]
		poly.FirstLink = DT_NULL_LINK

		if poly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
			continue
		}

		for j := int(poly.VertCount) - 1; j >= 0; j-- {
			if poly.Neis[j] == 0 || (poly.Neis[j]&DT_EXT_LINK) != 0 {
				continue
			}

			idx := allocLink(tile)
			if idx != DT_NULL_LINK {
				link := &tile.Links[idx]
				link.Ref = base | PolyRef(poly.Neis[j]-1)
				link.Edge = uint8(j)
				link.Side = 0xff
				link.Bmin = 0
				link.Bmax = 0
				link.Next = poly.FirstLink
				poly.FirstLink = idx
			}
		}
	}
}

// baseOffMeshLinks mirrors dtNavMesh::baseOffMeshLinks
// (DetourNavMesh.cpp).
func (m *NavMesh) baseOffMeshLinks(tile *MeshTile) {
	if tile == nil {
		return
	}

	base := m.GetPolyRefBase(tile)

	for i := range tile.OffMeshCons {
		con := &tile.OffMeshCons[i]
		poly := &tile.Polys[con.Poly]

		halfExtents := Vec3{con.Rad, tile.Header.WalkableClimb, con.Rad}

		p := Vec3{con.Pos[0], con.Pos[1], con.Pos[2]}
		nearestPt := Vec3{}
		ref := m.findNearestPolyInTile(tile, &p, &halfExtents, &nearestPt)
		if ref == 0 {
			continue
		}
		if dtSqrF(nearestPt[0]-p[0])+dtSqrF(nearestPt[2]-p[2]) > dtSqrF(con.Rad) {
			continue
		}
		v := tile.vertAt(int(poly.Verts[0]))
		*v = nearestPt

		idx := allocLink(tile)
		if idx != DT_NULL_LINK {
			link := &tile.Links[idx]
			link.Ref = ref
			link.Edge = 0
			link.Side = 0xff
			link.Bmin = 0
			link.Bmax = 0
			link.Next = poly.FirstLink
			poly.FirstLink = idx
		}

		tidx := allocLink(tile)
		if tidx != DT_NULL_LINK {
			landPolyIdx := decodePolyIdPoly(ref)
			landPoly := &tile.Polys[landPolyIdx]
			link := &tile.Links[tidx]
			link.Ref = base | PolyRef(con.Poly)
			link.Edge = 0xff
			link.Side = 0xff
			link.Bmin = 0
			link.Bmax = 0
			link.Next = landPoly.FirstLink
			landPoly.FirstLink = tidx
		}
	}
}

// closestPointOnDetailEdges mirrors the closestPointOnDetailEdges
// template in DetourNavMesh.cpp. onlyBoundary restricts the search to
// detail-triangle edges flagged DT_DETAIL_EDGE_BOUNDARY.
func (m *NavMesh) closestPointOnDetailEdges(tile *MeshTile, polyIdx int, pos *Vec3, onlyBoundary bool) Vec3 {
	tilePoly := &tile.Polys[polyIdx]
	pd := &tile.DetailMeshes[polyIdx]

	dmin := float32(math.MaxFloat32)
	var tmin float32
	var pmin, pmax *Vec3
	var pminV, pmaxV Vec3

	for i := 0; i < int(pd.TriCount); i++ {
		tris := tile.DetailTris[(int(pd.TriBase)+i)*4:]
		const anyBoundaryEdge = (DT_DETAIL_EDGE_BOUNDARY << 0) |
			(DT_DETAIL_EDGE_BOUNDARY << 2) |
			(DT_DETAIL_EDGE_BOUNDARY << 4)
		if onlyBoundary && (tris[3]&anyBoundaryEdge) == 0 {
			continue
		}

		var v [3]*Vec3
		var vv [3]Vec3
		for j := 0; j < 3; j++ {
			if tris[j] < tilePoly.VertCount {
				vv[j] = *tile.vertAt(int(tilePoly.Verts[tris[j]]))
			} else {
				vv[j] = *(*Vec3)(tile.DetailVerts[(int(pd.VertBase)+(int(tris[j])-int(tilePoly.VertCount)))*3:])
			}
			v[j] = &vv[j]
		}

		for k, j := 0, 2; k < 3; j, k = k, k+1 {
			if (getDetailTriEdgeFlags(tris[3], j)&DT_DETAIL_EDGE_BOUNDARY) == 0 &&
				(onlyBoundary || tris[j] < tris[k]) {
				continue
			}

			d, t := distancePtSegSqr2D(pos, v[j], v[k])
			if d < dmin {
				dmin = d
				tmin = t
				pminV = *v[j]
				pmaxV = *v[k]
				pmin = &pminV
				pmax = &pmaxV
			}
		}
	}

	if pmin == nil {
		// No detail edge found (empty detail mesh); C++ would read a
		// null pointer here. Fall back to the query position.
		return *pos
	}
	var closest Vec3
	vLerp(&closest, pmin, pmax, tmin)
	return closest
}

// getDetailTriEdgeFlags mirrors dtGetDetailTriEdgeFlags
// (DetourNavMesh.h).
func getDetailTriEdgeFlags(triFlags uint8, edgeIndex int) int {
	return int(triFlags>>(edgeIndex*2)) & 0x3
}

// getPolyHeight mirrors dtNavMesh::getPolyHeight (DetourNavMesh.cpp).
// It reports the detail-mesh height at pos when pos is over the polygon.
func (m *NavMesh) getPolyHeight(tile *MeshTile, polyIdx int, pos *Vec3) (float32, bool) {
	poly := &tile.Polys[polyIdx]
	if poly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
		return 0, false
	}

	pd := &tile.DetailMeshes[polyIdx]

	verts := make([]Vec3, DT_VERTS_PER_POLYGON)
	nv := int(poly.VertCount)
	for i := 0; i < nv; i++ {
		verts[i] = *tile.vertAt(int(poly.Verts[i]))
	}

	if !pointInPolygon(pos, verts[:nv]) {
		return 0, false
	}

	for j := 0; j < int(pd.TriCount); j++ {
		t := tile.DetailTris[(int(pd.TriBase)+j)*4:]
		var v [3]Vec3
		for k := 0; k < 3; k++ {
			if t[k] < poly.VertCount {
				v[k] = *tile.vertAt(int(poly.Verts[t[k]]))
			} else {
				v[k] = *(*Vec3)(tile.DetailVerts[(int(pd.VertBase)+(int(t[k])-int(poly.VertCount)))*3:])
			}
		}
		if h, ok := closestHeightPointTriangle(pos, &v[0], &v[1], &v[2]); ok {
			return h, true
		}
	}

	closest := m.closestPointOnDetailEdges(tile, polyIdx, pos, false)
	return closest[1], true
}

// closestPointOnPoly mirrors dtNavMesh::closestPointOnPoly
// (DetourNavMesh.cpp).
func (m *NavMesh) closestPointOnPoly(ref PolyRef, pos *Vec3) (Vec3, bool) {
	tile, poly := m.GetTileAndPolyByRefUnsafe(ref)
	polyIdx := polyIndex(tile, poly)

	closest := *pos
	if h, ok := m.getPolyHeight(tile, polyIdx, pos); ok {
		closest[1] = h
		return closest, true
	}

	if poly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
		v0 := tile.vertAt(int(poly.Verts[0]))
		v1 := tile.vertAt(int(poly.Verts[1]))
		_, t := distancePtSegSqr2D(pos, v0, v1)
		vLerp(&closest, v0, v1, t)
		return closest, false
	}

	return m.closestPointOnDetailEdges(tile, polyIdx, pos, true), false
}

// polyIndex returns the index of poly within tile.Polys.
func polyIndex(tile *MeshTile, poly *Poly) int {
	for i := range tile.Polys {
		if &tile.Polys[i] == poly {
			return i
		}
	}
	return -1
}

// findNearestPolyInTile mirrors dtNavMesh::findNearestPolyInTile
// (DetourNavMesh.cpp).
func (m *NavMesh) findNearestPolyInTile(tile *MeshTile, center, halfExtents *Vec3, nearestPt *Vec3) PolyRef {
	var bmin, bmax Vec3
	vSub(&bmin, center, halfExtents)
	vAdd(&bmax, center, halfExtents)

	polys := m.queryPolygonsInTile(tile, &bmin, &bmax, 128)

	nearest := PolyRef(0)
	nearestDistanceSqr := float32(math.MaxFloat32)
	for _, ref := range polys {
		closestPtPoly, posOverPoly := m.closestPointOnPoly(ref, center)

		var diff Vec3
		vSub(&diff, center, &closestPtPoly)
		var d float32
		if posOverPoly {
			d = dtAbsF(diff[1]) - tile.Header.WalkableClimb
			if d > 0 {
				d = d * d
			} else {
				d = 0
			}
		} else {
			d = vLenSqr(&diff)
		}

		if d < nearestDistanceSqr {
			*nearestPt = closestPtPoly
			nearestDistanceSqr = d
			nearest = ref
		}
	}

	return nearest
}

// queryPolygonsInTile mirrors dtNavMesh::queryPolygonsInTile
// (DetourNavMesh.cpp). It returns the refs of the polygons overlapping
// the query box, up to maxPolys of them.
func (m *NavMesh) queryPolygonsInTile(tile *MeshTile, qmin, qmax *Vec3, maxPolys int) []PolyRef {
	var polys []PolyRef
	if len(tile.BVTree) > 0 {
		node := 0
		end := len(tile.BVTree)
		tbmin := tile.Header.Bmin
		tbmax := tile.Header.Bmax
		qfac := tile.Header.BVQuantFactor

		var bmin, bmax [3]uint16
		minx := dtClampF(qmin[0], tbmin[0], tbmax[0]) - tbmin[0]
		miny := dtClampF(qmin[1], tbmin[1], tbmax[1]) - tbmin[1]
		minz := dtClampF(qmin[2], tbmin[2], tbmax[2]) - tbmin[2]
		maxx := dtClampF(qmax[0], tbmin[0], tbmax[0]) - tbmin[0]
		maxy := dtClampF(qmax[1], tbmin[1], tbmax[1]) - tbmin[1]
		maxz := dtClampF(qmax[2], tbmin[2], tbmax[2]) - tbmin[2]
		bmin[0] = uint16(qfac*minx) & 0xfffe
		bmin[1] = uint16(qfac*miny) & 0xfffe
		bmin[2] = uint16(qfac*minz) & 0xfffe
		bmax[0] = uint16(qfac*maxx+1) | 1
		bmax[1] = uint16(qfac*maxy+1) | 1
		bmax[2] = uint16(qfac*maxz+1) | 1

		base := m.GetPolyRefBase(tile)
		for node < end {
			n := &tile.BVTree[node]
			overlap := overlapQuantBounds(bmin, bmax, n.Bmin, n.Bmax)
			isLeafNode := n.I >= 0

			if isLeafNode && overlap {
				if len(polys) < maxPolys {
					polys = append(polys, base|PolyRef(n.I))
				}
			}

			if overlap || isLeafNode {
				node++
			} else {
				node += int(-n.I)
			}
		}
		return polys
	}

	var bmin, bmax Vec3
	base := m.GetPolyRefBase(tile)
	for i := range tile.Polys {
		p := &tile.Polys[i]
		if p.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
			continue
		}
		v := tile.vertAt(int(p.Verts[0]))
		bmin = *v
		bmax = *v
		for j := 1; j < int(p.VertCount); j++ {
			v = tile.vertAt(int(p.Verts[j]))
			vMin(&bmin, v)
			vMax(&bmax, v)
		}
		if overlapBounds(qmin, qmax, &bmin, &bmax) {
			if len(polys) < maxPolys {
				polys = append(polys, base|PolyRef(i))
			}
		}
	}
	return polys
}

// AddTile mirrors dtNavMesh::addTile (DetourNavMesh.cpp). data is the
// serialized tile from CreateNavMeshData; lastRef restores a tile at a
// previous reference (0 for a fresh tile). It returns the new tile
// reference.
func (m *NavMesh) AddTile(data []byte, flags int, lastRef TileRef) (TileRef, Status) {
	if len(data) < meshHeaderSize {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	header := parseMeshHeader(data)
	if header.Magic != DT_NAVMESH_MAGIC {
		return 0, DT_FAILURE | DT_WRONG_MAGIC
	}
	if header.Version != DT_NAVMESH_VERSION {
		return 0, DT_FAILURE | DT_WRONG_VERSION
	}

	if m.GetTileAt(int(header.X), int(header.Y), int(header.Layer)) != nil {
		return 0, DT_FAILURE | DT_ALREADY_OCCUPIED
	}

	// Section sizes (dtAlign4 of the C++ struct sizes, as in addTile).
	headerSize := align4(meshHeaderSize)
	vertsSize := align4(4 * 3 * int(header.VertCount))
	polysSize := align4(polySize * int(header.PolyCount))
	linksSize := align4(linkSize * int(header.MaxLinkCount))
	detailMeshesSize := align4(polyDetailSize * int(header.DetailMeshCount))
	detailVertsSize := align4(4 * 3 * int(header.DetailVertCount))
	detailTrisSize := align4(4 * int(header.DetailTriCount))
	bvTreeSize := align4(bvNodeSize * int(header.BVNodeCount))
	offMeshLinksSize := align4(offMeshConnectionSize * int(header.OffMeshConCount))
	totalSize := headerSize + vertsSize + polysSize + linksSize +
		detailMeshesSize + detailVertsSize + detailTrisSize +
		bvTreeSize + offMeshLinksSize
	// The C++ reads the sections unchecked; the Go port validates the
	// buffer up front instead of reading out of bounds.
	if len(data) < totalSize {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}

	var tile *MeshTile
	if lastRef == 0 {
		if m.nextFree != nil {
			tile = m.nextFree
			m.nextFree = tile.next
			tile.next = nil
		}
	} else {
		tileIndex := int(decodePolyIdTile(PolyRef(lastRef)))
		if tileIndex >= m.maxTiles {
			return 0, DT_FAILURE | DT_OUT_OF_MEMORY
		}
		target := m.tiles[tileIndex]
		var prev *MeshTile
		tile = m.nextFree
		for tile != nil && tile != target {
			prev = tile
			tile = tile.next
		}
		if tile != target {
			return 0, DT_FAILURE | DT_OUT_OF_MEMORY
		}
		if prev == nil {
			m.nextFree = tile.next
		} else {
			prev.next = tile.next
		}

		tile.Salt = decodePolyIdSalt(PolyRef(lastRef))
	}

	if tile == nil {
		return 0, DT_FAILURE | DT_OUT_OF_MEMORY
	}

	h := computeTileHash(int(header.X), int(header.Y), m.tileLutMask)
	tile.next = m.posLookup[h]
	m.posLookup[h] = tile

	d := headerSize
	tile.Verts = parseVerts(data[d:], int(header.VertCount))
	d += vertsSize
	tile.Polys = parsePolys(data[d:], int(header.PolyCount))
	d += polysSize
	tile.Links = parseLinks(data[d:], int(header.MaxLinkCount))
	d += linksSize
	tile.DetailMeshes = parsePolyDetails(data[d:], int(header.DetailMeshCount))
	d += detailMeshesSize
	tile.DetailVerts = parseVerts(data[d:], int(header.DetailVertCount))
	d += detailVertsSize
	tile.DetailTris = make([]byte, int(header.DetailTriCount)*4)
	copy(tile.DetailTris, data[d:d+int(header.DetailTriCount)*4])
	d += detailTrisSize
	if bvTreeSize > 0 {
		tile.BVTree = parseBVNodes(data[d:], int(header.BVNodeCount))
	} else {
		tile.BVTree = nil
	}
	d += bvTreeSize
	tile.OffMeshCons = parseOffMeshCons(data[d:], int(header.OffMeshConCount))
	d += offMeshLinksSize

	if int(header.MaxLinkCount) > 0 {
		tile.LinksFreeList = 0
		tile.Links[int(header.MaxLinkCount)-1].Next = DT_NULL_LINK
		for i := 0; i < int(header.MaxLinkCount)-1; i++ {
			tile.Links[i].Next = uint32(i + 1)
		}
	} else {
		tile.LinksFreeList = DT_NULL_LINK
	}

	tile.Header = &header
	tile.Data = data
	tile.DataSize = len(data)
	tile.Flags = flags

	m.connectIntLinks(tile)
	m.baseOffMeshLinks(tile)
	m.connectExtOffMeshLinks(tile, tile, -1)

	const maxNeis = 32
	neis := m.GetTilesAt(int(header.X), int(header.Y), maxNeis)
	for _, nei := range neis {
		if nei == tile {
			continue
		}
		m.connectExtLinks(tile, nei, -1)
		m.connectExtLinks(nei, tile, -1)
		m.connectExtOffMeshLinks(tile, nei, -1)
		m.connectExtOffMeshLinks(nei, tile, -1)
	}

	for i := 0; i < 8; i++ {
		nneis := m.GetNeighbourTilesAt(int(header.X), int(header.Y), i, maxNeis)
		for _, nei := range nneis {
			m.connectExtLinks(tile, nei, i)
			m.connectExtLinks(nei, tile, oppositeTile(i))
			m.connectExtOffMeshLinks(tile, nei, i)
			m.connectExtOffMeshLinks(nei, tile, oppositeTile(i))
		}
	}

	return m.GetTileRef(tile), DT_SUCCESS
}

// RemoveTile mirrors dtNavMesh::removeTile (DetourNavMesh.cpp). It
// returns the tile's data when the mesh does not own it.
func (m *NavMesh) RemoveTile(ref TileRef) ([]byte, int, Status) {
	if ref == 0 {
		return nil, 0, DT_FAILURE | DT_INVALID_PARAM
	}
	tileIndex := int(decodePolyIdTile(PolyRef(ref)))
	tileSalt := decodePolyIdSalt(PolyRef(ref))
	if tileIndex >= m.maxTiles {
		return nil, 0, DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[tileIndex]
	if tile.Salt != tileSalt {
		return nil, 0, DT_FAILURE | DT_INVALID_PARAM
	}

	h := computeTileHash(int(tile.Header.X), int(tile.Header.Y), m.tileLutMask)
	var prev *MeshTile
	cur := m.posLookup[h]
	for cur != nil {
		if cur == tile {
			if prev != nil {
				prev.next = cur.next
			} else {
				m.posLookup[h] = cur.next
			}
			break
		}
		prev = cur
		cur = cur.next
	}

	const maxNeis = 32
	neis := m.GetTilesAt(int(tile.Header.X), int(tile.Header.Y), maxNeis)
	for _, nei := range neis {
		if nei == tile {
			continue
		}
		m.unconnectLinks(nei, tile)
	}

	for i := 0; i < 8; i++ {
		nneis := m.GetNeighbourTilesAt(int(tile.Header.X), int(tile.Header.Y), i, maxNeis)
		for _, nei := range nneis {
			m.unconnectLinks(nei, tile)
		}
	}

	var data []byte
	var dataSize int
	if tile.Flags&DT_TILE_FREE_DATA == 0 {
		data = tile.Data
		dataSize = tile.DataSize
	}

	tile.Header = nil
	tile.Flags = 0
	tile.LinksFreeList = 0
	tile.Polys = nil
	tile.Verts = nil
	tile.Links = nil
	tile.DetailMeshes = nil
	tile.DetailVerts = nil
	tile.DetailTris = nil
	tile.BVTree = nil
	tile.OffMeshCons = nil

	tile.Salt = (tile.Salt + 1) & ((1 << DT_SALT_BITS) - 1)
	if tile.Salt == 0 {
		tile.Salt++
	}

	tile.next = m.nextFree
	m.nextFree = tile

	return data, dataSize, DT_SUCCESS
}

// CalcTileLoc mirrors dtNavMesh::calcTileLoc (DetourNavMesh.cpp).
func (m *NavMesh) CalcTileLoc(pos *Vec3) (int, int) {
	tx := int(math.Floor(float64((pos[0] - m.orig[0]) / m.tileWidth)))
	ty := int(math.Floor(float64((pos[2] - m.orig[2]) / m.tileHeight)))
	return tx, ty
}

// GetTileAt mirrors dtNavMesh::getTileAt (DetourNavMesh.cpp).
func (m *NavMesh) GetTileAt(x, y, layer int) *MeshTile {
	h := computeTileHash(x, y, m.tileLutMask)
	for tile := m.posLookup[h]; tile != nil; tile = tile.next {
		if tile.Header != nil &&
			int(tile.Header.X) == x &&
			int(tile.Header.Y) == y &&
			int(tile.Header.Layer) == layer {
			return tile
		}
	}
	return nil
}

// GetNeighbourTilesAt mirrors dtNavMesh::getNeighbourTilesAt
// (DetourNavMesh.cpp).
func (m *NavMesh) GetNeighbourTilesAt(x, y, side, maxTiles int) []*MeshTile {
	nx, ny := x, y
	switch side {
	case 0:
		nx++
	case 1:
		nx++
		ny++
	case 2:
		ny++
	case 3:
		nx--
		ny++
	case 4:
		nx--
	case 5:
		nx--
		ny--
	case 6:
		ny--
	case 7:
		nx++
		ny--
	}
	return m.GetTilesAt(nx, ny, maxTiles)
}

// GetTilesAt mirrors dtNavMesh::getTilesAt (DetourNavMesh.cpp): all
// tiles at the grid location, all layers, up to maxTiles of them.
func (m *NavMesh) GetTilesAt(x, y, maxTiles int) []*MeshTile {
	var tiles []*MeshTile
	h := computeTileHash(x, y, m.tileLutMask)
	for tile := m.posLookup[h]; tile != nil; tile = tile.next {
		if tile.Header != nil &&
			int(tile.Header.X) == x &&
			int(tile.Header.Y) == y {
			if len(tiles) < maxTiles {
				tiles = append(tiles, tile)
			}
		}
	}
	return tiles
}

// GetTileRefAt mirrors dtNavMesh::getTileRefAt (DetourNavMesh.cpp).
func (m *NavMesh) GetTileRefAt(x, y, layer int) TileRef {
	h := computeTileHash(x, y, m.tileLutMask)
	for tile := m.posLookup[h]; tile != nil; tile = tile.next {
		if tile.Header != nil &&
			int(tile.Header.X) == x &&
			int(tile.Header.Y) == y &&
			int(tile.Header.Layer) == layer {
			return m.GetTileRef(tile)
		}
	}
	return 0
}

// GetTileByRef mirrors dtNavMesh::getTileByRef (DetourNavMesh.cpp).
func (m *NavMesh) GetTileByRef(ref TileRef) *MeshTile {
	if ref == 0 {
		return nil
	}
	tileIndex := decodePolyIdTile(PolyRef(ref))
	tileSalt := decodePolyIdSalt(PolyRef(ref))
	if int(tileIndex) >= m.maxTiles {
		return nil
	}
	tile := m.tiles[tileIndex]
	if tile.Salt != tileSalt {
		return nil
	}
	return tile
}

// GetMaxTiles mirrors dtNavMesh::getMaxTiles.
func (m *NavMesh) GetMaxTiles() int { return m.maxTiles }

// GetTile mirrors dtNavMesh::getTile: the tile at slot i.
func (m *NavMesh) GetTile(i int) *MeshTile { return m.tiles[i] }

// GetTileAndPolyByRef mirrors dtNavMesh::getTileAndPolyByRef
// (DetourNavMesh.cpp).
func (m *NavMesh) GetTileAndPolyByRef(ref PolyRef) (*MeshTile, *Poly, Status) {
	if ref == 0 {
		return nil, nil, DT_FAILURE
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}
	if int(ip) >= len(m.tiles[it].Polys) {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	return tile, &tile.Polys[ip], DT_SUCCESS
}

// GetTileAndPolyByRefUnsafe mirrors
// dtNavMesh::getTileAndPolyByRefUnsafe (DetourNavMesh.cpp). It does no
// validation; out-of-range references return nils instead of reading
// out of bounds as the C++ does.
func (m *NavMesh) GetTileAndPolyByRefUnsafe(ref PolyRef) (*MeshTile, *Poly) {
	_, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles || m.tiles[it].Header == nil || int(ip) >= len(m.tiles[it].Polys) {
		return nil, nil
	}
	tile := m.tiles[it]
	return tile, &tile.Polys[ip]
}

// IsValidPolyRef mirrors dtNavMesh::isValidPolyRef (DetourNavMesh.cpp).
func (m *NavMesh) IsValidPolyRef(ref PolyRef) bool {
	if ref == 0 {
		return false
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return false
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return false
	}
	if int(ip) >= len(m.tiles[it].Polys) {
		return false
	}
	return true
}

// GetPolyRefBase mirrors dtNavMesh::getPolyRefBase (DetourNavMesh.cpp).
func (m *NavMesh) GetPolyRefBase(tile *MeshTile) PolyRef {
	if tile == nil {
		return 0
	}
	return m.EncodePolyID(tile.Salt, uint32(tile.index), 0)
}

// GetTileRef mirrors dtNavMesh::getTileRef (DetourNavMesh.cpp).
func (m *NavMesh) GetTileRef(tile *MeshTile) TileRef {
	if tile == nil {
		return 0
	}
	return TileRef(m.EncodePolyID(tile.Salt, uint32(tile.index), 0))
}

// GetOffMeshConnectionPolyEndPoints mirrors
// dtNavMesh::getOffMeshConnectionPolyEndPoints (DetourNavMesh.cpp).
func (m *NavMesh) GetOffMeshConnectionPolyEndPoints(prevRef, polyRef PolyRef) (Vec3, Vec3, Status) {
	var startPos, endPos Vec3
	if polyRef == 0 {
		return startPos, endPos, DT_FAILURE
	}
	salt, it, ip := DecodePolyID(polyRef)
	if int(it) >= m.maxTiles {
		return startPos, endPos, DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return startPos, endPos, DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return startPos, endPos, DT_FAILURE | DT_INVALID_PARAM
	}
	poly := &tile.Polys[ip]

	if poly.GetType() != DT_POLYTYPE_OFFMESH_CONNECTION {
		return startPos, endPos, DT_FAILURE
	}

	idx0, idx1 := 0, 1
	for i := poly.FirstLink; i != DT_NULL_LINK; i = tile.Links[i].Next {
		if tile.Links[i].Edge == 0 {
			if tile.Links[i].Ref != prevRef {
				idx0 = 1
				idx1 = 0
			}
			break
		}
	}

	startPos = *tile.vertAt(int(poly.Verts[idx0]))
	endPos = *tile.vertAt(int(poly.Verts[idx1]))
	return startPos, endPos, DT_SUCCESS
}

// GetOffMeshConnectionByRef mirrors dtNavMesh::getOffMeshConnectionByRef
// (DetourNavMesh.cpp).
func (m *NavMesh) GetOffMeshConnectionByRef(ref PolyRef) *OffMeshConnection {
	if ref == 0 {
		return nil
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return nil
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return nil
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return nil
	}
	poly := &tile.Polys[ip]
	if poly.GetType() != DT_POLYTYPE_OFFMESH_CONNECTION {
		return nil
	}
	idx := int(ip) - int(tile.Header.OffMeshBase)
	if idx < 0 || idx >= len(tile.OffMeshCons) {
		return nil
	}
	return &tile.OffMeshCons[idx]
}

// SetPolyFlags mirrors dtNavMesh::setPolyFlags (DetourNavMesh.cpp).
func (m *NavMesh) SetPolyFlags(ref PolyRef, flags uint16) Status {
	if ref == 0 {
		return DT_FAILURE
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	tile.Polys[ip].Flags = flags
	return DT_SUCCESS
}

// GetPolyFlags mirrors dtNavMesh::getPolyFlags (DetourNavMesh.cpp).
func (m *NavMesh) GetPolyFlags(ref PolyRef) (uint16, Status) {
	if ref == 0 {
		return 0, DT_FAILURE
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	return tile.Polys[ip].Flags, DT_SUCCESS
}

// SetPolyArea mirrors dtNavMesh::setPolyArea (DetourNavMesh.cpp).
func (m *NavMesh) SetPolyArea(ref PolyRef, area uint8) Status {
	if ref == 0 {
		return DT_FAILURE
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return DT_FAILURE | DT_INVALID_PARAM
	}
	tile.Polys[ip].SetArea(area)
	return DT_SUCCESS
}

// GetPolyArea mirrors dtNavMesh::getPolyArea (DetourNavMesh.cpp).
func (m *NavMesh) GetPolyArea(ref PolyRef) (uint8, Status) {
	if ref == 0 {
		return 0, DT_FAILURE
	}
	salt, it, ip := DecodePolyID(ref)
	if int(it) >= m.maxTiles {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	if m.tiles[it].Salt != salt || m.tiles[it].Header == nil {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	tile := m.tiles[it]
	if int(ip) >= len(tile.Polys) {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}
	return tile.Polys[ip].GetArea(), DT_SUCCESS
}

// tileStateSize mirrors dtNavMesh::getTileStateSize: dtTileState is 16
// bytes (magic, version, tile ref); each dtPolyState is 4 bytes
// (flags uint16, area uint8, 1 tail pad byte).
func tileStateSize(tile *MeshTile) int {
	if tile == nil {
		return 0
	}
	return align4(16) + align4(4*len(tile.Polys))
}

// GetTileStateSize mirrors dtNavMesh::getTileStateSize.
func (m *NavMesh) GetTileStateSize(tile *MeshTile) int { return tileStateSize(tile) }

// StoreTileState mirrors dtNavMesh::storeTileState
// (DetourNavMesh.cpp).
func (m *NavMesh) StoreTileState(tile *MeshTile, data []byte) Status {
	sizeReq := tileStateSize(tile)
	if len(data) < sizeReq {
		return DT_FAILURE | DT_BUFFER_TOO_SMALL
	}

	binary.LittleEndian.PutUint32(data[0:], uint32(DT_NAVMESH_STATE_MAGIC))
	binary.LittleEndian.PutUint32(data[4:], uint32(DT_NAVMESH_STATE_VERSION))
	binary.LittleEndian.PutUint64(data[8:], uint64(m.GetTileRef(tile)))

	off := align4(16)
	for i := range tile.Polys {
		binary.LittleEndian.PutUint16(data[off:], tile.Polys[i].Flags)
		data[off+2] = tile.Polys[i].GetArea()
		data[off+3] = 0
		off += 4
	}
	return DT_SUCCESS
}

// RestoreTileState mirrors dtNavMesh::restoreTileState
// (DetourNavMesh.cpp).
func (m *NavMesh) RestoreTileState(tile *MeshTile, data []byte) Status {
	sizeReq := tileStateSize(tile)
	if len(data) < sizeReq {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	magic := int32(binary.LittleEndian.Uint32(data[0:]))
	version := int32(binary.LittleEndian.Uint32(data[4:]))
	ref := TileRef(binary.LittleEndian.Uint64(data[8:]))

	if magic != DT_NAVMESH_STATE_MAGIC {
		return DT_FAILURE | DT_WRONG_MAGIC
	}
	if version != DT_NAVMESH_STATE_VERSION {
		return DT_FAILURE | DT_WRONG_VERSION
	}
	if ref != m.GetTileRef(tile) {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	off := align4(16)
	for i := range tile.Polys {
		tile.Polys[i].Flags = binary.LittleEndian.Uint16(data[off:])
		tile.Polys[i].SetArea(data[off+2])
		off += 4
	}
	return DT_SUCCESS
}

// Tile wire parsing. Field offsets match the C++ struct layouts in
// DetourNavMesh.h (see the wire sizes in navmesh.go); all reads are
// little-endian, as CreateNavMeshData writes them.

func parseMeshHeader(data []byte) MeshHeader {
	le := binary.LittleEndian
	h := MeshHeader{
		Magic:           int32(le.Uint32(data[0:])),
		Version:         int32(le.Uint32(data[4:])),
		X:               int32(le.Uint32(data[8:])),
		Y:               int32(le.Uint32(data[12:])),
		Layer:           int32(le.Uint32(data[16:])),
		UserID:          le.Uint32(data[20:]),
		PolyCount:       int32(le.Uint32(data[24:])),
		VertCount:       int32(le.Uint32(data[28:])),
		MaxLinkCount:    int32(le.Uint32(data[32:])),
		DetailMeshCount: int32(le.Uint32(data[36:])),
		DetailVertCount: int32(le.Uint32(data[40:])),
		DetailTriCount:  int32(le.Uint32(data[44:])),
		BVNodeCount:     int32(le.Uint32(data[48:])),
		OffMeshConCount: int32(le.Uint32(data[52:])),
		OffMeshBase:     int32(le.Uint32(data[56:])),
		WalkableHeight:  math.Float32frombits(le.Uint32(data[60:])),
		WalkableRadius:  math.Float32frombits(le.Uint32(data[64:])),
		WalkableClimb:   math.Float32frombits(le.Uint32(data[68:])),
		BVQuantFactor:   math.Float32frombits(le.Uint32(data[96:])),
	}
	for i := 0; i < 3; i++ {
		h.Bmin[i] = math.Float32frombits(le.Uint32(data[72+i*4:]))
		h.Bmax[i] = math.Float32frombits(le.Uint32(data[84+i*4:]))
	}
	return h
}

func parseVerts(data []byte, count int) []float32 {
	verts := make([]float32, count*3)
	le := binary.LittleEndian
	for i := range verts {
		verts[i] = math.Float32frombits(le.Uint32(data[i*4:]))
	}
	return verts
}

func parsePolys(data []byte, count int) []Poly {
	polys := make([]Poly, count)
	le := binary.LittleEndian
	for i := range polys {
		off := i * polySize
		p := &polys[i]
		p.FirstLink = le.Uint32(data[off:])
		for j := 0; j < DT_VERTS_PER_POLYGON; j++ {
			p.Verts[j] = le.Uint16(data[off+4+j*2:])
			p.Neis[j] = le.Uint16(data[off+16+j*2:])
		}
		p.Flags = le.Uint16(data[off+28:])
		p.VertCount = data[off+30]
		p.AreaAndType = data[off+31]
	}
	return polys
}

func parseLinks(data []byte, count int) []Link {
	links := make([]Link, count)
	le := binary.LittleEndian
	for i := range links {
		off := i * linkSize
		l := &links[i]
		l.Ref = PolyRef(le.Uint64(data[off:]))
		l.Next = le.Uint32(data[off+8:])
		l.Edge = data[off+12]
		l.Side = data[off+13]
		l.Bmin = data[off+14]
		l.Bmax = data[off+15]
	}
	return links
}

func parsePolyDetails(data []byte, count int) []PolyDetail {
	details := make([]PolyDetail, count)
	le := binary.LittleEndian
	for i := range details {
		off := i * polyDetailSize
		d := &details[i]
		d.VertBase = le.Uint32(data[off:])
		d.TriBase = le.Uint32(data[off+4:])
		d.VertCount = data[off+8]
		d.TriCount = data[off+9]
	}
	return details
}

func parseBVNodes(data []byte, count int) []BVNode {
	nodes := make([]BVNode, count)
	le := binary.LittleEndian
	for i := range nodes {
		off := i * bvNodeSize
		n := &nodes[i]
		for j := 0; j < 3; j++ {
			n.Bmin[j] = le.Uint16(data[off+j*2:])
			n.Bmax[j] = le.Uint16(data[off+6+j*2:])
		}
		n.I = int32(le.Uint32(data[off+12:]))
	}
	return nodes
}

func parseOffMeshCons(data []byte, count int) []OffMeshConnection {
	cons := make([]OffMeshConnection, count)
	le := binary.LittleEndian
	for i := range cons {
		off := i * offMeshConnectionSize
		c := &cons[i]
		for j := 0; j < 6; j++ {
			c.Pos[j] = math.Float32frombits(le.Uint32(data[off+j*4:]))
		}
		c.Rad = math.Float32frombits(le.Uint32(data[off+24:]))
		c.Poly = le.Uint16(data[off+28:])
		c.Flags = data[off+30]
		c.Side = data[off+31]
		c.UserID = le.Uint32(data[off+32:])
	}
	return cons
}

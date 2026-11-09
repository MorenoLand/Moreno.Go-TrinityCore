// Tile layout types for the Detour navigation-mesh tile format
// (dep/recastnavigation/Detour/Include/DetourNavMesh.h). CreateNavMeshData
// in navmeshbuilder.go writes these fields explicitly in little-endian
// order; the structs below mirror the C++ field order for reference and
// for parsing tile data back, and are never binary-written as Go structs
// (Go padding would differ from the C++ layout).
package detour

// CreateParams is the tile creation params type: an alias for
// NavMeshCreateParams, which mirrors dtNavMeshCreateParams
// (DetourNavMeshBuilder.h) field for field. CreateNavMeshData takes
// *CreateParams.
//
// Note on Verts: the brief draft listed Verts as []float32, but the C++
// field is voxel coordinates (const unsigned short*, dequantized in
// dtCreateNavMeshData as bmin + v*cs/ch), so Verts is []uint16 here;
// DetailVerts is the []float32 field.
type CreateParams = NavMeshCreateParams

// MeshHeader mirrors dtMeshHeader (DetourNavMesh.h). Wire size is 100
// bytes: 15 int32/uint32 fields, 3 float32 walkable values, bmin/bmax,
// bvQuantFactor.
type MeshHeader struct {
	Magic           int32
	Version         int32
	X               int32
	Y               int32
	Layer           int32
	UserID          uint32
	PolyCount       int32
	VertCount       int32
	MaxLinkCount    int32
	DetailMeshCount int32
	DetailVertCount int32
	DetailTriCount  int32
	BVNodeCount     int32
	OffMeshConCount int32
	OffMeshBase     int32
	WalkableHeight  float32
	WalkableRadius  float32
	WalkableClimb   float32
	Bmin            [3]float32
	Bmax            [3]float32
	BVQuantFactor   float32
}

// Poly mirrors dtPoly (DetourNavMesh.h). Wire size is 32 bytes.
// AreaAndType packs the area id (low 6 bits) and the polygon type
// (high 2 bits, dtPolyTypes); FirstLink is 0 in freshly created tile
// data and is set when the tile is loaded.
type Poly struct {
	FirstLink   uint32
	Verts       [DT_VERTS_PER_POLYGON]uint16
	Neis        [DT_VERTS_PER_POLYGON]uint16
	Flags       uint16
	VertCount   uint8
	AreaAndType uint8
}

// PolyDetail mirrors dtPolyDetail (DetourNavMesh.h). Wire size is 12
// bytes (10 bytes of fields plus 2 bytes tail padding).
type PolyDetail struct {
	VertBase  uint32
	TriBase   uint32
	VertCount uint8
	TriCount  uint8
}

// BVNode mirrors dtBVNode (DetourNavMesh.h). Wire size is 16 bytes. I
// is the polygon index for a leaf, or the negative escape count for an
// interior node.
type BVNode struct {
	Bmin [3]uint16
	Bmax [3]uint16
	I    int32
}

// Link mirrors dtLink (DetourNavMesh.h). Wire size is 16 bytes because
// DT_POLYREF64 makes dtPolyRef a uint64 in this tree. Links are never
// written by dtCreateNavMeshData: the region is zeroed and links are
// rebuilt when the tile is loaded.
type Link struct {
	Ref  uint64
	Next uint32
	Edge uint8
	Side uint8
	Bmin uint8
	Bmax uint8
}

// OffMeshConnection mirrors dtOffMeshConnection (DetourNavMesh.h). Wire
// size is 36 bytes: endpoints (ax, ay, az, bx, by, bz), radius,
// polygon, flags, side, user id.
type OffMeshConnection struct {
	Pos    [6]float32
	Rad    float32
	Poly   uint16
	Flags  uint8
	Side   uint8
	UserID uint32
}

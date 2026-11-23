// Navigation-mesh query object: pathfinding and local queries against a
// NavMesh. A faithful port of dep/recastnavigation/Detour/Source/
// DetourNavMeshQuery.cpp (with the types from Include/
// DetourNavMeshQuery.h).
//
// Go API notes: C++ out-parameters become multiple return values. Sliced
// pathfinding (initSlicedFindPath/updateSlicedFindPath/
// finalizeSlicedFindPath) is not ported in this strip; everything else in
// DetourNavMeshQuery.h is here.
package detour

import "math"

// H_SCALE is the search heuristic scale (DetourNavMeshQuery.cpp).
const hScale = 0.999

// Raycast options (dtRaycastOptions, DetourNavMesh.h).
const (
	// DT_RAYCAST_USE_COSTS: raycast calculates movement cost along the ray.
	DT_RAYCAST_USE_COSTS = 0x01
)

// Straight-path flags (dtStraightPathFlags, DetourNavMesh.h).
const (
	DT_STRAIGHTPATH_START              = 0x01
	DT_STRAIGHTPATH_END                = 0x02
	DT_STRAIGHTPATH_OFFMESH_CONNECTION = 0x04
)

// Straight-path options (dtStraightPathOptions, DetourNavMesh.h).
const (
	DT_STRAIGHTPATH_AREA_CROSSINGS = 0x01
	DT_STRAIGHTPATH_ALL_CROSSINGS  = 0x02
)

// Find-path options (dtFindPathOptions, DetourNavMesh.h).
const (
	// DT_FINDPATH_ANY_ANGLE: use raycasts during pathfind to shortcut.
	DT_FINDPATH_ANY_ANGLE = 0x02
)

// QueryFilter mirrors dtQueryFilter (DetourNavMeshQuery.h): polygon
// filtering and traversal costs. The default implementation is used
// (DT_VIRTUAL_QUERYFILTER is not defined in this tree).
type QueryFilter struct {
	areaCost     [DT_MAX_AREAS]float32
	includeFlags uint16
	excludeFlags uint16
}

// NewQueryFilter mirrors dtQueryFilter::dtQueryFilter: all area costs
// 1.0, all flags included, none excluded.
func NewQueryFilter() *QueryFilter {
	f := &QueryFilter{includeFlags: 0xffff}
	for i := range f.areaCost {
		f.areaCost[i] = 1.0
	}
	return f
}

// PassFilter mirrors dtQueryFilter::passFilter.
func (f *QueryFilter) PassFilter(ref PolyRef, tile *MeshTile, poly *Poly) bool {
	return (poly.Flags&f.includeFlags) != 0 && (poly.Flags&f.excludeFlags) == 0
}

// GetCost mirrors dtQueryFilter::getCost: the distance between pa and pb
// scaled by the current polygon's area cost.
func (f *QueryFilter) GetCost(pa, pb Vec3, prevRef PolyRef, prevTile *MeshTile, prevPoly *Poly, curRef PolyRef, curTile *MeshTile, curPoly *Poly, nextRef PolyRef, nextTile *MeshTile, nextPoly *Poly) float32 {
	return vDist(&pa, &pb) * f.areaCost[curPoly.GetArea()]
}

// GetAreaCost mirrors dtQueryFilter::getAreaCost.
func (f *QueryFilter) GetAreaCost(i int) float32 { return f.areaCost[i] }

// SetAreaCost mirrors dtQueryFilter::setAreaCost.
func (f *QueryFilter) SetAreaCost(i int, cost float32) { f.areaCost[i] = cost }

// GetIncludeFlags mirrors dtQueryFilter::getIncludeFlags.
func (f *QueryFilter) GetIncludeFlags() uint16 { return f.includeFlags }

// SetIncludeFlags mirrors dtQueryFilter::setIncludeFlags.
func (f *QueryFilter) SetIncludeFlags(flags uint16) { f.includeFlags = flags }

// GetExcludeFlags mirrors dtQueryFilter::getExcludeFlags.
func (f *QueryFilter) GetExcludeFlags() uint16 { return f.excludeFlags }

// SetExcludeFlags mirrors dtQueryFilter::setExcludeFlags.
func (f *QueryFilter) SetExcludeFlags(flags uint16) { f.excludeFlags = flags }

// PolyQuery mirrors dtPolyQuery (DetourNavMeshQuery.h): a callback
// receiving batches of polygons found by QueryPolygons.
type PolyQuery interface {
	Process(tile *MeshTile, polys []*Poly, refs []PolyRef)
}

// NavMeshQuery mirrors dtNavMeshQuery (DetourNavMeshQuery.h).
type NavMeshQuery struct {
	nav          *NavMesh
	tinyNodePool *NodePool
	nodePool     *NodePool
	openList     *NodeQueue
}

// NewNavMeshQuery mirrors dtAllocNavMeshQuery.
func NewNavMeshQuery() *NavMeshQuery { return &NavMeshQuery{} }

// Init mirrors dtNavMeshQuery::init (DetourNavMeshQuery.cpp). maxNodes
// must be in (0, 65535]; the C++ asserts the lower bound, the Go port
// returns DT_FAILURE | DT_INVALID_PARAM for it.
func (q *NavMeshQuery) Init(nav *NavMesh, maxNodes int) Status {
	if maxNodes <= 0 || maxNodes > int(DT_NULL_IDX) || maxNodes > (1<<DT_NODE_PARENT_BITS)-1 {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	q.nav = nav

	if q.nodePool == nil || q.nodePool.MaxNodes() < maxNodes {
		hashSize := int(nextPow2(uint32(maxNodes / 4)))
		if hashSize == 0 {
			hashSize = 1
		}
		q.nodePool = NewNodePool(maxNodes, hashSize)
	} else {
		q.nodePool.Clear()
	}

	if q.tinyNodePool == nil {
		q.tinyNodePool = NewNodePool(64, 32)
	} else {
		q.tinyNodePool.Clear()
	}

	if q.openList == nil || q.openList.Capacity() < maxNodes {
		q.openList = NewNodeQueue(maxNodes)
	} else {
		q.openList.Clear()
	}

	return DT_SUCCESS
}

// GetAttachedNavMesh mirrors dtNavMeshQuery::getAttachedNavMesh.
func (q *NavMeshQuery) GetAttachedNavMesh() *NavMesh { return q.nav }

// GetNodePool mirrors dtNavMeshQuery::getNodePool.
func (q *NavMeshQuery) GetNodePool() *NodePool { return q.nodePool }

// findNearestPolyQuery mirrors dtFindNearestPolyQuery
// (DetourNavMeshQuery.cpp).
type findNearestPolyQuery struct {
	query              *NavMeshQuery
	center             Vec3
	nearestDistanceSqr float32
	nearestRef         PolyRef
	nearestPoint       Vec3
}

func newFindNearestPolyQuery(query *NavMeshQuery, center Vec3) *findNearestPolyQuery {
	return &findNearestPolyQuery{
		query:              query,
		center:             center,
		nearestDistanceSqr: float32(math.MaxFloat32),
	}
}

func (pq *findNearestPolyQuery) Process(tile *MeshTile, polys []*Poly, refs []PolyRef) {
	for _, ref := range refs {
		closestPtPoly, posOverPoly, _ := pq.query.ClosestPointOnPoly(ref, pq.center)

		var diff Vec3
		vSub(&diff, &pq.center, &closestPtPoly)
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

		if d < pq.nearestDistanceSqr {
			pq.nearestPoint = closestPtPoly
			pq.nearestDistanceSqr = d
			pq.nearestRef = ref
		}
	}
}

// FindNearestPoly mirrors dtNavMeshQuery::findNearestPoly
// (DetourNavMeshQuery.cpp). It returns the nearest polygon reference (0
// when the search box touches no polygon) and the nearest point on it;
// the point is only meaningful when the reference is non-zero.
func (q *NavMeshQuery) FindNearestPoly(center, halfExtents Vec3, filter *QueryFilter) (PolyRef, Vec3, Status) {
	query := newFindNearestPolyQuery(q, center)

	if st := q.QueryPolygonsCallback(center, halfExtents, filter, query); StatusFailed(st) {
		return 0, Vec3{}, st
	}

	if query.nearestRef == 0 {
		return 0, Vec3{}, DT_SUCCESS
	}
	return query.nearestRef, query.nearestPoint, DT_SUCCESS
}

// queryPolygonsInTile mirrors dtNavMeshQuery::queryPolygonsInTile
// (DetourNavMeshQuery.cpp). Note the C++ asymmetry, preserved here:
// the BV-tree branch does NOT skip off-mesh connection polygons, while
// the brute-force branch does.
func (q *NavMeshQuery) queryPolygonsInTile(tile *MeshTile, qmin, qmax *Vec3, filter *QueryFilter, query PolyQuery) {
	const batchSize = 32
	var polyRefs []PolyRef
	var polys []*Poly
	flush := func() {
		if len(polyRefs) > 0 {
			query.Process(tile, polys, polyRefs)
			polyRefs = polyRefs[:0]
			polys = polys[:0]
		}
	}
	push := func(ref PolyRef, poly *Poly) {
		polyRefs = append(polyRefs, ref)
		polys = append(polys, poly)
		if len(polyRefs) == batchSize {
			flush()
		}
	}

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

		base := q.nav.GetPolyRefBase(tile)
		for node < end {
			n := &tile.BVTree[node]
			overlap := overlapQuantBounds(bmin, bmax, n.Bmin, n.Bmax)
			isLeafNode := n.I >= 0

			if isLeafNode && overlap {
				ref := base | PolyRef(n.I)
				if filter.PassFilter(ref, tile, &tile.Polys[n.I]) {
					push(ref, &tile.Polys[n.I])
				}
			}

			if overlap || isLeafNode {
				node++
			} else {
				node += int(-n.I)
			}
		}
	} else {
		var bmin, bmax Vec3
		base := q.nav.GetPolyRefBase(tile)
		for i := range tile.Polys {
			p := &tile.Polys[i]
			if p.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
				continue
			}
			ref := base | PolyRef(i)
			if !filter.PassFilter(ref, tile, p) {
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
				push(ref, p)
			}
		}
	}

	flush()
}

// collectPolysQuery mirrors dtCollectPolysQuery
// (DetourNavMeshQuery.cpp).
type collectPolysQuery struct {
	polys    []PolyRef
	maxPolys int
	overflow bool
}

func (c *collectPolysQuery) Process(tile *MeshTile, polys []*Poly, refs []PolyRef) {
	numLeft := c.maxPolys - len(c.polys)
	toCopy := len(refs)
	if toCopy > numLeft {
		c.overflow = true
		toCopy = numLeft
	}
	c.polys = append(c.polys, refs[:toCopy]...)
}

// QueryPolygons mirrors dtNavMeshQuery::queryPolygons (the array
// overload, DetourNavMeshQuery.cpp). It returns the refs of the
// polygons overlapping the search box, up to maxPolys of them; the
// status carries DT_BUFFER_TOO_SMALL when the result was truncated.
func (q *NavMeshQuery) QueryPolygons(center, halfExtents Vec3, filter *QueryFilter, maxPolys int) ([]PolyRef, Status) {
	if maxPolys < 0 {
		return nil, DT_FAILURE | DT_INVALID_PARAM
	}

	collector := &collectPolysQuery{maxPolys: maxPolys}
	if st := q.QueryPolygonsCallback(center, halfExtents, filter, collector); StatusFailed(st) {
		return nil, st
	}
	if collector.overflow {
		return collector.polys, DT_SUCCESS | DT_BUFFER_TOO_SMALL
	}
	return collector.polys, DT_SUCCESS
}

// QueryPolygonsCallback mirrors dtNavMeshQuery::queryPolygons (the
// dtPolyQuery overload, DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) QueryPolygonsCallback(center, halfExtents Vec3, filter *QueryFilter, query PolyQuery) Status {
	if !vIsFinite(&center) || !vIsFinite(&halfExtents) || filter == nil || query == nil {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	var bmin, bmax Vec3
	vSub(&bmin, &center, &halfExtents)
	vAdd(&bmax, &center, &halfExtents)

	minx, miny := q.nav.CalcTileLoc(&bmin)
	maxx, maxy := q.nav.CalcTileLoc(&bmax)

	const maxNeis = 32
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			neis := q.nav.GetTilesAt(x, y, maxNeis)
			for _, tile := range neis {
				q.queryPolygonsInTile(tile, &bmin, &bmax, filter, query)
			}
		}
	}

	return DT_SUCCESS
}

// FindPath mirrors dtNavMeshQuery::findPath (DetourNavMeshQuery.cpp).
// It returns the polygon corridor from the start polygon to the end
// polygon; when the end cannot be reached the path ends at the polygon
// nearest the end and the status carries DT_PARTIAL_RESULT.
func (q *NavMeshQuery) FindPath(startRef, endRef PolyRef, startPos, endPos Vec3, filter *QueryFilter, maxPath int) ([]PolyRef, Status) {
	if !q.nav.IsValidPolyRef(startRef) || !q.nav.IsValidPolyRef(endRef) ||
		!vIsFinite(&startPos) || !vIsFinite(&endPos) ||
		filter == nil || maxPath <= 0 {
		return nil, DT_FAILURE | DT_INVALID_PARAM
	}

	if startRef == endRef {
		return []PolyRef{startRef}, DT_SUCCESS
	}

	q.nodePool.Clear()
	q.openList.Clear()

	startNode := q.nodePool.GetNode(startRef, 0)
	startNode.Pos = startPos
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = vDist(&startPos, &endPos) * hScale
	startNode.ID = startRef
	startNode.Flags = DT_NODE_OPEN
	q.openList.Push(startNode)

	lastBestNode := startNode
	lastBestNodeCost := startNode.Total

	outOfNodes := false

	for !q.openList.Empty() {
		bestNode := q.openList.Pop()
		bestNode.Flags &^= DT_NODE_OPEN
		bestNode.Flags |= DT_NODE_CLOSED

		if bestNode.ID == endRef {
			lastBestNode = bestNode
			break
		}

		bestRef := bestNode.ID
		bestTile, bestPoly := q.nav.GetTileAndPolyByRefUnsafe(bestRef)

		var parentRef PolyRef
		var parentTile *MeshTile
		var parentPoly *Poly
		if bestNode.Pidx != 0 {
			parentRef = q.nodePool.GetNodeAtIdx(bestNode.Pidx).ID
		}
		if parentRef != 0 {
			parentTile, parentPoly = q.nav.GetTileAndPolyByRefUnsafe(parentRef)
		}

		for i := bestPoly.FirstLink; i != DT_NULL_LINK; i = bestTile.Links[i].Next {
			neighbourRef := bestTile.Links[i].Ref

			if neighbourRef == 0 || neighbourRef == parentRef {
				continue
			}

			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			var crossSide uint8
			if bestTile.Links[i].Side != 0xff {
				crossSide = bestTile.Links[i].Side >> 1
			}

			neighbourNode := q.nodePool.GetNode(neighbourRef, crossSide)
			if neighbourNode == nil {
				outOfNodes = true
				continue
			}

			if neighbourNode.Flags == 0 {
				// The C++ ignores the return status here; on failure
				// the node position is left untouched.
				if mid, st := q.getEdgeMidPoint(bestRef, bestPoly, bestTile, neighbourRef, neighbourPoly, neighbourTile); !StatusFailed(st) {
					neighbourNode.Pos = mid
				}
			}

			var cost, heuristic float32

			if neighbourRef == endRef {
				curCost := filter.GetCost(bestNode.Pos, neighbourNode.Pos,
					parentRef, parentTile, parentPoly,
					bestRef, bestTile, bestPoly,
					neighbourRef, neighbourTile, neighbourPoly)
				endCost := filter.GetCost(neighbourNode.Pos, endPos,
					bestRef, bestTile, bestPoly,
					neighbourRef, neighbourTile, neighbourPoly,
					0, nil, nil)
				cost = bestNode.Cost + curCost + endCost
				heuristic = 0
			} else {
				curCost := filter.GetCost(bestNode.Pos, neighbourNode.Pos,
					parentRef, parentTile, parentPoly,
					bestRef, bestTile, bestPoly,
					neighbourRef, neighbourTile, neighbourPoly)
				cost = bestNode.Cost + curCost
				heuristic = vDist(&neighbourNode.Pos, &endPos) * hScale
			}

			total := cost + heuristic

			if (neighbourNode.Flags&DT_NODE_OPEN) != 0 && total >= neighbourNode.Total {
				continue
			}
			if (neighbourNode.Flags&DT_NODE_CLOSED) != 0 && total >= neighbourNode.Total {
				continue
			}

			neighbourNode.Pidx = q.nodePool.GetNodeIdx(bestNode)
			neighbourNode.ID = neighbourRef
			neighbourNode.Flags &^= DT_NODE_CLOSED
			neighbourNode.Cost = cost
			neighbourNode.Total = total

			if (neighbourNode.Flags & DT_NODE_OPEN) != 0 {
				q.openList.Modify(neighbourNode)
			} else {
				neighbourNode.Flags |= DT_NODE_OPEN
				q.openList.Push(neighbourNode)
			}

			if heuristic < lastBestNodeCost {
				lastBestNodeCost = heuristic
				lastBestNode = neighbourNode
			}
		}
	}

	path, status := q.getPathToNode(lastBestNode, maxPath)

	if lastBestNode.ID != endRef {
		status |= DT_PARTIAL_RESULT
	}
	if outOfNodes {
		status |= DT_OUT_OF_NODES
	}

	return path, status
}

// getPathToNode mirrors dtNavMeshQuery::getPathToNode
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) getPathToNode(endNode *Node, maxPath int) ([]PolyRef, Status) {
	curNode := endNode
	length := 0
	for curNode != nil {
		length++
		curNode = q.nodePool.GetNodeAtIdx(curNode.Pidx)
	}

	curNode = endNode
	writeCount := length
	for writeCount > maxPath {
		curNode = q.nodePool.GetNodeAtIdx(curNode.Pidx)
		writeCount--
	}

	path := make([]PolyRef, writeCount)
	for i := writeCount - 1; i >= 0; i-- {
		path[i] = curNode.ID
		curNode = q.nodePool.GetNodeAtIdx(curNode.Pidx)
	}

	if length > maxPath {
		return path, DT_SUCCESS | DT_BUFFER_TOO_SMALL
	}
	return path, DT_SUCCESS
}

// GetPortalPoints mirrors dtNavMeshQuery::getPortalPoints (the ref
// overload, DetourNavMeshQuery.cpp). It returns the portal edge between
// two polygons and their polygon types.
func (q *NavMeshQuery) GetPortalPoints(from, to PolyRef) (Vec3, Vec3, uint8, uint8, Status) {
	var left, right Vec3
	fromTile, fromPoly, st := q.nav.GetTileAndPolyByRef(from)
	if StatusFailed(st) {
		return left, right, 0, 0, DT_FAILURE | DT_INVALID_PARAM
	}
	fromType := fromPoly.GetType()

	toTile, toPoly, st := q.nav.GetTileAndPolyByRef(to)
	if StatusFailed(st) {
		return left, right, 0, 0, DT_FAILURE | DT_INVALID_PARAM
	}
	toType := toPoly.GetType()

	left, right, st = q.getPortalPoints(from, fromPoly, fromTile, to, toPoly, toTile)
	return left, right, fromType, toType, st
}

// getPortalPoints mirrors dtNavMeshQuery::getPortalPoints (the tile/poly
// overload, DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) getPortalPoints(from PolyRef, fromPoly *Poly, fromTile *MeshTile, to PolyRef, toPoly *Poly, toTile *MeshTile) (Vec3, Vec3, Status) {
	var left, right Vec3

	var link *Link
	for i := fromPoly.FirstLink; i != DT_NULL_LINK; i = fromTile.Links[i].Next {
		if fromTile.Links[i].Ref == to {
			link = &fromTile.Links[i]
			break
		}
	}
	if link == nil {
		return left, right, DT_FAILURE | DT_INVALID_PARAM
	}

	if fromPoly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
		for i := fromPoly.FirstLink; i != DT_NULL_LINK; i = fromTile.Links[i].Next {
			if fromTile.Links[i].Ref == to {
				v := int(fromTile.Links[i].Edge)
				left = *fromTile.vertAt(int(fromPoly.Verts[v]))
				right = left
				return left, right, DT_SUCCESS
			}
		}
		return left, right, DT_FAILURE | DT_INVALID_PARAM
	}

	if toPoly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
		for i := toPoly.FirstLink; i != DT_NULL_LINK; i = toTile.Links[i].Next {
			if toTile.Links[i].Ref == from {
				v := int(toTile.Links[i].Edge)
				left = *toTile.vertAt(int(toPoly.Verts[v]))
				right = left
				return left, right, DT_SUCCESS
			}
		}
		return left, right, DT_FAILURE | DT_INVALID_PARAM
	}

	v0 := int(fromPoly.Verts[link.Edge])
	v1 := int(fromPoly.Verts[(int(link.Edge)+1)%int(fromPoly.VertCount)])
	left = *fromTile.vertAt(v0)
	right = *fromTile.vertAt(v1)

	if link.Side != 0xff {
		if link.Bmin != 0 || link.Bmax != 255 {
			s := float32(1.0) / 255.0
			tmin := float32(link.Bmin) * s
			tmax := float32(link.Bmax) * s
			vLerp(&left, fromTile.vertAt(v0), fromTile.vertAt(v1), tmin)
			vLerp(&right, fromTile.vertAt(v0), fromTile.vertAt(v1), tmax)
		}
	}

	return left, right, DT_SUCCESS
}

// GetEdgeMidPoint mirrors dtNavMeshQuery::getEdgeMidPoint (the ref
// overload, DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) GetEdgeMidPoint(from, to PolyRef) (Vec3, Status) {
	left, right, _, _, st := q.GetPortalPoints(from, to)
	if StatusFailed(st) {
		return Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}
	return Vec3{
		(left[0] + right[0]) * 0.5,
		(left[1] + right[1]) * 0.5,
		(left[2] + right[2]) * 0.5,
	}, DT_SUCCESS
}

// getEdgeMidPoint mirrors dtNavMeshQuery::getEdgeMidPoint (the tile/poly
// overload, DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) getEdgeMidPoint(from PolyRef, fromPoly *Poly, fromTile *MeshTile, to PolyRef, toPoly *Poly, toTile *MeshTile) (Vec3, Status) {
	left, right, st := q.getPortalPoints(from, fromPoly, fromTile, to, toPoly, toTile)
	if StatusFailed(st) {
		return Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}
	return Vec3{
		(left[0] + right[0]) * 0.5,
		(left[1] + right[1]) * 0.5,
		(left[2] + right[2]) * 0.5,
	}, DT_SUCCESS
}

// RaycastHit mirrors dtRaycastHit (DetourNavMeshQuery.h).
type RaycastHit struct {
	// T is the hit parameter (FLT_MAX when no wall was hit).
	T float32
	// HitNormal is the normal of the nearest wall hit.
	HitNormal Vec3
	// HitEdgeIndex is the index of the edge on the final polygon where
	// the wall was hit.
	HitEdgeIndex int
	// Path is the visited polygons.
	Path []PolyRef
	// PathCount is the number of visited polygons (len(Path)).
	PathCount int
	// MaxPath is the maximum number of polygons Path can hold.
	MaxPath int
	// PathCost is the cost of the path until hit.
	PathCost float32
}

// Raycast mirrors dtNavMeshQuery::raycast (the dtRaycastHit overload,
// DetourNavMeshQuery.cpp). hit is filled in place; its Path is reset
// and refilled up to hit.MaxPath entries.
func (q *NavMeshQuery) Raycast(startRef PolyRef, startPos, endPos Vec3, filter *QueryFilter, options uint32, hit *RaycastHit, prevRef PolyRef) Status {
	if hit == nil {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	hit.T = 0
	hit.Path = hit.Path[:0]
	hit.PathCount = 0
	hit.PathCost = 0

	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&startPos) || !vIsFinite(&endPos) ||
		filter == nil ||
		(prevRef != 0 && !q.nav.IsValidPolyRef(prevRef)) {
		return DT_FAILURE | DT_INVALID_PARAM
	}

	var dir, curPos, lastPos Vec3
	verts := make([]Vec3, 0, DT_VERTS_PER_POLYGON+1)

	curPos = startPos
	vSub(&dir, &endPos, &startPos)
	hit.HitNormal = Vec3{}

	status := DT_SUCCESS

	var prevTile, tile, nextTile *MeshTile
	var prevPoly, poly, nextPoly *Poly
	curRef := startRef

	tile, poly = q.nav.GetTileAndPolyByRefUnsafe(curRef)
	nextTile, prevTile = tile, tile
	nextPoly, prevPoly = poly, poly
	if prevRef != 0 {
		prevTile, prevPoly = q.nav.GetTileAndPolyByRefUnsafe(prevRef)
	}

	for curRef != 0 {
		nv := 0
		verts = verts[:0]
		for i := 0; i < int(poly.VertCount); i++ {
			verts = append(verts, *tile.vertAt(int(poly.Verts[i])))
			nv++
		}

		tmin, tmax, _, segMax, ok := intersectSegmentPoly2D(&startPos, &endPos, verts)
		_ = tmin
		if !ok {
			hit.PathCount = len(hit.Path)
			return status
		}

		hit.HitEdgeIndex = segMax

		if tmax > hit.T {
			hit.T = tmax
		}

		if len(hit.Path) < hit.MaxPath {
			hit.Path = append(hit.Path, curRef)
		} else {
			status |= DT_BUFFER_TOO_SMALL
		}

		if segMax == -1 {
			hit.T = float32(math.MaxFloat32)
			hit.PathCount = len(hit.Path)

			if options&DT_RAYCAST_USE_COSTS != 0 {
				hit.PathCost += filter.GetCost(curPos, endPos, prevRef, prevTile, prevPoly, curRef, tile, poly, curRef, tile, poly)
			}
			return status
		}

		var nextRef PolyRef

		for i := poly.FirstLink; i != DT_NULL_LINK; i = tile.Links[i].Next {
			link := &tile.Links[i]

			if int(link.Edge) != segMax {
				continue
			}

			nextTile, nextPoly = q.nav.GetTileAndPolyByRefUnsafe(link.Ref)

			if nextPoly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
				continue
			}

			if !filter.PassFilter(link.Ref, nextTile, nextPoly) {
				continue
			}

			if link.Side == 0xff {
				nextRef = link.Ref
				break
			}

			if link.Bmin == 0 && link.Bmax == 255 {
				nextRef = link.Ref
				break
			}

			v0 := int(poly.Verts[link.Edge])
			v1 := int(poly.Verts[(int(link.Edge)+1)%int(poly.VertCount)])
			left := tile.vertAt(v0)
			right := tile.vertAt(v1)

			if link.Side == 0 || link.Side == 4 {
				s := float32(1.0) / 255.0
				lmin := left[2] + (right[2]-left[2])*(float32(link.Bmin)*s)
				lmax := left[2] + (right[2]-left[2])*(float32(link.Bmax)*s)
				if lmin > lmax {
					lmin, lmax = lmax, lmin
				}

				z := startPos[2] + (endPos[2]-startPos[2])*tmax
				if z >= lmin && z <= lmax {
					nextRef = link.Ref
					break
				}
			} else if link.Side == 2 || link.Side == 6 {
				s := float32(1.0) / 255.0
				lmin := left[0] + (right[0]-left[0])*(float32(link.Bmin)*s)
				lmax := left[0] + (right[0]-left[0])*(float32(link.Bmax)*s)
				if lmin > lmax {
					lmin, lmax = lmax, lmin
				}

				x := startPos[0] + (endPos[0]-startPos[0])*tmax
				if x >= lmin && x <= lmax {
					nextRef = link.Ref
					break
				}
			}
		}

		if options&DT_RAYCAST_USE_COSTS != 0 {
			lastPos = curPos
			vMad(&curPos, &startPos, &dir, hit.T)
			e1 := &verts[segMax]
			e2 := &verts[(segMax+1)%nv]
			var eDir, diff Vec3
			vSub(&eDir, e2, e1)
			vSub(&diff, &curPos, e1)
			var s float32
			if dtSqrF(eDir[0]) > dtSqrF(eDir[2]) {
				s = diff[0] / eDir[0]
			} else {
				s = diff[2] / eDir[2]
			}
			curPos[1] = e1[1] + eDir[1]*s

			hit.PathCost += filter.GetCost(lastPos, curPos, prevRef, prevTile, prevPoly, curRef, tile, poly, nextRef, nextTile, nextPoly)
		}

		if nextRef == 0 {
			a := segMax
			b := segMax + 1
			if b >= nv {
				b = 0
			}
			va := &verts[a]
			vb := &verts[b]
			dx := vb[0] - va[0]
			dz := vb[2] - va[2]
			hit.HitNormal[0] = dz
			hit.HitNormal[1] = 0
			hit.HitNormal[2] = -dx
			vNormalize(&hit.HitNormal)

			hit.PathCount = len(hit.Path)
			return status
		}

		prevRef = curRef
		curRef = nextRef
		prevTile = tile
		tile = nextTile
		prevPoly = poly
		poly = nextPoly
	}

	hit.PathCount = len(hit.Path)

	return status
}

// RaycastSimple mirrors the backward-compatible dtNavMeshQuery::raycast
// overload that returns t, the hit normal and the visited path
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) RaycastSimple(startRef PolyRef, startPos, endPos Vec3, filter *QueryFilter, maxPath int) (float32, Vec3, []PolyRef, Status) {
	hit := &RaycastHit{MaxPath: maxPath}
	st := q.Raycast(startRef, startPos, endPos, filter, 0, hit, 0)
	return hit.T, hit.HitNormal, hit.Path, st
}

// appendVertex mirrors dtNavMeshQuery::appendVertex
// (DetourNavMeshQuery.cpp). straightPathFlags and straightPathRefs may
// be nil, mirroring the optional C++ out-parameters.
func appendVertex(pos Vec3, flags uint8, ref PolyRef, straightPath *[]Vec3, straightPathFlags *[]uint8, straightPathRefs *[]PolyRef, maxStraightPath int) Status {
	if len(*straightPath) > 0 && vEqual(&(*straightPath)[len(*straightPath)-1], &pos) {
		if straightPathFlags != nil {
			(*straightPathFlags)[len(*straightPathFlags)-1] = flags
		}
		if straightPathRefs != nil {
			(*straightPathRefs)[len(*straightPathRefs)-1] = ref
		}
	} else {
		*straightPath = append(*straightPath, pos)
		if straightPathFlags != nil {
			*straightPathFlags = append(*straightPathFlags, flags)
		}
		if straightPathRefs != nil {
			*straightPathRefs = append(*straightPathRefs, ref)
		}

		if len(*straightPath) >= maxStraightPath {
			return DT_SUCCESS | DT_BUFFER_TOO_SMALL
		}

		if flags == DT_STRAIGHTPATH_END {
			return DT_SUCCESS
		}
	}
	return DT_IN_PROGRESS
}

// appendPortals mirrors dtNavMeshQuery::appendPortals
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) appendPortals(startIdx, endIdx int, endPos Vec3, path []PolyRef, straightPath *[]Vec3, straightPathFlags *[]uint8, straightPathRefs *[]PolyRef, maxStraightPath, options int) Status {
	startPos := (*straightPath)[len(*straightPath)-1]
	for i := startIdx; i < endIdx; i++ {
		from := path[i]
		fromTile, fromPoly, st := q.nav.GetTileAndPolyByRef(from)
		if StatusFailed(st) {
			return DT_FAILURE | DT_INVALID_PARAM
		}

		to := path[i+1]
		toTile, toPoly, st := q.nav.GetTileAndPolyByRef(to)
		if StatusFailed(st) {
			return DT_FAILURE | DT_INVALID_PARAM
		}

		left, right, st := q.getPortalPoints(from, fromPoly, fromTile, to, toPoly, toTile)
		if StatusFailed(st) {
			break
		}

		if options&DT_STRAIGHTPATH_AREA_CROSSINGS != 0 {
			if fromPoly.GetArea() == toPoly.GetArea() {
				continue
			}
		}

		if s, t, ok := intersectSegSeg2D(&startPos, &endPos, &left, &right); ok {
			_ = s
			var pt Vec3
			vLerp(&pt, &left, &right, t)

			stat := appendVertex(pt, 0, path[i+1],
				straightPath, straightPathFlags, straightPathRefs,
				maxStraightPath)
			if stat != DT_IN_PROGRESS {
				return stat
			}
		}
	}
	return DT_IN_PROGRESS
}

// FindStraightPath mirrors dtNavMeshQuery::findStraightPath
// (DetourNavMeshQuery.cpp): string-pulling over the polygon corridor.
// It returns the path points, their flags, and the polygon entered at
// each point.
func (q *NavMeshQuery) FindStraightPath(startPos, endPos Vec3, path []PolyRef, maxStraightPath, options int) ([]Vec3, []uint8, []PolyRef, Status) {
	if !vIsFinite(&startPos) || !vIsFinite(&endPos) ||
		len(path) == 0 || path[0] == 0 || maxStraightPath <= 0 {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}
	pathSize := len(path)

	closestStartPos, st := q.ClosestPointOnPolyBoundary(path[0], startPos)
	if StatusFailed(st) {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	closestEndPos, st := q.ClosestPointOnPolyBoundary(path[pathSize-1], endPos)
	if StatusFailed(st) {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	var straightPath []Vec3
	var straightPathFlags []uint8
	var straightPathRefs []PolyRef

	stat := appendVertex(closestStartPos, DT_STRAIGHTPATH_START, path[0],
		&straightPath, &straightPathFlags, &straightPathRefs,
		maxStraightPath)
	if stat != DT_IN_PROGRESS {
		return straightPath, straightPathFlags, straightPathRefs, stat
	}

	if pathSize > 1 {
		var portalApex, portalLeft, portalRight Vec3
		portalApex = closestStartPos
		portalLeft = portalApex
		portalRight = portalApex
		apexIndex := 0
		leftIndex := 0
		rightIndex := 0

		var leftPolyType, rightPolyType uint8

		leftPolyRef := path[0]
		rightPolyRef := path[0]

		for i := 0; i < pathSize; i++ {
			var left, right Vec3
			var toType uint8

			if i+1 < pathSize {
				// Next portal. fromType is ignored, as in C++.
				var st Status
				left, right, _, toType, st = q.GetPortalPoints(path[i], path[i+1])
				if StatusFailed(st) {
					closestEndPos, st = q.ClosestPointOnPolyBoundary(path[i], endPos)
					if StatusFailed(st) {
						return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
					}

					if options&(DT_STRAIGHTPATH_AREA_CROSSINGS|DT_STRAIGHTPATH_ALL_CROSSINGS) != 0 {
						q.appendPortals(apexIndex, i, closestEndPos, path,
							&straightPath, &straightPathFlags, &straightPathRefs,
							maxStraightPath, options)
					}

					appendVertex(closestEndPos, 0, path[i],
						&straightPath, &straightPathFlags, &straightPathRefs,
						maxStraightPath)

					status := DT_SUCCESS | DT_PARTIAL_RESULT
					if len(straightPath) >= maxStraightPath {
						status |= DT_BUFFER_TOO_SMALL
					}
					return straightPath, straightPathFlags, straightPathRefs, status
				}

				if i == 0 {
					if d, _ := distancePtSegSqr2D(&portalApex, &left, &right); d < dtSqrF(0.001) {
						continue
					}
				}
			} else {
				left = closestEndPos
				right = closestEndPos

				toType = DT_POLYTYPE_GROUND
			}

			if triArea2D(&portalApex, &portalRight, &right) <= 0.0 {
				if vEqual(&portalApex, &portalRight) || triArea2D(&portalApex, &portalLeft, &right) > 0.0 {
					portalRight = right
					if i+1 < pathSize {
						rightPolyRef = path[i+1]
					} else {
						rightPolyRef = 0
					}
					rightPolyType = toType
					rightIndex = i
				} else {
					if options&(DT_STRAIGHTPATH_AREA_CROSSINGS|DT_STRAIGHTPATH_ALL_CROSSINGS) != 0 {
						stat = q.appendPortals(apexIndex, leftIndex, portalLeft, path,
							&straightPath, &straightPathFlags, &straightPathRefs,
							maxStraightPath, options)
						if stat != DT_IN_PROGRESS {
							return straightPath, straightPathFlags, straightPathRefs, stat
						}
					}

					portalApex = portalLeft
					apexIndex = leftIndex

					var flags uint8
					if leftPolyRef == 0 {
						flags = DT_STRAIGHTPATH_END
					} else if leftPolyType == DT_POLYTYPE_OFFMESH_CONNECTION {
						flags = DT_STRAIGHTPATH_OFFMESH_CONNECTION
					}
					ref := leftPolyRef

					stat = appendVertex(portalApex, flags, ref,
						&straightPath, &straightPathFlags, &straightPathRefs,
						maxStraightPath)
					if stat != DT_IN_PROGRESS {
						return straightPath, straightPathFlags, straightPathRefs, stat
					}

					portalLeft = portalApex
					portalRight = portalApex
					leftIndex = apexIndex
					rightIndex = apexIndex

					i = apexIndex
					continue
				}
			}

			if triArea2D(&portalApex, &portalLeft, &left) >= 0.0 {
				if vEqual(&portalApex, &portalLeft) || triArea2D(&portalApex, &portalRight, &left) < 0.0 {
					portalLeft = left
					if i+1 < pathSize {
						leftPolyRef = path[i+1]
					} else {
						leftPolyRef = 0
					}
					leftPolyType = toType
					leftIndex = i
				} else {
					if options&(DT_STRAIGHTPATH_AREA_CROSSINGS|DT_STRAIGHTPATH_ALL_CROSSINGS) != 0 {
						stat = q.appendPortals(apexIndex, rightIndex, portalRight, path,
							&straightPath, &straightPathFlags, &straightPathRefs,
							maxStraightPath, options)
						if stat != DT_IN_PROGRESS {
							return straightPath, straightPathFlags, straightPathRefs, stat
						}
					}

					portalApex = portalRight
					apexIndex = rightIndex

					var flags uint8
					if rightPolyRef == 0 {
						flags = DT_STRAIGHTPATH_END
					} else if rightPolyType == DT_POLYTYPE_OFFMESH_CONNECTION {
						flags = DT_STRAIGHTPATH_OFFMESH_CONNECTION
					}
					ref := rightPolyRef

					stat = appendVertex(portalApex, flags, ref,
						&straightPath, &straightPathFlags, &straightPathRefs,
						maxStraightPath)
					if stat != DT_IN_PROGRESS {
						return straightPath, straightPathFlags, straightPathRefs, stat
					}

					portalLeft = portalApex
					portalRight = portalApex
					leftIndex = apexIndex
					rightIndex = apexIndex

					i = apexIndex
					continue
				}
			}
		}

		if options&(DT_STRAIGHTPATH_AREA_CROSSINGS|DT_STRAIGHTPATH_ALL_CROSSINGS) != 0 {
			stat = q.appendPortals(apexIndex, pathSize-1, closestEndPos, path,
				&straightPath, &straightPathFlags, &straightPathRefs,
				maxStraightPath, options)
			if stat != DT_IN_PROGRESS {
				return straightPath, straightPathFlags, straightPathRefs, stat
			}
		}
	}

	appendVertex(closestEndPos, DT_STRAIGHTPATH_END, 0,
		&straightPath, &straightPathFlags, &straightPathRefs,
		maxStraightPath)

	status := DT_SUCCESS
	if len(straightPath) >= maxStraightPath {
		status |= DT_BUFFER_TOO_SMALL
	}
	return straightPath, straightPathFlags, straightPathRefs, status
}

// ClosestPointOnPoly mirrors dtNavMeshQuery::closestPointOnPoly
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) ClosestPointOnPoly(ref PolyRef, pos Vec3) (Vec3, bool, Status) {
	if !q.nav.IsValidPolyRef(ref) || !vIsFinite(&pos) {
		return Vec3{}, false, DT_FAILURE | DT_INVALID_PARAM
	}

	closest, posOverPoly := q.nav.closestPointOnPoly(ref, &pos)
	return closest, posOverPoly, DT_SUCCESS
}

// ClosestPointOnPolyBoundary mirrors
// dtNavMeshQuery::closestPointOnPolyBoundary (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) ClosestPointOnPolyBoundary(ref PolyRef, pos Vec3) (Vec3, Status) {
	tile, poly, st := q.nav.GetTileAndPolyByRef(ref)
	if StatusFailed(st) {
		return Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	if !vIsFinite(&pos) {
		return Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	verts := make([]Vec3, 0, DT_VERTS_PER_POLYGON)
	for i := 0; i < int(poly.VertCount); i++ {
		verts = append(verts, *tile.vertAt(int(poly.Verts[i])))
	}
	nv := len(verts)

	edged := make([]float32, nv)
	edget := make([]float32, nv)
	inside := distancePtPolyEdgesSqr(&pos, verts, edged, edget)
	if inside {
		return pos, DT_SUCCESS
	}

	dmin := edged[0]
	imin := 0
	for i := 1; i < nv; i++ {
		if edged[i] < dmin {
			dmin = edged[i]
			imin = i
		}
	}
	va := &verts[imin]
	vb := &verts[(imin+1)%nv]
	var closest Vec3
	vLerp(&closest, va, vb, edget[imin])
	return closest, DT_SUCCESS
}

// GetPolyHeight mirrors dtNavMeshQuery::getPolyHeight
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) GetPolyHeight(ref PolyRef, pos Vec3) (float32, Status) {
	tile, poly, st := q.nav.GetTileAndPolyByRef(ref)
	if StatusFailed(st) {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}

	if !vIsFinite2D(&pos) {
		return 0, DT_FAILURE | DT_INVALID_PARAM
	}

	if poly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
		v0 := tile.vertAt(int(poly.Verts[0]))
		v1 := tile.vertAt(int(poly.Verts[1]))
		_, t := distancePtSegSqr2D(&pos, v0, v1)
		return v0[1] + (v1[1]-v0[1])*t, DT_SUCCESS
	}

	if h, ok := q.nav.getPolyHeight(tile, polyIndex(tile, poly), &pos); ok {
		return h, DT_SUCCESS
	}
	return 0, DT_FAILURE | DT_INVALID_PARAM
}

// MoveAlongSurface mirrors dtNavMeshQuery::moveAlongSurface
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) MoveAlongSurface(startRef PolyRef, startPos, endPos Vec3, filter *QueryFilter, maxVisitedSize int) (Vec3, []PolyRef, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&startPos) || !vIsFinite(&endPos) ||
		filter == nil || maxVisitedSize <= 0 {
		return Vec3{}, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	status := DT_SUCCESS

	const maxStack = 48
	stack := make([]*Node, 0, maxStack)

	q.tinyNodePool.Clear()

	startNode := q.tinyNodePool.GetNode(startRef, 0)
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_CLOSED
	stack = append(stack, startNode)

	bestPos := startPos
	bestDist := float32(math.MaxFloat32)
	var bestNode *Node

	var searchPos Vec3
	vLerp(&searchPos, &startPos, &endPos, 0.5)
	searchRadSqr := dtSqrF(vDist(&startPos, &endPos)/2.0 + 0.001)

	verts := make([]Vec3, DT_VERTS_PER_POLYGON)

	for len(stack) > 0 {
		curNode := stack[0]
		stack = stack[1:]

		curRef := curNode.ID
		curTile, curPoly := q.nav.GetTileAndPolyByRefUnsafe(curRef)

		nverts := int(curPoly.VertCount)
		for i := 0; i < nverts; i++ {
			verts[i] = *curTile.vertAt(int(curPoly.Verts[i]))
		}

		if pointInPolygon(&endPos, verts[:nverts]) {
			bestNode = curNode
			bestPos = endPos
			break
		}

		for i, j := 0, int(curPoly.VertCount)-1; i < int(curPoly.VertCount); j, i = i, i+1 {
			const maxNeis = 8
			var neis [maxNeis]PolyRef
			nneis := 0

			if curPoly.Neis[j]&DT_EXT_LINK != 0 {
				for k := curPoly.FirstLink; k != DT_NULL_LINK; k = curTile.Links[k].Next {
					link := &curTile.Links[k]
					if link.Edge == uint8(j) {
						if link.Ref != 0 {
							neiTile, neiPoly := q.nav.GetTileAndPolyByRefUnsafe(link.Ref)
							if filter.PassFilter(link.Ref, neiTile, neiPoly) {
								if nneis < maxNeis {
									neis[nneis] = link.Ref
									nneis++
								}
							}
						}
					}
				}
			} else if curPoly.Neis[j] != 0 {
				idx := uint32(curPoly.Neis[j] - 1)
				ref := q.nav.GetPolyRefBase(curTile) | PolyRef(idx)
				if filter.PassFilter(ref, curTile, &curTile.Polys[idx]) {
					neis[nneis] = ref
					nneis++
				}
			}

			if nneis == 0 {
				vj := &verts[j]
				vi := &verts[i]
				distSqr, tseg := distancePtSegSqr2D(&endPos, vj, vi)
				if distSqr < bestDist {
					vLerp(&bestPos, vj, vi, tseg)
					bestDist = distSqr
					bestNode = curNode
				}
			} else {
				for k := 0; k < nneis; k++ {
					neighbourNode := q.tinyNodePool.GetNode(neis[k], 0)
					if neighbourNode == nil {
						continue
					}
					if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
						continue
					}

					vj := &verts[j]
					vi := &verts[i]
					distSqr, _ := distancePtSegSqr2D(&searchPos, vj, vi)
					if distSqr > searchRadSqr {
						continue
					}

					if len(stack) < maxStack {
						neighbourNode.Pidx = q.tinyNodePool.GetNodeIdx(curNode)
						neighbourNode.Flags |= DT_NODE_CLOSED
						stack = append(stack, neighbourNode)
					}
				}
			}
		}
	}

	var visited []PolyRef
	if bestNode != nil {
		var prev *Node
		node := bestNode
		for node != nil {
			next := q.tinyNodePool.GetNodeAtIdx(node.Pidx)
			node.Pidx = q.tinyNodePool.GetNodeIdx(prev)
			prev = node
			node = next
		}

		node = prev
		for node != nil {
			visited = append(visited, node.ID)
			if len(visited) >= maxVisitedSize {
				status |= DT_BUFFER_TOO_SMALL
				break
			}
			node = q.tinyNodePool.GetNodeAtIdx(node.Pidx)
		}
	}

	return bestPos, visited, status
}

// FindPolysAroundCircle mirrors dtNavMeshQuery::findPolysAroundCircle
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) FindPolysAroundCircle(startRef PolyRef, centerPos Vec3, radius float32, filter *QueryFilter, maxResult int) ([]PolyRef, []PolyRef, []float32, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&centerPos) ||
		radius < 0 || !isFinite(radius) ||
		filter == nil || maxResult < 0 {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	q.nodePool.Clear()
	q.openList.Clear()

	startNode := q.nodePool.GetNode(startRef, 0)
	startNode.Pos = centerPos
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_OPEN
	q.openList.Push(startNode)

	status := DT_SUCCESS

	var resultRef, resultParent []PolyRef
	var resultCost []float32

	radiusSqr := dtSqrF(radius)

	for !q.openList.Empty() {
		bestNode := q.openList.Pop()
		bestNode.Flags &^= DT_NODE_OPEN
		bestNode.Flags |= DT_NODE_CLOSED

		bestRef := bestNode.ID
		bestTile, bestPoly := q.nav.GetTileAndPolyByRefUnsafe(bestRef)

		var parentRef PolyRef
		var parentTile *MeshTile
		var parentPoly *Poly
		if bestNode.Pidx != 0 {
			parentRef = q.nodePool.GetNodeAtIdx(bestNode.Pidx).ID
		}
		if parentRef != 0 {
			parentTile, parentPoly = q.nav.GetTileAndPolyByRefUnsafe(parentRef)
		}

		if len(resultRef) < maxResult {
			resultRef = append(resultRef, bestRef)
			resultParent = append(resultParent, parentRef)
			resultCost = append(resultCost, bestNode.Total)
		} else {
			status |= DT_BUFFER_TOO_SMALL
		}

		for i := bestPoly.FirstLink; i != DT_NULL_LINK; i = bestTile.Links[i].Next {
			link := &bestTile.Links[i]
			neighbourRef := link.Ref
			if neighbourRef == 0 || neighbourRef == parentRef {
				continue
			}

			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			va, vb, st := q.getPortalPoints(bestRef, bestPoly, bestTile, neighbourRef, neighbourPoly, neighbourTile)
			if StatusFailed(st) {
				continue
			}

			distSqr, _ := distancePtSegSqr2D(&centerPos, &va, &vb)
			if distSqr > radiusSqr {
				continue
			}

			neighbourNode := q.nodePool.GetNode(neighbourRef, 0)
			if neighbourNode == nil {
				status |= DT_OUT_OF_NODES
				continue
			}

			if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
				continue
			}

			if neighbourNode.Flags == 0 {
				vLerp(&neighbourNode.Pos, &va, &vb, 0.5)
			}

			cost := filter.GetCost(bestNode.Pos, neighbourNode.Pos,
				parentRef, parentTile, parentPoly,
				bestRef, bestTile, bestPoly,
				neighbourRef, neighbourTile, neighbourPoly)

			total := bestNode.Total + cost

			if (neighbourNode.Flags&DT_NODE_OPEN) != 0 && total >= neighbourNode.Total {
				continue
			}

			neighbourNode.ID = neighbourRef
			neighbourNode.Pidx = q.nodePool.GetNodeIdx(bestNode)
			neighbourNode.Total = total

			if (neighbourNode.Flags & DT_NODE_OPEN) != 0 {
				q.openList.Modify(neighbourNode)
			} else {
				neighbourNode.Flags = DT_NODE_OPEN
				q.openList.Push(neighbourNode)
			}
		}
	}

	return resultRef, resultParent, resultCost, status
}

// polyArea mirrors the triangle-fan area loop shared by findRandomPoint
// and findRandomPointAroundCircle (DetourNavMeshQuery.cpp): the sum of
// dtTriArea2D over (verts[0], verts[j-1], verts[j]) for j >= 2.
func polyArea(tile *MeshTile, poly *Poly) float32 {
	var area float32
	for j := 2; j < int(poly.VertCount); j++ {
		va := tile.vertAt(int(poly.Verts[0]))
		vb := tile.vertAt(int(poly.Verts[j-1]))
		vc := tile.vertAt(int(poly.Verts[j]))
		area += triArea2D(va, vb, vc)
	}
	return area
}

// randomPointOnPoly mirrors the tail shared by findRandomPoint and
// findRandomPointAroundCircle (DetourNavMeshQuery.cpp): pick a uniform
// point inside the (convex) polygon with two frand() draws. The caller
// sets the point's height from getPolyHeight.
func (q *NavMeshQuery) randomPointOnPoly(tile *MeshTile, poly *Poly, frand func() float32) Vec3 {
	verts := make([]Vec3, poly.VertCount)
	for j := 0; j < int(poly.VertCount); j++ {
		verts[j] = *tile.vertAt(int(poly.Verts[j]))
	}
	areas := make([]float32, poly.VertCount)
	return randomPointInConvexPoly(verts, areas, frand(), frand())
}

// FindRandomPoint mirrors dtNavMeshQuery::findRandomPoint
// (DetourNavMeshQuery.cpp). It picks a tile by reservoir sampling (all
// tiles weigh 1.0), then a ground polygon weighted by area, then a uniform
// point on that polygon. frand supplies uniform [0,1) draws, as the C++
// float (*frand)() callback does.
func (q *NavMeshQuery) FindRandomPoint(filter *QueryFilter, frand func() float32) (PolyRef, Vec3, Status) {
	if filter == nil || frand == nil {
		return 0, Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	// Randomly pick one tile. Assume that all tiles cover roughly the
	// same area.
	var tile *MeshTile
	tsum := float32(0)
	for i := 0; i < q.nav.GetMaxTiles(); i++ {
		t := q.nav.GetTile(i)
		if t == nil || t.Header == nil {
			continue
		}
		// Choose random tile using reservoir sampling.
		const area = 1.0
		tsum += area
		u := frand()
		if u*tsum <= area {
			tile = t
		}
	}
	if tile == nil {
		return 0, Vec3{}, DT_FAILURE
	}

	// Randomly pick one polygon weighted by polygon area.
	var poly *Poly
	var polyRef PolyRef
	base := q.nav.GetPolyRefBase(tile)

	areaSum := float32(0)
	for i := 0; i < int(tile.Header.PolyCount); i++ {
		p := &tile.Polys[i]
		// Do not return off-mesh connection polygons.
		if p.GetType() != DT_POLYTYPE_GROUND {
			continue
		}
		// Must pass filter
		ref := base | PolyRef(i)
		if !filter.PassFilter(ref, tile, p) {
			continue
		}
		polyArea := polyArea(tile, p)
		// Choose random polygon weighted by area, using reservoir sampling.
		areaSum += polyArea
		u := frand()
		if u*areaSum <= polyArea {
			poly = p
			polyRef = ref
		}
	}
	if poly == nil {
		return 0, Vec3{}, DT_FAILURE
	}

	// Randomly pick point on polygon.
	pt := q.randomPointOnPoly(tile, poly, frand)
	h, st := q.GetPolyHeight(polyRef, pt)
	if StatusFailed(st) {
		return 0, Vec3{}, st
	}
	pt[1] = h
	return polyRef, pt, DT_SUCCESS
}

// FindRandomPointAroundCircle mirrors dtNavMeshQuery::
// findRandomPointAroundCircle (DetourNavMeshQuery.cpp). It Dijkstra-walks
// the polys within maxRadius of centerPos from startRef, reservoir-picks
// one ground polygon weighted by area, then returns a uniform point on
// it. frand supplies uniform [0,1) draws.
func (q *NavMeshQuery) FindRandomPointAroundCircle(startRef PolyRef, centerPos Vec3, maxRadius float32, filter *QueryFilter, frand func() float32) (PolyRef, Vec3, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&centerPos) ||
		maxRadius < 0 || !isFinite(maxRadius) ||
		filter == nil || frand == nil {
		return 0, Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	startTile, startPoly := q.nav.GetTileAndPolyByRefUnsafe(startRef)
	if !filter.PassFilter(startRef, startTile, startPoly) {
		return 0, Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	q.nodePool.Clear()
	q.openList.Clear()

	startNode := q.nodePool.GetNode(startRef, 0)
	startNode.Pos = centerPos
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_OPEN
	q.openList.Push(startNode)

	status := DT_SUCCESS

	radiusSqr := dtSqrF(maxRadius)
	areaSum := float32(0)

	var randomTile *MeshTile
	var randomPoly *Poly
	var randomPolyRef PolyRef

	for !q.openList.Empty() {
		bestNode := q.openList.Pop()
		bestNode.Flags &^= DT_NODE_OPEN
		bestNode.Flags |= DT_NODE_CLOSED

		// Get poly and tile.
		// The API input has been checked already, skip checking internal data.
		bestRef := bestNode.ID
		bestTile, bestPoly := q.nav.GetTileAndPolyByRefUnsafe(bestRef)

		// Place random locations on ground.
		if bestPoly.GetType() == DT_POLYTYPE_GROUND {
			// Calc area of the polygon.
			polyArea := polyArea(bestTile, bestPoly)
			// Choose random polygon weighted by area, using reservoir sampling.
			areaSum += polyArea
			u := frand()
			if u*areaSum <= polyArea {
				randomTile = bestTile
				randomPoly = bestPoly
				randomPolyRef = bestRef
			}
		}

		// Get parent poly and tile.
		var parentRef PolyRef
		if bestNode.Pidx != 0 {
			parentRef = q.nodePool.GetNodeAtIdx(bestNode.Pidx).ID
		}

		for i := bestPoly.FirstLink; i != DT_NULL_LINK; i = bestTile.Links[i].Next {
			link := &bestTile.Links[i]
			neighbourRef := link.Ref
			// Skip invalid neighbours and do not follow back to parent.
			if neighbourRef == 0 || neighbourRef == parentRef {
				continue
			}

			// Expand to neighbour
			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			// Do not advance if the polygon is excluded by the filter.
			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			// Find edge and calc distance to the edge.
			va, vb, st := q.getPortalPoints(bestRef, bestPoly, bestTile, neighbourRef, neighbourPoly, neighbourTile)
			if StatusFailed(st) {
				continue
			}

			// If the circle is not touching the next polygon, skip it.
			distSqr, _ := distancePtSegSqr2D(&centerPos, &va, &vb)
			if distSqr > radiusSqr {
				continue
			}

			neighbourNode := q.nodePool.GetNode(neighbourRef, 0)
			if neighbourNode == nil {
				status |= DT_OUT_OF_NODES
				continue
			}

			if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
				continue
			}

			// Cost
			if neighbourNode.Flags == 0 {
				vLerp(&neighbourNode.Pos, &va, &vb, 0.5)
			}

			total := bestNode.Total + vDist(&bestNode.Pos, &neighbourNode.Pos)

			// The node is already in open list and the new result is worse, skip.
			if (neighbourNode.Flags&DT_NODE_OPEN) != 0 && total >= neighbourNode.Total {
				continue
			}

			neighbourNode.ID = neighbourRef
			neighbourNode.Flags &^= DT_NODE_CLOSED
			neighbourNode.Pidx = q.nodePool.GetNodeIdx(bestNode)
			neighbourNode.Total = total

			if (neighbourNode.Flags & DT_NODE_OPEN) != 0 {
				q.openList.Modify(neighbourNode)
			} else {
				neighbourNode.Flags = DT_NODE_OPEN
				q.openList.Push(neighbourNode)
			}
		}
	}

	if randomPoly == nil {
		return 0, Vec3{}, DT_FAILURE
	}

	// Randomly pick point on polygon.
	pt := q.randomPointOnPoly(randomTile, randomPoly, frand)
	h, stat := q.GetPolyHeight(randomPolyRef, pt)
	// C++ quirk (DetourNavMeshQuery.cpp, findRandomPointAroundCircle):
	// the tail tests the loop's `status`, not getPolyHeight's `stat`,
	// but returns `stat` — so a failed getPolyHeight is silently
	// ignored when the search itself succeeded.
	if StatusFailed(status) {
		return 0, Vec3{}, stat
	}
	pt[1] = h
	return randomPolyRef, pt, DT_SUCCESS
}

// FindPolysAroundShape mirrors dtNavMeshQuery::findPolysAroundShape
// (DetourNavMeshQuery.cpp). verts is the convex search polygon (CCW),
// given as vertices.
func (q *NavMeshQuery) FindPolysAroundShape(startRef PolyRef, verts []Vec3, filter *QueryFilter, maxResult int) ([]PolyRef, []PolyRef, []float32, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		len(verts) < 3 ||
		filter == nil || maxResult < 0 {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	if startRef == 0 || !q.nav.IsValidPolyRef(startRef) {
		return nil, nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	q.nodePool.Clear()
	q.openList.Clear()

	var centerPos Vec3
	for i := range verts {
		vAdd(&centerPos, &centerPos, &verts[i])
	}
	vScale(&centerPos, &centerPos, 1.0/float32(len(verts)))

	startNode := q.nodePool.GetNode(startRef, 0)
	startNode.Pos = centerPos
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_OPEN
	q.openList.Push(startNode)

	status := DT_SUCCESS

	var resultRef, resultParent []PolyRef
	var resultCost []float32

	for !q.openList.Empty() {
		bestNode := q.openList.Pop()
		bestNode.Flags &^= DT_NODE_OPEN
		bestNode.Flags |= DT_NODE_CLOSED

		bestRef := bestNode.ID
		bestTile, bestPoly := q.nav.GetTileAndPolyByRefUnsafe(bestRef)

		var parentRef PolyRef
		var parentTile *MeshTile
		var parentPoly *Poly
		if bestNode.Pidx != 0 {
			parentRef = q.nodePool.GetNodeAtIdx(bestNode.Pidx).ID
		}
		if parentRef != 0 {
			parentTile, parentPoly = q.nav.GetTileAndPolyByRefUnsafe(parentRef)
		}

		if len(resultRef) < maxResult {
			resultRef = append(resultRef, bestRef)
			resultParent = append(resultParent, parentRef)
			resultCost = append(resultCost, bestNode.Total)
		} else {
			status |= DT_BUFFER_TOO_SMALL
		}

		for i := bestPoly.FirstLink; i != DT_NULL_LINK; i = bestTile.Links[i].Next {
			link := &bestTile.Links[i]
			neighbourRef := link.Ref
			if neighbourRef == 0 || neighbourRef == parentRef {
				continue
			}

			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			va, vb, st := q.getPortalPoints(bestRef, bestPoly, bestTile, neighbourRef, neighbourPoly, neighbourTile)
			if StatusFailed(st) {
				continue
			}

			tmin, tmax, _, _, ok := intersectSegmentPoly2D(&va, &vb, verts)
			if !ok {
				continue
			}
			if tmin > 1.0 || tmax < 0.0 {
				continue
			}

			neighbourNode := q.nodePool.GetNode(neighbourRef, 0)
			if neighbourNode == nil {
				status |= DT_OUT_OF_NODES
				continue
			}

			if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
				continue
			}

			if neighbourNode.Flags == 0 {
				vLerp(&neighbourNode.Pos, &va, &vb, 0.5)
			}

			cost := filter.GetCost(bestNode.Pos, neighbourNode.Pos,
				parentRef, parentTile, parentPoly,
				bestRef, bestTile, bestPoly,
				neighbourRef, neighbourTile, neighbourPoly)

			total := bestNode.Total + cost

			if (neighbourNode.Flags&DT_NODE_OPEN) != 0 && total >= neighbourNode.Total {
				continue
			}

			neighbourNode.ID = neighbourRef
			neighbourNode.Pidx = q.nodePool.GetNodeIdx(bestNode)
			neighbourNode.Total = total

			if (neighbourNode.Flags & DT_NODE_OPEN) != 0 {
				q.openList.Modify(neighbourNode)
			} else {
				neighbourNode.Flags = DT_NODE_OPEN
				q.openList.Push(neighbourNode)
			}
		}
	}

	return resultRef, resultParent, resultCost, status
}

// GetPathFromDijkstraSearch mirrors
// dtNavMeshQuery::getPathFromDijkstraSearch (DetourNavMeshQuery.cpp).
// It must be called immediately after FindPolysAroundCircle or
// FindPolysAroundShape.
func (q *NavMeshQuery) GetPathFromDijkstraSearch(endRef PolyRef, maxPath int) ([]PolyRef, Status) {
	if !q.nav.IsValidPolyRef(endRef) || maxPath < 0 {
		return nil, DT_FAILURE | DT_INVALID_PARAM
	}

	nodes := q.nodePool.FindNodes(endRef, 1)
	if len(nodes) != 1 || (nodes[0].Flags&DT_NODE_CLOSED) == 0 {
		return nil, DT_FAILURE | DT_INVALID_PARAM
	}

	return q.getPathToNode(nodes[0], maxPath)
}

// FindLocalNeighbourhood mirrors dtNavMeshQuery::findLocalNeighbourhood
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) FindLocalNeighbourhood(startRef PolyRef, centerPos Vec3, radius float32, filter *QueryFilter, maxResult int) ([]PolyRef, []PolyRef, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&centerPos) ||
		radius < 0 || !isFinite(radius) ||
		filter == nil || maxResult < 0 {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	const maxStack = 48
	stack := make([]*Node, 0, maxStack)

	q.tinyNodePool.Clear()

	startNode := q.tinyNodePool.GetNode(startRef, 0)
	startNode.Pidx = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_CLOSED
	stack = append(stack, startNode)

	radiusSqr := dtSqrF(radius)

	pa := make([]Vec3, DT_VERTS_PER_POLYGON)
	pb := make([]Vec3, DT_VERTS_PER_POLYGON)

	status := DT_SUCCESS

	var resultRef, resultParent []PolyRef
	if len(resultRef) < maxResult {
		resultRef = append(resultRef, startNode.ID)
		resultParent = append(resultParent, 0)
	} else {
		status |= DT_BUFFER_TOO_SMALL
	}

	for len(stack) > 0 {
		curNode := stack[0]
		stack = stack[1:]

		curRef := curNode.ID
		curTile, curPoly := q.nav.GetTileAndPolyByRefUnsafe(curRef)

		for i := curPoly.FirstLink; i != DT_NULL_LINK; i = curTile.Links[i].Next {
			link := &curTile.Links[i]
			neighbourRef := link.Ref
			if neighbourRef == 0 {
				continue
			}

			neighbourNode := q.tinyNodePool.GetNode(neighbourRef, 0)
			if neighbourNode == nil {
				continue
			}
			if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
				continue
			}

			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			if neighbourPoly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
				continue
			}

			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			va, vb, st := q.getPortalPoints(curRef, curPoly, curTile, neighbourRef, neighbourPoly, neighbourTile)
			if StatusFailed(st) {
				continue
			}

			distSqr, _ := distancePtSegSqr2D(&centerPos, &va, &vb)
			if distSqr > radiusSqr {
				continue
			}

			// Mark visited before the overlap test so the poly is
			// not visited again if the test fails.
			neighbourNode.Flags |= DT_NODE_CLOSED
			neighbourNode.Pidx = q.tinyNodePool.GetNodeIdx(curNode)

			npa := int(neighbourPoly.VertCount)
			for k := 0; k < npa; k++ {
				pa[k] = *neighbourTile.vertAt(int(neighbourPoly.Verts[k]))
			}

			overlap := false
			for j := range resultRef {
				pastRef := resultRef[j]

				connected := false
				for k := curPoly.FirstLink; k != DT_NULL_LINK; k = curTile.Links[k].Next {
					if curTile.Links[k].Ref == pastRef {
						connected = true
						break
					}
				}
				if connected {
					continue
				}

				pastTile, pastPoly := q.nav.GetTileAndPolyByRefUnsafe(pastRef)

				npb := int(pastPoly.VertCount)
				for k := 0; k < npb; k++ {
					pb[k] = *pastTile.vertAt(int(pastPoly.Verts[k]))
				}

				if overlapPolyPoly2D(pa[:npa], pb[:npb]) {
					overlap = true
					break
				}
			}
			if overlap {
				continue
			}

			if len(resultRef) < maxResult {
				resultRef = append(resultRef, neighbourRef)
				resultParent = append(resultParent, curRef)
			} else {
				status |= DT_BUFFER_TOO_SMALL
			}

			if len(stack) < maxStack {
				stack = append(stack, neighbourNode)
			}
		}
	}

	return resultRef, resultParent, status
}

// segInterval mirrors dtSegInterval (DetourNavMeshQuery.cpp).
type segInterval struct {
	ref  PolyRef
	tmin int16
	tmax int16
}

// insertInterval mirrors insertInterval (DetourNavMeshQuery.cpp).
func insertInterval(ints []segInterval, tmin, tmax int16, ref PolyRef) []segInterval {
	const maxInts = 16
	if len(ints)+1 > maxInts {
		return ints
	}
	idx := 0
	for idx < len(ints) {
		if tmax <= ints[idx].tmin {
			break
		}
		idx++
	}
	ints = append(ints, segInterval{})
	copy(ints[idx+1:], ints[idx:])
	ints[idx] = segInterval{ref: ref, tmin: tmin, tmax: tmax}
	return ints
}

// GetPolyWallSegments mirrors dtNavMeshQuery::getPolyWallSegments
// (DetourNavMeshQuery.cpp). Each segment is a pair of vertices; refs
// holds the neighbour polygon per segment (0 for walls) and is nil
// unless storePortals is set.
func (q *NavMeshQuery) GetPolyWallSegments(ref PolyRef, filter *QueryFilter, maxSegments int, storePortals bool) ([][2]Vec3, []PolyRef, Status) {
	tile, poly, st := q.nav.GetTileAndPolyByRef(ref)
	if StatusFailed(st) {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	if filter == nil || maxSegments < 0 {
		return nil, nil, DT_FAILURE | DT_INVALID_PARAM
	}

	var segments [][2]Vec3
	var segmentRefs []PolyRef
	status := DT_SUCCESS

	store := func(a, b Vec3, ref PolyRef) {
		if len(segments) < maxSegments {
			segments = append(segments, [2]Vec3{a, b})
			if storePortals {
				segmentRefs = append(segmentRefs, ref)
			}
		} else {
			status |= DT_BUFFER_TOO_SMALL
		}
	}

	for i, j := 0, int(poly.VertCount)-1; i < int(poly.VertCount); j, i = i, i+1 {
		var ints []segInterval
		if poly.Neis[j]&DT_EXT_LINK != 0 {
			for k := poly.FirstLink; k != DT_NULL_LINK; k = tile.Links[k].Next {
				link := &tile.Links[k]
				if link.Edge == uint8(j) {
					if link.Ref != 0 {
						neiTile, neiPoly := q.nav.GetTileAndPolyByRefUnsafe(link.Ref)
						if filter.PassFilter(link.Ref, neiTile, neiPoly) {
							ints = insertInterval(ints, int16(link.Bmin), int16(link.Bmax), link.Ref)
						}
					}
				}
			}
		} else {
			var neiRef PolyRef
			if poly.Neis[j] != 0 {
				idx := uint32(poly.Neis[j] - 1)
				neiRef = q.nav.GetPolyRefBase(tile) | PolyRef(idx)
				if !filter.PassFilter(neiRef, tile, &tile.Polys[idx]) {
					neiRef = 0
				}
			}

			if neiRef != 0 && !storePortals {
				continue
			}

			vj := *tile.vertAt(int(poly.Verts[j]))
			vi := *tile.vertAt(int(poly.Verts[i]))
			store(vj, vi, neiRef)

			continue
		}

		ints = insertInterval(ints, -1, 0, 0)
		ints = insertInterval(ints, 255, 256, 0)

		vj := *tile.vertAt(int(poly.Verts[j]))
		vi := *tile.vertAt(int(poly.Verts[i]))
		for k := 1; k < len(ints); k++ {
			if storePortals && ints[k].ref != 0 {
				tmin := float32(ints[k].tmin) / 255.0
				tmax := float32(ints[k].tmax) / 255.0
				var a, b Vec3
				vLerp(&a, &vj, &vi, tmin)
				vLerp(&b, &vj, &vi, tmax)
				store(a, b, ints[k].ref)
			}

			imin := int(ints[k-1].tmax)
			imax := int(ints[k].tmin)
			if imin != imax {
				tmin := float32(imin) / 255.0
				tmax := float32(imax) / 255.0
				var a, b Vec3
				vLerp(&a, &vj, &vi, tmin)
				vLerp(&b, &vj, &vi, tmax)
				store(a, b, 0)
			}
		}
	}

	return segments, segmentRefs, status
}

// FindDistanceToWall mirrors dtNavMeshQuery::findDistanceToWall
// (DetourNavMeshQuery.cpp). hitPos and hitNormal are only meaningful
// when hitDist is less than maxRadius.
func (q *NavMeshQuery) FindDistanceToWall(startRef PolyRef, centerPos Vec3, maxRadius float32, filter *QueryFilter) (float32, Vec3, Vec3, Status) {
	if !q.nav.IsValidPolyRef(startRef) ||
		!vIsFinite(&centerPos) ||
		maxRadius < 0 || !isFinite(maxRadius) ||
		filter == nil {
		return 0, Vec3{}, Vec3{}, DT_FAILURE | DT_INVALID_PARAM
	}

	q.nodePool.Clear()
	q.openList.Clear()

	startNode := q.nodePool.GetNode(startRef, 0)
	startNode.Pos = centerPos
	startNode.Pidx = 0
	startNode.Cost = 0
	startNode.Total = 0
	startNode.ID = startRef
	startNode.Flags = DT_NODE_OPEN
	q.openList.Push(startNode)

	radiusSqr := dtSqrF(maxRadius)

	var hitPos Vec3

	status := DT_SUCCESS

	for !q.openList.Empty() {
		bestNode := q.openList.Pop()
		bestNode.Flags &^= DT_NODE_OPEN
		bestNode.Flags |= DT_NODE_CLOSED

		bestRef := bestNode.ID
		bestTile, bestPoly := q.nav.GetTileAndPolyByRefUnsafe(bestRef)

		var parentRef PolyRef
		if bestNode.Pidx != 0 {
			parentRef = q.nodePool.GetNodeAtIdx(bestNode.Pidx).ID
		}

		for i, j := 0, int(bestPoly.VertCount)-1; i < int(bestPoly.VertCount); j, i = i, i+1 {
			if bestPoly.Neis[j]&DT_EXT_LINK != 0 {
				solid := true
				for k := bestPoly.FirstLink; k != DT_NULL_LINK; k = bestTile.Links[k].Next {
					link := &bestTile.Links[k]
					if link.Edge == uint8(j) {
						if link.Ref != 0 {
							neiTile, neiPoly := q.nav.GetTileAndPolyByRefUnsafe(link.Ref)
							if filter.PassFilter(link.Ref, neiTile, neiPoly) {
								solid = false
							}
						}
						break
					}
				}
				if !solid {
					continue
				}
			} else if bestPoly.Neis[j] != 0 {
				idx := uint32(bestPoly.Neis[j] - 1)
				ref := q.nav.GetPolyRefBase(bestTile) | PolyRef(idx)
				if filter.PassFilter(ref, bestTile, &bestTile.Polys[idx]) {
					continue
				}
			}

			vj := bestTile.vertAt(int(bestPoly.Verts[j]))
			vi := bestTile.vertAt(int(bestPoly.Verts[i]))
			distSqr, tseg := distancePtSegSqr2D(&centerPos, vj, vi)

			if distSqr > radiusSqr {
				continue
			}

			radiusSqr = distSqr
			hitPos[0] = vj[0] + (vi[0]-vj[0])*tseg
			hitPos[1] = vj[1] + (vi[1]-vj[1])*tseg
			hitPos[2] = vj[2] + (vi[2]-vj[2])*tseg
		}

		for i := bestPoly.FirstLink; i != DT_NULL_LINK; i = bestTile.Links[i].Next {
			link := &bestTile.Links[i]
			neighbourRef := link.Ref
			if neighbourRef == 0 || neighbourRef == parentRef {
				continue
			}

			neighbourTile, neighbourPoly := q.nav.GetTileAndPolyByRefUnsafe(neighbourRef)

			if neighbourPoly.GetType() == DT_POLYTYPE_OFFMESH_CONNECTION {
				continue
			}

			va := bestTile.vertAt(int(bestPoly.Verts[link.Edge]))
			vb := bestTile.vertAt(int(bestPoly.Verts[(int(link.Edge)+1)%int(bestPoly.VertCount)]))
			distSqr, _ := distancePtSegSqr2D(&centerPos, va, vb)

			if distSqr > radiusSqr {
				continue
			}

			if !filter.PassFilter(neighbourRef, neighbourTile, neighbourPoly) {
				continue
			}

			neighbourNode := q.nodePool.GetNode(neighbourRef, 0)
			if neighbourNode == nil {
				status |= DT_OUT_OF_NODES
				continue
			}

			if (neighbourNode.Flags & DT_NODE_CLOSED) != 0 {
				continue
			}

			if neighbourNode.Flags == 0 {
				if mid, st := q.getEdgeMidPoint(bestRef, bestPoly, bestTile, neighbourRef, neighbourPoly, neighbourTile); !StatusFailed(st) {
					neighbourNode.Pos = mid
				}
			}

			total := bestNode.Total + vDist(&bestNode.Pos, &neighbourNode.Pos)

			if (neighbourNode.Flags&DT_NODE_OPEN) != 0 && total >= neighbourNode.Total {
				continue
			}

			neighbourNode.ID = neighbourRef
			neighbourNode.Flags &^= DT_NODE_CLOSED
			neighbourNode.Pidx = q.nodePool.GetNodeIdx(bestNode)
			neighbourNode.Total = total

			if (neighbourNode.Flags & DT_NODE_OPEN) != 0 {
				q.openList.Modify(neighbourNode)
			} else {
				neighbourNode.Flags |= DT_NODE_OPEN
				q.openList.Push(neighbourNode)
			}
		}
	}

	var hitNormal Vec3
	vSub(&hitNormal, &centerPos, &hitPos)
	vNormalize(&hitNormal)

	hitDist := float32(math.Sqrt(float64(radiusSqr)))

	return hitDist, hitPos, hitNormal, status
}

// IsValidPolyRef mirrors dtNavMeshQuery::isValidPolyRef
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) IsValidPolyRef(ref PolyRef, filter *QueryFilter) bool {
	tile, poly, st := q.nav.GetTileAndPolyByRef(ref)
	if StatusFailed(st) {
		return false
	}
	if !filter.PassFilter(ref, tile, poly) {
		return false
	}
	return true
}

// IsInClosedList mirrors dtNavMeshQuery::isInClosedList
// (DetourNavMeshQuery.cpp).
func (q *NavMeshQuery) IsInClosedList(ref PolyRef) bool {
	if q.nodePool == nil {
		return false
	}

	nodes := q.nodePool.FindNodes(ref, DT_MAX_STATES_PER_NODE)
	for _, node := range nodes {
		if (node.Flags & DT_NODE_CLOSED) != 0 {
			return true
		}
	}
	return false
}

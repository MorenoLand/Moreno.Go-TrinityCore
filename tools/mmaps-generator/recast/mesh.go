// Polygon mesh building mirrors dep/recastnavigation/Recast/Source/RecastMesh.cpp
// (rcBuildPolyMesh, rcMergePolyMeshes, rcCopyPolyMesh) and the rcPolyMesh
// struct in dep/recastnavigation/Recast/Include/Recast.h.
// The detail mesh (rcBuildPolyMeshDetail) lives in a separate file.
package recast

import "math"

// MeshNullIdx is the rcPolyMesh null vertex/polygon index
// (Recast.h RC_MESH_NULL_IDX).
const MeshNullIdx = 0xffff

// multipleRegs is the rcPolyMesh region id for polygons spanning
// multiple regions (Recast.h RC_MULTIPLE_REGS).
const multipleRegs = 0

// vertexBucketCount mirrors VERTEX_BUCKET_COUNT (RecastMesh.cpp).
const vertexBucketCount = 1 << 12

// triRemovable is the high bit triangulate() sets on indices whose
// vertex can be removed (RecastMesh.cpp triangulate).
const triRemovable = int32(-1 << 31)

// PolyMesh mirrors rcPolyMesh (Recast.h): Verts holds 3 uint16 per vertex
// (x, y, z); Polys holds 2*Nvp uint16 per polygon (vertex indices followed
// by neighbour polygon indices).
type PolyMesh struct {
	Verts  []uint16
	Polys  []uint16
	Regs   []uint16
	Flags  []uint16
	Areas  []uint8
	Nverts int
	Npolys int
	// Maxpolys is the number of allocated polygons.
	Maxpolys     int
	Nvp          int
	Bmin, Bmax   [3]float32
	Cs           float32
	Ch           float32
	BorderSize   int
	MaxEdgeError float32
}

// meshEdge mirrors rcEdge (RecastMesh.cpp).
type meshEdge struct {
	vert     [2]uint16
	polyEdge [2]uint16
	poly     [2]uint16
}

// buildMeshAdjacency mirrors buildMeshAdjacency (RecastMesh.cpp).
func buildMeshAdjacency(polys []uint16, npolys, nverts, vertsPerPoly int) bool {
	maxEdgeCount := npolys * vertsPerPoly
	firstEdge := make([]uint16, nverts+maxEdgeCount)
	nextEdge := firstEdge[nverts:]
	edgeCount := 0

	edges := make([]meshEdge, maxEdgeCount)

	for i := 0; i < nverts; i++ {
		firstEdge[i] = MeshNullIdx
	}

	for i := 0; i < npolys; i++ {
		t := polys[i*vertsPerPoly*2:]
		for j := 0; j < vertsPerPoly; j++ {
			if t[j] == MeshNullIdx {
				break
			}
			v0 := t[j]
			v1 := t[0]
			if j+1 < vertsPerPoly && t[j+1] != MeshNullIdx {
				v1 = t[j+1]
			}
			if v0 < v1 {
				edge := &edges[edgeCount]
				edge.vert[0] = v0
				edge.vert[1] = v1
				edge.poly[0] = uint16(i)
				edge.polyEdge[0] = uint16(j)
				edge.poly[1] = uint16(i)
				edge.polyEdge[1] = 0
				// Insert edge
				nextEdge[edgeCount] = firstEdge[v0]
				firstEdge[v0] = uint16(edgeCount)
				edgeCount++
			}
		}
	}

	for i := 0; i < npolys; i++ {
		t := polys[i*vertsPerPoly*2:]
		for j := 0; j < vertsPerPoly; j++ {
			if t[j] == MeshNullIdx {
				break
			}
			v0 := t[j]
			v1 := t[0]
			if j+1 < vertsPerPoly && t[j+1] != MeshNullIdx {
				v1 = t[j+1]
			}
			if v0 > v1 {
				for e := firstEdge[v1]; e != MeshNullIdx; e = nextEdge[e] {
					edge := &edges[e]
					if edge.vert[1] == v0 && edge.poly[0] == edge.poly[1] {
						edge.poly[1] = uint16(i)
						edge.polyEdge[1] = uint16(j)
						break
					}
				}
			}
		}
	}

	// Store adjacency
	for i := 0; i < edgeCount; i++ {
		e := edges[i]
		if e.poly[0] != e.poly[1] {
			p0 := polys[int(e.poly[0])*vertsPerPoly*2:]
			p1 := polys[int(e.poly[1])*vertsPerPoly*2:]
			p0[vertsPerPoly+int(e.polyEdge[0])] = e.poly[1]
			p1[vertsPerPoly+int(e.polyEdge[1])] = e.poly[0]
		}
	}

	return true
}

// computeVertexHash mirrors computeVertexHash (RecastMesh.cpp).
func computeVertexHash(x, y, z int) int {
	h1 := uint32(0x8da6b343)
	h2 := uint32(0xd8163841)
	h3 := uint32(0xcb1ab31f)
	n := h1*uint32(x) + h2*uint32(y) + h3*uint32(z)
	return int(n & (vertexBucketCount - 1))
}

// addVertex mirrors addVertex (RecastMesh.cpp).
func addVertex(x, y, z uint16, verts []uint16, firstVert, nextVert []int, nv *int) uint16 {
	bucket := computeVertexHash(int(x), 0, int(z))
	i := firstVert[bucket]

	for i != -1 {
		v := verts[i*3:]
		if v[0] == x && absInt(int(v[1])-int(y)) <= 2 && v[2] == z {
			return uint16(i)
		}
		i = nextVert[i] // next
	}

	// Could not find, create new.
	i = *nv
	*nv++
	v := verts[i*3:]
	v[0] = x
	v[1] = y
	v[2] = z
	nextVert[i] = firstVert[bucket]
	firstVert[bucket] = i

	return uint16(i)
}

// meshPrev mirrors prev (RecastMesh.cpp).
func meshPrev(i, n int) int {
	if i-1 >= 0 {
		return i - 1
	}
	return n - 1
}

// meshNext mirrors next (RecastMesh.cpp).
func meshNext(i, n int) int {
	if i+1 < n {
		return i + 1
	}
	return 0
}

// meshArea2 mirrors area2 (RecastMesh.cpp).
func meshArea2(a, b, c []int32) int {
	return int(b[0]-a[0])*int(c[2]-a[2]) - int(c[0]-a[0])*int(b[2]-a[2])
}

// meshXorb mirrors xorb (RecastMesh.cpp).
func meshXorb(x, y bool) bool {
	return !x != !y
}

// meshLeft mirrors left (RecastMesh.cpp).
func meshLeft(a, b, c []int32) bool {
	return meshArea2(a, b, c) < 0
}

// meshLeftOn mirrors leftOn (RecastMesh.cpp).
func meshLeftOn(a, b, c []int32) bool {
	return meshArea2(a, b, c) <= 0
}

// meshCollinear mirrors collinear (RecastMesh.cpp).
func meshCollinear(a, b, c []int32) bool {
	return meshArea2(a, b, c) == 0
}

// meshIntersectProp mirrors intersectProp (RecastMesh.cpp).
func meshIntersectProp(a, b, c, d []int32) bool {
	// Eliminate improper cases.
	if meshCollinear(a, b, c) || meshCollinear(a, b, d) ||
		meshCollinear(c, d, a) || meshCollinear(c, d, b) {
		return false
	}

	return meshXorb(meshLeft(a, b, c), meshLeft(a, b, d)) &&
		meshXorb(meshLeft(c, d, a), meshLeft(c, d, b))
}

// meshBetween mirrors between (RecastMesh.cpp).
func meshBetween(a, b, c []int32) bool {
	if !meshCollinear(a, b, c) {
		return false
	}
	// If ab not vertical, check betweenness on x; else on y.
	if a[0] != b[0] {
		return ((a[0] <= c[0]) && (c[0] <= b[0])) || ((a[0] >= c[0]) && (c[0] >= b[0]))
	}
	return ((a[2] <= c[2]) && (c[2] <= b[2])) || ((a[2] >= c[2]) && (c[2] >= b[2]))
}

// meshIntersect mirrors intersect (RecastMesh.cpp).
func meshIntersect(a, b, c, d []int32) bool {
	if meshIntersectProp(a, b, c, d) {
		return true
	} else if meshBetween(a, b, c) || meshBetween(a, b, d) ||
		meshBetween(c, d, a) || meshBetween(c, d, b) {
		return true
	}
	return false
}

// meshVequal mirrors vequal (RecastMesh.cpp).
func meshVequal(a, b []int32) bool {
	return a[0] == b[0] && a[2] == b[2]
}

// meshDiagonalie mirrors diagonalie (RecastMesh.cpp).
func meshDiagonalie(i, j, n int, verts []int32, indices []int32) bool {
	d0 := verts[(indices[i]&0x0fffffff)*4:]
	d1 := verts[(indices[j]&0x0fffffff)*4:]

	// For each edge (k,k+1) of P
	for k := 0; k < n; k++ {
		k1 := meshNext(k, n)
		// Skip edges incident to i or j
		if !((k == i) || (k1 == i) || (k == j) || (k1 == j)) {
			p0 := verts[(indices[k]&0x0fffffff)*4:]
			p1 := verts[(indices[k1]&0x0fffffff)*4:]

			if meshVequal(d0, p0) || meshVequal(d1, p0) || meshVequal(d0, p1) || meshVequal(d1, p1) {
				continue
			}

			if meshIntersect(d0, d1, p0, p1) {
				return false
			}
		}
	}
	return true
}

// meshInCone mirrors inCone (RecastMesh.cpp).
func meshInCone(i, j, n int, verts []int32, indices []int32) bool {
	pi := verts[(indices[i]&0x0fffffff)*4:]
	pj := verts[(indices[j]&0x0fffffff)*4:]
	pi1 := verts[(indices[meshNext(i, n)]&0x0fffffff)*4:]
	pin1 := verts[(indices[meshPrev(i, n)]&0x0fffffff)*4:]

	// If P[i] is a convex vertex [ i+1 left or on (i-1,i) ].
	if meshLeftOn(pin1, pi, pi1) {
		return meshLeft(pi, pj, pin1) && meshLeft(pj, pi, pi1)
	}
	// Assume (i-1,i,i+1) not collinear.
	// else P[i] is reflex.
	return !(meshLeftOn(pi, pj, pi1) && meshLeftOn(pj, pi, pin1))
}

// meshDiagonal mirrors diagonal (RecastMesh.cpp).
func meshDiagonal(i, j, n int, verts []int32, indices []int32) bool {
	return meshInCone(i, j, n, verts, indices) && meshDiagonalie(i, j, n, verts, indices)
}

// meshDiagonalieLoose mirrors diagonalieLoose (RecastMesh.cpp).
func meshDiagonalieLoose(i, j, n int, verts []int32, indices []int32) bool {
	d0 := verts[(indices[i]&0x0fffffff)*4:]
	d1 := verts[(indices[j]&0x0fffffff)*4:]

	// For each edge (k,k+1) of P
	for k := 0; k < n; k++ {
		k1 := meshNext(k, n)
		// Skip edges incident to i or j
		if !((k == i) || (k1 == i) || (k == j) || (k1 == j)) {
			p0 := verts[(indices[k]&0x0fffffff)*4:]
			p1 := verts[(indices[k1]&0x0fffffff)*4:]

			if meshVequal(d0, p0) || meshVequal(d1, p0) || meshVequal(d0, p1) || meshVequal(d1, p1) {
				continue
			}

			if meshIntersectProp(d0, d1, p0, p1) {
				return false
			}
		}
	}
	return true
}

// meshInConeLoose mirrors inConeLoose (RecastMesh.cpp).
func meshInConeLoose(i, j, n int, verts []int32, indices []int32) bool {
	pi := verts[(indices[i]&0x0fffffff)*4:]
	pj := verts[(indices[j]&0x0fffffff)*4:]
	pi1 := verts[(indices[meshNext(i, n)]&0x0fffffff)*4:]
	pin1 := verts[(indices[meshPrev(i, n)]&0x0fffffff)*4:]

	// If P[i] is a convex vertex [ i+1 left or on (i-1,i) ].
	if meshLeftOn(pin1, pi, pi1) {
		return meshLeftOn(pi, pj, pin1) && meshLeftOn(pj, pi, pi1)
	}
	// Assume (i-1,i,i+1) not collinear.
	// else P[i] is reflex.
	return !(meshLeftOn(pi, pj, pi1) && meshLeftOn(pj, pi, pin1))
}

// meshDiagonalLoose mirrors diagonalLoose (RecastMesh.cpp).
func meshDiagonalLoose(i, j, n int, verts []int32, indices []int32) bool {
	return meshInConeLoose(i, j, n, verts, indices) && meshDiagonalieLoose(i, j, n, verts, indices)
}

// triangulate mirrors triangulate (RecastMesh.cpp). Returns the number of
// triangles, or the negated count when the contour was too messed up to
// finish (loose recovery failed).
func triangulate(n int, verts []int32, indices []int32, tris []int32) int {
	ntris := 0
	dst := 0

	// The last bit of the index is used to indicate if the vertex can be removed.
	for i := 0; i < n; i++ {
		i1 := meshNext(i, n)
		i2 := meshNext(i1, n)
		if meshDiagonal(i, i2, n, verts, indices) {
			indices[i1] |= triRemovable
		}
	}

	for n > 3 {
		minLen := -1
		mini := -1
		for i := 0; i < n; i++ {
			i1 := meshNext(i, n)
			if indices[i1]&triRemovable != 0 {
				p0 := verts[(indices[i]&0x0fffffff)*4:]
				p2 := verts[(indices[meshNext(i1, n)]&0x0fffffff)*4:]

				dx := int(p2[0]) - int(p0[0])
				dy := int(p2[2]) - int(p0[2])
				length := dx*dx + dy*dy

				if minLen < 0 || length < minLen {
					minLen = length
					mini = i
				}
			}
		}

		if mini == -1 {
			// We might get here because the contour has overlapping segments, like this:
			//
			//  A o-o=====o---o B
			//   /  |C   D|    \.
			//  o   o     o     o
			//  :   :     :     :
			// We'll try to recover by loosing up the inCone test a bit so that a diagonal
			// like A-B or C-D can be found and we can continue.
			minLen = -1
			mini = -1
			for i := 0; i < n; i++ {
				i1 := meshNext(i, n)
				i2 := meshNext(i1, n)
				if meshDiagonalLoose(i, i2, n, verts, indices) {
					p0 := verts[(indices[i]&0x0fffffff)*4:]
					p2 := verts[(indices[meshNext(i2, n)]&0x0fffffff)*4:]
					dx := int(p2[0]) - int(p0[0])
					dy := int(p2[2]) - int(p0[2])
					length := dx*dx + dy*dy

					if minLen < 0 || length < minLen {
						minLen = length
						mini = i
					}
				}
			}
			if mini == -1 {
				// The contour is messed up. This sometimes happens
				// if the contour simplification is too aggressive.
				return -ntris
			}
		}

		i := mini
		i1 := meshNext(i, n)
		i2 := meshNext(i1, n)

		tris[dst+0] = indices[i] & 0x0fffffff
		tris[dst+1] = indices[i1] & 0x0fffffff
		tris[dst+2] = indices[i2] & 0x0fffffff
		dst += 3
		ntris++

		// Removes P[i1] by copying P[i+1]...P[n-1] left one index.
		n--
		for k := i1; k < n; k++ {
			indices[k] = indices[k+1]
		}

		if i1 >= n {
			i1 = 0
		}
		i = meshPrev(i1, n)
		// Update diagonal flags.
		if meshDiagonal(meshPrev(i, n), i1, n, verts, indices) {
			indices[i] |= triRemovable
		} else {
			indices[i] &= 0x0fffffff
		}

		if meshDiagonal(i, meshNext(i1, n), n, verts, indices) {
			indices[i1] |= triRemovable
		} else {
			indices[i1] &= 0x0fffffff
		}
	}

	// Append the remaining triangle.
	tris[dst+0] = indices[0] & 0x0fffffff
	tris[dst+1] = indices[1] & 0x0fffffff
	tris[dst+2] = indices[2] & 0x0fffffff
	ntris++

	return ntris
}

// countPolyVerts mirrors countPolyVerts (RecastMesh.cpp).
func countPolyVerts(p []uint16, nvp int) int {
	for i := 0; i < nvp; i++ {
		if p[i] == MeshNullIdx {
			return i
		}
	}
	return nvp
}

// uleft mirrors uleft (RecastMesh.cpp).
func uleft(a, b, c []uint16) bool {
	return (int(b[0])-int(a[0]))*(int(c[2])-int(a[2]))-
		(int(c[0])-int(a[0]))*(int(b[2])-int(a[2])) < 0
}

// getPolyMergeValue mirrors getPolyMergeValue (RecastMesh.cpp).
func getPolyMergeValue(pa, pb []uint16, verts []uint16, ea, eb *int, nvp int) int {
	na := countPolyVerts(pa, nvp)
	nb := countPolyVerts(pb, nvp)

	// If the merged polygon would be too big, do not merge.
	if na+nb-2 > nvp {
		return -1
	}

	// Check if the polygons share an edge.
	*ea = -1
	*eb = -1

	for i := 0; i < na; i++ {
		va0 := pa[i]
		va1 := pa[(i+1)%na]
		if va0 > va1 {
			va0, va1 = va1, va0
		}
		for j := 0; j < nb; j++ {
			vb0 := pb[j]
			vb1 := pb[(j+1)%nb]
			if vb0 > vb1 {
				vb0, vb1 = vb1, vb0
			}
			if va0 == vb0 && va1 == vb1 {
				*ea = i
				*eb = j
				break
			}
		}
	}

	// No common edge, cannot merge.
	if *ea == -1 || *eb == -1 {
		return -1
	}

	// Check to see if the merged polygon would be convex.
	va := pa[(*ea+na-1)%na]
	vb := pa[*ea]
	vc := pb[(*eb+2)%nb]
	if !uleft(verts[va*3:], verts[vb*3:], verts[vc*3:]) {
		return -1
	}

	va = pb[(*eb+nb-1)%nb]
	vb = pb[*eb]
	vc = pa[(*ea+2)%na]
	if !uleft(verts[va*3:], verts[vb*3:], verts[vc*3:]) {
		return -1
	}

	va = pa[*ea]
	vb = pa[(*ea+1)%na]

	dx := int(verts[va*3+0]) - int(verts[vb*3+0])
	dy := int(verts[va*3+2]) - int(verts[vb*3+2])

	return dx*dx + dy*dy
}

// mergePolyVerts mirrors mergePolyVerts (RecastMesh.cpp).
func mergePolyVerts(pa, pb []uint16, ea, eb int, tmp []uint16, nvp int) {
	na := countPolyVerts(pa, nvp)
	nb := countPolyVerts(pb, nvp)

	// Merge polygons.
	for i := 0; i < nvp; i++ {
		tmp[i] = MeshNullIdx
	}
	n := 0
	// Add pa
	for i := 0; i < na-1; i++ {
		tmp[n] = pa[(ea+1+i)%na]
		n++
	}
	// Add pb
	for i := 0; i < nb-1; i++ {
		tmp[n] = pb[(eb+1+i)%nb]
		n++
	}

	copy(pa, tmp[:nvp])
}

// pushFront mirrors pushFront (RecastMesh.cpp).
func pushFront(v int, arr []int, an *int) {
	*an++
	for i := *an - 1; i > 0; i-- {
		arr[i] = arr[i-1]
	}
	arr[0] = v
}

// pushBack mirrors pushBack (RecastMesh.cpp).
func pushBack(v int, arr []int, an *int) {
	arr[*an] = v
	*an++
}

// canRemoveVertex mirrors canRemoveVertex (RecastMesh.cpp).
func canRemoveVertex(mesh *PolyMesh, rem uint16) bool {
	nvp := mesh.Nvp

	// Count number of polygons to remove.
	numRemovedVerts := 0
	numTouchedVerts := 0
	numRemainingEdges := 0
	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		nv := countPolyVerts(p, nvp)
		numRemoved := 0
		numVerts := 0
		for j := 0; j < nv; j++ {
			if p[j] == rem {
				numTouchedVerts++
				numRemoved++
			}
			numVerts++
		}
		if numRemoved != 0 {
			numRemovedVerts += numRemoved
			numRemainingEdges += numVerts - (numRemoved + 1)
		}
	}

	// There would be too few edges remaining to create a polygon.
	// This can happen for example when a tip of a triangle is marked
	// as deletion, but there are no other polys that share the vertex.
	// In this case, the vertex should not be removed.
	if numRemainingEdges <= 2 {
		return false
	}

	// Find edges which share the removed vertex.
	maxEdges := numTouchedVerts * 2
	nedges := 0
	edges := make([]int, maxEdges*3)

	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		nv := countPolyVerts(p, nvp)

		// Collect edges which touches the removed vertex.
		for j, k := 0, nv-1; j < nv; k, j = j, j+1 {
			if p[j] == rem || p[k] == rem {
				// Arrange edge so that a=rem.
				a := int(p[j])
				b := int(p[k])
				if b == int(rem) {
					a, b = b, a
				}

				// Check if the edge exists
				exists := false
				for m := 0; m < nedges; m++ {
					e := edges[m*3:]
					if e[1] == b {
						// Exists, increment vertex share count.
						e[2]++
						exists = true
					}
				}
				// Add new edge.
				if !exists {
					e := edges[nedges*3:]
					e[0] = a
					e[1] = b
					e[2] = 1
					nedges++
				}
			}
		}
	}

	// There should be no more than 2 open edges.
	// This catches the case that two non-adjacent polygons
	// share the removed vertex. In that case, do not remove the vertex.
	numOpenEdges := 0
	for i := 0; i < nedges; i++ {
		if edges[i*3+2] < 2 {
			numOpenEdges++
		}
	}
	if numOpenEdges > 2 {
		return false
	}

	return true
}

// removeVertex mirrors removeVertex (RecastMesh.cpp).
func removeVertex(mesh *PolyMesh, rem uint16, maxTris int) bool {
	nvp := mesh.Nvp

	// Count number of polygons to remove.
	numRemovedVerts := 0
	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		nv := countPolyVerts(p, nvp)
		for j := 0; j < nv; j++ {
			if p[j] == rem {
				numRemovedVerts++
			}
		}
	}

	nedges := 0
	edges := make([]int, numRemovedVerts*nvp*4)

	nhole := 0
	hole := make([]int, numRemovedVerts*nvp)

	nhreg := 0
	hreg := make([]int, numRemovedVerts*nvp)

	nharea := 0
	harea := make([]int, numRemovedVerts*nvp)

	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		nv := countPolyVerts(p, nvp)
		hasRem := false
		for j := 0; j < nv; j++ {
			if p[j] == rem {
				hasRem = true
			}
		}
		if hasRem {
			// Collect edges which does not touch the removed vertex.
			for j, k := 0, nv-1; j < nv; k, j = j, j+1 {
				if p[j] != rem && p[k] != rem {
					e := edges[nedges*4:]
					e[0] = int(p[k])
					e[1] = int(p[j])
					e[2] = int(mesh.Regs[i])
					e[3] = int(mesh.Areas[i])
					nedges++
				}
			}
			// Remove the polygon.
			p2 := mesh.Polys[(mesh.Npolys-1)*nvp*2:]
			if &p[0] != &p2[0] {
				copy(p, p2[:nvp])
			}
			for k := nvp; k < nvp*2; k++ {
				p[k] = MeshNullIdx
			}
			mesh.Regs[i] = mesh.Regs[mesh.Npolys-1]
			mesh.Areas[i] = mesh.Areas[mesh.Npolys-1]
			mesh.Npolys--
			i--
		}
	}

	// Remove vertex.
	for i := int(rem); i < mesh.Nverts-1; i++ {
		mesh.Verts[i*3+0] = mesh.Verts[(i+1)*3+0]
		mesh.Verts[i*3+1] = mesh.Verts[(i+1)*3+1]
		mesh.Verts[i*3+2] = mesh.Verts[(i+1)*3+2]
	}
	mesh.Nverts--

	// Adjust indices to match the removed vertex layout.
	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		nv := countPolyVerts(p, nvp)
		for j := 0; j < nv; j++ {
			if p[j] > rem {
				p[j]--
			}
		}
	}
	for i := 0; i < nedges; i++ {
		if edges[i*4+0] > int(rem) {
			edges[i*4+0]--
		}
		if edges[i*4+1] > int(rem) {
			edges[i*4+1]--
		}
	}

	if nedges == 0 {
		return true
	}

	// Start with one vertex, keep appending connected
	// segments to the start and end of the hole.
	pushBack(edges[0], hole, &nhole)
	pushBack(edges[2], hreg, &nhreg)
	pushBack(edges[3], harea, &nharea)

	for nedges != 0 {
		match := false

		for i := 0; i < nedges; i++ {
			ea := edges[i*4+0]
			eb := edges[i*4+1]
			r := edges[i*4+2]
			a := edges[i*4+3]
			add := false
			if hole[0] == eb {
				// The segment matches the beginning of the hole boundary.
				pushFront(ea, hole, &nhole)
				pushFront(r, hreg, &nhreg)
				pushFront(a, harea, &nharea)
				add = true
			} else if hole[nhole-1] == ea {
				// The segment matches the end of the hole boundary.
				pushBack(eb, hole, &nhole)
				pushBack(r, hreg, &nhreg)
				pushBack(a, harea, &nharea)
				add = true
			}
			if add {
				// The edge segment was added, remove it.
				edges[i*4+0] = edges[(nedges-1)*4+0]
				edges[i*4+1] = edges[(nedges-1)*4+1]
				edges[i*4+2] = edges[(nedges-1)*4+2]
				edges[i*4+3] = edges[(nedges-1)*4+3]
				nedges--
				match = true
				i--
			}
		}

		if !match {
			break
		}
	}

	tris := make([]int32, nhole*3)
	tverts := make([]int32, nhole*4)
	thole := make([]int32, nhole)

	// Generate temp vertex array for triangulation.
	for i := 0; i < nhole; i++ {
		pi := hole[i]
		tverts[i*4+0] = int32(mesh.Verts[pi*3+0])
		tverts[i*4+1] = int32(mesh.Verts[pi*3+1])
		tverts[i*4+2] = int32(mesh.Verts[pi*3+2])
		tverts[i*4+3] = 0
		thole[i] = int32(i)
	}

	// Triangulate the hole.
	ntris := triangulate(nhole, tverts, thole, tris)
	if ntris < 0 {
		ntris = -ntris
	}

	// Merge the hole triangles back to polygons.
	polys := make([]uint16, (ntris+1)*nvp)
	pregs := make([]uint16, ntris)
	pareas := make([]uint8, ntris)

	tmpPoly := polys[ntris*nvp:]

	// Build initial polygons.
	npolys := 0
	for i := 0; i < ntris*nvp; i++ {
		polys[i] = MeshNullIdx
	}
	for j := 0; j < ntris; j++ {
		t := tris[j*3:]
		if t[0] != t[1] && t[0] != t[2] && t[1] != t[2] {
			polys[npolys*nvp+0] = uint16(hole[t[0]])
			polys[npolys*nvp+1] = uint16(hole[t[1]])
			polys[npolys*nvp+2] = uint16(hole[t[2]])

			// If this polygon covers multiple region types then
			// mark it as such
			if hreg[t[0]] != hreg[t[1]] || hreg[t[1]] != hreg[t[2]] {
				pregs[npolys] = multipleRegs
			} else {
				pregs[npolys] = uint16(hreg[t[0]])
			}

			pareas[npolys] = uint8(harea[t[0]])
			npolys++
		}
	}
	if npolys == 0 {
		return true
	}

	// Merge polygons.
	if nvp > 3 {
		for {
			// Find best polygons to merge.
			bestMergeVal := 0
			bestPa := 0
			bestPb := 0
			bestEa := 0
			bestEb := 0

			for j := 0; j < npolys-1; j++ {
				pj := polys[j*nvp:]
				for k := j + 1; k < npolys; k++ {
					pk := polys[k*nvp:]
					ea, eb := 0, 0
					v := getPolyMergeValue(pj, pk, mesh.Verts, &ea, &eb, nvp)
					if v > bestMergeVal {
						bestMergeVal = v
						bestPa = j
						bestPb = k
						bestEa = ea
						bestEb = eb
					}
				}
			}

			if bestMergeVal > 0 {
				// Found best, merge.
				pa := polys[bestPa*nvp:]
				pb := polys[bestPb*nvp:]
				mergePolyVerts(pa, pb, bestEa, bestEb, tmpPoly, nvp)
				if pregs[bestPa] != pregs[bestPb] {
					pregs[bestPa] = multipleRegs
				}

				last := polys[(npolys-1)*nvp:]
				if bestPb != npolys-1 {
					copy(pb, last[:nvp])
				}
				pregs[bestPb] = pregs[npolys-1]
				pareas[bestPb] = pareas[npolys-1]
				npolys--
			} else {
				// Could not merge any polygons, stop.
				break
			}
		}
	}

	// Store polygons.
	for i := 0; i < npolys; i++ {
		if mesh.Npolys >= maxTris {
			break
		}
		p := mesh.Polys[mesh.Npolys*nvp*2:]
		for k := 0; k < nvp*2; k++ {
			p[k] = MeshNullIdx
		}
		for j := 0; j < nvp; j++ {
			p[j] = polys[i*nvp+j]
		}
		mesh.Regs[mesh.Npolys] = pregs[i]
		mesh.Areas[mesh.Npolys] = pareas[i]
		mesh.Npolys++
		if mesh.Npolys > maxTris {
			return false
		}
	}

	return true
}

// BuildPolyMesh mirrors rcBuildPolyMesh (RecastMesh.cpp).
//
// @note If the mesh data is to be used to construct a Detour navigation mesh,
// then the upper limit must be retricted to <= #DT_VERTS_PER_POLYGON.
func BuildPolyMesh(cset *ContourSet, nvp int, mesh *PolyMesh) bool {
	mesh.Bmin = cset.Bmin
	mesh.Bmax = cset.Bmax
	mesh.Cs = cset.Cs
	mesh.Ch = cset.Ch
	mesh.BorderSize = cset.BorderSize
	mesh.MaxEdgeError = cset.MaxError

	maxVertices := 0
	maxTris := 0
	maxVertsPerCont := 0
	for i := 0; i < cset.Nconts; i++ {
		// Skip null contours.
		if cset.Conts[i].Nverts < 3 {
			continue
		}
		maxVertices += cset.Conts[i].Nverts
		maxTris += cset.Conts[i].Nverts - 2
		maxVertsPerCont = maxInt(maxVertsPerCont, cset.Conts[i].Nverts)
	}

	if maxVertices >= 0xfffe {
		return false
	}

	vflags := make([]uint8, maxVertices)

	mesh.Verts = make([]uint16, maxVertices*3)
	mesh.Polys = make([]uint16, maxTris*nvp*2)
	mesh.Regs = make([]uint16, maxTris)
	mesh.Areas = make([]uint8, maxTris)

	mesh.Nverts = 0
	mesh.Npolys = 0
	mesh.Nvp = nvp
	mesh.Maxpolys = maxTris

	for i := range mesh.Polys {
		mesh.Polys[i] = MeshNullIdx
	}

	nextVert := make([]int, maxVertices)

	firstVert := make([]int, vertexBucketCount)
	for i := range firstVert {
		firstVert[i] = -1
	}

	indices := make([]int32, maxVertsPerCont)
	tris := make([]int32, maxVertsPerCont*3)
	polys := make([]uint16, (maxVertsPerCont+1)*nvp)
	tmpPoly := polys[maxVertsPerCont*nvp:]

	for i := 0; i < cset.Nconts; i++ {
		cont := &cset.Conts[i]

		// Skip null contours.
		if cont.Nverts < 3 {
			continue
		}

		// Triangulate contour
		for j := 0; j < cont.Nverts; j++ {
			indices[j] = int32(j)
		}

		ntris := triangulate(cont.Nverts, cont.Verts, indices, tris)
		if ntris <= 0 {
			// Bad triangulation, should not happen.
			ntris = -ntris
		}

		// Add and merge vertices.
		for j := 0; j < cont.Nverts; j++ {
			v := cont.Verts[j*4:]
			indices[j] = int32(addVertex(uint16(v[0]), uint16(v[1]), uint16(v[2]),
				mesh.Verts, firstVert, nextVert, &mesh.Nverts))
			if v[3]&borderVertex != 0 {
				// This vertex should be removed.
				vflags[indices[j]] = 1
			}
		}

		// Build initial polygons.
		npolys := 0
		for k := 0; k < maxVertsPerCont*nvp; k++ {
			polys[k] = MeshNullIdx
		}
		for j := 0; j < ntris; j++ {
			t := tris[j*3:]
			if t[0] != t[1] && t[0] != t[2] && t[1] != t[2] {
				polys[npolys*nvp+0] = uint16(indices[t[0]])
				polys[npolys*nvp+1] = uint16(indices[t[1]])
				polys[npolys*nvp+2] = uint16(indices[t[2]])
				npolys++
			}
		}
		if npolys == 0 {
			continue
		}

		// Merge polygons.
		if nvp > 3 {
			for {
				// Find best polygons to merge.
				bestMergeVal := 0
				bestPa := 0
				bestPb := 0
				bestEa := 0
				bestEb := 0

				for j := 0; j < npolys-1; j++ {
					pj := polys[j*nvp:]
					for k := j + 1; k < npolys; k++ {
						pk := polys[k*nvp:]
						ea, eb := 0, 0
						v := getPolyMergeValue(pj, pk, mesh.Verts, &ea, &eb, nvp)
						if v > bestMergeVal {
							bestMergeVal = v
							bestPa = j
							bestPb = k
							bestEa = ea
							bestEb = eb
						}
					}
				}

				if bestMergeVal > 0 {
					// Found best, merge.
					pa := polys[bestPa*nvp:]
					pb := polys[bestPb*nvp:]
					mergePolyVerts(pa, pb, bestEa, bestEb, tmpPoly, nvp)
					lastPoly := polys[(npolys-1)*nvp:]
					if bestPb != npolys-1 {
						copy(pb, lastPoly[:nvp])
					}
					npolys--
				} else {
					// Could not merge any polygons, stop.
					break
				}
			}
		}

		// Store polygons.
		for j := 0; j < npolys; j++ {
			p := mesh.Polys[mesh.Npolys*nvp*2:]
			q := polys[j*nvp:]
			for k := 0; k < nvp; k++ {
				p[k] = q[k]
			}
			mesh.Regs[mesh.Npolys] = cont.Reg
			mesh.Areas[mesh.Npolys] = cont.Area
			mesh.Npolys++
			if mesh.Npolys > maxTris {
				return false
			}
		}
	}

	// Remove edge vertices.
	for i := 0; i < mesh.Nverts; i++ {
		if vflags[i] != 0 {
			if !canRemoveVertex(mesh, uint16(i)) {
				continue
			}
			if !removeVertex(mesh, uint16(i), maxTris) {
				// Failed to remove vertex
				return false
			}
			// Remove vertex
			// Note: mesh.Nverts is already decremented inside removeVertex()!
			// Fixup vertex flags
			for j := i; j < mesh.Nverts; j++ {
				vflags[j] = vflags[j+1]
			}
			i--
		}
	}

	// Calculate adjacency.
	if !buildMeshAdjacency(mesh.Polys, mesh.Npolys, mesh.Nverts, nvp) {
		return false
	}

	// Find portal edges
	if mesh.BorderSize > 0 {
		w := cset.Width
		h := cset.Height
		for i := 0; i < mesh.Npolys; i++ {
			p := mesh.Polys[i*2*nvp:]
			for j := 0; j < nvp; j++ {
				if p[j] == MeshNullIdx {
					break
				}
				// Skip connected edges.
				if p[nvp+j] != MeshNullIdx {
					continue
				}
				nj := j + 1
				if nj >= nvp || p[nj] == MeshNullIdx {
					nj = 0
				}
				va := mesh.Verts[p[j]*3:]
				vb := mesh.Verts[p[nj]*3:]

				if int(va[0]) == 0 && int(vb[0]) == 0 {
					p[nvp+j] = 0x8000 | 0
				} else if int(va[2]) == h && int(vb[2]) == h {
					p[nvp+j] = 0x8000 | 1
				} else if int(va[0]) == w && int(vb[0]) == w {
					p[nvp+j] = 0x8000 | 2
				} else if int(va[2]) == 0 && int(vb[2]) == 0 {
					p[nvp+j] = 0x8000 | 3
				}
			}
		}
	}

	// Just allocate the mesh flags array. The user is resposible to fill it.
	mesh.Flags = make([]uint16, mesh.Npolys)

	return true
}

// MergePolyMeshes mirrors rcMergePolyMeshes (RecastMesh.cpp).
func MergePolyMeshes(meshes []*PolyMesh, out *PolyMesh) bool {
	if len(meshes) == 0 {
		return true
	}

	out.Nvp = meshes[0].Nvp
	out.Cs = meshes[0].Cs
	out.Ch = meshes[0].Ch
	out.Bmin = meshes[0].Bmin
	out.Bmax = meshes[0].Bmax

	maxVerts := 0
	maxPolys := 0
	maxVertsPerMesh := 0
	for i := 0; i < len(meshes); i++ {
		out.Bmin = vMin(out.Bmin, meshes[i].Bmin)
		out.Bmax = vMax(out.Bmax, meshes[i].Bmax)
		maxVertsPerMesh = maxInt(maxVertsPerMesh, meshes[i].Nverts)
		maxVerts += meshes[i].Nverts
		maxPolys += meshes[i].Npolys
	}

	out.Nverts = 0
	out.Verts = make([]uint16, maxVerts*3)

	out.Npolys = 0
	out.Polys = make([]uint16, maxPolys*2*out.Nvp)
	for i := range out.Polys {
		out.Polys[i] = MeshNullIdx
	}

	out.Regs = make([]uint16, maxPolys)
	out.Areas = make([]uint8, maxPolys)
	out.Flags = make([]uint16, maxPolys)

	nextVert := make([]int, maxVerts)

	firstVert := make([]int, vertexBucketCount)
	for i := range firstVert {
		firstVert[i] = -1
	}

	vremap := make([]uint16, maxVertsPerMesh)

	for i := 0; i < len(meshes); i++ {
		pmesh := meshes[i]

		ox := uint16(math.Floor(float64((pmesh.Bmin[0]-out.Bmin[0])/out.Cs + 0.5)))
		oz := uint16(math.Floor(float64((pmesh.Bmin[2]-out.Bmin[2])/out.Cs + 0.5)))

		isMinX := ox == 0
		isMinZ := oz == 0
		isMaxX := uint16(math.Floor(float64((out.Bmax[0]-pmesh.Bmax[0])/out.Cs+0.5))) == 0
		isMaxZ := uint16(math.Floor(float64((out.Bmax[2]-pmesh.Bmax[2])/out.Cs+0.5))) == 0
		isOnBorder := isMinX || isMinZ || isMaxX || isMaxZ

		for j := 0; j < pmesh.Nverts; j++ {
			v := pmesh.Verts[j*3:]
			vremap[j] = addVertex(v[0]+ox, v[1], v[2]+oz,
				out.Verts, firstVert, nextVert, &out.Nverts)
		}

		for j := 0; j < pmesh.Npolys; j++ {
			tgt := out.Polys[out.Npolys*2*out.Nvp:]
			src := pmesh.Polys[j*2*pmesh.Nvp:]
			out.Regs[out.Npolys] = pmesh.Regs[j]
			out.Areas[out.Npolys] = pmesh.Areas[j]
			out.Flags[out.Npolys] = pmesh.Flags[j]
			out.Npolys++
			for k := 0; k < out.Nvp; k++ {
				if src[k] == MeshNullIdx {
					break
				}
				tgt[k] = vremap[src[k]]
			}

			if isOnBorder {
				for k := out.Nvp; k < out.Nvp*2; k++ {
					if src[k]&0x8000 != 0 && src[k] != 0xffff {
						dir := src[k] & 0xf
						switch dir {
						case 0: // Portal x-
							if isMinX {
								tgt[k] = src[k]
							}
						case 1: // Portal z+
							if isMaxZ {
								tgt[k] = src[k]
							}
						case 2: // Portal x+
							if isMaxX {
								tgt[k] = src[k]
							}
						case 3: // Portal z-
							if isMinZ {
								tgt[k] = src[k]
							}
						}
					}
				}
			}
		}
	}

	// Calculate adjacency.
	if !buildMeshAdjacency(out.Polys, out.Npolys, out.Nverts, out.Nvp) {
		return false
	}

	return true
}

// CopyPolyMesh mirrors rcCopyPolyMesh (RecastMesh.cpp).
func CopyPolyMesh(src, dst *PolyMesh) bool {
	dst.Nverts = src.Nverts
	dst.Npolys = src.Npolys
	dst.Maxpolys = src.Npolys
	dst.Nvp = src.Nvp
	dst.Bmin = src.Bmin
	dst.Bmax = src.Bmax
	dst.Cs = src.Cs
	dst.Ch = src.Ch
	dst.BorderSize = src.BorderSize
	dst.MaxEdgeError = src.MaxEdgeError

	dst.Verts = append([]uint16(nil), src.Verts[:src.Nverts*3]...)
	dst.Polys = append([]uint16(nil), src.Polys[:src.Npolys*2*src.Nvp]...)
	dst.Regs = append([]uint16(nil), src.Regs[:src.Npolys]...)
	dst.Areas = append([]uint8(nil), src.Areas[:src.Npolys]...)
	dst.Flags = append([]uint16(nil), src.Flags[:src.Npolys]...)

	return true
}

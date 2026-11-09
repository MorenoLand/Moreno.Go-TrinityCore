// Detail mesh building mirrors dep/recastnavigation/Recast/Source/RecastMeshDetail.cpp
// (rcBuildPolyMeshDetail, rcMergePolyMeshDetails) and the rcPolyMeshDetail
// struct in dep/recastnavigation/Recast/Include/Recast.h.
package recast

import "math"

// unsetHeight is the height-patch value for "no height sampled"
// (RecastMeshDetail.cpp RC_UNSET_HEIGHT).
const unsetHeight = 0xffff

// PolyMeshDetail mirrors rcPolyMeshDetail (Recast.h): Meshes holds 4 uint32
// per sub-mesh (first vertex, vertex count, first triangle, triangle count),
// Verts holds 3 floats per vertex, Tris holds 4 uint8 per triangle
// (3 vertex indices plus edge flags).
type PolyMeshDetail struct {
	Meshes  []uint32
	Verts   []float32
	Tris    []uint8
	Nmeshes int
	Nverts  int
	Ntris   int
}

// heightPatch mirrors rcHeightPatch (RecastMeshDetail.cpp).
type heightPatch struct {
	data          []uint16
	xmin, ymin    int
	width, height int
}

// intArray mirrors rcIntArray (Recast.h): a resizable int slice.
type intArray []int

func (a *intArray) resize(n int) {
	if n < len(*a) {
		*a = (*a)[:n]
	} else if n > len(*a) {
		*a = append(*a, make([]int, n-len(*a))...)
	}
}

func (a *intArray) push(v int) {
	*a = append(*a, v)
}

func (a *intArray) pop() int {
	v := (*a)[len(*a)-1]
	*a = (*a)[:len(*a)-1]
	return v
}

// detailVdot2 mirrors vdot2 (RecastMeshDetail.cpp): 2D dot product on xz.
func detailVdot2(a, b []float32) float32 {
	return a[0]*b[0] + a[2]*b[2]
}

// detailVdistSq2 mirrors vdistSq2 (RecastMeshDetail.cpp).
func detailVdistSq2(p, q []float32) float32 {
	dx := q[0] - p[0]
	dy := q[2] - p[2]
	return dx*dx + dy*dy
}

// detailVdist2 mirrors vdist2 (RecastMeshDetail.cpp).
func detailVdist2(p, q []float32) float32 {
	return float32(math.Sqrt(float64(detailVdistSq2(p, q))))
}

// detailVcross2 mirrors vcross2 (RecastMeshDetail.cpp).
func detailVcross2(p1, p2, p3 []float32) float32 {
	u1 := p2[0] - p1[0]
	v1 := p2[2] - p1[2]
	u2 := p3[0] - p1[0]
	v2 := p3[2] - p1[2]
	return u1*v2 - v1*u2
}

// circumCircle mirrors circumCircle (RecastMeshDetail.cpp). Returns the
// circumcircle radius and whether the points were non-collinear.
func circumCircle(p1, p2, p3 []float32, c []float32) (float32, bool) {
	const eps = float32(1e-6)
	// Calculate the circle relative to p1, to avoid some precision issues.
	v1 := [3]float32{0, 0, 0}
	v2 := [3]float32{p2[0] - p1[0], p2[1] - p1[1], p2[2] - p1[2]}
	v3 := [3]float32{p3[0] - p1[0], p3[1] - p1[1], p3[2] - p1[2]}

	cp := detailVcross2(v1[:], v2[:], v3[:])
	if float32(math.Abs(float64(cp))) > eps {
		v1Sq := detailVdot2(v1[:], v1[:])
		v2Sq := detailVdot2(v2[:], v2[:])
		v3Sq := detailVdot2(v3[:], v3[:])
		c[0] = (v1Sq*(v2[2]-v3[2]) + v2Sq*(v3[2]-v1[2]) + v3Sq*(v1[2]-v2[2])) / (2 * cp)
		c[1] = 0
		c[2] = (v1Sq*(v3[0]-v2[0]) + v2Sq*(v1[0]-v3[0]) + v3Sq*(v2[0]-v1[0])) / (2 * cp)
		r := detailVdist2(c, v1[:])
		c[0] += p1[0]
		c[1] += p1[1]
		c[2] += p1[2]
		return r, true
	}

	c[0] = p1[0]
	c[1] = p1[1]
	c[2] = p1[2]
	return 0, false
}

// distPtTri mirrors distPtTri (RecastMeshDetail.cpp).
func distPtTri(p, a, b, c []float32) float32 {
	v0 := [3]float32{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	v1 := [3]float32{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v2 := [3]float32{p[0] - a[0], p[1] - a[1], p[2] - a[2]}

	dot00 := detailVdot2(v0[:], v0[:])
	dot01 := detailVdot2(v0[:], v1[:])
	dot02 := detailVdot2(v0[:], v2[:])
	dot11 := detailVdot2(v1[:], v1[:])
	dot12 := detailVdot2(v1[:], v2[:])

	// Compute barycentric coordinates
	invDenom := 1.0 / (dot00*dot11 - dot01*dot01)
	u := (dot11*dot02 - dot01*dot12) * invDenom
	v := (dot00*dot12 - dot01*dot02) * invDenom

	// If point lies inside the triangle, return interpolated y-coord.
	const eps = float32(1e-4)
	if u >= -eps && v >= -eps && (u+v) <= 1+eps {
		y := a[1] + v0[1]*u + v1[1]*v
		return float32(math.Abs(float64(y - p[1])))
	}
	return float32(math.MaxFloat32)
}

// distPtSegF mirrors distancePtSeg (RecastMeshDetail.cpp).
func distPtSegF(pt, p, q []float32) float32 {
	pqx := q[0] - p[0]
	pqy := q[1] - p[1]
	pqz := q[2] - p[2]
	dx := pt[0] - p[0]
	dy := pt[1] - p[1]
	dz := pt[2] - p[2]
	d := pqx*pqx + pqy*pqy + pqz*pqz
	t := pqx*dx + pqy*dy + pqz*dz
	if d > 0 {
		t /= d
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	dx = p[0] + t*pqx - pt[0]
	dy = p[1] + t*pqy - pt[1]
	dz = p[2] + t*pqz - pt[2]

	return dx*dx + dy*dy + dz*dz
}

// distPtSeg2dF mirrors distancePtSeg2d (RecastMeshDetail.cpp).
func distPtSeg2dF(pt, p, q []float32) float32 {
	pqx := q[0] - p[0]
	pqz := q[2] - p[2]
	dx := pt[0] - p[0]
	dz := pt[2] - p[2]
	d := pqx*pqx + pqz*pqz
	t := pqx*dx + pqz*dz
	if d > 0 {
		t /= d
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	dx = p[0] + t*pqx - pt[0]
	dz = p[2] + t*pqz - pt[2]

	return dx*dx + dz*dz
}

// distToTriMesh mirrors distToTriMesh (RecastMeshDetail.cpp).
func distToTriMesh(p []float32, verts []float32, tris []int, ntris int) float32 {
	dmin := float32(math.MaxFloat32)
	for i := 0; i < ntris; i++ {
		va := verts[tris[i*4+0]*3:]
		vb := verts[tris[i*4+1]*3:]
		vc := verts[tris[i*4+2]*3:]
		d := distPtTri(p, va, vb, vc)
		if d < dmin {
			dmin = d
		}
	}
	if dmin == float32(math.MaxFloat32) {
		return -1
	}
	return dmin
}

// distToPoly mirrors distToPoly (RecastMeshDetail.cpp).
func distToPoly(nvert int, verts []float32, p []float32) float32 {
	dmin := float32(math.MaxFloat32)
	c := false
	for i, j := 0, nvert-1; i < nvert; j, i = i, i+1 {
		vi := verts[i*3:]
		vj := verts[j*3:]
		if (vi[2] > p[2]) != (vj[2] > p[2]) &&
			p[0] < (vj[0]-vi[0])*(p[2]-vi[2])/(vj[2]-vi[2])+vi[0] {
			c = !c
		}
		dmin = minFloat(dmin, distPtSeg2dF(p, vj, vi))
	}
	if c {
		return -dmin
	}
	return dmin
}

// getHeight mirrors getHeight (RecastMeshDetail.cpp).
func getHeight(fx, fy, fz float32, ics, ch float32, radius int, hp *heightPatch) uint16 {
	ix := int(float32(math.Floor(float64(fx*ics + 0.01))))
	iz := int(float32(math.Floor(float64(fz*ics + 0.01))))
	ix = clampInt(ix-hp.xmin, 0, hp.width-1)
	iz = clampInt(iz-hp.ymin, 0, hp.height-1)
	h := hp.data[ix+iz*hp.width]
	if h == unsetHeight {
		// Special case when data might be bad.
		// Walk adjacent cells in a spiral up to 'radius', and look
		// for a pixel which has a valid height.
		x, z := 1, 0
		dx, dz := 1, 0
		maxSize := radius*2 + 1
		maxIter := maxSize*maxSize - 1

		nextRingIterStart := 8
		nextRingIters := 16

		dmin := float32(math.MaxFloat32)
		for i := 0; i < maxIter; i++ {
			nx := ix + x
			nz := iz + z

			if nx >= 0 && nz >= 0 && nx < hp.width && nz < hp.height {
				nh := hp.data[nx+nz*hp.width]
				if nh != unsetHeight {
					d := float32(math.Abs(float64(float32(nh)*ch - fy)))
					if d < dmin {
						h = nh
						dmin = d
					}
				}
			}

			// We are searching in a grid which looks approximately like this:
			//  __________
			// |2 ______ 2|
			// | |1 __ 1| |
			// | | |__| | |
			// | |______| |
			// |__________|
			// We want to find the best height as close to the center cell as possible. This means that
			// if we find a height in one of the neighbor cells to the center, we don't want to
			// expand further out than the 8 neighbors - we want to limit our search to the closest
			// of these "rings", but the best height in the ring.
			// For example, the center is just 1 cell. We checked that at the entrance to the function.
			// The next "ring" contains 8 cells (marked 1 above). Those are all the neighbors to the center cell.
			// The next one again contains 16 cells (marked 2). In general each ring has 8 additional cells, which
			// can be thought of as adding 2 cells around the "center" of each side when we expand the ring.
			// Here we detect if we are about to enter the next ring, and if we are and we have found
			// a height, we abort the search.
			if i+1 == nextRingIterStart {
				if h != unsetHeight {
					break
				}

				nextRingIterStart += nextRingIters
				nextRingIters += 8
			}

			if (x == z) || ((x < 0) && (x == -z)) || ((x > 0) && (x == 1-z)) {
				tmp := dx
				dx = -dz
				dz = tmp
			}
			x += dx
			z += dz
		}
	}
	return h
}

// edgeUndef and edgeHull mirror EdgeValues (RecastMeshDetail.cpp).
const (
	edgeUndef = -1
	edgeHull  = -2
)

// findEdge mirrors findEdge (RecastMeshDetail.cpp).
func findEdge(edges []int, nedges, s, t int) int {
	for i := 0; i < nedges; i++ {
		e := edges[i*4:]
		if (e[0] == s && e[1] == t) || (e[0] == t && e[1] == s) {
			return i
		}
	}
	return edgeUndef
}

// addEdge mirrors addEdge (RecastMeshDetail.cpp).
func addEdge(edges *intArray, nedges *int, maxEdges, s, t, l, r int) int {
	if *nedges >= maxEdges {
		return edgeUndef
	}

	// Add edge if not already in the triangulation.
	e := findEdge(*edges, *nedges, s, t)
	if e == edgeUndef {
		edge := (*edges)[*nedges*4:]
		edge[0] = s
		edge[1] = t
		edge[2] = l
		edge[3] = r
		n := *nedges
		*nedges++
		return n
	}
	return edgeUndef
}

// updateLeftFace mirrors updateLeftFace (RecastMeshDetail.cpp).
func updateLeftFace(e []int, s, t, f int) {
	if e[0] == s && e[1] == t && e[2] == edgeUndef {
		e[2] = f
	} else if e[1] == s && e[0] == t && e[3] == edgeUndef {
		e[3] = f
	}
}

// overlapSegSeg2d mirrors overlapSegSeg2d (RecastMeshDetail.cpp).
func overlapSegSeg2d(a, b, c, d []float32) int {
	a1 := detailVcross2(a, b, d)
	a2 := detailVcross2(a, b, c)
	if a1*a2 < 0.0 {
		a3 := detailVcross2(c, d, a)
		a4 := a3 + a2 - a1
		if a3*a4 < 0.0 {
			return 1
		}
	}
	return 0
}

// overlapEdges mirrors overlapEdges (RecastMeshDetail.cpp).
func overlapEdges(pts []float32, edges []int, nedges, s1, t1 int) bool {
	for i := 0; i < nedges; i++ {
		s0 := edges[i*4+0]
		t0 := edges[i*4+1]
		// Same or connected edges do not overlap.
		if s0 == s1 || s0 == t1 || t0 == s1 || t0 == t1 {
			continue
		}
		if overlapSegSeg2d(pts[s0*3:], pts[t0*3:], pts[s1*3:], pts[t1*3:]) != 0 {
			return true
		}
	}
	return false
}

// completeFacet mirrors completeFacet (RecastMeshDetail.cpp).
func completeFacet(pts []float32, npts int, edges *intArray, nedges *int, maxEdges int, nfaces *int, e int) {
	const eps = float32(1e-5)

	// Cache s and t.
	var s, t int
	if (*edges)[e*4+2] == edgeUndef {
		s = (*edges)[e*4+0]
		t = (*edges)[e*4+1]
	} else if (*edges)[e*4+3] == edgeUndef {
		s = (*edges)[e*4+1]
		t = (*edges)[e*4+0]
	} else {
		// Edge already completed.
		return
	}

	// Find best point on left of edge.
	pt := npts
	c := [3]float32{0, 0, 0}
	r := float32(-1)
	for u := 0; u < npts; u++ {
		if u == s || u == t {
			continue
		}
		if detailVcross2(pts[s*3:], pts[t*3:], pts[u*3:]) > eps {
			if r < 0 {
				// The circle is not updated yet, do it now.
				pt = u
				r, _ = circumCircle(pts[s*3:], pts[t*3:], pts[u*3:], c[:])
				continue
			}
			d := detailVdist2(c[:], pts[u*3:])
			tol := float32(0.001)
			if d > r*(1+tol) {
				// Outside current circumcircle, skip.
				continue
			} else if d < r*(1-tol) {
				// Inside safe circumcircle, update circle.
				pt = u
				r, _ = circumCircle(pts[s*3:], pts[t*3:], pts[u*3:], c[:])
			} else {
				// Inside epsilon circum circle, do extra tests to make sure the edge is valid.
				// s-u and t-u cannot overlap with s-pt nor t-pt if they exists.
				if overlapEdges(pts, *edges, *nedges, s, u) {
					continue
				}
				if overlapEdges(pts, *edges, *nedges, t, u) {
					continue
				}
				// Edge is valid.
				pt = u
				r, _ = circumCircle(pts[s*3:], pts[t*3:], pts[u*3:], c[:])
			}
		}
	}

	// Add new triangle or update edge info if s-t is on hull.
	if pt < npts {
		// Update face information of edge being completed.
		updateLeftFace((*edges)[e*4:], s, t, *nfaces)

		// Add new edge or update face info of old edge.
		e2 := findEdge(*edges, *nedges, pt, s)
		if e2 == edgeUndef {
			addEdge(edges, nedges, maxEdges, pt, s, *nfaces, edgeUndef)
		} else {
			updateLeftFace((*edges)[e2*4:], pt, s, *nfaces)
		}

		// Add new edge or update face info of old edge.
		e2 = findEdge(*edges, *nedges, t, pt)
		if e2 == edgeUndef {
			addEdge(edges, nedges, maxEdges, t, pt, *nfaces, edgeUndef)
		} else {
			updateLeftFace((*edges)[e2*4:], t, pt, *nfaces)
		}

		*nfaces++
	} else {
		updateLeftFace((*edges)[e*4:], s, t, edgeHull)
	}
}

// delaunayHull mirrors delaunayHull (RecastMeshDetail.cpp).
func delaunayHull(npts int, pts []float32, nhull int, hull []int, tris, edges *intArray) {
	nfaces := 0
	nedges := 0
	maxEdges := npts * 10
	edges.resize(maxEdges * 4)

	for i, j := 0, nhull-1; i < nhull; j, i = i, i+1 {
		addEdge(edges, &nedges, maxEdges, hull[j], hull[i], edgeHull, edgeUndef)
	}

	currentEdge := 0
	for currentEdge < nedges {
		if (*edges)[currentEdge*4+2] == edgeUndef {
			completeFacet(pts, npts, edges, &nedges, maxEdges, &nfaces, currentEdge)
		}
		if (*edges)[currentEdge*4+3] == edgeUndef {
			completeFacet(pts, npts, edges, &nedges, maxEdges, &nfaces, currentEdge)
		}
		currentEdge++
	}

	// Create tris
	tris.resize(nfaces * 4)
	for i := range *tris {
		(*tris)[i] = -1
	}

	for i := 0; i < nedges; i++ {
		e := (*edges)[i*4:]
		if e[3] >= 0 {
			// Left face
			t := (*tris)[e[3]*4:]
			if t[0] == -1 {
				t[0] = e[0]
				t[1] = e[1]
			} else if t[0] == e[1] {
				t[2] = e[0]
			} else if t[1] == e[0] {
				t[2] = e[1]
			}
		}
		if e[2] >= 0 {
			// Right
			t := (*tris)[e[2]*4:]
			if t[0] == -1 {
				t[0] = e[1]
				t[1] = e[0]
			} else if t[0] == e[0] {
				t[2] = e[1]
			} else if t[1] == e[1] {
				t[2] = e[0]
			}
		}
	}

	for i := 0; i < len(*tris)/4; i++ {
		t := (*tris)[i*4:]
		if t[0] == -1 || t[1] == -1 || t[2] == -1 {
			last := len(*tris) - 4
			t[0] = (*tris)[last+0]
			t[1] = (*tris)[last+1]
			t[2] = (*tris)[last+2]
			t[3] = (*tris)[last+3]
			tris.resize(len(*tris) - 4)
			i--
		}
	}
}

// polyMinExtent mirrors polyMinExtent (RecastMeshDetail.cpp).
func polyMinExtent(verts []float32, nverts int) float32 {
	minDist := float32(math.MaxFloat32)
	for i := 0; i < nverts; i++ {
		ni := (i + 1) % nverts
		p1 := verts[i*3:]
		p2 := verts[ni*3:]
		maxEdgeDist := float32(0)
		for j := 0; j < nverts; j++ {
			if j == i || j == ni {
				continue
			}
			d := distPtSeg2dF(verts[j*3:], p1, p2)
			maxEdgeDist = maxFloat(maxEdgeDist, d)
		}
		minDist = minFloat(minDist, maxEdgeDist)
	}
	return float32(math.Sqrt(float64(minDist)))
}

// detailPrev mirrors prev (RecastMeshDetail.cpp).
func detailPrev(i, n int) int {
	if i-1 >= 0 {
		return i - 1
	}
	return n - 1
}

// detailNext mirrors next (RecastMeshDetail.cpp).
func detailNext(i, n int) int {
	if i+1 < n {
		return i + 1
	}
	return 0
}

// triangulateHull mirrors triangulateHull (RecastMeshDetail.cpp).
func triangulateHull(verts []float32, nhull int, hull []int, nin int, tris *intArray) {
	start := 0
	left := 1
	right := nhull - 1

	// Start from an ear with shortest perimeter.
	// This tends to favor well formed triangles as starting point.
	dmin := float32(math.MaxFloat32)
	for i := 0; i < nhull; i++ {
		if hull[i] >= nin {
			continue // Ears are triangles with original vertices as middle vertex while others are actually line segments on edges
		}
		pi := detailPrev(i, nhull)
		ni := detailNext(i, nhull)
		pv := verts[hull[pi]*3:]
		cv := verts[hull[i]*3:]
		nv := verts[hull[ni]*3:]
		d := detailVdist2(pv, cv) + detailVdist2(cv, nv) + detailVdist2(nv, pv)
		if d < dmin {
			start = i
			left = ni
			right = pi
			dmin = d
		}
	}

	// Add first triangle
	tris.push(hull[start])
	tris.push(hull[left])
	tris.push(hull[right])
	tris.push(0)

	// Triangulate the polygon by moving left or right,
	// depending on which triangle has shorter perimeter.
	// This heuristic was chose emprically, since it seems
	// handle tesselated straight edges well.
	for detailNext(left, nhull) != right {
		// Check to see if se should advance left or right.
		nleft := detailNext(left, nhull)
		nright := detailPrev(right, nhull)

		cvleft := verts[hull[left]*3:]
		nvleft := verts[hull[nleft]*3:]
		cvright := verts[hull[right]*3:]
		nvright := verts[hull[nright]*3:]
		dleft := detailVdist2(cvleft, nvleft) + detailVdist2(nvleft, cvright)
		dright := detailVdist2(cvright, nvright) + detailVdist2(cvleft, nvright)

		if dleft < dright {
			tris.push(hull[left])
			tris.push(hull[nleft])
			tris.push(hull[right])
			tris.push(0)
			left = nleft
		} else {
			tris.push(hull[left])
			tris.push(hull[nright])
			tris.push(hull[right])
			tris.push(0)
			right = nright
		}
	}
}

// getJitterX mirrors getJitterX (RecastMeshDetail.cpp).
func getJitterX(i int) float32 {
	return float32((uint32(i)*0x8da6b343)&0xffff)/65535.0*2.0 - 1.0
}

// getJitterY mirrors getJitterY (RecastMeshDetail.cpp).
func getJitterY(i int) float32 {
	return float32((uint32(i)*0xd8163841)&0xffff)/65535.0*2.0 - 1.0
}

// buildPolyDetail mirrors buildPolyDetail (RecastMeshDetail.cpp).
func buildPolyDetail(in []float32, nin int, sampleDist, sampleMaxError float32,
	heightSearchRadius int, chf *CompactHeightfield, hp *heightPatch,
	verts []float32, nverts *int, tris, edges, samples *intArray) bool {
	const maxVerts = 127
	const maxTris = 255 // Max tris for delaunay is 2n-2-k (n=num verts, k=num hull verts).
	const maxVertsPerEdge = 32
	edge := make([]float32, (maxVertsPerEdge+1)*3)
	hull := make([]int, maxVerts)
	nhull := 0

	*nverts = nin

	for i := 0; i < nin; i++ {
		verts[i*3+0] = in[i*3+0]
		verts[i*3+1] = in[i*3+1]
		verts[i*3+2] = in[i*3+2]
	}

	edges.resize(0)
	tris.resize(0)

	cs := chf.Cs
	ics := 1.0 / cs

	// Calculate minimum extents of the polygon based on input data.
	minExtent := polyMinExtent(verts, *nverts)

	// Tessellate outlines.
	// This is done in separate pass in order to ensure
	// seamless height values across the ply boundaries.
	if sampleDist > 0 {
		for i, j := 0, nin-1; i < nin; j, i = i, i+1 {
			vj := in[j*3:]
			vi := in[i*3:]
			swapped := false
			// Make sure the segments are always handled in same order
			// using lexological sort or else there will be seams.
			if float32(math.Abs(float64(vj[0]-vi[0]))) < 1e-6 {
				if vj[2] > vi[2] {
					vj, vi = vi, vj
					swapped = true
				}
			} else {
				if vj[0] > vi[0] {
					vj, vi = vi, vj
					swapped = true
				}
			}
			// Create samples along the edge.
			dx := vi[0] - vj[0]
			dy := vi[1] - vj[1]
			dz := vi[2] - vj[2]
			d := float32(math.Sqrt(float64(dx*dx + dz*dz)))
			nn := 1 + int(float32(math.Floor(float64(d/sampleDist))))
			if nn >= maxVertsPerEdge {
				nn = maxVertsPerEdge - 1
			}
			if *nverts+nn >= maxVerts {
				nn = maxVerts - 1 - *nverts
			}

			for k := 0; k <= nn; k++ {
				u := float32(k) / float32(nn)
				pos := edge[k*3:]
				pos[0] = vj[0] + dx*u
				pos[1] = vj[1] + dy*u
				pos[2] = vj[2] + dz*u
				pos[1] = float32(getHeight(pos[0], pos[1], pos[2], ics, chf.Ch, heightSearchRadius, hp)) * chf.Ch
			}
			// Simplify samples.
			idx := make([]int, maxVertsPerEdge+1)
			idx[0] = 0
			idx[1] = nn
			nidx := 2
			for k := 0; k < nidx-1; {
				a := idx[k]
				b := idx[k+1]
				va := edge[a*3:]
				vb := edge[b*3:]
				// Find maximum deviation along the segment.
				maxd := float32(0)
				maxi := -1
				for m := a + 1; m < b; m++ {
					dev := distPtSegF(edge[m*3:], va, vb)
					if dev > maxd {
						maxd = dev
						maxi = m
					}
				}
				// If the max deviation is larger than accepted error,
				// add new point, else continue to next segment.
				if maxi != -1 && maxd > sampleMaxError*sampleMaxError {
					for m := nidx; m > k; m-- {
						idx[m] = idx[m-1]
					}
					idx[k+1] = maxi
					nidx++
				} else {
					k++
				}
			}

			hull[nhull] = j
			nhull++
			// Add new vertices.
			if swapped {
				for k := nidx - 2; k > 0; k-- {
					verts[*nverts*3+0] = edge[idx[k]*3+0]
					verts[*nverts*3+1] = edge[idx[k]*3+1]
					verts[*nverts*3+2] = edge[idx[k]*3+2]
					hull[nhull] = *nverts
					nhull++
					*nverts++
				}
			} else {
				for k := 1; k < nidx-1; k++ {
					verts[*nverts*3+0] = edge[idx[k]*3+0]
					verts[*nverts*3+1] = edge[idx[k]*3+1]
					verts[*nverts*3+2] = edge[idx[k]*3+2]
					hull[nhull] = *nverts
					nhull++
					*nverts++
				}
			}
		}
	}

	// If the polygon minimum extent is small (sliver or small triangle), do not try to add internal points.
	if minExtent < sampleDist*2 {
		triangulateHull(verts, nhull, hull, nin, tris)
		return true
	}

	// Tessellate the base mesh.
	// We're using the triangulateHull instead of delaunayHull as it tends to
	// create a bit better triangulation for long thin triangles when there
	// are no internal points.
	triangulateHull(verts, nhull, hull, nin, tris)

	if len(*tris) == 0 {
		// Could not triangulate the poly, make sure there is some valid data there.
		return true
	}

	if sampleDist > 0 {
		// Create sample locations in a grid.
		bmin := [3]float32{in[0], in[1], in[2]}
		bmax := bmin
		for i := 1; i < nin; i++ {
			v := [3]float32{in[i*3+0], in[i*3+1], in[i*3+2]}
			bmin = vMin(bmin, v)
			bmax = vMax(bmax, v)
		}
		x0 := int(float32(math.Floor(float64(bmin[0] / sampleDist))))
		x1 := int(float32(math.Ceil(float64(bmax[0] / sampleDist))))
		z0 := int(float32(math.Floor(float64(bmin[2] / sampleDist))))
		z1 := int(float32(math.Ceil(float64(bmax[2] / sampleDist))))
		samples.resize(0)
		for z := z0; z < z1; z++ {
			for x := x0; x < x1; x++ {
				pt := [3]float32{
					float32(x) * sampleDist,
					(bmax[1] + bmin[1]) * 0.5,
					float32(z) * sampleDist,
				}
				// Make sure the samples are not too close to the edges.
				if distToPoly(nin, in, pt[:]) > -sampleDist/2 {
					continue
				}
				samples.push(x)
				samples.push(int(getHeight(pt[0], pt[1], pt[2], ics, chf.Ch, heightSearchRadius, hp)))
				samples.push(z)
				samples.push(0) // Not added
			}
		}

		// Add the samples starting from the one that has the most
		// error. The procedure stops when all samples are added
		// or when the max error is within treshold.
		nsamples := len(*samples) / 4
		for iter := 0; iter < nsamples; iter++ {
			if *nverts >= maxVerts {
				break
			}

			// Find sample with most error.
			bestpt := [3]float32{0, 0, 0}
			bestd := float32(0)
			besti := -1
			for i := 0; i < nsamples; i++ {
				s := (*samples)[i*4:]
				if s[3] != 0 {
					continue // skip added.
				}
				// The sample location is jittered to get rid of some bad triangulations
				// which are cause by symmetrical data from the grid structure.
				pt := [3]float32{
					float32(s[0])*sampleDist + getJitterX(i)*cs*0.1,
					float32(s[1]) * chf.Ch,
					float32(s[2])*sampleDist + getJitterY(i)*cs*0.1,
				}
				d := distToTriMesh(pt[:], verts, *tris, len(*tris)/4)
				if d < 0 {
					continue // did not hit the mesh.
				}
				if d > bestd {
					bestd = d
					besti = i
					bestpt = pt
				}
			}
			// If the max error is within accepted threshold, stop tesselating.
			if bestd <= sampleMaxError || besti == -1 {
				break
			}
			// Mark sample as added.
			(*samples)[besti*4+3] = 1
			// Add the new sample point.
			verts[*nverts*3+0] = bestpt[0]
			verts[*nverts*3+1] = bestpt[1]
			verts[*nverts*3+2] = bestpt[2]
			*nverts++

			// Create new triangulation.
			// TODO: Incremental add instead of full rebuild.
			edges.resize(0)
			tris.resize(0)
			delaunayHull(*nverts, verts, nhull, hull, tris, edges)
		}
	}

	ntris := len(*tris) / 4
	if ntris > maxTris {
		tris.resize(maxTris * 4)
	}

	return true
}

// dirForOffset mirrors rcGetDirForOffset (Recast.h).
func dirForOffset(x, y int) int {
	dirs := [5]int{3, 0, -1, 2, 1}
	return dirs[((y+1)<<1)+x]
}

// seedArrayWithPolyCenter mirrors seedArrayWithPolyCenter (RecastMeshDetail.cpp).
func seedArrayWithPolyCenter(chf *CompactHeightfield, poly []uint16, npoly int,
	verts []uint16, bs int, hp *heightPatch, array *intArray) {
	// Note: Reads to the compact heightfield are offset by border size (bs)
	// since border size offset is already removed from the polymesh vertices.

	offset := [9 * 2]int{
		0, 0, -1, -1, 0, -1, 1, -1, 1, 0, 1, 1, 0, 1, -1, 1, -1, 0,
	}

	// Find cell closest to a poly vertex
	startCellX, startCellY, startSpanIndex := 0, 0, -1
	dmin := int(unsetHeight)
	for j := 0; j < npoly && dmin > 0; j++ {
		for k := 0; k < 9 && dmin > 0; k++ {
			ax := int(verts[poly[j]*3+0]) + offset[k*2+0]
			ay := int(verts[poly[j]*3+1])
			az := int(verts[poly[j]*3+2]) + offset[k*2+1]
			if ax < hp.xmin || ax >= hp.xmin+hp.width ||
				az < hp.ymin || az >= hp.ymin+hp.height {
				continue
			}

			c := chf.Cells[(ax+bs)+(az+bs)*chf.Width]
			for i := int(c.Index); i < int(c.Index+c.Count) && dmin > 0; i++ {
				s := chf.Spans[i]
				d := absInt(ay - int(s.Y))
				if d < dmin {
					startCellX = ax
					startCellY = az
					startSpanIndex = i
					dmin = d
				}
			}
		}
	}

	// Find center of the polygon
	pcx, pcy := 0, 0
	for j := 0; j < npoly; j++ {
		pcx += int(verts[poly[j]*3+0])
		pcy += int(verts[poly[j]*3+2])
	}
	pcx /= npoly
	pcy /= npoly

	// Use seeds array as a stack for DFS
	array.resize(0)
	array.push(startCellX)
	array.push(startCellY)
	array.push(startSpanIndex)

	dirs := [4]int{0, 1, 2, 3}
	for i := range hp.data {
		hp.data[i] = 0
	}
	// DFS to move to the center. Note that we need a DFS here and can not just move
	// directly towards the center without recording intermediate nodes, even though the polygons
	// are convex. In very rare we can get stuck due to contour simplification if we do not
	// record nodes.
	cx, cy, ci := -1, -1, -1
	for {
		if len(*array) < 3 {
			break
		}

		ci = array.pop()
		cy = array.pop()
		cx = array.pop()

		if cx == pcx && cy == pcy {
			break
		}

		// If we are already at the correct X-position, prefer direction
		// directly towards the center in the Y-axis; otherwise prefer
		// direction in the X-axis
		var directDir int
		if cx == pcx {
			if pcy > cy {
				directDir = dirForOffset(0, 1)
			} else {
				directDir = dirForOffset(0, -1)
			}
		} else {
			if pcx > cx {
				directDir = dirForOffset(1, 0)
			} else {
				directDir = dirForOffset(-1, 0)
			}
		}

		// Push the direct dir last so we start with this on next iteration
		dirs[directDir], dirs[3] = dirs[3], dirs[directDir]

		cs := chf.Spans[ci]
		for i := 0; i < 4; i++ {
			dir := dirs[i]
			if GetCon(&cs, dir) == NotConnected {
				continue
			}

			newX := cx + DirOffsetX(dir)
			newY := cy + DirOffsetY(dir)

			hpx := newX - hp.xmin
			hpy := newY - hp.ymin
			if hpx < 0 || hpx >= hp.width || hpy < 0 || hpy >= hp.height {
				continue
			}

			if hp.data[hpx+hpy*hp.width] != 0 {
				continue
			}

			hp.data[hpx+hpy*hp.width] = 1
			array.push(newX)
			array.push(newY)
			array.push(int(chf.Cells[(newX+bs)+(newY+bs)*chf.Width].Index) + GetCon(&cs, dir))
		}

		dirs[directDir], dirs[3] = dirs[3], dirs[directDir]
	}

	array.resize(0)
	// getHeightData seeds are given in coordinates with borders
	array.push(cx + bs)
	array.push(cy + bs)
	array.push(ci)

	for i := range hp.data {
		hp.data[i] = 0xffff
	}
	cs := chf.Spans[ci]
	hp.data[cx-hp.xmin+(cy-hp.ymin)*hp.width] = cs.Y
}

// push3 mirrors push3 (RecastMeshDetail.cpp).
func push3(queue *intArray, v1, v2, v3 int) {
	queue.resize(len(*queue) + 3)
	(*queue)[len(*queue)-3] = v1
	(*queue)[len(*queue)-2] = v2
	(*queue)[len(*queue)-1] = v3
}

// getHeightData mirrors getHeightData (RecastMeshDetail.cpp).
func getHeightData(chf *CompactHeightfield, poly []uint16, npoly int,
	verts []uint16, bs int, hp *heightPatch, queue *intArray, region int) {
	// Note: Reads to the compact heightfield are offset by border size (bs)
	// since border size offset is already removed from the polymesh vertices.

	queue.resize(0)
	// Set all heights to RC_UNSET_HEIGHT.
	for i := range hp.data {
		hp.data[i] = 0xffff
	}

	empty := true

	// We cannot sample from this poly if it was created from polys
	// of different regions. If it was then it could potentially be overlapping
	// with polys of that region and the heights sampled here could be wrong.
	if region != multipleRegs {
		// Copy the height from the same region, and mark region borders
		// as seed points to fill the rest.
		for hy := 0; hy < hp.height; hy++ {
			y := hp.ymin + hy + bs
			for hx := 0; hx < hp.width; hx++ {
				x := hp.xmin + hx + bs
				c := chf.Cells[x+y*chf.Width]
				for i := int(c.Index); i < int(c.Index+c.Count); i++ {
					s := chf.Spans[i]
					if int(s.Reg) == region {
						// Store height
						hp.data[hx+hy*hp.width] = s.Y
						empty = false

						// If any of the neighbours is not in same region,
						// add the current location as flood fill start
						border := false
						for dir := 0; dir < 4; dir++ {
							if GetCon(&s, dir) != NotConnected {
								ax := x + DirOffsetX(dir)
								ay := y + DirOffsetY(dir)
								ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dir)
								as := chf.Spans[ai]
								if int(as.Reg) != region {
									border = true
									break
								}
							}
						}
						if border {
							push3(queue, x, y, i)
						}
						break
					}
				}
			}
		}
	}

	// if the polygon does not contain any points from the current region (rare, but happens)
	// or if it could potentially be overlapping polygons of the same region,
	// then use the center as the seed point.
	if empty {
		seedArrayWithPolyCenter(chf, poly, npoly, verts, bs, hp, queue)
	}

	const retractSize = 256
	head := 0

	// We assume the seed is centered in the polygon, so a BFS to collect
	// height data will ensure we do not move onto overlapping polygons and
	// sample wrong heights.
	for head*3 < len(*queue) {
		cx := (*queue)[head*3+0]
		cy := (*queue)[head*3+1]
		ci := (*queue)[head*3+2]
		head++
		if head >= retractSize {
			head = 0
			if len(*queue) > retractSize*3 {
				copy(*queue, (*queue)[retractSize*3:])
			}
			queue.resize(len(*queue) - retractSize*3)
		}

		cs := chf.Spans[ci]
		for dir := 0; dir < 4; dir++ {
			if GetCon(&cs, dir) == NotConnected {
				continue
			}

			ax := cx + DirOffsetX(dir)
			ay := cy + DirOffsetY(dir)
			hx := ax - hp.xmin - bs
			hy := ay - hp.ymin - bs

			if uint(hx) >= uint(hp.width) || uint(hy) >= uint(hp.height) {
				continue
			}

			if hp.data[hx+hy*hp.width] != unsetHeight {
				continue
			}

			ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&cs, dir)
			as := chf.Spans[ai]

			hp.data[hx+hy*hp.width] = as.Y

			push3(queue, ax, ay, ai)
		}
	}
}

// getEdgeFlags mirrors getEdgeFlags (RecastMeshDetail.cpp).
// The flag returned by this function matches dtDetailTriEdgeFlags in Detour.
// Figure out if edge (va,vb) is part of the polygon boundary.
func getEdgeFlags(va, vb, vpoly []float32, npoly int) uint8 {
	thrSqr := float32(0.001) * float32(0.001)
	for i, j := 0, npoly-1; i < npoly; j, i = i, i+1 {
		if distPtSeg2dF(va, vpoly[j*3:], vpoly[i*3:]) < thrSqr &&
			distPtSeg2dF(vb, vpoly[j*3:], vpoly[i*3:]) < thrSqr {
			return 1
		}
	}
	return 0
}

// getTriFlags mirrors getTriFlags (RecastMeshDetail.cpp).
func getTriFlags(va, vb, vc, vpoly []float32, npoly int) uint8 {
	flags := uint8(0)
	flags |= getEdgeFlags(va, vb, vpoly, npoly) << 0
	flags |= getEdgeFlags(vb, vc, vpoly, npoly) << 2
	flags |= getEdgeFlags(vc, va, vpoly, npoly) << 4
	return flags
}

// BuildPolyMeshDetail mirrors rcBuildPolyMeshDetail (RecastMeshDetail.cpp).
func BuildPolyMeshDetail(mesh *PolyMesh, chf *CompactHeightfield, sampleDist, sampleMaxError float32, dmesh *PolyMeshDetail) bool {
	if mesh.Nverts == 0 || mesh.Npolys == 0 {
		return true
	}

	nvp := mesh.Nvp
	cs := mesh.Cs
	ch := mesh.Ch
	orig := mesh.Bmin
	borderSize := mesh.BorderSize
	heightSearchRadius := maxInt(1, int(float32(math.Ceil(float64(mesh.MaxEdgeError)))))

	edges := make(intArray, 0, 64)
	tris := make(intArray, 0, 512)
	arr := make(intArray, 0, 512)
	samples := make(intArray, 0, 512)
	verts := make([]float32, 256*3)
	hp := heightPatch{}
	nPolyVerts := 0
	maxhw, maxhh := 0, 0

	bounds := make([]int, mesh.Npolys*4)
	poly := make([]float32, nvp*3)

	// Find max size for a polygon area.
	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]
		xmin := chf.Width
		xmax := 0
		ymin := chf.Height
		ymax := 0
		for j := 0; j < nvp; j++ {
			if p[j] == MeshNullIdx {
				break
			}
			v := mesh.Verts[p[j]*3:]
			xmin = minInt(xmin, int(v[0]))
			xmax = maxInt(xmax, int(v[0]))
			ymin = minInt(ymin, int(v[2]))
			ymax = maxInt(ymax, int(v[2]))
			nPolyVerts++
		}
		xmin = maxInt(0, xmin-1)
		xmax = minInt(chf.Width, xmax+1)
		ymin = maxInt(0, ymin-1)
		ymax = minInt(chf.Height, ymax+1)
		bounds[i*4+0] = xmin
		bounds[i*4+1] = xmax
		bounds[i*4+2] = ymin
		bounds[i*4+3] = ymax
		if xmin >= xmax || ymin >= ymax {
			continue
		}
		maxhw = maxInt(maxhw, xmax-xmin)
		maxhh = maxInt(maxhh, ymax-ymin)
	}

	hp.data = make([]uint16, maxhw*maxhh)

	dmesh.Nmeshes = mesh.Npolys
	dmesh.Nverts = 0
	dmesh.Ntris = 0
	dmesh.Meshes = make([]uint32, dmesh.Nmeshes*4)

	vcap := nPolyVerts + nPolyVerts/2
	tcap := vcap * 2

	dmesh.Nverts = 0
	dmesh.Verts = make([]float32, vcap*3)
	dmesh.Ntris = 0
	dmesh.Tris = make([]uint8, tcap*4)

	for i := 0; i < mesh.Npolys; i++ {
		p := mesh.Polys[i*nvp*2:]

		// Store polygon vertices for processing.
		npoly := 0
		for j := 0; j < nvp; j++ {
			if p[j] == MeshNullIdx {
				break
			}
			v := mesh.Verts[p[j]*3:]
			poly[j*3+0] = float32(v[0]) * cs
			poly[j*3+1] = float32(v[1]) * ch
			poly[j*3+2] = float32(v[2]) * cs
			npoly++
		}

		// Get the height data from the area of the polygon.
		hp.xmin = bounds[i*4+0]
		hp.ymin = bounds[i*4+2]
		hp.width = bounds[i*4+1] - bounds[i*4+0]
		hp.height = bounds[i*4+3] - bounds[i*4+2]
		getHeightData(chf, p, npoly, mesh.Verts, borderSize, &hp, &arr, int(mesh.Regs[i]))

		// Build detail mesh.
		nverts := 0
		if !buildPolyDetail(poly, npoly,
			sampleDist, sampleMaxError,
			heightSearchRadius, chf, &hp,
			verts, &nverts, &tris,
			&edges, &samples) {
			return false
		}

		// Move detail verts to world space.
		for j := 0; j < nverts; j++ {
			verts[j*3+0] += orig[0]
			verts[j*3+1] += orig[1] + chf.Ch // Is this offset necessary?
			verts[j*3+2] += orig[2]
		}
		// Offset poly too, will be used to flag checking.
		for j := 0; j < npoly; j++ {
			poly[j*3+0] += orig[0]
			poly[j*3+1] += orig[1]
			poly[j*3+2] += orig[2]
		}

		// Store detail submesh.
		ntris := len(tris) / 4

		dmesh.Meshes[i*4+0] = uint32(dmesh.Nverts)
		dmesh.Meshes[i*4+1] = uint32(nverts)
		dmesh.Meshes[i*4+2] = uint32(dmesh.Ntris)
		dmesh.Meshes[i*4+3] = uint32(ntris)

		// Store vertices, allocate more memory if necessary.
		if dmesh.Nverts+nverts > vcap {
			for dmesh.Nverts+nverts > vcap {
				vcap += 256
			}

			newv := make([]float32, vcap*3)
			copy(newv, dmesh.Verts[:dmesh.Nverts*3])
			dmesh.Verts = newv
		}
		for j := 0; j < nverts; j++ {
			dmesh.Verts[dmesh.Nverts*3+0] = verts[j*3+0]
			dmesh.Verts[dmesh.Nverts*3+1] = verts[j*3+1]
			dmesh.Verts[dmesh.Nverts*3+2] = verts[j*3+2]
			dmesh.Nverts++
		}

		// Store triangles, allocate more memory if necessary.
		if dmesh.Ntris+ntris > tcap {
			for dmesh.Ntris+ntris > tcap {
				tcap += 256
			}
			newt := make([]uint8, tcap*4)
			copy(newt, dmesh.Tris[:dmesh.Ntris*4])
			dmesh.Tris = newt
		}
		for j := 0; j < ntris; j++ {
			t := tris[j*4:]
			dmesh.Tris[dmesh.Ntris*4+0] = uint8(t[0])
			dmesh.Tris[dmesh.Ntris*4+1] = uint8(t[1])
			dmesh.Tris[dmesh.Ntris*4+2] = uint8(t[2])
			dmesh.Tris[dmesh.Ntris*4+3] = getTriFlags(verts[t[0]*3:], verts[t[1]*3:], verts[t[2]*3:], poly, npoly)
			dmesh.Ntris++
		}
	}

	// Trim the over-allocated tails; counts are authoritative.
	dmesh.Verts = dmesh.Verts[:dmesh.Nverts*3]
	dmesh.Tris = dmesh.Tris[:dmesh.Ntris*4]

	return true
}

// MergePolyMeshDetails mirrors rcMergePolyMeshDetails (RecastMeshDetail.cpp):
// it concatenates the sub-meshes, rebasing each mesh's vert and tri bases
// by the running totals. Nil entries are skipped. MapBuilder uses it in
// buildMoveMapTile to merge every sub-tile detail mesh before
// dtCreateNavMeshData. Counts are clamped to the slices
// actually present, so a malformed detail mesh degrades instead of
// indexing out of bounds.
func MergePolyMeshDetails(details []*PolyMeshDetail, out *PolyMeshDetail) bool {
	if out == nil {
		return false
	}
	maxVerts, maxTris, maxMeshes := 0, 0, 0
	for _, dm := range details {
		if dm == nil {
			continue
		}
		maxVerts += dm.Nverts
		maxTris += dm.Ntris
		maxMeshes += dm.Nmeshes
	}
	out.Nmeshes = 0
	out.Meshes = make([]uint32, maxMeshes*4)
	out.Ntris = 0
	out.Tris = make([]uint8, maxTris*4)
	out.Nverts = 0
	out.Verts = make([]float32, maxVerts*3)
	for _, dm := range details {
		if dm == nil {
			continue
		}
		nmeshes := dm.Nmeshes
		if nmeshes > len(dm.Meshes)/4 {
			nmeshes = len(dm.Meshes) / 4
		}
		for j := 0; j < nmeshes; j++ {
			src := dm.Meshes[j*4:]
			dst := out.Meshes[out.Nmeshes*4:]
			dst[0] = uint32(out.Nverts) + src[0]
			dst[1] = src[1]
			dst[2] = uint32(out.Ntris) + src[2]
			dst[3] = src[3]
			out.Nmeshes++
		}
		nverts := dm.Nverts
		if nverts*3 > len(dm.Verts) {
			nverts = len(dm.Verts) / 3
		}
		for k := 0; k < nverts; k++ {
			copy(out.Verts[out.Nverts*3:], dm.Verts[k*3:k*3+3])
			out.Nverts++
		}
		ntris := dm.Ntris
		if ntris*4 > len(dm.Tris) {
			ntris = len(dm.Tris) / 4
		}
		for k := 0; k < ntris; k++ {
			copy(out.Tris[out.Ntris*4:], dm.Tris[k*4:k*4+4])
			out.Ntris++
		}
	}
	return true
}

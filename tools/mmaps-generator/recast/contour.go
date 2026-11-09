// Contour building mirrors dep/recastnavigation/Recast/Source/RecastContour.cpp
// (rcBuildContours) and the rcContour / rcContourSet structs in
// dep/recastnavigation/Recast/Include/Recast.h.
package recast

import "sort"

// Contour build flags (Recast.h rcBuildContoursFlags).
const (
	ContourTessWallEdges = 0x01
	ContourTessAreaEdges = 0x02
)

// Contour vertex flag bits (Recast.h).
const (
	borderReg      = 0x8000
	borderVertex   = 0x10000
	areaBorder     = 0x20000
	contourRegMask = 0xffff
)

// Contour mirrors rcContour (Recast.h): verts/rverts hold 4 ints per vertex
// (x, y, z, region/connection flags).
type Contour struct {
	Verts   []int32
	Nverts  int
	Rverts  []int32
	Nrverts int
	Reg     uint16
	Area    uint8
}

// ContourSet mirrors rcContourSet (Recast.h).
type ContourSet struct {
	Conts      []Contour
	Nconts     int
	Bmin, Bmax [3]float32
	Cs, Ch     float32
	Width      int
	Height     int
	BorderSize int
	MaxError   float32
}

// getCornerHeight mirrors getCornerHeight (RecastContour.cpp).
func getCornerHeight(x, y, i, dir int, chf *CompactHeightfield, isBorderVertex *bool) int {
	s := chf.Spans[i]
	ch := int(s.Y)
	dirp := (dir + 1) & 0x3

	var regs [4]uint32

	// Combine region and area codes in order to prevent
	// border vertices which are in between two areas to be removed.
	regs[0] = uint32(chf.Spans[i].Reg) | (uint32(chf.Areas[i]) << 16)

	if GetCon(&s, dir) != NotConnected {
		ax := x + DirOffsetX(dir)
		ay := y + DirOffsetY(dir)
		ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dir)
		as := chf.Spans[ai]
		ch = maxInt(ch, int(as.Y))
		regs[1] = uint32(chf.Spans[ai].Reg) | (uint32(chf.Areas[ai]) << 16)
		if GetCon(&as, dirp) != NotConnected {
			ax2 := ax + DirOffsetX(dirp)
			ay2 := ay + DirOffsetY(dirp)
			ai2 := int(chf.Cells[ax2+ay2*chf.Width].Index) + GetCon(&as, dirp)
			as2 := chf.Spans[ai2]
			ch = maxInt(ch, int(as2.Y))
			regs[2] = uint32(chf.Spans[ai2].Reg) | (uint32(chf.Areas[ai2]) << 16)
		}
	}
	if GetCon(&s, dirp) != NotConnected {
		ax := x + DirOffsetX(dirp)
		ay := y + DirOffsetY(dirp)
		ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dirp)
		as := chf.Spans[ai]
		ch = maxInt(ch, int(as.Y))
		regs[3] = uint32(chf.Spans[ai].Reg) | (uint32(chf.Areas[ai]) << 16)
		if GetCon(&as, dir) != NotConnected {
			ax2 := ax + DirOffsetX(dir)
			ay2 := ay + DirOffsetY(dir)
			ai2 := int(chf.Cells[ax2+ay2*chf.Width].Index) + GetCon(&as, dir)
			as2 := chf.Spans[ai2]
			ch = maxInt(ch, int(as2.Y))
			regs[2] = uint32(chf.Spans[ai2].Reg) | (uint32(chf.Areas[ai2]) << 16)
		}
	}

	// Check if the vertex is special edge vertex, these vertices will be removed later.
	for j := 0; j < 4; j++ {
		a := j
		b := (j + 1) & 0x3
		c := (j + 2) & 0x3
		d := (j + 3) & 0x3

		// The vertex is a border vertex there are two same exterior cells in a row,
		// followed by two interior cells and none of the regions are out of bounds.
		twoSameExts := (regs[a]&regs[b]&borderReg) != 0 && regs[a] == regs[b]
		twoInts := ((regs[c] | regs[d]) & borderReg) == 0
		intsSameArea := (regs[c] >> 16) == (regs[d] >> 16)
		noZeros := regs[a] != 0 && regs[b] != 0 && regs[c] != 0 && regs[d] != 0
		if twoSameExts && twoInts && intsSameArea && noZeros {
			*isBorderVertex = true
			break
		}
	}

	return ch
}

// walkContourRaw mirrors walkContour (RecastContour.cpp). It is named
// walkContourRaw because RecastRegion.cpp's walkContour already occupies
// the walkContour name in this package.
func walkContourRaw(x, y, i int, chf *CompactHeightfield, flags []uint8, points *[]int32) {
	// Choose the first non-connected edge
	dir := 0
	for (flags[i] & (1 << uint(dir))) == 0 {
		dir++
	}

	startDir := dir
	starti := i

	area := chf.Areas[i]

	iter := 0
	for iter++; iter < 40000; {
		if flags[i]&(1<<uint(dir)) != 0 {
			// Choose the edge corner
			isBorderVertex := false
			isAreaBorder := false
			px := x
			py := getCornerHeight(x, y, i, dir, chf, &isBorderVertex)
			pz := y
			switch dir {
			case 0:
				pz++
			case 1:
				px++
				pz++
			case 2:
				px++
			}
			r := 0
			s := chf.Spans[i]
			if GetCon(&s, dir) != NotConnected {
				ax := x + DirOffsetX(dir)
				ay := y + DirOffsetY(dir)
				ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dir)
				r = int(chf.Spans[ai].Reg)
				if area != chf.Areas[ai] {
					isAreaBorder = true
				}
			}
			if isBorderVertex {
				r |= borderVertex
			}
			if isAreaBorder {
				r |= areaBorder
			}
			*points = append(*points, int32(px), int32(py), int32(pz), int32(r))

			flags[i] &^= (1 << uint(dir)) // Remove visited edges
			dir = (dir + 1) & 0x3         // Rotate CW
		} else {
			ni := -1
			nx := x + DirOffsetX(dir)
			ny := y + DirOffsetY(dir)
			s := chf.Spans[i]
			if GetCon(&s, dir) != NotConnected {
				nc := chf.Cells[nx+ny*chf.Width]
				ni = int(nc.Index) + GetCon(&s, dir)
			}
			if ni == -1 {
				// Should not happen.
				return
			}
			x = nx
			y = ny
			i = ni
			dir = (dir + 3) & 0x3 // Rotate CCW
		}

		if starti == i && startDir == dir {
			break
		}
	}
}

// distancePtSeg mirrors distancePtSeg (RecastContour.cpp).
func distancePtSeg(x, z, px, pz, qx, qz int) float32 {
	pqx := float32(qx - px)
	pqz := float32(qz - pz)
	dx := float32(x - px)
	dz := float32(z - pz)
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

	dx = float32(px) + t*pqx - float32(x)
	dz = float32(pz) + t*pqz - float32(z)

	return dx*dx + dz*dz
}

// insertPoint inserts the raw point maxi into simplified after index i,
// mirroring the resize-and-shift idiom in simplifyContour (RecastContour.cpp).
func insertSimplifiedPoint(simplified []int32, i, maxi int, points []int32) []int32 {
	simplified = append(simplified, 0, 0, 0, 0)
	n := len(simplified) / 4
	for j := n - 1; j > i; j-- {
		simplified[j*4+0] = simplified[(j-1)*4+0]
		simplified[j*4+1] = simplified[(j-1)*4+1]
		simplified[j*4+2] = simplified[(j-1)*4+2]
		simplified[j*4+3] = simplified[(j-1)*4+3]
	}
	simplified[(i+1)*4+0] = points[maxi*4+0]
	simplified[(i+1)*4+1] = points[maxi*4+1]
	simplified[(i+1)*4+2] = points[maxi*4+2]
	simplified[(i+1)*4+3] = int32(maxi)
	return simplified
}

// simplifyContour mirrors simplifyContour (RecastContour.cpp).
func simplifyContour(points, simplified []int32, maxError float32, maxEdgeLen, buildFlags int) []int32 {
	// Add initial points.
	hasConnections := false
	for i := 0; i < len(points); i += 4 {
		if (points[i+3] & contourRegMask) != 0 {
			hasConnections = true
			break
		}
	}

	if hasConnections {
		// The contour has some portals to other regions.
		// Add a new point to every location where the region changes.
		for i, ni := 0, len(points)/4; i < ni; i++ {
			ii := (i + 1) % ni
			differentRegs := (points[i*4+3] & contourRegMask) != (points[ii*4+3] & contourRegMask)
			areaBorders := (points[i*4+3] & areaBorder) != (points[ii*4+3] & areaBorder)
			if differentRegs || areaBorders {
				simplified = append(simplified, points[i*4+0], points[i*4+1], points[i*4+2], int32(i))
			}
		}
	}

	if len(simplified) == 0 {
		// If there is no connections at all,
		// create some initial points for the simplification process.
		// Find lower-left and upper-right vertices of the contour.
		llx := points[0]
		lly := points[1]
		llz := points[2]
		lli := 0
		urx := points[0]
		ury := points[1]
		urz := points[2]
		uri := 0
		for i := 0; i < len(points); i += 4 {
			x := points[i+0]
			y := points[i+1]
			z := points[i+2]
			if x < llx || (x == llx && z < llz) {
				llx = x
				lly = y
				llz = z
				lli = i / 4
			}
			if x > urx || (x == urx && z > urz) {
				urx = x
				ury = y
				urz = z
				uri = i / 4
			}
		}
		simplified = append(simplified, llx, lly, llz, int32(lli))
		simplified = append(simplified, urx, ury, urz, int32(uri))
	}

	// Add points until all raw points are within
	// error tolerance to the simplified shape.
	pn := len(points) / 4
	for i := 0; i < len(simplified)/4; {
		ii := (i + 1) % (len(simplified) / 4)

		ax := simplified[i*4+0]
		az := simplified[i*4+2]
		ai := simplified[i*4+3]

		bx := simplified[ii*4+0]
		bz := simplified[ii*4+2]
		bi := simplified[ii*4+3]

		// Find maximum deviation from the segment.
		var maxd float32
		maxi := -1
		var ci, cinc, endi int

		// Traverse the segment in lexilogical order so that the
		// max deviation is calculated similarly when traversing
		// opposite segments.
		if bx > ax || (bx == ax && bz > az) {
			cinc = 1
			ci = (int(ai) + cinc) % pn
			endi = int(bi)
		} else {
			cinc = pn - 1
			ci = (int(bi) + cinc) % pn
			endi = int(ai)
			ax, bx = bx, ax
			az, bz = bz, az
		}

		// Tessellate only outer edges or edges between areas.
		if (points[ci*4+3]&contourRegMask) == 0 ||
			(points[ci*4+3]&areaBorder) != 0 {
			for ci != endi {
				d := distancePtSeg(int(points[ci*4+0]), int(points[ci*4+2]), int(ax), int(az), int(bx), int(bz))
				if d > maxd {
					maxd = d
					maxi = ci
				}
				ci = (ci + cinc) % pn
			}
		}

		// If the max deviation is larger than accepted error,
		// add new point, else continue to next segment.
		if maxi != -1 && maxd > (maxError*maxError) {
			simplified = insertSimplifiedPoint(simplified, i, maxi, points)
		} else {
			i++
		}
	}

	// Split too long edges.
	if maxEdgeLen > 0 && (buildFlags&(ContourTessWallEdges|ContourTessAreaEdges)) != 0 {
		for i := 0; i < len(simplified)/4; {
			ii := (i + 1) % (len(simplified) / 4)

			ax := simplified[i*4+0]
			az := simplified[i*4+2]
			ai := simplified[i*4+3]

			bx := simplified[ii*4+0]
			bz := simplified[ii*4+2]
			bi := simplified[ii*4+3]

			// Find maximum deviation from the segment.
			maxi := -1
			ci := (int(ai) + 1) % pn

			// Tessellate only outer edges or edges between areas.
			tess := false
			// Wall edges.
			if (buildFlags&ContourTessWallEdges) != 0 && (points[ci*4+3]&contourRegMask) == 0 {
				tess = true
			}
			// Edges between areas.
			if (buildFlags&ContourTessAreaEdges) != 0 && (points[ci*4+3]&areaBorder) != 0 {
				tess = true
			}

			if tess {
				dx := bx - ax
				dz := bz - az
				if dx*dx+dz*dz > int32(maxEdgeLen*maxEdgeLen) {
					// Round based on the segments in lexilogical order so that the
					// max tesselation is consistent regardles in which direction
					// segments are traversed.
					n := int(bi) - int(ai)
					if bi < ai {
						n = int(bi) + pn - int(ai)
					}
					if n > 1 {
						if bx > ax || (bx == ax && bz > az) {
							maxi = (int(ai) + n/2) % pn
						} else {
							maxi = (int(ai) + (n+1)/2) % pn
						}
					}
				}
			}

			// If the max deviation is larger than accepted error,
			// add new point, else continue to next segment.
			if maxi != -1 {
				simplified = insertSimplifiedPoint(simplified, i, maxi, points)
			} else {
				i++
			}
		}
	}

	for i := 0; i < len(simplified)/4; i++ {
		// The edge vertex flag is take from the current raw point,
		// and the neighbour region is take from the next raw point.
		ai := (int(simplified[i*4+3]) + 1) % pn
		bi := simplified[i*4+3]
		simplified[i*4+3] = (points[ai*4+3] & (contourRegMask | areaBorder)) | (points[bi*4+3] & borderVertex)
	}

	return simplified
}

// calcAreaOfPolygon2D mirrors calcAreaOfPolygon2D (RecastContour.cpp).
func calcAreaOfPolygon2D(verts []int32, nverts int) int {
	area := 0
	for i, j := 0, nverts-1; i < nverts; j, i = i, i+1 {
		vi := verts[i*4:]
		vj := verts[j*4:]
		area += int(vi[0])*int(vj[2]) - int(vj[0])*int(vi[2])
	}
	return (area + 1) / 2
}

// contourPrev mirrors prev (RecastContour.cpp).
func contourPrev(i, n int) int {
	if i-1 >= 0 {
		return i - 1
	}
	return n - 1
}

// contourNext mirrors next (RecastContour.cpp).
func contourNext(i, n int) int {
	if i+1 < n {
		return i + 1
	}
	return 0
}

// contourArea2 mirrors area2 (RecastContour.cpp).
func contourArea2(a, b, c []int32) int {
	return int(b[0]-a[0])*int(c[2]-a[2]) - int(c[0]-a[0])*int(b[2]-a[2])
}

// contourXorb mirrors xorb (RecastContour.cpp).
func contourXorb(x, y bool) bool {
	return !x != !y
}

// contourLeft mirrors left (RecastContour.cpp).
func contourLeft(a, b, c []int32) bool {
	return contourArea2(a, b, c) < 0
}

// contourLeftOn mirrors leftOn (RecastContour.cpp).
func contourLeftOn(a, b, c []int32) bool {
	return contourArea2(a, b, c) <= 0
}

// contourCollinear mirrors collinear (RecastContour.cpp).
func contourCollinear(a, b, c []int32) bool {
	return contourArea2(a, b, c) == 0
}

// contourIntersectProp mirrors intersectProp (RecastContour.cpp).
func contourIntersectProp(a, b, c, d []int32) bool {
	// Eliminate improper cases.
	if contourCollinear(a, b, c) || contourCollinear(a, b, d) ||
		contourCollinear(c, d, a) || contourCollinear(c, d, b) {
		return false
	}

	return contourXorb(contourLeft(a, b, c), contourLeft(a, b, d)) &&
		contourXorb(contourLeft(c, d, a), contourLeft(c, d, b))
}

// contourBetween mirrors between (RecastContour.cpp).
func contourBetween(a, b, c []int32) bool {
	if !contourCollinear(a, b, c) {
		return false
	}
	// If ab not vertical, check betweenness on x; else on y.
	if a[0] != b[0] {
		return ((a[0] <= c[0]) && (c[0] <= b[0])) || ((a[0] >= c[0]) && (c[0] >= b[0]))
	}
	return ((a[2] <= c[2]) && (c[2] <= b[2])) || ((a[2] >= c[2]) && (c[2] >= b[2]))
}

// contourIntersect mirrors intersect (RecastContour.cpp).
func contourIntersect(a, b, c, d []int32) bool {
	if contourIntersectProp(a, b, c, d) {
		return true
	} else if contourBetween(a, b, c) || contourBetween(a, b, d) ||
		contourBetween(c, d, a) || contourBetween(c, d, b) {
		return true
	}
	return false
}

// contourVequal mirrors vequal (RecastContour.cpp).
func contourVequal(a, b []int32) bool {
	return a[0] == b[0] && a[2] == b[2]
}

// contourIntersectSeg mirrors intersectSegCountour (RecastContour.cpp).
func contourIntersectSeg(d0, d1 []int32, i, n int, verts []int32) bool {
	// For each edge (k,k+1) of P
	for k := 0; k < n; k++ {
		k1 := contourNext(k, n)
		// Skip edges incident to i.
		if i == k || i == k1 {
			continue
		}
		p0 := verts[k*4:]
		p1 := verts[k1*4:]
		if contourVequal(d0, p0) || contourVequal(d1, p0) || contourVequal(d0, p1) || contourVequal(d1, p1) {
			continue
		}

		if contourIntersect(d0, d1, p0, p1) {
			return true
		}
	}
	return false
}

// contourInCone mirrors inCone (RecastContour.cpp).
func contourInCone(i, n int, verts []int32, pj []int32) bool {
	pi := verts[i*4:]
	pi1 := verts[contourNext(i, n)*4:]
	pin1 := verts[contourPrev(i, n)*4:]

	// If P[i] is a convex vertex [ i+1 left or on (i-1,i) ].
	if contourLeftOn(pin1, pi, pi1) {
		return contourLeft(pi, pj, pin1) && contourLeft(pj, pi, pi1)
	}
	// Assume (i-1,i,i+1) not collinear.
	// else P[i] is reflex.
	return !(contourLeftOn(pi, pj, pi1) && contourLeftOn(pj, pi, pin1))
}

// removeDegenerateSegments mirrors removeDegenerateSegments (RecastContour.cpp).
func removeDegenerateSegments(simplified []int32) []int32 {
	// Remove adjacent vertices which are equal on xz-plane,
	// or else the triangulator will get confused.
	npts := len(simplified) / 4
	for i := 0; i < npts; i++ {
		ni := contourNext(i, npts)

		if contourVequal(simplified[i*4:], simplified[ni*4:]) {
			// Degenerate segment, remove.
			for j := i; j < len(simplified)/4-1; j++ {
				simplified[j*4+0] = simplified[(j+1)*4+0]
				simplified[j*4+1] = simplified[(j+1)*4+1]
				simplified[j*4+2] = simplified[(j+1)*4+2]
				simplified[j*4+3] = simplified[(j+1)*4+3]
			}
			simplified = simplified[:len(simplified)-4]
			npts--
		}
	}
	return simplified
}

// mergeContours mirrors mergeContours (RecastContour.cpp).
func mergeContours(ca, cb *Contour, ia, ib int) {
	maxVerts := ca.Nverts + cb.Nverts + 2
	verts := make([]int32, 0, maxVerts*4)

	// Copy contour A.
	for i := 0; i <= ca.Nverts; i++ {
		src := ca.Verts[((ia+i)%ca.Nverts)*4:]
		verts = append(verts, src[0], src[1], src[2], src[3])
	}

	// Copy contour B
	for i := 0; i <= cb.Nverts; i++ {
		src := cb.Verts[((ib+i)%cb.Nverts)*4:]
		verts = append(verts, src[0], src[1], src[2], src[3])
	}

	ca.Verts = verts
	ca.Nverts = len(verts) / 4

	cb.Verts = nil
	cb.Nverts = 0
}

// contourHole mirrors rcContourHole (RecastContour.cpp).
type contourHole struct {
	contour  int // index into ContourSet.Conts
	minx     int
	minz     int
	leftmost int
}

// contourRegion mirrors rcContourRegion (RecastContour.cpp).
type contourRegion struct {
	outline int // index into ContourSet.Conts, -1 when none
	holes   []contourHole
}

// potentialDiagonal mirrors rcPotentialDiagonal (RecastContour.cpp).
type potentialDiagonal struct {
	vert int
	dist int
}

// findLeftMostVertex mirrors findLeftMostVertex (RecastContour.cpp).
func findLeftMostVertex(contour *Contour) (minx, minz, leftmost int) {
	minx = int(contour.Verts[0])
	minz = int(contour.Verts[2])
	leftmost = 0
	for i := 1; i < contour.Nverts; i++ {
		x := int(contour.Verts[i*4+0])
		z := int(contour.Verts[i*4+2])
		if x < minx || (x == minx && z < minz) {
			minx = x
			minz = z
			leftmost = i
		}
	}
	return minx, minz, leftmost
}

// mergeRegionHoles mirrors mergeRegionHoles (RecastContour.cpp).
func mergeRegionHoles(cset *ContourSet, region *contourRegion) {
	// Sort holes from left to right.
	for i := range region.holes {
		region.holes[i].minx, region.holes[i].minz, region.holes[i].leftmost =
			findLeftMostVertex(&cset.Conts[region.holes[i].contour])
	}

	sort.Slice(region.holes, func(a, b int) bool {
		if region.holes[a].minx == region.holes[b].minx {
			return region.holes[a].minz < region.holes[b].minz
		}
		return region.holes[a].minx < region.holes[b].minx
	})

	maxVerts := cset.Conts[region.outline].Nverts
	for i := range region.holes {
		maxVerts += cset.Conts[region.holes[i].contour].Nverts
	}

	diags := make([]potentialDiagonal, maxVerts)
	outline := &cset.Conts[region.outline]

	// Merge holes into the outline one by one.
	for i := range region.holes {
		hole := &cset.Conts[region.holes[i].contour]

		index := -1
		bestVertex := region.holes[i].leftmost
		for iter := 0; iter < hole.Nverts; iter++ {
			// Find potential diagonals.
			// The 'best' vertex must be in the cone described by 3 cosequtive vertices of the outline.
			ndiags := 0
			corner := hole.Verts[bestVertex*4:]
			for j := 0; j < outline.Nverts; j++ {
				if contourInCone(j, outline.Nverts, outline.Verts, corner) {
					dx := int(outline.Verts[j*4+0]) - int(corner[0])
					dz := int(outline.Verts[j*4+2]) - int(corner[2])
					diags[ndiags].vert = j
					diags[ndiags].dist = dx*dx + dz*dz
					ndiags++
				}
			}
			// Sort potential diagonals by distance, we want to make the connection as short as possible.
			sort.Slice(diags[:ndiags], func(a, b int) bool {
				return diags[a].dist < diags[b].dist
			})

			// Find a diagonal that is not intersecting the outline not the remaining holes.
			index = -1
			for j := 0; j < ndiags; j++ {
				pt := outline.Verts[diags[j].vert*4:]
				// Mirrors the C++ call as written (diags[i].vert, the hole
				// index, not the diagonal index j).
				intersect := contourIntersectSeg(pt, corner, diags[i].vert, outline.Nverts, outline.Verts)
				for k := i; k < len(region.holes) && !intersect; k++ {
					intersect = intersect || contourIntersectSeg(pt, corner, -1, cset.Conts[region.holes[k].contour].Nverts, cset.Conts[region.holes[k].contour].Verts)
				}
				if !intersect {
					index = diags[j].vert
					break
				}
			}
			// If found non-intersecting diagonal, stop looking.
			if index != -1 {
				break
			}
			// All the potential diagonals for the current vertex were intersecting, try next vertex.
			bestVertex = (bestVertex + 1) % hole.Nverts
		}

		if index == -1 {
			continue
		}
		mergeContours(outline, hole, index, bestVertex)
		// outline.Verts was replaced by mergeContours; re-read the pointer.
		outline = &cset.Conts[region.outline]
	}
}

// BuildContoursFlags mirrors rcBuildContours (RecastContour.cpp).
func BuildContoursFlags(chf *CompactHeightfield, maxError float32, maxEdgeLen int, cset *ContourSet, buildFlags int) bool {
	w := chf.Width
	h := chf.Height
	borderSize := chf.BorderSize

	cset.Bmin = chf.Bmin
	cset.Bmax = chf.Bmax
	if borderSize > 0 {
		// If the heightfield was build with bordersize, remove the offset.
		pad := float32(borderSize) * chf.Cs
		cset.Bmin[0] += pad
		cset.Bmin[2] += pad
		cset.Bmax[0] -= pad
		cset.Bmax[2] -= pad
	}
	cset.Cs = chf.Cs
	cset.Ch = chf.Ch
	cset.Width = chf.Width - chf.BorderSize*2
	cset.Height = chf.Height - chf.BorderSize*2
	cset.BorderSize = chf.BorderSize
	cset.MaxError = maxError

	conts := make([]Contour, 0, maxInt(int(chf.MaxRegions), 8))

	flags := make([]uint8, chf.SpanCount)

	// Mark boundaries.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				var res uint8
				s := chf.Spans[i]
				if chf.Spans[i].Reg == 0 || (chf.Spans[i].Reg&borderReg) != 0 {
					flags[i] = 0
					continue
				}
				for dir := 0; dir < 4; dir++ {
					var r uint16
					if GetCon(&s, dir) != NotConnected {
						ax := x + DirOffsetX(dir)
						ay := y + DirOffsetY(dir)
						ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
						r = chf.Spans[ai].Reg
					}
					if r == chf.Spans[i].Reg {
						res |= (1 << uint(dir))
					}
				}
				flags[i] = res ^ 0xf // Inverse, mark non connected edges.
			}
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				if flags[i] == 0 || flags[i] == 0xf {
					flags[i] = 0
					continue
				}
				reg := chf.Spans[i].Reg
				if reg == 0 || (reg&borderReg) != 0 {
					continue
				}
				area := chf.Areas[i]

				var verts []int32
				var simplified []int32

				walkContourRaw(x, y, i, chf, flags, &verts)

				simplified = simplifyContour(verts, simplified, maxError, maxEdgeLen, buildFlags)
				simplified = removeDegenerateSegments(simplified)

				// Store region->contour remap info.
				// Create contour.
				if len(simplified)/4 >= 3 {
					cont := Contour{
						Nverts:  len(simplified) / 4,
						Verts:   append([]int32(nil), simplified...),
						Nrverts: len(verts) / 4,
						Rverts:  append([]int32(nil), verts...),
						Reg:     reg,
						Area:    area,
					}
					if borderSize > 0 {
						// If the heightfield was build with bordersize, remove the offset.
						for j := 0; j < cont.Nverts; j++ {
							v := cont.Verts[j*4:]
							v[0] -= int32(borderSize)
							v[2] -= int32(borderSize)
						}
						for j := 0; j < cont.Nrverts; j++ {
							v := cont.Rverts[j*4:]
							v[0] -= int32(borderSize)
							v[2] -= int32(borderSize)
						}
					}
					conts = append(conts, cont)
				}
			}
		}
	}

	cset.Conts = conts
	cset.Nconts = len(conts)

	// Merge holes if needed.
	if cset.Nconts > 0 {
		// Calculate winding of all polygons.
		winding := make([]int8, cset.Nconts)
		nholes := 0
		for i := 0; i < cset.Nconts; i++ {
			cont := &cset.Conts[i]
			// If the contour is wound backwards, it is a hole.
			if calcAreaOfPolygon2D(cont.Verts, cont.Nverts) < 0 {
				winding[i] = -1
			} else {
				winding[i] = 1
			}
			if winding[i] < 0 {
				nholes++
			}
		}

		if nholes > 0 {
			// Collect outline contour and holes contours per region.
			// We assume that there is one outline and multiple holes.
			nregions := int(chf.MaxRegions) + 1
			regions := make([]contourRegion, nregions)
			for i := range regions {
				regions[i].outline = -1
			}

			for i := 0; i < cset.Nconts; i++ {
				cont := &cset.Conts[i]
				// Positively would contours are outlines, negative holes.
				if winding[i] > 0 {
					regions[cont.Reg].outline = i
				} else {
					regions[cont.Reg].holes = append(regions[cont.Reg].holes, contourHole{contour: i})
				}
			}

			// Finally merge each regions holes into the outline.
			for i := 0; i < nregions; i++ {
				reg := &regions[i]
				if len(reg.holes) == 0 {
					continue
				}

				if reg.outline >= 0 {
					mergeRegionHoles(cset, reg)
				}
				// else: the region does not have an outline.
				// This can happen if the contour becomes self-overlapping
				// because of too aggressive simplification settings.
			}
		}
	}

	return true
}

// BuildContours mirrors rcBuildContours (RecastContour.cpp) with the default
// build flags RC_CONTOUR_TESS_WALL_EDGES.
func BuildContours(chf *CompactHeightfield, maxError float32, maxEdgeLen int, cset *ContourSet) bool {
	return BuildContoursFlags(chf, maxError, maxEdgeLen, cset, ContourTessWallEdges)
}

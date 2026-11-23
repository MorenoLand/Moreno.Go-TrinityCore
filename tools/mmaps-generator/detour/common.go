// Vector and computational-geometry helpers, mirroring the inline
// functions in dep/recastnavigation/Detour/Include/DetourCommon.h and the
// implementations in Detour/Source/DetourCommon.cpp. All vectors are
// [3]float32 in (x, y, z) order, matching C++ float[3].
package detour

import "math"

// Vec3 is a 3-component float vector, mirroring C++ float[3].
type Vec3 = [3]float32

func vCross(dest, v1, v2 *Vec3) {
	dest[0] = v1[1]*v2[2] - v1[2]*v2[1]
	dest[1] = v1[2]*v2[0] - v1[0]*v2[2]
	dest[2] = v1[0]*v2[1] - v1[1]*v2[0]
}

func vDot(v1, v2 *Vec3) float32 {
	return v1[0]*v2[0] + v1[1]*v2[1] + v1[2]*v2[2]
}

func vMad(dest, v1, v2 *Vec3, s float32) {
	dest[0] = v1[0] + v2[0]*s
	dest[1] = v1[1] + v2[1]*s
	dest[2] = v1[2] + v2[2]*s
}

func vLerp(dest, v1, v2 *Vec3, t float32) {
	dest[0] = v1[0] + (v2[0]-v1[0])*t
	dest[1] = v1[1] + (v2[1]-v1[1])*t
	dest[2] = v1[2] + (v2[2]-v1[2])*t
}

func vAdd(dest, v1, v2 *Vec3) {
	dest[0] = v1[0] + v2[0]
	dest[1] = v1[1] + v2[1]
	dest[2] = v1[2] + v2[2]
}

func vSub(dest, v1, v2 *Vec3) {
	dest[0] = v1[0] - v2[0]
	dest[1] = v1[1] - v2[1]
	dest[2] = v1[2] - v2[2]
}

func vScale(dest, v *Vec3, t float32) {
	dest[0] = v[0] * t
	dest[1] = v[1] * t
	dest[2] = v[2] * t
}

func vMin(mn, v *Vec3) {
	if v[0] < mn[0] {
		mn[0] = v[0]
	}
	if v[1] < mn[1] {
		mn[1] = v[1]
	}
	if v[2] < mn[2] {
		mn[2] = v[2]
	}
}

func vMax(mx, v *Vec3) {
	if v[0] > mx[0] {
		mx[0] = v[0]
	}
	if v[1] > mx[1] {
		mx[1] = v[1]
	}
	if v[2] > mx[2] {
		mx[2] = v[2]
	}
}

func vCopy(dest, a *Vec3) { *dest = *a }

func vLenSqr(v *Vec3) float32 { return v[0]*v[0] + v[1]*v[1] + v[2]*v[2] }

func vDist(v1, v2 *Vec3) float32 {
	dx := v2[0] - v1[0]
	dy := v2[1] - v1[1]
	dz := v2[2] - v1[2]
	return float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
}

func vDistSqr(v1, v2 *Vec3) float32 {
	dx := v2[0] - v1[0]
	dy := v2[1] - v1[1]
	dz := v2[2] - v1[2]
	return dx*dx + dy*dy + dz*dz
}

func vDist2D(v1, v2 *Vec3) float32 {
	dx := v2[0] - v1[0]
	dz := v2[2] - v1[2]
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

func vNormalize(v *Vec3) {
	d := float32(1.0) / float32(math.Sqrt(float64(v[0]*v[0]+v[1]*v[1]+v[2]*v[2])))
	v[0] *= d
	v[1] *= d
	v[2] *= d
}

// vEqual mirrors dtVequal (DetourCommon.h).
func vEqual(p0, p1 *Vec3) bool {
	const thr = float32(1.0/16384.0) * float32(1.0/16384.0)
	return vDistSqr(p0, p1) < thr
}

func isFinite(f float32) bool { return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0) }

// vIsFinite mirrors dtVisfinite (DetourCommon.h).
func vIsFinite(v *Vec3) bool { return isFinite(v[0]) && isFinite(v[1]) && isFinite(v[2]) }

// vIsFinite2D mirrors dtVisfinite2D (DetourCommon.h).
func vIsFinite2D(v *Vec3) bool { return isFinite(v[0]) && isFinite(v[2]) }

func vDot2D(u, v *Vec3) float32 { return u[0]*v[0] + u[2]*v[2] }

func vPerp2D(u, v *Vec3) float32 { return u[2]*v[0] - u[0]*v[2] }

// triArea2D mirrors dtTriArea2D (DetourCommon.h).
func triArea2D(a, b, c *Vec3) float32 {
	abx := b[0] - a[0]
	abz := b[2] - a[2]
	acx := c[0] - a[0]
	acz := c[2] - a[2]
	return acx*abz - abx*acz
}

// overlapQuantBounds mirrors dtOverlapQuantBounds (DetourCommon.h).
func overlapQuantBounds(amin, amax, bmin, bmax [3]uint16) bool {
	if amin[0] > bmax[0] || amax[0] < bmin[0] {
		return false
	}
	if amin[1] > bmax[1] || amax[1] < bmin[1] {
		return false
	}
	if amin[2] > bmax[2] || amax[2] < bmin[2] {
		return false
	}
	return true
}

// overlapBounds mirrors dtOverlapBounds (DetourCommon.h).
func overlapBounds(amin, amax, bmin, bmax *Vec3) bool {
	if amin[0] > bmax[0] || amax[0] < bmin[0] {
		return false
	}
	if amin[1] > bmax[1] || amax[1] < bmin[1] {
		return false
	}
	if amin[2] > bmax[2] || amax[2] < bmin[2] {
		return false
	}
	return true
}

func dtMinF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func dtMaxF(a, b float32) float32 {
	if a < b {
		return b
	}
	return a
}

func dtAbsF(a float32) float32 {
	if a < 0 {
		return -a
	}
	return a
}

func dtSqrF(a float32) float32 { return a * a }

func dtClampF(v, mn, mx float32) float32 {
	if v < mn {
		return mn
	}
	if v > mx {
		return mx
	}
	return v
}

// nextPow2 mirrors dtNextPow2 (DetourCommon.h).
func nextPow2(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

// ilog2 mirrors dtIlog2 (DetourCommon.h).
func ilog2(v uint32) uint32 {
	var r uint32
	var shift uint32
	if v > 0xffff {
		r = 4
	} else {
		r = 0
	}
	v >>= r
	if v > 0xff {
		shift = 3
	} else {
		shift = 0
	}
	v >>= shift
	r |= shift
	if v > 0xf {
		shift = 2
	} else {
		shift = 0
	}
	v >>= shift
	r |= shift
	if v > 0x3 {
		shift = 1
	} else {
		shift = 0
	}
	v >>= shift
	r |= shift
	r |= v >> 1
	return r
}

// oppositeTile mirrors dtOppositeTile (DetourCommon.h).
func oppositeTile(side int) int { return (side + 4) & 0x7 }

// closestPtPointTriangle mirrors dtClosestPtPointTriangle
// (DetourCommon.cpp), Ericson's region-test formulation.
func closestPtPointTriangle(closest, p, a, b, c *Vec3) {
	var ab, ac, ap Vec3
	vSub(&ab, b, a)
	vSub(&ac, c, a)
	vSub(&ap, p, a)
	d1 := vDot(&ab, &ap)
	d2 := vDot(&ac, &ap)
	if d1 <= 0.0 && d2 <= 0.0 {
		vCopy(closest, a)
		return
	}

	var bp Vec3
	vSub(&bp, p, b)
	d3 := vDot(&ab, &bp)
	d4 := vDot(&ac, &bp)
	if d3 >= 0.0 && d4 <= d3 {
		vCopy(closest, b)
		return
	}

	vc := d1*d4 - d3*d2
	if vc <= 0.0 && d1 >= 0.0 && d3 <= 0.0 {
		v := d1 / (d1 - d3)
		closest[0] = a[0] + v*ab[0]
		closest[1] = a[1] + v*ab[1]
		closest[2] = a[2] + v*ab[2]
		return
	}

	var cp Vec3
	vSub(&cp, p, c)
	d5 := vDot(&ab, &cp)
	d6 := vDot(&ac, &cp)
	if d6 >= 0.0 && d5 <= d6 {
		vCopy(closest, c)
		return
	}

	vb := d5*d2 - d1*d6
	if vb <= 0.0 && d2 >= 0.0 && d6 <= 0.0 {
		w := d2 / (d2 - d6)
		closest[0] = a[0] + w*ac[0]
		closest[1] = a[1] + w*ac[1]
		closest[2] = a[2] + w*ac[2]
		return
	}

	va := d3*d6 - d5*d4
	if va <= 0.0 && (d4-d3) >= 0.0 && (d5-d6) >= 0.0 {
		w := (d4 - d3) / ((d4 - d3) + (d5 - d6))
		closest[0] = b[0] + w*(c[0]-b[0])
		closest[1] = b[1] + w*(c[1]-b[1])
		closest[2] = b[2] + w*(c[2]-b[2])
		return
	}

	denom := float32(1.0) / (va + vb + vc)
	v := vb * denom
	w := vc * denom
	closest[0] = a[0] + ab[0]*v + ac[0]*w
	closest[1] = a[1] + ab[1]*v + ac[1]*w
	closest[2] = a[2] + ab[2]*v + ac[2]*w
}

// closestHeightPointTriangle mirrors dtClosestHeightPointTriangle
// (DetourCommon.cpp). It reports the interpolated height h when p lies
// inside the triangle on the xz-plane.
func closestHeightPointTriangle(p, a, b, c *Vec3) (float32, bool) {
	const eps = float32(1e-6)
	var v0, v1, v2 Vec3
	vSub(&v0, c, a)
	vSub(&v1, b, a)
	vSub(&v2, p, a)

	denom := v0[0]*v1[2] - v0[2]*v1[0]
	if float32(math.Abs(float64(denom))) < eps {
		return 0, false
	}

	u := v1[2]*v2[0] - v1[0]*v2[2]
	v := v0[0]*v2[2] - v0[2]*v2[0]

	if denom < 0 {
		denom = -denom
		u = -u
		v = -v
	}

	if u >= 0.0 && v >= 0.0 && (u+v) <= denom {
		return a[1] + (v0[1]*u+v1[1]*v)/denom, true
	}
	return 0, false
}

// intersectSegmentPoly2D mirrors dtIntersectSegmentPoly2D
// (DetourCommon.cpp). It returns tmin/tmax and the edge indices the
// segment enters/leaves on (segMin/segMax, -1 when the segment end is
// inside the polygon).
func intersectSegmentPoly2D(p0, p1 *Vec3, verts []Vec3) (tmin, tmax float32, segMin, segMax int, ok bool) {
	const eps = float32(0.00000001)
	tmin = 0
	tmax = 1
	segMin = -1
	segMax = -1

	nverts := len(verts)
	var dir Vec3
	vSub(&dir, p1, p0)

	for i, j := 0, nverts-1; i < nverts; j, i = i, i+1 {
		var edge, diff Vec3
		vSub(&edge, &verts[i], &verts[j])
		vSub(&diff, p0, &verts[j])
		n := vPerp2D(&edge, &diff)
		d := vPerp2D(&dir, &edge)
		if float32(math.Abs(float64(d))) < eps {
			if n < 0 {
				return 0, 0, 0, 0, false
			}
			continue
		}
		t := n / d
		if d < 0 {
			if t > tmin {
				tmin = t
				segMin = j
				if tmin > tmax {
					return 0, 0, 0, 0, false
				}
			}
		} else {
			if t < tmax {
				tmax = t
				segMax = j
				if tmax < tmin {
					return 0, 0, 0, 0, false
				}
			}
		}
	}
	return tmin, tmax, segMin, segMax, true
}

// distancePtSegSqr2D mirrors dtDistancePtSegSqr2D (DetourCommon.cpp).
// It returns the squared 2D distance from pt to segment p-q and the
// segment parameter t of the closest point.
func distancePtSegSqr2D(pt, p, q *Vec3) (float32, float32) {
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
	return dx*dx + dz*dz, t
}

// pointInPolygon mirrors dtPointInPolygon (DetourCommon.cpp): pnpoly on
// the xz-plane.
func pointInPolygon(pt *Vec3, verts []Vec3) bool {
	nverts := len(verts)
	c := false
	for i, j := 0, nverts-1; i < nverts; j, i = i, i+1 {
		vi := &verts[i]
		vj := &verts[j]
		if ((vi[2] > pt[2]) != (vj[2] > pt[2])) &&
			(pt[0] < (vj[0]-vi[0])*(pt[2]-vi[2])/(vj[2]-vi[2])+vi[0]) {
			c = !c
		}
	}
	return c
}

// distancePtPolyEdgesSqr mirrors dtDistancePtPolyEdgesSqr
// (DetourCommon.cpp): the pnpoly test plus per-edge squared distances.
// ed/et are filled with one entry per edge (edge j runs vj -> vi).
func distancePtPolyEdgesSqr(pt *Vec3, verts []Vec3, ed, et []float32) bool {
	nverts := len(verts)
	c := false
	for i, j := 0, nverts-1; i < nverts; j, i = i, i+1 {
		vi := &verts[i]
		vj := &verts[j]
		if ((vi[2] > pt[2]) != (vj[2] > pt[2])) &&
			(pt[0] < (vj[0]-vi[0])*(pt[2]-vi[2])/(vj[2]-vi[2])+vi[0]) {
			c = !c
		}
		ed[j], et[j] = distancePtSegSqr2D(pt, vj, vi)
	}
	return c
}

func projectPoly(axis *Vec3, poly []Vec3) (float32, float32) {
	rmin := vDot2D(axis, &poly[0])
	rmax := rmin
	for i := 1; i < len(poly); i++ {
		d := vDot2D(axis, &poly[i])
		if d < rmin {
			rmin = d
		}
		if d > rmax {
			rmax = d
		}
	}
	return rmin, rmax
}

func overlapRange(amin, amax, bmin, bmax, eps float32) bool {
	return !((amin+eps) > bmax || (amax-eps) < bmin)
}

// overlapPolyPoly2D mirrors dtOverlapPolyPoly2D (DetourCommon.cpp):
// separating-axis test on the xz-plane.
func overlapPolyPoly2D(polya, polyb []Vec3) bool {
	const eps = float32(1e-4)
	for i, j := 0, len(polya)-1; i < len(polya); j, i = i, i+1 {
		va := &polya[j]
		vb := &polya[i]
		n := Vec3{vb[2] - va[2], 0, -(vb[0] - va[0])}
		amin, amax := projectPoly(&n, polya)
		bmin, bmax := projectPoly(&n, polyb)
		if !overlapRange(amin, amax, bmin, bmax, eps) {
			return false
		}
	}
	for i, j := 0, len(polyb)-1; i < len(polyb); j, i = i, i+1 {
		va := &polyb[j]
		vb := &polyb[i]
		n := Vec3{vb[2] - va[2], 0, -(vb[0] - va[0])}
		amin, amax := projectPoly(&n, polya)
		bmin, bmax := projectPoly(&n, polyb)
		if !overlapRange(amin, amax, bmin, bmax, eps) {
			return false
		}
	}
	return true
}

// randomPointInConvexPoly mirrors dtRandomPointInConvexPoly
// (DetourCommon.cpp). s and t are the caller's [0,1) random values;
// areas is scratch space of length >= npts.
func randomPointInConvexPoly(pts []Vec3, areas []float32, s, t float32) Vec3 {
	npts := len(pts)
	areasum := float32(0)
	for i := 2; i < npts; i++ {
		areas[i] = triArea2D(&pts[0], &pts[i-1], &pts[i])
		areasum += dtMaxF(0.001, areas[i])
	}
	thr := s * areasum
	acc := float32(0)
	u := float32(1.0)
	tri := npts - 1
	for i := 2; i < npts; i++ {
		dacc := areas[i]
		if thr >= acc && thr < (acc+dacc) {
			u = (thr - acc) / dacc
			tri = i
			break
		}
		acc += dacc
	}

	v := float32(math.Sqrt(float64(t)))

	a := 1 - v
	b := (1 - u) * v
	c := u * v
	pa := &pts[0]
	pb := &pts[tri-1]
	pc := &pts[tri]

	return Vec3{
		a*pa[0] + b*pb[0] + c*pc[0],
		a*pa[1] + b*pb[1] + c*pc[1],
		a*pa[2] + b*pb[2] + c*pc[2],
	}
}

func vPerpXZ(a, b *Vec3) float32 { return a[0]*b[2] - a[2]*b[0] }

// intersectSegSeg2D mirrors dtIntersectSegSeg2D (DetourCommon.cpp).
func intersectSegSeg2D(ap, aq, bp, bq *Vec3) (float32, float32, bool) {
	var u, v, w Vec3
	vSub(&u, aq, ap)
	vSub(&v, bq, bp)
	vSub(&w, ap, bp)
	d := vPerpXZ(&u, &v)
	if float32(math.Abs(float64(d))) < 1e-6 {
		return 0, 0, false
	}
	return vPerpXZ(&v, &w) / d, vPerpXZ(&u, &w) / d, true
}

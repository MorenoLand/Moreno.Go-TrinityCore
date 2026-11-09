package recast

import "math"

// Rasterization port (RecastRasterization.cpp).

func overlapBounds(amin, amax, bmin, bmax [3]float32) bool {
	overlap := true
	if amin[0] > bmax[0] || amax[0] < bmin[0] {
		overlap = false
	}
	if amin[1] > bmax[1] || amax[1] < bmin[1] {
		overlap = false
	}
	if amin[2] > bmax[2] || amax[2] < bmin[2] {
		overlap = false
	}
	return overlap
}

// dividePoly divides a convex polygon into two convex polygons on both sides
// of a line (RecastRasterization.cpp). in/out slices hold xyz triples.
func dividePoly(in []float32, nin int, out1, out2 []float32, x float32, axis int) (nout1, nout2 int) {
	var d [12]float32
	for i := 0; i < nin; i++ {
		d[i] = x - in[i*3+axis]
	}

	m := 0
	n := 0
	for i, j := 0, nin-1; i < nin; j, i = i, i+1 {
		ina := d[j] >= 0
		inb := d[i] >= 0
		if ina != inb {
			s := d[j] / (d[j] - d[i])
			out1[m*3+0] = in[j*3+0] + (in[i*3+0]-in[j*3+0])*s
			out1[m*3+1] = in[j*3+1] + (in[i*3+1]-in[j*3+1])*s
			out1[m*3+2] = in[j*3+2] + (in[i*3+2]-in[j*3+2])*s
			copy(out2[n*3:], out1[m*3:m*3+3])
			m++
			n++
			// Add the i'th point to the right polygon. Do NOT add points that are on the dividing line
			// since these were already added above.
			if d[i] > 0 {
				copy(out1[m*3:], in[i*3:i*3+3])
				m++
			} else if d[i] < 0 {
				copy(out2[n*3:], in[i*3:i*3+3])
				n++
			}
		} else { // same side
			// Add the i'th point to the right polygon. Addition is done even for points on the dividing line.
			if d[i] >= 0 {
				copy(out1[m*3:], in[i*3:i*3+3])
				m++
				if d[i] != 0 {
					continue
				}
			}
			copy(out2[n*3:], in[i*3:i*3+3])
			n++
		}
	}

	return m, n
}

func rasterizeTri(v0, v1, v2 [3]float32, area uint8, hf *Heightfield,
	bmin, bmax [3]float32, cs, ics, ich float32, flagMergeThr int) bool {
	w := hf.Width
	h := hf.Height
	by := bmax[1] - bmin[1]

	// Calculate the bounding box of the triangle.
	tmin := v0
	tmax := v0
	tmin = vMin(tmin, v1)
	tmin = vMin(tmin, v2)
	tmax = vMax(tmax, v1)
	tmax = vMax(tmax, v2)

	// If the triangle does not touch the bbox of the heightfield, skip the triangle.
	if !overlapBounds(bmin, bmax, tmin, tmax) {
		return true
	}

	// Calculate the footprint of the triangle on the grid's y-axis.
	y0 := int((tmin[2] - bmin[2]) * ics)
	y1 := int((tmax[2] - bmin[2]) * ics)
	y0 = clampInt(y0, 0, h-1)
	y1 = clampInt(y1, 0, h-1)

	// Clip the triangle into all grid cells it touches.
	var buf [7 * 3 * 4]float32
	in := buf[0 : 7*3]
	inrow := buf[7*3 : 7*3+7*3]
	p1 := buf[7*3*2 : 7*3*2+7*3]
	p2 := buf[7*3*3 : 7*3*3+7*3]

	copy(in[0:], v0[:])
	copy(in[1*3:], v1[:])
	copy(in[2*3:], v2[:])
	nvIn := 3

	for y := y0; y <= y1; y++ {
		// Clip polygon to row. Store the remaining polygon as well.
		cz := bmin[2] + float32(y)*cs
		var nvrow int
		nvrow, nvIn = dividePoly(in, nvIn, inrow, p1, cz+cs, 2)
		// rcSwap(in, p1)
		in, p1 = p1, in
		if nvrow < 3 {
			continue
		}

		// Find the horizontal bounds in the row.
		minX := inrow[0]
		maxX := inrow[0]
		for i := 1; i < nvrow; i++ {
			if minX > inrow[i*3] {
				minX = inrow[i*3]
			}
			if maxX < inrow[i*3] {
				maxX = inrow[i*3]
			}
		}
		x0 := int((minX - bmin[0]) * ics)
		x1 := int((maxX - bmin[0]) * ics)
		x0 = clampInt(x0, 0, w-1)
		x1 = clampInt(x1, 0, w-1)

		nv2 := nvrow

		for x := x0; x <= x1; x++ {
			// Clip polygon to column. Store the remaining polygon as well.
			cx := bmin[0] + float32(x)*cs
			var nv int
			nv, nv2 = dividePoly(inrow, nv2, p1, p2, cx+cs, 0)
			// rcSwap(inrow, p2)
			inrow, p2 = p2, inrow
			if nv < 3 {
				continue
			}

			// Calculate min and max of the span.
			smin := p1[1]
			smax := p1[1]
			for i := 1; i < nv; i++ {
				smin = minFloat(smin, p1[i*3+1])
				smax = maxFloat(smax, p1[i*3+1])
			}
			smin -= bmin[1]
			smax -= bmin[1]
			// Skip the span if it is outside the heightfield bbox.
			if smax < 0.0 {
				continue
			}
			if smin > by {
				continue
			}
			// Clamp the span to the heightfield bbox.
			if smin < 0.0 {
				smin = 0
			}
			if smax > by {
				smax = by
			}

			// Snap the span to the heightfield height grid.
			ismin := uint16(clampInt(int(math.Floor(float64(smin*ich))), 0, SpanMaxHeight))
			ismax := uint16(clampInt(int(math.Ceil(float64(smax*ich))), int(ismin)+1, SpanMaxHeight))

			if !addSpan(hf, x, y, ismin, ismax, area, flagMergeThr) {
				return false
			}
		}
	}

	return true
}

// RasterizeTriangle mirrors rcRasterizeTriangle (RecastRasterization.cpp).
func RasterizeTriangle(v0, v1, v2 [3]float32, area uint8, hf *Heightfield, flagMergeThr int) bool {
	ics := float32(1.0) / hf.Cs
	ich := float32(1.0) / hf.Ch
	return rasterizeTri(v0, v1, v2, area, hf, hf.Bmin, hf.Bmax, hf.Cs, ics, ich, flagMergeThr)
}

// RasterizeTriangles mirrors rcRasterizeTriangles (RecastRasterization.cpp,
// int-index overload used by MapBuilder).
func RasterizeTriangles(verts []float32, tris []int32, areas []uint8, hf *Heightfield, flagMergeThr int) bool {
	ics := float32(1.0) / hf.Cs
	ich := float32(1.0) / hf.Ch
	for i := 0; i < len(tris)/3; i++ {
		v0 := [3]float32{verts[tris[i*3+0]*3], verts[tris[i*3+0]*3+1], verts[tris[i*3+0]*3+2]}
		v1 := [3]float32{verts[tris[i*3+1]*3], verts[tris[i*3+1]*3+1], verts[tris[i*3+1]*3+2]}
		v2 := [3]float32{verts[tris[i*3+2]*3], verts[tris[i*3+2]*3+1], verts[tris[i*3+2]*3+2]}
		if !rasterizeTri(v0, v1, v2, areas[i], hf, hf.Bmin, hf.Bmax, hf.Cs, ics, ich, flagMergeThr) {
			return false
		}
	}
	return true
}

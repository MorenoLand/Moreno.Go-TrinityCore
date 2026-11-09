// Package recast is a Go port of the Recast navigation-mesh build pipeline
// (dep/recastnavigation/Recast) as used by TrinityCore's mmaps_generator
// (src/tools/mmaps_generator/MapBuilder.cpp, TileBuilder::buildMoveMapTile).
//
// Only the stages the generator needs are ported, in build order:
// heightfield creation, triangle rasterization, walkable marking and
// filters, compact heightfield, erosion, median filtering, distance field.
// Regions, contours, polymesh and detail mesh live in later stages and are
// NOT in this package yet.
package recast

import "math"

// Area ids (Recast.h RC_NULL_AREA; src/common/Collision/Maps/MapDefines.h NavArea).
const (
	NullArea        = 0
	AreaEmpty       = 0
	AreaGround      = 11
	AreaGroundSteep = 10
	AreaWater       = 9
	AreaMagmaSlime  = 8
	AreaMaxValue    = AreaGround
	AreaMinValue    = AreaMagmaSlime
	AreaAllMask     = 0x3F
)

// NotConnected is the rcCompactSpan connection value for "no neighbour"
// (Recast.h RC_NOT_CONNECTED).
const NotConnected = 0x3f

// SpanMaxHeight is the max rcSpan smin/smax value (Recast.h).
const SpanMaxHeight = (1 << 16) - 1

// Config mirrors rcConfig (Recast.h).
type Config struct {
	Width  int
	Height int

	TileSize   int
	BorderSize int

	Cs float32
	Ch float32

	Bmin [3]float32
	Bmax [3]float32

	WalkableSlopeAngle         float32
	WalkableSlopeAngleNotSteep float32
	WalkableHeight             int
	WalkableClimb              int
	WalkableRadius             int

	MaxEdgeLen             int
	MaxSimplificationError float32
	MinRegionArea          int
	MergeRegionArea        int
	MaxVertsPerPoly        int
	DetailSampleDist       float32
	DetailSampleMaxError   float32
}

// Span is one rcSpan: a vertical span in a heightfield column.
type Span struct {
	Smin uint16
	Smax uint16
	Area uint8
	Next *Span
}

// Heightfield mirrors rcHeightfield.
type Heightfield struct {
	Width  int
	Height int
	Bmin   [3]float32
	Bmax   [3]float32
	Cs     float32
	Ch     float32
	Spans  []*Span // width*height, head of the per-cell span list
}

// CompactCell mirrors rcCompactCell.
type CompactCell struct {
	Index uint32
	Count uint32
}

// CompactSpan mirrors rcCompactSpan.
type CompactSpan struct {
	Y   uint16
	Reg uint16
	Con uint32 // 4 packed 6-bit neighbour connections
	H   uint8
}

// CompactHeightfield mirrors rcCompactHeightfield.
type CompactHeightfield struct {
	Width          int
	Height         int
	SpanCount      int
	WalkableHeight int
	WalkableClimb  int
	BorderSize     int
	MaxDistance    uint16
	MaxRegions     uint16
	Bmin           [3]float32
	Bmax           [3]float32
	Cs             float32
	Ch             float32
	Cells          []CompactCell
	Spans          []CompactSpan
	Dist           []uint16
	Areas          []uint8
}

func clampInt(v, mn, mx int) int {
	if v < mn {
		return mn
	}
	if v > mx {
		return mx
	}
	return v
}

func clampFloat(v, mn, mx float32) float32 {
	if v < mn {
		return mn
	}
	if v > mx {
		return mx
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func vCopy(dst, v [3]float32) [3]float32 { return v }

func vMin(mn, v [3]float32) [3]float32 {
	return [3]float32{
		minFloat(mn[0], v[0]),
		minFloat(mn[1], v[1]),
		minFloat(mn[2], v[2]),
	}
}

func vMax(mx, v [3]float32) [3]float32 {
	return [3]float32{
		maxFloat(mx[0], v[0]),
		maxFloat(mx[1], v[1]),
		maxFloat(mx[2], v[2]),
	}
}

func minFloat(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func vSub(a, b [3]float32) [3]float32 { return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func vCross(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func vNormalize(v [3]float32) [3]float32 {
	d := float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])))
	if d == 0 {
		return v
	}
	d = 1.0 / d
	return [3]float32{v[0] * d, v[1] * d, v[2] * d}
}

// SetCon mirrors rcSetCon (Recast.h).
func SetCon(s *CompactSpan, dir, i int) {
	shift := uint(dir * 6)
	s.Con = (s.Con &^ (0x3f << shift)) | ((uint32(i) & 0x3f) << shift)
}

// GetCon mirrors rcGetCon (Recast.h).
func GetCon(s *CompactSpan, dir int) int {
	shift := uint(dir * 6)
	return int(s.Con>>shift) & 0x3f
}

// DirOffsetX mirrors rcGetDirOffsetX (Recast.h).
func DirOffsetX(dir int) int {
	offset := [4]int{-1, 0, 1, 0}
	return offset[dir&0x03]
}

// DirOffsetY mirrors rcGetDirOffsetY (Recast.h).
func DirOffsetY(dir int) int {
	offset := [4]int{0, 1, 0, -1}
	return offset[dir&0x03]
}

// CalcBounds mirrors rcCalcBounds (Recast.cpp).
func CalcBounds(verts []float32, nv int) (bmin, bmax [3]float32) {
	bmin = [3]float32{verts[0], verts[1], verts[2]}
	bmax = bmin
	for i := 1; i < nv; i++ {
		v := [3]float32{verts[i*3], verts[i*3+1], verts[i*3+2]}
		bmin = vMin(bmin, v)
		bmax = vMax(bmax, v)
	}
	return bmin, bmax
}

// CalcGridSize mirrors rcCalcGridSize (Recast.cpp).
func CalcGridSize(bmin, bmax [3]float32, cs float32) (w, h int) {
	w = int((bmax[0]-bmin[0])/cs + 0.5)
	h = int((bmax[2]-bmin[2])/cs + 0.5)
	return w, h
}

// CreateHeightfield mirrors rcCreateHeightfield (Recast.cpp).
func CreateHeightfield(width, height int, bmin, bmax [3]float32, cs, ch float32) *Heightfield {
	return &Heightfield{
		Width:  width,
		Height: height,
		Bmin:   bmin,
		Bmax:   bmax,
		Cs:     cs,
		Ch:     ch,
		Spans:  make([]*Span, width*height),
	}
}

// AddSpan mirrors rcAddSpan (RecastRasterization.cpp).
func AddSpan(hf *Heightfield, x, y int, smin, smax uint16, area uint8, flagMergeThr int) bool {
	return addSpan(hf, x, y, smin, smax, area, flagMergeThr)
}

func addSpan(hf *Heightfield, x, y int, smin, smax uint16, area uint8, flagMergeThr int) bool {
	idx := x + y*hf.Width

	s := &Span{Smin: smin, Smax: smax, Area: area}

	// Empty cell, add the first span.
	if hf.Spans[idx] == nil {
		hf.Spans[idx] = s
		return true
	}
	var prev *Span
	cur := hf.Spans[idx]

	// Insert and merge spans.
	for cur != nil {
		if cur.Smin > s.Smax {
			// Current span is further than the new span, break.
			break
		} else if cur.Smax < s.Smin {
			// Current span is before the new span, advance.
			prev = cur
			cur = cur.Next
		} else {
			// Merge spans.
			if cur.Smin < s.Smin {
				s.Smin = cur.Smin
			}
			if cur.Smax > s.Smax {
				s.Smax = cur.Smax
			}

			// Merge flags.
			if absInt(int(s.Smax)-int(cur.Smax)) <= flagMergeThr {
				if cur.Area > s.Area {
					s.Area = cur.Area
				}
			}

			// Remove current span.
			next := cur.Next
			if prev != nil {
				prev.Next = next
			} else {
				hf.Spans[idx] = next
			}
			cur = next
		}
	}

	// Insert new span.
	if prev != nil {
		s.Next = prev.Next
		prev.Next = s
	} else {
		s.Next = hf.Spans[idx]
		hf.Spans[idx] = s
	}

	return true
}

func calcTriNormal(v0, v1, v2 [3]float32) [3]float32 {
	e0 := vSub(v1, v0)
	e1 := vSub(v2, v0)
	return vNormalize(vCross(e0, e1))
}

// MarkWalkableTriangles mirrors rcMarkWalkableTriangles (Recast.cpp): only
// sets the area id for walkable triangles.
func MarkWalkableTriangles(walkableSlopeAngle float32, verts []float32, tris []int32, areas []uint8, areaType uint8) {
	walkableThr := float32(math.Cos(float64(walkableSlopeAngle) / 180.0 * math.Pi))
	for i := 0; i < len(tris)/3; i++ {
		tri := tris[i*3:]
		norm := calcTriNormal(
			[3]float32{verts[tri[0]*3], verts[tri[0]*3+1], verts[tri[0]*3+2]},
			[3]float32{verts[tri[1]*3], verts[tri[1]*3+1], verts[tri[1]*3+2]},
			[3]float32{verts[tri[2]*3], verts[tri[2]*3+1], verts[tri[2]*3+2]},
		)
		if norm[1] > walkableThr {
			areas[i] = areaType
		}
	}
}

// ClearUnwalkableTriangles mirrors rcClearUnwalkableTriangles (Recast.cpp):
// only sets the area id for unwalkable triangles.
func ClearUnwalkableTriangles(walkableSlopeAngle float32, verts []float32, tris []int32, areas []uint8) {
	walkableThr := float32(math.Cos(float64(walkableSlopeAngle) / 180.0 * math.Pi))
	for i := 0; i < len(tris)/3; i++ {
		tri := tris[i*3:]
		norm := calcTriNormal(
			[3]float32{verts[tri[0]*3], verts[tri[0]*3+1], verts[tri[0]*3+2]},
			[3]float32{verts[tri[1]*3], verts[tri[1]*3+1], verts[tri[1]*3+2]},
			[3]float32{verts[tri[2]*3], verts[tri[2]*3+1], verts[tri[2]*3+2]},
		)
		if norm[1] <= walkableThr {
			areas[i] = NullArea
		}
	}
}

// GetHeightFieldSpanCount mirrors rcGetHeightFieldSpanCount (Recast.cpp).
func GetHeightFieldSpanCount(hf *Heightfield) int {
	spanCount := 0
	for y := 0; y < hf.Height; y++ {
		for x := 0; x < hf.Width; x++ {
			for s := hf.Spans[x+y*hf.Width]; s != nil; s = s.Next {
				if s.Area != NullArea {
					spanCount++
				}
			}
		}
	}
	return spanCount
}

// BuildCompactHeightfield mirrors rcBuildCompactHeightfield (Recast.cpp).
func BuildCompactHeightfield(walkableHeight, walkableClimb int, hf *Heightfield) *CompactHeightfield {
	w := hf.Width
	h := hf.Height
	spanCount := GetHeightFieldSpanCount(hf)

	chf := &CompactHeightfield{
		Width:          w,
		Height:         h,
		SpanCount:      spanCount,
		WalkableHeight: walkableHeight,
		WalkableClimb:  walkableClimb,
		MaxRegions:     0,
		Bmin:           hf.Bmin,
		Bmax:           hf.Bmax,
		Cs:             hf.Cs,
		Ch:             hf.Ch,
		Cells:          make([]CompactCell, w*h),
		Spans:          make([]CompactSpan, spanCount),
		Areas:          make([]uint8, spanCount),
	}
	chf.Bmax[1] += float32(walkableHeight) * hf.Ch
	for i := range chf.Areas {
		chf.Areas[i] = NullArea
	}

	const maxHeight = 0xffff

	// Fill in cells and spans.
	idx := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := hf.Spans[x+y*w]
			// If there are no spans at this cell, just leave the data to index=0, count=0.
			if s == nil {
				continue
			}
			c := &chf.Cells[x+y*w]
			c.Index = uint32(idx)
			c.Count = 0
			for s != nil {
				if s.Area != NullArea {
					bot := int(s.Smax)
					top := maxHeight
					if s.Next != nil {
						top = int(s.Next.Smin)
					}
					chf.Spans[idx].Y = uint16(clampInt(bot, 0, 0xffff))
					chf.Spans[idx].H = uint8(clampInt(top-bot, 0, 0xff))
					chf.Areas[idx] = s.Area
					idx++
					c.Count++
				}
				s = s.Next
			}
		}
	}

	// Find neighbour connections.
	const maxLayers = NotConnected - 1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := &chf.Spans[i]

				for dir := 0; dir < 4; dir++ {
					SetCon(s, dir, NotConnected)
					nx := x + DirOffsetX(dir)
					ny := y + DirOffsetY(dir)
					// First check that the neighbour cell is in bounds.
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}

					// Iterate over all neighbour spans and check if any of them is
					// accessible from the current cell.
					nc := chf.Cells[nx+ny*w]
					for k := int(nc.Index); k < int(nc.Index+nc.Count); k++ {
						ns := chf.Spans[k]
						bot := maxInt(int(s.Y), int(ns.Y))
						top := minInt(int(s.Y)+int(s.H), int(ns.Y)+int(ns.H))

						// Check that the gap between the spans is walkable,
						// and that the climb height between the gaps is not too high.
						if (top-bot) >= walkableHeight && absInt(int(ns.Y)-int(s.Y)) <= walkableClimb {
							// Mark direction as walkable.
							lidx := k - int(nc.Index)
							if lidx < 0 || lidx > maxLayers {
								continue
							}
							SetCon(s, dir, lidx)
							break
						}
					}
				}
			}
		}
	}

	return chf
}

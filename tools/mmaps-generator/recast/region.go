// Region building: watershed (rcBuildRegions), monotone
// (rcBuildRegionsMonotone) and layer (rcBuildLayerRegions) partitioning
// (dep/recastnavigation/Recast/Source/RecastRegion.cpp).
//
// calculateDistanceField, boxBlur and rcBuildDistanceField already live in
// area.go; this file holds the region code that consumes the distance field.
package recast

// BorderReg is the region-id flag marking border regions (Recast.h
// RC_BORDER_REG).
const BorderReg = 0x8000

// nullNeighbour is the "no neighbour" sweep id (RecastRegion.cpp
// RC_NULL_NEI).
const nullNeighbour = 0xffff

// levelStackEntry mirrors LevelStackEntry (RecastRegion.cpp).
type levelStackEntry struct {
	x     int
	y     int
	index int
}

// dirtyEntry mirrors DirtyEntry (RecastRegion.cpp).
type dirtyEntry struct {
	index     int
	region    uint16
	distance2 uint16
}

// floodRegion mirrors floodRegion (RecastRegion.cpp).
func floodRegion(x, y, i int, level, r uint16, chf *CompactHeightfield,
	srcReg, srcDist []uint16, stack *[]levelStackEntry) bool {
	w := chf.Width

	area := chf.Areas[i]

	// Flood fill mark region.
	*stack = (*stack)[:0]
	*stack = append(*stack, levelStackEntry{x, y, i})
	srcReg[i] = r
	srcDist[i] = 0

	lev := uint16(0)
	if level >= 2 {
		lev = level - 2
	}
	count := 0

	for len(*stack) > 0 {
		back := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		cx := back.x
		cy := back.y
		ci := back.index

		cs := chf.Spans[ci]

		// Check if any of the neighbours already have a valid region set.
		var ar uint16
		for dir := 0; dir < 4; dir++ {
			// 8 connected
			if GetCon(&cs, dir) != NotConnected {
				ax := cx + DirOffsetX(dir)
				ay := cy + DirOffsetY(dir)
				ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&cs, dir)
				if chf.Areas[ai] != area {
					continue
				}
				nr := srcReg[ai]
				if nr&BorderReg != 0 { // Do not take borders into account.
					continue
				}
				if nr != 0 && nr != r {
					ar = nr
					break
				}

				as := chf.Spans[ai]

				dir2 := (dir + 1) & 0x3
				if GetCon(&as, dir2) != NotConnected {
					ax2 := ax + DirOffsetX(dir2)
					ay2 := ay + DirOffsetY(dir2)
					ai2 := int(chf.Cells[ax2+ay2*w].Index) + GetCon(&as, dir2)
					if chf.Areas[ai2] != area {
						continue
					}
					nr2 := srcReg[ai2]
					if nr2 != 0 && nr2 != r {
						ar = nr2
						break
					}
				}
			}
		}
		if ar != 0 {
			srcReg[ci] = 0
			continue
		}

		count++

		// Expand neighbours.
		for dir := 0; dir < 4; dir++ {
			if GetCon(&cs, dir) != NotConnected {
				ax := cx + DirOffsetX(dir)
				ay := cy + DirOffsetY(dir)
				ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&cs, dir)
				if chf.Areas[ai] != area {
					continue
				}
				if chf.Dist[ai] >= lev && srcReg[ai] == 0 {
					srcReg[ai] = r
					srcDist[ai] = 0
					*stack = append(*stack, levelStackEntry{ax, ay, ai})
				}
			}
		}
	}

	return count > 0
}

// expandRegions mirrors expandRegions (RecastRegion.cpp).
func expandRegions(maxIter int, level uint16, chf *CompactHeightfield,
	srcReg, srcDist []uint16, stack []levelStackEntry, fillStack bool) {
	w := chf.Width
	h := chf.Height

	if fillStack {
		// Find cells revealed by the raised level.
		stack = stack[:0]
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := chf.Cells[x+y*w]
				for i := int(c.Index); i < int(c.Index+c.Count); i++ {
					if chf.Dist[i] >= level && srcReg[i] == 0 && chf.Areas[i] != NullArea {
						stack = append(stack, levelStackEntry{x, y, i})
					}
				}
			}
		}
	} else { // use cells in the input stack
		// mark all cells which already have a region
		for j := 0; j < len(stack); j++ {
			i := stack[j].index
			if srcReg[i] != 0 {
				stack[j].index = -1
			}
		}
	}

	var dirtyEntries []dirtyEntry
	iter := 0
	for len(stack) > 0 {
		failed := 0
		dirtyEntries = dirtyEntries[:0]

		for j := 0; j < len(stack); j++ {
			x := stack[j].x
			y := stack[j].y
			i := stack[j].index
			if i < 0 {
				failed++
				continue
			}

			r := srcReg[i]
			d2 := uint16(0xffff)
			area := chf.Areas[i]
			s := chf.Spans[i]
			for dir := 0; dir < 4; dir++ {
				if GetCon(&s, dir) == NotConnected {
					continue
				}
				ax := x + DirOffsetX(dir)
				ay := y + DirOffsetY(dir)
				ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
				if chf.Areas[ai] != area {
					continue
				}
				if srcReg[ai] > 0 && (srcReg[ai]&BorderReg) == 0 {
					if int(srcDist[ai])+2 < int(d2) {
						r = srcReg[ai]
						d2 = srcDist[ai] + 2
					}
				}
			}
			if r != 0 {
				stack[j].index = -1 // mark as used
				dirtyEntries = append(dirtyEntries, dirtyEntry{i, r, d2})
			} else {
				failed++
			}
		}

		// Copy entries that differ between src and dst to keep them in sync.
		for i := 0; i < len(dirtyEntries); i++ {
			idx := dirtyEntries[i].index
			srcReg[idx] = dirtyEntries[i].region
			srcDist[idx] = dirtyEntries[i].distance2
		}

		if failed == len(stack) {
			break
		}

		if level > 0 {
			iter++
			if iter >= maxIter {
				break
			}
		}
	}
}

// sortCellsByLevel mirrors sortCellsByLevel (RecastRegion.cpp).
func sortCellsByLevel(startLevel int, chf *CompactHeightfield,
	srcReg []uint16, nbStacks int, stacks [][]levelStackEntry,
	loglevelsPerStack int) {
	w := chf.Width
	h := chf.Height
	startLevel = startLevel >> loglevelsPerStack

	for j := 0; j < nbStacks; j++ {
		stacks[j] = stacks[j][:0]
	}

	// put all cells in the level range into the appropriate stacks
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				if chf.Areas[i] == NullArea || srcReg[i] != 0 {
					continue
				}

				level := int(chf.Dist[i]) >> loglevelsPerStack
				sId := startLevel - level
				if sId >= nbStacks {
					continue
				}
				if sId < 0 {
					sId = 0
				}

				stacks[sId] = append(stacks[sId], levelStackEntry{x, y, i})
			}
		}
	}
}

// appendStacks mirrors appendStacks (RecastRegion.cpp).
func appendStacks(srcStack []levelStackEntry, dstStack []levelStackEntry,
	srcReg []uint16) []levelStackEntry {
	for j := 0; j < len(srcStack); j++ {
		i := srcStack[j].index
		if i < 0 || srcReg[i] != 0 {
			continue
		}
		dstStack = append(dstStack, srcStack[j])
	}
	return dstStack
}

// rcRegion mirrors rcRegion (RecastRegion.cpp).
type rcRegion struct {
	spanCount        int
	id               uint16
	areaType         uint8
	remap            bool
	visited          bool
	overlap          bool
	connectsToBorder bool
	ymin             uint16
	ymax             uint16
	connections      []int
	floors           []int
}

func newRcRegion(i uint16) rcRegion {
	return rcRegion{
		spanCount: 0,
		id:        i,
		areaType:  0,
		remap:     false,
		visited:   false,
		overlap:   false,
		ymin:      0xffff,
		ymax:      0,
	}
}

func removeAdjacentDuplicates(a *[]int) {
	for i := 0; i < len(*a) && len(*a) > 1; {
		ni := (i + 1) % len(*a)
		if (*a)[i] == (*a)[ni] {
			// Remove duplicate
			*a = append((*a)[:i], (*a)[i+1:]...)
		} else {
			i++
		}
	}
}

// removeAdjacentNeighbours mirrors removeAdjacentNeighbours
// (RecastRegion.cpp).
func removeAdjacentNeighbours(reg *rcRegion) {
	removeAdjacentDuplicates(&reg.connections)
}

// replaceNeighbour mirrors replaceNeighbour (RecastRegion.cpp).
func replaceNeighbour(reg *rcRegion, oldId, newId uint16) {
	neiChanged := false
	for i := 0; i < len(reg.connections); i++ {
		if reg.connections[i] == int(oldId) {
			reg.connections[i] = int(newId)
			neiChanged = true
		}
	}
	for i := 0; i < len(reg.floors); i++ {
		if reg.floors[i] == int(oldId) {
			reg.floors[i] = int(newId)
		}
	}
	if neiChanged {
		removeAdjacentNeighbours(reg)
	}
}

// canMergeWithRegion mirrors canMergeWithRegion (RecastRegion.cpp).
func canMergeWithRegion(rega, regb *rcRegion) bool {
	if rega.areaType != regb.areaType {
		return false
	}
	n := 0
	for i := 0; i < len(rega.connections); i++ {
		if rega.connections[i] == int(regb.id) {
			n++
		}
	}
	if n > 1 {
		return false
	}
	for i := 0; i < len(rega.floors); i++ {
		if rega.floors[i] == int(regb.id) {
			return false
		}
	}
	return true
}

// addUniqueFloorRegion mirrors addUniqueFloorRegion (RecastRegion.cpp).
func addUniqueFloorRegion(reg *rcRegion, n int) {
	for i := 0; i < len(reg.floors); i++ {
		if reg.floors[i] == n {
			return
		}
	}
	reg.floors = append(reg.floors, n)
}

// mergeRegions mirrors mergeRegions (RecastRegion.cpp).
func mergeRegions(rega, regb *rcRegion) bool {
	aid := rega.id
	bid := regb.id

	// Duplicate current neighbourhood.
	acon := make([]int, len(rega.connections))
	copy(acon, rega.connections)
	bcon := regb.connections

	// Find insertion point on A.
	insa := -1
	for i := 0; i < len(acon); i++ {
		if acon[i] == int(bid) {
			insa = i
			break
		}
	}
	if insa == -1 {
		return false
	}

	// Find insertion point on B.
	insb := -1
	for i := 0; i < len(bcon); i++ {
		if bcon[i] == int(aid) {
			insb = i
			break
		}
	}
	if insb == -1 {
		return false
	}

	// Merge neighbours.
	rega.connections = rega.connections[:0]
	for i, ni := 0, len(acon); i < ni-1; i++ {
		rega.connections = append(rega.connections, acon[(insa+1+i)%ni])
	}

	for i, ni := 0, len(bcon); i < ni-1; i++ {
		rega.connections = append(rega.connections, bcon[(insb+1+i)%ni])
	}

	removeAdjacentNeighbours(rega)

	for j := 0; j < len(regb.floors); j++ {
		addUniqueFloorRegion(rega, regb.floors[j])
	}
	rega.spanCount += regb.spanCount
	regb.spanCount = 0
	regb.connections = regb.connections[:0]

	return true
}

// isRegionConnectedToBorder mirrors isRegionConnectedToBorder
// (RecastRegion.cpp).
func isRegionConnectedToBorder(reg *rcRegion) bool {
	// Region is connected to border if
	// one of the neighbours is null id.
	for i := 0; i < len(reg.connections); i++ {
		if reg.connections[i] == 0 {
			return true
		}
	}
	return false
}

// isSolidEdge mirrors isSolidEdge (RecastRegion.cpp).
func isSolidEdge(chf *CompactHeightfield, srcReg []uint16,
	x, y, i, dir int) bool {
	s := chf.Spans[i]
	var r uint16
	if GetCon(&s, dir) != NotConnected {
		ax := x + DirOffsetX(dir)
		ay := y + DirOffsetY(dir)
		ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dir)
		r = srcReg[ai]
	}
	if r == srcReg[i] {
		return false
	}
	return true
}

// regionWalkContour mirrors walkContour (RecastRegion.cpp).
func regionWalkContour(x, y, i, dir int, chf *CompactHeightfield,
	srcReg []uint16, cont *[]int) {
	startDir := dir
	starti := i

	ss := chf.Spans[i]
	var curReg uint16
	if GetCon(&ss, dir) != NotConnected {
		ax := x + DirOffsetX(dir)
		ay := y + DirOffsetY(dir)
		ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&ss, dir)
		curReg = srcReg[ai]
	}
	*cont = append(*cont, int(curReg))

	iter := 0
	for {
		iter++
		if iter >= 40000 {
			break
		}
		s := chf.Spans[i]

		if isSolidEdge(chf, srcReg, x, y, i, dir) {
			// Choose the edge corner
			var r uint16
			if GetCon(&s, dir) != NotConnected {
				ax := x + DirOffsetX(dir)
				ay := y + DirOffsetY(dir)
				ai := int(chf.Cells[ax+ay*chf.Width].Index) + GetCon(&s, dir)
				r = srcReg[ai]
			}
			if r != curReg {
				curReg = r
				*cont = append(*cont, int(curReg))
			}

			dir = (dir + 1) & 0x3 // Rotate CW
		} else {
			ni := -1
			nx := x + DirOffsetX(dir)
			ny := y + DirOffsetY(dir)
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

	// Remove adjacent duplicates.
	if len(*cont) > 1 {
		removeAdjacentDuplicates(cont)
	}
}

// mergeAndFilterRegions mirrors mergeAndFilterRegions (RecastRegion.cpp).
func mergeAndFilterRegions(minRegionArea, mergeRegionSize int,
	maxRegionId *uint16, chf *CompactHeightfield,
	srcReg []uint16, overlaps *[]int) bool {
	w := chf.Width
	h := chf.Height

	nreg := int(*maxRegionId) + 1
	regions := make([]rcRegion, nreg)

	// Construct regions
	for i := 0; i < nreg; i++ {
		regions[i] = newRcRegion(uint16(i))
	}

	// Find edge of a region and find connections around the contour.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				r := srcReg[i]
				if r == 0 || int(r) >= nreg {
					continue
				}

				reg := &regions[r]
				reg.spanCount++

				// Update floors.
				for j := int(c.Index); j < int(c.Index+c.Count); j++ {
					if i == j {
						continue
					}
					floorId := srcReg[j]
					if floorId == 0 || int(floorId) >= nreg {
						continue
					}
					if floorId == r {
						reg.overlap = true
					}
					addUniqueFloorRegion(reg, int(floorId))
				}

				// Have found contour
				if len(reg.connections) > 0 {
					continue
				}

				reg.areaType = chf.Areas[i]

				// Check if this cell is next to a border.
				ndir := -1
				for dir := 0; dir < 4; dir++ {
					if isSolidEdge(chf, srcReg, x, y, i, dir) {
						ndir = dir
						break
					}
				}

				if ndir != -1 {
					// The cell is at border.
					// Walk around the contour to find all the neighbours.
					regionWalkContour(x, y, i, ndir, chf, srcReg, &reg.connections)
				}
			}
		}
	}

	// Remove too small regions.
	var stack []int
	var trace []int
	for i := 0; i < nreg; i++ {
		reg := &regions[i]
		if reg.id == 0 || (reg.id&BorderReg) != 0 {
			continue
		}
		if reg.spanCount == 0 {
			continue
		}
		if reg.visited {
			continue
		}

		// Count the total size of all the connected regions.
		// Also keep track of the regions connects to a tile border.
		connectsToBorder := false
		spanCount := 0
		stack = stack[:0]
		trace = trace[:0]

		reg.visited = true
		stack = append(stack, i)

		for len(stack) > 0 {
			// Pop
			ri := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			creg := &regions[ri]

			spanCount += creg.spanCount
			trace = append(trace, ri)

			for j := 0; j < len(creg.connections); j++ {
				if creg.connections[j]&BorderReg != 0 {
					connectsToBorder = true
					continue
				}
				neireg := &regions[creg.connections[j]]
				if neireg.visited {
					continue
				}
				if neireg.id == 0 || (neireg.id&BorderReg) != 0 {
					continue
				}
				// Visit
				stack = append(stack, int(neireg.id))
				neireg.visited = true
			}
		}

		// If the accumulated regions size is too small, remove it.
		// Do not remove areas which connect to tile borders
		// as their size cannot be estimated correctly and removing them
		// can potentially remove necessary areas.
		if spanCount < minRegionArea && !connectsToBorder {
			// Kill all visited regions.
			for j := 0; j < len(trace); j++ {
				regions[trace[j]].spanCount = 0
				regions[trace[j]].id = 0
			}
		}
	}

	// Merge too small regions to neighbour regions.
	mergeCount := 0
	for {
		mergeCount = 0
		for i := 0; i < nreg; i++ {
			reg := &regions[i]
			if reg.id == 0 || (reg.id&BorderReg) != 0 {
				continue
			}
			if reg.overlap {
				continue
			}
			if reg.spanCount == 0 {
				continue
			}

			// Check to see if the region should be merged.
			if reg.spanCount > mergeRegionSize && isRegionConnectedToBorder(reg) {
				continue
			}

			// Small region with more than 1 connection.
			// Or region which is not connected to a border at all.
			// Find smallest neighbour region that connects to this one.
			smallest := 0xfffffff
			mergeId := reg.id
			for j := 0; j < len(reg.connections); j++ {
				if reg.connections[j]&BorderReg != 0 {
					continue
				}
				mreg := &regions[reg.connections[j]]
				if mreg.id == 0 || (mreg.id&BorderReg) != 0 || mreg.overlap {
					continue
				}
				if mreg.spanCount < smallest &&
					canMergeWithRegion(reg, mreg) &&
					canMergeWithRegion(mreg, reg) {
					smallest = mreg.spanCount
					mergeId = mreg.id
				}
			}
			// Found new id.
			if mergeId != reg.id {
				oldId := reg.id
				target := &regions[mergeId]

				// Merge neighbours.
				if mergeRegions(target, reg) {
					// Fixup regions pointing to current region.
					for j := 0; j < nreg; j++ {
						if regions[j].id == 0 || (regions[j].id&BorderReg) != 0 {
							continue
						}
						// If another region was already merged into current region
						// change the nid of the previous region too.
						if regions[j].id == oldId {
							regions[j].id = mergeId
						}
						// Replace the current region with the new one if the
						// current regions is neighbour.
						replaceNeighbour(&regions[j], oldId, mergeId)
					}
					mergeCount++
				}
			}
		}
		if mergeCount == 0 {
			break
		}
	}

	// Compress region Ids.
	for i := 0; i < nreg; i++ {
		regions[i].remap = false
		if regions[i].id == 0 {
			continue // Skip nil regions.
		}
		if (regions[i].id & BorderReg) != 0 {
			continue // Skip external regions.
		}
		regions[i].remap = true
	}

	regIdGen := uint16(0)
	for i := 0; i < nreg; i++ {
		if !regions[i].remap {
			continue
		}
		oldId := regions[i].id
		regIdGen++
		newId := regIdGen
		for j := i; j < nreg; j++ {
			if regions[j].id == oldId {
				regions[j].id = newId
				regions[j].remap = false
			}
		}
	}
	*maxRegionId = regIdGen

	// Remap regions.
	for i := 0; i < chf.SpanCount; i++ {
		if (srcReg[i] & BorderReg) == 0 {
			srcReg[i] = regions[srcReg[i]].id
		}
	}

	// Return regions that we found to be overlapping.
	for i := 0; i < nreg; i++ {
		if regions[i].overlap {
			*overlaps = append(*overlaps, int(regions[i].id))
		}
	}

	return true
}

// addUniqueConnection mirrors addUniqueConnection (RecastRegion.cpp).
func addUniqueConnection(reg *rcRegion, n int) {
	for i := 0; i < len(reg.connections); i++ {
		if reg.connections[i] == n {
			return
		}
	}
	reg.connections = append(reg.connections, n)
}

// mergeAndFilterLayerRegions mirrors mergeAndFilterLayerRegions
// (RecastRegion.cpp).
func mergeAndFilterLayerRegions(minRegionArea int, maxRegionId *uint16,
	chf *CompactHeightfield, srcReg []uint16) bool {
	w := chf.Width
	h := chf.Height

	nreg := int(*maxRegionId) + 1
	regions := make([]rcRegion, nreg)

	// Construct regions
	for i := 0; i < nreg; i++ {
		regions[i] = newRcRegion(uint16(i))
	}

	// Find region neighbours and overlapping regions.
	var lregs []int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]

			lregs = lregs[:0]

			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]
				ri := srcReg[i]
				if ri == 0 || int(ri) >= nreg {
					continue
				}
				reg := &regions[ri]

				reg.spanCount++

				if s.Y < reg.ymin {
					reg.ymin = s.Y
				}
				if s.Y > reg.ymax {
					reg.ymax = s.Y
				}

				// Collect all region layers.
				lregs = append(lregs, int(ri))

				// Update neighbours
				for dir := 0; dir < 4; dir++ {
					if GetCon(&s, dir) != NotConnected {
						ax := x + DirOffsetX(dir)
						ay := y + DirOffsetY(dir)
						ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
						rai := srcReg[ai]
						if rai > 0 && int(rai) < nreg && rai != ri {
							addUniqueConnection(reg, int(rai))
						}
						if (rai & BorderReg) != 0 {
							reg.connectsToBorder = true
						}
					}
				}
			}

			// Update overlapping regions.
			for i := 0; i < len(lregs)-1; i++ {
				for j := i + 1; j < len(lregs); j++ {
					if lregs[i] != lregs[j] {
						ri := &regions[lregs[i]]
						rj := &regions[lregs[j]]
						addUniqueFloorRegion(ri, lregs[j])
						addUniqueFloorRegion(rj, lregs[i])
					}
				}
			}
		}
	}

	// Create 2D layers from regions.
	layerId := uint16(1)

	for i := 0; i < nreg; i++ {
		regions[i].id = 0
	}

	// Merge montone regions to create non-overlapping areas.
	var stack []int
	for i := 1; i < nreg; i++ {
		root := &regions[i]
		// Skip already visited.
		if root.id != 0 {
			continue
		}

		// Start search.
		root.id = layerId

		stack = stack[:0]
		stack = append(stack, i)

		for len(stack) > 0 {
			// Pop front
			reg := &regions[stack[0]]
			stack = append(stack[:0], stack[1:]...)

			ncons := len(reg.connections)
			for j := 0; j < ncons; j++ {
				nei := reg.connections[j]
				regn := &regions[nei]
				// Skip already visited.
				if regn.id != 0 {
					continue
				}
				// Skip if the neighbour is overlapping root region.
				overlap := false
				for k := 0; k < len(root.floors); k++ {
					if root.floors[k] == nei {
						overlap = true
						break
					}
				}
				if overlap {
					continue
				}

				// Deepen
				stack = append(stack, nei)

				// Mark layer id
				regn.id = layerId
				// Merge current layers to root.
				for k := 0; k < len(regn.floors); k++ {
					addUniqueFloorRegion(root, regn.floors[k])
				}
				if regn.ymin < root.ymin {
					root.ymin = regn.ymin
				}
				if regn.ymax > root.ymax {
					root.ymax = regn.ymax
				}
				root.spanCount += regn.spanCount
				regn.spanCount = 0
				root.connectsToBorder = root.connectsToBorder || regn.connectsToBorder
			}
		}

		layerId++
	}

	// Remove small regions
	for i := 0; i < nreg; i++ {
		if regions[i].spanCount > 0 && regions[i].spanCount < minRegionArea && !regions[i].connectsToBorder {
			reg := regions[i].id
			for j := 0; j < nreg; j++ {
				if regions[j].id == reg {
					regions[j].id = 0
				}
			}
		}
	}

	// Compress region Ids.
	for i := 0; i < nreg; i++ {
		regions[i].remap = false
		if regions[i].id == 0 {
			continue // Skip nil regions.
		}
		if (regions[i].id & BorderReg) != 0 {
			continue // Skip external regions.
		}
		regions[i].remap = true
	}

	regIdGen := uint16(0)
	for i := 0; i < nreg; i++ {
		if !regions[i].remap {
			continue
		}
		oldId := regions[i].id
		regIdGen++
		newId := regIdGen
		for j := i; j < nreg; j++ {
			if regions[j].id == oldId {
				regions[j].id = newId
				regions[j].remap = false
			}
		}
	}
	*maxRegionId = regIdGen

	// Remap regions.
	for i := 0; i < chf.SpanCount; i++ {
		if (srcReg[i] & BorderReg) == 0 {
			srcReg[i] = regions[srcReg[i]].id
		}
	}

	return true
}

// paintRectRegion mirrors paintRectRegion (RecastRegion.cpp).
func paintRectRegion(minx, maxx, miny, maxy int, regId uint16,
	chf *CompactHeightfield, srcReg []uint16) {
	w := chf.Width
	for y := miny; y < maxy; y++ {
		for x := minx; x < maxx; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				if chf.Areas[i] != NullArea {
					srcReg[i] = regId
				}
			}
		}
	}
}

// rcSweepSpan mirrors rcSweepSpan (RecastRegion.cpp).
type rcSweepSpan struct {
	rid uint16 // row id
	id  uint16 // region id
	ns  uint16 // number samples
	nei uint16 // neighbour id
}

// sweepRegions runs the shared row sweep of rcBuildRegionsMonotone and
// rcBuildLayerRegions (RecastRegion.cpp). It returns the next free region id
// with srcReg holding per-row ids; ids below rid are remapped per row.
func sweepRegions(chf *CompactHeightfield, borderSize int, id uint16,
	srcReg []uint16) uint16 {
	w := chf.Width
	h := chf.Height

	nsweeps := maxInt(chf.Width, chf.Height)
	sweeps := make([]rcSweepSpan, nsweeps)

	prev := make([]int, 256)

	// Sweep one line at a time.
	for y := borderSize; y < h-borderSize; y++ {
		// Collect spans from this row.
		if len(prev) < int(id)+1 {
			prev = make([]int, id+1)
		} else {
			for i := range prev {
				prev[i] = 0
			}
		}
		rid := uint16(1)

		for x := borderSize; x < w-borderSize; x++ {
			c := chf.Cells[x+y*w]

			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]
				if chf.Areas[i] == NullArea {
					continue
				}

				// -x
				var previd uint16
				if GetCon(&s, 0) != NotConnected {
					ax := x + DirOffsetX(0)
					ay := y + DirOffsetY(0)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 0)
					if (srcReg[ai]&BorderReg) == 0 && chf.Areas[i] == chf.Areas[ai] {
						previd = srcReg[ai]
					}
				}

				if previd == 0 {
					previd = rid
					rid++
					// C++ allocates max(w,h) sweep entries; grow if a row
					// holds more spans than that (multi-span cells).
					for int(previd) >= len(sweeps) {
						sweeps = append(sweeps, rcSweepSpan{})
					}
					sweeps[previd].rid = previd
					sweeps[previd].ns = 0
					sweeps[previd].nei = 0
				}

				// -y
				if GetCon(&s, 3) != NotConnected {
					ax := x + DirOffsetX(3)
					ay := y + DirOffsetY(3)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 3)
					if srcReg[ai] != 0 && (srcReg[ai]&BorderReg) == 0 && chf.Areas[i] == chf.Areas[ai] {
						nr := srcReg[ai]
						if sweeps[previd].nei == 0 || sweeps[previd].nei == nr {
							sweeps[previd].nei = nr
							sweeps[previd].ns++
							prev[nr]++
						} else {
							sweeps[previd].nei = nullNeighbour
						}
					}
				}

				srcReg[i] = previd
			}
		}

		// Create unique ID.
		for i := uint16(1); i < rid; i++ {
			if sweeps[i].nei != nullNeighbour && sweeps[i].nei != 0 &&
				prev[sweeps[i].nei] == int(sweeps[i].ns) {
				sweeps[i].id = sweeps[i].nei
			} else {
				sweeps[i].id = id
				id++
			}
		}

		// Remap IDs
		for x := borderSize; x < w-borderSize; x++ {
			c := chf.Cells[x+y*w]

			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				if srcReg[i] > 0 && srcReg[i] < rid {
					srcReg[i] = sweeps[srcReg[i]].id
				}
			}
		}
	}

	return id
}

// markBorderRegions paints the four border strips with border region ids
// (shared by rcBuildRegions, rcBuildRegionsMonotone and
// rcBuildLayerRegions in RecastRegion.cpp). It returns the next free id.
func markBorderRegions(chf *CompactHeightfield, borderSize int,
	id uint16, srcReg []uint16) uint16 {
	w := chf.Width
	h := chf.Height
	if borderSize > 0 {
		// Make sure border will not overflow.
		bw := minInt(w, borderSize)
		bh := minInt(h, borderSize)
		// Paint regions
		paintRectRegion(0, bw, 0, h, id|BorderReg, chf, srcReg)
		id++
		paintRectRegion(w-bw, w, 0, h, id|BorderReg, chf, srcReg)
		id++
		paintRectRegion(0, w, 0, bh, id|BorderReg, chf, srcReg)
		id++
		paintRectRegion(0, w, h-bh, h, id|BorderReg, chf, srcReg)
		id++
	}
	return id
}

// BuildRegionsMonotone mirrors rcBuildRegionsMonotone (RecastRegion.cpp).
// The distance field must be created with BuildDistanceField first.
func BuildRegionsMonotone(chf *CompactHeightfield, borderSize, minRegionArea, mergeRegionArea int) bool {
	var id uint16 = 1

	srcReg := make([]uint16, chf.SpanCount)

	// Mark border regions.
	id = markBorderRegions(chf, borderSize, id, srcReg)

	chf.BorderSize = borderSize

	id = sweepRegions(chf, borderSize, id, srcReg)

	// Merge regions and filter out small regions.
	var overlaps []int
	chf.MaxRegions = id
	if !mergeAndFilterRegions(minRegionArea, mergeRegionArea, &chf.MaxRegions, chf, srcReg, &overlaps) {
		return false
	}

	// Monotone partitioning does not generate overlapping regions.

	// Store the result out.
	for i := 0; i < chf.SpanCount; i++ {
		chf.Spans[i].Reg = srcReg[i]
	}

	return true
}

// BuildRegions mirrors rcBuildRegions (RecastRegion.cpp, watershed
// partitioning). The distance field must be created with
// BuildDistanceField first.
func BuildRegions(chf *CompactHeightfield, borderSize, minRegionArea, mergeRegionArea int) bool {
	const logNbStacks = 3
	const nbStacks = 1 << logNbStacks
	lvlStacks := make([][]levelStackEntry, nbStacks)
	for i := 0; i < nbStacks; i++ {
		lvlStacks[i] = make([]levelStackEntry, 0, 256)
	}

	var stack []levelStackEntry

	srcReg := make([]uint16, chf.SpanCount)
	srcDist := make([]uint16, chf.SpanCount)

	var regionId uint16 = 1
	level := (int(chf.MaxDistance) + 1) &^ 1

	// TODO: Figure better formula, expandIters defines how much the
	// watershed "overflows" and simplifies the regions. Tying it to
	// agent radius was usually good indication how greedy it could be.
	//	const int expandIters = 4 + walkableRadius * 2;
	const expandIters = 8

	// Mark border regions.
	regionId = markBorderRegions(chf, borderSize, regionId, srcReg)

	chf.BorderSize = borderSize

	sId := -1
	for level > 0 {
		if level >= 2 {
			level -= 2
		} else {
			level = 0
		}
		sId = (sId + 1) & (nbStacks - 1)

		if sId == 0 {
			sortCellsByLevel(level, chf, srcReg, nbStacks, lvlStacks, 1)
		} else {
			// copy left overs from last level
			lvlStacks[sId] = appendStacks(lvlStacks[sId-1], lvlStacks[sId], srcReg)
		}

		// Expand current regions until no empty connected cells found.
		expandRegions(expandIters, uint16(level), chf, srcReg, srcDist, lvlStacks[sId], false)

		// Mark new regions with IDs.
		for j := 0; j < len(lvlStacks[sId]); j++ {
			current := lvlStacks[sId][j]
			x := current.x
			y := current.y
			i := current.index
			if i >= 0 && srcReg[i] == 0 {
				if floodRegion(x, y, i, uint16(level), regionId, chf, srcReg, srcDist, &stack) {
					if regionId == 0xFFFF {
						return false
					}

					regionId++
				}
			}
		}
	}

	// Expand current regions until no empty connected cells found.
	expandRegions(expandIters*8, 0, chf, srcReg, srcDist, stack, true)

	// Merge regions and filter out small regions.
	var overlaps []int
	chf.MaxRegions = regionId
	if !mergeAndFilterRegions(minRegionArea, mergeRegionArea, &chf.MaxRegions, chf, srcReg, &overlaps) {
		return false
	}

	// If overlapping regions were found during merging, split those regions.
	if len(overlaps) > 0 {
		// C++ logs "rcBuildRegions: %d overlapping regions."; the
		// regions are left as merged (no split step exists upstream).
		_ = overlaps
	}

	// Write the result out.
	for i := 0; i < chf.SpanCount; i++ {
		chf.Spans[i].Reg = srcReg[i]
	}

	return true
}

// BuildLayerRegions mirrors rcBuildLayerRegions (RecastRegion.cpp).
func BuildLayerRegions(chf *CompactHeightfield, borderSize, minRegionArea int) bool {
	var id uint16 = 1

	srcReg := make([]uint16, chf.SpanCount)

	// Mark border regions.
	id = markBorderRegions(chf, borderSize, id, srcReg)

	chf.BorderSize = borderSize

	id = sweepRegions(chf, borderSize, id, srcReg)

	// Merge monotone regions to layers and remove small regions.
	chf.MaxRegions = id
	if !mergeAndFilterLayerRegions(minRegionArea, &chf.MaxRegions, chf, srcReg) {
		return false
	}

	// Store the result out.
	for i := 0; i < chf.SpanCount; i++ {
		chf.Spans[i].Reg = srcReg[i]
	}

	return true
}

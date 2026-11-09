package recast

// Area processing port (RecastArea.cpp erode/median, RecastRegion.cpp
// distance field).

// ErodeWalkableArea mirrors rcErodeWalkableArea (RecastArea.cpp).
func ErodeWalkableArea(radius int, chf *CompactHeightfield) bool {
	w := chf.Width
	h := chf.Height

	dist := make([]uint8, chf.SpanCount)
	for i := range dist {
		dist[i] = 0xff
	}

	// Mark boundary cells.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				if chf.Areas[i] == NullArea {
					dist[i] = 0
				} else {
					s := chf.Spans[i]
					nc := 0
					for dir := 0; dir < 4; dir++ {
						if GetCon(&s, dir) != NotConnected {
							nx := x + DirOffsetX(dir)
							ny := y + DirOffsetY(dir)
							nidx := int(chf.Cells[nx+ny*w].Index) + GetCon(&s, dir)
							if chf.Areas[nidx] != NullArea {
								nc++
							}
						}
					}
					// At least one missing neighbour.
					if nc != 4 {
						dist[i] = 0
					}
				}
			}
		}
	}

	// Pass 1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]

				if GetCon(&s, 0) != NotConnected {
					// (-1,0)
					ax := x + DirOffsetX(0)
					ay := y + DirOffsetY(0)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 0)
					as := chf.Spans[ai]
					nd := uint8(minInt(int(dist[ai])+2, 255))
					if nd < dist[i] {
						dist[i] = nd
					}

					// (-1,-1)
					if GetCon(&as, 3) != NotConnected {
						aax := ax + DirOffsetX(3)
						aay := ay + DirOffsetY(3)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 3)
						nd = uint8(minInt(int(dist[aai])+3, 255))
						if nd < dist[i] {
							dist[i] = nd
						}
					}
				}
				if GetCon(&s, 3) != NotConnected {
					// (0,-1)
					ax := x + DirOffsetX(3)
					ay := y + DirOffsetY(3)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 3)
					as := chf.Spans[ai]
					nd := uint8(minInt(int(dist[ai])+2, 255))
					if nd < dist[i] {
						dist[i] = nd
					}

					// (1,-1)
					if GetCon(&as, 2) != NotConnected {
						aax := ax + DirOffsetX(2)
						aay := ay + DirOffsetY(2)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 2)
						nd = uint8(minInt(int(dist[aai])+3, 255))
						if nd < dist[i] {
							dist[i] = nd
						}
					}
				}
			}
		}
	}

	// Pass 2
	for y := h - 1; y >= 0; y-- {
		for x := w - 1; x >= 0; x-- {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]

				if GetCon(&s, 2) != NotConnected {
					// (1,0)
					ax := x + DirOffsetX(2)
					ay := y + DirOffsetY(2)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 2)
					as := chf.Spans[ai]
					nd := uint8(minInt(int(dist[ai])+2, 255))
					if nd < dist[i] {
						dist[i] = nd
					}

					// (1,1)
					if GetCon(&as, 1) != NotConnected {
						aax := ax + DirOffsetX(1)
						aay := ay + DirOffsetY(1)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 1)
						nd = uint8(minInt(int(dist[aai])+3, 255))
						if nd < dist[i] {
							dist[i] = nd
						}
					}
				}
				if GetCon(&s, 1) != NotConnected {
					// (0,1)
					ax := x + DirOffsetX(1)
					ay := y + DirOffsetY(1)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 1)
					as := chf.Spans[ai]
					nd := uint8(minInt(int(dist[ai])+2, 255))
					if nd < dist[i] {
						dist[i] = nd
					}

					// (-1,1)
					if GetCon(&as, 0) != NotConnected {
						aax := ax + DirOffsetX(0)
						aay := ay + DirOffsetY(0)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 0)
						nd = uint8(minInt(int(dist[aai])+3, 255))
						if nd < dist[i] {
							dist[i] = nd
						}
					}
				}
			}
		}
	}

	thr := uint8(radius * 2)
	for i := 0; i < chf.SpanCount; i++ {
		if dist[i] < thr {
			chf.Areas[i] = NullArea
		}
	}

	return true
}

func insertSort(a []uint8) {
	for i := 1; i < len(a); i++ {
		value := a[i]
		j := i - 1
		for j >= 0 && a[j] > value {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = value
	}
}

// MedianFilterWalkableArea mirrors rcMedianFilterWalkableArea (RecastArea.cpp).
func MedianFilterWalkableArea(chf *CompactHeightfield) bool {
	w := chf.Width
	h := chf.Height

	areas := make([]uint8, chf.SpanCount)
	for i := range areas {
		areas[i] = 0xff
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]
				if chf.Areas[i] == NullArea {
					areas[i] = chf.Areas[i]
					continue
				}

				var nei [9]uint8
				for j := 0; j < 9; j++ {
					nei[j] = chf.Areas[i]
				}

				for dir := 0; dir < 4; dir++ {
					if GetCon(&s, dir) != NotConnected {
						ax := x + DirOffsetX(dir)
						ay := y + DirOffsetY(dir)
						ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
						if chf.Areas[ai] != NullArea {
							nei[dir*2+0] = chf.Areas[ai]
						}

						as := chf.Spans[ai]
						dir2 := (dir + 1) & 0x3
						if GetCon(&as, dir2) != NotConnected {
							ax2 := ax + DirOffsetX(dir2)
							ay2 := ay + DirOffsetY(dir2)
							ai2 := int(chf.Cells[ax2+ay2*w].Index) + GetCon(&as, dir2)
							if chf.Areas[ai2] != NullArea {
								nei[dir*2+1] = chf.Areas[ai2]
							}
						}
					}
				}
				insertSort(nei[:])
				areas[i] = nei[4]
			}
		}
	}

	copy(chf.Areas, areas)

	return true
}

func calculateDistanceField(chf *CompactHeightfield, src []uint16) uint16 {
	w := chf.Width
	h := chf.Height

	// Init distance and points.
	for i := 0; i < chf.SpanCount; i++ {
		src[i] = 0xffff
	}

	// Mark boundary cells.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]
				area := chf.Areas[i]

				nc := 0
				for dir := 0; dir < 4; dir++ {
					if GetCon(&s, dir) != NotConnected {
						ax := x + DirOffsetX(dir)
						ay := y + DirOffsetY(dir)
						ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
						if area == chf.Areas[ai] {
							nc++
						}
					}
				}
				if nc != 4 {
					src[i] = 0
				}
			}
		}
	}

	// Pass 1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]

				if GetCon(&s, 0) != NotConnected {
					// (-1,0)
					ax := x + DirOffsetX(0)
					ay := y + DirOffsetY(0)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 0)
					as := chf.Spans[ai]
					if int(src[ai])+2 < int(src[i]) {
						src[i] = uint16(int(src[ai]) + 2)
					}

					// (-1,-1)
					if GetCon(&as, 3) != NotConnected {
						aax := ax + DirOffsetX(3)
						aay := ay + DirOffsetY(3)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 3)
						if int(src[aai])+3 < int(src[i]) {
							src[i] = uint16(int(src[aai]) + 3)
						}
					}
				}
				if GetCon(&s, 3) != NotConnected {
					// (0,-1)
					ax := x + DirOffsetX(3)
					ay := y + DirOffsetY(3)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 3)
					as := chf.Spans[ai]
					if int(src[ai])+2 < int(src[i]) {
						src[i] = uint16(int(src[ai]) + 2)
					}

					// (1,-1)
					if GetCon(&as, 2) != NotConnected {
						aax := ax + DirOffsetX(2)
						aay := ay + DirOffsetY(2)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 2)
						if int(src[aai])+3 < int(src[i]) {
							src[i] = uint16(int(src[aai]) + 3)
						}
					}
				}
			}
		}
	}

	// Pass 2
	for y := h - 1; y >= 0; y-- {
		for x := w - 1; x >= 0; x-- {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]

				if GetCon(&s, 2) != NotConnected {
					// (1,0)
					ax := x + DirOffsetX(2)
					ay := y + DirOffsetY(2)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 2)
					as := chf.Spans[ai]
					if int(src[ai])+2 < int(src[i]) {
						src[i] = uint16(int(src[ai]) + 2)
					}

					// (1,1)
					if GetCon(&as, 1) != NotConnected {
						aax := ax + DirOffsetX(1)
						aay := ay + DirOffsetY(1)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 1)
						if int(src[aai])+3 < int(src[i]) {
							src[i] = uint16(int(src[aai]) + 3)
						}
					}
				}
				if GetCon(&s, 1) != NotConnected {
					// (0,1)
					ax := x + DirOffsetX(1)
					ay := y + DirOffsetY(1)
					ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, 1)
					as := chf.Spans[ai]
					if int(src[ai])+2 < int(src[i]) {
						src[i] = uint16(int(src[ai]) + 2)
					}

					// (-1,1)
					if GetCon(&as, 0) != NotConnected {
						aax := ax + DirOffsetX(0)
						aay := ay + DirOffsetY(0)
						aai := int(chf.Cells[aax+aay*w].Index) + GetCon(&as, 0)
						if int(src[aai])+3 < int(src[i]) {
							src[i] = uint16(int(src[aai]) + 3)
						}
					}
				}
			}
		}
	}

	var maxDist uint16
	for i := 0; i < chf.SpanCount; i++ {
		if src[i] > maxDist {
			maxDist = src[i]
		}
	}
	return maxDist
}

func boxBlur(chf *CompactHeightfield, thr int, src, dst []uint16) []uint16 {
	w := chf.Width
	h := chf.Height

	thr *= 2

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := chf.Cells[x+y*w]
			for i := int(c.Index); i < int(c.Index+c.Count); i++ {
				s := chf.Spans[i]
				cd := src[i]
				if int(cd) <= thr {
					dst[i] = cd
					continue
				}

				d := int(cd)
				for dir := 0; dir < 4; dir++ {
					if GetCon(&s, dir) != NotConnected {
						ax := x + DirOffsetX(dir)
						ay := y + DirOffsetY(dir)
						ai := int(chf.Cells[ax+ay*w].Index) + GetCon(&s, dir)
						d += int(src[ai])

						as := chf.Spans[ai]
						dir2 := (dir + 1) & 0x3
						if GetCon(&as, dir2) != NotConnected {
							ax2 := ax + DirOffsetX(dir2)
							ay2 := ay + DirOffsetY(dir2)
							ai2 := int(chf.Cells[ax2+ay2*w].Index) + GetCon(&as, dir2)
							d += int(src[ai2])
						} else {
							d += int(cd)
						}
					} else {
						d += int(cd) * 2
					}
				}
				dst[i] = uint16((d + 5) / 9)
			}
		}
	}
	return dst
}

// BuildDistanceField mirrors rcBuildDistanceField (RecastRegion.cpp).
func BuildDistanceField(chf *CompactHeightfield) bool {
	src := make([]uint16, chf.SpanCount)
	dst := make([]uint16, chf.SpanCount)

	maxDist := calculateDistanceField(chf, src)
	chf.MaxDistance = maxDist

	// Blur (C++ swaps src/dst after the blur and stores src; net effect is
	// chf.dist holds the blurred field).
	chf.Dist = boxBlur(chf, 1, src, dst)

	return true
}

package recast

// Filters port (RecastFilter.cpp).

// FilterLowHangingWalkableObstacles mirrors
// rcFilterLowHangingWalkableObstacles.
func FilterLowHangingWalkableObstacles(walkableClimb int, hf *Heightfield) {
	w := hf.Width
	h := hf.Height

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var ps *Span
			previousWalkable := false
			previousArea := uint8(NullArea)

			for s := hf.Spans[x+y*w]; s != nil; ps, s = s, s.Next {
				walkable := s.Area != NullArea
				// If current span is not walkable, but there is walkable
				// span just below it, mark the span above it walkable too.
				if !walkable && previousWalkable {
					if absInt(int(s.Smax)-int(ps.Smax)) <= walkableClimb {
						s.Area = previousArea
					}
				}
				// Copy walkable flag so that it cannot propagate
				// past multiple non-walkable objects.
				previousWalkable = walkable
				previousArea = s.Area
			}
		}
	}
}

// FilterLedgeSpans mirrors rcFilterLedgeSpans.
func FilterLedgeSpans(walkableHeight, walkableClimb int, hf *Heightfield) {
	w := hf.Width
	h := hf.Height
	const maxHeight = 0xffff

	// Mark border spans.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for s := hf.Spans[x+y*w]; s != nil; s = s.Next {
				// Skip non walkable spans.
				if s.Area == NullArea {
					continue
				}

				bot := int(s.Smax)
				top := maxHeight
				if s.Next != nil {
					top = int(s.Next.Smin)
				}

				// Find neighbours minimum height.
				minh := maxHeight

				// Min and max height of accessible neighbours.
				asmin := int(s.Smax)
				asmax := int(s.Smax)

				for dir := 0; dir < 4; dir++ {
					dx := x + DirOffsetX(dir)
					dy := y + DirOffsetY(dir)
					// Skip neighbours which are out of bounds.
					if dx < 0 || dy < 0 || dx >= w || dy >= h {
						minh = minInt(minh, -walkableClimb-bot)
						continue
					}

					// From minus infinity to the first span.
					ns := hf.Spans[dx+dy*w]
					nbot := -walkableClimb
					ntop := maxHeight
					if ns != nil {
						ntop = int(ns.Smin)
					}
					// Skip neighbour if the gap between the spans is too small.
					if minInt(top, ntop)-maxInt(bot, nbot) > walkableHeight {
						minh = minInt(minh, nbot-bot)
					}

					// Rest of the spans.
					for ns = hf.Spans[dx+dy*w]; ns != nil; ns = ns.Next {
						nbot = int(ns.Smax)
						if ns.Next != nil {
							ntop = int(ns.Next.Smin)
						} else {
							ntop = maxHeight
						}
						// Skip neighbour if the gap between the spans is too small.
						if minInt(top, ntop)-maxInt(bot, nbot) > walkableHeight {
							minh = minInt(minh, nbot-bot)

							// Find min/max accessible neighbour height.
							if absInt(nbot-bot) <= walkableClimb {
								if nbot < asmin {
									asmin = nbot
								}
								if nbot > asmax {
									asmax = nbot
								}
							}
						}
					}
				}

				// The current span is close to a ledge if the drop to any
				// neighbour span is less than the walkableClimb.
				if minh < -walkableClimb {
					s.Area = NullArea
				} else if (asmax - asmin) > walkableClimb {
					// If the difference between all neighbours is too large,
					// we are at steep slope, mark the span as ledge.
					s.Area = NullArea
				}
			}
		}
	}
}

// FilterWalkableLowHeightSpans mirrors rcFilterWalkableLowHeightSpans.
func FilterWalkableLowHeightSpans(walkableHeight int, hf *Heightfield) {
	w := hf.Width
	h := hf.Height
	const maxHeight = 0xffff

	// Remove walkable flag from spans which do not have enough
	// space above them for the agent to stand there.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for s := hf.Spans[x+y*w]; s != nil; s = s.Next {
				bot := int(s.Smax)
				top := maxHeight
				if s.Next != nil {
					top = int(s.Next.Smin)
				}
				if (top - bot) <= walkableHeight {
					s.Area = NullArea
				}
			}
		}
	}
}

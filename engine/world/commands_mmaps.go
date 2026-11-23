package world

import (
	"context"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/detour"
)

// mmaps command port: mmaps_commandscript (cs_mmaps.cpp), the "mmap" root
// with 5 arms in C++ table order (loadedtiles, loc, path, stats, testarea).
// TWENTY-SEVENTH of 40 Commands groups (cs_script_loader.cpp decl 45 /
// call 90; call order re-verified this run: misc(89) -> mmaps(90) ->
// modify(91)). The "mmap" root is a pure container in C++: the deprecated
// 6-arg ChatCommandBuilder overload (ChatCommand.h:260-263) drops the
// RBAC_PERM_COMMAND_MMAP 536 permission and the console flag, keeping only
// the sub-table, so the root permission is dead and bare ".mmap" prints
// the permission-filtered help listing with no gate; the Go tree follows
// the audited gobject container-root convention and prints the syntax
// line. Trinity checks permission only on the invoker leaf node, so each
// arm gates exactly its own C++ permission (RBAC.h:405-409).
//
// All 5 arms are native:
//
//   - `loadedtiles` is native: the manager's navmesh/query gate and the
//     per-slot tile scan (cs_mmaps.cpp:189-212) run over Server.mmaps.
//   - `loc` is native: the grid tile location math (cs_mmaps.cpp:133-134)
//     plus the navmesh tile location (Calc from the mesh params orig,
//     findNearestPoly, getTileAndPolyByRef, cs_mmaps.cpp:140-187) run over
//     Server.mmaps and the Detour query port.
//   - `stats` is native: the header, pathfinding, manager-counts, and
//     per-tile dtNavMesh aggregate lines (cs_mmaps.cpp:214-260) run over
//     Server.mmaps; a map with no mesh loaded still reports exactly
//     "NavMesh not loaded for current map." like C++.
//   - `path` is native: the navmesh gate (cs_mmaps.cpp:67-71) then a
//     PathGenerator from the selected unit to the player
//     (pathgenerator.go), with the exact C++ message lines; only the
//     VISUAL_WAYPOINT summon per point (:120-121) has no Go bridge (no
//     command-layer temp-summon spawns at an arbitrary position).
//   - `testarea` is native: the creature range search runs over the live
//     motion map (same map + instance, alive only, the
//     AnyUnitInObjectRangeCheck terms — phase-mask overlap, 40yd 3D plus
//     both combat reaches, strict < on squared distance,
//     GridNotifiers.h:1050-1055), then one real PathGenerator per
//     creature (cs_mmaps.cpp:282-287) over the measured sweep time.
//
// Console-vs-chat branches are moot (Go commands are always sessioned).
//
// R13: the root and all 5 arms return bool — bare/unknown subcommands return
// false (the framework shows the help listing), denial is a silent false
// (the C++ framework denies before invoking the handler), and every
// message leg returns true.

// handleCmdMMap dispatches the "mmap" root (arm table cs_mmaps.cpp:51-55,
// root table :59-62). Bare ".mmap" returns false: the container root node
// carries no permission in C++ (the 536 perm is dropped by the deprecated
// ChatCommandBuilder overload) and has no handler, so the framework shows
// the help listing.
func (s *session) handleCmdMMap(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here, over the invoker-visible
	// arms only (FilteredCommandListIterator, ChatCommand.cpp:207-237).
	// ".mmap l" / ".mmap lo" are ambiguous (loadedtiles/loc) and list
	// both, like C++, when both are visible.
	visible := func(perm uint32) bool { return s.commandAllowed(ctx, perm) }
	sub, cands, ok := groupMatchNameVisible(args[0], []groupSubcommand{
		{"loadedtiles", []uint32{permissionCommandMMapLoadedTiles}},
		{"loc", []uint32{permissionCommandMMapLoc}},
		{"path", []uint32{permissionCommandMMapPath}},
		{"stats", []uint32{permissionCommandMMapStats}},
		{"testarea", []uint32{permissionCommandMMapTestArea}},
	}, visible)
	if !ok {
		if len(cands) > 1 {
			s.groupAmbiguousMessage(args[0], cands)
			return true
		}
		return false
	}
	switch sub {
	case "loadedtiles":
		return s.handleMmapLoadedTilesCommand(ctx)
	case "loc":
		return s.handleMmapLocCommand(ctx)
	case "path":
		return s.handleMmapPathCommand(ctx, args[1:])
	case "stats":
		return s.handleMmapStatsCommand(ctx)
	case "testarea":
		return s.handleMmapTestAreaCommand(ctx)
	}
	return false
}

// handleMmapLoadedTilesCommand mirrors HandleMmapLoadedTilesCommand
// (cs_mmaps.cpp:189-212, RBAC 537): the navmesh/query gate, then one
// "[%02i, %02i]" line per loaded tile slot (getMaxTiles slots, skipping
// empty ones), over Server.mmaps.
func (s *session) handleMmapLoadedTilesCommand(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMMapLoadedTiles) {
		return false
	}
	// The C++ handler (cs_mmaps.cpp:196-201) reports the navmesh as
	// missing when either the mesh or the per-instance query is absent.
	mgr := s.server.mmapManager()
	navmesh := mgr.GetNavMesh(s.player.Map)
	navmeshquery := mgr.GetNavMeshQuery(s.player.Map, s.player.InstanceID)
	if navmesh == nil || navmeshquery == nil {
		s.sendSysMessage("NavMesh not loaded for current map.")
		return true
	}
	s.sendSysMessage("mmap loadedtiles:")
	for i := 0; i < navmesh.GetMaxTiles(); i++ {
		tile := navmesh.GetTile(i)
		if tile == nil || tile.Header == nil {
			continue
		}
		s.sendSysMessage(fmt.Sprintf("[%02d, %02d]", tile.Header.X, tile.Header.Y))
	}
	return true
}

// handleMmapLocCommand mirrors HandleMmapLocCommand (cs_mmaps.cpp:126-187,
// RBAC 538): the grid tile location is native math; the navmesh tile
// location runs over Server.mmaps and the Detour query port.
func (s *session) handleMmapLocCommand(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMMapLoc) {
		return false
	}
	s.sendSysMessage("mmap tileloc:")
	// int32 gx = 32 - player->GetPositionX() / SIZE_OF_GRIDS (cs_mmaps.cpp:
	// 133-134); goSizeOfGrids mirrors SIZE_OF_GRIDS (GridDefines.h:38),
	// already used by the .go grid arm (commands.go:1923-1924).
	gx := int32(32 - s.player.X/goSizeOfGrids)
	gy := int32(32 - s.player.Y/goSizeOfGrids)
	s.sendSysMessage(fmt.Sprintf("%03d%02d%02d.mmtile", s.player.Map, gx, gy))
	s.sendSysMessage(fmt.Sprintf("tileloc [%d, %d]", gy, gx))
	// The navmesh tile location (cs_mmaps.cpp:140-187) runs over
	// Server.mmaps and the Detour query port.
	mgr := s.server.mmapManager()
	navmesh := mgr.GetNavMesh(s.player.Map)
	navmeshquery := mgr.GetNavMeshQuery(s.player.Map, s.player.InstanceID)
	if navmesh == nil || navmeshquery == nil {
		// Exactly like the C++ handler (cs_mmaps.cpp:144).
		s.sendSysMessage("NavMesh not loaded for current map.")
		return true
	}
	// float const* min = navmesh->getParams()->orig (cs_mmaps.cpp:149);
	// tilex/tiley divide the Detour-space coords (y - min[0], x - min[2])
	// by SIZE_OF_GRIDS (:156-157).
	min := navmesh.GetParams().Orig
	tilex := int32((s.player.Y - min[0]) / goSizeOfGrids)
	tiley := int32((s.player.X - min[2]) / goSizeOfGrids)
	s.sendSysMessage(fmt.Sprintf("Calc   [%02d, %02d]", tilex, tiley))
	// float location[3] = {y, z, x} (cs_mmaps.cpp:153): Detour axes are
	// (WoW y, WoW z, WoW x). Extents {3, 5, 3} (:154); the default
	// dtQueryFilter (:160) is NewQueryFilter.
	location := detour.Vec3{s.player.Y, s.player.Z, s.player.X}
	extents := detour.Vec3{3.0, 5.0, 3.0}
	polyRef, _, st := navmeshquery.FindNearestPoly(location, extents, detour.NewQueryFilter())
	if detour.StatusFailed(st) {
		s.sendSysMessage("Dt     [??,??] (invalid poly, probably no tile loaded)")
		return true
	}
	if polyRef == 0 { // INVALID_POLYREF (cs_mmaps.cpp:167-168)
		s.sendSysMessage("Dt     [??, ??] (invalid poly, probably no tile loaded)")
		return true
	}
	tile, _, st := navmesh.GetTileAndPolyByRef(polyRef)
	if detour.StatusSucceed(st) && tile != nil {
		s.sendSysMessage(fmt.Sprintf("Dt     [%02d,%02d]", tile.Header.X, tile.Header.Y))
		return true
	}
	s.sendSysMessage("Dt     [??,??] (no tile loaded)")
	return true
}

// handleMmapPathCommand mirrors HandleMmapPathCommand (cs_mmaps.cpp:
// 65-124, RBAC 539): the navmesh gate, then a PathGenerator from the
// selected unit (the getSelectedUnit twin: debugSelectedUnit falls back
// to the invoker, Chat.cpp:312-321, so the C++ "Invalid target/source
// selection" leg is unreachable for a sessioned player) to the player's
// position, with the exact C++ message lines. The VISUAL_WAYPOINT summon
// per point (cs_mmaps.cpp:120-121) has no Go bridge: no command-layer
// temp-summon spawns a creature at an arbitrary position with a 9s
// despawn, so the markers are not summoned; every text line is native.
func (s *session) handleMmapPathCommand(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandMMapPath) {
		return false
	}
	if s.server == nil || s.player == nil {
		s.sendSysMessage("NavMesh not loaded for current map.")
		return true
	}
	if s.server.mmapManager().GetNavMesh(s.player.Map) == nil {
		s.sendSysMessage("NavMesh not loaded for current map.")
		return true
	}

	s.sendSysMessage("mmap path:")

	targetName, source := s.mmapPathTarget()

	useStraightPath := false
	useRaycast := false
	if len(args) > 0 {
		switch args[0] {
		case "true":
			useStraightPath = true
		case "line", "ray", "raycast":
			useRaycast = true
		}
	}

	path := NewPathGenerator(s.server, source)
	path.SetUseStraightPath(useStraightPath)
	path.SetUseRaycast(useRaycast)
	result := path.CalculatePath(s.player.X, s.player.Y, s.player.Z, false)

	pointPath := path.PathPoints()
	s.sendSysMessage(fmt.Sprintf("%s's path to %s:", targetName, s.player.Name))
	building := "SmoothPath"
	if useStraightPath {
		building = "StraightPath"
	} else if useRaycast {
		building = "Raycast"
	}
	s.sendSysMessage(fmt.Sprintf("Building: %s", building))
	resultStr := "false"
	if result {
		resultStr = "true"
	}
	s.sendSysMessage(fmt.Sprintf("Result: %s - Length: %d - Type: %d", resultStr, len(pointPath), uint32(path.PathType())))
	start := path.StartPosition()
	end := path.EndPosition()
	actualEnd := path.ActualEndPosition()
	s.sendSysMessage(fmt.Sprintf("StartPosition     (%.3f, %.3f, %.3f)", start.X, start.Y, start.Z))
	s.sendSysMessage(fmt.Sprintf("EndPosition       (%.3f, %.3f, %.3f)", end.X, end.Y, end.Z))
	s.sendSysMessage(fmt.Sprintf("ActualEndPosition (%.3f, %.3f, %.3f)", actualEnd.X, actualEnd.Y, actualEnd.Z))

	if !sessionIsGameMaster(s) {
		s.sendSysMessage("Enable GM mode to see the path points.")
	}
	return true
}

// mmapPathTarget resolves the .mmap path source the getSelectedUnit way
// (Chat.cpp:312-321): the selected creature across instances, else the
// selected player, else the invoker.
func (s *session) mmapPathTarget() (string, pathGeneratorSource) {
	if s.selection != 0 && s.server != nil {
		if m := s.debugSelectedCreatureMotion(); m != nil {
			return m.Name, newCreaturePathSource(s.server, m)
		}
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts.player.Name, newPlayerPathSource(ts)
		}
	}
	return s.player.Name, newPlayerPathSource(s)
}

// mmapPathfindingEnabled mirrors DisableMgr::IsPathfindingEnabled
// (DisableMgr.cpp:418-422) via the server-level check (mmap.go): the
// PathGenerator constructor gates on the same check
// (PathGenerator.cpp:41-46). For MMAP disables IsDisabledFor returns
// true on the entry's mere presence (DisableMgr.cpp:397-399), so flags
// are ignored; rows are cached per map and dropped by .reload disables.
func (s *session) mmapPathfindingEnabled(ctx context.Context, mapID uint32) bool {
	if s.server == nil {
		return false
	}
	return s.server.mmapPathfindingEnabled(mapID)
}

// handleMmapStatsCommand mirrors HandleMmapStatsCommand (cs_mmaps.cpp:
// 214-260, RBAC 540): the header, pathfinding, and manager-counts lines
// are native, and the per-tile navmesh aggregates run over Server.mmaps.
func (s *session) handleMmapStatsCommand(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMMapStats) {
		return false
	}
	s.sendSysMessage("mmap stats:")
	enabled := "dis"
	if s.mmapPathfindingEnabled(ctx, s.player.Map) {
		enabled = "en"
	}
	s.sendSysMessage(fmt.Sprintf("  global mmap pathfinding is %sabled", enabled))
	// MMapManager::getLoadedMapsCount/getLoadedTilesCount
	// (cs_mmaps.cpp:221), over Server.mmaps.
	mgr := s.server.mmapManager()
	s.sendSysMessage(fmt.Sprintf(" %d maps loaded with %d tiles overall", mgr.LoadedMapsCount(), mgr.LoadedTilesCount()))
	// The per-tile dtNavMesh stats (cs_mmaps.cpp:233-258); report the
	// navmesh as missing exactly like the C++ handler does (:226) when
	// the map has no mesh loaded.
	navmesh := mgr.GetNavMesh(s.player.Map)
	if navmesh == nil {
		s.sendSysMessage("NavMesh not loaded for current map.")
		return true
	}
	var tileCount, nodeCount, polyCount, vertCount, triCount, triVertCount, dataSize uint32
	for i := 0; i < navmesh.GetMaxTiles(); i++ {
		tile := navmesh.GetTile(i)
		if tile == nil || tile.Header == nil {
			continue
		}
		tileCount++
		nodeCount += uint32(tile.Header.BVNodeCount)
		polyCount += uint32(tile.Header.PolyCount)
		vertCount += uint32(tile.Header.VertCount)
		triCount += uint32(tile.Header.DetailTriCount)
		triVertCount += uint32(tile.Header.DetailVertCount)
		dataSize += uint32(tile.DataSize)
	}
	s.sendSysMessage("Navmesh stats:")
	s.sendSysMessage(fmt.Sprintf(" %d tiles loaded", tileCount))
	s.sendSysMessage(fmt.Sprintf(" %d BVTree nodes", nodeCount))
	s.sendSysMessage(fmt.Sprintf(" %d polygons (%d vertices)", polyCount, vertCount))
	s.sendSysMessage(fmt.Sprintf(" %d triangles (%d vertices)", triCount, triVertCount))
	s.sendSysMessage(fmt.Sprintf(" %.2f MB of data (not including pointers)", float64(dataSize)/1048576))
	return true
}

// handleMmapTestAreaCommand mirrors HandleMmapTestArea (cs_mmaps.cpp:262-295,
// RBAC 541): creatures within 40 yards of the invoker (alive only, same map
// + instance + phase, reach-extended strict distance — the
// AnyUnitInObjectRangeCheck terms), then one PathGenerator per creature to
// the invoker's position; the paths count is C++'s per-creature attempt
// count (cs_mmaps.cpp:282-287) over the measured sweep time.
func (s *session) handleMmapTestAreaCommand(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandMMapTestArea) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	const radius = 40.0
	start := time.Now()
	var targets []*creatureMotion
	if s.server != nil {
		// AnyUnitInObjectRangeCheck (GridNotifiers.h:1050-1055) is IsAlive()
		// && IsWithinDistInMap(u, 40): same map (motionMapLocked) + phase-mask
		// overlap (Object.h:324-325) + _IsWithinDist (Object.cpp:1147-1167)
		// with maxdist = 40 + own combat reach + target combat reach,
		// strict < on the squared distance (Position.h:146-148).
		playerPhase := s.currentPlayerPhaseMask()
		ownReach := float64(miscPlayerCombatReach(s.player))
		s.server.motionMu.Lock()
		for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
			if m == nil || m.Health == 0 {
				continue // C++ IsAlive(): Go's health-based alive read (lua_objects.go IsAlive)
			}
			phase := m.PhaseMask
			if phase == 0 {
				phase = 1 // PHASEMASK_NORMAL, as for unset motions
			}
			if phase&playerPhase == 0 {
				continue
			}
			maxDist := radius + ownReach + float64(m.CombatReach)
			dx, dy, dz := float64(m.X-s.player.X), float64(m.Y-s.player.Y), float64(m.Z-s.player.Z)
			if dx*dx+dy*dy+dz*dz < maxDist*maxDist {
				targets = append(targets, m)
			}
		}
		s.server.motionMu.Unlock()
	}
	if len(targets) == 0 {
		s.sendSysMessage("No creatures in 40.000000 yard range.")
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Found %d Creatures.", len(targets)))
	// One PathGenerator per creature (cs_mmaps.cpp:282-287), to the
	// invoker's position. Paths run after the sweep lock is released:
	// CalculatePath reads terrain state, which must not run under
	// motionMu. The count is C++'s per-creature attempt count.
	paths := 0
	for _, m := range targets {
		path := NewPathGenerator(s.server, newCreaturePathSource(s.server, m))
		path.CalculatePath(s.player.X, s.player.Y, s.player.Z, false)
		paths++
	}
	s.sendSysMessage(fmt.Sprintf("Generated %d paths in %d ms", paths, time.Since(start).Milliseconds()))
	return true
}

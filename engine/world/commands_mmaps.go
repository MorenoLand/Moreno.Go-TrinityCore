package world

import (
	"context"
	"fmt"
	"strings"
	"time"
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
// Of the 5 arms, 3 are native or partially native and 2 are
// documented-blocked:
//
//   - `loc` is partially native: the grid tile location math (cs_mmaps.cpp:
//     133-134) needs only the player position, which is modeled. The navmesh
//     tile location (dtNavMeshQuery::findNearestPoly) has no Go bridge, so
//     the arm ends with the C++-exact "NavMesh not loaded for current map."
//     report, which is the true state of the Go tree (no navmesh is ever
//     loaded).
//   - `stats` is partially native: "mmap stats:", the "global mmap
//     pathfinding is %sabled" line (DisableMgr::IsPathfindingEnabled =
//     CONFIG_ENABLE_MMAPS && no DISABLE_TYPE_MMAP disables row; for MMAP
//     disables the entry's mere presence disables pathfinding,
//     DisableMgr.cpp:403-404), and the " %u maps loaded with %u tiles
//     overall" line (the Go MMapManager loads nothing, so both counts are
//     exactly 0) are real. The per-tile dtNavMesh stats have no Go bridge,
//     so the arm ends with the C++-exact "NavMesh not loaded for current
//     map." report.
//   - `path` has no Go bridge: PathGenerator/dtNavMesh are unbuilt. The C++
//     handler gates on the navmesh (cs_mmaps.cpp:97-101) before printing
//     "mmap path:" (:103), so with no navmesh loaded the output is exactly
//     the missing report.
//   - `loadedtiles` has no Go bridge: dtNavMesh is unbuilt. The C++ handler
//     (cs_mmaps.cpp:207) reports the navmesh as missing before printing
//     anything, so the Go arm emits exactly that line.
//   - `testarea` is partially native: the creature range search runs over
//     the live motion map (same map + instance, alive only, the
//     AnyUnitInObjectRangeCheck terms — phase-mask overlap, 40yd 3D plus
//     both combat reaches, strict < on squared distance,
//     GridNotifiers.h:1050-1055), but PathGenerator/dtNavMesh are unbuilt,
//     so no path is actually computed: the paths count mirrors C++'s
//     unconditional per-creature attempt count (cs_mmaps.cpp:282-287) over
//     the measured sweep time.
//
// Console-vs-chat branches are moot (Go commands are always sessioned).

// handleCmdMMap dispatches the "mmap" root (arm table cs_mmaps.cpp:51-55,
// root table :59-62).
func (s *session) handleCmdMMap(ctx context.Context, args []string) {
	const syntax = "Syntax: .mmap loadedtiles|loc|path|stats|testarea"
	if len(args) == 0 {
		// Bare ".mmap" matches the container root node, which carries no
		// permission in C++ (the 536 perm is dropped by the deprecated
		// ChatCommandBuilder overload); print the syntax line with no
		// gate, per the audited gobject container-root convention.
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("loadedtiles", sub):
		s.handleMmapLoadedTilesCommand(ctx)
	case strings.HasPrefix("loc", sub):
		s.handleMmapLocCommand(ctx)
	case strings.HasPrefix("path", sub):
		s.handleMmapPathCommand(ctx)
	case strings.HasPrefix("stats", sub):
		s.handleMmapStatsCommand(ctx)
	case strings.HasPrefix("testarea", sub):
		s.handleMmapTestAreaCommand(ctx)
	default:
		s.sendSysMessage(syntax)
	}
}

// handleMmapLoadedTilesCommand mirrors HandleMmapLoadedTilesCommand
// (cs_mmaps.cpp:189-212, RBAC 537). Documented-blocked: dtNavMesh has no Go
// bridge.
func (s *session) handleMmapLoadedTilesCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapLoadedTiles) {
		return
	}
	// The C++ handler (cs_mmaps.cpp:196) reports the navmesh as missing
	// before printing anything, so the Go arm emits exactly that line.
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapLocCommand mirrors HandleMmapLocCommand (cs_mmaps.cpp:126-187,
// RBAC 538): the grid tile location is native math; the navmesh tile
// location has no Go bridge.
func (s *session) handleMmapLocCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapLoc) {
		return
	}
	s.sendSysMessage("mmap tileloc:")
	// int32 gx = 32 - player->GetPositionX() / SIZE_OF_GRIDS (cs_mmaps.cpp:
	// 133-134); goSizeOfGrids mirrors SIZE_OF_GRIDS (GridDefines.h:38),
	// already used by the .go grid arm (commands.go:1923-1924).
	gx := int32(32 - s.player.X/goSizeOfGrids)
	gy := int32(32 - s.player.Y/goSizeOfGrids)
	s.sendSysMessage(fmt.Sprintf("%03d%02d%02d.mmtile", s.player.Map, gx, gy))
	s.sendSysMessage(fmt.Sprintf("tileloc [%d, %d]", gy, gx))
	// The navmesh tile location (findNearestPoly :162 / getTileAndPolyByRef,
	// cs_mmaps.cpp:140-187) has no Go bridge; report the navmesh as missing
	// exactly like the C++ handler does (cs_mmaps.cpp:144).
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapPathCommand mirrors HandleMmapPathCommand (cs_mmaps.cpp:65-124,
// RBAC 539). Documented-blocked: PathGenerator/dtNavMesh have no Go bridge.
func (s *session) handleMmapPathCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapPath) {
		return
	}
	// The C++ handler gates on the navmesh (cs_mmaps.cpp:67-71) before
	// printing "mmap path:" (:73), so with no navmesh loaded the output
	// is exactly the missing report; the target-selection check is
	// unreachable here.
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// mmapPathfindingEnabled mirrors DisableMgr::IsPathfindingEnabled
// (DisableMgr.cpp:418-422): CONFIG_ENABLE_MMAPS and no DISABLE_TYPE_MMAP
// (7) disables row for the map. For MMAP disables IsDisabledFor returns true
// on the entry's mere presence (DisableMgr.cpp:404-405), so flags are ignored.
func (s *session) mmapPathfindingEnabled(ctx context.Context, mapID uint32) bool {
	if s.server == nil || !s.server.Config.EnableMmaps {
		return false
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var one int
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM disables WHERE sourceType = ? AND entry = ?", disableTypeMMap, mapID).Scan(&one); err == nil {
			return false
		}
	}
	return true
}

// handleMmapStatsCommand mirrors HandleMmapStatsCommand (cs_mmaps.cpp:
// 214-260, RBAC 540): the header, pathfinding, and manager-counts lines
// are native; the per-tile navmesh stats have no Go bridge.
func (s *session) handleMmapStatsCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapStats) {
		return
	}
	s.sendSysMessage("mmap stats:")
	enabled := "dis"
	if s.mmapPathfindingEnabled(ctx, s.player.Map) {
		enabled = "en"
	}
	s.sendSysMessage(fmt.Sprintf("  global mmap pathfinding is %sabled", enabled))
	// MMapManager::getLoadedMapsCount/getLoadedTilesCount
	// (cs_mmaps.cpp:221) have no Go bridge, but the Go manager loads
	// nothing, so both counts are exactly 0 == the C++ output on an
	// empty manager.
	s.sendSysMessage(" 0 maps loaded with 0 tiles overall")
	// The per-tile dtNavMesh stats (cs_mmaps.cpp:233-258) have no Go
	// bridge; report the navmesh as missing exactly like the C++ handler
	// does (cs_mmaps.cpp:226).
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapTestAreaCommand mirrors HandleMmapTestArea (cs_mmaps.cpp:262-295,
// RBAC 541): creatures within 40 yards of the invoker (alive only, same map
// + instance + phase, reach-extended strict distance — the
// AnyUnitInObjectRangeCheck terms), then one path generation per creature.
// PathGenerator has no Go bridge, so the paths count is C++'s per-creature
// attempt count, not computed paths.
func (s *session) handleMmapTestAreaCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapTestArea) {
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	const radius = 40.0
	start := time.Now()
	count := 0
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
				count++
			}
		}
		s.server.motionMu.Unlock()
	}
	if count == 0 {
		s.sendSysMessage("No creatures in 40.000000 yard range.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("Found %d Creatures.", count))
	s.sendSysMessage(fmt.Sprintf("Generated %d paths in %d ms", count, time.Since(start).Milliseconds()))
}

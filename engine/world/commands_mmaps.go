package world

import (
	"context"
	"fmt"
	"strings"
)

// mmaps command port: mmaps_commandscript (cs_mmaps.cpp), the "mmap" root
// with 5 arms in C++ table order (loadedtiles, loc, path, stats, testarea).
// TWENTY-SEVENTH of 39 Commands groups (cs_script_loader.cpp decl 45 /
// call 90; call order re-verified this run: misc(89) -> mmaps(90) ->
// modify(91)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:404-409); the root permission 536 covers the bare ".mmap".
//
// Of the 5 arms, 2 are native or partially native and 3 are
// documented-blocked:
//
//   - `loc` is partially native: the grid tile location math (cs_mmaps.cpp:
//     164-170) needs only the player position, which is modeled. The navmesh
//     tile location (dtNavMeshQuery::findNearestPoly) has no Go bridge, so
//     the arm ends with the C++-exact "NavMesh not loaded for current map."
//     report, which is the true state of the Go tree (no navmesh is ever
//     loaded).
//   - `stats` is partially native: "mmap stats:" and the "global mmap
//     pathfinding is %sabled" line are real (DisableMgr::IsPathfindingEnabled
//     = CONFIG_ENABLE_MMAPS && no DISABLE_TYPE_MMAP disables row; for MMAP
//     disables the entry's mere presence disables pathfinding,
//     MMAP_DISABLE_PATHFINDING = 0x0, DisableMgr.cpp:404). The
//     MMapManager::getLoadedMapsCount/getLoadedTilesCount line and the
//     per-tile dtNavMesh stats have no Go bridge, so the arm ends with the
//     C++-exact "NavMesh not loaded for current map." report.
//   - `path` has no Go bridge: PathGenerator/dtNavMesh are unbuilt. The C++
//     handler prints "mmap path:" then reports the navmesh as missing
//     (cs_mmaps.cpp:100) before touching target selection, so the Go arm
//     emits exactly those two lines.
//   - `loadedtiles` has no Go bridge: dtNavMesh is unbuilt. The C++ handler
//     (cs_mmaps.cpp:207) reports the navmesh as missing before printing
//     anything, so the Go arm emits exactly that line.
//   - `testarea` has no Go bridge: the creature range search
//     (Cell::VisitGridObjects) and PathGenerator are unbuilt, so no creatures
//     can be located. The arm reports the C++-exact empty-search result.
//
// Console-vs-chat branches are moot (Go commands are always sessioned).

// handleCmdMMap dispatches the "mmap" root (cs_mmaps.cpp:52-66).
func (s *session) handleCmdMMap(ctx context.Context, args []string) {
	const syntax = "Syntax: .mmap loadedtiles|loc|path|stats|testarea"
	if len(args) == 0 {
		// Bare ".mmap" matches the root node, whose own permission is 536.
		if s.miscDeny(ctx, permissionCommandMMap) {
			return
		}
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
// (cs_mmaps.cpp:205-229, RBAC 537). Documented-blocked: dtNavMesh has no Go
// bridge.
func (s *session) handleMmapLoadedTilesCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapLoadedTiles) {
		return
	}
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapLocCommand mirrors HandleMmapLocCommand (cs_mmaps.cpp:160-203,
// RBAC 538): the grid tile location is native math; the navmesh tile
// location has no Go bridge.
func (s *session) handleMmapLocCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapLoc) {
		return
	}
	s.sendSysMessage("mmap tileloc:")
	// int32 gx = 32 - player->GetPositionX() / SIZE_OF_GRIDS (cs_mmaps.cpp:
	// 167-168); goSizeOfGrids mirrors SIZE_OF_GRIDS (MapDefines.h), already
	// used by the .go grid arm (commands.go:1144).
	gx := int32(32 - s.player.X/goSizeOfGrids)
	gy := int32(32 - s.player.Y/goSizeOfGrids)
	s.sendSysMessage(fmt.Sprintf("%03d%02d%02d.mmtile", s.player.Map, gx, gy))
	s.sendSysMessage(fmt.Sprintf("tileloc [%d, %d]", gy, gx))
	// The navmesh tile location (findNearestPoly / getTileAndPolyByRef,
	// cs_mmaps.cpp:173-201) has no Go bridge; report the navmesh as missing
	// exactly like the C++ handler does (cs_mmaps.cpp:178).
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapPathCommand mirrors HandleMmapPathCommand (cs_mmaps.cpp:94-140,
// RBAC 539). Documented-blocked: PathGenerator/dtNavMesh have no Go bridge.
func (s *session) handleMmapPathCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapPath) {
		return
	}
	s.sendSysMessage("mmap path:")
	// The C++ handler reports the missing navmesh (cs_mmaps.cpp:100) before
	// the target-selection check, so that check is unreachable here.
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// mmapPathfindingEnabled mirrors DisableMgr::IsPathfindingEnabled
// (DisableMgr.cpp:418-422): CONFIG_ENABLE_MMAPS and no DISABLE_TYPE_MMAP
// (7) disables row for the map. For MMAP disables IsDisabledFor returns true
// on the entry's mere presence (DisableMgr.cpp:404), so flags are ignored.
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
// 231-278, RBAC 540): the header and pathfinding lines are native; the
// manager counts and per-tile navmesh stats have no Go bridge.
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
	// MMapManager::getLoadedMapsCount/getLoadedTilesCount and the per-tile
	// dtNavMesh stats (cs_mmaps.cpp:240-273) have no Go bridge; report the
	// navmesh as missing exactly like the C++ handler does (cs_mmaps.cpp:
	// 244).
	s.sendSysMessage("NavMesh not loaded for current map.")
}

// handleMmapTestAreaCommand mirrors HandleMmapTestArea (cs_mmaps.cpp:280-312,
// RBAC 541). Documented-blocked: the creature range search
// (Cell::VisitGridObjects) and PathGenerator have no Go bridge, so no
// creatures can be located; report the C++-exact empty-search result.
func (s *session) handleMmapTestAreaCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandMMapTestArea) {
		return
	}
	s.sendSysMessage("No creatures in 40.000000 yard range.")
}

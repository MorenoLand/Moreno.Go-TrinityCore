package world

import (
	"context"
	"sort"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// BattlegroundMgr pure arms, ported from
// src/server/game/Battlegrounds/BattlegroundMgr.cpp. These are the scheduler
// packing and client-instance-id arms that do not need live BG instances.
//
// NOT ported (unported dependencies): BattlegroundMgr::Update's queue-drain
// arm (BattlegroundMgr.cpp:128-144 — decodes the packed ids back into
// BattlegroundQueue::BattlegroundQueueUpdate calls, which need live BG
// instances per the 05:10 run entry); AddToBGFreeSlotQueue /
// RemoveFromBGFreeSlotQueue (BattlegroundMgr.cpp:999-1013 — push_front and
// erase-by-instance-id over BGFreeSlotQueueContainer, which holds live
// Battleground* and has no Go store); GetBattlegroundThroughClientInstance
// (BattlegroundMgr.cpp:259-266 — bgDataStore lookup by client instance id,
// same missing store); the SendPacketToTeam delivery sites of the
// player joined/left builders below (Battleground.cpp:911, 1000 — the
// instance roster has no Go twin; the builders are ported, the fan-out is
// not).

// bgQueueScheduleID is the Go form of the packing arm of
// BattlegroundMgr::ScheduleQueueUpdate (BattlegroundMgr.cpp:844-852):
// scheduleId = (mmr << 32) | (arenaType << 24) | (queueType << 16) |
// (bgType << 8) | bracket.
func bgQueueScheduleID(arenaMatchmakerRating uint32, arenaType, bgQueueTypeId, bgTypeId, bracketID uint8) uint64 {
	return (uint64(arenaMatchmakerRating) << 32) | (uint64(arenaType) << 24) |
		(uint64(bgQueueTypeId) << 16) | (uint64(bgTypeId) << 8) | uint64(bracketID)
}

// enqueueBGQueueUpdateScheduler is the Go form of the dedup-push arm of
// BattlegroundMgr::ScheduleQueueUpdate (BattlegroundMgr.cpp:849-850):
// appends id only if it is not already scheduled.
func enqueueBGQueueUpdateScheduler(scheduler []uint64, id uint64) []uint64 {
	for _, existing := range scheduler {
		if existing == id {
			return scheduler
		}
	}
	return append(scheduler, id)
}

// claimClientVisibleInstanceID is the Go form of
// BattlegroundMgr::CreateClientVisibleInstanceId
// (BattlegroundMgr.cpp:325-348): arenas have no client instance id (0); the
// ordered used-set is gap-scanned for the lowest free positive id and the
// chosen id is inserted. C++ quirk, ported exactly: when a gap is found the
// break leaves lastId one below the gap and the final ++lastId lands on the
// first in-use id above the gap, so a set like {1,3} yields 3 rather than 2
// (the insert then no-ops); the gap is only honoured when the used ids form
// a clean 1..N prefix, e.g. {1,2} -> 3.
func claimClientVisibleInstanceID(used []uint32, isArena bool) (uint32, []uint32) {
	if isArena {
		return 0, used
	}
	var lastID uint32
	for _, id := range used {
		lastID++
		if lastID != id {
			break
		}
		lastID = id
	}
	lastID++
	out := append([]uint32(nil), used...)
	i := sort.Search(len(out), func(i int) bool { return out[i] >= lastID })
	if i >= len(out) || out[i] != lastID {
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = lastID
	}
	return lastID, out
}

// buildPlayerLeftBattlegroundPacket is the Go form of
// BattlegroundMgr::BuildPlayerLeftBattlegroundPacket
// (BattlegroundMgr.cpp:247-251): SMSG_BATTLEGROUND_PLAYER_LEFT carrying the
// leaver's guid.
func buildPlayerLeftBattlegroundPacket(guid uint64) []byte {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	return buf.Bytes()
}

// buildPlayerJoinedBattlegroundPacket is the Go form of
// BattlegroundMgr::BuildPlayerJoinedBattlegroundPacket
// (BattlegroundMgr.cpp:253-257): SMSG_BATTLEGROUND_PLAYER_JOINED carrying
// the joiner's guid.
func buildPlayerJoinedBattlegroundPacket(guid uint64) []byte {
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	return buf.Bytes()
}

// BattlegroundMgr instance-lifecycle arms, ported from
// src/server/game/Battlegrounds/BattlegroundMgr.cpp. CreateNewBattleground
// (BattlegroundMgr.cpp:351-440) resolves the queued type through
// GetRandomBG, copies the BG template via the per-type switch, then runs the
// setup sequence: SetBracket, SetInstanceID (sMapMgr->GenerateInstanceId),
// SetClientInstanceID (CreateClientVisibleInstanceId, ported above),
// Reset -> SetStatus(STATUS_WAIT_JOIN), SetArenaType, SetTypeID(original),
// SetRandomTypeID(resolved), SetRated, SetRandom, and the arena max-players
// arm below. AddBattleground (BattlegroundMgr.cpp:1015-1019) and
// RemoveBattleground (BattlegroundMgr.cpp:1021-1024) are container ops over
// bgDataStore[typeId].m_Battlegrounds[instanceId]; all three need live BG
// instances plus the template store, and none has a Go twin.
//
// NOT ported: the GetBattlegroundTemplate registry lookup and template-copy
// switch (BattlegroundMgr.cpp:355-408); GetRandomBG (BattlegroundMgr.cpp:968-
// 990 — weighted pick over the BattlemasterEntry MapID list; the
// weighted-pick kernel itself is ported below and Weight now loads with the
// template store, but the MapID list still needs the BattlemasterEntry
// record); the Set*/Reset wiring chain (BattlegroundMgr.cpp:417-422); the
// start-location and script-id arms of LoadBattlegroundTemplates
// (BattlegroundMgr.cpp:518-571 — AllianceStartLoc/HordeStartLoc resolved
// through WorldSafeLocs.dbc, ScriptName through the script registry) and
// the CreateBattleground template-init wiring (BattlegroundMgr.cpp:444-503
// — map id, locale name, start positions, level range, script id). The
// row-read, gating, and validation arms of LoadBattlegroundTemplates are
// ported below as loadBattlegroundTemplates; the per-map registry
// (_battlegroundMapTemplates) has no Go twin, battlegroundTypeForMap covers
// the map->type direction instead.

// arenaMaxPlayersPerTeam is the Go form of the arena max-players arm of
// BattlegroundMgr::CreateNewBattleground (BattlegroundMgr.cpp:424-438):
// 2v2/3v3/5v5 set maxPlayersPerTeam to 2/3/5, and SetMaxPlayers is called
// with perTeam * 2.
func arenaMaxPlayersPerTeam(arenaType uint8) uint32 {
	switch arenaType {
	case ArenaType2v2:
		return 2
	case ArenaType3v3:
		return 3
	case ArenaType5v5:
		return 5
	}
	return 0
}

// bgInstanceIsRandom is the Go form of the random-flag arm of
// BattlegroundMgr::CreateNewBattleground (BattlegroundMgr.cpp:415): a new
// instance is random when the resolved type differs from the queued
// (original) type and the BG is not an arena.
func bgInstanceIsRandom(resolvedBgTypeID, originalBgTypeID uint32, isArena bool) bool {
	return resolvedBgTypeID != originalBgTypeID && !isArena
}

// weightedRandomIndex is the Go form of the selection kernel behind
// Trinity::Containers::SelectRandomWeightedContainerElement
// (src/common/Utilities/Containers.h:136-141) and urandweighted
// (src/common/Utilities/Random.cpp:87-91 — std::discrete_distribution over
// the weight array): index i is picked with probability w_i / sum. The
// caller supplies the uniform [0,1) roll, matching the seeded
// RandomEngine draw in C++. BattlegroundMgr::GetRandomBG uses it over the
// BattlemasterEntry MapID list with each candidate's Weight.

func weightedRandomIndex(weights []float64, roll float64) int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	pick := roll * sum
	for i, w := range weights {
		pick -= w
		if pick < 0 {
			return i
		}
	}
	return len(weights) - 1
}

// bgTemplate is the Go form of BattlegroundTemplate (BattlegroundMgr.h):
// the battleground_template row fields Go consumes. StartLocation,
// MaxStartDistSq, and ScriptId are not stored: they feed only
// CreateBattleground's instance wiring (BattlegroundMgr.cpp:444-503),
// which has no Go twin yet; the BattlemasterEntry pointer is replaced by
// the BattlemasterList.dbc gate in the loader plus battlegroundTypeForMap
// for the map->type direction.
type bgTemplate struct {
	id                uint32
	minPlayersPerTeam uint32
	maxPlayersPerTeam uint32
	minLevel          uint8
	maxLevel          uint8
	weight            uint8
}

// loadBattlegroundTemplates mirrors BattlegroundMgr::LoadBattlegroundTemplates
// (BattlegroundMgr.cpp:518-571): one load, keyed by bg type id, clearing
// and refilling the store as C++ does. Rows are skipped exactly as in C++:
// DisableMgr-disabled types, types with no BattlemasterList.dbc entry,
// MaxPlayersPerTeam == 0 or MinPlayersPerTeam > MaxPlayersPerTeam, and
// MinLevel == 0 or MaxLevel == 0 or MinLevel > MaxLevel. A nil world DB
// leaves the store unloaded so a later caller retries; a query failure
// loads an empty store, the C++ empty-table shape.
func (s *Server) loadBattlegroundTemplates(ctx context.Context) {
	if s == nil {
		return
	}
	s.bgTemplatesMu.Lock()
	defer s.bgTemplatesMu.Unlock()
	if s.bgTemplatesLoaded {
		return
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.bgTemplatesLoaded = true
	templates := make(map[uint32]*bgTemplate)
	s.bgTemplates = templates
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT ID, MinPlayersPerTeam, MaxPlayersPerTeam, MinLvl, MaxLvl, Weight FROM battleground_template`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, minPlayers, maxPlayers, minLvl, maxLvl, weight uint32
		if err := rows.Scan(&id, &minPlayers, &maxPlayers, &minLvl, &maxLvl, &weight); err != nil {
			continue
		}
		var one int
		if err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT 1 FROM disables WHERE sourceType = ? AND entry = ?`, disableTypeBattleground, id).Scan(&one); err == nil {
			continue
		}
		if !s.battlegroundTemplateHasBattlemasterEntry(id) {
			continue
		}
		if maxPlayers == 0 || minPlayers > maxPlayers {
			if s.Logger != nil {
				s.Logger.Error("skipping battleground_template row with bad player counts", "id", id, "minPlayersPerTeam", minPlayers, "maxPlayersPerTeam", maxPlayers)
			}
			continue
		}
		if minLvl == 0 || maxLvl == 0 || minLvl > maxLvl {
			if s.Logger != nil {
				s.Logger.Error("skipping battleground_template row with bad level range", "id", id, "minLevel", minLvl, "maxLevel", maxLvl)
			}
			continue
		}
		templates[id] = &bgTemplate{id: id, minPlayersPerTeam: minPlayers, maxPlayersPerTeam: maxPlayers, minLevel: uint8(minLvl), maxLevel: uint8(maxLvl), weight: uint8(weight)}
	}
}

// bgTemplateForType returns the loaded battleground_template row for a BG
// type id, or nil when the store holds no row for it (unseeded table,
// loader-skipped row, or no world DB). It lazy-loads the store exactly
// like bgPrematureMinPlayers, so callers never query the table per call:
// in C++ every reader goes through the loaded _battlegroundTemplates map
// (BattlegroundMgr::GetBattlegroundTemplate), never a fresh DB read.
func (s *Server) bgTemplateForType(ctx context.Context, bgTypeID uint32) *bgTemplate {
	if s == nil {
		return nil
	}
	s.loadBattlegroundTemplates(ctx)
	s.bgTemplatesMu.Lock()
	defer s.bgTemplatesMu.Unlock()
	return s.bgTemplates[bgTypeID]
}

// invalidateBattlegroundTemplates drops the loaded store so the next
// bgTemplateForType lookup reloads it: the Go twin of C++'s
// `reload battleground_template` (cs_reload.cpp: HandleReloadBattlegroundTemplateCommand
// re-runs LoadBattlegroundTemplates). Without it the load-once store would
// keep serving pre-reload rows after a reload command.
func (s *Server) invalidateBattlegroundTemplates() {
	if s == nil {
		return
	}
	s.bgTemplatesMu.Lock()
	defer s.bgTemplatesMu.Unlock()
	s.bgTemplatesLoaded = false
	s.bgTemplates = nil
}

// battlegroundTemplateHasBattlemasterEntry mirrors the BattlemasterList.dbc
// gate of LoadBattlegroundTemplates (BattlegroundMgr.cpp:535-541): a row
// whose type has no BattlemasterList entry is not created. It shares the
// battlemasterBGType lookup shape (BattlemasterList file + Find), without
// the battlemaster_entry table hop, since the template row already carries
// the type id.
func (s *Server) battlegroundTemplateHasBattlemasterEntry(bgTypeID uint32) bool {
	if s == nil || s.Data == nil {
		return false
	}
	file, err := s.Data.File("BattlemasterList")
	if err != nil {
		return false
	}
	_, found := file.Find(bgTypeID)
	return found
}

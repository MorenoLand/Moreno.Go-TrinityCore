package world

import (
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

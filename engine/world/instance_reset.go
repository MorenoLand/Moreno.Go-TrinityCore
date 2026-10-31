package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Instance-reset scheduler.
//
// C++ owners:
//   - InstanceSaveManager::LoadResetTimes (InstanceSaveMgr.cpp:303-446):
//     instance_reset rows are hour-normalized, expired rows advanced to the
//     next cycle, rows for unknown map/difficulty pairs deleted, and rows
//     for DBC MapDifficulty pairs with a reset window inserted; the first
//     warning stage still in the future is scheduled per pair.
//   - InstanceSaveManager::Update (InstanceSaveMgr.cpp:521-550): the queue
//     fires warnings at reset-3600, reset-900, reset-300 and the reset
//     itself at reset-60 (ResetTimeDelay = {3600, 900, 300, 60}); a fired
//     warning schedules the next stage, and the reset schedules the next
//     cycle's first warning.
//   - InstanceSaveManager::_ResetOrWarnAll (InstanceSaveMgr.cpp:627-725):
//     on reset, expired character binds (extendState = 0 or non-permanent)
//     and group binds for the map/difficulty are deleted, instance rows
//     with no surviving non-expired bind are deleted, remaining binds are
//     demoted one extend state (EXTENDED -> NORMAL -> EXPIRED, the
//     promotion leg of the head entry), the next global reset is computed
//     via GetSubsequentResetTime and stored, and loaded maps either warn
//     their players or reset.
//   - InstanceMap::Reset (Map.cpp:4070-4131, INSTANCE_RESET_GLOBAL):
//     players inside a reset map without a live extended bind to that
//     instance are marked instance-invalid, which starts the 60s homebind
//     countdown (Player::UpdateHomebindTime); Go's updateHomebindTimers
//     already runs that countdown once instanceValid is false.
//   - InstanceSaveManager::_ResetInstance (InstanceSaveMgr.cpp:596-625)
//     via the type-0 queue event: a normal dungeon save whose own
//     resettime passes is deleted with its respawn times; players inside
//     are NOT marked invalid (that leg belongs to the global reset only),
//     the map unloads when empty. Go keeps no loaded maps, so the row,
//     bind and respawn deletion is the durable effect.
//   - InstanceSave::GetResetTimeForDB (InstanceSaveMgr.cpp:221-229):
//     only normal dungeons store resettime > 0 in the instance table
//     (raids and heroic difficulty store 0), so the per-save sweep below
//     can never collide with the global promotion leg: a save the global
//     reset promotes never appears in the per-save query.
//
// Deviations, all load-bearing:
//   - Go keeps no reset-hour config (C++ Instance.ResetTimeHour, default
//     4) and no reset-time rate config (RATE_INSTANCE_RESET_TIME,
//     default 1), so the hour is fixed at 4 and the rate at 1.
//   - Go keeps no in-memory InstanceSave objects and no loaded InstanceMap
//     objects, so there is no queue of InstResetEvent structs: the stage
//     machine is polled from the 100ms world tick against in-memory
//     per-pair schedules, which fires within one tick of the C++ queue
//     times. The per-save sweep is throttled to 60s (a save expiring
//     mid-interval fires up to 60s late; C++ fires within one tick).
//   - C++ _ResetOrWarnAll's "delete respawn times even if the maps are
//     not loaded" leg is a @todo in C++; Go implements it via
//     deleteRespawnRowsForInstance for every deleted instance row, which
//     is what Go's admission model needs (dead bosses must not stream
//     alive into a fresh instance under a reused key).
//   - The per-save sweep deletes bind rows for offline players too.
//     C++ _ResetSave unbinds only loaded players' in-memory binds and
//     leaves offline rows orphaned until the next LoadInstances; orphan
//     binds are inert in Go (admission joins the instance table) and Go
//     never re-runs the startup orphan sweep at runtime, so deleting
//     them here is the same observable outcome without the orphans.

// instanceResetDelays mirrors InstanceSaveManager::ResetTimeDelay
// (InstanceSaveMgr.cpp:36): stage 1 fires at reset-3600, stage 2 at
// reset-900, stage 3 at reset-300, stage 4 (the reset) at reset-60.
var instanceResetDelays = [4]int64{3600, 900, 300, 60}

// instanceResetHour is the C++ Instance.ResetTimeHour default (World.cpp:979).
const instanceResetHour = 4

// instanceResetPerSaveSweepSeconds throttles the type-0 per-save sweep.
const instanceResetPerSaveSweepSeconds = 60

type instanceResetKey struct {
	MapID      uint32
	Difficulty uint32
}

type instanceResetSchedule struct {
	resetTime int64
	period    int64
	nextStage int // 1..3 = warnings, 4 = the reset
	nextAt    int64
}

// instanceResetLocalHour mirrors GetLocalHourTimestamp (Util.cpp:82-95)
// with onlyAfterTime = false: the local midnight of ts's local date plus
// instanceResetHour hours.
func instanceResetLocalHour(ts int64) int64 {
	local := time.Unix(ts, 0).Local()
	return time.Date(local.Year(), local.Month(), local.Day(), instanceResetHour, 0, 0, 0, time.Local).Unix()
}

// instanceResetPeriod mirrors the LoadResetTimes period computation
// (InstanceSaveMgr.cpp:393-396) with rate 1: the DBC reset window
// rounded down to whole days, at least one day.
func instanceResetPeriod(raidDuration uint32) int64 {
	period := int64(raidDuration/86400) * 86400
	if period < 86400 {
		period = 86400
	}
	return period
}

// instanceResetSubsequent mirrors GetSubsequentResetTime
// (InstanceSaveMgr.cpp:450-459): the local-hour timestamp of the day
// after resetTime plus one period.
func instanceResetSubsequent(resetTime, period int64) int64 {
	base := ((resetTime+60)/86400)*86400 + period
	return instanceResetLocalHour(base)
}

// instanceResetAdvanceExpired mirrors the LoadResetTimes expired arm
// (InstanceSaveMgr.cpp:414-425): advance t past now by whole periods,
// then take the local-hour timestamp.
func instanceResetAdvanceExpired(t, now, period int64) int64 {
	day := (t / 86400) * 86400
	today := (now / 86400) * 86400
	steps := (today-day)/period + 1
	return instanceResetLocalHour(day + steps*period)
}

// instanceResetInitial mirrors the LoadResetTimes missing-row arm
// (InstanceSaveMgr.cpp:400-410): the local-hour timestamp of tomorrow
// plus one period.
func instanceResetInitial(now, period int64) int64 {
	today := (now / 86400) * 86400
	return instanceResetLocalHour(today + period)
}

// instanceResetNextStage mirrors the LoadResetTimes schedule block
// (InstanceSaveMgr.cpp:430-437): the first warning stage whose fire
// time is still in the future, else stage 4 (the reset at reset-60).
func instanceResetNextStage(resetTime, now int64) (int, int64) {
	for stage := 1; stage < 4; stage++ {
		if at := resetTime - instanceResetDelays[stage-1]; at > now {
			return stage, at
		}
	}
	return 4, resetTime - instanceResetDelays[3]
}

// initInstanceResets mirrors InstanceSaveManager::LoadResetTimes. It
// runs once, from the first updateInstanceResets call, which the world
// tick orders after cleanupExpiredInstanceSaves in the same tick —
// matching C++ LoadInstances (expired delete) running before
// LoadResetTimes (global-time advance), so a reset missed while down is
// handled by the startup delete and the scheduler advances to the next
// cycle instead of re-firing the missed reset.
func (s *Server) initInstanceResets(ctx context.Context) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || s.Data == nil {
		return
	}
	now := time.Now().Unix()
	db := s.CharactersStore.DB
	rows, err := db.QueryContext(ctx, "SELECT mapid, difficulty, resettime FROM instance_reset")
	if err != nil {
		return
	}
	present := make(map[instanceResetKey]bool)
	type storedRow struct {
		key       instanceResetKey
		resetTime int64
	}
	var stored []storedRow
	for rows.Next() {
		var mapID, difficulty, resetTime int64
		if err := rows.Scan(&mapID, &difficulty, &resetTime); err != nil || mapID <= 0 || difficulty < 0 || mapID > int64(^uint32(0)) || difficulty > int64(^uint32(0)) {
			continue
		}
		key := instanceResetKey{MapID: uint32(mapID), Difficulty: uint32(difficulty)}
		present[key] = true
		stored = append(stored, storedRow{key: key, resetTime: resetTime})
	}
	rows.Close()
	schedules := make(map[instanceResetKey]*instanceResetSchedule)
	for _, row := range stored {
		diff, found, derr := s.Data.MapDifficulty(row.key.MapID, row.key.Difficulty)
		if derr != nil || !found || diff.RaidDuration == 0 {
			// C++ deletes rows for invalid map/difficulty pairs.
			_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_GLOBAL_INSTANCE_RESETTIME", row.key.MapID, row.key.Difficulty)
			continue
		}
		period := instanceResetPeriod(diff.RaidDuration)
		resetTime := row.resetTime
		if normalized := instanceResetLocalHour(resetTime); normalized != resetTime {
			// C++ rewrites the row when the configured hour shifts it.
			_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_GLOBAL_INSTANCE_RESETTIME", normalized, row.key.MapID, row.key.Difficulty)
			resetTime = normalized
		}
		if resetTime < now {
			resetTime = instanceResetAdvanceExpired(resetTime, now, period)
			_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_GLOBAL_INSTANCE_RESETTIME", resetTime, row.key.MapID, row.key.Difficulty)
		}
		stage, at := instanceResetNextStage(resetTime, now)
		schedules[row.key] = &instanceResetSchedule{resetTime: resetTime, period: period, nextStage: stage, nextAt: at}
	}
	if pairs, err := s.Data.MapDifficulties(); err == nil {
		for _, pair := range pairs {
			if pair.RaidDuration == 0 {
				continue
			}
			key := instanceResetKey{MapID: pair.MapID, Difficulty: pair.Difficulty}
			if present[key] {
				continue
			}
			period := instanceResetPeriod(pair.RaidDuration)
			resetTime := instanceResetInitial(now, period)
			if _, err := s.CharactersStore.ExecStatement(ctx, "CHAR_INS_GLOBAL_INSTANCE_RESETTIME", key.MapID, key.Difficulty, resetTime); err != nil {
				continue
			}
			stage, at := instanceResetNextStage(resetTime, now)
			schedules[key] = &instanceResetSchedule{resetTime: resetTime, period: period, nextStage: stage, nextAt: at}
		}
	}
	s.instanceResetMu.Lock()
	s.instanceResets = schedules
	s.instanceResetsLoaded = true
	s.instanceResetMu.Unlock()
}

// updateInstanceResets runs the global reset stage machine and the
// per-save expiry sweep from the world tick. The global leg runs first
// so a save the global reset just promoted or deleted is never seen by
// the per-save sweep in the same tick.
func (s *Server) updateInstanceResets(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	s.instanceResetMu.Lock()
	loaded := s.instanceResetsLoaded
	s.instanceResetMu.Unlock()
	if !loaded {
		s.initInstanceResets(ctx)
		s.instanceResetMu.Lock()
		loaded = s.instanceResetsLoaded
		s.instanceResetMu.Unlock()
		if !loaded {
			return
		}
	}
	nowUnix := now.Unix()
	s.instanceResetMu.Lock()
	snapshot := make([]struct {
		key   instanceResetKey
		sched *instanceResetSchedule
	}, 0, len(s.instanceResets))
	for key, sched := range s.instanceResets {
		if sched == nil {
			continue
		}
		copySched := *sched
		snapshot = append(snapshot, struct {
			key   instanceResetKey
			sched *instanceResetSchedule
		}{key: key, sched: &copySched})
	}
	lastSweep := s.lastInstancePerSaveSweep
	s.instanceResetMu.Unlock()
	for _, item := range snapshot {
		// C++ Update fires a queue event only once its time is past
		// (t < now, strictly).
		if nowUnix <= item.sched.nextAt {
			continue
		}
		if item.sched.nextStage < 4 {
			timeLeft := item.sched.resetTime - nowUnix
			if timeLeft < 0 {
				timeLeft = 0
			}
			s.sendInstanceResetWarning(ctx, item.key.MapID, item.key.Difficulty, uint32(timeLeft))
			item.sched.nextStage++
			item.sched.nextAt = item.sched.resetTime - instanceResetDelays[item.sched.nextStage-1]
		} else {
			s.resetInstanceGlobal(ctx, item.key)
			next := instanceResetSubsequent(item.sched.resetTime, item.sched.period)
			_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_GLOBAL_INSTANCE_RESETTIME", next, item.key.MapID, item.key.Difficulty)
			item.sched.resetTime = next
			item.sched.nextStage, item.sched.nextAt = instanceResetNextStage(next, nowUnix)
		}
		s.instanceResetMu.Lock()
		if s.instanceResets != nil {
			s.instanceResets[item.key] = item.sched
		}
		s.instanceResetMu.Unlock()
	}
	if nowUnix-lastSweep >= instanceResetPerSaveSweepSeconds {
		s.sweepExpiredInstanceSaves(ctx, nowUnix)
		s.instanceResetMu.Lock()
		s.lastInstancePerSaveSweep = nowUnix
		s.instanceResetMu.Unlock()
	}
}

// sendInstanceResetWarning mirrors Player::SendInstanceResetWarning
// (Player.cpp:23086-23111) for the scheduled-warning case (welcome =
// false) and InstanceMap::SendResetWarnings (Map.cpp:4188-4193): every
// online player in a loaded instance of the map/difficulty gets
// SMSG_RAID_INSTANCE_MESSAGE with the type selected from the time left
// (over 6h -> WELCOME, over 1h -> WARNING_HOURS, over 5m ->
// WARNING_MIN, else WARNING_MIN_SOON); only the WELCOME form carries
// the two trailing bytes.
func (s *Server) sendInstanceResetWarning(ctx context.Context, mapID, difficulty uint32, timeLeft uint32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	var warningType uint32
	switch {
	case timeLeft > 21600:
		warningType = 4 // RAID_INSTANCE_WELCOME (Player.h:636)
	case timeLeft > 3600:
		warningType = 1 // RAID_INSTANCE_WARNING_HOURS (Player.h:633)
	case timeLeft > 300:
		warningType = 2 // RAID_INSTANCE_WARNING_MIN (Player.h:634)
	default:
		warningType = 3 // RAID_INSTANCE_WARNING_MIN_SOON (Player.h:635)
	}
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT id, difficulty FROM instance WHERE map = ?", mapID)
	if err != nil {
		return
	}
	targets := make(map[uint32]bool)
	for rows.Next() {
		var id, diff int64
		if err := rows.Scan(&id, &diff); err != nil || id <= 0 || id > int64(^uint32(0)) {
			continue
		}
		if uint32(diff) == difficulty {
			targets[uint32(id)] = true
		}
	}
	rows.Close()
	if len(targets) == 0 {
		return
	}
	s.sessionsMu.RLock()
	recipients := make([]*session, 0, 16)
	for sess := range s.sessions {
		if sess == nil || sess.player == nil || !sess.worldReady.Load() {
			continue
		}
		if sess.player.Map != mapID || !targets[sess.player.InstanceID] {
			continue
		}
		recipients = append(recipients, sess)
	}
	s.sessionsMu.RUnlock()
	buf := protocol.NewBuffer(18)
	buf.WriteU32(warningType)
	buf.WriteU32(mapID)
	buf.WriteU32(difficulty)
	buf.WriteU32(timeLeft)
	if warningType == 4 {
		buf.WriteU8(0) // is locked
		buf.WriteU8(0) // is extended
	}
	for _, sess := range recipients {
		_ = sess.write(uint16(protocol.OpcodeSMSG_RAID_INSTANCE_MESSAGE), buf.Bytes(), true)
	}
}

// resetInstanceGlobal mirrors the warn = false arm of
// InstanceSaveManager::_ResetOrWarnAll (InstanceSaveMgr.cpp:643-694),
// in C++ transaction order: delete expired character binds, delete
// group binds, delete instance rows with no surviving non-expired bind,
// demote remaining binds one extend state. The doomed instance ids are
// collected after the bind deletes (so the delete predicate sees the
// post-delete bind state, as the C++ transaction does) and before the
// instance-row delete, then their respawn rows and in-memory records
// are wiped and online players inside them are marked instance-invalid
// (the InstanceMap::Reset GLOBAL leg), which starts their 60s homebind
// countdown via updateHomebindTimers. Players in a surviving
// (promoted) instance keep their validity.
func (s *Server) resetInstanceGlobal(ctx context.Context, key instanceResetKey) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	db := s.CharactersStore.DB
	_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_EXPIRED_CHAR_INSTANCE_BY_MAP_DIFF", key.MapID, key.Difficulty)
	_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_GROUP_INSTANCE_BY_MAP_DIFF", key.MapID, key.Difficulty)
	rows, err := db.QueryContext(ctx, `SELECT id FROM instance WHERE map = ? AND difficulty = ? AND (SELECT guid FROM character_instance WHERE extendState != 0 AND instance = id LIMIT 1) IS NULL`, key.MapID, key.Difficulty)
	doomed := make(map[uint32]bool)
	if err == nil {
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil || id <= 0 || id > int64(^uint32(0)) {
				continue
			}
			doomed[uint32(id)] = true
		}
		rows.Close()
	}
	// The remaining C++ transaction arms run even if the doomed
	// collection failed (then with an empty doomed set, so the respawn
	// wipe and player invalidation below are skipped but the bind and
	// row deletes still land).
	_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_EXPIRED_INSTANCE_BY_MAP_DIFF", key.MapID, key.Difficulty)
	_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_EXPIRE_CHAR_INSTANCE_BY_MAP_DIFF", key.MapID, key.Difficulty)
	for id := range doomed {
		s.deleteRespawnRowsForInstance(ctx, key.MapID, id)
	}
	if len(doomed) == 0 {
		return
	}
	s.sessionsMu.RLock()
	targets := make([]*session, 0, 16)
	for sess := range s.sessions {
		if sess == nil || sess.player == nil || !sess.worldReady.Load() {
			continue
		}
		if sess.player.Map != key.MapID || !doomed[sess.player.InstanceID] {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		if sessionIsGameMaster(sess) {
			continue
		}
		sess.instanceValid = false
	}
}

// instanceHasLivePlayers mirrors InstanceMap::SetResetSchedule's
// !HavePlayers gate (Map.cpp:4194-4204): the type-0 reset event for a
// normal instance is only armed while the map is empty. Entering
// cancels it (Map.cpp:3966) and leaving re-arms it (Map.cpp:4006),
// so a normal instance is never reset while players are inside.
// Go keeps no loaded maps; a worldReady session admitted to the
// map/instance pair (the hasActiveWorldportInstance signal) is the
// occupancy equivalent, GMs included as in C++ HavePlayers. A session
// in the ack window also counts: completeWorldPort assigns
// player.InstanceID at selection (characters.go:1673) while worldReady
// is still false (it commits only at admission), so a worldReady-only
// test let the sweep delete the row and binds under an acking player
// before the persist step; the kept selection recreates them there
// (ensureWorldportInstanceSave, the InstanceMap::AddPlayerToMap
// get-or-create analog, Map.cpp:3887-3892). The player's live Map is
// already the dest map
// (teleportToOptions overwrites it at initiation), so the Map match
// keeps a pre-ack transfer from counting toward its origin instance.
func (s *Server) instanceHasLivePlayers(mapID, instanceID uint32) bool {
	if s == nil {
		return false
	}
	target := uint64(mapID)<<32 | uint64(instanceID)
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess == nil {
			continue
		}
		if sess.worldReady.Load() && sess.worldInstance.Load() == target {
			return true
		}
		if sess.player != nil && sess.player.Map == mapID && sess.player.InstanceID == instanceID {
			return true
		}
	}
	return false
}

// sweepExpiredInstanceSaves mirrors the type-0 queue event of
// InstanceSaveManager::Update (InstanceSaveMgr.cpp:526-530) feeding
// _ResetInstance: normal dungeon saves whose own resettime passed are
// deleted with their binds and respawn rows. It never touches
// instanceValid (see the file header): players inside keep playing
// until they leave, as C++ keeps them in the map until it unloads.
// A save with live players inside is skipped: leaving re-arms the
// C++ event at the original (already past) reset time, so the next
// 60s sweep after the map empties is the equivalent firing.
func (s *Server) sweepExpiredInstanceSaves(ctx context.Context, nowUnix int64) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT id, map FROM instance WHERE resettime > 0 AND resettime < ?", nowUnix)
	if err != nil {
		return
	}
	type expiredSave struct {
		id    uint32
		mapID uint32
	}
	var expired []expiredSave
	for rows.Next() {
		var id, mapID int64
		if err := rows.Scan(&id, &mapID); err != nil || id <= 0 || id > int64(^uint32(0)) {
			continue
		}
		expired = append(expired, expiredSave{id: uint32(id), mapID: uint32(mapID)})
	}
	rows.Close()
	for _, save := range expired {
		if s.instanceHasLivePlayers(save.mapID, save.id) {
			continue
		}
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_INSTANCE_BY_INSTANCE", save.id)
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_CHAR_INSTANCE_BY_INSTANCE", save.id)
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_DEL_GROUP_INSTANCE_BY_INSTANCE", save.id)
		s.deleteRespawnRowsForInstance(ctx, save.mapID, save.id)
	}
}

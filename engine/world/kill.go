package world

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// unitDynFlagLootable marks a corpse lootable in UNIT_FIELD_DYNAMIC_FLAGS.
const unitDynFlagLootable uint32 = 0x00000001

// creatureRespawn records when a killed spawn restores its health.
type creatureRespawn struct {
	GUID    uint32
	At      time.Time
	Health  uint32
	Entry   uint32
	Map     uint32
	X, Y, Z float32
}

// xpTable mirrors TDB 3.3.5a `player_xp_for_level`; preferred from the world
// DB when populated (per-dialect SQL), otherwise this reference curve.
var xpCurve = [81]uint32{
	0, 400, 900, 1400, 2000, 2700, 3400, 4200, 5100, 6100, 7200,
	8400, 9700, 11100, 12600, 14200, 15900, 17700, 19600, 21600, 23800,
	26100, 28500, 31000, 33600, 36300, 39100, 42000, 45000, 48100, 51300,
	54700, 58200, 61800, 65600, 69500, 73600, 77800, 82200, 86800, 91600,
	96600, 101800, 107200, 112800, 118600, 124500, 130600, 136900, 143400, 150100,
	157000, 164100, 171400, 178900, 186600, 194500, 202700, 211000, 219600, 228400,
	237400, 246700, 256200, 266000, 276100, 286400, 297000, 307900, 319000, 330400,
	342100, 354000, 366200, 378700, 391500, 404700, 418200, 432000, 446200, 0,
}

var (
	xpTableMu   sync.RWMutex
	xpTableRows []uint32
	xpTableInit bool
)

func (s *Server) xpForLevel(ctx context.Context, level uint32) uint32 {
	if level == 0 || level >= uint32(len(xpCurve)) {
		return 0
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		xpTableMu.RLock()
		ready := xpTableInit && xpTableRows != nil
		rows := xpTableRows
		xpTableMu.RUnlock()
		if !ready {
			loaded := make([]uint32, len(xpCurve))
			copy(loaded, xpCurve[:])
			query := "SELECT Level, Experience FROM player_xp_for_level"
			if result, err := s.WorldStore.DB.QueryContext(ctx, query); err == nil {
				for result.Next() {
					var level, experience int64
					if result.Scan(&level, &experience) == nil && level > 0 && level < int64(len(loaded)) {
						loaded[level] = uint32(experience)
					}
				}
				result.Close()
				any := false
				for _, v := range loaded {
					if v != 0 {
						any = true
						break
					}
				}
				if !any {
					copy(loaded, xpCurve[:])
				}
			}
			xpTableMu.Lock()
			xpTableRows = loaded
			xpTableInit = true
			xpTableMu.Unlock()
			rows = loaded
		}
		if rows != nil {
			return rows[level]
		}
	}
	return xpCurve[level]
}

// baseXPForLevel loads exploration_basexp (TC's _baseXPTable) for the kill
// XP formula; falls back to the curve above when the table has no row.
func (s *Server) baseXPForLevel(ctx context.Context, level uint32) uint32 {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return xpCurve[level]
	}
	var base int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT basexp FROM exploration_basexp WHERE level = ?", level).Scan(&base); err != nil || base <= 0 {
		fallback := xpCurve[level]
		if fallback == 0 {
			return 0
		}
		return fallback / 100
	}
	return uint32(base)
}

// grayLevel ports Formulas::XP::GetGrayLevel.
func grayLevel(playerLevel uint32) uint32 {
	switch {
	case playerLevel <= 5:
		return 0
	case playerLevel <= 39:
		return playerLevel - 5 - playerLevel/10
	case playerLevel <= 59:
		return playerLevel - 1 - playerLevel/5
	default:
		return playerLevel - 9
	}
}

// zeroDifference ports Formulas::XP::GetZeroDifference.
func zeroDifference(playerLevel uint32) uint32 {
	switch {
	case playerLevel < 8:
		return 5
	case playerLevel < 10:
		return 6
	case playerLevel < 12:
		return 7
	case playerLevel < 16:
		return 8
	case playerLevel < 20:
		return 9
	case playerLevel < 30:
		return 11
	case playerLevel < 40:
		return 12
	case playerLevel < 45:
		return 13
	case playerLevel < 50:
		return 14
	case playerLevel < 55:
		return 15
	case playerLevel < 60:
		return 16
	default:
		return 17
	}
}

// killXPGain ports Formulas::XP::BaseGain for solo kills.
func (s *Server) killXPGain(ctx context.Context, playerLevel, mobLevel uint32) uint32 {
	baseExp := s.baseXPForLevel(ctx, mobLevel)
	if baseExp == 0 {
		return 0
	}
	if mobLevel >= playerLevel {
		levelDiff := mobLevel - playerLevel
		if levelDiff > 4 {
			levelDiff = 4
		}
		return ((playerLevel*5+baseExp)*(20+levelDiff)/10 + 1) / 2
	}
	if mobLevel > grayLevel(playerLevel) {
		zd := zeroDifference(playerLevel)
		return (playerLevel*5 + baseExp) * (zd + mobLevel - playerLevel) / zd
	}
	return 0
}

// onCreatureKilled runs the full death chain for a melee kill: XP and
// level-ups, lootable corpse flag, respawn scheduling and quest credit.
func (s *session) onCreatureKilled(ctx context.Context, target combatTarget) {
	if s.player == nil {
		return
	}
	creatureEntry := uint32((target.GUID >> 24) & 0xFFFFFF)
	guid := uint32(target.GUID & 0x00FFFFFF)
	now := time.Now()

	// XP with the reference gray/zero-difference curve.
	var mobLevel int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(maxlevel, 0), minlevel, 1) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&mobLevel)
	} else {
		mobLevel = int64(target.Level)
		if mobLevel == 0 {
			mobLevel = 1
		}
	}
	if s.server != nil {
		standardGUID := creatureWorldGUID(guid, creatureEntry)
		s.server.lootMu.Lock()
		if s.server.creatureLootOwners == nil {
			s.server.creatureLootOwners = make(map[uint64]lootOwnerState)
		}
		owner := lootOwnerState{PlayerGUID: s.playerGUID, GroupID: s.groupID}
		s.server.creatureLootOwners[target.GUID] = owner
		s.server.creatureLootOwners[standardGUID] = owner
		s.server.lootMu.Unlock()
		xp := s.server.killXPGain(ctx, uint32(s.player.Level), uint32(mobLevel))
		if xp > 0 {
			s.grantXPWithVictim(ctx, xp, target.GUID)
		}

		// Mark the corpse lootable for everyone in range (dynamic flags update).
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_, _ = s.server.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = 0 WHERE guid = ?", guid)
		}
		s.server.broadcastCreatureValuesUpdate(s.player.Map, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})

		// Schedule the respawn with the spawn's original health.
		s.server.scheduleCreatureRespawn(ctx, guid, uint32(math.Max(float64(target.Health), 1)), now)
	}

	// Quest kill credit: RequiredNpcOrGo entries plus KillCredit templates.
	s.creditQuestKills(ctx, creatureEntry, target.GUID)
	s.updateAchievementCriteria(criteriaTypeKillCreature, creatureEntry, 1)
	var creatureType uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&creatureType)
	}
	if creatureType > 0 {
		s.updateAchievementCriteria(criteriaTypeKillCreatureType, creatureType, 1)
	}
	// Complete raid criteria when in a raid group
	if s.groupID != 0 && s.server != nil {
		if grp := s.server.getGroup(s.groupID); grp != nil && grp.IsRaid {
			s.updateAchievementCriteria(criteriaTypeCompleteRaid, uint32(len(grp.Members)), 1)
		}
	}
	// Mal'Ganis defeated (Heroic & Normal CoT: Stratholme, entry 26533)
	if creatureEntry == 26533 {
		s.updateAchievementCriteria(criteriaTypeMalGanisDefeated, creatureEntry, 1)
	}
	s.startTimedAchievement(timedTypeCreature, creatureEntry)

	// Clear any active auras/DoTs ticking on this creature
	if s.server != nil {
		s.server.clearCreatureAuras(target.GUID)
	}

	// Alterac Valley (Map 30) creature kills (Generals, Captains, Mine bosses)
	if s.server != nil && s.player.Map == 30 {
		s.server.handleAVCreatureKilled(s, creatureEntry)
	}

	// Strand of the Ancients (Map 607) creature kills (Demolishers)
	if s.server != nil && s.player.Map == 607 {
		s.server.handleSACreatureKilled(s, creatureEntry)
	}

	// Isle of Conquest (Map 628) creature kills (Bosses & Vehicles)
	if s.server != nil && s.player.Map == 628 {
		s.server.handleICCreatureKilled(s, creatureEntry)
	}
}

// grantXP applies XP with repeated level-ups and updates the client fields.
func (s *session) grantXP(ctx context.Context, amount uint32) {
	s.grantXPWithVictim(ctx, amount, 0)
}

// grantXPWithVictim applies XP with combat log SMSG_LOG_XPGAIN, repeated level-ups, and client field updates.
func (s *session) grantXPWithVictim(ctx context.Context, amount uint32, victimGUID uint64) {
	if s.player == nil || amount == 0 || s.player.Level >= 80 {
		return
	}

	// SMSG_LOG_XPGAIN (0x1D0): victimGUID (8), totalXP (4), type (1), [baseXP (4), groupRate (4)], rafBonus (1)
	xpLog := protocol.NewBuffer(22)
	xpLog.WriteU64(victimGUID)
	xpLog.WriteU32(amount)
	if victimGUID != 0 {
		xpLog.WriteU8(0)       // 0 = kill XP
		xpLog.WriteU32(amount) // base XP
		xpLog.WriteF32(1.0)    // group rate
	} else {
		xpLog.WriteU8(1) // 1 = non-kill XP (quest, exploration)
	}
	xpLog.WriteU8(0) // recruit-a-friend flag
	_ = s.write(uint16(protocol.OpcodeSMSG_LOG_XPGAIN), xpLog.Bytes(), true)

	s.player.XP += amount
	for s.player.Level < 80 {
		needed := s.server.xpForLevel(ctx, uint32(s.player.Level))
		if needed == 0 || s.player.XP < needed {
			break
		}
		s.player.XP -= needed
		oldHP := s.player.MaxHealth
		oldMana := s.player.MaxPowers[0]
		oldStats := s.player.Stats
		s.player.Level++
		s.setAchievementCriteria(criteriaTypeReachLevel, uint32(s.player.Level), uint32(s.player.Level))
		_ = s.calculatePlayerStats(ctx, s.player)
		s.player.Health = s.player.MaxHealth
		if len(s.player.MaxPowers) > 0 {
			s.player.Powers[0] = s.player.MaxPowers[0]
		}
		healthDelta := uint32(0)
		if s.player.MaxHealth > oldHP {
			healthDelta = s.player.MaxHealth - oldHP
		}
		manaDelta := uint32(0)
		if s.player.MaxPowers[0] > oldMana {
			manaDelta = s.player.MaxPowers[0] - oldMana
		}
		// SMSG_LEVELUP_INFO: 56 bytes (level, healthDelta, 7 powerDeltas, 5 statDeltas)
		levelPacket := protocol.NewBuffer(56)
		levelPacket.WriteU32(uint32(s.player.Level))
		levelPacket.WriteU32(healthDelta)
		levelPacket.WriteU32(manaDelta) // PowerDelta[0] (Mana)
		for i := 1; i < 7; i++ {
			levelPacket.WriteU32(0) // PowerDelta[1..6]
		}
		for i := 0; i < 5; i++ {
			statDelta := uint32(0)
			if s.player.Stats[i] > oldStats[i] {
				statDelta = s.player.Stats[i] - oldStats[i]
			}
			levelPacket.WriteU32(statDelta) // StatDelta[0..4]: Str, Agi, Sta, Int, Spi
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_LEVELUP_INFO), levelPacket.Bytes(), true)
		for i := range s.player.Skills {
			maxSkill := uint16(s.player.Level) * 5
			if s.player.Skills[i].Max < maxSkill && s.player.Skills[i].Max > 0 {
				s.player.Skills[i].Max = maxSkill
			}
		}
		if s.player.Level >= 10 {
			_ = s.sendTalentsInfo(false)
		}
		s.updatePetOnLevelUp(ctx)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET level = ?, xp = ?, health = ? WHERE guid = ?", s.player.Level, s.player.XP, s.player.Health, s.playerGUID)
			for _, sk := range s.player.Skills {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET max = ? WHERE guid = ? AND skill = ?", sk.Max, s.playerGUID, sk.Skill)
			}
		}
	}
	s.sendPlayerUpdate()
}

// broadcastCreatureValueUpdate pushes an UPDATETYPE_VALUES block for a
// creature to sessions within visibility range.
func (s *Server) broadcastCreatureValueUpdate(guid uint64, mapID uint32, x, y float32, fields map[int]uint32) {
	s.broadcastCreatureValuesUpdate(mapID, guid, fields)
}

// scheduleCreatureRespawn stores spawn health to restore after
// creature.spawntimesecs like Map::AddToActiveMap respawn handling.
func (s *Server) scheduleCreatureRespawn(ctx context.Context, guid, health uint32, now time.Time) {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var seconds int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(spawntimesecs, 0), 300) FROM creature WHERE guid = ?", guid).Scan(&seconds); err != nil || seconds <= 0 {
		seconds = 300
	}
	var entry, mapID int64
	var x, y, z float64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map, position_x, position_y, position_z FROM creature WHERE guid = ?", guid).Scan(&entry, &mapID, &x, &y, &z)
	s.motionMu.Lock()
	if s.creatureRespawns == nil {
		s.creatureRespawns = make(map[uint32]creatureRespawn)
	}
	s.creatureRespawns[guid] = creatureRespawn{GUID: guid, At: now.Add(time.Duration(seconds) * time.Second), Health: health, Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z)}
	s.motionMu.Unlock()
}

// processCreatureRespawns restores expired spawns; called from world tick.
func (s *Server) processCreatureRespawns(ctx context.Context, now time.Time) {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.motionMu.Lock()
	due := make([]creatureRespawn, 0, 8)
	for guid, respawn := range s.creatureRespawns {
		if now.After(respawn.At) {
			due = append(due, respawn)
			delete(s.creatureRespawns, guid)
		}
	}
	s.motionMu.Unlock()
	for _, respawn := range due {
		if _, err := s.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = ? WHERE guid = ?", respawn.Health, respawn.GUID); err != nil {
			continue
		}
		if respawn.Entry != 0 {
			rawGUID := creatureWorldGUID(respawn.GUID, respawn.Entry)
			s.motionMu.Lock()
			if motion := s.creatureMotion[rawGUID]; motion != nil {
				motion.Health = respawn.Health
				motion.MaxHealth = respawn.Health
				motion.X, motion.Y, motion.Z = respawn.X, respawn.Y, respawn.Z
				motion.InCombat, motion.TargetGUID, motion.Moving = false, 0, false
			}
			s.motionMu.Unlock()
			s.lootMu.Lock()
			delete(s.creatureLoot, rawGUID)
			delete(s.creatureLootOwners, rawGUID)
			s.lootMu.Unlock()
			s.broadcastCreatureValuesUpdate(respawn.Map, rawGUID, map[int]uint32{unitFieldHealth: respawn.Health, unitFieldDynamicFlags: 0})
		}
	}
}

// creditQuestKills advances RequiredNpcOrGo objectives for quests in the
// log, sending SMSG_QUESTUPDATE_ADD_KILL per TC
// Player::KilledMonster / SendQuestUpdateAddCreatureOrGo.
func (s *session) creditQuestKills(ctx context.Context, creatureEntry uint32, victimGUID uint64) {
	if s.player == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	// KillCredit templates redirect credit to another entry.
	var credits [2]int64
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(KillCredit1, 0), COALESCE(KillCredit2, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&credits[0], &credits[1])
	entries := []uint32{creatureEntry}
	for _, credit := range credits {
		if credit > 0 {
			entries = append(entries, uint32(credit))
		}
	}
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		entry := s.player.QuestLog[slot]
		if entry.QuestID == 0 {
			continue
		}
		var reqIDs, reqCounts [4]int64
		err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4, RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4 FROM quest_template WHERE ID = ?", entry.QuestID).Scan(&reqIDs[0], &reqIDs[1], &reqIDs[2], &reqIDs[3], &reqCounts[0], &reqCounts[1], &reqCounts[2], &reqCounts[3])
		if err != nil {
			continue
		}
		progressed := false
		for objective := 0; objective < 4; objective++ {
			required := uint32(reqCounts[objective])
			if required == 0 || reqIDs[objective] == 0 {
				continue
			}
			match := false
			for _, e := range entries {
				if uint32(reqIDs[objective]) == e {
					match = true
					break
				}
			}
			if !match {
				continue
			}
			if uint32(entry.Counters[objective]) >= required {
				continue
			}
			entry.Counters[objective]++
			progressed = true
			update := protocol.NewBuffer(24)
			update.WriteU32(entry.QuestID)
			update.WriteU32(uint32(reqIDs[objective]))
			update.WriteU32(uint32(entry.Counters[objective]))
			update.WriteU32(required)
			update.WriteU64(victimGUID)
			_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_ADD_KILL), update.Bytes(), true)
			countColumn := "mobcount" + string(rune('1'+objective))
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET "+countColumn+" = ? WHERE guid = ? AND quest = ?", int64(entry.Counters[objective]), s.playerGUID, entry.QuestID)
		}
		if !progressed {
			continue
		}
		s.player.QuestLog[slot] = entry
		s.sendPlayerQuestLogUpdate(slot)
		if s.questObjectivesComplete(ctx, entry.QuestID, entry) {
			entry.State = 1
			s.player.QuestLog[slot] = entry
			s.sendPlayerQuestLogUpdate(slot)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, entry.QuestID)
			_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_COMPLETE), nil, true)
		}
	}
}

// questObjectivesComplete checks kill and item objectives against the log
// entry and character inventory.
func (s *session) questObjectivesComplete(ctx context.Context, questID uint32, entry questLogEntry) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var reqIDs, reqCounts [4]int64
	var itemIDs, itemCounts [6]int64
	var requiredPlayerKills, timeLimit, requiredMoney, repFaction, repValue int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4, RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4, RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6, RequiredItemCount1, RequiredItemCount2, RequiredItemCount3, RequiredItemCount4, RequiredItemCount5, RequiredItemCount6, RequiredPlayerKills, TimeLimit, RewOrReqMoney, RepObjectiveFaction, RepObjectiveValue FROM quest_template WHERE ID = ?", questID).Scan(&reqIDs[0], &reqIDs[1], &reqIDs[2], &reqIDs[3], &reqCounts[0], &reqCounts[1], &reqCounts[2], &reqCounts[3], &itemIDs[0], &itemIDs[1], &itemIDs[2], &itemIDs[3], &itemIDs[4], &itemIDs[5], &itemCounts[0], &itemCounts[1], &itemCounts[2], &itemCounts[3], &itemCounts[4], &itemCounts[5], &requiredPlayerKills, &timeLimit, &requiredMoney, &repFaction, &repValue)
	if err != nil {
		return false
	}
	var specialFlags int64
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	for objective := 0; objective < 4; objective++ {
		if reqCounts[objective] > 0 && uint32(entry.Counters[objective]) < uint32(reqCounts[objective]) {
			return false
		}
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		for index := 0; index < 6; index++ {
			if itemIDs[index] == 0 || itemCounts[index] == 0 {
				continue
			}
			var have int64
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?", s.playerGUID, itemIDs[index]).Scan(&have)
			if have < itemCounts[index] {
				return false
			}
		}
	}
	if requiredPlayerKills > 0 && int64(entry.PlayerCount) < requiredPlayerKills || timeLimit > 0 && entry.Timer == 0 || specialFlags&0x02 != 0 && !entry.Explored {
		return false
	}
	if requiredMoney < 0 && (s.player == nil || int64(s.player.Money) < -requiredMoney) {
		return false
	}
	if repFaction > 0 && s.player != nil {
		standing := int64(0)
		for _, reputation := range s.player.Reputations {
			if int64(reputation.FactionID) == repFaction {
				standing = int64(totalReputationStanding(reputation))
				break
			}
		}
		if standing < repValue {
			return false
		}
	}
	return true
}

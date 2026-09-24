package world

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureTypeCritter         uint32 = 8
	creatureTypeTotem           uint32 = 11
	creatureFlagExtraNoXPAtKill uint32 = 0x00000040
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

func ResolveGroupXPRate(count uint32, raid bool) float32 {
	if raid || count <= 2 {
		return 1
	}
	switch count {
	case 3:
		return 1.166
	case 4:
		return 1.3
	default:
		return 1.4
	}
}

func ResolveGroupXPShare(baseXP, playerLevel, sumLevel, maxLevel, maxNonGrayLevel uint32, groupRate float32) uint32 {
	if baseXP == 0 || playerLevel == 0 || sumLevel == 0 || maxNonGrayLevel == 0 || playerLevel > maxNonGrayLevel {
		return 0
	}
	rate := groupRate * float32(playerLevel) / float32(sumLevel)
	share := float32(baseXP) * rate
	if maxLevel != maxNonGrayLevel {
		return uint32(share/2) + 1
	}
	return uint32(share)
}

func (s *Server) adjustCreatureKillXP(ctx context.Context, target combatTarget, creatureEntry, baseXP uint32) uint32 {
	if s == nil || baseXP == 0 {
		return 0
	}
	var creatureType, rank, flagsExtra int64
	experienceModifier := float64(1)
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0), COALESCE(rank, 0), COALESCE(ExperienceModifier, 1), COALESCE(flags_extra, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&creatureType, &rank, &experienceModifier, &flagsExtra)
	}
	if uint32(creatureType) == creatureTypeCritter || uint32(creatureType) == creatureTypeTotem || uint32(flagsExtra)&creatureFlagExtraNoXPAtKill != 0 {
		return 0
	}
	s.motionMu.Lock()
	motion := s.creatureMotion[target.GUID]
	isPet := motion != nil && motion.PetID != 0 && motion.OwnerGUID != 0
	s.motionMu.Unlock()
	if isPet {
		return 0
	}
	multiplier := float32(1)
	var isDungeon, isBattleground bool
	if s.Data != nil {
		if mapInfo, found, err := s.Data.Map(target.Map); err == nil && found {
			isDungeon, isBattleground = mapInfo.IsDungeon(), mapInfo.IsBattleground()
		}
	}
	if rank > 0 {
		if isDungeon {
			multiplier *= 2.75
		} else {
			multiplier *= 2
		}
	}
	multiplier *= float32(experienceModifier)
	if isBattleground {
		multiplier *= float32(s.Config.XPRateBattlegroundKill)
	} else {
		multiplier *= float32(s.Config.XPRateKill)
	}
	return uint32(float32(baseXP) * multiplier)
}

func ResolveGroupXPMapEligibility(memberMap, rewardMap, memberInstance, rewardInstance uint32) bool {
	return memberMap == rewardMap && memberInstance == rewardInstance
}

func ResolveGroupXPDistance(distance, memberReach, rewardReach float64) float64 {
	if memberReach <= 0 {
		memberReach = 1.5
	}
	if rewardReach <= 0 {
		rewardReach = 1.5
	}
	distance -= memberReach + rewardReach
	if distance < 0 {
		return 0
	}
	return distance
}

func ResolveNpcBotXPGain(xp uint32, botCount, reduction uint8) uint32 {
	if botCount <= 1 || reduction == 0 {
		return xp
	}
	rate := 100 - int(botCount-1)*int(reduction)
	if rate < 10 {
		rate = 10
	}
	return xp * uint32(rate) / 100
}

func (s *session) applyNpcBotXPReduction(xp uint32) uint32 {
	if xp == 0 || s == nil || s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		return xp
	}
	return ResolveNpcBotXPGain(xp, s.server.runtimeNpcBotCountByOwner(s.playerGUID), uint8(s.server.Config.NPCBots.XPReduction))
}

func (s *Server) atGroupKillRewardDistance(member *session, target combatTarget, dungeon bool, rewardInstance uint32) bool {
	if s == nil || member == nil || member.player == nil || !ResolveGroupXPMapEligibility(member.player.Map, target.Map, member.player.InstanceID, rewardInstance) {
		return false
	}
	distance := ResolveGroupXPDistance(distance3D(member.player.X, member.player.Y, member.player.Z, target.X, target.Y, target.Z), float64(member.player.CombatReach), float64(target.CombatReach))
	return dungeon || distance <= s.Config.MaxGroupXPDistance
}

func (s *session) experienceAuraMultiplier() float32 {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return 1
	}
	modifiers := make([]PetFocusModifier, 0)
	type effectKey struct {
		SpellID uint32
		Effect  uint8
	}
	seen := make(map[effectKey]struct{})
	appendSpell := func(spellID uint32, spell wotlk.Spell, mask uint8, current *activeAura) {
		for index, effect := range spell.Effects {
			if effect.Effect == 0 || effect.Aura != 200 || mask&(1<<uint(index)) == 0 {
				continue
			}
			key := effectKey{SpellID: spellID, Effect: uint8(index)}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			amount := effect.BasePoints + 1
			if current != nil {
				amount = current.Amounts[index]
				if amount == 0 && current.AuraType == effect.Aura && current.Amount != 0 {
					amount = int32(current.Amount)
				}
				if amount == 0 && current.BaseAmounts[index] == 0 && effect.BasePoints != -1 {
					amount = effect.BasePoints + 1
				}
			}
			modifiers = append(modifiers, PetFocusModifier{SpellID: spellID, Effect: uint8(index), AuraType: effect.Aura, Amount: amount, StackGroup: s.server.petAuraStackGroup(spellID, effect.Aura)})
		}
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		if spell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
			appendSpell(aura.SpellID, spell, aura.EffectMask, aura)
		}
	}
	for _, learned := range s.player.Spells {
		if !learned.Active || learned.Disabled {
			continue
		}
		if spell, found, err := s.server.Data.Spell(learned.ID); err == nil && found && spell.Attributes&spellAttributePassive != 0 {
			appendSpell(learned.ID, spell, 0x07, nil)
		}
	}
	sort.SliceStable(modifiers, func(i, j int) bool {
		if modifiers[i].SpellID != modifiers[j].SpellID {
			return modifiers[i].SpellID < modifiers[j].SpellID
		}
		return modifiers[i].Effect < modifiers[j].Effect
	})
	return ResolveAuraPercentMultiplierByType(modifiers, spellAuraIncreaseXPPercent)
}

func (s *session) rewardCreatureKillXP(ctx context.Context, target combatTarget, creatureEntry, mobLevel uint32) {
	if s == nil || s.server == nil || s.player == nil || mobLevel == 0 {
		return
	}
	group := s.server.getGroup(s.groupID)
	if group == nil {
		baseXP := s.server.killXPGain(ctx, uint32(s.player.Level), mobLevel)
		xp := s.server.adjustCreatureKillXP(ctx, target, creatureEntry, baseXP)
		if xp > 0 {
			xp = uint32(float32(xp) * s.experienceAuraMultiplier())
			xp = s.applyNpcBotXPReduction(xp)
			if xp > 0 {
				s.grantXPWithVictimGroup(ctx, xp, target.GUID, false)
			}
		}
		return
	}
	s.server.groupsMu.RLock()
	members := append([]groupMember(nil), group.Members...)
	isRaidGroup := group.IsRaid
	s.server.groupsMu.RUnlock()
	targetMapIsDungeon, targetMapIsRaid, targetMapIsBattleground := false, false, false
	if s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(target.Map); err == nil && found {
			targetMapIsDungeon, targetMapIsRaid, targetMapIsBattleground = mapInfo.IsDungeon(), mapInfo.IsRaid(), mapInfo.IsBattleground()
		}
	}
	type recipient struct {
		session *session
		level   uint32
	}
	recipients := make([]recipient, 0, len(members))
	seen := make(map[uint64]struct{}, len(members))
	count, sumLevel, maxLevel, maxNonGrayLevel := uint32(0), uint32(0), uint32(0), uint32(0)
	addRecipient := func(member *session, killer bool) {
		if member == nil || member.player == nil || !member.playerLoaded {
			return
		}
		guid := member.playerGUID
		if _, exists := seen[guid]; exists {
			return
		}
		seen[guid] = struct{}{}
		if !killer && !s.server.atGroupKillRewardDistance(member, target, targetMapIsDungeon, s.player.InstanceID) {
			return
		}
		if member.player.Health == 0 {
			return
		}
		level := uint32(member.player.Level)
		if level == 0 {
			return
		}
		recipients = append(recipients, recipient{session: member, level: level})
		count++
		sumLevel += level
		if level > maxLevel {
			maxLevel = level
		}
		if mobLevel > grayLevel(level) && level > maxNonGrayLevel {
			maxNonGrayLevel = level
		}
	}
	killerFound := false
	for _, member := range members {
		if member.GUID == s.playerGUID {
			killerFound = true
		}
		addRecipient(s.server.findSessionByGUID(member.GUID), member.GUID == s.playerGUID)
	}
	if !killerFound {
		addRecipient(s, true)
	}
	if count == 0 || sumLevel == 0 || maxNonGrayLevel == 0 {
		return
	}
	baseXP := s.server.killXPGain(ctx, maxNonGrayLevel, mobLevel)
	baseXP = s.server.adjustCreatureKillXP(ctx, target, creatureEntry, baseXP)
	if baseXP == 0 {
		return
	}
	groupRate := float32(1)
	if !targetMapIsBattleground {
		groupRate = ResolveGroupXPRate(count, targetMapIsRaid && isRaidGroup)
	}
	for _, member := range recipients {
		share := ResolveGroupXPShare(baseXP, member.level, sumLevel, maxLevel, maxNonGrayLevel, groupRate)
		if share == 0 {
			continue
		}
		share = uint32(float32(share) * member.session.experienceAuraMultiplier())
		share = member.session.applyNpcBotXPReduction(share)
		if share > 0 {
			member.session.grantXPWithVictimGroup(ctx, share, target.GUID, true)
		}
	}
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
	mobLevel := uint32(target.Level)
	if mobLevel == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var fallbackLevel int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(maxlevel, 0), minlevel, 1) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&fallbackLevel)
		if fallbackLevel > 0 {
			mobLevel = uint32(fallbackLevel)
		}
	}
	if mobLevel == 0 {
		mobLevel = 1
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
		s.rewardCreatureKillXP(ctx, target, creatureEntry, mobLevel)

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
	if s == nil {
		return
	}
	s.grantXPWithVictimGroup(ctx, amount, victimGUID, s.groupID != 0)
}

func (s *session) grantXPWithVictimGroup(ctx context.Context, amount uint32, victimGUID uint64, grouped bool) {
	if s.player == nil || amount == 0 {
		return
	}
	petXP := amount
	if grouped {
		petXP /= 2
	}
	grantPetXP := func() {
		if victimGUID != 0 {
			s.giveHunterPetXP(ctx, petXP)
		}
	}
	maxPlayerLevel := uint8(s.server.Config.MaxPlayerLevel)
	if s.player.Level >= maxPlayerLevel {
		grantPetXP()
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
	for s.player.Level < maxPlayerLevel {
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
	grantPetXP()
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
func (s *session) creditPlayerKillQuest(ctx context.Context) {
	if s == nil || s.player == nil || s.bgData.InstanceID != 0 || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for slot, entry := range s.player.QuestLog {
		if entry.QuestID == 0 || entry.State != 0 {
			continue
		}
		var required, zone, questType int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT RequiredPlayerKills, ZoneOrSort, QuestType FROM quest_template WHERE ID = ?", entry.QuestID).Scan(&required, &zone, &questType); err != nil || required <= 0 || zone != int64(s.player.Zone) || int64(entry.PlayerCount) >= required || !s.questAllowedInRaid(uint32(questType)) {
			continue
		}
		entry.PlayerCount++
		entry.Counters[0] = entry.PlayerCount
		s.player.QuestLog[slot] = entry
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET playercount = ? WHERE guid = ? AND quest = ?", entry.PlayerCount, s.playerGUID, entry.QuestID)
		packet := protocol.NewBuffer(12)
		packet.WriteU32(entry.QuestID)
		packet.WriteU32(uint32(entry.PlayerCount))
		packet.WriteU32(uint32(required))
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_ADD_PVP_KILL), packet.Bytes(), true)
		s.sendPlayerQuestLogUpdate(slot)
		if s.questObjectivesComplete(ctx, entry.QuestID, entry) {
			entry.State = questCompleteStateFlag(questStatusComplete)
			s.player.QuestLog[slot] = entry
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, entry.QuestID)
			s.sendPlayerQuestLogUpdate(slot)
			_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_COMPLETE), nil, true)
		}
		break
	}
}

func (s *session) questAllowedInRaid(questType uint32) bool {
	if s == nil || s.player == nil || s.server == nil {
		return true
	}
	groupRaid := false
	if s.groupID != 0 {
		if group := s.server.findGroupByID(s.groupID); group != nil {
			groupRaid = group.IsRaid
		}
	}
	return QuestAllowedInRaid(questType, s.player.RaidDifficulty, groupRaid, s.server.Config.QuestIgnoreRaid)
}

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

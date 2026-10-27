package world

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureTypeCritter         uint32 = 8
	creatureTypeTotem           uint32 = 11
	creatureFlagExtraNoXPAtKill uint32 = 0x00000040
)

// creatureFlagExtraGuard mirrors CREATURE_FLAG_EXTRA_GUARD
// (CreatureData.h): the GuardAI::Permissible gate (GuardAI.cpp:29-35).
const creatureFlagExtraGuard uint32 = 0x00000080

// creatureFlagExtraTauntDiminish mirrors CREATURE_FLAG_EXTRA_TAUNT_DIMINISH
// (CreatureData.h:56): only creatures carrying this flag take the
// 0.65/0.4225/0.274625 taunt duration ladder
// (Unit::ApplyDiminishingToDuration, Unit.cpp:9064-9082). Go runs the DR
// duration path for player targets only, so this is documented for the
// creature path, not yet consumed.
const creatureFlagExtraTauntDiminish uint32 = 0x00080000

// spellAttr0CastableWhileDead mirrors SPELL_ATTR0_CASTABLE_WHILE_DEAD
// (SharedDefines.h:435): the UnitAI::FillAISpellInfo AICOND_DIE gate
// (UnitAI.cpp:196) bridged in onCreatureKilled.
const spellAttr0CastableWhileDead uint32 = 0x00800000

// unitDynFlagLootable marks a corpse lootable in UNIT_FIELD_DYNAMIC_FLAGS.
const unitDynFlagLootable uint32 = 0x00000001

// unitDynFlagTapped mirrors UNIT_DYNFLAG_TAPPED (SharedDefines.h:3130):
// set by Creature::SetLootRecipient on the first player-attributed hit.
const unitDynFlagTapped uint32 = 0x00000004

// unitDynFlagDead mirrors UNIT_DYNFLAG_DEAD (SharedDefines.h:3133):
// the feign-death corpse flag (AuraEffect::HandleFeignDeath).
const unitDynFlagDead uint32 = 0x00000020

// unitDynFlagTrackUnit mirrors UNIT_DYNFLAG_TRACK_UNIT (SharedDefines.h:3129):
// set by AuraEffect::HandleAuraModStalked (SpellAuraEffects.cpp) on a unit
// carrying a live 68-effect aura (Hunter's Mark etc.).
const unitDynFlagTrackUnit uint32 = 0x00000002

// creatureFlagExtraNoPlayerDamageReq mirrors
// CREATURE_FLAG_EXTRA_NO_PLAYER_DAMAGE_REQ (CreatureData.h:58): the
// creature needs no player damage for kill credit.
const creatureFlagExtraNoPlayerDamageReq uint32 = 0x00200000

// recordCreatureTap mirrors the tap block of Unit::DealDamage
// (Unit.cpp:872-876): the first player-attributed damage sets the loot
// recipient (Creature::SetLootRecipient, Creature.cpp:1328-1355 — the
// charmer/owner-resolved player GUID plus group, UNIT_DYNFLAG_TAPPED;
// a non-player, non-vehicle attacker taps nothing, Creature.cpp:1342-1347)
// and lowers m_PlayerDamageReq (Creature::LowerPlayerDamageReq,
// Creature.cpp:1577-1582, capped at the pre-hit health) — but only when the
// attacker counts: the !attacker / IsControlledByPlayer / player-temp-summon
// gate (Unit.cpp:873-876). countsForDamageReq carries that gate: true for
// player, pet, and player-controlled/charm attackers; false for
// wild-creature damage (shield retaliation, shared damage), which in C++
// leaves the requirement standing. Call with motionMu held. Returns true
// when the tap is newly set — the caller broadcasts the dynamic-flags
// update after unlocking. A zero tap playerGUID (the C++ !attacker leg)
// lowers the requirement but sets no recipient.
func (s *Server) recordCreatureTap(motion *creatureMotion, playerGUID, groupID uint64, damage, healthBefore uint32, countsForDamageReq bool) bool {
	if motion == nil {
		return false
	}
	newlyTapped := false
	if playerGUID != 0 && motion.TapPlayerGUID == 0 {
		motion.TapPlayerGUID = playerGUID
		motion.TapGroupID = groupID
		motion.DynamicFlags |= unitDynFlagTapped
		newlyTapped = true
	}
	if countsForDamageReq {
		dealt := damage
		if healthBefore < damage {
			dealt = healthBefore
		}
		if motion.PlayerDamageReq > dealt {
			motion.PlayerDamageReq -= dealt
		} else {
			motion.PlayerDamageReq = 0
		}
	}
	return newlyTapped
}

// tapAttackerAttribution resolves the C++ tap attribution for recursive
// damage paths (damage-shield retaliation, shared damage) whose attacker is
// not the direct session: Creature::SetLootRecipient (Creature.cpp:1342-1347)
// taps nothing for a non-player, non-vehicle attacker — the recipient is the
// attacker's charmer/owner player or the player itself — and the
// LowerPlayerDamageReq arm (Unit.cpp:872-876) counts only a null attacker,
// a player-controlled attacker (Unit.cpp:6056 minions; 6219/6259/6265 player
// charms — Player.cpp:389 sets m_ControlledByPlayer for players), or a temp
// summon with a player summoner. A creature-charmed attacker
// (Unit.cpp:6224/6271) is not player-controlled: the owner lookup below
// yields nothing and the hit neither taps nor lowers the requirement.
// Returns the tap player (0 = no tap), its group, and whether the hit
// lowers the damage requirement. Callers hold motionMu; the owner session
// lookup follows the in-tree findSessionByGUID-under-motionMu precedent
// (rage.go).
func (s *Server) tapAttackerAttribution(attackerGUID uint64, attackerSess *session, attackerMotion *creatureMotion) (tapPlayer, tapGroup uint64, counts bool) {
	if attackerGUID == 0 {
		// Unit.cpp:874 !attacker leg: no recipient, but the requirement lowers.
		return 0, 0, true
	}
	if attackerSess != nil && attackerSess.player != nil {
		return attackerSess.playerGUID, attackerSess.groupID, true
	}
	if attackerMotion == nil {
		return 0, 0, false
	}
	// Player-controlled attacker (pet, player charm) or temp summon: the tap
	// player is the controlling/summoning player via the motion's OwnerGUID.
	if attackerMotion.UnitFlags&unitFlagPlayerControlled != 0 || attackerMotion.TempSummonLifetime > 0 {
		if owner := s.findSessionByGUID(attackerMotion.OwnerGUID); owner != nil && owner.player != nil {
			return owner.playerGUID, owner.groupID, true
		}
	}
	return 0, 0, false
}

// creatureRespawn records when a killed spawn restores its health.
type creatureRespawn struct {
	GUID       uint32
	At         time.Time
	Health     uint32
	Entry      uint32
	Map        uint32
	InstanceID uint32
	X, Y, Z    float32
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

// xpBoostActiveToday ports the IsXPBoostActive gate of boosted_xp.cpp: the
// XP.Boost.Daymask bit for today's weekday must be set (1 = Sunday, matching
// tm_wday from TimeBreakdown, i.e. Go's time.Weekday numbering).
func xpBoostActiveToday(daymask uint32) bool {
	return daymask&(1<<uint(time.Now().Weekday())) != 0
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

// creatureIsRacialLeader mirrors Creature::IsRacialLeader (Creature.h:95):
// the creature_template.RacialLeader flag drives the "HK: Leader" honor arm
// of Player::RewardHonor on creature kills.
func (sv *Server) creatureIsRacialLeader(ctx context.Context, entry uint32) bool {
	if sv == nil || sv.WorldStore == nil || sv.WorldStore.DB == nil || entry == 0 {
		return false
	}
	var racialLeader int64
	if err := sv.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(RacialLeader, 0) FROM creature_template WHERE entry = ?", entry).Scan(&racialLeader); err != nil {
		return false
	}
	return racialLeader != 0
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
	motion := s.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID)
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
			amount := effect.BasePoints
			if current != nil {
				amount = current.Amounts[index]
				if amount == 0 && current.AuraType == effect.Aura && current.Amount != 0 {
					amount = int32(current.Amount)
				}
				if amount == 0 && current.BaseAmounts[index] == 0 && effect.BasePoints != -1 {
					amount = effect.BasePoints
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
	// Target map flags: shared by the group-rate gate and the racial-leader
	// honor arm below.
	targetMapIsDungeon, targetMapIsRaid, targetMapIsBattleground := false, false, false
	if s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(target.Map); err == nil && found {
			targetMapIsDungeon, targetMapIsRaid, targetMapIsBattleground = mapInfo.IsDungeon(), mapInfo.IsRaid(), mapInfo.IsBattleground()
		}
	}
	// KillRewarder::_InitXP (KillRewarder.cpp:119-124): off battlegrounds a
	// vehicle-mounted killer zeroes the XP init while honor, reputation and
	// kill credit still fire; on battlegrounds (XP-only rewards) the zero XP
	// skips the reward entirely.
	vehicleNoXP := s.player.VehicleGUID != 0 && !targetMapIsBattleground
	if s.player.VehicleGUID != 0 && targetMapIsBattleground {
		return
	}
	// KillRewarder::_RewardReputation (KillRewarder.cpp:179-184): skipped on
	// battlegrounds and when the victim disables reputation gain
	// (Creature::IsReputationGainDisabled, Creature.h:321).
	var killRep *onkillReputation
	if !targetMapIsBattleground && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		repDisabled := false
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			s.server.motionMu.Lock()
			repDisabled = motion.DisableReputationGain
			s.server.motionMu.Unlock()
		}
		if !repDisabled {
			killRep, _ = loadOnkillReputation(ctx, s.server.WorldStore.DB, creatureEntry)
		}
	}
	group := s.server.getGroup(s.groupID)
	if group == nil {
		if !vehicleNoXP {
			baseXP := s.server.killXPGain(ctx, uint32(s.player.Level), mobLevel)
			xp := s.server.adjustCreatureKillXP(ctx, target, creatureEntry, baseXP)
			if xp > 0 {
				xp = uint32(float32(xp) * s.experienceAuraMultiplier())
				xp = s.applyNpcBotXPReduction(xp)
				if xp > 0 {
					s.grantXPWithVictimGroup(ctx, xp, target.GUID, false)
				}
			}
		}
		// KillRewarder::_RewardHonor (KillRewarder.cpp:134-138): the
		// racial-leader honor arm also fires for solo kills.
		if s.racialLeaderHonorEligible(ctx, target, creatureEntry, targetMapIsBattleground) {
			s.grantRacialLeaderHonor(ctx, target.GUID, 1)
		}
		// KillRewarder::_RewardReputation (KillRewarder.cpp:179-184,
		// _RewardPlayer KillRewarder.cpp:218-222): solo rate is 1.0.
		if killRep != nil {
			s.rewardKillReputation(ctx, killRep, mobLevel, 1.0, target.Map)
		}
		return
	}
	s.server.groupsMu.RLock()
	members := append([]groupMember(nil), group.Members...)
	isRaidGroup := group.IsRaid
	s.server.groupsMu.RUnlock()
	type recipient struct {
		session *session
		level   uint32
	}
	recipients := make([]recipient, 0, len(members))
	seen := make(map[uint64]struct{}, len(members))
	count, sumLevel, maxLevel, maxNonGrayLevel := uint32(0), uint32(0), uint32(0), uint32(0)
	addRecipient := func(member *session, killer bool) {
		if member == nil || member.player == nil || !member.worldReady.Load() {
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
		// KillRewarder::_InitGroupData (KillRewarder.cpp:99-128): the killer
		// is counted even when dead; every other member must be alive
		// (IsAlive — dead and ghost alike are excluded).
		if !killer && member.isDeadOrGhost() {
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
	// KillRewarder::_RewardGroup 3.1.3 (KillRewarder.cpp:248-256): reputation
	// rewards every member within reward distance, dead members included
	// ("Even dead players and corpses are rewarded", KillRewarder.cpp:182) —
	// unlike XP, which skips them. The killer is always a rep recipient.
	repRecipients := make([]recipient, 0, len(members))
	repSeen := make(map[uint64]struct{}, len(members))
	addRepRecipient := func(member *session, killer bool) {
		if member == nil || member.player == nil || !member.worldReady.Load() {
			return
		}
		guid := member.playerGUID
		if _, exists := repSeen[guid]; exists {
			return
		}
		if level := uint32(member.player.Level); level > 0 {
			if killer || s.server.atGroupKillRewardDistance(member, target, targetMapIsDungeon, s.player.InstanceID) {
				repSeen[guid] = struct{}{}
				repRecipients = append(repRecipients, recipient{session: member, level: level})
			}
		}
	}
	killerFound := false
	for _, member := range members {
		isKiller := member.GUID == s.playerGUID
		if isKiller {
			killerFound = true
		}
		ms := s.server.findSessionByGUID(member.GUID)
		addRecipient(ms, isKiller)
		addRepRecipient(ms, isKiller)
	}
	if !killerFound {
		addRecipient(s, true)
		addRepRecipient(s, true)
	}
	// KillRewarder::_RewardGroup (KillRewarder.cpp:236-256): the honor arm
	// runs even when the victim is gray for every member — a null
	// _maxNotGrayMember still enters the reward loop off battlegrounds.
	if s.racialLeaderHonorEligible(ctx, target, creatureEntry, targetMapIsBattleground) {
		for _, member := range recipients {
			member.session.grantRacialLeaderHonor(ctx, target.GUID, count)
		}
	}
	groupRate := float32(1)
	if !targetMapIsBattleground {
		groupRate = ResolveGroupXPRate(count, targetMapIsRaid && isRaidGroup)
	}
	if !vehicleNoXP {
		if count == 0 || sumLevel == 0 || maxNonGrayLevel == 0 {
			return
		}
		baseXP := s.server.killXPGain(ctx, maxNonGrayLevel, mobLevel)
		baseXP = s.server.adjustCreatureKillXP(ctx, target, creatureEntry, baseXP)
		if baseXP == 0 {
			return
		}
		for _, member := range recipients {
			// KillRewarder::_RewardXP (KillRewarder.cpp:142-151): a dead
			// member's share is zeroed in the group case — no XP is granted,
			// but the member already counted in the denominators above.
			if member.session.isDeadOrGhost() {
				continue
			}
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
	// KillRewarder::_RewardReputation (KillRewarder.cpp:179-184,
	// _RewardPlayer KillRewarder.cpp:218-222): the group rate is
	// _groupRate * level / _sumLevel, 1.0 in dungeons.
	if killRep != nil && sumLevel > 0 {
		for _, member := range repRecipients {
			rate := groupRate * float32(member.level) / float32(sumLevel)
			if targetMapIsDungeon {
				rate = 1.0
			}
			member.session.rewardKillReputation(ctx, killRep, mobLevel, rate, target.Map)
		}
	}
}

// onCreatureKilled runs the full death chain for a melee kill: XP and
// level-ups, lootable corpse flag, respawn scheduling and quest credit.
// killer is the killing creature motion, or nil when the attacker is the
// player session s itself.
func (s *session) onCreatureKilled(ctx context.Context, target combatTarget, killer *creatureMotion) {
	if s.player == nil {
		return
	}
	// ThreatManager::RemoveMeFromThreatLists (ThreatManager.cpp:690-697):
	// C++ setDeathState(JUST_DIED) runs CombatStop (Unit.cpp:8901-8907),
	// which pairs ClearAllThreat (the dead unit's own table — cleared and
	// SMSG_THREAT_CLEAR-broadcast by every kill path before this call)
	// with dropping the dead unit as a victim from every other threat
	// table. Fires before the post-kill dispatch below, matching the C++
	// order (threat teardown inside setDeathState ahead of Kill's
	// post-processing, Unit.cpp:11320-11324). All six kill paths funnel
	// through here, so this one call covers melee, spell, DoT, and both
	// pet kill sites.
	if s.server != nil {
		s.server.removeThreatVictimFromAllLists(target.Map, target.InstanceID, target.GUID)
		// Unit::setDeathState (Unit.cpp:8901-8907) -> Unit::CombatStop
		// (Unit.cpp:5774) -> Creature::SetTarget (Creature.cpp:3221): death
		// clears the visual target — dead mobs must not keep a stare
		// target (Creature.cpp:2094). Ahead of the JustDied script hooks
		// like C++.
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			s.server.clearCreatureVisualTarget(motion)
		}
		// Unit::setDeathState (Unit.cpp:8909-8911): any combo points
		// pointed at the dying unit are lost — every holder's bank is
		// cleared (ClearComboPointHolders).
		s.server.clearComboPointHolders(target.GUID)
		// Unit::Kill's party-kill broadcast (Unit.cpp:11208-11232): when
		// the killing blow lands and the creature's player-damage
		// requirement is satisfied
		// (Creature::IsDamageEnoughForLootingAndReward, Creature.h:322),
		// the killer's group receives SMSG_PARTYKILLLOG {killer, victim};
		// a solo killer gets it direct, plus the no-group-looter
		// SMSG_LOOT_LIST for the creature via SendMessageToSet. The
		// cross-map loot-recipient fallback (Unit.cpp:11178-11199) is
		// unreachable in Go — taps are set by same-map hits — so the
		// killer session is always the C++ `player`. The victim-side
		// exploit arm (player-owned pet victim) never reaches this
		// funnel; only wild-creature kills land here. The group looter
		// election itself stays lazy at loot-open (loot.go).
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			if motion.PlayerDamageReq == 0 || motion.FlagsExtra&creatureFlagExtraNoPlayerDamageReq != 0 {
				killLog := protocol.NewBuffer(16)
				killLog.WriteU64(s.playerGUID)
				killLog.WriteU64(target.GUID)
				if s.groupID != 0 {
					s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_PARTYKILLLOG), killLog.Bytes())
				} else {
					_ = s.write(uint16(protocol.OpcodeSMSG_PARTYKILLLOG), killLog.Bytes(), true)
					noLooter := protocol.NewBuffer(10)
					noLooter.WriteU64(target.GUID)
					noLooter.WriteU8(0)
					noLooter.WriteU8(0)
					_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_LIST), noLooter.Bytes(), true)
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_LOOT_LIST), noLooter.Bytes(), s)
				}
			}
		}
	}
	creatureEntry := uint32((target.GUID >> 24) & 0xFFFFFF)
	guid := uint32(target.GUID & 0x00FFFFFF)
	now := time.Now()

	// Unit::Kill's dungeon instance-binding block (Unit.cpp:11404-11425):
	// an INSTANCE_BIND-flagged creature killed in a raid/heroic dungeon
	// permanently binds every player in the instance (and their group).
	s.maybePermBindInstance(ctx, target, creatureEntry)

	// Eluna CREATURE_EVENT_ON_DIED (4), with the event-23 On_Reset pre-fire
	// (Eluna::JustDied, CreatureHooks.cpp:143-152). The motion is already
	// at Health 0 with the lootable flag set by every kill path, matching
	// C++ firing after death registration (Unit.cpp:11391).
	if s.server != nil {
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3), both Unit::Kill arms,
			// ahead of ai->JustDied (event 4): the pet arm precedes the
			// branch arm in C++ (Unit.cpp:11324-11335 before 11385-11387)
			// but observes no flag-dependent state, so both fire here.
			victim := s.server.luaMotionCreature(motion)
			if killer != nil {
				if killer.OwnerGUID != 0 {
					// Pet attacker: the pet arm fires for the owner's pet —
					// the killer itself — on top of the branch arm, i.e.
					// C++'s double pet->AI()->KilledUnit(victim) dispatch.
					s.server.fireCreatureTargetDied(ctx, killer, victim)
				}
				// Branch arm: attacker->AI()->KilledUnit(victim), after the
				// lootable flag the caller already set (Unit.cpp:11385-11387).
				s.server.fireCreatureTargetDied(ctx, killer, victim)
			} else if pet := s.livePetMotion(); pet != nil {
				// Player attacker: only the pet arm fires (Unit::Kill pet
				// arm — player = attacker itself).
				s.server.fireCreatureTargetDied(ctx, pet, victim)
			}
			s.fireCreatureDied(ctx, motion)
			// Death clears the assistance state with the engagement: the
			// flag re-arms at the next engage (the Unit::AttackStop reset
			// twin, Unit.cpp:5783) and the pending delayed assist dies.
			motion.AlreadyCallAssistance = false
			motion.AssistAt = time.Time{}
			motion.AssistVictim = 0
			motion.Assistants = nil
			// BossAI::_JustDied (ScriptedCreature.cpp:512-518): native death
			// hook after the Eluna event-23/4 fire, matching the
			// Eluna-first/native-second order used at respawn (kill.go:865).
			if motion.BossAI != nil {
				motion.BossAI.OnDied(ctx, s.server, motion)
			}
			// CombatAI::JustDied (CombatAI.cpp:68-73): AICOND_DIE spells —
			// castable-while-dead per UnitAI::FillAISpellInfo
			// (SPELL_ATTR0_CASTABLE_WHILE_DEAD, UnitAI.cpp:196) — are cast
			// at the killer, triggered. The passive/infinite-duration
			// (AICOND_AGGRO) and scheduled (AICOND_COMBAT) arms have no
			// bridge: Go casts every template spell through the same
			// combat-tick rotation. Packet-only via castCreatureSpell (no
			// damage application on this path).
			killerGUID := s.playerGUID
			if killer != nil && killer.GUID != 0 {
				killerGUID = killer.GUID
			}
			s.server.fireCreatureDeathCast(ctx, motion, killerGUID)
			// GuardAI::JustDied (GuardAI.cpp:72-76) ->
			// Creature::SendZoneUnderAttackMessage (Creature.cpp:2706-2712):
			// a guard slain by a player broadcasts SMSG_ZONE_UNDER_ATTACK
			// with the guard's area id to the killer's opposite team.
			// Guards ride CREATURE_FLAG_EXTRA_GUARD (0x80); s is the
			// kill-attributed session (the C++ charmer/owner-player arm).
			if motion.FlagsExtra&creatureFlagExtraGuard != 0 && s.player != nil {
				if _, areaID := s.server.zoneAndAreaID(motion.Map, motion.X, motion.Y, motion.Z, 0); areaID != 0 {
					s.server.broadcastZoneUnderAttack(teamForRace(s.player.Race), areaID)
				}
			}
		}
	}

	if s.server != nil {
		standardGUID := creatureWorldGUID(guid, creatureEntry)
		// Unit::Kill (Unit.cpp:11244, 11376-11381): the kill-time fill
		// starts from a cleared loot — a stale pickpocket window's state
		// must not survive into the corpse window — and the
		// UNIT_DYNFLAG_LOOTABLE decision is re-evaluated from the fresh
		// fill, so motion.Looted resets for the new corpse. The loot
		// owner is (re)written below, after this clear.
		s.server.clearLootState(target.Map, target.InstanceID, target.GUID, standardGUID)
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			s.server.motionMu.Lock()
			motion.Looted = false
			s.server.motionMu.Unlock()
		}
	}

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
	// creatureType backs the KILL-proc critter gate below and the
	// KILL_CREATURE_TYPE achievement credit further down; one query serves
	// both.
	var creatureType uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&creatureType)
	}
	if s.server != nil {
		standardGUID := creatureWorldGUID(guid, creatureEntry)
		// Unit::Kill (Unit.cpp:11169-11204): the damage requirement
		// (IsDamageEnoughForLootingAndReward) gates the loot recipient —
		// a failed requirement clears it (SetLootRecipient(nullptr)) —
		// while the player-pet exploit arm gates only the reward block
		// (PARTY_KILL, loot generation, XP). The loot owner is the
		// first-hit tapper (DealDamage, Unit.cpp:872-876), not the
		// killer — the killer fallback covers tap-less kills (GM
		// commands) where C++ would leave the recipient empty.
		damageReqMet := true
		petVictim := false
		tapPlayer, tapGroup := s.playerGUID, s.groupID
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			if motion.FlagsExtra&creatureFlagExtraNoPlayerDamageReq == 0 && motion.PlayerDamageReq != 0 {
				damageReqMet = false
			}
			if motion.PetType != 0 && motion.OwnerGUID != 0 {
				petVictim = true
			}
			if motion.TapPlayerGUID != 0 {
				tapPlayer, tapGroup = motion.TapPlayerGUID, motion.TapGroupID
			}
		}
		if !damageReqMet {
			// Unit::Kill (Unit.cpp:11172-11175): a failed requirement
			// clears the loot recipient (SetLootRecipient(nullptr),
			// Creature.cpp:1330-1338) — the corpse sheds LOOTABLE|TAPPED.
			// The kill paths already broadcast the corpse flags before
			// this gate ran, so re-broadcast the cleared value.
			if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
				s.server.motionMu.Lock()
				motion.TapPlayerGUID, motion.TapGroupID = 0, 0
				motion.DynamicFlags &^= unitDynFlagTapped | unitDynFlagLootable
				flags := motion.DynamicFlags
				s.server.motionMu.Unlock()
				s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldDynamicFlags: flags})
			}
		}
		if damageReqMet {
			s.server.lootMu.Lock()
			if s.server.creatureLootOwners == nil {
				s.server.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
			}
			owner := lootOwnerState{PlayerGUID: tapPlayer, GroupID: tapGroup}
			s.server.creatureLootOwners[lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}] = owner
			s.server.creatureLootOwners[lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: standardGUID}] = owner
			s.server.lootMu.Unlock()
		} else {
			// Player::SendLoot (Player.cpp:8805-8810): with the recipient
			// cleared, every open answers LOOT_ERROR_DIDNT_KILL. Go's
			// creatureLootAllowed defaults an absent owner record to
			// allowed, so the denial is recorded explicitly — a zero
			// owner matches no player GUID and hides the sparkle via
			// creatureLootSparkleVisible, mirroring the missing
			// UNIT_DYNFLAG_LOOTABLE. Skinning still works: openSkinningLoot
			// overwrites the owner with the skinner (SetLootRecipient, Player.cpp:8841).
			s.server.lootMu.Lock()
			if s.server.creatureLootOwners == nil {
				s.server.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
			}
			denied := lootOwnerState{}
			s.server.creatureLootOwners[lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}] = denied
			s.server.creatureLootOwners[lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: standardGUID}] = denied
			s.server.lootMu.Unlock()
		}
		if damageReqMet && !petVictim {
			s.rewardCreatureKillXP(ctx, target, creatureEntry, mobLevel)
		}

		// Unit::Kill (Unit.cpp:11257-11279): after the reward, the KILL proc
		// leg fires on the killer's auras, then the per-kill GET_KILLING_BLOWS
		// criteria. The C++ pet/totem arm procs the OWNER with (KILL, NONE);
		// pet kills already run on the owner session s here, so one KILL
		// event covers it — the pet's own aura leg is unmodeled (no pet
		// aura/proc loop). The (KILL, KILLED) general arm is gated on the
		// victim not being a critter (creatureTypeCritter = 8, SharedDefines.h:2668). The victim-side KILLED and DEATH legs have
		// no bearer: creature auras live in a server-side map with no
		// generic proc loop (only the damage-aura break bridge).
		if creatureType != creatureTypeCritter {
			// Player::isHonorOrXPTarget (Player.cpp:24064-24083): the
			// creature must be above the killer's gray level and not a
			// critter (excluded by the outer gate) or totem.
			honorTarget := s.player != nil && mobLevel > grayLevel(uint32(s.player.Level)) && !s.server.isTotemGUID(target.GUID)
			s.procKillAuraTriggers(ctx, target.GUID, s.playerGUID, procFlagKill, honorTarget)
		}
		// Unit::Kill (Unit.cpp:11283-11290): the general-arm KILL leg on
		// the killer creature's own auras (pet and wild-creature killers —
		// the player-attacker KILL leg fired above), plus the victim-side
		// KILLED and DEATH legs on the victim creature's own auras. C++
		// order: owner arm (above), general arm (KILL on attacker, KILLED
		// on victim), DEATH on victim, then the killing-blow criteria. The
		// general arm is gated on the victim not being a critter; the DEATH
		// leg is unconditional. The victim motion still holds its auras
		// here (clearCreatureAuras runs below); the two spells.go kill paths
		// that cleared auras pre-kill simply evaluate empty sets.
		if killer != nil && creatureType != creatureTypeCritter {
			s.server.fireCreatureAuraProcLoop(ctx, killer, target.GUID, procEventInfo{
				typeMask:       procFlagKill,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      killer.GUID,
			})
		}
		if victim := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); victim != nil {
			killerGUID := s.playerGUID
			if killer != nil {
				killerGUID = killer.GUID
			}
			if creatureType != creatureTypeCritter {
				s.server.fireCreatureAuraProcLoop(ctx, victim, killerGUID, procEventInfo{
					typeMask:       procFlagKilled,
					spellTypeMask:  procSpellTypeMaskAll,
					spellPhaseMask: procSpellPhaseNone,
					hitMask:        procHitNone,
					actorGUID:      victim.GUID,
				})
			}
			s.server.fireCreatureAuraProcLoop(ctx, victim, target.GUID, procEventInfo{
				typeMask:       procFlagDeath,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      victim.GUID,
			})
		}
		s.creditKillingBlowCriteria(nil)

		// Boss_Announcer::OnCreatureKill (Custom/cs_worldboss_kill_announce.cpp:20-32):
		// fires from Unit::Kill's ScriptMgr hook block (Unit.cpp:11468-11482),
		// after the achievement-credit block, only when the attacker is a
		// player (killer == nil here — a non-nil killer motion means a pet or
		// creature landed the blow, where C++ attacker->ToPlayer() is null
		// and the hook never fires) and the victim is a world boss
		// (Creature::isWorldBoss, Creature.cpp:2353-2359: type_flags &
		// CREATURE_TYPE_FLAG_BOSS_MOB, 0x4, SharedDefines.h:2731; the IsPet()
		// exclusion is vacuous — Go kill targets are creature motions only).
		// Broadcasts via World::SendServerMessage(SERVER_MSG_STRING, msg, 0)
		// (World.cpp:3074), bridged by broadcastServerMessageAll; the killer
		// link mirrors ChatHandler::playerLink (Chat.h:119). The companion
		// OnUpdateZone arm (area 420 run-speed set) is commented-out/dev-debug
		// code in C++ and is deliberately not bridged.
		if killer == nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var bossName string
			var typeFlags uint32
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(name, ''), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&bossName, &typeFlags); err == nil && typeFlags&0x4 != 0 {
				killerLink := "|cffffffff|Hplayer:" + s.player.Name + "|h[" + s.player.Name + "]|h|r"
				s.server.broadcastServerMessageAll(serverMessageString, "|CFF64FF64World Boss |CFFFFFFFF["+bossName+"]|r|CFF64FF64 has been slain by|r "+killerLink+"|CFF64FF64.")
			}
		}

		if target.InstanceID == 0 {
			if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				_, _ = s.server.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = 0 WHERE guid = ?", guid)
			}
			// Respawn at full health: target.Health is stale (captured before
			// the killing blow) and 0 at death; C++ respawn restores max via
			// SetSpawnHealth.
			s.server.scheduleCreatureRespawn(ctx, guid, uint32(math.Max(float64(target.MaxHealth), 1)), now)
		} else {
			s.server.scheduleInstanceCreatureRespawn(ctx, target, uint32(math.Max(float64(target.MaxHealth), 1)), now)
		}
		corpseDynFlags := uint32(1) // UNIT_DYNFLAG_LOOTABLE
		if s.server != nil {
			if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil && motion.PetID != 0 && motion.OwnerGUID != 0 && motion.PetType == petTypeHunter {
				// Pet::setDeathState CORPSE (Pet.cpp:568-574): hunter-pet
				// corpses are non-lootable — SetUInt32Value(UNIT_DYNAMIC_FLAGS,
				// UNIT_DYNFLAG_NONE) — and non-skinnable (the skinnable clear
				// rides the pet-death arm block below).
				s.server.motionMu.Lock()
				motion.DynamicFlags = 0
				s.server.motionMu.Unlock()
				corpseDynFlags = 0
			}
		}
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: corpseDynFlags,
		})
	}

	// Pet::setDeathState (Pet.cpp:566-588): the CORPSE arm for hunter pets
	// (PetType 1 = HUNTER_PET, petTypeHunter): the corpse is non-lootable
	// (dynamic flags cleared above) and non-skinnable, and the pet loses
	// one happiness level (ModifyPower(POWER_HAPPINESS, -HAPPINESS_LEVEL_SIZE),
	// unsigned-clamped at zero) unless it died in a battleground or arena.
	// Powers[4] is POWER_HAPPINESS (aura_power.go) and motion.Happiness is
	// the persisted source (pet_save.go), so both are updated.
	if s.server != nil {
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil && motion.PetID != 0 && motion.OwnerGUID != 0 && motion.PetType == petTypeHunter {
			bgOrArena := false
			if s.server.Data != nil {
				if mapInfo, found, err := s.server.Data.Map(target.Map); err == nil && found {
					bgOrArena = mapInfo.IsBattleground() || mapInfo.IsBattleArena()
				}
			}
			s.server.motionMu.Lock()
			motion.UnitFlags &^= unitFlagSkinnable
			if !bgOrArena {
				if motion.Powers[4] > petHappinessLevelSize {
					motion.Powers[4] -= petHappinessLevelSize
				} else {
					motion.Powers[4] = 0
				}
				motion.Happiness = motion.Powers[4]
			}
			happiness, unitFlags := motion.Powers[4], motion.UnitFlags
			s.server.motionMu.Unlock()
			s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
				unitFieldPower1 + 4: happiness,
				unitFieldFlags:      unitFlags,
			})
		}
	}

	// Quest kill credit (KillRewarder::_RewardKillCredit, KillRewarder.cpp:183-191):
	// RequiredNpcOrGo entries plus KillCredit templates, shared with the
	// group at reward distance.
	s.grantCreatureKillQuestCredit(ctx, creatureEntry, creatureType, target)
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

	// Clear any active auras/DoTs ticking on this creature
	if s.server != nil {
		// AuraEffect::HandleChannelDeathItem remove-by-death leg
		// (SpellAuraEffects.cpp:4690): 86 auras on the dying creature
		// (Drain Soul) grant their items to the aura casters; runs before
		// the strip, mirroring RemoveAllAurasOnDeath's remove-mode.
		key := creatureAuraKeyForTarget(target)
		s.server.auraMu.Lock()
		var deathAuras []*activeAura
		for _, aura := range s.server.activeCreatureAuras[key] {
			deathAuras = append(deathAuras, aura)
		}
		s.server.auraMu.Unlock()
		var tapPlayerGUID, tapGroupID uint64
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			tapPlayerGUID, tapGroupID = motion.TapPlayerGUID, motion.TapGroupID
		}
		s.server.grantChannelDeathItems(ctx, deathAuras, mobLevel, true, tapPlayerGUID, tapGroupID)
		// RemoveAllAurasOnDeath (Unit.cpp): death-persistent auras
		// (SPELL_ATTR3_DEATH_PERSISTENT) survive the strip.
		s.server.clearCreatureAurasOnDeath(key)
	}

	// Unit::Kill battleground tail (Unit.cpp:11446): player && InBattleground.
	// bgData.InstanceID is the Go mirror (Player.h:1906: bgInstanceID != 0);
	// map presence alone is not the gate (see handleNoFunnelBattlegroundKill).
	// Alterac Valley (Map 30) creature kills (Generals, Captains, Mine bosses)
	if s.server != nil && s.bgData.InstanceID != 0 && (s.player.Map == 30 || s.hasActiveBGMap(30)) {
		s.server.handleAVCreatureKilled(s, creatureEntry)
	}

	// Strand of the Ancients (Map 607) creature kills (Demolishers)
	if s.server != nil && s.bgData.InstanceID != 0 && (s.player.Map == 607 || s.hasActiveBGMap(607)) {
		s.server.handleSACreatureKilled(s, creatureEntry)
	}

	// Isle of Conquest (Map 628) creature kills (Bosses & Vehicles)
	if s.server != nil && s.bgData.InstanceID != 0 && (s.player.Map == 628 || s.hasActiveBGMap(628)) {
		s.server.handleICCreatureKilled(s, creatureEntry)
	}

	// TempSummon corpse/dead legs at death time (TemporarySummon.cpp:66-170):
	// the in-range passes only observe summons near players, so an
	// out-of-range corpse previously started its type-5 timer late (first
	// observation) and a type-1/4 corpse lingered to the prune. This
	// funnel runs wherever the kill happened, so run the dead legs
	// here: types 1/4 unsummon at once, type 5 starts its timer at
	// death; other types leave the corpse to the prune, as
	// checkDeadTempSummon holds. Kills outside this funnel
	// (creature-killed summons) keep the first-observation stamp as
	// fallback. Placed last so every kill broadcast, loot write, and
	// reward above still sees the corpse.
	if s.server != nil {
		if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
			s.server.checkDeadTempSummon(ctx, motion, now)
		}
	}
}

// fireCreatureDeathCast is the CombatAI::JustDied AICOND_DIE leg
// (CombatAI.cpp:68-73; the condition itself is UnitAI::FillAISpellInfo,
// UnitAI.cpp:196: a template spell is AICOND_DIE exactly when it carries
// SPELL_ATTR0_CASTABLE_WHILE_DEAD). The dead creature casts those
// template spells at the killer, triggered, packet-only — no damage
// application, as on the funnel path. The funnel path and every
// no-funnel death chain share it so the leg cannot drift between
// paths again.
func (s *Server) fireCreatureDeathCast(ctx context.Context, dead *creatureMotion, killerGUID uint64) {
	if s == nil || dead == nil {
		return
	}
	if len(dead.Spells) == 0 && s.WorldStore != nil && s.WorldStore.DB != nil {
		dead.Spells = s.loadCreatureSpells(ctx, dead.Entry)
	}
	if len(dead.Spells) > 0 && s.Data != nil {
		for _, spellID := range dead.Spells {
			if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found && spellInfo.Attributes&spellAttr0CastableWhileDead != 0 {
				s.castCreatureSpellTriggered(ctx, dead, spellID, killerGUID)
			}
		}
	}
}

// fireGuardDeathBroadcast is the GuardAI::JustDied leg
// (GuardAI.cpp:72-76) for the no-funnel death chains: a guard slain
// by a player broadcasts SMSG_ZONE_UNDER_ATTACK with the guard's area
// id to the killer's opposite team (Creature::SendZoneUnderAttackMessage,
// Creature.cpp:2706-2712). The killer resolves through
// GetCharmerOrOwnerPlayerOrPlayerItself (Object.cpp:2226-2232): a
// creature killer counts only via its charmer/owner player, and only
// when that player is loaded — an online session in Go. A
// wild-creature killer, or a player-owned killer whose owner is
// offline, resolves to nil and broadcasts nothing; the funnel path
// keeps its own session-player gate (a player killer has no motion).
func (s *Server) fireGuardDeathBroadcast(dead, killer *creatureMotion) {
	if s == nil || dead == nil || killer == nil || killer.OwnerGUID == 0 {
		return
	}
	if dead.FlagsExtra&creatureFlagExtraGuard == 0 {
		return
	}
	owner := s.findSessionByGUID(killer.OwnerGUID)
	if owner == nil || owner.player == nil {
		return
	}
	if _, areaID := s.zoneAndAreaID(dead.Map, dead.X, dead.Y, dead.Z, 0); areaID != 0 {
		s.broadcastZoneUnderAttack(teamForRace(owner.player.Race), areaID)
	}
}

// recordCreatureKillLoot is the Unit::Kill loot leg (Unit.cpp:11169-11267)
// for the no-funnel death chains: creature loot is filled at kill time,
// never at loot-open (Player.cpp:8805, "for creature, loot is filled
// when creature is killed"), and only inside the reward block - an
// online player must own the kill, either as the corpse's tapper or as
// the killer's charmer/owner player (Creature::GetLootRecipient uses
// FindConnectedPlayer, Creature.cpp:1314-1319, so an offline tapper or
// owner counts for nothing). Go fills lazily at open, so the kill-time
// legs are the stale-loot clear plus the owner/denial record the funnel
// writes: a cleared pickpocket window must not survive into the corpse
// window, a tapper/owner-owned corpse fills for its owner at open, and
// a player-less corpse records an explicit denial (the funnel's
// damage-requirement denial arm, including the TAPPED|LOOTABLE flag
// clear and re-broadcast) instead of defaulting to allowed. The damage
// requirement is the funnel's own gate (NO_PLAYER_DAMAGE_REQ flag or a
// zero requirement). The group-member fallback for an off-map recipient
// (Unit.cpp:11182-11195) is not modeled, as in the funnel. XP,
// party-kill and group-looter election stay funnel-only.
func (s *Server) recordCreatureKillLoot(victim, killer *creatureMotion) {
	if s == nil || victim == nil {
		return
	}
	targetGUID := victim.GUID
	guid := uint32(targetGUID & 0x00FFFFFF)
	entry := uint32((targetGUID >> 24) & 0xFFFFFF)
	standardGUID := creatureWorldGUID(guid, entry)
	s.clearLootState(victim.Map, victim.InstanceID, targetGUID, standardGUID)
	s.motionMu.Lock()
	victim.Looted = false
	s.motionMu.Unlock()
	damageReqMet := true
	if victim.FlagsExtra&creatureFlagExtraNoPlayerDamageReq == 0 && victim.PlayerDamageReq != 0 {
		damageReqMet = false
	}
	var ownerGUID, ownerGroup uint64
	if victim.TapPlayerGUID != 0 {
		if tap := s.findSessionByGUID(victim.TapPlayerGUID); tap != nil && tap.player != nil {
			ownerGUID, ownerGroup = victim.TapPlayerGUID, victim.TapGroupID
		}
	}
	if ownerGUID == 0 && killer != nil && killer.OwnerGUID != 0 {
		if own := s.findSessionByGUID(killer.OwnerGUID); own != nil && own.player != nil {
			ownerGUID, ownerGroup = own.playerGUID, own.groupID
		}
	}
	if !damageReqMet || ownerGUID == 0 {
		s.motionMu.Lock()
		victim.TapPlayerGUID, victim.TapGroupID = 0, 0
		victim.DynamicFlags &^= unitDynFlagTapped | unitDynFlagLootable
		flags := victim.DynamicFlags
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(victim.Map, victim.InstanceID, targetGUID, map[int]uint32{unitFieldDynamicFlags: flags})
		s.lootMu.Lock()
		if s.creatureLootOwners == nil {
			s.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
		}
		denied := lootOwnerState{}
		s.creatureLootOwners[lootObjectKey{MapID: victim.Map, InstanceID: victim.InstanceID, GUID: targetGUID}] = denied
		s.creatureLootOwners[lootObjectKey{MapID: victim.Map, InstanceID: victim.InstanceID, GUID: standardGUID}] = denied
		s.lootMu.Unlock()
		return
	}
	s.lootMu.Lock()
	if s.creatureLootOwners == nil {
		s.creatureLootOwners = make(map[lootObjectKey]lootOwnerState)
	}
	owner := lootOwnerState{PlayerGUID: ownerGUID, GroupID: ownerGroup}
	s.creatureLootOwners[lootObjectKey{MapID: victim.Map, InstanceID: victim.InstanceID, GUID: targetGUID}] = owner
	s.creatureLootOwners[lootObjectKey{MapID: victim.Map, InstanceID: victim.InstanceID, GUID: standardGUID}] = owner
	s.lootMu.Unlock()
}

// grantCreatureKillReward is the Unit::Kill reward leg (Unit.cpp:11199-11267)
// for the no-funnel death chains. The block is NOT player-killer-gated:
// `player` starts as the killer's charmer/owner player
// (attacker->GetCharmerOrOwnerPlayerOrPlayerItself, Object.cpp:2226-2232)
// and is overwritten by the corpse's loot recipient when one exists, so a
// wild creature's killing blow still rewards an online tapper, and a
// player-owned killer still rewards its online owner. A wild killer with
// no online tapper or owner resolves to nil and rewards nothing. The
// gates are the funnel's: the damage requirement
// (IsDamageEnoughForLootingAndReward) and the player-owned-pet-victim
// exploit arm (Unit.cpp:11199-11204) — the latter also suppresses the
// PARTYKILLLOG, which the funnel never needs to gate because pet victims
// never reach it. The group-looter election stays lazy at loot-open
// (loot.go), as in the funnel; the off-map recipient group-member
// fallback (Unit.cpp:11182-11195) is not modeled, as in the loot leg.
// Quest kill credit rides the same RewardPlayerAndGroupAtKill call
// (KillRewarder::_RewardKillCredit, KillRewarder.cpp:186-195) via the
// funnel's grantCreatureKillQuestCredit twin, after the XP arm; the
// instance-bind leg is NOT folded in here — Unit.cpp:11404-11425 runs
// outside the reward gate (no damage/pet-victim check), so it gets its
// own helper and call site below.
// Placed after recordCreatureKillLoot at each site: the loot leg already
// resolved the same owner, and on the denial path the damage gate below
// returns before the cleared tap can fall back to the killer's owner.
func (s *Server) grantCreatureKillReward(ctx context.Context, victim, killer *creatureMotion) {
	if s == nil || victim == nil {
		return
	}
	if victim.FlagsExtra&creatureFlagExtraNoPlayerDamageReq == 0 && victim.PlayerDamageReq != 0 {
		return
	}
	if victim.PetType != 0 && victim.OwnerGUID != 0 {
		return
	}
	var player *session
	if victim.TapPlayerGUID != 0 {
		player = s.findSessionByGUID(victim.TapPlayerGUID)
	}
	if player == nil && killer != nil && killer.OwnerGUID != 0 {
		player = s.findSessionByGUID(killer.OwnerGUID)
	}
	if player == nil || player.player == nil {
		return
	}
	killLog := protocol.NewBuffer(16)
	killLog.WriteU64(player.playerGUID)
	killLog.WriteU64(victim.GUID)
	if player.groupID != 0 {
		s.broadcastToGroup(player.groupID, uint16(protocol.OpcodeSMSG_PARTYKILLLOG), killLog.Bytes())
	} else {
		_ = player.write(uint16(protocol.OpcodeSMSG_PARTYKILLLOG), killLog.Bytes(), true)
		noLooter := protocol.NewBuffer(10)
		noLooter.WriteU64(victim.GUID)
		noLooter.WriteU8(0)
		noLooter.WriteU8(0)
		_ = player.write(uint16(protocol.OpcodeSMSG_LOOT_LIST), noLooter.Bytes(), true)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_LOOT_LIST), noLooter.Bytes(), player)
	}
	creatureEntry := uint32((victim.GUID >> 24) & 0xFFFFFF)
	mobLevel := uint32(victim.Level)
	if mobLevel == 0 && s.WorldStore != nil && s.WorldStore.DB != nil {
		var fallbackLevel int64
		_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(maxlevel, 0), minlevel, 1) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&fallbackLevel)
		if fallbackLevel > 0 {
			mobLevel = uint32(fallbackLevel)
		}
	}
	if mobLevel == 0 {
		mobLevel = 1
	}
	player.rewardCreatureKillXP(ctx, combatTarget{GUID: victim.GUID, Map: victim.Map, InstanceID: victim.InstanceID, X: victim.X, Y: victim.Y, Z: victim.Z, CombatReach: victim.CombatReach}, creatureEntry, mobLevel)
	var creatureType uint32
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&creatureType)
	}
	player.grantCreatureKillQuestCredit(ctx, creatureEntry, creatureType, combatTarget{GUID: victim.GUID, Map: victim.Map, InstanceID: victim.InstanceID, X: victim.X, Y: victim.Y, Z: victim.Z, CombatReach: victim.CombatReach})
}

// bindInstanceOnNoFunnelKill is the Unit::Kill dungeon instance-binding
// leg (Unit.cpp:11404-11425) for the no-funnel death chains, kept separate
// from grantCreatureKillReward because C++ runs it outside the reward
// block: no damage-requirement or pet-victim gate, only a
// player-attributed kill of an INSTANCE_BIND-flagged creature in a raid
// or heroic dungeon. Attribution is the ATTACKER's charmer/owner player
// (attacker->GetCharmerOrOwnerPlayerOrPlayerItself, or attacker == victim
// for a suicide), NOT the reward player — a wild killer with an online
// tapper still binds nothing, and an owner-gone killer binds nothing
// (C++'s offline-owner bind is a @todo there, unimplemented here too).
// Callers place it after grantCreatureKillReward: C++ runs the bind block
// after JustDied (Unit.cpp:11391 vs :11404) and the reward block precedes
// it. The non-raid/non-heroic arm only extends the save's reset time,
// which Go's read-only resettime model does not schedule (as in
// maybePermBindInstance, the funnel twin whose dungeon/flag/difficulty
// checks this mirrors).
func (s *Server) bindInstanceOnNoFunnelKill(ctx context.Context, victim, killer *creatureMotion) {
	if s == nil || victim == nil || killer == nil || victim.InstanceID == 0 {
		return
	}
	if killer.GUID != victim.GUID {
		attackerPlayer := s.findSessionByGUID(killer.OwnerGUID)
		if attackerPlayer == nil {
			attackerPlayer = s.findSessionByGUID(killer.CharmerGUID)
		}
		if attackerPlayer == nil {
			return
		}
	}
	if s.Data == nil {
		return
	}
	mapEntry, found, mapErr := s.Data.Map(victim.Map)
	if mapErr != nil || !found || !mapEntry.IsDungeon() {
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var difficulty uint32
	if err := s.CharactersStore.DB.QueryRowContext(ctx, "SELECT difficulty FROM instance WHERE id = ?", victim.InstanceID).Scan(&difficulty); err != nil {
		return
	}
	creatureEntry := uint32((victim.GUID >> 24) & 0xFFFFFF)
	var flagsExtra uint32
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT flags_extra FROM creature_template WHERE entry = ?", creatureEntry).Scan(&flagsExtra); err != nil {
		return
	}
	if !(mapEntry.IsRaid() || difficulty == 1) {
		return
	}
	if flagsExtra&0x1 == 0 {
		return
	}
	s.permBindAllPlayers(ctx, victim.Map, victim.InstanceID, difficulty)
}

// creditNoFunnelKillingBlow is the Unit::Kill killing-blow achievement
// leg (Unit.cpp:11293-11297) for the no-funnel death chains:
// UpdateAchievementCriteria(ACHIEVEMENT_CRITERIA_TYPE_GET_KILLING_BLOWS)
// fires on every kill for the attacker's charmer/owner player
// (attacker->GetCharmerOrOwnerPlayerOrPlayerItself, Object.cpp:2226-2232),
// for creature victims as well as player victims. Unlike the reward leg
// the victim's tapper plays no part, and unlike the bind leg there is no
// attacker == victim suicide arm: a wild killer credits nobody, and an
// owner-gone killer credits nobody (C++ ObjectAccessor::GetPlayer returns
// nil offline). Resolution is C++-exact (Unit.h:1170: IsCharmed ?
// GetCharmerGUID() : GetOwnerGUID()); the victim is a creature motion,
// so the session method takes nil, the funnel's convention (kill.go:975).
// C++ runs the leg before setDeathState so criteria_data aura rules can
// still read the victim's auras; Go's credit reads no victim state, and
// the funnel itself credits after its death-state clears, so the tail
// placement after bindInstanceOnNoFunnelKill is behavior-identical.
func (s *Server) creditNoFunnelKillingBlow(killer *creatureMotion) {
	if s == nil || killer == nil {
		return
	}
	guid := killer.OwnerGUID
	if killer.Charmed {
		guid = killer.CharmerGUID
	}
	player := s.findSessionByGUID(guid)
	if player == nil || player.player == nil {
		return
	}
	player.creditKillingBlowCriteria(nil)
}

// handleNoFunnelBattlegroundKill is the Unit::Kill battleground tail
// (Unit.cpp:11451+) for the no-funnel death chains: player &&
// InBattleground -> bg->HandleKillUnit(creature victim, player). The
// funnel already runs this tail via its map dispatch (AV 30, SA 607,
// IC 628); no no-funnel chain did. Attribution is the same Kill-level
// player as creditNoFunnelKillingBlow (attacker->
// GetCharmerOrOwnerPlayerOrPlayerItself, Unit.h:1170: IsCharmed ?
// CharmerGUID : OwnerGUID) - a wild killer resolves to no session and
// an owner-gone killer likewise, C++-correct. Only AV, IC, and SA
// override HandleKillUnit in C++ (Battleground.h:428 base is empty;
// WSG, AB, and EOTS have no override), so the dispatch mirrors the
// funnel's three maps and the per-BG handlers re-gate on the session
// map. Both dispatch sites gate on bgData.InstanceID != 0, the direct
// InBattleground mirror (Player.h:1906: bgInstanceID != 0). The queue
// check (sessionInBattlegroundType, Status InProgress) is NOT used:
// C++'s GetBattleground() nil arm reduces to GetBattlegroundId() == 0
// (Player.cpp: GetBattleground), i.e. the same instance id, and queue
// status is player queue bookkeeping, not the BG instance's status
// (AV/IC gate on that separately via Winner; SA has no status gate in
// C++). Map presence alone would fire for a player standing on a BG
// map with no BG instance (GM teleport / post-leave), where C++ fires
// nothing; the manager-lookup-miss half of GetBattleground() has no Go
// equivalent (per-map state is getOrCreate), recorded as residual.
func (s *Server) handleNoFunnelBattlegroundKill(victim, killer *creatureMotion) {
	if s == nil || victim == nil || killer == nil {
		return
	}
	guid := killer.OwnerGUID
	if killer.Charmed {
		guid = killer.CharmerGUID
	}
	player := s.findSessionByGUID(guid)
	if player == nil || player.player == nil {
		return
	}
	if player.bgData.InstanceID == 0 {
		return
	}
	// Independent ifs, not a switch: C++ dispatches on the killer's
	// battleground, and a slot holder's victim entry is BG-specific,
	// so each handler's own entry switch makes cross-firing inert.
	if player.player.Map == AVMapID || player.hasActiveBGMap(AVMapID) {
		s.handleAVCreatureKilled(player, victim.Entry)
	}
	if player.player.Map == SAMapID || player.hasActiveBGMap(SAMapID) {
		s.handleSACreatureKilled(player, victim.Entry)
	}
	if player.player.Map == ICMapID || player.hasActiveBGMap(ICMapID) {
		s.handleICCreatureKilled(player, victim.Entry)
	}
}

// clearCreatureDeathState is the Unit::setDeathState (Unit.cpp:8901-8911)
// leg for the no-funnel death chains: setDeathState runs CombatStop,
// which clears the visual target (AttackStop -> Creature::SetTarget,
// Unit.cpp:5774/Creature.cpp:2094) and the combo-point holders
// (ClearComboPointHolders), and the AttackStop reset (Unit.cpp:5783)
// drops the once-per-engagement assistance latch with its pending
// delayed assist — the funnel's kill.go:699-704 and :776-779 twins.
// C++ runs setDeathState ahead of KilledUnit/JustDied (Unit.cpp:11320
// vs :11324-11387), so callers place this after the threat-victim
// drop and before the event-3/4 hooks, matching the funnel's
// pre-hook visual/combo position.
func (s *Server) clearCreatureDeathState(dead *creatureMotion) {
	if s == nil || dead == nil {
		return
	}
	s.clearCreatureVisualTarget(dead)
	s.clearComboPointHolders(dead.GUID)
	dead.AlreadyCallAssistance = false
	dead.AssistAt = time.Time{}
	dead.AssistVictim = 0
	dead.Assistants = nil
}

// firePVPKillHook fires Eluna PLAYER_EVENT_ON_KILL_PLAYER (6):
// Eluna::OnPVPKill (PlayerHooks.cpp:155-161), (event, killer, killed), void
// (CallAllFunctions). Unit::Kill tail (Unit.cpp:11466-11480): attacker and
// victim are both players. No victim != attacker guard in C++.
func (s *Server) firePVPKillHook(ctx context.Context, killer, victim *session) {
	if s == nil || killer == nil || victim == nil {
		return
	}
	killer.triggerPlayerEvent(ctx, scripting.PlayerEventKillPlayer, killer.luaPlayer(), victim.luaPlayer())
}

// fireCreatureKillHook fires Eluna PLAYER_EVENT_ON_KILL_CREATURE (7):
// Eluna::OnCreatureKill (PlayerHooks.cpp:163-169), (event, killer, killed),
// void (CallAllFunctions). Unit::Kill tail: attacker is a player, victim a
// creature. Pet/wild-creature attackers fire nothing in C++
// (attacker->ToPlayer() is nil), so only direct-player-attacker sites call this.
func (s *Server) fireCreatureKillHook(ctx context.Context, killer *session, victim *creatureMotion) {
	if s == nil || killer == nil || victim == nil {
		return
	}
	killer.triggerPlayerEvent(ctx, scripting.PlayerEventKillCreature, killer.luaPlayer(), s.luaMotionCreature(victim))
}

// fireKilledByCreatureHook fires Eluna PLAYER_EVENT_ON_KILLED_BY_CREATURE (8):
// Eluna::OnPlayerKilledByCreature (PlayerHooks.cpp:171-177),
// (event, killer, killed), void (CallAllFunctions). Unit::Kill tail: attacker
// is a creature, victim a player. For pet kills C++ pushes the pet itself
// (attacker->ToCreature()), not the owner.
func (s *Server) fireKilledByCreatureHook(ctx context.Context, killer *creatureMotion, victim *session) {
	if s == nil || killer == nil || victim == nil {
		return
	}
	victim.triggerPlayerEvent(ctx, scripting.PlayerEventKilledByCreature, s.luaMotionCreature(killer), victim.luaPlayer())
}

// fireGiveXPHook fires Eluna PLAYER_EVENT_ON_GIVE_XP (12): Eluna::OnGiveXP
// (PlayerHooks.cpp:228-252), (event, player, amount, victim), where each
// handler's numeric return rewrites amount for the next handler
// (ReplaceArgument). Player::GiveXP (Player.cpp:2658) fires it after the
// dead/no-XP-gain/loot-recipient guards, before the max-level early return
// and before the AutoBalance/xp_boost PlayerScript hooks, so the Go call
// sits at the top of grantXPWithVictimGroup. The victim is the kill target
// creature object, or nil for quest/exploration XP (C++ pushes a nil Unit).
func (s *session) fireGiveXPHook(ctx context.Context, amount uint32, victimGUID uint64) uint32 {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return amount
	}
	var victim any
	if victimGUID != 0 && s.player != nil {
		if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, victimGUID); motion != nil {
			victim = s.server.luaMotionCreature(motion)
		} else {
			victim = s.server.findPlayer(victimGUID)
		}
	}
	args := []any{scripting.PlayerEventGiveXP, s.luaPlayer(), amount, victim}
	_, err := s.server.Features.Scripts.TriggerPlayerEventUpdated(ctx, scripting.PlayerEventGiveXP, args, func(returns []any) {
		if len(returns) == 0 {
			return
		}
		if n, ok := returns[0].(float64); ok {
			if n < 0 {
				amount = 0
			} else {
				amount = uint32(int64(n))
			}
			args[2] = amount
		}
	})
	if err != nil {
		s.debug("lua give-xp hook failed", "account", s.accountName, "error", err)
	}
	return amount
}

// fireLevelChangedHook fires Eluna PLAYER_EVENT_ON_LEVEL_CHANGE (13):
// Eluna::OnLevelChanged (PlayerHooks.cpp:179-185), (event, player, oldLevel),
// void (CallAllFunctions). Player::GiveLevel (Player.cpp:2787) fires it once
// per level gained, after the level-up processing; the GM .character level
// path goes through GiveLevel for online targets (cs_character.cpp), so it
// fires from the Go level-up loop and handleCharacterLevel alike.
func (s *session) fireLevelChangedHook(ctx context.Context, oldLevel uint8) {
	if s == nil {
		return
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventLevelChange, s.luaPlayer(), oldLevel)
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
	// Eluna::OnGiveXP (PlayerHooks.cpp:228): fires before the PlayerScript
	// hooks below; a numeric handler return rewrites the amount.
	amount = s.fireGiveXPHook(ctx, amount, victimGUID)
	if amount == 0 {
		return
	}
	// AutoBalance_PlayerScript::OnGiveXP (AutoBalance.cpp): dungeon kill XP
	// scales by current/max instance players. The C++ arm is not gated on
	// AutoBalance.enable, only on victim && DungeonScaleDownXP.
	if victimGUID != 0 && s.server.Config.AutoBalance.DungeonScaleDownXP {
		amount = s.server.autoBalanceScaleDungeonXP(ctx, s, amount)
		if amount == 0 {
			return
		}
	}
	// xp_boost_PlayerScript::OnGiveXP (boosted_xp.cpp:40-46): when the
	// XP.Boost.Daymask bit for today's weekday is set (1 = Sunday, matching
	// tm_wday), the XP amount is multiplied by XP.Boost.Rate with the C++
	// uint32 *= float truncation. Fires after the AutoBalance hook: the
	// CMake script-module glob orders Custom before World, so the Custom
	// module's OnGiveXP runs first in FOREACH_SCRIPT order.
	if daymask := s.server.Config.XPBoostDaymask; daymask != 0 && xpBoostActiveToday(daymask) {
		amount = uint32(float64(amount) * s.server.Config.XPBoostRate)
		if amount == 0 {
			return
		}
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
	// Player::GiveXP (Player.cpp:2665-2673): a recruit-a-friend link triples
	// the grant (bonus_xp = 2 * xp, kill and non-kill XP alike); RaF does
	// NOT stack with rested experience — the rested arm (which also drains
	// the rest bonus) only runs without the link.
	recruitAFriend := s.getsRecruitAFriendBonus(true)
	bonusXP := uint32(0)
	if victimGUID != 0 {
		if recruitAFriend {
			bonusXP = 2 * amount
			s.setRestBonus(s.player, s.player.RestBonus)
		} else {
			if s.player.RestBonus > 0 {
				bonusXP = uint32(s.player.RestBonus)
				if bonusXP > amount {
					bonusXP = amount
				}
			}
			s.setRestBonus(s.player, s.player.RestBonus-float32(bonusXP))
		}
	} else if recruitAFriend {
		bonusXP = 2 * amount
	}
	totalXP := amount + bonusXP

	// SMSG_LOG_XPGAIN (0x1D0): victimGUID (8), totalXP (4), type (1), [baseXP (4), groupRate (4)], rafBonus (1)
	xpLog := protocol.NewBuffer(22)
	xpLog.WriteU64(victimGUID)
	xpLog.WriteU32(totalXP)
	if victimGUID != 0 {
		xpLog.WriteU8(0)       // 0 = kill XP
		xpLog.WriteU32(amount) // base XP
		xpLog.WriteF32(1.0)    // group rate
	} else {
		xpLog.WriteU8(1) // 1 = non-kill XP (quest, exploration)
	}
	// Player::SendLogXPGain (Player.cpp:2637): the RaF flag tells the
	// client the grant already includes the recruit-a-friend bonus.
	if recruitAFriend {
		xpLog.WriteU8(1)
	} else {
		xpLog.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_LOG_XPGAIN), xpLog.Bytes(), true)

	s.player.XP += totalXP
	for s.player.Level < maxPlayerLevel {
		needed := s.server.xpForLevel(ctx, uint32(s.player.Level))
		if needed == 0 || s.player.XP < needed {
			break
		}
		s.player.XP -= needed
		s.applyLevelUp(ctx, s.player.MaxHealth, s.player.MaxPowers[0])
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
	var rawSeconds int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(spawntimesecs, 0) FROM creature WHERE guid = ?", guid).Scan(&rawSeconds); err != nil || rawSeconds < 0 {
		rawSeconds = 0
	}
	var entry, mapID int64
	var x, y, z float64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map, position_x, position_y, position_z FROM creature WHERE guid = ?", guid).Scan(&entry, &mapID, &x, &y, &z)
	if rawSeconds == 0 && s.autoBalanceIsDungeonBoss(ctx, uint32(entry)) {
		// Creature::setDeathState JUST_DIED (Creature.cpp:2085-2093): a
		// dungeon boss (Creature::IsDungeonBoss, Creature.h:112 — the
		// instance_encounters kill-credit set behind
		// CREATURE_FLAG_EXTRA_DUNGEON_BOSS) with no respawn delay never
		// respawns; the old NULLIF(...,0)->300 default resurrected it
		// after 5 minutes.
		return
	}
	seconds := rawSeconds
	if seconds <= 0 {
		seconds = 300
	}
	// Eluna CREATURE_EVENT_ON_CORPSE_REMOVED (26): C++ fires it from
	// Creature::RemoveCorpse (Creature.cpp:393/429) and applies the rewrite as
	// m_respawnTime = max(now + respawnDelay, m_respawnTime) — the hook can
	// only extend the respawn timer, never shorten it. Go has no separate
	// corpse-removal phase; the respawn delay is consumed here, so the hook
	// fires with the spawntimesecs delay and only an extension takes effect.
	if motion := s.findCreatureMotion(uint32(mapID), 0, creatureWorldGUID(guid, uint32(entry))); motion != nil {
		delay := uint32(seconds)
		if rewritten := s.fireCreatureCorpseRemoved(ctx, motion, delay); rewritten > delay {
			seconds = int64(rewritten)
		}
	}
	s.motionMu.Lock()
	if s.creatureRespawns == nil {
		s.creatureRespawns = make(map[uint32]creatureRespawn)
	}
	s.creatureRespawns[guid] = creatureRespawn{GUID: guid, At: now.Add(time.Duration(seconds) * time.Second), Health: health, Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z)}
	s.motionMu.Unlock()
}

// reseatRespawnedEntry reverts a scripted UpdateEntry transform when the
// creature respawns: C++ Creature::Respawn re-inits from the DB row
// (Creature::LoadCreatureFromDB -> InitEntry(original entry),
// Creature.cpp:2156-2215), so a transform never survives death. The latch
// is cleared so the idle sweep re-affirms DB values again.
func (s *Server) reseatRespawnedEntry(ctx context.Context, motion *creatureMotion, dbEntry uint32) {
	if s == nil || motion == nil || dbEntry == 0 {
		return
	}
	s.motionMu.Lock()
	overridden := motion.EntryOverride
	s.motionMu.Unlock()
	if !overridden {
		return
	}
	if s.seatCreatureEntry(ctx, motion, dbEntry) {
		s.motionMu.Lock()
		motion.EntryOverride = false
		s.motionMu.Unlock()
	}
}

func (s *Server) scheduleInstanceCreatureRespawn(ctx context.Context, target combatTarget, health uint32, now time.Time) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || target.InstanceID == 0 {
		return
	}
	guid := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32(target.GUID>>24) & 0x00FFFFFF
	var rawSeconds int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(spawntimesecs, 0) FROM creature WHERE guid = ?", guid).Scan(&rawSeconds); err != nil || rawSeconds < 0 {
		rawSeconds = 0
	}
	var spawnEntry int64
	var mapID int64
	var x, y, z float64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map, position_x, position_y, position_z FROM creature WHERE guid = ?", guid).Scan(&spawnEntry, &mapID, &x, &y, &z)
	if spawnEntry <= 0 {
		spawnEntry = int64(entry)
	}
	if rawSeconds == 0 && s.autoBalanceIsDungeonBoss(ctx, uint32(spawnEntry)) {
		// Creature::setDeathState JUST_DIED (Creature.cpp:2085-2093): a
		// dungeon boss (Creature::IsDungeonBoss, Creature.h:112) with no
		// respawn delay never respawns in the instance; the old
		// NULLIF(...,0)->300 default resurrected it after 5 minutes.
		return
	}
	seconds := rawSeconds
	if seconds <= 0 {
		seconds = 300
	}
	// Eluna CREATURE_EVENT_ON_CORPSE_REMOVED (26), instance path: same
	// extend-only rewrite semantics as the world path above
	// (Creature::RemoveCorpse, Creature.cpp:393/429).
	if motion := s.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
		delay := uint32(seconds)
		if rewritten := s.fireCreatureCorpseRemoved(ctx, motion, delay); rewritten > delay {
			seconds = int64(rewritten)
		}
	}
	key := instanceAdmissionKey{MapID: target.Map, InstanceID: target.InstanceID}
	s.motionMu.Lock()
	if s.instanceCreatureRespawns == nil {
		s.instanceCreatureRespawns = make(map[instanceAdmissionKey]map[uint32]creatureRespawn)
	}
	if s.instanceCreatureRespawns[key] == nil {
		s.instanceCreatureRespawns[key] = make(map[uint32]creatureRespawn)
	}
	s.instanceCreatureRespawns[key][guid] = creatureRespawn{GUID: guid, At: now.Add(time.Duration(seconds) * time.Second), Health: health, Entry: uint32(spawnEntry), Map: target.Map, InstanceID: target.InstanceID, X: float32(x), Y: float32(y), Z: float32(z)}
	s.motionMu.Unlock()
}

// processCreatureRespawns restores expired spawns; called from world tick.
// Reference: Creature::Respawn (Creature.cpp:2156-2215). The C++ force,
// entry/display re-selection (UpdateEntry/SelectLevel/random display
// gender), MotionMaster re-init, InitializeReactState, ai->Reset(), pool
// membership, and corpse-removal/despawn legs have no Go analog — no corpse
// objects, no spawn pools, no MotionMaster, and Lua CREATURE_EVENT_ON_SPAWN
// (5) is the AI re-init point. Respawn only restores the recorded spawn
// health/position and clears the combat flags; the motion's own threat
// table is already empty from the death chain (cleared at the kill site,
// and the dead unit was dropped from everyone else's table by
// removeThreatVictimFromAllLists), so no threat reset is needed here.
func (s *Server) processCreatureRespawns(ctx context.Context, now time.Time) {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.motionMu.Lock()
	due := make([]creatureRespawn, 0, 8)
	type instanceRespawn struct {
		key     instanceAdmissionKey
		respawn creatureRespawn
	}
	instanceDue := make([]instanceRespawn, 0, 8)
	for guid, respawn := range s.creatureRespawns {
		if now.After(respawn.At) {
			due = append(due, respawn)
			delete(s.creatureRespawns, guid)
		}
	}
	for key, respawns := range s.instanceCreatureRespawns {
		for guid, respawn := range respawns {
			if now.After(respawn.At) {
				instanceDue = append(instanceDue, instanceRespawn{key: key, respawn: respawn})
				delete(respawns, guid)
			}
		}
		if len(respawns) == 0 {
			delete(s.instanceCreatureRespawns, key)
		}
	}
	s.motionMu.Unlock()
	for _, respawn := range due {
		if _, err := s.WorldStore.DB.ExecContext(ctx, "UPDATE creature SET curhealth = ? WHERE guid = ?", respawn.Health, respawn.GUID); err != nil {
			continue
		}
		if respawn.Entry != 0 {
			rawGUID := creatureWorldGUID(respawn.GUID, respawn.Entry)
			var motion *creatureMotion
			s.motionMu.Lock()
			if m := s.findCreatureMotionLocked(respawn.Map, 0, rawGUID); m != nil {
				motion = m
				motion.Health = respawn.Health
				motion.MaxHealth = respawn.Health
				motion.BaseMaxHealth = respawn.Health
				motion.X, motion.Y, motion.Z = respawn.X, respawn.Y, respawn.Z
				motion.InCombat, motion.TargetGUID, motion.Moving = false, 0, false
				motion.Looted = false
				// Creature::JUST_RESPAWNED (Creature.cpp:2121-2125):
				// SetLootRecipient(nullptr) + ResetPlayerDamageReq().
				motion.TapPlayerGUID, motion.TapGroupID = 0, 0
				motion.PlayerDamageReq = respawn.Health / 2
			}
			s.motionMu.Unlock()
			s.reseatRespawnedEntry(ctx, motion, respawn.Entry)
			s.clearLootState(respawn.Map, 0, rawGUID)
			s.broadcastCreatureValuesUpdateInInstance(respawn.Map, 0, rawGUID, map[int]uint32{unitFieldHealth: respawn.Health, unitFieldDynamicFlags: 0})
			// Eluna CREATURE_EVENT_ON_SPAWN (5), with the event-23 On_Reset
			// pre-fire (Eluna::JustRespawned, CreatureHooks.cpp:210-216).
			if motion != nil {
				s.fireCreatureSpawned(ctx, motion)
				// Creature::Respawn (Creature.cpp:2198): ai->Reset() runs on
				// every respawn — the instance path already calls OnReset;
				// the world path needs the same arm (Eluna-first, matching
				// ElunaCreatureAI::JustAppeared calling sEluna before
				// ScriptedAI::JustAppeared).
				if motion.BossAI != nil {
					motion.BossAI.OnReset(ctx, s, motion)
				}
			}
		}
	}
	for _, item := range instanceDue {
		respawn := item.respawn
		s.motionMu.Lock()
		motion := s.findCreatureMotionLocked(item.key.MapID, item.key.InstanceID, creatureWorldGUID(respawn.GUID, respawn.Entry))
		if motion != nil {
			motion.Health, motion.MaxHealth, motion.DynamicFlags = respawn.Health, respawn.Health, 0
			motion.X, motion.Y, motion.Z = respawn.X, respawn.Y, respawn.Z
			motion.InCombat, motion.Evading, motion.TargetGUID, motion.Moving = false, false, 0, false
			motion.Looted = false
			motion.Refreshed = now
			// Creature::JUST_RESPAWNED (Creature.cpp:2121-2125):
			// SetLootRecipient(nullptr) + ResetPlayerDamageReq().
			motion.TapPlayerGUID, motion.TapGroupID = 0, 0
			motion.DynamicFlags &^= unitDynFlagTapped | unitDynFlagLootable
			motion.PlayerDamageReq = respawn.Health / 2
		}
		s.motionMu.Unlock()
		s.reseatRespawnedEntry(ctx, motion, respawn.Entry)
		s.clearLootState(item.key.MapID, item.key.InstanceID, creatureWorldGUID(respawn.GUID, respawn.Entry))
		if motion != nil {
			// Eluna CREATURE_EVENT_ON_SPAWN (5), with the event-23 On_Reset
			// pre-fire (Eluna::JustRespawned, CreatureHooks.cpp:210-216) —
			// ahead of the native boss reset, matching ElunaCreatureAI::
			// JustAppeared calling sEluna before ScriptedAI::JustAppeared.
			s.fireCreatureSpawned(ctx, motion)
			if motion.BossAI != nil {
				motion.BossAI.OnReset(ctx, s, motion)
			}
			s.broadcastCreatureValuesUpdateInInstance(item.key.MapID, item.key.InstanceID, motion.GUID, map[int]uint32{unitFieldHealth: motion.Health, unitFieldDynamicFlags: 0})
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
			// Player::KilledPlayerCreditForQuest (Player.cpp:16723):
			// completion runs through Player::CompleteQuest — C++ sends
			// no SMSG_QUESTUPDATE_COMPLETE here (Player::SendQuestComplete
			// fires only from Player::AreaExploredOrEventHappens,
			// Player.cpp:16518), so none rides along.
			s.completeQuest(ctx, entry.QuestID)
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

// grantCreatureKillQuestCredit runs the KillRewarder kill-credit leg
// (KillRewarder.cpp:183-191, 236-256): kill credit is skipped on
// battleground maps (the _isBattleGround arm); solo, the killer is
// credited directly; in a group every member at group reward distance
// shares the credit (the killer is always in range), gated on
// alive-or-no-corpse (Player::RewardPlayerAndGroupAtEvent,
// Player.cpp:24132-24158 — same map and instance, corpse-anchored,
// dungeon-always, else within MaxGroupXPDistance /
// CONFIG_GROUP_XP_DISTANCE).
// grantCreatureKillQuestCredit runs KillRewarder::_RewardKillCredit
// (KillRewarder.cpp:186-195) per rewarded member: the KilledMonster fan-out
// plus the KILL_CREATURE_TYPE achievement criteria (which C++ fires for every
// group member in the reward loop, not just the killer). The alive-or-
// corpse-unreleased gate is Player.cpp:24152's IsAlive() || !GetCorpse().
func (s *session) grantCreatureKillQuestCredit(ctx context.Context, creatureEntry, creatureType uint32, target combatTarget) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	if s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(target.Map); err == nil && found && mapInfo.IsBattleground() {
			return
		}
	}
	if s.groupID == 0 {
		s.creditQuestKills(ctx, creatureEntry, target.GUID)
		s.updateAchievementCriteria(criteriaTypeKillCreatureType, creatureType, 1)
		return
	}
	inDungeon := s.isDungeonMap(target.Map)
	credited := false
	for _, m := range s.server.getGroupSessions(s.groupID) {
		if m == nil || m.player == nil {
			continue
		}
		if m == s {
			credited = true
		} else {
			if m.player.Map != target.Map || m.player.InstanceID != target.InstanceID {
				continue
			}
			mx, my, mz := groupRewardAnchorPos(m)
			if !inDungeon && distance3D(mx, my, mz, target.X, target.Y, target.Z) > s.server.Config.MaxGroupXPDistance {
				continue
			}
		}
		// Quest objectives update only for alive members or dead members
		// whose corpse is not yet released (Player.cpp:24152).
		if m.isDeadOrGhost() && m.player.hasCorpse {
			continue
		}
		m.creditQuestKills(ctx, creatureEntry, target.GUID)
		m.updateAchievementCriteria(criteriaTypeKillCreatureType, creatureType, 1)
	}
	if !credited {
		s.creditQuestKills(ctx, creatureEntry, target.GUID)
		s.updateAchievementCriteria(criteriaTypeKillCreatureType, creatureType, 1)
	}
}

// creditQuestKills runs Player::KilledMonster (Player.cpp:16624): the
// KillCredit1/2 template fan-out, then the per-quest credit loop shared
// with the gameobject-activate arm. Each fanned-out entry runs the full
// Player::KilledMonsterCredit (Player.cpp:16637): the timed achievement
// starts FIRST, then the per-entry kill criteria, then the quest loop.
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
	for _, entry := range entries {
		s.startTimedAchievement(timedTypeCreature, entry)
		s.updateAchievementCriteria(criteriaTypeKillCreature, entry, 1)
	}
	s.creditQuestObjectiveEntries(ctx, entries, victimGUID, false)
}

// creditQuestGOActivation runs Player::KillCreditGO (Player.cpp:16757): the
// CAST-flag arm for gameobject-activate objectives (negative
// RequiredNpcOrGo entries) — no KillCredit template fan-out and no
// raid-group gate (C++ has neither here).
func (s *session) creditQuestGOActivation(ctx context.Context, goEntry uint32, guid uint64) {
	if s.player == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	s.creditQuestObjectiveEntries(ctx, []uint32{goEntry}, guid, true)
}

// creditQuestObjectiveEntries is the shared Player::KilledMonsterCredit /
// Player::KillCreditGO quest loop. goActivate selects the sign of the
// matched RequiredNpcOrGo objectives (negative for gameobject-activate,
// positive for kill credit — each C++ arm skips the other's sign) and the
// SMSG_QUESTUPDATE_ADD_KILL entry form: creature entries are sent raw while
// gameobject ids go out as (-id)|0x80000000
// (Player::SendQuestUpdateAddCreatureOrGo, Player.cpp:17104-17107). The KILL
// and CAST special flags are load-computed from the same nonzero
// RequiredNpcOrGo columns (ObjectMgr.cpp:5004), so objective matching makes
// them vacuous here. The raid-group gate
// (!GetGroup() || !GetGroup()->isRaidGroup() ||
// qInfo->IsAllowedInRaid(GetMap()->GetDifficulty()), Player.cpp:16655)
// applies to kill credit only.
func (s *session) creditQuestObjectiveEntries(ctx context.Context, entries []uint32, guid uint64, goActivate bool) {
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		entry := s.player.QuestLog[slot]
		if entry.QuestID == 0 {
			continue
		}
		var reqIDs, reqCounts [4]int64
		var questType int64
		err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4, RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4, QuestType FROM quest_template WHERE ID = ?", entry.QuestID).Scan(&reqIDs[0], &reqIDs[1], &reqIDs[2], &reqIDs[3], &reqCounts[0], &reqCounts[1], &reqCounts[2], &reqCounts[3], &questType)
		if err != nil {
			continue
		}
		if !goActivate && !s.questAllowedInRaid(uint32(questType)) {
			continue
		}
		progressed := false
		for objective := 0; objective < 4; objective++ {
			required := uint32(reqCounts[objective])
			reqID := reqIDs[objective]
			if required == 0 || reqID == 0 {
				continue
			}
			var want, packetEntry uint32
			if goActivate {
				if reqID > 0 {
					continue
				}
				want = uint32(-reqID)
				packetEntry = want | 0x80000000
			} else {
				if reqID < 0 {
					continue
				}
				want = uint32(reqID)
				packetEntry = want
			}
			match := false
			for _, e := range entries {
				if e == want {
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
			update.WriteU32(packetEntry)
			update.WriteU32(uint32(entry.Counters[objective]))
			update.WriteU32(required)
			update.WriteU64(guid)
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
			// Player::KilledMonsterCredit (Player.cpp:16636): completion
			// runs through Player::CompleteQuest — C++ sends no
			// SMSG_QUESTUPDATE_COMPLETE here (Player::SendQuestComplete
			// fires only from Player::AreaExploredOrEventHappens,
			// Player.cpp:16518), so none rides along.
			entry.State = questCompleteStateFlag(questStatusComplete)
			s.player.QuestLog[slot] = entry
			s.completeQuest(ctx, entry.QuestID)
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

// handleEffectKillCredit processes SPELL_EFFECT_KILL_CREDIT (90).
// Mirrors TrinityCore Spell::EffectKillCreditPersonal (SpellEffects.cpp:5253-5261):
// effect 90 credits the single target player via Player::KilledMonsterCredit
// (Player.cpp:16636) — no group iteration, no Burn Body special-case (those
// belong to effect 134, handleEffectKillCreditGroup).
func (s *session) handleEffectKillCredit(ctx context.Context, targetGUID uint64, miscValue int32) {
	if s == nil || s.player == nil {
		return
	}
	var entry uint32
	if miscValue > 0 {
		entry = uint32(miscValue)
	}
	if entry == 0 {
		return
	}

	// Spell::EffectKillCreditPersonal requires a player unit target
	// (SpellEffects.cpp:5258).
	targetSess := s.resolveKillCreditTarget(targetGUID)
	if targetSess == nil {
		return
	}
	targetSess.grantKillCreditMonster(ctx, entry, targetGUID)
}

// resolveKillCreditTarget resolves the player session for a kill-credit
// target GUID: the caster's own session when the target is the caster or
// unset, else the online session holding that player GUID.
func (s *session) resolveKillCreditTarget(targetGUID uint64) *session {
	var targetSess *session
	if targetGUID == 0 || targetGUID == s.playerGUID {
		targetSess = s
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}
	if targetSess == nil || targetSess.player == nil {
		return nil
	}
	return targetSess
}

// grantKillCreditMonster runs Player::KilledMonsterCredit (Player.cpp:16636):
// the timed achievement and the KILL_CREATURE criterion run before the quest
// loop; the quest loop itself rides creditQuestKills (the real_entry-via-
// creature-GUID arm is vacuous here — C++ passes the player target as
// pRewardSource, so creature_guid is empty and real_entry == entry).
func (m *session) grantKillCreditMonster(ctx context.Context, entry uint32, targetGUID uint64) {
	// The timed achievement and the KILL_CREATURE criterion ride
	// creditQuestKills (the KilledMonsterCredit arms).
	m.creditQuestKills(ctx, entry, targetGUID)
}

// handleEffectKillCreditGroup processes SPELL_EFFECT_KILL_CREDIT group variant (134).
// Mirrors TrinityCore Spell::EffectKillCredit (SpellEffects.cpp:5264-5280)
// plus the Player::RewardPlayerAndGroupAtEvent (Player.cpp:24132-24158) and
// Player::KilledMonsterCredit (Player.cpp:16636) tails it calls.
// creatureEntry is the effect's MiscValue, with the Burn Body (42793)
// special-case entry 24008 (Fallen Combatant). Grouped: every member at
// Player::IsAtGroupRewardDistance of the target player (same map and
// instance, dungeon-always, else within MaxGroupXPDistance /
// CONFIG_GROUP_XP_DISTANCE, dead members anchored on their corpse via
// groupRewardAnchorPos — Player.cpp:24165-24166) gets the credit, but only
// while alive or not yet released (Player.cpp:24152). Solo: the target
// player gets it directly.
func (s *session) handleEffectKillCreditGroup(ctx context.Context, targetGUID uint64, spellID uint32, miscValue int32) {
	if s == nil || s.player == nil {
		return
	}
	entry := uint32(0)
	if miscValue > 0 {
		entry = uint32(miscValue)
	}
	if entry == 0 && spellID == 42793 { // Burn Body
		entry = 24008 // Fallen Combatant
	}
	if entry == 0 {
		return
	}

	// Spell::EffectKillCredit requires a player unit target
	// (SpellEffects.cpp:5269).
	targetSess := s.resolveKillCreditTarget(targetGUID)
	if targetSess == nil {
		return
	}

	if targetSess.groupID == 0 || targetSess.server == nil {
		targetSess.grantKillCreditMonster(ctx, entry, targetGUID)
		return
	}
	tx, ty, tz := targetSess.player.X, targetSess.player.Y, targetSess.player.Z
	dungeon := targetSess.isDungeonMap(targetSess.player.Map)
	for _, m := range targetSess.server.getGroupSessions(targetSess.groupID) {
		if m == nil || m.player == nil {
			continue
		}
		if m.player.Map != targetSess.player.Map || m.player.InstanceID != targetSess.player.InstanceID {
			continue
		}
		mx, my, mz := groupRewardAnchorPos(m)
		if !dungeon && distance3D(mx, my, mz, tx, ty, tz) > targetSess.server.Config.MaxGroupXPDistance {
			continue
		}
		if m.isDeadOrGhost() && m.player.hasCorpse {
			continue
		}
		m.grantKillCreditMonster(ctx, entry, targetGUID)
	}
}

// applyLevelUp runs one Player::GiveLevel step (Player.cpp:2694-2795): the
// level increment, reach-level criteria, stat recalculation, SMSG_LEVELUP_INFO
// with health/power/stat deltas, skill/talent updates, pet sync, level-reward
// mail, DB persist, and the level-changed hook. oldHP/oldMana are the
// pre-level MaxHealth/MaxPowers[0] used for the packet deltas.
func (s *session) applyLevelUp(ctx context.Context, oldHP, oldMana uint32) {
	oldStats := s.player.Stats
	oldLevel := s.player.Level
	s.player.Level++
	s.setAchievementCriteria(criteriaTypeReachLevel, uint32(s.player.Level), uint32(s.player.Level))
	// AutoBalance_PlayerScript::OnLevelChanged (AutoBalance.cpp).
	s.autoBalanceOnLevelChanged()
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
	// Reference: Player::GiveLevel mails level rewards from mail_level_reward
	// (Player.cpp:2768) once per reached level.
	s.sendLevelUpMail(ctx, s.player.Level)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET level = ?, xp = ?, health = ? WHERE guid = ?", s.player.Level, s.player.XP, s.player.Health, s.playerGUID)
		for _, sk := range s.player.Skills {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET max = ? WHERE guid = ? AND skill = ?", sk.Max, s.playerGUID, sk.Skill)
		}
	}
	// Player::GiveLevel (Player.cpp:2787): OnPlayerLevelChanged fires once
	// per level gained, after the level-up processing.
	s.fireLevelChangedHook(ctx, oldLevel)
}

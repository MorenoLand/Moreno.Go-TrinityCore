package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellAttributePassive uint32 = 0x00000040
	spellCastFlagStart    uint32 = 0x00000002
	spellCastFlagGo       uint32 = 0x00000100
	spellCastFlagPending  uint32 = 0x00000001

	spellAttr3MainHand             uint32 = 0x00000400 // SPELL_ATTR3_MAIN_HAND: Require main hand weapon (SharedDefines.h:533)
	spellAttr3ReqOffhand           uint32 = 0x01000000 // SPELL_ATTR3_REQ_OFFHAND: Require offhand weapon (SharedDefines.h:547)
	spellAttr3ReqWand              uint32 = 0x00400000 // SPELL_ATTR3_REQ_WAND: Requires equipped Wand (SharedDefines.h:545)
	spellAttr5HideDuration         uint32 = 0x00000400 // SPELL_ATTR5_HIDE_DURATION (SharedDefines.h:607)
	spellAttr5CanChannelWhenMoving uint32 = 0x00000001 // SPELL_ATTR5_CAN_CHANNEL_WHEN_MOVING (SharedDefines.h:597)
	spellAttr5SingleTarget         uint32 = 0x00000020 // SPELL_ATTR5_SINGLE_TARGET_SPELL (SharedDefines.h:602)

	spellInterruptFlagMovement uint32 = 0x01 // SPELL_INTERRUPT_FLAG_MOVEMENT (SpellDefines.h:30)

	spellAttr1NotBreakStealth uint32 = 0x00000020 // SPELL_ATTR1_NOT_BREAK_STEALTH (SharedDefines.h:454)
	spellAttr1NoThreat        uint32 = 0x00000400 // SPELL_ATTR1_NO_THREAT (SharedDefines.h:459) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr3NoInitialAggro  uint32 = 0x00020000 // SPELL_ATTR3_NO_INITIAL_AGGRO (SharedDefines.h:540) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7)

	spellAttr0Ability                     uint32 = 0x00000010 // SPELL_ATTR0_ABILITY (SharedDefines.h:416)
	spellAttr0ReqAmmo                     uint32 = 0x00000002 // SPELL_ATTR0_REQ_AMMO (SharedDefines.h:413)
	spellAttr0Tradespell                  uint32 = 0x00000020 // SPELL_ATTR0_TRADESPELL (SharedDefines.h:417)
	spellAttr3NoDoneBonus                 uint32 = 0x20000000 // SPELL_ATTR3_NO_DONE_BONUS (SharedDefines.h:552) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr3TreatAsPeriodic             uint32 = 0x02000000 // SPELL_ATTR3_TREAT_AS_PERIODIC (SharedDefines.h:548) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr3StackForDiffCasters         uint32 = 0x00000080 // SPELL_ATTR3_STACK_FOR_DIFF_CASTERS (SharedDefines.h:530) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr7NoPushbackOnDamage          uint32 = 0x00000040 // SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE (SharedDefines.h:677) — ATTR7 is Go's AttributesEx7 (Spell.dbc field 11 = AttributesExG)
	spellAttr7DispelCharges               uint32 = 0x00000400 // SPELL_ATTR7_DISPEL_CHARGES (SharedDefines.h:681) — ATTR7 is Go's AttributesEx7 (Spell.dbc field 11 = AttributesExG)
	spellAttr6AssistIgnoreImmuneFlag      uint32 = 0x00000008 // SPELL_ATTR6_ASSIST_IGNORE_IMMUNE_FLAG (SharedDefines.h:637) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr6CanTargetUntargetable       uint32 = 0x01000000 // SPELL_ATTR6_CAN_TARGET_UNTARGETABLE (SharedDefines.h:658) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr4NotStealable                uint32 = 0x00000040 // SPELL_ATTR4_NOT_STEALABLE (SharedDefines.h:566) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr0UnaffectedByInvulnerability uint32 = 0x20000000 // SPELL_ATTR0_UNAFFECTED_BY_INVULNERABILITY (SharedDefines.h:441)
	spellAttr0NotShapeshift               uint32 = 0x00010000 // SPELL_ATTR0_NOT_SHAPESHIFT (SharedDefines.h:428)
	spellAttr2NotNeedShapeshift           uint32 = 0x00080000 // SPELL_ATTR2_NOT_NEED_SHAPESHIFT (SharedDefines.h:505) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr1CantBeReflected             uint32 = 0x00000080 // SPELL_ATTR1_CANT_BE_REFLECTED (SharedDefines.h:456)
	spellAttr2CanTargetDead               uint32 = 0x00000001 // SPELL_ATTR2_CAN_TARGET_DEAD (SharedDefines.h:486) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr2AutorepeatFlag              uint32 = 0x00000020 // SPELL_ATTR2_AUTOREPEAT_FLAG (SharedDefines.h:491) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr2NotResetAutoActions         uint32 = 0x00020000 // SPELL_ATTR2_NOT_RESET_AUTO_ACTIONS (SharedDefines.h:503) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)

	targetFlagCorpseEnemy uint32 = 0x00000200 // TARGET_FLAG_CORPSE_ENEMY (SpellInfo.h:57)
	targetFlagUnitDead    uint32 = 0x00000400 // TARGET_FLAG_UNIT_DEAD (SpellInfo.h:58)
	targetFlagCorpseAlly  uint32 = 0x00008000 // TARGET_FLAG_CORPSE_ALLY (SpellInfo.h:63)

	spellDamageClassMagic uint32 = 1 // SPELL_DAMAGE_CLASS_MAGIC (SharedDefines.h:1580)

	spellAttr0StopAttackTarget       uint32 = 0x00100000 // SPELL_ATTR0_STOP_ATTACK_TARGET (SharedDefines.h:432)
	spellAttr0DisabledWhileActive    uint32 = 0x02000000 // SPELL_ATTR0_DISABLED_WHILE_ACTIVE (SharedDefines.h:437)
	spellAttr0LevelDamageCalculation uint32 = 0x00080000 // SPELL_ATTR0_LEVEL_DAMAGE_CALCULATION (SharedDefines.h:431)

	spellFailedEquippedItemClass         uint8 = 29  // SPELL_FAILED_EQUIPPED_ITEM_CLASS (SharedDefines.h:1011)
	spellFailedEquippedItemClassMainhand uint8 = 30  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_MAINHAND (SharedDefines.h:1012)
	spellFailedEquippedItemClassOffhand  uint8 = 31  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_OFFHAND (SharedDefines.h:1013)
	spellFailedNotInFront                uint8 = 61  // SPELL_FAILED_NOT_INFRONT (SharedDefines.h:1042)
	spellFailedBadTargets                uint8 = 12  // SPELL_FAILED_BAD_TARGETS (SharedDefines.h:992)
	spellFailedBmOrInvisGod              uint8 = 159 // SPELL_FAILED_BM_OR_INVISGOD (SharedDefines.h:1141)
	spellFailedTargetIsPlayer            uint8 = 117 // SPELL_FAILED_TARGET_IS_PLAYER (SharedDefines.h:1099)
	spellFailedAffectingCombat           uint8 = 1
	spellFailedFoodLowLevel              uint8 = 35
	spellFailedNoPet                     uint8 = 84
	spellFailedWrongPetFood              uint8 = 135
	spellFailedNotReady                  uint8 = 67  // SPELL_FAILED_NOT_READY (SharedDefines.h:1049)
	spellFailedDontReport                uint8 = 27  // SPELL_FAILED_DONT_REPORT (SharedDefines.h:1009)
	spellFailedSilenced                  uint8 = 104 // SPELL_FAILED_SILENCED (SharedDefines.h:1086)
	spellFailedCasterDead                uint8 = 23  // SPELL_FAILED_CASTER_DEAD (SharedDefines.h:1003)
	spellFailedNotFishable               uint8 = 58  // SPELL_FAILED_NOT_FISHABLE (SharedDefines.h:1040)
	spellFailedCharmed                   uint8 = 24  // SPELL_FAILED_CHARMED (SharedDefines.h:1006)
	spellFailedConfused                  uint8 = 26  // SPELL_FAILED_CONFUSED (SharedDefines.h:1008)
	spellFailedFleeing                   uint8 = 34  // SPELL_FAILED_FLEEING (SharedDefines.h:1016)
	spellFailedCasterAuraState           uint8 = 22  // SPELL_FAILED_CASTER_AURASTATE (SharedDefines.h:1004)
	spellFailedTargetAuraState           uint8 = 111 // SPELL_FAILED_TARGET_AURASTATE (SharedDefines.h:1093)
	spellFailedNotShapeshift             uint8 = 68  // SPELL_FAILED_NOT_SHAPESHIFT (SharedDefines.h:1050)
	spellFailedOnlyShapeshift            uint8 = 94  // SPELL_FAILED_ONLY_SHAPESHIFT (SharedDefines.h:1076)
	spellFailedRequiresSpellFocus        uint8 = 102 // SPELL_FAILED_REQUIRES_SPELL_FOCUS (SharedDefines.h:1084)
	spellFailedTotemCategory             uint8 = 130 // SPELL_FAILED_TOTEM_CATEGORY (SharedDefines.h:1112)
	spellFailedTotems                    uint8 = 131 // SPELL_FAILED_TOTEMS (SharedDefines.h:1113)
	spellFailedLowLevel                  uint8 = 48  // SPELL_FAILED_LOWLEVEL (SharedDefines.h:1030)
	spellFailedNotKnown                  uint8 = 63  // SPELL_FAILED_NOT_KNOWN (SharedDefines.h:1045)
	spellFailedItemEnchantTradeWindow    uint8 = 182 // SPELL_FAILED_ITEM_ENCHANT_TRADE_WINDOW (SharedDefines.h:1164)

	spellImplicitTargetUnitPet uint32 = 5 // TARGET_UNIT_PET (SharedDefines.h:1446)

	itemClassWeapon = 2
	itemClassArmor  = 4

	itemSubclassArmorBuckler = 5
	itemSubclassArmorShield  = 6

	spellEffectEnergize                      = 30
	spellEffectParry                         = 22
	spellEffectPowerBurn                     = 62
	spellEffectThreat                        = 63
	spellEffectTriggerSpell                  = 64
	spellEffectHealMaxHealth                 = 67
	spellEffectCreateItem                    = 24
	spellEffectCreateItem2                   = 70
	spellEffectLearnSpell                    = 36
	spellEffectLearnPetSpell                 = 57 // SPELL_EFFECT_LEARN_PET_SPELL (SharedDefines.h:868)
	spellEffectResurrect                     = 18
	spellEffectReputation                    = 103
	spellEffectQuestComplete                 = 16
	spellEffectHealthLeech                   = 9
	spellEffectPowerDrain                    = 8
	spellEffectHealMechanical                = 75  // SPELL_EFFECT_HEAL_MECHANICAL (SharedDefines.h:886)
	spellEffectHealPct                       = 136 // SPELL_EFFECT_HEAL_PCT (SharedDefines.h:947)
	spellEffectEnergizePct                   = 137 // SPELL_EFFECT_ENERGIZE_PCT (SharedDefines.h:948)
	spellAuraMounted                         = 78
	spellAuraModParryPercent                 = 47
	spellAuraModSpellCritChance              = 57  // SPELL_AURA_MOD_SPELL_CRIT_CHANCE (SpellAuraDefines.h:137)
	spellAuraModSpellCritChanceSchool        = 71  // SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL (SpellAuraDefines.h:151)
	spellAuraModCritPct                      = 290 // SPELL_AURA_MOD_CRIT_PCT (SpellAuraDefines.h:370)
	spellAuraConfuse                         = 5
	spellAuraCharm                           = 6
	spellAuraFear                            = 7
	spellAuraStun                            = 12
	spellAuraRoot                            = 26
	spellAuraStealth                         = 16
	spellAuraInvisibility                    = 18
	spellAuraStealthDetect                   = 17
	spellAuraInvisibilityDetect              = 19
	spellAuraStealthLevel                    = 154
	spellAuraTrackStealthed                  = 151
	spellAuraConvertRune                     = 249
	spellAuraDamagePercentDone               = 79
	spellAuraAttackPowerPercent              = 166
	spellAuraRangedAttackPowerPercent        = 167
	spellAuraCastingSpeedNotStack            = 65
	spellAuraHasteSpells                     = 216
	spellAuraFakeInebriation                 = 304
	unitStandFlagCreep                       = 0x02
	playerAuraVisionStealth                  = 0x20
	playerAuraVisionInvis                    = 0x40
	playerFieldByteTrackStealthed     uint32 = 0x00000002
)

// isSelfCastOnly checks if all active spell effects target the caster unit.
func isSelfCastOnly(spell wotlk.Spell) bool {
	hasEffect := false
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		hasEffect = true
		// TrinityCore SpellInfo::IsSelfCast requires TARGET_UNIT_CASTER (1) for every active effect.
		if eff.ImplicitTargetA != 1 {
			return false
		}
	}
	return hasEffect
}

func isAreaEnemySpell(spell wotlk.Spell) bool {
	if !isHarmfulSpell(spell) {
		return false
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.Effect == 129 {
			return true
		}
		if eff.Effect == 27 && eff.ImplicitTargetA == 18 {
			return true
		}
		if isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

func isAreaEnemyTargetType(target uint32) bool {
	switch target {
	case 2, 15, 16, 22, 24, 28, 54, 104:
		return true
	default:
		return false
	}
}

// spellEffectTargetsUnit mirrors SpellEffectInfo::GetUsedTargetObjectType()
// (the static _data table, SpellInfo.cpp:610-614) reporting true when the
// effect's used target object type is TARGET_OBJECT_TYPE_UNIT (SpellInfo.h:103).
// Spell::prepare (Spell.cpp:3178-3188) removes AURA_INTERRUPT_FLAG_SPELL_ATTACK
// auras when the breaking-stealth spell has any unit-typed effect.
func spellEffectTargetsUnit(effect uint32) bool {
	switch effect {
	case 1, 2, 6, 7, 8, 9, 10, 11, 16, 17, 19, 20, 21, 22, 23, 24, 25, 26,
		30, 31, 34, 35, 36, 37, 38, 39, 40, 41, 44, 45, 46, 47, 48, 49,
		51, 52, 55, 57, 58, 59, 60, 62, 63, 65, 66, 67, 68, 70, 71, 73,
		74, 75, 78, 79, 80, 82, 84, 90, 91, 92, 93, 94, 95, 96, 97, 98,
		100, 102, 103, 108, 110, 111, 112, 114, 115, 117, 118, 119, 120,
		121, 123, 124, 125, 126, 128, 129, 130, 131, 132, 133, 134, 136,
		137, 138, 139, 140, 141, 142, 143, 146, 147, 150, 153, 154, 155,
		157, 159, 160, 161, 162, 163, 164:
		return true
	default:
		return false
	}
}

func (s *session) spellAreaEnemyTargets(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isAreaEnemySpell(spell) || s.server.Data == nil {
		return nil
	}
	radius := float32(0)
	cone := false
	destinationCenter := false
	for _, eff := range spell.Effects {
		areaTargetA := isAreaEnemyTargetType(eff.ImplicitTargetA) || (eff.Effect == 27 && eff.ImplicitTargetA == 18)
		areaTargetB := isAreaEnemyTargetType(eff.ImplicitTargetB) || (eff.Effect == 27 && eff.ImplicitTargetB == 18)
		if eff.Effect == 0 || (!areaTargetA && !areaTargetB) {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 24 || targetType == 54 || targetType == 104 {
				cone = true
			}
			if targetType == 16 || targetType == 18 || targetType == 28 {
				destinationCenter = true
			}
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && value > radius {
			radius = value
		}
	}
	if radius <= 0 {
		return nil
	}
	centerX, centerY, centerZ := s.player.X, s.player.Y, s.player.Z
	if destinationCenter && target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		centerX, centerY, centerZ = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if destinationCenter && target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			centerX, centerY, centerZ = destination.X, destination.Y, destination.Z
		}
	}
	player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, faction, unitFlags, flagsExtra, health uint32) {
		if mapID != player.Map || instanceID != player.InstanceID || health == 0 || spellTargetUnitBlocked(spell, unitFlags, flagsExtra, false) || distance3D(x, y, z, centerX, centerY, centerZ) > float64(radius) || !s.server.isAttackableFaction(faction, player) {
			return
		}
		if cone && !hasInArc(s.player.Orientation, s.player.X, s.player.Y, x, y, math.Pi/2) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		seen[guid] = struct{}{}
		targets = append(targets, guid)
	}
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(player.Map, player.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	for _, motion := range motions {
		motionGUIDs[motion.GUID] = struct{}{}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.Faction, motion.UnitFlags, motion.FlagsExtra, motion.Health)
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == s || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil || targetSession.player.Health == 0 || targetSession.player.Map != player.Map || targetSession.player.InstanceID != player.InstanceID || targetSession.playerAlliance() == s.playerAlliance() {
			continue
		}
		if distance3D(targetSession.player.X, targetSession.player.Y, targetSession.player.Z, centerX, centerY, centerZ) > float64(radius) {
			continue
		}
		if cone && !hasInArc(s.player.Orientation, s.player.X, s.player.Y, targetSession.player.X, targetSession.player.Y, math.Pi/2) {
			continue
		}
		if _, ok := seen[targetSession.playerGUID]; ok {
			continue
		}
		seen[targetSession.playerGUID] = struct{}{}
		targets = append(targets, targetSession.playerGUID)
	}
	s.server.sessionsMu.RUnlock()
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, player.Map, float64(centerX-radius), float64(centerX+radius), float64(centerY-radius), float64(centerY+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						accept(guid, uint32(mapID), player.InstanceID, float32(x), float32(y), float32(z), uint32(faction), uint32(unitFlags), uint32(flagsExtra), uint32(health))
					}
				}
			}
		}
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		// Spell.cpp:1207,1293 — cap the area/cone target list to MaxAffectedTargets
		// plus SPELL_AURA_MOD_MAX_AFFECTED_TARGETS aura modifiers, then
		// Trinity::Containers::RandomResize (Containers.h:77) keeps exactly that
		// many targets chosen uniformly at random; shuffle + truncate draws the
		// same uniform subset.
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}

// spellAffectedBySpellFamilyMask mirrors SpellInfo::IsAffected (SpellInfo.cpp:1305)
// as invoked by AuraEffect::IsAffectedOnSpell (SpellAuraEffects.cpp:848): the aura
// spell's SpellFamilyName and the aura effect's SpellClassMask must match the spell.
func spellAffectedBySpellFamilyMask(auraFamilyName uint32, auraFamilyFlags [3]uint32, spell wotlk.Spell) bool {
	if auraFamilyName == 0 {
		return true
	}
	if auraFamilyName != spell.SpellFamilyName {
		return false
	}
	if auraFamilyFlags != [3]uint32{} {
		if auraFamilyFlags[0]&spell.SpellFamilyFlags[0] == 0 && auraFamilyFlags[1]&spell.SpellFamilyFlags[1] == 0 && auraFamilyFlags[2]&spell.SpellFamilyFlags[2] == 0 {
			return false
		}
	}
	return true
}

// totalAuraModifierByAffectMask mirrors Unit::GetTotalAuraModifierByAffectMask
// (Unit.cpp:5017): sums aura amounts of the given type only from aura effects
// whose spell affects the given spell via the spell-family affect mask.
func (s *session) totalAuraModifierByAffectMask(auraType uint32, spell wotlk.Spell) int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total int32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != auraType {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != auraType || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints + 1
			}
			total += amount
		}
	}
	return total
}

func (s *session) calculateSpellPowerCost(spell wotlk.Spell) uint32 {
	cost := spell.ManaCost
	if spell.ManaCostPct > 0 && s.player != nil {
		pType := spell.PowerType
		if pType == 0 { // Mana: calculate percentage from BaseMana per TrinityCore Player::GetCreateMana()
			basePower := s.player.BaseMana
			if basePower == 0 {
				basePower = s.player.MaxPowers[0]
			}
			if basePower == 0 {
				basePower = 100
			}
			cost += (basePower * spell.ManaCostPct) / 100
		} else if pType < 7 {
			basePower := s.player.MaxPowers[pType]
			if basePower == 0 {
				basePower = s.player.Powers[pType]
			}
			if basePower == 0 {
				basePower = 100
			}
			cost += (basePower * spell.ManaCostPct) / 100
		}
	}
	return cost
}

func (s *session) hasSpellReagents(ctx context.Context, spell wotlk.Spell) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	for i := range spell.Reagent {
		if spell.Reagent[i] <= 0 {
			continue
		}
		itemID := spell.Reagent[i]
		need := spell.ReagentCount[i]
		if need == 0 {
			need = 1
		}
		var have int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count),0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, int64(itemID)).Scan(&have)
		if err != nil || have < int64(need) {
			return false
		}
	}
	return true
}

func (s *session) takeSpellReagents(ctx context.Context, spell wotlk.Spell) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for i := range spell.Reagent {
		if spell.Reagent[i] <= 0 {
			continue
		}
		itemID := spell.Reagent[i]
		need := int64(spell.ReagentCount[i])
		if need == 0 {
			need = 1
		}
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.item, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? ORDER BY ii.count DESC`, s.playerGUID, int64(itemID))
		if err != nil {
			continue
		}
		type stack struct {
			guid  int64
			count int64
		}
		var stacks []stack
		for rows.Next() {
			var st stack
			if rows.Scan(&st.guid, &st.count) == nil {
				stacks = append(stacks, st)
			}
		}
		rows.Close()
		for _, st := range stacks {
			if need <= 0 {
				break
			}
			if st.count <= need {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `DELETE FROM character_inventory WHERE guid = ? AND item = ?`, s.playerGUID, st.guid)
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `DELETE FROM item_instance WHERE guid = ?`, st.guid)
				need -= st.count
			} else {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `UPDATE item_instance SET count = count - ? WHERE guid = ?`, need, st.guid)
				need = 0
			}
		}
	}
}

func (s *session) handleCastSpell(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.server.Data == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	castID, err := reader.ReadU8()
	if err != nil {
		return false
	}
	spellID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if s.isDeadOrGhost() {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterDead), true)
		return true
	}
	if s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 || s.hasAuraType(spellAuraCharm) {
		failure := spellFailedCharmed
		if s.player.UnitFlags&unitFlagConfused != 0 {
			failure = spellFailedConfused
		} else if s.player.UnitFlags&unitFlagFleeing != 0 {
			failure = spellFailedFleeing
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		return true
	}
	clientCastFlags, err := reader.ReadU8()
	if err != nil {
		return false
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil {
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "malformed targets", "error", err)
		return false
	}
	if clientCastFlags&0x02 != 0 {
		// SpellCastTargets m_elevation / m_speed (HandleClientCastFlags,
		// SpellHandler.cpp:30): projectile data consumed by
		// SelectImplicitTrajTargets (Spell.cpp:1626). The optional
		// embedded movement block C++ reads next has no Go consumer —
		// movement arrives via the standalone movement opcodes.
		if target.TrajElevation, err = reader.ReadF32(); err != nil {
			return false
		}
		if target.TrajSpeed, err = reader.ReadF32(); err != nil {
			return false
		}
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil {
		s.debug("spell lookup failed", "account", s.accountName, "spell", spellID, "error", err)
		return true
	}
	learned := s.hasActiveSpell(spellID)
	gmMode := s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0
	if !found || spell.Attributes&spellAttributePassive != 0 || !canPlayerCastSpell(learned, gmMode) {
		s.debug("spell cast ignored", "account", s.accountName, "spell", spellID, "reason", spellCastIgnoreReason(spell, found, learned))
		return true
	}
	// Spell::prepare server-side gate (Spell.cpp:3082-3087) via
	// Unit::IsNonMeleeSpellCast(false, true, true, isAutoshoot)
	// (Unit.cpp:3182-3210): only a cast-bar cast blocks a new client-initiated
	// cast; channeled and autorepeat casts never trigger
	// SPELL_FAILED_SPELL_IN_PROGRESS — the new cast breaks them instead
	// (Unit::SetCurrentCastSpell, Unit.cpp:3064).
	if s.genericCastInProgress() && !s.autoShotNonBlockingCast(spellID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 105), true) // SPELL_FAILED_SPELL_IN_PROGRESS = 105
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "another spell cast is in progress")
		return true
	}
	nowUnix := time.Now().Unix()
	if s.isSchoolLocked(spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "school lockout active")
		return true
	}
	if s.hasAuraType(18) && (spell.SchoolMask > 1 || spell.SchoolMask == 0) && spell.PreventionType != spellPreventionTypePacify {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedSilenced), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "silenced")
		return true
	}
	// Shapeshift/stance requirements (SpellInfo::CheckShapeshift, SpellInfo.cpp:1455;
	// gated in Spell::CheckCast at Spell.cpp:5247-5275, before the caster-state block):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	// The GetTalentSpellCost talent-learn exception has no Go equivalent (noted gap).
	if !s.hasIgnoreShapeshiftAura(spell) {
		if shapeResult := s.checkShapeshiftCast(spell); shapeResult != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, shapeResult), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "shapeshift requirement not met", "result", shapeResult)
			return true
		}
	}
	// Caster aura spell requirements (Spell::CheckCast caster-state block, Spell.cpp:5305-5308):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.CasterAuraSpell != 0 && !s.hasAura(spell.CasterAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required caster aura missing", "aura", spell.CasterAuraSpell)
		return true
	}
	if spell.ExcludeCasterAuraSpell != 0 && s.hasAura(spell.ExcludeCasterAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded caster aura present", "aura", spell.ExcludeCasterAuraSpell)
		return true
	}
	// Caster aura state requirements (Spell::CheckCast caster-state block, Spell.cpp:5298-5304):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.CasterAuraState != 0 && !s.hasAuraState(spell.CasterAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required caster aura state missing", "state", spell.CasterAuraState)
		return true
	}
	if spell.ExcludeCasterAuraState != 0 && s.hasAuraState(spell.ExcludeCasterAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded caster aura state present", "state", spell.ExcludeCasterAuraState)
		return true
	}
	if spell.RequiresSpellFocus != 0 && !s.spellFocusFound(ctx, spell) {
		s.sendCastFailed(ctx, castID, spell, spellFailedRequiresSpellFocus)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "no spell focus object in range", "focus", spell.RequiresSpellFocus)
		return true
	}
	if s.isGCDActive(spell) {
		// Spell::CheckCast (Spell.cpp:5227-5228): DISABLED_WHILE_ACTIVE spells
		// report DONT_REPORT instead of NOT_READY on GCD.
		reason := spellFailedNotReady
		if spell.Attributes&spellAttr0DisabledWhileActive != 0 {
			reason = spellFailedDontReport
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, reason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "global cooldown active")
		return true
	}
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID && cd.End > nowUnix {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "on cooldown")
			return true
		}
	}
	if categoryID := spell.Category; categoryID != 0 {
		for _, cooldown := range s.player.Cooldowns {
			if cooldown.Category == categoryID && cooldown.End > nowUnix && cooldown.CategoryEnd > nowUnix {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "category cooldown active", "category", categoryID)
				return true
			}
		}
	}
	// Self-cast only spells (e.g. Demon Skin, Demon Armor, Ice Barrier) must always target the caster
	if isSelfCastOnly(spell) {
		if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 && target.UnitGUID != s.playerGUID {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "self-cast only spell cannot target other units")
			return true
		}
		target.UnitGUID = s.playerGUID
		target.Flags = protocol.SpellTargetFlagUnit
	}
	if isFishingSpell(spellID) && target.Flags&protocol.SpellTargetFlagDestLocation == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotFishable), true)
		return true
	}
	for _, effect := range spell.Effects {
		if effect.Effect != 101 {
			continue
		}
		if _, failure := s.checkPetFood(ctx, target.ItemGUID); failure != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "pet food validation", "failure", failure)
			return true
		}
	}
	// Learn-spell pet gates (Spell::CheckCast per-effect block,
	// Spell.cpp:5570-5618): LEARN_SPELL on TargetA == TARGET_UNIT_PET and
	// LEARN_PET_SPELL require the caster's pet and reject when the learn
	// spell's own SpellLevel exceeds the pet's level.
	if failure := s.checkLearnSpellCast(ctx, spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "learn spell pet validation", "failure", failure)
		return true
	}
	cost := s.calculateSpellPowerCost(spell)
	pType := spell.PowerType
	// Spell::CheckPower (Spell.cpp:6665-6670) checks rune costs when
	// PowerType == POWER_RUNE; RuneCostID (Spell.dbc field 226) was loaded
	// in store.go but never read in world/ — the earlier "wired" claim was
	// an overclaim. Genuine logic now: Spell::CheckRuneCost parity.
	if spell.PowerType == 5 && !s.checkRuneCost(spell, time.Now().UnixMilli()) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "runes on cooldown")
		return true
	}
	if cost > 0 && pType < 7 && s.player.Powers[pType] < cost {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not enough power", "power", s.player.Powers[pType], "cost", cost)
		return true
	}
	if !s.hasSpellReagents(ctx, spell) {
		s.sendCastFailed(ctx, castID, spell, 100) // SPELL_FAILED_REAGENTS = 100
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "missing reagents")
		return true
	}

	// Totem item/category requirements (Spell::CheckCast, Spell.cpp:6823-6856):
	// run right after the reagent check, matching C++ CheckCast relative order.
	// Client-initiated casts only — triggered casts go through castSpellDirect.
	if failReason := s.checkSpellTotemRequirements(ctx, spell); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "totem requirements not met", "failReason", failReason)
		return true
	}

	if failReason, ok := s.checkSpellEquippedItemRequirements(ctx, spell); !ok {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "equipped item requirements not met", "failReason", failReason)
		return true
	}

	isAutoRepeat := (spell.AttributesEx1&0x20 != 0) || spellID == 75 || spellID == 5019
	targetGUID := uint64(0)
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		targetGUID = target.UnitGUID
	} else if s.selection != 0 {
		targetGUID = s.selection
	}

	// Target creature-type gate (SpellInfo::CheckTarget, SpellInfo.cpp:1728):
	// sits ahead of the aura-state/aura-spell gates in C++ CheckTarget order.
	// Only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if failReason := s.checkTargetCreatureType(ctx, spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target creature type mismatch", "failReason", failReason)
		return true
	}

	// Target aura spell requirements (SpellInfo::CheckTarget, SpellInfo.cpp:1769-1772):
	// only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.TargetAuraSpell != 0 && targetGUID != 0 && !s.targetHasAura(ctx, targetGUID, spell.TargetAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required target aura missing", "aura", spell.TargetAuraSpell)
		return true
	}
	if spell.ExcludeTargetAuraSpell != 0 && targetGUID != 0 && s.targetHasAura(ctx, targetGUID, spell.ExcludeTargetAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded target aura present", "aura", spell.ExcludeTargetAuraSpell)
		return true
	}
	// Target aura state requirements (SpellInfo::CheckTarget, SpellInfo.cpp:1760-1766):
	// only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// C++ skips these for vehicle casters and charmer-owned targets; Go has
	// neither concept, so the check always applies here.
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.TargetAuraState != 0 && targetGUID != 0 && !s.targetHasAuraState(ctx, targetGUID, spell.TargetAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required target aura state missing", "state", spell.TargetAuraState)
		return true
	}
	if spell.ExcludeTargetAuraState != 0 && targetGUID != 0 && s.targetHasAuraState(ctx, targetGUID, spell.ExcludeTargetAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded target aura state present", "state", spell.ExcludeTargetAuraState)
		return true
	}

	// Flying-target gate (SpellInfo::CheckTarget, SpellInfo.cpp:1745-1747):
	// an explicit unit target in flight rejects the cast with
	// SPELL_FAILED_BAD_TARGETS unless the spell carries
	// SPELL_ATTR0_CU_ALLOW_INFLIGHT_TARGET. Only checked when a unit target
	// exists, like the sibling gates above.
	if s.explicitTargetFlyingBlocked(spell, targetGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target in flight", "target", targetGUID)
		return true
	}

	// Auto-repeat toggle: if already repeating this spell on this target, toggle it off (TC SpellHandler.cpp:420-430)
	if isAutoRepeat && s.autoRepeatSpell == spellID && s.autoRepeatTarget == targetGUID {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		return true
	}

	// Range and Ammo checks for ranged / auto-repeat spells (TC Spell::CheckCast)
	if targetGUID != 0 && targetGUID != s.playerGUID {
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			pReach := float32(1.5)
			if s.player.CombatReach > 0 {
				pReach = s.player.CombatReach
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z)
			if spellID == 75 { // Auto Shot (TC: Range 114, SPELL_RANGE_RANGED)
				minRange := calcMeleeRange(pReach, tgt.CombatReach)
				if dist < minRange {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 128), true) // SPELL_FAILED_TOO_CLOSE = 128
					return true
				}
				if dist > 35.0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					return true
				}
				if s.player.AmmoID == 0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 75), true) // SPELL_FAILED_NO_AMMO = 75
					return true
				}
			} else if spellID == 5019 { // Shoot wand (Range 4)
				if dist > 30.0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					return true
				}
			} else if rangeEntry, ok, _ := s.server.Data.SpellRange(spell.RangeIndex); ok {
				// DBC-driven range check (TC Spell::CheckRange)
				harmful := isHarmfulSpell(spell)
				maxRange := rangeEntry.MaxFriendly
				minRange := rangeEntry.MinFriendly
				if harmful {
					maxRange = rangeEntry.MaxHostile
					minRange = rangeEntry.MinHostile
				}
				if maxRange > 0 && dist > float64(maxRange) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "out of range", "dist", dist, "max", maxRange)
					return true
				}
				if minRange > 0 && dist < float64(minRange) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 128), true) // SPELL_FAILED_TOO_CLOSE = 128
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "too close", "dist", dist, "min", minRange)
					return true
				}
			}

			// Positional and facing checks (TrinityCore Spell::CheckCast, Spell.cpp:5200-5300)
			customAttr := s.server.getSpellCustomAttr(spellID)
			// 1. Behind target requirement (SPELL_ATTR0_CU_REQ_CASTER_BEHIND_TARGET = 0x20000):
			// If target has caster in frontal 180° arc, caster is NOT behind target!
			// Returns SPELL_FAILED_NOT_BEHIND = 57 ("You must be behind your target.").
			if customAttr&SpellCustomAttrReqCasterBehindTarget != 0 {
				if hasInArc(tgt.Orientation, tgt.X, tgt.Y, s.player.X, s.player.Y, math.Pi) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 57), true) // SPELL_FAILED_NOT_BEHIND = 57
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not behind target")
					return true
				}
			}

			// 2. Target facing caster requirement (SPELL_ATTR0_CU_REQ_TARGET_FACING_CASTER = 0x10000):
			// Target must have caster in its frontal 180° arc (e.g. Gouge).
			// Returns SPELL_FAILED_NOT_INFRONT = 61 ("You must be in front of your target.").
			if customAttr&SpellCustomAttrReqTargetFacingCaster != 0 {
				if !hasInArc(tgt.Orientation, tgt.X, tgt.Y, s.player.X, s.player.Y, math.Pi) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotInFront), true)
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target not facing caster")
					return true
				}
			}

			// 3. Caster facing target requirement:
			// If spell has SPELL_FACING_FLAG_INFRONT (0x1) from Spell.dbc field 19 (or Auto Shot / Shoot Wand):
			// Target must be within caster's 120° frontal cone (2*pi/3).
			// Returns SPELL_FAILED_UNIT_NOT_INFRONT = 134 ("Target needs to be in front of you.").
			if (spell.FacingCasterFlags&SpellFacingFlagInfront != 0) || spellID == 75 || spellID == 5019 {
				if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, tgt.X, tgt.Y, 2.0*math.Pi/3.0) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedUnitNotInFront), true)
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target not in front")
					return true
				}
			}
		}
	}

	// Dispel check: if spell has only SPELL_EFFECT_DISPEL effects (and not area-targeting), verify target has dispellable auras
	// Mirrors TrinityCore Spell::CheckCast (Spell.cpp:5520-5565)
	if failReason := s.checkDispelPreCast(spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "nothing to dispel")
		return true
	}

	// Spellsteal check: SPELL_EFFECT_STEAL_BENEFICIAL_BUFF (126) fails at cast
	// time when the target carries no stealable aura (Spell::CheckCast,
	// Spell.cpp:5957-5984).
	if failReason := s.checkStealPreCast(spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "nothing to steal")
		return true
	}

	// Unit::SetCurrentCastSpell (Unit.cpp:3064-3090): registering the new cast
	// breaks the other containers. A generic cast breaks the active channel
	// ("generic spells always break channeled not delayed spells") and any
	// autorepeat that is not Auto Shot — wand Shoot breaks on a new cast
	// while Auto Shot persists through casts, matching in-game behavior. A
	// channeled cast breaks the channel it replaces plus non-Auto-Shot
	// autorepeat; a new wand Shoot (5019) breaks the channel. A new Auto Shot
	// (75) breaks nothing ("only Auto Shoot does not break anything"). Go has
	// no DELAYED channel state, so the C++ withDelayed=false terms are
	// vacuous; C++ sends no packet for these server-side breaks. The gate
	// above already rejected while a cast-bar cast runs, so
	// interruptCurrentCast is the same-container break (SetCurrentCastSpell's
	// InterruptSpell(CSpellType, false)) and a no-op except in the Auto-Shot
	// exception path, which is skipped here.
	if spellID != 75 {
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
	}
	if s.autoRepeatSpell != 0 && s.autoRepeatSpell != 75 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
	}
	s.procCastAuras()

	s.lastCastTime = time.Now()
	castTime := s.calculateSpellCastTime(spell)
	// Spell::prepare (Spell.cpp:3139-3149): channeled spells and spells with
	// cast time cannot start while moving, unless the channel allows movement.
	if (isChanneledSpell(spell) || castTime > 0) && s.isMoving && spell.InterruptFlags&spellInterruptFlagMovement != 0 {
		if castTime > 0 || spell.AttributesEx5&spellAttr5CanChannelWhenMoving == 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedMoving), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "casting while moving")
			return true
		}
	}
	// Spell::prepare (Spell.cpp:3175-3189): stealth breaks at cast start.
	if spell.AttributesEx&spellAttr1NotBreakStealth == 0 {
		s.removeAurasWithInterruptFlags(auraInterruptFlagCast)
		for _, eff := range spell.Effects {
			if spellEffectTargetsUnit(eff.Effect) {
				s.removeAurasWithInterruptFlags(auraInterruptFlagSpellAttack)
				break
			}
		}
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_START), protocol.BuildSpellStart(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagStart, castTime, target), true); err != nil {
		return false
	}

	// Spell::prepare (Spell.cpp:3188-3193) sends SMSG_SPELL_START before
	// TriggerGlobalCooldown.
	s.triggerGlobalCooldown(spell)

	if castTime > 0 {
		s.castMu.Lock()
		castState := &activeCastState{
			CastID:       castID,
			SpellID:      spellID,
			StartAt:      time.Now(),
			CastTimeMs:   castTime,
			InterruptFlg: spell.InterruptFlags,
		}
		castState.Timer = time.AfterFunc(time.Duration(castTime)*time.Millisecond, func() {
			s.castMu.Lock()
			if castState.Cancelled {
				s.castMu.Unlock()
				return
			}
			s.castMu.Unlock()
			s.finishSpellCast(context.Background(), castID, spellID, spell, target, 0)
		})
		s.activeCast = castState
		s.castMu.Unlock()
	} else {
		s.finishSpellCast(context.Background(), castID, spellID, spell, target, 0)
	}

	s.debug("spell cast accepted", "account", s.accountName, "spell", spellID, "cast_id", castID, "cast_time", castTime, "cost", cost)
	return true
}

// genericCastInProgress mirrors the Spell::prepare server-side gate
// (Spell.cpp:3082-3087) via Unit::IsNonMeleeSpellCast(false, true, true,
// isAutoshoot) (Unit.cpp:3182-3210): only a non-finished cast-bar
// (CURRENT_GENERIC_SPELL) cast blocks a new client-initiated cast.
// Channeled casts (skipChanneled=true) and autorepeat casts
// (skipAutorepeat=true) never trigger SPELL_FAILED_SPELL_IN_PROGRESS — the
// new cast breaks them on registration instead (Unit::SetCurrentCastSpell,
// Unit.cpp:3064). Go arms activeCast only for castTime > 0, so instant casts
// can't be in progress here, matching C++'s skipInstant outcome by
// construction; Go has no DELAYED (missile in flight) state, so that term is
// vacuous. Triggered casts never reach this gate (castSpellDirect), matching
// TRIGGERED_IGNORE_CAST_IN_PROGRESS; C++'s m_cast_count term is exactly the
// client-initiated indicator (SpellHandler.cpp:445).
func (s *session) genericCastInProgress() bool {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.activeCast != nil && !s.activeCast.Cancelled
}

// autoShotNonBlockingCast is the C++ isAutoshoot exception in
// Unit::IsNonMeleeSpellCast (Unit.cpp:3192-3195): a new Auto Shot (75) cast
// is not blocked by an in-progress cast-bar cast carrying
// SPELL_ATTR2_NOT_RESET_AUTO_ACTIONS, and on registration it breaks nothing
// ("only Auto Shoot does not break anything", Unit::SetCurrentCastSpell,
// Unit.cpp:3112-3125).
func (s *session) autoShotNonBlockingCast(spellID uint32) bool {
	if spellID != 75 {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if s.activeCast == nil || s.activeCast.Cancelled {
		return false
	}
	cur, found, _ := s.server.Data.Spell(s.activeCast.SpellID)
	return found && cur.AttributesEx1&spellAttr2NotResetAutoActions != 0
}

// validateSpellRange checks DBC range (plus the Auto Shot / Shoot special
// cases) against current caster/target positions. Returns 0 on success or a
// SPELL_FAILED_* code. C++ authority: Spell::CheckRange via Spell::CheckCast.
func (s *session) validateSpellRange(ctx context.Context, spellID uint32, spell wotlk.Spell, targetGUID uint64) uint8 {
	tgt, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		return 0
	}
	pReach := float32(1.5)
	if s.player.CombatReach > 0 {
		pReach = s.player.CombatReach
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z)
	if spellID == 75 { // Auto Shot
		if dist < calcMeleeRange(pReach, tgt.CombatReach) {
			return 128 // SPELL_FAILED_TOO_CLOSE
		}
		if dist > 35.0 {
			return 97 // SPELL_FAILED_OUT_OF_RANGE
		}
		return 0
	}
	if spellID == 5019 { // Shoot
		if dist > 30.0 {
			return 97 // SPELL_FAILED_OUT_OF_RANGE
		}
		return 0
	}
	rangeEntry, ok, _ := s.server.Data.SpellRange(spell.RangeIndex)
	if !ok {
		return 0
	}
	harmful := isHarmfulSpell(spell)
	maxRange := rangeEntry.MaxFriendly
	minRange := rangeEntry.MinFriendly
	if harmful {
		maxRange = rangeEntry.MaxHostile
		minRange = rangeEntry.MinHostile
	}
	if maxRange > 0 && dist > float64(maxRange) {
		return 97 // SPELL_FAILED_OUT_OF_RANGE
	}
	if minRange > 0 && dist < float64(minRange) {
		return 128 // SPELL_FAILED_TOO_CLOSE
	}
	return 0
}

// checkLearnSpellCast mirrors the per-effect pet gates in Spell::CheckCast
// (Spell.cpp:5570-5618): SPELL_EFFECT_LEARN_SPELL (36) with TargetA ==
// TARGET_UNIT_PET (5) requires the caster's active pet and rejects with
// SPELL_FAILED_LOWLEVEL (48) when the learn spell's own SpellLevel exceeds
// the pet's level; SPELL_EFFECT_LEARN_PET_SPELL (57) requires the unit target
// to be the caster's own pet when a unit target is present. The
// caster-must-be-player terms are vacuous here — client casts always come from
// a player session. Returns the SPELL_FAILED_* result code, 0 on success.
func (s *session) checkLearnSpellCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, effect := range spell.Effects {
		switch effect.Effect {
		case spellEffectLearnSpell:
			if effect.ImplicitTargetA != spellImplicitTargetUnitPet {
				continue
			}
		case spellEffectLearnPetSpell:
			if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
				continue // C++ only gates when a unit target is present (Spell.cpp:5599)
			}
			if s.petNumberForGUID(target.UnitGUID) == 0 {
				return spellFailedBadTargets
			}
		default:
			continue
		}
		petID := s.activePetNumber()
		if petID == 0 {
			return spellFailedNoPet
		}
		if _, found, err := s.server.Data.Spell(effect.TriggerSpell); err != nil || !found {
			return spellFailedNotKnown
		}
		if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return 0
		}
		var petLevel int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT level FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petID).Scan(&petLevel); err != nil {
			return 0 // data anomaly: active pet without a row — no gate instead of a false reject
		}
		if petLevel < 0 {
			petLevel = 0
		}
		if spell.SpellLevel > uint32(petLevel) {
			return spellFailedLowLevel
		}
	}
	return 0
}

func (s *session) finishSpellCast(ctx context.Context, castID uint8, spellID uint32, spell wotlk.Spell, target protocol.SpellTargetData, castItemGUID uint64) {
	if s.player == nil {
		return
	}
	var completedCast *activeCastState
	s.castMu.Lock()
	if s.activeCast != nil && s.activeCast.CastID == castID && s.activeCast.SpellID == spellID {
		if s.activeCast.Cancelled {
			s.castMu.Unlock()
			return
		}
		completedCast = s.activeCast
	}
	s.castMu.Unlock()
	if completedCast != nil {
		defer func() {
			s.castMu.Lock()
			if s.activeCast == completedCast {
				s.activeCast = nil
			}
			s.castMu.Unlock()
		}()
	}
	if s.isDeadOrGhost() {
		canResurrect := false
		for _, effect := range spell.Effects {
			if effect.Effect == spellEffectResurrectNew {
				canResurrect = true
				break
			}
		}
		if !canResurrect {
			return
		}
	}

	// Spell::_cast (Spell.cpp:3335) re-runs CheckCast(false) when the cast timer
	// finishes; power drained mid-cast must fail the cast, not clamp to zero.
	pType := spell.PowerType
	cost := s.calculateSpellPowerCost(spell)
	if pType < 7 && cost > 0 && s.player.Powers[pType] < cost {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		return
	}
	// Spell::_cast re-runs CheckCast at completion; runes spent mid-cast
	// must fail the cast too (Spell::CheckPower rune check, Spell.cpp:6665).
	if spell.PowerType == 5 && !s.checkRuneCost(spell, time.Now().UnixMilli()) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		return
	}

	// Spell::_cast revalidates CheckCast at completion: the target may have
	// moved during the cast bar.
	if target.UnitGUID != 0 {
		if failCode := s.validateSpellRange(ctx, spellID, spell, target.UnitGUID); failCode != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failCode), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "range", "code", failCode)
			return
		}
		// CheckCast also revalidates line of sight at completion.
		if tgt, ok := s.getCombatTarget(ctx, target.UnitGUID); ok && s.server != nil {
			if !s.server.hasLineOfSight(s.player.Map, s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 47), true) // SPELL_FAILED_LINE_OF_SIGHT = 47
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "line of sight")
				return
			}
			// Unit targets that died during the cast bar fail with
			// SPELL_FAILED_TARGETS_DEAD (SpellInfo::CheckTarget, SpellInfo.cpp:1715)
			// unless the spell allows dead targets (SpellInfo::IsAllowingDeadTarget,
			// SpellInfo.cpp:1177) or can resurrect.
			if tgt.Health == 0 && target.UnitGUID != s.playerGUID {
				canResurrect := false
				for _, effect := range spell.Effects {
					if effect.Effect == spellEffectResurrectNew {
						canResurrect = true
						break
					}
				}
				if !canResurrect && !spellAllowsDeadTarget(spell) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 109), true)
					s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "target dead")
					return
				}
			}
		}
	}

	// Spell::CheckCast trade-slot gate (Spell.cpp:6167-6171): a spell cast
	// from an item (enchanting vellum via CMSG_USE_ITEM) cannot target the
	// trade window's non-traded slot — SPELL_FAILED_ITEM_ENCHANT_TRADE_WINDOW —
	// and is never deferred into the trade data. The gate runs before the
	// deferral below, matching _cast order (CheckCast(false) precedes the
	// TARGET_FLAG_TRADE_ITEM deferral, Spell.cpp:3335-3372); it is not gated
	// on trade state because C++ checks m_CastItem before the NOT_TRADING
	// terms. Book casts (CMSG_CAST_SPELL) always pass 0 here (Spell.cpp:584:
	// m_CastItem is set only by CastItemUseSpell), so only item casts trip it.
	if target.Flags&protocol.SpellTargetFlagTradeItem != 0 && castItemGUID != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedItemEnchantTradeWindow), true)
		s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "item enchant trade window")
		return
	}

	// Spell::_cast (Spell.cpp:3357-3372): a cast-bar-completed spell targeting
	// the trade window's non-traded slot is deferred into the trade data while
	// the trade is not in its accept process — the spell fires when the trade
	// executes (TradeHandler.cpp:364-438). m_CastItem is null for CMSG_CAST_SPELL
	// book casts (Spell.cpp:584, set only by CastItemUseSpell / UpdatePointers),
	// so the stored cast-item GUID is 0. The deferred activeCast cleanup at the
	// top of this function mirrors cleanupSpell(SPELL_FAILED_DONT_REPORT)'s
	// silent drop; nothing is sent to the client here.
	if target.Flags&protocol.SpellTargetFlagTradeItem != 0 && s.trade != nil && !s.trade.InAcceptProcess {
		s.setTradeSpell(spellID, 0)
		s.debug("spell cast deferred to trade", "account", s.accountName, "spell", spellID)
		return
	}

	// Spell::SelectImplicitTargetDestTargets (Spell.cpp:1433) and
	// Spell::SelectImplicitDestDestTargets (Spell.cpp:1464): resolve the
	// spell destination from target-relative / dest-relative implicit
	// targets once, before the area selection and persistent-area read
	// sites below consume it.
	target, destOK := s.resolveImplicitSpellDestination(ctx, spell, spellID, target)
	if !destOK {
		// Spell.cpp:1111: no nearby entry object found ->
		// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
		s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby entry object")
		return
	}

	hitTargets := make([]uint64, 0, 1)
	// Spell::CheckCast routes the explicit unit target through
	// SpellInfo::CheckExplicitTarget (Spell.cpp:5365, SpellInfo.cpp:1799),
	// which applies the Unit::IsValidAttackTarget/IsValidAssistTarget flag
	// gates (Object.cpp:2972/3127/2991/3134, bundled in spellTargetUnitBlocked;
	// the NON_ATTACKABLE/TRIGGER/NO_COMBAT bundle always rejects on the
	// attack path but only for negative spells on the assist path,
	// Object.cpp:2980/3131)
	// and the hostility/faction gates (Object.cpp "can't attack friendly
	// targets" / "can't assist non-friendly targets", plus the PARTY/RAID
	// membership terms, via explicitTargetFactionBlocked), failing the cast
	// with SPELL_FAILED_BAD_TARGETS (SharedDefines.h:992). The
	// SpellInfo::CheckTarget GM/invisibility gate (SpellInfo.cpp:1736-1743,
	// explicitTargetGMBlocked) fails the cast with
	// SPELL_FAILED_BM_OR_INVISGOD (SharedDefines.h:1141).
	// Self is exempt: IsValidAssistTarget returns true for self (Object.cpp:3092).
	explicitUnitGUID := uint64(0)
	if isSelfCastOnly(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		explicitUnitGUID = target.UnitGUID
	} else if s.selection != 0 {
		explicitUnitGUID = s.selection
	} else if !isHarmfulSpell(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	}
	if explicitUnitGUID != 0 && explicitUnitGUID != s.playerGUID {
		if tgt, ok := s.getCombatTarget(ctx, explicitUnitGUID); ok {
			// SpellInfo.cpp:1799-1816: the ENEMY explicit mask runs the
			// Unit::IsValidAttackTarget flag gates (bundle always rejects,
			// Object.cpp:2980) and the ALLY/PARTY/RAID masks run the
			// WorldObject::IsValidAssistTarget gates (bundle only for
			// negative spells, Object.cpp:3131).
			explicitMask := spellExplicitUnitTargetMask(spell)
			assist := explicitMask&(targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid) != 0
			if spellTargetUnitBlocked(spell, tgt.UnitFlags, tgt.FlagsExtra, assist) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target blocked")
				return
			}
			// SpellInfo.cpp:1799-1816: the ENEMY/ALLY/PARTY/RAID explicit
			// masks carry the hostility/faction gates.
			if s.explicitTargetFactionBlocked(explicitMask, explicitUnitGUID, tgt) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target faction mismatch")
				return
			}
			// SpellInfo.cpp:1736-1743: GM-invisible or GM-mode player targets
			// reject the cast with SPELL_FAILED_BM_OR_INVISGOD; self is
			// exempt (this block only runs when explicitUnitGUID != s.playerGUID).
			if s.explicitTargetGMBlocked(explicitUnitGUID) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBmOrInvisGod), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target GM/invisible")
				return
			}
		}
		hitTargets = append(hitTargets, explicitUnitGUID)
	}
	areaSpell := isAreaEnemySpell(spell)
	friendlyAreaSpell := isFriendlyAreaSpell(spell)
	friendlyNearbySpell := isFriendlyNearbySpell(spell)
	entryNearbySpell := isEntryNearbySpell(spell)
	goNearbyEntrySpell := isGONearbyEntrySpell(spell)
	entryAreaSpell := isEntryAreaSpell(spell)
	goAreaSpell := isGOAreaSpell(spell)
	goConeSpell := isGOConeSpell(spell)
	friendlyConeSpell := isFriendlyConeSpell(spell)
	friendlyLastTargetAreaSpell := isFriendlyLastTargetAreaSpell(spell)
	friendlyTargetAreaRaidClassSpell := isFriendlyTargetAreaRaidClassSpell(spell)
	// List-producing friendly/entry selections skip the single-target immune gate
	// and chain-jump expansion the same way area spells do (Spell.cpp:1227
	// area selection never calls SelectImplicitChainTargets).
	friendlyListSpell := friendlyAreaSpell || friendlyConeSpell || friendlyLastTargetAreaSpell || friendlyTargetAreaRaidClassSpell || entryAreaSpell || goAreaSpell || goConeSpell
	if areaSpell {
		hitTargets = s.spellAreaEnemyTargets(ctx, spell, target)
	} else if friendlyAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227): friendly
		// PARTY/ALLY/RAID area targets (20/30/31/33/34/56) replace the
		// hit-target list the same way enemy area targets do.
		hitTargets = s.spellFriendlyAreaTargets(ctx, spell, target)
	} else if friendlyNearbySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036): the single
		// nearest PARTY/ALLY/RAID unit (3/4/58) becomes the target.
		if nearby, ok := s.spellFriendlyNearbyTarget(ctx, spell, target); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby target")
			return
		}
	} else if entryNearbySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
		// TARGET_UNIT_NEARBY_ENTRY (38) — the single nearest entry-matched
		// unit becomes the target; no match fails the cast.
		if nearby, ok := s.spellEntryNearbyTarget(ctx, spell, spellID); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no entry target")
			return
		}
	} else if entryAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227):
		// TARGET_UNIT_SRC_AREA_ENTRY (7) / TARGET_UNIT_DEST_AREA_ENTRY (8)
		// — every unit in the area matching the entry conditions becomes a
		// target; an empty area never fails the cast.
		hitTargets = s.spellEntryAreaTargets(ctx, spell, spellID, target)
	} else if goNearbyEntrySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
		// TARGET_GAMEOBJECT_NEARBY_ENTRY (40) — the single nearest
		// entry-matched gameobject becomes the target; no match fails the
		// cast. The GO guid flows through hitTargets like C++'s AddGOTarget
		// list; Go has no gameobject-effect consumer downstream.
		if nearby, ok := s.spellEntryNearbyGOTarget(ctx, spell, spellID); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby gameobject target")
			return
		}
	} else if friendlyConeSpell {
		// Spell::SelectImplicitConeTargets (Spell.cpp:1176): friendly
		// ALLY/ENTRY cone targets (59/60).
		hitTargets = s.spellFriendlyConeTargets(ctx, spell, target)
	} else if goAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227):
		// TARGET_GAMEOBJECT_SRC_AREA (51) / TARGET_GAMEOBJECT_DEST_AREA
		// (52) — every gameobject in the area becomes a target; an empty
		// area never fails the cast. The GO guids flow through hitTargets
		// like C++'s AddGOTarget list; Go has no gameobject-effect
		// consumer downstream.
		hitTargets = s.spellGOAreaTargets(ctx, spell, spellID, target)
	} else if goConeSpell {
		// Spell::SelectImplicitConeTargets (Spell.cpp:1176):
		// TARGET_GAMEOBJECT_CONE (108) — every gameobject in the caster's
		// front cone becomes a target; an empty cone never fails the cast.
		hitTargets = s.spellGOConeTargets(ctx, spell, spellID)
	} else if friendlyLastTargetAreaSpell || friendlyTargetAreaRaidClassSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227) with LAST (37)
		// or TARGET (61) reference: area around the last/explicit target.
		hitTargets = s.spellFriendlyRefCenteredAreaTargets(ctx, spell, target, hitTargets)
	}
	s.spawnPersistentAreaAura(ctx, spell, target)
	targetGUID := uint64(0)
	if len(hitTargets) > 0 {
		targetGUID = hitTargets[0]
	}

	// Auto-repeat ranged spells (e.g. Auto Shot, Shoot Wand) (TC: CURRENT_AUTOREPEAT_SPELL)
	if (spell.AttributesEx1&0x20 != 0) || spellID == 75 || spellID == 5019 {
		s.autoRepeatSpell = spellID
		s.autoRepeatTarget = targetGUID
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			s.executeRangedAttack(ctx, tgt, spellID)
		}
		return
	}

	// Spell hit check for offensive spells targeting another unit
	var missStatus []protocol.SpellMissStatus
	isReflected := false
	if !areaSpell && targetGUID != 0 && targetGUID != s.playerGUID && isHarmfulSpell(spell) {
		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		if targetSess != nil && targetSess.checkSpellReflection(spell) {
			isReflected = true
			missStatus = []protocol.SpellMissStatus{{
				TargetGUID:    targetGUID,
				Reason:        protocol.SpellMissReflect,
				ReflectStatus: 2,
			}}
			targetGUID = s.playerGUID
			hitTargets = []uint64{s.playerGUID}
		} else if targetSess != nil && targetSess.isImmuneToSpell(spell) {
			hitTargets = nil
			missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissImmune}}
		} else {
			targetLevel := uint8(1)
			if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
				targetLevel = tgt.Level
			}
			isPlayerVictim := targetSess != nil
			bonusHit := 0.0
			if s.player != nil {
				bonusHit = s.getSpellHitPct()
			}
			missInfo := magicSpellHitResult(s.player.Level, targetLevel, isPlayerVictim, bonusHit)
			if missInfo != protocol.SpellMissNone {
				hitTargets = nil
				missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: missInfo}}
			} else if isBinarySpell(spell) {
				var resistances [7]uint32
				if targetSess != nil && targetSess.player != nil {
					resistances = targetSess.player.Resistances
				} else if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
					resistances = tgt.Resistances
				}
				pen := uint32(0)
				if s.player != nil {
					pen = s.player.SpellPenetration
				}
				chaosBolt := spell.SpellFamilyName == spellFamilyWarlock && spell.SpellIconID == 3178
				if checkBinarySpellResist(resistances, uint8(spell.SchoolMask), pen, s.player.Level, targetLevel, chaosBolt) {
					hitTargets = nil
					missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissResist}}
				}
			}
		}
	} else if !areaSpell && !friendlyListSpell && targetGUID != 0 && targetGUID != s.playerGUID && !isHarmfulSpell(spell) {
		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		if targetSess != nil && targetSess.isImmuneToSpell(spell) {
			hitTargets = nil
			missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissImmune}}
		}
	}

	// Spell::SelectImplicitChainTargets (Spell.cpp:1582): chain spells
	// (EffectChainTarget > 1) jump from the primary target to nearby units;
	// chainJumpIndex records each target's jump order (0 = primary) so the
	// per-jump EffectChainAmplitude falloff can be applied at effect time.
	chainJumpIndex := make(map[uint64]int)
	if !areaSpell && !friendlyListSpell && targetGUID != 0 {
		if jumps, isChainHeal := chainSpellJumps(spell); jumps > 0 {
			for i, extraGUID := range s.spellSearchChainTargets(ctx, spell, targetGUID, jumps, isChainHeal) {
				hitTargets = append(hitTargets, extraGUID)
				chainJumpIndex[extraGUID] = i + 1
			}
		}
	}

	castTimeStamp := uint32(time.Now().UnixMilli())
	castFlags := spellCastFlagGo
	var remainingPower *uint32
	if pType < 7 {
		castFlags |= protocol.SpellCastFlagPowerLeftSelf
		power := s.player.Powers[pType]
		remainingPower = &power
	}
	if pType < 7 && cost > 0 {
		// Re-validation above guarantees sufficient power; C++ TakePower deducts.
		s.player.Powers[pType] -= cost
		powerPacket := protocol.NewBuffer(13)
		powerPacket.WritePackedGUID(s.playerGUID)
		powerPacket.WriteU8(uint8(pType))
		powerPacket.WriteU32(s.player.Powers[pType])
		_ = s.write(uint16(protocol.OpcodeSMSG_POWER_UPDATE), powerPacket.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_POWER_UPDATE), powerPacket.Bytes(), s)
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			col := fmt.Sprintf("power%d", pType+1)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, fmt.Sprintf("UPDATE characters SET %s = ? WHERE guid = ?", col), s.player.Powers[pType], s.playerGUID)
		}
	}
	s.takeSpellReagents(ctx, spell)

	// Spell::TakePower (Spell.cpp:4838-4844) spends runes for POWER_RUNE
	// spells via TakeRunePower. didHit mirrors C++ (false only when the
	// primary target missed; chain jumps appended to hitTargets above
	// must not flip it).
	didHit := true
	for _, miss := range missStatus {
		if miss.TargetGUID == targetGUID {
			didHit = false
			break
		}
	}
	if spell.PowerType == 5 {
		s.takeRunePower(ctx, spell, didHit, time.Now().UnixMilli())
		s.sendRuneCooldownUpdate()
	}

	// Spell::_cast (Spell.cpp:3462-3470) calls SendSpellCooldown() before
	// HandleLaunchPhase() and SendSpellGo(): the cooldown packet must reach
	// the client before SMSG_SPELL_GO.
	categoryID, categoryRecoveryTime := spell.Category, spell.CategoryRecoveryTime
	now := time.Now()
	categoryEnd := now.Unix()
	if categoryRecoveryTime > 0 {
		categoryEnd = now.Add(time.Duration(categoryRecoveryTime) * time.Millisecond).Unix()
	}
	applyCooldown := len(hitTargets) > 0 && !isFishingSpell(spellID)
	if applyCooldown && (spell.RecoveryTime > 0 || categoryRecoveryTime > 0) {
		cooldownEnd := categoryEnd
		if spell.RecoveryTime > 0 {
			cooldownEnd = now.Add(time.Duration(spell.RecoveryTime) * time.Millisecond).Unix()
		}
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, Category: categoryID, End: cooldownEnd, CategoryEnd: categoryEnd})
	}
	if applyCooldown && spell.RecoveryTime > 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, spell.RecoveryTime), true)
	}
	if applyCooldown && spellID == 8690 {
		cooldownEnd := now.Add(15 * time.Minute).Unix() // 15 min cooldown
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, Item: 6948, Category: categoryID, End: cooldownEnd, CategoryEnd: categoryEnd})
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, 900000), true)
	}

	goPacket := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, castID, spellID, castFlags, castTimeStamp, hitTargets, missStatus, target, remainingPower)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, true)
	if s.server != nil {
		nearbyFlags := castFlags &^ protocol.SpellCastFlagPowerLeftSelf
		nearbyPacket := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, nearbyFlags, castTimeStamp, hitTargets, missStatus, target)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
	}
	if isFishingSpell(spellID) {
		s.spawnFishingBobber(ctx, target)
		return
	}

	if len(hitTargets) == 0 {
		// Spell missed, do not trigger channel or effects
		return
	}

	// Spell::_handle_immediate_phase (Spell.cpp:3718): initial spell threat
	// (HandleThreatSpells, Spell.cpp:5096) lands before any effect handling.
	s.handleSpellInitialThreat(ctx, spell, hitTargets)

	// Reference Spell::handle_immediate: channeled spells begin their timed
	// channel lifecycle after the cast completes. The resolved destination is
	// recorded with the channel (C++ channeledSpell->m_targets dest), so
	// spells triggered during the channel can resolve TARGET_DEST_CHANNEL_TARGET.
	if isChanneledSpell(spell) {
		hasDest := target.Flags&protocol.SpellTargetFlagDestLocation != 0
		s.startChannel(castID, spellID, spell, targetGUID, hasDest, target.Destination.X, target.Destination.Y, target.Destination.Z)
	}
	s.updateAchievementCriteria(criteriaTypeCastSpell, spellID, 1)
	s.updateAchievementCriteria(criteriaTypeCastSpell2, spellID, 1)
	s.startTimedAchievement(timedTypeSpellCast, spellID)

	// Reference SpellEffects.cpp:3858-3874: Spell 7266 (Duel)
	if spellID == 7266 && targetGUID != 0 && targetGUID != s.playerGUID && s.server != nil {
		if partner := s.server.findSessionByGUID(targetGUID); partner != nil && partner.player != nil {
			s.duelPartner = targetGUID
			partner.duelPartner = s.playerGUID
			arbiterGUID := uint64(s.playerGUID) | (uint64(0xF110) << 48)
			s.player.DuelArbiter = arbiterGUID
			partner.player.DuelArbiter = arbiterGUID
			s.player.DuelTeam = 0
			partner.player.DuelTeam = 0
			s.sendPlayerUpdate()
			partner.sendPlayerUpdate()

			midX := s.player.X + (partner.player.X-s.player.X)/2
			midY := s.player.Y + (partner.player.Y-s.player.Y)/2
			midZ := s.player.Z
			s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ = midX, midY, midZ
			partner.duelArbiterX, partner.duelArbiterY, partner.duelArbiterZ = midX, midY, midZ
			s.duelOutOfBounds = time.Time{}
			partner.duelOutOfBounds = time.Time{}

			reqBuf := protocol.NewBuffer(16)
			reqBuf.WriteU64(arbiterGUID)
			reqBuf.WriteU64(s.playerGUID)
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_REQUESTED), reqBuf.Bytes(), true)
			_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_REQUESTED), reqBuf.Bytes(), true)
		}
	}

	// Apply spell effects
	applyEffects := func(effCtx context.Context) {
		if len(missStatus) > 0 && !isReflected {
			return
		}
		// Per-(cast, target) first-merge marker for the aura re-apply path
		// (Spell.cpp:2842): only the first aura effect per target runs the
		// ModStackAmount(+1) merge, later effects only refresh their amounts.
		castMerged := make(map[uint64]struct{})
		interruptHandled := false
		damageEffectSeen := false
		for effectIndex, eff := range spell.Effects {
			if eff.Effect == 0 {
				continue
			}
			switch eff.Effect {
			case 1: // SPELL_EFFECT_INSTAKILL
				damageEffectSeen = true
				for _, effectTarget := range hitTargets {
					if effectTarget != 0 {
						s.executeSpellInstantKill(effCtx, effectTarget, spellID)
					}
				}
			case 2, 17, 31, 58, 87: // Damage effects (School damage, Weapon damage, etc.)
				damageEffectSeen = true
				damage := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					if effectTarget != 0 && (effectTarget != s.playerGUID || isReflected) {
						s.executeSpellDamage(effCtx, effectTarget, spellID, chainScaledAmount(damage, eff, chainJumpIndex[effectTarget]), effectIndex)
					}
				}
			case 10, 136, 105: // Heal effects
				heal := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					s.executeSpellHeal(effCtx, effectTarget, spellID, chainScaledAmount(heal, eff, chainJumpIndex[effectTarget]), effectIndex)
				}
			case spellEffectEnergize:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					s.applySpellEnergize(effCtx, effectTarget, eff.MiscValue, amount)
				}
			case spellEffectPowerBurn:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					// SpellEffects.cpp:1383: the drained power is dealt as
					// damage scaled by the effect value multiplier.
					if burned := s.applySpellPowerBurn(effCtx, effectTarget, eff.MiscValue, amount, spellID); burned > 0 {
						s.executeSpellDamage(effCtx, effectTarget, spellID, effectValueMultiplied(burned, eff.Amplitude), effectIndex)
					}
				}
			case spellEffectParry:
				if s.player != nil && !s.player.CanParry {
					s.player.CanParry = true
					s.updatePlayerParryPercentage(s.player, s.player.Level)
					s.sendPlayerUpdate()
				}
			case spellEffectTriggerSpell:
				for _, effectTarget := range hitTargets {
					if eff.TriggerSpell != 0 && eff.TriggerSpell != spellID {
						s.castSpellDirect(effCtx, eff.TriggerSpell, effectTarget)
					}
				}
			case 3:
				s.addOwnerPetAuraSource(effCtx, spellID, uint8(effectIndex))
			case spellEffectThreat:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					s.applySpellThreat(effCtx, effectTarget, amount)
				}
			case spellEffectHealMaxHealth:
				for _, effectTarget := range hitTargets {
					s.executeSpellMaxHealthHeal(effCtx, effectTarget, spellID)
				}
			case 6, 27, 35: // Apply Aura
				durationMs := uint32(0)
				if spell.DurationIndex > 0 && s.server != nil && s.server.Data != nil {
					if val, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level)); err == nil && ok && val > 0 {
						durationMs = uint32(val)
					}
				}
				if durationMs == 0 && eff.AuraPeriod > 0 {
					durationMs = eff.AuraPeriod * 5
				}
				periodMs := eff.AuraPeriod
				if periodMs == 0 && (eff.Aura == 3 || eff.Aura == 8 || eff.Aura == 23 || eff.Aura == 24 || eff.Aura == 89) {
					periodMs = 3000
				}
				amount := uint32(eff.BasePoints + 1)
				if amount <= 1 && isAreaEnemySpell(spell) {
					for _, areaEffect := range spell.Effects {
						if areaEffect.Effect == 27 && areaEffect.BasePoints >= 0 {
							amount = uint32(areaEffect.BasePoints + 1)
							break
						}
					}
				}
				if amount == 0 {
					if eff.Aura == 3 || eff.Aura == 23 || eff.Aura == 89 {
						amount = uint32(10 + int(s.player.Level)*2)
					} else if eff.Aura == 8 || eff.Aura == 20 {
						amount = uint32(15 + int(s.player.Level)*3)
					}
				}
				// Spell power bonus for periodic effects and absorption shields (TrinityCore Unit::SpellDamageBonusDone / SpellHealingBonusDone)
				if s.player != nil && s.player.SpellPower > 0 {
					if periodMs > 0 && (eff.Aura == 3 || eff.Aura == 23 || eff.Aura == 89 || eff.Aura == 8 || eff.Aura == 20) {
						tickBonus := uint32(math.Round(float64(s.player.SpellPower) * (float64(periodMs) / 15000.0)))
						amount += tickBonus
					} else if eff.Aura == SpellAuraSchoolAbsorb || eff.Aura == SpellAuraManaShield || eff.Aura == SpellAuraMagicAbsorb {
						shieldBonus := uint32(math.Round(float64(s.player.SpellPower) * 0.8068))
						amount += shieldBonus
					}
				}
				schoolMask := spell.SchoolMask
				if schoolMask == 0 {
					schoolMask = 1
				}

				for _, effectTarget := range hitTargets {
					auraTarget := s.playerGUID
					if isHarmfulSpell(spell) && isAreaEnemySpell(spell) && effectTarget != 0 && effectTarget != s.playerGUID {
						auraTarget = effectTarget
					} else if eff.ImplicitTargetA == 1 || isSelfCastOnly(spell) || isReflected {
						auraTarget = s.playerGUID
					} else if eff.ImplicitTargetA == 6 || isHarmfulAura(eff.Aura) {
						if effectTarget != 0 && effectTarget != s.playerGUID {
							auraTarget = effectTarget
						}
					} else if eff.ImplicitTargetA == 21 {
						if effectTarget != 0 {
							auraTarget = effectTarget
						}
					} else if effectTarget != 0 && effectTarget != s.playerGUID && isHarmfulSpell(spell) {
						auraTarget = effectTarget
					}
					s.applyAuraToTarget(effCtx, auraTarget, spell, eff, durationMs, periodMs, amount, schoolMask, castMerged, false, s.playerGUID)
				}
			case spellEffectResurrectNew: // SPELL_EFFECT_RESURRECT_NEW: self resurrect chain
				s.applySelfResurrectEffect(spell)
			case 5: // SPELL_EFFECT_TELEPORT_UNITS (e.g. Hearthstone 8690, Astral Recall 556)
				if (spellID == 8690 || spellID == 556) && s.player != nil {
					hbMap, hbX, hbY, hbZ := s.player.HomebindMap, s.player.HomebindX, s.player.HomebindY, s.player.HomebindZ
					if hbX == 0 && hbY == 0 && hbZ == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
						_ = s.server.WorldStore.DB.QueryRowContext(effCtx, "SELECT map, position_x, position_y, position_z FROM playercreateinfo WHERE race = ? AND class = ? LIMIT 1", s.player.Race, s.player.Class).Scan(&hbMap, &hbX, &hbY, &hbZ)
					}
					s.teleportTo(hbMap, hbX, hbY, hbZ, 0)
				}
			case 162: // SPELL_EFFECT_TALENT_SPEC_SELECT
				targetSpec := uint8(0)
				if eff.BasePoints+1 >= 2 {
					targetSpec = 1
				}
				s.activateSpec(effCtx, targetSpec)
			case 74: // SPELL_EFFECT_APPLY_GLYPH
				glyphPropID := uint16(eff.MiscValue)
				s.applyGlyph(effCtx, s.targetGlyphSlot, glyphPropID)
			case 56: // SPELL_EFFECT_SUMMON_PET
				s.handleSummonPet(effCtx, spellID, uint32(eff.MiscValue))
			case 28: // SPELL_EFFECT_SUMMON
				s.handleSummonPet(effCtx, spellID, uint32(eff.MiscValue))
			case 101: // SPELL_EFFECT_FEED_PET
				s.handleFeedPet(effCtx, spellID, target.ItemGUID, eff.TriggerSpell, uint8(effectIndex))
			case 102: // SPELL_EFFECT_DISMISS_PET
				s.handleDismissPet(effCtx)
			case 109: // SPELL_EFFECT_RESURRECT_PET
				s.handleResurrectPet(effCtx, spellID)
			case 55: // SPELL_EFFECT_TAMECREATURE
				s.handleTameCreature(effCtx, spellID, targetGUID)
			case 135: // SPELL_EFFECT_CALL_PET
				s.handleSummonPet(effCtx, spellID, 0)
			case 38: // SPELL_EFFECT_DISPEL
				s.handleEffectDispel(effCtx, targetGUID, spell, eff)
			case 108: // SPELL_EFFECT_DISPEL_MECHANIC
				s.handleEffectDispelMechanic(effCtx, targetGUID, spell, eff)
			case 114: // SPELL_EFFECT_ATTACK_ME (EffectTaunt)
				s.handleEffectTaunt(effCtx, targetGUID, spellID)
			case 126: // SPELL_EFFECT_STEAL_BENEFICIAL_BUFF
				s.handleEffectSpellsteal(effCtx, targetGUID, spell, eff)
			case spellEffectInterruptCast: // 68: SPELL_EFFECT_INTERRUPT_CAST
				s.handleEffectInterruptCast(effCtx, targetGUID, spell, eff)
				interruptHandled = true
			case spellEffectCreateItem: // 24: SPELL_EFFECT_CREATE_ITEM
				s.handleEffectCreateItem(effCtx, targetGUID, spell, eff)
			case spellEffectCreateItem2: // 70: SPELL_EFFECT_CREATE_ITEM_2
				s.handleEffectCreateItem(effCtx, targetGUID, spell, eff)
			case spellEffectLearnSpell: // 36: SPELL_EFFECT_LEARN_SPELL
				if eff.TriggerSpell != 0 {
					s.learnSpell(effCtx, eff.TriggerSpell)
				}
			case spellEffectResurrect: // 18: SPELL_EFFECT_RESURRECT
				s.handleEffectResurrect(effCtx, targetGUID, spell, eff)
			case spellEffectReputation: // 103: SPELL_EFFECT_REPUTATION
				if eff.MiscValue != 0 {
					s.giveReputation(effCtx, uint32(eff.MiscValue), eff.BasePoints+1)
				}
			case spellEffectQuestComplete: // 16: SPELL_EFFECT_QUEST_COMPLETE
				if eff.MiscValue != 0 {
					s.completeQuest(effCtx, uint32(eff.MiscValue))
				}
			case spellEffectHealthLeech: // 9: SPELL_EFFECT_HEALTH_LEECH
				s.handleEffectHealthLeech(effCtx, spellID, hitTargets, effectIndex, eff)
			case spellEffectPowerDrain: // 8: SPELL_EFFECT_POWER_DRAIN
				amount := eff.BasePoints + 1
				// SpellEffects.cpp:1277-1282: the drain amount is direct
				// damage for the caster's SpellDamageBonusDone before the
				// drain, so the caster's spell power scales it. Go models
				// the spellpower term of SpellDamageBonusDone (see
				// executeSpellDamage); the damage-taken side has no Go
				// infra.
				if s.player != nil && s.player.SpellPower > 0 {
					amount += int32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effectIndex, false)))
				}
				for _, effectTarget := range hitTargets {
					// SpellEffects.cpp:1301: the caster regains the drained
					// power scaled by the effect value multiplier, never
					// from a self drain.
					if drained := s.applySpellPowerBurn(effCtx, effectTarget, eff.MiscValue, amount, spellID); drained > 0 {
						if effectTarget != s.playerGUID {
							s.applySpellEnergize(effCtx, s.playerGUID, eff.MiscValue, int32(effectValueMultiplied(drained, eff.Amplitude)))
						}
					}
				}
			default:
				s.debug("unhandled spell effect", "spell", spellID, "effect", eff.Effect, "index", effectIndex)
			}
		}
		if s.server != nil && isHarmfulSpell(spell) && !damageEffectSeen {
			for _, effectTarget := range hitTargets {
				if effectTarget != 0 && effectTarget != s.playerGUID {
					s.server.triggerCreatureAggro(effCtx, effectTarget, s.playerGUID)
				}
			}
		}
		if isTauntSpell(spellID) {
			s.handleEffectTaunt(effCtx, targetGUID, spellID)
		}
		if isTotemSpell(spellID) {
			s.summonTotem(effCtx, spellID)
		} else if spellID == 36936 { // Totemic Recall
			s.destroyAllTotems()
		}
		if !interruptHandled && isInterruptSpell(spellID) {
			s.handleEffectInterruptCast(effCtx, targetGUID, spell, wotlk.SpellEffect{})
		}
		if spellID == 2641 { // Dismiss Pet
			s.handleDismissPet(effCtx)
		} else if spellID == 883 { // Call Pet
			s.handleSummonPet(effCtx, spellID, 0)
		} else if spellID == 31687 { // Summon Water Elemental
			s.handleSummonPet(effCtx, spellID, 510)
		} else if spellID == 46584 { // Raise Dead
			s.handleSummonPet(effCtx, spellID, 26125)
		} else if spellID == 63645 {
			s.activateSpec(effCtx, 0)
		} else if spellID == 63644 {
			s.activateSpec(effCtx, 1)
		} else if (spellID == 63624 || spellID == 63680) && s.player != nil && s.player.TalentGroupsCount < 2 {
			s.player.TalentGroupsCount = 2
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(effCtx, "UPDATE characters SET talentGroupsCount = 2 WHERE guid = ?", s.playerGUID)
			}
			_ = s.sendTalentsInfo(false)
		}
	}

	// Reference Spell.cpp:2156-2169:
	// If spell has projectile speed and target is not self, delay effect execution until missile arrival
	if spell.Speed > 0 && targetGUID != 0 && targetGUID != s.playerGUID {
		dist := float32(20.0) // default 20 yards if positions unknown
		if target, ok := s.getCombatTarget(ctx, targetGUID); ok {
			dx := target.X - s.player.X
			dy := target.Y - s.player.Y
			dz := target.Z - s.player.Z
			computedDist := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
			if computedDist > 5.0 {
				dist = computedDist
			} else {
				dist = 5.0
			}
		}
		timeDelayMs := int(math.Floor(float64(dist) / float64(spell.Speed) * 1000.0))
		if timeDelayMs > 0 {
			if timeDelayMs > 4000 {
				timeDelayMs = 4000
			}
			time.AfterFunc(time.Duration(timeDelayMs)*time.Millisecond, func() {
				applyEffects(context.Background())
				s.stopAttackOnSpellFinish(spell)
			})
			return
		}
	}

	applyEffects(ctx)
	s.stopAttackOnSpellFinish(spell)
}

// stopAttackOnSpellFinish stops the caster's auto-attack for spells carrying
// SPELL_ATTR0_STOP_ATTACK_TARGET.
// C++ authority: Spell::finish (Spell.cpp:3978-3983) calls AttackStop().
func (s *session) stopAttackOnSpellFinish(spell wotlk.Spell) {
	if spell.Attributes&spellAttr0StopAttackTarget == 0 {
		return
	}
	victim := s.attackTarget
	s.attackTarget = 0
	if s.autoRepeatSpell != 0 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	}
	_ = s.sendAttackStop(victim, false)
	s.debug("attack stopped by spell", "account", s.accountName, "spell", spell.ID)
}

func (s *session) spawnPersistentAreaAura(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	var persistent wotlk.SpellEffect
	found := false
	for _, effect := range spell.Effects {
		if effect.Effect == 27 {
			persistent, found = effect, true
			break
		}
	}
	if !found || spell.DurationIndex == 0 || s.server.Data == nil {
		return
	}
	durationMs, durationFound, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level))
	if err != nil || !durationFound || durationMs <= 0 {
		return
	}
	radius, radiusFound, err := s.server.Data.SpellRadius(persistent.RadiusIndex, uint32(s.player.Level))
	if err != nil || !radiusFound || radius <= 0 {
		return
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	if target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			x, y, z = destination.X, destination.Y, destination.Z
		}
	}
	lowGUID := s.server.nextDynamicSpellLowGUID()
	auraEffect := persistent
	for _, effect := range spell.Effects {
		if effect.Effect == 6 && effect.Aura != 0 {
			auraEffect = effect
			break
		}
	}
	periodMs := auraEffect.AuraPeriod
	if periodMs == 0 && (auraEffect.Aura == 3 || auraEffect.Aura == 8 || auraEffect.Aura == 23 || auraEffect.Aura == 24 || auraEffect.Aura == 89) {
		periodMs = 3000
	}
	amount := uint32(0)
	if auraEffect.BasePoints >= 0 {
		amount = uint32(auraEffect.BasePoints + 1)
	}
	if amount == 0 && (auraEffect.Aura == 3 || auraEffect.Aura == 23 || auraEffect.Aura == 89) {
		amount = uint32(10 + int(s.player.Level)*2)
	}
	schoolMask := uint8(spell.SchoolMask)
	if schoolMask == 0 {
		schoolMask = 1
	}
	object := &dynamicSpellObjectState{GUID: dynamicSpellGUID(lowGUID), CasterGUID: s.playerGUID, SpellID: uint64(spell.ID), Map: s.player.Map, InstanceID: s.player.InstanceID, X: x, Y: y, Z: z, Orientation: s.player.Orientation, Radius: radius, CastTime: uint32(time.Now().UnixMilli()), SpellData: spell, AuraEffect: auraEffect, AuraDurationMs: uint32(durationMs), AuraPeriodMs: periodMs, AuraAmount: amount, AuraSchoolMask: schoolMask, NextAuraTick: time.Now().Add(time.Duration(periodMs) * time.Millisecond)}
	s.server.spawnDynamicSpellObject(object, time.Duration(durationMs)*time.Millisecond)
}

// spellBonusMultiplier mirrors the coefficient selection in TrinityCore
// Unit::SpellDamageBonusDone (Unit.cpp:6685) and Unit::SpellHealingBonusDone
// (Unit.cpp:7562): SpellEffectInfo::BonusMultiplier = Spell.dbc
// EffectBonusCoefficient (fields 229-231). A negative DBC value falls back to
// the default (Cast Time / 3.5) coefficient, x1.88 for healing.
func (s *session) spellBonusMultiplier(spellID uint32, effIndex int, heal bool) float64 {
	mult := 0.857 // standard 3.0s cast (~85.7%)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			coeff := -1.0
			if effIndex >= 0 && effIndex < len(spell.Effects) {
				coeff = float64(spell.Effects[effIndex].BonusCoefficient)
			}
			if coeff < 0 {
				if spell.CastingTimeIndex > 0 {
					if ct, ok, _ := s.server.Data.SpellCastTime(spell.CastingTimeIndex); ok && ct > 0 {
						coeff = float64(ct) / 3500.0
						if coeff > 1.0 {
							coeff = 1.0
						}
					}
				} else {
					coeff = 1.5 / 3.5 // instant cast coefficient ~0.4286
				}
				if heal {
					coeff *= 1.88
				}
			}
			mult = coeff
		}
	}
	return mult
}

func (s *session) executeSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32, effIndex int) uint32 {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return 0
	}

	// Apply Spell Power bonus (TrinityCore Unit::SpellDamageBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		damage += uint32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effIndex, false)))
	}

	// Use the school mask from the Spell DBC (field 17). Fallback to physical (1).
	schoolMask := uint8(1)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.SchoolMask != 0 {
			schoolMask = uint8(spell.SchoolMask)
		}
	}

	return s.executeDirectSpellDamage(ctx, targetGUID, spellID, damage, schoolMask)
}

func (s *session) executeSpellInstantKill(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}
	packet := protocol.NewBuffer(20)
	packet.WriteU64(s.playerGUID)
	packet.WriteU64(targetGUID)
	packet.WriteU32(spellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLINSTAKILLLOG), packet.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToInstance(target.Map, target.InstanceID, uint16(protocol.OpcodeSMSG_SPELLINSTAKILLLOG), packet.Bytes(), s)
	}
	s.executeDirectSpellDamageWithFlags(ctx, targetGUID, spellID, target.Health, 1, true)
}

func (s *session) executeDirectSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8) uint32 {
	return s.executeDirectSpellDamageWithFlags(ctx, targetGUID, spellID, damage, schoolMask, false)
}

// spellDamagePushesBack mirrors the pushback half of Unit::DealDamage
// (Unit.cpp:937): damage dealt by a spell carrying
// SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE or SPELL_ATTR3_TREAT_AS_PERIODIC never
// delays the victim's cast or channel, and self-inflicted damage never
// pushes back (C++ victim != attacker). A spell that fails to load degrades
// to pushback, like the C++ null-spellProto path.
func (s *session) spellDamagePushesBack(spellID uint32, victimGUID uint64) bool {
	if victimGUID == s.playerGUID {
		return false
	}
	if s.server == nil || s.server.Data == nil {
		return true
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return true
	}
	return spell.AttributesEx3&spellAttr3TreatAsPeriodic == 0 && spell.AttributesEx7&spellAttr7NoPushbackOnDamage == 0
}

func (s *session) executeDirectSpellDamageWithFlags(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8, instantKill bool) uint32 {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return 0
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	if !isPlayerVictim && s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID) {
		hitInfo := uint32(0x01) // SPELL_HIT_TYPE_MISS
		damage = 0
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, 0, schoolMask, 0, 0, hitInfo), true)
		return 0
	}
	isHit := true
	if targetGUID != s.playerGUID && !instantKill {
		isHit = s.rollSpellHit(target.Level, isPlayerVictim)
	}
	hitInfo := uint32(0)
	resisted := uint32(0)
	absorbed := uint32(0)

	if !isHit {
		hitInfo = 0x01 // SPELL_HIT_TYPE_MISS
		damage = 0
	} else {
		// Spell crit roll (fixed damage backlash spells do not crit, per TrinityCore SPELL_ATTR4_FIXED_DAMAGE)
		crit := false
		spellKnown := false
		if s.server != nil && s.server.Data != nil {
			if _, found, err := s.server.Data.Spell(spellID); err == nil && found {
				spellKnown = true
			}
		}
		if !instantKill && spellKnown && spellID != 31117 && spellID != 64085 {
			crit = s.rollSpellCrit(target.GUID, schoolMask)
		}
		if crit {
			mult := 1.5
			if s.server != nil && s.server.Data != nil {
				if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
					mult = s.getSpellCritMultiplier(sp)
				} else {
					mult = s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: uint32(schoolMask)})
				}
			} else {
				mult = s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: uint32(schoolMask)})
			}
			damage = uint32(math.Round(float64(damage) * mult))
			hitInfo = 0x02 // SPELL_HIT_TYPE_CRIT
		}

		if !instantKill && schoolMask > 1 && s.player != nil && s.player.Level > 0 {
			chaosBolt := false
			if s.server != nil && s.server.Data != nil {
				if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
					chaosBolt = sp.SpellFamilyName == spellFamilyWarlock && sp.SpellIconID == 3178
				}
			}
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, target.Resistances, s.player.Level, target.Level, !isPlayerVictim, chaosBolt, s.player.SpellPenetration)
		}

		if instantKill {
			damage, hitInfo, resisted, absorbed = target.Health, 0, 0, 0
		}
		if isPlayerVictim {
			if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil {
				// Reference BE_SPELL_TARGET (28) and TIMED 6: being the
				// target of a hostile spell credits the victim and starts
				// target-timed criteria.
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget, spellID, 1)
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget2, spellID, 1)
				playerSess.startTimedAchievement(timedTypeSpellTarget, spellID)
				if !instantKill && playerSess.isImmuneToDamage(uint32(schoolMask)) {
					damage = 0
				}
				isCrit := (hitInfo & 0x02) != 0 // SPELL_HIT_TYPE_CRIT
				if !instantKill {
					playerSess.applyResilienceToDamage(true, &damage, isCrit, CombatRatingCritTakenSpell)
				}
				if !instantKill && damage > 0 {
					absorbed, damage = playerSess.applyAbsorptionShields(damage, schoolMask)
				}
			}
		} else if !instantKill && s.server != nil && damage > 0 {
			absorbed, damage = s.server.applyCreatureAbsorptionShields(creatureAuraKeyForTarget(target), damage, schoolMask)
		}
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	s.updateAchievementCriteria(criteriaTypeDamageDone, 0, damage)
	s.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)

	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo), true)

	// Trigger spell cast/hit procs (TrinityCore Unit::ProcDamageAndSpellFor);
	// suppressed for triggered casts (TRIGGERED_DISALLOW_PROC_EVENTS parity).
	if s.triggeredNoProcEvents == 0 {
		s.procSpellCastAndHitEffects(ctx, target, spellID)
	}

	s.lastCombatTime = time.Now()
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}

	// If target is an online player (e.g. duel opponent or PvP)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil && playerSess.player != nil {
			playerSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, damage)
			playerSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
			playerSess.lastCombatTime = time.Now()
			if playerSess.player.UnitFlags&unitFlagInCombat == 0 {
				playerSess.player.UnitFlags |= unitFlagInCombat
			}
			_ = playerSess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo), true)
			if damage > 0 && damage >= playerSess.player.Health {
				if s.duelPartner == target.GUID && s.player.DuelTeam != 0 {
					// Duel defeat: loser drops to 1 HP and duel completes
					playerSess.player.Health = 1
					playerSess.sendPlayerUpdate()
					s.endDuel(true, s.playerGUID, false)
				} else {
					playerSess.player.Health = 0
					playerSess.sendPlayerUpdate()
					if s.server != nil {
						s.server.creditHonorableKill(s, playerSess)
					}
					playerSess.killPlayer(ctx)
					s.server.handleWGPlayerDeath(playerSess, s)
				}
			} else if damage > 0 {
				playerSess.player.Health -= damage
				if s.spellDamagePushesBack(spellID, playerSess.playerGUID) {
					playerSess.delayCurrentCast()
					playerSess.delayCurrentChannel()
				}
				playerSess.procDamageAuras(true, damage)
				playerSess.sendPlayerUpdate()
			}
			return damage
		}
	}

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		if motion != nil {
			s.server.clearInstanceEncounter(motion)
			motion.Health = 0
			motion.DynamicFlags |= unitDynFlagLootable
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})
		s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target)
		s.debug("target slain by spell", "account", s.accountName, "spell", spellID, "guid", target.GUID)
	} else {
		newHealth := target.Health - damage
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		if motion != nil {
			motion.Health = newHealth
			motion.InCombat = true
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(target.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
			inMelee := dist <= meleeAttackRange
			threat := float32(damage) * s.getThreatMultiplier(uint32(schoolMask))
			switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
			if switched && newVictim != motion.TargetGUID {
				motion.TargetGUID = newVictim
				entries := motion.ThreatMgr.SortedEntries()
				s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
			} else {
				motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
			}
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
			}
			motion.Moving = true
		}
		s.server.motionMu.Unlock()
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
		s.server.procCreatureDamageAuras(creatureAuraKeyForTarget(target), true, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		s.server.triggerPetDefensive(s.player.Map, s.player.InstanceID, s.playerGUID, targetGUID)
	}
	return damage
}

// castSpellDirect triggers an immediate, instant cast of a spell without cast time or resource cost.
// Mirrors TrinityCore Unit::CastSpell (Spell.cpp: triggered = true).
func (s *session) castSpellDirect(ctx context.Context, spellID uint32, targetGUID uint64) {
	s.castSpellDirectWithOptions(ctx, spellID, targetGUID, false)
}

func (s *session) castFirstLoginSpell(ctx context.Context, spellID uint32, targetGUID uint64) {
	s.castSpellDirectWithOptions(ctx, spellID, targetGUID, true)
}

func (s *session) castSpellDirectWithOptions(ctx context.Context, spellID uint32, targetGUID uint64, firstLogin bool) {
	s.castSpellDirectWithOverrides(ctx, spellID, targetGUID, firstLogin, nil)
}

func (s *session) castSpellDirectWithBasePoint(ctx context.Context, spellID uint32, targetGUID uint64, basePoint uint32) {
	value := int32(basePoint)
	s.castSpellDirectWithOverrides(ctx, spellID, targetGUID, false, &value)
}

func (s *session) castSpellDirectWithOverrides(ctx context.Context, spellID uint32, targetGUID uint64, firstLogin bool, basePoint0 *int32) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Triggered casts map to C++ Unit::CastSpell(..., triggered=true), which sets
	// TRIGGERED_FULL_MASK including TRIGGERED_DISALLOW_PROC_EVENTS
	// (SpellDefines.h:155). C++ suppresses the attacker's own proc rolls for such
	// spells (Unit::TriggerAurasProcOnEvent, Unit.cpp:10424, Spell::IsProcDisabled).
	s.triggeredNoProcEvents++
	defer func() { s.triggeredNoProcEvents-- }()

	var spell wotlk.Spell
	found := false
	if s.server != nil && s.server.Data != nil {
		var err error
		spell, found, err = s.server.Data.Spell(spellID)
		if err != nil || !found {
			found = false
		}
	}
	if !found {
		spell = wotlk.Spell{ID: spellID}
	}
	if basePoint0 != nil && len(spell.Effects) != 0 {
		spell.Effects[0].BasePoints = *basePoint0 - 1
	}

	castID := uint8(0) // C++ m_cast_count stays 0 for triggered casts (Spell.cpp:602; set nonzero only for client-initiated casts, Player.cpp:8251)
	now := time.Now()
	castTimeStamp := uint32(now.UnixMilli())
	if firstLogin {
		castTimeStamp = gameTimeMS()
	}
	hitTargets := []uint64{targetGUID}
	spellTargetFlags := protocol.SpellTargetFlagUnitWireMask
	if firstLogin {
		spellTargetFlags = protocol.SpellTargetFlagUnit
	}
	spellTarget := protocol.SpellTargetData{Flags: spellTargetFlags, UnitGUID: targetGUID}
	// Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032): a triggered
	// spell with channel-dest implicit targets (76/106) resolves its
	// destination from the currently channeled spell, so the SMSG_SPELL_GO
	// spell-target block and dest-consuming read sites see it. Nothing is
	// resolved when no channel is live (the C++ null gate).
	if x, y, z, ok := s.channelDestForSpell(ctx, spell); ok {
		spellTarget.Flags |= protocol.SpellTargetFlagDestLocation
		spellTarget.Destination = protocol.SpellTargetLocation{X: x, Y: y, Z: z}
	}
	// Cast flags mirror Spell::SendSpellGo for a triggered player cast
	// (Spell.cpp:4283-4330): PENDING for triggered non-auto-repeat casts with
	// cast count 0 (Spell.cpp:4292), POWER_LEFT_SELF + remaining power for
	// non-health powers (Spell.cpp:4297), NO_GCD when the spell has no start
	// recovery time (Spell.cpp:4325). RUNE_LIST never applies to triggered
	// casts (FULL_MASK carries TRIGGERED_IGNORE_POWER_AND_REAGENT_COST).
	// AMMO (Spell.cpp:4295) is skipped: Go has no ammo display data, and the
	// flag without the 8-byte Ammo block would corrupt the packet.
	castFlags := uint32(spellCastFlagGo)
	if spell.AttributesEx1&spellAttr2AutorepeatFlag == 0 {
		castFlags |= spellCastFlagPending
	}
	var remainingPower *uint32
	if spell.PowerType != 0xFFFFFFFE /* POWER_HEALTH = -2 in C++ */ && int(spell.PowerType) < len(s.player.Powers) {
		castFlags |= protocol.SpellCastFlagPowerLeftSelf
		power := s.player.Powers[spell.PowerType]
		remainingPower = &power
	}
	if spell.StartRecoveryTime == 0 {
		castFlags |= protocol.SpellCastFlagNoGCD
	}
	// Spell::IsNeedSendToClient (Spell.cpp:7543): triggered casts only send
	// SMSG_SPELL_GO when the spell has a visual, is channeled, or has speed.
	if spell.SpellVisual[0] != 0 || spell.SpellVisual[1] != 0 || isChanneledSpell(spell) || spell.Speed > 0 {
		goPkt := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, castID, spellID, castFlags, castTimeStamp, hitTargets, nil, spellTarget, remainingPower)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
		if s.server != nil {
			// C++ sends the caster a self-only packet carrying POWER_LEFT_SELF
			// and re-broadcasts to the set with the flag (and power) stripped
			// (Spell.cpp:4353-4366).
			nearbyPacket := goPkt
			if castFlags&protocol.SpellCastFlagPowerLeftSelf != 0 {
				nearbyPacket = protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, castFlags&^protocol.SpellCastFlagPowerLeftSelf, castTimeStamp, hitTargets, nil, spellTarget)
			}
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
		}
	}

	// Spell::_handle_immediate_phase (Spell.cpp:3718): initial spell threat
	// (HandleThreatSpells, Spell.cpp:5096) applies to triggered casts too,
	// before any effect handling.
	s.handleSpellInitialThreat(ctx, spell, hitTargets)

	durationMs := uint32(0)
	if s.server != nil && s.server.Data != nil && spell.DurationIndex > 0 {
		lvl := uint32(80)
		if s.player != nil && s.player.Level > 0 {
			lvl = uint32(s.player.Level)
		}
		if dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, lvl); err == nil && ok && dur > 0 {
			durationMs = uint32(dur)
		}
	}
	if durationMs == 0 {
		switch spellID {
		case ProcSpellBerserking:
			durationMs = 12000
		case ProcSpellMongoose:
			durationMs = 15000
		case ProcSpellExecutioner:
			durationMs = 15000
		case ProcSpellCrusader:
			durationMs = 15000
		case ProcSpellCrippling:
			durationMs = 12000
		case ProcSpellDeadlyPois:
			durationMs = 12000
		case ProcSpellWoundPois:
			durationMs = 15000
		}
	}

	hasExplicitEffects := false
	// Per-(cast, target) first-merge marker for the aura re-apply path
	// (Spell.cpp:2842): only the first aura effect per target runs the
	// ModStackAmount(+1) merge, later effects only refresh their amounts.
	castMerged := make(map[uint64]struct{})
	for effectIndex, eff := range spell.Effects {
		if eff.Effect == 0 && eff.Aura == 0 {
			continue
		}
		hasExplicitEffects = true
		if eff.Effect == 1 { // SPELL_EFFECT_INSTAKILL
			s.executeSpellInstantKill(ctx, targetGUID, spellID)
		} else if eff.Effect == 2 { // SPELL_EFFECT_SCHOOL_DAMAGE
			baseDmg := uint32(eff.BasePoints + 1)
			if baseDmg == 0 {
				if spellID == ProcSpellFieryWeapon {
					baseDmg = 40
				} else if spellID == ProcSpellInstantPois {
					baseDmg = 280
				}
			}
			s.executeDirectSpellDamage(ctx, targetGUID, spellID, baseDmg, uint8(spell.SchoolMask))
		} else if eff.Effect == 6 || eff.Aura != 0 { // SPELL_EFFECT_APPLY_AURA
			amount := uint32(eff.BasePoints + 1)
			schoolMask := spell.SchoolMask
			if schoolMask == 0 {
				schoolMask = 1
			}
			s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, eff.AuraPeriod, amount, schoolMask, castMerged, false, s.playerGUID)
		} else if eff.Effect == 10 { // SPELL_EFFECT_HEAL
			healAmount := uint32(eff.BasePoints + 1)
			if healAmount == 0 && spellID == ProcSpellCrusader {
				healAmount = 100
			}
			s.executeSpellHeal(ctx, targetGUID, spellID, healAmount, effectIndex)
		} else if eff.Effect == spellEffectEnergize {
			s.applySpellEnergize(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1)
		} else if eff.Effect == spellEffectPowerBurn {
			if burned := s.applySpellPowerBurn(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1, spellID); burned > 0 {
				s.executeSpellDamage(ctx, targetGUID, spellID, effectValueMultiplied(burned, eff.Amplitude), effectIndex)
			}
		} else if eff.Effect == spellEffectTriggerSpell {
			if eff.TriggerSpell != 0 && eff.TriggerSpell != spellID {
				s.castSpellDirect(ctx, eff.TriggerSpell, targetGUID)
			}
		} else if eff.Effect == spellEffectThreat {
			s.applySpellThreat(ctx, targetGUID, eff.BasePoints+1)
		} else if eff.Effect == spellEffectHealMaxHealth {
			s.executeSpellMaxHealthHeal(ctx, targetGUID, spellID)
		}
	}

	if !hasExplicitEffects {
		eff := wotlk.SpellEffect{Effect: 6, Aura: 4}
		s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, 0, 0, 1, nil, false, s.playerGUID)
	}
}

func (s *session) applySpellEnergize(ctx context.Context, targetGUID uint64, powerType int32, amount int32) {
	s.adjustSpellPower(ctx, targetGUID, powerType, int64(amount))
}

// effectValueMultiplied applies the effect's value multiplier
// (Spell.dbc EffectAmplitude, SpellInfo.cpp:344) the way
// SpellEffectInfo::CalcValueMultiplier does: raw multiplication, no
// zero default. The SPELLMOD_VALUE_MULTIPLIER term has no Go infra.
func effectValueMultiplied(value uint32, amplitude float32) uint32 {
	return uint32(int32(float64(value) * float64(amplitude)))
}

// drainManaResilienceReduction mirrors the resilience term in
// Spell::EffectPowerDrain and Spell::EffectPowerBurn
// (SpellEffects.cpp:1285-1287): mana drains are reduced by the target's
// spell crit damage reduction, i.e. the min(resiliencePct*2.2, 33.0)%
// slice of getResilienceStats. C++'s GetCombatRatingDamageReduction
// returns 0 for non-players, and Go's creature motions carry no combat
// ratings, so the term only fires on player targets.
func drainManaResilienceReduction(target *session, amount uint32) uint32 {
	if target == nil || target.player == nil || amount == 0 {
		return 0
	}
	if int(CombatRatingCritTakenSpell) >= len(target.player.CombatRatings) {
		return 0
	}
	rating := target.player.CombatRatings[CombatRatingCritTakenSpell]
	if rating == 0 || target.player.Level == 0 {
		return 0
	}
	_, critDmgRed, _ := getResilienceStats(target.player.Level, rating)
	if critDmgRed <= 0 {
		return 0
	}
	// Unit::GetCombatRatingDamageReduction = CalculatePct(damage, pct),
	// which truncates for uint32.
	reduction := uint32(float64(amount) * float64(critDmgRed) / 100.0)
	if reduction >= amount {
		return amount
	}
	return reduction
}

func (s *session) applySpellPowerBurn(ctx context.Context, targetGUID uint64, powerType int32, amount int32, spellID uint32) uint32 {
	if s == nil || s.player == nil || powerType < 0 || powerType >= 7 || amount <= 0 {
		return 0
	}
	target := s.spellPowerTarget(targetGUID)
	if target == nil {
		// SpellEffects.cpp:1265-1282 (EffectPowerDrain) and :1352-1369
		// (EffectPowerBurn): the power terms fire on any alive unit whose
		// power type matches, not only players. Creature motions carry a
		// power model (Powers/MaxPowers/PowerType); motions without
		// populated max powers stay a no-op via the maximum != 0 gate.
		return s.applySpellPowerBurnToCreature(targetGUID, powerType, amount, spellID)
	}
	if target.player == nil || target.player.Health == 0 || playerPowerType(target.player) != uint8(powerType) {
		return 0
	}
	maximum := target.player.MaxPowers[uint32(powerType)]
	if maximum == 0 {
		return 0
	}
	burn := int64(amount)
	if spellID == 8129 {
		burn = int64(maximum) * burn / 100
		casterMax := s.player.MaxPowers[uint32(powerType)]
		cap := int64(casterMax) * int64(amount) * 2 / 100
		if burn > cap {
			burn = cap
		}
	}
	if burn <= 0 {
		return 0
	}
	// SpellEffects.cpp:1285-1287 (EffectPowerDrain) and the matching
	// EffectPowerBurn term: resilience reduces mana drains by the
	// target's spell crit damage reduction (added in 2.4). This is the
	// crit-damage slice only — not the flat resilience damage reduction
	// that applyResilienceToDamage applies to real damage.
	if powerType == 0 { // POWER_MANA
		burn -= int64(drainManaResilienceReduction(target, uint32(burn)))
	}
	if burn <= 0 {
		return 0
	}
	if burn > int64(target.player.Powers[uint32(powerType)]) {
		burn = int64(target.player.Powers[uint32(powerType)])
	}
	if burn <= 0 {
		return 0
	}
	target.adjustSpellPower(ctx, targetGUID, powerType, -burn)
	return uint32(burn)
}

// applySpellPowerBurnToCreature mirrors the creature-target arm of
// Spell::EffectPowerDrain/EffectPowerBurn: an alive unit whose power type
// matches loses up to burn power, and the drained amount is returned for
// the caster-gain (drain) or damage (burn) follow-ons. The Mana Burn 8129
// target/caster cap runs against the motion's max powers; the resilience
// term is dead here because C++'s GetCombatRatingDamageReduction returns
// 0 for non-players (see drainManaResilienceReduction).
func (s *session) applySpellPowerBurnToCreature(targetGUID uint64, powerType int32, amount int32, spellID uint32) uint32 {
	if s == nil || s.player == nil || s.server == nil || targetGUID == 0 {
		return 0
	}
	index := uint32(powerType)
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Health == 0 || motion.PowerType != index || motion.MaxPowers[index] == 0 {
		s.server.motionMu.Unlock()
		return 0
	}
	burn := int64(amount)
	if spellID == 8129 { // Mana Burn: burn x% of target's mana, capped at 2x% of caster's
		burn = int64(motion.MaxPowers[index]) * burn / 100
		cap := int64(s.player.MaxPowers[index]) * int64(amount) * 2 / 100
		if burn > cap {
			burn = cap
		}
	}
	if burn > int64(motion.Powers[index]) {
		burn = int64(motion.Powers[index])
	}
	if burn <= 0 {
		s.server.motionMu.Unlock()
		return 0
	}
	next := motion.Powers[index] - uint32(burn)
	motion.Powers[index] = next
	if index == 0 {
		motion.Mana = next
	} else if index == 4 {
		motion.Happiness = next
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldPower1 + int(index): next})
	return uint32(burn)
}

func (s *session) spellPowerTarget(targetGUID uint64) *session {
	if s == nil || s.player == nil {
		return nil
	}
	if targetGUID == 0 || targetGUID == s.playerGUID || s.server == nil {
		return s
	}
	return s.server.findSessionByGUID(targetGUID)
}

func (s *session) adjustSpellPower(ctx context.Context, targetGUID uint64, powerType int32, delta int64) {
	if s == nil || s.player == nil || powerType < 0 || powerType >= 7 || delta == 0 {
		return
	}
	if s.server != nil && targetGUID != 0 && targetGUID != s.playerGUID {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		if motion != nil && motion.PetID != 0 && motion.Health > 0 {
			index := uint32(powerType)
			maximum, old := motion.MaxPowers[index], motion.Powers[index]
			if maximum != 0 {
				var next uint32
				if delta < 0 {
					drain := uint64(-(delta + 1)) + 1
					if drain >= uint64(old) {
						next = 0
					} else {
						next = old - uint32(drain)
					}
				} else if delta >= int64(maximum-old) {
					next = maximum
				} else {
					next = old + uint32(delta)
				}
				if next != old {
					motion.Powers[index] = next
					if index == 0 {
						motion.Mana = next
					} else if index == 4 {
						motion.Happiness = next
					}
					mapID, petGUID := motion.Map, motion.GUID
					s.server.motionMu.Unlock()
					s.server.broadcastCreatureValuesUpdateInInstance(mapID, motion.InstanceID, petGUID, map[int]uint32{unitFieldPower1 + int(index): next})
					return
				}
			}
		}
		s.server.motionMu.Unlock()
	}
	target := s.spellPowerTarget(targetGUID)
	if target == nil || target.player == nil || target.player.Health == 0 {
		return
	}
	index := uint32(powerType)
	maximum := target.player.MaxPowers[index]
	if maximum == 0 {
		return
	}
	old := target.player.Powers[index]
	var newPower uint32
	if delta < 0 {
		drain := uint64(-delta)
		if drain >= uint64(old) {
			newPower = 0
		} else {
			newPower = old - uint32(drain)
		}
	} else {
		newPower = old + uint32(delta)
		if newPower < old || newPower > maximum {
			newPower = maximum
		}
	}
	if newPower == old {
		return
	}
	target.player.Powers[index] = newPower
	packet := protocol.NewBuffer(13)
	packet.WritePackedGUID(target.playerGUID)
	packet.WriteU8(uint8(powerType))
	packet.WriteU32(newPower)
	_ = target.write(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), true)
	if target.server != nil {
		target.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), target)
		if target.server.CharactersStore != nil && target.server.CharactersStore.DB != nil {
			col := fmt.Sprintf("power%d", index+1)
			_, _ = target.server.CharactersStore.DB.ExecContext(ctx, fmt.Sprintf("UPDATE characters SET %s = ? WHERE guid = ?", col), newPower, target.playerGUID)
		}
	}
}

func (s *session) applySpellThreat(ctx context.Context, targetGUID uint64, amount int32) {
	if s == nil || s.player == nil || s.server == nil || targetGUID == 0 || amount <= 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Health == 0 || isCreaturePassive(motion) {
		s.server.motionMu.Unlock()
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion.GUID)
	}
	wasInCombat := motion.InCombat
	inMelee := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z) <= meleeAttackRange
	switched, victim := motion.ThreatMgr.AddThreat(s.playerGUID, float32(amount), inMelee)
	motion.TargetGUID = victim
	motion.InCombat = true
	motion.Moving = false
	mapID := motion.Map
	entries := motion.ThreatMgr.SortedEntries()
	guid := motion.GUID
	s.server.motionMu.Unlock()
	if switched {
		s.server.broadcastHighestThreatUpdateInInstance(mapID, s.player.InstanceID, guid, victim, entries)
	}
	if !wasInCombat {
		s.server.broadcastAIReactionInInstance(mapID, s.player.InstanceID, guid, 2)
		startPkt := buildAttackStart(guid, s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, s)
	}
	_ = ctx
}

func (s *session) executeSpellMaxHealthHeal(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	if targetGUID == 0 || targetGUID == s.playerGUID {
		if s.player.Health == 0 {
			return
		}
		s.player.Health = s.player.MaxHealth
		s.sendPlayerUpdate()
		return
	}
	if s.server == nil {
		return
	}
	if target := s.server.findSessionByGUID(targetGUID); target != nil && target.player != nil && target.player.Health > 0 {
		target.player.Health = target.player.MaxHealth
		target.sendPlayerUpdate()
		return
	}
	if creature, ok := s.getCombatTarget(ctx, targetGUID); ok && creature.Health > 0 {
		s.executeSpellHeal(ctx, targetGUID, spellID, creature.MaxHealth, 0)
	}
}

func (s *session) executeSpellHeal(ctx context.Context, targetGUID uint64, spellID, heal uint32, effIndex int) {
	if s.player == nil {
		return
	}
	if targetGUID == 0 {
		targetGUID = s.playerGUID
	}

	targetSess := s
	if targetGUID != s.playerGUID && s.server != nil {
		if other := s.server.findSessionByGUID(targetGUID); other != nil && other.player != nil {
			targetSess = other
			// Reference BE_SPELL_TARGET (28) and TIMED 6: being the target of a
			// spell credits the victim/target and starts target-timed criteria.
			other.updateAchievementCriteria(criteriaTypeBeSpellTarget, spellID, 1)
			other.updateAchievementCriteria(criteriaTypeBeSpellTarget2, spellID, 1)
			other.startTimedAchievement(timedTypeSpellTarget, spellID)
		}
	}

	// Apply Spell Power bonus to healing (TrinityCore Unit::SpellHealingBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		heal += uint32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effIndex, true)))
	}

	// Roll healing critical strike (TrinityCore: 150% healing on crit, modified by metagem)
	isCrit := s.rollSpellCrit(0, 2)
	if isCrit {
		mult := s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: 2})
		heal = uint32(math.Round(float64(heal) * mult))
	}

	effectiveHeal := heal
	if targetSess.player.Health+heal > targetSess.player.MaxHealth {
		effectiveHeal = targetSess.player.MaxHealth - targetSess.player.Health
		targetSess.player.Health = targetSess.player.MaxHealth
	} else {
		targetSess.player.Health += heal
	}
	s.updateAchievementCriteria(criteriaTypeHealingDone, 0, heal)
	s.setAchievementCriteria(criteriaTypeHighestHealCasted, 0, heal)
	targetSess.updateAchievementCriteria(criteriaTypeTotalHealingReceived, 0, effectiveHeal)
	targetSess.setAchievementCriteria(criteriaTypeHighestHealingRecv, 0, heal)
	overheal := heal - effectiveHeal
	if s.server != nil {
		s.server.updateArenaHealingScore(s, effectiveHeal)
	}

	// TC Unit.cpp:6550: packet = packed(target), packed(healer), spellID, heal, overheal, absorb, crit, unused
	healPkt := buildSpellHealLog(targetGUID, s.playerGUID, spellID, heal, overheal, 0, isCrit)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), healPkt, true)
	if targetSess != s {
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), healPkt, true)
	}
	targetSess.sendPlayerUpdate()
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ? WHERE guid = ?", targetSess.player.Health, targetSess.playerGUID)
	}

	if s.server != nil && effectiveHeal > 0 {
		s.server.distributeHealingThreat(ctx, s.playerGUID, targetGUID, effectiveHeal)
	}
}

func buildSpellNonMeleeDamageLog(targetGUID, attackerGUID uint64, spellID, damage, overkill uint32, schoolMask uint8, extra ...uint32) []byte {
	// Layout matches TrinityCore Unit::SendSpellNonMeleeDamageLog (Unit.cpp:5302)
	// packed target, packed attacker, spellID, damage, overkill, schoolMask,
	// absorbed, resist, periodicLog, unused, blocked, HitInfo, HitInfo&debugMask
	absorb := uint32(0)
	resist := uint32(0)
	hitInfo := uint32(0)
	if len(extra) > 0 {
		absorb = extra[0]
	}
	if len(extra) > 1 {
		resist = extra[1]
	}
	if len(extra) > 2 {
		hitInfo = extra[2]
	}
	buf := protocol.NewBuffer(64)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(attackerGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(damage)
	buf.WriteU32(overkill)
	buf.WriteU8(schoolMask)
	buf.WriteU32(absorb)  // Absorbed
	buf.WriteU32(resist)  // Resist
	buf.WriteU8(0)        // periodicLog (0 = show spell name prefix)
	buf.WriteU8(0)        // unused
	buf.WriteU32(0)       // blocked
	buf.WriteU32(hitInfo) // HitInfo flags (0 = normal hit, 2 = SPELL_HIT_TYPE_CRIT)
	buf.WriteU8(0)        // HitInfo & debugMask (always 0, no crit/hit debug)
	return buf.Bytes()
}

func buildSpellHealLog(targetGUID, healerGUID uint64, spellID, healAmount, overheal, absorb uint32, crit bool) []byte {
	buf := protocol.NewBuffer(32)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(healerGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(healAmount)
	buf.WriteU32(overheal)
	buf.WriteU32(absorb)
	if crit {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU8(0)
	return buf.Bytes()
}

func (s *session) hasActiveSpell(spellID uint32) bool {
	for _, spell := range s.player.Spells {
		if spell.ID == spellID {
			return spell.Active && !spell.Disabled
		}
	}
	if s.player != nil && s.player.ShapeshiftForm != 0 && s.server != nil && s.server.Data != nil {
		if form, found, err := s.server.Data.ShapeshiftForm(uint32(s.player.ShapeshiftForm)); err == nil && found {
			for _, presetSpell := range form.PresetSpellIDs {
				if presetSpell == spellID {
					return true
				}
			}
		}
	}
	return false
}

func canPlayerCastSpell(learned, gmMode bool) bool {
	return learned || gmMode
}

func spellCastIgnoreReason(spell wotlk.Spell, found, learned bool) string {
	if !found {
		return "unknown spell"
	}
	if spell.Attributes&spellAttributePassive != 0 {
		return "passive spell"
	}
	if !learned {
		return "spell not learned"
	}
	return "unavailable"
}

// sendInterrupted mirrors C++ Spell::SendInterrupted (Spell.cpp:4624): after
// the caster receives its own SMSG_CAST_FAILED, SMSG_SPELL_FAILURE and
// SMSG_SPELL_FAILED_OTHER are broadcast to the set with the packed caster GUID.
func (s *session) sendInterrupted(castID uint8, spellID uint32, result uint8) {
	if s.server == nil || s.player == nil {
		return
	}
	payload := protocol.BuildSpellFailure(s.playerGUID, castID, spellID, result)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_FAILURE), payload, s)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_FAILED_OTHER), payload, s)
}

func (s *session) interruptCurrentCast() {
	s.castMu.Lock()
	if s.activeCast != nil {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		castID := s.activeCast.CastID
		spellID := s.activeCast.SpellID
		s.activeCast = nil
		// Spell::cancel (Spell.cpp:3210-3225) calls CancelGlobalCooldown() when
		// cancelled in SPELL_STATE_PREPARING.
		s.castMu.Unlock()
		s.cancelGlobalCooldown(spellID)

		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedInterrupted), true)
		s.sendInterrupted(castID, spellID, spellFailedInterrupted)
		return
	}
	s.castMu.Unlock()
}

// interruptSpellsOnMovement breaks the active cast and channel when the
// player starts moving, for spells carrying SPELL_INTERRUPT_FLAG_MOVEMENT.
// C++ authority: Unit::InterruptNonMeleeSpells via the movement interrupt
// mask (Unit.cpp).
func (s *session) interruptSpellsOnMovement() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	cast := s.activeCast
	channel := s.activeChannel
	castBreaks := cast != nil && !cast.Cancelled && cast.InterruptFlg&spellInterruptFlagMovement != 0
	channelBreaks := channel != nil && !channel.Stopped && channel.Spell.InterruptFlags&spellInterruptFlagMovement != 0
	s.castMu.Unlock()

	if castBreaks {
		s.interruptCurrentCast()
	}
	if channelBreaks {
		s.interruptCurrentChannel()
	}
}

func (s *session) stopSpellLifecycle() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	if s.activeCast != nil {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		s.activeCast = nil
	}
	channel := s.activeChannel
	if channel != nil {
		if channel.Timer != nil {
			channel.Timer.Stop()
		}
		if channel.TickTimer != nil {
			channel.TickTimer.Stop()
		}
		channel.Stopped = true
		s.activeChannel = nil
	}
	s.castMu.Unlock()
}

func (s *session) handleCancelCast(payload []byte) bool {
	reader := protocol.NewReader(payload)
	castID, _ := reader.ReadU8()
	spellID, _ := reader.ReadU32()

	s.castMu.Lock()
	if s.activeCast != nil {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		curCastID := s.activeCast.CastID
		curSpellID := s.activeCast.SpellID
		s.activeCast = nil
		s.castMu.Unlock()

		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(curCastID, curSpellID, spellFailedInterrupted), true)
		return true
	}
	s.castMu.Unlock()

	_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedInterrupted), true)
	return true
}

func (s *session) handleCancelChanneling(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Reference: cancel clears the running channel and its bar.
	s.interruptCurrentChannel()
	return true
}

func (s *session) handleCancelAura(payload []byte) bool {
	reader := protocol.NewReader(payload)
	spellID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.removeAura(spellID)
	return true
}

// activeAura tracks an applied periodic or timed aura on a unit (player or creature).
type activeAura struct {
	SpellID                    uint32
	DispelType                 uint32
	Mechanic                   uint32
	AuraType                   uint32
	EffectMask                 uint8
	CasterGUID                 uint64
	TargetGUID                 uint64
	ChannelTargetGUID          uint64
	TargetKey                  creatureAuraKey
	ItemGUID                   uint64
	SchoolMask                 uint32
	MiscValue                  int32
	Amount                     uint32
	Amounts                    [3]int32
	BaseAmounts                [3]int32
	RecalculateMask            uint8
	CritChance                 float32
	ApplyResilience            bool
	DurationMs                 uint32
	PeriodMs                   uint32
	RemainingMs                uint32
	DurationUpdatedAt          time.Time
	Slot                       uint8
	Positive                   bool
	CasterLevel                uint8
	StackCount                 uint8
	SingleTarget               bool
	RemainingCharges           uint8
	StackAmount                uint32
	HideDuration               bool
	AuraInterruptFlags         uint32
	TriggerSpell               uint32
	DRGroup                    DiminishingGroup
	DamageTaken                uint32
	OwnerPetAura               bool
	OwnerPetAuraSourceSpell    uint32
	OwnerPetAuraSourceEffect   uint8
	OwnerPetAuraSourceDamage   int32
	OwnerPetAuraRemoveOnChange bool
	Timer                      *time.Timer
	TickTimer                  *time.Timer
	Stopped                    bool
}

func isHarmfulAura(auraType uint32) bool {
	switch auraType {
	case 3: // SPELL_AURA_PERIODIC_DAMAGE
		return true
	case 5: // SPELL_AURA_MOD_CONFUSE
		return true
	case 6: // SPELL_AURA_MOD_CHARM
		return true
	case 7: // SPELL_AURA_MOD_FEAR
		return true
	case 11: // SPELL_AURA_MOD_TAUNT
		return true
	case 12: // SPELL_AURA_MOD_STUN
		return true
	case 26: // SPELL_AURA_MOD_ROOT
		return true
	case 27: // SPELL_AURA_MOD_SILENCE
		return true
	case 33: // SPELL_AURA_MOD_DECREASE_SPEED
		return true
	case 53: // SPELL_AURA_PERIODIC_LEECH
		return true
	case 89: // SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		return true
	default:
		return false
	}
}

func isHarmfulSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.Effect == 1 || eff.Effect == 2 || eff.Effect == 87 || eff.Effect == 108 || eff.Effect == 17 {
			return true
		}
		if eff.Effect == 6 && isHarmfulAura(eff.Aura) {
			return true
		}
		if eff.Effect == 129 {
			return true
		}
		if eff.Effect == 27 && (eff.TriggerSpell != 0 || eff.Aura == 3 || eff.Aura == 89 || isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) || eff.ImplicitTargetA == 18 || eff.ImplicitTargetB == 18) {
			return true
		}
		if eff.ImplicitTargetA == 2 || eff.ImplicitTargetA == 6 || eff.ImplicitTargetA == 15 || eff.ImplicitTargetA == 16 || eff.ImplicitTargetA == 24 || eff.ImplicitTargetA == 54 || eff.ImplicitTargetA == 104 {
			return true
		}
	}
	return false
}

func magicSpellHitResult(casterLevel, victimLevel uint8, isPlayerVictim bool, bonusHit ...float64) uint8 {
	lchance := int32(11)
	if isPlayerVictim {
		lchance = 7
	}
	leveldif := int32(victimLevel) - int32(casterLevel)
	var modHitChance float64
	if leveldif < 3 {
		modHitChance = float64(96 - leveldif)
	} else {
		modHitChance = float64(94 - (leveldif-2)*lchance)
	}
	if len(bonusHit) > 0 {
		modHitChance += bonusHit[0]
	}
	if modHitChance < 1.0 {
		modHitChance = 1.0
	} else if modHitChance > 100.0 {
		modHitChance = 100.0
	}
	roll := rand.Float64() * 100.0
	if roll >= modHitChance {
		return protocol.SpellMissMiss
	}
	return protocol.SpellMissNone
}

// isBinarySpell checks whether a spell is binary (i.e. does not deal direct damage,
// but applies harmful magic debuffs or crowd control that can be fully resisted).
// Mirrors TrinityCore SpellInfo::IsBinary (SpellInfo.cpp:3200).
func isBinarySpell(spell wotlk.Spell) bool {
	// Evaluate on the stripped mask: C++ removes the physical bit at load when
	// magic schools are present (SpellMgr.cpp: CU_SCHOOLMASK_NORMAL_WITH_MAGIC).
	schoolMask := spell.SchoolMask
	if schoolMask&0x7E != 0 {
		schoolMask &^= 1
	}
	// Physical (1) and Holy (2) spells are never resisted by magic resistance
	if schoolMask == 0 || schoolMask&1 != 0 || schoolMask&2 != 0 {
		return false
	}
	if !isHarmfulSpell(spell) {
		return false
	}
	// Direct damage spells suffer partial resistance rather than binary full resist
	for _, eff := range spell.Effects {
		if eff.Effect == 2 || eff.Effect == 17 || eff.Effect == 31 || eff.Effect == 58 || eff.Effect == 87 {
			return false
		}
	}
	return true
}

// minResistanceForMask mirrors Unit::GetResistance(SpellSchoolMask)
// (Unit.cpp:13581): the minimum resistance across the schools in the mask.
// The physical bit is stripped when magic schools are present, mirroring the
// load-time SchoolMask normalization (SpellMgr.cpp:
// SPELL_ATTR0_CU_SCHOOLMASK_NORMAL_WITH_MAGIC).
func minResistanceForMask(resistances [7]uint32, schoolMask uint8) uint32 {
	if schoolMask&0x7E != 0 {
		schoolMask &^= 1
	}
	best := uint32(0)
	found := false
	for i := uint8(0); i < 7; i++ {
		if schoolMask&(1<<i) != 0 && (!found || resistances[i] < best) {
			best = resistances[i]
			found = true
		}
	}
	if !found {
		return ^uint32(0)
	}
	return best
}

// averageResistReduction mirrors Unit::CalculateAverageResistReduction
// (Unit.cpp:1777): template resistance reduced by spell penetration; holy
// template resistance and Chaos Bolt (warlock, icon 3178) template resistance
// are ignored; the level-based term (+5 per level the victim out-levels the
// caster, impenetrable) is skipped for binary spells.
func averageResistReduction(resistances [7]uint32, schoolMask uint8, casterPenetration uint32, casterLevel, victimLevel uint8, binary, chaosBolt bool) float64 {
	templateRes := minResistanceForMask(resistances, schoolMask)
	if schoolMask&2 != 0 || chaosBolt {
		templateRes = 0
	}
	effectiveRes := templateRes
	if casterPenetration >= effectiveRes {
		effectiveRes = 0
	} else {
		effectiveRes -= casterPenetration
	}

	res := float64(effectiveRes)
	if !binary && victimLevel > casterLevel {
		res += float64(victimLevel-casterLevel) * 5.0
	}

	const bossLevel = 83
	const bossResistanceConstant = 510.0
	resConstant := float64(victimLevel) * 5.0
	if victimLevel == bossLevel {
		resConstant = bossResistanceConstant
	}
	if res <= 0 {
		return 0
	}
	return res / (res + resConstant)
}

// checkBinarySpellResist rolls the full resist for a binary spell, with the
// average resist reduction as the resist chance. Mirrors the binary branch of
// WorldObject::MagicSpellHitResult (Object.cpp:2585-2592).
func checkBinarySpellResist(resistances [7]uint32, schoolMask uint8, casterPenetration uint32, casterLevel, victimLevel uint8, chaosBolt bool) bool {
	averageResist := averageResistReduction(resistances, schoolMask, casterPenetration, casterLevel, victimLevel, true, chaosBolt)
	if averageResist <= 0.0 {
		return false
	}
	if averageResist > 1.0 {
		averageResist = 1.0
	}

	return rand.Float64() < averageResist
}

// schoolMaskToResistanceIndex converts SpellSchoolMask to resistance index:
// 0: Physical (Armor), 1: Holy, 2: Fire, 3: Nature, 4: Frost, 5: Shadow, 6: Arcane.
func schoolMaskToResistanceIndex(schoolMask uint8) uint8 {
	switch {
	case schoolMask&1 != 0:
		return 0
	case schoolMask&2 != 0:
		return 1
	case schoolMask&4 != 0:
		return 2
	case schoolMask&8 != 0:
		return 3
	case schoolMask&16 != 0:
		return 4
	case schoolMask&32 != 0:
		return 5
	case schoolMask&64 != 0:
		return 6
	default:
		return 0
	}
}

// calcMagicSpellResistance mirrors Unit::CalcSpellResistedDamage
// (Unit.cpp:1704): the discrete 0%-100% partial-resist roll in 10% steps,
// truncated like the C++ uint32 cast, with a fall-through roll resolving to
// full resist (clamped by the caller like DamageInfo::ResistDamage,
// Unit.cpp:208). Holy can only be partially resisted by creatures (the
// level-based term; template holy resistance is ignored); non-magical
// schools are never resisted.
func calcMagicSpellResistance(damage uint32, schoolMask uint8, resistances [7]uint32, casterLevel, victimLevel uint8, victimIsCreature, chaosBolt bool, casterPenetration ...uint32) (resisted uint32, remainingDamage uint32) {
	if damage == 0 || schoolMask == 0 || schoolMask&0x7E == 0 {
		return 0, damage
	}
	if schoolMask&2 != 0 && !victimIsCreature {
		return 0, damage
	}

	pen := uint32(0)
	if len(casterPenetration) > 0 {
		pen = casterPenetration[0]
	}

	averageResist := averageResistReduction(resistances, schoolMask, pen, casterLevel, victimLevel, false, chaosBolt)
	if averageResist <= 0.0 {
		return 0, damage
	}

	var discreteProb [11]float64
	if averageResist <= 0.1 {
		discreteProb[0] = 1.0 - 7.5*averageResist
		discreteProb[1] = 5.0 * averageResist
		discreteProb[2] = 2.5 * averageResist
	} else {
		for i := 0; i < 11; i++ {
			p := 0.5 - 2.5*math.Abs(0.1*float64(i)-averageResist)
			if p > 0 {
				discreteProb[i] = p
			}
		}
	}

	roll := rand.Float64()
	probSum := 0.0
	step := 11
	for i := 0; i < 11; i++ {
		probSum += discreteProb[i]
		if roll < probSum {
			step = i
			break
		}
	}

	resisted = damage * uint32(step) / 10
	if resisted > damage {
		resisted = damage
	}
	remainingDamage = damage - resisted
	return resisted, remainingDamage
}

func (s *Server) clearCreatureAuras(key creatureAuraKey) {
	if s == nil || key.GUID == 0 {
		return
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			for _, aura := range auras {
				if aura != nil {
					aura.Stopped = true
					if aura.Timer != nil {
						aura.Timer.Stop()
					}
					if aura.TickTimer != nil {
						aura.TickTimer.Stop()
					}
				}
			}
			delete(s.activeCreatureAuras, key)
		}
	}
	if s.creatureAuras != nil {
		delete(s.creatureAuras, key)
	}
}

func (s *session) clearActiveAuras() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if s.activeAuras != nil {
		for _, aura := range s.activeAuras {
			if aura != nil {
				aura.Stopped = true
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				if aura.DRGroup != DiminishingNone {
					s.applyDiminishingAura(aura.DRGroup, false)
					aura.DRGroup = DiminishingNone
				}
			}
		}
		s.activeAuras = make(map[uint32]*activeAura)
	}
}

func (s *session) sendAuraUpdate(slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32) {
	s.sendAuraUpdateWithStack(slot, spellID, remove, positive, maxDurationMs, durationMs, 1)
}

func (s *session) sendAuraUpdateWithStack(slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32, stackCount uint8) {
	if !remove && s.server != nil && s.server.Data != nil {
		if spell, found, _ := s.server.Data.Spell(spellID); found {
			maxDurationMs, durationMs = auraWireDurations(spell, maxDurationMs, durationMs)
		}
	}
	effectMask := uint8(0x01)
	s.castMu.Lock()
	if aura := s.activeAuras[spellID]; aura != nil && aura.EffectMask != 0 {
		effectMask = aura.EffectMask
	}
	s.castMu.Unlock()
	level := uint8(1)
	if s.player != nil && s.player.Level > 0 {
		level = s.player.Level
	}
	pkt := protocol.BuildAuraUpdateWithStackEffect(s.playerGUID, s.playerGUID, slot, spellID, remove, positive, maxDurationMs, durationMs, level, stackCount, effectMask)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), pkt, true)
}

func auraWireDurations(spell wotlk.Spell, maxDurationMs, durationMs uint32) (uint32, uint32) {
	if spell.AttributesEx5&spellAttr5HideDuration != 0 {
		return 0, 0
	}
	return maxDurationMs, durationMs
}

func spellEffectMask(spell wotlk.Spell, effect wotlk.SpellEffect) uint8 {
	for index, candidate := range spell.Effects {
		if candidate == effect && index < 8 {
			return uint8(1 << uint(index))
		}
	}
	return 0x01
}

func (s *session) applyAura(spellID uint32) {
	s.applyAuraWithDuration(spellID, 1800000)
}

func (s *session) applyAuraWithDuration(spellID uint32, durationMs uint32) {
	if s.auras == nil {
		s.auras = make(map[uint32]struct{})
	}
	if s.auraSlots == nil {
		s.auraSlots = make(map[uint32]uint8)
	}
	s.auras[spellID] = struct{}{}

	slot, ok := s.auraSlots[spellID]
	if !ok {
		used := make(map[uint8]bool)
		for _, sl := range s.auraSlots {
			used[sl] = true
		}
		var freeSlot uint8
		for sl := uint8(0); sl < 64; sl++ {
			if !used[sl] {
				freeSlot = sl
				break
			}
		}
		slot = freeSlot
		s.auraSlots[spellID] = slot
	}

	s.castMu.Lock()
	if s.activeAuras == nil {
		s.activeAuras = make(map[uint32]*activeAura)
	}
	if existing, exists := s.activeAuras[spellID]; exists && existing != nil {
		existing.Stopped = true
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.TickTimer != nil {
			existing.TickTimer.Stop()
		}
		// Aura::UnregisterSingleTarget (SpellAuras.cpp:1210): the replaced
		// aura leaves the caster's single-cast list.
		s.server.unregisterSingleCastAura(existing)
	}
	var auraInterruptFlags uint32
	var auraType uint32
	var dispelType uint32
	var mechanic uint32
	var miscValue int32
	var effectMask, recalculateMask uint8
	var amounts, baseAmounts [3]int32
	var effectAmount uint32
	stackCount := uint8(1)
	var stackAmount, procCharges uint32
	hideDuration := false
	if s.server != nil && s.server.Data != nil {
		if sp, found, _ := s.server.Data.Spell(spellID); found {
			auraInterruptFlags = sp.AuraInterruptFlags
			dispelType = sp.DispelType
			mechanic = sp.Mechanic
			stackAmount = sp.StackAmount
			hideDuration = sp.AttributesEx5&spellAttr5HideDuration != 0
			procCharges = sp.ProcCharges
			if sp.StackAmount == 0 && sp.ProcCharges > 0 {
				stackCount = uint8(sp.ProcCharges)
			}
			for index, effect := range sp.Effects {
				if effect.Effect == 0 || (index > 0 && auraType != 0 && effect.Aura != spellAuraMounted && !(auraType == 4 && effect.Aura == 4)) {
					continue
				}
				auraType = effect.Aura
				miscValue = effect.MiscValue
				if effect.Aura != 0 {
					effectMask |= 1 << uint(index)
					effectAmount = uint32(effect.BasePoints + 1)
					amounts[index] = effect.BasePoints + 1
					baseAmounts[index] = effect.BasePoints
					if auraEffectCanBeRecalculated(effect.Aura) {
						recalculateMask |= 1 << uint(index)
					}
				}
				if auraType == spellAuraMounted {
					break
				}
			}
			if mask, mountMisc, mountAmounts, mountBaseAmounts, mountedFlight := mountedFlightAuraEffects(sp); mountedFlight {
				effectMask, auraType, miscValue = mask, spellAuraMounted, mountMisc
				amounts, baseAmounts = mountAmounts, mountBaseAmounts
				recalculateMask = 0
				for index, effect := range sp.Effects {
					if mask&(1<<uint(index)) == 0 {
						continue
					}
					if auraEffectCanBeRecalculated(effect.Aura) {
						recalculateMask |= 1 << uint(index)
					}
					if effect.Aura == spellAuraMounted {
						effectAmount = uint32(effect.BasePoints + 1)
					}
				}
			}
		}
	}
	mounted := auraType == spellAuraMounted
	if mounted {
		s.clearOtherMountedAuras(spellID)
		durationMs = 0
		stackCount = 1
		procCharges = 0
	}
	positive := !isHarmfulAura(auraType)
	if spellID == 15007 {
		positive = false
	}
	if effectMask == 0 {
		effectMask = 0x01
	}
	aura := &activeAura{
		SpellID:            spellID,
		DispelType:         dispelType,
		Mechanic:           mechanic,
		AuraType:           auraType,
		EffectMask:         effectMask,
		CasterGUID:         s.playerGUID,
		TargetGUID:         s.playerGUID,
		MiscValue:          miscValue,
		Amount:             effectAmount,
		Amounts:            amounts,
		BaseAmounts:        baseAmounts,
		RecalculateMask:    recalculateMask,
		DurationMs:         durationMs,
		RemainingMs:        durationMs,
		DurationUpdatedAt:  time.Now(),
		SingleTarget:       isSingleTargetAuraSpellID(s.server.Data, spellID),
		Slot:               slot,
		Positive:           positive,
		AuraInterruptFlags: auraInterruptFlags,
		StackAmount:        stackAmount,
		HideDuration:       hideDuration,
		RemainingCharges:   uint8(procCharges),
	}
	if durationMs > 0 && durationMs < 18000000 {
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.removeAura(spellID)
		})
	}
	s.activeAuras[spellID] = aura
	// Unit::_AddAura single-target dance (Unit.cpp:3397-3420), like the
	// applyAuraToTarget fresh-apply paths.
	var scPurge []singleCastEntry
	if s.server != nil && s.server.Data != nil {
		if sp, found, _ := s.server.Data.Spell(spellID); found && spellIsSingleTarget(sp) {
			scPurge = s.server.registerSingleCastAura(sp, aura)
		}
	}
	s.castMu.Unlock()
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}

	if s.activeAuraHasEffect(aura, spellAuraModParryPercent) {
		s.updatePlayerParryPercentage(s.player, s.player.Level)
	}
	if mounted {
		s.applyMountedDisplay(context.Background(), aura)
	}
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, durationMs, durationMs, stackCount)
	s.sendPlayerUpdate()
	if auraType == 4 {
		s.addOwnerPetAuraEffects(context.Background(), spellID, effectMask)
	}
	if mounted {
		s.sendRuntimeMovementUpdates(spellAuraMounted)
	}
}

func (s *session) removeAura(spellID uint32) {
	wasMounted := false
	wasMovementControl := false
	wasConfused := false
	wasFleeing := false
	wasCharmed := false
	wasForcedReaction := false
	forcedReactionFaction := uint32(0)
	forcedReactionRank := uint32(0)
	wasTransform := false
	wasShapeshift := false
	wasStealth := false
	wasInvisibility := false
	wasTrackStealthed := false
	wasVisibilityAura := false
	wasMovementSpeedAura := false
	wasMountedFlight := false
	wasParryAura := false
	removedAuraType := uint32(0)
	removedFakeInebriation := uint32(0)
	removedEffectMask := uint8(0)
	s.castMu.Lock()
	if s.activeAuras != nil {
		if aura, ok := s.activeAuras[spellID]; ok && aura != nil {
			removedEffectMask = aura.EffectMask
			removedAuraType = aura.AuraType
			wasParryAura = s.activeAuraHasEffect(aura, spellAuraModParryPercent)
			wasMounted = aura.AuraType == spellAuraMounted
			wasMountedFlight = wasMounted && s.activeAuraHasEffect(aura, spellAuraMountedFlightSpeed)
			wasMovementControl = aura.AuraType == spellAuraStun || aura.AuraType == spellAuraRoot
			wasConfused = aura.AuraType == spellAuraConfuse
			wasFleeing = aura.AuraType == spellAuraFear
			wasCharmed = aura.AuraType == spellAuraCharm
			wasForcedReaction = aura.AuraType == 139
			if wasForcedReaction && aura.MiscValue >= 0 {
				forcedReactionFaction = uint32(aura.MiscValue)
				forcedReactionRank = aura.Amount
			}
			wasTransform = aura.AuraType == 56
			wasShapeshift = aura.AuraType == 36
			wasStealth = aura.AuraType == spellAuraStealth
			wasInvisibility = aura.AuraType == spellAuraInvisibility
			wasTrackStealthed = aura.AuraType == spellAuraTrackStealthed
			wasVisibilityAura = affectsPlayerVisibility(aura.AuraType)
			wasMovementSpeedAura = movementSpeedAura(aura.AuraType)
			if aura.AuraType == spellAuraFakeInebriation {
				removedFakeInebriation = aura.Amount
			}
			aura.Stopped = true
			if aura.Timer != nil {
				aura.Timer.Stop()
			}
			if aura.TickTimer != nil {
				aura.TickTimer.Stop()
			}
			if aura.DRGroup != DiminishingNone {
				s.applyDiminishingAura(aura.DRGroup, false)
				aura.DRGroup = DiminishingNone
			}
			delete(s.activeAuras, spellID)
			s.server.unregisterSingleCastAura(aura)
		}
	}
	s.castMu.Unlock()
	if removedEffectMask != 0 {
		s.removeOwnerPetAuraEffects(context.Background(), spellID, removedEffectMask)
	}
	s.removeOwnerPetAurasForSpell(context.Background(), spellID)

	if s.auras != nil {
		delete(s.auras, spellID)
	}
	if s.auraSlots != nil {
		if slot, ok := s.auraSlots[spellID]; ok {
			s.sendAuraUpdate(slot, 0, true, false, 0, 0)
			delete(s.auraSlots, spellID)
		}
	}
	if wasMounted && !s.hasAuraType(spellAuraMounted) && s.player != nil {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
		s.sendPlayerDismount()
	}
	if (wasTransform || wasShapeshift) && s.player != nil {
		s.refreshTransformDisplay(context.Background())
	}
	if wasStealth && s.player != nil && !s.hasAuraType(spellAuraStealth) {
		s.player.StandFlags &^= unitStandFlagCreep
		s.player.AuraVision &^= playerAuraVisionStealth
	}
	if wasInvisibility && s.player != nil && !s.hasAuraType(spellAuraInvisibility) {
		s.player.AuraVision &^= playerAuraVisionInvis
	}
	if wasTrackStealthed && s.player != nil && !s.hasAuraType(spellAuraTrackStealthed) {
		s.player.PlayerFieldBytes &^= playerFieldByteTrackStealthed
	}
	if wasMovementControl && !s.hasAuraType(spellAuraStun) && !s.hasAuraType(spellAuraRoot) {
		s.rooted = false
		if s.player != nil {
			s.player.UnitFlags &^= unitFlagStunned
			s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT))
		}
	}
	if wasConfused && !s.hasAuraType(spellAuraConfuse) && s.player != nil {
		s.player.UnitFlags &^= unitFlagConfused
	}
	if wasFleeing && !s.hasAuraType(spellAuraFear) && s.player != nil {
		s.player.UnitFlags &^= unitFlagFleeing
	}
	if wasCharmed && !s.hasAuraType(spellAuraCharm) {
		s.sendClientControl(s.playerGUID, true)
	}
	if wasForcedReaction {
		_ = s.sendForcedReactions()
		if forcedReactionFaction != 0 {
			rank := forcedReactionRank
			for _, reputation := range s.player.Reputations {
				if reputation.FactionID == forcedReactionFaction {
					if current := uint32(reputationRank(int64(reputation.Standing))); current > rank {
						rank = current
					}
					break
				}
			}
			if rank >= 4 {
				s.stopAttacksForFaction(context.Background(), forcedReactionFaction)
			}
		}
	}
	if removedFakeInebriation > 0 && s.player != nil {
		if removedFakeInebriation >= s.player.FakeInebriation {
			s.player.FakeInebriation = 0
		} else {
			s.player.FakeInebriation -= removedFakeInebriation
		}
	}
	if wasParryAura && s.player != nil {
		s.updatePlayerParryPercentage(s.player, s.player.Level)
	}
	s.sendPlayerUpdate()
	if wasVisibilityAura && s.server != nil {
		s.server.refreshPlayerVisibility()
	}
	if wasMovementSpeedAura {
		s.sendRuntimeMovementUpdates(removedAuraType)
	}
	if wasMountedFlight {
		s.sendRuntimeMovementUpdates(spellAuraMountedFlightSpeed)
	}
}

func (s *session) hasAura(spellID uint32) bool {
	if s.auras == nil {
		return false
	}
	_, ok := s.auras[spellID]
	return ok
}

// targetHasAura reports whether the unit named by targetGUID currently has aura
// auraSpell, for the target-side aura-spell requirement (SpellInfo::CheckTarget,
// SpellInfo.cpp:1769-1772). Player targets resolve to their session's aura set;
// creature targets consult the server creature aura maps.
func (s *session) targetHasAura(ctx context.Context, targetGUID uint64, auraSpell uint32) bool {
	if auraSpell == 0 || targetGUID == 0 {
		return false
	}
	if s.player != nil && targetGUID == s.playerGUID {
		return s.hasAura(auraSpell)
	}
	if s.server != nil {
		if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil {
			return targetSess.hasAura(auraSpell)
		}
		target, ok := s.getCombatTarget(ctx, targetGUID)
		if ok {
			key := creatureAuraKeyForTarget(target)
			s.server.auraMu.Lock()
			defer s.server.auraMu.Unlock()
			if _, found := s.server.creatureAuras[key][auraSpell]; found {
				return true
			}
			_, found := s.server.activeCreatureAuras[key][auraSpell]
			return found
		}
	}
	return false
}

func (s *session) clearOtherMountedAuras(spellID uint32) {
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted && aura.SpellID != spellID {
			s.removeAura(aura.SpellID)
		}
	}
}

func (s *session) applyMountedDisplay(ctx context.Context, aura *activeAura) {
	if s == nil || s.player == nil || aura == nil {
		return
	}
	entry := uint32(aura.MiscValue)
	if aura.SpellID == 62061 {
		if s.activeAuraHasEffect(aura, wotlk.MountedFlightSpeedAura) {
			entry = 24906
		} else {
			entry = 15665
		}
	}
	var displayID int64
	if entry > 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(modelid1, 0), NULLIF(modelid2, 0), NULLIF(modelid3, 0), NULLIF(modelid4, 0), 0) FROM creature_template WHERE entry = ?", entry).Scan(&displayID); err == nil && displayID > 0 {
			s.player.MountDisplayID = uint32(displayID)
		}
	}
	s.sendPlayerMountUpdate()
}

func (s *session) refreshTransformDisplay(ctx context.Context) {
	if s == nil || s.player == nil {
		return
	}
	s.loadTransformDisplay(ctx, s.player)
}

// refreshAuraEffectBasepoints mirrors the per-effect half of the
// _TryStackingOrRefreshingExistingAura basepoint update (Unit.cpp:3360-3372):
// the re-cast's basepoints and computed amount replace this effect's slots.
func refreshAuraEffectBasepoints(existing *activeAura, spell wotlk.Spell, eff wotlk.SpellEffect, amount uint32) {
	// Aura::SetStackAmount (SpellAuras.cpp:1008) recalculates every effect's
	// amount from the new basepoints via AuraEffect::CalculateAmount, whose
	// final term is amount *= GetBase()->GetStackAmount()
	// (SpellAuraEffects.cpp:537) — unconditional, all aura types. The merge
	// path carries the new cast's amount directly (the CalculateAmount
	// pre-stack value), so the stack multiplier lands here; a zero
	// StackCount reads as one stack, the codebase's existing convention.
	stacks := int32(existing.StackCount)
	if stacks < 1 {
		stacks = 1
	}
	for index, candidate := range spell.Effects {
		if candidate != eff {
			continue
		}
		existing.BaseAmounts[index] = eff.BasePoints
		scaled := int32(amount) * stacks
		existing.Amounts[index] = scaled
		existing.Amount = uint32(scaled)
		break
	}
}

// castMerged tracks, for one cast, the target GUIDs whose existing aura
// already ran the re-apply merge; a nil map means the caller applies a
// single aura effect per spell and keeps the old always-merge behavior.

// spellFirstRank mirrors SpellInfo::GetFirstRankSpell (SpellInfo.cpp:3323)
// via the spell_ranks world table, cached by petAuraStackGroups. A spell
// with no rank row is its own first rank.
func (s *Server) spellFirstRank(spellID uint32) uint32 {
	if s == nil {
		return spellID
	}
	if first, ok := s.petAuraStackGroups().firstRank[spellID]; ok {
		return first
	}
	return spellID
}

func auraTriggersSpell(spell wotlk.Spell, target uint32) bool {
	for _, eff := range spell.Effects {
		if eff.TriggerSpell == target {
			return true
		}
	}
	return false
}

// rankPurgeTarget is one aura the rank-chain no-stack purge selected for
// removal; the caller removes them after unlocking.
type rankPurgeTarget struct {
	spellID uint32
	slot    uint8
}

// Periodic aura types exempted from the different-caster rank-chain purge
// (Aura::CanStackWith, SpellAuras.cpp:1955-1976).
const (
	spellAuraPeriodicDamage                = 3   // SPELL_AURA_PERIODIC_DAMAGE (SpellAuraDefines.h:83)
	spellAuraPeriodicHeal                  = 8   // SPELL_AURA_PERIODIC_HEAL (SpellAuraDefines.h:88)
	spellAuraObsModHealth                  = 20  // SPELL_AURA_OBS_MOD_HEALTH (SpellAuraDefines.h:100)
	spellAuraObsModPower                   = 21  // SPELL_AURA_OBS_MOD_POWER (SpellAuraDefines.h:101)
	spellAuraPeriodicTriggerSpell          = 23  // SPELL_AURA_PERIODIC_TRIGGER_SPELL (SpellAuraDefines.h:103)
	spellAuraPeriodicEnergize              = 24  // SPELL_AURA_PERIODIC_ENERGIZE (SpellAuraDefines.h:104)
	spellAuraPeriodicLeech                 = 53  // SPELL_AURA_PERIODIC_LEECH (SpellAuraDefines.h:133)
	spellAuraPeriodicManaLeech             = 64  // SPELL_AURA_PERIODIC_MANA_LEECH (SpellAuraDefines.h:144)
	spellAuraPowerBurn                     = 162 // SPELL_AURA_POWER_BURN (SpellAuraDefines.h:242)
	spellAuraPeriodicDummy                 = 226 // SPELL_AURA_PERIODIC_DUMMY (SpellAuraDefines.h:306)
	spellAuraPeriodicTriggerSpellWithValue = 227 // SPELL_AURA_PERIODIC_TRIGGER_SPELL_WITH_VALUE (SpellAuraDefines.h:307)
)

// rankChainNoStackPurge mirrors the rank-chain term of
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671) via
// Aura::CanStackWith (SpellAuras.cpp:1880-2007): a fresh aura application
// removes the target's existing auras that share its spell rank chain under
// a different spell ID. Both caster cases are modeled: same-caster pairs
// always purge (SpellAuras.cpp:2004); different-caster pairs purge unless
// C++ lets them stack — the channeled exemption, the
// SPELL_ATTR3_STACK_FOR_DIFF_CASTERS read, and the periodic-aura-type
// exemption (SpellAuras.cpp:1943-1976) — so two players' Corruption ranks on
// one mob coexist while a single player's re-cast at a different rank
// replaces the old one.
// Honored in both directions: the trigger-spell mutual exclusion
// (SpellAuras.cpp:1901-1906), the spell-family gate (SpellAuras.cpp:1934),
// and the enchant-proc item edge (SpellAuras.cpp:1998-2000, degrade-open
// without the CU attr). Passive new spells skip the purge (the
// IsPassiveStackableWithRanks early-out shape, Unit.cpp:3643; Go holds no
// passive aura instances). IsMultiSlotAura (SpellAuras.cpp:1148) and
// CONTROL_VEHICLE are vacuous in Go. Spell-group stack rules are handled by
// the companion spellGroupNoStackPurge (rules EXCLUSIVE and
// EXCLUSIVE_FROM_SAME_CASTER) and exclusiveHighestVerdict (rule
// EXCLUSIVE_HIGHEST, both directions).
// Returns the auras to remove; the caller removes them after unlocking.
func (s *Server) rankChainNoStackPurge(newSpell wotlk.Spell, newCasterGUID, newItemGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if s.spellFirstRank(id) != newFirst {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		if exSpell.SpellFamilyName != newSpell.SpellFamilyName {
			continue
		}
		if aura.CasterGUID != newCasterGUID {
			if newSpell.AttributesEx3&spellAttr3StackForDiffCasters != 0 {
				continue
			}
			if isChanneledSpell(exSpell) {
				continue
			}
			if rankChainPeriodicStacksForDiffCasters(newSpell, exSpell) {
				continue
			}
		}
		if newItemGUID != 0 && aura.ItemGUID != 0 && newItemGUID != aura.ItemGUID {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// spellIsSingleTarget mirrors SpellInfo::IsSingleTarget
// (SpellInfo.cpp:1380-1395): the SPELL_ATTR5_SINGLE_TARGET_SPELL flag, or
// the JUDGEMENT spell-specific. The nil firstRank resolver is safe — the
// classifier guards it, and the judgement branch is ID-based.
func spellIsSingleTarget(spell wotlk.Spell) bool {
	if spell.AttributesEx5&spellAttr5SingleTarget != 0 {
		return true
	}
	return spellSpecific(spell, nil) == spellSpecificJudgement
}

// isSingleTargetWith mirrors Aura::IsSingleTargetWith
// (SpellAuras.cpp:1187-1208): same rank chain, or both spells sharing the
// JUDGEMENT / MAGE_POLYMORPH spell-specific.
func (s *Server) isSingleTargetWith(newSpell, exSpell wotlk.Spell) bool {
	if s.spellFirstRank(newSpell.ID) == s.spellFirstRank(exSpell.ID) {
		return true
	}
	newSpec := spellSpecific(newSpell, s.spellFirstRank)
	switch newSpec {
	case spellSpecificJudgement, spellSpecificMagePolymorph:
		return spellSpecific(exSpell, s.spellFirstRank) == newSpec
	}
	return false
}

// singleTargetNoStackPurge mirrors the single-target registration dance in
// Unit::_AddAura (Unit.cpp:3397-3420) with Aura::IsSingleTargetWith
// (SpellAuras.cpp:1187-1208): a fresh single-target aura (SpellInfo::
// IsSingleTarget, SpellInfo.cpp:1380-1395) removes the caster's other
// single-target auras that are single-target with it — same-rank-chain
// pairs, or a second judgement / polymorph from the same caster. The
// trigger-spell mutual exclusion the _RemoveNoStackAurasDueToAura purges
// honor does not exist in the C++ dance, so it is not applied here. C++
// dances the caster's cross-target single-cast list; Go has no such list,
// so this covers the same-target case (re-judging / re-sheeping the same
// unit) and the cross-target remainder stays open. Returns the auras to
// remove; the caller removes them after unlocking.
func (s *Server) singleTargetNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil || !spellIsSingleTarget(newSpell) {
		return nil
	}
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if aura.CasterGUID != newCasterGUID {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found || !spellIsSingleTarget(exSpell) {
			continue
		}
		if !s.isSingleTargetWith(newSpell, exSpell) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// singleCastEntry is one single-target aura registered on its caster,
// mirroring an element of Unit::m_scAuras (Unit.h:1287-1288). The aura
// pointer identifies the exact instance: removals verify it before expiring,
// so a re-created aura with the same spell ID is never touched.
type singleCastEntry struct {
	aura *activeAura
}

// sameSingleCastTarget reports whether two registry entries live on the same
// target: creature entries compare the full aura key, player entries the
// target GUID.
func sameSingleCastTarget(a, b *activeAura) bool {
	if a.TargetKey.GUID != 0 || b.TargetKey.GUID != 0 {
		return a.TargetKey == b.TargetKey
	}
	return a.TargetGUID == b.TargetGUID
}

// singleCastDanceLocked implements the single-target registration dance from
// Unit::_AddAura (Unit.cpp:3397-3420): when register is non-nil the fresh
// single-target aura is recorded on its caster's single-cast list
// (Unit::m_scAuras), replacing any entry for the same target+spell. Either
// way it returns the caster's other registered auras that are single-target
// with the new spell (Aura::IsSingleTargetWith, SpellAuras.cpp:1187-1208) on
// a different target — the cross-target dance. Same-target entries are left
// alone: that case is owned by singleTargetNoStackPurge. The caller must hold
// no aura locks; it removes the returned entries after unlocking via
// expireSingleCastEntry. Caller holds s.singleCastMu.
func (s *Server) singleCastDanceLocked(spell wotlk.Spell, register *activeAura, casterGUID uint64, exclude *activeAura) []singleCastEntry {
	var kept []singleCastEntry
	var purge []singleCastEntry
	for _, e := range s.singleCastAuras[casterGUID] {
		if e.aura == nil || e.aura == register || e.aura == exclude {
			continue
		}
		if register != nil && sameSingleCastTarget(e.aura, register) {
			// Same target: the new registration replaces a same-spell
			// entry; a different-spell entry stays registered and is
			// removed by the same-target purge (unregister cleans up).
			if e.aura.SpellID == register.SpellID {
				continue
			}
			kept = append(kept, e)
			continue
		}
		if s.Data == nil {
			kept = append(kept, e)
			continue
		}
		exSpell, found, err := s.Data.Spell(e.aura.SpellID)
		if err != nil || !found || !s.isSingleTargetWith(spell, exSpell) {
			kept = append(kept, e)
			continue
		}
		purge = append(purge, e)
	}
	if register != nil {
		kept = append(kept, singleCastEntry{aura: register})
	}
	if len(kept) == 0 {
		delete(s.singleCastAuras, casterGUID)
	} else {
		s.singleCastAuras[casterGUID] = kept
	}
	return purge
}

// registerSingleCastAura mirrors the registration half of Unit::_AddAura's
// dance (Unit.cpp:3397-3420): a fresh single-target aura
// (SpellInfo::IsSingleTarget, SpellInfo.cpp:1380-1395) is recorded on its
// caster's single-cast list, and the caster's other single-target-with auras
// on other targets are returned for removal. C++ gates on a non-null caster;
// a zero caster GUID degrades to no registration.
func (s *Server) registerSingleCastAura(spell wotlk.Spell, aura *activeAura) []singleCastEntry {
	if s == nil || aura == nil || aura.CasterGUID == 0 {
		return nil
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	if s.singleCastAuras == nil {
		s.singleCastAuras = make(map[uint64][]singleCastEntry)
	}
	return s.singleCastDanceLocked(spell, aura, aura.CasterGUID, nil)
}

// crossTargetSingleCastPurge runs only the purge half of the dance for a
// caster without registering anything, excluding one aura instance. Used by
// the spell-steal create path (below).
func (s *Server) crossTargetSingleCastPurge(casterGUID uint64, spell wotlk.Spell, exclude *activeAura) []singleCastEntry {
	if s == nil || casterGUID == 0 {
		return nil
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	return s.singleCastDanceLocked(spell, nil, casterGUID, exclude)
}

// unregisterSingleCastAura mirrors Aura::UnregisterSingleTarget
// (SpellAuras.cpp:1210-1216): a removed aura leaves its caster's single-cast
// list. No-op when the aura was never registered (DB-loaded auras,
// steal-created auras, non-single-target spells).
func (s *Server) unregisterSingleCastAura(aura *activeAura) {
	if s == nil || aura == nil || aura.CasterGUID == 0 {
		return
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	entries := s.singleCastAuras[aura.CasterGUID]
	for i, e := range entries {
		if e.aura == aura {
			entries[i] = entries[len(entries)-1]
			entries = entries[:len(entries)-1]
			break
		}
	}
	if len(entries) == 0 {
		delete(s.singleCastAuras, aura.CasterGUID)
	} else {
		s.singleCastAuras[aura.CasterGUID] = entries
	}
}

// expireSingleCastEntry removes a cross-target dance purge entry, but only
// when the registered aura instance is still the live one — mirroring the
// (*itr) != aura pointer check in Unit::_AddAura's dance loop
// (Unit.cpp:3409-3419). A re-created aura with the same spell ID is a
// different pointer and is never touched.
func (s *session) expireSingleCastEntry(e singleCastEntry) {
	if s == nil || s.server == nil || e.aura == nil {
		return
	}
	aura := e.aura
	if aura.TargetKey.GUID != 0 {
		key := aura.TargetKey
		s.server.auraMu.Lock()
		cur := s.server.activeCreatureAuras[key][aura.SpellID]
		s.server.auraMu.Unlock()
		if cur == aura {
			s.expireCreatureAura(key, aura.SpellID, aura.Slot)
		}
		return
	}
	if aura.TargetGUID == 0 {
		return
	}
	ts := s.server.findSessionByGUID(aura.TargetGUID)
	if ts == nil {
		return
	}
	ts.castMu.Lock()
	cur := ts.activeAuras[aura.SpellID]
	ts.castMu.Unlock()
	if cur == aura {
		ts.expirePlayerAura(aura.SpellID)
	}
}

// rankChainPeriodicStacksForDiffCasters mirrors the periodic-aura exemption
// in Aura::CanStackWith (SpellAuras.cpp:1955-1976): DOT/HOT-style auras of
// one rank chain from different casters stack. Effects are index-aligned
// exactly like C++ (the area gate reads Effects[i] on both spells); a
// periodic type targeting an area on either side (Replenishment-style)
// keeps the purge. isAreaAuraTarget was verified index-identical to
// SpellImplicitTargetInfo::IsArea from the C++ implicit-target table
// (SpellInfo.cpp:218-341). PERIODIC_DAMAGE_PERCENT (89) is deliberately
// absent — C++ lists only PERIODIC_DAMAGE (3).
func rankChainPeriodicStacksForDiffCasters(newSpell, exSpell wotlk.Spell) bool {
	for i, eff := range newSpell.Effects {
		switch eff.Aura {
		case spellAuraPeriodicDamage, spellAuraPeriodicDummy, spellAuraPeriodicHeal,
			spellAuraPeriodicTriggerSpell, spellAuraPeriodicEnergize,
			spellAuraPeriodicManaLeech, spellAuraPeriodicLeech,
			spellAuraPowerBurn, spellAuraObsModPower, spellAuraObsModHealth,
			spellAuraPeriodicTriggerSpellWithValue:
		default:
			continue
		}
		if isAreaAuraTarget(eff.ImplicitTargetA) || isAreaAuraTarget(eff.ImplicitTargetB) {
			continue
		}
		if i < len(exSpell.Effects) {
			ex := exSpell.Effects[i]
			if isAreaAuraTarget(ex.ImplicitTargetA) || isAreaAuraTarget(ex.ImplicitTargetB) {
				continue
			}
		}
		return true
	}
	return false
}

// Spell group stack rules (SpellMgr.h:326-334).
const (
	spellGroupStackRuleDefault             = 0 // SPELL_GROUP_STACK_RULE_DEFAULT
	spellGroupStackRuleExclusive           = 1 // SPELL_GROUP_STACK_RULE_EXCLUSIVE
	spellGroupStackRuleExclusiveSameCaster = 2 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_FROM_SAME_CASTER
	spellGroupStackRuleExclusiveSameEffect = 3 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_SAME_EFFECT
	spellGroupStackRuleExclusiveHighest    = 4 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_HIGHEST
)

// spellGroupStackRule mirrors SpellMgr::CheckSpellGroupStackRules
// (SpellMgr.cpp:438-488): group membership is read by first-rank spell ID
// (the petAuraStackCache spellGroups map is already expanded and
// first-rank-keyed), groups that overlap on both spells only through a
// shared nested subgroup are excluded, and the lowest group ID carrying a
// non-default rule wins.
func (s *Server) spellGroupStackRule(first1, first2 uint32) uint8 {
	cache := s.petAuraStackGroups()
	g1, ok1 := cache.spellGroups[first1]
	g2, ok2 := cache.spellGroups[first2]
	if !ok1 || !ok2 {
		return spellGroupStackRuleDefault
	}
	var common []uint32
	for g := range g1 {
		if _, ok := g2[g]; !ok {
			continue
		}
		excluded := false
		for _, sub := range cache.groupSubgroups[g] {
			_, in1 := g1[sub]
			_, in2 := g2[sub]
			if in1 && in2 {
				excluded = true
				break
			}
		}
		if !excluded {
			common = append(common, g)
		}
	}
	sort.Slice(common, func(i, j int) bool { return common[i] < common[j] })
	for _, g := range common {
		if rule := cache.groupRules[g]; rule != spellGroupStackRuleDefault {
			return rule
		}
	}
	return spellGroupStackRuleDefault
}

// spellGroupNoStackPurge mirrors the CheckSpellGroupStackRules terms of
// Aura::CanStackWith (SpellAuras.cpp:1924-1942) inside
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671): a fresh aura
// purges existing auras whose first-rank spell shares an EXCLUSIVE group
// with the new spell, or an EXCLUSIVE_FROM_SAME_CASTER group when the
// caster matches. The trigger-spell mutual exclusion (SpellAuras.cpp:
// 1901-1906) is honored — a triggered/triggering pair stacks, so it never
// purges. Rule EXCLUSIVE_SAME_EFFECT falls through to break in C++ (no
// purge); rule EXCLUSIVE_HIGHEST is handled by the companion
// exclusiveHighestVerdict (the IsHighestExclusiveAura port, Unit.cpp:13991).
// Returns the auras to remove; the caller removes them after unlocking.
func (s *Server) spellGroupNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		switch s.spellGroupStackRule(newFirst, s.spellFirstRank(id)) {
		case spellGroupStackRuleExclusive:
		case spellGroupStackRuleExclusiveSameCaster:
			if aura.CasterGUID != newCasterGUID {
				continue
			}
		default:
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// SpellSpecificType values (SpellInfo.h:149-174) for the
// SpellInfo::GetSpellSpecific classifier (_LoadSpellSpecific,
// SpellInfo.cpp:2041-2235).
const (
	spellSpecificNormal              = 0
	spellSpecificSeal                = 1
	spellSpecificAura                = 3
	spellSpecificSting               = 4
	spellSpecificCurse               = 5
	spellSpecificAspect              = 6
	spellSpecificTracker             = 7
	spellSpecificWarlockArmor        = 8
	spellSpecificMageArmor           = 9
	spellSpecificElementalShield     = 10
	spellSpecificMagePolymorph       = 11
	spellSpecificJudgement           = 13
	spellSpecificWarlockCorruption   = 17
	spellSpecificFood                = 19
	spellSpecificDrink               = 20
	spellSpecificFoodAndDrink        = 21
	spellSpecificPresence            = 22
	spellSpecificCharm               = 23
	spellSpecificScroll              = 24
	spellSpecificMageArcaneBrillance = 25
	spellSpecificWarriorEnrage       = 26
	spellSpecificPriestDivineSpirit  = 27
	spellSpecificHand                = 28
)

// Spell family names and aura/effect IDs consumed by the SpellSpecific
// classifier (SharedDefines.h:3581-3596, SpellAuraDefines.h, SpellDefines.h:65).
const (
	spellFamilyGeneric     = 0
	spellFamilyMage        = 3
	spellFamilyPriest      = 6
	spellFamilyHunter      = 9
	spellFamilyPaladin     = 10
	spellFamilyShaman      = 11
	spellFamilyDeathKnight = 15

	spellAuraModPossess     = 2
	spellAuraTrackCreatures = 44
	spellAuraTrackResources = 45
	spellAuraModRegen       = 84
	spellAuraModPowerRegen  = 85
	spellAuraModPossessPet  = 128
	spellAuraAoeCharm       = 177

	spellEffectApplyAura           = 6
	spellEffectPersistentAreaAura  = 27
	spellEffectApplyAreaAuraParty  = 35
	spellEffectApplyAreaAuraRaid   = 65
	spellEffectApplyAreaAuraPet    = 119
	spellEffectApplyAreaAuraFriend = 128
	spellEffectApplyAreaAuraEnemy  = 129
	spellEffectApplyAreaAuraOwner  = 143
)

// spellEffectIsAuraEffect mirrors SpellEffectInfo::IsAura (SpellInfo.cpp:370-373)
// via IsUnitOwnedAuraEffect (SpellAuras.cpp:273): an APPLY_AURA-family effect
// carrying a real aura type.
func spellEffectIsAuraEffect(eff wotlk.SpellEffect) bool {
	if eff.Aura == 0 {
		return false
	}
	switch eff.Effect {
	case spellEffectApplyAura, spellEffectPersistentAreaAura,
		spellEffectApplyAreaAuraParty, spellEffectApplyAreaAuraRaid,
		spellEffectApplyAreaAuraPet, spellEffectApplyAreaAuraFriend,
		spellEffectApplyAreaAuraEnemy, spellEffectApplyAreaAuraOwner:
		return true
	default:
		return false
	}
}

// spellHasAura mirrors SpellInfo::HasAura (SpellInfo.cpp:890-896).
func spellHasAura(spell wotlk.Spell, aura uint32) bool {
	for _, eff := range spell.Effects {
		if eff.Aura == aura && spellEffectIsAuraEffect(eff) {
			return true
		}
	}
	return false
}

// spellSpecific mirrors SpellInfo::_LoadSpellSpecific (SpellInfo.cpp:2041-2235):
// the per-spell exclusivity classifier consumed by the spell-specific stack
// gates (SpellInfo.cpp:1398-1449) and by _LoadAuraState's seal term
// (SpellInfo.cpp:1971). firstRank resolves GetFirstRankSpell()->Id for the
// scroll branch (nil degrades to the spell's own ID, like a missing chain
// entry); the seal branch never touches it, so spellAuraState can pass nil.
func spellSpecific(spell wotlk.Spell, firstRank func(uint32) uint32) uint8 {
	switch spell.SpellFamilyName {
	case spellFamilyGeneric:
		// Food / Drinks (mostly)
		if spell.AuraInterruptFlags&auraInterruptFlagNotSeated != 0 {
			food, drink := false, false
			for _, eff := range spell.Effects {
				if !spellEffectIsAuraEffect(eff) {
					continue
				}
				switch eff.Aura {
				// Food
				case spellAuraModRegen, spellAuraObsModHealth:
					food = true
				// Drink
				case spellAuraModPowerRegen, spellAuraObsModPower:
					drink = true
				}
			}
			switch {
			case food && drink:
				return spellSpecificFoodAndDrink
			case food:
				return spellSpecificFood
			case drink:
				return spellSpecificDrink
			}
		} else {
			if firstRank != nil {
				switch firstRank(spell.ID) {
				case 8118, // Strength
					8099, // Stamina
					8112, // Spirit
					8096, // Intellect
					8115, // Agility
					8091: // Armor
					return spellSpecificScroll
				}
			}
			switch spell.ID {
			case 12880, // Enrage (Enrage)
				14201,
				14202,
				14203,
				14204,
				57518, // Enrage (Wrecking Crew)
				57519,
				57520,
				57521,
				57522,
				57514, // Enrage (Imp. Defensive Stance)
				57516:
				return spellSpecificWarriorEnrage
			}
		}
	case spellFamilyMage:
		// family flags 18(Molten), 25(Frost/Ice), 28(Mage)
		if spell.SpellFamilyFlags[0]&0x12040000 != 0 {
			return spellSpecificMageArmor
		}
		// Arcane brillance and Arcane intelect (normal check fails because of flags difference)
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return spellSpecificMageArcaneBrillance
		}
		if spell.SpellFamilyFlags[0]&0x1000000 != 0 && spell.Effects[0].Aura == spellAuraConfuse {
			return spellSpecificMagePolymorph
		}
	case spellFamilyWarrior:
		if spell.ID == 12292 { // Death Wish
			return spellSpecificWarriorEnrage
		}
	case spellFamilyWarlock:
		// only warlock curses have this
		if spell.DispelType == DispelCurse {
			return spellSpecificCurse
		}
		// Warlock (Demon Armor | Demon Skin | Fel Armor)
		if spell.SpellFamilyFlags[1]&0x20000020 != 0 || spell.SpellFamilyFlags[2]&0x00000010 != 0 {
			return spellSpecificWarlockArmor
		}
		// seed of corruption and corruption
		if spell.SpellFamilyFlags[1]&0x10 != 0 || spell.SpellFamilyFlags[0]&0x2 != 0 {
			return spellSpecificWarlockCorruption
		}
	case spellFamilyPriest:
		// Divine Spirit and Prayer of Spirit
		if spell.SpellFamilyFlags[0]&0x20 != 0 {
			return spellSpecificPriestDivineSpirit
		}
	case spellFamilyHunter:
		// only hunter stings have this
		if spell.DispelType == DispelPoison {
			return spellSpecificSting
		}
		// only hunter aspects have this (but not all aspects in hunter family)
		if spell.SpellFamilyFlags[0]&0x00380000 != 0 || spell.SpellFamilyFlags[1]&0x00440000 != 0 || spell.SpellFamilyFlags[2]&0x00001010 != 0 {
			return spellSpecificAspect
		}
	case spellFamilyPaladin:
		// Collection of all the seal family flags. No other paladin spell has any of those.
		if spell.SpellFamilyFlags[1]&0x26000C00 != 0 || spell.SpellFamilyFlags[0]&0x0A000000 != 0 {
			return spellSpecificSeal
		}
		if spell.SpellFamilyFlags[0]&0x00002190 != 0 {
			return spellSpecificHand
		}
		// Judgement of Wisdom, Judgement of Light, Judgement of Justice
		switch spell.ID {
		case 20184, 20185, 20186:
			return spellSpecificJudgement
		}
		// only paladin auras have this (for palaldin class family)
		if spell.SpellFamilyFlags[2]&0x00000020 != 0 {
			return spellSpecificAura
		}
	case spellFamilyShaman:
		// family flags 10 (Lightning), 42 (Earth), 37 (Water), proc shield from T2 8 pieces bonus
		if spell.SpellFamilyFlags[1]&0x420 != 0 || spell.SpellFamilyFlags[0]&0x00000400 != 0 || spell.ID == 23552 {
			return spellSpecificElementalShield
		}
	case spellFamilyDeathKnight:
		if spell.ID == 48266 || spell.ID == 48263 || spell.ID == 48265 {
			return spellSpecificPresence
		}
	}
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectApplyAura {
			switch eff.Aura {
			case spellAuraCharm, spellAuraModPossessPet, spellAuraModPossess, spellAuraAoeCharm:
				return spellSpecificCharm
			case spellAuraTrackCreatures:
				/// @workaround For non-stacking tracking spells (We need generic solution)
				if spell.ID == 30645 { // Gas Cloud Tracking
					return spellSpecificNormal
				}
				return spellSpecificTracker
			case spellAuraTrackResources, spellAuraTrackStealthed:
				return spellSpecificTracker
			}
		}
	}
	return spellSpecificNormal
}

// isAuraExclusiveBySpecificWith mirrors SpellInfo::IsAuraExclusiveBySpecificWith
// (SpellInfo.cpp:1398-1429).
func isAuraExclusiveBySpecificWith(spec1, spec2 uint8) bool {
	switch spec1 {
	case spellSpecificTracker,
		spellSpecificWarlockArmor,
		spellSpecificMageArmor,
		spellSpecificElementalShield,
		spellSpecificMagePolymorph,
		spellSpecificPresence,
		spellSpecificCharm,
		spellSpecificScroll,
		spellSpecificWarriorEnrage,
		spellSpecificMageArcaneBrillance,
		spellSpecificPriestDivineSpirit:
		return spec1 == spec2
	case spellSpecificFood:
		return spec2 == spellSpecificFood || spec2 == spellSpecificFoodAndDrink
	case spellSpecificDrink:
		return spec2 == spellSpecificDrink || spec2 == spellSpecificFoodAndDrink
	case spellSpecificFoodAndDrink:
		return spec2 == spellSpecificFood || spec2 == spellSpecificDrink || spec2 == spellSpecificFoodAndDrink
	default:
		return false
	}
}

// isAuraExclusiveBySpecificPerCasterWith mirrors
// SpellInfo::IsAuraExclusiveBySpecificPerCasterWith (SpellInfo.cpp:1431-1449).
func isAuraExclusiveBySpecificPerCasterWith(spec1, spec2 uint8) bool {
	switch spec1 {
	case spellSpecificSeal,
		spellSpecificHand,
		spellSpecificAura,
		spellSpecificSting,
		spellSpecificCurse,
		spellSpecificAspect,
		spellSpecificJudgement,
		spellSpecificWarlockCorruption:
		return spec1 == spec2
	default:
		return false
	}
}

// spellSpecificNoStackPurge mirrors the "check spell specific stack rules" term
// of Aura::CanStackWith (SpellAuras.cpp:1912-1921) inside
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671): a fresh aura purges
// existing auras whose SpellSpecific classification is exclusive with the new
// spell's, or — with the same caster — whose per-caster specific matches. The
// TRACK_RESOURCES config term (SpellAuras.cpp:1912-1916) precedes the specific
// gates: when both auras carry SPELL_AURA_TRACK_RESOURCES they stack only if
// AllowTrackBothResources is set (C++ CONFIG_ALLOW_TRACK_BOTH_RESOURCES,
// worldserver.conf default false); otherwise the existing tracking aura is
// purged. The trigger-spell mutual exclusion (SpellAuras.cpp:1901-1906) is
// honored like the companion purge functions. Passive new spells skip the
// purge (the IsPassiveStackableWithRanks early-out shape, Unit.cpp:3643; Go
// holds no passive aura instances). Returns the auras to remove; the caller
// removes them after unlocking.
func (s *Server) spellSpecificNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	var purge []rankPurgeTarget
	// The config term applies regardless of classification, so it runs before
	// the spellSpecificNormal early return below.
	if !s.Config.AllowTrackBothResources && spellHasAura(newSpell, spellAuraTrackResources) {
		for id, aura := range existing {
			if id == newSpell.ID || aura == nil || aura.Stopped {
				continue
			}
			exSpell, found, err := s.Data.Spell(id)
			if err != nil || !found {
				continue
			}
			if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
				continue
			}
			if spellHasAura(exSpell, spellAuraTrackResources) {
				purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			}
		}
	}
	newSpec := spellSpecific(newSpell, s.spellFirstRank)
	if newSpec == spellSpecificNormal {
		return purge
	}
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		// Tracking pairs were already decided by the config term above; C++
		// never evaluates the specific gates for them (SpellAuras.cpp:1916).
		if spellHasAura(newSpell, spellAuraTrackResources) && spellHasAura(exSpell, spellAuraTrackResources) {
			continue
		}
		exSpec := spellSpecific(exSpell, s.spellFirstRank)
		sameCaster := aura.CasterGUID == newCasterGUID
		if !isAuraExclusiveBySpecificWith(newSpec, exSpec) &&
			!(sameCaster && isAuraExclusiveBySpecificPerCasterWith(newSpec, exSpec)) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// spellEffectIsAreaAura mirrors SpellEffectInfo::IsAreaAuraEffect
// (SpellInfo.cpp:385-395): the APPLY_AREA_AURA_* effect family
// (SharedDefines.h:846/876/930/939/940/954).
func spellEffectIsAreaAura(effect uint32) bool {
	switch effect {
	case 35, 65, 119, 128, 129, 143:
		return true
	default:
		return false
	}
}

// newSpellAuraEffectMask is the bit mask of the spell effects the aura
// sweep routes to applyAuraToTarget (eff.Effect == 6 || eff.Aura != 0) —
// the Go counterpart of the new aura's create-time effect mask used by the
// IsHighestExclusiveAuraEffect tie-break (Unit.cpp:14001).
func newSpellAuraEffectMask(spell wotlk.Spell) uint8 {
	var mask uint8
	for index, eff := range spell.Effects {
		if index >= 8 {
			break
		}
		if eff.Effect == 6 || eff.Aura != 0 {
			mask |= 1 << uint(index)
		}
	}
	return mask
}

func popcount8(v uint8) int {
	n := 0
	for v != 0 {
		n += int(v & 1)
		v >>= 1
	}
	return n
}

// exclusiveHighestVerdict mirrors Unit::IsHighestExclusiveAura and
// IsHighestExclusiveAuraEffect (Unit.cpp:13991-14036) for pairs whose
// CheckSpellGroupStackRules verdict is SPELL_GROUP_STACK_RULE_EXCLUSIVE_HIGHEST
// (SpellMgr.cpp:438-488, rule 4): for the new effect, every existing aura
// effect of the same aura type on the target is compared by absolute amount,
// with the effect-mask bit-count difference as the tie-break. A strictly
// higher new effect purges the existing aura — except an area aura owned by
// the target itself, which C++ never removes (SpellAuras.cpp:696,
// "no removing of area auras from the original owner, as that completely
// cancels them"); a strictly lower new effect suppresses the whole new aura
// (Unit.cpp:3648 / SpellAuras.cpp:696, addUnit=false); an exact tie purges
// the existing aura via the CanStackWith rule-4 term (SpellAuras.cpp:1928,
// "existing aura is lower/equal"), honoring the trigger-spell mutual
// exclusion (SpellAuras.cpp:1901-1906) like the companion purge functions.
// Purges already collected stay applied when a later comparison suppresses
// the new aura, matching C++'s immediate removals before its early return.
type exclusiveHighestVerdict struct {
	purge      []rankPurgeTarget
	suppressed bool
}

func (s *Server) exclusiveHighestVerdict(newSpell wotlk.Spell, eff wotlk.SpellEffect, newAmount int32, targetGUID uint64, existing map[uint32]*activeAura) exclusiveHighestVerdict {
	var out exclusiveHighestVerdict
	if s == nil || s.Data == nil {
		return out
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return out
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	newMask := newSpellAuraEffectMask(newSpell)
	newBits := popcount8(newMask)
	newAbs := absAuraAmount(newAmount)
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if s.spellGroupStackRule(newFirst, s.spellFirstRank(id)) != spellGroupStackRuleExclusiveHighest {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		for index, exEff := range exSpell.Effects {
			if index >= 8 || exEff.Aura != eff.Aura {
				continue
			}
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			diff := newAbs - absAuraAmount(aura.Amounts[index])
			if diff == 0 {
				diff = int64(newBits) - int64(popcount8(aura.EffectMask))
			}
			switch {
			case diff < 0:
				out.suppressed = true
				return out
			case diff > 0:
				if isExistingAreaAuraOfTarget(aura, exSpell, targetGUID) {
					continue
				}
				out.purge = append(out.purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			default:
				if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
					continue
				}
				out.purge = append(out.purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			}
		}
	}
	return out
}

// isExistingAreaAuraOfTarget reports the SpellAuras.cpp:696 area-aura guard:
// the existing aura carries an applied area-aura effect and its owner
// (the caster) is the target itself.
func isExistingAreaAuraOfTarget(aura *activeAura, exSpell wotlk.Spell, targetGUID uint64) bool {
	if aura == nil || aura.CasterGUID != targetGUID {
		return false
	}
	for index, exEff := range exSpell.Effects {
		if index >= 8 {
			break
		}
		if aura.EffectMask&(1<<uint(index)) != 0 && spellEffectIsAreaAura(exEff.Effect) {
			return true
		}
	}
	return false
}

// applyAuraToTarget applies one aura effect to a player target. casterGUID is
// the aura's caster: normally the casting session's player, but the
// spellsteal path passes the victim aura's original caster
// (Unit::RemoveAurasDueToSpellBySteal, Unit.cpp:4020:
// createInfo.SetCasterGUID(aura->GetCasterGUID())) — the no-stack purge's
// same-caster terms and the wire caster field key on it, not on the stealer.
func (s *session) applyAuraToTarget(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, durationMs, periodMs, amount, schoolMask uint32, castMerged map[uint64]struct{}, skipSingleCastReg bool, casterGUID uint64) {
	if s.player == nil {
		return
	}
	channelTargetGUID := uint64(0)
	if eff.Aura == 23 && eff.TriggerSpell != 0 {
		channelTargetGUID = s.channelTargetForSpell(spell.ID)
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	mountEffectMask, mountMiscValue, mountAmounts, mountBaseAmounts, mountedFlight := mountedFlightAuraEffects(spell)

	positive := !isHarmfulAura(eff.Aura) && eff.ImplicitTargetA != 6
	if isAreaEnemySpell(spell) {
		positive = false
	}

	// Target is a player (self or other online player)
	var targetSess *session
	if targetGUID == s.playerGUID || targetGUID == 0 {
		targetSess = s
		targetGUID = s.playerGUID
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	if targetSess != nil && targetSess.player != nil {
		if targetSess.player.Health == 0 {
			return
		}
		if targetSess.isImmuneToSpell(spell) {
			return
		}
		if eff.Aura == 36 {
			for _, existing := range targetSess.loadedAuras() {
				if existing != nil && existing.AuraType == 36 && existing.SpellID != spell.ID {
					targetSess.removeAura(existing.SpellID)
				}
			}
		}
		if eff.Aura == spellAuraMounted || mountedFlight {
			targetSess.clearOtherMountedAuras(spell.ID)
			durationMs = 0
			periodMs = 0
		}

		var drGroup DiminishingGroup
		if !positive && durationMs > 0 {
			var ok bool
			drGroup, durationMs, ok = targetSess.applyDiminishingToDuration(spell.ID, spell.Mechanic, durationMs, true)
			if !ok {
				// Target is immune to crowd control due to DR
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(1, spell.ID, 38), true) // SPELL_FAILED_IMMUNE = 38
				return
			}
			targetSess.incrDiminishing(drGroup)
			targetSess.applyDiminishingAura(drGroup, true)
		}

		targetSess.castMu.Lock()
		if targetSess.auras == nil {
			targetSess.auras = make(map[uint32]struct{})
		}
		if targetSess.auraSlots == nil {
			targetSess.auraSlots = make(map[uint32]uint8)
		}
		if targetSess.activeAuras == nil {
			targetSess.activeAuras = make(map[uint32]*activeAura)
		}

		previous := targetSess.activeAuras[spell.ID]
		if existing := previous; existing != nil && !existing.Stopped {
			// Spell::DoSpellEffectHit (Spell.cpp:2842): only the first aura
			// effect per (cast, target) runs the merge — the first hit sets
			// hitInfo.HitAura and later effects only AddStaticApplication.
			// Go's per-effect sweep would otherwise run the ModStackAmount(+1)
			// merge once per aura effect, so a re-cast of a stackable
			// multi-effect aura would reach max stacks N× too fast. Later
			// effects only refresh this effect's basepoints/amounts.
			firstMerge := true
			if castMerged != nil {
				if _, ok := castMerged[targetGUID]; ok {
					firstMerge = false
				} else {
					castMerged[targetGUID] = struct{}{}
				}
			}
			if !firstMerge {
				refreshAuraEffectBasepoints(existing, spell, eff, amount)
				targetSess.castMu.Unlock()
				return
			}
			// Unit::_TryStackingOrRefreshingExistingAura (Unit.cpp:3326) ->
			// Aura::ModStackAmount(+1) (SpellAuras.cpp:1030): a re-cast merges
			// into the existing aura instead of replacing it — one more stack,
			// clamped to the spell's stack amount (1 when the spell is not
			// stackable). The +1 refresh always resets the charge count to the
			// spell's max charges and the duration to the new cast's duration
			// (DoSpellEffectHit, Spell.cpp:2899-2910). Go holds one aura per
			// spell ID, so the C++ caster-GUID match is vacuous.
			maxStack := int32(spell.StackAmount)
			if maxStack == 0 {
				maxStack = 1
			}
			cur := int32(existing.StackCount)
			if cur == 0 {
				cur = 1
			}
			if cur++; cur > maxStack {
				cur = maxStack
			}
			stackCount := uint8(cur)
			existing.StackCount = stackCount
			existing.RemainingCharges = uint8(spell.ProcCharges)
			// Unit::_TryStackingOrRefreshingExistingAura basepoint update
			// (Unit.cpp:3360-3372): a re-cast overwrites the aura's per-effect
			// basepoints with the new cast's values, and the live amounts are
			// recalculated from them (Aura::SetStackAmount, SpellAuras.cpp:1008).
			// Go has no stack-scaled amount recalc, so the amounts follow the
			// new cast directly; the stack-scaled multiplier half stays unmodeled.
			refreshAuraEffectBasepoints(existing, spell, eff, amount)
			existing.DurationMs = durationMs
			existing.RemainingMs = durationMs
			existing.DurationUpdatedAt = time.Now()
			// Aura::RefreshTimers(resetPeriodicTimer) (Spell.cpp:2854):
			// resetPeriodicTimer = StackAmount < 2 &&
			// !(triggeredCastFlags & TRIGGERED_DONT_RESET_PERIODIC_TIMER).
			// Every C++ triggered cast carries the bit — TRIGGERED_FULL_MASK
			// (0x0007FFFF) includes TRIGGERED_DONT_RESET_PERIODIC_TIMER
			// (0x00020000, SpellDefines.h:151) — so a triggered re-cast of a
			// non-stackable aura keeps the periodic tick countdown alive
			// (AuraEffect::CalculatePeriodic, SpellAuraEffects.cpp:631). The
			// triggered-cast funnel bumps triggeredNoProcEvents, so the
			// counter doubles as the FULL_MASK marker here; the
			// client-initiated path never sets it.
			resetPeriodic := spell.StackAmount < 2 && s.triggeredNoProcEvents == 0
			if existing.Timer != nil {
				existing.Timer.Stop()
				existing.Timer = nil
			}
			if resetPeriodic {
				if existing.TickTimer != nil {
					existing.TickTimer.Stop()
					existing.TickTimer = nil
				}
				existing.PeriodMs = periodMs
			}
			slot, effectMask, charges := existing.Slot, existing.EffectMask, existing.RemainingCharges
			targetSess.castMu.Unlock()
			if resetPeriodic && periodMs > 0 {
				targetSess.schedulePlayerPeriodicTick(existing, periodMs)
			}
			if durationMs > 0 && durationMs < 18000000 {
				targetSess.castMu.Lock()
				existing.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
					targetSess.expirePlayerAura(spell.ID)
				})
				targetSess.castMu.Unlock()
			}
			// Charges ride the stack-count field of the aura update
			// (player_auras.go), same convention as the fresh-apply path.
			wireStack := stackCount
			if !mountedFlight && eff.Aura != spellAuraMounted && spell.StackAmount == 0 && spell.ProcCharges > 0 {
				wireStack = charges
			}
			wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
			updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, casterGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, wireStack, effectMask)
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
			if s.server != nil {
				s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, targetSess)
			}
			targetSess.sendPlayerUpdate()
			return
		}
		if existing := previous; existing != nil {
			existing.Stopped = true
			if existing.Timer != nil {
				existing.Timer.Stop()
			}
			if existing.TickTimer != nil {
				existing.TickTimer.Stop()
			}
			if existing.DRGroup != DiminishingNone {
				targetSess.applyDiminishingAura(existing.DRGroup, false)
				existing.DRGroup = DiminishingNone
			}
		}

		// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
		// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
		// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
		// removes it before the no-stack purge). The strictly-higher purges
		// are applied below with the other no-stack purges.
		highest := s.server.exclusiveHighestVerdict(spell, eff, int32(amount), targetGUID, targetSess.activeAuras)
		if highest.suppressed {
			targetSess.castMu.Unlock()
			return
		}

		targetSess.auras[spell.ID] = struct{}{}
		slot, ok := targetSess.auraSlots[spell.ID]
		if !ok {
			used := make(map[uint8]bool)
			for _, sl := range targetSess.auraSlots {
				used[sl] = true
			}
			var freeSlot uint8
			for sl := uint8(0); sl < 64; sl++ {
				if !used[sl] {
					freeSlot = sl
					break
				}
			}
			slot = freeSlot
			targetSess.auraSlots[spell.ID] = slot
		}

		aura := &activeAura{
			SpellID:            spell.ID,
			DispelType:         spell.DispelType,
			Mechanic:           spell.Mechanic,
			AuraType:           eff.Aura,
			EffectMask:         spellEffectMask(spell, eff),
			CasterGUID:         casterGUID,
			TargetGUID:         targetGUID,
			ChannelTargetGUID:  channelTargetGUID,
			SchoolMask:         schoolMask,
			MiscValue:          eff.MiscValue,
			Amount:             amount,
			DurationMs:         durationMs,
			PeriodMs:           periodMs,
			RemainingMs:        durationMs,
			DurationUpdatedAt:  time.Now(),
			Slot:               slot,
			Positive:           positive,
			CasterLevel:        s.player.Level,
			SingleTarget:       isSingleTargetAuraSpell(spell),
			AuraInterruptFlags: spell.AuraInterruptFlags,
			TriggerSpell:       eff.TriggerSpell,
			DRGroup:            drGroup,
			StackAmount:        spell.StackAmount,
			HideDuration:       spell.AttributesEx5&spellAttr5HideDuration != 0,
			RemainingCharges:   uint8(spell.ProcCharges),
		}
		setAuraEffectPersistence(aura, spell, eff, amount)
		if eff.Aura == 4 && previous != nil && previous.AuraType == 4 {
			currentMask := spellEffectMask(spell, eff)
			aura.EffectMask |= previous.EffectMask
			for index := range spell.Effects {
				bit := uint8(1 << uint(index))
				if previous.EffectMask&bit == 0 || currentMask&bit != 0 {
					continue
				}
				aura.Amounts[index], aura.BaseAmounts[index] = previous.Amounts[index], previous.BaseAmounts[index]
				aura.RecalculateMask |= previous.RecalculateMask & bit
			}
		}
		if previous != nil && (previous.AuraType == 36 || eff.Aura == 36) {
			aura.EffectMask |= previous.EffectMask
			aura.Amounts, aura.BaseAmounts = previous.Amounts, previous.BaseAmounts
			aura.RecalculateMask |= previous.RecalculateMask
			if previous.AuraType == 36 && eff.Aura != 36 {
				aura.AuraType, aura.MiscValue = previous.AuraType, previous.MiscValue
			}
		}
		if mountedFlight {
			aura.EffectMask |= mountEffectMask
			aura.AuraType = spellAuraMounted
			aura.MiscValue = mountMiscValue
			aura.Positive = true
			aura.DurationMs, aura.RemainingMs, aura.PeriodMs = 0, 0, 0
			aura.DRGroup, aura.StackCount, aura.RemainingCharges = DiminishingNone, 1, 0
			aura.RecalculateMask &^= mountEffectMask
			for index, effect := range spell.Effects {
				bit := uint8(1 << uint(index))
				if mountEffectMask&bit == 0 {
					continue
				}
				aura.Amounts[index], aura.BaseAmounts[index] = mountAmounts[index], mountBaseAmounts[index]
				if effect == eff {
					aura.Amounts[index] = int32(amount)
				}
				if auraEffectCanBeRecalculated(effect.Aura) {
					aura.RecalculateMask |= bit
				}
				if effect.Aura == spellAuraMounted {
					aura.Amount = uint32(aura.Amounts[index])
				}
			}
		}
		targetSess.activeAuras[spell.ID] = aura
		// Unit::_AddAura single-target dance (Unit.cpp:3397-3420): a fresh
		// single-target aura registers on its caster's single-cast list and
		// purges the caster's other single-target-with auras on other
		// targets. Steal-created auras skip registration (Unit.cpp:4028).
		var scPurge []singleCastEntry
		if !skipSingleCastReg && spellIsSingleTarget(spell) {
			scPurge = s.server.registerSingleCastAura(spell, aura)
		}
		// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): the fresh
		// aura purges auras of other spells it can't stack with — the
		// rank-chain term (Aura::CanStackWith, SpellAuras.cpp:1994-2004),
		// the spell-group exclusive terms (SpellAuras.cpp:1924-1932),
		// the spell-specific exclusivity gates (SpellAuras.cpp:1914-1921),
		// and the EXCLUSIVE_HIGHEST comparisons (Unit.cpp:13991) — plus
		// the _AddAura single-target dance (Unit.cpp:3397-3420) for
		// single-target auras. The purge keys on the new aura's caster
		// (the same-caster terms compare against it), which is the
		// casterGUID param — not the casting session — because the steal
		// and dynamic-object paths create the aura with a foreign caster
		// (Unit::RemoveAurasDueToSpellBySteal, Unit.cpp:4020:
		// createInfo.SetCasterGUID(aura->GetCasterGUID())).
		purgeIDs := s.server.rankChainNoStackPurge(spell, casterGUID, aura.ItemGUID, targetSess.activeAuras)
		purgeIDs = append(purgeIDs, s.server.spellGroupNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.spellSpecificNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.singleTargetNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, highest.purge...)
		targetSess.castMu.Unlock()
		for _, purgeID := range purgeIDs {
			targetSess.expirePlayerAura(purgeID.spellID)
		}
		for _, e := range scPurge {
			s.expireSingleCastEntry(e)
		}
		if eff.Aura == spellAuraModParryPercent {
			targetSess.updatePlayerParryPercentage(targetSess.player, targetSess.player.Level)
		}
		if eff.Aura == spellAuraFakeInebriation {
			targetSess.player.FakeInebriation += amount
		}
		if eff.Aura == spellAuraStun || eff.Aura == spellAuraRoot {
			targetSess.rooted = true
			if eff.Aura == spellAuraStun {
				targetSess.player.UnitFlags |= unitFlagStunned
			}
			targetSess.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
		}
		if eff.Aura == spellAuraConfuse || eff.Aura == spellAuraFear {
			targetSess.interruptCurrentCast()
			targetSess.interruptCurrentChannel()
			if targetSess.attackTarget != 0 {
				_ = targetSess.handleAttackStop()
			}
			if eff.Aura == spellAuraConfuse {
				targetSess.player.UnitFlags |= unitFlagConfused
			} else {
				targetSess.player.UnitFlags |= unitFlagFleeing
			}
		}
		if eff.Aura == spellAuraCharm {
			targetSess.clearOtherMountedAuras(0)
			targetSess.interruptCurrentCast()
			targetSess.interruptCurrentChannel()
			if targetSess.attackTarget != 0 {
				_ = targetSess.handleAttackStop()
			}
			targetSess.sendClientControl(targetSess.playerGUID, false)
		}
		if eff.Aura == 139 {
			_ = targetSess.sendForcedReactions()
			if eff.MiscValue >= 0 && amount >= 4 {
				targetSess.stopAttacksForFaction(ctx, uint32(eff.MiscValue))
			}
		}
		if eff.Aura == spellAuraMounted {
			targetSess.applyMountedDisplay(ctx, aura)
			aura.StackCount = 1
			aura.RemainingCharges = 0
		}
		if eff.Aura == spellAuraStealth {
			targetSess.player.StandFlags |= unitStandFlagCreep
			targetSess.player.AuraVision |= playerAuraVisionStealth
		}
		if eff.Aura == spellAuraInvisibility {
			targetSess.player.AuraVision |= playerAuraVisionInvis
		}
		if eff.Aura == spellAuraTrackStealthed {
			targetSess.player.PlayerFieldBytes |= playerFieldByteTrackStealthed
		}
		if eff.Aura == 36 || eff.Aura == 56 {
			targetSess.refreshTransformDisplay(ctx)
		}

		stackCount := uint8(1)
		if !mountedFlight && eff.Aura != spellAuraMounted && spell.StackAmount == 0 && spell.ProcCharges > 0 {
			stackCount = uint8(spell.ProcCharges)
		}
		wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, stackCount, spellEffectMask(spell, eff))
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, targetSess)
		}
		targetSess.sendPlayerUpdate()
		if eff.Aura == 4 {
			targetSess.addOwnerPetAuraEffects(ctx, spell.ID, spellEffectMask(spell, eff))
		}
		if movementSpeedAura(eff.Aura) {
			targetSess.sendRuntimeMovementUpdates(eff.Aura)
		}
		if affectsPlayerVisibility(eff.Aura) && targetSess.server != nil {
			targetSess.server.refreshPlayerVisibility()
		}

		if periodMs > 0 {
			targetSess.schedulePlayerPeriodicTick(aura, periodMs)
		}
		if durationMs > 0 && durationMs < 18000000 {
			targetSess.castMu.Lock()
			aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
				targetSess.expirePlayerAura(spell.ID)
			})
			targetSess.castMu.Unlock()
		}
		return
	}

	// Target is a creature in the world
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}
	targetKey := creatureAuraKeyForTarget(target)

	if s.server == nil || s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, targetGUID) {
		return
	}
	s.server.auraMu.Lock()
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
	}
	if s.server.creatureAuras[targetKey] == nil {
		s.server.creatureAuras[targetKey] = make(map[uint32]struct{})
	}
	s.server.creatureAuras[targetKey][spell.ID] = struct{}{}

	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[creatureAuraKey]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[targetKey] == nil {
		s.server.activeCreatureAuras[targetKey] = make(map[uint32]*activeAura)
	}
	if existing, exists := s.server.activeCreatureAuras[targetKey][spell.ID]; exists && existing != nil && !existing.Stopped {
		// Same ModStackAmount(+1) merge as the player path above
		// (Unit.cpp:3326, SpellAuras.cpp:1030). Per Spell::DoSpellEffectHit
		// (Spell.cpp:2842) only the first aura effect per (cast, target)
		// merges — castMerged carries the first-merge marker, later effects
		// only refresh this effect's basepoints/amounts.
		firstMerge := true
		if castMerged != nil {
			if _, ok := castMerged[targetGUID]; ok {
				firstMerge = false
			} else {
				castMerged[targetGUID] = struct{}{}
			}
		}
		if !firstMerge {
			refreshAuraEffectBasepoints(existing, spell, eff, amount)
			s.server.auraMu.Unlock()
			return
		}
		maxStack := int32(spell.StackAmount)
		if maxStack == 0 {
			maxStack = 1
		}
		cur := int32(existing.StackCount)
		if cur == 0 {
			cur = 1
		}
		if cur++; cur > maxStack {
			cur = maxStack
		}
		stackCount := uint8(cur)
		existing.StackCount = stackCount
		existing.RemainingCharges = uint8(spell.ProcCharges)
		// Creature-side mirror of the player merge's basepoint update
		// (Unit.cpp:3360-3372, Aura::SetStackAmount, SpellAuras.cpp:1008).
		refreshAuraEffectBasepoints(existing, spell, eff, amount)
		existing.DurationMs = durationMs
		existing.RemainingMs = durationMs
		existing.DurationUpdatedAt = time.Now()
		// Creature-side mirror of the player merge above: a triggered re-cast
		// keeps the periodic tick countdown (Spell.cpp:2854 —
		// TRIGGERED_DONT_RESET_PERIODIC_TIMER rides TRIGGERED_FULL_MASK on
		// the Go triggered-cast funnel too).
		resetPeriodic := spell.StackAmount < 2 && s.triggeredNoProcEvents == 0
		if existing.Timer != nil {
			existing.Timer.Stop()
			existing.Timer = nil
		}
		if resetPeriodic {
			if existing.TickTimer != nil {
				existing.TickTimer.Stop()
				existing.TickTimer = nil
			}
			existing.PeriodMs = periodMs
		}
		slot, effectMask := existing.Slot, existing.EffectMask
		s.server.auraMu.Unlock()
		if resetPeriodic && periodMs > 0 {
			s.scheduleCreaturePeriodicTick(existing, periodMs)
		}
		if durationMs > 0 && durationMs < 18000000 {
			s.server.auraMu.Lock()
			existing.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
				s.expireCreatureAura(targetKey, spell.ID, slot)
			})
			s.server.auraMu.Unlock()
		}
		wireStack := stackCount
		if spell.StackAmount == 0 && spell.ProcCharges > 0 {
			wireStack = uint8(spell.ProcCharges)
		}
		wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, wireStack, effectMask)
		_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, s)
		return
	}
	if existing, exists := s.server.activeCreatureAuras[targetKey][spell.ID]; exists && existing != nil {
		existing.Stopped = true
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.TickTimer != nil {
			existing.TickTimer.Stop()
		}
	}

	// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
	// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
	// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
	// removes it before the no-stack purge). The strictly-higher purges
	// are applied below with the other no-stack purges.
	highest := s.server.exclusiveHighestVerdict(spell, eff, int32(amount), targetGUID, s.server.activeCreatureAuras[targetKey])
	if highest.suppressed {
		s.server.auraMu.Unlock()
		return
	}

	slot := uint8(len(s.server.activeCreatureAuras[targetKey]) % 64)
	aura := &activeAura{
		SpellID:           spell.ID,
		DispelType:        spell.DispelType,
		Mechanic:          spell.Mechanic,
		AuraType:          eff.Aura,
		EffectMask:        spellEffectMask(spell, eff),
		CasterGUID:        s.playerGUID,
		TargetGUID:        targetGUID,
		TargetKey:         targetKey,
		SchoolMask:        schoolMask,
		MiscValue:         eff.MiscValue,
		Amount:            amount,
		DurationMs:        durationMs,
		PeriodMs:          periodMs,
		RemainingMs:       durationMs,
		DurationUpdatedAt: time.Now(),
		Slot:              slot,
		Positive:          positive,
		CasterLevel:       s.player.Level,
		SingleTarget:      isSingleTargetAuraSpell(spell),
		TriggerSpell:      eff.TriggerSpell,
		StackAmount:       spell.StackAmount,
		HideDuration:      spell.AttributesEx5&spellAttr5HideDuration != 0,
		RemainingCharges:  uint8(spell.ProcCharges),
	}
	s.server.activeCreatureAuras[targetKey][spell.ID] = aura
	// Unit::_AddAura single-target dance (Unit.cpp:3397-3420): a fresh
	// single-target aura registers on its caster's single-cast list and
	// purges the caster's other single-target-with auras on other targets.
	// Steal-created auras skip registration (Unit.cpp:4028).
	var scPurge []singleCastEntry
	if !skipSingleCastReg && spellIsSingleTarget(spell) {
		scPurge = s.server.registerSingleCastAura(spell, aura)
	}
	// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): the fresh aura
	// purges auras of other spells it can't stack with — the rank-chain
	// term (Aura::CanStackWith, SpellAuras.cpp:1994-2004), the spell-group
	// exclusive terms (SpellAuras.cpp:1924-1932), the spell-specific
	// exclusivity gates (SpellAuras.cpp:1914-1921), and the EXCLUSIVE_HIGHEST
	// comparisons (Unit.cpp:13991) — plus the _AddAura single-target dance
	// (Unit.cpp:3397-3420) for single-target auras. The purge keys on the
	// new aura's caster — the casterGUID param — because the
	// dynamic-object path creates the aura with the object's caster
	// (Unit.cpp:4020 shape).
	purge := s.server.rankChainNoStackPurge(spell, casterGUID, aura.ItemGUID, s.server.activeCreatureAuras[targetKey])
	purge = append(purge, s.server.spellGroupNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.spellSpecificNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.singleTargetNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, highest.purge...)
	s.server.auraMu.Unlock()
	for _, p := range purge {
		s.expireCreatureAura(targetKey, p.spellID, p.slot)
	}
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}
	if eff.Aura == spellAuraCharm {
		spells, reactState, commandState, controlled := s.server.charmCreature(ctx, targetKey, s.playerGUID, s.player.Race)
		if controlled {
			s.sendClientControl(targetGUID, true)
			s.sendCharmPetSpells(targetGUID, spells, reactState, commandState)
		}
	}

	stackCount := uint8(1)
	if spell.StackAmount == 0 && spell.ProcCharges > 0 {
		stackCount = uint8(spell.ProcCharges)
	}
	wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
	updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, stackCount, spellEffectMask(spell, eff))
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, s)

	if periodMs > 0 {
		s.scheduleCreaturePeriodicTick(aura, periodMs)
	}
	if durationMs > 0 && durationMs < 18000000 {
		s.server.auraMu.Lock()
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.expireCreatureAura(targetKey, spell.ID, slot)
		})
		s.server.auraMu.Unlock()
	}
}

func affectsPlayerVisibility(auraType uint32) bool {
	switch auraType {
	case spellAuraStealth, spellAuraInvisibility, spellAuraStealthDetect, spellAuraInvisibilityDetect, spellAuraStealthLevel:
		return true
	default:
		return false
	}
}

func AffectsPlayerVisibility(auraType uint32) bool {
	return affectsPlayerVisibility(auraType)
}

func (ts *session) schedulePlayerPeriodicTick(aura *activeAura, periodMs uint32) {
	ts.castMu.Lock()
	ts.schedulePlayerPeriodicTickLocked(aura, periodMs)
	ts.castMu.Unlock()
}

func (ts *session) schedulePlayerPeriodicTickLocked(aura *activeAura, periodMs uint32) {
	aura.TickTimer = time.AfterFunc(time.Duration(periodMs)*time.Millisecond, func() {
		ts.castMu.Lock()
		if aura.Stopped || ts.player == nil || ts.player.Health == 0 {
			ts.castMu.Unlock()
			return
		}
		advanceAuraDuration(aura, time.Now())
		stillRunning := aura.RemainingMs > 0 || aura.DurationMs == 0
		ts.castMu.Unlock()

		ts.executePeriodicTickOnPlayer(aura)

		if stillRunning {
			ts.castMu.Lock()
			if !aura.Stopped {
				ts.schedulePlayerPeriodicTickLocked(aura, periodMs)
			}
			ts.castMu.Unlock()
		}
	})
}

func (ts *session) executePeriodicTickOnPlayer(aura *activeAura) {
	if aura.AuraType == 23 && aura.TriggerSpell != 0 {
		ts.castSpellDirect(context.Background(), aura.TriggerSpell, ts.periodicTriggerTarget(aura))
		return
	}
	ts.playerStateMu.Lock()
	defer ts.playerStateMu.Unlock()
	if ts.player == nil || ts.player.Health == 0 {
		return
	}

	switch aura.AuraType {
	case 3, 89: // SPELL_AURA_PERIODIC_DAMAGE, SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		dmg := aura.Amount
		resisted := uint32(0)
		if aura.SchoolMask&1 != 0 && ts.player.Armor > 0 {
			dmg = calcArmorReducedDamage(float64(ts.player.Armor), aura.CasterLevel, dmg)
		} else if aura.SchoolMask > 1 && aura.CasterLevel > 0 {
			pen := uint32(0)
			if ts.server != nil {
				if cs := ts.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), ts.player.Resistances, aura.CasterLevel, ts.player.Level, false, false, pen)
		}
		if aura.CasterGUID != aura.TargetGUID {
			ts.applyResilienceToDamage(true, &dmg, false, CombatRatingCritTakenSpell)
		}
		if dmg < 1 && resisted == 0 {
			dmg = 1
		}
		absorbed := uint32(0)
		if dmg > 0 {
			absorbed, dmg = ts.applyAbsorptionShields(dmg, uint8(aura.SchoolMask))
		}
		targetHealth := ts.player.Health
		overkill := uint32(0)
		if dmg >= targetHealth && targetHealth > 0 {
			overkill = dmg - targetHealth
		}

		logPkt := protocol.BuildPeriodicAuraLogDamage(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, dmg, overkill, aura.SchoolMask, absorbed, resisted, false)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}

		if dmg >= targetHealth {
			if ts.duelPartner != 0 && ts.player.DuelTeam != 0 {
				ts.player.Health = 1
				ts.sendPlayerUpdate()
				if ts.server != nil {
					if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil {
						casterSess.endDuel(true, casterSess.playerGUID, false)
					}
				}
			} else {
				ts.player.Health = 0
				ts.sendPlayerUpdate()
				ts.killPlayer(context.Background())
			}
			ts.clearActiveAuras()
		} else {
			ts.player.Health -= dmg
			ts.procDamageAuras(false, dmg)
			ts.sendPlayerUpdate()
		}

	case 8, 20: // SPELL_AURA_PERIODIC_HEAL, SPELL_AURA_OBS_MOD_HEALTH
		heal := aura.Amount
		curHP := ts.player.Health
		maxHP := ts.player.MaxHealth
		newHP := curHP + heal
		overheal := uint32(0)
		if newHP > maxHP {
			overheal = newHP - maxHP
			newHP = maxHP
		}

		logPkt := protocol.BuildPeriodicAuraLogHeal(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, heal, overheal, 0, false)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}

		ts.player.Health = newHP
		ts.sendPlayerUpdate()

		if ts.server != nil && heal > overheal {
			ts.server.distributeHealingThreat(context.Background(), aura.CasterGUID, aura.TargetGUID, heal-overheal)
		}

	case 24: // SPELL_AURA_PERIODIC_ENERGIZE
		powerType := uint32(aura.MiscValue)
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, powerType, aura.Amount)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}
		ts.adjustSpellPower(context.Background(), aura.TargetGUID, aura.MiscValue, int64(aura.Amount))
	}
}

func (s *session) channelTargetForSpell(spellID uint32) uint64 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if channel := s.activeChannel; channel != nil && !channel.Stopped && channel.SpellID == spellID {
		return channel.TargetGUID
	}
	return 0
}

func (s *session) periodicTriggerTarget(aura *activeAura) uint64 {
	if aura == nil {
		return 0
	}
	if aura.ChannelTargetGUID != 0 {
		return aura.ChannelTargetGUID
	}
	if targetGUID := s.channelTargetForSpell(aura.SpellID); targetGUID != 0 {
		return targetGUID
	}
	return aura.TargetGUID
}

func (ts *session) expirePlayerAura(spellID uint32) {
	ts.castMu.Lock()
	if ts.activeAuras != nil {
		if aura, ok := ts.activeAuras[spellID]; ok && aura != nil {
			aura.Stopped = true
			if aura.TickTimer != nil {
				aura.TickTimer.Stop()
			}
			delete(ts.activeAuras, spellID)
			ts.server.unregisterSingleCastAura(aura)
		}
	}
	ts.castMu.Unlock()
	ts.removeAura(spellID)
}

func (s *session) scheduleCreaturePeriodicTick(aura *activeAura, periodMs uint32) {
	if s.server == nil {
		return
	}
	s.server.auraMu.Lock()
	s.scheduleCreaturePeriodicTickLocked(aura, periodMs)
	s.server.auraMu.Unlock()
}

func (s *session) scheduleCreaturePeriodicTickLocked(aura *activeAura, periodMs uint32) {
	aura.TickTimer = time.AfterFunc(time.Duration(periodMs)*time.Millisecond, func() {
		if s.server == nil {
			return
		}
		s.server.auraMu.Lock()
		if aura.Stopped {
			s.server.auraMu.Unlock()
			return
		}
		advanceAuraDuration(aura, time.Now())
		stillRunning := aura.RemainingMs > 0 || aura.DurationMs == 0
		s.server.auraMu.Unlock()

		targetAlive := s.executePeriodicTickOnCreature(aura)
		if !targetAlive {
			return
		}

		if stillRunning {
			s.server.auraMu.Lock()
			if !aura.Stopped {
				s.scheduleCreaturePeriodicTickLocked(aura, periodMs)
			}
			s.server.auraMu.Unlock()
		}
	})
}

func (s *session) executePeriodicTickOnCreature(aura *activeAura) bool {
	ctx := context.Background()
	key := aura.TargetKey
	target, ok := s.getCombatTarget(ctx, aura.TargetGUID)
	if !ok || target.Map != key.Map || target.InstanceID != key.InstanceID || target.Health == 0 || (s.server != nil && s.server.isCreatureEvadingInInstance(key.Map, key.InstanceID, key.GUID)) {
		if s.server != nil {
			s.server.clearCreatureAuras(key)
		}
		return false
	}

	switch aura.AuraType {
	case 3, 89: // SPELL_AURA_PERIODIC_DAMAGE, SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		dmg := aura.Amount
		resisted := uint32(0)
		if aura.SchoolMask&1 != 0 && target.Armor > 0 {
			dmg = calcArmorReducedDamage(float64(target.Armor), aura.CasterLevel, dmg)
		} else if aura.SchoolMask > 1 && aura.CasterLevel > 0 {
			pen := uint32(0)
			if s.server != nil {
				if cs := s.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), target.Resistances, aura.CasterLevel, target.Level, true, false, pen)
		}
		if dmg < 1 && resisted == 0 {
			dmg = 1
		}
		targetHealth := target.Health
		overkill := uint32(0)
		if dmg >= targetHealth && targetHealth > 0 {
			overkill = dmg - targetHealth
		}

		logPkt := protocol.BuildPeriodicAuraLogDamage(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, dmg, overkill, aura.SchoolMask, 0, resisted, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}

		if dmg >= targetHealth {
			// Target slain by DoT
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
				if motion != nil {
					s.server.clearInstanceEncounter(motion)
					motion.Health = 0
					motion.DynamicFlags |= unitDynFlagLootable
					motion.InCombat = false
					motion.TargetGUID = 0
					motion.Moving = false
					if motion.ThreatMgr != nil {
						motion.ThreatMgr.ClearThreat()
					}
				}
				s.server.motionMu.Unlock()

				s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
				s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
					unitFieldHealth:       0,
					unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
				})
				s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
				s.server.clearCreatureAuras(key)
			}
			_ = s.sendAttackStop(target.GUID, true)
			s.attackTarget = 0
			s.onCreatureKilled(ctx, target)
			return false
		} else {
			newHealth := targetHealth - dmg
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
				if motion != nil {
					motion.Health = newHealth
					motion.InCombat = true
					if motion.ThreatMgr == nil {
						motion.ThreatMgr = NewThreatManager(target.GUID)
					}
					dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
					inMelee := dist <= meleeAttackRange
					motion.ThreatMgr.AddThreat(s.playerGUID, float32(dmg), inMelee)
					motion.Moving = true
				}
				s.server.motionMu.Unlock()
				s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
				s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
			}
			return true
		}

	case 8, 20: // SPELL_AURA_PERIODIC_HEAL, SPELL_AURA_OBS_MOD_HEALTH
		heal := aura.Amount
		curHP := target.Health
		maxHP := target.MaxHealth
		newHP := curHP + heal
		overheal := uint32(0)
		if newHP > maxHP {
			overheal = newHP - maxHP
			newHP = maxHP
		}

		logPkt := protocol.BuildPeriodicAuraLogHeal(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, heal, overheal, 0, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
			s.server.motionMu.Lock()
			motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, aura.TargetGUID)
			if motion != nil {
				motion.Health = newHP
			}
			s.server.motionMu.Unlock()
			s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHP})
		}
		return true

	case 23: // SPELL_AURA_PERIODIC_TRIGGER_SPELL
		if aura.TriggerSpell != 0 && s.server != nil && s.server.Data != nil {
			if trigger, found, err := s.server.Data.Spell(aura.TriggerSpell); err == nil && found {
				controlledByPlayer := aura.CasterGUID == s.playerGUID
				if !controlledByPlayer && s.server != nil {
					controlledByPlayer = s.server.findSessionByGUID(aura.CasterGUID) != nil
				}
				if damage, ok := creatureSpellDamage(s.server, trigger, uint32(aura.CasterLevel), controlledByPlayer); ok && damage > 0 {
					// C++ HandlePeriodicTriggerSpellAuraTick casts via
					// CastSpellExtraArgs(AuraEffect) = TRIGGERED_FULL_MASK, so
					// TRIGGERED_DISALLOW_PROC_EVENTS applies here too.
					s.triggeredNoProcEvents++
					s.executeSpellDamage(ctx, aura.TargetGUID, aura.TriggerSpell, damage, 0)
					s.triggeredNoProcEvents--
				}
			}
		}
		return true

	case 24: // SPELL_AURA_PERIODIC_ENERGIZE
		powerType := uint32(aura.MiscValue)
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, powerType, aura.Amount)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}
		s.adjustSpellPower(ctx, aura.TargetGUID, aura.MiscValue, int64(aura.Amount))
		return true
	}
	return true
}

func (s *session) expireCreatureAura(key creatureAuraKey, spellID uint32, slot uint8) {
	if s.server == nil {
		return
	}
	wasCharm := false
	charmerGUID := uint64(0)
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras != nil {
		if auras, ok := s.server.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				wasCharm = aura.AuraType == spellAuraCharm
				charmerGUID = aura.CasterGUID
				aura.Stopped = true
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
				s.server.unregisterSingleCastAura(aura)
			}
		}
	}
	if s.server.creatureAuras != nil {
		if auras, ok := s.server.creatureAuras[key]; ok {
			delete(auras, spellID)
		}
	}
	s.server.auraMu.Unlock()
	if wasCharm {
		s.server.uncharmCreature(key, charmerGUID)
		if charmer := s.server.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}

	removePkt := protocol.BuildAuraUpdate(key.GUID, s.playerGUID, slot, 0, true, false, 0, 0, 1)
	s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

func (s *Server) removeCreatureAura(key creatureAuraKey, spellID uint32) {
	if s == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	wasCharm := false
	charmerGUID := uint64(0)
	s.auraMu.Lock()
	var slot uint8
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				wasCharm = aura.AuraType == spellAuraCharm
				charmerGUID = aura.CasterGUID
				aura.Stopped = true
				slot = aura.Slot
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
				s.unregisterSingleCastAura(aura)
			}
		}
	}
	if s.creatureAuras != nil {
		if auras, ok := s.creatureAuras[key]; ok {
			delete(auras, spellID)
		}
	}
	s.auraMu.Unlock()
	if wasCharm {
		s.uncharmCreature(key, charmerGUID)
		if charmer := s.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}

	removePkt := protocol.BuildAuraUpdate(key.GUID, 0, slot, 0, true, false, 0, 0, 1)
	s.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

// handleCancelMountAura processes CMSG_CANCEL_MOUNT_AURA (0x375).
// Reference: WorldSession::HandleCancelMountAuraOpcode (SpellHandler.cpp:544).
func (s *session) handleCancelMountAura(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted {
			s.removeAura(aura.SpellID)
		}
	}
	if s.player.MountDisplayID != 0 {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
		s.sendPlayerDismount()
	}
	return true
}

// handleCancelGrowthAura processes CMSG_CANCEL_GROWTH_AURA (0x29B).
// Reference: WorldSession::HandleCancelGrowthAuraOpcode (SpellHandler.cpp:535).
func (s *session) handleCancelGrowthAura(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.scale != 1.0 {
		s.scale = 1.0
		s.sendPlayerUpdate()
	}
	return true
}

// handleCancelAutoRepeatSpell processes CMSG_CANCEL_AUTO_REPEAT_SPELL (0x26D).
// Reference: WorldSession::HandleCancelAutoRepeatSpellOpcode (SpellHandler.cpp:553) and CombatPackets.h:115.
func (s *session) handleCancelAutoRepeatSpell(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.autoRepeatSpell = 0
	s.autoRepeatTarget = 0
	buf := protocol.NewBuffer(9)
	buf.WritePackedGUID(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	return true
}

// handleCancelTempEnchantment processes CMSG_CANCEL_TEMP_ENCHANTMENT (0x379).
// Reference: WorldSession::HandleCancelTempEnchantmentOpcode (ItemHandler.cpp:1145).
func (s *session) handleCancelTempEnchantment(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}
	_ = slot
	s.sendPlayerUpdate()
	return true
}

// handleCorpseMapPositionQuery processes CMSG_CORPSE_MAP_POSITION_QUERY (0x4B6).
// Reference: WorldSession::HandleCorpseMapPositionQuery (QueryHandler.cpp:317).
func (s *session) handleCorpseMapPositionQuery(payload []byte) bool {
	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // unk

	buf := protocol.NewBuffer(16)
	buf.WriteF32(0)
	buf.WriteF32(0)
	buf.WriteF32(0)
	buf.WriteF32(0)
	return s.write(uint16(protocol.OpcodeSMSG_CORPSE_MAP_POSITION_QUERY_RESPONSE), buf.Bytes(), true) == nil
}

func buildCastFailed(castID uint8, spellID uint32, result uint8) []byte {
	buf := protocol.NewBuffer(6)
	buf.WriteU8(castID)
	buf.WriteU32(spellID)
	buf.WriteU8(result)
	return buf.Bytes()
}

func (s *session) savePlayerSpellCooldowns(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || tx == nil || state == nil || s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_CHAR_SPELL_COOLDOWNS", state.GUID); err != nil {
		return err
	}
	bySpell := make(map[uint32]spellCooldown, len(state.Cooldowns))
	for _, cooldown := range state.Cooldowns {
		bySpell[cooldown.Spell] = cooldown
	}
	spellIDs := make([]uint32, 0, len(bySpell))
	for spellID := range bySpell {
		spellIDs = append(spellIDs, spellID)
	}
	sort.Slice(spellIDs, func(i, j int) bool { return spellIDs[i] < spellIDs[j] })
	now := time.Now().Unix()
	for _, spellID := range spellIDs {
		cooldown := bySpell[spellID]
		if cooldown.End <= now {
			continue
		}
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_CHAR_SPELL_COOLDOWN", state.GUID, cooldown.Spell, cooldown.Item, cooldown.End, cooldown.Category, cooldown.CategoryEnd); err != nil {
			return err
		}
	}
	return nil
}

func buildSpellCooldown(playerGUID uint64, spellID uint32, cooldownDurationMs uint32) []byte {
	buf := protocol.NewBuffer(8 + 1 + 4 + 4)
	buf.WriteU64(playerGUID)
	buf.WriteU8(0) // flags = 0
	buf.WriteU32(spellID)
	buf.WriteU32(cooldownDurationMs)
	return buf.Bytes()
}

// handleFarSight processes CMSG_FAR_SIGHT (0x27A).
// Reference: WorldSession::HandleFarSightOpcode (SpellHandler.cpp).
func (s *session) handleFarSight(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	op := payload[0]
	s.debug("far sight opcode", "account", s.accountName, "op", op)
	return true
}

// handleGetMirrorImageData processes CMSG_GET_MIRRORIMAGE_DATA (0x401).
// Reference: WorldSession::HandleMirrorImageDataRequest (SpellHandler.cpp:635).
func (s *session) handleGetMirrorImageData(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()

	buf := protocol.NewBuffer(68)
	buf.WriteU64(guid)
	buf.WriteU32(0) // displayId
	buf.WriteU8(s.player.Race)
	buf.WriteU8(s.player.Gender)
	buf.WriteU8(s.player.Class)
	buf.WriteU8(s.player.Skin)
	buf.WriteU8(s.player.Face)
	buf.WriteU8(s.player.HairStyle)
	buf.WriteU8(s.player.HairColor)
	buf.WriteU8(s.player.FacialStyle)
	buf.WriteU32(0) // guildId
	for i := 0; i < 11; i++ {
		buf.WriteU32(0) // outfit item displays
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MIRRORIMAGE_DATA), buf.Bytes(), true)
	return true
}

// handleTotemDestroyed processes CMSG_TOTEM_DESTROYED (0x413).
// Reference: WorldSession::HandleTotemDestroyed (SpellHandler.cpp:582).
func (s *session) handleTotemDestroyed(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	slotID := payload[0]
	if slotID >= 4 {
		return true
	}
	s.destroyTotem(slotID)
	s.debug("totem destroyed", "account", s.accountName, "slot", slotID)
	return true
}

// handleSpellClick processes CMSG_SPELLCLICK (0x410).
// Reference: WorldSession::HandleSpellClick (SpellHandler.cpp:616) -> Unit::HandleSpellClick (Unit.cpp:12982).
func (s *session) handleSpellClick(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadU64()
	if err != nil || targetGUID == 0 {
		return true
	}

	npcEntry := uint32((targetGUID >> 24) & 0x00FFFFFF)
	s.debug("spell click", "account", s.accountName, "target", targetGUID, "entry", npcEntry)

	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}

	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_id, cast_flags, user_type FROM npc_spellclick_spells WHERE npc_entry = ?", npcEntry)
	if err != nil {
		return true
	}
	defer rows.Close()

	type spellClick struct {
		spellID   uint32
		castFlags uint8
		userType  uint8
	}
	var clicks []spellClick
	for rows.Next() {
		var sp, cf, ut uint32
		if err := rows.Scan(&sp, &cf, &ut); err == nil && sp > 0 {
			clicks = append(clicks, spellClick{
				spellID:   sp,
				castFlags: uint8(cf),
				userType:  uint8(ut),
			})
		}
	}

	for _, click := range clicks {
		targetUnit := targetGUID
		if click.castFlags&0x02 != 0 { // NPC_CLICK_CAST_TARGET_CLICKER
			targetUnit = s.playerGUID
		}

		if s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(click.spellID); err == nil && found {
				targetData := protocol.SpellTargetData{
					Flags:    protocol.SpellTargetFlagUnitWireMask,
					UnitGUID: targetUnit,
				}
				s.finishSpellCast(ctx, 0, click.spellID, spell, targetData, 0)
			}
		}
	}
	return true
}

// handleTalentWipeConfirm processes MSG_TALENT_WIPE_CONFIRM (0x2AA).
// Reference: WorldSession::HandleTalentWipeConfirmOpcode (SpellHandler.cpp:732).
func (s *session) handleTalentWipeConfirm(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	wipeGUID, _ := r.ReadU64()

	// Clear player talents and unlearn all talent spells
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		rows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
		if err == nil {
			var unlearnSpells []uint32
			for rows.Next() {
				var sp int64
				if rows.Scan(&sp) == nil && sp > 0 {
					unlearnSpells = append(unlearnSpells, uint32(sp))
				}
			}
			rows.Close()
			for _, sp := range unlearnSpells {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, sp)
				if s.hasAura(sp) {
					s.removeAura(sp)
				}
				s.removeOwnerPetAurasForSpell(ctx, sp)
				unlearnBuf := protocol.NewBuffer(4)
				unlearnBuf.WriteU32(sp)
				_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), unlearnBuf.Bytes(), true)
			}
		}
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
	}
	s.player.Talents = make(map[uint32]uint8)

	buf := protocol.NewBuffer(12)
	buf.WriteU64(wipeGUID)
	buf.WriteU32(0) // free or cost
	_ = s.write(uint16(protocol.OpcodeMSG_TALENT_WIPE_CONFIRM), buf.Bytes(), true)

	// Cast visual untalent effect 14867 from trainer to player
	castPkt := protocol.NewBuffer(16)
	castPkt.WritePackedGUID(wipeGUID)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WriteU8(1)
	castPkt.WriteU32(14867)
	castPkt.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), true)
	_ = s.sendTalentsInfo(false)
	s.sendPlayerUpdate()
	return true
}

const PlayerExtraHas310Flyer uint32 = 0x0040

type LearnedMountSpell struct {
	ID                 uint32
	MountedFlightSpeed int
}

type MountState struct {
	extraFlags uint32
	spells     map[uint32]LearnedMountSpell
}

func NewMountState(extraFlags uint32, spells []LearnedMountSpell) *MountState {
	state := &MountState{extraFlags: extraFlags, spells: make(map[uint32]LearnedMountSpell, len(spells))}
	for _, spell := range spells {
		state.spells[spell.ID] = spell
	}
	return state
}

func (s *MountState) ExtraFlags() uint32 {
	return s.extraFlags
}

func (s *MountState) Has310Flyer(checkAllSpells bool, excludeSpellID uint32) bool {
	if !checkAllSpells {
		return s.extraFlags&PlayerExtraHas310Flyer != 0
	}
	s.extraFlags &^= PlayerExtraHas310Flyer
	for _, spell := range s.spells {
		if spell.ID != excludeSpellID && spell.MountedFlightSpeed == 310 {
			s.extraFlags |= PlayerExtraHas310Flyer
			return true
		}
	}
	return false
}

func (s *MountState) SetHas310Flyer(enabled bool) {
	if enabled {
		s.extraFlags |= PlayerExtraHas310Flyer
	} else {
		s.extraFlags &^= PlayerExtraHas310Flyer
	}
}

func (s *MountState) LearnSpell(spell LearnedMountSpell) {
	s.spells[spell.ID] = spell
	if spell.MountedFlightSpeed == 310 {
		s.SetHas310Flyer(true)
	}
}

func (s *MountState) UnlearnSpell(id uint32) bool {
	if _, ok := s.spells[id]; !ok {
		return s.Has310Flyer(false, 0)
	}
	delete(s.spells, id)
	return s.Has310Flyer(true, 0)
}

func (s *MountState) PreferredFlightSpeed(canFly bool) int {
	if !canFly {
		return 0
	}
	if s.Has310Flyer(false, 0) {
		return 310
	}
	return 280
}

// handleRemoveGlyph processes CMSG_REMOVE_GLYPH (0x48A).
// Reference: WorldSession::HandleRemoveGlyph (SpellHandler.cpp:840): read the
// slot index, and when a glyph is socketed there clear it, persist the change,
// and resend the talent panel so the client drops the glyph. The reference
// also removes the glyph's granted aura; the Go server has no aura engine yet.
func (s *session) handleRemoveGlyph(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}
	if slot >= 6 {
		return true
	}
	spec := s.player.ActiveTalentGroup
	if spec >= 2 {
		spec = 0
	}
	s.player.Glyphs[spec][slot] = 0

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		col := fmt.Sprintf("glyph%d", slot+1)
		_, _ = cdb.ExecContext(ctx, fmt.Sprintf("UPDATE character_glyphs SET %s = 0 WHERE guid = ? AND talentGroup = ?", col), s.playerGUID, spec)
	}
	_ = s.sendTalentsInfo(false)
	return true
}

// applyGlyph sockets a glyph into the specified slot for the active talent group.
// Reference: Spell::EffectApplyGlyph (SpellEffects.cpp:4018).
func (s *session) applyGlyph(ctx context.Context, slot uint8, glyphPropID uint16) {
	if s.player == nil || slot >= 6 || glyphPropID == 0 {
		return
	}
	spec := s.player.ActiveTalentGroup
	if spec >= 2 {
		spec = 0
	}
	s.player.Glyphs[spec][slot] = glyphPropID

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		col := fmt.Sprintf("glyph%d", slot+1)
		_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO character_glyphs (guid, talentGroup, glyph1, glyph2, glyph3, glyph4, glyph5, glyph6) VALUES (?, ?, 0, 0, 0, 0, 0, 0)", s.playerGUID, spec)
		_, _ = cdb.ExecContext(ctx, fmt.Sprintf("UPDATE character_glyphs SET %s = ? WHERE guid = ? AND talentGroup = ?", col), glyphPropID, s.playerGUID, spec)
	}
	_ = s.sendTalentsInfo(false)
}

// handleUpdateMissileTrajectory processes CMSG_UPDATE_MISSILE_TRAJECTORY (0x462).
// Reference: WorldSession::HandleUpdateMissileTrajectory (MiscHandler.cpp:1545).
func (s *session) handleUpdateMissileTrajectory(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 45 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	spellID, _ := r.ReadU32()
	elevation, _ := r.ReadF32()
	speed, _ := r.ReadF32()
	fireX, _ := r.ReadF32()
	fireY, _ := r.ReadF32()
	fireZ, _ := r.ReadF32()
	impactX, _ := r.ReadF32()
	impactY, _ := r.ReadF32()
	impactZ, _ := r.ReadF32()
	moveStop, _ := r.ReadU8()

	s.debug("update missile trajectory", "account", s.accountName, "guid", guid, "spell", spellID, "elevation", elevation, "speed", speed, "fire", []float32{fireX, fireY, fireZ}, "impact", []float32{impactX, impactY, impactZ}, "moveStop", moveStop)

	if moveStop != 0 && r.Remaining() >= 4 {
		opcode, _ := r.ReadU32()
		s.handleMovement(ctx, opcode, payload[r.Position():])
	}
	return true
}

// handleUpdateProjectilePosition processes CMSG_UPDATE_PROJECTILE_POSITION (0x4BE).
// Reference: WorldSession::HandleUpdateProjectilePosition (SpellHandler.cpp:816).
func (s *session) handleUpdateProjectilePosition(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 25 {
		return true
	}
	r := protocol.NewReader(payload)
	casterGuid, _ := r.ReadU64()
	spellID, _ := r.ReadU32()
	castCount, _ := r.ReadU8()
	hitX, _ := r.ReadF32()
	hitY, _ := r.ReadF32()
	hitZ, _ := r.ReadF32()

	s.debug("update projectile position", "account", s.accountName, "guid", casterGuid, "spell", spellID, "castCount", castCount, "hit", []float32{hitX, hitY, hitZ})
	return true
}

func (s *session) sendActionButtons() {
	if s.player == nil {
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ACTION_BUTTONS), buildActionButtons(s.player.Actions), true)
}

// activateSpec mirrors Player::ActivateSpec (Player.cpp:26292).
func (s *session) activateSpec(ctx context.Context, targetSpec uint8) {
	if s.player == nil || targetSpec >= s.player.TalentGroupsCount || targetSpec == s.player.ActiveTalentGroup {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB

	// 1. Unlearn talents of current spec
	rows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
	if err == nil {
		var oldSpells []uint32
		for rows.Next() {
			var sp int64
			if rows.Scan(&sp) == nil && sp > 0 {
				oldSpells = append(oldSpells, uint32(sp))
			}
		}
		rows.Close()
		for _, sp := range oldSpells {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, sp)
			if s.hasAura(sp) {
				s.removeAura(sp)
			}
			s.removeOwnerPetAurasForSpell(ctx, sp)
			unlearnBuf := protocol.NewBuffer(4)
			unlearnBuf.WriteU32(sp)
			_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), unlearnBuf.Bytes(), true)
		}
	}

	// 2. Set new active talent group
	s.player.ActiveTalentGroup = targetSpec
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET activeTalentGroup = ? WHERE guid = ?", targetSpec, s.playerGUID)

	// 3. Load talents for new spec
	s.loadPlayerTalents(ctx, s.player)

	// 4. Teach spells for new spec
	var newSpells []uint32
	tRows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, targetSpec)
	if err == nil {
		for tRows.Next() {
			var sp int64
			if tRows.Scan(&sp) == nil && sp > 0 {
				newSpells = append(newSpells, uint32(sp))
			}
		}
		tRows.Close()
		for _, sp := range newSpells {
			_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", s.playerGUID, sp)
			if !hasLearnedSpell(s.player.Spells, sp) {
				s.player.Spells = append(s.player.Spells, learnedSpell{ID: sp, Active: true})
			}
			learnBuf := protocol.NewBuffer(6)
			learnBuf.WriteU32(sp)
			learnBuf.WriteU16(0)
			_ = s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), learnBuf.Bytes(), true)
		}
	}

	// 5. Load and send action buttons for new spec
	if actions, err := s.loadActionButtons(ctx, s.playerGUID, s.player.Spells); err == nil {
		s.player.Actions = actions
		s.sendActionButtons()
	}

	// 6. Send talents info update
	_ = s.sendTalentsInfo(false)
}

// Channel and pushback state machine, mirroring the reference:
//   - Spell::handle_immediate / SendChannelStart (Spell.cpp:3568/4661):
//     channeled spells (SPELL_ATTR1_CHANNELED_1 0x04 | CHANNELED_2 0x40 in
//     AttributesEx field 6) start a channel after SPELL_GO, announced with
//     MSG_CHANNEL_START (packed caster GUID, spell id, duration) and periodic
//     effect ticks every EffectAuraPeriod milliseconds.
//   - Spell::Delayed (Spell.cpp:7250): a player taking damage while casting a
//     spell with SPELL_INTERRUPT_FLAG_PUSH_BACK 0x02 loses 500ms per hit, at
//     most twice per cast, clamped to the remaining cast time, announced via
//     SMSG_SPELL_DELAYED (packed caster GUID, delay).
//   - Spell::DelayedChannel (Spell.cpp:7290): a channeling player taking
//     damage on a spell with CHANNEL_FLAG_DELAY 0x4000 loses 25% of the total
//     channel duration per hit, at most twice, announced via MSG_CHANNEL_UPDATE.
//   - Movement interrupts: movement flags cancel the active cast and channel
//     (movement.go), matching reference movement-cast interruption.

const (
	spellAttr1Channeled1     uint32 = 0x04
	spellAttr1Channeled2     uint32 = 0x40
	spellInterruptPushBack   uint32 = 0x02
	spellInterruptAbortOnDmg uint32 = 0x10
	channelFlagDelay         uint32 = 0x4000
	maxSpellPushbacks        int    = 2
	defaultCastPushbackMs    uint32 = 500
)

// activeChannelState tracks one running channeled spell.
type activeChannelState struct {
	CastID     uint8
	SpellID    uint32
	TargetGUID uint64
	TargetKey  creatureAuraKey
	Spell      wotlk.Spell
	DurationMs uint32
	Remaining  time.Duration
	PeriodMs   uint32
	Pushbacks  int
	Timer      *time.Timer
	TickTimer  *time.Timer
	DrainTimer *time.Timer
	Stopped    bool
	// HasDest/DestX/DestY/DestZ record the channeled spell's destination
	// (C++ SpellCastTargets::HasDst on the channeled Spell::m_targets).
	// Spell::SelectImplicitChannelTargets reads it for
	// TARGET_DEST_CHANNEL_TARGET (Spell.cpp:1010).
	HasDest bool
	DestX   float32
	DestY   float32
	DestZ   float32
}

func isChanneledSpell(spell wotlk.Spell) bool {
	return spell.AttributesEx&(spellAttr1Channeled1|spellAttr1Channeled2) != 0
}

// spellAllowsDeadTarget mirrors SpellInfo::IsAllowingDeadTarget (SpellInfo.cpp:1177):
// ATTR2_CAN_TARGET_DEAD or a corpse/dead-unit bit in the DBC Targets mask (Spell.dbc field 16).
func spellAllowsDeadTarget(spell wotlk.Spell) bool {
	return spell.AttributesEx1&spellAttr2CanTargetDead != 0 ||
		spell.Targets&(targetFlagCorpseEnemy|targetFlagUnitDead|targetFlagCorpseAlly) != 0
}

// sendChannelUpdate mirrors Spell::SendChannelUpdate: packed caster GUID plus
// remaining channel milliseconds; zero clears the client channel bar.
func (s *session) sendChannelUpdate(remainingMs uint32) {
	packet := protocol.NewBuffer(12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(remainingMs)
	_ = s.write(uint16(protocol.OpcodeMSG_CHANNEL_UPDATE), packet.Bytes(), true)
	if remainingMs == 0 {
		s.sendPlayerUpdate()
	}
}

// startChannel begins the channeled phase of a finished cast: broadcast the
// channel start, schedule periodic ticks, and arm completion. hasDest/dest*
// record the cast's destination (SpellCastTargets::HasDst analog) so
// SelectImplicitChannelTargets parity (Spell.cpp:1010) can resolve
// TARGET_DEST_CHANNEL_TARGET for spells triggered during the channel.
func (s *session) startChannel(castID uint8, spellID uint32, spell wotlk.Spell, targetGUID uint64, hasDest bool, destX, destY, destZ float32) {
	if s.player == nil || s.server.Data == nil {
		return
	}
	var durationMs int32 = 0
	if value, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level)); err == nil && ok {
		durationMs = value
	}
	if durationMs <= 0 {
		return // instant or infinite channels have no timed lifecycle here
	}
	period := uint32(0)
	for _, effect := range spell.Effects {
		if effect.Effect != 0 && effect.AuraPeriod > period {
			period = effect.AuraPeriod
		}
	}

	// In WotLK 3.3.5, channeled spells scale with spell haste: duration and tick interval are compressed
	// Mirrors TrinityCore Spell::Prepare (Spell.cpp:650-700):
	hastePct := s.getSpellHastePct()
	if hastePct > 0 {
		durationMs = int32(math.Round(float64(durationMs) / (1.0 + hastePct/100.0)))
		if period > 0 {
			period = uint32(math.Round(float64(period) / (1.0 + hastePct/100.0)))
		}
	}
	channel := &activeChannelState{
		CastID:     castID,
		SpellID:    spellID,
		TargetGUID: targetGUID,
		TargetKey:  creatureAuraKeyForPlayer(*s.player, targetGUID),
		Spell:      spell,
		DurationMs: uint32(durationMs),
		Remaining:  time.Duration(durationMs) * time.Millisecond,
		PeriodMs:   period,
		HasDest:    hasDest,
		DestX:      destX,
		DestY:      destY,
		DestZ:      destZ,
	}
	s.castMu.Lock()
	s.activeChannel = channel
	s.castMu.Unlock()

	packet := protocol.NewBuffer(16)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(spellID)
	packet.WriteU32(uint32(durationMs))
	_ = s.write(uint16(protocol.OpcodeMSG_CHANNEL_START), packet.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_CHANNEL_START), packet.Bytes(), s)
	}
	s.sendPlayerUpdate()

	channel.Timer = time.AfterFunc(channel.Remaining, func() { s.finishChannel() })
	if period > 0 && period <= uint32(durationMs) {
		channel.TickTimer = time.AfterFunc(time.Duration(period)*time.Millisecond, func() { s.channelTick() })
	}
	// SpellAuras.cpp:436: an aura with ManaPerSecond or ManaPerSecondPerLevel
	// drains the caster every second.
	if spell.ManaPerSecond != 0 || spell.ManaPerSecondPerLevel != 0 {
		channel.DrainTimer = time.AfterFunc(time.Second, func() { s.channelDrainTick() })
	}
	s.debug("channel started", "account", s.accountName, "spell", spellID, "duration_ms", durationMs, "period_ms", period)
}

// finishChannel completes the channel: clear state and zero the bar.
func (s *session) finishChannel() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	s.activeChannel = nil
	if channel.Timer != nil {
		channel.Timer.Stop()
	}
	if channel.TickTimer != nil {
		channel.TickTimer.Stop()
	}
	if channel.DrainTimer != nil {
		channel.DrainTimer.Stop()
	}
	channel.Stopped = true
	spellID := channel.SpellID
	s.castMu.Unlock()

	s.sendChannelUpdate(0)
	s.debug("channel finished", "account", s.accountName, "spell", spellID)
}

// interruptCurrentChannel stops the active channel without the completion
// path (movement, new cast, cancel).
func (s *session) interruptCurrentChannel() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	s.activeChannel = nil
	if channel.Timer != nil {
		channel.Timer.Stop()
	}
	if channel.TickTimer != nil {
		channel.TickTimer.Stop()
	}
	if channel.DrainTimer != nil {
		channel.DrainTimer.Stop()
	}
	channel.Stopped = true
	s.castMu.Unlock()

	s.expirePlayerAura(channel.SpellID)
	if s.server != nil && s.player != nil && channel.TargetGUID != 0 {
		if target := s.server.findSessionByGUID(channel.TargetGUID); target != nil {
			if target != s {
				target.expirePlayerAura(channel.SpellID)
			}
		} else {
			key := channel.TargetKey
			if key.GUID == 0 {
				key = creatureAuraKeyForPlayer(*s.player, channel.TargetGUID)
			}
			s.server.auraMu.Lock()
			_, hasAura := s.server.activeCreatureAuras[key][channel.SpellID]
			s.server.auraMu.Unlock()
			if hasAura {
				s.server.removeCreatureAura(key, channel.SpellID)
			}
		}
	}
	s.sendChannelUpdate(0)
}

// channelTick applies one periodic effect tick of the channeled spell and
// schedules the next while the channel is alive.
func (s *session) channelTick() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	next := time.Duration(channel.PeriodMs) * time.Millisecond
	spell := channel.Spell
	targetGUID := channel.TargetGUID
	remaining := channel.Remaining
	s.castMu.Unlock()

	ctx := context.Background()
	for effectIndex, effect := range spell.Effects {
		if effect.Effect == 0 || effect.Effect == 6 && effect.Aura == 23 {
			continue
		}
		amount := uint32(effect.BasePoints + 1)
		if amount == 0 {
			continue
		}
		switch effect.Effect {
		case 2, 87, 108, 17: // damage effects tick on the target
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount, effectIndex)
			}
		case 6, 10, 136, 105: // auras and heals tick on the target or caster
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount, effectIndex)
			} else {
				s.executeSpellHeal(ctx, s.playerGUID, spell.ID, amount, effectIndex)
			}
		}
	}

	s.castMu.Lock()
	channel = s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	remaining -= next
	if remaining > 0 {
		channel.TickTimer = time.AfterFunc(next, func() { s.channelTick() })
		s.castMu.Unlock()
		return
	}
	s.castMu.Unlock()
}

// channelDrainTick applies one second of the channeled spell's mana drain and
// re-arms while the channel lives. Mirrors Aura::Update (SpellAuras.cpp:844):
// manaPerSecond = ManaPerSecond + ManaPerSecondPerLevel * caster level;
// POWER_HEALTH drains health (needs strictly more than the drain), other
// powers need at least the drain, and a shortfall removes the aura (ends the
// channel here).
func (s *session) channelDrainTick() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	spell := channel.Spell
	s.castMu.Unlock()

	if s.player == nil {
		return
	}
	manaPerSecond := spell.ManaPerSecond + spell.ManaPerSecondPerLevel*uint32(s.player.Level)
	if manaPerSecond > 0 {
		if spell.PowerType == 0xFFFFFFFE { // POWER_HEALTH = -2 in C++
			if s.player.Health > manaPerSecond {
				s.player.Health -= manaPerSecond
				s.sendPlayerUpdate()
			} else {
				s.interruptCurrentChannel()
				return
			}
		} else if spell.PowerType < 7 {
			if s.player.Powers[spell.PowerType] >= manaPerSecond {
				s.adjustSpellPower(context.Background(), s.playerGUID, int32(spell.PowerType), -int64(manaPerSecond))
			} else {
				s.interruptCurrentChannel()
				return
			}
		}
	}

	s.castMu.Lock()
	channel = s.activeChannel
	if channel != nil && !channel.Stopped {
		channel.DrainTimer = time.AfterFunc(time.Second, func() { s.channelDrainTick() })
	}
	s.castMu.Unlock()
}

// getPushbackReductionLocked returns total percent pushback reduction from active auras (SPELL_AURA_REDUCE_PUSHBACK = 149).
// Assumes s.castMu is held.
// Reference: TrinityCore Spell::Delayed / Spell::DelayedChannel: delayReduce += playerCaster->GetTotalAuraModifier(SPELL_AURA_REDUCE_PUSHBACK) - 100.
func (s *session) getPushbackReductionLocked() int32 {
	if s == nil {
		return 0
	}
	var reduction int32
	for _, a := range s.activeAuras {
		if a != nil && !a.Stopped && a.AuraType == 149 { // SPELL_AURA_REDUCE_PUSHBACK
			reduction += int32(a.Amount)
		}
	}
	if reduction > 100 {
		reduction = 100
	}
	return reduction
}

func (s *session) getPushbackReduction() int32 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.getPushbackReductionLocked()
}

// delayCurrentCast mirrors Spell::Delayed: called when the player takes
// damage during a timed cast. Requires SPELL_INTERRUPT_FLAG_PUSH_BACK, at
// most two pushbacks per cast, 500ms each clamped to remaining time, and
// announces SMSG_SPELL_DELAYED. Spells with SPELL_INTERRUPT_FLAG_ABORT_ON_DMG
// are aborted entirely on direct damage.
func (s *session) delayCurrentCast() {
	if s.player == nil {
		return
	}
	s.castMu.Lock()
	cast := s.activeCast
	if cast == nil || cast.CastTimeMs == 0 {
		s.castMu.Unlock()
		return
	}
	// Direct damage completely aborts spells with SPELL_INTERRUPT_FLAG_ABORT_ON_DMG (0x10).
	// Reference: TrinityCore Unit.cpp:944-945.
	if cast.InterruptFlg&spellInterruptAbortOnDmg != 0 {
		s.castMu.Unlock()
		s.interruptCurrentCast()
		return
	}
	if cast.InterruptFlg&spellInterruptPushBack == 0 || cast.Pushbacks >= maxSpellPushbacks {
		s.castMu.Unlock()
		return
	}
	reduction := s.getPushbackReductionLocked()
	if reduction >= 100 {
		s.castMu.Unlock()
		return
	}
	elapsed := time.Since(cast.StartAt)
	remaining := time.Duration(cast.CastTimeMs)*time.Millisecond - elapsed
	if remaining <= 0 {
		s.castMu.Unlock()
		return
	}
	delayMs := defaultCastPushbackMs
	if reduction > 0 {
		delayMs = uint32(float64(delayMs) * float64(100-reduction) / 100.0)
	}
	delay := time.Duration(delayMs) * time.Millisecond
	if delay > remaining {
		delay = remaining
	}
	cast.Pushbacks++
	newRemaining := remaining + delay
	cast.StartAt = time.Now().Add(-(time.Duration(cast.CastTimeMs)*time.Millisecond - newRemaining))
	if cast.Timer != nil {
		cast.Timer.Reset(newRemaining)
	}
	pushbacks := cast.Pushbacks
	spellID := cast.SpellID
	s.castMu.Unlock()

	packet := protocol.NewBuffer(8)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(uint32(delay.Milliseconds()))
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_DELAYED), packet.Bytes(), true)
	s.debug("cast pushed back", "account", s.accountName, "spell", spellID, "delay_ms", delay.Milliseconds(), "count", pushbacks)
}

// delayCurrentChannel mirrors Spell::DelayedChannel: called when the player
// takes damage while channeling. Requires CHANNEL_FLAG_DELAY, at most two
// pushbacks, 25% of the total channel duration per hit, announced via
// MSG_CHANNEL_UPDATE with the new remaining time.
func (s *session) delayCurrentChannel() {
	if s.player == nil {
		return
	}
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped || channel.Spell.ChannelInterrupt&channelFlagDelay == 0 || channel.Pushbacks >= maxSpellPushbacks {
		s.castMu.Unlock()
		return
	}
	reduction := s.getPushbackReductionLocked()
	if reduction >= 100 {
		s.castMu.Unlock()
		return
	}
	delayMs := channel.DurationMs / 4 // 25% of total duration per hit
	if reduction > 0 {
		delayMs = uint32(float64(delayMs) * float64(100-reduction) / 100.0)
	}
	if delayMs == 0 {
		s.castMu.Unlock()
		return
	}
	if time.Duration(delayMs)*time.Millisecond >= channel.Remaining {
		delayMs = uint32(channel.Remaining.Milliseconds())
		channel.Remaining = 0
	} else {
		channel.Remaining -= time.Duration(delayMs) * time.Millisecond
	}
	channel.Pushbacks++
	remaining := channel.Remaining
	if channel.Timer != nil {
		channel.Timer.Reset(remaining)
	}
	spellID := channel.SpellID
	s.castMu.Unlock()

	s.sendChannelUpdate(uint32(remaining.Milliseconds()))
	s.debug("channel pushed back", "account", s.accountName, "spell", spellID, "delay_ms", delayMs, "count", channel.Pushbacks)
}

type itemTemplateClassInfo struct {
	Class    uint32
	SubClass uint32
	InvType  uint32
}

func (s *Server) getItemTemplateClassInfo(ctx context.Context, entry uint32) (itemTemplateClassInfo, bool) {
	if entry == 0 {
		return itemTemplateClassInfo{}, false
	}
	s.itemTemplateMu.RLock()
	if s.itemTemplates != nil {
		if info, ok := s.itemTemplates[entry]; ok {
			s.itemTemplateMu.RUnlock()
			return info, true
		}
	}
	s.itemTemplateMu.RUnlock()

	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return itemTemplateClassInfo{}, false
	}

	var class, subclass, invType uint32
	err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT class, subclass, InventoryType FROM item_template WHERE entry = ? LIMIT 1", entry).Scan(&class, &subclass, &invType)
	if err != nil {
		return itemTemplateClassInfo{}, false
	}

	info := itemTemplateClassInfo{Class: class, SubClass: subclass, InvType: invType}
	s.itemTemplateMu.Lock()
	if s.itemTemplates == nil {
		s.itemTemplates = make(map[uint32]itemTemplateClassInfo)
	}
	s.itemTemplates[entry] = info
	s.itemTemplateMu.Unlock()
	return info, true
}

func (s *session) getItemTemplateClassInfo(ctx context.Context, entry uint32) (itemTemplateClassInfo, bool) {
	if s == nil || s.server == nil {
		return itemTemplateClassInfo{}, false
	}
	return s.server.getItemTemplateClassInfo(ctx, entry)
}

// isItemFitToSpell verifies if an item's class, subclass, and inventory type satisfy the spell's requirements.
// Reference: TrinityCore Item::IsFitToSpellRequirements (Item.cpp:799-832).
func isItemFitToSpell(spell wotlk.Spell, class uint32, subclass uint32, invType uint32) bool {
	if spell.EquippedItemClass >= 0 {
		if uint32(spell.EquippedItemClass) != class {
			return false
		}
		if spell.EquippedItemSubClass != 0 {
			if (spell.EquippedItemSubClass & (1 << subclass)) == 0 {
				return false
			}
		}
	}
	if spell.EquippedItemInvTypes != 0 {
		if (spell.EquippedItemInvTypes & (1 << invType)) == 0 {
			return false
		}
	}
	return true
}

// checkSpellEquippedItemRequirements validates equipped weapon and armor requirements for spells before cast execution.
// References: TrinityCore Spell::CheckCast (Spell.cpp:6768-6775, 7206-7237) and Player::HasItemFitToSpellRequirements (Player.cpp:23916-23969).
func (s *session) checkSpellEquippedItemRequirements(ctx context.Context, spell wotlk.Spell) (uint8, bool) {
	if spell.EquippedItemClass < 0 || s == nil || s.player == nil || s.player.Equipment == "" {
		return 0, true
	}

	// 1. Main hand weapon requirement (SPELL_ATTR3_MAIN_HAND)
	if spell.AttributesEx3&spellAttr3MainHand != 0 {
		mainHandEntry := s.getEquipmentItem(equipSlotMainhand)
		if mainHandEntry == 0 {
			return spellFailedEquippedItemClass, false
		}
		if info, ok := s.getItemTemplateClassInfo(ctx, mainHandEntry); ok {
			if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	// 2. Offhand weapon requirement (SPELL_ATTR3_REQ_OFFHAND)
	if spell.AttributesEx3&spellAttr3ReqOffhand != 0 {
		offHandEntry := s.getEquipmentItem(equipSlotOffhand)
		if offHandEntry == 0 {
			return spellFailedEquippedItemClass, false
		}
		if info, ok := s.getItemTemplateClassInfo(ctx, offHandEntry); ok {
			if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	// 3. General item class requirements (ITEM_CLASS_WEAPON, ITEM_CLASS_ARMOR)
	switch spell.EquippedItemClass {
	case itemClassWeapon:
		if spell.AttributesEx3&(spellAttr3MainHand|spellAttr3ReqOffhand) == 0 {
			weaponSlots := []uint8{equipSlotMainhand, equipSlotOffhand, equipSlotRanged}
			hasFitWeapon := false
			for _, slot := range weaponSlots {
				entry := s.getEquipmentItem(slot)
				if entry == 0 {
					continue
				}
				info, ok := s.getItemTemplateClassInfo(ctx, entry)
				if !ok || isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					hasFitWeapon = true
					break
				}
			}
			if !hasFitWeapon {
				return spellFailedEquippedItemClass, false
			}
		}

	case itemClassArmor:
		// Shield requirement (subclass 6 = shield, subclass 5 = buckler)
		if spell.EquippedItemSubClass&((1<<itemSubclassArmorBuckler)|(1<<itemSubclassArmorShield)) != 0 {
			offhandEntry := s.getEquipmentItem(equipSlotOffhand)
			if offhandEntry == 0 {
				return spellFailedEquippedItemClass, false
			}
			if info, ok := s.getItemTemplateClassInfo(ctx, offhandEntry); ok {
				if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					return spellFailedEquippedItemClass, false
				}
			}
		} else {
			hasFitArmor := false
			for slot := uint8(0); slot < equipSlotEnd; slot++ {
				if slot == equipSlotMainhand || slot == equipSlotTabard {
					continue
				}
				entry := s.getEquipmentItem(slot)
				if entry == 0 {
					continue
				}
				info, ok := s.getItemTemplateClassInfo(ctx, entry)
				if !ok || isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					hasFitArmor = true
					break
				}
			}
			if !hasFitArmor {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	return 0, true
}

func (s *session) handleEffectCreateItem(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if eff.ItemType == 0 {
		return
	}
	playerGUID := targetGUID
	if playerGUID == 0 {
		playerGUID = s.playerGUID
	}
	if playerGUID == 0 {
		return
	}
	count := int32(eff.BasePoints + 1)
	if count < 1 {
		count = 1
	}
	if _, err := s.storeOrStackItem(ctx, playerGUID, eff.ItemType, uint32(count)); err != nil {
		return
	}
}

func (s *session) handleEffectResurrect(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if targetGUID == 0 || s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		return
	}
	if targetSess.player.Health > 0 {
		return
	}
	if targetSess.resurrection != nil {
		return
	}
	healthPct := eff.BasePoints + 1
	if healthPct < 1 {
		healthPct = 1
	}
	if healthPct > 100 {
		healthPct = 100
	}
	maxHealth := targetSess.player.MaxHealth
	if maxHealth == 0 {
		maxHealth = 1
	}
	health := uint32(int64(maxHealth) * int64(healthPct) / 100)
	maxMana := targetSess.player.MaxPowers[0]
	mana := uint32(int64(maxMana) * int64(healthPct) / 100)
	targetSess.setResurrectRequestData(s.playerGUID, 0, 0, 0, 0, health, mana)
	targetSess.sendResurrectRequest(s.playerGUID, "", false, false)
}

func (s *session) handleEffectHealthLeech(ctx context.Context, spellID uint32, hitTargets []uint64, effIndex int, eff wotlk.SpellEffect) {
	if s.playerGUID == 0 {
		return
	}
	amount := eff.BasePoints + 1
	if amount < 0 {
		return
	}
	damageAmount := uint32(amount)
	for _, target := range hitTargets {
		if target == 0 || target == s.playerGUID {
			continue
		}
		// SpellEffects.cpp:1548 (-GetHealthGain(-damage)): the leech heal is the
		// damage the target actually lost — post-absorb, overkill excluded —
		// scaled by the effect value multiplier, not the rolled damage.
		healthBefore := uint32(0)
		if tgt, ok := s.getCombatTarget(ctx, target); ok {
			healthBefore = tgt.Health
		}
		dealt := s.executeSpellDamage(ctx, target, spellID, damageAmount, effIndex)
		if dealt > healthBefore {
			dealt = healthBefore
		}
		s.executeSpellHeal(ctx, s.playerGUID, spellID, effectValueMultiplied(dealt, eff.Amplitude), effIndex)
	}
}

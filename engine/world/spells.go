package world

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellAttributePassive uint32 = 0x00000040
	spellCastFlagStart    uint32 = 0x00000002
	spellCastFlagGo       uint32 = 0x00000100

	spellAttr3MainHand   uint32 = 0x00000400 // SPELL_ATTR3_MAIN_HAND: Require main hand weapon (SharedDefines.h:533)
	spellAttr3ReqOffhand uint32 = 0x01000000 // SPELL_ATTR3_REQ_OFFHAND: Require offhand weapon (SharedDefines.h:547)
	spellAttr3ReqWand    uint32 = 0x00400000 // SPELL_ATTR3_REQ_WAND: Requires equipped Wand (SharedDefines.h:545)

	spellFailedEquippedItemClass         uint8 = 29  // SPELL_FAILED_EQUIPPED_ITEM_CLASS (SharedDefines.h:1011)
	spellFailedEquippedItemClassMainhand uint8 = 30  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_MAINHAND (SharedDefines.h:1012)
	spellFailedEquippedItemClassOffhand  uint8 = 31  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_OFFHAND (SharedDefines.h:1013)
	spellFailedNotInFront                uint8 = 61  // SPELL_FAILED_NOT_INFRONT (SharedDefines.h:1042)
	spellFailedBadTargets                uint8 = 12  // SPELL_FAILED_BAD_TARGETS (SharedDefines.h:992)
	spellFailedNotReady                  uint8 = 67  // SPELL_FAILED_NOT_READY (SharedDefines.h:1049)
	spellFailedSilenced                  uint8 = 104 // SPELL_FAILED_SILENCED (SharedDefines.h:1086)
	spellFailedCasterDead                uint8 = 23  // SPELL_FAILED_CASTER_DEAD (SharedDefines.h:1003)
	spellFailedNotFishable               uint8 = 58  // SPELL_FAILED_NOT_FISHABLE (SharedDefines.h:1040)

	itemClassWeapon = 2
	itemClassArmor  = 4

	itemSubclassArmorBuckler = 5
	itemSubclassArmorShield  = 6

	spellEffectEnergize      = 30
	spellEffectPowerBurn     = 62
	spellEffectThreat        = 63
	spellEffectTriggerSpell  = 64
	spellEffectHealMaxHealth = 67
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
	player := playerPos{Map: s.player.Map, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID uint32, x, y, z float32, faction, unitFlags, flagsExtra, health uint32) {
		if mapID != player.Map || health == 0 || creatureCombatDisabled(unitFlags, flagsExtra) || distance3D(x, y, z, centerX, centerY, centerZ) > float64(radius) || !s.server.isHostileFaction(faction, player) {
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
	motions := make([]*creatureMotion, 0, len(s.server.creatureMotion))
	for _, motion := range s.server.creatureMotion {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	for _, motion := range motions {
		accept(motion.GUID, motion.Map, motion.X, motion.Y, motion.Z, motion.Faction, motion.UnitFlags, motion.FlagsExtra, motion.Health)
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == s || !targetSession.authed || !targetSession.playerLoaded || targetSession.player == nil || targetSession.player.Health == 0 || targetSession.player.Map != player.Map || targetSession.playerAlliance() == s.playerAlliance() {
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
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), COALESCE(NULLIF(c.curhealth, 0), NULLIF(t.maxlevel * 30, 0), 1) FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, player.Map, float64(centerX-radius), float64(centerX+radius), float64(centerY-radius), float64(centerY+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					accept(creatureWorldGUID(uint32(low), uint32(entry)), uint32(mapID), float32(x), float32(y), float32(z), uint32(faction), uint32(unitFlags), uint32(flagsExtra), uint32(health))
				}
			}
		}
	}
	return targets
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
		if _, err = reader.ReadF32(); err != nil {
			return false
		}
		if _, err = reader.ReadF32(); err != nil {
			return false
		}
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil {
		s.debug("spell lookup failed", "account", s.accountName, "spell", spellID, "error", err)
		return true
	}
	learned := s.hasActiveSpell(spellID)
	if !found || spell.Attributes&spellAttributePassive != 0 || !learned {
		s.debug("spell cast ignored", "account", s.accountName, "spell", spellID, "reason", spellCastIgnoreReason(spell, found, learned))
		return true
	}
	if s.castInProgress() {
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
	if s.isGCDActive(spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
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
	cost := s.calculateSpellPowerCost(spell)
	pType := spell.PowerType
	if cost > 0 && pType < 7 && s.player.Powers[pType] < cost {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not enough power", "power", s.player.Powers[pType], "cost", cost)
		return true
	}

	if failReason, ok := s.checkSpellEquippedItemRequirements(ctx, spell); !ok {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
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
			// Returns SPELL_FAILED_UNIT_NOT_INFRONT = 81 ("Target needs to be in front of you.").
			if (spell.FacingCasterFlags&SpellFacingFlagInfront != 0) || spellID == 75 || spellID == 5019 {
				if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, tgt.X, tgt.Y, 2.0*math.Pi/3.0) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 81), true) // SPELL_FAILED_UNIT_NOT_INFRONT = 81
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

	// Interrupt any existing spell cast (TC: Unit::InterruptNonMeleeSpells)
	s.interruptCurrentCast()
	s.interruptCurrentChannel()
	s.procCastAuras()
	s.triggerGlobalCooldown(spell)

	s.lastCastTime = time.Now()
	castTime := s.calculateSpellCastTime(spell)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_START), protocol.BuildSpellStart(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagStart, castTime, target), true); err != nil {
		return false
	}

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
			s.finishSpellCast(context.Background(), castID, spellID, spell, target)
		})
		s.activeCast = castState
		s.castMu.Unlock()
	} else {
		s.finishSpellCast(context.Background(), castID, spellID, spell, target)
	}

	s.debug("spell cast accepted", "account", s.accountName, "spell", spellID, "cast_id", castID, "cast_time", castTime, "cost", cost)
	return true
}

func (s *session) castInProgress() bool {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.activeCast != nil && !s.activeCast.Cancelled
}

func (s *session) finishSpellCast(ctx context.Context, castID uint8, spellID uint32, spell wotlk.Spell, target protocol.SpellTargetData) {
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

	hitTargets := make([]uint64, 0, 1)
	if isSelfCastOnly(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		hitTargets = append(hitTargets, target.UnitGUID)
	} else if s.selection != 0 {
		hitTargets = append(hitTargets, s.selection)
	} else if !isHarmfulSpell(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	}
	areaSpell := isAreaEnemySpell(spell)
	if areaSpell {
		hitTargets = s.spellAreaEnemyTargets(ctx, spell, target)
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
				resIndex := schoolMaskToResistanceIndex(uint8(spell.SchoolMask))
				victimRes := uint32(0)
				if targetSess != nil && targetSess.player != nil && resIndex < 7 {
					victimRes = targetSess.player.Resistances[resIndex]
				} else if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok && resIndex < 7 {
					victimRes = tgt.Resistances[resIndex]
				}
				pen := uint32(0)
				if s.player != nil {
					pen = s.player.SpellPenetration
				}
				if checkBinarySpellResist(victimRes, pen, s.player.Level, targetLevel) {
					hitTargets = nil
					missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissResist}}
				}
			}
		}
	} else if !areaSpell && targetGUID != 0 && targetGUID != s.playerGUID && !isHarmfulSpell(spell) {
		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		if targetSess != nil && targetSess.isImmuneToSpell(spell) {
			hitTargets = nil
			missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissImmune}}
		}
	}

	castTimeStamp := uint32(time.Now().UnixMilli())
	pType := spell.PowerType
	cost := s.calculateSpellPowerCost(spell)
	if pType < 7 && cost > 0 {
		if s.player.Powers[pType] >= cost {
			s.player.Powers[pType] -= cost
		} else {
			s.player.Powers[pType] = 0
		}
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
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, missStatus, target), true)
	if isFishingSpell(spellID) {
		s.spawnFishingBobber(ctx, target)
		return
	}

	if len(hitTargets) == 0 {
		// Spell missed, do not trigger channel, cooldown, or effects
		return
	}

	// Reference Spell::handle_immediate: channeled spells begin their timed
	// channel lifecycle after the cast completes.
	if isChanneledSpell(spell) {
		s.startChannel(castID, spellID, spell, targetGUID)
	}
	s.updateAchievementCriteria(criteriaTypeCastSpell, spellID, 1)
	s.updateAchievementCriteria(criteriaTypeCastSpell2, spellID, 1)
	s.startTimedAchievement(timedTypeSpellCast, spellID)
	if spell.RecoveryTime > 0 {
		nowUnix := time.Now().Unix()
		cooldownEnd := nowUnix + int64((spell.RecoveryTime+999)/1000)
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, End: cooldownEnd})
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, uint32(spell.RecoveryTime)), true)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_spell_cooldown (guid, spell, item, time, categoryId, categoryEnd) VALUES (?, ?, 0, ?, 0, 0)", s.playerGUID, spellID, cooldownEnd)
		}
	}
	if spellID == 8690 {
		nowUnix := time.Now().Unix()
		cooldownEnd := nowUnix + 900 // 15 min cooldown
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, End: cooldownEnd})
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, 900000), true)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_spell_cooldown (guid, spell, item, time, categoryId, categoryEnd) VALUES (?, ?, 6948, ?, 0, 0)", s.playerGUID, spellID, cooldownEnd)
		}
	}

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
		interruptHandled := false
		damageEffectSeen := false
		for _, eff := range spell.Effects {
			if eff.Effect == 0 {
				continue
			}
			switch eff.Effect {
			case 2, 17, 31, 58, 87: // Damage effects (School damage, Weapon damage, etc.)
				damageEffectSeen = true
				damage := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					if effectTarget != 0 && (effectTarget != s.playerGUID || isReflected) {
						s.executeSpellDamage(effCtx, effectTarget, spellID, damage)
					}
				}
			case 10, 136, 105: // Heal effects
				heal := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					s.executeSpellHeal(effCtx, effectTarget, spellID, heal)
				}
			case spellEffectEnergize:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					s.applySpellEnergize(effCtx, effectTarget, eff.MiscValue, amount)
				}
			case spellEffectPowerBurn:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					if burned := s.applySpellPowerBurn(effCtx, effectTarget, eff.MiscValue, amount, spellID); burned > 0 {
						s.executeSpellDamage(effCtx, effectTarget, spellID, burned)
					}
				}
			case spellEffectTriggerSpell:
				for _, effectTarget := range hitTargets {
					if eff.TriggerSpell != 0 && eff.TriggerSpell != spellID {
						s.castSpellDirect(effCtx, eff.TriggerSpell, effectTarget)
					}
				}
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
				if durationMs == 0 && eff.Effect == 6 && !isHarmfulAura(eff.Aura) {
					durationMs = 1800000 // 30 min default for passive buffs
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
					s.applyAuraToTarget(effCtx, auraTarget, spell, eff, durationMs, periodMs, amount, schoolMask)
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
				s.handleFeedPet(effCtx, spellID)
			case 102: // SPELL_EFFECT_DISMISS_PET
				s.handleDismissPet(effCtx)
			case 109: // SPELL_EFFECT_RESURRECT_PET
				s.handleResurrectPet(effCtx, spellID)
			case 54: // SPELL_EFFECT_TAMECREATURE
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
			})
			return
		}
	}

	applyEffects(ctx)
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
	object := &dynamicSpellObjectState{GUID: dynamicSpellGUID(lowGUID), CasterGUID: s.playerGUID, SpellID: uint64(spell.ID), Map: s.player.Map, X: x, Y: y, Z: z, Orientation: s.player.Orientation, Radius: radius, CastTime: uint32(time.Now().UnixMilli()), SpellData: spell, AuraEffect: auraEffect, AuraDurationMs: uint32(durationMs), AuraPeriodMs: periodMs, AuraAmount: amount, AuraSchoolMask: schoolMask, NextAuraTick: time.Now().Add(time.Duration(periodMs) * time.Millisecond)}
	s.server.spawnDynamicSpellObject(object, time.Duration(durationMs)*time.Millisecond)
}

func (s *session) executeSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32) {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}

	// Apply Spell Power bonus (TrinityCore Unit::SpellDamageBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		coeff := 0.857 // standard 3.0s cast (~85.7%)
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
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
			}
		}
		damage += uint32(math.Round(float64(s.player.SpellPower) * coeff))
	}

	// Use the school mask from the Spell DBC (field 17). Fallback to physical (1).
	schoolMask := uint8(1)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.SchoolMask != 0 {
			schoolMask = uint8(spell.SchoolMask)
		}
	}

	s.executeDirectSpellDamage(ctx, targetGUID, spellID, damage, schoolMask)
}

func (s *session) executeDirectSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8) {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	if !isPlayerVictim && s.server != nil && s.server.isCreatureEvading(target.GUID) {
		hitInfo := uint32(0x01) // SPELL_HIT_TYPE_MISS
		damage = 0
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, 0, schoolMask, 0, 0, hitInfo), true)
		return
	}
	isHit := true
	if targetGUID != s.playerGUID {
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
		if spellKnown && spellID != 31117 && spellID != 64085 {
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

		if schoolMask > 1 && s.player != nil && s.player.Level > 0 {
			resIdx := schoolMaskToResistanceIndex(schoolMask)
			victimRes := target.Resistances[resIdx]
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, victimRes, s.player.Level, target.Level, s.player.SpellPenetration)
		}

		if isPlayerVictim {
			if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil {
				// Reference BE_SPELL_TARGET (28) and TIMED 6: being the
				// target of a hostile spell credits the victim and starts
				// target-timed criteria.
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget, spellID, 1)
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget2, spellID, 1)
				playerSess.startTimedAchievement(timedTypeSpellTarget, spellID)
				if playerSess.isImmuneToDamage(uint32(schoolMask)) {
					damage = 0
				}
				isCrit := (hitInfo & 0x02) != 0 // SPELL_HIT_TYPE_CRIT
				playerSess.applyResilienceToDamage(true, &damage, isCrit, CombatRatingCritTakenSpell)
				if damage > 0 {
					absorbed, damage = playerSess.applyAbsorptionShields(damage, schoolMask)
				}
			}
		} else if s.server != nil && damage > 0 {
			absorbed, damage = s.server.applyCreatureAbsorptionShields(target.GUID, damage, schoolMask)
		}
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	s.updateAchievementCriteria(criteriaTypeDamageDone, 0, damage)
	s.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)

	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo), true)

	// Trigger spell cast/hit procs (TrinityCore Unit::ProcDamageAndSpellFor)
	s.procSpellCastAndHitEffects(ctx, target, spellID)

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
				playerSess.delayCurrentCast()
				playerSess.delayCurrentChannel()
				playerSess.procDamageAuras(true, damage)
				playerSess.sendPlayerUpdate()
			}
			return
		}
	}

	low := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32((target.GUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(low, entry)

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
		if motion != nil {
			motion.Health = 0
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		s.server.stopCreatureMotion(target.Map, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})
		s.server.broadcastThreatClear(target.Map, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target)
		s.debug("target slain by spell", "account", s.accountName, "spell", spellID, "guid", target.GUID)
	} else {
		newHealth := target.Health - damage
		s.server.motionMu.Lock()
		motion := s.server.creatureMotion[target.GUID]
		if motion == nil {
			motion = s.server.creatureMotion[stdKey]
		}
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
				s.server.broadcastHighestThreatUpdate(motion.Map, motion.GUID, newVictim, entries)
			} else {
				motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
			}
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
			}
			motion.Moving = true
		}
		s.server.motionMu.Unlock()
		s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
		s.server.procCreatureDamageAuras(target.GUID, true, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		s.server.triggerPetDefensive(s.playerGUID, targetGUID)
	}
}

// castSpellDirect triggers an immediate, instant cast of a spell without cast time or resource cost.
// Mirrors TrinityCore Unit::CastSpell (Spell.cpp: triggered = true).
func (s *session) castSpellDirect(ctx context.Context, spellID uint32, targetGUID uint64) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

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

	castID := uint8(1)
	now := time.Now()
	castTimeStamp := uint32(now.UnixMilli())
	hitTargets := []uint64{targetGUID}
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
	goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, nil, spellTarget)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
	}

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
	for _, eff := range spell.Effects {
		if eff.Effect == 0 && eff.Aura == 0 {
			continue
		}
		hasExplicitEffects = true
		if eff.Effect == 2 { // SPELL_EFFECT_SCHOOL_DAMAGE
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
			s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, eff.AuraPeriod, amount, schoolMask)
		} else if eff.Effect == 10 { // SPELL_EFFECT_HEAL
			healAmount := uint32(eff.BasePoints + 1)
			if healAmount == 0 && spellID == ProcSpellCrusader {
				healAmount = 100
			}
			s.executeSpellHeal(ctx, targetGUID, spellID, healAmount)
		} else if eff.Effect == spellEffectEnergize {
			s.applySpellEnergize(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1)
		} else if eff.Effect == spellEffectPowerBurn {
			if burned := s.applySpellPowerBurn(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1, spellID); burned > 0 {
				s.executeSpellDamage(ctx, targetGUID, spellID, burned)
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
		s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, 0, 0, 1)
	}
}

func (s *session) applySpellEnergize(ctx context.Context, targetGUID uint64, powerType int32, amount int32) {
	s.adjustSpellPower(ctx, targetGUID, powerType, int64(amount))
}

func (s *session) applySpellPowerBurn(ctx context.Context, targetGUID uint64, powerType int32, amount int32, spellID uint32) uint32 {
	if s == nil || s.player == nil || powerType < 0 || powerType >= 7 || amount <= 0 {
		return 0
	}
	target := s.spellPowerTarget(targetGUID)
	if target == nil || target.player == nil || target.player.Health == 0 || classPowerType(target.player.Class) != uint8(powerType) {
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
	if burn > int64(target.player.Powers[uint32(powerType)]) {
		burn = int64(target.player.Powers[uint32(powerType)])
	}
	if burn <= 0 {
		return 0
	}
	target.adjustSpellPower(ctx, targetGUID, powerType, -burn)
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
	motion := s.server.creatureMotion[targetGUID]
	if motion == nil {
		low := uint32(targetGUID & 0x00FFFFFF)
		entry := uint32((targetGUID >> 24) & 0x00FFFFFF)
		motion = s.server.creatureMotion[creatureWorldGUID(low, entry)]
	}
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
		s.server.broadcastHighestThreatUpdate(mapID, guid, victim, entries)
	}
	if !wasInCombat {
		s.server.broadcastAIReaction(mapID, guid, 2)
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
		s.executeSpellHeal(ctx, targetGUID, spellID, creature.MaxHealth)
	}
}

func (s *session) executeSpellHeal(ctx context.Context, targetGUID uint64, spellID, heal uint32) {
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
		coeff := 0.857
		if s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
				if spell.CastingTimeIndex > 0 {
					if ct, ok, _ := s.server.Data.SpellCastTime(spell.CastingTimeIndex); ok && ct > 0 {
						coeff = float64(ct) / 3500.0
						if coeff > 1.0 {
							coeff = 1.0
						}
					}
				} else {
					coeff = 1.5 / 3.5
				}
			}
		}
		heal += uint32(math.Round(float64(s.player.SpellPower) * coeff))
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
	return false
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
		s.castMu.Unlock()

		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedInterrupted), true)
		return
	}
	s.castMu.Unlock()
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
	SpellID            uint32
	DispelType         uint32
	Mechanic           uint32
	AuraType           uint32
	CasterGUID         uint64
	TargetGUID         uint64
	SchoolMask         uint32
	MiscValue          int32
	Amount             uint32
	DurationMs         uint32
	PeriodMs           uint32
	RemainingMs        uint32
	Slot               uint8
	Positive           bool
	CasterLevel        uint8
	StackCount         uint8
	RemainingCharges   uint8
	StackAmount        uint32
	AuraInterruptFlags uint32
	TriggerSpell       uint32
	DRGroup            DiminishingGroup
	DamageTaken        uint32
	Timer              *time.Timer
	TickTimer          *time.Timer
	Stopped            bool
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
		if eff.Effect == 2 || eff.Effect == 87 || eff.Effect == 108 || eff.Effect == 17 {
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
	// Physical (1) and Holy (2) spells are never resisted by magic resistance
	if spell.SchoolMask == 0 || spell.SchoolMask&1 != 0 || spell.SchoolMask&2 != 0 {
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

// checkBinarySpellResist calculates whether a binary spell is fully resisted by the victim.
// Effective resistance is reduced by caster spell penetration (level difference resistance cannot be penetrated).
// Mirrors TrinityCore Unit::CalculateAverageResistReduction & Unit::MagicSpellHitResult (Unit.cpp:1721-1775).
func checkBinarySpellResist(victimResistance, casterPenetration uint32, casterLevel, victimLevel uint8) bool {
	effectiveRes := victimResistance
	if casterPenetration >= effectiveRes {
		effectiveRes = 0
	} else {
		effectiveRes -= casterPenetration
	}

	res := float64(effectiveRes)
	// Level-based resistance: 5 resistance per level difference if victim is higher level (cannot be penetrated)
	if victimLevel > casterLevel {
		res += float64(victimLevel-casterLevel) * 5.0
	}
	if res <= 0 {
		return false
	}

	const bossLevel = 83
	const bossResistanceConstant = 510.0
	resConstant := float64(victimLevel) * 5.0
	if victimLevel >= bossLevel {
		resConstant = bossResistanceConstant
	}
	if resConstant < 5.0 {
		resConstant = 5.0
	}

	averageResist := res / (res + resConstant)
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

// calcMagicSpellResistance computes magic damage resisted based on target resistance, caster penetration, and levels,
// matching TrinityCore Unit::CalculateAverageResistReduction and Unit::CalcSpellResistance (Unit.cpp:1721-1775).
func calcMagicSpellResistance(damage uint32, schoolMask uint8, victimResistance uint32, casterLevel, victimLevel uint8, casterPenetration ...uint32) (resisted uint32, remainingDamage uint32) {
	if damage == 0 || schoolMask == 0 || schoolMask&1 != 0 {
		return 0, damage
	}
	if schoolMask&2 != 0 {
		// Holy damage cannot be resisted in WotLK
		return 0, damage
	}

	pen := uint32(0)
	if len(casterPenetration) > 0 {
		pen = casterPenetration[0]
	}

	effectiveRes := victimResistance
	if pen >= effectiveRes {
		effectiveRes = 0
	} else {
		effectiveRes -= pen
	}

	res := float64(effectiveRes)
	// Level-based resistance: 5 resistance per level difference if victim is higher level (cannot be penetrated)
	if victimLevel > casterLevel {
		res += float64(victimLevel-casterLevel) * 5.0
	}

	const bossLevel = 83
	const bossResistanceConstant = 510.0
	resConstant := float64(victimLevel) * 5.0
	if victimLevel >= bossLevel {
		resConstant = bossResistanceConstant
	}
	if resConstant < 5.0 {
		resConstant = 5.0
	}

	averageResist := res / (res + resConstant)
	if averageResist <= 0.0 {
		return 0, damage
	}
	if averageResist > 1.0 {
		averageResist = 1.0
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
	step := 0
	for i := 0; i < 11; i++ {
		probSum += discreteProb[i]
		if roll < probSum {
			step = i
			break
		}
	}

	resPercent := float64(step) * 0.1
	resisted = uint32(math.Round(float64(damage) * resPercent))
	if resisted > damage {
		resisted = damage
	}
	remainingDamage = damage - resisted
	return resisted, remainingDamage
}

func (s *Server) clearCreatureAuras(guid uint64) {
	if s == nil {
		return
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[guid]; ok {
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
			delete(s.activeCreatureAuras, guid)
		}
	}
	if s.creatureAuras != nil {
		delete(s.creatureAuras, guid)
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
	level := uint8(1)
	if s.player != nil && s.player.Level > 0 {
		level = s.player.Level
	}
	pkt := protocol.BuildAuraUpdate(s.playerGUID, s.playerGUID, slot, spellID, remove, positive, maxDurationMs, durationMs, level)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), pkt, true)
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
	}
	var auraInterruptFlags uint32
	var auraType uint32
	var dispelType uint32
	var mechanic uint32
	var miscValue int32
	if s.server != nil && s.server.Data != nil {
		if sp, found, _ := s.server.Data.Spell(spellID); found {
			auraInterruptFlags = sp.AuraInterruptFlags
			dispelType = sp.DispelType
			mechanic = sp.Mechanic
			if len(sp.Effects) > 0 {
				auraType = sp.Effects[0].Aura
				miscValue = sp.Effects[0].MiscValue
			}
		}
	}
	positive := !isHarmfulAura(auraType)
	if spellID == 15007 {
		positive = false
	}
	aura := &activeAura{
		SpellID:            spellID,
		DispelType:         dispelType,
		Mechanic:           mechanic,
		AuraType:           auraType,
		CasterGUID:         s.playerGUID,
		TargetGUID:         s.playerGUID,
		MiscValue:          miscValue,
		DurationMs:         durationMs,
		RemainingMs:        durationMs,
		Slot:               slot,
		Positive:           positive,
		AuraInterruptFlags: auraInterruptFlags,
	}
	if durationMs > 0 && durationMs < 18000000 {
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.removeAura(spellID)
		})
	}
	s.activeAuras[spellID] = aura
	s.castMu.Unlock()

	s.sendAuraUpdate(slot, spellID, false, positive, durationMs, durationMs)
	s.sendPlayerUpdate()
}

func (s *session) removeAura(spellID uint32) {
	s.castMu.Lock()
	if s.activeAuras != nil {
		if aura, ok := s.activeAuras[spellID]; ok && aura != nil {
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
		}
	}
	s.castMu.Unlock()

	if s.auras != nil {
		delete(s.auras, spellID)
	}
	if s.auraSlots != nil {
		if slot, ok := s.auraSlots[spellID]; ok {
			s.sendAuraUpdate(slot, 0, true, false, 0, 0)
			delete(s.auraSlots, spellID)
		}
	}
	s.sendPlayerUpdate()
}

func (s *session) hasAura(spellID uint32) bool {
	if s.auras == nil {
		return false
	}
	_, ok := s.auras[spellID]
	return ok
}

func (s *session) applyAuraToTarget(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, durationMs, periodMs, amount, schoolMask uint32) {
	if s.player == nil {
		return
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}

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

		if existing, exists := targetSess.activeAuras[spell.ID]; exists && existing != nil {
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
			CasterGUID:         s.playerGUID,
			TargetGUID:         targetGUID,
			SchoolMask:         schoolMask,
			MiscValue:          eff.MiscValue,
			Amount:             amount,
			DurationMs:         durationMs,
			PeriodMs:           periodMs,
			RemainingMs:        durationMs,
			Slot:               slot,
			Positive:           positive,
			CasterLevel:        s.player.Level,
			AuraInterruptFlags: spell.AuraInterruptFlags,
			TriggerSpell:       eff.TriggerSpell,
			DRGroup:            drGroup,
			StackAmount:        spell.StackAmount,
			RemainingCharges:   uint8(spell.ProcCharges),
		}
		targetSess.activeAuras[spell.ID] = aura
		targetSess.castMu.Unlock()

		stackCount := uint8(1)
		if spell.StackAmount == 0 && spell.ProcCharges > 0 {
			stackCount = uint8(spell.ProcCharges)
		}
		updatePkt := protocol.BuildAuraUpdateWithStack(targetGUID, s.playerGUID, slot, spell.ID, false, positive, durationMs, durationMs, s.player.Level, stackCount)
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, targetSess)
		}
		targetSess.sendPlayerUpdate()

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

	if s.server == nil || s.server.isCreatureEvading(targetGUID) {
		return
	}
	s.server.auraMu.Lock()
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[uint64]map[uint32]struct{})
	}
	if s.server.creatureAuras[targetGUID] == nil {
		s.server.creatureAuras[targetGUID] = make(map[uint32]struct{})
	}
	s.server.creatureAuras[targetGUID][spell.ID] = struct{}{}

	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[uint64]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[targetGUID] == nil {
		s.server.activeCreatureAuras[targetGUID] = make(map[uint32]*activeAura)
	}
	if existing, exists := s.server.activeCreatureAuras[targetGUID][spell.ID]; exists && existing != nil {
		existing.Stopped = true
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.TickTimer != nil {
			existing.TickTimer.Stop()
		}
	}
	slot := uint8(len(s.server.activeCreatureAuras[targetGUID]) % 64)
	aura := &activeAura{
		SpellID:          spell.ID,
		DispelType:       spell.DispelType,
		Mechanic:         spell.Mechanic,
		AuraType:         eff.Aura,
		CasterGUID:       s.playerGUID,
		TargetGUID:       targetGUID,
		SchoolMask:       schoolMask,
		MiscValue:        eff.MiscValue,
		Amount:           amount,
		DurationMs:       durationMs,
		PeriodMs:         periodMs,
		RemainingMs:      durationMs,
		Slot:             slot,
		Positive:         positive,
		CasterLevel:      s.player.Level,
		TriggerSpell:     eff.TriggerSpell,
		StackAmount:      spell.StackAmount,
		RemainingCharges: uint8(spell.ProcCharges),
	}
	s.server.activeCreatureAuras[targetGUID][spell.ID] = aura
	s.server.auraMu.Unlock()

	stackCount := uint8(1)
	if spell.StackAmount == 0 && spell.ProcCharges > 0 {
		stackCount = uint8(spell.ProcCharges)
	}
	updatePkt := protocol.BuildAuraUpdateWithStack(targetGUID, s.playerGUID, slot, spell.ID, false, positive, durationMs, durationMs, s.player.Level, stackCount)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, s)

	if periodMs > 0 {
		s.scheduleCreaturePeriodicTick(aura, periodMs)
	}
	if durationMs > 0 && durationMs < 18000000 {
		s.server.auraMu.Lock()
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.expireCreatureAura(targetGUID, spell.ID, slot)
		})
		s.server.auraMu.Unlock()
	}
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
		if aura.RemainingMs >= periodMs {
			aura.RemainingMs -= periodMs
		} else {
			aura.RemainingMs = 0
		}
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
			resIdx := schoolMaskToResistanceIndex(uint8(aura.SchoolMask))
			vRes := ts.player.Resistances[resIdx]
			pen := uint32(0)
			if ts.server != nil {
				if cs := ts.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), vRes, aura.CasterLevel, ts.player.Level, pen)
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
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, 0, aura.Amount)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}
	}
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
		if aura.RemainingMs >= periodMs {
			aura.RemainingMs -= periodMs
		} else {
			aura.RemainingMs = 0
		}
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
	target, ok := s.getCombatTarget(ctx, aura.TargetGUID)
	if !ok || target.Health == 0 || (s.server != nil && s.server.isCreatureEvading(aura.TargetGUID)) {
		if s.server != nil {
			s.server.clearCreatureAuras(aura.TargetGUID)
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
			resIdx := schoolMaskToResistanceIndex(uint8(aura.SchoolMask))
			vRes := target.Resistances[resIdx]
			pen := uint32(0)
			if s.server != nil {
				if cs := s.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), vRes, aura.CasterLevel, target.Level, pen)
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
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}

		if dmg >= targetHealth {
			// Target slain by DoT
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.creatureMotion[aura.TargetGUID]
				if motion == nil {
					low := uint32(aura.TargetGUID & 0x00FFFFFF)
					entry := uint32((aura.TargetGUID >> 24) & 0x00FFFFFF)
					motion = s.server.creatureMotion[creatureWorldGUID(low, entry)]
				}
				if motion != nil {
					motion.Health = 0
					motion.InCombat = false
					motion.TargetGUID = 0
					motion.Moving = false
					if motion.ThreatMgr != nil {
						motion.ThreatMgr.ClearThreat()
					}
				}
				s.server.motionMu.Unlock()

				s.server.stopCreatureMotion(target.Map, target.GUID, target.X, target.Y, target.Z)
				s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{
					unitFieldHealth:       0,
					unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
				})
				s.server.broadcastThreatClear(target.Map, target.GUID)
				s.server.clearCreatureAuras(aura.TargetGUID)
			}
			_ = s.sendAttackStop(target.GUID, true)
			s.attackTarget = 0
			s.onCreatureKilled(ctx, target)
			return false
		} else {
			newHealth := targetHealth - dmg
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.creatureMotion[aura.TargetGUID]
				if motion == nil {
					low := uint32(aura.TargetGUID & 0x00FFFFFF)
					entry := uint32((aura.TargetGUID >> 24) & 0x00FFFFFF)
					motion = s.server.creatureMotion[creatureWorldGUID(low, entry)]
				}
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
				s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
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
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
			s.server.motionMu.Lock()
			motion := s.server.creatureMotion[aura.TargetGUID]
			if motion != nil {
				motion.Health = newHP
			}
			s.server.motionMu.Unlock()
			s.server.broadcastCreatureValuesUpdate(target.Map, target.GUID, map[int]uint32{unitFieldHealth: newHP})
		}
		return true

	case 23: // SPELL_AURA_PERIODIC_TRIGGER_SPELL
		if aura.TriggerSpell != 0 && s.server != nil && s.server.Data != nil {
			if trigger, found, err := s.server.Data.Spell(aura.TriggerSpell); err == nil && found {
				if damage, ok := creatureSpellDamage(trigger); ok && damage > 0 {
					s.executeSpellDamage(ctx, aura.TargetGUID, aura.TriggerSpell, damage)
				}
			}
		}
		return true

	case 24: // SPELL_AURA_PERIODIC_ENERGIZE
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, 0, aura.Amount)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}
		return true
	}
	return true
}

func (s *session) expireCreatureAura(creatureGUID uint64, spellID uint32, slot uint8) {
	if s.server == nil {
		return
	}
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras != nil {
		if auras, ok := s.server.activeCreatureAuras[creatureGUID]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				aura.Stopped = true
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
			}
		}
	}
	if s.server.creatureAuras != nil {
		if auras, ok := s.server.creatureAuras[creatureGUID]; ok {
			delete(auras, spellID)
		}
	}
	s.server.auraMu.Unlock()

	removePkt := protocol.BuildAuraUpdate(creatureGUID, s.playerGUID, slot, 0, true, false, 0, 0, 1)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, s)
}

func (s *Server) removeCreatureAura(creatureGUID uint64, spellID uint32) {
	if s == nil || creatureGUID == 0 || spellID == 0 {
		return
	}
	s.auraMu.Lock()
	var slot uint8
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[creatureGUID]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				aura.Stopped = true
				slot = aura.Slot
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
			}
		}
	}
	if s.creatureAuras != nil {
		if auras, ok := s.creatureAuras[creatureGUID]; ok {
			delete(auras, spellID)
		}
	}
	s.auraMu.Unlock()

	removePkt := protocol.BuildAuraUpdate(creatureGUID, 0, slot, 0, true, false, 0, 0, 1)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

// handleCancelMountAura processes CMSG_CANCEL_MOUNT_AURA (0x375).
// Reference: WorldSession::HandleCancelMountAuraOpcode (SpellHandler.cpp:544).
func (s *session) handleCancelMountAura(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.player.MountDisplayID != 0 {
		s.player.MountDisplayID = 0
		s.sendPlayerUpdate()
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
				s.finishSpellCast(ctx, 0, click.spellID, spell, targetData)
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
			learnBuf := protocol.NewBuffer(6)
			learnBuf.WriteU32(sp)
			learnBuf.WriteU16(0)
			_ = s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), learnBuf.Bytes(), true)
		}
	}

	// 5. Load and send action buttons for new spec
	if actions, err := s.loadActionButtons(ctx, s.playerGUID, s.player.Race, s.player.Class); err == nil {
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
	Spell      wotlk.Spell
	DurationMs uint32
	Remaining  time.Duration
	PeriodMs   uint32
	Pushbacks  int
	Timer      *time.Timer
	TickTimer  *time.Timer
	Stopped    bool
}

func isChanneledSpell(spell wotlk.Spell) bool {
	return spell.AttributesEx1&(spellAttr1Channeled1|spellAttr1Channeled2) != 0
}

// sendChannelUpdate mirrors Spell::SendChannelUpdate: packed caster GUID plus
// remaining channel milliseconds; zero clears the client channel bar.
func (s *session) sendChannelUpdate(remainingMs uint32) {
	packet := protocol.NewBuffer(12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(remainingMs)
	_ = s.write(uint16(protocol.OpcodeMSG_CHANNEL_UPDATE), packet.Bytes(), true)
}

// startChannel begins the channeled phase of a finished cast: broadcast the
// channel start, schedule periodic ticks, and arm completion.
func (s *session) startChannel(castID uint8, spellID uint32, spell wotlk.Spell, targetGUID uint64) {
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
		Spell:      spell,
		DurationMs: uint32(durationMs),
		Remaining:  time.Duration(durationMs) * time.Millisecond,
		PeriodMs:   period,
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

	channel.Timer = time.AfterFunc(channel.Remaining, func() { s.finishChannel() })
	if period > 0 && period <= uint32(durationMs) {
		channel.TickTimer = time.AfterFunc(time.Duration(period)*time.Millisecond, func() { s.channelTick() })
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
	channel.Stopped = true
	s.castMu.Unlock()

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
	for _, effect := range spell.Effects {
		if effect.Effect == 0 {
			continue
		}
		amount := uint32(effect.BasePoints + 1)
		if amount == 0 {
			continue
		}
		switch effect.Effect {
		case 2, 87, 108, 17: // damage effects tick on the target
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount)
			}
		case 6, 10, 136, 105: // auras and heals tick on the target or caster
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount)
			} else {
				s.executeSpellHeal(ctx, s.playerGUID, spell.ID, amount)
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

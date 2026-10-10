package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Explicit-target hostility/faction gates for SpellInfo::CheckExplicitTarget.

// SpellCastTargetFlags unit bits (SpellInfo.h:41-47). ENEMY/ALLY/PARTY/RAID
// are never sent by the client; they only validate the explicit unit target.
const (
	targetFlagUnit           uint32 = 0x2
	targetFlagUnitRaid       uint32 = 0x4
	targetFlagUnitParty      uint32 = 0x8
	targetFlagUnitEnemy      uint32 = 0x80
	targetFlagUnitAlly       uint32 = 0x100
	targetFlagUnitPassenger  uint32 = 0x100000 // TARGET_FLAG_UNIT_PASSENGER (SpellInfo.h:68)
	targetFlagSourceLocation uint32 = 0x20     // TARGET_FLAG_SOURCE_LOCATION (SpellInfo.h:44)
	targetFlagDestLocation   uint32 = 0x40     // TARGET_FLAG_DEST_LOCATION (SpellInfo.h:45)
	targetFlagGameObject     uint32 = 0x800    // TARGET_FLAG_GAMEOBJECT (SpellInfo.h:54)
	targetFlagGameObjectItem uint32 = 0x4000   // TARGET_FLAG_GAMEOBJECT_ITEM (SpellInfo.h:58)
	targetFlagItem           uint32 = 0x10     // TARGET_FLAG_ITEM (SpellInfo.h:52)
	targetFlagUnitMinipet    uint32 = 0x10000  // TARGET_FLAG_UNIT_MINIPET (SpellInfo.h:60)
	// targetFlagUnitMask mirrors TARGET_FLAG_UNIT_MASK (SpellInfo.h:70-71).
	targetFlagUnitMask uint32 = targetFlagUnit | targetFlagUnitRaid | targetFlagUnitParty | targetFlagUnitEnemy | targetFlagUnitAlly | targetFlagUnitDead | targetFlagUnitMinipet | targetFlagUnitPassenger
)

// UNIT_FIELD_BYTES_2 PvP-flag byte values (UnitDefines.h:103-106); Go's
// playerState.PVPFlags mirrors the byte exactly.
const (
	pvpFlagPvP       uint8 = 0x01 // UNIT_BYTE2_FLAG_PVP
	pvpFlagFFA       uint8 = 0x04 // UNIT_BYTE2_FLAG_FFA_PVP
	pvpFlagSanctuary uint8 = 0x08 // UNIT_BYTE2_FLAG_SANCTUARY
)

// spellExplicitUnitTargetMask mirrors the unit-target half of
// SpellImplicitTargetInfo::GetExplicitTargetMask (SpellInfo.cpp:134-210).
// Per effect and per TargetA/TargetB, TARGET-reference rows map their
// selection check type to the validating flag: TARGET_CHECK_ENEMY ->
// TARGET_FLAG_UNIT_ENEMY, TARGET_CHECK_ALLY -> TARGET_FLAG_UNIT_ALLY,
// TARGET_CHECK_PARTY -> TARGET_FLAG_UNIT_PARTY, TARGET_CHECK_RAID ->
// TARGET_FLAG_UNIT_RAID, TARGET_CHECK_PASSENGER ->
// TARGET_FLAG_UNIT_PASSENGER. TARGET_OBJECT_TYPE_DEST rows are grouped
// with the UNIT rows under TARGET_REFERENCE_TYPE_TARGET (SpellInfo.cpp:158),
// so target 53 (TARGET_DEST_TARGET_ENEMY, TARGET_CHECK_ENEMY) maps to
// TARGET_FLAG_UNIT_ENEMY as well. The table rows used are:
// 6 -> ENEMY; 21, 45 -> ALLY; 35 -> PARTY; 57 -> RAID (SpellInfo.cpp:226,
// 241, 255, 265, 277); 95 -> PASSENGER (SpellInfo.cpp:184, 315).
// Target 90 (TARGET_UNIT_TARGET_MINIPET) carries TARGET_CHECK_DEFAULT and
// falls through to plain TARGET_FLAG_UNIT, so the TARGET_FLAG_UNIT_MINIPET
// bit (0x10000, SpellInfo.h:64) is never produced by GetExplicitTargetMask
// and its CheckExplicitTarget arm (SpellInfo.cpp:1806-1808) is dead on the
// DBC-driven path.
func spellExplicitUnitTargetMask(spell wotlk.Spell) uint32 {
	mask := uint32(0)
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, target := range [2]uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch target {
			case 6:
				mask |= targetFlagUnitEnemy
			case 21, 45:
				mask |= targetFlagUnitAlly
			case 35:
				mask |= targetFlagUnitParty
			case 53:
				mask |= targetFlagUnitEnemy // TARGET_DEST_TARGET_ENEMY: DEST rows group with UNIT rows under TARGET_REFERENCE_TYPE_TARGET (SpellInfo.cpp:158-184)
			case 57:
				mask |= targetFlagUnitRaid
			case 90:
				mask |= targetFlagUnit // TARGET_CHECK_DEFAULT fall-through to plain TARGET_FLAG_UNIT (SpellInfo.cpp:184)
			case 95:
				mask |= targetFlagUnitPassenger
			}
		}
	}
	return mask
}

// spellEffectExplicitUsedTargetFlag ports the used-target-object-type column
// of SpellEffectInfo::_data (SpellInfo.cpp:618-775) for effects whose static
// implicit-target type is EFFECT_IMPLICIT_TARGET_EXPLICIT (SpellInfo.h:144),
// as a target-flag mask via GetTargetFlagMask (SpellInfo.cpp:38-64). Effects
// absent from the table are not EXPLICIT-typed and contribute nothing.
func spellEffectExplicitUsedTargetFlag(effect uint32) uint32 {
	switch effect {
	case 5, 29, 43, 69, 83, 144, 145:
		return targetFlagDestLocation | targetFlagUnit // TARGET_OBJECT_TYPE_UNIT_AND_DEST
	case 18, 113:
		return targetFlagCorpseAlly // TARGET_OBJECT_TYPE_CORPSE_ALLY
	case 116:
		return targetFlagCorpseEnemy // TARGET_OBJECT_TYPE_CORPSE_ENEMY
	case 27, 28, 50, 56, 72, 76, 81, 104, 105, 106, 107, 109, 135, 149:
		return targetFlagDestLocation // TARGET_OBJECT_TYPE_DEST
	case 33:
		return targetFlagGameObjectItem // TARGET_OBJECT_TYPE_GOBJ_ITEM
	case 86, 87, 88, 89:
		return targetFlagGameObject // TARGET_OBJECT_TYPE_GOBJ
	case 53, 54, 99, 101, 127, 156, 158:
		return targetFlagItem // TARGET_OBJECT_TYPE_ITEM
	case 1, 2, 6, 7, 8, 9, 10, 11, 16, 17, 19, 24, 31, 35, 36, 38, 40, 41,
		44, 45, 55, 57, 58, 59, 62, 63, 65, 66, 67, 68, 70, 71, 73, 75,
		80, 82, 90, 91, 92, 95, 96, 98, 100, 102, 103, 108, 111, 112,
		114, 115, 117, 119, 120, 121, 123, 124, 125, 126, 128, 129, 130,
		132, 133, 136, 137, 138, 139, 140, 141, 142, 143, 146, 147, 150,
		153, 154, 157, 159, 160, 161, 162, 163, 164:
		return targetFlagUnit // TARGET_OBJECT_TYPE_UNIT
	default:
		return 0
	}
}

// spellMissingExplicitTargetMask mirrors the GetMissingTargetMask arm of
// SpellInfo::_InitializeExplicitTargetMask (SpellInfo.cpp:3358-3371): for
// each effect whose static implicit-target type is
// EFFECT_IMPLICIT_TARGET_EXPLICIT, the used target object type's flag mask
// (SpellEffectInfo::GetMissingTargetMask, SpellInfo.cpp:581-603) is OR'd into
// the explicit mask when the effect's own implicit targets don't already
// provide it. baseMask is the mask accumulated from the DBC Targets field and
// the per-effect GetExplicitTargetMask bits, matching the sequential C++
// loop; the src/dst latch mirrors the trailing switch of
// SpellImplicitTargetInfo::GetExplicitTargetMask (SpellInfo.cpp:203-212) —
// no implicit target number carries TARGET_OBJECT_TYPE_UNIT_AND_DEST, so the
// flag-based latch is exact. The no-max-range strip (SpellInfo.cpp:3367-3369)
// is unmodeled: it needs the DBC SpellRange row, which the pure mask
// functions cannot reach; it only fires for self-range spells whose EXPLICIT
// used type is uncovered, a combination with no known spell.
func spellMissingExplicitTargetMask(spell wotlk.Spell, baseMask uint32) uint32 {
	mask := baseMask
	var srcSet, dstSet bool
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, tgt := range [2]uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			fl := spellImplicitTargetObjectFlag(tgt)
			if fl&targetFlagSourceLocation != 0 {
				srcSet = true
			}
			if fl&targetFlagDestLocation != 0 {
				dstSet = true
			}
		}
		used := spellEffectExplicitUsedTargetFlag(eff.Effect)
		if used == 0 {
			continue
		}
		effMask := used
		provided := spellEffectProvidedTargetMask(eff) | mask
		if provided&targetFlagUnitMask != 0 {
			effMask &^= targetFlagUnitMask
		}
		if provided&(targetFlagCorpseAlly|targetFlagCorpseEnemy) != 0 {
			effMask &^= targetFlagUnitMask | targetFlagCorpseAlly | targetFlagCorpseEnemy
		}
		if provided&targetFlagGameObjectItem != 0 {
			effMask &^= targetFlagGameObjectItem | targetFlagGameObject | targetFlagItem
		}
		if provided&targetFlagGameObject != 0 {
			effMask &^= targetFlagGameObject | targetFlagGameObjectItem
		}
		if provided&targetFlagItem != 0 {
			effMask &^= targetFlagItem | targetFlagGameObjectItem
		}
		if dstSet || provided&targetFlagDestLocation != 0 {
			effMask &^= targetFlagDestLocation
		}
		if srcSet || provided&targetFlagSourceLocation != 0 {
			effMask &^= targetFlagSourceLocation
		}
		mask |= effMask
	}
	return mask &^ baseMask
}

// spellExplicitObjectTargetMask mirrors the object-target half of
// SpellInfo::GetExplicitTargetMask (SpellInfo.cpp:1956;
// _InitializeExplicitTargetMask, SpellInfo.cpp:3344-3370) as tested by
// CheckExplicitTarget's null-target arm (SpellInfo.cpp:1784-1790):
// (mask & (TARGET_FLAG_UNIT_MASK | TARGET_FLAG_GAMEOBJECT_MASK |
// TARGET_FLAG_CORPSE_MASK)) != 0. The DBC Targets field is OR'd in first
// (SpellInfo.cpp:3347), then the per-effect GetExplicitTargetMask bits
// (SpellInfo.cpp:134-210): the unit half via spellNeedsExplicitUnitTarget
// (the named check types plus the plain-TARGET_FLAG_UNIT fall-through for
// TARGET-reference UNIT/UNIT_AND_DEST/DEST rows), TARGET_FLAG_GAMEOBJECT
// from target 23 (TARGET_GAMEOBJECT_TARGET — the only TARGET-reference GOBJ
// row; 40/51/52/108 use CASTER/SRC/DEST references and contribute nothing),
// and TARGET_FLAG_GAMEOBJECT_ITEM from target 26
// (TARGET_GAMEOBJECT_ITEM_TARGET). The CORPSE bits never survive
// GetExplicitTargetMask (TARGET_OBJECT_TYPE_CORPSE hits the default arm),
// so the null arm's CORPSE leg is dead in C++. The SRC/DEST-location and
// TRAJ legs are irrelevant to the null arm. The GetMissingTargetMask
// extension (SpellInfo.cpp:3364) for EFFECT_IMPLICIT_TARGET_EXPLICIT
// effects rides spellMissingExplicitTargetMask above.
// explicitWireTargetFitsMask mirrors the object-target validity arm of
// Spell::InitExplicitTargets (Spell.cpp:668-678): a wire target whose object
// type the spell's explicit mask does not need is dropped before the
// selection/self fallbacks run. A gameobject wire target is never adopted as
// the explicit unit target — C++ keeps it as the GO object target on the
// GOTargetInfo path (Go's gameobject_destructible.go wire-GO append is the
// mirror). Corpse wire targets are always dropped: GetExplicitTargetMask
// never produces corpse bits (SpellInfo.cpp:134-212), so the C++ CORPSE arm
// always removes them. A unit wire target (player, creature, pet, minipet —
// C++ ToUnit covers them all, including a not-released player corpse) needs
// a unit bit in the mask.
func explicitWireTargetFitsMask(spell wotlk.Spell, target protocol.SpellTargetData) bool {
	if target.Flags&protocol.SpellTargetFlagGameObject != 0 {
		return false
	}
	if target.Flags&(protocol.SpellTargetFlagCorpseEnemy|protocol.SpellTargetFlagCorpseAlly) != 0 {
		return false
	}
	return spellExplicitObjectTargetMask(spell)&targetFlagUnitMask != 0
}

func spellExplicitObjectTargetMask(spell wotlk.Spell) uint32 {
	mask := spell.Targets & (targetFlagUnitMask | targetFlagGameObject | targetFlagGameObjectItem)
	if spellNeedsExplicitUnitTarget(spell) {
		mask |= targetFlagUnit
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, target := range [2]uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch target {
			case 23:
				mask |= targetFlagGameObject
			case 26:
				mask |= targetFlagGameObjectItem
			}
		}
	}
	// The GetMissingTargetMask extension (SpellInfo.cpp:3364) for
	// EFFECT_IMPLICIT_TARGET_EXPLICIT effects.
	return mask | spellMissingExplicitTargetMask(spell, mask)
}

// explicitSelectionTargetOK mirrors the selection-adoption gate in
// Spell::SetTargetMap (Spell.cpp:684-690): the caster's current selection
// becomes the fallback explicit target only when it resolves to a unit in
// the world (ObjectAccessor::GetUnit) and passes
// SpellInfo::CheckExplicitTarget (SpellInfo.cpp:1793-1816) — the
// IsValidAttackTarget/IsValidAssistTarget flag gates
// (spellTargetUnitBlocked) and the hostility/faction gates
// (explicitTargetFactionBlocked). The GM-invisibility arm is a CheckTarget
// leg (SpellInfo.cpp:1736-1743), not a CheckExplicitTarget leg, so it does
// not participate here.
func (s *session) explicitSelectionTargetOK(ctx context.Context, spell wotlk.Spell, guid uint64) bool {
	tgt, ok := s.getCombatTarget(ctx, guid)
	if !ok {
		return false
	}
	explicitMask := spellExplicitUnitTargetMask(spell)
	assist := explicitMask&(targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid) != 0
	if spellTargetUnitBlocked(spell, tgt.UnitFlags, tgt.FlagsExtra, assist) {
		return false
	}
	return !s.explicitTargetFactionBlocked(explicitMask, guid, tgt)
}

// explicitTargetFactionBlocked mirrors the hostility terms that
// SpellInfo::CheckExplicitTarget (SpellInfo.cpp:1799-1816) applies to an
// explicit unit target. TARGET_FLAG_UNIT_ENEMY runs the
// Unit::IsValidAttackTarget hostility term ("can't attack friendly targets"
// - IsFriendlyTo fails in either direction, Object.cpp), so neutral units
// stay valid targets; TARGET_FLAG_UNIT_
// ALLY/PARTY/RAID runs the WorldObject::IsValidAssistTarget term ("can't
// assist non-friendly targets" - GetReactionTo below REP_NEUTRAL in both
// directions, Object.cpp:3145), with the PARTY/RAID membership terms
// (Unit::IsInPartyWith/Unit::IsInRaidWith, Unit.cpp:12126).
// TARGET_FLAG_UNIT_PASSENGER runs the unitTarget->IsOnVehicle(unitCaster)
// term (SpellInfo.cpp:1809-1811) via explicitTargetPassengerBlocked. The
// TARGET_FLAG_UNIT_MINIPET arm is dead (see spellExplicitUnitTargetMask).
// Returns true when no branch passes and the cast must fail with
// SPELL_FAILED_BAD_TARGETS.
func (s *session) explicitTargetFactionBlocked(mask uint32, explicitUnitGUID uint64, tgt combatTarget) bool {
	if mask&(targetFlagUnitEnemy|targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid|targetFlagUnitPassenger) == 0 {
		return false
	}
	targetSess := s.server.findSessionByGUID(explicitUnitGUID)
	friendly, hostile := false, false
	duelHostile := false
	if targetSess != nil && targetSess.player != nil {
		// Duel opponents are hostile even across the same faction
		// (WorldObject::GetReactionTo, Object.cpp:2744-2746: "duel - always
		// hostile to opponent"). Go's DuelTeam is the in-progress marker,
		// set after the accept countdown like DUEL_STATE_IN_PROGRESS
		// (Player.cpp:20791-20796).
		duelHostile = s.duelPartner != 0 && s.duelPartner == targetSess.playerGUID &&
			s.player.DuelTeam != 0 && targetSess.player.DuelTeam != 0
		friendly = !duelHostile && targetSess.playerAlliance() == s.playerAlliance()
		hostile = duelHostile || !friendly
	} else {
		caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
		friendly = s.server.isFriendlyFaction(tgt.Faction, caster)
		hostile = !friendly && s.server.isHostileFaction(tgt.Faction, caster)
	}
	if mask&targetFlagUnitEnemy != 0 && !friendly && !s.explicitTargetAttackPvPBlocked(targetSess, duelHostile) {
		return false
	}
	if mask&targetFlagUnitAlly != 0 && !hostile && !s.explicitTargetAssistPvPBlocked(targetSess) {
		return false
	}
	if mask&targetFlagUnitParty != 0 && !hostile && !s.explicitTargetAssistPvPBlocked(targetSess) && s.sameGroupAs(targetSess, false) {
		return false
	}
	if mask&targetFlagUnitRaid != 0 && !hostile && !s.explicitTargetAssistPvPBlocked(targetSess) && s.sameGroupAs(targetSess, true) {
		return false
	}
	if mask&targetFlagUnitPassenger != 0 && !s.explicitTargetPassengerBlocked(explicitUnitGUID) {
		return false
	}
	return true
}

// explicitTargetPassengerBlocked mirrors the TARGET_FLAG_UNIT_PASSENGER arm
// of SpellInfo::CheckExplicitTarget (SpellInfo.cpp:1809-1811):
// unitTarget->IsOnVehicle(unitCaster) (Unit.cpp:12074-12077: m_vehicle &&
// m_vehicle == vehicle->GetVehicleKit()). The caster is always the session
// player on this path, so the caster's vehicle kit is the one registered
// under their own GUID (getVehicleKit, vehicle.go:404); a missing kit means
// the caster is not a vehicle and the target cannot ride it. The target
// must occupy one of that kit's seats (VehicleKit.GetSeatForPassenger,
// vehicle.go:94) — kit-pointer equality collapses to the same base GUID,
// the m_vehicle non-null leg collapses to seat occupancy, and seats store
// raw GUIDs so creature passengers ride the same lookup. The unitCaster
// null arm is vacuous (the caster is always a player session here).
// Returns true when the cast must fail with SPELL_FAILED_BAD_TARGETS.
func (s *session) explicitTargetPassengerBlocked(explicitUnitGUID uint64) bool {
	if s == nil || s.server == nil || s.player == nil {
		return true
	}
	kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.playerGUID)
	if kit == nil {
		return true
	}
	seatID, _, _ := kit.GetSeatForPassenger(explicitUnitGUID)
	return seatID < 0
}

// explicitTargetAttackPvPBlocked mirrors the player-vs-player tail of
// WorldObject::IsValidAttackTarget (Object.cpp:3045-3080) behind the
// explicit TARGET_FLAG_UNIT_ENEMY check: a duel opponent in progress is
// attackable, sanctuary on either side blocks, otherwise the target must be
// PvP-flagged or both sides in FFA. The UNIT_BYTE2_FLAG_UNK1 fallback term
// is dead in C++ (the flag is never set), so a non-flagged non-FFA target
// blocks the cast. Creature targets degrade to the faction verdict - every
// term here is player-only in C++. Returns true when the cast must fail.
func (s *session) explicitTargetAttackPvPBlocked(targetSess *session, duelHostile bool) bool {
	if targetSess == nil || targetSess.player == nil {
		return false
	}
	if duelHostile {
		return false
	}
	attackerFlags := s.player.PVPFlags
	targetFlags := targetSess.player.PVPFlags
	if attackerFlags&pvpFlagSanctuary != 0 || targetFlags&pvpFlagSanctuary != 0 {
		return true
	}
	if targetFlags&pvpFlagPvP != 0 {
		return false
	}
	if attackerFlags&pvpFlagFFA != 0 && targetFlags&pvpFlagFFA != 0 {
		return false
	}
	return true
}

// explicitTargetAssistPvPBlocked mirrors the player-vs-player PvP terms of
// WorldObject::IsValidAssistTarget (Object.cpp:3160-3171) behind the
// explicit TARGET_FLAG_UNIT_ALLY/PARTY/RAID checks: a player mid-duel with
// someone else cannot be assisted, a player in an FFA zone cannot be
// assisted from outside one, and a PvP-flagged player outside sanctuary
// cannot be assisted from inside one. Self is exempt from the duel term
// (C++ compares the caster's and target's players). Creature targets
// degrade to the faction verdict - every term here is player-only in C++.
// Returns true when the cast must fail.
func (s *session) explicitTargetAssistPvPBlocked(targetSess *session) bool {
	if targetSess == nil || targetSess.player == nil || targetSess == s {
		return false
	}
	if targetSess.duelPartner != 0 {
		return true
	}
	targetFlags := targetSess.player.PVPFlags
	attackerFlags := s.player.PVPFlags
	if targetFlags&pvpFlagFFA != 0 && attackerFlags&pvpFlagFFA == 0 {
		return true
	}
	if targetFlags&pvpFlagPvP != 0 && attackerFlags&pvpFlagSanctuary != 0 && targetFlags&pvpFlagSanctuary == 0 {
		return true
	}
	return false
}

// sameGroupAs is the player-player half of Unit::IsInPartyWith/
// Unit::IsInRaidWith (Unit.cpp:12126): same group, raid flag matched.
// Creature/pet/charm terms have no Go model.
func (s *session) sameGroupAs(targetSess *session, raid bool) bool {
	if s == nil || targetSess == nil || s.groupID == 0 || targetSess.groupID == 0 || s.groupID != targetSess.groupID {
		return false
	}
	g := s.server.findGroupByID(s.groupID)
	return g != nil && g.IsRaid == raid
}

// explicitTargetGMBlocked mirrors the GM/invisibility gate in
// SpellInfo::CheckTarget (SpellInfo.cpp:1736-1743) for the explicit unit
// path: a player target that is GM-invisible (Unit::IsVisible false) or in
// GM mode (Player::IsGameMaster) rejects the cast with
// SPELL_FAILED_BM_OR_INVISGOD. C++ guards this with unitTarget != caster
// (self is exempt), GetTypeId() == TYPEID_PLAYER (creatures are never
// checked), and (caster->GetAffectingPlayer() || !IsPositive()) - the
// GetAffectingPlayer term is always true in Go since the caster is always
// the player session, so player casters gate positive spells too, like
// C++. Lookup failure degrades to the old accept path. Returns true when
// the cast must fail.
func (s *session) explicitTargetGMBlocked(explicitUnitGUID uint64) bool {
	targetSess := s.server.findSessionByGUID(explicitUnitGUID)
	if targetSess == nil || targetSess.player == nil {
		return false
	}
	flags := targetSess.player.ExtraFlags
	return flags&playerExtraGMInvisible != 0 || flags&playerExtraGMOn != 0
}

// explicitTargetFlyingBlocked mirrors the flying-target gate in
// SpellInfo::CheckTarget (SpellInfo.cpp:1745-1747) for the explicit unit
// path: a unit target with UNIT_STATE_IN_FLIGHT rejects the cast with
// SPELL_FAILED_BAD_TARGETS unless the spell carries
// SPELL_ATTR0_CU_ALLOW_INFLIGHT_TARGET (AttributesCu, SpellInfo.h:196/422,
// mirrored via Server.getSpellCustomAttr). C++ runs this gate in CheckCast
// on the explicit unit target (Spell.cpp:197-201); implicit area/cone/chain
// selection skips CheckTarget entirely (AddUnitTarget with checkIfValid
// false, Spell.cpp:1217/1303 and SelectImplicitChainTargets), so the gate
// applies only here. Go's only in-flight units are players on taxi paths
// (session.inFlight, taxi.go:544; isInFlight mirrors Unit::IsInFlight,
// taxi.go:560); creatures never carry the state. Returns true when the cast
// must fail.
func (s *session) explicitTargetFlyingBlocked(spell wotlk.Spell, explicitUnitGUID uint64) bool {
	if s == nil || explicitUnitGUID == 0 {
		return false
	}
	if s.server != nil && s.server.getSpellCustomAttr(spell.ID)&SpellCustomAttrAllowInflightTarget != 0 {
		return false
	}
	if s.player != nil && explicitUnitGUID == s.playerGUID {
		return s.inFlight
	}
	if s.server != nil {
		if targetSess := s.server.findSessionByGUID(explicitUnitGUID); targetSess != nil {
			return targetSess.inFlight
		}
	}
	return false
}

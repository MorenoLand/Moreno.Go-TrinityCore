package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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
// Per effect and per TargetA/TargetB, unit targets with TARGET reference
// type map their selection check type to the validating flag:
// TARGET_CHECK_ENEMY -> TARGET_FLAG_UNIT_ENEMY, TARGET_CHECK_ALLY ->
// TARGET_FLAG_UNIT_ALLY, TARGET_CHECK_PARTY -> TARGET_FLAG_UNIT_PARTY,
// TARGET_CHECK_RAID -> TARGET_FLAG_UNIT_RAID, TARGET_CHECK_PASSENGER ->
// TARGET_FLAG_UNIT_PASSENGER. The table rows used are:
// 6 -> ENEMY; 21, 45 -> ALLY; 35 -> PARTY; 57 -> RAID (SpellInfo.cpp:226,
// 241, 255, 265, 277); 95 -> PASSENGER (SpellInfo.cpp:184, 315). Target 90
// (TARGET_UNIT_TARGET_MINIPET) carries TARGET_CHECK_DEFAULT and falls
// through to plain TARGET_FLAG_UNIT, so the TARGET_FLAG_UNIT_MINIPET bit
// (0x10000, SpellInfo.h:64) is never produced by GetExplicitTargetMask and
// its CheckExplicitTarget arm (SpellInfo.cpp:1806-1808) is dead on the
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
			case 57:
				mask |= targetFlagUnitRaid
			case 95:
				mask |= targetFlagUnitPassenger
			}
		}
	}
	return mask
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

package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Explicit-target hostility/faction gates for SpellInfo::CheckExplicitTarget.

// SpellCastTargetFlags unit bits (SpellInfo.h:41-47). ENEMY/ALLY/PARTY/RAID
// are never sent by the client; they only validate the explicit unit target.
const (
	targetFlagUnit      uint32 = 0x2
	targetFlagUnitRaid  uint32 = 0x4
	targetFlagUnitParty uint32 = 0x8
	targetFlagUnitEnemy uint32 = 0x80
	targetFlagUnitAlly  uint32 = 0x100
)

// spellExplicitUnitTargetMask mirrors the unit-target half of
// SpellImplicitTargetInfo::GetExplicitTargetMask (SpellInfo.cpp:134-210).
// Per effect and per TargetA/TargetB, unit targets with TARGET reference
// type map their selection check type to the validating flag:
// TARGET_CHECK_ENEMY -> TARGET_FLAG_UNIT_ENEMY, TARGET_CHECK_ALLY ->
// TARGET_FLAG_UNIT_ALLY, TARGET_CHECK_PARTY -> TARGET_FLAG_UNIT_PARTY,
// TARGET_CHECK_RAID -> TARGET_FLAG_UNIT_RAID. The table rows used are:
// 6 -> ENEMY; 21, 45 -> ALLY; 35 -> PARTY; 57 -> RAID (SpellInfo.cpp:226,
// 241, 255, 265, 277).
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
// (Unit::IsInPartyWith/Unit::IsInRaidWith, Unit.cpp:12126). Returns true
// when no branch passes and the cast must fail with SPELL_FAILED_BAD_TARGETS.
func (s *session) explicitTargetFactionBlocked(mask uint32, explicitUnitGUID uint64, tgt combatTarget) bool {
	if mask&(targetFlagUnitEnemy|targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid) == 0 {
		return false
	}
	targetSess := s.server.findSessionByGUID(explicitUnitGUID)
	friendly, hostile := false, false
	if targetSess != nil && targetSess.player != nil {
		// PvP: same alliance counts friendly, opposite hostile. C++ refines
		// this with PvP flags, FFA, duels and sanctuary - no Go model for
		// those terms.
		friendly = targetSess.playerAlliance() == s.playerAlliance()
		hostile = !friendly
	} else {
		caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
		friendly = s.server.isFriendlyFaction(tgt.Faction, caster)
		hostile = !friendly && s.server.isHostileFaction(tgt.Faction, caster)
	}
	if mask&targetFlagUnitEnemy != 0 && !friendly {
		return false
	}
	if mask&targetFlagUnitAlly != 0 && !hostile {
		return false
	}
	if mask&targetFlagUnitParty != 0 && !hostile && s.sameGroupAs(targetSess, false) {
		return false
	}
	if mask&targetFlagUnitRaid != 0 && !hostile && s.sameGroupAs(targetSess, true) {
		return false
	}
	return true
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

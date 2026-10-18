package world

import (
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Creature/pet-caster twins of the faction arms of
// SpellInfo::CheckExplicitTarget (SpellInfo.cpp:1793-1811), which
// Spell::CheckCast runs at Spell.cpp:5365 — ahead of the CheckTarget block
// (5372). The player path bridges these in explicit_target_faction.go; this
// file is the Server-level twin for creature and pet casters, where the
// caster is a creatureMotion and the affecting player (Unit::
// GetAffectingPlayer) is the owner session on the pet path or the resolved
// charm/owner session (nil for a wild caster) on the wild path.

const (
	creatureTypeFlagTreatAsRaidUnit uint32 = 0x04000000 // CREATURE_TYPE_FLAG_TREAT_AS_RAID_UNIT (SharedDefines.h:2755)
	creatureTypeFlagCanAssist       uint32 = 0x00001000 // CREATURE_TYPE_FLAG_CAN_ASSIST (SharedDefines.h:2743)
	creatureTypeFlagGhostVisible    uint32 = 0x00000002 // CREATURE_TYPE_FLAG_GHOST_VISIBLE (SharedDefines.h:2730)
	creatureTypeFlagInteractDead    uint32 = 0x00000080 // CREATURE_TYPE_FLAG_CAN_INTERACT_WHILE_DEAD (SharedDefines.h:2736)
	factionTemplateFlagContested    uint32 = 0x00001000 // FACTION_TEMPLATE_FLAG_CONTESTED_GUARD (DBCEnums.h:320)
)

// checkCreatureExplicitTargetFaction mirrors the ENEMY/ALLY/PARTY/RAID/
// MINIPET/PASSENGER arms of SpellInfo::CheckExplicitTarget
// (SpellInfo.cpp:1793-1811) for a creature/pet caster. attacker is the
// caster's affecting player session (nil when none resolves). Returns false
// when no arm passes and the cast must fizzle with SPELL_FAILED_BAD_TARGETS;
// both caller paths fizzle silently like the other CheckCast twins. C++
// only enters the faction block for a unit target when the explicit mask
// carries one of these bits (1793); the null-target arm is bridged
// separately by the callers.
func (s *Server) checkCreatureExplicitTargetFaction(m *creatureMotion, attacker *session, spell wotlk.Spell, targetGUID uint64) bool {
	if s == nil || m == nil || targetGUID == 0 {
		return true
	}
	mask := spellExplicitUnitTargetMask(spell)
	if mask&(targetFlagUnitEnemy|targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid|targetFlagUnitMinipet|targetFlagUnitPassenger) == 0 {
		return true
	}
	// TARGET_FLAG_UNIT_ENEMY arm (SpellInfo.cpp:1796-1798):
	// caster->IsValidAttackTarget(unitTarget, this).
	if mask&targetFlagUnitEnemy != 0 && s.creatureExplicitAttackPasses(m, attacker, targetGUID) {
		return true
	}
	// TARGET_FLAG_UNIT_ALLY/PARTY/RAID arms (SpellInfo.cpp:1799-1805): the
	// PARTY/RAID bits additionally require Unit::IsInPartyWith/
	// Unit::IsInRaidWith (Unit.cpp:12126/12152); every one of the three then
	// requires caster->IsValidAssistTarget(unitTarget, this).
	if mask&targetFlagUnitAlly != 0 ||
		(mask&targetFlagUnitParty != 0 && s.creatureInPartyWith(m, attacker, targetGUID)) ||
		(mask&targetFlagUnitRaid != 0 && s.creatureInRaidWith(m, attacker, targetGUID)) {
		if s.creatureExplicitAssistPasses(m, attacker, spell, targetGUID) {
			return true
		}
	}
	// TARGET_FLAG_UNIT_MINIPET arm (SpellInfo.cpp:1806-1808): the bit can
	// arrive via the raw DBC Targets field (folded by
	// spellExplicitUnitTargetMask, SpellInfo.cpp:3347); the implicit-target
	// and missing-mask paths never produce it. C++ passes only when the
	// target GUID equals the caster's UNIT_FIELD_CRITTER
	// (Unit::SetMinion add/remove arms, Unit.cpp:6085-6087/6111-6114).
	// Creature casters have no Go critter model, so the arm collapses to a
	// never-pass — the tail's BAD_TARGETS fizzle below is C++-exact for
	// them (a MINIPET-only mask fails unless the target is the caster's
	// own critter, which a creature never has).
	// TARGET_FLAG_UNIT_PASSENGER arm (SpellInfo.cpp:1809-1811):
	// unitTarget->IsOnVehicle(unitCaster) (Unit.cpp:12074-12077: the
	// target's vehicle kit is the caster's kit).
	if mask&targetFlagUnitPassenger != 0 && s.creatureTargetOnCasterVehicle(m, targetGUID) {
		return true
	}
	return false
}

// factionTemplatesHostile mirrors FactionTemplateEntry::IsHostileTo
// (DBCStructure.h:710-722) in C++ evaluation order: an enemy-list hit wins
// over a friend-list hit, then the enemy-group mask. This is the
// creature-vs-creature verdict — reputation, contested-guard and forced-rank
// terms are player-only in C++ (Object.cpp:2708-2840) and intentionally
// absent here.
func (s *Server) factionTemplatesHostile(a, b uint32) bool {
	if s.Data == nil || a == 0 || b == 0 {
		return false
	}
	aTpl, aFound, aErr := s.Data.FactionTemplate(a)
	bTpl, bFound, bErr := s.Data.FactionTemplate(b)
	if aErr != nil || bErr != nil || !aFound || !bFound {
		return false
	}
	for _, enemy := range aTpl.Enemies {
		if enemy != 0 && enemy == bTpl.Faction {
			return true
		}
	}
	for _, friend := range aTpl.Friends {
		if friend != 0 && friend == bTpl.Faction {
			return false
		}
	}
	return aTpl.EnemyGroup&bTpl.FactionGroup != 0
}

// factionTemplatesFriendly mirrors FactionTemplateEntry::IsFriendlyTo
// (DBCStructure.h:700-708): an enemy-list hit vetoes, then the friend list,
// then the friend/faction-group cross terms.
func (s *Server) factionTemplatesFriendly(a, b uint32) bool {
	if s.Data == nil || a == 0 || b == 0 {
		return false
	}
	aTpl, aFound, aErr := s.Data.FactionTemplate(a)
	bTpl, bFound, bErr := s.Data.FactionTemplate(b)
	if aErr != nil || bErr != nil || !aFound || !bFound {
		return false
	}
	for _, enemy := range aTpl.Enemies {
		if enemy != 0 && enemy == bTpl.Faction {
			return false
		}
	}
	for _, friend := range aTpl.Friends {
		if friend != 0 && friend == bTpl.Faction {
			return true
		}
	}
	return aTpl.FriendGroup&bTpl.FactionGroup != 0 || aTpl.FactionGroup&bTpl.FriendGroup != 0
}

// creatureExplicitTargetPlayerPos builds the playerPos view of a target
// player session for the faction verdicts, like creatureFriendlyToTarget
// (boss_ai.go) does.
func (s *Server) creatureExplicitTargetPlayerPos(ts *session) playerPos {
	return playerPos{
		FactionTemplate: ts.effectiveFactionTemplate(ts.player.Race),
		Race:            ts.player.Race,
		Class:           ts.player.Class,
		Sess:            ts,
		Reputations:     playerReputationMap(ts.player.Reputations),
	}
}

// creatureSameOwner mirrors the charmer/owner short-circuit of
// WorldObject::GetReactionTo (Object.cpp:2712-2714): units sharing a
// charmer-or-owner are always friendly to each other.
func creatureSameOwner(m *creatureMotion, targetGUID uint64, tm *creatureMotion, ts *session) bool {
	if m == nil {
		return false
	}
	if ts != nil && ts.player != nil {
		return m.OwnerGUID != 0 && ts.playerGUID == m.OwnerGUID
	}
	if tm == nil {
		return false
	}
	return m.OwnerGUID != 0 && tm.OwnerGUID == m.OwnerGUID
}

// playerAtWarWith reports the at-war term of the PvC attack path
// (Object.cpp:3036-3053) for a player session against a faction template's
// faction.
func (s *Server) playerAtWarWith(ts *session, factionTemplateID uint32) bool {
	if ts == nil || ts.player == nil || s.Data == nil {
		return false
	}
	tpl, found, err := s.Data.FactionTemplate(factionTemplateID)
	if err != nil || !found {
		return false
	}
	saved, ok := playerReputationMap(ts.player.Reputations)[tpl.Faction]
	return ok && saved.Flags&factionFlagAtWar != 0
}

// creatureContestedHostile mirrors the contested-guard hostile term
// (Object.cpp:2806-2809, 3041-3043): a contested-guard faction is hostile
// to a player carrying the contested-PvP flag.
func (s *Server) creatureContestedHostile(creatureFaction uint32, ts *session) bool {
	if s.Data == nil || ts == nil {
		return false
	}
	tpl, found, err := s.Data.FactionTemplate(creatureFaction)
	if err != nil || !found {
		return false
	}
	return tpl.Flags&factionTemplateFlagContested != 0 && ts.contestedPvPActive(time.Now())
}

// factionCanHaveReputation mirrors FactionEntry::CanHaveReputation
// (ReputationIndex >= 0): the reputation arm of GetReactionTo only runs for
// factions with a reputation row.
func (s *Server) factionCanHaveReputation(factionID uint32) bool {
	if s.Data == nil {
		return false
	}
	rep, found, err := s.Data.Reputation(factionID, 1, 1)
	return err == nil && found && rep.ReputationList >= 0
}

// creatureExplicitAttackPasses mirrors the TARGET_FLAG_UNIT_ENEMY arm's
// caster->IsValidAttackTarget(unitTarget, this) (Object.cpp:2930) for a
// creature/pet caster. Only the hostility verdict is bridged: the dead, GM
// and untargetable terms are outcome-equivalent with the already-bridged
// CheckTarget arms (dead/GM) and the CheckCast LOS arm — C++ evaluates them
// here at Spell.cpp:5365 but the cast fizzles either way on these silent
// paths — while CanSeeOrDetect, UNIT_STATE_UNATTACKABLE, the immunity
// flags, the trap-owner arms and the npcbot arms have no Go model
// (documented no-bridge). Unresolvable targets fail open, like the LOS
// arms.
func (s *Server) creatureExplicitAttackPasses(m *creatureMotion, attacker *session, targetGUID uint64) bool {
	// Creature attackers carrying CREATURE_TYPE_FLAG_TREAT_AS_RAID_UNIT
	// never attack (Object.cpp:3055-3057).
	if m.TypeFlags&creatureTypeFlagTreatAsRaidUnit != 0 {
		return false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return s.creatureAttackPlayerPasses(m, attacker, ts)
	}
	tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID)
	if tm == nil {
		return true
	}
	return s.creatureAttackCreaturePasses(m, attacker, tm)
}

// creatureAttackPlayerPasses mirrors IsValidAttackTarget against a player
// target. With an affecting player (pet/charmed caster) C++ runs the
// player-controlled branch of GetReactionTo (Object.cpp:2736-2785): duel,
// same-group, FFA and the at-war reputation term, then the PvP tail
// (Object.cpp:3059-3080). A wild caster has no affecting player, so the PvP
// tail never runs and the verdict is the both-direction friendly refusal
// (Object.cpp:3028-3030) plus the contested-guard hostile term.
func (s *Server) creatureAttackPlayerPasses(m *creatureMotion, attacker, ts *session) bool {
	tpos := s.creatureExplicitTargetPlayerPos(ts)
	if creatureSameOwner(m, ts.playerGUID, nil, ts) {
		return false
	}
	if attacker != nil && attacker.player != nil {
		// Duel term (Object.cpp:2744-2746): the owner's duel opponent is
		// always hostile to the pet.
		if attacker.duelPartner != 0 && attacker.duelPartner == ts.playerGUID &&
			attacker.player.DuelTeam != 0 && ts.player.DuelTeam != 0 {
			return true
		}
		// Same-group term (Object.cpp:2748-2751): the owner's group/raid
		// reads friendly, so the pet cannot attack into it.
		if attacker.sameGroupAs(ts, false) || attacker.sameGroupAs(ts, true) {
			return false
		}
		// FFA term (Object.cpp:2753-2754): both sides FFA-flagged are
		// hostile.
		if attacker.player.PVPFlags&pvpFlagFFA != 0 && ts.player.PVPFlags&pvpFlagFFA != 0 {
			return true
		}
		// Reputation term (Object.cpp:2763-2785): for a faction with a
		// reputation row the verdict is the at-war flag alone; without a
		// row C++ falls through to the faction-template verdict. The
		// forced-rank arm (GetForcedRankIfAny) has no Go model.
		tpl, found, err := s.Data.FactionTemplate(tpos.FactionTemplate)
		if err == nil && found && s.factionCanHaveReputation(tpl.Faction) {
			if !s.playerAtWarWith(attacker, tpos.FactionTemplate) {
				return false
			}
		} else if !s.factionTemplatesHostile(m.Faction, tpos.FactionTemplate) {
			return false
		}
		// PvP tail (Object.cpp:3059-3080) via the player-path twin.
		return !attacker.explicitTargetAttackPvPBlocked(ts, false)
	}
	if s.creatureContestedHostile(m.Faction, ts) {
		return true
	}
	return !s.isFriendlyFaction(m.Faction, tpos) && !s.factionTemplatesFriendly(tpos.FactionTemplate, m.Faction)
}

// creatureAttackCreaturePasses mirrors IsValidAttackTarget against a
// creature target. Wild casters run the CvC arm (Object.cpp:3018-3020):
// hostile in either direction. A player-controlled caster (pet/charmed)
// runs the PvC arm (Object.cpp:3022-3053): the friendly refusal first, then
// the contested-guard carve-out and the at-war term against the target's
// faction — with a reputation row the at-war flag alone decides (not at war
// means no attack even when the templates are hostile); without a row the
// template hostility stands.
func (s *Server) creatureAttackCreaturePasses(m *creatureMotion, attacker *session, tm *creatureMotion) bool {
	if creatureSameOwner(m, tm.GUID, tm, nil) {
		return false
	}
	hostile := s.factionTemplatesHostile(m.Faction, tm.Faction) || s.factionTemplatesHostile(tm.Faction, m.Faction)
	if attacker == nil || attacker.player == nil {
		return hostile
	}
	if s.factionTemplatesFriendly(m.Faction, tm.Faction) || s.factionTemplatesFriendly(tm.Faction, m.Faction) {
		return false
	}
	tpl, found, err := s.Data.FactionTemplate(tm.Faction)
	if err != nil || !found {
		return hostile
	}
	if tpl.Flags&factionTemplateFlagContested != 0 && attacker.contestedPvPActive(time.Now()) {
		return true
	}
	if saved, ok := playerReputationMap(attacker.player.Reputations)[tpl.Faction]; ok {
		return saved.Flags&factionFlagAtWar != 0
	}
	return hostile
}

// creatureHostileToTarget mirrors the caster→target direction of the
// "can't assist non-friendly targets" refusal (Object.cpp:3145): the
// reaction must be below REP_NEUTRAL in both directions to fail. Against a
// player the CvP at-war clamp applies (Object.cpp:2817-2823): at war the
// rank never exceeds neutral, so it is not hostile either.
func (s *Server) creatureHostileToTarget(m *creatureMotion, targetGUID uint64) bool {
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if creatureSameOwner(m, targetGUID, nil, ts) {
			return false
		}
		tpos := s.creatureExplicitTargetPlayerPos(ts)
		if s.creatureContestedHostile(m.Faction, ts) {
			return true
		}
		return s.isHostileFaction(m.Faction, tpos) && !s.playerAtWarWith(ts, m.Faction)
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		if creatureSameOwner(m, targetGUID, tm, nil) {
			return false
		}
		return s.factionTemplatesHostile(m.Faction, tm.Faction)
	}
	return false
}

// creatureTargetHostileTo mirrors the target→caster direction of the
// non-friendly refusal: a player target uses the faction-only tail
// (Object.cpp:2800-2840 — reputation terms are the caster's, not the
// target's, so only the template terms apply; forced ranks unmodeled).
func (s *Server) creatureTargetHostileTo(m *creatureMotion, targetGUID uint64) bool {
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if creatureSameOwner(m, targetGUID, nil, ts) {
			return false
		}
		return s.factionTemplatesHostile(s.creatureExplicitTargetPlayerPos(ts).FactionTemplate, m.Faction)
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		if creatureSameOwner(m, targetGUID, tm, nil) {
			return false
		}
		return s.factionTemplatesHostile(tm.Faction, m.Faction)
	}
	return false
}

// creatureExplicitAssistPasses mirrors the TARGET_FLAG_UNIT_ALLY arm's
// caster->IsValidAssistTarget(unitTarget, this) (Object.cpp:3087) for a
// creature/pet caster. Bridged: the self arm, the both-direction
// non-friendly refusal (with the TREAT_AS_RAID_UNIT caster exemption), the
// PvP tail when both sides are player-controlled, and the PvC type-flags
// arm. No-bridge: the dead and GM terms (outcome-equivalent with the
// bridged CheckTarget arms), CanSeeOrDetect, UNIT_STATE_UNATTACKABLE,
// UNIT_FLAG_NOT_SELECTABLE, the immunity flags, the caster-on-vehicle
// exemption, and the npcbot arms. The target-side IsPvP term of the PvC arm
// has no creature-side Go model and fails open.
func (s *Server) creatureExplicitAssistPasses(m *creatureMotion, attacker *session, spell wotlk.Spell, targetGUID uint64) bool {
	// Can always assist self (Object.cpp:3092).
	if targetGUID == m.GUID {
		return true
	}
	// Can't assist non-friendly targets (Object.cpp:3145-3148): the refusal
	// needs both directions below neutral; a TREAT_AS_RAID_UNIT caster is
	// exempt.
	if m.TypeFlags&creatureTypeFlagTreatAsRaidUnit == 0 &&
		s.creatureHostileToTarget(m, targetGUID) && s.creatureTargetHostileTo(m, targetGUID) {
		return false
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if attacker != nil {
			// PvP tail (Object.cpp:3149-3171): both sides
			// player-controlled, via the player-path twin.
			return !attacker.explicitTargetAssistPvPBlocked(ts)
		}
		return true
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil && attacker != nil {
		// PvC arm (Object.cpp:3172-3180): a player-controlled caster
		// assists a creature only with the raid-unit/can-assist type
		// flags, unless the spell ignores the immune flag.
		if spell.AttributesEx6&spellAttr6AssistIgnoreImmuneFlag == 0 {
			return tm.TypeFlags&(creatureTypeFlagTreatAsRaidUnit|creatureTypeFlagCanAssist) != 0
		}
	}
	return true
}

// creatureInPartyWith / creatureInRaidWith mirror the PARTY/RAID membership
// terms of Unit::IsInPartyWith / Unit::IsInRaidWith (Unit.cpp:12126/12152):
// u1 is the caster's charmer-or-owner-or-self, u2 the target's; both
// players compare groups, a player+unit pair passes on
// TREAT_AS_RAID_UNIT, and a unit+unit pair passes on equal faction
// templates (Unit::GetFaction is UNIT_FIELD_FACTIONTEMPLATE, Unit.h:906).
// The npcbot arms have no Go model.
func (s *Server) creatureInPartyWith(m *creatureMotion, attacker *session, targetGUID uint64) bool {
	return s.creatureInGroupWith(m, attacker, targetGUID, false)
}

func (s *Server) creatureInRaidWith(m *creatureMotion, attacker *session, targetGUID uint64) bool {
	return s.creatureInGroupWith(m, attacker, targetGUID, true)
}

func (s *Server) creatureInGroupWith(m *creatureMotion, attacker *session, targetGUID uint64, raid bool) bool {
	if targetGUID == m.GUID {
		return true
	}
	var u1Player *session
	var u1Motion *creatureMotion
	if attacker != nil {
		u1Player = attacker
	} else {
		u1Motion = m
	}
	var u2Player *session
	var u2Motion *creatureMotion
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		u2Player = ts
	} else if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		if os := s.findSessionByGUID(tm.OwnerGUID); tm.OwnerGUID != 0 && os != nil {
			u2Player = os
		} else {
			u2Motion = tm
		}
	} else {
		return false
	}
	if u1Player != nil && u2Player != nil {
		if u1Player == u2Player {
			return true
		}
		return u1Player.sameGroupAs(u2Player, raid)
	}
	if u1Player != nil && u2Motion != nil {
		return u2Motion.TypeFlags&creatureTypeFlagTreatAsRaidUnit != 0
	}
	if u2Player != nil && u1Motion != nil {
		return u1Motion.TypeFlags&creatureTypeFlagTreatAsRaidUnit != 0
	}
	return u1Motion != nil && u2Motion != nil && u1Motion.Faction == u2Motion.Faction
}

// creatureTargetOnCasterVehicle mirrors the TARGET_FLAG_UNIT_PASSENGER arm
// (SpellInfo.cpp:1809-1811): unitTarget->IsOnVehicle(unitCaster)
// (Unit.cpp:12074-12077 — the target's vehicle kit is the caster's kit, so
// the target must occupy one of the caster's seats).
func (s *Server) creatureTargetOnCasterVehicle(m *creatureMotion, targetGUID uint64) bool {
	kit := s.getVehicleKit(m.Map, m.InstanceID, m.GUID)
	if kit == nil {
		return false
	}
	seatID, _, _ := kit.GetSeatForPassenger(targetGUID)
	return seatID >= 0
}

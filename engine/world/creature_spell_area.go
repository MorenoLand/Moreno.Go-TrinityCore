package world

import (
	"context"
)

// SpellInfo::CheckLocation spell_area DB leg for creature casters
// (SpellInfo.cpp:1558-1567; SpellArea::IsFitToRequirements, SpellMgr.cpp:689).
// CheckLocation runs this leg with m_caster->ToPlayer() — null for creature
// casters — and every constrained term needs a non-null player: gender
// (691), raceMask (695), questStart/questEnd (703/707), auraSpell (711) all
// fail for a null player, and the extra-conditions switch (58600/58730
// no-fly zones, 56617/56618 WG factory phase, 57940/58045 Essence of
// Wintergrasp, 74411 Dampening) is player-gated too. Only the area term can
// pass for a creature, so a spell with spell_area rows fails
// (SPELL_FAILED_INCORRECT_AREA) unless some row is unconstrained for a null
// player. The player-battleground leg inside IsFitToRequirements is vacuous
// (player is null). Runs for triggered casts too (no C++ triggered
// exemption).

// spellAreaExtraConditionSpell reports whether the spell carries one of the
// player-gated extra conditions inside SpellArea::IsFitToRequirements
// (SpellMgr.cpp:718-781): every case opens with "if (!player) return false",
// so no rule can fit for a creature caster.
func spellAreaExtraConditionSpell(spellID uint32) bool {
	switch spellID {
	case 58600, 58730, // no-fly zones: Dalaran, Wintergrasp
		56618, 56617, // WG factory phase shift: Horde, Alliance
		57940, 58045, // Essence of Wintergrasp
		74411: // Battleground Dampening
		return true
	}
	return false
}

// creatureSpellAreaRuleFitsNullPlayer is the IsFitToRequirements twin for a
// null player: the area term is the only one that can pass.
func creatureSpellAreaRuleFitsNullPlayer(rule spellAreaRule, zoneID, areaID uint32) bool {
	if rule.Gender != 2 { // GENDER_NONE
		return false
	}
	if rule.RaceMask != 0 {
		return false
	}
	if rule.Area != 0 && rule.Area != zoneID && rule.Area != areaID {
		return false
	}
	if rule.QuestStart != 0 || rule.QuestEnd != 0 {
		return false
	}
	if rule.AuraSpell != 0 {
		return false
	}
	return true
}

// creatureSpellAreaRulesFitNullPlayer reports whether at least one spell_area
// rule fits for a creature caster at the given zone/area.
func creatureSpellAreaRulesFitNullPlayer(spellID uint32, rules []spellAreaRule, zoneID, areaID uint32) bool {
	if spellAreaExtraConditionSpell(spellID) {
		return false
	}
	for _, rule := range rules {
		if creatureSpellAreaRuleFitsNullPlayer(rule, zoneID, areaID) {
			return true
		}
	}
	return false
}

// creatureSpellAreaRules is the server-level spell_area query for the
// creature path; ok is false when no rows exist (same convention as the
// session twin).
func (s *Server) creatureSpellAreaRules(ctx context.Context, spellID uint32) ([]spellAreaRule, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return nil, false
	}
	return fetchSpellAreaRules(ctx, s.WorldStore.DB, spellID)
}

package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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

// checkMotionSpellCheckLocation is the SpellInfo::CheckLocation twin
// (SpellInfo.cpp:1505-1567) for motion casters (wild creatures and pets).
// Zone/area come from the motion position (Server.zoneAndAreaID, the Go
// model of Unit::GetZoneAndAreaId); the caster's ToPlayer() is nil in C++
// here, and the npcbot exemption is vacuous (no npcbot model). Bridged
// legs: AreaGroupId (1508-1527, SPELL_FAILED_INCORRECT_AREA — unknown
// group data keeps the fail-open bridge convention) and
// SPELL_ATTR6_NOT_IN_RAID_INSTANCE (1552-1556). No bridge:
// SPELL_ATTR4_CAST_ONLY_IN_OUTLAND (1529-1550, session-twin parity) and
// the battleground-spell switch (1569-1631, every arm needs a non-nil
// player; creatures never cast flag spells). The spell_area DB leg
// (1558-1567) runs with the null-player IsFitToRequirements twin
// (creatureSpellAreaRulesFitNullPlayer) after the raid-instance arm (1552)
// and ahead of the battleground-spell switch (1569), matching C++
// relative order. Runs for triggered casts too (no C++ triggered
// exemption); false means fizzle (no client on this path).
func (s *Server) checkMotionSpellCheckLocation(ctx context.Context, m *creatureMotion, spell wotlk.Spell) bool {
	if m == nil {
		return true
	}
	if spell.AreaGroupID > 0 {
		zoneID, areaID := s.zoneAndAreaID(m.Map, m.X, m.Y, m.Z, 0)
		if allowed, known, groupErr := s.Data.AreaGroupAllows(uint32(spell.AreaGroupID), zoneID, areaID); groupErr == nil && known && !allowed {
			return false
		}
	}
	if spell.AttributesEx6&spellAttr6NotInRaidInstance != 0 {
		if entry, found, err := s.Data.Map(m.Map); err != nil || !found || entry.IsRaid() {
			return false
		}
	}
	if rules, hasRules := s.creatureSpellAreaRules(ctx, spell.ID); hasRules {
		zoneID, areaID := s.zoneAndAreaID(m.Map, m.X, m.Y, m.Z, 0)
		if !creatureSpellAreaRulesFitNullPlayer(spell.ID, rules, zoneID, areaID) {
			return false
		}
	}
	return true
}

// applyCreatureSpellAreaRows mirrors the spell_area arm of
// Aura::HandleAuraSpecificMods (SpellAuras.cpp:1311-1330) for creature
// targets: at aura apply AND remove, every spell_area row keyed by the aura
// (aura_spell) is enforced immediately — rows whose requirements no longer
// fit drop the row's spell from the creature, fitting autocast rows are cast
// when missing. C++ resolves the target's zone/area and passes a null player
// for non-players, so the null-player IsFitToRequirements twin
// (creatureSpellAreaRuleFitsNullPlayer) is the fit test. The fitting
// autocast cast is target->CastSpell(target, spellId, true): a triggered
// self-cast, which runs through the creature cast funnel when the motion
// still resolves.
func (s *Server) applyCreatureSpellAreaRows(key creatureAuraKey, auraSpellID uint32) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || key.GUID == 0 {
		return
	}
	ctx := context.Background()
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT spell, area, quest_start, quest_end, racemask, gender, autocast, quest_start_status, quest_end_status FROM spell_area WHERE aura_spell = ?`, auraSpellID)
	if err != nil {
		return
	}
	defer rows.Close()
	var zoneID, areaID uint32
	motion := s.findCreatureMotion(key.Map, key.InstanceID, key.GUID)
	if motion != nil {
		zoneID, areaID = s.zoneAndAreaID(motion.Map, motion.X, motion.Y, motion.Z, 0)
	}
	for rows.Next() {
		var spellID uint32
		var rule spellAreaRule
		var gender, autocast int64
		if err := rows.Scan(&spellID, &rule.Area, &rule.QuestStart, &rule.QuestEnd, &rule.RaceMask, &gender, &autocast, &rule.QuestStartStatus, &rule.QuestEndStatus); err != nil {
			continue
		}
		rule.Gender, rule.Autocast = uint8(gender), autocast != 0
		fits := creatureSpellAreaRuleFitsNullPlayer(rule, zoneID, areaID)
		if fits && spellAreaExtraConditionSpell(spellID) {
			fits = false
		}
		if !fits {
			s.removeCreatureAura(key, spellID)
			continue
		}
		if rule.Autocast {
			s.auraMu.Lock()
			_, has := s.activeCreatureAuras[key][spellID]
			s.auraMu.Unlock()
			if !has && motion != nil {
				s.castCreatureSpellInternal(ctx, motion, spellID, key.GUID, true)
			}
		}
	}
}

// applyCreatureLinkedApplyArms mirrors the apply arm of
// Aura::HandleAuraSpecificMods' spell_linked block (SpellAuras.cpp:1336-1350)
// for creature targets: rows of spell_linked_spell with type 2
// (SPELL_LINK_AURA) keyed by the applied aura fire on application.
// Negative ids ApplySpellImmune the linked spell on the target — Go has no
// ApplySpellImmune model (immunity.go), so that arm stays unbridged, like the
// player twin. Positive ids AddAura the linked spell on the creature target
// from the aura's caster: Go runs them through the session's aura apply path
// (which carries the creature-target branch) with the recorded caster, the
// same model as the player twin (applyAuraLinkedAurasAtApply in spells.go).
// Caster level for duration/amount follows the Aura ctor's _casterInfo.Level
// twin (auraStoredCasterLevel): live session level, same-map motion level,
// else the spell's SpellLevel. The onReapply stack-amount propagate arm
// (SpellAuras.cpp:1381) has no linked-aura registry in Go and stays open,
// like the player twin.
func (s *session) applyCreatureLinkedApplyArms(ctx context.Context, targetGUID uint64, spellID uint32, casterGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	caster := casterGUID
	if caster == 0 {
		caster = s.playerGUID
	}
	for _, id := range spellLinkedEffects(ctx, s.server.WorldStore.DB, int32(spellID), spellLinkAuraType) {
		if id <= 0 {
			continue
		}
		linked, found, err := s.server.Data.Spell(uint32(id))
		if err != nil || !found {
			continue
		}
		casterLevel := uint32(s.auraStoredCasterLevel(caster, linked))
		var durationMs uint32
		if linked.DurationIndex != 0 {
			if dur, ok, derr := s.server.Data.SpellDuration(linked.DurationIndex, casterLevel); derr == nil && ok && dur > 0 {
				durationMs = uint32(dur)
			}
		}
		castMerged := make(map[uint64]struct{})
		for effIndex := range linked.Effects {
			eff := linked.Effects[effIndex]
			if eff.Aura == 0 {
				continue
			}
			var amount uint32
			if bp := eff.CalcValueForLevel(linked, casterLevel); bp > 0 {
				amount = uint32(bp)
			}
			s.applyAuraToTarget(ctx, targetGUID, linked, eff, effIndex, durationMs, eff.AuraPeriod, amount, linked.SchoolMask, castMerged, false, caster, false, 0, false, nil)
		}
	}
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

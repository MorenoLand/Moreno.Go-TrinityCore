package world

import "github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"

type statCandidate struct {
	order       int // Slot order — apply-order proxy for tie-breaking
	spellID     uint32
	effectIndex int
	amount      int32
	firstRank   uint32
	auraType    uint32 // Aura type of the candidate effect (0 when unused)
}

// auraStatMods mirrors the flat/pct stat-aura handlers
// (AuraEffect::HandleAuraModStat / HandleModPercentStat /
// HandleModTotalPercentStat, SpellAuraEffects.cpp:3379-3510): aura 29
// (MOD_STAT) applies flat TOTAL_VALUE modifiers per stat index
// (misc 0..4; -1/-2 = all five; other misc values are rejected like the
// C++ log-and-return), aura 80 (MOD_PERCENT_STAT) multiplies the
// BASE_PCT per stat (misc 0..4; -1 = all five), and aura 137
// (MOD_TOTAL_STAT_PERCENTAGE) multiplies the TOTAL_PCT per stat
// (misc 0..4; -1 = all five). C++ gates 80/137 to players; Go has no
// creature stat model, so the whole fold is player-scoped and the
// 29-on-creature leg stays a documented gap.
//
// The exclusive-same-effect gate (Unit::GetHighestExclusiveSameEffectSpellGroupValue,
// Unit.cpp:13967; SpellMgr::AddSameEffectStackRuleSpellGroups,
// SpellMgr.cpp:404; and the remove-leg reinstatement in
// HandleAuraModStat:3404-3413) reduces to winner-takes-all per stat:
// among contending effects in a common group carrying
// SPELL_GROUP_STACK_RULE_EXCLUSIVE_SAME_EFFECT, only the highest-|amount|
// one applies. Ties resolve to the earliest-applied record (Slot order),
// matching C++'s first-wins on the apply leg; C++'s mutual-dormancy
// quirk for exact same-spell ties is not replicated. A spell is assumed
// to sit in at most one EXCLUSIVE_SAME_EFFECT group per aura type, as
// C++ itself asserts (SpellMgr.cpp:404).
func (s *session) auraStatMods(excludeSpellID uint32, extra ...*activeAura) (flat [5]int32, baseMult, totalMult [5]float32) {
	for i := range baseMult {
		baseMult[i] = 1
		totalMult[i] = 1
	}
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return flat, baseMult, totalMult
	}
	var c29, c80, c137 [5][]statCandidate
	auras := s.loadedAuras()
	auras = append(auras, extra...)
	for i, aura := range auras {
		if aura == nil || aura.SpellID == excludeSpellID {
			continue
		}
		// Same Stopped convention as auraHealthMods: the extra records
		// (the just-removed aura on the remove leg) are already flagged
		// Stopped, and only the live map records honor Stopped here.
		if i < len(auras)-len(extra) && aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			c := statCandidate{order: i, spellID: aura.SpellID, effectIndex: index, amount: aura.Amounts[index], firstRank: s.server.spellFirstRank(aura.SpellID)}
			switch effect.Aura {
			case spellAuraModStat:
				if effect.MiscValue < -2 || effect.MiscValue > 4 {
					continue
				}
				if effect.MiscValue < 0 {
					for st := 0; st < 5; st++ {
						c29[st] = append(c29[st], c)
					}
				} else {
					c29[effect.MiscValue] = append(c29[effect.MiscValue], c)
				}
			case spellAuraModPercentStat, spellAuraModTotalStatPct:
				if effect.MiscValue < -1 || effect.MiscValue > 4 {
					continue
				}
				dst := &c80
				if effect.Aura == spellAuraModTotalStatPct {
					dst = &c137
				}
				if effect.MiscValue < 0 {
					for st := 0; st < 5; st++ {
						(*dst)[st] = append((*dst)[st], c)
					}
				} else {
					(*dst)[effect.MiscValue] = append((*dst)[effect.MiscValue], c)
				}
			}
		}
	}
	cache := s.server.petAuraStackGroups()
	for st := 0; st < 5; st++ {
		for _, w := range statWinners(c29[st], cache) {
			flat[st] += w.amount
		}
		m := float32(1)
		for _, w := range statWinners(c80[st], cache) {
			m *= (100 + float32(w.amount)) / 100
		}
		baseMult[st] = m
		m = 1
		for _, w := range statWinners(c137[st], cache) {
			m *= (100 + float32(w.amount)) / 100
		}
		totalMult[st] = m
	}
	return flat, baseMult, totalMult
}

// statWinners applies the exclusive-same-effect gate to one stat's
// candidate list: a candidate is suppressed when a peer sharing a
// common EXCLUSIVE_SAME_EFFECT group has a strictly larger |amount|,
// or an equal |amount| with an earlier application order. Ungrouped
// candidates always apply.
func statWinners(cands []statCandidate, cache *petAuraStackCache) []statCandidate {
	var out []statCandidate
	for _, c := range cands {
		suppressed := false
		for _, p := range cands {
			if p.spellID == c.spellID && p.effectIndex == c.effectIndex && p.order == c.order {
				continue
			}
			if !sharesExclusiveSameEffectGroup(c.firstRank, p.firstRank, cache) {
				continue
			}
			pa, ca := absAuraAmount(p.amount), absAuraAmount(c.amount)
			if pa > ca || (pa == ca && p.order < c.order) {
				suppressed = true
				break
			}
		}
		if !suppressed {
			out = append(out, c)
		}
	}
	return out
}

// sharesExclusiveSameEffectGroup mirrors the group-membership half of
// GetHighestExclusiveSameEffectSpellGroupValue (Unit.cpp:13967-13992):
// the two first-rank spells share at least one spell group carrying
// SPELL_GROUP_STACK_RULE_EXCLUSIVE_SAME_EFFECT.
func sharesExclusiveSameEffectGroup(first1, first2 uint32, cache *petAuraStackCache) bool {
	if cache == nil {
		return false
	}
	g1, ok1 := cache.spellGroups[first1]
	if !ok1 {
		return false
	}
	g2, ok2 := cache.spellGroups[first2]
	if !ok2 {
		return false
	}
	for g := range g1 {
		if cache.groupRules[g] != spellGroupStackRuleExclusiveSameEffect {
			continue
		}
		if _, ok := g2[g]; ok {
			return true
		}
	}
	return false
}

// auraTouchesStats reports whether any of the aura's live effects is a
// stat modifier (29/80/137), mirroring the three handlers' table rows
// (SpellAuraEffects.cpp:94/145/202).
func (s *session) auraTouchesStats(aura *activeAura) bool {
	for _, auraType := range []uint32{spellAuraModStat, spellAuraModPercentStat, spellAuraModTotalStatPct} {
		if s.activeAuraHasEffect(aura, auraType) {
			return true
		}
	}
	return false
}

// spellHasStatModAura reports whether any effect of the spell feeds
// UNIT_MOD_STAT_* through AuraEffect::HandleStatFlatModifier (29,
// SpellAuraEffects.cpp:3379-3413) or HandleModTotalPercentStat (137, :3525+).
// Those are the auras whose apply/remove runs Guardian::UpdateStats via
// Unit::UpdateUnitMod (Unit.cpp:9304-9310) — recomputing the pet's aura-fed
// UNIT_FIELD_STAT0+stat fields and the stamina/intellect-fed maxima.
// Aura 80 (HandleModPercentStat) is player-only ("only players have base
// stats", SpellAuraEffects.cpp:3427) and never touches pet stats.
func spellHasStatModAura(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Aura == spellAuraModStat || eff.Aura == spellAuraModTotalStatPct {
			return true
		}
	}
	return false
}

// spellHasHealthPowerAura reports whether the spell carries any
// UNIT_MOD_HEALTH / UNIT_MOD_POWER modifier aura (34/230/250/133/282 health,
// 35/132 power) — the trigger gate for the max-health/max-power recompute
// beside spellHasStatModAura's 29/137 pair (SpellAuraEffects.cpp:3650-3768).
func spellHasHealthPowerAura(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		switch eff.Aura {
		case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2,
			spellAuraModIncreaseHealthPercent, spellAuraModBaseHealthPct,
			spellAuraModIncreaseEnergy, spellAuraModIncreaseEnergyPercent:
			return true
		}
	}
	return false
}

// auraHealthPowerHalves captures the per-spell current-health/power halves of
// the triggering health/power aura (SpellAuraEffects.cpp:3650-3768):
// 34/230/250 heal/drop current health by the flat amount (:3650-3665),
// 133 re-anchors current health to the pre-change health pct on apply and
// remove alike (:3729-3754), 282 adjusts no current health (:3757-3768),
// 132 shifts current power by the max-power change per misc-gated power
// (:3698-3727), 35 adjusts no current power. The 34 amount resolves from the
// live record on the apply leg (registered before the trigger fires); on the
// remove leg the record is already erased (Unit::_UnapplyAura erases before
// the handlers, Unit.cpp:3548), so the trigger passes a pre-erase snapshot
// of the erased record — C++ remove uses the stored effect amount
// (amt = apply ? GetAmount() : -GetAmount(), :3658), never a fresh DBC
// lookup. Effects whose mask bit is clear never ran a C++ handler, so they
// contribute nothing — the same gate the live folds use. Callers must not
// hold auraMu.
type auraHealthPowerHalves struct {
	flatHealth   int32
	hasHealthPct bool
	powerPct     [7]bool
}

func (s *Server) creatureAuraHealthPowerHalves(key creatureAuraKey, spell wotlk.Spell, spellID uint32, apply bool, removed *activeAura) auraHealthPowerHalves {
	var h auraHealthPowerHalves
	var rec *activeAura
	if apply && s != nil {
		s.auraMu.Lock()
		if r := s.activeCreatureAuras[key][spellID]; r != nil && !r.Stopped {
			rec = r
		}
		s.auraMu.Unlock()
	} else {
		// The erase already ran; the snapshot carries the stored amounts
		// the C++ remove leg reads from the AuraEffect object.
		rec = removed
	}
	if rec == nil || rec.EffectMask == 0 {
		return h
	}
	liveAmounts := map[int]int32{}
	for i, v := range rec.Amounts {
		liveAmounts[i] = v
	}
	usedStoredAmount := false
	for index, eff := range spell.Effects {
		if rec.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		switch eff.Aura {
		case spellAuraModIncreaseHealth, spellAuraModIncreaseHealthOld, spellAuraModIncreaseHealth2:
			amt, ok := liveAmounts[index]
			if !ok || amt == 0 {
				if rec.AuraType == eff.Aura && !usedStoredAmount {
					amt = int32(rec.Amount)
					usedStoredAmount = true
				}
			}
			if amt == 0 {
				amt = eff.BasePoints
			}
			h.flatHealth += amt
		case spellAuraModIncreaseHealthPercent:
			h.hasHealthPct = true
		case spellAuraModIncreaseEnergyPercent:
			if p := eff.MiscValue; p >= 0 && p < 7 {
				h.powerPct[p] = true
			}
		}
	}
	return h
}

// statStaminaIndex is STAT_STAMINA (SharedDefines.h: the Stats enum runs
// 0 strength, 1 agility, 2 stamina, 3 intellect, 4 spirit).
const statStaminaIndex = 2

// statAuraHealthPreserveForEffect captures the pre-recalc health state
// for the HandleModTotalPercentStat stamina arm
// (SpellAuraEffects.cpp:3497-3502): a 137 aura touching stamina on an
// SPELL_ATTR0_ABILITY spell re-anchors current health to the pre-apply
// pct of the new max. Returns nil when the arm does not apply; the
// returned func must run after the stat recalc.
func (s *session) statAuraHealthPreserveForEffect(spell wotlk.Spell, eff wotlk.SpellEffect) func() {
	if eff.Aura != spellAuraModTotalStatPct || (eff.MiscValue != statStaminaIndex && eff.MiscValue != -1) {
		return nil
	}
	return s.captureStatHealthPreserve(spell)
}

// statAuraHealthPreserveForAura is the remove-leg variant: the erased
// record's own effect mask decides which 137 effects ran.
func (s *session) statAuraHealthPreserveForAura(spellID uint32, aura *activeAura) func() {
	if aura == nil || s == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return nil
	}
	for index, effect := range spell.Effects {
		if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		if effect.Aura == spellAuraModTotalStatPct && (effect.MiscValue == statStaminaIndex || effect.MiscValue == -1) {
			return s.captureStatHealthPreserve(spell)
		}
	}
	return nil
}

// captureStatHealthPreserve snapshots the current health pct against the
// live (pre-recalc) max and returns the re-anchor func. C++
// (SpellAuraEffects.cpp:3497-3502): SetHealth(max(pct * newMax,
// zeroHealth ? 0 : 1)); zeroHealth is the dead/ghost state, which Go
// tracks as Health == 0.
func (s *session) captureStatHealthPreserve(spell wotlk.Spell) func() {
	if s == nil || s.player == nil || spell.Attributes&spellAttr0Ability == 0 {
		return nil
	}
	var pct float64
	if max := s.effectiveMaxHealth(); max > 0 && s.player.Health > 0 {
		pct = float64(s.player.Health) / float64(max)
	}
	zeroHealth := s.player.Health == 0
	return func() {
		newMax := s.effectiveMaxHealth()
		nh := uint32(pct * float64(newMax))
		if !zeroHealth && nh < 1 {
			nh = 1
		}
		s.player.Health = nh
	}
}

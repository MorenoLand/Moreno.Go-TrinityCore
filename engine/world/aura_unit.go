package world

// playerFactionForBroadcast mirrors the UNIT_FIELD_FACTION write of
// HandleAuraModFaction (SpellAuraEffects.cpp): an active 243 effect overrides
// the race faction with its misc value; the remove leg restores the race
// faction via forcedFactionAura's no-aura fallback.
func (s *session) playerFactionForBroadcast(race uint8) uint32 {
	if s != nil {
		if forced, ok := s.forcedFactionAura(); ok {
			return forced
		}
	}
	if s == nil || s.server == nil {
		return 0
	}
	return s.server.raceFaction(race)
}

// auraObjectScale mirrors Unit::RecalculateObjectScale (Unit.cpp:10593-10598):
// the native player scale (1.0) plus (sum of SPELL_AURA_MOD_SCALE (61) and
// SPELL_AURA_MOD_SCALE_2 (239) amounts)/100 — C++ CalculatePct(1.0f,
// scaleAuras) is scaleAuras/100 — floored at 0.1 for players. The AuraEffect
// legs (HandleAuraModScale, SpellAuraEffects.cpp) only recalculate, so a live
// fold over the session's auras matches with no apply/remove hook.
func (s *session) auraObjectScale() float32 {
	if s == nil {
		return 1
	}
	scale := 1.0 + float64(s.totalAuraModifier(spellAuraModScale)+s.totalAuraModifier(spellAuraModScale2))/100
	if scale < 0.1 {
		scale = 0.1
	}
	return float32(scale)
}

// forcedFactionAura mirrors the AuraEffect::HandleAuraModFaction apply leg
// (SpellAuraEffects.cpp): while an SPELL_AURA_MOD_FACTION (243) effect is
// active the unit's faction is the effect's misc value instead of the race
// faction. When several are active the latest-applied one wins (C++ re-sets
// per apply); the remove leg's PLAYER_CONTROLLED restore lives in
// removeAura. Documented delta: C++'s remove leg restores the native faction
// unconditionally even when a second faction aura remains — the live fold
// keeps the remaining aura's faction instead.
func (s *session) forcedFactionAura() (uint32, bool) {
	if s == nil {
		return 0, false
	}
	var faction uint32
	var slot uint8
	found := false
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		if !s.activeAuraHasEffect(aura, spellAuraModFaction) {
			continue
		}
		misc := int32(aura.MiscValue)
		if s.server != nil && s.server.Data != nil {
			if spell, ok, err := s.server.Data.Spell(aura.SpellID); err == nil && ok {
				for index, effect := range spell.Effects {
					if aura.EffectMask&(1<<uint(index)) != 0 && effect.Aura == spellAuraModFaction {
						misc = effect.MiscValue
						break
					}
				}
			}
		}
		if misc < 0 {
			continue
		}
		if !found || aura.Slot >= slot {
			faction, slot, found = uint32(misc), aura.Slot, true
		}
	}
	return faction, found
}

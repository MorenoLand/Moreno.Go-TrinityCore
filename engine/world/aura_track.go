package world

// playerTrackFieldMask mirrors AuraEffect::HandleAuraTrackCreatures and
// HandleAuraTrackResources (SpellAuraEffects.cpp:2448-2480): while a
// TRACK_CREATURES (44) or TRACK_RESOURCES (45) effect is active,
// PLAYER_TRACK_CREATURES (1022) / PLAYER_TRACK_RESOURCES (1023) carries
// bit 1 << (miscValue-1). The C++ remove legs clear the bit with no
// HasAuraType gate, so the live fold over the remaining auras is exactly
// the C++ apply/remove semantics. Both handlers are player-only in C++
// (TYPEID_PLAYER gate); Go's aura paths are player sessions already.
func (s *session) playerTrackFieldMask(auraType uint32) uint32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var mask uint32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		sp, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range sp.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 || effect.Aura != auraType {
				continue
			}
			if effect.MiscValue >= 1 && effect.MiscValue <= 32 {
				mask |= 1 << (uint32(effect.MiscValue) - 1)
			}
		}
	}
	return mask
}

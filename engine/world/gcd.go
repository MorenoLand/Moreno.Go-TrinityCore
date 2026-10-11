package world

import (
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// C++ owners: CanHaveGlobalCooldown (Spell.cpp:8215-8222),
// Spell::TriggerGlobalCooldown (Spell.cpp:8232-8261),
// Spell::CancelGlobalCooldown (Spell.cpp:8264-8277),
// SpellHistory::HasGlobalCooldown/AddGlobalCooldown/CancelGlobalCooldown
// (SpellHistory.cpp:585-600), GCDLimits (Spell.cpp:8209-8213),
// SpellDmgClass (SharedDefines.h:1574-1582).

const (
	// enum GCDLimits { MIN_GCD = 1000, MAX_GCD = 1500 } (Spell.cpp:8209-8213).
	gcdMinMs = int64(1000)
	gcdMaxMs = int64(1500)
	// Only this StartRecoveryCategory with this StartRecoveryTime gets
	// haste scaling (Spell.cpp:8249-8258).
	gcdHasteRecoveryCategory = uint32(133)
	gcdHasteRecoveryTime     = uint32(1500)
)

// triggerGlobalCooldown mirrors Spell::TriggerGlobalCooldown.
// C++ keys the cooldown by StartRecoveryCategory (AddGlobalCooldown), so the
// session stores per-category expiry times instead of a single GCD slot.
func (s *session) triggerGlobalCooldown(spell wotlk.Spell) {
	// CanHaveGlobalCooldown is vacuous in Go: only player sessions cast.
	// SPELLMOD_GLOBAL_COOLDOWN is not applied here (applySpellMod covers
	// the taking-window side per Player::ApplySpellMod, but wiring the
	// GCD arm needs the prepare-time taking context — the Backdraft PCT
	// ordering leg — which Go does not model yet; noted gap).
	if spell.StartRecoveryCategory == 0 {
		return
	}
	// Spell::TriggerGlobalCooldown (Spell.cpp:8238-8241): a player holding
	// CHEAT_COOLDOWN skips the global cooldown entirely, allowing rapid
	// casts. The C++ gate is TYPEID_PLAYER-only — pet and charmed-creature
	// casters still trigger GCD — so it lives here on the session path,
	// not in the motion twin (triggerPetGlobalCooldown).
	if s.player != nil && s.player.ActiveCheats&cheatCooldown != 0 {
		return
	}

	gcd := int64(spell.StartRecoveryTime)

	// Apply haste rating (Spell.cpp:8249-8258): only category 133 / 1500ms
	// spells, excluding melee and ranged damage classes and REQ_AMMO/ABILITY
	// spells. C++ truncates (int32(float(gcd) * UNIT_MOD_CAST_SPEED)) then
	// clamps to [MIN_GCD, MAX_GCD] via RoundToInterval (Util.h:84-87).
	if spell.StartRecoveryCategory == gcdHasteRecoveryCategory &&
		spell.StartRecoveryTime == gcdHasteRecoveryTime &&
		spell.DefenseType != 2 && spell.DefenseType != 3 && // SPELL_DAMAGE_CLASS_MELEE / RANGED
		spell.Attributes&spellAttr0ReqAmmo == 0 &&
		spell.Attributes&spellAttr0Ability == 0 {
		speed := 1.0 / (1.0 + s.getSpellHastePct()/100.0) // UNIT_MOD_CAST_SPEED
		gcd = int64(float64(gcd) * speed)
		if gcd < gcdMinMs {
			gcd = gcdMinMs
		}
		if gcd > gcdMaxMs {
			gcd = gcdMaxMs
		}
	}

	if gcd == 0 {
		return
	}
	s.castMu.Lock()
	if s.gcdCooldowns == nil {
		s.gcdCooldowns = make(map[uint32]int64)
	}
	s.gcdCooldowns[spell.StartRecoveryCategory] = time.Now().UnixMilli() + gcd
	s.castMu.Unlock()
}

// isGCDActive mirrors Spell::HasGlobalCooldown (SpellHistory::HasGlobalCooldown):
// the lookup is keyed by the spell's own StartRecoveryCategory.
func (s *session) isGCDActive(spell wotlk.Spell) bool {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	end, ok := s.gcdCooldowns[spell.StartRecoveryCategory]
	return ok && end > time.Now().UnixMilli()
}

// cancelGlobalCooldown mirrors Spell::CancelGlobalCooldown (Spell.cpp:8264-8277):
// clearing the category entry of the spell whose cast was interrupted, only
// when the interrupted spell actually triggered a GCD (StartRecoveryTime != 0).
func (s *session) cancelGlobalCooldown(spellID uint32) {
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || spell.StartRecoveryTime == 0 || spell.StartRecoveryCategory == 0 {
		return
	}
	s.castMu.Lock()
	delete(s.gcdCooldowns, spell.StartRecoveryCategory)
	s.castMu.Unlock()
}

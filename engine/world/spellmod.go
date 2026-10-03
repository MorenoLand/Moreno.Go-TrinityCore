package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// SpellModOp values (SpellDefines.h:80-110).
const (
	spellModDamage                = 0
	spellModDuration              = 1
	spellModThreat                = 2
	spellModEffect1               = 3
	spellModCharges               = 4
	spellModRange                 = 5
	spellModRadius                = 6
	spellModCriticalChance        = 7
	spellModAllEffects            = 8
	spellModNotLoseCastingTime    = 9
	spellModCastingTime           = 10
	spellModCooldown              = 11
	spellModEffect2               = 12
	spellModIgnoreArmor           = 13
	spellModCost                  = 14
	spellModCritDamageBonus       = 15
	spellModResistMissChance      = 16
	spellModJumpTargets           = 17
	spellModChanceOfSuccess       = 18
	spellModActivationTime        = 19
	spellModDamageMultiplier      = 20
	spellModGlobalCooldown        = 21
	spellModDot                   = 22
	spellModEffect3               = 23
	spellModBonusMultiplier       = 24
	spellModProcPerMinute         = 26 // spellmod 25 unused
	spellModValueMultiplier       = 27
	spellModResistDispelChance    = 28
	spellModCritDamageBonus2      = 29
	spellModSpellCostRefundOnFail = 30
	spellModOpCount               = 31 // MAX_SPELLMOD
)

// Aura types that create spell modifiers (SpellAuraDefines.h:187-188); the
// C++ SpellModType values are the aura type values (Player.h:95-96).
const (
	spellAuraAddFlatModifier uint32 = 107
	spellAuraAddPctModifier  uint32 = 108
)

// spellAttr0CuCanCrit (SpellInfo.h:185) gates SPELLMOD_CRITICAL_CHANCE
// application (Player::IsAffectedBySpellmod, Player.cpp:21299).
const spellAttr0CuCanCrit uint32 = 0x00000080

// spellModifier mirrors TrinityCore's SpellModifier (Player.h:143-154): one
// per aura effect of type SPELL_AURA_ADD_FLAT_MODIFIER / ADD_PCT_MODIFIER,
// owned by the aura and registered on the player while the aura is active.
// The per-op registry lives on the session (m_spellMods[MAX_SPELLMOD],
// Player.h) and is guarded by castMu.
type spellModifier struct {
	op          uint8
	modType     uint8 // 107 flat / 108 pct, == C++ SpellModType values
	effectIndex uint8
	spellID     uint32
	value       int32
	mask        [3]uint32
	usesCharges bool
	owner       *activeAura
}

type spellModFrame struct {
	opcode  uint16
	payload []byte
}

// spellModTakingContext mirrors one activation of Player::m_spellModTakingSpell
// (Player.cpp:21436-21445): the taking cast's registry of "used" auras
// (Spell::m_appliedMods, Spell.h:527), populated by Player::ApplyModToSpell
// (Player.cpp:21415-21426). Go has no cast Spell object, so the taking spell
// is a session-scoped stack of contexts: Spell::cast (Spell.cpp:3266-3280)
// pushes a fresh taking spell for a nested (triggered) cast and restores the
// outer one when it returns, which the stack reproduces — a nested cast
// registers its mods on its own context, never the outer one.
type spellModTakingContext struct {
	applied map[*activeAura]struct{}
}

// beginSpellModTaking mirrors the SetSpellModTakingSpell(spell, true) legs:
// Spell::_cast head (Spell.cpp:3323), Spell::handle_delayed head (3640), and
// the event-processor delayed branch (7616) which sets it around
// handle_immediate. Each activation gets a fresh registry; nested casts
// push their own (Spell::cast, Spell.cpp:3266-3280).
func (s *session) beginSpellModTaking() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	s.spellModTaking = append(s.spellModTaking, &spellModTakingContext{applied: make(map[*activeAura]struct{})})
}

// endSpellModTaking mirrors the SetSpellModTakingSpell(spell, false) legs:
// the _cast tail and failure exits (Spell.cpp:3418, 3519), the
// handle_delayed tail (3697), and the event-processor branch tail (7621).
// Pops the innermost context; a no-op with an empty stack (the C++
// mismatched-spell remove is a no-op too).
func (s *session) endSpellModTaking() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if len(s.spellModTaking) == 0 {
		return
	}
	s.spellModTaking = s.spellModTaking[:len(s.spellModTaking)-1]
}

// spellModTakingCurrent returns the innermost taking context, or nil when no
// cast holds the taking window — the Go model of the m_spellModTakingSpell
// redirect at the top of Player::ApplySpellMod (Player.cpp:21349-21350).
func (s *session) spellModTakingCurrent() *spellModTakingContext {
	if s == nil {
		return nil
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if len(s.spellModTaking) == 0 {
		return nil
	}
	return s.spellModTaking[len(s.spellModTaking)-1]
}

// spellModTakingApplied reports whether the taking cast already registered
// the aura — Player::HasSpellModApplied (Player.cpp:21429-21434) against the
// taking spell's m_appliedMods.
func (s *session) spellModTakingApplied(taking *spellModTakingContext, owner *activeAura) bool {
	if s == nil || taking == nil || owner == nil {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	_, ok := taking.applied[owner]
	return ok
}

// registerSpellModApplied mirrors Player::ApplyModToSpell
// (Player.cpp:21415-21426): the taking cast registers the mod's owner aura
// (charge-using auras only while they still hold charges — "don't do
// anything with no charges"); the proc system and IsAffectedBySpellmod read
// the registry back.
func (s *session) registerSpellModApplied(taking *spellModTakingContext, owner *activeAura, usesCharges bool) {
	if s == nil || taking == nil || owner == nil {
		return
	}
	if usesCharges && owner.RemainingCharges == 0 {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	taking.applied[owner] = struct{}{}
}

// spellModEffectValue resolves the modifier amount with the same fallback
// chain Go uses for aura effect amounts (totalAuraModifierByAffectMask).
func spellModEffectValue(aura *activeAura, index int, effect wotlk.SpellEffect) int32 {
	amount := aura.Amounts[index]
	if amount == 0 {
		amount = int32(aura.Amount)
	}
	if amount == 0 {
		amount = effect.BasePoints + 1
	}
	return amount
}

// spellModFrames builds the SMSG_SET_FLAT_SPELL_MODIFIER /
// SMSG_SET_PCT_SPELL_MODIFIER frames for one modifier, mirroring the packet
// loop of Player::AddSpellMod (Player.cpp:21384-21407): one frame per set bit
// of the modifier's 96-bit mask, carrying the summed value of the same-type
// modifiers already registered for the op on that bit, plus (apply) or minus
// (!apply) this modifier's value. Callers pass the registry state the C++
// packet loop observes: before insert on apply, before erase on remove.
func spellModFrames(existing []*spellModifier, mod *spellModifier, apply bool) []spellModFrame {
	opcode := uint16(protocol.OpcodeSMSG_SET_FLAT_SPELL_MODIFIER)
	if mod.modType == uint8(spellAuraAddPctModifier) {
		opcode = uint16(protocol.OpcodeSMSG_SET_PCT_SPELL_MODIFIER)
	}
	var frames []spellModFrame
	for i := 0; i < 3; i++ {
		for eff := 0; eff < 32; eff++ {
			bit := uint32(1) << uint(eff)
			if mod.mask[i]&bit == 0 {
				continue
			}
			var val int32
			for _, m := range existing {
				if m == nil || m.modType != mod.modType || m.mask[i]&bit == 0 {
					continue
				}
				val += m.value
			}
			if apply {
				val += mod.value
			} else {
				val -= mod.value
			}
			buf := protocol.NewBuffer(6)
			buf.WriteU8(uint8(eff + 32*i))
			buf.WriteU8(mod.op)
			buf.WriteI32(val)
			frames = append(frames, spellModFrame{opcode: opcode, payload: buf.Bytes()})
		}
	}
	return frames
}

// addSpellMods registers the spell modifiers of the aura's effects of type
// SPELL_AURA_ADD_FLAT_MODIFIER / SPELL_AURA_ADD_PCT_MODIFIER, mirroring
// AuraEffect::CalculateSpellMod (SpellAuraEffects.cpp:634-653) feeding
// AuraEffect::ApplySpellMod(apply=true) (SpellAuraEffects.cpp:755-760).
// Frames are built and registered sequentially per effect so each modifier's
// packet observes the previously registered ones, as in C++.
func (s *session) addSpellMods(aura *activeAura) {
	if s == nil || aura == nil || s.server == nil || s.server.Data == nil {
		return
	}
	sp, found, err := s.server.Data.Spell(aura.SpellID)
	if err != nil || !found {
		return
	}
	s.castMu.Lock()
	var frames []spellModFrame
	for index, effect := range sp.Effects {
		if index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
			continue
		}
		if effect.Aura != spellAuraAddFlatModifier && effect.Aura != spellAuraAddPctModifier {
			continue
		}
		if effect.MiscValue < 0 || effect.MiscValue >= spellModOpCount {
			continue
		}
		mod := &spellModifier{
			op:          uint8(effect.MiscValue),
			modType:     uint8(effect.Aura),
			effectIndex: uint8(index),
			spellID:     aura.SpellID,
			value:       spellModEffectValue(aura, index, effect),
			mask:        effect.SpellClassMask,
			usesCharges: sp.ProcCharges > 0,
			owner:       aura,
		}
		frames = append(frames, spellModFrames(s.spellMods[mod.op], mod, true)...)
		s.spellMods[mod.op] = append(s.spellMods[mod.op], mod)
	}
	s.castMu.Unlock()
	for _, f := range frames {
		_ = s.write(f.opcode, f.payload, true)
	}
}

// dropSpellMods unregisters every modifier owned by the aura of spellID,
// mirroring AuraEffect::ApplySpellMod(apply=false) (SpellAuraEffects.cpp:755-760).
// Each modifier's frames are built against the registry state before that
// modifier's own removal, matching C++'s send-before-erase order per effect
// (Player.cpp:21408-21412).
func (s *session) dropSpellMods(spellID uint32) {
	if s == nil {
		return
	}
	s.castMu.Lock()
	var frames []spellModFrame
	for op := 0; op < spellModOpCount; op++ {
		var remove []*spellModifier
		for _, m := range s.spellMods[op] {
			if m != nil && m.owner != nil && m.owner.SpellID == spellID {
				remove = append(remove, m)
			}
		}
		for _, m := range remove {
			frames = append(frames, spellModFrames(s.spellMods[op], m, false)...)
			kept := s.spellMods[op][:0]
			for _, k := range s.spellMods[op] {
				if k != m {
					kept = append(kept, k)
				}
			}
			for i := len(kept); i < len(s.spellMods[op]); i++ {
				s.spellMods[op][i] = nil
			}
			s.spellMods[op] = kept
		}
	}
	s.castMu.Unlock()
	for _, f := range frames {
		_ = s.write(f.opcode, f.payload, true)
	}
}

// refreshSpellModValues updates registered modifier amounts after an in-place
// aura refresh, mirroring AuraEffect::ChangeAmount -> CalculateSpellMod
// (SpellAuraEffects.cpp:657-683), which refreshes m_spellmod->value without
// re-sending the client packet.
func (s *session) refreshSpellModValues(aura *activeAura) {
	if s == nil || aura == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	for op := 0; op < spellModOpCount; op++ {
		for _, m := range s.spellMods[op] {
			if m == nil || m.owner != aura || int(m.effectIndex) >= len(aura.Amounts) {
				continue
			}
			m.value = aura.Amounts[m.effectIndex]
		}
	}
}

// spellModAffectsSpell mirrors Player::IsAffectedBySpellmod
// (Player.cpp:21278-21306). The attribute-gated terms are C++-exact; the
// charge leg (Player.cpp:21294 — a charge-using aura at 0 charges applies
// only if the taking cast already registered it) is folded by the caller,
// which owns the taking context.
func (s *session) spellModAffectsSpell(modSpellID uint32, mask [3]uint32, op uint8, spell wotlk.Spell) bool {
	if op == spellModDuration {
		if dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, 1); err == nil && ok && dur == -1 {
			return false
		}
	}
	if op == spellModCriticalChance && spell.Attributes&spellAttr0CuCanCrit == 0 {
		return false
	}
	affectSpell, found, err := s.server.Data.Spell(modSpellID)
	if err != nil || !found {
		return false
	}
	return spellAffectedBySpellFamilyMask(affectSpell.SpellFamilyName, mask, spell)
}

// applySpellMod mirrors the int32 instantiation of Player::ApplySpellMod
// (Player.cpp:21309-21370): it folds the registered flat and pct modifiers
// affecting spell into basevalue as (basevalue + totalFlat) * totalMul,
// truncated back to int32. Mods from charge-using auras take the
// charged-mod slot (highest Priority wins in C++; wotlk.Spell has no
// Priority field, so the first one wins — noted). The taking-spell redirect
// (Player.cpp:21349-21350) is modeled by spellModTakingCurrent: when a cast
// holds the taking window, applied mods register on its context
// (ApplyModToSpell, Player.cpp:21415-21426). The nil-spell semantics are
// C++-exact outside the window; inside it, the HasSpellModApplied-gated PCT
// terms for SPELLMOD_CRITICAL_CHANCE / SPELLMOD_GLOBAL_COOLDOWN engage
// (Surge of Light / Backdraft per-mod application ordering).
func (s *session) applySpellMod(spell wotlk.Spell, op uint8, basevalue int32) int32 {
	if s == nil || s.server == nil || s.server.Data == nil || op >= spellModOpCount {
		return basevalue
	}
	// SpellInfo::IsAffectedBySpellMods (SpellInfo.cpp:1319).
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return basevalue
	}
	totalFlat, totalMul := s.spellModTotals(spell, op, basevalue >= 10000)
	return int32(float64(basevalue+totalFlat) * totalMul)
}

// applySpellModFloat mirrors the float instantiation of
// Player::ApplySpellMod (Player.cpp:21309-21370): the base value stays in
// the float domain through the whole computation, matching basevalue =
// T(float(basevalue + totalflat) * totalmul) with T=float — fractional
// chances (proc chance rolls) are never truncated through int32.
func (s *session) applySpellModFloat(spell wotlk.Spell, op uint8, basevalue float64) float64 {
	if s == nil || s.server == nil || s.server.Data == nil || op >= spellModOpCount {
		return basevalue
	}
	// SpellInfo::IsAffectedBySpellMods (SpellInfo.cpp:1319).
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return basevalue
	}
	totalFlat, totalMul := s.spellModTotals(spell, op, basevalue >= 10000)
	return (basevalue + float64(totalFlat)) * totalMul
}

// spellModTotals runs the modifier fold of Player::ApplySpellMod
// (Player.cpp:21309-21370) shared by both instantiations, returning the
// folded flat total and pct multiplier. instantBaseOK is the
// basevalue >= T(10000) instant-cast guard, evaluated in the caller's
// domain as in the C++ template.
func (s *session) spellModTotals(spell wotlk.Spell, op uint8, instantBaseOK bool) (int32, float64) {
	type candidate struct {
		modType     uint8
		value       int32
		mask        [3]uint32
		spellID     uint32
		usesCharges bool
		owner       *activeAura
	}
	s.castMu.Lock()
	cands := make([]candidate, 0, len(s.spellMods[op]))
	for _, m := range s.spellMods[op] {
		if m == nil || m.owner == nil || m.owner.Stopped {
			continue
		}
		cands = append(cands, candidate{m.modType, m.value, m.mask, m.spellID, m.usesCharges, m.owner})
	}
	s.castMu.Unlock()

	// The taking window is the Go model of the spell!=nullptr context in
	// Player::ApplySpellMod (Player.cpp:21349-21350): outside a cast the
	// fold runs with nil-spell semantics.
	taking := s.spellModTakingCurrent()

	var totalMul float64 = 1.0
	var totalFlat int32
	var charged *candidate
	apply := func(c *candidate) {
		if c.modType == uint8(spellAuraAddFlatModifier) {
			totalFlat += c.value
		} else {
			// PCT branch (Player.cpp:21324-21344).
			if op == spellModCastingTime && c.value <= -100 && instantBaseOK {
				return
			}
			// Player::ApplySpellMod PCT special cases (Player.cpp:21328-21337):
			// HasSpellModApplied (Player.cpp:21429-21434) reads the taking
			// cast's applied-mods registry — the live taking context here.
			// A PCT critical-chance mod applies only when the same mod
			// already registered on this cast (Surge of Light: an earlier
			// leg of the cast, not a mid-cast proc); a PCT GCD mod likewise
			// (Backdraft: only when its cast-time reduction leg applied
			// first). With no taking window the nil-spell semantics give
			// false, so the mods are skipped exactly as in C++ prepare-time.
			if !s.spellModTakingApplied(taking, c.owner) {
				if op == spellModCriticalChance || op == spellModGlobalCooldown {
					return
				}
			}
			totalMul += float64(c.value) / 100.0
		}
		// Player::ApplyModToSpell (Player.cpp:21415-21426) runs at the end
		// of the per-mod fold: the taking cast registers the mod's owner
		// aura so later legs of the same cast (and the proc system) see it
		// as used. Charge-using auras register only while they still hold
		// charges.
		if taking != nil {
			s.registerSpellModApplied(taking, c.owner, c.usesCharges)
		}
	}
	for i := range cands {
		c := &cands[i]
		// Player::IsAffectedBySpellmod charge leg (Player.cpp:21294): a mod
		// whose aura uses charges applies to a taking cast only while the
		// aura still holds charges, unless this cast already registered it
		// above — the first mod leg of the cast keeps the bonus even if
		// the charge drops mid-cast.
		if taking != nil && c.usesCharges && c.owner != nil && c.owner.RemainingCharges == 0 &&
			!s.spellModTakingApplied(taking, c.owner) {
			continue
		}
		if !s.spellModAffectsSpell(c.spellID, c.mask, op, spell) {
			continue
		}
		if c.usesCharges {
			if charged == nil {
				charged = c
			}
			continue
		}
		apply(c)
	}
	if charged != nil {
		apply(charged)
	}
	return totalFlat, totalMul
}

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
// (Player.cpp:21278-21306). The charge-drop / m_appliedMods terms need a cast
// Spell object, which Go has no model for (standing gap); the attribute-gated
// terms are C++-exact.
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

// applySpellMod mirrors Player::ApplySpellMod (Player.cpp:21309-21370): it
// folds the registered flat and pct modifiers affecting spell into basevalue
// as (basevalue + totalFlat) * totalMul. Mods from charge-using auras take
// the charged-mod slot (highest Priority wins in C++; wotlk.Spell has no
// Priority field, so the first one wins — noted). Go has no cast Spell
// object, so the m_spellModTakingSpell redirect and the ApplyModToSpell
// charge-drop registration have no model (standing gaps); the nil-spell
// semantics are C++-exact: the HasSpellModApplied-gated PCT terms for
// SPELLMOD_CRITICAL_CHANCE / SPELLMOD_GLOBAL_COOLDOWN never apply.
func (s *session) applySpellMod(spell wotlk.Spell, op uint8, basevalue int32) int32 {
	if s == nil || s.server == nil || s.server.Data == nil || op >= spellModOpCount {
		return basevalue
	}
	// SpellInfo::IsAffectedBySpellMods (SpellInfo.cpp:1319).
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return basevalue
	}
	type candidate struct {
		modType     uint8
		value       int32
		mask        [3]uint32
		spellID     uint32
		usesCharges bool
	}
	s.castMu.Lock()
	cands := make([]candidate, 0, len(s.spellMods[op]))
	for _, m := range s.spellMods[op] {
		if m == nil || m.owner == nil || m.owner.Stopped {
			continue
		}
		cands = append(cands, candidate{m.modType, m.value, m.mask, m.spellID, m.usesCharges})
	}
	s.castMu.Unlock()

	var totalMul float64 = 1.0
	var totalFlat int32
	var charged *candidate
	apply := func(c *candidate) {
		if c.modType == uint8(spellAuraAddFlatModifier) {
			totalFlat += c.value
			return
		}
		// PCT branch (Player.cpp:21324-21344).
		if op == spellModCastingTime && c.value <= -100 && basevalue >= 10000 {
			return
		}
		if op == spellModCriticalChance || op == spellModGlobalCooldown {
			return
		}
		totalMul += float64(c.value) / 100.0
	}
	for i := range cands {
		c := &cands[i]
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
	return int32(float64(basevalue+totalFlat) * totalMul)
}

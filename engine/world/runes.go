package world

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Death Knight rune constants.
// C++ owners: Player.h:263-288 (RuneType, RuneInfo, MAX_RUNES),
// Player.h:2117-2127 (GetCurrentRune/GetRuneCooldown/SetRuneCooldown),
// SharedDefines.h:300-301 (POWER_RUNE/POWER_RUNIC_POWER).
const (
	runeBlood  = 0
	runeUnholy = 1
	runeFrost  = 2
	runeDeath  = 3

	maxRunes           = 6
	runeBaseCooldownMs = 10000 // RUNE_BASE_COOLDOWN (Player.h:263)
	runeMissCooldownMs = 1500  // RUNE_MISS_COOLDOWN (Player.h:264), applied when the spell misses

	powerRune       = 5 // POWER_RUNE (SharedDefines.h:300)
	powerRunicPower = 6 // POWER_RUNIC_POWER (SharedDefines.h:301)

	classDeathKnight = 6

	spellEmpowerRuneWeapon uint32 = 47568 // Empower Rune Weapon (SpellEffects.cpp:5350)
)

// dkRuneState mirrors Player::m_runes (Player.h:286): per-slot base and
// current rune types plus the cooldown end timestamp (Unix ms). Rune
// cooldowns are session-transient like in C++; they reset on login.
// graceStartMs is the m_runeGraceCooldown window (Player.cpp:1334-1344,
// 24855-24869): the Unix-ms moment the slot's current ready stretch began;
// -1 means no window (C++ 0xFFFFFFFF: init and exit-combat states). While a
// rune sits ready, grace banks up to 2500ms, and an in-combat spend shaves
// the banked grace off the new cooldown.
type dkRuneState struct {
	base          [maxRunes]uint8
	current       [maxRunes]uint8
	cooldownEndMs [maxRunes]int64
	graceStartMs  [maxRunes]int64
	lastUsed      uint8
}

// dkLoginCurrentRuneTypes resolves the Death Knight's CURRENT rune layout at login:
// {blood, blood, unholy, unholy, frost, frost} adjusted by learned
// SPELL_AURA_CONVERT_RUNE passives (Blood of the North / Reaping /
// Death Rune Mastery). C++ keeps base runes fixed (SetBaseRune runs only in
// Player::InitRunes, Player.cpp:25008) and converts the CURRENT runes when
// the passive auras apply (HandleAuraConvertRune, SpellAuraEffects.cpp:4847),
// so this feeds current[] only; base[] stays the fixed layout and a spent
// death rune reverts to its true base (RestoreBaseRune, Player.cpp:24904).
// It runs on every player update build for the regen values and at init.
func (s *session) dkLoginCurrentRuneTypes() [maxRunes]uint8 {
	runeTypes := [maxRunes]uint8{runeBlood, runeBlood, runeUnholy, runeUnholy, runeFrost, runeFrost}
	spells := append([]learnedSpell(nil), s.player.Spells...)
	sort.Slice(spells, func(i, j int) bool { return spells[i].ID < spells[j].ID })
	passiveSpells := make([]wotlk.Spell, 0)
	if s.server != nil && s.server.Data != nil {
		for _, learned := range spells {
			if !learned.Active || learned.Disabled {
				continue
			}
			spell, found, err := s.server.Data.Spell(learned.ID)
			if err != nil || !found || spell.Attributes&spellAttributePassive == 0 {
				continue
			}
			passiveSpells = append(passiveSpells, spell)
		}
	}
	return ResolveDeathKnightRuneTypes(runeTypes, passiveSpells)
}

// initRunes initializes the rune state for a Death Knight at login.
// Player::InitRunes (Player.cpp:24995-25014): base runes are the fixed slot
// layout and never change afterwards (SetBaseRune has no other caller);
// SPELL_AURA_CONVERT_RUNE passives convert the CURRENT runes when their
// auras apply (HandleAuraConvertRune, SpellAuraEffects.cpp:4847-4875), which
// at login — all runes ready — is exactly the dkLoginCurrentRuneTypes fold. A spent
// death rune therefore reverts to its true base on a hit (RestoreBaseRune,
// Player.cpp:24904-24922), instead of staying death forever.
func (s *session) initRunes() {
	if s.player == nil || s.player.Class != classDeathKnight {
		return
	}
	base := [maxRunes]uint8{runeBlood, runeBlood, runeUnholy, runeUnholy, runeFrost, runeFrost}
	current := s.dkLoginCurrentRuneTypes()
	rs := &dkRuneState{base: base, current: current}
	for i := range rs.graceStartMs {
		rs.graceStartMs[i] = -1
	}
	s.runes = rs
}

// runeGraceMs mirrors the m_runeGraceCooldown accumulation
// (Player.cpp:1334-1344): grace banks while the rune sits ready, capped at
// 2500ms. -1 is the C++ 0xFFFFFFFF no-window state (init / exit combat).
func (rs *dkRuneState) runeGraceMs(i int, nowMs int64) int64 {
	if rs == nil || i < 0 || i >= maxRunes || rs.graceStartMs[i] < 0 {
		return 0
	}
	grace := nowMs - rs.graceStartMs[i]
	if grace < 0 {
		return 0
	}
	if grace > 2500 {
		return 2500
	}
	return grace
}

// runeCooldownRemainingMs returns the remaining cooldown of a rune slot in
// milliseconds, mirroring Player::GetRuneCooldown (Player.h:2120).
func (rs *dkRuneState) runeCooldownRemainingMs(index int, nowMs int64) int64 {
	remaining := rs.cooldownEndMs[index] - nowMs
	if remaining < 0 {
		return 0
	}
	return remaining
}

// runeBaseCooldownMsForSlot mirrors Player::GetRuneBaseCooldown
// (Player.cpp:24840): RUNE_BASE_COOLDOWN reduced by live
// SPELL_AURA_MOD_POWER_REGEN_PERCENT (110) effects whose MiscValue is
// POWER_RUNE and whose MiscValueB names the slot's base rune type, in
// C++ uint32 arithmetic (cooldown * (100 - amount) / 100 per aura).
func (s *session) runeBaseCooldownMsForSlot(index int) int64 {
	cooldown := int64(runeBaseCooldownMs)
	if s == nil || s.player == nil || s.runes == nil || index < 0 || index >= maxRunes ||
		s.server == nil || s.server.Data == nil {
		return cooldown
	}
	base := int32(s.runes.base[index])
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for i, effect := range spell.Effects {
			if i >= len(aura.Amounts) || aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModPowerRegenPct || effect.MiscValue != powerRune || effect.MiscValueB != base {
				continue
			}
			cooldown = cooldown * (100 - int64(aura.Amounts[i])) / 100
		}
	}
	return cooldown
}

// updatePlayerRuneRegenValues mirrors Player::UpdateRuneRegen
// (StatSystem.cpp:937-956): PLAYER_RUNE_REGEN_1 + rune holds 1000 / cooldown,
// where cooldown is the base cooldown of the first slot whose base rune is
// that type. It runs on every player update build, so the C++ trigger
// (HandleModPowerRegen, SpellAuraEffects.cpp:3625-3628 — MOD_POWER_REGEN
// auras with misc POWER_RUNE applying/removing) needs no separate hook.
func (s *session) updatePlayerRuneRegenValues(values []uint32) {
	if s == nil || s.player == nil || s.player.Class != classDeathKnight || s.runes == nil {
		return
	}
	for runeType := 0; runeType < runeDeath; runeType++ {
		cooldown := int64(0)
		for i := 0; i < maxRunes; i++ {
			if int(s.runes.base[i]) == runeType {
				cooldown = s.runeBaseCooldownMsForSlot(i)
				break
			}
		}
		if cooldown <= 0 {
			continue
		}
		values[playerRuneRegenStart+runeType] = math.Float32bits(float32(1000.0 / float64(cooldown)))
	}
}

// checkRuneCost mirrors Spell::CheckRuneCost (Spell.cpp:4918-4954): it
// verifies the Death Knight has enough ready runes for the spell's
// SpellRuneCost entry. Returns false when the cast must fail with
// SPELL_FAILED_NO_POWER (85). Death runes act as wildcards; rune costs are
// satisfied from matching current-type runes first, exactly like C++.
func (s *session) checkRuneCost(spell wotlk.Spell, nowMs int64) bool {
	if spell.PowerType != powerRune || spell.RuneCostID == 0 {
		return true
	}
	if s.player == nil || s.player.Class != classDeathKnight {
		return true
	}
	if s.server == nil || s.server.Data == nil {
		return true
	}
	cost, found, err := s.server.Data.SpellRuneCost(spell.RuneCostID)
	if err != nil || !found {
		return true
	}
	// SpellRuneCost.dbc columns 1-3 are [0] Blood [1] Unholy [2] Frost
	// (DBCStructure.h:1591), matching the RuneType enum order 0,1,2.
	if cost.RuneCost[0] == 0 && cost.RuneCost[1] == 0 && cost.RuneCost[2] == 0 {
		return true
	}
	var runeCost [4]int32
	for i := 0; i < runeDeath; i++ {
		// Spell::CheckRuneCost (Spell.cpp:4928-4932): SPELLMOD_COST folds
		// per rune type through the mod owner (the player caster here).
		runeCost[i] = s.applySpellMod(spell, spellModCost, int32(cost.RuneCost[i]))
	}
	runeCost[runeDeath] = maxRunes // calculated later
	rs := s.runes
	if rs == nil {
		// No state yet (login still in flight): C++ runes start ready.
		return true
	}
	for i := 0; i < maxRunes; i++ {
		cur := rs.current[i]
		if rs.runeCooldownRemainingMs(i, nowMs) == 0 && runeCost[cur] > 0 {
			runeCost[cur]--
		}
	}
	for i := 0; i < runeDeath; i++ {
		if runeCost[i] > 0 {
			runeCost[runeDeath] += runeCost[i]
		}
	}
	return runeCost[runeDeath] <= maxRunes
}

// takeRunePower mirrors Spell::TakeRunePower (Spell.cpp:4956-5039): it
// spends the runes for a cast Death Knight spell, in C++ order:
// matching current-type runes first, then death runes whose base type
// matches a leftover cost, then any remaining death rune. Death runes
// revert to their base type on a hit (RestoreBaseRune) and keep the death
// type on a miss. On a hit the spell's RunicPower gain is granted.
// It returns the rune-ready mask from before the spend, mirroring the
// m_runesState = player->GetRunesState() capture at Spell.cpp:4966 (0 when
// the spend is skipped, since C++ captures the state only past the same
// early returns); Spell::SendSpellGo writes it as RuneData.Start.
func (s *session) takeRunePower(ctx context.Context, spell wotlk.Spell, didHit bool, nowMs int64) uint8 {
	if s.player == nil || s.player.Class != classDeathKnight {
		return 0
	}
	if s.server == nil || s.server.Data == nil {
		return 0
	}
	runeCostData, found, err := s.server.Data.SpellRuneCost(spell.RuneCostID)
	if err != nil || !found {
		return 0
	}
	noRuneCost := runeCostData.RuneCost[0] == 0 && runeCostData.RuneCost[1] == 0 && runeCostData.RuneCost[2] == 0
	if noRuneCost && runeCostData.RunicPower == 0 {
		return 0
	}
	if s.runes == nil {
		s.initRunes()
	}
	rs := s.runes
	if rs == nil {
		return 0
	}
	maskBefore := s.runeReadyMask(nowMs)
	setRuneCooldown := func(i int) {
		cd := s.runeBaseCooldownMsForSlot(i)
		if !didHit {
			cd = runeMissCooldownMs
		}
		// Player::SetRuneCooldown grace arm (Player.cpp:24855-24869): a rune
		// spent in combat shaves the banked grace — up to 2.5s accumulated
		// while the rune sat ready — off the new cooldown, and the grace
		// window restarts when the rune becomes ready again. Out-of-combat
		// spends leave the window untouched (C++ arms it only inside
		// casted && IsInCombat); the m_lastRuneGraceTimers write has no
		// reader in the C++ tree, so Go keeps no twin.
		if s.player.UnitFlags&unitFlagInCombat != 0 {
			if grace := rs.runeGraceMs(i, nowMs); grace > 0 && cd > 0 {
				if grace > cd {
					grace = cd
				}
				cd -= grace
			}
			rs.graceStartMs[i] = nowMs + cd
		}
		rs.cooldownEndMs[i] = nowMs + cd
	}
	var runeCost [4]int32
	for i := 0; i < runeDeath; i++ {
		// Spell::TakeRunePower (Spell.cpp:4971-4974): same SPELLMOD_COST
		// fold as the check arm — the spend mirrors the check's modded cost.
		runeCost[i] = s.applySpellMod(spell, spellModCost, int32(runeCostData.RuneCost[i]))
	}
	runeCost[runeDeath] = 0 // calculated later
	for i := 0; i < maxRunes; i++ {
		cur := rs.current[i]
		if rs.runeCooldownRemainingMs(i, nowMs) == 0 && runeCost[cur] > 0 {
			setRuneCooldown(i)
			rs.lastUsed = cur
			runeCost[cur]--
		}
	}
	// Find a death rune where the base rune matches the one we need.
	runeCost[runeDeath] = runeCost[runeBlood] + runeCost[runeUnholy] + runeCost[runeFrost]
	if runeCost[runeDeath] > 0 {
		for i := 0; i < maxRunes; i++ {
			cur := rs.current[i]
			base := rs.base[i]
			if rs.runeCooldownRemainingMs(i, nowMs) == 0 && cur == runeDeath && runeCost[base] > 0 {
				setRuneCooldown(i)
				rs.lastUsed = cur
				runeCost[base]--
				runeCost[cur]--
				if didHit {
					rs.current[i] = base // RestoreBaseRune; death type kept on a miss
				}
				if runeCost[runeDeath] == 0 {
					break
				}
			}
		}
	}
	// Grab any death rune for whatever cost is left.
	if runeCost[runeDeath] > 0 {
		for i := 0; i < maxRunes; i++ {
			cur := rs.current[i]
			if rs.runeCooldownRemainingMs(i, nowMs) == 0 && cur == runeDeath {
				setRuneCooldown(i)
				rs.lastUsed = cur
				runeCost[cur]--
				if didHit {
					rs.current[i] = rs.base[i] // RestoreBaseRune
				}
				if runeCost[runeDeath] == 0 {
					break
				}
			}
		}
	}
	// Runic power is only gained on a hit; RATE_POWER_RUNICPOWER_INCOME
	// defaults to 1.0 in C++ and Go has no rate config (noted gap).
	if didHit && runeCostData.RunicPower > 0 {
		s.adjustSpellPower(ctx, s.playerGUID, powerRunicPower, int64(runeCostData.RunicPower))
	}
	return maskBefore
}

// runeReadyMask mirrors Player::GetRunesState (Player.h:2117): bit i is set
// when rune slot i has no cooldown remaining.
func (s *session) runeReadyMask(nowMs int64) uint8 {
	if s == nil || s.runes == nil {
		return 0
	}
	var mask uint8
	for i := 0; i < maxRunes; i++ {
		if s.runes.runeCooldownRemainingMs(i, nowMs) == 0 {
			mask |= 1 << uint(i)
		}
	}
	return mask
}

// sendRuneCooldownUpdate mirrors the per-rune cooldown byte of
// Player::ResyncRunes (Player.cpp:24962): 255 means ready, 0 means a full
// base-cooldown remaining.
func (s *session) sendRuneCooldownUpdate() {
	if s.player == nil || s.player.Class != classDeathKnight || s.runes == nil {
		return
	}
	nowMs := time.Now().UnixMilli()
	buf := protocol.NewBuffer(16)
	buf.WriteU32(maxRunes)
	for i := 0; i < maxRunes; i++ {
		buf.WriteU8(s.runes.current[i])
		remaining := s.runes.runeCooldownRemainingMs(i, nowMs)
		buf.WriteU8(uint8(255) - uint8(remaining*255/runeBaseCooldownMs))
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_RESYNC_RUNES), buf.Bytes(), true)
}

// handleEffectActivateRune mirrors Spell::EffectActivateRune
// (SpellEffects.cpp:5318-5360): on a Death Knight caster, runes of the
// effect's MiscValue type with a cooldown remaining have their cooldowns
// zeroed (damage of them, 1 when damage is 0); then the Empower Rune
// Weapon sweep zeros the cooldowns on every frost-base rune, running once
// for effIndex 0. Player::SetRuneCooldown always calls ResyncRunes
// (Player.cpp:24855-24881), so the rune bytes are pushed after any change.
// C++ runs this at SPELL_EFFECT_HANDLE_LAUNCH; Go has no launch-phase
// dispatch, so the caster arm runs once per cast.
func (s *session) handleEffectActivateRune(spell wotlk.Spell, eff wotlk.SpellEffect, effectIndex int) {
	if s == nil || s.player == nil || s.player.Class != classDeathKnight || s.runes == nil {
		return // SpellEffects.cpp:5323-5328: TYPEID_PLAYER + CLASS_DEATH_KNIGHT gates
	}
	nowMs := time.Now().UnixMilli()
	rs := s.runes
	count := eff.CalcValueForLevel(spell, uint32(s.player.Level))
	if count == 0 {
		count = 1
	}
	changed := false
	runeType := uint8(eff.MiscValue)
	for j := 0; j < maxRunes && count > 0; j++ {
		// SpellEffects.cpp:5340-5345: rune on cooldown and of the
		// MiscValue current type -> SetRuneCooldown(j, 0).
		if rs.runeCooldownRemainingMs(j, nowMs) > 0 && rs.current[j] == runeType {
			rs.cooldownEndMs[j] = 0
			// The rune became ready now: a grace window banked from the
			// spend-time ready estimate restarts here — C++'s timer resumes
			// accumulating once the rune is actually ready.
			if rs.graceStartMs[j] > nowMs {
				rs.graceStartMs[j] = nowMs
			}
			count--
			changed = true
		}
	}
	// SpellEffects.cpp:5348-5359: Empower Rune Weapon frost-base sweep,
	// once per cast ("Need to do this just once").
	if spell.ID == spellEmpowerRuneWeapon && effectIndex == 0 {
		for i := 0; i < maxRunes; i++ {
			if rs.runeCooldownRemainingMs(i, nowMs) > 0 && rs.base[i] == runeFrost {
				rs.cooldownEndMs[i] = 0
				if rs.graceStartMs[i] > nowMs {
					rs.graceStartMs[i] = nowMs
				}
				changed = true
			}
		}
	}
	if changed {
		s.sendRuneCooldownUpdate()
	}
}

// applyConvertRuneAura mirrors AuraEffect::HandleAuraConvertRune's apply leg
// (SpellAuraEffects.cpp:4847-4875): up to amount runes whose CURRENT type is
// the effect's MiscValue and which are not on cooldown convert to MiscValueB.
// The DK-class gate mirrors the ToPlayer/CLASS_DEATH_KNIGHT checks. C++
// notifies each conversion with SMSG_CONVERT_RUNE, which Go has no opcode
// for — the ResyncRunes push below carries the same current-type state.
func (s *session) applyConvertRuneAura(eff wotlk.SpellEffect, amount uint32) {
	if s == nil || s.player == nil || s.player.Class != classDeathKnight || s.runes == nil {
		return
	}
	nowMs := time.Now().UnixMilli()
	rs := s.runes
	remaining := amount
	from, to := uint8(eff.MiscValue), uint8(eff.MiscValueB)
	changed := false
	for i := 0; i < maxRunes && remaining > 0; i++ {
		if rs.current[i] != from || rs.runeCooldownRemainingMs(i, nowMs) > 0 {
			continue
		}
		rs.current[i] = to
		remaining--
		changed = true
	}
	if changed {
		s.sendRuneCooldownUpdate()
	}
}

// removeConvertRuneAura mirrors the remove leg (SpellAuraEffects.cpp:4876-4877)
// through Player::RemoveRunesByAuraEffect (Player.cpp:24893-24901): slots the
// aura converted revert to their base type. C++ tracks per-slot convert auras
// and reverts only when none remain; Go keeps no such set, so a slot reverts
// only when no other still-present 249 source converts the same source type
// to the same target type.
func (s *session) removeConvertRuneAura(spellID uint32) {
	if s == nil || s.player == nil || s.player.Class != classDeathKnight || s.runes == nil || s.server == nil || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	rs := s.runes
	changed := false
	for _, eff := range spell.Effects {
		if eff.Aura != spellAuraConvertRune {
			continue
		}
		from, to := uint8(eff.MiscValue), uint8(eff.MiscValueB)
		for i := 0; i < maxRunes; i++ {
			if rs.current[i] != to || rs.base[i] != from {
				continue
			}
			if s.hasOtherConvertRuneSource(spellID, from, to) {
				continue
			}
			rs.current[i] = rs.base[i]
			changed = true
		}
	}
	if changed {
		s.sendRuneCooldownUpdate()
	}
}

// hasOtherConvertRuneSource reports whether a 249 conversion from→to is still
// provided by something other than the aura being removed: another active 249
// aura, or a still-learned convert-rune passive (C++ keeps such passives in
// the slot's ConvertAuras set; Go baked them into current[] at login, so the
// learned-spell scan is the twin of that set).
func (s *session) hasOtherConvertRuneSource(excludeSpellID uint32, from, to uint8) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped || aura.SpellID == excludeSpellID {
			continue
		}
		spell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for _, eff := range spell.Effects {
			if eff.Aura == spellAuraConvertRune && uint8(eff.MiscValue) == from && uint8(eff.MiscValueB) == to {
				return true
			}
		}
	}
	if s.player != nil {
		for _, learned := range s.player.Spells {
			if !learned.Active || learned.Disabled || learned.ID == excludeSpellID {
				continue
			}
			spell, found, err := s.server.Data.Spell(learned.ID)
			if err != nil || !found || spell.Attributes&spellAttributePassive == 0 {
				continue
			}
			for _, eff := range spell.Effects {
				if eff.Aura == spellAuraConvertRune && uint8(eff.MiscValue) == from && uint8(eff.MiscValueB) == to {
					return true
				}
			}
		}
	}
	return false
}

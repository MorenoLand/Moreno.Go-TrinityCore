package world

import (
	"context"
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
)

// dkRuneState mirrors Player::m_runes (Player.h:286): per-slot base and
// current rune types plus the cooldown end timestamp (Unix ms). Rune
// cooldowns are session-transient like in C++; they reset on login.
type dkRuneState struct {
	base          [maxRunes]uint8
	current       [maxRunes]uint8
	cooldownEndMs [maxRunes]int64
	lastUsed      uint8
}

// dkBaseRuneTypes resolves the base rune layout for a Death Knight:
// {blood, blood, unholy, unholy, frost, frost} adjusted by learned
// SPELL_AURA_CONVERT_RUNE passives (Blood of the North / Reaping /
// Death Rune Mastery). This is the shared source for initRunes and the
// SMSG_RESYNC_RUNES packet; the resolution logic previously lived inline
// in sendResyncRunes.
func (s *session) dkBaseRuneTypes() [maxRunes]uint8 {
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

// initRunes initializes the rune state for a Death Knight at login,
// mirroring C++ where runes start ready with base types applied
// (Player login path; Player::ResyncRunes sends the initial state).
func (s *session) initRunes() {
	if s.player == nil || s.player.Class != classDeathKnight {
		return
	}
	base := s.dkBaseRuneTypes()
	s.runes = &dkRuneState{base: base, current: base}
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
// (Player.cpp:24840): RUNE_BASE_COOLDOWN reduced by
// SPELL_AURA_MOD_POWER_REGEN_PERCENT auras with MiscValue == POWER_RUNE.
// Go has no such aura plumbing, so the base cooldown is unmodified
// (noted gap, not a stub).
func (s *session) runeBaseCooldownMsForSlot(index int) int64 {
	return runeBaseCooldownMs
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
		runeCost[i] = int32(cost.RuneCost[i])
		// SPELLMOD_COST has no Go spellmod infra (noted gap); C++ applies
		// it here via ApplySpellMod.
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
func (s *session) takeRunePower(ctx context.Context, spell wotlk.Spell, didHit bool, nowMs int64) {
	if s.player == nil || s.player.Class != classDeathKnight {
		return
	}
	if s.server == nil || s.server.Data == nil {
		return
	}
	runeCostData, found, err := s.server.Data.SpellRuneCost(spell.RuneCostID)
	if err != nil || !found {
		return
	}
	noRuneCost := runeCostData.RuneCost[0] == 0 && runeCostData.RuneCost[1] == 0 && runeCostData.RuneCost[2] == 0
	if noRuneCost && runeCostData.RunicPower == 0 {
		return
	}
	if s.runes == nil {
		s.initRunes()
	}
	rs := s.runes
	if rs == nil {
		return
	}
	setRuneCooldown := func(i int) {
		if didHit {
			rs.cooldownEndMs[i] = nowMs + s.runeBaseCooldownMsForSlot(i)
		} else {
			rs.cooldownEndMs[i] = nowMs + runeMissCooldownMs
		}
	}
	var runeCost [4]int32
	for i := 0; i < runeDeath; i++ {
		runeCost[i] = int32(runeCostData.RuneCost[i])
		// SPELLMOD_COST has no Go spellmod infra (noted gap).
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

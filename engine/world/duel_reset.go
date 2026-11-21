package world

import (
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Duel-reset engine bridge for src/server/scripts/World/duel_reset.cpp
// (DuelResetScript, registered via AddSC_duel_reset): when the duel-reset
// configs are on, the 3s countdown start saves each duelist's cooldowns,
// health and mana and resets them, and a DUEL_WON end restores the saved
// state. The Lua script (lua_scripts/world/duel_reset.lua) subscribes to
// PlayerEventDuelStart/PlayerEventDuelEnd (scripting events 10/11) and
// calls these methods; the config gates live here because the scripting
// runtime has no world-config bridge, and the gates wrap exactly the
// operations the C++ script gates (duel_reset.cpp:24-75).

// saveDuelCooldownState mirrors SpellHistory::SaveCooldownStateBeforeDuel
// (SpellHistory.cpp:683-686): the spell-cooldown map is snapshotted.
func (s *session) saveDuelCooldownState() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelCooldowns {
		return
	}
	s.playerStateMu.Lock()
	defer s.playerStateMu.Unlock()
	s.duelCooldownsBefore = append(s.duelCooldownsBefore[:0], s.player.Cooldowns...)
}

// resetDuelSpellCooldowns mirrors DuelResetScript::ResetSpellCooldowns
// (duel_reset.cpp:77-122). Cooldowns whose modded total and category
// durations are under 10 minutes, whose remaining time is under 10
// minutes, and — on duel start — whose elapsed time exceeds 30s are
// cleared with the update arm (SMSG_CLEAR_COOLDOWN per spell, the
// ResetCooldown(spellId, true) analog via clearPlayerCooldown). The
// player's pet gets ResetAllCooldowns (duel_reset.cpp:119-120).
// Fidelity notes: SpellInfo is sSpellMgr->AssertSpellInfo in C++; a
// missing DBC row is skipped here (the terrain.go convention). OnHold
// entries are skipped explicitly below (the predicate term; their +30-day
// End already excludes them via the remaining check).
func (s *session) resetDuelSpellCooldowns(onStart bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || !s.server.Config.ResetDuelCooldowns {
		return
	}
	data := s.server.Data
	s.playerStateMu.RLock()
	cooldowns := append([]spellCooldown(nil), s.player.Cooldowns...)
	s.playerStateMu.RUnlock()
	now := time.Now().Unix()
	const tenMinutesMs = int64(10 * 60 * 1000)
	for _, cd := range cooldowns {
		spell, found, err := data.Spell(cd.Spell)
		if err != nil || !found {
			continue
		}
		if cd.OnHold {
			continue
		}
		remainingMs := (cd.End - now) * 1000
		if remainingMs <= 0 {
			continue
		}
		// duel_reset.cpp:86-93: ApplySpellMod(SPELLMOD_COOLDOWN) folds over
		// the total (and, unless ATTR6_IGNORE_CATEGORY_COOLDOWN_MODS, the
		// category), and SPELL_AURA_MOD_COOLDOWN adds to the caster's own
		// spells — the same fold as the cast path (spells.go:8987-9005).
		totalMs := int64(s.applySpellMod(spell, spellModCooldown, int32(spell.RecoveryTime)))
		if auraMod := s.totalAuraModifier(spellAuraModCooldown); auraMod != 0 && playerHasSpell(s.player, cd.Spell) {
			totalMs += int64(auraMod) * 1000
		}
		categoryMs := int64(spell.CategoryRecoveryTime)
		if spell.AttributesEx6&spellAttr6IgnoreCategoryCooldownMods == 0 {
			categoryMs = int64(s.applySpellMod(spell, spellModCooldown, int32(spell.CategoryRecoveryTime)))
		}
		if totalMs >= tenMinutesMs || categoryMs >= tenMinutesMs || remainingMs >= tenMinutesMs {
			continue
		}
		if onStart {
			if totalMs-remainingMs <= 30*1000 {
				continue
			}
			if categoryMs-remainingMs <= 30*1000 {
				continue
			}
		}
		s.clearPlayerCooldown(cd.Spell)
	}
	if motion := s.controlledPetMotion(s.player.PetGUID); motion != nil {
		s.server.motionMu.Lock()
		motion.SpellCooldowns = make(map[uint32]time.Time)
		motion.SpellCooldownCategories = make(map[uint32]uint32)
		motion.SpellCooldownCategoryEnds = make(map[uint32]time.Time)
		s.server.motionMu.Unlock()
	}
}

// restoreDuelCooldownState mirrors
// SpellHistory::RestoreCooldownStateAfterDuel (SpellHistory.cpp:688-729).
// Duel-made long cooldowns (RecoveryTime or CategoryRecoveryTime over 10
// minutes, raw DBC values) are carried into the saved set, overwriting
// the pre-duel entry for that spell; every other saved entry then wins
// over what the duel left behind, except the onHold gate
// (SpellHistory.cpp:705-708): a saved entry is restored only when
// neither it nor the live entry is onHold — a live onHold entry stays,
// and a saved onHold entry restores nothing. Live entries with no saved
// counterpart stay (the merge only overwrites); ResetSpellCooldowns(false)
// already cleared the short live ones before this runs. The final list
// is spell-id ordered (C++ _spellCooldowns is a std::map) so the
// last-row category holder reads deterministically. Category cooldowns
// are not preserved (SpellHistory.cpp:690). The client gets the restored
// durations in one SMSG_SPELL_COOLDOWN (BuildCooldownPacket,
// SPELL_COOLDOWN_FLAG_INCLUDE_EVENT_COOLDOWNS = 0x2).
func (s *session) restoreDuelCooldownState() {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || !s.server.Config.ResetDuelCooldowns {
		return
	}
	data := s.server.Data
	now := time.Now().Unix()
	const tenMinutesMs = int64(10 * 60 * 1000)
	s.playerStateMu.Lock()
	saved := make(map[uint32]spellCooldown, len(s.duelCooldownsBefore))
	for _, cd := range s.duelCooldownsBefore {
		saved[cd.Spell] = cd
	}
	live := make(map[uint32]spellCooldown, len(s.player.Cooldowns))
	for _, cd := range s.player.Cooldowns {
		live[cd.Spell] = cd
	}
	// Carry: a long duel-made cooldown overwrites the saved entry for
	// its spell (SpellHistory.cpp:694-702), so it survives the merge.
	for spell, cd := range live {
		spellInfo, found, err := data.Spell(spell)
		if err != nil || !found {
			continue
		}
		if int64(spellInfo.RecoveryTime) > tenMinutesMs || int64(spellInfo.CategoryRecoveryTime) > tenMinutesMs {
			saved[spell] = cd
		}
	}
	// Merge: live entries stay unless a saved entry overwrites them; the
	// onHold gate (SpellHistory.cpp:705-708) keeps a live onHold entry
	// and restores nothing for a saved onHold entry.
	restored := live
	for spell, cd := range saved {
		if cd.OnHold {
			continue
		}
		if liveCd, ok := live[spell]; ok && liveCd.OnHold {
			continue
		}
		restored[spell] = cd
	}
	cooldowns := make([]spellCooldown, 0, len(restored))
	for _, cd := range restored {
		cooldowns = append(cooldowns, cd)
	}
	sort.Slice(cooldowns, func(i, j int) bool { return cooldowns[i].Spell < cooldowns[j].Spell })
	s.player.Cooldowns = cooldowns
	packetCooldowns := make([]spellCooldown, 0, len(cooldowns))
	for _, cd := range cooldowns {
		remainingMs := (cd.End - now) * 1000
		// SpellHistory.cpp:716-720: the client restore skips expired,
		// over-10-minute and onHold cooldowns.
		if cd.OnHold || remainingMs <= 0 || remainingMs > tenMinutesMs {
			continue
		}
		packetCooldowns = append(packetCooldowns, cd)
	}
	s.playerStateMu.Unlock()
	if len(packetCooldowns) > 0 {
		buf := protocol.NewBuffer(8 + 1 + len(packetCooldowns)*(4+4))
		buf.WriteU64(s.playerGUID)
		buf.WriteU8(0x2) // SPELL_COOLDOWN_FLAG_INCLUDE_EVENT_COOLDOWNS (SpellHistory.h:41)
		for _, cd := range packetCooldowns {
			buf.WriteU32(cd.Spell)
			buf.WriteU32(uint32((cd.End - now) * 1000))
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buf.Bytes(), true)
	}
}

// saveDuelHealth / restoreDuelHealth mirror Player::SaveHealthBeforeDuel /
// RestoreHealthAfterDuel (Player.h:1801/1803).
func (s *session) saveDuelHealth() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelHealthMana {
		return
	}
	s.playerStateMu.Lock()
	s.duelHealthBefore = s.player.Health
	s.playerStateMu.Unlock()
}

func (s *session) restoreDuelHealth() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelHealthMana {
		return
	}
	s.playerStateMu.Lock()
	s.player.Health = s.duelHealthBefore
	s.playerStateMu.Unlock()
	s.sendPlayerUpdate()
}

// saveDuelMana / restoreDuelMana mirror Player::SaveManaBeforeDuel /
// RestoreManaAfterDuel (Player.h:1802/1804). The mana-only restore (not
// the full-power one) matches the duel_reset.cpp:64-70 gates: only a
// mana-class player (or druid) gets mana back.
func (s *session) saveDuelMana() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelHealthMana {
		return
	}
	s.playerStateMu.Lock()
	s.duelManaBefore = s.player.Powers[0] // POWER_MANA (SharedDefines.h:295)
	s.playerStateMu.Unlock()
}

func (s *session) restoreDuelMana() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelHealthMana {
		return
	}
	s.playerStateMu.Lock()
	s.player.Powers[0] = s.duelManaBefore
	s.playerStateMu.Unlock()
	s.sendPlayerUpdate()
}

// resetAllPowers mirrors Player::ResetAllPowers (Player.cpp:2274-2294):
// full health; full mana / zeroed rage / full energy / zeroed runic
// power by power type.
func (s *session) resetAllPowers() {
	if s == nil || s.player == nil || s.server == nil || !s.server.Config.ResetDuelHealthMana {
		return
	}
	s.playerStateMu.Lock()
	s.player.Health = s.player.MaxHealth
	switch playerPowerType(s.player) {
	case 0: // POWER_MANA
		s.player.Powers[0] = s.player.MaxPowers[0]
	case 1: // POWER_RAGE
		s.player.Powers[1] = 0
	case 3: // POWER_ENERGY
		s.player.Powers[3] = s.player.MaxPowers[3]
	case 6: // POWER_RUNIC_POWER
		s.player.Powers[6] = 0
	}
	s.playerStateMu.Unlock()
	s.sendPlayerUpdate()
}

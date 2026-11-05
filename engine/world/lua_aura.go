package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaAuraRef is the live handle behind a Lua Aura object (AuraMethods.h).
// It re-resolves the record on every call instead of holding the
// *activeAura pointer: a removed aura reads as gone (nil/0/-1) and its
// writes no-op, matching Eluna's Invalidate-on-Remove without a
// dedicated invalidation flag. The player side reads the session's
// activeAuras unlocked, the HasAura bridge precedent in lua_player.go
// (s.auras reads are likewise unlocked from Lua); the creature side
// takes auraMu, the lua_creature_events.go HasAura/GetStackAmount
// precedent. GetAura only ever builds this ref from an engine record
// (session.activeAuras / server.activeCreatureAuras): presence-only
// auras — the Lua AddAura sets (player s.auras, creature creatureAuras)
// — carry no record and GetAura returns nil for them, the same class
// as the creature unit-level GetDuration/GetStackAmount 0-reads.
type luaAuraRef struct {
	server      *Server
	spellID     uint32
	player      *session
	creatureKey creatureAuraKey
	mapID       uint32
	instanceID  uint32
	// snapshot is the removed record the OnRemove hook hands out: the
	// aura is already erased from the live maps when the hook fires, so
	// resolve() returns this instead of a nil lookup and the handler
	// still reads the removed aura's last values (caster, amounts,
	// tick count). Only the remove hook sets it; writes through it land
	// on the detached record and are inert.
	snapshot *activeAura
	// removeMode is the AuraRemoveMode (SpellAuraDefines.h:61-69) the
	// aura was removed with, carried so the OnRemove hook can gate on
	// it like GetTargetApplication()->GetRemoveMode(). Only the remove
	// hook sets it; apply/periodic refs read 0 (AURA_REMOVE_NONE), the
	// C++ AuraApplication default outside a removal.
	removeMode auraRemoveMode
}

func (r *luaAuraRef) resolve() *activeAura {
	if r == nil {
		return nil
	}
	if r.snapshot != nil {
		return r.snapshot
	}
	if r.player != nil {
		return r.player.activeAuras[r.spellID]
	}
	if r.server == nil {
		return nil
	}
	r.server.auraMu.Lock()
	defer r.server.auraMu.Unlock()
	return r.server.activeCreatureAuras[r.creatureKey][r.spellID]
}

// auraRemainingMs is the live remaining duration in ms, the
// lua_creature_events.go GetDuration pattern (RemainingMs at last
// refresh minus elapsed since DurationUpdatedAt). Permanent auras
// (DurationMs 0, no expiry timer) read -1, the SpellInfo::GetDuration
// infinite convention the lua_spell.go GetDuration leg already uses,
// not the unit-level GetDuration 0-read.
func auraRemainingMs(aura *activeAura) int32 {
	if aura == nil {
		return 0
	}
	if aura.DurationMs == 0 {
		return -1
	}
	remaining := int64(aura.RemainingMs)
	if !aura.DurationUpdatedAt.IsZero() {
		if elapsed := time.Since(aura.DurationUpdatedAt).Milliseconds(); elapsed > 0 {
			remaining -= elapsed
		}
	}
	if remaining < 0 {
		remaining = 0
	}
	return int32(remaining)
}

// rearmAuraTimer stops the record's expiry timer and arms a fresh one
// for durationMs, following each side's own apply convention: the
// player load/apply legs arm only for 0 < duration < 18000000
// (player_auras.go load, applyAuraFromCaster) and the creature
// armExpiry leg arms only under the same cap (boss_ai.go). The
// player callback is expirePlayerAura (the load-path timed expiry,
// with its final-aligned-tick leg); the creature callback is
// removeCreatureAuraWithMode with auraRemoveByExpire, the armExpiry
// twin exactly. A duration of 0 stops the timer and zeroes the
// remaining: Go has no Aura::Update sweep to expire it on the next
// tick, so a 0-duration record lingers until removed, documented
// rather than papered over with a synchronous remove (C++ SetDuration
// does not remove synchronously either).
func (r *luaAuraRef) rearmAuraTimer(aura *activeAura, durationMs uint32) {
	if aura == nil {
		return
	}
	if aura.Timer != nil {
		aura.Timer.Stop()
		aura.Timer = nil
	}
	if durationMs == 0 || durationMs >= 18000000 {
		return
	}
	if r.player != nil {
		spellID := r.spellID
		owner := r.player
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			owner.expirePlayerAura(spellID)
		})
		return
	}
	if r.server != nil {
		key := r.creatureKey
		spellID := r.spellID
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			r.server.removeCreatureAuraWithMode(key, spellID, auraRemoveByExpire)
		})
	}
}

func (r *luaAuraRef) casterLevel(aura *activeAura) uint32 {
	if aura == nil {
		return 0
	}
	if r.server != nil && aura.CasterGUID != 0 {
		if obj := r.server.luaUnitObjectForGUID(r.mapID, r.instanceID, aura.CasterGUID); obj != nil {
			if unit, ok := obj.(*scripting.Object); ok && unit != nil {
				if level, ok := unit.Fields["Level"]; ok {
					switch level := level.(type) {
					case uint32:
						return level
					case uint8:
						return uint32(level)
					}
				}
			}
		}
	}
	// The C++ leg (Aura::GetCaster()->GetLevel()) has no fallback for
	// an unresolvable caster; Go falls back to the record's stored
	// caster level (Aura::_casterInfo.Level, SpellAuras.cpp:445-451)
	// rather than returning 0 for an offline caster's aura.
	return uint32(aura.StoredCasterLevel)
}

func (r *luaAuraRef) ownerObject() any {
	if r == nil {
		return nil
	}
	if r.player != nil {
		return r.player.luaPlayer()
	}
	if r.server == nil {
		return nil
	}
	if motion := r.server.findCreatureMotion(r.mapID, r.instanceID, r.creatureKey.GUID); motion != nil {
		return r.server.luaMotionCreature(motion)
	}
	return nil
}

// luaAuraObject builds the Eluna Aura object (AuraMethods.h, all 12
// methods) plus the AuraEffect periodic reads the RegisterAuraEvent
// hook needs: GetTickNumber/GetTotalTicks/GetAmount are C++
// AuraEffect methods (SpellAuraEffects.h:79-80, SpellAuraEffects.cpp:
// 525-550), surfaced on the Aura object because the hook fires once
// per aura tick and Go holds one TickCount/Amounts record per aura
// (all periodic effects of one aura tick in the same Update pass, so
// the per-effect counters agree). SetDuration/SetMaxDuration/
// SetStackAmount/Remove are the live mutations: SetDuration rewrites the remaining + re-arms the
// expiry timer (a field-only write would leave the old timer firing
// at the old deadline); SetMaxDuration rewrites DurationMs only, so
// a later refresh picks the new max up, exactly as Eluna documents;
// SetStackAmount writes the live stack counter (StackCount; the wire
// read is auraWireStackCount, since a DBC StackAmount of 0 means the
// count rides RemainingCharges) and rescales amounts via
// rescaleAuraAmountsByStack (SpellAuras.cpp:1008), refreshing the
// duration when the new count reaches or exceeds the old one, per
// Eluna's SetStackAmount doc; Remove runs the side's plain remove
// funnel (removeAura / removeCreatureAura, both auraRemoveByDefault,
// the Aura::Remove() default), so linked-aura and stat-fold remove
// arms run exactly as for Unit::RemoveAura.
func (r *luaAuraRef) luaAuraObject() *scripting.Object {
	if r == nil {
		return nil
	}
	spellID := r.spellID
	methods := map[string]scripting.ObjectMethod{}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Aura" })
	methods["GetAuraId"] = luaNoArgs(func() any { return spellID })
	methods["GetCasterGUID"] = luaNoArgs(func() any {
		if aura := r.resolve(); aura != nil {
			return aura.CasterGUID
		}
		return uint64(0)
	})
	methods["GetCaster"] = luaNoArgs(func() any {
		aura := r.resolve()
		if aura == nil || aura.CasterGUID == 0 || r.server == nil {
			return nil
		}
		return r.server.luaUnitObjectForGUID(r.mapID, r.instanceID, aura.CasterGUID)
	})
	methods["GetCasterLevel"] = luaNoArgs(func() any { return r.casterLevel(r.resolve()) })
	methods["GetDuration"] = luaNoArgs(func() any { return auraRemainingMs(r.resolve()) })
	methods["GetMaxDuration"] = luaNoArgs(func() any {
		aura := r.resolve()
		if aura == nil {
			return int32(0)
		}
		if aura.DurationMs == 0 {
			return int32(-1)
		}
		return int32(aura.DurationMs)
	})
	methods["GetStackAmount"] = luaNoArgs(func() any { return uint32(auraWireStackCount(r.resolve())) })
	// GetTickNumber is AuraEffect::GetTickNumber (SpellAuraEffects.h),
	// the _ticksDone counter: both periodic-tick executors increment
	// TickCount before the hook fires (spells.go), so the first tick
	// already reads 1, like C++ (SpellAuraEffects.cpp:827-830). A
	// removed aura reads 0.
	methods["GetTickNumber"] = luaNoArgs(func() any {
		if aura := r.resolve(); aura != nil {
			return aura.TickCount
		}
		return uint32(0)
	})
	// GetRemoveMode is AuraApplication::GetRemoveMode
	// (SpellAuras.h): the AuraRemoveMode the aura was removed with
	// (SpellAuraDefines.h:61-69: NONE 0, DEFAULT 1, CANCEL 2,
	// ENEMY_SPELL 3, EXPIRE 4, DEATH 5 — the Go auraRemoveMode iota
	// in spells.go carries the same values in the same order, so the
	// read is a plain int cast, not a translation table). Only the
	// OnRemove hook's snapshot carries a real mode; apply/periodic
	// refs read 0.
	methods["GetRemoveMode"] = luaNoArgs(func() any { return int(r.removeMode) })
	// GetTotalTicks is AuraEffect::GetTotalTicks
	// (SpellAuraEffects.cpp:540-550): 0 when the aura has no amplitude
	// or is permanent, else maxDuration/amplitude, +1 when the spell
	// carries SPELL_ATTR5_START_PERIODIC_AT_APPLY. An unknown spell
	// record reads the +1 leg as 0 rather than failing the call.
	methods["GetTotalTicks"] = luaNoArgs(func() any {
		aura := r.resolve()
		if aura == nil || aura.PeriodMs == 0 || aura.DurationMs == 0 {
			return uint32(0)
		}
		total := aura.DurationMs / aura.PeriodMs
		if r.server != nil && r.server.Data != nil {
			if sp, found, err := r.server.Data.Spell(spellID); err == nil && found {
				if sp.AttributesEx5&spellAttr5StartPeriodicAtApply != 0 {
					total++
				}
			}
		}
		return total
	})
	// GetAmount([effectIndex]) is AuraEffect::GetAmount: the live
	// effect amount (Amounts, rescaled by stack), defaulting to effect
	// 0. An out-of-range index or a removed aura reads 0.
	methods["GetAmount"] = func(_ context.Context, args []any) ([]any, error) {
		index := 0
		if len(args) > 0 && args[0] != nil {
			v, err := luaInt64Arg(args, 0)
			if err != nil {
				return nil, err
			}
			index = int(v)
		}
		aura := r.resolve()
		if aura == nil || index < 0 || index >= len(aura.Amounts) {
			return []any{int32(0)}, nil
		}
		return []any{aura.Amounts[index]}, nil
	}
	// GetTriggerSpell([effectIndex]) is SpellInfo::Effects[eff].
	// TriggerSpell (SpellInfo.h, the DBC trigger of a
	// PERIODIC_TRIGGER_SPELL effect): the spell id the Go default
	// case 23/227 cast would fire. Reads the loaded Spell.dbc record
	// for this aura's spell; an unknown spell, an out-of-range
	// index, or a removed aura reads 0, so a Lua port that casts
	// only when the id is nonzero never fires a bogus 0-id cast.
	// Default effect 0, the C++ GetEffIndex() twin for the
	// single-effect auras this bridge serves.
	methods["GetTriggerSpell"] = func(_ context.Context, args []any) ([]any, error) {
		index := 0
		if len(args) > 0 && args[0] != nil {
			v, err := luaInt64Arg(args, 0)
			if err != nil {
				return nil, err
			}
			index = int(v)
		}
		if r.server == nil || r.server.Data == nil || index < 0 {
			return []any{uint32(0)}, nil
		}
		sp, found, err := r.server.Data.Spell(spellID)
		if err != nil || !found || index >= len(sp.Effects) {
			return []any{uint32(0)}, nil
		}
		return []any{sp.Effects[index].TriggerSpell}, nil
	}
	// GetSpellEffectCalcValue(spellID[, effectIndex]) is
	// SpellInfo::Effects[eff].CalcValue() (SpellInfo.cpp:402-440,
	// the no-caster overload): the DBC base points plus the DieSides
	// roll, with no level scaling (caster is null, so
	// RealPointsPerLevel never applies). Reads the loaded Spell.dbc
	// record for the given spell, not this aura's spell, so a handler
	// can price another spell's effect — the blood-queen pact
	// damage leg (boss_blood_queen_lana_thel.cpp:740-741). An unknown
	// spell, a missing id, or an out-of-range index reads 0, the
	// GetTriggerSpell convention, so a port that casts only on a
	// nonzero value never fires a bogus 0-id cast.
	methods["GetSpellEffectCalcValue"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{int32(0)}, nil
		}
		targetSpell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		index := 0
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			index = int(v)
		}
		if r.server == nil || r.server.Data == nil || index < 0 {
			return []any{int32(0)}, nil
		}
		sp, found, err := r.server.Data.Spell(targetSpell)
		if err != nil || !found || index >= len(sp.Effects) {
			return []any{int32(0)}, nil
		}
		return []any{sp.Effects[index].CalcValue()}, nil
	}
	// GetSpellStackAmount(spellID) is SpellInfo::StackAmount
	// (Spell.dbc field 49 = CumulativeAura, DBCStructure.h:1440):
	// the max stack count of the given spell. Reads the loaded
	// Spell.dbc record for the given spell, not this aura's spell.
	// An unknown spell reads 0, the GetTriggerSpell convention —
	// the choking-vines handler (spell_generic.cpp:968-974) gates
	// its wound cast on GetStackAmount() == StackAmount, so a 0
	// read never fires the cast. Default this aura's spell.
	methods["GetSpellStackAmount"] = func(_ context.Context, args []any) ([]any, error) {
		targetSpell := spellID
		if len(args) > 0 && args[0] != nil {
			v, err := luaUint32Arg(args, 0)
			if err != nil {
				return nil, err
			}
			targetSpell = v
		}
		if r.server == nil || r.server.Data == nil {
			return []any{uint32(0)}, nil
		}
		sp, found, err := r.server.Data.Spell(targetSpell)
		if err != nil || !found {
			return []any{uint32(0)}, nil
		}
		return []any{sp.StackAmount}, nil
	}
	// GetSpellEffectIsEffect(spellID, effectIndex) is
	// SpellInfo::Effects[eff].IsEffect() (SpellInfo.h: the effect
	// slot carries a real effect when Effect != SPELL_EFFECT_NONE):
	// whether the given spell's effect slot is populated. Reads the
	// loaded Spell.dbc record for the given spell. An unknown spell
	// or an out-of-range index reads false — the
	// remove-on-full-health handler (spell_generic.cpp:2955-2969)
	// treats a missing slot as no effect, matching the C++
	// onlyEffect gate.
	methods["GetSpellEffectIsEffect"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 2 {
			return []any{false}, nil
		}
		targetSpell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		index, err := luaInt64Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if r.server == nil || r.server.Data == nil || index < 0 {
			return []any{false}, nil
		}
		sp, found, err := r.server.Data.Spell(targetSpell)
		if err != nil || !found || int(index) >= len(sp.Effects) {
			return []any{false}, nil
		}
		return []any{sp.Effects[index].Effect != 0}, nil
	}
	// GetSpellEffectMiscValue(spellID, effectIndex) is
	// SpellInfo::Effects[eff].MiscValue (Spell.dbc, the
	// AuraEffect::GetMiscValue twin): the per-effect misc value a
	// C++ handler reads off another aura's effect — the drain-soul
	// glyph leg (spell_warlock.cpp:557-559) rolls against the
	// MiscValue of the glyph aura's EFFECT_0. Reads the loaded
	// Spell.dbc record for the given spell; an unknown spell or an
	// out-of-range index reads 0, the GetTriggerSpell convention.
	methods["GetSpellEffectMiscValue"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{int32(0)}, nil
		}
		targetSpell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		index := 0
		if len(args) > 1 && args[1] != nil {
			v, err := luaInt64Arg(args, 1)
			if err != nil {
				return nil, err
			}
			index = int(v)
		}
		if r.server == nil || r.server.Data == nil || index < 0 {
			return []any{int32(0)}, nil
		}
		sp, found, err := r.server.Data.Spell(targetSpell)
		if err != nil || !found || index >= len(sp.Effects) {
			return []any{int32(0)}, nil
		}
		return []any{sp.Effects[index].MiscValue}, nil
	}
	// GetProcChance is SpellInfo::ProcChance (Spell.dbc field 35,
	// DBCStructure.h:1425): the roll_chance_i percent a C++
	// periodic handler gates on. An unknown spell or a removed aura
	// reads 0, so the gate never fires without DBC data.
	methods["GetProcChance"] = luaNoArgs(func() any {
		if r.server == nil || r.server.Data == nil {
			return uint32(0)
		}
		sp, found, err := r.server.Data.Spell(spellID)
		if err != nil || !found {
			return uint32(0)
		}
		return sp.ProcChance
	})
	methods["GetOwner"] = luaNoArgs(func() any { return r.ownerObject() })
	methods["SetDuration"] = func(_ context.Context, args []any) ([]any, error) {
		duration, err := luaInt64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		aura := r.resolve()
		if aura == nil {
			return nil, nil
		}
		if duration <= 0 {
			if r.server != nil && r.player == nil {
				r.server.auraMu.Lock()
				aura.RemainingMs = 0
				aura.DurationUpdatedAt = time.Now()
				r.rearmAuraTimer(aura, 0)
				r.server.auraMu.Unlock()
			} else {
				aura.RemainingMs = 0
				aura.DurationUpdatedAt = time.Now()
				r.rearmAuraTimer(aura, 0)
			}
			return nil, nil
		}
		if r.server != nil && r.player == nil {
			r.server.auraMu.Lock()
			aura.RemainingMs = uint32(duration)
			aura.DurationUpdatedAt = time.Now()
			r.rearmAuraTimer(aura, uint32(duration))
			r.server.auraMu.Unlock()
		} else {
			aura.RemainingMs = uint32(duration)
			aura.DurationUpdatedAt = time.Now()
			r.rearmAuraTimer(aura, uint32(duration))
		}
		return nil, nil
	}
	methods["SetMaxDuration"] = func(_ context.Context, args []any) ([]any, error) {
		duration, err := luaInt64Arg(args, 0)
		if err != nil {
			return nil, err
		}
		aura := r.resolve()
		if aura == nil {
			return nil, nil
		}
		if duration < 0 {
			duration = 0
		}
		if r.server != nil && r.player == nil {
			r.server.auraMu.Lock()
			aura.DurationMs = uint32(duration)
			r.server.auraMu.Unlock()
		} else {
			aura.DurationMs = uint32(duration)
		}
		return nil, nil
	}
	methods["SetStackAmount"] = func(_ context.Context, args []any) ([]any, error) {
		amount, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		// C++ takes uint8: truncate, the CHECKVAL<uint8> value width.
		newCount := uint8(amount)
		if r.server != nil && r.player == nil {
			r.server.auraMu.Lock()
			defer r.server.auraMu.Unlock()
			aura := r.server.activeCreatureAuras[r.creatureKey][r.spellID]
			if aura == nil {
				return nil, nil
			}
			oldCount := uint32(auraWireStackCount(aura))
			aura.StackCount = newCount
			rescaleAuraAmountsByStack(aura, oldCount, uint32(newCount))
			if uint32(newCount) >= oldCount && aura.DurationMs > 0 {
				aura.RemainingMs = aura.DurationMs
				aura.DurationUpdatedAt = time.Now()
				r.rearmAuraTimer(aura, aura.DurationMs)
			}
			return nil, nil
		}
		aura := r.resolve()
		if aura == nil {
			return nil, nil
		}
		oldCount := uint32(auraWireStackCount(aura))
		aura.StackCount = newCount
		rescaleAuraAmountsByStack(aura, oldCount, uint32(newCount))
		if uint32(newCount) >= oldCount && aura.DurationMs > 0 {
			aura.RemainingMs = aura.DurationMs
			aura.DurationUpdatedAt = time.Now()
			r.rearmAuraTimer(aura, aura.DurationMs)
		}
		return nil, nil
	}
	methods["Remove"] = func(_ context.Context, _ []any) ([]any, error) {
		if r.player != nil {
			r.player.removeAura(r.spellID)
			return nil, nil
		}
		if r.server != nil {
			r.server.removeCreatureAura(r.creatureKey, r.spellID)
		}
		return nil, nil
	}
	return &scripting.Object{Type: "Aura", Fields: map[string]any{"Entry": spellID}, Methods: methods}
}

// rankedAuraEffectAmount is Unit::GetAuraEffectOfRankedSpell
// (Unit.cpp:4498-4508): the first aura in the chain of the queried
// spell — GetFirstSpellInChain/GetNextSpellInChain, here the
// spell_ranks.first_spell_id twin spellFirstRank — carrying the
// effect index, or false when no chain member is present. Auras of
// one chain never coexist (same-chain applications replace), so
// scan order is immaterial; when the world DB has no spell_ranks
// row, spellFirstRank returns the spell itself and this degrades
// to the exact-ID lookup. The caller passes a snapshot slice:
// the player side uses loadedAuras, the creature side a copy taken
// under auraMu, so a concurrent map write cannot fault the scan.
func (s *Server) rankedAuraEffectAmount(auras []*activeAura, spellID uint32, effIndex int) (int32, bool) {
	if s == nil || effIndex < 0 {
		return 0, false
	}
	first := s.spellFirstRank(spellID)
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		if s.spellFirstRank(aura.SpellID) != first {
			continue
		}
		if effIndex >= len(aura.Amounts) || aura.EffectMask&(1<<uint(effIndex)) == 0 {
			continue
		}
		return aura.Amounts[effIndex], true
	}
	return 0, false
}

// creatureDummyAuraAmountByIconEffIndex is the creature-side twin
// of session.dummyAuraAmountByIconEffIndex (spell_stats.go:1223):
// Unit::GetDummyAuraEffect(family, icon, effIndex)
// (Unit.cpp:4540-4543 -> 4510-4522) over the motion's engine auras —
// the first live DUMMY aura whose spell matches family+icon (with
// no family flags set) at exactly the given effect index.
func (s *Server) creatureDummyAuraAmountByIconEffIndex(key creatureAuraKey, family, iconID uint32, effIndex int) (int32, bool) {
	if s == nil || s.Data == nil || effIndex < 0 {
		return 0, false
	}
	s.auraMu.Lock()
	auras := make([]*activeAura, 0, len(s.activeCreatureAuras[key]))
	for _, aura := range s.activeCreatureAuras[key] {
		auras = append(auras, aura)
	}
	s.auraMu.Unlock()
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found || auraSpell.SpellFamilyName != family || auraSpell.SpellIconID != iconID {
			continue
		}
		if auraSpell.SpellFamilyFlags[0] != 0 || auraSpell.SpellFamilyFlags[1] != 0 || auraSpell.SpellFamilyFlags[2] != 0 {
			continue
		}
		if effIndex >= len(aura.Amounts) || effIndex >= len(auraSpell.Effects) || aura.EffectMask&(1<<uint(effIndex)) == 0 {
			continue
		}
		eff := auraSpell.Effects[effIndex]
		if !spellEffectIsAuraEffect(eff) || eff.Aura != spellAuraDummy {
			continue
		}
		amount := aura.Amounts[effIndex]
		if amount == 0 {
			amount = eff.BasePoints
		}
		return amount, true
	}
	return 0, false
}

// fireAuraPeriodicTick dispatches the RegisterAuraEvent hook for the aura's
// spell (AuraEventOnPeriodic, args (event, aura, target)) and reports
// whether any handler returned boolean true. That is the Go model of
// Aura::CallScriptEffectPeriodicHandlers (SpellAuras.cpp:2341-2361): a
// true return is AuraScript PreventDefaultAction(), and the caller skips
// the default tick — the channel re-face and the whole PeriodicTick switch
// (SpellAuraEffects.cpp:868-872). The hook fires for every periodic aura
// type, like C++; Lua errors are logged by the runtime, never propagated.
func (s *Server) fireAuraPeriodicTick(ctx context.Context, aura *activeAura, auraObj *scripting.Object, target any) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || aura == nil || auraObj == nil {
		return false
	}
	results, _ := s.Features.Scripts.TriggerAuraEvent(ctx, aura.SpellID, scripting.AuraEventOnPeriodic, scripting.AuraEventOnPeriodic, auraObj, target)
	return luaHookVeto(results)
}

// fireAuraApply dispatches the RegisterAuraEvent OnApply hook for a
// freshly applied aura (args (event, aura, target)). Notification-only:
// returns are ignored, there is no PreventDefault model for apply (see
// the AuraEvents contract in runtime.go).
func (s *Server) fireAuraApply(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnApply, scripting.AuraEventOnApply, auraObj, target)
}

// fireAuraRemove dispatches the RegisterAuraEvent OnRemove hook for a
// removed aura (args (event, aura, target)). Notification-only like
// fireAuraApply; the aura object reads the removed record's snapshot.
func (s *Server) fireAuraRemove(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnRemove, scripting.AuraEventOnRemove, auraObj, target)
}

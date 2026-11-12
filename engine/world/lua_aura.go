package world

import (
	"context"
	"fmt"
	"strconv"
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
	// pending is the not-yet-applied record the OnCheckAreaTarget hook
	// hands out: the aura has no live record on the candidate target
	// (that is what the hook is deciding), so resolve() returns this
	// instead of a live lookup — a live lookup would find a same-spell
	// aura from another caster and report the wrong caster. Only the
	// check hook sets it; GetCasterGUID/GetCaster read the incoming
	// caster, amount/duration reads are 0, and GetOwner reads nil
	// (the C++ owner is the dynobj).
	pending *activeAura
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
	if r.pending != nil {
		return r.pending
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
	// GetSpellMaxDuration is SpellInfo::GetMaxDuration
	// (SpellInfo.cpp:3084-3089): the MaxDuration column of
	// SpellDuration.dbc for this aura's spell, no per-level
	// scaling. An unknown spell (no Spell.dbc) reads 0, the
	// GetTriggerSpell convention. Backs the Cut to the Chase
	// (51664) refresh arm (spell_rogue.cpp:196-233), which reads
	// the spell-info max, not the live aura's DurationMs.
	methods["GetSpellMaxDuration"] = luaNoArgs(func() any {
		if r.server == nil || r.server.Data == nil {
			return int32(0)
		}
		sp, found, err := r.server.Data.Spell(spellID)
		if err != nil || !found {
			return int32(0)
		}
		maxDur, ok, err := r.server.Data.SpellMaxDuration(sp.DurationIndex)
		if err != nil || !ok {
			return int32(0)
		}
		return int32(maxDur)
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
	// GetSpellMaxTicks(spellID) is SpellInfo::GetMaxTicks
	// (SpellInfo.cpp, duration/amplitude over the periodic effects):
	// the tick count of the given spell. Reads the loaded Spell.dbc
	// record for the given spell, not this aura's spell. An unknown
	// spell reads 0, the GetTriggerSpell convention — the
	// piercing-shots handler (spell_hunter.cpp:1091-1105) divides the
	// bleed by it, so a 0 read never fires the cast. Default this
	// aura's spell.
	methods["GetSpellMaxTicks"] = func(_ context.Context, args []any) ([]any, error) {
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
		return []any{r.server.serverSpellMaxTicks(sp)}, nil
	}
	// GetSpellWithRank(firstSpellID) is sSpellMgr->GetSpellWithRank
	// (SpellMgr.cpp): the spell id of this aura's rank in
	// firstSpellID's chain, or 0 when the chain has no such rank. The
	// rank is this aura's own spell rank (SpellInfo::GetRank via the
	// spell_ranks world table, spells.go spellRank) — the
	// improved-blizzard leg (spell_mage.cpp:484-488) casts the
	// rank-matched Chilled on the proc target. An unknown rank reads
	// 0, the GetTriggerSpell convention, so a port that casts only on
	// a nonzero id never fires a bogus 0-id cast.
	methods["GetSpellWithRank"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) < 1 {
			return []any{uint32(0)}, nil
		}
		firstSpell, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if r.server == nil {
			return []any{uint32(0)}, nil
		}
		return []any{r.server.spellWithRank(firstSpell, r.server.spellRank(spellID))}, nil
	}
	// GetRank is SpellInfo::GetRank (SpellInfo.cpp) via the
	// spell_ranks world table (spells.go spellRank): the 1-based rank
	// of this aura's spell within its first-rank chain, or 0 when the
	// spell has no rank row. The ignite leg (spell_mage.cpp:1071)
	// scales the dot by 8 * rank; a 0 read means no DBC rank data,
	// so a port that gates on a nonzero rank never fires a bogus
	// 0-damage cast.
	methods["GetRank"] = luaNoArgs(func() any {
		if r.server == nil {
			return uint8(0)
		}
		return r.server.spellRank(spellID)
	})
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
	// Remove([mode]) is Aura::Remove(mode): drops this aura, recording
	// the remove mode the OnRemove handlers read via GetRemoveMode.
	// No argument keeps the old mode-blind call (auraRemoveByDefault);
	// mode is the AuraRemoveMode iota (None 0, Default 1, Cancel 2,
	// EnemySpell 3, Expire 4, Death 5 — spells.go:34384, the
	// SpellAuraDefines.h:61-69 twin). The kalecgos
	// curse-of-boundless-agony OnApply cancel needs mode 2 so the
	// already-ported ev3 handler's CANCEL guard sees it.
	methods["Remove"] = func(_ context.Context, args []any) ([]any, error) {
		mode := auraRemoveByDefault
		if len(args) > 0 && args[0] != nil {
			v, err := luaInt64Arg(args, 0)
			if err != nil {
				return nil, err
			}
			if v < 0 || v > 5 {
				return nil, fmt.Errorf("invalid aura remove mode")
			}
			mode = auraRemoveMode(v)
		}
		if r.player != nil {
			r.player.removeAuraWithMode(r.spellID, mode)
			return nil, nil
		}
		if r.server != nil {
			r.server.removeCreatureAuraWithMode(r.creatureKey, r.spellID, mode)
		}
		return nil, nil
	}
	// SetAmount(amount[, effectIndex]) is AuraEffect::SetAmount: writes
	// the live effect amount — Amounts[index] plus the effect-0
	// Amount the damage/heal ticks read, the rescaleAuraAmountsByStack
	// twin — and the write side of GetAmount. The kalecgos
	// curse-of-boundless-agony periodic-update doubling needs it
	// (boss_kalecgos.cpp:772-776). Default effect 0; an out-of-range
	// index is a silent no-op, the GetAmount read convention.
	methods["SetAmount"] = func(_ context.Context, args []any) ([]any, error) {
		amount, err := luaInt64Arg(args, 0)
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
		if index < 0 || index >= 3 {
			return nil, nil
		}
		if r.server != nil && r.player == nil {
			r.server.auraMu.Lock()
			defer r.server.auraMu.Unlock()
			aura := r.server.activeCreatureAuras[r.creatureKey][r.spellID]
			if aura == nil {
				return nil, nil
			}
			aura.Amounts[index] = int32(amount)
			if index == 0 {
				aura.Amount = uint32(amount)
			}
			return nil, nil
		}
		aura := r.resolve()
		if aura == nil {
			return nil, nil
		}
		aura.Amounts[index] = int32(amount)
		if index == 0 {
			aura.Amount = uint32(amount)
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

// hasAuraRemoveHook reports whether any RegisterAuraEvent(...,
// OnRemove, ...) handler is registered for the spell, so the remove
// funnels can fire the hook before the CONTROL_VEHICLE default leg
// only when a handler could run (the hasAuraBeforeDispelHook twin).
func (s *Server) hasAuraRemoveHook(spellID uint32) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	return s.Features.Scripts.HasHook("aura:"+strconv.FormatUint(uint64(spellID), 10), scripting.AuraEventOnRemove)
}

// fireAuraRemove dispatches the RegisterAuraEvent OnRemove hook for a
// removed aura (args (event, aura, target)) and reports whether any
// handler returned boolean true. That is the Go model of AuraScript
// PreventDefaultAction() in an OnEffectRemove handler:
// AuraEffect::HandleEffect (SpellAuraEffects.cpp:727-744) calls the
// script remove handlers BEFORE the default effect handler and skips
// the default when prevented, while AfterEffectRemove handlers run
// after it — Go fires ev3 once per removal, so the pre-leg fire in
// the remove funnels carries the veto and the AfterEffectRemove legs
// ported on ev3 run in the same single fire (their bodies never
// depend on the default leg). The aura object reads the removed
// record's snapshot.
func (s *Server) fireAuraRemove(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return false
	}
	results, _ := s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnRemove, scripting.AuraEventOnRemove, auraObj, target)
	return luaHookVeto(results)
}

// fireAuraChangeAmount dispatches the RegisterAuraEvent OnChangeAmount hook
// for an aura whose stack count rose on a merge re-cast (args (event, aura,
// target)). Notification-only like fireAuraApply: the AURA_EFFECT_HANDLE_
// CHANGE_AMOUNT handlers run after the stack is stored, and there is no
// PreventDefault model for amount changes.
func (s *Server) fireAuraChangeAmount(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnChangeAmount, scripting.AuraEventOnChangeAmount, auraObj, target)
}

// fireAuraDispel dispatches the RegisterAuraEvent OnDispel hook for a
// dispelled aura (args (event, aura, target, dispeller)). Notification-only
// like fireAuraApply: the C++ AfterDispel handlers run after the dispel
// resolves and cannot veto it, and the dispeller is already fixed by the
// time the hook fires.
func (s *Server) fireAuraDispel(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any, dispeller any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnDispel, scripting.AuraEventOnDispel, auraObj, target, dispeller)
}

// hasAuraBeforeDispelHook reports whether any RegisterAuraEvent(...,
// OnBeforeDispel, ...) handler is registered for the spell, so the
// dispel funnels can skip the before-dispel fire when no handler could
// run (the hasAuraProcHook twin).
func (s *Server) hasAuraBeforeDispelHook(spellID uint32) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	return s.Features.Scripts.HasHook("aura:"+strconv.FormatUint(uint64(spellID), 10), scripting.AuraEventOnBeforeDispel)
}

// fireAuraBeforeDispel dispatches the RegisterAuraEvent OnBeforeDispel
// hook for an aura about to be dispelled (args (event, aura, target,
// dispeller)) and reports whether any handler returned boolean true.
// That is the Go model of DispelInfo::SetRemovedCharges(0) in an
// AuraScript BeforeDispel handler: the caller skips the charge/stack
// decrement for that dispel application, matching C++
// (Unit::RemoveAurasDueToSpellByDispel, Unit.cpp:3936-3967, calls
// CallScriptDispel before ModCharges/ModStackAmount and
// CallScriptAfterDispel after). The hook fires from the snapshot taken
// before the decrement, so the aura object still resolves GetCaster.
// Lua errors are logged by the runtime, never propagated.
func (s *Server) fireAuraBeforeDispel(ctx context.Context, spellID uint32, auraObj *scripting.Object, target any, dispeller any) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || auraObj == nil {
		return false
	}
	results, _ := s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnBeforeDispel, scripting.AuraEventOnBeforeDispel, auraObj, target, dispeller)
	return luaHookVeto(results)
}

// hasAuraProcHook reports whether any RegisterAuraEvent(..., OnProc, ...)
// handler is registered for the spell, so the proc loop can skip building
// the aura/actor objects when no handler could run (Runtime.HasHook twin
// of the Eluna Bindings::HasBindingsFor gate).
func (s *Server) hasAuraProcHook(spellID uint32) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	return s.Features.Scripts.HasHook("aura:"+strconv.FormatUint(uint64(spellID), 10), scripting.AuraEventOnProc)
}

// fireAuraProc dispatches the RegisterAuraEvent OnProc hook for one
// eligible proc effect (args (event, aura, target, actor, procTarget,
// damage); target is the aura holder, actor the proc actor, procTarget
// the proc target, damage the proc event's damage) and reports whether
// any handler returned boolean true. That is the Go model of AuraScript
// PreventDefaultAction() in an OnEffectProc handler: the caller skips
// the effect's default leg. The hook fires after the proc gates and
// charge/cooldown consumption, like C++ (PrepareProcToTrigger runs
// before TriggerProcOnEvent), so a veto never refunds a spent charge.
// Lua errors are logged by the runtime, never propagated.
func (s *Server) fireAuraProc(ctx context.Context, aura *activeAura, auraObj *scripting.Object, target any, actor any, procTarget any, damage uint32) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || aura == nil || auraObj == nil {
		return false
	}
	results, _ := s.Features.Scripts.TriggerAuraEvent(ctx, aura.SpellID, scripting.AuraEventOnProc, scripting.AuraEventOnProc, auraObj, target, actor, procTarget, damage)
	return luaHookVeto(results)
}

// hasAuraCheckAreaTargetHook reports whether any RegisterAuraEvent(...,
// OnCheckAreaTarget, ...) handler is registered for the spell, so the
// dynobj apply sites can skip building the pending aura/target objects
// when no handler could run (the hasAuraProcHook twin).
func (s *Server) hasAuraCheckAreaTargetHook(spellID uint32) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	return s.Features.Scripts.HasHook("aura:"+strconv.FormatUint(uint64(spellID), 10), scripting.AuraEventOnCheckAreaTarget)
}

// fireAuraCheckAreaTarget dispatches the RegisterAuraEvent
// OnCheckAreaTarget hook for a candidate dynobj-aura target (args
// (event, aura, target)) and reports whether the aura may be applied.
// That is the Go model of Aura::CallScriptCheckAreaTargetHandlers
// (SpellAuras.cpp:2233-2248): the C++ results are ANDed, so any handler
// returning boolean false rejects the target; a handler returning true
// or nothing passes it. A rejected target is not added to the dynobj's
// holder map, so the next target-map pass re-checks it, like C++
// (Aura::UpdateTargetMap re-evaluates candidates every pass). No hook
// registered, or an unresolvable target object, passes the target.
func (s *Server) fireAuraCheckAreaTarget(ctx context.Context, spellID uint32, casterGUID uint64, mapID, instanceID uint32, targetGUID uint64) bool {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || !s.hasAuraCheckAreaTargetHook(spellID) {
		return true
	}
	target := s.luaUnitObjectForGUID(mapID, instanceID, targetGUID)
	if target == nil {
		return true
	}
	ref := &luaAuraRef{server: s, spellID: spellID, mapID: mapID, instanceID: instanceID, pending: &activeAura{CasterGUID: casterGUID}}
	results, _ := s.Features.Scripts.TriggerAuraEvent(ctx, spellID, scripting.AuraEventOnCheckAreaTarget, scripting.AuraEventOnCheckAreaTarget, ref.luaAuraObject(), target)
	for _, result := range results {
		if blocked, ok := result.(bool); ok && !blocked {
			return false
		}
	}
	return true
}

// dispelAuraSnapshot copies the aura record a dispel is about to
// decrement, so the OnDispel hook's aura object still resolves GetCaster
// and the amounts after a full removal erases the live record
// (Unit::RemoveAurasDueToSpellByDispel, Unit.cpp:3936-3967, calls the
// AfterDispel hook after ModCharges/ModStackAmount). A nil session reads
// the creature-aura map under auraMu; a non-nil session reads the
// target's activeAuras under its castMu. Returns nil when no live
// record exists (already gone, e.g. a concurrent expiry).
func (s *Server) dispelAuraSnapshot(ts *session, key creatureAuraKey, spellID uint32) *activeAura {
	if ts != nil {
		ts.castMu.Lock()
		defer ts.castMu.Unlock()
		if aura := ts.activeAuras[spellID]; aura != nil {
			snap := *aura
			return &snap
		}
		return nil
	}
	if s == nil {
		return nil
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if auras := s.activeCreatureAuras[key]; auras != nil {
		if aura := auras[spellID]; aura != nil {
			snap := *aura
			return &snap
		}
	}
	return nil
}

// dispelAuraRef builds the luaAuraRef for the OnDispel hook from a
// snapshot captured before the charge decrement (see
// dispelAuraSnapshot). The snapshot is what resolve() returns, so the
// handler reads the dispelled aura's last values even when the live
// record is gone.
func (s *Server) dispelAuraRef(ts *session, key creatureAuraKey, spellID uint32, snap *activeAura, mapID, instanceID uint32) *luaAuraRef {
	if ts != nil {
		return &luaAuraRef{server: s, spellID: spellID, player: ts, mapID: mapID, instanceID: instanceID, snapshot: snap}
	}
	return &luaAuraRef{server: s, spellID: spellID, creatureKey: key, mapID: mapID, instanceID: instanceID, snapshot: snap}
}

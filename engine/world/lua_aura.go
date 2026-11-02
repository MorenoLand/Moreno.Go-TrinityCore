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
}

func (r *luaAuraRef) resolve() *activeAura {
	if r == nil {
		return nil
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
// methods). SetDuration/SetMaxDuration/SetStackAmount/Remove are the
// live mutations: SetDuration rewrites the remaining + re-arms the
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

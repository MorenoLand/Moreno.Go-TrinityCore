package world

import (
	"context"
	"fmt"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// spellScriptAction carries the script-side mutations a
// PLAYER_EVENT_ON_SPELL_CAST (5) handler requests on the in-flight cast
// (SpellMethods.h Cancel/Finish/Cast/SetAutoRepeat). The Lua Spell object
// is built per fire, so the action is per fire too: the fire site reads
// the flags the moment the synchronous trigger returns, and a Spell object
// stashed past its fire mutates flags nobody reads. Abort flags are
// sticky across handlers and abort wins over re-cast when handlers set
// both.
type spellScriptAction struct {
	cancelled     bool
	finished      bool
	castAgain     bool
	castSkipCheck bool
	autoRepeatSet bool
	autoRepeat    bool
}

// spellObjectParams carries the cast-time facts a Lua Spell object reads.
// The Spell object is a snapshot of the in-flight cast, not a live C++
// Spell*: Go executes casts through finishSpellCast (client/item casts)
// and castSpellDirectWithOverrides (triggered casts), neither of which
// keeps a Spell object alive, so the object is built from the cast's
// arguments at the OnPlayerSpellCast fire site (Spell.cpp:3302) and the
// target/caster legs resolve live when a handler calls them, matching
// Eluna's Push(Spell*) of the live object.
type spellObjectParams struct {
	spell          wotlk.Spell
	castTimeMs     int32
	targetGUID     uint64
	targetItemGUID uint64
	castItemGUID   uint64
	dest           protocol.SpellTargetLocation
	hasDest        bool
	action         *spellScriptAction
}

func nilSpellObject(object *scripting.Object) any {
	if object == nil {
		return nil
	}
	return object
}

// luaSpellObject builds the Eluna Spell object (SpellMethods.h). The read
// legs are the ones Go can answer from the cast: GetEntry from the spell
// id, GetCaster from the session player, GetCastTime from the computed
// cast time (activeCast value on the client path, DBC base otherwise),
// GetPowerCost from calculateSpellPowerCost with the item-cast zeroing
// (prepare zeroes m_powerCost for item casts, Spell.cpp:3093),
// GetDuration from SpellDurationBase, which is SpellInfo::GetDuration
// (SpellInfo.cpp:3077-3082: base column, -1 infinite, passive-with-no-
// entry -1). GetTarget follows Eluna's GO -> item -> unit order
// (SpellMethods.h GetTarget); corpse targets have no Go object surface
// and fall through to nil, as do unresolvable targets. GetTargetDest
// returns the packet destination when the dest flag is set, else three
// nils, matching the no-dst arm. SetAutoRepeat, Cast, Cancel and Finish
// write into p.action, which the fire site reads the moment the
// synchronous trigger returns: Cancel/Finish abort the cast before any
// power/effect leg (Spell::_cast notices a handler Cancel only at the
// SelectSpellTargets FINISHED check, Spell.cpp:3413, after pet-assist),
// Cast re-enters the cast funnel and the outer call returns without
// running effects twice, and SetAutoRepeat only affects IsAutoRepeat
// read-back inside the handler — every C++ m_autoRepeat consumer for
// this cast (prepare/update/CheckCast movement arm, Spell.cpp:3101,
// 3153, 5320) already ran before the hook fires, and Go has no
// current-spell slot to re-register, so the flag has no post-handler
// effect. IsAutoRepeat otherwise reads the session auto-repeat slot
// (Unit::_UpdateAutoRepeatSpell model): true only when this spell id
// is the active auto-repeat spell, matching Spell::m_autoRepeat
// (Spell.h:533) for the spell whose object this is.
func (s *session) luaSpellObject(p spellObjectParams) *scripting.Object {
	if s == nil {
		return nil
	}
	spellID := p.spell.ID
	durationMs := int32(0)
	if s.server != nil && s.server.Data != nil {
		if dur, found, err := s.server.Data.SpellDurationBase(p.spell.DurationIndex); err == nil && found {
			durationMs = dur
		} else if !found && p.spell.Attributes&spellAttributePassive != 0 {
			durationMs = -1
		}
	}
	powerCost := uint32(0)
	if p.castItemGUID == 0 {
		powerCost = s.calculateSpellPowerCost(p.spell)
	}
	castTimeMs := p.castTimeMs
	methods := map[string]scripting.ObjectMethod{}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Spell" })
	methods["GetEntry"] = luaNoArgs(func() any { return spellID })
	methods["GetCaster"] = luaNoArgs(func() any { return nilSpellObject(s.luaPlayer()) })
	methods["GetCastTime"] = luaNoArgs(func() any { return castTimeMs })
	methods["GetPowerCost"] = luaNoArgs(func() any { return powerCost })
	methods["GetDuration"] = luaNoArgs(func() any { return durationMs })
	methods["IsAutoRepeat"] = luaNoArgs(func() any {
		if p.action != nil && p.action.autoRepeatSet {
			return p.action.autoRepeat
		}
		return s.autoRepeatSpell == spellID && spellID != 0
	})
	methods["SetAutoRepeat"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("argument 1 is required")
		}
		repeat, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("argument 1 must be boolean")
		}
		if p.action != nil {
			p.action.autoRepeatSet = true
			p.action.autoRepeat = repeat
		}
		return nil, nil
	}
	methods["Cancel"] = func(_ context.Context, _ []any) ([]any, error) {
		if p.action != nil {
			p.action.cancelled = true
		}
		return nil, nil
	}
	methods["Finish"] = func(_ context.Context, _ []any) ([]any, error) {
		if p.action != nil {
			p.action.finished = true
		}
		return nil, nil
	}
	methods["Cast"] = func(_ context.Context, args []any) ([]any, error) {
		skipCheck := false
		if len(args) > 0 {
			b, ok := args[0].(bool)
			if !ok {
				return nil, fmt.Errorf("argument 1 must be boolean")
			}
			skipCheck = b
		}
		if p.action != nil {
			p.action.castAgain = true
			p.action.castSkipCheck = skipCheck
		}
		return nil, nil
	}
	methods["GetTarget"] = func(ctx context.Context, _ []any) ([]any, error) {
		if p.targetGUID != 0 && uint16(p.targetGUID>>48) == 0xF110 {
			return []any{nilSpellObject(s.luaGameObject(ctx, p.targetGUID))}, nil
		}
		if p.targetItemGUID != 0 {
			return []any{nilSpellObject(s.sessionLuaItem(ctx, p.targetItemGUID))}, nil
		}
		if p.targetGUID != 0 && s.server != nil && s.player != nil {
			return []any{s.server.luaUnitObjectForGUID(s.player.Map, s.player.InstanceID, p.targetGUID)}, nil
		}
		return []any{nil}, nil
	}
	// GetTargetDest returns three values, not one: wrap it separately so
	// the triple is not collapsed into a single table value.
	methods["GetTargetDest"] = func(_ context.Context, _ []any) ([]any, error) {
		if !p.hasDest {
			return []any{nil, nil, nil}, nil
		}
		return []any{p.dest.X, p.dest.Y, p.dest.Z}, nil
	}
	return &scripting.Object{Type: "Spell", Fields: map[string]any{"Entry": spellID}, Methods: methods}
}

// firePlayerSpellCast dispatches PLAYER_EVENT_ON_SPELL_CAST (5) as
// (event, player, spell, skipCheck), mirroring Eluna::OnSpellCast
// (PlayerHooks.cpp:324-329) via ScriptMgr::OnPlayerSpellCast
// (ScriptMgr.cpp:2110). Eluna discards the returns for this hook, so
// the fire discards them too — but the trigger runs synchronously, so
// a handler's Cancel/Finish/Cast/SetAutoRepeat writes land on the
// returned action before the caller continues, which is how those
// methods take effect despite the discarded returns.
func (s *session) firePlayerSpellCast(ctx context.Context, p spellObjectParams, skipCheck bool) *spellScriptAction {
	action := &spellScriptAction{}
	p.action = action
	if s == nil || s.player == nil {
		return action
	}
	player := s.luaPlayer()
	if player == nil {
		return action
	}
	spell := s.luaSpellObject(p)
	if spell == nil {
		return action
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventSpellCast, player, spell, skipCheck)
	return action
}

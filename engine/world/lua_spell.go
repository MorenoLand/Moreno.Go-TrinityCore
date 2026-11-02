package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

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
// stay out: they mutate the C++ Spell state machine (current-spell
// slots, cast timer, finish path), which Go models as functions, not
// as a live object — a snapshot object cannot cancel or finish a cast.
// IsAutoRepeat reads the session auto-repeat slot (Unit::_UpdateAuto-
// RepeatSpell model): true only when this spell id is the active
// auto-repeat spell, matching Spell::m_autoRepeat (Spell.h:533).
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
	isAutoRepeat := s.autoRepeatSpell == spellID && spellID != 0
	methods := map[string]scripting.ObjectMethod{}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Spell" })
	methods["GetEntry"] = luaNoArgs(func() any { return spellID })
	methods["GetCaster"] = luaNoArgs(func() any { return nilSpellObject(s.luaPlayer()) })
	methods["GetCastTime"] = luaNoArgs(func() any { return castTimeMs })
	methods["GetPowerCost"] = luaNoArgs(func() any { return powerCost })
	methods["GetDuration"] = luaNoArgs(func() any { return durationMs })
	methods["IsAutoRepeat"] = luaNoArgs(func() any { return isAutoRepeat })
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
// the fire discards them too: a handler cannot cancel a cast here.
func (s *session) firePlayerSpellCast(ctx context.Context, p spellObjectParams, skipCheck bool) {
	if s == nil || s.player == nil {
		return
	}
	player := s.luaPlayer()
	if player == nil {
		return
	}
	spell := s.luaSpellObject(p)
	if spell == nil {
		return
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventSpellCast, player, spell, skipCheck)
}

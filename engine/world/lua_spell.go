package world

import (
	"context"
	"fmt"
	"strconv"

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
// (Spell.h:533) for the spell whose object this is. GetEffectValue
// (effectIndex) is SpellScript::GetEffectValue (m_spell->damage) via
// SpellEffect.CalcValueForLevel at the caster's level, memoized per
// index, 0 without a DBC row — see the method.
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
	// GetEffectValue(effectIndex) is SpellScript::GetEffectValue
	// (SpellScript.cpp:666-677): m_spell->damage, the per-effect value
	// HandleEffects computes as damage = CalculateDamage(i)
	// (Spell.cpp:5165) before the script-effect handlers run, i.e.
	// SpellEffectInfo::CalcValue (SpellInfo.cpp:402), whose Go twin
	// is SpellEffect.CalcValueForLevel (store.go). C++ returns 0
	// outside effect hooks (IsInEffectHook); the Go object carries no
	// hook context, so ports call this only from OnEffectHit /
	// OnEffectHitTarget (events 4/5) handlers, where the value is
	// live. Memoized per effect index: C++ computes the value once
	// per effect handling, so a second call in the same handler must
	// return the same roll (DieSides). A spell with no DBC row, or an
	// out-of-range index, returns 0; Lua ports guard on a positive
	// value. The custom-base-points leg (m_spellValue) has no Go
	// model: the DBC value is the twin for the standard path.
	casterLevel := uint32(0)
	if s.player != nil {
		casterLevel = uint32(s.player.Level)
	}
	effectValues := map[int]int32{}
	methods["GetEffectValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index >= uint32(len(p.spell.Effects)) {
			return []any{int32(0)}, nil
		}
		if value, ok := effectValues[int(index)]; ok {
			return []any{value}, nil
		}
		value := p.spell.Effects[index].CalcValueForLevel(p.spell, casterLevel)
		effectValues[int(index)] = value
		return []any{value}, nil
	}
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

// luaCreatureSpellObject builds the Eluna Spell object for a creature's
// current spell (Unit::GetCurrentSpell, UnitMethods.h:785-792), the
// server-side twin of the session luaSpellObject: creatures have no
// session, so GetCaster resolves to the creature itself and GetTarget
// resolves the pending/channel target through luaUnitObjectForGUID.
// GetCastTime is the DBC base cast time (the session builder's
// no-active-cast fallback); the live cast-bar value exists only on the
// player path. A spell with no DBC row still reports its entry —
// GetCurrentSpell returns the live Spell* regardless of DBC presence,
// and the DBC-backed reads fall back to 0/-1 like the session builder.
// GetEffectValue is the session builder's twin (same contract).
func (s *Server) luaCreatureSpellObject(m *creatureMotion, spellID uint32, targetGUID uint64) *scripting.Object {
	if s == nil || m == nil || spellID == 0 {
		return nil
	}
	var castTimeMs int32
	durationMs := int32(0)
	spell := wotlk.Spell{}
	if s.Data != nil {
		if row, found, err := s.Data.Spell(spellID); err == nil && found {
			spell = row
			castTimeMs = int32(s.aiSpellCastTimeMs(spell))
			if dur, found, err := s.Data.SpellDurationBase(spell.DurationIndex); err == nil && found {
				durationMs = dur
			} else if !found && spell.Attributes&spellAttributePassive != 0 {
				durationMs = -1
			}
		}
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetObjectType"] = luaNoArgs(func() any { return "Spell" })
	methods["GetEntry"] = luaNoArgs(func() any { return spellID })
	methods["GetCastTime"] = luaNoArgs(func() any { return castTimeMs })
	methods["GetDuration"] = luaNoArgs(func() any { return durationMs })
	// GetEffectValue(effectIndex) is the creature-path twin of the
	// session builder's method (same contract: SpellScript::
	// GetEffectValue, m_spell->damage via CalcValueForLevel at the
	// caster's level, memoized per index, 0 without a DBC row).
	effectValues := map[int]int32{}
	methods["GetEffectValue"] = func(_ context.Context, args []any) ([]any, error) {
		index, err := luaUint32Arg(args, 0)
		if err != nil {
			return nil, err
		}
		if index >= uint32(len(spell.Effects)) {
			return []any{int32(0)}, nil
		}
		if value, ok := effectValues[int(index)]; ok {
			return []any{value}, nil
		}
		value := spell.Effects[index].CalcValueForLevel(spell, m.Level)
		effectValues[int(index)] = value
		return []any{value}, nil
	}
	methods["GetPowerCost"] = luaNoArgs(func() any { return uint32(0) })
	methods["GetCaster"] = luaNoArgs(func() any { return s.luaMotionCreature(m) })
	methods["GetTarget"] = luaNoArgs(func() any {
		if targetGUID == 0 {
			return nil
		}
		return s.luaUnitObjectForGUID(m.Map, m.InstanceID, targetGUID)
	})
	return &scripting.Object{Type: "Spell", Fields: map[string]any{"Entry": spellID}, Methods: methods}
}

// luaCreatureCurrentSpell resolves a creature's current spell slot for
// GetCurrentSpell (UnitMethods.h:785-792): GENERIC is the pending
// twin/inline cast (the twin path stores no target, so its GetTarget
// reads nil), CHANNELED is the live channel stamp, MELEE/AUTOREPEAT
// read nil (no Go slots). A post-GO CastingUntil stamp with no pending
// spell ID is not a current spell — that spell already finished.
// Takes no locks itself; callers hold motionMu on the handler surface.
func (s *Server) luaCreatureCurrentSpell(m *creatureMotion, spellType int) any {
	if s == nil || m == nil {
		return nil
	}
	switch spellType {
	case 1:
		if m.PendingInlineSpellID != 0 {
			return s.luaCreatureSpellObject(m, m.PendingInlineSpellID, m.PendingInlineTargetGUID)
		}
		if m.PendingCastSpellID != 0 {
			return s.luaCreatureSpellObject(m, m.PendingCastSpellID, 0)
		}
		return nil
	case 2:
		if !s.creatureChannelLive(m) {
			return nil
		}
		return s.luaCreatureSpellObject(m, m.ChannelSpellID, m.ChannelTargetGUID)
	default:
		return nil
	}
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

// fireSpellCast dispatches the RegisterSpellEvent OnCast hook for the
// spell (SpellEventOnCast, args (event, spell)). Notification-only:
// C++ OnCast handlers cannot veto the cast. The spell object is the
// caller's Lua Spell object (player path: luaSpellObject, creature
// path: luaCreatureSpellObject); a nil object fires nothing.
func (s *Server) fireSpellCast(ctx context.Context, spellID uint32, spellObj *scripting.Object) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventOnCast, scripting.SpellEventOnCast, spellObj)
}

// fireSpellHit dispatches the RegisterSpellEvent OnHit hook for one hit
// target (SpellEventOnHit, args (event, spell, hitTarget)).
// Notification-only like fireSpellCast: the C++ damage/healing rewrite
// has no Go model. The hitTarget is the Lua unit object or nil.
func (s *Server) fireSpellHit(ctx context.Context, spellID uint32, spellObj *scripting.Object, hitTarget any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventOnHit, scripting.SpellEventOnHit, spellObj, hitTarget)
}

// fireSpellAfterHit dispatches the RegisterSpellEvent AfterHit hook for
// one hit target (SpellEventAfterHit, args (event, spell, hitTarget)).
// Notification-only like fireSpellHit: C++ AfterHit handlers run after
// damage and cannot veto it.
func (s *Server) fireSpellAfterHit(ctx context.Context, spellID uint32, spellObj *scripting.Object, hitTarget any) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventAfterHit, scripting.SpellEventAfterHit, spellObj, hitTarget)
}

// fireSpellEffectHit dispatches the RegisterSpellEvent OnEffectHit hook
// for one effect index (SpellEventOnEffectHit, args (event, spell,
// effectIndex)). Notification-only like fireSpellCast: the C++
// PreventHitDefaultEffect veto has no Go model.
func (s *Server) fireSpellEffectHit(ctx context.Context, spellID uint32, spellObj *scripting.Object, effectIndex int) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventOnEffectHit, scripting.SpellEventOnEffectHit, spellObj, effectIndex)
}

// fireSpellEffectHitTarget dispatches the RegisterSpellEvent
// OnEffectHitTarget hook for one (effect, hit target) pair
// (SpellEventOnEffectHitTarget, args (event, spell, hitTarget,
// effectIndex)). Notification-only like fireSpellEffectHit.
func (s *Server) fireSpellEffectHitTarget(ctx context.Context, spellID uint32, spellObj *scripting.Object, hitTarget any, effectIndex int) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventOnEffectHitTarget, scripting.SpellEventOnEffectHitTarget, spellObj, hitTarget, effectIndex)
}

// fireSpellBeforeHit dispatches the RegisterSpellEvent BeforeHit hook
// for one hit target (SpellEventBeforeHit, args (event, spell,
// hitTarget, missInfo)). Notification-only like fireSpellHit: the
// missInfo is the C++ SpellMissInfo number, 0 on a hit.
func (s *Server) fireSpellBeforeHit(ctx context.Context, spellID uint32, spellObj *scripting.Object, hitTarget any, missInfo uint8) {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil {
		return
	}
	_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventBeforeHit, scripting.SpellEventBeforeHit, spellObj, hitTarget, missInfo)
}

// spellTargetList is the Go side of the SpellTargetList object handed to
// OnObjectAreaTargetSelect handlers: the effect's current target GUIDs,
// mutated in place by the Lua methods (the C++
// std::list<WorldObject*>& model). GUIDs, not objects, are the state —
// the Lua objects are rebuilt per GetTargets call from the live
// motions/sessions, so a stashed object never goes stale for GUID
// purposes, and Remove/Add take unit objects so no GUID ever crosses
// Lua as a float64 number (a 64-bit GUID does not survive that trip).
type spellTargetList struct {
	guids []uint64
}

func (s *Server) spellTargetListObject(list *spellTargetList, resolve func(uint64) any) *scripting.Object {
	methods := map[string]scripting.ObjectMethod{}
	methods["GetCount"] = luaNoArgs(func() any { return len(list.guids) })
	methods["GetTargets"] = func(_ context.Context, _ []any) ([]any, error) {
		objs := make([]any, 0, len(list.guids))
		for _, guid := range list.guids {
			if obj := resolve(guid); obj != nil {
				objs = append(objs, obj)
			}
		}
		return []any{objs}, nil
	}
	methods["Remove"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, nil
		}
		if guid := luaObjectGUID(args[0]); guid != 0 {
			for i, kept := range list.guids {
				if kept == guid {
					list.guids = append(list.guids[:i], list.guids[i+1:]...)
					break
				}
			}
		}
		return nil, nil
	}
	methods["Clear"] = func(_ context.Context, _ []any) ([]any, error) {
		list.guids = list.guids[:0]
		return nil, nil
	}
	methods["Add"] = func(_ context.Context, args []any) ([]any, error) {
		if len(args) == 0 {
			return nil, nil
		}
		if guid := luaObjectGUID(args[0]); guid != 0 {
			for _, kept := range list.guids {
				if kept == guid {
					return nil, nil
				}
			}
			list.guids = append(list.guids, guid)
		}
		return nil, nil
	}
	return &scripting.Object{Type: "SpellTargetList", Fields: map[string]any{}, Methods: methods}
}

// spellEffectTargetHasLeg reports whether either implicit-target slot of
// one effect carries a leg of the given family, applying the
// second-slot skip rule (Spell.cpp:807-830 family) the sweeps use:
// a skipped B slot selects nothing.
func spellEffectTargetHasLeg(eff wotlk.SpellEffect, check func(uint32) bool) bool {
	if check(eff.ImplicitTargetA) {
		return true
	}
	return check(eff.ImplicitTargetB) && !implicitTargetSecondSlotSkipped(eff.ImplicitTargetA, eff.ImplicitTargetB)
}

// spellEffectHasSelectingLeg reports whether one effect selects area
// targets at all — the fire gate for OnObjectAreaTargetSelect. GO-area
// legs (51/52/108) are excluded: the list holds units only, so a
// GO-only effect has no list to filter.
func spellEffectHasSelectingLeg(eff wotlk.SpellEffect) bool {
	if eff.Effect == 0 {
		return false
	}
	if spellEffectTargetHasLeg(eff, isAreaEnemyTargetType) || spellEffectTargetHasLeg(eff, isFriendlyAreaTargetType) {
		return true
	}
	if spellEffectTargetHasLeg(eff, isEntryAreaTargetType) || spellEffectTargetHasLeg(eff, isRefCenteredAreaTargetType) {
		return true
	}
	return effectHasEnemyConeLeg(eff) || effectHasFriendlyConeLeg(eff)
}

// fireSpellTargetSelects dispatches the RegisterSpellEvent
// OnObjectAreaTargetSelect hook for every selecting effect of one
// creature cast, in effect order (C++ fires per effect inside
// SelectImplicitAreaTargets), and returns the per-effect filtered
// lists keyed by effect index — nil when no hook is registered for
// the spell, so callers keep the flat lists untouched. The list each
// handler sees is the effect's per-effect partition with the caster
// removed unless the effect carries a friendly-area leg (enemy,
// entry, ref-centered and cone lists never contain the caster in
// C++; friendly-area lists do). Handlers run in registration order
// on the same mutable list object per effect, so each sees the
// previous handler's edits, like C++ hook chaining on the one list.
func (s *Server) fireSpellTargetSelects(ctx context.Context, m *creatureMotion, spellID uint32, spellObj *scripting.Object, spell wotlk.Spell, hitTargets []uint64, part bossConePartition) map[int][]uint64 {
	if s == nil || s.Features == nil || s.Features.Scripts == nil || spellObj == nil || m == nil {
		return nil
	}
	if !s.Features.Scripts.HasHook("spell:"+strconv.FormatUint(uint64(spellID), 10), scripting.SpellEventOnObjectAreaTargetSelect) {
		return nil
	}
	resolve := func(guid uint64) any { return s.luaUnitObjectForGUID(m.Map, m.InstanceID, guid) }
	var filtered map[int][]uint64
	for i := range spell.Effects {
		eff := spell.Effects[i]
		if !spellEffectHasSelectingLeg(eff) {
			continue
		}
		list := bossConeEffectTargets(eff, hitTargets, part)
		if !spellEffectTargetHasLeg(eff, isFriendlyAreaTargetType) && m.GUID != 0 {
			// Fresh slice: the partition returns hitTargets itself for
			// non-cone effects, so filtering in place would corrupt it.
			kept := make([]uint64, 0, len(list))
			for _, guid := range list {
				if guid != m.GUID {
					kept = append(kept, guid)
				}
			}
			list = kept
		}
		targetList := &spellTargetList{guids: append([]uint64(nil), list...)}
		_, _ = s.Features.Scripts.TriggerSpellEvent(ctx, spellID, scripting.SpellEventOnObjectAreaTargetSelect, scripting.SpellEventOnObjectAreaTargetSelect, spellObj, i, s.spellTargetListObject(targetList, resolve))
		if filtered == nil {
			filtered = make(map[int][]uint64)
		}
		filtered[i] = targetList.guids
	}
	return filtered
}

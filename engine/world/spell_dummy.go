package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Spell::EffectDummy (SpellEffects.cpp:792-824) runs at HIT_TARGET per
// target: the DB-script leg (no Go model), then Eluna's OnDummyEffect —
// the pet-aura leg returns first (player caster with a pet_aura row for
// the spell/effect), so no hook fires for pet-aura spells. C++ fires
// exactly three arms: gameobject target, TYPEID_UNIT target, item target.
// Player targets take no arm (the unit arm gates on TYPEID_UNIT) and
// corpse targets take none either. The session (player-cast) sites —
// finishSpellCast and castSpellDirectWithOverrides — fire with the
// session player as caster; the creature-cast site
// (castCreatureSpellInternalWithBasePoints) fires with the casting
// creature as caster through fireCreatureSpellDummyEffectForTargets
// below. The pet-aura early return is player-caster-only in C++, so no
// pet-aura gate runs on the creature site; the item arm is vacuous there
// (creature casts carry no item target). Returns are discarded on all
// three arms (CallAllFunctions, no retval).

// fireSpellDummyEffect fires one target's dummy-effect hooks for a
// dummy effect the session spell engine just applied. The pet-aura gate
// reads the session's registered source map (populated by
// addOwnerPetAuraSource at the same fire site): a registered source
// means C++ took the AddPetAura early return and fired no hook.
func (s *session) fireSpellDummyEffect(ctx context.Context, spellID uint32, effectIndex uint8, targetGUID uint64) {
	if s == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil || targetGUID == 0 {
		return
	}
	if s.hasOwnerPetAuraSource(spellID, effectIndex) {
		return
	}
	caster := s.luaPlayer()
	switch uint16(targetGUID >> 48) {
	case 0xF130:
		motion := s.findCreatureMotion(targetGUID)
		if motion == nil {
			return
		}
		creature := s.server.luaMotionCreature(motion)
		if creature == nil {
			return
		}
		args := []any{scripting.CreatureEventOnDummyEffect, caster, spellID, int(effectIndex), creature}
		if _, err := s.server.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDummyEffect, args...); err != nil {
			s.debug("lua creature dummy effect failed", "error", err)
		}
		if _, err := s.server.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDummyEffect, args...); err != nil {
			s.debug("lua creature dummy effect failed", "error", err)
		}
	case 0xF110:
		goObj := s.luaGameObject(ctx, targetGUID)
		if goObj == nil {
			goObj = s.server.serverLuaGameObject(ctx, targetGUID)
		}
		if goObj == nil {
			return
		}
		// C++ dummy pushes the target LAST (caster, spellId, effIndex,
		// go), unlike every other GO hook, which pushes it first — so
		// this bypasses fireGameObjectEvent's go-first convention and
		// calls TriggerGameObjectEvent directly (it prepends the event).
		entry := uint32((targetGUID >> 24) & 0x00FFFFFF)
		if _, err := s.server.Features.Scripts.TriggerGameObjectEvent(ctx, entry, scripting.GameObjectEventOnDummyEffect, caster, spellID, int(effectIndex), goObj); err != nil {
			s.debug("lua gameobject dummy effect failed", "error", err)
		}
	case 0x4000:
		item := s.sessionLuaItem(ctx, targetGUID)
		if item == nil {
			return
		}
		entry := objectUint32OrZero(item, "Entry")
		if entry == 0 {
			return
		}
		if _, err := s.server.Features.Scripts.TriggerItemEvent(ctx, entry, scripting.ItemEventOnDummyEffect, caster, spellID, int(effectIndex), item); err != nil {
			s.debug("lua item dummy effect failed", "error", err)
		}
	}
}

// fireSpellDummyEffectForTargets fires the dummy-effect hooks once per
// target per effect over a hit list, in C++ HIT_TARGET order. Hit lists
// can carry a target twice (a wire primary rejoined after an area list),
// and C++ fires once per TargetInfo, so the loop dedupes.
func (s *session) fireSpellDummyEffectForTargets(ctx context.Context, spellID uint32, effectIndex uint8, targetGUIDs []uint64) {
	seen := make(map[uint64]struct{}, len(targetGUIDs))
	for _, guid := range targetGUIDs {
		if guid == 0 {
			continue
		}
		if _, dup := seen[guid]; dup {
			continue
		}
		seen[guid] = struct{}{}
		s.fireSpellDummyEffect(ctx, spellID, effectIndex, guid)
	}
}

// fireCreatureSpellDummyEffect fires one target's dummy-effect hooks
// for a dummy effect the creature-cast engine just applied, mirroring
// the session dispatcher above with the casting creature as caster.
// C++'s pet-aura early return is player-caster-only, so no gate runs
// here; player and corpse targets take no arm, and the item arm is
// vacuous on this path (castCreatureSpellInternal carries no item
// target), so only the creature and gameobject arms fire.
func (s *Server) fireCreatureSpellDummyEffect(ctx context.Context, m *creatureMotion, spellID uint32, effectIndex uint8, targetGUID uint64) {
	if s == nil || m == nil || s.Features == nil || s.Features.Scripts == nil || targetGUID == 0 {
		return
	}
	caster := s.luaMotionCreature(m)
	if caster == nil {
		return
	}
	switch uint16(targetGUID >> 48) {
	case 0xF130:
		motion := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID)
		if motion == nil {
			return
		}
		creature := s.luaMotionCreature(motion)
		if creature == nil {
			return
		}
		args := []any{scripting.CreatureEventOnDummyEffect, caster, spellID, int(effectIndex), creature}
		if _, err := s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDummyEffect, args...); err != nil {
			s.debug("lua creature dummy effect failed", "error", err)
		}
		if _, err := s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDummyEffect, args...); err != nil {
			s.debug("lua creature dummy effect failed", "error", err)
		}
	case 0xF110:
		goObj := s.serverLuaGameObject(ctx, targetGUID)
		if goObj == nil {
			return
		}
		// C++ dummy pushes the target LAST (caster, spellId, effIndex,
		// go), unlike every other GO hook, which pushes it first — so
		// this calls TriggerGameObjectEvent directly, matching the
		// session dispatcher's bypass of fireGameObjectEvent.
		entry := uint32((targetGUID >> 24) & 0x00FFFFFF)
		if _, err := s.Features.Scripts.TriggerGameObjectEvent(ctx, entry, scripting.GameObjectEventOnDummyEffect, caster, spellID, int(effectIndex), goObj); err != nil {
			s.debug("lua gameobject dummy effect failed", "error", err)
		}
	}
}

// fireCreatureSpellDummyEffectForTargets fires the dummy-effect hooks
// once per target per effect over a hit list, applying the creature
// legs' verdict gates: evaded, immune, and missed targets take no arm,
// and a landed reflect retargets onto the caster, following the threat
// leg's convention (a reflect recorded with a miss status skips).
func (s *Server) fireCreatureSpellDummyEffectForTargets(ctx context.Context, m *creatureMotion, spellID uint32, effectIndex uint8, targetGUIDs []uint64, reflected map[uint64]uint8, evaded map[uint64]bool, immune map[uint64]bool, missed map[uint64]uint8) {
	seen := make(map[uint64]struct{}, len(targetGUIDs))
	for _, guid := range targetGUIDs {
		if guid == 0 || evaded[guid] || immune[guid] || missed[guid] != protocol.SpellMissNone {
			continue
		}
		if status, isReflected := reflected[guid]; isReflected {
			if status != protocol.SpellMissNone {
				continue
			}
			guid = m.GUID
		}
		if _, dup := seen[guid]; dup {
			continue
		}
		seen[guid] = struct{}{}
		s.fireCreatureSpellDummyEffect(ctx, m, spellID, effectIndex, guid)
	}
}

// hasOwnerPetAuraSource reports whether a pet-aura source row is
// registered for the spell/effect — the Go mirror of C++'s
// GetPetAura(spellId, effIndex) non-nil arm in EffectDummy.
func (s *session) hasOwnerPetAuraSource(spellID uint32, effectIndex uint8) bool {
	if s == nil {
		return false
	}
	s.ownerPetAuraMu.Lock()
	defer s.ownerPetAuraMu.Unlock()
	_, ok := s.ownerPetAuraSources[ownerPetAuraKey{SpellID: spellID, EffectIndex: effectIndex}]
	return ok
}

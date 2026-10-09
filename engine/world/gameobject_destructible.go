package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// GameObjectDestructibleState mirrors GameObjectDestructibleState
// (GameObject.h:38-43).
const (
	goDestructibleIntact     uint8 = 0
	goDestructibleDamaged    uint8 = 1
	goDestructibleDestroyed  uint8 = 2
	goDestructibleRebuilding uint8 = 3
)

const (
	goFlagDamaged   uint32 = 0x00000200 // GO_FLAG_DAMAGED (SharedDefines.h:1645)
	goFlagDestroyed uint32 = 0x00000400 // GO_FLAG_DESTROYED (SharedDefines.h:1646)
)

// destructibleKey identifies one destructible-building gameobject instance.
// Static DB spawns have no live Go state object, so their runtime health
// lives in Server.destructibleHealth under this key.
type destructibleKey struct {
	Map        uint32
	InstanceID uint32
	LowGUID    uint32
}

// goDestructibleHealth is the per-instance runtime record of a
// destructible-building gameobject's health (GameObject::m_goValue.Building),
// plus the runtime-visible fields the damage/repair/destruction-state spell
// effects mutate (flags, display id, anim progress, destructible state).
type goDestructibleHealth struct {
	Health       uint32
	MaxHealth    uint32
	DamagedHits  uint32 // template building.damagedNumHits (Data5): damaged-state threshold
	DisplayID    uint32
	Flags        uint32
	AnimProgress uint8
	State        uint8 // last-known GameObjectDestructibleState
	GOState      uint8 // g.state, for the bytes1 dword on static GOs
	GOArtKit     uint8 // template_addon artkit0, for the bytes1 dword on static GOs
}

// goDestructibleTemplate carries the gameobject_template columns the
// destructible-building spell effects read (GameObjectData.h:376-400).
type goDestructibleTemplate struct {
	Entry       uint32
	DisplayID   uint32
	IntactHits  uint32 // building.intactNumHits (Data0)
	DamagedHits uint32 // building.damagedNumHits (Data5)
	Faction     uint32 // template_addon faction
}

// destructibleBuildingTemplate fetches the template row for a gameobject
// entry, succeeding only for GAMEOBJECT_TYPE_DESTRUCTIBLE_BUILDING (33).
func (s *Server) destructibleBuildingTemplate(ctx context.Context, entry uint32) (goDestructibleTemplate, bool) {
	var tpl goDestructibleTemplate
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || entry == 0 {
		return tpl, false
	}
	var typ, displayID, intact, damaged int64
	err := s.WorldStore.DB.QueryRowContext(ctx,
		`SELECT t.type, t.displayId, COALESCE(t.Data0, 0), COALESCE(t.Data5, 0)
		FROM gameobject_template AS t WHERE t.entry = ? LIMIT 1`, entry).
		Scan(&typ, &displayID, &intact, &damaged)
	if err != nil || typ != int64(GameObjectTypeDestructibleBuilding) {
		return tpl, false
	}
	var faction int64
	_ = s.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(faction, 0) FROM gameobject_template_addon WHERE entry = ? LIMIT 1`, entry).
		Scan(&faction)
	tpl = goDestructibleTemplate{
		Entry:       entry,
		DisplayID:   uint32(displayID),
		IntactHits:  uint32(intact),
		DamagedHits: uint32(damaged),
		Faction:     uint32(faction),
	}
	return tpl, true
}

// destructibleHealthFor resolves the per-instance health record for a
// destructible-building GO, lazy-initializing it from the template
// (GameObject::Create, GameObject.cpp:357-360: Health = MaxHealth =
// intactNumHits + damagedNumHits, SetGoAnimProgress(255)). It returns false
// when the GO is not a destructible building or carries no health model
// (MaxHealth == 0 — the C++ ModifyHealth early return, GameObject.cpp:2229).
func (s *Server) destructibleHealthFor(ctx context.Context, mapID, instanceID uint32, goGUID uint64) (goDestructibleHealth, goDestructibleTemplate, bool) {
	if s == nil || goGUID == 0 {
		return goDestructibleHealth{}, goDestructibleTemplate{}, false
	}
	var entry uint32
	var goState, goArtKit uint8
	if dyn := s.gameObjectState(mapID, instanceID, goGUID); dyn != nil {
		if dyn.Type != GameObjectTypeDestructibleBuilding {
			return goDestructibleHealth{}, goDestructibleTemplate{}, false
		}
		entry = dyn.Entry
		goState, goArtKit = dyn.State, dyn.ArtKit
	} else {
		entry = uint32(goGUID>>24) & 0xFFFFFF
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			var state, artkit int64
			_ = s.WorldStore.DB.QueryRowContext(ctx,
				`SELECT COALESCE(g.state, 1), COALESCE(ta.artkit0, 0) FROM gameobject AS g
				LEFT JOIN gameobject_template_addon AS ta ON ta.entry = g.id
				WHERE g.guid = ? LIMIT 1`, uint32(goGUID)).
				Scan(&state, &artkit)
			goState, goArtKit = uint8(state), uint8(artkit)
		}
	}
	tpl, ok := s.destructibleBuildingTemplate(ctx, entry)
	if !ok {
		return goDestructibleHealth{}, goDestructibleTemplate{}, false
	}
	maxHealth := tpl.IntactHits + tpl.DamagedHits
	if maxHealth == 0 {
		return goDestructibleHealth{}, goDestructibleTemplate{}, false
	}
	key := destructibleKey{Map: mapID, InstanceID: instanceID, LowGUID: uint32(goGUID)}
	s.objectsMu.Lock()
	rec, found := s.destructibleHealth[key]
	if !found {
		rec = goDestructibleHealth{
			Health:       maxHealth,
			MaxHealth:    maxHealth,
			DamagedHits:  tpl.DamagedHits,
			DisplayID:    tpl.DisplayID,
			AnimProgress: 255,
			State:        goDestructibleIntact,
			GOState:      goState,
			GOArtKit:     goArtKit,
		}
		if s.destructibleHealth == nil {
			s.destructibleHealth = make(map[destructibleKey]goDestructibleHealth)
		}
		s.destructibleHealth[key] = rec
	}
	s.objectsMu.Unlock()
	return rec, tpl, true
}

// storeDestructibleHealth persists a mutated health record.
func (s *Server) storeDestructibleHealth(mapID, instanceID uint32, lowGUID uint32, rec goDestructibleHealth) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	if s.destructibleHealth == nil {
		s.destructibleHealth = make(map[destructibleKey]goDestructibleHealth)
	}
	s.destructibleHealth[destructibleKey{Map: mapID, InstanceID: instanceID, LowGUID: lowGUID}] = rec
	s.objectsMu.Unlock()
}

// destructibleSpawnOverride returns the runtime flags/display/anim-progress
// override for a static destructible-building GO so its create blocks carry
// the damaged/destroyed state (static GOs have no live state object).
func (s *Server) destructibleSpawnOverride(mapID, instanceID uint32, lowGUID uint32) *goDestructibleHealth {
	if s == nil {
		return nil
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	if rec, ok := s.destructibleHealth[destructibleKey{Map: mapID, InstanceID: instanceID, LowGUID: lowGUID}]; ok {
		out := rec
		return &out
	}
	return nil
}

// gameObjectTargetFaction resolves the faction of a spell-targeted GO: the
// live state's faction for dynamic GOs, the template_addon faction for
// static ones.
func (s *Server) gameObjectTargetFaction(ctx context.Context, mapID, instanceID uint32, goGUID uint64) (uint32, bool) {
	if s == nil || goGUID == 0 {
		return 0, false
	}
	if dyn := s.gameObjectState(mapID, instanceID, goGUID); dyn != nil {
		return dyn.Faction, true
	}
	tpl, ok := s.destructibleBuildingTemplate(ctx, uint32(goGUID>>24)&0xFFFFFF)
	if !ok {
		return 0, false
	}
	return tpl.Faction, true
}

// modifyDestructibleHealth mirrors GameObject::ModifyHealth
// (GameObject.cpp:2227-2271): the MaxHealth/zero-change early return, the
// double-destruction guard, the [0, MaxHealth] clamp, SetGoAnimProgress
// (health * 255 / MaxHealth), the SMSG_DESTRUCTIBLE_BUILDING_DAMAGE packet
// to the attacker's player, and the health-driven state transition
// (destroyed / damaged / intact, otherwise the state is unchanged).
func (s *Server) modifyDestructibleHealth(ctx context.Context, mapID, instanceID uint32, goGUID uint64, change int32, attackerGUID uint64, spellID uint32, notify *session) {
	rec, _, ok := s.destructibleHealthFor(ctx, mapID, instanceID, goGUID)
	if !ok {
		return
	}
	if rec.MaxHealth == 0 || change == 0 {
		return
	}
	if change < 0 && rec.Health == 0 {
		// prevent double destructions of the same object
		return
	}
	newHealth := int64(rec.Health) + int64(change)
	switch {
	case newHealth <= 0:
		newHealth = 0
	case newHealth >= int64(rec.MaxHealth):
		newHealth = int64(rec.MaxHealth)
	}
	rec.Health = uint32(newHealth)
	rec.AnimProgress = uint8(uint64(rec.Health) * 255 / uint64(rec.MaxHealth))
	// Health lands before the state transition (C++ sets m_goValue first,
	// then SetDestructibleState(newState, attacker, false)).
	s.storeDestructibleHealth(mapID, instanceID, uint32(goGUID), rec)
	if notify != nil && notify.player != nil {
		buf := protocol.NewBuffer(48)
		buf.WritePackedGUID(goGUID)
		buf.WritePackedGUID(attackerGUID)
		buf.WritePackedGUID(notify.playerGUID)
		buf.WriteU32(uint32(-change))
		buf.WriteU32(spellID)
		_ = notify.write(uint16(protocol.OpcodeSMSG_DESTRUCTIBLE_BUILDING_DAMAGE), buf.Bytes(), true)
	}
	newState := rec.State
	switch {
	case rec.Health == 0:
		newState = goDestructibleDestroyed
	case rec.Health <= rec.DamagedHits:
		newState = goDestructibleDamaged
	case rec.Health == rec.MaxHealth:
		newState = goDestructibleIntact
	}
	if newState == rec.State {
		return
	}
	s.setDestructibleBuildingState(ctx, mapID, instanceID, goGUID, newState, false)
}

// setDestructibleBuildingState mirrors GameObject::SetDestructibleState
// (GameObject.cpp:2274-2362): the flag flips, the display-id swap, and the
// optional health reset for the target state. Documented no-bridge:
// EnableCollision (no Go collision model), EventInform + AI()->Damaged/
// AI()->Destroyed (no Go GO-event/AI model), Battleground::DestroyGate (no
// Go battleground model), and the DestructibleModelData State1/2/3Wmo
// display ids (no Go DBC model — the template displayId stands in).
func (s *Server) setDestructibleBuildingState(ctx context.Context, mapID, instanceID uint32, goGUID uint64, state uint8, setHealth bool) {
	rec, tpl, ok := s.destructibleHealthFor(ctx, mapID, instanceID, goGUID)
	if !ok {
		return
	}
	switch state {
	case goDestructibleIntact:
		rec.Flags &^= goFlagDamaged | goFlagDestroyed
		rec.DisplayID = tpl.DisplayID
		if setHealth {
			rec.Health = rec.MaxHealth
			rec.AnimProgress = 255
		}
	case goDestructibleDamaged:
		rec.Flags &^= goFlagDestroyed
		rec.Flags |= goFlagDamaged
		rec.DisplayID = tpl.DisplayID
		if setHealth {
			rec.Health = rec.DamagedHits
			maxHealth := rec.MaxHealth
			if maxHealth == 0 {
				maxHealth = 1 // C++ guards the same division (GameObject.cpp:2314)
			}
			rec.AnimProgress = uint8(uint64(rec.Health) * 255 / uint64(maxHealth))
		}
	case goDestructibleDestroyed:
		rec.Flags &^= goFlagDamaged
		rec.Flags |= goFlagDestroyed
		rec.DisplayID = tpl.DisplayID
		if setHealth {
			rec.Health = 0
			rec.AnimProgress = 0
		}
	case goDestructibleRebuilding:
		rec.Flags &^= goFlagDamaged | goFlagDestroyed
		rec.DisplayID = tpl.DisplayID
		if setHealth {
			rec.Health = rec.MaxHealth
			rec.AnimProgress = 255
		}
	default:
		return
	}
	rec.State = state
	s.storeDestructibleHealth(mapID, instanceID, uint32(goGUID), rec)
	// A live dynamic state follows the record so its create blocks stay
	// consistent; the values-update broadcast below is the immediate,
	// client-visible path for both dynamic and static GOs.
	var bytes1 uint32
	haveBytes1 := false
	s.objectsMu.Lock()
	if dyn := s.gameObjectStateLocked(mapID, instanceID, goGUID); dyn != nil {
		dyn.Flags = rec.Flags
		dyn.DisplayID = rec.DisplayID
		dyn.AnimProgress = rec.AnimProgress
		bytes1 = uint32(dyn.State) | uint32(dyn.Type)<<8 | uint32(dyn.ArtKit)<<16 | uint32(dyn.AnimProgress)<<24
		haveBytes1 = true
	}
	s.objectsMu.Unlock()
	fields := map[int]uint32{gameObjectFlags: rec.Flags, gameObjectDisplayID: rec.DisplayID}
	if haveBytes1 {
		fields[gameObjectBytes1] = bytes1
	} else {
		fields[gameObjectBytes1] = uint32(rec.GOState) | uint32(GameObjectTypeDestructibleBuilding)<<8 | uint32(rec.GOArtKit)<<16 | uint32(rec.AnimProgress)<<24
	}
	s.broadcastGameObjectValuesUpdateInInstance(mapID, instanceID, goGUID, fields)
}

// spellGameObjectTargetGUID extracts the 0xF110 gameobject target GUID from
// spell target data (the same extraction handleEffectOpenLock and
// handleEffectActivateObject use).
func spellGameObjectTargetGUID(target protocol.SpellTargetData) uint64 {
	if target.Flags&protocol.SpellTargetFlagGameObject != 0 && target.UnitGUID != 0 && uint16(target.UnitGUID>>48) == 0xF110 {
		return target.UnitGUID
	}
	return 0
}

// spellEffectGOTargets mirrors the Spell::GOTargetInfo::DoTargetSpellHit
// iteration (Spell.cpp:2663): HandleEffects runs once per gameobject target
// — the explicit wire target plus every implicit GO target riding hitTargets
// (TARGET_GAMEOBJECT_NEARBY_ENTRY / SRC_AREA / CONE selections land there;
// Go previously had no consumer for them downstream). Deduped on GUID so an
// explicit GO target that also sits in hitTargets is hit once.
func spellEffectGOTargets(target protocol.SpellTargetData, hitTargets []uint64) []uint64 {
	var out []uint64
	seen := make(map[uint64]struct{})
	add := func(guid uint64) {
		if guid == 0 {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		seen[guid] = struct{}{}
		out = append(out, guid)
	}
	add(spellGameObjectTargetGUID(target))
	for _, guid := range hitTargets {
		if uint16(guid>>48) == 0xF110 {
			add(guid)
		}
	}
	return out
}

// handleEffectGameObjectDamageGO is the per-gameobject core of
// handleEffectGameObjectDamage: Spell::EffectGameObjectDamage runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per gameobject target
// (GOTargetInfo::DoTargetSpellHit, Spell.cpp:2663).
func (s *session) handleEffectGameObjectDamageGO(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, goGUID uint64) {
	if s == nil || s.server == nil || s.player == nil || goGUID == 0 {
		return
	}
	mapID, instanceID := s.player.Map, s.player.InstanceID
	// Faction gate (SpellEffects.cpp:5431-5435): damage applies unless the
	// GO's faction resolves and the caster is friendly to it (or the
	// caster's faction template is unknown).
	if faction, ok := s.server.gameObjectTargetFaction(ctx, mapID, instanceID, goGUID); ok && faction != 0 {
		targetTpl, targetOK := s.factionTemplateEntry(faction)
		casterTpl, casterOK := s.factionTemplateEntry(s.server.raceFaction(s.player.Race))
		if targetOK && (!casterOK || factionTemplateFriendlyTo(casterTpl, targetTpl)) {
			return
		}
	}
	damage := eff.BasePoints + 1
	s.server.modifyDestructibleHealth(ctx, mapID, instanceID, goGUID, -damage, s.playerGUID, spellID, s)
}

// handleEffectGameObjectDamage mirrors Spell::EffectGameObjectDamage
// (SpellEffects.cpp:5424), which runs at SPELL_EFFECT_HANDLE_HIT_TARGET per
// gameobject target: the friendly-faction gate (Wintergrasp walls, Ulduar
// storm beacons) then ModifyHealth(-damage, caster, spellId).
func (s *session) handleEffectGameObjectDamage(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, target protocol.SpellTargetData, hitTargets []uint64) {
	for _, goGUID := range spellEffectGOTargets(target, hitTargets) {
		s.handleEffectGameObjectDamageGO(ctx, spellID, eff, goGUID)
	}
}

// handleEffectGameObjectRepair mirrors Spell::EffectGameObjectRepair
// (SpellEffects.cpp:5439), which runs at SPELL_EFFECT_HANDLE_HIT_TARGET per
// gameobject target: ModifyHealth(+damage, caster) with no faction gate.
// handleEffectGameObjectRepairGO is the per-gameobject core of
// handleEffectGameObjectRepair: Spell::EffectGameObjectRepair runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per gameobject target
// (GOTargetInfo::DoTargetSpellHit, Spell.cpp:2663).
func (s *session) handleEffectGameObjectRepairGO(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, goGUID uint64) {
	if s == nil || s.server == nil || s.player == nil || goGUID == 0 {
		return
	}
	damage := eff.BasePoints + 1
	s.server.modifyDestructibleHealth(ctx, s.player.Map, s.player.InstanceID, goGUID, damage, s.playerGUID, spellID, s)
}

// handleEffectGameObjectRepair mirrors Spell::EffectGameObjectRepair
// (SpellEffects.cpp:5439), which runs at SPELL_EFFECT_HANDLE_HIT_TARGET per
// gameobject target: ModifyHealth(+damage, caster), no faction gate.
func (s *session) handleEffectGameObjectRepair(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, target protocol.SpellTargetData, hitTargets []uint64) {
	for _, goGUID := range spellEffectGOTargets(target, hitTargets) {
		s.handleEffectGameObjectRepairGO(ctx, spellID, eff, goGUID)
	}
}

// handleEffectGameObjectSetDestructionState mirrors
// Spell::EffectGameObjectSetDestructionState (SpellEffects.cpp:5450), which
// runs at SPELL_EFFECT_HANDLE_HIT_TARGET per gameobject target:
// SetDestructibleState(MiscValue, caster, true).
// handleEffectGameObjectSetDestructionStateGO is the per-gameobject core of
// handleEffectGameObjectSetDestructionState:
// Spell::EffectGameObjectSetDestructionState runs at
// SPELL_EFFECT_HANDLE_HIT_TARGET per gameobject target
// (GOTargetInfo::DoTargetSpellHit, Spell.cpp:2663).
func (s *session) handleEffectGameObjectSetDestructionStateGO(ctx context.Context, eff wotlk.SpellEffect, goGUID uint64) {
	if s == nil || s.server == nil || s.player == nil || goGUID == 0 {
		return
	}
	state := uint8(eff.MiscValue)
	if state > goDestructibleRebuilding {
		// C++ casts MiscValue to the enum blindly; an out-of-range value
		// hits no switch arm and is a no-op.
		return
	}
	s.server.setDestructibleBuildingState(ctx, s.player.Map, s.player.InstanceID, goGUID, state, true)
}

// handleEffectGameObjectSetDestructionState mirrors
// Spell::EffectGameObjectSetDestructionState (SpellEffects.cpp:5450), which
// runs at SPELL_EFFECT_HANDLE_HIT_TARGET per gameobject target:
// SetDestructibleState(MiscValue, caster, true).
func (s *session) handleEffectGameObjectSetDestructionState(ctx context.Context, eff wotlk.SpellEffect, target protocol.SpellTargetData, hitTargets []uint64) {
	for _, goGUID := range spellEffectGOTargets(target, hitTargets) {
		s.handleEffectGameObjectSetDestructionStateGO(ctx, eff, goGUID)
	}
}

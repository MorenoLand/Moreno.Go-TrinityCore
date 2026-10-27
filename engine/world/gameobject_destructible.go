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

// spellEffectGOTargetKinds classifies an effect's gameobject implicit
// targets (SpellInfo.cpp:216-340 data table) into the two AddGOTarget
// selection families: object targets (23 TARGET_GAMEOBJECT_TARGET, 26
// TARGET_GAMEOBJECT_ITEM_TARGET — DEFAULT category, the explicit wire
// target via SelectImplicitTargetObjectTargets, Spell.cpp:1558) and list
// targets (40 NEARBY_ENTRY, 51 SRC_AREA, 52 DEST_AREA, 108 CONE —
// NEARBY/AREA/CONE categories, whose picks ride hitTargets).
func spellEffectGOTargetKinds(eff wotlk.SpellEffect) (object, list bool) {
	for _, tgt := range [2]uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
		switch tgt {
		case 23, 26:
			object = true
		case 40, 51, 52, 108:
			list = true
		}
	}
	return object, list
}

// spellEffectGOExplicitFallback reports whether an effect with no
// gameobject implicit target still takes the explicit GO target: the
// EFFECT_IMPLICIT_TARGET_EXPLICIT arm (Spell.cpp:1709) adds it when the
// effect's implicit targets leave the GO bit missing
// (GetMissingTargetMask, SpellInfo.cpp:581). Only the GOBJ/GOBJ_ITEM
// used-type effects carry that bit: open-lock 33 and activate-object /
// damage-family 86/87/88/89 (SpellInfo.cpp:651,704-707).
func spellEffectGOExplicitFallback(eff uint32) bool {
	switch eff {
	case spellEffectOpenLock, spellEffectActivateObject, spellEffectGameObjectDamage, spellEffectGameObjectRepair, spellEffectGameObjectSetDestructionState:
		return true
	}
	return false
}

// spellEffectGOTargetsForEffect mirrors the per-effect mask of
// Spell::GOTargetInfo (Spell.cpp:2193, DoProcessTargetContainer :3554):
// DoTargetSpellHit runs for an effect only when the target's EffectMask
// holds that effect's bit, and each selection arm adds only its own
// effects' bits — so an object-target effect fires on the wire GO only,
// a list-target effect on the list picks only, and an effect with no GO
// implicit target on the wire GO via the EXPLICIT fallback. A mixed
// object+list effect takes the union, as C++ runs both arms. This replaces
// the flat list: previously every GO effect fired on every GO in
// hitTargets, so an area damage effect damaged the wire-only door and an
// activate-object effect opened every GO in the area list.
func spellEffectGOTargetsForEffect(target protocol.SpellTargetData, hitTargets []uint64, eff wotlk.SpellEffect) []uint64 {
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
	object, list := spellEffectGOTargetKinds(eff)
	wire := spellGameObjectTargetGUID(target)
	if object || (!object && !list && spellEffectGOExplicitFallback(eff.Effect)) {
		add(wire)
	}
	if list {
		for _, guid := range hitTargets {
			if uint16(guid>>48) == 0xF110 {
				add(guid)
			}
		}
	}
	return out
}

// spellEffectGOTargetMember reports whether a GO belongs to one effect's
// AddGOTarget mask: object-category membership is the wire GO, list
// membership is a GO in the hit list, and a no-GO-target EXPLICIT effect
// claims the wire GO via the fallback.
func spellEffectGOTargetMember(eff wotlk.SpellEffect, goGUID, wire uint64, hitGOs map[uint64]struct{}) bool {
	object, list := spellEffectGOTargetKinds(eff)
	if object {
		return goGUID == wire
	}
	if list {
		_, ok := hitGOs[goGUID]
		return ok
	}
	return goGUID == wire && spellEffectGOExplicitFallback(eff.Effect)
}

// filterGOPacketTargetsByEffectMask mirrors UpdateSpellCastDataTargets
// (Spell.cpp:4512), which writes only m_UniqueGOTargetInfo into
// HitTargets: a GO survives into the packet only if some real effect's
// mask holds it, and a non-destructible GO survives only via a
// non-damage-family effect (AddGOTarget, Spell.cpp:2193, strips
// damage-family bits from its mask). This generalizes the old
// damage-family-only filter to mixed spells: a non-destructible GO
// selected only by damage-family arms drops from the packet even when the
// spell also carries other effects, while a GO claimed by any non-damage
// effect stays. Unit GUIDs pass through, and the wire GO's packet target
// field is untouched. Effect 0 is not a real effect
// (SpellEffectInfo::IsEffect, SpellInfo.cpp:360).
func (s *Server) filterGOPacketTargetsByEffectMask(ctx context.Context, mapID, instanceID uint32, spell wotlk.Spell, target protocol.SpellTargetData, hits []uint64) []uint64 {
	if s == nil {
		return hits
	}
	wire := spellGameObjectTargetGUID(target)
	hitGOs := make(map[uint64]struct{})
	for _, guid := range hits {
		if uint16(guid>>48) == 0xF110 {
			hitGOs[guid] = struct{}{}
		}
	}
	out := make([]uint64, 0, len(hits))
	for _, guid := range hits {
		if guid == 0 || uint16(guid>>48) != 0xF110 {
			out = append(out, guid)
			continue
		}
		keep := false
		for i := range spell.Effects {
			eff := spell.Effects[i]
			if eff.Effect == 0 {
				continue
			}
			if !spellEffectGOTargetMember(eff, guid, wire, hitGOs) {
				continue
			}
			if eff.Effect == spellEffectGameObjectDamage || eff.Effect == spellEffectGameObjectRepair || eff.Effect == spellEffectGameObjectSetDestructionState {
				if s.gameObjectIsDestructibleBuilding(ctx, mapID, instanceID, guid) {
					keep = true
					break
				}
				continue
			}
			keep = true
			break
		}
		if keep {
			out = append(out, guid)
		}
	}
	return out
}

// gameObjectIsDestructibleBuilding is the type-only half of the AddGOTarget
// gate (Spell.cpp:2193): the live dynamic type, else the static template
// type. It deliberately does NOT reuse destructibleHealthFor, which also
// requires a health model — a destructible building with no health record
// still survives the mask strip in C++.
func (s *Server) gameObjectIsDestructibleBuilding(ctx context.Context, mapID, instanceID uint32, goGUID uint64) bool {
	if s == nil || goGUID == 0 {
		return false
	}
	if dyn := s.gameObjectState(mapID, instanceID, goGUID); dyn != nil {
		return dyn.Type == GameObjectTypeDestructibleBuilding
	}
	_, ok := s.destructibleBuildingTemplate(ctx, uint32(goGUID>>24)&0xFFFFFF)
	return ok
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
		casterTpl, casterOK := s.factionTemplateEntry(s.effectiveFactionTemplate(s.player.Race))
		if targetOK && (!casterOK || factionTemplateFriendlyTo(casterTpl, targetTpl)) {
			return
		}
	}
	var dmgSpell wotlk.Spell
	if s.server != nil && s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
			dmgSpell = sp
		}
	}
	damage := eff.CalcValueForLevel(dmgSpell, uint32(s.player.Level))
	s.server.modifyDestructibleHealth(ctx, mapID, instanceID, goGUID, -damage, s.playerGUID, spellID, s)
}

// handleEffectGameObjectDamage mirrors Spell::EffectGameObjectDamage
// (SpellEffects.cpp:5424), which runs at SPELL_EFFECT_HANDLE_HIT_TARGET per
// gameobject target: the friendly-faction gate (Wintergrasp walls, Ulduar
// storm beacons) then ModifyHealth(-damage, caster, spellId).
func (s *session) handleEffectGameObjectDamage(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, target protocol.SpellTargetData, hitTargets []uint64) {
	for _, goGUID := range spellEffectGOTargetsForEffect(target, hitTargets, eff) {
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
	var dmgSpell wotlk.Spell
	if s.server != nil && s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
			dmgSpell = sp
		}
	}
	damage := eff.CalcValueForLevel(dmgSpell, uint32(s.player.Level))
	s.server.modifyDestructibleHealth(ctx, s.player.Map, s.player.InstanceID, goGUID, damage, s.playerGUID, spellID, s)
}

// handleEffectGameObjectRepair mirrors Spell::EffectGameObjectRepair
// (SpellEffects.cpp:5439), which runs at SPELL_EFFECT_HANDLE_HIT_TARGET per
// gameobject target: ModifyHealth(+damage, caster), no faction gate.
func (s *session) handleEffectGameObjectRepair(ctx context.Context, spellID uint32, eff wotlk.SpellEffect, target protocol.SpellTargetData, hitTargets []uint64) {
	for _, goGUID := range spellEffectGOTargetsForEffect(target, hitTargets, eff) {
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
	for _, goGUID := range spellEffectGOTargetsForEffect(target, hitTargets, eff) {
		s.handleEffectGameObjectSetDestructionStateGO(ctx, eff, goGUID)
	}
}

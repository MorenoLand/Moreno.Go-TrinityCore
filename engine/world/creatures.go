package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	// maxVisibilityDistance is the MAX_VISIBILITY_DISTANCE twin
	// (ObjectDefines.h:28 = SIZE_OF_GRIDS, GridDefines.h:38): the range at
	// which a far-visible (WorldObject::SetFarVisible) object stays visible,
	// per WorldObject::GetVisibilityRange (Object.cpp:1515-1522).
	maxVisibilityDistance          = 533.3333
	creatureTypeMask        uint32 = 0x00000009
	creatureUpdateFlags     uint16 = 0x0060
	creatureValuesCount            = 148
	unitVirtualItemSlotID          = 56
	unitFieldMountDisplayID        = 69
	unitFieldBytes1                = 74
	unitFieldDynamicFlags          = 79
	unitFieldNPCFlags              = 82
	unitNPCEmoteState              = 83
	unitFieldBytes2                = 122
)

// visibilityDistanceOverride is the VisibilityDistances table twin
// (ObjectDefines.h:28-37, Object.cpp:58-66): a creature_addon /
// creature_template_addon visibilityDistanceType selects the range at
// which the creature is visible (Creature::LoadCreaturesAddon,
// Creature.cpp:2675-2676). Normal (0) means no override, so the map
// range applies; out-of-range values clamp to Normal, matching the
// ObjectMgr load validation (ObjectMgr.cpp:785-789, 1326-1330).
func visibilityDistanceOverride(typ uint8) (float64, bool) {
	switch typ {
	case 1:
		return 25, true
	case 2:
		return 50, true
	case 3:
		return 200, true
	case 4:
		return 400, true
	case 5:
		return maxVisibilityDistance, true
	default:
		return 0, false
	}
}

type creatureSpawn struct {
	GUID                   uint32
	RawGUID                uint64
	Entry                  uint32
	Map                    uint32
	X                      float32
	Y                      float32
	Z                      float32
	Orientation            float32
	Model                  uint32
	Faction                uint32
	NPCFlags               uint32
	UnitFlags              uint32
	UnitFlags2             uint32
	DynamicFlags           uint32
	CreatedByGUID          uint64
	CreatedBySpell         uint32
	Level                  uint32
	Health                 uint32
	MaxHealth              uint32
	RegenerateHealth       bool
	Mana                   uint32
	Scale                  float32
	HoverHeight            float32
	BoundingRadius         float32
	CombatReach            float32
	WalkSpeed              float32
	RunSpeed               float32
	AttackTime             uint32
	RangedAttack           uint32
	Mount                  uint32
	Bytes1                 uint32
	Bytes2                 uint32
	Emote                  uint32
	VisibilityDistanceType uint8 // creature_addon / creature_template_addon visibilityDistanceType (Creature.cpp:2675-2676); 0 = Normal, no override
	emoteStateKnown        bool  // set by applyCreatureMotionToSpawn when Lua overrode Emote via the motion record (forces the field into the update mask even when clearing to 0)
	standStateKnown        bool  // set by applyCreatureMotionToSpawn when Lua overrode Bytes1 byte 0 via the motion record (forces the field into the update mask even when standing back up to 0)
	mountKnown             bool  // set by applyCreatureMotionToSpawn when Lua overrode Mount via the motion record (forces the field into the update mask even when dismounting to 0)
	sheathStateKnown       bool  // set by applyCreatureMotionToSpawn when Lua overrode Bytes2 byte 0 via the motion record (forces the field into the update mask even when unarming to 0)
	pvpFlagsKnown          bool  // set by applyCreatureMotionToSpawn when Lua overrode Bytes2 byte 1 via the motion record (forces the field into the update mask even when clearing sanctuary to 0)
	ScriptTargetGUID       uint64
	targetKnown            bool // set by applyCreatureMotionToSpawn when Lua overrode the visual target via the motion record (forces the field into the update mask even when clearing to 0)
	Item1                  uint32
	Item2                  uint32
	Item3                  uint32
	TransportGUID          uint64
	TransportX             float32
	TransportY             float32
	TransportZ             float32
	TransportO             float32
	FlagsExtra             uint32
	TypeFlags              uint32
	PhaseMask              uint32 // live phase for the create fan-out gate (Object.cpp:1659-1662): DB c.phaseMask for respawns, summoner phase for summons/totems/pets; 0 = unset, read as PHASEMASK_NORMAL (1) by the helper
}

func creatureSpawnHealth(regenerate bool, current, maximum uint32) uint32 {
	if regenerate {
		return maximum
	}
	return current
}

// applyGameEventModelEquip applies one game_event_model_equip override
// to a sweep spawn (GameEventMgr::ChangeEquipOrModel,
// GameEventMgr.cpp:1422-1479): modelid replaces the display only when
// positive (the C++ apply guard), equipment_id 0 clears the virtual
// item slots (Creature::LoadEquipment(0), Creature.cpp:1798-1807) and a
// positive id swaps in the resolved template items.
func applyGameEventModelEquip(spawn *creatureSpawn, over gameEventModelEquipOverride) {
	if spawn == nil {
		return
	}
	if over.modelID > 0 {
		spawn.Model = over.modelID
	}
	if over.equipmentID == 0 {
		spawn.Item1, spawn.Item2, spawn.Item3 = 0, 0, 0
	} else {
		spawn.Item1, spawn.Item2, spawn.Item3 = over.item1, over.item2, over.item3
	}
}

func (s *Server) buildNearbyCreatureUpdates(ctx context.Context, state playerState, phaseMask uint32, observer *session) (*protocol.Packet, int, error) {
	distance := s.visibilityRangeForMap(state.Map)
	if distance <= 0 {
		return nil, 0, nil
	}
	// GetVisibilityRange twin (Object.cpp:1515-1522): when any far-visible
	// creature exists on this map/instance the bounding-box prefilter widens
	// to MAX_VISIBILITY_DISTANCE; each row is range-checked against its own
	// effective range below.
	boxDist := distance
	if s.anyFarVisibleCreature(state.Map, state.InstanceID) {
		boxDist = maxVisibilityDistance
	}
	// The DB override widens the box too: a Large (200) / Gigantic (400) /
	// Infinite (533.3333) row beyond the map range never reaches the
	// per-row GetVisibilityRange check otherwise (Tiny/Small sit inside the
	// map range, so only the three wide overrides need the widening).
	// The per-map max is cached (addon rows are static for the process);
	// the box takes the widest of the map range, the far-visible MAX, and
	// the widest DB override on the map.
	if maxOverride := s.maxVisibilityDistanceOverrideForMap(ctx, state.Map); maxOverride > boxDist {
		boxDist = maxOverride
	}
	isGM := state.ExtraFlags&playerExtraGMOn != 0 || state.PlayerFlags&playerFlagGM != 0
	// seesHidden is the SERVERSIDE_VISIBILITY_GM detect arm of
	// WorldObject::CanSeeOrDetect (Object.cpp:1622-1630), distinct from the
	// GM_ON phase-anywhere arm above: Player::SetGMVisible(true) resets the
	// detect to SEC_PLAYER while GM_ON stays set, so a GM who just ran
	// `.gm visible on` stops seeing serverside-hidden creatures (spirit
	// healers/guides, GHOST_VISIBILITY creatures) even though the phase
	// bypass still applies (Player.cpp:2504-2523).
	seesHidden := observer != nil && observer.gmVisibilityDetect != 0
	isGhost := state.Health > 0 && state.PlayerFlags&playerFlagGhost != 0
	// Runtime 261-phase overrides (AuraEffect::HandlePhase creature arm,
	// SpellAuraEffects.cpp:1581-1632): the query only sees the spawn's
	// static c.phaseMask row, so spawn GUIDs whose override intersects the
	// observer's phase mask are OR'd into the phase predicate.
	phaseClause, phaseOverrideArgs := creaturePhasePredicate("? OR (c.phaseMask & ?) <> 0", s.creaturePhaseOverrideGUIDs(state.Map, phaseMask))
	// Event creatures spawn only while their event runs; game_event_npcflag
	// flags OR into the template npcflag during events (guards gaining
	// seasonal gossip/questgiver flags).
	var selectArgs []any
	npcFlagExpr := "0"
	if flagClause, ok := s.gameEventNPCFlagClause(ctx, &selectArgs); ok {
		npcFlagExpr = flagClause
	}
	activeEvents := s.activeEventList(ctx)
	var eventArgs []any
	eventClause := gameEventSpawnClause("gec.eventEntry", activeEvents, &eventArgs)
	modelEquipOverrides := s.gameEventModelEquipOverrides(ctx, activeEvents)
	// IsSpiritService (Unit.h:1021) tests the live UNIT_NPC_FLAGS
	// (template | spawn npcflag override), so the ghost-visibility predicate
	// uses COALESCE(NULLIF(c.npcflag,0),t.npcflag,0), not the template
	// column alone.
	fullQuery := `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.orientation,
		COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), NULLIF(t.modelid2, 0), NULLIF(t.modelid3, 0), NULLIF(t.modelid4, 0), 1),
		t.faction, (t.npcflag | ` + npcFlagExpr + `), t.unit_flags, t.dynamicflags,
		t.maxlevel, c.curhealth, t.RegenHealth, c.curmana, t.scale, t.HoverHeight, t.speed_walk, t.speed_run, t.BaseAttackTime, t.RangeAttackTime,
		CASE WHEN ca.guid IS NOT NULL THEN COALESCE(ca.mount, 0) ELSE COALESCE(cta.mount, 0) END,
		CASE WHEN ca.guid IS NOT NULL THEN COALESCE(ca.bytes1, 0) ELSE COALESCE(cta.bytes1, 0) END,
		CASE WHEN ca.guid IS NOT NULL THEN COALESCE(ca.bytes2, 0) ELSE COALESCE(cta.bytes2, 0) END,
		CASE WHEN ca.guid IS NOT NULL THEN COALESCE(ca.emote, 0) ELSE COALESCE(cta.emote, 0) END,
		CASE WHEN ca.guid IS NOT NULL THEN COALESCE(ca.visibilityDistanceType, 0) ELSE COALESCE(cta.visibilityDistanceType, 0) END,
		COALESCE(eq.ItemID1, 0),
		COALESCE(eq.ItemID2, 0),
		COALESCE(eq.ItemID3, 0),
		COALESCE(t.flags_extra, 0), COALESCE(t.type_flags, 0)
		FROM creature AS c
		JOIN creature_template AS t ON t.entry = c.id
		LEFT JOIN creature_addon AS ca ON ca.guid = c.guid
		LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id
		LEFT JOIN creature_equip_template AS eq ON eq.CreatureID = c.id AND eq.ID = COALESCE(NULLIF(c.equipment_id, 0), 1)
		LEFT JOIN game_event_creature AS gec ON gec.guid = c.guid
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (` + phaseClause + `)
		AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(NULLIF(c.npcflag, 0), t.npcflag, 0) & 0xC000) = 0))
		AND ` + eventClause + `
		ORDER BY c.guid`
	queryArgs := make([]any, 0, len(selectArgs)+9+len(eventArgs)+len(phaseOverrideArgs))
	queryArgs = append(queryArgs, selectArgs...)
	queryArgs = append(queryArgs, state.Map, float64(state.X)-boxDist, float64(state.X)+boxDist, float64(state.Y)-boxDist, float64(state.Y)+boxDist, isGM, phaseMask)
	queryArgs = append(queryArgs, phaseOverrideArgs...)
	queryArgs = append(queryArgs, seesHidden, isGhost)
	queryArgs = append(queryArgs, eventArgs...)
	rows, err := s.WorldStore.DB.QueryContext(ctx, fullQuery, queryArgs...)
	if err != nil {
		fallbackQuery := `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.orientation,
			COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), 1),
			t.faction, t.npcflag, t.unit_flags, t.dynamicflags,
			t.maxlevel, c.curhealth, t.RegenHealth, c.curmana, t.scale, t.speed_walk, t.speed_run, t.BaseAttackTime, t.RangeAttackTime,
			COALESCE(t.flags_extra, 0), COALESCE(t.type_flags, 0)
			FROM creature AS c
			JOIN creature_template AS t ON t.entry = c.id
			WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
			AND (` + phaseClause + `)
			AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(NULLIF(c.npcflag, 0), t.npcflag, 0) & 0xC000) = 0))
			ORDER BY c.guid`
		fallbackArgs := []any{state.Map, float64(state.X) - boxDist, float64(state.X) + boxDist, float64(state.Y) - boxDist, float64(state.Y) + boxDist, isGM, phaseMask}
		fallbackArgs = append(fallbackArgs, phaseOverrideArgs...)
		fallbackArgs = append(fallbackArgs, seesHidden, isGhost)
		rows, err = s.WorldStore.DB.QueryContext(ctx, fallbackQuery, fallbackArgs...)
		if err != nil {
			if missingTable(err) {
				return nil, 0, nil
			}
			return nil, 0, err
		}
		spawns := make([]creatureSpawn, 0)
		count := 0
		for rows.Next() {
			var guid, entry, mapID, model, faction, npcFlags, unitFlags, dynamicFlags, level, health, regenerateHealth, mana, attackTime, rangedAttack, flagsExtra, typeFlags int64
			var x, y, z, orientation, scale, walkSpeed, runSpeed float64
			if err := rows.Scan(&guid, &entry, &mapID, &x, &y, &z, &orientation, &model, &faction, &npcFlags, &unitFlags, &dynamicFlags, &level, &health, &regenerateHealth, &mana, &scale, &walkSpeed, &runSpeed, &attackTime, &rangedAttack, &flagsExtra, &typeFlags); err != nil {
				return nil, count, err
			}
			visRange := distance
			if s.creatureScriptFarVisible(state.Map, state.InstanceID, uint32(guid), uint32(entry)) {
				visRange = maxVisibilityDistance
			}
			if math.Hypot(x-float64(state.X), y-float64(state.Y)) > visRange || !validMovementPosition(float32(x), float32(y), float32(z), float32(orientation)) {
				continue
			}
			spawn := creatureSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), Model: uint32(model), Faction: uint32(faction), NPCFlags: uint32(npcFlags), UnitFlags: uint32(unitFlags), DynamicFlags: uint32(dynamicFlags), Level: uint32(level), Health: uint32(health), RegenerateHealth: regenerateHealth != 0, Mana: uint32(mana), Scale: float32(scale), HoverHeight: 1, WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: uint32(attackTime), RangedAttack: uint32(rangedAttack), FlagsExtra: uint32(flagsExtra), TypeFlags: uint32(typeFlags)}
			if over, ok := modelEquipOverrides[spawn.GUID]; ok {
				applyGameEventModelEquip(&spawn, over)
			}
			spawns = append(spawns, spawn)
			count++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, count, err
		}
		transportSpawns := filterNewTransportCreaturePassengers(observer, s.nearbyTransportCreaturePassengers(state, distance))
		spawns = append(spawns, transportSpawns...)
		count += len(transportSpawns)
		if count == 0 {
			return nil, 0, nil
		}
		updates := protocol.NewUpdateData()
		for index := range spawns {
			stats := s.loadCreatureStats(ctx, spawns[index].Entry)
			spawns[index].BoundingRadius, spawns[index].CombatReach, spawns[index].MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
			spawns[index].Health = creatureSpawnHealth(spawns[index].RegenerateHealth, spawns[index].Health, stats.MaxHealth)
			s.applyCreatureMotionToSpawn(state, &spawns[index])
			if s.creatureCorpseRemoved(spawns[index].GUID, state.Map, state.InstanceID) {
				count--
				continue
			}
			if s.spawnGroupHiddenFromSweep(ctx, state.Map, state.InstanceID, spawnTypeCreature, spawns[index].GUID) {
				count--
				continue
			}
			if !seesHidden && s.creatureScriptHidden(state.Map, state.InstanceID, spawns[index].GUID, spawns[index].Entry) {
				count--
				continue
			}
			if isGhost && !seesHidden && !s.creatureVisibleToGhostSeer(state, &spawns[index]) {
				count--
				continue
			}
			updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawns[index], observer))
		}
		packet, err := updates.BuildPacket(0)
		return packet, count, err
	}
	// A guid with two passing game_event_creature rows joins twice; the
	// grid holds it once (GameEventSpawn is idempotent per guid), so the
	// second row is dropped here.
	seenGUIDs := make(map[uint32]struct{})
	spawns := make([]creatureSpawn, 0)
	count := 0
	for rows.Next() {
		var guid, entry, mapID, model, faction, npcFlags, unitFlags, dynamicFlags, level, health, regenerateHealth, mana, attackTime, rangedAttack, mount, bytes1, bytes2, emote, visDistType, item1, item2, item3, flagsExtra, typeFlags int64
		var x, y, z, orientation, scale, hoverHeight, walkSpeed, runSpeed float64
		if err := rows.Scan(&guid, &entry, &mapID, &x, &y, &z, &orientation, &model, &faction, &npcFlags, &unitFlags, &dynamicFlags, &level, &health, &regenerateHealth, &mana, &scale, &hoverHeight, &walkSpeed, &runSpeed, &attackTime, &rangedAttack, &mount, &bytes1, &bytes2, &emote, &visDistType, &item1, &item2, &item3, &flagsExtra, &typeFlags); err != nil {
			return nil, count, err
		}
		if _, dup := seenGUIDs[uint32(guid)]; dup {
			continue
		}
		seenGUIDs[uint32(guid)] = struct{}{}
		visRange := distance
		if override, ok := visibilityDistanceOverride(uint8(visDistType)); ok {
			visRange = override
		} else if s.creatureScriptFarVisible(state.Map, state.InstanceID, uint32(guid), uint32(entry)) {
			visRange = maxVisibilityDistance
		}
		if math.Hypot(x-float64(state.X), y-float64(state.Y)) > visRange || !validMovementPosition(float32(x), float32(y), float32(z), float32(orientation)) {
			continue
		}
		spawn := creatureSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), Model: uint32(model), Faction: uint32(faction), NPCFlags: uint32(npcFlags), UnitFlags: uint32(unitFlags), DynamicFlags: uint32(dynamicFlags), Level: uint32(level), Health: uint32(health), RegenerateHealth: regenerateHealth != 0, Mana: uint32(mana), Scale: float32(scale), HoverHeight: float32(hoverHeight), WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: uint32(attackTime), RangedAttack: uint32(rangedAttack), Mount: uint32(mount), Bytes1: uint32(bytes1), Bytes2: uint32(bytes2), Emote: uint32(emote), VisibilityDistanceType: uint8(visDistType), Item1: uint32(item1), Item2: uint32(item2), Item3: uint32(item3), FlagsExtra: uint32(flagsExtra), TypeFlags: uint32(typeFlags)}
		if over, ok := modelEquipOverrides[spawn.GUID]; ok {
			applyGameEventModelEquip(&spawn, over)
		}
		spawns = append(spawns, spawn)
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, count, err
	}
	transportSpawns := filterNewTransportCreaturePassengers(observer, s.nearbyTransportCreaturePassengers(state, distance))
	spawns = append(spawns, transportSpawns...)
	count += len(transportSpawns)
	if count == 0 {
		return nil, 0, nil
	}
	updates := protocol.NewUpdateData()
	for index := range spawns {
		stats := s.loadCreatureStats(ctx, spawns[index].Entry)
		spawns[index].BoundingRadius, spawns[index].CombatReach, spawns[index].MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
		spawns[index].Health = creatureSpawnHealth(spawns[index].RegenerateHealth, spawns[index].Health, stats.MaxHealth)
		s.applyCreatureMotionToSpawn(state, &spawns[index])
		if s.creatureCorpseRemoved(spawns[index].GUID, state.Map, state.InstanceID) {
			count--
			continue
		}
		if s.spawnGroupHiddenFromSweep(ctx, state.Map, state.InstanceID, spawnTypeCreature, spawns[index].GUID) {
			count--
			continue
		}
		if !seesHidden && s.creatureScriptHidden(state.Map, state.InstanceID, spawns[index].GUID, spawns[index].Entry) {
			count--
			continue
		}
		if isGhost && !seesHidden && !s.creatureVisibleToGhostSeer(state, &spawns[index]) {
			count--
			continue
		}
		updates.AddUpdateBlock(buildCreatureUpdate(ctx, spawns[index], observer))
	}
	packet, err := updates.BuildPacket(0)
	return packet, count, err
}

// creatureVisibleToGhostSeer judges one nearby-creature row for a ghost
// seer (Object.cpp:1584-1601 corpse leg, 1632-1647 ghost leg). Ghost-visible
// classes pass outright (Creature.cpp:1142, 1174-1177): spirit healers/guides
// (live npcflag 0xC000), flags_extra CREATURE_FLAG_EXTRA_GHOST_VISIBILITY
// (0x400), and type_flags CREATURE_TYPE_FLAG_GHOST_VISIBLE (0x2, the
// ALIVE|GHOST class). An ALIVE-only row reaches the ghost only through the
// corpse leg: the seer's corpse within sight range of both the seer and the
// creature (2D, IsWithinDist with is3D=false). The sight-range skip C++ pairs
// with the leg (corpseCheck bypassing the viewpoint distance check) is NOT
// ported, as on the player path: the row already passed the seer-target
// distance gate, and dropping it would send creatures up to twice the range.
func (s *Server) creatureVisibleToGhostSeer(state playerState, spawn *creatureSpawn) bool {
	if spawn == nil {
		return false
	}
	if spawn.FlagsExtra&0x400 != 0 || spawn.NPCFlags&0xC000 != 0 || spawn.TypeFlags&0x2 != 0 {
		return true
	}
	if !state.hasCorpse {
		return false
	}
	sightRange := s.visibilityRangeForMap(state.Map)
	if override, ok := visibilityDistanceOverride(spawn.VisibilityDistanceType); ok {
		sightRange = override
	} else if s.creatureScriptFarVisible(state.Map, state.InstanceID, spawn.GUID, spawn.Entry) {
		sightRange = maxVisibilityDistance
	}
	if sightRange <= 0 {
		return false
	}
	return math.Hypot(float64(state.corpseX-state.X), float64(state.corpseY-state.Y)) <= sightRange &&
		math.Hypot(float64(state.corpseX-spawn.X), float64(state.corpseY-spawn.Y)) <= sightRange
}

// creatureScriptHidden reports the Unit::SetVisible script override
// (motion.ScriptHidden, Unit.cpp:8656): a hidden creature is visible to GM
// seers only (SERVERSIDE_VISIBILITY_GM, SEC_GAMEMASTER).
func (s *Server) creatureScriptHidden(mapID, instanceID uint32, guid, entry uint32) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.findCreatureMotionLocked(mapID, instanceID, creatureWorldGUID(guid, entry)); motion != nil {
		return motion.ScriptHidden
	}
	return false
}

// creatureScriptFarVisible reports the WorldObject::SetFarVisible script
// override (motion.FarVisible, Object.cpp:1049-1055).
func (s *Server) creatureScriptFarVisible(mapID, instanceID uint32, guid, entry uint32) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.findCreatureMotionLocked(mapID, instanceID, creatureWorldGUID(guid, entry)); motion != nil {
		return motion.FarVisible
	}
	return false
}

// anyFarVisibleCreature reports whether any creature motion on the given
// map/instance currently carries the far-visible script override. Used to
// widen the nearby-creature bounding-box prefilter to
// MAX_VISIBILITY_DISTANCE (the WorldObject::GetVisibilityRange twin,
// Object.cpp:1515-1522).
func (s *Server) anyFarVisibleCreature(mapID, instanceID uint32) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, motion := range s.motionMapLocked(mapID, instanceID) {
		if motion != nil && motion.FarVisible {
			return true
		}
	}
	return false
}

// maxVisibilityDistanceOverrideForMap returns the widest creature_addon /
// creature_template_addon visibilityDistanceType override among creature
// rows on the map, mapped through the VisibilityDistances table
// (ObjectDefines.h:28-37, Object.cpp:58-66; Creature.cpp:2675-2676), or 0
// when no row carries one. The guid addon wins over the template addon
// per row, matching the whole-row CASE in buildNearbyCreatureUpdates
// (Creature::GetCreatureAddon precedence, Creature.cpp:2611-2621).
// Cached per map: addon rows are static for the life of the process (no
// runtime mutation path), and the sweep runs on every player move. A
// query failure (missing addon tables on a minimal DB) caches 0, matching
// the fallback sweep's no-override per-row behavior.
func (s *Server) maxVisibilityDistanceOverrideForMap(ctx context.Context, mapID uint32) float64 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0
	}
	s.visOverrideMu.Lock()
	defer s.visOverrideMu.Unlock()
	if s.visOverrideMax == nil {
		s.visOverrideMax = make(map[uint32]float64)
	}
	if max, ok := s.visOverrideMax[mapID]; ok {
		return max
	}
	var maxType int64
	err := s.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(COALESCE(ca.visibilityDistanceType, cta.visibilityDistanceType, 0)), 0)
		FROM creature AS c
		LEFT JOIN creature_addon AS ca ON ca.guid = c.guid
		LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id
		WHERE c.map = ?`, mapID).Scan(&maxType)
	max := 0.0
	if err == nil {
		if override, ok := visibilityDistanceOverride(uint8(maxType)); ok {
			max = override
		}
	}
	s.visOverrideMax[mapID] = max
	return max
}

func (s *Server) applyCreatureMotionToSpawn(state playerState, spawn *creatureSpawn) {
	if s == nil || spawn == nil {
		return
	}
	// The sweep/transport queries carry the raw addon bytes2, but C++
	// LoadCreaturesAddon (Creature.cpp:2656-2668) applies only byte 0
	// (sheath), never sets byte 1, and forces bytes 2/3 to 0 — strip
	// bytes 1-3 here so no create block sends them. The live sheath /
	// PVP overlays below re-apply bytes 0/1 afterwards. Placed before
	// the transport early return so passengers are covered too.
	spawn.Bytes2 &= 0xFF
	if spawn.TransportGUID != 0 {
		// Transport passengers keep frame-computed positions (the tick
		// relocates them from the current frame, fresher than the motion
		// record), but health/flags/faction must still come from the live
		// motion: the streaming paths otherwise build create blocks from
		// static spawn health, so a killed passenger reappears alive to
		// every late observer. C++ keeps the corpse in _staticPassengers
		// until RemoveCorpse (Transport.cpp RemovePassenger, Object.cpp
		// removal arm), sliding at health 0; buildCreatureUpdate forces
		// the lootable flag when Health == 0.
		if motion := s.findCreatureMotion(state.Map, state.InstanceID, creatureWorldGUID(spawn.GUID, spawn.Entry)); motion != nil {
			spawn.Health, spawn.UnitFlags, spawn.DynamicFlags, spawn.Faction = motion.Health, motion.UnitFlags, motion.DynamicFlags, motion.Faction
			spawn.CreatedByGUID = motion.CreatedByGUID
		}
		return
	}
	if motion := s.findCreatureMotion(state.Map, state.InstanceID, creatureWorldGUID(spawn.GUID, spawn.Entry)); motion != nil {
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation = motion.X, motion.Y, motion.Z, motion.Orientation
		spawn.Health, spawn.UnitFlags, spawn.DynamicFlags, spawn.Faction = motion.Health, motion.UnitFlags, motion.DynamicFlags, motion.Faction
		spawn.CreatedByGUID = motion.CreatedByGUID
		if motion.EntryOverride && motion.Entry != spawn.Entry {
			// A scripted UpdateEntry transform re-seated the motion on a
			// new template (Creature::UpdateEntry, Creature.cpp:552): the
			// create block must carry the NEW entry so late-joining
			// clients resolve the name/model from the right template,
			// while the raw GUID keeps the original entry embedded —
			// C++ fixes the GUID at Object::Create and only SetEntry
			// changes (InitEntry, Creature.cpp:486). Level/health/attack
			// time/scale come from the new template's seated values
			// (loadCreatureStats above used the DB row's entry).
			spawn.Entry = motion.Entry
			spawn.RawGUID = motion.GUID
			spawn.Level = motion.Level
			spawn.MaxHealth = motion.MaxHealth
			spawn.AttackTime = motion.AttackTime
			spawn.Scale = motion.BaseScale
			spawn.CombatReach = motion.CombatReach
		}
		if motion.NPCFlagsKnown {
			// A Lua arm overrode the live NPC flags (SetNPCFlags /
			// SetFlag(82) / RemoveFlag(82)): the spawn must carry the live
			// value, not the DB-template row, or the next rebuild would
			// clobber it. Untouched creatures keep the DB/game-event value.
			spawn.NPCFlags = motion.NPCFlags
		}
		if motion.EmoteStateKnown {
			spawn.Emote = motion.EmoteState
			spawn.emoteStateKnown = true
		}
		if motion.bytes1CacheKnown {
			// The seeded template bytes1 carries bytes 2/3 (vis flag /
			// anim tier) the stand-state arm below would drop; byte 0
			// stays the live stand state when a script set one. For DB
			// spawns this is identity with the sweep query's whole-row
			// bytes1; for summons it is the only path those bytes take.
			spawn.Bytes1 = motion.bytes1Cache
			if motion.StandStateKnown {
				spawn.Bytes1 = (spawn.Bytes1 & 0xFFFFFF00) | uint32(motion.StandState)
			}
			spawn.standStateKnown = true
		} else if motion.StandStateKnown {
			spawn.Bytes1 = (spawn.Bytes1 & 0xFFFFFF00) | uint32(motion.StandState)
			spawn.standStateKnown = true
		}
		if motion.SheathStateKnown {
			spawn.Bytes2 = (spawn.Bytes2 & 0xFFFFFF00) | uint32(motion.SheathState)
			spawn.sheathStateKnown = true
		}
		if motion.PVPFlagsKnown {
			spawn.Bytes2 = (spawn.Bytes2 & 0xFFFF00FF) | uint32(motion.PVPFlags)<<8
			spawn.pvpFlagsKnown = true
		}
		if motion.ScriptTargetKnown {
			// A Lua arm overrode the visual target (SetTarget): the spawn
			// must carry the live value, not the default empty field, or
			// the next rebuild would resurrect a stale stare target (or
			// drop a live one).
			spawn.ScriptTargetGUID = motion.ScriptTargetGUID
			spawn.targetKnown = true
		}
		if motion.MountKnown {
			spawn.Mount = motion.Mount
			spawn.mountKnown = true
		}
		if motion.DisplayIDKnown {
			// A Lua arm overrode the live display model (SetDisplayId): the
			// spawn must carry the live value, not the DB-template row, or
			// the next rebuild would clobber it (the EmoteState/StandState
			// precedent).
			spawn.Model = motion.DisplayID
		}
		if motion.ScriptScaleKnown {
			// A Lua arm overrode the live object scale (SetScale): the
			// spawn must carry the live value, not the template row, or
			// the RecalculateObjectScale twin below would clobber it (and
			// the aura fold applies on top of the script base).
			spawn.Scale = motion.ScriptScale
		}
	}
}

func filterNewTransportCreaturePassengers(observer *session, spawns []creatureSpawn) []creatureSpawn {
	if observer == nil {
		return spawns
	}
	visible := make([]creatureSpawn, 0, len(spawns))
	for _, spawn := range spawns {
		guid := spawn.RawGUID
		if guid == 0 {
			guid = creatureWorldGUID(spawn.GUID, spawn.Entry)
		}
		if !observer.markTransportPassengerVisible(guid, spawn.TransportGUID) {
			continue
		}
		// Grid-stream phase leg (Object.cpp:1659-1662): DB rows are
		// SQL-gated by the phase predicate, but transport passengers
		// are appended unchecked. A passenger whose stamped phase
		// (loadTransportPassengers c.phaseMask, 0 read as
		// PHASEMASK_NORMAL) misses the observer's phase is dropped
		// after marking, like the ghost filter in
		// buildNearbyCreatureUpdates below; the transport update tick
		// reconciles the mark (clear + out-of-range, then re-mark and
		// create when the gate passes).
		passengerPhase := spawn.PhaseMask
		if passengerPhase == 0 {
			passengerPhase = 1
		}
		if passengerPhase&observer.currentPlayerPhaseMask() == 0 {
			continue
		}
		visible = append(visible, spawn)
	}
	return visible
}

// canSeeSpellClickOnCreature mirrors Player::CanSeeSpellClickOn (Player.cpp:25749-25762):
// the UNIT_NPC_FLAG_SPELLCLICK bit is only sent to a player when the creature
// carries the flag, at least one npc_spellclick_spells row exists for its
// entry, and every row fits SpellClickInfo::IsFitToRequirements
// (ObjectMgr.cpp:194) — the userType gate plus the ConditionMgr spell-click
// conditions. C++ filters userType >= SPELL_CLICK_USER_MAX (4) at DB load; Go
// filters at use, like handleSpellClick. The userType semantics follow
// handleSpellClick's documented adaptation (FRIEND = not hostile; RAID/PARTY
// reject for creatures, which are never in groups).
func (s *session) canSeeSpellClickOnCreature(ctx context.Context, clickEntry uint32, creatureFaction uint32, creatureGUID uint64) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_id, user_type FROM npc_spellclick_spells WHERE npc_entry = ?", clickEntry)
	if err != nil {
		return false
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var spellID, userType uint32
		if err := rows.Scan(&spellID, &userType); err != nil || spellID == 0 || userType >= 4 {
			continue
		}
		count++
		fits := true
		switch userType {
		case 1: // SPELL_CLICK_USER_FRIEND
			player := playerPos{Race: s.player.Race, Class: s.player.Class, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
			if s.server.isHostileFaction(creatureFaction, player) {
				fits = false
			}
		case 2, 3: // SPELL_CLICK_USER_RAID / SPELL_CLICK_USER_PARTY
			fits = false
		}
		if fits {
			if meets, err := s.meetSpellClickConditions(ctx, clickEntry, spellID, creatureGUID); err != nil || !meets {
				fits = false
			}
		}
		if !fits {
			return false
		}
	}
	return count > 0
}

func buildCreatureUpdate(ctx context.Context, spawn creatureSpawn, observer *session) []byte {
	values := make([]uint32, creatureValuesCount)
	rawGUID := spawn.RawGUID
	if rawGUID == 0 {
		rawGUID = creatureWorldGUID(spawn.GUID, spawn.Entry)
	}
	values[0] = uint32(rawGUID)
	values[1] = uint32(rawGUID >> 32)
	values[2] = creatureTypeMask
	values[objectFieldEntry] = spawn.Entry
	values[unitFieldHealth] = spawn.Health
	values[unitFieldLevel] = maxUint32(spawn.Level, 1)
	values[unitFieldFaction] = spawn.Faction
	if spawn.Item1 != 0 {
		values[unitVirtualItemSlotID] = spawn.Item1
	}
	if spawn.Item2 != 0 {
		values[unitVirtualItemSlotID+1] = spawn.Item2
	}
	if spawn.Item3 != 0 {
		values[unitVirtualItemSlotID+2] = spawn.Item3
	}
	values[unitFieldFlags] = spawn.UnitFlags
	values[unitFieldFlags2] = spawn.UnitFlags2
	values[unitFieldDynamicFlags] = spawn.DynamicFlags
	values[unitFieldCreatedBy] = uint32(spawn.CreatedByGUID)
	values[unitFieldCreatedBy+1] = uint32(spawn.CreatedByGUID >> 32)
	values[unitFieldCreatedBySpell] = spawn.CreatedBySpell
	values[unitModCastSpeed] = math.Float32bits(1)
	if spawn.Health == 0 {
		values[unitFieldDynamicFlags] = 1 // UNIT_DYNFLAG_LOOTABLE
		// Unit::BuildValuesUpdateBlockForPlayer (Unit.cpp:13921-13931) masks
		// UNIT_DYNFLAG_LOOTABLE per viewer via Player::isAllowedToLoot: a dead
		// spawn only sparkles for the observer when the sparkle-visibility
		// check passes for them.
		if observer != nil && observer.server != nil && observer.player != nil &&
			!observer.server.creatureLootSparkleVisible(spawn.Map, observer.player.InstanceID, rawGUID, observer.player.GUID, observer.groupID, observer.hasPendingBind()) {
			values[unitFieldDynamicFlags] &^= unitDynFlagLootable
		}
	}
	values[unitFieldNPCFlags] = spawn.NPCFlags
	// Object::BuildValuesUpdateBlockForPlayer (Unit.cpp:13844-13850): the
	// UNIT_NPC_FLAG_SPELLCLICK bit is masked per observer via
	// Player::CanSeeSpellClickOn.
	if spawn.NPCFlags&unitNPCFlagSpellclick != 0 && observer != nil &&
		!observer.canSeeSpellClickOnCreature(ctx, spawn.Entry, spawn.Faction, rawGUID) {
		values[unitFieldNPCFlags] &^= unitNPCFlagSpellclick
	}
	values[unitFieldAttackTime] = maxUint32(spawn.AttackTime, 2000)
	values[unitFieldAttackTimeOffhand] = maxUint32(spawn.AttackTime, 2000)
	boundingRadius := spawn.BoundingRadius
	if boundingRadius <= 0 {
		boundingRadius = 0.306349
	}
	combatReach := spawn.CombatReach
	if combatReach <= 0 {
		combatReach = 1.5
	}
	values[unitFieldBoundingRadius] = math.Float32bits(boundingRadius)
	values[unitFieldCombatReach] = math.Float32bits(combatReach)
	values[unitFieldDisplayID] = spawn.Model
	values[unitFieldNativeDisplayID] = spawn.Model
	hoverHeight := spawn.HoverHeight
	if hoverHeight <= 0 {
		hoverHeight = 1
	}
	values[unitFieldHoverHeight] = math.Float32bits(hoverHeight)
	if spawn.Mount != 0 || spawn.mountKnown {
		values[unitFieldMountDisplayID] = spawn.Mount
	}
	if spawn.Bytes1 != 0 || spawn.standStateKnown {
		values[unitFieldBytes1] = spawn.Bytes1
	}
	// AuraEffect::HandleAuraUntrackable (SpellAuraEffects.cpp:2521-2537) on
	// creatures: UNIT_STAND_FLAGS_UNTRACKABLE on the UNIT_FIELD_BYTES_1
	// vis-flag byte (UNIT_BYTES_1_OFFSET_VIS_FLAG = 2, bits 16-23), as a live
	// fold over the creature's auras like the scale fold above. The C++
	// remove leg's HasAuraType gate is the same live-set check.
	if observer != nil && observer.server != nil && observer.player != nil && rawGUID != 0 &&
		observer.server.creatureHasAuraType(creatureAuraKey{Map: observer.player.Map, InstanceID: observer.player.InstanceID, GUID: rawGUID}, spellAuraUntrackable) {
		values[unitFieldBytes1] |= uint32(unitStandFlagUntrackable) << 16
	}
	// AuraEffect::HandleAuraModStalked (SpellAuraEffects.cpp) on creatures: a
	// live 68-effect aura sets UNIT_DYNFLAG_TRACK_UNIT in the dynamic flags
	// (Hunter's Mark etc.), cleared behind the HasAuraType gate on remove —
	// a live fold over the same set is the equivalent check. The C++
	// UpdateObjectVisibility arm has no Go model; the flag rides the
	// creature update like the 120 fold above. The player-target arm has no
	// Go model (UNIT_FIELD_DYNAMIC_FLAGS is not written on player updates).
	if observer != nil && observer.server != nil && observer.player != nil && rawGUID != 0 &&
		observer.server.creatureHasAuraType(creatureAuraKey{Map: observer.player.Map, InstanceID: observer.player.InstanceID, GUID: rawGUID}, spellAuraModStalked) {
		values[unitFieldDynamicFlags] |= unitDynFlagTrackUnit
	}
	if spawn.Emote != 0 || spawn.emoteStateKnown {
		values[unitNPCEmoteState] = spawn.Emote
	}
	if spawn.Bytes2 != 0 || spawn.sheathStateKnown || spawn.pvpFlagsKnown {
		values[unitFieldBytes2] = spawn.Bytes2
	}
	if spawn.ScriptTargetGUID != 0 || spawn.targetKnown {
		// UNIT_FIELD_TARGET is unwritten unless a script arm set it
		// (Unit::SetTarget); the known flag forces the field into the
		// update even when clearing back to empty.
		values[unitFieldTarget] = uint32(spawn.ScriptTargetGUID)
		values[unitFieldTarget+1] = uint32(spawn.ScriptTargetGUID >> 32)
	}
	if spawn.MaxHealth > 0 {
		values[unitFieldMaxHealth] = spawn.MaxHealth
	} else if spawn.Health > 0 {
		values[unitFieldMaxHealth] = spawn.Health
	} else {
		values[unitFieldMaxHealth] = maxUint32(spawn.Level*30, 100)
	}
	values[unitFieldHealth+1] = spawn.Mana
	values[unitFieldMaxPower1] = spawn.Mana
	if spawn.Scale > 0 {
		values[objectFieldScale] = math.Float32bits(spawn.Scale)
	}
	// Unit::RecalculateObjectScale (Unit.cpp:10593-10598) on creatures:
	// native template scale plus the live 61/239 aura sums over 100,
	// floored at 0.01 for non-players.
	if observer != nil && observer.server != nil && observer.player != nil && rawGUID != 0 {
		scaleSum := observer.server.creatureAuraScaleSum(creatureAuraKey{Map: observer.player.Map, InstanceID: observer.player.InstanceID, GUID: rawGUID})
		if scaleSum != 0 {
			scale := float64(spawn.Scale) + float64(scaleSum)/100
			if scale < 0.01 {
				scale = 0.01
			}
			values[objectFieldScale] = math.Float32bits(float32(scale))
		}
	}
	mask := protocol.NewUpdateMask(len(values))
	for index, value := range values {
		if value != 0 {
			_ = mask.Set(index)
		}
	}
	block := protocol.NewBuffer(256)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(rawGUID)
	block.WriteU8(3)
	block.WriteU16(creatureUpdateFlags)
	if spawn.TransportGUID != 0 {
		block.WriteU32(movementOnTransport)
	} else {
		block.WriteU32(0)
	}
	block.WriteU16(0)
	block.WriteU32(uint32(time.Now().UnixMilli()))
	block.WriteF32(spawn.X)
	block.WriteF32(spawn.Y)
	block.WriteF32(spawn.Z)
	block.WriteF32(spawn.Orientation)
	if spawn.TransportGUID != 0 {
		block.WritePackedGUID(spawn.TransportGUID)
		block.WriteF32(spawn.TransportX)
		block.WriteF32(spawn.TransportY)
		block.WriteF32(spawn.TransportZ)
		block.WriteF32(spawn.TransportO)
		block.WriteU32(0)
		block.WriteI8(-1)
	}
	block.WriteU32(0)
	// Create-block speed order (Object.cpp:337-345): WALK, RUN, RUN_BACK, SWIM,
	// SWIM_BACK, FLIGHT, FLIGHT_BACK, TURN_RATE, PITCH_RATE; bases from
	// baseMoveSpeed (Unit.cpp:88-98). Only WALK and RUN carry the template
	// multipliers: m_speed_rate defaults to 1.0 (Unit.cpp:376) and
	// Creature::Initialize sets only WALK/RUN from the template while SWIM and
	// FLIGHT are set to 1.0 (Creature.cpp:529-532); the back-speed UpdateSpeed
	// arms only apply debuffs (Unit.cpp:8675-8677), so the template multipliers
	// never reach RUN_BACK/SWIM/SWIM_BACK/FLIGHT/FLIGHT_BACK at spawn.
	// Documented delta: a later UpdateSpeed(MOVE_SWIM/MOVE_FLIGHT) would fold
	// the template speed_run into the rate (Unit.cpp:8750); Go never re-sends
	// creature speeds after create, so the init-time rates stand.
	for _, speed := range []float32{2.5 * spawn.WalkSpeed, 7 * spawn.RunSpeed, 4.5, 4.722222, 2.5, 7, 4.5, 3.141594, 3.14} {
		block.WriteF32(speed)
	}
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index, value := range values {
		if mask.Has(index) {
			block.WriteU32(value)
		}
	}
	return block.Bytes()
}

// splineFlagFlying is MoveSplineFlag::Flying (0x00002000,
// MoveSplineFlag.h:40): smooth (catmullrom) interpolation + flying
// animation. Set on the wire only where C++ calls MoveSplineInit::SetFly
// (MoveSplineInit.h:146) — the MoveCirclePath flying branch, the
// FlightPath generator (taxi.go already sends it), and the three script
// MovebyPath legs (razorscale's is dead code, arthas/chromie + eye of
// acherus unported); no other generator sets it, so waypoint/chase/
// follow splines stay linear even for flying creatures.
// splineFlagAnimation is MoveSplineFlag::Animation (0x00200000): the
// flags word carries the bit (it is NOT inside Mask_No_Monster_Move)
// and the tier byte + effect_start_time follow the flags, before the
// duration (MovementPacketBuilder.cpp:71-79). AnimationTier values are
// UnitDefines.h:81-88 (Ground 0, Swim 1, Hover 2, Fly 3, Submerged 4);
// animTierNone (-1) sends no animation leg at all.
const (
	splineFlagFlying           uint32 = 0x00002000
	splineFlagAnimation        uint32 = 0x00200000
	splineFlagParabolic        uint32 = 0x00000800
	splineFlagFalling          uint32 = 0x00000200 // MoveSplineFlag::Falling (MoveSplineFlag.h:35)
	splineFlagOrientationFixed uint32 = 0x00004000
	animTierGround                    = 0
	animTierHover                     = 2
	animTierNone                      = -1
)

func buildMonsterMove(rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, facing float32, hasFacing bool, transportGUID uint64, flying bool, animTier int) []byte {
	packet := protocol.NewBuffer(64)
	packet.WritePackedGUID(rawGUID)
	if transportGUID != 0 {
		// MoveSplineInit::Launch/Stop (MoveSplineInit.cpp:131-136, 181-186):
		// an on-transport spline switches opcode to SMSG_MONSTER_MOVE_TRANSPORT
		// and inserts the transport GUID + seat after the mover GUID; the body
		// is identical. Go models no creature transport seat; C++ defaults an
		// unseated passenger to -1 (MovementInfo.h:40).
		packet.WritePackedGUID(transportGUID)
		packet.WriteI8(-1)
	}
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(startX)
	packet.WriteF32(startY)
	packet.WriteF32(startZ)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	if hasFacing {
		// MoveSplineInit::SetFacing(float) (MoveSplineInit.cpp:219) arms the
		// Final_Angle leg of PacketBuilder::WriteCommonMonsterMovePart
		// (MovementPacketBuilder.cpp:58-61): facing type 4
		// (MonsterMoveFacingAngle) followed by the f32 angle. The Final_Angle
		// bit is inside Mask_Final_Facing, which WriteCommonMonsterMovePart
		// strips from the wire SplineFlags (Mask_No_Monster_Move), so it
		// never lands in the flags word below (only Flying/Animation do).
		packet.WriteU8(4) // MonsterMoveFacingAngle
		packet.WriteF32(facing)
	} else {
		packet.WriteU8(0) // MonsterMoveNormal
	}
	splineFlags := uint32(0)
	if flying {
		splineFlags |= splineFlagFlying
	}
	if animTier >= 0 {
		splineFlags |= splineFlagAnimation
	}
	packet.WriteU32(splineFlags)
	if animTier >= 0 {
		// MovementPacketBuilder.cpp:73-76: the animation leg writes the
		// tier byte + effect_start_time between the flags and the
		// duration; the tier value itself is stripped from the flags
		// word (Mask_Animations), so the flags above stay tier-free.
		packet.WriteU8(uint8(animTier))
		packet.WriteU32(0) // effect_start_time
	}
	packet.WriteU32(duration) // Duration in ms
	packet.WriteU32(1)        // Points count
	packet.WriteF32(destX)
	packet.WriteF32(destY)
	packet.WriteF32(destZ)
	return packet.Bytes()
}

// appendPackXYZ is ByteBuffer::appendPackXYZ (ByteBuffer.h:463-470):
// the PackedXYZ wire form of a midpoint offset, 0.25yd units, x/y
// 11 bits and z 10 bits of a uint32. The (int) truncation toward
// zero and the bit masks match C++ on two's-complement targets, so
// negative offsets encode identically.
func appendPackXYZ(packet *protocol.Buffer, x, y, z float32) {
	packed := uint32(int32(x/0.25)&0x7FF) | uint32(int32(y/0.25)&0x7FF)<<11 | uint32(int32(z/0.25)&0x3FF)<<22
	packet.WriteU32(packed)
}

// buildMonsterMoveMulti is the multi-point twin of buildMonsterMove:
// MovementPacketBuilder.cpp WriteLinearPath (MovementPacketBuilder
// .cpp:95-115) for a MoveSplineInit::MovebyPath poly path. points
// are the path controls with points[0] the start (PathGenerator::
// GetPath, path[0] corrected to the launch position by
// MoveSplineInit::Launch): the spline holds the controls plus a
// duplicate last point (SplineBase::InitLinear, Spline.cpp), so the
// wire count is len(points)-2, the destination is the last control,
// and the middle points (controls[2..len-2]) ride as PackedXYZ
// offsets from the midpoint of the first and last controls — the
// first control after the start is never written, exactly as C++
// writes it. A path under 3 points falls back to the single-point
// writer, preserving its count-1 form.
func buildMonsterMoveMulti(rawGUID uint64, startX, startY, startZ float32, points []pathVec, duration uint32, facing float32, hasFacing bool, transportGUID uint64, flying bool, animTier int) []byte {
	if len(points) < 3 {
		dest := pathVec{startX, startY, startZ}
		if len(points) > 0 {
			dest = points[len(points)-1]
		}
		return buildMonsterMove(rawGUID, startX, startY, startZ, dest.X, dest.Y, dest.Z, duration, facing, hasFacing, transportGUID, flying, animTier)
	}
	packet := protocol.NewBuffer(64 + 4*len(points))
	packet.WritePackedGUID(rawGUID)
	if transportGUID != 0 {
		packet.WritePackedGUID(transportGUID)
		packet.WriteI8(-1)
	}
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(startX)
	packet.WriteF32(startY)
	packet.WriteF32(startZ)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	if hasFacing {
		packet.WriteU8(4) // MonsterMoveFacingAngle
		packet.WriteF32(facing)
	} else {
		packet.WriteU8(0) // MonsterMoveNormal
	}
	splineFlags := uint32(0)
	if flying {
		splineFlags |= splineFlagFlying
	}
	if animTier >= 0 {
		splineFlags |= splineFlagAnimation
	}
	packet.WriteU32(splineFlags)
	if animTier >= 0 {
		packet.WriteU8(uint8(animTier))
		packet.WriteU32(0) // effect_start_time
	}
	packet.WriteU32(duration) // Duration in ms
	last := len(points) - 1
	packet.WriteU32(uint32(last - 1)) // Points count (WriteLinearPath last_idx)
	packet.WriteF32(points[last].X)
	packet.WriteF32(points[last].Y)
	packet.WriteF32(points[last].Z)
	middleX := (points[1].X + points[last].X) / 2
	middleY := (points[1].Y + points[last].Y) / 2
	middleZ := (points[1].Z + points[last].Z) / 2
	for i := 2; i <= last-1; i++ {
		appendPackXYZ(packet, middleX-points[i].X, middleY-points[i].Y, middleZ-points[i].Z)
	}
	return packet.Bytes()
}

// buildMonsterMoveFalling is the MoveFall wire twin of buildMonsterMove
// (MovementPacketBuilder.cpp WriteCommonMonsterMovePart): the flags word
// carries Falling (0x00000200, MoveSplineFlag.h:35 — outside
// Mask_No_Monster_Move, so it survives onto the wire) and, unlike
// Parabolic, adds no leg between Duration and the point count — the
// client derives the fall elevation from the flag + duration.
func buildMonsterMoveFalling(rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, transportGUID uint64) []byte {
	packet := protocol.NewBuffer(64)
	packet.WritePackedGUID(rawGUID)
	if transportGUID != 0 {
		packet.WritePackedGUID(transportGUID)
		packet.WriteI8(-1)
	}
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(startX)
	packet.WriteF32(startY)
	packet.WriteF32(startZ)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	packet.WriteU8(0)                               // MonsterMoveNormal
	packet.WriteU32(splineFlagFalling)              // SplineFlags (Falling)
	packet.WriteU32(duration)                       // Duration in ms
	packet.WriteU32(1)                              // Points count
	packet.WriteF32(destX)
	packet.WriteF32(destY)
	packet.WriteF32(destZ)
	return packet.Bytes()
}

// buildMonsterMoveParabolic is the MoveJump wire twin of buildMonsterMove
// (MovementPacketBuilder.cpp WriteCommonMonsterMovePart + WriteMonsterMove):
// MoveJump never sets a facing arm (hasOrientation=false), the flags field
// carries Parabolic (0x00000800, MoveSplineFlag.h:37 — it is NOT inside
// Mask_No_Monster_Move, so it survives onto the wire), and the parabolic
// leg writes vertical_acceleration + effect_start_time (0, SetParabolic
// (amplitude, 0)) between Duration and the point count. Everything else
// matches the linear layout exactly. orientationFixed carries
// MoveSplineFlag::OrientationFixed (0x00004000, MoveSplineFlag.h:40 —
// likewise outside Mask_No_Monster_Move): MoveKnockbackFrom sets it, so
// the client keeps the model orientation instead of re-facing along the
// spline; MoveJump never sets it.
func buildMonsterMoveParabolic(rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, verticalAccel float32, transportGUID uint64, orientationFixed bool) []byte {
	packet := protocol.NewBuffer(64)
	packet.WritePackedGUID(rawGUID)
	if transportGUID != 0 {
		packet.WritePackedGUID(transportGUID)
		packet.WriteI8(-1)
	}
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(startX)
	packet.WriteF32(startY)
	packet.WriteF32(startZ)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	packet.WriteU8(0)                               // MonsterMoveNormal
	splineFlags := splineFlagParabolic
	if orientationFixed {
		splineFlags |= splineFlagOrientationFixed
	}
	packet.WriteU32(splineFlags)   // SplineFlags (Parabolic [+ OrientationFixed])
	packet.WriteU32(duration)      // Duration in ms
	packet.WriteF32(verticalAccel) // vertical_acceleration
	packet.WriteU32(0)             // effect_start_time
	packet.WriteU32(1)             // Points count
	packet.WriteF32(destX)
	packet.WriteF32(destY)
	packet.WriteF32(destZ)
	return packet.Bytes()
}

func creatureWorldGUID(guid, entry uint32) uint64 {
	return uint64(guid) | uint64(entry)<<24 | uint64(0xF130)<<48
}

func maxUint32(value, fallback uint32) uint32 {
	if value == 0 {
		return fallback
	}
	return value
}

func (s *Server) broadcastMonsterMove(mapID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, phase uint32) {
	s.broadcastMonsterMoveMode(mapID, rawGUID, startX, startY, startZ, destX, destY, destZ, duration, false, phase, 0)
}

func (s *Server) broadcastMonsterMoveMode(mapID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, walk bool, phase uint32, transportGUID uint64) {
	// Documented delta vs C++ (audited 2026-10-07): WaypointMovementGenerator
	// selects walk speed via init.SetWalk (velocity only — MoveSplineInitArgs.h:58,
	// MoveSplineInit.cpp:111) and never calls Creature::SetWalk, so C++ sends no
	// SMSG_SPLINE_MOVE_SET_WALK_MODE for walk waypoint nodes (the packet is sent
	// only by Creature::SetWalk, Creature.cpp:3017, i.e. persistent walk state).
	// Go sends the mode packet per walk node so walk-speed splines animate as
	// walk instead of run-in-place; deliberate client-visible improvement.
	packet := buildMonsterMove(rawGUID, startX, startY, startZ, destX, destY, destZ, duration, 0, false, transportGUID, false, animTierNone)
	moveOpcode := protocol.OpcodeSMSG_MONSTER_MOVE
	if transportGUID != 0 {
		moveOpcode = protocol.OpcodeSMSG_MONSTER_MOVE_TRANSPORT
	}
	modeOpcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
	if walk {
		modeOpcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	if phase == 0 {
		phase = 1
	}
	for _, sess := range targets {
		if sess.player == nil || math.Hypot(float64(startX-sess.player.X), float64(startY-sess.player.Y)) > distance {
			continue
		}
		if phase&sess.currentPlayerPhaseMask() == 0 {
			continue
		}
		_ = sess.write(uint16(modeOpcode), modePacket.Bytes(), true)
		_ = sess.write(uint16(moveOpcode), packet, true)
	}
	// Creature-target re-stream (see reStreamFarsightObservers): the spline
	// destination is where the viewpoint now sits. Map-only, matching this
	// form's map-wide fan-out; the form itself has no live callers (only
	// the broadcastMonsterMove wrapper), so no seer gate is ported here.
	s.reStreamFarsightObservers(mapID, 0, false, rawGUID, destX, destY, destZ, false)
}

func (s *Server) broadcastMonsterMoveInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, walk bool, facing float32, hasFacing bool, phase uint32, transportGUID uint64) {
	s.broadcastMonsterMoveFlagsInInstance(mapID, instanceID, rawGUID, startX, startY, startZ, destX, destY, destZ, duration, walk, facing, hasFacing, phase, transportGUID, false, animTierNone)
}

// broadcastMonsterMoveFlagsInInstance is broadcastMonsterMoveInInstance
// with the spline flags C++ sets via MoveSplineInit::SetFly /
// SetAnimation: flying rides the MoveCirclePath flying branch
// (MotionMaster.cpp:851-855) and animTier the MoveLand/MoveTakeoff and
// waypoint LAND/TAKEOFF legs (Ground 0 / Hover 2, UnitDefines.h:81-88).
// Every other launch passes flying=false, animTier=animTierNone — no
// other generator sets either flag, so their packets are byte-identical
// to before.
func (s *Server) broadcastMonsterMoveFlagsInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, walk bool, facing float32, hasFacing bool, phase uint32, transportGUID uint64, flying bool, animTier int) {
	packet := buildMonsterMove(rawGUID, startX, startY, startZ, destX, destY, destZ, duration, facing, hasFacing, transportGUID, flying, animTier)
	s.broadcastMonsterMovePacketInInstance(mapID, instanceID, rawGUID, startX, startY, destX, destY, destZ, walk, phase, transportGUID, packet)
}

// broadcastMonsterMoveMultiInInstance is the multi-point twin of
// broadcastMonsterMoveFlagsInInstance: the SMSG_MONSTER_MOVE for a
// CalculatePath poly path (Movement::MoveSplineInit::MovebyPath,
// MovementPacketBuilder.cpp WriteLinearPath), carrying every path
// point instead of a single destination so the client follows the
// routed path instead of a straight line. A 2-point path falls back
// to the single-point writer (buildMonsterMoveMulti).
func (s *Server) broadcastMonsterMoveMultiInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ float32, points []pathVec, duration uint32, walk bool, facing float32, hasFacing bool, phase uint32, transportGUID uint64, flying bool, animTier int) {
	packet := buildMonsterMoveMulti(rawGUID, startX, startY, startZ, points, duration, facing, hasFacing, transportGUID, flying, animTier)
	end := points[len(points)-1]
	s.broadcastMonsterMovePacketInInstance(mapID, instanceID, rawGUID, startX, startY, end.X, end.Y, end.Z, walk, phase, transportGUID, packet)
}

// broadcastMonsterMovePacketInInstance is the shared fan-out of the
// monster-move broadcasts: the walk/run mode packet plus the move
// packet to the in-instance, in-range, phase-visible set, then the
// creature-target farsight re-stream at the spline destination.
func (s *Server) broadcastMonsterMovePacketInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, destX, destY, destZ float32, walk bool, phase uint32, transportGUID uint64, packet []byte) {
	moveOpcode := protocol.OpcodeSMSG_MONSTER_MOVE
	if transportGUID != 0 {
		moveOpcode = protocol.OpcodeSMSG_MONSTER_MOVE_TRANSPORT
	}
	modeOpcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
	if walk {
		modeOpcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	if phase == 0 {
		phase = 1
	}
	for _, sess := range targets {
		if sess.player == nil {
			continue
		}
		// Seer distance leg (CanSeeOrDetect GetViewpoint leg,
		// Object.cpp:1618, as on the create helper): a bound observer
		// receives the mover's spline when the mover is viewpoint-near,
		// not only body-near — otherwise the viewpoint-near mover's own
		// packets never reach the observer at all. Unresolvable viewpoints
		// fail open to the body coords, preserving the old gate exactly.
		cx, cy := sess.player.X, sess.player.Y
		if sess.player.FarsightGUID != 0 {
			if vx, vy, _, ok := s.seerDeliveryPosition(mapID, instanceID, sess.player.GUID, sess.player.FarsightGUID); ok {
				cx, cy = vx, vy
			}
		}
		if math.Hypot(float64(startX-cx), float64(startY-cy)) > distance {
			continue
		}
		if phase&sess.currentPlayerPhaseMask() == 0 {
			continue
		}
		_ = sess.write(uint16(modeOpcode), modePacket.Bytes(), true)
		_ = sess.write(uint16(moveOpcode), packet, true)
	}
	// Creature-target re-stream (see reStreamFarsightObservers): the spline
	// destination is where the viewpoint now sits.
	s.reStreamFarsightObservers(mapID, instanceID, true, rawGUID, destX, destY, destZ, false)
}

// broadcastMonsterMoveJumpInInstance is the MoveJump fan-out twin of
// broadcastMonsterMoveInInstance with the parabolic packet: a run-mode
// packet plus the parabolic move packet to the same in-instance,
// in-range, phase-visible set. The jump carries no walk-mode leg.
func (s *Server) broadcastMonsterMoveJumpInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, verticalAccel float32, phase uint32, transportGUID uint64, orientationFixed bool) {
	packet := buildMonsterMoveParabolic(rawGUID, startX, startY, startZ, destX, destY, destZ, duration, verticalAccel, transportGUID, orientationFixed)
	moveOpcode := protocol.OpcodeSMSG_MONSTER_MOVE
	if transportGUID != 0 {
		moveOpcode = protocol.OpcodeSMSG_MONSTER_MOVE_TRANSPORT
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	if phase == 0 {
		phase = 1
	}
	for _, sess := range targets {
		if sess.player == nil {
			continue
		}
		cx, cy := sess.player.X, sess.player.Y
		if sess.player.FarsightGUID != 0 {
			if vx, vy, _, ok := s.seerDeliveryPosition(mapID, instanceID, sess.player.GUID, sess.player.FarsightGUID); ok {
				cx, cy = vx, vy
			}
		}
		if math.Hypot(float64(startX-cx), float64(startY-cy)) > distance {
			continue
		}
		if phase&sess.currentPlayerPhaseMask() == 0 {
			continue
		}
		_ = sess.write(uint16(protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE), modePacket.Bytes(), true)
		_ = sess.write(uint16(moveOpcode), packet, true)
	}
	s.reStreamFarsightObservers(mapID, instanceID, true, rawGUID, destX, destY, destZ, false)
}

// broadcastMonsterMoveFallInInstance is the MoveFall fan-out twin of
// broadcastMonsterMoveJumpInInstance with the falling packet: a run-mode
// packet plus the falling move packet to the same in-instance,
// in-range, phase-visible set. The fall carries no walk-mode leg.
func (s *Server) broadcastMonsterMoveFallInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, phase uint32, transportGUID uint64) {
	packet := buildMonsterMoveFalling(rawGUID, startX, startY, startZ, destX, destY, destZ, duration, transportGUID)
	moveOpcode := protocol.OpcodeSMSG_MONSTER_MOVE
	if transportGUID != 0 {
		moveOpcode = protocol.OpcodeSMSG_MONSTER_MOVE_TRANSPORT
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	if phase == 0 {
		phase = 1
	}
	for _, sess := range targets {
		if sess.player == nil {
			continue
		}
		cx, cy := sess.player.X, sess.player.Y
		if sess.player.FarsightGUID != 0 {
			if vx, vy, _, ok := s.seerDeliveryPosition(mapID, instanceID, sess.player.GUID, sess.player.FarsightGUID); ok {
				cx, cy = vx, vy
			}
		}
		if math.Hypot(float64(startX-cx), float64(startY-cy)) > distance {
			continue
		}
		if phase&sess.currentPlayerPhaseMask() == 0 {
			continue
		}
		_ = sess.write(uint16(protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE), modePacket.Bytes(), true)
		_ = sess.write(uint16(moveOpcode), packet, true)
	}
	s.reStreamFarsightObservers(mapID, instanceID, true, rawGUID, destX, destY, destZ, false)
}

func (s *Server) buildCreatureValuesUpdate(guid uint64, fields map[int]uint32) (*protocol.Packet, error) {
	values := make([]uint32, creatureValuesCount)
	mask := protocol.NewUpdateMask(creatureValuesCount)
	for index, value := range fields {
		if index < 0 || index >= creatureValuesCount {
			continue
		}
		values[index] = value
		if err := mask.Set(index); err != nil {
			return nil, err
		}
	}
	block := protocol.NewBuffer(64 + len(fields)*4)
	block.WriteU8(protocol.UpdateValues)
	block.WritePackedGUID(guid)
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index := 0; index < creatureValuesCount; index++ {
		if mask.Has(index) {
			block.WriteU32(values[index])
		}
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block.Bytes())
	return updates.BuildPacket(0)
}

// isUnitStandState is Unit::IsStandState (Unit.cpp:10551-10555): not a sit
// state (Unit::IsSitState, Unit.cpp:10542-10549) and neither SLEEP (3) nor
// KNEEL (8) — DEAD (7) and SUBMERGED (9) count as standing.
func isUnitStandState(s uint8) bool {
	switch s {
	case 1, 2, 4, 5, 6: // SIT, SIT_CHAIR, SIT_LOW_CHAIR, SIT_MEDIUM_CHAIR, SIT_HIGH_CHAIR
		return false
	}
	return s != 3 && s != 8 // SLEEP, KNEEL
}

// creatureTemplateBytes1 mirrors the spawn query's bytes1 COALESCE
// (creatures.go: creatures list query): creature_addon.bytes1 wins over
// creature_template_addon.bytes1, else 0. Only the non-stand bytes are
// ever read from it — byte 0 is the motion's live StandState, and byte 1
// is forced 0 (LoadCreaturesAddon, Creature.cpp:2642).
func (s *Server) creatureTemplateBytes1(dbGUID uint32) uint32 {
	var b uint32
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || dbGUID == 0 {
		return 0
	}
	_ = s.WorldStore.DB.QueryRowContext(context.Background(),
		`SELECT COALESCE(ca.bytes1, cta.bytes1, 0) FROM creature AS c
		 LEFT JOIN creature_addon AS ca ON ca.guid = c.guid
		 LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id
		 WHERE c.guid = ?`, dbGUID).Scan(&b)
	return b &^ 0xFF00
}

// standCreatureMotionUp mirrors the creature half of
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
// non-friendly non-positive (or dispel-effect) spell hit stands a
// non-standing unit target up — Unit::SetStandState (Unit.cpp:10557-10570)
// via SetByteValue on UNIT_FIELD_BYTES_1 byte 0. The live stand byte is
// the Lua override when StandStateKnown, else the template byte 0; the
// broadcast merges the cleared byte 0 into the template's other bytes
// (stand flags / vis flag), and StandStateKnown is set so the next spawn
// rebuild keeps the value (applyCreatureMotionToSpawn). The player-only
// SMSG_STANDSTATE_UPDATE leg (Unit.cpp:10564-10568) is N/A for creatures.
// Standing strips AURA_INTERRUPT_FLAG_NOT_SEATED auras (Unit.cpp:10560).
// Returns true when the creature was stood up.
func (s *Server) standCreatureMotionUp(motion *creatureMotion) bool {
	if s == nil || motion == nil {
		return false
	}
	s.motionMu.Lock()
	liveKnown := motion.StandStateKnown
	live := motion.StandState
	cacheKnown := motion.bytes1CacheKnown
	cache := motion.bytes1Cache
	s.motionMu.Unlock()
	if !cacheKnown {
		cache = s.creatureTemplateBytes1(uint32(motion.GUID & 0xFFFFFF))
		s.motionMu.Lock()
		motion.bytes1Cache, motion.bytes1CacheKnown = cache, true
		s.motionMu.Unlock()
	}
	if !liveKnown {
		live = uint8(cache & 0xFF)
	}
	if isUnitStandState(live) {
		return false
	}
	s.motionMu.Lock()
	motion.StandState, motion.StandStateKnown = 0, true
	full := cache & 0xFFFFFF00
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	s.motionMu.Unlock()
	s.removeCreatureAurasWithInterruptFlags(motion, auraInterruptFlagNotSeated)
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldBytes1: full})
	return true
}

func (s *Server) broadcastCreatureValuesUpdateInInstance(mapID, instanceID uint32, guid uint64, fields map[int]uint32) {
	if stored, ok := fields[unitFieldDynamicFlags]; ok {
		s.fanOutCreatureDynamicFlagsUpdate(mapID, instanceID, guid, fields, stored)
		return
	}
	packet, err := s.buildCreatureValuesUpdate(guid, fields)
	if err != nil || packet == nil {
		return
	}
	// WorldObject::BuildUpdate (Object.cpp:3620) builds values updates
	// "for all visible players" — the motion-centered visibility set, not
	// the whole instance. An unlocated creature keeps the instance-wide
	// fan-out so no recipient is dropped.
	if m := s.findCreatureMotion(mapID, instanceID, guid); m != nil {
		s.fanOutCreatureValuesUpdate(packet, mapID, instanceID, guid, true, m.X, m.Y, m.Z, m.PhaseMask)
		return
	}
	s.fanOutCreatureValuesUpdate(packet, mapID, instanceID, guid, false, 0, 0, 0, 0)
}

// fanOutCreatureDynamicFlagsUpdate mirrors Unit::BuildValuesUpdate
// (Unit.cpp:13653-13670): the stored TAPPED bits are rebuilt per viewer from
// the creature's loot recipient — TAPPED for everyone, plus TAPPED_BY_PLAYER
// for the tapper and its group — and LOOTABLE is masked by
// Player::isAllowedToLoot. One block is built per distinct value.
func (s *Server) fanOutCreatureDynamicFlagsUpdate(mapID, instanceID uint32, guid uint64, fields map[int]uint32, stored uint32) {
	var tapPlayer, tapGroup uint64
	var tapped, gated bool
	var cx, cy, cz float32
	var phase uint32
	if motion := s.findCreatureMotion(mapID, instanceID, guid); motion != nil {
		tapPlayer, tapGroup = motion.TapPlayerGUID, motion.TapGroupID
		tapped = tapPlayer != 0 || tapGroup != 0
		gated, cx, cy, cz, phase = true, motion.X, motion.Y, motion.Z, motion.PhaseMask
	}
	variants := make(map[uint32]*protocol.Packet, 4)
	s.fanOutCreatureValuesUpdateByViewer(func(viewerGUID, viewerGroupID uint64, pendingBind bool) *protocol.Packet {
		flags := stored &^ (unitDynFlagTapped | unitDynFlagTappedByPlayer)
		if tapped {
			flags |= unitDynFlagTapped
			if tapPlayer == viewerGUID || tapGroup != 0 && tapGroup == viewerGroupID {
				flags |= unitDynFlagTappedByPlayer
			}
		}
		if flags&unitDynFlagLootable != 0 && !s.creatureLootSparkleVisible(mapID, instanceID, guid, viewerGUID, viewerGroupID, pendingBind) {
			flags &^= unitDynFlagLootable
		}
		if variant, ok := variants[flags]; ok {
			return variant
		}
		variantFields := fields
		if flags != stored {
			variantFields = make(map[int]uint32, len(fields))
			for index, value := range fields {
				variantFields[index] = value
			}
			variantFields[unitFieldDynamicFlags] = flags
		}
		packet, err := s.buildCreatureValuesUpdate(guid, variantFields)
		if err != nil {
			packet = nil
		}
		variants[flags] = packet
		return packet
	}, mapID, instanceID, guid, gated, cx, cy, cz, phase)
}

// broadcastCreatureValuesUpdateAtInInstance is the motionMu-held twin of
// broadcastCreatureValuesUpdateInInstance: the caller holds the motion lock
// and passes the creature's coordinates, so no motion lookup runs here.
// The per-viewer loot-sparkle mask is skipped on this path — that check
// takes motionMu through findCreatureMotion — so the base packet goes to
// the visibility set; the mask only matters for lootable-flag broadcasts,
// and the one lock-held site carrying that flag deadlocked on the mask
// path before this variant existed.
func (s *Server) broadcastCreatureValuesUpdateAtInInstance(mapID, instanceID uint32, guid uint64, fields map[int]uint32, x, y, z float32, phase uint32) {
	packet, err := s.buildCreatureValuesUpdate(guid, fields)
	if err != nil || packet == nil {
		return
	}
	s.fanOutCreatureValuesUpdate(packet, mapID, instanceID, guid, true, x, y, z, phase)
}

// fanOutCreatureValuesUpdate delivers a built creature values-update block
// to an instance's sessions, distance-gated to the 100yd visibility range
// around (cx, cy, cz) when gated, instance-wide otherwise. The masked
// variant, when present, goes to viewers the loot sparkle is hidden from.
// C++ sends values updates only to players with the object at client
// (HaveAtClient), and the client set is built around the viewpoint
// viewpoint (UpdateVisibilityForPlayer visits around m_seer,
// Player.cpp:22798), so the gated distance leg is a union: a recipient
// qualifies on its body position or on its viewpoint position, matching
// the create helpers' viewpoint set plus the body-centered grid stream;
// an unresolvable viewpoint contributes nothing, never dropping a
// body-qualified recipient.
func (s *Server) fanOutCreatureValuesUpdate(packet *protocol.Packet, mapID, instanceID uint32, guid uint64, gated bool, cx, cy, cz float32, phase uint32) {
	s.fanOutCreatureValuesUpdateByViewer(func(uint64, uint64, bool) *protocol.Packet { return packet }, mapID, instanceID, guid, gated, cx, cy, cz, phase)
}

func (s *Server) fanOutCreatureValuesUpdateByViewer(packetFor func(viewerGUID, viewerGroupID uint64, pendingBind bool) *protocol.Packet, mapID, instanceID uint32, guid uint64, gated bool, cx, cy, cz float32, phase uint32) {
	s.sessionsMu.RLock()
	targets := make([]seerRecipient, 0, len(s.sessions))
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		targets = append(targets, seerRecipient{sess: sess, guid: sess.player.GUID, farsight: sess.player.FarsightGUID, x: sess.player.X, y: sess.player.Y, z: sess.player.Z})
	}
	s.sessionsMu.RUnlock()
	if gated && phase == 0 {
		phase = 1
	}
	for _, target := range targets {
		sess := target.sess
		if gated {
			if distance3D(target.x, target.y, target.z, cx, cy, cz) > 100.0 {
				if vx, vy, vz, ok := s.seerDeliveryPosition(mapID, instanceID, target.guid, target.farsight); !ok || distance3D(vx, vy, vz, cx, cy, cz) > 100.0 {
					continue
				}
			}
		}
		if gated && phase&sess.currentPlayerPhaseMask() == 0 {
			continue
		}
		packet := packetFor(sess.player.GUID, sess.groupID, sess.hasPendingBind())
		if packet == nil {
			continue
		}
		_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
}

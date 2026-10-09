package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	dynamicObjectTypeMask          uint32 = 0x0041
	dynamicObjectHighGUID          uint64 = 0xF100
	dynamicObjectTypeArea          uint8  = 1 // DYNAMIC_OBJECT_AREA_SPELL (DynamicObject.h:31)
	dynamicObjectTypeFarsightFocus uint8  = 2 // DYNAMIC_OBJECT_FARSIGHT_FOCUS (DynamicObject.h:32)
	dynamicObjectFlags                    = uint16(0x0150)
)

type dynamicSpellObjectState struct {
	GUID, CasterGUID, SpellID uint64
	Map, InstanceID           uint32
	X, Y, Z, Orientation      float32
	Radius                    float32
	CastTime                  uint32
	ObjectType                uint8 // DYNAMICOBJECT_BYTES type: area spell vs farsight focus
	IsFarsightFocus           bool  // SetCasterViewpoint viewpoint; cleared on despawn
	SpellData                 wotlk.Spell
	AuraEffect                wotlk.SpellEffect
	AuraDurationMs            uint32
	AuraPeriodMs              uint32
	AuraAmount                uint32
	AuraSchoolMask            uint8
	NextAuraTick              time.Time
	DespawnTimer              *time.Timer
	// ExpiresAt is the wall-clock deadline the DespawnTimer was armed for;
	// Spell::DelayedChannel's dynobj->Delay arm (Spell.cpp:7324-7326)
	// shortens it on channel pushback.
	ExpiresAt time.Time
	// AuraHolders tracks the target GUIDs currently holding this object's
	// persistent-area aura — the Go analog of Aura::UpdateTargetMap's
	// application set (SpellAuras.cpp:643). Targets that leave the radius
	// lose the aura on the next update pass, and the set scopes the
	// despawn strip to this object's applications.
	AuraHolders map[uint64]struct{}
}

func dynamicSpellGUID(low uint32) uint64 { return (dynamicObjectHighGUID << 48) | uint64(low) }

func (s *Server) nextDynamicSpellLowGUID() uint32 {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	s.nextDynamicSpellGUID++
	if s.nextDynamicSpellGUID == 0 {
		s.nextDynamicSpellGUID = 1
	}
	return s.nextDynamicSpellGUID
}

func buildDynamicSpellObjectUpdate(object *dynamicSpellObjectState) []byte {
	values := make([]uint32, 12)
	values[0] = uint32(object.GUID)
	values[1] = uint32(object.GUID >> 32)
	values[2] = dynamicObjectTypeMask
	values[3] = uint32(object.SpellID)
	values[4] = math.Float32bits(1)
	values[6] = uint32(object.CasterGUID)
	values[7] = uint32(object.CasterGUID >> 32)
	values[8] = uint32(dynamicObjectTypeArea)
	if object.ObjectType != 0 {
		// DYNAMICOBJECT_BYTES carries the dynobj type (DynamicObject.h:31-32);
		// area-spell objects are the historical default, farsight-focus
		// objects set it explicitly (EffectAddFarsight).
		values[8] = uint32(object.ObjectType)
	}
	values[9] = uint32(object.SpellID)
	values[10] = math.Float32bits(object.Radius)
	values[11] = object.CastTime
	mask := protocol.NewUpdateMask(len(values))
	for index, value := range values {
		if value != 0 {
			_ = mask.Set(index)
		}
	}
	block := protocol.NewBuffer(128)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(object.GUID)
	block.WriteU8(6)
	block.WriteU16(dynamicObjectFlags)
	block.WriteU8(0)
	block.WriteF32(object.X)
	block.WriteF32(object.Y)
	block.WriteF32(object.Z)
	block.WriteF32(object.X)
	block.WriteF32(object.Y)
	block.WriteF32(object.Z)
	block.WriteF32(object.Orientation)
	block.WriteF32(0)
	block.WriteU32(uint32(object.GUID))
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index, value := range values {
		if mask.Has(index) {
			block.WriteU32(value)
		}
	}
	return block.Bytes()
}

func (s *Server) spawnDynamicSpellObject(object *dynamicSpellObjectState, duration time.Duration) {
	if s == nil || object == nil || duration <= 0 {
		return
	}
	if caster := s.findSessionByGUID(object.CasterGUID); caster != nil && caster.player != nil {
		object.Map, object.InstanceID = caster.player.Map, caster.player.InstanceID
	}
	s.objectsMu.Lock()
	if s.dynamicSpellObjects == nil {
		s.dynamicSpellObjects = make(map[uint64]*dynamicSpellObjectState)
	}
	s.dynamicSpellObjects[object.GUID] = object
	object.ExpiresAt = time.Now().Add(duration)
	object.DespawnTimer = time.AfterFunc(duration, func() { s.despawnDynamicSpellObject(object.GUID) })
	s.objectsMu.Unlock()
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildDynamicSpellObjectUpdate(object))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastToInstance(object.Map, object.InstanceID, packet.Opcode, packet.Payload.Bytes(), nil)
	}
}

func (s *Server) updateDynamicSpellAuras(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	objects := make([]*dynamicSpellObjectState, 0, len(s.dynamicSpellObjects))
	for _, object := range s.dynamicSpellObjects {
		if object == nil || object.AuraPeriodMs == 0 || now.Before(object.NextAuraTick) {
			continue
		}
		object.NextAuraTick = now.Add(time.Duration(object.AuraPeriodMs) * time.Millisecond)
		objects = append(objects, object)
	}
	s.objectsMu.Unlock()
	for _, object := range objects {
		caster := s.findSessionByGUID(object.CasterGUID)
		if caster == nil || caster.player == nil || caster.player.Map != object.Map || caster.player.InstanceID != object.InstanceID {
			s.despawnDynamicSpellObject(object.GUID)
			continue
		}
		target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagDestLocation, Destination: protocol.SpellTargetLocation{X: object.X, Y: object.Y, Z: object.Z}}
		areaTargets := caster.spellAreaEnemyTargets(ctx, object.SpellData, target)
		inRadius := make(map[uint64]struct{}, len(areaTargets))
		for _, targetGUID := range areaTargets {
			// Aura::UpdateTargetMap (SpellAuras.cpp:700-701): dynobj auras
			// don't hit flying targets.
			if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.inFlight {
				continue
			}
			inRadius[targetGUID] = struct{}{}
		}
		spellID := uint32(object.SpellID)
		// Aura::UpdateTargetMap's removal arm (SpellAuras.cpp:663-684):
		// applications whose targets left the area are removed. The 500ms
		// C++ cadence is coarser here — removal rides the aura period tick.
		for holderGUID := range object.AuraHolders {
			if _, ok := inRadius[holderGUID]; ok {
				continue
			}
			s.expireDynobjHolderAura(object, holderGUID, spellID)
			delete(object.AuraHolders, holderGUID)
		}
		for _, targetGUID := range areaTargets {
			if _, ok := inRadius[targetGUID]; !ok {
				continue
			}
			if caster.hasDynamicAreaAura(targetGUID, object.SpellData.ID) {
				continue
			}
			caster.applyAuraToTarget(ctx, targetGUID, object.SpellData, object.AuraEffect, object.AuraDurationMs, object.AuraPeriodMs, object.AuraAmount, uint32(object.AuraSchoolMask), nil, false, object.CasterGUID, true)
			if object.AuraHolders == nil {
				object.AuraHolders = make(map[uint64]struct{})
			}
			object.AuraHolders[targetGUID] = struct{}{}
		}
	}
}

// expireDynobjHolderAura removes a persistent-area aura from a holder that
// left the dynobj radius — the Aura::UpdateTargetMap removal arm
// (SpellAuras.cpp:663-684). The dynobj match gates (persistent-area flag +
// caster GUID) mirror removeDynobjAura so a same-spell aura from another
// source is never stripped.
func (s *Server) expireDynobjHolderAura(object *dynamicSpellObjectState, holderGUID uint64, spellID uint32) {
	if s == nil || object == nil || holderGUID == 0 || spellID == 0 {
		return
	}
	if ts := s.findSessionByGUID(holderGUID); ts != nil {
		ts.removeDynobjAura(spellID, object.CasterGUID)
		return
	}
	s.stripDynobjCreatureAura(creatureAuraKey{Map: object.Map, InstanceID: object.InstanceID, GUID: holderGUID}, spellID, object.CasterGUID)
}

// stripDynobjCreatureAura removes the persistent-area aura (spellID, dynobj
// caster) from a creature target — the creature half of
// DynamicObject::RemoveFromWorld's RemoveAura leg (DynamicObject.cpp:63-83,
// 205-213) and of the Aura::UpdateTargetMap removal arm
// (SpellAuras.cpp:663-684). The match on the persistent-area flag and the
// dynobj caster GUID runs atomically with the removal under auraMu, so a
// same-spell aura from another source is never stripped. Removal mirrors
// removeCreatureAura's charm/taunt/packet tail.
func (s *Server) stripDynobjCreatureAura(key creatureAuraKey, spellID uint32, casterGUID uint64) {
	if s == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	var removed bool
	var wasCharm, wasTaunt bool
	var charmerGUID, taunterGUID uint64
	var slot uint8
	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil &&
				!aura.Stopped && aura.CasterGUID == casterGUID && aura.PersistentAreaAura {
				wasCharm = aura.AuraType == spellAuraCharm
				charmerGUID = aura.CasterGUID
				wasTaunt = aura.AuraType == spellAuraModTaunt
				taunterGUID = aura.CasterGUID
				aura.Stopped = true
				slot = aura.Slot
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
				s.unregisterSingleCastAura(aura)
				removed = true
			}
		}
	}
	if removed && s.creatureAuras != nil {
		if auras, ok := s.creatureAuras[key]; ok {
			delete(auras, spellID)
		}
	}
	s.auraMu.Unlock()
	if !removed {
		return
	}
	if wasTaunt {
		s.clearCreatureTaunt(key, taunterGUID)
	}
	if wasCharm {
		s.uncharmCreature(key, charmerGUID)
		if charmer := s.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}
	removePkt := protocol.BuildAuraUpdate(key.GUID, 0, slot, 0, true, false, 0, 0, 1)
	s.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

func (s *session) hasDynamicAreaAura(targetGUID uint64, spellID uint32) bool {
	if s == nil || s.server == nil {
		return false
	}
	if target := s.server.findSessionByGUID(targetGUID); target != nil {
		if target.player == nil || target.player.Map != s.player.Map || target.player.InstanceID != s.player.InstanceID {
			return false
		}
		target.castMu.Lock()
		_, found := target.activeAuras[spellID]
		target.castMu.Unlock()
		return found
	}
	s.server.auraMu.Lock()
	_, found := s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, targetGUID)][spellID]
	s.server.auraMu.Unlock()
	return found
}

func (s *Server) despawnDynamicSpellObject(guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	s.objectsMu.Lock()
	object, ok := s.dynamicSpellObjects[guid]
	if ok && object != nil && object.DespawnTimer != nil {
		object.DespawnTimer.Stop()
	}
	if ok {
		delete(s.dynamicSpellObjects, guid)
	}
	s.objectsMu.Unlock()
	if !ok || object == nil {
		return
	}
	// DynamicObject::RemoveFromWorld (DynamicObject.cpp:63-83): the dynobj's
	// own aura is removed first, which strips it from every target
	// (Aura::_Remove) — target auras never linger past the dynobj.
	s.removeDynamicAreaAuras(object)
	// DynamicObject::RemoveCasterViewpoint (DynamicObject.cpp:223): when a
	// farsight-focus object despawns, the caster's viewpoint is removed
	// (Player::SetViewpoint false arm, Player.cpp:24618) — but only if the
	// caster is still looking through this object.
	if object.IsFarsightFocus {
		if caster := s.findSessionByGUID(object.CasterGUID); caster != nil && caster.player != nil && caster.player.FarsightGUID == guid {
			caster.player.FarsightGUID = 0
			caster.sendPlayerUpdate()
		}
	}
	// DynamicObject::Remove (DynamicObject.cpp:176-182) despawns through
	// WorldObject::SendObjectDeSpawnAnim (Object.cpp:1826-1831):
	// SMSG_GAMEOBJECT_DESPAWN_ANIM carrying only the GUID — never
	// SMSG_DESTROY_OBJECT.
	packet := protocol.NewBuffer(8)
	packet.WriteU64(object.GUID)
	s.broadcastToInstance(object.Map, object.InstanceID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), packet.Bytes(), nil)
}

// removeDynamicAreaAuras mirrors DynamicObject::RemoveFromWorld's RemoveAura
// leg (DynamicObject.cpp:63-83, 206-213): when the dynobj goes away its
// aura is _Remove'd, stripping it from every target immediately — target
// auras never linger past the dynobj. The sweep is scoped to this dynobj's
// spell, caster, and persistent-area-aura flag so same-spell auras from
// other sources survive.
func (s *Server) removeDynamicAreaAuras(object *dynamicSpellObjectState) {
	if s == nil || object == nil {
		return
	}
	spellID := uint32(object.SpellID)
	casterGUID := object.CasterGUID
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess == nil || sess.player == nil {
			continue
		}
		if sess.player.Map != object.Map || sess.player.InstanceID != object.InstanceID {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, ts := range targets {
		ts.removeDynobjAura(spellID, casterGUID)
	}
	s.auraMu.Lock()
	var creatureKeys []creatureAuraKey
	for key, auras := range s.activeCreatureAuras {
		if key.Map != object.Map || key.InstanceID != object.InstanceID {
			continue
		}
		if aura := auras[spellID]; aura != nil && !aura.Stopped && aura.CasterGUID == casterGUID && aura.PersistentAreaAura {
			creatureKeys = append(creatureKeys, key)
		}
	}
	s.auraMu.Unlock()
	for _, key := range creatureKeys {
		s.removeCreatureAura(key, spellID)
	}
}

// removeDynobjAura drops the session's persistent-area aura of spellID when
// it came from casterGUID's dynobj; other auras of the same spell survive.
func (s *session) removeDynobjAura(spellID uint32, casterGUID uint64) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	s.castMu.Lock()
	aura := s.activeAuras[spellID]
	match := aura != nil && !aura.Stopped && aura.CasterGUID == casterGUID && aura.PersistentAreaAura
	s.castMu.Unlock()
	if match {
		s.removeAura(spellID)
	}
}

// delayChannelDynamicObject mirrors the dynobj arm of Spell::DelayedChannel
// (Spell.cpp:7324-7326): the caster's persistent-area object for the
// channeled spell has its remaining life shortened by the pushback delay —
// the "partial interrupt of persistent area auras" (DynamicObject::Delay,
// DynamicObject.cpp:194-197). A non-positive remainder despawns the object,
// matching the removal a negative C++ duration produces on the next update.
func (s *Server) delayChannelDynamicObject(casterGUID uint64, spellID uint32, delayMs int32) {
	if s == nil || delayMs <= 0 {
		return
	}
	s.objectsMu.Lock()
	var target *dynamicSpellObjectState
	for _, object := range s.dynamicSpellObjects {
		if object == nil || object.IsFarsightFocus || object.ObjectType == dynamicObjectTypeFarsightFocus {
			continue
		}
		if object.CasterGUID == casterGUID && object.SpellID == uint64(spellID) {
			target = object
			break
		}
	}
	if target == nil {
		s.objectsMu.Unlock()
		return
	}
	remaining := time.Until(target.ExpiresAt) - time.Duration(delayMs)*time.Millisecond
	if remaining <= 0 {
		s.objectsMu.Unlock()
		s.despawnDynamicSpellObject(target.GUID)
		return
	}
	target.ExpiresAt = time.Now().Add(remaining)
	if target.DespawnTimer != nil {
		target.DespawnTimer.Reset(remaining)
	}
	s.objectsMu.Unlock()
}

func (s *session) streamDynamicSpellObjects() {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	updates := protocol.NewUpdateData()
	s.server.objectsMu.RLock()
	for _, object := range s.server.dynamicSpellObjects {
		if object == nil || object.Map != s.player.Map || object.InstanceID != s.player.InstanceID || math.Hypot(float64(object.X-s.player.X), float64(object.Y-s.player.Y)) > distance {
			continue
		}
		updates.AddUpdateBlock(buildDynamicSpellObjectUpdate(object))
	}
	s.server.objectsMu.RUnlock()
	if updates.HasData() {
		if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
			_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
	}
}

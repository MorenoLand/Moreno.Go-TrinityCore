package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	dynamicObjectTypeMask uint32 = 0x0041
	dynamicObjectHighGUID uint64 = 0xF100
	dynamicObjectTypeArea uint8  = 1
	dynamicObjectFlags           = uint16(0x0150)
)

type dynamicSpellObjectState struct {
	GUID, CasterGUID, SpellID uint64
	Map, InstanceID           uint32
	X, Y, Z, Orientation      float32
	Radius                    float32
	CastTime                  uint32
	SpellData                 wotlk.Spell
	AuraEffect                wotlk.SpellEffect
	AuraDurationMs            uint32
	AuraPeriodMs              uint32
	AuraAmount                uint32
	AuraSchoolMask            uint8
	NextAuraTick              time.Time
	DespawnTimer              *time.Timer
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
		for _, targetGUID := range caster.spellAreaEnemyTargets(ctx, object.SpellData, target) {
			if caster.hasDynamicAreaAura(targetGUID, object.SpellData.ID) {
				continue
			}
			caster.applyAuraToTarget(ctx, targetGUID, object.SpellData, object.AuraEffect, object.AuraDurationMs, object.AuraPeriodMs, object.AuraAmount, uint32(object.AuraSchoolMask), nil, false, object.CasterGUID)
		}
	}
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
	packet := protocol.NewBuffer(9)
	packet.WriteU64(object.GUID)
	packet.WriteU8(0)
	s.broadcastToInstance(object.Map, object.InstanceID, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), packet.Bytes(), nil)
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

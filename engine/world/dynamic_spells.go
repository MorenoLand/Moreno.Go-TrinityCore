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
	PhaseMask                 uint32 // caster phase at spawn; 0 = unknown, fan-out stays phase-blind
	X, Y, Z, Orientation      float32
	// TransportGUID + TransportOffset are the DynamicObject::Create
	// transport arm (DynamicObject.cpp:114-126): a dynobj whose caster
	// rides a transport stores the passenger offset
	// (TransportBase::CalculatePassengerOffset, VehicleDefines.h:145-156)
	// and the transport GUID, so the patch follows the transport — the
	// world position is re-derived from the offset on every 500ms pass.
	// Zero TransportGUID means the caster rode no transport at spawn.
	TransportGUID                                                          uint64
	TransportOffsetX, TransportOffsetY, TransportOffsetZ, TransportOffsetO float32
	Radius                                                                 float32
	CastTime                                                               uint32
	ObjectType                                                             uint8 // DYNAMICOBJECT_BYTES type: area spell vs farsight focus
	IsFarsightFocus                                                        bool  // SetCasterViewpoint viewpoint; cleared on despawn
	SpellData                                                              wotlk.Spell
	AuraEffect                                                             wotlk.SpellEffect
	// AuraEffectIndex is the DBC effect row feeding AuraEffect; -1 when
	// no aura effect was found (mirrors the synthetic-effIndex
	// convention in applyAuraToTarget). Carried so the target-map
	// refresh can run the per-effect immunity gate (Spell.cpp:2108-2112
	// via Aura::UpdateTargetMap, SpellAuras.cpp:690-695) instead of
	// skipping it with the -1 dummy.
	AuraEffectIndex int
	AuraDurationMs  uint32
	AuraPeriodMs    uint32
	AuraAmount      uint32
	AuraSchoolMask  uint8
	// AuraEffects is the DynObjAura effect list (MAX_EFFECT_MASK): every
	// spell effect carrying an Aura, each with its own period, amount and
	// shared-tick schedule. AuraEffect/AuraEffectIndex/AuraPeriodMs/
	// AuraAmount above carry the primary entry (first with a nonzero
	// period, else the first), which drives the record's single periodic
	// timer and the NextAuraTick catch-up below.
	AuraEffects  []dynobjAuraEffect
	NextAuraTick time.Time
	// NextTargetMapPass is the next UpdateTargetMap pass deadline. C++
	// refreshes the dynobj target map every UPDATE_TARGET_MAP_INTERVAL
	// (500ms, SpellAuras.h:43) — applying the debuff to newcomers via
	// DynObjAura::UpdateOwner(apply=true) (SpellAuras.cpp:812-823) and
	// stripping leavers (SpellAuras.cpp:663-684) — independent of the
	// damage amplitude. NextAuraTick carries the dynobj's single shared
	// AuraEffect timer schedule (SpellAuras.cpp:815-843): all registered
	// applications tick simultaneously at one amplitude, so newly applied
	// auras align their first tick to it instead of starting their own
	// period timer.
	NextTargetMapPass time.Time
	// NextUpkeepTick is the next m_timeCla drain deadline for the dynobj's
	// own aura (SpellAuras.cpp:838-857, via DynamicObject::Update,
	// DynamicObject.cpp:158-173): the caster pays ManaPerSecond once per
	// second, and a failed payment despawns the whole patch. Zero arms the
	// first drain one second after spawn (Aura ctor, SpellAuras.cpp:436-437).
	NextUpkeepTick time.Time
	DespawnTimer   *time.Timer
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
	// AuraHolderMasks carries the FillTargetMap effect mask last applied
	// (or adopted) per holder — the Go analog of the AuraApplication
	// effect mask Aura::UpdateTargetMap compares against the fresh target
	// map (SpellAuras.cpp:672-679). A holder whose mask changed is
	// stripped and re-applied with the new mask; comparing FillTargetMap
	// masks, not the post-immunity record mask, keeps a partially immune
	// target from churning every pass.
	AuraHolderMasks map[uint64]uint8
}

// dynobjAuraEffect is one aura effect of a DynObjAura's effect mask.
// Spell::EffectPersistentAA creates the dynobj aura with MAX_EFFECT_MASK
// (SpellEffects.cpp:1755-1756), so every spell effect carrying an Aura is
// in the mask — not just the first APPLY_AURA row — and
// DynObjAura::FillTargetMap (SpellAuras.cpp:2763-2785) registers each target
// with a per-effect bitmask (targets[unit] |= 1 << effIndex). Each entry
// carries its own amplitude and amount: Aura::UpdateOwner
// (SpellAuras.cpp:815-843) updates every effect on its own periodic timer,
// so a damage effect and a slow effect do not share one schedule. The list
// is DBC order; the primary entry (first with a nonzero period, else the
// first entry) drives the record's single periodic timer and the dynobj's
// NextAuraTick catch-up, since Go holds one TickTimer per aura record.
type dynobjAuraEffect struct {
	Effect   wotlk.SpellEffect
	Index    int
	PeriodMs uint32
	Amount   uint32
	NextTick time.Time
}

// dynobjAuraEffectList builds the DynObjAura effect list for spell at the
// caster's level: every effect with a nonzero Aura. The per-effect period
// and amount folds are the spawn sites' old single-effect rules,
// generalized: the 3000ms default for periodic damage/heal/trigger auras
// and the level-based default amount for periodic damage/heal auras.
func dynobjAuraEffectList(spell wotlk.Spell, casterLevel int, now time.Time) []dynobjAuraEffect {
	var list []dynobjAuraEffect
	for i, effect := range spell.Effects {
		// The mask rule matches applyAuraFromCaster's effectMask build:
		// an effect with no Effect row carries no aura even if the Aura
		// column is nonzero, so it never enters the list.
		if effect.Effect == 0 || effect.Aura == 0 {
			continue
		}
		periodMs := effect.AuraPeriod
		if periodMs == 0 && (effect.Aura == 3 || effect.Aura == 8 || effect.Aura == 23 || effect.Aura == 24 || effect.Aura == 89) {
			periodMs = 3000
		}
		// The AuraEffect ctor default amount is the raw DBC base
		// (SpellAuraEffects.cpp:386) — no +1, and no non-negative clamp:
		// a negative base is a real amount (a -71 slow), carried
		// two's-complement through the uint32 amount into the signed
		// Amounts[i] slots the aura_haste/stats consumers read.
		amount := uint32(effect.BasePoints)
		if amount == 0 && (effect.Aura == 3 || effect.Aura == 23 || effect.Aura == 89) {
			amount = uint32(10 + casterLevel*2)
		}
		// AuraEffect::ResetPeriodic (SpellAuraEffects.cpp:553-563): with
		// SPELL_ATTR5_START_PERIODIC_AT_APPLY the shared periodic timer
		// starts AT the amplitude, so the first tick fires on the first
		// world update after creation — the shared schedule starts now,
		// not a period out.
		nextTick := now.Add(time.Duration(periodMs) * time.Millisecond)
		if spell.AttributesEx5&spellAttr5StartPeriodicAtApply != 0 {
			nextTick = now
		}
		list = append(list, dynobjAuraEffect{Effect: effect, Index: i, PeriodMs: periodMs, Amount: amount, NextTick: nextTick})
	}
	return list
}

// dynobjPrimaryEffect returns the list entry driving the aura record's
// single periodic timer: the first effect with a nonzero amplitude, else
// the first entry (a patch with no periodic effect has no tick at all).
func dynobjPrimaryEffect(list []dynobjAuraEffect) *dynobjAuraEffect {
	for i := range list {
		if list[i].PeriodMs > 0 {
			return &list[i]
		}
	}
	if len(list) > 0 {
		return &list[0]
	}
	return nil
}

// dynobjCheckType is one SpellTargetCheckTypes selection
// (SpellInfo.cpp:218-330) as DynObjAura::FillTargetMap derives it per
// effect (SpellAuras.cpp:2763-2785).
type dynobjCheckType uint8

const (
	dynobjCheckDefault dynobjCheckType = iota
	dynobjCheckEnemy
	dynobjCheckAlly
	dynobjCheckParty
	dynobjCheckRaid
	dynobjCheckRaidClass
	dynobjCheckEntry
)

// dynobjCheckTypeForTarget maps an implicit target value to its
// SpellImplicitTargetInfo check type (SpellInfo.cpp:218-330).
func dynobjCheckTypeForTarget(target uint32) dynobjCheckType {
	switch target {
	case 2, 6, 15, 16, 24, 28, 53, 54, 93, 104:
		return dynobjCheckEnemy
	case 4, 21, 29, 30, 31, 45, 59:
		return dynobjCheckAlly
	case 3, 20, 33, 34, 35, 37:
		return dynobjCheckParty
	case 56, 57, 58:
		return dynobjCheckRaid
	case 61:
		return dynobjCheckRaidClass
	case 7, 8, 38, 40, 46, 60, 110:
		return dynobjCheckEntry
	default:
		return dynobjCheckDefault
	}
}

// dynobjTargetIsDestRef reports the TARGET_REFERENCE_TYPE_DEST rows
// (SpellInfo.cpp:218-330): only those TargetB values override TargetA's
// check type in DynObjAura::FillTargetMap (SpellAuras.cpp:2763-2785).
func dynobjTargetIsDestRef(target uint32) bool {
	switch target {
	case 8, 16, 28, 29, 31, 34, 52, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 91, 107:
		return true
	default:
		return false
	}
}

// dynobjEffectCheckType mirrors DynObjAura::FillTargetMap's per-effect
// selection-type derivation (SpellAuras.cpp:2770-2774): TargetA's check
// type, or TargetB's when TargetB is DEST-reference.
func dynobjEffectCheckType(eff wotlk.SpellEffect) dynobjCheckType {
	check := dynobjCheckTypeForTarget(eff.ImplicitTargetA)
	if dynobjTargetIsDestRef(eff.ImplicitTargetB) {
		check = dynobjCheckTypeForTarget(eff.ImplicitTargetB)
	}
	return check
}

// dynobjTargetMasks builds the per-target effect mask
// (DynObjAura::FillTargetMap, SpellAuras.cpp:2763-2785): each aura effect
// selects its own targets by check type and sets its bit on them.
// enemy/friendly/def are the candidate sets for the ENEMY, ALLY/PARTY/
// RAID and DEFAULT/ENTRY selections; inParty and inRaid narrow the
// friendly set for PARTY/RAID effects (a nil predicate keeps the whole
// friendly set — no group context degrades to the friendly sweep rather
// than dropping the caster, which always passes IsInPartyWith).
func dynobjTargetMasks(effects []dynobjAuraEffect, enemy, friendly, def map[uint64]struct{}, inParty, inRaid func(uint64) bool) map[uint64]uint8 {
	masks := make(map[uint64]uint8)
	add := func(set map[uint64]struct{}, bit uint8, keep func(uint64) bool) {
		for guid := range set {
			if keep != nil && !keep(guid) {
				continue
			}
			masks[guid] |= bit
		}
	}
	for _, ae := range effects {
		if ae.Index < 0 {
			// The synthetic fallback row (Index -1, built when no
			// effect carries an Aura) is not a DBC effect index:
			// C++ FillTargetMap only sets bits for real effIndex
			// 0..MAX-1 with HasEffect (SpellAuras.cpp:2763-2785), so
			// it selects nothing — today its bit computes to 0 and
			// seeds zero-mask holder entries in the passes.
			continue
		}
		bit := uint8(1 << uint(ae.Index))
		switch dynobjEffectCheckType(ae.Effect) {
		case dynobjCheckEnemy:
			add(enemy, bit, nil)
		case dynobjCheckAlly:
			add(friendly, bit, nil)
		case dynobjCheckParty:
			add(friendly, bit, inParty)
		case dynobjCheckRaid:
			add(friendly, bit, inRaid)
		case dynobjCheckRaidClass:
			// RAID_CLASS also requires the target's class to match the
			// referer's; the passes carry no class per GUID, so the raid
			// membership half stands in — no RAID_CLASS dynobj effect in
			// the seed, so the refinement is unobservable there.
			add(friendly, bit, inRaid)
		case dynobjCheckEntry:
			// ENTRY has no faction arm in WorldObjectSpellTargetCheck
			// (Spell.cpp:8316) — the entry filter lives in the
			// selection helpers, not the area check — so it rides the
			// DEFAULT set like C++.
			add(def, bit, nil)
		default:
			add(def, bit, nil)
		}
	}
	return masks
}

// dynobjDefaultTargets sweeps every unit in the dynobj cylinder for a
// TARGET_CHECK_DEFAULT effect (DynObjAura::FillTargetMap,
// SpellAuras.cpp:2763-2785 via WorldObjectSpellAreaTargetCheck,
// Spell.cpp:8409-8430): the DEFAULT arm runs no faction check, so
// enemies and allies alike are selected — the cylinder (2D dist + |dz|
// <= radius) around the dynobj, alive, not flying, on the object's
// map/instance. Totems are included: only the ENEMY/ALLY/PARTY/RAID
// arms reject them (Spell.cpp:8333/8342/8354).
func (s *Server) dynobjDefaultTargets(object *dynamicSpellObjectState) map[uint64]struct{} {
	targets := make(map[uint64]struct{})
	if s == nil || object == nil {
		return targets
	}
	radius := float64(object.Radius)
	inCylinder := func(x, y, z float32) bool {
		dx, dy := float64(x-object.X), float64(y-object.Y)
		if dx*dx+dy*dy > radius*radius {
			return false
		}
		dz := float64(z - object.Z)
		return dz <= radius && -dz <= radius
	}
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess != nil && sess.player != nil && sess.player.Map == object.Map && sess.player.InstanceID == object.InstanceID {
			players = append(players, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, vs := range players {
		if vs.player == nil || vs.inFlight {
			continue
		}
		if !inCylinder(vs.player.X, vs.player.Y, vs.player.Z) {
			continue
		}
		if vs.player.Health == 0 && object.SpellData.Attributes&spellAttr0Passive == 0 && object.SpellData.AttributesEx1&spellAttr2CanTargetDead == 0 {
			continue
		}
		targets[vs.playerGUID] = struct{}{}
	}
	s.motionMu.Lock()
	var motions []*creatureMotion
	for _, tm := range s.motionMapLocked(object.Map, object.InstanceID) {
		if tm != nil {
			motions = append(motions, tm)
		}
	}
	s.motionMu.Unlock()
	for _, tm := range motions {
		if inCylinder(tm.X, tm.Y, tm.Z) {
			targets[tm.GUID] = struct{}{}
		}
	}
	return targets
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
	// Object::BuildMovementUpdate (Object.cpp:354-378), UPDATEFLAG_POSITION
	// leg: a transport passenger carries the transport's packed GUID
	// (DynamicObject::Create sets it via Transport::AddPassenger,
	// DynamicObject.cpp:114-126) and the passenger offset in place of
	// the repeated world position, so the client re-derives the patch's
	// world position from the transport as it moves. The offset is the
	// dynobjCaptureTransport value; the first position stays the world
	// position dynobjFollowTransport keeps current.
	if object.TransportGUID != 0 {
		block.WritePackedGUID(object.TransportGUID)
	} else {
		block.WriteU8(0)
	}
	block.WriteF32(object.X)
	block.WriteF32(object.Y)
	block.WriteF32(object.Z)
	if object.TransportGUID != 0 {
		block.WriteF32(object.TransportOffsetX)
		block.WriteF32(object.TransportOffsetY)
		block.WriteF32(object.TransportOffsetZ)
	} else {
		block.WriteF32(object.X)
		block.WriteF32(object.Y)
		block.WriteF32(object.Z)
	}
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

func (s *Server) findDynamicSpellObject(guid uint64) *dynamicSpellObjectState {
	if s == nil || guid == 0 {
		return nil
	}
	s.objectsMu.RLock()
	defer s.objectsMu.RUnlock()
	return s.dynamicSpellObjects[guid]
}

func (s *Server) spawnDynamicSpellObject(object *dynamicSpellObjectState, duration time.Duration) {
	if s == nil || object == nil || duration <= 0 {
		return
	}
	// DynamicObject::CreateDynamicObject (DynamicObject.cpp:105-112):
	// an invalid position (non-finite or out-of-bounds coordinate)
	// rejects the object outright — the patch is never created.
	if !validTrinityMapCoordinates(object.X, object.Y, object.Z, object.Orientation) {
		return
	}
	if caster := s.findSessionByGUID(object.CasterGUID); caster != nil && caster.player != nil {
		object.Map, object.InstanceID = caster.player.Map, caster.player.InstanceID
	}
	// DynamicObject::Create takes the caster's phase (DynamicObject.cpp);
	// the dynobj's own SendMessageToSet fan-out keys on it.
	if caster := s.findSessionByGUID(object.CasterGUID); caster != nil && caster.player != nil {
		object.PhaseMask = caster.currentPlayerPhaseMask()
	} else if m := s.findCreatureMotion(object.Map, object.InstanceID, object.CasterGUID); m != nil {
		object.PhaseMask = m.PhaseMask
	}
	// DynamicObject::Create transport arm (DynamicObject.cpp:114-126):
	// a caster riding a transport spawns the dynobj as a passenger — the
	// offset is computed from the transport's current position and the
	// object is added to the transport before it enters the map. Go has
	// no passenger registry for dynobjs; the stored GUID + offset is the
	// AddPassenger analog, and dynobjFollowTransport re-derives the
	// world position from it on every 500ms pass.
	s.dynobjCaptureTransport(object)
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
		if object.PhaseMask != 0 {
			s.broadcastToVisibleSetFromPhase(packet.Opcode, packet.Payload.Bytes(), object.Map, object.InstanceID, object.X, object.Y, object.Z, object.PhaseMask)
		} else {
			s.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), object.Map, object.InstanceID, object.X, object.Y, object.Z, nil)
		}
	}
}

// dynobjCaptureTransport records the DynamicObject::Create transport arm
// (DynamicObject.cpp:114-126) on a dynobj at spawn: when the caster rides
// a transport, the passenger offset is computed from the transport's
// current position via CalculatePassengerOffset
// (TransportBase::CalculatePassengerOffset, VehicleDefines.h:145-156).
// A caster on no transport, or on a transport Go cannot resolve (an
// instance transport outside the continent registry), leaves the fields
// zero and the patch stays world-anchored, as before. Called before
// registration so the spawn broadcast already carries the world position
// the offset was computed from.
func (s *Server) dynobjCaptureTransport(object *dynamicSpellObjectState) {
	if s == nil || object == nil {
		return
	}
	var transportGUID uint64
	if sess := s.findSessionByGUID(object.CasterGUID); sess != nil && sess.player != nil {
		transportGUID = sess.player.TransportGUID
	} else if m := s.findCreatureMotion(object.Map, object.InstanceID, object.CasterGUID); m != nil {
		transportGUID = m.TransportGUID
	}
	if transportGUID == 0 {
		return
	}
	spawn, ok := s.transportSpawnForGUID(transportGUID)
	if !ok {
		return
	}
	object.TransportGUID = transportGUID
	object.TransportOffsetX, object.TransportOffsetY, object.TransportOffsetZ, object.TransportOffsetO = CalculatePassengerOffset(spawn.X, spawn.Y, spawn.Z, spawn.Orientation, object.X, object.Y, object.Z, object.Orientation)
}

// dynobjFollowTransport re-derives a transport passenger dynobj's world
// position from its stored passenger offset and the transport's current
// position (TransportBase::CalculatePassengerPosition, the inverse of
// the Create-time CalculatePassengerOffset). A transport that no longer
// resolves leaves the last position in place — the patch keeps ticking
// where the transport dropped it rather than despawning.
func (s *Server) dynobjFollowTransport(object *dynamicSpellObjectState) {
	if s == nil || object == nil || object.TransportGUID == 0 {
		return
	}
	spawn, ok := s.transportSpawnForGUID(object.TransportGUID)
	if !ok {
		return
	}
	object.X, object.Y, object.Z, object.Orientation = CalculatePassengerPosition(spawn.X, spawn.Y, spawn.Z, spawn.Orientation, object.TransportOffsetX, object.TransportOffsetY, object.TransportOffsetZ, object.TransportOffsetO)
}

// dynobjTargetMapInterval mirrors UPDATE_TARGET_MAP_INTERVAL
// (SpellAuras.h:43): the dynobj refreshes its target map every 500ms —
// applying the aura debuff to newcomers (DynObjAura::UpdateOwner(apply=true),
// SpellAuras.cpp:812-823) and stripping leavers (SpellAuras.cpp:663-684) —
// independent of the damage amplitude, which ticks on the dynobj's single
// shared AuraEffect timer (SpellAuras.cpp:815-843).
const dynobjTargetMapInterval = 500 * time.Millisecond

// dynobjCreatureTargetImmune mirrors the whole-spell half of the
// DynObjAura::UpdateTargetMap registration check (SpellAuras.cpp:687-695):
// a target immune to the whole spell is never registered. The per-effect
// half rides the apply funnels (applyAuraToTarget's creature branch and
// serverApplyAuraToCreatureTarget's per-effect loop); this covers the
// whole-spell half those funnels leave to hit-resolution dispatch, which
// the dynobj 500ms pass never runs. The friendly verdict is the caller's:
// the player-path tick's targets come from the enemy sweep unless the
// spell is friendly, and the creature-path tick already filtered on it.
// The dynobj caster is live at both pass sites (a resolved player session
// in updateDynamicSpellAuras, a live motion in
// updateCreatureDynobjTargetMap), so the whole-spell gate passes
// casterLive true for the per-effect 267 arm (Unit.cpp:7987).
func (s *Server) dynobjCreatureTargetImmune(ctx context.Context, spell wotlk.Spell, targetGUID uint64, mapID, instanceID uint32, friendly bool) bool {
	if s == nil {
		return false
	}
	tm := s.findCreatureMotion(mapID, instanceID, targetGUID)
	if tm == nil {
		return false
	}
	stats := s.loadCreatureStats(ctx, tm.Entry)
	key := creatureAuraKeyForMotion(tm)
	return creatureImmuneToSpell(s, key, spell, stats, s.isTotemGUID(targetGUID), nil, true, tm.Faction, func(uint32) bool { return friendly })
}

// dynobjUpkeepTick mirrors the m_timeCla arm of Aura::Update
// (SpellAuras.cpp:838-857) for the dynobj's own aura: DynamicObject::Update
// (DynamicObject.cpp:158-173) runs the aura update on the patch itself, so
// the caster pays ManaPerSecond + ManaPerSecondPerLevel * casterLevel once
// per second no matter how many targets stand in the patch, and a failed
// payment removes the aura, which despawns the whole object. Returns false
// when the dynobj must despawn. The first drain lands one second after
// spawn (Aura ctor, SpellAuras.cpp:436-437: m_timeCla starts at 1000).
func (s *Server) dynobjUpkeepTick(object *dynamicSpellObjectState, now time.Time) bool {
	if s == nil || object == nil || !auraUpkeepNeeded(object.SpellData) {
		return true
	}
	if object.NextUpkeepTick.IsZero() {
		object.NextUpkeepTick = now.Add(auraUpkeepIntervalMs * time.Millisecond)
		return true
	}
	if now.Before(object.NextUpkeepTick) {
		return true
	}
	object.NextUpkeepTick = now.Add(auraUpkeepIntervalMs * time.Millisecond)
	return s.auraUpkeepTick(uint32(object.SpellID), object.CasterGUID, object.Map, object.InstanceID)
}

func (s *Server) updateDynamicSpellAuras(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	objects := make([]*dynamicSpellObjectState, 0, len(s.dynamicSpellObjects))
	for _, object := range s.dynamicSpellObjects {
		if object == nil {
			continue
		}
		// C++ refreshes the target map every 500ms regardless of
		// periodicity (Aura::UpdateTargetMap, SpellAuras.cpp:643-767);
		// only the tick catch-up needs a non-zero period (it would
		// spin forever on a zero period).
		if object.AuraPeriodMs != 0 {
			period := time.Duration(object.AuraPeriodMs) * time.Millisecond
			for !now.Before(object.NextAuraTick) {
				object.NextAuraTick = object.NextAuraTick.Add(period)
			}
		}
		if now.Before(object.NextTargetMapPass) {
			continue
		}
		object.NextTargetMapPass = now.Add(dynobjTargetMapInterval)
		objects = append(objects, object)
	}
	s.objectsMu.Unlock()
	for _, object := range objects {
		// DynamicObject transport following (DynamicObject.cpp:114-126
		// + Transport::UpdatePassengerPositions): the patch rides the
		// transport — re-derive the world position from the stored
		// passenger offset on every 500ms pass so the radius sweep and
		// the upkeep/tick legs track the moving transport. Runs ahead
		// of the upkeep and caster gates so both the player and the
		// creature target-map passes see the followed position.
		s.dynobjFollowTransport(object)
		// DynObjAura upkeep (SpellAuras.cpp:838-857 via
		// DynamicObject::Update, DynamicObject.cpp:158-173): the m_timeCla
		// drain runs once per second on the dynobj's own aura, and a
		// failed payment despawns the whole patch with all its
		// applications — never one target's aura at a time.
		if !s.dynobjUpkeepTick(object, now) {
			s.despawnDynamicSpellObject(object.GUID)
			continue
		}
		caster := s.findSessionByGUID(object.CasterGUID)
		if caster == nil || caster.player == nil || caster.player.Map != object.Map || caster.player.InstanceID != object.InstanceID {
			// Creature-caster dynobj (creatureEffectPersistentAreaAura):
			// no player session exists by construction, so the session
			// gate above would despawn the object on its first pass.
			// A live creature motion on the object's map/instance keeps
			// it for its duration and runs the creature target-map pass
			// (newcomers gain the aura, leavers lose it); a vanished
			// caster still despawns, as before.
			if caster := s.findCreatureMotion(object.Map, object.InstanceID, object.CasterGUID); caster != nil {
				s.updateCreatureDynobjTargetMap(ctx, object, caster)
				continue
			}
			s.despawnDynamicSpellObject(object.GUID)
			continue
		}
		target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagDestLocation, Destination: protocol.SpellTargetLocation{X: object.X, Y: object.Y, Z: object.Z}}
		// DynObjAura::FillTargetMap (SpellAuras.cpp:2763-2785) picks the
		// check type per effect from its implicit targets, so the
		// target map carries a per-effect bitmask — the old whole-spell
		// sweep applied every effect to every target in one set, and a
		// DEFAULT-check effect (27085/42939 eff1 trigger, 31266 eff1,
		// 47346 eff1 — tgtA 1/0) rode the enemy sweep and never reached
		// friendlies standing in the patch. Each check class sweeps
		// only when an aura effect needs it; the friendly sweep runs
		// unchecked (a harmful spell can carry a friendly-area effect,
		// Holy Nova 15237), and the DEFAULT set is a fresh cylinder
		// sweep since neither spell sweep selects without a faction leg.
		needsEnemy, needsFriendly, needsDefault := false, false, false
		for _, ae := range object.AuraEffects {
			switch dynobjEffectCheckType(ae.Effect) {
			case dynobjCheckEnemy:
				needsEnemy = true
			case dynobjCheckAlly, dynobjCheckParty, dynobjCheckRaid, dynobjCheckRaidClass:
				needsFriendly = true
			default:
				needsDefault = true
			}
		}
		var enemySet, friendlySet, defaultSet map[uint64]struct{}
		if needsEnemy {
			enemySet = make(map[uint64]struct{})
			for _, guid := range caster.spellAreaEnemyTargets(ctx, object.SpellData, target) {
				enemySet[guid] = struct{}{}
			}
		}
		if needsFriendly {
			friendlySet = make(map[uint64]struct{})
			for _, guid := range caster.spellFriendlyAreaTargetsUnchecked(ctx, object.SpellData, target) {
				friendlySet[guid] = struct{}{}
			}
		}
		if needsDefault {
			defaultSet = s.dynobjDefaultTargets(object)
		}
		_, partyMembers, raidMembers, inGroup := caster.friendlyGroupSnapshot()
		dynobjGroupMember := func(guid uint64, members map[uint64]struct{}) bool {
			if _, ok := members[guid]; ok {
				return true
			}
			if m := s.findCreatureMotion(object.Map, object.InstanceID, guid); m != nil {
				owner := m.OwnerGUID
				if owner == 0 {
					owner = m.CharmOwnerGUID
				}
				_, ok := members[owner]
				return ok
			}
			return false
		}
		inParty := func(guid uint64) bool {
			if guid == caster.playerGUID {
				return true
			}
			return inGroup && dynobjGroupMember(guid, partyMembers)
		}
		inRaid := func(guid uint64) bool {
			if guid == caster.playerGUID {
				return true
			}
			return inGroup && dynobjGroupMember(guid, raidMembers)
		}
		masks := dynobjTargetMasks(object.AuraEffects, enemySet, friendlySet, defaultSet, inParty, inRaid)
		inRadius := make(map[uint64]struct{}, len(masks))
		for targetGUID := range masks {
			// Aura::UpdateTargetMap (SpellAuras.cpp:700-701): dynobj auras
			// don't hit flying targets.
			if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.inFlight {
				continue
			}
			inRadius[targetGUID] = struct{}{}
		}
		spellID := uint32(object.SpellID)
		// Aura::UpdateTargetMap's removal arm (SpellAuras.cpp:663-684):
		// applications whose targets left the area are removed on the 500ms
		// target-map pass, like C++.
		for holderGUID := range object.AuraHolders {
			if _, ok := masks[holderGUID]; !ok {
				// No effect selects this target anymore (left the area
				// for every effect, or the check-type split dropped it).
				s.expireDynobjHolderAura(object, holderGUID, spellID)
				delete(object.AuraHolders, holderGUID)
				delete(object.AuraHolderMasks, holderGUID)
				continue
			}
			if _, ok := inRadius[holderGUID]; !ok {
				s.expireDynobjHolderAura(object, holderGUID, spellID)
				delete(object.AuraHolders, holderGUID)
				delete(object.AuraHolderMasks, holderGUID)
				continue
			}
			if prev, hasPrev := object.AuraHolderMasks[holderGUID]; hasPrev && prev != masks[holderGUID] {
				// Aura::UpdateTargetMap (SpellAuras.cpp:672-679): an
				// application whose effect mask changed is unapplied
				// and recreated with the new mask — strip it here and
				// let the apply loop below re-apply it with the new mask.
				s.expireDynobjHolderAura(object, holderGUID, spellID)
				delete(object.AuraHolders, holderGUID)
				delete(object.AuraHolderMasks, holderGUID)
				continue
			}
			// Aura::UpdateTargetMap (SpellAuras.cpp:666-672): an application
			// still in the area is re-validated every pass — a target failing
			// CanBeAppliedOn is removed now and re-applied on a later pass
			// when it passes again. The IsInWorld half rides inRadius
			// membership; the CheckAreaTarget half is the script-handler
			// check below (a no-op true when no handler is registered).
			if !s.fireAuraCheckAreaTarget(ctx, spellID, object.CasterGUID, object.Map, object.InstanceID, holderGUID) {
				s.expireDynobjHolderAura(object, holderGUID, spellID)
				delete(object.AuraHolders, holderGUID)
				delete(object.AuraHolderMasks, holderGUID)
			}
		}
		// The newly applied aura's first tick aligns to the dynobj's shared
		// amplitude schedule (SpellAuras.cpp:815-843) instead of starting a
		// private period timer: a target entering just before a tick takes
		// damage almost immediately, and all applications tick in phase.
		// The apply below loops the target's masked effects only (the
		// FillTargetMap bitmask), primary first so the fresh record takes
		// the primary's AuraType and periodic timer; later effects merge
		// through the castMerged funnel (refreshAuraEffectBasepoints +
		// EffectMask OR) without touching the timer. Each effect's first
		// tick aligns to its own shared schedule (its own NextTick).
		for targetGUID, mask := range masks {
			if _, ok := inRadius[targetGUID]; !ok {
				continue
			}
			if _, ok := object.AuraHolders[targetGUID]; ok {
				continue
			}
			if caster.hasDynamicAreaAura(targetGUID, object.SpellData.ID) {
				continue
			}
			// Aura::UpdateTargetMap (SpellAuras.cpp:643-767) runs the
			// DoCheckAreaTarget handlers before applying a dynobj aura
			// to a newcomer; a rejection leaves the target out of the
			// target map (and out of AuraHolders below), so the next
			// pass re-checks it.
			if !s.fireAuraCheckAreaTarget(ctx, spellID, object.CasterGUID, object.Map, object.InstanceID, targetGUID) {
				continue
			}
			// DynObjAura::UpdateTargetMap (SpellAuras.cpp:687-695) never
			// registers a whole-spell immune target; the per-effect half
			// rides applyAuraToTarget, but the whole-spell half is left
			// to hit-resolution dispatch there, which the 500ms pass
			// never runs. Player targets already get both gates inside
			// applyAuraToTarget, so only creature targets need the check
			// here; targets come from the enemy sweep unless the spell
			// is friendly.
			if s.findSessionByGUID(targetGUID) == nil && s.dynobjCreatureTargetImmune(ctx, object.SpellData, targetGUID, object.Map, object.InstanceID, isFriendlyAreaSpell(object.SpellData)) {
				continue
			}
			// The masked primary follows dynobjPrimaryEffect's rule
			// restricted to the target's mask: first periodic effect in
			// the mask, else the first masked effect — a leading dummy
			// outside the mask must not own the record's timer.
			var primary *dynobjAuraEffect
			for i := range object.AuraEffects {
				ae := &object.AuraEffects[i]
				if mask&(1<<uint(ae.Index)) == 0 {
					continue
				}
				if primary == nil {
					primary = ae
				}
				if ae.PeriodMs > 0 {
					primary = ae
					break
				}
			}
			if primary != nil {
				castMerged := make(map[uint64]struct{})
				applyOne := func(ae dynobjAuraEffect) {
					sharedDelayMs := uint32(0)
					if d := ae.NextTick.Sub(now); d > 0 {
						sharedDelayMs = uint32(d / time.Millisecond)
					}
					caster.applyAuraToTarget(ctx, targetGUID, object.SpellData, ae.Effect, ae.Index, object.AuraDurationMs, ae.PeriodMs, ae.Amount, uint32(object.AuraSchoolMask), castMerged, false, object.CasterGUID, true, 0, false, &sharedDelayMs)
				}
				applyOne(*primary)
				for _, ae := range object.AuraEffects {
					if ae.Index == primary.Index || mask&(1<<uint(ae.Index)) == 0 {
						continue
					}
					applyOne(ae)
				}
				// refreshAuraEffectBasepoints sets the record's scalar
				// Amount to the last merged effect's amount, but the
				// periodic tick reads it for the primary effect — restore
				// it so a trailing slow/dummy never zeroes the damage.
				if targetSess := caster.server.findSessionByGUID(targetGUID); targetSess != nil {
					targetSess.restoreDynobjPrimaryAmount(uint32(object.SpellID), object.CasterGUID, primary.Amount)
				}
			}
			if object.AuraHolders == nil {
				object.AuraHolders = make(map[uint64]struct{})
			}
			object.AuraHolders[targetGUID] = struct{}{}
			if object.AuraHolderMasks == nil {
				object.AuraHolderMasks = make(map[uint64]uint8)
			}
			object.AuraHolderMasks[targetGUID] = mask
		}
	}
	// Caster-owned area auras (area_auras.go) share this 100ms tick's
	// 500ms target-map pass shape: newcomers gain the aura, leavers
	// lose it (Aura::UpdateTargetMap, SpellAuras.cpp:643-767).
	s.updateAreaAuras(ctx, now)
}

// updateCreatureDynobjTargetMap runs the 500ms target-map pass for a
// creature-caster dynobj (creatureEffectPersistentAreaAura): newcomers
// inside the radius gain the aura, leavers lose it (Aura::UpdateTargetMap,
// SpellAuras.cpp:643-767), mirroring the session pass above. Cast-time
// applications are not seeded into AuraHolders, so a holder already
// carrying the aura is adopted into the map without re-applying (a
// re-apply would refresh its duration every pass); a holder in the map
// whose aura is gone is applied fresh. The gates mirror the cast-time
// loop: in-flight players skip, the dead gate rides the spell's passive /
// CAN_TARGET_DEAD flags, and creature targets ride
// serverApplyAuraToCreatureTarget's own dead/evade gates.
func (s *Server) updateCreatureDynobjTargetMap(ctx context.Context, object *dynamicSpellObjectState, caster *creatureMotion) {
	if s == nil || object == nil || caster == nil {
		return
	}
	spell := object.SpellData
	spellID := uint32(object.SpellID)
	// DynObjAura::FillTargetMap (SpellAuras.cpp:2763-2785) selects per
	// effect, so the pass keeps one candidate set per check class and
	// lets dynobjTargetMasks split them — the old single friendly
	// verdict (isFriendlyAreaSpell, whole-spell) applied every effect
	// to every target in one set, so a DEFAULT-check effect (27085/
	// 42939 eff1 trigger, 31266 eff1, 47346 eff1 — tgtA 1/0) never
	// reached friendlies in a hostile patch, and a friendly effect of
	// a harmful spell never reached allies.
	enemySet := make(map[uint64]struct{})
	friendlySet := make(map[uint64]struct{})
	defaultSet := make(map[uint64]struct{})
	playerTargets := make(map[uint64]*session)
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess != nil && sess.player != nil && sess.player.Map == object.Map && sess.player.InstanceID == object.InstanceID {
			players = append(players, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, vs := range players {
		if vs.player == nil || vs.inFlight {
			continue
		}
		if distance3D(vs.player.X, vs.player.Y, vs.player.Z, object.X, object.Y, object.Z) > float64(object.Radius) {
			continue
		}
		if vs.player.Health == 0 && spell.Attributes&spellAttr0Passive == 0 && spell.AttributesEx1&spellAttr2CanTargetDead == 0 {
			continue
		}
		defaultSet[vs.playerGUID] = struct{}{}
		if s.creatureFriendlyToTarget(caster, vs.playerGUID) {
			friendlySet[vs.playerGUID] = struct{}{}
		} else {
			enemySet[vs.playerGUID] = struct{}{}
		}
		playerTargets[vs.playerGUID] = vs
	}
	type creatureTarget struct {
		guid    uint64
		x, y, z float32
	}
	s.motionMu.Lock()
	var creatureTargets []creatureTarget
	for _, tm := range s.motionMapLocked(object.Map, object.InstanceID) {
		if tm != nil {
			creatureTargets = append(creatureTargets, creatureTarget{guid: tm.GUID, x: tm.X, y: tm.Y, z: tm.Z})
		}
	}
	s.motionMu.Unlock()
	for _, tm := range creatureTargets {
		if distance3D(tm.x, tm.y, tm.z, object.X, object.Y, object.Z) > float64(object.Radius) {
			continue
		}
		defaultSet[tm.guid] = struct{}{}
		if s.creatureFriendlyToTarget(caster, tm.guid) {
			friendlySet[tm.guid] = struct{}{}
		} else {
			enemySet[tm.guid] = struct{}{}
		}
	}
	// PARTY/RAID membership resolves through the caster's owner
	// (Unit::IsInPartyWith, Unit.cpp:12126): a wild caster has no group,
	// so only itself passes; a pet/charmed caster's owner's group decides.
	ownerGUID := caster.OwnerGUID
	if ownerGUID == 0 {
		ownerGUID = caster.CharmOwnerGUID
	}
	var partyMembers, raidMembers map[uint64]struct{}
	var inGroup bool
	if ownerSess := s.findSessionByGUID(ownerGUID); ownerSess != nil {
		_, partyMembers, raidMembers, inGroup = ownerSess.friendlyGroupSnapshot()
	}
	memberOf := func(guid uint64, members map[uint64]struct{}) bool {
		if _, ok := members[guid]; ok {
			return true
		}
		if m := s.findCreatureMotion(object.Map, object.InstanceID, guid); m != nil {
			owner := m.OwnerGUID
			if owner == 0 {
				owner = m.CharmOwnerGUID
			}
			_, ok := members[owner]
			return ok
		}
		return false
	}
	inParty := func(guid uint64) bool {
		if guid == caster.GUID {
			return true
		}
		return inGroup && memberOf(guid, partyMembers)
	}
	inRaid := func(guid uint64) bool {
		if guid == caster.GUID {
			return true
		}
		return inGroup && memberOf(guid, raidMembers)
	}
	masks := dynobjTargetMasks(object.AuraEffects, enemySet, friendlySet, defaultSet, inParty, inRaid)
	for holderGUID := range object.AuraHolders {
		if _, ok := masks[holderGUID]; !ok {
			s.expireDynobjHolderAura(object, holderGUID, spellID)
			delete(object.AuraHolders, holderGUID)
			delete(object.AuraHolderMasks, holderGUID)
			continue
		}
		if prev, hasPrev := object.AuraHolderMasks[holderGUID]; hasPrev && prev != masks[holderGUID] {
			// Aura::UpdateTargetMap (SpellAuras.cpp:672-679): an
			// application whose effect mask changed is unapplied and
			// recreated with the new mask — strip it here and let the
			// apply loop below re-apply it with the new mask.
			s.expireDynobjHolderAura(object, holderGUID, spellID)
			delete(object.AuraHolders, holderGUID)
			delete(object.AuraHolderMasks, holderGUID)
			continue
		}
		// Aura::UpdateTargetMap (SpellAuras.cpp:666-672): an application
		// still in the area is re-validated every pass — a target failing
		// CanBeAppliedOn is removed now and re-applied on a later pass
		// when it passes again. The IsInWorld half rides inRadius
		// membership; the CheckAreaTarget half is the script-handler
		// check below (a no-op true when no handler is registered).
		if !s.fireAuraCheckAreaTarget(ctx, spellID, object.CasterGUID, object.Map, object.InstanceID, holderGUID) {
			s.expireDynobjHolderAura(object, holderGUID, spellID)
			delete(object.AuraHolders, holderGUID)
			delete(object.AuraHolderMasks, holderGUID)
		}
	}
	if object.AuraHolders == nil {
		object.AuraHolders = make(map[uint64]struct{})
	}
	// The newly applied aura's first tick aligns to the dynobj's shared
	// amplitude schedule (SpellAuras.cpp:815-843) instead of starting a
	// private period timer: a target entering just before a tick takes
	// damage almost immediately, and all applications tick in phase —
	// the player-path twin in updateDynamicSpellAuras.
	sharedDelayMs := uint32(0)
	if d := object.NextAuraTick.Sub(time.Now()); d > 0 {
		sharedDelayMs = uint32(d / time.Millisecond)
	}
	for guid, mask := range masks {
		if _, ok := object.AuraHolders[guid]; ok {
			continue
		}
		if s.creatureDynobjAuraPresent(object, guid) {
			// The holder-loop re-validation arm above (SpellAuras.cpp:666-672)
			// runs for adopted holders on later passes; a cast-time holder
			// is re-validated here before adoption, so a verdict that
			// flipped between the cast-time fill and the first pass strips
			// the aura instead of adopting it for another pass.
			if !s.fireAuraCheckAreaTarget(ctx, spellID, object.CasterGUID, object.Map, object.InstanceID, guid) {
				s.expireDynobjHolderAura(object, guid, spellID)
				continue
			}
			object.AuraHolders[guid] = struct{}{}
			if object.AuraHolderMasks == nil {
				object.AuraHolderMasks = make(map[uint64]uint8)
			}
			object.AuraHolderMasks[guid] = mask
			continue
		}
		// Aura::UpdateTargetMap (SpellAuras.cpp:643-767) runs the
		// DoCheckAreaTarget handlers before applying a dynobj aura
		// to a newcomer; a rejection leaves the target out of the
		// target map (and out of AuraHolders below), so the next
		// pass re-checks it.
		if !s.fireAuraCheckAreaTarget(ctx, spellID, object.CasterGUID, object.Map, object.InstanceID, guid) {
			continue
		}
		// DynObjAura::UpdateTargetMap (SpellAuras.cpp:687-695) never
		// registers an immune target: whole-spell immunity blocks the
		// target and per-effect immunity strips that effect's mask bit
		// (serverApplyAuraToCreatureTarget loops every aura effect and
		// skips the immune ones, so the record keeps the rest). A
		// skipped target is not registered, so the next pass re-checks
		// it. The friendly verdict rides the creature-caster helpers,
		// so the school-immunity exemption (Unit.cpp:7909) applies to
		// a positive spell on a friendly target while the
		// ID/dispel/mechanic/all-effects arms still block it, as in C++.
		if vs, ok := playerTargets[guid]; ok && vs != nil {
			targetFriendly := s.creatureFriendlyToTarget(caster, guid)
			if vs.isImmuneToSpellCreatureCaster(spell, targetFriendly) {
				continue
			}
			// No per-effect pre-check here: applyAuraFromCaster strips
			// each immune effect from the mask internally
			// (Unit::AddAura, Unit.cpp:12291-12297), so a target immune
			// to only one of the dynobj's effects keeps the rest — the
			// Aura::UpdateTargetMap mask-strip arm (SpellAuras.cpp:687-695).
			// The FillTargetMap mask rides allowedMask, so a recipient
			// selected by one effect takes only that effect's bit.
			vs.applyAuraFromCaster(spellID, object.AuraDurationMs, caster.GUID, uint8(caster.Level), object.AuraPeriodMs, &sharedDelayMs, true, false, mask)
		} else {
			if s.dynobjCreatureTargetImmune(ctx, spell, guid, object.Map, object.InstanceID, s.creatureFriendlyToTarget(caster, guid)) {
				continue
			}
			s.serverApplyAuraToCreatureTarget(ctx, caster, spell, object.AuraDurationMs, guid, nil, true, &sharedDelayMs, mask, false, false)
		}
		object.AuraHolders[guid] = struct{}{}
		if object.AuraHolderMasks == nil {
			object.AuraHolderMasks = make(map[uint64]uint8)
		}
		object.AuraHolderMasks[guid] = mask
	}
}

// creatureDynobjAuraPresent reports whether guid already carries the
// dynobj's aura from this caster — the map-membership check the
// target-map pass needs when AuraHolders was not seeded at cast time.
func (s *Server) creatureDynobjAuraPresent(object *dynamicSpellObjectState, guid uint64) bool {
	if s == nil || object == nil || guid == 0 {
		return false
	}
	spellID := uint32(object.SpellID)
	if ts := s.findSessionByGUID(guid); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		aura := ts.activeAuras[spellID]
		ts.castMu.Unlock()
		return aura != nil && !aura.Stopped && aura.CasterGUID == object.CasterGUID && aura.PersistentAreaAura
	}
	key := creatureAuraKey{Map: object.Map, InstanceID: object.InstanceID, GUID: guid}
	s.auraMu.Lock()
	aura := s.activeCreatureAuras[key][spellID]
	s.auraMu.Unlock()
	return aura != nil && !aura.Stopped && aura.CasterGUID == object.CasterGUID && aura.PersistentAreaAura
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
	var wasCharm, wasTaunt, wasDetaunt, wasPossessPet bool
	var charmerGUID, taunterGUID, detaunterGUID uint64
	var slot uint8
	s.auraMu.Lock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil &&
				!aura.Stopped && aura.CasterGUID == casterGUID && aura.PersistentAreaAura {
				wasCharm = aura.AuraType == spellAuraCharm || aura.AuraType == spellAuraAoeCharm || aura.AuraType == spellAuraModPossess
				wasPossessPet = aura.AuraType == spellAuraModPossessPet
				charmerGUID = aura.CasterGUID
				wasTaunt = aura.AuraType == spellAuraModTaunt
				taunterGUID = aura.CasterGUID
				wasDetaunt = aura.AuraType == spellAuraModDetaunt
				detaunterGUID = aura.CasterGUID
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
	if wasDetaunt {
		s.clearCreatureDetaunt(key.Map, key.InstanceID, detaunterGUID, key.GUID)
	}
	if wasCharm {
		s.uncharmCreature(key, charmerGUID)
		if charmer := s.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}
	if wasPossessPet {
		s.unpossessPetTail(key, charmerGUID)
	}
	removePkt := protocol.BuildAuraUpdate(key.GUID, 0, slot, 0, true, false, 0, 0, 1)
	s.broadcastToVisibleSetForCreature(uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, key.Map, key.InstanceID, key.GUID)
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
			// SetViewpoint false arm (Player.cpp:24638-24652): the seer
			// returns to the caster — re-stream the body neighbourhood
			// (Go has no visibility tick to do it on the next pass).
			caster.streamNearbyObjects(context.Background())
		}
	}
	// DynamicObject::Remove (DynamicObject.cpp:176-182) despawns through
	// WorldObject::SendObjectDeSpawnAnim (Object.cpp:1826-1831):
	// SMSG_GAMEOBJECT_DESPAWN_ANIM carrying only the GUID — never
	// SMSG_DESTROY_OBJECT.
	packet := protocol.NewBuffer(8)
	packet.WriteU64(object.GUID)
	if object.PhaseMask != 0 {
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), packet.Bytes(), object.Map, object.InstanceID, object.X, object.Y, object.Z, object.PhaseMask)
	} else {
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), packet.Bytes(), object.Map, object.InstanceID, object.X, object.Y, object.Z, nil)
	}
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

// restoreDynobjPrimaryAmount resets a dynobj aura record's scalar Amount
// to the primary effect's amount. A multi-effect apply loop merges each
// later effect through refreshAuraEffectBasepoints, which leaves the
// scalar on the last merged effect — but the periodic tick reads the
// scalar for the primary effect, so a trailing slow or dummy would zero
// or replace the damage. Amounts[] per index are already exact.
func (s *session) restoreDynobjPrimaryAmount(spellID uint32, casterGUID uint64, amount uint32) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	aura := s.activeAuras[spellID]
	if aura == nil || aura.Stopped || aura.CasterGUID != casterGUID || !aura.PersistentAreaAura {
		return
	}
	aura.Amount = amount
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

// despawnCasterDynObjects mirrors the dynobj arm of Spell::cancel
// (Spell.cpp:3251-3255): m_originalCaster->RemoveDynObject(m_spellInfo->Id)
// sweeps every dynamic object the caster owns for the cancelled spell.
// Interrupting a channeled persistent-area spell (Blizzard, Hurricane,
// Rain of Fire) despawns its patch; interrupting a recast mid-cast also
// kills the previous cast's patch — RemoveDynObject has no channeled
// gate. Farsight-focus objects are included: DynamicObject::AddToWorld
// binds them to the caster (DynamicObject.cpp:59), so the C++ sweep
// reaches them too. Natural channel end keeps the object for its aura
// duration (Spell::finish has no such arm), so only the interrupt paths
// call this.
func (s *Server) despawnCasterDynObjects(casterGUID uint64, spellID uint32) {
	if s == nil || casterGUID == 0 || spellID == 0 {
		return
	}
	s.objectsMu.Lock()
	guids := make([]uint64, 0, 2)
	for _, object := range s.dynamicSpellObjects {
		if object != nil && object.CasterGUID == casterGUID && object.SpellID == uint64(spellID) {
			guids = append(guids, object.GUID)
		}
	}
	s.objectsMu.Unlock()
	for _, guid := range guids {
		s.despawnDynamicSpellObject(guid)
	}
}

// despawnAllCasterDynObjects mirrors Unit::RemoveAllDynObjects
// (Unit.cpp:5193-5197), reached at logout via Unit::RemoveFromWorld
// (Unit.cpp:9795-9796): every dynamic object the leaving unit owns is
// removed, not just the current spell's. Without this, persistent-area
// patches (Blizzard etc.) keep affecting players after the caster
// logs out.
func (s *Server) despawnAllCasterDynObjects(casterGUID uint64) {
	if s == nil || casterGUID == 0 {
		return
	}
	s.objectsMu.Lock()
	guids := make([]uint64, 0, 2)
	for _, object := range s.dynamicSpellObjects {
		if object != nil && object.CasterGUID == casterGUID {
			guids = append(guids, object.GUID)
		}
	}
	s.objectsMu.Unlock()
	for _, guid := range guids {
		s.despawnDynamicSpellObject(guid)
	}
}

func (s *session) streamDynamicSpellObjects() {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	s.streamDynamicSpellObjectsAt(s.player.X, s.player.Y)
}

func (s *session) streamDynamicSpellObjectsAt(x, y float32) {
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
		if object == nil || object.Map != s.player.Map || object.InstanceID != s.player.InstanceID || math.Hypot(float64(object.X-x), float64(object.Y-y)) > distance {
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

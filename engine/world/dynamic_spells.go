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
	Radius                    float32
	CastTime                  uint32
	ObjectType                uint8 // DYNAMICOBJECT_BYTES type: area spell vs farsight focus
	IsFarsightFocus           bool  // SetCasterViewpoint viewpoint; cleared on despawn
	SpellData                 wotlk.Spell
	AuraEffect                wotlk.SpellEffect
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
		areaTargets := caster.spellAreaEnemyTargets(ctx, object.SpellData, target)
		if isFriendlyAreaSpell(object.SpellData) {
			// DynObjAura::FillTargetMap (SpellAuras.cpp:2763) picks the
			// check type per effect from its implicit targets; the enemy
			// path above only covers TARGET_CHECK_ENEMY, so friendly
			// persistent-area auras route through the ALLY/PARTY/RAID path.
			areaTargets = caster.spellFriendlyAreaTargets(ctx, object.SpellData, target)
		}
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
		// applications whose targets left the area are removed on the 500ms
		// target-map pass, like C++.
		for holderGUID := range object.AuraHolders {
			if _, ok := inRadius[holderGUID]; !ok {
				s.expireDynobjHolderAura(object, holderGUID, spellID)
				delete(object.AuraHolders, holderGUID)
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
			}
		}
		// The newly applied aura's first tick aligns to the dynobj's shared
		// amplitude schedule (SpellAuras.cpp:815-843) instead of starting a
		// private period timer: a target entering just before a tick takes
		// damage almost immediately, and all applications tick in phase.
		// The apply below loops the dynobj's whole effect list (the
		// MAX_EFFECT_MASK aura), primary first so the fresh record takes
		// the primary's AuraType and periodic timer; later effects merge
		// through the castMerged funnel (refreshAuraEffectBasepoints +
		// EffectMask OR) without touching the timer. Each effect's first
		// tick aligns to its own shared schedule (its own NextTick).
		primary := dynobjPrimaryEffect(object.AuraEffects)
		for _, targetGUID := range areaTargets {
			if _, ok := inRadius[targetGUID]; !ok {
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
					if ae.Index == primary.Index {
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
	friendly := isFriendlyAreaSpell(spell)
	inRadius := make(map[uint64]struct{})
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
		if friendly != s.creatureFriendlyToTarget(caster, vs.playerGUID) {
			continue
		}
		if vs.player.Health == 0 && spell.Attributes&spellAttr0Passive == 0 && spell.AttributesEx1&spellAttr2CanTargetDead == 0 {
			continue
		}
		inRadius[vs.playerGUID] = struct{}{}
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
		if friendly != s.creatureFriendlyToTarget(caster, tm.guid) {
			continue
		}
		inRadius[tm.guid] = struct{}{}
	}
	for holderGUID := range object.AuraHolders {
		if _, ok := inRadius[holderGUID]; !ok {
			s.expireDynobjHolderAura(object, holderGUID, spellID)
			delete(object.AuraHolders, holderGUID)
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
	for guid := range inRadius {
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
			vs.applyAuraFromCaster(spellID, object.AuraDurationMs, caster.GUID, uint8(caster.Level), object.AuraPeriodMs, &sharedDelayMs, true, false)
		} else {
			if s.dynobjCreatureTargetImmune(ctx, spell, guid, object.Map, object.InstanceID, s.creatureFriendlyToTarget(caster, guid)) {
				continue
			}
			s.serverApplyAuraToCreatureTarget(ctx, caster, spell, object.AuraDurationMs, guid, nil, true, &sharedDelayMs)
		}
		object.AuraHolders[guid] = struct{}{}
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

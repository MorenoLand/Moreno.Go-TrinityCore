package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// Caster-owned area auras (UnitAura::FillTargetMap, SpellAuras.cpp:2649-2762)
// for creature and player casters.
//
// A spell whose effects are SPELL_EFFECT_APPLY_AREA_AURA_PARTY (35),
// RAID (65), PET (119), FRIEND (128), ENEMY (129) or OWNER (143) is
// EffectUnused in the C++ effect table (SpellEffects.cpp:104/134/188/
// 197/198/212) — there is no HIT leg. The aura is created on the caster
// by Spell::DoSpellEffectHit (Spell.cpp:2840-2872, the caster is the
// spell's only hit target: tgtA=1 TARGET_UNIT_CASTER on every witness
// row) and its recipients come from UnitAura::FillTargetMap: a radius
// search around the caster per effect set, re-run by Aura::UpdateTargetMap
// (SpellAuras.cpp:643-767) every UPDATE_TARGET_MAP_INTERVAL (500ms,
// SpellAuras.h:43) until the aura expires or the caster dies. The
// creature path used to gate creatureEffectApplyAura on effect 6 only,
// so creature-cast area auras never landed at all; widening that gate
// would direct-apply e.g. 64227's periodic-damage aura (aura 3) to the
// caster itself, so the fan-out below mirrors the C++ owner model
// instead: the aura lives on the caster, recipients are selected by
// radius + relation, and the 500ms pass adopts late joiners and strips
// leavers.
//
// The player triggered-cast path (castSpellDirectWithOverrides) and the
// client path (finishSpellCast/applyEffects, registerClientAreaAura hook)
// both register the fan-out and strip area effects from their hit loops;
// the 500ms pass re-selects after. The pet funnel fans 35/65/128/129 out
// to its area targets (pet_combat.go:6843; 65/128 ride the effect-id ally
// legs, petAreaAllyWantedMaskForEffect) and 119/143 through the
// radius-gated owner leg.
//
// KNOWN DELTA (single-record model, shared with the player path and the
// dynobj path): the fan-out applies the whole spell record, so on a mixed
// spell (31673: eff0/eff1 effect-6 + eff2 area) area recipients also carry
// the effect-6 bits — C++ FillTargetMap registers only the area effects'
// bits for them (SpellAuras.cpp:2760-2761). Per-effect masks would need a
// new param through both apply funnels; a separate unit.
type areaAuraSelection uint8

const (
	areaAuraParty areaAuraSelection = iota
	areaAuraRaid
	areaAuraFriend
	areaAuraEnemy
	areaAuraPet
	areaAuraOwner
)

func areaAuraSelectionForEffect(effect uint32) (areaAuraSelection, bool) {
	switch effect {
	case spellEffectApplyAreaAuraParty:
		return areaAuraParty, true
	case spellEffectApplyAreaAuraRaid:
		return areaAuraRaid, true
	case spellEffectApplyAreaAuraPet:
		return areaAuraPet, true
	case spellEffectApplyAreaAuraFriend:
		return areaAuraFriend, true
	case spellEffectApplyAreaAuraEnemy:
		return areaAuraEnemy, true
	case spellEffectApplyAreaAuraOwner:
		return areaAuraOwner, true
	default:
		return 0, false
	}
}

// spellHasAreaAuraEffect mirrors SpellEffectInfo::IsAreaAuraEffect
// (SpellInfo.cpp:385-395) restricted to effects carrying a real aura:
// true when any effect of the spell is an area-aura effect.
func spellHasAreaAuraEffect(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Aura == 0 {
			continue
		}
		if _, ok := areaAuraSelectionForEffect(eff.Effect); ok {
			return true
		}
	}
	return false
}

type areaAuraKey struct {
	casterGUID uint64
	spellID    uint32
}

type areaAuraEffect struct {
	index     int
	effect    wotlk.SpellEffect
	selection areaAuraSelection
	radius    float32
}

// areaAuraState is one live caster-owned area aura: the C++
// UnitAura owned by the caster, with the recipient set FillTargetMap
// last selected. Holders carry the aura as a PersistentAreaAura
// application (same flag the dynobj pass uses), so the 500ms pass can
// strip leavers and expiry can strip everyone without touching a
// same-spell aura from another source.
type areaAuraState struct {
	key               areaAuraKey
	spell             wotlk.Spell
	mapID, instanceID uint32
	durationMs        uint32
	expiresAt         time.Time // zero when durationMs is 0: permanent until the caster dies/despawns
	nextTargetMapPass time.Time
	effects           []areaAuraEffect
	holders           map[uint64]struct{}
}

// areaAuraOwnerGUID mirrors Unit::GetCharmerOrOwner (Unit.h): the
// charmer when the caster is charmed, else the owner. 0 for a wild caster.
func areaAuraOwnerGUID(m *creatureMotion) uint64 {
	if m == nil {
		return 0
	}
	if m.CharmerGUID != 0 {
		return m.CharmerGUID
	}
	return m.OwnerGUID
}

// areaAuraInPartyWith mirrors Unit::IsInPartyWith / IsInRaidWith
// (Unit.cpp:12126-12156, 12155-12184) for a creature caster: self is
// always in, the charmer-or-owner collapses both sides (a pet is in
// party with its owner even groupless), two players share a groupID,
// and two creatures share a faction. The TREAT_AS_RAID_UNIT carve-out
// has no Go model and stays out. Party and raid collapse to the one
// Go groupID; the distinction (same-group vs same-raid) is latent.
func (s *Server) areaAuraInPartyWith(caster *creatureMotion, targetGUID uint64) bool {
	if s == nil || caster == nil || targetGUID == 0 {
		return false
	}
	if targetGUID == caster.GUID {
		return true
	}
	refGUID := caster.GUID
	var refSess *session
	if ownerGUID := areaAuraOwnerGUID(caster); ownerGUID != 0 {
		refGUID = ownerGUID
		refSess = s.findSessionByGUID(ownerGUID)
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		if targetGUID == refGUID {
			return true
		}
		if refSess != nil && ts.groupID != 0 && ts.groupID == refSess.groupID {
			return true
		}
		return false
	}
	tm := s.findCreatureMotion(caster.Map, caster.InstanceID, targetGUID)
	if tm == nil {
		return false
	}
	if targetRef := areaAuraOwnerGUID(tm); targetRef != 0 && targetRef == refGUID {
		return true
	}
	if targetGUID == refGUID {
		return true
	}
	if refSess != nil {
		return false
	}
	return tm.Faction == caster.Faction
}

// areaAuraFillTargets runs UnitAura::FillTargetMap (SpellAuras.cpp:2649-
// 2762) for one pass: the union over the aura's area effects of the
// radius selection around the caster. PARTY/RAID/FRIEND/ENEMY scan the
// live sessions and creature motions on the caster's map/instance; PET
// is the caster itself plus its owner in radius; OWNER is the owner in
// radius. The C++ WorldObjectSpellAreaTargetCheck gates (Spell.cpp:8337-
// 8374) ride the Go twins: IsValidAssistTarget is creatureFriendlyToTarget
// (AND-ed into PARTY/RAID/FRIEND), IsValidAttackTarget is !friendly, the
// totem exclusion applies to every selection, and the dead gate is the
// dynobj pass's passive/CAN_TARGET_DEAD leg. No phase gate: the dynobj
// pass is phase-blind too. No in-flight skip: only DynObjAura::UpdateTargetMap
// (SpellAuras.cpp:700-701) excludes flying targets, not UnitAura.
func (s *Server) areaAuraFillTargets(ctx context.Context, state *areaAuraState, caster *creatureMotion) map[uint64]struct{} {
	targets := make(map[uint64]struct{})
	if s == nil || state == nil || caster == nil {
		return targets
	}
	spell := state.spell
	deadOK := spell.Attributes&spellAttr0Passive != 0 || spell.AttributesEx1&spellAttr2CanTargetDead != 0
	inRadius := func(x, y, z float32, radius float32) bool {
		return distance3D(x, y, z, caster.X, caster.Y, caster.Z) <= float64(radius)
	}
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess != nil && sess.player != nil && sess.player.Map == state.mapID && sess.player.InstanceID == state.instanceID {
			players = append(players, sess)
		}
	}
	s.sessionsMu.RUnlock()
	type creatureTarget struct {
		guid    uint64
		x, y, z float32
		health  uint32
	}
	s.motionMu.Lock()
	var creatures []creatureTarget
	for _, tm := range s.motionMapLocked(state.mapID, state.instanceID) {
		if tm != nil {
			creatures = append(creatures, creatureTarget{guid: tm.GUID, x: tm.X, y: tm.Y, z: tm.Z, health: tm.Health})
		}
	}
	s.motionMu.Unlock()
	for _, ae := range state.effects {
		switch ae.selection {
		case areaAuraPet:
			// SpellAuras.cpp:2703-2707: the owner itself is pushed
			// unconditionally (it sits at distance 0 from itself),
			// then falls through to the OWNER arm for the charmer/owner.
			targets[caster.GUID] = struct{}{}
			fallthrough
		case areaAuraOwner:
			// SpellAuras.cpp:2707-2713: the charmer/owner within radius.
			ownerGUID := areaAuraOwnerGUID(caster)
			if ownerGUID == 0 {
				continue
			}
			if vs := s.findSessionByGUID(ownerGUID); vs != nil && vs.player != nil {
				if vs.player.Map == state.mapID && vs.player.InstanceID == state.instanceID && inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) && (deadOK || vs.player.Health != 0) {
					targets[ownerGUID] = struct{}{}
				}
				continue
			}
			if tm := s.findCreatureMotion(state.mapID, state.instanceID, ownerGUID); tm != nil && inRadius(tm.X, tm.Y, tm.Z, ae.radius) && (deadOK || tm.Health != 0) {
				targets[ownerGUID] = struct{}{}
			}
		case areaAuraParty, areaAuraRaid:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if !s.creatureFriendlyToTarget(caster, vs.playerGUID) || !s.areaAuraInPartyWith(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) || s.isTotemGUID(tm.guid) {
					continue
				}
				if !s.creatureFriendlyToTarget(caster, tm.guid) || !s.areaAuraInPartyWith(caster, tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		case areaAuraFriend:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if !s.creatureFriendlyToTarget(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) || s.isTotemGUID(tm.guid) {
					continue
				}
				if !s.creatureFriendlyToTarget(caster, tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		case areaAuraEnemy:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if s.creatureFriendlyToTarget(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) {
					continue
				}
				if s.creatureFriendlyToTarget(caster, tm.guid) || s.isTotemGUID(tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		}
	}
	return targets
}

// playerAreaAuraInPartyWith mirrors Unit::IsInPartyWith (Unit.cpp:12126-12156)
// for a player caster: self is always in; the target's charmer-or-owner-or-self
// collapses a pet to its owner, so a pet of the caster or of a same-group player
// is in; two players share a groupID. A wild creature is never in a player's
// party (the TREAT_AS_RAID_UNIT carve-out has no Go model, same as the creature
// twin). Raid collapses to the same groupID, as there.
func (s *Server) playerAreaAuraInPartyWith(caster *session, targetGUID uint64) bool {
	if s == nil || caster == nil || caster.player == nil || targetGUID == 0 {
		return false
	}
	if targetGUID == caster.playerGUID {
		return true
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return caster.groupID != 0 && caster.groupID == ts.groupID
	}
	tm := s.findCreatureMotion(caster.player.Map, caster.player.InstanceID, targetGUID)
	if tm == nil {
		return false
	}
	if ownerGUID := areaAuraOwnerGUID(tm); ownerGUID != 0 {
		if ownerGUID == caster.playerGUID {
			return true
		}
		if os := s.findSessionByGUID(ownerGUID); os != nil && os.player != nil {
			return caster.groupID != 0 && caster.groupID == os.groupID
		}
		return false
	}
	return false
}

// areaAuraFillTargetsForPlayer runs UnitAura::FillTargetMap (SpellAuras.cpp:2649-
// 2762) for a player caster — the areaAuraFillTargets twin. The caster is selected
// for PARTY/RAID/FRIEND (distance 0, in party with self, friendly to self) and
// excluded for ENEMY; PET selects the caster itself, and OWNER selects nothing —
// a player has no charmer/owner in the Go model (GetCharmerOrOwner on a player is
// nil unless charmed). Totems are excluded from every selection
// (WorldObjectSpellTargetCheck, Spell.cpp:8337-8374), unlike the creature twin,
// which excludes them for ENEMY only.
func (s *Server) areaAuraFillTargetsForPlayer(ctx context.Context, state *areaAuraState, caster *session) map[uint64]struct{} {
	targets := make(map[uint64]struct{})
	if s == nil || state == nil || caster == nil || caster.player == nil {
		return targets
	}
	spell := state.spell
	deadOK := spell.Attributes&spellAttr0Passive != 0 || spell.AttributesEx1&spellAttr2CanTargetDead != 0
	inRadius := func(x, y, z float32, radius float32) bool {
		return distance3D(x, y, z, caster.player.X, caster.player.Y, caster.player.Z) <= float64(radius)
	}
	s.sessionsMu.RLock()
	var players []*session
	for sess := range s.sessions {
		if sess != nil && sess.player != nil && sess.player.Map == state.mapID && sess.player.InstanceID == state.instanceID {
			players = append(players, sess)
		}
	}
	s.sessionsMu.RUnlock()
	type creatureTarget struct {
		guid    uint64
		x, y, z float32
		health  uint32
	}
	s.motionMu.Lock()
	var creatures []creatureTarget
	for _, tm := range s.motionMapLocked(state.mapID, state.instanceID) {
		if tm != nil {
			creatures = append(creatures, creatureTarget{guid: tm.GUID, x: tm.X, y: tm.Y, z: tm.Z, health: tm.Health})
		}
	}
	s.motionMu.Unlock()
	for _, ae := range state.effects {
		switch ae.selection {
		case areaAuraPet:
			// SpellAuras.cpp:2703-2707: the owner itself is pushed
			// unconditionally, then falls through to the OWNER arm — a
			// player caster has no charmer/owner, so the fallthrough
			// selects nothing and PET is the caster alone.
			targets[caster.playerGUID] = struct{}{}
		case areaAuraOwner:
			// SpellAuras.cpp:2707-2713: the charmer/owner within radius —
			// a player caster has none in the Go model.
		case areaAuraParty, areaAuraRaid:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if !s.playerFriendlyToTarget(caster, vs.playerGUID) || !s.playerAreaAuraInPartyWith(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) || s.isTotemGUID(tm.guid) {
					continue
				}
				if !s.playerFriendlyToTarget(caster, tm.guid) || !s.playerAreaAuraInPartyWith(caster, tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		case areaAuraFriend:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if !s.playerFriendlyToTarget(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) || s.isTotemGUID(tm.guid) {
					continue
				}
				if !s.playerFriendlyToTarget(caster, tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		case areaAuraEnemy:
			for _, vs := range players {
				if vs.player == nil || (!deadOK && vs.player.Health == 0) || !inRadius(vs.player.X, vs.player.Y, vs.player.Z, ae.radius) {
					continue
				}
				if s.playerFriendlyToTarget(caster, vs.playerGUID) {
					continue
				}
				targets[vs.playerGUID] = struct{}{}
			}
			for _, tm := range creatures {
				if (!deadOK && tm.health == 0) || !inRadius(tm.x, tm.y, tm.z, ae.radius) || s.isTotemGUID(tm.guid) {
					continue
				}
				if s.playerFriendlyToTarget(caster, tm.guid) {
					continue
				}
				targets[tm.guid] = struct{}{}
			}
		}
	}
	return targets
}

// areaAuraPresent mirrors creatureDynobjAuraPresent for a caster-owned
// area aura: the holder carries this spell from this caster as a
// PersistentAreaAura application.
func (s *Server) areaAuraPresent(state *areaAuraState, guid uint64) bool {
	if s == nil || state == nil || guid == 0 {
		return false
	}
	if ts := s.findSessionByGUID(guid); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		aura := ts.activeAuras[state.key.spellID]
		ts.castMu.Unlock()
		return aura != nil && !aura.Stopped && aura.CasterGUID == state.key.casterGUID && aura.PersistentAreaAura
	}
	key := creatureAuraKey{Map: state.mapID, InstanceID: state.instanceID, GUID: guid}
	s.auraMu.Lock()
	aura := s.activeCreatureAuras[key][state.key.spellID]
	s.auraMu.Unlock()
	return aura != nil && !aura.Stopped && aura.CasterGUID == state.key.casterGUID && aura.PersistentAreaAura
}

// expireAreaAuraHolder removes a caster-owned area aura from a holder
// that left the radius — the Aura::UpdateTargetMap removal arm
// (SpellAuras.cpp:663-684), the expireDynobjHolderAura twin. The match
// on (spell, caster, PersistentAreaAura) never strips a same-spell aura
// from another source.
func (s *Server) expireAreaAuraHolder(state *areaAuraState, holderGUID uint64) {
	if s == nil || state == nil || holderGUID == 0 {
		return
	}
	if ts := s.findSessionByGUID(holderGUID); ts != nil {
		ts.removeDynobjAura(state.key.spellID, state.key.casterGUID)
		return
	}
	s.stripDynobjCreatureAura(creatureAuraKey{Map: state.mapID, InstanceID: state.instanceID, GUID: holderGUID}, state.key.spellID, state.key.casterGUID)
}

// applyAreaAuraToHolder applies the caster-owned area aura to one
// FillTargetMap recipient — the Aura::UpdateTargetMap registration arm
// (SpellAuras.cpp:687-732): per-effect immunity strips inside the apply
// paths (Unit::AddAura, Unit.cpp:12291-12297), whole-spell immunity
// blocks the target, and the exclusive-highest verdict rides the
// persistent-area flavor of both funnels. Returns whether the aura is
// present afterwards, so a suppressed or immune target is not adopted
// into the holder set (the dynobj pass's creatureDynobjAuraPresent
// gate). The periodic timer rides the record's primary effect, as in
// the dynobj cast-time fill; there is no shared dynobj schedule to
// align to, so no initial-delay override.
func (s *Server) applyAreaAuraToHolder(ctx context.Context, state *areaAuraState, caster *creatureMotion, holderGUID uint64) bool {
	if s == nil || state == nil || caster == nil || holderGUID == 0 {
		return false
	}
	if !s.fireAuraCheckAreaTarget(ctx, state.key.spellID, state.key.casterGUID, state.mapID, state.instanceID, holderGUID) {
		return false
	}
	if vs := s.findSessionByGUID(holderGUID); vs != nil && vs.player != nil {
		vs.applyAuraFromCaster(state.key.spellID, state.durationMs, state.key.casterGUID, uint8(caster.Level), 0, nil, true, false)
	} else {
		// DynObjAura::UpdateTargetMap (SpellAuras.cpp:687-695) never
		// registers a whole-spell immune target; the per-effect half
		// rides serverApplyAuraToCreatureTarget, but the whole-spell
		// half needs this pre-check, the updateCreatureDynobjTargetMap
		// twin. Player targets already get both gates inside
		// applyAuraFromCaster.
		if s.dynobjCreatureTargetImmune(ctx, state.spell, holderGUID, state.mapID, state.instanceID, s.creatureFriendlyToTarget(caster, holderGUID)) {
			return false
		}
		s.serverApplyAuraToCreatureTarget(ctx, caster, state.spell, state.durationMs, holderGUID, nil, true, nil)
	}
	return s.areaAuraPresent(state, holderGUID)
}

// applyPlayerAreaAuraToHolder applies the player-caster area aura to one
// FillTargetMap recipient — the applyAreaAuraToHolder twin for a player caster.
// Player holders ride applyAuraFromCaster's whole-record path (persistent-area
// flavor, like the creature twin); creature holders ride the dynobj pass's
// per-effect applyAuraToTarget funnel (persistent-area flavor, shared
// castMerged) after the whole-spell immunity pre-check, with the friendly
// verdict from the player caster.
func (s *Server) applyPlayerAreaAuraToHolder(ctx context.Context, state *areaAuraState, caster *session, holderGUID uint64) bool {
	if s == nil || state == nil || caster == nil || caster.player == nil || holderGUID == 0 {
		return false
	}
	if !s.fireAuraCheckAreaTarget(ctx, state.key.spellID, state.key.casterGUID, state.mapID, state.instanceID, holderGUID) {
		return false
	}
	if vs := s.findSessionByGUID(holderGUID); vs != nil && vs.player != nil {
		vs.applyAuraFromCaster(state.key.spellID, state.durationMs, state.key.casterGUID, caster.player.Level, 0, nil, true, false)
	} else {
		if s.dynobjCreatureTargetImmune(ctx, state.spell, holderGUID, state.mapID, state.instanceID, s.playerFriendlyToTarget(caster, holderGUID)) {
			return false
		}
		schoolMask := state.spell.SchoolMask
		if schoolMask == 0 {
			schoolMask = 1
		}
		castMerged := make(map[uint64]struct{})
		for i, eff := range state.spell.Effects {
			if eff.Aura == 0 {
				continue
			}
			caster.applyAuraToTarget(ctx, holderGUID, state.spell, eff, i, state.durationMs, eff.AuraPeriod, uint32(eff.BasePoints), schoolMask, castMerged, false, state.key.casterGUID, true, 0, false, nil)
		}
	}
	return s.areaAuraPresent(state, holderGUID)
}

// playerAreaAuraDurationMs mirrors the creature twin's duration fold
// (boss_ai.go:9978-9995) and Aura::CalcMaxDuration (SpellAuras.cpp:871-886):
// the DBC duration with the caster's SPELLMOD_DURATION mods. No haste fold,
// matching the creature twin.
func (s *session) playerAreaAuraDurationMs(spell wotlk.Spell) uint32 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || spell.DurationIndex == 0 {
		return 0
	}
	dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level))
	if err != nil || !ok || dur <= 0 {
		return 0
	}
	if modded := s.applySpellMod(spell, spellModDuration, dur); modded > 0 {
		dur = modded
	}
	return uint32(dur)
}

// registerPlayerAreaAura creates the caster-owned area aura at cast time for a
// player caster — the registerCreatureAreaAura twin (Spell::DoSpellEffectHit,
// Spell.cpp:2840-2872, creates the aura on the caster; Aura::UpdateTargetMap
// fills it on the first update tick, so the cast-time fill runs here and the
// 500ms pass re-selects after). A re-cast refreshes the state and carries the
// old holder set over; durationMs arrives pre-folded (SPELLMOD_DURATION via
// the castSpellDirectWithOverrides caller).
func (s *session) registerPlayerAreaAura(ctx context.Context, spell wotlk.Spell, durationMs uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || !spellHasAreaAuraEffect(spell) {
		return
	}
	now := time.Now()
	state := &areaAuraState{
		key:               areaAuraKey{casterGUID: s.playerGUID, spellID: spell.ID},
		spell:             spell,
		mapID:             s.player.Map,
		instanceID:        s.player.InstanceID,
		durationMs:        durationMs,
		nextTargetMapPass: now.Add(dynobjTargetMapInterval),
		holders:           make(map[uint64]struct{}),
	}
	if durationMs > 0 {
		state.expiresAt = now.Add(time.Duration(durationMs) * time.Millisecond)
	}
	level := uint32(s.player.Level)
	for i, eff := range spell.Effects {
		if eff.Aura == 0 {
			continue
		}
		selection, ok := areaAuraSelectionForEffect(eff.Effect)
		if !ok {
			continue
		}
		// SpellAuras.cpp:2684: radius = Effects[effIndex].CalcRadius(ref) —
		// same zero-radius rule as the creature twin (no max-range
		// fallback in FillTargetMap).
		radius, _, _ := s.server.Data.SpellRadius(eff.RadiusIndex, level)
		if radius < 0 {
			radius = 0
		}
		state.effects = append(state.effects, areaAuraEffect{index: i, effect: eff, selection: selection, radius: radius})
	}
	if len(state.effects) == 0 {
		return
	}
	s.server.objectsMu.Lock()
	if old := s.server.areaAuras[state.key]; old != nil {
		for holderGUID := range old.holders {
			state.holders[holderGUID] = struct{}{}
		}
	}
	if s.server.areaAuras == nil {
		s.server.areaAuras = make(map[areaAuraKey]*areaAuraState)
	}
	s.server.areaAuras[state.key] = state
	s.server.objectsMu.Unlock()
	for guid := range s.server.areaAuraFillTargetsForPlayer(ctx, state, s) {
		if s.server.applyPlayerAreaAuraToHolder(ctx, state, s, guid) {
			state.holders[guid] = struct{}{}
		}
	}
}

// registerCreatureAreaAura creates the caster-owned area aura at cast
// time: Spell::DoSpellEffectHit (Spell.cpp:2840-2872) creates the aura
// on the caster and Aura::UpdateTargetMap fills it on the first update
// tick, so the cast-time fill runs here and the 500ms pass re-selects
// after. A re-cast refreshes the state (TryRefreshStackOrCreate) and
// carries the old holder set over, so leavers of the previous cast
// still get stripped. DurationMs arrives pre-folded (SPELLMOD_DURATION
// via the creatureEffectApplyAura caller). No SpellScript prevent skip:
// the area effects are EffectUnused, so a PreventHitDefaultEffect veto
// on one is vacuous in C++ — the aura is created in DoSpellEffectHit
// before the effect handler ever runs.
func (s *Server) registerCreatureAreaAura(ctx context.Context, m *creatureMotion, spell wotlk.Spell, durationMs uint32) {
	if s == nil || m == nil || s.Data == nil || !spellHasAreaAuraEffect(spell) {
		return
	}
	now := time.Now()
	state := &areaAuraState{
		key:               areaAuraKey{casterGUID: m.GUID, spellID: spell.ID},
		spell:             spell,
		mapID:             m.Map,
		instanceID:        m.InstanceID,
		durationMs:        durationMs,
		nextTargetMapPass: now.Add(dynobjTargetMapInterval),
		holders:           make(map[uint64]struct{}),
	}
	if durationMs > 0 {
		state.expiresAt = now.Add(time.Duration(durationMs) * time.Millisecond)
	}
	level := casterLevel(m)
	for i, eff := range spell.Effects {
		if eff.Aura == 0 {
			continue
		}
		selection, ok := areaAuraSelectionForEffect(eff.Effect)
		if !ok {
			continue
		}
		// SpellAuras.cpp:2684: radius = Effects[effIndex].CalcRadius(ref).
		// CalcRadius returns 0 with no RadiusEntry (SpellInfo.cpp:551),
		// and FillTargetMap has NO max-range fallback (that workaround
		// lives in the spell-targeting selectors, Spell.cpp:1194/1284,
		// not here) — so a missing/zero radius is kept as 0, and the
		// pass below selects only distance-0 units (the caster itself),
		// exactly as the C++ radius-0 search does.
		radius, _, _ := s.Data.SpellRadius(eff.RadiusIndex, level)
		if radius < 0 {
			radius = 0
		}
		state.effects = append(state.effects, areaAuraEffect{index: i, effect: eff, selection: selection, radius: radius})
	}
	if len(state.effects) == 0 {
		return
	}
	s.objectsMu.Lock()
	if old := s.areaAuras[state.key]; old != nil {
		for holderGUID := range old.holders {
			state.holders[holderGUID] = struct{}{}
		}
	}
	if s.areaAuras == nil {
		s.areaAuras = make(map[areaAuraKey]*areaAuraState)
	}
	s.areaAuras[state.key] = state
	s.objectsMu.Unlock()
	// A re-cast refreshes every in-radius holder's record (C++
	// TryRefreshStackOrCreate refreshes the aura duration), so carried
	// holders are re-applied here, not just adopted — unlike the 500ms
	// pass, which must not refresh durations every pass.
	for guid := range s.areaAuraFillTargets(ctx, state, m) {
		if s.applyAreaAuraToHolder(ctx, state, m, guid) {
			state.holders[guid] = struct{}{}
		}
	}
}

// updateAreaAuras runs the 500ms target-map pass for every live caster-owned
// area aura — Aura::Update (SpellAuras.cpp:813-820) calling UpdateTargetMap
// (SpellAuras.cpp:643-767) on the interval, hooked into the same world tick as
// the dynobj pass. An expired aura strips all holders and drops; a dead,
// despawned or logged-out caster does the same (the C++ aura dies with its
// owner). Leavers are stripped, joiners applied, and holders failing
// CanBeAppliedOn are removed now and re-applied on a later pass when they pass
// again. Player casters resolve through their session (a map/instance change
// reads as the caster leaving, like a despawned creature).
func (s *Server) updateAreaAuras(ctx context.Context, now time.Time) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	var due []*areaAuraState
	for _, state := range s.areaAuras {
		if state == nil {
			continue
		}
		if now.Before(state.nextTargetMapPass) {
			continue
		}
		state.nextTargetMapPass = now.Add(dynobjTargetMapInterval)
		due = append(due, state)
	}
	s.objectsMu.Unlock()
	for _, state := range due {
		caster := s.findCreatureMotion(state.mapID, state.instanceID, state.key.casterGUID)
		var casterSess *session
		if caster == nil {
			if cs := s.findSessionByGUID(state.key.casterGUID); cs != nil && cs.player != nil && cs.player.Map == state.mapID && cs.player.InstanceID == state.instanceID {
				casterSess = cs
			}
		}
		expired := !state.expiresAt.IsZero() && !now.Before(state.expiresAt)
		casterGone := caster == nil && casterSess == nil
		casterDead := (caster != nil && caster.Health == 0) || (casterSess != nil && casterSess.player.Health == 0)
		if casterGone || casterDead || expired {
			for holderGUID := range state.holders {
				s.expireAreaAuraHolder(state, holderGUID)
			}
			s.objectsMu.Lock()
			if cur := s.areaAuras[state.key]; cur == state {
				delete(s.areaAuras, state.key)
			}
			s.objectsMu.Unlock()
			continue
		}
		var inRadius map[uint64]struct{}
		if caster != nil {
			inRadius = s.areaAuraFillTargets(ctx, state, caster)
		} else {
			inRadius = s.areaAuraFillTargetsForPlayer(ctx, state, casterSess)
		}
		for holderGUID := range state.holders {
			if _, ok := inRadius[holderGUID]; !ok {
				s.expireAreaAuraHolder(state, holderGUID)
				delete(state.holders, holderGUID)
				continue
			}
			if !s.fireAuraCheckAreaTarget(ctx, state.key.spellID, state.key.casterGUID, state.mapID, state.instanceID, holderGUID) {
				s.expireAreaAuraHolder(state, holderGUID)
				delete(state.holders, holderGUID)
			}
		}
		for guid := range inRadius {
			if _, ok := state.holders[guid]; ok {
				continue
			}
			if s.areaAuraPresent(state, guid) {
				// Cast-time holder adopted without re-applying: a
				// re-apply would refresh its duration every pass
				// (the updateCreatureDynobjTargetMap twin).
				state.holders[guid] = struct{}{}
				continue
			}
			applied := false
			if caster != nil {
				applied = s.applyAreaAuraToHolder(ctx, state, caster, guid)
			} else {
				applied = s.applyPlayerAreaAuraToHolder(ctx, state, casterSess, guid)
			}
			if applied {
				state.holders[guid] = struct{}{}
			}
		}
	}
}

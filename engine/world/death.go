package world

import (
	"context"
	"database/sql"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Death lifecycle conversion of the reference chain:
//   - Player::KillPlayer (Player.cpp:4770) triggered by lethal creature damage
//   - Player::Update 6 minute auto-release timer (Player.cpp:1303-1312)
//   - WorldSession::HandleRepopRequest (MiscHandler.cpp:61)
//   - Player::BuildPlayerRepop (Player.cpp:4627)
//   - Player::RepopAtGraveyard (Player.cpp:5109)
//   - ObjectMgr::GetClosestGraveyard / GetDefaultGraveyard (ObjectMgr.cpp:6849/6864)

const (
	soulShardItemID       = 6265
	glyphOfDrainSoulSpell = 58070
	drainSoulFamilyFlag0  = 0x4000
	// playerFieldByteReleaseTimer is PLAYER_FIELD_BYTE_RELEASE_TIMER within
	// PLAYER_FIELD_BYTES byte 0 (Player.h:417); set while the client shows the
	// auto release spirit countdown.
	playerFieldByteReleaseTimer uint32 = 0x00000008
	unitFieldPlayerFieldBytes          = 1197 // PLAYER_FIELD_BYTES = UNIT_END + 0x0419

	deathExpireStepSeconds = 5 * 60 // DEATH_EXPIRE_STEP (Player.cpp:174)
	maxDeathCount          = 3      // MAX_DEATH_COUNT (Player.cpp:175)

	autoRepopDelay = 6 * time.Minute // KillPlayer m_deathTimer (Player.cpp:4786)

	corpseReclaimRadius = 39.0 // CORPSE_RECLAIM_RADIUS (Corpse.h:35)

	corpseTypeBones uint32 = 0
	corpseTypePvE   uint32 = 1 // CORPSE_RESURRECTABLE_PVE
	corpseTypePvP   uint32 = 2 // CORPSE_RESURRECTABLE_PVP

	spellRaiseAllyInitial = 61999 // spell_dk_raise_ally_initial (spell_dk.cpp:2803)
	spellRaiseAlly        = 46619 // SPELL_DK_RAISE_ALLY (spell_dk.cpp:2784), the applied aura
	broadcastTextRiseAlly = 33055 // TEXT_RISE_ALLY (spell_dk.cpp:2786)
	chatRaidBossWhisper   = 0x2A  // CHAT_MSG_RAID_BOSS_WHISPER (SharedDefines.h:3204)

	corpseFlagBones    uint32 = 0x01
	corpseFlagUnk2     uint32 = 0x04
	corpseFlagLootable uint32 = 0x20 // CORPSE_FLAG_LOOTABLE (Corpse.h:45)
	// corpseDynFlagLootable mirrors CORPSE_DYNFLAG_LOOTABLE
	// (SharedDefines.h:3140): the dynamic flag RemovedInsignia sets on
	// the bones so the looter's client shows them lootable
	// (Player.cpp:8510), stripped when the loot is fully taken
	// (LootHandler.cpp:321).
	corpseDynFlagLootable uint32 = 0x0001
	playerFlagHideHelm    uint32 = 0x00000400
	playerFlagHideCloak   uint32 = 0x00000800

	teamAlliance uint32 = 469
	teamHorde    uint32 = 67

	defaultGraveyardAlliance uint32 = 4  // Westfall (ObjectMgr.cpp:6855)
	defaultGraveyardHorde    uint32 = 10 // Crossroads (ObjectMgr.cpp:6853)
)

// grantChannelDeathItems mirrors AuraEffect::HandleChannelDeathItem's
// remove-by-death leg (SpellAuraEffects.cpp:4690-4743): each
// SPELL_AURA_CHANNEL_DEATH_ITEM (86) effect on the dying unit grants its
// ItemType x amount to the aura's caster when the caster is a player. Go's
// death paths strip auras via clearActiveAuras with no per-aura remove legs
// and no remove-mode tracking, so this runs explicitly before the strip on
// both the player-death (killPlayer) and creature-death (onCreatureKilled)
// paths. auras are the victim's pre-strip aura records; tapPlayerGUID/
// tapGroupID are the dead creature's tap (unused for player victims).
func (s *Server) grantChannelDeathItems(ctx context.Context, auras []*activeAura, victimLevel uint32, victimIsCreature bool, tapPlayerGUID, tapGroupID uint64) {
	if s == nil || s.Data == nil || len(auras) == 0 {
		return
	}
	for _, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		// AuraEffect::HandleChannelDeathItem fires only on removal with
		// AURA_REMOVE_BY_DEATH (SpellAuraEffects.cpp:4692); death-persistent
		// and passive auras are never stripped by RemoveAllAurasOnDeath, so
		// the handler never fires for them — exclude them from the
		// pre-strip set.
		if spellDeathPersistent(s.Data, aura.SpellID) || spellIsPassive(s.Data, aura.SpellID) {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		caster := s.findSessionByGUID(aura.CasterGUID)
		if caster == nil || caster.player == nil {
			continue
		}
		for index, effect := range spell.Effects {
			if effect.Aura != spellAuraChannelDeathItem || index >= len(aura.Amounts) || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			amount := aura.Amounts[index]
			if amount <= 0 || effect.ItemType == 0 {
				continue
			}
			count := uint32(amount)
			if effect.ItemType == soulShardItemID {
				// Soul Shard only from units that grant XP or honor
				// (Player::isHonorOrXPTarget, Player.cpp:24064-24083); the
				// critter/totem exclusion has no Go creature-type model, so
				// the gray-level gate is the modeled arm; a nonzero
				// MinCreatureScaledXpRatio lets gray-level victims pass.
				if victimLevel <= grayLevel(uint32(caster.player.Level)) && creatureScaledXPRatio(s) == 0 {
					continue
				}
				if victimIsCreature && tapPlayerGUID != caster.playerGUID && (tapGroupID == 0 || tapGroupID != caster.groupID) {
					continue
				}
				// Drain Soul + Glyph of Drain Soul: roll_chance_i on the
				// glyph's misc for an extra shard (C++ casts 58068; the
				// outcome-equivalent is one more item here).
				if spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[0]&drainSoulFamilyFlag0 != 0 {
					caster.castMu.Lock()
					glyph, ok := caster.activeAuras[glyphOfDrainSoulSpell]
					caster.castMu.Unlock()
					if ok && glyph != nil && !glyph.Stopped && glyph.MiscValue > 0 && rand.Intn(100) < int(glyph.MiscValue) {
						count++
					}
				}
			}
			// Player::CanStoreNewItem's partial-count arm has no Go
			// counterpart (canStoreNewItem is all-or-nothing); a full
			// failure drops the grant like C++'s count==0 return.
			if caster.canStoreNewItem(ctx, caster.playerGUID, effect.ItemType, count) != equipErrOk {
				continue
			}
			res, err := caster.storeOrStackItem(ctx, caster.playerGUID, effect.ItemType, count)
			if err != nil || res == nil {
				continue
			}
			slotForPush := uint32(res.Slot)
			if res.IsStack {
				slotForPush = 0xFFFFFFFF
			}
			_ = caster.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(caster.playerGUID, res.ClientBag, slotForPush, effect.ItemType, count, res.InventoryCount, res.IsStack), true)
		}
	}
}

// isCharmFamilyAura reports whether an aura type is one of the four
// Unit::RemoveCharmAuras types (Unit.cpp:6525-6530). Unit::setDeathState
// (Unit.cpp:8916-8920) runs RemoveAllControlled before
// RemoveAllAurasOnDeath, so a dying charmer releases its charmed units
// unconditionally — the owned-aura loop's death-persistent gate never gets
// a chance to spare a charm aura. The death sweep mirrors that order by
// exempting the charm family from the gate; the remove funnels then run
// their uncharm/unpossess tails (removeCreatureAuraWithMode, and the
// player funnel's client-control restore).
func isCharmFamilyAura(auraType uint32) bool {
	return auraType == spellAuraCharm || auraType == spellAuraAoeCharm || auraType == spellAuraModPossess || auraType == spellAuraModPossessPet
}

// sweepOwnedAurasOnDeath mirrors the m_ownedAuras loop of
// Unit::RemoveAllAurasOnDeath (Unit.cpp:4364-4370): auras the dying unit CAST
// on other targets are destroyed with AURA_REMOVE_BY_DEATH — passive and
// death-persistent auras are skipped, exactly like the applied loop, except
// the charm family (isCharmFamilyAura), which setDeathState's
// RemoveAllControlled already released unconditionally, and
// SPELL_AURA_CONTROL_VEHICLE, which setDeathState's ExitVehicle
// (Unit.cpp:8916, before RemoveAllControlled) already removed by type
// with no death-persistence check — the sweep's strip of a vehicle aura
// models ExitVehicle, not the death strip, and the remove funnel then
// runs the wasControlVehicle exit tail. Passive auras reach the stores through creature casts (charm re-casts
// the creature's passive template spells, creaturemotion.go), so the
// passive gate is live here too.
// Records are matched on aura.CasterGUID == casterGUID, which creature casts
// set to the motion GUID (boss_ai.go:1063) and player casts to the player
// GUID. The per-caster merge deviation is parked: a same-spell record from
// another caster keeps its own CasterGUID and is never matched here. Lock
// discipline: session/aura snapshots are collected under the map locks and
// released before the funnel calls, which take the locks themselves.
func (s *Server) sweepOwnedAurasOnDeath(casterGUID uint64) {
	if s == nil || casterGUID == 0 {
		return
	}
	// Player targets.
	s.sessionsMu.RLock()
	var sessions []*session
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess == nil || sess.player == nil {
			continue
		}
		sess.castMu.Lock()
		var strip []uint32
		if sess.activeAuras != nil {
			for id, aura := range sess.activeAuras {
				if aura == nil || aura.CasterGUID != casterGUID || ((spellIsPassive(s.Data, id) || spellDeathPersistent(s.Data, id)) && !isCharmFamilyAura(aura.AuraType) && aura.AuraType != spellAuraControlVehicle) {
					continue
				}
				strip = append(strip, id)
			}
		}
		sess.castMu.Unlock()
		for _, id := range strip {
			sess.removeAuraWithMode(id, auraRemoveByDeath)
		}
	}
	// Creature targets.
	type ownedStrip struct {
		key creatureAuraKey
		id  uint32
	}
	s.auraMu.Lock()
	var cstrip []ownedStrip
	if s.activeCreatureAuras != nil {
		for key, auras := range s.activeCreatureAuras {
			for id, aura := range auras {
				if aura == nil || aura.CasterGUID != casterGUID || ((spellIsPassive(s.Data, id) || spellDeathPersistent(s.Data, id)) && !isCharmFamilyAura(aura.AuraType) && aura.AuraType != spellAuraControlVehicle) {
					continue
				}
				cstrip = append(cstrip, ownedStrip{key: key, id: id})
			}
		}
	}
	s.auraMu.Unlock()
	for _, cs := range cstrip {
		s.removeCreatureAuraWithMode(cs.key, cs.id, auraRemoveByDeath)
	}
}

// sweepOwnedAurasOnLeaveWorld mirrors the first arm of
// Unit::RemoveAreaAurasDueToLeaveWorld (Unit.cpp:4249-4266), reached at
// logout via Unit::RemoveFromWorld (Unit.cpp:9802): auras the leaving
// unit owns on other targets are removed with the default mode. Unlike
// the death sweep there is no death-persistent gate — C++ strips every
// owned application on leave-world. Foreign-owned auras on the leaving
// unit itself are already cleared by clearActiveAuras.
func (s *Server) sweepOwnedAurasOnLeaveWorld(casterGUID uint64) {
	if s == nil || casterGUID == 0 {
		return
	}
	s.sessionsMu.RLock()
	var sessions []*session
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess == nil || sess.player == nil {
			continue
		}
		sess.castMu.Lock()
		var strip []uint32
		if sess.activeAuras != nil {
			for id, aura := range sess.activeAuras {
				if aura == nil || aura.CasterGUID != casterGUID {
					continue
				}
				strip = append(strip, id)
			}
		}
		sess.castMu.Unlock()
		for _, id := range strip {
			sess.removeAuraWithMode(id, auraRemoveByDefault)
		}
	}
	type ownedStrip struct {
		key creatureAuraKey
		id  uint32
	}
	s.auraMu.Lock()
	var cstrip []ownedStrip
	if s.activeCreatureAuras != nil {
		for key, auras := range s.activeCreatureAuras {
			for id, aura := range auras {
				if aura == nil || aura.CasterGUID != casterGUID {
					continue
				}
				cstrip = append(cstrip, ownedStrip{key: key, id: id})
			}
		}
	}
	s.auraMu.Unlock()
	for _, cs := range cstrip {
		s.removeCreatureAuraWithMode(cs.key, cs.id, auraRemoveByDefault)
	}
}

func (s *session) currentPlayerPhaseMask() uint32 {
	if s == nil || s.player == nil {
		return 1
	}
	// An explicit .modify phase override wins over the GM and aura legs:
	// C++ stores the mask in one slot, and SetGameMaster itself writes
	// PHASEMASK_ANYWHERE into it (Player.cpp:2456). Delta: Go's GM leg
	// is computed, so toggling GM on/off does not overwrite or restore
	// the override the way C++'s GM-off aura restore does; only another
	// .modify phase call replaces it.
	if s.player.PhaseMaskOverrideSet {
		return s.player.PhaseMaskOverride
	}
	if s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0 {
		return ^uint32(0)
	}
	s.castMu.Lock()
	phaseMask := uint32(0)
	for _, aura := range s.activeAuras {
		if aura != nil && !aura.Stopped && aura.AuraType == spellAuraPhase {
			phaseMask |= uint32(aura.MiscValue)
		}
	}
	s.castMu.Unlock()
	if phaseMask == 0 {
		return 1
	}
	return phaseMask
}

// corpseMapKey scopes a corpse GUID sequence to one map instance
// (Map::GenerateLowGuid is per-Map, Map.h:624-629).
type corpseMapKey struct {
	mapID, instanceID uint32
}

// corpseKey identifies one corpse object on a map instance by its owner
// (Map::_corpsesByPlayer is keyed the same way, Map.cpp:4603-4605).
type corpseKey struct {
	mapID, instanceID uint32
	ownerLow          uint64
}

// corpseGUIDKey identifies one corpse object by its assigned low GUID.
type corpseGUIDKey struct {
	mapID, instanceID uint32
	low               uint32
}

// corpseBonesExpiry is the bones lifetime (Corpse::IsExpired, Corpse.cpp:209:
// CORPSE_BONES expires after 60 minutes; Map::RemoveOldCorpses,
// Map.cpp:4693-4701, despawns them on the corpse sweep).
const corpseBonesExpiry = 60 * time.Minute

// corpseBonesEntry is one server-side bones object (Map::_corpseBones):
// the ownerless visual Map::ConvertCorpseToBones creates (Map.cpp:4627-4676),
// kept so late arrivals and relogs see it (buildNearbyCorpseUpdates) until
// the 60-minute expiry. Bones keep the corpse's low GUID
// (bones->Create(corpse->GetGUID().GetCounter()), Map.cpp:4653), so the
// owner->low mapping stays live while the bones entry exists.
type corpseBonesEntry struct {
	lowGUID                      uint32
	displayID, bytes1, bytes2    uint32
	guildID, flags, dynamicFlags uint32
	phaseMask                    uint32
	x, y, z, orientation         float32
	createdAt                    time.Time
	// lootLevel is the RemovedInsignia stash (Player::RemovedInsignia,
	// Player.cpp:8513-8515): the victim's level is stored in the bones'
	// loot gold field until the first SendLoot fills it, then consumed
	// (Player.cpp:8729-8731 reads it into pLevel and loot.clear() drops
	// it, so a re-open after a fully-looted release rolls gold from 0).
	// lootRecipient is the looter the insignia belongs to
	// (Player.cpp:8514); lootFilled marks the first-open fill
	// (Player.cpp:8727-8742), after which lootGold/lootItems are the
	// rolled window instead of the stash.
	lootLevel      uint32
	lootRecipient  uint64
	lootFilled     bool
	lootGold       uint32
	lootItems      map[uint8]lootItem
	lootQuestItems map[uint8]lootItem
	// lootNormalSlotCount is the fill-time NormalSlotCount
	// (activeLootState's display-slot base, Loot.cpp:618-703): the
	// items vector never shrinks on take, so the base is captured at
	// fill time and survives takes.
	lootNormalSlotCount uint8
}

// corpseLowGUID returns the map-scoped low GUID for a corpse object,
// assigning the next sequence value on first sight (Map::GenerateLowGuid,
// Map.h:624; ObjectGuidGenerator starts at 1, ObjectGuid.h:281-308).
// The assignment is stable while the corpse or its bones exist; a later
// corpse for the same owner gets a fresh value from the sequence, which
// never rewinds.
func (s *Server) corpseLowGUID(mapID, instanceID uint32, ownerLow uint64) uint32 {
	if s == nil {
		return uint32(ownerLow)
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	return s.corpseLowGUIDLocked(mapID, instanceID, ownerLow)
}

func (s *Server) corpseLowGUIDLocked(mapID, instanceID uint32, ownerLow uint64) uint32 {
	key := corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}
	if s.corpseGUIDByOwner != nil {
		if low, ok := s.corpseGUIDByOwner[key]; ok {
			return low
		}
	}
	if s.corpseGUIDSeq == nil {
		s.corpseGUIDSeq = make(map[corpseMapKey]uint32)
	}
	mapKey := corpseMapKey{mapID: mapID, instanceID: instanceID}
	low := s.corpseGUIDSeq[mapKey] + 1
	s.corpseGUIDSeq[mapKey] = low
	if s.corpseGUIDByOwner == nil {
		s.corpseGUIDByOwner = make(map[corpseKey]uint32)
	}
	s.corpseGUIDByOwner[key] = low
	if s.corpseOwnerByGUID == nil {
		s.corpseOwnerByGUID = make(map[corpseGUIDKey]uint64)
	}
	s.corpseOwnerByGUID[corpseGUIDKey{mapID: mapID, instanceID: instanceID, low: low}] = ownerLow
	return low
}

// corpseOwnerLow is the reverse lookup for a map-assigned corpse low GUID
// (ObjectAccessor::GetCorpse / GetCorpseByPlayer twin): the owner whose
// corpse carries this GUID on the map instance, or false when no such
// corpse is registered.
func (s *Server) corpseOwnerLow(mapID, instanceID uint32, low uint32) (uint64, bool) {
	if s == nil {
		return 0, false
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	if s.corpseOwnerByGUID == nil {
		return 0, false
	}
	owner, ok := s.corpseOwnerByGUID[corpseGUIDKey{mapID: mapID, instanceID: instanceID, low: low}]
	return owner, ok
}

// dropCorpseGUIDMapping drops one corpse's GUID mapping (Map::RemoveCorpse
// erases the _corpsesByPlayer entry, Map.cpp:4617-4620) without touching a
// standing bones entry for the same owner: the bones keep their low GUID
// in the entry itself, and only the mapping must go so a later corpse for
// the owner mints fresh instead of reusing the bones' value.
func (s *Server) dropCorpseGUIDMapping(mapID, instanceID uint32, ownerLow uint64) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	key := corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}
	if low, ok := s.corpseGUIDByOwner[key]; ok {
		delete(s.corpseGUIDByOwner, key)
		delete(s.corpseOwnerByGUID, corpseGUIDKey{mapID: mapID, instanceID: instanceID, low: low})
	}
}

// dropCorpseGUIDMappings is the owner-wide form of dropCorpseGUIDMapping,
// for corpse removals that do not know the corpse's map (the login
// resurrect path): every mapping for the owner goes, standing bones
// entries stay.
func (s *Server) dropCorpseGUIDMappings(ownerLow uint64) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	for key, low := range s.corpseGUIDByOwner {
		if key.ownerLow != ownerLow {
			continue
		}
		delete(s.corpseGUIDByOwner, key)
		delete(s.corpseOwnerByGUID, corpseGUIDKey{mapID: key.mapID, instanceID: key.instanceID, low: low})
	}
}

// releaseCorpseGUIDForNewCorpse runs before a new corpse object is created
// for an owner whose previous death's bones still stand on the map: the
// bones keep their low GUID (bones->Create(corpse->GetGUID().GetCounter()),
// Map.cpp:4653) while the new corpse mints fresh from the sequence
// (Player::CreateCorpse, Player.cpp:4822). Without the release the new
// corpse would share its GUID with the standing bones.
func (s *Server) releaseCorpseGUIDForNewCorpse(mapID, instanceID uint32, ownerLow uint64) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	key := corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}
	if _, ok := s.corpseBones[key]; !ok {
		return
	}
	if low, ok := s.corpseGUIDByOwner[key]; ok {
		delete(s.corpseGUIDByOwner, key)
		delete(s.corpseOwnerByGUID, corpseGUIDKey{mapID: mapID, instanceID: instanceID, low: low})
	}
}

// forgetCorpseGUIDsForMap drops every corpse GUID mapping and bones
// entry on a map instance (Map::DeleteCorpseData, Map.cpp:4585-4592: the
// rows go with the unloading map, so the objects they described are gone
// and their low GUIDs must not be handed to a later corpse).
func (s *Server) forgetCorpseGUIDsForMap(mapID, instanceID uint32) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	for key, low := range s.corpseGUIDByOwner {
		if key.mapID != mapID || key.instanceID != instanceID {
			continue
		}
		delete(s.corpseGUIDByOwner, key)
		delete(s.corpseOwnerByGUID, corpseGUIDKey{mapID: key.mapID, instanceID: key.instanceID, low: low})
		delete(s.corpseBones, key)
	}
}

// ensureCorpseGUIDsForMap assigns low GUIDs to every resurrectable corpse
// row on a map instance (Map::LoadCorpseData, Map.cpp:4551-4583, mints a
// fresh GenerateLowGuid per loaded row): a corpse GUID that reaches a
// lookup before any player rendered the corpse still resolves to its
// owner instead of reading as a stale or foreign GUID.
func (s *Server) ensureCorpseGUIDsForMap(ctx context.Context, mapID, instanceID uint32) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT guid FROM corpse WHERE mapId = ? AND instanceId = ? AND corpseType <> ?", mapID, instanceID, corpseTypeBones)
	if err != nil {
		return
	}
	defer rows.Close()
	var owners []uint64
	for rows.Next() {
		var owner uint64
		if err := rows.Scan(&owner); err == nil && owner != 0 {
			owners = append(owners, owner)
		}
	}
	_ = rows.Close()
	if len(owners) == 0 {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	for _, owner := range owners {
		s.corpseLowGUIDLocked(mapID, instanceID, owner)
	}
}

// registerCorpseBones records the bones object a conversion created, so
// it survives in the world past the creating broadcast (Map::AddCorpse +
// AddToMap, Map.cpp:4667-4670).
func (s *Server) registerCorpseBones(mapID, instanceID uint32, ownerLow uint64, entry corpseBonesEntry) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	if s.corpseBones == nil {
		s.corpseBones = make(map[corpseKey]*corpseBonesEntry)
	}
	e := entry
	s.corpseBones[corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}] = &e
}

// bonesEntryLocked returns the bones entry for a map-assigned low GUID,
// resolving the owner through the reverse mapping first (the
// ObjectAccessor::GetCorpse twin); the caller holds corpseMu.
func (s *Server) bonesEntryLocked(mapID, instanceID uint32, low uint32) (*corpseBonesEntry, uint64, bool) {
	if s.corpseOwnerByGUID == nil || s.corpseBones == nil {
		return nil, 0, false
	}
	ownerLow, ok := s.corpseOwnerByGUID[corpseGUIDKey{mapID: mapID, instanceID: instanceID, low: low}]
	if !ok {
		return nil, 0, false
	}
	entry, ok := s.corpseBones[corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}]
	if entry == nil {
		return nil, 0, false
	}
	return entry, ownerLow, true
}

// setBonesInsigniaLoot attaches the RemovedInsignia loot record to a
// converted bones object (Player.cpp:8510-8515): the dynamic lootable
// flag, the victim's level stashed as the loot gold, and the looter as
// lootRecipient. It returns the bones GUID, or 0 when no bones stand
// for the owner on the map.
func (s *Server) setBonesInsigniaLoot(mapID, instanceID uint32, ownerLow uint64, level uint32, recipient uint64) uint64 {
	if s == nil {
		return 0
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	entry, ok := s.corpseBones[corpseKey{mapID: mapID, instanceID: instanceID, ownerLow: ownerLow}]
	if entry == nil || !ok {
		return 0
	}
	entry.dynamicFlags |= corpseDynFlagLootable
	entry.lootLevel = level
	entry.lootRecipient = recipient
	entry.lootFilled = false
	entry.lootGold = 0
	entry.lootItems = nil
	entry.lootQuestItems = nil
	return uint64(0xF101)<<48 | uint64(entry.lowGUID)
}

// bonesLootSnapshot copies a bones entry's loot state for a SendLoot
// open (Player::SendLoot, Player.cpp:8712-8749 reads bones->loot
// through the same object the other arms mutate, so the open works
// from a snapshot taken under the registry lock).
func (s *Server) bonesLootSnapshot(mapID, instanceID uint32, low uint32) (corpseBonesEntry, uint64, bool) {
	if s == nil {
		return corpseBonesEntry{}, 0, false
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	entry, ownerLow, ok := s.bonesEntryLocked(mapID, instanceID, low)
	if !ok {
		return corpseBonesEntry{}, 0, false
	}
	return *entry, ownerLow, true
}

// storeBonesLootFilled records the first-open fill on a bones entry
// (Player.cpp:8727-8742: FillLoot + the gold formula, after which
// loot_type is no longer LOOT_NONE). First writer wins: a concurrent
// open re-reads the stored fill instead of rolling its own.
func (s *Server) storeBonesLootFilled(mapID, instanceID uint32, low uint32, gold uint32, items, questItems map[uint8]lootItem, normalSlotCount uint8) bool {
	if s == nil {
		return false
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	entry, _, ok := s.bonesEntryLocked(mapID, instanceID, low)
	if !ok || entry.lootFilled {
		return false
	}
	entry.lootFilled = true
	entry.lootGold = gold
	entry.lootLevel = 0
	entry.lootItems = copyLootItems(items)
	entry.lootQuestItems = copyLootItems(questItems)
	entry.lootNormalSlotCount = normalSlotCount
	return true
}

// syncBonesLoot writes a take back to the bones entry (the loot window
// is a view of bones->loot, Player.cpp:8724, so money and item takes
// land on the shared object, not only on the viewer's copy).
func (s *Server) syncBonesLoot(mapID, instanceID uint32, low uint32, gold uint32, items, questItems map[uint8]lootItem) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	entry, _, ok := s.bonesEntryLocked(mapID, instanceID, low)
	if !ok || !entry.lootFilled {
		return
	}
	entry.lootGold = gold
	entry.lootItems = copyLootItems(items)
	entry.lootQuestItems = copyLootItems(questItems)
}

// clearBonesLoot runs the fully-looted release leg on a bones object
// (WorldSession::DoLootRelease corpse arm, LootHandler.cpp:311-322):
// loot.clear() plus the CORPSE_DYNFLAG_LOOTABLE strip. The level stash
// stays consumed (Player.cpp:8731), so a later re-open rolls gold
// from 0, as in C++.
func (s *Server) clearBonesLoot(mapID, instanceID uint32, low uint32) {
	if s == nil {
		return
	}
	s.corpseMu.Lock()
	defer s.corpseMu.Unlock()
	entry, _, ok := s.bonesEntryLocked(mapID, instanceID, low)
	if !ok {
		return
	}
	entry.lootFilled = false
	entry.lootGold = 0
	entry.lootItems = nil
	entry.lootQuestItems = nil
	entry.dynamicFlags &^= corpseDynFlagLootable
}

func copyLootItems(in map[uint8]lootItem) map[uint8]lootItem {
	if in == nil {
		return nil
	}
	out := make(map[uint8]lootItem, len(in))
	for slot, it := range in {
		out[slot] = it
	}
	return out
}

// expireCorpseBones despawns bones past the 60-minute horizon
// (Map::RemoveOldCorpses, Map.cpp:4693-4701: RemoveCorpse + delete) and
// drops their GUID mappings, returning the number removed. The destroy
// goes to players near the bones, not near any session.
func (s *Server) expireCorpseBones(ctx context.Context) int {
	if s == nil {
		return 0
	}
	now := time.Now()
	s.corpseMu.Lock()
	var expired []corpseBonesEntry
	var expiredKeys []corpseKey
	for key, entry := range s.corpseBones {
		if entry == nil || now.Sub(entry.createdAt) < corpseBonesExpiry {
			continue
		}
		expired = append(expired, *entry)
		expiredKeys = append(expiredKeys, key)
	}
	for _, key := range expiredKeys {
		low := s.corpseBones[key].lowGUID
		delete(s.corpseBones, key)
		delete(s.corpseGUIDByOwner, key)
		delete(s.corpseOwnerByGUID, corpseGUIDKey{mapID: key.mapID, instanceID: key.instanceID, low: low})
	}
	s.corpseMu.Unlock()
	for i, entry := range expired {
		key := expiredKeys[i]
		updates := protocol.NewUpdateData()
		updates.AddOutOfRangeGUID(uint64(0xF101)<<48 | uint64(entry.lowGUID))
		if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
			s.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), key.mapID, key.instanceID, entry.x, entry.y, entry.z, nil)
		}
	}
	return len(expired)
}

func (s *Server) buildNearbyCorpseUpdates(ctx context.Context, state playerState, phaseMask uint32) (*protocol.Packet, int, error) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || s.Config.VisibilityDistanceContinents <= 0 {
		return nil, 0, nil
	}
	distance := float64(s.Config.VisibilityDistanceContinents)
	// Reference Map::LoadCorpseData (Map.cpp:4551-4583) scopes the map's
	// corpse grid by (mapId, instanceId): corpses from another dungeon
	// instance of the same map are never visible.
	// Map::LoadCorpseData (Map.cpp:4567-4571) rejects bones rows — bones
	// are in-world objects, not DB rows, so they render from the bones
	// registry below, never from this query.
	rows, err := s.CharactersStore.DB.QueryContext(ctx, `SELECT guid, mapId, posX, posY, posZ, orientation, displayId, itemCache, bytes1, bytes2, guildId, flags, dynFlags, corpseType, phaseMask
		FROM corpse WHERE mapId = ? AND instanceId = ? AND corpseType <> ? AND posX BETWEEN ? AND ? AND posY BETWEEN ? AND ? ORDER BY guid`, state.Map, state.InstanceID, corpseTypeBones, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance)
	if err != nil {
		if missingTable(err) || isMissingColumn(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer rows.Close()
	type gridCorpse struct {
		guid, mapID, displayID, bytes1, bytes2, guildID, flags, dynamicFlags, corpseType, phaseMask int64
		x, y, z, orientation                                                                        float64
		itemCache                                                                                   string
	}
	var corpses []gridCorpse
	for rows.Next() {
		var c gridCorpse
		if err := rows.Scan(&c.guid, &c.mapID, &c.x, &c.y, &c.z, &c.orientation, &c.displayID, &c.itemCache, &c.bytes1, &c.bytes2, &c.guildID, &c.flags, &c.dynamicFlags, &c.corpseType, &c.phaseMask); err != nil {
			return nil, 0, err
		}
		if c.guid <= 0 || uint32(c.mapID) != state.Map || uint32(c.phaseMask)&phaseMask == 0 || math.Hypot(c.x-float64(state.X), c.y-float64(state.Y)) > distance || !validMovementPosition(float32(c.x), float32(c.y), float32(c.z), float32(c.orientation)) {
			continue
		}
		corpses = append(corpses, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// The equipment cache rides the same packing as the fresh-corpse visual
	// (Corpse::LoadCorpseFromDB restores CORPSE_FIELD_ITEM from the stored
	// row, Corpse.cpp:155-159). Rows written since the packed format landed
	// carry _ConcatFields output (Corpse.cpp:107, Object.cpp:143-148): 19
	// space-separated DisplayInfoID | (InventoryType << 24) values, read
	// verbatim with no template lookup. Older rows carry the Go-original
	// loadEquipmentCache string (23 slots x entry/enchantment pairs);
	// those still resolve through one batched template lookup, told apart
	// by field count (19 packed vs 46 legacy).
	var worldDB *sql.DB
	if s.WorldStore != nil {
		worldDB = s.WorldStore.DB
	}
	caches := make([][corpseItemSlots]uint32, len(corpses))
	for i, c := range corpses {
		if fields := strings.Fields(c.itemCache); len(fields) == corpseItemSlots {
			for slot := 0; slot < corpseItemSlots; slot++ {
				if v, err := strconv.ParseUint(fields[slot], 10, 32); err == nil {
					caches[i][slot] = uint32(v)
				}
			}
		}
	}
	if worldDB != nil {
		parsed := make([][]uint32, len(corpses))
		entries := make([]uint32, 0, len(corpses)*corpseItemSlots)
		seen := make(map[uint32]bool)
		for i, c := range corpses {
			if len(strings.Fields(c.itemCache)) == corpseItemSlots {
				continue
			}
			fields := strings.Fields(c.itemCache)
			parsed[i] = make([]uint32, corpseItemSlots)
			for slot := 0; slot < corpseItemSlots; slot++ {
				base := slot * 2
				if base >= len(fields) {
					continue
				}
				entry, err := strconv.ParseUint(fields[base], 10, 32)
				if err != nil || entry == 0 {
					continue
				}
				parsed[i][slot] = uint32(entry)
				if !seen[uint32(entry)] {
					seen[uint32(entry)] = true
					entries = append(entries, uint32(entry))
				}
			}
		}
		if len(entries) > 0 {
			placeholders := strings.Repeat("?,", len(entries))
			query := "SELECT entry, displayid, InventoryType FROM item_template WHERE entry IN (" + placeholders[:len(placeholders)-1] + ")"
			args := make([]any, len(entries))
			for i, entry := range entries {
				args[i] = entry
			}
			if qrows, err := worldDB.QueryContext(ctx, query, args...); err == nil {
				byEntry := make(map[uint32][2]uint32, len(entries))
				for qrows.Next() {
					var entry, display, invType uint32
					if qrows.Scan(&entry, &display, &invType) == nil {
						byEntry[entry] = [2]uint32{display, invType}
					}
				}
				_ = qrows.Close()
				for i := range corpses {
					if parsed[i] == nil {
						continue
					}
					for slot := 0; slot < corpseItemSlots; slot++ {
						if v, ok := byEntry[parsed[i][slot]]; ok {
							caches[i][slot] = v[0] | (v[1] << 24)
						}
					}
				}
			}
		}
	}
	updates := protocol.NewUpdateData()
	count := 0
	for i, c := range corpses {
		guid := uint64(c.guid)
		ownerGUID := guid
		if c.corpseType == int64(corpseTypeBones) {
			ownerGUID = 0
		}
		// Player::CreateCorpse (Player.cpp:4822) mints the corpse GUID
		// from the map's sequence (Map::GenerateLowGuid), and
		// LoadCorpseData (:4576) re-mints one per loaded row — the low
		// 32 bits are never the owner GUID.
		corpseGUID := uint64(0xF101)<<48 | uint64(s.corpseLowGUID(state.Map, state.InstanceID, guid))
		block := buildCorpseCreateBlockWithFields(corpseGUID, corpseObjectFields{OwnerGUID: ownerGUID, DisplayID: uint32(c.displayID), Items: caches[i], Bytes1: uint32(c.bytes1), Bytes2: uint32(c.bytes2), GuildID: uint32(c.guildID), Flags: uint32(c.flags), DynamicFlags: uint32(c.dynamicFlags)}, float32(c.x), float32(c.y), float32(c.z), float32(c.orientation))
		updates.AddUpdateBlock(block)
		count++
	}
	// Bones live in the registry, not the DB (Map::_corpseBones,
	// Map.cpp:4605-4606): a player arriving after the conversion sees
	// them here, where the row query above cannot reach them.
	if s != nil {
		s.corpseMu.Lock()
		bones := make([]corpseBonesEntry, 0, len(s.corpseBones))
		for key, entry := range s.corpseBones {
			if entry == nil || key.mapID != state.Map || key.instanceID != state.InstanceID {
				continue
			}
			bones = append(bones, *entry)
		}
		s.corpseMu.Unlock()
		for _, b := range bones {
			if b.phaseMask&phaseMask == 0 || math.Hypot(float64(b.x)-float64(state.X), float64(b.y)-float64(state.Y)) > distance || !validMovementPosition(b.x, b.y, b.z, b.orientation) {
				continue
			}
			corpseGUID := uint64(0xF101)<<48 | uint64(b.lowGUID)
			block := buildCorpseCreateBlockWithFields(corpseGUID, corpseObjectFields{DisplayID: b.displayID, Bytes1: b.bytes1, Bytes2: b.bytes2, GuildID: b.guildID, Flags: b.flags, DynamicFlags: b.dynamicFlags}, b.x, b.y, b.z, b.orientation)
			updates.AddUpdateBlock(block)
			count++
		}
	}
	if count == 0 {
		return nil, 0, nil
	}
	packet, err := updates.BuildPacket(0)
	return packet, count, err
}

func isBattlegroundMap(mapID uint32) bool {
	switch mapID {
	case 30, 489, 529, 559, 562, 566, 572, 607, 617, 618, 628:
		return true
	default:
		return false
	}
}

func (s *session) isDeadOrGhost() bool {
	return s == nil || s.player == nil || (s.player.Health == 0 && s.player.MaxHealth > 0) || s.player.PlayerFlags&playerFlagGhost != 0
}

// copseReclaimDelay mirrors the static table in Player.cpp:177.
var copseReclaimDelay = [maxDeathCount]uint32{30, 60, 120}

func ClampLoadedDeathExpireTime(now, deathExpire int64) int64 {
	if deathExpire > now+int64(maxDeathCount)*deathExpireStepSeconds {
		return now + int64(maxDeathCount)*deathExpireStepSeconds - 1
	}
	return deathExpire
}

func CalculateLoadedCorpseReclaimDelay(now, deathExpire, ghostTime int64, reclaimEnabled bool) (uint32, bool) {
	if ghostTime > deathExpire {
		return 0, false
	}
	count := uint64(0)
	if reclaimEnabled && deathExpire > ghostTime {
		count = uint64(deathExpire-ghostTime) / deathExpireStepSeconds
		if count >= maxDeathCount {
			count = maxDeathCount - 1
		}
	}
	expected := ghostTime + int64(copseReclaimDelay[count])
	if now >= expected {
		return 0, false
	}
	return uint32(expected - now), true
}

// corpseReclaimDelaySeconds mirrors Player::GetCorpseReclaimDelay: PvE deaths
// with Death.CorpseReclaimDelay.PvE disabled return 0; PvP deaths with the PvP
// option disabled still use the first table entry. The death count is derived
// from deathExpireTime the same way the reference derives it.
func (s *session) corpseReclaimDelaySeconds(pvp bool) uint32 {
	if pvp {
		if !s.server.Config.DeathCorpseReclaimDelayPvP {
			return copseReclaimDelay[0]
		}
	} else if !s.server.Config.DeathCorpseReclaimDelayPvE {
		return 0
	}
	now := time.Now().Unix()
	count := uint64(0)
	if deathExpire := s.deathExpireTime; deathExpire > 0 && now < deathExpire-1 {
		count = uint64(deathExpire-1-now) / deathExpireStepSeconds
	}
	if count >= maxDeathCount {
		count = maxDeathCount - 1
	}
	return copseReclaimDelay[count]
}

// updateCorpseReclaimDelay mirrors Player::UpdateCorpseReclaimDelay: each death
// within the deathExpireTime window pushes the expire time one further step
// into the future, capped at MAX_DEATH_COUNT steps.
func (s *session) updateCorpseReclaimDelay(pvp bool) {
	if pvp && !s.server.Config.DeathCorpseReclaimDelayPvP {
		return
	}
	if !pvp && !s.server.Config.DeathCorpseReclaimDelayPvE {
		return
	}
	now := time.Now().Unix()
	if s.deathExpireTime > now {
		count := uint64(s.deathExpireTime-now)/deathExpireStepSeconds + 1
		if count < maxDeathCount {
			s.deathExpireTime = now + int64(count+1)*deathExpireStepSeconds
		} else {
			s.deathExpireTime = now + maxDeathCount*deathExpireStepSeconds
		}
	} else {
		s.deathExpireTime = now + deathExpireStepSeconds
	}
}

// sendCorpseReclaimDelay mirrors Player::SendCorpseReclaimDelay: one u32
// remaining time in milliseconds.
func (s *session) sendCorpseReclaimDelay(delay uint32) {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(delay * 1000)
	_ = s.write(uint16(protocol.OpcodeSMSG_CORPSE_RECLAIM_DELAY), packet.Bytes(), true)
}

func CorpseReleaseTimerRequired(instanceType uint32) bool {
	return instanceType == 0
}

func ShouldConvertLoadedCorpseToBones(playerMap, corpseMap uint32, alive bool) bool {
	return alive && playerMap == corpseMap
}

// spiritOfRedemptionSpellID is the Spirit of Redemption form spell cast by
// the Unit::Kill talent arm (Unit.cpp:11313); its DBC duration is 15s.
const (
	spiritOfRedemptionSpellID    uint32 = 27827
	spiritOfRedemptionDurationMs uint32 = 15000
)

// killPlayer mirrors Player::KillPlayer for the lethal-damage call site: root
// the corpse in place, keep health at zero, raise the release timer flag on
// non-instance maps (the Go server has no instance maps), start the 6 minute
// auto-release timer, and notify the client of the corpse reclaim delay.
// killer is the killing player session — the Go analog of Unit::Kill's
// attacker->GetCharmerOrOwnerPlayerOrPlayerItself() arm (Unit.cpp:11279).
// It is nil for wild-creature, fall-to-void, and GM kills; environmental
// self-damage (lava, drowning, fatigue, falls) passes the victim session
// itself, since C++ deals it as attacker == victim (Player.cpp:784).
// pvpDeath carries Unit::Kill's attacker arm (Unit.cpp:11341-11343): true when
// the killer resolves to a player (attacker->GetCharmerOrOwnerPlayerOrPlayerItself()),
// feeding the corpse type and the reclaim delay. A non-nil killer forces it
// true inside (the owner-session resolution is the same arm), so callers
// passing a pet owner's session need not set it.
// durabilityLoss carries Unit::Kill's durabilityLoss param (Unit.h:940,
// default true from DealDamage): false for GM .die/.damage (cs_misc.cpp:608,
// 2116) and for non-fall environmental deaths (Player::EnvironmentalDamage
// passes false and re-applies the loss only for DAMAGE_FALL,
// Player.cpp:793-803). The on-death gate itself is
// (durabilityLoss && !pvp && !inBG) || (pvp && DurabilityLoss.InPvP)
// (Unit.cpp:11350-11357); the rate is Config.DurabilityLossOnDeath
// (World.cpp:606-617).
func (s *session) killPlayer(ctx context.Context, killer *session, pvpDeath, durabilityLoss bool) {
	if s.player == nil || s.player.Health > 0 {
		return
	}
	// Unit::Kill Spirit of Redemption gate (Unit.cpp:11298-11301): a priest
	// with the talent defers death — the JUST_DIED cascade below is skipped
	// now and runs when the spirit form fades (HandleSpiritOfRedemption
	// remove leg, SpellAuraEffects.cpp:1527-1553, "die at aura end"). The
	// Unit::Kill post-SoR arms (PvP-death flag, BG handlers, duel interrupt,
	// durability) still run at the original death: the C++ flow gates only
	// setDeathState(JUST_DIED) on spiritOfRedemption.
	spiritOfRedemption := s.hasSpiritOfRedemptionTalent()
	// Player::KillPlayer (Player.cpp:4771-4772): a flying player who is not
	// on a transport starts a fall — "don't create corpse at this moment,
	// player might be falling" (Player.cpp:4797), so the corpse spawns at
	// ground level at release. The Go bridge forces the client's fall state
	// (the SMSG_MOVE_NORMAL_FALL + MSG_MOVE_FALLING broadcast precedent from
	// the feather-fall remove leg); the client lands and MSG_MOVE_FALL_LAND
	// carries the grounded position. Skipped while the spirit form holds:
	// Unit::Kill gates setDeathState(JUST_DIED) on spiritOfRedemption, so
	// KillPlayer only runs at the deferred death.
	if !spiritOfRedemption && s.player.TransportGUID == 0 {
		if mi, ok := s.lastMovementInfoSnapshot(); ok && mi.Flags&movementFlying != 0 {
			s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_NORMAL_FALL))
			s.broadcastLoginMovementState(protocol.OpcodeMSG_MOVE_FALL_LAND, movementFalling)
		}
	}
	// Unit::setDeathState(JUST_DIED) (Unit.cpp:8901-8921) runs
	// CombatStop (Unit.cpp:5809-5826: AttackStop, RemoveAllAttackers,
	// SendAttackSwingCancelAttack, EndAllPvECombat/SuppressPvPCombat)
	// with the ThreatManager::RemoveMeFromThreatLists leg
	// (ThreatManager.cpp:690-697): a dead player stops being a victim
	// in every creature/pet threat table and drops its own attack
	// target, so melee cannot auto-resume on resurrect and the
	// InCombat flag clears. stopPlayerCombat is the victim-side
	// CombatStop twin. Placed first, matching the C++ order where
	// setDeathState (and its CombatStop) runs inside Unit::Kill ahead
	// of all post-kill processing. Skipped while the spirit form
	// holds: the player is still alive, so the CombatStop leg runs at
	// the deferred death.
	if !spiritOfRedemption && s.server != nil {
		s.stopPlayerCombat(ctx)
		// Unit::setDeathState(JUST_DIED) (Unit.cpp:8956) fires
		// zoneScript->OnUnitDeath(this) inside setDeathState, after
		// the CombatStop leg above — the instance-script player-death
		// fire lands here, at the same point the creature funnel
		// fires it. Skipped while the spirit form holds: the
		// deferred death fires it in completeSpiritOfRedemptionDeath.
		s.server.triggerInstanceUnitDeathPlayer(ctx, s)
	}
	// Unit::setDeathState(JUST_DIED) (Unit.cpp) runs inside Unit::Kill via
	// InterruptNonMeleeSpells(false): a PREPARING generic cast is cancelled
	// (Spell::cancel, Spell.cpp:3210-3225 — CancelGlobalCooldown refunds the
	// GCD, then SendInterrupted + SendCastResult(SPELL_FAILED_INTERRUPTED)),
	// and an active channel is broken the same way. interruptCurrentCast /
	// interruptCurrentChannel carry exactly those legs, so death no longer
	// leaves a phantom GCD or a running cast bar. Deferred while the spirit
	// form holds: the JUST_DIED cascade runs at
	// completeSpiritOfRedemptionDeath instead.
	if !spiritOfRedemption {
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
		s.cancelNextSwingSpell()
		// Unit::setDeathState(JUST_DIED) (Unit.cpp:8909-8911) runs
		// InterruptNonMeleeSpells(false), which interrupts the
		// CURRENT_AUTOREPEAT_SPELL too (Unit.cpp:3219-3220).
		s.cancelAutoRepeatOnInterrupt()
	}
	// Player::setDeathState(JUST_DIED), Player.cpp:1412 — RemovePet(nullptr,
	// PET_SAVE_NOT_IN_SLOT, true) dismisses the pet at death rather than
	// keeping it alive on a dead owner; RemoveGhoul has no Go analog (no
	// raised-ghoul pet model). Placed before the penalty/achievement legs,
	// matching the C++ relative order. Skipped while the spirit form holds.
	if !spiritOfRedemption {
		// TempSummon::Update (TemporarySummon.cpp:66-170) has no
		// owner-death arm: a spell-duration guardian outlives its dead
		// owner, unlike the genuine pet RemovePet dismisses (Player.cpp:
		// 1412). Totems keep their owner-death unsummon below
		// (Totem::Update, Totem.cpp:47-57).
		if !s.activePetIsTemporaryGuardian(ctx) {
			s.unsummonPet(ctx, petSaveNotInSlot)
		}
		// Totem::Update (Totem.cpp:47-57): a totem unsummons when its
		// owner is not alive. Go runs no per-totem update tick, so the
		// owner-death unsummon lands in the JUST_DIED cascade next to the
		// pet dismissal, skipped while the spirit form holds like the
		// rest of the cascade. Previously totems kept pulsing on a corpse.
		s.destroyAllTotems()
	}
	// Player::setDeathState(JUST_DIED), Player.cpp:1385-1404: drunkenness is
	// cleared (SetDrunkValue(0)), banked combo points are lost
	// (ClearComboPoints), and any pending resurrect request is dropped
	// (ClearResurrectRequestData). Skipped while the spirit form holds: the
	// JUST_DIED cascade, including its setDeathState, is deferred until the
	// spirit fades (Unit.cpp:11298-11301).
	if !spiritOfRedemption {
		s.player.DrunkenState = 0
		s.clearSessionComboPoints()
		// Unit::setDeathState (Unit.cpp:8909-8911): combo points other
		// players banked on the dying player are lost too
		// (ClearComboPointHolders).
		if s.server != nil {
			s.server.clearComboPointHolders(s.playerGUID)
		}
		// Unit::setDeathState JUST_DIED (Unit.cpp:8927): reactive timers
		// drop (ClearAllReactives).
		s.clearAllReactives()
		s.resurrection = nil
	}
	// Unit::Kill (Unit.cpp:11279, 11341-11343): the PvP-death flag is
	// SetPvPDeath(player != nullptr), where player is the attacker's
	// GetCharmerOrOwnerPlayerOrPlayerItself — a player-owned pet's kill is a
	// PvP death (corpse type, reclaim delay, durability gate). killer
	// arrives here as exactly that resolution (nil for wild
	// creatures/environment), so a non-nil killer marks the death PvP even
	// when the caller passed pvpDeath=false. Placed after the pet arm,
	// matching the C++ relative order. Runs at the original death even under
	// Spirit of Redemption (Unit.cpp:11344-11346: "at original death (not at
	// SpiritOfRedemtionTalent timeout)").
	if killer != nil {
		pvpDeath = true
	}
	s.pvpDeath = pvpDeath
	if !spiritOfRedemption {
		// Player::KillPlayer (Player.cpp:4783): the release-timer byte flag
		// is set only on non-instanceable maps AND while the victim carries
		// no SPELL_AURA_PREVENT_RESURRECTION (314) aura — ApplyModFlag with a
		// false leg clears it, so a 314-marked death hides "Release Spirit".
		instanceable := false
		if s.server != nil && s.server.Data != nil {
			if mapInfo, found, err := s.server.Data.Map(s.player.Map); err == nil && found {
				instanceable = mapInfo.InstanceType != 0
			}
		}
		if s.playerLoaded && !instanceable && !s.hasAuraType(spellAuraPreventResurrection) {
			s.player.PlayerFieldBytes |= playerFieldByteReleaseTimer
		} else if s.playerLoaded {
			s.player.PlayerFieldBytes &^= playerFieldByteReleaseTimer
		}
		s.deathTimer = time.Now().Add(autoRepopDelay)
	}
	if !spiritOfRedemption {
		s.updateAchievementCriteria(criteriaTypeDeath, 0, 1)
		s.updateAchievementCriteria(criteriaTypeDeathAtMap, s.player.Map, 1)
		if s.server != nil && s.server.Data != nil {
			if mapInfo, found, err := s.server.Data.Map(s.player.Map); err == nil && found && mapInfo.InstanceType != 0 {
				s.updateAchievementCriteria(criteriaTypeDeathInDungeon, 0, 1)
			}
		}
		s.resetAchievementCriteriaByCondition(criteriaConditionNoDeath, 0)
	}
	if s.server != nil {
		// Unit::Kill battleground tail (Unit.cpp:11446-11455): bg
		// HandleKillPlayer runs only for a killer-side player who is
		// InBattleground (Player.h:1906: bgInstanceID != 0) with a live
		// battleground — the victim-map dispatch below used to fire on
		// any death on a BG map, so a wild-creature or environment kill
		// of a BG-map player decremented AV/IC reinforcements and
		// dropped WSG/EOTS flags where C++ fires nothing (player is
		// nil), and a killer with no BG instance (GM-teleported pair
		// on a BG map) fired the same arms. Suicide is NOT excluded:
		// the tail gates on player only, so a self-kill still runs
		// HandleKillPlayer. SA has no C++ override, so its call now
		// carries only the base score leg below.
		// The manager-lookup-miss half of GetBattleground() has no Go
		// equivalent (per-map state is getOrCreate), the HandleKillUnit
		// residual. WG is NOT gated here: Battlefield HandleKill is
		// the separate player && attacker != victim arm and its handler
		// already nil-gates the killer.
		if killer != nil && killer.bgData.InstanceID != 0 {
			if !IsArenaMap(s.player.Map) {
				// Base Battleground::HandleKillPlayer leg
				// (Battleground.cpp:1662-1694): victim deaths, killer
				// honorable kills + killing blows, same-team
				// at-distance honorable kills — battleground_scores.go.
				// Arena matches keep their own ArenaPlayerScore table
				// and their packet serializes killing blows only, so
				// the base deaths/honorable-kills rows stay out here;
				// the arena killing-blows leg runs below, on the
				// killer's arena, for every lethal path.
				s.server.recordBGKillPlayerScores(s, killer)
				// Battleground.cpp:1690-1691: non-arena victims take
				// UNIT_FLAG_SKINNABLE so the insignia can be removed;
				// buildPlayerRepop already clears it at repop.
				s.player.UnitFlags |= unitFlagSkinnable
			}
			wsgMap := s.server.carriedWSGMap(s.playerGUID)
			if wsgMap == 0 {
				wsgMap = s.player.Map
			}
			s.server.handleWSGPlayerDeath(s, wsgMap)
			eotsMap := s.server.carriedEOTSMap(s.playerGUID)
			if eotsMap == 0 {
				eotsMap = s.player.Map
			}
			s.server.handleEOTSPlayerDeath(s, eotsMap)
			s.server.handleAVPlayerDeath(s)
			s.server.handleSAPlayerDeath(s)
			s.server.handleICPlayerDeath(s)
			// Arena::HandleKillPlayer (Arena.cpp:118-126) base leg:
			// the killer's arena killing-blows credit. Placed ahead
			// of the victim-arena alive/win arm below, mirroring the
			// C++ base-scores-then-world-state order; the helper
			// no-ops for non-arena killers.
			s.server.creditArenaKillingBlow(s, killer)
			s.server.handleArenaPlayerDeath(s)
		}
		s.server.handleWGPlayerDeath(s, killer)
	}
	// Unit::Kill proc legs (Unit.cpp:11257-11272) and the per-kill
	// GET_KILLING_BLOWS criteria (Unit.cpp:11277-11279) — must run before
	// the aura/combat removal below ("Proc auras on death — must be before
	// aura/combat remove", and the killing-blow update "before setDeathState
	// to be able to require auras on target"). C++ order: KILL on the
	// killer/owner, KILLED on the victim, DEATH on the victim, then the
	// killing-blow criteria. The pet-owner (KILL, NONE) arm is covered by the
	// killer leg: pet kills already arrive here with killer = the owner
	// session. killerGUID doubles as the victim-side actor GUID (the
	// damage-path convention); nil-killer environment kills pass 0.
	var killerGUID uint64
	if killer != nil {
		killerGUID = killer.playerGUID
		// Player::isHonorOrXPTarget (Player.cpp:24064-24083): the victim
		// must be above the killer's gray level; player victims are never
		// critters or totems, and a nonzero MinCreatureScaledXpRatio lets
		// gray-level victims pass (Player.cpp:24070).
		victimHonorTarget := false
		if s.player != nil && killer.player != nil {
			victimHonorTarget = uint32(s.player.Level) > grayLevel(uint32(killer.player.Level)) || creatureScaledXPRatio(s.server) > 0
		}
		killer.procKillAuraTriggers(ctx, s.playerGUID, killer.playerGUID, procFlagKill, victimHonorTarget)
	}
	attackerHonorTarget := false
	if killer != nil && killer.player != nil && s.player != nil {
		attackerHonorTarget = uint32(killer.player.Level) > grayLevel(uint32(s.player.Level)) || creatureScaledXPRatio(s.server) > 0
	}
	s.procKillAuraTriggers(ctx, killerGUID, killerGUID, procFlagKilled, attackerHonorTarget)
	s.procKillAuraTriggers(ctx, s.playerGUID, s.playerGUID, procFlagDeath, true)
	if killer != nil {
		killer.creditKillingBlowCriteria(s)
	}
	// Player::setDeathState JUST_DIED (Player.cpp:1411-1416) + the Unit::Kill
	// Spirit of Redemption arm (Unit.cpp:11302-11310): the priceless-
	// resurrection fill runs BEFORE the death aura strip (both C++ sites fill
	// before Unit::setDeathState/RemoveAllAurasOnDeath remove the soulstone
	// buff), and only when the field is currently 0. Go's clearActiveAuras
	// never touches the player field, so one fill here covers both C++ sites
	// (the second C++ fill is a no-op once the field is set).
	if s.player.SelfResSpell == 0 {
		s.player.SelfResSpell = s.getResurrectionSpellId(ctx)
	}
	// AuraEffect::HandleChannelDeathItem remove-by-death leg
	// (SpellAuraEffects.cpp:4690): 86 auras on the dying player grant their
	// items to the aura casters; runs before RemoveAllAurasOnDeath's strip.
	s.server.grantChannelDeathItems(ctx, s.loadedAuras(), uint32(s.player.Level), false, 0, 0)
	if spiritOfRedemption {
		// Unit::Kill Spirit of Redemption arm (Unit.cpp:11302-11314):
		// RemoveAllAurasOnDeath, then CastSpell(27827). Go holds no passive
		// aura instances, so clearActiveAuras is the RemoveAllAurasOnDeath
		// analog over the applied set; the talent passive survives in the
		// learned-spell list. PLAYER_SELF_RES_SPELL needs no save/restore:
		// clearActiveAuras never touches the player field. The 27827 apply leg
		// (stand state + SetHealth(1)) lives in applyAuraWithDuration.
		// The death-persistent exemption rides clearDeathAuras
		// (Unit::RemoveAllAurasOnDeath, Unit.cpp).
		s.clearDeathAuras()
		s.applyAuraWithDuration(spiritOfRedemptionSpellID, spiritOfRedemptionDurationMs)
	} else {
		s.clearDeathAuras()
		// Unit::setDeathState (Unit.cpp:8921) runs RemoveAllAurasOnDeath,
		// which strips the SPELL_AURA_CONTROL_VEHICLE auras the
		// passengers cast on the dying base (Vehicle.cpp:256-257) and
		// ejects them (SpellAuraEffects.cpp:2987-3008) — but Go models no
		// ride aura for creature passengers and a player passenger may
		// board aura-less through Lua, so the dying base's kit is swept
		// explicitly. The strip above already exited aura-carrying
		// player passengers; the sweep is a no-op for them.
		s.server.exitDeadPlayerBasePassengers(ctx, s.player.Map, s.player.InstanceID, s.playerGUID)
		// Unit::setDeathState(JUST_DIED) (Unit.cpp): SetHealth(0) then
		// SetPower(GetPowerType(), 0) — the current power type is zeroed
		// with health; health is already 0 (killPlayer early-returns above
		// 0). Deferred with the rest of the cascade while the spirit form
		// holds (Unit.cpp:11298-11301).
		if pt := playerPowerType(s.player); int(pt) < len(s.player.Powers) {
			s.player.Powers[pt] = 0
		}
	}
	if !spiritOfRedemption {
		s.clearDiminishings()
		s.stopMirrorTimers()
		s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
	}
	s.sendPlayerUpdate()
	// Player::UpdateCorpseReclaimDelay (Player.cpp:24324) and
	// Player::GetCorpseReclaimDelay (Player.cpp:24353) read the PvP-death
	// flag rather than taking a parameter. Deferred while the spirit form
	// holds: no corpse exists until the real death.
	if !spiritOfRedemption {
		s.updateCorpseReclaimDelay(s.pvpDeath)
		s.sendCorpseReclaimDelay(s.corpseReclaimDelaySeconds(s.pvpDeath))
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET death_expire_time = ? WHERE guid = ?", s.deathExpireTime, s.playerGUID)
		}
	}
	// Unit::Kill (Unit.cpp:11359-11365): a duelist killed by anyone other
	// than the duel opponent (creature, environment, third party — every
	// death that reaches here bypassed the duel-defeat leg) interrupts the
	// duel instead of completing it. Placed with the death-penalty legs,
	// matching the C++ player-victim branch order.
	if s.duelPartner != 0 {
		s.interruptDuel()
	}
	// Unit::Kill (Unit.cpp:11350-11357): the on-death durability gate —
	// (durabilityLoss && !player && !victim->InBattleground()) ||
	// (player && CONFIG_DURABILITY_LOSS_IN_PVP). player is the
	// GetCharmerOrOwnerPlayerOrPlayerItself arm, so killer != nil (already
	// folded into s.pvpDeath above) is its Go twin; InBattleground is
	// bgData.InstanceID != 0 (Player.h:1906). The rate is
	// RATE_DURABILITY_LOSS_ON_DEATH ("DurabilityLoss.OnDeath", default 10.0,
	// World.cpp:606-617: out of [0,100] clamps to 0, then /100). Previously
	// Go hardcoded 0.10, suppressed the loss only on pvpDeath (so BG
	// creature kills wrongly cost durability), and never applied it for
	// PvP kills even with the InPvP config on.
	inBG := s.bgData.InstanceID != 0
	lossRate := 0.0
	lossInPvP := false
	if s.server != nil {
		lossRate = s.server.Config.DurabilityLossOnDeath
		if lossRate < 0 || lossRate > 100 {
			lossRate = 0
		}
		lossRate /= 100
		lossInPvP = s.server.Config.DurabilityLossInPvP
	}
	if (durabilityLoss && !s.pvpDeath && !inBG) || (s.pvpDeath && lossInPvP) {
		s.durabilityLossAll(ctx, lossRate, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_DURABILITY_DAMAGE_DEATH), []byte{}, true)
	}
	s.debug("player killed", "account", s.accountName, "guid", s.playerGUID)
}

// hasSpiritOfRedemptionTalent mirrors the Unit::Kill Spirit of Redemption
// gate (Unit.cpp:11298-11301): the victim carries SPELL_AURA_DUMMY with
// SPELLFAMILY_PRIEST family flags 0x200 (the priest talent passive, spell
// 20711). Go holds no passive aura instances, so the learned passive spell
// list stands in for the applied aura-effect check.
func (s *session) hasSpiritOfRedemptionTalent() bool {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, learned := range s.player.Spells {
		if !learned.Active || learned.Disabled {
			continue
		}
		spell, found, err := s.server.Data.Spell(learned.ID)
		if err != nil || !found {
			continue
		}
		if spell.Attributes&spellAttributePassive == 0 {
			continue
		}
		if spell.SpellFamilyName != spellFamilyPriest || len(spell.SpellFamilyFlags) < 3 || spell.SpellFamilyFlags[2]&0x200 == 0 {
			continue
		}
		for _, eff := range spell.Effects {
			if eff.Aura == spellAuraDummy {
				return true
			}
		}
	}
	return false
}

// completeSpiritOfRedemptionDeath runs the JUST_DIED cascade deferred by the
// Spirit of Redemption arm: AuraEffect::HandleSpiritOfRedemption's remove leg
// (SpellAuraEffects.cpp:1527-1553) calls setDeathState(JUST_DIED) when the
// spirit form fades ("die at aura end"). The Unit::Kill post-SoR arms (PvP
// flag, BG handlers, duel interrupt, durability) already ran at the original
// death and are not repeated. The C++ leg keeps the 1 HP alongside the death
// state; the Go death model keys on Health==0, so health is clamped to keep
// that invariant.
func (s *session) completeSpiritOfRedemptionDeath(ctx context.Context) {
	if s == nil || s.player == nil || s.player.Health == 0 {
		return
	}
	s.player.Health = 0
	if s.server != nil {
		// Deferred Unit::setDeathState(JUST_DIED) CombatStop twin
		// (Unit.cpp:8901-8921 → CombatStop, Unit.cpp:5809-5826): the
		// spirit form kept the player alive, so the attack-stop and
		// InCombat clear land at the deferred death, like the
		// threat-list removal the twin carries.
		s.stopPlayerCombat(ctx)
		// Deferred Unit::setDeathState(JUST_DIED) (Unit.cpp:8956):
		// the instance-script player-death fire, landing at the
		// deferred death — the same point killPlayer fires it for
		// the immediate death.
		s.server.triggerInstanceUnitDeathPlayer(ctx, s)
	}
	// Deferred Unit::setDeathState(JUST_DIED) cascade: the spirit form kept
	// the player alive, so the cast/channel interrupt (and its GCD refund)
	// lands now, matching Unit::Kill's deferred setDeathState.
	s.interruptCurrentCast()
	s.interruptCurrentChannel()
	s.cancelNextSwingSpell()
	// Unit::setDeathState(JUST_DIED) (Unit.cpp:8909-8911) runs
	// InterruptNonMeleeSpells(false), which interrupts the
	// CURRENT_AUTOREPEAT_SPELL too (Unit.cpp:3219-3220).
	s.cancelAutoRepeatOnInterrupt()
	// The deferred cascade keeps the same guardian exemption as the
	// immediate one above (TempSummon::Update has no owner-death arm).
	if !s.activePetIsTemporaryGuardian(ctx) {
		s.unsummonPet(ctx, petSaveNotInSlot)
	}
	// Deferred Totem::Update owner-death unsummon (Totem.cpp:47-57),
	// landing with the deferred JUST_DIED cascade.
	s.destroyAllTotems()
	if s.playerLoaded && s.player.PlayerFieldBytes&playerFieldByteReleaseTimer == 0 {
		s.player.PlayerFieldBytes |= playerFieldByteReleaseTimer
	}
	s.deathTimer = time.Now().Add(autoRepopDelay)
	s.updateAchievementCriteria(criteriaTypeDeath, 0, 1)
	s.updateAchievementCriteria(criteriaTypeDeathAtMap, s.player.Map, 1)
	if s.server != nil && s.server.Data != nil {
		if mapInfo, found, err := s.server.Data.Map(s.player.Map); err == nil && found && mapInfo.InstanceType != 0 {
			s.updateAchievementCriteria(criteriaTypeDeathInDungeon, 0, 1)
		}
	}
	s.resetAchievementCriteriaByCondition(criteriaConditionNoDeath, 0)
	s.clearDiminishings()
	s.stopMirrorTimers()
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
	// The deferred JUST_DIED cascade (Unit.cpp:11298-11301): the death aura
	// strip carries the death-persistent exemption
	// (Unit::RemoveAllAurasOnDeath), and the current power type zeroes with
	// health (Unit::setDeathState, Unit.cpp).
	s.clearDeathAuras()
	// The dying player base's kit sweep, deferred with the rest of the
	// cascade: the passengers stay seated through the spirit form and
	// eject only at the real death, like the immediate-death arm above.
	s.server.exitDeadPlayerBasePassengers(ctx, s.player.Map, s.player.InstanceID, s.playerGUID)
	if pt := playerPowerType(s.player); int(pt) < len(s.player.Powers) {
		s.player.Powers[pt] = 0
	}
	s.sendPlayerUpdate()
	s.updateCorpseReclaimDelay(s.pvpDeath)
	s.sendCorpseReclaimDelay(s.corpseReclaimDelaySeconds(s.pvpDeath))
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET death_expire_time = ? WHERE guid = ?", s.deathExpireTime, s.playerGUID)
	}
	s.debug("player killed (spirit of redemption expired)", "account", s.accountName, "guid", s.playerGUID)
}

// sendForcedMovement sends one of the forced movement packets used by
// Player::SetMovement (SMSG_FORCE_MOVE_ROOT, SMSG_FORCE_MOVE_UNROOT,
// SMSG_MOVE_WATER_WALK, SMSG_MOVE_LAND_WALK): packed GUID plus a zero
// movement counter.
func (s *session) sendForcedMovement(opcode uint16) {
	packet := protocol.NewBuffer(12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(0)
	_ = s.write(opcode, packet.Bytes(), true)
}

// buildPlayerRepop mirrors Player::BuildPlayerRepop: announce the repop with
// SMSG_PRE_RESURRECT, record the corpse at the death location, convert the
// body to a ghost with one health point, switch to water walking, unroot (unless
// the session is logging out, mirroring Player.cpp:4662-4663), and
// send the corpse reclaim delay. The reference ghost auras 8326 (Ghost) and
// buildCorpseCreateBlock constructs an SMSG_UPDATE_OBJECT block creating a visible
// Corpse object in the world, matching TrinityCore Corpse::Create and Corpse::BuildValuesUpdate.
type corpseObjectFields struct {
	OwnerGUID    uint64
	DisplayID    uint32
	Bytes1       uint32
	Bytes2       uint32
	GuildID      uint32
	Flags        uint32
	DynamicFlags uint32
	// Items is the CORPSE_FIELD_ITEM equipment cache (values[11..29]):
	// per-slot DisplayInfoID | (InventoryType << 24) for the 19 equipment
	// slots (Player::CreateCorpse, Player.cpp:4854-4864). Bones visuals
	// leave it zeroed (Map::ConvertCorpseToBones clears every slot).
	Items [19]uint32
}

func buildCorpseCreateBlock(corpseGUID, ownerGUID uint64, displayID uint32, posX, posY, posZ, orientation float32, isBones bool) []byte {
	fields := corpseObjectFields{OwnerGUID: ownerGUID, DisplayID: displayID, Flags: corpseFlagUnk2}
	if isBones {
		fields.OwnerGUID = 0
		fields.Flags |= corpseFlagBones
	}
	return buildCorpseCreateBlockWithFields(corpseGUID, fields, posX, posY, posZ, orientation)
}

func buildCorpseCreateBlockWithFields(corpseGUID uint64, fields corpseObjectFields, posX, posY, posZ, orientation float32) []byte {
	values := make([]uint32, 36)
	values[0] = uint32(corpseGUID)
	values[1] = uint32(corpseGUID >> 32)
	values[2] = 0x81 // TYPEMASK_OBJECT (0x01) | TYPEMASK_CORPSE (0x80)
	values[3] = 0
	values[4] = math.Float32bits(1.0)
	values[6] = uint32(fields.OwnerGUID)
	values[7] = uint32(fields.OwnerGUID >> 32)
	values[10] = fields.DisplayID
	for i := 0; i < 19 && i < len(fields.Items); i++ {
		values[11+i] = fields.Items[i] // CORPSE_FIELD_ITEM + i
	}
	values[30] = fields.Bytes1
	values[31] = fields.Bytes2
	values[32] = fields.GuildID
	values[33] = fields.Flags
	values[34] = fields.DynamicFlags

	mask := protocol.NewUpdateMask(len(values))
	for idx, val := range values {
		if val != 0 {
			_ = mask.Set(idx)
		}
	}
	_ = mask.Set(1) // GUID high

	block := protocol.NewBuffer(128)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(corpseGUID)
	block.WriteU8(7)       // TYPEID_CORPSE
	block.WriteU16(0x0150) // UPDATEFLAG_POSITION (0x0100) | UPDATEFLAG_STATIONARY_POSITION (0x0040) | UPDATEFLAG_LOWGUID (0x0010)
	block.WriteU8(0)       // no transport
	block.WriteF32(posX)
	block.WriteF32(posY)
	block.WriteF32(posZ)
	block.WriteF32(posX)
	block.WriteF32(posY)
	block.WriteF32(posZ)
	block.WriteF32(orientation)
	block.WriteF32(orientation)
	block.WriteU32(uint32(corpseGUID & 0xFFFFFFFF))

	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for i := 0; i < len(values); i++ {
		if mask.Has(i) {
			block.WriteU32(values[i])
		}
	}
	return block.Bytes()
}

func corpseAppearance(player *playerState) (uint32, uint32) {
	if player == nil {
		return 0, 0
	}
	return uint32(player.Race)<<8 | uint32(player.Gender)<<16 | uint32(player.Skin)<<24, uint32(player.Face) | uint32(player.HairStyle)<<8 | uint32(player.HairColor)<<16 | uint32(player.FacialStyle)<<24
}

func corpseFlags(player *playerState, battlegroundLootable bool) uint32 {
	flags := corpseFlagUnk2
	if player != nil {
		if player.PlayerFlags&playerFlagHideHelm != 0 {
			flags |= 0x08
		}
		if player.PlayerFlags&playerFlagHideCloak != 0 {
			flags |= 0x10
		}
	}
	// Player::CreateCorpse (Player.cpp:4845-4846): battleground corpses carry
	// CORPSE_FLAG_LOOTABLE (0x20) so the insignia can be removed. The
	// !InArena() sub-arm rides battlegroundTypeForMap at the bgLootable
	// site; the insignia loot itself is documented unbridged at loot.go:601.
	if battlegroundLootable {
		flags |= corpseFlagLootable
	}
	return flags
}

// corpseItemSlots is EQUIPMENT_SLOT_END (Player.h): the 19 worn-gear slots
// the corpse item cache covers (Player.cpp:4854). The Go equipment string
// carries 23 slots (4 bag slots follow); only the first 19 are packed here.
const corpseItemSlots = 19

// resolveCorpseItemCache builds the CORPSE_FIELD_ITEM equipment cache for a
// corpse visual: per worn slot, DisplayInfoID | (InventoryType << 24)
// (Player::CreateCorpse, Player.cpp:4857-4863). equipment is the
// entry/enchantments pair string from loadEquipmentCache (slots beyond the
// string or with no resolvable template row stay 0). Unlike the mirror-image
// arm, the cache carries no hide-helm/hide-cloak suppression — the client
// applies those from CORPSE_FIELD_FLAGS.
func resolveCorpseItemCache(ctx context.Context, db *sql.DB, equipment string) [corpseItemSlots]uint32 {
	var items [corpseItemSlots]uint32
	fields := strings.Fields(equipment)
	entries := make([]uint32, 0, corpseItemSlots)
	slots := make([]int, 0, corpseItemSlots)
	for slot := 0; slot < corpseItemSlots; slot++ {
		base := slot * 2
		if base >= len(fields) {
			continue
		}
		entry, err := strconv.ParseUint(fields[base], 10, 32)
		if err != nil || entry == 0 {
			continue
		}
		entries = append(entries, uint32(entry))
		slots = append(slots, slot)
	}
	if len(entries) == 0 || db == nil {
		return items
	}
	placeholders := strings.Repeat("?,", len(entries))
	query := "SELECT entry, displayid, InventoryType FROM item_template WHERE entry IN (" + placeholders[:len(placeholders)-1] + ")"
	args := make([]any, len(entries))
	for i, entry := range entries {
		args[i] = entry
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return items
	}
	defer rows.Close()
	byEntry := make(map[uint32][2]uint32, len(entries))
	for rows.Next() {
		var entry, display, invType uint32
		if rows.Scan(&entry, &display, &invType) != nil {
			continue
		}
		byEntry[entry] = [2]uint32{display, invType}
	}
	for i, slot := range slots {
		if v, ok := byEntry[entries[i]]; ok {
			items[slot] = v[0] | (v[1] << 24)
		}
	}
	return items
}

// formatCorpseItemCache renders the CORPSE_FIELD_ITEM cache the way
// Corpse::SaveToDB persists it (Corpse.cpp:107): Object::_ConcatFields
// (Object.cpp:143-148) writes the 19 packed per-slot values as
// space-separated decimals, each followed by a space. LoadCorpseFromDB
// (Corpse.cpp:155-159) reads the same string back verbatim.
func formatCorpseItemCache(items [corpseItemSlots]uint32) string {
	var b strings.Builder
	for _, v := range items {
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte(' ')
	}
	return b.String()
}

func (s *session) spawnCorpseObject(ctx context.Context, displayID uint32, battlegroundLootable bool) {
	if s.player == nil {
		return
	}
	// Player::CreateCorpse (Player.cpp:4822): the corpse GUID comes from
	// the map's sequence, not the owner. The release is the backstop for
	// the buildPlayerRepop call: standing bones from a previous death
	// keep their value while this corpse mints fresh.
	s.server.releaseCorpseGUIDForNewCorpse(s.player.Map, s.player.InstanceID, s.playerGUID)
	corpseGUID := uint64(0xF101)<<48 | uint64(s.server.corpseLowGUID(s.player.Map, s.player.InstanceID, s.playerGUID))
	// Player::BuildPlayerRepop (Player.cpp:4640+): m_corpse anchors corpse-based
	// distance checks while dead (Player::IsAtGroupRewardDistance).
	s.player.hasCorpse = true
	s.player.corpseX, s.player.corpseY, s.player.corpseZ = s.player.X, s.player.Y, s.player.Z
	// The corpse snapshot is the ConvertCorpseToBones source for corpses
	// with no DB row (battlegrounds/arenas, Player.cpp:4867-4868 save-skip):
	// C++ converts the in-memory corpse object, so its map, display and
	// orientation must survive the ghost's later movement.
	s.player.corpseMap, s.player.corpseInstanceID = s.player.Map, s.player.InstanceID
	s.player.corpseDisplayID = displayID
	s.player.corpseOrientation = s.player.Orientation
	bytes1, bytes2 := corpseAppearance(s.player)
	var worldDB *sql.DB
	if s.server != nil && s.server.WorldStore != nil {
		worldDB = s.server.WorldStore.DB
	}
	fields := corpseObjectFields{OwnerGUID: s.playerGUID, DisplayID: displayID, Bytes1: bytes1, Bytes2: bytes2, GuildID: s.player.GuildID, Flags: corpseFlags(s.player, battlegroundLootable), Items: resolveCorpseItemCache(ctx, worldDB, s.player.Equipment)}
	block := buildCorpseCreateBlockWithFields(corpseGUID, fields, s.player.X, s.player.Y, s.player.Z, s.player.Orientation)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		if s.server != nil {
			// Map::ConvertCorpseToBones / corpse AddToMap fan out on the
			// corpse's map (Map.cpp:4627-4670): center on the corpse
			// position, which here equals the player position at creation.
			s.server.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), s.player.Map, s.player.InstanceID, s.player.X, s.player.Y, s.player.Z, s)
		}
	}
}

// despawnCorpseObject mirrors the corpse despawn on the corpse's own map
// (Map::RemoveCorpse): the out-of-range update goes to players near the
// corpse, not near the player.
func (s *session) despawnCorpseObject(mapID, instanceID uint32, x, y, z float32) {
	// Mirror of the m_corpse clear on reclaim/resurrect: corpse-anchored
	// distance checks fall back to the player position.
	if s.player != nil {
		s.player.hasCorpse = false
		s.player.corpseMap, s.player.corpseInstanceID = 0, 0
		s.player.corpseDisplayID, s.player.corpseOrientation = 0, 0
	}
	// The destroy carries the corpse's map-assigned GUID, matching the
	// create block spawnCorpseObject sent.
	corpseGUID := uint64(0xF101)<<48 | uint64(s.server.corpseLowGUID(mapID, instanceID, s.playerGUID))
	updates := protocol.NewUpdateData()
	updates.AddOutOfRangeGUID(corpseGUID)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), mapID, instanceID, x, y, z, s)
		}
	}
}

// buildPlayerRepop converts the dead player into a ghost, persists the corpse record,
// spawns the physical Corpse object into the world grid, and applies ghost visual auras.
// loggingOut mirrors WorldSession::isLogingOut (WorldSession.h:456): during logout
// (m_playerLogout is set before BuildPlayerRepop in WorldSession::LogoutPlayer,
// WorldSession.cpp:502) the MOVE_UNROOT leg is skipped — Player.cpp:4662-4663.
// No-bridge: C++ aborts before the ghost conversion when a corpse already
// exists on the map (Player.cpp:4640-4646); Go converts the previous corpse
// to bones first (Player::CreateCorpse's SpawnCorpseBones arm), so the leg
// is unreachable. When both reclaim-delay configs are off C++ returns -1
// configs are off C++ returns -1 and skips the packet (Player.cpp:24374+4670);
// Go sends a 0ms SMSG_CORPSE_RECLAIM_DELAY instead.
func (s *session) buildPlayerRepop(ctx context.Context, loggingOut bool) {
	if s.player == nil {
		return
	}
	preResurrect := protocol.NewBuffer(12)
	preResurrect.WritePackedGUID(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_PRE_RESURRECT), preResurrect.Bytes(), true)

	// Player::BuildPlayerRepop (Player.cpp:4644-4649): the player cannot
	// have a corpse already on the current map — after the repop auras
	// (20584/8326, cast before the check in C++; re-applied here, a
	// no-op refresh for an already-released ghost), an existing corpse
	// logs and returns with no new corpse, no health=1, no
	// water-walk/unroot and no reclaim-delay packet. Reached on a
	// second repop without reclaim (hearth resurrect while already a
	// ghost, MiscHandler.cpp:1505-1519). The login path is excluded: a
	// CORPSE-state login carries its row by design (repopOnLogin +
	// loadedCorpseRow) and C++ reaches BuildPlayerRepop with no corpse
	// object loaded yet, so the check cannot fire there. BG deaths
	// carry no DB row (save-skip), so the in-memory BG corpse is
	// invisible to this check — that half needs the bones registry.
	if !(s.player.repopOnLogin && s.player.loadedCorpseRow) {
		// Battleground/arena corpses carry no DB row (the
		// Player.cpp:4867-4868 save-skip), so the row check misses them;
		// the session snapshot is the in-memory GetCorpseByPlayer twin
		// there (Map.cpp:4629-4631) — RemovedInsignia's forced repop of
		// a BG victim reaches this branch and must not mint a second
		// corpse over the one about to become bones.
		hasCorpse := false
		if corpse, ok := s.loadCorpse(ctx); ok && corpse.MapID == s.player.Map {
			hasCorpse = true
		} else if s.player.hasCorpse && s.player.corpseMap == s.player.Map {
			hasCorpse = true
		}
		if hasCorpse {
			if s.player.Race == 4 {
				s.applyAura(20584)
			}
			s.applyAura(8326)
			s.debug("buildPlayerRepop: player already has a corpse on this map", "account", s.accountName, "guid", s.playerGUID)
			return
		}
	}

	displayID := uint32(0)
	if s.server.Data != nil {
		if race, found, err := s.server.Data.Race(uint32(s.player.Race)); err == nil && found {
			displayID = race.MaleDisplayID
			if s.player.Gender != 0 {
				displayID = race.FemaleDisplayID
			}
		}
	}

	// Player::CreateCorpse (Player.cpp:4817-4818): the corpse type comes from
	// the PvP-death flag, which the creation consumes.
	corpseType := corpseTypePvE
	if s.pvpDeath {
		corpseType = corpseTypePvP
	}
	s.pvpDeath = false
	// Player::CreateCorpse (Player.cpp:4845-4846): the LOOTABLE flag is a
	// battleground arm (InBattleground(), Player.h:1906); Go mirrors it with
	// the session BG instance id. The visible corpse object always carries
	// these flags; the DB row only exists outside battlegrounds (the
	// BG/arena save-skip at Player.cpp:4867-4868, gated in buildPlayerRepop).
	// Player::CreateCorpse (Player.cpp:4838-4842): the LOOTABLE flag is a
	// battleground arm that excludes arenas (InBattleground() &&
	// !InArena()); battlegroundTypeForMap derives the arena property
	// from the BG map (arena maps 559/562/572/617/618), the same
	// template read the reclaim arena gate uses below.
	bgLootable := s.bgData.InstanceID != 0
	if _, _, isArena, ok := battlegroundTypeForMap(s.player.Map); ok && isArena {
		bgLootable = false
	}
	// Player::CreateCorpse (Player.cpp:4812) opens with SpawnCorpseBones():
	// a second death converts the existing corpse to bones
	// (Map::ConvertCorpseToBones deletes the old row outside the BG/arena
	// save-skip gate and spawns the ownerless bones visual per the
	// DEATH_BONES configs) instead of only deleting the row.
	// convertCorpseToBones is the Go analog; a missing DB row falls
	// through to the session snapshot (battleground/arena corpses convert
	// from the in-memory object, Map.cpp:4629-4631), and only a missing
	// snapshot matches ConvertCorpseToBones' GetCorpseByPlayer miss.
	// Player::LoadCorpse (Player.cpp:18301-18313): a dead login keeps the
	// existing corpse row as-is — its reclaim delay counts from the original
	// time and its PvP type is preserved — instead of the convert+rewrite a
	// fresh death performs.
	keepCorpseRow := s.player.repopOnLogin && s.player.loadedCorpseRow
	// Corpse::Create (Corpse.cpp:72-94) fails on !IsPositionValid()
	// (Trinity::IsValidMapCoord, GridDefines.h:216-234 — finite
	// coordinates within the map half-size, the validMovementPosition
	// twin); Player::CreateCorpse (Player.cpp:4822-4826) returns
	// nullptr and BuildPlayerRepop (Player.cpp:4649-4656) aborts — no
	// ghost, no reclaim-delay packet. The ghost auras were already cast
	// above, so they ride along as in C++. A login keeps its row by
	// design and never reaches CreateCorpse, so the check skips it.
	if !keepCorpseRow && !validMovementPosition(s.player.X, s.player.Y, s.player.Z, s.player.Orientation) {
		if s.player.Race == 4 {
			s.applyAura(20584)
		}
		s.applyAura(8326)
		s.debug("buildPlayerRepop: death position invalid, no corpse created", "account", s.accountName, "guid", s.playerGUID)
		return
	}
	if !keepCorpseRow {
		s.convertCorpseToBones(ctx, false, false)
		// A new corpse object is about to be created below: if the
		// previous death's bones still stand on this map, release the
		// mapping now — the bones keep their low GUID and the new
		// corpse mints fresh (Map.cpp:4653 vs Player.cpp:4822). The
		// release runs before the insert so a concurrent nearby render
		// of the fresh row cannot claim the bones' value first.
		s.server.releaseCorpseGUIDForNewCorpse(s.player.Map, s.player.InstanceID, s.playerGUID)
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && !battlegroundMap(s.player.Map) && !keepCorpseRow {
		// Column order mirrors SaveToDB (Corpse.cpp:102-119): the dynFlags
		// 0 is exact — Player::CreateCorpse never sets CORPSE_FIELD_DYNAMIC_FLAGS
		// on a fresh corpse — and instanceId is the map's instance id
		// (Corpse::SaveToDB writes GetInstanceId(), Object.cpp:1846), 0 on
		// world maps, the dungeon/BG instance id elsewhere.
		// Player::CreateCorpse (Player.cpp:4867-4868) skips the DB save when
		// the map is a battleground or arena — "we do not need to save
		// corpses for BG/arenas" — while still creating the in-memory corpse
		// object (with the LOOTABLE flag for the insignia). The gate is the
		// map's battleground/arena property, not the LOOTABLE flag: arenas
		// carry no LOOTABLE flag (Player.cpp:4841) but still skip the save.
		// itemCache is the packed _ConcatFields string (Corpse.cpp:107):
		// the 19 DisplayInfoID | (InventoryType << 24) values, not the
		// Go-original entry/enchantment pairs.
		bytes1, bytes2 := corpseAppearance(s.player)
		var worldDB *sql.DB
		if s.server.WorldStore != nil {
			worldDB = s.server.WorldStore.DB
		}
		_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_CORPSE",
			s.playerGUID, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, s.player.Map,
			displayID, formatCorpseItemCache(resolveCorpseItemCache(ctx, worldDB, s.player.Equipment)), bytes1, bytes2, s.player.GuildID, corpseFlags(s.player, bgLootable), 0, time.Now().Unix(), corpseType, s.player.InstanceID, s.currentPlayerPhaseMask())
	}

	s.player.PlayerFlags |= playerFlagGhost
	// The release-timer byte is owned by Player::KillPlayer (set/clear,
	// Player.cpp:4783), the login corpse load (Player.cpp:18315), and the
	// SPELL_AURA_PREVENT_RESURRECTION apply/remove legs — neither
	// BuildPlayerRepop nor ResurrectPlayer touches it, so it stays set
	// through repop and resurrect.
	s.player.Health = 1
	s.deathTimer = time.Time{}
	if s.player.Race == 4 { // RACE_NIGHTELF
		s.applyAura(20584) // Wisp Spirit
	}
	s.applyAura(8326) // Ghost
	s.spawnCorpseObject(ctx, displayID, bgLootable)
	s.player.UnitFlags &^= unitFlagSkinnable
	s.sendPlayerUpdate()
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
	if !loggingOut {
		s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT))
	}
	// Player::CalculateCorpseReclaimDelay (Player.cpp:24353): at repop time
	// the PvP leg comes from the corpse type, not the (consumed) flag.
	s.sendCorpseReclaimDelay(s.corpseReclaimDelaySeconds(corpseType != corpseTypePvE))
	s.stopMirrorTimers()
}

// areaFlagNeedFly is AREA_FLAG_NEED_FLY (DBCEnums.h:259): zones flagged
// this way revive the ghost automatically at the graveyard instead of
// leaving it a ghost (Player::RepopAtGraveyard, Player.cpp:5117-5122).
const areaFlagNeedFly uint32 = 0x00001000

// repopAtGraveyard mirrors Player::RepopAtGraveyard (Player.cpp:5109-5157):
// stop the auto-release countdown, locate the graveyard linked to the ghost
// zone, teleport there, point the client corpse map at the graveyard, and in
// battlegrounds automatically queue into the spirit wave. Ghosts that died
// in unreachable spots are revived at the graveyard instead of staying
// ghosts.
func (s *session) repopAtGraveyard(ctx context.Context) {
	if s.player == nil {
		return
	}
	// Reference clears the repop countdown here (m_deathTimer = 0,
	// Player.cpp:5135) so the Player::Update auto-release arm cannot fire a
	// second time for the same death.
	s.deathTimer = time.Time{}

	// Reference auto-revive (Player.cpp:5117-5122): a NEED_FLY zone, a
	// transport, or below the map minimum height teleports with
	// TELE_REVIVE_AT_TELEPORT and converts the corpse to bones first. The
	// transport leg is bridged via s.player.TransportGUID and the NEED_FLY
	// leg via the AreaTable DBC entry of the player's area; the
	// minimum-height leg has no bridge — Go models no terrain min-height, so
	// the homebind fallback on a missing grave (Player.cpp:5151-5152) is
	// unreachable too.
	shouldResurrect := s.player.TransportGUID != 0
	if !shouldResurrect && s.isDeadOrGhost() && s.areaID != 0 && s.server != nil && s.server.Data != nil {
		if area, found, err := s.server.Data.Area(s.areaID); err == nil && found && area.Flags&areaFlagNeedFly != 0 {
			shouldResurrect = true
		}
	}
	if shouldResurrect {
		s.spawnCorpseBones(ctx)
	}

	// Player.cpp:5127-5128 (RepopAtGraveyard): a battleground member's grave
	// comes from bg->GetClosestGraveyard, never the standing map, so an
	// off-map roster member repops to the BG node grave; a standing BG map
	// keeps its own dispatch.
	repopMap := s.player.Map
	if repopMap != ICMapID && repopMap != SAMapID && repopMap != AVMapID && repopMap != WSGMapID && repopMap != ABMapID && repopMap != EOTSMapID {
		if s.hasActiveBGMap(ICMapID) {
			repopMap = ICMapID
		} else if s.hasActiveBGMap(SAMapID) {
			repopMap = SAMapID
		} else if s.hasActiveBGMap(AVMapID) {
			repopMap = AVMapID
		} else if s.hasActiveBGMap(WSGMapID) {
			repopMap = WSGMapID
		} else if s.hasActiveBGMap(ABMapID) {
			repopMap = ABMapID
		} else if s.hasActiveBGMap(EOTSMapID) {
			repopMap = EOTSMapID
		}
	}
	team := playerTeam(s.player.Race)
	var grave wotlk.WorldSafeLoc
	var ok bool
	if loc, found := s.server.closestRepopGraveyard(s.player.X, s.player.Y, s.player.Z, repopMap, team); found {
		grave, ok = loc, true
	} else if s.inWintergraspBattlefield() {
		// Player.cpp:5130-5131: a zone owned by a battlefield takes its
		// grave from Battlefield::GetClosestGraveyard — nearest grave the
		// player's team controls, else nothing. A miss stays put: the
		// battlefield leg replaces the generic lookup, it does not fall
		// through to it. closestWGGraveyard compares against TeamControl
		// (WGTeamAlliance 0 / WGTeamHorde 1), so the team here is
		// teamForRace, not the 469/67 graveyard-faction team above.
		if loc, found := s.server.closestWGGraveyard(teamForRace(s.player.Race), s.player.X, s.player.Y); found {
			grave, ok = loc, true
		}
	} else if !hasBGClosestGraveyardOverride(repopMap) {
		// Player.cpp:5127-5134: a battleground member's grave comes from
		// bg->GetClosestGraveyard alone, with no generic fallback, so an
		// override miss stays put (Player.cpp:5141-5152) instead of
		// falling through to the ObjectMgr lookup; only a map with no
		// override reaches the generic leg.
		grave, ok = s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, repopMap, s.player.Zone, team)
	}
	if ok {
		if s.teleportTo(grave.MapID, grave.X, grave.Y, grave.Z, s.player.Orientation) {
			// Player::RepopAtGraveyard (Player.cpp:5144-5146) sends
			// SMSG_DEATH_RELEASE_LOC whenever the player is still dead
			// after TeleportTo returns — including the far-teleport
			// revive case, where the revive is deferred to the
			// worldport ack (MovementHandler.cpp:145-146). The
			// near-teleport revive arm gates on !IsAlive
			// (Player.cpp:1749), so an alive repop caller (the logout
			// pending-bind path) on a transport is teleported, not
			// cut to half health.
			sendReleaseLoc := func() {
				packet := protocol.NewBuffer(16)
				packet.WriteU32(grave.MapID)
				packet.WriteF32(grave.X)
				packet.WriteF32(grave.Y)
				packet.WriteF32(grave.Z)
				_ = s.write(uint16(protocol.OpcodeSMSG_DEATH_RELEASE_LOC), packet.Bytes(), true)
			}
			if shouldResurrect {
				if s.farTeleportPending {
					// TELE_REVIVE_AT_TELEPORT on a far teleport: C++
					// revives after the map switch, at the worldport ack
					// (MovementHandler.cpp:145-146), not at initiation.
					s.teleportReviveAtAck = true
					if s.isDeadOrGhost() {
						sendReleaseLoc()
					}
				} else if s.isDeadOrGhost() {
					// TELE_REVIVE_AT_TELEPORT: same-map TeleportTo
					// resurrects at half health/mana inside the teleport
					// (Player.cpp:1749-1750).
					s.resurrectPlayer(ctx, 0.5)
				}
			} else if s.isDeadOrGhost() {
				// SMSG_DEATH_RELEASE_LOC is sent only when dead (Player.cpp:5146);
				// the alive homebind-fallback caller skips it.
				sendReleaseLoc()
			}
		}
	} else {
		s.debug("no graveyard found, staying at current location", "account", s.accountName, "guid", s.playerGUID)
	}

	// Reference clears PLAYER_FLAGS_IS_OUT_OF_BOUNDS on every repop
	// (Player.cpp:5155); set by the movement undermap leg (movement.go).
	s.player.PlayerFlags &^= playerFlagOutOfBounds

	// In battlegrounds, automatically queue for wave resurrection.
	// C++ queues only on the queue/gossip opcodes, but the client
	// behavior the queue reproduces is automatic: players are
	// automatically included in the resurrection wave upon appearing
	// at the graveyard, and the time-until-resurrection dialog appears
	// on joining the wave (Wowpedia, "Spirit Guide" — verified
	// 2026-11-21). The queue/gossip opcodes are the re-join paths for
	// ghosts who left the wave, not the entry path; removing this
	// block would leave repopped ghosts out of the wave until they
	// talked to the healer, contradicting the automatic inclusion.
	// The time packet below keeps the client rez timer in sync since
	// these ghosts never sent a query.
	if s.inBattlegroundWaveMap() {
		if s.server != nil {
			s.server.spiritWaveMu.Lock()
			if s.server.spiritReviveQueue == nil {
				s.server.spiritReviveQueue = make(map[uint64]uint64)
			}
			s.server.spiritReviveQueue[s.playerGUID] = 0
			s.server.spiritWaveMu.Unlock()

			s.applyAura(2584) // SPELL_WAITING_FOR_RESURRECT
			s.sendAreaSpiritHealerTime(0, s.server.spiritWaveTimeLeftMs())
		}
	}
}

// playerTeam maps a race to the graveyard faction domain: TEAM_ALLIANCE (469),
// TEAM_HORDE (67), or 0 when the race has no team assignment.
func playerTeam(race uint8) uint32 {
	if isAllianceRace(race) {
		return teamAlliance
	}
	switch race {
	case 2, 5, 6, 8, 10:
		return teamHorde
	}
	return 0
}

// closestRepopGraveyard is the battleground leg of Player::RepopAtGraveyard
// (Player.cpp:5127-5128): when the repop map is a battleground map, the grave
// comes from that battleground's GetClosestGraveyard override, never the
// generic zone lookup. It is deliberately separate from closestGraveyard (the
// ObjectMgr generic path the spirit-healer and corpse-query legs use), so the
// BG overrides do not leak into those legs. A false return on an override map
// stays put (Player.cpp:5141-5152: no grave, no generic fallback); only a map
// with no override falls through to closestGraveyard.
func (s *Server) closestRepopGraveyard(x, y, z float32, mapID, team uint32) (wotlk.WorldSafeLoc, bool) {
	switch mapID {
	case ICMapID:
		return s.closestICGraveyard(x, y, team)
	case SAMapID:
		return s.closestSAGraveyard(x, y, z, team)
	case AVMapID:
		return s.closestAVGraveyard(x, y, team)
	case WSGMapID:
		return s.closestWSGraveyard(team)
	case ABMapID:
		return s.closestABGraveyard(x, y, team)
	case EOTSMapID:
		return s.closestEYGraveyard(x, y, z, team)
	default:
		return wotlk.WorldSafeLoc{}, false
	}
}

// hasBGClosestGraveyardOverride reports whether mapID is one of the six
// battleground maps with a GetClosestGraveyard override (AV/WS/AB/EY/IC/SA);
// arena maps use the base Battleground::GetClosestGraveyard generic leg
// (Battleground.cpp:1777-1779) and have no override.
func hasBGClosestGraveyardOverride(mapID uint32) bool {
	switch mapID {
	case ICMapID, SAMapID, AVMapID, WSGMapID, ABMapID, EOTSMapID:
		return true
	default:
		return false
	}
}

// closestGraveyard mirrors ObjectMgr::GetClosestGraveyard.
func (s *Server) closestGraveyard(ctx context.Context, x, y, z float32, mapID, zoneID, team uint32) (wotlk.WorldSafeLoc, bool) {
	if s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		return wotlk.WorldSafeLoc{}, false
	}
	mapEntry, hasMapEntry, mapErr := s.Data.Map(mapID)
	if mapErr != nil {
		s.debug("graveyard map lookup failed", "map", mapID, "error", mapErr)
		hasMapEntry = false
	}
	if hasMapEntry && mapEntry.AreaTableID != 0 {
		zoneID = s.rootAreaID(mapEntry.AreaTableID)
	} else {
		zoneID = s.rootAreaID(zoneID)
	}
	defaultFor := func() (wotlk.WorldSafeLoc, bool) {
		id := uint32(0)
		switch team {
		case teamAlliance:
			id = defaultGraveyardAlliance
		case teamHorde:
			id = defaultGraveyardHorde
		default:
			return wotlk.WorldSafeLoc{}, false
		}
		loc, found, err := s.Data.WorldSafeLoc(id)
		if err != nil {
			s.debug("default graveyard lookup failed", "error", err)
			return wotlk.WorldSafeLoc{}, false
		}
		return loc, found
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT ID, Faction FROM graveyard_zone WHERE GhostZone = ?", zoneID)
	if err != nil {
		s.debug("graveyard zone query failed", "error", err)
		return defaultFor()
	}
	defer rows.Close()
	locations := make([]wotlk.WorldSafeLoc, 0, 4)
	for rows.Next() {
		var safeLocID, faction uint32
		if err := rows.Scan(&safeLocID, &faction); err != nil {
			continue
		}
		if faction != 0 && team != 0 && faction != team {
			continue
		}
		loc, found, err := s.Data.WorldSafeLoc(safeLocID)
		if err != nil || !found {
			continue
		}
		locations = append(locations, loc)
	}
	if err := rows.Err(); err != nil {
		s.debug("graveyard zone rows failed", "error", err)
	}
	if loc, found := closestGraveyardCandidate(locations, mapID, mapEntry, hasMapEntry, x, y, z); found {
		return loc, true
	}
	return defaultFor()
}

func (s *Server) rootAreaID(areaID uint32) uint32 {
	for depth := 0; areaID != 0 && depth < 8; depth++ {
		area, found, err := s.Data.Area(areaID)
		if err != nil || !found || area.ParentAreaID == 0 {
			return areaID
		}
		areaID = area.ParentAreaID
	}
	return areaID
}

func closestGraveyardCandidate(locations []wotlk.WorldSafeLoc, mapID uint32, mapEntry wotlk.MapEntry, hasMapEntry bool, x, y, z float32) (wotlk.WorldSafeLoc, bool) {
	type candidate struct {
		loc  wotlk.WorldSafeLoc
		dist float32
	}
	var near, entrance *candidate
	var far *wotlk.WorldSafeLoc
	for _, loc := range locations {
		if loc.MapID == mapID {
			dx, dy, dz := loc.X-x, loc.Y-y, loc.Z-z
			dist := dx*dx + dy*dy + dz*dz
			if near == nil || dist < near.dist {
				near = &candidate{loc: loc, dist: dist}
			}
		} else if hasMapEntry && mapEntry.CorpseMapID >= 0 && uint32(mapEntry.CorpseMapID) == loc.MapID && (mapEntry.CorpseX != 0 || mapEntry.CorpseY != 0) {
			dx, dy := loc.X-mapEntry.CorpseX, loc.Y-mapEntry.CorpseY
			dist := dx*dx + dy*dy
			if entrance == nil || dist < entrance.dist {
				entrance = &candidate{loc: loc, dist: dist}
			}
		} else {
			// ObjectMgr::GetClosestGraveyard overwrites unconditionally
			// (ObjectMgr.cpp:6936-6945) — the last iterated far-zone
			// graveyard wins, not the first.
			locCopy := loc
			far = &locCopy
		}
	}
	if near != nil {
		return near.loc, true
	}
	if entrance != nil {
		return entrance.loc, true
	}
	if far != nil {
		return *far, true
	}
	return wotlk.WorldSafeLoc{}, false
}

// handleRepopRequest mirrors WorldSession::HandleRepopRequest: alive players
// and players that are already ghosts are ignored, and the reference
// SPELL_AURA_PREVENT_RESURRECTION aura-type guard (MiscHandler.cpp:66-67) is
// honored via hasAuraType (spell id 58549 previously checked here is the
// Wintergrasp Tenacity spell, not a prevent-resurrection aura); otherwise the
// repop flow (corpse creation, ghost conversion) runs followed by the
// graveyard teleport. The payload carries one bool (CheckInstance) which the
// reference also reads but does not act on.
func (s *session) handleRepopRequest(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU8(); err != nil {
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.player.Health > 0 || s.player.PlayerFlags&playerFlagGhost != 0 || s.hasAuraType(spellAuraPreventResurrection) {
		return true
	}
	// Eluna PLAYER_EVENT_ON_REPOP (35): Eluna::OnRepop
	// (PlayerHooks.cpp:87-92), (event, player), void (CallAllFunctions).
	// WorldSession::HandleRepopRequest (MiscHandler.cpp:84) fires it after the
	// guards, before the pet dismissal and the ghost conversion.
	s.triggerPlayerEvent(ctx, scripting.PlayerEventRepop, s.luaPlayer())
	// WorldSession::HandleRepopRequest, MiscHandler.cpp:87-89 — dismiss any
	// pet before the repop (e.g. a warlock re-summon while dead); C++ calls
	// RemovePet, which never touches TempSummons, so temporary guardians
	// survive the repop like they survive the JUST_DIED cascade.
	if !s.activePetIsTemporaryGuardian(ctx) {
		s.unsummonPet(ctx, petSaveNotInSlot)
	}
	s.buildPlayerRepop(ctx, false)
	s.repopAtGraveyard(ctx)
	return true
}

// updatePlayerDeathTimers runs the reference Player::Update auto-release:
// after six minutes a dead player that has not released is converted to a
// ghost and teleported to the graveyard (Player.cpp:1302). The reference
// skips this on instanceable maps, under SPELL_AURA_PREVENT_RESURRECTION,
// and while ghouled; the Go server has no instance maps and no raised-ghoul
// pet model.
func (s *Server) updatePlayerDeathTimers(ctx context.Context, now time.Time) {
	s.sessionsMu.RLock()
	var due []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if s.Data != nil {
			if mapEntry, found, err := s.Data.Map(sess.player.Map); err == nil && found && mapEntry.InstanceType != 0 {
				continue
			}
		}
		if sess.player.Health == 0 && sess.player.PlayerFlags&playerFlagGhost == 0 && !sess.hasAuraType(spellAuraPreventResurrection) && !sess.deathTimer.IsZero() && !now.Before(sess.deathTimer) {
			due = append(due, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range due {
		sess.buildPlayerRepop(ctx, false)
		sess.repopAtGraveyard(ctx)
	}
}

// updateSpiritHealerResurrectWaves pulses every 30 seconds to resurrect ghosts
// queued at battleground spirit healers.
// Reference: Battleground::_ProcessResurrect (Battleground.cpp:285-345):
// at >= RESURRECTION_INTERVAL (30000ms, Battleground.h:147) the wave arm moves the
// revive queue into the resurrect queue and plays the visuals (the healer casts
// SPELL_SPIRIT_HEAL on itself; each queued player gets SPELL_RESURRECTION_VISUAL);
// more than half a second later the resurrect arm fires ResurrectPlayer(1.0) +
// 6962 + SPELL_SPIRIT_HEAL_MANA + SpawnCorpseBones on each queued player, so the
// client sees the spirit-heal effect on the NPC first.
func (s *Server) updateSpiritHealerResurrectWaves(ctx context.Context, now time.Time) {
	s.spiritWaveMu.Lock()
	if s.lastSpiritWave.IsZero() {
		s.lastSpiritWave = now
	}
	var visuals []spiritWaveDue
	if now.Sub(s.lastSpiritWave) >= 30*time.Second {
		s.lastSpiritWave = now
		if len(s.spiritReviveQueue) != 0 {
			if s.spiritResurrectPending == nil {
				s.spiritResurrectPending = make(map[uint64]spiritWavePending)
			}
			for pGUID, spiritGUID := range s.spiritReviveQueue {
				visuals = append(visuals, spiritWaveDue{playerGUID: pGUID, spiritGUID: spiritGUID})
				s.spiritResurrectPending[pGUID] = spiritWavePending{spiritGUID: spiritGUID, waveAt: now}
			}
			s.spiritReviveQueue = make(map[uint64]uint64)
		}
	}
	var due []spiritWaveDue
	for pGUID, pending := range s.spiritResurrectPending {
		if !now.Before(pending.waveAt.Add(500 * time.Millisecond)) {
			due = append(due, spiritWaveDue{playerGUID: pGUID, spiritGUID: pending.spiritGUID})
			delete(s.spiritResurrectPending, pGUID)
		}
	}
	s.spiritWaveMu.Unlock()

	// Battleground.cpp:305-312: the healer casts SPELL_SPIRIT_HEAL on
	// itself once per healer — the !sh guard fires it for the first
	// in-world ghost at that healer only, ahead of that ghost's visual.
	// A zero spirit GUID (the repop auto-queue arm) has no healer to cast.
	castGuides := make(map[uint64]bool)
	for _, item := range visuals {
		if sess := s.findSessionByGUID(item.playerGUID); sess != nil && sess.worldReady.Load() && sess.player != nil && sess.player.PlayerFlags&playerFlagGhost != 0 {
			if item.spiritGUID != 0 && !castGuides[item.spiritGUID] {
				castGuides[item.spiritGUID] = true
				s.broadcastSpiritHealCast(item.spiritGUID, sess)
			}
			// Battleground.cpp:312: the resurrection visual lands as a triggered
			// CastSpell, not an aura — use the instant visual-cast path.
			sess.castVisualSpell(24171) // SPELL_RESURRECTION_VISUAL
		}
	}
	for _, item := range due {
		sess := s.findSessionByGUID(item.playerGUID)
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.player.PlayerFlags&playerFlagGhost == 0 {
			continue
		}
		sess.resurrectPlayer(ctx, 1.0)
		sess.applyAura(6962)
		sess.applyAura(44535) // SPELL_SPIRIT_HEAL_MANA
		sess.removeAura(2584) // SPELL_WAITING_FOR_RESURRECT
		sess.spawnCorpseBones(ctx)
		sess.debug("spirit wave resurrected ghost", "account", sess.accountName, "guid", item.playerGUID, "spirit", item.spiritGUID)
	}
}

// broadcastSpiritHealCast sends a spirit healer's SPELL_SPIRIT_HEAL
// SMSG_SPELL_GO (Battleground.cpp:311, triggered self-cast: GO only, no
// SPELL_START) to the healer's visibility set: the healer's motion
// position when it has one, else the ghost's position — the ghost queued
// at the healer, so it stands where the healer stands.
func (s *Server) broadcastSpiritHealCast(spiritGUID uint64, sess *session) {
	if s == nil || sess == nil || sess.player == nil || spiritGUID == 0 {
		return
	}
	mapID, instanceID := sess.player.Map, sess.player.InstanceID
	x, y, z := sess.player.X, sess.player.Y, sess.player.Z
	var spiritPhase uint32
	spiritPhaseKnown := false
	if m := s.findCreatureMotion(mapID, instanceID, spiritGUID); m != nil {
		x, y, z = m.X, m.Y, m.Z
		spiritPhase, spiritPhaseKnown = m.PhaseMask, true
	}
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: spiritGUID}
	castFlags := spellCastFlagGo | spellCastFlagPending
	if s.Data != nil {
		if spellInfo, found, err := s.Data.Spell(22012); err == nil && found {
			target = finalizeSessionSrcTarget(spellInfo, spellGoPacketTarget(spellInfo, target), false, x, y, z)
			if spellInfo.StartRecoveryTime == 0 {
				castFlags |= protocol.SpellCastFlagNoGCD
			}
		}
	}
	// SendSpellGo (Spell.cpp:4292/4297/4325): a triggered cast with cast
	// count 0 carries CAST_FLAG_PENDING and CastID 0 (Spell.cpp:4331/602); POWER_LEFT_SELF (:4297) needs
	// a player or pet caster, and the healer is neither, so it stays
	// off; NO_GCD (:4325) has no caster gate, so it applies when the
	// resolved spell has no start recovery time. AMMO (:4295) stays off
	// — the creature virtual-item ammo model is unbridged (boss_ai.go
	// precedent) — and ADJUST_MISSILE (:4316) needs a packet traj speed
	// this hand-built target never carries.
	pkt := protocol.BuildSpellGo(spiritGUID, spiritGUID, 0, 22012, castFlags, uint32(time.Now().UnixMilli()), []uint64{spiritGUID}, nil, target)
	// SendMessageToSet on the healer (Battleground.cpp:311) carries the
	// MessageDistDeliverer phase leg; without a motion the healer's phase
	// is unknown, so the old phase-blind fan-out is kept there.
	if spiritPhaseKnown {
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_SPELL_GO), pkt, mapID, instanceID, x, y, z, spiritPhase)
	} else {
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELL_GO), pkt, mapID, instanceID, x, y, z, nil)
	}
}

// spiritWavePending records a wave's visual pass for one ghost: the spirit
// guide it queued at and when the visual landed, so the actual resurrect can
// fire >=500ms later like _ProcessResurrect's m_LastResurrectTime > 500 arm.
type spiritWavePending struct {
	spiritGUID uint64
	waveAt     time.Time
}

// spiritWaveDue carries a ghost whose 500ms visual delay has elapsed into the
// resurrect arm of updateSpiritHealerResurrectWaves.
type spiritWaveDue struct {
	playerGUID uint64
	spiritGUID uint64
}

type corpseObjectState struct {
	MapID, InstanceID                              uint32
	DisplayID, Bytes1, Bytes2, Flags, DynamicFlags uint32
	GuildID                                        uint32
	CorpseType                                     uint32
	PhaseMask                                      uint32
	GhostTime                                      int64
	X, Y, Z, Orientation                           float32
}

func (s *session) loadCorpseObject(ctx context.Context) (corpseObjectState, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return corpseObjectState{}, false
	}
	var mapID, instanceID, displayID, bytes1, bytes2, flags, dynamicFlags, corpseType, ghostTime, phaseMask int64
	var corpse corpseObjectState
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId, instanceId, posX, posY, posZ, orientation, displayId, bytes1, bytes2, flags, dynFlags, corpseType, time, phaseMask FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones).Scan(&mapID, &instanceID, &corpse.X, &corpse.Y, &corpse.Z, &corpse.Orientation, &displayID, &bytes1, &bytes2, &flags, &dynamicFlags, &corpseType, &ghostTime, &phaseMask); err != nil {
		return corpseObjectState{}, false
	}
	corpse.MapID = uint32(mapID)
	corpse.InstanceID = uint32(instanceID)
	corpse.DisplayID = uint32(displayID)
	corpse.Bytes1 = uint32(bytes1)
	corpse.Bytes2 = uint32(bytes2)
	corpse.Flags = uint32(flags)
	corpse.DynamicFlags = uint32(dynamicFlags)
	corpse.CorpseType = uint32(corpseType)
	corpse.PhaseMask = uint32(phaseMask)
	corpse.GhostTime = ghostTime
	var guildID int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guildId FROM corpse WHERE guid = ?", s.playerGUID).Scan(&guildID); err == nil {
		corpse.GuildID = uint32(guildID)
	}
	return corpse, true
}

// luaCorpseObject builds the Eluna Corpse object for the session player's
// resurrectable corpse (Player::GetCorpse, PlayerMethods.h:1581-1588;
// CorpseMethods.h). The row comes from loadCorpseObject, so bones are
// excluded and BG/arena deaths, which Go never persists (the
// Player.cpp:4867-4868 save-skip), never reach here. The corpse GUID is
// the map-assigned low GUID (Player.cpp:4822), not the owner GUID.
// The generic object fallback supplies GetGUID/GetMapId/GetObjectType(7)
// and position reads, while the five Corpse methods are explicit here.
// Only the ghost time is mutable from Lua (ResetGhostTime,
// Corpse.cpp:146-149), so SaveToDB persists just the time column: a full
// delete+insert (Corpse::SaveToDB :95-123) would also rewrite itemCache,
// which loadCorpseObject does not capture, while an UPDATE preserves it.
// A reclaimed corpse (row gone) saves nothing.
func (s *session) luaCorpseObject(state corpseObjectState) *scripting.Object {
	if s == nil {
		return nil
	}
	ownerGUID := s.playerGUID
	corpseGUID := uint64(0xF101)<<48 | uint64(s.server.corpseLowGUID(state.MapID, state.InstanceID, ownerGUID))
	ghostTime := state.GhostTime
	methods := map[string]scripting.ObjectMethod{}
	methods["GetOwnerGUID"] = luaNoArgs(func() any { return ownerGUID })
	methods["GetGhostTime"] = luaNoArgs(func() any { return uint32(ghostTime) })
	methods["GetType"] = luaNoArgs(func() any { return state.CorpseType })
	methods["ResetGhostTime"] = func(_ context.Context, _ []any) ([]any, error) {
		ghostTime = time.Now().Unix()
		return nil, nil
	}
	methods["SaveToDB"] = func(ctx context.Context, _ []any) ([]any, error) {
		if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return nil, nil
		}
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE corpse SET time = ? WHERE guid = ?", ghostTime, ownerGUID)
		return nil, nil
	}
	return &scripting.Object{Type: "Corpse", Fields: map[string]any{"GUID": corpseGUID, "OwnerGUID": ownerGUID, "Map": state.MapID, "MapId": state.MapID, "InstanceId": state.InstanceID, "X": state.X, "Y": state.Y, "Z": state.Z, "Orientation": state.Orientation, "DisplayID": state.DisplayID, "CorpseType": state.CorpseType, "InWorld": true}, Methods: methods}
}

func (s *session) sendLoadedCorpse(ctx context.Context) bool {
	if _, ok := s.loadCorpseObject(ctx); !ok {
		return false
	}
	// Player::CalculateCorpseReclaimDelay's load=true arm has no C++
	// callers (Player.cpp:24346-24385) — no reclaim-delay packet is ever
	// sent on ghost login — so only the water-walk goes out here.
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
	return true
}

func (s *session) prepareLoginResurrection(ctx context.Context, state *playerState) {
	if s == nil || state == nil || state.AtLogin&uint32(atLoginResurrect) == 0 {
		return
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ?", state.GUID)
		// The corpse object is gone with the row (no bones are created on
		// this path), so the low GUID must not be reused by the next
		// corpse for this owner. Standing bones entries stay: the row
		// delete does not remove bones (Player::OfflineResurrect has no
		// ConvertCorpseToBones leg), so only the mappings go.
		s.server.dropCorpseGUIDMappings(state.GUID)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ?, health = ?, playerFlags = ? WHERE guid = ?", ^uint32(atLoginResurrect), maxUint32(state.MaxHealth/2, 1), state.PlayerFlags&^playerFlagGhost, state.GUID)
	}
	s.castMu.Lock()
	for _, spellID := range []uint32{8326, 20584} {
		if aura, ok := s.activeAuras[spellID]; ok && aura != nil {
			aura.Stopped = true
			if aura.Timer != nil {
				aura.Timer.Stop()
			}
			if aura.TickTimer != nil {
				aura.TickTimer.Stop()
			}
			delete(s.activeAuras, spellID)
		}
		delete(s.auras, spellID)
		delete(s.auraSlots, spellID)
	}
	s.castMu.Unlock()
	// Player::LoadCorpse's AT_LOGIN_RESURRECT leg runs the full
	// ResurrectPlayer (Player.cpp:18310), which always recasts item
	// obtain-spell auras (Player.cpp:4732).
	s.castAllObtainSpells(ctx)
	state.PlayerFlags &^= playerFlagGhost
	state.Health = maxUint32(state.MaxHealth/2, 1)
	state.Powers[0] = state.MaxPowers[0] / 2
	state.Powers[1] = 0
	state.Powers[3] = state.MaxPowers[3] / 2
	state.AtLogin &^= uint32(atLoginResurrect)
	// m_deathExpireTime is never zeroed by ResurrectPlayer — the only
	// zeroing site is the Player constructor (Player.cpp:274); the
	// escalation window survives across resurrections and relogs.
}

func battlegroundMap(mapID uint32) bool {
	switch mapID {
	case 30, 489, 529, 566, 559, 562, 572, 607, 617, 618, 628:
		return true
	default:
		return false
	}
}

func (s *Server) shouldCreateCorpseBones(mapID uint32) bool {
	if s == nil {
		return false
	}
	if battlegroundMap(mapID) {
		return s.Config.DeathBonesBattleground
	}
	return s.Config.DeathBonesWorld
}

func (s *session) shouldCreateCorpseBones(mapID uint32) bool {
	if s == nil || s.server == nil {
		return false
	}
	return s.server.shouldCreateCorpseBones(mapID)
}

// spawnCorpseBones mirrors Player::SpawnCorpseBones and Map::ConvertCorpseToBones:
// remove the resurrectable corpse from persistence, then optionally create ownerless bones at its stored location.
func (s *session) spawnCorpseBones(ctx context.Context) {
	s.convertCorpseToBones(ctx, false, false)
}

func (s *session) spawnLoadedCorpseBones(ctx context.Context) {
	s.convertCorpseToBones(ctx, true, false)
}

// convertCorpseToBonesInsignia mirrors the ConvertCorpseToBones call
// inside Player::RemovedInsignia (Player.cpp:8505): the insignia leg
// creates bones regardless of the DEATH_BONES configs
// (Map.cpp:4650-4654, `insignia || ...`) and returns the bones GUID,
// or 0 when no corpse stood to convert (GetCorpseByPlayer miss,
// Map.cpp:4629-4631).
func (s *session) convertCorpseToBonesInsignia(ctx context.Context) uint64 {
	return s.convertCorpseToBones(ctx, true, true)
}

func (s *session) convertCorpseToBones(ctx context.Context, destroyVisible, insignia bool) uint64 {
	corpse, ok := s.loadCorpseObject(ctx)
	if !ok {
		// Battleground/arena corpses have no DB row (the
		// Player.cpp:4867-4868 save-skip), but C++ converts the
		// in-memory corpse object all the same (Map::ConvertCorpseToBones
		// works off GetCorpseByPlayer, Map.cpp:4629-4631); the session
		// snapshot taken at spawnCorpseObject is that object here.
		return s.convertSessionCorpseToBones(ctx, destroyVisible, insignia)
	}
	if destroyVisible {
		s.despawnCorpseObject(corpse.MapID, corpse.InstanceID, corpse.X, corpse.Y, corpse.Z)
	}
	if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones); err != nil {
		return 0
	}
	if !insignia && !s.shouldCreateCorpseBones(corpse.MapID) {
		s.server.dropCorpseGUIDMapping(corpse.MapID, corpse.InstanceID, s.playerGUID)
		return 0
	}
	return s.createCorpseBones(corpse.MapID, corpse.InstanceID, corpse.DisplayID, corpse.Bytes1, corpse.Bytes2, corpse.GuildID, corpse.DynamicFlags, corpse.PhaseMask, corpse.X, corpse.Y, corpse.Z, corpse.Orientation, insignia)
}

// convertSessionCorpseToBones is the no-row half of ConvertCorpseToBones:
// a battleground/arena corpse converts from the session snapshot, never
// from the DB. With no snapshot there is no corpse object to convert,
// matching the GetCorpseByPlayer miss.
func (s *session) convertSessionCorpseToBones(ctx context.Context, destroyVisible, insignia bool) uint64 {
	if s == nil || s.server == nil || s.player == nil || !s.player.hasCorpse {
		return 0
	}
	// Snapshot before the despawn below clears it: despawnCorpseObject
	// zeroes the session snapshot as the m_corpse clear does. The corpse
	// flags do not carry over (ConvertCorpseToBones overwrites them with
	// UNK2|BONES, Map.cpp:4659) and fresh corpses carry dynFlags 0, so
	// neither is snapshotted.
	mapID, instanceID := s.player.corpseMap, s.player.corpseInstanceID
	displayID := s.player.corpseDisplayID
	x, y, z, orientation := s.player.corpseX, s.player.corpseY, s.player.corpseZ, s.player.corpseOrientation
	if destroyVisible {
		s.despawnCorpseObject(mapID, instanceID, x, y, z)
	}
	if !insignia && !s.shouldCreateCorpseBones(mapID) {
		s.server.dropCorpseGUIDMapping(mapID, instanceID, s.playerGUID)
		return 0
	}
	bytes1, bytes2 := corpseAppearance(s.player)
	return s.createCorpseBones(mapID, instanceID, displayID, bytes1, bytes2, s.player.GuildID, 0, s.currentPlayerPhaseMask(), x, y, z, orientation, insignia)
}

// createCorpseBones broadcasts the bones create block for a converted
// corpse and registers the bones object (Map::ConvertCorpseToBones,
// Map.cpp:4653-4670): bones keep the corpse's low GUID, carry
// UNK2|BONES with the item slots cleared, and live in the registry until
// the 60-minute expiry so late arrivals see them. The conversion consumes
// the corpse object (Map::RemoveCorpse, Map.cpp:4610-4621), so the session
// snapshot goes with it — a later conversion must not re-bone the stale
// position, and the next corpse mints fresh via releaseCorpseGUIDForNewCorpse.
func (s *session) createCorpseBones(mapID, instanceID, displayID, bytes1, bytes2, guildID, dynamicFlags, phaseMask uint32, x, y, z, orientation float32, insignia bool) uint64 {
	if s == nil || s.server == nil {
		return 0
	}
	if s.player != nil {
		s.player.hasCorpse = false
		s.player.corpseMap, s.player.corpseInstanceID = 0, 0
		s.player.corpseDisplayID, s.player.corpseOrientation = 0, 0
	}
	// RemovedInsignia sets CORPSE_DYNFLAG_LOOTABLE on the converted
	// bones (Player.cpp:8510); the bit rides the create block itself,
	// not a later update, so it is folded in here.
	if insignia {
		dynamicFlags |= corpseDynFlagLootable
	}
	lowGUID := s.server.corpseLowGUID(mapID, instanceID, s.playerGUID)
	corpseGUID := uint64(0xF101)<<48 | uint64(lowGUID)
	fields := corpseObjectFields{DisplayID: displayID, Bytes1: bytes1, Bytes2: bytes2, GuildID: guildID, Flags: corpseFlagUnk2 | corpseFlagBones, DynamicFlags: dynamicFlags}
	block := buildCorpseCreateBlockWithFields(corpseGUID, fields, x, y, z, orientation)
	s.server.registerCorpseBones(mapID, instanceID, s.playerGUID, corpseBonesEntry{lowGUID: lowGUID, displayID: displayID, bytes1: bytes1, bytes2: bytes2, guildID: guildID, flags: corpseFlagUnk2 | corpseFlagBones, dynamicFlags: dynamicFlags, phaseMask: phaseMask, x: x, y: y, z: z, orientation: orientation, createdAt: time.Now()})
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		// Bones are AddToMap'd on the corpse's map (Map.cpp:4627-4670):
		// center the fan-out on the corpse, not the player.
		s.server.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), mapID, instanceID, x, y, z, s)
	}
	return corpseGUID
}

// corpseResurrectableExpiry is the Corpse::IsExpired horizon for
// resurrectable corpses (Corpse.cpp:211 — m_time < t - 3 * DAY); expired
// corpses are converted to bones by the periodic sweep.
const corpseResurrectableExpiry = 72 * time.Hour

// corpseExpirySweepInterval mirrors the WUPDATE_CORPSES timer
// (World.cpp:2106 — "Erase corpses once every 20 minutes").
const corpseExpirySweepInterval = 20 * time.Minute

// expireOldCorpses mirrors Map::RemoveOldCorpses (Map.cpp:4681-4703): every
// resurrectable corpse older than corpseResurrectableExpiry is converted to
// bones, and bones older than corpseBonesExpiry are despawned. An online
// owner's visible corpse is despawned and replaced with bones via
// spawnLoadedCorpseBones (the RemoveCorpse + ConvertCorpseToBones arm); an
// offline owner's corpse still becomes visible bones for nearby players
// (Map::RemoveOldCorpses, Map.cpp:4681-4703) before the row is deleted —
// bones are never persisted. Returns the number of expired corpses and
// bones handled.
// No-bridge: the grid-loaded leg (!IsRemovalGrid in ConvertCorpseToBones)
// has no analog — Go has no grid model and the packet broadcast is
// unconditional.
func (s *Server) expireOldCorpses(ctx context.Context) int {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0
	}
	cutoff := time.Now().Unix() - int64(corpseResurrectableExpiry/time.Second)
	// Corpse::IsExpired (Corpse.cpp:202-206) is also true when no
	// character-cache entry exists for the owner, and Map::RemoveOldCorpses
	// (Map.cpp:4678-4690) converts such corpses to bones at the next
	// sweep — the deleted-character arm is disjunctive with the 72h
	// cutoff (an AND would leave orphans visible up to 3 days).
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT guid, mapId, instanceId, posX, posY, posZ, orientation, displayId, bytes1, bytes2, guildId, flags, dynFlags, phaseMask FROM corpse WHERE corpseType <> ? AND (time < ? OR NOT EXISTS (SELECT 1 FROM characters WHERE guid = corpse.guid))", corpseTypeBones, cutoff)
	if err != nil {
		return 0
	}
	type expiredCorpse struct {
		guid                                                  uint64
		mapID, instanceID, displayID, bytes1, bytes2, guildID uint32
		flags, dynFlags, phaseMask                            uint32
		x, y, z, orientation                                  float32
	}
	var corpses []expiredCorpse
	for rows.Next() {
		var c expiredCorpse
		var mapID, instanceID, displayID, bytes1, bytes2, guildID, flags, dynFlags, phaseMask int64
		if err := rows.Scan(&c.guid, &mapID, &instanceID, &c.x, &c.y, &c.z, &c.orientation, &displayID, &bytes1, &bytes2, &guildID, &flags, &dynFlags, &phaseMask); err != nil {
			continue
		}
		c.mapID, c.instanceID = uint32(mapID), uint32(instanceID)
		c.displayID, c.bytes1, c.bytes2, c.guildID = uint32(displayID), uint32(bytes1), uint32(bytes2), uint32(guildID)
		c.flags, c.dynFlags, c.phaseMask = uint32(flags), uint32(dynFlags), uint32(phaseMask)
		corpses = append(corpses, c)
	}
	_ = rows.Close()
	handled := 0
	for _, c := range corpses {
		if sess := s.findSessionByGUID(c.guid); sess != nil {
			sess.spawnLoadedCorpseBones(ctx)
		} else {
			// Map::RemoveOldCorpses converts the corpse even when the owner
			// is offline (Map.cpp:4681-4703): broadcast the bones visual on
			// the corpse's own map, then delete the row (bones are never
			// persisted, Player.cpp:4867 / Map.cpp:4636). The bones keep
			// the corpse's low GUID and join the registry like the
			// online arm, so a player arriving later still sees them.
			lowGUID := s.corpseLowGUID(c.mapID, c.instanceID, c.guid)
			if s.shouldCreateCorpseBones(c.mapID) {
				corpseGUID := uint64(0xF101)<<48 | uint64(lowGUID)
				// Map::ConvertCorpseToBones (Map.cpp:4659) OVERWRITES the
				// bones flags with UNK2|BONES; the corpse's hide-helm,
				// hide-cloak and BG-lootable bits do not carry over.
				fields := corpseObjectFields{DisplayID: c.displayID, Bytes1: c.bytes1, Bytes2: c.bytes2, GuildID: c.guildID, Flags: corpseFlagUnk2 | corpseFlagBones, DynamicFlags: c.dynFlags}
				block := buildCorpseCreateBlockWithFields(corpseGUID, fields, c.x, c.y, c.z, c.orientation)
				s.registerCorpseBones(c.mapID, c.instanceID, c.guid, corpseBonesEntry{lowGUID: lowGUID, displayID: c.displayID, bytes1: c.bytes1, bytes2: c.bytes2, guildID: c.guildID, flags: corpseFlagUnk2 | corpseFlagBones, dynamicFlags: c.dynFlags, phaseMask: c.phaseMask, x: c.x, y: c.y, z: c.z, orientation: c.orientation, createdAt: time.Now()})
				updates := protocol.NewUpdateData()
				updates.AddUpdateBlock(block)
				if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
					s.broadcastToVisibleSet(packet.Opcode, packet.Payload.Bytes(), c.mapID, c.instanceID, c.x, c.y, c.z, nil)
				}
			} else {
				s.dropCorpseGUIDMapping(c.mapID, c.instanceID, c.guid)
			}
			if _, err := s.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ? AND corpseType <> ?", c.guid, corpseTypeBones); err != nil {
				continue
			}
		}
		handled++
	}
	// Map::RemoveOldCorpses (Map.cpp:4693-4701) sweeps expired bones in the
	// same pass as expired corpses.
	handled += s.expireCorpseBones(ctx)
	return handled
}

// updateCorpseExpiry runs the corpse sweep from the world tick, mirroring
// the WUPDATE_CORPSES arm in World::Update (World.cpp:2514-2522).
func (s *Server) updateCorpseExpiry(ctx context.Context) {
	if s == nil {
		return
	}
	now := time.Now()
	if !s.lastCorpseExpiry.IsZero() && now.Sub(s.lastCorpseExpiry) < corpseExpirySweepInterval {
		return
	}
	s.lastCorpseExpiry = now
	s.expireOldCorpses(ctx)
}

// deleteCorpseDataForMap mirrors Map::DeleteCorpseData as run when an
// instance map unloads during reset (Map.cpp:4182): corpse rows for the
// reset (map, instance) pair go with the map (CHAR_DEL_CORPSES_FROM_MAP).
// The CMSG_RESET_INSTANCES handlers never delete corpses directly — only
// the per-map unload does — so callers gate this on the reset actually
// emptying the map.
func (s *session) deleteCorpseDataForMap(ctx context.Context, mapID, instanceID uint32) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil {
		return
	}
	_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_DEL_CORPSES_FROM_MAP", mapID, instanceID)
	s.server.forgetCorpseGUIDsForMap(mapID, instanceID)
}

// resurrectPlayer mirrors Player::ResurrectPlayer (Player.cpp:4682) with the
// C++ default applySickness=false (Player.h:1698): only the spirit-healer
// path inflicts resurrection sickness (NPCHandler.cpp:221, bridged by the
// explicit true arm at handleSpiritHealerActivate). Battleground joins/exits,
// BG spirit waves, corpse reclaim, WG hearth resurrects, teleport revives,
// area-trigger revives and GM .revive all resurrect clean.
// ResurrectUsingRequestDataImpl (Player.cpp:24228) is the same false arm,
// bridged by resurrectPlayerNoSickness.
func (s *session) resurrectPlayer(ctx context.Context, restorePercent float32) {
	s.resurrectPlayerWithBones(ctx, restorePercent, false, false)
}

// resurrectPlayerNoSickness mirrors the ResurrectUsingRequestDataImpl arm
// (Player.cpp:24228: ResurrectPlayer(0.0f, false)): an accepted resurrect
// request never inflicts resurrection sickness.
func (s *session) resurrectPlayerNoSickness(ctx context.Context, restorePercent float32) {
	s.resurrectPlayerWithBones(ctx, restorePercent, false, false)
}

// resurrectPlayerWithBones mirrors Player::ResurrectPlayer (Player.cpp:4682);
// toBones converts the corpse to bones (Player::SpawnCorpseBones, the
// EffectSelfResurrect arm) instead of despawning it. applySickness is the
// ResurrectPlayer applySickness parameter: true runs the resurrection
// sickness leg (Player.cpp:4740-4753) after the ghost auras are removed,
// matching the C++ order.
func (s *session) resurrectPlayerWithBones(ctx context.Context, restorePercent float32, toBones, applySickness bool) {
	if s.player == nil {
		return
	}
	packet := protocol.NewBuffer(16)
	packet.WriteU32(0xFFFFFFFF)
	packet.WriteF32(0)
	packet.WriteF32(0)
	packet.WriteF32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_DEATH_RELEASE_LOC), packet.Bytes(), true)
	s.player.PlayerFlags &^= playerFlagGhost
	s.deathTimer = time.Time{}
	s.removeAura(8326)
	s.removeAura(20584)
	// Player::ResurrectPlayer clears PLAYER_FLAGS_IS_OUT_OF_BOUNDS on every
	// resurrect (Player.cpp:4688); repop is not the only path here — a
	// resurrect request lands at the corpse, which can sit under the map.
	s.player.PlayerFlags &^= playerFlagOutOfBounds
	// Player::ResurrectPlayer (Player.cpp:4694-4697): a recruiter or
	// recruited player carries UNIT_DYNFLAG_REFER_A_FRIEND.
	if s.isRecruiter || s.recruiterID != 0 {
		s.player.DynamicFlags |= unitDynFlagReferAFriend
	}
	// Player::ResurrectPlayer restores health/powers (Player.cpp:4701-4707)
	// before the visibility update and the Eluna hook — a Lua OnResurrect
	// handler observes the restored values, not zero.
	if restorePercent > 0 {
		s.player.Health = uint32(float32(s.player.MaxHealth) * restorePercent)
		s.player.Powers[0] = uint32(float32(s.player.MaxPowers[0]) * restorePercent) // mana
		s.player.Powers[1] = 0                                                       // rage
		s.player.Powers[3] = uint32(float32(s.player.MaxPowers[3]) * restorePercent) // energy
	}
	// Player::ResurrectPlayer (Player.cpp:4732): death strips the item
	// obtain-spell auras (RemoveAllAurasOnDeath), so every resurrect recasts
	// them — otherwise they stay missing until relog. Runs before the Eluna
	// hook, matching the C++ CastAllObtainSpells -> OnResurrect order.
	s.castAllObtainSpells(ctx)
	// Eluna PLAYER_EVENT_ON_RESURRECT (36): Eluna::OnResurrect
	// (PlayerHooks.cpp:94-99), (event, player), void (CallAllFunctions).
	// Player::ResurrectPlayer (Player.cpp:4735) fires it after the visibility
	// update and the obtain-spell recast, before resurrection sickness.
	s.triggerPlayerEvent(ctx, scripting.PlayerEventResurrect, s.luaPlayer())
	if applySickness {
		s.applyResurrectionSickness()
	}
	// Player::ResurrectPlayer (Player.cpp:4682-4753) does no corpse
	// handling at all — removal rides only an explicit SpawnCorpseBones
	// call by the caller (ResurrectUsingRequestDataImpl :24236, the
	// spirit-healer SendSpiritResurrect, the BG/arena/WG wave and
	// reclaim legs), so the no-bones path leaves the corpse object
	// alone: the hearth resurrect (HandleHearthAndResurrect,
	// MiscHandler.cpp:1505-1523) resurrects without boning, and the
	// corpse stays visible at the death location.
	if toBones {
		s.convertCorpseToBones(ctx, false, false)
	}
	s.persistResurrectionState(ctx)
	s.sendPlayerUpdate()
	if s.server != nil && (s.player.Map == ICMapID || s.hasActiveBGMap(ICMapID)) {
		s.server.applyICNodeAuras(s)
	}
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_LAND_WALK))
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT))
	s.refreshNearbyObjects(ctx)
}

func (s *session) persistResurrectionState(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ?, playerFlags = ? WHERE guid = ?", s.player.Health, s.player.PlayerFlags, s.playerGUID)
}

// resurrectionData mirrors Player::_resurrectionData (ResurrectionData,
// Player.h:871): the caster, the caster location for the teleport, the
// restored health and mana values carried by the resurrect spell effect,
// and the applied aura id (Raise Ally, Player.cpp:24222; 0 on the plain
// resurrect paths, SpellEffects.cpp:270/4296).
type resurrectionData struct {
	GUID    uint64
	MapID   uint32
	X, Y, Z float32
	Health  uint32
	Mana    uint32
	Aura    uint32
}

// setResurrectRequestData mirrors Player::SetResurrectRequestData. The
// reference asserts that no request is outstanding; the caller is expected to
// check first, so an overwrite here is logged and refused. appliedAura is the
// Raise Ally arm (spell_dk.cpp:2841); the plain EffectResurrect paths pass 0
// (SpellEffects.cpp:270, 4296).
func (s *session) setResurrectRequestData(casterGUID uint64, mapID uint32, x, y, z float32, health, mana, appliedAura uint32) {
	if s.resurrection != nil {
		s.debug("resurrect request overwritten", "account", s.accountName, "guid", s.playerGUID)
		return
	}
	s.resurrection = &resurrectionData{GUID: casterGUID, MapID: mapID, X: x, Y: y, Z: z, Health: health, Mana: mana, Aura: appliedAura}
}

// sendResurrectRequest mirrors Spell::SendResurrectRequest: raw caster GUID,
// length-prefixed caster name (empty for player casters, the client resolves
// those by GUID), the spirit healer resurrection sickness flag, and the flag
// overriding the corpse reclaim delay for spells that ignore the timer. The
// name is NUL-terminated (ByteBuffer.h:212-218), hence WriteCString.
func (s *session) sendResurrectRequest(casterGUID uint64, name string, spiritHealer, reclaimDelayApplies bool) {
	packet := protocol.NewBuffer(25 + len(name))
	packet.WriteU64(casterGUID)
	packet.WriteU32(uint32(len(name)) + 1)
	packet.WriteCString(name)
	packet.WriteU8(boolByte(spiritHealer))
	// 1 = the corpse reclaim delay applies (the spell lacks
	// SPELL_ATTR3_IGNORE_RESURRECTION_TIMER, Spell.cpp:4709).
	packet.WriteU8(boolByte(reclaimDelayApplies))
	_ = s.write(uint16(protocol.OpcodeSMSG_RESURRECT_REQUEST), packet.Bytes(), true)
}

// handleRaiseAllyInitial mirrors spell_dk_raise_ally_initial's HandleDummy
// (spell_dk.cpp:2828-2838): the DUMMY effect on a dead player hit target
// registers a resurrect request carrying the Raise Ally aura id (the effect
// value, 46619) with the caster's location, and sends the request — the
// accept path then applies the aura instead of plain resurrecting
// (ResurrectUsingRequestData, Player.cpp:24200). The script's cast gates are
// honored per target: a non-player or alive target registers nothing
// (SPELL_FAILED_TARGET_NOT_DEAD), as does an already-risen (ghouled) one
// (SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW); HandleDummy's own
// IsResurrectRequested guard skips targets with a pending request.
func (s *session) handleRaiseAllyInitial(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	aura := uint32(0)
	if eff.BasePoints >= 0 {
		aura = uint32(eff.BasePoints) // GetEffectValue: the Raise Ally spell id (unrolled base points)
	}
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 || targetGUID == s.playerGUID || uint16(targetGUID>>48) == 0xF110 {
			continue
		}
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess == nil || targetSess.player == nil {
			continue
		}
		if !targetSess.isDeadOrGhost() || targetSess.playerHasAuraSpell(spellRaiseAlly) || targetSess.resurrection != nil {
			continue
		}
		// SetResurrectRequestData stores the caster's position
		// (Player.cpp:22247, WorldRelocate(*caster)); health and mana are 0.
		targetSess.setResurrectRequestData(s.playerGUID, s.player.Map, s.player.X, s.player.Y, s.player.Z, 0, 0, aura)
		targetSess.sendResurrectRequest(s.playerGUID, "", false, spell.AttributesEx3&spellAttr3IgnoreResurrectionTimer == 0)
	}
}

func boolByte(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

// handleResurrectResponse mirrors WorldSession::HandleResurrectResponse:
// alive players ignore the packet, a zero response clears the pending request,
// and an accepted response must match the stored resurrecter before the stored
// health, mana, and location are applied.
func (s *session) handleResurrectResponse(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return true
	}
	reader := protocol.NewReader(payload)
	// ResurrectResponse::Read (MiscPackets.cpp:237-241) reads the resurrecter
	// with `_worldPacket >> Resurrecter` — ByteBuffer's operator>>(ObjectGuid&)
	// reads a raw 8-byte GUID, not a packed one (packed requires ReadAsPacked).
	// A packed read consumed the mask plus seven GUID bytes and left the eighth
	// GUID byte to be misread as Response, so accept packets never matched the
	// stored resurrecter and resurrection requests silently failed.
	resurrecter, err := reader.ReadU64()
	if err != nil {
		return true
	}
	response, err := reader.ReadU8()
	if err != nil {
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Reference IsAlive() is death-state based: ghosts and corpses are not alive.
	if s.player.PlayerFlags&playerFlagGhost == 0 && s.player.Health > 0 {
		return true
	}
	if response == 0 {
		s.resurrection = nil
		return true
	}
	if s.resurrection == nil || s.resurrection.GUID != resurrecter {
		return true
	}
	data := *s.resurrection
	// Player::ResurrectUsingRequestData (Player.cpp:24200-24214): requests
	// carrying an applied aura (DK Raise Ally, spell_dk.cpp:2841) cast the
	// aura on the dead player and return — the plain teleport/resurrect/
	// bones legs never run. RemoveGhoul has no Go analog (no raised-ghoul
	// pet model); the aura's risen-ally summon (spell_dk_raise_ally, 46619)
	// stays unbridged — applyAura holds the shell until the spell-script
	// summon model exists.
	if data.Aura != 0 {
		s.applyAura(data.Aura)
		s.sendRaiseAllyWhisper(ctx, data.GUID)
		s.resurrection = nil
		return true
	}
	// Player::ResurrectUsingRequestDataImpl (Player.cpp:24214-24226):
	// ResurrectPlayer(0.0f, false) emits no packet carrying intermediate
	// values — SetHealth/SetPower follow inside the same call — so the
	// stored values are applied BEFORE the helper's update, not after:
	// the helper otherwise sends one update with the ghost's 1 HP and
	// the ghost flag already cleared, then a second with the real values.
	s.player.Health = data.Health
	s.player.Powers[0] = data.Mana
	s.player.Powers[1] = 0 // rage
	if s.player.MaxPowers[3] > 0 {
		s.player.Powers[3] = s.player.MaxPowers[3] // full energy
	}
	// Reference teleports to the caster location before resurrecting so the
	// player does not revive into nearby creatures at the corpse; the delayed
	// teleport retry path has no Go equivalent because teleportTo is sync.
	if data.MapID != s.player.Map || data.X != s.player.X || data.Y != s.player.Y || data.Z != s.player.Z {
		s.teleportTo(data.MapID, data.X, data.Y, data.Z, s.player.Orientation)
	}
	s.resurrectPlayerNoSickness(ctx, 0)
	s.persistResurrectionState(ctx)
	s.resurrection = nil
	s.spawnCorpseBones(ctx)
	s.sendPlayerUpdate()
	return true
}

// sendRaiseAllyWhisper mirrors spell_dk_raise_ally's SendText
// (spell_dk.cpp:2915-2919): the original caster whispers the rise text
// (broadcast_text 33055) to the risen player as a raid-boss whisper
// (Unit::Whisper, Unit.cpp:14103 — CHAT_MSG_RAID_BOSS_WHISPER,
// LANG_UNIVERSAL, the sender's gendered text, sent only to the target).
// The script's ghoul summon, possess charm, scaling auras, and aura AI have
// no Go model (no temp-summon system), so only the whisper rides the
// applyAura shell; a missing broadcast_text row stays silent like the C++
// GetBroadcastText miss.
func (s *session) sendRaiseAllyWhisper(ctx context.Context, originalCasterGUID uint64) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	casterSess := s.server.findSessionByGUID(originalCasterGUID)
	if casterSess == nil || casterSess.player == nil {
		return
	}
	var text, text1 string
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Text, Text1 FROM broadcast_text WHERE ID = ?", broadcastTextRiseAlly).Scan(&text, &text1); err != nil || text == "" {
		return
	}
	if casterSess.player.Gender == 1 && text1 != "" {
		text = text1
	}
	packet := protocol.BuildChatMessageWithOptions(chatRaidBossWhisper, languageUniversal, originalCasterGUID, s.playerGUID, text, "", true, casterSess.player.Name, casterSess.chatTag())
	_ = s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), packet, true)
}

// corpseRecord is one row of the characters.corpse table as written by
// buildPlayerRepop.
type corpseRecord struct {
	MapID       uint32
	X, Y, Z     float32
	Orientation float32
	CorpseType  uint32
	GhostTime   int64
	InstanceID  uint32
}

// loadCorpse mirrors Player::GetCorpse: the resurrectable corpse of this
// player (corpseType PvE or PvP); bones (type 0) are not returned.
func (s *session) loadCorpse(ctx context.Context) (corpseRecord, bool) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return corpseRecord{}, false
	}
	row := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId, posX, posY, posZ, orientation, corpseType, time, instanceId FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones)
	var corpse corpseRecord
	if err := row.Scan(&corpse.MapID, &corpse.X, &corpse.Y, &corpse.Z, &corpse.Orientation, &corpse.CorpseType, &corpse.GhostTime, &corpse.InstanceID); err != nil {
		return corpseRecord{}, false
	}
	return corpse, true
}

// corpseTarget is one in-world corpse as Spell::AddCorpseTarget sees it:
// the characters.corpse row keyed by its owner, scoped to the caster's
// map+instance the way ObjectAccessor::GetCorpse is (Corpse::GetMap / the
// map corpse grid). The corpse GUID is the map-assigned low GUID
// (Player.cpp:4822), never the owner GUID.
type corpseTarget struct {
	GUID        uint64
	OwnerGUID   uint64
	MapID       uint32
	InstanceID  uint32
	X, Y, Z     float32
	Orientation float32
	Type        uint32
}

func isCorpseGUID(guid uint64) bool { return uint16(guid>>48) == 0xF101 }

// listCorpsesOnMap lists the non-bones corpses on a map+instance: the
// characters.corpse rows Spell::SearchAreaTargets visits through the
// corpse grid (Spell.cpp:1881-1888, GRID_MAP_TYPE_MASK_CORPSE). The row
// guid is the owner's player GUID; the corpse GUID is the map-assigned
// low GUID from the registry. Bones (type 0) never pass
// SpellInfo::CheckTarget (SpellInfo.cpp:1700) and are excluded here, like
// loadCorpse does.
func (s *Server) listCorpsesOnMap(ctx context.Context, mapID, instanceID uint32) []corpseTarget {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return nil
	}
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT guid, posX, posY, posZ, corpseType FROM corpse WHERE mapId = ? AND instanceId = ? AND corpseType <> ?", mapID, instanceID, corpseTypeBones)
	if err != nil {
		return nil
	}
	defer rows.Close()
	corpses := make([]corpseTarget, 0)
	for rows.Next() {
		var ownerLow uint32
		var corpse corpseTarget
		if err := rows.Scan(&ownerLow, &corpse.X, &corpse.Y, &corpse.Z, &corpse.Type); err != nil {
			continue
		}
		corpse.GUID = uint64(0xF101)<<48 | uint64(s.corpseLowGUID(mapID, instanceID, uint64(ownerLow)))
		corpse.OwnerGUID = uint64(ownerLow)
		corpse.MapID = mapID
		corpse.InstanceID = instanceID
		corpses = append(corpses, corpse)
	}
	return corpses
}

// lookupCorpse mirrors ObjectAccessor::GetCorpse: the corpse row for a corpse
// GUID on the caster's map+instance, or false when the row is missing or
// lives on another map/instance (a stale or foreign corpse GUID resolves to
// no object, like C++'s null GetCorpseTarget). The wire GUID's low 32 bits
// are the map-assigned sequence value, so the owner comes from the
// registry first; a GUID that reaches here before any render pass assigned
// it falls back to the map-load mint (ensureCorpseGUIDsForMap), mirroring
// LoadCorpseData's per-row GenerateLowGuid.
func (s *Server) lookupCorpse(ctx context.Context, corpseGUID uint64, mapID, instanceID uint32) (corpseTarget, bool) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || !isCorpseGUID(corpseGUID) {
		return corpseTarget{}, false
	}
	ownerLow, ok := s.corpseOwnerLow(mapID, instanceID, uint32(corpseGUID&0xFFFFFFFF))
	if !ok {
		s.ensureCorpseGUIDsForMap(ctx, mapID, instanceID)
		ownerLow, ok = s.corpseOwnerLow(mapID, instanceID, uint32(corpseGUID&0xFFFFFFFF))
		if !ok {
			return corpseTarget{}, false
		}
	}
	var corpse corpseTarget
	row := s.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid, mapId, instanceId, posX, posY, posZ, orientation, corpseType FROM corpse WHERE guid = ?", ownerLow)
	var guid uint32
	if err := row.Scan(&guid, &corpse.MapID, &corpse.InstanceID, &corpse.X, &corpse.Y, &corpse.Z, &corpse.Orientation, &corpse.Type); err != nil {
		return corpseTarget{}, false
	}
	if corpse.MapID != mapID || corpse.InstanceID != instanceID {
		return corpseTarget{}, false
	}
	corpse.GUID = corpseGUID
	corpse.OwnerGUID = uint64(guid)
	return corpse, true
}

// serverCorpseHitTargetOwnerGUID is the Server-level form of
// corpseHitTargetOwnerGUID below, for creature casters: a corpse hit
// target resolves to its owner's player GUID when the corpse exists on
// the given map+instance and the owner is online
// (ObjectAccessor::FindPlayer twin); a non-corpse GUID passes through
// unchanged.
func (s *Server) serverCorpseHitTargetOwnerGUID(ctx context.Context, targetGUID uint64, mapID, instanceID uint32) (uint64, bool) {
	if !isCorpseGUID(targetGUID) {
		return targetGUID, true
	}
	if s == nil {
		return 0, false
	}
	corpse, ok := s.lookupCorpse(ctx, targetGUID, mapID, instanceID)
	if !ok {
		return 0, false
	}
	ts := s.findSessionByGUID(corpse.OwnerGUID)
	if ts == nil || ts.player == nil {
		return 0, false
	}
	return corpse.OwnerGUID, true
}

// corpseHitTargetOwnerGUID mirrors the m_corpseTarget arm of
// Spell::EffectResurrect / EffectResurrectNew (SpellEffects.cpp:256-257,
// 4280-4281): a corpse hit target resolves to its owner's player GUID when
// the corpse exists on the caster's map+instance and the owner is online
// (ObjectAccessor::FindPlayer twin); a non-corpse GUID passes through
// unchanged.
func (s *session) corpseHitTargetOwnerGUID(ctx context.Context, targetGUID uint64) (uint64, bool) {
	if !isCorpseGUID(targetGUID) {
		return targetGUID, true
	}
	if s == nil || s.server == nil || s.player == nil {
		return 0, false
	}
	return s.server.serverCorpseHitTargetOwnerGUID(ctx, targetGUID, s.player.Map, s.player.InstanceID)
}

// handleReclaimCorpse mirrors WorldSession::HandleReclaimCorpse: a ghost in
// range of its own resurrectable corpse after the reclaim delay elapses is
// resurrected (half health/mana, full in battlegrounds) and the corpse is
// turned into bones. Arenas block the reclaim via battlegroundTypeForMap.
func (s *session) handleReclaimCorpse(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	// MiscPackets.cpp:229: _worldPacket >> CorpseGUID — raw 8-byte read.
	if _, err := reader.ReadU64(); err != nil {
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Reference: IsAlive() or not-yet-released. The ghost flag distinguishes
	// the dead body (no flag) from the released ghost, matching the reference
	// death-state machine; health alone cannot (ghosts carry one health point).
	if s.player.PlayerFlags&playerFlagGhost == 0 {
		return true
	}
	// MiscHandler.cpp:581-583: no corpse reclaim in arenas. Player::InArena()
	// (Player.cpp:23647) == the active Battleground template isArena();
	// battlegroundTypeForMap derives the same template property from the BG
	// map (arena maps 559/562/572/617/618), and bgData.InstanceID != 0 marks
	// a session inside a BG instance.
	if s.bgData.InstanceID != 0 {
		if _, _, isArena, ok := battlegroundTypeForMap(s.player.Map); ok && isArena {
			return true
		}
	}
	corpse, ok := s.loadCorpse(ctx)
	if !ok {
		return true
	}
	// prevent resurrect before the reclaim delay after body release finished
	if corpse.GhostTime+int64(s.corpseReclaimDelaySeconds(corpse.CorpseType != corpseTypePvE)) > time.Now().Unix() {
		return true
	}
	if corpse.MapID != s.player.Map || corpse.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, corpse.X, corpse.Y, corpse.Z) > corpseReclaimRadius {
		return true
	}
	// WorldSession::HandleReclaimCorpse (MiscHandler.cpp:576-603): battleground
	// corpse reclaims resurrect at full health/mana (ResurrectPlayer(1.0f)),
	// everywhere else at half. InBattleground() is the session BG instance id
	// (Player.h:1906); the arena block is handled above via
	// battlegroundTypeForMap.
	restore := float32(0.5)
	if s.bgData.InstanceID != 0 {
		restore = 1.0
	}
	s.resurrectPlayer(ctx, restore)
	s.spawnCorpseBones(ctx)
	return true
}

const (
	spellEffectResurrectNew = 113        // SPELL_EFFECT_RESURRECT_NEW (SharedDefines.h:924)
	npcFlagGossip           = 0x00000001 // UNIT_NPC_FLAG_GOSSIP (UnitDefines.h)
	npcFlagQuestgiver       = 0x00000002 // UNIT_NPC_FLAG_QUESTGIVER (UnitDefines.h:187)
	npcFlagSpiritHealer     = 0x00004000
	npcFlagSpiritGuide      = 0x00008000
	npcFlagSpiritService    = npcFlagSpiritHealer | npcFlagSpiritGuide
)

// handleSelfRes mirrors WorldSession::HandleSelfResOpcode (SpellHandler.cpp:602):
// an empty opcode that casts the spell stored in PLAYER_SELF_RES_SPELL and
// clears the field. The stored spell's resurrect effect registers a resurrect
// request from the player to the player, which the client answers through
// CMSG_RESURRECT_RESPONSE, exactly like the reference EffectResurrectNew chain.
func (s *session) handleSelfRes(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// WorldSession::HandleSelfResOpcode, SpellHandler.cpp:605-606 — silent
	// return under SPELL_AURA_PREVENT_RESURRECTION.
	if s.hasAuraType(spellAuraPreventResurrection) {
		return true
	}
	spellID := s.player.SelfResSpell
	if spellID == 0 {
		return true
	}
	// SpellHandler.cpp:607-611 (HandleSelfResOpcode): the cast goes first
	// and PLAYER_SELF_RES_SPELL is cleared unconditionally after it — even
	// when the spell id resolves to nothing (C++ CastSpell no-ops on an
	// unknown id and the clear still runs).
	if s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			s.finishSpellCast(ctx, 0, spellID, spell, protocol.SpellTargetData{}, 0, 0, 0, nil)
		} else {
			s.debug("self resurrect spell lookup failed", "account", s.accountName, "spell", spellID, "found", found, "error", err)
		}
	}
	s.player.SelfResSpell = 0
	// SpellHandler.cpp:602-614 sends no packet here: cast, clear the field,
	// done. The field clear rides the next periodic update like C++'s
	// SetUInt32Value.
	return true
}

// getResurrectionSpellId mirrors Player::GetResurrectionSpellId (Player.cpp:24021-24060):
// the priceless-resurrection scan that fills PLAYER_SELF_RES_SPELL on death,
// before the death aura strip removes the buffs it reads. Soulstone dummy auras
// (SpellVisual[0]==99, SpellIconID==92) map to their rank's resurrect spell at
// prio 3; Twisting Nether 23701 applies at prio 2 on its 10% roll (its else-if
// can overwrite an earlier soulstone, matching the C++ per-effect iteration);
// Reincarnation (20608 learned, 21169 off cooldown, glyph 58059 or an Ankh
// 17030 in inventory) applies at prio 1.
func (s *session) getResurrectionSpellId(ctx context.Context) uint32 {
	if s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var prio uint32
	var spellID uint32
	// C++ iterates the dummy AuraEffect list in apply order; Go keeps auras in
	// a spell-keyed map, so dummy auras are visited in ascending spell order.
	// The soulstone+23701 coexistence corner then resolves by spell id rather
	// than apply order (documented deviation).
	s.castMu.Lock()
	ids := make([]uint32, 0, len(s.activeAuras))
	for id, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == spellAuraDummy {
			ids = append(ids, id)
		}
	}
	s.castMu.Unlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		spell, found, err := s.server.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if prio < 2 && spell.SpellVisual[0] == 99 && spell.SpellIconID == 92 {
			switch id {
			case 20707:
				spellID = 3026
			case 20762:
				spellID = 20758
			case 20763:
				spellID = 20759
			case 20764:
				spellID = 20760
			case 20765:
				spellID = 20761
			case 27239:
				spellID = 27240
			case 47883:
				spellID = 47882
			default:
				// TC_LOG_ERROR("entities.player", "Unhandled spell %u: S.Resurrection", id):
				// a visual/icon-matching dummy aura with no rank mapping leaves
				// prio and spellID untouched (the C++ continue skips prio = 3).
				s.debug("unhandled soulstone aura in resurrection scan", "account", s.accountName, "spell", id)
				continue
			}
			prio = 3
		} else if id == 23701 && rand.Intn(100) < 10 {
			// roll_chance_i(10): Random.h:59-62, chance > irand(0, 99).
			prio = 2
			spellID = 23700
		}
	}
	// Reincarnation passive (Glyph of Renewed Life 58059 or an Ankh 17030).
	if prio < 1 && s.hasLearnedSpell(20608) && !s.spellHasCooldown(21169) && (s.hasAura(58059) || s.inventoryItemCount(ctx, 17030) > 0) {
		spellID = 21169
	}
	return spellID
}

// spellHasCooldown mirrors SpellHistory::HasCooldown (SpellHistory.cpp:473-487)
// with the default ignoreCategoryCooldown=false: the per-spell entry, then
// the category arm over the single _categoryCooldowns holder
// (playerCategoryCooldownBlocked: EraseCooldown, SpellHistory.h:140-144,
// erases the category entry when the holder spell's entry is erased).
func (s *session) spellHasCooldown(spellID uint32) bool {
	if s.player == nil {
		return false
	}
	nowMs := time.Now().UnixMilli()
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID && cd.End > nowMs {
			return true
		}
	}
	category := uint32(0)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			category = spell.Category
		}
	}
	if category == 0 {
		return false
	}
	return s.playerCategoryCooldownBlocked(category, nowMs)
}

// inventoryItemCount mirrors Player::GetItemCount (Player.cpp): the total
// stack count of an item entry across the character's inventory.
func (s *session) inventoryItemCount(ctx context.Context, entry uint32) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var count uint32
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, entry).Scan(&count); err != nil {
		return 0
	}
	return count
}

// applySelfResurrectEffect mirrors Spell::EffectResurrectNew (SpellEffects.cpp:246)
// for the self-cast case: a dead player gets a resurrect request from itself
// carrying the effect damage as health and MiscValue as mana.
func (s *session) applySelfResurrectEffect(spell wotlk.Spell) {
	if s.player == nil || !s.isDeadOrGhost() {
		return
	}
	if s.resurrection != nil {
		return // already have one active request
	}
	for _, effect := range spell.Effects {
		if effect.Effect != spellEffectResurrectNew {
			continue
		}
		health := uint32(1)
		if effect.BasePoints >= 0 {
			health = uint32(effect.CalcValueForLevel(spell, uint32(s.player.Level))) // damage as computed by CalcValue
		}
		mana := uint32(0)
		if effect.MiscValue > 0 {
			mana = uint32(effect.MiscValue)
		}
		// EffectResurrectNew (SpellEffects.cpp:246-275) logs the effect,
		// stores the request at the caster's location, and sends the request
		// whose reclaim-delay byte is !HasAttribute(IGNORE_RESURRECTION_TIMER).
		s.setResurrectRequestData(s.playerGUID, s.player.Map, s.player.X, s.player.Y, s.player.Z, health, mana, 0)
		s.sendResurrectRequest(s.playerGUID, "", false, spell.AttributesEx3&spellAttr3IgnoreResurrectionTimer == 0)
		s.sendResurrectLog(spell.ID, s.playerGUID, s.playerGUID)
		return
	}
}

// applyResurrectNewToTargets mirrors Spell::EffectResurrectNew
// (SpellEffects.cpp:246-275): each dead player hit target without an active
// resurrect request gets a request from the caster carrying the effect damage
// as health and MiscValue as mana. A corpse hit target resolves to its owner
// (the m_corpseTarget arm, SpellEffects.cpp:256-257); the self-cast arm lives
// in applySelfResurrectEffect.
func (s *session) applyResurrectNewToTargets(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect, hitTargets []uint64) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	health := uint32(1)
	if eff.BasePoints >= 0 {
		health = uint32(eff.CalcValueForLevel(spell, uint32(s.player.Level))) // damage as computed by CalcValue
	}
	mana := uint32(0)
	if eff.MiscValue > 0 {
		mana = uint32(eff.MiscValue)
	}
	reclaimDelayApplies := spell.AttributesEx3&spellAttr3IgnoreResurrectionTimer == 0
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 || targetGUID == s.playerGUID || uint16(targetGUID>>48) == 0xF110 {
			continue
		}
		if ownerGUID, ok := s.corpseHitTargetOwnerGUID(ctx, targetGUID); ok {
			targetGUID = ownerGUID
		} else if isCorpseGUID(targetGUID) {
			continue
		}
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess == nil || targetSess.player == nil {
			continue
		}
		// Same ghost correction as handleEffectResurrect: C++ gates on
		// !IsAlive, and Go ghosts carry Health=1, so the old Health>0 gate
		// skipped ghost targets.
		if !targetSess.isDeadOrGhost() || targetSess.resurrection != nil {
			continue
		}
		// EffectResurrectNew (SpellEffects.cpp:246-275) logs the effect,
		// stores the request at the caster's location, and sends the request
		// whose reclaim-delay byte is !HasAttribute(IGNORE_RESURRECTION_TIMER).
		targetSess.setResurrectRequestData(s.playerGUID, s.player.Map, s.player.X, s.player.Y, s.player.Z, health, mana, 0)
		targetSess.sendResurrectRequest(s.playerGUID, "", false, reclaimDelayApplies)
		s.sendResurrectLog(spell.ID, s.playerGUID, targetGUID)
	}
}

// removedInsignia mirrors Player::RemovedInsignia
// (Player.cpp:8486-8520): an enemy removing the insignia of a dead
// battleground / world-PvP victim forces the victim's repop when the
// spirit is unreleased, turns the corpse into lootable bones, and
// opens the insignia loot window for the looter. Reached from
// Spell::EffectSkinPlayerCorpse (SpellEffects.cpp:5126-5146).
func (s *session) removedInsignia(ctx context.Context, looter *session) {
	if s == nil || s.player == nil || looter == nil || looter.player == nil {
		return
	}
	// The corpse stands at the death location; the forced repop below
	// teleports the victim to the graveyard, so the corpse's map is
	// captured before it moves.
	corpseMap, corpseInstance := s.player.Map, s.player.InstanceID
	// Player.cpp:8489: only in a battleground or a world-PvP zone
	// (WorldObject::IsInWorldPvpZone, Object.cpp:1122-1130, is the
	// Wintergrasp zone alone).
	if s.bgData.InstanceID == 0 && s.player.Zone != WGZoneID {
		return
	}
	// Player.cpp:8493-8498: if not released spirit, do it — m_deathTimer
	// > 0 is the unreleased countdown (KillPlayer sets it,
	// Player.cpp:4786; repop clears it, Player.cpp:5135).
	if !s.deathTimer.IsZero() {
		s.deathTimer = time.Time{}
		s.buildPlayerRepop(ctx, false)
		s.repopAtGraveyard(ctx)
	}
	// Player.cpp:8500-8502: _corpseLocation.WorldRelocate() relocates
	// the stored corpse location to the player's position — the victim
	// has not moved since death, so the corpse already stands there;
	// no Go analog is needed.
	// Player.cpp:8505-8509: convert to bones (the insignia leg creates
	// them regardless of the DEATH_BONES configs, Map.cpp:4650-4654)
	// and flag them lootable; a missing corpse ends the call
	// (Map.cpp:4629-4631).
	bonesGUID := s.convertCorpseToBonesInsignia(ctx)
	if bonesGUID == 0 {
		return
	}
	// Player.cpp:8513-8515: the victim's level is stashed in the bones'
	// loot gold field and the looter is recorded as lootRecipient.
	if s.server.setBonesInsigniaLoot(corpseMap, corpseInstance, s.playerGUID, uint32(s.player.Level), looter.playerGUID) == 0 {
		return
	}
	// Player.cpp:8516: the looter's loot window opens on the bones.
	looter.sendBonesLoot(ctx, bonesGUID, corpseMap, corpseInstance)
}

// handleEffectSkinPlayerCorpse mirrors Spell::EffectSkinPlayerCorpse
// (SpellEffects.cpp:5126-5146): at SPELL_EFFECT_HANDLE_HIT_TARGET per
// unit target, or the m_corpseTarget, a dead player target's insignia
// is removed (RemovedInsignia). Corpse GUIDs resolve to their owner
// the same way the resurrect arms do (corpseHitTargetOwnerGUID).
func (s *session) handleEffectSkinPlayerCorpse(ctx context.Context, hitTargets []uint64) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	for _, targetGUID := range hitTargets {
		if targetGUID == 0 || targetGUID == s.playerGUID || uint16(targetGUID>>48) == 0xF110 {
			continue
		}
		if ownerGUID, ok := s.corpseHitTargetOwnerGUID(ctx, targetGUID); ok {
			targetGUID = ownerGUID
		} else if isCorpseGUID(targetGUID) {
			continue
		}
		targetSess := s.server.findSessionByGUID(targetGUID)
		if targetSess == nil || targetSess.player == nil {
			continue
		}
		// SpellEffects.cpp:5143: the target must be dead.
		if !targetSess.isDeadOrGhost() {
			continue
		}
		targetSess.removedInsignia(ctx, s)
	}
}

// handleEffectSelfResurrect mirrors Spell::EffectSelfResurrect (SpellEffects.cpp:4429-4462),
// the SPELL_EFFECT_SELF_RESURRECT (94) HANDLE_HIT caster arm: a dead in-world
// player caster resurrects at once with flat (damage<0) or percent health and
// mana, rage zeroed, energy restored to full, and the corpse converted to
// bones (Player::SpawnCorpseBones) rather than despawned.
func (s *session) handleEffectSelfResurrect(ctx context.Context, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s == nil || s.player == nil || !s.isDeadOrGhost() {
		return
	}
	damage := eff.CalcValueForLevel(spell, uint32(s.player.Level))
	var health, mana uint32
	if damage < 0 {
		health = uint32(-damage)
		mana = uint32(eff.MiscValue)
	} else {
		health = uint32(float32(s.player.MaxHealth) * float32(damage) / 100)
		if s.player.MaxPowers[0] > 0 {
			mana = uint32(float32(s.player.MaxPowers[0]) * float32(damage) / 100)
		}
	}
	// SpellEffects.cpp:4429-4462 (EffectSelfResurrect): ResurrectPlayer(0.0f)
	// emits no packet carrying intermediate values — SetHealth/SetPower
	// follow inside the same call — so the computed values are applied
	// BEFORE the helper's update, not after: the helper otherwise sends
	// one update with the ghost's 1 HP and the ghost flag already cleared.
	s.player.Health = health
	s.player.Powers[0] = mana
	s.player.Powers[1] = 0
	s.player.Powers[3] = s.player.MaxPowers[3]
	s.resurrectPlayerWithBones(ctx, 0, true, false)
	s.persistResurrectionState(ctx)
	s.sendPlayerUpdate()
}

// creatureIsSpiritService resolves the npcflag of a spawned creature and
// tests it against the caller's mask: Unit::IsSpiritService
// (UNIT_NPC_FLAG_SPIRITHEALER | SPIRITGUIDE) for the area-spirit-healer
// handlers, UNIT_NPC_FLAG_SPIRITHEALER alone for the activate handler
// (NPCHandler.cpp:198 passes only SPIRITHEALER).
func (s *session) creatureIsSpiritService(ctx context.Context, guid uint64, mask uint32) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || guid == 0 {
		return false
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	var npcFlag uint32
	var err error
	if entry != 0 {
		err = s.server.WorldStore.DB.QueryRowContext(ctx,
			"SELECT COALESCE(NULLIF(c.npcflag, 0), t.npcflag, 0) FROM creature_template t LEFT JOIN creature c ON c.guid = ? WHERE t.entry = ?", low, entry).Scan(&npcFlag)
	} else {
		err = s.server.WorldStore.DB.QueryRowContext(ctx,
			"SELECT COALESCE(NULLIF(c.npcflag, 0), t.npcflag, 0) FROM creature c JOIN creature_template t ON t.entry = c.id WHERE c.guid = ? OR c.guid = ?", low, guid).Scan(&npcFlag)
	}
	if err != nil {
		return false
	}
	return npcFlag&mask != 0
}

// sendAreaSpiritHealerTime mirrors BattlegroundMgr::SendAreaSpiritHealerQueryOpcode:
// send the time remaining until next spirit healer resurrection pulse.
func (s *session) sendAreaSpiritHealerTime(guid uint64, timeLeft uint32) {
	packet := protocol.NewBuffer(12)
	packet.WriteU64(guid)
	packet.WriteU32(timeLeft)
	_ = s.write(uint16(protocol.OpcodeSMSG_AREA_SPIRIT_HEALER_TIME), packet.Bytes(), true)
}

// spiritWaveTimeLeftMs mirrors the 30000 - GetLastResurrectTime() computation in
// BattlegroundMgr::SendAreaSpiritHealerQueryOpcode (BattlegroundMgr.cpp:720):
// milliseconds until the next 30-second resurrection wave.
func (s *Server) spiritWaveTimeLeftMs() uint32 {
	s.spiritWaveMu.Lock()
	defer s.spiritWaveMu.Unlock()
	if elapsed := time.Since(s.lastSpiritWave); elapsed < 30*time.Second {
		return uint32((30*time.Second - elapsed).Milliseconds())
	}
	return 30000
}

// inBattlegroundWaveMap reports whether the player's map runs spirit-healer
// resurrection waves — the Go analog of HandleAreaSpiritHealer*Opcode's
// `_player->GetBattleground()` non-null gate (MiscHandler.cpp:1459-1502). Go has
// no live non-arena BG instance model, so the BG map set stands in:
// 30 AV, 489 WSG, 529 AB, 566 EOTS, 607 SOTA, 628 IOC.
func (s *session) inBattlegroundWaveMap() bool {
	if s == nil || s.player == nil {
		return false
	}
	switch s.player.Map {
	case 30, 489, 529, 566, 607, 628:
		return true
	}
	return false
}

// handleAreaSpiritHealerQuery mirrors WorldSession::HandleAreaSpiritHealerQueryOpcode
// (MiscHandler.cpp:1459): the creature must exist on the player's map and offer
// spirit service (== GetMap()->GetCreature + IsSpiritService; C++ performs no
// interact/distance check here). The existence gate is luaCreature — spirit
// healers never enter the motion system. The time packet is answered only in
// a battleground context — C++ sends it only via the live Battleground /
// Battlefield, otherwise silence.
func (s *session) handleAreaSpiritHealerQuery(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		// Truncated packet: WorldSession::Update catches ByteBufferException and
		// skips the packet (WorldSession.cpp:425-432) — drop, conn open.
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// MiscHandler.cpp:1466-1472: GetMap()->GetCreature(guid) must resolve —
	// a fabricated spirit-healer guid is silently dropped. Spirit healers
	// are excluded from the motion system (creaturemotion.go spawn filter),
	// so luaCreature — the static-spawn resolver on the player's map, same
	// one the gossip path uses — is the existence gate here.
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.luaCreature(ctx, guid) == nil {
		return true
	}
	if !s.creatureIsSpiritService(ctx, guid, npcFlagSpiritService) {
		return true
	}
	// MiscHandler.cpp:1468-1477: the query routes through the live
	// Battleground AND, independently, through the Battlefield owning the
	// player's zone (Battlefield::SendAreaSpiritHealerQueryOpcode,
	// Battlefield.cpp:635-641). The BG leg needs a live instance
	// (GetBattleground() non-null, Player.h:1906) — a BG map alone is
	// silent. The Wintergrasp leg is zone-based, not instance-based.
	if s.inBattlegroundWaveMap() && s.bgData.InstanceID != 0 {
		s.sendAreaSpiritHealerTime(guid, s.server.spiritWaveTimeLeftMs())
	}
	if s.inWintergraspBattlefield() {
		s.sendAreaSpiritHealerTime(guid, s.server.wgSpiritHealerTimeLeftMs())
	}
	return true
}

// handleAreaSpiritHealerQueue mirrors WorldSession::HandleAreaSpiritHealerQueueOpcode
// (MiscHandler.cpp:1482): same gates as the query, then the player joins the
// revive queue (== Battleground::AddPlayerToResurrectQueue, Battleground.cpp:1239 —
// the SPELL_WAITING_FOR_RESURRECT cast is the queue's only per-player effect).
// C++ queues only via the live Battleground / Battlefield. The Go-original
// interact/distance gate is dropped (C++ has none here); like C++, the queue
// handler itself sends no time packet — the client already received it from
// the query. The gossip-hello spirit-guide arm above is the one C++ path that
// sends the time packet right after queueing (NPCHandler.cpp:171-185), so it
// sends it there; the repop auto-queue arm below is the only other sender,
// since its ghosts never sent a query.
func (s *session) handleAreaSpiritHealerQueue(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return true
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// MiscHandler.cpp:1489-1491: the queue opcode shares the query opcode's
	// existence gate — a guid with no spawned creature on the player's map
	// is dropped even when its template carries the spirit-service flag.
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.luaCreature(ctx, guid) == nil {
		return true
	}
	if !s.creatureIsSpiritService(ctx, guid, npcFlagSpiritService) {
		return true
	}
	// MiscHandler.cpp:1484-1500: the queue opcode routes through the live
	// Battleground AND the Battlefield independently
	// (Battlefield::AddPlayerToResurrectQueue, Battlefield.cpp:605-616).
	// The BG leg needs a live instance (GetBattleground() non-null,
	// Player.h:1906) — a BG map alone does not queue.
	if s.inBattlegroundWaveMap() && s.bgData.InstanceID != 0 {
		s.server.spiritWaveMu.Lock()
		if s.server.spiritReviveQueue == nil {
			s.server.spiritReviveQueue = make(map[uint64]uint64)
		}
		s.server.spiritReviveQueue[s.playerGUID] = guid
		s.server.spiritWaveMu.Unlock()

		s.applyAura(2584) // SPELL_WAITING_FOR_RESURRECT
	}
	if s.inWintergraspBattlefield() {
		// BattlefieldWG::OnCreatureCreate spirit-guide assignment (see
		// assignWGSpiritGuide): Go has no creature-create dispatch for
		// DB spawns, so the guide is assigned when a ghost first queues
		// at it -- the ghost stands at the guide, so the player's
		// current area is the guide's area.
		_, areaID := s.server.zoneAndAreaID(s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Zone)
		s.server.assignWGSpiritGuide(guid, areaID)
		// BfGraveyard::AddPlayer (Battlefield.cpp:618-627): the waiting
		// aura lands only on a fresh queue insert.
		if s.server.queueWGSpiritRevive(s.playerGUID, guid) {
			s.applyAura(2584) // SPELL_WAITING_FOR_RESURRECT
		}
	}
	return true
}

// handleHearthAndResurrect mirrors WorldSession::HandleHearthAndResurrect (MiscHandler.cpp:1505):
// if flying, ignore. A player in a battlefield zone never reaches the
// Wintergrasp-flag repop arm: Battlefield::PlayerAskToLeave
// (Battlefield.cpp:382-387) teleports them to Dalaran (571, 5804.1499,
// 624.7710, 647.7670, 1.64) and returns. Otherwise, if the player's area
// has AREA_FLAG_WINTERGRASP_2, repop the player (creating a corpse if needed),
// resurrect with 100% health and powers, and teleport to the homebind location.
func (s *session) handleHearthAndResurrect(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.isInFlight() {
		return true
	}
	// MiscHandler.cpp:1508-1512 + BattlefieldMgr.cpp:121-133: the gate is
	// the zone id having a registered battlefield — Battlefield::m_IsEnabled
	// is not consulted (PlayerAskToLeave teleports unconditionally,
	// Battlefield.cpp:382-387), so a GM-disabled Wintergrasp still asks the
	// player to leave instead of repopping in place.
	if s.player.Zone == WGZoneID {
		s.teleportTo(WGMapID, 5804.1499, 624.7710, 647.7670, 1.64)
		return true
	}
	// MiscHandler.cpp:1517-1518: the Wintergrasp flag is read from the
	// player's AREA id (GetAreaId), not the zone — the server-side sub-area
	// from zoneAndAreaID mirrors it; the zone row alone may not carry it.
	canHearthAndRes := false
	if s.server.Data != nil {
		_, areaID := s.server.zoneAndAreaID(s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Zone)
		if areaID != 0 {
			area, found, err := s.server.Data.Area(areaID)
			if err == nil && found && area.Flags&wotlk.AreaFlagWintergrasp2 != 0 {
				canHearthAndRes = true
			}
		}
	}
	if !canHearthAndRes {
		return true
	}
	s.buildPlayerRepop(ctx, false)
	s.resurrectPlayer(ctx, 1.0)
	// MiscHandler.cpp:1519: TeleportTo(m_homebind...) runs unconditionally —
	// no stay-in-place fallback when the homebind is zero (the old
	// all-zero guard was an invented arm with no C++ counterpart).
	s.teleportTo(s.player.HomebindMap, s.player.HomebindX, s.player.HomebindY, s.player.HomebindZ, s.player.Orientation)
	return true
}

// resSicknessSpellID mirrors the ChrRaces ResSicknessSpellID lookup in
// Player::ResurrectPlayer (Player.cpp:4746): the race's own resurrection
// sickness spell, falling back to 15007 when the race row is absent or
// carries none.
func (s *session) resSicknessSpellID(race uint8) uint32 {
	if s != nil && s.server != nil && s.server.Data != nil {
		if r, found, _ := s.server.Data.Race(uint32(race)); found && r.ResSicknessSpellID != 0 {
			return r.ResSicknessSpellID
		}
	}
	return 15007
}

// applyResurrectionSickness mirrors the applySickness leg of
// Player::ResurrectPlayer (Player.cpp:4740-4753): characters at or above
// CONFIG_DEATH_SICKNESS_LEVEL (default 11) suffer the race's own
// ResSicknessSpellID — one minute per level above 10 for levels 11-19,
// ten minutes at level 20+. The C++ CastSpell(..., true) is a triggered
// apply, which applyAuraWithDuration mirrors.
func (s *session) applyResurrectionSickness() {
	// CONFIG_DEATH_SICKNESS_LEVEL ("Death.SicknessLevel", default 11,
	// Player.cpp:4743): one minute per level above startLevel-1 for
	// startLevel..startLevel+8, ten minutes at startLevel+9+.
	startLevel := uint8(11)
	if s != nil && s.server != nil {
		startLevel = uint8(s.server.Config.DeathSicknessLevel)
	}
	if s == nil || s.player == nil || s.player.Level < startLevel {
		return
	}
	durationMinutes := s.player.Level - startLevel + 1
	if durationMinutes > 10 {
		durationMinutes = 10
	}
	s.applyAuraWithDuration(s.resSicknessSpellID(s.player.Race), uint32(durationMinutes)*60*1000)
}

// handleSpiritHealerActivate processes CMSG_SPIRIT_HEALER_ACTIVATE (0x21C).
// Reference: WorldSession::HandleSpiritHealerActivateOpcode (NPCHandler.cpp:198)
// and WorldSession::SendSpiritResurrect (NPCHandler.cpp:219).
func (s *session) handleSpiritHealerActivate(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	if !s.creatureIsSpiritService(ctx, guid, npcFlagSpiritHealer) {
		return true
	}
	if !s.canInteractWithNPC(ctx, guid, uint64(npcFlagSpiritHealer)) {
		return true
	}
	// NPCHandler.cpp:212-214 (HandleSpiritHealerActivateOpcode) — activating
	// the spirit healer strips a faked death.
	s.removeFakeDeath()
	corpse, hasCorpse := s.loadCorpse(ctx)
	var corpseGrave wotlk.WorldSafeLoc
	corpseGraveFound := false
	if hasCorpse {
		// ObjectMgr.cpp:6864-6867 (GetClosestGraveyard): the zone is derived
		// from the corpse's coordinates (sMapMgr->GetZoneId), not the ghost's
		// current zone.
		corpseZone, _ := s.server.zoneAndAreaID(corpse.MapID, corpse.X, corpse.Y, corpse.Z, s.player.Zone)
		corpseGrave, corpseGraveFound = s.server.closestGraveyard(ctx, corpse.X, corpse.Y, corpse.Z, corpse.MapID, corpseZone, playerTeam(s.player.Race))
	}
	s.resurrectPlayerWithBones(ctx, 0.5, false, true)
	s.durabilityLossAll(ctx, 0.25, true)
	// Resurrection sickness is applied inside resurrectPlayerWithBones (the
	// ResurrectPlayer(0.5f, true) arm, NPCHandler.cpp:221), matching the
	// reference order: sickness before the durability loss.
	s.spawnCorpseBones(ctx)
	if corpseGraveFound {
		ghostGrave, ghostGraveFound := s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.Zone, playerTeam(s.player.Race))
		if !ghostGraveFound || corpseGrave.ID != ghostGrave.ID {
			s.teleportTo(corpseGrave.MapID, corpseGrave.X, corpseGrave.Y, corpseGrave.Z, s.player.Orientation)
		}
	}
	return true
}

// handleCorpseQuery processes MSG_CORPSE_QUERY (0x216).
// Reference: WorldSession::HandleCorpseQueryOpcode (QueryHandler.cpp:144).
func (s *session) handleCorpseQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	corpse, ok := s.loadCorpse(ctx)
	if !ok {
		buf := protocol.NewBuffer(1)
		buf.WriteU8(0) // corpse not found
		_ = s.write(uint16(protocol.OpcodeMSG_CORPSE_QUERY), buf.Bytes(), true)
		return true
	}
	// QueryHandler.cpp:161-178: when the corpse lies on a different map than the
	// player and that map is a dungeon with a corpse entrance map, the marker is
	// drawn at the dungeon entrance instead of the (undisplayable) interior.
	mapID, x, y, z := corpse.MapID, corpse.X, corpse.Y, corpse.Z
	if mapID != s.player.Map && s.server.Data != nil {
		if mapEntry, found, err := s.server.Data.Map(mapID); err == nil && found &&
			mapEntry.IsDungeon() && mapEntry.CorpseMapID >= 0 {
			if _, entranceFound, entranceErr := s.server.Data.Map(uint32(mapEntry.CorpseMapID)); entranceErr == nil && entranceFound {
				mapID = uint32(mapEntry.CorpseMapID)
				x, y = mapEntry.CorpseX, mapEntry.CorpseY
				// QueryHandler.cpp:175: the entrance Z is
				// entranceMap->GetHeight(phaseMask, x, y, MAX_HEIGHT). From
				// MAX_HEIGHT the vmap ray (50 down) and the dynamic GO-floor
				// ray provably never hit, so mapHeight reduces exactly to the
				// grid height; only a missing grid still differs (C++ sends
				// INVALID_HEIGHT, Go keeps the corpse's Z so the marker stays
				// drawable).
				if height, ok := s.server.mapHeight(mapID, x, y, terrainMaxHeight, true, terrainDefaultHeightSearch); ok {
					z = height
				}
			}
		}
	}
	buf := protocol.NewBuffer(25)
	buf.WriteU8(1) // corpse found
	buf.WriteU32(mapID)
	buf.WriteF32(x)
	buf.WriteF32(y)
	buf.WriteF32(z)
	buf.WriteU32(corpse.MapID)
	buf.WriteU32(0) // unknown
	_ = s.write(uint16(protocol.OpcodeMSG_CORPSE_QUERY), buf.Bytes(), true)
	return true
}

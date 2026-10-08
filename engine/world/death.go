package world

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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
	// playerFieldByteReleaseTimer is PLAYER_FIELD_BYTE_RELEASE_TIMER within
	// PLAYER_FIELD_BYTES byte 0 (Player.h:417); set while the client shows the
	// auto release spirit countdown.
	playerFieldByteReleaseTimer uint32 = 0x00000008
	unitFieldPlayerFieldBytes          = 1197 // PLAYER_FIELD_BYTES = UNIT_END + 0x0419

	deathExpireStepSeconds = 5 * 60 // DEATH_EXPIRE_STEP (Player.cpp:174)
	maxDeathCount          = 3      // MAX_DEATH_COUNT (Player.cpp:175)

	autoRepopDelay = 6 * time.Minute // KillPlayer m_deathTimer (Player.cpp:4786)

	corpseReclaimRadius = 39.0 // CORPSE_RECLAIM_RADIUS (Corpse.h:35)

	corpseTypeBones     uint32 = 0
	corpseTypePvE       uint32 = 1 // CORPSE_RESURRECTABLE_PVE
	corpseTypePvP       uint32 = 2 // CORPSE_RESURRECTABLE_PVP
	corpsePhaseAuraType uint32 = 261
	corpseFlagBones     uint32 = 0x01
	corpseFlagUnk2      uint32 = 0x04
	corpseFlagLootable  uint32 = 0x20 // CORPSE_FLAG_LOOTABLE (Corpse.h:45)
	playerFlagHideHelm  uint32 = 0x00000400
	playerFlagHideCloak uint32 = 0x00000800

	teamAlliance uint32 = 469
	teamHorde    uint32 = 67

	defaultGraveyardAlliance uint32 = 4  // Westfall (ObjectMgr.cpp:6855)
	defaultGraveyardHorde    uint32 = 10 // Crossroads (ObjectMgr.cpp:6853)
)

func (s *session) currentPlayerPhaseMask() uint32 {
	if s == nil || s.player == nil {
		return 1
	}
	if s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0 {
		return ^uint32(0)
	}
	s.castMu.Lock()
	phaseMask := uint32(0)
	for _, aura := range s.activeAuras {
		if aura != nil && !aura.Stopped && aura.AuraType == corpsePhaseAuraType {
			phaseMask |= uint32(aura.MiscValue)
		}
	}
	s.castMu.Unlock()
	if phaseMask == 0 {
		return 1
	}
	return phaseMask
}

func (s *Server) buildNearbyCorpseUpdates(ctx context.Context, state playerState, phaseMask uint32) (*protocol.Packet, int, error) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil || s.Config.VisibilityDistanceContinents <= 0 {
		return nil, 0, nil
	}
	distance := float64(s.Config.VisibilityDistanceContinents)
	// Reference Map::LoadCorpseData (Map.cpp:4551-4583) scopes the map's
	// corpse grid by (mapId, instanceId): corpses from another dungeon
	// instance of the same map are never visible.
	rows, err := s.CharactersStore.DB.QueryContext(ctx, `SELECT guid, mapId, posX, posY, posZ, orientation, displayId, itemCache, bytes1, bytes2, guildId, flags, dynFlags, corpseType, phaseMask
		FROM corpse WHERE mapId = ? AND instanceId = ? AND posX BETWEEN ? AND ? AND posY BETWEEN ? AND ? ORDER BY guid`, state.Map, state.InstanceID, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance)
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
	// row, Corpse.cpp:155-159); one batched template lookup covers every
	// corpse on the grid.
	var worldDB *sql.DB
	if s.WorldStore != nil {
		worldDB = s.WorldStore.DB
	}
	caches := make([][corpseItemSlots]uint32, len(corpses))
	if worldDB != nil {
		parsed := make([][]uint32, len(corpses))
		entries := make([]uint32, 0, len(corpses)*corpseItemSlots)
		seen := make(map[uint32]bool)
		for i, c := range corpses {
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
		corpseGUID := guid | (uint64(0xF101) << 48)
		block := buildCorpseCreateBlockWithFields(corpseGUID, corpseObjectFields{OwnerGUID: ownerGUID, DisplayID: uint32(c.displayID), Items: caches[i], Bytes1: uint32(c.bytes1), Bytes2: uint32(c.bytes2), GuildID: uint32(c.guildID), Flags: uint32(c.flags), DynamicFlags: uint32(c.dynamicFlags)}, float32(c.x), float32(c.y), float32(c.z), float32(c.orientation))
		updates.AddUpdateBlock(block)
		count++
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
// killer is the killing player session (nil for environment, fall, and GM
// kills) — the Go analog of Unit::Kill's
// attacker->GetCharmerOrOwnerPlayerOrPlayerItself() arm (Unit.cpp:11279).
// pvpDeath carries Unit::Kill's attacker arm (Unit.cpp:11341-11343): true when
// the killer resolves to a player (attacker->GetCharmerOrOwnerPlayerOrPlayerItself()),
// feeding the corpse type and the reclaim delay.
func (s *session) killPlayer(ctx context.Context, killer *session, pvpDeath bool) {
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
	// ThreatManager::RemoveMeFromThreatLists on the dead player
	// (ThreatManager.cpp:690-697, reached on player death through
	// Unit::setDeathState → CombatStop, Unit.cpp:8901-8907): a dead
	// player must stop being a victim in every creature/pet threat
	// table on the map/instance, exactly like a dead creature.
	// Placed first, matching the C++ order where setDeathState (and
	// its CombatStop) runs inside Unit::Kill ahead of all post-kill
	// processing. Skipped while the spirit form holds: the player is
	// still alive, so the threat leg runs at the deferred death.
	if !spiritOfRedemption && s.server != nil {
		s.server.removeThreatVictimFromAllLists(s.player.Map, s.player.InstanceID, s.playerGUID)
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
	}
	// Player::setDeathState(JUST_DIED), Player.cpp:1412 — RemovePet(nullptr,
	// PET_SAVE_NOT_IN_SLOT, true) dismisses the pet at death rather than
	// keeping it alive on a dead owner; RemoveGhoul has no Go analog (no
	// raised-ghoul pet model). Placed before the penalty/achievement legs,
	// matching the C++ relative order. Skipped while the spirit form holds.
	if !spiritOfRedemption {
		s.unsummonPet(ctx, petSaveNotInSlot)
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
		s.resurrection = nil
	}
	// Unit::Kill (Unit.cpp:11341-11343): remember the victim's PvP death for
	// corpse type and corpse reclaim delay, stored until CreateCorpse (the
	// SetPvPDeath(player != nullptr) leg). Placed after the pet arm, matching
	// the C++ relative order. Runs at the original death even under Spirit of
	// Redemption (Unit.cpp:11344-11346: "at original death (not at
	// SpiritOfRedemtionTalent timeout)").
	s.pvpDeath = pvpDeath
	if !spiritOfRedemption {
		if s.playerLoaded && s.player.PlayerFieldBytes&playerFieldByteReleaseTimer == 0 {
			s.player.PlayerFieldBytes |= playerFieldByteReleaseTimer
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
		s.server.handleWSGPlayerDeath(s)
		s.server.handleEOTSPlayerDeath(s)
		s.server.handleAVPlayerDeath(s)
		s.server.handleSAPlayerDeath(s)
		s.server.handleICPlayerDeath(s)
		s.server.handleArenaPlayerDeath(s)
		s.server.handleWGPlayerDeath(s, nil)
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
		killer.procKillAuraTriggers(ctx, s.playerGUID, killer.playerGUID, procFlagKill)
	}
	s.procKillAuraTriggers(ctx, killerGUID, killerGUID, procFlagKilled)
	s.procKillAuraTriggers(ctx, s.playerGUID, s.playerGUID, procFlagDeath)
	if killer != nil {
		killer.creditKillingBlowCriteria()
	}
	if spiritOfRedemption {
		// Unit::Kill Spirit of Redemption arm (Unit.cpp:11302-11314):
		// RemoveAllAurasOnDeath, then CastSpell(27827). Go holds no passive
		// aura instances, so clearActiveAuras is the RemoveAllAurasOnDeath
		// analog over the applied set; the talent passive survives in the
		// learned-spell list. PLAYER_SELF_RES_SPELL needs no save/restore:
		// clearActiveAuras never touches the player field, and the
		// GetResurrectionSpellId fill sub-arm is unmodeled (no
		// reincarnation/soulstone self-res model in Go). The 27827 apply leg
		// (stand state + SetHealth(1)) lives in applyAuraWithDuration.
		s.clearActiveAuras()
		s.applyAuraWithDuration(spiritOfRedemptionSpellID, spiritOfRedemptionDurationMs)
	} else {
		s.clearActiveAuras()
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
	// Unit::Kill (Unit.cpp:11351-11357): 10% durability loss on death, but
	// not for PvP deaths — CONFIG_DURABILITY_LOSS_IN_PVP defaults false —
	// and not in battlegrounds (no BG-membership model on the session; the
	// PvP-death gate covers the player-killer case).
	if !s.pvpDeath {
		s.durabilityLossAll(ctx, 0.10, false)
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
		s.server.removeThreatVictimFromAllLists(s.player.Map, s.player.InstanceID, s.playerGUID)
	}
	// Deferred Unit::setDeathState(JUST_DIED) cascade: the spirit form kept
	// the player alive, so the cast/channel interrupt (and its GCD refund)
	// lands now, matching Unit::Kill's deferred setDeathState.
	s.interruptCurrentCast()
	s.interruptCurrentChannel()
	s.unsummonPet(ctx, petSaveNotInSlot)
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
	s.clearActiveAuras()
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
	// !InArena() sub-arm has no bridge — Go has no arena system — and the
	// insignia loot itself is documented unbridged at loot.go:601.
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

func (s *session) spawnCorpseObject(ctx context.Context, displayID uint32, battlegroundLootable bool) {
	if s.player == nil {
		return
	}
	corpseGUID := s.playerGUID | (uint64(0xF101) << 48)
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
			s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
		}
	}
}

func (s *session) despawnCorpseObject() {
	corpseGUID := s.playerGUID | (uint64(0xF101) << 48)
	updates := protocol.NewUpdateData()
	updates.AddOutOfRangeGUID(corpseGUID)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
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
	bgLootable := s.bgData.InstanceID != 0
	// Player::CreateCorpse (Player.cpp:4812) opens with SpawnCorpseBones():
	// a second death converts the existing corpse to bones
	// (Map::ConvertCorpseToBones deletes the old row outside the BG/arena
	// save-skip gate and spawns the ownerless bones visual per the
	// DEATH_BONES configs) instead of only deleting the row.
	// convertCorpseToBones is the Go analog; it early-returns when no row
	// exists, matching ConvertCorpseToBones' GetCorpseByPlayer miss.
	s.convertCorpseToBones(ctx, false)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && !bgLootable {
		// Column order mirrors SaveToDB (Corpse.cpp:102-119): the dynFlags
		// 0 is exact — Player::CreateCorpse never sets CORPSE_FIELD_DYNAMIC_FLAGS
		// on a fresh corpse — and instanceId is the map's instance id
		// (Corpse::SaveToDB writes GetInstanceId(), Object.cpp:1846), 0 on
		// world maps, the dungeon/BG instance id elsewhere.
		// Player::CreateCorpse (Player.cpp:4867-4868) skips the DB save when
		// the map is a battleground or arena — "we do not need to save
		// corpses for BG/arenas" — while still creating the in-memory corpse
		// object (with the LOOTABLE flag for the insignia). Go has no arena
		// system, so the gate is the battleground leg only: the old row is
		// still deleted (C++ ConvertCorpseToBones deletes it outside the
		// gate), but no new row is written, so BG deaths leave no stale
		// corpse row behind for the reclaim / map-admission / login loaders.
		bytes1, bytes2 := corpseAppearance(s.player)
		_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_CORPSE",
			s.playerGUID, s.player.X, s.player.Y, s.player.Z, s.player.Orientation, s.player.Map,
			displayID, s.player.Equipment, bytes1, bytes2, s.player.GuildID, corpseFlags(s.player, bgLootable), 0, time.Now().Unix(), corpseType, s.player.InstanceID, s.currentPlayerPhaseMask())
	}

	s.player.PlayerFlags |= playerFlagGhost
	s.player.PlayerFieldBytes &^= playerFieldByteReleaseTimer
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

	grave, ok := s.server.closestGraveyard(ctx, s.player.X, s.player.Y, s.player.Z, s.player.Map, s.player.Zone, playerTeam(s.player.Race))
	if ok {
		s.teleportTo(grave.MapID, grave.X, grave.Y, grave.Z, s.player.Orientation)
		if shouldResurrect {
			// TELE_REVIVE_AT_TELEPORT: TeleportTo resurrects at half
			// health/mana inside the teleport (Player.cpp:1749-1750).
			s.resurrectPlayer(ctx, 0.5)
		} else if s.isDeadOrGhost() {
			// SMSG_DEATH_RELEASE_LOC is sent only when dead (Player.cpp:5146);
			// the alive homebind-fallback caller skips it.
			packet := protocol.NewBuffer(16)
			packet.WriteU32(grave.MapID)
			packet.WriteF32(grave.X)
			packet.WriteF32(grave.Y)
			packet.WriteF32(grave.Z)
			_ = s.write(uint16(protocol.OpcodeSMSG_DEATH_RELEASE_LOC), packet.Bytes(), true)
		}
	} else {
		s.debug("no graveyard found, staying at current location", "account", s.accountName, "guid", s.playerGUID)
	}

	// Reference clears PLAYER_FLAGS_IS_OUT_OF_BOUNDS on every repop
	// (Player.cpp:5155); Go never sets the flag, so the clear is a no-op.
	s.player.PlayerFlags &^= playerFlagOutOfBounds

	// In battlegrounds, automatically queue for wave resurrection (Go-original:
	// C++ queues only when the ghost talks to the spirit healer and sends
	// CMSG_AREA_SPIRIT_HEALER_QUEUE; the time packet below keeps the client rez
	// timer in sync since these ghosts never sent a query).
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

// closestGraveyard mirrors ObjectMgr::GetClosestGraveyard.
func (s *Server) closestGraveyard(ctx context.Context, x, y, z float32, mapID, zoneID, team uint32) (wotlk.WorldSafeLoc, bool) {
	// BattlegroundIC::GetClosestGraveyard replaces the generic zone lookup
	// wholesale for IoC: owned-node graves first, per-team fallback after.
	if mapID == ICMapID {
		if loc, found := s.closestICGraveyard(x, y, team); found {
			return loc, true
		}
	}
	// BattlegroundSA::GetClosestGraveyard replaces the generic zone lookup
	// wholesale for SotA: beach/defender-last fallback plus the nearest owned
	// capturable graveyard.
	if mapID == SAMapID {
		if loc, found := s.closestSAGraveyard(x, y, z, team); found {
			return loc, true
		}
	}
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
		} else if far == nil {
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
		return false
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.player.Health > 0 || s.player.PlayerFlags&playerFlagGhost != 0 || s.hasAuraType(spellAuraPreventResurrection) {
		return true
	}
	// WorldSession::HandleRepopRequest, MiscHandler.cpp:87-89 — dismiss any
	// pet before the repop (e.g. a warlock re-summon while dead); RemoveGhoul
	// has no Go analog (see killPlayer).
	s.unsummonPet(ctx, petSaveNotInSlot)
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
	var visuals []uint64
	if now.Sub(s.lastSpiritWave) >= 30*time.Second {
		s.lastSpiritWave = now
		if len(s.spiritReviveQueue) != 0 {
			if s.spiritResurrectPending == nil {
				s.spiritResurrectPending = make(map[uint64]spiritWavePending)
			}
			for pGUID, spiritGUID := range s.spiritReviveQueue {
				visuals = append(visuals, pGUID)
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

	for _, pGUID := range visuals {
		if sess := s.findSessionByGUID(pGUID); sess != nil && sess.worldReady.Load() && sess.player != nil && sess.player.PlayerFlags&playerFlagGhost != 0 {
			sess.applyAura(24171) // SPELL_RESURRECTION_VISUAL
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
	MapID, DisplayID, Bytes1, Bytes2, Flags, DynamicFlags uint32
	GuildID                                               uint32
	CorpseType                                            uint32
	GhostTime                                             int64
	X, Y, Z, Orientation                                  float32
}

func (s *session) loadCorpseObject(ctx context.Context) (corpseObjectState, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return corpseObjectState{}, false
	}
	var mapID, displayID, bytes1, bytes2, flags, dynamicFlags, corpseType, ghostTime int64
	var corpse corpseObjectState
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId, posX, posY, posZ, orientation, displayId, bytes1, bytes2, flags, dynFlags, corpseType, time FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones).Scan(&mapID, &corpse.X, &corpse.Y, &corpse.Z, &corpse.Orientation, &displayID, &bytes1, &bytes2, &flags, &dynamicFlags, &corpseType, &ghostTime); err != nil {
		return corpseObjectState{}, false
	}
	corpse.MapID = uint32(mapID)
	corpse.DisplayID = uint32(displayID)
	corpse.Bytes1 = uint32(bytes1)
	corpse.Bytes2 = uint32(bytes2)
	corpse.Flags = uint32(flags)
	corpse.DynamicFlags = uint32(dynamicFlags)
	corpse.CorpseType = uint32(corpseType)
	corpse.GhostTime = ghostTime
	var guildID int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guildId FROM corpse WHERE guid = ?", s.playerGUID).Scan(&guildID); err == nil {
		corpse.GuildID = uint32(guildID)
	}
	return corpse, true
}

func (s *session) sendLoadedCorpse(ctx context.Context) bool {
	corpse, ok := s.loadCorpseObject(ctx)
	if !ok {
		return false
	}
	pvp := corpse.CorpseType == corpseTypePvP
	reclaimEnabled := s.server.Config.DeathCorpseReclaimDelayPvE
	if pvp {
		reclaimEnabled = s.server.Config.DeathCorpseReclaimDelayPvP
	}
	if delay, ok := CalculateLoadedCorpseReclaimDelay(time.Now().Unix(), s.deathExpireTime, corpse.GhostTime, reclaimEnabled); ok {
		s.sendCorpseReclaimDelay(delay)
	}
	s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
	return true
}

func (s *session) prepareLoginResurrection(ctx context.Context, state *playerState) {
	if s == nil || state == nil || state.AtLogin&uint32(atLoginResurrect) == 0 {
		return
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ?", state.GUID)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ?, health = ?, playerFlags = ?, death_expire_time = 0 WHERE guid = ?", ^uint32(atLoginResurrect), maxUint32(state.MaxHealth/2, 1), state.PlayerFlags&^playerFlagGhost, state.GUID)
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
	state.PlayerFlags &^= playerFlagGhost
	state.PlayerFieldBytes &^= playerFieldByteReleaseTimer
	state.Health = maxUint32(state.MaxHealth/2, 1)
	state.Powers[0] = state.MaxPowers[0] / 2
	state.Powers[1] = 0
	state.Powers[3] = state.MaxPowers[3] / 2
	state.AtLogin &^= uint32(atLoginResurrect)
	s.deathExpireTime = 0
}

func battlegroundMap(mapID uint32) bool {
	switch mapID {
	case 30, 489, 529, 566, 559, 562, 572, 607, 617, 618, 628:
		return true
	default:
		return false
	}
}

func (s *session) shouldCreateCorpseBones(mapID uint32) bool {
	if s == nil || s.server == nil {
		return false
	}
	if battlegroundMap(mapID) {
		return s.server.Config.DeathBonesBattleground
	}
	return s.server.Config.DeathBonesWorld
}

// spawnCorpseBones mirrors Player::SpawnCorpseBones and Map::ConvertCorpseToBones:
// remove the resurrectable corpse from persistence, then optionally create ownerless bones at its stored location.
func (s *session) spawnCorpseBones(ctx context.Context) {
	s.convertCorpseToBones(ctx, false)
}

func (s *session) spawnLoadedCorpseBones(ctx context.Context) {
	s.convertCorpseToBones(ctx, true)
}

func (s *session) convertCorpseToBones(ctx context.Context, destroyVisible bool) {
	corpse, ok := s.loadCorpseObject(ctx)
	if !ok {
		return
	}
	if destroyVisible {
		s.despawnCorpseObject()
	}
	if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ? AND corpseType <> ?", s.playerGUID, corpseTypeBones); err != nil {
		return
	}
	if !s.shouldCreateCorpseBones(corpse.MapID) {
		return
	}
	corpseGUID := s.playerGUID | (uint64(0xF101) << 48)
	fields := corpseObjectFields{DisplayID: corpse.DisplayID, Bytes1: corpse.Bytes1, Bytes2: corpse.Bytes2, GuildID: corpse.GuildID, Flags: corpseFlagUnk2 | corpseFlagBones, DynamicFlags: corpse.DynamicFlags}
	block := buildCorpseCreateBlockWithFields(corpseGUID, fields, corpse.X, corpse.Y, corpse.Z, corpse.Orientation)
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block)
	packet, err := updates.BuildPacket(0)
	if err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
	}
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
// bones. An online owner's visible corpse is despawned and replaced with
// bones via spawnLoadedCorpseBones (the RemoveCorpse + ConvertCorpseToBones
// arm); an offline owner's row is simply deleted — C++ bones are never
// persisted either, so with no session to show them the delete is
// outcome-equivalent. Returns the number of expired corpses handled.
// No-bridge: the 60-minute bones expiry (Corpse.cpp:209, Map.cpp:4693-4701)
// has no Go analog — Go bones are fire-and-forget client visuals with no
// server-side state (no _corpseBones analog, no DB row), so nothing exists
// to expire; clients clear them on map change/logout. The grid-loaded leg
// (!IsRemovalGrid in ConvertCorpseToBones) likewise has no analog — Go has
// no grid model and the packet broadcast is unconditional.
func (s *Server) expireOldCorpses(ctx context.Context) int {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return 0
	}
	cutoff := time.Now().Unix() - int64(corpseResurrectableExpiry/time.Second)
	rows, err := s.CharactersStore.DB.QueryContext(ctx, "SELECT guid FROM corpse WHERE corpseType <> ? AND time < ?", corpseTypeBones, cutoff)
	if err != nil {
		return 0
	}
	var guids []uint64
	for rows.Next() {
		var guid uint64
		if err := rows.Scan(&guid); err == nil {
			guids = append(guids, guid)
		}
	}
	_ = rows.Close()
	handled := 0
	for _, guid := range guids {
		if sess := s.findSessionByGUID(guid); sess != nil {
			sess.spawnLoadedCorpseBones(ctx)
		} else if _, err := s.CharactersStore.DB.ExecContext(ctx, "DELETE FROM corpse WHERE guid = ? AND corpseType <> ?", guid, corpseTypeBones); err != nil {
			continue
		}
		handled++
	}
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

// deleteCorpseDataForResetBinds mirrors Map::DeleteCorpseData as run from the
// instance-reset path (InstanceSaveMgr.cpp:618, Map.cpp:4182): every
// non-permanent dungeon bind the reset drops gets its corpse rows deleted via
// CHAR_DEL_CORPSES_FROM_MAP, so no corpse survives the reset. The registered
// statement existed but was never called. Binds are read before the caller
// deletes the character_instance rows.
func (s *session) deleteCorpseDataForResetBinds(ctx context.Context, charGUID uint64) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil {
		return
	}
	for _, b := range s.instanceBindsForCharacter(ctx, charGUID) {
		if b.permanent || !s.isDungeonMap(b.mapID) {
			continue
		}
		_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_DEL_CORPSES_FROM_MAP", b.mapID, b.instanceID)
	}
}

// resurrectPlayer mirrors Player::ResurrectPlayer for the core state: clear
// the ghost flag and death timer, restore land walking and control, and point
// the corpse map at an invalid map id. When restorePercent is positive the
// reference health and power restoration applies (half of maximum health and
// mana, zero rage, half energy).
func (s *session) resurrectPlayer(ctx context.Context, restorePercent float32) {
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
	s.player.PlayerFieldBytes &^= playerFieldByteReleaseTimer
	s.deathTimer = time.Time{}
	s.removeAura(8326)
	s.removeAura(20584)
	s.despawnCorpseObject()
	if restorePercent > 0 {
		s.player.Health = uint32(float32(s.player.MaxHealth) * restorePercent)
		s.player.Powers[0] = uint32(float32(s.player.MaxPowers[0]) * restorePercent) // mana
		s.player.Powers[1] = 0                                                       // rage
		s.player.Powers[3] = uint32(float32(s.player.MaxPowers[3]) * restorePercent) // energy
	}
	s.persistResurrectionState(ctx)
	s.sendPlayerUpdate()
	if s.player.Map == ICMapID && s.server != nil {
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
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ?, playerFlags = ?, death_expire_time = 0 WHERE guid = ?", s.player.Health, s.player.PlayerFlags, s.playerGUID)
}

// resurrectionData mirrors Player::_resurrectionData (ResurrectionData):
// the caster, the caster location for the teleport, and the restored health
// and mana values carried by the resurrect spell effect.
type resurrectionData struct {
	GUID    uint64
	MapID   uint32
	X, Y, Z float32
	Health  uint32
	Mana    uint32
}

// setResurrectRequestData mirrors Player::SetResurrectRequestData. The
// reference asserts that no request is outstanding; the caller is expected to
// check first, so an overwrite here is logged and refused.
func (s *session) setResurrectRequestData(casterGUID uint64, mapID uint32, x, y, z float32, health, mana uint32) {
	if s.resurrection != nil {
		s.debug("resurrect request overwritten", "account", s.accountName, "guid", s.playerGUID)
		return
	}
	s.resurrection = &resurrectionData{GUID: casterGUID, MapID: mapID, X: x, Y: y, Z: z, Health: health, Mana: mana}
}

// sendResurrectRequest mirrors Spell::SendResurrectRequest: raw caster GUID,
// length-prefixed caster name (empty for player casters, the client resolves
// those by GUID), the spirit healer resurrection sickness flag, and the flag
// overriding the corpse reclaim delay for spells that ignore the timer.
func (s *session) sendResurrectRequest(casterGUID uint64, name string, spiritHealer, ignoreReclaimTimer bool) {
	packet := protocol.NewBuffer(24 + len(name))
	packet.WriteU64(casterGUID)
	packet.WriteU32(uint32(len(name)) + 1)
	packet.WriteString(name)
	packet.WriteU8(boolByte(spiritHealer))
	packet.WriteU8(boolByte(ignoreReclaimTimer))
	_ = s.write(uint16(protocol.OpcodeSMSG_RESURRECT_REQUEST), packet.Bytes(), true)
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
	reader := protocol.NewReader(payload)
	// MiscPackets.cpp:237-241 — Resurrecter is a packed client GUID, not a
	// raw uint64. Reading it raw consumed the mask plus seven GUID bytes and
	// left the eighth GUID byte to be misread as Response, so accept packets
	// never matched the stored resurrecter and resurrection requests silently
	// failed.
	resurrecter, err := reader.ReadPackedGUID()
	if err != nil {
		return false
	}
	response, err := reader.ReadU8()
	if err != nil {
		return false
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
	// Reference teleports to the caster location before resurrecting so the
	// player does not revive into nearby creatures at the corpse; the delayed
	// teleport retry path has no Go equivalent because teleportTo is sync.
	if data.MapID != s.player.Map || data.X != s.player.X || data.Y != s.player.Y || data.Z != s.player.Z {
		s.teleportTo(data.MapID, data.X, data.Y, data.Z, s.player.Orientation)
	}
	s.resurrectPlayer(ctx, 0)
	s.player.Health = data.Health
	s.player.Powers[0] = data.Mana
	s.player.Powers[1] = 0 // rage
	if s.player.MaxPowers[3] > 0 {
		s.player.Powers[3] = s.player.MaxPowers[3] // full energy
	}
	s.persistResurrectionState(ctx)
	s.resurrection = nil
	s.spawnCorpseBones(ctx)
	s.sendPlayerUpdate()
	return true
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

// handleReclaimCorpse mirrors WorldSession::HandleReclaimCorpse: a ghost in
// range of its own resurrectable corpse after the reclaim delay elapses is
// resurrected (half health/mana, full in battlegrounds) and the corpse is
// turned into bones. The arena guard has no Go arena system yet.
func (s *session) handleReclaimCorpse(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	_, err := reader.ReadU64()
	if err != nil {
		reader = protocol.NewReader(payload)
		if _, err = reader.ReadPackedGUID(); err != nil {
			return false
		}
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
	// (Player.h:1906); the arena reclaim block stays documented unbridged (no
	// Go arena system).
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
	s.player.SelfResSpell = 0
	s.sendPlayerUpdate()
	if s.server.Data == nil {
		return true
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		s.debug("self resurrect spell lookup failed", "account", s.accountName, "spell", spellID, "found", found, "error", err)
		return true
	}
	s.finishSpellCast(ctx, 0, spellID, spell, protocol.SpellTargetData{}, 0, 0)
	return true
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
			health = uint32(effect.BasePoints + 1) // damage as computed by CalcValue
		}
		mana := uint32(0)
		if effect.MiscValue > 0 {
			mana = uint32(effect.MiscValue)
		}
		s.setResurrectRequestData(s.playerGUID, s.player.Map, s.player.X, s.player.Y, s.player.Z, health, mana)
		s.sendResurrectRequest(s.playerGUID, "", false, false)
		return
	}
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
// (MiscHandler.cpp:1459): the creature must exist and offer spirit service
// (== GetMap()->GetCreature + IsSpiritService; C++ performs no interact/distance
// check here). The time packet is answered only in a battleground context —
// C++ sends it only via the live Battleground / Battlefield, otherwise silence.
func (s *session) handleAreaSpiritHealerQuery(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if !s.creatureIsSpiritService(ctx, guid, npcFlagSpiritService) {
		return true
	}
	if !s.inBattlegroundWaveMap() {
		return true
	}
	s.sendAreaSpiritHealerTime(guid, s.server.spiritWaveTimeLeftMs())
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
		return false
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if !s.creatureIsSpiritService(ctx, guid, npcFlagSpiritService) {
		return true
	}
	if !s.inBattlegroundWaveMap() {
		return true
	}
	s.server.spiritWaveMu.Lock()
	if s.server.spiritReviveQueue == nil {
		s.server.spiritReviveQueue = make(map[uint64]uint64)
	}
	s.server.spiritReviveQueue[s.playerGUID] = guid
	s.server.spiritWaveMu.Unlock()

	s.applyAura(2584) // SPELL_WAITING_FOR_RESURRECT
	return true
}

// handleHearthAndResurrect mirrors WorldSession::HandleHearthAndResurrect (MiscHandler.cpp:1505):
// if flying, ignore. Battlefield ask-to-leave has no Go battlefield system yet.
// If the player's area has AREA_FLAG_WINTERGRASP_2, repop the player (creating a corpse if needed),
// resurrect with 100% health and powers, and teleport to the homebind location.
func (s *session) handleHearthAndResurrect(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.isInFlight() {
		return true
	}
	canHearthAndRes := false
	if s.server.Data != nil && s.player.Zone != 0 {
		area, found, err := s.server.Data.Area(s.player.Zone)
		if err == nil && found && area.Flags&wotlk.AreaFlagWintergrasp2 != 0 {
			canHearthAndRes = true
		}
	}
	if !canHearthAndRes {
		return true
	}
	s.buildPlayerRepop(ctx, false)
	s.resurrectPlayer(ctx, 1.0)
	destMap := s.player.HomebindMap
	destX := s.player.HomebindX
	destY := s.player.HomebindY
	destZ := s.player.HomebindZ
	if destMap == 0 && destX == 0 && destY == 0 && destZ == 0 {
		destMap = s.player.Map
		destX, destY, destZ = s.player.X, s.player.Y, s.player.Z
	}
	s.teleportTo(destMap, destX, destY, destZ, s.player.Orientation)
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
	corpse, hasCorpse := s.loadCorpse(ctx)
	var corpseGrave wotlk.WorldSafeLoc
	corpseGraveFound := false
	if hasCorpse {
		corpseGrave, corpseGraveFound = s.server.closestGraveyard(ctx, corpse.X, corpse.Y, corpse.Z, corpse.MapID, s.player.Zone, playerTeam(s.player.Race))
	}
	s.resurrectPlayer(ctx, 0.5)
	s.durabilityLossAll(ctx, 0.25, true)
	if s.player.Level > 10 {
		// Characters level 1-10 have no sickness (CONFIG_DEATH_SICKNESS_LEVEL
		// default 11, Player.cpp:4748).
		// Characters level 11-19 suffer 1 minute per level above 10 (1-9 minutes).
		// Characters level 20+ suffer 10 minutes of sickness (TC Player::ResurrectPlayer:4740-4753).
		// The spell is the race's own ResSicknessSpellID (Player.cpp:4746), not
		// a hardcoded 15007.
		durationMinutes := s.player.Level - 10
		if durationMinutes > 10 {
			durationMinutes = 10
		}
		s.applyAuraWithDuration(s.resSicknessSpellID(s.player.Race), uint32(durationMinutes)*60*1000)
	}
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
	buf := protocol.NewBuffer(25)
	buf.WriteU8(1) // corpse found
	buf.WriteU32(corpse.MapID)
	buf.WriteF32(corpse.X)
	buf.WriteF32(corpse.Y)
	buf.WriteF32(corpse.Z)
	buf.WriteU32(corpse.MapID)
	buf.WriteU32(0) // unknown
	_ = s.write(uint16(protocol.OpcodeMSG_CORPSE_QUERY), buf.Bytes(), true)
	return true
}

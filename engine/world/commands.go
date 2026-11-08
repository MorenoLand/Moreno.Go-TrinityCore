package world

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/crypto"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func (s *session) sendSysMessage(msg string) {
	if s != nil && s.addonCmdActive {
		// Reference: AddonChannelCommandHandler::SendSysMessage
		// (Chat.cpp:938-957) — while a "TrinityCore\t" remote-console
		// command runs, output goes back as framed addon whispers to self,
		// not as system chat messages.
		s.sendAddonChannelSysMessage(msg)
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), protocol.BuildSystemChatMessage(msg), true)
}

func (s *session) sendPlayerUpdate() {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	if !s.playerLoaded {
		updates, err := s.server.buildPlayerUpdate(*s.player)
		if err == nil && updates != nil {
			_ = s.write(updates.Opcode, updates.Payload.Bytes(), true)
		}
		return
	}
	channelGUID, channelSpell := uint64(0), uint32(0)
	s.castMu.Lock()
	if s.activeChannel != nil {
		channelGUID, channelSpell = s.activeChannel.ChannelObjectGUID, s.activeChannel.SpellID
	}
	s.castMu.Unlock()
	fields := map[int]uint32{
		unitFieldHealth:                   s.player.Health,
		unitFieldMaxHealth:                s.effectiveMaxHealth(),
		unitFieldLevel:                    uint32(s.player.Level),
		unitFieldFaction:                  s.effectiveFactionTemplate(s.player.Race),
		unitFieldFlags:                    s.player.UnitFlags,
		unitFieldBytes1:                   uint32(s.player.StandState) | uint32(s.player.StandFlags)<<16,
		unitFieldPlayerFlags:              s.player.PlayerFlags,
		unitFieldPlayerFieldBytes:         playerFieldBytesValue(*s.player),
		unitFieldTarget:                   uint32(s.selection),
		unitFieldTarget + 1:               uint32(s.selection >> 32),
		unitFieldChannelObject:            uint32(channelGUID),
		unitFieldChannelObject + 1:        uint32(channelGUID >> 32),
		unitFieldChannelSpell:             channelSpell,
		unitFieldGuildID:                  s.player.GuildID,
		unitFieldGuildRank:                uint32(s.player.GuildRank),
		unitFieldBytes2:                   uint32(s.player.SheathState) | uint32(s.player.PVPFlags)<<8,
		unitFieldPlayerBytes2:             uint32(s.player.FacialStyle) | uint32(s.player.BankBagSlots)<<16 | uint32(s.player.RestState)<<24,
		playerFieldRestStateExperience:    uint32(s.player.RestBonus),
		playerFieldBytes2:                 uint32(s.player.AuraVision) << 24,
		unitFieldPlayerBytes3:             uint32(s.player.Gender) | uint32(uint8(s.player.DrunkenState))<<8,
		playerFieldFakeInebriation:        s.player.FakeInebriation,
		unitFieldChosenTitle:              s.player.ChosenTitle,
		unitFieldKnownCurrencies:          uint32(s.player.KnownCurrency),
		unitFieldKnownCurrencies + 1:      uint32(s.player.KnownCurrency >> 32),
		unitFieldSummon:                   uint32(s.player.PetGUID),
		unitFieldSummon + 1:               uint32(s.player.PetGUID >> 32),
		unitFieldSummonedBy:               uint32(s.player.SummonedByGUID),
		unitFieldSummonedBy + 1:           uint32(s.player.SummonedByGUID >> 32),
		playerFieldDuelArbiter:            uint32(s.player.DuelArbiter),
		playerFieldDuelArbiter + 1:        uint32(s.player.DuelArbiter >> 32),
		playerFieldDuelTeam:               s.player.DuelTeam,
		playerFieldFarsight:               uint32(s.player.FarsightGUID),
		playerFieldFarsight + 1:           uint32(s.player.FarsightGUID >> 32),
		unitFieldXP:                       s.player.XP,
		unitFieldNextLevelXP:              playerNextLevelXP(s.player.Level),
		unitFieldCoinage:                  s.player.Money,
		unitFieldMountDisplayID:           s.player.MountDisplayID,
		unitFieldAttackTime:               s.player.AttackTime,
		unitFieldAttackTimeOffhand:        s.player.OffhandAttackTime,
		unitFieldRangedAttackTime:         s.player.RangedAttackTime,
		unitFieldMinDamage:                math.Float32bits(s.player.MinDamage),
		unitFieldMaxDamage:                math.Float32bits(s.player.MaxDamage),
		unitFieldAttackPower:              s.player.AttackPower,
		unitFieldRangedAttackPower:        s.player.RangedAttackPower,
		unitFieldResistances:              s.player.Armor,
		playerFieldKills:                  uint32(s.player.TodayKills) | uint32(s.player.YesterdayKills)<<16,
		playerFieldTodayContribution:      s.player.TodayHonorPoints,
		playerFieldYesterdayContribution:  s.player.YesterdayHonorPoints,
		playerFieldLifetimeHonorableKills: s.player.TotalKills,
		playerFieldHonorCurrency:          s.player.TotalHonorPoints,
		playerFieldArenaCurrency:          s.player.ArenaPoints,
	}
	if s.server.Data != nil {
		if race, found, err := s.server.Data.Race(uint32(s.player.Race)); err == nil && found {
			nativeDisplayID := race.MaleDisplayID
			if s.player.Gender != 0 {
				nativeDisplayID = race.FemaleDisplayID
			}
			displayID := nativeDisplayID
			if s.player.TransformDisplayID != 0 {
				displayID = s.player.TransformDisplayID
			}
			fields[unitFieldDisplayID] = displayID
			fields[unitFieldNativeDisplayID] = nativeDisplayID
		}
	}
	for i := 0; i < 5; i++ {
		fields[unitFieldStat0+i] = s.player.Stats[i]
		fields[unitFieldPosStat0+i] = s.player.Stats[i]
	}
	for i, p := range s.player.Powers {
		fields[unitFieldPower1+i] = p
		// AuraEffect::HandleAuraModIncreaseEnergy / ...Percent
		// (SpellAuraEffects.cpp:3686-3726): max power is aura-modulated
		// live, like max health (aura_health.go).
		fields[unitFieldMaxPower1+i] = s.effectiveMaxPower(i)
	}
	for i := 0; i < 25; i++ {
		fields[playerFieldCombatRating1+i] = s.player.CombatRatings[i]
	}
	for i, sk := range s.player.Skills {
		if i >= playerMaxSkills {
			break
		}
		idx := playerSkillInfoStart + i*3
		fields[idx] = uint32(sk.Skill) | uint32(sk.Step)<<16
		fields[idx+1] = uint32(sk.Value) | uint32(sk.Max)<<16
		temp, talent := s.skillAuraBonus(uint32(sk.Skill))
		fields[idx+2] = skillBonusFieldValue(sk.Bonus, temp, talent)
	}
	equipment := strings.Fields(s.player.Equipment)
	for slot := 0; slot < playerVisibleItemCount; slot++ {
		base := slot * 2
		itemID := uint32(0)
		enchant := uint32(0)
		if base < len(equipment) {
			if id, err := strconv.ParseUint(equipment[base], 10, 32); err == nil {
				itemID = uint32(id)
			}
		}
		if base+1 < len(equipment) {
			if enc, err := strconv.ParseUint(equipment[base+1], 10, 32); err == nil {
				enchant = uint32(enc)
			}
		}
		fields[playerVisibleItemStart+slot*2] = itemID
		fields[playerVisibleItemStart+slot*2+1] = enchant
	}
	s.sendPlayerValuesUpdate(fields)
}

func (s *session) sendPlayerValuesUpdate(fields map[int]uint32) {
	if s == nil || s.server == nil || s.player == nil || len(fields) == 0 {
		return
	}
	packet, err := s.server.buildPlayerValuesUpdateForTarget(s.playerGUID, fields, true)
	if err != nil || packet == nil {
		return
	}
	_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	s.server.broadcastPlayerValuesUpdateFromSession(s, fields)
}

// isBeingTeleported mirrors Player::IsBeingTeleported (Player.h:1748):
// either semaphore set. The near semaphore is Go's nearTeleportPending
// (set by teleportTo, cleared by the move-teleport ack); the far semaphore
// is farTeleportPending (set by the far branch, cleared when the world
// port completes).
func (s *session) isBeingTeleported() bool {
	return s.nearTeleportPending || s.farTeleportPending
}

// teleportTo runs a full near/far teleport. A far teleport detaches the
// player from their transport (Player::TeleportTo, Player.cpp:1711-1716 —
// RemovePassenger unless TELE_TO_NOT_LEAVE_TRANSPORT); use
// teleportToStayOnTransport for the transport's own map-change teleport,
// which keeps passengers attached (Transport.cpp:676).
func (s *session) teleportTo(mapID uint32, x, y, z, orientation float32) bool {
	return s.teleportToStayOnTransport(mapID, x, y, z, orientation, false)
}

func (s *session) teleportToStayOnTransport(mapID uint32, x, y, z, orientation float32, stayOnTransport bool) bool {
	return s.teleportToOptions(mapID, x, y, z, orientation, stayOnTransport, false)
}

// teleportToOptions is the full Player::TeleportTo twin: stayOnTransport
// carries TELE_TO_NOT_LEAVE_TRANSPORT (Transport.cpp:676) and teleToSpell
// carries TELE_TO_SPELL (SpellEffects.cpp:1227-1228) — a teleport spell's own
// far teleport must not interrupt its caster's cast (Player.cpp:1841-1842).
func (s *session) teleportToOptions(mapID uint32, x, y, z, orientation float32, stayOnTransport, teleToSpell bool) bool {
	if s.player == nil {
		return false
	}
	if s.server != nil && !s.validTrinityMapLocation(mapID, x, y, z, orientation) {
		return false
	}
	if s.server != nil {
		entry, found, err := s.server.Data.Map(mapID)
		if err != nil || !found {
			return false
		}
		difficulty := uint32(0)
		if entry.IsDungeon() {
			difficulty = uint32(s.player.DungeonDifficulty)
			if entry.IsRaid() {
				difficulty = uint32(s.player.RaidDifficulty)
			}
			_, difficulty, found, err = s.server.Data.DownscaledMapDifficulty(mapID, difficulty)
			if err != nil || !found {
				return false
			}
		}
		if !s.skipMapDisableCheck(context.Background()) && s.mapDisabledForTeleport(context.Background(), entry, mapID, difficulty) {
			s.sendTransferAborted(mapID, transferAbortMapNotAllowed, 0)
			return false
		}
		if entry.IsBattleground() || entry.IsBattleArena() {
			if s.bgData.InstanceID == 0 {
				return false
			}
		}
		if uint32(s.accountExpansion) < entry.ExpansionID {
			s.sendTransferAborted(mapID, transferAbortExpansionLevel, uint8(entry.ExpansionID))
			return false
		}
	}
	// Player::TeleportTo (Player.cpp:1703-1704): a teleport leaves the
	// vehicle first — after the validity gates above (which return
	// without exiting, as in C++) and before the transport handling
	// below, so a vehicle passenger never arrives with a stale
	// VehicleGUID pointing at a vehicle on the origin map.
	s.exitVehicle(context.Background())
	oldMap := s.player.Map
	sameMap := oldMap == mapID
	if !sameMap && s.server != nil && !s.canStartMapTeleport(context.Background(), mapID) {
		return false
	}
	// Player::TeleportTo far-teleport player cleanup (Player.cpp:1805-1822),
	// in C++ order: ResetContestedPvP, battleground leave, arena cooldown
	// strip — all before the temporary pet unsummon below.
	if !sameMap {
		// Player::ResetContestedPvP (Player.cpp:20761-20766): the
		// contested-PvP flag and timer die on teleport. The
		// ClearUnitState(UNIT_STATE_ATTACK_PLAYER) term has no Go model (no
		// per-unit state field).
		if s.player.PlayerFlags&playerFlagContestedPVP != 0 {
			s.player.PlayerFlags &^= playerFlagContestedPVP
			s.contestedPVPEnd = time.Time{}
			s.sendPlayerUpdate()
		}
		// Player.cpp:1809-1817: a far teleport whose target map differs from
		// the battleground's leaves the BG without teleporting to the entry
		// point (LeaveBattleground(false)). activeBattlegroundMap is the
		// GetBattleground()->GetMapId() analog — the port leg is no-bridge
		// in Go, so the BG's map lives on the active queue slot. The
		// skipEntryTeleport flag avoids the entry-point teleport bouncing
		// the player back instead of landing at the far destination; the
		// deserter-track arm still fires (Player.cpp:22499), and the
		// deserter cast is immediate, not delayed: LeaveBattleground(false)
		// runs inside TeleportTo before SetSemaphoreTeleportFar, so
		// IsBeingTeleportedFar is false there (Player.cpp:1814 vs 22479).
		// The DELAYED_SPELL_CAST_DESERTER schedule is the entry-point
		// leave's arm, not this one's (see handleLeaveBattlefield).
		// Resolve the arena roster BEFORE the leave arm below: the leave
		// funnel deletes the PlayerTeams entry, and C++ gates this arm on
		// the map being left (mEntry), not on surviving BG membership.
		var leavingArena *arenaBattlegroundState
		if s.server != nil {
			leavingArena = s.server.findArenaStateForPlayer(s.playerGUID)
		}
		if bgMap := s.activeBattlegroundMap(); s.bgData.InstanceID != 0 && bgMap != 0 && bgMap != mapID {
			s.handleLeaveBattlefield(context.Background(), nil, false, true)
		}
		// Player.cpp:1819-1822: RemoveArenaSpellCooldowns(true) — teleporting
		// off an arena map resets spell cooldowns (the < 10 min recovery
		// filter, Player.cpp:3942-3955), except for game masters. The
		// RemoveArenaAuras twin stays a standing no-bridge: its aura
		// positivity / spell-attribute filter (Unit.cpp:4329-4343) has no Go
		// model. A sim roster member stands off the arena map (no-bridge
		// port), so the map leg alone misses them: the roster leg is the
		// admitted-player analog of mEntry, destination-guarded so a
		// teleport onto the match map itself does not reset.
		if (IsArenaMap(s.player.Map) || (leavingArena != nil && leavingArena.MapID != mapID)) && !sessionIsGameMaster(s) {
			s.removeArenaSpellCooldowns()
		}
	}
	if !sameMap && s.player.PetGUID != 0 {
		s.temporarilyUnsummonPet(context.Background())
	} else if sameMap && s.server != nil && s.player.PetGUID != 0 {
		visibilityDistance := s.server.Config.VisibilityDistanceContinents
		if visibilityDistance <= 0 {
			visibilityDistance = 150
		}
		s.server.motionMu.Lock()
		pet := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
		petX, petY, petZ := float32(0), float32(0), float32(0)
		if pet != nil {
			petX, petY, petZ = pet.X, pet.Y, pet.Z
		}
		s.server.motionMu.Unlock()
		if pet == nil || distance3D(petX, petY, petZ, x, y, z) > float64(visibilityDistance) {
			s.temporarilyUnsummonPet(context.Background())
		}
	}
	movement := s.movementInfoForCreate(*s.player)
	s.stopPlayerCombat(context.Background())
	// Player::TeleportTo (Player.cpp:1711-1716): a teleport leaves the
	// transport unless TELE_TO_NOT_LEAVE_TRANSPORT — the detach arm sits
	// ahead of the same-map/far branch, so a same-map teleport drops a
	// transport passenger at the destination instead of gluing them to
	// the transport at a recomputed offset. Detaching before the
	// snapshot keeps TRANSFER_PENDING / NEW_WORLD faithful to C++'s
	// post-detach GetTransport() checks, and a stale TransportGUID can no
	// longer keep the old map's transport relocating the player after
	// arrival (broadcastTransportMovement).
	hadTransport := s.player.TransportGUID != 0
	if !stayOnTransport {
		s.player.TransportGUID = 0
		s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
		s.player.TransportSeat = 0
	}
	transportGUID := s.player.TransportGUID
	transportX, transportY, transportZ, transportO := s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO
	transportAttached := false
	var transportSpawn gameObjectSpawn
	if transportGUID != 0 && s.server != nil {
		transportSpawn, transportAttached = s.server.transportSpawnForGUID(transportGUID)
	}
	if sameMap {
		s.nearTeleportPending = true
	} else {
		s.clearLastMovementInfo()
		s.farTeleportOriginOrientation = s.player.Orientation
		s.farTeleportOriginMap = s.player.Map
		s.farTeleportOriginInstanceID = s.player.InstanceID
		s.nearTeleportPending = false
		s.nearTeleportDest = nearTeleportDestination{}
		// Player::TeleportTo far branch (Player.cpp:1801) clears the selection;
		// the near branch holds it, so the clear lives here, not above the branch.
		s.selection = 0
		s.player.Selection = 0
		// Player::RemoveFromWorld (Player.cpp:1976-1979) via Map::RemovePlayerFromMap
		// (Map.cpp:1016): a far teleport releases the active loot while the player is
		// still on the origin map at the origin position, before SMSG_NEW_WORLD — so the
		// creature/gameobject cleanup legs evaluate the origin map and distance (a corpse
		// just looted is adjacent, so they run). Go has no RemoveFromWorld step and
		// overwrites player.Map/coords below, so the release must run here: by
		// completeWorldPort time the map/distance gates would suppress those legs.
		s.releaseActiveLoot()
		// Far-teleport map-leave vehicle eject (Player.cpp:1855-1860,
		// Unit.cpp:9789, Vehicle.cpp:253): the base's kit drops and its
		// passengers exit at the origin position, before the overwrite
		// below moves the base — an ack-time eject would read the
		// destination coords instead. The near branch never reaches
		// here, so near teleports keep passengers seated.
		s.ejectVehiclePassengersAtMapLeave(context.Background())
		s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Orientation = mapID, x, y, z, orientation
	}
	if !sameMap {
		s.lastZoneUpdate = time.Time{}
		s.worldReady.Store(false)
		s.farTeleportPending = true
		s.visiblePlayersMu.Lock()
		s.visiblePlayers = nil
		s.visiblePlayersMu.Unlock()
		s.resetTransportPassengerVisibility()
	}
	s.isFalling = false
	s.isMoving = false

	if sameMap {
		movement.GUID = s.playerGUID
		movement.Flags &= movementPlayerStatusMask
		if s.server != nil {
			movement.Time = s.server.gameTimeMilliseconds()
		}
		movement.X, movement.Y, movement.Z, movement.Orientation = x, y, z, orientation
		movement.FallTime, movement.Jump, movement.HasJump, movement.SplineElevation, movement.HasSpline = 0, [4]float32{}, false, 0, false
		movement.HasPitch = movement.Flags&(movementSwimming|movementFlying) != 0 || movement.Flags2&movement2Pitch != 0
		if transportAttached {
			movement.Flags |= movementOnTransport
			if movement.Transport == nil {
				movement.Transport = &transportMovement{}
			}
			movement.Transport.GUID = transportGUID
			movement.Transport.Seat = s.player.TransportSeat
			movement.Transport.X, movement.Transport.Y, movement.Transport.Z, movement.Transport.Orientation = CalculatePassengerOffset(transportSpawn.X, transportSpawn.Y, transportSpawn.Z, transportSpawn.Orientation, x, y, z, orientation)
		} else {
			movement.Flags &^= movementOnTransport
			movement.Transport = nil
		}
		s.nearTeleportDest = nearTeleportDestination{X: x, Y: y, Z: z, Orientation: orientation, Movement: movement}
		// Player::TeleportTo (Player.cpp:1763-1764): the near-teleport
		// packet is sent only when the session is not logging out; the
		// destination itself is still recorded above for the save.
		// C++ sends nothing to nearby players either, since their
		// broadcast rides the client's ack, which never arrives.
		if !s.loggingOut {
			selfPacket, nearbyPacket := buildTeleportMovementPackets(s.playerGUID, movement)
			_ = s.write(uint16(protocol.OpcodeMSG_MOVE_TELEPORT_ACK), selfPacket, true)
			if s.server != nil {
				s.server.broadcastTeleportMovement(s, nearbyPacket, x, y, z)
			}
		}
	} else {
		// Player::TeleportTo far branch (Player.cpp:1836-1845), in C++ order:
		// RemoveAllDynObjects first, then the spellcast stop — but not when
		// the teleport is the spell's own (TELE_TO_SPELL, Player.cpp:1841-1842:
		// "not attempt interrupt teleportation spell at caster teleport") —
		// then the CHANGE_MAP | MOVE | TURNING aura strip. The interrupt twins
		// no-op when nothing is active, which subsumes C++'s
		// IsNonMeleeSpellCast(true) gate; the IsInterruptable (Executing)
		// gate rides interruptCurrentCast.
		if s.server != nil {
			s.server.despawnAllCasterDynObjects(s.playerGUID)
		}
		if !teleToSpell {
			s.interruptCurrentCast()
			s.interruptCurrentChannel()
			s.cancelAutoRepeatOnInterrupt()
		}
		s.removeAurasWithInterruptFlags(auraInterruptFlagChangeMap | auraInterruptFlagMove | auraInterruptFlagTurning)
		pending := protocol.NewBuffer(12)
		pending.WriteU32(mapID)
		if transportAttached {
			spawn, _ := s.server.transportSpawnForGUID(transportGUID)
			pending.WriteU32(spawn.Entry)
			pending.WriteU32(oldMap)
		}
		// Player::TeleportTo (Player.cpp:1847-1856): the transfer-pending
		// packet is sent only when the session is not logging out.
		if !s.loggingOut {
			_ = s.write(uint16(protocol.OpcodeSMSG_TRANSFER_PENDING), pending.Bytes(), true)
		}
		packet := protocol.NewBuffer(20)
		packet.WriteU32(mapID)
		if transportAttached {
			packet.WriteF32(transportX)
			packet.WriteF32(transportY)
			packet.WriteF32(transportZ)
			packet.WriteF32(transportO)
		} else {
			packet.WriteF32(x)
			packet.WriteF32(y)
			packet.WriteF32(z)
			packet.WriteF32(orientation)
		}
		// Player::TeleportTo (Player.cpp:1875-1878): SMSG_NEW_WORLD and the
		// SendSavedInstances that follows it are sent only when the
		// session is not logging out.
		if !s.loggingOut {
			_ = s.write(uint16(protocol.OpcodeSMSG_NEW_WORLD), packet.Bytes(), true)
			s.sendSavedInstances(context.Background())
		}
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.Exec("UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ?, instance_id = ?, zone = ? WHERE guid = ?",
			mapID, x, y, z, orientation, s.saveTimeInstanceID(), s.player.Zone, s.playerGUID)
		// The initiation write persists the destination; when the
		// teleport detached a transport, the origin transport row must
		// not survive it, or a crash mid-pending would respawn the
		// player back on the transport at stale offsets instead of at
		// the destination. The row is also the anchor a logout
		// double-failure leaves behind (full save and fallback both
		// failed), so it carries the save-time instance and the live
		// zone rather than whatever the last good save wrote: near,
		// the save-time rule value (0/bgData, set above before this
		// write); far, the live instance at initiation — which, for
		// a homebind fallback initiated after an ack assignment, is
		// the assigned instance, so the double-failure row matches
		// the row the full save would have written. Login recomputes
		// zone from map+coords, so the zone column is freshness only.
		if hadTransport && s.player.TransportGUID == 0 {
			_, _ = s.server.CharactersStore.DB.Exec("UPDATE characters SET trans_x = 0, trans_y = 0, trans_z = 0, trans_o = 0, transguid = 0 WHERE guid = ?",
				s.playerGUID)
		}
	}
	if sameMap {
		s.lastFallZ = s.player.Z
	} else {
		s.lastFallZ = z
	}
	s.lastFallTime = 0
	return true
}

// removeArenaSpellCooldowns mirrors Player::RemoveArenaSpellCooldowns(true)
// (Player.cpp:3942-3955): cooldowns on spells whose RecoveryTime and
// CategoryRecoveryTime are both under 10 minutes are reset — each erase sends
// SMSG_CLEAR_COOLDOWN (SpellHistory::ResetCooldowns pred arm, update=true).
// The active pet's cooldowns are reset in full while the pet still exists, so
// this runs before the temporary pet unsummon arm.
func (s *session) removeArenaSpellCooldowns() {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	const arenaCooldownKeepMs = int64(10 * 60 * 1000)
	s.playerStateMu.Lock()
	snapshot := make([]spellCooldown, len(s.player.Cooldowns))
	copy(snapshot, s.player.Cooldowns)
	s.playerStateMu.Unlock()
	cleared := make(map[uint32]struct{})
	for _, cd := range snapshot {
		spell, found, err := s.server.Data.Spell(cd.Spell)
		if err != nil || !found {
			continue
		}
		rec := int64(spell.RecoveryTime)
		if rec < 0 {
			rec = 0
		}
		cat := int64(spell.CategoryRecoveryTime)
		if cat < 0 {
			cat = 0
		}
		// The C++ predicate resets when BOTH recoveries are under 10 min.
		if rec < arenaCooldownKeepMs && cat < arenaCooldownKeepMs {
			cleared[cd.Spell] = struct{}{}
		}
	}
	if len(cleared) == 0 {
		return
	}
	s.playerStateMu.Lock()
	kept := s.player.Cooldowns[:0]
	for _, cd := range s.player.Cooldowns {
		if _, drop := cleared[cd.Spell]; !drop {
			kept = append(kept, cd)
		}
	}
	s.player.Cooldowns = kept
	s.playerStateMu.Unlock()
	for spellID := range cleared {
		_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buildClearCooldown(s.playerGUID, spellID), true)
	}
	if s.server != nil && s.player.PetGUID != 0 {
		s.server.motionMu.Lock()
		if pet := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID); pet != nil {
			pet.SpellCooldowns = make(map[uint32]time.Time)
			pet.SpellCategoryCooldowns = make(map[uint32]time.Time)
			pet.SpellCooldownCategories = make(map[uint32]uint32)
			pet.SpellCooldownCategoryEnds = make(map[uint32]time.Time)
		}
		s.server.motionMu.Unlock()
	}
}

func (s *session) executeCommand(ctx context.Context, line string) bool {
	// C++ TryExecuteCommand strips leading/trailing COMMAND_DELIMITER
	// (' ') from the whole line before tokenizing (ChatCommand.cpp:253-255).
	line = strings.Trim(line, " ")
	s.commandRawLine = line
	fields := strings.Fields(line)
	s.commandFieldCount = len(fields)
	if len(fields) == 0 {
		s.commandRawLine = ""
		s.commandFieldCount = 0
		return false
	}
	// Commands resolve through the prefix-matching command tree
	// (ChatCommandNode::TryExecuteCommand), so '.mod spee 10' dispatches
	// to '.modify speed 10'.
	result := s.dispatchCommand(ctx, fields)
	s.commandRawLine = ""
	s.commandFieldCount = 0
	return result
}

// rawTailAfterTokens mirrors Trinity::Impl::ChatCommands::tokenize
// (ChatCommandHelpers.h:55-70): it skips count space-separated tokens,
// each token running to the next ' ' with the following run of spaces
// skipped, and returns the verbatim remainder. Interior whitespace of
// the remainder is preserved; a line with fewer than count tokens (or
// only spaces after the last) yields "".
func rawTailAfterTokens(line string, count int) string {
	rest := line
	for i := 0; i < count; i++ {
		delim := strings.IndexByte(rest, ' ')
		if delim < 0 {
			return ""
		}
		next := delim
		for next < len(rest) && rest[next] == ' ' {
			next++
		}
		if next >= len(rest) {
			return ""
		}
		rest = rest[next:]
	}
	return rest
}

// commandTail returns the verbatim Tail for a handler's args: the raw
// remainder of the dispatched line starting at args[0]. The dispatcher's
// args are always a suffix of the token stream (resolve canonicalizes
// spellings but never adds or drops tokens), so the tail starts
// len(args) tokens before the end of the line. Without a dispatched
// line (direct calls) it falls back to the joined args.
func (s *session) commandTail(args []string) string {
	return s.commandTailAfter(args, 0)
}

// commandTailAfter returns the verbatim remainder starting at
// args[extra:] — the Tail after a handler consumed extra leading
// tokens of its own args (e.g. the mute reason after player+minutes,
// the npc-whisper text after the receiver name).
func (s *session) commandTailAfter(args []string, extra int) string {
	if s.commandRawLine == "" || s.commandFieldCount == 0 {
		if extra >= len(args) {
			return ""
		}
		return strings.Join(args[extra:], " ")
	}
	skip := s.commandFieldCount - len(args) + extra
	if skip < 0 || skip > s.commandFieldCount {
		if extra >= len(args) {
			return ""
		}
		return strings.Join(args[extra:], " ")
	}
	return rawTailAfterTokens(s.commandRawLine, skip)
}

func (s *session) handleCmdHelp(ctx context.Context, args []string) {
	s.sendCommandHelpFor(ctx, args)
}

// sendCommandHelpFor mirrors Trinity::ChatCommands::SendCommandHelpFor
// (ChatCommand.cpp:322-500): the raw tail is walked token by token
// through the command tree with case-insensitive prefix matching (an
// exact name wins over partial matches, as in resolve). A token with
// no match at the top level is LANG_CMD_INVALID; below a matched
// parent the parent's help prints first, then LANG_SUBCMD_INVALID; a
// token matching several names prints the ambiguity list; a fully
// resolved path prints that node's help. An empty tail lists the
// top-level commands and then the help for "help" itself, as
// HandleHelpCommand does (cs_misc.cpp:695-703). The Go tree carries
// no per-node help text, so every node prints the generic header +
// no-help line; LANG wordings are plain-English recalls (no TDB seed
// in-tree). Prefix matching and listings are filtered to the
// invoker-visible nodes (FilteredCommandListIterator,
// ChatCommand.cpp:207-237), as the R13 census wires perms.
func (s *session) sendCommandHelpFor(ctx context.Context, args []string) {
	tree := s.buildCommandTree()
	visible := s.commandTreeVisible(ctx)
	if len(args) == 0 {
		s.sendTopLevelCommandList(ctx)
		if helpNode := tree.children["help"]; helpNode != nil {
			s.sendCommandHelp(ctx, helpNode)
		}
		return
	}
	node := tree
	var path []string
	for _, token := range args {
		matches := commandNodePrefixMatches(node, token, visible)
		if len(matches) == 0 {
			if len(path) == 0 {
				s.sendSysMessage(fmt.Sprintf("%s is not a valid command.", token))
			} else {
				s.sendCommandHelp(ctx, node)
				s.sendSysMessage(fmt.Sprintf("%s is not a valid subcommand of %s.", token, strings.Join(path, " ")))
			}
			return
		}
		if len(matches) > 1 {
			s.sendSysMessage("There are multiple commands matching '" + strings.ToLower(token) + "'. Did you mean:")
			for _, candidate := range matches {
				s.sendSysMessage(candidate)
			}
			return
		}
		child := node.children[matches[0]]
		if child == nil {
			return
		}
		node = child
		path = append(path, matches[0])
	}
	s.sendCommandHelp(ctx, node)
}

// commandNodePrefixMatches mirrors the FilteredCommandListIterator
// prefix match in SendCommandHelpFor (ChatCommand.cpp:322-500) and the
// matching arm of resolve: invoker-visible children whose name starts
// with the token (case-insensitive), plus a visible alias target, with
// an exact name winning over partial matches. Candidates come back
// sorted, as the ambiguity list prints them. A nil visible predicate
// matches everything.
func commandNodePrefixMatches(node *commandNode, token string, visible func(*commandNode) bool) []string {
	if node == nil {
		return nil
	}
	lower := strings.ToLower(token)
	matches := make([]string, 0, 2)
	for name, child := range node.children {
		if visible != nil && !visible(child) {
			continue
		}
		if strings.HasPrefix(name, lower) {
			matches = append(matches, name)
		}
	}
	if target, ok := node.aliases[lower]; ok && (visible == nil || visible(node.children[target])) {
		duplicate := false
		for _, existing := range matches {
			if existing == target {
				duplicate = true
				break
			}
		}
		if !duplicate {
			matches = append(matches, target)
		}
	}
	for _, candidate := range matches {
		if candidate == lower {
			return []string{candidate}
		}
	}
	sort.Strings(matches)
	return matches
}

// sendCommandHelp mirrors ChatCommandNode::SendCommandHelp
// (ChatCommand.cpp:172-208) for a resolved node: the node's help
// text, or the generic header + no-help line when the tree stores
// none, followed by the visible-subcommand list (ChatCommand.cpp:191:
// an entry is skipped unless it is invoker-visible or has visible
// subcommands, and takes the "..." suffix when it has visible
// subcommands).
func (s *session) sendCommandHelp(ctx context.Context, node *commandNode) {
	if node == nil {
		return
	}
	visible := s.commandTreeVisible(ctx)
	s.sendSysMessage(fmt.Sprintf("Command: %s", node.name))
	s.sendSysMessage(fmt.Sprintf("No help available for command '%s'.", node.name))
	if len(node.children) == 0 {
		return
	}
	s.sendSysMessage("Available subcommands:")
	subs := make([]string, 0, len(node.children))
	for sub, child := range node.children {
		if visible(child) {
			subs = append(subs, sub)
		}
	}
	sort.Strings(subs)
	for _, sub := range subs {
		line := "    " + sub
		if s.hasVisibleSubCommands(node.children[sub], visible) {
			line += " ..."
		}
		s.sendSysMessage(line)
	}
}

// hasVisibleSubCommands mirrors ChatCommandNode::HasVisibleSubCommands
// (ChatCommand.cpp): true when any subcommand of the node is visible.
func (s *session) hasVisibleSubCommands(node *commandNode, visible func(*commandNode) bool) bool {
	if node == nil {
		return false
	}
	for _, child := range node.children {
		if visible(child) {
			return true
		}
	}
	return false
}

// sendTopLevelCommandList renders the empty-tail arm of
// SendCommandHelpFor (ChatCommand.cpp:322-500): LANG_AVAILABLE_CMDS
// plus one line per visible top-level command, in the ".name sub1|sub2"
// form handleCmdCommands established, with only visible subcommands
// listed. Shared by .commands and bare .help.
func (s *session) sendTopLevelCommandList(ctx context.Context) {
	tree := s.buildCommandTree()
	if tree == nil {
		return
	}
	visible := s.commandTreeVisible(ctx)
	names := make([]string, 0, len(tree.children))
	for name, child := range tree.children {
		if visible(child) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	s.sendSysMessage("=== Available Commands ===")
	for _, name := range names {
		node := tree.children[name]
		subs := make([]string, 0, len(node.children))
		for sub, child := range node.children {
			if visible(child) {
				subs = append(subs, sub)
			}
		}
		sort.Strings(subs)
		line := "." + name
		if len(subs) > 0 {
			line += " " + strings.Join(subs, "|")
		}
		s.sendSysMessage(line)
	}
}

// gmVisualAura is the VISUAL_AURA spell id applied while GM-invisible
// (cs_gm.cpp:189, HandleGMVisibleCommand).
const gmVisualAura uint32 = 37800

// handleCmdGM dispatches the ".gm" arms (cs_gm.cpp gm_commandscript,
// FOURTEENTH Commands file): chat, fly, ingame, list, visible, on, off —
// each RBAC-gated per the C++ ChatCommandTable.
func (s *session) handleCmdGM(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// The C++ handlers take Optional<bool> or no args (cs_gm.cpp:73-216)
	// and the dispatcher consumes the whole string (ChatCommand.h:112),
	// so trailing tokens fail the command instead of being dropped.
	switch strings.ToLower(args[0]) {
	case "on", "off", "ingame", "list":
		if len(args) != 1 {
			return false
		}
	case "chat", "fly", "visible", "vis":
		if len(args) > 2 {
			return false
		}
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "on":
		return s.handleCmdGMOn(ctx)
	case "off":
		return s.handleCmdGMOff(ctx)
	case "chat":
		return s.handleCmdGMChat(ctx, args[1:])
	case "fly":
		return s.handleCmdGMFly(ctx, args[1:])
	case "visible", "vis":
		return s.handleCmdGMVisible(ctx, args[1:])
	case "ingame":
		return s.handleCmdGMInGame(ctx)
	case "list":
		return s.handleCmdGMList(ctx)
	default:
		return false
	}
}

// handleCmdGMOn mirrors HandleGMOnCommand (cs_gm.cpp:208).
// Fidelity gap: Player::SetGameMaster(true) also sets FACTION_FRIENDLY,
// UNIT_FLAG2_ALLOW_CHEAT_SPELLS, clears the FFA PvP byte flag, stops combat
// with pets, forces PHASEMASK_ANYWHERE and the serverside GM visibility —
// none of those engine effects have Go bridges; the flag changes are the
// portable core and are stored for real. The GM visibility detect is stored
// too: SetGameMaster(true) sets it to the account security, which is what
// lets the session see serverside-hidden creatures (Object.cpp:2457).
func (s *session) handleCmdGMOn(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandGM) {
		return false
	}
	if s.player != nil {
		s.player.ExtraFlags |= playerExtraGMOn
		s.player.PlayerFlags |= playerFlagGM
		s.gmVisibilityDetect = s.security
		s.updateWorldReadyGM()
		s.persistExtraFlags()
		s.sendPlayerUpdate()
		s.refreshNearbyObjects(ctx)
	}
	// LANG_GM_ON (332).
	s.sendNotification("GM mode is ON.")
	return true
}

// handleCmdGMOff mirrors HandleGMOffCommand (cs_gm.cpp:216).
// Fidelity gap: same SetGameMaster engine-effect gap as the on arm
// (faction/phase/FFA-PvP/pet-faction/combat restore unported).
func (s *session) handleCmdGMOff(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandGM) {
		return false
	}
	if s.player != nil {
		s.player.ExtraFlags &^= playerExtraGMOn
		s.player.PlayerFlags &^= playerFlagGM
		// SetGameMaster(false) restores the GM visibility detect to
		// SEC_PLAYER (Player.cpp:2476); hidden creatures re-hide via the
		// refresh below.
		s.gmVisibilityDetect = 0
		s.updateWorldReadyGM()
		s.persistExtraFlags()
		s.sendPlayerUpdate()
		s.refreshNearbyObjects(ctx)
	}
	// LANG_GM_OFF (333).
	s.sendNotification("GM mode is OFF.")
	return true
}

// handleCmdGMChat mirrors HandleGMChatCommand (cs_gm.cpp:73).
func (s *session) handleCmdGMChat(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandGMChat) {
		return false
	}
	if len(args) == 0 {
		// No argument reports the current badge state; only players holding
		// the staff-badge permission report ON.
		if s.commandAllowed(ctx, permissionChatUseStaffBadge) && s.gmChat {
			// LANG_GM_CHAT_ON (334).
			s.sendNotification("GM chat is ON.")
		} else {
			// LANG_GM_CHAT_OFF (335).
			s.sendNotification("GM chat is OFF.")
		}
		return true
	}
	enable, ok := cheatArgBool(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .gm chat [on|off]")
		return true
	}
	s.gmChat = enable
	if s.player != nil {
		if enable {
			s.player.ExtraFlags |= playerExtraGMChat
		} else {
			s.player.ExtraFlags &^= playerExtraGMChat
		}
		s.persistExtraFlags()
	}
	if enable {
		s.sendNotification("GM chat is ON.")
	} else {
		s.sendNotification("GM chat is OFF.")
	}
	return true
}

// handleCmdGMFly mirrors HandleGMFlyCommand (cs_gm.cpp:96, plain bool arg).
// The C++ dispatcher never runs the handler on a missing arg: bool rides
// the integral ArgInfo (ChatCommandArgs.h:53-56), whose empty token returns
// nullopt, so the dispatcher prints command help instead. A missing arg
// therefore returns false silently (the framework shows the gm group help).
// The C++ broadcasts the SMSG_MOVE_SET/UNSET_CAN_FLY packet via
// SendMessageToSet (cs_gm.cpp:108-115: target plus everyone in range);
// setFlyMode fans the packet out to the nearby sessions as well.
func (s *session) handleCmdGMFly(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandGMFly) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	enable, ok := cheatArgBool(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .gm fly [on|off]")
		return true
	}
	// getSelectedPlayer: own player when nothing is targeted; the C++ falls
	// back to the handler's own player when the selection resolves null.
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil {
			target = ts
		}
	}
	name := ""
	if target.player != nil {
		name = target.player.Name
	}
	target.setFlyMode(enable)
	// LANG_COMMAND_FLYMODE_STATUS (477); GetNameLink has no Go bridge, so the
	// plain name is used (same convention as the character port).
	state := "off"
	if enable {
		state = "on"
	}
	s.sendSysMessage(fmt.Sprintf("Fly mode %s for %s.", name, state))
	return true
}

// handleCmdGMVisible mirrors HandleGMVisibleCommand (cs_gm.cpp:185).
// Fidelity gap: SetGMVisible's SetAcceptWhispers(false) and channel
// SetInvisible have no Go bridges; the aura + flag + visibility-detect
// changes are the portable core.
func (s *session) handleCmdGMVisible(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandGMVisible) {
		return false
	}
	if s.player == nil {
		return true
	}
	if len(args) == 0 {
		// LANG_YOU_ARE with LANG_VISIBLE/LANG_INVISIBLE; isGMVisible() is
		// exactly the PLAYER_EXTRA_GM_INVISIBLE flag test (Player.h:967).
		if s.player.ExtraFlags&playerExtraGMInvisible == 0 {
			s.sendSysMessage("You are Visible.")
		} else {
			s.sendSysMessage("You are Invisible.")
		}
		return true
	}
	visible, ok := cheatArgBool(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .gm visible [on|off]")
		return true
	}
	if visible {
		if s.hasAura(gmVisualAura) {
			s.removeAura(gmVisualAura)
		}
		// Player::SetGMVisible(true): clear the invisible flag and reset the
		// GM visibility detect to SEC_PLAYER (Player.cpp:2506-2509) — GM_ON
		// stays set, but serverside-hidden creatures re-hide from this
		// session on the refresh below (Object.cpp:1622-1630).
		s.player.ExtraFlags &^= playerExtraGMInvisible
		s.gmVisibilityDetect = 0
		s.updateWorldReadyGM()
		s.persistExtraFlags()
		s.sendPlayerUpdate()
		s.refreshNearbyObjects(ctx)
		// LANG_INVISIBLE_VISIBLE (578).
		s.sendNotification("You are now visible.")
		return true
	}
	// Player::SetGMVisible(false): aura + invisible flag + SetGameMaster(true)
	// (SetGameMaster engine effects are the documented gm-on gap); the
	// visibility detect becomes the account security, so hidden creatures
	// become visible to this session.
	s.applyAuraWithDuration(gmVisualAura, 0) // AddAura: permanent
	s.player.ExtraFlags |= playerExtraGMInvisible
	s.player.ExtraFlags |= playerExtraGMOn
	s.player.PlayerFlags |= playerFlagGM
	s.gmVisibilityDetect = s.security
	s.updateWorldReadyGM()
	s.persistExtraFlags()
	s.sendPlayerUpdate()
	s.refreshNearbyObjects(ctx)
	// LANG_INVISIBLE_INVISIBLE (577).
	s.sendNotification("You are now invisible.")
	return true
}

// handleCmdGMInGame mirrors HandleGMListIngameCommand (cs_gm.cpp:129): the
// online-GM list for a session (the C++ console branch is moot — the Go
// command path is always sessioned, same convention as the arena port).
func (s *session) handleCmdGMInGame(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandGMIngame) {
		return false
	}
	if s.server == nil {
		return true
	}
	type gmEntry struct {
		name     string
		security uint8
	}
	var list []gmEntry
	s.server.sessionsMu.RLock()
	for sess := range s.server.sessions {
		if sess == nil || sess.player == nil {
			continue
		}
		psec := sess.security
		isGM := sess.player.ExtraFlags&playerExtraGMOn != 0 || sess.player.PlayerFlags&playerFlagGM != 0
		if !isGM {
			// RBAC_PERM_COMMANDS_APPEAR_IN_GM_LIST plus security at or below
			// the GM.InGMList.Level config (World.cpp:1010, default
			// SEC_ADMINISTRATOR=3).
			if !sess.commandAllowed(ctx, permissionCommandsAppearInGMList) || int(psec) > s.server.Config.GMLevelInGmList {
				continue
			}
		}
		// Player::IsVisibleGloballyFor (Player.cpp:22599): GM-visible units
		// are visible to everyone; invisible GMs show only to GM accounts
		// holding at least the target's security level.
		if sess.player.ExtraFlags&playerExtraGMInvisible != 0 && sess != s {
			if s.security == 0 || psec > s.security {
				continue
			}
		}
		list = append(list, gmEntry{name: sess.player.Name, security: psec})
	}
	s.server.sessionsMu.RUnlock()
	if len(list) == 0 {
		// LANG_GMS_NOT_LOGGED (17).
		s.sendSysMessage("There are no GMs currently logged in.")
		return true
	}
	// LANG_GMS_ON_SRV (16).
	s.sendSysMessage("Currently logged in GMs:")
	s.sendSysMessage("========================")
	for _, e := range list {
		s.sendSysMessage(fmt.Sprintf("|    %s GMLevel %d", e.name, e.security))
	}
	s.sendSysMessage("========================")
	return true
}

// handleCmdGMList mirrors HandleGMListFullCommand (cs_gm.cpp:161): the
// database list of GM accounts (console branch moot — always sessioned).
func (s *session) handleCmdGMList(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandGMList) {
		return false
	}
	if s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil {
		return true
	}
	// LOGIN_SEL_GM_ACCOUNTS (LoginDatabase.cpp:89); SEC_MODERATOR=1.
	rows, err := s.server.AuthStore.DB.QueryContext(ctx,
		"SELECT a.username, aa.SecurityLevel FROM account a, account_access aa WHERE a.id = aa.AccountID AND aa.SecurityLevel >= ? AND (aa.RealmID = -1 OR aa.RealmID = ?)",
		1, s.server.RealmID)
	if err != nil {
		// A failed query prints the empty-list message, as a null result
		// does in C++ (cs_gm.cpp:161-188); there is no silent-false leg.
		s.sendSysMessage("No GMs found.")
		return true
	}
	defer rows.Close()
	type gmAccount struct {
		username string
		security uint32
	}
	var accounts []gmAccount
	for rows.Next() {
		var a gmAccount
		if err := rows.Scan(&a.username, &a.security); err != nil {
			// No C++ counterpart (a valid result set always scans);
			// print the rows collected so far.
			break
		}
		accounts = append(accounts, a)
	}
	if len(accounts) == 0 {
		// LANG_GMLIST_EMPTY (599).
		s.sendSysMessage("No GMs found.")
		return true
	}
	// LANG_GMLIST (597).
	s.sendSysMessage("List of GMs:")
	s.sendSysMessage("========================")
	for _, a := range accounts {
		s.sendSysMessage(fmt.Sprintf("|    %s GMLevel %d", a.username, a.security))
	}
	s.sendSysMessage("========================")
	return true
}

// Cheat flag bits mirroring the CHEAT_* enum (Player.h:822-829).
const (
	cheatGod       uint32 = 0x01
	cheatCasttime  uint32 = 0x02
	cheatCooldown  uint32 = 0x04
	cheatPower     uint32 = 0x08
	cheatWaterwalk uint32 = 0x10
)

// cheatArgBool mirrors Trinity::StringTo<bool> (StringConvert.h:146-166):
// 1/y/on/yes/true and 0/n/off/no/false, case-insensitive.
func cheatArgBool(arg string) (bool, bool) {
	switch strings.ToLower(arg) {
	case "1", "y", "on", "yes", "true":
		return true, true
	case "0", "n", "off", "no", "false":
		return false, true
	}
	return false, false
}

// cheatOnOff reports a cheat flag as the C++ "ON"/"OFF" words used by the
// status command.
func cheatOnOff(enabled bool) string {
	if enabled {
		return "ON"
	}
	return "OFF"
}

// handleCmdDebug mirrors the debug_commandscript table (cs_debug.cpp:99):
// the "debug" root with the areatriggers, anim, worldstate, objectcount,
// dummy, neargraveyard, lootrecipient, instancespawn, los, play, send,
// threat, threatinfo, combat, guidlimits, wpgps, transport, arena, bg,
// questreset, raidreset, spawnvehicle, entervehicle, getvalue, setvalue,
// setbit, Mod32Value and update arms, gated
// on RBAC_PERM_COMMAND_DEBUG (RBAC.h:214, Console::No).
func (s *session) handleCmdDebug(ctx context.Context, args []string) bool {
	if s.player == nil {
		return true
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .debug areatriggers|anim|worldstate|objectcount|dummy|neargraveyard|lootrecipient|instancespawn|los|play|send|threat|threatinfo|combat|guidlimits|wpgps|transport|arena|bg|questreset|raidreset|spawnvehicle|entervehicle|getvalue|setvalue|setbit|Mod32Value|update|getitemvalue|getitemstate|setitemvalue|setaurastate|itemexpire|moveflags")
		return true
	}
	switch strings.ToLower(args[0]) {
	case "areatriggers", "threat", "lootrecipient", "arena", "wpgps", "combat", "los", "dummy", "threatinfo", "bg":
		// No-arg debug arms take no further tokens (ChatCommand.h:112
		// whole-string-consumed); the handlers below used to drop them.
		if len(args) > 1 {
			s.sendSysMessage("Syntax: .debug areatriggers|anim|worldstate|objectcount|dummy|neargraveyard|lootrecipient|instancespawn|los|play|send|threat|threatinfo|combat|guidlimits|wpgps|transport|arena|bg|questreset|raidreset|spawnvehicle|entervehicle|getvalue|setvalue|setbit|Mod32Value|update|getitemvalue|getitemstate|setitemvalue|setaurastate|itemexpire|moveflags")
			return true
		}
		switch strings.ToLower(args[0]) {
		case "areatriggers":
			return s.handleDebugAreaTriggers(ctx)
		case "dummy":
			return s.handleDebugDummy(ctx)
		case "lootrecipient":
			return s.handleDebugLootRecipient(ctx)
		case "los":
			return s.handleDebugLoS(ctx)
		case "threat":
			return s.handleDebugThreat(ctx)
		case "threatinfo":
			return s.handleDebugThreatInfo(ctx)
		case "combat":
			return s.handleDebugCombat(ctx)
		case "wpgps":
			return s.handleDebugWPGPS(ctx)
		case "arena":
			return s.handleDebugArena(ctx)
		case "bg":
			return s.handleDebugBattleground(ctx)
		}
		return true
	case "anim":
		return s.handleDebugAnim(ctx, args[1:])
	case "worldstate":
		return s.handleDebugWorldState(ctx, args[1:])
	case "objectcount":
		return s.handleDebugObjectCount(ctx, args[1:])
	case "neargraveyard":
		return s.handleDebugNearGraveyard(ctx, args[1:])
	case "instancespawn":
		return s.handleDebugInstanceSpawns(ctx, args[1:])
	case "play":
		return s.handleDebugPlay(ctx, args[1:])
	case "send":
		return s.handleDebugSend(ctx, args[1:])
	case "guidlimits":
		return s.handleDebugGuidLimits(ctx, args[1:])
	case "transport":
		return s.handleDebugTransport(ctx, args[1:])
	case "questreset":
		return s.handleDebugQuestReset(ctx, args[1:])
	case "raidreset":
		return s.handleDebugRaidReset(ctx, args[1:])
	case "spawnvehicle":
		return s.handleDebugSpawnVehicle(ctx, args[1:])
	case "entervehicle":
		return s.handleDebugEnterVehicle(ctx, args[1:])
	case "getvalue":
		return s.handleDebugGetValue(ctx, args[1:])
	case "setvalue":
		return s.handleDebugSetValue(ctx, args[1:])
	case "setbit":
		return s.handleDebugSetBit(ctx, args[1:])
	case "mod32value":
		return s.handleDebugMod32Value(ctx, args[1:])
	case "update":
		return s.handleDebugUpdate(ctx, args[1:])
	case "getitemvalue":
		return s.handleDebugGetItemValue(ctx, args[1:])
	case "getitemstate":
		return s.handleDebugGetItemState(ctx, args[1:])
	case "setitemvalue":
		return s.handleDebugSetItemValue(ctx, args[1:])
	case "setaurastate":
		return s.handleDebugSetAuraState(ctx, args[1:])
	case "itemexpire":
		return s.handleDebugItemExpire(ctx, args[1:])
	case "moveflags":
		return s.handleDebugMoveflags(ctx, args[1:])
	default:
		s.sendSysMessage("Syntax: .debug areatriggers|anim|worldstate|objectcount|dummy|neargraveyard|lootrecipient|instancespawn|los|play|send|threat|threatinfo|combat|guidlimits|wpgps|transport|arena|bg|questreset|raidreset|spawnvehicle|entervehicle|getvalue|setvalue|setbit|Mod32Value|update|getitemvalue|getitemstate|setitemvalue|setaurastate|itemexpire|moveflags")
	}
	return true
}

// handleDebugAreaTriggers mirrors HandleDebugAreaTriggersCommand
// (cs_debug.cpp:432): a bare toggle — while set, HandleAreaTriggerOpcode
// reports each reached trigger id (MiscHandler.cpp:675). The trinity_string
// texts (1202/1203) are unseeded in-tree, so plain prose stands in like the
// other unseeded LANG_* notices.
func (s *session) handleDebugAreaTriggers(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandDebug) {
		return false
	}
	s.player.debugAreaTriggers = !s.player.debugAreaTriggers
	if s.player.debugAreaTriggers {
		s.sendSysMessage("Now debug Area Triggers")
	} else {
		s.sendSysMessage("Now no debug Area Triggers")
	}
	return true
}

// handleCmdCheat mirrors cheat_commandscript::GetCommands (cs_cheat.cpp:44):
// the "cheat" root with the god, casttime, cooldown, power, waterwalk,
// status, taxi and explore arms, each gated on its RBAC_PERM_COMMAND_CHEAT_*
// permission (RBAC.h:206-213, all Console::No).
func (s *session) handleCmdCheat(ctx context.Context, args []string) bool {
	if s.player == nil {
		return true
	}
	// The cheat table (cs_cheat.cpp:44-64) has no "" entry: a bare or
	// unknown invocation shows the cheat group help (silent false).
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "god":
		return s.handleCheatGod(ctx, args[1:])
	case "casttime":
		return s.handleCheatCasttime(ctx, args[1:])
	case "cooldown":
		return s.handleCheatCooldown(ctx, args[1:])
	case "power":
		return s.handleCheatPower(ctx, args[1:])
	case "waterwalk":
		return s.handleCheatWaterwalk(ctx, args[1:])
	case "status":
		// HandleCheatStatusCommand takes no arguments; a trailing token
		// fails the whole-string-consumed check (ChatCommand.h:112).
		if len(args) > 1 {
			return false
		}
		return s.handleCheatStatus(ctx)
	case "taxi":
		return s.handleCheatTaxi(ctx, args[1:])
	case "explore":
		return s.handleCheatExplore(ctx, args[1:])
	default:
		return false
	}
}

// cheatToggle mirrors the Optional<bool> toggle arms of cs_cheat.cpp: an
// explicit argument sets the flag, a missing argument flips the current
// state (HandleGodModeCheatCommand et al.). ChatCommand.h:112 — the entire
// string must be consumed, so a trailing token fails the command.
func (s *session) cheatToggle(flag uint32, args []string) (bool, bool) {
	if len(args) > 1 {
		return false, false
	}
	enable := s.player.ActiveCheats&flag == 0
	if len(args) > 0 {
		v, ok := cheatArgBool(args[0])
		if !ok {
			return false, false
		}
		enable = v
	}
	if enable {
		s.player.ActiveCheats |= flag
	} else {
		s.player.ActiveCheats &^= flag
	}
	return enable, true
}

// handleCheatGod mirrors HandleGodModeCheatCommand (cs_cheat.cpp:66). The
// damage-path consumers of CHEAT_GOD are bridged: Unit::DealDamage (Unit.cpp:735)
// via negateGodModeDamage at every player-victim damage site (combat.go melee /
// ranged, spells.go direct / periodic, pet_combat.go melee / spell,
// creaturemotion.go creature melee / spell, and environmentalDamage in
// movement.go — C++ Player::EnvironmentalDamage routes through DealDamage at
// Player.cpp:784, which also covers the HandleFall zeroing at Player.cpp:25390)
// and Spell::EffectInstaKill (SpellEffects.cpp:283) via godCheatActive in
// executeSpellInstantKill.
func (s *session) handleCheatGod(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatGod) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	enable, ok := s.cheatToggle(cheatGod, args)
	if !ok {
		s.sendSysMessage("Syntax: .cheat god [on|off]")
		return true
	}
	if enable {
		s.sendSysMessage("Godmode is ON. You won't take damage.")
	} else {
		s.sendSysMessage("Godmode is OFF. You can take damage.")
	}
	return true
}

// handleCheatCasttime mirrors HandleCasttimeCheatCommand (cs_cheat.cpp:87).
// Fidelity gap: the CHEAT_CASTTIME consumer (Spell.cpp:3127, cast-time
// zeroing) has no Go bridge; the flag is stored and reported only.
func (s *session) handleCheatCasttime(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatCasttime) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	enable, ok := s.cheatToggle(cheatCasttime, args)
	if !ok {
		s.sendSysMessage("Syntax: .cheat casttime [on|off]")
		return true
	}
	if enable {
		s.sendSysMessage("CastTime Cheat is ON. Your spells won't have a casttime.")
	} else {
		s.sendSysMessage("CastTime Cheat is OFF. Your spells will have a casttime.")
	}
	return true
}

// handleCheatCooldown mirrors HandleCoolDownCheatCommand (cs_cheat.cpp:108).
// Fidelity gap: the CHEAT_COOLDOWN consumers (Spell.cpp:3522/8241) have no
// Go bridge (gcd.go notes CHEAT_COOLDOWN infra as unbuilt); the flag is
// stored and reported only.
func (s *session) handleCheatCooldown(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatCooldown) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	enable, ok := s.cheatToggle(cheatCooldown, args)
	if !ok {
		s.sendSysMessage("Syntax: .cheat cooldown [on|off]")
		return true
	}
	if enable {
		s.sendSysMessage("Cooldown Cheat is ON. You are not on the global cooldown.")
	} else {
		s.sendSysMessage("Cooldown Cheat is OFF. You are on the global cooldown.")
	}
	return true
}

// handleCheatPower mirrors HandlePowerCheatCommand (cs_cheat.cpp:129).
// The CHEAT_POWER consumer (Spell.cpp:4792, power-cost skip) is bridged in
// finishSpellCast's TakePower legs (spells.go): the flag skips power/rune
// deduction and the five-second-rule timer there.
func (s *session) handleCheatPower(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatPower) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	enable, ok := s.cheatToggle(cheatPower, args)
	if !ok {
		s.sendSysMessage("Syntax: .cheat power [on|off]")
		return true
	}
	if enable {
		s.sendSysMessage("Power Cheat is ON. You don't need mana/rage/energy to use spells.")
	} else {
		s.sendSysMessage("Power Cheat is OFF. You need mana/rage/energy to use spells.")
	}
	return true
}

// handleCheatWaterwalk mirrors HandleWaterWalkCheatCommand (cs_cheat.cpp:185):
// the CHEAT_WATERWALK bit plus the immediate forced movement (Player::
// SetMovement(MOVE_WATER_WALK / MOVE_LAND_WALK)).
func (s *session) handleCheatWaterwalk(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatWaterwalk) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	enable, ok := s.cheatToggle(cheatWaterwalk, args)
	if !ok {
		s.sendSysMessage("Syntax: .cheat waterwalk [on|off]")
		return true
	}
	if enable {
		s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
		s.sendSysMessage("Waterwalking is ON. You can walk on water.")
	} else {
		s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_LAND_WALK))
		s.sendSysMessage("Waterwalking is OFF. You can't walk on water.")
	}
	return true
}

// handleCheatStatus mirrors HandleCheatStatusCommand (cs_cheat.cpp:150): the
// LANG_COMMAND_CHEAT_STATUS (357) header plus one line per flag (358-362,
// 364). The trinity_string English is inlined; Go has no lang-text helper
// and the strings live in the world DB, not the repo.
func (s *session) handleCheatStatus(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatStatus) {
		return false
	}
	s.sendSysMessage("Cheat status")
	s.sendSysMessage(fmt.Sprintf("Godmode is %s.", cheatOnOff(s.player.ActiveCheats&cheatGod != 0)))
	s.sendSysMessage(fmt.Sprintf("Cooldown cheat is %s.", cheatOnOff(s.player.ActiveCheats&cheatCooldown != 0)))
	s.sendSysMessage(fmt.Sprintf("CastTime cheat is %s.", cheatOnOff(s.player.ActiveCheats&cheatCasttime != 0)))
	s.sendSysMessage(fmt.Sprintf("Power cheat is %s.", cheatOnOff(s.player.ActiveCheats&cheatPower != 0)))
	s.sendSysMessage(fmt.Sprintf("Waterwalk is %s.", cheatOnOff(s.player.ActiveCheats&cheatWaterwalk != 0)))
	s.sendSysMessage(fmt.Sprintf("All taxi nodes enabled is %s.", cheatOnOff(s.isTaxiCheater())))
	return true
}

// handleCheatTaxi mirrors HandleTaxiCheatCommand (cs_cheat.cpp:210): the
// target is the selected online player, else the handler's own player (the
// C++ getSelectedPlayer null falls back to self), guarded by
// HasLowerSecurity; the PLAYER_EXTRA_TAXICHEAT bit rides the existing
// persistExtraFlags path.
func (s *session) handleCheatTaxi(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCheatTaxi) {
		return false
	}
	// Optional<bool> plus the whole-string-consumed check (ChatCommand.h:112):
	// the framework parses the argument before the handler runs, so a
	// trailing token fails before target resolution and a bad token
	// reports the parse message.
	if len(args) > 1 {
		return false
	}
	enable := false
	hasArg := false
	if len(args) > 0 {
		v, ok := cheatArgBool(args[0])
		if !ok {
			s.sendSysMessage("Syntax: .cheat taxi [on|off]")
			return true
		}
		enable, hasArg = v, true
	}
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil {
			target = ts
		}
	}
	// ChatHandler::HasLowerSecurity prints LANG_YOURS_SECURITY_IS_LOW and
	// sets the error flag (Chat.cpp:96-99), so the leg reports (message +
	// true), not a silent false.
	if target != s && s.characterTargetLowerSecurity(ctx, characterTarget{online: target}) {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	if target.player == nil {
		return true
	}
	if !hasArg {
		enable = !target.isTaxiCheater()
	}
	if enable {
		target.player.ExtraFlags |= playerExtraTaxiCheat
	} else {
		target.player.ExtraFlags &^= playerExtraTaxiCheat
	}
	target.persistExtraFlags()
	// ChatHandler::GetNameLink has no Go bridge; the plain name is used.
	if enable {
		s.sendSysMessage(fmt.Sprintf("You give taxis to %s.", target.player.Name))
		if target != s {
			target.sendSysMessage(fmt.Sprintf("%s adds all taxi nodes for you.", s.player.Name))
		}
	} else {
		s.sendSysMessage(fmt.Sprintf("You remove taxis from %s.", target.player.Name))
		if target != s {
			target.sendSysMessage(fmt.Sprintf("%s removes all taxi nodes from you.", s.player.Name))
		}
	}
	return true
}

// handleCheatExplore mirrors HandleExploreCheatCommand (cs_cheat.cpp:238):
// getSelectedPlayer falls back to self with nothing targeted (null ->
// LANG_NO_CHAR_SELECTED only for an offline/non-player target),
// the bool is a required argument (not optional), and the
// PLAYER_EXPLORED_ZONES flags land on the handler's own player exactly like
// the C++ loop (a known upstream quirk: the messages name the target while
// the flags apply to self).
func (s *session) handleCheatExplore(ctx context.Context, args []string) bool {
	// The permission check runs before argument parsing in C++
	// (ChatCommandNode::TryExecuteCommand checks IsInvokerVisible before
	// the invoker parses its args); the sibling arms already check first.
	if !s.commandAllowed(ctx, permissionCommandCheatExplore) {
		return false
	}
	syntax := "Syntax: .cheat explore on|off"
	if len(args) != 1 {
		return false
	}
	reveal, ok := cheatArgBool(args[0])
	if !ok {
		s.sendSysMessage(syntax)
		return true
	}
	// ChatHandler::getSelectedPlayer (Chat.cpp:300-308): the handler's own
	// player when nothing is targeted; null only for an offline/non-player
	// target, and the C++ arm then fails with LANG_NO_CHAR_SELECTED.
	var target *session
	if s.selection != 0 && s.server != nil {
		target = s.server.playerSessionForGUID(s.selection)
	} else if s.selection == 0 {
		target = s
	}
	if target == nil || target.player == nil {
		s.sendSysMessage("No character selected.")
		return true
	}
	explored := uint32(0)
	if reveal {
		explored = ^uint32(0)
	}
	fields := make(map[int]uint32, len(s.player.ExploredZones))
	for index := range s.player.ExploredZones {
		s.player.ExploredZones[index] = explored
		fields[playerExploredZonesStart+index] = explored
	}
	s.persistExploredZones(ctx)
	s.sendPlayerValuesUpdate(fields)
	if reveal {
		s.sendSysMessage(fmt.Sprintf("You give explore-all to %s.", target.player.Name))
		if target != s {
			target.sendSysMessage(fmt.Sprintf("%s gives you explore-all.", s.player.Name))
		}
	} else {
		s.sendSysMessage(fmt.Sprintf("You give explore-nothing to %s.", target.player.Name))
		if target != s {
			target.sendSysMessage(fmt.Sprintf("%s gives you explore-nothing.", s.player.Name))
		}
	}
	return true
}

func (s *session) sendNotification(msg string) {
	if s != nil && s.addonCmdActive {
		// Approximate ChatHandler::HasSentErrorMessage() for the addon
		// console: Go's command layer has no error/info sysmessage split,
		// so an error notification during execution marks the command
		// failed ('f'); plain output sysmessages do not. The notification
		// itself is still delivered normally.
		s.addonCmdFailed = true
	}
	buf := protocol.NewBuffer(len(msg) + 1)
	buf.WriteCString(msg)
	_ = s.write(uint16(protocol.OpcodeSMSG_NOTIFICATION), buf.Bytes(), true)
}

func (s *session) setFlyMode(enable bool) {
	if s.player == nil {
		return
	}
	buf := protocol.NewBuffer(16)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(0) // movement counter
	opcode := uint16(protocol.OpcodeSMSG_MOVE_UNSET_CAN_FLY)
	if enable {
		opcode = uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)
	}
	_ = s.write(opcode, buf.Bytes(), true)
	// SendMessageToSet reaches the target and everyone in range; the
	// direct write above covers the target, so fan the same packet out
	// to the nearby sessions (broadcastToNearby excludes the source).
	if s.server != nil {
		s.server.broadcastToNearby(opcode, buf.Bytes(), s)
	}
}

func (s *session) refreshNearbyObjects(ctx context.Context) {
	if !s.playerLoaded || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	if s.gmVisibilityDetect == 0 {
		// The session cannot see serverside-hidden creatures (plain player,
		// or a GM whose visibility detect was reset to SEC_PLAYER by
		// SetGMVisible(true)/SetGameMaster(false)): destroy any hidden
		// creatures that were previously visible, so e.g. `.gm visible on`
		// re-hides spirit healers and GHOST_VISIBILITY creatures
		// (Object.cpp:1622-1633, Player.cpp:2504-2523).
		s.destroyHiddenCreaturesInRange(ctx, *s.player)
		s.destroyHiddenGameObjectsInRange(ctx, *s.player)
	}
	s.streamNearbyObjects(ctx)
}

func (s *session) destroyHiddenCreaturesInRange(ctx context.Context, state playerState) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	query := `SELECT c.guid, c.id
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND ((c.phaseMask <> 0 AND (c.phaseMask & 1) = 0) OR (COALESCE(t.flags_extra, 0) & 0x400) <> 0 OR (COALESCE(t.npcflag, 0) & 0xC000) <> 0)`
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, query, state.Map, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var low, entry int64
		if rows.Scan(&low, &entry) == nil {
			s.sendDestroyObject(creatureWorldGUID(uint32(low), uint32(entry)), false)
		}
	}
}

func (s *session) destroyHiddenGameObjectsInRange(ctx context.Context, state playerState) {
	if s == nil || s.server == nil {
		return
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	hidden := make(map[uint64]struct{})
	s.server.objectsMu.RLock()
	if state.InstanceID == 0 {
		for guid := range s.server.hiddenGameObjects {
			hidden[guid] = struct{}{}
		}
	}
	instanceKey := instanceAdmissionKey{MapID: state.Map, InstanceID: state.InstanceID}
	for guid := range s.server.instanceHiddenGameObjects[instanceKey] {
		hidden[guid] = struct{}{}
	}
	for guid, dyn := range s.server.dynamicGameObjects {
		if dyn != nil && dyn.Map == state.Map && dyn.InstanceID == state.InstanceID && math.Hypot(float64(dyn.X-state.X), float64(dyn.Y-state.Y)) <= distance {
			if _, ok := hidden[guid]; ok {
				s.sendDestroyObject(guid, false)
				delete(hidden, guid)
			}
		}
	}
	for guid, dyn := range s.server.instanceGameObjects[instanceKey] {
		if dyn != nil && math.Hypot(float64(dyn.X-state.X), float64(dyn.Y-state.Y)) <= distance {
			if _, ok := hidden[guid]; ok {
				s.sendDestroyObject(guid, false)
				delete(hidden, guid)
			}
		}
	}
	s.server.objectsMu.RUnlock()
	if len(hidden) == 0 || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT guid, id, position_x, position_y FROM gameobject WHERE map = ? AND position_x BETWEEN ? AND ? AND position_y BETWEEN ? AND ?`, state.Map, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var low, entry int64
		var x, y float64
		if rows.Scan(&low, &entry, &x, &y) != nil {
			continue
		}
		guid := gameObjectGUID(uint32(low), uint32(entry))
		if _, ok := hidden[guid]; ok {
			s.sendDestroyObject(guid, false)
			delete(hidden, guid)
		}
	}
}

func (s *session) streamNearbyObjects(ctx context.Context) {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return
	}
	s.streamNearbyObjectsAt(ctx, s.player.X, s.player.Y, s.player.Z)
}

// streamNearbyObjectsAt streams the neighbourhood around the given coords
// instead of the player's own: a possessing player's viewpoint is the
// possessed creature (Player.cpp:24261-24269), so its stream centers on the
// creature. The viewpoint copy keeps the observer's GUID, map, phase and
// flags; only the coords move, so the faction leg (Race-only) is unaffected.
func (s *session) streamNearbyObjectsAt(ctx context.Context, x, y, z float32) {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return
	}
	s.lastStreamX = x
	s.lastStreamY = y
	s.lastStreamZ = z
	viewpoint := *s.player
	viewpoint.X, viewpoint.Y, viewpoint.Z = x, y, z
	if packet, count, err := s.server.buildNearbyCreatureUpdates(ctx, viewpoint, s.currentPlayerPhaseMask(), s); err == nil && count > 0 && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
	if packet, _, created := s.server.buildNearbyPlayerUpdatesWithCreatedFrom(s, viewpoint); packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		s.sendVisiblePlayerAuras(created)
		s.sendVisibleCreatureAuras(viewpoint)
	}
	if packet, count, err := s.server.buildNearbyGameObjectUpdates(ctx, viewpoint, false, s); err == nil && count > 0 && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
	s.streamDynamicSpellObjectsAt(x, y)
}

// streamFarsightNeighborhood streams the neighbourhood around a viewpoint
// target after PLAYER_FARSIGHT moves onto it (Player::SetViewpoint apply arm,
// Player.cpp:24618-24636): C++ re-centers the next UpdateVisibilityForPlayer
// tick on m_seer (Player.cpp:22794-22799, Cell::VisitAllObjects(m_seer, ...)),
// so viewpoint-near objects arrive as creates. Go has no visibility tick —
// streams fire only on movement/teleport/zone events — so the set site must
// stream explicitly, and the clear site must re-stream the body neighbourhood.
// Resolves a creature motion or an online player on the caster's map and
// instance; an unresolvable or cross-map target streams nothing (fail open:
// the field update already went out, and the seer-gated fan-outs still deliver
// viewpoint-near spawns and updates). The stream is additive like every other
// Go stream — no range-out destroys, matching the walk-away path.
func (s *session) streamFarsightNeighborhood(ctx context.Context, targetGUID uint64) {
	if !s.playerLoaded || s.player == nil || s.server == nil {
		return
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	if m := s.findCreatureMotion(targetGUID); m != nil {
		s.streamNearbyObjectsAt(ctx, m.X, m.Y, m.Z)
		return
	}
	if target := s.server.findSessionByGUID(targetGUID); target != nil && target.player != nil && target.player.Map == s.player.Map && target.player.InstanceID == s.player.InstanceID {
		s.streamNearbyObjectsAt(ctx, target.player.X, target.player.Y, target.player.Z)
	}
}

func (s *session) sendDestroyObject(guid uint64, onDeath bool) {
	buf := protocol.NewBuffer(9)
	buf.WriteU64(guid)
	if onDeath {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), buf.Bytes(), true)
}

// goGridCenterID and goSizeOfGrids mirror GridDefines.h (CENTER_GRID_ID =
// MAX_NUMBER_OF_GRIDS/2 = 32, SIZE_OF_GRIDS = 533.3333f), used by
// HandleGoGridCommand (cs_go.cpp:205).
const (
	goGridCenterID = 32
	goSizeOfGrids  = 533.3333
)

// goDoTeleport mirrors go_commandscript::DoTeleport (cs_go.cpp:73): reject
// invalid coordinates (LANG_INVALID_TARGET_COORD = 263), stop an active taxi
// flight, then teleport. teleportTo is the Go Player::TeleportTo bridge.
// DELTA: DoTeleport also rejects transport maps (IsTransportMap); that leg
// is not enforced here because goDoTeleport is shared with appear/summon,
// whose C++ paths do not reject them. The go arms that route through
// DoTeleport use goDoTeleportChecked below instead.
// The non-flight case snapshots the recall position (SaveRecallPosition,
// Player.h:1989); the flight case finishes the taxi instead.
func (s *session) goDoTeleport(ctx context.Context, mapID uint32, x, y, z, o float32) bool {
	if s.player == nil {
		return false
	}
	if s.server == nil || !s.validTrinityMapLocation(mapID, x, y, z, o) {
		s.sendSysMessage(fmt.Sprintf("You cannot teleport to given coordinates (%.2f, %.2f, map %d).", x, y, mapID))
		return false
	}
	if s.isInFlight() {
		s.finishTaxiFlight()
	} else {
		s.saveRecallPosition()
	}
	return s.teleportTo(mapID, x, y, z, o)
}

// goDoTeleportChecked is the go-arm DoTeleport twin (cs_go.cpp:73-91): it
// adds the transport-map rejection (IsTransportMap, ObjectMgr.h:1588) that
// goDoTeleport omits for its shared callers. Only the arms whose C++
// handler routes through DoTeleport use it (creature, creature id,
// gameobject, gameobject id, taxinode, areatrigger, xyz, offset);
// graveyard/grid/zonexy/ticket call TeleportTo directly in C++
// (cs_go.cpp:198,228,307,349) and stay on goDoTeleport.
func (s *session) goDoTeleportChecked(ctx context.Context, mapID uint32, x, y, z, o float32) bool {
	if s.isTransportMap(ctx, mapID) {
		s.sendSysMessage(fmt.Sprintf("You cannot teleport to given coordinates (%.2f, %.2f, map %d).", x, y, mapID))
		return false
	}
	return s.goDoTeleport(ctx, mapID, x, y, z, o)
}

// isTransportMap mirrors ObjectMgr::IsTransportMap: a map is a transport
// map when a MO_TRANSPORT (type 15) gameobject_template row carries it in
// data0 (ObjectMgr.cpp:7638).
func (s *session) isTransportMap(ctx context.Context, mapID uint32) bool {
	db := s.goWorldDB()
	if db == nil {
		return false
	}
	var found int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM gameobject_template WHERE type = 15 AND data0 = ? LIMIT 1", mapID).Scan(&found)
	return err == nil && found == 1
}

// handleCmdGo dispatches the ".go" arms (cs_go.cpp go_commandscript,
// FIFTEENTH Commands file): creature, creature id, gameobject, gameobject id,
// graveyard, grid, taxinode, areatrigger, zonexy, xyz, ticket, offset,
// instance, boss — each gated on RBAC_PERM_COMMAND_GO (377) per the C++
// ChatCommandTable. All 14 arms are ported natively; zonexy rides the new
// WorldMapArea store plus the AreaTable parent leg (see handleCmdGoZoneXY).
//
// The bool reports C++ handler success: RBAC denial returns false silently
// so dispatchCommand shows the node's help listing (ChatCommand.cpp:296-309);
// framework parse failures return false silently; paths that print a message
// return true.
func (s *session) handleCmdGo(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .go creature <spawnId>|creature id <entry>|gameobject <spawnId>|gameobject id <entry>|graveyard <gyId>|grid <x> <y> [map]|taxinode <nodeId>|areatrigger <id>|zonexy <x> <y> [area]|xyz <x> <y> [z] [map] [o]|ticket <id>|offset <dx> [dy] [dz] [do]|instance <label...>|boss <name...>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	// The GO permission lives on the leaf arms (cs_go.cpp:48-61), so it is
	// checked only once a subcommand matches; an unknown subcommand prints
	// usage even for a session without the permission.
	switch strings.ToLower(args[0]) {
	case "creature", "gameobject", "graveyard", "grid", "taxinode", "areatrigger", "zonexy", "xyz", "ticket", "offset", "instance", "boss":
	default:
		s.sendSysMessage(syntax)
		return true
	}
	if !s.commandAllowed(ctx, permissionCommandGO) {
		return false
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "creature":
		return s.handleCmdGoCreature(ctx, args[1:])
	case "gameobject":
		return s.handleCmdGoGameObject(ctx, args[1:])
	case "graveyard":
		return s.handleCmdGoGraveyard(ctx, args[1:])
	case "grid":
		return s.handleCmdGoGrid(ctx, args[1:])
	case "taxinode":
		return s.handleCmdGoTaxiNode(ctx, args[1:])
	case "areatrigger":
		return s.handleCmdGoAreaTrigger(ctx, args[1:])
	case "zonexy":
		return s.handleCmdGoZoneXY(ctx, args[1:])
	case "xyz":
		return s.handleCmdGoXYZ(ctx, args[1:])
	case "ticket":
		return s.handleCmdGoTicket(ctx, args[1:])
	case "offset":
		return s.handleCmdGoOffset(ctx, args[1:])
	case "instance":
		return s.handleCmdGoInstance(ctx, args[1:])
	case "boss":
		return s.handleCmdGoBoss(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
		return true
	}
}

// goWorldDB returns the world database or nil, following the handleCmdTele pattern.
func (s *session) goWorldDB() *sql.DB {
	if s.server == nil || s.server.WorldStore == nil {
		return nil
	}
	return s.server.WorldStore.DB
}

// handleCmdGoCreature mirrors HandleGoCreatureSpawnIdCommand (cs_go.cpp:96)
// and HandleGoCreatureCIdCommand (cs_go.cpp:109), teleporting to the spawn
// point of the creature with the given spawn id or, via "id", the spawn of
// the given creature entry. C++ teleports to the first spawn even when several
// entries match (LANG_COMMAND_GOCREATMULTIPLE is a warning, not an abort).
// DoTeleport carries the spawn point whole, orientation included, so both
// forms select the row orientation instead of keeping the player's. The
// args accept their Variant<Hyperlink<...>, ...> link forms
// (|Hcreature:| spawn guid, |Hcreature_entry:| entry).
func (s *session) handleCmdGoCreature(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// The whole string must be consumed (ChatCommand.h:112): the spawn
	// form takes exactly one token, the "id" form exactly two.
	if goIDForm(args) {
		if len(args) != 2 {
			return false
		}
	} else if len(args) != 1 {
		return false
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if goIDForm(args) {
		entry, ok := goParseLinkOrUint(args[1], "creature_entry")
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid creature entry.")
			return true
		}
		rows, err := db.QueryContext(ctx, "SELECT guid, map, position_x, position_y, position_z, orientation FROM creature WHERE id = ?", entry)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Creature lookup error: %v", err))
			return true
		}
		defer rows.Close()
		type spawn struct {
			guid       uint32
			mapID      uint32
			x, y, z, o float32
		}
		var spawns []spawn
		for rows.Next() {
			var sp spawn
			if err := rows.Scan(&sp.guid, &sp.mapID, &sp.x, &sp.y, &sp.z, &sp.o); err != nil {
				continue
			}
			spawns = append(spawns, sp)
		}
		if len(spawns) == 0 {
			s.sendSysMessage("Could not find creature.") // LANG_COMMAND_GOCREATNOTFOUND (268)
			return true
		}
		if len(spawns) > 1 {
			s.sendSysMessage("More than one creature found with the given entry.") // LANG_COMMAND_GOCREATMULTIPLE (269)
		}
		first := spawns[0]
		s.goDoTeleportChecked(ctx, first.mapID, first.x, first.y, first.z, first.o)
		return true
	}
	spawnID, ok := goParseLinkOrUint(args[0], "creature")
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid creature spawn ID.")
		return true
	}
	var mapID uint32
	var x, y, z, o float32
	err := db.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z, orientation FROM creature WHERE guid = ?", spawnID).Scan(&mapID, &x, &y, &z, &o)
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage("Could not find creature.") // LANG_COMMAND_GOCREATNOTFOUND (268)
		return true
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Creature lookup error: %v", err))
		return true
	}
	s.goDoTeleportChecked(ctx, mapID, x, y, z, o)
	return true
}

// handleCmdGoGameObject mirrors HandleGoGameObjectSpawnIdCommand (cs_go.cpp:136)
// and HandleGoGameObjectGOIdCommand (cs_go.cpp:149): same shape as the creature
// arms, against the gameobject table (LANG_COMMAND_GOOBJNOTFOUND = 267).
// The spawn orientation rides along, as in the creature arms; the C++ arms
// take plain uint32 args, so no link forms here.
func (s *session) handleCmdGoGameObject(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// The whole string must be consumed (ChatCommand.h:112): the spawn
	// form takes exactly one token, the "id" form exactly two.
	if goIDForm(args) {
		if len(args) != 2 {
			return false
		}
	} else if len(args) != 1 {
		return false
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	if goIDForm(args) {
		entry, ok := parseCommandUint32(args[1])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid gameobject entry.")
			return true
		}
		rows, err := db.QueryContext(ctx, "SELECT guid, map, position_x, position_y, position_z, orientation FROM gameobject WHERE id = ?", uint32(entry))
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
			return true
		}
		defer rows.Close()
		type spawn struct {
			guid       uint32
			mapID      uint32
			x, y, z, o float32
		}
		var spawns []spawn
		for rows.Next() {
			var sp spawn
			if err := rows.Scan(&sp.guid, &sp.mapID, &sp.x, &sp.y, &sp.z, &sp.o); err != nil {
				continue
			}
			spawns = append(spawns, sp)
		}
		if len(spawns) == 0 {
			s.sendSysMessage("Could not find gameobject.") // LANG_COMMAND_GOOBJNOTFOUND (267)
			return true
		}
		if len(spawns) > 1 {
			s.sendSysMessage("More than one gameobject found with the given entry.") // LANG_COMMAND_GOCREATMULTIPLE (269)
		}
		first := spawns[0]
		s.goDoTeleportChecked(ctx, first.mapID, first.x, first.y, first.z, first.o)
		return true
	}
	spawnID, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid gameobject spawn ID.")
		return true
	}
	var mapID uint32
	var x, y, z, o float32
	err := db.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z, orientation FROM gameobject WHERE guid = ?", uint32(spawnID)).Scan(&mapID, &x, &y, &z, &o)
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage("Could not find gameobject.") // LANG_COMMAND_GOOBJNOTFOUND (267)
		return true
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Gameobject lookup error: %v", err))
		return true
	}
	s.goDoTeleportChecked(ctx, mapID, x, y, z, o)
	return true
}

// handleCmdGoGraveyard mirrors HandleGoGraveyardCommand (cs_go.cpp:183):
// teleport to the WorldSafeLocs.dbc entry with the given id
// (LANG_COMMAND_GRAVEYARDNOEXIST = 449), keeping the player's orientation.
// The recall snapshot rides goDoTeleport (see above).
func (s *session) handleCmdGoGraveyard(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	gyID, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid graveyard ID.")
		return true
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("DBC data not available.")
		return true
	}
	gy, found, err := s.server.Data.WorldSafeLoc(uint32(gyID))
	if err != nil || !found {
		s.sendSysMessage(fmt.Sprintf("Graveyard %d does not exist.", gyID))
		return true
	}
	s.goDoTeleport(ctx, gy.MapID, gy.X, gy.Y, gy.Z, s.player.Orientation)
	return true
}

// handleCmdGoGrid mirrors HandleGoGridCommand (cs_go.cpp:205): teleport to the
// center of the given grid cell on the player's map (or the given map).
// cs_go.cpp:228 resolves Z as max(GetHeight(x, y, MAX_HEIGHT),
// GetWaterLevel(x, y)); from MAX_HEIGHT the vmap leg provably never hits, so
// mapHeight is exactly the GetHeight half, and mapWaterLevel is the
// GetWaterLevel half (cs_go.cpp:228), so a water cell now reads the
// surface; a missing grid keeps the player's Z.
func (s *session) handleCmdGoGrid(ctx context.Context, args []string) bool {
	if len(args) < 2 || len(args) > 3 {
		return false
	}
	gridX, ok1 := parseCommandFloat(args[0])
	gridY, ok2 := parseCommandFloat(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid grid coordinates.")
		return true
	}
	mapID := s.player.Map
	if len(args) > 2 {
		m, ok := parseCommandUint32(args[2])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid map ID.")
			return true
		}
		mapID = uint32(m)
	}
	x := float32((gridX - goGridCenterID + 0.5) * goSizeOfGrids)
	y := float32((gridY - goGridCenterID + 0.5) * goSizeOfGrids)
	z := s.player.Z
	if height, ok := s.server.mapHeight(mapID, x, y, terrainMaxHeight, true, terrainDefaultHeightSearch); ok {
		z = height
	}
	if water, ok := s.server.mapWaterLevel(mapID, x, y); ok && water > z {
		z = water
	}
	s.goDoTeleport(ctx, mapID, x, y, z, s.player.Orientation)
	return true
}

// handleCmdGoTaxiNode mirrors HandleGoTaxinodeCommand (cs_go.cpp:234):
// teleport to the position of the given TaxiNodes.dbc node
// (LANG_COMMAND_GOTAXINODENOTFOUND = 347).
func (s *session) handleCmdGoTaxiNode(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	nodeID, ok := goParseLinkOrUint(args[0], "taxinode")
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid taxinode.")
		return true
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("DBC data not available.")
		return true
	}
	node, found, err := s.server.Data.TaxiNode(nodeID)
	if err != nil || !found {
		s.sendSysMessage(fmt.Sprintf("Could not find taxinode %d.", nodeID))
		return true
	}
	s.goDoTeleportChecked(ctx, uint32(node.ContinentID), node.X, node.Y, node.Z, 0)
	return true
}

// handleCmdGoAreaTrigger mirrors HandleGoAreaTriggerCommand (cs_go.cpp:246):
// teleport to the position of the given AreaTrigger.dbc entry
// (LANG_COMMAND_GOAREATRNOTFOUND = 262).
func (s *session) handleCmdGoAreaTrigger(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	triggerID, ok := goParseLinkOrUint(args[0], "areatrigger")
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid areatrigger.")
		return true
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("DBC data not available.")
		return true
	}
	at, found, err := s.server.Data.AreaTrigger(triggerID)
	if err != nil || !found {
		s.sendSysMessage(fmt.Sprintf("Could not find areatrigger %d.", triggerID))
		return true
	}
	s.goDoTeleportChecked(ctx, at.ContinentID, at.X, at.Y, at.Z, 0)
	return true
}

// goIDForm reports whether args select the nested "creature id" /
// "gameobject id" C++ row (cs_go.cpp:50,52): the dispatcher prefix-matches
// the nested name, so any unique prefix of "id" ("i", "id") selects it.
func goIDForm(args []string) bool {
	return len(args) > 1 && len(args[0]) > 0 && strings.HasPrefix("id", strings.ToLower(args[0]))
}

// goParseLinkOrUint accepts a C++ Variant<Hyperlink<T>, uint32> argument: a
// |H<tag>:<id>| link (bare or full colored form, via gpsLinkTagData) or a
// plain number. It reports false when neither form parses; a link of the
// wrong type fails the same way C++ fails the typed parse.
func goParseLinkOrUint(arg, tag string) (uint32, bool) {
	if linkTag, data := gpsLinkTagData(arg); linkTag == tag {
		if id, ok := parseCommandUint32(data); ok {
			return id, true
		}
		return 0, false
	}
	if id, ok := parseCommandUint32(arg); ok {
		return id, true
	}
	return 0, false
}

// handleCmdGoZoneXY mirrors HandleGoZoneXYCommand (cs_go.cpp:281): teleport
// to zone-percentage coordinates on the zone's continent, converting them
// with Zone2MapCoordinates (DBCStores.cpp:776) and resolving Z as
// max(GetHeight, GetWaterLevel) like the grid arm. The area defaults to the
// player's zone (Player::GetZoneId); the parent-area leg follows
// AreaTableEntry::ParentAreaID (a missing parent row keeps the area itself
// instead of the C++ ASSERT), and the instanceable leg uses
// MapEntry::Instanceable (DBCStructure.h:1100: InstanceType != MAP_COMMON).
// A WorldMapArea row missing for the zone leaves the coordinates unconverted
// (the C++ not-listed fallback); a missing WorldMapArea.dbc file does the
// same. A missing grid keeps the player's Z, as in the grid/xyz arms.
func (s *session) handleCmdGoZoneXY(ctx context.Context, args []string) bool {
	if len(args) < 2 || len(args) > 3 {
		return false
	}
	x, ok1 := parseCommandFloat(args[0])
	y, ok2 := parseCommandFloat(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid zone coordinates.")
		return true
	}
	areaID := s.player.Zone
	if len(args) > 2 {
		id, ok := goParseLinkOrUint(args[2], "area")
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid area.")
			return true
		}
		areaID = id
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("DBC data not available.")
		return true
	}
	areaEntry, found, err := s.server.Data.Area(areaID)
	if err != nil || !found || x < 0 || x > 100 || y < 0 || y > 100 {
		s.sendSysMessage(fmt.Sprintf("Invalid zone coordinates (%.2f, %.2f, area %d).", x, y, areaID))
		return true
	}
	zoneEntry := areaEntry
	if areaEntry.ParentAreaID != 0 {
		if parent, found, err := s.server.Data.Area(areaEntry.ParentAreaID); err == nil && found {
			zoneEntry = parent
		}
	}
	mapEntry, found, err := s.server.Data.Map(zoneEntry.ContinentID)
	if err != nil || !found {
		s.sendSysMessage(fmt.Sprintf("You cannot teleport to given coordinates (%.2f, %.2f, map %d).", x, y, zoneEntry.ContinentID))
		return true
	}
	if mapEntry.InstanceType != 0 {
		s.sendSysMessage(fmt.Sprintf("Invalid zone map: area %d (%s) is on instance map %d (%s).", areaEntry.ID, areaEntry.Name, mapEntry.ID, mapEntry.MapName))
		return true
	}
	fx, fy := float32(x), float32(y)
	if wma, found, err := s.server.Data.WorldMapArea(zoneEntry.ID); err == nil && found {
		fx, fy = zoneToMapCoordinates(fx, fy, wma.LocLeft, wma.LocRight, wma.LocTop, wma.LocBottom)
	}
	z := s.player.Z
	if height, ok := s.server.mapHeight(zoneEntry.ContinentID, fx, fy, terrainMaxHeight, true, terrainDefaultHeightSearch); ok {
		z = height
	}
	if water, ok := s.server.mapWaterLevel(zoneEntry.ContinentID, fx, fy); ok && water > z {
		z = water
	}
	s.goDoTeleport(ctx, zoneEntry.ContinentID, fx, fy, z, s.player.Orientation)
	return true
}

// zoneToMapCoordinates mirrors Zone2MapCoordinates (DBCStores.cpp:776):
// client map coordinates are swapped, then scaled into the WorldMapArea
// bounds (x from the top/bottom span, y from the left/right span).
func zoneToMapCoordinates(x, y, locLeft, locRight, locTop, locBottom float32) (float32, float32) {
	x, y = y, x
	x = x*(locBottom-locTop)/100 + locTop
	y = y*(locRight-locLeft)/100 + locLeft
	return x, y
}

// handleCmdGoXYZ mirrors HandleGoXYZCommand (cs_go.cpp:309): teleport to the
// given coordinates on the player's map (or the given map), keeping the given
// orientation or 0. With Z omitted, the height is max(GetHeight,
// GetWaterLevel) of cs_go.cpp:331 via mapHeight + mapWaterLevel; a
// missing grid keeps the player's Z.
func (s *session) handleCmdGoXYZ(ctx context.Context, args []string) bool {
	if len(args) < 2 || len(args) > 5 {
		return false
	}
	x, ok1 := parseCommandFloat(args[0])
	y, ok2 := parseCommandFloat(args[1])
	if !ok1 || !ok2 {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid coordinates.")
		return true
	}
	z := float64(s.player.Z)
	zGiven := false
	if len(args) > 2 {
		zv, ok := parseCommandFloat(args[2])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid height.")
			return true
		}
		z = zv
		zGiven = true
	}
	mapID := s.player.Map
	if len(args) > 3 {
		m, ok := parseCommandUint32(args[3])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid map ID.")
			return true
		}
		mapID = uint32(m)
	}
	var o float64
	if len(args) > 4 {
		ov, ok := parseCommandFloat(args[4])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid orientation.")
			return true
		}
		o = ov
	}
	// cs_go.cpp:331: with Z omitted C++ uses max(GetHeight(x, y, MAX_HEIGHT),
	// GetWaterLevel(x, y)); mapHeight is exactly the GetHeight half from
	// MAX_HEIGHT and mapWaterLevel the water half, and a missing grid
	// keeps the player's Z.
	if !zGiven {
		if height, ok := s.server.mapHeight(mapID, float32(x), float32(y), terrainMaxHeight, true, terrainDefaultHeightSearch); ok {
			z = float64(height)
		}
		if water, ok := s.server.mapWaterLevel(mapID, float32(x), float32(y)); ok && water > float32(z) {
			z = float64(water)
		}
	}
	s.goDoTeleportChecked(ctx, mapID, float32(x), float32(y), float32(z), float32(o))
	return true
}

// handleCmdGoTicket mirrors HandleGoTicketCommand (cs_go.cpp:337): teleport
// to the position recorded on the given GM ticket (GmTicket::TeleportTo,
// TicketMgr.cpp:251, uses orientation 0). LANG_COMMAND_TICKETNOTEXIST = 2005
// sends without an error flag, so the arm reports and returns.
func (s *session) handleCmdGoTicket(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	ticketID, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
		s.sendSysMessage("Invalid ticket ID.")
		return true
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	var mapID uint32
	var x, y, z float32
	err := db.QueryRowContext(ctx, "SELECT mapId, posX, posY, posZ FROM gm_ticket WHERE id = ?", uint32(ticketID)).Scan(&mapID, &x, &y, &z)
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage("Ticket does not exist.")
		return true
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Ticket lookup error: %v", err))
		return true
	}
	s.goDoTeleport(ctx, mapID, x, y, z, 0)
	return true
}

// handleCmdGoOffset mirrors HandleGoOffsetCommand (cs_go.cpp:358): shift the
// player's position by the given offset and teleport there. Position::
// RelocateOffset (Position.cpp:36) applies the XY offset rotated by the
// player's orientation; Z and orientation add linearly.
func (s *session) handleCmdGoOffset(ctx context.Context, args []string) bool {
	if len(args) < 1 || len(args) > 4 {
		return false
	}
	vals := make([]float64, 4)
	for i := 0; i < len(args) && i < 4; i++ {
		v, ok := parseCommandFloat(args[i])
		if !ok {
			// Unparseable typed token: LANG_CMDPARSER_* + SetSentErrorMessage.
			s.sendSysMessage("Invalid offset.")
			return true
		}
		vals[i] = v
	}
	o := float64(s.player.Orientation)
	dx, dy, dz, doff := vals[0], vals[1], vals[2], vals[3]
	nx := s.player.X + float32(dx*math.Cos(o)+dy*math.Sin(o+math.Pi))
	ny := s.player.Y + float32(dy*math.Cos(o)+dx*math.Sin(o))
	nz := s.player.Z + float32(dz)
	s.goDoTeleportChecked(ctx, s.player.Map, nx, ny, nz, normalizeOrientation(float32(o+doff)))
	return true
}

// instanceGoBackTrigger mirrors ObjectMgr::GetGoBackTrigger (ObjectMgr.cpp:7246):
// the areatrigger_teleport row whose target is the instance's entrance map
// (the instance_template parent for dungeons, the map's CorpseMapID otherwise)
// and whose DBC trigger sits inside the instance map. Rows are scanned in id
// order like the C++ AreaTriggerContainer.
func (s *session) instanceGoBackTrigger(ctx context.Context, mapID uint32) (tMap uint32, x, y, z, o float32, ok bool) {
	db := s.goWorldDB()
	if db == nil || s.server == nil || s.server.Data == nil {
		return 0, 0, 0, 0, 0, false
	}
	mapEntry, found, err := s.server.Data.Map(mapID)
	if err != nil || !found || mapEntry.CorpseMapID < 0 {
		return 0, 0, 0, 0, 0, false
	}
	var entranceMap uint32
	if mapEntry.IsDungeon() {
		var parent uint32
		if err := db.QueryRowContext(ctx, "SELECT parent FROM instance_template WHERE map = ?", mapID).Scan(&parent); err != nil {
			return 0, 0, 0, 0, 0, false
		}
		entranceMap = parent
	} else {
		entranceMap = uint32(mapEntry.CorpseMapID)
	}
	rows, err := db.QueryContext(ctx, "SELECT id, target_position_x, target_position_y, target_position_z, target_orientation FROM areatrigger_teleport WHERE target_map = ? ORDER BY id", entranceMap)
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var tx, ty, tz, to float32
		if err := rows.Scan(&id, &tx, &ty, &tz, &to); err != nil {
			continue
		}
		at, found, err := s.server.Data.AreaTrigger(id)
		if err == nil && found && at.ContinentID == mapID {
			return entranceMap, tx, ty, tz, to, true
		}
	}
	return 0, 0, 0, 0, 0, false
}

// instanceEntranceTrigger mirrors ObjectMgr::GetMapEntranceTrigger
// (ObjectMgr.cpp:7279): the first areatrigger_teleport row targeting the map
// whose DBC trigger entry exists.
func (s *session) instanceEntranceTrigger(ctx context.Context, mapID uint32) (x, y, z, o float32, ok bool) {
	db := s.goWorldDB()
	if db == nil || s.server == nil || s.server.Data == nil {
		return 0, 0, 0, 0, false
	}
	rows, err := db.QueryContext(ctx, "SELECT id, target_position_x, target_position_y, target_position_z, target_orientation FROM areatrigger_teleport WHERE target_map = ? ORDER BY id", mapID)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var tx, ty, tz, to float32
		if err := rows.Scan(&id, &tx, &ty, &tz, &to); err != nil {
			continue
		}
		if _, found, err := s.server.Data.AreaTrigger(id); err == nil && found {
			return tx, ty, tz, to, true
		}
	}
	return 0, 0, 0, 0, false
}

// handleCmdGoInstance mirrors HandleGoInstanceCommand (cs_go.cpp:366):
// fuzzy-match instance script names against the labels, then teleport to the
// instance gate (exit trigger, orientation + PI) or the instance start.
// Script names come straight from the instance_template.script column.
// LANG ids: 1189 no match, 1190/1191 multiple, 1193 no entrance,
// 1194 no exit, 1195 went to gate, 1196 went to start, 1197 gate failed,
// 1198 start failed.
func (s *session) handleCmdGoInstance(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	db := s.goWorldDB()
	if db == nil || s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	rows, err := db.QueryContext(ctx, "SELECT map, script FROM instance_template")
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Instance lookup error: %v", err))
		return true
	}
	defer rows.Close()
	type instanceMatch struct {
		count   uint32
		mapID   uint32
		mapName string
		script  string
	}
	var matches []instanceMatch
	for rows.Next() {
		var mapID uint32
		var script string
		if err := rows.Scan(&mapID, &script); err != nil {
			continue
		}
		mapEntry, found, err := s.server.Data.Map(mapID)
		if err != nil || !found {
			continue
		}
		var count uint32
		for _, label := range args {
			if goContainsFold(script, label) {
				count++
			}
		}
		if count > 0 {
			matches = append(matches, instanceMatch{count: count, mapID: mapID, mapName: mapEntry.MapName, script: script})
		}
	}
	if len(matches) == 0 {
		s.sendSysMessage("No instances match your request.")
		return true
	}
	var maxCount uint32
	for _, m := range matches {
		if m.count > maxCount {
			maxCount = m.count
		}
	}
	top := 0
	for _, m := range matches {
		if m.count == maxCount {
			top++
		}
	}
	if top > 1 {
		s.sendSysMessage("Multiple instances match your request. Please refine your search.")
		for _, m := range matches {
			if m.count == maxCount {
				s.sendSysMessage(fmt.Sprintf("%s (ID: %d), script: %s", m.mapName, m.mapID, m.script))
			}
		}
		return true
	}
	var target instanceMatch
	for _, m := range matches {
		if m.count == maxCount {
			target = m
			break
		}
	}
	if s.isInFlight() {
		s.finishTaxiFlight()
	} else {
		s.saveRecallPosition()
	}
	if tMap, x, y, z, o, ok := s.instanceGoBackTrigger(ctx, target.mapID); ok {
		if s.teleportTo(tMap, x, y, z, o+float32(math.Pi)) {
			s.sendSysMessage(fmt.Sprintf("Teleported to the instance gate of %s (%d).", target.mapName, target.mapID))
			return true
		}
		parentName := ""
		if parent, found, err := s.server.Data.Map(tMap); err == nil && found {
			parentName = parent.MapName
		}
		s.sendSysMessage(fmt.Sprintf("Could not teleport to the instance gate of %s (%d). Entrance map %s (%d).", target.mapName, target.mapID, parentName, tMap))
	} else {
		s.sendSysMessage(fmt.Sprintf("Instance %s (%d) has no exit.", target.mapName, target.mapID))
	}
	if x, y, z, o, ok := s.instanceEntranceTrigger(ctx, target.mapID); ok {
		if s.teleportTo(target.mapID, x, y, z, o) {
			s.sendSysMessage(fmt.Sprintf("Teleported to the instance start of %s (%d).", target.mapName, target.mapID))
			return true
		}
		s.sendSysMessage(fmt.Sprintf("Could not teleport to the instance start of %s (%d).", target.mapName, target.mapID))
	} else {
		s.sendSysMessage(fmt.Sprintf("Instance %s (%d) has no entrance.", target.mapName, target.mapID))
	}
	return true
}

// goContainsFold reports whether hay contains needle, case-insensitively,
// mirroring StringContainsStringI used by the instance/boss matchers.
func goContainsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

// handleCmdGoBoss mirrors HandleGoBossCommand (cs_go.cpp:451): fuzzy-match
// dungeon bosses against the needles, then teleport to the boss's spawn. The
// boss set reproduces the DUNGEON_BOSS flag assignment (ObjectMgr.cpp:6085):
// instance_encounters rows with creditType 0 (ENCOUNTER_CREDIT_KILL_CREATURE)
// plus their creature_template difficulty entries. Matching runs over both
// the script name and the creature name. LANG ids: 1205 no match, 1206/1207
// multiple, 1208/1209 multiple spawns, 1210 teleport failed, 1211 went to boss.
func (s *session) handleCmdGoBoss(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	db := s.goWorldDB()
	if db == nil {
		s.sendSysMessage("Database not available.")
		return true
	}
	bossEntries := make(map[uint32]struct{})
	rows, err := db.QueryContext(ctx, "SELECT creditEntry FROM instance_encounters WHERE creditType = 0")
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Boss lookup error: %v", err))
		return true
	}
	var credits []uint32
	for rows.Next() {
		var entry uint32
		if err := rows.Scan(&entry); err == nil && entry != 0 {
			credits = append(credits, entry)
		}
	}
	rows.Close()
	for _, entry := range credits {
		bossEntries[entry] = struct{}{}
		var d1, d2, d3 uint32
		if err := db.QueryRowContext(ctx, "SELECT difficulty_entry_1, difficulty_entry_2, difficulty_entry_3 FROM creature_template WHERE entry = ?", entry).Scan(&d1, &d2, &d3); err == nil {
			for _, d := range []uint32{d1, d2, d3} {
				if d != 0 {
					bossEntries[d] = struct{}{}
				}
			}
		}
	}
	type bossSpawnPoint struct {
		guid       uint32
		mapID      uint32
		x, y, z, o float32
	}
	type bossMatch struct {
		count  uint32
		entry  uint32
		name   string
		script string
		spawns []bossSpawnPoint
	}
	var matches []bossMatch
	for entry := range bossEntries {
		var name, script string
		if err := db.QueryRowContext(ctx, "SELECT name, ScriptName FROM creature_template WHERE entry = ?", entry).Scan(&name, &script); err != nil {
			continue
		}
		var count uint32
		for _, needle := range args {
			if goContainsFold(script, needle) || goContainsFold(name, needle) {
				count++
			}
		}
		if count == 0 {
			continue
		}
		srows, err := db.QueryContext(ctx, "SELECT guid, map, position_x, position_y, position_z, orientation FROM creature WHERE id = ?", entry)
		if err != nil {
			continue
		}
		var spawns []bossSpawnPoint
		for srows.Next() {
			var sp bossSpawnPoint
			if err := srows.Scan(&sp.guid, &sp.mapID, &sp.x, &sp.y, &sp.z, &sp.o); err == nil {
				spawns = append(spawns, sp)
			}
		}
		srows.Close()
		if len(spawns) == 0 {
			continue
		}
		matches = append(matches, bossMatch{count: count, entry: entry, name: name, script: script, spawns: spawns})
	}
	if len(matches) == 0 {
		s.sendSysMessage("No bosses match your request.")
		return true
	}
	var maxCount uint32
	for _, m := range matches {
		if m.count > maxCount {
			maxCount = m.count
		}
	}
	top := 0
	for _, m := range matches {
		if m.count == maxCount {
			top++
		}
	}
	if top > 1 {
		s.sendSysMessage("Multiple bosses match your request. Please refine your search.")
		for _, m := range matches {
			if m.count == maxCount {
				s.sendSysMessage(fmt.Sprintf("%s (ID: %d), script: %s", m.name, m.entry, m.script))
			}
		}
		return true
	}
	var boss bossMatch
	for _, m := range matches {
		if m.count == maxCount {
			boss = m
			break
		}
	}
	if len(boss.spawns) > 1 {
		s.sendSysMessage(fmt.Sprintf("Boss %s (%d) has multiple spawn points:", boss.name, boss.entry))
		for _, sp := range boss.spawns {
			mapName := ""
			if s.server != nil && s.server.Data != nil {
				if me, found, err := s.server.Data.Map(sp.mapID); err == nil && found {
					mapName = me.MapName
				}
			}
			s.sendSysMessage(fmt.Sprintf("Spawn %d on map %d (%s) at (%.2f, %.2f, %.2f, %.2f)", sp.guid, sp.mapID, mapName, sp.x, sp.y, sp.z, sp.o))
		}
		return true
	}
	if s.isInFlight() {
		s.finishTaxiFlight()
	} else {
		s.saveRecallPosition()
	}
	sp := boss.spawns[0]
	if !s.teleportTo(sp.mapID, sp.x, sp.y, sp.z, sp.o) {
		mapName := ""
		if s.server != nil && s.server.Data != nil {
			if me, found, err := s.server.Data.Map(sp.mapID); err == nil && found {
				mapName = me.MapName
			}
		}
		s.sendSysMessage(fmt.Sprintf("Could not teleport to spawn %d of boss %s (%d) on map %s.", sp.guid, boss.name, boss.entry, mapName))
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Teleported to boss %s (%d), spawn %d.", boss.name, boss.entry, sp.guid))
	return true
}

func (s *session) handleCmdCast(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// cast_commandscript (cs_cast.cpp) sub-arms, wired on the player and
	// creature cast funnels: self/back/target ride castCreatureSpell (and
	// the player funnel for a selected player), dist/dest ride the
	// explicit-destination entries (player AoF funnel, creature AoF leg).
	switch strings.ToLower(args[0]) {
	case "back":
		return s.handleCmdCastBack(ctx, args)
	case "dist":
		return s.handleCmdCastDist(ctx, args)
	case "self":
		return s.handleCmdCastSelf(ctx, args)
	case "target":
		return s.handleCmdCastTarget(ctx, args)
	case "dest":
		return s.handleCmdCastDest(ctx, args)
	}
	// Bare arm: HandleCastCommand - player casts at the selected unit.
	if !s.commandAllowed(ctx, permissionCommandCast) {
		return false
	}
	const bareSyntax = "Syntax: .cast <spellId> [triggered]"
	spellID, ok := s.castCommandSpell(args[0])
	if !ok {
		return true
	}
	// ChatCommand.h:112 - the entire string must be consumed; a trailing
	// token fails the command before the handler runs.
	if len(args) > 2 {
		return false
	}
	// SpellMgr::IsSpellValid has no Go model; DBC-loaded spells are assumed
	// structurally valid, so the LANG_COMMAND_SPELL_BROKEN branch is unreachable.
	if s.selection == 0 {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	// GetTriggerFlags mirror: the optional flag must be a prefix of
	// "triggered" (e.g. "trig"); anything else fails the command. The C++
	// match is case-sensitive (StringStartsWith, Util.h:335), so "Trig"
	// fails. Go's command cast pipeline only models triggered-cast
	// semantics (TRIGGERED_FULL_MASK: no cast time, power, or proc rolls),
	// so the TRIGGERED_NONE normal-cast path is a documented fidelity gap
	// — the flag is accepted but changes nothing.
	if _, ok := castCommandTriggeredArg(s, args, 1, bareSyntax); !ok {
		return false
	}
	s.castSpellDirect(ctx, spellID, s.selection)
	return true
}

// castCommandSpell mirrors the bare arm's spell validation (cs_cast.cpp
// CheckSpellExistsAndIsValid minus the IsSpellValid leg: DBC-loaded spells
// are assumed structurally valid, as in the bare arm). The C++ SpellInfo
// argument also accepts a shift-clicked spell link
// (ChatCommandArgs.cpp ArgInfo<SpellInfo const*>::TryConsume, via
// SpellInfoVisitor over Hyperlink<spell>), so a |Hspell:id| token resolves
// the same way (miscParseSpellArg precedent); enchant/glyph/talent/trade
// links, which the visitor also resolves to a spell, stay out — no Go
// model for those link forms on this path.
func (s *session) castCommandSpell(spellArg string) (uint32, bool) {
	var spellID uint64
	if linkID := miscExtractLinkID(spellArg, "Hspell"); linkID != 0 {
		spellID = uint64(linkID)
	} else {
		// The bare id parses base-0 (StringTo<uint32>, base 0); the link
		// id above parses base-10 (base_tag StoreTo, Hyperlinks.h:107-131).
		v, ok := parseCommandUint32(spellArg)
		if !ok {
			s.sendSysMessage("Invalid spell ID.")
			return 0, false
		}
		spellID = uint64(v)
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Spell data is unavailable.")
		return 0, false
	}
	if _, found, err := s.server.Data.Spell(uint32(spellID)); err != nil || !found {
		s.sendSysMessage("There is no such spell.")
		return 0, false
	}
	return uint32(spellID), true
}

// castCommandTriggered mirrors GetTriggerFlags (cs_cast.cpp): an absent flag
// is TRIGGERED_NONE, a prefix of "triggered" is TRIGGERED_FULL_DEBUG_MASK,
// anything else fails the command silently (GetTriggerFlags returns nullopt). The C++ match is
// case-sensitive (StringStartsWith, Util.h:335), so "Trig" fails.
func (s *session) castCommandTriggered(trigArg, syntax string) (bool, bool) {
	if trigArg == "" {
		return false, true
	}
	if !strings.HasPrefix("triggered", trigArg) {
		return false, false
	}
	return true, true
}

// castCommandTriggeredArg reads the optional triggered flag at trigIdx.
func castCommandTriggeredArg(s *session, args []string, trigIdx int, syntax string) (bool, bool) {
	trigArg := ""
	if len(args) > trigIdx {
		trigArg = args[trigIdx]
	}
	return s.castCommandTriggered(trigArg, syntax)
}

// castCommandSelectedCreature mirrors getSelectedCreature (Chat.cpp) for the
// cast arms: the selection resolved through the GM's map/instance motion map.
func (s *session) castCommandSelectedCreature() *creatureMotion {
	if s == nil || s.selection == 0 {
		return nil
	}
	return s.findCreatureMotion(s.selection)
}

// castCommandCreatureCast fires a creature cast honoring the triggered flag
// (castCreatureSpell vs castCreatureSpellTriggered).
func (s *session) castCommandCreatureCast(ctx context.Context, motion *creatureMotion, spellID uint32, targetGUID uint64, triggered bool) {
	if triggered {
		s.server.castCreatureSpellTriggered(ctx, motion, spellID, targetGUID)
	} else {
		s.server.castCreatureSpell(ctx, motion, spellID, targetGUID)
	}
}

// handleCmdCastSelf wires HandleCastSelfCommand (cs_cast.cpp): the selected
// unit casts the spell on itself. A selected player rides the player funnel
// (triggered-only, so the flag is accepted but changes nothing, as in the
// bare arm); a selected creature rides the creature funnel with the flag
// honored.
func (s *session) handleCmdCastSelf(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCastSelf) {
		return false
	}
	const syntax = "Syntax: .cast self <spellId> [triggered]"
	if len(args) < 2 {
		return false
	}
	// The framework parses the spell argument before the handler runs, so
	// a nonexistent spell fails before the selection check (the 2026-11-03
	// note had this backwards for the parse leg; the handler-internal
	// selection-first order only governs an existing-but-invalid spell,
	// whose IsSpellValid leg has no Go model).
	spellID, ok := s.castCommandSpell(args[1])
	if !ok {
		return true
	}
	if len(args) > 3 {
		return false
	}
	if s.selection == 0 {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	triggered, ok := castCommandTriggeredArg(s, args, 2, syntax)
	if !ok {
		return false
	}
	if ts := s.lookupSelectedPlayer(); ts != nil {
		ts.castSpellDirect(ctx, spellID, ts.playerGUID)
		return true
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	s.castCommandCreatureCast(ctx, motion, spellID, motion.GUID, triggered)
	return true
}

// handleCmdCastBack wires HandleCastBackCommand (cs_cast.cpp): the selected
// creature casts the spell on the invoking player.
func (s *session) handleCmdCastBack(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCastBack) {
		return false
	}
	const syntax = "Syntax: .cast back <spellId> [triggered]"
	if len(args) < 2 {
		return false
	}
	// Framework-first spell order, as in the self arm: a nonexistent spell
	// fails before the handler's selection check.
	spellID, ok := s.castCommandSpell(args[1])
	if !ok {
		return true
	}
	if len(args) > 3 {
		return false
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	triggered, ok := castCommandTriggeredArg(s, args, 2, syntax)
	if !ok {
		return false
	}
	s.castCommandCreatureCast(ctx, motion, spellID, s.playerGUID, triggered)
	return true
}

// handleCmdCastTarget wires HandleCastTargetCommad (cs_cast.cpp): the
// selected creature casts the spell on its victim (motion.TargetGUID, the
// threat-manager combat victim).
func (s *session) handleCmdCastTarget(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCastTarget) {
		return false
	}
	const syntax = "Syntax: .cast target <spellId> [triggered]"
	if len(args) < 2 {
		return false
	}
	// Framework-first spell order, as in the self arm; the victim check
	// keeps its handler-internal place after the selection check.
	spellID, ok := s.castCommandSpell(args[1])
	if !ok {
		return true
	}
	if len(args) > 3 {
		return false
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	if motion.TargetGUID == 0 {
		s.sendSysMessage("The selected target has no victim.")
		return true
	}
	triggered, ok := castCommandTriggeredArg(s, args, 2, syntax)
	if !ok {
		return false
	}
	s.castCommandCreatureCast(ctx, motion, spellID, motion.TargetGUID, triggered)
	return true
}

// handleCmdCastDist wires HandleCastDistCommand (cs_cast.cpp): the player
// casts the spell at the GetClosePoint point for the given distance. The
// dist lands in the size slot (Object.cpp:3307-3311), so the point sits
// combatReach + dist along the player's orientation at the player's z; the
// UpdateAllowedPositionZ ground clamp and the LoS walk have no Go model
// (transmittedClosePoint precedent). The triggered flag is validated but
// the player funnel is triggered-only (castSpellDirectWithOverrides is
// Go's Unit::CastSpell(id, triggered=true)), so a no-flag dist cast rides
// the triggered path exactly like the bare arm; the TRIGGERED_NONE
// normal-cast path is the documented triggered-only pipeline gap, not a
// dist-specific dropped flag (audit D10 corrected).
func (s *session) handleCmdCastDist(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCastDist) {
		return false
	}
	const syntax = "Syntax: .cast dist <spellId> <dist> [triggered]"
	// The framework parses the spell and dist arguments before the
	// handler runs: a garbage spell token reports before the missing-dist
	// check, and a bad dist token reports the parse message.
	if len(args) < 2 {
		return false
	}
	spellID, ok := s.castCommandSpell(args[1])
	if !ok {
		return true
	}
	if len(args) < 3 {
		return false
	}
	dist, ok := parseCommandFloat(args[2])
	if !ok {
		// Unparseable float token (cs_cast.cpp:134): the framework prints
		// LANG_CMDPARSER_STRING_VALUE_INVALID + SetSentErrorMessage
		// (non-finite rejected, ChatCommandArgs.h).
		s.sendSysMessage(syntax)
		return true
	}
	if len(args) > 4 {
		return false
	}
	if _, ok := castCommandTriggeredArg(s, args, 3, syntax); !ok {
		return false
	}
	if s.player == nil {
		return true
	}
	offset := s.player.CombatReach + float32(dist)
	dest := protocol.SpellTargetLocation{
		X: s.player.X + offset*float32(math.Cos(float64(s.player.Orientation))),
		Y: s.player.Y + offset*float32(math.Sin(float64(s.player.Orientation))),
		Z: s.player.Z,
	}
	s.castSpellDirectWithOverrides(ctx, spellID, 0, false, nil, 0, &dest, 0)
	return true
}

// handleCmdCastDest wires HandleCastDestCommand (cs_cast.cpp): the selected
// unit casts the spell at the given coordinates. A selected player rides
// the player AoF funnel (triggered-only, flag accepted but ignored); a
// selected creature rides the creature AoF leg with the flag honored.
func (s *session) handleCmdCastDest(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandCastDest) {
		return false
	}
	const syntax = "Syntax: .cast dest <spellId> <x> <y> <z> [triggered]"
	if len(args) < 2 {
		return false
	}
	// Framework-first order, as in the self arm: the spell and the three
	// coordinates are all parsed before the handler runs, so they fail
	// before the selection check.
	spellID, ok := s.castCommandSpell(args[1])
	if !ok {
		return true
	}
	if len(args) < 5 {
		return false
	}
	coords := make([]float32, 3)
	for i := 0; i < 3; i++ {
		v, ok := parseCommandFloat(args[2+i])
		if !ok {
			// Unparseable float token (cs_cast.cpp:201): the framework
			// prints LANG_CMDPARSER_STRING_VALUE_INVALID +
			// SetSentErrorMessage (non-finite rejected, ChatCommandArgs.h).
			s.sendSysMessage(syntax)
			return true
		}
		coords[i] = float32(v)
	}
	if len(args) > 6 {
		return false
	}
	if s.selection == 0 {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	triggered, ok := castCommandTriggeredArg(s, args, 5, syntax)
	if !ok {
		return false
	}
	if ts := s.lookupSelectedPlayer(); ts != nil {
		dest := protocol.SpellTargetLocation{X: coords[0], Y: coords[1], Z: coords[2]}
		ts.castSpellDirectWithOverrides(ctx, spellID, 0, false, nil, 0, &dest, 0)
		return true
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a character or creature.")
		return true
	}
	dest := [3]float32{coords[0], coords[1], coords[2]}
	s.server.castCreatureSpellInternalWithBasePoints(ctx, motion, spellID, 0, triggered, nil, &dest, false, 0)
	return true
}

// blockedCommandArm gates a blocked command sub-arm on its RBAC permission (mirroring
// the ChatCommandTable permission check, which runs before the handler body)
// and reports the missing bridge honestly instead of stubbing the arm.
// Denial is silent false: the C++ framework never invokes a denied leaf
// (ChatCommand.cpp:481-488), so no notification is sent.
func (s *session) blockedCommandArm(ctx context.Context, permissionID uint32, reason string) bool {
	if !s.commandAllowed(ctx, permissionID) {
		return false
	}
	s.sendSysMessage(reason)
	return true
}

// handleCmdAhbot dispatches ".ahbot items|ratio|rebuild|reload|status"
// (cs_ahbot.cpp ahbotCommandTable). Every arm drives the AuctionHouseBot
// engine (sAuctionBot / sAuctionBotConfig: SetItemsAmount[ForQuality],
// SetItemsRatio[ForHouse], Rebuild, ReloadAllConfig, PrepareStatusInfos),
// which has no Go model, so each arm is RBAC-gated on its own C++
// permission and reports the missing bridge honestly instead of stubbing
// the arm (blockedCommandArm pattern). The RBAC permission is checked
// BEFORE the shape (ChatCommandNode checks IsInvokerVisible before the
// invoker parses its args); the shape is still validated against the C++
// typed dispatcher before the blocked message, because a
// malformed invocation never reaches the engine in C++: the bare items
// handler takes a 7-element array and the bare ratio handler three uint32s,
// so a bare arm with no numbers fails the parse (the array/uint32
// TryConsume calls return nullopt on an empty token, ChatCommandArgs.h),
// a quality/house arm needs exactly one uint32, and rebuild/status take an
// Optional<EXACT_SEQUENCE("all")> (case-insensitive per ChatCommandTags.h),
// so any other trailing token fails the whole-string-consumed check
// (ChatCommand.h ConsumeFromOffset) and the framework shows help.
func (s *session) handleCmdAhbot(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	const blocked = "ahbot is not supported: the auction house bot engine has no Go model."
	switch strings.ToLower(args[0]) {
	case "items":
		if len(args) >= 2 {
			if matches := ahbotQualityMatches(args[1]); len(matches) > 1 {
				s.sendSysMessage(fmt.Sprintf("Subcommand '%s' of command '.ahbot items' is ambiguous, please use one of the following:", args[1]))
				for _, name := range matches {
					s.sendSysMessage("  " + name)
				}
				return true
			}
		}
		perm := ahbotItemsPerm(args)
		if !s.commandAllowed(ctx, perm) {
			return false
		}
		if ok, msgLeg := ahbotItemsLeg(args); !ok {
			if msgLeg {
				s.sendSysMessage("Syntax: .ahbot items [gray|white|green|blue|purple|orange|yellow]")
				return true
			}
			return false
		}
		return s.blockedCommandArm(ctx, perm, blocked)
	case "ratio":
		perm := ahbotRatioPerm(args)
		if !s.commandAllowed(ctx, perm) {
			return false
		}
		if ok, msgLeg := ahbotRatioLeg(args); !ok {
			if msgLeg {
				s.sendSysMessage("Syntax: .ahbot ratio [alliance|horde|neutral]")
				return true
			}
			return false
		}
		return s.blockedCommandArm(ctx, perm, blocked)
	case "rebuild":
		if !s.commandAllowed(ctx, permissionCommandAhbotRebuild) {
			return false
		}
		if len(args) > 2 || (len(args) == 2 && !strings.EqualFold(args[1], "all")) {
			return false
		}
		return s.blockedCommandArm(ctx, permissionCommandAhbotRebuild, blocked)
	case "reload":
		if !s.commandAllowed(ctx, permissionCommandAhbotReload) {
			return false
		}
		if len(args) > 1 {
			return false
		}
		return s.blockedCommandArm(ctx, permissionCommandAhbotReload, blocked)
	case "status":
		if !s.commandAllowed(ctx, permissionCommandAhbotStatus) {
			return false
		}
		if len(args) > 2 || (len(args) == 2 && !strings.EqualFold(args[1], "all")) {
			return false
		}
		return s.blockedCommandArm(ctx, permissionCommandAhbotStatus, blocked)
	}
	return false
}

// parseCommandUint32 mirrors the C++ uint32 argument parse (StringTo<uint32>
// with base 0, StringConvert.h:43-69): 0x is hex and 0b is binary, anything
// else is base 10 — a leading 0 is NOT octal ("010" is 10, "08" is 8). No
// sign, no underscores, no whitespace, and the value must fit in 32 bits.
// std::from_chars rejects both '+' and '-' for unsigned types (verified
// against the toolchain's <charconv>), so neither is accepted here.
func parseCommandUint32(s string) (uint32, bool) {
	if s == "" || strings.ContainsAny(s, "_+-") {
		return 0, false
	}
	base, digits := 10, s
	switch {
	case len(s) > 2 && strings.EqualFold(s[:2], "0x"):
		base, digits = 16, s[2:]
	case len(s) > 2 && strings.EqualFold(s[:2], "0b"):
		base, digits = 2, s[2:]
	}
	if digits == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// parseCommandUint8 mirrors parseCommandUint32 for 8-bit unsigned args
// (StringTo<uint8> with base 0, StringConvert.h:43-69): 0x hex, 0b binary,
// else decimal, never octal; no sign, no underscores, must fit in 8 bits.
func parseCommandUint8(s string) (uint8, bool) {
	if s == "" || strings.ContainsAny(s, "_+-") {
		return 0, false
	}
	base, digits := 10, s
	switch {
	case len(s) > 2 && strings.EqualFold(s[:2], "0x"):
		base, digits = 16, s[2:]
	case len(s) > 2 && strings.EqualFold(s[:2], "0b"):
		base, digits = 2, s[2:]
	}
	if digits == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(digits, base, 8)
	if err != nil {
		return 0, false
	}
	return uint8(v), true
}

// parseCommandUint64 mirrors parseCommandUint32 for 64-bit unsigned args
// (StringTo<uint64> with base 0, StringConvert.h:43-69): 0x hex, 0b binary,
// else decimal, never octal; no sign, no underscores, must fit in 64 bits.
func parseCommandUint64(s string) (uint64, bool) {
	if s == "" || strings.ContainsAny(s, "_+-") {
		return 0, false
	}
	base, digits := 10, s
	switch {
	case len(s) > 2 && strings.EqualFold(s[:2], "0x"):
		base, digits = 16, s[2:]
	case len(s) > 2 && strings.EqualFold(s[:2], "0b"):
		base, digits = 2, s[2:]
	}
	if digits == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseCppBase0Int mirrors StringTo<T> with base 0 for signed integrals
// (StringConvert.h:43-69): a 0x/0b prefix (case-insensitive) selects hex or
// binary, anything else is decimal, never octal; the whole string must be
// consumed. A leading '-' is accepted by from_chars, a leading '+' is
// rejected, and "-0x10" fails (the prefix check runs before the sign, so the
// "0x" is not at the front). Callers range-check to their C++ type.
func parseCppBase0Int(s string) (int64, bool) {
	if s == "" || strings.HasPrefix(s, "+") {
		return 0, false
	}
	base, digits := 10, s
	switch {
	case len(s) > 2 && strings.EqualFold(s[:2], "0x"):
		base, digits = 16, s[2:]
	case len(s) > 2 && strings.EqualFold(s[:2], "0b"):
		base, digits = 2, s[2:]
	}
	if digits == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(digits, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseCommandFloat mirrors the C++ float ArgInfo (ChatCommandArgs.h:62-72):
// StringTo<float> with base 0 via std::stold (hex floats accepted,
// underscores rejected by the full-consume check) plus an explicit
// isfinite rejection, so inf/nan never reach the handler. Go's ParseFloat
// accepts underscores and inf/nan but rejects a hex float without a p
// exponent, so both legs are normalized here (a bare 0x hex float gets a
// p0 exponent, mirroring stold; 0b/0o prefixes fail in C++ too).
func parseCommandFloat(s string) (float64, bool) {
	if s == "" || strings.Contains(s, "_") {
		return 0, false
	}
	switch strings.ToLower(s) {
	case "inf", "+inf", "-inf", "infinity", "+infinity", "-infinity", "nan", "+nan", "-nan":
		return 0, false
	}
	digits := s
	if len(digits) > 0 && (digits[0] == '+' || digits[0] == '-') {
		digits = digits[1:]
	}
	if strings.HasPrefix(digits, "0b") || strings.HasPrefix(digits, "0B") || strings.HasPrefix(digits, "0o") || strings.HasPrefix(digits, "0O") {
		return 0, false
	}
	text := s
	if (strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X")) && !strings.ContainsAny(digits, "pP") {
		text = s + "p0"
	}
	v, err := strconv.ParseFloat(text, 32)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func ahbotUint32(s string) bool {
	_, ok := parseCommandUint32(s)
	return ok
}

// ahbotQualityMatches lists the qualities a token prefix-matches
// (ChatCommand.cpp:265-285: an ambiguous prefix lists its matches instead
// of executing; only "g" is ambiguous, gray vs green).
func ahbotQualityMatches(tok string) []string {
	lower := strings.ToLower(tok)
	qualities := []string{"gray", "white", "green", "blue", "purple", "orange", "yellow"}
	var matches []string
	for _, q := range qualities {
		if strings.HasPrefix(q, lower) {
			matches = append(matches, q)
		}
	}
	return matches
}

// ahbotQualityToken resolves a quality token the way the C++ subcommand
// traversal does (ChatCommand.cpp FilteredCommandListIterator): an exact
// name wins, otherwise a unique case-insensitive prefix matches; an
// ambiguous prefix (e.g. "g" for gray/green) or an unknown token matches
// nothing. The Go command tree only canonicalizes the ahbot node's own
// children, so this second-level resolution has to happen here.
func ahbotQualityToken(tok string) (string, bool) {
	lower := strings.ToLower(tok)
	qualities := []string{"gray", "white", "green", "blue", "purple", "orange", "yellow"}
	for _, q := range qualities {
		if q == lower {
			return q, true
		}
	}
	var match string
	for _, q := range qualities {
		if strings.HasPrefix(q, lower) {
			if match != "" {
				return "", false
			}
			match = q
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

// ahbotHouseToken is the ahbotQualityToken equivalent for the ratio houses.
func ahbotHouseToken(tok string) (string, bool) {
	lower := strings.ToLower(tok)
	houses := []string{"alliance", "horde", "neutral"}
	for _, h := range houses {
		if h == lower {
			return h, true
		}
	}
	var match string
	for _, h := range houses {
		if strings.HasPrefix(h, lower) {
			if match != "" {
				return "", false
			}
			match = h
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

// ahbotItemsLeg classifies an items invocation the way the C++ typed
// dispatcher does (ChatCommandArgs.h): (valid, messageLeg). A quality arm
// takes exactly one uint32 amount; an unknown quality token falls through
// to the bare array arm, which takes exactly seven uint32s. A non-uint32
// token reports the framework parse message (messageLeg); a short array,
// a missing amount, or trailing tokens fail silently.
func ahbotItemsLeg(args []string) (bool, bool) {
	if len(args) >= 2 {
		if _, ok := ahbotQualityToken(args[1]); ok {
			switch {
			case len(args) == 2:
				return false, false
			case !ahbotUint32(args[2]):
				return false, true
			case len(args) > 3:
				return false, false
			default:
				return true, false
			}
		}
	}
	nums := args[1:]
	if len(nums) > 7 {
		nums = nums[:7]
	}
	for _, tok := range nums {
		if !ahbotUint32(tok) {
			return false, true
		}
	}
	if len(args)-1 != 7 {
		return false, false
	}
	return true, false
}

// ahbotRatioLeg is the ahbotItemsLeg equivalent for ratio: house + one
// uint32, or exactly three uint32s for the bare arm.
func ahbotRatioLeg(args []string) (bool, bool) {
	if len(args) >= 2 {
		if _, ok := ahbotHouseToken(args[1]); ok {
			switch {
			case len(args) == 2:
				return false, false
			case !ahbotUint32(args[2]):
				return false, true
			case len(args) > 3:
				return false, false
			default:
				return true, false
			}
		}
	}
	nums := args[1:]
	if len(nums) > 3 {
		nums = nums[:3]
	}
	for _, tok := range nums {
		if !ahbotUint32(tok) {
			return false, true
		}
	}
	if len(args)-1 != 3 {
		return false, false
	}
	return true, false
}

// ahbotItemsPerm mirrors the ahbotItemsAmountCommandTable permission ladder:
// a quality sub-token picks its own permission; anything else falls
// through to the bare array handler, which carries the items permission
// (an unknown token matches no quality row, so the "" entry handles it).
func ahbotItemsPerm(args []string) uint32 {
	if len(args) >= 2 {
		if quality, ok := ahbotQualityToken(args[1]); ok {
			switch quality {
			case "gray":
				return permissionCommandAhbotItemsGray
			case "white":
				return permissionCommandAhbotItemsWhite
			case "green":
				return permissionCommandAhbotItemsGreen
			case "blue":
				return permissionCommandAhbotItemsBlue
			case "purple":
				return permissionCommandAhbotItemsPurple
			case "orange":
				return permissionCommandAhbotItemsOrange
			case "yellow":
				return permissionCommandAhbotItemsYellow
			}
		}
	}
	return permissionCommandAhbotItems
}

// ahbotRatioPerm mirrors the ahbotItemsRatioCommandTable permission ladder:
// a house sub-token picks its own permission; anything else falls through
// to the bare triple handler, which carries the ratio permission.
func ahbotRatioPerm(args []string) uint32 {
	if len(args) >= 2 {
		if house, ok := ahbotHouseToken(args[1]); ok {
			switch house {
			case "alliance":
				return permissionCommandAhbotRatioAlliance
			case "horde":
				return permissionCommandAhbotRatioHorde
			case "neutral":
				return permissionCommandAhbotRatioNeutral
			}
		}
	}
	return permissionCommandAhbotRatio
}

// handleCmdCharacter dispatches ".character customize|changefaction|changerace|
// changeaccount|deleted|erase|level|rename|reputation|titles"
// (cs_character.cpp characterCommandTable), gating each arm on its RBAC permission.
func (s *session) handleCmdCharacter(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	deletedNested := false
	switch sub {
	case "customize":
		perm = permissionCommandCharacterCustomize
	case "changefaction":
		perm = permissionCommandCharacterChangeFaction
	case "changerace":
		perm = permissionCommandCharacterChangeRace
	case "changeaccount":
		perm = permissionCommandCharacterChangeAccount
	case "deleted":
		deletedNested = true // permission resolved per nested arm in handleCharacterDeleted
	case "erase":
		perm = permissionCommandCharacterErase
	case "level":
		perm = permissionCommandCharacterLevel
	case "rename":
		perm = permissionCommandCharacterRename
	case "reputation":
		perm = permissionCommandCharacterReputation
	case "titles":
		perm = permissionCommandCharacterTitles
	default:
		return false
	}
	if !deletedNested && !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "customize":
		return s.handleCharacterAtLoginFlag(ctx, rest, atLoginCustomize, "customize")
	case "changefaction":
		return s.handleCharacterAtLoginFlag(ctx, rest, atLoginChangeFaction, "change faction")
	case "changerace":
		return s.handleCharacterAtLoginFlag(ctx, rest, atLoginChangeRace, "change race")
	case "changeaccount":
		return s.handleCharacterChangeAccount(ctx, rest)
	case "deleted":
		return s.handleCharacterDeleted(ctx, rest)
	case "erase":
		return s.handleCharacterErase(ctx, rest)
	case "level":
		return s.handleCharacterLevel(ctx, rest)
	case "rename":
		return s.handleCharacterRename(ctx, rest)
	case "reputation":
		return s.handleCharacterReputation(ctx, rest)
	case "titles":
		return s.handleCharacterTitles(ctx, rest)
	}
	return false
}

// characterTarget mirrors the Trinity PlayerIdentifier resolve used by the
// character command handlers: a connected session wins, otherwise the
// character row supplies the guid. With no name the C++ arms use
// PlayerIdentifier::FromTargetOrSelf (selection if it is a connected player,
// else the handler's own player); the FromTarget arms use
// resolveCharacterTargetFromTarget instead, which fails with no selection.
type characterTarget struct {
	name   string
	guid   uint64
	online *session
}

// resolveFailKind distinguishes the two C++ failure modes of a
// PlayerIdentifier resolve: no target at all (nullopt -> the handler
// returns false silently) vs a bad name/GUID token (framework parse error
// -> the handler prints and returns true). The character family needs the
// distinction because the same helper failure is silent-false for some arms
// (level, at-login flags) and message+true for others (titles, reputation).
type resolveFailKind int

const (
	resolveOK resolveFailKind = iota
	resolveFailSilent
	resolveFailMessage
)

// resolveCharacterTarget consumes an optional leading character name and
// returns the resolved target plus the remaining args. It reports false and
// sends the C++ LANG_PLAYER_NOT_FOUND equivalent when the name resolves to
// nothing. Learn/LFG keep this wrapper (their ticks classified the failure
// as message+true); the character family uses resolveCharacterTargetKind
// for the silent-vs-message split.
func (s *session) resolveCharacterTarget(ctx context.Context, args []string) (characterTarget, []string, bool) {
	t, rest, fail := s.resolveCharacterTargetKind(ctx, args)
	if fail != resolveOK {
		s.sendSysMessage("Player not found.")
		return t, rest, false
	}
	return t, rest, true
}

// resolveCharacterTargetKind is the message-free core of
// resolveCharacterTarget: it classifies the failure instead of printing.
func (s *session) resolveCharacterTargetKind(ctx context.Context, args []string) (characterTarget, []string, resolveFailKind) {
	var t characterTarget
	if len(args) == 0 {
		// PlayerIdentifier::FromTargetOrSelf (ChatCommandTags.cpp): the
		// selected connected player wins; anything else falls back to self.
		if s.selection != 0 && s.server != nil {
			if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
				t.online = ts
				t.guid = ts.playerGUID
				t.name = ts.player.Name
				return t, args, resolveOK
			}
		}
		if s.player == nil {
			return t, args, resolveFailSilent
		}
		t.online = s
		t.guid = s.playerGUID
		t.name = s.player.Name
		return t, args, resolveOK
	}
	// C++ PlayerIdentifier::TryConsume (ChatCommandTags.cpp:88-121)
	// accepts three forms in order: a |Hplayer link, a numeric player
	// GUID, or a plain name. The link and GUID forms were missing here.
	var name string
	if strings.HasPrefix(args[0], "|") {
		name = normalizePlayerName(extractPlayerNameFromLink(args[0]))
	} else if guidVal, ok := parseCppBase0Int(args[0]); ok && guidVal > 0 && guidVal <= math.MaxUint32 {
		// Base-0 GUID form (StringTo<uint32> base 0): "0x10" is GUID 16,
		// "12abc" falls through to the name branch below.
		guid64 := uint64(guidVal)
		if online := s.server.playerSessionForGUID(guid64); online != nil && online.player != nil {
			t.online = online
			t.guid = online.playerGUID
			t.name = online.player.Name
			return t, args[1:], resolveOK
		}
		chars := s.server.CharactersStore
		if chars != nil && chars.DB != nil {
			var dbName string
			if err := chars.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid64).Scan(&dbName); err == nil && dbName != "" {
				t.guid = guid64
				t.name = dbName
				return t, args[1:], resolveOK
			}
		}
		return t, args, resolveFailMessage
	} else {
		name = normalizePlayerName(args[0])
	}
	t.name = name
	if name == "" {
		return t, args, resolveFailMessage
	}
	if online := s.sessionForPlayerName(name); online != nil {
		t.online = online
		t.guid = online.playerGUID
		if online.player != nil {
			t.name = online.player.Name
		}
		return t, args[1:], resolveOK
	}
	chars := s.server.CharactersStore
	if chars == nil || chars.DB == nil {
		return t, args, resolveFailMessage
	}
	if err := chars.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&t.guid); err != nil || t.guid == 0 {
		return t, args, resolveFailMessage
	}
	return t, args[1:], resolveOK
}

// resolveCharacterTargetFromTarget mirrors the C++ FromTarget arms
// (customize/changefaction/changerace/rename/changeaccount,
// cs_character.cpp:292/397/421/445/469): with no name argument the target is
// the selected connected player only — Player::GetSelectedPlayer returns
// null with no selection or a non-player selection, so the handler fails
// instead of falling back to self. With a name argument the shared
// resolveCharacterTarget forms apply. R13: no message is sent here; the
// caller classifies the failure (silent-false for no selection, message+true
// for a bad token).
func (s *session) resolveCharacterTargetFromTarget(ctx context.Context, args []string) (characterTarget, []string, resolveFailKind) {
	if len(args) == 0 {
		if s.selection != 0 && s.server != nil {
			if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
				return characterTarget{online: ts, guid: ts.playerGUID, name: ts.player.Name}, args, resolveOK
			}
		}
		return characterTarget{}, args, resolveFailSilent
	}
	return s.resolveCharacterTargetKind(ctx, args)
}

// characterArmBlocked RBAC-gates a documented-only character arm and reports
// the missing bridge honestly instead of stubbing the behavior.
func (s *session) characterArmBlocked(perm uint32, reason string) {
	s.sendSysMessage("Command recognized but not available: " + reason + ".")
}

// handleCharacterAtLoginFlag mirrors HandleCharacterCustomizeCommand,
// HandleCharacterChangeFactionCommand and HandleCharacterChangeRaceCommand
// (cs_character.cpp:394/418/442): online targets get the at-login flag set on
// the live player, offline targets get CHAR_UPD_ADD_AT_LOGIN_FLAG.
func (s *session) handleCharacterAtLoginFlag(ctx context.Context, args []string, flag uint64, what string) bool {
	t, rest, fail := s.resolveCharacterTargetFromTarget(ctx, args)
	if fail != resolveOK {
		if fail == resolveFailMessage {
			s.sendSysMessage("Player not found.")
			return true
		}
		return false
	}
	if len(rest) != 0 {
		return false
	}
	if t.online != nil && t.online.player != nil {
		t.online.player.AtLogin |= uint32(flag)
		t.online.sendPlayerUpdate()
		s.sendSysMessage(fmt.Sprintf("Set %s flag for %s. Please relog.", what, t.name))
		return true
	}
	chars := s.server.CharactersStore
	if chars == nil {
		return false
	}
	_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(flag), t.guid)
	s.sendSysMessage(fmt.Sprintf("Set %s flag for %s (GUID: %d).", what, t.name, t.guid))
	return true
}

// handleCharacterLevel mirrors HandleLevelUpCommand (cs_character.cpp:740):
// the level argument is a delta added to the current level, clamped to
// 1..STRONG_MAX_LEVEL (255, DBCEnums.h:53). Online targets are
// leveled in place with XP reset; offline targets get CHAR_UPD_LEVEL.
// InitTalentForLevel has no Go bridge: talents are left untouched.
func (s *session) handleCharacterLevel(ctx context.Context, args []string) bool {
	t, rest, fail := s.resolveCharacterTargetKind(ctx, args)
	if fail != resolveOK {
		// D1: a lone token that fails the PlayerIdentifier resolve is
		// retried as the int16 delta for self (Optional-consumer omit-path,
		// cs_character.cpp:740-745) — ".character level 5" levels self +5.
		if fail == resolveFailMessage && len(args) == 1 {
			if deltaVal, ok := parseCppBase0Int(args[0]); ok && deltaVal >= math.MinInt16 && deltaVal <= math.MaxInt16 {
				if self, _, selfFail := s.resolveCharacterTargetKind(ctx, nil); selfFail == resolveOK {
					t, rest = self, args
				} else {
					return false
				}
			} else {
				s.sendSysMessage("Player not found.")
				return true
			}
		} else if fail == resolveFailMessage {
			s.sendSysMessage("Player not found.")
			return true
		} else {
			return false
		}
	}
	if len(rest) != 1 {
		return false
	}
	// C++ level arg is int16 parsed base-0 (StringTo<int16>, base 0).
	deltaVal, ok := parseCppBase0Int(rest[0])
	if !ok || deltaVal < math.MinInt16 || deltaVal > math.MaxInt16 {
		s.sendSysMessage("Syntax: .character level [$player] <level-delta>")
		return true
	}
	delta := int(deltaVal)
	oldLevel := 0
	if t.online != nil && t.online.player != nil {
		oldLevel = int(t.online.player.Level)
	} else {
		chars := s.server.CharactersStore
		if chars == nil || chars.DB == nil {
			return false
		}
		var lvl uint8
		if err := chars.DB.QueryRowContext(ctx, "SELECT level FROM characters WHERE guid = ?", t.guid).Scan(&lvl); err != nil {
			return false
		}
		oldLevel = int(lvl)
	}
	newLevel := oldLevel + delta
	if newLevel < 1 {
		newLevel = 1
	}
	if newLevel > strongMaxLevel {
		newLevel = strongMaxLevel
	}
	if t.online != nil && t.online.player != nil {
		t.online.player.Level = uint8(newLevel)
		t.online.player.XP = 0
		// AutoBalance_PlayerScript::OnLevelChanged (AutoBalance.cpp).
		t.online.autoBalanceOnLevelChanged()
		t.online.sendPlayerUpdate()
		// Player::GiveLevel (Player.cpp:2787): the C++ .character level
		// command goes through GiveLevel, which fires OnPlayerLevelChanged.
		t.online.fireLevelChangedHook(ctx, uint8(oldLevel))
		if t.online != s && isVisibleGloballyFor(s, t.online) { // needReportToTarget
			invokerName := ""
			if s.player != nil {
				invokerName = s.player.Name
			}
			switch {
			case oldLevel == newLevel:
				t.online.sendSysMessage(fmt.Sprintf("Your level progress has been reset by %s.", invokerName))
			case oldLevel < newLevel:
				t.online.sendSysMessage(fmt.Sprintf("Your level has been increased to %d by %s.", newLevel, invokerName))
			default:
				t.online.sendSysMessage(fmt.Sprintf("Your level has been decreased to %d by %s.", newLevel, invokerName))
			}
		}
	} else {
		chars := s.server.CharactersStore
		if chars == nil {
			return false
		}
		_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_LEVEL"), uint8(newLevel), t.guid)
	}
	if t.online == nil || t.online != s { // C++: handler line suppressed when invoker == target
		s.sendSysMessage(fmt.Sprintf("Changed level of %s to %d.", t.name, newLevel))
	}
	return true
}

// handleCharacterRename mirrors HandleCharacterRenameCommand
// (cs_character.cpp:286): with a new name the character is renamed
// immediately (online targets are kicked, like the C++ KickPlayer), without
// one the AT_LOGIN_RENAME flag is set for the next login. The C++
// ObjectMgr::CheckPlayerName rules and the reserved-name
// RBAC_PERM_SKIP_CHECK_CHARACTER_CREATION_RESERVEDNAME exemption have no Go
// bridge: only name normalization and the duplicate-name DB check apply.
func (s *session) handleCharacterRename(ctx context.Context, args []string) bool {
	t, rest, fail := s.resolveCharacterTargetFromTarget(ctx, args)
	if fail != resolveOK {
		// D2: a single token is the new name with an empty player
		// (Optional-consumer omit-path, cs_character.cpp:288-289) — silent
		// false. With two tokens the first is an unresolvable player name:
		// the parse-error leg prints.
		if len(args) == 1 {
			return false
		}
		s.sendSysMessage("Player not found.")
		return true
	}
	if s.characterTargetLowerSecurity(ctx, t) {
		return false // C++ HasLowerSecurity: silent fail
	}
	chars := s.server.CharactersStore
	if chars == nil {
		return false
	}
	if len(rest) == 0 {
		// At-login rename flag on the resolved target.
		if t.online != nil && t.online.player != nil {
			t.online.player.AtLogin |= uint32(atLoginRename)
			t.online.sendPlayerUpdate()
			s.sendSysMessage(fmt.Sprintf("Set rename flag for %s. Please relog to choose a new name.", t.name))
			return true
		}
		_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ADD_AT_LOGIN_FLAG"), uint16(atLoginRename), t.guid)
		s.sendSysMessage(fmt.Sprintf("Set rename flag for %s (GUID: %d).", t.name, t.guid))
		return true
	}
	// ChatCommand.h:112 — the entire string must be consumed; a trailing
	// token fails the command.
	if len(rest) > 1 {
		return false
	}
	newName := normalizePlayerName(rest[0])
	if newName == "" {
		s.sendSysMessage("Incorrect value.")
		return true
	}
	if row, err := chars.QueryRowStatement(ctx, database.StatementID("CHAR_SEL_CHECK_NAME"), newName); err == nil {
		var one int
		if row.Scan(&one) == nil {
			s.sendSysMessage(fmt.Sprintf("Name %s is already in use.", newName))
			return true
		}
	}
	_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_DEL_DECLINED_NAME"), t.guid)
	if t.online != nil {
		if t.online.player != nil {
			t.online.player.Name = newName
			t.online.sendPlayerUpdate()
		}
		s.kickSession(t.online)
	} else {
		_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_NAME_BY_GUID"), newName, t.guid)
	}
	// sCharacterCache->UpdateCharacterData has no Go character-cache bridge.
	s.sendSysMessage(fmt.Sprintf("Renamed player %s to %s.", t.name, newName))
	return true
}

// characterTargetLowerSecurity mirrors ChatHandler::HasLowerSecurity
// (Chat.cpp:54-102): the command fails when the target's account outranks
// the handler's security level. Three C++ legs: the RBAC bypass (a handler
// holding RBAC_PERM_CHECK_FOR_LOWER_SECURITY skips a non-strong check when
// CONFIG_GM_LOWER_SECURITY is off — Go has no such config, and C++
// defaults it off at World.cpp:1025, so the bypass applies whenever the
// permission is held); the realm-specific account lookup (already inside
// accountSecurityLevel: RealmID or -1); and the unresolvable-target fail
// (no session and no account row fails the command, it does not
// silently allow it as a security-0 target).
func (s *session) characterTargetLowerSecurity(ctx context.Context, t characterTarget) bool {
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil && s.authed {
		if granted, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionCheckForLowerSecurity); err == nil && granted {
			return false
		}
	}
	var accountID uint32
	if t.online != nil {
		accountID = t.online.accountID
	} else if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.guid).Scan(&accountID); err != nil {
			return true
		}
	} else {
		return true
	}
	return s.security < s.accountSecurityLevel(ctx, accountID)
}

// characterTargetLowerSecurityStrong mirrors ChatHandler::HasLowerSecurity
// with strong=true (Chat.cpp:54-102): equal security also fails, and the
// RBAC_PERM_CHECK_FOR_LOWER_SECURITY bypass never applies to a strong check
// (the bypass is gated on !strong). Used by the mute/unmute arms
// (cs_misc.cpp:1865/1932).
func (s *session) characterTargetLowerSecurityStrong(ctx context.Context, t characterTarget) bool {
	var accountID uint32
	if t.online != nil {
		accountID = t.online.accountID
	} else if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.guid).Scan(&accountID); err != nil {
			return true
		}
	} else {
		return true
	}
	return s.security <= s.accountSecurityLevel(ctx, accountID)
}

// handleCharacterChangeAccount mirrors HandleCharacterChangeAccountCommand
// (cs_character.cpp:466): the target is kicked, CHAR_UPD_ACCOUNT_BY_GUID
// moves the row, and the destination account's character count is capped at
// CONFIG_CHARACTERS_PER_REALM. The sCharacterCache account-id update has no
// Go bridge: the row read above is the source of truth.
func (s *session) handleCharacterChangeAccount(ctx context.Context, args []string) bool {
	t, rest, fail := s.resolveCharacterTargetFromTarget(ctx, args)
	if fail != resolveOK {
		if fail == resolveFailMessage {
			s.sendSysMessage("Player not found.")
			return true
		}
		return false
	}
	if len(rest) != 1 {
		return false
	}
	accountName := rest[0]
	newAccountID := s.accountIDByName(ctx, accountName)
	if newAccountID == 0 {
		// AccountIdentifier::TryConsume (ChatCommandTags.cpp:62-83) tries
		// the name first, then a base-10 numeric account id with a GetName
		// existence check.
		if id64, err := strconv.ParseUint(accountName, 10, 32); err == nil {
			if name := s.accountNameByID(ctx, uint32(id64)); name != "" {
				newAccountID = uint32(id64)
				accountName = name
			}
		}
	}
	if newAccountID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", rest[0]))
		return true
	}
	chars := s.server.CharactersStore
	if chars == nil || chars.DB == nil {
		return false
	}
	var oldAccountID uint32
	if err := chars.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.guid).Scan(&oldAccountID); err != nil {
		s.sendSysMessage("Player not found.")
		return true
	}
	if newAccountID == oldAccountID {
		return true // C++: nothing to do
	}
	var charCount int
	_ = chars.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE account = ?", newAccountID).Scan(&charCount)
	if charCount >= int(s.server.Config.CharactersPerRealm) {
		s.sendSysMessage(fmt.Sprintf("Account %s (%d) has too many characters.", accountName, newAccountID))
		return true
	}
	if t.online != nil {
		s.kickSession(t.online)
	}
	_, _ = chars.ExecStatement(ctx, database.StatementID("CHAR_UPD_ACCOUNT_BY_GUID"), newAccountID, t.guid)
	s.sendSysMessage(fmt.Sprintf("Changed ownership of player %s from account %d to account %s.", t.name, oldAccountID, accountName))
	return true
}

// handleCharacterReputation mirrors HandleCharacterReputationCommand
// (cs_character.cpp:522) for connected targets: one line per known faction
// with rank name and standing. The faction name resolves via the Faction DBC
// (enUS, field 23); the session-locale name has no Go bridge (D7). Offline
// targets are rejected exactly like the C++ IsConnected check.
func (s *session) handleCharacterReputation(ctx context.Context, args []string) bool {
	t, rest, ok := s.resolveCharacterTarget(ctx, args)
	if !ok {
		return true
	}
	if len(rest) != 0 {
		return false
	}
	if t.online == nil || t.online.player == nil {
		s.sendSysMessage("Player not found.")
		return true
	}
	rankNames := [...]string{"Hated", "Hostile", "Unfriendly", "Neutral", "Friendly", "Honored", "Revered", "Exalted"}
	for _, rep := range t.online.player.Reputations {
		standing := int64(rep.Base) + int64(rep.Standing)
		rank := reputationRank(standing)
		// FactionEntry.Name is the enUS string at field 23
		// (DBCStructure.h:671); a missing entry prints "#Not found#" like
		// the C++ (cs_character.cpp:545). The link + locale-word shape
		// follows handleLookupFaction.
		factionName := "#Not found#"
		if s.server != nil && s.server.Data != nil {
			if file, fileErr := s.server.Data.File("Faction"); fileErr == nil && file != nil {
				if record, ok := file.Find(rep.FactionID); ok {
					if name, nerr := record.String(23); nerr == nil && name != "" {
						factionName = name
					}
				}
			}
		}
		line := fmt.Sprintf("%d - |cffffffff|Hfaction:%d|h[%s enUS]|h|r %s (%d)", rep.FactionID, rep.FactionID, factionName, rankNames[rank], standing)
		if rep.Flags&factionFlagVisible != 0 {
			line += " visible"
		}
		if rep.Flags&factionFlagAtWar != 0 {
			line += " at war"
		}
		if rep.Flags&factionFlagPeaceForced != 0 {
			line += " peace forced"
		}
		if rep.Flags&factionFlagHidden != 0 {
			line += " hidden"
		}
		if rep.Flags&factionFlagInvisibleForced != 0 {
			line += " invisible forced"
		}
		if rep.Flags&factionFlagInactive != 0 {
			line += " inactive"
		}
		s.sendSysMessage(line)
	}
	return true
}

// deletedCharacterInfo mirrors character_commandscript::DeletedInfo
// (cs_character.cpp:89): deleteInfos_Account stores the numeric account id.
type deletedCharacterInfo struct {
	guid        uint64
	name        string
	accountID   uint32
	accountName string
	deleteDate  int64
}

// handleCharacterDeleted dispatches ".character deleted
// delete|list|restore|old" (cs_character.cpp characterDeletedCommandTable),
// gating each nested arm on its RBAC permission.
func (s *session) handleCharacterDeleted(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	// C++ TryExecuteCommand descends token-by-token through every subtable
	// with prefix matching (ChatCommand.cpp:246); resolve() stops after the
	// first subcommand token, so the nested token is canonicalized here
	// (exact-name priority, ambiguity lists the matches).
	sub := strings.ToLower(args[0])
	matches := make([]string, 0, 2)
	for _, name := range []string{"delete", "list", "restore", "old"} {
		if strings.HasPrefix(name, sub) {
			matches = append(matches, name)
		}
	}
	for _, candidate := range matches {
		if candidate == sub {
			matches = []string{candidate}
			break
		}
	}
	if len(matches) == 0 {
		return false
	}
	if len(matches) > 1 {
		s.sendSysMessage("There are multiple commands matching '" + sub + "'. Did you mean:")
		for _, candidate := range matches {
			s.sendSysMessage(candidate)
		}
		return true
	}
	sub = matches[0]
	rest := args[1:]
	var perm uint32
	switch sub {
	case "delete":
		perm = permissionCommandCharacterDeletedDelete
	case "list":
		perm = permissionCommandCharacterDeletedList
	case "restore":
		perm = permissionCommandCharacterDeletedRestore
	case "old":
		perm = permissionCommandCharacterDeletedOld
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "delete":
		return s.handleCharacterDeletedDelete(ctx, rest)
	case "list":
		return s.handleCharacterDeletedList(ctx, rest)
	case "restore":
		return s.handleCharacterDeletedRestore(ctx, rest)
	case "old":
		return s.handleCharacterDeletedOld(ctx, rest)
	}
	return false
}

// deletedCharacterInfoList mirrors GetDeletedCharacterInfoList
// (cs_character.cpp:114): a numeric needle searches by guid, anything else by
// normalized name, an empty needle lists every deleted character. The bool
// reports false only for an unnormalizable needle (C++ :122 returns false
// silently); a valid needle with no matches returns an empty list with true.
func (s *session) deletedCharacterInfoList(ctx context.Context, needle string) ([]deletedCharacterInfo, bool) {
	chars := s.server.CharactersStore
	if chars == nil {
		return nil, true
	}
	var rows *sql.Rows
	var err error
	switch {
	case needle == "":
		rows, err = chars.QueryStatement(ctx, database.StatementID("CHAR_SEL_CHAR_DEL_INFO"))
	case isNumericString(needle):
		rows, err = chars.QueryStatement(ctx, database.StatementID("CHAR_SEL_CHAR_DEL_INFO_BY_GUID"), needle)
	default:
		if name := normalizePlayerName(needle); name == "" {
			return nil, false
		} else {
			rows, err = chars.QueryStatement(ctx, database.StatementID("CHAR_SEL_CHAR_DEL_INFO_BY_NAME"), name)
		}
	}
	if err != nil {
		return nil, true
	}
	defer rows.Close()
	var found []deletedCharacterInfo
	for rows.Next() {
		var info deletedCharacterInfo
		var deleteAccount sql.NullInt64
		if err := rows.Scan(&info.guid, &info.name, &deleteAccount, &info.deleteDate); err != nil {
			continue
		}
		// deleteInfos_Account is stored numeric by CHAR_UPD_DELETE_INFO;
		// NULL reads as 0 like fields[2].GetUInt32() (cs_character.cpp:145),
		// and the row is still listed with an empty account name.
		if deleteAccount.Valid {
			info.accountID = uint32(deleteAccount.Int64)
		}
		info.accountName = s.accountNameByID(ctx, info.accountID)
		found = append(found, info)
	}
	return found, true
}

// isNumericString mirrors TrinityCore isNumeric for the deleted-list needle.
func isNumericString(str string) bool {
	if str == "" {
		return false
	}
	for i := 0; i < len(str); i++ {
		if str[i] < '0' || str[i] > '9' {
			return false
		}
	}
	return true
}

// sendDeletedCharacterList mirrors HandleCharacterDeletedListHelper
// (cs_character.cpp:167): one line per deleted character. The bars/header
// print only for the console in C++ (!handler->GetSession()), so the in-game
// path prints just the lines.
func (s *session) sendDeletedCharacterList(found []deletedCharacterInfo) {
	for _, info := range found {
		accountName := info.accountName
		if accountName == "" {
			accountName = "<Not existing>"
		}
		dateStr := time.Unix(info.deleteDate, 0).UTC().Format("2006-01-02 15:04:05")
		s.sendSysMessage(fmt.Sprintf("%d - %s [%s (%d)] deleted %s", info.guid, info.name, accountName, info.accountID, dateStr))
	}
}

// handleCharacterDeletedList mirrors HandleCharacterDeletedListCommand
// (cs_character.cpp:581).
func (s *session) handleCharacterDeletedList(ctx context.Context, args []string) bool {
	// The needle is Optional in C++ (HandleCharacterDeletedListCommand,
	// cs_character.cpp:581), but a second token fails the
	// whole-string-consumed check (ChatCommand.h:112).
	if len(args) > 1 {
		return false
	}
	needle := ""
	if len(args) > 0 {
		needle = args[0]
	}
	found, ok := s.deletedCharacterInfoList(ctx, needle)
	if !ok {
		return false
	}
	if len(found) == 0 {
		s.sendSysMessage("No deleted characters found.")
		return true
	}
	s.sendDeletedCharacterList(found)
	return true
}

// handleCharacterDeletedRestore mirrors HandleCharacterDeletedRestoreCommand
// (cs_character.cpp:614): every match is restored unless a new name is given,
// which requires exactly one match. The restore skips characters whose
// account no longer exists, whose account is full (>= 10 characters, the C++
// hardcoded cap), or whose name is taken by a live character.
func (s *session) handleCharacterDeletedRestore(ctx context.Context, args []string) bool {
	if len(args) == 0 || len(args) > 3 {
		return false
	}
	needle := args[0]
	found, ok := s.deletedCharacterInfoList(ctx, needle)
	if !ok {
		return false
	}
	if len(found) == 0 {
		s.sendSysMessage("No deleted characters found.")
		return true
	}
	// D3(a): the new account is parsed before the handler runs in C++
	// (AccountIdentifier::TryConsume), so a bad account errors before the
	// "Restoring…" header and list print.
	var newAccountID uint32
	var newAccountName string
	if len(args) > 2 {
		newAccountID = s.accountIDByName(ctx, args[2])
		if newAccountID == 0 {
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[2]))
			return true
		}
		newAccountName = args[2]
	}
	s.sendSysMessage("Restoring deleted characters:")
	s.sendDeletedCharacterList(found)
	if len(args) == 1 {
		for _, info := range found {
			s.restoreDeletedCharacter(ctx, info)
		}
		return true
	}
	if len(found) != 1 {
		s.sendSysMessage("Rename requires exactly one matching character.")
		return true
	}
	info := found[0]
	// The C++ assigns the new name verbatim (cs_character.cpp:614-655);
	// normalizePlayerName here would restore "bob" as "Bob".
	info.name = args[1]
	if len(args) > 2 {
		info.accountID = newAccountID
		info.accountName = newAccountName
	}
	s.restoreDeletedCharacter(ctx, info)
	return true
}

// restoreDeletedCharacter mirrors HandleCharacterDeletedRestoreHelper
// (cs_character.cpp:205).
func (s *session) restoreDeletedCharacter(ctx context.Context, info deletedCharacterInfo) {
	if info.accountName == "" {
		s.sendSysMessage(fmt.Sprintf("Skipping %s (%d): account %d does not exist.", info.name, info.guid, info.accountID))
		return
	}
	var charCount int
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE account = ?", info.accountID).Scan(&charCount)
	if charCount >= 10 {
		s.sendSysMessage(fmt.Sprintf("Skipping %s (%d): account %d is full.", info.name, info.guid, info.accountID))
		return
	}
	var taken int
	if row, err := s.server.CharactersStore.QueryRowStatement(ctx, database.StatementID("CHAR_SEL_CHECK_NAME"), info.name); err == nil && row.Scan(&taken) == nil {
		s.sendSysMessage(fmt.Sprintf("Skipping %s (%d): name is taken by a live character.", info.name, info.guid))
		return
	}
	_, _ = s.server.CharactersStore.ExecStatement(ctx, database.StatementID("CHAR_UPD_RESTORE_DELETE_INFO"), info.name, info.accountID, info.guid)
	// C++ HandleCharacterDeletedRestoreHelper prints nothing on success.
}

// handleCharacterDeletedDelete mirrors HandleCharacterDeletedDeleteCommand
// (cs_character.cpp:672): every match is permanently wiped via the
// Player::DeleteFromDB path (mail return sweep + owned-state cleanup).
func (s *session) handleCharacterDeletedDelete(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	found, ok := s.deletedCharacterInfoList(ctx, args[0])
	if !ok {
		return false
	}
	if len(found) == 0 {
		s.sendSysMessage("No deleted characters found.")
		return true
	}
	s.sendSysMessage("Deleting characters:")
	s.sendDeletedCharacterList(found)
	for _, info := range found {
		s.deleteCharacterFromDB(ctx, info.guid, 0)
	}
	return true
}

// handleCharacterDeletedOld mirrors HandleCharacterDeletedOldCommand
// (cs_character.cpp:706): permanently wipes characters deleted more than
// $days ago. The C++ CONFIG_CHARDELETE_KEEP_DAYS config has no Go bridge,
// so the days argument is required.
func (s *session) handleCharacterDeletedOld(ctx context.Context, args []string) bool {
	if len(args) != 1 {
		return false
	}
	// C++ days is Optional<uint16> parsed base-0 (StringTo<uint16>, base 0);
	// an explicit 0 is valid (DeleteOldCharacters(0) wipes every deleted
	// character — the keep-days config only gates the no-arg form, D3).
	daysVal, ok := parseCppBase0Int(args[0])
	if !ok || daysVal < 0 || daysVal > math.MaxUint16 {
		s.sendSysMessage("Syntax: .character deleted old $days")
		return true
	}
	days := int(daysVal)
	cutoff := time.Now().Unix() - int64(days)*86400
	// CHAR_SEL_CHAR_OLD_CHARS selects the guid AND the owning account, like
	// Player::DeleteOldCharacters (Player.cpp:4585-4600) — the account feeds
	// the doomed-mail rehoming in DeleteFromDB instead of 0.
	rows, err := s.server.CharactersStore.QueryStatement(ctx, database.StatementID("CHAR_SEL_CHAR_OLD_CHARS"), cutoff)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint64
		var account sql.NullInt64
		if rows.Scan(&guid, &account) != nil {
			continue
		}
		var accountID uint32
		if account.Valid {
			accountID = uint32(account.Int64)
		}
		s.deleteCharacterFromDB(ctx, guid, accountID)
	}
	// C++ prints nothing and returns true (:717); the Go success line is
	// dropped (D4).
	return true
}

// deleteCharacterFromDB mirrors the Player::DeleteFromDB sweep used by the
// erase and deleted-delete arms: COD mails return to senders, owned state is
// wiped, then the character row goes away. The C++ realm-char-count update
// (sWorld->UpdateRealmCharCount) has no Go bridge.
func (s *session) deleteCharacterFromDB(ctx context.Context, guid uint64, accountID uint32) {
	chars := s.server.CharactersStore
	if chars == nil || chars.DB == nil {
		return
	}
	// The mail-return write phase is pure database work, but it draws mail IDs
	// from the server's in-memory counter and reports new-mail notifies — both
	// retry-unsafe. IDs are reserved up front (reused identically on a deadlock
	// replay) and notifies fire only after a successful commit, mirroring
	// DatabaseWorkerPool::DirectCommitTransaction's whole-body retry.
	doomed, err := s.loadCharacterDoomedMails(ctx, chars.DB, guid)
	if err != nil {
		return
	}
	mailIDs := s.server.reserveMailIDs(len(doomed))
	var notifyGUIDs []uint64
	txErr := chars.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		nextMailID := 0
		notifies, err := s.rehomeCharacterDoomedMails(txCtx, tx, guid, accountID, doomed, func() uint32 {
			id := mailIDs[nextMailID]
			nextMailID++
			return id
		})
		if err != nil {
			return err
		}
		notifyGUIDs = notifies
		if err := deleteCharacterPrelude(txCtx, tx, guid); err != nil {
			return err
		}
		if err := deleteCharacterOwnedState(txCtx, tx, guid, s.server.Config.DeletedCharacterTicketTrace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(txCtx, "DELETE FROM characters WHERE guid = ?", guid); err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return
	}
	for _, notifyGUID := range notifyGUIDs {
		s.sendMailNotify(notifyGUID)
	}
	s.releaseNpcBotOwnerCache(guid)
}

// handleCharacterErase mirrors HandleCharacterEraseCommand
// (cs_character.cpp:720): the character is kicked if online, then permanently
// wiped via the Player::DeleteFromDB path.
func (s *session) handleCharacterErase(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	t, rest, ok := s.resolveCharacterTarget(ctx, args)
	if !ok {
		return true
	}
	if len(rest) != 0 {
		return false
	}
	var accountID uint32
	if t.online != nil {
		accountID = t.online.accountID
		s.kickSession(t.online)
	} else {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.guid).Scan(&accountID)
	}
	accountName := s.accountNameByID(ctx, accountID)
	s.deleteCharacterFromDB(ctx, t.guid, accountID)
	s.sendSysMessage(fmt.Sprintf("Deleted character %s (%d) of account %s (%d).", t.name, t.guid, accountName, accountID))
	return true
}

// handleCmdLevelup dispatches ".levelup" (cs_character.cpp commandTable), the
// console-hidden alias of the character level arm gated on
// RBAC_PERM_COMMAND_LEVELUP.
func (s *session) handleCmdLevelup(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandLevelup) {
		return false
	}
	return s.handleCharacterLevel(ctx, args)
}

// handleCmdPDump dispatches ".pdump copy|load|write" (cs_character.cpp
// pdumpCommandTable). The PlayerDump writer/reader has no Go bridge, so each
// arm is RBAC-gated and reports the missing bridge honestly.
func (s *session) handleCmdPDump(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	var perm uint32
	switch sub {
	case "copy":
		perm = permissionCommandPDumpCopy
	case "load":
		perm = permissionCommandPDumpLoad
	case "write":
		perm = permissionCommandPDumpWrite
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	s.characterArmBlocked(perm,
		"pdump "+sub+" needs the PlayerDump writer/reader: no dump serialization exists in Go")
	return true
}

// upperOnlyLatin mirrors TrinityCore Utf8ToUpperOnlyLatin: only ASCII
// a-z are uppercased, everything else is passed through unchanged.
// Reference: AccountMgr::CreateAccount / HandleAccountSetPasswordCommand
// (AccountMgr.cpp, cs_account.cpp).
func upperOnlyLatin(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *session) authDB() *sql.DB {
	if s.server == nil || s.server.AuthStore == nil {
		return nil
	}
	return s.server.AuthStore.DB
}

// accountIDByName mirrors AccountMgr::GetId: resolves an account name to
// its id, or 0 when the account does not exist.
func (s *session) accountIDByName(ctx context.Context, name string) uint32 {
	db := s.authDB()
	if db == nil {
		return 0
	}
	var id uint32
	if err := db.QueryRowContext(ctx, "SELECT id FROM account WHERE UPPER(username) = UPPER(?)", upperOnlyLatin(name)).Scan(&id); err != nil {
		return 0
	}
	return id
}

// accountNameByID mirrors AccountMgr::GetName.
func (s *session) accountNameByID(ctx context.Context, id uint32) string {
	db := s.authDB()
	if db == nil {
		return ""
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT username FROM account WHERE id = ?", id).Scan(&name); err != nil {
		return ""
	}
	return name
}

// accountSecurityLevel mirrors AccountMgr::GetSecurity (AccountMgr.cpp:285):
// LOGIN_GET_GMLEVEL_BY_REALMID returns the realm-specific row first
// (ORDER BY RealmID DESC), falling back to the all-realms (-1) row, or
// SEC_PLAYER when neither exists. MAX over both rows would wrongly promote
// a global GM on a realm where they were explicitly demoted (same caveat as
// the package-level accountSecurityLevel in server.go).
func (s *session) accountSecurityLevel(ctx context.Context, id uint32) uint8 {
	db := s.authDB()
	if db == nil {
		return 0
	}
	var level uint8
	if err := db.QueryRowContext(ctx,
		"SELECT SecurityLevel FROM account_access WHERE AccountID = ? AND (RealmID = ? OR RealmID = -1) ORDER BY RealmID DESC LIMIT 1",
		id, s.server.RealmID).Scan(&level); err != nil {
		return 0
	}
	return level
}

// canModifyAccount mirrors the HasLowerSecurityAccount guard used by the
// .account set commands: the target must have strictly lower security than
// the handler (which also rejects self-application).
func (s *session) canModifyAccount(ctx context.Context, targetID uint32) bool {
	if s.accountSecurityLevel(ctx, targetID) >= s.security {
		s.sendSysMessage("Your security level is too low.")
		return false
	}
	return true
}

// checkAccountPassword mirrors AccountMgr::CheckPassword: the SRP6 verifier
// stored for the account must validate against the supplied password.
// passwordChangeSecurity mirrors sWorld->getIntConfig(CONFIG_ACC_PASSCHANGESEC)
// (World.cpp:1442): 0 = PW_NONE, 1 = PW_EMAIL, 2 = PW_RBAC (AccountMgr.h:36-38).
// With no server config the default PW_NONE applies.
func (s *session) passwordChangeSecurity() int32 {
	if s.server == nil {
		return 0
	}
	return s.server.Config.AccountPassChangeSecurity
}

// checkAccountEmail mirrors AccountMgr::CheckEmail as used by the account
// handlers (cs_account.cpp:455,517): the stored and supplied emails compare
// after upperOnlyLatin folding.
func (s *session) checkAccountEmail(ctx context.Context, accountID uint32, email string) bool {
	db := s.authDB()
	if db == nil {
		return false
	}
	var current string
	if err := db.QueryRowContext(ctx, "SELECT email FROM account WHERE id = ?", accountID).Scan(&current); err != nil {
		return false
	}
	return upperOnlyLatin(current) == upperOnlyLatin(email)
}

func (s *session) checkAccountPassword(ctx context.Context, accountID uint32, password string) bool {
	name := s.accountNameByID(ctx, accountID)
	if name == "" {
		return false
	}
	db := s.authDB()
	if db == nil {
		return false
	}
	var salt, verifier []byte
	if err := db.QueryRowContext(ctx, "SELECT salt, verifier FROM account WHERE id = ?", accountID).Scan(&salt, &verifier); err != nil {
		return false
	}
	if len(salt) != crypto.SRP6SaltLength || len(verifier) != crypto.SRP6VerifierLength {
		return false
	}
	var saltArr [crypto.SRP6SaltLength]byte
	var verifierArr [crypto.SRP6VerifierLength]byte
	copy(saltArr[:], salt)
	copy(verifierArr[:], verifier)
	return crypto.CheckLogin(upperOnlyLatin(name), upperOnlyLatin(password), saltArr, verifierArr)
}

// changeAccountPassword mirrors AccountMgr::ChangePassword: stores a fresh
// SRP6 salt/verifier pair for the account. Returns "" on success, otherwise
// the message to send.
func (s *session) changeAccountPassword(ctx context.Context, accountID uint32, newPassword string) string {
	name := s.accountNameByID(ctx, accountID)
	if name == "" {
		return "Account does not exist."
	}
	if len([]rune(newPassword)) > 16 { // MAX_PASS_STR (AccountMgr.h)
		return "Password too long."
	}
	salt, verifier, err := crypto.MakeRegistrationData(upperOnlyLatin(name), upperOnlyLatin(newPassword))
	if err != nil {
		return "Could not change password."
	}
	if _, err := s.authDB().ExecContext(ctx, "UPDATE account SET salt = ?, verifier = ? WHERE id = ?", salt[:], verifier[:], accountID); err != nil {
		return "Could not change password."
	}
	return ""
}

// changeAccountEmail mirrors AccountMgr::ChangeEmail / ChangeRegEmail:
// writes the email or registration email column. Returns "" on success.
func (s *session) changeAccountEmail(ctx context.Context, accountID uint32, newEmail string, regMail bool) string {
	if s.accountNameByID(ctx, accountID) == "" {
		return "Account does not exist."
	}
	if len([]rune(newEmail)) > 64 { // MAX_EMAIL_STR (AccountMgr.h)
		return "Email too long."
	}
	column := "email"
	if regMail {
		column = "reg_mail"
	}
	if _, err := s.authDB().ExecContext(ctx, "UPDATE account SET "+column+" = ? WHERE id = ?", upperOnlyLatin(newEmail), accountID); err != nil {
		return "Could not change email."
	}
	return ""
}

// remoteIP returns the session's remote address without the port, like
// WorldSession::GetRemoteAddress.
func (s *session) remoteIP() string {
	if s.conn == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(s.conn.RemoteAddr().String())
	if err != nil {
		return s.conn.RemoteAddr().String()
	}
	return host
}

func (s *session) handleCmdAccount(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccount) {
		return false
	}

	if len(args) == 0 {
		// HandleAccountCommand (cs_account.cpp:564): bare ".account" shows
		// the session security level, the password-change security type
		// (CONFIG_ACC_PASSCHANGESEC), the RBAC email-required line when
		// PW_RBAC and the caller holds RBAC_PERM_EMAIL_CONFIRM_FOR_PASS_CHANGE
		// (49), and the account's own email only to holders of
		// RBAC_PERM_MAY_CHECK_OWN_EMAIL (50).
		s.sendSysMessage(fmt.Sprintf("Your account security level: %d.", s.security))
		pwConfig := s.passwordChangeSecurity()
		switch pwConfig {
		case 0: // PW_NONE
			s.sendSysMessage("Lowest level: No Email input required.")
		case 1: // PW_EMAIL
			s.sendSysMessage("Highest level: Email input required.")
		case 2: // PW_RBAC
			s.sendSysMessage("Special level: Your account may require email input depending on settings. That is the case if another line is printed.")
		default:
			s.sendSysMessage("Unknown security level: Config error?")
		}
		if pwConfig == 2 && s.commandAllowed(ctx, permissionEmailConfirmForPassChange) {
			// LANG_RBAC_EMAIL_REQUIRED; the trinity_string text is unseeded
			// in-tree, so plain prose stands in like the other LANG notices.
			s.sendSysMessage("Email input is required to change your password.")
		}
		if s.commandAllowed(ctx, permissionMayCheckOwnEmail) {
			if db := s.authDB(); db != nil {
				var email string
				if err := db.QueryRowContext(ctx, "SELECT email FROM account WHERE id = ?", s.accountID).Scan(&email); err == nil {
					s.sendSysMessage(fmt.Sprintf("Email: %s", email))
				}
			}
		}
		return true
	}
	switch strings.ToLower(args[0]) {
	case "set":
		return s.handleCmdAccountSet(ctx, args[1:])
	case "addon":
		return s.handleCmdAccountAddon(ctx, args[1:])
	case "email":
		return s.handleCmdAccountEmail(ctx, args[1:])
	case "password":
		return s.handleCmdAccountPassword(ctx, args[1:])
	case "lock":
		return s.handleCmdAccountLock(ctx, args[1:])
	case "create":
		return s.handleCmdAccountCreate(ctx, args[1:])
	case "delete":
		return s.handleCmdAccountDelete(ctx, args[1:])
	case "onlinelist":
		if len(args) > 1 {
			return false
		}
		return s.handleCmdAccountOnlineList(ctx)
	case "2fa":
		return s.handleCmdAccount2FA(ctx, args[1:])
	default:
		return false
	}
}

// handleCmdAccountSet dispatches the ".account set" subgroup
// (cs_account.cpp accountSetCommandTable).
func (s *session) handleCmdAccountSet(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "gmlevel", "seclevel":
		return s.handleCmdAccountSetSecLevel(ctx, args[1:])
	case "addon":
		return s.handleCmdAccountSetAddon(ctx, args[1:])
	case "password":
		return s.handleCmdAccountSetPassword(ctx, args[1:])
	case "2fa":
		return s.handleCmdAccountSet2FA(ctx, args[1:])
	default:
		if len(args) >= 2 && strings.ToLower(args[0]) == "sec" {
			switch strings.ToLower(args[1]) {
			case "email", "regmail":
				return s.handleCmdAccountSetSecEmail(ctx, "sec "+strings.ToLower(args[1]), args[2:])
			}
		}
		return false
	}
}

// accountSecurityForRealm mirrors AccountMgr::GetSecurity (AccountMgr.cpp:285):
// LOGIN_GET_GMLEVEL_BY_REALMID returns the realm-specific row first
// (ORDER BY RealmID DESC), falling back to the all-realms (-1) row, or
// SEC_PLAYER when neither exists. This differs from accountSecurityLevel's
// MAX over both rows, which overstates a realm-specific level below the
// global one.
func (s *session) accountSecurityForRealm(ctx context.Context, id uint32, realmID int32) uint8 {
	db := s.authDB()
	if db == nil {
		return 0
	}
	var level uint8
	if err := db.QueryRowContext(ctx,
		"SELECT SecurityLevel FROM account_access WHERE AccountID = ? AND (RealmID = ? OR RealmID = -1) ORDER BY RealmID DESC LIMIT 1",
		id, realmID).Scan(&level); err != nil {
		return 0
	}
	return level
}

// handleCmdAccountSetSecLevel mirrors HandleAccountSetSecLevelCommand
// (cs_account.cpp:657): ".account set gmlevel|seclevel [<account>] <level>
// [<realm>]". The account form is optional in C++ (selected player's account
// via getSelectedPlayer, which falls back to self); the realm defaults to -1
// (all realms), not the current realm. Security compares use the
// realm-aware GetSecurity above, the realm -1 form carries the cross-realm
// higher-rank test (LOGIN_SEL_ACCOUNT_ACCESS_SECLEVEL_TEST), and the write
// follows UpdateAccountAccess (delete the account's access rows for the
// realm scope, then insert only when the level is non-zero).
func (s *session) handleCmdAccountSetSecLevel(ctx context.Context, args []string) bool {
	// The bool reports C++ handler success: the silent-false paths (no
	// selected player) return false so dispatchCommand shows the node's
	// help listing (ChatCommand.cpp:296-309); paths that print a message
	// return true. The RBAC denial also returns false: C++ never invokes
	// a denied leaf, the walk stops at the parent and shows its help.
	if !s.commandAllowed(ctx, permissionCommandAccountSetSecLevel) {
		return false
	}

	var targetID uint32
	var targetAcct string
	var sec uint64
	var realmID int32 = -1
	switch len(args) {
	case 1:
		target := s.lookupSelectedPlayerOrSelf()
		if target == nil {
			return false // C++ getSelectedPlayer null -> return false, silently
		}
		targetID = target.accountID
		targetAcct = s.accountNameByID(ctx, targetID)
		v, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set gmlevel [<account>] <level> [<realm>]")
			return true
		}
		sec = v
	case 2, 3:
		targetAcct = args[0]
		v, err := strconv.ParseUint(args[1], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set gmlevel [<account>] <level> [<realm>]")
			return true
		}
		sec = v
		if len(args) == 3 {
			r, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil {
				s.sendSysMessage("Syntax: .account set gmlevel [<account>] <level> [<realm>]")
				return true
			}
			realmID = int32(r)
		}
		targetID = s.accountIDByName(ctx, targetAcct)
		if targetID == 0 {
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", targetAcct))
			return true
		}
	default:
		return false
	}
	if sec >= 4 { // SEC_CONSOLE = 4; HandleAccountSetSecLevelCommand rejects >= SEC_CONSOLE
		s.sendSysMessage("Incorrect value.") // LANG_BAD_VALUE (115)
		return true
	}
	// can set security level only for a target with less security and to a
	// level less than we have (also restricts setting our own security).
	// Without an auth DB the session security stands in for the invoker's
	// realm-aware level, as the previous implementation did.
	playerSecurity := s.security
	var targetSecurity uint8
	if s.authDB() != nil {
		playerSecurity = s.accountSecurityForRealm(ctx, s.accountID, realmID)
		targetSecurity = s.accountSecurityForRealm(ctx, targetID, realmID)
	}
	if targetSecurity >= playerSecurity || uint8(sec) >= playerSecurity {
		s.sendSysMessage("Your security level is too low.")
		return true
	}
	// Check and abort if the target gm has a higher rank on one of the
	// realms and the new realm is -1.
	if realmID == -1 && playerSecurity != 4 { // IsConsoleAccount
		if db := s.authDB(); db != nil {
			var one int
			if err := db.QueryRowContext(ctx,
				"SELECT 1 FROM account_access WHERE AccountID = ? AND SecurityLevel > ?",
				targetID, sec).Scan(&one); err == nil {
				s.sendSysMessage("Your security level is too low.")
				return true
			}
		}
	}
	if realmID < -1 {
		s.sendSysMessage("Invalid realm ID.")
		return true
	}
	if db := s.authDB(); db != nil {
		if realmID == -1 {
			_, _ = db.ExecContext(ctx, "DELETE FROM account_access WHERE AccountID = ?", targetID)
		} else {
			_, _ = db.ExecContext(ctx, "DELETE FROM account_access WHERE AccountID = ? AND (RealmID = ? OR RealmID = -1)", targetID, realmID)
		}
		if sec != 0 {
			_, _ = db.ExecContext(ctx, "INSERT INTO account_access (AccountID, SecurityLevel, RealmID) VALUES (?, ?, ?)", targetID, sec, realmID)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Security level for %s set to %d.", targetAcct, sec))
	return true
}

// handleCmdAccountSetAddon mirrors HandleAccountSetAddonCommand
// (cs_account.cpp:604): ".account set addon [<account>] <expansion>".
func (s *session) handleCmdAccountSetAddon(ctx context.Context, args []string) bool {
	// The bool reports C++ handler success: the silent-false paths (no
	// selected player, lower-security target, expansion above
	// CONFIG_EXPANSION) return false so dispatchCommand shows the node's
	// help listing (ChatCommand.cpp:296-309); paths that print a message
	// return true. The RBAC denial also returns false: C++ never invokes
	// a denied leaf, the walk stops at the parent and shows its help.
	if !s.commandAllowed(ctx, permissionCommandAccountSetAddon) {
		return false
	}

	var targetID uint32
	var targetName string
	var expansion uint8
	switch len(args) {
	case 1:
		// No account form: C++ uses getSelectedPlayer (Chat.cpp:300: no
		// selection resolves to the invoker), not unconditionally self.
		target := s.lookupSelectedPlayerOrSelf()
		if target == nil {
			return false // C++ getSelectedPlayer null -> return false, silently
		}
		targetID = target.accountID
		targetName = s.accountNameByID(ctx, targetID)
		v, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set addon [<account>] <expansion>")
			return true
		}
		expansion = uint8(v)
	case 2:
		targetID = s.accountIDByName(ctx, args[0])
		if targetID == 0 {
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
			return true
		}
		targetName = args[0]
		v, err := strconv.ParseUint(args[1], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set addon [<account>] <expansion>")
			return true
		}
		expansion = uint8(v)
	default:
		return false
	}
	if targetID != s.accountID && !s.canModifyAccount(ctx, targetID) {
		// canModifyAccount prints "Your security level is too low."
		// (HasLowerSecurityAccount + SetSentErrorMessage, Chat.cpp:74-102),
		// so the leg is message + true, not message + help.
		return true
	}
	if s.server != nil && uint32(expansion) > s.server.Config.Expansion { // CONFIG_EXPANSION
		return false
	}
	if db := s.authDB(); db != nil {
		_, _ = db.ExecContext(ctx, "UPDATE account SET expansion = ? WHERE id = ?", expansion, targetID)
	}
	s.sendSysMessage(fmt.Sprintf("Expansion for account %s [%d] set to %d.", targetName, targetID, expansion))
	return true
}

// handleCmdAccountSetPassword mirrors HandleAccountSetPasswordCommand
// (cs_account.cpp:747): ".account set password <account> <password> <confirm>".
func (s *session) handleCmdAccountSetPassword(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountSetPassword) {
		return false
	}

	if len(args) < 3 {
		return false
	}
	if len(args) > 3 {
		return false
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return true
	}
	if !s.canModifyAccount(ctx, targetID) {
		return true
	}
	if args[1] != args[2] {
		s.sendSysMessage("New passwords do not match.")
		return true
	}
	if msg := s.changeAccountPassword(ctx, targetID, args[1]); msg != "" {
		s.sendSysMessage(msg)
		return true
	}
	s.sendSysMessage("Password changed.")
	return true
}

// handleCmdAccountSetSecEmail mirrors HandleAccountSetEmailCommand /
// HandleAccountSetRegEmailCommand (cs_account.cpp:862/917):
// ".account set sec email|regmail <account> <email> <confirm>".
func (s *session) handleCmdAccountSetSecEmail(ctx context.Context, action string, args []string) bool {
	perm := permissionCommandAccountSetSecEmail
	if action == "sec regmail" {
		perm = permissionCommandAccountSetSecRegmail
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}

	if len(args) < 3 {
		return false
	}
	if len(args) > 3 {
		return false
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return true
	}
	if !s.canModifyAccount(ctx, targetID) {
		return true
	}
	if args[1] != args[2] {
		s.sendSysMessage("New emails do not match.")
		return true
	}
	if msg := s.changeAccountEmail(ctx, targetID, args[1], strings.HasSuffix(action, "regmail")); msg != "" {
		s.sendSysMessage(msg)
		return true
	}
	s.sendSysMessage("Email changed.")
	return true
}

// handleCmdAccountAddon mirrors HandleAccountAddonCommand (cs_account.cpp:217):
// ".account addon <expansion>" sets the caller's own expansion level.
func (s *session) handleCmdAccountAddon(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountAddon) {
		return false
	}

	if len(args) < 1 {
		return false
	}
	expansion, err := strconv.ParseUint(args[0], 10, 8)
	if err != nil {
		s.sendSysMessage("Syntax: .account addon <expansion>")
		return true
	}
	if len(args) > 1 {
		return false
	}
	if s.server != nil && uint32(expansion) > s.server.Config.Expansion { // CONFIG_EXPANSION
		s.sendSysMessage("Invalid expansion value.")
		return true
	}
	if db := s.authDB(); db != nil {
		_, _ = db.ExecContext(ctx, "UPDATE account SET expansion = ? WHERE id = ?", uint8(expansion), s.accountID)
	}
	s.sendSysMessage(fmt.Sprintf("Expansion set to %d.", expansion))
	return true
}

// handleCmdAccountEmail mirrors HandleAccountEmailCommand (cs_account.cpp:430):
// ".account email <oldEmail> <password> <newEmail> <confirmEmail>" changes
// the caller's own email after verifying the old email and password.
func (s *session) handleCmdAccountEmail(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountEmail) {
		return false
	}

	if len(args) < 4 {
		return false
	}
	if len(args) > 4 {
		return false
	}
	oldEmail, password, email, emailConfirm := args[0], args[1], args[2], args[3]
	db := s.authDB()
	if db == nil {
		return true
	}
	var current string
	if err := db.QueryRowContext(ctx, "SELECT email FROM account WHERE id = ?", s.accountID).Scan(&current); err != nil {
		s.sendSysMessage("Wrong email.")
		return true
	}
	if upperOnlyLatin(current) != upperOnlyLatin(oldEmail) { // AccountMgr::CheckEmail
		s.sendSysMessage("Wrong email.")
		return true
	}
	if !s.checkAccountPassword(ctx, s.accountID, password) {
		s.sendSysMessage("Wrong old password.")
		return true
	}
	if email == oldEmail { // cs_account.cpp:455: raw compare, not case-folded (the CheckEmail verification above correctly folds both sides)
		s.sendSysMessage("Old email is the new email.")
		return true
	}
	if email != emailConfirm {
		s.sendSysMessage("New emails do not match.")
		return true
	}
	if msg := s.changeAccountEmail(ctx, s.accountID, email, false); msg != "" {
		s.sendSysMessage(msg)
		return true
	}
	s.sendSysMessage("Email changed.")
	return true
}

// handleCmdAccountPassword mirrors HandleAccountPasswordCommand
// (cs_account.cpp:499): ".account password <old> <new> <confirm> [email]"
// changes the caller's own password after verifying the old one. With
// CONFIG_ACC_PASSCHANGESEC = PW_EMAIL (1), or PW_RBAC (2) when the caller
// holds RBAC_PERM_EMAIL_CONFIRM_FOR_PASS_CHANGE (49), the optional fourth
// argument must match the account email first (cs_account.cpp:517-528);
// the email check runs before the new-password confirmation.
func (s *session) handleCmdAccountPassword(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountPassword) {
		return false
	}

	if len(args) < 3 {
		return false
	}
	if len(args) > 4 {
		return false
	}
	oldPassword, newPassword, confirmPassword := args[0], args[1], args[2]
	if !s.checkAccountPassword(ctx, s.accountID, oldPassword) {
		s.sendSysMessage("Wrong old password.")
		return true
	}
	pwConfig := s.passwordChangeSecurity()
	if pwConfig == 1 || (pwConfig == 2 && s.commandAllowed(ctx, permissionEmailConfirmForPassChange)) {
		confirmEmail := ""
		if len(args) > 3 {
			confirmEmail = args[3]
		}
		if !s.checkAccountEmail(ctx, s.accountID, confirmEmail) {
			s.sendSysMessage("Wrong email.")
			return true
		}
	}
	if newPassword != confirmPassword {
		s.sendSysMessage("New passwords do not match.")
		return true
	}
	if msg := s.changeAccountPassword(ctx, s.accountID, newPassword); msg != "" {
		s.sendSysMessage(msg)
		return true
	}
	s.sendSysMessage("Password changed.")
	return true
}

// handleCmdAccountLock mirrors HandleAccountLockIpCommand /
// HandleAccountLockCountryCommand (cs_account.cpp:379/409):
// ".account lock ip|country on|off".
// parseAccountLockBool mirrors StringTo<bool> (StringConvert.h:146-164,
// non-strict): 1/y/on/yes/true and 0/n/off/no/false, case-insensitive;
// anything else fails the parse, so the caller shows usage instead of
// silently unlocking.
func parseAccountLockBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "1", "y", "on", "yes", "true":
		return true, true
	case "0", "n", "off", "no", "false":
		return false, true
	}
	return false, false
}

func (s *session) handleCmdAccountLock(ctx context.Context, args []string) bool {
	if len(args) < 2 {
		return false
	}
	if len(args) > 2 {
		return false
	}
	// Lock ip (223) and lock country (222) carry separate permissions
	// (cs_account.cpp accountCommandTable). The subcommand resolves first:
	// C++ shows help for an unknown subcommand before any leaf permission
	// is consulted; only then is that leaf's permission checked.
	lockPerm := permissionCommandAccountLockIP
	switch strings.ToLower(args[0]) {
	case "ip":
	case "country":
		lockPerm = permissionCommandAccountLockCountry
	default:
		return false
	}
	if !s.commandAllowed(ctx, lockPerm) {
		return false
	}
	db := s.authDB()
	if db == nil {
		return true
	}
	state, ok := parseAccountLockBool(args[1])
	if !ok {
		// Unparseable bool token: the framework prints
		// LANG_CMDPARSER_STRING_VALUE_INVALID + SetSentErrorMessage.
		s.sendSysMessage("Syntax: .account lock ip|country on|off")
		return true
	}
	switch strings.ToLower(args[0]) {
	case "ip":
		var locked int
		if state {
			locked = 1
		}
		_, _ = db.ExecContext(ctx, "UPDATE account SET locked = ? WHERE id = ?", locked, s.accountID)
		if state {
			s.sendSysMessage("Account locked to your IP.")
		} else {
			s.sendSysMessage("Account unlocked.")
		}
		return true
	case "country":
		country := "00"
		if state {
			country = ""
			if s.server != nil && s.server.ipLocations != nil {
				country = s.server.ipLocations.Country(s.remoteIP())
			}
			if country == "" {
				s.sendSysMessage("No IP2Location information - account not locked") // cs_account.cpp:393 literal
				return true
			}
		}
		_, _ = db.ExecContext(ctx, "UPDATE account SET lock_country = ? WHERE id = ?", country, s.accountID)
		if state {
			s.sendSysMessage(fmt.Sprintf("Account locked to country %s.", country))
		} else {
			s.sendSysMessage("Account unlocked.")
		}
		return true
	default:
		return false
	}
}

// accountOpResult mirrors TrinityCore's AccountOpResult (AccountMgr.h).
type accountOpResult int

const (
	accountOpOK accountOpResult = iota
	accountOpNameTooLong
	accountOpPassTooLong
	accountOpNameAlreadyExist
	accountOpNameNotExist
	accountOpDBInternalError
)

// handleCmdAccountCreate mirrors HandleAccountCreateCommand (cs_account.cpp:238):
// ".account create <account> <password> [<email>]".
func (s *session) handleCmdAccountCreate(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountCreate) {
		return false
	}

	if len(args) < 2 {
		return false
	}
	if len(args) > 3 {
		return false
	}
	accountName, password := args[0], args[1]
	email := ""
	if len(args) > 2 {
		email = args[2]
	}
	if strings.Contains(accountName, "@") {
		// LANG_ACCOUNT_USE_BNET_COMMANDS (cs_account.cpp:241)
		s.sendSysMessage("Use battlenet account commands for battlenet accounts.")
		return true
	}
	switch s.createAccount(ctx, accountName, password, email) {
	case accountOpOK:
		s.sendSysMessage(fmt.Sprintf("Account %s created.", accountName))
		s.debug("account created", "by", s.accountName, "ip", s.remoteIP(), "account", accountName)
	case accountOpNameTooLong:
		s.sendSysMessage("Account name too long.")
	case accountOpPassTooLong:
		s.sendSysMessage("Password too long.")
	case accountOpNameAlreadyExist:
		s.sendSysMessage("Account already exists.")
	case accountOpDBInternalError:
		// LANG_ACCOUNT_NOT_CREATED_SQL_ERROR (1007), distinct from
		// LANG_ACCOUNT_NOT_CREATED (1008).
		s.sendSysMessage(fmt.Sprintf("Account %s was not created (database error).", accountName))
	default:
		s.sendSysMessage(fmt.Sprintf("Account %s was not created.", accountName))
	}
	return true
}

// createAccount mirrors AccountMgr::CreateAccount (AccountMgr.cpp:45): utf8
// length checks, upper-only-latin normalization, duplicate check, SRP6
// registration data, and the LOGIN_INS_REALM_CHARACTERS_INIT seed row.
func (s *session) createAccount(ctx context.Context, username, password, email string) accountOpResult {
	if utf8.RuneCountInString(username) > 16 { // MAX_ACCOUNT_STR (AccountMgr.h)
		return accountOpNameTooLong
	}
	if utf8.RuneCountInString(password) > 16 { // MAX_PASS_STR (AccountMgr.h)
		return accountOpPassTooLong
	}
	username = upperOnlyLatin(username)
	password = upperOnlyLatin(password)
	email = upperOnlyLatin(email)
	if s.accountIDByName(ctx, username) != 0 {
		return accountOpNameAlreadyExist
	}
	salt, verifier, err := crypto.MakeRegistrationData(username, password)
	if err != nil {
		return accountOpDBInternalError
	}
	if s.server == nil || s.server.AuthStore == nil {
		return accountOpDBInternalError
	}
	// LOGIN_INS_ACCOUNT: (username, salt, verifier, reg_mail, email); C++
	// stores the email in both mail columns.
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_INS_ACCOUNT", username, salt[:], verifier[:], email, email); err != nil {
		return accountOpDBInternalError
	}
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_INS_REALM_CHARACTERS_INIT"); err != nil {
		return accountOpDBInternalError
	}
	return accountOpOK
}

// handleCmdAccountDelete mirrors HandleAccountDeleteCommand (cs_account.cpp:286):
// ".account delete <account>".
func (s *session) handleCmdAccountDelete(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountDelete) {
		return false
	}

	if len(args) < 1 {
		return false
	}
	if len(args) > 1 {
		return false
	}
	accountName := args[0]
	targetID := s.accountIDByName(ctx, accountName)
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
		return true
	}
	// cs_account.cpp: commands can delete only accounts with lower security
	// (this also rejects self-application); mirrors the canModifyAccount
	// HasLowerSecurityAccount guard used by the set subgroup.
	if !s.canModifyAccount(ctx, targetID) {
		return true
	}
	switch s.deleteAccount(ctx, targetID) {
	case accountOpOK:
		s.sendSysMessage(fmt.Sprintf("Account %s deleted.", accountName))
	case accountOpNameNotExist:
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
	case accountOpDBInternalError:
		// LANG_ACCOUNT_NOT_DELETED_SQL_ERROR (1002), distinct from
		// LANG_ACCOUNT_NOT_DELETED (1003).
		s.sendSysMessage(fmt.Sprintf("Account %s was not deleted (database error).", accountName))
	default:
		s.sendSysMessage(fmt.Sprintf("Account %s was not deleted.", accountName))
	}
	return true
}

// deleteAccount mirrors AccountMgr::DeleteAccount (AccountMgr.cpp): kicks
// online characters of the account, wipes each character (Player::DeleteFromDB
// with updateRealmChars=false), clears the account-scoped character tables,
// then deletes the account rows in a login-DB transaction.
func (s *session) deleteAccount(ctx context.Context, accountID uint32) accountOpResult {
	if s.server == nil || s.server.AuthStore == nil || s.server.CharactersStore == nil {
		return accountOpDBInternalError
	}
	var one int
	row, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_BY_ID", accountID)
	if err != nil {
		return accountOpDBInternalError
	}
	if err := row.Scan(&one); err != nil {
		return accountOpNameNotExist
	}
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_CHARS_BY_ACCOUNT_ID", accountID)
	if err != nil {
		return accountOpDBInternalError
	}
	var guids []uint64
	for rows.Next() {
		var guid uint64
		if err := rows.Scan(&guid); err != nil {
			rows.Close()
			return accountOpDBInternalError
		}
		guids = append(guids, guid)
	}
	rows.Close()
	for _, guid := range guids {
		// AccountMgr::DeleteAccount: kick the player if online, then
		// Player::DeleteFromDB(guid, accountId, false) (no realmcharacters
		// update - the account row goes away anyway).
		if sess := s.server.playerSessionForGUID(guid); sess != nil {
			sess.debug("session closed: account deleted", "account", sess.accountName)
			s.server.sessionsMu.Lock()
			delete(s.server.sessions, sess)
			s.server.sessionsMu.Unlock()
			if sess.conn != nil {
				_ = sess.conn.Close()
			}
		}
		chars := s.server.CharactersStore
		doomed, err := s.loadCharacterDoomedMails(ctx, chars.DB, guid)
		if err != nil {
			return accountOpDBInternalError
		}
		mailIDs := s.server.reserveMailIDs(len(doomed))
		var notifyGUIDs []uint64
		txErr := chars.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
			nextMailID := 0
			notifies, err := s.rehomeCharacterDoomedMails(txCtx, tx, guid, accountID, doomed, func() uint32 {
				id := mailIDs[nextMailID]
				nextMailID++
				return id
			})
			if err != nil {
				return err
			}
			notifyGUIDs = notifies
			if err := deleteCharacterPrelude(txCtx, tx, guid); err != nil {
				return err
			}
			if err := deleteCharacterOwnedState(txCtx, tx, guid, s.server.Config.DeletedCharacterTicketTrace); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, "DELETE FROM characters WHERE guid = ?", guid); err != nil {
				return err
			}
			return nil
		})
		if txErr != nil {
			return accountOpDBInternalError
		}
		for _, notifyGUID := range notifyGUIDs {
			s.sendMailNotify(notifyGUID)
		}
		s.releaseNpcBotOwnerCache(guid)
	}
	for _, stmt := range []string{"CHAR_DEL_TUTORIALS", "CHAR_DEL_ACCOUNT_DATA", "CHAR_DEL_CHARACTER_BAN"} {
		if _, err := s.server.CharactersStore.ExecStatement(ctx, database.StatementID(stmt), accountID); err != nil {
			return accountOpDBInternalError
		}
	}
	authStore := s.server.AuthStore
	if err := authStore.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		for _, stmt := range []string{"LOGIN_DEL_ACCOUNT", "LOGIN_DEL_ACCOUNT_ACCESS", "LOGIN_DEL_REALM_CHARACTERS", "LOGIN_DEL_ACCOUNT_BANNED", "LOGIN_DEL_ACCOUNT_MUTED"} {
			if _, err := authStore.ExecStatementTx(txCtx, tx, database.StatementID(stmt), accountID); err != nil {
				return err
			}
		}
		// C++ AccountMgr::DeleteAccount (AccountMgr.cpp:77-150) issues no explicit
		// rbac_account_permissions delete: MySQL's ON DELETE CASCADE FK
		// (fk__rbac_account_permissions__account in sql/mysql/auth.sql) removes them.
		// SQLite leaves PRAGMA foreign_keys off, so without this the rows would
		// orphan; the explicit delete reproduces the cascade end state on both dialects.
		if _, err := tx.ExecContext(txCtx, "DELETE FROM rbac_account_permissions WHERE accountId = ?", accountID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return accountOpDBInternalError
	}
	return accountOpOK
}

// handleCmdAccountOnlineList mirrors HandleAccountOnlineListCommand
// (cs_account.cpp:333): ".account onlinelist".
func (s *session) handleCmdAccountOnlineList(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountOnlineList) {
		return false
	}

	if s.server == nil || s.server.CharactersStore == nil || s.server.AuthStore == nil {
		return true
	}
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_CHARACTER_ONLINE")
	if err != nil {
		// C++ treats a missing result as the empty list
		// (cs_account.cpp:336-341, LANG_ACCOUNT_LIST_EMPTY).
		s.sendSysMessage("No characters online.")
		return true
	}
	defer rows.Close()
	type onlineChar struct {
		name    string
		account uint32
		mapID   uint16
		zone    uint16
	}
	var online []onlineChar
	for rows.Next() {
		var c onlineChar
		if err := rows.Scan(&c.name, &c.account, &c.mapID, &c.zone); err != nil {
			return true
		}
		online = append(online, c)
	}
	if len(online) == 0 {
		s.sendSysMessage("No characters online.")
		return true
	}
	// LANG_ACCOUNT_LIST_BAR_HEADER (1015) + LANG_ACCOUNT_LIST_HEADER (1010)
	// + LANG_ACCOUNT_LIST_BAR (1012) frame the rows, and a final bar closes
	// the list (cs_account.cpp:346-348,377); the "=" bar is the established
	// Go rendering of LANG_ACCOUNT_LIST_BAR (commands_list.go).
	s.sendSysMessage("================================================")
	s.sendSysMessage("Account | Character | LastIP | Map | Zone | Expansion | GM")
	s.sendSysMessage("================================================")
	for _, c := range online {
		// LOGIN_SEL_ACCOUNT_INFO: username, last_ip, SecurityLevel, expansion.
		var username, lastIP string
		var security, expansion sql.NullInt64
		infoRow, infoErr := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_INFO", c.account)
		if infoErr != nil {
			s.sendSysMessage(fmt.Sprintf("Error listing %s.", c.name))
			continue
		}
		if err := infoRow.Scan(&username, &lastIP, &security, &expansion); err != nil {
			s.sendSysMessage(fmt.Sprintf("Error listing %s.", c.name))
			continue
		}
		s.sendSysMessage(fmt.Sprintf("%s | %s | %s | %d | %d | %d | %d",
			username, c.name, lastIP, c.mapID, c.zone, expansion.Int64, security.Int64))
	}
	s.sendSysMessage("================================================") // LANG_ACCOUNT_LIST_BAR (1012), closing frame
	return true
}

// totpSuggestions mirrors the static suggestions map in
// HandleAccount2FASetupCommand (cs_account.cpp:119): per-account pending
// 2FA secrets awaiting token confirmation.
var (
	totpSuggestionsMu sync.Mutex
	totpSuggestions   = make(map[uint32][]byte)
)

// handleCmdAccount2FA dispatches ".account 2fa setup|remove".
func (s *session) handleCmdAccount2FA(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "setup":
		return s.handleCmdAccount2FASetup(ctx, args[1:])
	case "remove":
		return s.handleCmdAccount2FARemove(ctx, args[1:])
	default:
		return false
	}
}

// totpMasterKey mirrors sSecretMgr->GetSecret(SECRET_TOTP_MASTER_KEY) as used
// by the C++ world server's 2FA commands: the world process reads its own
// TOTPMasterSecret config, exactly like worldserver.conf's Secret.TOTPMasterKey.
func (s *session) totpMasterKey() (key [16]byte, present bool) {
	if s.server == nil {
		return key, false
	}
	secret := strings.TrimSpace(s.server.Config.TotpMasterSecret)
	if secret == "" {
		return key, false
	}
	parsed, err := crypto.ParseMasterKey(secret)
	if err != nil {
		return key, false
	}
	return parsed, true
}

// handleCmdAccount2FASetup mirrors HandleAccount2FASetupCommand (cs_account.cpp:85).
func (s *session) handleCmdAccount2FASetup(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccount2FASetup) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	var token *uint32
	if len(args) == 1 {
		t, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			// Unparseable Optional<uint32> token: the framework prints
			// LANG_CMDPARSER_STRING_VALUE_INVALID + SetSentErrorMessage.
			s.sendSysMessage("Invalid token.")
			return true
		}
		t32 := uint32(t)
		token = &t32
	}

	masterKey, masterPresent := s.totpMasterKey()
	if s.server == nil || s.server.AuthStore == nil || !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return true
	}
	accountID := s.accountID
	var existing []byte
	totpRow, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_TOTP_SECRET", accountID)
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	if err := totpRow.Scan(&existing); err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	if len(existing) != 0 {
		s.sendSysMessage("2FA is already set up on this account.")
		return true
	}
	totpSuggestionsMu.Lock()
	secret, ok := totpSuggestions[accountID]
	if !ok {
		secret = make([]byte, 20) // TOTP::RECOMMENDED_SECRET_LENGTH (TOTP.h)
		if _, err := rand.Read(secret); err != nil {
			totpSuggestionsMu.Unlock()
			s.sendSysMessage("Unknown error.")
			return true
		}
		totpSuggestions[accountID] = secret
	}
	totpSuggestionsMu.Unlock()
	if ok && token != nil {
		// Suggestion already existed and a token was supplied: validate it.
		if crypto.ValidateTOTP(secret, *token, time.Now()) {
			stored := append([]byte(nil), secret...)
			if err := crypto.EncryptWithRandomIV(&stored, masterKey); err != nil {
				s.sendSysMessage("Unknown error.")
				return true
			}
			if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", stored, accountID); err != nil {
				s.sendSysMessage("Unknown error.")
				return true
			}
			totpSuggestionsMu.Lock()
			delete(totpSuggestions, accountID)
			totpSuggestionsMu.Unlock()
			s.sendSysMessage("2FA setup complete.")
			return true
		}
		s.sendSysMessage("Invalid token.")
	}
	// New suggestion, or no token specified: output the TOTP parameters.
	s.sendSysMessage(fmt.Sprintf("Suggested 2FA secret: %s", crypto.Base32Encode(secret)))
	return true
}

// handleCmdAccount2FARemove mirrors HandleAccount2FARemoveCommand (cs_account.cpp:149).
func (s *session) handleCmdAccount2FARemove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccount2FARemove) {
		return false
	}
	if len(args) > 1 {
		return false
	}
	var token *uint32
	if len(args) == 1 {
		t, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			// Unparseable Optional<uint32> token: the framework prints
			// LANG_CMDPARSER_STRING_VALUE_INVALID + SetSentErrorMessage.
			s.sendSysMessage("Invalid token.")
			return true
		}
		t32 := uint32(t)
		token = &t32
	}

	masterKey, masterPresent := s.totpMasterKey()
	if s.server == nil || s.server.AuthStore == nil || !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return true
	}
	accountID := s.accountID
	var stored []byte
	totpRow, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_TOTP_SECRET", accountID)
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	if err := totpRow.Scan(&stored); err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	if len(stored) == 0 {
		s.sendSysMessage("2FA is not set up on this account.")
		return true
	}
	if token != nil {
		secret := append([]byte(nil), stored...)
		if err := crypto.DecryptWithTrailingIVAndTag(&secret, masterKey); err != nil {
			s.debug("account 2fa remove: invalid ciphertext", "account", s.accountName)
			s.sendSysMessage("Unknown error.")
			return true
		}
		if crypto.ValidateTOTP(secret, *token, time.Now()) {
			if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", nil, accountID); err != nil {
				s.sendSysMessage("Unknown error.")
				return true
			}
			s.sendSysMessage("2FA removed.")
			return true
		}
		s.sendSysMessage("Invalid token.")
	}
	s.sendSysMessage("You must supply your current token to remove 2FA.")
	return true
}

// handleCmdAccountSet2FA mirrors HandleAccountSet2FACommand (cs_account.cpp:798):
// ".account set 2fa <account> <secret|off>".
func (s *session) handleCmdAccountSet2FA(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandAccountSet2FA) {
		return false
	}

	if len(args) < 2 {
		return false
	}
	if len(args) > 2 {
		return false
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return true
	}
	// HandleAccountSet2FACommand uses the HasLowerSecurityAccount guard.
	if !s.canModifyAccount(ctx, targetID) {
		return true
	}
	if s.server == nil || s.server.AuthStore == nil {
		return true
	}
	if args[1] == "off" {
		if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", nil, targetID); err != nil {
			s.sendSysMessage("Unknown error.")
			return true
		}
		s.sendSysMessage("2FA removed.")
		return true
	}
	masterKey, masterPresent := s.totpMasterKey()
	if !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return true
	}
	decoded, err := crypto.Base32Decode(args[1])
	if err != nil {
		s.sendSysMessage("Invalid 2FA secret.")
		return true
	}
	if len(decoded)+crypto.AESIVSize+crypto.AESTagSize > 128 {
		s.sendSysMessage("2FA secret too long.")
		return true
	}
	stored := append([]byte(nil), decoded...)
	if err := crypto.EncryptWithRandomIV(&stored, masterKey); err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", stored, targetID); err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	s.sendSysMessage(fmt.Sprintf("2FA secret set for account %s.", args[0]))
	return true
}

// handleCmdAchievement processes ".achievement add" (cs_achievement.cpp,
// AddSC_achievement_commandscript): HandleAchievementAddCommand grants the
// given achievement to the selected player (the commander's own player when
// nothing is targeted), mirroring Player::CompletedAchievement through
// completeAchievement. Gated by RBAC_PERM_COMMAND_ACHIEVEMENT_ADD (231);
// Console::No has no Go console path to suppress.
func (s *session) handleCmdAchievement(ctx context.Context, args []string) bool {
	// C++ order: the dispatch-time permission check (IsInvokerVisible) runs
	// before the AchievementEntry* argument parse, so a denied caller with
	// a bad id is denied, not parse-failed; the parse (including the store
	// lookup) runs before the handler body.
	if !s.commandAllowed(ctx, permissionCommandAchievementAdd) {
		return false
	}
	if len(args) == 0 || !strings.EqualFold(args[0], "add") {
		return false
	}
	args = args[1:]
	if len(args) == 0 || len(args) > 1 {
		return false
	}
	achievementID, ok := s.parseAchievementArgID(args[0])
	if !ok {
		// A bad hyperlink form returns bare nullopt in C++
		// (ChatCommandArgs.cpp:34-44) -> silent false; a non-numeric plain
		// token prints LANG_CMDPARSER_STRING_VALUE_INVALID -> message + true.
		if strings.HasPrefix(args[0], "|c") {
			return false
		}
		s.sendSysMessage("Invalid achievement ID.")
		return true
	}
	// The Trinity ChatCommand parser resolves the AchievementEntry* argument
	// against sAchievementStore before invoking the handler; an unknown id
	// never reaches HandleAchievementAddCommand.
	achievementIndex.mu.RLock()
	_, found := achievementIndex.achieveByID[uint32(achievementID)]
	achievementIndex.mu.RUnlock()
	if !found {
		// Unknown id via hyperlink -> bare nullopt -> silent false; via a
		// plain numeric token -> LANG_CMDPARSER_ACHIEVEMENT_NO_EXIST.
		if strings.HasPrefix(args[0], "|c") {
			return false
		}
		s.sendSysMessage("Unknown achievement ID.")
		return true
	}
	// ChatHandler::getSelectedPlayer: own player when nothing is targeted,
	// otherwise the connected player matching the selection (null, i.e. no
	// character, when the target is offline or not a player).
	target := s
	if s.selection != 0 {
		if s.server == nil {
			return true
		}
		target = s.server.playerSessionForGUID(s.selection)
		if target == nil {
			s.sendSysMessage("No character selected.")
			return true
		}
	}
	// C++ sends no success message here; the completion itself notifies.
	target.completeAchievement(uint32(achievementID))
	return true
}

// parseAchievementArgID mirrors the C++ AchievementEntry* argument parse
// (ChatCommandArgs.cpp:34-44): a numeric id, or a pasted achievement
// hyperlink validated the way LinkTags::achievement::StoreTo does
// (parseAchievementLinkData); an unparseable argument fails the parse.
func (s *session) parseAchievementArgID(arg string) (uint64, bool) {
	if strings.HasPrefix(arg, "|c") {
		info := parseSingleHyperlink(arg)
		if !info.ok || info.tag != "achievement" {
			return 0, false
		}
		t := &hyperlinkDataTokenizer{str: info.data}
		if _, ok := s.parseAchievementLinkData(t); !ok {
			return 0, false
		}
		id, err := strconv.ParseUint(strings.SplitN(info.data, ":", 2)[0], 10, 32)
		if err != nil {
			return 0, false
		}
		return id, true
	}
	id, err := strconv.ParseUint(arg, 10, 32)
	if err != nil {
		return 0, false
	}
	return id, true
}

// commandAllowed mirrors WorldSession::HasPermission (RBAC.h:828):
// membership in _globalPerms, i.e. expanded granted rows (account rows plus
// the security level's rbac_default_permissions) minus expanded denied rows
// (RBAC.cpp:223-232, LoadFromDBCallback). There is no security-level
// short-circuit in C++: a sec>=1 account with the permission denied, or
// without any grant for it, is refused. The security>=1 leg below is only
// the tree's DB-unavailable fallback (same pattern as
// commandNotFoundNotified/skipChatSpamCheck in chat.go): with no rows to
// consult, a GM account keeps its commands rather than losing them all.
func (s *session) commandAllowed(ctx context.Context, permissionID uint32) bool {
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		if hasPerm, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionID); err == nil {
			return hasPerm
		}
	}
	return s.security >= 1
}

// splitQuotedArgs re-splits command fields after the dispatcher tokenized the
// raw line on whitespace, keeping "double-quoted segments" together (quotes
// retained), so the Trinity ChatCommand QuotedString arguments survive the
// dispatcher's strings.Fields tokenization. A quote opens a segment only at
// the start of a token (so O'Brien stays one literal token); backslash
// escapes the next character inside a quoted segment, as
// QuotedString::TryConsume does (ChatCommandTags.cpp:30-59). It returns nil
// when a quoted segment is never closed: C++ rejects the unterminated
// QuotedString parse outright.
func splitQuotedArgs(args []string) []string {
	return splitQuotedArgsMode(args, false)
}

// splitQuotedArgsLenient mirrors splitQuotedArgs but keeps an unterminated
// quoted segment as its final token instead of rejecting the line:
// ChatHandler::extractQuotedArg (Chat.cpp:714-744) is strtok-based and runs
// an unterminated quote to end-of-string, so the guild create/delete/invite/
// rename arms and the send mail/items subject/text args accept it. The
// framework QuotedString args (arena create/rename) keep the strict form.
func splitQuotedArgsLenient(args []string) []string {
	return splitQuotedArgsMode(args, true)
}

func splitQuotedArgsMode(args []string, lenient bool) []string {
	line := strings.Join(args, " ")
	var toks []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
				continue
			}
			if c == quote {
				quote = 0
				cur.WriteByte(c)
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if (c == '"' || c == '\'') && cur.Len() == 0 {
			quote = c
			cur.WriteByte(c)
			continue
		}
		if c == ' ' {
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if quote != 0 && !lenient {
		return nil
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	return toks
}

// unquoteCommandToken mirrors the Trinity ChatCommand QuotedString unwrap:
// a matching quote pair (double or single) is stripped.
func unquoteCommandToken(tok string) string {
	if len(tok) >= 2 && ((strings.HasPrefix(tok, "\"") && strings.HasSuffix(tok, "\"")) ||
		(strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'"))) {
		return tok[1 : len(tok)-1]
	}
	return tok
}

// normalizePlayerName mirrors TrinityCore normalizePlayerName: first letter
// uppercase, the rest lowercase.
func normalizePlayerName(name string) string {
	if name == "" {
		return name
	}
	r, size := utf8.DecodeRuneInString(name)
	return strings.ToUpper(string(r)) + strings.ToLower(name[size:])
}

// parseArenaTeamType mirrors the Trinity ChatCommand enum parse for
// ArenaTeamTypes (cs_arena.cpp HandleArenaCreateCommand): case-insensitive
// prefix match against the EnumUtils titles ARENA_TEAM_2v2/3v3/5v5
// (enuminfo_ArenaTeam.cpp:34-36), then the numeric fallback
// StringTo<uint8> (ChatCommandArgs.h:177-210). "2v2"/"3v3"/"5v5" are NOT
// valid: Match() needs a prefix of the constant name and StringTo<uint8>
// rejects them, so C++ surfaces LANG_CMDPARSER_STRING_VALUE_INVALID.
func parseArenaTeamType(tok string) (uint32, bool) {
	switch strings.ToLower(tok) {
	case "2":
		return ArenaTeamType2v2, true
	case "3":
		return ArenaTeamType3v3, true
	case "5":
		return ArenaTeamType5v5, true
	}
	lower := strings.ToLower(tok)
	matches := 0
	var typ uint32
	for name, t := range map[string]uint32{"arena_team_2v2": ArenaTeamType2v2, "arena_team_3v3": ArenaTeamType3v3, "arena_team_5v5": ArenaTeamType5v5} {
		if strings.HasPrefix(name, lower) {
			matches++
			typ = t
		}
	}
	if matches == 1 {
		return typ, true
	}
	// C++ enum parse falls back to the underlying numeric type
	// (ChatCommandArgs.h:177-210): StringTo<uint8> base 0, so "0x2" and
	// "0b11" are valid team types and "6" is not.
	if v, ok := parseCommandUint32(tok); ok {
		switch v {
		case ArenaTeamType2v2, ArenaTeamType3v3, ArenaTeamType5v5:
			return v, true
		}
	}
	return 0, false
}

// resolveArenaCaptain mirrors the Trinity PlayerIdentifier parse used by the
// arena command's Optional<PlayerIdentifier> arguments (ChatCommandTags.cpp
// PlayerIdentifier::TryConsume): a numeric token is a character low GUID,
// otherwise the token is normalized and looked up as a character name.
func (s *session) resolveArenaCaptain(ctx context.Context, cdb *sql.DB, token string) (uint64, string, bool) {
	if low, ok := parseCommandUint32(token); ok {
		var name string
		if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", low).Scan(&name); err != nil {
			return 0, "", false
		}
		return uint64(low), name, true
	}
	var guid uint64
	var name string
	if err := cdb.QueryRowContext(ctx, "SELECT guid, name FROM characters WHERE name = ?", normalizePlayerName(token)).Scan(&guid, &name); err != nil {
		return 0, "", false
	}
	return guid, name, true
}

// arenaTargetOrSelf mirrors PlayerIdentifier::FromTargetOrSelf for the arena
// command: the selected online player when one is targeted, otherwise the
// session's own player.
func (s *session) arenaTargetOrSelf(ctx context.Context, cdb *sql.DB) (uint64, string, bool) {
	if s.server != nil && s.selection != 0 {
		if target := s.server.playerSessionForGUID(s.selection); target != nil && target.playerLoaded && target.player != nil {
			return target.playerGUID, target.player.Name, true
		}
	}
	if !s.playerLoaded || s.player == nil {
		return 0, "", false
	}
	return s.playerGUID, s.player.Name, true
}

// arenaTeamIsFighting mirrors ArenaTeam::IsFighting (ArenaTeam.cpp): true
// when any online member is currently on a battle-arena map.
func (s *session) arenaTeamIsFighting(ctx context.Context, cdb *sql.DB, teamID uint32) bool {
	rows, err := cdb.QueryContext(ctx, "SELECT guid FROM arena_team_member WHERE arenaTeamId = ?", teamID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint64
		if err := rows.Scan(&guid); err != nil {
			continue
		}
		if s.server == nil {
			continue
		}
		if sess := s.server.playerSessionForGUID(guid); sess != nil && sess.playerLoaded && sess.player != nil && s.server.Data != nil {
			if mapEntry, found, err := s.server.Data.Map(sess.player.Map); err == nil && found && mapEntry.IsBattleArena() {
				return true
			}
		}
	}
	return false
}

// arenaCharactersDB returns the character database or nil, mirroring the
// nil-checked cdb pattern used by the arena packet handlers.
func (s *session) arenaCharactersDB() *sql.DB {
	if s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	return s.server.CharactersStore.DB
}

// handleCmdArena dispatches ".arena" (cs_arena.cpp,
// AddSC_arena_commandscript): create|disband|rename|captain|info|lookup, each
// gated by its RBAC_PERM_COMMAND_ARENA_* permission (RBAC.h:147-152).
func (s *session) handleCmdArena(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "create":
		perm = permissionCommandArenaCreate
	case "disband":
		perm = permissionCommandArenaDisband
	case "rename":
		perm = permissionCommandArenaRename
	case "captain":
		perm = permissionCommandArenaCaptain
	case "info":
		perm = permissionCommandArenaInfo
	case "lookup":
		perm = permissionCommandArenaLookup
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "create":
		return s.handleCmdArenaCreate(ctx, rest)
	case "disband":
		return s.handleCmdArenaDisband(ctx, rest)
	case "rename":
		return s.handleCmdArenaRename(ctx, rest)
	case "captain":
		return s.handleCmdArenaCaptain(ctx, rest)
	case "info":
		return s.handleCmdArenaInfo(ctx, rest)
	case "lookup":
		return s.handleCmdArenaLookup(ctx, rest)
	default:
		return false
	}
}

// handleCmdArenaCreate processes ".arena create [<captain>] <name> <type>"
// (cs_arena.cpp HandleArenaCreateCommand). The emblem values
// (backgroundColor 4293102085, emblemStyle 101, emblemColor 4293253939,
// borderStyle 4, borderColor 4284049911) are the C++-exact ArenaTeam::Create
// call constants; the rating starts at the CONFIG_ARENA_START_RATING default
// of 0 (World.cpp:1217).
func (s *session) handleCmdArenaCreate(ctx context.Context, args []string) bool {
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	toks := splitQuotedArgs(args)
	if toks == nil {
		return false
	}
	var captainGUID uint64
	var captainName string
	rest := toks
	// Optional<PlayerIdentifier> captain: the first token is the captain only
	// when it resolves to a character; otherwise it is the team name.
	if len(toks) > 0 && !strings.HasPrefix(toks[0], "\"") {
		if guid, name, ok := s.resolveArenaCaptain(ctx, cdb, toks[0]); ok {
			captainGUID, captainName = guid, name
			rest = toks[1:]
		} else if len(toks) >= 3 {
			// Optional<PlayerIdentifier> try-with fails with
			// LANG_CMDPARSER_CHAR_NAME_NO_EXIST (message + true); the
			// omit-branch then fails on the unconsumed token too.
			s.sendSysMessage("Character not found.")
			return true
		}
	}
	if len(rest) < 2 {
		return false
	}
	if len(rest) > 2 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	name := unquoteCommandToken(rest[0])
	teamType, ok := parseArenaTeamType(rest[1])
	if !ok {
		// Bad enum token: LANG_CMDPARSER_STRING_VALUE_INVALID + flag.
		s.sendSysMessage("Invalid arena team type. Use 2, 3 or 5.")
		return true
	}
	// No empty-name leg: C++ has none (QuotedString "" parses, Create
	// only checks captain-exists + name-taken, ArenaTeam.cpp:48-56).
	// LANG_ARENA_ERROR_NAME_EXISTS (858). C++ checks the name before the
	// captain fallback (cs_arena.cpp HandleArenaCreateCommand), so a taken
	// name reports here even when no captain target resolves.
	var clash uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", name).Scan(&clash); err == nil {
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" already exists.", name))
		return true
	}
	if captainGUID == 0 {
		guid, name, ok := s.arenaTargetOrSelf(ctx, cdb)
		if !ok {
			// cs_arena.cpp:70-73: bare `return false`, no message, no flag.
			return false
		}
		captainGUID, captainName = guid, name
	}
	// sCharacterCache->GetCharacterArenaTeamIdByGuid(captain, type):
	// LANG_ARENA_ERROR_SIZE (859).
	var existingTeam uint32
	if err := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", captainGUID, teamType).Scan(&existingTeam); err == nil {
		s.sendSysMessage(fmt.Sprintf("%s already has an arena team of the same type.", captainName))
		return true
	}
	// ArenaTeamMgr::GenerateArenaTeamId: NextArenaTeamId++.
	teamID := s.server.generateArenaTeamID(ctx, cdb)
	if teamID == 0 {
		s.sendSysMessage("Unknown error.")
		return true
	}
	// ArenaTeam::Create + CHAR_INS_ARENA_TEAM. Stats.Rating starts at
	// CONFIG_ARENA_START_RATING (World.cpp:1217, default 0); Stats.Rating
	// is uint16 (ArenaTeam.h:106), so the config value truncates.
	teamRating := uint32(uint16(s.server.Config.ArenaStartRating))
	if _, err := cdb.ExecContext(ctx, "INSERT INTO arena_team (arenaTeamId, name, captainGuid, type, rating, backgroundColor, emblemStyle, emblemColor, borderStyle, borderColor) VALUES (?, ?, ?, ?, ?, 4293102085, 101, 4293253939, 4, 4284049911)", teamID, name, captainGUID, teamType, teamRating); err != nil {
		// ArenaTeam::Create failure prints LANG_BAD_VALUE (cs_arena.cpp:85-89).
		s.sendSysMessage("Invalid value.")
		return true
	}
	// ArenaTeam::AddMember(captain): Player::RemovePetitionsAndSigns with the
	// arena charter type, which equals the team type (SharedDefines.h:3790).
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE playerguid = ? AND type = ?", captainGUID, teamType)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE ownerguid = ? AND type = ?", captainGUID, teamType)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition WHERE ownerguid = ? AND type = ?", captainGUID, teamType)
	// CHAR_INS_ARENA_TEAM_MEMBER; the captain's personal rating follows
	// the AddMember leg (ArenaTeam.cpp:126-131) via
	// arenaTeamJoinPersonalRating.
	personalRating := arenaTeamJoinPersonalRating(s.server.Config.ArenaStartPersonalRating, teamRating)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO arena_team_member (arenaTeamId, guid, weekGames, weekWins, seasonGames, seasonWins, personalRating) VALUES (?, ?, 0, 0, 0, 0, ?)", teamID, captainGUID, personalRating)
	// ArenaTeam::Create runs AddMember(captain), which sets the online
	// captain's live slot fields (ArenaTeam.cpp:175-178); the DB rows
	// alone leave the client showing no team until relog.
	if captSess := s.server.findSessionByGUID(captainGUID); captSess != nil {
		setLiveArenaTeamSlot(captSess, teamType, teamID, true)
	}
	// LANG_ARENA_CREATE (864).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" created with id %d (type %dv%d, captain guid %d).", name, teamID, teamType, teamType, captainGUID))
	s.debug("arena team created", "id", teamID, "name", name, "captain", captainName)
	return true
}

// handleCmdArenaDisband processes ".arena disband <teamId>"
// (cs_arena.cpp HandleArenaDisbandCommand), mirroring the parameterless
// ArenaTeam::Disband: per-member cleanup followed by CHAR_DEL_ARENA_TEAM and
// CHAR_DEL_ARENA_TEAM_MEMBERS.
func (s *session) handleCmdArenaDisband(ctx context.Context, args []string) bool {
	if len(args) < 1 {
		return false
	}
	id, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable uint32 token: LANG_CMDPARSER_STRING_VALUE_INVALID.
		s.sendSysMessage("Invalid team ID.")
		return true
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	teamID := uint32(id)
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	var name string
	var teamType uint32
	if err := cdb.QueryRowContext(ctx, "SELECT name, type FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&name, &teamType); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return true
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return true
	}
	// ArenaTeam::Disband runs DelMember per member (ArenaTeam.cpp:406-409):
	// each online member gets ERR_ARENA_TEAM_QUIT_S and its live slot
	// fields cleared (ArenaTeam.cpp:353-357), as the captain-leave path
	// in arena_team.go already does. Without this the members keep
	// showing the team until relog.
	if rows, err := cdb.QueryContext(ctx, "SELECT guid FROM arena_team_member WHERE arenaTeamId = ?", teamID); err == nil {
		for rows.Next() {
			var mGUID uint64
			if rows.Scan(&mGUID) == nil {
				if ms := s.server.findSessionByGUID(mGUID); ms != nil && ms.player != nil {
					ms.sendArenaTeamCommandResult(arenaTeamQuitS, name, "", 0)
					clearLiveArenaTeamSlot(ms, teamType)
				}
			}
		}
		rows.Close()
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM arena_team_member WHERE arenaTeamId = ?", teamID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM arena_team WHERE arenaTeamId = ?", teamID)
	// LANG_ARENA_DISBAND (865).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d) disbanded.", name, teamID))
	s.debug("arena team disbanded", "id", teamID, "name", name)
	return true
}

// handleCmdArenaRename processes ".arena rename <oldName> <newName>"
// (cs_arena.cpp HandleArenaRenameCommand), mirroring ArenaTeam::SetName: the
// rename fails (LANG_BAD_VALUE) when the name is unchanged, empty, or longer
// than 24 characters. The reserved-name and charter-name validations have no
// Go bridge and are documented as a gap.
func (s *session) handleCmdArenaRename(ctx context.Context, args []string) bool {
	toks := splitQuotedArgs(args)
	if len(toks) < 2 {
		return false
	}
	if len(toks) > 2 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	oldName, newName := unquoteCommandToken(toks[0]), unquoteCommandToken(toks[1])
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	var teamID uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", oldName).Scan(&teamID); err != nil {
		// LANG_ARENA_ERROR_NAME_NOT_FOUND (861).
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" not found.", oldName))
		return true
	}
	var clash uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", newName).Scan(&clash); err == nil {
		// LANG_ARENA_ERROR_NAME_EXISTS (858).
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" already exists.", newName))
		return true
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return true
	}
	if newName == "" || newName == oldName || len(newName) > 24 {
		// ArenaTeam::SetName counts bytes (name.length(), ArenaTeam.cpp:276),
		// not runes.
		s.sendSysMessage("Invalid value.")
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE arena_team SET name = ? WHERE arenaTeamId = ?", newName, teamID); err != nil {
		s.sendSysMessage("Invalid value.")
		return true
	}
	// LANG_ARENA_RENAME (866).
	s.sendSysMessage(fmt.Sprintf("Arena team %d renamed from \"%s\" to \"%s\".", teamID, oldName, newName))
	s.debug("arena team renamed", "id", teamID, "from", oldName, "to", newName)
	return true
}

// handleCmdArenaCaptain processes ".arena captain <teamId> [<player>]"
// (cs_arena.cpp HandleArenaCaptainCommand), mirroring ArenaTeam::SetCaptain
// (CHAR_UPD_ARENA_TEAM_CAPTAIN).
func (s *session) handleCmdArenaCaptain(ctx context.Context, args []string) bool {
	if len(args) < 1 {
		return false
	}
	id, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable uint32 token: LANG_CMDPARSER_STRING_VALUE_INVALID.
		s.sendSysMessage("Invalid team ID.")
		return true
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	teamID := uint32(id)
	if len(args) > 2 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	var teamName string
	var captainGUID uint64
	var teamType uint32
	if err := cdb.QueryRowContext(ctx, "SELECT name, captainGuid, type FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&teamName, &captainGUID, &teamType); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return true
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return true
	}
	var targetGUID uint64
	var targetName string
	if len(args) > 1 {
		guid, name, ok := s.resolveArenaCaptain(ctx, cdb, args[1])
		if !ok {
			s.sendSysMessage("Character not found.")
			return true
		}
		targetGUID, targetName = guid, name
	} else {
		guid, name, ok := s.arenaTargetOrSelf(ctx, cdb)
		if !ok {
			// cs_arena.cpp:178-181: bare `return false`, no message.
			return false
		}
		targetGUID, targetName = guid, name
	}
	var isMember uint32
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", teamID, targetGUID).Scan(&isMember); err != nil || isMember == 0 {
		// LANG_ARENA_ERROR_NOT_MEMBER (862).
		s.sendSysMessage(fmt.Sprintf("%s is not a member of arena team \"%s\".", targetName, teamName))
		return true
	}
	if captainGUID == targetGUID {
		// LANG_ARENA_ERROR_CAPTAIN (863).
		s.sendSysMessage(fmt.Sprintf("%s is already the captain of arena team \"%s\".", targetName, teamName))
		return true
	}
	oldCaptainName := "<unknown>"
	_ = cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", captainGUID).Scan(&oldCaptainName)
	if _, err := cdb.ExecContext(ctx, "UPDATE arena_team SET captainGuid = ? WHERE arenaTeamId = ?", targetGUID, teamID); err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	// ArenaTeam::SetCaptain (ArenaTeam.cpp:287-310) flips the live member
	// flag: the old captain's goes to 1, the new captain's to 0 (promote
	// buttons), as the packet promote path in arena_team.go does.
	if oldSess := s.server.findSessionByGUID(captainGUID); oldSess != nil && oldSess.player != nil {
		setLiveArenaTeamMemberFlag(oldSess, teamType, false)
	}
	if newSess := s.server.findSessionByGUID(targetGUID); newSess != nil && newSess.player != nil {
		setLiveArenaTeamMemberFlag(newSess, teamType, true)
	}
	// LANG_ARENA_CAPTAIN (867).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d): captain changed from %s to %s.", teamName, teamID, oldCaptainName, targetName))
	s.debug("arena team captain changed", "id", teamID, "from", oldCaptainName, "to", targetName)
	return true
}

// handleCmdArenaInfo processes ".arena info <teamId>"
// (cs_arena.cpp HandleArenaInfoCommand): header plus one line per member,
// with the "- Captain" marker on the captain's line.
func (s *session) handleCmdArenaInfo(ctx context.Context, args []string) bool {
	if len(args) < 1 {
		return false
	}
	id, ok := parseCommandUint32(args[0])
	if !ok {
		// Unparseable uint32 token: LANG_CMDPARSER_STRING_VALUE_INVALID.
		s.sendSysMessage("Invalid team ID.")
		return true
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	teamID := uint32(id)
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	var name string
	var rating uint32
	var teamType uint32
	var captainGUID uint64
	if err := cdb.QueryRowContext(ctx, "SELECT name, rating, type, captainGuid FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&name, &rating, &teamType, &captainGUID); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return true
	}
	// LANG_ARENA_INFO_HEADER (868).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d): rating %d, type %dv%d.", name, teamID, rating, teamType, teamType))
	rows, err := cdb.QueryContext(ctx, "SELECT m.guid, COALESCE(c.name, ''), m.personalRating FROM arena_team_member AS m LEFT JOIN characters AS c ON c.guid = m.guid WHERE m.arenaTeamId = ?", teamID)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint64
		var memberName string
		var personalRating uint16
		if err := rows.Scan(&guid, &memberName, &personalRating); err != nil {
			continue
		}
		captainMark := ""
		if guid == captainGUID {
			captainMark = " - Captain"
		}
		// LANG_ARENA_INFO_MEMBERS (869).
		s.sendSysMessage(fmt.Sprintf("%s (%d): personal rating %d%s", memberName, guid, personalRating, captainMark))
	}
	return true
}

// handleCmdArenaLookup processes ".arena lookup <name>"
// (cs_arena.cpp HandleArenaLookupCommand): case-insensitive substring match
// over all arena teams, like TrinityCore StringContainsStringI. The C++ loop
// only emits rows when handler->GetSession() is set; the Go command path is
// always a session, so rows are always emitted.
func (s *session) handleCmdArenaLookup(ctx context.Context, args []string) bool {
	needle := s.commandTail(args)
	if strings.TrimSpace(needle) == "" {
		// cs_arena.cpp:226-227: bare `return false`, no message.
		return false
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	rows, err := cdb.QueryContext(ctx, "SELECT arenaTeamId, name, type FROM arena_team")
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return true
	}
	defer rows.Close()
	found := false
	lowerNeedle := strings.ToLower(needle)
	for rows.Next() {
		var teamID uint32
		var name string
		var teamType uint32
		if err := rows.Scan(&teamID, &name, &teamType); err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(name), lowerNeedle) {
			// LANG_ARENA_LOOKUP (870).
			s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d, type %dv%d).", name, teamID, teamType, teamType))
			found = true
		}
	}
	if !found {
		// LANG_ARENA_ERROR_NAME_NOT_FOUND (861).
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" not found.", needle))
	}
	return true
}

// handleCmdBF dispatches ".bf" (cs_bf.cpp, AddSC_bf_commandscript):
// start|stop|switch|timer|enable, each gated by its RBAC_PERM_COMMAND_BF_*
// permission (RBAC.h:172-176).
func (s *session) handleCmdBF(ctx context.Context, args []string) bool {
	if s.server == nil {
		return true
	}
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "start":
		perm = permissionCommandBfStart
	case "stop":
		perm = permissionCommandBfStop
	case "switch":
		perm = permissionCommandBfSwitch
	case "timer":
		perm = permissionCommandBfTimer
	case "enable":
		perm = permissionCommandBfEnable
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "start":
		return s.handleCmdBFStart(ctx, rest)
	case "stop":
		return s.handleCmdBFStop(ctx, rest)
	case "switch":
		return s.handleCmdBFSwitch(ctx, rest)
	case "timer":
		return s.handleCmdBFTimer(ctx, rest)
	case "enable":
		return s.handleCmdBFEnable(ctx, rest)
	}
	return false
}

// bfBattleID parses the <battleId> argument. It mirrors
// BattlefieldMgr::GetBattlefieldByBattleId: WGBattleID (1,
// BATTLEFIELD_BATTLEID_WG, Battlefield.h:34) is the only registered battle id,
// so any other id fails the command exactly like the C++ null-battlefield
// return-false path. The second return reports whether the caller may
// proceed; when false, the third is the bool the caller must return: a
// missing id or an unknown id is silent false, while an unparseable
// non-empty token prints the parser error and returns true
// (ChatCommandArgs.h numeric ArgInfo: LANG_CMDPARSER_STRING_VALUE_INVALID
// + SetSentErrorMessage suppresses help).
func (s *session) bfBattleID(args []string) (uint32, bool, bool) {
	if len(args) < 1 {
		return 0, false, false
	}
	id, ok := parseCommandUint32(args[0])
	if !ok {
		s.sendSysMessage("Invalid battlefield ID.")
		return 0, false, true
	}
	if id != WGBattleID {
		return 0, false, false
	}
	return id, true, true
}

// handleCmdBFStart processes ".bf start <battleId>" (cs_bf.cpp
// HandleBattlefieldStart). Battlefield::StartBattle (Battlefield.cpp:316) is a
// no-op when the battle is already active, so the native StartWGBattle is
// skipped in that case; the defender team is preserved from current state.
func (s *session) handleCmdBFStart(ctx context.Context, args []string) bool {
	if _, proceed, ret := s.bfBattleID(args); !proceed {
		return ret
	}
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	wg := s.server.getOrCreateWGState()
	wg.mu.Lock()
	active := wg.IsActive
	defender := wg.DefenderTeam
	wg.mu.Unlock()
	if !active {
		s.server.StartWGBattle(defender)
	}
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command start used)")
	return true
}

// handleCmdBFStop processes ".bf stop <battleId>" (cs_bf.cpp
// HandleBattlefieldEnd): EndBattle(true) — the defenders hold the fortress.
func (s *session) handleCmdBFStop(ctx context.Context, args []string) bool {
	if _, proceed, ret := s.bfBattleID(args); !proceed {
		return ret
	}
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	s.server.EndWGBattle(true)
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command stop used)")
	return true
}

// handleCmdBFSwitch processes ".bf switch <battleId>" (cs_bf.cpp
// HandleBattlefieldSwitch): EndBattle(false) — the attackers breached the
// vault and become the new defenders.
func (s *session) handleCmdBFSwitch(ctx context.Context, args []string) bool {
	if _, proceed, ret := s.bfBattleID(args); !proceed {
		return ret
	}
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	s.server.EndWGBattle(false)
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command switch used)")
	return true
}

// handleCmdBFTimer processes ".bf timer <battleId> <time>" (cs_bf.cpp
// HandleBattlefieldTimer): SetTimer(time * IN_MILLISECONDS) then
// SendInitWorldStatesToAll.
func (s *session) handleCmdBFTimer(ctx context.Context, args []string) bool {
	if _, proceed, ret := s.bfBattleID(args); !proceed {
		return ret
	}
	if len(args) < 2 {
		return false
	}
	secs, ok := parseCommandUint32(args[1])
	if !ok {
		s.sendSysMessage("Invalid time value.")
		return true
	}
	if len(args) > 2 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	wg := s.server.getOrCreateWGState()
	wg.mu.Lock()
	wg.Timer = time.Duration(secs) * time.Second
	wg.EndTime = time.Now().Add(wg.Timer)
	wg.mu.Unlock()
	s.server.broadcastWGInitWorldStates()
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command timer used)")
	return true
}

// handleCmdBFEnable processes ".bf enable <battleId>" (cs_bf.cpp
// HandleBattlefieldEnable): toggles Battlefield::m_IsEnabled
// (ToggleBattlefield), mirrored on wgBattlegroundState.Enabled; the Update and
// player-enter-zone paths consult it (BattlefieldMgr.cpp:100, 130).
func (s *session) handleCmdBFEnable(ctx context.Context, args []string) bool {
	if _, proceed, ret := s.bfBattleID(args); !proceed {
		return ret
	}
	if len(args) > 1 {
		// C++ ConsumeFromOffset requires the whole string to be consumed
		// (ChatCommand.h:104-108); a trailing token fails the parse.
		return false
	}
	wg := s.server.getOrCreateWGState()
	wg.mu.Lock()
	wg.Enabled = !wg.Enabled
	enabled := wg.Enabled
	wg.mu.Unlock()
	if enabled {
		s.server.sendGlobalGMMessage(ctx, "Wintergrasp is enabled")
	} else {
		s.server.sendGlobalGMMessage(ctx, "Wintergrasp is disabled")
	}
	return true
}

// sendGlobalGMMessage mirrors ChatHandler::SendGlobalGMSysMessage /
// World::SendGlobalGMMessage (Chat.cpp:138, World.cpp): the message goes to
// every in-world session holding rbac::RBAC_PERM_RECEIVE_GLOBAL_GM_TEXTMESSAGE
// (44), evaluated through the native RBAC grant path.
func (s *Server) sendGlobalGMMessage(ctx context.Context, msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if !sess.commandAllowed(ctx, permissionReceiveGlobalGMTextMessage) {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		sess.sendSysMessage(msg)
	}
}

// sendGlobalSysMessage mirrors ChatHandler::SendGlobalSysMessage: the
// message goes to every in-world session, with no RBAC 44 GM gate (the
// reputation reload arms in cs_reload.cpp use it, unlike every other
// reload arm, which uses SendGlobalGMSysMessage).
func (s *Server) sendGlobalSysMessage(ctx context.Context, msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		targets = append(targets, sess)
	}
	s.sessionsMu.RUnlock()
	for _, sess := range targets {
		sess.sendSysMessage(msg)
	}
}

// ---- deserter_commandscript (cs_deserter.cpp) ----

// Deserter debuff spells (cs_deserter.cpp:32-35).
const (
	deserterSpellInstance = 71041 // LFG_SPELL_DUNGEON_DESERTER
	deserterSpellBG       = 26013 // BG_SPELL_DESERTER
)

// handleCmdDeserter mirrors the deserter_commandscript nested tables
// (cs_deserter.cpp:58-77): "deserter instance|bg add <time>" applies the
// matching deserter aura with a custom duration (HandleDeserterAdd), and
// "deserter instance|bg remove" drops it (HandleDeserterRemove).
// The bool reports C++ handler success: RBAC denial returns false
// silently so dispatchCommand shows the node's help listing
// (ChatCommand.cpp:296-309); the framework arg-parse failures (a missing
// or unparseable time, extra tokens on remove) return false the same
// way — C++ never invokes the handler there (ChatCommand.h:
// ConsumeFromOffset, "the entire string must be consumed"). Paths that
// print a message (no character selected, LANG_BAD_VALUE) return true:
// C++ sends them with SetSentErrorMessage, which suppresses the help.
func (s *session) handleCmdDeserter(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .deserter instance|bg add|remove [time]")
		return true
	}
	group := strings.ToLower(args[0])
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .deserter " + group + " add|remove [time]")
		return true
	}
	var perm uint32
	var spellID uint32
	var adding bool
	var permAdd, permRemove uint32
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the add/remove arm the same way here.
	matchArm := func(token string) bool {
		lower := strings.ToLower(token)
		switch {
		case strings.HasPrefix("add", lower):
			perm = permAdd
			adding = true
			return true
		case strings.HasPrefix("remove", lower):
			perm = permRemove
			return true
		}
		return false
	}
	switch group {
	case "instance":
		spellID = deserterSpellInstance
		permAdd, permRemove = permissionCommandDeserterInstanceAdd, permissionCommandDeserterInstanceRemove
	case "bg":
		spellID = deserterSpellBG
		permAdd, permRemove = permissionCommandDeserterBGAdd, permissionCommandDeserterBGRemove
	default:
		s.sendSysMessage("Syntax: .deserter instance|bg add|remove [time]")
		return true
	}
	if !matchArm(args[1]) {
		s.sendSysMessage("Syntax: .deserter " + group + " add|remove [time]")
		return true
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	// ChatHandler::getSelectedPlayer: own player when nothing is targeted,
	// otherwise the connected player matching the selection (null, i.e. no
	// character, when the target is offline or not a player).
	target := s
	if s.selection != 0 {
		if s.server == nil {
			return true
		}
		target = s.server.playerSessionForGUID(s.selection)
		if target == nil {
			s.sendSysMessage("No character selected.")
			return true
		}
	}
	if adding {
		if len(args) != 3 {
			return false
		}
		secs, ok := parseCommandUint32(args[2])
		if !ok {
			return false
		}
		if secs == 0 {
			// LANG_BAD_VALUE: zero time, like the C++ !time check.
			s.sendSysMessage("Incorrect value.")
			return true
		}
		// Player::AddAura returns null when the spell entry is missing, which
		// the C++ handler reports as LANG_BAD_VALUE.
		if s.server == nil || s.server.Data == nil {
			return true
		}
		if _, found, _ := s.server.Data.Spell(spellID); !found {
			s.sendSysMessage("Incorrect value.")
			return true
		}
		// Aura::SetDuration(time * IN_MILLISECONDS).
		target.applyAuraWithDuration(spellID, secs*1000)
		return true
	}
	// HandleDeserterRemove takes no arguments (cs_deserter.cpp:143).
	if len(args) != 2 {
		return false
	}
	target.removeAura(spellID)
	return true
}

// ---- disable_commandscript (cs_disable.cpp) ----

// DisableType values mirror the DisableType enum (DisableMgr.h:24-36).
const (
	disableTypeSpell               = 0
	disableTypeQuest               = 1
	disableTypeMap                 = 2
	disableTypeBattleground        = 3
	disableTypeAchievementCriteria = 4
	disableTypeOutdoorPvP          = 5
	disableTypeVMap                = 6
	disableTypeMMap                = 7
)

// maxOutdoorPVPTypes mirrors MAX_OUTDOORPVP_TYPES (OutdoorPvP.h:28-36):
// OUTDOOR_PVP_HP = 1 through OUTDOOR_PVP_EP = 6.
const maxOutdoorPVPTypes = 7

// disableCommandType mirrors one arm of the add/remove ChatCommandTables
// (cs_disable.cpp:52-75): the DisableType, the label the handlers echo back
// in their messages, and the matching add/remove RBAC grants (RBAC.h:221-237).
type disableCommandType struct {
	disableType uint8
	label       string
	permAdd     uint32
	permRemove  uint32
}

// disableCommandTypes mirrors the arm names of the add/remove tables
// (cs_disable.cpp:52-75) in table order.
//
// Enforcement status of the stored rows (D-5): the add/remove arms write
// rows for all eight DisableTypes, but only some have Go consumers.
// DISABLE_TYPE_OUTDOORPVP (5) has exactly one C++ consumer —
// OutdoorPvPMgr::LoadFromDB (OutdoorPvPMgr.cpp:70) skips OutdoorPvP types
// with a disables row — and Go has no OutdoorPvP manager (server.go:2613),
// so outdoorpvp rows are stored but unenforced until a manager lands.
// DISABLE_TYPE_MMAP (7) is consumed in C++ by PathGenerator::CalculatePath
// (PathGenerator.cpp:41) and Map::LoadMMapTile (Map.cpp:169) via
// IsPathfindingEnabled; Go runs no navmesh pathfinder (creaturemotion.go:3920)
// and loads no mmap tiles, so mmap rows are enforced only in the `mmap stats`
// display (commands_mmaps.go:144). DISABLE_TYPE_VMAP (6) is enforced on the
// terrain path (terrain.go: vmapDisabledFor).
var disableCommandTypes = []disableCommandType{
	{disableTypeSpell, "spell", permissionCommandDisableAddSpell, permissionCommandDisableRemoveSpell},
	{disableTypeQuest, "quest", permissionCommandDisableAddQuest, permissionCommandDisableRemoveQuest},
	{disableTypeMap, "map", permissionCommandDisableAddMap, permissionCommandDisableRemoveMap},
	{disableTypeBattleground, "battleground", permissionCommandDisableAddBattleground, permissionCommandDisableRemoveBattleground},
	{disableTypeAchievementCriteria, "achievement criteria", permissionCommandDisableAddAchievementCriteria, permissionCommandDisableRemoveAchievementCriteria},
	{disableTypeOutdoorPvP, "outdoorpvp", permissionCommandDisableAddOutdoorPvP, permissionCommandDisableRemoveOutdoorPvP},
	{disableTypeVMap, "vmap", permissionCommandDisableAddVMap, permissionCommandDisableRemoveVMap},
	{disableTypeMMap, "mmap", permissionCommandDisableAddMMap, permissionCommandDisableRemoveMMap},
}

// handleCmdDisable mirrors disable_commandscript::GetCommands
// (cs_disable.cpp:58-84): the "disable" root with the "add" and "remove"
// groups, each arm named after its DisableType and gated on the matching
// RBAC_PERM_COMMAND_DISABLE_* permission (RBAC.h:221-237, all Console::Yes).
// The bool reports C++ handler success: RBAC denial returns false
// silently so dispatchCommand shows the node's help listing
// (ChatCommand.cpp:296-309); paths that print a message return true.
func (s *session) handleCmdDisable(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .disable add|remove <spell|quest|map|battleground|achievement_criteria|outdoorpvp|vmap|mmap> <entry> [flags] <comment>"
	if len(args) < 2 {
		s.sendSysMessage(syntax)
		return true
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the add/remove group and the type arm the same way here.
	mode := ""
	switch lower := strings.ToLower(args[0]); {
	case strings.HasPrefix("add", lower):
		mode = "add"
	case strings.HasPrefix("remove", lower):
		mode = "remove"
	}
	if mode == "" {
		s.sendSysMessage(syntax)
		return true
	}
	var info *disableCommandType
	lowerType := strings.ToLower(args[1])
	var matches []*disableCommandType
	for i := range disableCommandTypes {
		// The "achievement criteria" arm is spelled "achievement_criteria"
		// in the C++ command table.
		name := strings.ReplaceAll(disableCommandTypes[i].label, " ", "_")
		if name == lowerType {
			info = &disableCommandTypes[i]
			matches = nil
			break
		}
		if strings.HasPrefix(name, lowerType) {
			matches = append(matches, &disableCommandTypes[i])
		}
	}
	if info == nil {
		if len(matches) == 1 {
			info = matches[0]
		} else if len(matches) > 1 {
			// ChatCommand.cpp:266-288: a partial token matching two
			// subcommands lists the candidates and executes nothing;
			// "m" matches both map and mmap.
			s.sendSysMessage("There are multiple commands matching '" + lowerType + "'. Did you mean:")
			for _, m := range matches {
				s.sendSysMessage(".disable " + mode + " " + strings.ReplaceAll(m.label, " ", "_"))
			}
			return true
		}
	}
	if info == nil {
		s.sendSysMessage(syntax)
		return true
	}
	perm := info.permRemove
	if mode == "add" {
		perm = info.permAdd
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	if mode == "add" {
		return s.handleDisableAdd(ctx, info, args[2:])
	}
	return s.handleDisableRemove(ctx, info, args[2:])
}

// handleDisableAdd mirrors HandleAddDisables (cs_disable.cpp:93-217): the
// entry is validated against the matching data store, duplicates are
// rejected, and the row lands in the disables table (WORLD_INS_DISABLES).
// The bool reports C++ handler success: the missing-entry/missing-comment
// and entry-0 legs return false silently (cs_disable.cpp:87,94), so
// dispatchCommand shows the node's help listing; the outdoorpvp add arm
// alone swallows that false (HandleAddDisableOutdoorPvPCommand ignores
// HandleAddDisables' return, :258-265) and returns true — except on
// empty args, where the wrapper's !*args gate (:260-261) still returns
// false. Validation and already-disabled legs print their message and
// return true: C++ sends them with SetSentErrorMessage, which
// suppresses the help.
func (s *session) handleDisableAdd(ctx context.Context, info *disableCommandType, args []string) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	// C++: strtok(entry) missing or atoi == 0 -> return false, and a
	// missing comment token -> return false, both with no message sent
	// (cs_disable.cpp:87,94). The outdoorpvp add arm alone swallows
	// that false (HandleAddDisableOutdoorPvPCommand ignores
	// HandleAddDisables' return, :258-265), so those failures return
	// true for outdoorpvp only — except on empty args, where the
	// wrapper's !*args gate (:260-261) still returns false.
	if len(args) == 0 {
		return false
	}
	if len(args) < 3 {
		if info.disableType == disableTypeOutdoorPvP {
			return true
		}
		return false
	}
	// C++: entry is uint32(atoi(entryStr)), flags uint8(atoi(flagsStr));
	// cAtoi keeps the atoi leniency (leading-digit runs, sign, wrap).
	entry := uint32(cAtoi(args[0]))
	if entry == 0 {
		if info.disableType == disableTypeOutdoorPvP {
			return true
		}
		return false
	}
	flags := uint8(cAtoi(args[1]))
	comment := strings.Join(args[2:], " ")
	switch info.disableType {
	case disableTypeSpell:
		// sSpellMgr->GetSpellInfo -> LANG_COMMAND_NOSPELLFOUND.
		if s.server.Data == nil {
			return true
		}
		if _, found, _ := s.server.Data.Spell(entry); !found {
			s.sendSysMessage("No spell found.")
			return true
		}
	case disableTypeQuest:
		// sObjectMgr->GetQuestTemplate -> LANG_COMMAND_QUEST_NOTFOUND.
		if !characterQuestTemplateExists(ctx, s.server.WorldStore.DB, entry) {
			s.sendSysMessage(fmt.Sprintf("No quest template found for entry %d.", entry))
			return true
		}
	case disableTypeMap, disableTypeVMap, disableTypeMMap:
		// sMapStore.LookupEntry -> LANG_COMMAND_NOMAPFOUND.
		if s.server.Data == nil {
			return true
		}
		if _, found, _ := s.server.Data.Map(entry); !found {
			s.sendSysMessage("No map found.")
			return true
		}
	case disableTypeBattleground:
		// sBattlemasterListStore.LookupEntry ->
		// LANG_COMMAND_NO_BATTLEGROUND_FOUND.
		if s.server.Data == nil {
			return true
		}
		file, fileErr := s.server.Data.File("BattlemasterList")
		if fileErr != nil {
			return true
		}
		if _, found := file.Find(entry); !found {
			s.sendSysMessage("No battleground found.")
			return true
		}
	case disableTypeAchievementCriteria:
		// sAchievementMgr->GetAchievementCriteria ->
		// LANG_COMMAND_NO_ACHIEVEMENT_CRITERIA_FOUND.
		s.server.loadAchievementIndex()
		achievementIndex.mu.RLock()
		_, found := achievementIndex.byID[entry]
		achievementIndex.mu.RUnlock()
		if !found {
			s.sendSysMessage("No achievement criteria found.")
			return true
		}
	case disableTypeOutdoorPvP:
		// entry > MAX_OUTDOORPVP_TYPES ->
		// LANG_COMMAND_NO_OUTDOOR_PVP_FORUND.
		if entry > maxOutdoorPVPTypes {
			// cs_disable.cpp:163 sends LANG_COMMAND_NO_OUTDOOR_PVP_FORUND
			// with no entry argument.
			s.sendSysMessage("No outdoor PvP found.")
			return true
		}
	}
	row, err := s.server.WorldStore.QueryRowStatement(ctx, database.StatementID("WORLD_SEL_DISABLES"), entry, info.disableType)
	alreadyDisabled := false
	if err == nil {
		var existing uint32
		alreadyDisabled = row.Scan(&existing) == nil
	}
	if alreadyDisabled {
		s.sendSysMessage(fmt.Sprintf("This %s (Id: %d) is already disabled.", info.label, entry))
		return true
	}
	if _, err := s.server.WorldStore.ExecStatement(ctx, database.StatementID("WORLD_INS_DISABLES"), entry, info.disableType, uint16(flags), comment); err != nil {
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Add Disabled %s (Id: %d) for reason %s", info.label, entry, comment))
	return true
}

// handleDisableRemove mirrors HandleRemoveDisables (cs_disable.cpp:274-326):
// the row is looked up in the disables table (WORLD_SEL_DISABLES) and
// deleted (WORLD_DEL_DISABLES).
// The bool reports C++ handler success: the missing-entry and entry-0
// legs return false silently (cs_disable.cpp:286), so dispatchCommand
// shows the node's help listing; the not-disabled leg prints its
// message and returns true (SetSentErrorMessage in C++).
func (s *session) handleDisableRemove(ctx context.Context, info *disableCommandType, args []string) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	if len(args) == 0 {
		return false
	}
	// C++: uint32(atoi(entryStr)) with the same !atoi -> false gate as add.
	entry := uint32(cAtoi(args[0]))
	if entry == 0 {
		return false
	}
	row, err := s.server.WorldStore.QueryRowStatement(ctx, database.StatementID("WORLD_SEL_DISABLES"), entry, info.disableType)
	disabled := false
	if err == nil {
		var existing uint32
		disabled = row.Scan(&existing) == nil
	}
	if !disabled {
		s.sendSysMessage(fmt.Sprintf("This %s (Id: %d) is not disabled.", info.label, entry))
		return true
	}
	if _, err := s.server.WorldStore.ExecStatement(ctx, database.StatementID("WORLD_DEL_DISABLES"), entry, info.disableType); err != nil {
		return true
	}
	s.sendSysMessage(fmt.Sprintf("Remove Disabled %s (Id: %d)", info.label, entry))
	return true
}

// handleCmdEvent mirrors event_commandscript::GetCommands (cs_event.cpp:42-64):
// the "event" root with the activelist/start/stop/info arms, each gated on
// the matching RBAC_PERM_COMMAND_EVENT_* permission (RBAC.h:239-242, all
// Console::Yes). The Trinity parser prefix-matches arm names; the same
// first-match rule applies here (C++ table order: activelist, start, stop,
// info).
func (s *session) handleCmdEvent(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .event activelist|start|stop|info [eventId]"
	if len(args) < 1 {
		return false
	}
	arms := []struct {
		name string
		perm uint32
	}{
		{"activelist", permissionCommandEventActivelist},
		{"start", permissionCommandEventStart},
		{"stop", permissionCommandEventStop},
		{"info", permissionCommandEventInfo},
	}
	lower := strings.ToLower(args[0])
	arm := ""
	var perm uint32
	for _, a := range arms {
		if strings.HasPrefix(a.name, lower) {
			arm = a.name
			perm = a.perm
			break
		}
	}
	if arm == "" {
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch arm {
	case "activelist":
		// ChatCommand.h:112: the no-arg arm takes no trailing tokens.
		if len(args) > 1 {
			return false
		}
		return s.handleEventActiveList(ctx)
	case "info":
		return s.handleEventInfo(ctx, args[1:])
	case "start":
		return s.handleEventStart(ctx, args[1:])
	case "stop":
		return s.handleEventStop(ctx, args[1:])
	}
	return false
}

// handleEventActiveList mirrors HandleEventActiveListCommand
// (cs_event.cpp:66-88): one line per active event, ascending by event id
// (std::set<uint16> order), then "No event found." when the list is empty
// (LANG_NOEVENTFOUND, 584). The line renders LANG_EVENT_ENTRY_LIST_CHAT
// (583) exactly as the lookup arm does (cs_lookup.cpp:280-283): the same
// LANG id with the same (id, id, description, active) args, so the two arms
// share one rendering — the second id is the |Hgameevent:|h hyperlink id.
// The console-vs-chat branch (LANG 1103 vs 583) is moot here: Go commands
// always run in a session, so the chat form is emitted. TDB-TEXT GAP: the
// LANG 583/584 format strings live in the trinity_string table (TDB data —
// no data rows exist in Go sql/ or the C++ sql/ tree), so the wording and
// separators follow the lookup arm's established form and are unverified
// against the TDB text (same class as the tick-309 R3 blocker).
func (s *session) handleEventActiveList(ctx context.Context) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	data := s.server.loadGameEventDataMap(ctx)
	active := s.server.cachedActiveGameEvents(ctx)
	ids := make([]int64, 0, len(active))
	for id := range active {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		desc := ""
		if ev, ok := data[id]; ok {
			desc = ev.Description
		}
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hgameevent:%d|h[%s]|h|r Active", id, id, desc))
	}
	if len(ids) == 0 {
		s.sendSysMessage("No event found.")
	}
	return true
}

// handleEventInfo mirrors HandleEventInfoCommand (cs_event.cpp:90-123): the
// event is looked up by id (isValid-gated, else LANG_EVENT_NOT_EXIST 585),
// then the description, active marker, start/end timestamps, occurrence,
// length and the next state-change time are printed (LANG_EVENT_INFO, 586).
// The active marker comes from the same GAMEEVENT_NORMAL-only snapshot the
// Go tree already computes; world-event rows (world_event != 0) report their
// schedule columns but their state-driven active flag has no Go model.
// The eight LANG 586 args print in C++ order across the six lines; the
// occurrence/length render via secsToTimeStringFull (Util.cpp:97 FullText)
// and the timestamps via timeToTimestampStr (Util.cpp:272), both verified
// against the C++ helpers. TDB-TEXT GAP: the LANG 586 format string itself
// lives in the trinity_string table (TDB data, not in-repo — see
// handleEventActiveList), so the line labels and the single- vs multi-line
// split follow the prior rendering and are unverified against the TDB text.
func (s *session) handleEventInfo(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .event info <eventId>"
	// ChatCommand.h:112: trailing tokens fail the single-arg parse.
	if len(args) != 1 {
		return false
	}
	id, err := parseGameEventID(args[0])
	if err != nil {
		s.sendSysMessage(syntax)
		return true
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	data := s.server.loadGameEventDataMap(ctx)
	ev, ok := data[id]
	if !ok || !ev.isValid() {
		s.sendSysMessage(fmt.Sprintf("Event %d does not exist.", id))
		return true
	}
	_, isActive := s.server.cachedActiveGameEvents(ctx)[id]
	activeStr := ""
	if isActive {
		activeStr = "Active"
	}
	now := time.Now().Unix()
	delay := gameEventNextCheck(ev, now)
	nextStr := "-"
	// C++ (cs_event.cpp:114-115): the next-change time prints whenever
	// gameTime + NextCheck lands in [start, end) — including a start more
	// than a day out, which the old <86400 guard wrongly suppressed.
	if now+delay >= ev.Start && now+delay < ev.End {
		nextStr = timeToTimestampStr(time.Unix(now+delay, 0))
	}
	s.sendSysMessage(fmt.Sprintf("Event %d: %s %s", id, ev.Description, activeStr))
	s.sendSysMessage(fmt.Sprintf("Start: %s", timeToTimestampStr(time.Unix(ev.Start, 0))))
	s.sendSysMessage(fmt.Sprintf("End: %s", timeToTimestampStr(time.Unix(ev.End, 0))))
	s.sendSysMessage(fmt.Sprintf("Occurence: %s", secsToTimeStringFull(uint64(ev.Occurrence)*60)))
	s.sendSysMessage(fmt.Sprintf("Length: %s", secsToTimeStringFull(uint64(ev.Length)*60)))
	s.sendSysMessage(fmt.Sprintf("Next: %s", nextStr))
	return true
}

// handleEventStart mirrors HandleEventStartCommand (cs_event.cpp:125-152):
// the id checks (LANG 585/587) run against the real game_event rows, then
// StartEvent(eventId, true) runs through the forced-override layer behind
// the StartGameEvent Lua global (luaGameEventControl, conditions.go):
// the event joins the cached active set at once, and the 100ms world-tick
// 34/35 diff fires the start transition; the ApplyNewEvent legs run on
// the same tick transition (applyGameEventTransition: announce +
// worldstate write; spawn application is query-time). DELTAS vs C++: no
// start/end schedule rewrite (force accepted, ignored by the override
// layer), and the hook fires on the next tick rather than synchronously;
// the override persists until the opposite call instead of expiring with
// the schedule window.
func (s *session) handleEventStart(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .event start <eventId>"
	// ChatCommand.h:112: trailing tokens fail the single-arg parse.
	if len(args) != 1 {
		return false
	}
	id, err := parseGameEventID(args[0])
	if err != nil {
		s.sendSysMessage(syntax)
		return true
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	data := s.server.loadGameEventDataMap(ctx)
	ev, ok := data[id]
	if !ok || id < 1 || !ev.isValid() {
		s.sendSysMessage(fmt.Sprintf("Event %d does not exist.", id))
		return true
	}
	if _, isActive := s.server.cachedActiveGameEvents(ctx)[id]; isActive {
		s.sendSysMessage(fmt.Sprintf("Event %d is already active.", id))
		return true
	}
	s.server.luaGameEventControl(uint32(id), true, true)
	return true
}

// handleEventStop mirrors HandleEventStopCommand (cs_event.cpp:154-182):
// the id checks (LANG 585/588) run against the real rows, then
// StopEvent(eventId, true) runs through the forced-override layer behind
// the StopGameEvent Lua global (luaGameEventControl, conditions.go):
// the event leaves the cached active set at once, and the 100ms
// world-tick 34/35 diff fires the stop transition; the UnApplyEvent legs
// run on the same tick transition (applyGameEventTransition: worldstate
// cleanup; spawn removal is query-time). DELTAS vs C++: no forced-state
// DB reset, and the hook fires on the next tick rather than
// synchronously.
func (s *session) handleEventStop(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .event stop <eventId>"
	// ChatCommand.h:112: trailing tokens fail the single-arg parse.
	if len(args) != 1 {
		return false
	}
	id, err := parseGameEventID(args[0])
	if err != nil {
		s.sendSysMessage(syntax)
		return true
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	data := s.server.loadGameEventDataMap(ctx)
	ev, ok := data[id]
	if !ok || id < 1 || !ev.isValid() {
		s.sendSysMessage(fmt.Sprintf("Event %d does not exist.", id))
		return true
	}
	if _, isActive := s.server.cachedActiveGameEvents(ctx)[id]; !isActive {
		s.sendSysMessage(fmt.Sprintf("Event %d is not active.", id))
		return true
	}
	s.server.luaGameEventControl(uint32(id), false, true)
	return true
}

// persistExtraFlags writes extra_flags immediately so GM mode survives
// restarts the way TrinityCore's SaveToDB round-trip does.
func (s *session) persistExtraFlags() {
	if s.player == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.Exec("UPDATE characters SET extra_flags = ? WHERE guid = ?", s.player.ExtraFlags, s.playerGUID)
}

// commandNode mirrors TrinityCore ChatCommandNode resolution: tokens are
// matched case-insensitively against children by unique prefix, aliases
// cover names that are not prefixes, and ambiguous tokens list candidates.
type commandNode struct {
	name     string
	children map[string]*commandNode
	aliases  map[string]string // alias -> canonical child name
	invoke   func(ctx context.Context, args []string) bool
	// perm is the node's RBAC permission (rbac::RBAC_PERM_COMMAND_*); 0
	// means the C++ node carries none (old-form sub-table group) or the
	// family is still unwired, and the node counts as visible (see
	// commandNodeVisible). New-form group nodes carry their own
	// permission; visibility still falls through to subcommands.
	perm uint32
}

func (n *commandNode) add(name string, invoke func(ctx context.Context, args []string) bool, subs []string, aliases map[string]string) *commandNode {
	child := &commandNode{name: name, invoke: invoke, children: make(map[string]*commandNode), aliases: make(map[string]string)}
	for _, sub := range subs {
		child.children[sub] = &commandNode{name: sub}
	}
	for alias, target := range aliases {
		child.aliases[alias] = target
	}
	n.children[name] = child
	return child
}

// withPerm records the node's own RBAC permission (new-form ChatCommand
// group entries carry one, e.g. titles TITLES, modify speed MODIFY_SPEED)
// and returns the node so a root.add / addWithPerms call can chain it.
func (n *commandNode) withPerm(perm uint32) *commandNode {
	n.perm = perm
	return n
}

// addWithPerms mirrors add but records the RBAC permission of each
// subcommand, as the ChatCommandTable entries do (cs_arena.cpp:45-53,
// cs_ban.cpp:51-76, cs_bf.cpp:44, and the R13 census for every other
// family). resolve() filters prefix matches to the invoker-visible
// nodes (FilteredCommandListIterator, ChatCommand.cpp:207-237). A 0
// entry means the C++ node carries no permission (old-form sub-table
// group: character deleted, account set/lock/2fa, gobject add/set,
// lookup player, npc add/set) and stays visible.
// group returns the named child group node, creating it when absent.
// A group node carries no handler and no permission of its own (the
// C++ sub-table has none; the family's bare handler lives on the ""
// entry, ChatCommand.cpp:46-55): visibility falls through to its
// children via commandNodeVisible. Existing bare children registered
// by add/addWithPerms are reused, so a 2-level "set"/"lock"/"2fa"
// entry deepens in place.
func (n *commandNode) group(name string) *commandNode {
	if n.children == nil {
		n.children = make(map[string]*commandNode)
	}
	if child, ok := n.children[name]; ok {
		if child.children == nil {
			child.children = make(map[string]*commandNode)
		}
		if child.aliases == nil {
			child.aliases = make(map[string]string)
		}
		return child
	}
	child := &commandNode{name: name, children: make(map[string]*commandNode), aliases: make(map[string]string)}
	n.children[name] = child
	return child
}

// leaf returns the named child leaf node with its handler and RBAC
// permission, creating it when absent. The handler receives only the
// tokens after the leaf name, as the C++ invoker receives the tail
// after the deepest matched node (ChatCommand.cpp:246-300).
func (n *commandNode) leaf(name string, invoke func(ctx context.Context, args []string) bool, perm uint32) *commandNode {
	child := n.group(name)
	child.invoke = invoke
	child.perm = perm
	return child
}

func (n *commandNode) addWithPerms(name string, invoke func(ctx context.Context, args []string) bool, subPerms map[string]uint32, aliases map[string]string) *commandNode {
	child := &commandNode{name: name, invoke: invoke, children: make(map[string]*commandNode), aliases: make(map[string]string)}
	for sub, perm := range subPerms {
		child.children[sub] = &commandNode{name: sub, perm: perm}
	}
	for alias, target := range aliases {
		child.aliases[alias] = target
	}
	n.children[name] = child
	return child
}

// commandTokens holds the canonicalized tokens once resolution completes.
type commandTokens []string

// resolve walks the tree token by token, rewriting each token to the
// canonical child name it matched (prefix or alias). Returns the deepest
// node and the rewritten tokens; consumed counts matched structural tokens.
// Only invoker-visible children match (FilteredCommandListIterator,
// ChatCommand.cpp:207-237), so a prefix that matches several names but
// leaves one visible resolves to it instead of reporting ambiguity; a nil
// visible predicate matches everything.
func (n *commandNode) resolve(tokens []string, visible func(*commandNode) bool) (*commandNode, commandTokens, int, []string, int) {
	node := n
	rewritten := make(commandTokens, 0, len(tokens))
	consumed := 0
	nodeDepth := 0
	// Once an invoke-bearing node is reached, the next token is its
	// subcommand and is canonicalized against its children (so 'spee' ->
	// 'speed'); every token after that is the handler's raw parameter and
	// passes through verbatim. C++ TryExecuteCommand descends only
	// through subcommand maps; once the leaf invoker is reached the
	// remainder is the handler's raw args, so canonicalizing later tokens
	// rewrites free-text parameters ('.ban account c ...' banned
	// 'CHARACTER', '.arena lookup c' falsely reported ambiguity).
	paramDone := false
	for _, token := range tokens {
		// The subcommand token is the only parameter token matched
		// against this node's children; anything after it is the
		// handler's raw argument and must not be rewritten or
		// reported ambiguous against the sibling subcommands.
		if node.invoke != nil && paramDone {
			break
		}
		lower := strings.ToLower(token)
		matches := make([]string, 0, 2)
		for name, child := range node.children {
			if visible != nil && !visible(child) {
				continue
			}
			if strings.HasPrefix(name, lower) {
				matches = append(matches, name)
			}
		}
		if target, ok := node.aliases[lower]; ok {
			if visible == nil || visible(node.children[target]) {
				duplicate := false
				for _, existing := range matches {
					if existing == target {
						duplicate = true
						break
					}
				}
				if !duplicate {
					matches = append(matches, target)
				}
			}
		}
		if len(matches) == 0 {
			break
		}
		// Exact name matches take priority over partial ones
		// (ChatCommandNode::TryExecuteCommand skips the ambiguity check
		// when the token equals a child name verbatim).
		exact := make([]string, 0, 1)
		for _, candidate := range matches {
			if candidate == lower {
				exact = append(exact, candidate)
			}
		}
		if len(exact) == 1 {
			matches = exact
		}
		if len(matches) > 1 {
			sort.Strings(matches)
			return node, rewritten, consumed, matches, nodeDepth
		}
		canonical := matches[0]
		child := node.children[canonical]
		if child == nil {
			break
		}
		// Descend into the child when it is structural: it carries its
		// own handler or further subcommands (the 3-level groups the
		// R13 census wires, e.g. account set sec email, debug play
		// cinematic — C++ tokenizes multi-word table names into real
		// tree levels, LoadCommandsIntoMap, ChatCommand.cpp:57-70, and
		// TryExecuteCommand descends them with prefix matching at
		// every level). A bare child with neither is a parameter name
		// of the current handler (the 2-level registration): the
		// subcommand token only. Later tokens are raw handler
		// parameters and must not be rewritten against the sibling
		// subcommands (see the resolve comment above).
		if node.invoke != nil && child.invoke == nil && len(child.children) == 0 {
			paramDone = true
			rewritten = append(rewritten, canonical)
			consumed++
			continue
		}
		node = child
		nodeDepth = len(rewritten) + 1
		rewritten = append(rewritten, canonical)
		consumed++
	}
	return node, rewritten, consumed, nil, nodeDepth
}

// ---- ban_commandscript (cs_ban.cpp) ----

// Ban modes mirror BanMode (SharedDefines.h:3487): BAN_ACCOUNT,
// BAN_CHARACTER, BAN_IP.
const (
	banModeAccount = iota
	banModeCharacter
	banModeIP
)

// banResult mirrors BanReturn (SharedDefines.h:3493): BAN_SUCCESS,
// BAN_SYNTAX_ERROR, BAN_NOTFOUND, BAN_EXISTS.
type banResult int

const (
	banSuccess banResult = iota
	banSyntaxError
	banNotFound
	banExists
)

// timeStringToSecs mirrors TimeStringToSecs (Util.cpp): parses duration
// strings like "1d2h30m"; returns 0 on bad format.
func timeStringToSecs(timestring string) uint32 {
	var secs, buffer uint32
	for i := 0; i < len(timestring); i++ {
		c := timestring[i]
		if c >= '0' && c <= '9' {
			buffer *= 10
			buffer += uint32(c - '0')
			continue
		}
		var multiplier uint32
		switch c {
		case 'd':
			multiplier = 86400 // DAY
		case 'h':
			multiplier = 3600 // HOUR
		case 'm':
			multiplier = 60 // MINUTE
		case 's':
			multiplier = 1
		default:
			return 0 // bad format
		}
		buffer *= multiplier
		secs += buffer
		buffer = 0
	}
	return secs
}

// secsToTimeStringShort mirrors secsToTimeString(secs,
// TimeFormat::ShortText) (Util.cpp): "1d2h3m4s", omitting zero units.
func secsToTimeStringShort(timeInSecs uint64) string {
	secs := timeInSecs % 60
	minutes := timeInSecs % 3600 / 60
	hours := timeInSecs % 86400 / 3600
	days := timeInSecs / 86400
	var b strings.Builder
	if days > 0 {
		b.WriteString(strconv.FormatUint(days, 10))
		b.WriteByte('d')
	}
	if hours > 0 {
		b.WriteString(strconv.FormatUint(hours, 10))
		b.WriteByte('h')
	}
	if minutes > 0 {
		b.WriteString(strconv.FormatUint(minutes, 10))
		b.WriteByte('m')
	}
	if secs > 0 || (days == 0 && hours == 0 && minutes == 0) {
		b.WriteString(strconv.FormatUint(secs, 10))
		b.WriteByte('s')
	}
	return b.String()
}

// secsToTimeStringFull mirrors secsToTimeString(secs, TimeFormat::FullText)
// (Util.cpp:97): "1 Day 2 Hours 3 Minutes 4 Seconds.", omitting zero units
// except a bare 0, which renders "0 Second.". cs_event.cpp calls it with the
// default FullText format for the event occurrence and length lines.
func secsToTimeStringFull(timeInSecs uint64) string {
	secs := timeInSecs % 60
	minutes := timeInSecs % 3600 / 60
	hours := timeInSecs % 86400 / 3600
	days := timeInSecs / 86400
	var b strings.Builder
	if days > 0 {
		b.WriteString(strconv.FormatUint(days, 10))
		if days == 1 {
			b.WriteString(" Day ")
		} else {
			b.WriteString(" Days ")
		}
	}
	if hours > 0 {
		b.WriteString(strconv.FormatUint(hours, 10))
		if hours <= 1 {
			b.WriteString(" Hour ")
		} else {
			b.WriteString(" Hours ")
		}
	}
	if minutes > 0 {
		b.WriteString(strconv.FormatUint(minutes, 10))
		if minutes == 1 {
			b.WriteString(" Minute ")
		} else {
			b.WriteString(" Minutes ")
		}
	}
	if secs > 0 || (days == 0 && hours == 0 && minutes == 0) {
		b.WriteString(strconv.FormatUint(secs, 10))
		if secs <= 1 {
			b.WriteString(" Second.")
		} else {
			b.WriteString(" Seconds.")
		}
	}
	return b.String()
}

// timeToTimestampStr mirrors TimeToTimestampStr (Util.cpp:272):
// "YYYY-MM-DD_HH-MM-SS" in local time.
func timeToTimestampStr(t time.Time) string {
	return t.Local().Format("2006-01-02_15-04-05")
}

// gameEventNextCheck mirrors GameEventMgr::NextCheck (GameEventMgr.cpp:81-114)
// for GAMEEVENT_NORMAL rows: the delay until the next state change of the
// event, capped at max_ge_check_delay (1 day, GameEventMgr.h:31). The
// world-event state branches (NEXTPHASE/FINISHED/CONDITIONS) have no Go
// model; the normal-row schedule math applies to every row.
func gameEventNextCheck(ev gameEventFull, now int64) int64 {
	const maxGeCheckDelay = 86400
	if now > ev.End {
		return maxGeCheckDelay
	}
	if ev.Start > now {
		return ev.Start - now
	}
	occurrence := ev.Occurrence * 60
	length := ev.Length * 60
	if occurrence <= 0 {
		return maxGeCheckDelay
	}
	elapsed := (now - ev.Start) % occurrence
	var delay int64
	if elapsed < length {
		delay = length - elapsed
	} else {
		delay = occurrence - elapsed
	}
	if ev.End < now+delay {
		return ev.End - now
	}
	return delay
}

// parseGameEventID mirrors the Variant<Hyperlink<gameevent>, uint16> arm
// parameter (cs_event.cpp): a bare event number, or an |Hgameevent:id|h link.
func parseGameEventID(arg string) (int64, error) {
	t := strings.TrimSpace(arg)
	if strings.HasPrefix(t, "|Hgameevent:") {
		t = t[len("|Hgameevent:"):]
		j := 0
		for j < len(t) && t[j] >= '0' && t[j] <= '9' {
			j++
		}
		if j == 0 || j >= len(t) || t[j] != '|' {
			return 0, fmt.Errorf("invalid game event id %q", arg)
		}
		t = t[:j]
	}
	// Variant<Hyperlink<gameevent>, uint16> parses base-0
	// (StringConvert.h:43-69): 0x/0b accepted, never octal, no sign,
	// no trailing junk, 0-65535.
	v, ok := parseCppBase0Int(t)
	if !ok || v < 0 || v > 65535 {
		return 0, fmt.Errorf("invalid game event id %q", arg)
	}
	return v, nil
}

// cAtoi mirrors C's atoi (cs_ban.cpp gates durations with !atoi(durationStr)):
// skips leading whitespace, takes an optional sign and the digit run,
// returns 0 when no digits are present.
func cAtoi(s string) int {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\v' || s[i] == '\f' || s[i] == '\r') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	n, digits := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		digits++
		i++
	}
	if digits == 0 {
		return 0
	}
	if neg {
		return -n
	}
	return n
}

// isBanIPAddress mirrors the IsIPAddress argument check of the .ban ip /
// .baninfo ip / .unban ip handlers (Util.cpp:295-303): boost
// make_address, which accepts IPv4 and IPv6, as net.ParseIP does.
func isBanIPAddress(s string) bool {
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

// banAuthorName mirrors ChatHandler::GetSession()->GetPlayerName() with the
// "Server" fallback (cs_ban.cpp HandleBanHelper).
func (s *session) banAuthorName() string {
	if s.player != nil && s.player.Name != "" {
		return s.player.Name
	}
	return "Server"
}

// sessionForPlayerName mirrors ObjectAccessor::FindConnectedPlayerByName for
// the ban command paths: the online session whose player has the name.
func (s *session) sessionForPlayerName(name string) *session {
	if s.server == nil {
		return nil
	}
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for sess := range s.server.sessions {
		if sess != nil && sess.player != nil && sess.player.Name == name {
			return sess
		}
	}
	return nil
}

// kickSession disconnects one session (WorldSession::KickPlayer equivalent
// used by the account-delete path).
func (s *session) kickSession(target *session) {
	if s == nil || s.server == nil {
		return
	}
	s.server.kickSession(target)
}

// kickSession disconnects one session (WorldSession::KickPlayer equivalent
// used by the account-delete path). The Server form is shared by the
// Eluna Kick global, which has no calling session.
func (s *Server) kickSession(target *session) {
	if s == nil || target == nil {
		return
	}
	s.sessionsMu.Lock()
	delete(s.sessions, target)
	s.sessionsMu.Unlock()
	if target.conn != nil {
		_ = target.conn.Close()
	}
}

// kickAccountSessions mirrors the World::BanAccount kick loop
// (World.cpp:2866-2871): disconnects every session of the account except
// the author's own session. C++ kicks a single FindSession because its map
// is account-keyed; Go's map is session-keyed, so the sweep is the same
// kick once the login duplicate-kick invariant (one live session per
// account) holds — and the sweep also cleans up any race leftover.
func (s *session) kickAccountSessions(accountID uint32, author string) {
	if s == nil || s.server == nil {
		return
	}
	s.server.kickAccountSessions(accountID, author)
}

// kickAccountSessions is the Server form of the session helper above,
// shared by the Eluna Ban global.
func (s *Server) kickAccountSessions(accountID uint32, author string) {
	if s == nil {
		return
	}
	var victims []*session
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess == nil || sess.accountID != accountID {
			continue
		}
		name := ""
		if sess.player != nil {
			name = sess.player.Name
		}
		if name == author {
			continue
		}
		victims = append(victims, sess)
	}
	s.sessionsMu.RUnlock()
	for _, v := range victims {
		s.kickSession(v)
	}
}

// worldBanAccount mirrors World::BanAccount (World.cpp:2785): refuses an
// already-banned account, inserts the ban rows, and kicks affected sessions
// (for IP, every session on that IP). An IP ban succeeds even when nobody is
// affected yet.
func (s *session) worldBanAccount(ctx context.Context, mode int, nameOrIP, durationStr, reason, author string) banResult {
	if s == nil || s.server == nil {
		return banSyntaxError
	}
	return s.server.banAccount(ctx, mode, nameOrIP, timeStringToSecs(durationStr), reason, author)
}

// banAccount is the Server form of the session helper above, shared by
// the Eluna Ban global, which takes seconds directly like the C++
// uint32 BanAccount overload Eluna calls.
func (s *Server) banAccount(ctx context.Context, mode int, nameOrIP string, durationSecs uint32, reason, author string) banResult {
	if s == nil || s.AuthStore == nil || s.CharactersStore == nil {
		return banSyntaxError
	}
	auth := s.AuthStore
	// AccountMgr::IsBannedAccount (AccountMgr.cpp): prevent banning an
	// already banned account.
	if mode == banModeAccount {
		var id uint32
		var uname string
		if row, err := auth.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_BANNED_BY_USERNAME", nameOrIP); err == nil {
			if err := row.Scan(&id, &uname); err == nil {
				return banExists
			}
		}
	}
	var accounts []uint32
	if mode == banModeIP {
		// LOGIN_SEL_ACCOUNT_BY_IP: accounts to kick; the IP row is
		// inserted even when the list is empty.
		rows, err := auth.QueryStatement(ctx, "LOGIN_SEL_ACCOUNT_BY_IP", nameOrIP)
		if err != nil {
			return banSyntaxError
		}
		for rows.Next() {
			var id uint32
			var uname string
			if err := rows.Scan(&id, &uname); err != nil {
				rows.Close()
				return banSyntaxError
			}
			accounts = append(accounts, id)
		}
		rows.Close()
		// LOGIN_INS_IP_BANNED.
		if _, err := auth.ExecStatement(ctx, "LOGIN_INS_IP_BANNED", nameOrIP, durationSecs, author, reason); err != nil {
			return banSyntaxError
		}
	} else {
		var id uint32
		var err error
		if mode == banModeAccount {
			// LOGIN_SEL_ACCOUNT_ID_BY_NAME.
			row, qerr := auth.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_ID_BY_NAME", nameOrIP)
			if qerr == nil {
				err = row.Scan(&id)
			} else {
				err = qerr
			}
		} else {
			// CHAR_SEL_ACCOUNT_BY_NAME.
			row, qerr := s.CharactersStore.QueryRowStatement(ctx, "CHAR_SEL_ACCOUNT_BY_NAME", nameOrIP)
			if qerr == nil {
				err = row.Scan(&id)
			} else {
				err = qerr
			}
		}
		if err != nil || id == 0 {
			return banNotFound // Nobody to ban
		}
		accounts = []uint32{id}
		if err := auth.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
			for _, account := range accounts {
				// LOGIN_UPD_ACCOUNT_NOT_BANNED: make sure there is only
				// one active ban.
				if _, err := auth.ExecStatementTx(txCtx, tx, "LOGIN_UPD_ACCOUNT_NOT_BANNED", account); err != nil {
					return err
				}
				// LOGIN_INS_ACCOUNT_BANNED.
				if _, err := auth.ExecStatementTx(txCtx, tx, "LOGIN_INS_ACCOUNT_BANNED", account, durationSecs, author, reason); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return banSyntaxError
		}
	}
	// Disconnect all affected players (for IP it can be several).
	for _, account := range accounts {
		s.kickAccountSessions(account, author)
	}
	return banSuccess
}

// worldBanCharacter mirrors World::BanCharacter (World.cpp:2907): bans the
// character by name, kicking it when online.
func (s *session) worldBanCharacter(ctx context.Context, name, durationStr, reason, author string) banResult {
	if s.server == nil || s.server.CharactersStore == nil {
		return banSyntaxError
	}
	chars := s.server.CharactersStore
	durationSecs := timeStringToSecs(durationStr)
	// ObjectAccessor::FindConnectedPlayerByName, else the character cache.
	var guid uint64
	online := s.sessionForPlayerName(name)
	if online != nil {
		guid = online.playerGUID
	} else {
		// sCharacterCache->GetCharacterGuidByName.
		if err := chars.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&guid); err != nil || guid == 0 {
			return banNotFound // Nobody to ban
		}
	}
	// Use transaction in order to ensure the order of the queries.
	if err := chars.RunTransaction(ctx, func(txCtx context.Context, tx *sql.Tx) error {
		// CHAR_UPD_CHARACTER_BAN + CHAR_INS_CHARACTER_BAN.
		if _, err := chars.ExecStatementTx(txCtx, tx, "CHAR_UPD_CHARACTER_BAN", guid); err != nil {
			return err
		}
		if _, err := chars.ExecStatementTx(txCtx, tx, "CHAR_INS_CHARACTER_BAN", guid, durationSecs, author, reason); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return banSyntaxError
	}
	if online != nil {
		s.kickSession(online)
	}
	return banSuccess
}

// worldRemoveBanAccount mirrors World::RemoveBanAccount (World.cpp:2878).
func (s *session) worldRemoveBanAccount(ctx context.Context, mode int, nameOrIP string) bool {
	if s.server == nil || s.server.AuthStore == nil || s.server.CharactersStore == nil {
		return false
	}
	auth := s.server.AuthStore
	if mode == banModeIP {
		// LOGIN_DEL_IP_NOT_BANNED.
		_, _ = auth.ExecStatement(ctx, "LOGIN_DEL_IP_NOT_BANNED", nameOrIP)
		return true
	}
	var account uint32
	if mode == banModeAccount {
		// AccountMgr::GetId.
		row, err := auth.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_ID_BY_NAME", nameOrIP)
		if err != nil || row.Scan(&account) != nil {
			return false
		}
	} else {
		// sCharacterCache->GetCharacterAccountIdByName.
		row, err := s.server.CharactersStore.QueryRowStatement(ctx, "CHAR_SEL_ACCOUNT_BY_NAME", nameOrIP)
		if err != nil || row.Scan(&account) != nil {
			return false
		}
	}
	if account == 0 {
		return false
	}
	// LOGIN_UPD_ACCOUNT_NOT_BANNED.
	_, _ = auth.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_NOT_BANNED", account)
	return true
}

// worldRemoveBanCharacter mirrors World::RemoveBanCharacter (World.cpp:2947).
func (s *session) worldRemoveBanCharacter(ctx context.Context, name string) bool {
	if s.server == nil || s.server.CharactersStore == nil {
		return false
	}
	chars := s.server.CharactersStore
	var guid uint64
	if online := s.sessionForPlayerName(name); online != nil {
		guid = online.playerGUID
	} else {
		if err := chars.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&guid); err != nil || guid == 0 {
			return false
		}
	}
	// CHAR_UPD_CHARACTER_BAN.
	_, _ = chars.ExecStatement(ctx, "CHAR_UPD_CHARACTER_BAN", guid)
	return true
}

// handleCmdBan dispatches ".ban account|character|playeraccount|ip"
// (cs_ban.cpp banCommandTable), gating each arm on its RBAC permission.
func (s *session) handleCmdBan(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "account":
		perm = permissionCommandBanAccount
	case "character":
		perm = permissionCommandBanCharacter
	case "playeraccount":
		perm = permissionCommandBanPlayerAccount
	case "ip":
		perm = permissionCommandBanIP
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "account":
		return s.handleBanHelper(ctx, banModeAccount, rest)
	case "character":
		return s.handleCmdBanCharacter(ctx, rest)
	case "playeraccount":
		return s.handleBanHelper(ctx, banModeCharacter, rest)
	case "ip":
		return s.handleBanHelper(ctx, banModeIP, rest)
	}
	return false
}

// handleBanHelper mirrors HandleBanHelper (cs_ban.cpp:183): parses
// "<name|ip> <duration> <reason...>", validates per mode, and applies
// World::BanAccount, reporting BAN_SUCCESS / BAN_NOTFOUND / BAN_EXISTS.
func (s *session) handleBanHelper(ctx context.Context, mode int, args []string) bool {
	if len(args) < 3 {
		return false
	}
	nameOrIP := args[0]
	durationStr := args[1]
	reason := strings.Join(args[2:], " ")
	// C++: if (!durationStr || !atoi(durationStr)) return false.
	if cAtoi(durationStr) == 0 || reason == "" {
		return false
	}
	switch mode {
	case banModeAccount:
		if !utf8.ValidString(nameOrIP) {
			// LANG_ACCOUNT_NOT_EXIST (413): Utf8ToUpperOnlyLatin fails
			// on invalid UTF-8 (Util.cpp) instead of passing the name on.
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", nameOrIP))
			return true
		}
		nameOrIP = upperOnlyLatin(nameOrIP)
	case banModeCharacter:
		if nameOrIP == "" || !utf8.ValidString(nameOrIP) {
			// LANG_PLAYER_NOT_FOUND (499).
			s.sendSysMessage("Player not found.")
			return true
		}
		nameOrIP = normalizePlayerName(nameOrIP)
	case banModeIP:
		if !isBanIPAddress(nameOrIP) {
			// C++ returns false with no error message (cs_ban.cpp:198-199).
			return false
		}
	}
	author := s.banAuthorName()
	switch s.worldBanAccount(ctx, mode, nameOrIP, durationStr, reason, author) {
	case banSuccess:
		// The CONFIG_SHOW_BAN_IN_WORLD branch (SendWorldText with
		// LANG_BAN_ACCOUNT_YOUBANNEDMESSAGE_WORLD 11006 /
		// LANG_BAN_ACCOUNT_YOUPERMBANNEDMESSAGE_WORLD 11007) is not
		// wired: no SendWorldText/SendGlobalText broadcast bridge exists
		// in Go (same gap as the world_chat audit), so the handler-only
		// message is always emitted.
		if cAtoi(durationStr) > 0 {
			// LANG_BAN_YOUBANNED (408).
			s.sendSysMessage(fmt.Sprintf("You have banned %s for %s, reason: %s.", nameOrIP, secsToTimeStringShort(uint64(timeStringToSecs(durationStr))), reason))
		} else {
			// LANG_BAN_YOUPERMBANNED (409).
			s.sendSysMessage(fmt.Sprintf("You have permanently banned %s, reason: %s.", nameOrIP, reason))
		}
	case banSyntaxError:
		return false
	case banNotFound:
		// LANG_BAN_NOTFOUND (410).
		kind := "account"
		if mode == banModeCharacter {
			kind = "character"
		} else if mode == banModeIP {
			kind = "ip"
		}
		s.sendSysMessage(fmt.Sprintf("%s %s not found.", kind, nameOrIP))
	case banExists:
		// LANG_BAN_EXISTS (1188).
		s.sendSysMessage("Account is already banned.")
	}
	return true
}

// handleCmdBanCharacter processes ".ban character <name> <duration> <reason>"
// (cs_ban.cpp HandleBanCharacterCommand).
func (s *session) handleCmdBanCharacter(ctx context.Context, args []string) bool {
	if len(args) < 3 {
		return false
	}
	name := args[0]
	durationStr := args[1]
	reason := strings.Join(args[2:], " ")
	if cAtoi(durationStr) == 0 || reason == "" {
		return false
	}
	if name == "" || !utf8.ValidString(name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return true
	}
	name = normalizePlayerName(name)
	author := s.banAuthorName()
	switch s.worldBanCharacter(ctx, name, durationStr, reason, author) {
	case banSuccess:
		// CONFIG_SHOW_BAN_IN_WORLD branch not wired (see handleBanHelper).
		if cAtoi(durationStr) > 0 {
			// LANG_BAN_YOUBANNED (408).
			s.sendSysMessage(fmt.Sprintf("You have banned %s for %s, reason: %s.", name, secsToTimeStringShort(uint64(timeStringToSecs(durationStr))), reason))
		} else {
			// LANG_BAN_YOUPERMBANNED (409).
			s.sendSysMessage(fmt.Sprintf("You have permanently banned %s, reason: %s.", name, reason))
		}
	case banNotFound:
		// LANG_BAN_NOTFOUND (410).
		s.sendSysMessage(fmt.Sprintf("character %s not found.", name))
	default:
		// The C++ switch handles only BAN_SUCCESS and BAN_NOTFOUND; any
		// other result falls through to return true with no message
		// (cs_ban.cpp HandleBanCharacterCommand), so no syntax line here.
	}
	return true
}

// handleCmdBanInfo dispatches ".baninfo account|character|ip"
// (cs_ban.cpp baninfoCommandTable).
func (s *session) handleCmdBanInfo(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "account":
		perm = permissionCommandBanInfoAccount
	case "character":
		perm = permissionCommandBanInfoCharacter
	case "ip":
		perm = permissionCommandBanInfoIP
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "account":
		return s.handleCmdBanInfoAccount(ctx, rest)
	case "character":
		return s.handleCmdBanInfoCharacter(ctx, rest)
	case "ip":
		return s.handleCmdBanInfoIP(ctx, rest)
	}
	return false
}

// banHistoryEntry is one row of account_banned / character_banned for the
// baninfo history output.
type banHistoryEntry struct {
	bandate  int64
	duration uint64 // unbandate-bandate; 0 = permanent
	active   bool
	unban    int64
	reason   string
	by       string
}

// sendBanHistory mirrors the HandleBanInfoHelper / character-info row loop
// (cs_ban.cpp): LANG_BANINFO_BANHISTORY (417) header followed by one
// LANG_BANINFO_HISTORYENTRY (418) line per row. bandateFmt matches the C++
// source: FROM_UNIXTIME ("2006-01-02 15:04:05") for accounts,
// TimeToTimestampStr ("2006-01-02_15-04-05") for characters.
func (s *session) sendBanHistory(name string, entries []banHistoryEntry, bandateFmt string) {
	// LANG_BANINFO_BANHISTORY (417).
	s.sendSysMessage(fmt.Sprintf("Ban history for %s:", name))
	now := timeNow()
	for _, e := range entries {
		active := e.active && (e.duration == 0 || e.unban >= now)
		banTime := secsToTimeStringShort(e.duration)
		if e.duration == 0 {
			// LANG_BANINFO_INFINITE (419).
			banTime = "Infinite"
		}
		yesNo := "No" // LANG_NO (422)
		if active {
			yesNo = "Yes" // LANG_YES (421)
		}
		// LANG_BANINFO_HISTORYENTRY (418).
		s.sendSysMessage(fmt.Sprintf("%s | %s | active: %s | %s | by %s",
			time.Unix(e.bandate, 0).Format(bandateFmt), banTime, yesNo, e.reason, e.by))
	}
}

// collectBanHistory reads the ban rows for scanBanHistory.
func collectBanHistory(rows *sql.Rows) ([]banHistoryEntry, error) {
	var entries []banHistoryEntry
	for rows.Next() {
		var e banHistoryEntry
		var active int
		if err := rows.Scan(&e.bandate, &e.duration, &active, &e.unban, &e.reason, &e.by); err != nil {
			return nil, err
		}
		e.active = active != 0
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// handleCmdBanInfoAccount processes ".baninfo account <name>"
// (cs_ban.cpp HandleBanInfoAccountCommand / HandleBanInfoHelper).
func (s *session) handleCmdBanInfoAccount(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.AuthStore == nil {
		return true
	}
	if len(args) == 0 {
		return false
	}
	// C++ takes the whole remainder as the account name.
	accountName := upperOnlyLatin(strings.Join(args, " "))
	var accountID uint32
	row, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_ID_BY_NAME", accountName)
	if err != nil || row.Scan(&accountID) != nil || accountID == 0 {
		// LANG_ACCOUNT_NOT_EXIST (413).
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
		return true
	}
	// C++ raw query: SELECT FROM_UNIXTIME(bandate), unbandate-bandate,
	// active, unbandate, banreason, bannedby FROM account_banned
	// WHERE id = ? ORDER BY bandate ASC (FROM_UNIXTIME is MySQL-only; the
	// timestamp is formatted in Go as local time instead).
	rows, err := s.server.AuthStore.DB.QueryContext(ctx, "SELECT bandate, unbandate-bandate, active, unbandate, banreason, bannedby FROM account_banned WHERE id = ? ORDER BY bandate ASC", accountID)
	if err != nil {
		// C++ !result prints LANG_BANINFO_NOACCOUNTBAN (cs_ban.cpp:282-285).
		s.sendSysMessage(fmt.Sprintf("No ban history for account %s.", accountName))
		return true
	}
	entries, err := collectBanHistory(rows)
	rows.Close()
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("No ban history for account %s.", accountName))
		return true
	}
	if len(entries) == 0 {
		// LANG_BANINFO_NOACCOUNTBAN (416).
		s.sendSysMessage(fmt.Sprintf("No ban history for account %s.", accountName))
		return true
	}
	s.sendBanHistory(accountName, entries, "2006-01-02 15:04:05")
	return true
}

// handleCmdBanInfoCharacter processes ".baninfo character <name>"
// (cs_ban.cpp HandleBanInfoCharacterCommand).
func (s *session) handleCmdBanInfoCharacter(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.CharactersStore == nil {
		return true
	}
	if len(args) == 0 {
		return false
	}
	// C++ takes the whole remainder as the name (std::string name(args));
	// a trailing token makes the lookup miss instead of being ignored.
	name := strings.Join(args, " ")
	if name == "" || !utf8.ValidString(name) {
		// LANG_BANINFO_NOCHARACTER (414).
		s.sendSysMessage("Character not found.")
		return true
	}
	name = normalizePlayerName(name)
	var guid uint64
	if online := s.sessionForPlayerName(name); online != nil {
		// ObjectAccessor::FindPlayerByName.
		guid = online.playerGUID
	} else {
		// sCharacterCache->GetCharacterGuidByName.
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", name).Scan(&guid); err != nil || guid == 0 {
			// LANG_BANINFO_NOCHARACTER (414).
			s.sendSysMessage("Character not found.")
			return true
		}
	}
	// CHAR_SEL_BANINFO.
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_BANINFO", guid)
	if err != nil {
		// C++ !result prints LANG_CHAR_NOT_BANNED (cs_ban.cpp:341-344).
		s.sendSysMessage(fmt.Sprintf("%s is not banned.", name))
		return true
	}
	entries, err := collectBanHistory(rows)
	rows.Close()
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("%s is not banned.", name))
		return true
	}
	if len(entries) == 0 {
		// LANG_CHAR_NOT_BANNED (1136).
		s.sendSysMessage(fmt.Sprintf("%s is not banned.", name))
		return true
	}
	s.sendBanHistory(name, entries, "2006-01-02_15-04-05")
	return true
}

// handleCmdBanInfoIP processes ".baninfo ip <ip>"
// (cs_ban.cpp HandleBanInfoIPCommand).
func (s *session) handleCmdBanInfoIP(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.AuthStore == nil {
		return true
	}
	if len(args) == 0 {
		// C++ returns false with no error message (cs_ban.cpp:365-366).
		return false
	}
	// C++ takes the whole remainder (strtok with an empty delimiter), so a
	// trailing token fails the IsIPAddress check instead of being ignored.
	ip := strings.Join(args, " ")
	if !isBanIPAddress(ip) {
		// C++ returns false with no error message (cs_ban.cpp:372-373).
		return false
	}
	// C++ raw query: SELECT ip, FROM_UNIXTIME(bandate),
	// FROM_UNIXTIME(unbandate), unbandate-UNIX_TIMESTAMP(), banreason,
	// bannedby, unbandate-bandate FROM ip_banned WHERE ip = ?.
	var bandate, unbandate int64
	var reason, by string
	err := s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT bandate, unbandate, banreason, bannedby FROM ip_banned WHERE ip = ?", ip).Scan(&bandate, &unbandate, &reason, &by)
	if err != nil {
		// LANG_BANINFO_NOIP (415).
		s.sendSysMessage("IP address is not banned.")
		return true
	}
	permanent := unbandate == bandate
	unbanStr := "Never"   // LANG_BANINFO_NEVER (420)
	banTime := "Infinite" // LANG_BANINFO_INFINITE (419)
	if !permanent {
		unbanStr = time.Unix(unbandate, 0).Format("2006-01-02 15:04:05")
		// C++ selects unbandate-UNIX_TIMESTAMP() raw (cs_ban.cpp:379)
		// and prints it through secsToTimeString (Util.cpp:97). Both
		// columns are int unsigned, so MySQL evaluates the subtraction
		// in unsigned arithmetic and an expired row wraps to a huge
		// uint64 (2^64 - elapsed) instead of going negative; GetUInt64
		// keeps the wrap. Expired rows are reachable here: the cleanup
		// statement (LoginDatabase.cpp:27) runs at authserver start and
		// in the banlist handlers, not in baninfo. The uint64 conversion
		// below reproduces the wrap exactly; the old clamp to 0 was an
		// unintentional deviation.
		banTime = secsToTimeStringShort(uint64(unbandate - timeNow()))
	}
	// LANG_BANINFO_IPENTRY (423).
	s.sendSysMessage(fmt.Sprintf("%s | banned: %s | unban: %s | duration: %s | %s | by %s",
		ip, time.Unix(bandate, 0).Format("2006-01-02 15:04:05"), unbanStr, banTime, reason, by))
	return true
}

// handleCmdBanList dispatches ".banlist account|character|ip [filter]"
// (cs_ban.cpp banlistCommandTable). The Go command path is always a
// session, so only the C++ GetSession() short-output branches are ported
// (same treatment as the arena lookup port).
func (s *session) handleCmdBanList(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "account":
		perm = permissionCommandBanListAccount
	case "character":
		perm = permissionCommandBanListCharacter
	case "ip":
		perm = permissionCommandBanListIP
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "account":
		return s.handleCmdBanListAccount(ctx, rest)
	case "character":
		return s.handleCmdBanListCharacter(ctx, rest)
	case "ip":
		return s.handleCmdBanListIP(ctx, rest)
	}
	return false
}

// handleCmdBanListAccount processes ".banlist account [filter]"
// (cs_ban.cpp HandleBanListAccountCommand).
func (s *session) handleCmdBanListAccount(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.AuthStore == nil {
		return true
	}
	auth := s.server.AuthStore
	// LOGIN_DEL_EXPIRED_IP_BANS runs at the top of the handler.
	_, _ = auth.ExecStatement(ctx, "LOGIN_DEL_EXPIRED_IP_BANS")
	var filter string
	if len(args) > 0 {
		filter = args[0]
	}
	var rows *sql.Rows
	var err error
	if filter == "" {
		// LOGIN_SEL_ACCOUNT_BANNED_ALL.
		rows, err = auth.QueryStatement(ctx, "LOGIN_SEL_ACCOUNT_BANNED_ALL")
	} else {
		// LOGIN_SEL_ACCOUNT_BANNED_BY_FILTER.
		rows, err = auth.QueryStatement(ctx, "LOGIN_SEL_ACCOUNT_BANNED_BY_FILTER", filter)
	}
	if err != nil {
		// C++ !result prints LANG_BANLIST_NOACCOUNT (cs_ban.cpp:420-423).
		s.sendSysMessage("No banned accounts found.")
		return true
	}
	var ids []uint32
	for rows.Next() {
		var id uint32
		var uname string
		if err := rows.Scan(&id, &uname); err != nil {
			rows.Close()
			return true
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		// LANG_BANLIST_NOACCOUNT (425).
		s.sendSysMessage("No banned accounts found.")
		return true
	}
	// LANG_BANLIST_MATCHINGACCOUNT (428).
	s.sendSysMessage("Matching banned accounts:")
	for _, id := range ids {
		var uname string
		if err := auth.DB.QueryRowContext(ctx, "SELECT account.username FROM account, account_banned WHERE account_banned.id = ? AND account_banned.id = account.id", id).Scan(&uname); err == nil {
			s.sendSysMessage(uname)
		}
	}
	return true
}

// handleCmdBanListCharacter processes ".banlist character <filter>"
// (cs_ban.cpp HandleBanListCharacterCommand).
func (s *session) handleCmdBanListCharacter(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.CharactersStore == nil {
		return true
	}
	chars := s.server.CharactersStore
	if len(args) == 0 {
		return false
	}
	// CHAR_SEL_GUID_BY_NAME_FILTER.
	rows, err := chars.QueryStatement(ctx, "CHAR_SEL_GUID_BY_NAME_FILTER", args[0])
	if err != nil {
		// C++ !result prints LANG_BANLIST_NOCHARACTER (cs_ban.cpp:520-523).
		s.sendSysMessage("No banned characters found.")
		return true
	}
	var guids []uint64
	for rows.Next() {
		var guid uint64
		var name string
		if err := rows.Scan(&guid, &name); err != nil {
			rows.Close()
			return true
		}
		guids = append(guids, guid)
	}
	rows.Close()
	if len(guids) == 0 {
		// LANG_BANLIST_NOCHARACTER (426).
		s.sendSysMessage("No banned characters found.")
		return true
	}
	// LANG_BANLIST_MATCHINGCHARACTER (1131).
	s.sendSysMessage("Matching banned characters:")
	for _, guid := range guids {
		// CHAR_SEL_BANNED_NAME.
		row, err := chars.QueryRowStatement(ctx, "CHAR_SEL_BANNED_NAME", guid)
		if err != nil {
			continue
		}
		var name string
		if err := row.Scan(&name); err == nil {
			s.sendSysMessage(name)
		}
	}
	return true
}

// handleCmdBanListIP processes ".banlist ip [filter]"
// (cs_ban.cpp HandleBanListIPCommand).
func (s *session) handleCmdBanListIP(ctx context.Context, args []string) bool {
	if s.server == nil || s.server.AuthStore == nil {
		return true
	}
	auth := s.server.AuthStore
	// LOGIN_DEL_EXPIRED_IP_BANS runs at the top of the handler.
	_, _ = auth.ExecStatement(ctx, "LOGIN_DEL_EXPIRED_IP_BANS")
	var filter string
	if len(args) > 0 {
		filter = args[0]
	}
	var rows *sql.Rows
	var err error
	if filter == "" {
		// LOGIN_SEL_IP_BANNED_ALL.
		rows, err = auth.QueryStatement(ctx, "LOGIN_SEL_IP_BANNED_ALL")
	} else {
		// LOGIN_SEL_IP_BANNED_BY_IP.
		rows, err = auth.QueryStatement(ctx, "LOGIN_SEL_IP_BANNED_BY_IP", filter)
	}
	if err != nil {
		// C++ !result prints LANG_BANLIST_NOIP (cs_ban.cpp:617-620).
		s.sendSysMessage("No banned IPs found.")
		return true
	}
	var ips []string
	for rows.Next() {
		var ip string
		var bandate, unbandate int64
		var by, reason string
		if err := rows.Scan(&ip, &bandate, &unbandate, &by, &reason); err != nil {
			rows.Close()
			return true
		}
		ips = append(ips, ip)
	}
	rows.Close()
	if len(ips) == 0 {
		// LANG_BANLIST_NOIP (424).
		s.sendSysMessage("No banned IPs found.")
		return true
	}
	// LANG_BANLIST_MATCHINGIP (427).
	s.sendSysMessage("Matching banned IPs:")
	for _, ip := range ips {
		s.sendSysMessage(ip)
	}
	return true
}

// handleCmdUnBan dispatches ".unban account|character|playeraccount|ip"
// (cs_ban.cpp unbanCommandTable).
func (s *session) handleCmdUnBan(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	var perm uint32
	switch sub {
	case "account":
		perm = permissionCommandUnBanAccount
	case "character":
		perm = permissionCommandUnBanCharacter
	case "playeraccount":
		perm = permissionCommandUnBanPlayerAccount
	case "ip":
		perm = permissionCommandUnBanIP
	default:
		return false
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	switch sub {
	case "account":
		return s.handleUnBanHelper(ctx, banModeAccount, rest)
	case "character":
		return s.handleCmdUnBanCharacter(ctx, rest)
	case "playeraccount":
		return s.handleUnBanHelper(ctx, banModeCharacter, rest)
	case "ip":
		return s.handleUnBanHelper(ctx, banModeIP, rest)
	}
	return false
}

// handleUnBanHelper mirrors HandleUnBanHelper (cs_ban.cpp:683).
func (s *session) handleUnBanHelper(ctx context.Context, mode int, args []string) bool {
	if len(args) == 0 {
		return false
	}
	nameOrIP := args[0]
	switch mode {
	case banModeAccount:
		if !utf8.ValidString(nameOrIP) {
			// LANG_ACCOUNT_NOT_EXIST (413): Utf8ToUpperOnlyLatin fails
			// on invalid UTF-8 (Util.cpp) instead of passing the name on.
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", nameOrIP))
			return true
		}
		nameOrIP = upperOnlyLatin(nameOrIP)
	case banModeCharacter:
		if nameOrIP == "" || !utf8.ValidString(nameOrIP) {
			// LANG_PLAYER_NOT_FOUND (499).
			s.sendSysMessage("Player not found.")
			return true
		}
		nameOrIP = normalizePlayerName(nameOrIP)
	case banModeIP:
		if !isBanIPAddress(nameOrIP) {
			// C++ returns false with no error message (cs_ban.cpp:745-746).
			return false
		}
	}
	// World::RemoveBanAccount (World.cpp:2878).
	if s.worldRemoveBanAccount(ctx, mode, nameOrIP) {
		// LANG_UNBAN_UNBANNED (411).
		s.sendSysMessage(fmt.Sprintf("%s has been unbanned.", nameOrIP))
	} else {
		// LANG_UNBAN_ERROR (412).
		s.sendSysMessage(fmt.Sprintf("Failed to unban %s.", nameOrIP))
	}
	return true
}

// handleCmdUnBanCharacter processes ".unban character <name>"
// (cs_ban.cpp HandleUnBanCharacterCommand).
func (s *session) handleCmdUnBanCharacter(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		return false
	}
	name := args[0]
	if name == "" || !utf8.ValidString(name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return true
	}
	name = normalizePlayerName(name)
	// World::RemoveBanCharacter (World.cpp:2947); no success message in C++.
	if !s.worldRemoveBanCharacter(ctx, name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return true
	}
	return true
}

// buildCommandTree assembles the command hierarchy with the canonical
// TrinityCore names; aliases mark non-prefix spellings.
func (s *session) buildCommandTree() *commandNode {
	root := &commandNode{name: "", children: make(map[string]*commandNode)}
	root.addWithPerms("gm", func(ctx context.Context, args []string) bool { return s.handleCmdGM(ctx, args) }, map[string]uint32{"chat": permissionCommandGMChat, "fly": permissionCommandGMFly, "ingame": permissionCommandGMIngame, "list": permissionCommandGMList, "visible": permissionCommandGMVisible, "on": permissionCommandGM, "off": permissionCommandGM}, map[string]string{"vis": "visible"})
	root.addWithPerms("cheat", func(ctx context.Context, args []string) bool { return s.handleCmdCheat(ctx, args) }, map[string]uint32{"god": permissionCommandCheatGod, "casttime": permissionCommandCheatCasttime, "cooldown": permissionCommandCheatCooldown, "power": permissionCommandCheatPower, "waterwalk": permissionCommandCheatWaterwalk, "status": permissionCommandCheatStatus, "taxi": permissionCommandCheatTaxi, "explore": permissionCommandCheatExplore}, nil)
	debugNode := root.addWithPerms("debug", func(ctx context.Context, args []string) bool { return s.handleCmdDebug(ctx, args) }, map[string]uint32{"areatriggers": permissionCommandDebug, "anim": permissionCommandDebug, "worldstate": permissionCommandDebug, "objectcount": permissionCommandDebug, "dummy": permissionCommandDebug, "neargraveyard": permissionCommandDebug, "lootrecipient": permissionCommandDebug, "instancespawn": permissionCommandDebug, "los": permissionCommandDebug, "play": permissionCommandDebug, "send": permissionCommandDebug, "threat": permissionCommandDebug, "threatinfo": permissionCommandDebug, "combat": permissionCommandDebug, "guidlimits": permissionCommandDebug, "wpgps": permissionCommandDebug, "transport": permissionCommandDebug, "arena": permissionCommandDebug, "bg": permissionCommandDebug, "questreset": permissionCommandDebug, "raidreset": permissionCommandDebug, "spawnvehicle": permissionCommandDebug, "entervehicle": permissionCommandDebug, "getvalue": permissionCommandDebug, "setvalue": permissionCommandDebug, "setbit": permissionCommandDebug, "mod32value": permissionCommandDebug, "update": permissionCommandDebug, "getitemvalue": permissionCommandDebug, "setitemvalue": permissionCommandDebug, "setaurastate": permissionCommandDebug, "itemexpire": permissionCommandDebug, "moveflags": permissionCommandDebug}, nil)
	// R13 depth (cs_debug.cpp:63-82): "play" and "send" are
	// sub-tables, so debug play cinematic / debug send spellfail are
	// 3-token paths with prefix matching at every level. The arm
	// name is prepended for the existing exact-switch handlers, and
	// the nil-player guard of handleCmdDebug is preserved: the leaf
	// path bypasses the family handler.
	debugPlay := debugNode.group("play")
	for _, arm := range []string{"cinematic", "movie", "sound", "music"} {
		arm := arm
		debugPlay.leaf(arm, func(ctx context.Context, args []string) bool {
			if s.player == nil {
				return true
			}
			return s.handleDebugPlay(ctx, append([]string{arm}, args...))
		}, permissionCommandDebug)
	}
	debugSend := debugNode.group("send")
	for _, arm := range []string{"spellfail", "buyerror", "sellerror", "equiperror", "channelnotify", "chatmessage", "qpartymsg", "qinvalidmsg", "setphaseshift", "largepacket", "opcode"} {
		arm := arm
		debugSend.leaf(arm, func(ctx context.Context, args []string) bool {
			if s.player == nil {
				return true
			}
			return s.handleDebugSend(ctx, append([]string{arm}, args...))
		}, permissionCommandDebug)
	}
	// R13 depth (cs_debug.cpp:118-123): "asan" and "warden" are
	// multi-word sub-tables (debug asan memoryleak, debug warden
	// force), so they are 3-token paths with prefix matching at
	// every level. The asan arms are #ifdef ASAN only in C++ and
	// silent no-ops on a non-ASAN build; warden force has no Go
	// Warden and always reports the system disabled.
	debugASAN := debugNode.group("asan")
	debugASAN.leaf("memoryleak", func(ctx context.Context, args []string) bool {
		return s.handleDebugASANMemoryLeak(ctx, args)
	}, permissionCommandDebug)
	debugASAN.leaf("outofbounds", func(ctx context.Context, args []string) bool {
		return s.handleDebugASANOutOfBounds(ctx, args)
	}, permissionCommandDebug)
	debugWarden := debugNode.group("warden")
	debugWarden.leaf("force", func(ctx context.Context, args []string) bool {
		return s.handleDebugWardenForce(ctx, args)
	}, permissionCommandDebug)
	root.addWithPerms("tele", func(ctx context.Context, args []string) bool { return s.handleCmdTele(ctx, args) }, map[string]uint32{"add": permissionCommandTeleAdd, "del": permissionCommandTeleDel, "name": permissionCommandTeleName, "group": permissionCommandTeleGroup}, nil)
	ticketNode := root.addWithPerms("ticket", func(ctx context.Context, args []string) bool { return s.handleCmdTicket(ctx, args) }, map[string]uint32{"assign": permissionCommandTicketAssign, "close": permissionCommandTicketClose, "closedlist": permissionCommandTicketClosedlist, "comment": permissionCommandTicketComment, "complete": permissionCommandTicketComplete, "delete": permissionCommandTicketDelete, "escalate": permissionCommandTicketEscalate, "escalatedlist": permissionCommandTicketEscalatedlist, "list": permissionCommandTicketList, "onlinelist": permissionCommandTicketOnlinelist, "reset": permissionCommandTicketReset, "response": permissionCommandTicketResponse, "togglesystem": permissionCommandTicketTogglesystem, "unassign": permissionCommandTicketUnassign, "viewid": permissionCommandTicketViewid, "viewname": permissionCommandTicketViewname}, nil).withPerm(permissionCommandTicket)
	// R13 depth (cs_ticket.cpp:52-53): "response" is a sub-table
	// with append/appendln leaves and no "" entry, so the group node
	// stays handler-less. The family switch in handleCmdTicket stays
	// as the fallback for an invisible group.
	ticketResponse := ticketNode.group("response")
	ticketResponse.leaf("append", func(ctx context.Context, args []string) bool {
		return s.handleTicketResponse(ctx, append([]string{"append"}, args...))
	}, permissionCommandTicketResponseAppend)
	ticketResponse.leaf("appendln", func(ctx context.Context, args []string) bool {
		return s.handleTicketResponse(ctx, append([]string{"appendln"}, args...))
	}, permissionCommandTicketResponseAppendln)
	root.addWithPerms("titles", func(ctx context.Context, args []string) bool { return s.handleCmdTitles(ctx, args) }, map[string]uint32{"add": permissionCommandTitlesAdd, "current": permissionCommandTitlesCurrent, "remove": permissionCommandTitlesRemove, "set": permissionCommandTitlesSet}, nil).withPerm(permissionCommandTitles)
	root.addWithPerms("wp", func(ctx context.Context, args []string) bool { return s.handleCmdWp(ctx, args) }, map[string]uint32{"add": permissionCommandWpAdd, "event": permissionCommandWpEvent, "load": permissionCommandWpLoad, "modify": permissionCommandWpModify, "unload": permissionCommandWpUnload, "reload": permissionCommandWpReload, "show": permissionCommandWpShow}, nil).withPerm(permissionCommandWp)
	goNode := root.addWithPerms("go", func(ctx context.Context, args []string) bool { return s.handleCmdGo(ctx, args) }, map[string]uint32{"creature": permissionCommandGO, "gameobject": permissionCommandGO, "graveyard": permissionCommandGO, "grid": permissionCommandGO, "taxinode": permissionCommandGO, "areatrigger": permissionCommandGO, "zonexy": permissionCommandGO, "xyz": permissionCommandGO, "ticket": permissionCommandGO, "offset": permissionCommandGO, "instance": permissionCommandGO, "boss": permissionCommandGO}, nil)
	// R13 depth (cs_go.cpp:48-61): "creature id" and "gameobject id"
	// are multi-word entries, so creature/gameobject are 3-token
	// paths; each also carries the bare spawn-id handler, so the
	// group node keeps its handler via the family switch.
	goCreature := goNode.group("creature")
	goCreature.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdGo(ctx, append([]string{"creature"}, args...))
	}
	goCreature.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdGo(ctx, append([]string{"creature", "id"}, args...))
	}, permissionCommandGO)
	goGameObject := goNode.group("gameobject")
	goGameObject.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdGo(ctx, append([]string{"gameobject"}, args...))
	}
	goGameObject.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdGo(ctx, append([]string{"gameobject", "id"}, args...))
	}, permissionCommandGO)
	modifyNode := root.addWithPerms("modify", func(ctx context.Context, args []string) bool { return s.handleCmdModify(ctx, args) }, map[string]uint32{"hp": permissionCommandModifyHP, "mana": permissionCommandModifyMana, "energy": permissionCommandModifyEnergy, "rage": permissionCommandModifyRage, "runicpower": permissionCommandModifyRunicPower, "money": permissionCommandModifyMoney, "honor": permissionCommandModifyHonor, "arenapoints": permissionCommandModifyArenaPoints, "xp": permissionCommandModifyXP, "drunk": permissionCommandModifyDrunk, "scale": permissionCommandModifyScale, "spell": permissionCommandModifySpell, "standstate": permissionCommandModifyStandState, "mount": permissionCommandModifyMount, "gender": permissionCommandModifyGender, "bit": permissionCommandModifyBit, "faction": permissionCommandModifyFaction, "phase": permissionCommandModifyPhase, "speed": permissionCommandModifySpeed, "talentpoints": permissionCommandModifyTalentPoints, "reputation": permissionCommandModifyReputation}, nil).withPerm(permissionCommandModify)
	// R13 depth (cs_modify.cpp:47-52): "speed" is a sub-table with
	// all/backwalk/fly/walk/swim leaves plus a "" entry (the
	// all-speeds handler), so the group node keeps its handler via
	// the family switch.
	modifySpeed := modifyNode.group("speed")
	modifySpeed.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed"}, args...))
	}
	modifySpeed.leaf("all", func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed", "all"}, args...))
	}, permissionCommandModifySpeedAll)
	modifySpeed.leaf("backwalk", func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed", "backwalk"}, args...))
	}, permissionCommandModifySpeedBackwalk)
	modifySpeed.leaf("fly", func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed", "fly"}, args...))
	}, permissionCommandModifySpeedFly)
	modifySpeed.leaf("walk", func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed", "walk"}, args...))
	}, permissionCommandModifySpeedWalk)
	modifySpeed.leaf("swim", func(ctx context.Context, args []string) bool {
		return s.handleCmdModify(ctx, append([]string{"speed", "swim"}, args...))
	}, permissionCommandModifySpeedSwim)
	root.addWithPerms("additem", func(ctx context.Context, args []string) bool { return s.handleCmdAddItem(ctx, args) }, map[string]uint32{"set": permissionCommandAddItemSet}, nil).withPerm(permissionCommandAddItem)
	root.addWithPerms("cast", func(ctx context.Context, args []string) bool { return s.handleCmdCast(ctx, args) }, map[string]uint32{"back": permissionCommandCastBack, "dist": permissionCommandCastDist, "self": permissionCommandCastSelf, "target": permissionCommandCastTarget, "dest": permissionCommandCastDest}, nil).withPerm(permissionCommandCast)
	serverNode := root.addWithPerms("server", func(ctx context.Context, args []string) bool { return s.handleCmdServer(ctx, args) }, map[string]uint32{"corpses": permissionCommandServerCorpses, "debug": permissionCommandServerDebug, "exit": permissionCommandServerExit, "idlerestart": permissionCommandServerIdlerestart, "idleshutdown": permissionCommandServerIdleshutdown, "info": permissionCommandServerInfo, "motd": permissionCommandServerMotd, "plimit": permissionCommandServerPlimit, "restart": permissionCommandServerRestart, "shutdown": permissionCommandServerShutdown, "set": permissionCommandServerSet}, nil).withPerm(permissionCommandServer)
	// R13 depth (cs_server.cpp:66-94): restart/shutdown/idlerestart/
	// idleshutdown are sub-tables with cancel/force leaves and a ""
	// entry (the timed handler), so those group nodes keep their
	// handler via the family switch; "set" has no "" entry and stays
	// handler-less.
	serverRestart := serverNode.group("restart")
	serverRestart.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"restart"}, args...))
	}
	serverRestart.leaf("cancel", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"restart", "cancel"}, args...))
	}, permissionCommandServerRestartCancel)
	serverRestart.leaf("force", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"restart", "force"}, args...))
	}, permissionCommandServerRestartForce)
	serverShutdown := serverNode.group("shutdown")
	serverShutdown.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"shutdown"}, args...))
	}
	serverShutdown.leaf("cancel", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"shutdown", "cancel"}, args...))
	}, permissionCommandServerShutdownCancel)
	serverShutdown.leaf("force", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"shutdown", "force"}, args...))
	}, permissionCommandServerShutdownForce)
	serverSet := serverNode.group("set")
	serverSet.leaf("loglevel", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"set", "loglevel"}, args...))
	}, permissionCommandServerSetLoglevel)
	serverSet.leaf("motd", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"set", "motd"}, args...))
	}, permissionCommandServerSetMotd)
	serverSet.leaf("closed", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"set", "closed"}, args...))
	}, permissionCommandServerSetClosed)
	serverIdleRestart := serverNode.group("idlerestart")
	serverIdleRestart.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"idlerestart"}, args...))
	}
	serverIdleRestart.leaf("cancel", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"idlerestart", "cancel"}, args...))
	}, permissionCommandServerIdlerestartCancel)
	serverIdleShutdown := serverNode.group("idleshutdown")
	serverIdleShutdown.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"idleshutdown"}, args...))
	}
	serverIdleShutdown.leaf("cancel", func(ctx context.Context, args []string) bool {
		return s.handleCmdServer(ctx, append([]string{"idleshutdown", "cancel"}, args...))
	}, permissionCommandServerIdleshutdownCancel)
	characterNode := root.addWithPerms("character", func(ctx context.Context, args []string) bool { return s.handleCmdCharacter(ctx, args) }, map[string]uint32{"customize": permissionCommandCharacterCustomize, "changefaction": permissionCommandCharacterChangeFaction, "changerace": permissionCommandCharacterChangeRace, "changeaccount": permissionCommandCharacterChangeAccount, "deleted": 0, "erase": permissionCommandCharacterErase, "level": permissionCommandCharacterLevel, "rename": permissionCommandCharacterRename, "reputation": permissionCommandCharacterReputation, "titles": permissionCommandCharacterTitles}, map[string]string{"char": "character"})
	// R13 depth (cs_character.cpp:57-60, old-form table): "deleted"
	// is a sub-table with delete/list/restore/old leaves and no ""
	// entry, so the group node stays handler-less.
	characterDeleted := characterNode.group("deleted")
	characterDeleted.leaf("delete", func(ctx context.Context, args []string) bool {
		return s.handleCmdCharacter(ctx, append([]string{"deleted", "delete"}, args...))
	}, permissionCommandCharacterDeletedDelete)
	characterDeleted.leaf("list", func(ctx context.Context, args []string) bool {
		return s.handleCmdCharacter(ctx, append([]string{"deleted", "list"}, args...))
	}, permissionCommandCharacterDeletedList)
	characterDeleted.leaf("restore", func(ctx context.Context, args []string) bool {
		return s.handleCmdCharacter(ctx, append([]string{"deleted", "restore"}, args...))
	}, permissionCommandCharacterDeletedRestore)
	characterDeleted.leaf("old", func(ctx context.Context, args []string) bool {
		return s.handleCmdCharacter(ctx, append([]string{"deleted", "old"}, args...))
	}, permissionCommandCharacterDeletedOld)
	root.add("levelup", func(ctx context.Context, args []string) bool { return s.handleCmdLevelup(ctx, args) }, nil, nil).withPerm(permissionCommandLevelup)
	root.addWithPerms("pdump", func(ctx context.Context, args []string) bool { return s.handleCmdPDump(ctx, args) }, map[string]uint32{"copy": permissionCommandPDumpCopy, "load": permissionCommandPDumpLoad, "write": permissionCommandPDumpWrite}, nil)
	accountNode := root.addWithPerms("account", func(ctx context.Context, args []string) bool { return s.handleCmdAccount(ctx, args) }, map[string]uint32{"set": 0, "password": permissionCommandAccountPassword, "addon": permissionCommandAccountAddon, "email": permissionCommandAccountEmail, "lock": 0, "2fa": 0, "create": permissionCommandAccountCreate, "delete": permissionCommandAccountDelete, "onlinelist": permissionCommandAccountOnlineList}, map[string]string{"acct": "account"})
	// R13 depth (cs_account.cpp:53-76): "set", "lock" and "2fa" are
	// sub-tables and "sec regmail"/"sec email" multi-word names, so
	// the C++ tree carries account set sec email as a 4-token path
	// with prefix matching at every level. The family switch in
	// handleCmdAccount stays as the fallback for an invisible group.
	accountSet := accountNode.group("set")
	accountSet.leaf("addon", func(ctx context.Context, args []string) bool { return s.handleCmdAccountSetAddon(ctx, args) }, permissionCommandAccountSetAddon)
	accountSet.leaf("gmlevel", func(ctx context.Context, args []string) bool { return s.handleCmdAccountSetSecLevel(ctx, args) }, permissionCommandAccountSetSecLevel)
	accountSet.leaf("seclevel", func(ctx context.Context, args []string) bool { return s.handleCmdAccountSetSecLevel(ctx, args) }, permissionCommandAccountSetSecLevel)
	accountSet.leaf("password", func(ctx context.Context, args []string) bool { return s.handleCmdAccountSetPassword(ctx, args) }, permissionCommandAccountSetPassword)
	accountSet.leaf("2fa", func(ctx context.Context, args []string) bool { return s.handleCmdAccountSet2FA(ctx, args) }, permissionCommandAccountSet2FA)
	accountSetSec := accountSet.group("sec")
	accountSetSec.leaf("email", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccountSetSecEmail(ctx, "sec email", args)
	}, permissionCommandAccountSetSecEmail)
	accountSetSec.leaf("regmail", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccountSetSecEmail(ctx, "sec regmail", args)
	}, permissionCommandAccountSetSecRegmail)
	account2FA := accountNode.group("2fa")
	account2FA.leaf("setup", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccount2FA(ctx, append([]string{"setup"}, args...))
	}, permissionCommandAccount2FASetup)
	account2FA.leaf("remove", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccount2FA(ctx, append([]string{"remove"}, args...))
	}, permissionCommandAccount2FARemove)
	accountLock := accountNode.group("lock")
	accountLock.leaf("ip", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccountLock(ctx, append([]string{"ip"}, args...))
	}, permissionCommandAccountLockIP)
	accountLock.leaf("country", func(ctx context.Context, args []string) bool {
		return s.handleCmdAccountLock(ctx, append([]string{"country"}, args...))
	}, permissionCommandAccountLockCountry)
	root.addWithPerms("achievement", func(ctx context.Context, args []string) bool { return s.handleCmdAchievement(ctx, args) }, map[string]uint32{"add": permissionCommandAchievementAdd}, nil)
	root.addWithPerms("ahbot", func(ctx context.Context, args []string) bool { return s.handleCmdAhbot(ctx, args) }, map[string]uint32{"items": permissionCommandAhbotItems, "ratio": permissionCommandAhbotRatio, "rebuild": permissionCommandAhbotRebuild, "reload": permissionCommandAhbotReload, "status": permissionCommandAhbotStatus}, nil)
	root.addWithPerms("arena", func(ctx context.Context, args []string) bool { return s.handleCmdArena(ctx, args) }, map[string]uint32{"create": permissionCommandArenaCreate, "disband": permissionCommandArenaDisband, "rename": permissionCommandArenaRename, "captain": permissionCommandArenaCaptain, "info": permissionCommandArenaInfo, "lookup": permissionCommandArenaLookup}, nil)
	root.addWithPerms("ban", func(ctx context.Context, args []string) bool { return s.handleCmdBan(ctx, args) }, map[string]uint32{"account": permissionCommandBanAccount, "character": permissionCommandBanCharacter, "playeraccount": permissionCommandBanPlayerAccount, "ip": permissionCommandBanIP}, nil)
	root.addWithPerms("baninfo", func(ctx context.Context, args []string) bool { return s.handleCmdBanInfo(ctx, args) }, map[string]uint32{"account": permissionCommandBanInfoAccount, "character": permissionCommandBanInfoCharacter, "ip": permissionCommandBanInfoIP}, nil)
	root.addWithPerms("banlist", func(ctx context.Context, args []string) bool { return s.handleCmdBanList(ctx, args) }, map[string]uint32{"account": permissionCommandBanListAccount, "character": permissionCommandBanListCharacter, "ip": permissionCommandBanListIP}, nil)
	root.addWithPerms("unban", func(ctx context.Context, args []string) bool { return s.handleCmdUnBan(ctx, args) }, map[string]uint32{"account": permissionCommandUnBanAccount, "character": permissionCommandUnBanCharacter, "playeraccount": permissionCommandUnBanPlayerAccount, "ip": permissionCommandUnBanIP}, nil)
	root.addWithPerms("bf", func(ctx context.Context, args []string) bool { return s.handleCmdBF(ctx, args) }, map[string]uint32{"start": permissionCommandBfStart, "stop": permissionCommandBfStop, "switch": permissionCommandBfSwitch, "timer": permissionCommandBfTimer, "enable": permissionCommandBfEnable}, nil)
	deserterNode := root.add("deserter", func(ctx context.Context, args []string) bool { return s.handleCmdDeserter(ctx, args) }, []string{"instance", "bg"}, nil)
	// R13 depth (cs_deserter.cpp:44-53): "instance" and "bg" are
	// sub-tables with add/remove leaves and no "" entries, so the
	// group nodes stay handler-less.
	deserterInstance := deserterNode.group("instance")
	deserterInstance.leaf("add", func(ctx context.Context, args []string) bool {
		return s.handleCmdDeserter(ctx, append([]string{"instance", "add"}, args...))
	}, permissionCommandDeserterInstanceAdd)
	deserterInstance.leaf("remove", func(ctx context.Context, args []string) bool {
		return s.handleCmdDeserter(ctx, append([]string{"instance", "remove"}, args...))
	}, permissionCommandDeserterInstanceRemove)
	deserterBG := deserterNode.group("bg")
	deserterBG.leaf("add", func(ctx context.Context, args []string) bool {
		return s.handleCmdDeserter(ctx, append([]string{"bg", "add"}, args...))
	}, permissionCommandDeserterBGAdd)
	deserterBG.leaf("remove", func(ctx context.Context, args []string) bool {
		return s.handleCmdDeserter(ctx, append([]string{"bg", "remove"}, args...))
	}, permissionCommandDeserterBGRemove)
	disableNode := root.add("disable", func(ctx context.Context, args []string) bool { return s.handleCmdDisable(ctx, args) }, []string{"add", "remove"}, nil)
	// R13 depth (cs_disable.cpp:44-75, old-form tables): "add" and
	// "remove" are sub-tables with spell/quest/map/battleground/
	// achievement_criteria/outdoorpvp/vmap/mmap leaves and no ""
	// entries, so the group nodes stay handler-less. The family
	// switch keeps its own per-type perm gate and its map/mmap
	// ambiguity listing as the fallback for an invisible group.
	disableAdd := disableNode.group("add")
	for _, typ := range []struct {
		name string
		perm uint32
	}{
		{"spell", permissionCommandDisableAddSpell},
		{"quest", permissionCommandDisableAddQuest},
		{"map", permissionCommandDisableAddMap},
		{"battleground", permissionCommandDisableAddBattleground},
		{"achievement_criteria", permissionCommandDisableAddAchievementCriteria},
		{"outdoorpvp", permissionCommandDisableAddOutdoorPvP},
		{"vmap", permissionCommandDisableAddVMap},
		{"mmap", permissionCommandDisableAddMMap},
	} {
		typ := typ
		disableAdd.leaf(typ.name, func(ctx context.Context, args []string) bool {
			return s.handleCmdDisable(ctx, append([]string{"add", typ.name}, args...))
		}, typ.perm)
	}
	disableRemove := disableNode.group("remove")
	for _, typ := range []struct {
		name string
		perm uint32
	}{
		{"spell", permissionCommandDisableRemoveSpell},
		{"quest", permissionCommandDisableRemoveQuest},
		{"map", permissionCommandDisableRemoveMap},
		{"battleground", permissionCommandDisableRemoveBattleground},
		{"achievement_criteria", permissionCommandDisableRemoveAchievementCriteria},
		{"outdoorpvp", permissionCommandDisableRemoveOutdoorPvP},
		{"vmap", permissionCommandDisableRemoveVMap},
		{"mmap", permissionCommandDisableRemoveMMap},
	} {
		typ := typ
		disableRemove.leaf(typ.name, func(ctx context.Context, args []string) bool {
			return s.handleCmdDisable(ctx, append([]string{"remove", typ.name}, args...))
		}, typ.perm)
	}
	root.addWithPerms("event", func(ctx context.Context, args []string) bool { return s.handleCmdEvent(ctx, args) }, map[string]uint32{"activelist": permissionCommandEventActivelist, "start": permissionCommandEventStart, "stop": permissionCommandEventStop, "info": permissionCommandEventInfo}, nil)
	gobjectNode := root.addWithPerms("gobject", func(ctx context.Context, args []string) bool { return s.handleCmdGObject(ctx, args) }, map[string]uint32{"activate": permissionCommandGObjectActivate, "delete": permissionCommandGObjectDelete, "info": permissionCommandGObjectInfo, "move": permissionCommandGObjectMove, "near": permissionCommandGObjectNear, "target": permissionCommandGObjectTarget, "turn": permissionCommandGObjectTurn, "spawngroup": permissionCommandGObjectSpawnGroup, "despawngroup": permissionCommandGObjectDespawnGroup, "add": 0, "set": 0}, map[string]string{"gob": "gobject"})
	// R13 depth (cs_gobject.cpp:61-71, old-form table): "add temp"
	// and "set phase"/"set state" are multi-word entries, so add/set
	// are 3-token paths; "add" also carries the bare handler, so the
	// group node keeps its handler via the family switch, while "set"
	// has no "" entry and stays handler-less.
	gobjectAdd := gobjectNode.group("add")
	gobjectAdd.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdGObject(ctx, append([]string{"add"}, args...))
	}
	gobjectAdd.leaf("temp", func(ctx context.Context, args []string) bool {
		return s.handleCmdGObject(ctx, append([]string{"add", "temp"}, args...))
	}, permissionCommandGObjectAddTemp)
	gobjectSet := gobjectNode.group("set")
	gobjectSet.leaf("phase", func(ctx context.Context, args []string) bool {
		return s.handleCmdGObject(ctx, append([]string{"set", "phase"}, args...))
	}, permissionCommandGObjectSetPhase)
	gobjectSet.leaf("state", func(ctx context.Context, args []string) bool {
		return s.handleCmdGObject(ctx, append([]string{"set", "state"}, args...))
	}, permissionCommandGObjectSetState)
	root.addWithPerms("group", func(ctx context.Context, args []string) bool { return s.handleCmdGroup(ctx, args) }, map[string]uint32{"set": permissionCommandGroupSet, "leader": permissionCommandGroupLeader, "disband": permissionCommandGroupDisband, "remove": permissionCommandGroupRemove, "join": permissionCommandGroupJoin, "list": permissionCommandGroupList, "summon": permissionCommandGroupSummon}, nil).withPerm(permissionCommandGroup)
	root.addWithPerms("guild", func(ctx context.Context, args []string) bool { return s.handleCmdGuild(ctx, args) }, map[string]uint32{"create": permissionCommandGuildCreate, "delete": permissionCommandGuildDelete, "invite": permissionCommandGuildInvite, "uninvite": permissionCommandGuildUninvite, "rank": permissionCommandGuildRank, "rename": permissionCommandGuildRename, "info": permissionCommandGuildInfo}, nil).withPerm(permissionCommandGuild)
	honorNode := root.addWithPerms("honor", func(ctx context.Context, args []string) bool { return s.handleCmdHonor(ctx, args) }, map[string]uint32{"add": permissionCommandHonorAdd, "update": permissionCommandHonorUpdate}, nil).withPerm(permissionCommandHonor)
	// R13 depth (cs_honor.cpp:45-51): "add" is a sub-table with a
	// kill leaf and a "" entry (the amount handler), so the group
	// node keeps its handler via the family switch.
	honorAdd := honorNode.group("add")
	honorAdd.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdHonor(ctx, append([]string{"add"}, args...))
	}
	honorAdd.leaf("kill", func(ctx context.Context, args []string) bool {
		return s.handleCmdHonor(ctx, append([]string{"add", "kill"}, args...))
	}, permissionCommandHonorAddKill)
	root.addWithPerms("instance", func(ctx context.Context, args []string) bool { return s.handleCmdInstance(ctx, args) }, map[string]uint32{"listbinds": permissionCommandInstanceListBinds, "unbind": permissionCommandInstanceUnbind, "stats": permissionCommandInstanceStats, "savedata": permissionCommandInstanceSaveData, "setbossstate": permissionCommandInstanceSetBossState, "getbossstate": permissionCommandInstanceGetBossState}, nil).withPerm(permissionCommandInstance)
	learnNode := root.add("learn", func(ctx context.Context, args []string) bool { return s.handleCmdLearn(ctx, args) }, []string{"all", "my"}, nil)
	// R13 depth (cs_learn.cpp:47-60, old-form tables): "all" and "my"
	// are sub-tables with no "" entries (the bare learn handler
	// lives on the learn node itself), so both group nodes stay
	// handler-less. The family switch stays as the fallback for an
	// invisible group.
	learnAll := learnNode.group("all")
	for _, arm := range []struct {
		name string
		perm uint32
	}{
		{"blizzard", permissionCommandLearnAllGM},
		{"debug", permissionCommandLearn},
		{"crafts", permissionCommandLearnAllCrafts},
		{"default", permissionCommandLearnAllDefault},
		{"languages", permissionCommandLearnAllLang},
		{"recipes", permissionCommandLearnAllRecipes},
		{"talents", permissionCommandLearnAllTalents},
		{"pettalents", permissionCommandLearnMyPetTalents},
	} {
		arm := arm
		learnAll.leaf(arm.name, func(ctx context.Context, args []string) bool {
			return s.handleCmdLearn(ctx, append([]string{"all", arm.name}, args...))
		}, arm.perm)
	}
	learnMy := learnNode.group("my")
	for _, arm := range []string{"trainer", "quests"} {
		arm := arm
		learnMy.leaf(arm, func(ctx context.Context, args []string) bool {
			return s.handleCmdLearn(ctx, append([]string{"my", arm}, args...))
		}, permissionCommandLearnAllMySpells)
	}
	lookupNode := root.addWithPerms("lookup", func(ctx context.Context, args []string) bool { return s.handleCmdLookup(ctx, args) }, map[string]uint32{"area": permissionCommandLookupArea, "creature": permissionCommandLookupCreature, "event": permissionCommandLookupEvent, "faction": permissionCommandLookupFaction, "item": permissionCommandLookupItem, "object": permissionCommandLookupObject, "quest": permissionCommandLookupQuest, "player": 0, "skill": permissionCommandLookupSkill, "spell": permissionCommandLookupSpell, "taxinode": permissionCommandLookupTaxinode, "tele": permissionCommandLookupTele, "title": permissionCommandLookupTitle, "map": permissionCommandLookupMap}, nil)
	// R13 depth (cs_lookup.cpp:66-80): "item id", "item set",
	// "quest id", "spell id" and "map id" are multi-word entries, so
	// item/quest/spell/map are 3-token paths; each also carries the
	// bare name handler, so those group nodes keep their handler via
	// the family switch. "player" is a real sub-table with
	// ip/account/email leaves and no "" entry.
	lookupItem := lookupNode.group("item")
	lookupItem.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"item"}, args...))
	}
	lookupItem.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"item", "id"}, args...))
	}, permissionCommandLookupItemID)
	lookupItem.leaf("set", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"item", "set"}, args...))
	}, permissionCommandLookupItemSet)
	lookupQuest := lookupNode.group("quest")
	lookupQuest.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"quest"}, args...))
	}
	lookupQuest.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"quest", "id"}, args...))
	}, permissionCommandLookupQuestID)
	lookupSpell := lookupNode.group("spell")
	lookupSpell.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"spell"}, args...))
	}
	lookupSpell.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"spell", "id"}, args...))
	}, permissionCommandLookupSpellID)
	lookupMap := lookupNode.group("map")
	lookupMap.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"map"}, args...))
	}
	lookupMap.leaf("id", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"map", "id"}, args...))
	}, permissionCommandLookupMapID)
	lookupPlayer := lookupNode.group("player")
	lookupPlayer.leaf("ip", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"player", "ip"}, args...))
	}, permissionCommandLookupPlayerIP)
	lookupPlayer.leaf("account", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"player", "account"}, args...))
	}, permissionCommandLookupPlayerAccount)
	lookupPlayer.leaf("email", func(ctx context.Context, args []string) bool {
		return s.handleCmdLookup(ctx, append([]string{"player", "email"}, args...))
	}, permissionCommandLookupPlayerEmail)
	root.addWithPerms("lfg", func(ctx context.Context, args []string) bool { return s.handleCmdLFG(ctx, args) }, map[string]uint32{"player": permissionCommandLfgPlayer, "group": permissionCommandLfgGroup, "queue": permissionCommandLfgQueue, "clean": permissionCommandLfgClean, "options": permissionCommandLfgOptions}, nil)
	root.addWithPerms("list", func(ctx context.Context, args []string) bool { return s.handleCmdList(ctx, args) }, map[string]uint32{"creature": permissionCommandListCreature, "item": permissionCommandListItem, "object": permissionCommandListObject, "auras": permissionCommandListAuras, "mail": permissionCommandListMail, "spawnpoints": permissionCommandListSpawnpoints, "respawns": permissionCommandListRespawns}, nil)
	root.add("channel", func(ctx context.Context, args []string) bool { return s.handleCmdChannel(ctx, args) }, []string{"set"}, nil).withPerm(permissionCommandChannelSetOwnership)
	root.add("nameannounce", func(ctx context.Context, args []string) bool { return s.handleCmdNameAnnounceCommand(ctx, args) }, nil, nil).withPerm(permissionCommandNameAnnounce)
	root.add("gmnameannounce", func(ctx context.Context, args []string) bool {
		return s.handleCmdGMNameAnnounceCommand(ctx, args)
	}, nil, nil).withPerm(permissionCommandGMNameAnnounce)
	root.add("announce", func(ctx context.Context, args []string) bool { return s.handleCmdAnnounceCommand(ctx, args) }, nil, nil).withPerm(permissionCommandAnnounce)
	root.add("gmannounce", func(ctx context.Context, args []string) bool { return s.handleCmdGMAnnounceCommand(ctx, args) }, nil, nil).withPerm(permissionCommandGMAnnounce)
	root.add("notify", func(ctx context.Context, args []string) bool { return s.handleCmdNotifyCommand(ctx, args) }, nil, nil).withPerm(permissionCommandNotify)
	root.add("gmnotify", func(ctx context.Context, args []string) bool { return s.handleCmdGMNotifyCommand(ctx, args) }, nil, nil).withPerm(permissionCommandGMNotify)
	root.add("whispers", func(ctx context.Context, args []string) bool { return s.handleCmdWhispers(ctx, args) }, nil, nil).withPerm(permissionCommandWhispers)
	worldChatInvoke := func(ctx context.Context, args []string) bool { return s.handleCmdWorldChat(ctx, args) }
	root.add("chat", worldChatInvoke, nil, nil).withPerm(permissionCommandSave)
	root.add("c", worldChatInvoke, nil, nil).withPerm(permissionCommandSave)
	root.add("world", worldChatInvoke, nil, nil).withPerm(permissionCommandSave)
	root.add("unlearn", func(ctx context.Context, args []string) bool { return s.handleCmdUnLearn(ctx, args) }, nil, nil).withPerm(permissionCommandUnlearn)
	root.add("appear", func(ctx context.Context, args []string) bool { return s.handleCmdAppear(ctx, args) }, nil, nil).withPerm(permissionCommandAppear)
	root.add("aura", func(ctx context.Context, args []string) bool { return s.handleCmdAura(ctx, args) }, nil, nil).withPerm(permissionCommandAura)
	root.add("unaura", func(ctx context.Context, args []string) bool { return s.handleCmdUnAura(ctx, args) }, nil, nil).withPerm(permissionCommandUnAura)
	root.add("bank", func(ctx context.Context, args []string) bool { return s.handleCmdBank(ctx, args) }, nil, nil).withPerm(permissionCommandBank)
	root.add("bindsight", func(ctx context.Context, args []string) bool { return s.handleCmdBindSight(ctx, args) }, nil, nil).withPerm(permissionCommandBindSight)
	root.add("unbindsight", func(ctx context.Context, args []string) bool { return s.handleCmdUnBindSight(ctx, args) }, nil, nil).withPerm(permissionCommandUnBindSight)
	root.add("combatstop", func(ctx context.Context, args []string) bool { return s.handleCmdCombatStop(ctx, args) }, nil, nil).withPerm(permissionCommandCombatStop)
	root.add("cometome", func(ctx context.Context, args []string) bool { return s.handleCmdComeToMe(ctx, args) }, nil, nil).withPerm(permissionCommandComeToMe)
	root.add("commands", func(ctx context.Context, args []string) bool { return s.handleCmdCommands(ctx, args) }, nil, nil).withPerm(permissionCommandCommands)
	root.add("cooldown", func(ctx context.Context, args []string) bool { return s.handleCmdCooldown(ctx, args) }, nil, nil).withPerm(permissionCommandCooldown)
	root.add("damage", func(ctx context.Context, args []string) bool { return s.handleCmdDamage(ctx, args) }, nil, nil).withPerm(permissionCommandDamage)
	root.add("dev", func(ctx context.Context, args []string) bool { return s.handleCmdDev(ctx, args) }, nil, nil).withPerm(permissionCommandDev)
	root.add("die", func(ctx context.Context, args []string) bool { return s.handleCmdDie(ctx, args) }, nil, nil).withPerm(permissionCommandDie)
	root.add("distance", func(ctx context.Context, args []string) bool { return s.handleCmdDistance(ctx, args) }, nil, nil).withPerm(permissionCommandDistance)
	root.add("flusharenapoints", func(ctx context.Context, args []string) bool { return s.handleCmdFlushArenaPoints(ctx, args) }, nil, nil).withPerm(permissionCommandFlushArenaPoints)
	root.add("freeze", func(ctx context.Context, args []string) bool { return s.handleCmdFreeze(ctx, args) }, nil, nil).withPerm(permissionCommandFreeze)
	root.add("revive", func(ctx context.Context, args []string) bool { return s.handleCmdRevive2(ctx, args) }, nil, nil).withPerm(permissionCommandRevive)
	root.add("dismount", func(ctx context.Context, args []string) bool { return s.handleCmdDismount(ctx, args) }, nil, nil).withPerm(permissionCommandDismount)
	root.add("saveall", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdSaveAll(ctx)
	}, nil, nil).withPerm(permissionCommandSaveAll)
	root.add("save", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdSave2(ctx)
	}, nil, nil).withPerm(permissionCommandSave)
	root.add("gps", func(ctx context.Context, args []string) bool { return s.handleCmdGPS(ctx, args) }, nil, nil).withPerm(permissionCommandGPS)
	root.add("guid", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdGUID(ctx)
	}, nil, nil).withPerm(permissionCommandGUID)
	root.add("help", func(ctx context.Context, args []string) bool { return s.handleCmdHelp2(ctx, args) }, nil, nil).withPerm(permissionCommandHelp)
	root.add("hidearea", func(ctx context.Context, args []string) bool { return s.handleCmdHideArea(ctx, args) }, nil, nil).withPerm(permissionCommandHideArea)
	root.add("itemmove", func(ctx context.Context, args []string) bool { return s.handleCmdItemMove(ctx, args) }, nil, nil).withPerm(permissionCommandItemMove)
	root.add("kick", func(ctx context.Context, args []string) bool { return s.handleCmdKickPlayer(ctx, args) }, nil, nil).withPerm(permissionCommandKick)
	root.add("linkgrave", func(ctx context.Context, args []string) bool { return s.handleCmdLinkGrave(ctx, args) }, nil, nil).withPerm(permissionCommandLinkGrave)
	root.add("listfreeze", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdListFreeze(ctx)
	}, nil, nil).withPerm(permissionCommandListFreeze)
	root.add("maxskill", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdMaxSkill(ctx)
	}, nil, nil).withPerm(permissionCommandMaxSkill)
	root.add("movegens", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdMovegens(ctx)
	}, nil, nil).withPerm(permissionCommandMovegens)
	root.add("mute", func(ctx context.Context, args []string) bool { return s.handleCmdMute(ctx, args) }, nil, nil).withPerm(permissionCommandMute)
	root.add("mutehistory", func(ctx context.Context, args []string) bool { return s.handleCmdMuteHistory(ctx, args) }, nil, nil).withPerm(permissionCommandMuteHistory)
	root.add("neargrave", func(ctx context.Context, args []string) bool { return s.handleCmdNearGrave(ctx, args) }, nil, nil).withPerm(permissionCommandNearGrave)
	root.add("pinfo", func(ctx context.Context, args []string) bool { return s.handleCmdPInfo(ctx, args) }, nil, nil).withPerm(permissionCommandPInfo)
	root.add("playall", func(ctx context.Context, args []string) bool { return s.handleCmdPlayAll(ctx, args) }, nil, nil).withPerm(permissionCommandPlayAll)
	root.add("possess", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdPossess(ctx)
	}, nil, nil).withPerm(permissionCommandPossess)
	root.add("pvpstats", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdPvPstats(ctx)
	}, nil, nil).withPerm(permissionCommandPvPstats)
	root.add("recall", func(ctx context.Context, args []string) bool { return s.handleCmdRecall(ctx, args) }, nil, nil).withPerm(permissionCommandRecall)
	root.add("repairitems", func(ctx context.Context, args []string) bool { return s.handleCmdRepairItems(ctx, args) }, nil, nil).withPerm(permissionCommandRepairItems)
	root.add("respawn", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdRespawn(ctx)
	}, nil, nil).withPerm(permissionCommandRespawn)
	root.add("setskill", func(ctx context.Context, args []string) bool { return s.handleCmdSetSkill(ctx, args) }, nil, nil).withPerm(permissionCommandSetSkill)
	root.add("showarea", func(ctx context.Context, args []string) bool { return s.handleCmdShowArea(ctx, args) }, nil, nil).withPerm(permissionCommandShowArea)
	root.add("summon", func(ctx context.Context, args []string) bool { return s.handleCmdSummon(ctx, args) }, nil, nil).withPerm(permissionCommandSummon)
	root.add("unfreeze", func(ctx context.Context, args []string) bool { return s.handleCmdUnFreeze(ctx, args) }, nil, nil).withPerm(permissionCommandUnFreeze)
	root.add("unmute", func(ctx context.Context, args []string) bool { return s.handleCmdUnmute(ctx, args) }, nil, nil).withPerm(permissionCommandUnMute)
	root.add("unpossess", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdUnPossess(ctx)
	}, nil, nil).withPerm(permissionCommandUnPossess)
	root.add("unstuck", func(ctx context.Context, args []string) bool { return s.handleCmdUnstuck(ctx, args) }, nil, nil).withPerm(permissionCommandUnstuck)
	root.add("wchange", func(ctx context.Context, args []string) bool { return s.handleCmdChangeWeather(ctx, args) }, nil, nil).withPerm(permissionCommandChangeWeather)
	root.add("mailbox", func(ctx context.Context, args []string) bool {
		// ChatCommand.h:112 - the C++ handler takes no args; a trailing
		// token fails the whole-string-consumed check.
		if len(args) > 0 {
			return false
		}
		return s.handleCmdMailBox(ctx)
	}, nil, nil).withPerm(permissionCommandMailBox)
	root.addWithPerms("mmap", func(ctx context.Context, args []string) bool { return s.handleCmdMMap(ctx, args) }, map[string]uint32{"loadedtiles": permissionCommandMMapLoadedTiles, "loc": permissionCommandMMapLoc, "path": permissionCommandMMapPath, "stats": permissionCommandMMapStats, "testarea": permissionCommandMMapTestArea}, nil).withPerm(permissionCommandMMap)
	root.add("morph", func(ctx context.Context, args []string) bool { return s.handleCmdMorph(ctx, args) }, nil, nil).withPerm(permissionCommandMorph)
	root.add("demorph", func(ctx context.Context, args []string) bool { return s.handleCmdDeMorph(ctx) }, nil, nil).withPerm(permissionCommandDeMorph)
	npcNode := root.addWithPerms("npc", func(ctx context.Context, args []string) bool { return s.handleCmdNPC(ctx, args) }, map[string]uint32{"add": 0, "set": 0, "move": permissionCommandNPCMove, "delete": permissionCommandNPCDelete, "near": permissionCommandNPCNear, "info": permissionCommandNPCInfo, "playemote": permissionCommandNPCPlayEmote, "say": permissionCommandNPCSay, "textemote": permissionCommandNPCTextEmote, "whisper": permissionCommandNPCWhisper, "yell": permissionCommandNPCYell, "tame": permissionCommandNPCTame, "spawngroup": permissionCommandNPCSpawnGroup, "despawngroup": permissionCommandNPCDespawnGroup, "follow": permissionCommandNPCFollow, "evade": permissionCommandNPCEvade, "showloot": permissionCommandNPCShowLoot}, nil)
	// R13 depth (cs_npc.cpp:61-86): "add" and "set" are sub-tables.
	// The "add" group also carries the "" entry (HandleNpcAddCommand,
	// ".npc add <entry>"), so the group node keeps its handler;
	// "set" has no "" entry and stays handler-less. The nil-player
	// guard of handleCmdNPC is preserved: the leaf path bypasses
	// the family handler. handleNPCAddTable/handleNPCSetTable stay
	// as the fallback for an invisible group.
	npcAdd := npcNode.group("add")
	npcAdd.invoke = func(ctx context.Context, args []string) bool {
		if s.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		return s.handleNPCAddTable(ctx, args)
	}
	npcAdd.leaf("formation", func(ctx context.Context, args []string) bool {
		if s.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		return s.handleNPCAddFormation(ctx, args)
	}, permissionCommandNPCAddFormation)
	npcAdd.leaf("item", func(ctx context.Context, args []string) bool {
		if s.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		return s.handleNPCAddItem(ctx, args)
	}, permissionCommandNPCAddItem)
	npcAdd.leaf("move", func(ctx context.Context, args []string) bool {
		if s.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		return s.handleNPCAddMove(ctx, args)
	}, permissionCommandNPCAddMove)
	npcAdd.leaf("temp", func(ctx context.Context, args []string) bool {
		if s.player == nil {
			s.sendSysMessage("You must be in game to use that command.")
			return true
		}
		return s.handleNPCAddTemp(ctx, args)
	}, permissionCommandNPCAddTemp)
	npcSet := npcNode.group("set")
	npcSetLeaf := func(name string, invoke func(ctx context.Context, args []string) bool, perm uint32) {
		npcSet.leaf(name, func(ctx context.Context, args []string) bool {
			if s.player == nil {
				s.sendSysMessage("You must be in game to use that command.")
				return true
			}
			return invoke(ctx, args)
		}, perm)
	}
	npcSetLeaf("allowmove", func(ctx context.Context, args []string) bool { return s.handleNPCSetAllowMove(ctx) }, permissionCommandNPCSetAllowMove)
	npcSetLeaf("entry", s.handleNPCSetEntry, permissionCommandNPCSetEntry)
	npcSetLeaf("factionid", func(ctx context.Context, args []string) bool { return s.handleNPCSetFactionID(ctx, args) }, permissionCommandNPCSetFactionID)
	npcSetLeaf("flag", func(ctx context.Context, args []string) bool { return s.handleNPCSetFlag(ctx, args) }, permissionCommandNPCSetFlag)
	npcSetLeaf("level", func(ctx context.Context, args []string) bool { return s.handleNPCSetLevel(ctx, args) }, permissionCommandNPCSetLevel)
	npcSetLeaf("link", func(ctx context.Context, args []string) bool { return s.handleNPCSetLink(ctx, args) }, permissionCommandNPCSetLink)
	npcSetLeaf("model", func(ctx context.Context, args []string) bool { return s.handleNPCSetModel(ctx, args) }, permissionCommandNPCSetModel)
	npcSetLeaf("movetype", s.handleNPCSetMoveType, permissionCommandNPCSetMoveType)
	npcSetLeaf("phase", func(ctx context.Context, args []string) bool { return s.handleNPCSetPhase(ctx, args) }, permissionCommandNPCSetPhase)
	npcSetLeaf("wanderdistance", s.handleNPCSetWanderDistance, permissionCommandNPCSetSpawnDist)
	npcSetLeaf("spawntime", s.handleNPCSetSpawnTime, permissionCommandNPCSetSpawnTime)
	npcSetLeaf("data", func(ctx context.Context, args []string) bool { return s.handleNPCSetData(ctx, args) }, permissionCommandNPCSetData)
	root.add("npcbot", func(ctx context.Context, args []string) bool { return s.handleCmdNpcBot(ctx, args) }, []string{"add", "remove", "spawn", "move", "delete", "lookup", "revive", "reloadconfig", "command", "info", "hide", "unhide", "show", "recall", "kill", "suicide", "distance", "order", "set"}, nil)
	root.addWithPerms("quest", func(ctx context.Context, args []string) bool { return s.handleCmdQuest(ctx, args) }, map[string]uint32{"add": permissionCommandQuestAdd, "complete": permissionCommandQuestComplete, "remove": permissionCommandQuestRemove, "reward": permissionCommandQuestReward}, nil).withPerm(permissionCommandQuest)
	root.addWithPerms("pet", func(ctx context.Context, args []string) bool { return s.handleCmdPet(ctx, args) }, map[string]uint32{"create": permissionCommandPetCreate, "learn": permissionCommandPetLearn, "unlearn": permissionCommandPetUnlearn, "level": permissionCommandPetLevel}, nil).withPerm(permissionCommandPet)
	root.addWithPerms("rbac", func(ctx context.Context, args []string) bool { return s.handleCmdRBAC(ctx, args) }, map[string]uint32{"account": permissionCommandRBACAcc, "list": permissionCommandRBACList}, nil).withPerm(permissionCommandRBAC)
	reloadNode := root.addWithPerms("reload", func(ctx context.Context, args []string) bool { return s.handleCmdReload(ctx, args) }, map[string]uint32{"all": permissionCommandReloadAll}, nil).withPerm(permissionCommandReload)
	// R13 depth (cs_reload.cpp:63-73): "all" is a sub-table with
	// achievement/area/gossips/item/locales/loot/npc/quest/scripts/
	// spell leaves plus a "" entry (reload everything), so the group
	// node keeps its handler via the family switch.
	reloadAll := reloadNode.group("all")
	reloadAll.invoke = func(ctx context.Context, args []string) bool {
		return s.handleCmdReload(ctx, append([]string{"all"}, args...))
	}
	for _, group := range []struct {
		name string
		perm uint32
	}{
		{"achievement", permissionCommandReloadAllAchievement},
		{"area", permissionCommandReloadAllArea},
		{"gossips", permissionCommandReloadAllGossip},
		{"item", permissionCommandReloadAllItem},
		{"locales", permissionCommandReloadAllLocales},
		{"loot", permissionCommandReloadAllLoot},
		{"npc", permissionCommandReloadAllNpc},
		{"quest", permissionCommandReloadAllQuest},
		{"scripts", permissionCommandReloadAllScripts},
		{"spell", permissionCommandReloadAllSpell},
	} {
		group := group
		reloadAll.leaf(group.name, func(ctx context.Context, args []string) bool {
			return s.handleCmdReload(ctx, append([]string{"all", group.name}, args...))
		}, group.perm)
	}
	root.addWithPerms("reset", func(ctx context.Context, args []string) bool { return s.handleCmdReset(ctx, args) }, map[string]uint32{"achievements": permissionCommandResetAchievements, "honor": permissionCommandResetHonor, "level": permissionCommandResetLevel, "spells": permissionCommandResetSpells, "stats": permissionCommandResetStats, "talents": permissionCommandResetTalents, "all": permissionCommandResetAll}, nil).withPerm(permissionCommandReset)
	root.addWithPerms("send", func(ctx context.Context, args []string) bool { return s.handleCmdSend(ctx, args) }, map[string]uint32{"items": permissionCommandSendItems, "mail": permissionCommandSendMail, "message": permissionCommandSendMessage, "money": permissionCommandSendMoney}, nil).withPerm(permissionCommandSend)
	return root
}

// commandNodeVisible mirrors ChatCommandNode::IsVisible (ChatCommand.h:200):
// invoker-visible, or any visible subcommand. A node carrying a permission
// is invoker-visible when the session holds it; a node with no permission
// (perm 0: old-form group node, or a family still awaiting the census)
// counts as invoker-visible. Group nodes in new-form tables carry their
// own permission (titles, modify speed) but stay visible through a
// visible subcommand, exactly as C++ does.
func (s *session) commandNodeVisible(ctx context.Context, n *commandNode, allowed func(uint32) bool) bool {
	if n == nil {
		return false
	}
	if n.perm == 0 || allowed(n.perm) {
		return true
	}
	for _, child := range n.children {
		if s.commandNodeVisible(ctx, child, allowed) {
			return true
		}
	}
	return false
}

// commandTreeVisible loads the invoker's permission set once per command
// (FilteredCommandListIterator consults the same set for every candidate;
// ChatCommand.cpp:207-237). With no auth DB to consult it falls back to the
// commandAllowed rule (GM accounts keep their commands).
func (s *session) commandTreeVisible(ctx context.Context) func(*commandNode) bool {
	allowed := func(uint32) bool { return s.security >= 1 }
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		if granted, err := loadGrantedPermissions(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security); err == nil {
			allowed = func(perm uint32) bool {
				_, ok := granted[perm]
				return ok
			}
		}
	}
	return func(n *commandNode) bool { return s.commandNodeVisible(ctx, n, allowed) }
}

// dispatchCommand resolves a partial command like '.mod spee 10' to its
// canonical invocation ('.modify speed 10') like ChatCommandNode::
// TryExecuteCommand, reporting ambiguous matches with the TC wording.
func (s *session) dispatchCommand(ctx context.Context, fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	root := s.buildCommandTree()
	visible := s.commandTreeVisible(ctx)
	node, rewritten, _, ambiguous, nodeDepth := root.resolve(fields, visible)
	if ambiguous != nil {
		s.sendSysMessage("There are multiple commands matching '" + strings.ToLower(fields[len(rewritten)]) + "'. Did you mean:")
		for _, candidate := range ambiguous {
			// ChatCommand.cpp:277-280 prints each candidate with the
			// "..." suffix when it has visible subcommands.
			if child := node.children[candidate]; child != nil {
				for _, sub := range child.children {
					if visible(sub) {
						candidate += " ..."
						break
					}
				}
			}
			s.sendSysMessage(candidate)
		}
		return true
	}
	if node == root {
		return false
	}
	args := make([]string, 0, len(fields)-nodeDepth)
	args = append(args, rewritten[nodeDepth:]...)
	args = append(args, fields[len(rewritten):]...)
	if node.invoke == nil {
		s.sendSysMessage("Usage: ." + strings.Join(rewritten, " "))
		return true
	}
	// ChatCommand.cpp:296-309: a handler that returns false without having
	// sent an error message makes the framework show the node's help
	// listing. Handlers that print their own message return true, so only
	// the silent-false paths (no selection, lower-security refusal,
	// over-limit expansion, ...) reach this arm.
	if !node.invoke(ctx, args) {
		s.sendCommandHelp(ctx, node)
	}
	return true
}

// handleSetFactionCheat processes CMSG_SET_FACTION_CHEAT (0x126).
// Reference: WorldSession::HandleSetFactionCheat (CharacterHandler.cpp:1043):
// the handler answers with ReputationMgr::SendState(nullptr)
// (ReputationMgr.cpp:184), i.e. SMSG_SET_FACTION_STANDING — not
// SMSG_INITIALIZE_FACTIONS. With no faction argument the C++ packet carries
// only factions still marked needSend; Go flushes every standing change
// eagerly (no needSend model), so the pending set is empty and the packet is
// the empty standing update. The _sendFactionIncreased byte has no Go
// counterpart (rank-increase state is sent inline by the standing updates).
func (s *session) handleSetFactionCheat(ctx context.Context, payload []byte) bool {
	s.debug("handleSetFactionCheat received", "account", s.accountName)
	if s.playerLoaded && s.player != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_SET_FACTION_STANDING), buildFactionStandingState(nil, false), true)
	}
	return true
}

// handleWorldTeleport processes CMSG_WORLD_TELEPORT (0x008).
// Reference: WorldSession::HandleWorldTeleportOpcode (MiscHandler.cpp:1070).
func (s *session) handleWorldTeleport(ctx context.Context, payload []byte) bool {
	// WorldPackets::Misc::WorldTeleport::Read (MiscPackets.cpp:197): Time u32
	// + MapID u32 + Pos XYZ + Facing = 24 bytes; a short packet fails
	// deserialization server-side, so the gate is the full 24.
	if !s.playerLoaded || s.player == nil || len(payload) < 24 {
		return true
	}

	// WorldSession::HandleWorldTeleportOpcode (MiscHandler.cpp:1072): the
	// in-flight check runs before the permission gate — a player in flight
	// has the packet silently ignored.
	if s.isInFlight() {
		return true
	}

	allowed := s.commandAllowed(ctx, permissionOpcodeWorldTeleport)
	if !allowed {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}

	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // time
	mapID, _ := r.ReadU32()
	x, _ := r.ReadF32()
	y, _ := r.ReadF32()
	z, _ := r.ReadF32()
	o, _ := r.ReadF32()
	s.teleportTo(mapID, x, y, z, o)
	return true
}

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
	pvpFlags := s.player.PVPFlags
	if s.server.Config.GameType == 4 || s.server.Config.GameType == 6 {
		pvpFlags |= 0x01
	}
	channelGUID, channelSpell := uint64(0), uint32(0)
	s.castMu.Lock()
	if s.activeChannel != nil {
		channelGUID, channelSpell = s.activeChannel.TargetGUID, s.activeChannel.SpellID
	}
	s.castMu.Unlock()
	fields := map[int]uint32{
		unitFieldHealth:                   s.player.Health,
		unitFieldMaxHealth:                s.player.MaxHealth,
		unitFieldLevel:                    uint32(s.player.Level),
		unitFieldFaction:                  s.server.raceFaction(s.player.Race),
		unitFieldFlags:                    unitFlagPlayerControlled | s.player.UnitFlags,
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
		unitFieldBytes2:                   uint32(s.player.SheathState) | uint32(pvpFlags)<<8,
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
		playerFieldDuelArbiter:            uint32(s.player.DuelArbiter),
		playerFieldDuelArbiter + 1:        uint32(s.player.DuelArbiter >> 32),
		playerFieldDuelTeam:               s.player.DuelTeam,
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
		fields[unitFieldMaxPower1+i] = s.player.MaxPowers[i]
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
		fields[idx+2] = uint32(sk.Bonus)
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

func (s *session) teleportTo(mapID uint32, x, y, z, orientation float32) bool {
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
	oldMap := s.player.Map
	sameMap := oldMap == mapID
	if !sameMap && s.server != nil && !s.canStartMapTeleport(context.Background(), mapID) {
		return false
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
	s.selection = 0
	s.player.Selection = 0
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
		s.nearTeleportPending = false
		s.nearTeleportDest = nearTeleportDestination{}
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
		selfPacket, nearbyPacket := buildTeleportMovementPackets(s.playerGUID, movement)
		_ = s.write(uint16(protocol.OpcodeMSG_MOVE_TELEPORT_ACK), selfPacket, true)
		if s.server != nil {
			s.server.broadcastTeleportMovement(s, nearbyPacket)
		}
	} else {
		pending := protocol.NewBuffer(12)
		pending.WriteU32(mapID)
		if transportAttached {
			spawn, _ := s.server.transportSpawnForGUID(transportGUID)
			pending.WriteU32(spawn.Entry)
			pending.WriteU32(oldMap)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_TRANSFER_PENDING), pending.Bytes(), true)
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
		_ = s.write(uint16(protocol.OpcodeSMSG_NEW_WORLD), packet.Bytes(), true)
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.Exec("UPDATE characters SET map = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ? WHERE guid = ?",
			mapID, x, y, z, orientation, s.playerGUID)
	}
	if sameMap {
		s.lastFallZ = s.player.Z
	} else {
		s.lastFallZ = z
	}
	s.lastFallTime = 0
	return true
}

func (s *session) executeCommand(ctx context.Context, line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	// Commands resolve through the prefix-matching command tree
	// (ChatCommandNode::TryExecuteCommand), so '.mod spee 10' dispatches
	// to '.modify speed 10'.
	return s.dispatchCommand(ctx, fields)
}

func (s *session) handleCmdHelp(args []string) {
	s.sendSysMessage("=== Available Commands ===")
	s.sendSysMessage(".gm on|off|chat|fly|visible - Toggle GM modes")
	s.sendSysMessage(".tele <name> - Teleport to location")
	s.sendSysMessage(".go xyz <x> <y> <z> [map] - Teleport to coordinates")
	s.sendSysMessage(".modify hp|mana|speed|fly|scale|money|level <val>")
	s.sendSysMessage(".additem <itemId> [count] - Add item to inventory")
	s.sendSysMessage(".learn <spellId> | .unlearn <spellId> - Manage spells")
	s.sendSysMessage(".cast <spellId> - Cast a spell")
	s.sendSysMessage(".cheat explore on|off - Toggle explored areas")
	s.sendSysMessage(".lookup item|spell|creature|tele|quest <name>")
	s.sendSysMessage(".server info|motd - Server status and info")
	s.sendSysMessage(".character level|rename|customize|changefaction|changerace")
	s.sendSysMessage(".account addon|email|password|lock ... | .account set gmlevel|addon|password|sec ...")
}

func (s *session) handleCmdGM(args []string) {
	if len(args) == 0 {
		if s.player != nil && s.player.PlayerFlags&0x00000008 != 0 {
			s.sendSysMessage("GM mode is ON")
		} else {
			s.sendSysMessage("GM mode is OFF")
		}
		return
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "on":
		if s.player != nil {
			s.player.PlayerFlags |= playerFlagGM
			s.player.ExtraFlags |= playerExtraGMOn | playerExtraGMChat
			s.gmChat = true
			s.updateWorldReadyGM()
			s.persistExtraFlags()
			s.sendPlayerUpdate()
			s.refreshNearbyObjects(context.Background())
		}
		s.sendNotification("Game Master mode is ON")
		s.sendSysMessage("GM mode is ON")
	case "off":
		if s.player != nil {
			s.player.PlayerFlags &= ^playerFlagGM
			s.player.ExtraFlags &= ^(playerExtraGMOn | playerExtraGMInvisible | playerExtraGMChat)
			s.gmChat = false
			s.updateWorldReadyGM()
			s.persistExtraFlags()
			s.sendPlayerUpdate()
			s.refreshNearbyObjects(context.Background())
		}
		s.sendNotification("Game Master mode is OFF")
		s.sendSysMessage("GM mode is OFF")
	case "chat":
		if len(args) > 1 && strings.ToLower(args[1]) == "off" {
			s.gmChat = false
			if s.player != nil {
				s.player.ExtraFlags &= ^playerExtraGMChat
				s.persistExtraFlags() // persist chat badge state across restarts
			}
			s.sendNotification("GM chat badge is OFF")
			s.sendSysMessage("GM chat badge is OFF")
		} else {
			s.gmChat = true
			if s.player != nil {
				s.player.ExtraFlags |= playerExtraGMChat
				s.persistExtraFlags() // persist chat badge state across restarts
			}
			s.sendNotification("GM chat badge is ON")
			s.sendSysMessage("GM chat badge is ON")
		}
	case "fly":
		enable := true
		if len(args) > 1 && strings.ToLower(args[1]) == "off" {
			enable = false
		}
		s.setFlyMode(enable)
		name := ""
		if s.player != nil {
			name = s.player.Name
		}
		if enable {
			s.sendSysMessage("Set fly mode on for " + name)
		} else {
			s.sendSysMessage("Set fly mode off for " + name)
		}
	case "visible", "vis":
		if len(args) > 1 && strings.ToLower(args[1]) == "off" {
			if s.player != nil {
				s.player.ExtraFlags |= playerExtraGMInvisible | playerExtraGMOn
				s.player.PlayerFlags |= playerFlagGM
				s.updateWorldReadyGM()
				s.persistExtraFlags()
				s.sendPlayerUpdate()
				s.refreshNearbyObjects(context.Background())
			}
			s.sendNotification("You are now invisible.")
			s.sendSysMessage("GM visibility is OFF (Invisible)")
		} else {
			if s.player != nil {
				s.player.ExtraFlags &= ^playerExtraGMInvisible
				s.persistExtraFlags()
				s.sendPlayerUpdate()
				s.refreshNearbyObjects(context.Background())
			}
			s.sendNotification("You are now visible.")
			s.sendSysMessage("GM visibility is ON")
		}
	default:
		s.sendSysMessage("Syntax: .gm on|off|chat|fly|visible")
	}
}

func (s *session) handleCmdCheat(ctx context.Context, args []string) bool {
	if len(args) != 2 || strings.ToLower(args[0]) != "explore" {
		s.sendSysMessage("Syntax: .cheat explore on|off")
		return true
	}
	enabled := false
	switch strings.ToLower(args[1]) {
	case "on", "1", "true":
		enabled = true
	case "off", "0", "false":
	default:
		s.sendSysMessage("Syntax: .cheat explore on|off")
		return true
	}
	allowed := false
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		allowed, _ = accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionCommandCheatExplore)
	}
	if !allowed {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}
	if s.player == nil {
		return true
	}
	explored := uint32(0)
	if enabled {
		explored = ^uint32(0)
	}
	fields := make(map[int]uint32, len(s.player.ExploredZones))
	for index := range s.player.ExploredZones {
		s.player.ExploredZones[index] = explored
		fields[playerExploredZonesStart+index] = explored
	}
	s.persistExploredZones(ctx)
	s.sendPlayerValuesUpdate(fields)
	if enabled {
		s.sendSysMessage("All areas explored.")
	} else {
		s.sendSysMessage("All areas unexplored.")
	}
	return true
}

func (s *session) sendNotification(msg string) {
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
	if enable {
		_ = s.write(uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY), buf.Bytes(), true)
	} else {
		_ = s.write(uint16(protocol.OpcodeSMSG_MOVE_UNSET_CAN_FLY), buf.Bytes(), true)
	}
}

func (s *session) refreshNearbyObjects(ctx context.Context) {
	if !s.playerLoaded || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	isGM := (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0)
	if !isGM {
		// When GM mode is turned OFF, destroy any GM-only creatures that were previously visible
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
	s.lastStreamX = s.player.X
	s.lastStreamY = s.player.Y
	s.lastStreamZ = s.player.Z
	if packet, count, err := s.server.buildNearbyCreatureUpdates(ctx, *s.player, s.currentPlayerPhaseMask(), s); err == nil && count > 0 && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
	if packet, _, created := s.server.buildNearbyPlayerUpdatesWithCreated(s); packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		s.sendVisiblePlayerAuras(created)
		s.sendVisibleCreatureAuras(*s.player)
	}
	if packet, count, err := s.server.buildNearbyGameObjectUpdates(ctx, *s.player, false, s); err == nil && count > 0 && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
	s.streamDynamicSpellObjects()
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

func (s *session) handleCmdTele(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .tele <location_name>")
		return
	}
	locName := strings.Join(args, " ")
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	var mapID uint32
	var x, y, z, ori float32
	var foundName string
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT map, position_x, position_y, position_z, orientation, name FROM game_tele WHERE name LIKE ? LIMIT 1",
		"%"+locName+"%").Scan(&mapID, &x, &y, &z, &ori, &foundName)
	if errors.Is(err, sql.ErrNoRows) {
		s.sendSysMessage(fmt.Sprintf("Teleport location not found: %s", locName))
		return
	}
	if err != nil {
		s.sendSysMessage(fmt.Sprintf("Teleport lookup error: %v", err))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Teleporting to %s (%d, %.2f, %.2f, %.2f)...", foundName, mapID, x, y, z))
	s.teleportTo(mapID, x, y, z, ori)
}

func (s *session) handleCmdGo(ctx context.Context, args []string) {
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .go xyz <x> <y> <z> [map]")
		return
	}
	startIdx := 0
	if strings.ToLower(args[0]) == "xyz" {
		startIdx = 1
	}
	if len(args) < startIdx+3 {
		s.sendSysMessage("Syntax: .go xyz <x> <y> <z> [map]")
		return
	}
	x, err1 := strconv.ParseFloat(args[startIdx], 32)
	y, err2 := strconv.ParseFloat(args[startIdx+1], 32)
	z, err3 := strconv.ParseFloat(args[startIdx+2], 32)
	if err1 != nil || err2 != nil || err3 != nil {
		s.sendSysMessage("Invalid coordinates.")
		return
	}
	mapID := uint32(0)
	if s.player != nil {
		mapID = s.player.Map
	}
	if len(args) > startIdx+3 {
		if m, err := strconv.ParseUint(args[startIdx+3], 10, 32); err == nil {
			mapID = uint32(m)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Teleporting to Map %d (%.2f, %.2f, %.2f)...", mapID, x, y, z))
	s.teleportTo(mapID, float32(x), float32(y), float32(z), 0)
}

func (s *session) handleCmdModify(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .modify hp|mana|speed|fly|scale|money|level <val>")
		return
	}
	sub := strings.ToLower(args[0])
	valStr := args[1]
	switch sub {
	case "hp", "health":
		val, err := strconv.ParseUint(valStr, 10, 32)
		if err != nil {
			s.sendSysMessage("Invalid value.")
			return
		}
		if s.player != nil {
			s.player.Health = uint32(val)
			s.player.MaxHealth = uint32(val)
			s.sendPlayerUpdate()
		}
		s.sendSysMessage(fmt.Sprintf("Health set to %d.", val))
	case "mana", "power":
		val, err := strconv.ParseUint(valStr, 10, 32)
		if err != nil {
			s.sendSysMessage("Invalid value.")
			return
		}
		if s.player != nil {
			s.player.Powers[0] = uint32(val)
			s.player.MaxPowers[0] = uint32(val)
			s.sendPlayerUpdate()
		}
		s.sendSysMessage(fmt.Sprintf("Mana set to %d.", val))
	case "speed", "run":
		val, err := strconv.ParseFloat(valStr, 32)
		if err != nil || val <= 0 {
			s.sendSysMessage("Invalid speed multiplier (e.g. 1.0, 2.5).")
			return
		}
		speed := float32(val) * 7.0
		buf := protocol.NewBuffer(17)
		buf.WritePackedGUID(s.playerGUID)
		buf.WriteU32(0)
		buf.WriteU8(1)
		buf.WriteF32(speed)
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_RUN_SPEED_CHANGE), buf.Bytes(), true)
		s.sendSysMessage(fmt.Sprintf("Speed set to %.2fx (%.2f).", val, speed))
	case "fly":
		val, err := strconv.ParseFloat(valStr, 32)
		if err != nil || val <= 0 {
			s.sendSysMessage("Invalid flight speed multiplier.")
			return
		}
		speed := float32(val) * 7.0
		buf := protocol.NewBuffer(16)
		buf.WritePackedGUID(s.playerGUID)
		buf.WriteU32(0)
		buf.WriteF32(speed)
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE), buf.Bytes(), true)
		s.sendSysMessage(fmt.Sprintf("Flight speed set to %.2fx (%.2f).", val, speed))
	case "scale":
		val, err := strconv.ParseFloat(valStr, 32)
		if err != nil || val <= 0 {
			s.sendSysMessage("Invalid scale value.")
			return
		}
		s.scale = float32(val)
		s.sendPlayerUpdate()
		s.sendSysMessage(fmt.Sprintf("Scale set to %.2f.", val))
	case "money", "gold":
		val, err := strconv.ParseUint(valStr, 10, 32)
		if err != nil {
			s.sendSysMessage("Invalid money amount.")
			return
		}
		amount := uint32(val)
		if sub == "gold" {
			amount *= 10000
		}
		if s.player != nil {
			s.player.Money = amount
			s.sendPlayerUpdate()
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", amount, s.playerGUID)
		}
		s.sendSysMessage(fmt.Sprintf("Money set to %d copper (%d gold).", amount, amount/10000))
	case "level":
		val, err := strconv.ParseUint(valStr, 10, 8)
		if err != nil || val == 0 || val > 80 {
			s.sendSysMessage("Invalid level (1-80).")
			return
		}
		if s.player != nil {
			s.player.Level = uint8(val)
			s.sendPlayerUpdate()
		}
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET level = ? WHERE guid = ?", val, s.playerGUID)
		}
		s.sendSysMessage(fmt.Sprintf("Level set to %d.", val))
	default:
		s.sendSysMessage(fmt.Sprintf("Unknown modify property: %s", sub))
	}
}

func (s *session) handleCmdAddItem(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .additem <itemId> [count]")
		return
	}
	itemID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid item ID.")
		return
	}
	count := uint32(1)
	if len(args) > 1 {
		if c, err := strconv.ParseUint(args[1], 10, 32); err == nil && c > 0 {
			count = uint32(c)
		}
	}
	var itemName string
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name FROM item_template WHERE entry = ?", itemID).Scan(&itemName)
	}
	if itemName == "" {
		itemName = fmt.Sprintf("Item #%d", itemID)
	}

	res, err := s.storeOrStackItem(ctx, s.playerGUID, uint32(itemID), count)
	if err != nil {
		s.sendSysMessage("Inventory is full.")
		return
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	pushSlot := uint32(res.Slot)
	if res.IsStack {
		pushSlot = 0xFFFFFFFF
	}
	push := buildItemPushResult(s.playerGUID, res.ClientBag, pushSlot, uint32(itemID), count, res.InventoryCount, res.IsStack)
	_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), push, true)
	s.sendSysMessage(fmt.Sprintf("Added item %s [%d] x%d to inventory.", itemName, itemID, count))
}

func (s *session) handleCmdLearn(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .learn <spellId> | .learn all")
		return
	}
	if strings.ToLower(args[0]) == "all" {
		s.sendSysMessage("Learned all class spells.")
		return
	}
	spellID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid spell ID.")
		return
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"INSERT INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)",
			s.playerGUID, spellID)
	}
	s.learnOwnerPetAuraSources(ctx, uint32(spellID))
	pkt := protocol.NewBuffer(6)
	pkt.WriteU32(uint32(spellID))
	pkt.WriteU16(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), pkt.Bytes(), true)
	s.sendSysMessage(fmt.Sprintf("Learned spell %d.", spellID))
}

func (s *session) handleCmdUnlearn(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .unlearn <spellId|talents>")
		return
	}
	// Reference Player::ResetTalents via .unlearn talents with the escalating
	// gold cost curve; GMs reset for free like the reference command paths.
	if strings.EqualFold(args[0], "talents") {
		if s.player == nil {
			return
		}
		free := s.player.ExtraFlags&playerExtraGMOn != 0
		if !s.resetTalents(ctx, free) {
			s.sendSysMessage("You do not have enough gold to reset your talents.")
			return
		}
		if free {
			s.sendSysMessage("Your talents have been reset (no cost).")
		} else {
			s.sendSysMessage(fmt.Sprintf("Your talents have been reset for %dg.", s.player.ResetTalentsCost/goldUnit))
		}
		return
	}
	spellID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid spell ID.")
		return
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"DELETE FROM character_spell WHERE guid = ? AND spell = ?",
			s.playerGUID, spellID)
	}
	if s.hasAura(uint32(spellID)) {
		s.removeAura(uint32(spellID))
	}
	s.removeOwnerPetAurasForSpell(ctx, uint32(spellID))
	pkt := protocol.NewBuffer(4)
	pkt.WriteU32(uint32(spellID))
	_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), pkt.Bytes(), true)
	s.sendSysMessage(fmt.Sprintf("Unlearned spell %d.", spellID))
}

func (s *session) handleCmdCast(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .cast <spellId>")
		return
	}
	spellID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid spell ID.")
		return
	}
	if s.server == nil || s.server.Data == nil {
		s.sendSysMessage("Spell data is unavailable.")
		return
	}
	if _, found, err := s.server.Data.Spell(uint32(spellID)); err != nil || !found {
		s.sendSysMessage("Unknown spell ID.")
		return
	}
	targetGUID := s.selection
	if targetGUID == 0 {
		targetGUID = s.playerGUID
	}
	s.castSpellDirect(ctx, uint32(spellID), targetGUID)
	s.sendSysMessage(fmt.Sprintf("Casting spell %d.", spellID))
}

func (s *session) handleCmdLookup(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .lookup item|spell|creature|tele|quest <name>")
		return
	}
	sub := strings.ToLower(args[0])
	query := "%" + strings.Join(args[1:], " ") + "%"
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage("Database not available.")
		return
	}
	switch sub {
	case "item":
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT entry, name FROM item_template WHERE name LIKE ? LIMIT 10", query)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Lookup failed: %v", err))
			return
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id uint32
			var name string
			if err := rows.Scan(&id, &name); err == nil {
				s.sendSysMessage(fmt.Sprintf("Item %d: %s", id, name))
				count++
			}
		}
		if count == 0 {
			s.sendSysMessage("No items found.")
		}
	case "creature", "npc":
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT entry, name FROM creature_template WHERE name LIKE ? LIMIT 10", query)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Lookup failed: %v", err))
			return
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id uint32
			var name string
			if err := rows.Scan(&id, &name); err == nil {
				s.sendSysMessage(fmt.Sprintf("Creature %d: %s", id, name))
				count++
			}
		}
		if count == 0 {
			s.sendSysMessage("No creatures found.")
		}
	case "tele":
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT id, name FROM game_tele WHERE name LIKE ? LIMIT 10", query)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Lookup failed: %v", err))
			return
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id uint32
			var name string
			if err := rows.Scan(&id, &name); err == nil {
				s.sendSysMessage(fmt.Sprintf("Teleport %d: %s", id, name))
				count++
			}
		}
		if count == 0 {
			s.sendSysMessage("No teleport locations found.")
		}
	case "quest":
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT ID, LogTitle FROM quest_template WHERE LogTitle LIKE ? LIMIT 10", query)
		if err != nil {
			s.sendSysMessage(fmt.Sprintf("Lookup failed: %v", err))
			return
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id uint32
			var title string
			if err := rows.Scan(&id, &title); err == nil {
				s.sendSysMessage(fmt.Sprintf("Quest %d: %s", id, title))
				count++
			}
		}
		if count == 0 {
			s.sendSysMessage("No quests found.")
		}
	default:
		s.sendSysMessage(fmt.Sprintf("Unknown lookup type: %s", sub))
	}
}

func (s *session) handleCmdServer(ctx context.Context, args []string) {
	if len(args) == 0 || strings.ToLower(args[0]) == "info" {
		s.server.sessionsMu.RLock()
		online := len(s.server.sessions)
		s.server.sessionsMu.RUnlock()
		s.sendSysMessage(fmt.Sprintf("Go-MorenoCore (WotLK 3.3.5a 12340) | Online players: %d | Realm ID: %d", online, s.server.RealmID))
		return
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "motd":
		s.sendSysMessage(fmt.Sprintf("MOTD: %s", s.server.Config.Motd))
	case "restart", "shutdown":
		s.sendSysMessage("Server restart/shutdown command issued.")
	default:
		s.sendSysMessage("Syntax: .server info|motd")
	}
}

func (s *session) handleCmdCharacter(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .character level|rename|customize|changefaction|changerace")
		return
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "level":
		if len(args) > 1 {
			s.handleCmdModify(ctx, []string{"level", args[1]})
		} else {
			s.sendSysMessage("Syntax: .character level <1-80>")
		}
	case "rename":
		if s.player != nil {
			s.player.AtLogin |= 0x01
		}
		s.sendSysMessage("Rename flag set. Please relog to choose a new name.")
	case "customize":
		if s.player != nil {
			s.player.AtLogin |= 0x08
		}
		s.sendSysMessage("Customize flag set. Please relog to customize appearance.")
	case "changefaction":
		if s.player != nil {
			s.player.AtLogin |= 0x40
		}
		s.sendSysMessage("Change faction flag set. Please relog to change faction.")
	case "changerace":
		if s.player != nil {
			s.player.AtLogin |= 0x80
		}
		s.sendSysMessage("Change race flag set. Please relog to change race.")
	default:
		s.sendSysMessage(fmt.Sprintf("Unknown character subcommand: %s", sub))
	}
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

// accountSecurityLevel mirrors AccountMgr::GetSecurity: highest access row
// for this realm (or realm -1, the all-realms row) wins.
func (s *session) accountSecurityLevel(ctx context.Context, id uint32) uint8 {
	db := s.authDB()
	if db == nil {
		return 0
	}
	var level uint8
	if err := db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(SecurityLevel), 0) FROM account_access WHERE AccountID = ? AND (RealmID = ? OR RealmID = -1)",
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

func (s *session) handleCmdAccount(ctx context.Context, args []string) {
	if len(args) == 0 {
		// HandleAccountCommand (cs_account.cpp:564): bare ".account" shows
		// the session security level and the account's own email address.
		s.sendSysMessage(fmt.Sprintf("Your account security level: %d.", s.security))
		if db := s.authDB(); db != nil {
			var email string
			if err := db.QueryRowContext(ctx, "SELECT email FROM account WHERE id = ?", s.accountID).Scan(&email); err == nil && email != "" {
				s.sendSysMessage(fmt.Sprintf("Email: %s", email))
			}
		}
		return
	}
	switch strings.ToLower(args[0]) {
	case "set":
		s.handleCmdAccountSet(ctx, args[1:])
	case "addon":
		s.handleCmdAccountAddon(ctx, args[1:])
	case "email":
		s.handleCmdAccountEmail(ctx, args[1:])
	case "password":
		s.handleCmdAccountPassword(ctx, args[1:])
	case "lock":
		s.handleCmdAccountLock(ctx, args[1:])
	case "create":
		s.handleCmdAccountCreate(ctx, args[1:])
	case "delete":
		s.handleCmdAccountDelete(ctx, args[1:])
	case "onlinelist":
		s.handleCmdAccountOnlineList(ctx)
	case "2fa":
		s.handleCmdAccount2FA(ctx, args[1:])
	default:
		s.sendSysMessage("Syntax: .account [addon <expansion>|email <old> <password> <new> <confirm>|password <old> <new> <confirm>|lock <ip|country> <on|off>|2fa setup|2fa remove|create <account> <password> [<email>]|delete <account>|onlinelist|set ...]")
	}
}

// handleCmdAccountSet dispatches the ".account set" subgroup
// (cs_account.cpp accountSetCommandTable).
func (s *session) handleCmdAccountSet(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .account set gmlevel|seclevel|addon|password|sec|2fa <account> ...")
		return
	}
	switch strings.ToLower(args[0]) {
	case "gmlevel", "sec", "seclevel":
		s.handleCmdAccountSetSecLevel(ctx, args[1:])
	case "addon":
		s.handleCmdAccountSetAddon(ctx, args[1:])
	case "password":
		s.handleCmdAccountSetPassword(ctx, args[1:])
	case "2fa":
		s.handleCmdAccountSet2FA(ctx, args[1:])
	default:
		if len(args) >= 2 && strings.ToLower(args[0]) == "sec" {
			switch strings.ToLower(args[1]) {
			case "email", "regmail":
				s.handleCmdAccountSetSecEmail(ctx, "sec "+strings.ToLower(args[1]), args[2:])
				return
			}
		}
		s.sendSysMessage("Syntax: .account set gmlevel|seclevel|addon|password|sec|2fa <account> ...")
	}
}

// handleCmdAccountSetSecLevel extends the native ".account set gmlevel"
// with the HasLowerSecurityAccount guard from
// HandleAccountSetSecLevelCommand (cs_account.cpp:657).
func (s *session) handleCmdAccountSetSecLevel(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .account set gmlevel <account> <level>")
		return
	}
	targetAcct := args[0]
	val := args[1]
	sec, err := strconv.ParseUint(val, 10, 8)
	if err != nil || sec >= 4 { // SEC_CONSOLE = 4; HandleAccountSetSecLevelCommand rejects >= SEC_CONSOLE
		s.sendSysMessage("Invalid security level (0-3).")
		return
	}
	targetID := s.accountIDByName(ctx, targetAcct)
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", targetAcct))
		return
	}
	// can set security level only for target with less security and to less
	// security than we have (also restricts setting our own security).
	if !s.canModifyAccount(ctx, targetID) || uint8(sec) >= s.security {
		if uint8(sec) >= s.security {
			s.sendSysMessage("Your security level is too low.")
		}
		return
	}
	if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		_, err = s.server.AuthStore.DB.ExecContext(ctx,
			"INSERT INTO account_access (AccountID, SecurityLevel, RealmID) VALUES ((SELECT id FROM account WHERE UPPER(username) = UPPER(?)), ?, ?) ON CONFLICT(AccountID, RealmID) DO UPDATE SET SecurityLevel = ?",
			targetAcct, sec, s.server.RealmID, sec)
		if err != nil {
			_, _ = s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account_access SET SecurityLevel = ? WHERE AccountID = (SELECT id FROM account WHERE UPPER(username) = UPPER(?))", sec, targetAcct)
		}
	}
	s.sendSysMessage(fmt.Sprintf("Security level for %s set to %d.", targetAcct, sec))
}

// handleCmdAccountSetAddon mirrors HandleAccountSetAddonCommand
// (cs_account.cpp:604): ".account set addon [<account>] <expansion>".
func (s *session) handleCmdAccountSetAddon(ctx context.Context, args []string) {
	var targetID uint32
	var targetName string
	var expansion uint8
	switch len(args) {
	case 1:
		targetID = s.accountID
		targetName = s.accountNameByID(ctx, targetID)
		v, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set addon [<account>] <expansion>")
			return
		}
		expansion = uint8(v)
	case 2:
		targetID = s.accountIDByName(ctx, args[0])
		if targetID == 0 {
			s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
			return
		}
		targetName = args[0]
		v, err := strconv.ParseUint(args[1], 10, 8)
		if err != nil {
			s.sendSysMessage("Syntax: .account set addon [<account>] <expansion>")
			return
		}
		expansion = uint8(v)
	default:
		s.sendSysMessage("Syntax: .account set addon [<account>] <expansion>")
		return
	}
	if targetID != s.accountID && !s.canModifyAccount(ctx, targetID) {
		return
	}
	if s.server != nil && uint32(expansion) > s.server.Config.Expansion { // CONFIG_EXPANSION
		s.sendSysMessage("Invalid expansion value.")
		return
	}
	if db := s.authDB(); db != nil {
		_, _ = db.ExecContext(ctx, "UPDATE account SET expansion = ? WHERE id = ?", expansion, targetID)
	}
	s.sendSysMessage(fmt.Sprintf("Expansion for account %s [%d] set to %d.", targetName, targetID, expansion))
}

// handleCmdAccountSetPassword mirrors HandleAccountSetPasswordCommand
// (cs_account.cpp:747): ".account set password <account> <password> <confirm>".
func (s *session) handleCmdAccountSetPassword(ctx context.Context, args []string) {
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .account set password <account> <password> <confirm>")
		return
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return
	}
	if !s.canModifyAccount(ctx, targetID) {
		return
	}
	if args[1] != args[2] {
		s.sendSysMessage("New passwords do not match.")
		return
	}
	if msg := s.changeAccountPassword(ctx, targetID, args[1]); msg != "" {
		s.sendSysMessage(msg)
		return
	}
	s.sendSysMessage("Password changed.")
}

// handleCmdAccountSetSecEmail mirrors HandleAccountSetEmailCommand /
// HandleAccountSetRegEmailCommand (cs_account.cpp:862/917):
// ".account set sec email|regmail <account> <email> <confirm>".
func (s *session) handleCmdAccountSetSecEmail(ctx context.Context, action string, args []string) {
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .account set sec email|regmail <account> <email> <confirm>")
		return
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return
	}
	if !s.canModifyAccount(ctx, targetID) {
		return
	}
	if args[1] != args[2] {
		s.sendSysMessage("New emails do not match.")
		return
	}
	if msg := s.changeAccountEmail(ctx, targetID, args[1], strings.HasSuffix(action, "regmail")); msg != "" {
		s.sendSysMessage(msg)
		return
	}
	s.sendSysMessage("Email changed.")
}

// handleCmdAccountAddon mirrors HandleAccountAddonCommand (cs_account.cpp:217):
// ".account addon <expansion>" sets the caller's own expansion level.
func (s *session) handleCmdAccountAddon(ctx context.Context, args []string) {
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .account addon <expansion>")
		return
	}
	expansion, err := strconv.ParseUint(args[0], 10, 8)
	if err != nil {
		s.sendSysMessage("Syntax: .account addon <expansion>")
		return
	}
	if s.server != nil && uint32(expansion) > s.server.Config.Expansion { // CONFIG_EXPANSION
		s.sendSysMessage("Invalid expansion value.")
		return
	}
	if db := s.authDB(); db != nil {
		_, _ = db.ExecContext(ctx, "UPDATE account SET expansion = ? WHERE id = ?", uint8(expansion), s.accountID)
	}
	s.sendSysMessage(fmt.Sprintf("Expansion set to %d.", expansion))
}

// handleCmdAccountEmail mirrors HandleAccountEmailCommand (cs_account.cpp:430):
// ".account email <oldEmail> <password> <newEmail> <confirmEmail>" changes
// the caller's own email after verifying the old email and password.
func (s *session) handleCmdAccountEmail(ctx context.Context, args []string) {
	if len(args) < 4 {
		s.sendSysMessage("Syntax: .account email <oldEmail> <password> <newEmail> <confirmEmail>")
		return
	}
	oldEmail, password, email, emailConfirm := args[0], args[1], args[2], args[3]
	db := s.authDB()
	if db == nil {
		return
	}
	var current string
	if err := db.QueryRowContext(ctx, "SELECT email FROM account WHERE id = ?", s.accountID).Scan(&current); err != nil {
		s.sendSysMessage("Could not change email.")
		return
	}
	if upperOnlyLatin(current) != upperOnlyLatin(oldEmail) { // AccountMgr::CheckEmail
		s.sendSysMessage("Wrong email.")
		return
	}
	if !s.checkAccountPassword(ctx, s.accountID, password) {
		s.sendSysMessage("Wrong old password.")
		return
	}
	if upperOnlyLatin(email) == upperOnlyLatin(oldEmail) {
		s.sendSysMessage("Old email is the new email.")
		return
	}
	if email != emailConfirm {
		s.sendSysMessage("New emails do not match.")
		return
	}
	if msg := s.changeAccountEmail(ctx, s.accountID, email, false); msg != "" {
		s.sendSysMessage(msg)
		return
	}
	s.sendSysMessage("Email changed.")
}

// handleCmdAccountPassword mirrors HandleAccountPasswordCommand
// (cs_account.cpp:499): ".account password <old> <new> <confirm>" changes
// the caller's own password after verifying the old one. The optional
// email-confirmation branch (CONFIG_ACC_PASSCHANGESEC) has no config
// counterpart here, so it is treated as PW_NONE.
func (s *session) handleCmdAccountPassword(ctx context.Context, args []string) {
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .account password <oldPassword> <newPassword> <confirmPassword>")
		return
	}
	oldPassword, newPassword, confirmPassword := args[0], args[1], args[2]
	if !s.checkAccountPassword(ctx, s.accountID, oldPassword) {
		s.sendSysMessage("Wrong old password.")
		return
	}
	if newPassword != confirmPassword {
		s.sendSysMessage("New passwords do not match.")
		return
	}
	if msg := s.changeAccountPassword(ctx, s.accountID, newPassword); msg != "" {
		s.sendSysMessage(msg)
		return
	}
	s.sendSysMessage("Password changed.")
}

// handleCmdAccountLock mirrors HandleAccountLockIpCommand /
// HandleAccountLockCountryCommand (cs_account.cpp:379/409):
// ".account lock ip|country on|off".
func (s *session) handleCmdAccountLock(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .account lock ip|country on|off")
		return
	}
	db := s.authDB()
	if db == nil {
		return
	}
	state := strings.ToLower(args[1]) == "on"
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
	case "country":
		country := "00"
		if state {
			country = ""
			if s.server != nil && s.server.ipLocations != nil {
				country = s.server.ipLocations.Country(s.remoteIP())
			}
			if country == "" {
				s.sendSysMessage("No IP location information - account not locked.")
				return
			}
		}
		_, _ = db.ExecContext(ctx, "UPDATE account SET lock_country = ? WHERE id = ?", country, s.accountID)
		if state {
			s.sendSysMessage(fmt.Sprintf("Account locked to country %s.", country))
		} else {
			s.sendSysMessage("Account unlocked.")
		}
	default:
		s.sendSysMessage("Syntax: .account lock ip|country on|off")
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
func (s *session) handleCmdAccountCreate(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .account create <account> <password> [<email>]")
		return
	}
	accountName, password := args[0], args[1]
	email := ""
	if len(args) > 2 {
		email = args[2]
	}
	if strings.Contains(accountName, "@") {
		// LANG_ACCOUNT_USE_BNET_COMMANDS (cs_account.cpp:241)
		s.sendSysMessage("Use battlenet account commands for battlenet accounts.")
		return
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
	default:
		s.sendSysMessage(fmt.Sprintf("Account %s was not created.", accountName))
	}
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
func (s *session) handleCmdAccountDelete(ctx context.Context, args []string) {
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .account delete <account>")
		return
	}
	accountName := args[0]
	targetID := s.accountIDByName(ctx, accountName)
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
		return
	}
	// cs_account.cpp: commands can delete only accounts with lower security
	// (this also rejects self-application); mirrors the canModifyAccount
	// HasLowerSecurityAccount guard used by the set subgroup.
	if !s.canModifyAccount(ctx, targetID) {
		return
	}
	switch s.deleteAccount(ctx, targetID) {
	case accountOpOK:
		s.sendSysMessage(fmt.Sprintf("Account %s deleted.", accountName))
	case accountOpNameNotExist:
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
	default:
		s.sendSysMessage(fmt.Sprintf("Account %s was not deleted.", accountName))
	}
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
		tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
		if err != nil {
			return accountOpDBInternalError
		}
		if err := s.deleteCharacterReturnMails(ctx, tx, guid, accountID); err != nil {
			tx.Rollback()
			return accountOpDBInternalError
		}
		if err := deleteCharacterOwnedState(ctx, tx, guid); err != nil {
			tx.Rollback()
			return accountOpDBInternalError
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM characters WHERE guid = ?", guid); err != nil {
			tx.Rollback()
			return accountOpDBInternalError
		}
		if err := tx.Commit(); err != nil {
			return accountOpDBInternalError
		}
	}
	for _, stmt := range []string{"CHAR_DEL_TUTORIALS", "CHAR_DEL_ACCOUNT_DATA", "CHAR_DEL_CHARACTER_BAN"} {
		if _, err := s.server.CharactersStore.ExecStatement(ctx, database.StatementID(stmt), accountID); err != nil {
			return accountOpDBInternalError
		}
	}
	tx, err := s.server.AuthStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return accountOpDBInternalError
	}
	for _, stmt := range []string{"LOGIN_DEL_ACCOUNT", "LOGIN_DEL_ACCOUNT_ACCESS", "LOGIN_DEL_REALM_CHARACTERS", "LOGIN_DEL_ACCOUNT_BANNED", "LOGIN_DEL_ACCOUNT_MUTED"} {
		if _, err := s.server.AuthStore.ExecStatementTx(ctx, tx, database.StatementID(stmt), accountID); err != nil {
			tx.Rollback()
			return accountOpDBInternalError
		}
	}
	if err := tx.Commit(); err != nil {
		return accountOpDBInternalError
	}
	return accountOpOK
}

// handleCmdAccountOnlineList mirrors HandleAccountOnlineListCommand
// (cs_account.cpp:333): ".account onlinelist".
func (s *session) handleCmdAccountOnlineList(ctx context.Context) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.AuthStore == nil {
		return
	}
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_CHARACTER_ONLINE")
	if err != nil {
		return
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
			return
		}
		online = append(online, c)
	}
	if len(online) == 0 {
		s.sendSysMessage("No characters online.")
		return
	}
	s.sendSysMessage("Account | Character | LastIP | Map | Zone | Expansion | GM")
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
}

// totpSuggestions mirrors the static suggestions map in
// HandleAccount2FASetupCommand (cs_account.cpp:119): per-account pending
// 2FA secrets awaiting token confirmation.
var (
	totpSuggestionsMu sync.Mutex
	totpSuggestions   = make(map[uint32][]byte)
)

// handleCmdAccount2FA dispatches ".account 2fa setup|remove".
func (s *session) handleCmdAccount2FA(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .account 2fa setup [<token>] | .account 2fa remove [<token>]")
		return
	}
	var token *uint32
	if len(args) > 1 {
		t, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			s.sendSysMessage("Invalid token.")
			return
		}
		t32 := uint32(t)
		token = &t32
	}
	switch strings.ToLower(args[0]) {
	case "setup":
		s.handleCmdAccount2FASetup(ctx, token)
	case "remove":
		s.handleCmdAccount2FARemove(ctx, token)
	default:
		s.sendSysMessage("Syntax: .account 2fa setup [<token>] | .account 2fa remove [<token>]")
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
func (s *session) handleCmdAccount2FASetup(ctx context.Context, token *uint32) {
	masterKey, masterPresent := s.totpMasterKey()
	if s.server == nil || s.server.AuthStore == nil || !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return
	}
	accountID := s.accountID
	var existing []byte
	totpRow, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_TOTP_SECRET", accountID)
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if err := totpRow.Scan(&existing); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if len(existing) != 0 {
		s.sendSysMessage("2FA is already set up on this account.")
		return
	}
	totpSuggestionsMu.Lock()
	secret, ok := totpSuggestions[accountID]
	if !ok {
		secret = make([]byte, 20) // TOTP::RECOMMENDED_SECRET_LENGTH (TOTP.h)
		if _, err := rand.Read(secret); err != nil {
			totpSuggestionsMu.Unlock()
			s.sendSysMessage("Unknown error.")
			return
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
				return
			}
			if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", stored, accountID); err != nil {
				s.sendSysMessage("Unknown error.")
				return
			}
			totpSuggestionsMu.Lock()
			delete(totpSuggestions, accountID)
			totpSuggestionsMu.Unlock()
			s.sendSysMessage("2FA setup complete.")
			return
		}
		s.sendSysMessage("Invalid token.")
	}
	// New suggestion, or no token specified: output the TOTP parameters.
	s.sendSysMessage(fmt.Sprintf("Suggested 2FA secret: %s", crypto.Base32Encode(secret)))
}

// handleCmdAccount2FARemove mirrors HandleAccount2FARemoveCommand (cs_account.cpp:149).
func (s *session) handleCmdAccount2FARemove(ctx context.Context, token *uint32) {
	masterKey, masterPresent := s.totpMasterKey()
	if s.server == nil || s.server.AuthStore == nil || !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return
	}
	accountID := s.accountID
	var stored []byte
	totpRow, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_TOTP_SECRET", accountID)
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if err := totpRow.Scan(&stored); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if len(stored) == 0 {
		s.sendSysMessage("2FA is not set up on this account.")
		return
	}
	if token != nil {
		secret := append([]byte(nil), stored...)
		if err := crypto.DecryptWithTrailingIVAndTag(&secret, masterKey); err != nil {
			s.debug("account 2fa remove: invalid ciphertext", "account", s.accountName)
			s.sendSysMessage("Unknown error.")
			return
		}
		if crypto.ValidateTOTP(secret, *token, time.Now()) {
			if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", nil, accountID); err != nil {
				s.sendSysMessage("Unknown error.")
				return
			}
			s.sendSysMessage("2FA removed.")
			return
		}
		s.sendSysMessage("Invalid token.")
	}
	s.sendSysMessage("You must supply your current token to remove 2FA.")
}

// handleCmdAccountSet2FA mirrors HandleAccountSet2FACommand (cs_account.cpp:798):
// ".account set 2fa <account> <secret|off>".
func (s *session) handleCmdAccountSet2FA(ctx context.Context, args []string) {
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .account set 2fa <account> <secret|off>")
		return
	}
	targetID := s.accountIDByName(ctx, args[0])
	if targetID == 0 {
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", args[0]))
		return
	}
	// HandleAccountSet2FACommand uses the HasLowerSecurityAccount guard.
	if !s.canModifyAccount(ctx, targetID) {
		return
	}
	if s.server == nil || s.server.AuthStore == nil {
		return
	}
	if args[1] == "off" {
		if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", nil, targetID); err != nil {
			s.sendSysMessage("Unknown error.")
			return
		}
		s.sendSysMessage("2FA removed.")
		return
	}
	masterKey, masterPresent := s.totpMasterKey()
	if !masterPresent {
		s.sendSysMessage("2FA commands are not set up (no TOTP master secret configured).")
		return
	}
	decoded, err := crypto.Base32Decode(args[1])
	if err != nil {
		s.sendSysMessage("Invalid 2FA secret.")
		return
	}
	if len(decoded)+crypto.AESIVSize+crypto.AESTagSize > 128 {
		s.sendSysMessage("2FA secret too long.")
		return
	}
	stored := append([]byte(nil), decoded...)
	if err := crypto.EncryptWithRandomIV(&stored, masterKey); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_TOTP_SECRET", stored, targetID); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("2FA secret set for account %s.", args[0]))
}

// handleCmdAchievement processes ".achievement add" (cs_achievement.cpp,
// AddSC_achievement_commandscript): HandleAchievementAddCommand grants the
// given achievement to the selected player (the commander's own player when
// nothing is targeted), mirroring Player::CompletedAchievement through
// completeAchievement. Gated by RBAC_PERM_COMMAND_ACHIEVEMENT_ADD (231);
// Console::No has no Go console path to suppress.
func (s *session) handleCmdAchievement(ctx context.Context, args []string) {
	if len(args) == 0 || !strings.EqualFold(args[0], "add") {
		s.sendSysMessage("Syntax: .achievement add <achievementId>")
		return
	}
	args = args[1:]
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .achievement add <achievementId>")
		return
	}
	achievementID, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid achievement ID.")
		return
	}
	allowed := s.security >= 1
	if !allowed && s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		hasPerm, permErr := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionCommandAchievementAdd)
		if permErr == nil && hasPerm {
			allowed = true
		}
	}
	if !allowed {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	// The Trinity ChatCommand parser resolves the AchievementEntry* argument
	// against sAchievementStore before invoking the handler; an unknown id
	// never reaches HandleAchievementAddCommand.
	achievementIndex.mu.RLock()
	_, found := achievementIndex.achieveByID[uint32(achievementID)]
	achievementIndex.mu.RUnlock()
	if !found {
		s.sendSysMessage("Unknown achievement ID.")
		return
	}
	// ChatHandler::getSelectedPlayer: own player when nothing is targeted,
	// otherwise the connected player matching the selection (null, i.e. no
	// character, when the target is offline or not a player).
	target := s
	if s.selection != 0 {
		if s.server == nil {
			return
		}
		target = s.server.playerSessionForGUID(s.selection)
		if target == nil {
			s.sendSysMessage("No character selected.")
			return
		}
	}
	target.completeAchievement(uint32(achievementID))
	s.sendSysMessage(fmt.Sprintf("Achievement %d added.", achievementID))
}

// commandAllowed mirrors the achievement-port permission gate
// (handleCmdAchievement): security >= 1 grants the command, otherwise the
// RBAC grant is consulted via accountHasPermission.
func (s *session) commandAllowed(ctx context.Context, permissionID uint32) bool {
	if s.security >= 1 {
		return true
	}
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		if hasPerm, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionID); err == nil && hasPerm {
			return true
		}
	}
	return false
}

// splitQuotedArgs re-splits command fields after the dispatcher tokenized the
// raw line on whitespace, keeping "double-quoted segments" together (quotes
// retained), so the Trinity ChatCommand QuotedString arguments survive the
// dispatcher's strings.Fields tokenization.
func splitQuotedArgs(args []string) []string {
	line := strings.Join(args, " ")
	var toks []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			inQuote = !inQuote
			cur.WriteByte(c)
			continue
		}
		if c == ' ' && !inQuote {
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	return toks
}

// unquoteCommandToken mirrors the Trinity ChatCommand QuotedString unwrap.
func unquoteCommandToken(tok string) string {
	if len(tok) >= 2 && strings.HasPrefix(tok, "\"") && strings.HasSuffix(tok, "\"") {
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
// (enuminfo_ArenaTeam.cpp:34-36). The numeric and short aliases are a
// Go-side convenience; the C++ parser only accepts the constant-name
// prefixes.
func parseArenaTeamType(tok string) (uint32, bool) {
	switch strings.ToLower(tok) {
	case "2", "2v2":
		return ArenaTeamType2v2, true
	case "3", "3v3":
		return ArenaTeamType3v3, true
	case "5", "5v5":
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
	return 0, false
}

// resolveArenaCaptain mirrors the Trinity PlayerIdentifier parse used by the
// arena command's Optional<PlayerIdentifier> arguments (ChatCommandTags.cpp
// PlayerIdentifier::TryConsume): a numeric token is a character low GUID,
// otherwise the token is normalized and looked up as a character name.
func (s *session) resolveArenaCaptain(ctx context.Context, cdb *sql.DB, token string) (uint64, string, bool) {
	if low, err := strconv.ParseUint(token, 10, 32); err == nil {
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
func (s *session) handleCmdArena(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .arena create|disband|rename|captain|info|lookup ...")
		return
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
		s.sendSysMessage("Syntax: .arena create|disband|rename|captain|info|lookup ...")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "create":
		s.handleCmdArenaCreate(ctx, rest)
	case "disband":
		s.handleCmdArenaDisband(ctx, rest)
	case "rename":
		s.handleCmdArenaRename(ctx, rest)
	case "captain":
		s.handleCmdArenaCaptain(ctx, rest)
	case "info":
		s.handleCmdArenaInfo(ctx, rest)
	case "lookup":
		s.handleCmdArenaLookup(ctx, rest)
	}
}

// handleCmdArenaCreate processes ".arena create [<captain>] <name> <type>"
// (cs_arena.cpp HandleArenaCreateCommand). The emblem values
// (backgroundColor 4293102085, emblemStyle 101, emblemColor 4293253939,
// borderStyle 4, borderColor 4284049911) are the C++-exact ArenaTeam::Create
// call constants; the rating starts at the CONFIG_ARENA_START_RATING default
// of 0 (World.cpp:1217).
func (s *session) handleCmdArenaCreate(ctx context.Context, args []string) {
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	toks := splitQuotedArgs(args)
	var captainGUID uint64
	var captainName string
	rest := toks
	// Optional<PlayerIdentifier> captain: the first token is the captain only
	// when it resolves to a character; otherwise it is the team name.
	if len(toks) > 0 && !strings.HasPrefix(toks[0], "\"") && len(toks) >= 3 {
		if guid, name, ok := s.resolveArenaCaptain(ctx, cdb, toks[0]); ok {
			captainGUID, captainName = guid, name
			rest = toks[1:]
		}
	}
	if len(rest) < 2 {
		s.sendSysMessage("Syntax: .arena create [<captain>] \"<name>\" <2|3|5>")
		return
	}
	name := unquoteCommandToken(rest[0])
	teamType, ok := parseArenaTeamType(rest[1])
	if !ok {
		s.sendSysMessage("Invalid arena team type. Use 2, 3 or 5.")
		return
	}
	if captainGUID == 0 {
		guid, name, ok := s.arenaTargetOrSelf(ctx, cdb)
		if !ok {
			s.sendSysMessage("No character selected.")
			return
		}
		captainGUID, captainName = guid, name
	}
	if name == "" {
		s.sendSysMessage("Syntax: .arena create [<captain>] \"<name>\" <2|3|5>")
		return
	}
	// LANG_ARENA_ERROR_NAME_EXISTS (858).
	var clash uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", name).Scan(&clash); err == nil {
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" already exists.", name))
		return
	}
	// sCharacterCache->GetCharacterArenaTeamIdByGuid(captain, type):
	// LANG_ARENA_ERROR_SIZE (859).
	var existingTeam uint32
	if err := cdb.QueryRowContext(ctx, "SELECT m.arenaTeamId FROM arena_team_member AS m JOIN arena_team AS t ON t.arenaTeamId = m.arenaTeamId WHERE m.guid = ? AND t.type = ?", captainGUID, teamType).Scan(&existingTeam); err == nil {
		s.sendSysMessage(fmt.Sprintf("%s already has an arena team of the same type.", captainName))
		return
	}
	// ArenaTeamMgr::GenerateArenaTeamId: NextArenaTeamId++.
	var teamID uint32
	if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(arenaTeamId), 0) + 1 FROM arena_team").Scan(&teamID); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	// ArenaTeam::Create + CHAR_INS_ARENA_TEAM.
	if _, err := cdb.ExecContext(ctx, "INSERT INTO arena_team (arenaTeamId, name, captainGuid, type, rating, backgroundColor, emblemStyle, emblemColor, borderStyle, borderColor) VALUES (?, ?, ?, ?, 0, 4293102085, 101, 4293253939, 4, 4284049911)", teamID, name, captainGUID, teamType); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	// ArenaTeam::AddMember(captain): Player::RemovePetitionsAndSigns with the
	// arena charter type, which equals the team type (SharedDefines.h:3790).
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE playerguid = ? AND type = ?", captainGUID, teamType)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition_sign WHERE ownerguid = ? AND type = ?", captainGUID, teamType)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM petition WHERE ownerguid = ? AND type = ?", captainGUID, teamType)
	// CHAR_INS_ARENA_TEAM_MEMBER; personal rating starts at the
	// CONFIG_ARENA_START_PERSONAL_RATING default of 1000 (World.cpp:1218).
	_, _ = cdb.ExecContext(ctx, "INSERT INTO arena_team_member (arenaTeamId, guid, weekGames, weekWins, seasonGames, seasonWins, personalRating) VALUES (?, ?, 0, 0, 0, 0, 1000)", teamID, captainGUID)
	// LANG_ARENA_CREATE (864).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" created with id %d (type %dv%d, captain guid %d).", name, teamID, teamType, teamType, captainGUID))
	s.debug("arena team created", "id", teamID, "name", name, "captain", captainName)
}

// handleCmdArenaDisband processes ".arena disband <teamId>"
// (cs_arena.cpp HandleArenaDisbandCommand), mirroring the parameterless
// ArenaTeam::Disband: per-member cleanup followed by CHAR_DEL_ARENA_TEAM and
// CHAR_DEL_ARENA_TEAM_MEMBERS.
func (s *session) handleCmdArenaDisband(ctx context.Context, args []string) {
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .arena disband <teamId>")
		return
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid team ID.")
		return
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	teamID := uint32(id)
	var name string
	if err := cdb.QueryRowContext(ctx, "SELECT name FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&name); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM arena_team_member WHERE arenaTeamId = ?", teamID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM arena_team WHERE arenaTeamId = ?", teamID)
	// LANG_ARENA_DISBAND (865).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d) disbanded.", name, teamID))
	s.debug("arena team disbanded", "id", teamID, "name", name)
}

// handleCmdArenaRename processes ".arena rename <oldName> <newName>"
// (cs_arena.cpp HandleArenaRenameCommand), mirroring ArenaTeam::SetName: the
// rename fails (LANG_BAD_VALUE) when the name is unchanged, empty, or longer
// than 24 characters. The reserved-name and charter-name validations have no
// Go bridge and are documented as a gap.
func (s *session) handleCmdArenaRename(ctx context.Context, args []string) {
	toks := splitQuotedArgs(args)
	if len(toks) < 2 {
		s.sendSysMessage("Syntax: .arena rename \"<oldName>\" \"<newName>\"")
		return
	}
	oldName, newName := unquoteCommandToken(toks[0]), unquoteCommandToken(toks[1])
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	var teamID uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", oldName).Scan(&teamID); err != nil {
		// LANG_ARENA_ERROR_NAME_NOT_FOUND (861).
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" not found.", oldName))
		return
	}
	var clash uint32
	if err := cdb.QueryRowContext(ctx, "SELECT arenaTeamId FROM arena_team WHERE name = ?", newName).Scan(&clash); err == nil {
		// LANG_ARENA_ERROR_NAME_EXISTS (858).
		s.sendSysMessage(fmt.Sprintf("Arena team with name \"%s\" already exists.", newName))
		return
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return
	}
	if newName == "" || newName == oldName || len([]rune(newName)) > 24 {
		s.sendSysMessage("Invalid value.")
		return
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE arena_team SET name = ? WHERE arenaTeamId = ?", newName, teamID); err != nil {
		s.sendSysMessage("Invalid value.")
		return
	}
	// LANG_ARENA_RENAME (866).
	s.sendSysMessage(fmt.Sprintf("Arena team %d renamed from \"%s\" to \"%s\".", teamID, oldName, newName))
	s.debug("arena team renamed", "id", teamID, "from", oldName, "to", newName)
}

// handleCmdArenaCaptain processes ".arena captain <teamId> [<player>]"
// (cs_arena.cpp HandleArenaCaptainCommand), mirroring ArenaTeam::SetCaptain
// (CHAR_UPD_ARENA_TEAM_CAPTAIN).
func (s *session) handleCmdArenaCaptain(ctx context.Context, args []string) {
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .arena captain <teamId> [<player>]")
		return
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid team ID.")
		return
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	teamID := uint32(id)
	var teamName string
	var captainGUID uint64
	if err := cdb.QueryRowContext(ctx, "SELECT name, captainGuid FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&teamName, &captainGUID); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return
	}
	if s.arenaTeamIsFighting(ctx, cdb, teamID) {
		// LANG_ARENA_ERROR_COMBAT (860).
		s.sendSysMessage("Arena team is in combat.")
		return
	}
	var targetGUID uint64
	var targetName string
	if len(args) > 1 {
		guid, name, ok := s.resolveArenaCaptain(ctx, cdb, args[1])
		if !ok {
			s.sendSysMessage("Character not found.")
			return
		}
		targetGUID, targetName = guid, name
	} else {
		guid, name, ok := s.arenaTargetOrSelf(ctx, cdb)
		if !ok {
			s.sendSysMessage("No character selected.")
			return
		}
		targetGUID, targetName = guid, name
	}
	var isMember uint32
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM arena_team_member WHERE arenaTeamId = ? AND guid = ?", teamID, targetGUID).Scan(&isMember); err != nil || isMember == 0 {
		// LANG_ARENA_ERROR_NOT_MEMBER (862).
		s.sendSysMessage(fmt.Sprintf("%s is not a member of arena team \"%s\".", targetName, teamName))
		return
	}
	if captainGUID == targetGUID {
		// LANG_ARENA_ERROR_CAPTAIN (863).
		s.sendSysMessage(fmt.Sprintf("%s is already the captain of arena team \"%s\".", targetName, teamName))
		return
	}
	oldCaptainName := "<unknown>"
	_ = cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", captainGUID).Scan(&oldCaptainName)
	if _, err := cdb.ExecContext(ctx, "UPDATE arena_team SET captainGuid = ? WHERE arenaTeamId = ?", targetGUID, teamID); err != nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	// LANG_ARENA_CAPTAIN (867).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d): captain changed from %s to %s.", teamName, teamID, oldCaptainName, targetName))
	s.debug("arena team captain changed", "id", teamID, "from", oldCaptainName, "to", targetName)
}

// handleCmdArenaInfo processes ".arena info <teamId>"
// (cs_arena.cpp HandleArenaInfoCommand): header plus one line per member,
// with the "- Captain" marker on the captain's line.
func (s *session) handleCmdArenaInfo(ctx context.Context, args []string) {
	if len(args) < 1 {
		s.sendSysMessage("Syntax: .arena info <teamId>")
		return
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid team ID.")
		return
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	teamID := uint32(id)
	var name string
	var rating uint32
	var teamType uint32
	var captainGUID uint64
	if err := cdb.QueryRowContext(ctx, "SELECT name, rating, type, captainGuid FROM arena_team WHERE arenaTeamId = ?", teamID).Scan(&name, &rating, &teamType, &captainGUID); err != nil {
		// LANG_ARENA_ERROR_NOT_FOUND (857).
		s.sendSysMessage(fmt.Sprintf("Arena team with id %d not found.", teamID))
		return
	}
	// LANG_ARENA_INFO_HEADER (868).
	s.sendSysMessage(fmt.Sprintf("Arena team \"%s\" (id %d): rating %d, type %dv%d.", name, teamID, rating, teamType, teamType))
	rows, err := cdb.QueryContext(ctx, "SELECT m.guid, COALESCE(c.name, ''), m.personalRating FROM arena_team_member AS m LEFT JOIN characters AS c ON c.guid = m.guid WHERE m.arenaTeamId = ?", teamID)
	if err != nil {
		return
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
}

// handleCmdArenaLookup processes ".arena lookup <name>"
// (cs_arena.cpp HandleArenaLookupCommand): case-insensitive substring match
// over all arena teams, like TrinityCore StringContainsStringI. The C++ loop
// only emits rows when handler->GetSession() is set; the Go command path is
// always a session, so rows are always emitted.
func (s *session) handleCmdArenaLookup(ctx context.Context, args []string) {
	needle := strings.Join(args, " ")
	if strings.TrimSpace(needle) == "" {
		s.sendSysMessage("Syntax: .arena lookup <name>")
		return
	}
	cdb := s.arenaCharactersDB()
	if cdb == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	rows, err := cdb.QueryContext(ctx, "SELECT arenaTeamId, name, type FROM arena_team")
	if err != nil {
		s.sendSysMessage("Unknown error.")
		return
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
}

// handleCmdBF dispatches ".bf" (cs_bf.cpp, AddSC_bf_commandscript):
// start|stop|switch|timer|enable, each gated by its RBAC_PERM_COMMAND_BF_*
// permission (RBAC.h:172-176).
func (s *session) handleCmdBF(ctx context.Context, args []string) {
	if s.server == nil {
		s.sendSysMessage("Unknown error.")
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .bf start|stop|switch|timer|enable <battleId> [time]")
		return
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
		s.sendSysMessage("Syntax: .bf start|stop|switch|timer|enable <battleId> [time]")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "start":
		s.handleCmdBFStart(ctx, rest)
	case "stop":
		s.handleCmdBFStop(ctx, rest)
	case "switch":
		s.handleCmdBFSwitch(ctx, rest)
	case "timer":
		s.handleCmdBFTimer(ctx, rest)
	case "enable":
		s.handleCmdBFEnable(ctx, rest)
	}
}

// bfBattleID parses the <battleId> argument. It mirrors
// BattlefieldMgr::GetBattlefieldByBattleId: WGBattleID (1,
// BATTLEFIELD_BATTLEID_WG, Battlefield.h:34) is the only registered battle id,
// so any other id fails the command exactly like the C++ null-battlefield
// return-false path.
func (s *session) bfBattleID(args []string, usage string) (uint32, bool) {
	if len(args) < 1 {
		s.sendSysMessage(usage)
		return 0, false
	}
	id, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil || uint32(id) != WGBattleID {
		s.sendSysMessage("Invalid battlefield ID.")
		return 0, false
	}
	return uint32(id), true
}

// handleCmdBFStart processes ".bf start <battleId>" (cs_bf.cpp
// HandleBattlefieldStart). Battlefield::StartBattle (Battlefield.cpp:316) is a
// no-op when the battle is already active, so the native StartWGBattle is
// skipped in that case; the defender team is preserved from current state.
func (s *session) handleCmdBFStart(ctx context.Context, args []string) {
	if _, ok := s.bfBattleID(args, "Syntax: .bf start <battleId>"); !ok {
		return
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
}

// handleCmdBFStop processes ".bf stop <battleId>" (cs_bf.cpp
// HandleBattlefieldEnd): EndBattle(true) — the defenders hold the fortress.
func (s *session) handleCmdBFStop(ctx context.Context, args []string) {
	if _, ok := s.bfBattleID(args, "Syntax: .bf stop <battleId>"); !ok {
		return
	}
	s.server.EndWGBattle(true)
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command stop used)")
}

// handleCmdBFSwitch processes ".bf switch <battleId>" (cs_bf.cpp
// HandleBattlefieldSwitch): EndBattle(false) — the attackers breached the
// vault and become the new defenders.
func (s *session) handleCmdBFSwitch(ctx context.Context, args []string) {
	if _, ok := s.bfBattleID(args, "Syntax: .bf switch <battleId>"); !ok {
		return
	}
	s.server.EndWGBattle(false)
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command switch used)")
}

// handleCmdBFTimer processes ".bf timer <battleId> <time>" (cs_bf.cpp
// HandleBattlefieldTimer): SetTimer(time * IN_MILLISECONDS) then
// SendInitWorldStatesToAll.
func (s *session) handleCmdBFTimer(ctx context.Context, args []string) {
	if _, ok := s.bfBattleID(args, "Syntax: .bf timer <battleId> <time>"); !ok {
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .bf timer <battleId> <time>")
		return
	}
	secs, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		s.sendSysMessage("Invalid time value.")
		return
	}
	wg := s.server.getOrCreateWGState()
	wg.mu.Lock()
	wg.Timer = time.Duration(secs) * time.Second
	wg.EndTime = time.Now().Add(wg.Timer)
	wg.mu.Unlock()
	s.server.broadcastWGInitWorldStates()
	s.server.sendGlobalGMMessage(ctx, "Wintergrasp (Command timer used)")
}

// handleCmdBFEnable processes ".bf enable <battleId>" (cs_bf.cpp
// HandleBattlefieldEnable): toggles Battlefield::m_IsEnabled
// (ToggleBattlefield), mirrored on wgBattlegroundState.Enabled; the Update and
// player-enter-zone paths consult it (BattlefieldMgr.cpp:100, 130).
func (s *session) handleCmdBFEnable(ctx context.Context, args []string) {
	if _, ok := s.bfBattleID(args, "Syntax: .bf enable <battleId>"); !ok {
		return
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
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if !sess.commandAllowed(ctx, permissionReceiveGlobalGMTextMessage) {
			continue
		}
		sess.sendSysMessage(msg)
	}
}

func (s *session) handleCmdNPC(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .npc add <entry> | .npc info | .npc say <text> | .npc yell <text>")
		return
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "info":
		s.sendSysMessage(fmt.Sprintf("Target selection: %d", s.selection))
	case "say":
		if len(args) > 1 {
			msg := strings.Join(args[1:], " ")
			s.server.broadcastChat(s, nil, chatSay, 0, msg, "")
		}
	case "yell":
		if len(args) > 1 {
			msg := strings.Join(args[1:], " ")
			s.server.broadcastChat(s, nil, chatYell, 0, msg, "")
		}
	default:
		s.sendSysMessage(fmt.Sprintf("NPC command %s accepted.", sub))
	}
}

func (s *session) handleCmdGObject(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .gob add <entry> | .gob target | .gob near")
		return
	}
	s.sendSysMessage(fmt.Sprintf("GameObject command %s accepted.", args[0]))
}

func (s *session) handleCmdRevive(ctx context.Context, args []string) {
	if s.player == nil {
		return
	}
	s.resurrectPlayer(ctx, 1.0)
	s.sendSysMessage("You have been revived.")
}

func (s *session) handleCmdDismount(ctx context.Context) {
	if s.player == nil {
		return
	}
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted {
			s.removeAura(aura.SpellID)
		}
	}
	s.player.MountDisplayID = 0
	s.sendPlayerUpdate()
	s.sendSysMessage("You have dismounted.")
}

// persistExtraFlags writes extra_flags immediately so GM mode survives
// restarts the way TrinityCore's SaveToDB round-trip does.
func (s *session) persistExtraFlags() {
	if s.player == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.Exec("UPDATE characters SET extra_flags = ? WHERE guid = ?", s.player.ExtraFlags, s.playerGUID)
}

func (s *session) handleCmdSave(ctx context.Context) {
	if s.player == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, err := s.server.CharactersStore.DB.ExecContext(ctx,
		"UPDATE characters SET position_x = ?, position_y = ?, position_z = ?, orientation = ?, map = ?, zone = ?, health = ?, money = ?, playerFlags = ?, equipmentCache = ?, extra_flags = ? WHERE guid = ?",
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation, s.player.Map, s.player.Zone, s.player.Health, s.player.Money, s.player.PlayerFlags, s.player.Equipment, s.player.ExtraFlags, s.playerGUID)
	if err != nil {
		s.sendSysMessage("Failed to save character: " + err.Error())
		return
	}
	s.sendSysMessage("Player character saved.")
}

// commandNode mirrors TrinityCore ChatCommandNode resolution: tokens are
// matched case-insensitively against children by unique prefix, aliases
// cover names that are not prefixes, and ambiguous tokens list candidates.
type commandNode struct {
	name     string
	children map[string]*commandNode
	aliases  map[string]string // alias -> canonical child name
	invoke   func(ctx context.Context, args []string) bool
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

// commandTokens holds the canonicalized tokens once resolution completes.
type commandTokens []string

// resolve walks the tree token by token, rewriting each token to the
// canonical child name it matched (prefix or alias). Returns the deepest
// node and the rewritten tokens; consumed counts matched structural tokens.
func (n *commandNode) resolve(tokens []string) (*commandNode, commandTokens, int, []string, int) {
	node := n
	rewritten := make(commandTokens, 0, len(tokens))
	consumed := 0
	nodeDepth := 0
	// Once an invoke-bearing node is reached, remaining tokens are its
	// handler's parameters: they are still canonicalized against its
	// children (so 'spee' -> 'speed') but do not descend further.
	for _, token := range tokens {
		lower := strings.ToLower(token)
		matches := make([]string, 0, 2)
		for name := range node.children {
			if strings.HasPrefix(name, lower) {
				matches = append(matches, name)
			}
		}
		if target, ok := node.aliases[lower]; ok {
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
		if node.invoke != nil {
			// Parameter canonicalization for the already-selected handler.
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
// .baninfo ip / .unban ip handlers (documented deviation: net.ParseIP also
// accepts IPv6, which the C++ IPv4-only check rejects).
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
	if s.server == nil || target == nil {
		return
	}
	s.server.sessionsMu.Lock()
	delete(s.server.sessions, target)
	s.server.sessionsMu.Unlock()
	if target.conn != nil {
		_ = target.conn.Close()
	}
}

// kickAccountSessions mirrors the World::BanAccount kick loop
// (World.cpp:2866-2871): disconnects every session of the account except
// the author's own session.
func (s *session) kickAccountSessions(accountID uint32, author string) {
	if s.server == nil {
		return
	}
	var victims []*session
	s.server.sessionsMu.RLock()
	for sess := range s.server.sessions {
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
	s.server.sessionsMu.RUnlock()
	for _, v := range victims {
		s.kickSession(v)
	}
}

// worldBanAccount mirrors World::BanAccount (World.cpp:2785): refuses an
// already-banned account, inserts the ban rows, and kicks affected sessions
// (for IP, every session on that IP). An IP ban succeeds even when nobody is
// affected yet.
func (s *session) worldBanAccount(ctx context.Context, mode int, nameOrIP, durationStr, reason, author string) banResult {
	if s.server == nil || s.server.AuthStore == nil || s.server.CharactersStore == nil {
		return banSyntaxError
	}
	auth := s.server.AuthStore
	durationSecs := timeStringToSecs(durationStr)
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
			row, qerr := s.server.CharactersStore.QueryRowStatement(ctx, "CHAR_SEL_ACCOUNT_BY_NAME", nameOrIP)
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
		tx, err := auth.DB.BeginTx(ctx, nil)
		if err != nil {
			return banSyntaxError
		}
		for _, account := range accounts {
			// LOGIN_UPD_ACCOUNT_NOT_BANNED: make sure there is only one
			// active ban.
			if _, err := auth.ExecStatementTx(ctx, tx, "LOGIN_UPD_ACCOUNT_NOT_BANNED", account); err != nil {
				tx.Rollback()
				return banSyntaxError
			}
			// LOGIN_INS_ACCOUNT_BANNED.
			if _, err := auth.ExecStatementTx(ctx, tx, "LOGIN_INS_ACCOUNT_BANNED", account, durationSecs, author, reason); err != nil {
				tx.Rollback()
				return banSyntaxError
			}
		}
		if err := tx.Commit(); err != nil {
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
	tx, err := chars.DB.BeginTx(ctx, nil)
	if err != nil {
		return banSyntaxError
	}
	// CHAR_UPD_CHARACTER_BAN + CHAR_INS_CHARACTER_BAN.
	if _, err := chars.ExecStatementTx(ctx, tx, "CHAR_UPD_CHARACTER_BAN", guid); err != nil {
		tx.Rollback()
		return banSyntaxError
	}
	if _, err := chars.ExecStatementTx(ctx, tx, "CHAR_INS_CHARACTER_BAN", guid, durationSecs, author, reason); err != nil {
		tx.Rollback()
		return banSyntaxError
	}
	if err := tx.Commit(); err != nil {
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
func (s *session) handleCmdBan(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .ban account|character|playeraccount|ip ...")
		return
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
		s.sendSysMessage("Syntax: .ban account|character|playeraccount|ip ...")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "account":
		s.handleBanHelper(ctx, banModeAccount, rest)
	case "character":
		s.handleCmdBanCharacter(ctx, rest)
	case "playeraccount":
		s.handleBanHelper(ctx, banModeCharacter, rest)
	case "ip":
		s.handleBanHelper(ctx, banModeIP, rest)
	}
}

// handleBanHelper mirrors HandleBanHelper (cs_ban.cpp:183): parses
// "<name|ip> <duration> <reason...>", validates per mode, and applies
// World::BanAccount, reporting BAN_SUCCESS / BAN_NOTFOUND / BAN_EXISTS.
func (s *session) handleBanHelper(ctx context.Context, mode int, args []string) {
	syntax := "Syntax: .ban account|playeraccount <name> <duration> <reason>"
	if mode == banModeIP {
		syntax = "Syntax: .ban ip <ip> <duration> <reason>"
	}
	if len(args) < 3 {
		s.sendSysMessage(syntax)
		return
	}
	nameOrIP := args[0]
	durationStr := args[1]
	reason := strings.Join(args[2:], " ")
	// C++: if (!durationStr || !atoi(durationStr)) return false.
	if cAtoi(durationStr) == 0 || reason == "" {
		s.sendSysMessage(syntax)
		return
	}
	switch mode {
	case banModeAccount:
		nameOrIP = upperOnlyLatin(nameOrIP)
	case banModeCharacter:
		if nameOrIP == "" || !utf8.ValidString(nameOrIP) {
			// LANG_PLAYER_NOT_FOUND (499).
			s.sendSysMessage("Player not found.")
			return
		}
		nameOrIP = normalizePlayerName(nameOrIP)
	case banModeIP:
		if !isBanIPAddress(nameOrIP) {
			return // C++ returns false silently
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
		s.sendSysMessage(syntax)
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
}

// handleCmdBanCharacter processes ".ban character <name> <duration> <reason>"
// (cs_ban.cpp HandleBanCharacterCommand).
func (s *session) handleCmdBanCharacter(ctx context.Context, args []string) {
	if len(args) < 3 {
		s.sendSysMessage("Syntax: .ban character <name> <duration> <reason>")
		return
	}
	name := args[0]
	durationStr := args[1]
	reason := strings.Join(args[2:], " ")
	if cAtoi(durationStr) == 0 || reason == "" {
		s.sendSysMessage("Syntax: .ban character <name> <duration> <reason>")
		return
	}
	if name == "" || !utf8.ValidString(name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return
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
		s.sendSysMessage("Syntax: .ban character <name> <duration> <reason>")
	}
}

// handleCmdBanInfo dispatches ".baninfo account|character|ip"
// (cs_ban.cpp baninfoCommandTable).
func (s *session) handleCmdBanInfo(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .baninfo account|character|ip ...")
		return
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
		s.sendSysMessage("Syntax: .baninfo account|character|ip ...")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "account":
		s.handleCmdBanInfoAccount(ctx, rest)
	case "character":
		s.handleCmdBanInfoCharacter(ctx, rest)
	case "ip":
		s.handleCmdBanInfoIP(ctx, rest)
	}
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
func (s *session) handleCmdBanInfoAccount(ctx context.Context, args []string) {
	if s.server == nil || s.server.AuthStore == nil {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .baninfo account <name>")
		return
	}
	// C++ takes the whole remainder as the account name.
	accountName := upperOnlyLatin(strings.Join(args, " "))
	var accountID uint32
	row, err := s.server.AuthStore.QueryRowStatement(ctx, "LOGIN_SEL_ACCOUNT_ID_BY_NAME", accountName)
	if err != nil || row.Scan(&accountID) != nil || accountID == 0 {
		// LANG_ACCOUNT_NOT_EXIST (413).
		s.sendSysMessage(fmt.Sprintf("Account %s does not exist.", accountName))
		return
	}
	// C++ raw query: SELECT FROM_UNIXTIME(bandate), unbandate-bandate,
	// active, unbandate, banreason, bannedby FROM account_banned
	// WHERE id = ? ORDER BY bandate ASC (FROM_UNIXTIME is MySQL-only; the
	// timestamp is formatted in Go as local time instead).
	rows, err := s.server.AuthStore.DB.QueryContext(ctx, "SELECT bandate, unbandate-bandate, active, unbandate, banreason, bannedby FROM account_banned WHERE id = ? ORDER BY bandate ASC", accountID)
	if err != nil {
		return
	}
	entries, err := collectBanHistory(rows)
	rows.Close()
	if err != nil {
		return
	}
	if len(entries) == 0 {
		// LANG_BANINFO_NOACCOUNTBAN (416).
		s.sendSysMessage(fmt.Sprintf("No ban history for account %s.", accountName))
		return
	}
	s.sendBanHistory(accountName, entries, "2006-01-02 15:04:05")
}

// handleCmdBanInfoCharacter processes ".baninfo character <name>"
// (cs_ban.cpp HandleBanInfoCharacterCommand).
func (s *session) handleCmdBanInfoCharacter(ctx context.Context, args []string) {
	if s.server == nil || s.server.CharactersStore == nil {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .baninfo character <name>")
		return
	}
	name := args[0]
	if name == "" || !utf8.ValidString(name) {
		// LANG_BANINFO_NOCHARACTER (414).
		s.sendSysMessage("Character not found.")
		return
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
			return
		}
	}
	// CHAR_SEL_BANINFO.
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_BANINFO", guid)
	if err != nil {
		return
	}
	entries, err := collectBanHistory(rows)
	rows.Close()
	if err != nil {
		return
	}
	if len(entries) == 0 {
		// LANG_CHAR_NOT_BANNED (1136).
		s.sendSysMessage(fmt.Sprintf("%s is not banned.", name))
		return
	}
	s.sendBanHistory(name, entries, "2006-01-02_15-04-05")
}

// handleCmdBanInfoIP processes ".baninfo ip <ip>"
// (cs_ban.cpp HandleBanInfoIPCommand).
func (s *session) handleCmdBanInfoIP(ctx context.Context, args []string) {
	if s.server == nil || s.server.AuthStore == nil {
		return
	}
	if len(args) == 0 || !isBanIPAddress(args[0]) {
		return // C++ returns false silently
	}
	ip := args[0]
	// C++ raw query: SELECT ip, FROM_UNIXTIME(bandate),
	// FROM_UNIXTIME(unbandate), unbandate-UNIX_TIMESTAMP(), banreason,
	// bannedby, unbandate-bandate FROM ip_banned WHERE ip = ?.
	var bandate, unbandate int64
	var reason, by string
	err := s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT bandate, unbandate, banreason, bannedby FROM ip_banned WHERE ip = ?", ip).Scan(&bandate, &unbandate, &reason, &by)
	if err != nil {
		// LANG_BANINFO_NOIP (415).
		s.sendSysMessage("IP address is not banned.")
		return
	}
	permanent := unbandate == bandate
	unbanStr := "Never"   // LANG_BANINFO_NEVER (420)
	banTime := "Infinite" // LANG_BANINFO_INFINITE (419)
	if !permanent {
		unbanStr = time.Unix(unbandate, 0).Format("2006-01-02 15:04:05")
		remaining := unbandate - timeNow()
		if remaining < 0 {
			remaining = 0
		}
		banTime = secsToTimeStringShort(uint64(remaining))
	}
	// LANG_BANINFO_IPENTRY (423).
	s.sendSysMessage(fmt.Sprintf("%s | banned: %s | unban: %s | duration: %s | %s | by %s",
		ip, time.Unix(bandate, 0).Format("2006-01-02 15:04:05"), unbanStr, banTime, reason, by))
}

// handleCmdBanList dispatches ".banlist account|character|ip [filter]"
// (cs_ban.cpp banlistCommandTable). The Go command path is always a
// session, so only the C++ GetSession() short-output branches are ported
// (same treatment as the arena lookup port).
func (s *session) handleCmdBanList(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .banlist account|character|ip [filter]")
		return
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
		s.sendSysMessage("Syntax: .banlist account|character|ip [filter]")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "account":
		s.handleCmdBanListAccount(ctx, rest)
	case "character":
		s.handleCmdBanListCharacter(ctx, rest)
	case "ip":
		s.handleCmdBanListIP(ctx, rest)
	}
}

// handleCmdBanListAccount processes ".banlist account [filter]"
// (cs_ban.cpp HandleBanListAccountCommand).
func (s *session) handleCmdBanListAccount(ctx context.Context, args []string) {
	if s.server == nil || s.server.AuthStore == nil {
		return
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
		return
	}
	var ids []uint32
	for rows.Next() {
		var id uint32
		var uname string
		if err := rows.Scan(&id, &uname); err != nil {
			rows.Close()
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		// LANG_BANLIST_NOACCOUNT (425).
		s.sendSysMessage("No banned accounts found.")
		return
	}
	// LANG_BANLIST_MATCHINGACCOUNT (428).
	s.sendSysMessage("Matching banned accounts:")
	for _, id := range ids {
		var uname string
		if err := auth.DB.QueryRowContext(ctx, "SELECT account.username FROM account, account_banned WHERE account_banned.id = ? AND account_banned.id = account.id", id).Scan(&uname); err == nil {
			s.sendSysMessage(uname)
		}
	}
}

// handleCmdBanListCharacter processes ".banlist character <filter>"
// (cs_ban.cpp HandleBanListCharacterCommand).
func (s *session) handleCmdBanListCharacter(ctx context.Context, args []string) {
	if s.server == nil || s.server.CharactersStore == nil {
		return
	}
	chars := s.server.CharactersStore
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .banlist character <filter>")
		return
	}
	// CHAR_SEL_GUID_BY_NAME_FILTER.
	rows, err := chars.QueryStatement(ctx, "CHAR_SEL_GUID_BY_NAME_FILTER", args[0])
	if err != nil {
		return
	}
	var guids []uint64
	for rows.Next() {
		var guid uint64
		var name string
		if err := rows.Scan(&guid, &name); err != nil {
			rows.Close()
			return
		}
		guids = append(guids, guid)
	}
	rows.Close()
	if len(guids) == 0 {
		// LANG_BANLIST_NOCHARACTER (426).
		s.sendSysMessage("No banned characters found.")
		return
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
}

// handleCmdBanListIP processes ".banlist ip [filter]"
// (cs_ban.cpp HandleBanListIPCommand).
func (s *session) handleCmdBanListIP(ctx context.Context, args []string) {
	if s.server == nil || s.server.AuthStore == nil {
		return
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
		return
	}
	var ips []string
	for rows.Next() {
		var ip string
		var bandate, unbandate int64
		var by, reason string
		if err := rows.Scan(&ip, &bandate, &unbandate, &by, &reason); err != nil {
			rows.Close()
			return
		}
		ips = append(ips, ip)
	}
	rows.Close()
	if len(ips) == 0 {
		// LANG_BANLIST_NOIP (424).
		s.sendSysMessage("No banned IPs found.")
		return
	}
	// LANG_BANLIST_MATCHINGIP (427).
	s.sendSysMessage("Matching banned IPs:")
	for _, ip := range ips {
		s.sendSysMessage(ip)
	}
}

// handleCmdUnBan dispatches ".unban account|character|playeraccount|ip"
// (cs_ban.cpp unbanCommandTable).
func (s *session) handleCmdUnBan(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .unban account|character|playeraccount|ip ...")
		return
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
		s.sendSysMessage("Syntax: .unban account|character|playeraccount|ip ...")
		return
	}
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return
	}
	switch sub {
	case "account":
		s.handleUnBanHelper(ctx, banModeAccount, rest)
	case "character":
		s.handleCmdUnBanCharacter(ctx, rest)
	case "playeraccount":
		s.handleUnBanHelper(ctx, banModeCharacter, rest)
	case "ip":
		s.handleUnBanHelper(ctx, banModeIP, rest)
	}
}

// handleUnBanHelper mirrors HandleUnBanHelper (cs_ban.cpp:683).
func (s *session) handleUnBanHelper(ctx context.Context, mode int, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .unban account|playeraccount|ip <name|ip>")
		return
	}
	nameOrIP := args[0]
	switch mode {
	case banModeAccount:
		nameOrIP = upperOnlyLatin(nameOrIP)
	case banModeCharacter:
		if nameOrIP == "" || !utf8.ValidString(nameOrIP) {
			// LANG_PLAYER_NOT_FOUND (499).
			s.sendSysMessage("Player not found.")
			return
		}
		nameOrIP = normalizePlayerName(nameOrIP)
	case banModeIP:
		if !isBanIPAddress(nameOrIP) {
			return // C++ returns false silently
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
}

// handleCmdUnBanCharacter processes ".unban character <name>"
// (cs_ban.cpp HandleUnBanCharacterCommand).
func (s *session) handleCmdUnBanCharacter(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .unban character <name>")
		return
	}
	name := args[0]
	if name == "" || !utf8.ValidString(name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return
	}
	name = normalizePlayerName(name)
	// World::RemoveBanCharacter (World.cpp:2947); no success message in C++.
	if !s.worldRemoveBanCharacter(ctx, name) {
		// LANG_PLAYER_NOT_FOUND (499).
		s.sendSysMessage("Player not found.")
		return
	}
}

// buildCommandTree assembles the command hierarchy with the canonical
// TrinityCore names; aliases mark non-prefix spellings.
func (s *session) buildCommandTree() *commandNode {
	root := &commandNode{name: "", children: make(map[string]*commandNode)}
	root.add("help", func(ctx context.Context, args []string) bool { s.handleCmdHelp(args); return true }, nil, map[string]string{"?": "help"})
	root.add("gm", func(ctx context.Context, args []string) bool { s.handleCmdGM(args); return true }, []string{"on", "off", "chat", "fly", "visible"}, map[string]string{"vis": "visible"})
	root.add("cheat", func(ctx context.Context, args []string) bool { return s.handleCmdCheat(ctx, args) }, []string{"explore"}, nil)
	root.add("tele", func(ctx context.Context, args []string) bool { s.handleCmdTele(ctx, args); return true }, nil, nil)
	root.add("go", func(ctx context.Context, args []string) bool { s.handleCmdGo(ctx, args); return true }, nil, nil)
	root.add("modify", func(ctx context.Context, args []string) bool { s.handleCmdModify(ctx, args); return true }, []string{"hp", "health", "mana", "power", "speed", "run", "fly", "scale", "money", "gold", "level"}, map[string]string{"mod": "modify"})
	root.add("additem", func(ctx context.Context, args []string) bool { s.handleCmdAddItem(ctx, args); return true }, nil, map[string]string{"item": "additem"})
	root.add("learn", func(ctx context.Context, args []string) bool { s.handleCmdLearn(ctx, args); return true }, nil, nil)
	root.add("unlearn", func(ctx context.Context, args []string) bool { s.handleCmdUnlearn(ctx, args); return true }, nil, nil)
	root.add("cast", func(ctx context.Context, args []string) bool { s.handleCmdCast(ctx, args); return true }, nil, nil)
	root.add("lookup", func(ctx context.Context, args []string) bool { s.handleCmdLookup(ctx, args); return true }, []string{"item", "spell", "creature", "npc", "tele", "quest"}, nil)
	root.add("server", func(ctx context.Context, args []string) bool { s.handleCmdServer(ctx, args); return true }, []string{"info", "motd", "restart", "shutdown"}, nil)
	root.add("character", func(ctx context.Context, args []string) bool { s.handleCmdCharacter(ctx, args); return true }, []string{"level", "rename", "customize", "changefaction", "changerace"}, map[string]string{"char": "character"})
	root.add("account", func(ctx context.Context, args []string) bool { s.handleCmdAccount(ctx, args); return true }, []string{"set", "password", "addon", "email", "lock"}, map[string]string{"acct": "account"})
	root.add("achievement", func(ctx context.Context, args []string) bool { s.handleCmdAchievement(ctx, args); return true }, []string{"add"}, nil)
	root.add("arena", func(ctx context.Context, args []string) bool { s.handleCmdArena(ctx, args); return true }, []string{"create", "disband", "rename", "captain", "info", "lookup"}, nil)
	root.add("ban", func(ctx context.Context, args []string) bool { s.handleCmdBan(ctx, args); return true }, []string{"account", "character", "playeraccount", "ip"}, nil)
	root.add("baninfo", func(ctx context.Context, args []string) bool { s.handleCmdBanInfo(ctx, args); return true }, []string{"account", "character", "ip"}, nil)
	root.add("banlist", func(ctx context.Context, args []string) bool { s.handleCmdBanList(ctx, args); return true }, []string{"account", "character", "ip"}, nil)
	root.add("unban", func(ctx context.Context, args []string) bool { s.handleCmdUnBan(ctx, args); return true }, []string{"account", "character", "playeraccount", "ip"}, nil)
	root.add("bf", func(ctx context.Context, args []string) bool { s.handleCmdBF(ctx, args); return true }, []string{"start", "stop", "switch", "timer", "enable"}, nil)
	root.add("npc", func(ctx context.Context, args []string) bool { s.handleCmdNPC(ctx, args); return true }, []string{"info", "say", "yell"}, nil)
	root.add("gobject", func(ctx context.Context, args []string) bool { s.handleCmdGObject(ctx, args); return true }, nil, map[string]string{"gob": "gobject"})
	root.add("revive", func(ctx context.Context, args []string) bool { s.handleCmdRevive(ctx, args); return true }, nil, map[string]string{"res": "revive", "rev": "revive"})
	root.add("dismount", func(ctx context.Context, args []string) bool { s.handleCmdDismount(ctx); return true }, nil, nil)
	root.add("save", func(ctx context.Context, args []string) bool { s.handleCmdSave(ctx); return true }, nil, map[string]string{"saveall": "save"})
	return root
}

// dispatchCommand resolves a partial command like '.mod spee 10' to its
// canonical invocation ('.modify speed 10') like ChatCommandNode::
// TryExecuteCommand, reporting ambiguous matches with the TC wording.
func (s *session) dispatchCommand(ctx context.Context, fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	root := s.buildCommandTree()
	node, rewritten, _, ambiguous, nodeDepth := root.resolve(fields)
	if ambiguous != nil {
		s.sendSysMessage("There are multiple commands matching '" + strings.ToLower(fields[len(rewritten)]) + "'. Did you mean:")
		for _, candidate := range ambiguous {
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
	return node.invoke(ctx, args)
}

// handleSetFactionCheat processes CMSG_SET_FACTION_CHEAT (0x126).
// Reference: WorldSession::HandleSetFactionCheat (CharacterHandler.cpp:1043).
func (s *session) handleSetFactionCheat(ctx context.Context, payload []byte) bool {
	s.debug("handleSetFactionCheat received", "account", s.accountName)
	if s.playerLoaded && s.player != nil {
		_ = s.write(uint16(protocol.OpcodeSMSG_INITIALIZE_FACTIONS), buildInitialReputations(*s.player), true)
	}
	return true
}

// handleWorldTeleport processes CMSG_WORLD_TELEPORT (0x008).
// Reference: WorldSession::HandleWorldTeleportOpcode (MiscHandler.cpp:1070).
func (s *session) handleWorldTeleport(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 20 {
		return true
	}

	allowed := s.security >= 1 || (s.player.ExtraFlags&playerExtraGMOn != 0)
	if !allowed && s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		hasPerm, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionOpcodeWorldTeleport)
		if err == nil && hasPerm {
			allowed = true
		}
	}
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

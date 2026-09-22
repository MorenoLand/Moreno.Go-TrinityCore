package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

type loginStage struct {
	Name  string
	Match func(uint32) bool
}

func main() {
	tracePath := flag.String("trace", "", "recorded protocol trace JSONL")
	selfCheck := flag.Bool("self-check", false, "validate the login loading-order regression guard")
	flag.Parse()
	if *selfCheck {
		if err := runSelfCheck(); err != nil {
			fail(err.Error())
		}
		fmt.Println("login loading-order self-check passed")
		return
	}
	if *tracePath == "" {
		fail("-trace is required")
	}
	file, err := os.Open(*tracePath)
	if err != nil {
		fail(err.Error())
	}
	trace, err := protocoltrace.Load(file)
	file.Close()
	if err != nil {
		fail(err.Error())
	}
	loginOpcode := uint32(protocol.OpcodeCMSG_PLAYER_LOGIN)
	checked := 0
	for index, event := range trace.Events {
		if event.Direction != protocoltrace.ClientToServer || event.Opcode != loginOpcode {
			continue
		}
		if err := checkLogin(trace, index); err != nil {
			fail(fmt.Sprintf("login %d: %v", checked+1, err))
		}
		checked++
	}
	if checked == 0 {
		fail("trace contains no CMSG_PLAYER_LOGIN event")
	}
	fmt.Printf("login traces checked=%d order=create-update-gate-passed\n", checked)
}

func runSelfCheck() error {
	login := uint32(protocol.OpcodeCMSG_PLAYER_LOGIN)
	verify := uint32(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD)
	criteria := uint32(protocol.OpcodeSMSG_CRITERIA_UPDATE)
	bad := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: criteria}}}
	if err := rejectPreVerifyAchievementPackets(bad, 0); err == nil {
		return fmt.Errorf("pre-verify achievement packet was not rejected")
	}
	good := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}}}
	if err := rejectPreVerifyAchievementPackets(good, 0); err != nil {
		return fmt.Errorf("valid verify-world ordering was rejected: %w", err)
	}
	goodAchievement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: verify}, {Direction: protocoltrace.ServerToClient, Opcode: criteria}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED)}}}
	if err := rejectPreVerifyAchievementPackets(goodAchievement, 0); err != nil {
		return fmt.Errorf("post-verify achievement packet was rejected: %w", err)
	}
	validMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL)}}}
	if err := checkLoginMovementOrder(validMovement, 0); err != nil {
		return fmt.Errorf("valid movement ordering was rejected: %w", err)
	}
	invalidMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}}}
	if err := checkLoginMovementOrder(invalidMovement, 0); err == nil {
		return fmt.Errorf("out-of-order movement packets were not rejected")
	}
	preTimeSyncMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ)}}}
	if err := checkLoginMovementOrder(preTimeSyncMovement, 0); err == nil {
		return fmt.Errorf("movement packet before time sync was not rejected")
	}
	for _, compressed := range []bool{false, true} {
		event, err := loginCreateFixture(compressed)
		if err != nil {
			return fmt.Errorf("login create fixture build failed: %w", err)
		}
		if err := requireCreateBlock(event); err != nil {
			return fmt.Errorf("login create fixture rejected: %w", err)
		}
	}
	payloadChecks := []struct {
		name     string
		opcode   protocol.Opcode
		payload  []byte
		validate func(protocoltrace.Event) error
	}{
		{"verify-world", protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD, loginVerifyFixture(), requireLoginVerifyWorld},
		{"instance-difficulty", protocol.OpcodeSMSG_INSTANCE_DIFFICULTY, make([]byte, 8), requireEightBytePayload},
		{"initial-spells", protocol.OpcodeSMSG_INITIAL_SPELLS, []byte{0, 0, 0, 0, 0}, requireInitialSpells},
		{"unlearn-spells", protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS, []byte{0, 0, 0, 0}, requireUnlearnSpells},
		{"action-buttons", protocol.OpcodeSMSG_ACTION_BUTTONS, actionButtonsFixture(), requireActionButtons},
		{"factions", protocol.OpcodeSMSG_INITIALIZE_FACTIONS, initialFactionsFixture(), requireInitialFactions},
		{"dance-moves", protocol.OpcodeSMSG_LEARNED_DANCE_MOVES, make([]byte, 8), func(event protocoltrace.Event) error { return requirePayloadLength(event, 8) }},
		{"feature-status", protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS, make([]byte, 2), func(event protocoltrace.Event) error { return requirePayloadLength(event, 2) }},
		{"bind-point", protocol.OpcodeSMSG_BIND_POINT_UPDATE, make([]byte, 20), func(event protocoltrace.Event) error { return requirePayloadLength(event, 20) }},
		{"time-speed", protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED, loginTimeSpeedFixture(), requireLoginTimeSpeed},
		{"login-effect", protocol.OpcodeSMSG_SPELL_GO, loginEffectFixture(), requireLoginEffect},
		{"group-list", protocol.OpcodeSMSG_GROUP_LIST, groupListFixture(), requireGroupList},
		{"world-states", protocol.OpcodeSMSG_INIT_WORLD_STATES, initWorldStatesFixture(), requireInitWorldStates},
		{"forced-reactions", protocol.OpcodeSMSG_SET_FORCED_REACTIONS, make([]byte, 4), requireForcedReactions},
		{"resync-runes", protocol.OpcodeSMSG_RESYNC_RUNES, resyncRunesFixture(), requireResyncRunes},
		{"time-sync", protocol.OpcodeSMSG_TIME_SYNC_REQ, make([]byte, 4), requireTimeSyncRequest},
		{"aura-update-all", protocol.OpcodeSMSG_AURA_UPDATE_ALL, auraUpdateFixture(), requireAuraUpdateAll},
		{"item-time-update", protocol.OpcodeSMSG_ITEM_TIME_UPDATE, protocol.BuildItemTimeUpdate(0x4000000000000106, 1234), requirePayloadLengthExact(12)},
		{"item-enchant-time-update", protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE, protocol.BuildItemEnchantTimeUpdate(0x106, 0x4000000000000106, 2, 1234), requirePayloadLengthExact(24)},
		{"pet-spells", protocol.OpcodeSMSG_PET_SPELLS, petSpellsFixture(), requirePetSpells},
		{"quest-giver-details", protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS, questGiverDetailsFixture(), requireQuestGiverDetails},
		{"quest-status-multiple", protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE, make([]byte, 4), requireQuestStatusMultiple},
		{"taxi-node-status", protocol.OpcodeSMSG_TAXINODE_STATUS, make([]byte, 9), requirePayloadLengthExact(9)},
	}
	for _, check := range payloadChecks {
		event := protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: uint32(check.opcode), Payload: base64.StdEncoding.EncodeToString(check.payload)}
		if err := check.validate(event); err != nil {
			return fmt.Errorf("%s payload fixture rejected: %w", check.name, err)
		}
	}
	return nil
}

func loginVerifyFixture() []byte {
	buf := protocol.NewBuffer(20)
	buf.WriteI32(0)
	for range 4 {
		buf.WriteF32(0)
	}
	return buf.Bytes()
}

func loginTimeSpeedFixture() []byte {
	buf := protocol.NewBuffer(12)
	buf.WriteU32(0)
	buf.WriteF32(0.5)
	buf.WriteU32(0)
	return buf.Bytes()
}

func questGiverDetailsFixture() []byte {
	packet := protocol.NewBuffer(256)
	packet.WriteU64(1)
	packet.WriteU64(0)
	packet.WriteU32(100)
	packet.WriteCString("Quest")
	packet.WriteCString("Details")
	packet.WriteCString("Objectives")
	packet.WriteU8(1)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU8(0)
	packet.WriteU32(0)
	packet.WriteU32(0)
	for index := 0; index < 10+15; index++ {
		packet.WriteU32(0)
	}
	packet.WriteI32(0)
	return packet.Bytes()
}

func groupListFixture() []byte {
	packet := protocol.NewBuffer(64)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU64(uint64(0x1F50)<<48 | 1)
	packet.WriteU32(0)
	packet.WriteU32(1)
	packet.WriteCString("Noradinia")
	packet.WriteU64(126)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU64(127)
	packet.WriteU8(3)
	packet.WriteU64(0)
	packet.WriteU8(2)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	return packet.Bytes()
}

func loginEffectFixture() []byte {
	power := uint32(777)
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: 0x106}
	return protocol.BuildSpellGoWithPower(0x106, 0x106, 0, 836, 0x901, 123, []uint64{0x106}, nil, target, &power)
}

func initWorldStatesFixture() []byte {
	buf := protocol.NewBuffer(14)
	buf.WriteI32(0)
	buf.WriteI32(0)
	buf.WriteI32(0)
	buf.WriteU16(0)
	return buf.Bytes()
}

func resyncRunesFixture() []byte {
	buf := protocol.NewBuffer(16)
	buf.WriteU32(6)
	for range 6 {
		buf.WriteU8(0)
		buf.WriteU8(0)
	}
	return buf.Bytes()
}

func auraUpdateFixture() []byte {
	return protocol.BuildAuraUpdateAll(0x106, []protocol.AuraUpdateRecord{{CasterGUID: 0x106, Slot: 0, SpellID: 836, EffectMask: 0x01, Positive: true, MaxDurationMs: 1000, DurationMs: 500, CasterLevel: 10, StackCount: 2}})
}

func petSpellsFixture() []byte {
	buf := protocol.NewBuffer(64)
	buf.WriteU64(0xF140000000000106)
	buf.WriteU16(23)
	buf.WriteU32(0)
	buf.WriteU8(1)
	buf.WriteU8(1)
	buf.WriteU16(0)
	for range 10 {
		buf.WriteU32(0)
	}
	buf.WriteU8(2)
	buf.WriteU32(6307 | 0x81000000)
	buf.WriteU32(7799 | 0x81000000)
	buf.WriteU8(0)
	return buf.Bytes()
}

func actionButtonsFixture() []byte {
	buf := protocol.NewBuffer(1 + 144*4)
	buf.WriteU8(1)
	for range 144 {
		buf.WriteU32(0)
	}
	return buf.Bytes()
}

func initialFactionsFixture() []byte {
	buf := protocol.NewBuffer(4 + 128*5)
	buf.WriteU32(128)
	for range 128 {
		buf.WriteU8(0)
		buf.WriteU32(0)
	}
	return buf.Bytes()
}

func loginCreateFixture(compressed bool) (protocoltrace.Event, error) {
	item := protocol.NewBuffer(64)
	item.WriteU8(protocol.UpdateCreateObject2)
	item.WritePackedGUID(0x4001)
	item.WriteU8(1)
	item.WriteU16(0x0010)
	item.WriteU32(1)
	item.WriteU8(2)
	item.WriteU32(0x00000006)
	item.WriteU32(0)
	item.WriteU32(3)
	item.WriteU32(0)
	player := protocol.NewBuffer(512)
	player.WriteU8(protocol.UpdateCreateObject2)
	player.WritePackedGUID(1)
	player.WriteU8(4)
	player.WriteU16(0x0061)
	player.WriteU32(0)
	player.WriteU16(0)
	player.WriteU32(1)
	player.WriteF32(1)
	player.WriteF32(2)
	player.WriteF32(3)
	player.WriteF32(0)
	player.WriteU32(0)
	for range 9 {
		player.WriteF32(1)
	}
	mask := make([]uint32, 42)
	values := map[int]uint32{0: 1, 2: 0x19, 4: math.Float32bits(1), 23: 0x01020304, 24: 100, 32: 100, 54: 10, 59: 8, 67: 123, 68: 123}
	for field := range values {
		mask[field/32] |= 1 << uint(field%32)
	}
	player.WriteU8(uint8(len(mask)))
	for _, block := range mask {
		player.WriteU32(block)
	}
	for field := 0; field < len(mask)*32; field++ {
		if mask[field/32]&(1<<uint(field%32)) != 0 {
			player.WriteU32(values[field])
		}
	}
	body := protocol.NewBuffer(4 + item.Len() + player.Len())
	body.WriteU32(2)
	body.Write(item.Bytes())
	body.Write(player.Bytes())
	payload := body.Bytes()
	opcode := uint32(protocol.OpcodeSMSG_UPDATE_OBJECT)
	if compressed {
		var compressedBody bytes.Buffer
		writer := zlib.NewWriter(&compressedBody)
		if _, err := writer.Write(payload); err != nil {
			return protocoltrace.Event{}, err
		}
		if err := writer.Close(); err != nil {
			return protocoltrace.Event{}, err
		}
		encoded := protocol.NewBuffer(4 + compressedBody.Len())
		encoded.WriteU32(uint32(len(payload)))
		encoded.Write(compressedBody.Bytes())
		payload = encoded.Bytes()
		opcode = uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)
	}
	return protocoltrace.Event{Direction: protocoltrace.ServerToClient, Opcode: opcode, Payload: base64.StdEncoding.EncodeToString(payload)}, nil
}

func checkLogin(trace protocoltrace.Trace, start int) error {
	if err := rejectPreVerifyAchievementPackets(trace, start); err != nil {
		return err
	}
	if err := checkLoginMovementOrder(trace, start); err != nil {
		return err
	}
	stages := []loginStage{
		{"MSG_SET_DUNGEON_DIFFICULTY", exact(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY)},
		{"SMSG_LOGIN_VERIFY_WORLD", exact(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD)},
		{"SMSG_ACCOUNT_DATA_TIMES", exact(protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES)},
		{"SMSG_FEATURE_SYSTEM_STATUS", exact(protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS)},
		{"SMSG_MOTD", exact(protocol.OpcodeSMSG_MOTD)},
		{"SMSG_LEARNED_DANCE_MOVES", exact(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES)},
		{"SMSG_CONTACT_LIST", exact(protocol.OpcodeSMSG_CONTACT_LIST)},
		{"SMSG_BIND_POINT_UPDATE", exact(protocol.OpcodeSMSG_BIND_POINT_UPDATE)},
		{"SMSG_TALENTS_INFO", exact(protocol.OpcodeSMSG_TALENTS_INFO)},
		{"SMSG_INSTANCE_DIFFICULTY", exact(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY)},
		{"SMSG_INITIAL_SPELLS", exact(protocol.OpcodeSMSG_INITIAL_SPELLS)},
		{"SMSG_SEND_UNLEARN_SPELLS", exact(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS)},
		{"SMSG_ACTION_BUTTONS", exact(protocol.OpcodeSMSG_ACTION_BUTTONS)},
		{"SMSG_INITIALIZE_FACTIONS", exact(protocol.OpcodeSMSG_INITIALIZE_FACTIONS)},
		{"SMSG_ALL_ACHIEVEMENT_DATA", exact(protocol.OpcodeSMSG_ALL_ACHIEVEMENT_DATA)},
		{"SMSG_EQUIPMENT_SET_LIST", exact(protocol.OpcodeSMSG_EQUIPMENT_SET_LIST)},
		{"SMSG_LOGIN_SET_TIME_SPEED", exact(protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED)},
		{"SMSG_SET_FORCED_REACTIONS", exact(protocol.OpcodeSMSG_SET_FORCED_REACTIONS)},
		{"player create update", func(opcode uint32) bool {
			return opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) || opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)
		}},
		{"SMSG_INIT_WORLD_STATES", exact(protocol.OpcodeSMSG_INIT_WORLD_STATES)},
		{"SMSG_TIME_SYNC_REQ", exact(protocol.OpcodeSMSG_TIME_SYNC_REQ)},
		{"SMSG_SPELL_GO", exact(protocol.OpcodeSMSG_SPELL_GO)},
	}
	position := start
	verifyIndex := -1
	playerCreateIndex := -1
	for _, stage := range stages {
		found := -1
		for index := position + 1; index < len(trace.Events); index++ {
			event := trace.Events[index]
			if event.Direction == protocoltrace.ServerToClient && stage.Match(event.Opcode) {
				found = index
				break
			}
			if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("missing or out-of-order %s", stage.Name)
		}
		if stage.Name == "player create update" {
			if err := requireCreateBlock(trace.Events[found]); err != nil {
				return err
			}
			playerCreateIndex = found
		}
		var validate func(protocoltrace.Event) error
		switch stage.Name {
		case "SMSG_LOGIN_VERIFY_WORLD":
			validate = requireLoginVerifyWorld
		case "SMSG_LEARNED_DANCE_MOVES":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 8) }
		case "SMSG_FEATURE_SYSTEM_STATUS":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 2) }
		case "SMSG_BIND_POINT_UPDATE":
			validate = func(event protocoltrace.Event) error { return requirePayloadLength(event, 20) }
		case "SMSG_INSTANCE_DIFFICULTY":
			validate = requireEightBytePayload
		case "SMSG_INITIAL_SPELLS":
			validate = requireInitialSpells
		case "SMSG_SEND_UNLEARN_SPELLS":
			validate = requireUnlearnSpells
		case "SMSG_ACTION_BUTTONS":
			validate = requireActionButtons
		case "SMSG_INITIALIZE_FACTIONS":
			validate = requireInitialFactions
		case "SMSG_SET_FORCED_REACTIONS":
			validate = requireForcedReactions
		case "SMSG_INIT_WORLD_STATES":
			validate = requireInitWorldStates
		case "SMSG_TIME_SYNC_REQ":
			validate = requireTimeSyncRequest
		case "SMSG_SPELL_GO":
			validate = requireLoginEffect
		}
		if validate != nil {
			if err := validate(trace.Events[found]); err != nil {
				return fmt.Errorf("%s: %w", stage.Name, err)
			}
		}
		if stage.Name == "SMSG_LOGIN_SET_TIME_SPEED" {
			if err := requireLoginTimeSpeed(trace.Events[found]); err != nil {
				return err
			}
		}
		if stage.Name == "SMSG_LOGIN_VERIFY_WORLD" {
			verifyIndex = found
		}
		position = found
	}
	if err := checkOptionalLoginPayloads(trace, start); err != nil {
		return err
	}
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_TRIGGER_CINEMATIC) {
			continue
		}
		if verifyIndex >= 0 && index < verifyIndex {
			return fmt.Errorf("SMSG_TRIGGER_CINEMATIC was sent before SMSG_LOGIN_VERIFY_WORLD")
		}
		if playerCreateIndex >= 0 && index > playerCreateIndex {
			return fmt.Errorf("SMSG_TRIGGER_CINEMATIC was sent after player create update")
		}
	}
	return nil
}

func checkOptionalLoginPayloads(trace protocoltrace.Trace, start int) error {
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		var validate func(protocoltrace.Event) error
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_RESYNC_RUNES):
			validate = requireResyncRunes
		case uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL):
			validate = requireAuraUpdateAll
		case uint32(protocol.OpcodeSMSG_ITEM_TIME_UPDATE):
			validate = requirePayloadLengthExact(12)
		case uint32(protocol.OpcodeSMSG_ITEM_ENCHANT_TIME_UPDATE):
			validate = requirePayloadLengthExact(24)
		case uint32(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE):
			validate = requireQuestStatusMultiple
		case uint32(protocol.OpcodeSMSG_TAXINODE_STATUS):
			validate = requirePayloadLengthExact(9)
		case uint32(protocol.OpcodeSMSG_PET_SPELLS):
			validate = requirePetSpells
		case uint32(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS):
			validate = requireQuestGiverDetails
		case uint32(protocol.OpcodeSMSG_GROUP_LIST):
			validate = requireGroupList
		}
		if validate != nil {
			if err := validate(event); err != nil {
				return fmt.Errorf("%s: %w", opcodeName(event.Opcode), err)
			}
		}
	}
	return nil
}

func requireLoginTimeSpeed(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login time field is truncated: %w", err)
	}
	speed, err := reader.ReadF32()
	if err != nil {
		return fmt.Errorf("login time speed is truncated: %w", err)
	}
	holiday, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("login holiday offset is truncated: %w", err)
	}
	if speed != 0.5 || holiday != 0 || reader.Remaining() != 0 {
		return fmt.Errorf("invalid login time-speed payload speed=%v holiday=%d remaining=%d", speed, holiday, reader.Remaining())
	}
	return nil
}

func requireLoginEffect(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("login effect caster GUID is truncated: %w", err)
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("login effect caster-unit GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("login effect cast ID is truncated: %w", err)
	}
	spellID, err := reader.ReadU32()
	if err != nil || spellID != 836 {
		return fmt.Errorf("login effect spell ID=%d, want 836", spellID)
	}
	flags, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("login effect cast flags are truncated: %w", err)
	}
	if flags&protocol.SpellCastFlagPowerLeftSelf == 0 || flags&0x100 == 0 || flags&0x1 == 0 {
		return fmt.Errorf("login effect cast flags=0x%08x, want unknown-9/pending/power-left-self", flags)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("login effect cast time is truncated: %w", err)
	}
	hitCount, err := reader.ReadU8()
	if err != nil || hitCount != 1 {
		return fmt.Errorf("login effect hit count=%d, want 1", hitCount)
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("login effect hit target is truncated: %w", err)
	}
	missCount, err := reader.ReadU8()
	if err != nil || missCount != 0 {
		return fmt.Errorf("login effect miss count=%d, want 0", missCount)
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil {
		return fmt.Errorf("login effect target is truncated: %w", err)
	}
	if target.Flags != protocol.SpellTargetFlagUnit || target.UnitGUID != 0x106 {
		return fmt.Errorf("login effect target flags/guid=0x%08x/0x%x", target.Flags, target.UnitGUID)
	}
	power, err := reader.ReadU32()
	if err != nil || power != 777 {
		return fmt.Errorf("login effect remaining power=%d, want 777", power)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected login effect payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireInitWorldStates(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	for range 3 {
		if _, err := reader.ReadI32(); err != nil {
			return fmt.Errorf("world-state map/zone/area is truncated: %w", err)
		}
	}
	count, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("world-state count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 8); err != nil {
		return fmt.Errorf("world-state entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected world-state payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireForcedReactions(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("forced-reaction count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 8); err != nil {
		return fmt.Errorf("forced-reaction entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected forced-reaction payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireResyncRunes(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 6 {
		return fmt.Errorf("rune count=%d, want 6", count)
	}
	if _, err := reader.Read(int(count) * 2); err != nil {
		return fmt.Errorf("rune entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected rune payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireTimeSyncRequest(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) != 4 {
		return fmt.Errorf("time-sync payload length=%d, want 4", len(payload))
	}
	reader := protocol.NewReader(payload)
	counter, err := reader.ReadU32()
	if err != nil || counter != 0 {
		return fmt.Errorf("initial time-sync counter=%d, want 0", counter)
	}
	return nil
}

func requirePayloadLengthExact(length int) func(protocoltrace.Event) error {
	return func(event protocoltrace.Event) error {
		return requirePayloadLength(event, length)
	}
}

func requireAuraUpdateAll(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("aura target GUID is truncated: %w", err)
	}
	count := 0
	for reader.Remaining() > 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura slot is truncated: %w", err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("aura spell is truncated: %w", err)
		}
		flags, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("aura flags are truncated: %w", err)
		}
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura caster level is truncated: %w", err)
		}
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("aura stack count is truncated: %w", err)
		}
		if flags&protocol.AuraFlagCaster == 0 {
			if _, err := reader.ReadPackedGUID(); err != nil {
				return fmt.Errorf("aura caster GUID is truncated: %w", err)
			}
		}
		if flags&protocol.AuraFlagDuration != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("aura max duration is truncated: %w", err)
			}
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("aura duration is truncated: %w", err)
			}
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("aura update has no records")
	}
	return nil
}

func requireQuestStatusMultiple(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest-status count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 9); err != nil {
		return fmt.Errorf("quest-status entries are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected quest-status payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireQuestGiverDetails(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("quest giver GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("quest inform GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("quest ID is truncated: %w", err)
	}
	for _, field := range []string{"title", "details", "objectives"} {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("quest %s is truncated: %w", field, err)
		}
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("quest auto-launch flag is truncated: %w", err)
	}
	if _, err := reader.Read(9); err != nil {
		return fmt.Errorf("quest header fields are truncated: %w", err)
	}
	choiceCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest choice count is truncated: %w", err)
	}
	if _, err := reader.Read(int(choiceCount) * 12); err != nil {
		return fmt.Errorf("quest choice items are truncated: %w", err)
	}
	rewardCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("quest reward count is truncated: %w", err)
	}
	if _, err := reader.Read(int(rewardCount) * 12); err != nil {
		return fmt.Errorf("quest reward items are truncated: %w", err)
	}
	if _, err := reader.Read(40 + 60); err != nil {
		return fmt.Errorf("quest reward fields are truncated: %w", err)
	}
	emoteCount, err := reader.ReadI32()
	if err != nil || emoteCount < 0 {
		return fmt.Errorf("quest emote count=%d", emoteCount)
	}
	if _, err := reader.Read(int(emoteCount) * 8); err != nil {
		return fmt.Errorf("quest emotes are truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected quest-details payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requirePetSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) == 8 {
		return nil
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("pet GUID is truncated: %w", err)
	}
	if _, err := reader.ReadU16(); err != nil {
		return fmt.Errorf("pet family is truncated: %w", err)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("pet duration is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("pet react state is truncated: %w", err)
	}
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("pet command state is truncated: %w", err)
	}
	if _, err := reader.ReadU16(); err != nil {
		return fmt.Errorf("pet flags are truncated: %w", err)
	}
	if _, err := reader.Read(10 * 4); err != nil {
		return fmt.Errorf("pet action bar is truncated: %w", err)
	}
	spellCount, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("pet spell count is truncated: %w", err)
	}
	if _, err := reader.Read(int(spellCount) * 4); err != nil {
		return fmt.Errorf("pet spell list is truncated: %w", err)
	}
	cooldownCount, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("pet cooldown count is truncated: %w", err)
	}
	if _, err := reader.Read(int(cooldownCount) * 14); err != nil {
		return fmt.Errorf("pet cooldown list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected pet-spell payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireLoginVerifyWorld(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadI32(); err != nil {
		return fmt.Errorf("map ID is truncated: %w", err)
	}
	for index := 0; index < 4; index++ {
		value, readErr := reader.ReadF32()
		if readErr != nil {
			return fmt.Errorf("position field %d is truncated: %w", index, readErr)
		}
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("position field %d is not finite", index)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected login verify payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireEightBytePayload(event protocoltrace.Event) error {
	return requirePayloadLength(event, 8)
}

func requirePayloadLength(event protocoltrace.Event, want int) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) != want {
		return fmt.Errorf("payload length=%d, want %d", len(payload), want)
	}
	return nil
}

func requireInitialSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	if _, err := reader.ReadU8(); err != nil {
		return fmt.Errorf("initial spell flags are truncated: %w", err)
	}
	count, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("initial spell count is truncated: %w", err)
	}
	for index := uint16(0); index < count; index++ {
		if _, err := reader.Read(6); err != nil {
			return fmt.Errorf("initial spell %d is truncated: %w", index, err)
		}
	}
	cooldowns, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("initial cooldown count is truncated: %w", err)
	}
	for index := uint16(0); index < cooldowns; index++ {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("initial cooldown %d is truncated: %w", index, err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected initial-spell payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireUnlearnSpells(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("unlearn count is truncated: %w", err)
	}
	if _, err := reader.Read(int(count) * 4); err != nil {
		return fmt.Errorf("unlearn spell list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected unlearn payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireActionButtons(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	state := byte(0)
	if len(payload) > 0 {
		state = payload[0]
	}
	if len(payload) != 1+144*4 || state != 1 {
		return fmt.Errorf("action-button payload length/state=%d/%d, want 577/1", len(payload), state)
	}
	return nil
}

func requireInitialFactions(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if len(payload) < 4 {
		return fmt.Errorf("faction payload is truncated")
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 128 {
		return fmt.Errorf("faction count=%d, want 128", count)
	}
	if _, err := reader.Read(128 * 5); err != nil {
		return fmt.Errorf("faction state list is truncated: %w", err)
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected faction payload bytes=%d", reader.Remaining())
	}
	return nil
}

func requireGroupList(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	groupType, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("group type is truncated: %w", err)
	}
	if _, err := reader.Read(3); err != nil {
		return fmt.Errorf("group slot state is truncated: %w", err)
	}
	if groupType&0x08 != 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("LFG status is truncated: %w", err)
		}
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("LFG dungeon is truncated: %w", err)
		}
	}
	groupGUID, err := reader.ReadU64()
	if err != nil {
		return fmt.Errorf("group GUID is truncated: %w", err)
	}
	if groupGUID>>48 != 0x1F50 || groupGUID&0xFFFFFFFF == 0 {
		return fmt.Errorf("group GUID=0x%016x, want HighGuid::Group 0x1F50 with nonzero low word", groupGUID)
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("group counter is truncated: %w", err)
	}
	memberCount, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("group member count is truncated: %w", err)
	}
	for index := uint32(0); index < memberCount; index++ {
		if _, err := reader.ReadCString(); err != nil {
			return fmt.Errorf("group member %d name is truncated: %w", index, err)
		}
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("group member %d GUID is truncated: %w", index, err)
		}
		if _, err := reader.Read(4); err != nil {
			return fmt.Errorf("group member %d state is truncated: %w", index, err)
		}
	}
	if _, err := reader.ReadU64(); err != nil {
		return fmt.Errorf("group leader GUID is truncated: %w", err)
	}
	if memberCount > 0 {
		if _, err := reader.ReadU8(); err != nil {
			return fmt.Errorf("group loot method is truncated: %w", err)
		}
		if _, err := reader.ReadU64(); err != nil {
			return fmt.Errorf("group master looter GUID is truncated: %w", err)
		}
		if _, err := reader.Read(4); err != nil {
			return fmt.Errorf("group difficulty state is truncated: %w", err)
		}
	}
	if reader.Remaining() != 0 {
		return fmt.Errorf("unexpected group-list payload bytes=%d", reader.Remaining())
	}
	return nil
}

func rejectPreVerifyAchievementPackets(trace protocoltrace.Trace, start int) error {
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD) {
			return nil
		}
		if event.Direction == protocoltrace.ServerToClient && (event.Opcode == uint32(protocol.OpcodeSMSG_CRITERIA_UPDATE) || event.Opcode == uint32(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED)) {
			return fmt.Errorf("%s was sent before SMSG_LOGIN_VERIFY_WORLD", opcodeName(event.Opcode))
		}
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			return fmt.Errorf("login ended before SMSG_LOGIN_VERIFY_WORLD")
		}
	}
	return fmt.Errorf("missing SMSG_LOGIN_VERIFY_WORLD")
}

func checkLoginMovementOrder(trace protocoltrace.Trace, start int) error {
	order := map[uint32]int{
		uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK):           0,
		uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL):         1,
		uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER):            2,
		uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY):          3,
		uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE): 4,
		uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT):           5,
		uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES):            6,
		uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL):           7,
	}
	last := -1
	seen := make(map[int]struct{}, len(order))
	timeSyncSeen := false
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		if event.Opcode == uint32(protocol.OpcodeSMSG_TIME_SYNC_REQ) {
			timeSyncSeen = true
			continue
		}
		stage, ok := order[event.Opcode]
		if !ok {
			continue
		}
		if !timeSyncSeen {
			return fmt.Errorf("movement packet %s arrived before SMSG_TIME_SYNC_REQ", opcodeName(event.Opcode))
		}
		if _, alreadySeen := seen[stage]; alreadySeen {
			continue
		}
		if stage < last {
			return fmt.Errorf("movement packet %s arrived after a later movement stage", opcodeName(event.Opcode))
		}
		seen[stage] = struct{}{}
		last = stage
	}
	return nil
}

func opcodeName(opcode uint32) string {
	if name, ok := protocol.OpcodeNames[protocol.Opcode(opcode)]; ok {
		return name
	}
	return fmt.Sprintf("opcode 0x%X", opcode)
}

func exact(opcode protocol.Opcode) func(uint32) bool {
	return func(value uint32) bool { return value == uint32(opcode) }
}

func requireCreateBlock(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	if event.Opcode == uint32(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			return fmt.Errorf("compressed player update decode failed: %w", err)
		}
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count == 0 {
		return fmt.Errorf("player update has no blocks")
	}
	playerFound := false
	for index := uint32(0); index < count; index++ {
		kind, err := reader.ReadU8()
		if err != nil {
			return fmt.Errorf("player update block %d is truncated: %w", index, err)
		}
		switch kind {
		case protocol.UpdateOutOfRangeObjects:
			outOfRange, err := reader.ReadU32()
			if err != nil {
				return fmt.Errorf("out-of-range update is truncated: %w", err)
			}
			for guidIndex := uint32(0); guidIndex < outOfRange; guidIndex++ {
				if _, err := reader.ReadPackedGUID(); err != nil {
					return fmt.Errorf("out-of-range GUID is truncated: %w", err)
				}
			}
		case protocol.UpdateCreateObject, protocol.UpdateCreateObject2:
			typeID, err := parseCreateObjectBlock(reader)
			if err != nil {
				return fmt.Errorf("create block %d: %w", index, err)
			}
			if typeID == 1 || typeID == 2 {
				if playerFound {
					return fmt.Errorf("item/container create block %d arrived after self-player create", index)
				}
			}
			if typeID == 4 {
				if playerFound {
					return fmt.Errorf("duplicate self-player create block %d", index)

				}
				playerFound = true
			}
		case protocol.UpdateValues:
			if err := skipValuesUpdate(reader); err != nil {
				return fmt.Errorf("values block %d: %w", index, err)
			}
		case protocol.UpdateMovement:
			if err := skipMovementUpdate(reader); err != nil {
				return fmt.Errorf("movement block %d: %w", index, err)
			}
		default:
			return fmt.Errorf("unsupported update block kind=%d", kind)
		}
	}
	if !playerFound {
		return fmt.Errorf("player create block not found")
	}
	return nil
}

func parseCreateObjectBlock(reader *protocol.Buffer) (uint8, error) {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return 0, fmt.Errorf("create GUID is truncated: %w", err)
	}
	typeID, err := reader.ReadU8()
	if err != nil {
		return 0, fmt.Errorf("create type is truncated: %w", err)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return 0, fmt.Errorf("create movement flags are truncated: %w", err)
	}
	if err := skipCreateMovement(reader, flags); err != nil {
		return 0, err
	}
	mask, values, err := readUpdateValues(reader)
	if err != nil {
		return 0, err
	}
	if typeID != 4 {
		return typeID, nil
	}
	if flags != 0x0061 {
		return 0, fmt.Errorf("player create flags=0x%X, want 0x61", flags)
	}
	if len(mask) != 42 {
		return 0, fmt.Errorf("player update mask blocks=%d, want 42", len(mask))
	}
	for field := 1326; field < len(mask)*32; field++ {
		if updateMaskHas(mask, field) {
			return 0, fmt.Errorf("player update mask sets out-of-range field %d", field)
		}
	}
	for _, field := range []int{0, 2, 4, 23, 24, 32, 54, 59, 67, 68} {
		if !updateMaskHas(mask, field) {
			return 0, fmt.Errorf("player update mask omits required field %d", field)
		}
	}
	if values[2] != 0x19 || values[23] == 0 || values[54] == 0 || values[67] == 0 || values[68] == 0 || values[24] == 0 || values[32] == 0 {
		return 0, fmt.Errorf("player create required field values are invalid")
	}
	if values[59]&0x00000008 == 0 {
		return 0, fmt.Errorf("player create omits UNIT_FLAG_PLAYER_CONTROLLED")
	}
	return typeID, nil
}

func skipCreateMovement(reader *protocol.Buffer, flags uint16) error {
	if flags&0x20 != 0 {
		if err := skipLivingMovement(reader); err != nil {
			return err
		}
	} else if flags&0x100 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("position transport GUID is truncated: %w", err)
		}
		if _, err := reader.Read(32); err != nil {
			return fmt.Errorf("position movement is truncated: %w", err)
		}
	} else if flags&0x40 != 0 {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("stationary movement is truncated: %w", err)
		}
	}
	if flags&0x8 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("unknown movement field is truncated: %w", err)
		}
	}
	if flags&0x10 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("low-guid movement field is truncated: %w", err)
		}
	}
	if flags&0x4 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("target movement field is truncated: %w", err)
		}
	}
	if flags&0x2 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("transport movement field is truncated: %w", err)
		}
	}
	if flags&0x80 != 0 {
		if _, err := reader.Read(8); err != nil {
			return fmt.Errorf("vehicle movement field is truncated: %w", err)
		}
	}
	if flags&0x200 != 0 {
		if _, err := reader.Read(8); err != nil {
			return fmt.Errorf("rotation movement field is truncated: %w", err)
		}
	}
	return nil
}

func readUpdateValues(reader *protocol.Buffer) ([]uint32, map[int]uint32, error) {
	maskBlocks, err := reader.ReadU8()
	if err != nil {
		return nil, nil, fmt.Errorf("update mask is truncated: %w", err)
	}
	mask := make([]uint32, maskBlocks)
	for index := range mask {
		mask[index], err = reader.ReadU32()
		if err != nil {
			return nil, nil, fmt.Errorf("update mask block %d is truncated: %w", index, err)
		}
	}
	values := make(map[int]uint32)
	for index, bits := range mask {
		for bit := uint(0); bit < 32; bit++ {
			field := index*32 + int(bit)
			if bits&(uint32(1)<<bit) == 0 {
				continue
			}
			value, readErr := reader.ReadU32()
			if readErr != nil {
				return nil, nil, fmt.Errorf("update field %d is truncated: %w", field, readErr)
			}
			values[field] = value
		}
	}
	return mask, values, nil
}

func skipValuesUpdate(reader *protocol.Buffer) error {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("values GUID is truncated: %w", err)
	}
	_, _, err := readUpdateValues(reader)
	return err
}

func skipMovementUpdate(reader *protocol.Buffer) error {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("movement GUID is truncated: %w", err)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("movement flags are truncated: %w", err)
	}
	return skipCreateMovement(reader, flags)
}

func skipLivingMovement(reader *protocol.Buffer) error {
	movementFlags, err := reader.ReadU32()
	if err != nil {
		return fmt.Errorf("player movement flags are truncated: %w", err)
	}
	extraFlags, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("player extra movement flags are truncated: %w", err)
	}
	if _, err := reader.Read(20); err != nil {
		return fmt.Errorf("player movement position is truncated: %w", err)
	}
	if movementFlags&0x200 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("player transport GUID is truncated: %w", err)
		}
		if _, err := reader.Read(17); err != nil {
			return fmt.Errorf("player transport offsets are truncated: %w", err)
		}
		if extraFlags&0x1 != 0 {
			if _, err := reader.ReadU32(); err != nil {
				return fmt.Errorf("player interpolated transport time is truncated: %w", err)
			}
		}
	}
	if movementFlags&(0x2000|0x4000) != 0 || extraFlags&0x2 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return fmt.Errorf("player pitch is truncated: %w", err)
		}
	}
	if _, err := reader.ReadU32(); err != nil {
		return fmt.Errorf("player fall time is truncated: %w", err)
	}
	if movementFlags&0x1000 != 0 {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("player jump movement is truncated: %w", err)
		}
	}
	if movementFlags&0x04000000 != 0 {
		if _, err := reader.ReadF32(); err != nil {
			return fmt.Errorf("player spline elevation is truncated: %w", err)
		}
	}
	if _, err := reader.Read(36); err != nil {
		return fmt.Errorf("player movement speeds are truncated: %w", err)
	}
	return nil
}

func updateMaskHas(mask []uint32, field int) bool {
	block := field / 32
	bit := uint(field % 32)
	return block >= 0 && block < len(mask) && mask[block]&(uint32(1)<<bit) != 0
}

func eventPayload(event protocoltrace.Event) ([]byte, error) {
	return protocoltrace.Trace{Events: []protocoltrace.Event{event}}.Payload(event)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

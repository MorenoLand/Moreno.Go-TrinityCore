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
	validMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_FEATHER_FALL)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_HOVER)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MULTIPLE_MOVES)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_AURA_UPDATE_ALL)}}}
	if err := checkLoginMovementOrder(validMovement, 0); err != nil {
		return fmt.Errorf("valid movement ordering was rejected: %w", err)
	}
	invalidMovement := protocoltrace.Trace{Events: []protocoltrace.Event{{Direction: protocoltrace.ClientToServer, Opcode: login}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY)}, {Direction: protocoltrace.ServerToClient, Opcode: uint32(protocol.OpcodeSMSG_MOVE_WATER_WALK)}}}
	if err := checkLoginMovementOrder(invalidMovement, 0); err == nil {
		return fmt.Errorf("out-of-order movement packets were not rejected")
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
	return nil
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
		if stage.Name == "SMSG_LOGIN_VERIFY_WORLD" {
			verifyIndex = found
		}
		position = found
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
	for index := start + 1; index < len(trace.Events); index++ {
		event := trace.Events[index]
		if event.Direction == protocoltrace.ClientToServer && (event.Opcode == uint32(protocol.OpcodeCMSG_PLAYER_LOGIN) || event.Opcode == uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST)) {
			break
		}
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		stage, ok := order[event.Opcode]
		if !ok {
			continue
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
			player, err := parseCreateObjectBlock(reader)
			if err != nil {
				return fmt.Errorf("create block %d: %w", index, err)
			}
			if player {
				return nil
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
	return fmt.Errorf("player create block not found")
}

func parseCreateObjectBlock(reader *protocol.Buffer) (bool, error) {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return false, fmt.Errorf("create GUID is truncated: %w", err)
	}
	typeID, err := reader.ReadU8()
	if err != nil {
		return false, fmt.Errorf("create type is truncated: %w", err)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return false, fmt.Errorf("create movement flags are truncated: %w", err)
	}
	if err := skipCreateMovement(reader, flags); err != nil {
		return false, err
	}
	mask, values, err := readUpdateValues(reader)
	if err != nil {
		return false, err
	}
	if typeID != 4 {
		return false, nil
	}
	if flags != 0x0061 {
		return false, fmt.Errorf("player create flags=0x%X, want 0x61", flags)
	}
	if len(mask) != 42 {
		return false, fmt.Errorf("player update mask blocks=%d, want 42", len(mask))
	}
	for field := 1326; field < len(mask)*32; field++ {
		if updateMaskHas(mask, field) {
			return false, fmt.Errorf("player update mask sets out-of-range field %d", field)
		}
	}
	for _, field := range []int{0, 2, 4, 23, 24, 32, 54, 59, 67, 68} {
		if !updateMaskHas(mask, field) {
			return false, fmt.Errorf("player update mask omits required field %d", field)
		}
	}
	if values[2] != 0x19 || values[23] == 0 || values[54] == 0 || values[67] == 0 || values[68] == 0 || values[24] == 0 || values[32] == 0 {
		return false, fmt.Errorf("player create required field values are invalid")
	}
	if values[59]&0x00000008 == 0 {
		return false, fmt.Errorf("player create omits UNIT_FLAG_PLAYER_CONTROLLED")
	}
	return true, nil
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

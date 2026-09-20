package main

import (
	"flag"
	"fmt"
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
	flag.Parse()
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

func checkLogin(trace protocoltrace.Trace, start int) error {
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
		}
		position = found
	}
	return nil
}

func exact(opcode protocol.Opcode) func(uint32) bool {
	return func(value uint32) bool { return value == uint32(opcode) }
}

func requireCreateBlock(event protocoltrace.Event) error {
	payload, err := eventPayload(event)
	if err != nil {
		return err
	}
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count == 0 {
		return fmt.Errorf("player update has no blocks")
	}
	kind, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("player update block is truncated: %w", err)
	}
	if kind != protocol.UpdateCreateObject && kind != protocol.UpdateCreateObject2 {
		return fmt.Errorf("first player update block kind=%d, want create", kind)
	}
	return requirePlayerCreateFields(reader)
}

func requirePlayerCreateFields(reader *protocol.Buffer) error {
	if _, err := reader.ReadPackedGUID(); err != nil {
		return fmt.Errorf("player create GUID is truncated: %w", err)
	}
	typeID, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("player create type is truncated: %w", err)
	}
	if typeID != 4 {
		return fmt.Errorf("first create object type=%d, want player type 4", typeID)
	}
	flags, err := reader.ReadU16()
	if err != nil {
		return fmt.Errorf("player movement flags are truncated: %w", err)
	}
	if flags&0x20 != 0 {
		if err := skipLivingMovement(reader); err != nil {
			return err
		}
	} else if flags&0x40 != 0 {
		if _, err := reader.Read(16); err != nil {
			return fmt.Errorf("player stationary movement is truncated: %w", err)
		}
	}
	if flags&0x8 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("player unknown movement field is truncated: %w", err)
		}
	}
	if flags&0x10 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("player low-guid movement field is truncated: %w", err)
		}
	}
	if flags&0x4 != 0 {
		if _, err := reader.ReadPackedGUID(); err != nil {
			return fmt.Errorf("player target movement field is truncated: %w", err)
		}
	}
	if flags&0x2 != 0 {
		if _, err := reader.ReadU32(); err != nil {
			return fmt.Errorf("player transport movement field is truncated: %w", err)
		}
	}
	maskBlocks, err := reader.ReadU8()
	if err != nil {
		return fmt.Errorf("player update mask is truncated: %w", err)
	}
	mask := make([]uint32, maskBlocks)
	for index := range mask {
		mask[index], err = reader.ReadU32()
		if err != nil {
			return fmt.Errorf("player update mask block %d is truncated: %w", index, err)
		}
	}
	if !updateMaskHas(mask, 0) {
		return fmt.Errorf("player update mask omits OBJECT_FIELD_GUID low word")
	}
	if !updateMaskHas(mask, 2) || !updateMaskHas(mask, 23) || !updateMaskHas(mask, 67) {
		return fmt.Errorf("player update mask omits required type, race/class, or display fields")
	}
	return nil
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

package scripting

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"

	"github.com/Shopify/go-lua"
)

const packetMetaTable = "MorenoCore.Packet"

type Packet struct {
	Opcode  uint32
	Data    []byte
	ReadPos int
}

func pushPacket(state *lua.State, packet *Packet) {
	state.PushUserData(packet)
	lua.SetMetaTableNamed(state, packetMetaTable)
}

func installPacketMetaTable(state *lua.State) {
	lua.NewMetaTable(state, packetMetaTable)
	lua.SetFunctions(state, []lua.RegistryFunction{{Name: "__index", Function: packetIndex}}, 0)
	state.Pop(1)
}

func packetIndex(state *lua.State) int {
	packet := lua.CheckUserData(state, 1, packetMetaTable).(*Packet)
	name := lua.CheckString(state, 2)
	state.PushGoFunction(func(call *lua.State) int { return packetMethod(call, packet, name) })
	return 1
}

func packetMethod(state *lua.State, packet *Packet, name string) int {
	switch name {
	case "GetOpcode":
		state.PushUnsigned(uint(packet.Opcode))
		return 1
	case "GetSize":
		state.PushUnsigned(uint(len(packet.Data)))
		return 1
	case "SetOpcode":
		value := checkLuaUint64(state, 2)
		if value > math.MaxUint32 {
			lua.ArgumentError(state, 2, "opcode must be uint32")
		}
		if value >= 0x51F { // NUM_MSG_TYPES (Opcodes.h:1340)
			lua.ArgumentError(state, 2, "valid opcode expected")
		}
		packet.Opcode = uint32(value)
		return 0
	case "ReadByte":
		value, err := packetRead(packet, 1)
		if err != nil {
			return packetError(state, err)
		}
		state.PushInteger(int(int8(value[0])))
		return 1
	case "ReadUByte":
		value, err := packetRead(packet, 1)
		if err != nil {
			return packetError(state, err)
		}
		state.PushUnsigned(uint(value[0]))
		return 1
	case "ReadShort":
		value, err := packetRead(packet, 2)
		if err != nil {
			return packetError(state, err)
		}
		state.PushInteger(int(int16(binary.LittleEndian.Uint16(value))))
		return 1
	case "ReadUShort":
		value, err := packetRead(packet, 2)
		if err != nil {
			return packetError(state, err)
		}
		state.PushUnsigned(uint(binary.LittleEndian.Uint16(value)))
		return 1
	case "ReadLong":
		value, err := packetRead(packet, 4)
		if err != nil {
			return packetError(state, err)
		}
		state.PushInteger(int(int32(binary.LittleEndian.Uint32(value))))
		return 1
	case "ReadULong":
		value, err := packetRead(packet, 4)
		if err != nil {
			return packetError(state, err)
		}
		state.PushUnsigned(uint(binary.LittleEndian.Uint32(value)))
		return 1
	case "ReadFloat":
		value, err := packetRead(packet, 4)
		if err != nil {
			return packetError(state, err)
		}
		state.PushNumber(float64(math.Float32frombits(binary.LittleEndian.Uint32(value))))
		return 1
	case "ReadDouble":
		value, err := packetRead(packet, 8)
		if err != nil {
			return packetError(state, err)
		}
		state.PushNumber(math.Float64frombits(binary.LittleEndian.Uint64(value)))
		return 1
	case "ReadGUID":
		value, err := packetRead(packet, 8)
		if err != nil {
			return packetError(state, err)
		}
		pushUInt64(state, binary.LittleEndian.Uint64(value))
		return 1
	case "ReadString":
		start := packet.ReadPos
		end := bytes.IndexByte(packet.Data[start:], 0)
		if end < 0 {
			// C++ ReadCString reads to the end of the buffer when no
			// terminator is present; only invalid UTF-8 throws.
			packet.ReadPos = len(packet.Data)
			state.PushString(string(packet.Data[start:]))
			return 1
		}
		packet.ReadPos += end + 1
		state.PushString(string(packet.Data[start : start+end]))
		return 1
	case "WriteByte":
		packet.Data = append(packet.Data, byte(int8(lua.CheckInteger(state, 2))))
		return 0
	case "WriteUByte":
		packet.Data = append(packet.Data, byte(lua.CheckUnsigned(state, 2)))
		return 0
	case "WriteShort":
		var data [2]byte
		binary.LittleEndian.PutUint16(data[:], uint16(int16(lua.CheckInteger(state, 2))))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteUShort":
		var data [2]byte
		binary.LittleEndian.PutUint16(data[:], uint16(lua.CheckUnsigned(state, 2)))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteLong":
		var data [4]byte
		binary.LittleEndian.PutUint32(data[:], uint32(int32(lua.CheckInteger(state, 2))))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteULong":
		var data [4]byte
		binary.LittleEndian.PutUint32(data[:], uint32(lua.CheckUnsigned(state, 2)))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteFloat":
		var data [4]byte
		binary.LittleEndian.PutUint32(data[:], math.Float32bits(float32(lua.CheckNumber(state, 2))))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteDouble":
		var data [8]byte
		binary.LittleEndian.PutUint64(data[:], math.Float64bits(lua.CheckNumber(state, 2)))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteGUID":
		var data [8]byte
		binary.LittleEndian.PutUint64(data[:], checkLuaUint64(state, 2))
		packet.Data = append(packet.Data, data[:]...)
		return 0
	case "WriteString":
		packet.Data = append(packet.Data, []byte(lua.CheckString(state, 2))...)
		packet.Data = append(packet.Data, 0)
		return 0
	default:
		state.PushNil()
		return 1
	}
}

func packetRead(packet *Packet, size int) ([]byte, error) {
	if size < 0 || packet.ReadPos < 0 || packet.ReadPos+size > len(packet.Data) {
		return nil, errors.New("packet read exceeds buffer")
	}
	value := packet.Data[packet.ReadPos : packet.ReadPos+size]
	packet.ReadPos += size
	return value, nil
}

func packetError(state *lua.State, err error) int { lua.Errorf(state, "%s", err); return 0 }

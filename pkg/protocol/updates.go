package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"sort"
)

const (
	UpdateValues uint8 = iota
	UpdateMovement
	UpdateCreateObject
	UpdateCreateObject2
	UpdateOutOfRangeObjects
	UpdateNearObjects
)

type UpdateMask struct {
	bits   []uint32
	fields int
}

func NewUpdateMask(fields int) *UpdateMask {
	if fields < 0 {
		fields = 0
	}
	return &UpdateMask{bits: make([]uint32, (fields+31)/32), fields: fields}
}

func (m *UpdateMask) Set(index int) error {
	if m == nil || index < 0 || index >= m.fields {
		return errors.New("update field index out of range")
	}
	m.bits[index/32] |= 1 << uint(index%32)
	return nil
}

func (m *UpdateMask) Clear(index int) error {
	if m == nil || index < 0 || index >= m.fields {
		return errors.New("update field index out of range")
	}
	m.bits[index/32] &^= 1 << uint(index%32)
	return nil
}

func (m *UpdateMask) Has(index int) bool {
	return m != nil && index >= 0 && index < m.fields && m.bits[index/32]&(1<<uint(index%32)) != 0
}
func (m *UpdateMask) AppendTo(packet *Buffer) {
	if m == nil {
		return
	}
	for _, part := range m.bits {
		packet.WriteU32(part)
	}
}
func (m *UpdateMask) BlockCount() int {
	if m == nil {
		return 0
	}
	return len(m.bits)
}

type UpdateData struct {
	blocks     [][]byte
	outOfRange []uint64
}

func NewUpdateData() *UpdateData { return &UpdateData{} }
func (u *UpdateData) AddUpdateBlock(block []byte) {
	u.blocks = append(u.blocks, append([]byte(nil), block...))
}
func (u *UpdateData) AddOutOfRangeGUID(guid uint64) { u.outOfRange = append(u.outOfRange, guid) }
func (u *UpdateData) HasData() bool {
	return u != nil && (len(u.blocks) != 0 || len(u.outOfRange) != 0)
}

func (u *UpdateData) BuildPacket(compressionLevel int) (*Packet, error) {
	if u == nil {
		return nil, errors.New("update data is nil")
	}
	guids := append([]uint64(nil), u.outOfRange...)
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	body := NewBuffer(4)
	blockCount := len(u.blocks)
	if len(guids) != 0 {
		blockCount++
	}
	body.WriteU32(uint32(blockCount))
	if len(guids) != 0 {
		body.WriteU8(UpdateOutOfRangeObjects)
		body.WriteU32(uint32(len(guids)))
		for _, guid := range guids {
			body.WritePackedGUID(guid)
		}
	}
	for _, block := range u.blocks {
		body.Write(block)
	}
	return buildUpdatePacket(body.Bytes(), compressionLevel)
}

func MergeUpdatePackets(packets ...*Packet) (*Packet, error) {
	valid := make([]*Packet, 0, len(packets))
	for _, packet := range packets {
		if packet != nil {
			valid = append(valid, packet)
		}
	}
	if len(valid) == 0 {
		return nil, nil
	}
	if len(valid) == 1 {
		return valid[0], nil
	}
	body := NewBuffer(4)
	count := 0
	for _, packet := range valid {
		raw, err := updatePacketBody(packet)
		if err != nil {
			return nil, err
		}
		if len(raw) < 4 {
			return nil, errors.New("update packet body is too short")
		}
		count += int(binary.LittleEndian.Uint32(raw[:4]))
		body.Write(raw[4:])
	}
	merged := NewBuffer(body.Len() + 4)
	merged.WriteU32(uint32(count))
	merged.Write(body.Bytes())
	return buildUpdatePacket(merged.Bytes(), 0)
}

func updatePacketBody(packet *Packet) ([]byte, error) {
	if packet == nil || packet.Payload == nil {
		return nil, errors.New("update packet is nil")
	}
	switch packet.Opcode {
	case uint16(OpcodeSMSG_UPDATE_OBJECT):
		return packet.Payload.Bytes(), nil
	case uint16(OpcodeSMSG_COMPRESSED_UPDATE_OBJECT):
		return DecompressUpdatePayload(packet.Payload.Bytes())
	default:
		return nil, errors.New("packet is not an update-object packet")
	}
}

func buildUpdatePacket(body []byte, compressionLevel int) (*Packet, error) {
	if len(body) <= 100 {
		return PacketFrom(uint16(OpcodeSMSG_UPDATE_OBJECT), body), nil
	}
	var compressed bytes.Buffer
	level := compressionLevel
	if level == 0 {
		level = zlib.BestSpeed
	}
	writer, err := zlib.NewWriterLevel(&compressed, level)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(body); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	payload := NewBuffer(compressed.Len() + 4)
	payload.WriteU32(uint32(len(body)))
	payload.Write(compressed.Bytes())
	return PacketFrom(uint16(OpcodeSMSG_COMPRESSED_UPDATE_OBJECT), payload.Bytes()), nil
}

func DecompressUpdatePayload(payload []byte) ([]byte, error) {
	if len(payload) < 4 {
		return nil, errors.New("compressed update payload is too short")
	}
	reader, err := zlib.NewReader(bytes.NewReader(payload[4:]))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if _, err := output.ReadFrom(reader); err != nil {
		return nil, err
	}
	if err := reader.Close(); err != nil {
		return nil, err
	}
	want := binary.LittleEndian.Uint32(payload[:4])
	if uint32(output.Len()) != want {
		return nil, errors.New("compressed update size mismatch")
	}
	return output.Bytes(), nil
}

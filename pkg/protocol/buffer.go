package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"time"
)

type Buffer struct {
	data []byte
	pos  int
}

func NewBuffer(capacity int) *Buffer {
	if capacity < 0 {
		capacity = 0
	}
	return &Buffer{data: make([]byte, 0, capacity)}
}

func NewReader(data []byte) *Buffer { return &Buffer{data: data} }
func (b *Buffer) Bytes() []byte     { return b.data }
func (b *Buffer) Len() int          { return len(b.data) }
func (b *Buffer) Position() int     { return b.pos }
func (b *Buffer) Remaining() int    { return len(b.data) - b.pos }
func (b *Buffer) ResetRead()        { b.pos = 0 }

func (b *Buffer) SetPosition(pos int) error {
	if pos < 0 || pos > len(b.data) {
		return errors.New("protocol buffer position out of range")
	}
	b.pos = pos
	return nil
}

func (b *Buffer) Write(data []byte) { b.data = append(b.data, data...) }

func (b *Buffer) Put(pos int, data []byte) error {
	if pos < 0 || pos+len(data) > len(b.data) {
		return errors.New("protocol buffer put out of range")
	}
	copy(b.data[pos:], data)
	return nil
}

func (b *Buffer) WriteBool(value bool) {
	if value {
		b.data = append(b.data, 1)
	} else {
		b.data = append(b.data, 0)
	}
}
func (b *Buffer) WriteU8(value uint8) { b.data = append(b.data, value) }
func (b *Buffer) WriteU16(value uint16) {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], value)
	b.Write(data[:])
}
func (b *Buffer) WriteU32(value uint32) {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	b.Write(data[:])
}
func (b *Buffer) WriteU64(value uint64) {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], value)
	b.Write(data[:])
}
func (b *Buffer) WriteI8(value int8)        { b.WriteU8(uint8(value)) }
func (b *Buffer) WriteI16(value int16)      { b.WriteU16(uint16(value)) }
func (b *Buffer) WriteI32(value int32)      { b.WriteU32(uint32(value)) }
func (b *Buffer) WriteI64(value int64)      { b.WriteU64(uint64(value)) }
func (b *Buffer) WriteF32(value float32)    { b.WriteU32(math.Float32bits(value)) }
func (b *Buffer) WriteF64(value float64)    { b.WriteU64(math.Float64bits(value)) }
func (b *Buffer) WriteCString(value string) { b.Write([]byte(value)); b.WriteU8(0) }
func (b *Buffer) WriteString(value string)  { b.Write([]byte(value)) }

// PackTime encodes a time into the client's packed 4-byte time format
// (ByteBuffer::AppendPackedTime, src/server/shared/Packets/ByteBuffer.cpp:132).
func PackTime(value time.Time) uint32 {
	value = value.Local()
	return uint32(value.Year()-2000)<<24 | uint32(value.Month()-1)<<20 | uint32(value.Day()-1)<<14 | uint32(value.Weekday())<<11 | uint32(value.Hour())<<6 | uint32(value.Minute())
}

func (b *Buffer) WritePackedTime(value time.Time) {
	b.WriteU32(PackTime(value))
}

func (b *Buffer) WritePackedGUID(value uint64) {
	var mask uint8
	var bytes [8]byte
	for i := range bytes {
		bytes[i] = byte(value >> (8 * i))
		if bytes[i] != 0 {
			mask |= 1 << i
		}
	}
	b.WriteU8(mask)
	for i := range bytes {
		if mask&(1<<i) != 0 {
			b.WriteU8(bytes[i])
		}
	}
}

func (b *Buffer) read(size int) ([]byte, error) {
	if size < 0 || b.pos+size > len(b.data) {
		return nil, io.ErrUnexpectedEOF
	}
	data := b.data[b.pos : b.pos+size]
	b.pos += size
	return data, nil
}

func (b *Buffer) Read(size int) ([]byte, error) { return b.read(size) }
func (b *Buffer) ReadBool() (bool, error)       { value, err := b.ReadU8(); return value != 0, err }
func (b *Buffer) ReadU8() (uint8, error) {
	data, err := b.read(1)
	if err != nil {
		return 0, err
	}
	return data[0], nil
}
func (b *Buffer) ReadU16() (uint16, error) {
	data, err := b.read(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(data), nil
}
func (b *Buffer) ReadU32() (uint32, error) {
	data, err := b.read(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}
func (b *Buffer) ReadU64() (uint64, error) {
	data, err := b.read(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(data), nil
}
func (b *Buffer) ReadI8() (int8, error)   { value, err := b.ReadU8(); return int8(value), err }
func (b *Buffer) ReadI16() (int16, error) { value, err := b.ReadU16(); return int16(value), err }
func (b *Buffer) ReadI32() (int32, error) { value, err := b.ReadU32(); return int32(value), err }
func (b *Buffer) ReadI64() (int64, error) { value, err := b.ReadU64(); return int64(value), err }
func (b *Buffer) ReadF32() (float32, error) {
	value, err := b.ReadU32()
	return math.Float32frombits(value), err
}
func (b *Buffer) ReadF64() (float64, error) {
	value, err := b.ReadU64()
	return math.Float64frombits(value), err
}
func (b *Buffer) ReadString(size int) (string, error) {
	data, err := b.read(size)
	return string(data), err
}

func (b *Buffer) ReadCString() (string, error) {
	for i := b.pos; i < len(b.data); i++ {
		if b.data[i] == 0 {
			value := string(b.data[b.pos:i])
			b.pos = i + 1
			return value, nil
		}
	}
	return "", io.ErrUnexpectedEOF
}

func (b *Buffer) ReadPackedGUID() (uint64, error) {
	mask, err := b.ReadU8()
	if err != nil {
		return 0, err
	}
	var value uint64
	for i := 0; i < 8; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		part, err := b.ReadU8()
		if err != nil {
			return 0, err
		}
		value |= uint64(part) << (8 * i)
	}
	return value, nil
}

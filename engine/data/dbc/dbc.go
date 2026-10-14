package dbc

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
)

const headerSize = 20

type File struct {
	RecordCount     uint32
	FieldCount      uint32
	RecordSize      uint32
	StringBlockSize uint32
	data            []byte
	strings         []byte
	indexOnce       sync.Once
	index           map[uint32]Record
}

type Record struct {
	data    []byte
	strings []byte
	fields  uint32
}

func Open(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*File, error) {
	// C++ owners: the map_extractor's DBCFile::open
	// (src/tools/map_extractor/dbcfile.cpp) and the server's DBCFileLoader
	// (src/common/DataStores/DBCFileLoader.cpp). Both read the WDBC magic,
	// the four header words, then recordSize*recordCount+stringSize bytes
	// with the string table laid out directly after the records.
	// DELTAS: the extractor's DBCFile::open rejects recordSize !=
	// fieldCount*4 outright; the server's DBCFileLoader imposes no such
	// gate (it strides by recordSize and offsets fields from the format
	// string). Parse is shared by both paths, so it keeps the tolerant
	// union: it rejects only recordSize > fieldCount*4 and the zero-size
	// degenerate case. Real client DBCs always satisfy the strict
	// equality, so the leniency is unreachable on real files.
	// Record.String additionally requires NUL termination inside the
	// string block where C++ getString returns the raw pointer (and would
	// over-read); Go-safer, same on real files.
	if len(data) < headerSize || string(data[:4]) != "WDBC" {
		return nil, errors.New("invalid DBC header")
	}
	file := &File{RecordCount: binary.LittleEndian.Uint32(data[4:8]), FieldCount: binary.LittleEndian.Uint32(data[8:12]), RecordSize: binary.LittleEndian.Uint32(data[12:16]), StringBlockSize: binary.LittleEndian.Uint32(data[16:20])}
	if file.RecordSize > file.FieldCount*4 || (file.FieldCount > 0 && file.RecordSize == 0) {
		return nil, errors.New("invalid record size")
	}
	recordBytes := uint64(file.RecordCount) * uint64(file.RecordSize)
	end := uint64(headerSize) + recordBytes + uint64(file.StringBlockSize)
	if end > uint64(len(data)) {
		return nil, errors.New("DBC data is truncated")
	}
	file.data = data[headerSize : headerSize+int(recordBytes)]
	stringsStart := headerSize + int(recordBytes)
	file.strings = data[stringsStart : stringsStart+int(file.StringBlockSize)]
	return file, nil
}

func (f *File) Records() int {
	return int(f.RecordCount)
}

func (f *File) Record(index int) (Record, error) {
	if index < 0 || index >= int(f.RecordCount) {
		return Record{}, errors.New("DBC record index out of range")
	}
	start := index * int(f.RecordSize)
	return Record{data: f.data[start : start+int(f.RecordSize)], strings: f.strings, fields: f.FieldCount}, nil
}

func (f *File) Find(id uint32) (Record, bool) {
	f.indexOnce.Do(func() {
		f.index = make(map[uint32]Record, f.RecordCount)
		for index := 0; index < int(f.RecordCount); index++ {
			record, err := f.Record(index)
			if err == nil {
				f.index[record.Uint32Unchecked(0)] = record
			}
		}
	})
	record, ok := f.index[id]
	return record, ok
}

func (r Record) Data() []byte {
	return r.data
}

func (r Record) ByteAt(offset int) (uint8, error) {
	if offset < 0 || offset >= len(r.data) {
		return 0, errors.New("DBC byte offset out of range")
	}
	return r.data[offset], nil
}

func (r Record) Uint32At(offset int) (uint32, error) {
	if offset < 0 || offset+4 > len(r.data) {
		return 0, errors.New("DBC uint32 offset out of range")
	}
	return binary.LittleEndian.Uint32(r.data[offset : offset+4]), nil
}

func (r Record) Int32At(offset int) (int32, error) {
	val, err := r.Uint32At(offset)
	return int32(val), err
}

func (r Record) Bytes(field int) ([]byte, error) {
	if field < 0 || field >= int(r.fields) {
		return nil, errors.New("DBC field index out of range")
	}
	start := field * 4
	return r.data[start : start+4], nil
}

func (r Record) Uint32(field int) (uint32, error) {
	data, err := r.Bytes(field)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

func (r Record) Uint32Unchecked(field int) uint32 {
	return binary.LittleEndian.Uint32(r.data[field*4 : field*4+4])
}

func (r Record) Int32(field int) (int32, error) {
	value, err := r.Uint32(field)
	return int32(value), err
}

func (r Record) Float32(field int) (float32, error) {
	value, err := r.Uint32(field)
	return math.Float32frombits(value), err
}

func (r Record) String(field int) (string, error) {
	offset, err := r.Uint32(field)
	if err != nil {
		return "", err
	}
	if offset >= uint32(len(r.strings)) {
		return "", errors.New("DBC string offset out of range")
	}
	value := r.strings[offset:]
	if end := strings.IndexByte(string(value), 0); end >= 0 {
		return string(value[:end]), nil
	}
	return "", errors.New("DBC string is not terminated")
}

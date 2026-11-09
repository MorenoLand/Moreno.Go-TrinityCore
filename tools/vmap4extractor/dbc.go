// C++ owner: src/tools/vmap4_extractor/dbcfile.cpp + dbcfile.h
//   - DBCFile::open (WDBC magic, record/field counts, record size, string
//     size, whole-file read), DBCFile::getRecord, Record::getUInt /
//     getInt / getFloat / getString. The C++ reader loads through MPQFile
//     (first archive containing the file wins); here the caller supplies a
//     resolver with the same first-hit-wins semantics.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

type dbcFile struct {
	data        []byte
	recordSize  uint32
	recordCount uint32
	fieldCount  uint32
	stringSize  uint32
	stringTable []byte
}

func openDBC(data []byte) (*dbcFile, error) {
	if len(data) < 20 {
		return nil, errors.New("DBC file is too short for a header")
	}
	if string(data[:4]) != "WDBC" {
		return nil, errors.New("DBC file has a bad magic, expected WDBC")
	}
	f := &dbcFile{
		recordCount: binary.LittleEndian.Uint32(data[4:8]),
		fieldCount:  binary.LittleEndian.Uint32(data[8:12]),
		recordSize:  binary.LittleEndian.Uint32(data[12:16]),
		stringSize:  binary.LittleEndian.Uint32(data[16:20]),
	}
	// C++ owner: dbcfile.cpp asserts fieldCount*4 >= recordSize.
	if uint64(f.fieldCount)*4 < uint64(f.recordSize) {
		return nil, fmt.Errorf("DBC field count %d is too small for record size %d", f.fieldCount, f.recordSize)
	}
	need := 20 + uint64(f.recordSize)*uint64(f.recordCount) + uint64(f.stringSize)
	if uint64(len(data)) < need {
		return nil, fmt.Errorf("DBC file is truncated: need %d bytes, have %d", need, len(data))
	}
	f.data = data[20 : 20+uint64(f.recordSize)*uint64(f.recordCount)]
	f.stringTable = data[20+uint64(f.recordSize)*uint64(f.recordCount) : need]
	return f, nil
}

func (f *dbcFile) records() uint32 { return f.recordCount }

func (f *dbcFile) recordBase(id uint32) ([]byte, error) {
	if id >= f.recordCount {
		return nil, fmt.Errorf("DBC record %d is out of range (%d records)", id, f.recordCount)
	}
	base := uint64(id) * uint64(f.recordSize)
	return f.data[base : base+uint64(f.recordSize)], nil
}

func (f *dbcFile) getUInt(id, field uint32) (uint32, error) {
	if field >= f.fieldCount {
		return 0, fmt.Errorf("DBC field %d is out of range (%d fields)", field, f.fieldCount)
	}
	rec, err := f.recordBase(id)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(rec[field*4:]), nil
}

func (f *dbcFile) getFloat(id, field uint32) (float32, error) {
	v, err := f.getUInt(id, field)
	if err != nil {
		return 0, err
	}
	return float32FromBits(v), nil
}

func (f *dbcFile) getString(id, field uint32) (string, error) {
	off, err := f.getUInt(id, field)
	if err != nil {
		return "", err
	}
	if uint64(off) >= uint64(len(f.stringTable)) {
		return "", fmt.Errorf("DBC string offset %d is out of range", off)
	}
	end := off
	for end < uint32(len(f.stringTable)) && f.stringTable[end] != 0 {
		end++
	}
	return string(f.stringTable[off:end]), nil
}

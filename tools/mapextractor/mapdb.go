// MorenoCore Map Extractor DBC support: the minimal WDBC record reader the
// .map writer needs for Map.dbc / LiquidType.dbc, plus the client build
// reader.
//
// C++ owner: src/tools/map_extractor/System.cpp — ReadMapDBC (:208-238),
// ReadLiquidTypeTableDBC (:240-260), ReadBuild (:171-206), main() locale
// probe (:1118-1195); DBCFile in src/tools/map_extractor/dbcfile.h.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mpq"
)

// mapDBCEntry is one Map.dbc row (id, name), mirroring the C++ map_id
// struct (System.cpp:44-48).
type mapDBCEntry struct {
	ID   uint32
	Name string
}

// dbcFile is a parsed WDBC file: a fixed-size record table followed by a
// string block. String fields store a uint32 offset into the string block.
type dbcFile struct {
	recordCount int
	fieldCount  int
	recordSize  int
	records     []byte
	strings     []byte
}

func parseDBC(data []byte) (dbcFile, error) {
	if len(data) < 20 || string(data[0:4]) != "WDBC" {
		return dbcFile{}, errors.New("invalid DBC file format")
	}
	recordCount := int(binary.LittleEndian.Uint32(data[4:]))
	fieldCount := int(binary.LittleEndian.Uint32(data[8:]))
	recordSize := int(binary.LittleEndian.Uint32(data[12:]))
	stringBlockSize := int(binary.LittleEndian.Uint32(data[16:]))
	if recordCount < 0 || fieldCount <= 0 || recordSize <= 0 || stringBlockSize < 0 {
		return dbcFile{}, errors.New("invalid DBC header")
	}
	if len(data) < 20+recordCount*recordSize+stringBlockSize {
		return dbcFile{}, errors.New("truncated DBC file")
	}
	return dbcFile{
		recordCount: recordCount,
		fieldCount:  fieldCount,
		recordSize:  recordSize,
		records:     data[20 : 20+recordCount*recordSize],
		strings:     data[20+recordCount*recordSize : 20+recordCount*recordSize+stringBlockSize],
	}, nil
}

// uintField reads a uint32 field (DBCFile::getUInt).
func (d dbcFile) uintField(record, field int) (uint32, error) {
	if record < 0 || record >= d.recordCount || field < 0 || field >= d.fieldCount || (field+1)*4 > d.recordSize {
		return 0, fmt.Errorf("DBC field %d out of range for record %d", field, record)
	}
	off := record*d.recordSize + field*4
	return binary.LittleEndian.Uint32(d.records[off:]), nil
}

// stringField reads a string field (DBCFile::getString): a uint32 offset
// into the string block, NUL-terminated.
func (d dbcFile) stringField(record, field int) (string, error) {
	off, err := d.uintField(record, field)
	if err != nil {
		return "", err
	}
	if int(off) >= len(d.strings) {
		return "", fmt.Errorf("DBC string offset %d out of range for record %d", off, record)
	}
	end := int(off)
	for end < len(d.strings) && d.strings[end] != 0 {
		end++
	}
	return string(d.strings[off:end]), nil
}

// readMapDBC loads DBFilesClient\Map.dbc: field 0 is the map id, field 1
// the map name. C++ fatal-exits on a missing/invalid file or an overlong
// name; Go returns the error instead.
func readMapDBC(input string) ([]mapDBCEntry, error) {
	fmt.Print("Read Map.dbc file... ")
	data, err := readClientAsset(input, `DBFilesClient\Map.dbc`)
	if err != nil {
		return nil, fmt.Errorf("invalid Map.dbc file format: %w", err)
	}
	dbc, err := parseDBC(data)
	if err != nil {
		return nil, fmt.Errorf("invalid Map.dbc file format: %w", err)
	}
	maps := make([]mapDBCEntry, 0, dbc.recordCount)
	for x := 0; x < dbc.recordCount; x++ {
		id, err := dbc.uintField(x, 0)
		if err != nil {
			return nil, err
		}
		name, err := dbc.stringField(x, 1)
		if err != nil {
			return nil, err
		}
		if len(name) >= 64 {
			return nil, errors.New("map name too long")
		}
		maps = append(maps, mapDBCEntry{ID: id, Name: name})
	}
	fmt.Printf("Done! (%d maps loaded)\n", len(maps))
	return maps, nil
}

// readLiquidTypeDBC loads DBFilesClient\LiquidType.dbc into an id ->
// SoundBank map (DBCFile::getUInt(0) / getUInt(3), System.cpp:248-255).
func readLiquidTypeDBC(input string) (map[uint32]uint8, error) {
	fmt.Print("Read LiquidType.dbc file...")
	data, err := readClientAsset(input, `DBFilesClient\LiquidType.dbc`)
	if err != nil {
		return nil, fmt.Errorf("invalid LiquidType.dbc file format: %w", err)
	}
	dbc, err := parseDBC(data)
	if err != nil {
		return nil, fmt.Errorf("invalid LiquidType.dbc file format: %w", err)
	}
	liquidTypes := make(map[uint32]uint8, dbc.recordCount)
	for x := 0; x < dbc.recordCount; x++ {
		id, err := dbc.uintField(x, 0)
		if err != nil {
			return nil, err
		}
		soundBank, err := dbc.uintField(x, 3)
		if err != nil {
			return nil, err
		}
		liquidTypes[id] = uint8(soundBank)
	}
	fmt.Printf("Done! (%d LiquidTypes loaded)\n", len(liquidTypes))
	return liquidTypes, nil
}

// parseBuildNumber extracts the build from a component.wow-<lang>.txt
// file: the digits after version=" (ReadBuild, System.cpp:171-206).
// C++ atoi()s the string and fatal-exits when the result is <= 0.
func parseBuildNumber(text string) (uint32, error) {
	pos := strings.Index(text, `version="`)
	if pos < 0 {
		return 0, errors.New("invalid component.wow build file format")
	}
	start := pos + len(`version="`)
	end := strings.Index(text[start:], `"`)
	if end < 0 {
		return 0, errors.New("invalid component.wow build file format")
	}
	build := atoiPrefix(text[start : start+end])
	if build <= 0 {
		return 0, errors.New("invalid component.wow build file format")
	}
	return uint32(build), nil
}

// atoiPrefix mirrors C atoi: leading whitespace, an optional sign, then a
// run of decimal digits; 0 when no digits lead.
func atoiPrefix(s string) int {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\v' || s[i] == '\f' || s[i] == '\r') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	value := 0
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		value = value*10 + int(s[i]-'0')
		digits++
		i++
	}
	if digits == 0 {
		return 0
	}
	if neg {
		return -value
	}
	return value
}

// readClientBuild returns the client build from the first detected
// locale's component.wow-<lang>.txt (System.cpp main(): the first locale
// in langs[] probe order). With no locale MPQ on disk C++ prints "No
// locales detected" and extracts nothing; Go instead scans the archives
// for any component.wow-*.txt so a bare-MPQ input still yields .map
// files, and errors only when no build file exists anywhere.
func readClientBuild(input string) (uint32, error) {
	locales := mpq.DetectLocales(input)
	if len(locales) > 0 {
		data, err := readClientAsset(input, "component.wow-"+locales[0]+".txt")
		if err != nil {
			return 0, fmt.Errorf("not found component.wow-%s.txt file: %w", locales[0], err)
		}
		return parseBuildNumber(string(data))
	}
	archives, err := mpq.OrderedArchives(input)
	if err != nil {
		return 0, fmt.Errorf("no locales detected: cannot determine client build: %w", err)
	}
	for _, path := range archives {
		archive, openErr := mpq.Open(path)
		if openErr != nil {
			continue
		}
		files, listErr := archive.ListFiles()
		if listErr != nil {
			archive.Close()
			continue
		}
		for _, name := range files {
			if strings.HasPrefix(name, "component.wow-") && strings.HasSuffix(name, ".txt") {
				data, readErr := archive.ReadFile(name)
				archive.Close()
				if readErr != nil {
					return 0, readErr
				}
				return parseBuildNumber(string(data))
			}
		}
		archive.Close()
	}
	return 0, errors.New("no locales detected: cannot determine client build")
}

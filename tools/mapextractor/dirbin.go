package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	modelFlagM2         uint32 = 1
	modelFlagWorldSpawn uint32 = 1 << 1
	modelFlagHasBound   uint32 = 1 << 2
)

// modelGatePass mirrors the C++ model gate: Doodad::Extract
// (vmap4_extractor/model.cpp:143-158) and MapObject::Extract
// (vmap4_extractor/wmo.cpp:519-537) open "<szWorkDirWmo>/<name>"
// ("./Buildings") and write no dir record when the file is missing or its
// int32 vertex count at offset 8 reads as zero. With an explicit -model-dir
// (converted VMAP047 models, not raw client files) the equivalent signal is
// whether the metadata loader finds the model at all.
func modelGatePass(modelDir, name string) bool {
	if modelDir != "" {
		_, ok := findRawModel(modelDir, name)
		return ok
	}
	return rawModelHasVertices("./Buildings", name)
}

// rawModelHasVertices opens the raw client model file and reports whether
// its int32 vertex count at offset 8 reads as nonzero, like the C++
// fopen + fseek(8) + fread arm.
func rawModelHasVertices(modelDir, name string) bool {
	f, err := os.Open(filepath.Join(modelDir, name))
	if err != nil {
		return false
	}
	defer f.Close()
	if _, err := f.Seek(8, io.SeekStart); err != nil {
		return false
	}
	var nVertices int32
	if err := binary.Read(f, binary.LittleEndian, &nVertices); err != nil {
		return false
	}
	return nVertices != 0
}

func buildADTDirBin(info adtInfo, mapID, tileX, tileY uint32) ([]byte, error) {
	return buildADTDirBinWithModelDir(info, mapID, tileX, tileY, "")
}

func buildADTDirBinWithModelDir(info adtInfo, mapID, tileX, tileY uint32, modelDir string) ([]byte, error) {
	ids := make(map[[2]uint32]uint32)
	nextID := uint32(1)
	uniqueID := func(clientID uint32, doodadID uint16) uint32 {
		key := [2]uint32{clientID, uint32(doodadID)}
		if value, ok := ids[key]; ok {
			return value
		}
		ids[key] = nextID
		nextID++
		return ids[key]
	}
	var output bytes.Buffer
	order := info.InstanceOrder
	if len(order) == 0 {
		order = make([]adtModelInstanceRef, 0, len(info.WorldModels)+len(info.Doodads))
		for index := range info.Doodads {
			order = append(order, adtModelInstanceRef{Doodad: true, Index: index})
		}
		for index := range info.WorldModels {
			order = append(order, adtModelInstanceRef{Index: index})
		}
	}
	for _, reference := range order {
		if reference.Doodad {
			if reference.Index < 0 || reference.Index >= len(info.Doodads) {
				return nil, fmt.Errorf("MDDF instance index %d is out of range", reference.Index)
			}
			instance := info.Doodads[reference.Index]
			if int(instance.NameID) >= len(info.DoodadNames) {
				return nil, fmt.Errorf("MDDF name id %d is out of range", instance.NameID)
			}
			name := plainModelName(info.DoodadNames[instance.NameID])
			if name == "" {
				return nil, fmt.Errorf("MDDF name id %d has an empty model name", instance.NameID)
			}
			if !modelGatePass(modelDir, name) {
				continue
			}
			writeDirRecord(&output, mapID, tileX, tileY, modelSpawn{Flags: modelFlagM2 | worldSpawnFlag(tileX, tileY), ID: uniqueID(instance.UniqueID, 0), Position: fixModelVector(instance.Position), Rotation: instance.Rotation, Scale: instance.Scale, Name: name})
			continue
		}
		if reference.Index < 0 || reference.Index >= len(info.WorldModels) {
			return nil, fmt.Errorf("MODF instance index %d is out of range", reference.Index)
		}
		instance := info.WorldModels[reference.Index]
		if instance.Flags&1 != 0 {
			continue
		}
		if int(instance.NameID) >= len(info.WorldModelNames) {
			return nil, fmt.Errorf("MODF name id %d is out of range", instance.NameID)
		}
		name := plainModelName(info.WorldModelNames[instance.NameID])
		if name == "" {
			return nil, fmt.Errorf("MODF name id %d has an empty model name", instance.NameID)
		}
		if !modelGatePass(modelDir, name) {
			continue
		}
		position := instance.Position
		if position[0] == 0 && position[2] == 0 {
			position[0], position[2] = 533.33333*32, 533.33333*32
		}
		writeDirRecord(&output, mapID, tileX, tileY, modelSpawn{Flags: modelFlagHasBound | worldSpawnFlag(tileX, tileY), ADTID: instance.NameSet, ID: uniqueID(instance.UniqueID, 0), Position: fixModelVector(position), Rotation: instance.Rotation, Scale: 1, HasBounds: true, BoundsMin: fixModelVector(instance.BoundsMin), BoundsMax: fixModelVector(instance.BoundsMax), Name: name})
		if err := appendWMODoodads(&output, modelDir, info.WorldModelNames[instance.NameID], instance, mapID, tileX, tileY, uniqueID); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func buildWDTDirBin(info wdtInfo, mapID uint32) ([]byte, error) {
	return buildWDTDirBinWithModelDir(info, mapID, "")
}

func buildWDTDirBinWithModelDir(info wdtInfo, mapID uint32, modelDir string) ([]byte, error) {
	order := make([]adtModelInstanceRef, len(info.GlobalWMOModels))
	for index := range order {
		order[index] = adtModelInstanceRef{Index: index}
	}
	return buildADTDirBinWithModelDir(adtInfo{WorldModels: info.GlobalWMOModels, WorldModelNames: info.GlobalWMOModelNames, InstanceOrder: order}, mapID, 65, 65, modelDir)
}

func appendWMODoodads(output *bytes.Buffer, modelDir, modelName string, instance adtWorldModelInstance, mapID, tileX, tileY uint32, uniqueID func(uint32, uint16) uint32) error {
	if modelDir == "" {
		return nil
	}
	metadata, found, err := loadWMODoodadMetadata(modelDir, modelName)
	if err != nil {
		return fmt.Errorf("read WMO doodad metadata for %q: %w", modelName, err)
	}
	if !found || int(instance.DoodadSet) >= len(metadata.Sets) {
		return nil
	}
	set := metadata.Sets[instance.DoodadSet]
	ordinal := uint16(0)
	for _, reference := range metadata.Refs {
		if uint32(reference) < set.StartIndex || uint32(reference) >= set.StartIndex+set.Count {
			continue
		}
		if int(reference) >= len(metadata.Doodads) {
			return fmt.Errorf("WMO doodad reference %d is out of range", reference)
		}
		doodad := metadata.Doodads[reference]
		name, ok := metadata.Names[doodad.NameIndex]
		if !ok || !modelHasGeometry(modelDir, name) {
			continue
		}
		ordinal++
		position, rotation := transformWMODoodad(instance, doodad)
		writeDirRecord(output, mapID, tileX, tileY, modelSpawn{Flags: modelFlagM2 | worldSpawnFlag(tileX, tileY), ID: uniqueID(instance.UniqueID, ordinal), Position: position, Rotation: rotation, Scale: doodad.Scale, Name: plainDoodadName(name)})
	}
	return nil
}

type modelSpawn struct {
	Flags                uint32
	ADTID                uint16
	ID                   uint32
	Position, Rotation   [3]float32
	Scale                float32
	HasBounds            bool
	BoundsMin, BoundsMax [3]float32
	Name                 string
}

func writeDirRecord(output *bytes.Buffer, mapID, tileX, tileY uint32, spawn modelSpawn) {
	_ = binary.Write(output, binary.LittleEndian, mapID)
	_ = binary.Write(output, binary.LittleEndian, tileX)
	_ = binary.Write(output, binary.LittleEndian, tileY)
	_ = binary.Write(output, binary.LittleEndian, spawn.Flags)
	_ = binary.Write(output, binary.LittleEndian, uint16(spawn.ADTID))
	_ = binary.Write(output, binary.LittleEndian, spawn.ID)
	_ = binary.Write(output, binary.LittleEndian, spawn.Position)
	_ = binary.Write(output, binary.LittleEndian, spawn.Rotation)
	_ = binary.Write(output, binary.LittleEndian, spawn.Scale)
	if spawn.HasBounds {
		_ = binary.Write(output, binary.LittleEndian, spawn.BoundsMin)
		_ = binary.Write(output, binary.LittleEndian, spawn.BoundsMax)
	}
	_ = binary.Write(output, binary.LittleEndian, uint32(len(spawn.Name)))
	output.WriteString(spawn.Name)
}

func worldSpawnFlag(tileX, tileY uint32) uint32 {
	if tileX == 65 && tileY == 65 {
		return modelFlagWorldSpawn
	}
	return 0
}

func fixModelVector(value [3]float32) [3]float32 {
	return [3]float32{value[2], value[0], value[1]}
}

func plainModelName(name string) string {
	if separator := strings.LastIndexAny(name, "\\/"); separator >= 0 {
		name = name[separator+1:]
	}
	if len(name) < 3 {
		return name
	}
	data := []byte(name)
	for index := 0; index < len(data)-3; index++ {
		previousAlpha := index > 0 && isASCIIAlpha(data[index-1])
		if previousAlpha && data[index] >= 'A' && data[index] <= 'Z' {
			data[index] |= 0x20
		} else if !previousAlpha && data[index] >= 'a' && data[index] <= 'z' {
			data[index] &^= 0x20
		}
	}
	// C++ owner: fixnamen (vmap4_extractor/adtfile.cpp:43-56) — the last 3
	// chars (extension) are unconditionally ORed with 0x20, not just A-Z.
	for index := len(data) - 3; index < len(data); index++ {
		data[index] |= 0x20
	}
	for index := 0; index < len(data)-3; index++ {
		if data[index] == ' ' {
			data[index] = '_'
		}
	}
	return string(data)
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

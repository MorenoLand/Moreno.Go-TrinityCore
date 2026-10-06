package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const (
	modelFlagM2         uint32 = 1
	modelFlagWorldSpawn uint32 = 1 << 1
	modelFlagHasBound   uint32 = 1 << 2
)

type mapSpawnRecord struct {
	Flags      uint32
	ADTID      uint16
	ID         uint32
	Position   vector3
	Rotation   vector3
	Scale      float32
	BoundsLow  vector3
	BoundsHigh vector3
	Name       string
}

type mapAssembly struct {
	Unique map[uint32]*mapSpawnRecord
	Tiles  map[uint32][]uint32
}

func packTileID(tileX, tileY uint32) uint32 { return tileX<<16 | tileY }

func readDirBin(path string) (map[uint32]*mapAssembly, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	maps := make(map[uint32]*mapAssembly)
	for {
		var mapID uint32
		err := binary.Read(file, binary.LittleEndian, &mapID)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		var tileX, tileY uint32
		if err := binary.Read(file, binary.LittleEndian, &tileX); err != nil {
			return nil, err
		}
		if err := binary.Read(file, binary.LittleEndian, &tileY); err != nil {
			return nil, err
		}
		spawn, err := readModelSpawn(file)
		if err != nil {
			return nil, err
		}
		current := maps[mapID]
		if current == nil {
			current = &mapAssembly{Unique: make(map[uint32]*mapSpawnRecord), Tiles: make(map[uint32][]uint32)}
			maps[mapID] = current
		}
		if _, exists := current.Unique[spawn.ID]; !exists {
			copySpawn := spawn
			current.Unique[spawn.ID] = &copySpawn
		}
		tile := packTileID(tileX, tileY)
		current.Tiles[tile] = append(current.Tiles[tile], spawn.ID)
	}
	return maps, nil
}

func readModelSpawn(reader io.Reader) (mapSpawnRecord, error) {
	var spawn mapSpawnRecord
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Flags); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.ADTID); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.ID); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Position); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Rotation); err != nil {
		return spawn, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &spawn.Scale); err != nil {
		return spawn, err
	}
	if spawn.Flags&modelFlagHasBound != 0 {
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsLow); err != nil {
			return spawn, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsHigh); err != nil {
			return spawn, err
		}
	}
	var nameLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil {
		return spawn, err
	}
	if nameLen > 500 {
		return spawn, errors.New("model spawn name is too long")
	}
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(reader, name); err != nil {
		return spawn, err
	}
	spawn.Name = string(name)
	return spawn, nil
}

func writeModelSpawn(writer io.Writer, spawn mapSpawnRecord) error {
	values := []any{spawn.Flags, spawn.ADTID, spawn.ID, spawn.Position, spawn.Rotation, spawn.Scale}
	for _, value := range values {
		if err := binary.Write(writer, binary.LittleEndian, value); err != nil {
			return err
		}
	}
	if spawn.Flags&modelFlagHasBound != 0 {
		if err := binary.Write(writer, binary.LittleEndian, spawn.BoundsLow); err != nil {
			return err
		}
		if err := binary.Write(writer, binary.LittleEndian, spawn.BoundsHigh); err != nil {
			return err
		}
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(spawn.Name))); err != nil {
		return err
	}
	_, err := io.WriteString(writer, spawn.Name)
	return err
}

func calculateM2Bound(srcDir string, spawn *mapSpawnRecord) error {
	data, err := os.ReadFile(filepath.Join(srcDir, spawn.Name))
	if err != nil {
		return err
	}
	raw, err := readRawModel(bytes.NewReader(data))
	if err != nil {
		return err
	}
	boundSet := false
	for _, group := range raw.Groups {
		for _, vertex := range group.Vertices {
			transformed := transformModelVertex(vertex, spawn.Rotation, spawn.Scale)
			if !boundSet {
				spawn.BoundsLow, spawn.BoundsHigh = transformed, transformed
				boundSet = true
			} else {
				spawn.BoundsLow = minVector(spawn.BoundsLow, transformed)
				spawn.BoundsHigh = maxVector(spawn.BoundsHigh, transformed)
			}
		}
	}
	if !boundSet {
		return fmt.Errorf("model %q has no geometry", spawn.Name)
	}
	spawn.BoundsLow = addVector(spawn.BoundsLow, spawn.Position)
	spawn.BoundsHigh = addVector(spawn.BoundsHigh, spawn.Position)
	spawn.Flags |= modelFlagHasBound
	return nil
}

func transformModelVertex(value, rotation vector3, scale float32) vector3 {
	x, y, z := float64(rotation.Z)*math.Pi/180, float64(rotation.X)*math.Pi/180, float64(rotation.Y)*math.Pi/180
	sx, cx := math.Sin(x), math.Cos(x)
	sy, cy := math.Sin(y), math.Cos(y)
	sz, cz := math.Sin(z), math.Cos(z)
	input := vector3{value.X * scale, value.Y * scale, value.Z * scale}
	return vector3{
		X: float32(cy*cz*float64(input.X) + (cz*sx*sy-cx*sz)*float64(input.Y) + (cx*cz*sy+sx*sz)*float64(input.Z)),
		Y: float32(cy*sz*float64(input.X) + (cx*cz+sx*sy*sz)*float64(input.Y) + (-cz*sx+cx*sy*sz)*float64(input.Z)),
		Z: float32(-sy*float64(input.X) + cy*sx*float64(input.Y) + cx*cy*float64(input.Z)),
	}
}

func addVector(a, b vector3) vector3 { return vector3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

func prepareMapSpawns(srcDir string, assembly *mapAssembly) error {
	for _, spawn := range assembly.Unique {
		if spawn.Flags&modelFlagM2 != 0 && spawn.Flags&modelFlagHasBound == 0 {
			if err := calculateM2Bound(srcDir, spawn); err != nil {
				return fmt.Errorf("calculate M2 bound for %s: %w", spawn.Name, err)
			}
		} else if spawn.Flags&modelFlagWorldSpawn != 0 && spawn.Flags&modelFlagHasBound != 0 {
			offset := vector3{533.33333 * 32, 533.33333 * 32, 0}
			spawn.BoundsLow = addVector(spawn.BoundsLow, offset)
			spawn.BoundsHigh = addVector(spawn.BoundsHigh, offset)
		}
	}
	return nil
}

func finiteVector(value vector3) bool {
	return !math.IsInf(float64(value.X), 0) && !math.IsNaN(float64(value.X)) &&
		!math.IsInf(float64(value.Y), 0) && !math.IsNaN(float64(value.Y)) &&
		!math.IsInf(float64(value.Z), 0) && !math.IsNaN(float64(value.Z))
}

func exportGameobjectModels(srcDir, destDir string) error {
	list, err := os.Open(filepath.Join(srcDir, "temp_gameobject_models"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer list.Close()
	ident := make([]byte, 8)
	if _, err := io.ReadFull(list, ident); err != nil || string(ident[:7]) != rawVMapMagic || ident[7] != 0 {
		return nil
	}
	out, err := os.Create(filepath.Join(destDir, "GameObjectModels.dtree"))
	if err != nil {
		return nil
	}
	defer out.Close()
	if _, err := io.WriteString(out, vMapMagic); err != nil {
		return err
	}
	for {
		var displayID uint32
		err := binary.Read(list, binary.LittleEndian, &displayID)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("temp_gameobject_models seems to be corrupted: %w", err)
		}
		var isWmo byte
		var nameLen uint32
		if err := binary.Read(list, binary.LittleEndian, &isWmo); err != nil {
			return fmt.Errorf("temp_gameobject_models seems to be corrupted: %w", err)
		}
		if err := binary.Read(list, binary.LittleEndian, &nameLen); err != nil || nameLen >= 500 {
			return fmt.Errorf("temp_gameobject_models seems to be corrupted: %w", err)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(list, name); err != nil {
			return fmt.Errorf("temp_gameobject_models seems to be corrupted: %w", err)
		}
		data, err := os.ReadFile(filepath.Join(srcDir, string(name)))
		if err != nil {
			continue
		}
		raw, err := readRawModel(bytes.NewReader(data))
		if err != nil {
			continue
		}
		boundSet := false
		var low, high vector3
		for _, group := range raw.Groups {
			for _, vertex := range group.Vertices {
				if !boundSet {
					low, high = vertex, vertex
					boundSet = true
				} else {
					low = minVector(low, vertex)
					high = maxVector(high, vertex)
				}
			}
		}
		if !boundSet || !finiteVector(low) || !finiteVector(high) {
			continue
		}
		if err := binary.Write(out, binary.LittleEndian, displayID); err != nil {
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, isWmo); err != nil {
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, nameLen); err != nil {
			return err
		}
		if _, err := out.Write(name); err != nil {
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, low); err != nil {
			return err
		}
		if err := binary.Write(out, binary.LittleEndian, high); err != nil {
			return err
		}
	}
	return nil
}

func assembleMapTrees(srcDir, destDir string) error {
	dirBin := filepath.Join(srcDir, "dir_bin")
	if _, err := os.Stat(dirBin); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	maps, err := readDirBin(dirBin)
	if err != nil {
		return err
	}
	mapIDs := make([]uint32, 0, len(maps))
	for mapID := range maps {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Slice(mapIDs, func(i, j int) bool { return mapIDs[i] < mapIDs[j] })
	for _, mapID := range mapIDs {
		if err := prepareMapSpawns(srcDir, maps[mapID]); err != nil {
			return err
		}
		if err := writeMapFiles(destDir, mapID, maps[mapID]); err != nil {
			return err
		}
	}
	return nil
}

func writeMapFiles(destDir string, mapID uint32, assembly *mapAssembly) error {
	ids := make([]uint32, 0, len(assembly.Unique))
	for id := range assembly.Unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	primitives := make([]bihPrimitive, len(ids))
	nodeIndex := make(map[uint32]uint32, len(ids))
	for index, id := range ids {
		spawn := assembly.Unique[id]
		primitives[index] = bihPrimitive{Low: spawn.BoundsLow, High: spawn.BoundsHigh, Index: uint32(index)}
		nodeIndex[id] = uint32(index)
	}
	var tree bytes.Buffer
	tree.WriteString(vMapMagic)
	global := assembly.Tiles[packTileID(65, 65)]
	if len(global) == 0 {
		tree.WriteByte(1)
	} else {
		tree.WriteByte(0)
	}
	tree.WriteString("NODE")
	if err := writeBIH(&tree, primitives); err != nil {
		return err
	}
	tree.WriteString("GOBJ")
	for _, id := range global {
		if err := writeModelSpawn(&tree, *assembly.Unique[id]); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(destDir, fmt.Sprintf("%03d.vmtree", mapID)), tree.Bytes(), 0644); err != nil {
		return err
	}
	keys := make([]uint32, 0, len(assembly.Tiles))
	for key := range assembly.Tiles {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, key := range keys {
		spawns := assembly.Tiles[key]
		if assembly.Unique[spawns[0]].Flags&modelFlagWorldSpawn != 0 {
			continue
		}
		tileX, tileY := key>>16, key&0xFF
		var tile bytes.Buffer
		tile.WriteString(vMapMagic)
		if err := binary.Write(&tile, binary.LittleEndian, uint32(len(spawns))); err != nil {
			return err
		}
		for _, id := range spawns {
			if err := writeModelSpawn(&tile, *assembly.Unique[id]); err != nil {
				return err
			}
			if err := binary.Write(&tile, binary.LittleEndian, nodeIndex[id]); err != nil {
				return err
			}
		}
		name := fmt.Sprintf("%03d_%02d_%02d.vmtile", mapID, tileX, tileY)
		if err := os.WriteFile(filepath.Join(destDir, name), tile.Bytes(), 0644); err != nil {
			return err
		}
	}
	return nil
}

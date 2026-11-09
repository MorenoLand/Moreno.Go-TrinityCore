// C++ owners: src/tools/vmap4_extractor/adtfile.cpp (ADTFile::init),
// wmo.cpp (MapObject::Extract), model.cpp (Doodad::Extract,
// Doodad::ExtractSet). Packed ADT record layouts come from adtfile.h.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	modFlagM2         uint32 = 1
	modFlagWorldSpawn uint32 = 1 << 1
	modFlagHasBound   uint32 = 1 << 2
)

// adtMDDF mirrors C++ ADT::MDDF (adtfile.h, packed, 36 bytes).
type adtMDDF struct {
	ID       uint32
	UniqueID uint32
	Position vec3
	Rotation vec3
	Scale    uint16
	Flags    uint16
}

// adtMODF mirrors C++ ADT::MODF (adtfile.h, packed, 64 bytes).
type adtMODF struct {
	ID        uint32
	UniqueID  uint32
	Position  vec3
	Rotation  vec3
	BoundsLo  vec3
	BoundsHi  vec3
	Flags     uint16
	DoodadSet uint16
	NameSet   uint16
	Scale     uint16
}

func parseMDDF(data []byte) (adtMDDF, error) {
	var d adtMDDF
	if len(data) < 36 {
		return d, fmt.Errorf("truncated MDDF record: %d bytes", len(data))
	}
	d.ID = binary.LittleEndian.Uint32(data[0:4])
	d.UniqueID = binary.LittleEndian.Uint32(data[4:8])
	d.Position = readVec3(data[8:20])
	d.Rotation = readVec3(data[20:32])
	d.Scale = binary.LittleEndian.Uint16(data[32:34])
	d.Flags = binary.LittleEndian.Uint16(data[34:36])
	return d, nil
}

func parseMODF(data []byte) (adtMODF, error) {
	var m adtMODF
	if len(data) < 64 {
		return m, fmt.Errorf("truncated MODF record: %d bytes", len(data))
	}
	m.ID = binary.LittleEndian.Uint32(data[0:4])
	m.UniqueID = binary.LittleEndian.Uint32(data[4:8])
	m.Position = readVec3(data[8:20])
	m.Rotation = readVec3(data[20:32])
	m.BoundsLo = readVec3(data[32:44])
	m.BoundsHi = readVec3(data[44:56])
	m.Flags = binary.LittleEndian.Uint16(data[56:58])
	m.DoodadSet = binary.LittleEndian.Uint16(data[58:60])
	m.NameSet = binary.LittleEndian.Uint16(data[60:62])
	m.Scale = binary.LittleEndian.Uint16(data[62:64])
	return m, nil
}

func readVec3(data []byte) vec3 {
	return vec3{
		mathFloat32(data[0:4]),
		mathFloat32(data[4:8]),
		mathFloat32(data[8:12]),
	}
}

// mapObjectExtract mirrors C++ MapObject::Extract (wmo.cpp). Destructible
// WMOs (Flags & 1) are skipped — the vmap handles those in the dynamic
// tree. The record is skipped entirely when the extracted WMO has no
// vertices: the C++ read of the .vmo header at offset 8 yields 0 and the
// function returns before writing.
func (e *extractor) mapObjectExtract(def adtMODF, wmoInstName string, mapID, tileX, tileY uint32) {
	if def.Flags&0x1 != 0 {
		return
	}
	if !e.wmoHasVertices(wmoInstName) {
		return
	}
	position := def.Position
	if position.X == 0 && position.Z == 0 {
		position.X = 533.33333 * 32
		position.Z = 533.33333 * 32
	}
	position = fixCoords(position)
	boundsLo := fixCoords(def.BoundsLo)
	boundsHi := fixCoords(def.BoundsHi)
	scale := float32(1.0)
	uniqueID := e.generateUniqueObjectID(def.UniqueID, 0)
	flags := modFlagHasBound
	if tileX == 65 && tileY == 65 {
		flags |= modFlagWorldSpawn
	}
	// C++ writes NameSet from the MODF record, then the unique id, the
	// fixed position, the RAW rotation, scale, fixed bounds, and name.
	_ = e.writeDirBinRecord(mapID, tileX, tileY, flags, def.NameSet, uniqueID, position, def.Rotation, scale, &boundsLo, &boundsHi, wmoInstName)
}

// wmoHasVertices mirrors the C++ MapObject::Extract / Doodad::Extract
// prologue: open Buildings/<name>, read the uint32 at offset 8 (the total
// triangle count patched by ExtractSingleWmo), and require it nonzero.
func (e *extractor) wmoHasVertices(wmoInstName string) bool {
	data, err := readOutputFile(e.output, wmoInstName)
	if err != nil || len(data) < 12 {
		return false
	}
	return binary.LittleEndian.Uint32(data[8:12]) != 0
}

// doodadExtract mirrors C++ Doodad::Extract (model.cpp): scale is
// Scale/1024, position is fixCoords, rotation is written RAW, and the
// record carries MOD_M2 (+ MOD_WORLDSPAWN on the 65,65 global tile).
func (e *extractor) doodadExtract(def adtMDDF, modelInstName string, mapID, tileX, tileY uint32) {
	if !e.wmoHasVertices(modelInstName) {
		return
	}
	// C++ owner: model.cpp — "scale factor - divide by 1024. blizzard
	// devs must be on crack, why not just use a float?"
	scale := float32(def.Scale) / 1024.0
	position := fixCoords(def.Position)
	uniqueID := e.generateUniqueObjectID(def.UniqueID, 0)
	flags := modFlagM2
	if tileX == 65 && tileY == 65 {
		flags |= modFlagWorldSpawn
	}
	_ = e.writeDirBinRecord(mapID, tileX, tileY, flags, 0, uniqueID, position, def.Rotation, scale, nil, nil, modelInstName)
}

// doodadExtractSet mirrors C++ Doodad::ExtractSet (model.cpp): for every
// doodad reference of the WMO instance that falls inside the instance's
// doodad set, the doodad is placed in world space by rotating its local
// offset with the WMO's rotation matrix and adding the WMO position, and
// its rotation is the doodad quaternion composed with the WMO rotation,
// converted back to Euler angles. G3D calls are transcribed in g3d.go.
func (e *extractor) doodadExtractSet(doodadData *wmoDoodadData, wmo adtMODF, mapID, tileX, tileY uint32) {
	if doodadData == nil {
		return
	}
	if int(wmo.DoodadSet) >= len(doodadData.Sets) {
		return
	}
	// G3D::Vector3 wmoPosition(wmo.Position.z, wmo.Position.x, wmo.Position.y)
	wmoPosition := vec3{wmo.Position.Z, wmo.Position.X, wmo.Position.Y}
	// G3D::Matrix3::fromEulerAnglesZYX(rot.y, rot.x, rot.z) in radians.
	wmoRotation := fromEulerAnglesZYX(toRadians(wmo.Rotation.Y), toRadians(wmo.Rotation.X), toRadians(wmo.Rotation.Z))
	setData := doodadData.Sets[wmo.DoodadSet]
	doodadID := uint16(0)
	// C++ iterates an unordered_set here, so its order is unspecified;
	// sorting the refs keeps Go output deterministic run to run. The
	// assembler keys spawns by unique id, so record order is immaterial
	// downstream either way.
	refs := make([]uint16, 0, len(doodadData.References))
	for ref := range doodadData.References {
		refs = append(refs, ref)
	}
	sortUint16(refs)
	for _, ref := range refs {
		if uint32(ref) < setData.StartIndex || uint32(ref) >= setData.StartIndex+setData.Count {
			continue
		}
		doodad := doodadData.Spawns[ref]
		modelInstName := fixedPlainName(doodadData.Paths[doodad.NameIndex])
		// C++ rewrites a trailing .mdx/.mdl to .m2 for the output name.
		if n := len(modelInstName); n > 3 {
			ext := modelInstName[n-4:]
			if ext == ".mdx" || ext == ".mdl" {
				modelInstName = modelInstName[:n-2] + "2"
			}
		}
		if !e.wmoHasVertices(modelInstName) {
			continue
		}
		doodadID++
		// position = wmoPosition + wmoRotation * doodad.Position
		position := addVec3(wmoPosition, wmoRotation.mulVec(vec3{doodad.Position[0], doodad.Position[1], doodad.Position[2]}))
		// rotation = (Quat(doodad.Rotation) * wmoRotation).toEulerAnglesXYZ
		// with the C++ (rotation.z, rotation.x, rotation.y) swizzle,
		// then degrees.
		rotMat := quat{doodad.Rotation[0], doodad.Rotation[1], doodad.Rotation[2], doodad.Rotation[3]}.toRotationMatrix().mul(wmoRotation)
		xAngle, yAngle, zAngle, _ := rotMat.toEulerAnglesXYZ()
		rotation := vec3{toDegrees(yAngle), toDegrees(zAngle), toDegrees(xAngle)}
		uniqueID := e.generateUniqueObjectID(wmo.UniqueID, doodadID)
		flags := modFlagM2
		if tileX == 65 && tileY == 65 {
			flags |= modFlagWorldSpawn
		}
		_ = e.writeDirBinRecord(mapID, tileX, tileY, flags, 0, uniqueID, position, rotation, doodad.Scale, nil, nil, modelInstName)
	}
}

// flipChunkName mirrors C++ flipcc (mpq_libmpq04.h) as used by the ADT
// and WDT readers: chunk ids are stored byte-reversed on disk, so the
// logical name is the reverse of the four stored bytes. (The WMO reader
// uses canonicalWMOChunk instead, which accepts either orientation.)
func flipChunkName(raw []byte) string {
	return string([]byte{raw[3], raw[2], raw[1], raw[0]})
}

// parseADT mirrors C++ ADTFile::init (adtfile.cpp): MMDX extracts the
// referenced models (recording fixed plain names), MWMO extracts the
// referenced WMOs, MDDF writes a Doodad record per placement, MODF writes
// a MapObject record plus the WMO's doodad set. The dir_bin file is
// appended to, opened once per run by the extractor.
func (e *extractor) parseADT(data []byte, mapID, tileX, tileY uint32) error {
	modelNames := make([]string, 0)
	wmoNames := make([]string, 0)
	for offset := 0; offset < len(data); {
		if len(data)-offset < 8 {
			break
		}
		name := flipChunkName(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if size < 0 || size > len(data)-offset {
			return fmt.Errorf("invalid ADT chunk %q size %d", name, size)
		}
		chunk := data[offset : offset+size]
		next := offset + size
		switch name {
		case "MMDX":
			for p := 0; p < len(chunk); {
				end := p
				for end < len(chunk) && chunk[end] != 0 {
					end++
				}
				path := string(chunk[p:end])
				// C++ fixnamens the full path, then takes the plain
				// name and fixname2's it.
				fixed := []byte(path)
				fixnamen(fixed)
				plain := []byte(plainName(string(fixed)))
				fixname2(plain)
				modelNames = append(modelNames, string(plain))
				e.extractSingleModel(path)
				p = end + 1
			}
		case "MWMO":
			for p := 0; p < len(chunk); {
				end := p
				for end < len(chunk) && chunk[end] != 0 {
					end++
				}
				path := string(chunk[p:end])
				plain := fixedPlainName(path)
				wmoNames = append(wmoNames, plain)
				e.extractSingleWmo(path)
				p = end + 1
			}
		case "MDDF":
			if size%36 != 0 {
				return fmt.Errorf("invalid MDDF chunk size %d", size)
			}
			for i := 0; i < size/36; i++ {
				def, err := parseMDDF(chunk[i*36:])
				if err != nil {
					return err
				}
				if int(def.ID) >= len(modelNames) {
					continue
				}
				e.doodadExtract(def, modelNames[def.ID], mapID, tileX, tileY)
			}
		case "MODF":
			if size%64 != 0 {
				return fmt.Errorf("invalid MODF chunk size %d", size)
			}
			for i := 0; i < size/64; i++ {
				def, err := parseMODF(chunk[i*64:])
				if err != nil {
					return err
				}
				if int(def.ID) >= len(wmoNames) {
					continue
				}
				name := wmoNames[def.ID]
				e.mapObjectExtract(def, name, mapID, tileX, tileY)
				e.doodadExtractSet(e.wmoDoodads[name], def, mapID, tileX, tileY)
			}
		}
		offset = next
	}
	return nil
}

// readOutputFile reads a file previously written under the output dir.
// Names are plain file names (no separators); the path stays flat under
// the output directory exactly like C++ Buildings/<name>.
func readOutputFile(output, name string) ([]byte, error) {
	return os.ReadFile(output + "/" + name)
}

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type rawWMODoodadSet struct {
	StartIndex uint32
	Count      uint32
}

type rawWMODoodad struct {
	NameIndex uint32
	Position  [3]float32
	Rotation  [4]float32
	Scale     float32
}

type rawWMODoodadMetadata struct {
	Names   map[uint32]string
	Sets    []rawWMODoodadSet
	Doodads []rawWMODoodad
	Refs    []uint16
}

func loadWMODoodadMetadata(modelDir, modelName string) (rawWMODoodadMetadata, bool, error) {
	var metadata rawWMODoodadMetadata
	path, ok := findRawModel(modelDir, modelName)
	if !ok {
		return metadata, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, true, err
	}
	marker := bytes.LastIndex(data, []byte("DODM"))
	if marker < 0 || marker+8 > len(data) {
		return metadata, true, nil
	}
	size := int(binary.LittleEndian.Uint32(data[marker+4:]))
	if size < 8 || marker+8+size > len(data) {
		return metadata, true, errors.New("truncated WMO doodad metadata")
	}
	reader := bytes.NewReader(data[marker+8 : marker+8+size])
	magic := make([]byte, 4)
	if _, err := reader.Read(magic); err != nil || string(magic) != "MCWM" {
		return metadata, true, errors.New("invalid WMO doodad metadata magic")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil || version != 1 {
		return metadata, true, errors.New("unsupported WMO doodad metadata version")
	}
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil || count > 1000000 {
		return metadata, true, errors.New("invalid WMO doodad name count")
	}
	metadata.Names = make(map[uint32]string, count)
	for index := uint32(0); index < count; index++ {
		var offset, length uint32
		if err := binary.Read(reader, binary.LittleEndian, &offset); err != nil || binary.Read(reader, binary.LittleEndian, &length) != nil || length > uint32(reader.Len()) {
			return metadata, true, errors.New("invalid WMO doodad name")
		}
		name := make([]byte, length)
		if _, err := reader.Read(name); err != nil {
			return metadata, true, err
		}
		metadata.Names[offset] = string(name)
	}
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil || count > 65535 {
		return metadata, true, errors.New("invalid WMO doodad set count")
	}
	metadata.Sets = make([]rawWMODoodadSet, count)
	for index := range metadata.Sets {
		if err := binary.Read(reader, binary.LittleEndian, &metadata.Sets[index]); err != nil {
			return metadata, true, err
		}
	}
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil || count > 10000000 {
		return metadata, true, errors.New("invalid WMO doodad count")
	}
	metadata.Doodads = make([]rawWMODoodad, count)
	for index := range metadata.Doodads {
		doodad := &metadata.Doodads[index]
		if err := binary.Read(reader, binary.LittleEndian, &doodad.NameIndex); err != nil || binary.Read(reader, binary.LittleEndian, &doodad.Position) != nil || binary.Read(reader, binary.LittleEndian, &doodad.Rotation) != nil || binary.Read(reader, binary.LittleEndian, &doodad.Scale) != nil {
			return metadata, true, errors.New("invalid WMO doodad record")
		}
		var color uint32
		if err := binary.Read(reader, binary.LittleEndian, &color); err != nil {
			return metadata, true, err
		}
	}
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil || count > 65535 {
		return metadata, true, errors.New("invalid WMO doodad reference count")
	}
	metadata.Refs = make([]uint16, count)
	if err := binary.Read(reader, binary.LittleEndian, &metadata.Refs); err != nil {
		return metadata, true, err
	}
	return metadata, true, nil
}

func findRawModel(modelDir, modelName string) (string, bool) {
	if modelDir == "" {
		return "", false
	}
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return "", false
	}
	wanted := map[string]struct{}{}
	base := filepath.Base(filepath.ToSlash(modelName))
	wanted[strings.ToLower(base)] = struct{}{}
	wanted[strings.ToLower(plainDoodadName(base))] = struct{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := wanted[strings.ToLower(entry.Name())]; ok {
			return filepath.Join(modelDir, entry.Name()), true
		}
	}
	return "", false
}

func modelHasGeometry(modelDir, modelName string) bool {
	path, ok := findRawModel(modelDir, modelName)
	if !ok {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && len(data) >= 12 && string(data[:8]) == "VMAP047\x00" && binary.LittleEndian.Uint32(data[8:]) > 0
}

func plainDoodadName(name string) string {
	name = plainModelName(name)
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".mdx" || ext == ".mdl" {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".m2"
	}
	return name
}

type doodadQuaternion struct{ X, Y, Z, W float64 }

func quaternionMultiply(a, b doodadQuaternion) doodadQuaternion {
	return doodadQuaternion{X: a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y, Y: a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X, Z: a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W, W: a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z}
}

func quaternionFromEulerZYX(z, y, x float64) doodadQuaternion {
	hx, hy, hz := x/2, y/2, z/2
	qx := doodadQuaternion{X: math.Sin(hx), W: math.Cos(hx)}
	qy := doodadQuaternion{Y: math.Sin(hy), W: math.Cos(hy)}
	qz := doodadQuaternion{Z: math.Sin(hz), W: math.Cos(hz)}
	return quaternionMultiply(quaternionMultiply(qz, qy), qx)
}

func rotateDoodadVector(q doodadQuaternion, value [3]float32) [3]float32 {
	v := doodadQuaternion{X: float64(value[0]), Y: float64(value[1]), Z: float64(value[2])}
	conjugate := doodadQuaternion{X: -q.X, Y: -q.Y, Z: -q.Z, W: q.W}
	rotated := quaternionMultiply(quaternionMultiply(q, v), conjugate)
	return [3]float32{float32(rotated.X), float32(rotated.Y), float32(rotated.Z)}
}

// doodadRotationEulerXYZ extracts G3D XYZ Euler angles from the composed
// WMO doodad rotation matrix and returns them in the C++ dir-record slot
// order (Y, Z, X) in degrees.
// C++ owner: (G3D::Quat(doodad.quat).toRotationMatrix() * wmoRotation)
// .toEulerAnglesXYZ(rotation.z, rotation.x, rotation.y)
// (vmap4_extractor/model.cpp:231-236) with G3D::Matrix3::toEulerAnglesXYZ
// (dep/g3dlite/source/Matrix3.cpp:1383-1409) and G3D::Matrix3(const Quat&)
// (Matrix3.cpp:122-142, Watt & Watt pg 362). The old Go code extracted ZYX
// Euler (roll, pitch, yaw), producing genuinely different triples — e.g. a
// pure Z 90° rotation wrote (0, 0, 90) where C++ writes (0, 90, 0).
func doodadRotationEulerXYZ(q doodadQuaternion) [3]float32 {
	n := math.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
	x, y, z, w := q.X/n, q.Y/n, q.Z/n, q.W/n
	xx, xy, xz, xw := 2*x*x, 2*x*y, 2*x*z, 2*x*w
	yy, yz, yw := 2*y*y, 2*y*z, 2*y*w
	zz, zw := 2*z*z, 2*z*w
	m00, m01, m02 := 1-yy-zz, xy-zw, xz+yw
	m10, m11, m12 := xy+zw, 1-xx-zz, yz-xw
	m22 := 1 - xx - yy
	var ex, ey, ez float64
	switch {
	case m02 <= -1:
		ex = -math.Atan2(m10, m11)
		ey = -math.Pi / 2
	case m02 >= 1:
		ex = math.Atan2(m10, m11)
		ey = math.Pi / 2
	default:
		ex = math.Atan2(-m12, m22)
		ey = math.Asin(m02)
		ez = math.Atan2(-m01, m00)
	}
	const rad2deg = 180 / math.Pi
	return [3]float32{float32(ey * rad2deg), float32(ez * rad2deg), float32(ex * rad2deg)}
}

func transformWMODoodad(instance adtWorldModelInstance, doodad rawWMODoodad) ([3]float32, [3]float32) {
	worldPosition := [3]float32{instance.Position[2], instance.Position[0], instance.Position[1]}
	worldRotation := quaternionFromEulerZYX(float64(instance.Rotation[1])*math.Pi/180, float64(instance.Rotation[0])*math.Pi/180, float64(instance.Rotation[2])*math.Pi/180)
	local := rotateDoodadVector(worldRotation, doodad.Position)
	position := [3]float32{worldPosition[0] + local[0], worldPosition[1] + local[1], worldPosition[2] + local[2]}
	doodadRotation := doodadQuaternion{X: float64(doodad.Rotation[0]), Y: float64(doodad.Rotation[1]), Z: float64(doodad.Rotation[2]), W: float64(doodad.Rotation[3])}
	return position, doodadRotationEulerXYZ(quaternionMultiply(doodadRotation, worldRotation))
}

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

func extractM2(data []byte) ([]byte, error) {
	if len(data) < 240 || (string(data[:4]) != "MD20" && string(data[:4]) != "MD21") {
		return nil, errors.New("unsupported M2 model header")
	}
	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset : offset+4]) }
	nIndices, indicesOffset := read(216), read(220)
	nVertices, verticesOffset := read(224), read(228)
	if nIndices == 0 || nIndices%3 != 0 || nVertices == 0 || nIndices > 100000000 || nVertices > 100000000 {
		return nil, errors.New("M2 bounding geometry is empty or unreasonable")
	}
	if uint64(indicesOffset)+uint64(nIndices)*2 > uint64(len(data)) || uint64(verticesOffset)+uint64(nVertices)*12 > uint64(len(data)) {
		return nil, errors.New("M2 bounding geometry is truncated")
	}
	indices := make([]uint16, nIndices)
	for index := range indices {
		indices[index] = binary.LittleEndian.Uint16(data[int(indicesOffset)+index*2:])
		if uint32(indices[index]) >= nVertices {
			return nil, errors.New("M2 bounding index is out of range")
		}
	}
	// C++ owner: model.cpp ConvertToVMAPModel swaps indices[i] and
	// indices[i+1] when (i%3)-1==0, i.e. i%3==1, in a separate pass AFTER
	// all indices are read. Swapping inside the read loop corrupts the
	// array (the i+1 slot is still unread), so the pass stays separate.
	for index := 0; index < len(indices); index++ {
		if index%3 == 1 && index+1 < len(indices) {
			indices[index], indices[index+1] = indices[index+1], indices[index]
		}
	}
	vertices := make([][3]float32, nVertices)
	for index := range vertices {
		base := int(verticesOffset) + index*12
		x := math.Float32frombits(binary.LittleEndian.Uint32(data[base:]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(data[base+4:]))
		z := math.Float32frombits(binary.LittleEndian.Uint32(data[base+8:]))
		// C++ owner: model.cpp applies fixCoordSystem (x, z, -y) at load
		// and then the ConvertToVMAPModel swap (y=-z, z=old y); the two
		// steps cancel out, so the written vertices are the raw ones.
		// Applying only the second step (x, -z, y) diverges from C++.
		v := fixCoordSystem(vec3{x, y, z})
		tmp := v.Y
		v.Y = -v.Z
		v.Z = tmp
		vertices[index] = [3]float32{v.X, v.Y, v.Z}
	}
	var output bytes.Buffer
	output.WriteString("VMAP047")
	output.WriteByte(0)
	_ = binary.Write(&output, binary.LittleEndian, nVertices)
	_ = binary.Write(&output, binary.LittleEndian, uint32(1))
	_ = binary.Write(&output, binary.LittleEndian, [3]uint32{})
	_ = binary.Write(&output, binary.LittleEndian, [6]float32{})
	_ = binary.Write(&output, binary.LittleEndian, uint32(0))
	writeRawChunk(&output, "GRP ", 8)
	_ = binary.Write(&output, binary.LittleEndian, uint32(1))
	_ = binary.Write(&output, binary.LittleEndian, uint32(0))
	writeRawChunk(&output, "INDX", 4+nIndices*2)
	_ = binary.Write(&output, binary.LittleEndian, nIndices)
	_ = binary.Write(&output, binary.LittleEndian, indices)
	writeRawChunk(&output, "VERT", 4+nVertices*12)
	_ = binary.Write(&output, binary.LittleEndian, nVertices)
	_ = binary.Write(&output, binary.LittleEndian, vertices)
	if output.Len() == 0 {
		return nil, fmt.Errorf("empty M2 output")
	}
	return output.Bytes(), nil
}

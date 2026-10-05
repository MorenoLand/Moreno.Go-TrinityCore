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
		if index%3 == 1 {
			indices[index], indices[index+1] = indices[index+1], indices[index]
		}
		if uint32(indices[index]) >= nVertices {
			return nil, errors.New("M2 bounding index is out of range")
		}
	}
	vertices := make([][3]float32, nVertices)
	for index := range vertices {
		base := int(verticesOffset) + index*12
		x := math.Float32frombits(binary.LittleEndian.Uint32(data[base:]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(data[base+4:]))
		z := math.Float32frombits(binary.LittleEndian.Uint32(data[base+8:]))
		vertices[index] = [3]float32{x, -z, y}
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

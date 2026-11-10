// IntermediateValues debug output port
// (src/tools/mmaps_generator/IntermediateValues.cpp/.h,
// src/tools/mmaps_generator/MapBuilder.cpp:924-935,
// src/tools/mmaps_generator/PathGenerator.cpp:206 --debugOutput):
// with --debugOutput the tool writes the Recast intermediates as binary
// dumps plus a .obj/.map/.mesh export of the input geometry, for use with
// RecastDemo. Files land in meshes/ relative to the working directory, as
// in C++ (no directory is created; a missing meshes/ fails the open with
// the C++ perror-style message).
package main

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/recast"
)

// intermediateValues gathers the debug info holding and output
// (IntermediateValues.h). Only polyMesh and polyMeshDetail are ever
// populated: TileBuilder::buildMoveMapTile assigns only the merged meshes
// (MapBuilder.cpp:754-774); heightfield, compactHeightfield and contours
// stay nil, so writeIV writes only .pmesh/.dmesh in this C++ version.
type intermediateValues struct {
	heightfield        *recast.Heightfield
	compactHeightfield *recast.CompactHeightfield
	contours           *recast.ContourSet
	polyMesh           *recast.PolyMesh
	polyMeshDetail     *recast.PolyMeshDetail
}

// writeIV writes the binary dumps (IntermediateValues.cpp:31-69): one file
// per non-nil intermediate, meshes/%03u%02i%02i.<ext> with (mapID, tileY,
// tileX), extension hf/chf/cs/pmesh/dmesh.
func (iv *intermediateValues) writeIV(mapID, tileX, tileY uint32) {
	tileString := fmt.Sprintf("[%02d,%02d]: ", tileX, tileY)
	fmt.Printf("%sWriting debug output...                       \r", tileString)

	name := fmt.Sprintf("meshes/%03d%02d%02d.", mapID, tileY, tileX)
	debugWrite := func(ext string, write func(*os.File)) {
		fileName := name + ext
		file, err := os.Create(fileName)
		if err != nil {
			// C++ perrors "Failed to open %s for writing!" here.
			fmt.Printf("%sFailed to open %s for writing!: %v\n", tileString, fileName, err)
		} else {
			write(file)
			file.Close()
		}
		fmt.Printf("%sWriting debug output...                       \r", tileString)
	}

	if iv.heightfield != nil {
		debugWrite("hf", func(f *os.File) { debugWriteHeightfield(f, iv.heightfield) })
	}
	if iv.compactHeightfield != nil {
		debugWrite("chf", func(f *os.File) { debugWriteCompactHeightfield(f, iv.compactHeightfield) })
	}
	if iv.contours != nil {
		debugWrite("cs", func(f *os.File) { debugWriteContourSet(f, iv.contours) })
	}
	if iv.polyMesh != nil {
		debugWrite("pmesh", func(f *os.File) { debugWritePolyMesh(f, iv.polyMesh) })
	}
	if iv.polyMeshDetail != nil {
		debugWrite("dmesh", func(f *os.File) { debugWritePolyMeshDetail(f, iv.polyMeshDetail) })
	}
}

// debugWriteHeightfield dumps an rcHeightfield (IntermediateValues.cpp:71-107):
// cs, ch, width, height, bmin, bmax, then per cell a span count followed by
// each span as a raw rcSpan. A 64-bit rcSpan is 16 bytes: one uint32 bitfield
// word (smin:16 | smax:16), the area byte, 3 pad bytes, and the 8-byte next
// pointer. Go has no heap pointer to write for the next slot, so it writes
// zeros there — the C++ pointer value is a heap address no reader can use.
func debugWriteHeightfield(file *os.File, mesh *recast.Heightfield) {
	if file == nil || mesh == nil {
		return
	}
	w := binary.LittleEndian
	write := func(v any) { _ = binary.Write(file, w, v) }
	write(mesh.Cs)
	write(mesh.Ch)
	write(int32(mesh.Width))
	write(int32(mesh.Height))
	write(mesh.Bmin)
	write(mesh.Bmax)
	for y := 0; y < mesh.Height; y++ {
		for x := 0; x < mesh.Width; x++ {
			spanCount := 0
			for span := mesh.Spans[x+y*mesh.Width]; span != nil; span = span.Next {
				spanCount++
			}
			write(int32(spanCount))
			for span := mesh.Spans[x+y*mesh.Width]; span != nil; span = span.Next {
				write(uint32(span.Smin) | uint32(span.Smax)<<16)
				write(span.Area)
				write([3]byte{})
				write([8]byte{})
			}
		}
	}
}

// debugWriteCompactHeightfield dumps an rcCompactHeightfield
// (IntermediateValues.cpp:109-146). rcCompactCell is one uint32
// (index:24 | count:8); rcCompactSpan is 8 bytes: y, reg, one uint32
// (con:24 | h:8).
func debugWriteCompactHeightfield(file *os.File, chf *recast.CompactHeightfield) {
	if file == nil || chf == nil {
		return
	}
	w := binary.LittleEndian
	write := func(v any) { _ = binary.Write(file, w, v) }
	write(int32(chf.Width))
	write(int32(chf.Height))
	write(int32(chf.SpanCount))
	write(int32(chf.WalkableHeight))
	write(int32(chf.WalkableClimb))
	write(chf.MaxDistance)
	write(chf.MaxRegions)
	write(chf.Bmin)
	write(chf.Bmax)
	write(chf.Cs)
	write(chf.Ch)
	tmp := int32(0)
	if chf.Cells != nil {
		tmp |= 1
	}
	if chf.Spans != nil {
		tmp |= 2
	}
	if chf.Dist != nil {
		tmp |= 4
	}
	if chf.Areas != nil {
		tmp |= 8
	}
	write(tmp)
	if chf.Cells != nil {
		for _, cell := range chf.Cells[:chf.Width*chf.Height] {
			write((cell.Index & 0xffffff) | (cell.Count << 24))
		}
	}
	if chf.Spans != nil {
		for _, span := range chf.Spans[:chf.SpanCount] {
			write(span.Y)
			write(span.Reg)
			write((span.Con & 0xffffff) | (uint32(span.H) << 24))
		}
	}
	if chf.Dist != nil {
		write(chf.Dist[:chf.SpanCount])
	}
	if chf.Areas != nil {
		write(chf.Areas[:chf.SpanCount])
	}
}

// debugWriteContourSet dumps an rcContourSet (IntermediateValues.cpp:148-167).
func debugWriteContourSet(file *os.File, cs *recast.ContourSet) {
	if file == nil || cs == nil {
		return
	}
	w := binary.LittleEndian
	write := func(v any) { _ = binary.Write(file, w, v) }
	write(cs.Cs)
	write(cs.Ch)
	write(cs.Bmin)
	write(cs.Bmax)
	write(int32(cs.Nconts))
	for i := 0; i < cs.Nconts; i++ {
		cont := cs.Conts[i]
		write(cont.Area)
		write(cont.Reg)
		write(int32(cont.Nverts))
		write(cont.Verts[:cont.Nverts*4])
		write(int32(cont.Nrverts))
		write(cont.Rverts[:cont.Nrverts*4])
	}
}

// debugWritePolyMesh dumps an rcPolyMesh (IntermediateValues.cpp:169-186).
func debugWritePolyMesh(file *os.File, mesh *recast.PolyMesh) {
	if file == nil || mesh == nil {
		return
	}
	w := binary.LittleEndian
	write := func(v any) { _ = binary.Write(file, w, v) }
	write(mesh.Cs)
	write(mesh.Ch)
	write(int32(mesh.Nvp))
	write(mesh.Bmin)
	write(mesh.Bmax)
	write(int32(mesh.Nverts))
	write(mesh.Verts[:mesh.Nverts*3])
	write(int32(mesh.Npolys))
	write(mesh.Polys[:mesh.Npolys*mesh.Nvp*2])
	write(mesh.Flags[:mesh.Npolys])
	write(mesh.Areas[:mesh.Npolys])
	write(mesh.Regs[:mesh.Npolys])
}

// debugWritePolyMeshDetail dumps an rcPolyMeshDetail
// (IntermediateValues.cpp:188-199).
func debugWritePolyMeshDetail(file *os.File, mesh *recast.PolyMeshDetail) {
	if file == nil || mesh == nil {
		return
	}
	w := binary.LittleEndian
	write := func(v any) { _ = binary.Write(file, w, v) }
	write(int32(mesh.Nverts))
	write(mesh.Verts[:mesh.Nverts*3])
	write(int32(mesh.Ntris))
	write(mesh.Tris[:mesh.Ntris*4])
	write(int32(mesh.Nmeshes))
	write(mesh.Meshes[:mesh.Nmeshes*4])
}

// generateObjFile writes the geometry export (IntermediateValues.cpp:201-275):
// meshes/map%03u%02u%02u.obj (mapID, tileY, tileX) with the liquid tris
// first and the solid tris offset by the liquid vert count
// (TerrainBuilder::copyIndices), meshes/%03u.map holding a single NUL byte,
// and meshes/%03u%02u%02u.mesh with vertCount, verts, triCount, tris.
func (iv *intermediateValues) generateObjFile(mapID, tileX, tileY uint32, md *MeshData) {
	objFileName := fmt.Sprintf("meshes/map%03d%02d%02d.obj", mapID, tileY, tileX)
	objFile, err := os.Create(objFileName)
	if err != nil {
		fmt.Printf("Failed to open %s for writing!: %v\n", objFileName, err)
		return
	}

	allVerts := append(append([]float32{}, md.LiquidVerts...), md.SolidVerts...)
	allTris := append([]int32{}, md.LiquidTris...)
	liquidVertCount := len(md.LiquidVerts) / 3
	for _, t := range md.SolidTris {
		allTris = append(allTris, t+int32(liquidVertCount))
	}
	vertCount := len(allVerts) / 3
	triCount := len(allTris) / 3

	for i := 0; i < vertCount; i++ {
		fmt.Fprintf(objFile, "v %f %f %f\n", allVerts[i*3], allVerts[i*3+1], allVerts[i*3+2])
	}
	for i := 0; i < triCount; i++ {
		fmt.Fprintf(objFile, "f %d %d %d\n", allTris[i*3]+1, allTris[i*3+1]+1, allTris[i*3+2]+1)
	}
	objFile.Close()

	tileString := fmt.Sprintf("[%02d,%02d]: ", tileY, tileX)
	fmt.Printf("%sWriting debug output...                       \r", tileString)

	mapFileName := fmt.Sprintf("meshes/%03d.map", mapID)
	mapFile, err := os.Create(mapFileName)
	if err != nil {
		fmt.Printf("Failed to open %s for writing!: %v\n", mapFileName, err)
		return
	}
	mapFile.Write([]byte{0})
	mapFile.Close()

	meshFileName := fmt.Sprintf("meshes/%03d%02d%02d.mesh", mapID, tileY, tileX)
	meshFile, err := os.Create(meshFileName)
	if err != nil {
		fmt.Printf("Failed to open %s for writing!: %v\n", meshFileName, err)
		return
	}
	w := binary.LittleEndian
	_ = binary.Write(meshFile, w, int32(vertCount))
	_ = binary.Write(meshFile, w, allVerts)
	_ = binary.Write(meshFile, w, int32(triCount))
	_ = binary.Write(meshFile, w, allTris)
	meshFile.Close()
}

// writeDebugOutput runs the MapBuilder.cpp:924-935 debug block: restore the
// border padding on the merged polymesh verts (stored in sub-tile space,
// v[0]/v[2] += borderSize, so the debug visualization is correct) and write
// the IntermediateValues dump. The caller runs this after the .mmtile
// write, so mutating the merged verts here cannot affect the tile output.
// A nil merged mesh is skipped: C++ dereferences the null iv.polyMesh if
// the build never reached the merge, which is a crash, not a dump.
func writeDebugOutput(mapID, tileX, tileY uint32, borderSize int, pmesh *recast.PolyMesh, dmesh *recast.PolyMeshDetail, md *MeshData) {
	if pmesh == nil {
		return
	}
	for i := 0; i < pmesh.Nverts; i++ {
		pmesh.Verts[i*3] += uint16(borderSize)
		pmesh.Verts[i*3+2] += uint16(borderSize)
	}
	iv := &intermediateValues{polyMesh: pmesh, polyMeshDetail: dmesh}
	iv.generateObjFile(mapID, tileX, tileY, md)
	iv.writeIV(mapID, tileX, tileY)
}

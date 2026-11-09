// MapBuilder orchestration port (src/tools/mmaps_generator/MapBuilder.cpp/.h,
// src/common/Collision/Maps/MapDefines.h): tile config, map-specific Recast
// config, tile bounds, skip rules, dtNavMeshParams/.mmap header writing,
// .mmtile header, and the stage-1 tile build (heightfield through distance
// field — the Recast pipeline in TileBuilder::buildMoveMapTile up to
// rcBuildDistanceField).
//
// Stages NOT ported yet: rcBuildRegions, rcBuildContours, rcBuildPolyMesh,
// rcBuildPolyMeshDetail, rcMergePolyMesh(es/Details), dtCreateNavMeshData and
// the .mmtile write. Those need RecastRegion/Contour/Mesh/MeshDetail plus
// DetourNavMeshBuilder (~6,500 C++ lines) and are the next unit.
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/recast"
)

// Nav area ids (src/common/Collision/Maps/MapDefines.h NavArea).
const (
	navAreaEmpty       = 0
	navAreaGround      = 11
	navAreaGroundSteep = 10
	navAreaWater       = 9
	navAreaMagmaSlime  = 8
	navAreaMaxValue    = navAreaGround
	navAreaMinValue    = navAreaMagmaSlime
	navAreaAllMask     = 0x3F
)

// Nav terrain flags (MapDefines.h NavTerrainFlag).
const (
	navGround      = 1 << (navAreaMaxValue - navAreaGround)
	navGroundSteep = 1 << (navAreaMaxValue - navAreaGroundSteep)
	navWater       = 1 << (navAreaMaxValue - navAreaWater)
	navMagmaSlime  = 1 << (navAreaMaxValue - navAreaMagmaSlime)
)

// Detour constants (dep/recastnavigation/Detour/Include/DetourNavMesh.h).
const (
	dtPolyBits        = 31
	dtVertsPerPolygon = 6
	dtNavMeshVersion  = 7
	mmapMagic         = 0x4d4d4150 // 'MMAP'
	mmapVersion       = 13
)

// MmapTileHeader mirrors MMAP::MmapTileHeader (MapDefines.h): 20 bytes,
// written ahead of the Detour tile data in every .mmtile file.
type MmapTileHeader struct {
	MmapMagic   uint32
	DtVersion   uint32
	MmapVersion uint32
	Size        uint32
	UsesLiquids byte
	Padding     [3]byte
}

func newMmapTileHeader(navDataSize uint32, usesLiquids bool) MmapTileHeader {
	h := MmapTileHeader{
		MmapMagic:   mmapMagic,
		DtVersion:   dtNavMeshVersion,
		MmapVersion: mmapVersion,
		Size:        navDataSize,
	}
	if usesLiquids {
		h.UsesLiquids = 1
	}
	return h
}

// Marshal returns the 20-byte little-endian header.
func (h MmapTileHeader) Marshal() []byte {
	buf := make([]byte, 20)
	binary.LittleEndian.PutUint32(buf[0:4], h.MmapMagic)
	binary.LittleEndian.PutUint32(buf[4:8], h.DtVersion)
	binary.LittleEndian.PutUint32(buf[8:12], h.MmapVersion)
	binary.LittleEndian.PutUint32(buf[12:16], h.Size)
	buf[16] = h.UsesLiquids
	return buf
}

// NavMeshParams mirrors dtNavMeshParams (DetourNavMesh.h): 28 bytes, the
// entire contents of a %03u.mmap file as written by MapBuilder::buildNavMesh.
type NavMeshParams struct {
	TileWidth  float32
	TileHeight float32
	Orig       [3]float32
	MaxTiles   uint32
	MaxPolys   uint32
}

// Marshal returns the 28-byte little-endian params.
func (p NavMeshParams) Marshal() []byte {
	buf := make([]byte, 28)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(p.TileWidth))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(p.TileHeight))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(p.Orig[0]))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(p.Orig[1]))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(p.Orig[2]))
	binary.LittleEndian.PutUint32(buf[20:24], p.MaxTiles)
	binary.LittleEndian.PutUint32(buf[24:28], p.MaxPolys)
	return buf
}

// TileConfig mirrors MMAP::TileConfig (MapBuilder.h).
type TileConfig struct {
	BaseUnitDim   float32
	VertexPerMap  int
	VertexPerTile int
	TilesPerMap   int
}

func newTileConfig(bigBaseUnit bool) TileConfig {
	var tc TileConfig
	if bigBaseUnit {
		tc.BaseUnitDim = 0.5333333
	} else {
		tc.BaseUnitDim = 0.2666666
	}
	// These are WORLD UNIT based metrics; values have to divide
	// GRID_SIZE (533.3333).
	tc.VertexPerMap = int(moveMapGridSize/tc.BaseUnitDim + 0.5)
	if bigBaseUnit {
		tc.VertexPerTile = 40
	} else {
		tc.VertexPerTile = 80
	}
	tc.TilesPerMap = tc.VertexPerMap / tc.VertexPerTile
	return tc
}

// GetMapSpecificConfig mirrors MapBuilder::GetMapSpecificConfig.
func getMapSpecificConfig(mapID uint32, bmin, bmax [3]float32, tileConfig TileConfig,
	bigBaseUnit bool, maxWalkableAngle, maxWalkableAngleNotSteep *float32) recast.Config {
	var config recast.Config
	config.Bmin = bmin
	config.Bmax = bmax

	config.MaxVertsPerPoly = dtVertsPerPolygon
	config.Cs = tileConfig.BaseUnitDim
	config.Ch = tileConfig.BaseUnitDim
	// Keeping these 2 slope angles the same reduces a lot the number of polys.
	// 55 should be the minimum, maybe 70 is ok (keep in mind blink uses mmaps), 85 is too much for players.
	config.WalkableSlopeAngle = 55
	if maxWalkableAngle != nil {
		config.WalkableSlopeAngle = *maxWalkableAngle
	}
	config.WalkableSlopeAngleNotSteep = 55
	if maxWalkableAngleNotSteep != nil {
		config.WalkableSlopeAngleNotSteep = *maxWalkableAngleNotSteep
	}
	config.TileSize = tileConfig.VertexPerTile
	if bigBaseUnit {
		config.WalkableRadius = 1
	} else {
		config.WalkableRadius = 2
	}
	config.BorderSize = config.WalkableRadius + 3
	config.MaxEdgeLen = tileConfig.VertexPerTile + 1 // anything bigger than tileSize
	if bigBaseUnit {
		config.WalkableHeight = 3
	} else {
		config.WalkableHeight = 6
	}
	// A value >= 3|6 allows npcs to walk over some fences.
	// A value >= 4|8 allows npcs to walk over all fences.
	if bigBaseUnit {
		config.WalkableClimb = 3
	} else {
		config.WalkableClimb = 6
	}
	config.MinRegionArea = 60 * 60
	config.MergeRegionArea = 50 * 50
	config.MaxSimplificationError = 1.8 // eliminates most jagged edges (tiny polygons)
	config.DetailSampleDist = config.Cs * 16
	config.DetailSampleMaxError = config.Ch * 1

	switch mapID {
	case 562: // Blade's Edge Arena: allows walking on the ropes to the pillars.
		config.WalkableRadius = 0
	case 48: // Blackfathom Deeps: reduces the chance of underground levels.
		config.Ch *= 2
	}

	return config
}

// fltMin is C++ FLT_MIN (smallest normalized float32, 0x00800000) —
// NOT Go's math.SmallestNonzeroFloat32, which is the smallest denormal.
const fltMin = float32(1.17549435e-38)

// getTileBounds mirrors MapBuilder::getTileBounds.
func getTileBounds(tileX, tileY uint32, verts []float32, vertCount int) (bmin, bmax [3]float32) {
	// This is for elevation.
	if verts != nil && vertCount > 0 {
		bmin, bmax = recast.CalcBounds(verts, vertCount)
	} else {
		bmin[1] = fltMin
		bmax[1] = math.MaxFloat32
	}

	// This is for width and depth.
	bmax[0] = float32(32-int32(tileX)) * moveMapGridSize
	bmax[2] = float32(32-int32(tileY)) * moveMapGridSize
	bmin[0] = bmax[0] - moveMapGridSize
	bmin[2] = bmax[2] - moveMapGridSize
	return bmin, bmax
}

func isTransportMap(mapID uint32) bool {
	switch mapID {
	case 582, 584, 586, 587, 588, 589, 590, 591, 592, 593, 594, 596,
		610, 612, 613, 614, 620, 621, 622, 623, 641, 642, 647, 672,
		673, 712, 713, 718:
		return true
	}
	return false
}

func isContinentMap(mapID uint32) bool {
	switch mapID {
	case 0, 1, 530, 571:
		return true
	}
	return false
}

// shouldSkipMap mirrors MapBuilder::shouldSkipMap.
func shouldSkipMap(mapID uint32, targetMap int, skipContinents, skipJunkMaps, skipBattlegrounds bool) bool {
	if targetMap >= 0 {
		return uint32(targetMap) != mapID
	}

	if skipContinents && isContinentMap(mapID) {
		return true
	}

	if skipJunkMaps {
		switch mapID {
		case 13, 25, 29, 42, 169, 451, 573, 597, 605, 606:
			return true
		default:
			if isTransportMap(mapID) {
				return true
			}
		}
	}

	if skipBattlegrounds {
		switch mapID {
		case 30, 37, 489, 529, 566, 607, 628:
			return true
		}
	}

	return false
}

// shouldSkipTile mirrors TileBuilder::shouldSkipTile: a tile is skipped when
// a valid .mmtile already exists for it.
func shouldSkipTile(mapID, tileX, tileY uint32, outputDir string) bool {
	fileName := filepath.Join(outputDir, fmt.Sprintf("%03d%02d%02d.mmtile", mapID, tileY, tileX))
	data, err := os.ReadFile(fileName)
	if err != nil || len(data) < 20 {
		return false
	}
	header := MmapTileHeader{
		MmapMagic:   binary.LittleEndian.Uint32(data[0:4]),
		DtVersion:   binary.LittleEndian.Uint32(data[4:8]),
		MmapVersion: binary.LittleEndian.Uint32(data[8:12]),
	}
	if header.MmapMagic != mmapMagic || header.DtVersion != uint32(dtNavMeshVersion) {
		return false
	}
	if header.MmapVersion != mmapVersion {
		return false
	}
	return true
}

// buildNavMeshParams mirrors MapBuilder::buildNavMesh (header half): computes
// the dtNavMeshParams for a map from its tile list. The caller writes the
// marshalled params as the %03u.mmap file.
func buildNavMeshParams(tiles []mapTile) NavMeshParams {
	var tileXMin, tileYMin uint32 = 64, 64
	var tileXMax, tileYMax uint32
	for _, tile := range tiles {
		if tile.TileX > tileXMax {
			tileXMax = tile.TileX
		} else if tile.TileX < tileXMin {
			tileXMin = tile.TileX
		}
		if tile.TileY > tileYMax {
			tileYMax = tile.TileY
		} else if tile.TileY < tileYMin {
			tileYMin = tile.TileY
		}
	}

	// Use Max because '32 - tileX' is negative for values over 32.
	bmin, _ := getTileBounds(tileXMax, tileYMax, nil, 0)

	var params NavMeshParams
	params.TileWidth = moveMapGridSize
	params.TileHeight = moveMapGridSize
	params.Orig = bmin
	params.MaxTiles = uint32(len(tiles))
	params.MaxPolys = uint32(1) << dtPolyBits
	return params
}

// tileStage1Stats is the per-tile result of the stage-1 build.
type tileStage1Stats struct {
	SubTiles      int
	Spans         int
	WalkableSpans int
	MaxDistance   uint16
	Skipped       bool
	Empty         bool
}

// buildTileStage1 runs TileBuilder::buildMoveMapTile up to rcBuildDistanceField
// (the stages ported so far) for one map tile. It returns per-tile build
// statistics; the polymesh/detail/Detour stages are the next unit.
func buildTileStage1(mapID, tileX, tileY uint32, md *MeshData, bmin, bmax [3]float32,
	tileConfig TileConfig, bigBaseUnit bool, maxWalkableAngle, maxWalkableAngleNotSteep *float32) tileStage1Stats {
	var stats tileStage1Stats

	tVerts := md.SolidVerts
	tTris := md.SolidTris
	tTriCount := len(md.SolidTris) / 3

	lVerts := md.LiquidVerts
	lTris := md.LiquidTris
	lTriFlags := md.LiquidType

	tilesPerMap := tileConfig.TilesPerMap
	config := getMapSpecificConfig(mapID, bmin, bmax, tileConfig, bigBaseUnit, maxWalkableAngle, maxWalkableAngleNotSteep)

	// This sets the dimensions of the heightfield.
	config.Width, config.Height = recast.CalcGridSize(config.Bmin, config.Bmax, config.Cs)

	// Initialize per tile config.
	tileCfg := config
	tileCfg.Width = config.TileSize + config.BorderSize*2
	tileCfg.Height = config.TileSize + config.BorderSize*2

	for y := 0; y < tilesPerMap; y++ {
		for x := 0; x < tilesPerMap; x++ {
			// Calculate the per tile bounding box.
			tileCfg.Bmin[0] = config.Bmin[0] + float32(x*config.TileSize-config.BorderSize)*config.Cs
			tileCfg.Bmin[2] = config.Bmin[2] + float32(y*config.TileSize-config.BorderSize)*config.Cs
			tileCfg.Bmax[0] = config.Bmin[0] + float32((x+1)*config.TileSize+config.BorderSize)*config.Cs
			tileCfg.Bmax[2] = config.Bmin[2] + float32((y+1)*config.TileSize+config.BorderSize)*config.Cs

			// Build heightfield.
			solid := recast.CreateHeightfield(tileCfg.Width, tileCfg.Height, tileCfg.Bmin, tileCfg.Bmax, tileCfg.Cs, tileCfg.Ch)

			// Mark all walkable triangles, both liquids and solids.
			// Triangles with slope less than walkableSlopeAngleNotSteep get
			// NAV_AREA_GROUND; slope between that and walkableSlopeAngle gets
			// NAV_AREA_GROUND_STEEP (pre-seeded below, then cleared above the
			// angle, then marked below the not-steep angle).
			triFlags := make([]uint8, tTriCount)
			for i := range triFlags {
				triFlags[i] = navAreaGroundSteep
			}
			recast.ClearUnwalkableTriangles(tileCfg.WalkableSlopeAngle, tVerts, tTris, triFlags)
			recast.MarkWalkableTriangles(tileCfg.WalkableSlopeAngleNotSteep, tVerts, tTris, triFlags, navAreaGround)
			recast.RasterizeTriangles(tVerts, tTris, triFlags, solid, config.WalkableClimb)

			recast.FilterLowHangingWalkableObstacles(config.WalkableClimb, solid)
			recast.FilterLedgeSpans(tileCfg.WalkableHeight, tileCfg.WalkableClimb, solid)
			recast.FilterWalkableLowHeightSpans(tileCfg.WalkableHeight, solid)

			// Add liquid triangles.
			recast.RasterizeTriangles(lVerts, lTris, lTriFlags, solid, config.WalkableClimb)

			// Compact heightfield spans.
			chf := recast.BuildCompactHeightfield(tileCfg.WalkableHeight, tileCfg.WalkableClimb, solid)

			// Build polymesh intermediates.
			recast.ErodeWalkableArea(config.WalkableRadius, chf)
			recast.MedianFilterWalkableArea(chf)
			recast.BuildDistanceField(chf)

			stats.SubTiles++
			stats.Spans += chf.SpanCount
			for _, a := range chf.Areas {
				if a != recast.NullArea {
					stats.WalkableSpans++
				}
			}
			if chf.MaxDistance > stats.MaxDistance {
				stats.MaxDistance = chf.MaxDistance
			}
		}
	}

	return stats
}

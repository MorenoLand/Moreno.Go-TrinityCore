// MorenoCore MoveMap Generator: compiles navigation mesh tiles from map and vmap data.
//
// Go port of TrinityCore's mmaps_generator (src/tools/mmaps_generator/:
// MapBuilder, TerrainBuilder, PathGenerator). Pipeline status:
//   - DONE: tile discovery (maps/ + vmaps/*.vmtile), dtNavMeshParams/.mmap
//     writing, .map terrain loading (TerrainBuilder::loadMap), vertex
//     cleaning, off-mesh connections, Recast stage 1 (heightfield,
//     rasterization, walkable marking/filters, compact heightfield, erosion,
//     median filter, distance field).
//   - DONE: loadVMap (StaticMapTree/.vmtree/.vmo reader), Recast stages 2-4
//     (regions, contours, polymesh, detail mesh, merged meshes),
//     dtCreateNavMeshData and the .mmtile write.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const moveMapGridSize float32 = 533.3333

const (
	mapFileHeaderSize = 44
	mapMagic          = "MAPS"
	mapVersionMagic   = "v1.9"
)

type mapTile struct {
	MapID uint32
	TileX uint32
	TileY uint32
	Path  string
}

func discoverMapTiles(dir string, targetMap int) (map[uint32][]mapTile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	groups := make(map[uint32][]mapTile)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".map") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(data) < mapFileHeaderSize {
			return nil, fmt.Errorf("map tile %s is shorter than the reference %d-byte header", path, mapFileHeaderSize)
		}
		if string(data[:4]) != mapMagic || string(data[4:8]) != mapVersionMagic {
			return nil, fmt.Errorf("map tile %s has unsupported magic/version", path)
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if len(base) != 7 {
			return nil, fmt.Errorf("map tile %s does not match %03d%02d%02d.map naming", path, 0, 0, 0)
		}
		mapID, err := strconv.ParseUint(base[:3], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("map tile %s has invalid map ID", path)
		}
		tileY, err := strconv.ParseUint(base[3:5], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("map tile %s has invalid tile Y", path)
		}
		tileX, err := strconv.ParseUint(base[5:], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("map tile %s has invalid tile X", path)
		}
		tile := mapTile{MapID: uint32(mapID), TileX: uint32(tileX), TileY: uint32(tileY), Path: path}
		if targetMap >= 0 && tile.MapID != uint32(targetMap) {
			continue
		}
		groups[tile.MapID] = append(groups[tile.MapID], tile)
	}
	return groups, nil
}

// discoverVMapTiles mirrors the vmaps half of MapBuilder::discoverTiles:
// tile ids come from "%03u_%02u_%02u.vmtile" names (mapID_tileY_tileX).
// Tiles already known from maps/ are not duplicated.
func discoverVMapTiles(dir string, targetMap int, groups map[uint32][]mapTile) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	seen := make(map[uint32]map[[2]uint32]bool)
	for mapID, tiles := range groups {
		m := make(map[[2]uint32]bool)
		for _, t := range tiles {
			m[[2]uint32{t.TileX, t.TileY}] = true
		}
		seen[mapID] = m
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".vmtile") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if len(base) != 10 || base[3] != '_' || base[6] != '_' {
			continue
		}
		mapID, err := strconv.ParseUint(base[:3], 10, 32)
		if err != nil {
			continue
		}
		tileY, err := strconv.ParseUint(base[4:6], 10, 32)
		if err != nil {
			continue
		}
		tileX, err := strconv.ParseUint(base[7:], 10, 32)
		if err != nil {
			continue
		}
		if targetMap >= 0 && int(mapID) != targetMap {
			continue
		}
		if seen[uint32(mapID)] == nil {
			seen[uint32(mapID)] = make(map[[2]uint32]bool)
		}
		key := [2]uint32{uint32(tileX), uint32(tileY)}
		if seen[uint32(mapID)][key] {
			continue
		}
		seen[uint32(mapID)][key] = true
		groups[uint32(mapID)] = append(groups[uint32(mapID)], mapTile{
			MapID: uint32(mapID), TileX: uint32(tileX), TileY: uint32(tileY),
			Path: filepath.Join(dir, entry.Name()),
		})
	}
	return nil
}

// writeNavMeshParams writes the %03u.mmap file for each map: the 28-byte
// dtNavMeshParams, mirroring MapBuilder::buildNavMesh.
func writeNavMeshParams(output string, groups map[uint32][]mapTile) (int, error) {
	mapIDs := make([]uint32, 0, len(groups))
	for mapID := range groups {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Slice(mapIDs, func(i, j int) bool { return mapIDs[i] < mapIDs[j] })
	for _, mapID := range mapIDs {
		params := buildNavMeshParams(groups[mapID])
		path := filepath.Join(output, fmt.Sprintf("%03d.mmap", mapID))
		if err := os.WriteFile(path, params.Marshal(), 0o644); err != nil {
			return 0, err
		}
	}
	return len(mapIDs), nil
}

func printBanner() {
	fmt.Println("==========================================================")
	fmt.Println(" MorenoCore MoveMap Generator (Go Parity Version)")
	fmt.Println(" Builds navigation mesh tiles from map and vmap data")
	fmt.Println("==========================================================")
}

type tileResult struct {
	tile  mapTile
	stats tileBuildStats
}

func main() {
	mapsDir := flag.String("maps", "maps", "Path to extracted map data directory")
	vmapsDir := flag.String("vmaps", "vmaps", "Path to compiled vmap data directory")
	outputDir := flag.String("output", "mmaps", "Output directory for compiled navigation mesh tiles")
	threads := flag.Int("threads", runtime.NumCPU(), "Number of concurrent generator worker threads")
	targetMap := flag.Int("map", -1, "Target map ID (-1 for all maps)")
	skipLiquid := flag.Bool("skipLiquid", false, "Skip liquid geometry calculation")
	offMeshFile := flag.String("offMeshFile", "", "Path to off-mesh connections file")
	bigBaseUnit := flag.Bool("bigBaseUnit", false, "Use big base unit (0.5333333) instead of 0.2666666")
	skipContinents := flag.Bool("skipContinents", false, "Skip continent maps (0, 1, 530, 571)")
	skipJunkMaps := flag.Bool("skipJunkMaps", true, "Skip junk/test/transport maps")
	skipBattlegrounds := flag.Bool("skipBattlegrounds", false, "Skip battleground maps")
	maxAngle := flag.Float64("maxAngle", -1, "Maximum walkable slope angle in degrees (default 55)")
	maxAngleNotSteep := flag.Float64("maxAngleNotSteep", -1, "Maximum non-steep walkable slope angle in degrees (default 55)")
	tileFlag := flag.String("tile", "", "Build a single tile as X,Y (requires -map)")
	debugOutput := flag.Bool("debugOutput", false, "Write RecastDemo debug files (meshes/*.hf/.chf/.cs/.pmesh/.dmesh/.obj/.mesh) like C++ --debugOutput")
	flag.Parse()

	printBanner()

	fmt.Printf("Settings: maps='%s', vmaps='%s', output='%s', threads=%d, skipLiquid=%v\n",
		*mapsDir, *vmapsDir, *outputDir, *threads, *skipLiquid)

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory '%s': %v\n", *outputDir, err)
		os.Exit(1)
	}

	start := time.Now()

	startMaps, err := discoverMapTiles(*mapsDir, *targetMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to inspect map geometry files: %v\n", err)
		os.Exit(1)
	}
	if err := discoverVMapTiles(*vmapsDir, *targetMap, startMaps); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to inspect vmap tile files: %v\n", err)
		os.Exit(1)
	}
	mapsFound := 0
	for _, tiles := range startMaps {
		mapsFound += len(tiles)
	}

	fmt.Printf("Located %d map geometry files in '%s'\n", mapsFound, *mapsDir)
	if *targetMap >= 0 {
		fmt.Printf("Generating MoveMap tiles for map %d...\n", *targetMap)
	} else {
		fmt.Println("Generating MoveMap tiles across all maps...")
	}

	if mapsFound == 0 {
		fmt.Println("No reference .map geometry files were found; no output was fabricated.")
		return
	}

	// Filter skipped maps (MapBuilder::shouldSkipMap) before writing headers.
	mapIDs := make([]uint32, 0, len(startMaps))
	for mapID := range startMaps {
		mapIDs = append(mapIDs, mapID)
	}
	sort.Slice(mapIDs, func(i, j int) bool { return mapIDs[i] < mapIDs[j] })
	kept := make(map[uint32][]mapTile)
	for _, mapID := range mapIDs {
		if shouldSkipMap(mapID, *targetMap, *skipContinents, *skipJunkMaps, *skipBattlegrounds) {
			continue
		}
		kept[mapID] = startMaps[mapID]
	}

	headers, err := writeNavMeshParams(*outputDir, kept)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write navigation headers: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d dtNavMeshParams (.mmap) headers.\n", headers)

	var maxWalkableAngle, maxWalkableAngleNotSteep *float32
	if *maxAngle >= 0 && *maxAngle <= 90 {
		v := float32(*maxAngle)
		maxWalkableAngle = &v
	}
	if *maxAngleNotSteep >= 0 && *maxAngleNotSteep <= 90 {
		v := float32(*maxAngleNotSteep)
		maxWalkableAngleNotSteep = &v
	}
	tileConfig := newTileConfig(*bigBaseUnit)

	// dtNavMeshParams orig per map (MapBuilder::buildNavMesh), needed for
	// the dtCreateNavMeshData tileX/tileY placement in buildMoveMapTile.
	navOrigs := make(map[uint32][3]float32, len(kept))
	for mapID, tiles := range kept {
		navOrigs[mapID] = buildNavMeshParams(tiles).Orig
	}

	// Build the tile job list (MapBuilder::buildMap queue).
	var jobs []mapTile
	if *tileFlag != "" && *targetMap >= 0 {
		parts := strings.Split(*tileFlag, ",")
		if len(parts) == 2 {
			tx, err1 := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
			ty, err2 := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
			if err1 == nil && err2 == nil {
				jobs = []mapTile{{MapID: uint32(*targetMap), TileX: uint32(tx), TileY: uint32(ty)}}
			}
		}
		if len(jobs) == 0 {
			fmt.Fprintf(os.Stderr, "Invalid --tile value %q, expected X,Y\n", *tileFlag)
			os.Exit(1)
		}
	} else {
		for _, mapID := range mapIDs {
			tiles, ok := kept[mapID]
			if !ok {
				continue
			}
			sort.Slice(tiles, func(i, j int) bool {
				if tiles[i].TileY != tiles[j].TileY {
					return tiles[i].TileY < tiles[j].TileY
				}
				return tiles[i].TileX < tiles[j].TileX
			})
			jobs = append(jobs, tiles...)
		}
	}

	// Worker pool (MapBuilder::buildMaps TileBuilder threads); each worker
	// owns its TerrainBuilder like the C++ TileBuilder does.
	workerCount := *threads
	if workerCount < 1 {
		workerCount = 1
	}
	jobCh := make(chan mapTile)
	resultCh := make(chan tileResult, len(jobs))
	var wg sync.WaitGroup
	var processed uint32
	totalTiles := uint32(len(jobs))
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tb := newTerrainBuilder(*skipLiquid, *mapsDir, *vmapsDir)
			for tile := range jobCh {
				if shouldSkipTile(tile.MapID, tile.TileX, tile.TileY, *outputDir) {
					resultCh <- tileResult{tile: tile, stats: tileBuildStats{Skipped: true}}
					atomic.AddUint32(&processed, 1)
					continue
				}
				done := atomic.AddUint32(&processed, 1)
				pct := uint32(0)
				if totalTiles > 0 {
					pct = done * 100 / totalTiles
				}
				fmt.Printf("%d%% [Map %03d] Building tile [%02d,%02d]\n", pct, tile.MapID, tile.TileX, tile.TileY)

				var md MeshData
				tb.LoadMap(tile.MapID, tile.TileX, tile.TileY, &md)
				// C++ passes (tileY, tileX) here — the swapped order in
				// TileBuilder::buildTile is mirrored faithfully.
				tb.LoadVMap(tile.MapID, tile.TileY, tile.TileX, &md)

				if len(md.SolidVerts) == 0 && len(md.LiquidVerts) == 0 {
					resultCh <- tileResult{tile: tile, stats: tileBuildStats{Empty: true}}
					continue
				}

				// Remove unused vertices.
				CleanVertices(&md.SolidVerts, &md.SolidTris)
				CleanVertices(&md.LiquidVerts, &md.LiquidTris)

				// Gather all mesh data for bounds calculation.
				allVerts := make([]float32, 0, len(md.LiquidVerts)+len(md.SolidVerts))
				allVerts = append(allVerts, md.LiquidVerts...)
				allVerts = append(allVerts, md.SolidVerts...)
				if len(allVerts) == 0 {
					resultCh <- tileResult{tile: tile, stats: tileBuildStats{Empty: true}}
					continue
				}
				bmin, bmax := getTileBounds(tile.TileX, tile.TileY, allVerts, len(allVerts)/3)

				tb.LoadOffMeshConnections(tile.MapID, tile.TileX, tile.TileY, &md, *offMeshFile)

				stats := buildMoveMapTile(tile.MapID, tile.TileX, tile.TileY, &md, bmin, bmax,
					tileConfig, *bigBaseUnit, maxWalkableAngle, maxWalkableAngleNotSteep,
					*outputDir, navOrigs[tile.MapID], tb.UsesLiquids(), *debugOutput)
				resultCh <- tileResult{tile: tile, stats: stats}
			}
		}()
	}
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	var built, skipped, empty, written, totalSpans, totalWalkable, totalPolys, totalVerts, totalNavData int
	var maxDist uint16
	for res := range resultCh {
		switch {
		case res.stats.Skipped:
			skipped++
		case res.stats.Empty:
			empty++
		default:
			built++
			totalSpans += res.stats.Spans
			totalWalkable += res.stats.WalkableSpans
			totalPolys += res.stats.Polys
			totalVerts += res.stats.Verts
			totalNavData += res.stats.NavDataSize
			if res.stats.Written {
				written++
			}
			if res.stats.MaxDistance > maxDist {
				maxDist = res.stats.MaxDistance
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("Complete in %v: %d tiles built (%d .mmtile written), %d skipped (valid .mmtile exists), %d empty, %d compact spans (%d walkable), %d polys, %d verts, %d navmesh bytes, max distance %d.\n",
		elapsed.Round(time.Millisecond), built, written, skipped, empty, totalSpans, totalWalkable, totalPolys, totalVerts, totalNavData, maxDist)
	fmt.Printf("Output directory: '%s'\n", *outputDir)
}

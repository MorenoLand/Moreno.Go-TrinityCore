// MMapManager port: MMAP::MMapManager (MMapManager.h, MMapManager.cpp) with
// the singleton access of MMAP::MMapFactory (MMapFactory.h, MMapFactory.cpp).
// The manager owns one dtNavMesh per map id plus one dtNavMeshQuery per
// instance id, loads %03u.mmap navmesh params and %03u%02i%02i.mmtile tiles
// from <DataDir>/mmaps, and tracks loaded-tile counts for the .mmap stats
// command (cs_mmaps.cpp:214-260).
//
// Fidelity notes against the C++:
//
//   - The C++ manager carries no lock: callers run on the map thread and
//     InitializeThreadUnsafe exists precisely to pre-register map ids before
//     threads start (MMapManager.cpp: InitializeThreadUnsafe). The Go server
//     is concurrent, so every method here takes the manager's own sync.Mutex,
//     the same guard pattern as Server.terrainMu over the terrain maps
//     (server.go: terrainMu, terrain.go). The threadSafeEnvironment flag is
//     kept with its C++ meaning (MMapManager.h: ctor true; set false by
//     InitializeThreadUnsafe).
//   - C++ aborts the process on two paths: loadMapData for an unregistered
//     map id after startup (MMapManager.cpp: ABORT_MSG) and unloadMap of a
//     tile whose removeTile fails (MMapManager.cpp: ABORT()). Go has no
//     abort equivalent in this tree; both paths return false instead.
//   - loadMap's basePath parameter is ignored in C++ too (MMapManager.cpp:
//     loadMap takes /*basePath*/ and reads sConfigMgr DataDir); the Go
//     manager reads Server.Config.GameDataDir, the DataDir twin used by the
//     terrain/vmap loaders (terrain.go).
//   - getLoadedMapsCount returns loadedMMaps.size() (MMapManager.h), which
//     counts pre-registered-but-unloaded (nil) entries; LoadedMapsCount
//     keeps that: len(loadedMMaps), not the count of loaded meshes.
//   - C++ dtNavMeshParams is 28 bytes, orig-first (DetourNavMesh.h:
//     orig[3], tileWidth, tileHeight, maxTiles, maxPolys); loadMapData
//     reads the file in exactly that order. This agrees with the
//     tools/mmaps-generator NavMeshParams.Marshal fix (orig-first); a
//     .mmap written by the generator and read here round-trips.
//   - The .mmtile tile data passed to addTile starts AFTER the 20-byte
//     MmapTileHeader (MMapManager.cpp: loadMap allocates fileHeader.size
//     bytes past the header); only the header's magic, mmap version and
//     size are validated, never its dtVersion, matching C++.
//
// Unwired callers (faithful manager, no fabricated driver): no Go Map/Grid
// driver exists for Map::LoadMMap / Map::UnloadGrid (Map.cpp:167-175, 1722),
// so nothing calls LoadMap/UnloadMap yet; no Map.dbc enumeration exists
// for World::LoadDBCStores' InitializeThreadUnsafe call (World.cpp:1620-1626),
// so the manager starts in thread-safe mode with no pre-registered ids and
// registers maps lazily on first loadMapData, exactly the C++ thread-safe
// path. PathGenerator (the .mmap path/testarea consumer) lands in a sibling
// unit; it reads the mesh through GetNavMesh/GetNavMeshQuery here.
package world

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/tools/mmaps-generator/detour"
)

const (
	// mmapMagic is 'MMAP' (MapDefines.h:24).
	mmapMagic = 0x4d4d4150
	// mmapVersion is MMAP_VERSION (MapDefines.h:25).
	mmapVersion = 13
	// mmapParamsSize is sizeof(dtNavMeshParams): 3 + 2 floats + 2 ints
	// (DetourNavMesh.h:339-346).
	mmapParamsSize = 28
	// mmapTileHeaderSize is sizeof(MmapTileHeader), asserted 20 in C++
	// (MapDefines.h:41).
	mmapTileHeaderSize = 20
	// mmapNavMeshQueryMaxNodes is the 1024-node query pool
	// GetNavMeshQuery inits with (MMapManager.cpp: GetNavMeshQuery).
	mmapNavMeshQueryMaxNodes = 1024
)

// mmapData mirrors MMAP::MMapData (MMapManager.h): one dtNavMesh plus one
// dtNavMeshQuery per instance id (queries are not thread-safe, so C++
// keeps one per instance) and the packed-tile-id -> tile-ref set.
type mmapData struct {
	navMesh        *detour.NavMesh
	navMeshQueries map[uint32]*detour.NavMeshQuery
	loadedTileRefs map[uint32]detour.TileRef
}

// MMapManager mirrors MMAP::MMapManager (MMapManager.h). The zero value is
// not usable: use newMMapManager, which sets threadSafeEnvironment true
// like the C++ ctor (MMapManager.h: MMapManager() : loadedTiles(0),
// thread_safe_environment(true)).
type MMapManager struct {
	mu sync.Mutex

	dataDir string

	// loadedMMaps mirrors MMapDataSet loadedMMaps (MMapManager.h). A nil
	// value is a pre-registered-but-unloaded map (InitializeThreadUnsafe
	// inserts nullptr entries; unloadMap(mapId) restores the nil
	// placeholder instead of erasing), so presence and loadedness differ.
	loadedMMaps map[uint32]*mmapData

	loadedTiles           uint32
	threadSafeEnvironment bool
}

// newMMapManager mirrors MMapFactory::createOrGetMMapManager's lazy
// construction (MMapFactory.cpp:25-31): one manager per data dir.
func newMMapManager(dataDir string) *MMapManager {
	return &MMapManager{
		dataDir:               dataDir,
		loadedMMaps:           make(map[uint32]*mmapData),
		threadSafeEnvironment: true,
	}
}

// mmapManager is the Server's lazy accessor for the MMapManager singleton,
// the Go twin of MMAP::MMapFactory::createOrGetMMapManager
// (MMapFactory.cpp:25-31). There is no Go twin of MMapFactory::clear: the
// manager dies with the Server. A nil Server yields a throwaway manager
// (empty data dir, loads nothing) rather than a panic.
func (s *Server) mmapManager() *MMapManager {
	if s == nil {
		return newMMapManager("")
	}
	s.mmapMu.Lock()
	defer s.mmapMu.Unlock()
	if s.mmaps == nil {
		s.mmaps = newMMapManager(s.Config.GameDataDir)
	}
	return s.mmaps
}

// InitializeThreadUnsafe mirrors MMapManager::InitializeThreadUnsafe
// (MMapManager.cpp): pre-register every map id the manager will ever serve
// (World.cpp:1620-1626 builds the list from sMapStore) and switch to the
// thread-unsafe environment, in which an unregistered map id is a fatal
// error in C++ (here: loadMapData returns false). Insert semantics match
// C++: an already-present entry is left alone.
func (m *MMapManager) InitializeThreadUnsafe(mapIDs []uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range mapIDs {
		if _, ok := m.loadedMMaps[id]; !ok {
			m.loadedMMaps[id] = nil
		}
	}
	m.threadSafeEnvironment = false
}

// getMMapDataLocked mirrors MMapManager::GetMMapData (MMapManager.cpp):
// the entry if present and loaded, nil otherwise (a nil placeholder reads
// as end()). Caller must hold m.mu.
func (m *MMapManager) getMMapDataLocked(mapID uint32) *mmapData {
	data, ok := m.loadedMMaps[mapID]
	if !ok || data == nil {
		return nil
	}
	return data
}

// parseMMapParams decodes the 28-byte %03u.mmap file in C++ struct order:
// orig[3], tileWidth, tileHeight, maxTiles, maxPolys
// (DetourNavMesh.h:339-346; fread(&params, sizeof(dtNavMeshParams), 1,
// file) in MMapManager.cpp: loadMapData). Little-endian throughout.
func parseMMapParams(b []byte) *detour.NavMeshParams {
	return &detour.NavMeshParams{
		Orig: detour.Vec3{
			math.Float32frombits(binary.LittleEndian.Uint32(b[0:4])),
			math.Float32frombits(binary.LittleEndian.Uint32(b[4:8])),
			math.Float32frombits(binary.LittleEndian.Uint32(b[8:12])),
		},
		TileWidth:  math.Float32frombits(binary.LittleEndian.Uint32(b[12:16])),
		TileHeight: math.Float32frombits(binary.LittleEndian.Uint32(b[16:20])),
		MaxTiles:   int(int32(binary.LittleEndian.Uint32(b[20:24]))),
		MaxPolys:   int(int32(binary.LittleEndian.Uint32(b[24:28]))),
	}
}

// loadMapDataLocked mirrors MMapManager::loadMapData (MMapManager.cpp):
// return true if the map's dtNavMesh is already loaded, else read
// <DataDir>/mmaps/%03u.mmap (MAP_FILE_NAME_FORMAT "%s/mmaps/%03i.mmap"),
// init the mesh from its params, and store it. Caller must hold m.mu;
// loadMapData is the locking entry point.
func (m *MMapManager) loadMapDataLocked(mapID uint32) bool {
	if data, ok := m.loadedMMaps[mapID]; ok {
		if data != nil {
			return true
		}
	} else {
		if m.threadSafeEnvironment {
			m.loadedMMaps[mapID] = nil
		} else {
			// C++ ABORT_MSGs here ("Invalid mapId %u passed to
			// MMapManager after startup in thread unsafe
			// environment"); Go returns false instead.
			return false
		}
	}

	raw, err := os.ReadFile(filepath.Join(m.dataDir, "mmaps", fmt.Sprintf("%03d.mmap", mapID)))
	if err != nil || len(raw) < mmapParamsSize {
		return false
	}

	mesh := &detour.NavMesh{}
	if detour.StatusFailed(mesh.Init(parseMMapParams(raw[:mmapParamsSize]))) {
		return false
	}

	m.loadedMMaps[mapID] = &mmapData{
		navMesh:        mesh,
		navMeshQueries: make(map[uint32]*detour.NavMeshQuery),
		loadedTileRefs: make(map[uint32]detour.TileRef),
	}
	return true
}

// loadMapData mirrors MMapManager::loadMapData (MMapManager.cpp): the
// locking entry point for loadMapDataLocked. It reads the 28-byte .mmap
// params orig-first; see parseMMapParams.
func (m *MMapManager) loadMapData(mapID uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadMapDataLocked(mapID)
}

// packTileID mirrors MMapManager::packTileID (MMapManager.cpp):
// uint32(x << 16 | y).
func packTileID(x, y int32) uint32 {
	return uint32(x<<16 | y)
}

// LoadMap mirrors MMapManager::loadMap (MMapManager.cpp): ensure the map's
// mesh is loaded, refuse an already-loaded tile, then read
// <DataDir>/mmaps/%03u%02i%02i.mmtile (TILE_FILE_NAME_FORMAT), validate its
// 20-byte MmapTileHeader (magic MMAP, version 13, size within the file),
// and addTile the bytes past the header with DT_TILE_FREE_DATA. The tile
// data handed to Detour starts at the dtMeshHeader, exactly as in C++.
func (m *MMapManager) LoadMap(mapID uint32, x, y int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.loadMapDataLocked(mapID) {
		return false
	}
	data := m.loadedMMaps[mapID]

	packedGridPos := packTileID(x, y)
	if _, ok := data.loadedTileRefs[packedGridPos]; ok {
		return false
	}

	raw, err := os.ReadFile(filepath.Join(m.dataDir, "mmaps", fmt.Sprintf("%03d%02d%02d.mmtile", mapID, x, y)))
	if err != nil || len(raw) < mmapTileHeaderSize {
		return false
	}
	if binary.LittleEndian.Uint32(raw[0:4]) != mmapMagic {
		return false
	}
	if binary.LittleEndian.Uint32(raw[8:12]) != mmapVersion {
		return false
	}
	size := binary.LittleEndian.Uint32(raw[12:16])
	// C++ (pos = ftell after the header; size > file end - pos is
	// corrupt): the tile payload must fit in the bytes past the header.
	if uint64(size) > uint64(len(raw)-mmapTileHeaderSize) {
		return false
	}

	ref, st := data.navMesh.AddTile(raw[mmapTileHeaderSize:mmapTileHeaderSize+int(size)], detour.DT_TILE_FREE_DATA, 0)
	if detour.StatusFailed(st) {
		return false
	}
	data.loadedTileRefs[packedGridPos] = ref
	m.loadedTiles++
	return true
}

// UnloadMap mirrors MMapManager::unloadMap(uint32, int32, int32)
// (MMapManager.cpp): remove one tile by packed grid position. A removeTile
// failure aborts in C++; here it returns false.
func (m *MMapManager) UnloadMap(mapID uint32, x, y int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := m.getMMapDataLocked(mapID)
	if data == nil {
		return false
	}
	packedGridPos := packTileID(x, y)
	ref, ok := data.loadedTileRefs[packedGridPos]
	if !ok {
		return false
	}
	if _, _, st := data.navMesh.RemoveTile(ref); detour.StatusFailed(st) {
		return false
	}
	delete(data.loadedTileRefs, packedGridPos)
	m.loadedTiles--
	return true
}

// UnloadMapAll mirrors MMapManager::unloadMap(uint32) (MMapManager.cpp):
// remove every tile of the map, drop the mesh and its per-instance
// queries, and leave the nil placeholder so a later loadMapData can reload
// the map. A per-tile removeTile failure is skipped (C++ logs and
// continues) and keeps its loadedTiles count, matching C++.
func (m *MMapManager) UnloadMapAll(mapID uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.loadedMMaps[mapID]
	if !ok || data == nil {
		return false
	}
	for _, ref := range data.loadedTileRefs {
		if _, _, st := data.navMesh.RemoveTile(ref); detour.StatusFailed(st) {
			continue
		}
		m.loadedTiles--
	}
	m.loadedMMaps[mapID] = nil
	return true
}

// UnloadMapInstance mirrors MMapManager::unloadMapInstance (MMapManager.cpp):
// drop one instance's dtNavMeshQuery (C++ dtFreeNavMeshQuery frees it;
// Go drops the reference).
func (m *MMapManager) UnloadMapInstance(mapID, instanceID uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := m.getMMapDataLocked(mapID)
	if data == nil {
		return false
	}
	if _, ok := data.navMeshQueries[instanceID]; !ok {
		return false
	}
	delete(data.navMeshQueries, instanceID)
	return true
}

// GetNavMesh mirrors MMapManager::GetNavMesh (MMapManager.cpp): the map's
// dtNavMesh, or nil when the map is not loaded.
func (m *MMapManager) GetNavMesh(mapID uint32) *detour.NavMesh {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := m.getMMapDataLocked(mapID)
	if data == nil {
		return nil
	}
	return data.navMesh
}

// GetNavMeshQuery mirrors MMapManager::GetNavMeshQuery (MMapManager.cpp):
// the instance's dtNavMeshQuery, allocating and initing it with 1024 nodes
// on first use; nil when the map is not loaded or the init fails. The
// returned query is NOT thread-safe (MMapManager.h), matching C++.
func (m *MMapManager) GetNavMeshQuery(mapID, instanceID uint32) *detour.NavMeshQuery {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := m.getMMapDataLocked(mapID)
	if data == nil {
		return nil
	}
	if q, ok := data.navMeshQueries[instanceID]; ok {
		return q
	}
	q := detour.NewNavMeshQuery()
	if detour.StatusFailed(q.Init(data.navMesh, mmapNavMeshQueryMaxNodes)) {
		return nil
	}
	data.navMeshQueries[instanceID] = q
	return data.navMeshQueries[instanceID]
}

// LoadedTilesCount mirrors MMapManager::getLoadedTilesCount
// (MMapManager.h).
func (m *MMapManager) LoadedTilesCount() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadedTiles
}

// LoadedMapsCount mirrors MMapManager::getLoadedMapsCount (MMapManager.h):
// len(loadedMMaps), including pre-registered-but-unloaded entries.
func (m *MMapManager) LoadedMapsCount() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return uint32(len(m.loadedMMaps))
}

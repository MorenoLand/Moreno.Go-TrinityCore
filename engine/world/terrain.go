package world

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	terrainGridSize             = float64(533.3333)
	terrainCenterGrid           = 32.0
	terrainMapHalfSize          = float32(0.5 * 64.0 * 533.33333333)
	terrainAreaNoArea    uint16 = 0x0001
	terrainModelM2              = uint32(0x00000001)
	terrainModelHasBound        = uint32(0x00000004)
)

type terrainVector struct{ X, Y, Z float32 }
type terrainSpawn struct {
	Flags      uint32
	ADTID      uint16
	ID         uint32
	Position   terrainVector
	Rotation   terrainVector
	Scale      float32
	BoundsLow  terrainVector
	BoundsHigh terrainVector
	Name       string
}
type terrainTriangle struct{ A, B, C uint32 }
type terrainGroup struct {
	Low       terrainVector
	High      terrainVector
	Flags     uint32
	ID        uint32
	Vertices  []terrainVector
	Triangles []terrainTriangle
}
type terrainModel struct {
	Root   uint32
	Groups []terrainGroup
}
type terrainWMOHit struct {
	Root   uint32
	ADT    uint16
	Group  uint32
	Ground float32
}

func ResolveTerrainZoneAndArea(dataDir string, mapID uint32, x, y, z float32, fallback uint32) (uint32, uint32, error) {
	if dataDir == "" {
		return 0, 0, fmt.Errorf("terrain data directory is required")
	}
	server := &Server{Config: config.Config{GameDataDir: dataDir}, Data: wotlk.NewStore(filepath.Join(dataDir, "dbc")), terrainTiles: make(map[uint64][]terrainSpawn), terrainTileKnown: make(map[uint64]bool), terrainModels: make(map[string]*terrainModel)}
	zoneID, areaID := server.zoneAndAreaID(mapID, x, y, z, fallback)
	return zoneID, areaID, nil
}

func (s *Server) zoneAndAreaID(mapID uint32, x, y, z float32, fallback uint32) (uint32, uint32) {
	areaID, found := s.mapWMOAreaID(mapID, x, y, z)
	if !found {
		areaID, found = s.mapAreaID(mapID, x, y)
	}
	if !found && s.Data != nil {
		if mapEntry, ok, err := s.Data.Map(mapID); err == nil && ok {
			areaID = mapEntry.AreaTableID
			found = areaID != 0
		}
	}
	if !found {
		areaID = fallback
	}
	zoneID := areaID
	if s.Data != nil && areaID != 0 {
		if area, ok, err := s.Data.Area(areaID); err == nil && ok && area.ParentAreaID != 0 {
			zoneID = area.ParentAreaID
		}
	}
	if zoneID == 0 {
		zoneID = fallback
	}
	return zoneID, areaID
}

func (s *session) updateZoneAndArea(ctx context.Context, force bool) {
	if s == nil || s.server == nil || s.player == nil || !s.playerLoaded {
		return
	}
	now := time.Now()
	if !force && !s.lastZoneUpdate.IsZero() && now.Sub(s.lastZoneUpdate) < time.Second {
		return
	}
	zoneID, areaID := s.server.zoneAndAreaID(s.player.Map, s.player.X, s.player.Y, s.player.Z, s.player.Zone)
	oldZone, oldArea := s.player.Zone, s.areaID
	s.player.Zone, s.areaID, s.lastZoneUpdate = zoneID, areaID, now
	if zoneID == 0 {
		s.player.Zone = oldZone
	}
	stateChanged := false
	if oldZone != s.player.Zone || oldArea != areaID {
		stateChanged = s.applyZoneState(s.player, s.player.Zone, areaID)
	}
	if oldZone != s.player.Zone {
		s.updateLocalChannels(s.player.Zone)
		s.exploreZone(ctx, s.player.Zone)
		_ = s.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), buildInitWorldStates(*s.player, areaID, s.server.Config.ArenaSeasonID, s.server.Config.ArenaSeasonInProgress), true)
	}
	if stateChanged || (oldArea != areaID && oldZone == s.player.Zone) {
		s.sendPlayerUpdate()
	}
}

func (s *Server) terrainTile(mapID uint32, tileX, tileY int) []terrainSpawn {
	key := uint64(mapID)<<32 | uint64(tileX&0xFFFF)<<16 | uint64(tileY&0xFFFF)
	s.terrainMu.Lock()
	if s.terrainTileKnown[key] {
		result := s.terrainTiles[key]
		s.terrainMu.Unlock()
		return result
	}
	s.terrainTileKnown[key] = true
	s.terrainMu.Unlock()
	path := filepath.Join(s.Config.GameDataDir, "vmaps", fmt.Sprintf("%03d_%02d_%02d.vmtile", mapID, tileY, tileX))
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 12 || string(data[:8]) != "VMAP_4.7" {
		s.terrainMu.Lock()
		s.terrainTiles[key] = nil
		s.terrainMu.Unlock()
		return nil
	}
	reader := bytes.NewReader(data[8:])
	var count uint32
	if binary.Read(reader, binary.LittleEndian, &count) != nil || count > 1000000 {
		return nil
	}
	spawns := make([]terrainSpawn, 0, count)
	for i := uint32(0); i < count; i++ {
		spawn, spawnErr := readTerrainSpawn(reader)
		if spawnErr != nil {
			spawns = nil
			break
		}
		var nodeIndex uint32
		if binary.Read(reader, binary.LittleEndian, &nodeIndex) != nil {
			spawns = nil
			break
		}
		spawns = append(spawns, spawn)
	}
	s.terrainMu.Lock()
	s.terrainTiles[key] = spawns
	s.terrainMu.Unlock()
	return spawns
}

func readTerrainSpawn(reader *bytes.Reader) (terrainSpawn, error) {
	var spawn terrainSpawn
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
	if spawn.Flags&terrainModelHasBound != 0 {
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsLow); err != nil {
			return spawn, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &spawn.BoundsHigh); err != nil {
			return spawn, err
		}
	}
	var nameLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil || nameLen > 1024 || uint64(nameLen) > uint64(reader.Len()) {
		return spawn, fmt.Errorf("invalid terrain model name")
	}
	name := make([]byte, nameLen)
	if _, err := io.ReadFull(reader, name); err != nil {
		return spawn, err
	}
	spawn.Name = string(name)
	return spawn, nil
}

func (s *Server) terrainModel(name string) *terrainModel {
	s.terrainMu.Lock()
	if model, found := s.terrainModels[name]; found {
		s.terrainMu.Unlock()
		return model
	}
	s.terrainMu.Unlock()
	model, err := readTerrainModel(filepath.Join(s.Config.GameDataDir, "vmaps", filepath.FromSlash(strings.ReplaceAll(name, "\\", "/"))+".vmo"))
	s.terrainMu.Lock()
	s.terrainModels[name] = model
	s.terrainMu.Unlock()
	if err != nil {
		return nil
	}
	return model
}

func readTerrainModel(path string) (*terrainModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(data)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != "VMAP_4.7" {
		return nil, fmt.Errorf("invalid terrain model")
	}
	chunk, size, err := readTerrainChunk(reader)
	if err != nil || chunk != "WMOD" || size < 8 {
		return nil, fmt.Errorf("invalid terrain model header")
	}
	var root uint32
	if binary.Read(reader, binary.LittleEndian, &root) != nil {
		return nil, fmt.Errorf("invalid terrain model root")
	}
	model := &terrainModel{Root: root}
	if reader.Len() == 0 {
		return model, nil
	}
	chunk, _, err = readTerrainChunk(reader)
	if err != nil || chunk != "GMOD" {
		return model, nil
	}
	var count uint32
	if binary.Read(reader, binary.LittleEndian, &count) != nil || count > 100000 {
		return nil, fmt.Errorf("invalid terrain group count")
	}
	model.Groups = make([]terrainGroup, count)
	for i := range model.Groups {
		group := &model.Groups[i]
		if binary.Read(reader, binary.LittleEndian, &group.Low) != nil || binary.Read(reader, binary.LittleEndian, &group.High) != nil || binary.Read(reader, binary.LittleEndian, &group.Flags) != nil || binary.Read(reader, binary.LittleEndian, &group.ID) != nil {
			return nil, fmt.Errorf("invalid terrain group header")
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "VERT" {
			return nil, fmt.Errorf("invalid terrain vertices")
		}
		var vertices uint32
		if binary.Read(reader, binary.LittleEndian, &vertices) != nil || vertices > 10000000 {
			return nil, fmt.Errorf("invalid terrain vertex count")
		}
		group.Vertices = make([]terrainVector, vertices)
		if binary.Read(reader, binary.LittleEndian, &group.Vertices) != nil {
			return nil, fmt.Errorf("invalid terrain vertices")
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "TRIM" {
			return nil, fmt.Errorf("invalid terrain triangles")
		}
		var triangles uint32
		if binary.Read(reader, binary.LittleEndian, &triangles) != nil || triangles > 10000000 {
			return nil, fmt.Errorf("invalid terrain triangle count")
		}
		group.Triangles = make([]terrainTriangle, triangles)
		for index := range group.Triangles {
			if binary.Read(reader, binary.LittleEndian, &group.Triangles[index].A) != nil || binary.Read(reader, binary.LittleEndian, &group.Triangles[index].B) != nil || binary.Read(reader, binary.LittleEndian, &group.Triangles[index].C) != nil {
				return nil, fmt.Errorf("invalid terrain triangle")
			}
		}
		chunk, _, err = readTerrainChunk(reader)
		if err != nil || chunk != "MBIH" || skipTerrainBIH(reader) != nil {
			return nil, fmt.Errorf("invalid terrain mesh index")
		}
		chunk, liquidSize, err := readTerrainChunk(reader)
		if err != nil || chunk != "LIQU" || uint64(liquidSize) > uint64(reader.Len()) {
			return nil, fmt.Errorf("invalid terrain liquid")
		}
		if _, err := reader.Seek(int64(liquidSize), io.SeekCurrent); err != nil {
			return nil, err
		}
	}
	chunk, _, err = readTerrainChunk(reader)
	if err != nil || chunk != "GBIH" || skipTerrainBIH(reader) != nil {
		return nil, fmt.Errorf("invalid terrain group index")
	}
	return model, nil
}

func readTerrainChunk(reader *bytes.Reader) (string, uint32, error) {
	name := make([]byte, 4)
	if _, err := io.ReadFull(reader, name); err != nil {
		return "", 0, err
	}
	var size uint32
	if binary.Read(reader, binary.LittleEndian, &size) != nil {
		return "", 0, io.ErrUnexpectedEOF
	}
	return string(name), size, nil
}

func skipTerrainBIH(reader *bytes.Reader) error {
	var low, high terrainVector
	if binary.Read(reader, binary.LittleEndian, &low) != nil || binary.Read(reader, binary.LittleEndian, &high) != nil {
		return io.ErrUnexpectedEOF
	}
	var treeCount uint32
	if binary.Read(reader, binary.LittleEndian, &treeCount) != nil || treeCount > 100000000 || uint64(treeCount)*4 > uint64(reader.Len()) {
		return fmt.Errorf("invalid terrain tree")
	}
	if _, err := reader.Seek(int64(treeCount)*4, io.SeekCurrent); err != nil {
		return err
	}
	var objectCount uint32
	if binary.Read(reader, binary.LittleEndian, &objectCount) != nil || objectCount > 100000000 || uint64(objectCount)*4 > uint64(reader.Len()) {
		return fmt.Errorf("invalid terrain objects")
	}
	_, err := reader.Seek(int64(objectCount)*4, io.SeekCurrent)
	return err
}

func terrainContains(low, high, value terrainVector) bool {
	return value.X >= low.X && value.X <= high.X && value.Y >= low.Y && value.Y <= high.Y && value.Z >= low.Z && value.Z <= high.Z
}
func terrainSub(a, b terrainVector) terrainVector {
	return terrainVector{a.X - b.X, a.Y - b.Y, a.Z - b.Z}
}
func terrainAdd(a, b terrainVector) terrainVector {
	return terrainVector{a.X + b.X, a.Y + b.Y, a.Z + b.Z}
}
func terrainScale(a terrainVector, scale float32) terrainVector {
	return terrainVector{a.X * scale, a.Y * scale, a.Z * scale}
}
func terrainDot(a, b terrainVector) float64 {
	return float64(a.X)*float64(b.X) + float64(a.Y)*float64(b.Y) + float64(a.Z)*float64(b.Z)
}
func terrainCross(a, b terrainVector) terrainVector {
	return terrainVector{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func terrainRotation(rotation terrainVector) [3][3]float64 {
	x, y, z := float64(rotation.X)*math.Pi/180, float64(rotation.Y)*math.Pi/180, float64(rotation.Z)*math.Pi/180
	sx, cx, sy, cy, sz, cz := math.Sin(x), math.Cos(x), math.Sin(y), math.Cos(y), math.Sin(z), math.Cos(z)
	return [3][3]float64{{cy * cz, cz*sx*sy - cx*sz, cx*cz*sy + sx*sz}, {cy * sz, cx*cz + sx*sy*sz, -cz*sx + cx*sy*sz}, {-sy, cy * sx, cx * cy}}
}

func terrainModelPoint(spawn terrainSpawn, point terrainVector) terrainVector {
	m := terrainRotation(spawn.Rotation)
	delta := terrainSub(point, spawn.Position)
	scale := spawn.Scale
	if scale == 0 {
		scale = 1
	}
	return terrainVector{float32((m[0][0]*float64(delta.X) + m[1][0]*float64(delta.Y) + m[2][0]*float64(delta.Z)) / float64(scale)), float32((m[0][1]*float64(delta.X) + m[1][1]*float64(delta.Y) + m[2][1]*float64(delta.Z)) / float64(scale)), float32((m[0][2]*float64(delta.X) + m[1][2]*float64(delta.Y) + m[2][2]*float64(delta.Z)) / float64(scale))}
}

func terrainWorldPoint(spawn terrainSpawn, point terrainVector) terrainVector {
	m := terrainRotation(spawn.Rotation)
	scale := spawn.Scale
	if scale == 0 {
		scale = 1
	}
	return terrainAdd(spawn.Position, terrainVector{float32((m[0][0]*float64(point.X) + m[0][1]*float64(point.Y) + m[0][2]*float64(point.Z)) * float64(scale)), float32((m[1][0]*float64(point.X) + m[1][1]*float64(point.Y) + m[1][2]*float64(point.Z)) * float64(scale)), float32((m[2][0]*float64(point.X) + m[2][1]*float64(point.Y) + m[2][2]*float64(point.Z)) * float64(scale))})
}

func terrainTriangleDistance(origin terrainVector, triangle terrainTriangle, vertices []terrainVector, limit float64) (float64, bool) {
	if int(triangle.A) >= len(vertices) || int(triangle.B) >= len(vertices) || int(triangle.C) >= len(vertices) {
		return 0, false
	}
	direction := terrainVector{0, 0, -1}
	e1 := terrainSub(vertices[triangle.B], vertices[triangle.A])
	e2 := terrainSub(vertices[triangle.C], vertices[triangle.A])
	p := terrainCross(direction, e2)
	a := terrainDot(e1, p)
	if math.Abs(a) < 1e-5 {
		return 0, false
	}
	f := 1 / a
	u := f * terrainDot(terrainSub(origin, vertices[triangle.A]), p)
	if u < 0 || u > 1 {
		return 0, false
	}
	q := terrainCross(terrainSub(origin, vertices[triangle.A]), e1)
	v := f * terrainDot(direction, q)
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := f * terrainDot(e2, q)
	return t, t > 0 && t < limit
}

func terrainModelHit(spawn terrainSpawn, model *terrainModel, point terrainVector) (terrainWMOHit, bool) {
	if model == nil || spawn.Flags&terrainModelM2 != 0 || spawn.Flags&terrainModelHasBound == 0 || !terrainContains(spawn.BoundsLow, spawn.BoundsHigh, point) {
		return terrainWMOHit{}, false
	}
	local := terrainModelPoint(spawn, point)
	best := terrainWMOHit{Root: model.Root, ADT: spawn.ADTID, Ground: -float32(math.Inf(1))}
	found := false
	for _, group := range model.Groups {
		if !terrainContains(group.Low, group.High, local) {
			continue
		}
		origin := local
		origin.Z += 0.1
		bestDistance := math.Inf(1)
		for _, triangle := range group.Triangles {
			if distance, ok := terrainTriangleDistance(origin, triangle, group.Vertices, bestDistance); ok {
				bestDistance = distance
			}
		}
		if math.IsInf(bestDistance, 1) {
			continue
		}
		world := terrainWorldPoint(spawn, terrainAdd(origin, terrainVector{0, 0, float32(-bestDistance)}))
		if !found || world.Z > best.Ground {
			best.Group, best.Ground, found = group.ID, world.Z, true
		}
	}
	return best, found
}

func (s *Server) mapWMOAreaID(mapID uint32, x, y, z float32) (uint32, bool) {
	tileX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	tileY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return 0, false
	}
	point := terrainVector{terrainMapHalfSize - x, terrainMapHalfSize - y, z}
	best := terrainWMOHit{Ground: -float32(math.Inf(1))}
	found := false
	for _, spawn := range s.terrainTile(mapID, 63-tileX, 63-tileY) {
		if spawn.Flags&terrainModelM2 != 0 || spawn.Flags&terrainModelHasBound == 0 {
			continue
		}
		model := s.terrainModel(spawn.Name)
		hit, ok := terrainModelHit(spawn, model, point)
		if ok && (!found || hit.Ground > best.Ground) {
			best, found = hit, true
		}
	}
	if !found || z < best.Ground-2 {
		return 0, false
	}
	areaID, ok, err := s.Data.WMOArea(int32(best.Root), int32(best.ADT), int32(best.Group))
	return areaID, ok && err == nil
}

func (s *Server) mapAreaID(mapID uint32, x, y float32) (uint32, bool) {
	gridX := int(float64(x)/terrainGridSize + terrainCenterGrid)
	gridY := int(float64(y)/terrainGridSize + terrainCenterGrid)
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return 0, false
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, 63-gridX, 63-gridY))
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	header := make([]byte, 44)
	if _, err := io.ReadFull(file, header); err != nil || string(header[:4]) != "MAPS" || string(header[4:8]) != "v1.9" {
		return 0, false
	}
	areaOffset := binary.LittleEndian.Uint32(header[12:16])
	if _, err := file.Seek(int64(areaOffset), io.SeekStart); err != nil {
		return 0, false
	}
	areaHeader := make([]byte, 8)
	if _, err := io.ReadFull(file, areaHeader); err != nil || string(areaHeader[:4]) != "AREA" {
		return 0, false
	}
	flags := binary.LittleEndian.Uint16(areaHeader[4:6])
	gridArea := uint32(binary.LittleEndian.Uint16(areaHeader[6:8]))
	if flags&terrainAreaNoArea != 0 {
		return gridArea, gridArea != 0
	}
	areas := make([]byte, 16*16*2)
	if _, err := io.ReadFull(file, areas); err != nil {
		return 0, false
	}
	localX := int(16*(terrainCenterGrid-float64(x)/terrainGridSize)) & 15
	localY := int(16*(terrainCenterGrid-float64(y)/terrainGridSize)) & 15
	index := (localX*16 + localY) * 2
	areaID := uint32(binary.LittleEndian.Uint16(areas[index : index+2]))
	return areaID, areaID != 0
}

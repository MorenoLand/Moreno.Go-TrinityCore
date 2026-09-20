package world

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

const (
	terrainGridSize          = float64(533.3333)
	terrainCenterGrid        = 32.0
	terrainAreaNoArea uint16 = 0x0001
)

func (s *Server) zoneAndAreaID(mapID uint32, x, y, _ float32, fallback uint32) (uint32, uint32) {
	areaID, found := s.mapAreaID(mapID, x, y)
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

func (s *Server) mapAreaID(mapID uint32, x, y float32) (uint32, bool) {
	gridX := int(math.Floor(terrainCenterGrid - float64(x)/terrainGridSize))
	gridY := int(math.Floor(terrainCenterGrid - float64(y)/terrainGridSize))
	if gridX < 0 || gridX >= 64 || gridY < 0 || gridY >= 64 {
		return 0, false
	}
	path := filepath.Join(s.Config.GameDataDir, "maps", fmt.Sprintf("%03d%02d%02d.map", mapID, gridX, gridY))
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

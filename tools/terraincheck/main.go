package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/world"
)

type terrainFixture struct {
	Name     string  `json:"name"`
	Map      uint32  `json:"map"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Z        float32 `json:"z"`
	Fallback uint32  `json:"fallback"`
	Zone     uint32  `json:"zone"`
	Area     uint32  `json:"area"`
}

func main() {
	dataDir := flag.String("data", "", "extracted game data directory containing dbc, maps, and vmaps")
	fixturePath := flag.String("fixture", "tools/terraincheck/fixtures/area.json", "terrain fixture JSON")
	flag.Parse()
	if *dataDir == "" {
		fail(fmt.Errorf("-data is required"))
	}
	data, err := os.ReadFile(*fixturePath)
	if err != nil {
		fail(err)
	}
	var fixtures []terrainFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		fail(err)
	}
	for _, fixture := range fixtures {
		zone, area, err := world.ResolveTerrainZoneAndArea(*dataDir, fixture.Map, fixture.X, fixture.Y, fixture.Z, fixture.Fallback)
		if err != nil {
			fail(fmt.Errorf("%s: %w", fixture.Name, err))
		}
		if zone != fixture.Zone || area != fixture.Area {
			fail(fmt.Errorf("%s: got zone=%d area=%d want zone=%d area=%d", fixture.Name, zone, area, fixture.Zone, fixture.Area))
		}
		fmt.Printf("%s: zone=%d area=%d\n", fixture.Name, zone, area)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

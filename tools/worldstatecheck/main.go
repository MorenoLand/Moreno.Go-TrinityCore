package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/world"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type worldStateFixture struct {
	Name            string     `json:"name"`
	Map             uint32     `json:"map"`
	Zone            uint32     `json:"zone"`
	Area            uint32     `json:"area"`
	ArenaSeason     uint32     `json:"arena_season"`
	ArenaInProgress bool       `json:"arena_in_progress"`
	States          [][2]int32 `json:"states"`
}

func main() {
	fixturePath := flag.String("fixture", "tools/worldstatecheck/fixtures/worldstates.json", "world-state fixture JSON")
	flag.Parse()
	data, err := os.ReadFile(*fixturePath)
	if err != nil {
		fail(err)
	}
	var fixtures []worldStateFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		fail(err)
	}
	for _, fixture := range fixtures {
		payload := world.BuildInitialWorldStates(fixture.Map, fixture.Zone, fixture.Area, fixture.ArenaSeason, fixture.ArenaInProgress)
		reader := protocol.NewReader(payload)
		mapID, err := reader.ReadI32()
		if err != nil {
			fail(fmt.Errorf("%s: map: %w", fixture.Name, err))
		}
		zoneID, err := reader.ReadI32()
		if err != nil {
			fail(fmt.Errorf("%s: zone: %w", fixture.Name, err))
		}
		areaID, err := reader.ReadI32()
		if err != nil {
			fail(fmt.Errorf("%s: area: %w", fixture.Name, err))
		}
		count, err := reader.ReadU16()
		if err != nil {
			fail(fmt.Errorf("%s: count: %w", fixture.Name, err))
		}
		if mapID != int32(fixture.Map) || zoneID != int32(fixture.Zone) || areaID != int32(fixture.Area) || int(count) != len(fixture.States) {
			fail(fmt.Errorf("%s: header mismatch", fixture.Name))
		}
		for index, expected := range fixture.States {
			variable, err := reader.ReadI32()
			if err != nil {
				fail(fmt.Errorf("%s state %d variable: %w", fixture.Name, index, err))
			}
			value, err := reader.ReadI32()
			if err != nil {
				fail(fmt.Errorf("%s state %d value: %w", fixture.Name, index, err))
			}
			if variable != expected[0] || value != expected[1] {
				fail(fmt.Errorf("%s state %d: got %d=%d want %d=%d", fixture.Name, index, variable, value, expected[0], expected[1]))
			}
		}
		fmt.Printf("%s: states=%d\n", fixture.Name, count)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

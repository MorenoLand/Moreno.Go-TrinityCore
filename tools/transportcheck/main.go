package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/world"
	_ "modernc.org/sqlite"
)

func main() {
	dataDir := flag.String("data", "", "game data directory containing dbc")
	worldDB := flag.String("world-db", "", "SQLite world database file")
	flag.Parse()
	if *dataDir == "" || *worldDB == "" {
		fail(fmt.Errorf("-data and -world-db are required"))
	}
	store := wotlk.NewStore(filepath.Join(*dataDir, "dbc"))
	db, err := sql.Open("sqlite", *worldDB)
	if err != nil {
		fail(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(context.Background(), `SELECT tr.guid, tr.entry, COALESCE(gt.data0, 0), COALESCE(gt.data1, 0), COALESCE(gt.data2, 0) FROM transports AS tr JOIN gameobject_template AS gt ON gt.entry = tr.entry WHERE gt.type = 15 ORDER BY tr.guid`)
	if err != nil {
		fail(err)
	}
	defer rows.Close()
	count, invalid := 0, 0
	for rows.Next() {
		var guid, entry, pathID int64
		var speed, acceleration float64
		if err := rows.Scan(&guid, &entry, &pathID, &speed, &acceleration); err != nil {
			fail(err)
		}
		count++
		points, err := store.TaxiPathPoints(uint32(pathID))
		if err == nil {
			eventIDs := make(map[uint32]struct{})
			for _, point := range points {
				if point.ArrivalEventID != 0 {
					eventIDs[point.ArrivalEventID] = struct{}{}
				}
				if point.DepartureEventID != 0 {
					eventIDs[point.DepartureEventID] = struct{}{}
				}
			}
			var path *world.TransportTrajectory
			path, err = world.NewTransportTrajectory(points, float32(speed), float32(acceleration))
			if err == nil {
				commands := make(map[int64]int)
				for eventID := range eventIDs {
					scriptRows, queryErr := db.QueryContext(context.Background(), "SELECT command FROM event_scripts WHERE id = ?", eventID)
					if queryErr != nil {
						err = queryErr
						break
					}
					for scriptRows.Next() {
						var command int64
						if scanErr := scriptRows.Scan(&command); scanErr != nil {
							err = scanErr
							break
						}
						commands[command]++
					}
					if closeErr := scriptRows.Close(); err == nil && closeErr != nil {
						err = closeErr
					}
					if err != nil {
						break
					}
				}
				maps := make(map[uint32]struct{}, len(points))
				for _, point := range points {
					if point.MapID >= 0 {
						maps[uint32(point.MapID)] = struct{}{}
					}
				}
				for progress := uint32(0); progress < path.Period(); progress += 1000 {
					mapID, x, y, z, orientation := path.Position(progress)
					if _, ok := maps[mapID]; !ok || math.IsNaN(float64(x)) || math.IsNaN(float64(y)) || math.IsNaN(float64(z)) || math.IsNaN(float64(orientation)) {
						err = fmt.Errorf("route produced invalid state at progress %d: map=%d position=%f,%f,%f orientation=%f", progress, mapID, x, y, z, orientation)
						break
					}
				}
				if err == nil {
					fmt.Printf("guid=%d entry=%d path=%d nodes=%d event_ids=%d event_script_commands=%v period_ms=%d\n", guid, entry, pathID, len(points), len(eventIDs), commands, path.Period())
				}
			}
		}
		if err != nil {
			invalid++
			fmt.Fprintf(os.Stderr, "guid=%d entry=%d path=%d: %v\n", guid, entry, pathID, err)
		}
	}
	if err := rows.Err(); err != nil {
		fail(err)
	}
	if count == 0 {
		fail(fmt.Errorf("world database has no continent transports"))
	}
	if invalid != 0 {
		fail(fmt.Errorf("%d of %d continent transport routes are invalid", invalid, count))
	}
	fmt.Printf("validated %d continent transport routes\n", count)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

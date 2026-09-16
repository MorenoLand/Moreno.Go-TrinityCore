package world

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

func TestLoadEnumEquipmentFallsBackToInventory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"CREATE TABLE character_inventory (guid INTEGER, bag INTEGER, slot INTEGER, item INTEGER)",
		"CREATE TABLE item_instance (guid INTEGER PRIMARY KEY, itemEntry INTEGER)",
		"INSERT INTO item_instance VALUES (100, 9001), (101, 9002), (102, 9003)",
		"INSERT INTO character_inventory VALUES (1, 0, 0, 100), (1, 0, 15, 101), (1, 0, 19, 102)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{CharactersStore: store}}
	character := enumCharacter{GUID: 1, Equipment: ""}
	sess.loadEnumEquipment(context.Background(), &character)
	fields := strings.Fields(character.Equipment)
	if len(fields) != int(inventorySlotBagEnd)*2 || fields[0] != "9001" || fields[30] != "9002" || fields[38] != "9003" {
		t.Fatalf("equipment=%q", character.Equipment)
	}
}

func TestLoadEquipmentCachePreservesExistingValues(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{CharactersStore: store}}
	fields := make([]string, int(inventorySlotBagEnd)*2)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "1234"
	cached := strings.Join(fields, " ")
	if got := sess.loadEquipmentCache(context.Background(), 99, cached); got != cached {
		t.Fatalf("equipment cache changed: got %q want %q", got, cached)
	}
}

func TestLoadEnumEquipmentPacksVisibleEnchantments(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"CREATE TABLE character_inventory (guid INTEGER, bag INTEGER, slot INTEGER, item INTEGER)",
		"CREATE TABLE item_instance (guid INTEGER PRIMARY KEY, itemEntry INTEGER, enchantments TEXT)",
		"INSERT INTO item_instance VALUES (100, 9001, '123 60000 0 456 60000 0')",
		"INSERT INTO character_inventory VALUES (1, 0, 0, 100)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	sess := &session{server: &Server{CharactersStore: &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}}}
	character := enumCharacter{GUID: 1}
	sess.loadEnumEquipment(context.Background(), &character)
	fields := strings.Fields(character.Equipment)
	if len(fields) != int(inventorySlotBagEnd)*2 || fields[1] != "29884539" {
		t.Fatalf("equipment=%q", character.Equipment)
	}
}

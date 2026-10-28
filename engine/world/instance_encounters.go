package world

import (
	"context"
)

// dungeonEncounterKey identifies one DungeonEncounter credit row: the
// (map, difficulty) pair comes from the DungeonEncounter.dbc row, the
// credit type and entry from the world `instance_encounters` row
// (ObjectMgr::LoadInstanceEncounters, ObjectMgr.cpp:6030-6115).
type dungeonEncounterKey struct {
	MapID       uint32
	Difficulty  uint32
	CreditType  uint32
	CreditEntry uint32
}

type dungeonEncounterCredit struct {
	Bit                  uint32
	LastEncounterDungeon uint32
}

const (
	dungeonEncounterCreditKillCreature uint32 = 0
	dungeonEncounterCreditCastSpell    uint32 = 1
)

// loadDungeonEncounters mirrors ObjectMgr's instance-encounter store: each
// `instance_encounters` row joins its DungeonEncounter.dbc entry (by the
// row's entry id) and lands under its (map, difficulty, credit type, credit
// entry). The DBC Bit is the completedEncounters mask bit
// (DBCStructure.h:605-615: field 4, "encounter index for creating completed
// mask"). A failed load (missing DBC file, missing world table) leaves the
// map empty and marks the load done, so kill time never retries or logs per
// kill; the table is re-read only on the next server start, matching the
// other startup loaders. The kill-credit lookup below is the dungeon-boss
// gate itself: C++ sets CREATURE_FLAG_EXTRA_DUNGEON_BOSS exactly on the
// templates named by these rows (plus their difficulty variants), and
// KillRewarder credits only IsDungeonBoss victims, so a table hit is
// equivalent to the flag check.
func (s *Server) loadDungeonEncounters(ctx context.Context) {
	if s == nil {
		return
	}
	s.dungeonEncounterMu.Lock()
	defer s.dungeonEncounterMu.Unlock()
	if s.dungeonEncountersLoaded {
		return
	}
	s.dungeonEncountersLoaded = true
	if s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		return
	}
	dbcEntries, err := s.Data.DungeonEncounters()
	if err != nil {
		return
	}
	dbcByID := make(map[uint32]struct {
		mapID, difficulty, bit uint32
	}, len(dbcEntries))
	for _, entry := range dbcEntries {
		dbcByID[entry.ID] = struct {
			mapID, difficulty, bit uint32
		}{entry.MapID, entry.Difficulty, entry.Bit}
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT entry, creditType, creditEntry, lastEncounterDungeon FROM instance_encounters")
	if err != nil {
		return
	}
	defer rows.Close()
	credits := make(map[dungeonEncounterKey]dungeonEncounterCredit)
	for rows.Next() {
		var entry, creditType, creditEntry, lastDungeon uint32
		if err := rows.Scan(&entry, &creditType, &creditEntry, &lastDungeon); err != nil {
			continue
		}
		dbc, ok := dbcByID[entry]
		if !ok {
			continue
		}
		credits[dungeonEncounterKey{MapID: dbc.mapID, Difficulty: dbc.difficulty, CreditType: creditType, CreditEntry: creditEntry}] = dungeonEncounterCredit{Bit: dbc.bit, LastEncounterDungeon: lastDungeon}
	}
	s.dungeonEncounters = credits
}

// creditDungeonEncounterKill mirrors KillRewarder::Reward step 5
// (KillRewarder.cpp:285-288): a dungeon-boss kill inside a live instance
// ORs 1 << Bit into the instance's completedEncounters mask
// (InstanceScript::UpdateEncounterState, InstanceScript.cpp:734-754).
// C++ keeps the mask in the live InstanceScript and persists it on the next
// SetBossState SaveToDB (which returns early when the script has no save
// data); Go has no live InstanceScript, and its only mask consumer — the
// group-lock warning — reads the instance row, so the write goes straight
// to the row via CHAR_UPD_INSTANCE_DATA, preserving the row's data string.
// The row's own difficulty column keys the lookup, so normal/heroic/10/25
// DBC rows resolve exactly as GetDungeonEncounterList does. The
// lastEncounterDungeon LFG finish (FinishDungeon) has no Go LFG flow and
// is recorded on the credit but not acted on.
func (s *session) creditDungeonEncounterKill(ctx context.Context, target combatTarget, creatureEntry uint32) {
	if s == nil || s.server == nil || target.InstanceID == 0 {
		return
	}
	s.server.loadDungeonEncounters(ctx)
	s.server.dungeonEncounterMu.Lock()
	credits := s.server.dungeonEncounters
	s.server.dungeonEncounterMu.Unlock()
	if len(credits) == 0 {
		return
	}
	var difficulty int64
	var mask int64
	var data string
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT difficulty, completedEncounters, data FROM instance WHERE id = ?", target.InstanceID).Scan(&difficulty, &mask, &data); err != nil {
		return
	}
	credit, ok := credits[dungeonEncounterKey{MapID: target.Map, Difficulty: uint32(difficulty), CreditType: dungeonEncounterCreditKillCreature, CreditEntry: creatureEntry}]
	if !ok || credit.Bit >= 32 {
		return
	}
	newMask := uint32(mask) | (1 << credit.Bit)
	if newMask == uint32(mask) {
		return
	}
	_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_INSTANCE_DATA", newMask, data, target.InstanceID)
}

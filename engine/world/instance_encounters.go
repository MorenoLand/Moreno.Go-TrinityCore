package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
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
// mask"). Rows sharing one key append in load order, not overwrite: C++
// stores a DungeonEncounterList per (map, difficulty)
// (ObjectMgr.cpp:6107-6108) and UpdateEncounterState ORs every matching
// row's bit, stopping after the first row marked lastEncounterDungeon
// (InstanceScript.cpp:744-754) — a single-value map would credit only the
// last row when one kill or cast completes two encounters. C++ load-time
// skips Go cannot replicate (invalid LFG dungeon id, duplicate final mark)
// stay in the list: Go has no LFG dungeon table, so the break above is
// the replicated half of that filter. A failed load (missing DBC file, missing world table) leaves the
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
	credits := make(map[dungeonEncounterKey][]dungeonEncounterCredit)
	for rows.Next() {
		var entry, creditType, creditEntry, lastDungeon uint32
		if err := rows.Scan(&entry, &creditType, &creditEntry, &lastDungeon); err != nil {
			continue
		}
		dbc, ok := dbcByID[entry]
		if !ok {
			continue
		}
		key := dungeonEncounterKey{MapID: dbc.mapID, Difficulty: dbc.difficulty, CreditType: creditType, CreditEntry: creditEntry}
		credits[key] = append(credits[key], dungeonEncounterCredit{Bit: dbc.bit, LastEncounterDungeon: lastDungeon})
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
// DBC rows resolve exactly as GetDungeonEncounterList does. A
// lastEncounterDungeon credit finishes the instance's LFG group via
// finishLFGDungeonForInstance (the UpdateEncounterState dungeonId block).
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
	creditList, ok := credits[dungeonEncounterKey{MapID: target.Map, Difficulty: uint32(difficulty), CreditType: dungeonEncounterCreditKillCreature, CreditEntry: creatureEntry}]
	if !ok {
		return
	}
	newMask := uint32(mask)
	var finishedDungeon uint32
	for _, credit := range creditList {
		if credit.Bit >= 32 {
			continue
		}
		newMask |= 1 << credit.Bit
		if credit.LastEncounterDungeon != 0 {
			finishedDungeon = credit.LastEncounterDungeon
			break
		}
	}
	if newMask != uint32(mask) {
		_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_INSTANCE_DATA", newMask, data, target.InstanceID)
	}
	// InstanceScript::UpdateEncounterState (InstanceScript.cpp:756-773):
	// a final-encounter credit finishes the first LFG group found among
	// the instance players, even when the mask bit was already set (the
	// C++ runs the dungeonId block regardless; FinishDungeon's own
	// FINISHED guard makes a repeat finish a no-op).
	if finishedDungeon != 0 {
		s.server.finishLFGDungeonForInstance(target.Map, target.InstanceID, finishedDungeon)
	}
}

// finishLFGDungeonForInstance mirrors the UpdateEncounterState dungeonId
// block (InstanceScript.cpp:756-773): it scans the instance players for
// the first one in an LFG group and finishes that group, then returns —
// other LFG groups in the same instance are never tried, matching the
// C++ return after the first LFG-grouped player. The scan collects group
// ids under the session lock and resolves them after release, so the
// finish runs outside both locks.
func (s *Server) finishLFGDungeonForInstance(mapID, instanceID, dungeonID uint32) {
	if s == nil || dungeonID == 0 {
		return
	}
	s.sessionsMu.RLock()
	var groupIDs []uint64
	for sess := range s.sessions {
		if !sess.authed || !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID || sess.groupID == 0 {
			continue
		}
		groupIDs = append(groupIDs, sess.groupID)
	}
	s.sessionsMu.RUnlock()
	for _, gid := range groupIDs {
		if g := s.getGroup(gid); g == nil || !g.IsLFG {
			continue
		}
		s.completeLFGDungeon(uint32(gid), dungeonID)
		return
	}
}

// loadDungeonCreditSpells mirrors ObjectMgr::LoadSpellScriptNames
// (ObjectMgr.cpp:5765: SELECT spell_id, ScriptName FROM spell_script_names),
// filtered to the dungeon-credit script: spell_gen_dungeon_credit is the
// only spell script whose AfterHit arm credits an encounter
// (spell_generic.cpp:1553-1580). A failed load leaves the set empty and
// marks it done, like loadDungeonEncounters: the cast path never retries
// or logs per cast.
func (s *Server) loadDungeonCreditSpells(ctx context.Context) {
	if s == nil {
		return
	}
	s.dungeonCreditSpellMu.Lock()
	defer s.dungeonCreditSpellMu.Unlock()
	if s.dungeonCreditSpellsLoaded {
		return
	}
	s.dungeonCreditSpellsLoaded = true
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT spell_id FROM spell_script_names WHERE ScriptName = ?", "spell_gen_dungeon_credit")
	if err != nil {
		return
	}
	defer rows.Close()
	spells := make(map[uint32]struct{})
	for rows.Next() {
		var spellID uint32
		if err := rows.Scan(&spellID); err != nil {
			continue
		}
		spells[spellID] = struct{}{}
	}
	s.dungeonCreditSpells = spells
}

// creditDungeonEncounterSpellCast is the spell-cast twin of
// creditDungeonEncounterKill: InstanceScript::UpdateEncounterStateForSpellCast
// (InstanceScript.cpp:783-786) routes to UpdateEncounterState with
// ENCOUNTER_CREDIT_CAST_SPELL (type 1), ORing 1 << Bit into the live mask;
// Go writes the row directly, like the kill twin, because the row is the
// only mask consumer.
func (s *Server) creditDungeonEncounterSpellCast(ctx context.Context, mapID, instanceID, spellID uint32) {
	if s == nil || instanceID == 0 {
		return
	}
	s.loadDungeonEncounters(ctx)
	s.dungeonEncounterMu.Lock()
	credits := s.dungeonEncounters
	s.dungeonEncounterMu.Unlock()
	if len(credits) == 0 {
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	var difficulty int64
	var mask int64
	var data string
	if err := s.CharactersStore.DB.QueryRowContext(ctx, "SELECT difficulty, completedEncounters, data FROM instance WHERE id = ?", instanceID).Scan(&difficulty, &mask, &data); err != nil {
		return
	}
	creditList, ok := credits[dungeonEncounterKey{MapID: mapID, Difficulty: uint32(difficulty), CreditType: dungeonEncounterCreditCastSpell, CreditEntry: spellID}]
	if !ok {
		return
	}
	newMask := uint32(mask)
	var finishedDungeon uint32
	for _, credit := range creditList {
		if credit.Bit >= 32 {
			continue
		}
		newMask |= 1 << credit.Bit
		if credit.LastEncounterDungeon != 0 {
			finishedDungeon = credit.LastEncounterDungeon
			break
		}
	}
	if newMask != uint32(mask) {
		_, _ = s.CharactersStore.ExecStatement(ctx, "CHAR_UPD_INSTANCE_DATA", newMask, data, instanceID)
	}
	// Same UpdateEncounterState dungeonId block as the kill twin: a
	// final-encounter spell credit finishes the instance's LFG group.
	if finishedDungeon != 0 {
		s.finishLFGDungeonForInstance(mapID, instanceID, finishedDungeon)
	}
}

// creatureSpellHasLiveHit mirrors the AfterHit skip set in
// Spell::PreprocessTarget / TargetInfo::DoDamageAndTriggers
// (Spell.cpp:2346-2350, 2629-2650): a bounced reflect, evade, immune, or
// miss verdict nulls _spellHitTarget, so AfterHit never fires for it; a
// landed reflect retargets _spellHitTarget to the caster, so it counts.
func creatureSpellHasLiveHit(hitTargets []uint64, reflected map[uint64]uint8, evaded, immune map[uint64]bool, missed map[uint64]uint8) bool {
	for _, guid := range hitTargets {
		if guid == 0 {
			continue
		}
		if status, ok := reflected[guid]; ok {
			if status == protocol.SpellMissNone {
				return true
			}
			continue
		}
		if evaded[guid] || immune[guid] || missed[guid] != protocol.SpellMissNone {
			continue
		}
		return true
	}
	return false
}

// maybeCreditDungeonEncounterSpellCast is the spell_gen_dungeon_credit
// AfterHit arm (spell_generic.cpp:1553-1580): the script's Load() gate
// admits only TYPEID_UNIT casters, which the creature funnels satisfy by
// construction — the player cast path never reaches this call — and the
// _handled once-per-cast latch is the single call per cast, not one per
// hit target. A wild (non-instance) caster has InstanceID 0 and credits
// nothing, matching GetInstanceScript() returning null.
func (s *Server) maybeCreditDungeonEncounterSpellCast(ctx context.Context, m *creatureMotion, spellID uint32, hitTargets []uint64, reflected map[uint64]uint8, evaded, immune map[uint64]bool, missed map[uint64]uint8) {
	if s == nil || m == nil || m.InstanceID == 0 {
		return
	}
	if !creatureSpellHasLiveHit(hitTargets, reflected, evaded, immune, missed) {
		return
	}
	s.loadDungeonCreditSpells(ctx)
	s.dungeonCreditSpellMu.Lock()
	_, bound := s.dungeonCreditSpells[spellID]
	s.dungeonCreditSpellMu.Unlock()
	if !bound {
		return
	}
	s.creditDungeonEncounterSpellCast(ctx, m.Map, m.InstanceID, spellID)
}

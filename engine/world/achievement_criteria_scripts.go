package world

// Achievement criteria scripts mirroring TrinityCore
// src/server/scripts/World/achievement_scripts.cpp (AddSC_achievement_scripts).
//
// C++ dispatches ACHIEVEMENT_CRITERIA_DATA_TYPE_SCRIPT (AchievementMgr.cpp:371-376)
// through ScriptMgr::OnCriteriaCheck into the AchievementCriteriaScript classes
// below. The Go criteria pipeline evaluates achievement_criteria_data rules in
// meetsCriteriaRequirements; criteriaDataTypeScript rules without this dispatch
// were silently treated as met, so script-gated achievements (e.g. "Save the
// Day") could be credited without their real conditions. checkAchievementCriteriaScript
// is the Go form of that dispatch, keyed by the ScriptName column. Unknown names
// return false, matching C++ GET_SCRIPT_RET's default for unregistered scripts
// (ScriptMgr.cpp:1959).

// CreatureAI::GetData type numbers for the boss-file achievement latches
// (the AI-local enum values, not instance DATA_* ids).
const (
	dataAuriayaCrazyCatLady   = 30063077 // boss_auriaya.cpp:109 DATA_CRAZY_CAT_LADY
	dataAuriayaNineLives      = 30763077 // boss_auriaya.cpp:108 DATA_NINE_LIVES
	dataSjonnirAbuseTheOoze   = 2        // boss_sjonnir.cpp:54 DATA_ABUSE_THE_OOZE
	dataKingDredRaptorsKilled = 2        // boss_king_dred.cpp:43 DATA_RAPTORS_KILLED
	dataBrannSparklinNews     = 1        // halls_of_stone.cpp:121 DATA_BRANN_SPARKLIN_NEWS
)

// bossLatchData reads one CreatureAI::SetData slot off the killed
// creature's motion — the Lua creature:SetData store
// (lua_creature_events.go), guarded by the server motionMu the same
// way the Lua GetData reader takes it. Absent types read 0, the C++
// member default. A nil victim or server reads 0 (fail-closed).
func bossLatchData(s *Server, victim *creatureMotion, dataID uint32) uint32 {
	if s == nil || victim == nil {
		return 0
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return victim.ScriptData[dataID]
}

// checkAchievementCriteriaScript evaluates one DATA_TYPE_SCRIPT criteria gate.
// source is the player earning the criteria; target is the kill target when the
// criteria fires from a kill (nil for creature kills and non-kill criteria),
// mirroring AchievementCriteriaScript::OnCheck(Player* source, Unit* target).
// victim is the killed creature's motion when the criteria fires from a
// creature kill (nil otherwise): the C++ OnCheck receives the kill target
// as a Unit*, and boss-file criteria read the target's AI GetData slots
// off it (boss_auriaya.cpp, boss_sjonnir.cpp, boss_king_dred.cpp) — the
// Lua SetData/GetData store on the motion (lua_creature_events.go).
// The kill-credit path (kill.go creditQuestKills) resolves the victim
// before the update, so a nil victim here is fail-closed, matching the
// unknown-script-name default below.
func checkAchievementCriteriaScript(name string, source, target *session, victim *creatureMotion) bool {
	switch name {
	case "achievement_save_the_day": // scripts/World/achievement_scripts.cpp:53-65
		// BattlegroundWS::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_SAVE_THE_DAY):
		// the victim was carrying a flag and their own team's flag is on its
		// base (BattlegroundWS.cpp:875-879). GetFlagState(playerTarget->GetTeam())
		// reads the victim's team flag; Go's per-team flag state lives on the
		// map's WSG state with WSGFlagStateOnBase == the C++ ON_BASE arm.
		if source == nil || target == nil || target.player == nil {
			return false
		}
		if !sessionInBattlegroundType(target, 2) { // BATTLEGROUND_WS
			return false
		}
		wsg := source.server.getOrCreateWSGState(battlegroundMapForType(2))
		if wsg == nil {
			return false
		}
		wsg.mu.Lock()
		defer wsg.mu.Unlock()
		if teamForRace(target.player.Race) == 0 {
			return wsg.AllianceFlagState == WSGFlagStateOnBase
		}
		return wsg.HordeFlagState == WSGFlagStateOnBase

	case "achievement_bg_av_perfection": // achievement_scripts.cpp:185-197
		// BattlegroundAV::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_AV_PERFECTION):
		// all of the player's own towers controlled by their team, all enemy
		// towers destroyed, and their captain alive (BattlegroundAV.cpp:1523-1566).
		// m_CaptainAlive[0] is Balinda (alliance), [1] is Galvangar (horde).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 1) { // BATTLEGROUND_AV
			return false
		}
		av := source.server.getOrCreateAVState(battlegroundMapForType(1))
		if av == nil {
			return false
		}
		av.mu.Lock()
		defer av.mu.Unlock()
		alliance := teamForRace(source.player.Race) == 0
		ownStart, ownEnd := AVNodeDunBaldarSouth, AVNodeStoneheartBunker
		foeStart, foeEnd := AVNodeIcebloodTower, AVNodeFrostwolfWestTower
		ownTeam, captainAlive := AVTeamAlliance, av.AllianceCaptainAlive
		if !alliance {
			ownStart, ownEnd = AVNodeIcebloodTower, AVNodeFrostwolfWestTower
			foeStart, foeEnd = AVNodeDunBaldarSouth, AVNodeStoneheartBunker
			ownTeam, captainAlive = AVTeamHorde, av.HordeCaptainAlive
		}
		for i := ownStart; i <= ownEnd; i++ {
			if av.Nodes[i].State != AVNodeStateControlled || av.Nodes[i].Owner != ownTeam {
				return false
			}
		}
		for i := foeStart; i <= foeEnd; i++ {
			if av.Nodes[i].State != AVNodeStateDestroyed {
				return false
			}
		}
		return captainAlive

	case "achievement_everything_counts": // achievement_scripts.cpp:171-183
		// BattlegroundAV::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_EVERYTHING_COUNTS):
		// both mines owned by the player's team (BattlegroundAV.cpp:1516-1521).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 1) { // BATTLEGROUND_AV
			return false
		}
		av := source.server.getOrCreateAVState(battlegroundMapForType(1))
		if av == nil {
			return false
		}
		av.mu.Lock()
		defer av.mu.Unlock()
		ownTeam := AVTeamAlliance
		if teamForRace(source.player.Race) != 0 {
			ownTeam = AVTeamHorde
		}
		return av.Mines[AVNorthMine].Owner == ownTeam && av.Mines[AVSouthMine].Owner == ownTeam

	case "achievement_bg_control_all_nodes": // achievement_scripts.cpp:39-51
		// BattlegroundAB::IsAllNodesControlledByTeam: all 5 dynamic nodes
		// occupied by the player's team (BattlegroundAB.cpp:696-705).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 3) { // BATTLEGROUND_AB
			return false
		}
		ab := source.server.getOrCreateABState(battlegroundMapForType(3))
		if ab == nil {
			return false
		}
		ab.mu.Lock()
		defer ab.mu.Unlock()
		want := ABNodeStateControlledAlliance
		if teamForRace(source.player.Race) != 0 {
			want = ABNodeStateControlledHorde
		}
		for i := range ab.Nodes {
			if ab.Nodes[i].State != want {
				return false
			}
		}
		return true

	case "achievement_bg_ic_resource_glut": // achievement_scripts.cpp:67-79
		// Both Isle of Conquest resource buffs (BattlegroundIC.h:707-708).
		if source == nil {
			return false
		}
		return source.playerHasAura(68719) && source.playerHasAura(68720)

	case "achievement_flirt_with_disaster_perf_check": // achievement_scripts.cpp:265-280
		// One of the three Love-is-in-the-Air perfumes (70233/70234/70235).
		if source == nil {
			return false
		}
		return source.playerHasAura(70233) || source.playerHasAura(70234) || source.playerHasAura(70235)

	case "achievement_arena_2v2_kills", "achievement_arena_3v3_kills", "achievement_arena_5v5_kills": // achievement_scripts.cpp:132-151
		// InArena() && the arena's type matches the script's _arenaType
		// (registered as achievement_arena_NvN_kills with ARENA_TYPE_2v2/3v3/5v5).
		if source == nil {
			return false
		}
		want := uint8(2)
		switch name {
		case "achievement_arena_3v3_kills":
			want = 3
		case "achievement_arena_5v5_kills":
			want = 5
		}
		for i := range source.bgQueues {
			e := &source.bgQueues[i]
			if e.Active && e.IsArena && e.Status == BGStatusInProgress && e.ArenaType == want {
				return true
			}
		}
		return false

	case "achievement_tilted": // achievement_scripts.cpp:223-241
		// In an Argent Tournament ring area with a duel whose DuelInfo::IsMounted
		// is set — C++ captures isMounted = (spellId == 62875), the joust duel
		// spell (SpellEffects.cpp:3866-3868); Go captures it as duelMounted.
		if source == nil || source.player == nil {
			return false
		}
		switch source.areaID {
		case 4658, 4670, 4671, 4672, 4673, 4669: // AREA_ARGENT_TOURNAMENT_FIELDS + rings
		default:
			return false
		}
		return source.duelPartner != 0 && source.duelMounted

	case "achievement_killed_exp_or_honor_target": // achievement_scripts.cpp:282-292
		// Player::isHonorOrXPTarget (Player.cpp:24064-24083): the victim must be
		// above the killer's gray level. Player victims are never critters or
		// totems and the scaled-XP config defaults to 0, so the gray check is
		// the modeled arm (same verdict as the kill path in death.go).
		if source == nil || source.player == nil || target == nil || target.player == nil {
			return false
		}
		return uint32(target.player.Level) > grayLevel(uint32(source.player.Level))

	case "achievement_sickly_gazelle": // achievement_scripts.cpp:153-169
		// The kill target is a mounted player (target->ToPlayer()->IsMounted()).
		if target == nil {
			return false
		}
		return target.isPlayerMounted()

	case "achievement_resilient_victory": // achievement_scripts.cpp:25-37
		// BattlegroundAB::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_RESILIENT_VICTORY):
		// the player's own team holds the 500-point disadvantage latch
		// (BattlegroundAB.cpp:707-713).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 3) { // BATTLEGROUND_AB
			return false
		}
		ab := source.server.getOrCreateABState(battlegroundMapForType(3))
		if ab == nil {
			return false
		}
		ab.mu.Lock()
		defer ab.mu.Unlock()
		return ab.Scores500Disadvantage[teamForRace(source.player.Race)]

	case "achievement_bg_sa_defense_of_ancients": // achievement_scripts.cpp:199-211
		// BattlegroundSA::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_DEFENSE_OF_THE_ANCIENTS):
		// the player is a defender and no gate has been destroyed
		// (BattlegroundSA.cpp:1061-1066).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 9) { // BATTLEGROUND_SA
			return false
		}
		sa := source.server.getOrCreateSAState(battlegroundMapForType(9))
		if sa == nil {
			return false
		}
		sa.mu.Lock()
		defer sa.mu.Unlock()
		return teamForRace(source.player.Race) != sa.Attackers && !sa.GateDestroyed

	case "achievement_not_even_a_scratch": // achievement_scripts.cpp:244-256
		// BattlegroundSA::CheckAchievementCriteriaMeet(BG_CRITERIA_CHECK_NOT_EVEN_A_SCRATCH):
		// all of the player's team's demolishers are still alive
		// (BattlegroundSA.cpp:1061-1066).
		if source == nil || source.player == nil {
			return false
		}
		if !sessionInBattlegroundType(source, 9) { // BATTLEGROUND_SA
			return false
		}
		sa := source.server.getOrCreateSAState(battlegroundMapForType(9))
		if sa == nil {
			return false
		}
		sa.mu.Lock()
		defer sa.mu.Unlock()
		return sa.DemolishersAlive[teamForRace(source.player.Race)]

	case "achievement_crazy_cat_lady": // boss_auriaya.cpp:679-695
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_CRAZY_CAT_LADY
		// 30063077) — true while no Sanctum Sentry died: the latch starts
		// set (ctor/Reset, boss_auriaya.cpp:137/:142) and the sentry
		// JustDied relay (boss_auriaya.cpp:301) clears it via DoAction(0);
		// the Lua port keeps it in the boss motion's SetData store.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataAuriayaCrazyCatLady) != 0

	case "achievement_nine_lives": // boss_auriaya.cpp:661-677
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_NINE_LIVES
		// 30763077) — true once a Feral Defender really died: the latch
		// starts clear and the defender JustDied relay
		// (boss_auriaya.cpp:452) sets it via DoAction(1); the feign-death
		// revives (DamageTaken :427-446) never reach JustDied.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataAuriayaNineLives) != 0

	case "achievement_abuse_the_ooze": // boss_sjonnir.cpp:301-319
		// OnCheck: Sjonnir->AI()->GetData(DATA_ABUSE_THE_OOZE 2) >= 5 —
		// the counter starts at 0 (Reset :89) and each Iron Sludge
		// JustDied (boss_sjonnir.cpp:291) increments it via DoAction(1);
		// the Lua port keeps it in the boss motion's SetData store.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSjonnirAbuseTheOoze) >= 5

	case "achievement_brann_spankin_new": // halls_of_stone.cpp:719-735
		// OnCheck: Brann->AI()->GetData(DATA_BRANN_SPARKLIN_NEWS 1) —
		// the latch starts true (Initialize :289) and Brann's
		// DamageTaken (halls_of_stone.cpp:405-408) clears it the first
		// time he takes damage while the tribunal is IN_PROGRESS; the
		// Lua port keeps it in the boss motion's SetData store
		// (npc_brann_hos.lua: ev5 spawn init + ev9 DamageTaken leg).
		// The Reset re-init leg is vacuous: the latch can only be false
		// while escorting, and Reset skips Initialize while escorting
		// (halls_of_stone.cpp:304-313).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataBrannSparklinNews) != 0

	case "achievement_king_dred": // boss_king_dred.cpp:286-303
		// OnCheck: Dred->AI()->GetData(DATA_RAPTORS_KILLED 2) >= 6 —
		// the counter starts at 0 (Reset :70) and each raptor JustDied
		// (boss_king_dred.cpp:220/:276) increments it via DoAction(1);
		// the Lua port keeps it in the boss motion's SetData store.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataKingDredRaptorsKilled) >= 6

	// The remaining registrations are unportable without their Go state:
	// - achievement_bg_ic_glaive_grave / achievement_bg_ic_mowed_down /
	//   achievement_bg_sa_artillery: GetVehicleCreatureBase entry checks; Go has no
	//   vehicle model. C++ returns false when the checks can't be met; without the
	//   state, false is the only honest verdict, so these are fail-closed.
	case "achievement_bg_ic_glaive_grave",
		"achievement_bg_ic_mowed_down",
		"achievement_bg_sa_artillery":
		return false
	}
	return false
}

// sessionInBattlegroundType is the Go form of Player::GetBattleground() != nil
// for a battleground type: the session has an active, in-progress queue entry
// for that type. (Arenas are excluded — these checks are all BG-zone checks.)
func sessionInBattlegroundType(sess *session, bgTypeID uint32) bool {
	if sess == nil {
		return false
	}
	for i := range sess.bgQueues {
		e := &sess.bgQueues[i]
		if e.Active && !e.IsArena && e.BgTypeID == bgTypeID && e.Status == BGStatusInProgress {
			return true
		}
	}
	return false
}

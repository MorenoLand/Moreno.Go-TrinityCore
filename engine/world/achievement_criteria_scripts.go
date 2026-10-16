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

// checkAchievementCriteriaScript evaluates one DATA_TYPE_SCRIPT criteria gate.
// source is the player earning the criteria; target is the kill target when the
// criteria fires from a kill (nil for creature kills and non-kill criteria),
// mirroring AchievementCriteriaScript::OnCheck(Player* source, Unit* target).
func checkAchievementCriteriaScript(name string, source, target *session) bool {
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

	// The remaining registrations are unportable without their Go state:
	// - achievement_bg_sa_defense_of_ancients / achievement_not_even_a_scratch:
	//   BattlegroundSA's _allVehiclesAlive / _gateDestroyed (BattlegroundSA.cpp:1040-1049)
	//   have no Go twin.
	// - achievement_bg_ic_glaive_grave / achievement_bg_ic_mowed_down /
	//   achievement_bg_sa_artillery: GetVehicleCreatureBase entry checks; Go has no
	//   vehicle model. C++ returns false when the checks can't be met; without the
	//   state, false is the only honest verdict, so these are fail-closed.
	case "achievement_bg_sa_defense_of_ancients",
		"achievement_not_even_a_scratch",
		"achievement_bg_ic_glaive_grave",
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

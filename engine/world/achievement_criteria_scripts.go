package world

import (
	"math"
	"strconv"
)

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
	dataAuriayaCrazyCatLady            = 30063007 // boss_auriaya.cpp:109 DATA_CRAZY_CAT_LADY
	dataAuriayaNineLives               = 30763077 // boss_auriaya.cpp:108 DATA_NINE_LIVES
	dataSjonnirAbuseTheOoze            = 2        // boss_sjonnir.cpp:54 DATA_ABUSE_THE_OOZE
	dataKingDredRaptorsKilled          = 2        // boss_king_dred.cpp:43 DATA_RAPTORS_KILLED
	dataBrannSparklinNews              = 1        // halls_of_stone.cpp:121 DATA_BRANN_SPARKLIN_NEWS
	dataLichKingPlagueStack            = 70337    // boss_the_lich_king.cpp:369 DATA_PLAGUE_STACK
	dataLichKingVileSpiritExplosions   = 45814622 // boss_the_lich_king.cpp:370 DATA_VILE
	dataXT002GravityBomb               = 3        // boss_xt002.cpp:114 DATA_GRAVITY_BOMB_CASUALTY
	dataXT002HealthRecovered           = 2        // boss_xt002.cpp:109-114 DATA_HEALTH_RECOVERED
	dataVolkhanShattered               = 2042     // boss_volkhan.cpp:77 DATA_SHATTER_RESISTANT
	dataIchoronDehydration             = 1        // boss_ichoron.cpp:71 DATA_DEHYDRATION
	dataThaddiusPolarity               = 1        // boss_thaddius.cpp:77 DATA_POLARITY_CROSSED
	dataKelthuzadAbomsKilled           = 1        // boss_kelthuzad.cpp:83 DATA_ABOMINATION_DEATH_COUNT
	dataAlgalonFedOnTears              = 30043005 // boss_algalon_the_observer.cpp:184 DATA_HAS_FED_ON_TEARS
	dataFaerlinaFrenzyDispels          = 1        // boss_faerlina.cpp:65 DATA_FRENZY_DISPELS
	dataSindragosaMysticBuffet         = 0        // boss_sindragosa.cpp:141 DATA_MYSTIC_BUFFET_STACK
	dataGarfrostPermafrost             = 0        // boss_forgemaster_garfrost.cpp:66 ACHIEV_DOESNT_GO_TO_ELEVEN
	dataGunshipShipVisits              = 5        // boss_icecrown_gunship_battle.cpp:212 ACTION_SHIP_VISITS
	dataHadronoxWebbedDoors            = 3        // boss_hadronox.cpp:119 DATA_HADRONOX_WEBBED_DOORS
	dataFreyaGettingBackToNature       = 1        // boss_freya.cpp:210 DATA_GETTING_BACK_TO_NATURE
	dataFreyaKnockOnWood               = 2        // boss_freya.cpp:211 DATA_KNOCK_ON_WOOD
	dataVezaxSmellSaronite             = 31813188 // boss_general_vezax.cpp:98 DATA_SMELL_SARONITE
	dataVezaxShadowdodger              = 29962997 // boss_general_vezax.cpp:99 DATA_SHADOWDODGER
	dataFestergutInoculatedStack       = 69291    // boss_festergut.cpp:81 DATA_INOCULATED_STACK
	dataHeiganSafetyDance              = 19962139 // boss_heigan.cpp:64 DATA_SAFETY_DANCE
	dataLoathebSporeLoser              = 21822183 // boss_loatheb.cpp:55 DATA_ACHIEVEMENT_SPORE_LOSER
	dataSartharionTwilightAchievements = 5        // obsidian_sanctum.h:35 TWILIGHT_ACHIEVEMENTS
	dataMoorabiLessRabi                = 1        // boss_moorabi.cpp:60 DATA_LESS_RABI
	dataKelesethOnTheRocks             = 8        // boss_keleseth.cpp:67 DATA_ON_THE_ROCKS
	dataNadoxRespectElders             = 6        // boss_elder_nadox.cpp:54 DATA_RESPECT_YOUR_ELDERS
	dataGalDarahShareTheLove           = 1        // boss_gal_darah.cpp:81 DATA_SHARE_THE_LOVE
	dataTelestraSplit                  = 2        // boss_magus_telestra.cpp:62 DATA_SPLIT_PERSONALITY
	dataDevourerThreeFaced             = 1        // boss_devourer_of_souls.cpp:125 DATA_THREE_FACED
	dataZuramatVoidDance               = 2153     // boss_zuramat.cpp:48 DATA_VOID_DANCE
	dataJedogaVolunteerWork            = 1        // boss_jedoga_shadowseeker.cpp:137 DATA_VOLUNTEER_WORK
	dataKrikthirWatchersDead           = 1        // Lua-side slot; boss_krikthir_the_gatewatcher.cpp:1036-1061 reads instance watcher liveness
	dataMimironSetupBomb               = 1        // boss_mimiron.cpp:187 DATA_SETUP_BOMB
	dataMimironSetupMine               = 0        // boss_mimiron.cpp:186 DATA_SETUP_MINE
	dataMimironSetupRocket             = 2        // boss_mimiron.cpp:188 DATA_SETUP_ROCKET
	dataMimironFirefighter             = 4        // boss_mimiron.cpp:190 DATA_FIREFIGHTER
	dataFlameLeviathanShutout          = 29112912 // boss_flame_leviathan.cpp:143 DATA_SHUTOUT
	dataFlameLeviathanOrbit            = 1        // boss_flame_leviathan.cpp:144 DATA_ORBIT_ACHIEVEMENTS
	dataAssemblyPhase3                 = 1        // boss_assembly_of_iron.cpp:133 DATA_PHASE_3
	dataIgnisShattered                 = 29252926 // boss_ignis.cpp:86 DATA_SHATTERED
	dataXT002HardMode                  = 1        // boss_xt002.cpp:111 DATA_HARD_MODE
	dataRazorscaleIronDwarf            = 29232924 // boss_razorscale.cpp:164 DATA_IRON_DWARF_MEDIUM_RARE
	dataRazorscaleQuickShave           = 29192921 // boss_razorscale.cpp:163 DATA_QUICK_SHAVE
	dataBronjahmSoulPower              = 1        // boss_bronjahm.cpp:68 DATA_SOUL_POWER
	dataAnomalusChaosTheory            = 1        // boss_anomalus.cpp:70 DATA_CHAOS_THEORY
	dataEregosRubyVoid                 = 0        // boss_eregos.cpp:76 DATA_RUBY_VOID
	dataEregosEmeraldVoid              = 1        // boss_eregos.cpp:77 DATA_EMERALD_VOID
	dataEregosAmberVoid                = 2        // boss_eregos.cpp:78 DATA_AMBER_VOID
	dataSkadiLoveToSkadi               = 0        // boss_skadi.cpp:112 DATA_LOVE_TO_SKADI
	dataSvalaIncredibleHulk            = 2043     // boss_svala.cpp:125 DATA_INCREDIBLE_HULK
	dataYmironKingsBane                = 2157     // boss_ymiron.cpp:99 DATA_KINGS_BANE
	dataThorimDontStandInLightning     = 29712972 // boss_thorim.cpp:315 ACHIEVEMENT_DONT_STAND_IN_THE_LIGHTNING
	dataThorimLoseYourIllusion         = 31763183 // boss_thorim.cpp:317 ACHIEVEMENT_LOSE_YOUR_ILLUSION
	dataSapphironHundredClub           = 21462147 // boss_sapphiron.cpp:88 DATA_THE_HUNDRED_CLUB
	dataTrollgoreConsumptionJunction   = 1        // boss_trollgore.cpp:57 DATA_CONSUMPTION_JUNCTION
	dataDreamPortalMissed              = 2        // boss_valithria_dreamwalker.cpp:149 MISSED_PORTALS
	dataMadeAMess                      = 45374613 // boss_deathbringer_saurfang.cpp:207 DATA_MADE_A_MESS
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

// vehicleCreatureEntry is the Go form of Unit::GetVehicleCreatureBase()->
// GetEntry(): the entry of the creature vehicle the player is riding.
// The base GUID is the session's VehicleGUID stamp (vehicle.go boarding
// path); the kit is created at creature spawn with the template entry
// (creaturemotion.go:1374/1582). A player base (IsPlayer kit) reads
// false — C++ ToCreature() on a player base is null — as does a
// missing kit or a player who is not seated.
func vehicleCreatureEntry(source *session) (uint32, bool) {
	if source == nil || source.player == nil || source.server == nil || source.player.VehicleGUID == 0 {
		return 0, false
	}
	kit := source.server.getVehicleKit(source.player.Map, source.player.InstanceID, source.player.VehicleGUID)
	if kit == nil || kit.IsPlayer {
		return 0, false
	}
	return kit.CreatureEntry, true
}

// vehicleID is the Go form of Unit::GetVehicle()->GetVehicleInfo()->ID:
// the DBC vehicle ID of the vehicle the player is riding (VehicleKit.VehicleID,
// vehicle.go:68). A player base reads false, as does a missing kit or a
// player who is not seated — the same lookup vehicleCreatureEntry uses.
func vehicleID(source *session) (uint32, bool) {
	if source == nil || source.player == nil || source.server == nil || source.player.VehicleGUID == 0 {
		return 0, false
	}
	kit := source.server.getVehicleKit(source.player.Map, source.player.InstanceID, source.player.VehicleGUID)
	if kit == nil || kit.IsPlayer {
		return 0, false
	}
	return kit.VehicleID, true
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
		// 30063007) — true while no Sanctum Sentry died: the latch starts
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

	case "achievement_been_waiting_long_time": // boss_the_lich_king.cpp:2827-2832
		// OnCheck: target->GetAI()->GetData(DATA_PLAGUE_STACK 70337)
		// >= 30 — the running peak: C++ SetData is max() (:674-677)
		// and the Lua twin keeps the peak in the boss motion's SetData
		// store (boss_the_lich_king.lua:1011-1012/:1044-1045). Known
		// pre-existing Lua delta (header :988-997): the peak freezes
		// after the first plague jump (jump-aura caster attribution),
		// so this may under-fire on multi-jump fights.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataLichKingPlagueStack) >= 30

	case "achievement_neck_deep_in_vile": // boss_the_lich_king.cpp:2835-2846
		// OnCheck: !target->GetAI()->GetData(DATA_VILE 45814622) —
		// the vile-spirit explosion counter: C++ SetData adds
		// (:677-679) and Reset zeroes it (:525). The Lua twin
		// writes 1 per non-empty 70502 damage-select
		// (boss_the_lich_king.lua:3330); the Go SetData store
		// overwrites, so any explosion reads 1. No Reset clear:
		// Go fires ev23 before ev4 in the kill paths
		// (lua_creature_events.go:6954, kill.go:1091), so a
		// Reset-time clear would zero the latch before this
		// kill-credit read — the same wipe-persistence delta as
		// the plague-stack peak freeze.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataLichKingVileSpiritExplosions) == 0

	case "achievement_flu_shot_shortage": // boss_festergut.cpp:440-450
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_INOCULATED_STACK
		// 69291) < 3 — the running peak: C++ SetData is max() (:261-264)
		// and the Lua twin keeps the peak in the boss motion's SetData
		// store (boss_festergut.lua, blightedSporesRemove). Dormant until
		// Spell.dbc ships the Gas Spore 69278 -> Blighted Spores 69290
		// trigger link, so no aura can apply yet and the latch reads 0
		// (< 3) — matching C++ for a zero-spore fight.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFestergutInoculatedStack) < 3

	case "achievement_nerf_gravity_bombs": // boss_xt002.cpp:1009-1014
		// OnCheck: !GetData(DATA_GRAVITY_BOMB_CASUALTY 3) — true
		// while nobody died to a gravity bomb: the latch starts clear
		// (ctor) and the gravity-bomb aura periodic
		// (boss_xt002.cpp:739) sets it; the Lua twin writes it at
		// boss_xt002.lua:401. Absent reads 0, the C++ member default.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataXT002GravityBomb) == 0

	case "achievement_thaddius_shocking": // boss_thaddius.cpp:1241-1243
		// OnCheck: GetData(DATA_POLARITY_CROSSED 1) — the
		// shockingEligibility latch, true in the ctor (:164) and
		// cleared (:286) / re-armed (:241) by the polarity legs; the
		// Lua twin keeps it in the boss motion's SetData store
		// (boss_thaddius.lua:501/:543/:562).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataThaddiusPolarity) != 0

	case "achievement_just_cant_get_enough": // boss_kelthuzad.cpp:1014-1024
		// OnCheck: the instance's Kel'Thuzad (GetGuidData
		// DATA_KELTHUZAD) AI GetData(DATA_ABOMINATION_DEATH_COUNT 1)
		// >= 18. On the Go kill-credit path the victim IS Kel'Thuzad
		// (creditQuestKills resolves the killed creature), so the
		// latch read is the same slot; the Lua twin increments it on
		// ACTION_ABOMINATION_DIED (boss_kelthuzad.lua:1073) and zeroes
		// it at Reset (:445/:464).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataKelthuzadAbomsKilled) >= 18

	case "achievement_he_feeds_on_your_tears": // boss_algalon_the_observer.cpp:1270-1280
		// OnCheck: !GetData(DATA_HAS_FED_ON_TEARS 30043005) — true
		// while Algalon never killed a player: the latch is set in
		// KilledUnit (boss_algalon_the_observer.cpp:500-510) and lives
		// on the AI object (ctor-only clear :290), so it survives
		// wipes; the Lua twin writes it in algalonTargetDied
		// (boss_algalon_the_observer.lua) and the per-motion
		// ScriptData store is fresh only on respawn, the C++ twin.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataAlgalonFedOnTears) == 0

	case "achievement_momma_said_knock_you_out": // boss_faerlina.cpp:256-265
		// OnCheck: !GetData(DATA_FRENZY_DISPELS 1) — true while
		// Faerlina's Frenzy was never dispelled: the counter
		// increments in SpellHit (boss_faerlina.cpp:128-140) when
		// Widow's Embrace hits her; the Lua twin increments it in
		// faerlinaSpellHit (boss_faerlina.lua) and zeroes it at Reset.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFaerlinaFrenzyDispels) == 0

	case "achievement_all_you_can_eat": // boss_sindragosa.cpp:1513-1524
		// OnCheck: GetData(DATA_MYSTIC_BUFFET_STACK 0) <= 5 — the
		// running peak of the Mystic Buffet aura stack on any player,
		// tracked in SpellHitTarget (boss_sindragosa.cpp:432-441) off
		// the difficulty-resolved 70127 hit; the Lua twin keeps the
		// peak in the boss motion's SetData store
		// (boss_sindragosa.lua). Dormant until the DBC aura trigger
		// (70128 -> 70127) runs: without Spell.dbc the helper hits
		// never occur and the slot stays 0, the fail-closed default.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSindragosaMysticBuffet) <= 5

	case "achievement_doesnt_go_to_eleven": // boss_forgemaster_garfrost.cpp:335-349
		// OnCheck: GetData(ACHIEV_DOESNT_GO_TO_ELEVEN 0) <= 10 — the
		// running peak of the Permafrost helper aura stack on any
		// player, tracked in SpellHitTarget
		// (boss_forgemaster_garfrost.cpp:174-186) off the
		// RAID_MODE(68786, 70336) helper hit; the Lua twin keeps the
		// peak in the boss motion's SetData store
		// (boss_forgemaster_garfrost.lua). Dormant until the DBC aura
		// trigger (70326 -> helper) runs: without Spell.dbc the helper
		// hits never occur and the slot stays 0, the fail-closed
		// default.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataGarfrostPermafrost) <= 10

	case "achievement_im_on_a_boat": // boss_icecrown_gunship_battle.cpp:2209-2218
		// OnCheck: GetData(ACTION_SHIP_VISITS 5) <= 2 — the max
		// visits to the enemy ship over all players, counted per
		// player GUID in npc_gunshipAI::SetGUID
		// (boss_icecrown_gunship_battle.cpp:803-813), fed by the
		// deck-aura apply (spell_igb_on_gunship_deck :1808-1817);
		// the Lua twin keeps the per-player counts in a table and
		// writes the running max into the gunship motion's SetData
		// store (boss_icecrown_gunship_battle.lua). Dormant until the
		// deck auras (70120/70121) are applied by the engine.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataGunshipShipVisits) <= 2

	case "achievement_hadronox_denied": // boss_hadronox.cpp:1116-1131
		// OnCheck: !GetData(DATA_HADRONOX_WEBBED_DOORS 3) — true
		// while Hadronox never webbed the doors: the latch is set in
		// MovementInform at the final step
		// (boss_hadronox.cpp:197-208); the Lua twin sets it when the
		// SetStep machine reaches the final step
		// (boss_hadronox.lua). Absent reads 0, the C++ member default.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataHadronoxWebbedDoors) == 0

	case "achievement_getting_back_to_nature": // boss_freya.cpp:1612-1621
		// OnCheck: target->GetAI()->GetData(DATA_GETTING_BACK_TO_NATURE
		// 1) >= 25 — the attuned-to-nature stack, 150 at engage
		// (:386-388), lowered only by the REMOVE_*STACK hits
		// (:520/:678/:684); the Lua twin keeps the counter in the
		// boss motion's SetData store (boss_freya.lua). The
		// decrement legs are dormant until the wave/elemental legs
		// land; in the Go-modeled fight no trio can be defeated, so
		// the counter stays 150 — matching C++ for a zero-trio-
		// defeat fight.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFreyaGettingBackToNature) >= 25

	case "achievement_knock_on_wood": // boss_freya.cpp:1623-1632
		// OnCheck: GetData(DATA_KNOCK_ON_WOOD 2) >= 1 — the
		// alive-at-pull elder count; the Lua twin writes it at engage
		// (boss_freya.lua) and zeroes it at Reset.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFreyaKnockOnWood) >= 1

	case "achievement_knock_knock_on_wood": // boss_freya.cpp:1634-1643
		// OnCheck: GetData(DATA_KNOCK_ON_WOOD 2) >= 2, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFreyaKnockOnWood) >= 2

	case "achievement_knock_knock_knock_on_wood": // boss_freya.cpp:1645-1655
		// OnCheck: GetData(DATA_KNOCK_ON_WOOD 2) == 3, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFreyaKnockOnWood) == 3

	case "achievement_smell_saronite": // boss_general_vezax.cpp:575-595
		// OnCheck: Vezax->AI()->GetData(DATA_SMELL_SARONITE
		// 31813188) — true from ctor/Reset until a Saronite Vapor
		// dies (DoAction(ACTION_VAPORS_DIE) :268-270); the Lua twin
		// keeps it in the boss motion's SetData store
		// (boss_general_vezax.lua: ev5/ev23 set, ev38 clear). The
		// clear leg is dormant: the vapor summon machine is unported,
		// so in the Go-modeled fight no vapor can die and the latch
		// stays 1, matching C++ for a zero-vapor-death fight.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataVezaxSmellSaronite) != 0

	case "achievement_shadowdodger": // boss_general_vezax.cpp:555-573
		// OnCheck: Vezax->AI()->GetData(DATA_SHADOWDODGER
		// 29962997) — true from ctor/Reset until a player is hit by
		// SPELL_SHADOW_CRASH_HIT 62659 (SpellHitTarget :222-226); the
		// Lua twin keeps it in the boss motion's SetData store
		// (boss_general_vezax.lua: vezaxReset/vezaxSpawned set, ev15
		// clear). Dormant until Spell.dbc ships the 62660 -> 62659
		// trigger link, so in the Go-modeled fight no 62659 hit can
		// occur and the latch stays 1, matching C++ for a zero-hit
		// fight — the same dormancy as the LK spell-script legs.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataVezaxShadowdodger) != 0

	case "achievement_safety_dance": // boss_heigan.cpp:259-278
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_SAFETY_DANCE
		// 19962139) — true while no player died: the latch is set at
		// JustEngagedWith (boss_heigan.cpp:127) and cleared in
		// KilledUnit on a player kill (:98-104); the Lua twin keeps
		// it in the boss motion's SetData store (boss_heigan.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataHeiganSafetyDance) != 0

	case "achievement_spore_loser": // boss_loatheb.cpp:154-166
		// OnCheck: target->GetAI()->GetData(DATA_ACHIEVEMENT_SPORE_LOSER
		// 21822183) — true while no spore died: the latch starts set
		// (ctor/Reset, boss_loatheb.cpp:65/:72) and the spore death
		// (SummonedCreatureDies :84-86) clears it; the Lua twin keeps
		// it in the boss motion's SetData store (boss_loatheb.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataLoathebSporeLoser) != 0

	case "achievement_twilight_assist": // obsidian_sanctum.cpp:1021-1031
		// OnCheck: target->GetAI()->GetData(TWILIGHT_ACHIEVEMENTS) >= 1 —
		// the drake count from FetchDragons (boss_sartharion.cpp:283-347);
		// the Lua twin writes it into the Sartharion motion's SetData
		// store at engage (boss_sartharion.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSartharionTwilightAchievements) >= 1

	case "achievement_twilight_duo": // obsidian_sanctum.cpp:1032-1042
		// OnCheck: GetData(TWILIGHT_ACHIEVEMENTS) >= 2, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSartharionTwilightAchievements) >= 2

	case "achievement_twilight_zone": // obsidian_sanctum.cpp:1043-1053
		// OnCheck: GetData(TWILIGHT_ACHIEVEMENTS) == 3, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSartharionTwilightAchievements) == 3

	case "achievement_once_bitten_twice_shy_n", "achievement_once_bitten_twice_shy_v": // boss_blood_queen_lana_thel.cpp:793-826
		// OnCheck: WasVampire(source GUID) over the _vampires GuidSet
		// (boss_blood_queen_lana_thel.cpp:466-469/:508) — _n is
		// !WasVampire, _v is WasVampire. The Lua twin records each
		// boss-cast bite as a "vampire_<guidLow>" key in the shared
		// instanceData table (boss_blood_queen_lana_thel.lua),
		// cleared at engage; GetGUIDLow is the low 32 bits of the
		// player GUID on both sides (lua_player.go:105).
		if source == nil || source.player == nil || source.server == nil || source.server.Features == nil || source.server.Features.Scripts == nil {
			return false
		}
		v, ok := source.server.Features.Scripts.InstanceDataNumber(631, source.player.InstanceID, "vampire_"+strconv.FormatUint(uint64(uint32(source.player.GUID)), 10))
		wasVampire := ok && v >= 1
		if name == "achievement_once_bitten_twice_shy_v" {
			return wasVampire
		}
		return !wasVampire

	case "achievement_ive_gone_and_made_a_mess": // boss_deathbringer_saurfang.cpp:1238-1256
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_MADE_A_MESS
		// 45374613) — true while _fallenChampionCastCount < 3/5
		// (10/25-man, :540-546): the count increments in DoAction(
		// ACTION_MARK_OF_THE_FALLEN_CHAMPION) (:580-592), which the
		// blood-link aura periodic tick (:1028) fires at full energy.
		// The Lua twin keeps the latch in Saurfang's SetData store
		// (boss_deathbringer_saurfang.lua): set at engage, cleared
		// when the count reaches the 3/5 threshold. Dormant clear:
		// the blood-link tick is an ev1 aura handler, so the count
		// only moves once Spell.dbc ships.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMadeAMess) != 0

	case "achievement_less_rabi": // boss_moorabi.cpp:198-215
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_LESS_RABI 1) —
		// true while Moorabi never transformed: the latch starts set
		// (ctor/Reset, boss_moorabi.cpp:83) and the SpellHit(55098) landing
		// (:128-136) clears it; the Lua twin writes it in the boss
		// motion's SetData store (boss_moorabi.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMoorabiLessRabi) != 0

	case "achievement_on_the_rocks": // boss_keleseth.cpp:372-380
		// OnCheck: target->GetAI()->GetData(DATA_ON_THE_ROCKS 8) — true
		// while no frost tomb died: the latch starts set (ctor/Reset,
		// boss_keleseth.cpp:131/:140) and the frost-tomb JustDied relay
		// (:99-105) clears it; the Lua twin keeps it in the boss
		// motion's SetData store (boss_keleseth.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataKelesethOnTheRocks) != 0

	case "achievement_respect_your_elders": // boss_elder_nadox.cpp:287-296
		// OnCheck: target->GetAI()->GetData(DATA_RESPECT_YOUR_ELDERS 6) —
		// true while no Ahn'kahar Guardian died: the latch starts set
		// (Reset, boss_elder_nadox.cpp:66-70) and SummonedCreatureDies
		// (:94-100) clears it on a guardian death; the Lua twin keeps
		// it in the boss motion's SetData store (boss_elder_nadox.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataNadoxRespectElders) != 0

	case "achievement_consumption_junction": // boss_trollgore.cpp:342-354
		// OnCheck: target->ToCreature()->AI()->GetData(DATA_CONSUMPTION_JUNCTION 1)
		// — true while the boss never stacked Consume to 10: the latch starts
		// set (Initialize/Reset, boss_trollgore.cpp:84-87/:89-93) and the
		// UpdateAI stack check (:154-159) clears it when the Consume-buff
		// (49381/59805) stack exceeds 9; the Lua twin keeps it in the boss
		// motion's SetData store (boss_trollgore.lua: consumptionJunctionTick
		// 1s poll, engage set, alive-gated ev23 re-arm). Known delta: the
		// buff itself is applied by the unported spell_trollgore_consume
		// SpellScript, so if Go never stacks it the gate over-awards vs
		// C++ on slow kills.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataTrollgoreConsumptionJunction) != 0

	case "achievement_share_the_love": // boss_gal_darah.cpp:341-358
		// OnCheck: GalDarah->AI()->GetData(DATA_SHARE_THE_LOVE 1) >= 5 —
		// the count of distinct impaled players, fed by the impaling
		// charge SpellScript (boss_gal_darah.cpp:259-276); the Lua twin
		// counts distinct spell-hit targets in the boss motion's
		// SetData store (boss_gal_darah.lua) and zeroes it at Reset.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataGalDarahShareTheLove) >= 5

	case "achievement_split_personality": // boss_magus_telestra.cpp:362-381
		// OnCheck: Telestra->AI()->GetData(DATA_SPLIT_PERSONALITY 2) == 2 —
		// the split counter, incremented when the 3rd death of a split
		// arrives with both gaps < 5s (boss_magus_telestra.cpp:165-177);
		// the Lua twin tracks magus deaths in the boss motion's
		// SetData store (boss_magus_telestra.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataTelestraSplit) == 2

	case "achievement_three_faced": // boss_devourer_of_souls.cpp:475-493
		// OnCheck: Devourer->AI()->GetData(DATA_THREE_FACED 1) — true
		// while no Phantom Blast hit a player: the latch starts set
		// (Initialize, boss_devourer_of_souls.cpp:143-146; Reset
		// :147-149 calls only _Reset(), never re-arms — ctor-only) and
		// SpellHitTarget (:225-228) clears it on a 70322 hit; the Lua
		// twin keeps it in the boss motion's SetData store
		// (boss_devourer_of_souls.lua). Dormant until the engine ever
		// delivers a 70322 hit (the Lua cast is 68982).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataDevourerThreeFaced) != 0

	case "achievement_void_dance": // boss_zuramat.cpp:209-227
		// OnCheck: Zuramat->AI()->GetData(DATA_VOID_DANCE 2153) — true
		// while no Void Sentry died: the latch starts set
		// (Initialize/Reset, boss_zuramat.cpp:62-66) and
		// SummonedCreatureDies (:86-90) clears it; the Lua twin keeps
		// it in the boss motion's SetData store (boss_zuramat.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataZuramatVoidDance) != 0

	case "achievement_volunteer_work": // boss_jedoga_shadowseeker.cpp:481-495
		// OnCheck: Jedoga->AI()->GetData(DATA_VOLUNTEER_WORK 1) == 1 —
		// true while no Twilight Volunteer died: the latch is set in
		// the ctor only (:145) and SummonedCreatureDies (:253-260)
		// clears it; a sacrificed volunteer does not die (:428-444),
		// so sacrifice never clears it. The Lua twin writes it at
		// spawn and clears it on the ev21 leg
		// (boss_jedoga_shadowseeker.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataJedogaVolunteerWork) == 1

	case "achievement_watch_him_die": // boss_krikthir_the_gatewatcher.cpp:1036-1061
		// OnCheck: all three Watchers alive at Krik'thir kill time —
		// the loop `continue`s only while each watcher is alive and
		// returns false on the first dead/missing one, so the gate
		// passes only with zero watcher deaths. The Lua twin counts
		// watcher deaths in a Lua-side slot on Krik'thir's motion
		// (boss_krikthir_the_gatewatcher.lua); absent reads 0, the
		// all-alive default. Edge: C++ returns false when a watcher
		// is missing from the instance (:1049-1052), but the Lua
		// counter cannot see a never-spawned watcher, so the gate
		// passes in that case.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataKrikthirWatchersDead) == 0

	case "achievement_snakes_whyd_it_have_to_be_snakes": // boss_slad_ran.cpp:284-296
		// OnCheck: !WasWrapped(source GUID) over the _wrappedPlayers
		// GuidSet (boss_slad_ran.cpp:191-201), fed by the constrictor's
		// stack-5 wrap leg (:223-240). The Lua twin records each wrap
		// as a "wrapped_<guidLow>" key in the shared instanceData
		// table (boss_slad_ran.lua), cleared at engage; GetGUIDLow is
		// the low 32 bits of the player GUID on both sides
		// (lua_player.go:105). Coordinator port tick 417: the per-
		// player set is expressible via the once_bitten_twice_shy
		// instanceData pattern, not the scalar bossLatchData store.
		if source == nil || source.player == nil || source.server == nil || source.server.Features == nil || source.server.Features.Scripts == nil {
			return false
		}
		v, ok := source.server.Features.Scripts.InstanceDataNumber(604, source.player.InstanceID, "wrapped_"+strconv.FormatUint(uint64(uint32(source.player.GUID)), 10))
		wasWrapped := ok && v >= 1
		return !wasWrapped

	case "achievement_setup_boom": // boss_mimiron.cpp:2789-2798
		// OnCheck: target->GetAI()->GetData(DATA_SETUP_BOMB 1) — true
		// while no bomb-bot explosion hit a player: the latch starts
		// set (MK II ctor/Reset, boss_mimiron.cpp:692-695/:832-835)
		// and the bomb-bot SpellScript (:1717-1718) clears it; the Lua
		// twin keeps it in the MK II motion's SetData store
		// (boss_leviathan_mk_ii.lua). Dormant clear: bomb bots never
		// spawn in Go (ACU summon arm unported), so 63801 never fires.
		// The latch lives on the Leviathan MK II AI (33432);
		// boss_mimironAI has no GetData override, so the kill target
		// here is the MK II.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMimironSetupBomb) != 0

	case "achievement_setup_mine": // boss_mimiron.cpp:2800-2809
		// OnCheck: target->GetAI()->GetData(DATA_SETUP_MINE 0) — true
		// while no proximity-mine explosion hit a player: the latch
		// starts set (MK II ctor/Reset) and the mine ev15 leg
		// (npc_mimiron_proximity_mine.lua, spell 66351) clears it; the
		// Lua twin keeps it in the MK II motion's SetData store.
		// Dormant until MK II's proximity-mine summon arm lands.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMimironSetupMine) != 0

	case "achievement_setup_rocket": // boss_mimiron.cpp:2811-2820
		// OnCheck: target->GetAI()->GetData(DATA_SETUP_ROCKET 2) —
		// true while no rocket strike hit a player: the latch starts
		// set (MK II ctor/Reset) and the rocket-strike SpellScript
		// (:2303-2304) clears it; the Lua twin keeps it in the MK II
		// motion's SetData store (boss_leviathan_mk_ii.lua). Dormant
		// clear: VX-001 rocket-strike arm unported, so 63041 never fires.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMimironSetupRocket) != 0

	case "achievement_firefighter": // boss_mimiron.cpp:2822-2832
		// OnCheck: target->GetAI()->GetData(DATA_FIREFIGHTER 4) —
		// the MK II _fireFighter latch (boss_leviathan_mk_iiAI
		// GetData :766-786; boss_mimironAI has no GetData override,
		// so the kill target here is the MK II, the setup_boom/mine/
		// rocket precedent). The latch starts clear (ctor/Reset) and
		// is set only by the hard-mode button chain (go_mimiron_
		// hardmode_button -> computer -> controller DoAction,
		// boss_mimiron.cpp:1688/:1427/:1453/:415/:738, unported);
		// the Lua twin writes 0 at spawn/reset
		// (boss_leviathan_mk_ii.lua) and the set leg stays out, so
		// the gate is fail-closed until the button chain ports.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataMimironFirefighter) != 0

	case "achievement_shutout": // boss_flame_leviathan.cpp:1395-1409
		// OnCheck: leviathan->AI()->GetData(DATA_SHUTOUT) — true
		// while no shutdown happened: the latch starts set
		// (Initialize :238) and the EVENT_SHUTDOWN arm (:428-429)
		// clears it; the Lua twin keeps it in the leviathan motion's
		// SetData store (boss_flame_leviathan.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFlameLeviathanShutout) != 0

	case "achievement_orbital_bombardment": // boss_flame_leviathan.cpp:1426-1442
		// OnCheck: GetData(DATA_ORBIT_ACHIEVEMENTS) >= 1 — the active
		// tower count: 4 at hard-mode activation (lorekeeper gossip ->
		// DoAction(5)), minus one per destroyed tower (GO destroyed
		// legs); 0 while hard mode is off. Lua twin in
		// boss_flame_leviathan.lua, read off the killed leviathan.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFlameLeviathanOrbit) >= 1

	case "achievement_orbital_devastation": // boss_flame_leviathan.cpp:1444-1460
		// OnCheck: GetData(DATA_ORBIT_ACHIEVEMENTS) >= 2, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFlameLeviathanOrbit) >= 2

	case "achievement_nuked_from_orbit": // boss_flame_leviathan.cpp:1462-1478
		// OnCheck: GetData(DATA_ORBIT_ACHIEVEMENTS) >= 3, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFlameLeviathanOrbit) >= 3

	case "achievement_orbit_uary": // boss_flame_leviathan.cpp:1480-1499
		// OnCheck: GetData(DATA_ORBIT_ACHIEVEMENTS) == 4, same latch as above.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataFlameLeviathanOrbit) == 4

	case "achievement_assembly_i_choose_you": // boss_assembly_of_iron.cpp:765-773
		// OnCheck: target->GetAI()->GetData(DATA_PHASE_3 1) — 1 once the
		// killed boss reached phase 3 (engage 1 + two supercharges); the
		// Lua twin keeps the phase in the boss motion's SetData store
		// (boss_assembly_of_iron.lua: ev1/ev23/superchargeSurvivors).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataAssemblyPhase3) != 0

	case "achievement_dont_stand_in_the_lightning": // boss_thorim.cpp:2129-2138
		// OnCheck: GetData(ACHIEVEMENT_DONT_STAND_IN_THE_LIGHTNING
		// 29712972) != 0 — true from Reset/Initialize (:456-461) until
		// SPELL_LIGHTNING_RELEASE 62466 hits a player (SpellHitTarget
		// :560-562); the Lua twin keeps it in the boss motion's SetData
		// store (boss_thorim.lua: ev23 set, ev15 clear). The clear leg
		// is dormant: the 62466 cast rides the unported phase-2 / pillar
		// machine, so in the Go-modeled fight the latch stays 1,
		// matching C++ for a zero-lightning-release-hit fight.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataThorimDontStandInLightning) != 0

	case "achievement_lose_your_illusion": // boss_thorim.cpp:2140-2149
		// OnCheck: GetData(ACHIEVEMENT_LOSE_YOUR_ILLUSION 31763183)
		// != 0 — the same slot doubles as DATA_THORIM_HARDMODE
		// (:531-533); true from Reset/Initialize until
		// SPELL_TOUCH_OF_DOMINION_TRIGGERED 62565 lands with Sif in
		// the instance (SpellHit :548-556); the Lua twin keeps it in
		// the boss motion's SetData store (boss_thorim.lua: ev23 set,
		// ev14 Sif-gated clear). The clear leg is dormant: Sif is
		// unregistered and the Sif-channel machine is unported, so in
		// the Go-modeled fight the latch stays 1, matching C++ for a
		// fight where Sif never despawned.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataThorimLoseYourIllusion) != 0

	case "achievement_i_ll_take_you_all_on": // boss_thorim.cpp:2151-2162
		// OnCheck: OutOfArenaCheck(source) — !IsInBounds(ArenaBoundaries).
		// In-bounds = inside the arena circle AND outside the inverted
		// lightning-field circle; no Lua latch exists by design, this is
		// a pure position check on the earning player.
		if source == nil || source.player == nil {
			return false
		}
		dx := float64(source.player.X) - 2134.77
		dy := float64(source.player.Y) - (-262.307)
		inArena := dx*dx+dy*dy <= 45.4*45.4
		dx2 := float64(source.player.X) - 2135.178
		dy2 := float64(source.player.Y) - (-321.122)
		outsideField := dx2*dx2+dy2*dy2 > 32.0*32.0
		return !(inArena && outsideField)

	case "achievement_ignis_shattered": // boss_ignis.cpp:491-501
		// OnCheck: GetData(DATA_SHATTERED 29252926) != 0 — true once two
		// iron constructs shatter within 5s; the Lua twin keeps it in the
		// boss motion's SetData store (boss_ignis.lua: ignisDoAction +
		// ironConstructDamageTaken; cleared at engage). Dormant until the
		// BRITTLE aura machine ports — the latch defaults false.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataIgnisShattered) != 0

	case "achievement_heartbreaker": // boss_xt002.cpp:989-1001
		// OnCheck: GetData(DATA_HARD_MODE 1) != 0 — true once the heart
		// died and EVENT_ENTER_HARD_MODE fired; the Lua twin sets it in
		// the boss motion's SetData store (boss_xt002.lua: xt002HeartDied
		// ev4 -> xt002DoAction ev38; ev23 zeroes it). Dormant until the
		// heart-exposure machine ports — defaults false.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataXT002HardMode) != 0

	case "achievement_iron_dwarf_medium_rare": // boss_razorscale.cpp:1702-1711
		// OnCheck: the killed Dark Rune Guardian's GetData(
		// DATA_IRON_DWARF_MEDIUM_RARE 29232924) — set by the flame-breath
		// SpellScript on a lethal breath hit; the Lua twin sets it in the
		// guardian motion's SetData store (boss_razorscale.lua:
		// darkRuneGuardianSpellHit ev14, lethal = dead at handler time).
		// Live since tick 453 (EVENT_FLAME_BREATH / FLAME_BREATH_GROUND
		// ported in boss_razorscale.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataRazorscaleIronDwarf) != 0

	case "achievement_quick_shave": // boss_razorscale.cpp:507-512
		// OnCheck: Razorscale's GetData(DATA_QUICK_SHAVE 29192921) —
		// 1 iff _flyCount <= 1, i.e. she took off at most once: the
		// Lua twin counts FLIGHT_2 arrivals in razorscaleFlyCount
		// (boss_razorscale.lua razorscaleMovementInform) and writes
		// the latch 1 on Reset (alive) and 0 when the count passes 1.
		// No Reset clear needed: the ev23 write is alive-gated, so
		// the death-time read sees the fight's latch (the neck_deep
		// ev23-before-ev4 ordering, kill.go:1091).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataRazorscaleQuickShave) != 0

	case "achievement_bronjahm_soul_power": // boss_bronjahm.cpp:390-399
		// OnCheck: target->GetAI()->GetData(DATA_SOUL_POWER 1) >= 4 — the
		// count of live Corrupted Soul Fragments in the boss's summon
		// list (boss_bronjahm.cpp:139-154); the Lua twin keeps the count
		// in the boss motion's SetData store (boss_bronjahm.lua:
		// JustSummoned +1, SummonedCreatureDies/Despawn -1). Known
		// delta: the consume leg (fragment MovementInform :249) is
		// unported, so fragments C++ would consume keep counting
		// until killed/despawned — over-awards vs C++.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataBronjahmSoulPower) >= 4

	case "achievement_chaos_theory": // boss_anomalus.cpp:272-291
		// OnCheck: Anomalus->AI()->GetData(DATA_CHAOS_THEORY 1) — true
		// while no Chaotic Rift died: the latch starts set (ctor/Reset,
		// boss_anomalus.cpp:86-90) and the rift SummonedCreatureDies
		// relay (:130-134) clears it; the Lua twin keeps it in the boss
		// motion's SetData store (boss_anomalus.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataAnomalusChaosTheory) != 0

	case "achievement_ruby_void", "achievement_emerald_void", "achievement_amber_void": // boss_eregos.cpp:292-312
		// OnCheck: target->GetAI()->GetData(DATA_*_VOID) — true while no
		// drake vehicle of that type was present at engage: the latches
		// start set (Initialize, boss_eregos.cpp:95-99) and
		// JustEngagedWith (:124-128) clears each whose drake vehicle
		// (27756/27692/27755) is within 500y; the Lua twin keeps them
		// in the boss motion's SetData store (boss_eregos.lua). The
		// three names share one C++ class (achievement_gen_eregos_void)
		// with a per-name _data slot.
		if source == nil || victim == nil {
			return false
		}
		switch name {
		case "achievement_ruby_void":
			return bossLatchData(source.server, victim, dataEregosRubyVoid) != 0
		case "achievement_emerald_void":
			return bossLatchData(source.server, victim, dataEregosEmeraldVoid) != 0
		default:
			return bossLatchData(source.server, victim, dataEregosAmberVoid) != 0
		}

	case "achievement_girl_love_to_skadi": // boss_skadi.cpp:936-952
		// OnCheck: skadi->AI()->GetData(DATA_LOVE_TO_SKADI 0) == 1 —
		// true between the first harpoon hit (ACTION_HARPOON_HIT,
		// boss_skadi.cpp:291-295) and the next drake breath (:259-262);
		// the Lua twin keeps the love latch in the boss motion's
		// SetData store (boss_skadi.lua: Reset 0 / graufSpellHit 1).
		// Known delta: the drake-breath 1->2 leg is unported
		// (spline-chain MovementInform), so the latch never returns
		// to 2 in Lua.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSkadiLoveToSkadi) == 1

	case "achievement_incredible_hulk": // boss_svala.cpp:602-611
		// OnCheck: target->GetAI()->GetData(DATA_INCREDIBLE_HULK 2043)
		// — true when the killed Scourge Hulk took lethal damage from
		// Svala (DamageTaken, boss_svala.cpp:560-564); the Lua twin
		// writes it in hulkDamageTaken (boss_svala.lua) and zeroes it
		// at Reset. The kill target is the hulk, so victim is its
		// motion here.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSvalaIncredibleHulk) != 0

	case "achievement_kings_bane": // boss_ymiron.cpp:332-345
		// OnCheck: Ymiron->AI()->GetData(DATA_KINGS_BANE 2157) — true
		// while no player was hit by Bane: the latch starts set
		// (Initialize/Reset, boss_ymiron.cpp:122-130) and SpellHitTarget
		// (:153-157) clears it when SPELL_BANE_HIT 59203 lands on a
		// player; the Lua twin keeps it in the boss motion's SetData
		// store (boss_ymiron.lua).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataYmironKingsBane) != 0

	case "achievement_defenseless": // boss_cyanigosa.cpp:129-145
		// OnCheck: instance->GetData(DATA_DEFENSELESS 14) != 0 — the
		// Violet Hold latch (instance_violet_hold.cpp:474-475; ctor
		// default true, cleared by door damage :385 and crystal
		// activation :928-936). The Lua twin is the shared instanceData
		// defenseless key (npc_violet_hold_activation_crystal.lua
		// writes 0 on gossip); absent reads as the C++ ctor default.
		// Known deltas: the door-damage clear is unported (Destroy
		// Door Seal AuraScript) and the wipe re-arm is unported (no
		// instance Update hook) — after a wipe following crystal use
		// the key stays 0 until a fresh instance.
		if source == nil || source.player == nil || source.server == nil || source.server.Features == nil || source.server.Features.Scripts == nil {
			return false
		}
		if v, ok := source.server.Features.Scripts.InstanceDataNumber(source.player.Map, source.player.InstanceID, "defenseless"); ok {
			return v >= 1
		}
		return true

	case "achievement_bg_ic_glaive_grave": // achievement_scripts.cpp:81-96
		// The player is riding a glaive thrower (BattlegroundIC.h:41-42:
		// NPC_GLAIVE_THROWER_A 34802, NPC_GLAIVE_THROWER_H 35273).
		entry, ok := vehicleCreatureEntry(source)
		return ok && (entry == 34802 || entry == 35273)

	case "achievement_bg_ic_mowed_down": // achievement_scripts.cpp:98-113
		// The player is riding a keep cannon (BattlegroundIC.h:37:
		// NPC_KEEP_CANNON 34944).
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 34944

	case "achievement_bg_sa_artillery": // achievement_scripts.cpp:115-130
		// The player is riding an anti-personnel cannon
		// (BattlegroundSA.h:85: NPC_ANTI_PERSONNAL_CANNON 27894).
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 27894

	case "achievement_three_car_garage_demolisher": // boss_flame_leviathan.cpp:1350-1358
		// The player is riding a demolisher (VEHICLE_DEMOLISHER 33109,
		// boss_flame_leviathan.cpp:138).
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 33109

	case "achievement_three_car_garage_chopper": // boss_flame_leviathan.cpp:1367-1375
		// The player is riding a chopper (VEHICLE_CHOPPER 33062, :137).
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 33062

	case "achievement_three_car_garage_siege": // boss_flame_leviathan.cpp:1384-1392
		// The player is riding a siege engine (VEHICLE_SIEGE 33060, :136).
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 33060

	case "achievement_denyin_the_scion": // boss_malygos.cpp:2429-2436
		// The player is riding a melee hover disk (eye_of_eternity.h:53:
		// NPC_HOVER_DISK_MELEE 30234). C++ reads GetVehicleBase(), not
		// GetVehicleCreatureBase(); a player base reads entry 0 there,
		// so the IsPlayer exclusion in vehicleCreatureEntry is the twin.
		entry, ok := vehicleCreatureEntry(source)
		return ok && entry == 30234

	case "achievement_wg_didnt_stand_a_chance": // zone_wintergrasp.cpp:467-490
		// OnCheck: target->ToPlayer() must be mounted, and
		// source->GetVehicle()->GetVehicleInfo()->ID == 244 (Wintergrasp
		// Tower Cannon). The Go vehicle kit carries the DBC vehicle ID as
		// VehicleID (vehicle.go:68), stamped from creature_template.VehicleId
		// at spawn (creaturemotion.go:1372-1374); vehicleID reads it off the
		// same kit vehicleCreatureEntry uses. isPlayerMounted is nil-safe
		// and false for non-player targets (commands_misc.go:1685).
		if source == nil || target == nil {
			return false
		}
		if !target.isPlayerMounted() {
			return false
		}
		id, ok := vehicleID(source)
		return ok && id == 244

	case "achievement_the_hundred_club": // boss_sapphiron.cpp:637-646
		// OnCheck: GetData(DATA_THE_HUNDRED_CLUB 21462147) != 0 —
		// true from Initialize/Reset (:131-134/:154-168) until
		// SPELL_CHECK_RESISTS 60539 hits a unit with frost resistance
		// > 100 (SpellHitTarget :193-203); the Lua twin keeps it in
		// the boss motion's SetData store (boss_sapphiron.lua: ev23
		// alive-gated set, ev5 set, ev15 clear via the new
		// player/creature GetResistance bridges).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataSapphironHundredClub) != 0

	case "achievement_intense_cold": // boss_keristrasza.cpp:290-308
		// OnCheck: the player's GUID must NOT be in the boss's
		// _intenseColdList — pushed by the spell_intense_cold
		// AuraScript periodic tick (EFFECT_1, :263-278) when the
		// 48094 aura's stack reaches 2 (the C++ @todo notes the
		// caster should be the boss). The Lua twin keeps the set
		// as per-player "intenseCold_<guidLow>" keys in the shared
		// instanceData table (the blood-queen vampire_ precedent),
		// cleared at engage; a missing key reads 0, the C++
		// empty-list default. Dormant until the DBC aura triggers
		// run (no Spell.dbc in tree): the 48094 periodic ticks
		// never fire, so the set stays empty and the check passes
		// — the sindragosa/garfrost peak-stack dormancy class.
		if source == nil || victim == nil || source.player == nil || source.server == nil || source.server.Features == nil || source.server.Features.Scripts == nil {
			return false
		}
		v, ok := source.server.Features.Scripts.InstanceDataNumber(source.player.Map, source.player.InstanceID, "intenseCold_"+strconv.FormatUint(uint64(uint32(source.player.GUID)), 10))
		return !(ok && v != 0)

	case "achievement_portal_jockey": // boss_valithria_dreamwalker.cpp:1231-1239
		// OnCheck: target && !target->GetAI()->GetData(MISSED_PORTALS) —
		// the portal's GetData returns 0 only once used (npc_dream_portal
		// _used, :913-930); the Lua twin keeps it in the portal motion's
		// SetData store (boss_valithria_dreamwalker.lua: ev22 writes 1 at
		// summon, ev39 writes 0 on a handled click). The update only fires
		// from handleSpellClick on a real clicked portal motion, whose ev22
		// init ran — so an absent slot (never summoned) never reaches this
		// read. Dormant until the pre-effect summon leg lands: the boss
		// EVENT_DREAM_PORTAL pre-summon targeting is DBC-side, so no portal
		// exists for a player to click yet.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataDreamPortalMissed) == 0

	case "achievement_shatter_resistant": // boss_volkhan.cpp:490-499
		// OnCheck: GetData(DATA_SHATTER_RESISTANT 2042) < 5 — the
		// GolemsShattered counter (GetData :221-225, init 0 :108),
		// incremented only in ShatterGolem() (:188) behind
		// m_bCanShatterGolem, set only by EVENT_SHATTERING_STOMP
		// (:256-266) gated on !m_bHasTemper && modifier >= 3. The
		// gate is unreachable in this snapshot: temper is set at the
		// first 80% threshold (:302) before the modifier can reach 3
		// (60% threshold), and EVENT_PAUSE — the only temper clear
		// (:243-253) — fires once at 3.5s and dies (its reschedule
		// rides the striking branch, and striking starts only after
		// temper). The counter is provably always 0, so the bare read
		// ports the snapshot's behavior exactly, bug included. If
		// the temper/stomp machine is ever ported/fixed, this case
		// needs a Lua write leg or it will over-award.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataVolkhanShattered) < 5

	case "achievement_nerf_engineering": // boss_xt002.cpp:980-986
		// OnCheck: !GetData(DATA_HEALTH_RECOVERED 2) — _healthRecovered
		// (init false :149) is set only in PassengerBoarded (:268-278)
		// when a scrapbot (NPC_XS013_SCRAPBOT 33343) boards. No
		// PassengerBoarded bridge exists and scrapbots never spawn in
		// Go (the energy-orb / toy-pile / recharge summon chain is
		// all SpellScript legs), so the latch is never written and
		// the bare read ports the Go-modeled fight exactly. If the
		// scrapbot machine is ever ported, the ride leg must write
		// this slot.
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataXT002HealthRecovered) == 0

	case "achievement_unbroken": // boss_flame_leviathan.cpp:1416-1423
		// OnCheck: instance->GetData(DATA_UNBROKEN 29052906) != 0 —
		// the instance latch (instance_ulduar.cpp:183 ctor true,
		// :761-763 SetData, :852-853 GetData). The Lua twin keeps it
		// in the shared instanceData table (boss_flame_leviathan.lua:
		// ev5 set). The only clear is spell_auto_repair (:1630-1645,
		// no SpellScript bridge), so the latch stays 1 in the
		// Go-modeled fight. Absent key reads false (fail-closed).
		if source == nil || source.player == nil || source.server == nil || source.server.Features == nil || source.server.Features.Scripts == nil {
			return false
		}
		if v, ok := source.server.Features.Scripts.InstanceDataNumber(source.player.Map, source.player.InstanceID, "unbroken"); ok {
			return v != 0
		}
		return false

	case "achievement_dehydration": // boss_ichoron.cpp:475-484
		// OnCheck: GetData(DATA_DEHYDRATION 1) != 0 — _dehydration
		// (init true :92, Reset :93-97) cleared only by DoAction
		// ACTION_WATER_GLOBULE_HIT (:119-125) from spell_ichoron_merge
		// (:366-371) behind the unported globule chain; the Lua twin
		// keeps it in the boss motion's SetData store (boss_ichoron.lua:
		// ev1 set).
		if source == nil || victim == nil {
			return false
		}
		return bossLatchData(source.server, victim, dataIchoronDehydration) != 0
	}
	return false
}

// checkInstanceAchievementCriteria evaluates one DATA_TYPE_INSTANCE_SCRIPT
// (18) criteria gate, the Go form of InstanceScript::
// CheckAchievementCriteriaMeet (AchievementMgr.cpp:404-424): the map's
// instance script evaluates the criteria against its own state, and
// C++ returns false for criteria its switch does not recognize. Maps
// with no Go instance gate fail closed, matching the unknown-script
// default for the type-11 gate above — never auto-passed.
func (s *session) checkInstanceAchievementCriteria(criteriaID uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return false
	}
	switch s.player.Map {
	case 624: // Vault of Archavon
		return checkVaultOfArchavonCriteria(s, criteriaID)
	case 649: // Trial of the Crusader
		return checkTrialOfTheCrusaderCriteria(s, criteriaID)
	case 249: // Onyxia's Lair
		return checkOnyxiasLairCriteria(s, criteriaID)
	case 533: // Naxxramas
		return checkNaxxramasCriteria(s, criteriaID)
	case 631: // Icecrown Citadel
		return checkIcecrownCitadelCriteria(s, criteriaID)
	case 603: // Ulduar
		return checkUlduarCriteria(s, criteriaID)
	default:
		return false
	}
}

// checkTrialOfTheCrusaderCriteria mirrors
// instance_trial_of_the_crusader_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_trial_of_the_crusader.cpp:621-654).
// The counters live in the Lua shared instanceData table (boss_northrend_
// beasts.lua snoboldCount, boss_lord_jaraxxus.lua mistressOfPainCount,
// the shared trialCounter); absent fields read as the C++ member
// defaults (0, and 50 for TrialCounter, instance :110/:118/:120).
// Criteria IDs are the TCRAchievementData enum
// (trial_of_the_crusader.h:247-269); the 10/25-player split is encoded
// in the criteria ID itself, so no raid-size check is needed here.
func checkTrialOfTheCrusaderCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	switch criteriaID {
	case 11779, 11802: // UPPER_BACK_PAIN 10-player (+heroic): SnoboldCount >= 2
		return num("snoboldCount", 0) >= 2
	case 11780, 11801: // UPPER_BACK_PAIN 25-player (+heroic): SnoboldCount >= 4
		return num("snoboldCount", 0) >= 4
	case 11838, 11861, 11839, 11862: // THREE_SIXTY_PAIN_SPIKE: MistressOfPainCount >= 2
		return num("mistressOfPainCount", 0) >= 2
	case 12344, 12338: // A_TRIBUTE_TO_SKILL: TrialCounter >= 25
		return num("trialCounter", 50) >= 25
	case 12347, 12341: // A_TRIBUTE_TO_MAD_SKILL: TrialCounter >= 45
		return num("trialCounter", 50) >= 45
	case 12349, 12343, 12350: // A_TRIBUTE_TO_INSANITY / REALM_FIRST_GRAND_CRUSADER: TrialCounter == 50
		return num("trialCounter", 50) == 50
	case 12358, 12359: // A_TRIBUTE_TO_IMMORTALITY: TrialCounter == 50 && TributeToImmortalityEligible
		// The eligibility latch (OnUnitDeath clears it when a player
		// dies while an encounter is in progress, instance :165-169)
		// is written by instance_trial_of_the_crusader.lua into the
		// shared instanceData table (tributeToImmortalityEligible,
		// 1 until the first in-progress player death) and read here
		// via num().
		return num("trialCounter", 50) == 50 && num("tributeToImmortalityEligible", 1) != 0
	case 12360: // A_TRIBUTE_TO_DEDICATED_INSANITY: C++ returns false (:652)
		return false
	default:
		return false
	}
}

// checkVaultOfArchavonCriteria mirrors
// instance_vault_of_archavon_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_vault_of_archavon.cpp:86-105).
// The death timestamps live in the Lua shared instanceData table
// (boss_archavon.lua / boss_emalon.lua / boss_koralon.lua write
// os.time() seconds into archavonDeath / emalonDeath / koralonDeath
// at DONE time, the GameTime::GetGameTime twin); absent fields read
// as the C++ member default 0. Criteria IDs are the
// VAAchievementCriteriaIds enum (vault_of_archavon.h:44-48); the
// 10/25-player split is encoded in the criteria ID itself, so no
// raid-size check is needed here.
func checkVaultOfArchavonCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	switch criteriaID {
	case 12018, 12019: // CRITERIA_EARTH_WIND_FIRE_10 / _25
		archavon := num("archavonDeath", 0)
		emalon := num("emalonDeath", 0)
		koralon := num("koralonDeath", 0)
		if archavon == 0 || emalon == 0 || koralon == 0 {
			return false
		}
		// C++ :97-101: all three pairwise |diffs| < MINUTE (60s).
		return math.Abs(archavon-emalon) < 60 &&
			math.Abs(emalon-koralon) < 60 &&
			math.Abs(koralon-archavon) < 60
	default:
		return false
	}
}

// checkOnyxiasLairCriteria mirrors
// instance_onyxias_lair_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_onyxias_lair.cpp:244-257).
// Both latches live in the Lua shared instanceData table as 1/0
// numbers (InstanceDataNumber reads numbers only): manyWhelpsHandleIt
// (set by the instance Update liftoff pump, instance_onyxias_lair.lua)
// and sheDeepBreathMore (cleared by the boss ev15 SpellHitTarget leg,
// boss_onyxia.lua). Absent fields read as the C++ member defaults
// (false for the whelps latch, true for the deep-breath latch,
// instance :66-70). Criteria IDs are the OLAchievementData enum
// (onyxias_lair.h:71-76); the 10/25-player split is encoded in the
// criteria ID itself.
func checkOnyxiasLairCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	switch criteriaID {
	case 12565, 12568: // ACHIEV_CRITERIA_MANY_WHELPS_10/25_PLAYER
		return num("manyWhelpsHandleIt", 0) >= 1
	case 12566, 12569: // ACHIEV_CRITERIA_DEEP_BREATH_10/25_PLAYER
		return num("sheDeepBreathMore", 1) >= 1
	default:
		return false
	}
}

// checkNaxxramasCriteria mirrors
// instance_naxxramas_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_naxxramas.cpp:522-553).
// The death timestamps live in the Lua shared instanceData table
// (boss_four_horsemen.lua writes GetGameTimeMS() into
// <which>DeathMS at JustDied time, the GameTime::GetGameTimeMS
// twin, cleared by Reset); absent fields read as the C++ member
// default 0, which fails the credit check exactly as a horseman
// reporting not dead does (boss_four_horsemen.cpp:146-152).
// EncounterCount is 15 (naxxramas.h:27); the boss states come from
// the Go instanceBossState reader, which reports TO_BE_DECIDED
// for never-touched encounters, so a fresh instance counts all 15
// alive. Criteria IDs are the raw numbers from the C++ switch —
// the C++ comments (:524-525) label 7600 25-man / 7601 10-man,
// but the arithmetic says otherwise, and Naxxramas has no heroic
// modes (spawn mode is 0 for 10-man, 1 for 25-man): 7600 passes
// only at mode 0 and 7601 only at mode 1, so 7600 is the 10-man
// criteria and 7601 the 25-man one; the arithmetic is ported
// verbatim, labels notwithstanding.
func checkNaxxramasCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	switch criteriaID {
	case 7600, 7601: // And They Would All Go Down Together
		// C++ :528-529: criteria_id + GetSpawnMode() == 7601
		// rejects the wrong raid size.
		if criteriaID+uint32(s.playerSpawnMode()) == 7601 {
			return false
		}
		// C++ :530-533: the baron's GetData(
		// DATA_HORSEMEN_CHECK_ACHIEVEMENT_CREDIT) — the timed-kill
		// read (boss_four_horsemen.cpp:140-164) over all four
		// death times: every horseman dead, max-min spread <=
		// 15 * IN_MILLISECONDS.
		baron := num("baronDeathMS", 0)
		thane := num("thaneDeathMS", 0)
		lady := num("ladyDeathMS", 0)
		sir := num("sirDeathMS", 0)
		if baron == 0 || thane == 0 || lady == 0 || sir == 0 {
			return false
		}
		minTime := math.Min(math.Min(baron, thane), math.Min(lady, sir))
		maxTime := math.Max(math.Max(baron, thane), math.Max(lady, sir))
		return maxTime-minTime <= 15000
	case 13233, 13234, 13235, 13236, 7616, // The Immortal (25-man)
		13237, 13238, 13239, 13240, 7617: // The Undying (10-man)
		// C++ :545-548: AreAllEncountersDone() && !playerDied.
		// AreAllEncountersDone (:511-520) counts encounters not
		// DONE and tolerates exactly one — the boss whose death
		// is being checked, since this runs before its
		// SetBossState(DONE). The playerDied latch (OnUnitDeath
		// sets it when a player dies while an encounter is in
		// progress, instance :248-254) is written by
		// instance_naxxramas.lua into the shared instanceData
		// table (playerDied, 1 after the first in-progress
		// player death) and read here via num().
		alive := 0
		for bossID := uint32(0); bossID < 15; bossID++ {
			if s.server.instanceBossState(mapID, instanceID, bossID) != encounterStateDone {
				alive++
			}
		}
		if alive > 1 {
			return false
		}
		return num("playerDied", 0) == 0
	default:
		return false
	}
}

// ulduarConquerorBoss maps a Conqueror-of-Ulduar criteria id to its boss
// index (ulduar.h:32-49, BOSS_LEVIATHAN = 0 .. BOSS_YOGG_SARON = 12;
// criteria ids ulduar.h:372-398, 10-man then 25-man, same boss order).
var ulduarConquerorBoss = map[uint32]uint32{
	10042: 0, 10342: 1, 10340: 2, 10341: 3, 10598: 4, 10348: 5, 10351: 6, 10439: 7, 10403: 8, 10582: 9, 10347: 10, 10349: 11, 10350: 12,
	10352: 0, 10355: 1, 10353: 2, 10354: 3, 10599: 4, 10357: 5, 10363: 6, 10719: 7, 10404: 8, 10583: 9, 10361: 10, 10362: 11, 10364: 12,
}

// checkUlduarCriteria mirrors
// instance_ulduar_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_ulduar.cpp:865-936).
// The state lives in the Lua shared instanceData table:
// illusion is written by the voice's EVENT_ILLUSION arm
// (boss_yogg_saron.lua illusionTick, urand 0-2), keepersCount is
// incremented at the observation-ring keeper gossip summon
// (boss_yogg_saron.lua), driveMeCrazyEligible is set by Yogg's
// Reset (true) and the sanity OnRemove (false, boss_yogg_saron.lua),
// maxArmorItemLevel / maxWeaponItemLevel are banked by the Algalon
// item-level scan (boss_algalon_the_observer.lua). Absent fields
// read as the C++ member defaults (illusion 0, keepersCount 0,
// driveMeCrazyEligible true, item levels 0, instance :173-187).
// Criteria IDs are the UlduarAchievementCriteriaIds enum
// (ulduar.h:347-398); the 10/25-player split is encoded in the
// criteria ID itself, so no raid-size check is needed here.
func checkUlduarCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	switch criteriaID {
	case 10678: // HERALD_OF_TITANS
		// C++ :869-870: _maxArmorItemLevel <= 226 &&
		// _maxWeaponItemLevel <= 232 (ulduar.h:487-488).
		return num("maxArmorItemLevel", 0) <= 226 && num("maxWeaponItemLevel", 0) <= 232
	case 10321, 10324: // WAITS_DREAMING_STORMWIND 25/10
		// C++ :871-873: illusion == STORMWIND_ILLUSION (2).
		return num("illusion", 0) == 2
	case 10322, 10325: // WAITS_DREAMING_CHAMBER 25/10
		// C++ :874-876: illusion == CHAMBER_ILLUSION (0).
		return num("illusion", 0) == 0
	case 10323, 10326: // WAITS_DREAMING_ICECROWN 25/10
		// C++ :877-879: illusion == ICECROWN_ILLUSION (1).
		return num("illusion", 0) == 1
	case 10185, 10296: // DRIVE_ME_CRAZY 10/25
		// C++ :880-882: IsDriveMeCrazyEligible.
		return num("driveMeCrazyEligible", 1) != 0
	case 10410, 10414: // THREE_LIGHTS_IN_THE_DARKNESS 10/25
		// C++ :883-885: keepersCount <= 3.
		return num("keepersCount", 0) <= 3
	case 10388, 10415: // TWO_LIGHTS_IN_THE_DARKNESS 10/25
		// C++ :886-888: keepersCount <= 2.
		return num("keepersCount", 0) <= 2
	case 10409, 10416: // ONE_LIGHT_IN_THE_DARKNESS 10/25
		// C++ :889-891: keepersCount <= 1.
		return num("keepersCount", 0) <= 1
	case 10412, 10417, 10279: // ALONE_IN_THE_DARKNESS 10/25, REALM_FIRST_DEATHS_DEMISE
		// C++ :892-895: keepersCount == 0.
		return num("keepersCount", 0) == 0
	case 10042, 10342, 10340, 10341, 10598, 10348, 10351, 10439, 10403, 10582, 10347, 10349, 10350, // Conqueror of Ulduar 10-man
		10352, 10355, 10353, 10354, 10599, 10357, 10363, 10719, 10404, 10583, 10361, 10362, 10364: // Conqueror of Ulduar 25-man
		// C++ :896-933: (_CoUAchivePlayerDeathMask & (1 << boss))
		// == 0 — the mask is set in OnUnitDeath when a player
		// dies while a boss is IN_PROGRESS (instance :528-540),
		// written by instance_ulduar.lua into the shared
		// instanceData table (coUAchivePlayerDeathMask) and read
		// here via num(). The Naxxramas playerDied / ToC
		// TRIBUTE_TO_IMMORTALITY latches ride the same sender
		// since tick 468 (instance_naxxramas.lua /
		// instance_trial_of_the_crusader.lua OnUnitDeath
		// handlers, read by their own check functions).
		if boss, ok := ulduarConquerorBoss[criteriaID]; ok {
			return int64(num("coUAchivePlayerDeathMask", 0))&(int64(1)<<boss) == 0
		}
		return false
	default:
		return false
	}
}

// checkIcecrownCitadelCriteria mirrors
// instance_icecrown_citadel_InstanceMapScript::
// CheckAchievementCriteriaMeet (instance_icecrown_citadel.cpp:
// 1140-1178). Criteria IDs are the ICCAchievementData enum
// (icecrown_citadel.h:448-477). The four eligibility latches live
// in the C++ instance script (ctor :156-159 sets all true); only
// Boned has a bridged write path — the other three clear legs are
// a SpellScript (Nausea, Ooze Dance) or a DamageDealt hook (Orb
// Whisperer), neither of which Go exposes, so they stay
// fail-closed. The max-players legs read
// instance->ToInstanceMap()->GetMaxPlayers(); the spawn mode is
// the twin (0/2 = 10-man, 1/3 = 25-man).
func checkIcecrownCitadelCriteria(s *session, criteriaID uint32) bool {
	rt := s.server.Features.Scripts
	mapID, instanceID := s.player.Map, s.player.InstanceID
	num := func(key string, def float64) float64 {
		if v, ok := rt.InstanceDataNumber(mapID, instanceID, key); ok {
			return v
		}
		return def
	}
	is10Man := s.playerSpawnMode()%2 == 0
	switch criteriaID {
	case 12775, 12962, 13393, 13394: // CRITERIA_BONED_*
		// C++ :1144-1148 returns IsBonedEligible: set true by
		// marrowgar Reset (boss_lord_marrowgar.cpp:177), cleared
		// by the bone-spike 8s EVENT_FAIL_BONED timer
		// (:470-475). Both legs port in Lua via the shared
		// instanceData bonedEligible key; absent reads as the
		// C++ ctor default true.
		return num("bonedEligible", 1) >= 1
	case 12984, 12966, 12985, 12983: // CRITERIA_DANCES_WITH_OOZES_*
		// C++ :1149-1153 returns IsOozeDanceEligible: true at the
		// instance ctor (instance_icecrown_citadel.cpp:157),
		// re-armed by rotface JustReachedHome
		// (boss_rotface.cpp:155), cleared by the large/buff
		// combine 5-stack legs (boss_rotface.cpp:555/615) —
		// ported in Lua via the shared instanceData
		// oozeDanceEligible key (1/0 numbers); absent reads as
		// the C++ ctor default true. Dormant until Spell.dbc
		// ships (the combine casts are DBC-gated).
		return num("oozeDanceEligible", 1) >= 1
	case 12987, 12968, 12988, 12981: // CRITERIA_NAUSEA_*
		// C++ :1154-1158 returns IsNauseaEligible: set true by
		// putricide Reset (:236), cleared by
		// spell_putricide_regurgitated_ooze (boss_professor_
		// putricide.cpp:1382) — no SpellScript bridge,
		// fail-closed.
		return false
	case 13033, 12969, 13034, 13032: // CRITERIA_ORB_WHISPERER_*
		// C++ :1159-1163 returns IsOrbWhispererEligible: set
		// true by blood-prince Reset (:452), cleared when a
		// prince deals >23000/25000 direct spell damage
		// (boss_blood_prince_council.cpp:549/965) — no
		// DamageDealt hook in Go, fail-closed.
		return false
	case 13340, 12780, 13011: // CRITERIA_KILL_LANA_THEL_10M / ONCE_BITTEN_10N/10V
		return is10Man
	case 13360, 13012, 13013: // CRITERIA_KILL_LANA_THEL_25M / ONCE_BITTEN_25N/25V
		return !is10Man
	default:
		return false
	}
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

-- Blessed Banner (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_blessed_bannerAI — Reset() + EVENT_SPAWN + JustDied-drain arms). Zone-script unit
-- per northrend_script_loader.cpp order (howling_fjord done;
-- icecrown: npc_argent_valiant, npc_guardian_pavilion,
-- npc_tournament_training_dummy, npc_frostbrood_skytalon
-- documented-only, npc_blessed_banner ported).
-- Entry (BlessedBanner enum, verifiable from the C++ sources):
-- 30891 (NPC_BLESSED_BANNER). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arm (C++ Reset()): DoCast(SPELL_THREAT_PULSE 58113) —
-- the non-targeted self-cast — creature:CastSpell(creature,
-- spell) is the vaelastrasz self-cast convention — + Talk(
-- BANNER_SAY 0). The C++ Talk has no explicit target; the Go
-- Talk bridge takes a textId only (textId-only broadcast,
-- C++-exact for the line played — lecraft aggro-Talk
-- precedent). The banner never engages combat, so OnReset(23)
-- re-arms the same Reset fragment (C++ Reset semantics —
-- mushroom precedent); OnDied(4) carries the JustDied legs in C++ order
-- (zone_icecrown.cpp:455-459): Summons.DespawnAll() via the EVENT_SPAWN
-- latch, then the self-despawn.
-- Modeled in onSpawnOrReset: SetRegenerateHealth(false) (bridged —
-- lua_creature_events.go, honored by the Go regen tick). The ctor
-- SetCombatMovement(false) (zone_icecrown.cpp:421) is ported at
-- JustEngagedWith — the bridge's ctor-equivalent point (eredar_twins
-- convention).
-- Ported: EVENT_SPAWN 3s (DoSummon 31003 + 3x 30919 + 3x 30900,
-- CORPSE_TIMED_DESPAWN 30s = Lua type 5 + 30000ms, slad'ran precedent;
-- MovePoint walk-ins via the MoveTo twin; summons latched for the
-- JustDied drain) + the full event machine: EVENT_INTRO_1 15s
-- (Dalfors Talk 0) -> INTRO_2 5s (facing 6.215 + Talk 1) ->
-- INTRO_3 5s (Dalfors MovePoint + home to fight pos, priests
-- facing 5.7421 + home to intro pos, masons MovePoint + home to
-- action pos) -> EVENT_START_FIGHT 5s (LK Talk 0 + Dalfors
-- Talk 2) -> EVENT_WAVE_SPAWN 1s, 10-20s waves (LK Talk 1 at
-- wave 4, Talk 2 at wave 7; drudge + 2x plagebringer/captain
-- per wave, home to fight pos + AttackStart on the banner) ->
-- EVENT_HALOF 10-20s (LK Talk 3 + 2x drudge + Halof 30989) ->
-- victory on Halof death via event 21 (triggered 58084 self-cast
-- + DespawnEntry waves + Dalfors Talk 3 + EVENT_ENDED 10s
-- DespawnAll + self-despawn). MASON_ACTION facings ported;
-- its SetData(1,1) SAI triggers stay out (SetData is a
-- store-only transport, no SAI dispatch reads it);
-- AttackStart calls are present but no-op for the creature
-- victim today (EngageWithTarget resolves player sessions only);
-- victory fires once per banner (C++ would re-fire per UpdateAI
-- tick until ENDED — Talk spam avoided deliberately);
-- Stay-out: EVENT_MASON_ACTION SetData(1,1) SAI torch actions
-- (SetData stores type 1 = 1 on the mason, but no SAI dispatch
-- reads it in the model); wave/Halof AttackStart on the banner
-- is engine-no-op today (player-session victim resolution only);
-- the chase leg is approximated by MoveTo to the fight pos, so
-- waves converge on the fight and stand there until engaged.
-- 58084 + 58113 + 58121 + 30891 + 31003/30919/30900/30986/
-- 30984/30987/30989/31013 verifiable.
-- npc_argent_valiant — no NPC_ entry constant anywhere in the
-- C++ sources (the creature_template ScriptName binding is
-- DB-side, no TDB in this workspace — minigob precedent): no
-- Lua file written. The combat rotation (UpdateVictim -> charge
-- 63010 7s / shield-breaker 65147 10s via the omen 1s-pump
-- pattern) is stranded without an entry; the DamageTaken
-- duel-end machine (killing blow by a player -> damage = 0 +
-- pDoneBy->CastSpell(pDoneBy, 63049 kill credit, true) +
-- SetFaction(FACTION_FRIENDLY) + DespawnOrUnsummon(5s) +
-- SetHomePosition + EnterEvadeMode) now has its DamageTaken hook (ev9 rewrite), SetFaction,
-- Despawn, SetHomePosition bridges; the remaining blockers are
-- the missing entry, player self-cast 63049, and EnterEvadeMode; the ctor
-- MovePoint + SetFaction(FACTION_FRIENDLY) arm and the
-- MovementInform POINT_MOTION_TYPE -> SetFaction(FACTION_
-- MONSTER) flip sit behind the movement bridge (SetFaction IS
-- bridged — lua_creature_events.go:1125 — but the MovePoint /
-- MovementInform machine that drives the flip is not) —
-- documented-only. 63010/65147/63049 verifiable.
-- npc_guardian_pavilion — the trespasser machine is entirely
-- MoveInLineOfSight-driven (no proximity hook — Smeed /
-- mageguard_dalaran precedent): GetAreaId() != 4676 (Sunreaver)
-- && != 4677 (Silver Covenant) gate (area-ID bridge is live —
-- lua_creature_events.go — but the proximity / facing / team legs
-- below still block the machine), IsHostileTo + isInBackInMap(who, 5.0f)
-- gates (no proximity / facing bridges), HasAura(63987/63986)
-- trespasser gates (player:HasAura is bridged — lua_player.go:203;
-- still blocked by the proximity / area-ID / facing /
-- player-self-cast bridges),
-- GetTeamId() == TEAM_ALLIANCE branch (player:GetTeam IS bridged -
-- lua_player.go:87, teamForRace 0 = Alliance per achievement code;
-- the stale "no team bridge" note is retracted, the branch gate is
-- portable but the machine still stays out below),
-- who->CastSpell(who, 63987/63986, true)
-- self-cast on the player (no player-self-cast path from a
-- creature AI — corastrasza precedent) — documented-only. No
-- NPC_ entry constant in the C++ sources; 63987/63986/4676/
-- 4677 verifiable.
-- npc_tournament_training_dummy — ported in
-- northrend/npc_tournament_training_dummy.lua (all three
-- entries 33272/33229/33243): SpellHit credit machines,
-- DamageTaken damage=0 (event-9 rewrite — the old "no DamageTaken
-- hook" claim is retracted, lich_king_escape precedent), the
-- 33272 recast-defend leg, and the per-GUID vulnerable latch. The
-- 02:3x tick's duplicate block lived in this file and is removed
-- (it double-registered event 14 on 33272); this file keeps the
-- banner arms only.
-- npc_frostbrood_skytalon (VehicleAI) — the UpdateAI low-hp /
-- periodic arms run through VehicleAI::UpdateAI (the base is
-- unmodeled — wyrmrest_defender precedent); SpellHit 59335 ->
-- self-cast 54690 / 59319 -> DoCastAOE(59375) + EVENT_FLY_AWAY
-- 100ms (ev14 IS bridged — nagrand_banner precedent; the arms stay
-- out behind the unmodeled VehicleAI base and movement / summon
-- bridges); IsSummonedBy
-- MovePoint(POINT_GRAB_DECOY) + MovementInform -> DoCast(
-- summoner, 59318) behind the movement / summon bridges —
-- documented-only. No NPC_ entry constant in the C++ sources;
-- 59318/59375/54690/59335/59319 verifiable.

local SPELL_THREAT_PULSE = 58113

local BANNER_SAY = 0

local BLESSED_BANNER_ENTRY = 30891

local NPC_CRUSADER_LORD_DALFORS = 31003
local NPC_ARGENT_BATTLE_PRIEST = 30919
local NPC_ARGENT_MASON = 30900
local NPC_SCOURGE_DRUDGE = 30984
local NPC_HIDEOUS_PLAGEBRINGER = 30987
local NPC_REANIMATED_CAPTAIN = 30986
local NPC_HALOF_THE_DEATHBRINGER = 30989
local NPC_LK = 31013

local SPELL_CRUSADERS_SPIRE_VICTORY = 58084

local DALFORS_SAY_PRE_1 = 0
local DALFORS_SAY_PRE_2 = 1
local DALFORS_SAY_START = 2
local DALFORS_YELL_FINISHED = 3
local LK_TALK_1 = 0
local LK_TALK_2 = 1
local LK_TALK_3 = 2
local LK_TALK_4 = 3

-- C++ position arrays (zone_icecrown.cpp:363-404): DalforsPos[2] fight
-- pos; MasonNPos[0] wave-spawn pos; MasonNPos[2] action pos; DalforsPos[0]
-- Halof spawn pos; PriestNPos[1] intro pos (facing 5.7421 for all three).
local DALFORS_FIGHT_POS = {6426.343, 420.515, 508.650, 0.0}
local DALFORS_SPAWN_POS = {6458.703, 403.858, 490.498, 3.1205}
local MASON_SPAWN_POS = {
    {6462.929, 409.826, 489.392, 3.0968},
    {6462.650, 405.670, 489.576, 2.9414},
    {6462.646, 401.218, 489.601, 2.7864},
}
local MASON_ACTION_POS = {
    {6414.335, 454.904, 511.395, 2.8972},
    {6415.014, 446.849, 511.395, 3.1241},
    {6417.070, 438.824, 511.395, 3.6651},
}
local PRIEST_INTRO_POS = {
    {6421.480, 423.576, 510.781, 5.7421},
    {6419.778, 421.404, 510.972, 5.7421},
    {6423.516, 425.782, 510.774, 5.7421},
}

-- C++ EVENT_SPAWN crew (zone_icecrown.cpp:465-514): {entry, spawnX,
-- spawnY, spawnZ, spawnO, walkX, walkY, walkZ} — [0] spawn + [1] walk-in
-- positions; [2] fight/action positions belong to the unported INTRO_3
-- leg and stay out.
local spawnTable = {
    {NPC_CRUSADER_LORD_DALFORS, 6458.703, 403.858, 490.498, 3.1205, 6422.950, 423.335, 510.451},
    {NPC_ARGENT_BATTLE_PRIEST, 6462.025, 403.681, 489.721, 3.1007, 6421.480, 423.576, 510.781},
    {NPC_ARGENT_BATTLE_PRIEST, 6463.969, 407.198, 489.240, 2.2689, 6419.778, 421.404, 510.972},
    {NPC_ARGENT_BATTLE_PRIEST, 6464.371, 400.944, 489.186, 6.1610, 6423.516, 425.782, 510.774},
    {NPC_ARGENT_MASON, 6462.929, 409.826, 489.392, 3.0968, 6428.163, 421.960, 508.297},
    {NPC_ARGENT_MASON, 6462.650, 405.670, 489.576, 2.9414, 6426.250, 419.194, 508.219},
    {NPC_ARGENT_MASON, 6462.646, 401.218, 489.601, 2.7864, 6423.855, 416.598, 508.305},
}

local timers = {}
local bannerSummons = {} -- per-guid latched summons (C++ JustSummoned :450-452 latch; drained by JustDied :457)
local bannerState = {} -- per-guid machine state: {crew, phaseCount, victoryFired}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function despawnBannerSummons(guid)
    local list = bannerSummons[guid]
    if list then
        for _, summon in ipairs(list) do
            if summon then
                summon:Despawn()
            end
        end
        bannerSummons[guid] = nil
    end
end

-- Forward declarations for the cyclic intro/wave chain (tharon_ja idiom:
-- plain assignments below, never re-local, or pumps call nil).
local intro1, intro2, intro3, startFight, waveSpawn, halofEvent, masonAction, endedEvent

-- Wave/Halof summon leg (zone_icecrown.cpp:600-660, 665-688):
-- DoSummon default (type 5 + 30000ms, crew precedent) + SetHomePosition
-- to the Dalfors fight pos + AttackStart on the banner. AttackStart is
-- the exact twin call, but the Go funnel resolves player sessions only
-- (lua_creature_events.go EngageWithTarget), so a creature victim no-ops
-- today; the chase leg is approximated by MoveTo to the fight pos (the
-- C++ home, banner's neighborhood — crew walk-in precedent), so waves
-- converge on the fight and stand there until players engage them.
-- Full creature-victim combat stays engine-blocked (bridge gate +
-- funnel map/instance + combat-tick player-only target resolution;
-- tick-147 triage). Latched into bannerSummons for the JustDied
-- drain and the victory DespawnEntry sweep.
local function summonWaveCreature(creature, guid, entry, pos)
    local summon = creature:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4], 5, 30000)
    if summon then
        summon:SetHomePosition(DALFORS_FIGHT_POS[1], DALFORS_FIGHT_POS[2], DALFORS_FIGHT_POS[3], DALFORS_FIGHT_POS[4])
        summon:MoveTo(0, DALFORS_FIGHT_POS[1], DALFORS_FIGHT_POS[2], DALFORS_FIGHT_POS[3])
        summon:AttackStart(creature)
        local list = bannerSummons[guid]
        if list then
            list[#list + 1] = summon
        end
    end
    return summon
end

-- C++ GetClosestCreatureWithEntry(me, NPC_LK, 100) (moroes precedent:
-- FindNearestCreature takes entry + radius, no alive flag).
local function talkLK(creature, textId)
    local lk = creature:FindNearestCreature(NPC_LK, 100)
    if lk then
        lk:Talk(textId)
    end
end

-- C++ EVENT_INTRO_1 (zone_icecrown.cpp:515-522): 15s after SPAWN,
-- Dalfors Talk(PRE_1), then INTRO_2 in 5s. Nil-check = C++
-- ObjectAccessor nil-check; crew cannot die pre-fight in practice
-- (no enemies exist until the waves start).
intro1 = function(creature, guid)
    local st = bannerState[guid]
    if not st or not st.crew then
        return
    end
    if st.crew.dalfors then
        st.crew.dalfors:Talk(DALFORS_SAY_PRE_1)
    end
    schedule(guid, "intro2", 5000, function()
        intro2(creature, guid)
    end)
end

-- C++ EVENT_INTRO_2 (zone_icecrown.cpp:523-532): Dalfors faces 6.215
-- + Talk(PRE_2), then INTRO_3 in 5s.
intro2 = function(creature, guid)
    local st = bannerState[guid]
    if not st or not st.crew then
        return
    end
    if st.crew.dalfors then
        st.crew.dalfors:SetFacingTo(6.215)
        st.crew.dalfors:Talk(DALFORS_SAY_PRE_2)
    end
    schedule(guid, "intro3", 5000, function()
        intro3(creature, guid)
    end)
end

-- C++ EVENT_INTRO_3 (zone_icecrown.cpp:533-574): Dalfors MovePoint +
-- home to the fight pos; priests face 5.7421 + home to intro pos;
-- masons MovePoint + home to action pos; then START_FIGHT in 5s
-- and MASON_ACTION in 15s.
intro3 = function(creature, guid)
    local st = bannerState[guid]
    if not st or not st.crew then
        return
    end
    local crew = st.crew
    if crew.dalfors then
        crew.dalfors:MoveTo(0, DALFORS_FIGHT_POS[1], DALFORS_FIGHT_POS[2], DALFORS_FIGHT_POS[3])
        crew.dalfors:SetHomePosition(DALFORS_FIGHT_POS[1], DALFORS_FIGHT_POS[2], DALFORS_FIGHT_POS[3], DALFORS_FIGHT_POS[4])
    end
    for i, priest in ipairs(crew.priests) do
        if priest then
            local pos = PRIEST_INTRO_POS[i]
            priest:SetFacingTo(5.7421)
            priest:SetHomePosition(pos[1], pos[2], pos[3], pos[4])
        end
    end
    for i, mason in ipairs(crew.masons) do
        if mason then
            local pos = MASON_ACTION_POS[i]
            mason:MoveTo(0, pos[1], pos[2], pos[3])
            mason:SetHomePosition(pos[1], pos[2], pos[3], pos[4])
        end
    end
    schedule(guid, "startFight", 5000, function()
        startFight(creature, guid)
    end)
    schedule(guid, "masonAction", 15000, function()
        masonAction(creature, guid)
    end)
end

-- C++ EVENT_START_FIGHT (zone_icecrown.cpp:586-594): LK Talk 0 +
-- Dalfors Talk(START), then WAVE_SPAWN in 1s.
startFight = function(creature, guid)
    local st = bannerState[guid]
    if not st or not st.crew then
        return
    end
    talkLK(creature, LK_TALK_1)
    if st.crew.dalfors then
        st.crew.dalfors:Talk(DALFORS_SAY_START)
    end
    schedule(guid, "wave", 1000, function()
        waveSpawn(creature, guid)
    end)
end

-- C++ EVENT_WAVE_SPAWN (zone_icecrown.cpp:595-660): LK Talk 2 at
-- PhaseCount == 3 (4th wave) and Talk 3 at == 6 (7th wave), checked
-- before the increment; 1x drudge at the mason-3 spawn pos + 2x
-- plagebringer (urand(0,1) == 0, math.random() < 0.5 twin) or 2x
-- captain at the mason-1/2 spawn pos; next wave in 10-20s while
-- PhaseCount < 8, else HALOF in 10-20s.
waveSpawn = function(creature, guid)
    local st = bannerState[guid]
    if not st then
        return
    end
    if st.phaseCount == 3 then
        talkLK(creature, LK_TALK_2)
    elseif st.phaseCount == 6 then
        talkLK(creature, LK_TALK_3)
    end
    summonWaveCreature(creature, guid, NPC_SCOURGE_DRUDGE, MASON_SPAWN_POS[3])
    if math.random() < 0.5 then
        summonWaveCreature(creature, guid, NPC_HIDEOUS_PLAGEBRINGER, MASON_SPAWN_POS[1])
        summonWaveCreature(creature, guid, NPC_HIDEOUS_PLAGEBRINGER, MASON_SPAWN_POS[2])
    else
        summonWaveCreature(creature, guid, NPC_REANIMATED_CAPTAIN, MASON_SPAWN_POS[1])
        summonWaveCreature(creature, guid, NPC_REANIMATED_CAPTAIN, MASON_SPAWN_POS[2])
    end
    st.phaseCount = st.phaseCount + 1
    if st.phaseCount < 8 then
        schedule(guid, "wave", {10000, 20000}, function()
            waveSpawn(creature, guid)
        end)
    else
        schedule(guid, "halof", {10000, 20000}, function()
            halofEvent(creature, guid)
        end)
    end
end

-- C++ EVENT_HALOF (zone_icecrown.cpp:661-688): LK Talk 4 + 2x drudge
-- at the mason-1/2 spawn pos + Halof at the Dalfors spawn pos.
halofEvent = function(creature, guid)
    talkLK(creature, LK_TALK_4)
    summonWaveCreature(creature, guid, NPC_SCOURGE_DRUDGE, MASON_SPAWN_POS[1])
    summonWaveCreature(creature, guid, NPC_SCOURGE_DRUDGE, MASON_SPAWN_POS[2])
    summonWaveCreature(creature, guid, NPC_HALOF_THE_DEATHBRINGER, DALFORS_SPAWN_POS)
end

-- C++ EVENT_MASON_ACTION (zone_icecrown.cpp:575-585): masons face
-- their action-pos orientations + AI()->SetData(1, 1). The facing
-- legs are real; SetData stores type 1 = 1 on the mason but no SAI
-- dispatch reads it in the model, so the torch actions stay out.
masonAction = function(creature, guid)
    local st = bannerState[guid]
    if not st or not st.crew then
        return
    end
    for i, mason in ipairs(st.crew.masons) do
        if mason then
            mason:SetFacingTo(MASON_ACTION_POS[i][4])
            mason:SetData(1, 1)
        end
    end
end

-- C++ EVENT_ENDED (zone_icecrown.cpp:689-692): Summons.DespawnAll()
-- + self-despawn, in that order.
endedEvent = function(creature, guid)
    despawnBannerSummons(guid)
    bannerState[guid] = nil
    creature:Despawn()
end

-- C++ victory machine (zone_icecrown.cpp:693-707): every UpdateAI tick,
-- PhaseCount == 8 + Halof dead -> triggered victory self-cast +
-- DespawnEntry waves + Dalfors Talk(FINISHED) + EVENT_ENDED 10s.
-- Lua has no per-tick UpdateAI pump for the banner, so event 21
-- (SummonedCreatureDies, coren precedent — Lua summons carry the
-- OwnerGUID link) is the exact trigger: it fires precisely when
-- Halof dies, and the phaseCount + entry gates reproduce the C++
-- condition. Once-latch: C++ would re-fire per tick until ENDED.
local function onSummonedDied(event, creature, summon, killer)
    local guid = creature:GetGUID()
    local st = bannerState[guid]
    if not st or st.victoryFired then
        return
    end
    if st.phaseCount ~= 8 then
        return
    end
    if not summon or summon:GetEntry() ~= NPC_HALOF_THE_DEATHBRINGER then
        return
    end
    st.victoryFired = true
    creature:CastSpell(creature, SPELL_CRUSADERS_SPIRE_VICTORY, true)
    local list = bannerSummons[guid]
    if list then
        local kept = {}
        for _, s in ipairs(list) do
            if s then
                local e = s:GetEntry()
                if e == NPC_HIDEOUS_PLAGEBRINGER or e == NPC_REANIMATED_CAPTAIN or e == NPC_SCOURGE_DRUDGE or e == NPC_HALOF_THE_DEATHBRINGER then
                    s:Despawn()
                else
                    kept[#kept + 1] = s
                end
            end
        end
        bannerSummons[guid] = kept
    end
    if st.crew and st.crew.dalfors then
        st.crew.dalfors:Talk(DALFORS_YELL_FINISHED)
    end
    schedule(guid, "ended", 10000, function()
        endedEvent(creature, guid)
    end)
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 21, onSummonedDied)

-- C++ EVENT_SPAWN (zone_icecrown.cpp:465-514): 7 DoSummon (default
-- 30s CORPSE_TIMED_DESPAWN = Lua type 5 + 30000ms, slad'ran precedent)
-- + MovePoint(0, walk-in) each; no DoZoneInCombat/AttackStart in C++.
local function spawnBannerCrew(creature, guid)
    if not creature:IsAlive() then
        return
    end
    local list = {}
    bannerSummons[guid] = list
    -- Indexed slots, not list positions: a failed summon must leave a
    -- nil slot, not shift the crew mapping (Dalfors, priests 1-3,
    -- masons 1-3, in spawnTable order).
    local slots = {}
    for i, s in ipairs(spawnTable) do
        local summon = creature:SummonCreature(s[1], s[2], s[3], s[4], s[5], 5, 30000)
        slots[i] = summon
        if summon then
            summon:MoveTo(0, s[6], s[7], s[8])
            list[#list + 1] = summon
        end
    end
    -- C++ guidDalfors/guidPriest/guidMason latch (:468-510), now live
    -- instead of write-only: the intro/wave/victory legs read it.
    bannerState[guid] = {
        crew = {
            dalfors = slots[1],
            priests = {slots[2], slots[3], slots[4]},
            masons = {slots[5], slots[6], slots[7]},
        },
        phaseCount = 0,
        victoryFired = false,
    }
    -- C++ EVENT_SPAWN schedules EVENT_INTRO_1 15s (:514).
    schedule(guid, "intro1", 15000, function()
        intro1(creature, guid)
    end)
end

-- C++ Reset() arm in arm order: SetRegenerateHealth(false) (bridged —
-- lua_creature_events.go, honored by the Go regen tick) -> non-triggered
-- self-cast SPELL_THREAT_PULSE + Talk(BANNER_SAY) -> ScheduleEvent
-- (EVENT_SPAWN, 3s). The banner never engages, so the fragment is
-- one-shot per spawn / reset; the 3s EVENT_SPAWN crew (7 DoSummon +
-- MovePoint walk-ins, zone_icecrown.cpp:465-514) ports via the MoveTo
-- twin with return-value latching (coren precedent); per-NPC GUID vars
-- port as the crew table in spawnBannerCrew, driving the intro/wave
-- legs below.
local function onSpawnOrReset(_, creature)
    -- C++ npc_blessed_bannerAI::Reset(): SetRegenerateHealth(false) first,
    -- unconditionally (zone_icecrown.cpp:437-443), then the alive-gated
    -- cast + talk.
    creature:SetRegenerateHealth(false)
    -- C++ npc_blessed_bannerAI::Reset(): DoCast(SPELL_THREAT_PULSE) +
    -- Talk(BANNER_SAY) (zone_icecrown.cpp). Alive-gated so the death-leg
    -- ev23 fire (banner self-despawns in JustDied) is not phantom; the
    -- 3s EVENT_SPAWN crew ported above (MoveTo twin, coren latch).
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PULSE)
        creature:Talk(BANNER_SAY)
        -- C++ Reset: events.ScheduleEvent(EVENT_SPAWN, 3s) (:442).
        local guid = creature:GetGUID()
        cancelTimers(guid)
        schedule(guid, "spawn", 3000, function()
            spawnBannerCrew(creature, guid)
        end)
    end
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 5, onSpawnOrReset)
RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 23, onSpawnOrReset)

-- C++ npc_blessed_bannerAI::JustDied (zone_icecrown.cpp:455-459):
-- Summons.DespawnAll() ports via the EVENT_SPAWN latch (despawn before
-- self-Despawn, C++ order); me->DespawnOrUnsummon() ports
-- (creature:Despawn, bridged).
local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustDied order (:455-459): Summons.DespawnAll() first, then
    -- me->DespawnOrUnsummon().
    despawnBannerSummons(guid)
    bannerState[guid] = nil
    creature:Despawn()
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 4, onDied)

-- Leave combat (event 2): no C++ evade drain for the banner (Reset
-- re-schedules EVENT_SPAWN on ev23); cancel the pending crew only.
local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 2, onLeaveCombat)

-- C++ npc_blessed_bannerAI::MoveInLineOfSight (zone_icecrown.cpp:447) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the banner is a quest object, never attacks). Go's
-- fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :447 C++-exact (the cyanigosa precedent).
local function onMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 27, onMoveInLOS)
RegisterCreatureEvent(BLESSED_BANNER_ENTRY, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- zone_icecrown.cpp:461: custom UpdateAI never calls DoMeleeAttackIfReady (summon event machine in C++, never melees)
    creature:SetCombatMovement(false) -- zone_icecrown.cpp:421: ctor arm, ScriptedAI stands-and-casts (eredar_twins convention)
end)

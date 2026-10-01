-- Morogrim Tidewalker (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_morogrim_tidewalker.cpp (boss_morogrim_tidewalker,
-- npc_water_globule — the two AI classes AddSC_boss_morogrim_
-- tidewalker registers; NPC_TIDEWALKER_LURKER = 21920 has no AI
-- class in this file — documented only, not registered).
-- Entry 21213 verified externally (mmo4ever creature id 21213 —
-- Morogrim Tidewalker; hydross/gruul precedent); 21913 comes
-- from the file's own Creatures enum. The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk lines used: SAY_AGGRO=0 (pull — fired by StartEvent in
-- C++ JustEngagedWith; the modeled fight starts on pull),
-- SAY_SUMMON=1 (earthquake murloc summon — the Talk is
-- unconditional in C++ even though the summons have no bridge),
-- SAY_SUMMON_BUBL=2 (watery grave), SAY_SLAY=3 (kill — NO
-- TYPEID gate in C++, C++-exact), SAY_DEATH=4 (death), EMOTE_
-- WATERY_GRAVE=5, EMOTE_EARTHQUAKE=6, EMOTE_WATERY_GLOBULES=7.
-- All seven are referenced in the file.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention);
-- DoCastVictim takes nil as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_morogrim_tidewalker (21213): OnEnterCombat(1): per-GUID
-- scheduler reset to the C++ Initialize() values (tidalWave
-- 10000 / wateryGrave 30000 / earthquake 40000 / wateryGlobules
-- 0 / earthquakeArm false / phase2 false) + Talk(SAY_AGGRO) +
-- 1s scheduler pump (a port of boss_morogrim_tidewalkerAI::
-- UpdateAI — 1s granularity is exact for all C++ timers here).
-- Pump: earthquake 40s init — first fire -> DoCastVictim 37764,
-- earthquakeArm=true, re-arm 10000; second fire -> Talk(SAY_
-- SUMMON) + Talk(EMOTE_EARTHQUAKE), earthquakeArm=false,
-- re-arm 40000 + rand32()%5000 (C++-exact; the 10x Tidewalker
-- Lurker summons at the MurlocCords coords + the AttackStart
-- relay have no summon/cross-creature bridges — unmodeled,
-- below). Tidal wave 37730 10s then 20s -> DoCastVictim (C++-
-- exact). Phase 1 (!phase2): watery grave 30s -> Talk(SAY_
-- SUMMON_BUBL) + Talk(EMOTE_WATERY_GRAVE), re-arm 30000 (C++-
-- exact; the 4-target distinct pick, the waterfall teleport and
-- the player-side triggered casts of 38023/38024/38025/37850
-- have no player bridges — unmodeled, below). Phase 2: once-
-- guarded GetHealthPct() < 25 (C++ HealthBelowPct(25)) ->
-- phase2=true; watery globules — 0 init so it fires on the
-- first phase-2 tick -> Talk(EMOTE_WATERY_GLOBULES), re-arm
-- 25000 (C++-exact; the 4-target distinct pick + the player-
-- side triggered casts of 37854/37858/37860/37861 have no
-- player bridges — unmodeled, below). OnTargetDied(3): Talk(
-- SAY_SLAY) (no TYPEID gate, C++-exact). OnDied(4): Talk(SAY_
-- DEATH) + cleanup (the SetData(DONE) arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the SetData(NOT_STARTED) arm is instance-blocked).
-- Melee is engine-driven.
-- npc_water_globule (21913): OnEnterCombat(1): per-GUID reset
-- (check 1000) + 1s pump; pump: if the victim exists and is
-- within 5 yd (GetDistance bridge for the IsWithinDist gate) ->
-- CastSpell(victim, SPELL_GLOBULE_EXPLOSION 37871) non-
-- triggered (C++ DoCastVictim), creature:Despawn() + cancel the
-- pump (C++ DespawnOrUnsummon). The C++ Check_Timer re-arms at
-- 500ms after the first check; the 1s pump checks every 1000ms
-- instead (1s granularity, documented below). The Reset flag/
-- faction arms have no flag/faction bridges — unmodeled; the
-- MoveInLineOfSight aggro machine has no LOS hook — unmodeled;
-- the C++ do-NOT-melee arm has no no-melee bridge — engine
-- melee applies (leotheras demon-form precedent). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_MOROGRIMTIDEWALKEREVENT NOT_STARTED/IN_PROGRESS/
-- DONE bookkeeping and the JustEngagedWith Playercount arm
-- skipped); no summon bridge — the 10x Tidewalker Lurker
-- (21920) summons at the MurlocCords coords unmodeled; no
-- cross-creature bridge — the murloc AttackStart relay
-- unmodeled; no player bridges — the watery-grave waterfall
-- teleport + the player-side triggered casts (38023/38024/
-- 38025/37850) and the globule-summon player-side triggered
-- casts (37854/37858/37860/37861) unmodeled (the C++ target-
-- pick loops with dedup collapse with the unbridgeable casts);
-- no LOS/flag/faction/no-melee bridges for the globule (above);
-- the globule Check_Timer runs at 1s pump granularity instead
-- of the C++ 500ms re-arm; no SpellScript/AuraScript scripts
-- in this file.

local SPELL_TIDAL_WAVE = 37730
local SPELL_EARTHQUAKE = 37764
local SPELL_GLOBULE_EXPLOSION = 37871

local SAY_AGGRO = 0
local SAY_SUMMON = 1
local SAY_SUMMON_BUBL = 2
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_WATERY_GRAVE = 5
local EMOTE_EARTHQUAKE = 6
local EMOTE_WATERY_GLOBULES = 7

local ENTRY_MOROGRIM = 21213
local ENTRY_WATER_GLOBULE = 21913

local GLOBULE_EXPLOSION_RANGE = 5

local morogrimTimers = {}
local morogrimState = {}
local globuleTimers = {}

local function cancelPump(timers, guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetMorogrim(guid)
    cancelPump(morogrimTimers, guid)
    morogrimState[guid] = nil
end

-- C++ Initialize(): tidalWave 10000, wateryGrave 30000,
-- earthquake 40000, wateryGlobules 0, earthquakeArm false,
-- phase2 false.
local function freshMorogrimState()
    return {
        tidalWave = 10000,
        wateryGrave = 30000,
        earthquake = 40000,
        wateryGlobules = 0,
        earthquakeArm = false,
        phase2 = false,
    }
end

local function morogrimTick(creature, guid)
    local st = morogrimState[guid]
    if not st then
        return
    end

    -- Earthquake 40s init: first fire -> DoCastVictim, arm the
    -- second stage in 10s; second fire -> Talk + Talk(EMOTE),
    -- re-arm 40-45s (the 10x murloc summon + AttackStart relay
    -- is summon/cross-creature blocked — header).
    if st.earthquake <= 1000 then
        if not st.earthquakeArm then
            creature:CastSpell(nil, SPELL_EARTHQUAKE)
            st.earthquakeArm = true
            st.earthquake = 10000
        else
            creature:Talk(SAY_SUMMON)
            creature:Talk(EMOTE_EARTHQUAKE)
            st.earthquakeArm = false
            st.earthquake = 40000 + math.random(0, 4999)
        end
    else
        st.earthquake = st.earthquake - 1000
    end

    -- Tidal wave 10s then 20s: DoCastVictim (C++-exact).
    if st.tidalWave <= 1000 then
        creature:CastSpell(nil, SPELL_TIDAL_WAVE)
        st.tidalWave = 20000
    else
        st.tidalWave = st.tidalWave - 1000
    end

    if not st.phase2 then
        -- Watery grave 30s: talks + re-arm (the 4-target pick,
        -- teleport and player-side casts have no player bridges
        -- — header).
        if st.wateryGrave <= 1000 then
            creature:Talk(SAY_SUMMON_BUBL)
            creature:Talk(EMOTE_WATERY_GRAVE)
            st.wateryGrave = 30000
        else
            st.wateryGrave = st.wateryGrave - 1000
        end

        -- Phase 2: once-guarded below 25% (C++ HealthBelowPct).
        if creature:GetHealthPct() < 25 then
            st.phase2 = true
        end
    else
        -- Watery globules: 0 init so it fires on the first
        -- phase-2 tick, then every 25s; talks only — the target
        -- picks + player-side casts have no bridges (header).
        if st.wateryGlobules <= 1000 then
            creature:Talk(EMOTE_WATERY_GLOBULES)
            st.wateryGlobules = 25000
        else
            st.wateryGlobules = st.wateryGlobules - 1000
        end
    end
end

local function morogrimEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetMorogrim(guid)
    morogrimState[guid] = freshMorogrimState()
    creature:Talk(SAY_AGGRO)
    morogrimTimers[guid] = CreateLuaEvent(function()
        morogrimTick(creature, guid)
    end, 1000)
end

local function morogrimLeaveCombat(event, creature)
    resetMorogrim(creature:GetGUID())
end

-- C++ KilledUnit: NO TYPEID gate -> Talk(SAY_SLAY), C++-exact.
local function morogrimTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function morogrimDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetMorogrim(creature:GetGUID())
end

local function morogrimReset(event, creature)
    resetMorogrim(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MOROGRIM, 1, morogrimEnterCombat)
RegisterCreatureEvent(ENTRY_MOROGRIM, 2, morogrimLeaveCombat)
RegisterCreatureEvent(ENTRY_MOROGRIM, 3, morogrimTargetDied)
RegisterCreatureEvent(ENTRY_MOROGRIM, 4, morogrimDied)
RegisterCreatureEvent(ENTRY_MOROGRIM, 23, morogrimReset)

-- npc_water_globule (21913) — see header.

local function globuleTick(creature, guid)
    local victim = creature:GetVictim()
    if not victim then
        return
    end

    -- Check_Timer: within 5 yd of the victim -> DoCastVictim
    -- 37871 + DespawnOrUnsummon (C++-exact; 1s pump granularity
    -- instead of the C++ 500ms re-arm — header).
    if creature:GetDistance(victim) <= GLOBULE_EXPLOSION_RANGE then
        creature:CastSpell(victim, SPELL_GLOBULE_EXPLOSION)
        creature:Despawn()
        cancelPump(globuleTimers, guid)
        return
    end
end

local function globuleEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelPump(globuleTimers, guid)
    globuleTimers[guid] = CreateLuaEvent(function()
        globuleTick(creature, guid)
    end, 1000)
end

local function globuleCleanup(event, creature)
    cancelPump(globuleTimers, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 1, globuleEnterCombat)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 2, globuleCleanup)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 4, globuleCleanup)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 23, globuleCleanup)

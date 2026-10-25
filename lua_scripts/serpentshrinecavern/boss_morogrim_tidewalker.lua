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
-- UpdateAI — 1s granularity (not exact: the earthquake re-arm
-- 40000+rand%5000 is generally not a multiple of 1000, and the
-- globule re-arm is 500ms — both admitted below).
-- Pump: earthquake 40s init — first fire -> DoCastVictim 37764,
-- earthquakeArm=true, re-arm 10000; second fire -> Talk(SAY_
-- SUMMON) + 10x Tidewalker Lurker summons at the MurlocCords
-- coords (each DoZoneInCombat — the AttackStart substitute, since
-- lurkers carry no custom AI) + Talk(EMOTE_EARTHQUAKE),
-- earthquakeArm=false, re-arm 40000 + rand32()%5000 — PORTED
-- 2026-10-24 (the per-lurker separately-random target assignment
-- stays out; zone aggro substitutes for it). The
-- engine LOS-aggro scan does not replicate per-lurker random
-- targeting). Tidal wave 37730 10s then 20s -> DoCastVictim (C++-
-- exact). Phase 1 (!phase2): watery grave 30s -> Talk(SAY_
-- SUMMON_BUBL) + Talk(EMOTE_WATERY_GRAVE), re-arm 30000 (C++-
-- exact; the 4-target distinct pick and the player-side triggered
-- casts of 38023/38024/38025/37850 stay out — the true blockers are
-- (a) no random-players-only pick and (b) no triggered flag on
-- player:CastSpell (it exists but takes no triggered arg); C++
-- itself does no teleport call here, any waterfall movement is a
-- spell effect of the grave spells — unmodeled, below). Phase 2: once-
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
-- faction arms port to the dedicated ev23 globuleReset handler
-- (RemoveFlag/SetFlag NOT_SELECTABLE pair + SetFaction(FACTION_
-- MONSTER), C++-verbatim order, IsDead-gated off the death path —
-- SetFlag(59)/SetFaction bridges live, ouro precedent); the
-- MoveInLineOfSight aggro machine is unmodeled (CREATURE_EVENT_ON_MOVE_IN_LOS (27) exists, but the C++ arm's AttackStart relay has no creature-side bridge);
-- the C++ do-NOT-melee arm ports via creature:SetMeleeAttack(false)
-- on event 1 (2026-10-23 — the stale "no no-melee bridge / leotheras
-- precedent" claim is retracted). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (the DATA_MOROGRIMTIDEWALKEREVENT NOT_STARTED/IN_PROGRESS/
-- DONE bookkeeping and the JustEngagedWith Playercount arm
-- skipped); the 10x Tidewalker Lurker (21920) summons at the
-- MurlocCords coords are PORTED 2026-10-24 (SummonCreature + DoZoneInCombat;
-- stale "no summon bridge" claim retracted — the per-lurker
-- separately-random target assignment stays out, zone aggro substitutes);
-- cross-creature bridge — the murloc AttackStart relay
-- unmodeled; the watery-grave player-side triggered casts
-- (38023/38024/38025/37850) and the globule-summon player-side
-- triggered casts (37854/37858/37860/37861) stay out for the
-- (a)/(b) blockers above (the C++ target-pick loops with dedup
-- collapse with the unbridgeable casts);
-- the globule no-melee arm IS ported (creature:SetMeleeAttack bridge is live, 2026-10-23 — the stale "no no-melee bridge" claim is retracted); LOS event 27 exists but the AttackStart relay is still unbridged;
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
local ENTRY_TIDEWALKER_LURKER = 21920 -- boss_morogrim_tidewalker.cpp:71

-- C++ MurlocCords[10][4] (boss_morogrim_tidewalker.cpp:74-85).
local MURLOC_CORDS = {
    {424.36, -715.4, -7.14, 0.124},
    {425.13, -719.3, -7.14, 0.124},
    {425.05, -724.23, -7.14, 0.124},
    {424.91, -728.68, -7.14, 0.124},
    {424.84, -732.18, -7.14, 0.124},
    {321.05, -734.2, -13.15, 0.124},
    {321.05, -729.4, -13.15, 0.124},
    {321.05, -724.03, -13.15, 0.124},
    {321.05, -718.73, -13.15, 0.124},
    {321.05, -714.24, -13.15, 0.124},
}

local GLOBULE_EXPLOSION_RANGE = 5

-- UNIT_FIELD_FLAGS (UpdateFields.h:117: OBJECT_END+0x35 = 59);
-- UNIT_FLAG_NOT_SELECTABLE (SharedDefines.h); FACTION_MONSTER = 14.
-- The SetFlag(59)/SetFaction bridges are live (ouro precedent); the
-- old "Reset flag/faction arms have no flag/faction bridges" claim is
-- retracted 2026-10-21.
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local FACTION_MONSTER = 14

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

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() < 25 fires up to 1 HP early.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
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
    -- second stage in 10s; second fire -> Talk + 10x murloc
    -- summon + Talk(EMOTE), re-arm 40-45s (C++ boss_morogrim_
    -- tidewalker.cpp:195-207: per cord SummonCreature(TIDEWALKER_
    -- LURKER 21920, type 3, 10000ms) + AttackStart(random target);
    -- the per-lurker target assignment rides DoZoneInCombat here
    -- (the AttackStart substitute — lurkers carry no custom AI, so
    -- engine combat carries them; the C++ per-summon target-exists
    -- gate is vacuously true in combat).
    if st.earthquake <= 1000 then
        if not st.earthquakeArm then
            creature:CastSpell(nil, SPELL_EARTHQUAKE)
            st.earthquakeArm = true
            st.earthquake = 10000
        else
            creature:Talk(SAY_SUMMON)
            for _, cord in ipairs(MURLOC_CORDS) do
                local lurker = creature:SummonCreature(ENTRY_TIDEWALKER_LURKER,
                    cord[1], cord[2], cord[3], cord[4], 3, 10000)
                if lurker then
                    lurker:DoZoneInCombat()
                end
            end
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

        -- Phase 2: once-guarded below 25% (C++ HealthBelowPct(25),
        -- floored integer threshold).
        if healthBelowPct(creature, 25) then
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
    -- Repeats arg 0 = infinite (runtime.go:1189-1194: default repeats is
    -- 1, i.e. one-shot — the 2026-10-25 audit caught both pumps here
    -- firing exactly once, leaving the boss melee-only).
    morogrimTimers[guid] = CreateLuaEvent(function()
        morogrimTick(creature, guid)
    end, 1000, 0)
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
    if creature:IsWithinDistInMap(victim, GLOBULE_EXPLOSION_RANGE) then
        creature:CastSpell(victim, SPELL_GLOBULE_EXPLOSION)
        creature:Despawn()
        cancelPump(globuleTimers, guid)
        return
    end
end

local function globuleEnterCombat(event, creature)
    creature:SetMeleeAttack(false) -- boss_morogrim_tidewalker.cpp:344: custom UpdateAI never calls DoMeleeAttackIfReady (C++ comments "do NOT deal any melee damage to the target")
    local guid = creature:GetGUID()
    cancelPump(globuleTimers, guid)
    globuleTimers[guid] = CreateLuaEvent(function()
        globuleTick(creature, guid)
    end, 1000, 0)
end

local function globuleCleanup(event, creature)
    cancelPump(globuleTimers, creature:GetGUID())
end

-- C++ npc_water_globuleAI::Reset (boss_morogrim_tidewalker.cpp:319-326):
-- Initialize() (the Check_Timer=1000 leg is the 1s pump above) then the
-- RemoveFlag/SetFlag NOT_SELECTABLE pair (a net set regardless of
-- template state) + SetFaction(FACTION_MONSTER), C++-verbatim order.
-- The IsDead gate keeps it off the death path (C++ Reset never runs on
-- death; the twins' spark_overgrind IsDead-gate precedent).
local function globuleReset(event, creature)
    if not creature:IsDead() then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetFaction(FACTION_MONSTER)
    end
end

RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 1, globuleEnterCombat)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 2, globuleCleanup)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 4, globuleCleanup)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 23, globuleCleanup)
RegisterCreatureEvent(ENTRY_WATER_GLOBULE, 23, globuleReset)

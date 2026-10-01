-- Warbringer O'mrogg (Shattered Halls, Hellfire Citadel) — Lua
-- port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/boss_warbringer_omrogg.cpp (boss_warbringer_
-- omrogg — the boss AI class AddSC registers; npc_omrogg_
-- heads is registered alongside).
-- Second boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- cpp stays blocked on the instance-script model).
-- Entry: no NPC_OMROGG entry exists in shattered_halls.h and
-- instance_shattered_halls.cpp has no OnCreatureCreate
-- mapping for the boss, so the entry is verified externally:
-- wowhead npc=16809/warbringer-omrogg, mmo4ever creature
-- 16809. The two heads are C++ Creatures-enum constants in
-- the .cpp itself: NPC_LEFT_HEAD = 19523, NPC_RIGHT_HEAD =
-- 19524. The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- Talk: the boss itself only ever talks EMOTE_ENRAGE = 2
-- (fired from the burning-maul arm, C++-exact); YELL_DIE_L
-- = 0 / YELL_DIE_R = 1 and the whole GoCombat/GoCombatDelay/
-- Threat/ThreatDelay1/ThreatDelay2/Killing/KillingDelay
-- talk tables fire only as Talk() on the summoned head
-- creatures via ObjectAccessor GetCreature — unreachable in
-- this model.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): O'mrogg
-- (16809): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (blastWave 0 / blastCount 0 / fear
-- 8000 / burningMaul 25000 / thunderclap 15000 /
-- resetThreat 30000 — the C++ JustEngagedWith override only
-- summons the heads, talks through the left head and pokes
-- the instance, all summon/cross-creature/instance blocked,
-- so the Initialize() schedule lands on the only engagement
-- hook this model has) + 1s scheduler pump (a port of
-- UpdateAI in C++ arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump).
-- Pump: blast wave 30600 — fires only while blastCount is
-- set: 16s after burning maul then 5s later (2 casts total,
-- C++-exact bookkeeping), non-triggered DoCastSelf
-- (C++-exact); burning maul 30598: 25s init then 40s —
-- Talk(EMOTE_ENRAGE), non-triggered DoCastSelf (C++-exact),
-- re-arm 40000, blastWave = 16000, blastCount = 1
-- (C++-exact); reset threat 30s init then {25s,40s} —
-- bookkeeping only (the C++ arm's DoYellForThreat head-talk
-- sequence and the ResetThreatList/AddThreat calls have no
-- bridges, so the pick-and-reset is unmodeled), re-arm
-- regardless (C++-exact); fear 30584: 8s init then {15s,35s}
-- — non-triggered DoCastSelf (C++ DoCast(me), C++-exact),
-- re-arm {15s,35s} regardless (C++-exact); thunderclap
-- 30633: 15s init then {15s,30s} — non-triggered DoCastSelf
-- (C++-exact), re-arm {15s,30s} regardless (C++-exact).
-- OnDied(4): cleanup (the _JustDied arm is instance-blocked;
-- the head Talk(YELL_DIE_L)/SetData(SETDATA_YELL) death
-- relay is cross-creature blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state (the
-- _Reset arm is instance-blocked; the Reset() head
-- setDeathState(JUST_DIED) arms are cross-creature blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_shattered_halls.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_OMROGG = 1
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_OMROGG is an instance-side constant, unbridgeable;
-- the JustEngagedWith SetBossState(DATA_OMROGG, IN_PROGRESS)
-- arm, the Reset SetData(DATA_OMROGG, NOT_STARTED) arm and
-- the DATA_KARGATH cross-boss assist arms in instance_
-- shattered_halls.cpp are instance gated); no summon bridge
-- — the JustEngagedWith SummonCreature(NPC_LEFT_HEAD/
-- NPC_RIGHT_HEAD, TEMPSUMMON_DEAD_DESPAWN) calls never fire
-- and the JustSummoned GUID tracking + SetVisible(false)
-- setup is unreachable (steamrigger precedent); no
-- ObjectAccessor/cross-creature bridge — the whole Delay_
-- Timer yell machine (4s init, 3.5s re-arm; the AggroYell/
-- ThreatYell/ThreatYell2/KillingYell flags set only via the
-- head GUID relays in JustEngagedWith/DoYellForThreat/
-- KilledUnit) never runs because the heads never exist, and
-- its talk text ids belong to the head entries (19523/
-- 19524), so the lines cannot be re-pointed at the boss
-- without changing the spoken text — the machine is
-- unmodeled rather than ticked dead; npc_omrogg_heads
-- documented only, not registered — its SetData -> 4s
-- EVENT_DEATH_YELL -> Talk(YELL_DIE_R) + setDeathState arm
-- is reachable only via the boss JustDied ObjectAccessor
-- relay, and its Reset/JustEngagedWith overrides are empty
-- (ahune bunny / nethekurse fissure precedent); no threat
-- bridge — the ResetThreatList/AddThreat arms unmodeled
-- (timer bookkeeping exact); no difficulty bridge — the
-- H_SPELL_BURNING_MAUL 36056 variant unmodeled (thespia
-- precedent); the ScriptData "Heroic enabled. Spell timing
-- may need additional tweaks" (SD%Complete: 85) is an
-- upstream caveat, documented, not bridged; no movement
-- bridge — no movement arms in this file; no SpellScript/
-- AuraScript scripts in this file.

local SPELL_BLAST_WAVE = 30600
local SPELL_FEAR = 30584
local SPELL_THUNDERCLAP = 30633
local SPELL_BURNING_MAUL = 30598

local EMOTE_ENRAGE = 2

local ENTRY_OMROGG = 16809

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ Initialize() values (the JustEngagedWith summon +
-- head-talk + SetBossState arms are summon/cross-creature/
-- instance blocked, so the schedule lands on OnEnterCombat):
-- blastWave 0, blastCount 0, fear 8000, burningMaul 25000,
-- thunderclap 15000, resetThreat 30000.
local function freshState()
    return {
        blastWave = 0,
        blastCount = 0,
        fear = 8000,
        burningMaul = 25000,
        thunderclap = 15000,
        resetThreat = 30000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Blast wave 30600: fires only while blastCount is set —
    -- 16s after burning maul then 5s later, 2 casts total
    -- (C++-exact bookkeeping), non-triggered DoCastSelf.
    if st.blastCount > 0 then
        if st.blastWave <= 1000 then
            creature:CastSpell(nil, SPELL_BLAST_WAVE)
            st.blastCount = st.blastCount + 1
            if st.blastCount == 3 then
                st.blastCount = 0
            else
                st.blastWave = 5000
            end
        else
            st.blastWave = st.blastWave - 1000
        end
    end

    -- Burning maul 30598: 25s init then 40s — Talk(EMOTE_
    -- ENRAGE), non-triggered DoCastSelf (C++-exact), re-arm
    -- 40000, blastWave = 16000, blastCount = 1 (C++-exact).
    -- The heroic 36056 variant has no difficulty bridge
    -- (thespia precedent).
    if st.burningMaul <= 1000 then
        creature:Talk(EMOTE_ENRAGE)
        creature:CastSpell(nil, SPELL_BURNING_MAUL)
        st.burningMaul = 40000
        st.blastWave = 16000
        st.blastCount = 1
    else
        st.burningMaul = st.burningMaul - 1000
    end

    -- Reset threat: 30s init then {25s,40s} — bookkeeping
    -- only (the DoYellForThreat head-talk sequence and the
    -- ResetThreatList/AddThreat arms have no bridges), re-arm
    -- regardless (C++-exact).
    if st.resetThreat <= 1000 then
        st.resetThreat = math.random(25000, 39999)
    else
        st.resetThreat = st.resetThreat - 1000
    end

    -- Fear 30584: 8s init then {15s,35s} — non-triggered
    -- DoCastSelf (C++ DoCast(me), C++-exact), re-arm
    -- regardless (C++-exact).
    if st.fear <= 1000 then
        creature:CastSpell(nil, SPELL_FEAR)
        st.fear = math.random(15000, 34999)
    else
        st.fear = st.fear - 1000
    end

    -- Thunderclap 30633: 15s init then {15s,30s} —
    -- non-triggered DoCastSelf (C++-exact), re-arm regardless
    -- (C++-exact).
    if st.thunderclap <= 1000 then
        creature:CastSpell(nil, SPELL_THUNDERCLAP)
        st.thunderclap = math.random(15000, 29999)
    else
        st.thunderclap = st.thunderclap - 1000
    end
end

RegisterCreatureEvent(ENTRY_OMROGG, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- No event 3: the C++ KilledUnit override is entirely head-
-- gated (returns without the heads), so it is unreachable in
-- this model (broggok precedent).

RegisterCreatureEvent(ENTRY_OMROGG, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_OMROGG, 4, function(_, creature)
    -- No boss death talk: YELL_DIE_L/R fire only from the
    -- heads via the cross-creature relay (blocked).
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_OMROGG, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)

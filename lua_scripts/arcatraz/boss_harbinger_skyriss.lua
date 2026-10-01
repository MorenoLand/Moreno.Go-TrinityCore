-- Harbinger Skyriss (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_harbinger_skyriss.cpp (boss_harbinger_
-- skyriss and boss_harbinger_skyriss_illusion — the two
-- CreatureScript AI classes AddSC_boss_harbinger_skyriss
-- registers; no SpellScript/AuraScript scripts in this
-- file).
-- Fourth and final boss in the Arcatraz set per outland_
-- script_loader.cpp order; with this port the Arcatraz boss
-- roster is 4/4 COMPLETE (instance_arcatraz.cpp stays blocked
-- on the instance-script model).
-- Entry: 20912 (arcatraz.h carries no NPC_ constant for
-- Skyriss — it lists only Dalliah 20885 / Soccothrates 20886
-- / Mellichar 20904 / Millhouse 20977 / Alpha Pod Target
-- 21436; the entry is verifiable from the C++ sources:
-- arcatraz.cpp:260 ENTRY_SKYRISS = 20912 in the mellichar
-- stasis-pod summon wave enum; zereketh precedent for a boss
-- with no C++-side entry constant). instance_arcatraz.cpp
-- has no OnCreatureCreate mapping for the boss. The creature_
-- template ScriptName bindings are DB-side (no TDB in this
-- workspace).
-- Talk: SAY_INTRO = 0 (intro — fired from the C++ intro
-- phase-1 arm, C++-exact), SAY_AGGRO = 1 (pull — fired from
-- the C++ intro phase-2 arm, NOT from JustEngagedWith — the
-- C++ JustEngagedWith override is empty, C++-exact), SAY_
-- KILL = 2 (kill — the C++ KilledUnit talks unless the victim
-- is NPC_ALPHA_POD_TARGET 21436, C++-exact), SAY_MIND = 3
-- (domination — fired from the domination arm, C++-exact),
-- SAY_FEAR = 4 (fear — fired from the fear arm, C++-exact),
-- SAY_IMAGE = 5 (split — fired from DoSplit, C++-exact), SAY_
-- DEATH = 6 (death — fired from the C++ JustDied, C++-
-- exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 5 OnSpawn, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pumps); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Skyriss
-- (20912): OnSpawn(5): the C++ ctor / Initialize() sets the
-- intro (Intro = false, Intro_Phase = 1, Intro_Timer = 5000)
-- and the combat timers (mind rend 3000 / fear 15000 /
-- domination 30000 / mana burn 25000); the intro machine runs
-- from spawn — the C++ ticks it inside UpdateAI before the
-- !UpdateVictim early return — so a 1s intro pump starts on
-- spawn with phase 1 / 5000 (the C++ Reset() SetImmuneToAll
-- (!Intro) arm has no immune bridge and the pre-Intro
-- MoveInLineOfSight suppression has no LoS-aggro bridge, so
-- the intro runs on spawn, documented under deviations).
-- Intro pump: phase 1 Talk(SAY_INTRO), re-arm 25000; phase 2
-- Talk(SAY_AGGRO), re-arm 3000; phase 3 — Intro = true, the
-- pump self-cancels (the C++ Intro_Phase = 1 / Intro_Timer =
-- 5000 re-latch in Reset() lands here: an evade before Intro
-- restarts the intro machine — the C++ Reset() calls
-- Initialize() but leaves Intro untouched, so a wipe after
-- Intro never replays it, C++-exact).
-- OnEnterCombat(1): per-GUID reset to the C++ combat timer
-- schedule (mind rend 3000 / fear 15000 / domination 30000;
-- the C++ JustEngagedWith override is empty, so no engage
-- yell — the pull yell is the intro phase-2 SAY_AGGRO) + 1s
-- combat pump (a port of UpdateAI in C++ arm order — 1s
-- granularity exact for all C++ timers; the !UpdateVictim
-- early return collapses into the pump; the BossAI::
-- JustEngagedWith/JustDied/_Reset arms are instance-blocked,
-- DATA_HARBINGER_SKYRISS = 3 is an instance-side constant,
-- unbridgeable). Combat pump: image splits — if not yet
-- split at 66% and health is at or below 66% (C++ !HealthAbove
-- Pct(66), nethekurse precedent), DoSplit(66): Talk(SAY_
-- IMAGE) and set the 66 latch; then if not yet split at 33%
-- and health is at or below 33% (C++ !HealthAbovePct(33)),
-- DoSplit(33): Talk(SAY_IMAGE) and set the 33 latch (the C++
-- checks both arms in order on the same tick — a single hit
-- into <=33% fires both yells, C++-exact; the SPELL_66_
-- ILLUSION 36931 / SPELL_33_ILLUSION 36932 casts themselves
-- have no summon bridge — steamrigger precedent — so only
-- the talk is modeled); mind rend 36924 — 3s init then 8s,
-- target = random alive player in the instance excluding the
-- current victim (C++ SelectTarget(Random, 1), thespia
-- precedent), nil pick falls back to DoCastVictim (C++-
-- exact), non-triggered DoCast (C++-exact), re-arm 8000 (C++-
-- exact); fear 39415 — 15s init then 25s, Talk(SAY_FEAR), same
-- targeting (C++-exact), non-triggered, re-arm 25000 (C++-
-- exact; the IsNonMeleeSpellCast(false) early-return gate
-- has no cast-state bridge — the arm fires unconditionally,
-- netherspite precedent); domination 37162 — 30s init then
-- {16s,32s}, Talk(SAY_MIND), same targeting (C++-exact),
-- non-triggered, re-arm {16s,32s} (C++-exact; same cast-state
-- caveat). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_KILL) unless the killed unit's
-- entry is 21436 (NPC_ALPHA_POD_TARGET — the C++ won't yell
-- for pets/other units, vazruden entry-gate precedent, C++-
-- exact). OnDied(4): Talk(SAY_DEATH) + cleanup (the _JustDied
-- arm is instance-blocked). OnLeaveCombat(2)/OnReset(23):
-- cancel the combat pump, drop combat state, and restart the
-- intro machine from phase 1 / 5000 when Intro has not yet
-- completed (the C++ Reset() observable remainder, C++-
-- exact); the split latches persist across a wipe only after
-- Intro — matching the C++ Reset() not resetting them.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_arcatraz.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_HARBINGER_
-- SKYRISS = 3 bookkeeping in Reset/JustEngagedWith/JustDied
-- skipped); no game-object bridge — the intro phase-1
-- HandleGameObject(DATA_WARDENS_SHIELD, true) arm unmodeled;
-- no cross-creature bridge — the intro phase-2 mellic->
-- setDeathState(JUST_DIED)/SetHealth(0) kill and the phase-1
-- HandleGameObject(DATA_WARDENS_SHIELD, false) arm unmodeled
-- (instance GUID data); no immune bridge — the C++ Reset()
-- SetImmuneToAll(!Intro) arm and the intro phase-3 SetImmune
-- ToAll(false) arm unmodeled; no LoS-aggro bridge — the
-- MoveInLineOfSight Intro gate unmodeled (gargolmar
-- precedent); no summon bridge — the DoSplit illusion summons
-- and the JustSummoned health-set (66%/33%)/AttackStart-
-- random-target/Summon/immunity arms unmodeled (steamrigger
-- precedent); the illusion SpellIds H_SPELL_MIND_REND_IMAGE
-- 39021 never fire in the C++ AI — not bridged; no
-- difficulty bridge — the heroic-only H_SPELL_MANA_BURN
-- 39020 {16s,32s} arm unmodeled (thespia precedent); no
-- cast-state bridge — the DoSplit InterruptNonMeleeSpells
-- arm and the fear/domination IsNonMeleeSpellCast early-
-- return gates unmodeled (netherspite precedent); the
-- unused spell enums H_SPELL_MIND_REND 39017 / H_SPELL_
-- DOMINATION 39019 never fire in the C++ AI — not bridged
-- (millhouse unused-spells precedent); the ScriptData
-- "combatAI not fully implemented" caveat is upstream,
-- documented, not bridged; boss_harbinger_skyriss_illusion
-- (entries 21466/21467) documented only, not registered
-- (omor-heads precedent) — its C++ AI is a no-op: Reset()
-- only touches unit flags and immune-to-PC (no flag bridge)
-- and JustEngagedWith is empty, so there is nothing
-- bridgeable; no SpellScript/AuraScript scripts in this
-- file.

local SPELL_MIND_REND = 36924
local SPELL_FEAR = 39415
local SPELL_DOMINATION = 37162

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_KILL = 2
local SAY_MIND = 3
local SAY_FEAR = 4
local SAY_IMAGE = 5
local SAY_DEATH = 6

local ENTRY_SKYRISS = 20912
local ENTRY_ALPHA_POD_TARGET = 21436

local INTRO_PHASE_1 = 1
local INTRO_PHASE_2 = 2
local INTRO_PHASE_3 = 3

local introTimers = {}
local combatTimers = {}
local introStates = {}
local combatStates = {}

local function cancelIntroPump(guid)
    local id = introTimers[guid]
    if id then
        RemoveEventById(id)
        introTimers[guid] = nil
    end
end

local function cancelCombatPump(guid)
    local id = combatTimers[guid]
    if id then
        RemoveEventById(id)
        combatTimers[guid] = nil
    end
end

-- C++ Reset() / evade cleanup (the Reset() observable
-- remainder: the intro machine restarts from phase 1 / 5000
-- only while Intro has not completed — the C++ Reset() calls
-- Initialize() but leaves the Intro flag untouched, C++-
-- exact).
local function fullReset(guid)
    cancelIntroPump(guid)
    cancelCombatPump(guid)
    combatStates[guid] = nil
    local intro = introStates[guid]
    if intro and intro.done then
        return
    end
    introStates[guid] = { phase = INTRO_PHASE_1, timer = 5000, done = false }
end

local function introTick(creature, guid)
    local st = introStates[guid]
    if not st or st.done then
        return
    end

    if st.timer > 1000 then
        st.timer = st.timer - 1000
        return
    end

    -- Intro machine in C++ arm order — only the bridgeable
    -- Talk arms are modeled (the game-object / cross-creature
    -- / immune arms are instance- or bridge-blocked, see
    -- deviations).
    if st.phase == INTRO_PHASE_1 then
        creature:Talk(SAY_INTRO)
        st.phase = INTRO_PHASE_2
        st.timer = 25000
    elseif st.phase == INTRO_PHASE_2 then
        creature:Talk(SAY_AGGRO)
        st.phase = INTRO_PHASE_3
        st.timer = 3000
    elseif st.phase == INTRO_PHASE_3 then
        -- C++: me->SetImmuneToAll(false); Intro = true.
        st.done = true
        cancelIntroPump(guid)
    end
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 1): a random alive player in the
-- instance excluding the current victim (thespia precedent),
-- player-only.
local function randomNonVictimPlayer(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if not victim or p:GetGUID() ~= victim:GetGUID() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ DoSplit: InterruptNonMeleeSpells (no cast-state
-- bridge — netherspite precedent), Talk(SAY_IMAGE), then
-- DoCast(me, SPELL_66/33_ILLUSION) — the casts have no
-- summon bridge, so only the talk is modeled (steamrigger
-- precedent).
local function doSplit(creature)
    creature:Talk(SAY_IMAGE)
end

-- C++ combat timers (Initialize values; the C++ JustEngaged
-- With override is empty, so the whole schedule lands on
-- OnEnterCombat — omor precedent): mind rend 3000 / fear
-- 15000 / domination 30000; the split latches start false.
local function freshCombatState()
    return {
        mindRend = 3000,
        fear = 15000,
        domination = 30000,
        isImage66 = false,
        isImage33 = false,
    }
end

local function combatTick(creature, guid)
    local st = combatStates[guid]
    if not st then
        return
    end

    -- Image splits (C++-exact arm order): a single tick into
    -- <=33% fires both yells. C++ !HealthAbovePct(66/33) —
    -- nethekurse <= precedent.
    if not st.isImage66 and creature:GetHealthPct() <= 66 then
        doSplit(creature)
        st.isImage66 = true
    end
    if not st.isImage33 and creature:GetHealthPct() <= 33 then
        doSplit(creature)
        st.isImage33 = true
    end

    -- Mind rend 36924: 3s init then 8s — target = random alive
    -- player excluding the current victim, nil pick falls
    -- back to DoCastVictim (C++-exact), non-triggered DoCast
    -- (C++-exact), re-arm 8000 (C++-exact).
    if st.mindRend <= 1000 then
        local target = randomNonVictimPlayer(creature)
        creature:CastSpell(target, SPELL_MIND_REND)
        st.mindRend = 8000
    else
        st.mindRend = st.mindRend - 1000
    end

    -- Fear 39415: 15s init then 25s — Talk(SAY_FEAR), same
    -- targeting (C++-exact), non-triggered, re-arm 25000
    -- (C++-exact). The C++ IsNonMeleeSpellCast(false)
    -- early-return gate has no cast-state bridge — the arm
    -- fires unconditionally (netherspite precedent).
    if st.fear <= 1000 then
        creature:Talk(SAY_FEAR)
        local target = randomNonVictimPlayer(creature)
        creature:CastSpell(target, SPELL_FEAR)
        st.fear = 25000
    else
        st.fear = st.fear - 1000
    end

    -- Domination 37162: 30s init then {16s,32s} — Talk(SAY_
    -- MIND), same targeting (C++-exact), non-triggered,
    -- re-arm {16s,32s} (C++-exact; same cast-state caveat).
    if st.domination <= 1000 then
        creature:Talk(SAY_MIND)
        local target = randomNonVictimPlayer(creature)
        creature:CastSpell(target, SPELL_DOMINATION)
        st.domination = 16000 + math.random(0, 15999)
    else
        st.domination = st.domination - 1000
    end
end

-- C++: the intro machine runs from spawn — UpdateAI ticks
-- it before the !UpdateVictim early return — so the intro
-- pump starts here (see fight-shape header comment).
RegisterCreatureEvent(ENTRY_SKYRISS, 5, function(_, creature)
    local guid = creature:GetGUID()
    fullReset(guid)
    introTimers[guid] = CreateLuaEvent(function()
        introTick(creature, guid)
    end, 1000, 0)
end)

-- C++ JustEngagedWith is empty — the combat timer schedule
-- lands on OnEnterCombat (omor precedent); the pull yell is
-- the intro phase-2 SAY_AGGRO, not an engage yell.
RegisterCreatureEvent(ENTRY_SKYRISS, 1, function(_, creature)
    local guid = creature:GetGUID()
    cancelCombatPump(guid)
    combatStates[guid] = freshCombatState()
    combatTimers[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ Reset() (called on evade): _Reset() is instance-
-- blocked; the observable remainder — the Initialize() re-
-- latch with the Intro flag untouched — is fullReset.
RegisterCreatureEvent(ENTRY_SKYRISS, 2, function(_, creature)
    local guid = creature:GetGUID()
    fullReset(guid)
    local intro = introStates[guid]
    if intro and not intro.done then
        introTimers[guid] = CreateLuaEvent(function()
            introTick(creature, guid)
        end, 1000, 0)
    end
end)

RegisterCreatureEvent(ENTRY_SKYRISS, 3, function(_, creature, victim)
    -- C++ KilledUnit: Talk(SAY_KILL) unless the victim is
    -- NPC_ALPHA_POD_TARGET (won't yell for pets/other units —
    -- vazruden entry-gate precedent, C++-exact).
    if victim and victim:GetEntry() ~= ENTRY_ALPHA_POD_TARGET then
        creature:Talk(SAY_KILL)
    end
end)

RegisterCreatureEvent(ENTRY_SKYRISS, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    fullReset(creature:GetGUID())
end)

-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() re-latch as the evade hook.
RegisterCreatureEvent(ENTRY_SKYRISS, 23, function(_, creature)
    local guid = creature:GetGUID()
    fullReset(guid)
    local intro = introStates[guid]
    if intro and not intro.done then
        introTimers[guid] = CreateLuaEvent(function()
            introTick(creature, guid)
        end, 1000, 0)
    end
end)

-- Gluth (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_gluth.cpp
-- (boss_gluth (BossAI, BOSS_GLUTH); spell_gluth_decimate +
-- spell_gluth_zombiechow_search (SpellScripts via SpellScriptLoader);
-- npc_zombie_chow (ScriptedAI); AddSC_boss_gluth registers all four;
-- loader decl 67 / call 262 per northrend_script_loader.cpp — the
-- NINTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_noth(), under the "// Naxxramas" marker; the call after
-- it is AddSC_boss_sapphiron() (decl 68 / call 263)).
-- Entry: 15932 Gluth (naxxramas.h NPC_GLUTH — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_GLUTH -> GluthGUID and DATA_GLUTH
-- -> GluthGUID; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the four
-- script names hits boss_gluth.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No gluth lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent),
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random (amanitar
-- / boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gluth: EVENT_WOUND — DoCastVictim(Mortal Wound 54378), 10s
-- init, 10s repeat (moroes precedent); the STATE_GLUTH_EATING
-- deferral arm (events.Repeat(3s)) is PORTED (Lua state machine,
-- below). EVENT_ENRAGE — Talk(EMOTE_ENRAGE 2) +
-- DoCastSelf(Enrage 28371), randtime(16s,22s) init, randtime(16s,22s)
-- repeat (phase_hunter self-cast precedent); the eating-state 5s
-- deferral arm is PORTED (Lua state machine, below).
-- EVENT_DECIMATE — Talk(EMOTE_DECIMATE 1) + DoCastAOE(Decimate
-- 28374), randtime(110s,120s) init, re-scheduled randtime(110s,120s)
-- (C++ re-arms via ScheduleEvent, not Repeat — same interval);
-- DoCastAOE lowers to self-cast creature:CastSpell(creature, spell)
-- (boss_ai.go DoCastAOE self-cast convention — selin_fireheart /
-- mr_smite precedent; the old "no DoCastAOE bridge" claim was
-- wrong). The 20x EVENT_SEARCH_ZOMBIE_MULTI scheduling leg and the
-- eating-state 4s deferral are PORTED (Lua state machine + unique
-- one-shot keys, below). The decimate damage rider lives in
-- spell_gluth_decimate (PORTED below).
-- EVENT_BERSERK — Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat. JustEngagedWith schedules the four
-- ported events (BossAI::JustEngagedWith instance-bookkeeping leg has
-- no bridge — tharon_ja precedent); no aggro Talk in C++.
-- EMOTE_SPOTS_ONE (0) rides the ported single-search tick and
-- EMOTE_DEVOURS_ALL (3) the ported multi-search tick; EMOTE_DECIMATE
-- (1) rides the ported decimate tick. No KilledUnit Talk, no JustDied
-- Talk in C++ for gluth. All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- Zombie-eat machine — PORTED 2026-11-23 (strip 26): the Lua
-- summons list is the ev19/ev21-tracked ordered table (marli
-- precedent; ev19 fires on the group-summon path) — the old
-- "blocked on summon-list / cross-AI / react-state bridges" claims
-- are stale: creature:SetData (ev41 fire),
-- creature:DoAction (ev38), creature:SetReactState, AttackStop,
-- MoveCloserAndStop, MoveIdle and the speed legs are all bridged.
-- EVENT_SEARCH_ZOMBIE_SINGLE (12s init, RAID_MODE 7s/5s repeat),
-- EVENT_KILL_ZOMBIE_SINGLE (MovementInform id 1 -> 1s), the 20x
-- EVENT_SEARCH_ZOMBIE_MULTI series, the DoAction(ACTION_DECIMATE_EVENT)
-- receiver (SetData DECIMATED over tracked zombies), the
-- STATE_GLUTH_EATING deferrals (wound 3s / enrage 5s / decimate 4s),
-- and Reset (_Reset summons.DespawnAll twin via gluthWipe) are all
-- ported. Reset — _Reset + SetReactState(REACT_AGGRESSIVE) + SetSpeed(
-- MOVE_RUN, 12.0f) — the speed leg is ported via creature:SetSpeedRate
-- as the rate 12.0/7.0 (baseMoveSpeed[MOVE_RUN] = 7.0 yd/s, Unit.cpp);
-- SummonedCreatureDies — summons.Despawn (single, of an already-dead
-- zombie) drops the dead zombie from the tracked list.
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_SUMMON — PORTED below (summonTick): Is25ManRaid() ->
-- SummonCreatureGroup(SUMMON_GROUP_CHOW_25MAN 2) else
-- SummonCreatureGroup(SUMMON_GROUP_CHOW_10MAN 1)
-- (boss_gluth.cpp:185-191), 15s init, 10s repeat; the :139 pre-loop
-- HasUnitState(CASTING) gate covers EVENT_SUMMON too (500ms IsCasting
-- defer ported, mal_ganis idiom). Group coords DB-side;
-- creature:SummonCreatureGroup bridged
-- 2026-11-05 (tick 238). (The old "Is25ManRaid difficulty gate"
-- wording is stale — the branch is ported above.)
-- spell_gluth_decimate (28374/54426): HandleScriptEffect -> Decimate
-- damage 28375 via CastCustomSpell BP0 + HandleEvent -> DoAction(
-- ACTION_DECIMATE_EVENT) — PORTED below (ev5 effectIndex 0 +
-- ev4 effectIndex 2 + ev38 receiver, the "no SpellScript binding
-- bridge (razelikh precedent)" header claim is stale —
-- RegisterSpellEvent is live); the DoAction receiver body is PORTED
-- (strip 26 — SetData DECIMATED over the tracked zombies).
-- spell_gluth_zombiechow_search (28239/28404): AfterHit -> 5%-max-
-- health heal — PORTED below (ev3); LIVE now — both cast sites ride
-- the ported zombie-eat machine (single/multi search ticks).
-- npc_zombie_chow (ScriptedAI — entries 16360/30303 appear only in a
-- C++ comment; no NPC_ constants in naxxramas.h, no instance
-- bindings) — PORTED in lua_scripts/northrend/npc_zombie_chow.lua
-- (strip 26): ctor self-cast, ev41 SetData receiver (DECIMATED walk
-- leg + TOBE_EATEN stop/immune legs), ev7 decimated walk tick. The
-- DATA_GLUTH GUID leg rides the FindNearestCreature entry-scan twin
-- (no instance-creature registry in Go).

local ENTRY_GLUTH = 15932

local EMOTE_ENRAGE = 2
local EMOTE_DECIMATE = 1
local EMOTE_BERSERKER = 4

local SPELL_MORTAL_WOUND = 54378
local SPELL_ENRAGE = 28371
local SPELL_DECIMATE = 28374
local SPELL_DECIMATE_HEROIC = 54426 -- boss_gluth.cpp:40 (25-man twin)
local SPELL_DECIMATE_DMG = 28375
local SPELL_ZOMBIE_CHOW_SINGLE = 28239
local SPELL_ZOMBIE_CHOW_MULTI = 28404 -- boss_gluth.cpp:44
local ACTION_DECIMATE_EVENT = 2 -- boss_gluth.cpp:89
local SPELL_BERSERK = 26662

local timers = {}

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

local STATE_GLUTH_NORMAL = 1
local STATE_GLUTH_EATING = 2
local STATE_ZOMBIE_DECIMATED = 2
local STATE_ZOMBIE_TOBE_EATEN = 3
local DATA_ZOMBIE_STATE = 1
local EMOTE_SPOTS_ONE = 0
local EMOTE_DEVOURS_ALL = 3
local ENTRY_ZOMBIE_CHOW = 16360
local ENTRY_ZOMBIE_CHOW_HEROIC = 30303

-- Zombie-eat machine state, per Gluth GUID (boss_gluth.cpp:192-261):
-- state mirrors STATE_GLUTH_NORMAL/EATING, zombieGUID the saved
-- to-be-eaten GUID, zombies the ordered summon list (C++ SummonList
-- iteration order — first alive zombie in range wins), decimateCasts
-- the unique-key counter for the 20x multi one-shots.
local gluthState = {}

local function gluthStateFor(guid)
    local st = gluthState[guid]
    if not st then
        st = { state = STATE_GLUTH_NORMAL, zombieGUID = 0, zombies = {}, decimateCasts = 0 }
        gluthState[guid] = st
    end
    return st
end

local function removeTrackedZombie(st, guid)
    for i, z in ipairs(st.zombies) do
        if z.guid == guid then
            table.remove(st.zombies, i)
            return
        end
    end
end

-- C++ BossAI::_Reset (ScriptedCreature.cpp:499-516) despawns all
-- summons on evade/reset, never on death (marli precedent): despawn
-- the tracked zombies when alive, always drop the state table.
local function gluthWipe(creature, guid, despawn)
    cancelTimers(guid)
    local st = gluthState[guid]
    if st ~= nil then
        if despawn and creature:IsAlive() then
            for _, z in ipairs(st.zombies) do
                z.obj:Despawn()
            end
        end
        gluthState[guid] = nil
    end
end

-- CASTING gates PORTED (500ms IsCasting defer, mal_ganis idiom):
-- boss_gluthAI::UpdateAI pre-loop :139 + in-loop :264 gates cover
-- wound/enrage/decimate/berserk; the defer re-queues without
-- consuming the repeat.
-- C++ EVENT_WOUND: DoCastVictim(Mortal Wound 54378), 10s init, 10s
-- repeat.
local function woundTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wound", 500, function()
            woundTick(creature, guid)
        end)
        return
    end
    if gluthStateFor(guid).state == STATE_GLUTH_EATING then
        schedule(guid, "wound", 3000, function()
            woundTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MORTAL_WOUND)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: Talk(EMOTE_ENRAGE 2) + DoCastSelf(Enrage 28371),
-- randtime(16s,22s) init, randtime(16s,22s) repeat.
local function enrageTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
    if gluthStateFor(guid).state == STATE_GLUTH_EATING then
        schedule(guid, "enrage", 5000, function()
            enrageTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_ENRAGE)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", {16000, 22000}, function()
        enrageTick(creature, guid)
    end)
end

-- C++ EVENT_SEARCH_ZOMBIE_MULTI (boss_gluth.cpp:242-261): skipped
-- while EATING; first alive tracked zombie within 18.0yd 2D casts
-- the 28404 AoE (Talk EMOTE_DEVOURS_ALL). One-shot — the 20x 3s*i
-- series rides the decimate tick.
local function searchZombieMultiTick(creature, guid)
    local st = gluthStateFor(guid)
    if st.state == STATE_GLUTH_EATING then
        return
    end
    for _, z in ipairs(st.zombies) do
        if z.obj:IsAlive() and creature:GetDistance2d(z.obj) <= 18.0 then
            creature:Talk(EMOTE_DEVOURS_ALL)
            creature:CastSpell(creature, SPELL_ZOMBIE_CHOW_MULTI)
            return
        end
    end
end
-- C++ EVENT_DECIMATE: Talk(EMOTE_DECIMATE 1) + DoCastAOE(Decimate
-- 28374) as self-cast, randtime(110s,120s) init, re-scheduled
-- randtime(110s,120s) (C++ ScheduleEvent re-arm). The 25-man twin
-- 54426 rides the Is25ManRaid branch (C++ DoCastAOE maps via
-- GetSpellIdForDifficulty, which has no Go model — boss_ai.go:12751;
-- summonTick Is25ManRaid precedent below). The eating-state 4s
-- deferral and the 20x SEARCH_ZOMBIE_MULTI legs are PORTED (strip 26).
local function decimateTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "decimate", 500, function()
            decimateTick(creature, guid)
        end)
        return
    end
    local st = gluthStateFor(guid)
    if st.state == STATE_GLUTH_EATING then
        schedule(guid, "decimate", 4000, function()
            decimateTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_DECIMATE)
    if creature:Is25ManRaid() then
        creature:CastSpell(creature, SPELL_DECIMATE_HEROIC)
    else
        creature:CastSpell(creature, SPELL_DECIMATE)
    end
    -- C++ EVENT_DECIMATE (boss_gluth.cpp:175-178): 20x
    -- EVENT_SEARCH_ZOMBIE_MULTI, one shot each at 3s*i. Unique keys
    -- per cast so a re-cast does not cancel the previous series
    -- (C++ ScheduleEvent adds; the schedule helper replaces).
    st.decimateCasts = st.decimateCasts + 1
    local cast = st.decimateCasts
    for i = 1, 20 do
        schedule(guid, "multi" .. cast .. "_" .. i, 3000 * i, function()
            searchZombieMultiTick(creature, guid)
        end)
    end
    schedule(guid, "decimate", {110000, 120000}, function()
        decimateTick(creature, guid)
    end)
end

-- C++ EVENT_SEARCH_ZOMBIE_SINGLE (boss_gluth.cpp:192-225): first
-- alive tracked zombie within 10.0yd (IsWithinDistInMap twin:
-- GetDistance minus both combat reaches, halazzi precedent) is
-- marked to-be-eaten via cross-AI SetData (ev41 on the zombie),
-- Gluth speeds to 36.0, goes REACT_PASSIVE, stops attacking, Talks
-- EMOTE_SPOTS_ONE, and walks to 2.0yd of it (MoveCloserAndStop id 1);
-- state EATING. Repeat RAID_MODE(7s, 5s) — 10-man 7s, 25-man 5s.
local function searchZombieSingleTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "searchSingle", 500, function()
            searchZombieSingleTick(creature, guid)
        end)
        return
    end
    local st = gluthStateFor(guid)
    local target = nil
    for _, z in ipairs(st.zombies) do
        if z.obj:IsAlive() and creature:GetDistance(z.obj)
                - creature:GetCombatReach() - z.obj:GetCombatReach() < 10.0 then
            target = z.obj
            break
        end
    end
    if target then
        st.zombieGUID = target:GetGUIDLow()
        target:SetData(DATA_ZOMBIE_STATE, STATE_ZOMBIE_TOBE_EATEN)
        creature:SetSpeedRate(1, 36.0 / 7.0)
        creature:SetReactState(0)
        creature:AttackStop()
        creature:Talk(EMOTE_SPOTS_ONE)
        creature:MoveCloserAndStop(1, target, 2.0)
        st.state = STATE_GLUTH_EATING
    end
    local repeatMs = 7000
    if creature:Is25ManRaid() then
        repeatMs = 5000
    end
    schedule(guid, "searchSingle", repeatMs, function()
        searchZombieSingleTick(creature, guid)
    end)
end

-- C++ EVENT_KILL_ZOMBIE_SINGLE (boss_gluth.cpp:227-240): 1s after
-- the MoveCloserAndStop arrival (MovementInform id 1, below), cast
-- 28239 on the saved zombie if it is alive within 10.0yd; then
-- always reset: state NORMAL, speed 12.0, REACT_AGGRESSIVE.
local function killZombieSingleTick(creature, guid)
    local st = gluthStateFor(guid)
    local zombie = nil
    if st.zombieGUID ~= 0 then
        zombie = creature:GetCreatureByGUID(st.zombieGUID)
    end
    if zombie ~= nil and zombie:IsAlive() and creature:GetDistance(zombie)
            - creature:GetCombatReach() - zombie:GetCombatReach() < 10.0 then
        creature:CastSpell(zombie, SPELL_ZOMBIE_CHOW_SINGLE)
    end
    st.zombieGUID = 0
    st.state = STATE_GLUTH_NORMAL
    creature:SetSpeedRate(1, 12.0 / 7.0)
    creature:SetReactState(2)
end


-- C++ EVENT_SUMMON: Is25ManRaid() -> SummonCreatureGroup(
-- SUMMON_GROUP_CHOW_25MAN 2) else SummonCreatureGroup(
-- SUMMON_GROUP_CHOW_10MAN 1) (boss_gluth.cpp:185-191), 15s init,
-- 10s repeat (events.Repeat). The :139 pre-loop HasUnitState(CASTING)
-- gate covers EVENT_SUMMON too — 500ms IsCasting defer (mal_ganis idiom).
local function summonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon", 500, function()
            summonTick(creature, guid)
        end)
        return
    end
    if creature:Is25ManRaid() then
        creature:SummonCreatureGroup(2)
    else
        creature:SummonCreatureGroup(1)
    end
    schedule(guid, "summon", 10000, function()
        summonTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat (hard-enrage buff refresh).
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_BERSERKER)
    creature:CastSpell(creature, SPELL_BERSERK)
    schedule(guid, "berserk", 300000, function()
        berserkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: schedules EVENT_WOUND (10s), EVENT_ENRAGE
-- (randtime 16s,22s), EVENT_DECIMATE (randtime 110s,120s),
-- EVENT_BERSERK (8min). No aggro Talk in C++.
local BOSS_GLUTH = 8 -- naxxramas.h:38 (NAXEncounter)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local function gluthEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_GLUTH, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537), base-first (faerlina precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local st = gluthStateFor(guid)
    st.state = STATE_GLUTH_NORMAL
    st.zombieGUID = 0
    st.zombies = {}
    st.decimateCasts = 0
    schedule(guid, "decimate", {110000, 120000}, function()
        decimateTick(creature, guid)
    end)
    schedule(guid, "searchSingle", 12000, function()
        searchZombieSingleTick(creature, guid)
    end)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
    schedule(guid, "enrage", {16000, 22000}, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "berserk", 480000, function()
        berserkTick(creature, guid)
    end)
    schedule(guid, "summon", 15000, function()
        summonTick(creature, guid)
    end)
end

local function gluthLeaveCombat(event, creature)
    gluthWipe(creature, creature:GetGUIDLow(), true)
end

local function gluthDied(event, creature, killer)
    creature:SetBossState(BOSS_GLUTH, BOSS_STATE_DONE) -- BossAI::_JustDied (ScriptedCreature.cpp:519)
    gluthWipe(creature, creature:GetGUIDLow(), false)
end

local function gluthReset(event, creature)
    gluthWipe(creature, creature:GetGUIDLow(), true)
    if creature:GetBossState(BOSS_GLUTH) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_GLUTH, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
    end
    -- C++ boss_gluthAI::Reset (boss_gluth.cpp:107-114): _Reset() +
    -- zombie GUID clear + state NORMAL + SetReactState(REACT_AGGRESSIVE)
    -- (:112) + SetSpeed 12.0f below. The _Reset summons.DespawnAll leg
    -- rides gluthWipe (ev23, above); the react leg restores AGGRESSIVE
    -- after the eating-state PASSIVE (strip 26 machine).
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    -- C++ Reset() :113: SetSpeed(MOVE_RUN, 12.0f) — absolute form; the
    -- rate is 12.0 / baseMoveSpeed[MOVE_RUN] (7.0 yd/s, Unit.cpp:8816-8819).
    -- The decimate 36.0f leg stays out: it fires only inside the eating
    -- state behind the unported zombie machine (documented above).
    creature:SetSpeedRate(1, 12.0 / 7.0)
end

-- C++ boss_gluthAI::SummonedCreatureDies (boss_gluth.cpp:127-130):
-- summons.Despawn(summoned) — drop the dead zombie from the tracked
-- list (strip 26); dead zombies despawn via the engine death path.
local function gluthSummonedCreatureDied(event, creature, summon, killer)
    local st = gluthState[creature:GetGUIDLow()]
    if st ~= nil then
        removeTrackedZombie(st, summon:GetGUIDLow())
    end
end

-- C++ boss_gluthAI::JustSummoned (BossAI::JustSummoned, summons.Summon):
-- track every zombie chow the groups spawn, in summon order (the C++
-- SummonList iteration order). ev19 fires on the group-summon path
-- (lua_creature_events.go summonTempCreature).
local function gluthJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry ~= ENTRY_ZOMBIE_CHOW and entry ~= ENTRY_ZOMBIE_CHOW_HEROIC then
        return
    end
    local st = gluthStateFor(creature:GetGUIDLow())
    table.insert(st.zombies, { guid = summoned:GetGUIDLow(), obj = summoned })
end

-- C++ boss_gluthAI::MovementInform (boss_gluth.cpp:271-277): id 1
-- (the MoveCloserAndStop point) -> MoveIdle + KILL_ZOMBIE_SINGLE
-- in 1s. No type filter in C++ — mirrored exactly.
local function gluthMovementInform(event, creature, moveType, pointId)
    if pointId ~= 1 then
        return
    end
    creature:MoveIdle()
    local guid = creature:GetGUIDLow()
    schedule(guid, "killSingle", 1000, function()
        killZombieSingleTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_GLUTH, 1, gluthEnterCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 2, gluthLeaveCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 4, gluthDied)
RegisterCreatureEvent(ENTRY_GLUTH, 19, gluthJustSummoned)
RegisterCreatureEvent(ENTRY_GLUTH, 21, gluthSummonedCreatureDied)
RegisterCreatureEvent(ENTRY_GLUTH, 23, gluthReset)
RegisterCreatureEvent(ENTRY_GLUTH, 28, gluthMovementInform)

-- C++ spell_gluth_decimate::HandleScriptEffect (boss_gluth.cpp:313-326):
-- Decimate (28374 / 25-man 54426) hitting a unit deals (health - 5%
-- max health) as 28375 triggered with BP0, when positive. ev5 fires
-- once per (effect, hit target) from the creature cast path
-- (boss_ai.go:3435), effectIndex == 0 is the EFFECT_0 bind; the
-- conditions-table rows on effect 0 (boss_gluth.cpp:40) are DB-side
-- and unevaluated in Go — flagged, not a verdict-changer.
local function decimateDamageHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    local damage = hitTarget:GetHealth() - hitTarget:CountPctFromMaxHealth(5)
    if damage > 0 then
        caster:CastCustomSpell(hitTarget, SPELL_DECIMATE_DMG, true, damage)
    end
end

-- C++ spell_gluth_decimate::HandleEvent (boss_gluth.cpp:328-331): the
-- SEND_EVENT effect fires DoAction(ACTION_DECIMATE_EVENT) on the boss
-- AI once per cast (ev4, boss_ai.go:3373 — once per non-zero effect,
-- not per target). The receiver (boss_gluth.cpp:280-292) sets every
-- summoned zombie DECIMATED — PORTED (strip 26, gluthDoAction).
local function decimateEventHit(event, spell, effectIndex)
    if effectIndex ~= 2 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    caster:DoAction(ACTION_DECIMATE_EVENT)
end

local function gluthDoAction(event, creature, action)
    if action ~= ACTION_DECIMATE_EVENT then
        return
    end
    local st = gluthState[creature:GetGUIDLow()]
    if st == nil then
        return
    end
    for _, z in ipairs(st.zombies) do
        if z.obj:IsAlive() then
            z.obj:SetData(DATA_ZOMBIE_STATE, STATE_ZOMBIE_DECIMATED)
        end
    end
end

-- C++ spell_gluth_zombiechow_search::HealForEachTargetHit
-- (boss_gluth.cpp:368-371): AfterHit heals Gluth 5% max health per
-- unit hit (ev3 fires once per hit target, boss_ai.go:4215-4228).
-- 28239 single-target / 28404 multi (boss_gluth.cpp:44; census row
-- 386 is the 28404 entry). LIVE: both cast sites ride the ported
-- zombie-eat machine (EVENT_KILL_ZOMBIE_SINGLE :231,
-- EVENT_SEARCH_ZOMBIE_MULTI :258).
local function zombieChowHeal(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetEntry() ~= ENTRY_GLUTH then
        return
    end
    caster:SetHealth(caster:GetHealth() + caster:CountPctFromMaxHealth(5))
end

RegisterSpellEvent(SPELL_DECIMATE, 5, decimateDamageHit)
RegisterSpellEvent(SPELL_DECIMATE_HEROIC, 5, decimateDamageHit)
RegisterSpellEvent(SPELL_DECIMATE, 4, decimateEventHit)
RegisterSpellEvent(SPELL_DECIMATE_HEROIC, 4, decimateEventHit)
RegisterSpellEvent(SPELL_ZOMBIE_CHOW_SINGLE, 3, zombieChowHeal)
RegisterSpellEvent(SPELL_ZOMBIE_CHOW_MULTI, 3, zombieChowHeal)
RegisterCreatureEvent(ENTRY_GLUTH, 38, gluthDoAction)

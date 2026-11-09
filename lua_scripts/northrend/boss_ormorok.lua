-- Ormorok the Tree-Shaper (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_ormorok.cpp
-- (boss_ormorok (CreatureScript) via GetNexusAI<boss_ormorokAI>
-- (BossAI, DATA_ORMOROK); npc_crystal_spike_trigger
-- (CreatureScript); spell_crystal_spike (SpellScriptLoader);
-- AddSC_boss_ormorok() at end registers all three; loader decl 80 /
-- call 275 per northrend_script_loader.cpp — the FOURTH group of the
-- "// The Nexus: Nexus" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_anomalus() (decl 79 / call 274); the call after it
-- is AddSC_boss_keristrasza() (decl 81 / call 276).
-- Entry: 26794 Ormorok the Tree-Shaper (wowhead-verified 2026-10-25:
-- https://www.wowhead.com/npc=26794/ormorok-the-tree-shaper — no
-- NPC_ORMOROK const in the C++ tree, no instance_nexus.cpp
-- OnCreatureCreate case, same entry-block class as telestra).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent (maiden_of_virtue precedent), cancelled on 2/4/23;
-- melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- GUID-KEY FIX tick 511: timers + ormorokFrenzy + spikeDespawnTimers
-- keyed on GetGUIDLow() (stable uint32) — GetGUID() userdata keys
-- never match across calls, so the frenzy latch never read true
-- (re-fired on every hit below 25%) and leave/died/reset cancels
-- previously missed (tick 508 hadronox precedent).
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 1) + schedules EVENT_CRYSTAL_
-- SPIKES (12s) + EVENT_TRAMPLE (10s) + EVENT_SPELL_REFLECTION
-- (30s) + heroic EVENT_CRYSTALLINE_TANGLER (15s, IsHeroic
-- bridged — lavanthor precedent); the BossAI::JustEngagedWith
-- base arms are PORTED in ormorokEnterCombat (SetBossState +
-- pulse + me-form DoZone); setActive/ScheduleTasks legs out.
-- EVENT_CRYSTAL_SPIKES — Talk(SAY_CRYSTAL_SPIKES 4) + DoCastVictim
-- (SPELL_CRYSTAL_SPIKES 47958, 1-arg DoCast = victim cast) as
-- creature:CastSpell(nil, spell), 12s init / 12s repeat.
-- EVENT_TRAMPLE — DoCast(me, SPELL_TRAMPLE 48016) self-cast, 10s
-- init / 10s repeat.
-- EVENT_SPELL_REFLECTION — Talk(SAY_REFLECT 3) + DoCast(me, SPELL_
-- SPELL_REFLECTION 47981) self-cast, 30s init / 30s repeat.
-- EVENT_CRYSTALLINE_TANGLER (heroic only) — SelectTarget(Random, 0,
-- OrmorokTanglerPredicate) -> DoCast(target, SPELL_SUMMON_
-- CRYSTALLINE_TANGLER 61564): the predicate overload draws the
-- threat list with no playerOnly filter (UnitAI.h:176), so the pool
-- is GetAITargets + alive (C++ skips offline refs) + GetDistance2d
-- >= 5.0; 15s init / 15s unconditional re-arm.
-- DamageTaken — HealthBelowPct(25) one-shot latch -> Talk(SAY_FRENZY
-- 6) + DoCast(me, SPELL_FRENZY 48017) self-cast. NOTE the latch
-- reads CURRENT health, not post-damage: C++ uses HealthBelowPct,
-- not HealthBelowPctDamaged, and DamageTaken fires pre-application
-- (Unit.cpp:697-702), so the Lua twin is GetHealth() < floor(max *
-- 25 / 100) with NO damage subtraction (keristrasza's Damaged form
-- is the contrasting case). The latch therefore fires on the first
-- hit taken while already below 25%, one hit after the crossing —
-- that is the C++ behavior, preserved exactly.
-- JustDied — Talk(SAY_DEATH 2) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
-- KilledUnit — Talk(SAY_KILL 5) player-gated (nalorakk / kelthuzad
-- variant).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — BossAI::Reset + Initialize() (frenzy=false) — the latch
-- reset is ported on 2/4/23; the BossAI::_Reset SetBossState
-- (NOT_STARTED, != DONE-gated) leg PORTED 2026-11-05
-- (creature:SetBossState bridged, SetBossState census tick 249).
-- CASTING gates PORTED 2026-11-04 (500ms IsCasting defer, mal_ganis
-- idiom): own UpdateAI pre-loop :129 + in-loop :159 gates cover all
-- 4 timer arms (spikes/trample/reflection/tangler — defer precedes
-- Talk/cast/target pick; the event stays queued, no re-arm
-- consumed). DamageTaken frenzy latch ungated per C++ (separate
-- hook, no CASTING check).
-- npc_crystal_spike_trigger — IsSummonedBy entry switch (initial
-- 27101: SetFacingToObject + aura 47941; trigger 27079: aura +
-- GetData(DATA_COUNT)+1 from the owner) + FindNearestGameObject(GO_
-- CRYSTAL_SPIKE_TRAP 188537, 1.0f) Delete + 2s DespawnOrUnsummon
-- — PORTED 2026-11-08 (tick 426, below; ev22 IsSummonedBy,
-- SetFacingToObject, cross-AI GetData, FindNearestGameObject,
-- GO Delete and DespawnOrUnsummon all bridged). The GO Use leg
-- stays out: no generic GameObject::Use bridge, and the trap
-- spell id is DB template data with no 188537 row in either tree.
-- spell_crystal_spike — AuraScript periodic: entry-gated summon
-- spells (47936 / urand 47942/47943) while GetData(DATA_COUNT) <
-- MAX_COUNT 5 — PORTED 2026-11-08 (tick 425, below; dormant, the
-- npc_crystal_spike_trigger IsSummonedBy sender is unported, and
-- the < 5 guard reads 0 until that trigger port latches DATA_COUNT).

local ENTRY_ORMOROK = 26794

local DATA_ORMOROK = 3 -- nexus.h:33 (NEXDataTypes)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local SAY_AGGRO = 1
local SAY_DEATH = 2
local SAY_REFLECT = 3
local SAY_CRYSTAL_SPIKES = 4
local SAY_KILL = 5
local SAY_FRENZY = 6

local SPELL_SPELL_REFLECTION = 47981
local SPELL_TRAMPLE = 48016
local SPELL_FRENZY = 48017
local SPELL_SUMMON_CRYSTALLINE_TANGLER = 61564
local SPELL_CRYSTAL_SPIKES = 47958

local GO_ORMOROKS_CONTAINMENT_SPHERE = 188528 -- nexus.h:64
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_SEARCH_DIST = 1000 -- whole-instance radius; entries are unique per GO

local timers = {}
local ormorokFrenzy = {}

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

-- C++ EVENT_CRYSTAL_SPIKES (boss_ormorok.cpp:165-169): Talk(SAY_
-- CRYSTAL_SPIKES 4) + DoCast(SPELL_CRYSTAL_SPIKES 47958) 1-arg =
-- victim cast, 12s init / 12s repeat.
local function crystalSpikesTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spikes", 500, function()
            crystalSpikesTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_CRYSTAL_SPIKES)
    creature:CastSpell(nil, SPELL_CRYSTAL_SPIKES)
    schedule(guid, "spikes", 12000, function()
        crystalSpikesTick(creature, guid)
    end)
end

-- C++ EVENT_TRAMPLE (:160-163): DoCast(me, SPELL_TRAMPLE 48016)
-- self-cast, 10s init / 10s repeat.
local function trampleTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "trample", 500, function()
            trampleTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TRAMPLE)
    schedule(guid, "trample", 10000, function()
        trampleTick(creature, guid)
    end)
end

-- C++ EVENT_SPELL_REFLECTION (:170-174): Talk(SAY_REFLECT 3) +
-- DoCast(me, SPELL_SPELL_REFLECTION 47981) self-cast, 30s init /
-- 30s repeat.
local function spellReflectionTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "reflection", 500, function()
            spellReflectionTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_REFLECT)
    creature:CastSpell(creature, SPELL_SPELL_REFLECTION)
    schedule(guid, "reflection", 30000, function()
        spellReflectionTick(creature, guid)
    end)
end

-- C++ EVENT_CRYSTALLINE_TANGLER (:175-179): SelectTarget(Random, 0,
-- OrmorokTanglerPredicate) -> DoCast(target, SPELL_SUMMON_
-- CRYSTALLINE_TANGLER 61564); predicate = GetDistance2d >= 5.0f
-- (both reaches, Object.cpp:1187; branch idiom),
-- threat-list pool, no playerOnly filter (UnitAI.h:176); re-arm
-- 15s unconditional.
local function tanglerTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "tangler", 500, function()
            tanglerTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if t:IsAlive() and creature:GetDistance2d(t) - creature:GetCombatReach() - targetReach >= 5.0 then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)],
            SPELL_SUMMON_CRYSTALLINE_TANGLER)
    end
    schedule(guid, "tangler", 15000, function()
        tanglerTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:105-118): schedules + Talk(SAY_AGGRO 1);
-- the BossAI::JustEngagedWith SetBossState(IN_PROGRESS) leg
-- PORTED 2026-11-05 (base-first, ScriptedCreature.cpp:537).
local function ormorokEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
    -- C++ JustEngagedWith base call first (boss_ormorok.cpp:89):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:537-542) --
    -- SetBossState(IN_PROGRESS) + SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat, before Talk and the timer schedules (C++
    -- arm order); setActive/ScheduleTasks legs out.
    creature:SetBossState(DATA_ORMOROK, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "spikes", 12000, function()
        crystalSpikesTick(creature, guid)
    end)
    schedule(guid, "trample", 10000, function()
        trampleTick(creature, guid)
    end)
    schedule(guid, "reflection", 30000, function()
        spellReflectionTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "tangler", 15000, function()
            tanglerTick(creature, guid)
        end)
    end
end

local function ormorokLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
end

-- C++ DamageTaken (:120-128): if (!frenzy && HealthBelowPct(25)) —
-- CURRENT-health form, see header note — Talk(SAY_FRENZY 6) +
-- DoCast(me, SPELL_FRENZY 48017) self-cast, one-shot latch.
local function ormorokDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if ormorokFrenzy[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() < math.floor(maxHealth * 25 / 100) then
        ormorokFrenzy[guid] = true
        creature:Talk(SAY_FRENZY)
        creature:CastSpell(creature, SPELL_FRENZY)
    end
end

-- C++ JustDied: _JustDied() first, then Talk(SAY_DEATH 2)
-- (boss_ormorok.cpp:110-114); the _JustDied SetBossState(DONE)
-- leg PORTED 2026-11-05 (ScriptedCreature.cpp:519).
local function ormorokDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
    creature:SetBossState(DATA_ORMOROK, BOSS_STATE_DONE)
    -- C++ SetBossState override (instance_nexus.cpp:151-159): Ormorok
    -- DONE frees his containment sphere (RemoveFlag NOT_SELECTABLE).
    -- No boss-state-change event exists, so the leg rides the DONE
    -- write here (palehoof precedent); the instanceData flag feeds
    -- the instance OnGameObjectCreate / OnPlayerEnter applies.
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.ormorokDone = true
    end
    local sphere = creature:FindNearestGameObject(GO_ORMOROKS_CONTAINMENT_SPHERE, GO_SEARCH_DIST)
    if sphere then
        sphere:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: Talk(SAY_KILL 5) only when who->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function ormorokTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function ormorokReset(event, creature)
    -- C++ Reset (boss_ormorok.cpp:81-85): BossAI::Reset() first
    -- (events/scheduler cancel + _Reset SetBossState NOT_STARTED,
    -- != DONE-gated), then Initialize() (frenzy=false).
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:GetBossState(DATA_ORMOROK) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_ORMOROK, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
    end
    ormorokFrenzy[guid] = false
end

RegisterCreatureEvent(ENTRY_ORMOROK, 1, ormorokEnterCombat)
RegisterCreatureEvent(ENTRY_ORMOROK, 2, ormorokLeaveCombat)
RegisterCreatureEvent(ENTRY_ORMOROK, 3, ormorokTargetDied)
RegisterCreatureEvent(ENTRY_ORMOROK, 4, ormorokDied)
RegisterCreatureEvent(ENTRY_ORMOROK, 9, ormorokDamageTaken)
RegisterCreatureEvent(ENTRY_ORMOROK, 23, ormorokReset)

-- C++ spell_crystal_spike_AuraScript (boss_ormorok.cpp:273-294):
-- OnEffectPeriodic, EFFECT_0, SPELL_AURA_PERIODIC_DUMMY. Entry-gated
-- (INITIAL 27101 / TRIGGER 27079): initial casts crystalSpikeSummon[0]
-- 47936, trigger casts crystalSpikeSummon[urand(0, 2)] (47936/47942/
-- 47943), triggered, self-cast, only while AI GetData(DATA_COUNT
-- 1) < MAX_COUNT 5. No PreventDefaultAction in C++, so the handler
-- returns nothing. PORTED 2026-11-08 (tick 425). DORMANT: 47941 is
-- applied only by npc_crystal_spike_trigger IsSummonedBy
-- (boss_ormorok.cpp:211-235), which is unported, so the hook cannot
-- fire yet.
-- SETDATA CO-REQUISITE — LANDED 2026-11-08 (tick 426, trigger
-- port below): the C++ file contains ZERO SetData calls; _count is
-- a private AI member assigned in IsSummonedBy (initial 0; trigger
-- = owner trigger's GetData(DATA_COUNT) + 1) and read via GetData
-- (boss_ormorok.cpp:237-240). The Lua twin is
-- creature:SetData(DATA_COUNT, n) from the trigger's IsSummonedBy
-- (ev22); creature GetData reads 0 for an absent key
-- (lua_creature_events.go:809-822), so an unlatched spike reads 0
-- and the < 5 guard passes until the chain terminates at 5.
local SPELL_CRYSTAL_SPIKE_AURA = 47941 -- boss_ormorok.cpp:186
local NPC_CRYSTAL_SPIKE_INITIAL = 27101 -- boss_ormorok.cpp:179
local NPC_CRYSTAL_SPIKE_TRIGGER = 27079 -- boss_ormorok.cpp:180
local DATA_COUNT = 1 -- boss_ormorok.cpp:182
local MAX_COUNT = 5 -- boss_ormorok.cpp:183
local CRYSTAL_SPIKE_SUMMON = {47936, 47942, 47943} -- boss_ormorok.cpp:191-196

local function crystalSpikePeriodic(event, aura, target)
    local entry = target:GetEntry()
    if entry ~= NPC_CRYSTAL_SPIKE_INITIAL and entry ~= NPC_CRYSTAL_SPIKE_TRIGGER then
        return
    end
    local spell = CRYSTAL_SPIKE_SUMMON[1]
    if entry == NPC_CRYSTAL_SPIKE_TRIGGER then
        spell = CRYSTAL_SPIKE_SUMMON[math.random(3)]
    end
    if target:GetData(DATA_COUNT) < MAX_COUNT then
        target:CastSpell(target, spell, true)
    end
end

RegisterAuraEvent(SPELL_CRYSTAL_SPIKE_AURA, 1, crystalSpikePeriodic)

-- C++ npc_crystal_spike_triggerAI::IsSummonedBy
-- (boss_ormorok.cpp:211-235): entry switch on the summoned spike.
-- INITIAL (27101): _count = 0, SetFacingToObject(owner), self-cast
-- SPELL_CRYSTAL_SPIKE_AURA (47941) triggered. TRIGGER (27079):
-- self-cast 47941 triggered, _count = owner trigger's
-- GetData(DATA_COUNT 1) + 1 (owner->ToCreature() null-gated in C++;
-- the Lua twin reads summoner:GetData only when the summoner is a
-- creature — a non-creature owner leaves _count at its ctor 0, and
-- an absent ScriptData key reads 0). Any other entry (defensive
-- default, :227-229): _count = MAX_COUNT. TRIGGER only: nearest
-- GO_CRYSTAL_SPIKE_TRAP (188537, 1.0f) Use — STAYS OUT: no generic
-- GameObject::Use bridge exists in Go (only UseDoorOrButton, the
-- door/button arm — wrong for a trap, which casts
-- goInfo->trap.spellId per GameObject::Use, GameObject.cpp:1484-
-- 1545) and the trap spell id is DB template data with no
-- gameobject_template row for 188537 in either tree. Both entries:
-- _despawntimer = 2000, then UpdateAI (:243-260) despawns at 0,
-- deleting the trap first for TRIGGER only. PORTED 2026-11-08
-- (tick 426). DORMANT: the summon spells 47936/47942/47943 are
-- DBC-side, so no spike spawns in Go yet; the chain goes live when
-- the spell-effect summon leg lands, and the 47941 handler above
-- then reads the latched DATA_COUNT. The 2s timer is cancelled on
-- ev4: C++ UpdateAI stops on death, so a killed spike neither
-- deletes its trap nor despawns (corpse cleanup handles it) — and
-- a timer-closure IsAlive check would be vacuous here, since
-- creature IsAlive/GetHealth read the construction-time snapshot
-- (lua_creature_events.go:6418, object.go:241).
local GO_CRYSTAL_SPIKE_TRAP = 188537 -- boss_ormorok.cpp:189

local spikeDespawnTimers = {}

local function crystalSpikeTriggerSummoned(event, creature, summoner)
    local entry = creature:GetEntry()
    if entry == NPC_CRYSTAL_SPIKE_INITIAL then
        creature:SetData(DATA_COUNT, 0)
        if summoner then
            creature:SetFacingToObject(summoner)
        end
        creature:CastSpell(creature, SPELL_CRYSTAL_SPIKE_AURA, true)
    elseif entry == NPC_CRYSTAL_SPIKE_TRIGGER then
        creature:CastSpell(creature, SPELL_CRYSTAL_SPIKE_AURA, true)
        if summoner and summoner:GetObjectType() == "Creature" then
            creature:SetData(DATA_COUNT, summoner:GetData(DATA_COUNT) + 1)
        end
    else
        creature:SetData(DATA_COUNT, MAX_COUNT)
    end
    local guid = creature:GetGUIDLow()
    spikeDespawnTimers[guid] = CreateLuaEvent(function()
        spikeDespawnTimers[guid] = nil
        if creature:GetEntry() == NPC_CRYSTAL_SPIKE_TRIGGER then
            local trap = creature:FindNearestGameObject(GO_CRYSTAL_SPIKE_TRAP, 1.0)
            if trap then
                trap:Delete()
            end
        end
        creature:DespawnOrUnsummon()
    end, 2000)
end

-- C++ UpdateAI stops on death, so the 2s despawn timer dies with
-- the spike; the trap Delete leg dies with it.
local function crystalSpikeTriggerDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    local id = spikeDespawnTimers[guid]
    if id then
        RemoveEventById(id)
        spikeDespawnTimers[guid] = nil
    end
end

RegisterCreatureEvent(NPC_CRYSTAL_SPIKE_INITIAL, 22, crystalSpikeTriggerSummoned)
RegisterCreatureEvent(NPC_CRYSTAL_SPIKE_TRIGGER, 22, crystalSpikeTriggerSummoned)
RegisterCreatureEvent(NPC_CRYSTAL_SPIKE_INITIAL, 4, crystalSpikeTriggerDied)
RegisterCreatureEvent(NPC_CRYSTAL_SPIKE_TRIGGER, 4, crystalSpikeTriggerDied)

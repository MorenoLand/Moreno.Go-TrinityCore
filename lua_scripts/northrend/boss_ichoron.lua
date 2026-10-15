-- Ichoron (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_ichoron.cpp
-- (7 scripts — boss_ichoron (CreatureScript via GetVioletHoldAI
-- (BossAI), DATA_ICHORON = 5); npc_ichor_globule (zero Talk arms);
-- spell_ichoron_drained / spell_ichoron_merge /
-- spell_ichoron_protective_bubble / spell_ichoron_splatter
-- (SpellScriptLoader Spell/AuraScripts); achievement_dehydration;
-- all registered from inside AddSC_boss_ichoron() — loader decl 147
-- / call 342, the "// Violet Hold" block in AddNorthrendScripts(),
-- northrend_script_loader.cpp, after AddSC_boss_erekem() (341);
-- AddSC_boss_lavanthor() (343) follows).
-- Entry: 29313 Ichoron (violet_hold.h NPC_ICHORON, line 98) —
-- entry-verifiable, registration proceeds; the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_ICHORON = 5
-- (violet_hold.h line 55).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (the toravon randtime convention).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCast(me) (2-arg = non-triggered);
-- creature:CastSpell(target, spell) = DoCast(target, spell)
-- (2-arg = non-triggered); creature:CastSpell(creature, spell, true)
-- = DoCast(me, spell, true) (triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDeath, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_ichoron.cpp):
-- boss_ichoronAI::Reset (:98-101) — DoCast(me, SPELL_THREAT_PROC
-- 61732, true) (:100, triggered) -> CastSpell(creature, 61732, true);
-- fires on event 23 (evade/reset — the spawn-time Reset cast has no
-- spawn-event bridge, documented); the BossAI::Reset passthrough leg
-- has no bridge.
-- boss_ichoronAI::JustEngagedWith (:103-107) — Talk(SAY_AGGRO 0)
-- (:106) kept; seeds the two combat timers below (the
-- BossAI::JustEngagedWith passthrough leg has no bridge).
-- ScheduleTasks water-bolt-volley leg (:213-217) — DoCastAOE
-- (SPELL_WATER_BOLT_VOLLEY 54241) (:215, 2-arg = non-triggered) =
-- self-cast per the in-tree DoCastAOE convention ->
-- CastSpell(creature, 54241); init {10000, 15000}, re-arm
-- {10000, 15000}.
-- ScheduleTasks water-blast leg (:219-224) — SelectTarget(Random, 0,
-- 50.0f) (:221) -> DoCast(target, SPELL_WATER_BLAST 54237) (:222,
-- 2-arg = non-triggered) -> CastSpell(target, 54237); init
-- {6000, 9000}, re-arm {6000, 9000}.
-- KilledUnit (:162-165) — if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY 1) — the razuvious player-gated variant precedent.
-- JustDied (:167-171) — Talk(SAY_DEATH 2) — the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge.
-- DOCUMENTED DEVIATIONS:
-- The water-blast SelectTarget 50.0f distance filter (:221) has no
-- bridge — the randomPlayer pool (alive same-map/instance players,
-- the keleseth / thorim precedent) ignores distance. C++ also omits
-- the player-only flag on this arm, so strictly any unit qualifies.
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) pre-check has no
-- bridge — casts fire unconditionally on their timers (the hodir
-- precedent).
-- DOCUMENTED-ONLY (in this header; entry 29313):
-- SAY_SPAWN (3) is never Talk()ed (dead enum text, the cyanigosa
-- precedent).
-- Talk(SAY_ENRAGE 4) rides the UpdateAI health-threshold leg
-- (:194-198: HealthBelowPct(25) + no DRAINED aura -> FRENZY 54312
-- triggered) — no health-threshold bridge; joins the bridge queue.
-- Talk(SAY_SHATTER 5) + Talk(EMOTE_SHATTER 7) ride the DoAction
-- ACTION_PROTECTIVE_BUBBLE_SHATTERED leg (:127-140: AOE splatter /
-- burst, DRAINED cast, 30% max-health damage leg, scheduler.DelayAll)
-- — no DoAction bridge; joins the no-DoAction-bridge queue.
-- Talk(SAY_BUBBLE 6) rides the DoAction ACTION_DRAINED leg (:141-149:
-- HealthAbovePct(30) -> protective bubble recast, triggered) — same
-- queue.
-- ACTION_WATER_GLOBULE_HIT (3% max-health heal, _dehydration = false)
-- — same queue.
-- The ScheduleTasks Async leg (:207-211: SHRINK 54297 +
-- PROTECTIVE_BUBBLE 54306 self-casts) seeds the bubble phase machine
-- — per the gothik bubble-phase precedent it stays out: the
-- shatter / DRAINED / re-bubble legs are all unbridged DoAction and
-- AuraScript legs, so a ported bubble would never shatter.
-- The npc_ichor_globule machine (SpellHit TRANSFORM 54268 +
-- MoveFollow, MovementInform FOLLOW_MOTION_TYPE MERGE 54269 +
-- DespawnOrUnsummon, DamageTaken SPLASH 59516) — no SpellHit /
-- MovementInform / DamageTaken bridges.
-- spell_ichoron_drained (59820 AuraScript: expire -> DoAction
-- ACTION_DRAINED :332), spell_ichoron_merge (54269 SpellScript:
-- shrink stack -1 + DoAction ACTION_WATER_GLOBULE_HIT :370),
-- spell_ichoron_protective_bubble (54306 AuraScript: charges <= 1 on
-- remove -> DoAction ACTION_PROTECTIVE_BUBBLE_SHATTERED :406),
-- spell_ichoron_splatter (54259 AuraScript: periodic globule summon
-- 54258/54264/54265/54266/54267, expire -> shrink +10 stacks) — no
-- SpellScript / AuraScript bridges.
-- achievement_dehydration (OnCheck AI()->GetData(DATA_DEHYDRATION))
-- — no GetData bridge.
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_ICHORON) leg — no instance-script bridge.

local ENTRY_ICHORON = 29313

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_WATER_BLAST = 54237
local SPELL_WATER_BOLT_VOLLEY = 54241
local SPELL_THREAT_PROC = 61732

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — alive players on
-- the same map/instance; the 50.0f distance filter is unbridged
-- (documented in the header; the keleseth / thorim precedent).
local function randomPlayer(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks water-bolt-volley leg (:213-217):
-- DoCastAOE(SPELL_WATER_BOLT_VOLLEY), 2-arg = non-triggered;
-- init 10-15s / re-arm 10-15s.
local function waterBoltVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_WATER_BOLT_VOLLEY)
    schedule(guid, "volley", {10000, 15000}, function()
        waterBoltVolleyTick(creature, guid)
    end)
end

-- C++ ScheduleTasks water-blast leg (:219-224): DoCast(target,
-- SPELL_WATER_BLAST), 2-arg = non-triggered; init 6-9s / re-arm 6-9s.
local function waterBlastTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_WATER_BLAST)
    end
    schedule(guid, "blast", {6000, 9000}, function()
        waterBlastTick(creature, guid)
    end)
end

-- C++ boss_ichoronAI::JustEngagedWith (:103-107): Talk(SAY_AGGRO)
-- kept; seeds the combat timers.
local function ichoronEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "volley", {10000, 15000}, function()
        waterBoltVolleyTick(creature, guid)
    end)
    schedule(guid, "blast", {6000, 9000}, function()
        waterBlastTick(creature, guid)
    end)
end

-- C++ boss_ichoronAI::Reset (:98-101): BossAI::Reset clears the
-- scheduler (mirrored by cancelTimers); DoCast(me, SPELL_THREAT_PROC,
-- true) — triggered self-cast.
local function ichoronReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset(): DoCast(me, SPELL_THREAT_PROC, true) is Reset-only
    -- (boss_ichoron.cpp). Alive-gated; the timer cancel rides the death
    -- leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PROC, true)
    end
end

-- Shared timer wipe for events 2/4.
local function ichoronCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit (:162-165): player-gated Talk(SAY_SLAY) — the
-- razuvious player-gated variant precedent.
local function ichoronKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:167-171): Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge.
local function ichoronJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ICHORON, 1, ichoronEnterCombat)
RegisterCreatureEvent(ENTRY_ICHORON, 2, ichoronCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ICHORON, 3, ichoronKilledUnit)
RegisterCreatureEvent(ENTRY_ICHORON, 4, ichoronJustDied)
RegisterCreatureEvent(ENTRY_ICHORON, 4, ichoronCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ICHORON, 23, ichoronReset)

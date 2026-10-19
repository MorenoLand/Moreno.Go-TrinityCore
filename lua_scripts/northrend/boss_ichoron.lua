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
-- (:106) kept; seeds the three combat timers below (the
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
-- UpdateAI frenzy leg (:194-198) — !_isFrenzy && HealthBelowPct(25) +
-- no DRAINED aura -> Talk(SAY_ENRAGE 4) + DoCast(me, SPELL_FRENZY
-- 54312, true), one-flip _isFrenzy latch. The Oct-era "no
-- health-threshold bridge" claim is RETRACTED: creature:GetHealthPct()
-- is bridged via the generic object fallback (engine/scripting/
-- object.go:107 — elder_nadox retraction precedent) and HealthBelowPct
-- (25) strict-< is C++-exact as GetHealthPct() < 25; creature:HasAura
-- (59820) is bridged (engine/world/lua_objects.go:228). The per-tick
-- check rides a 1s re-armed CreateLuaEvent poll seeded on combat start
-- (the sindragosa 1s third-phase-check convention), latch table
-- frenzyFired[], wiped on events 2/4/23; the poll stops re-arming
-- once the latch fires.
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
-- (Talk(SAY_ENRAGE 4) frenzy leg was documented-only in the Oct
-- sweep; RETRACTED — ported above per the health-threshold retraction.)
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
-- DespawnOrUnsummon) — no SpellHit / MovementInform bridges; the
-- DamageTaken SPLASH 59516 arm IS bridged (event 9, registered
-- 2026-10-19 — the stale "no DamageTaken bridges" claim retracted).
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
local ENTRY_ICHOR_GLOBULE = 29321
local SPELL_SPLASH = 59516

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_ENRAGE = 4

local SPELL_WATER_BLAST = 54237
local SPELL_WATER_BOLT_VOLLEY = 54241
local SPELL_THREAT_PROC = 61732
local SPELL_FRENZY = 54312
local SPELL_DRAINED = 59820

local timers = {}

-- C++ _isFrenzy latch (:185): one-flip Talk(SAY_ENRAGE 4) + FRENZY
-- (54312) cast guard.
local frenzyFired = {}

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

-- C++ UpdateAI frenzy leg (:194-198): !_isFrenzy && HealthBelowPct(25)
-- && !HasAura(SPELL_DRAINED 59820) -> Talk(SAY_ENRAGE 4), DoCast(me,
-- SPELL_FRENZY 54312, true), latch. HealthBelowPct(25) is strict-<, so
-- GetHealthPct() < 25 is C++-exact (object.go float pct, threshold 25
-- integer — equivalent; elder_nadox retraction precedent). The per-tick
-- UpdateAI check rides a 1s re-armed CreateLuaEvent poll seeded on
-- combat start, wiped on 2/4/23 (the sindragosa third-phase-check
-- 1s convention).
local function frenzyTick(creature, guid)
    if not frenzyFired[guid]
            and creature:GetHealthPct() < 25
            and not creature:HasAura(SPELL_DRAINED) then
        creature:Talk(SAY_ENRAGE)
        creature:CastSpell(creature, SPELL_FRENZY, true)
        frenzyFired[guid] = true
    end
    if not frenzyFired[guid] then
        schedule(guid, "frenzy", 1000, function()
            frenzyTick(creature, guid)
        end)
    end
end

-- C++ boss_ichoronAI::JustEngagedWith (:103-107): Talk(SAY_AGGRO)
-- kept; seeds the combat timers.
local function ichoronEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frenzyFired[guid] = nil
    schedule(guid, "volley", {10000, 15000}, function()
        waterBoltVolleyTick(creature, guid)
    end)
    schedule(guid, "blast", {6000, 9000}, function()
        waterBlastTick(creature, guid)
    end)
    schedule(guid, "frenzy", 1000, function()
        frenzyTick(creature, guid)
    end)
end

-- C++ boss_ichoronAI::Reset (:98-101): BossAI::Reset clears the
-- scheduler (mirrored by cancelTimers); DoCast(me, SPELL_THREAT_PROC,
-- true) — triggered self-cast.
local function ichoronReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frenzyFired[guid] = nil
    -- C++ Reset(): DoCast(me, SPELL_THREAT_PROC, true) is Reset-only
    -- (boss_ichoron.cpp). Alive-gated; the timer cancel rides the death
    -- leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PROC, true)
    end
end

-- Shared timer wipe for events 2/4.
local function ichoronCancelCombatTimers(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    frenzyFired[guid] = nil
end

-- C++ KilledUnit (:162-165): player-gated Talk(SAY_SLAY) — the
-- razuvious player-gated variant precedent.
local function ichoronKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
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

-- C++ npc_ichor_globule::DamageTaken (boss_ichoron.cpp:279-290):
-- if (!_splashTriggered && damage >= me->GetHealth()): latch,
-- DoCastAOE(SPELL_SPLASH 59516). DoCastAOE is self-cast per
-- UnitAI.h:322, so creature:CastSpell(creature, 59516). C++ uses the
-- direct lethal comparison and lets the damage through (the
-- keep-at-1hp retail ideal is unimplemented in C++) — no
-- postDamage guard. The latch resets only in the AI ctor, so the Lua
-- mirror wipes the GUID entry on 2/4/23. Inert today: globules spawn
-- from the unported spell_ichoron_splatter AuraScript.
local ichorSplashLatch = {}

local function ichorGlobuleDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if ichorSplashLatch[guid] then
        return false, damage
    end
    if damage >= creature:GetHealth() then
        ichorSplashLatch[guid] = true
        creature:CastSpell(creature, SPELL_SPLASH)
    end
    return false, damage
end

local function ichorGlobuleCleanup(event, creature)
    ichorSplashLatch[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 2, ichorGlobuleCleanup)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 4, ichorGlobuleCleanup)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 9, ichorGlobuleDamageTaken)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 23, ichorGlobuleCleanup)

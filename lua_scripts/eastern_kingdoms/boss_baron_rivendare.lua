-- Baron Rivendare (Stratholme)
-- Lua port of src/server/scripts/EasternKingdoms/Stratholme/
-- boss_baron_rivendare.cpp
-- (boss_baron_rivendare : public BossAI(creature, TYPE_BARON)
-- (stratholme.h); constructor raises the RaiseDead latch false;
-- Reset -> Reset + TYPE_RAMSTEIN DONE guard -> SetData(TYPE_BARON,
-- NOT_STARTED) + BossAI::Reset(); JustEngagedWith -> TYPE_BARON
-- NOT_STARTED -> IN_PROGRESS + EventMap arms EVENT_SHADOWBOLT 5s
-- / EVENT_CLEAVE 8s / EVENT_MORTALSTRIKE 12s / EVENT_RAISE_DEAD
-- 15s + BossAI::JustEngagedWith; JustDied -> SetData(TYPE_BARON,
-- DONE) + BossAI::JustDied; UpdateAI UpdateVictim + events.Update
-- + UNIT_STATE_CASTING gate + inner-loop event switch + post-loop
-- casting re-gate + DoMeleeAttackIfReady; GetAI via
-- GetStratholmeAI -> GetInstanceAI) (loader lines 134/312).
-- npc_summoned_skeleton (SpellHit -> SPELL_DEATH_PACT_2 (17471) ->
-- DoCastSelf(SPELL_DEATH_PACT_3 = 17472, TRIGGERED)) is registered
-- by the same AddSC but has no C++-verifiable NPC entry and its
-- SpellHit hook has no Lua bridge — entry-unverifiable, queued.
-- The Stratholme block is OPEN.
-- Entry (verifiable from the C++ sources): STRCreatureIds names
-- NPC_BARON = 10440 in stratholme.h, and instance_stratholme.
-- cpp tracks the entry-10440 creature as the Baron's GUID
-- (baronGUID: OnCreatureCreate case NPC_BARON at line 158;
-- instance->GetCreature(baronGUID) at line 348, the Ramstein
-- summon leg) — the name-to-entry tie is C++-verified
-- (kalecgos precedent, ramstein/timmy-strength).
-- Whole-server-tree grep confirms boss_baron_rivendare.cpp as the
-- only source of the boss AI (eastern_kingdoms_script_loader.cpp
-- loader lines only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. There is no UNIT_STATE_CASTING gate in Go, so
-- timers fire unconditionally (maiden precedent); the C++
-- JustEngagedWith arms map onto OnEnterCombat timers with the
-- C++-exact init delays (maiden convention).
-- Verifiable numbers (file's own enums): SPELL_SHADOWBOLT =
-- 17393 / SPELL_CLEAVE = 15284 / SPELL_MORTALSTRIKE = 15708 /
-- SPELL_DEATH_PACT_1 = 17698 (heal Rivendare, damage skeleton) /
-- SPELL_DEATH_PACT_2 = 17471 (visual) / SPELL_DEATH_PACT_3 =
-- 17472 (instant kill self) / SPELL_RAISE_DEAD = 17473 (inits
-- 17698 and 17471) / RaiseDeadSpells = {17475, 17476, 17477,
-- 17478, 17479, 17480} / SPELL_UNHOLY_AURA = 17467 (loaded-only,
-- never cast by the C++ AI — data foundation, not a ported arm).
-- Ported arms (C++ UpdateAI event arms):
-- - Shadowbolt: 5s init -> 10s re-arm; SelectTarget(Random, 0) ->
--   random alive player on the same map+instance with NO distance
--   bound (C++ DefaultTargetSelector dist=0 = "ignored" = unlimited
--   range, UnitAI.h:63; the threat-list-only pick is approximated —
--   no threat-list bridge); nil pick casts nothing, 10s re-arm
--   unconditional (C++ events.Repeat(10s) sits outside the target
--   gate).
-- - Cleave: 8s init -> Repeat(7s, 17s) {7000, 17000} (maiden
--   range convention); DoCastVictim(15284) non-triggered.
-- - Mortal Strike: 12s init -> Repeat(10s, 25s) {10000, 25000};
--   DoCastVictim(15708) non-triggered.
-- - RaiseDead: 15s init -> 12s re-arm; if !RaiseDead:
--   DoCastSelf(SPELL_RAISE_DEAD = 17473) non-triggered + 6x
--   DoCastSelf(RaiseDeadSpells, TRIGGERED) -> CastSpell on self
--   (triggered flag not modeled — mr_smite convention) +
--   Talk(EMOTE_RAISE_DEAD = 0); else Talk(EMOTE_DEATH_PACT = 1);
--   per-guid RaiseDead latch, set on the raise half, cleared ONLY on
--   the death-pact half — C++ Reset/JustEngagedWith never touch the
--   bool (constructor false persists across evades; C++-exact, so
--   OnReset/OnEnterCombat do not clear it).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's instance->GetData(TYPE_RAMSTEIN)==DONE ->
--   instance->SetData(TYPE_BARON, NOT_STARTED) leg,
--   JustEngagedWith's SetData(TYPE_BARON, IN_PROGRESS) leg,
--   JustDied's SetData(TYPE_BARON, DONE) leg, the BossAI(
--   creature, TYPE_BARON) data-constructor leg, and the
--   GetStratholmeAI -> GetInstanceAI leg (instance-script model
--   blocked, standing).
-- - npc_summoned_skeleton's SpellHit(17471) -> DoCastSelf(17472,
--   TRIGGERED) leg (no skeleton NPC entry is C++-verifiable from
--   code — the 17475-17480 raise spells are summoners, not entry
--   evidence — and the SpellHit hook has no Lua bridge:
--   AuraScript/SpellScript-check-handler-blocked class).

local NPC_BARON = 10440

local SPELL_SHADOWBOLT = 17393
local SPELL_CLEAVE = 15284
local SPELL_MORTALSTRIKE = 15708
local SPELL_RAISE_DEAD = 17473

local RAISE_DEAD_SPELLS = {17475, 17476, 17477, 17478, 17479, 17480}

local EMOTE_RAISE_DEAD = 0
local EMOTE_DEATH_PACT = 1

local timers = {}
local raiseDead = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function randomPlayerOnMap(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onShadowbolt(creature, guid)
    local target = randomPlayerOnMap(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOWBOLT)
    end
    schedule(guid, "shadowbolt", 10000, function() onShadowbolt(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {7000, 17000}, function() onCleave(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALSTRIKE)
    end
    schedule(guid, "mortalstrike", {10000, 25000}, function() onMortalStrike(creature, guid) end)
end

local function onRaiseDead(creature, guid)
    if not raiseDead[guid] then
        creature:CastSpell(creature, SPELL_RAISE_DEAD)
        for _, spell in ipairs(RAISE_DEAD_SPELLS) do
            creature:CastSpell(creature, spell)
        end
        raiseDead[guid] = true
        creature:Talk(EMOTE_RAISE_DEAD)
    else
        raiseDead[guid] = false
        creature:Talk(EMOTE_DEATH_PACT)
    end
    schedule(guid, "raisedead", 12000, function() onRaiseDead(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowbolt", 5000, function() onShadowbolt(creature, guid) end)
    schedule(guid, "cleave", 8000, function() onCleave(creature, guid) end)
    schedule(guid, "mortalstrike", 12000, function() onMortalStrike(creature, guid) end)
    schedule(guid, "raisedead", 15000, function() onRaiseDead(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_BARON, 1, onEnterCombat)
RegisterCreatureEvent(NPC_BARON, 2, onCombatEnd)
RegisterCreatureEvent(NPC_BARON, 4, onCombatEnd)
RegisterCreatureEvent(NPC_BARON, 23, onReset)

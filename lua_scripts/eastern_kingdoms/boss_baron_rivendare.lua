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
-- by the same AddSC but has no C++-verifiable NPC entry — STAY OUT,
-- re-verified 2026-11-13 (hook-coverage round 4): the SpellHit arm
-- belongs to npc_summoned_skeleton (boss_baron_rivendare.cpp:155-164),
-- not to Rivendare himself; its entry lives only in the DB-side
-- ScriptName binding (RegisterStratholmeCreatureAI, stratholme.h:111)
-- and the raise-spell summon entries in spell data, neither in the
-- C++ tree. The ev14 bridge itself is live (CreatureEventOnHitBySpell
-- = 14, scripting/runtime.go:379; the old "event 15" note was wrong —
-- 15 is SpellHitTarget).
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
-- AttackIfReady. UpdateAI's UNIT_STATE_CASTING gates (pre-loop
-- gate and in-loop post-event gate, boss_baron_rivendare.cpp:105/
-- 144): PORTED — each arm defers 500ms while creature:IsCasting()
-- (live CastingUntil window, lua_creature_events.go:1950;
-- broodkeeper / kirtonos precedent), no re-arm consumed. The C++
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
--   random alive threat-list unit with NO distance bound (C++
--   DefaultTargetSelector dist=0 = "ignored" = unlimited range,
--   UnitAI.h:63; SelectTarget(Random, 0) defaults, playerOnly=false);
--   nil pick casts nothing, 10s re-arm
--   unconditional (C++ events.Repeat(10s) sits outside the target
--   gate).
-- - Cleave: 8s init -> Repeat(7s, 17s) {7000, 17000} (maiden
--   range convention); DoCastVictim(15284) non-triggered.
-- - Mortal Strike: 12s init -> Repeat(10s, 25s) {10000, 25000};
--   DoCastVictim(15708) non-triggered.
-- - RaiseDead: 15s init -> 12s re-arm; if !RaiseDead:
--   DoCastSelf(SPELL_RAISE_DEAD = 17473) non-triggered + 6x
--   DoCastSelf(RaiseDeadSpells, TRIGGERED) -> CastSpell on self,
--   triggered +
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
--   TRIGGERED) leg (boss_baron_rivendare.cpp:159-163): STAY OUT — no
--   skeleton NPC entry is C++-verifiable from code (no NPC_ const in
--   the file or stratholme.h:50-66; whole-tree grep for 17475-17480
--   hits only the spell enum at boss_baron_rivendare.cpp:39-44,58-59;
--   the summon entries live in spell data, and engine/data/dbc/
--   holds no Spell.dbc to check — probing would materialize stubs).
--   The ev14 hook itself is live (dispatch sites in spells.go,
--   boss_ai.go, pet_combat.go; args (event, creature, caster,
--   spellID)); the block is purely the missing entry, so no
--   RegisterCreatureEvent can be written.

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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function randomThreatTarget(creature)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function onShadowbolt(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function() onShadowbolt(creature, guid) end)
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOWBOLT)
    end
    schedule(guid, "shadowbolt", 10000, function() onShadowbolt(creature, guid) end)
end

local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onCleave(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {7000, 17000}, function() onCleave(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mortalstrike", 500, function() onMortalStrike(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALSTRIKE)
    end
    schedule(guid, "mortalstrike", {10000, 25000}, function() onMortalStrike(creature, guid) end)
end

local function onRaiseDead(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "raisedead", 500, function() onRaiseDead(creature, guid) end)
        return
    end
    if not raiseDead[guid] then
        creature:CastSpell(creature, SPELL_RAISE_DEAD)
        for _, spell in ipairs(RAISE_DEAD_SPELLS) do
            creature:CastSpell(creature, spell, true)
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
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shadowbolt", 5000, function() onShadowbolt(creature, guid) end)
    schedule(guid, "cleave", 8000, function() onCleave(creature, guid) end)
    schedule(guid, "mortalstrike", 12000, function() onMortalStrike(creature, guid) end)
    schedule(guid, "raisedead", 15000, function() onRaiseDead(creature, guid) end)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_baron_rivendare.cpp:87 base call last)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_baron_rivendare.cpp:87 base call last)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(NPC_BARON, 1, onEnterCombat)
RegisterCreatureEvent(NPC_BARON, 2, onCombatEnd)
RegisterCreatureEvent(NPC_BARON, 4, onCombatEnd)
RegisterCreatureEvent(NPC_BARON, 23, onReset)

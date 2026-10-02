-- Silithus: Qiraji war event spawns (Qiraji Wasp / Qiraji Tank /
-- Kaldorei Infantry / Anubisath Conqueror, the "A Pawn on the Eternal
-- Board" war waves) — Lua port of
-- src/server/scripts/Kalimdor/zone_silithus.cpp (class
-- npc_qiraj_war_spawn : CreatureScript("npc_qiraj_war_spawn") {
-- npc_qiraj_war_spawnAI : public ScriptedAI }; AddSC_silithus at end
-- registers the six zone scripts; kalimdor loader decl 119 / call
-- 232 per kalimdor_script_loader.cpp). Whole-server-tree grep for all
-- six script names ("go_crystalline_tear",
-- "npc_anachronos_quest_trigger", "npc_anachronos_the_ancient",
-- "npc_qiraj_war_spawn", "go_wind_stone",
-- "spell_silithus_summon_cultist_periodic") hits only
-- zone_silithus.cpp (sole-source verified); zero sql/ hits.
-- Entries: zone_silithus.cpp enum AnachronosTheAncient names
-- NPC_QIRAJI_WASP = 15414, NPC_QIRAJI_TANK = 15422,
-- NPC_KALDOREI_INFANTRY = 15423, NPC_ANUBISATH_CONQUEROR = 15424 —
-- kalecgos check PASSES for all four; creature_template ScriptName
-- binding stays DB-side. Eluna creature events: 1 OnEnterCombat,
-- 2 OnLeaveCombat, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The file has no UNIT_STATE_CASTING gates.
-- Ported arms (from npc_qiraj_war_spawnAI::UpdateAI, entry-gated
-- exactly as in C++ — qiraji-side entries 15414/15422/15424 get the
-- spell timers; all four entries get the stoned channel):
-- - Poison cloud (C++ SpellTimer1): DoCast(self, SPELL_POISON_CLOUD
--   28528) + DoCast(self, SPELL_SUMMON_POISON_CLOUD 24319), init
--   38500ms, repeat 300000ms, unconditional.
-- - Frost debuff (C++ SpellTimer2): DoCast(self,
--   SPELL_FROST_DEBUFF 35871), init 58000ms, repeat 300000ms,
--   unconditional.
-- - Fire explosion (C++ SpellTimer3): DoCast(self,
--   SPELL_FIRE_EXPLOSION 42075), init 80950ms, repeat 300000ms,
--   unconditional.
-- - Stoned channel (C++ SpellTimer4): DoCast(self,
--   SPELL_STONED_CHANNEL_CAST_VISUAL 15533), init 100000ms, repeat
--   2000ms, unconditional. Deviation documented below: the C++
--   preamble me->RemoveAllAttackers() + me->AttackStop() has no
--   bridge (no such methods on the creature Lua surface), so the
--   port models the cast itself, not the combat drop.
-- LeaveCombat/Died/Reset cancel timers (hyjal.lua convention).
-- Unmodeled (documented-only, no bridges):
-- - Target acquisition (C++ hasTarget/AttackStart via
--   me->FindNearestCreature(entry, 20, true) — kaldorei infantry
--   pick a random qiraji entry, qiraji-side picks nearest kaldorei):
--   no creature-lookup bridge on the Lua surface (standing).
-- - Stoned aura arm: if no Caelestrasz within 60y
--   (FindNearestCreature(NPC_CAELESTRASZ 15380, 60) —
--   creature-lookup bridge absent), DoCast(self, SPELL_STONED
--   33652) every tick: unreachable.
-- - JustDied: me->DespawnOrUnsummon() + LiveCounter() on the
--   trigger AI via MobGUID: no despawn bridge (giant_spotlight
--   precedent) and no cross-AI GUID-lookup bridge.

local ENTRY_QIRAJI_WASP       = 15414
local ENTRY_QIRAJI_TANK       = 15422
local ENTRY_KALDOREI_INFANTRY = 15423
local ENTRY_ANUBISATH_CONQUEROR = 15424

local SPELL_POISON_CLOUD          = 28528
local SPELL_SUMMON_POISON_CLOUD   = 24319
local SPELL_FROST_DEBUFF          = 35871
local SPELL_FIRE_EXPLOSION        = 42075
local SPELL_STONED_CHANNEL_VISUAL = 15533

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ SpellTimer1: poison cloud pair, repeat 300000ms unconditional.
local function onPoisonCloud(creature, guid)
    creature:CastSpell(creature, SPELL_POISON_CLOUD)
    creature:CastSpell(creature, SPELL_SUMMON_POISON_CLOUD)
    schedule(guid, "poison", 300000, function()
        onPoisonCloud(creature, guid)
    end)
end

-- C++ SpellTimer2: frost debuff, repeat 300000ms unconditional.
local function onFrostDebuff(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_DEBUFF)
    schedule(guid, "frost", 300000, function()
        onFrostDebuff(creature, guid)
    end)
end

-- C++ SpellTimer3: fire explosion, repeat 300000ms unconditional.
local function onFireExplosion(creature, guid)
    creature:CastSpell(creature, SPELL_FIRE_EXPLOSION)
    schedule(guid, "fire", 300000, function()
        onFireExplosion(creature, guid)
    end)
end

-- C++ SpellTimer4: stoned channel visual, repeat 2000ms
-- unconditional (RemoveAllAttackers/AttackStop preamble has no
-- bridge — documented deviation, see header).
local function onStonedChannel(creature, guid)
    creature:CastSpell(creature, SPELL_STONED_CHANNEL_VISUAL)
    schedule(guid, "stoned", 2000, function()
        onStonedChannel(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_QIRAJI_WASP or entry == ENTRY_QIRAJI_TANK
        or entry == ENTRY_ANUBISATH_CONQUEROR then
        schedule(guid, "poison", 38500, function()
            onPoisonCloud(creature, guid)
        end)
        schedule(guid, "frost", 58000, function()
            onFrostDebuff(creature, guid)
        end)
        schedule(guid, "fire", 80950, function()
            onFireExplosion(creature, guid)
        end)
    end
    schedule(guid, "stoned", 100000, function()
        onStonedChannel(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local ENTRIES = {
    ENTRY_QIRAJI_WASP,
    ENTRY_QIRAJI_TANK,
    ENTRY_KALDOREI_INFANTRY,
    ENTRY_ANUBISATH_CONQUEROR,
}
for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onLeaveCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, onReset)
end

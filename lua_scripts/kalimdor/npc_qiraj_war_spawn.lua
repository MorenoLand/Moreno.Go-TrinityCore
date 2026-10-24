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
-- binding stays DB-side. Eluna creature events: 1 OnEnterCombat
-- (first-engage arm latch only — C++ JustEngagedWith is empty),
-- 4 OnDied, 23 OnReset (evade/respawn re-arm; death-path pre-fire
-- skipped). Event 2 intentionally unused: the C++ timers tick outside
-- combat. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. The file has no UNIT_STATE_CASTING gates.
-- Ported arms (from npc_qiraj_war_spawnAI::UpdateAI, entry-gated
-- exactly as in C++ — qiraji-side entries 15414/15422/15424 get the
-- spell timers; all four entries get the stoned channel,
-- including its RemoveAllAttackers + AttackStop preamble
-- (both bridged — old no-bridge deviation retracted):
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
--   preamble me->RemoveAllAttackers() + me->AttackStop() is
--   ported in onStonedChannel below (C++ order,
--   zone_silithus.cpp SpellTimer4 arm), both methods bridged.
-- Died/Reset cancel + re-arm per the C++ Reset latch (see below);
-- the old leave-combat cancel was wrong (the C++ timers tick outside
-- combat). Unmodeled (documented-only, no bridges):
-- - Target acquisition (C++ hasTarget/AttackStart via
--   me->FindNearestCreature(entry, 20, true) — kaldorei infantry
--   pick a random qiraji entry, qiraji-side picks nearest kaldorei):
--   the lookup itself is now bridgeable
--   (creature:FindNearestCreature, engine/world/lua_creature_events.go:2105;
--   old no-creature-lookup claim retracted), but the arms stay
--   documented-only — the war-spawn NPCs ride the unported
--   Anachronos cinematic switch and their entries are unregistered.
-- - Stoned aura arm: if no Caelestrasz within 60y
--   (FindNearestCreature(NPC_CAELESTRASZ 15380, 60)), DoCast(self,
--   SPELL_STONED 33652) every tick: unreachable (same bar).
-- - JustDied: PORTED — me->DespawnOrUnsummon() as creature:Despawn()
--   at the top of onDied (illidari_spawn precedent, C++-exact:
--   despawn first, then the blocked leg). LiveCounter() on the
--   trigger AI via MobGUID stays documented-only: no cross-AI
--   GUID-lookup bridge (the true bar; the old "no despawn bridge"
--   was stale — creature:Despawn() is bridged,
--   lua_creature_events.go:214).

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
-- unconditional; preamble in C++ order (RemoveAllAttackers,
-- AttackStop, then the cast).
local function onStonedChannel(creature, guid)
    creature:RemoveAllAttackers()
    creature:AttackStop()
    creature:CastSpell(creature, SPELL_STONED_CHANNEL_VISUAL)
    schedule(guid, "stoned", 2000, function()
        onStonedChannel(creature, guid)
    end)
end

-- C++ SpawnCast table (zone_silithus.cpp:289-295): {100000, 2000,
-- 33652} Stoned / {38500, 300000, 28528} Poison Cloud / {58000,
-- 300000, 35871} Frost Debuff / {80950, 300000, 42075} Fire
-- Explosion. Timers tick from the first UpdateAI (spawn) and run
-- OUTSIDE combat — the C++ UpdateAI has no UpdateVictim gate before
-- the timer arms, and JustEngagedWith is EMPTY (no engage reset).
-- Go has no spawn fire site for summoned creatures (fireCreatureSpawned
-- runs only on the respawn/evade paths), so the earliest bridgeable
-- arm point is first enter combat; the timers then persist across
-- combat like the C++ (no leave-combat cancel, no engage re-arm —
-- the old combat-gated model restarted the timers on every engage,
-- which the C++ never does). Evade/respawn (event 23) re-arms the
-- init delays (C++ Reset() re-inits the Timers latch); the death-path
-- 23 pre-fire is skipped via the IsDead guard (overgrind precedent).

local armed = {}

local function armTimers(creature, guid)
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
    armed[guid] = true
end

-- C++ JustEngagedWith is empty: engaging neither arms nor resets the
-- timers. The first engage is only the earliest bridgeable arm point
-- (no summon-path spawn event in Go); the latch keeps later engages
-- from restarting them.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    if not armed[guid] then
        armTimers(creature, guid)
    end
end

-- C++ Reset() (evade/respawn) re-inits the Timers latch: cancel and
-- re-arm the init delays. The death-path 23 pre-fire must not re-arm
-- (23 fires before 4 on the kill path).
local function onReset(event, creature)
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    armed[guid] = nil
    armTimers(creature, guid)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    armed[guid] = nil
    -- C++ JustDied: me->DespawnOrUnsummon() first (illidari_spawn
    -- precedent); cross-GUID LiveCounter stays unbridged.
    creature:Despawn()
end

local ENTRIES = {
    ENTRY_QIRAJI_WASP,
    ENTRY_QIRAJI_TANK,
    ENTRY_KALDOREI_INFANTRY,
    ENTRY_ANUBISATH_CONQUEROR,
}
for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, onReset)
end

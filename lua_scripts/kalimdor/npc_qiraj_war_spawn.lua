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
-- zone_silithus.cpp (sole-source verified); zero sql/ hits except
-- go_wind_stone, which has 12 TDB 26091 gameobject_template rows and
-- is ported in zone_silithus.lua (2026-11-12).
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
-- - Target acquisition (C++ hasTarget/AttackStart arm): 1s pump,
--   victimless-only; qiraji-side nearest kaldorei 15423 within 20y,
--   kaldorei random qiraji entry nearest within 20y (no fallback);
--   AttackStart = creature:EngageWithTarget, creature-victim funnel
--   live (creaturemotion.go:1371 S1). The old stay-out is retired:
--   the entries ARE registered (boss_ai.go:16803-16806, commit
--   020ace72) and the arm is self-contained — it does not ride the
--   unported cinematic/trigger machine.
-- - Stoned aura arm: 1s pump; no Caelestrasz 15379 within 60y ->
--   DoCast(self, SPELL_STONED 33652), all four entries. (The old
--   header cited 15380 — that is Arygos; Caelestrasz is 15379.)
-- Died/Reset cancel + re-arm per the C++ Reset latch (see below);
-- the old leave-combat cancel was wrong (the C++ timers tick outside
-- combat). Unmodeled (documented-only, no bridges):
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
local ENTRY_CAELESTRASZ       = 15379

local SPELL_POISON_CLOUD          = 28528
local SPELL_SUMMON_POISON_CLOUD   = 24319
local SPELL_FROST_DEBUFF          = 35871
local SPELL_FIRE_EXPLOSION        = 42075
local SPELL_STONED_CHANNEL_VISUAL = 15533
local SPELL_STONED                = 33652

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

-- C++ UpdateAI hasTarget/AttackStart arm + stoned-aura arm
-- (zone_silithus.cpp:749-774): both tick every UpdateAI while the mob
-- is alive; the Go twin is a 1s pump (per-tick cadence is unmodelable,
-- same CreateLuaEvent approximation as the timer arms).
-- Target acquisition: qiraji-side entries take the nearest living
-- Kaldorei Infantry within 20y; Kaldorei Infantry picks one qiraji
-- entry at random (C++ urand(0,2): 0 tank, 1 conqueror, 2 wasp) and
-- takes the nearest living creature of that entry within 20y — a
-- miss on the picked entry finds nothing (no fallback to the other
-- entries). Only attempted while victimless (C++ hasTarget latch:
-- re-armed only when UpdateVictim fails). AttackStart rides
-- creature:EngageWithTarget (lua_creature_events.go:1276, AttackStart
-- alias :6125; the creature-victim funnel is triggerCreatureAggro S1,
-- creaturemotion.go:1371). FindNearestCreature(entry, 20) is the
-- alive-only lookup (lua_creature_events.go:5592).
-- Stoned aura: no Caelestrasz (15379, EternalBoard enum — NOT 15380,
-- that is Arygos) within 60y -> DoCast(self, SPELL_STONED 33652),
-- unconditional for all four entries.
local QIRAJI_ENTRIES = {
    ENTRY_QIRAJI_TANK,
    ENTRY_ANUBISATH_CONQUEROR,
    ENTRY_QIRAJI_WASP,
}

local function onAcquireAndStoned(creature, guid)
    local entry = creature:GetEntry()
    if not creature:GetVictim() then
        local target = nil
        if entry == ENTRY_QIRAJI_WASP or entry == ENTRY_QIRAJI_TANK
            or entry == ENTRY_ANUBISATH_CONQUEROR then
            target = creature:FindNearestCreature(ENTRY_KALDOREI_INFANTRY, 20)
        elseif entry == ENTRY_KALDOREI_INFANTRY then
            target = creature:FindNearestCreature(QIRAJI_ENTRIES[math.random(#QIRAJI_ENTRIES)], 20)
        end
        if target then
            creature:AttackStart(target)
        end
    end
    if not creature:FindNearestCreature(ENTRY_CAELESTRASZ, 60) then
        creature:CastSpell(creature, SPELL_STONED)
    end
    schedule(guid, "acquire", 1000, function()
        onAcquireAndStoned(creature, guid)
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
    -- C++ UpdateAI hasTarget/AttackStart + stoned-aura arms, same
    -- arming point as the spell timers (first engage / reset).
    schedule(guid, "acquire", 1000, function()
        onAcquireAndStoned(creature, guid)
    end)
    armed[guid] = true
end

-- C++ JustEngagedWith is empty: engaging neither arms nor resets the
-- timers. The first engage is only the earliest bridgeable arm point
-- (no summon-path spawn event in Go); the latch keeps later engages
-- from restarting them.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
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
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    armed[guid] = nil
    armTimers(creature, guid)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
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

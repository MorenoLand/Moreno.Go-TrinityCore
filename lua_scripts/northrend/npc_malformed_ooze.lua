-- Malformed Ooze (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/boss_sjonnir.cpp
-- (npc_malformed_ooze (CreatureScript) via
-- GetHallsOfStoneAI<npc_malformed_oozeAI> (ScriptedAI)).
-- Entry: 27981 Malformed Ooze (boss_sjonnir.cpp:45 local enum;
-- NPC_IRON_SLUDGE = 28165, :47) — entry-verifiable, registered in
-- engine/world/boss_ai.go (the npc_iron_sludge precedent).
-- Eluna creature events: 4 OnDied, 5 OnSpawn, 23 OnReset. Driven by
-- CreateLuaEvent timers; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- UpdateAI merge machine (boss_sjonnir.cpp:243-258): the merge check
-- runs BEFORE UpdateVictim, i.e. out of combat too — so the pump is
-- seeded on spawn (ev5, the ctor Initialize twin), not on enter-combat.
-- _mergeTimer 10s init (:237) / 3s re-loop (:257): FindNearestCreature
-- (NPC_MALFORMED_OOZE 27981, 3.0f, alive) -> DoSpawnCreature
-- (NPC_IRON_SLUDGE 28165, self position,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 20s) + the other ooze's
-- DisappearAndDie + self DisappearAndDie. The summon type is Go type 3
-- (creaturemotion.go TempSummonType map); the spawn position rides the
-- self-position precedent (bosses_opera wolf summon). After a merge
-- both oozes are gone, so the pump does NOT re-arm on that path — the
-- 3s re-arm runs only when no partner is found. Despawn is the
-- DisappearAndDie twin (lua_creature_events.go:3024).
-- Reset (boss_sjonnir.cpp:239-242): Initialize() re-seeds the 10s
-- timer — PORTED on ev23, alive-guarded (ev23 fires on death too in
-- Go; C++ Reset only runs on a living creature).
-- Why it matters: iron sludges spawn ONLY via ooze merges, so without
-- this bearer the ported sludge JustDied -> DoAction(ACTION_OOZE_DEAD)
-- relay, the abuseTheOoze counter, and achievement_abuse_the_ooze
-- (criteria script live, >= 5) could never fire — the achievement was
-- unreachable. The old "no proximity / summon / despawn / timer
-- bridges" header claim in boss_sjonnir.lua was stale: all four bridges
-- are live (FindNearestCreature lua_creature_events.go:6094 — skips
-- self and dead, the C++ alive=true twin; SummonCreature :3718;
-- Despawn :3024; CreateLuaEvent runtime.go:798).

local ENTRY_MALFORMED_OOZE = 27981

local NPC_IRON_SLUDGE = 28165

-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT = Go type 3
-- (creaturemotion.go TempSummonType map).
local SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT = 3

-- CreateLuaEvent timer harness (GUIDLow-keyed timers table, cancel on
-- events 4/23) — the maiden / bjarngrim-lieutenant convention.
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

-- C++ UpdateAI merge machine (boss_sjonnir.cpp:243-258): 10s init /
-- 3s re-loop; FindNearestCreature(27981, 3.0f, alive) -> spawn an iron
-- sludge at self position (20s out-of-combat despawn) + both oozes
-- DisappearAndDie. No IsCasting gate in C++ (the check runs before
-- UpdateVictim), so none here either.
local function mergeTick(creature, guid)
    local other = creature:FindNearestCreature(ENTRY_MALFORMED_OOZE, 3.0)
    if other then
        creature:SummonCreature(NPC_IRON_SLUDGE, creature:GetX(),
            creature:GetY(), creature:GetZ(), creature:GetO(),
            SUMMON_TYPE_TIMED_DESPAWN_OUT_OF_COMBAT, 20000)
        -- The partner's pending merge timer must be cancelled before it
        -- is despawned: Despawn fires no ev4, so an uncancelled timer
        -- would later fire mergeTick on a removed motion. The merging
        -- ooze's own timer does not re-arm on this path (both oozes are
        -- gone), so its table entry is dropped, not re-scheduled.
        cancelTimers(other:GetGUIDLow())
        timers[guid] = nil
        other:Despawn()
        creature:Despawn()
        return
    end
    schedule(guid, "merge", 3000, function()
        mergeTick(creature, guid)
    end)
end

-- C++ ctor Initialize (boss_sjonnir.cpp:233-237): _mergeTimer = 10000;
-- the ev5 spawn leg is the ctor twin (respawn rebuilds the AI, same as
-- the sjonnir ev5 SetGuidData precedent).
local function oozeSpawned(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "merge", 10000, function()
        mergeTick(creature, guid)
    end)
end

local function oozeDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_sjonnir.cpp:239-242): Initialize() — re-seed 10s.
local function oozeReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "merge", 10000, function()
        mergeTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_MALFORMED_OOZE, 4, oozeDied)
RegisterCreatureEvent(ENTRY_MALFORMED_OOZE, 5, oozeSpawned)
RegisterCreatureEvent(ENTRY_MALFORMED_OOZE, 23, oozeReset)

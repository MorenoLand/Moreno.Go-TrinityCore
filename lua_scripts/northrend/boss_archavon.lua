-- Archavon the Stone Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_archavon.cpp
-- (188 lines; 2 scripts — boss_archavon (CreatureScript via
-- GetVaultOfArchavonAI (BossAI), DATA_ARCHAVON = 0) and
-- spell_archavon_rock_shards (SpellScriptLoader); both registered from
-- inside AddSC_boss_archavon() — loader decl 139 / call 334, the FIRST
-- group of the "// Vault of Archavon" block in AddNorthrendScripts()).
-- Entries: 31125 Archavon the Stone Watcher (vault_of_archavon.h
-- NPC_ARCHAVON, line 37); DATA_ARCHAVON = 0 (vault_of_archavon.h
-- line 30). NPC_ARCHAVON_WARDER 32353 (boss_archavon.cpp enum
-- Creatures) is never referenced by any arm in this file — no legs use
-- it (no summon machine exists). Sole-source verified: "boss_archavon"
-- hits the owner .cpp + the loader decl/call lines only;
-- "spell_archavon_rock_shards" hits the owner .cpp only (registered
-- inside AddSC, never named in the loader); the clone's sql/ tree
-- (~/workspace/moreno-trinitycore/sql/, characters/ + world/) carries
-- zero archavon hits (empty of bindings). The 2026-10-06 00:21
-- "zero bridgeable arms" closure is RETRACTED for this file: the stale
-- "no-timer-bridge" doctrine is superseded by the CreateLuaEvent bridge
-- (engine/scripting/runtime.go:798 — the maiden_of_virtue / keleseth /
-- yogg_saron / svala precedent): module-local GUID-keyed timers table,
-- wipe on events 2 (leave combat), 4 (death), 23 (evade).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCast(me); creature:CastSpell
-- (target, spell[, triggered]) = DoCast(target, spell[, triggered])
-- (the 3rd CastSpell arg is the C++ DoCast 3rd arg; 2-arg / 1-arg
-- overloads = non-triggered). All five timer values here are fixed
-- (not urand), so plain ms delays carry the C++-exact timing — no
-- {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_archavon.cpp):
-- boss_archavonAI::JustEngagedWith (:69-77) — the four ScheduleEvent
-- calls are carried by the event-1 CreateLuaEvent scheduling below
-- (the xt002 precedent); BossAI::JustEngagedWith(who) passthrough and
-- the zero Talk arms are unmodeled.
-- EVENT_ROCK_SHARDS (:93-98) — SelectTarget(Random, 0) (:95, no
-- distance filter) -> DoCast(target, SPELL_ROCK_SHARDS 58678) (:96,
-- 2-arg = non-triggered) -> CastSpell(target, 58678); re-arm 15s
-- (:97). Random-player pool: GetPlayersInWorld, same map/instance,
-- alive (the keleseth / thorim GetPlayersInWorld precedent).
-- EVENT_CHOKING_CLOUD (:99-106) — SelectTarget(Random, 0, -10.0f,
-- true) (:101; random player, min 10y, playerOnly) -> DoCast(target,
-- SPELL_CRUSHING_LEAP 58960, true) (:102, 3-arg = TRIGGERED) ->
-- CastSpell(target, 58960, true); Talk(EMOTE_LEAP 1, target) (:103) ->
-- creature:Talk(1, target) (the targeted-emote whisper target is
-- bridgeable — the keleseth / mandokir / razorscale precedent);
-- re-arm 30s (:105).
-- EVENT_STOMP (:107-111) — DoCastVictim(SPELL_STOMP 58663) (:108,
-- 1-arg = non-triggered) -> CastSpell(nil, 58663); ScheduleEvent
-- (EVENT_IMPALE, 3s) (:109); re-arm STOMP 45s (:110).
-- EVENT_IMPALE (:112-114) — DoCastVictim(SPELL_IMPALE 58666) (:113,
-- 1-arg = non-triggered) -> CastSpell(nil, 58666); one-shot, no
-- re-arm (fires only via the 3s stomp-chain leg, C++-exact).
-- EVENT_BERSERK (:115-118) — DoCast(me, SPELL_BERSERK 47008) (:116,
-- 2-arg = non-triggered) -> CastSpell(creature, 47008); Talk
-- (EMOTE_BERSERK 0) (:117) -> creature:Talk(0); cast-before-Talk kept
-- (:116-117 order — the kologarn reversal-fix precedent); 5min
-- one-shot, no re-arm (the yogg_saron / thorim / assembly_of_iron /
-- hodir 5min+ one-shot berserk precedent).
-- DOCUMENTED DEVIATIONS:
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) gates (pre-check
-- :87-88 and the per-event :120 return) have no bridge — casts fire
-- unconditionally on their timers (the hodir UNIT_STATE_CASTING
-- precedent).
-- EVENT_CHOKING_CLOUD's SelectTarget min-range gate (-10.0f = skip
-- targets within 10y, :101) has no bridge — the cast fires on any
-- alive same-map/instance player (the thorim colossus
-- MaxDistance->random documented-deviation precedent).
-- DOCUMENTED-ONLY (in this header; entry 31125 is the sole
-- registration):
-- spell_archavon_rock_shards (SpellScriptLoader :141-176;
-- spell_archavon_rock_shards_SpellScript OnEffectHit HandleScript
-- :160-170 — the "// 58941 - Rock Shards" :140 hook firing the four
-- visual/damage self-casts 58689/58692/58695/58696, triggered) — no
-- SpellScript bridge; joins the no-SpellScript-bridge queue.
-- NPC_ARCHAVON_WARDER 32353 — unreferenced by every arm in this file
-- (no summon legs).
-- DoMeleeAttackIfReady() (:121) — no bridge.
-- No JustDied, no KilledUnit, no DamageTaken, no DoAction, no
-- MovementInform, no EnterEvadeMode override exist on this AI —
-- events 3/4 on the creature carry only the timer wipe.

local ENTRY_ARCHAVON = 31125

local EMOTE_BERSERK = 0
local EMOTE_LEAP = 1

local SPELL_ROCK_SHARDS = 58678
local SPELL_CRUSHING_LEAP = 58960
local SPELL_STOMP = 58663
local SPELL_IMPALE = 58666
local SPELL_BERSERK = 47008

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
-- the same map/instance, no distance filter (the keleseth / thorim
-- GetPlayersInWorld precedent).
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

-- C++ EVENT_ROCK_SHARDS (:93-98): DoCast(target, SPELL_ROCK_SHARDS),
-- 2-arg = non-triggered; re-arm unconditional (the SelectTarget if only
-- wraps the cast, :95-96), 15s.
local function rockShardsTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_ROCK_SHARDS)
    end
    schedule(guid, "rockshards", 15000, function()
        rockShardsTick(creature, guid)
    end)
end

-- C++ EVENT_CHOKING_CLOUD (:99-106): DoCast(target,
-- SPELL_CRUSHING_LEAP, true) — 3-arg = TRIGGERED; Talk(EMOTE_LEAP,
-- target); re-arm unconditional, 30s. The 10y min-range SelectTarget
-- gate is unmodeled (documented in the header).
local function chokingCloudTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_CRUSHING_LEAP, true)
        creature:Talk(EMOTE_LEAP, target)
    end
    schedule(guid, "chokingcloud", 30000, function()
        chokingCloudTick(creature, guid)
    end)
end

-- C++ EVENT_IMPALE (:112-114): DoCastVictim(SPELL_IMPALE), 1-arg =
-- non-triggered; one-shot, no re-arm.
local function impaleTick(creature, guid)
    creature:CastSpell(nil, SPELL_IMPALE)
end

-- C++ EVENT_STOMP (:107-111): DoCastVictim(SPELL_STOMP), 1-arg =
-- non-triggered; chains EVENT_IMPALE at 3s (:109); re-arm 45s (:110).
local function stompTick(creature, guid)
    creature:CastSpell(nil, SPELL_STOMP)
    schedule(guid, "impale", 3000, function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "stomp", 45000, function()
        stompTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:115-118): DoCast(me, SPELL_BERSERK), 2-arg =
-- non-triggered; Talk(EMOTE_BERSERK); cast-before-Talk kept; 5min
-- one-shot, no re-arm.
local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_BERSERK)
end

-- C++ boss_archavonAI::JustEngagedWith (:69-77): the four ScheduleEvent
-- calls (15s / 30s / 45s / 5min) seed the four combat timers below
-- (EVENT_IMPALE rides the stomp chain, not a combat-start timer);
-- the BossAI::JustEngagedWith(who) passthrough and the zero Talk arms
-- are unmodeled.
local function archavonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rockshards", 15000, function()
        rockShardsTick(creature, guid)
    end)
    schedule(guid, "chokingcloud", 30000, function()
        chokingCloudTick(creature, guid)
    end)
    schedule(guid, "stomp", 45000, function()
        stompTick(creature, guid)
    end)
    schedule(guid, "berserk", 300000, function()
        berserkTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function archavonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ARCHAVON, 1, archavonEnterCombat)
RegisterCreatureEvent(ENTRY_ARCHAVON, 2, archavonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ARCHAVON, 4, archavonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ARCHAVON, 23, archavonCancelCombatTimers)

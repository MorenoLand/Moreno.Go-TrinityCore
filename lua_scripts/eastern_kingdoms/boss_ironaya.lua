-- Ironaya (Uldaman)
-- Lua port of src/server/scripts/EasternKingdoms/Uldaman/
-- boss_ironaya.cpp
-- (boss_ironaya : public ScriptedAI (NOT BossAI — constructor
-- Initialize() zeroes uiArcingTimer 3000 / bHasCastKnockaway /
-- bHasCastWstomp + instance = creature->GetInstanceScript();
-- Reset -> Initialize(); JustEngagedWith empty; UpdateAI: no
-- target -> return; !bHasCastKnockaway && HealthBelowPct(50) &&
-- GetVictim -> DoCastVictim(10101 SPELL_KNOCKAWAY, true) +
-- GetThreatManager().ResetThreat(EnsureVictim()) + latch;
-- uiArcingTimer 3000 init -> DoCast(me, 8374 SPELL_ARCINGSMASH),
-- re-arm 13000; !bHasCastWstomp && HealthBelowPct(25) -> DoCast
-- (me, 11876 SPELL_WSTOMP) + latch; DoMeleeAttackIfReady;
-- GetAI via GetUldamanAI -> GetInstanceAI; loader lines
-- 150/329; SD%Complete: 100).
-- Entry (verifiable from the C++ sources): instance_uldaman.cpp
-- OnCreatureCreate switches on the raw entries with
-- name-comments — case 7228: // Ironaya -> ironayaGUID — the
-- name-to-entry tie is C++-verified (kalecgos entry-
-- verifiability check, ramstein strength: sole-source comment
-- tie, not DB).
-- Whole-server-tree grep confirms boss_ironaya.cpp as the sole
-- source of boss_ironaya (loader lines only otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. There is no UNIT_STATE_CASTING gate in
-- Go, so timers fire unconditionally (maiden precedent).
-- Verifiable numbers (file's own enums): SPELL_ARCINGSMASH =
-- 8374 / SPELL_KNOCKAWAY = 10101 / SPELL_WSTOMP = 11876.
-- Ported arms:
-- - OnEnterCombat(1): Arcing Smash 3s init -> 13s re-arm;
--   DoCast(me, 8374) non-triggered -> creature:CastSpell
--   (creature, 8374) (headless_horseman self-cast precedent).
-- - 1s health check (kalecgos "1s check" precedent — the C++
--   <50%/<25% legs run every UpdateAI tick): <50% one-shot
--   (per-guid latch, kirtonos/brutallus precedent) ->
--   DoCastVictim(10101, true) -> GetVictim + CastSpell
--   (mr_smite convention); <25% one-shot -> self-cast 11876.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The Knockaway leg's me->GetThreatManager().ResetThreat(me->
--   EnsureVictim()): no threat-table bridge in engine/scripting.
-- - GetUldamanAI -> GetInstanceAI leg: instance-script model
--   blocked (standing).
-- - The instance-side ActivateIronaya choreography
--   (SetFaction/RemoveFlag NOT_SELECTABLE/ROOT off/MovePoint to
--   IronayaPoint/SAY_AGGRO Talk) lives in instance_uldaman.cpp,
--   which stays DOCUMENTED-BLOCKED (instance-script model,
--   standing).

local NPC_IRONAYA = 7228

local SPELL_ARCINGSMASH = 8374
local SPELL_KNOCKAWAY = 10101
local SPELL_WSTOMP = 11876

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    state[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function healthPct(creature)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return 100
    end
    return creature:GetHealth() * 100 / max
end

local function onArcingSmash(creature, guid)
    creature:CastSpell(creature, SPELL_ARCINGSMASH)
    schedule(guid, "arcing", 13000, function() onArcingSmash(creature, guid) end)
end

local function onHealthCheck(creature, guid)
    local st = state[guid]
    if st then
        if not st.knockaway and healthPct(creature) < 50 then
            local victim = creature:GetVictim()
            if victim then
                creature:CastSpell(victim, SPELL_KNOCKAWAY, true)
                st.knockaway = true
            end
        end
        if not st.wstomp and healthPct(creature) < 25 then
            creature:CastSpell(creature, SPELL_WSTOMP)
            st.wstomp = true
        end
    end
    schedule(guid, "health", 1000, function() onHealthCheck(creature, guid) end)
end

local function onIronayaCombatStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { knockaway = false, wstomp = false }
    schedule(guid, "arcing", 3000, function() onArcingSmash(creature, guid) end)
    schedule(guid, "health", 1000, function() onHealthCheck(creature, guid) end)
end

local function onIronayaCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(NPC_IRONAYA, 1, onIronayaCombatStart)
RegisterCreatureEvent(NPC_IRONAYA, 2, onIronayaCombatEnd)
RegisterCreatureEvent(NPC_IRONAYA, 4, onIronayaCombatEnd)
RegisterCreatureEvent(NPC_IRONAYA, 23, onIronayaCombatEnd)

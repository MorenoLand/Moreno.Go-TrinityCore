-- Ironaya (Uldaman)
-- Lua port of src/server/scripts/EasternKingdoms/Uldaman/
-- boss_ironaya.cpp
-- (boss_ironaya : public ScriptedAI (NOT BossAI — constructor
-- Initialize() zeroes uiArcingTimer 3000 / bHasCastKnockaway /
-- bHasCastWstomp; the AI holds no InstanceScript* — the header's
-- old "instance = creature->GetInstanceScript()" line was false,
-- zero `instance` hits in boss_ironaya.cpp);
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
-- DoMeleeAttackIfReady. C++ has no UNIT_STATE_CASTING gate (zero
-- HasUnitState hits in boss_ironaya.cpp), so no defer applies and
-- timers fire unconditionally, matching C++ (the "no bridge"
-- phrasing is stale — IsCasting is live at
-- lua_creature_events.go:2275).
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
--   (mr_smite convention) + ResetThreat(victim) via
--   ModifyThreatByPercent(victim, -100); <25% one-shot -> self-cast 11876.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The Knockaway leg's me->GetThreatManager().ResetThreat(me->
--   EnsureVictim()) is PORTED (ModifyThreatByPercent(victim, -100)
--   = ScaleThreat x0 = ThreatManager::ResetThreat, bridged;
--   the old "no threat-table bridge" claim was stale).
-- - GetUldamanAI -> GetInstanceAI leg: instance-script model
--   blocked (standing).
-- - The instance-side ActivateIronaya choreography
--   (SetFaction/RemoveFlag NOT_SELECTABLE/ROOT off/MovePoint to
--   IronayaPoint/SAY_AGGRO Talk) is PORTED in uldaman.lua as the
--   go_keystone_chamber (124371) gossip chain (27s timer, entry
--   C++-verified in uldaman.h:38); the ironaya-side spawn freeze
--   below is its counterpart. C++ freezes Ironaya on spawn only
--   while encounter[2] != DONE (instance_uldaman.cpp:471-473) —
--   Lua has no persisted encounter state, so the freeze applies
--   on every spawn (documented delta; a fresh instance always
--   matches).
-- Ported arms (instance-side):
-- - OnSpawn(5): SetFrozenState (instance_uldaman.cpp:150-158) ->
--   FACTION_FRIENDLY + RemoveAllAuras + SetFlag(59, 0x02000000)
--   + SetRooted(true) + AddAura(10255 SPELL_MINION_FREEZE_ANIM).

local NPC_IRONAYA = 7228

local SPELL_ARCINGSMASH = 8374
local SPELL_KNOCKAWAY = 10101
local SPELL_WSTOMP = 11876
local SPELL_MINION_FREEZE_ANIM = 10255

local FACTION_FRIENDLY = 35

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ HealthBelowPct(pct) = GetHealth() < CountPctFromMaxHealth(pct),
-- and this tree's CalculatePct (Util.h:66) is T(base * (float)pct / 100)
-- truncated to integer — i.e. floor(max*pct/100). A float
-- health*100/max < pct comparison fires up to 1 HP early at fractional
-- boundaries (max=999, pct=50: C++ fires at health < 499, float fires at
-- health <= 499); compare against the floored integer threshold instead.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

local function onArcingSmash(creature, guid)
    creature:CastSpell(creature, SPELL_ARCINGSMASH)
    schedule(guid, "arcing", 13000, function() onArcingSmash(creature, guid) end)
end

local function onHealthCheck(creature, guid)
    local st = state[guid]
    if st then
        if not st.knockaway and healthBelowPct(creature, 50) then
            local victim = creature:GetVictim()
            if victim then
                creature:CastSpell(victim, SPELL_KNOCKAWAY, true)
                creature:ModifyThreatByPercent(victim, -100)
                st.knockaway = true
            end
        end
        if not st.wstomp and healthBelowPct(creature, 25) then
            creature:CastSpell(creature, SPELL_WSTOMP)
            st.wstomp = true
        end
    end
    schedule(guid, "health", 1000, function() onHealthCheck(creature, guid) end)
end

local function onIronayaCombatStart(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = { knockaway = false, wstomp = false }
    schedule(guid, "arcing", 3000, function() onArcingSmash(creature, guid) end)
    schedule(guid, "health", 1000, function() onHealthCheck(creature, guid) end)
end

local function onIronayaCombatEnd(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ instance_uldaman.cpp OnCreatureCreate case 7228 ->
-- SetFrozenState (instance_uldaman.cpp:150-158).
local function onIronayaSpawn(event, creature)
    creature:SetFaction(FACTION_FRIENDLY)
    creature:RemoveAllAuras()
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetRooted(true)
    creature:AddAura(SPELL_MINION_FREEZE_ANIM)
end

RegisterCreatureEvent(NPC_IRONAYA, 1, onIronayaCombatStart)
RegisterCreatureEvent(NPC_IRONAYA, 2, onIronayaCombatEnd)
RegisterCreatureEvent(NPC_IRONAYA, 4, onIronayaCombatEnd)
RegisterCreatureEvent(NPC_IRONAYA, 5, onIronayaSpawn)
RegisterCreatureEvent(NPC_IRONAYA, 23, onIronayaCombatEnd)

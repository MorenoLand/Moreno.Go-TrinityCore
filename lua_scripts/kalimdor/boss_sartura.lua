-- Temple of Ahn'Qiraj: Battleguard Sartura — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_sartura.cpp
-- (boss_sarturaAI : public BossAI(creature, DATA_SARTURA);
-- GetAI via GetAQ40AI<boss_sarturaAI>(creature) (AQ40ScriptName
-- "instance_temple_of_ahnqiraj" gate); AddSC_boss_sartura at end
-- registers boss_sartura + npc_sartura_royal_guard +
-- at_aq_battleguard_sartura; kalimdor loader decl 92 / call per
-- kalimdor_script_loader.cpp — sixth "// Temple of ahn'qiraj"
-- loader group, after bug_trio, before skeram).
-- Whole-server-tree grep confirms the cpp as the sole source of
-- "boss_sartura" (loader decl/call lines only otherwise).
-- Entry: Battleguard Sartura = 15516 (temple_of_ahnqiraj.h:76
-- NPC_SARTURA = 15516; creature_template ScriptName binding stays
-- DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Whirlwind (30s init -> 25-40s repeat): self-cast
--   SPELL_WHIRLWIND 26083; whirlwind mode 15s (DoMeleeAttackIfReady
--   suppressed while WhirlWind — engine tick owns melee).
-- - Enrage <20% health (latched): event 9 projected-health check
--   (jeklik convention), self-cast SPELL_ENRAGE 28747. C++ also
--   gates on !me->IsNonMeleeSpellCast(false) — no bridge.
-- - Hard enrage at 10min (latched): self-cast SPELL_ENRAGEHARD
--   28798.
-- - Talk: SAY_AGGRO 0 on engage, SAY_SLAY 1 on kill, SAY_DEATH 2
--   on death.
-- Unmodeled (documented-only, no bridges):
-- - BossAI ctor leg DATA_SARTURA (temple_of_ahnqiraj.h:31 = 1)
--   and _Reset() — instance bridge, standing.
-- - Whirlwind random-target switch (WhirlWindRandom 3-7s:
--   SelectTarget Random 100y + AddThreat + AttackStart) and the
--   AggroReset arms (45-55s -> 2-5s re-arm, AggroResetEnd 5s ->
--   35-45s): no SelectTarget/AttackStart/ResetThreatList bridge
--   (buru precedent — target switching unmodeled).
-- - at_aq_battleguard_sartura (AreaTriggerScript 4052):
--   GetInstanceScript + GetCreature(DATA_SARTURA) + AttackStart —
--   no at/InstanceScript bridge (document-only, like the ruins
--   instance).
-- - npc_sartura_royal_guard: Whirlwind 26038 self-cast (30s init
--   -> 25-40s, 15s whirlwind mode) and its KnockBack arm — no
--   creature entry constant anywhere in the C++ tree (only the
--   ScriptName registration), so the entry is unverifiable and
--   left unbound per identifier rules. C++ verbatim bug note: the
--   KnockBack_Timer (10s -> 10-20s) leg calls
--   DoCast(me, SPELL_WHIRLWINDADD) — the KnockBack 26027 constant
--   is declared but never used.
local ENTRY = 15516

local SPELL_WHIRLWIND = 26083
local SPELL_ENRAGE = 28747
local SPELL_ENRAGEHARD = 28798

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local timers = {}
local flags = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    flags[guid] = nil
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

local function st(guid)
    local s = flags[guid]
    if not s then
        s = { whirlwind = false, enraged = false, hardEnraged = false }
        flags[guid] = s
    end
    return s
end

-- C++ WhirlWind_Timer: !WhirlWind -> DoCast(me, 26083),
-- WhirlWind = true, WhirlWindEnd_Timer = 15000 ->
-- urand(25000,40000). Init 30000.
-- The per-3-7s random-target switch during whirlwind is no-bridge
-- (SelectTarget/AttackStart).
local function endWhirlwind(creature, guid)
    local s = st(guid)
    s.whirlwind = false
    schedule(guid, "whirlwind", math.random(25000, 40000), function()
        startWhirlwind(creature, guid)
    end)
end

function startWhirlwind(creature, guid)
    local s = st(guid)
    s.whirlwind = true
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind_end", 15000, function()
        endWhirlwind(creature, guid)
    end)
end

-- C++ Hard enrage: EnrageHard_Timer = 600000 -> DoCast(me, 28798),
-- latched.
local function onHardEnrage(creature, guid)
    local s = st(guid)
    if not s.hardEnraged then
        s.hardEnraged = true
        creature:CastSpell(creature, SPELL_ENRAGEHARD)
    end
end

-- C++ UpdateAI per-tick: !Enraged && !HealthAbovePct(20) &&
-- !me->IsNonMeleeSpellCast(false) -> DoCast(me, 28747) + latched.
-- Eluna event 9 is pre-damage, so projected-health check
-- (huhuran/jeklik convention). IsNonMeleeSpellCast: no bridge.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local s = st(guid)
    if s.enraged then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (health - damage) * 100 < 20 * maxHealth then
        s.enraged = true
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    st(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "whirlwind", 30000, function()
        startWhirlwind(creature, guid)
    end)
    schedule(guid, "hardenrage", 600000, function()
        onHardEnrage(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function onDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)

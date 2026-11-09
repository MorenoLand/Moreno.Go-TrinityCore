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
--   gates on !me->IsNonMeleeSpellCast(false) — PORTED as a 500ms
--   IsCasting defer (doan defer-before-latch-read precedent;
--   IsCasting is the live casting-state twin).
-- - Hard enrage at 10min (latched): self-cast SPELL_ENRAGEHARD
--   28798.
-- - Talk: SAY_AGGRO 0 on engage, SAY_SLAY 1 on kill, SAY_DEATH 2
--   on death.
-- Unmodeled (documented-only, no bridges):
-- - BossAI ctor leg DATA_SARTURA (temple_of_ahnqiraj.h:31 = 1)
--   and _Reset() PORTED as the ev1 IN_PROGRESS + ev23 NOT_STARTED
--   legs (see onEnterCombat / onReset); ev4 DONE PORTED below.
-- - Whirlwind random-target switch (WhirlWindRandom 3-7s:
--   SelectTarget Random 100y + AddThreat + AttackStart) and the
--   AggroReset arms (45-55s -> 2-5s re-arm, AggroResetEnd 5s ->
--   35-45s): PORTED 2026-10-24 (EngageWithTarget is the player-target
--   AttackStart twin; the "no AttackStart bridge" claim is retracted).
--   Deviations: the AggroReset countdown freezes while WhirlWind in C++
--   (the branch sits under !WhirlWind); the Lua tick re-checks after 5s
--   instead of freezing. The whirlwind-random tick self-terminates on
--   s.whirlwind == false, so no cancel is needed at whirlwind end.
-- - at_aq_battleguard_sartura (AreaTriggerScript 4052):
--   GetInstanceScript + GetCreature(DATA_SARTURA) + AttackStart —
--   PORTED 2026-10-26 via RegisterServerEvent(24): the trigger ID is
--   the C++ file's own comment, the server event fires for every
--   valid in-radius trigger (areatrigger.go:64), and the instance
--   lookup is substituted by the room-scale FindNearestCreature
--   idiom (500y, skadi/eregos precedent); C++ has no gates, so the
--   handler adds none.
-- - npc_sartura_royal_guard: Whirlwind 26038 self-cast (30s init
--   -> 25-40s, 15s whirlwind mode) and its KnockBack arm — no
--   creature entry constant anywhere in the C++ tree (only the
--   ScriptName registration), so the entry is unverifiable and
--   left unbound per identifier rules. C++ verbatim bug note: the
--   KnockBack_Timer (10s -> 10-20s) leg calls
--   DoCast(me, SPELL_WHIRLWINDADD) — the KnockBack 26027 constant
--   is declared but never used.
local DATA_SARTURA = 1 -- temple_of_ahnqiraj.h:31
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
        s = { whirlwind = false, enraged = false, hardEnraged = false, aggroReset = false }
        flags[guid] = s
    end
    return s
end

-- C++ WhirlWind_Timer: !WhirlWind -> DoCast(me, 26083),
-- WhirlWind = true, WhirlWindEnd_Timer = 15000 ->
-- urand(25000,40000). Init 30000.
-- The per-3-7s random-target switch during whirlwind is PORTED below
-- (whirlwindRandomTick: AddThreat 1.0 + EngageWithTarget).
local startWhirlwind, whirlwindRandomTick, aggroResetTick, aggroResetEnd
local function endWhirlwind(creature, guid)
    local s = st(guid)
    s.whirlwind = false
    schedule(guid, "whirlwind", math.random(25000, 40000), function()
        startWhirlwind(creature, guid)
    end)
end

startWhirlwind = function(creature, guid)
    local s = st(guid)
    s.whirlwind = true
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind_end", 15000, function()
        endWhirlwind(creature, guid)
    end)
    schedule(guid, "whirlwind_random", math.random(3000, 7000), function()
        whirlwindRandomTick(creature, guid)
    end)
end

-- C++ SelectTarget(Random, 1, 100.0f, true): UnitAI.h builds the list
-- victim-first, so offset 1 removes the victim, not the threat head;
-- then threat-list players alive within 100 yd.
local function randomThreatPlayer100SkipFirst(creature)
    local players = {}
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local aliveSeen = 0
    for _, p in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = p:GetGUID() == victimGuid
        elseif not p:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and p:GetObjectType() == "Player" and not p:IsDead()
                and creature:GetDistance(p) - creature:GetCombatReach() - 1.5 < 100 then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ WhirlWindRandom / AggroReset shared body (boss_sartura.cpp:124-127,
-- 151-154): AddThreat(target, 1.0) + AttackStart(target); EngageWithTarget
-- is the player-target AttackStart twin (a non-player C++ pick no-ops
-- through its session gate, so the engage fires exactly for player picks).
local function randomTargetSwitch(creature)
    local target = randomThreatPlayer100SkipFirst(creature)
    if target then
        creature:AddThreat(target, 1.0)
        creature:EngageWithTarget(target)
    end
end

whirlwindRandomTick = function(creature, guid)
    local s = st(guid)
    if not s.whirlwind then
        return
    end
    randomTargetSwitch(creature)
    schedule(guid, "whirlwind_random", math.random(3000, 7000), function()
        whirlwindRandomTick(creature, guid)
    end)
end

aggroResetTick = function(creature, guid)
    local s = st(guid)
    if s.whirlwind then
        schedule(guid, "aggro_reset", 5000, function()
            aggroResetTick(creature, guid)
        end)
        return
    end
    randomTargetSwitch(creature)
    if not s.aggroReset then
        s.aggroReset = true
        schedule(guid, "aggro_reset_end", 5000, function()
            aggroResetEnd(creature, guid)
        end)
    end
    schedule(guid, "aggro_reset", math.random(2000, 5000), function()
        aggroResetTick(creature, guid)
    end)
end

aggroResetEnd = function(creature, guid)
    local s = st(guid)
    s.aggroReset = false
    local per = timers[guid]
    if per and per["aggro_reset"] then
        RemoveEventById(per["aggro_reset"])
        per["aggro_reset"] = nil
    end
    schedule(guid, "aggro_reset", math.random(35000, 45000), function()
        aggroResetTick(creature, guid)
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
-- (huhuran/jeklik convention). The casting gate is PORTED as a
-- 500ms IsCasting defer (doan defer-before-latch-read precedent):
-- while casting, re-check every 500ms against current health, so a
-- heal back above 20% before a non-casting tick correctly never
-- fires it. Cancelled with the timers on 2/4/23.
-- !HealthAbovePct(20) fires AT exactly 20%: projected health <= floor(20M/100).
-- C++ is UpdateAI post-damage, so it never fires on the killing blow;
-- the postDamage > 0 guard (nefarian rule) keeps the port from enraging
-- while dying.
local function checkEnrageLatch(creature, guid, health)
    local s = st(guid)
    if s.enraged then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "enrage_check", 500, function()
            checkEnrageLatch(creature, guid, creature:GetHealth())
        end)
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if health > 0 and health <= math.floor(20 * maxHealth / 100) then
        s.enraged = true
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local s = st(guid)
    if s.enraged then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    checkEnrageLatch(creature, guid, health - damage)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    st(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith: Talk then BossAI::JustEngagedWith
    -- (boss_sartura.cpp:96-100) — SetBossState(DATA_SARTURA, IN_PROGRESS).
    creature:SetBossState(DATA_SARTURA, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    schedule(guid, "whirlwind", 30000, function()
        startWhirlwind(creature, guid)
    end)
    schedule(guid, "hardenrage", 600000, function()
        onHardEnrage(creature, guid)
    end)
    schedule(guid, "aggro_reset", math.random(45000, 55000), function()
        aggroResetTick(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset _Reset() leg (boss_sartura.cpp:90-94): alive-gated and
    -- != DONE-gated (ScriptedCreature.cpp:499-511; moroes/algalon precedent).
    if creature:IsAlive() and creature:GetBossState(DATA_SARTURA) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_SARTURA, BOSS_STATE_NOT_STARTED)
    end
end

local function onTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied: Talk then _JustDied() (boss_sartura.cpp:102-106) —
-- ev4 DONE leg in C++ order.
local function onDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_SARTURA, BOSS_STATE_DONE)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- at_aq_battleguard_sartura (boss_sartura.cpp:319-333): trigger 4052
-- engages Sartura on the triggering player (C++ order: instance
-- GetCreature(DATA_SARTURA), then AttackStart(player); no gates).
local function onAreaTrigger(event, player, triggerID)
    if triggerID ~= 4052 then
        return
    end
    local sartura = player:FindNearestCreature(ENTRY, 500)
    if sartura then
        sartura:EngageWithTarget(player)
    end
end

RegisterServerEvent(24, onAreaTrigger)

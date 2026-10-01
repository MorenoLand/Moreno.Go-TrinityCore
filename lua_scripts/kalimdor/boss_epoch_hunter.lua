-- Escape from Durnholde Keep: Epoch Hunter — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/EscapeFromDurnholdeKeep/
-- boss_epoch_hunter.cpp (151 lines; boss_epoch_hunterAI :
-- public ScriptedAI; GetAI via GetOldHillsbradAI<boss_epoch_hunterAI>
-- (OHScriptName "instance_old_hillsbrad" gate); AddSC_boss_epoch_hunter
-- at end registers the one script; kalimdor loader decl 36 / call 149
-- per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_epoch_hunter" (loader decl/call lines only otherwise).
-- Entry: old_hillsbrad.cpp:174 names ENTRY_EPOCH = 18096 and :615
-- summons it (Taretha's gossip summons the Epoch Hunter at 2639.13,
-- 698.55, 65.43 during the Thrall escort; old_hillsbrad.h:37 names
-- DATA_EPOCH = 9) — the name-to-entry tie is C++-verified; the
-- creature_template ScriptName binding stays DB-side. Not GUID-bound in
-- instance_old_hillsbrad.cpp (gossip-summoned, not creatureData-bound).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (1) + arm each timer at
--   its C++ constructor-init cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm,
--   hyjal.lua convention).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_SLAY (2); no TYPEID gate in C++ (shade_of_aran precedent).
-- - Death (C++ JustDied, non-instance leg): Talk SAY_DEATH (4).
-- - Sand Breath 31914 on the victim (C++ DoCastVictim — jeklik GetVictim
--   + CastSpell convention), non-triggered, init {8s,16s} ->
--   {10s,20s} + Talk SAY_BREATH (3) unconditionally.
-- - Impending Death 31916 on the victim (C++ DoCastVictim), non-triggered,
--   init {25s,30s} -> 25000 + rand32() % 5000.
-- - Wing Buffet 31475 on SelectTargetMethod::Random, 0 with no range cap
--   -> unbounded random alive player in the instance (nefarian
--   randomAlivePlayer convention), non-triggered, init 35s ->
--   25000 + rand32() % 10000.
-- - Magic Disruption Aura 33834 self-cast (C++ DoCast(me)), non-triggered,
--   init 40s -> 15s.
-- Unmodeled (documented-only, no bridges):
-- - Sand Breath's C++ pre-cast me->InterruptNonMeleeSpells(false) arm
--   has no spell-interrupt bridge (maiden precedent).
-- - JustDied's instance leg: if instance->GetData(TYPE_THRALL_EVENT
--   = 2) == IN_PROGRESS then instance->SetData(TYPE_THRALL_PART4 = 5,
--   DONE) — instance-data bridge blocked (standing); the Thrall escort
--   AI (old_hillsbrad.cpp) is a separate AddSC unit.
-- - SAY_ENTER 0 is declared in the C++ enum but never Talked in C++ (the
--   archimonde SAY_SOUL_CHARGE case) — no Talk arm exists to port.
-- - The SD%Complete: 60 comment's missing pre-event spawns and the
--   uncoordinated Thrall-escort speech need summon and EscortAI
--   bridges that do not exist.

local ENTRY = 18096

local SAY_ENTER  = 0
local SAY_AGGRO  = 1
local SAY_SLAY   = 2
local SAY_BREATH = 3
local SAY_DEATH  = 4

local SPELL_SAND_BREATH           = 31914
local SPELL_IMPENDING_DEATH       = 31916
local SPELL_WING_BUFFET           = 31475
local SPELL_MAGIC_DISRUPTION_AURA = 33834

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

-- nefarian convention: unbounded random alive player in the instance.
local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SandBreath_Timer: DoCastVictim(31914), non-triggered, init
-- {8s,16s} -> {10s,20s} + Talk SAY_BREATH (3) unconditionally (C++
-- talks even on a nil SelectTarget — same arm as anetheron's Swarm);
-- jeklik GetVictim + CastSpell convention.
local function onSandBreath(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_SAND_BREATH)
    end
    creature:Talk(SAY_BREATH)
    schedule(guid, "sandbreath", math.random(10000, 20000), function()
        onSandBreath(creature, guid)
    end)
end

-- C++ ImpendingDeath_Timer: DoCastVictim(31916), non-triggered, init
-- {25s,30s} -> 25000 + rand32() % 5000.
local function onImpendingDeath(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_IMPENDING_DEATH)
    end
    schedule(guid, "impendingdeath", 25000 + math.random(0, 4999), function()
        onImpendingDeath(creature, guid)
    end)
end

-- C++ WingBuffet_Timer: DoCast(SelectTarget(Random, 0), 31475),
-- non-triggered, init 35s -> 25000 + rand32() % 10000.
local function onWingBuffet(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_WING_BUFFET)
    end
    schedule(guid, "wingbuffet", 25000 + math.random(0, 9999), function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ Mda_Timer: DoCast(me, 33834), non-triggered, init 40s -> 15s.
local function onMagicDisruptionAura(creature, guid)
    creature:CastSpell(creature, SPELL_MAGIC_DISRUPTION_AURA)
    schedule(guid, "mda", 15000, function()
        onMagicDisruptionAura(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "sandbreath", math.random(8000, 16000), function()
        onSandBreath(creature, guid)
    end)
    schedule(guid, "impendingdeath", math.random(25000, 30000), function()
        onImpendingDeath(creature, guid)
    end)
    schedule(guid, "wingbuffet", 35000, function()
        onWingBuffet(creature, guid)
    end)
    schedule(guid, "mda", 40000, function()
        onMagicDisruptionAura(creature, guid)
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
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_SLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

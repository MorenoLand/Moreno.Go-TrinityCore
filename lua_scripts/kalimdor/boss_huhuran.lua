-- Temple of Ahn'Qiraj: Huhuran — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_huhuran.cpp
-- (boss_huhuranAI : public BossAI(creature, DATA_HUHURAN);
-- GetAI via GetAQ40AI<boss_huhuranAI>(creature) (AQ40ScriptName
-- "instance_temple_of_ahnqiraj" gate); AddSC_boss_huhuran at end
-- registers the boss script; kalimdor loader decl 90 / call per
-- kalimdor_script_loader.cpp — fourth "// Temple of ahn'qiraj"
-- loader group, after fankriss, before bug_trio).
-- Whole-server-tree grep confirms the cpp as the sole source of
-- "boss_huhuran" (loader decl/call lines only otherwise).
-- Entry: Huhuran = 15509 (armory npc=15509; consistent with the
-- temple_of_ahnqiraj.h sequence NPC_FANKRISS-adjacent NPC_KRI =
-- 15511; creature_template ScriptName binding stays DB-side).
-- No NPC_HUHURAN constant in temple_of_ahnqiraj.h.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Frenzy (25-35s): self-cast FRENZY 26051 + Talk(EMOTE_FRENZY_KILL 0);
--   Frenzy flag latched; FrenzyBack arm re-armed at 15s; on frenzy,
--   PoisonBolt leg armed at 3s (C++ sets PoisonBolt_Timer = 3000).
-- - FrenzyBack (15s): when Frenzy true -> clear Frenzy. C++ also calls
--   me->InterruptNonMeleeSpells(false) — no bridge (archimonde/azgalor
--   precedent — unmodeled); C++ uint32 underflow freezes the arm while
--   Frenzy is false, so the 15s repeat arm here is a documented
--   simplification.
-- - Wyvern Sting 26180 (18-28s -> 15-32s): random player via
--   GetPlayersInWorld (maiden_of_virtue convention — C++ SelectTarget
--   Random has no bridge).
-- - Acid Spit 26050 (8s -> 5-10s): DoCastVictim (jeklik GetVictim +
--   CastSpell convention).
-- - Noxious Poison 26053 (10-20s -> 12-24s): DoCastVictim.
-- - Poison Bolt 26052: C++-exact gate — fires only while Frenzy or
--   Berserk is true; without the flag the C++ timer is frozen (never
--   decremented), so the handler re-arms as a poll until a flag sets.
--   While frenzy/berserk: DoCastVictim, 3s repeat.
-- - Berserk (event 9 damage leg, projected health <31%, latched):
--   Talk(EMOTE_BERSERK 1) + self-cast BERSERK 26068. C++ also calls
--   me->InterruptNonMeleeSpells(false) — no bridge.
-- Unmodeled (documented-only, no bridges):
-- - BossAI ctor leg DATA_HUHURAN (temple_of_ahnqiraj.h:33 = 3) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15509

local SPELL_FRENZY = 26051
local SPELL_BERSERK = 26068
local SPELL_POISONBOLT = 26052
local SPELL_NOXIOUSPOISON = 26053
local SPELL_WYVERNSTING = 26180
local SPELL_ACIDSPIT = 26050

local EMOTE_FRENZY_KILL = 0
local EMOTE_BERSERK = 1

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
        s = { frenzy = false, berserk = false }
        flags[guid] = s
    end
    return s
end

-- C++ Frenzy_Timer: !Frenzy -> DoCast(me, 26051), Talk(0),
-- Frenzy = true, PoisonBolt_Timer = 3000, -> urand(25000,35000).
local function onFrenzy(creature, guid)
    local s = st(guid)
    if not s.frenzy then
        creature:CastSpell(creature, SPELL_FRENZY)
        creature:Talk(EMOTE_FRENZY_KILL)
        s.frenzy = true
        schedule(guid, "poisonbolt", 3000, function()
            onPoisonBolt(creature, guid)
        end)
    end
    schedule(guid, "frenzy", math.random(25000, 35000), function()
        onFrenzy(creature, guid)
    end)
end

-- C++ FrenzyBack_Timer 15s: Frenzy -> InterruptNonMeleeSpells (no
-- bridge) + Frenzy = false; -> 15000.
local function onFrenzyBack(creature, guid)
    local s = st(guid)
    if s.frenzy then
        s.frenzy = false
    end
    schedule(guid, "frenzyback", 15000, function()
        onFrenzyBack(creature, guid)
    end)
end

-- C++ Wyvern_Timer: SelectTarget Random -> DoCast(target, 26180),
-- init urand(18000,28000) -> urand(15000,32000).
local function onWyvern(creature, guid)
    local cands = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p and p:IsAlive() then
            cands[#cands + 1] = p
        end
    end
    if #cands > 0 then
        local target = cands[math.random(1, #cands)]
        creature:CastSpell(target, SPELL_WYVERNSTING)
    end
    schedule(guid, "wyvern", math.random(15000, 32000), function()
        onWyvern(creature, guid)
    end)
end

-- C++ Spit_Timer: DoCastVictim(26050), init 8000 -> urand(5000,10000).
local function onSpit(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ACIDSPIT)
    end
    schedule(guid, "spit", math.random(5000, 10000), function()
        onSpit(creature, guid)
    end)
end

-- C++ NoxiousPoison_Timer: DoCastVictim(26053),
-- init urand(10000,20000) -> urand(12000,24000).
local function onNoxious(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_NOXIOUSPOISON)
    end
    schedule(guid, "noxious", math.random(12000, 24000), function()
        onNoxious(creature, guid)
    end)
end

-- C++ PoisonBolt_Timer: fires only while Frenzy or Berserk;
-- DoCastVictim(26052), -> 3000. Init 4000.
function onPoisonBolt(creature, guid)
    local s = st(guid)
    if s.frenzy or s.berserk then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_POISONBOLT)
        end
        schedule(guid, "poisonbolt", 3000, function()
            onPoisonBolt(creature, guid)
        end)
    else
        -- Frozen-timer equivalent: re-arm as a quiet poll until a
        -- frenzy/berserk flag sets.
        schedule(guid, "poisonbolt", 1000, function()
            onPoisonBolt(creature, guid)
        end)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    st(guid)
    schedule(guid, "frenzy", math.random(25000, 35000), function()
        onFrenzy(creature, guid)
    end)
    schedule(guid, "frenzyback", 15000, function()
        onFrenzyBack(creature, guid)
    end)
    schedule(guid, "wyvern", math.random(18000, 28000), function()
        onWyvern(creature, guid)
    end)
    schedule(guid, "spit", 8000, function()
        onSpit(creature, guid)
    end)
    schedule(guid, "noxious", math.random(10000, 20000), function()
        onNoxious(creature, guid)
    end)
    schedule(guid, "poisonbolt", 4000, function()
        onPoisonBolt(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ has no JustDied override for boss_huhuranAI — cancel only.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ UpdateAI per-tick: !Berserk && HealthBelowPct(31) ->
-- InterruptNonMeleeSpells (no bridge) + Talk(EMOTE_BERSERK 1) +
-- DoCast(me, 26068) + Berserk = true (latched). Eluna event 9 is
-- pre-damage, so projected-health check (jeklik/aku_mai convention).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local s = st(guid)
    if s.berserk then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (health - damage) * 100 < 31 * maxHealth then
        s.berserk = true
        creature:Talk(EMOTE_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- AV Marshal / Warmaster (Alterac Valley) --
-- Lua port of src/server/scripts/EasternKingdoms/AlteracValley/
-- alterac_valley.cpp (npc_av_marshal_or_warmasterAI — combat scheduler +
-- Reset aura arm). Zone-script unit per eastern_kingdoms_script_loader.cpp
-- order (head of the Eastern Kingdoms pass: AddSC_alterac_valley;
-- boss_balinda follows).
-- Entries (file's own Creatures enum, verifiable from the C++ sources):
-- 14762 north / 14763 south / 14765 stonehearth / 14764 icewing marshals,
-- 14773 iceblood / 14776 tower point / 14777 west frostwolf / 14772 east
-- frostwolf warmasters. The creature_template ScriptName binding is
-- DB-side (no TDB in this workspace), but the entries themselves are
-- C++-verifiable so the port is registered (storm_cloud precedent).
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ Reset() event schedule + UpdateAI event machine):
-- EVENT_CHARGE_TARGET: victim-cast CHARGE 22911, {2,12}s init -> {10,25}s
--   (C++ reschedules undefined EVENT_CHARGE at line 141 — an upstream typo
--   that would not compile; the port re-arms the charge chain at the
--   stated {10,25}s — documented deviation);
-- EVENT_CLEAVE: victim-cast CLEAVE 40504, {1,11}s init -> {10,16}s;
-- EVENT_DEMORALIZING_SHOUT: self-cast 23511, 2s init -> {10,15}s;
-- EVENT_WHIRLWIND: self-cast 13736, {5,20}s init -> {10,25}s;
-- EVENT_ENRAGE: self-cast ENRAGE 8599, {5,20}s init -> {10,30}s;
-- the Reset/JustAppeared one-shot aura self-cast from the _auraPairs table
-- (C++ fires it on the first UpdateAI tick after spawn/reset — modeled as
-- OnSpawn(5) + OnReset(23), storm_cloud convention):
--   14762->45828 / 14763->45829 / 14765->45830 / 14764->45831 /
--   14773->45822 / 14776->45823 / 14777->45824 / 14772->45826.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts are
-- packet-visual), so timers fire unconditionally (maiden precedent); the
-- C++ casting-state re-check inside the event loop is likewise dropped.
-- Unmodeled: EVENT_CHECK_RESET (home-position 2D distance > 50yd ->
-- EnterEvadeMode) — no HomePosition bridge on the luaMotionCreature
-- surface (X/Y/Z reads only; GetDistance2d needs two objects), so the
-- leash arm is documented-only; evade still re-arms through OnReset(23)
-- when the engine leashes the creature.

local SPELL_CHARGE = 22911
local SPELL_CLEAVE = 40504
local SPELL_DEMORALIZING_SHOUT = 23511
local SPELL_ENRAGE = 8599
local SPELL_WHIRLWIND = 13736

local AURA_PAIRS = {
    [14762] = 45828,
    [14763] = 45829,
    [14765] = 45830,
    [14764] = 45831,
    [14773] = 45822,
    [14776] = 45823,
    [14777] = 45824,
    [14772] = 45826,
}

local ENTRIES = { 14762, 14763, 14764, 14765, 14772, 14773, 14776, 14777 }

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onSpawnOrReset(_, creature)
    local spellId = AURA_PAIRS[creature:GetEntry()]
    if spellId then
        creature:CastSpell(creature, spellId, true)
    end
end

local function onCharge(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHARGE)
    end
    schedule(guid, "charge", {10000, 25000}, function() onCharge(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {10000, 16000}, function() onCleave(creature, guid) end)
end

local function onShout(creature, guid)
    creature:CastSpell(creature, SPELL_DEMORALIZING_SHOUT)
    schedule(guid, "shout", {10000, 15000}, function() onShout(creature, guid) end)
end

local function onWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {10000, 25000}, function() onWhirlwind(creature, guid) end)
end

local function onEnrage(creature, guid)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", {10000, 30000}, function() onEnrage(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "charge", {2000, 12000}, function() onCharge(creature, guid) end)
    schedule(guid, "cleave", {1000, 11000}, function() onCleave(creature, guid) end)
    schedule(guid, "shout", 2000, function() onShout(creature, guid) end)
    schedule(guid, "whirlwind", {5000, 20000}, function() onWhirlwind(creature, guid) end)
    schedule(guid, "enrage", {5000, 20000}, function() onEnrage(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
    onSpawnOrReset(_, creature)
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 5, onSpawnOrReset)
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, onCombatEnd)
    RegisterCreatureEvent(entry, 4, onCombatEnd)
    RegisterCreatureEvent(entry, 23, onReset)
end

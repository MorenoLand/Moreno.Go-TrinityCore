-- Maraudon: Noxxion — Lua port of
-- src/server/scripts/Kalimdor/Maraudon/boss_noxxion.cpp
-- (SD%Complete: 100; class boss_noxxion : public CreatureScript {
-- boss_noxxionAI : public ScriptedAI }; GetAI via
-- GetMaraudonAI<boss_noxxionAI>; AddSC_boss_noxxion at end registers
-- the one script; kalimdor loader decl 62 / call 175 per
-- kalimdor_script_loader.cpp — the third "//Maraudon" loader group,
-- right after AddSC_boss_landslide()).
-- Entry: maraudon.h carries no NPC_ constants and the instance file
-- never names the boss — 13282 is wowhead-cited for "Noxxion"
-- (https://www.wowhead.com/classic/npc=13282/noxxion; no NPC_ constant
-- in the C++ tree; celebras / vishas / gelihast precedent); the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (C++ JustEngagedWith is empty — no Talk): arm each timer at
--   its C++ Initialize() cooldown (hyjal.lua convention; Reset's
--   Initialize() timer-reset half collapses into the engage re-arm).
-- - Toxic Volley 21687 on the victim (C++ DoCastVictim — jeklik
--   GetVictim + CastSpell convention), non-triggered (C++ 2-arg cast),
--   init 7s -> 9s.
-- - Uppercut 22916 on the victim, non-triggered, init 16s -> 12s.
-- - Invisible/adds phase (C++ AddsTimer init 19s -> 40s, armed on
--   engage; AddsTimer ticks only while visible, the 15s invisible
--   window freezes ALL timers via the C++ early return): SetFaction(35
--   FACTION_FRIENDLY), SetFlag(59 UNIT_FIELD_FLAGS, 0x02000000
--   NOT_SELECTABLE), SetDisplayId(11686 invisible model), 5x
--   SummonCreature(13456 at own pos + irand(-7,7) offsets, o, type 1 =
--   TIMED_OR_CORPSE_DESPAWN (C++ enum 2 minus the 0-based Lua scheme),
--   90000ms), then the 15s InvisibleTimer reappear arm: SetFaction(14
--   FACTION_MONSTER), RemoveFlag, SetDisplayId(11172 noxxion model),
--   re-arm adds at 40s (C++-exact: the re-armed 40s stays frozen through
--   the 15s), toxic volley / uppercut re-armed at full 9s / 12s
--   (documented skew vs the C++ frozen remainders, at most 9s / 12s).
-- Unmodeled (no bridges): InterruptNonMeleeSpells(false) at phase entry
--   (a cast in flight completes instead of interrupting); the adds'
--   AttackStart(victim) — the adds idle until the engine aggros them
--   (aran.lua water-elemental precedent). The C++ file declares zero
--   Talk lines and no KilledUnit override — no event-3 registration.

local ENTRY = 13282

local SPELL_TOXIC_VOLLEY = 21687
local SPELL_UPPERCUT     = 22916

local NPC_SPORE          = 13456
local DISPLAY_NOXXION    = 11172
local DISPLAY_INVISIBLE  = 11686
local FACTION_MONSTER    = 14
local FACTION_FRIENDLY   = 35
local UNIT_FIELD_FLAGS   = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- Forward declares: onAdds/onReappear reference each other through the
-- schedule closures (plain assignment, never re-`local function`).
local onAdds, onReappear

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

-- C++ ToxicVolley: DoCastVictim(21687), non-triggered, init 7s -> 9s.
local function onToxicVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_TOXIC_VOLLEY)
    end
    schedule(guid, "toxicvolley", 9000, function()
        onToxicVolley(creature, guid)
    end)
end

-- C++ Uppercut: DoCastVictim(22916), non-triggered, init 16s -> 12s.
local function onUppercut(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_UPPERCUT)
    end
    schedule(guid, "uppercut", 12000, function()
        onUppercut(creature, guid)
    end)
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ AddsTimer arm: InterruptNonMeleeSpells(false) has no bridge; the
-- rest is bridged. 5x DoSpawnCreature(13456, irand(-7,7), irand(-7,7),
-- 0, 0, TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN, 90s) + AttackStart(victim);
-- the AttackStart leg has no bridge (documented in the header).
onAdds = function(creature, guid)
    -- C++ freezes ALL timers while invisible (the early return skips the
    -- UpdateVictim block): freeze toxic volley / uppercut here; the adds
    -- timer itself was just re-armed and is re-scheduled on reappear.
    cancelKey(guid, "toxicvolley")
    cancelKey(guid, "uppercut")
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetDisplayId(DISPLAY_INVISIBLE)
    local x, y, z, o = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
    for _ = 1, 5 do
        creature:SummonCreature(NPC_SPORE,
            x + math.random(-7, 7), y + math.random(-7, 7), z, o, 1, 90000)
    end
    schedule(guid, "reappear", 15000, function()
        onReappear(creature, guid)
    end)
end

-- C++ InvisibleTimer: SetFaction(FACTION_MONSTER), RemoveFlag
-- (UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), SetDisplayId(11172).
onReappear = function(creature, guid)
    creature:SetFaction(FACTION_MONSTER)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetDisplayId(DISPLAY_NOXXION)
    -- AddsTimer was set to 40000 at the adds-arm fire and frozen through
    -- the 15s — re-arming 40s here is C++-exact; toxic volley / uppercut
    -- re-arm at full cadence (documented skew vs frozen remainders).
    schedule(guid, "toxicvolley", 9000, function()
        onToxicVolley(creature, guid)
    end)
    schedule(guid, "uppercut", 12000, function()
        onUppercut(creature, guid)
    end)
    schedule(guid, "adds", 40000, function()
        onAdds(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "toxicvolley", 7000, function()
        onToxicVolley(creature, guid)
    end)
    schedule(guid, "uppercut", 16000, function()
        onUppercut(creature, guid)
    end)
    schedule(guid, "adds", 19000, function()
        onAdds(creature, guid)
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
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- Felblood Kael'thas' Phoenix (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_felblood_kaelthas.cpp (npc_felblood_kaelthas_phoenixAI).
-- Entry 24674 'Phoenix', ScriptName npc_felblood_kaelthas_phoenix;
-- egg entry 24675 'Phoenix Egg', ScriptName '' (TDB VerifiedBuild
-- 12340, boss_felblood_kaelthas.lua header). Registered in
-- boss_ai.go (RegisterLuaBoss, boss_vaelastrasz precedent).
-- C++ arms (boss_felblood_kaelthas.cpp:386-480):
-- - Initialize (:390-394, ctor): REACT_PASSIVE + _isInEgg = false.
-- - IsSummonedBy (:396-402): DoZoneInCombat + self-cast BURN 44197
--   + self-cast REBIRTH 44196 + EVENT_ATTACK_PLAYERS 2s.
-- - DamageTaken (:406-433): lethal blow (damage >= health) with
--   !_isInEgg: AttackStop + REACT_PASSIVE + RemoveAllAuras +
--   SetFlag(NOT_SELECTABLE) + self-cast EMBER_BLAST 44199 +
--   DoSummon(PHOENIX_EGG 24675, own position, 0s) + instance->
--   GetCreature(DATA_KAELTHAS_SUNSTRIDER)->AI()->JustSummoned(egg)
--   + EVENT_HATCH_FROM_EGG 15s + _isInEgg = true; damage is then
--   ALWAYS rewritten to health-1 on a lethal blow, egg or not.
-- - SummonedCreatureDies (:435-439): egg destroyed within 15s ->
--   DespawnOrUnsummon.
-- - UpdateAI (:441-480): HATCH (:458-463): egg DespawnOrUnsummon +
--   RemoveAllAuras + EVENT_REBIRTH 2s; REBIRTH (:464-466): self-cast
--   44196 + EVENT_PREPARE_REENGAGE 2s; PREPARE_REENGAGE (:468-474):
--   _isInEgg = false + self-cast FULL_HEAL 17683 + self-cast BURN
--   44197 + RemoveFlag(NOT_SELECTABLE) + EVENT_ATTACK_PLAYERS 2s;
--   ATTACK_PLAYERS: REACT_AGGRESSIVE. JustEngagedWith is empty.
--   UpdateAI pre-loop CASTING gate (:444-445) is ported as 500ms
--   IsCasting defers under each timer arm's own key (felblood boss
--   file idiom).
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 5 OnSpawn
-- (IsSummonedBy twin — fires for temp summons, fireCreatureSpawned),
-- 9 OnDamageTaken, 21 OnSummonedCreatureDied, 23 OnReset. Driven by
-- CreateLuaEvent timers (felblood boss file idiom); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. DamageTaken returns (false, newDamage)
-- (felblood boss file / curator precedent).
-- Deviations from C++:
-- - The :422 cross-AI call (kaelthas->AI()->JustSummoned(egg)) has
--   no bridge, but for the egg entry it hits the default leg of
--   Kael'thas's JustSummoned (:251-268: summons.Summon bookkeeping
--   only; the sphere/flame-strike arms don't apply) — it only
--   registers the egg in Kael'thas's summons list for his
--   DespawnAll legs. The Go twin for that coverage is this file:
--   the egg is despawned from onLeaveCombat/onReset (Kael'thas
--   evade/reset DespawnAll twin), and the boss file latches
--   phoenixes (not eggs) for its own DespawnAll legs.
-- - EVENT_HATCH_FROM_EGG's ObjectAccessor::GetCreature(*me,
--   _eggGUID) (:459) has no bridge; the egg object returned by
--   SummonCreature is stored per-guid instead (SummonCreature
--   returns the object + stamps OwnerGUID, mimiron precedent).
-- - BURN 44197's periodic trigger 44198 is UNSEEDED (fetch
--   dispatched 2026-11-14) — the aura applies but ticks dormant
--   until 44198 seeds (marli precedent).
-- - The egg is summoned with type 5 CORPSE_TIMED_DESPAWN, 0ms
--   (C++ DoSummon default type + 0s: lives until killed or
--   manually despawned; corpse timer 0).

local SPELL_BURN = 44197
local SPELL_REBIRTH = 44196
local SPELL_EMBER_BLAST = 44199
local SPELL_FULL_HEAL = 17683

local NPC_PHOENIX_EGG = 24675 -- magisters_terrace.h

local ENTRY_PHOENIX = 24674

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local timers = {}
local inEgg = {}
local phoenixEgg = {} -- per-guid stored egg object (C++ _eggGUID twin)

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

-- C++ UpdateAI event chain (boss_felblood_kaelthas.cpp:458-474).
-- Cyclic (HATCH -> REBIRTH -> PREPARE -> ATTACK), so the handlers
-- are forward-declared and plain-assigned (tharon_ja / gal_darah
-- idiom — never re-local-function, malown time-bomb).
local onHatchFromEgg, onRebirth, onPrepareReengage, onAttackPlayers

-- C++ EVENT_ATTACK_PLAYERS: SetReactState(REACT_AGGRESSIVE).
onAttackPlayers = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "attack_players", 500, function() onAttackPlayers(creature, guid) end)
        return
    end
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

-- C++ EVENT_HATCH_FROM_EGG (:458-463): egg DespawnOrUnsummon +
-- RemoveAllAuras + EVENT_REBIRTH 2s.
onHatchFromEgg = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "hatch", 500, function() onHatchFromEgg(creature, guid) end)
        return
    end
    local egg = phoenixEgg[guid]
    if egg then
        egg:DespawnOrUnsummon()
        phoenixEgg[guid] = nil
    end
    creature:RemoveAllAuras()
    schedule(guid, "rebirth", 2000, function() onRebirth(creature, guid) end)
end

-- C++ EVENT_REBIRTH (:464-466): self-cast 44196 +
-- EVENT_PREPARE_REENGAGE 2s.
onRebirth = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rebirth", 500, function() onRebirth(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_REBIRTH)
    schedule(guid, "prepare_reengage", 2000, function() onPrepareReengage(creature, guid) end)
end

-- C++ EVENT_PREPARE_REENGAGE (:468-474): _isInEgg = false +
-- self-cast FULL_HEAL 17683 + self-cast BURN 44197 +
-- RemoveFlag(NOT_SELECTABLE) + EVENT_ATTACK_PLAYERS 2s.
onPrepareReengage = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "prepare_reengage", 500, function() onPrepareReengage(creature, guid) end)
        return
    end
    inEgg[guid] = false
    creature:CastSpell(creature, SPELL_FULL_HEAL)
    creature:CastSpell(creature, SPELL_BURN)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    schedule(guid, "attack_players", 2000, function() onAttackPlayers(creature, guid) end)
end

-- C++ IsSummonedBy (:396-402): DoZoneInCombat + self-cast BURN +
-- self-cast REBIRTH + EVENT_ATTACK_PLAYERS 2s. Initialize (:390-394)
-- runs first in C++ (ctor): REACT_PASSIVE + _isInEgg = false.
local function onSpawn(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    inEgg[guid] = false
    phoenixEgg[guid] = nil
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:DoZoneInCombat()
    creature:CastSpell(creature, SPELL_BURN)
    creature:CastSpell(creature, SPELL_REBIRTH)
    schedule(guid, "attack_players", 2000, function() onAttackPlayers(creature, guid) end)
end

-- C++ DamageTaken (:406-433): lethal blow with !_isInEgg runs the
-- egg sequence (AttackStop + REACT_PASSIVE + RemoveAllAuras +
-- SetFlag NOT_SELECTABLE + EMBER_BLAST self-cast + egg summon +
-- HATCH 15s); damage is ALWAYS rewritten to health-1 on a lethal
-- blow, egg or not (the rewrite sits outside the _isInEgg guard).
local function onDamageTaken(_, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local health = creature:GetHealth()
    local newDamage = damage
    if damage >= health then
        if not inEgg[guid] then
            inEgg[guid] = true
            creature:AttackStop()
            creature:SetReactState(0) -- REACT_PASSIVE
            creature:RemoveAllAuras()
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:CastSpell(creature, SPELL_EMBER_BLAST)
            local egg = creature:SummonCreature(NPC_PHOENIX_EGG, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 0)
            if egg then
                phoenixEgg[guid] = egg
            end
            schedule(guid, "hatch", 15000, function() onHatchFromEgg(creature, guid) end)
        end
        newDamage = health - 1
    end
    return false, newDamage
end

-- C++ SummonedCreatureDies (:435-439): egg destroyed within 15s ->
-- DespawnOrUnsummon. C++ ignores which summon died (only the egg is
-- ever its summon); the handler does the same.
local function onSummonedCreatureDies(_, creature, summon, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    inEgg[guid] = nil
    phoenixEgg[guid] = nil
    creature:DespawnOrUnsummon()
end

-- Egg cleanup twin for Kael'thas's summons.DespawnAll legs (the egg
-- is not in his Lua summons list — see header): on evade/reset the
-- phoenix despawns its stored egg, matching the C++ net effect.
-- On the death path ev23 fires before ev4 with the creature dead, so
-- the despawn is alive-gated (felblood boss file sphere precedent);
-- on death itself the egg persists (C++ JustDied calls no
-- DespawnAll), only timers/state clear.
local function clearState(guid)
    cancelTimers(guid)
    inEgg[guid] = nil
    phoenixEgg[guid] = nil
end

local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    local egg = phoenixEgg[guid]
    if egg then
        egg:DespawnOrUnsummon()
    end
    clearState(guid)
end

local function onDied(_, creature)
    clearState(creature:GetGUIDLow())
end

local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    if creature:IsAlive() then
        local egg = phoenixEgg[guid]
        if egg then
            egg:DespawnOrUnsummon()
        end
    end
    clearState(guid)
end

RegisterCreatureEvent(ENTRY_PHOENIX, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_PHOENIX, 4, onDied)
RegisterCreatureEvent(ENTRY_PHOENIX, 5, onSpawn)
RegisterCreatureEvent(ENTRY_PHOENIX, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_PHOENIX, 21, onSummonedCreatureDies)
RegisterCreatureEvent(ENTRY_PHOENIX, 23, onReset)

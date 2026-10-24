-- Coren Direbrew's Sisters (Blackrock Depths, Grim Guzzler) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_coren_direbrew.cpp
-- (npc_coren_direbrew_sisters — TaskScheduler combat AI).
-- Boss-script unit per eastern_kingdoms_script_loader.cpp order
-- (npc block inside AddSC_boss_coren_direbrew, after boss_doomrel's
-- tomb_of_seven group).
-- Entries (verifiable from the C++ sources): NPC_ILSA_DIREBREW = 26764
-- and NPC_URSULA_DIREBREW = 26822 in the coren_direbrew creatures enum
-- (boss_coren_direbrew.cpp lines 55-56). The creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace), but the
-- entries themselves are C++-verifiable so the port is registered
-- (boss_doomrel / npc_phalanx precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith :302-322 + TaskScheduler):
-- DoCastSelf(SPELL_PORT_TO_COREN 52850) non-triggered; DoCastSelf(
-- SPELL_BARRELED_CONTROL_AURA 50278) for Ursula (26822) else
-- DoCastSelf(SPELL_SEND_MUG_CONTROL_AURA 47369) non-triggered; then
-- the mug-chuck loop, Schedule(2s): SelectTarget(Random, 0, 0.0f,
-- false, true, -SPELL_HAS_DARK_BREWMAIDENS_BREW 47331) ->
-- DoCast(target, SPELL_CHUCK_MUG 50276), Repeat(4s). The scheduler's
-- !HasUnitState(UNIT_STATE_CASTING) validator is dropped (no bridge —
-- maiden precedent). The aura exclusion is bridged (plr:HasAura on the
-- player Lua surface, lua_player.go:203); the 0.0f any-range target is
-- modeled with 100 (curator precedent).
-- Deviations from C++: C++ SelectTarget passes playerOnly=false /
-- withTank=true — pets/totems can be picked and the tank is
-- preferred; the Lua port draws random from alive players only
-- (threat-list caveat shared with the keleseth/thorim/beast ports:
-- C++ draws from the threat list; the bridge can include nearby
-- unengaged players and excludes non-player units — dagran
-- HAND_OF_THAURISSAN counter-precedent documented in
-- boss_coren_direbrew.lua). The control-aura ticks are unported
-- AuraScripts (hellfire_peninsula precedent); only the engage casts
-- are ported.
-- Note: the DamageTaken sister summons in boss_coren_direbrew.cpp
-- (:182-194) are PORTED in boss_coren_direbrew.lua (summonSister:
-- SummonCreature(26764/26822, boss pos, type 6) + DoZoneInCombat
-- on the 66%/33% event-9 latches, with the 1s event-21 respawn
-- re-arms); this file stays the home for the engage arm only.

local ENTRY_ILSA_DIREBREW = 26764
local ENTRY_URSULA_DIREBREW = 26822

local SPELL_PORT_TO_COREN = 52850
local SPELL_BARRELED_CONTROL_AURA = 50278
local SPELL_SEND_MUG_CONTROL_AURA = 47369
local SPELL_CHUCK_MUG = 50276
local SPELL_HAS_DARK_BREWMAIDENS_BREW = 47331

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

-- C++ SelectTarget(Random, 0, 0.0f, false, true, -47331): any-range
-- random pick, playerOnly=false, withTank=true, excluding targets
-- with aura 47331. Bridged as random alive player within 100yd
-- excluding HasAura(47331) (curator any-range precedent; beast
-- threat-list caveat applies).
local function chuckTarget(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and not plr:HasAura(SPELL_HAS_DARK_BREWMAIDENS_BREW)
                and creature:IsWithinDist(plr, 100) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ JustEngagedWith mug-chuck arm: 2s init -> 4s loop,
-- DoCast(target, 50276) non-triggered, re-arm unconditional (C++
-- Repeat outside the target check).
local function onChuckMug(creature, guid)
    local target = chuckTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CHUCK_MUG)
    end
    schedule(guid, "chuckmug", 4000, function() onChuckMug(creature, guid) end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith: DoCastSelf(PORT_TO_COREN) non-triggered;
    -- Ursula gets the barreled control aura, Ilsa the send-mug one.
    creature:CastSpell(creature, SPELL_PORT_TO_COREN)
    if creature:GetEntry() == ENTRY_URSULA_DIREBREW then
        creature:CastSpell(creature, SPELL_BARRELED_CONTROL_AURA)
    else
        creature:CastSpell(creature, SPELL_SEND_MUG_CONTROL_AURA)
    end
    schedule(guid, "chuckmug", 2000, function() onChuckMug(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ILSA_DIREBREW, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_ILSA_DIREBREW, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_ILSA_DIREBREW, 4, onDied)
RegisterCreatureEvent(ENTRY_ILSA_DIREBREW, 23, onReset)
RegisterCreatureEvent(ENTRY_URSULA_DIREBREW, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_URSULA_DIREBREW, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_URSULA_DIREBREW, 4, onDied)
RegisterCreatureEvent(ENTRY_URSULA_DIREBREW, 23, onReset)

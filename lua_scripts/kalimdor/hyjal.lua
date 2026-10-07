-- Battle for Mount Hyjal zone scripts — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/hyjal.cpp
-- (npc_jaina_proudmoore / npc_thrall / npc_tyrande_whisperwind : public
-- CreatureScript; each AI : public hyjalAI; AddSC_hyjal registers all three;
-- kalimdor loader decl 26 / call 139 per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms hyjal.cpp as the sole source
-- of the three script names (loader lines only otherwise).
-- Entry: hyjal.h HYCreaturesIds names JAINA = 17772 / THRALL = 17852 /
-- TYRANDE = 17948 AND instance_hyjal.cpp cases each in OnCreatureCreate
-- (JainaProudmoore/Thrall/TyrandeWhisperwind GUID capture) with matching
-- GetGuidData DATA_JAINAPROUDMOORE/DATA_THRALL/DATA_TYRANDEWHISPERWIND
-- legs — the name-to-entry tie is C++-verified (ramstein strength); the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (hyjalAI spell table — hyjalAI.cpp JustEngagedWith/UpdateAI):
-- - Engage (C++ JustEngagedWith, non-dummy leg): Talk ATTACKED (0) + arm
--   each spell timer at its constructor cooldown. C++ rolls the urand
--   cooldown ONCE at AI construction and re-arms with that same stored
--   value every cycle; the port rolls the intervals once per entry at
--   load — same fixed-interval behavior.
-- - Timer expiry: pick target per TargetType (VICTIM -> GetVictim,
--   RANDOM -> SelectTarget(Random, 0) = threat-list pick excluding the
--   tank with no range cap, ported as random alive player in 100yd
--   excluding the current victim, SELF -> me); if target alive,
--   non-triggered DoCast (jeklik convention) and re-arm at the fixed
--   interval; if no valid target, C++ leaves the timer expired so the
--   next UpdateAI tick retries — modeled as a 1s retry (batrider
--   bomb precedent).
-- - Death: Talk DEATH (6) (C++ JustDied leg; the respawn/invisible/wave
--   bookkeeping has no bridge).
-- - Reset / leave combat: timers canceled (no C++ timer arm on Reset; the
--   timers arm only on engage); Reset re-casts SPELL_BRILLIANCE_AURA
--   31260 triggered on self for Jaina (hyjalAI.cpp:395).
-- Unmodeled (documented-only, no bridges): the hyjalAI wave machine —
-- StartEvent/Retreat/first-boss/WaveCount/RetreatType/IsDummy escort
-- choreography (GetHyjalAI -> GetInstanceAI leg; instance-script model
-- blocked, standing); all OnGossipHello options (gated on instance
-- GetInstanceData DATA_RAGEWINTERCHILLEVENT/DATA_ANETHERONEVENT/
-- DATA_ALLIANCE_RETREAT/DATA_KAZROGALEVENT/DATA_AZGALOREVENT — no
-- instance-data bridge); all OnGossipSelect arms (StartEvent/Retreat/
-- Debug toggle/DeSpawnVeins); the InterruptNonMeleeSpells pre-cast arm
-- (no bridge); Summons/respawn/wave-failure bookkeeping in JustDied.
-- npc_tyrande_whisperwind: NOT registered — no bridgeable hooks. Its
-- constructor sets zero spells, and its gossip is entirely
-- instance-gated (item offer only when DATA_AZGALOREVENT == DONE and the
-- player lacks ITEM_TEAR_OF_GODDESS 24494; OnGossipSelect grants the item
-- via CanStoreNewItem/StoreNewItem/SendNewItem). Registering the gossip
-- unconditionally would hand out the item outside the Azgalor-DONE gate —
-- a firesworn stub (harrison precedent); the .lua documents the arm for
-- when an instance-data bridge exists. C++ has no KilledUnit / combat
-- spell arms for Tyrande.

local ENTRY_JAINA = 17772
local ENTRY_THRALL = 17852

local YELL_ATTACKED = 0
local YELL_DEATH = 6

local SPELL_BLIZZARD = 31266
local SPELL_PYROBLAST = 31263
local SPELL_SUMMON_ELEMENTALS = 31264
local SPELL_CHAIN_LIGHTNING = 31330
local SPELL_SUMMON_DIRE_WOLF = 31331
local SPELL_BRILLIANCE_AURA = 31260

-- C++-verbatim constructor tables: {spell, cooldownLo, cooldownHi, target}.
-- Target: "self" / "victim" / "random".
local SPELLS = {
    ["npc_jaina_proudmoore"] = {
        { spell = SPELL_BLIZZARD, lo = 15000, hi = 35000, target = "random" },
        { spell = SPELL_PYROBLAST, lo = 5500, hi = 9500, target = "random" },
        { spell = SPELL_SUMMON_ELEMENTALS, lo = 15000, hi = 45000, target = "self" },
    },
    ["npc_thrall"] = {
        { spell = SPELL_CHAIN_LIGHTNING, lo = 3000, hi = 8000, target = "victim" },
        { spell = SPELL_SUMMON_DIRE_WOLF, lo = 6000, hi = 41000, target = "random" },
    },
}

local timers = {}

-- C++ rolls Spells[i].Cooldown ONCE at AI construction (the derived
-- class constructor: urand(...) in npc_jaina_proudmooreAI /
-- npc_thrallAI); JustEngagedWith arms SpellTimer[i] with it and every
-- expiry re-arms with the same stored value — a fixed interval per
-- creature, never re-rolled. Rolled once per entry at load below.
local intervals = {}
for name, specs in pairs(SPELLS) do
    local iv = {}
    intervals[name] = iv
    for i, spec in ipairs(specs) do
        iv[i] = math.random(spec.lo, spec.hi)
    end
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    intervals[guid] = nil
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

-- C++ TARGETTYPE_RANDOM: SelectTarget(SelectTargetMethod::Random, 0) =
-- DefaultTargetSelector(me, dist=0, playerOnly=false, withTank=false, 0):
-- the tank (main-threat) is EXCLUDED via _exception, and dist=0 means no
-- range cap (UnitAI.h comment: "if 0: ignored"; UnitAI.cpp:259-285
-- verified). Ported as a random alive player in 100yd excluding the
-- current victim — tank exclusion C++-exact, 100yd cap the documented
-- janalai approximation of the uncapped threat-list pick.
local function randomPlayerInRange(creature, maxDist, excludeVictim)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist
                and not (excludeVictim and victim and p == victim) then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ UpdateAI spell leg: pick target per TargetType; cast if alive and
-- re-arm at the fixed cooldown, else leave the timer expired (retry soon).
-- C++ DoCast(target, spell) is the 2-arg non-triggered form (DoCastAOE
-- 1-arg resolves to self-cast, also non-triggered).
local function onSpell(creature, guid, name, idx)
    local spec = SPELLS[name][idx]
    local iv = intervals[name]
    local target = nil
    if spec.target == "self" then
        target = creature
    elseif spec.target == "victim" then
        target = creature:GetVictim()
    else
        target = randomPlayerInRange(creature, 100, true)
    end
    if target and not target:IsDead() then
        creature:CastSpell(target, spec.spell)
        schedule(guid, "spell" .. idx, iv[idx], function()
            onSpell(creature, guid, name, idx)
        end)
    else
        schedule(guid, "spell" .. idx, 1000, function()
            onSpell(creature, guid, name, idx)
        end)
    end
end

local function onEnterCombat(name)
    return function(event, creature, target)
        local guid = creature:GetGUID()
        cancelTimers(guid)
        creature:Talk(YELL_ATTACKED)
        local iv = intervals[name]
        for i, spec in ipairs(SPELLS[name]) do
            local n, id = name, i
            schedule(guid, "spell" .. i, iv[i], function()
                onSpell(creature, guid, n, id)
            end)
        end
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: Initialize() + faction bookkeeping (no bridges); for Jaina
-- it also re-casts SPELL_BRILLIANCE_AURA 31260 TRIGGERED on self
-- (DoCast(me, spell, true) — hyjalAI.cpp:395; the aura is dropped on
-- evade per the hyjalAI.h comment, so Reset re-applies it). No C++ timer
-- arms on Reset — the SpellTimers arm only on engage.
local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if creature:GetEntry() == ENTRY_JAINA then
        creature:CastSpell(creature, SPELL_BRILLIANCE_AURA, true)
    end
end

-- C++ JustDied: Talk DEATH (the wave/instance bookkeeping has no bridge).
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(YELL_DEATH)
end

RegisterCreatureEvent(ENTRY_JAINA, 1, onEnterCombat("npc_jaina_proudmoore"))
RegisterCreatureEvent(ENTRY_JAINA, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_JAINA, 4, onDied)
RegisterCreatureEvent(ENTRY_JAINA, 23, onReset)

RegisterCreatureEvent(ENTRY_THRALL, 1, onEnterCombat("npc_thrall"))
RegisterCreatureEvent(ENTRY_THRALL, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_THRALL, 4, onDied)
RegisterCreatureEvent(ENTRY_THRALL, 23, onReset)

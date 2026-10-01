-- Teron Gorefiend (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_teron_gorefiend.cpp
-- (boss_teron_gorefiend only; npc_doom_blossom, npc_shadowy_
-- construct, the shadow_of_death / spiritual_vengeance AuraScripts,
-- the shadow_of_death_remove SpellScript and
-- at_teron_gorefiend_entrance documented below, not registered);
-- black_temple.h:34 (DATA_TERON_GOREFIEND = 3, fourth boss), :74
-- (NPC_TERON_GOREFIEND = 22871). Creature entry: 22871 Teron
-- Gorefiend (C++ ScriptName "boss_teron_gorefiend" per
-- RegisterBlackTempleCreatureAI in AddSC_boss_teron_gorefiend; the
-- creature_template ScriptName binding is DB-side — no TDB in this
-- workspace, so only the C++-side naming is verified). Talk lines
-- used: SAY_AGGRO=1 (pull), SAY_SLAY=2 (kill, player-only),
-- SAY_INCINERATE=3 (incinerate), SAY_BLOSSOM=4 (doom blossom),
-- SAY_CRUSHING=5 (crushing shadows), SAY_DEATH=6 (death);
-- SAY_INTRO=0 belongs to the unmodeled intro arm.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm berserk 45078 10min, non-triggered
-- self-cast, one-shot (C++ DoCast default is triggered=false —
-- vaelastrasz convention) / incinerate 40239 12s then {12s,20s},
-- non-triggered on a random alive player in the instance (C++
-- SelectTarget(Random, 0) takes any alive target, C++-exact) +
-- Talk(SAY_INCINERATE) / summon doom blossom 40188 8s then
-- {30s,40s}, triggered self-cast (C++ DoCastSelf(..., true)) +
-- Talk(SAY_BLOSSOM); the doom blossom summon (NPC 23123) has no
-- summon bridge — only the cast and Talk are kept / shadow of death
-- 40251 8s then {30s,35s}, non-triggered on a random alive player
-- in the instance within 100 yd excluding the current victim and
-- excluding players carrying 40268 (spiritual vengeance; the -aura
-- filter has a target-HasAura bridge here — C++-exact) / crushing
-- shadows 40243 18s then {18s,30s}, non-triggered self-cast + Talk
-- (SAY_CRUSHING); the SPELLVALUE_MAX_TARGETS=5 arm has no bridge —
-- the cast goes out unclamped. Nil-target ticks cast nothing but
-- keep the schedule (jeklik convention). OnTargetDied(3):
-- Talk(SAY_SLAY), TYPEID_PLAYER gate (terestian convention).
-- OnDied(4): Talk(SAY_DEATH); the DoCast(SPELL_SHADOW_OF_DEATH_
-- REMOVE 41999) arm has no bearer — its entire effect lives in the
-- spell_shade-of-death-remove SpellScript, which has no bridge
-- (standing SpellScript gap). OnLeaveCombat(2)/OnReset(23): cancel
-- timers. The C++ intro arms (Reset's DATA_TERON_GOREFIEND_INTRO
-- gate, DoAction(ACTION_START_INTRO) + Talk(SAY_INTRO) + 20s
-- EVENT_FINISH_INTRO, the NON_ATTACKABLE/NOT_SELECTABLE/REACT_
-- PASSIVE flag arms) have no bridge — the area trigger
-- at_teron_gorefiend_entrance that starts the intro (instance
-- GetCreature(DATA_TERON_GOREFIEND) -> DoAction) has no
-- area-trigger bridge and the instance-data/flag/react arms are
-- blocked on the instance-script model (standing gaps), so the
-- fight starts on pull, ragnaros-intro convention. The
-- EnterEvadeMode summons.DespawnAll/_DespawnAtEvade arms have no
-- bridges (evade-side cleanup is engine-side). The npc_doom_blossom
-- AI (NPC_DOOM_BLOSSOM 23123 per black_temple companion enum: hover
-- takeoff +8z, cast 40186, shadowbolt 40185 on a random target 12s
-- then every 2s) and npc_shadowy_construct AI (NPC_SHADOWY_
-- CONSTRUCT 23111: damage/school spell-immunity arms, atrophy 40327
-- on the victim 12s then {10s,12s}, 1s re-target loop via Teron's
-- threat pick excluding 40268-carrying players with 1,000,000
-- threat) are not registered — the C++ ScriptNames bind DB-side in
-- creature_template (no TDB in this workspace), so the entries
-- cannot be verified against the bindings from the C++ sources
-- alone (garr firesworn convention). The construct's
-- instance-creature lookup, ResetThreatList/AddThreat(1,000,000)
-- and ApplySpellImmune arms additionally have no bridges. The
-- spell_teron_gorefiend_shadow_of_death AuraScript (damage absorb
-- nullified, on-expire summons the spirit + 4 skeletrons + 40282 +
-- 40268) and spell_teron_gorefiend_spiritual_vengeance AuraScript
-- (on-remove KillSelf) have no AuraScript bridge (standing
-- AuraScript gap).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the DATA_TERON_GOREFIEND
-- bookkeeping arms skipped).

local ENTRY_TERON_GOREFIEND = 22871

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_INCINERATE = 3
local SAY_BLOSSOM = 4
local SAY_CRUSHING = 5
local SAY_DEATH = 6

local SPELL_INCINERATE = 40239
local SPELL_CRUSHING_SHADOWS = 40243
local SPELL_SHADOW_OF_DEATH = 40251
local SPELL_BERSERK = 45078
local SPELL_SUMMON_DOOM_BLOSSOM = 40188
local SPELL_SPIRITUAL_VENGEANCE = 40268

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

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0): any alive player in the instance,
-- no range cap, victim included.
local function randomAnyTarget(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, 100.0f, true, true,
-- -SPELL_SPIRITUAL_VENGEANCE): any alive player in the instance
-- within 100 yd, excluding the current victim (position 1), minus
-- players carrying 40268.
local function randomShadowDeathTarget(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if (not victim or p:GetGUID() ~= victim:GetGUID())
                and creature:GetDistance(p) <= 100
                and not p:HasAura(SPELL_SPIRITUAL_VENGEANCE) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_ENRAGE: non-triggered self-cast 45078; one-shot
-- (C++-exact — no re-arm).
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, false)
end

-- C++ EVENT_INCINERATE: non-triggered 40239 on a random target +
-- Talk(SAY_INCINERATE); re-arm {12s,20s} unconditional (C++-exact —
-- the repeat runs even when no target was found).
local function onIncinerate(creature, guid)
    local target = randomAnyTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_INCINERATE, false)
    end
    creature:Talk(SAY_INCINERATE)
    schedule(guid, "incinerate", 12000 + math.random(0, 8000), function()
        onIncinerate(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_DOOM_BLOSSOM: triggered self-cast 40188 +
-- Talk(SAY_BLOSSOM); re-arm {30s,40s}. The doom blossom summon has
-- no summon bridge.
local function onDoomBlossom(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_DOOM_BLOSSOM, true)
    creature:Talk(SAY_BLOSSOM)
    schedule(guid, "doomblossom", 30000 + math.random(0, 10000), function()
        onDoomBlossom(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_DEATH: non-triggered 40251 on the filtered
-- random target; re-arm {30s,35s} unconditional (C++-exact).
local function onShadowDeath(creature, guid)
    local target = randomShadowDeathTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_OF_DEATH, false)
    end
    schedule(guid, "shadowdeath", 30000 + math.random(0, 5000), function()
        onShadowDeath(creature, guid)
    end)
end

-- C++ EVENT_CRUSHING_SHADOWS: non-triggered self-cast 40243 +
-- Talk(SAY_CRUSHING); re-arm {18s,30s}. The SPELLVALUE_MAX_TARGETS
-- = 5 arm has no bridge — the cast goes out unclamped.
local function onCrushingShadows(creature, guid)
    creature:CastSpell(creature, SPELL_CRUSHING_SHADOWS, false)
    creature:Talk(SAY_CRUSHING)
    schedule(guid, "crushing", 18000 + math.random(0, 12000), function()
        onCrushingShadows(creature, guid)
    end)
end

local function teronEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
    schedule(guid, "incinerate", 12000, function()
        onIncinerate(creature, guid)
    end)
    schedule(guid, "doomblossom", 8000, function()
        onDoomBlossom(creature, guid)
    end)
    schedule(guid, "shadowdeath", 8000, function()
        onShadowDeath(creature, guid)
    end)
    schedule(guid, "crushing", 18000, function()
        onCrushingShadows(creature, guid)
    end)
end

local function teronResetState(guid)
    cancelTimers(guid)
end

local function teronLeaveCombat(event, creature)
    teronResetState(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY), TYPEID_PLAYER gate.
local function teronTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH). The DoCast(SPELL_SHADOW_OF_DEATH_
-- REMOVE) arm has no bearer — its effect lives in the unmodeled
-- SpellScript.
local function teronDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    teronResetState(creature:GetGUID())
end

local function teronReset(event, creature)
    teronResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 1, teronEnterCombat)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 2, teronLeaveCombat)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 3, teronTargetDied)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 4, teronDied)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 23, teronReset)

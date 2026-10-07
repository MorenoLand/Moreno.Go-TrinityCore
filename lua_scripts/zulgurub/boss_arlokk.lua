-- Arlokk (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_arlokk.cpp
-- (boss_arlokkAI + npc_zulian_prowlerAI + go_gong_of_bethekkAI);
-- zulgurub.h:33 (DATA_ARLOKK = 3), :49 (NPC_ARLOKK = 14515),
-- :51 (NPC_ZULIAN_PROWLER = 15101), :70 (GO_GONG_OF_BETHEKK = 180526).
-- Creature entries: 14515 Arlokk (C++ ScriptName "boss_arlokk" per
-- AddSC_boss_arlokk); 15101 Zulian prowler (C++ ScriptName
-- "npc_zulian_prowler"). The panther trigger 15091 has no C++ script
-- and is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++ PHASE_ONE=1 troll / PHASE_TWO=2 panther):
--   troll: Talk SAY_AGGRO(0); shadow word pain 24212 at {7s,9s} then
--     {5s,7s} on the victim (triggered); gouge 12540 at {12s,15s} on
--     the victim (triggered, one-shot — C++ does not re-arm it);
--     mark of arlokk 24210 at {9s,11s} then {120s,130s} on a random
--     alive player (C++: random of 2nd-4th threat not marked; no threat
--     model) with victim fallback, triggered cast, then Talk
--     SAY_FEAST_PROWLER(1); panther transform at {15s,20s} (one-shot).
--   transform: non-triggered self-cast 24190 (panther transform);
--     unequip/weapon/AttackStop/threat/passive/flag arms have no
--     bridge; self-cast 24222 (vanish visual) then 24223 (vanish);
--     1s later self-cast 24235 (super invis), move to a random spot
--     (no movement model — the boss just stays invisible);
--     9s later re-cast 24223 + 24235; {7s,10s} later re-appear:
--     RemoveAura(24235), RemoveAura(24223), arm ravage 24213 at
--     {10s,14s} (triggered on victim) and transform back at
--     {15s,18s}, phase -> panther.
--   transform back: RemoveAura(24190); self-cast 24222 vanish visual;
--     re-arm shadow word pain {4s,7s}, gouge {12s,15s}, transform
--     {16s,20s} (all PHASE_ONE), phase -> troll.
--   died: Talk SAY_DEATH(2), cancel timers.
-- Prowler (15101): OnReset self-casts the sneak auras 22766 + 7939
-- (C++-exact non-triggered casts; the MovePoint to Arlokk has no
-- bridge); OnEnterCombat removes both sneak auras (C++
-- JustEngagedWith). Melee is engine-driven. Its 6s attack timer,
-- SpellHit(24211) attack arm, and JustDied arlokk-SetData/despawn arms
-- have no bridge and are not modeled.
-- Deviations from C++: no summon model — the zulian prowlers, the
-- panther triggers 15091, and the SummonCreature(14515) from the gong
-- never exist, so EVENT_SUMMON_PROWLERS, the trigger scan, the
-- SetData count arms, and the whole go_gong_of_bethekk script (gong
-- flag/SendCustomAnim/summon arms have no bridge) are skipped; no
-- instance-script model — boss admission via the luaBossAI shim, and
-- the EnterEvadeMode gong-flag arm is skipped (the 4s despawn has no
-- bridge); no threat model — mark-of-arlokk target is a random alive
-- player (victim fallback), ResetThreatList skipped; no equipment
-- bridge — the UNIT_VIRTUAL_ITEM_SLOT_ID dagger arms skipped; no
-- flag/react-state bridge — the NON_ATTACKABLE/NOT_SELECTABLE/REACT_
-- PASSIVE arms skipped (the phase stays damageable); no stat-modifier
-- bridge — the C++ damage +/-35% hack skipped; no movement model —
-- the Reset MovePoint, vanish MovePoint, and AttackStart arms skipped;
-- Talk has no target bridge — SAY_FEAST_PROWLER plays as a plain yell.

local ENTRY_ARLOKK = 14515
local ENTRY_PROWLER = 15101

local SAY_AGGRO = 0
local SAY_FEAST_PROWLER = 1
local SAY_DEATH = 2

local SPELL_SHADOW_WORD_PAIN = 24212
local SPELL_GOUGE = 12540
local SPELL_MARK_OF_ARLOKK = 24210
local SPELL_RAVAGE = 24213
local SPELL_PANTHER_TRANSFORM = 24190
local SPELL_VANISH_VISUAL = 24222
local SPELL_VANISH = 24223
local SPELL_SUPER_INVIS = 24235
local SPELL_SNEAK_1 = 22766
local SPELL_SNEAK_2 = 7939

local PHASE_TROLL = 1
local PHASE_PANTHER = 2

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function arlokkState(guid)
    local st = state[guid]
    if not st then
        st = { panther = false }
        state[guid] = st
    end
    return st
end

local function alivePlayers(creature)
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

local function randomPlayer(creature)
    local candidates = alivePlayers(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ shadow word pain (troll): triggered victim cast, re-arm {5s,7s}
-- while still in troll form (phase gate collapses to a state check).
local function onShadowWordPain(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_WORD_PAIN, true)
    end
    schedule(timers, guid, "swp", math.random(5000, 7000), function()
        onShadowWordPain(creature, guid)
    end)
end

-- C++ gouge (troll): triggered victim cast, one-shot (C++ does not
-- re-arm EVENT_GOUGE).
local function onGouge(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GOUGE, true)
    end
end

-- C++ mark of arlokk (both phases): triggered cast on a random alive
-- player (victim fallback), then Talk(1); re-arm {120s,130s}.
local function onMarkOfArlokk(creature, guid)
    local target = randomPlayer(creature) or creature:GetVictim()
    if target then
        creature:CastSpell(target, SPELL_MARK_OF_ARLOKK, true)
        creature:Talk(SAY_FEAST_PROWLER)
    end
    schedule(timers, guid, "mark", math.random(120000, 130000), function()
        onMarkOfArlokk(creature, guid)
    end)
end

-- C++ EventMap phase semantics (src/common/Utilities/EventMap.cpp
-- ExecuteEvent): while no phase is set (_phase == 0 — the whole troll
-- stretch up to EVENT_VISIBLE), every due event fires regardless of
-- the phase it was scheduled with. So the transform does NOT interrupt
-- the troll timers: shadow word pain keeps firing through the vanish
-- chain. SetPhase(PHASE_TWO) at EVENT_VISIBLE lazily discards the
-- pending phase-one events; SetPhase(PHASE_ONE) at transform back
-- discards the pending phase-two events. The cancelKeys calls below
-- model exactly those two discards.
local function cancelKeys(store, guid, keys)
    local per = store[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

-- Forward declarations: onVanish2 schedules onRavage/onTransformBack,
-- which are defined below (halazzi/zuljin convention).
local onRavage
local onTransformBack

-- C++ panther transform: self-cast 24190 (non-triggered), vanish
-- visual 24222, vanish 24223; threat/passive/flag/equipment/movement
-- arms have no bridge. The C++ EVENT_TRANSFORM was scheduled with
-- phase PHASE_ONE, so the 1s vanish chain only runs in troll form.
local function onVanish2(creature, guid)
    local st = arlokkState(guid)
    if st.panther then
        return
    end
    creature:CastSpell(creature, SPELL_VANISH)
    creature:CastSpell(creature, SPELL_SUPER_INVIS)
    schedule(timers, guid, "visible", math.random(7000, 10000), function()
        local st2 = arlokkState(guid)
        if st2.panther then
            return
        end
        -- C++ EVENT_VISIBLE: SetPhase(PHASE_TWO) discards the pending
        -- phase-one timers (shadow word pain / gouge / transform).
        cancelKeys(timers, guid, { "swp", "gouge", "transform" })
        st2.panther = true
        creature:RemoveAura(SPELL_SUPER_INVIS)
        creature:RemoveAura(SPELL_VANISH)
        schedule(timers, guid, "ravage", math.random(10000, 14000),
            function()
                onRavage(creature, guid)
            end)
        schedule(timers, guid, "transformback", math.random(15000, 18000),
            function()
                onTransformBack(creature, guid)
            end)
    end)
end

local function onVanish(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    creature:CastSpell(creature, SPELL_SUPER_INVIS)
    schedule(timers, guid, "vanish2", 9000, function()
        onVanish2(creature, guid)
    end)
end

local function onTransform(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    -- C++ does not cancel the troll timers here: with _phase still 0
    -- they keep firing through the vanish chain (see cancelKeys note).
    creature:CastSpell(creature, SPELL_PANTHER_TRANSFORM)
    creature:CastSpell(creature, SPELL_VANISH_VISUAL)
    creature:CastSpell(creature, SPELL_VANISH)
    schedule(timers, guid, "vanish", 1000, function()
        onVanish(creature, guid)
    end)
end

-- C++ ravage (panther): triggered victim cast, re-arm {10s,14s} while
-- still in panther form.
onRavage = function(creature, guid)
    if not arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_RAVAGE, true)
    end
    schedule(timers, guid, "ravage", math.random(10000, 14000), function()
        onRavage(creature, guid)
    end)
end

-- C++ transform back: RemoveAura(24190), vanish visual; the weapon
-- damage-reset/equipment arms have no bridge. Troll timers are re-
-- armed fresh; the panther timers are dropped.
onTransformBack = function(creature, guid)
    local st = arlokkState(guid)
    if not st.panther then
        return
    end
    st.panther = false
    -- C++ transform back: SetPhase(PHASE_ONE) discards the pending
    -- phase-two timers (ravage / transform back); the troll timers are
    -- re-armed fresh below.
    cancelKeys(timers, guid, { "ravage", "transformback" })
    creature:RemoveAura(SPELL_PANTHER_TRANSFORM)
    creature:CastSpell(creature, SPELL_VANISH_VISUAL)
    schedule(timers, guid, "swp", math.random(4000, 7000), function()
        onShadowWordPain(creature, guid)
    end)
    schedule(timers, guid, "gouge", math.random(12000, 15000), function()
        onGouge(creature, guid)
    end)
    schedule(timers, guid, "transform", math.random(16000, 20000), function()
        onTransform(creature, guid)
    end)
end

local function arlokkResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { panther = false }
end

-- C++ JustEngagedWith: Talk SAY_AGGRO; arm shadow word pain, gouge,
-- mark of arlokk, transform. The panther-trigger scan and prowler
-- summon have no bridge (no creature enumeration, no summon model) —
-- the 6s prowler timer is not scheduled.
local function arlokkEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    arlokkResetState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "swp", math.random(7000, 9000), function()
        onShadowWordPain(creature, guid)
    end)
    schedule(timers, guid, "gouge", math.random(12000, 15000), function()
        onGouge(creature, guid)
    end)
    schedule(timers, guid, "mark", math.random(9000, 11000), function()
        onMarkOfArlokk(creature, guid)
    end)
    schedule(timers, guid, "transform", math.random(15000, 20000), function()
        onTransform(creature, guid)
    end)
end

local function arlokkLeaveCombat(event, creature)
    arlokkResetState(creature:GetGUID())
end

-- C++ JustDied: _JustDied (instance, no bridge), Talk SAY_DEATH.
local function arlokkDied(event, creature, killer)
    arlokkResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function arlokkReset(event, creature)
    arlokkResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ARLOKK, 1, arlokkEnterCombat)
RegisterCreatureEvent(ENTRY_ARLOKK, 2, arlokkLeaveCombat)
RegisterCreatureEvent(ENTRY_ARLOKK, 4, arlokkDied)
RegisterCreatureEvent(ENTRY_ARLOKK, 23, arlokkReset)

-- npc_zulian_prowler (15101, C++ ScriptName "npc_zulian_prowler"):
-- OnReset self-casts sneak auras 22766 + 7939; OnEnterCombat removes
-- both (C++ JustEngagedWith); melee is engine-driven. The MovePoint to
-- Arlokk, the 6s random-attack timer (no Attack bridge), the
-- SpellHit(24211) attack arm, and the JustDied arlokk-SetData/despawn
-- arms have no bridge and are not modeled.
local function prowlerEnterCombat(event, creature, target)
    creature:RemoveAura(SPELL_SNEAK_1)
    creature:RemoveAura(SPELL_SNEAK_2)
end

local function prowlerReset(event, creature)
    creature:CastSpell(creature, SPELL_SNEAK_1)
    creature:CastSpell(creature, SPELL_SNEAK_2)
end

RegisterCreatureEvent(ENTRY_PROWLER, 1, prowlerEnterCombat)
RegisterCreatureEvent(ENTRY_PROWLER, 23, prowlerReset)

-- go_gong_of_bethekk (180526, C++ ScriptName "go_gong_of_bethekk"):
-- NOT registered — its only arms (SetFlag NOT_SELECTABLE,
-- SendCustomAnim, SummonCreature(14515)) have no Lua bridge.

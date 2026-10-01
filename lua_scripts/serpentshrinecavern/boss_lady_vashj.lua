-- Lady Vashj (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_lady_vashj.cpp (boss_lady_vashj, npc_enchanted_elemental,
-- npc_tainted_elemental, npc_toxic_sporebat,
-- npc_shield_generator_channel — documented below, only the first
-- two registered; item_tainted_core documented below, not
-- registered; AddSC_boss_lady_vashj registers those five C++
-- ScriptNames plus the item script).
-- Entry 21212 verified from instance_serpent_shrine.cpp's
-- OnCreatureCreate (case 21212 -> LadyVashj, DATA_LADYVASHJ);
-- the add entries below come from the file's own enum constants
-- (TAINTED_ELEMENTAL = 22009, ENCHANTED_ELEMENTAL = 21958,
-- TOXIC_SPOREBAT = 22140, SHIED_GENERATOR_CHANNEL = 19870,
-- COILFANG_STRIDER = 22056, COILFANG_ELITE = 22055, TOXIC_SPORES_
-- TRIGGER = 22207). The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace). Talk lines used: SAY_AGGRO=1
-- (pull), SAY_PHASE2=3 (70% phase switch), SAY_BOWSHOT=5 (shoot/
-- multishot, fires only when rand32()%3 != 0 — C++-exact),
-- SAY_SLAY=6 (kill — NO TYPEID gate in C++, C++-exact), SAY_DEATH
-- =7 (death). SAY_INTRO=0 is defined but its only firing arm is
-- the MoveInLineOfSight intro — no LOS hook bridge, documented
-- only; SAY_PHASE1=2 is defined but never called anywhere in the
-- file — unused enum member, documented only (gruul precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention);
-- DoCastVictim takes nil as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat(1):
-- per-GUID scheduler reset to the C++ Initialize() values
-- (shockBlast 1..60000 / staticCharge 10000..25000 / entangle
-- 30000 / check 15000 / forkedLightning 2000 / entangled false /
-- phase 1) + Talk(SAY_AGGRO) + start of a 1s scheduler pump (a
-- port of boss_lady_vashjAI::UpdateAI — 1s granularity is exact
-- for all C++ timers). The instance SetData(IN_PROGRESS) arm and
-- the per-player DestroyItemCount(31088) tainted-core sweep arm
-- are instance/player-item blocked (below). Phase 1: Shock blast
-- 38509 1..60000ms init then {1s,15s} — DoCastVictim (C++-exact).
-- Static charge 38280 10000..25000ms init then {10s,30s} — random
-- alive player within 200 yd (C++ SelectTarget(Random, 0, 200,
-- true) player-only, netherspite distance-gate convention); cast
-- only when the pick lacks aura 38280 (HasAura bridge), else the
-- timer re-arms without casting (C++-exact). Entangle 38316 30s:
-- if not entangled -> DoCastVictim 38316, entangled=true, re-arm
-- 10s; else -> CastShootOrMultishot, entangled=false, re-arm
-- {20s,25s} (C++-exact). Phase 1: once-guarded GetHealthPct() <
-- 70 (C++ HealthBelowPct(70)) -> phase=2, Talk(SAY_PHASE2) (the
-- MotionMaster Clear + DoTeleportTo(MIDDLE_X/Y/Z) and the four
-- shield-generator-channel summons have no movement/teleport/
-- summon bridges — unmodeled, below). CheckTimer 15s init then
-- 5s: if no alive player in the instance is within 5 yd (the
-- venoxis/gruul 5-yd GetDistance stand-in for the C++ threat-
-- list IsWithinMeleeRange scan — no threat bridge) -> CastShoot
-- OrMultishot. CastShootOrMultishot: urand(0,1) -> Shoot 40873
-- or Multishot 38310 DoCastVictim (C++-exact); Talk(SAY_BOWSHOT)
-- only when math.random(0,2) != 0 (the C++ rand32()%3 gate).
-- Phase 2: Forked lightning 40088 2s then {2s,8s} — random alive
-- player pick, victim fallback (C++-exact). The enchanted/
-- tainted/Coilfang-elite/Coilfang-strider summon arms are summon
-- blocked (below); the phase-3 CheckTimer poll is instance-
-- blocked (instance->GetData(DATA_CANSTARTPHASE3)), so phase 3
-- is unreachable in this model (below). OnTargetDied(3): Talk(
-- SAY_SLAY) — no TYPEID gate (C++-exact). OnDied(4): Talk(SAY_
-- DEATH) + cleanup (the SetData(DONE) arm is instance-blocked).
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state (the SetCorpseDelay arm has no bridge).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the
-- DATA_LADYVASHJEVENT NOT_STARTED/IN_PROGRESS/DONE bookkeeping,
-- the DATA_CANSTARTPHASE3 gate, DATA_LADYVASHJ GUID lookups and
-- the 19s intro AggroTimer / Intro / CanAttack / UNIT_FLAG_NON_
-- ATTACKABLE MoveInLineOfSight machine skipped — no flag bridge);
-- no summon bridge — the 4x shield-generator-channel (19870),
-- enchanted-elemental (21958), tainted-elemental (22009),
-- Coilfang-elite (22055) and Coilfang-strider (22056) summon
-- arms unmodeled (firesworn/hydross convention), and the phase-3
-- sporebat (22140) summon arm is unreachable anyway; no
-- movement/teleport bridge — the phase-2 MotionMaster Clear +
-- DoTeleportTo and the enchanted-elemental MovePoint waypoint
-- machine unmodeled (the enchanted elemental is movement-driven
-- through and through: nearest-waypoint search, SetWalk, walk
-- 0.6f speeds, MovePoint phases, IsWithinDist3d checks — no
-- bridges, so "npc_enchanted_elemental" (21958) is documented
-- only and not registered); no cross-creature bridge — the
-- enchanted-elemental VashjGUID checks, the sporebat vashj-phase
-- CheckTimer (npc_toxic_sporebat, 22140: random MovePoint
-- waypoints + TOXIC_SPORES_TRIGGER 22207 summon + triggered
-- 38575 cast + phase-3 gated despawn — movement/summon/cross-
-- creature gated, documented only, not registered), the shield-
-- generator-channel magic-barrier upkeep (npc_shield_generator_
-- channel, 19870: DoCast(vashj, SPELL_MAGIC_BARRIER 38112, true)
-- via instance GUID, SetDisplayId(11686) invisible + NOT_
-- SELECTABLE flag — no cross-creature/display/flag bridges,
-- documented only, not registered), and the tainted-elemental
-- JustDied -> EventTaintedElementalDeath relay (unmodeled); no
-- threat bridge — the melee-range check is the 5-yd player
-- stand-in; the JustEngagedWith AddThreat(0.1f) arm is dropped;
-- no item bridges for item_tainted_core (Tainted Core 31088):
-- the OnUse arm needs instance->GetData(DATA_SHIELDGENERATOR1.
-- .4), cross-creature channel kill via the vashj AI's stored
-- channel GUIDs, player->CastSpell(38134) on a player target,
-- DestroyItemCount(31088) and GameObject-target resolution —
-- none bridgeable, documented only, not registered; phase 2's
-- "no victim + in combat -> EnterEvadeMode" arm has no evade
-- bridge — unmodeled.
-- npc_tainted_elemental (22009) port: OnEnterCombat(1) per-GUID
-- reset + 1s pump; Poison bolt 40095 {5s,10s} then {5s,10s} —
-- random alive player within 30 yd (C++ SelectTarget(Random, 0)
-- + IsWithinDistInMap(me, 30)); nil pick or out-of-range pick
-- casts nothing, timer re-arms (C++-exact). DespawnTimer 30s ->
-- creature:Despawn() + cancel pump (C++ setDeathState(DEAD), the
-- 1s anti-crash re-arm folded into the cancel). OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cleanup (the JustDied tainted-
-- death relay is cross-creature blocked).

local SPELL_SHOCK_BLAST = 38509
local SPELL_STATIC_CHARGE_TRIGGER = 38280
local SPELL_ENTANGLE = 38316
local SPELL_FORKED_LIGHTNING = 40088
local SPELL_SHOOT = 40873
local SPELL_MULTI_SHOT = 38310
local SPELL_POISON_BOLT = 40095

local SAY_AGGRO = 1
local SAY_PHASE2 = 3
local SAY_BOWSHOT = 5
local SAY_SLAY = 6
local SAY_DEATH = 7

local ENTRY_LADY_VASHJ = 21212
local ENTRY_TAINTED_ELEMENTAL = 22009

local MELEE_RANGE = 5
local STATIC_CHARGE_RANGE = 200
local POISON_BOLT_RANGE = 30

local vashjTimers = {}
local vashjState = {}
local elementalTimers = {}
local elementalState = {}

-- Alive players sharing the creature's map+instance (teron
-- convention).
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

-- C++ SelectTarget(Random, 0): random alive player in the instance.
local function pickRandomAlivePlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 0, <dist>, true): random alive player in
-- the instance within <dist> yd (netherspite convention — the
-- GetDistance bridge stands in for the C++ distance gate).
local function pickRandomAlivePlayerInRange(creature, maxDist)
    local found = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function cancelVashjPump(guid)
    local id = vashjTimers[guid]
    if id then
        RemoveEventById(id)
        vashjTimers[guid] = nil
    end
end

local function resetVashj(guid)
    cancelVashjPump(guid)
    vashjState[guid] = nil
end

-- C++ CastShootOrMultishot: urand(0,1) picks Shoot or Multishot on
-- the victim; the bowshot yell fires only when rand32()%3 != 0.
local function castShootOrMultishot(creature)
    if math.random(0, 1) == 0 then
        creature:CastSpell(nil, SPELL_SHOOT)
    else
        creature:CastSpell(nil, SPELL_MULTI_SHOT)
    end
    if math.random(0, 2) ~= 0 then
        creature:Talk(SAY_BOWSHOT)
    end
end

-- C++ Initialize(): shockBlast 1+rand60000, staticCharge
-- 10000+rand15000, entangle 30000, check 15000, forkedLightning
-- 2000, entangled false, phase 0 (0 until combat starts it).
local function freshVashjState()
    return {
        shockBlast = 1 + math.random(0, 59999),
        staticCharge = 10000 + math.random(0, 14999),
        entangle = 30000,
        check = 15000,
        forkedLightning = 2000,
        entangled = false,
        phase = 1,
    }
end

local function vashjTick(creature, guid)
    local st = vashjState[guid]
    if not st then
        return
    end

    if st.phase == 1 then
        -- Shock Blast: DoCastVictim, re-arm {1s,15s} (C++-exact).
        if st.shockBlast <= 1000 then
            creature:CastSpell(nil, SPELL_SHOCK_BLAST)
            st.shockBlast = 1000 + math.random(0, 13999)
        else
            st.shockBlast = st.shockBlast - 1000
        end

        -- Static Charge: random player within 200 yd, cast only
        -- when the pick lacks the trigger aura; re-arm {10s,30s}
        -- regardless (C++-exact).
        if st.staticCharge <= 1000 then
            local pick = pickRandomAlivePlayerInRange(creature, STATIC_CHARGE_RANGE)
            if pick and not pick:HasAura(SPELL_STATIC_CHARGE_TRIGGER) then
                creature:CastSpell(pick, SPELL_STATIC_CHARGE_TRIGGER)
            end
            st.staticCharge = 10000 + math.random(0, 19999)
        else
            st.staticCharge = st.staticCharge - 1000
        end

        -- Entangle arm: cast on the victim, then a Shoot/Multishot
        -- follow-up 10s later, then re-arm {20s,25s} (C++-exact).
        if st.entangle <= 1000 then
            if not st.entangled then
                creature:CastSpell(nil, SPELL_ENTANGLE)
                st.entangled = true
                st.entangle = 10000
            else
                castShootOrMultishot(creature)
                st.entangled = false
                st.entangle = 20000 + math.random(0, 4999)
            end
        else
            st.entangle = st.entangle - 1000
        end

        -- Phase 2 at <70% health, once (C++ HealthBelowPct(70) is
        -- evaluated every UpdateAI; the teleport and shield-
        -- channel summons have no bridges — unmodeled, header).
        if creature:GetHealthPct() < 70 then
            creature:Talk(SAY_PHASE2)
            st.phase = 2
        end

        -- CheckTimer: if nobody is in melee range, Shoot/Multishot
        -- (the C++ threat-list scan has no bridge — the 5-yd
        -- GetDistance stand-in is the venoxis/gruul convention).
        if st.check <= 1000 then
            local inMeleeRange = false
            for _, p in ipairs(playersInInstance(creature)) do
                if creature:GetDistance(p) <= MELEE_RANGE then
                    inMeleeRange = true
                    break
                end
            end
            if not inMeleeRange then
                castShootOrMultishot(creature)
            end
            st.check = 5000
        else
            st.check = st.check - 1000
        end
    else
        -- Phase 2: forked lightning 2s then {2s,8s} — random
        -- player pick, victim fallback (C++-exact). The summon
        -- arms and the phase-3 DATA_CANSTARTPHASE3 poll are
        -- blocked (header), so phase 3 is unreachable here.
        if st.forkedLightning <= 1000 then
            local pick = pickRandomAlivePlayer(creature)
            if not pick then
                pick = creature:GetVictim()
            end
            if pick then
                creature:CastSpell(pick, SPELL_FORKED_LIGHTNING)
            end
            st.forkedLightning = 2000 + math.random(0, 5999)
        else
            st.forkedLightning = st.forkedLightning - 1000
        end
    end
end

local function vashjEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetVashj(guid)
    vashjState[guid] = freshVashjState()
    creature:Talk(SAY_AGGRO)
    vashjTimers[guid] = CreateLuaEvent(function()
        vashjTick(creature, guid)
    end, 1000)
end

local function vashjLeaveCombat(event, creature)
    resetVashj(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY), no TYPEID gate.
local function vashjTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function vashjDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetVashj(creature:GetGUID())
end

local function vashjReset(event, creature)
    resetVashj(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LADY_VASHJ, 1, vashjEnterCombat)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 2, vashjLeaveCombat)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 3, vashjTargetDied)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 4, vashjDied)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 23, vashjReset)

-- npc_tainted_elemental (22009) — see header.

local function cancelElementalPump(guid)
    local id = elementalTimers[guid]
    if id then
        RemoveEventById(id)
        elementalTimers[guid] = nil
    end
end

local function resetElemental(guid)
    cancelElementalPump(guid)
    elementalState[guid] = nil
end

-- C++ Initialize(): poisonBolt {5s,10s}, despawn 30s.
local function freshElementalState()
    return {
        poisonBolt = 5000 + math.random(0, 4999),
        despawn = 30000,
    }
end

local function elementalTick(creature, guid)
    local st = elementalState[guid]
    if not st then
        return
    end

    -- Poison bolt: random alive player within 30 yd; nil or
    -- out-of-range pick casts nothing, timer re-arms (C++-exact).
    if st.poisonBolt <= 1000 then
        local pick = pickRandomAlivePlayerInRange(creature, POISON_BOLT_RANGE)
        if pick then
            creature:CastSpell(pick, SPELL_POISON_BOLT)
        end
        st.poisonBolt = 5000 + math.random(0, 4999)
    else
        st.poisonBolt = st.poisonBolt - 1000
    end

    -- 30s lifespan, then gone (C++ setDeathState(DEAD)).
    if st.despawn <= 1000 then
        creature:Despawn()
        resetElemental(guid)
    else
        st.despawn = st.despawn - 1000
    end
end

local function elementalEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetElemental(guid)
    elementalState[guid] = freshElementalState()
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 1000)
end

local function elementalCleanup(event, creature)
    resetElemental(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 1, elementalEnterCombat)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 2, elementalCleanup)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 4, elementalCleanup)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 23, elementalCleanup)

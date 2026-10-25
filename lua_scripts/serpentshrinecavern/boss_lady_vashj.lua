-- Lady Vashj (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_lady_vashj.cpp (boss_lady_vashj, npc_enchanted_elemental,
-- npc_tainted_elemental, npc_toxic_sporebat,
-- npc_shield_generator_channel — documented below; three entries
-- carry handlers (21212, 22009, 19870), the other two documented
-- only; item_tainted_core documented below, not
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
-- =7 (death). SAY_INTRO=0 fires from the MoveInLineOfSight intro
-- (event 27, per-GUID constructor latch — "no need do intro if
-- wiped"); SAY_PHASE1=2 has no Lua constant (C++-only enum member,
-- never talked in C++ either — omission correct). SAY_PHASE3=4
-- likewise has no constant: its branch is instance-gate-blocked (D1).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset, 27 OnMoveInLineOfSight. Timers via CreateLuaEvent
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
-- for all C++ timers). The instance SetData(IN_PROGRESS) arm stays
-- blocked; the per-player DestroyItemCount(31088) tainted-core sweep
-- is PORTED on engage (DestroyItemCount is bridged, D5). Phase 1:
-- Shock blast
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
-- MotionMaster Clear stays out (no bridge); the DoTeleportTo and
-- the four shield-generator-channel summons are PORTED below.
-- CheckTimer 15s init then
-- 5s: if no alive player in the instance is within melee range (the
-- C++ threat-list IsWithinMeleeRange scan — GetAITargets is the bridged
-- threat list, code already uses it) -> CastShoot
-- OrMultishot. CastShootOrMultishot: urand(0,1) -> Shoot 40873
-- or Multishot 38310 DoCastVictim (C++-exact); Talk(SAY_BOWSHOT)
-- only when math.random(0,2) != 0 (the C++ rand32()%3 gate).
-- Phase 2: Forked lightning 40088 2s then {2s,8s} — random alive
-- player pick, victim fallback (C++-exact). The enchanted/
-- tainted summon arms are ported below, as are the Coilfang-elite/
-- Coilfang-strider summon arms (2026-10-25); the phase-3 CheckTimer
-- poll is instance-
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
-- ATTACKABLE MoveInLineOfSight machine — the flag arms are PORTED
-- (2026-10-20); the instance bookkeeping stays skipped);
-- the passive summon legs are PORTED (the stale "no summon bridge"
-- claim is retracted): 4x shield-generator-channel (19870) at the
-- phase-2 transition, enchanted-elemental (21958, cycling ElementPos)
-- and tainted-elemental (22009, random ElementPos) on their C++-exact
-- timers, and the Coilfang-elite (22055) / Coilfang-strider (22056)
-- phase-2 summon arms are PORTED below (SummonCreature type 3, 5s +
-- EngageWithTarget, kargath-guard precedent — the old "sole role is
-- combat" stay-out is retracted). STAY-OUT: sporebat (22140) — phase 3
-- is unreachable (instance DATA_CANSTARTPHASE3 gate), so its summon
-- arm can never fire;
-- the phase-2 MotionMaster Clear stays out (no bridge) but its
-- DoTeleportTo is PORTED in the phase-2 arm below (2026-10-24 — the old
-- "no teleport bridge" claim was stale); the enchanted-elemental MovePoint
-- waypoint machine stays unmodeled (the enchanted elemental is movement-driven
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
-- via instance GUID — no cross-creature bridge, stays out; the
-- Reset() self-local arms — SetDisplayId(11686) invisible +
-- NOT_SELECTABLE flag — are PORTED below via a new ev23
-- registration (display/flag bridges live 2026-10-20)),
-- and the tainted-elemental
-- JustDied -> EventTaintedElementalDeath relay PORTED below
-- (FindNearestCreature + timer clamp); the melee-range check is
-- the 5-yd player stand-in; the npc_tainted_elemental
-- JustEngagedWith AddThreat(0.1f) arm (boss_lady_vashj.cpp:689-692,
-- misattributed to vashj here before) is PORTED below.
-- no item bridges for item_tainted_core (Tainted Core 31088):
-- the OnUse arm needs instance->GetData(DATA_SHIELDGENERATOR1.
-- .4), cross-creature channel kill via the vashj AI's stored
-- channel GUIDs, player->CastSpell(38134) on a player target,
-- DestroyItemCount(31088) and GameObject-target resolution —
-- none bridgeable, documented only, not registered; phase 2's
-- "no victim + in combat -> EnterEvadeMode" arm has no evade
-- bridge — unmodeled.
-- npc_tainted_elemental (22009) port: OnEnterCombat(1) per-GUID
-- reset + 1s pump + JustEngagedWith AddThreat(0.1f) PORTED
-- (elementalEnterCombat); Poison bolt 40095 {5s,10s} then {5s,10s} —
-- random alive player pick, cast only when within 30 yd (C++
-- SelectTarget(Random, 0) + IsWithinDistInMap(me, 30): gate
-- after pick, so an out-of-range pick wastes the round with no
-- cast; timer re-arms regardless). C++ playerOnly defaults false
-- (pets etc. eligible): the pick is the bridged threat list
-- (GetAITargets, D4), not the teron player-only pick. DespawnTimer
-- 30s ->
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

local SAY_INTRO = 0
local SAY_AGGRO = 1
local SAY_PHASE2 = 3
local SAY_BOWSHOT = 5
local SAY_SLAY = 6
local SAY_DEATH = 7

local ENTRY_LADY_VASHJ = 21212

-- UnitDefines.h:125; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:690/725).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local ENTRY_TAINTED_ELEMENTAL = 22009
local ENTRY_SHIELD_GENERATOR_CHANNEL = 19870 -- boss_lady_vashj.cpp:60 (C++ spells it SHIED_GENERATOR_CHANNEL)
local ENTRY_ENCHANTED_ELEMENTAL = 21958
local ENTRY_COILFANG_ELITE = 22055
local ENTRY_COILFANG_STRIDER = 22056

-- boss_lady_vashj.cpp:69-71 (MIDDLE_X/Y/Z): the platform center Vashj
-- teleports to at the phase-2 transition (:401-402).
local VASHJ_MIDDLE_X = 30.134
local VASHJ_MIDDLE_Y = -923.65
local VASHJ_MIDDLE_Z = 42.9

-- boss_lady_vashj.cpp:133-139 (ShieldGeneratorChannelPos) and
-- :83-92 (ElementPos), literal tables.
local SHIELD_GENERATOR_CHANNEL_POSITIONS = {
    {49.6262, -902.181, 43.0975, 3.95683},
    {10.988, -901.616, 42.5371, 5.4373},
    {10.3859, -944.036, 42.5446, 0.779888},
    {49.3126, -943.398, 42.5501, 2.40174},
}
local ELEMENT_POSITIONS = {
    {8.3, -835.3, 21.9, 5.0},
    {53.4, -835.3, 21.9, 4.5},
    {96.0, -861.9, 21.8, 4.0},
    {96.0, -986.4, 21.4, 2.5},
    {54.4, -1010.6, 22, 1.8},
    {9.8, -1012, 21.7, 1.4},
    {-35.0, -987.6, 21.5, 0.8},
    {-58.9, -901.6, 21.5, 6.0},
}

-- boss_lady_vashj.cpp:119-124 (CoilfangElitePos) and :126-131
-- (CoilfangStriderPos), literal tables.
local COILFANG_ELITE_POSITIONS = {
    {28.84, -923.28, 42.9, 6.0},
    {31.183281, -953.502625, 41.523602, 1.640957},
    {58.895180, -923.124268, 41.545307, 3.152848},
}
local COILFANG_STRIDER_POSITIONS = {
    {66.427010, -948.778503, 41.262245, 2.584220},
    {7.513962, -959.538208, 41.300422, 1.034629},
    {-12.843201, -907.798401, 41.239620, 6.087094},
}

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

-- C++ SelectTarget(Random, 0): random unit from the THREAT list
-- (any unit, pets included) — GetAITargets is the bridged threat
-- list (lua_creature_events.go:1705). Used for forked/poison/
-- Coilfang arms (2026-10-25 audit D4; the instance-player pick is
-- retired for these arms).
local function pickRandomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- C++ SelectTarget(Random, 0, <dist>, true): random THREAT-LIST
-- player within <dist> yd (GetTypeId() == 4 is TYPEID_PLAYER).
local function pickRandomThreatPlayerInRange(creature, maxDist)
    local found = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetTypeId() == 4 and creature:GetDistance(unit) <= maxDist then
            found[#found + 1] = unit
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

        -- Static Charge: random threat-list player within 200 yd,
        -- cast only when the pick lacks the trigger aura; re-arm
        -- {10s,30s} regardless (C++-exact).
        if st.staticCharge <= 1000 then
            local pick = pickRandomThreatPlayerInRange(creature, STATIC_CHARGE_RANGE)
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
        -- evaluated every UpdateAI). PORTED (boss_lady_vashj.cpp:393-408):
        -- MotionMaster Clear has no bridge (skipped), then DoTeleportTo
        -- the platform middle via creature:Teleport (the old "teleport
        -- has no bridge" claim was stale), 4x SummonCreature(
        -- SHIED_GENERATOR_CHANNEL 19870 at ShieldGeneratorChannelPos,
        -- TEMPSUMMON_CORPSE_DESPAWN Eluna type 4), then Talk(SAY_PHASE2).
        -- The stored channel returns feed the ev23 Reset cleanup
        -- (D3); the instance SetData bookkeeping stays blocked.
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 70 / 100) then
            creature:Teleport(VASHJ_MIDDLE_X, VASHJ_MIDDLE_Y, VASHJ_MIDDLE_Z)
            -- Store channel returns: C++ Reset kills each stored
            -- channel (setDeathState JUST_DIED); Lua despawns them
            -- on ev23 (D3).
            st.channels = {}
            for _, pos in ipairs(SHIELD_GENERATOR_CHANNEL_POSITIONS) do
                local ch = creature:SummonCreature(ENTRY_SHIELD_GENERATOR_CHANNEL,
                    pos[1], pos[2], pos[3], pos[4], 4, 0)
                if ch then
                    st.channels[#st.channels + 1] = ch
                end
            end
            creature:Talk(SAY_PHASE2)
            st.phase = 2
            -- Enchanted/tainted summon arms (:474-495) run in the C++
            -- phase-2 branch; like the elite/strider timers below they
            -- decrement only there, so the C++ init values still hold
            -- at transition and are seeded here.
            st.enchantedElementalPos = 0
            st.enchantedElemental = 5000
            st.taintedElemental = 50000
            st.enchanted = {}
            -- Coilfang elite/strider timers (C++ Initialize :173-174):
            -- elite 45000+rand5000, strider 60000+rand10000.
            st.coilfangElite = 45000 + math.random(0, 4999)
            st.coilfangStrider = 60000 + math.random(0, 9999)
        end

        -- CheckTimer (boss_lady_vashj.cpp:438-448): if nobody on the
        -- threat list is in melee range, Shoot/Multishot.
        -- creature:GetAITargets() is the bridged threat list
        -- (lua_creature_events.go); melee range is creature:IsWithinMeleeRange,
        -- replacing the retired
        -- "no threat bridge" instance-player approx 2026-10-23.
        if st.check <= 1000 then
            local inMeleeRange = false
            for _, unit in ipairs(creature:GetAITargets()) do
                if creature:IsWithinMeleeRange(unit) then
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
    elseif st.phase == 2 then
        -- Phase 2: forked lightning 2s then {2s,8s} — random
        -- threat-list pick, victim fallback (C++-exact). Phase 3 has
        -- no branch: entry needs DATA_CANSTARTPHASE3 (instance-
        -- blocked), and this guard keeps phase-2 arms from running
        -- under any other phase value.
        if st.forkedLightning <= 1000 then
            local pick = pickRandomThreatTarget(creature)
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

        -- Enchanted elementals (:476-486): SummonCreature(ENCHANTED_
        -- ELEMENTAL 21958 at ElementPos[EnchantedElementalPos], then
        -- the index cycles 0..7; TEMPSUMMON_CORPSE_DESPAWN Eluna
        -- type 4; re-arm 10000+rand5000 (C++-exact).
        if st.enchantedElemental <= 1000 then
            local pos = ELEMENT_POSITIONS[st.enchantedElementalPos + 1]
            local el = creature:SummonCreature(ENTRY_ENCHANTED_ELEMENTAL,
                pos[1], pos[2], pos[3], pos[4], 4, 0)
            -- Stored for the D7 mitigation: C++ elementals KillSelf
            -- once vashj leaves combat or dies; Lua despawns them on
            -- leave/reset/died (the MovePoint/surge machine itself
            -- stays out, no bridge).
            if el then
                st.enchanted[#st.enchanted + 1] = el
            end
            st.enchantedElementalPos = (st.enchantedElementalPos == 7) and 0 or (st.enchantedElementalPos + 1)
            st.enchantedElemental = 10000 + math.random(0, 4999)
        else
            st.enchantedElemental = st.enchantedElemental - 1000
        end

        -- Tainted elementals (:488-495): SummonCreature(TAINTED_
        -- ELEMENTAL 22009 at ElementPos[rand8], TEMPSUMMON_DEAD_DESPAWN
        -- Eluna type 6); re-arm 120000 (C++-exact). Delta: the
        -- EventTaintedElementalDeath 50s-after-death clamp is PORTED
        -- in elementalDied below (FindNearestCreature + min(timer, 50000)).
        if st.taintedElemental <= 1000 then
            local pos = ELEMENT_POSITIONS[math.random(1, 8)]
            creature:SummonCreature(ENTRY_TAINTED_ELEMENTAL,
                pos[1], pos[2], pos[3], pos[4], 6, 0)
            st.taintedElemental = 120000
        else
            st.taintedElemental = st.taintedElemental - 1000
        end

        -- Coilfang elites (:497-510): SummonCreature(COILFANG_ELITE
        -- 22055 at CoilfangElitePos[rand3], TEMPSUMMON_TIMED_DESPAWN_
        -- OUT_OF_COMBAT Eluna type 3, 5s) + AttackStart(random target,
        -- victim fallback) via EngageWithTarget (kargath-guard
        -- precedent); re-arm 45000+rand5000 (C++-exact).
        if st.coilfangElite <= 1000 then
            local pos = COILFANG_ELITE_POSITIONS[math.random(1, 3)]
            local elite = creature:SummonCreature(ENTRY_COILFANG_ELITE,
                pos[1], pos[2], pos[3], pos[4], 3, 5000)
            if elite then
                local pick = pickRandomThreatTarget(creature)
                if not pick then
                    pick = creature:GetVictim()
                end
                if pick then
                    elite:EngageWithTarget(pick)
                end
            end
            st.coilfangElite = 45000 + math.random(0, 4999)
        else
            st.coilfangElite = st.coilfangElite - 1000
        end

        -- Coilfang striders (:512-524): same shape as elites, entry
        -- 22056 at CoilfangStriderPos[rand3], re-arm 60000+rand10000.
        if st.coilfangStrider <= 1000 then
            local pos = COILFANG_STRIDER_POSITIONS[math.random(1, 3)]
            local strider = creature:SummonCreature(ENTRY_COILFANG_STRIDER,
                pos[1], pos[2], pos[3], pos[4], 3, 5000)
            if strider then
                local pick = pickRandomThreatTarget(creature)
                if not pick then
                    pick = creature:GetVictim()
                end
                if pick then
                    strider:EngageWithTarget(pick)
                end
            end
            st.coilfangStrider = 60000 + math.random(0, 9999)
        else
            st.coilfangStrider = st.coilfangStrider - 1000
        end
    end
end

-- Despawn stored phase-2 summons; MUST run before resetVashj nils
-- state. withChannels=true only on the ev23 Reset path (C++ Reset
-- kills channels; JustDied does not, and leave-combat has no
-- channel arm — elementals KillSelf on any vashj-out-of-combat/dead
-- leg, so they despawn on all three paths).
local function despawnStoredSummons(guid, withChannels)
    local st = vashjState[guid]
    if not st then
        return
    end
    if withChannels and st.channels then
        for _, ch in ipairs(st.channels) do
            if ch then
                ch:Despawn()
            end
        end
    end
    if st.enchanted then
        for _, el in ipairs(st.enchanted) do
            if el then
                el:Despawn()
            end
        end
    end
end

local function vashjEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetVashj(guid)
    vashjState[guid] = freshVashjState()
    -- C++ JustEngagedWith tainted-core sweep (D5): every map player
    -- loses one Tainted Core (31088); DestroyItemCount is bridged
    -- (lua_player.go:360). The SetData(IN_PROGRESS) leg stays
    -- instance-blocked. (Sighting-engage also lands here via ev1, a
    -- minor over-fire vs C++'s engage-only sweep.)
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == creature:GetMapId() and p:GetInstanceId() == creature:GetInstanceId() then
            p:DestroyItemCount(31088, 1, true)
        end
    end
    creature:Talk(SAY_AGGRO)
    vashjTimers[guid] = CreateLuaEvent(function()
        vashjTick(creature, guid)
    end, 1000, 0)
end

local function vashjLeaveCombat(event, creature)
    -- C++ elementals KillSelf when vashj leaves combat; channels are
    -- Reset-only (not leave-combat), so only enchanted despawn here.
    despawnStoredSummons(creature:GetGUID(), false)
    resetVashj(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY), no TYPEID gate.
local function vashjTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function vashjDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ elementals KillSelf on vashj death; channels are untouched
    -- by JustDied (CORPSE_DESPAWN is engine-side).
    despawnStoredSummons(creature:GetGUID(), false)
    resetVashj(creature:GetGUID())
end

local function vashjReset(event, creature)
    -- C++ Reset kills each stored shield channel (JUST_DIED) and the
    -- elementals KillSelf once vashj is out of combat (D3/D7).
    despawnStoredSummons(creature:GetGUID(), true)
    resetVashj(creature:GetGUID())
    -- C++ boss_lady_vashjAI::Reset has no speed leg: the MOVE_WALK /
    -- MOVE_RUN 0.6f pair (:591-592) belongs to npc_enchanted_elementalAI::
    -- Reset, and the elemental is unregistered (movement-driven, header).
    -- A prior port wrongly applied the run rate to vashj here; removed.
end

-- C++ MoveInLineOfSight: one-shot Talk(SAY_INTRO 0) on first sight,
-- constructor-latched (Intro cleared only in the AI constructor —
-- "no need do intro if wiped"), so the latch is per-GUID and never
-- cleared on 2/23 (ossirian convention; boss_lady_vashj.cpp:275-282).
-- The Go fire site is the aggro-engage branch (hostile/attackable
-- faction), a subset of C++'s any-unit LOS trigger — acceptable for
-- a one-shot intro line (ossirian precedent).
local vashjSaidIntro = {}

local function vashjMoveInLOS(event, creature, unit)
    local guid = creature:GetGUID()
    if not vashjSaidIntro[guid] then
        vashjSaidIntro[guid] = true
        creature:Talk(SAY_INTRO)
        -- boss_lady_vashj.cpp:160 — the C++ ctor sets
        -- UNIT_FLAG_NON_ATTACKABLE "only once on Creature create";
        -- the 19s AggroTimer arm (:325-338) clears it. Never canceled
        -- on 2/4/23 — C++ never cancels AggroTimer; the flag gates
        -- every aggro path, so no combat (and no evade) can intervene
        -- before it fires. Ported 2026-10-20.
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        CreateLuaEvent(function()
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        end, 19000)
    end
end

RegisterCreatureEvent(ENTRY_LADY_VASHJ, 1, vashjEnterCombat)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 2, vashjLeaveCombat)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 3, vashjTargetDied)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 4, vashjDied)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 23, vashjReset)
RegisterCreatureEvent(ENTRY_LADY_VASHJ, 27, vashjMoveInLOS)

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

    -- Poison bolt: random threat-list pick, cast only when within
    -- 30 yd — an out-of-range pick wastes the round (C++ SelectTarget
    -- (Random, 0) + IsWithinDistInMap(me, 30) gate-after-pick; timer
    -- re-arms regardless). GetAITargets is the bridged threat list
    -- (2026-10-25 audit D4; the old teron player-only pick is retired).
    if st.poisonBolt <= 1000 then
        local pick = pickRandomThreatTarget(creature)
        if pick and creature:IsWithinDistInMap(pick, POISON_BOLT_RANGE) then
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

local function elementalEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_lady_vashj.cpp:694: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    -- C++ npc_tainted_elemental::JustEngagedWith (boss_lady_vashj.cpp:689-692).
    if target ~= nil then
        creature:AddThreat(target, 0.1)
    end
    local guid = creature:GetGUID()
    resetElemental(guid)
    elementalState[guid] = freshElementalState()
    elementalTimers[guid] = CreateLuaEvent(function()
        elementalTick(creature, guid)
    end, 1000, 0)
end

local function elementalCleanup(event, creature)
    resetElemental(creature:GetGUID())
end

-- C++ npc_tainted_elemental::JustDied (boss_lady_vashj.cpp:685):
-- instance->GetGuidData(DATA_LADYVASHJ) -> EventTaintedElementalDeath
-- -> TaintedElementalTimer = min(timer, 50000) ("the next will spawn
-- 50 seconds after the previous one's death"). The receiver lookup is
-- bridged: FindNearestCreature (maulgar precedent) returns the live
-- vashj, nil when dead or absent; vashjState is shared in-file, so the
-- clamp lands directly on the pump's countdown.
local function elementalDied(event, creature)
    resetElemental(creature:GetGUID())
    local vashj = creature:FindNearestCreature(ENTRY_LADY_VASHJ, 100.0)
    if vashj ~= nil then
        local st = vashjState[vashj:GetGUID()]
        if st ~= nil and st.taintedElemental ~= nil and st.taintedElemental > 50000 then
            st.taintedElemental = 50000
        end
    end
end

RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 1, elementalEnterCombat)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 2, elementalCleanup)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 4, elementalDied)
RegisterCreatureEvent(ENTRY_TAINTED_ELEMENTAL, 23, elementalCleanup)

-- C++ npc_shield_generator_channel::Reset (boss_lady_vashj.cpp:853-859):
-- SetDisplayId(11686) // invisible + SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE). Both arms are self-local and bridged
-- (creature:SetDisplayId, lua_creature_events.go:643; creature:SetFlag,
-- lua_creature_events.go:968) — the stale "no cross-creature bridge"
-- framing retracted for the Reset arm (it never touches vashj). The
-- UpdateAI barrier upkeep (DoCast(vashj, SPELL_MAGIC_BARRIER 38112,
-- true) via instance GUID) rides the unbridged cross-creature bridge
-- and stays out.
local function shieldGeneratorReset(event, creature)
    creature:SetDisplayId(11686)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end
RegisterCreatureEvent(ENTRY_SHIELD_GENERATOR_CHANNEL, 23, shieldGeneratorReset)
RegisterCreatureEvent(ENTRY_SHIELD_GENERATOR_CHANNEL, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_lady_vashj.cpp:864: custom UpdateAI never calls DoMeleeAttackIfReady (barrier-upkeep machine in C++, never melees)
end)

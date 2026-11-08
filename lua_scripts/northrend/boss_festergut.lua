-- Festergut (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_festergut.cpp
-- (476 lines incl. license; 2 CreatureScripts
-- (boss_festergut (BossAI, DATA_FESTERGUT = 4),
-- npc_stinky_icc (ScriptedAI, Stinky)) + 2 SpellScripts
-- (spell_festergut_pungent_blight, spell_festergut_gastric_bloat)
-- + 1 AuraScript (spell_festergut_blighted_spores) + 1
-- AchievementCriteriaScript (achievement_flu_shot_shortage); all
-- registered from inside AddSC_boss_festergut(); loader decl 175 /
-- call 370 per northrend_script_loader.cpp — the FIFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_deathbringer_saurfang() (call 369); the call
-- after it is AddSC_boss_rotface() (call 371) — verified from the
-- loader this run; the checkpoint sequence
-- (boss_deathbringer_saurfang -> boss_festergut) is followed).
-- Entry: 36626 Festergut (icecrown_citadel.h NPC_FESTERGUT, line
-- 222; icecrown_citadel.h DATA_FESTERGUT = 4, line 78;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_FESTERGUT, line 238) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_festergut" hits boss_festergut.cpp (+ the loader
-- decl/call lines) only; this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No festergut lua
-- existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36626):
-- boss_festergut UpdateAI — Talk(SAY_PUNGENT_BLIGHT 4) /
-- Talk(EMOTE_WARN_PUNGENT_BLIGHT 5) ride the EVENT_INHALE_BLIGHT
-- scheduler leg, Talk(EMOTE_GAS_SPORE 2) /
-- Talk(EMOTE_WARN_GAS_SPORE 3) ride the EVENT_GAS_SPORE leg,
-- PORTED (2026-10-13): EVENT_GASTRIC_BLOAT — DoCastVictim(
-- SPELL_GASTRIC_BLOAT 72219) non-triggered, {12500,15000} init /
-- {15000,17500} re-arm (maiden CreateLuaEvent convention);
-- EVENT_BERSERK — single-fire 5min — Talk(SAY_BERSERK 8) +
-- DoCastSelf(SPELL_BERSERK2 47008) non-triggered.
-- PORTED (2026-10-14): EVENT_INHALE_BLIGHT — lua-local counter
-- machine 0->3 (rotface-hasten precedent); non-3rd DoCastSelf(
-- SPELL_INHALE_BLIGHT 69165) non-triggered, then ++counter, then a
-- non-triggered self-cast of gaseousBlight[counter] (69162/69164 —
-- the real inhale stack aura, :199-203; ported 2026-10-21); 3rd
-- inhale Talk(
-- EMOTE_WARN_PUNGENT_BLIGHT 5) + Talk(SAY_PUNGENT_BLIGHT 4) +
-- DoCastSelf(SPELL_PUNGENT_BLIGHT 69195) non-triggered, counter
-- reset, RescheduleEvent(EVENT_GAS_SPORE, 20s..25s); init
-- {25000,30000}, re-arm {33500,35000} both branches (:184-207).
-- RemoveBlight() PORTED (2026-10-21): nearest alive NPC_GAS_DUMMY
-- (36659) within 100yd via GetCreaturesInGrid (the FindNearestCreature
-- twin; RemoveAura bridged, lua_creature_events.go:158) — strips
-- gaseousBlight[3] from the boss + gaseousBlightVisual[3] from the
-- dummy, wired into Reset (:101-110) and JustDied (:113-119).
-- OUT legs inside the arm: gaseous-blight visual triggered casts
-- (:202, aura legs without the removal bridge), DoAction(
-- ACTION_FESTERGUT_GAS) cross-AI (:193-194).
-- PORTED (2026-10-14): EVENT_GAS_SPORE — Talk(
-- EMOTE_WARN_GAS_SPORE 3) + Talk(EMOTE_GAS_SPORE 2) + CastSpell(
-- creature, SPELL_GAS_SPORE 69278) non-triggered; init
-- {20000,25000}, re-arm {40000,45000} (:235-240). MAX_TARGETS
-- spell-mod leg (:238) has no Lua bridge — the DB spell is
-- raid-moded with the same 2/3 values, so the default cast is
-- faithful.
-- PORTED (2026-10-21): EVENT_VILE_GAS (:209-233, first fire from the
-- gas-spore arm's RescheduleEvent :240) — alive-player pools split by
-- reach-adjusted 5.0yd (raw dist minus boss combat reach), melee top-up
-- to 10-man minTargets 3 (25-man 8 has no bridge,
-- hodir precedent), 1 random target (25-man 3), DoCast 69240
-- non-triggered, re-arm {28s,35s} unconditional; GetAITargets +
-- GetDistance bridges (putricide/moroes precedents).
-- npc_stinky_icc::JustDied — festergut->AI()->Talk(SAY_STINKY_DEAD
-- 0) is cross-AI Talk with no bridge; NOT registered. Bridgeable-
-- in-principle but ENTRY-BLOCKED (the pit_of_saron flamebearer
-- precedent): no NPC_STINKY entry constant anywhere in-tree (this
-- clone has no TDB; RegisterCreatureEvent needs an entry ID).
-- The blocked arms: JustEngagedWith DoCastSelf(SPELL_PLAGUE_STENCH
-- 71805) 2-arg non-triggered (:304); EVENT_DECIMATE DoCastVictim(
-- SPELL_DECIMATE 71123) non-triggered {20s,25s} init/re-arm
-- (:321-323); EVENT_MORTAL_WOUND DoCastVictim(SPELL_MORTAL_WOUND
-- 71127) non-triggered {3s,7s} init / {10s,12.5s} re-arm (:325-327).
-- spell_festergut_pungent_blight::HandleScriptEffect —
-- festergut->AI()->Talk(EMOTE_PUNGENT_BLIGHT 6) is cross-AI Talk
-- riding a SpellScript — no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent);
-- spell_festergut_gastric_bloat joins the same queue.
-- spell_festergut_blighted_spores (AuraScript, cross-AI
-- festergut->AI()->SetData(DATA_INOCULATED_STACK) leg) PORTED tick
-- 461 via RegisterAuraEvent (ev3) below; achievement_flu_shot_shortage
-- PORTED tick 461 via the Go case in achievement_criteria_scripts.go.
-- Both dormant until Spell.dbc ships the Gas Spore 69278 ->
-- Blighted Spores 69290 trigger link.
-- boss_festergut JustEngagedWith legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- FindNearestCreature(NPC_GAS_DUMMY) stay out (no instance bridge);
-- professor DoAction(ACTION_FESTERGUT_COMBAT) stays out 2026-11-03:
-- SetPhase + MovePoint + REACT_PASSIVE + EngagementStart + heroic
-- goo timer is the professor's phase machine, and a movement-only
-- port would strand him (the DEATH return rides the unported timer
-- + EnterEvadeMode, no bridge). DoZoneInCombat :128 is PORTED
-- 2026-10-23 via the live bridge — C++ arm order, last in the
-- handler. Cross-AI relays PORTED 2026-11-03 via the grid-sweep
-- relayProfessor: ACTION_FESTERGUT_GAS (Talk + triggered visual,
-- in onInhaleBlight) and the ACTION_FESTERGUT_DEATH Talk leg at 4s
-- (in festergutJustDied; its EnterEvadeMode leg stays out, no
-- bridge). JustReachedHome instance->SetBossState(FAIL) (:141-144)
-- PORTED 2026-11-05 via creature:SetBossState on ev24 OnReachHome
-- (the opera ev24 precedent); the EnterEvadeMode cross-AI professor
-- EnterEvadeMode leg stays out (no bridge). NO engage IN_PROGRESS
-- leg exists: C++ JustEngagedWith sets no state — IN_PROGRESS is
-- written by Putricide's MovementInform POINT_FESTERGUT
-- (boss_professor_putricide.cpp:376) when he reaches the table, i.e.
-- it rides the unported ACTION_FESTERGUT_COMBAT chain above.
-- SpellHitTarget (boss_festergut.cpp:160-168) — STAYS OUT, retracted
-- 2026-10-20: the 11:55 tick's ev15 port stripped INOCULATED from the
-- boss himself (Go's cast path carries only the explicit target, so the
-- self-cast trigger delivers target=self); C++ strips it from the
-- players hit by the blight AoE, whom Go never enumerates. Fabricating
-- the per-player legs would invent hit targets — trigger-path blocked.
-- All 10 Talk() calls in the file accounted for (10 =
-- SAY_STINKY_DEAD 0 (1, cross-AI from npc_stinky_icc) + SAY_AGGRO 1
-- (1) + EMOTE_GAS_SPORE 2 (1) + EMOTE_WARN_GAS_SPORE 3 (1) +
-- SAY_PUNGENT_BLIGHT 4 (1) + EMOTE_WARN_PUNGENT_BLIGHT 5 (1) +
-- EMOTE_PUNGENT_BLIGHT 6 (1, from the SpellScript) + SAY_KILL 7
-- (1) + SAY_BERSERK 8 (1) + SAY_DEATH 9 (1)).

local ENTRY_FESTERGUT = 36626
local DATA_FESTERGUT = 4 -- icecrown_citadel.h:78
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SAY_AGGRO = 1
local SAY_KILL = 7
local SAY_DEATH = 9
local SAY_BERSERK = 8

local SPELL_GASTRIC_BLOAT = 72219
local SPELL_BERSERK2 = 47008
local SPELL_INHALE_BLIGHT = 69165
local SPELL_PUNGENT_BLIGHT = 69195
local SPELL_GAS_SPORE = 69278
local SPELL_INOCULATED = 69291 -- boss_festergut.cpp:52
local SPELL_ORANGE_BLIGHT_RESIDUE = 72144 -- icecrown_citadel.h:41
local DATA_INOCULATED_STACK = 69291 -- boss_festergut.cpp:81
local QUEST_RESIDUE_RENDEZVOUS_10 = 24873 -- icecrown_citadel.h:521
local QUEST_RESIDUE_RENDEZVOUS_25 = 24878 -- icecrown_citadel.h:522
local QUEST_STATUS_INCOMPLETE = 3 -- lua_player.go GetQuestStatus: NONE 0, COMPLETE 1, INCOMPLETE 3
local BLIGHTED_SPORES_AURAS = { 69290, 71222, 73033, 73034 } -- spell_script_names: spell_festergut_blighted_spores_aura (four raid-mode variants; 69290 verified on Wowhead WotLK, "Triggered by Gas Spore" 69278)
local SPELL_VILE_GAS = 69240
local EMOTE_WARN_PUNGENT_BLIGHT = 5
local EMOTE_PUNGENT_BLIGHT = 6
local SAY_PUNGENT_BLIGHT = 4
local EMOTE_WARN_GAS_SPORE = 3
local EMOTE_GAS_SPORE = 2

local NPC_GAS_DUMMY = 36659

local ENTRY_PROFESSOR_PUTRICIDE = 36678
local SAY_FESTERGUT_GASEOUS_BLIGHT = 0
local SAY_FESTERGUT_DEATH = 1
local SPELL_RELEASE_GAS_VISUAL = 69125

-- gaseousBlight[3] / gaseousBlightVisual[3] (boss_festergut.cpp:64-65):
-- the inhale stack auras on Festergut and their gas-dummy visuals.
local GASEOUS_BLIGHT = {69157, 69162, 69164}
local GASEOUS_BLIGHT_VISUAL = {69126, 69152, 69154}

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

-- C++ RemoveBlight() (boss_festergut.cpp:113-119; Reset :101-110
-- inlines the same loop): if an alive NPC_GAS_DUMMY is within 100yd,
-- strip the gaseous-blight auras from the boss and the visual auras
-- from the dummy. FindNearestCreature's nearest-alive twin: the
-- nearest GetCreaturesInGrid match (the mandokir IsAlive() precedent);
-- RemoveAura is bridged (lua_creature_events.go:158).
local function removeBlight(creature)
    local nearest, nearestDist
    for _, dummy in ipairs(creature:GetCreaturesInGrid(NPC_GAS_DUMMY, 100)) do
        if dummy:IsAlive() then
            local dist = creature:GetDistance(dummy)
            if not nearest or dist < nearestDist then
                nearest, nearestDist = dummy, dist
            end
        end
    end
    if nearest then
        for _, spell in ipairs(GASEOUS_BLIGHT) do
            creature:RemoveAura(spell)
        end
        for _, spell in ipairs(GASEOUS_BLIGHT_VISUAL) do
            nearest:RemoveAura(spell)
        end
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

-- C++ RescheduleEvent emulation: cancel one key's pending event
-- (EVENT_GAS_SPORE rescheduled from the 3rd EVENT_INHALE_BLIGHT,
-- boss_festergut.cpp:195).
local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ EVENT_GASTRIC_BLOAT: DoCastVictim(SPELL_GASTRIC_BLOAT)
-- non-triggered — init {12500,15000}, re-arm {15000,17500}
-- (boss_festergut.cpp:94/:243-244); maiden CreateLuaEvent timer
-- convention; victim-cast via CastSpell(nil, ...).
local function onGastricBloat(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :177 (+ in-loop :254, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "gastric_bloat", 500, function() onGastricBloat(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_GASTRIC_BLOAT)
    schedule(guid, "gastric_bloat", {15000, 17500}, function() onGastricBloat(creature, guid) end)
end

-- C++ EVENT_BERSERK: single-fire 5min — DoCastSelf
-- (SPELL_BERSERK2) non-triggered then Talk(SAY_BERSERK)
-- (:93/:246-248); cast-before-talk order kept.
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :177 (+ in-loop :254, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK2)
    creature:Talk(SAY_BERSERK)
end

local inhale = {}

-- Forward declaration: onInhaleBlight's pungent-blight re-arm
-- closure references onGasSpore before its local def below;
-- without this the closure would bind a nil global (moam/
-- malown/algalon/devourer_of_souls nil-call class).
local onGasSpore

-- C++ EVENT_INHALE_BLIGHT (:183-207): lua-local counter machine
-- 0->3 (rotface-hasten precedent); non-3rd DoCastSelf(
-- SPELL_INHALE_BLIGHT) non-triggered (:199); 3rd inhale Talk(
-- EMOTE_WARN_PUNGENT_BLIGHT) + Talk(SAY_PUNGENT_BLIGHT) +
-- DoCastSelf(SPELL_PUNGENT_BLIGHT) non-triggered (:189-191),
-- counter reset, RescheduleEvent(EVENT_GAS_SPORE, 20s..25s)
-- (:195). The :206 re-arm (33500ms..35s) applies to both branches.
-- Cross-AI DoAction relay (2026-11-03 strip): C++ reaches Professor
-- Putricide via instance->GetGuidData(DATA_PROFESSOR_PUTRICIDE); Lua
-- has no instance bridge, so the relay sweeps the 100yd grid for the
-- professor (the shade/saurfang grid-sweep precedent — exactly one
-- Putricide exists, so the first alive match is the C++ target).
local function relayProfessor(creature, fn)
    for _, professor in ipairs(creature:GetCreaturesInGrid(ENTRY_PROFESSOR_PUTRICIDE, 100)) do
        if professor:IsAlive() then
            fn(professor)
            return
        end
    end
end

local function onInhaleBlight(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :177 (+ in-loop :254, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "inhale_blight", 500, function() onInhaleBlight(creature, guid) end)
        return
    end
    -- C++ :185-186: RemoveBlight() fires at the top of EVERY inhale
    -- tick, before the counter check.
    removeBlight(creature)
    local count = inhale[guid] or 0
    if count == 3 then
        creature:Talk(EMOTE_WARN_PUNGENT_BLIGHT)
        creature:Talk(SAY_PUNGENT_BLIGHT)
        creature:CastSpell(creature, SPELL_PUNGENT_BLIGHT)
        -- spell_festergut_pungent_blight HandleScript: Talk(EMOTE_PUNGENT_BLIGHT) on hit (removal already in removeBlight above).
        creature:Talk(EMOTE_PUNGENT_BLIGHT)
        inhale[guid] = 0
        -- C++ :193-194 — professor DoAction(ACTION_FESTERGUT_GAS):
        -- Talk(SAY_FESTERGUT_GASEOUS_BLIGHT 0) + triggered self-cast
        -- SPELL_RELEASE_GAS_VISUAL 69125, in C++ order after the
        -- pungent cast and before the GAS_SPORE reschedule.
        relayProfessor(creature, function(professor)
            professor:Talk(SAY_FESTERGUT_GASEOUS_BLIGHT)
            professor:CastSpell(professor, SPELL_RELEASE_GAS_VISUAL, true)
        end)
        cancelTimer(guid, "gas_spore")
        schedule(guid, "gas_spore", {20000, 25000}, function() onGasSpore(creature, guid) end)
    else
        creature:CastSpell(creature, SPELL_INHALE_BLIGHT)
        inhale[guid] = count + 1
        -- C++ :199-203: DoCastSelf(SPELL_INHALE_BLIGHT) first, then
        -- ++_inhaleCounter, then (if the new counter < 3) a non-triggered
        -- self-cast of gaseousBlight[_inhaleCounter] — the real inhale
        -- stack aura. _inhaleCounter is 1/2 here (3 takes the pungent
        -- branch); C++ gaseousBlight[3] is 0-indexed ({69157, 69162,
        -- 69164}), Lua GASEOUS_BLIGHT is 1-indexed, so the C++
        -- gaseousBlight[_inhaleCounter] = GASEOUS_BLIGHT[count+2]
        -- (69162/69164). The 3rd CastSpell arg (orig caster = self) is a
        -- no-op here.
        if count + 1 < 3 then
            creature:CastSpell(creature, GASEOUS_BLIGHT[count + 2])
        end
    end
    schedule(guid, "inhale_blight", {33500, 35000}, function() onInhaleBlight(creature, guid) end)
end

-- C++ EVENT_VILE_GAS (:209-233): SelectTargetList(ranged, 25,
-- Random, 0, -5.0f, playerOnly) + SelectTargetList(melee, 25, Random,
-- 0, 5.0f, playerOnly) — DefaultTargetSelector excludes dist>0
-- outside-range and dist<0 inside-range (UnitAI.cpp:271-274), so
-- ranged = alive players OUTSIDE IsWithinCombatRange(5.0f) — raw distance
-- minus boss + player (1.5) reaches >= 5.0 — melee = inside; top ranged up from
-- melee to RAID_MODE minTargets(3,8,3,8)
-- -> 10-man 3 (no 10/25-man bridge — the hodir icicle precedent;
-- 25-man 8 documented in header); RandomResize(ranged,
-- RAID_MODE(1,3,1,3) -> 10-man 1) -> DoCast(target, SPELL_VILE_GAS)
-- non-triggered. Re-arm {28s,35s} unconditional (:232); first fire
-- comes from the gas-spore arm's RescheduleEvent(:240) 28s..35s after
-- the first gas spore. Threat-list pool via creature:GetAITargets()
-- (the putricide slime-puddle precedent).
local function vileGasTick(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :177 (+ in-loop :254, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "vile_gas", 500, function() vileGasTick(creature, guid) end)
        return
    end
    local ranged, melee = {}, {}
    -- C++ pool split is on IsWithinCombatRange(target, 5.0f) (Unit.cpp:583-595):
    -- raw 3D distance < 5.0 + bossReach + playerReach. Lua GetDistance is raw
    -- center distance (engine/scripting/object.go:285-293), so the boss's own
    -- reach is subtracted explicitly (the f07d8c52 GetCombatReach bridge
    -- idiom) plus the player reach 1.5 (pool is playerOnly).
    local reach = creature:GetCombatReach()
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            if creature:GetDistance(unit) - reach - 1.5 >= 5.0 then
                ranged[#ranged + 1] = unit
            else
                melee[#melee + 1] = unit
            end
        end
    end
    while #ranged < 3 and #melee > 0 do
        ranged[#ranged + 1] = table.remove(melee, math.random(#melee))
    end
    if #ranged > 0 then
        creature:CastSpell(table.remove(ranged, math.random(#ranged)), SPELL_VILE_GAS)
    end
    schedule(guid, "vile_gas", {28000, 35000}, function() vileGasTick(creature, guid) end)
end

-- C++ EVENT_GAS_SPORE (:235-240): Talk(EMOTE_WARN_GAS_SPORE) +
-- Talk(EMOTE_GAS_SPORE) + self-cast SPELL_GAS_SPORE non-triggered.
onGasSpore = function(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :177: 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "gas_spore", 500, function() onGasSpore(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_WARN_GAS_SPORE)
    creature:Talk(EMOTE_GAS_SPORE)
    creature:CastSpell(creature, SPELL_GAS_SPORE)
    schedule(guid, "gas_spore", {40000, 45000}, function() onGasSpore(creature, guid) end)
    -- C++ :240 — events.RescheduleEvent(EVENT_VILE_GAS, 28s, 35s):
    -- schedule() cancels any pending key first, the exact twin.
    schedule(guid, "vile_gas", {28000, 35000}, function() vileGasTick(creature, guid) end)
end

-- C++ boss_festergut::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent. No
-- SetBossState leg: C++ JustEngagedWith (:113-129) writes no state
-- (IN_PROGRESS rides Putricide's MovementInform, see header).
local function festergutJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ :128 — me-form DoZoneInCombat last in JustEngagedWith (after
    -- Talk(SAY_AGGRO) / gas-dummy / professor DoAction legs); C++ arm order.
    creature:DoZoneInCombat()
    schedule(guid, "gastric_bloat", {12500, 15000}, function() onGastricBloat(creature, guid) end)
    schedule(guid, "berserk", 300000, function() onBerserk(creature, guid) end)
    inhale[guid] = 0
    schedule(guid, "inhale_blight", {25000, 30000}, function() onInhaleBlight(creature, guid) end)
    schedule(guid, "gas_spore", {20000, 25000}, function() onGasSpore(creature, guid) end)
end

-- C++ Reset/JustDied _Reset()/_JustDied() timer machinery —
-- cancel/re-arm on combat events (the maiden convention).
local function festergutLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhale[guid] = nil
end

local function festergutReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhale[guid] = nil
    -- C++ Reset() :94 — _Reset() first: SetBossState NOT_STARTED
    -- only when the stored state != DONE (BossAI::_Reset,
    -- ScriptedCreature.cpp:499-509; the falric precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-05 via creature:SetBossState.
    if creature:GetBossState(DATA_FESTERGUT) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_FESTERGUT, BOSS_STATE_NOT_STARTED)
    end
    -- C++ Reset() (:92-110): me->RemoveAurasDueToSpell(SPELL_BERSERK2) —
    -- the RemoveAura bridge (the drakkari / falric idiom); then the
    -- RemoveBlight() loop (nearest alive gas dummy within 100yd gates
    -- both halves, exactly like C++ :102).
    creature:RemoveAura(SPELL_BERSERK2)
    removeBlight(creature)
end

-- C++ boss_festergut::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function festergutKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_festergut::JustDied: _JustDied() (:133) FIRST, then
-- Talk(SAY_DEATH) (:134) — the DONE leg PORTED 2026-11-05 via
-- creature:SetBossState, in C++ order BEFORE Talk (unlike marrowgar,
-- whose _JustDied runs after Talk). The timers must be cancelled here:
-- CreateLuaEvent timers have no death auto-cancel, so the
-- self-re-arming inhale/gas-spore/gastric-bloat legs would keep
-- firing on the corpse (the devourer death-cancel precedent).
local function festergutJustDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhale[guid] = nil
    creature:SetBossState(DATA_FESTERGUT, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ :136 — professor DoAction(ACTION_FESTERGUT_DEATH) schedules
    -- EVENT_FESTERGUT_DIES at 4s: Talk(SAY_FESTERGUT_DEATH 1) +
    -- EnterEvadeMode (no bridge — stays out). The Talk leg rides a
    -- 4s timer on the festergut GUID (cancelled on reset like the
    -- other festergut timers); C++ order between SAY_DEATH and
    -- RemoveBlight.
    schedule(guid, "professor_death_talk", 4000, function()
        relayProfessor(creature, function(professor)
            professor:Talk(SAY_FESTERGUT_DEATH)
        end)
    end)
    -- C++ order (:130-136): Talk(SAY_DEATH), then RemoveBlight().
    removeBlight(creature)
end

-- C++ boss_festergut::JustReachedHome (:141-144): _JustReachedHome()
-- (setActive(false), unmodeled) then SetBossState(FAIL) PORTED
-- 2026-11-05 via creature:SetBossState on ev24 OnReachHome
-- (creaturemotion.go home-move finalize, the opera ev24 precedent).
-- The EnterEvadeMode cross-AI professor EnterEvadeMode leg stays out
-- (no bridge — EnterEvadeMode has no Lua bridge, boss_ai.go:5753-5754).
local function festergutReachHome(event, creature)
    creature:SetBossState(DATA_FESTERGUT, BOSS_STATE_FAIL)
end

-- C++ boss_festergut::SpellHitTarget (boss_festergut.cpp:160-168):
-- PUNGENT_BLIGHT_HELPER (RAID_MODE 69195/71219/73031/73032) ->
-- unitTarget:RemoveAurasDueToSpell(INOCULATED_HELPER) (RAID_MODE
-- 69291/72101/72102/72103, same raid-mode index). STAYS OUT:
-- trigger-path blocked — the C++ arm strips INOCULATED from each PLAYER
-- hit by the blight AoE, but Go's creature cast path carries only the
-- explicit target (boss_ai.go:1497, no area fan-out); the 3rd-inhale
-- trigger is a self-cast, so event 15 would deliver target = the boss
-- himself, whom the C++ arm never strips. Retracts the 11:55 ev15 port
-- (13:28 verified map+gates but not the target correspondence): a
-- self-target strip fabricates a leg the C++ arm never performs.
-- RemoveAura stays bridged (lua_creature_events.go:157).

-- C++ spell_festergut_blighted_spores::ExtraEffect
-- (boss_festergut.cpp:403-419): OnEffectRemove EFFECT_0
-- SPELL_AURA_PERIODIC_DAMAGE, AURA_EFFECT_HANDLE_REAL — the target
-- casts SPELL_INOCULATED 69291 on itself, triggered, then
-- festergut->AI()->SetData(DATA_INOCULATED_STACK 69291,
-- GetStackAmount()) with max() semantics (:261-264), then
-- HandleResidue (:421-438): a player without the Orange Blight
-- Residue aura (72144) whose Residue Rendezvous quest (24873 10-man /
-- 24878 25-man) is INCOMPLETE casts 72144 on itself, triggered.
-- PORTED tick 461 via RegisterAuraEvent (3, OnRemove — the mandokir /
-- vaelastrasz precedent). The C++ festergut lookup rides the instance
-- guid store; the Lua twin uses the aura's caster — Gas Spore is cast
-- only by Festergut, so both resolve to the same creature — and the
-- 25-man quest read rides the caster too (Is25ManRaid is a
-- creature-only bridge; the map difficulty is the same value C++
-- reads via GetMap()). The residue leg mirrors the rotface
-- slime-spray residue handler (quest ids, caster 25-man read,
-- player CastSpell form). Dormant until Spell.dbc ships the Gas
-- Spore -> Blighted Spores trigger link: no aura can apply yet, so
-- the latch reads 0 and the achievement gate stays true, matching
-- C++ for a zero-spore fight.
local function blightedSporesRemove(event, aura, target)
    if target == nil then
        return
    end
    target:CastSpell(target, SPELL_INOCULATED, true)
    local festergut = aura:GetCaster()
    if festergut ~= nil then
        local stack = aura:GetStackAmount()
        if stack > festergut:GetData(DATA_INOCULATED_STACK) then
            festergut:SetData(DATA_INOCULATED_STACK, stack)
        end
    end
    if target:GetObjectType() ~= "Player" then
        return
    end
    if target:HasAura(SPELL_ORANGE_BLIGHT_RESIDUE) then
        return
    end
    local questId = QUEST_RESIDUE_RENDEZVOUS_10
    if festergut ~= nil and festergut:Is25ManRaid() then
        questId = QUEST_RESIDUE_RENDEZVOUS_25
    end
    if target:GetQuestStatus(questId) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    target:CastSpell(target, SPELL_ORANGE_BLIGHT_RESIDUE, true)
end

for _, auraId in ipairs(BLIGHTED_SPORES_AURAS) do
    RegisterAuraEvent(auraId, 3, blightedSporesRemove)
end

RegisterCreatureEvent(ENTRY_FESTERGUT, 1, festergutJustEngagedWith)
RegisterCreatureEvent(ENTRY_FESTERGUT, 2, festergutLeaveCombat)
RegisterCreatureEvent(ENTRY_FESTERGUT, 3, festergutKilledUnit)
RegisterCreatureEvent(ENTRY_FESTERGUT, 4, festergutJustDied)
RegisterCreatureEvent(ENTRY_FESTERGUT, 23, festergutReset)
RegisterCreatureEvent(ENTRY_FESTERGUT, 24, festergutReachHome)

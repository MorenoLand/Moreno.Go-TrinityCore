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
-- SPELL_INHALE_BLIGHT 69165) non-triggered; 3rd inhale Talk(
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
-- 5.0yd, melee top-up to 10-man minTargets 3 (25-man 8 has no bridge,
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
-- festergut->AI()->SetData(DATA_INOCULATED_STACK) leg) joins the
-- no-AuraScript-bridge queue. achievement_flu_shot_shortage joins
-- the no-achievement bridge queue (the gunship
-- achievement_im_on_a_boat precedent).
-- boss_festergut JustEngagedWith legs —
-- instance->CheckRequiredBosses / DoCastSpellOnPlayers /
-- FindNearestCreature(NPC_GAS_DUMMY) / DoZoneInCombat /
-- professor->AI()->DoAction(ACTION_FESTERGUT_COMBAT) — have no
-- instance / DoAction bridge; JustDied _JustDied + DoAction(
-- ACTION_FESTERGUT_DEATH) + RemoveBlight legs, JustReachedHome
-- instance->SetBossState(FAIL), EnterEvadeMode cross-AI professor
-- EnterEvadeMode — no instance / DoAction bridge.
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

local SAY_AGGRO = 1
local SAY_KILL = 7
local SAY_DEATH = 9
local SAY_BERSERK = 8

local SPELL_GASTRIC_BLOAT = 72219
local SPELL_BERSERK2 = 47008
local SPELL_INHALE_BLIGHT = 69165
local SPELL_PUNGENT_BLIGHT = 69195
local SPELL_GAS_SPORE = 69278
local SPELL_VILE_GAS = 69240
local EMOTE_WARN_PUNGENT_BLIGHT = 5
local SAY_PUNGENT_BLIGHT = 4
local EMOTE_WARN_GAS_SPORE = 3
local EMOTE_GAS_SPORE = 2

local NPC_GAS_DUMMY = 36659

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
    creature:CastSpell(nil, SPELL_GASTRIC_BLOAT)
    schedule(guid, "gastric_bloat", {15000, 17500}, function() onGastricBloat(creature, guid) end)
end

-- C++ EVENT_BERSERK: single-fire 5min — DoCastSelf
-- (SPELL_BERSERK2) non-triggered then Talk(SAY_BERSERK)
-- (:93/:246-248); cast-before-talk order kept.
local function onBerserk(creature, guid)
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
local function onInhaleBlight(creature, guid)
    -- C++ :185-186: RemoveBlight() fires at the top of EVERY inhale
    -- tick, before the counter check.
    removeBlight(creature)
    local count = inhale[guid] or 0
    if count == 3 then
        creature:Talk(EMOTE_WARN_PUNGENT_BLIGHT)
        creature:Talk(SAY_PUNGENT_BLIGHT)
        creature:CastSpell(creature, SPELL_PUNGENT_BLIGHT)
        inhale[guid] = 0
        cancelTimer(guid, "gas_spore")
        schedule(guid, "gas_spore", {20000, 25000}, function() onGasSpore(creature, guid) end)
    else
        creature:CastSpell(creature, SPELL_INHALE_BLIGHT)
        inhale[guid] = count + 1
    end
    schedule(guid, "inhale_blight", {33500, 35000}, function() onInhaleBlight(creature, guid) end)
end

-- C++ EVENT_VILE_GAS (:209-233): SelectTargetList(ranged, 25,
-- Random, 0, -5.0f, playerOnly) + SelectTargetList(melee, 25, Random,
-- 0, 5.0f, playerOnly) — DefaultTargetSelector excludes dist>0
-- outside-range and dist<0 inside-range (UnitAI.cpp:271-274), so
-- ranged = alive players at distance >= 5.0yd, melee = alive players
-- < 5.0yd; top ranged up from melee to RAID_MODE minTargets(3,8,3,8)
-- -> 10-man 3 (no 10/25-man bridge — the hodir icicle precedent;
-- 25-man 8 documented in header); RandomResize(ranged,
-- RAID_MODE(1,3,1,3) -> 10-man 1) -> DoCast(target, SPELL_VILE_GAS)
-- non-triggered. Re-arm {28s,35s} unconditional (:232); first fire
-- comes from the gas-spore arm's RescheduleEvent(:240) 28s..35s after
-- the first gas spore. Threat-list pool via creature:GetAITargets()
-- (the putricide slime-puddle precedent).
local function vileGasTick(creature, guid)
    local ranged, melee = {}, {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            if creature:GetDistance(unit) >= 5.0 then
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
    creature:Talk(EMOTE_WARN_GAS_SPORE)
    creature:Talk(EMOTE_GAS_SPORE)
    creature:CastSpell(creature, SPELL_GAS_SPORE)
    schedule(guid, "gas_spore", {40000, 45000}, function() onGasSpore(creature, guid) end)
    -- C++ :240 — events.RescheduleEvent(EVENT_VILE_GAS, 28s, 35s):
    -- schedule() cancels any pending key first, the exact twin.
    schedule(guid, "vile_gas", {28000, 35000}, function() vileGasTick(creature, guid) end)
end

-- C++ boss_festergut::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function festergutJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
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

-- C++ boss_festergut::JustDied: _JustDied() resets the EventMap
-- (boss_festergut.cpp:132) + Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent. The timers must be cancelled here:
-- CreateLuaEvent timers have no death auto-cancel, so the
-- self-re-arming inhale/gas-spore/gastric-bloat legs would keep
-- firing on the corpse (the devourer death-cancel precedent).
local function festergutJustDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhale[guid] = nil
    creature:Talk(SAY_DEATH)
    -- C++ order (:130-136): Talk(SAY_DEATH), then RemoveBlight().
    removeBlight(creature)
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

RegisterCreatureEvent(ENTRY_FESTERGUT, 1, festergutJustEngagedWith)
RegisterCreatureEvent(ENTRY_FESTERGUT, 2, festergutLeaveCombat)
RegisterCreatureEvent(ENTRY_FESTERGUT, 3, festergutKilledUnit)
RegisterCreatureEvent(ENTRY_FESTERGUT, 4, festergutJustDied)
RegisterCreatureEvent(ENTRY_FESTERGUT, 23, festergutReset)

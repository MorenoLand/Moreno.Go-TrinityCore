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
-- OUT legs inside the arm: RemoveBlight() aura-removal (no
-- RemoveAura bridge), gaseous-blight visual triggered casts
-- (:202, aura legs without the removal bridge), DoAction(
-- ACTION_FESTERGUT_GAS) cross-AI (:193-194).
-- PORTED (2026-10-14): EVENT_GAS_SPORE — Talk(
-- EMOTE_WARN_GAS_SPORE 3) + Talk(EMOTE_GAS_SPORE 2) + CastSpell(
-- creature, SPELL_GAS_SPORE 69278) non-triggered; init
-- {20000,25000}, re-arm {40000,45000} (:235-240). MAX_TARGETS
-- spell-mod leg (:238) has no Lua bridge — the DB spell is
-- raid-moded with the same 2/3 values, so the default cast is
-- faithful. RescheduleEvent(EVENT_VILE_GAS) (:240) rides the OUT
-- vile-gas event — dropped.
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
-- EnterEvadeMode — no instance / DoAction bridge. SpellHitTarget
-- is Talk-free (PUNGENT_BLIGHT_HELPER aura-removal leg only).
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
local EMOTE_WARN_PUNGENT_BLIGHT = 5
local SAY_PUNGENT_BLIGHT = 4
local EMOTE_WARN_GAS_SPORE = 3
local EMOTE_GAS_SPORE = 2

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

-- C++ EVENT_GAS_SPORE (:235-240): Talk(EMOTE_WARN_GAS_SPORE) +
-- Talk(EMOTE_GAS_SPORE) + self-cast SPELL_GAS_SPORE non-triggered.
onGasSpore = function(creature, guid)
    creature:Talk(EMOTE_WARN_GAS_SPORE)
    creature:Talk(EMOTE_GAS_SPORE)
    creature:CastSpell(creature, SPELL_GAS_SPORE)
    schedule(guid, "gas_spore", {40000, 45000}, function() onGasSpore(creature, guid) end)
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
end

RegisterCreatureEvent(ENTRY_FESTERGUT, 1, festergutJustEngagedWith)
RegisterCreatureEvent(ENTRY_FESTERGUT, 2, festergutLeaveCombat)
RegisterCreatureEvent(ENTRY_FESTERGUT, 3, festergutKilledUnit)
RegisterCreatureEvent(ENTRY_FESTERGUT, 4, festergutJustDied)
RegisterCreatureEvent(ENTRY_FESTERGUT, 23, festergutReset)

-- King Dred (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_king_dred.cpp
-- (boss_king_dred (BossAI), npc_drakkari_gutripper (ScriptedAI),
-- npc_drakkari_scytheclaw (ScriptedAI),
-- achievement_king_dred (AchievementCriteriaScript);
-- AddSC_boss_king_dred at end registers all via GetDrakTharonKeepAI /
-- new). The third Drak'Tharon Keep group in northrend_script_loader.cpp
-- order (decl 41 / call 236, immediately after AddSC_boss_novos();
-- next: AddSC_boss_tharon_ja()).
-- Entry: 27483 King Dred (drak_tharon_keep.h NPC_KING_DRED — kalecgos
-- pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- NOTE: unlike the other Drak'Tharon Keep bosses, King Dred has no
-- SAY/yell enum at all — no Talk arms exist in C++ to port.
-- Sole-source verified: whole-server-tree grep for "boss_king_dred",
-- "npc_drakkari_gutripper", "npc_drakkari_scytheclaw" and
-- "achievement_king_dred" hits boss_king_dred.cpp only (loader carries
-- only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance bookkeeping
-- has no bridge.
-- Ported arms (C++-exact for the modeled arms — the three
-- JustEngagedWith schedules are independent, none of the unmodeled
-- events gate them):
-- boss_king_dred: EVENT_GRIEVOUS_BITE DoCastVictim(Grievous Bite
-- 48920), 20s init, 20s repeat (event 1 scheduler);
-- EVENT_MANGLING_SLASH DoCastVictim(Mangling Slash 48873), 18500ms
-- init, 18500ms repeat; EVENT_PIERCING_SLASH DoCastVictim(Piercing
-- Slash 48878), 15s init, 15s repeat; EVENT_BELLOWING_ROAR non-
-- triggered DoCastAOE(Bellowing Roar 22686) -> self-cast CastSpell(
-- creature, spell), 33s init, 33s repeat; EVENT_FEARSOME_ROAR non-
-- triggered DoCastAOE(Fearsome Roar 48849) -> self-cast, randtime(
-- 10s,20s) init, 10-20s repeat (sulfuron convention). Both roars
-- carry no SpellScript and are not phase-gated, so the stale "no
-- DoCastAOE bridge" claim was wrong (selin_fireheart convention).
-- No Talk, no JustDied legs —
-- C++ JustDied is only _JustDied() (instance bookkeeping, no bridge).
-- EVENT_RAPTOR_CALL — DoCastVictim(SPELL_RAPTOR_CALL 59416, dummy) +
-- me->SummonCreature(RAND(NPC_DRAKKARI_GUTRIPPER 26641 /
-- NPC_DRAKKARI_SCYTHECLAW 26628), GetClosePoint(combatReach/3, 10.0f),
-- TEMPSUMMON_DEAD_DESPAWN 1s), randtime(20s,25s) init, 20-25s repeat —
-- PORTED 2026-10-24 (the stale "absent summon STRAND bridge" claim was
-- wrong: creature:SummonCreature is bridged, lua_creature_events.go:728;
-- the position leg rides creature:GetRelativePoint, lua_objects.go:519).
-- Unmodeled (no bridges — documented, not wired):
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) skip — no unit-state
-- bridge.
-- DoAction(ACTION_RAPTOR_KILLED 1) ++raptorsKilled + GetData(
-- DATA_RAPTORS_KILLED 2) — no DoAction/GetData bridges
-- (drakkari_colossus precedent); its only caller is
-- achievement_king_dred — see below.
-- npc_drakkari_gutripper (26641) / npc_drakkari_scytheclaw (26628) —
-- PORTED 2026-10-24 (they spawn via the now-ported RAPTOR_CALL arm):
-- ctor/Reset arms GutRipTimer/uiRendTimer = urand(10s,15s) ->
-- DoCastVictim(SPELL_GUT_RIP 49710 / SPELL_REND 13738, non-triggered),
-- re-arm urand(10s,15s); melee is engine-driven. JustDied cross-AI
-- DoAction(ACTION_RAPTOR_KILLED) stays out — ObjectAccessor +
-- instance-GetGuidData + DoAction have no bridges (the
-- achievement_king_dred counter with it, documented-only below).
-- achievement_king_dred — AchievementCriteriaScript (target
-- ToCreature -> AI()->GetData(DATA_RAPTORS_KILLED) >= 6) — no
-- achievement-criteria bridge (snakes precedent) + cross-AI GetData
-- bridge absent — documented-only.

local ENTRY_KING_DRED = 27483

local SPELL_GRIEVOUS_BITE = 48920
local SPELL_MANGLING_SLASH = 48873
local SPELL_PIERCING_SLASH = 48878
local SPELL_BELLOWING_ROAR = 22686
local SPELL_FEARSOME_ROAR = 48849
local SPELL_RAPTOR_CALL = 59416
local SPELL_GUT_RIP = 49710
local SPELL_REND = 13738

local NPC_DRAKKARI_GUTRIPPER = 26641
local NPC_DRAKKARI_SCYTHECLAW = 26628

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

-- C++ EVENT_GRIEVOUS_BITE: DoCastVictim; 20s init, 20s repeat.
local function grievousBiteTick(creature, guid)
    creature:CastSpell(nil, SPELL_GRIEVOUS_BITE)
    schedule(guid, "grievousbite", 20000, function()
        grievousBiteTick(creature, guid)
    end)
end

-- C++ EVENT_MANGLING_SLASH: DoCastVictim; 18500ms init, 18500ms
-- repeat.
local function manglingSlashTick(creature, guid)
    creature:CastSpell(nil, SPELL_MANGLING_SLASH)
    schedule(guid, "manglingslash", 18500, function()
        manglingSlashTick(creature, guid)
    end)
end

-- C++ EVENT_PIERCING_SLASH: DoCastVictim; 15s init, 15s repeat.
local function piercingSlashTick(creature, guid)
    creature:CastSpell(nil, SPELL_PIERCING_SLASH)
    schedule(guid, "piercingslash", 15000, function()
        piercingSlashTick(creature, guid)
    end)
end

-- C++ EVENT_BELLOWING_ROAR: non-triggered DoCastAOE(22686), 33s init,
-- 33s repeat. Lowers to non-triggered self-cast (selin_fireheart
-- DoCastAOE convention); 22686 carries no SpellScript and the arm is
-- not phase-gated, so the stale "no DoCastAOE bridge" header claim was
-- wrong.
local function bellowingRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowingroar", 33000, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ EVENT_FEARSOME_ROAR: non-triggered DoCastAOE(48849),
-- randtime(10s,20s) init, randtime(10s,20s) repeat (sulfuron
-- convention for the C++ 10s,20s urand overload).
local function fearsomeRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_FEARSOME_ROAR)
    schedule(guid, "fearsome_roar", math.random(10000, 20000), function()
        fearsomeRoarTick(creature, guid)
    end)
end

-- C++ EVENT_RAPTOR_CALL (boss_king_dred.cpp:144-152): DoCastVictim
-- (SPELL_RAPTOR_CALL 59416, dummy) + me->SummonCreature(
-- RAND(NPC_DRAKKARI_GUTRIPPER 26641 / NPC_DRAKKARI_SCYTHECLAW 26628),
-- GetClosePoint(combatReach/3, 10.0f), orientation 0,
-- TEMPSUMMON_DEAD_DESPAWN 1s); randtime(20s,25s) init and repeat.
-- GetClosePoint(size=combatReach/3, dist=10, relAngle=0) =
-- GetNearPoint(nullptr, 10+combatReach/3, orientation) (Object.cpp:3307)
-- = creature:GetRelativePoint(10 + reach/3, 0) — the bridge hardcodes
-- size=0 (lua_creature_events.go:519), so the size leg rides the dist
-- argument. o=0 falls back to the summoner's orientation on both sides
-- (Object.cpp:2061-2062 twin); DEAD_DESPAWN is type 6 in the engine's
-- 0-based summon-type scheme (lua_creature_events.go:936).
local function raptorCallTick(creature, guid)
    creature:CastSpell(nil, SPELL_RAPTOR_CALL)
    local reach = creature:GetCombatReach()
    local x, y, z = creature:GetRelativePoint(10 + reach / 3, 0)
    local entry = NPC_DRAKKARI_GUTRIPPER
    if math.random(2) == 2 then
        entry = NPC_DRAKKARI_SCYTHECLAW
    end
    creature:SummonCreature(entry, x, y, z, 0, 6, 1000)
    schedule(guid, "raptorcall", {20000, 25000}, function()
        raptorCallTick(creature, guid)
    end)
end

local function kingDredEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "raptorcall", {20000, 25000}, function()
        raptorCallTick(creature, guid)
    end)
    schedule(guid, "grievousbite", 20000, function()
        grievousBiteTick(creature, guid)
    end)
    schedule(guid, "manglingslash", 18500, function()
        manglingSlashTick(creature, guid)
    end)
    schedule(guid, "piercingslash", 15000, function()
        piercingSlashTick(creature, guid)
    end)
    schedule(guid, "bellowingroar", 33000, function()
        bellowingRoarTick(creature, guid)
    end)
    schedule(guid, "fearsome_roar", math.random(10000, 20000), function()
        fearsomeRoarTick(creature, guid)
    end)
end

local function kingDredLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function kingDredDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function kingDredReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_drakkari_gutripperAI / npc_drakkari_scytheclawAI
-- (boss_king_dred.cpp:179-222 / 227-270): ctor/Reset arm
-- GutRipTimer/uiRendTimer = urand(10s,15s); UpdateAI fires
-- DoCastVictim(SPELL_GUT_RIP 49710 / SPELL_REND 13738, non-triggered)
-- with urand(10s,15s) re-arm; melee is engine-driven. JustSummoned
-- (event 19) is the spawn twin (the shirrak precedent); the cross-AI
-- JustDied DoAction(ACTION_RAPTOR_KILLED) leg stays out (no
-- ObjectAccessor / instance-GetGuidData / DoAction bridges — the
-- achievement_king_dred counter with it, documented-only above).
local function raptorRotationTick(creature, guid, key, spell)
    creature:CastSpell(nil, spell)
    schedule(guid, key, math.random(10000, 15000), function()
        raptorRotationTick(creature, guid, key, spell)
    end)
end

local function gutripperSummoned(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "gutrip", math.random(10000, 15000), function()
        raptorRotationTick(creature, guid, "gutrip", SPELL_GUT_RIP)
    end)
end

local function scytheclawSummoned(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rend", math.random(10000, 15000), function()
        raptorRotationTick(creature, guid, "rend", SPELL_REND)
    end)
end

local function raptorGone(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KING_DRED, 1, kingDredEnterCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 2, kingDredLeaveCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 4, kingDredDied)
RegisterCreatureEvent(ENTRY_KING_DRED, 23, kingDredReset)

RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 19, gutripperSummoned)
RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 2, raptorGone)
RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 4, raptorGone)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 19, scytheclawSummoned)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 2, raptorGone)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 4, raptorGone)

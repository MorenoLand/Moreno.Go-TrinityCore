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
-- (gargolmar precedent); BossAI _Reset/_JustDied bookkeeping rides the
-- bridged creature:SetBossState (IN_PROGRESS/NOT_STARTED/DONE, tick 510)
-- + the live-raptor despawn sweep (summons.DespawnAll twin).
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
-- No Talk legs; JustDied is _JustDied() — the SetBossState(DONE) +
-- raptor-despawn legs are ported (tick 510).
-- EVENT_RAPTOR_CALL — DoCastVictim(SPELL_RAPTOR_CALL 59416, dummy) +
-- me->SummonCreature(RAND(NPC_DRAKKARI_GUTRIPPER 26641 /
-- NPC_DRAKKARI_SCYTHECLAW 26628), GetClosePoint(combatReach/3, 10.0f),
-- TEMPSUMMON_DEAD_DESPAWN 1s), randtime(20s,25s) init, 20-25s repeat —
-- PORTED 2026-10-24 (the stale "absent summon STRAND bridge" claim was
-- wrong: creature:SummonCreature is bridged, lua_creature_events.go:728;
-- the position leg rides creature:GetRelativePoint, lua_creature_events.go:3529 (motion twin; session twin lua_objects.go:3008).
-- Unmodeled (no bridges — documented, not wired):
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) pre-loop + in-loop gates
-- PORTED: all 6 boss timer arms defer 500ms while casting (no repeat
-- re-arm); raptor sub-AI rotation arms ungated per C++ (plain
-- countdowns, zero CASTING hits).
-- DoAction(ACTION_RAPTOR_KILLED 1) ++raptorsKilled + GetData(
-- DATA_RAPTORS_KILLED 2) — PORTED 2026-11-05 in kingDredDoAction (ev38)
-- + the raptor JustDied relays below (boss_king_dred.cpp:220, :276);
-- consumer achievement_king_dred ported (see below).
-- npc_drakkari_gutripper (26641) / npc_drakkari_scytheclaw (26628) —
-- PORTED 2026-10-24 (they spawn via the now-ported RAPTOR_CALL arm):
-- ctor/Reset arms GutRipTimer/uiRendTimer = urand(10s,15s) ->
-- DoCastVictim(SPELL_GUT_RIP 49710 / SPELL_REND 13738, non-triggered),
-- re-arm urand(10s,15s); melee is engine-driven. JustDied cross-AI
-- DoAction(ACTION_RAPTOR_KILLED) — PORTED 2026-11-05 in raptorDied
-- (dispatch ev38 + lookup GetInstanceCreature bridged; boss GUID latched
-- by the ev5 SetGuidData in this file; senders gutripper 26641 /
-- scytheclaw 26628 registered in boss_ai.go); sole consumer the now-ported
-- achievement_king_dred (counter ported above).
-- achievement_king_dred — AchievementCriteriaScript (target
-- ToCreature -> AI()->GetData(DATA_RAPTORS_KILLED) >= 6) — PORTED
-- 2026-11-05: checkAchievementCriteriaScript reads the victim motion's
-- SetData latch (engine/world/achievement_criteria_scripts.go), threaded
-- from the kill-credit path (kill.go creditQuestKills).

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

-- DATA_KING_DRED = 2 (drak_tharon_keep.h:33); DATA_RAPTORS_KILLED = 2 /
-- ACTION_RAPTOR_KILLED = 1 (boss_king_dred.cpp:42-44).
local DATA_KING_DRED = 2
local DATA_RAPTORS_KILLED = 2
local ACTION_RAPTOR_KILLED = 1

-- EncounterState (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1,
-- DONE 3 (the boss_anomalus.lua precedent).
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

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
-- Own UpdateAI pre-loop (:117) + in-loop (:157) CASTING gates:
-- 500ms defer, no repeat re-arm (mal_ganis idiom).
local function grievousBiteTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "grievousbite", 500, function()
            grievousBiteTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_GRIEVOUS_BITE)
    schedule(guid, "grievousbite", 20000, function()
        grievousBiteTick(creature, guid)
    end)
end

-- C++ EVENT_MANGLING_SLASH: DoCastVictim; 18500ms init, 18500ms
-- repeat. Same UpdateAI CASTING gates: 500ms defer.
local function manglingSlashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "manglingslash", 500, function()
            manglingSlashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MANGLING_SLASH)
    schedule(guid, "manglingslash", 18500, function()
        manglingSlashTick(creature, guid)
    end)
end

-- C++ EVENT_PIERCING_SLASH: DoCastVictim; 15s init, 15s repeat.
-- Same UpdateAI CASTING gates: 500ms defer.
local function piercingSlashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "piercingslash", 500, function()
            piercingSlashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_PIERCING_SLASH)
    schedule(guid, "piercingslash", 15000, function()
        piercingSlashTick(creature, guid)
    end)
end

-- C++ EVENT_BELLOWING_ROAR: non-triggered DoCastAOE(22686), 33s init,
-- 33s repeat. Lowers to non-triggered self-cast (selin_fireheart
-- DoCastAOE convention); 22686 carries no SpellScript and the arm is
-- not phase-gated, so the stale "no DoCastAOE bridge" header claim was
-- wrong. Same UpdateAI CASTING gates: 500ms defer.
local function bellowingRoarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bellowingroar", 500, function()
            bellowingRoarTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowingroar", 33000, function()
        bellowingRoarTick(creature, guid)
    end)
end

-- C++ EVENT_FEARSOME_ROAR: non-triggered DoCastAOE(48849),
-- randtime(10s,20s) init, randtime(10s,20s) repeat (sulfuron
-- convention for the C++ 10s,20s urand overload). Same UpdateAI
-- CASTING gates: 500ms defer.
local function fearsomeRoarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fearsome_roar", 500, function()
            fearsomeRoarTick(creature, guid)
        end)
        return
    end
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
-- size=0 (lua_creature_events.go:3529), so the size leg rides the dist
-- argument. o=0 falls back to the summoner's orientation on both sides
-- (Object.cpp:2061-2062 twin); DEAD_DESPAWN is type 6 in the engine's
-- 0-based summon-type scheme (lua_creature_events.go:936). Same boss
-- UpdateAI CASTING gates: 500ms defer (the summon leg rides the cast).
-- Live raptor summons (boss_king_dred.cpp:144-152 RAPTOR_CALL): C++
-- BossAI::_Reset/_JustDied call summons.DespawnAll(); the Lua twin
-- stores the SummonCreature returns and despawns them on leave/died/
-- reset. Despawn never fires ev4 (lua_creature_events.go Despawn fires
-- only event 20 on the summoner), so the sweep cannot inflate the
-- raptor-kill counter through the raptorDied relay.
local liveRaptors = {}

local function despawnRaptors()
    for _, raptor in ipairs(liveRaptors) do
        raptor:Despawn()
    end
    liveRaptors = {}
end

local function raptorCallTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "raptorcall", 500, function()
            raptorCallTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_RAPTOR_CALL)
    local reach = creature:GetCombatReach()
    local x, y, z = creature:GetRelativePoint(10 + reach / 3, 0)
    local entry = NPC_DRAKKARI_GUTRIPPER
    if math.random(2) == 2 then
        entry = NPC_DRAKKARI_SCYTHECLAW
    end
    local raptor = creature:SummonCreature(entry, x, y, z, 0, 6, 1000)
    if raptor ~= nil then
        liveRaptors[#liveRaptors + 1] = raptor
    end
    schedule(guid, "raptorcall", {20000, 25000}, function()
        raptorCallTick(creature, guid)
    end)
end

local function kingDredEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-539):
    -- instance->SetBossState(DATA_KING_DRED, IN_PROGRESS).
    creature:SetBossState(DATA_KING_DRED, BOSS_STATE_IN_PROGRESS)
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
    cancelTimers(creature:GetGUIDLow())
    despawnRaptors()
    -- C++ EnterEvadeMode ends with Reset() (CreatureAI.cpp:237);
    -- Reset (:73-76): raptorsKilled = 0; _Reset (ScriptedCreature.cpp:
    -- 499-511): summons.DespawnAll() + SetBossState NOT_STARTED (the
    -- != DONE gate is vacuous: a DONE boss never evades).
    creature:SetData(DATA_RAPTORS_KILLED, 0)
    creature:SetBossState(DATA_KING_DRED, BOSS_STATE_NOT_STARTED)
end

local function kingDredDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    despawnRaptors()
    -- C++ JustDied (:105-108) is _JustDied() (ScriptedCreature.cpp:
    -- 513-520): summons.DespawnAll() + SetBossState DONE.
    creature:SetBossState(DATA_KING_DRED, BOSS_STATE_DONE)
end

local function kingDredReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    despawnRaptors()
    -- C++ Reset (:73-76): raptorsKilled = 0. Alive-gated: ev23 fires on
    -- death too (fireCreatureDied runs Reset before the kill-credit
    -- criteria read, kill.go:1091/:1400), and a death-time zero would
    -- clobber the fight's counter before the achievement gate reads
    -- it; the wipe re-arm is preserved. The _Reset SetBossState
    -- (NOT_STARTED) leg rides the same guard (ScriptedCreature.cpp:
    -- 499-502); the != DONE gate is vacuous (a DONE boss never resets).
    if creature:IsAlive() then
        creature:SetData(DATA_RAPTORS_KILLED, 0)
        creature:SetBossState(DATA_KING_DRED, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ npc_drakkari_gutripperAI / npc_drakkari_scytheclawAI
-- (boss_king_dred.cpp:179-222 / 227-270): ctor/Reset arm
-- GutRipTimer/uiRendTimer = urand(10s,15s); UpdateAI fires
-- DoCastVictim(SPELL_GUT_RIP 49710 / SPELL_REND 13738, non-triggered)
-- with urand(10s,15s) re-arm; melee is engine-driven. The spawn hook is
-- event 22 (CREATURE_EVENT_ON_SUMMONED — the summoned creature's own
-- IsSummonedBy hook, lua_creature_events.go fireCreaturePetSummoned;
-- event 19 fires on the SUMMONER, so a registration on the raptor
-- entries could never fire — tick-510 audit correction of the
-- shirrak-precedent claim; the rotation was dead until this fix). The
-- cross-AI JustDied DoAction(ACTION_RAPTOR_KILLED) leg is portable —
-- dispatch IS bridged (ev38), lookup IS bridged (GetInstanceCreature
-- :5865, unlatched); sole consumer is the PORTED achievement_king_dred
-- (counter ported above).
-- GUID keys are GetGUIDLow() throughout (tick-510 GUID-key fix — the
-- GetGUID() userdata keys missed across handlers, so every cancel from
-- another handler was a no-op and chains leaked past evade/death).
local raptorRotation = {}

local function raptorRotationTick(creature, guid, key, spell)
    creature:CastSpell(nil, spell)
    schedule(guid, key, math.random(10000, 15000), function()
        raptorRotationTick(creature, guid, key, spell)
    end)
end

local function startRaptorRotation(creature, key, spell)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    raptorRotation[guid] = true
    schedule(guid, key, math.random(10000, 15000), function()
        raptorRotationTick(creature, guid, key, spell)
    end)
end

local function gutripperSummoned(event, creature)
    startRaptorRotation(creature, "gutrip", SPELL_GUT_RIP)
end

local function scytheclawSummoned(event, creature)
    startRaptorRotation(creature, "rend", SPELL_REND)
end

-- C++ Reset (:196-199 / :252-255) re-inits the rotation timer on evade.
-- Fires only when no rotation is live: a live rotation keeps its ctor
-- countdown (a plain re-schedule here would restart it whenever the
-- raptor re-engages mid-fight).
local function gutripperEnterCombat(event, creature)
    if not raptorRotation[creature:GetGUIDLow()] then
        startRaptorRotation(creature, "gutrip", SPELL_GUT_RIP)
    end
end

local function scytheclawEnterCombat(event, creature)
    if not raptorRotation[creature:GetGUIDLow()] then
        startRaptorRotation(creature, "rend", SPELL_REND)
    end
end

local function raptorGone(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    raptorRotation[guid] = nil
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the boss GUID goes in the instance store under DATA_KING_DRED so the
-- raptor relays can resolve it via GetInstanceCreature.
local function kingDredSpawned(event, creature)
    creature:SetGuidData(DATA_KING_DRED)
end

-- C++ DoAction (:91-95): ACTION_RAPTOR_KILLED increments the counter.
local function kingDredDoAction(event, creature, action)
    if action == ACTION_RAPTOR_KILLED then
        creature:SetData(DATA_RAPTORS_KILLED, creature:GetData(DATA_RAPTORS_KILLED) + 1)
    end
end

-- C++ npc_drakkari_gutripperAI / npc_drakkari_scytheclawAI JustDied
-- (:217-221 / :273-277): instance GetGuidData (DATA_KING_DRED) ->
-- DoAction(ACTION_RAPTOR_KILLED); the GetInstanceCreature twin resolves
-- the same stored GUID. Ev2 keeps the plain timer cancel (raptorGone);
-- the relay is JustDied-only.
local function raptorDied(event, creature)
    local dred = creature:GetInstanceCreature(DATA_KING_DRED)
    if dred then
        dred:DoAction(ACTION_RAPTOR_KILLED)
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    raptorRotation[guid] = nil
end

RegisterCreatureEvent(ENTRY_KING_DRED, 1, kingDredEnterCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 2, kingDredLeaveCombat)
RegisterCreatureEvent(ENTRY_KING_DRED, 4, kingDredDied)
RegisterCreatureEvent(ENTRY_KING_DRED, 5, kingDredSpawned)
RegisterCreatureEvent(ENTRY_KING_DRED, 23, kingDredReset)
RegisterCreatureEvent(ENTRY_KING_DRED, 38, kingDredDoAction)

RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 22, gutripperSummoned)
RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 1, gutripperEnterCombat)
RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 2, raptorGone)
RegisterCreatureEvent(NPC_DRAKKARI_GUTRIPPER, 4, raptorDied)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 22, scytheclawSummoned)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 1, scytheclawEnterCombat)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 2, raptorGone)
RegisterCreatureEvent(NPC_DRAKKARI_SCYTHECLAW, 4, raptorDied)

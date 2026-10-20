-- Frostsworn General (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (2916 lines; 25 scripts: 16 CreatureScripts
-- (npc_jaina_or_sylvanas_intro_hor, npc_jaina_or_sylvanas_escape_hor,
-- npc_the_lich_king_escape_hor, npc_ghostly_priest, npc_phantom_mage,
-- npc_phantom_hallucination, npc_shadowy_mercenary, npc_spectral_footman,
-- npc_tortured_rifleman, npc_frostsworn_general, npc_spiritual_reflection,
-- npc_raging_ghoul, npc_risen_witch_doctor, npc_lumbering_abomination,
-- npc_uther_quel_delar, npc_quel_delar_sword), 5 AreaTriggerScripts
-- (at_hor_intro_start, at_hor_waves_restarter, at_hor_impenetrable_door,
-- at_hor_shadow_throne, at_hor_uther_quel_delar_start), 3
-- SpellScriptLoaders (spell_hor_start_halls_of_reflection_quest_ae,
-- spell_hor_evasion, spell_hor_gunship_cannon_fire) + spell_hor_quel_delars_will
-- (SpellScript); all registered from inside AddSC_halls_of_reflection();
-- loader decl 167 / call 362 per northrend_script_loader.cpp — the SECOND
-- group of the "// Halls of Reflection" block in AddNorthrendScripts(),
-- immediately after AddSC_instance_halls_of_reflection() (call 361); the
-- calls after it are AddSC_boss_falric() (call 363) / AddSC_boss_marwyn()
-- (call 364) — verified from the loader this run; the checkpoint sequence
-- (instance_halls_of_reflection -> halls_of_reflection) is followed).
-- Entries: 36723 Frostsworn General (halls_of_reflection.h
-- NPC_FROSTSWORN_GENERAL, line 79;
-- instance_halls_of_reflection.cpp OnCreatureCreate binds case
-- NPC_FROSTSWORN_GENERAL, line 141) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. All other zone-script creatures
-- carry zero bridgeable arms and are NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent): the eight gauntlet-trash scripts
-- (ghostly_priest, phantom_mage, phantom_hallucination, shadowy_mercenary,
-- spectral_footman, tortured_rifleman) and the four escape-trash scripts
-- (raging_ghoul, risen_witch_doctor, lumbering_abomination,
-- spiritual_reflection) expose only scheduler-schedule legs in
-- JustEngagedWith / Reset / IsSummonedBy / JustDied (DoCastAOE at most)
-- with no Talk of their own; npc_jaina_or_sylvanas_intro_hor's Talk legs
-- all ride the instance SetData intro-event machine (no instance bridge);
-- npc_jaina_or_sylvanas_escape_hor has no Talk of its own (JustDied only
-- evades the Lich King); npc_the_lich_king_escape_hor's KilledUnit is a
-- DoPlaySoundToSet leg with no Talk; npc_uther_quel_delar and
-- npc_quel_delar_sword's Talk legs ride timer/event machines
-- (EVENT_UTHER_1..9, EVENT_QUEL_DELAR_INIT) with no bridges.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No frostsworn lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- npc_frostsworn_general JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent).
-- npc_frostsworn_general JustDied — Talk(SAY_DEATH 1) (event 4 — the
-- sjonnir JustDied-Talk precedent).
-- npc_frostsworn_general JustEngagedWith — EVENT_SHIELD 69222 at
--   5000ms and EVENT_SPIKE 69184 at 14000ms (C++ :1905-1913).
-- EVENT_SHIELD (:1929-1933) — random player within 45 yd (:1930),
--   cast 69222 (:1931), re-arm {8000,12000}ms (:1932).
-- EVENT_SPIKE (:1934-1938) — random player within 45 yd (:1935),
--   cast 69184 (:1936), re-arm {15000,20000}ms (:1937).
-- DOCUMENTED-ONLY (in this header; only entry 36723 registered):
-- npc_frostsworn_general Reset / JustEngagedWith / JustDied instance
-- legs — _instance->SetData(DATA_FROSTSWORN_GENERAL, NOT_STARTED /
-- IN_PROGRESS / DONE) — no instance bridge.
-- npc_frostsworn_general UpdateAI — EVENT_CLONE (:1939-1942)
-- SummonClones machine (:1950-1964: SelectTargetList + SummonCreature
-- + clone/ghost-visual casts + AttackStart) rides the no-summon /
-- no-instance-GetGuidData bridges, so it is documented-only;
-- DoZoneInCombat has no bridge.
-- The jaina/sylvanas intro Talk machine (SAY_JAINA_INTRO_1..11,
-- SAY_SYLVANAS_INTRO_1..8, SAY_UTHER_INTRO_A2/H2_*, SAY_LK_INTRO_1..3) and
-- the escape Talk machine all ride instance SetData / DoAction legs —
-- no instance-script bridge (the instance_violet_hold precedent); the
-- instance_processEvent captain Talk legs (SAY_CAPTAIN_FIRE /
-- SAY_CAPTAIN_FINAL) were already documented in the
-- instance_halls_of_reflection audit.
-- npc_the_lich_king_escape_hor KilledUnit — DoPlaySoundToSet
-- (SOUND_LK_SLAY_1/2) is ported via creature:PlayDirectSound in
-- npc_the_lich_king_escape.lua (22:19 tick; the old no-sound-
-- bridge note is stale); the ungated DamageTaken clamp is now
-- ported in npc_the_lich_king_escape.lua; the SetData(DATA_
-- ICEWALL) summon/Talk machine and MovementInform legs have no
-- bridges.
-- npc_uther_quel_delar UpdateAI EVENT_UTHER_1..9 Talk machine
-- (SAY_UTHER_QUEL_DELAR_1..6) rides Reset/DoAction/MovementInform
-- sequence legs with no bridges; the DamageTaken clamp is now ported
-- in npc_uther_quel_delar.lua, and the ACTION_UTHER_START_SCREAM /
-- ACTION_UTHER_OUTRO DoAction legs have no bridges.
-- npc_quel_delar_sword — the flight machine
-- (EVENT_QUEL_DELAR_INIT/FLIGHT_INIT/FLIGHT/LAND/FIGHT), the
-- SetImmuneToAll pre-fight leg, and the JustDied uther-DoAction leg
-- have no bridges; the three JustEngagedWith combat-cast arms are now
-- ported in npc_quel_delar_sword.lua.
-- The gauntlet-trash scripts (ghostly_priest, phantom_mage,
-- shadowy_mercenary, spectral_footman, tortured_rifleman) are now
-- ported in npc_hor_gauntlet_trash.lua (phantom_hallucination stays
-- out: entry data-driven, unregistered) and spiritual_reflection in
-- npc_hor_spiritual_reflection.lua; the escape-trash scripts
-- (raging_ghoul, risen_witch_doctor, lumbering_abomination) stay out —
-- all their cast timers are Reset-scheduled and Go fires events 23/5
-- only on the respawn path, so no spawn hook exists.
-- The five AreaTriggerScripts (at_hor_intro_start,
-- at_hor_waves_restarter, at_hor_impenetrable_door, at_hor_shadow_throne,
-- at_hor_uther_quel_delar_start) join the unbridged-area-trigger queue
-- (the pit_of_saron cavern triggers precedent).
-- The four spell scripts (spell_hor_start_halls_of_reflection_quest_ae,
-- spell_hor_evasion, spell_hor_gunship_cannon_fire,
-- spell_hor_quel_delars_will) join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent).

local ENTRY_FROSTSWORN_GENERAL = 36723

local SAY_AGGRO = 0
local SAY_DEATH = 1

local SPELL_SHIELD_THROWN = 69222
local SPELL_SPIKE         = 69184

local SOUND_LK_SLAY_1 = 17214
local SOUND_LK_SLAY_2 = 17215

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 45.0f, true): a random
-- alive player on the same map/instance within 45 yards (the devourer
-- GetPlayersInWorld precedent, plus the C++ distance filter).
local function randomPlayer45(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead()
                and creature:GetDistance(p) <= 45 then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SHIELD :1929-1933 — random player 45 yd, cast 69222,
-- re-arm {8000,12000}ms.
local function generalShieldTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    local target = randomPlayer45(creature)
    if target then
        creature:CastSpell(target, SPELL_SHIELD_THROWN)
    end
    schedule(guid, "shield", {8000, 12000}, function() generalShieldTick(creature, guid) end)
end

-- C++ EVENT_SPIKE :1934-1938 — random player 45 yd, cast 69184,
-- re-arm {15000,20000}ms.
local function generalSpikeTick(creature, guid)
    if timers[guid] == nil then
        return
    end
    local target = randomPlayer45(creature)
    if target then
        creature:CastSpell(target, SPELL_SPIKE)
    end
    schedule(guid, "spike", {15000, 20000}, function() generalSpikeTick(creature, guid) end)
end

-- C++ npc_frostsworn_generalAI::JustEngagedWith :1905-1913:
-- Talk(SAY_AGGRO) (the auriaya engage-port precedent), DoZoneInCombat
-- (no bridge), EVENT_SHIELD at 5000ms, EVENT_SPIKE at 14000ms
-- (EVENT_CLONE's SummonClones leg is documented-only).
local function frostswornGeneralJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shield", 5000, function() generalShieldTick(creature, guid) end)
    schedule(guid, "spike", 14000, function() generalSpikeTick(creature, guid) end)
end

-- C++ npc_frostsworn_generalAI::JustDied :1903-1908: Talk(SAY_DEATH)
-- (the sjonnir JustDied-Talk precedent); the _events.Reset() half is
-- covered by the timer cancel.
local function frostswornGeneralJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function frostswornGeneralTargetDied(event, creature, victim)
    -- C++ npc_frostsworn_generalAI::KilledUnit :1250-1254: player-gated
    -- DoPlaySoundToSet(me, RAND(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2)) — the
    -- npc_the_lich_king_escape precedent.
    if victim:GetObjectType() == "Player" then
        creature:PlayDirectSound(math.random(SOUND_LK_SLAY_1, SOUND_LK_SLAY_2))
    end
end

local function frostswornGeneralReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 1, frostswornGeneralJustEngagedWith)
RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 2, frostswornGeneralReset)
RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 3, frostswornGeneralTargetDied)
RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 4, frostswornGeneralJustDied)
RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 23, frostswornGeneralReset)

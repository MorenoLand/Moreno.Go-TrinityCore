-- Counselor Talbot (Borean Tundra, Last Rites quest chain) --
-- Lua port of npc_counselor_talbotAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 832-945;
-- AddSC at line 1888, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_COUNSELOR_TALBOT = 25301 (:413). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Talbot is summoned
-- at fixed coords (3747.23, 3614.936, 473.321) by npc_thassarian
-- (:513) and despawns with the quest, so he never fights outside his
-- staging area.
-- Zone-script unit per northrend_script_loader.cpp order (borean_tundra:
-- 20 registrations; cultist decl 199 / call 394).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize() (:844-851) timer seeds — SHADOWBOLT
--   urand(5000,12000), DEFLECTION urand(20000,25000), SOULBLAST
--   urand(12000,18000). C++ decrements these only inside the
--   UpdateVictim-gated UpdateAI (:878-907), so seeding on combat start
--   is the faithful Reset-seed lowering (svalna precedent).
-- - EVENT_SHADOWBOLT: non-triggered DoCastVictim(SPELL_SHADOW_BOLT
--   15537); re-arm urand(5000,12000).
-- - EVENT_DEFLECTION: non-triggered DoCastVictim(SPELL_DEFLECTION
--   51009); re-arm urand(20000,25000).
-- - EVENT_SOULBLAST: non-triggered DoCastVictim(SPELL_SOUL_BLAST
--   50992); re-arm urand(12000,18000).
-- - OnLeaveCombat / OnDied / OnReset: cancel GUID-keyed timers
--   (C++ events.Reset() residue).
-- - The UpdateAI `if (me->GetAreaId() == 4125)` gate (:889):
--   creature:GetAreaId() (engine/world/lua_creature_events.go,
--   Map::GetZoneAndAreaId twin) — the three combat-start seeds run
--   only inside area 4125. Talbot is stationary at his summon
--   coords, so combat-start evaluation is C++-exact; behaviorally
--   nil in practice (fixed-coords quest summon).
-- - UpdateAI bCheck leg (:876-886) as a 1000ms poll armed on Reset
--   (spawn; ev23 pre-fires at spawn, lua_creature_events.go:3690):
--   on the first tick where Leryssa armed bCheck (TalbotBCheck
--   global, set by her first arrival :977-979), latch the
--   leryssa/arlos GUIDs via FindNearestCreature (each independently,
--   C++-exact) and consume the flag. The poll is NOT in the combat
--   timers table — the C++ leg runs in UpdateAI regardless of
--   combat, so EnterCombat must not cancel it.
-- - JustDied (:920-942): guard on the latched GUIDs, resolve via
--   GetCreatureByGUID (Barnes idiom), then the tail in C++ order —
--   Arlos Talk SAY_ARLOS_1/2, Leryssa Talk SAY_LERYSSA_1, Arlos
--   KillSelf (creature:Kill, lua_creature_events.go:3753),
--   Leryssa RemoveAura(46957) + ClearUnitState(STUNNED) +
--   SetWalk(false) + MoveTo(0, 3722.11, 3564.20, 477.44) — the
--   MovePoint twin (lua_creature_events.go:5791), which fires
--   Leryssa's second arrival and her phase machine.
-- Unmodeled (documented-only, no bridges):
-- - JustDied RewardPlayerAndGroupAtEvent (:939-940): no Lua
--   bridge (zero hits in the engine for the method name; the Go
--   side only has the kill-rewarder path, kill.go:3074) — the
--   quest credit leg stays out; the rest of the tail is ported.
-- - MovementInform(POINT_MOTION_TYPE) -> ENSURE_AI(npc_thassarian)
--   talbotInPosition cross-AI latch (:860-868): the write target
--   is Thassarian's AI field on the unported escort/phase machine
--   — stays with that unit (same standing as the Leryssa
--   leryssaInPosition write before the ThassarianScene table
--   existed).
-- - npc_thassarian escort-scene react arms (zone_borean_tundra.cpp
--   :509/:566/:604/:667/:675: arthas->SetReactState(PASSIVE) on
--   WaypointReached(3), talbot->SetReactState(PASSIVE) at scene phase
--   2, leryssa->SetReactState(PASSIVE) at phase 6, talbot->
--   SetReactState(AGGRESSIVE) at phase 14, Thassarian himself
--   SetReactState(AGGRESSIVE)+AttackStart(talbot) at phase 15) —
--   cross-creature arms written by the unported npc_thassarian
--   escort/phase machine (no WaypointReached / escort / phase
--   bridges); Talbot's own AI never writes its react state.
-- Verifiable numbers (file's own enums): SPELL_SHADOW_BOLT = 15537
-- (:410), SPELL_DEFLECTION = 51009 (:827), SPELL_SOUL_BLAST = 50992
-- (:828), NPC_COUNSELOR_TALBOT = 25301 (:413).

local ENTRY_COUNSELOR_TALBOT = 25301

local SPELL_SHADOW_BOLT = 15537
local SPELL_DEFLECTION = 51009
local SPELL_SOUL_BLAST = 50992
local SPELL_STUN = 46957 -- zone_borean_tundra.cpp:409

local NPC_LERYSSA = 25251 -- :416
local NPC_GENERAL_ARLOS = 25250 -- :415
local UNIT_STATE_STUNNED = 8 -- Unit.h:214

local SAY_ARLOS_1 = 0
local SAY_ARLOS_2 = 1
local SAY_LERYSSA_1 = 0

-- Talbot JustDied MovePoint dest (:937).
local LERYSSA_MOVE_X = 3722.114502
local LERYSSA_MOVE_Y = 3564.201660
local LERYSSA_MOVE_Z = 477.441437

local timers = {}

-- C++ bCheck flag, set by Leryssa's first arrival (:977-979) and
-- consumed by the UpdateAI latch leg (:876-886). Talbot-side global
-- (one shared lua.State, runtime.go; netherspite cross-file
-- precedent), keyed by Talbot GUIDLow; the latch + poll id are
-- per-Talbot-GUID locals.
TalbotBCheck = TalbotBCheck or {}
local talbotLatch = {}
local talbotBcheckPoll = {}

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

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ ShadowBolt: DoCastVictim(SPELL_SHADOW_BOLT 15537);
-- urand(5000,12000) re-arm.
local function onShadowBolt(creature, guid)
    doCastVictim(creature, SPELL_SHADOW_BOLT)
    schedule(guid, "shadow_bolt", math.random(5000, 12000), function() onShadowBolt(creature, guid) end)
end

-- C++ Deflection: DoCastVictim(SPELL_DEFLECTION 51009);
-- urand(20000,25000) re-arm.
local function onDeflection(creature, guid)
    doCastVictim(creature, SPELL_DEFLECTION)
    schedule(guid, "deflection", math.random(20000, 25000), function() onDeflection(creature, guid) end)
end

-- C++ SoulBlast: DoCastVictim(SPELL_SOUL_BLAST 50992);
-- urand(12000,18000) re-arm.
local function onSoulBlast(creature, guid)
    doCastVictim(creature, SPELL_SOUL_BLAST)
    schedule(guid, "soul_blast", math.random(12000, 18000), function() onSoulBlast(creature, guid) end)
end

local function cancelBcheckPoll(guid)
    local id = talbotBcheckPoll[guid]
    if id then
        RemoveEventById(id)
        talbotBcheckPoll[guid] = nil
    end
end

-- C++ UpdateAI bCheck leg (:876-886) as a 1000ms poll: on the
-- first tick where the flag is armed, latch the GUIDs (each
-- independently, C++-exact) and consume the flag; unarmed ticks
-- re-arm. FindNearestCreature is the alive-only twin
-- (lua_creature_events.go:7219).
local function bcheckTick(creature, guid)
    talbotBcheckPoll[guid] = nil
    if not TalbotBCheck[guid] then
        talbotBcheckPoll[guid] = CreateLuaEvent(function() bcheckTick(creature, guid) end, 1000)
        return
    end
    TalbotBCheck[guid] = false
    local latch = {}
    local leryssa = creature:FindNearestCreature(NPC_LERYSSA, 50)
    if leryssa then
        latch.leryssaGUID = leryssa:GetGUIDLow()
    end
    local arlos = creature:FindNearestCreature(NPC_GENERAL_ARLOS, 50)
    if arlos then
        latch.arlosGUID = arlos:GetGUIDLow()
    end
    talbotLatch[guid] = latch
end

-- C++ Initialize()/Reset(): SHADOWBOLT urand(5s,12s), DEFLECTION
-- urand(20s,25s), SOULBLAST urand(12s,18s). Timers only tick
-- UpdateVictim-gated in C++, so seeding at combat start is faithful.
-- C++ wraps the three pumps in `if (me->GetAreaId() == 4125)`
-- (:889); Talbot is stationary at his summon coords, so evaluating
-- the gate once at combat start is C++-exact (formally; behaviorally
-- nil — he cannot fight outside the staging area).
local function onEnterCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:GetAreaId() == 4125 then
        schedule(guid, "shadow_bolt", math.random(5000, 12000), function() onShadowBolt(creature, guid) end)
        schedule(guid, "deflection", math.random(20000, 25000), function() onDeflection(creature, guid) end)
        schedule(guid, "soul_blast", math.random(12000, 18000), function() onSoulBlast(creature, guid) end)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied (:920-942): guard on the latched GUIDs, resolve
-- via GetCreatureByGUID (Barnes idiom), then the tail in C++
-- order. The MoveTo fires Leryssa's second arrival and her phase
-- machine; RewardPlayerAndGroupAtEvent stays out (no bridge).
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelBcheckPoll(guid)
    local latch = talbotLatch[guid]
    talbotLatch[guid] = nil
    TalbotBCheck[guid] = nil
    if not latch or not latch.leryssaGUID or not latch.arlosGUID then
        return
    end
    local leryssa = creature:GetCreatureByGUID(latch.leryssaGUID)
    local arlos = creature:GetCreatureByGUID(latch.arlosGUID)
    if not leryssa or not arlos then
        return
    end
    arlos:Talk(SAY_ARLOS_1)
    arlos:Talk(SAY_ARLOS_2)
    leryssa:Talk(SAY_LERYSSA_1)
    arlos:Kill()
    leryssa:RemoveAura(SPELL_STUN)
    leryssa:ClearUnitState(UNIT_STATE_STUNNED)
    leryssa:SetWalk(false)
    leryssa:MoveTo(0, LERYSSA_MOVE_X, LERYSSA_MOVE_Y, LERYSSA_MOVE_Z)
end

-- C++ Initialize()/Reset() (:844-851): clear the latch + bCheck,
-- re-arm the bCheck poll (the C++ UpdateAI leg runs from spawn).
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cancelBcheckPoll(guid)
    talbotLatch[guid] = nil
    TalbotBCheck[guid] = nil
    talbotBcheckPoll[guid] = CreateLuaEvent(function() bcheckTick(creature, guid) end, 1000)
end

RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 4, onDied)
RegisterCreatureEvent(ENTRY_COUNSELOR_TALBOT, 23, onReset)

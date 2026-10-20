-- Devourer of Souls (Forge of Souls) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/ForgeOfSouls/boss_devourer_of_souls.cpp
-- (500 lines incl. license; 5 scripts — boss_devourer_of_souls
-- (CreatureScript via GetForgeOfSoulsAI (BossAI), DATA_DEVOURER_OF_SOULS =
-- 1); spell_devourer_of_souls_mirrored_soul (SpellScript OnEffectHitTarget
-- -> CastSpell MIRRORED_SOUL_PROC_AURA on hit unit);
-- spell_devourer_of_souls_mirrored_soul_proc (AuraScript DoCheckProc caster
-- alive + OnEffectProc 45%-of-damage MIRRORED_SOUL_DAMAGE redirect);
-- spell_devourer_of_souls_mirrored_soul_target_selector (SpellScript
-- TARGET_UNIT_SRC_AREA_ENTRY random-single FilterTargets + OnEffectHitTarget
-- MIRRORED_SOUL_BUFF cast); achievement_three_faced (AchievementCriteriaScript
-- OnCheck target->ToCreature AI()->GetData(DATA_THREE_FACED)); all
-- registered from inside AddSC_boss_devourer_of_souls(); loader decl 158 /
-- call 353 per northrend_script_loader.cpp — the FOURTH and LAST group of
-- the "// Forge of Souls" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_bronjahm() (call 352); no call follows it in the Forge of
-- Souls block.
-- Entries: 36502 Devourer of Souls (forge_of_souls.h NPC_DEVOURER, line 41)
-- — entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. DATA_DEVOURER_OF_SOULS = 1
-- (forge_of_souls.h line 29). DATA_THREE_FACED = 1 (boss_devourer_of_souls.cpp
-- line 107).
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_devourer_of_souls" hits boss_devourer_of_souls.cpp only (+
-- the loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No devourer lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDeath,
-- 23 OnReset. Timers via CreateLuaEvent (timers table keyed by creature
-- GUID, cancelled on events 2/4/23 — the svala / yogg_saron convention);
-- CastSpell(nil, id) = victim-cast, CastSpell(creature, id) = self-cast,
-- CastSpell(target, id) = random-player cast; {min,max} delay = randtime
-- range (the maiden_of_virtue / curator / nightbane / toravon / emalon /
-- archavon convention). None of the ported arms is a TRIGGERED cast (all
-- DoCast* are 1-2 arg, non-triggered). Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady; the
-- HasUnitState(UNIT_STATE_CASTING) gates (:251) are unmodeled (no
-- unit-state bridge — creature casts are packet-visual in Go).
-- Ported arms (C++-exact for all modeled legs):
-- JustEngagedWith (:160-169): Talk(SAY_FACE_AGGRO 0) — unconditional;
-- seeds PHANTOM_BLAST 5000, MIRRORED_SOUL 8000, WELL_OF_SOULS 30000,
-- UNLEASHED_SOULS 20000, WAILING_SOULS {60000,70000} ms.
-- EVENT_PHANTOM_BLAST (:255-256): DoCastVictim(SPELL_PHANTOM_BLAST 68982)
-- — victim-cast, non-triggered; re-arm 5000 unconditional.
-- EVENT_MIRRORED_SOUL (:259-261): DoCastAOE(SPELL_MIRRORED_SOUL_TARGET_SELECTOR
-- 69048) = self-cast; Talk(EMOTE_MIRRORED_SOUL 5) after the cast (C++
-- order kept); re-arm {15000,30000}.
-- EVENT_WELL_OF_SOULS (:264-266): SelectTarget(Random, 0) -> randomPlayer
-- (the archavon GetPlayersInWorld precedent); DoCast(target,
-- SPELL_WELL_OF_SOULS 68820), non-triggered; re-arm 20000 unconditional
-- (the SelectTarget if only wraps the cast).
-- EVENT_UNLEASHED_SOULS (:269-275): SelectTarget(Random, 0) ->
-- DoCast(target, SPELL_UNLEASHED_SOULS 68939), non-triggered;
-- Talk(SAY_FACE_UNLEASH_SOUL 7) + Talk(EMOTE_UNLEASH_SOUL 6) are
-- unconditional in C++ (outside the target-if) and kept; re-arm 30000.
-- (SetDisplayId(DISPLAY_SORROW) + the 5s FACE_ANGER DISPLAY_ANGER restore
-- are ported — see below.)
-- EVENT_WAILING_SOULS (:282-310): SetDisplayId(DISPLAY_DESIRE) first
-- (:282) + Talk(SAY_FACE_WAILING_SOUL 9) +
-- Talk(EMOTE_WAILING_SOUL 8) (C++ order kept); DoCast(me,
-- SPELL_WAILING_SOULS_STARTING 68912) = self-cast; random target ->
-- DoCast(me, SPELL_WAILING_SOULS_BEAM 68875) (gated on a target being
-- found, C++ :287-290); SetTarget(Empty) (:302) + SetControlled(true, ROOT)
-- (:305, creature:SetRooted); then the 15-tick chain (:313-316): first tick at
-- 3000, DoCast(me, SPELL_WAILING_SOULS 68873) self-cast, re-arm 1000
-- until 15 casts land (:319), then the phase-end legs
-- (SetDisplayId(DISPLAY_ANGER) + SetRooted(false), :323-324) fire and
-- WAILING_SOULS re-arms at {60000,70000} (:326).
-- C++ delays all other events by 18s during the wailing phase (:307) —
-- modeled via delayTimers (thekal precedent): the other arms' timers
-- shift 18s, so no casts fire mid-wailing. No beam sweeps the facing
-- (no SetFacingTo bridge).
-- JustDied (:203): Talk(SAY_FACE_DEATH 4) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg is unmodeled).
-- KilledUnit (:176-196): player-gated, slay text selected by the CURRENT
-- runtime display (ANGER 30148 / SORROW 30149 / DESIRE 30150 ->
-- SAY_FACE_ANGER_SLAY 1 / SAY_FACE_SORROW_SLAY 2 / SAY_FACE_DESIRE_SLAY 3).
-- PORTED 2026-10-20 — the old "no display-ID bridge" rationale was stale:
-- creature:GetDisplayId() returns the LIVE display
-- (lua_creature_events.go), and the Reset / UNLEASHED_SOULS / FACE_ANGER /
-- WAILING_SOULS / phase-end arms below keep it C++-exact.
-- Reset (:150-153): SetControlled(false, UNIT_STATE_ROOT) +
-- SetDisplayId(DISPLAY_ANGER) PORTED on ev23 (creature:SetRooted +
-- SetDisplayId; SetReactState(REACT_AGGRESSIVE) has no bridge — skipped).
-- JustEngagedWith NPC_CRUCIBLE_OF_SOULS 60yd double-spawn-guard
-- SummonCreature leg (:163-164) — no Go summon bridge, does not port.
-- EVENT_UNLEASHED_SOULS SetDisplayId(DISPLAY_SORROW) (:271) + the
-- EVENT_FACE_ANGER 5s restore SetDisplayId(DISPLAY_ANGER) (:277-278) —
-- PORTED (the stale "no display-ID bridge" claim retracted — the bridge
-- went live 2026-10-20, lua_creature_events.go).
-- EVENT_WAILING_SOULS SetDisplayId(DISPLAY_DESIRE) (:282) PORTED at the
-- arm position; the phase-end restore (:322-324) ports the
-- SetDisplayId(DISPLAY_ANGER) + SetControlled(false, ROOT) legs
-- (creature:SetDisplayId + SetRooted) — REACT_AGGRESSIVE (:322) and
-- MoveChase (:325) have no bridge and stay out.
-- The wailing beam choreography (SetFacingToObject :288, beamAngle
-- SetFacingTo :314, InterruptNonMeleeSpells :297,
-- SetReactState(REACT_PASSIVE) :299, motion Clear :304) does not port —
-- triaged out in the facing/react hunts (beam-choreography movement legs);
-- SetTarget(Empty) :302 is PORTED (2026-10-20, creature:SetTarget bridge).
-- SpellHitTarget (:225-228): H_SPELL_PHANTOM_BLAST -> threeFaced = false
-- — event 15 IS a live SpellHitTarget bridge (boss_ai.go:1904), but the
-- latch feeds only GetData(DATA_THREE_FACED) (:231-236, no GetData bridge)
-- and achievement_three_faced (no achievement bridge), so registering 15
-- would be dead code; does not port.
-- GetData(DATA_THREE_FACED) (:231-236) — no GetData bridge, does not port.
-- The three spell_devourer_of_souls_mirrored_soul* SpellScript/AuraScript
-- hooks (:365-473) — no SpellScript / AuraScript bridge (standing
-- queues), do not port.
-- achievement_three_faced (:475-493) — no GetData bridge, does not port.
-- JustDied outro loop (:206-222): 22 outroPositions team-swapped
-- instance->instance->SummonCreature spawns with MovePoint legs plus
-- summon->AI()->Talk(SAY_JAINA_OUTRO / SAY_SYLVANAS_OUTRO 0, :218/220) —
-- instance-driven Talk on other creatures with no bridge (the
-- instance_violet_hold ScheduleCyanigosaIntro precedent), does not port.
-- The file carries a @todo (model id 36503/36504 during unleash soul;
-- outro npc movement).

local ENTRY_DEVOURER = 36502

local SAY_FACE_AGGRO = 0
local SAY_FACE_ANGER_SLAY = 1
local SAY_FACE_SORROW_SLAY = 2
local SAY_FACE_DESIRE_SLAY = 3
local SAY_FACE_DEATH = 4
local EMOTE_MIRRORED_SOUL = 5
local EMOTE_UNLEASH_SOUL = 6
local SAY_FACE_UNLEASH_SOUL = 7
local EMOTE_WAILING_SOUL = 8
local SAY_FACE_WAILING_SOUL = 9

local SPELL_PHANTOM_BLAST = 68982
local SPELL_MIRRORED_SOUL_TARGET_SELECTOR = 69048
local SPELL_WELL_OF_SOULS = 68820
local SPELL_UNLEASHED_SOULS = 68939
local SPELL_WAILING_SOULS_STARTING = 68912
local SPELL_WAILING_SOULS_BEAM = 68875
local SPELL_WAILING_SOULS = 68873

local DISPLAY_ANGER = 30148
local DISPLAY_SORROW = 30149
local DISPLAY_DESIRE = 30150

local timers = {}
local devourerState = {}

local function nowMs()
    -- GetCurrTime is wall-clock ms (Eluna global, engine globals.go:941);
    -- os.clock() is CPU time and stalls while the server idles, which would
    -- corrupt the due/remaining math used by delayTimers.
    return GetCurrTime()
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, t in pairs(per) do
            RemoveEventById(t.id)
        end
        timers[guid] = nil
    end
end

-- A fired entry removes itself from the table (C++ EventMap drops consumed
-- events from the map), so delayTimers only ever shifts still-armed
-- timers — a one-shot that already fired is not resurrected by a delay.
local function armEntry(guid, key, delayMs, fn)
    local entry = { due = nowMs() + delayMs, fn = fn }
    entry.id = CreateLuaEvent(function()
        local per = timers[guid]
        if per and per[key] == entry then
            per[key] = nil
        end
        fn()
    end, delayMs)
    return entry
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key].id)
    end
    local delayMs = delay
    if type(delayMs) == "table" then
        delayMs = math.random(delayMs[1], delayMs[2])
    end
    per[key] = armEntry(guid, key, delayMs, fn)
end

-- C++ EventMap::DelayEvents: push every armed timer out by extraMs,
-- preserving each timer's remaining time (thekal precedent).
local function delayTimers(guid, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local now = nowMs()
    for key, t in pairs(per) do
        RemoveEventById(t.id)
        local remaining = t.due - now
        if remaining < 0 then
            remaining = 0
        end
        per[key] = armEntry(guid, key, remaining + extraMs, t.fn)
    end
end

-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the boss_midnight randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — alive players on the
-- same map/instance, no distance filter (the keleseth / thorim
-- GetPlayersInWorld precedent).
local function randomPlayer(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_PHANTOM_BLAST (:255-256): DoCastVictim(SPELL_PHANTOM_BLAST),
-- victim-cast, non-triggered; re-arm unconditional, 5000ms.
local function phantomBlastTick(creature, guid)
    local state = devourerState[guid]
    if state == nil then
        return
    end
    creature:CastSpell(nil, SPELL_PHANTOM_BLAST)
    schedule(guid, "phantom", 5000, function()
        phantomBlastTick(creature, guid)
    end)
end

-- C++ EVENT_MIRRORED_SOUL (:259-261): DoCastAOE(SPELL_MIRRORED_SOUL_TARGET_SELECTOR)
-- = self-cast; Talk(EMOTE_MIRRORED_SOUL) after the cast (C++ order kept);
-- re-arm unconditional, {15000,30000}ms.
local function mirroredSoulTick(creature, guid)
    local state = devourerState[guid]
    if state == nil then
        return
    end
    creature:CastSpell(creature, SPELL_MIRRORED_SOUL_TARGET_SELECTOR)
    creature:Talk(EMOTE_MIRRORED_SOUL)
    schedule(guid, "mirror", {15000, 30000}, function()
        mirroredSoulTick(creature, guid)
    end)
end

-- C++ EVENT_WELL_OF_SOULS (:264-266): DoCast(target, SPELL_WELL_OF_SOULS),
-- non-triggered; re-arm unconditional (the SelectTarget if only wraps the
-- cast), 20000ms.
local function wellOfSoulsTick(creature, guid)
    local state = devourerState[guid]
    if state == nil then
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_WELL_OF_SOULS)
    end
    schedule(guid, "well", 20000, function()
        wellOfSoulsTick(creature, guid)
    end)
end

-- C++ EVENT_UNLEASHED_SOULS (:269-275): DoCast(target,
-- SPELL_UNLEASHED_SOULS), non-triggered; SetDisplayId(DISPLAY_SORROW)
-- (:271) after the cast (C++ order); Talk(SAY_FACE_UNLEASH_SOUL) +
-- Talk(EMOTE_UNLEASH_SOUL) are unconditional in C++ (outside the
-- target-if) and kept; re-arm unconditional, 30000ms; the
-- EVENT_FACE_ANGER 5s DISPLAY_ANGER restore (:277-278) is a one-shot.
local function unleashedSoulsTick(creature, guid)
    local state = devourerState[guid]
    if state == nil then
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_UNLEASHED_SOULS)
    end
    creature:SetDisplayId(DISPLAY_SORROW)
    creature:Talk(SAY_FACE_UNLEASH_SOUL)
    creature:Talk(EMOTE_UNLEASH_SOUL)
    schedule(guid, "faceanger", 5000, function()
        creature:SetDisplayId(DISPLAY_ANGER)
    end)
    schedule(guid, "unleashed", 30000, function()
        unleashedSoulsTick(creature, guid)
    end)
end

-- C++ EVENT_WAILING_SOULS_TICK (:313-326): the 15-tick chain — first tick
-- at 3000, DoCast(me, SPELL_WAILING_SOULS) self-cast, re-arm 1000 until 15
-- casts land (:319), then the phase ends and EVENT_WAILING_SOULS
-- re-arms at {60000,70000} (:326). The beamAngle
-- Forward-declared: the 60-70s re-arm closure in wailingSoulTick below
-- fires after this chunk loads, and the local def lands later in the file.
local wailingSoulsTick
-- facing sweep (:314) has no bridge; the phase-end restore legs
-- (REACT_AGGRESSIVE + DISPLAY_ANGER + unroot + MoveChase, :322-324) have
-- no bridge and do not port.
local function wailingSoulTick(creature, guid, remaining)
    creature:CastSpell(creature, SPELL_WAILING_SOULS)
    if remaining > 1 then
        schedule(guid, "wailingtick", 1000, function()
            wailingSoulTick(creature, guid, remaining - 1)
        end)
    else
        -- C++ phase-end (:322-324): SetReactState(REACT_AGGRESSIVE) and
        -- MoveChase have no bridge (skipped); SetDisplayId(DISPLAY_ANGER)
        -- + SetControlled(false, UNIT_STATE_ROOT) are bridged.
        creature:SetDisplayId(DISPLAY_ANGER)
        creature:SetRooted(false)
        schedule(guid, "wailing", {60000, 70000}, function()
            wailingSoulsTick(creature, guid)
        end)
    end
end

-- C++ EVENT_WAILING_SOULS (:282-310): SetDisplayId(DISPLAY_DESIRE)
-- first (:282), then Talk(SAY_FACE_WAILING_SOUL) +
-- Talk(EMOTE_WAILING_SOUL) (C++ order kept); DoCast(me,
-- SPELL_WAILING_SOULS_STARTING) self-cast; random target ->
-- DoCast(me, SPELL_WAILING_SOULS_BEAM), gated on a target being found
-- (:287-290); pushes every other armed timer out 18s (:307, delayTimers —
-- no other events during wailing souls); starts the 15-tick chain at
-- 3000ms (:309). The beam/movement
-- choreography (SetFacingToObject :288, InterruptNonMeleeSpells :297,
-- REACT_PASSIVE :299, motion Clear :304) does not port — triaged out
-- in the facing/react hunts; SetTarget(Empty) :302 is PORTED
-- (2026-10-20, creature:SetTarget bridge) and
-- SetControlled(true, UNIT_STATE_ROOT) :305 via creature:SetRooted.
-- Assigned to the forward-declared upvalue (moam precedent): `local function`
-- here would shadow it and leave the re-arm closure bound to nil.
wailingSoulsTick = function(creature, guid)
    local state = devourerState[guid]
    if state == nil then
        return
    end
    creature:SetDisplayId(DISPLAY_DESIRE)
    creature:Talk(SAY_FACE_WAILING_SOUL)
    creature:Talk(EMOTE_WAILING_SOUL)
    creature:CastSpell(creature, SPELL_WAILING_SOULS_STARTING)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(creature, SPELL_WAILING_SOULS_BEAM)
    end
    -- C++ :302 SetTarget(ObjectGuid::Empty) — "remove any target" after the
    -- beam, before the motion Clear; ported 2026-10-20 via the
    -- creature:SetTarget bridge (0 = ObjectGuid::Empty).
    creature:SetTarget(0)
    -- C++ :305 SetControlled(true, UNIT_STATE_ROOT) — after the
    -- unbridged :304 motion Clear; the root leg alone keeps the boss
    -- stationary via the chase-pause ScriptRooted gate (creaturemotion.go).
    creature:SetRooted(true)
    -- C++ :307 events.DelayEvents(18s) — push every other armed timer out
    -- 18s (no other events during wailing souls); the tick chain is
    -- scheduled after, so it is not delayed.
    delayTimers(guid, 18000)
    schedule(guid, "wailingtick", 3000, function()
        wailingSoulTick(creature, guid, 15)
    end)
end

-- C++ boss_devourer_of_soulsAI::JustEngagedWith (:160-169):
-- Talk(SAY_FACE_AGGRO) — unconditional — plus the five ScheduleEvent seeds
-- (PHANTOM_BLAST 5000 / MIRRORED_SOUL 8000 / WELL_OF_SOULS 30000 /
-- UNLEASHED_SOULS 20000 / WAILING_SOULS {60000,70000}ms). The
-- BossAI::JustEngagedWith passthrough leg and the NPC_CRUCIBLE_OF_SOULS
-- double-spawn-guard SummonCreature leg (:163-164, no Go summon bridge)
-- do not port.
local function devourerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    devourerState[guid] = {}
    creature:Talk(SAY_FACE_AGGRO)
    schedule(guid, "phantom", 5000, function()
        phantomBlastTick(creature, guid)
    end)
    schedule(guid, "mirror", 8000, function()
        mirroredSoulTick(creature, guid)
    end)
    schedule(guid, "well", 30000, function()
        wellOfSoulsTick(creature, guid)
    end)
    schedule(guid, "unleashed", 20000, function()
        unleashedSoulsTick(creature, guid)
    end)
    schedule(guid, "wailing", {60000, 70000}, function()
        wailingSoulsTick(creature, guid)
    end)
end

-- Shared timer/state wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function devourerCancelCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    devourerState[guid] = nil
end

-- C++ JustDied (:203): _JustDied() (passthrough, no bridge);
-- Talk(SAY_FACE_DEATH) — unconditional — the sjonnir JustDied-Talk
-- precedent; the DATA_TEAM_IN_INSTANCE outro-summon loop and the
-- summon->AI()->Talk(SAY_JAINA_OUTRO / SAY_SYLVANAS_OUTRO) legs are
-- instance-driven Talk on other creatures with no bridge and do not port.
local function devourerJustDied(event, creature)
    devourerCancelCombat(event, creature)
    creature:Talk(SAY_FACE_DEATH)
end

-- C++ boss_devourer_of_soulsAI::Reset (:148-155): _Reset() (timer/state
-- wipe) + SetControlled(false, UNIT_STATE_ROOT) + SetDisplayId(
-- DISPLAY_ANGER) + SetReactState(REACT_AGGRESSIVE) (no bridge — skipped) +
-- Initialize() (threeFaced latch — its only consumer is the unregistered
-- ev15 SpellHitTarget arm, so it stays out).
local function devourerReset(event, creature)
    devourerCancelCombat(event, creature)
    creature:SetRooted(false)
    creature:SetDisplayId(DISPLAY_ANGER)
end

-- C++ KilledUnit (:176-196): player-gated; slay text selected by the live
-- runtime display (DISPLAY_ANGER -> 1 / DISPLAY_SORROW -> 2 /
-- DISPLAY_DESIRE -> 3), kept C++-exact because every display-mutating arm
-- above is now ported.
local function devourerTargetDied(event, creature, victim)
    if not victim or victim:GetObjectType() ~= "Player" then
        return
    end
    local textId = 0
    local displayId = creature:GetDisplayId()
    if displayId == DISPLAY_ANGER then
        textId = SAY_FACE_ANGER_SLAY
    elseif displayId == DISPLAY_SORROW then
        textId = SAY_FACE_SORROW_SLAY
    elseif displayId == DISPLAY_DESIRE then
        textId = SAY_FACE_DESIRE_SLAY
    end
    if textId ~= 0 then
        creature:Talk(textId)
    end
end

RegisterCreatureEvent(ENTRY_DEVOURER, 1, devourerEnterCombat)
RegisterCreatureEvent(ENTRY_DEVOURER, 2, devourerCancelCombat)
RegisterCreatureEvent(ENTRY_DEVOURER, 3, devourerTargetDied)
RegisterCreatureEvent(ENTRY_DEVOURER, 4, devourerJustDied)
RegisterCreatureEvent(ENTRY_DEVOURER, 23, devourerReset)

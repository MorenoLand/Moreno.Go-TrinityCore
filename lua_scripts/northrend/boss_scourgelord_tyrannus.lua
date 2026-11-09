-- Scourgelord Tyrannus + Rimefang (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_scourgelord_tyrannus.cpp
-- (571 lines incl. license; 6 scripts — boss_tyrannus (CreatureScript via
-- GetPitOfSaronAI (BossAI), DATA_TYRANNUS = 2); boss_rimefang (CreatureScript
-- via GetPitOfSaronAI (ScriptedAI), DATA_RIMEFANG = 3);
-- player_overlord_brandAI (PlayerAI pushed onto overlord-brand victims:
-- DamageDealt -> SPELL_OVERLORD_BRAND_DAMAGE 69189, HealDone ->
-- SPELL_OVERLORD_BRAND_HEAL 69190); spell_tyrannus_overlord_brand /
-- spell_tyrannus_mark_of_rimefang (AuraScripts);
-- spell_tyrannus_rimefang_icy_blast (SpellScript: EFFECT_1
-- SPELL_EFFECT_TRIGGER_MISSILE -> SummonCreature(NPC_ICY_BLAST 36731) +
-- triggered SPELL_ICY_BLAST_AURA 69238); at_tyrannus_event_starter
-- (AreaTriggerScript: tyrannus->AI()->DoAction(ACTION_START_INTRO)); all
-- registered from inside AddSC_boss_tyrannus(); loader decl 164 / call 359
-- per northrend_script_loader.cpp — the FIFTH and LAST group of the
-- "// Pit of Saron" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_ick() (call 358)).
-- Entries: 36658 Tyrannus (pit_of_saron.h NPC_TYRANNUS, line 52;
-- instance_pit_of_saron.cpp OnCreatureCreate binds case NPC_TYRANNUS, line
-- 83); 36661 Rimefang (pit_of_saron.h NPC_RIMEFANG, line 53;
-- OnCreatureCreate-bound line 86) — both entry-verifiable; the CreatureScript
-- ScriptName binding is DB-side as usual. NPC_ICY_BLAST = 36731
-- (pit_of_saron.h, line 94); NPC_TYRANNUS_EVENTS = 36794 (line 55).
-- RETRACTION NOTE (2026-10-13): the Oct-2 version of this file was a
-- Talk-only port written under the no-timer-bridge doctrine. That doctrine
-- is stale and disproved — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798; the maiden_of_virtue / moroes / curator
-- / nightbane convention) — so every ungated timer-driven cast arm is
-- re-ported below, C++-exact for all modeled arms. Rimefang's
-- EVENT_ICY_BLAST is one such arm, so entry 36661 is now registered in
-- engine/world/boss_ai.go (RegisterLuaBoss("boss_rimefang", 36661)); the
-- Oct-2 "zero own Talk lines, not registered" call is retracted.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied
-- (player-gated), 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- ({min,max} table = randtime range); one module-local timers table keyed
-- by creature GUID serves both entries, cancelled on events 2/4/23 (the
-- bronjahm / devourer_of_souls convention); CastSpell(nil, id) =
-- victim-cast (DoCastVictim), CastSpell(creature, id) = self-cast
-- (DoCast(me)), CastSpell(target, id) = targeted cast; all DoCast* here are
-- 2-arg, non-triggered. Melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady (:284).
-- VEHICLE GATING (key question): Tyrannus rides Rimefang only during the
-- 36s intro — EVENT_INTRO_3 (:238-240) calls me->ExitVehicle() and the
-- combat schedule starts at EVENT_COMBAT_START (:242-252, 36s) after the
-- exit, so ALL FOUR Tyrannus combat arms are vehicle-UNGATED and port.
-- Rimefang's icy-blast arm is likewise ungated by its waypoint patrol (an
-- independent event arm). The vehicle legs themselves (ExitVehicle :239,
-- JustReachedHome InstallAllAccessories :322-326) have no bridge and stay
-- out — documented below.
-- Ported arms (C++ line refs; C++-exact for all modeled arms):
-- Tyrannus JustEngagedWith (:164) — Talk(SAY_AGGRO 8); NO BossAI
--   base call, so NO SetBossState(IN_PROGRESS) on engage — it lives
--   in DoAction(ACTION_START_INTRO) (:217) behind the unported
--   area-trigger machine. The EVENT_COMBAT_START seed legs (:250-252)
--   land here because Eluna event 1 fires when DoZoneInCombat()
--   (:249) puts Tyrannus in combat — the faithful combat-start
--   anchor: OVERLORD_BRAND 69172 {5000,7000}ms, FORCEFUL_SMASH 69155
--   {14000,16000}ms, MARK_OF_RIMEFANG 69275 {25000,27000}ms.
-- EVENT_OVERLORD_BRAND (:254-257) — SelectTarget(Random, 1, 0.0f, true) ->
--   random alive threat-list player past the first MaxThreat entry
--   (GetAITargets[2..], eck precedent); DoCast(target, 69172);
--   re-arm {11000,12000}ms.
-- EVENT_FORCEFUL_SMASH (:259-261) — DoCastVictim(69155) victim-cast;
--   chains EVENT_UNHOLY_POWER at 1000ms (no direct re-arm of smash here).
-- EVENT_UNHOLY_POWER (:263-267) — Talk(SAY_DARK_MIGHT_1 13) then
--   Talk(SAY_DARK_MIGHT_2 14) (C++ order kept), DoCast(me, 69167)
--   self-cast; re-arms EVENT_FORCEFUL_SMASH {40000,48000}ms (the smash ->
--   unholy -> smash loop).
-- EVENT_MARK_OF_RIMEFANG (:269-276) — Talk(SAY_MARK_RIMEFANG_1 11);
--   SelectTarget(Random, 1, 0.0f, true) -> random alive threat-list
--   player past the first MaxThreat entry (GetAITargets[2..]); if found,
--   Talk(SAY_MARK_RIMEFANG_2 12, target) (whisper-target bridge) +
--   DoCast(target, 69275); re-arm {24000,26000}ms.
-- Tyrannus KilledUnit (:187) — Talk(SAY_SLAY 9) player-gated (the
--   razuvious player-gated variant precedent).
-- Tyrannus JustDied (:193) — Talk(SAY_DEATH 10) FIRST, then
--   SetBossState(DATA_TYRANNUS 2, DONE) (C++ order :194/:196);
--   timers cancelled (the sjonnir JustDied-Talk precedent).
-- Tyrannus Reset (:150-162) / EnterEvadeMode (:178-186) — PORTED in
--   tyrannusReset (ev23): FAIL on the evade path (state was
--   IN_PROGRESS), NOT_STARTED otherwise (!= DONE); the REACT_PASSIVE
--   + NON_ATTACKABLE arms stay out (see documented-only note).
-- Rimefang DoAction(ACTION_START_RIMEFANG) (:328-336) — PORTED
--   2026-11-08 (tick 490) on the ev38 receiver below: the :335
--   EVENT_ICY_BLAST seed (15s) lands on Rimefang's own Eluna event 1
--   (DoZoneInCombat :333 fires it, the faithful anchor); the :332
--   phase flip is the chain's existence (the patrol only runs once the
--   receiver starts it). The receiver has no live sender yet — its
--   only caller is tyrannus's EVENT_COMBAT_START (:244), itself driven
--   by the dormant intro machine above.
-- Rimefang EVENT_ICY_BLAST (:368-371) — SelectTarget(Random, 0)
-- (playerOnly=false default, UnitAI.h:170) -> random threat-listed unit
-- (GetAITargets pick, no Player filter); DoCast(target, 69232);
-- re-arm 15s unconditional (the SelectTarget if only wraps the cast).
-- Cast-state gate, checked-no-gate (2026-11-04): boss UpdateAI (:221)
-- carries NO pre-loop CASTING gate — the sole :280 check sits AFTER
-- the event switch inside the loop, so it only suppresses LATER events
-- in the same UpdateAI call once an earlier event started a cast.
-- Lua ticks each execute as a single first-event firing, so the
-- in-loop gate never suppresses one: brand/smash/unholy/mark stay
-- ungated, C++-exact (not a deviation — the old "gate unmodeled"
-- note is corrected). Rimefang UpdateAI (:350) has zero CASTING
-- hits, so icy-blast stays ungated too.
-- The SelectTarget position-1 offset on the two Tyrannus random-target
-- arms (:255, :272) IS bridged — GetAITargets[2..] (eck precedent),
-- fixed 2026-10-25 from the plain GetPlayersInWorld pick; no target at
-- all when none qualify (Talk + cast stay inside the C++ target-found
-- legs where C++ has them: overlord brand :255-256, mark :272-275).
-- Rimefang's SelectTarget(Random, 0) (:369) is not playerOnly in C++ —
-- it is an alive-filtered pick over the bridged threat list
-- GetAITargets (the midnight precedent; C++ SelectTargetList skips
-- offline refs), no Player filter.
-- DOCUMENTED-ONLY (in this header; registrations: 36658 + 36661):
-- Tyrannus DoAction(ACTION_START_INTRO) (:207-217) + intro machine
--   (:219-252) — PORTED 2026-11-08 (tick 490) on the ev38 receiver
--   (dispatch IS bridged; the ick DoAction receiver precedent):
--   Talk(SAY_TYRANNUS_INTRO_1 6) + SetBossState(IN_PROGRESS) (:217),
--   EVENT_INTRO_2 Talk(SAY_TYRANNUS_INTRO_3 7) at 22s (:235-236),
--   EVENT_INTRO_3 ExitVehicle + MovePoint(miscPos) at 34s (:238-240,
--   ExitVehicle is bridged, lua_creature_events.go:3285, and no-ops
--   when not on a vehicle), EVENT_COMBAT_START at 36s (:242-252:
--   rimefang DoAction(ACTION_START_RIMEFANG), RemoveFlag(
--   NON_ATTACKABLE), SetReactState(REACT_AGGRESSIVE), DoCast(
--   SPELL_FULL_HEAL 43979), DoZoneInCombat — which fires ev1, where
--   the brand/smash/mark seeds already live, so COMBAT_START does
--   NOT re-seed). DORMANT: the only sender is the unported
--   at_tyrannus_event_starter area trigger (see below), so the
--   receiver has no live sender in Go yet (the terenas DoAction
--   precedent: receiver ported, sender unported). EVENT_INTRO_1
--   (:232-234) is a dead case — its only leg is a commented-out
--   Talk(SAY_GORKUN_INTRO_2) — so nothing is scheduled for it.
-- Tyrannus Reset / InitializeAI (:142-162) — the SetBossState legs
--   are PORTED in tyrannusReset below (Reset :156 NOT_STARTED direct
--   instance call; EnterEvadeMode :180 FAIL on the evade path; the
--   skadi / palehoof ev23 idiom). REACT_PASSIVE + UNIT_FLAG_NON_
--   ATTACKABLE stay out: the intro DoAction/area-trigger machine has
--   no live sender, and players engage by attacking — setting
--   NON_ATTACKABLE on spawn would block the only engage path and
--   strand the boss passive (the header's old "intro unported" note
--   understated this: it is a soft-lock, not a cosmetic gap).
--   InitializeAI (:142-148: GetBossState != DONE -> Reset, else
--   DespawnOrUnsummon) has no spawn-hook fire site in Go — ev23's
--   spawn pre-fire is a Reset, not InitializeAI, and a spawn-time
--   DespawnOrUnsummon on a static spawn cannot be verified against
--   Go's grid re-materialization, so the DONE-despawn leg stays out.
-- Tyrannus AttackStart NON_ATTACKABLE gate (:169-176) — no AttackStart
--   hook in Go (engage is Eluna event 1); the engine owns
--   attack-start, and the flag legs that would make the gate
--   observable stay out above.
-- EnterEvadeMode (:178-186): the SetBossState(FAIL) leg is PORTED
--   in tyrannusReset (ev23 evade path), and the rimefang->AI()->
--   EnterEvadeMode() leg is PORTED 2026-11-08 (tick 490) in the same
--   FAIL branch (creature:EnterEvadeMode, lua_creature_events.go:5014,
--   via the instanceData.rimefang bind); DespawnOrUnsummon stays out
--   (the Go evade funnel owns evade-despawn timing — boss_ai.go
--   despawnAtEvade).
-- Tyrannus JustDied instance legs (:193-206) — SetBossState(DONE)
--   is PORTED in tyrannusJustDied below (after Talk, C++ order); the
--   SetBossState-override DONE-spawn leg (instance_pit_of_saron.cpp
--   :175-190: Jaina part 2 / Sylvanas part 2 at EventLeaderPos2) is
--   PORTED 2026-11-08 (tick 490) in tyrannusJustDied below, C++ order
--   (inside SetBossState, before the rimefang leg); the rimefang->AI()->
--   DoAction(ACTION_END_COMBAT) leg is PORTED 2026-11-08 (tick 490)
--   via the rimefang ev38 receiver below; TEMPSUMMON_DEAD_DESPAWN
--   stays out — C++ gates it on ToTempSummon(), which is null for the
--   static-spawn boss, so the leg is vacuous in the real instance.
-- The Overlord Brand aura chain — spell_tyrannus_overlord_brand
--   AuraScript OnApply pushes player_overlord_brandAI (:398-428) onto the
--   victim (DamageDealt -> SPELL_OVERLORD_BRAND_DAMAGE 69189, HealDone ->
--   SPELL_OVERLORD_BRAND_HEAL 69190 with the addHealth * 5.5f leg) — no
--   AuraScript / PlayerAI-push bridge; the 69172 cast itself IS ported, the
--   redirect legs stay out.
-- The Mark -> Hoarfrost cross-creature chain —
--   spell_tyrannus_mark_of_rimefang AuraScript OnApply (:474-505) calls
--   rimefang->AI()->SetGUID(target-guid, GUID_HOARFROST) (:341-347),
--   scheduling EVENT_HOARFROST at 1s (:373-378: DoCast(target,
--   SPELL_HOARFROST 69246)) — PORTED 2026-11-08 (tick 427) via the
--   ev2 hook + a module-local target latch (markOfRimefangApply
--   below); no AI-SetGUID bridge exists, but its only consumer is
--   the 1s-delayed cast, which the latch reproduces. The 69275
--   cast IS ported.
-- Rimefang EVENT_MOVE_NEXT (:361-366) — waypoint patrol over rimefangPos
--   (MovePoint, waypoints 1..9 then wrap) — PORTED 2026-11-08 (tick 490)
--   via the DoAction(ACTION_START_RIMEFANG) receiver below: the patrol
--   re-schedules every 2s PHASE_COMBAT (the chain only exists once the
--   receiver starts it, and ev2/4/23 cancel it); waypoint index 0 is
--   never flown in C++ (the first tick bumps 0 -> 1), so the Lua index
--   starts at 1 the same way.
-- Rimefang Reset (:306-320: waypoint/GUID init + SetCanFly / REACT_PASSIVE
--   / NON_ATTACKABLE), JustReachedHome InstallAllAccessories (:322-326,
--   vehicle), DoAction legs (:328-340: ACTION_START_RIMEFANG /
--   ACTION_END_COMBAT) — the REACT_PASSIVE and SetCanFly(true) arms are
--   PORTED 2026-10-20/22 (creature:SetReactState / creature:SetCanFly);
--   the DoAction legs are PORTED 2026-11-08 (tick 490) on the ev38
--   receiver below (ACTION_START_RIMEFANG schedules the patrol +
--   DoZoneInCombat, which fires ev1 for the icy-blast seed; ACTION_END_
--   COMBAT is the _EnterEvadeMode twin, creature:EnterEvadeMode);
--   JustReachedHome stays out — CREATURE_EVENT_ON_REACH_HOME (24)
--   exists, but InstallAllAccessories has no bridge and no vehicle
--   bridge exists to mount accessories on. The :319 SetFlag(
--   NON_ATTACKABLE) is never cleared live anywhere in the rimefang AI
--   (permanent vehicle-dragon property; flags re-affirm to template on
--   the creature sweep) — VACUOUS, not ported.
-- spell_tyrannus_rimefang_icy_blast SpellScript (:507-539: EFFECT_1
--   SPELL_EFFECT_TRIGGER_MISSILE -> SummonCreature(NPC_ICY_BLAST 36731) +
--   triggered SPELL_ICY_BLAST_AURA 69238) — no SpellScript / summon bridge.
-- at_tyrannus_event_starter (:541-560) — the area-trigger SENDER stays
--   out: server event 24 IS bridged (areatrigger.go:64, the
--   at_scripts.lua precedent), but the trigger ID lives only in
--   AreaTrigger.dbc / the DB areatrigger_template — neither is in-tree,
--   and this clone carries no sql/ tree — so no handler can dispatch
--   on it; player:IsGameMaster has no bridge either, so the GM veto
--   (:545) could not be expressed. The DoAction receiver it would
--   trigger is ported above (dormant).
-- Upstream C++ findings: EVENT_INTRO_1 (:232-234) is a dead case — its only
-- leg is a commented-out Talk(SAY_GORKUN_INTRO_2); the SAY_GORKUN_*
-- (:34-36), SAY_JAYNA_OUTRO_* (:53-55) and SAY_SYLVANAS_OUTRO_* (:58-59)
-- yells are never Talked in this TU (the outro Talk legs live in
-- instance_pit_of_saron.cpp); EVENT_HOARFROST re-schedules unconditionally
-- on every mark-aura apply with no duplicate guard (:346).

local ENTRY_TYRANNUS = 36658
local ENTRY_RIMEFANG = 36661
local DATA_TYRANNUS = 2 -- pit_of_saron.h:34
local DATA_RIMEFANG = 3 -- pit_of_saron.h:37

local SAY_AGGRO = 8
local SAY_SLAY = 9
local SAY_DEATH = 10
local SAY_MARK_RIMEFANG_1 = 11
local SAY_MARK_RIMEFANG_2 = 12
local SAY_DARK_MIGHT_1 = 13
local SAY_DARK_MIGHT_2 = 14
local SAY_TYRANNUS_INTRO_1 = 6
local SAY_TYRANNUS_INTRO_3 = 7

local SPELL_OVERLORD_BRAND = 69172
local SPELL_FORCEFUL_SMASH = 69155
local SPELL_UNHOLY_POWER = 69167
local SPELL_MARK_OF_RIMEFANG = 69275
local SPELL_HOARFROST = 69246 -- boss_scourgelord_tyrannus.cpp:70
local SPELL_ICY_BLAST = 69232
local SPELL_FULL_HEAL = 43979 -- boss_scourgelord_tyrannus.cpp:77

-- boss_scourgelord_tyrannus.cpp:122-131 rimefangPos[0..9], verbatim
-- (Lua index = C++ index + 1); index 0 is never flown — the first
-- MOVE_NEXT tick bumps 0 -> 1 and the wrap is >= 10 -> 1, so the
-- patrol flies C++ 1..9 only (rimefangMoveNext below).
local RIMEFANG_POS = {
    {1017.299, 168.9740, 642.9259},
    {1047.868, 126.4931, 665.0453},
    {1069.828, 138.3837, 665.0453},
    {1063.042, 164.5174, 665.0453},
    {1031.158, 195.1441, 665.0453},
    {1019.087, 197.8038, 665.0453},
    {967.6233, 168.9670, 665.0453},
    {969.1198, 140.4722, 665.0453},
    {986.7153, 141.6424, 665.0453},
    {1012.601, 142.4965, 665.0453},
}

-- boss_scourgelord_tyrannus.cpp:110 miscPos (tyrannus combat start).
local MISC_POS_X = 1018.376
local MISC_POS_Y = 167.2495
local MISC_POS_Z = 628.2811

-- boss_scourgelord_tyrannus.cpp:117-122 Actions enum.
local ACTION_START_INTRO = 1
local ACTION_START_RIMEFANG = 2
local ACTION_END_COMBAT = 4

-- UnitDefines.h / UpdateFields.h: UNIT_FIELD_FLAGS = 59,
-- UNIT_FLAG_NON_ATTACKABLE = 0x00000002; REACT_AGGRESSIVE = 2.
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x00000002
local REACT_AGGRESSIVE = 2

-- instance_pit_of_saron.cpp:31 EventLeaderPos2; pit_of_saron.h:60.
local NPC_JAINA_PART2 = 38188
local NPC_SYLVANAS_PART2 = 38189
local EVENT_LEADER_X = 1054.368
local EVENT_LEADER_Y = 107.14620
local EVENT_LEADER_Z = 628.4467
local ALLIANCE = 0 -- TeamId TEAM_ALLIANCE; player:GetTeam() returns it

-- Rimefang waypoint index keyed by GUID (the _currentWaypoint twin).
local rimefangWaypoint = {}

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

-- Re-scheduling a key replaces the pending timer, so each named arm has
-- exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 1, 0.0f, true) (:255/:271) —
-- offset 1 pops the current victim (UnitAI.h non-distance list is
-- victim-first), not the threat-sorted head: exclude by victim GUID,
-- then a random alive player from the rest (dead targets are offline
-- refs, Go SortedEntries returns them too, so IsAlive filters).
local function randomPlayer(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetObjectType() == "Player" and t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :254-257 EVENT_OVERLORD_BRAND: DoCast(target, 69172), non-triggered;
-- re-arm {11000,12000}ms unconditional.
local function overlordBrandTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_OVERLORD_BRAND)
    end
    schedule(guid, "brand", {11000, 12000}, function() overlordBrandTick(creature, guid) end)
end

-- C++ :259-261 EVENT_FORCEFUL_SMASH: DoCastVictim(69155) victim-cast,
-- non-triggered; chains EVENT_UNHOLY_POWER at 1000ms (no direct re-arm).
local unholyPowerTick
local function forcefulSmashTick(creature, guid)
    creature:CastSpell(nil, SPELL_FORCEFUL_SMASH)
    schedule(guid, "unholy", 1000, function() unholyPowerTick(creature, guid) end)
end

-- C++ :263-267 EVENT_UNHOLY_POWER: Talk(13), Talk(14) (C++ order kept),
-- DoCast(me, 69167) self-cast, non-triggered; re-arms EVENT_FORCEFUL_SMASH
-- {40000,48000}ms.
unholyPowerTick = function(creature, guid)
    creature:Talk(SAY_DARK_MIGHT_1)
    creature:Talk(SAY_DARK_MIGHT_2)
    creature:CastSpell(creature, SPELL_UNHOLY_POWER)
    schedule(guid, "smash", {40000, 48000}, function() forcefulSmashTick(creature, guid) end)
end

-- C++ :269-276 EVENT_MARK_OF_RIMEFANG: Talk(11) unconditional; target-if:
-- Talk(12, target) (whisper-target bridge) + DoCast(target, 69275)
-- non-triggered; re-arm {24000,26000}ms unconditional.
local function markOfRimefangTick(creature, guid)
    creature:Talk(SAY_MARK_RIMEFANG_1)
    local target = randomPlayer(creature)
    if target then
        creature:Talk(SAY_MARK_RIMEFANG_2, target)
        creature:CastSpell(target, SPELL_MARK_OF_RIMEFANG)
    end
    schedule(guid, "mark", {24000, 26000}, function() markOfRimefangTick(creature, guid) end)
end

-- C++ spell_tyrannus_mark_of_rimefang AuraScript OnApply
-- (boss_scourgelord_tyrannus.cpp:484-494): AfterEffectApply, EFFECT_0
-- SPELL_AURA_DUMMY. When Mark of Rimefang (69275) applies, Rimefang is
-- resolved via instance GetGuidData(DATA_RIMEFANG) and its AI gets
-- SetGUID(marked player's GUID, GUID_HOARFROST) (:341-347), scheduling
-- EVENT_HOARFROST at 1s (:373-378: DoCast(target, SPELL_HOARFROST
-- 69246), clears the stored GUID). No AI-SetGUID bridge exists, but
-- its only consumer is that 1s-delayed cast on the passed GUID, which
-- the aura hook + a module-local latch reproduces: the latch holds the
-- marked target, the 1s timer casts on it and clears the latch. PORTED
-- 2026-11-08 (tick 427).
-- LIVE-ABLE: the 69275 cast itself is ported (markOfRimefangTick
-- above). The timer rides the file's schedule helper (cancelled on
-- 2/4/23), so a double-mark inside 1s fires once, slightly later
-- than C++ (marks re-arm 24-26s apart, so the edge is unreachable
-- in practice); the IsDead guard is the Go twin of the C++ cast's
-- dead-target failure.
local hoarfrostTarget = nil

local function markOfRimefangApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    local rimefang = caster:GetInstanceCreature(DATA_RIMEFANG)
    if rimefang == nil then
        return
    end
    hoarfrostTarget = target
    local guid = caster:GetGUID()
    schedule(guid, "hoarfrost", 1000, function()
        local victim = hoarfrostTarget
        hoarfrostTarget = nil
        if victim ~= nil and not victim:IsDead() then
            rimefang:CastSpell(victim, SPELL_HOARFROST)
        end
    end)
end

RegisterAuraEvent(SPELL_MARK_OF_RIMEFANG, 2, markOfRimefangApply)

-- C++ :242-252 EVENT_COMBAT_START: rimefang DoAction(
-- ACTION_START_RIMEFANG); phase flip; RemoveFlag(NON_ATTACKABLE);
-- REACT_AGGRESSIVE; DoCast(FULL_HEAL 43979); DoZoneInCombat (fires
-- ev1, where the brand/smash/mark seeds already live, so COMBAT_START
-- does NOT re-seed them).
local function tyrannusCombatStart(creature, guid)
    local mapObj = creature:GetMap()
    local data = nil
    if mapObj then
        data = mapObj:GetInstanceData()
    end
    if data and data.rimefang then
        data.rimefang:DoAction(ACTION_START_RIMEFANG)
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:CastSpell(creature, SPELL_FULL_HEAL)
    creature:DoZoneInCombat()
end

-- C++ boss_tyrannus::DoAction (boss_scourgelord_tyrannus.cpp:207-217)
-- + intro machine (:219-252), on the ev38 receiver (dispatch IS
-- bridged; the ick DoAction receiver precedent): Talk(
-- SAY_TYRANNUS_INTRO_1 6) + SetBossState(IN_PROGRESS) (:217);
-- EVENT_INTRO_2 Talk(SAY_TYRANNUS_INTRO_3 7) at 22s (:235-236);
-- EVENT_INTRO_3 ExitVehicle + MovePoint(miscPos) at 34s (:238-240,
-- ExitVehicle is bridged, lua_creature_events.go:3285, and no-ops
-- when not on a vehicle); EVENT_COMBAT_START at 36s (:242-252, the
-- tyrannusCombatStart leg above). EVENT_INTRO_1 (:232-234) is a dead
-- case — its only leg is a commented-out Talk — so nothing is
-- scheduled for it. DORMANT: the only sender is the unported
-- at_tyrannus_event_starter area trigger, so the receiver has no live
-- sender in Go yet (the terenas DoAction precedent: receiver ported,
-- sender unported).
local function tyrannusDoAction(event, creature, action)
    if action ~= ACTION_START_INTRO then
        return
    end
    local guid = creature:GetGUID()
    creature:Talk(SAY_TYRANNUS_INTRO_1)
    creature:SetBossState(DATA_TYRANNUS, 1) -- IN_PROGRESS (:217)
    schedule(guid, "intro2", 22000, function()
        creature:Talk(SAY_TYRANNUS_INTRO_3)
    end)
    schedule(guid, "intro3", 34000, function()
        creature:ExitVehicle()
        creature:MoveTo(0, MISC_POS_X, MISC_POS_Y, MISC_POS_Z)
    end)
    schedule(guid, "combatstart", 36000, function() tyrannusCombatStart(creature, guid) end)
end

-- C++ :164 JustEngagedWith: Talk(SAY_AGGRO) — NOTE: unlike the other
-- BossAI bosses in this strip, tyrannus does NOT call BossAI::
-- JustEngagedWith here, so NO SetBossState(IN_PROGRESS) lands on
-- engage (the trap a naive port would fall into). IN_PROGRESS is set
-- only by DoAction(ACTION_START_INTRO) (:217) — the receiver above
-- (its only sender, the area trigger, stays out; see the header).
-- The :250-252 EVENT_COMBAT_START seed legs land here — Eluna event 1
-- fires at the same combat-start moment (DoZoneInCombat :249).
local function tyrannusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    -- C++ :247 EVENT_COMBAT_START: me->SetReactState(REACT_AGGRESSIVE)
    -- (bridged, lua_creature_events.go:524); in C++ arm order it precedes
    -- the DoZoneInCombat (:249) that fires this handler, so it leads.
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    creature:Talk(SAY_AGGRO)
    schedule(guid, "brand", {5000, 7000}, function() overlordBrandTick(creature, guid) end)
    schedule(guid, "smash", {14000, 16000}, function() forcefulSmashTick(creature, guid) end)
    schedule(guid, "mark", {25000, 27000}, function() markOfRimefangTick(creature, guid) end)
end

-- C++ :187 KilledUnit: player-gated Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent.
local function tyrannusKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- Shared timer wipe for event 2 (the bronjahm convention).
local function tyrannusCancelCombat(event, creature)
    initState(creature:GetGUID())
end

-- C++ Reset (:150-162) + EnterEvadeMode (:178-186), routed through the
-- single Go ev23 fire site (the skadi / palehoof idiom): ev23 fires on
-- spawn pre-fire, on the death pre-fire, and on evade-reset. The
-- death pre-fire is skipped via the IsAlive guard (DONE lands in
-- ev4); the evade path (stored state IN_PROGRESS) sets FAIL — the
-- EnterEvadeMode leg (:180); anything else (!= DONE) is the Reset
-- leg (:156, direct instance call, no _Reset() — hence no DONE-gate
-- in C++ either, but the spawn pre-fire must not revive a DONE boss,
-- so the != DONE guard stands). The EnterEvadeMode rimefang leg
-- (:181-182) is PORTED in the FAIL branch below (creature:
-- EnterEvadeMode, lua_creature_events.go:5014, via the
-- instanceData.rimefang bind); the DespawnOrUnsummon leg stays out
-- — the Go evade funnel owns evade-despawn timing (boss_ai.go
-- despawnAtEvade).
local function tyrannusReset(event, creature)
    if not creature:IsAlive() then
        initState(creature:GetGUID())
        return
    end
    if creature:GetBossState(DATA_TYRANNUS) == 1 then
        creature:SetBossState(DATA_TYRANNUS, 2) -- FAIL: evade path
        -- C++ :181-182 EnterEvadeMode: rimefang->AI()->EnterEvadeMode().
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data and data.rimefang then
                data.rimefang:EnterEvadeMode()
            end
        end
    elseif creature:GetBossState(DATA_TYRANNUS) ~= 3 then
        creature:SetBossState(DATA_TYRANNUS, 0) -- NOT_STARTED: Reset / spawn path
    end
    initState(creature:GetGUID())
end

-- C++ :193 JustDied: Talk(SAY_DEATH) FIRST (:194), then SetBossState
-- (DATA_TYRANNUS 2, DONE) (:196) — the opposite order from bronjahm /
-- garfrost / ick, where _JustDied() leads. Timers cancelled. Then the
-- instance SetBossState-override DONE-spawn leg
-- (instance_pit_of_saron.cpp:175-190, fired inside SetBossState in
-- C++): Jaina part 2 (38188, Alliance) / Sylvanas part 2 (38189,
-- Horde) at EventLeaderPos2, TEMPSUMMON_MANUAL_DESPAWN (7) — team
-- read as instanceData.teamInInstance or 0 (the C++ ctor default);
-- the instanceData.tyrannus nil-guard mirrors the C++ instance->
-- GetCreature(_tyrannusGUID) check. Then the rimefang->AI()->
-- DoAction(ACTION_END_COMBAT) leg (:201-206) via the ev38 receiver
-- below. TEMPSUMMON_DEAD_DESPAWN stays out — C++ gates it on
-- ToTempSummon(), which is null for the static-spawn boss, so the leg
-- is vacuous in the real instance.
local function tyrannusJustDied(event, creature)
    initState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_TYRANNUS, 3)
    local mapObj = creature:GetMap()
    local data = nil
    if mapObj then
        data = mapObj:GetInstanceData()
    end
    if data and data.tyrannus then
        local team = data.teamInInstance or ALLIANCE
        local entry = NPC_SYLVANAS_PART2
        if team == ALLIANCE then
            entry = NPC_JAINA_PART2
        end
        creature:SummonCreature(entry, EVENT_LEADER_X, EVENT_LEADER_Y, EVENT_LEADER_Z, 0.0, 7, 0)
    end
    if data and data.rimefang then
        data.rimefang:DoAction(ACTION_END_COMBAT)
    end
end

-- Rimefang EVENT_ICY_BLAST (:368-371) — SelectTarget(Random, 0) with
-- playerOnly=false default (UnitAI.h:170): the pool is the whole threat
-- list (pets/totems included) — alive-filtered pick over GetAITargets
-- (the bridged threat list; midnight precedent; C++ SelectTargetList
-- skips offline refs, Go SortedEntries returns dead entries too),
-- no Player filter.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :368-371 Rimefang EVENT_ICY_BLAST: DoCast(target, 69232) on a random
-- target, non-triggered; re-arm 15s unconditional.
local function icyBlastTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_ICY_BLAST)
    end
    schedule(guid, "icyblast", 15000, function() icyBlastTick(creature, guid) end)
end

-- C++ :361-366 Rimefang EVENT_MOVE_NEXT: if (_currentWaypoint >= 10
-- || _currentWaypoint == 0) _currentWaypoint = 1; MovePoint(0,
-- rimefangPos[_currentWaypoint]); ++_currentWaypoint; re-arm 2s
-- PHASE_COMBAT. Lua index = C++ index + 1 (RIMEFANG_POS[1] is C++
-- rimefangPos[0], never flown), so the flight sequence is C++ 1..9
-- wrapping to 1, like C++. The chain only exists once the DoAction
-- receiver starts it, and ev2/4/23 cancel it (rimefangCancelCombat).
local function rimefangMoveNext(creature, guid)
    local wp = rimefangWaypoint[guid] or 0
    if wp >= 10 or wp == 0 then
        wp = 1
    end
    local pos = RIMEFANG_POS[wp + 1]
    creature:MoveTo(0, pos[1], pos[2], pos[3])
    rimefangWaypoint[guid] = wp + 1
    schedule(guid, "movenext", 2000, function() rimefangMoveNext(creature, guid) end)
end

-- C++ boss_rimefangAI::DoAction (boss_scourgelord_tyrannus.cpp:328-340)
-- on the ev38 receiver (dispatch IS bridged; the ick DoAction
-- receiver precedent): ACTION_START_RIMEFANG sets PHASE_COMBAT +
-- DoZoneInCombat (fires ev1, where the 15s EVENT_ICY_BLAST seed
-- lives) + the EVENT_MOVE_NEXT 500ms seed (:335); ACTION_END_COMBAT
-- is the _EnterEvadeMode twin (creature:EnterEvadeMode,
-- lua_creature_events.go:5014). The receiver has no live sender yet
-- — its only callers are tyrannus's EVENT_COMBAT_START (:244, the
-- intro machine above) and JustDied (:204, above).
local function rimefangDoAction(event, creature, action)
    local guid = creature:GetGUID()
    if action == ACTION_START_RIMEFANG then
        creature:DoZoneInCombat()
        schedule(guid, "movenext", 500, function() rimefangMoveNext(creature, guid) end)
    elseif action == ACTION_END_COMBAT then
        creature:EnterEvadeMode()
    end
end

-- C++ :328-336 DoAction(ACTION_START_RIMEFANG): the :335 EVENT_ICY_BLAST
-- seed (15s) lands on Rimefang's own Eluna event 1 — DoZoneInCombat() (:333)
-- puts Rimefang in combat at the same moment.
local function rimefangEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_scourgelord_tyrannus.cpp:350: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    initState(guid)
    schedule(guid, "icyblast", 15000, function() icyBlastTick(creature, guid) end)
end

local function rimefangCancelCombat(event, creature)
    local guid = creature:GetGUID()
    initState(guid)
    rimefangWaypoint[guid] = nil -- C++ Initialize(): _currentWaypoint = 0 on Reset
    -- C++ boss_scourgelord_tyrannus.cpp:317-318: me->SetCanFly(true) then
    -- me->SetReactState(REACT_PASSIVE) (rimefang Reset: PASSIVE is the
    -- permanent baseline, no flip in C++).
    creature:SetCanFly(true)
    creature:SetReactState(0) -- REACT_PASSIVE
end

RegisterCreatureEvent(ENTRY_TYRANNUS, 1, tyrannusEnterCombat)
RegisterCreatureEvent(ENTRY_TYRANNUS, 2, tyrannusCancelCombat)
RegisterCreatureEvent(ENTRY_TYRANNUS, 3, tyrannusKilledUnit)
RegisterCreatureEvent(ENTRY_TYRANNUS, 4, tyrannusJustDied)
RegisterCreatureEvent(ENTRY_TYRANNUS, 23, tyrannusReset)
RegisterCreatureEvent(ENTRY_TYRANNUS, 38, tyrannusDoAction)

RegisterCreatureEvent(ENTRY_RIMEFANG, 1, rimefangEnterCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 2, rimefangCancelCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 4, rimefangCancelCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 23, rimefangCancelCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 38, rimefangDoAction)

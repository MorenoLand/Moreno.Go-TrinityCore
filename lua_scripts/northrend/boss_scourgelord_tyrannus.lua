-- Scourgelord Tyrannus + Rimefang (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_scourgelord_tyrannus.cpp
-- (571 lines incl. license; 6 scripts — boss_tyrannus (CreatureScript via
-- GetPitOfSaronAI (BossAI), DATA_TYRANNUS = 6); boss_rimefang (CreatureScript
-- via GetPitOfSaronAI (ScriptedAI), DATA_RIMEFANG = 7);
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
-- Tyrannus JustEngagedWith (:164) — Talk(SAY_AGGRO 8); the
--   EVENT_COMBAT_START seed legs (:250-252) land here because Eluna event 1
--   fires when DoZoneInCombat() (:249) puts Tyrannus in combat — the
--   faithful combat-start anchor: OVERLORD_BRAND 69172 {5000,7000}ms,
--   FORCEFUL_SMASH 69155 {14000,16000}ms, MARK_OF_RIMEFANG 69275
--   {25000,27000}ms.
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
-- Tyrannus JustDied (:193) — Talk(SAY_DEATH 10); timers cancelled (the
--   sjonnir JustDied-Talk precedent).
-- Rimefang DoAction(ACTION_START_RIMEFANG) (:328-336) — the EVENT_ICY_BLAST
--   seed (15s, :335) lands on Rimefang's own Eluna event 1 (engine aggro —
--   the faithful anchor for the unported DoAction relay): the :332 phase
--   flip and :333 DoZoneInCombat stay out behind the unbridged DoAction
--   relay + intro machine (DoZoneInCombat bridge live 2026-10-23, but the
--   relay has no bearer — rimefang->AI()->DoAction has no bridge).
-- Rimefang EVENT_ICY_BLAST (:368-371) — SelectTarget(Random, 0)
-- (playerOnly=false default, UnitAI.h:170) -> random threat-listed unit
-- (GetAITargets pick, no Player filter); DoCast(target, 69232);
-- re-arm 15s unconditional (the SelectTarget if only wraps the cast).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally and the :280-281
-- early-return gate is unmodeled (the maiden / devourer precedent).
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
-- Tyrannus intro machine — DoAction(ACTION_START_INTRO) (:207-217:
--   Talk(SAY_TYRANNUS_INTRO_1 6) + EVENT_INTRO_1..3 / EVENT_COMBAT_START
--   scheduling) rides the unbridged at_tyrannus_event_starter area trigger
--   (:541-560, trigger :555) — no DoAction bridge (the krick ACTION_OUTRO
--   precedent); the EVENT_INTRO_1 dead case (:232-234, commented-out
--   Talk(SAY_GORKUN_INTRO_2)), the EVENT_INTRO_2 Talk(SAY_TYRANNUS_INTRO_3
--   7) (:235-236), the EVENT_INTRO_3 ExitVehicle + MovePoint(miscPos)
--   (:238-240), and the EVENT_COMBAT_START DoZoneInCombat /
--   SPELL_FULL_HEAL 43979 / rimefang->AI()->DoAction(ACTION_START_RIMEFANG)
--   legs (:242-249) ride it out with the machine.
-- Tyrannus Reset / InitializeAI (:142-162) — REACT_PASSIVE +
--   UNIT_FLAG_NON_ATTACKABLE + instance SetBossState(DATA_TYRANNUS,
--   NOT_STARTED/DONE) + DespawnOrUnsummon — no flag / instance
--   bridges (the react bridge exists — creature:SetReactState — but
--   the intro DoAction/area-trigger machine is unported, so the
--   PASSIVE arm stays out: porting it alone would strand the boss
--   passive).
-- Tyrannus AttackStart NON_ATTACKABLE gate (:169-176), EnterEvadeMode
--   (:178-186: instance SetBossState(FAIL) + rimefang EnterEvadeMode +
--   DespawnOrUnsummon) — no bridges.
-- Tyrannus JustDied instance legs (:193-206) — SetBossState(DONE) +
--   TEMPSUMMON_DEAD_DESPAWN + rimefang->AI()->DoAction(ACTION_END_COMBAT)
--   — no instance / DoAction bridges.
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
--   SPELL_HOARFROST 69246)) — no AuraScript / SetGUID bridge; the 69275
--   cast IS ported, the hoarfrost follow-up is never fabricated (the
--   phase/machine gating rule).
-- Rimefang EVENT_MOVE_NEXT (:361-366) — waypoint patrol over rimefangPos
--   (MovePoint, waypoints 1..9 then wrap) — movement choreography, no
--   bridge.
-- Rimefang Reset (:306-320: waypoint/GUID init + SetCanFly / REACT_PASSIVE
--   / NON_ATTACKABLE), JustReachedHome InstallAllAccessories (:322-326,
--   vehicle), DoAction legs (:328-340: ACTION_START_RIMEFANG /
--   ACTION_END_COMBAT) — no movement/vehicle/flag bridges (the
--   REACT_PASSIVE and SetCanFly(true) arms are PORTED 2026-10-20/22 —
--   creature:SetReactState / creature:SetCanFly).
--   The :319 SetFlag(NON_ATTACKABLE) is never cleared live anywhere in
--   the rimefang AI (permanent vehicle-dragon property; flags re-affirm
--   to template on the creature sweep) — VACUOUS, not ported.
-- spell_tyrannus_rimefang_icy_blast SpellScript (:507-539: EFFECT_1
--   SPELL_EFFECT_TRIGGER_MISSILE -> SummonCreature(NPC_ICY_BLAST 36731) +
--   triggered SPELL_ICY_BLAST_AURA 69238) — no SpellScript / summon bridge.
-- at_tyrannus_event_starter (:541-560) — unbridged area-trigger queue.
-- Upstream C++ findings: EVENT_INTRO_1 (:232-234) is a dead case — its only
-- leg is a commented-out Talk(SAY_GORKUN_INTRO_2); the SAY_GORKUN_*
-- (:34-36), SAY_JAYNA_OUTRO_* (:53-55) and SAY_SYLVANAS_OUTRO_* (:58-59)
-- yells are never Talked in this TU (the outro Talk legs live in
-- instance_pit_of_saron.cpp); EVENT_HOARFROST re-schedules unconditionally
-- on every mark-aura apply with no duplicate guard (:346).

local ENTRY_TYRANNUS = 36658
local ENTRY_RIMEFANG = 36661

local SAY_AGGRO = 8
local SAY_SLAY = 9
local SAY_DEATH = 10
local SAY_MARK_RIMEFANG_1 = 11
local SAY_MARK_RIMEFANG_2 = 12
local SAY_DARK_MIGHT_1 = 13
local SAY_DARK_MIGHT_2 = 14

local SPELL_OVERLORD_BRAND = 69172
local SPELL_FORCEFUL_SMASH = 69155
local SPELL_UNHOLY_POWER = 69167
local SPELL_MARK_OF_RIMEFANG = 69275
local SPELL_ICY_BLAST = 69232

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

-- C++ SelectTarget(SelectTargetMethod::Random, 1, 0.0f, true) — the
-- offset-1 skip pops the first MaxThreat-ordered entry BEFORE the
-- player filter (UnitAI.h:214-280), then a random alive player from
-- the rest: threat-ordered GetAITargets[2..] (eck precedent; dead
-- targets are offline refs, Go SortedEntries returns them too, so
-- IsAlive filters). Fixed 2026-10-25 from the GetPlayersInWorld
-- pool, which could pick threat-less players and ignored the skip.
local function randomPlayer(creature)
    local targets = creature:GetAITargets()
    local candidates = {}
    for i = 2, #targets do
        local t = targets[i]
        if t:GetObjectType() == "Player" and t:IsAlive() then
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

-- C++ :164 JustEngagedWith: Talk(SAY_AGGRO); the :250-252 EVENT_COMBAT_START
-- seed legs land here — Eluna event 1 fires at the same combat-start moment
-- (DoZoneInCombat :249).
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

-- Shared timer wipe for events 2/23 (the bronjahm convention).
local function tyrannusCancelCombat(event, creature)
    initState(creature:GetGUID())
end

-- C++ :193 JustDied: Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent;
-- timers cancelled.
local function tyrannusJustDied(event, creature)
    initState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
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
    initState(creature:GetGUID())
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
RegisterCreatureEvent(ENTRY_TYRANNUS, 23, tyrannusCancelCombat)

RegisterCreatureEvent(ENTRY_RIMEFANG, 1, rimefangEnterCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 2, rimefangCancelCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 4, rimefangCancelCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 23, rimefangCancelCombat)

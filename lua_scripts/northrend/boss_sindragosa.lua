-- Sindragosa (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sindragosa.cpp
-- (1556 lines incl. license; 5 CreatureScripts
-- (boss_sindragosa (BossAI, DATA_SINDRAGOSA = 11), npc_ice_tomb
-- (ScriptedAI), npc_spinestalker (ScriptedAI), npc_rimefang_icc
-- (ScriptedAI), npc_sindragosa_trash (ScriptedAI, shared AI for
-- NPC_FROSTWARDEN_HANDLER / NPC_FROSTWING_WHELP)) +
-- 13 Sindragosa-specific SpellScript/AuraScript legs
-- (spell_sindragosa_s_fury, spell_sindragosa_unchained_magic,
-- spell_sindragosa_frost_breath, spell_sindragosa_instability,
-- spell_sindragosa_frost_beacon, spell_sindragosa_ice_tomb_trap,
-- spell_sindragosa_icy_grip, spell_sindragosa_mystic_buffet,
-- spell_rimefang_icy_blast, spell_frostwarden_handler_order_whelp,
-- spell_frostwarden_handler_focus_fire,
-- spell_frostwarden_handler_focus_fire_aura (AuraScript),
-- spell_sindragosa_ice_tomb_target) + 2 generic
-- RegisterSpellScriptWithArgs spell_trigger_spell_from_caster legs
-- + 1 AreaTriggerScript
-- (at_sindragosa_lair) + 1 AchievementCriteriaScript
-- (achievement_all_you_can_eat); all registered from inside
-- AddSC_boss_sindragosa(); loader decl 182 / call 377 per
-- northrend_script_loader.cpp — the TWELFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_valithria_dreamwalker() (call 376) — verified
-- from the loader this run; the checkpoint sequence
-- (valithria_dreamwalker -> sindragosa) is followed).
-- Entry: 36853 Sindragosa (icecrown_citadel.h NPC_SINDRAGOSA, line
-- 304; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SINDRAGOSA, line 300; also summoned via instance
-- SummonCreature(NPC_SINDRAGOSA, SindragosaSpawnPos), line 492) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the RegisterIcecrownCitadelCreatureAI
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sindragosa" hits boss_sindragosa.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_sindragosa JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent; the instance
-- CheckRequiredBosses gate / EVADE_REASON_SEQUENCE_BREAK leg /
-- DoCastSpellOnPlayers LIGHT_S_HAMMER_TELEPORT leg / setActive leg
-- have no bridge and stay out; DoCastSelf FROST_AURA+PERMAEATING_CHILL
-- (:277-278) + SetBossState(IN_PROGRESS) (:280) PORTED 2026-11-06,
-- in C++ arm order; the SetCombatPulseDelay(5) +
-- SetFarVisible(true) arms are PORTED
-- (creature:SetCombatPulseDelay bridge, lua_creature_events.go; event 1,
-- C++ arm order); the DoZoneInCombat tail (:284) is PORTED
-- (creature:DoZoneInCombat, live bridge
-- engine/world/lua_creature_events.go, 2026-10-23)).
-- boss_sindragosa KilledUnit — Talk(SAY_KILL 8) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_sindragosa JustDied — Talk(SAY_DEATH 10) (event 4 — the
-- sjonnir JustDied-Talk precedent; the DoCastAOE
-- FROST_INFUSION_CREDIT leg PORTED (Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)
-- (:262-264); creature:Is25ManRaid() bridged lua_creature_events.go,
-- the old "no 25-man difficulty bridge" claim retracted).
-- PORTED (tick 190, RegisterAuraEvent): spell_sindragosa_frost_beacon
-- PeriodicTick — PreventDefaultAction + caster casts SPELL_ICE_TOMB_DAMAGE
-- 70157 triggered on the aura target (:1223-1232).
-- DOCUMENTED-ONLY (in this header; only entry 36853 registered):
-- PORTED (2026-10-13): JustEngagedWith — DoCastSelf(
-- SPELL_FROST_AURA 70084) + DoCastSelf(SPELL_PERMAEATING_CHILL 70109)
-- non-triggered (:277-278, maiden engage-cast convention);
-- EVENT_BERSERK — single-fire 10min — Talk(EMOTE_BERSERK_RAID 11) +
-- Talk(SAY_BERSERK 9) + DoCastSelf(SPELL_BERSERK 26662) non-triggered
-- (:242/:458-460), bare timer across all phases.
-- PORTED (2026-10-13 retraction re-audit): Reset() —
-- DoCastSelf(SPELL_TANK_MARKER 71039, triggered) (:241);
-- DamageTaken 35% latch — Talk(SAY_PHASE_2 6) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET 70128, triggered) via a 1s
-- one-shot timer (the EVENT_THIRD_PHASE_CHECK leg :403-410 /
-- :548-553; CancelEvent(EVENT_AIR_PHASE) is moot — no air-phase
-- timers exist in this port; RescheduleEvent(EVENT_ICY_GRIP) rides
-- the unbridged icy-grip leg); EVENT_ICE_TOMB — CastSpell(nil,
-- SPELL_ICE_TOMB_TARGET 69712, triggered) init {7s,10s} / re-arm
-- {16s,23s} (:515-518/:551 — the air-phase retry loop is
-- vacuously satisfied with no air-phase machine; the SPELLVALUE_
-- MAX_TARGETS(1) spell-mod leg has no bridge, default cast
-- faithful).
-- boss_sindragosa UpdateAI — the land/air phase machine gates cleave /
-- tail-smash / frost-breath / icy-grip / unchained-magic /
-- frost-bomb legs (phase-machine rule — porting bare timers would
-- fabricate air-phase casts) — all unbridged.
-- SAY_RESPITE_FOR_A_TORMENTED_SOUL 4 is declared in the text enum
-- but never referenced by a Talk() call — dead text evidence.
-- boss_sindragosa DoAction — ACTION_START_FROSTWYRM intro machine (ev38
-- dispatch bridged; senders at_sindragosa_lair OnTrigger
-- (boss_sindragosa.cpp:1505) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:493) unported, receiver arm is the unported
-- flight machine); JustReachedHome SetBossState FAIL leg PORTED
-- 2026-11-06 (sindragosaReachHome, ev24).
-- spell_sindragosa_ice_tomb_target::HandleSindragosaTalk —
-- creatureCaster->AI()->Talk(EMOTE_WARN_FROZEN_ORB 7, GetHitUnit())
-- is cross-AI Talk (no cross-AI-Talk bridge).
-- npc_ice_tomb (36980) — zero Talk lines — NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent); its SetGUID /
-- DoAction ACTION_TRIGGER_ASPHYXIATION / GUID-bookkeeping legs ride
-- bridges that do not exist. npc_spinestalker (37534) /
-- npc_rimefang_icc (37533) / npc_sindragosa_trash handler (37531)
-- ARE registered — their pure timer arms are PORTED below (the
-- maiden CreateLuaEvent precedent); their intro DoAction flight machines and the trash JustAppeared / SetData
-- instance legs stay out (ev38 dispatch bridged; senders at_sindragosa_lair
-- (boss_sindragosa.cpp:1492,1496) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:469,476) unported / no-motion / no-instance
-- bridges). The
-- frostwing whelp (37532) carries no timer arms — NOT registered.
-- at_sindragosa_lair joins the no-AreaTrigger bridge queue (the
-- boss_anubrekhan at_anubrekhan_entrance precedent);
-- achievement_all_you_can_eat joins the
-- no-achievement-criteria-bridge queue (the lana'thel achievement
-- precedent); the remaining 12 Sindragosa-specific SpellScript /
-- AuraScript legs + the 2 generic spell_trigger_spell_from_caster
-- with-args legs join the no-SpellScript-bridge /
-- no-AuraScript-bridge queues (the boss_moragg optic-link
-- precedent).
-- All 6 Talk() calls in the file accounted for (6 =
-- SAY_AGGRO 0 (1) + SAY_DEATH 10 (1) + SAY_KILL 8 (1) +
-- EMOTE_BERSERK_RAID 11 (1, scheduler) + SAY_BERSERK 9 (1,
-- scheduler) + SAY_PHASE_2 6 (1, scheduler)).
-- The phase-machine Talk legs (SAY_UNCHAINED_MAGIC 1,
-- EMOTE_WARN_BLISTERING_COLD 2, SAY_BLISTERING_COLD 3,
-- SAY_AIR_PHASE 5) were never ported to code — porting bare
-- timers would fabricate air-phase casts (phase-machine rule
-- above).

-- CAST-STATE GATES PORTED (2026-11-04): boss :450/:563 gates berserk /
-- ice-tomb / third-phase-check; spinestalker :730 (pre-loop only) gates
-- all 3 ticks; rimefang :862 (pre-loop only) gates frost-breath;
-- trash :986 (pre-loop only) gates both handler ticks; ice-tomb :618
-- UpdateAI has no gate (existence check only, NOT registered); engage /
-- DamageTaken / JustDied / JustSummoned legs ungated per C++ (separate
-- hooks / BasicEvent, not the scheduler).
local ENTRY_SINDRAGOSA = 36853
local ENTRY_SPINESTALKER = 37534 -- icecrown_citadel.h:305
local ENTRY_RIMEFANG = 37533 -- icecrown_citadel.h:306
local ENTRY_FROSTWARDEN_HANDLER = 37531 -- icecrown_citadel.h:307
local ENTRY_FROST_BOMB = 37186 -- icecrown_citadel.h:310

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 10
local SAY_BERSERK = 9
local EMOTE_BERSERK_RAID = 11
local SAY_PHASE_2 = 6

local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SPELL_TANK_MARKER = 71039
local SPELL_MYSTIC_BUFFET = 70128
local SPELL_FROST_BOMB_VISUAL = 70022 -- boss_sindragosa.cpp:74
local SPELL_BIRTH_NO_VISUAL = 40031 -- boss_sindragosa.cpp:75
local SPELL_ICE_TOMB_TARGET = 69712
local SPELL_ICE_TOMB_UNTARGETABLE = 69700
local SPELL_FROST_BEACON = 70126 -- boss_sindragosa.cpp:67
local SPELL_ICE_TOMB_DAMAGE = 70157 -- boss_sindragosa.cpp:71
local EMOTE_WARN_FROZEN_ORB = 7

local sindragosaThirdPhase = {}

local SPELL_FROST_AURA = 70084
local SPELL_PERMAEATING_CHILL = 70109
local SPELL_SHADOWS_FATE = 71169
local SPELL_FROST_INFUSION_CREDIT = 72289
local SPELL_BERSERK = 26662
local SPELL_BELLOWING_ROAR = 36922 -- :80
local SPELL_CLEAVE_SPINESTALKER = 40505 -- :81
local SPELL_TAIL_SWEEP = 71370 -- :82
local SPELL_FROST_BREATH_RIMEFANG = 71386 -- :85
local SPELL_FROST_AURA_RIMEFANG = 71387 -- :86
local SPELL_ORDER_WHELP = 71357 -- :92
local SPELL_CONCUSSIVE_SHOCK = 71337 -- :93

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

-- C++ EVENT_BERSERK: 10min single-fire — Talk(EMOTE_BERSERK_RAID) +
-- Talk(SAY_BERSERK) + DoCastSelf(SPELL_BERSERK) non-triggered
-- (:242/:458-460), bare timer across all phases.
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563): 500ms IsCasting defer, cast not consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_BERSERK_RAID)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ EVENT_ICE_TOMB: CastSpell(nullptr, SPELL_ICE_TOMB_TARGET,
-- TRIGGERED_FULL_MASK) with SPELLVALUE_MAX_TARGETS=1 (:515-518) — the
-- nullptr runs the spell's implicit random-enemy selection, so the tomb
-- lands on a random raid member, not the victim. The Go cast path binds an
-- explicit target GUID only, so nil would always tomb the tank;
-- randomPlayerTarget restores the C++ pick (tank stays in the pool, per the
-- implicit targeting). Re-arm {16s,23s}.
local function randomPlayerTarget(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and not plr:HasAura(SPELL_ICE_TOMB_UNTARGETABLE) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onIceTomb(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563); defer precedes the target pick: 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "ice_tomb", 500, function() onIceTomb(creature, guid) end)
        return
    end
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_ICE_TOMB_TARGET, true)
        -- spell_sindragosa_ice_tomb_target HandleSindragosaTalk: third-phase Talk to the hit unit.
        if sindragosaThirdPhase[guid] then
            creature:Talk(EMOTE_WARN_FROZEN_ORB, target)
        end
    end
    schedule(guid, "ice_tomb", {16000, 23000}, function() onIceTomb(creature, guid) end)
end

-- C++ EVENT_THIRD_PHASE_CHECK: Talk(SAY_PHASE_2) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET, true); seeds EVENT_ICE_TOMB
-- {7s,10s} — the _isInAirPhase leg is vacuously true (no air-phase
-- machine in this port).
local function onThirdPhaseCheck(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :450 (+ in-loop :563): 500ms IsCasting defer, Talk/cast/seed not consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "third_phase_check", 500, function() onThirdPhaseCheck(creature, guid) end)
        return
    end
    creature:Talk(SAY_PHASE_2)
    creature:CastSpell(creature, SPELL_MYSTIC_BUFFET, true)
    schedule(guid, "ice_tomb", {7000, 10000}, function() onIceTomb(creature, guid) end)
end

-- C++ boss_sindragosa::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:269-275) has no bridge.
local function sindragosaJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ :277-278 — DoCastSelf(SPELL_FROST_AURA) +
    -- DoCastSelf(SPELL_PERMAEATING_CHILL), non-triggered.
    creature:CastSpell(creature, SPELL_FROST_AURA)
    creature:CastSpell(creature, SPELL_PERMAEATING_CHILL)
    creature:Talk(SAY_AGGRO)
    -- C++ :280 — instance->SetBossState(DATA_SINDRAGOSA, IN_PROGRESS),
    -- after Talk, before SetCombatPulseDelay (C++ arm order). PORTED
    -- 2026-11-06 via creature:SetBossState (the stale "no bridge"
    -- header claim is retracted — lua_creature_events.go:5917).
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- boss_sindragosa.cpp:281 (JustEngagedWith arm)
    creature:SetFarVisible(true) -- boss_sindragosa.cpp:283 (the setActive leg still has no bridge)
    creature:DoZoneInCombat() -- boss_sindragosa.cpp:284 (JustEngagedWith tail; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
end

-- C++ Reset() _Reset() timer machinery — cancel/re-arm on combat
-- events (the maiden convention). C++ EnterEvadeMode override
-- (:287-292) suppresses the evade when _isInAirPhase +
-- EVADE_REASON_BOUNDARY — no air-phase flag exists in this port (the
-- phase machine is unported), so the suppression stays out; the base
-- evade path rides ev24 FAIL (ported above).
local function sindragosaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function sindragosaReset(event, creature)
    -- C++ Reset() — BossAI::Reset() runs _Reset() first: SetBossState
    -- NOT_STARTED only when the stored state != DONE (the rotface
    -- precedent — ev23 fires on spawn/death/evade too, and a DONE boss
    -- must stay DONE). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_SINDRAGOSA) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUID())
    sindragosaThirdPhase[creature:GetGUID()] = nil
    -- C++ Reset(): DoCastSelf(SPELL_TANK_MARKER, true) (:241),
    -- unconditional one-shot. The engine fires event 23 on death too
    -- (Eluna reset-then-died order, lua_creature_events.go) but the C++ AI
    -- Reset() never runs on death, so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TANK_MARKER, true)
        -- Fresh-spawn analog (stalagg/feugen precedent): C++ respawns a new
        -- creature at the template REACT_AGGRESSIVE, but the Go motion keeps
        -- the ev24 DEFENSIVE write below across respawns. C++ Reset() itself
        -- leaves react state untouched; on the evade path event 24 re-applies
        -- DEFENSIVE after this, so the transient here is harmless.
        creature:SetReactState(2)
    end
end

-- C++ boss_sindragosa::JustReachedHome (:294-300): BossAI::JustReachedHome()
-- is instance bookkeeping (no bridge), instance->SetBossState(FAIL) PORTED
-- 2026-11-06 via creature:SetBossState (ev24, marrowgar precedent);
-- SetCanFly(false) (:298) / SetDisableGravity(false) (:299) /
-- SetReactState(REACT_DEFENSIVE 1) (:300) ported in C++ arm order
-- (bridges live, lua_creature_events.go). The old "no bridges" note was
-- stale. Event 24 fires at the home-move completion (C++
-- HomeMovementGenerator::DoFinalize twin).
local function sindragosaReachHome(event, creature)
    -- C++ :297 — instance->SetBossState(DATA_SINDRAGOSA, FAIL), after
    -- BossAI::JustReachedHome() (no state) and before the fly/gravity/
    -- react arms (C++ order). PORTED 2026-11-06 via creature:SetBossState
    -- on ev24 (the marrowgar ev24 precedent; the stale "no event-24
    -- port precedent" header claim is retracted).
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_FAIL)
    creature:SetCanFly(false)
    creature:SetDisableGravity(false)
    creature:SetReactState(1)
end

-- C++ boss_sindragosa::DamageTaken: !_isThirdPhase && !HealthAbovePct(35)
-- — single-flip latch (gothik precedent); schedules
-- EVENT_THIRD_PHASE_CHECK 1s (:403-410).
local function sindragosaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not sindragosaThirdPhase[guid] and creature:GetHealthPct() <= 35 then
        sindragosaThirdPhase[guid] = true
        schedule(guid, "third_phase_check", 1000, function() onThirdPhaseCheck(creature, guid) end)
    end
    return true, damage
end

-- C++ boss_sindragosa::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function sindragosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_sindragosa::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
-- (the me->RemoveAllGameObjects() leg has no bridge — the garfrost
-- JustDied precedent; the _trappedPlayerGUID asphyxiation/ice-tomb
-- aura cleanup is cross-player GUID blocked — no bridge).
local function sindragosaJustDied(event, creature, killer)
    -- C++ _JustDied() resets the EventMap (boss_sindragosa.cpp:260):
    -- cancel the CreateLuaEvent timers — they have no death
    -- auto-cancel, so the self-re-arming ice-tomb / third-phase-check
    -- legs would keep firing on the corpse (festergut/rotface
    -- death-cancel precedent); drop the per-GUID third-phase latch
    -- like Reset does.
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sindragosaThirdPhase[guid] = nil
    -- C++ :260 — _JustDied() first: SetBossState DONE, before Talk
    -- (:261, C++ order). PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_SINDRAGOSA, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ :262-264 — Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)-gated
    -- DoCastAOE(SPELL_FROST_INFUSION_CREDIT, true) via self-cast
    -- (DoCastAOE = self-cast convention; C++ order kept).
    if creature:Is25ManRaid() and creature:HasAura(SPELL_SHADOWS_FATE) then
        creature:CastSpell(creature, SPELL_FROST_INFUSION_CREDIT, true)
    end
end

-- PORTED (2026-10-14): npc_spinestalker (37534) — Reset timer seeds
-- re-keyed to OnEnterCombat (the deathwhisper fanatic precedent):
-- EVENT_BELLOWING_ROAR DoCastSelf(SPELL_BELLOWING_ROAR) non-triggered
-- {20s,25s} -> {25s,30s} (:661/:737-739); EVENT_CLEAVE_SPINESTALKER
-- DoCastVictim(SPELL_CLEAVE_SPINESTALKER) non-triggered {10s,15s} ->
-- {10s,15s} (:662/:741-743); EVENT_TAIL_SWEEP
-- DoCastSelf(SPELL_TAIL_SWEEP) non-triggered {8s,12s} -> {22s,25s}
-- (:663/:745-747). Pure timer arms — no phase, random-target, or
-- summon gating (the maiden CreateLuaEvent precedent); the UpdateAI
-- CASTING gates are PORTED (500ms IsCasting defer, mal_ganis idiom
-- — IsCasting live, lua_creature_events.go:1950; the old "no bridge /
-- timers fire unconditionally" claim is retracted). The intro ACTION_START_FROSTWYRM flight machine stays out (ev38 dispatch
-- bridged; senders at_sindragosa_lair (boss_sindragosa.cpp:1492) + instance
-- OnCreatureRemove (instance_icecrown_citadel.cpp:469) unported;
-- MovePoint/MovementInform unbridged — SetDefaultMovementType IS
-- bridged (lua_objects.go:633, pure field write) but carries no motion).
local function onSpinestalkerRoar(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "roar", 500, function() onSpinestalkerRoar(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "roar", {25000, 30000}, function() onSpinestalkerRoar(creature, guid) end)
end

local function onSpinestalkerCleave(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onSpinestalkerCleave(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE_SPINESTALKER)
    schedule(guid, "cleave", {10000, 15000}, function() onSpinestalkerCleave(creature, guid) end)
end

local function onSpinestalkerTailSweep(creature, guid)
    -- C++ spinestalker UpdateAI pre-loop gate :730 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "tailsweep", 500, function() onSpinestalkerTailSweep(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tailsweep", {22000, 25000}, function() onSpinestalkerTailSweep(creature, guid) end)
end

local function spinestalkerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "roar", {20000, 25000}, function() onSpinestalkerRoar(creature, guid) end)
    schedule(guid, "cleave", {10000, 15000}, function() onSpinestalkerCleave(creature, guid) end)
    schedule(guid, "tailsweep", {8000, 12000}, function() onSpinestalkerTailSweep(creature, guid) end)
end

-- PORTED (2026-10-14): npc_rimefang_icc (37533) — JustEngagedWith
-- DoCastSelf(SPELL_FROST_AURA_RIMEFANG, true) (:850-853) +
-- EVENT_FROST_BREATH_RIMEFANG DoCastSelf(SPELL_FROST_BREATH)
-- non-triggered {12s,15s} -> {35s,40s} (:791/:868-871). The
-- EVENT_ICY_BLAST flight machine stays out (no-motion bridge); the intro flight machine stays out (ev38 dispatch bridged; senders
-- at_sindragosa_lair (boss_sindragosa.cpp:1496) + instance OnCreatureRemove
-- (instance_icecrown_citadel.cpp:476) unported).
local function onRimefangFrostBreath(creature, guid)
    -- C++ rimefang UpdateAI pre-loop gate :862 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "frostbreath", 500, function() onRimefangFrostBreath(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FROST_BREATH_RIMEFANG)
    schedule(guid, "frostbreath", {35000, 40000}, function() onRimefangFrostBreath(creature, guid) end)
end

local function rimefangEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_FROST_AURA_RIMEFANG, true)
    schedule(guid, "frostbreath", {12000, 15000}, function() onRimefangFrostBreath(creature, guid) end)
end

-- PORTED (2026-10-14): npc_sindragosa_trash (37531 handler only) —
-- Reset timer seeds re-keyed to OnEnterCombat:
-- EVENT_FROSTWARDEN_ORDER_WHELP DoCastSelf(SPELL_ORDER_WHELP)
-- non-triggered 3s -> 3s (:943/:994-996);
-- EVENT_CONCUSSIVE_SHOCK DoCastSelf(SPELL_CONCUSSIVE_SHOCK)
-- non-triggered {8s,10s} -> {10s,13s} (:944/:998-1000). The whelp
-- (37532) carries no timer arms — NOT registered. The
-- JustAppeared / SetData instance legs stay out (no instance
-- bridge).
local function onHandlerOrderWhelp(creature, guid)
    -- C++ trash UpdateAI pre-loop gate :986 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "orderwhelp", 500, function() onHandlerOrderWhelp(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_ORDER_WHELP)
    schedule(guid, "orderwhelp", 3000, function() onHandlerOrderWhelp(creature, guid) end)
end

local function onHandlerConcussiveShock(creature, guid)
    -- C++ trash UpdateAI pre-loop gate :986 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "concussive", 500, function() onHandlerConcussiveShock(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_CONCUSSIVE_SHOCK)
    schedule(guid, "concussive", {10000, 13000}, function() onHandlerConcussiveShock(creature, guid) end)
end

local function handlerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "orderwhelp", 3000, function() onHandlerOrderWhelp(creature, guid) end)
    schedule(guid, "concussive", {8000, 10000}, function() onHandlerConcussiveShock(creature, guid) end)
end

local function addLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function addReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustSummoned NPC_FROST_BOMB (boss_sindragosa.cpp:413-421):
-- CastSpell(summon, SPELL_FROST_BOMB_VISUAL 70022, triggered) +
-- CastSpell(summon, SPELL_BIRTH_NO_VISUAL 40031, triggered) in C++ order;
-- the 5500ms FrostBombExplosion BasicEvent's SPELL_FROST_BOMB 69845 cast
-- stays out (no exact Lua form); its visual-removal leg ports below via
-- a 5500ms CreateLuaEvent. LATENT: frost bombs spawn only from the
-- unported air-phase summon machine (anubrekhan latent precedent).
local function sindragosaJustSummoned(_, creature, summoned)
    if summoned:GetEntry() == ENTRY_FROST_BOMB then
        summoned:CastSpell(summoned, SPELL_FROST_BOMB_VISUAL, true)
        summoned:CastSpell(summoned, SPELL_BIRTH_NO_VISUAL, true)
        -- C++ FrostBombExplosion BasicEvent (boss_sindragosa.cpp:192-202,
        -- added at 5500ms): visual strip. Raw per-summon CreateLuaEvent, not
        -- the file's schedule() helper — its single per-GUID key would
        -- collapse concurrent bombs into one timer. The paired
        -- SPELL_FROST_BOMB 69845 cast stays out (no exact Lua form).
        CreateLuaEvent(function()
            summoned:RemoveAura(SPELL_FROST_BOMB_VISUAL)
        end, 5500)
    end
end

RegisterCreatureEvent(ENTRY_SINDRAGOSA, 1, sindragosaJustEngagedWith)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 2, sindragosaLeaveCombat)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 3, sindragosaKilledUnit)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 4, sindragosaJustDied)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 9, sindragosaDamageTaken)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 23, sindragosaReset)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 24, sindragosaReachHome)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 19, sindragosaJustSummoned)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 1, spinestalkerEnterCombat)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_SPINESTALKER, 4, addLeaveCombat) -- C++ boss_sindragosa.cpp:678-681: JustDied _events.Reset() only
RegisterCreatureEvent(ENTRY_SPINESTALKER, 23, addReset)
RegisterCreatureEvent(ENTRY_RIMEFANG, 1, rimefangEnterCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_RIMEFANG, 4, addLeaveCombat) -- C++ boss_sindragosa.cpp:805-808: JustDied _events.Reset() only
RegisterCreatureEvent(ENTRY_RIMEFANG, 23, addReset)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 1, handlerEnterCombat)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 2, addLeaveCombat)
RegisterCreatureEvent(ENTRY_FROSTWARDEN_HANDLER, 23, addReset)

-- C++ spell_sindragosa_frost_beacon::PeriodicTick
-- (boss_sindragosa.cpp:1228-1232): PreventDefaultAction; the aura
-- caster casts SPELL_ICE_TOMB_DAMAGE (70157) triggered on the aura
-- target each tick. Returns true, the PreventDefaultAction twin, so
-- the Go default trigger cast does not also fire.
local function frostBeaconTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, SPELL_ICE_TOMB_DAMAGE, true)
    end
    return true
end

RegisterAuraEvent(SPELL_FROST_BEACON, 1, frostBeaconTick)

-- Krick and Ick (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_krickandick.cpp
-- (671 lines incl. license; 8 scripts — boss_ick (CreatureScript via
-- GetPitOfSaronAI (BossAI), DATA_ICK = 1); boss_krick (CreatureScript via
-- GetPitOfSaronAI (ScriptedAI), DATA_KRICK = 4); spell_krick_explosive_barrage
-- (AuraScript); spell_ick_explosive_barrage (AuraScript);
-- spell_exploding_orb_hasty_grow (AuraScript); spell_krick_pursuit +
-- spell_krick_pursuit_AuraScript (SpellScript + AuraScript pair);
-- spell_krick_pursuit_confusion (AuraScript); all registered from inside
-- AddSC_boss_ick() — which registers boss_ick BEFORE boss_krick (:663-664);
-- loader decl 163 / call 358 per northrend_script_loader.cpp — the FOURTH
-- group of the "// Pit of Saron" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_garfrost() (call 357); the call after it is
-- AddSC_boss_tyrannus() (call 359).
-- Entries: 36476 Ick (pit_of_saron.h NPC_ICK, line 51) and 36477 Krick
-- (pit_of_saron.h NPC_KRICK, line 50; instance_pit_of_saron.cpp
-- OnCreatureCreate binds case NPC_KRICK) — both entry-verifiable, both
-- registered (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName bindings are DB-side as usual.
-- DATA_ICK = 1 (pit_of_saron.h line 33); DATA_KRICK = 4 (line 38).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ick"
-- hits boss_krickandick.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
--
-- RETRACTION (2026-10-13): the Oct-2 port of this file was written under
-- the stale no-timer-bridge doctrine and carried only boss_krick's KilledUnit
-- Talk; the whole boss_ick UpdateAI EventMap was filed as no-timer-bridge.
-- That doctrine is disproved — CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go:798; the maiden_of_virtue / moroes / curator /
-- nightbane / bronjahm / devourer_of_souls convention) — so boss_ick's
-- ungated non-triggered cast arms port now, and 36476 Ick IS registered
-- (Go: RegisterLuaBoss("boss_ick", 36476), in loader order before
-- RegisterLuaBoss("boss_krick", 36477), mirroring AddSC_boss_ick's
-- boss_ick-then-boss_krick registration order).
--
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent ({min,max} table =
-- randtime range); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- CastSpell(nil, spellId) = victim-cast (DoCastVictim — the bridge falls
-- back to TargetGUID); CastSpell(creature, spellId) = self-cast (DoCastAOE =
-- DoCast(nullptr, spell) self-cast and DoCastSelf = DoCast(me) per the
-- in-tree convention); 2-arg = non-triggered — none of the ported arms is a
-- TRIGGERED cast (all DoCast* calls are 1-2 arg; the one triggered=true
-- call, krick->CastSpell(krick, 69012, true) :263, is cross-creature and
-- stays out).
--
-- PORTED ARMS (C++ line refs; all C++-exact for the modeled arms):
-- boss_ick JustEngagedWith (:163-173) — seeds EVENT_MIGHTY_KICK 20s (:170)
--   and EVENT_SPECIAL {30s,35s} (:173). The krick->AI()->Talk(SAY_KRICK_AGGRO 0)
--   leg (:167-168) is instance-driven Talk on another creature's AI (no
--   instance bridge); the EVENT_TOXIC_WASTE 5s / EVENT_SHADOW_BOLT 10s seeds
--   (:171-172) belong to krick-cast arms with no cross-creature bridge —
--   documented-only below.
-- EVENT_MIGHTY_KICK (:249-252) — DoCastVictim(SPELL_MIGHTY_KICK 69021),
--   non-triggered victim-cast; re-arm 25s (:251).
-- EVENT_SPECIAL (:253-257) — RAND(EVENT_EXPLOSIVE_BARRAGE, EVENT_POISON_NOVA,
--   EVENT_PURSUIT) scheduled at 1s (:255; uniform math.random(3) keeping the
--   C++ argument order); re-arm {23s,28s} (:256).
-- EVENT_EXPLOSIVE_BARRAGE (:258-267) — DoCastAOE(SPELL_EXPLOSIVE_BARRAGE_ICK
--   69263) = non-triggered self-cast (:264); one-shot (no re-arm; fires only
--   via EVENT_SPECIAL picks). The krick->AI()->Talk(SAY_KRICK_BARRAGE_1 2 /
--   SAY_KRICK_BARRAGE_2 3) (:261-262) and krick->CastSpell(krick,
--   SPELL_EXPLOSIVE_BARRAGE_KRICK 69012, true) (:263) legs are cross-creature
--   with no bridge.
-- EVENT_POISON_NOVA (:268-274) — Talk(SAY_ICK_POISON_NOVA 0) (:272, Ick's own
--   Talk) + DoCastAOE(SPELL_POISON_NOVA 68989) = non-triggered self-cast
--   (:273); one-shot. The krick->AI()->Talk(SAY_KRICK_POISON_NOVA 4) leg
--   (:269-270) is cross-creature with no bridge.
-- EVENT_PURSUIT (:275-279) — DoCastSelf(SPELL_PURSUIT 68987,
--   { SPELLVALUE_MAX_TARGETS, 1 }) = non-triggered self-cast (:278); one-shot.
--   The krick->AI()->Talk(SAY_KRICK_CHASE 5) leg (:276-277) is cross-creature
--   with no bridge.
-- boss_ick JustDied / LeaveCombat / Reset (:176-189, :147-161) — timer
--   cancellation only; the vehicle RemoveAllPassengers, ForceCombatStop(krick)
--   and krick->AI()->DoAction(ACTION_OUTRO) legs (:180-187) have no bridge.
-- boss_krick KilledUnit (:324-330) — Talk(SAY_KRICK_SLAY 1) player-gated
--   (event 3 — the razuvious player-gated variant precedent).
--
-- DOCUMENTED-ONLY (no bridge):
-- EVENT_TOXIC_WASTE (:233-240) — krick->CastSpell(target, SPELL_TOXIC_WASTE
--   69024) on a SelectTarget(Random, 0, playerOnly) target (:236-237): the
--   caster is Krick and there is no cross-creature bridge.
-- EVENT_SHADOW_BOLT (:241-248) — krick->CastSpell(target, SPELL_SHADOW_BOLT
--   69028) on a SelectTarget(Random, 1, playerOnly) target (:244-245): same,
--   no cross-creature bridge.
-- boss_ick DoAction (:191-217) — ACTION_STORE_OLD_TARGET /
--   ACTION_RESET_THREAT threat store/reset machine: no DoAction /
--   threat-manager bridge.
-- boss_krick JustSummoned (:332-340) — _summons.Summon plus the
--   NPC_EXPLODING_ORB triggered self-casts (SPELL_EXPLODING_ORB 69017,
--   SPELL_AUTO_GROW 69020): no summon-list bridge.
-- boss_krick Reset (:315-322) — SetReactState(REACT_PASSIVE) +
--   SetFlag(UNIT_FLAG_NON_ATTACKABLE): no bridge (the intro-outro machine
--   rides it).
-- boss_krick DoAction(ACTION_OUTRO) (:342-367) — tyrannus NearTeleportTo /
--   SummonCreature choreography + MovePoint(POINT_KRICK_INTRO): no instance /
--   motion bridges.
-- boss_krick MovementInform (:369-377) — POINT_KRICK_INTRO landing ->
--   Talk(SAY_KRICK_OUTRO_1 6) + PHASE_OUTRO flip: no-MovementInform bridge
--   (the violet_hold saboteur precedent).
-- boss_krick UpdateAI outro machine (:379-514) — EVENT_OUTRO_1..13 /
--   EVENT_OUTRO_END: the timer-driven Talk(SAY_KRICK_OUTRO_3 7 /
--   SAY_KRICK_OUTRO_5 8 / SAY_KRICK_OUTRO_8 9) legs ride the unbridged
--   PHASE_OUTRO machine (MovementInform-gated, so the phase/machine gating
--   rule keeps them out); the jainaOrSylvanas->AI()->Talk(SAY_JAYNA_OUTRO_2/4/10)
--   / Talk(SAY_SYLVANAS_OUTRO_2/4/10) and
--   tyrannus->AI()->Talk(SAY_TYRANNUS_OUTRO_7/9) legs are instance-driven
--   Talk on other creatures' AI; the SummonCreature / MovePoint /
--   MOVEMENTFLAG_FLYING / SPELL_STRANGULATING / SPELL_KRICK_KILL_CREDIT /
--   SetStandState(dead) legs have no bridges.
-- The six SpellScript / AuraScript hooks (spell_krick_explosive_barrage,
--   spell_ick_explosive_barrage, spell_exploding_orb_hasty_grow,
--   spell_krick_pursuit + spell_krick_pursuit_AuraScript,
--   spell_krick_pursuit_confusion) join the no-SpellScript / no-AuraScript
--   bridge queues; ick->AI()->Talk(SAY_ICK_CHASE_1 1, target) inside
--   spell_krick_pursuit is spell-driven Talk with no bridge by construction.
--
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts are
--   packet-visual), so the :227 casting gate is unmodeled and timers fire
--   unconditionally (the maiden precedent); the `return` statements after
--   EVENT_MIGHTY_KICK (:252) and EVENT_SHADOW_BOLT (:248) that skip the rest
--   of the event loop that tick are unmodeled (independent Lua timers);
--   events.DelayEvents(20s) on EVENT_EXPLOSIVE_BARRAGE (:266) is modeled
--   via delayTimers (thekal precedent) — the other arms shift 20s;
--   EVENT_PURSUIT's { SPELLVALUE_MAX_TARGETS, 1 } custom leg
--   (:278) has no bridge, so pursuit is not limited to 1 target (the devourer
--   fear precedent); the pursuit threat/fixate machine (spell_krick_pursuit
--   SpellScript + AuraScript, DoAction store/reset legs) is unmodeled, so the
--   SPELL_PURSUIT cast produces no chase behavior — a fidelity gap, not a
--   fabricated cast; the EVENT_EXPLOSIVE_BARRAGE krick-presence gate (:259)
--   cannot be modeled (no cross-creature bridge) — Krick is always present
--   during combat (he only leaves in the post-death outro), so the gate is a
--   non-issue in practice.

local ENTRY_ICK = 36476
local ENTRY_KRICK = 36477

local SAY_KRICK_SLAY = 1
local SAY_ICK_POISON_NOVA = 0

local SPELL_MIGHTY_KICK = 69021
local SPELL_EXPLOSIVE_BARRAGE_ICK = 69263
local SPELL_POISON_NOVA = 68989
local SPELL_PURSUIT = 68987

local timers = {}

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

-- arlokk-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

local function initState(guid)
    cancelTimers(guid)
end

-- C++ :249-252 EVENT_MIGHTY_KICK: DoCastVictim(SPELL_MIGHTY_KICK),
-- non-triggered victim-cast; re-arm 25s (:251).
local function onMightyKick(creature, guid)
    creature:CastSpell(nil, SPELL_MIGHTY_KICK)
    schedule(guid, "mightykick", 25000, function() onMightyKick(creature, guid) end)
end

-- C++ :258-267 EVENT_EXPLOSIVE_BARRAGE: DoCastAOE(SPELL_EXPLOSIVE_BARRAGE_ICK)
-- = non-triggered self-cast (:264); one-shot — fires only via EVENT_SPECIAL
-- picks, no re-arm. The krick-presence gate (:259) and the krick Talk /
-- triggered self-cast legs (:261-263) have no cross-creature bridge (see
-- header). C++ :266 events.DelayEvents(20s) pushes every other armed timer
-- out 20s (delayTimers, thekal precedent).
local function onExplosiveBarrage(creature, guid)
    creature:CastSpell(creature, SPELL_EXPLOSIVE_BARRAGE_ICK)
    delayTimers(guid, 20000)
end

-- C++ :268-274 EVENT_POISON_NOVA: Talk(SAY_ICK_POISON_NOVA) (Ick's own Talk)
-- + DoCastAOE(SPELL_POISON_NOVA) = non-triggered self-cast; one-shot. The
-- krick->AI()->Talk(SAY_KRICK_POISON_NOVA) leg (:269-270) has no bridge.
local function onPoisonNova(creature, guid)
    creature:Talk(SAY_ICK_POISON_NOVA)
    creature:CastSpell(creature, SPELL_POISON_NOVA)
end

-- C++ :275-279 EVENT_PURSUIT: DoCastSelf(SPELL_PURSUIT,
-- { SPELLVALUE_MAX_TARGETS, 1 }) = non-triggered self-cast; one-shot. The
-- custom max-targets leg and the pursuit threat/fixate machine have no
-- bridge (see header); the krick->AI()->Talk(SAY_KRICK_CHASE) leg (:276-277)
-- has no bridge.
local function onPursuit(creature, guid)
    creature:CastSpell(creature, SPELL_PURSUIT)
end

-- C++ :253-257 EVENT_SPECIAL: RAND(EVENT_EXPLOSIVE_BARRAGE, EVENT_POISON_NOVA,
-- EVENT_PURSUIT) scheduled at 1s (:255; uniform pick keeping the C++ argument
-- order); re-arm {23s,28s} (:256).
local function onSpecial(creature, guid)
    local roll = math.random(3)
    if roll == 1 then
        schedule(guid, "specialpick", 1000, function() onExplosiveBarrage(creature, guid) end)
    elseif roll == 2 then
        schedule(guid, "specialpick", 1000, function() onPoisonNova(creature, guid) end)
    else
        schedule(guid, "specialpick", 1000, function() onPursuit(creature, guid) end)
    end
    schedule(guid, "special", {23000, 28000}, function() onSpecial(creature, guid) end)
end

-- C++ boss_ickAI::JustEngagedWith (:163-173): seeds EVENT_MIGHTY_KICK 20s
-- (:170) and EVENT_SPECIAL {30s,35s} (:173). The BossAI::JustEngagedWith
-- passthrough and the krick->AI()->Talk(SAY_KRICK_AGGRO) leg (:167-168) have
-- no bridge; the EVENT_TOXIC_WASTE 5s / EVENT_SHADOW_BOLT 10s seeds (:171-172)
-- belong to krick-cast arms with no cross-creature bridge.
local function ickEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    schedule(guid, "mightykick", 20000, function() onMightyKick(creature, guid) end)
    schedule(guid, "special", {30000, 35000}, function() onSpecial(creature, guid) end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron convention).
-- C++ JustDied (:176-189): _JustDied() passthrough, the vehicle
-- RemoveAllPassengers, ForceCombatStop(krick) and
-- krick->AI()->DoAction(ACTION_OUTRO) legs have no bridge — the outro never
-- fires in Lua (documented fidelity gap); timers still cancel on death.
local function ickCancelCombat(event, creature)
    initState(creature:GetGUID())
end

-- C++ boss_krickAI::KilledUnit (:324-330): if (victim->GetTypeId() !=
-- TYPEID_PLAYER) return; Talk(SAY_KRICK_SLAY) — the razuvious player-gated
-- variant precedent.
local function krickKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KRICK_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_ICK, 1, ickEnterCombat)
RegisterCreatureEvent(ENTRY_ICK, 2, ickCancelCombat)
RegisterCreatureEvent(ENTRY_ICK, 4, ickCancelCombat)
RegisterCreatureEvent(ENTRY_ICK, 23, ickCancelCombat)

RegisterCreatureEvent(ENTRY_KRICK, 3, krickKilledUnit)

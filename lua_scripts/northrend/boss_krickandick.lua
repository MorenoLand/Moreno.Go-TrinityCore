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
--   leg (:167-168) is PORTED in ickEnterCombat below (FindNearestCreature
--   boss Talk, mrfloppy/emily precedent); the EVENT_TOXIC_WASTE 5s /
--   EVENT_SHADOW_BOLT 10s seeds (:171-172) are PORTED in ickEnterCombat
--   below (SelectTarget pick verified 2026-10-24: Random,0 = any
--   threat-listed player, Random,1 = victim popped first — see handlers).
-- EVENT_MIGHTY_KICK (:249-252) — DoCastVictim(SPELL_MIGHTY_KICK 69021),
--   non-triggered victim-cast; re-arm 25s (:251).
-- EVENT_SPECIAL (:253-257) — RAND(EVENT_EXPLOSIVE_BARRAGE, EVENT_POISON_NOVA,
--   EVENT_PURSUIT) scheduled at 1s (:255; uniform math.random(3) keeping the
--   C++ argument order); re-arm {23s,28s} (:256).
-- EVENT_EXPLOSIVE_BARRAGE (:258-267) — PORTED (krick-presence gate wraps
--   Talk(SAY_KRICK_BARRAGE_1 2) + Talk(SAY_KRICK_BARRAGE_2 3) (:261-262) +
--   krick->CastSpell(krick, SPELL_EXPLOSIVE_BARRAGE_KRICK 69012, true) (:263)
--   + DoCastAOE(SPELL_EXPLOSIVE_BARRAGE_ICK 69263) (:264), C++ order;
--   DelayEvents outside the gate (:266); one-shot, fires only via
--   EVENT_SPECIAL picks).
-- EVENT_POISON_NOVA (:268-274) — PORTED: krick->AI()->Talk(
--   SAY_KRICK_POISON_NOVA 4) (:269-270) first, then Talk(SAY_ICK_POISON_NOVA 0)
--   (:272, Ick's own Talk) + DoCastAOE(SPELL_POISON_NOVA 68989) (:273),
--   C++ order; one-shot.
-- EVENT_PURSUIT (:275-279) — PORTED: krick->AI()->Talk(SAY_KRICK_CHASE 5)
--   (:276-277) first, then DoCastSelf(SPELL_PURSUIT 68987,
--   { SPELLVALUE_MAX_TARGETS, 1 }) (:278), C++ order; one-shot.
-- boss_ick JustDied / LeaveCombat / Reset (:176-189, :147-161) — timer
--   cancellation only; the vehicle RemoveAllPassengers, ForceCombatStop(krick)
--   and krick->AI()->DoAction(ACTION_OUTRO) legs (:180-187) have no bridge.
-- boss_krick KilledUnit (:324-330) — Talk(SAY_KRICK_SLAY 1) player-gated
--   (event 3 — the razuvious player-gated variant precedent).
--
-- DOCUMENTED-ONLY (no bridge):
-- EVENT_TOXIC_WASTE (:233-240) / EVENT_SHADOW_BOLT (:241-248) — PORTED
--   (2026-10-24): krick->CastSpell(target, 69024 / 69028), non-triggered,
--   via FindNearestCreature krick (barrage precedent) + the random-player
--   picks below; re-arms {7s,10s} / 15s unconditional per C++ (:239, :247).
-- boss_ick DoAction (:191-217) — ACTION_STORE_OLD_TARGET /
--   ACTION_RESET_THREAT threat store/reset machine: no DoAction /
--   threat-manager bridge.
-- boss_krick JustSummoned (:332-340) — _summons.Summon plus the
--   NPC_EXPLODING_ORB triggered self-casts (SPELL_EXPLODING_ORB 69017,
--   SPELL_AUTO_GROW 69020): no summon-list bridge.
-- boss_krick Reset (:315-322) — SetReactState(REACT_PASSIVE) +
--   SetFlag(UNIT_FLAG_NON_ATTACKABLE): PORTED via creature:SetReactState /
--   creature:SetFlag(59, ...) (2026-10-21; the intro-outro machine does NOT
--   restore them — no RemoveFlag / REACT_AGGRESSIVE anywhere in the C++).
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
--   SetStandState(dead) legs ride the unbridged outro machine (SetStandState
--   bridge live — the machine keeps them out).
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

-- UnitDefines.h:125/:340; UNIT_FIELD_FLAGS = 59 (UpdateFields.h:117).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x00000002
local REACT_PASSIVE = 0

local SAY_KRICK_SLAY = 1
local SAY_KRICK_BARRAGE_1 = 2
local SAY_KRICK_BARRAGE_2 = 3
local SAY_KRICK_POISON_NOVA = 4
local SAY_KRICK_CHASE = 5
local SAY_KRICK_AGGRO = 0 -- boss_krickandick.cpp:56
local SAY_ICK_POISON_NOVA = 0

local SPELL_MIGHTY_KICK = 69021
local SPELL_EXPLOSIVE_BARRAGE_ICK = 69263
local SPELL_POISON_NOVA = 68989
local SPELL_PURSUIT = 68987
local SPELL_EXPLOSIVE_BARRAGE_KRICK = 69012
local SPELL_TOXIC_WASTE = 69024
local SPELL_SHADOW_BOLT = 69028

local timers = {}

local function nowMs()
    -- GetCurrTime is wall-clock ms (Eluna global, registered at
    -- engine/scripting/runtime.go:784); os.clock() is CPU time and
    -- stalls while the server idles, which would corrupt the
    -- due/remaining math used by delayTimers.
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
-- picks, no re-arm. The krick-presence gate (:259) wraps the krick Talk /
-- triggered self-cast legs (:261-263) — PORTED below via FindNearestCreature
-- (aggro precedent); the ick cast rides inside the same gate per C++.
-- C++ :266 events.DelayEvents(20s) pushes every other armed timer
-- out 20s (delayTimers, thekal precedent).
local function onExplosiveBarrage(creature, guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        krick:Talk(SAY_KRICK_BARRAGE_1)
        krick:Talk(SAY_KRICK_BARRAGE_2)
        krick:CastSpell(krick, SPELL_EXPLOSIVE_BARRAGE_KRICK, true)
        creature:CastSpell(creature, SPELL_EXPLOSIVE_BARRAGE_ICK)
    end
    delayTimers(guid, 20000)
end

-- C++ :268-274 EVENT_POISON_NOVA: Talk(SAY_ICK_POISON_NOVA) (Ick's own Talk)
-- + DoCastAOE(SPELL_POISON_NOVA) = non-triggered self-cast; one-shot. The
-- krick->AI()->Talk(SAY_KRICK_POISON_NOVA) leg (:269-270) is PORTED below
-- (krick Talk first, C++ order :269-272).
local function onPoisonNova(creature, guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        krick:Talk(SAY_KRICK_POISON_NOVA)
    end
    creature:Talk(SAY_ICK_POISON_NOVA)
    creature:CastSpell(creature, SPELL_POISON_NOVA)
end

-- C++ :275-279 EVENT_PURSUIT: DoCastSelf(SPELL_PURSUIT,
-- { SPELLVALUE_MAX_TARGETS, 1 }) = non-triggered self-cast; one-shot. The
-- custom max-targets leg and the pursuit threat/fixate machine have no
-- bridge (see header); the krick->AI()->Talk(SAY_KRICK_CHASE) leg (:276-277)
-- is PORTED below (krick Talk first, C++ order :276-278).
local function onPursuit(creature, guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        krick:Talk(SAY_KRICK_CHASE)
    end
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

-- C++ SelectTarget(Random, 0, 0.0f, true) (:236-237), called on ICK:
-- random alive player from ick's threat list, no distance limit, tank
-- included (UnitAI.h: SelectTarget draws the threat list; dead targets
-- are offline refs, Go SortedEntries returns them too, so IsAlive
-- filters). Fixed 2026-10-25 from the GetPlayersInWorld same-map/
-- instance pool, which could pick threat-less players (tharon_ja /
-- violet-hold fix class).
local function randomPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, 0.0f, true) (:244-245): offset 1 pops the
-- current victim (UnitAI.h non-distance list is victim-first, then
-- the sorted remainder minus victim), not the threat-sorted head:
-- exclude by victim GUID, then random alive player among the rest.
local function randomNonVictimPlayer(creature)
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

-- C++ :233-240 EVENT_TOXIC_WASTE: krick-gated non-triggered cast on the
-- Random,0 pick; re-arm {7s,10s} unconditional (ScheduleEvent outside the
-- krick gate, :239).
local function onToxicWaste(creature, guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        local target = randomPlayer(creature)
        if target ~= nil then
            krick:CastSpell(target, SPELL_TOXIC_WASTE)
        end
    end
    schedule(guid, "toxicwaste", {7000, 10000}, function() onToxicWaste(creature, guid) end)
end

-- C++ :241-248 EVENT_SHADOW_BOLT: krick-gated non-triggered cast on the
-- Random,1 pick; re-arm 15s unconditional (:247). The C++ `return` that
-- skips the rest of the event loop that tick is unmodeled (independent
-- Lua timers, header deviations note).
local function onShadowBolt(creature, guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        local target = randomNonVictimPlayer(creature)
        if target ~= nil then
            krick:CastSpell(target, SPELL_SHADOW_BOLT)
        end
    end
    schedule(guid, "shadowbolt", 15000, function() onShadowBolt(creature, guid) end)
end

-- C++ boss_ickAI::JustEngagedWith (:163-173): seeds EVENT_MIGHTY_KICK 20s
-- (:170) and EVENT_SPECIAL {30s,35s} (:173). The krick->AI()->Talk(
-- SAY_KRICK_AGGRO 0) leg (:167-168) is PORTED below — cross-creature
-- Talk bridged via FindNearestCreature (the mrfloppy/emily precedent,
-- nil-guard mirrors GetKrick()); the stale "no bridge" note is
-- retracted. The BossAI::JustEngagedWith passthrough stays out; the
-- EVENT_TOXIC_WASTE 5s / EVENT_SHADOW_BOLT 10s seeds (:171-172) are
-- PORTED below — cross-creature target-cast rides the barrage precedent
-- (obtained-object CastSpell) plus the verified SelectTarget picks.
local function ickEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    initState(guid)
    local krick = creature:FindNearestCreature(ENTRY_KRICK, 40.0)
    if krick ~= nil then
        krick:Talk(SAY_KRICK_AGGRO)
    end
    schedule(guid, "mightykick", 20000, function() onMightyKick(creature, guid) end)
    schedule(guid, "toxicwaste", 5000, function() onToxicWaste(creature, guid) end)
    schedule(guid, "shadowbolt", 10000, function() onShadowBolt(creature, guid) end)
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

-- C++ boss_krickAI::Reset (boss_krickandick.cpp:315-322): unconditional
-- me->SetReactState(REACT_PASSIVE) + SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NON_ATTACKABLE). The C++ never clears either (no RemoveFlag /
-- REACT_AGGRESSIVE in the file — krick is the scripted outro NPC riding
-- Ick), so the one-sided port is C++-verbatim, not fabricated.
local function krickReset(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
end

RegisterCreatureEvent(ENTRY_KRICK, 3, krickKilledUnit)
RegisterCreatureEvent(ENTRY_KRICK, 23, krickReset)

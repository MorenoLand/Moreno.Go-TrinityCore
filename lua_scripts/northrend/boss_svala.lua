-- Svala Sorrowgrave (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_svala.cpp
-- (621 lines; 6 scripts — boss_svala (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_SVALA_SORROWGRAVE = 0);
-- npc_ritual_channeler (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); npc_spectator (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); spell_paralyze_pinnacle (SpellScriptLoader, SpellScript
-- OnObjectAreaTargetSelect); npc_scourge_hulk (CreatureScript via
-- GetUtgardePinnacleAI (ScriptedAI)); achievement_incredible_hulk
-- (AchievementCriteriaScript OnCheck); all registered from inside
-- AddSC_boss_svala(); loader decl 132 / call 327 per
-- northrend_script_loader.cpp — the FIRST group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_utgarde_keep() (call 325); the checkpoint sequence
-- (utgarde_keep -> boss_svala) is followed).
-- Entries: 26668 Svala Sorrowgrave (utgarde_pinnacle.h
-- NPC_SVALA_SORROWGRAVE, line 53); 26555 Scourge Hulk
-- (boss_svala.cpp enum Creatures — not in utgarde_pinnacle.h —
-- registered this run for its ungated timer arms; entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual). DATA_SVALA_SORROWGRAVE = 0 (utgarde_pinnacle.h line 31);
-- 27281 Ritual Channeler (boss_svala.cpp enum Creatures — not in
-- utgarde_pinnacle.h — registered this run for its heroic Reset aura;
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual). The intro form NPC_SVALA = 29281 (line
-- 59) UpdateEntry-transforms into 26668; its Talk arms ride the
-- EVENT_INTRO_* timer machine (see DOCUMENTED-ONLY).
-- Sole-source verified: whole-server-tree grep for all six script names
-- hits boss_svala.cpp only (+ the loader decl/call lines for
-- AddSC_boss_svala); the clone's sql/ tree (~/workspace/moreno-trinitycore/sql/,
-- characters/ + world/) carries zero svala hits (empty of bindings). The
-- lua was ported Oct 2 in d2645ec (Talk-only, stale "no-timer-bridge"
-- doctrine) and UPGRADED this run under the CreateLuaEvent doctrine
-- (engine/scripting/runtime.go:798 — the maiden_of_virtue / keleseth /
-- yogg_saron precedent): module-local GUID-keyed timers table, wipe on
-- events 2 (leave combat), 4 (death), 23 (evade). creature:CastSpell(nil,
-- spell) = DoCastVictim; creature:CastSpell(creature, spell) = DoCastSelf
-- / DoCastAOE (the selin_fireheart / mr_smite / boss_ai.go DoCastAOE
-- lowering precedent); the 3rd CastSpell arg is the C++ DoCast 3rd arg
-- (2-arg / 1-arg overloads = non-triggered); {minMs, maxMs} table delay =
-- randtime range. The 2026-10-02 "no-timer-bridge" header doctrine is
-- RETRACTED for this file: CreateLuaEvent is a real registered bridge,
-- so the ungated timer arms below are ported rather than parked.
-- Eluna creature events (Svala): 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 15 OnSpellHitTarget, 23
-- OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_svala.cpp):
-- boss_svala JustEngagedWith (:176) — Talk(SAY_AGGRO 2) (event 1;
-- BossAI::JustEngagedWith passthrough unmodeled — the auriaya
-- engage-port precedent).
-- boss_svala KilledUnit (:209-212) — Talk(SAY_SLAY 3) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the
-- nil-guard is the hodir-port convention — Eluna event 3 always
-- carries a victim, so C++-exactness is unaffected).
-- boss_svala JustDied (:217-221) — HandleEmoteCommand(EMOTE_ONESHOT_
-- FLYDEATH 445) is unconditional in C++ (outside the SACRIFICING
-- SetEquipmentSlots gate) and PerformEmote is bridged
-- (lua_creature_events.go:347, Unit::HandleEmoteCommand lowering) —
-- ported; Talk(SAY_DEATH 4) unconditional (event 4 — the sjonnir
-- JustDied-Talk precedent); the SACRIFICING-gated SetEquipmentSlots
-- leg is PORTED in svalaJustDied (bridged; the phase gate is the
-- sacrificed latch — see the phase-machine section above). The
-- _JustDied() instance passthrough has no bridge.
-- npc_scourge_hulk (26555) — the scourge hulk's two spell-timer arms
-- are ungated in its UpdateAI (no phase groups, no DoAction legs —
-- the yogg_saron adds precedent), scheduled here on event 1 with the
-- maiden CreateLuaEvent convention; its C++ Reset re-seeds both
-- timers, matching combat-start scheduling (the xt002 precedent):
--   EVENT_MIGHTY_BLOW arm (:571-577): DoCast(victim, SPELL_MIGHTY_BLOW
--   48697), 2-arg = non-triggered -> CastSpell(victim, 48697); init
--   urand(4s,9s), re-arm urand(12s,17s).
--   EVENT_VOLATILE_INFECTION arm (:581-586): DoCastVictim
--   (SPELL_VOLATILE_INFECTION 56785), 1-arg = non-triggered ->
--   CastSpell(nil, 56785); init urand(10s,14s), re-arm urand(13s,17s);
--   C++-exact already: H_SPELL_VOLATILE_INFECTION 59228 is a dead
--   enum entry (defined :58, never referenced — the hulk fires
--   56785 unconditionally in both modes, anomalus precedent), so
--   there is no heroic arm to model; the old "no difficulty
--   bridge" rationale was doubly stale (creature:IsHeroic() is
--   bridged, ymiron/vazruden precedent).
-- DOCUMENTED DEVIATION: the C++ victim-stun gate
-- (!victim->HasUnitState(UNIT_STATE_STUNNED), :574-575 — "Prevent
-- knocking back a ritual player") has no bridge — the cast fires
-- unconditionally (the hodir UNIT_STATE_CASTING precedent).
-- npc_ritual_channeler (27281) Reset (:427-432) — the IsHeroic()-gated
-- DoCast(me, SPELL_SHADOWS_IN_THE_DARK 59407), 2-arg = non-triggered ->
-- CastSpell(creature, 59407) (event 23 — the Reset AddAura port
-- category; creature:IsHeroic() is bridged, lua_creature_events.go:260)
-- — the old "reset leg is unmodeled" header claim is STALE and
-- RETRACTED: the gating predicate is bridged and the arm is ungated
-- (no phase machine, no timers). The Initialize() paralyzeTimer re-seed
-- and the paralyze timer itself ride unbridged instance
-- GetGuidData(DATA_SACRIFICED_PLAYER) — stay out; no event-23 fire at
-- first grid spawn, so the spawn-time heroic aura stays unmodeled (the
-- palehoof freeze-visual precedent).
-- PORTED this run (2026-10-23, evphase triage) — the NORMAL <->
-- SACRIFICING phase machine. The old "blocker is the phase machine
-- (SetPhase / SetCombatMovement / MoveChase)" rationale is STALE and
-- RETRACTED: SetPhase is unbridged, but the latch is Lua-modelable
-- (gurtogg/bronjahm precedent) because BOTH transitions ride bridged
-- hooks, and SetCombatMovement IS bridged
-- (lua_creature_events.go:1297). So:
-- boss_svala JustEngagedWith (:176): the intro machine is unported, so
--   the boss engages already in the post-intro NORMAL form — the C++
--   intro-end seeds (EVENT_INTRO_DESPAWN_ARTHAS :339-345: 7s /
--   {10s,20s}) land at enter combat (intro-collapse deviation, the
--   garfrost forge-jump precedent).
-- boss_svala intro-end mirror (:339): mirror SetGoState(GO_STATE_ACTIVE)
--   PORTED at enter combat (intro-collapse point, C++ order: mirror
--   leg before the timer seeds); intro-start mirror READY (:196)
--   stays out (no intro-start bearer in the collapsed model).
-- EVENT_SINISTER_STRIKE (:350-353) — DoCastVictim(SPELL_SINSTER_STRIKE
--   15667), non-triggered, init 7s / re-arm {5s,9s}, NORMAL-gated via
--   the sacrificed latch (re-arm guard kills the chain on the latch,
--   garfrost chilling-wave idiom).
-- EVENT_CALL_FLAMES (:354-358) — SelectTarget(Random, 0, 100.0f,
--   playerOnly) -> DoCast(target, SPELL_CALL_FLAMES 48258),
--   non-triggered, init {10s,20s} / re-arm {10s,20s} unconditional
--   (outside the target-if, C++-exact); the 100yd filter is a plain
--   GetDistance check on the garfrost GetPlayersInWorld precedent.
-- UpdateAI :244-252 — the one-shot HealthBelowPct(50) latch ->
--   SACRIFICING, ported on event 9 with the postDamage > 0 guard
--   (nefarian-form rule — C++ UpdateAI is post-damage) and strict <
--   (C++ HealthBelowPct, Unit.h:872). The ritual summon machine
--   (RITUAL_PREPARATION + CHANNELERS + DISARM, instance SetGuidData
--   legs, SAY_SACRIFICE_PLAYER fused with the summon — zarithrian
--   precedent) stays out; the NORMAL timer group goes inert
--   (cancelGroup = the phase-group semantics) and SetCombatMovement
--   (false) models the C++ stand-down (done in the 0s-scheduled
--   EVENT_RITUAL_PREPARATION).
-- EVENT_RITUAL_STRIKE skeleton (:376-382) — StopMoving PORTED below
--   (bridge live, lua_creature_events.go); MoveIdle has no bridge;
--   InterruptNonMeleeSpells(true) is bridged. The triggered
--   self-cast of SPELL_RITUAL_STRIKE_TRIGGER (48331) lands 48277 on
--   Svala via its unbridged SpellScript — collapsed to the direct
--   48277 hit so the C++-exact SpellHitTarget phase arm fires (event
--   15 fires on the caster's bindings — gruul shatter precedent — and
--   Svala is both caster and target); 2000ms = the C++ ritual chain
--   delay (PREPARATION 0s -> CHANNELERS 1s -> STRIKE 2s).
-- SpellHitTarget (:223-233) — the phase-change arm: the 48277 hit
--   while sacrificed flips to NORMAL (SetCombatMovement(true) +
--   fresh 7s / {10s,20s} re-arms, C++-exact). SVALADEAD is never set
--   in C++ (dead gate); the SelectTarget(300yd) + MoveChase leg has
--   no MotionMaster bridge — engine combat movement keeps chasing
--   the victim (documented deviation).
-- Reset (:155-170) — SetCombatMovement(true) (C++-exact) and
--   SetDisableGravity(IsInPhase(NORMAL)): the intro never runs, so
--   the boss is always in the post-intro hover form —
--   SetDisableGravity(true) (bridged, lua_creature_events.go:784;
--   C++ Reset with _introCompleted -> NORMAL -> true).
-- JustDied (:216) — the SACRIFICING-gated SetEquipmentSlots(false,
--   EQUIP_UNEQUIP, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE) = (0,
--   0xFFFFFFFF, 0xFFFFFFFF) (malchezaar ClearWeapons precedent;
--   bridged); the phase gate is the sacrificed latch.
-- DOCUMENTED DEVIATIONS (phase machine): the NORMAL arms are seeded
-- at enter combat rather than at intro end (the intro's ~47s of Talk
-- / transform / MoveTakeoff / spectator-flee is unported — no
-- MoveInLineOfSight bridge, cross-creature Arthas machine); the
-- ritual return is a 2000ms timer + direct 48277 hit rather than the
-- trigger->SpellScript chain; MoveChase legs are dropped (engine
-- movement). Every timer value and cast arg is otherwise C++-exact.
-- DOCUMENTED-ONLY (in this header; no registration beyond 26668 /
-- 26555 / 27281):
-- EVENT_RITUAL_PREPARATION (:359-369) — SACRIFICING-gated; Talk
-- (SAY_SACRIFICE_PLAYER 5) is fused with the ritual summon machine
-- (SelectTarget sacrifice -> instance SetGuidData(DATA_SACRIFICED_
-- PLAYER) + DoCast(target, SPELL_RITUAL_PREPARATION 48267) +
-- SetCombatMovement(false) + DoCast(me, SPELL_RITUAL_OF_THE_SWORD
-- 48276)) — summon / instance-GUID / MotionMaster legs, no bridges
-- (the zarithrian Talk-fused-with-summon precedent). Stay out.
-- EVENT_SPAWN_RITUAL_CHANNELERS / EVENT_RITUAL_STRIKE /
-- EVENT_RITUAL_DISARM (:370-384) — all SACRIFICING-gated summon/
-- interrupt legs (SPELL_RITUAL_CHANNELER_{1,2,3} 48271/48274/48275,
-- SPELL_RITUAL_STRIKE_TRIGGER 48331, SPELL_RITUAL_DISARM 54159) — no
-- bridges. Stay out.
-- The whole intro Talk machine — Talk(SAY_SVALA_INTRO_0 0) on the
-- pre-transform entry (EVENT_INTRO_SVALA_TALK_0), Talk(SAY_SVALA_
-- INTRO_1 0) / Talk(SAY_SVALA_INTRO_2 1) on the boss entry
-- (EVENT_INTRO_SVALA_TALK_1 / EVENT_INTRO_SVALA_TALK_2), and the
-- cross-creature arthas->AI()->Talk(SAY_DIALOG_OF_ARTHAS_1 0 /
-- SAY_DIALOG_OF_ARTHAS_2 1) — the intro machine (MoveInLineOfSight
-- IDLE -> INTRO, summon NPC_ARTHAS 29280, MotionMaster takeoff/land,
-- UpdateEntry 29281 -> 26668 — UpdateEntry itself bridged
-- 2026-10-22, but the MLOS-IDLE trigger + summon + motion legs
-- are not) stays out.
-- npc_spectator (26667) — MovementInform flee machine, no Talk arms,
-- no MovementInform bridge. Stay out.
-- spell_paralyze_pinnacle (SpellScript target filter) and
-- achievement_incredible_hulk (GetData(DATA_INCREDIBLE_HULK 2043))
-- — no SpellScript / GetData / achievement bridges. Stay out.
-- npc_scourge_hulk DamageTaken — PORTED: lethal damage from Svala
-- (attacker:GetEntry() == NPC_SVALA_SORROWGRAVE 26668) latches
-- killedByRitualStrike (boss_svala.cpp:560-564). Pure bool latch —
-- no damage rewrite (thekal convention: no return), fires on lethal
-- hits so no postDamage guard. The C++ latch is read only by
-- GetData(DATA_INCREDIBLE_HULK) for the achievement script, which
-- has no bridge — the latch is kept per-GUID in a Lua local so a
-- future bridge lands on real state.

local ENTRY_SVALA_SORROWGRAVE = 26668
local ENTRY_SCOURGE_HULK = 26555
local ENTRY_RITUAL_CHANNELER = 27281
local GO_UTGARDE_MIRROR = 191745

local SAY_AGGRO = 2
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_ONESHOT_FLYDEATH = 445

local SPELL_MIGHTY_BLOW = 48697
local SPELL_VOLATILE_INFECTION = 56785
local SPELL_SHADOWS_IN_THE_DARK = 59407
local SPELL_SINSTER_STRIKE = 15667
local SPELL_CALL_FLAMES = 48258
local SPELL_RITUAL_STRIKE_EFF_1 = 48277

-- ScriptedCreature.h: EQUIP_UNEQUIP = 0, EQUIP_NO_CHANGE = -1 (the Go
-- bridge takes uint32, so 0xFFFFFFFF = untouched — malchezaar
-- ClearWeapons precedent).
local EQUIP_UNEQUIP = 0
local EQUIP_NO_CHANGE = 0xFFFFFFFF

local GROUP_NORMAL = { "sinister", "flames" }

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

local function cancelGroup(guid, keys)
    local per = timers[guid]
    if not per then
        return
    end
    for _, key in ipairs(keys) do
        if per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
    end
end

-- NORMAL <-> SACRIFICING phase latch (the C++ EventMap SetPhase is
-- unbridged; the two transitions are both bridged hooks — the 50%
-- UpdateAI latch via event 9, the return via event 15 — so the latch
-- lives here, gurtogg/bronjahm precedent).
local svalaState = {}

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 100.0f, true) — random
-- alive player on the same map/instance within range (the garfrost
-- GetPlayersInWorld precedent + distance filter).
local function randomPlayerInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= range then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ :350-353 EVENT_SINISTER_STRIKE (NORMAL phase): DoCastVictim
-- (15667), non-triggered; re-arm {5s,9s}. The NORMAL group goes inert
-- on the sacrifice latch (cancelGroup below); the re-arm guard kills
-- the chain if the latch won the race (garfrost chilling-wave idiom).
local function onSinisterStrike(creature, guid)
    local st = svalaState[guid]
    if not st or st.sacrificed then
        return
    end
    creature:CastSpell(nil, SPELL_SINSTER_STRIKE)
    schedule(guid, "sinister", {5000, 9000}, function()
        onSinisterStrike(creature, guid)
    end)
end

-- C++ :354-358 EVENT_CALL_FLAMES (NORMAL phase): random player within
-- 100yd -> DoCast(target, 48258), non-triggered; re-arm {10s,20s}
-- unconditional (outside the target-if, C++-exact).
local function onCallFlames(creature, guid)
    local st = svalaState[guid]
    if not st or st.sacrificed then
        return
    end
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CALL_FLAMES)
    end
    schedule(guid, "flames", {10000, 20000}, function()
        onCallFlames(creature, guid)
    end)
end

-- Shared NORMAL-phase entry: SetCombatMovement(true) + the C++-exact
-- intro-end / SpellHitTarget inits (7s / {10s,20s}). Used at enter
-- combat (intro-collapse) and on the ritual-strike return.
local function enterNormalPhase(creature, guid)
    local st = svalaState[guid]
    if not st then
        return
    end
    st.sacrificed = false
    creature:SetCombatMovement(true)
    schedule(guid, "sinister", 7000, function()
        onSinisterStrike(creature, guid)
    end)
    schedule(guid, "flames", {10000, 20000}, function()
        onCallFlames(creature, guid)
    end)
end

-- C++ EVENT_RITUAL_STRIKE skeleton (:376-382): StopMoving PORTED below
-- (bridge live, lua_creature_events.go); MoveIdle has no bridge;
-- InterruptNonMeleeSpells(true) is bridged. The
-- triggered self-cast of SPELL_RITUAL_STRIKE_TRIGGER (48331) lands
-- SPELL_RITUAL_STRIKE_EFF_1 (48277) on Svala via its unbridged
-- SpellScript — collapsed to the direct 48277 hit so the C++-exact
-- SpellHitTarget phase arm (event 15, gruul shatter precedent) fires
-- (garfrost forge-jump-collapse precedent). 2000ms = the C++ ritual
-- chain delay (RITUAL_PREPARATION 0s -> SPAWN_CHANNELERS 1s ->
-- RITUAL_STRIKE 2s).
local function onRitualStrike(creature, guid)
    local st = svalaState[guid]
    if not st or not st.sacrificed then
        return
    end
    -- C++ boss_svala.cpp:377-380 order: StopMoving, MoveIdle (no bridge),
    -- InterruptNonMeleeSpells(true), triggered cast.
    creature:StopMoving()
    creature:InterruptNonMeleeSpells(true)
    creature:CastSpell(creature, SPELL_RITUAL_STRIKE_EFF_1, true)
end

-- C++ UpdateAI :244-252: one-shot HealthBelowPct(50) latch ->
-- SACRIFICING. UpdateAI is post-damage, so Lua event 9 needs the
-- postDamage > 0 guard (nefarian-form rule); strict < matches C++
-- HealthBelowPct (Unit.h:872). The ritual summon machine
-- (RITUAL_PREPARATION + CHANNELERS + DISARM, instance SetGuidData
-- legs, SAY_SACRIFICE_PLAYER fused with the summon — zarithrian
-- precedent) stays out; the NORMAL timer group goes inert
-- (cancelGroup = the phase-group semantics) and SetCombatMovement
-- (false) models the C++ stand-down (done in the 0s-scheduled
-- EVENT_RITUAL_PREPARATION).
local function svalaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = svalaState[guid]
    if not st or st.sacrificed then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage * 100 < maxHealth * 50 then
        st.sacrificed = true
        cancelGroup(guid, GROUP_NORMAL)
        creature:SetCombatMovement(false)
        schedule(guid, "ritualstrike", 2000, function()
            onRitualStrike(creature, guid)
        end)
    end
end

-- C++ boss_svalaAI::SpellHitTarget (:223-233): the phase-change arm —
-- the 48277 hit while not NORMAL flips to NORMAL and re-arms the
-- NORMAL set fresh. SVALADEAD is never set in C++ (dead gate); the
-- SelectTarget(300yd) + MoveChase leg has no MotionMaster bridge —
-- engine combat movement keeps chasing the victim (documented
-- deviation).
local function svalaSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_RITUAL_STRIKE_EFF_1 then
        return
    end
    local guid = creature:GetGUID()
    local st = svalaState[guid]
    if not st or not st.sacrificed then
        return
    end
    enterNormalPhase(creature, guid)
end

-- C++ boss_svalaAI::JustEngagedWith (:176): BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function svalaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    svalaState[guid] = { sacrificed = false }
    creature:Talk(SAY_AGGRO)
    -- The intro machine is unported, so the boss engages already in the
    -- post-intro NORMAL form: the C++ intro-end seeds
    -- (EVENT_INTRO_DESPAWN_ARTHAS :339-345: 7s / {10s,20s}) land here
    -- (intro-collapse deviation, garfrost forge-jump precedent).
    -- C++ intro-end order puts the mirror leg first (:339): the
    -- Utgarde mirror (entry unique in the instance, instance
    -- GetGuidData in C++) -> SetGoState(GO_STATE_ACTIVE), before the
    -- timer seeds. The intro-start mirror READY (:196, MoveInLOS
    -- IDLE->INTRO) stays out: the collapsed model has no intro-start
    -- bearer, and engage IS the post-intro point, so a READY here
    -- would invert C++ order.
    local mirror = creature:FindNearestGameObject(GO_UTGARDE_MIRROR, 100.0)
    if mirror then mirror:SetGoState(0) end -- GO_STATE_ACTIVE
    enterNormalPhase(creature, guid)
end

-- C++ KilledUnit (:209-212): if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent; the
-- nil-guard is the hodir-port convention (Eluna event 3 always carries a
-- victim, so C++-exactness is unaffected).
local function svalaKilledUnit(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied (:217-221): HandleEmoteCommand(EMOTE_ONESHOT_FLYDEATH
-- 445) unconditional — ported (PerformEmote bridged); Talk(SAY_DEATH) —
-- unconditional — the sjonnir JustDied-Talk precedent; SACRIFICING-gated
-- SetEquipmentSlots + _JustDied() instance passthrough stay out
-- (phase-gated / no bridge).
local function svalaJustDied(event, creature)
    local guid = creature:GetGUID()
    local st = svalaState[guid]
    -- C++ JustDied (:216): the SACRIFICING-gated SetEquipmentSlots
    -- (false, EQUIP_UNEQUIP, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE) = (0,
    -- 0xFFFFFFFF, 0xFFFFFFFF) (malchezaar ClearWeapons precedent;
    -- bridged). The phase gate is the sacrificed latch.
    if st and st.sacrificed then
        creature:SetEquipmentSlots(EQUIP_UNEQUIP, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE)
    end
    svalaState[guid] = nil
    cancelTimers(guid)
    creature:PerformEmote(EMOTE_ONESHOT_FLYDEATH)
    creature:Talk(SAY_DEATH)
end

-- Shared state wipe for events 2/23. C++ Reset (:155-170):
-- SetCombatMovement(true) (bridged); SetDisableGravity
-- (IsInPhase(NORMAL)) — the intro never runs, so the boss is always
-- in the post-intro hover form: SetDisableGravity(true) (bridged,
-- lua_creature_events.go:784; C++ Reset with _introCompleted ->
-- NORMAL -> true).
local function svalaWipeCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    svalaState[guid] = nil
    creature:SetCombatMovement(true)
    creature:SetDisableGravity(true)
end

-- C++ npc_ritual_channelerAI::Reset (:427-432): Initialize() (the
-- paralyzeTimer re-seed — the paralyze machine rides unbridged instance
-- GetGuidData(DATA_SACRIFICED_PLAYER)) + the IsHeroic()-gated DoCast(me,
-- SPELL_SHADOWS_IN_THE_DARK 59407), 2-arg = non-triggered ->
-- CastSpell(creature, 59407) (the Reset AddAura port category —
-- creature:IsHeroic() is bridged, lua_creature_events.go:260). No event-23
-- fire at first grid spawn, so the spawn-time heroic aura stays unmodeled
-- (the palehoof freeze-visual precedent).
local function channelerReset(event, creature)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_SHADOWS_IN_THE_DARK)
    end
end

-- C++ npc_scourge_hulkAI mightyBlow arm (:571-577): DoCast(victim,
-- SPELL_MIGHTY_BLOW 48697), 2-arg = non-triggered; init urand(4s,9s),
-- re-arm urand(12s,17s). The !HasUnitState(UNIT_STATE_STUNNED) gate is
-- unmodeled (documented in the header).
local function mightyBlowTick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIGHTY_BLOW)
    end
    schedule(guid, "mightyblow", {12000, 17000}, function()
        mightyBlowTick(creature, guid)
    end)
end

-- C++ npc_scourge_hulkAI volatileInfection arm (:581-586):
-- DoCastVictim(SPELL_VOLATILE_INFECTION 56785), 1-arg = non-triggered;
-- init urand(10s,14s), re-arm urand(13s,17s). No heroic variant —
-- H_SPELL_VOLATILE_INFECTION 59228 is a dead enum entry, never
-- referenced in the C++ (56785 fires both modes, anomalus
-- precedent).
local function volatileInfectionTick(creature, guid)
    creature:CastSpell(nil, SPELL_VOLATILE_INFECTION)
    schedule(guid, "volatileinfection", {13000, 17000}, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- C++ npc_scourge_hulkAI::Reset re-seeds both timers (Initialize,
-- :540-544) — the xt002 precedent: combat-start scheduling carries the
-- Reset inits.
local function hulkEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mightyblow", {4000, 9000}, function()
        mightyBlowTick(creature, guid)
    end)
    schedule(guid, "volatileinfection", {10000, 14000}, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 on the hulk.
local hulkRitualStrike = {}

-- C++ npc_scourge_hulkAI::DamageTaken (boss_svala.cpp:560-564).
local function hulkDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() and attacker ~= nil
            and attacker:GetEntry() == ENTRY_SVALA_SORROWGRAVE then
        hulkRitualStrike[creature:GetGUID()] = true
    end
end

-- Shared timer wipe for events 2/23/4 on the hulk.
local function hulkCancelCombatTimers(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    hulkRitualStrike[guid] = nil
end

RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 1, svalaEnterCombat)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 2, svalaWipeCombat)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 3, svalaKilledUnit)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 4, svalaJustDied)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 9, svalaDamageTaken)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 15, svalaSpellHitTarget)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 23, svalaWipeCombat)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 1, hulkEnterCombat)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 2, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 4, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 9, hulkDamageTaken)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 23, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_RITUAL_CHANNELER, 23, channelerReset)
RegisterCreatureEvent(ENTRY_RITUAL_CHANNELER, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_svala.cpp:435: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
end)

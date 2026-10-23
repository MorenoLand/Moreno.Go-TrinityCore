-- Volkhan (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_volkhan.cpp
-- (boss_volkhan (CreatureScript) via GetHallsOfLightningAI<boss_volkhanAI>
-- (BossAI); npc_molten_golem (CreatureScript) via
-- GetHallsOfLightningAI<npc_molten_golemAI> (ScriptedAI);
-- achievement_shatter_resistant (AchievementCriteriaScript) —
-- all registered from inside AddSC_boss_volkhan(); loader decl 101 /
-- call 296 per northrend_script_loader.cpp — the FOURTH group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_ionar() (decl 100 / call 295), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_instance_halls_of_lightning() (decl 102 / call 297) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_ionar -> boss_volkhan) is followed).
-- Entry: 28587 Volkhan (halls_of_lightning.h NPC_VOLKHAN, line 40;
-- DATA_VOLKHAN = 1, line 32; GO_VOLKHAN_DOOR = 191325, line 48);
-- instance_halls_of_lightning.cpp binds { GO_VOLKHAN_DOOR,
-- DATA_VOLKHAN, DOOR_TYPE_PASSAGE } (line 27), OnCreatureCreate case
-- NPC_VOLKHAN (line 54), GetGuidData case DATA_VOLKHAN (line 107) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_volkhan"
-- hits boss_volkhan.cpp + the loader decl/call lines for
-- AddSC_boss_volkhan() only; whole-server-tree grep for
-- "npc_molten_golem" hits boss_volkhan.cpp only; whole-server-tree
-- grep for "achievement_shatter_resistant" hits boss_volkhan.cpp
-- only; zero sql/ hits for all three. No volkhan lua existed.
-- NPC_MOLTEN_GOLEM = 28695 / NPC_BRITTLE_GOLEM = 28681 /
-- NPC_VOLKHAN_ANVIL = 28823 / MAX_GOLEM = 2 /
-- DATA_SHATTER_RESISTANT = 2042 — local enums in boss_volkhan.cpp
-- (the acolyte_of_shadron / stormforged_lieutenant local-enum
-- precedent) — entry-verifiable, no registration proceeds for them
-- (bridge-blocked, see below).
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the SetPhase /
-- ScheduleEvent legs and the BossAI::JustEngagedWith passthrough
-- have no bridges — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 3) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — eighteenth player-gated variant ported.
-- JustDied — Talk(SAY_DEATH 4) (event 4; the DespawnGolem leg
-- (ObjectAccessor + summon-list — no bridge) and the _JustDied boss
-- bookkeeping passthrough have no bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28587):
-- boss_volkhan Reset / Initialize (m_bIsStriking / m_bHasTemper /
-- m_bCanShatterGolem / timers / phases / GolemsShattered /
-- m_uiHealthAmountModifier flag clears + _Reset() + DespawnGolem +
-- SetPhase(PHASE_INTRO) + EVENT_FORGE_CAST schedule — no bridges).
-- AttackStart (0.0f AddThreat + SetInCombatWith both ways + the
-- m_bHasTemper-gated MoveChase leg — no motion / threat bridges).
-- DespawnGolem (ObjectAccessor GUID loop over m_lGolemGUIDList,
-- DespawnOrUnsummon — no ObjectAccessor / summon-list bridges).
-- ShatterGolem (the brittle-golem-only CastSpell(SPELL_SHATTER
-- 52429) + GolemsShattered increment machine, :175-193 — the real
-- blocker is the summon-list/ObjectAccessor cross-creature chain,
-- not the cast form).
-- JustSummoned (NPC_MOLTEN_GOLEM 28695 SummonList latch +
-- random-target MoveFollow + non-triggered SPELL_HEAT 52387
-- cast via the CastSpellExtraArgs overload with SetOriginalCaster
-- only (boss_volkhan.cpp:204 — no trigger flags; earlier header
-- claim of "triggered" corrected 2026-10-14) — no summon-list /
-- cast / motion / target-selection bridges).
-- MovementInform POINT_MOTION_TYPE / EVENT_FORGE_CAST (m_uiSummonPhase
-- 2 -> SetOrientation(2.29f) -> phase 3 — no motion bridge).
-- GetData(DATA_SHATTER_RESISTANT 2042) -> GolemsShattered (no
-- GetData bridge; consumer is achievement_shatter_resistant below).
-- UpdateAI event machine: EVENT_PAUSE (m_bIsStriking resume-chase +
-- flag clears, 3.5s); EVENT_SHATTERING_STOMP STAYS OUT — verdict
-- decided 2026-10-14: the arm is gated on !m_bHasTemper &&
-- m_uiHealthAmountModifier >= 3 (boss_volkhan.cpp:255-257), and
-- m_bHasTemper rides the unbridged forge/temper machine (set true
-- at the health-gate :296-303, cleared only by EVENT_PAUSE
-- :243-249 after the golem-summon phases) — a bare health-gated
-- port would fire stomps the C++ suppresses during the temper
-- window, and the m_bCanShatterGolem -> EVENT_SHATTER ->
-- ShatterGolem cross-creature chain has no summon-list bridge;
-- EVENT_SHATTER (ShatterGolem + 3s reschedule); EVENT_FORGE_CAST
-- (DoCast(SPELL_FORGE_VISUAL 52654), 15s, PHASE_INTRO only,
-- :276-278): the real blocker is the Reset-seeded out-of-combat
-- PHASE_INTRO origin (Reset :119-120, phase-restricted reschedule
-- — no Reset-phase bridge), not the timer/cast form; the Talk
-- arms 2, 6, 5 ride the unmodeled machine.
-- UpdateAI forge summon-phase machine: the m_uiHealthAmountModifier
-- health check (HealthBelowPct(100-20*m_uiHealthAmountModifier) ->
-- Talk(SAY_FORGE 1) + interrupt + m_bHasTemper = true + phase 1);
-- phase 1 Talk(EMOTE_TO_ANVIL 5) + MovePoint(home position) ->
-- phase 2; phase 3 anvil TEMPER 52238 / TEMPER_DUMMY 52654 casts
-- (NPC_VOLKHAN_ANVIL 28823, 1000.0f) -> delay timer -> phase 4;
-- phase 4 SelectTarget(MaxThreat) + SetReactState(REACT_AGGRESSIVE) (:336)
-- + SetInCombatWith + MoveFollow -> phase 5; phase 5
-- MAX_GOLEM 2 triggered SPELL_SUMMON_MOLTEN_GOLEM 52405 casts on
-- the anvil + m_bIsStriking = true -> phase 0 — the forge-strike
-- sequence is a motion/target-selection machine (MovePoint :312,
-- cross-creature Temper casts :324-325, triggered summon cast :349)
-- with phase gating (PHASE_INTRO forge-cast :276-278, PHASE_NORMAL
-- gates :126-129) — no motion / target-selection bridges.
-- npc_molten_golem (NPC_MOLTEN_GOLEM 28695) — TIMER ARMS PORTED
-- (boss_volkhan.cpp:391-479, npc_molten_golemAI): entry 28695
-- registered below.
-- EVENT_BLAST (:467-469): DoCast(me, SPELL_BLAST_WAVE 23113)
-- non-triggered -> creature:CastSpell(creature, 23113); 20s init,
-- unconditional 20s re-arm.
-- EVENT_IMMOLATION (:471-474): DoCastVictim(SPELL_IMMOLATION_STRIKE
-- 52433) non-triggered -> creature:CastSpell(nil, 52433); 5s init;
-- C++ reschedules EVENT_BLAST (not EVENT_IMMOLATION) at 5s — the C++
-- typo replicated as written (the same "blast" timer key is
-- overwritten, matching EventMap.ScheduleEvent overwrite).
-- DamageTaken kill-shot latch (:425-444): damage >= GetHealth() ->
-- UpdateEntry(28681) + SetHealth(1) + damage = 0 + RemoveAllAuras +
-- m_bIsFrozen = true — PORTED via event 9 (the thekal kill-shot
-- precedent: return false, 0; UpdateEntry bridged 2026-10-22
-- (Creature::UpdateEntry twin); SetHealth + RemoveAllAuras bridged on
-- the handler surface, lua_creature_events.go:166/:189). Timers canceled
-- on the latch; frozen flag cleared on event 23 (C++ Reset ->
-- Initialize). The UpdateEntry morph re-seats faction/flags/display/
-- stats from the brittle template (the "Set in DB" stunned/
-- non-attackable flags ride the template unit_flags).
-- DOCUMENTED DEVIATIONS: InterruptNonMeleeSpells (no bridge; Go
-- creature casts are packet-only, the interrupt is a net no-op) +
-- MotionMaster clear (no motion bridge) stay out.
-- AttackStart (:411-424: m_bIsFrozen-gated MoveChase) — no motion
-- bridge; melee is engine-driven in Go like DoMeleeAttackIfReady.
-- SpellHit (:444-449: SPELL_SHATTER 52429 dummy on a brittle golem
-- -> DespawnOrUnsummon) — PORTED via event 14 (golemSpellHit below;
-- golemFrozen latch = UpdateEntry(NPC_BRITTLE_GOLEM) proxy, set on the
-- same kill-shot transition as the C++).
-- achievement_shatter_resistant (AchievementCriteriaScript
-- OnCheck: target->GetAI()->GetData(DATA_SHATTER_RESISTANT 2042) <
-- 5 — no GetData / achievement bridges; joins the
-- unmodeled-achievement queue).

local ENTRY_MOLTEN_GOLEM = 28695
local ENTRY_BRITTLE_GOLEM = 28681 -- boss_volkhan.cpp:30 (NPC_BRITTLE_GOLEM)
local ENTRY_VOLKHAN = 28587

local SAY_AGGRO = 0
local SAY_SLAY = 3
local SAY_DEATH = 4

-- C++ JustEngagedWith: Talk(AGGRO) + SetPhase(PHASE_NORMAL) +
-- three ScheduleEvent legs + BossAI passthrough — only the Talk arm
-- is bridgeable.
local function volkhanEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function volkhanTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(DEATH) + DespawnGolem + _JustDied() — only
-- the Talk arm is bridgeable.
local function volkhanDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    -- C++ boss_volkhan::JustDied (boss_volkhan.cpp:146-152): DespawnGolem.
    -- The GUID-list loop has no bridge, so sweep the forge arena for his
    -- molten golems (ENTRY_MOLTEN_GOLEM 28695, the only such creatures in
    -- the instance) via GetCreaturesInGrid and Despawn the living.
    local golems = creature:GetCreaturesInGrid(ENTRY_MOLTEN_GOLEM, 100)
    for _, golem in pairs(golems) do
        if golem:IsAlive() then
            golem:Despawn()
        end
    end
end

RegisterCreatureEvent(ENTRY_VOLKHAN, 1, volkhanEnterCombat)
RegisterCreatureEvent(ENTRY_VOLKHAN, 3, volkhanTargetDied)
RegisterCreatureEvent(ENTRY_VOLKHAN, 4, volkhanDied)

local SPELL_BLAST_WAVE = 23113
local SPELL_IMMOLATION_STRIKE = 52433

-- GUID-keyed timers + frozen flag (C++ m_bIsFrozen, :405).
local golemTimers = {}
local golemFrozen = {}

local function golemCancelTimers(guid)
    local per = golemTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        golemTimers[guid] = nil
    end
end

local function golemSchedule(guid, key, delay, fn)
    local per = golemTimers[guid]
    if not per then
        per = {}
        golemTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ UpdateAI :467-469: EVENT_BLAST — DoCast(me, SPELL_BLAST_WAVE)
-- non-triggered, unconditional 20s re-arm.
local function golemBlastTick(creature, guid)
    if golemFrozen[guid] then return end
    creature:CastSpell(creature, SPELL_BLAST_WAVE)
    golemSchedule(guid, "blast", 20000, function()
        golemBlastTick(creature, guid)
    end)
end

-- C++ UpdateAI :471-474: EVENT_IMMOLATION — DoCastVictim(
-- SPELL_IMMOLATION_STRIKE) non-triggered; C++ reschedules
-- EVENT_BLAST at 5s (the C++ typo as written) — the same "blast"
-- key overwrites the blast timer, matching EventMap overwrite.
local function golemImmolationTick(creature, guid)
    if golemFrozen[guid] then return end
    creature:CastSpell(nil, SPELL_IMMOLATION_STRIKE)
    golemSchedule(guid, "blast", 5000, function()
        golemBlastTick(creature, guid)
    end)
end

-- C++ Reset/Initialize :398-403 schedules both timers; they tick only
-- in combat (UpdateAI :455). Lua re-keys to event 1 per the tree
-- convention (halion / moroes).
local function golemEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    golemCancelTimers(guid)
    golemSchedule(guid, "blast", 20000, function()
        golemBlastTick(creature, guid)
    end)
    golemSchedule(guid, "immolation", 5000, function()
        golemImmolationTick(creature, guid)
    end)
end

local function golemLeaveCombat(event, creature)
    golemCancelTimers(creature:GetGUID())
end

local function golemDied(event, creature, killer)
    golemCancelTimers(creature:GetGUID())
end

-- C++ DamageTaken :425-444: kill-shot (damage >= GetHealth()) ->
-- UpdateEntry(NPC_BRITTLE_GOLEM) + SetHealth(1) + damage = 0 +
-- RemoveAllAuras + AttackStop + InterruptNonMeleeSpells(false) +
-- MotionMaster clear + m_bIsFrozen = true. Event 9 fires pre-application
-- with the damage (fireCreatureDamageTaken, lua_creature_events.go); the
-- boolean + damage rewrite is the thekal kill-shot precedent.
-- UpdateEntry is the Creature::UpdateEntry twin (bridged 2026-10-22):
-- the brittle template's faction/flags/display/stats apply (the "Set in
-- DB" stunned/non-attackable flags ride the template unit_flags), and
-- the entry change dispatches subsequent hooks to the brittle entry's
-- registrations (registered below for both entries — the C++ AI object
-- keeps receiving hooks after the transform). AttackStop bridged
-- 2026-10-22 (C++ order RemoveAllAuras -> AttackStop). Interrupt /
-- MotionMaster stay out behind true blockers (see header deviations).
local function golemDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if damage >= creature:GetHealth() then
        creature:UpdateEntry(ENTRY_BRITTLE_GOLEM)
        creature:SetHealth(1)
        creature:RemoveAllAuras()
        -- C++ boss_volkhan.cpp:433: me->AttackStop() (C++ order
        -- RemoveAllAuras -> AttackStop).
        creature:AttackStop()
        golemFrozen[guid] = true
        golemCancelTimers(guid)
        return false, 0
    end
end

-- C++ Reset :407-410 -> Initialize: m_bIsFrozen = false +
-- reschedules (timers re-arm on the next event 1).
local function golemReset(event, creature)
    local guid = creature:GetGUID()
    golemFrozen[guid] = nil
    golemCancelTimers(guid)
end

RegisterCreatureEvent(ENTRY_MOLTEN_GOLEM, 1, golemEnterCombat)
RegisterCreatureEvent(ENTRY_MOLTEN_GOLEM, 2, golemLeaveCombat)
RegisterCreatureEvent(ENTRY_MOLTEN_GOLEM, 4, golemDied)
-- C++ SpellHit :444-449 — the dummy effect of SPELL_SHATTER
-- (52429) on a brittle golem -> DespawnOrUnsummon(). No heroic
-- variant of SPELL_SHATTER in the C++ enum, so
-- GetSpellIdForDifficulty returns 52429 as-is. The C++ gates on
-- me->GetEntry() == NPC_BRITTLE_GOLEM (28681) — registered for both
-- entries (the UpdateEntry twin re-dispatches hooks to the live entry,
-- mirroring the C++ AI object surviving the transform); the entry check
-- inside is the real C++ gate, replacing the golemFrozen latch proxy.
local SPELL_SHATTER = 52429
local function golemSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_SHATTER and creature:GetEntry() == ENTRY_BRITTLE_GOLEM then
        local guid = creature:GetGUID()
        golemCancelTimers(guid)
        golemFrozen[guid] = nil
        creature:Despawn()
    end
end

RegisterCreatureEvent(ENTRY_MOLTEN_GOLEM, 14, golemSpellHit)
RegisterCreatureEvent(ENTRY_BRITTLE_GOLEM, 9, golemDamageTaken)
RegisterCreatureEvent(ENTRY_BRITTLE_GOLEM, 14, golemSpellHit)
RegisterCreatureEvent(ENTRY_MOLTEN_GOLEM, 23, golemReset)
RegisterCreatureEvent(ENTRY_BRITTLE_GOLEM, 23, golemReset)

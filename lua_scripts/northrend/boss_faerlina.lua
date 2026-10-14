-- Grand Widow Faerlina (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_faerlina.cpp
-- (boss_faerlina (BossAI); npc_faerlina_add (ScriptedAI);
-- at_faerlina_entrance (OnlyOnceAreaTriggerScript);
-- achievement_momma_said_knock_you_out (AchievementCriteriaScript);
-- AddSC_boss_faerlina registers all four; loader decl 70 / call 265 per
-- northrend_script_loader.cpp — the TWELFTH Naxxramas group in
-- AddNorthrendScripts(), under the "// Naxxramas" marker; the call
-- before it is AddSC_boss_four_horsemen() (decl 69 / call 264), the
-- call after is AddSC_boss_heigan() (decl 71 / call 266)).
-- Entry: 15953 Faerlina (naxxramas.h NPC_FAERLINA :89 — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_FAERLINA -> DATA_FAERLINA GUID
-- (case NPC_FAERLINA, line 136) and the boss index -> BOSS_FAERLINA
-- :31; the CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-scripts-tree grep for "boss_faerlina" /
-- "npc_faerlina_add" / "at_faerlina_entrance" /
-- "achievement_momma_said_knock_you_out" hits boss_faerlina.cpp only
-- (loader carries only the decl/call lines); zero sql/ hits. No
-- faerlina lua existed. Talk() count = 6 grep-confirmed (zero spaced
-- "Talk (" variant) — ALL SIX accounted below.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady (jedoga bar —
-- engagement-only leg, documented). The EVENT_POISON timer rides
-- CreateLuaEvent (the maiden convention), cancelled on 2/4/23.
-- Ported arms (C++-exact for all modeled arms):
-- boss_faerlina: JustEngagedWith Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg, summons.
-- DoZoneInCombat, and the EVENT_FIRE / EVENT_FRENZY schedules ride
-- unbridged legs — tharon_ja precedent; the EVENT_POISON schedule
-- seeds the CreateLuaEvent machine below); KilledUnit Talk(SAY_SLAY 2) (event 3 —
-- C++-GATED: the Talk fires only when victim->GetTypeId()==
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType() ==
-- "Player" (nalorakk / kelthuzad player-gate precedent)); JustDied
-- Talk(SAY_DEATH 3) (event 4; _JustDied instance bookkeeping has no
-- bridge — tharon_ja precedent).
-- EVENT_POISON — PORTED (retraction re-audit 2026-10-14: the
-- "no DoCastAOE bridge / no aura-check bridge" claims are stale):
-- DoCastAOE(POISON_BOLT_VOLLEY 28796) non-triggered, gated on
-- !me->HasAura(SPELL_WIDOWS_EMBRACE_HELPER) — DoCastAOE lowers to
-- creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE self-cast
-- convention — selin_fireheart / mr_smite / razuvious precedent) and
-- the aura gate bridges via creature:HasAura (the freya
-- DRAINED_OF_POWER precedent); SPELL_WIDOWS_EMBRACE_HELPER is
-- RAID_MODE(28732, 54097) — no difficulty bridge, so both IDs are
-- checked (the applied aura is always the difficulty-appropriate
-- one); init randtime(10s,15s) (:104), repeat randtime(8s,15s).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_FIRE (init randtime(6s,18s), repeat randtime(6s,18s))
-- — SelectTarget(Random,0) -> DoCast(RAIN_OF_FIRE 28794) — no
-- random-target SelectTarget bridge (cairne/kazzak precedent);
-- EVENT_FRENZY (init Minutes(1)+randtime(0s,20s), repeat same) —
-- DoCastSelf(FRENZY 28798) + Talk(EMOTE_FRENZY 5) is moroes-ready in
-- isolation, but the C++ arm is gated: with GetAura(
-- SPELL_WIDOWS_EMBRACE_HELPER) active it skips the cast and
-- reschedules at aura duration +1ms (RAID_MODE 28732/54097). No
-- aura-duration bridge, so scheduling the cast ungated would fabricate
-- behavior (casting Frenzy through Widow's Embrace) — joins the
-- bridgeable-but-context-blocked queue (gluth deferral-arm precedent);
-- SummonAdds — SummonCreatureGroup(SUMMON_GROUP_WORSHIPPERS 1) +
-- 25-man SUMMON_GROUP_FOLLOWERS 2 — no summon bridge + no difficulty
-- bridge (kelidan precedent); Reset — _Reset() + _frenzyDispels=0 —
-- BossAI bookkeeping (tharon_ja precedent); SpellHit Talk(
-- EMOTE_WIDOW_EMBRACE 4, caster) + frenzy-dispel counter + Kill of the
-- caster — SpellHit never fires in the Lua surface (SpellHit ruling);
-- GetData(DATA_FRENZY_DISPELS 1) — consumed by the achievement (no
-- achievement bridge); at_faerlina_entrance Talk(SAY_GREET 0) via
-- instance GetGuidData(DATA_FAERLINA), NOT_STARTED-gated — no
-- area-trigger bridge (anubrekhan entrance precedent); achievement_
-- momma_said_knock_you_out (GetData(DATA_FRENZY_DISPELS)==0) — no
-- achievement bridge (tharon_ja precedent); npc_faerlina_add
-- (ScriptedAI — no RegisterLuaBoss surface, razuvious precedent):
-- Reset 10-man BIND/CHARM immunities (no immunity/difficulty bridges),
-- JustEngagedWith cross-AI DoZoneInCombat via GetGuidData(
-- DATA_FAERLINA) (no instance/cross-AI bridges), JustDied 10-man
-- DoCast(WIDOWS_EMBRACE 28732) on Faerlina (same bar), UpdateAI
-- DoCastVictim(ADD_FIREBALL 54095/54096 difficulty pair) — moroes-ready
-- in isolation but the ScriptedAI surface has no entry bridge — joins
-- the bridgeable-but-entry-blocked queue (25-man spell 54096 noted).

local ENTRY_FAERLINA = 15953

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_DEATH = 3

local SPELL_POISON_BOLT_VOLLEY = 28796
local SPELL_WIDOWS_EMBRACE_10 = 28732
local SPELL_WIDOWS_EMBRACE_25 = 54097

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the maiden / bjarngrim-lieutenant convention.
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

-- C++ UpdateAI EVENT_POISON (boss_faerlina.cpp:162-166):
-- DoCastAOE(SPELL_POISON_BOLT_VOLLEY 28796) non-triggered, gated on
-- !me->HasAura(SPELL_WIDOWS_EMBRACE_HELPER), randtime(10s,15s) init
-- (:104) / randtime(8s,15s) repeat.
local function poisonTick(creature, guid)
    if not creature:HasAura(SPELL_WIDOWS_EMBRACE_10)
        and not creature:HasAura(SPELL_WIDOWS_EMBRACE_25) then
        creature:CastSpell(creature, SPELL_POISON_BOLT_VOLLEY)
    end
    schedule(guid, "poison", {8000, 15000}, function()
        poisonTick(creature, guid)
    end)
end

local function faerlinaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "poison", {10000, 15000}, function()
        poisonTick(creature, guid)
    end)
end

local function faerlinaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function faerlinaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function faerlinaDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FAERLINA, 1, faerlinaEnterCombat)
RegisterCreatureEvent(ENTRY_FAERLINA, 2, faerlinaLeaveCombat)
RegisterCreatureEvent(ENTRY_FAERLINA, 3, faerlinaTargetDied)
RegisterCreatureEvent(ENTRY_FAERLINA, 4, faerlinaDied)
RegisterCreatureEvent(ENTRY_FAERLINA, 23, faerlinaLeaveCombat)

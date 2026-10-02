-- Krik'thir the Gatewatcher (Azjol-Nerub dungeon) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_krikthir_the_gatewatcher.cpp
-- (boss_krik_thir). Azjol-Nerub dungeon-script unit per
-- northrend_script_loader.cpp order (decl 27 / call 229, immediately
-- after AddSC_instance_ahnkahet(); the Azjol-Nerub dungeon block
-- opens here).
-- Entry: 28684 Krik'thir the Gatewatcher (azjol_nerub.h NPC_KRIKTHIR —
-- kalecgos pass; the GetAzjolNerubAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_krik_thir: EVENT_MIND_FLAY DoCastVictim(Mind Flay 52586),
-- randtime(1s,3s) init (C++ JustEngagedWith schedule; the passive
-- engage legs are unbridged but the arm itself is port-pattern-ready),
-- 9-11s repeat (math.random — slad_ran precedent); KilledUnit
-- Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 2) (event 4; the summons.clear() + _JustDied()
-- instance-bookkeeping legs have no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- The passive pre-fight model — ctor Reset leg SetReactState(REACT_PASSIVE)
-- (no react-state bridge — drakkari_colossus precedent); JustAppeared
-- -> SummonAdds() (instance->GetBossState(DATA_KRIKTHIR)==DONE gate +
-- SummonCreatureGroup(1..3) with DATA_PET_GROUP stamping — the summon
-- STRAND is absent, standing) — so nothing below is emulated.
-- MoveInLineOfSight passive-engage leg (JustEngagedWith(who) when passive
-- and within attack distance) — behind the absent react-state bridge.
-- EVENT_SEND_GROUP (DoCastAOE(SUBBOSS_AGGRO_TRIGGER 52343, triggered),
-- 70s repeat) — no DoCastAOE bridge (terestian/shazzrah precedent);
-- the trigger SpellScript's watcher-target selection (HandleTargets)
-- joins the no-SpellScript-binding queue (razelikh precedent).
-- EVENT_SWARM (DoCastAOE(SPELL_SWARM 52440) + Talk(SAY_SWARM 3), 5s
-- init, no repeat — C++ does not reschedule it) — no DoCastAOE bridge.
-- EVENT_FRENZY (HealthBelowPct(10) _hadFrenzy latch -> DoCastSelf(Frenzy
-- 28747) + DoCastAOE(CURSE_OF_FATIGUE 52592), 1s init, 15s repeat) —
-- no health-pct bridge (doomwalker precedent) + no DoCastAOE bridge.
-- The DoAction machine (no DoAction bridge — drakkari_colossus standing
-- precedent — and the payload is instance-model anyway):
-- -ACTION_GATEWATCHER_GREET (instance SetData(DATA_GATEWATCHER_GREET,1)
-- + Talk(SAY_PREFIGHT 4)) — instance-script model absent (standing).
-- ACTION_GASHRA/NARJIL/SILTHIL_DIED (_watchersActive decrement +
-- EVENT_SEND_GROUP reschedule) — cross-AI bridge absent.
-- ACTION_WATCHER_ENGAGED / ACTION_PET_ENGAGED (Talk(SAY_AGGRO 0) +
-- EVENT_SEND_GROUP 70s — the real C++ aggro yell, not wired here)
-- / ACTION_PET_EVADE -> EnterEvadeMode (summons.DespawnAll — no despawn
-- bridge, terestian precedent; _DespawnAtEvade) — documented-only.
-- SpellHit(SPELL_SUBBOSS_AGGRO_TRIGGER) -> DoZoneInCombat and
-- SpellHitTarget(SPELL_SUBBOSS_AGGRO_TRIGGER) -> Talk(SAY_SEND_GROUP 5)
-- — event 15 never fires (standing).
-- npc_watcher_gashra / npc_watcher_narjil / npc_watcher_silthik —
-- ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (28730 / 28729 / 28731 from
-- azjol_nerub.h — kalecgos pass): zero registration. Their whole
-- engagement model is the unbridged npc_gatewatcher_petAI machine:
-- REACT_PASSIVE via DATA_PET_GROUP stamping (react-state bridge absent),
-- group aggro (SetReactState(AGGRESSIVE) + AttackStart on same-group
-- adds), summoner DoAction(ACTION_PET_ENGAGED / ACTION_WATCHER_ENGAGED)
-- (cross-AI bridge absent), MoveInLineOfSight passive-engage leg, JustDied
-- -> instance->GetCreature(DATA_KRIKTHIR)->AI()->DoAction(ACTION_*_DIED)
-- (instance model absent), EnterEvadeMode -> summoner DoAction(
-- ACTION_PET_EVADE) (cross-AI absent). Port-pattern-ready rotation arms
-- — gashra: EVENT_ENRAGE DoCastSelf(ENRAGE 52470) 3-5s init, 12-20s
-- repeat + EVENT_INFECTED_BITE DoCastVictim(52469) 7-11s init, 23-27s
-- repeat; narjil: EVENT_BLINDING_WEBS DoCastVictim(52524) 13-18s init,
-- 23-27s repeat + EVENT_INFECTED_BITE DoCastVictim(52469) 7-11s init,
-- 20-25s repeat; silthik: EVENT_POISON_SPRAY DoCastVictim(52493) 16-19s
-- init, 13-19s repeat + EVENT_INFECTED_BITE DoCastVictim(52469) 3-5s
-- init, 20-24s repeat — stay unported: the watchers exist only as
-- SummonCreatureGroup summons (summon STRAND absent), so the bridged
-- arms would attach to creatures that never spawn, and wiring them
-- without the passive model fails the C++-exact port bar (amanitar
-- mushrooms precedent). The EVENT_WEB_WRAP arms (all three watchers,
-- DoCast(SelectTarget(Random, 0, 100.0f[, true]), WEB_WRAP 52086))
-- additionally need the absent random-target SelectTarget bridge
-- (cairne/kazzak precedent).
-- npc_anub_ar_warrior / npc_anub_ar_skirmisher /
-- npc_anub_ar_shadowcaster / npc_skittering_swarmer /
-- npc_skittering_infector / npc_gatewatcher_web_wrap — ENTRY
-- UNVERIFIABLE (no NPC constants in azjol_nerub.h or anywhere in the C++
-- sources; the adds are summoned from DB-side SummonCreatureGroup data,
-- not C++ evidence — belnistrasz/willix precedent, no invented
-- identifiers): zero registration; they join the bridgeable-but-entry-
-- blocked queue. Port-pattern-ready arms documented here: warrior
-- EVENT_CLEAVE DoCastVictim(49806) 7-9s init, 10-16s repeat +
-- EVENT_STRIKE DoCastVictim(52532) 5-10s init, 15-19s repeat; skirmisher
-- EVENT_ANUBAR_CHARGE behind the random-
-- target SelectTarget bridge + SpellHitTarget(CHARGE 52538) ->
-- DoCast(unitTarget, FIXATE_TRIGGER 52536) behind event-15 +
-- EVENT_BACKSTAB DoCastVictim(52540) gated on victim->isInBack (no
-- positional-facing bridge); shadowcaster EVENT_SHADOW_NOVA DoCastVictim
-- (52535) 10-14s init, 10-16s repeat (EVENT_SHADOW_BOLT 52534 behind the
-- random-target SelectTarget bridge); skittering_swarmer InitializeAI
-- (instance->GetCreature(DATA_KRIKTHIR)->getAttackerForHelper()
-- AttackStart + gatewatcher->AI()->JustSummoned — instance model
-- absent); skittering_infector JustDied DoCastAOE(ACID_SPLASH 52446) —
-- no DoCastAOE bridge (its InitializeAI is likewise instance-bound);
-- npc_gatewatcher_web_wrap (NullCreatureAI) JustDied -> summoner
-- RemoveAurasDueToSpell(WEB_WRAP_WRAPPED 52087) — no aura-removal
-- bridge (aeranas precedent) + summoner-unit bridge absent.
-- spell_gatewatcher_subboss_trigger (52343 watcher target selection)
-- / spell_anub_ar_skirmisher_fixate (52536 -> 52537) /
-- spell_gatewatcher_web_wrap (52086 expire -> 52087) — no
-- SpellScript/AuraScript binding bridge on the Lua surface (razelikh
-- precedent) — documented-only.
-- achievement_watch_him_die — AchievementCriteriaScript (instance
-- GetCreature per watcher DATA_ entry, all-dead check) — no
-- achievement-criteria bridge (snakes precedent) + instance-script
-- model absent — documented-only.

local ENTRY_KRIK_THIR = 28684

local SAY_SLAY  = 1
local SAY_DEATH = 2

local SPELL_MIND_FLAY = 52586

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

-- C++ EVENT_MIND_FLAY: DoCastVictim; randtime(1s,3s) init, 9-11s repeat.
local function mindFlayTick(creature, guid)
    creature:CastSpell(nil, SPELL_MIND_FLAY)
    local delay = math.random(9000, 11000)
    schedule(guid, "mindflay", delay, function()
        mindFlayTick(creature, guid)
    end)
end

local function krikThirEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mindflay", math.random(1000, 3000), function()
        mindFlayTick(creature, guid)
    end)
end

local function krikThirLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function krikThirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function krikThirDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function krikThirReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KRIK_THIR, 1, krikThirEnterCombat)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 2, krikThirLeaveCombat)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 3, krikThirTargetDied)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 4, krikThirDied)
RegisterCreatureEvent(ENTRY_KRIK_THIR, 23, krikThirReset)

-- Amanitar (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_amanitar.cpp
-- (boss_amanitar + npc_amanitar_mushrooms +
-- spell_amanitar_potent_fungus).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 34 / call 224, immediately after AddSC_boss_taldaram();
-- next: AddSC_boss_jedoga_shadowseeker() (decl 35 / call 225), then
-- AddSC_boss_volazj() (decl 36 / call 226, script boss_herald_volazj)).
-- Entry: 30258 Amanitar (ahnkahet.h NPC_AMANITAR — kalecgos pass; the
-- RegisterAhnKahetCreatureAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell[, triggered]) = DoCastSelf
-- (phase_hunter / apothecary_hanes precedent — the trailing true is
-- the triggered port).
-- Ported arms (C++-exact for all modeled arms):
-- boss_amanitar: EVENT_BASH — DoCastVictim(Bash 57094);
-- JustEngagedWith-scheduled 10-14s init, rescheduled 7-12s. The
-- only bridgeable arm of the slice.
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_MINI — SelectTarget(Random, 0, 0.0f, true, true,
-- -SPELL_MINI 57055) -> DoCastAOE(Mini 57055) (30s repeat; 1s
-- repeat when no valid target): the random-target SelectTarget
-- bridge is absent (cairne/kazzak precedent) and there is no
-- DoCastAOE bridge (terestian/shazzrah precedent).
-- EVENT_ROOT — SelectTarget(Random, 1, 100.0f, true) ->
-- DoCast(target, Entangling Roots 57095, triggered) (10-15s
-- repeat): random-target SelectTarget bridge absent.
-- EVENT_BOLT — SelectTarget(Random, 0, 100.0f, true) ->
-- DoCast(target, Venom Bolt Volley 57088, triggered) (18-22s
-- repeat): random-target SelectTarget bridge absent.
-- EVENT_SPAWN — SummonCreature(healthy 30391 40% / poisonous
-- 30435, TEMPSUMMON_CORPSE_TIMED_DESPAWN, 4s) over the 32
-- MushroomPositions (1s): summon STRAND absent (standing).
-- EVENT_RESPAWN — _mushroomsDeque dead-mushroom position re-spawn
-- machine (40-60s repeat): summon STRAND absent.
-- JustSummoned summons.Summon leg — no summon bridge.
-- SummonedCreatureDies mushroom-position bookkeeping (into the
-- deque the EVENT_RESPAWN leg reads) — cross-AI / summon bridges
-- absent.
-- EnterEvadeMode — summons.DespawnAll (no despawn bridge —
-- terestian precedent), instance
-- DoRemoveAurasDueToSpellOnPlayers(Mini) (instance-script model
-- absent — standing blocker), _DespawnAtEvade.
-- JustDied — _JustDied() instance bookkeeping only (no bridge),
-- DoCastAOE(Remove Mushroom Power 57283) (no DoCastAOE bridge),
-- instance DoRemoveAurasDueToSpellOnPlayers(Mini) (instance
-- model absent).
-- npc_amanitar_mushrooms — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (30391 NPC_HEALTHY_MUSHROOM / 30435 NPC_POISONOUS_MUSHROOM —
-- ahnkahet.h kalecgos pass): zero registration. Reset leg —
-- SetReactState(REACT_PASSIVE) (no react-state bridge —
-- drakkari_colossus precedent) + SetDisplayId(Modelid2) (no
-- display bridge) + DoCastSelf triplet (Putrid Mushroom 31690 /
-- Shrink 31691 triggered / Grow 57059 triggered) + the
-- healthy/poisonous visual-aura self-cast (56740 / 56741) + the
-- 800ms re-Grow scheduler: wiring the bridged self-casts without
-- the passive react-state bridge would leave the creature
-- engaging players — fails the C++-exact port bar. The
-- MoveInLineOfSight player-proximity poison leg (2.0f, _active
-- latch): target RemoveAurasDueToSpell(Potent Fungus 56648) (no
-- aura-removal bridge — aeranas precedent), DoCastAOE(Poisonous
-- Mushroom Poison Cloud 57061) (no DoCastAOE bridge),
-- SetObjectScale(0.1f) (no scale bridge), DespawnOrUnsummon(4s)
-- (no despawn bridge — terestian precedent). JustDied healthy
-- leg: DoCastAOE(Potent Fungus 56648, triggered) (no DoCastAOE
-- bridge). Joins the entry-verifiable-but-bridge-blocked queue.
-- spell_amanitar_potent_fungus — AuraScript (56648 Potent Fungus:
-- on-apply, if the target HasAura(Mini 57055) remove it and drop
-- the aura): no SpellScript/AuraScript binding bridge (razelikh
-- precedent) — documented-only.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent); BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.

local ENTRY_AMANITAR = 30258

local SPELL_BASH = 57094

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

-- C++ EVENT_BASH: JustEngagedWith-scheduled 10-14s init,
-- rescheduled 7-12s; DoCastVictim. UpdateAI returns early while
-- casting — the Go engine handles cast-gating, the timer just
-- re-arms.
local function bashTick(creature, guid)
    creature:CastSpell(nil, SPELL_BASH)
    schedule(guid, "bash", math.random(7000, 12000), function()
        bashTick(creature, guid)
    end)
end

local function amanitarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bash", math.random(10000, 14000), function()
        bashTick(creature, guid)
    end)
end

local function amanitarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function amanitarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function amanitarReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_AMANITAR, 1, amanitarEnterCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 2, amanitarLeaveCombat)
RegisterCreatureEvent(ENTRY_AMANITAR, 4, amanitarDied)
RegisterCreatureEvent(ENTRY_AMANITAR, 23, amanitarReset)

-- Elder Nadox (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_elder_nadox.cpp
-- (boss_elder_nadox). Ahn'kahet dungeon-script unit per
-- northrend_script_loader.cpp order (decl 32 / call 222, immediately
-- after AddSC_instance_gundrak(); next: boss_taldaram).
-- Entry: 29309 Elder Nadox (ahnkahet.h NPC_ELDER_NADOX — kalecgos
-- pass; the GetAhnKahetAI ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for all modeled arms):
-- boss_elder_nadox: JustEngagedWith Talk(SAY_AGGRO 0) (event 1);
-- EVENT_SUMMON_SWARMER DoCastSelf(Summon Swarmers 56119) + 33% Talk
-- (SAY_EGG_SAC 3), 10s init, 10s repeat (math.random for the C++ urand —
-- slad_ran precedent); KilledUnit Talk(SAY_SLAY 1) player-gated (event
-- 3, victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 2) (event 4).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_PLAGUE — DoCast(SelectTarget(Random, 0, 100, true),
-- SPELL_BROOD_PLAGUE 56130, triggered), 13s init, 15s repeat: the
-- random-target SelectTarget bridge is absent (cairne/kazzak
-- precedent), so the cast leg is unmodelable — documented-only.
-- EVENT_RAGE (DoCastSelf(H_SPELL_BROOD_RAGE 59465), 12s init, 10-50s
-- repeat) and EVENT_CHECK_ENRAGE (HasAura(SPELL_ENRAGE 26662) /
-- GetPositionZ() < 24.0f -> DoCastSelf(Enrage, triggered), 5s init,
-- 5s repeat) are both IsHeroic()-gated and there is no difficulty
-- bridge on the Lua surface (kelidan breaker precedent) — not
-- emulated even ungated, since normal-mode behavior must not fire
-- them; the map-height bridge for the Z check is absent anyway —
-- documented-only.
-- The HealthBelowPct(50) guardian-summon leg (Talk(EMOTE_HATCHES 4)
-- + DoCastSelf(SPELL_SUMMON_SWARM_GUARD 56120) + GuardianSummoned
-- latch) — no health-pct bridge on the Lua surface (doomwalker
-- precedent) — documented-only. SummonedCreatureDies(NPC_AHNKAHAR_
-- GUARDIAN 30176) -> GuardianDied latch and GetData(DATA_RESPECT_
-- YOUR_ELDERS) behind the absent cross-AI DoAction/GetData bridge
-- (zero hits) — documented-only.
-- npc_ahnkahar_nerubian — ENTRY UNVERIFIABLE (no NPC_AHNKAHAR_
-- NERUBIAN constant in ahnkahet.h or anywhere in the C++ sources;
-- the file enums name only the guardian 30176 / swarmer 30178 —
-- belnistrasz/willix precedent, no invented identifiers): zero
-- registration. The EVENT_SPRINT arm is PORT-PATTERN-READY but
-- entry-blocked: Reset-scheduled 13s init, DoCastSelf(Sprint 56354),
-- 20s repeat (the exact slad_ran RegisterCreatureEvent 1/23 pattern);
-- the HasUnitState(UNIT_STATE_CASTING) guard has no bridge but the
-- tick shape is unaffected.
-- spell_ahn_kahet_swarm (56159 — aura-stack bookkeeping around
-- SPELL_SWARM_BUFF 56281) — no SpellScript binding bridge on the
-- Lua surface (razelikh precedent) — documented-only.
-- achievement_respect_your_elders — AchievementCriteriaScript
-- (GetAI()->GetData(DATA_RESPECT_YOUR_ELDERS)) — no
-- achievement-criteria bridge on the Lua surface (snakes precedent)
-- + cross-AI GetData bridge absent — documented-only.

local ENTRY_ELDER_NADOX = 29309

local SAY_AGGRO   = 0
local SAY_SLAY    = 1
local SAY_DEATH   = 2
local SAY_EGG_SAC = 3

local SPELL_SUMMON_SWARMERS = 56119

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

-- C++ EVENT_SUMMON_SWARMER: DoCastSelf; 10s init, 10s repeat; 33%
-- Talk(SAY_EGG_SAC) on the same tick.
local function swarmerTick(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_SWARMERS)
    if math.random(1, 3) == 3 then
        creature:Talk(SAY_EGG_SAC)
    end
    schedule(guid, "swarmer", 10000, function()
        swarmerTick(creature, guid)
    end)
end

local function nadoxEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "swarmer", 10000, function()
        swarmerTick(creature, guid)
    end)
end

local function nadoxLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function nadoxTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function nadoxDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function nadoxReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ELDER_NADOX, 1, nadoxEnterCombat)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 2, nadoxLeaveCombat)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 3, nadoxTargetDied)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 4, nadoxDied)
RegisterCreatureEvent(ENTRY_ELDER_NADOX, 23, nadoxReset)

-- Trollgore (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_trollgore.cpp
-- (boss_trollgore (BossAI), npc_drakkari_invader (ScriptedAI),
-- spell_trollgore_consume (SpellScript 49380/59803),
-- spell_trollgore_corpse_explode (AuraScript 49555/59807),
-- spell_trollgore_invader_taunt (SpellScript 49405),
-- achievement_consumption_junction (AchievementCriteriaScript);
-- AddSC_boss_trollgore at end registers all via GetDrakTharonKeepAI /
-- RegisterSpellScript / new). The first Drak'Tharon Keep group in
-- northrend_script_loader.cpp order (decl 39 / call 234, immediately
-- after AddSC_instance_azjol_nerub(); next: AddSC_boss_novos()).
-- Entry: 26630 Trollgore (drak_tharon_keep.h NPC_TROLLGORE —
-- kalecgos pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_trollgore",
-- "npc_drakkari_invader", "spell_trollgore_consume",
-- "spell_trollgore_corpse_explode", "spell_trollgore_invader_taunt"
-- and "achievement_consumption_junction" hits boss_trollgore.cpp only
-- (loader carries only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim (moroes precedent). OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23) cancel the scheduler (gargolmar precedent); BossAI
-- _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for the modeled arms — the two
-- JustEngagedWith schedules are independent, none of the unmodeled
-- events gate them):
-- boss_trollgore: JustEngagedWith Talk(SAY_AGGRO 0) (event 1 — the
-- hook itself is bridged; its other legs — events.ScheduleEvent(
-- EVENT_CONSUME 15s) and EVENT_CORPSE_EXPLODE 3s (DoCastAOE 49380 /
-- 49555 — no DoCastAOE bridge, terestian/shazzrah precedent) +
-- EVENT_SPAWN 30-40s (invader-summoner trigger casts — ObjectAccessor
-- ::GetCreature(*me, instance->GetGuidData(DATA_TROLLGORE_INVADER_
-- SUMMONER_1+i)) + instance GUID-data + cross-AI bridges absent;
-- instance model absent anyway) — documented-only below);
-- EVENT_CRUSH DoCastVictim(Crush 49639), randtime(1s,5s) init,
-- 10-15s repeat (math.random — slad_ran precedent);
-- EVENT_INFECTED_WOUND DoCastVictim(Infected Wound 49637),
-- randtime(10s,60s) init, 25-35s repeat;
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 4) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_CONSUME — Talk(SAY_CONSUME 2) + DoCastAOE(Consume 49380),
-- 15s repeat — no DoCastAOE bridge (terestian/shazzrah precedent).
-- EVENT_CORPSE_EXPLODE — Talk(SAY_EXPLODE 3) + DoCastAOE(Corpse
-- Explode 49555), 15-19s repeat — no DoCastAOE bridge.
-- EVENT_SPAWN — the invader-summoner world-trigger casts (
-- trigger->CastSpell(trigger, RAND(SPELL_SUMMON_INVADER_A 49456 /
-- _B 49457 / _C 49458), me->GetGUID()) over the three
-- DATA_TROLLGORE_INVADER_SUMMONER_1..3 GUIDs), 30-40s repeat —
-- instance GetGuidData + ObjectAccessor cross-AI bridges absent +
-- instance model absent (standing).
-- The _consumptionJunction latch (Initialize/Reset true; UpdateAI
-- flips it false when GetAura(SPELL_CONSUME_BUFF_HELPER 49381 /
-- 59805)->GetStackAmount() > 9) — no aura-stack bridge; the
-- consumption-junction leg never reaches the Lua surface anyway.
-- GetData(DATA_CONSUMPTION_JUNCTION 1) — no cross-AI GetData bridge
-- (its only caller is achievement_consumption_junction — see below).
-- JustSummoned — summon->GetMotionMaster()->MovePoint(POINT_LANDING,
-- Landing) + summons.Summon — motion-master bridge absent (summon
-- STRAND absent — standing).
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) skip — no unit-state
-- bridge.
-- npc_drakkari_invader — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (27709 /
-- 27753 / 27754 — drak_tharon_keep.h NPC_DRAKKARI_INVADER_A/B/C,
-- kalecgos pass): zero registration. The whole AI is MovementInform(
-- POINT_MOTION_TYPE, POINT_LANDING) -> me->Dismount() +
-- SetImmuneToAll(false) + DoCastAOE(SPELL_INVADER_TAUNT 49405) —
-- motion-master bridge absent; dismount/immune-all bridges absent;
-- no DoCastAOE bridge (terestian/shazzrah precedent). Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_trollgore_consume (49380/59803 SpellScript: OnEffectHitTarget
-- EFFECT_1 SCRIPT_EFFECT -> target->CastSpell(GetCaster(),
-- SPELL_CONSUME_BUFF 49381, true)) / spell_trollgore_invader_taunt
-- (49405 SpellScript: OnEffectHitTarget SCRIPT_EFFECT ->
-- target->CastSpell(GetCaster(), GetEffectValue(), true)) /
-- spell_trollgore_corpse_explode (49555/59807 AuraScript:
-- OnEffectPeriodic(2nd tick) -> caster->CastSpell(GetTarget(),
-- SPELL_CORPSE_EXPLODE_DAMAGE 49618); AfterEffectRemove -> target
-- ToCreature DespawnOrUnsummon) — no SpellScript/AuraScript binding
-- bridge on the Lua surface (razelikh precedent) — documented-only.
-- achievement_consumption_junction — AchievementCriteriaScript
-- (target ToCreature -> AI()->GetData(DATA_CONSUMPTION_JUNCTION))
-- — no achievement-criteria bridge (snakes precedent) + cross-AI
-- GetData bridge absent — documented-only.

local ENTRY_TROLLGORE = 26630

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 4

local SPELL_CRUSH = 49639
local SPELL_INFECTED_WOUND = 49637

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

-- C++ EVENT_CRUSH: DoCastVictim; randtime(1s,5s) init, 10-15s repeat.
local function crushTick(creature, guid)
    creature:CastSpell(nil, SPELL_CRUSH)
    local delay = math.random(10000, 15000)
    schedule(guid, "crush", delay, function()
        crushTick(creature, guid)
    end)
end

-- C++ EVENT_INFECTED_WOUND: DoCastVictim; randtime(10s,60s) init,
-- 25-35s repeat.
local function infectedWoundTick(creature, guid)
    creature:CastSpell(nil, SPELL_INFECTED_WOUND)
    local delay = math.random(25000, 35000)
    schedule(guid, "infectedwound", delay, function()
        infectedWoundTick(creature, guid)
    end)
end

local function trollgoreEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "crush", math.random(1000, 5000), function()
        crushTick(creature, guid)
    end)
    schedule(guid, "infectedwound", math.random(10000, 60000), function()
        infectedWoundTick(creature, guid)
    end)
end

local function trollgoreLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function trollgoreTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function trollgoreDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function trollgoreReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_TROLLGORE, 1, trollgoreEnterCombat)
RegisterCreatureEvent(ENTRY_TROLLGORE, 2, trollgoreLeaveCombat)
RegisterCreatureEvent(ENTRY_TROLLGORE, 3, trollgoreTargetDied)
RegisterCreatureEvent(ENTRY_TROLLGORE, 4, trollgoreDied)
RegisterCreatureEvent(ENTRY_TROLLGORE, 23, trollgoreReset)

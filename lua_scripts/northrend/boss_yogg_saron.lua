-- Yogg-Saron (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_yogg_saron.cpp
-- (boss_voice_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_voice_of_yogg_saronAI> (BossAI),
-- BOSS_YOGG_SARON = 12); boss_sara (CreatureScript via
-- GetUlduarAI<boss_saraAI> (ScriptedAI)); boss_yogg_saron
-- (CreatureScript via GetUlduarAI<boss_yogg_saronAI>
-- (PassiveAI)); boss_brain_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_brain_of_yogg_saronAI> (PassiveAI));
-- fourteen npc_* scripts (npc_ominous_cloud /
-- npc_guardian_of_yogg_saron / npc_corruptor_tentacle /
-- npc_constrictor_tentacle / npc_crusher_tentacle /
-- npc_influence_tentacle / npc_descend_into_madness /
-- npc_immortal_guardian / npc_observation_ring_keeper /
-- npc_yogg_saron_keeper / npc_yogg_saron_illusions /
-- npc_garona / npc_turned_champion / npc_laughing_skull);
-- thirty-three spell_* scripts (all SpellScriptLoader, no
-- quoted names); all registered from inside
-- AddSC_boss_yogg_saron(); loader decl 122 / call 317 per
-- northrend_script_loader.cpp — the THIRTEENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_thorim() (decl 121 / call 316); the
-- checkpoint sequence (boss_thorim -> boss_yogg_saron) is
-- followed).
-- Entries: 33134 Sara (ulduar.h NPC_SARA, line 197);
-- 33288 Yogg-Saron (ulduar.h NPC_YOGG_SARON, line 82) —
-- both entry-verifiable, registrations proceed (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the eighteen quoted creature script names hits
-- boss_yogg_saron.cpp only (+ the loader decl/call lines
-- for AddSC_boss_yogg_saron); this clone carries no sql/
-- tree, so the ScriptName bindings are DB-side by
-- construction.
-- Audited artifact: this lua was ported 2026-10-02 09:40 in
-- e0f1acf and re-audited this run; the header below documents
-- that audit.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Sara Talk(SAY_SARA_AGGRO 2) (event 1;
-- the three ScheduleEvent legs (EVENT_SARAS_FERVOR /
-- EVENT_SARAS_BLESSING / EVENT_SARAS_ANGER) have no
-- bridges — the auriaya engage-port precedent; Sara's AI is
-- ScriptedAI, but the engage port lives on the Eluna
-- event-1 hook, not on the AI base).
-- JustDied — Yogg-Saron Talk(SAY_YOGG_SARON_DEATH 6)
-- (event 4; the cross-creature legs — Unit::Kill(voice),
-- sara/brain DisappearAndDie, keeper EnterEvadeMode — and
-- the player SPELL_SANITY / SPELL_INSANE aura removal have
-- no bridges — the sjonnir JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 33134 and 33288):
-- Sara KilledUnit — player-gated Talk(SAY_SARA_KILL 5)
-- (victim->GetTypeId() == TYPEID_PLAYER) AND
-- !me->IsInEvadeMode(); the player gate is bridgeable (the
-- razuvious precedent) but the evade gate is not — no
-- IsInEvadeMode in the Lua creature surface — so event 3
-- is NOT registered; a partial port would deviate from
-- C++ (yelling while evading), and C++-exactness wins.
-- Sara DamageTaken — lethal (damage >= health) is NOT a
-- death rewrite: damage = health - 1 (survive with 1 hp)
-- and Talk(SAY_SARA_TRANSFORM_1) fires only in PHASE_ONE
-- alongside cross-creature DoAction(ACTION_PHASE_TRANSFORM)
-- — no phase / DoAction bridges; the event-9 bridge
-- cannot model it — bridge-blocked.
-- Sara SpellHitTarget — Talk(SAY_SARA_FERVOR_HIT /
-- SAY_SARA_BLESSING_HIT / SAY_SARA_PSYCHOSIS_HIT) gated on
-- roll_chance_i(30) and !PHASE_TRANSFORM — no SpellHit
-- bridge.
-- Sara JustSummoned — Talk(SAY_SARA_DEATH_RAY) on
-- NPC_DEATH_ORB summon — no JustSummoned bridge.
-- Sara timer Talks — 2026-10-14 retraction re-audit: none ported on
-- combat-start timers; all six are phase-gated with unbridged schedule
-- legs (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a
-- real bridge; gating is the blocker, gothik precedent).
-- EVENT_TRANSFORM_1/2/3 (:821-827, Talk 7/8 + full-heal/DoAction arm)
-- — scheduled one-shot 4700/9500/14300ms from boss_saraAI::DamageTaken
-- (:732-734, PHASE_TRANSFORM) on lethal damage with a
-- damage=health-1 rewrite (:719-722), PHASE_ONE gate (:726), and
-- cross-creature voice->AI()->DoAction(ACTION_PHASE_TRANSFORM)
-- (:728-729); TRANSFORM_3 itself carries cross-creature
-- DoAction(ACTION_PHASE_TWO) legs (:831/:833) — no
-- DamageTaken / phase / DoAction bridges.
-- Yogg-Saron timer Talks — EVENT_YELL_BOW_DOWN (:993-995, Talk id 0,
-- one-shot 3s) fires from Reset (:935) on the voice's cross-creature
-- summon-driven Reset with SetPhase(PHASE_TWO) (:935-945; yogg is
-- PassiveAI, never enters combat) — no summon-driven-Reset bridge.
-- EVENT_SHADOW_BEACON (:996-1000, DoCastAOE 64465 + Emote 7, 45s
-- repeat) + EVENT_DEAFENING_ROAR (:1006-1011, DoCastAOE 64189 +
-- Talk 4/Emote 5, {20s,25s} repeat) — scheduled from cross-creature
-- DoAction(ACTION_PHASE_THREE) (:1024/:1027, PHASE_THREE), deafening
-- roar additionally gated on Is25ManRaid + instance DATA_KEEPERS_COUNT
-- (:1026) — no DoAction / instance-data bridges.
-- Yogg-Saron DoAction(ACTION_PHASE_THREE) —
-- Talk(SAY_YOGG_SARON_PHASE_3) — no DoAction bridge; joins
-- the no-DoAction-bridge queue.
-- Yogg-Saron SpellHit — Val'anyr loot-mode leg (no Talk) —
-- no SpellHit bridge.
-- Voice of Yogg-Saron (33280, ulduar.h line 203) —
-- JustEngagedWith has no Talk arm; JustDied has no Talk
-- arm (summons.Despawn + _JustDied() only); EnterEvadeMode
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) rides a
-- player-loop wipe leg (RemoveAurasDueToSpell SPELL_SANITY
-- / SPELL_INSANE) gated on PHASE_ONE — no wipe / aura /
-- phase bridges; timer Talks (cross-creature
-- EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE on
-- EVENT_EXTINGUISH_ALL_LIFE; EMOTE_YOGG_SARON_MADNESS +
-- SAY_YOGG_SARON_MADNESS on EVENT_ILLUSION) ride the
-- unbridgeable voice event machine (the mimiron
-- cross-creature precedent) — no registration.
-- Brain of Yogg-Saron (33890, ulduar.h line 218) —
-- DamageTaken has no Talk arm (aura/flag/DoAction legs
-- only); DoAction ACTION_INDUCE_MADNESS /
-- ACTION_TENTACLE_KILLED have no Talk arms — no
-- registration.
-- npc_observation_ring_keeper — Talk(SAY_KEEPER_CHOSEN_1/2,
-- player) gated on OnGossipSelect menuId 10333 — no
-- gossip-select bridge; no registration (observation-ring
-- keeper entries 33213 / 33241 / 33242 / 33244 entry-verifiable,
-- ulduar.h lines 199-202).
-- npc_yogg_saron_keeper (33410 / 33411 / 33412 / 33413,
-- ulduar.h lines 205-208) — no Talk arms — no registration.
-- npc_yogg_saron_illusions / npc_garona /
-- npc_turned_champion / npc_laughing_skull — roleplay
-- Talks (SAY_CHAMBER_ROLEPLAY_1-5 / SAY_ICECROWN_ROLEPLAY_1-6 /
-- SAY_STORMWIND_ROLEPLAY_1-7) fire from IsSummonedBy /
-- the illusion UpdateAI timer machine on instance
-- DATA_ILLUSION — no bridges; no registrations.
-- Tentacle / cloud / guardian creatures (33136 / 33292 /
-- 33943 / 33966 / 33983 / 33985 / 33988 / 34072, ulduar.h
-- lines 198 / 204 / 219 / 221 / 222 / 223 / 224 / 226) —
-- no bridgeable Talk arms — no registrations.
-- The Talk arms inside SpellScripts / AuraScripts —
-- Talk(EMOTE_OMINOUS_CLOUD_PLAYER_TOUCH) in
-- spell_yogg_saron_boil_ominously (HandleDummy);
-- Talk(WHISPER_VOICE_INSANE) in spell_yogg_saron_insane
-- (OnApply) — no SpellScript / AuraScript bridges; all
-- thirty-three spell scripts join the no-SpellScript /
-- no-AuraScript-bridge queues.

local ENTRY_SARA = 33134
local ENTRY_YOGG_SARON = 33288

local SAY_SARA_AGGRO = 2
local SAY_YOGG_SARON_DEATH = 6

-- C++ JustEngagedWith (boss_saraAI, ScriptedAI):
-- Talk(SAY_SARA_AGGRO) + three ScheduleEvent legs — only
-- the Talk arm is bridgeable; the auriaya engage-port
-- precedent.
local function saraEnterCombat(event, creature, target)
    creature:Talk(SAY_SARA_AGGRO)
end

-- C++ JustDied (boss_yogg_saronAI, PassiveAI):
-- Talk(SAY_YOGG_SARON_DEATH), then cross-creature kill /
-- disappear / evade legs and player sanity-aura removal —
-- only the Talk arm is bridgeable; the sjonnir
-- JustDied-Talk precedent.
local function yoggSaronDied(event, creature, killer)
    creature:Talk(SAY_YOGG_SARON_DEATH)
end

RegisterCreatureEvent(ENTRY_SARA, 1, saraEnterCombat)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 4, yoggSaronDied)

-- Timer-driven cast arms (the maiden CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23;
-- CastSpell(creature, spell) = DoCastAOE/DoCastSelf self-cast,
-- CastSpell(nil, spell) = DoCastVictim victim-cast; random targets via
-- GetPlayersInWorld — the baltharus/thorim precedents). All four arms are
-- ungated: no phase args on their schedules, no DoAction/MovementInform
-- scheduling, no instance legs. The blocked arms — all of Sara's
-- EVENT_SARAS_* timer casts (phase-arg PHASE_ONE + spellvalue
-- target-selector legs on no-bridge SpellScripts), every voice-of-Yogg-Saron
-- timer arm (instance-GUID/phase legs), every Yogg-Saron body timer arm
-- (PHASE_TWO/PHASE_THREE-gated), the keeper arms (DoAction/phase-gated) —
-- stay documented-only in the header above (the gothik precedent).
local ENTRY_GUARDIAN_OF_YOGG_SARON = 33136
local ENTRY_CORRUPTOR_TENTACLE = 33985
local ENTRY_CRUSHER_TENTACLE = 33966
local ENTRY_IMMORTAL_GUARDIAN = 33988

local SPELL_DARK_VOLLEY = 63038
local SPELL_SHADOW_NOVA = 62714
local SPELL_SHADOW_NOVA_2 = 65719
local SPELL_BLACK_PLAGUE = 64153
local SPELL_CURSE_OF_DOOM = 64157
local SPELL_APATHY = 64156
local SPELL_DRAINING_POISON = 64152
local SPELL_DIMINISH_POWER = 64145
local SPELL_DIMINSH_POWER = 64148
local SPELL_CRUSH = 64146
local SPELL_TENTACLE_VOID_ZONE = 64017
local SPELL_FOCUSED_ANGER = 57688
local SPELL_ERUPT = 64144
local SPELL_EMPOWERED = 64161
local SPELL_RECENTLY_SPAWNED = 64497
local SPELL_DRAIN_LIFE = 64159

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

local function randomPlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ npc_guardian_of_yogg_saronAI::EVENT_DARK_VOLLEY:
-- DoCastAOE(SPELL_DARK_VOLLEY 63038) (non-triggered), {10s,15s} init,
-- {10s,15s} repeat — scheduled in Reset with no phase arg (:1198,
-- :1215-1218 verified).
local function darkVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_VOLLEY)
    schedule(guid, "darkvolley", {10000, 15000}, function()
        darkVolleyTick(creature, guid)
    end)
end

local function guardianEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "darkvolley", {10000, 15000}, function()
        darkVolleyTick(creature, guid)
    end)
end

-- C++ npc_corruptor_tentacleAI::EVENT_CAST_RANDOM_SPELL: random-player
-- DoCast(target, RAND(SPELL_BLACK_PLAGUE 64153, SPELL_CURSE_OF_DOOM
-- 64157, SPELL_APATHY 64156, SPELL_DRAINING_POISON 64152))
-- (non-triggered), 1ms init, 3s repeat — scheduled in Reset with no phase
-- arg; SelectTarget(Random) carries no C++ range so any alive player on
-- the same map+instance is eligible (:1264, :1284-1288 verified). The
-- Reset-time DoCast(me, SPELL_TENTACLE_VOID_ZONE) + DoCastAOE(SPELL_ERUPT)
-- legs are ported on event 23 (corruptorReset).
local function corruptorTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        local spells = { SPELL_BLACK_PLAGUE, SPELL_CURSE_OF_DOOM, SPELL_APATHY, SPELL_DRAINING_POISON }
        creature:CastSpell(target, spells[math.random(#spells)])
    end
    schedule(guid, "randomspell", 3000, function()
        corruptorTick(creature, guid)
    end)
end

local function corruptorEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "randomspell", 1, function()
        corruptorTick(creature, guid)
    end)
end

-- C++ npc_corruptor_tentacleAI::Reset (:1260-1265): non-triggered
-- one-shots DoCast(me, SPELL_TENTACLE_VOID_ZONE 64017) +
-- DoCastAOE(SPELL_ERUPT 64144) ride event 23.
local function corruptorReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
    creature:CastSpell(creature, SPELL_ERUPT)
end

-- C++ npc_crusher_tentacleAI::EVENT_DIMINISH_POWER:
-- DoCast(SPELL_DIMINISH_POWER 64145) (non-triggered self-cast), {6s,8s}
-- init, {20s,30s} repeat — scheduled in Reset with no phase arg (:1370,
-- :1389-1392 verified). (SPELL_DIMINSH_POWER 64148 is a different enum
-- entry — the typo'd name — cast once in Reset, see crusherReset.)
local function diminishPowerTick(creature, guid)
    creature:CastSpell(creature, SPELL_DIMINISH_POWER)
    schedule(guid, "diminishpower", {20000, 30000}, function()
        diminishPowerTick(creature, guid)
    end)
end

local function crusherEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "diminishpower", {6000, 8000}, function()
        diminishPowerTick(creature, guid)
    end)
end

-- C++ npc_crusher_tentacleAI::Reset (:1362-1368): five non-triggered
-- one-shots — DoCast(me, SPELL_CRUSH 64146), DoCast(me,
-- SPELL_TENTACLE_VOID_ZONE 64017), DoCast(me, SPELL_DIMINSH_POWER 64148),
-- DoCast(me, SPELL_FOCUSED_ANGER 57688), DoCastAOE(SPELL_ERUPT 64144) —
-- ride event 23 (halion Reset one-shot precedent); the SetCombatMovement
-- leg has no bridge.
local function crusherReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_CRUSH)
    creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
    creature:CastSpell(creature, SPELL_DIMINSH_POWER)
    creature:CastSpell(creature, SPELL_FOCUSED_ANGER)
    creature:CastSpell(creature, SPELL_ERUPT)
end

-- C++ npc_immortal_guardianAI::EVENT_DRAIN_LIFE:
-- DoCast(SPELL_DRAIN_LIFE 64159) (non-triggered self-cast), {3s,13s} init,
-- {20s,30s} repeat — scheduled in Reset with no phase arg (:1491,
-- :1514-1517 verified).
local function drainLifeTick(creature, guid)
    creature:CastSpell(creature, SPELL_DRAIN_LIFE)
    schedule(guid, "drainlife", {20000, 30000}, function()
        drainLifeTick(creature, guid)
    end)
end

local function immortalEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "drainlife", {3000, 13000}, function()
        drainLifeTick(creature, guid)
    end)
end

-- C++ npc_immortal_guardianAI::Reset (:1489-1493): non-triggered
-- one-shots DoCast(me, SPELL_EMPOWERED 64161) + DoCast(me,
-- SPELL_RECENTLY_SPAWNED 64497) ride event 23; the DamageTaken 1% floor
-- has no health bridge.
local function immortalReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_EMPOWERED)
    creature:CastSpell(creature, SPELL_RECENTLY_SPAWNED)
end

local function cancelAll(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_guardian_of_yogg_saronAI::JustDied (:1190-1194): non-triggered
-- DoCastAOE(SPELL_SHADOW_NOVA 62714) + DoCastAOE(SPELL_SHADOW_NOVA_2
-- 65719) — self-cast form per the darkVolleyTick DoCastAOE convention.
local function guardianDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_SHADOW_NOVA)
    creature:CastSpell(creature, SPELL_SHADOW_NOVA_2)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 1, guardianEnterCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 4, guardianDied)
RegisterCreatureEvent(ENTRY_GUARDIAN_OF_YOGG_SARON, 23, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 1, corruptorEnterCombat)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CORRUPTOR_TENTACLE, 23, corruptorReset)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 1, crusherEnterCombat)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CRUSHER_TENTACLE, 23, crusherReset)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 1, immortalEnterCombat)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 2, cancelAll)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 4, cancelAll)
RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 23, immortalReset)

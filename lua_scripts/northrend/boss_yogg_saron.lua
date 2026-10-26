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
-- Reset — DoCast(me, SPELL_SHADOWY_BARRIER_YOGG 63894) +
-- DoCast(me, SPELL_KNOCK_AWAY 64022), both non-triggered and ungated
-- (:936-937), ported on event 23 (the urom evocate-reset precedent);
-- the _events phase/timer and loot-mode legs stay documented-only.
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 33134 and 33288), except the Sara KilledUnit below —
-- player-gated AND !IsInEvadeMode-gated Talk(SAY_SARA_KILL 5),
-- PORTED on event 3 (it was documented-only until the IsInEvadeMode
-- bridge was re-verified; C++-exact):
-- Sara DamageTaken — PORTED (saraDamageTaken, event 9): lethal
-- (damage >= health) is NOT a death rewrite: damage = health - 1
-- (survive with 1 hp), unconditional in C++; the first-lethal
-- PHASE_ONE chain (Talk 6 + 4700/9500/14300/14500ms Talks, full heal,
-- faction, phase-2 transform, barrier) rides the same handler, with
-- only the cross-creature DoAction legs staying out (see the handler).
-- Sara SpellHitTarget — Talk(SAY_SARA_FERVOR_HIT /
-- SAY_SARA_BLESSING_HIT / SAY_SARA_PSYCHOSIS_HIT) gated on
-- roll_chance_i(30) and !PHASE_TRANSFORM — stays out on trigger-path
-- grounds: the EVENT_SARAS_FERVOR / BLESSING / ANGER casts are unported
-- and the phase gate has no bridge, so event 15 never fires for them in
-- Go (event 15 itself is live — boss_ai.go:1904).
-- Sara JustSummoned — Talk(SAY_SARA_DEATH_RAY) on NPC_DEATH_ORB
-- summon + self-cast SPELL_DEATH_RAY_ORIGIN_VISUAL are PORTED
-- (saraJustSummoned; the orb arrives via a spell summon leg, unported
-- — LATENT). The 4 NPC_DEATH_RAY summons in the same arm stay out
-- (map GetHeight terrain bridge absent — frost-sphere precedent);
-- the voice-of-Yogg forward stays out (cross-AI, no bridge).
-- Sara Reset — PORTED (saraReset, event 23): RemoveAllAuras +
-- SetReactState(REACT_PASSIVE) + SetFaction(FACTION_FRIENDLY)
-- (:783-790); only the _events Reset/SetPhase(PHASE_ONE) legs stay
-- unmodeled (phase machine, no bridge).
-- Voice JustSummoned — the triggered one-shot legs are PORTED
-- (voiceJustSummoned, event 19): constrictor Lunge, descend portal
-- visual, immortal simple teleport (all triggered), Yogg emerge emote.
-- The crusher/corruptor REACT_PASSIVE (:660-665) + StartAttackEvent
-- attack kick (:430-439, 1s/5s delayed BasicEvent) stay out: the
-- passive arm without the AttackStart kick would strand the summons,
-- and AttackStart has no bridge (DoZoneInCombat seeds whole-map threat,
-- not C++'s single-target AttackStart).
-- Sara timer Talks — 2026-10-14 retraction re-audit: none ported on
-- combat-start timers; all six are phase-gated with unbridged schedule
-- legs (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a
-- real bridge; gating is the blocker, gothik precedent).
-- EVENT_TRANSFORM_1/2/3 (:821-827, Talk 7/8 + full-heal/DoAction arm)
-- — PORTED in self-arm form (saraDamageTaken, event 9): the
-- damage=health-1 rewrite (:719-722), PHASE_ONE first-lethal latch,
-- and the 4700/9500/14300/14500ms Talk/heal/faction/transform/barrier
-- legs; only the cross-creature voice->AI()->DoAction(
-- ACTION_PHASE_TRANSFORM) (:728-729) and TRANSFORM_3's
-- DoAction(ACTION_PHASE_TWO) legs (:831/:833) stay out (no DoAction
-- bridge).
-- Yogg-Saron timer Talks — EVENT_YELL_BOW_DOWN (:993-995, Talk id 0,
-- one-shot 3s) fires from Reset (:935) on the voice's cross-creature
-- summon-driven Reset with SetPhase(PHASE_TWO) (:935-945; yogg is
-- PassiveAI, never enters combat) — no summon-driven-Reset bridge.
-- EVENT_SHADOW_BEACON (:996-1000, DoCastAOE 64465 + Emote 7, 45s
-- repeat) + EVENT_DEAFENING_ROAR (:1006-1011, DoCastAOE 64189 +
-- Talk 4/Emote 5, {20s,25s} repeat) — scheduled from cross-creature
-- DoAction(ACTION_PHASE_THREE) (:1024/:1027, PHASE_THREE), deafening
-- roar additionally gated on Is25ManRaid + instance DATA_KEEPERS_COUNT
-- (:1026) — no DoAction / instance-data bridges (difficulty leg now
-- bridged, lua_creature_events.go, stranded on DoAction + instance).
-- Yogg-Saron DoAction(ACTION_PHASE_THREE) —
-- Talk(SAY_YOGG_SARON_PHASE_3) — no DoAction bridge; joins
-- the no-DoAction-bridge queue.
-- Yogg-Saron SpellHit — Val'anyr loot-mode leg (no Talk) —
-- PORTED via event 14 (yoggSaronSpellHit below); the Reset
-- ResetLootMode leg (:939) rides the unbridged scheduler
-- (ResetLootMode itself IS bridged, lua_objects.go:1003; the
-- ladder is blocked on keepers instance-data, not the bridge).
-- Voice of Yogg-Saron (33280, ulduar.h line 203) —
-- PORTED: JustEngagedWith DoCast(me, SPELL_SANITY_PERIODIC 63786)
-- (2-arg self, non-triggered, ungated, :525) + the 15s
-- EVENT_LOCK_DOOR DoCast(me, SPELL_INSANE_PERIODIC 64554)
-- one-shot (:556-557, no phase arg on the schedule) — both ride
-- event 1 with the SetInCombatWith keeper / DoCastAOE(
-- SPELL_SUMMON_GUARDIAN_2, spellvalue) / EVENT_EXTINGUISH_ALL_LIFE /
-- EVENT_SUMMON_GUARDIAN_OF_YOGG_SARON schedule legs left
-- documented-only (no instance / summon / spellvalue bridges —
-- the assembly_of_iron keeper-combat precedent); JustDied has no
-- Talk arm (summons.Despawn + _JustDied() only); EnterEvadeMode
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) rides a
-- player-loop wipe leg (RemoveAurasDueToSpell SPELL_SANITY
-- / SPELL_INSANE — PORTED via GetPlayersInWorld +
-- player:RemoveAura in yoggSaronReset/yoggSaronDied; the removals
-- are ungated in C++, only the whisper is PHASE_ONE-gated, and the
-- whisper keeps no phase bridge); timer Talks (cross-creature
-- EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE on
-- EVENT_EXTINGUISH_ALL_LIFE; EMOTE_YOGG_SARON_MADNESS +
-- SAY_YOGG_SARON_MADNESS on EVENT_ILLUSION) ride the
-- unbridgeable voice event machine (the mimiron
-- cross-creature precedent).
-- Brain of Yogg-Saron (33890, ulduar.h line 218) — Reset
-- DoCast(me, SPELL_MATCH_HEALTH 64066) PORTED below (event 23);
-- DamageTaken below-30% cascade (:1068-1093): DoCastAOE(
-- SPELL_SHATTERED_ILLUSION_REMOVE 65238, triggered) + DoCast(me,
-- SPELL_MATCH_HEALTH_2 64069, triggered) + DoCast(me,
-- SPELL_BRAIN_HURT_VISUAL 64361, triggered) — stays out with the
-- InterruptNonMeleeSpells / cross-creature DoAction / instance legs
-- (no bridges; the HasAura(64361) gate is bridged but the cascade
-- cannot be split); DoAction ACTION_INDUCE_MADNESS /
-- ACTION_TENTACLE_KILLED have no Talk arms.
-- npc_observation_ring_keeper — OnGossipSelect PORTED below
-- (observationKeeperGossipSelect): menuId 10333 never reaches the
-- Lua select hook (sender/action only — gossip.go:351), so the gate
-- uses the sender == 0 DB-menu proxy (akama precedent); the keeper
-- class defines no hello/quest menu of its own. Reset one-shots
-- PORTED below (event 23); ring-keeper entries 33213 / 33241 /
-- 33242 / 33244, ulduar.h lines 199-202.
-- npc_yogg_saron_keeper (33410 / 33411 / 33412 / 33413,
-- ulduar.h lines 205-208) — no Talk arms — Reset + JustEnteredCombat
-- one-shots PORTED below (events 23 / 1).
-- npc_yogg_saron_illusions / npc_garona /
-- npc_turned_champion / npc_laughing_skull — roleplay
-- Talks (SAY_CHAMBER_ROLEPLAY_1-5 / SAY_ICECROWN_ROLEPLAY_1-6 /
-- SAY_STORMWIND_ROLEPLAY_1-7) fire from IsSummonedBy /
-- the illusion UpdateAI timer machine on instance
-- DATA_ILLUSION — no bridges; no registrations (no RegisterLuaBoss
-- for 33437 / 33441 / 33442 / 33495 / 33523 / 33535 / 33962;
-- creature:FindNearestCreature is live (lua_creature_events.go:1237)
-- but has no handler to attach to). The FindNearestCreature legs
-- stay out on those grounds: boss_yogg_saron.cpp:1783-1818
-- (immolated-champion -> Lich King DeathGrasp relay, Lich King /
-- Llane roleplay Talks) and :1877-1882 (garona FindNearestCreature(
-- NPC_KING_LLANE, 50.0f) + DoCast(SPELL_ASSASSINATE) +
-- llane->CastSpell(llane, SPELL_PERMANENT_FEIGN_DEATH)). (Turned
-- champion's Reset DoCast(SPELL_VERTEX_COLOR_BLACK 39662)
-- one-shot is PORTED below — event 23 — not roleplay-gated.)
-- Tentacle / cloud / guardian creatures (33136 / 33292 /
-- 33943 / 33966 / 33983 / 33985 / 33988 / 34072, ulduar.h
-- lines 198 / 204 / 219 / 221 / 222 / 223 / 224 / 226) —
-- guardian (33136) / corruptor (33985) / crusher (33966) /
-- immortal (33988) timer + Reset arms registered below;
-- constrictor (33983) Reset one-shots DoCast(me,
-- SPELL_TENTACLE_VOID_ZONE_2 64384) + DoCastAOE(SPELL_ERUPT
-- 64144), influence (33943) Reset entry-conditional DoCast(me,
-- 64013/64010), ominous cloud (33292) Reset DoCast(me,
-- SPELL_OMINOUS_CLOUD_VISUAL 63084) PORTED below (event 23).
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
local SAY_SARA_KILL = 5
local SAY_SARA_DEATH_RAY = 10 -- boss_yogg_saron.cpp:49
local ENTRY_DEATH_ORB = 33882 -- ulduar.h:217
local ENTRY_DEATH_RAY = 33881 -- ulduar.h:216
local SPELL_DEATH_RAY_ORIGIN_VISUAL = 63893 -- boss_yogg_saron.cpp:230
local SPELL_DEATH_RAY_WARNING_VISUAL = 63882 -- boss_yogg_saron.cpp:890
local SAY_YOGG_SARON_DEATH = 6

local SPELL_SHADOWY_BARRIER_YOGG = 63894
local SPELL_KNOCK_AWAY = 64022
local SPELL_IN_THE_MAWS_OF_THE_OLD_GOD = 64184
local SPELL_SANITY = 63050
local SPELL_INSANE = 63120

local REACT_PASSIVE = 0 -- UnitDefines.h:340

-- C++ JustEngagedWith (boss_saraAI, ScriptedAI):
-- Talk(SAY_SARA_AGGRO) + three ScheduleEvent legs — only
-- the Talk arm is bridgeable; the auriaya engage-port
-- precedent.
local function saraEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_yogg_saron.cpp:792: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:Talk(SAY_SARA_AGGRO)
end

-- C++ boss_saraAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER && !me->IsInEvadeMode())
--     Talk(SAY_SARA_KILL) — both gates bridgeable.
local function saraKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" and not creature:IsInEvadeMode() then
        creature:Talk(SAY_SARA_KILL)
    end
end

-- C++ boss_yogg_saronAI::EnterEvadeMode (:478-490) + JustDied (:973-981):
-- player-loop RemoveAurasDueToSpell(SPELL_SANITY) /
-- RemoveAurasDueToSpell(SPELL_INSANE), ungated in both (no alive /
-- distance gate in C++). Called from ev23 (EnterEvadeMode; ev23 also
-- pre-fires on the Go death path, covering JustDied) and ev4
-- (JustDied, belt-and-braces — the removal is idempotent). The
-- respawn-path ev23 over-fire is a no-op: players never carry these
-- auras outside the fight. The PHASE_ONE-gated
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) has no phase bridge and
-- stays documented-only.
local function clearSanityAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_SANITY)
            p:RemoveAura(SPELL_INSANE)
        end
    end
end

-- C++ JustDied (boss_yogg_saronAI, PassiveAI):
-- Talk(SAY_YOGG_SARON_DEATH), then cross-creature kill /
-- disappear / evade legs and player sanity-aura removal —
-- only the Talk arm is bridgeable; the sjonnir
-- JustDied-Talk precedent.
local function yoggSaronDied(event, creature, killer)
    creature:Talk(SAY_YOGG_SARON_DEATH)
    clearSanityAuras(creature)
end

-- C++ boss_yogg_saronAI::Reset :931-945 — DoCast(me,
-- SPELL_SHADOWY_BARRIER_YOGG) + DoCast(me, SPELL_KNOCK_AWAY), both
-- non-triggered and ungated (the urom evocate-reset precedent); the
-- _events Reset/SetPhase(PHASE_TWO)/ScheduleEvent(EVENT_YELL_BOW_DOWN)
-- and phase/timer legs ride unbridged phase/timer machines (the
-- gothik phase-gating precedent; the loot-mode bridge is live —
-- keepers instance-data is the blocker, not the bridge).
local function yoggSaronReset(event, creature)
    -- C++ boss_yogg_saronAI::Reset(): DoCast(me, SHADOWY_BARRIER_YOGG);
    -- DoCast(me, KNOCK_AWAY) (boss_yogg_saron.cpp:931-937). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOWY_BARRIER_YOGG)
        creature:CastSpell(creature, SPELL_KNOCK_AWAY)
    end
    clearSanityAuras(creature)
end

RegisterCreatureEvent(ENTRY_SARA, 1, saraEnterCombat)
RegisterCreatureEvent(ENTRY_SARA, 3, saraKilledUnit)
-- C++ boss_saraAI::JustSummoned (:870-895): every summon
-- SetReactState(REACT_PASSIVE); NPC_DEATH_ORB -> Talk + self-cast
-- (non-triggered, C++ :877) in C++ order; NPC_DEATH_RAY -> self-cast
-- SPELL_DEATH_RAY_WARNING_VISUAL (non-triggered, C++ :890). LATENT: orbs spawn only
-- from the unported SPELL_DEATH_RAY cast (anubrekhan precedent).
local function saraJustSummoned(_, creature, summoned)
    summoned:SetReactState(0) -- REACT_PASSIVE
    if summoned:GetEntry() == ENTRY_DEATH_ORB then
        creature:Talk(SAY_SARA_DEATH_RAY)
        summoned:CastSpell(summoned, SPELL_DEATH_RAY_ORIGIN_VISUAL)
    elseif summoned:GetEntry() == ENTRY_DEATH_RAY then
        -- C++ boss_saraAI::JustSummoned (:889-890): non-triggered self-cast.
        summoned:CastSpell(summoned, SPELL_DEATH_RAY_WARNING_VISUAL)
    end
end
RegisterCreatureEvent(ENTRY_SARA, 19, saraJustSummoned)
-- C++ SpellHit :951-956 — Val'anyr: SPELL_IN_THE_MAWS_OF_THE_OLD_GOD
-- (64184) hit -> AddLootMode(32). Pure loot-mode leg, no Talk arm.
local function yoggSaronSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_IN_THE_MAWS_OF_THE_OLD_GOD then
        creature:AddLootMode(32)
    end
end

RegisterCreatureEvent(ENTRY_YOGG_SARON, 4, yoggSaronDied)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 14, yoggSaronSpellHit)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 23, yoggSaronReset)

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
-- stay documented-only in the header above (the gothik precedent);
-- voice EVENT_LOCK_DOOR + JustEngagedWith one-shots, constrictor /
-- influence / ominous-cloud / brain / turned-champion / observation-ring
-- and yogg-keeper Reset one-shots, and keeper JustEnteredCombat
-- entry-gated one-shots are ungated (no phase args, no DoAction /
-- phase-machine legs) and ride event 1/23 below.
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

-- C++ SelectTarget(Random) draws the threat list (UnitAI.h), never the
-- whole instance player pool — GetAITargets is the bridged threat list
-- (midnight precedent); SelectTargetList skips offline refs
-- (UnitAI.h), so the dead entries Go GetAITargets still carries
-- are filtered here.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
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
-- arg; SelectTarget(Random) draws the threat list (GetAITargets bridge),
-- not the map-wide player pool (:1264, :1284-1288 verified).
-- The Reset-time DoCast(me, SPELL_TENTACLE_VOID_ZONE) + DoCastAOE(SPELL_ERUPT)
-- legs are ported on event 23 (corruptorReset).
local function corruptorTick(creature, guid)
    local target = randomThreatTarget(creature)
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
    -- C++ npc_corruptor_tentacleAI ctor (:1257): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ npc_corruptor_tentacleAI::Reset(): DoCast(me, TENTACLE_VOID_ZONE);
    -- DoCastAOE(ERUPT) (boss_yogg_saron.cpp). Alive-gated; cancelTimers
    -- rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
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
-- (false) ctor arm (:1359) rides the same handler below.
local function crusherReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_crusher_tentacleAI ctor (:1359): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ npc_crusher_tentacleAI::Reset(): DoCast(me, CRUSH/TENTACLE_VOID_ZONE/
    -- DIMINSH_POWER/FOCUSED_ANGER); DoCastAOE(ERUPT) (boss_yogg_saron.cpp).
    -- Alive-gated; cancelTimers rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_CRUSH)
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE)
        creature:CastSpell(creature, SPELL_DIMINSH_POWER)
        creature:CastSpell(creature, SPELL_FOCUSED_ANGER)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
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
-- SPELL_RECENTLY_SPAWNED 64497) ride event 23; the DamageTaken 1%
-- floor is ported on event 9 below (the old "no health bridge"
-- claim was stale — creature:GetHealth/GetMaxHealth are bridged,
-- object.go fallback, sindragosa precedent).
local function immortalReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_immortal_guardianAI::Reset(): DoCast(me, EMPOWERED);
    -- DoCast(me, RECENTLY_SPAWNED) (boss_yogg_saron.cpp). Alive-gated;
    -- cancelTimers rides the death leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_EMPOWERED)
        creature:CastSpell(creature, SPELL_RECENTLY_SPAWNED)
    end
end

-- C++ npc_immortal_guardianAI::DamageTaken (:1494-1499): the 1% floor —
-- if (HealthBelowPctDamaged(1, damage)) damage = GetHealth() -
-- CountPctFromMaxHealth(1). PORTED 2026-10-17 via event 9's second-return
-- damage rewrite (volazj event-9 precedent; midnight/curator/nightbane
-- "return false, newDamage" convention): HealthBelowPctDamaged is the
-- int64 compare (int64(health)-int64(damage) < int64(1% of max),
-- Unit.h:873); CountPctFromMaxHealth(1) = math.floor(maxHealth/100)
-- per the nightbane CalculatePct convention. The C++ uint32 wrap when
-- health < 1% is unreachable — the floor pins health at exactly 1%.
local function immortalDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    local newDamage = damage
    local onePct = math.floor(maxHealth / 100)
    if health - damage < onePct then
        newDamage = health - onePct
    end
    return false, newDamage
end

RegisterCreatureEvent(ENTRY_IMMORTAL_GUARDIAN, 9, immortalDamageTaken)

local function cancelAll(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_saraAI::DamageTaken (:719-740): lethal damage is NOT a death
-- rewrite — damage = health - 1 (survive with 1 hp), unconditionally (the
-- rewrite sits outside the PHASE_ONE gate); in PHASE_ONE only, the
-- lethal hit also fires Talk(SAY_SARA_TRANSFORM_1 6) and schedules the
-- one-shot transform chain (:732-735, 4700/9500/14300/14500ms). The
-- chain's cross-creature legs — voice DoAction(ACTION_PHASE_TRANSFORM)
-- on the lethal hit, voice/mimiron DoAction(ACTION_PHASE_TWO) and the
-- SPELL_RIDE_YOGG_SARON_VEHICLE cast on Yogg at TRANSFORM_3/4 — have no
-- DoAction/cross-creature bridges and stay out; the phase-2 schedule
-- legs (EVENT_DEATH_RAY/MALADY/PSYCHOSIS/BRAIN_LINK, :842-845) ride the
-- unported phase machine and stay out. DamageTaken arms fire on lethal
-- hits, so no postDamage>0 guard (immortal precedent). The latch keeps
-- the one-shot chain first-lethal-only: C++ re-arms nothing once the
-- phase has left PHASE_ONE.
local SAY_SARA_TRANSFORM_1 = 6
local SAY_SARA_TRANSFORM_2 = 7
local SAY_SARA_TRANSFORM_3 = 8
local SAY_SARA_TRANSFORM_4 = 9
local SPELL_FULL_HEAL = 43978
local SPELL_PHASE_2_TRANSFORM = 65157
local SPELL_SHADOWY_BARRIER_SARA = 64775
local FACTION_FRIENDLY = 35
local FACTION_MONSTER_2 = 16
local saraTransformed = {}

local function saraDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local newDamage = damage
    if health > 0 and damage >= health then
        newDamage = health - 1
        local guid = creature:GetGUID()
        if not saraTransformed[guid] then
            saraTransformed[guid] = true
            creature:Talk(SAY_SARA_TRANSFORM_1)
            schedule(guid, "saratransform1", 4700, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_2)
                end
            end)
            schedule(guid, "saratransform2", 9500, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_3)
                end
            end)
            schedule(guid, "saratransform3", 14300, function()
                if creature:IsAlive() then
                    creature:Talk(SAY_SARA_TRANSFORM_4)
                    creature:CastSpell(creature, SPELL_FULL_HEAL)
                    creature:SetFaction(FACTION_MONSTER_2)
                end
            end)
            schedule(guid, "saratransform4", 14500, function()
                if creature:IsAlive() then
                    creature:CastSpell(creature, SPELL_PHASE_2_TRANSFORM)
                    creature:CastSpell(creature, SPELL_SHADOWY_BARRIER_SARA)
                end
            end)
        end
    end
    return false, newDamage
end

local function saraCancelTransform(event, creature)
    cancelTimers(creature:GetGUID())
    saraTransformed[creature:GetGUID()] = nil
end

-- C++ boss_saraAI::Reset (:783-790): RemoveAllAuras + SetReactState
-- (REACT_PASSIVE) + SetFaction(FACTION_FRIENDLY), all bridged; the
-- _events Reset/SetPhase(PHASE_ONE) legs ride the unported phase
-- machine. Reset also cancels the transform chain (C++ _events.Reset).
local function saraReset(event, creature)
    saraCancelTransform(event, creature)
    creature:RemoveAllAuras()
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:SetFaction(FACTION_FRIENDLY)
end

RegisterCreatureEvent(ENTRY_SARA, 9, saraDamageTaken)
RegisterCreatureEvent(ENTRY_SARA, 2, saraCancelTransform)
RegisterCreatureEvent(ENTRY_SARA, 4, saraCancelTransform)
RegisterCreatureEvent(ENTRY_SARA, 23, saraReset)

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

-- ===== Slice-E ungated one-shot ports (2026-10-15) =====
-- All arms below are ungated in C++: no phase args on their
-- schedules, no DoAction/MovementInform/phase-machine legs, no
-- instance legs — the stale "no-timer bridge" / "no random-target
-- bridge" / DoCastAOE doctrines retracted under the
-- CreateLuaEvent / DoCastAOE≡self-cast conventions (the gothik
-- phase-gating precedent).

local ENTRY_VOICE_OF_YOGG_SARON = 33280
local ENTRY_CONSTRICTOR_TENTACLE = 33983
local ENTRY_INFLUENCE_TENTACLE = 33943
local ENTRY_SUIT_OF_ARMOR = 33433
local ENTRY_OMINOUS_CLOUD = 33292
local ENTRY_BRAIN_OF_YOGG_SARON = 33890
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local ENTRY_TURNED_CHAMPION = 33962
local ENTRY_OBSERVATION_RING_KEEPER_HODIR = 33213
local ENTRY_OBSERVATION_RING_KEEPER_FREYA = 33241
local ENTRY_OBSERVATION_RING_KEEPER_THORIM = 33242
local ENTRY_OBSERVATION_RING_KEEPER_MIMIRON = 33244
local ENTRY_KEEPER_FREYA = 33410
local ENTRY_SANITY_WELL = 33991 -- NPC_SANITY_WELL (ulduar.h:225)
local SPELL_SANITY_WELL = 64169 -- boss_yogg_saron.cpp:218
local SPELL_SANITY_WELL_VISUAL = 63288 -- boss_yogg_saron.cpp:217
local ENTRY_KEEPER_HODIR = 33411
local ENTRY_KEEPER_MIMIRON = 33412
local ENTRY_KEEPER_THORIM = 33413
local SPELL_KEEPER_TELEPORT = 62940
local SAY_KEEPER_CHOSEN_1 = 0
local SAY_KEEPER_CHOSEN_2 = 1
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1

local SPELL_SANITY_PERIODIC = 63786
local SPELL_SUMMON_GUARDIAN_2 = 62978
local SPELL_INSANE_PERIODIC = 64554
local SPELL_TENTACLE_VOID_ZONE_2 = 64384
local SPELL_NONDESCRIPT_1 = 64013
local SPELL_NONDESCRIPT_2 = 64010
local SPELL_OMINOUS_CLOUD_VISUAL = 63084
local SPELL_MATCH_HEALTH = 64066
local SPELL_VERTEX_COLOR_BLACK = 39662
local SPELL_SIMPLE_TELEPORT_KEEPERS = 12980
local SPELL_KEEPER_ACTIVE = 62647
local SPELL_RESILIENCE_OF_NATURE = 62670
local SPELL_SANITY_WELL_SUMMON = 64170
local SPELL_FORTITUDE_OF_FROST = 62650
local SPELL_HODIRS_PROTECTIVE_GAZE = 64174
local SPELL_FURY_OF_THE_STORM = 62702
local SPELL_SPEED_OF_INVENTION = 62671

-- C++ boss_voice_of_yogg_saronAI::JustEngagedWith (:518-529):
-- DoCast(me, SPELL_SANITY_PERIODIC 63786) (2-arg self, non-triggered,
-- ungated, :523) + DoCastAOE(SPELL_SUMMON_GUARDIAN_2 62978,
-- {SPELLVALUE_MAX_TARGETS, 1}, :522) + events.ScheduleEvent(
-- EVENT_LOCK_DOOR, 15s) (no phase arg, :525). The SetInCombatWith
-- keeper machine (:518-519) and the EVENT_EXTINGUISH_ALL_LIFE /
-- EVENT_SUMMON_GUARDIAN_OF_YOGG_SARON schedules ride unbridged
-- instance/summon/phase machines (assembly_of_iron / gothik
-- precedents) — documented-only. DoCastAOE(62978) is a plain
-- straight-line arm (no machine state consumed), so the cast ports
-- as self-cast (DoCastAOE convention); the max-targets clamp has
-- no bridge and is dropped. Non-triggered: the SpellValueMod-only
-- CastSpellExtraArgs ctor leaves TriggerFlags at TRIGGERED_NONE
-- (SpellDefines.h:178).
local function voiceEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_yogg_saron.cpp:539: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:SetCombatMovement(false) -- ScriptedAI ctor arm, boss_yogg_saron.cpp:452 (ranged AI stands and casts)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith: DoCastAOE(SPELL_SUMMON_GUARDIAN_2 62978,
    -- {SPELLVALUE_MAX_TARGETS, 1}) (boss_yogg_saron.cpp:522) — plain
    -- self-cast arm (DoCastAOE convention), non-triggered; the
    -- max-targets clamp has no bridge and is dropped.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SUMMON_GUARDIAN_2)
    end
    -- C++ JustEngagedWith: DoCast(me, SPELL_SANITY_PERIODIC)
    -- (boss_yogg_saron.cpp:523). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SANITY_PERIODIC)
    end
    -- C++ EVENT_LOCK_DOOR: one-shot 15s -> DoCast(me,
    -- SPELL_INSANE_PERIODIC) (:556-557); the companion
    -- instance->SetBossState(BOSS_YOGG_SARON, IN_PROGRESS) leg has no
    -- instance bridge — cast only.
    schedule(guid, "lockdoor", 15000, function()
        if creature:IsAlive() then
            creature:CastSpell(creature, SPELL_INSANE_PERIODIC)
        end
    end)
end

-- C++ boss_voice_of_yogg_saronAI::JustSummoned (:647-675): only the
-- plain one-shot legs port — constrictor Lunge (triggered, :658),
-- descend portal visual (triggered, :667), immortal simple teleport
-- (triggered, :670), Yogg emerge emote (:655), crusher/corruptor
-- emerge emote (:660-665, passive + attack-kick legs withheld).
-- The guardian 1s and
-- crusher/corruptor 5s StartAttackEvent kicks + the crusher/corruptor
-- REACT_PASSIVE arm (:651-652, :660-665) stay out (AttackStart has no
-- bridge; passive without the kick would strand the summons).
local ENTRY_DESCEND_INTO_MADNESS = 34072 -- ulduar.h:226
local SPELL_LUNGE = 64131
local SPELL_TELEPORT_PORTAL_VISUAL = 64416
local SPELL_SIMPLE_TELEPORT = 64195
local EMOTE_ONESHOT_EMERGE = 449
local function voiceJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == ENTRY_CONSTRICTOR_TENTACLE then
        summoned:CastSpell(summoned, SPELL_LUNGE, true)
    elseif entry == ENTRY_DESCEND_INTO_MADNESS then
        summoned:CastSpell(summoned, SPELL_TELEPORT_PORTAL_VISUAL, true)
    elseif entry == ENTRY_IMMORTAL_GUARDIAN then
        summoned:CastSpell(summoned, SPELL_SIMPLE_TELEPORT, true)
    elseif entry == ENTRY_CRUSHER_TENTACLE or entry == ENTRY_CORRUPTOR_TENTACLE then
        -- C++ voice JustSummoned (:660-665): emerge emote only — the
        -- REACT_PASSIVE + StartAttackEvent kick legs stay out (no
        -- AttackStart bridge; passive without the kick would strand).
        summoned:PerformEmote(EMOTE_ONESHOT_EMERGE)
    elseif entry == ENTRY_YOGG_SARON then
        summoned:PerformEmote(EMOTE_ONESHOT_EMERGE)
    end
end

-- C++ npc_constrictor_tentacleAI::Reset (:1317-1320): non-triggered
-- one-shots DoCast(me, SPELL_TENTACLE_VOID_ZONE_2 64384) +
-- DoCastAOE(SPELL_ERUPT 64144) — self-cast form per the
-- DoCastAOE≡self-cast convention (boss_ai.go lowering). Event 23
-- (the halion/corruptor/crusher Reset precedent). The JustSummoned
-- LUNGE leg rides voiceJustSummoned above (triggered, voice-side :658).
local function constrictorReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_constrictor_tentacleAI ctor (:1314): SetCombatMovement(false).
    creature:SetCombatMovement(false)
    -- C++ Reset: DoCast(me, TENTACLE_VOID_ZONE_2); DoCastAOE(ERUPT)
    -- (boss_yogg_saron.cpp:1318-1319). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_TENTACLE_VOID_ZONE_2)
        creature:CastSpell(creature, SPELL_ERUPT)
    end
end

-- C++ npc_influence_tentacleAI::Reset (:1419-1423): DoCast(me,
-- me->GetEntry() == NPC_SUIT_OF_ARMOR (33433, ulduar.h:209) ?
-- SPELL_NONDESCRIPT_1 64013 : SPELL_NONDESCRIPT_2 64010) (2-arg,
-- non-triggered, ungated) — event 23; both entries registered (the
-- conditional proves the script serves both; bindings are DB-side).
-- JustDied DoAction(ACTION_TENTACLE_KILLED) stays out.
local function influenceReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: DoCast(me, GetEntry()==NPC_SUIT_OF_ARMOR ?
    -- NONDESCRIPT_1 : NONDESCRIPT_2) (boss_yogg_saron.cpp:1422).
    -- Alive-gated.
    if creature:IsAlive() then
        if creature:GetEntry() == ENTRY_SUIT_OF_ARMOR then
            creature:CastSpell(creature, SPELL_NONDESCRIPT_1)
        else
            creature:CastSpell(creature, SPELL_NONDESCRIPT_2)
        end
    end
end

-- C++ npc_ominous_cloudAI::Reset (:1158-1162): DoCast(me,
-- SPELL_OMINOUS_CLOUD_VISUAL 63084) (2-arg, non-triggered, ungated)
-- — event 23. The DoAction MoveCirclePath leg stays out.
local function ominousCloudReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: DoCast(me, OMINOUS_CLOUD_VISUAL)
    -- (boss_yogg_saron.cpp:1160). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_OMINOUS_CLOUD_VISUAL)
    end
end

-- C++ boss_brain_of_yogg_saronAI::Reset (:1061-1066): RemoveFlag
-- NOT_SELECTABLE + DoCast(me, SPELL_MATCH_HEALTH 64066)
-- (2-arg, non-triggered, ungated) — event 23. The
-- SetImmuneToPC/_summons.DespawnAll legs and DamageTaken legs
-- (:1070-1080, triggered casts + DoAction machine) stay out.
local function brainReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: RemoveFlag NOT_SELECTABLE (:1063), ungated,
    -- before the MATCH_HEALTH cast (:1065).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- C++ Reset: DoCast(me, MATCH_HEALTH)
    -- (boss_yogg_saron.cpp:1065). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_MATCH_HEALTH)
    end
end

-- C++ npc_turned_championAI::Reset (:1908-1912): DoCast(
-- SPELL_VERTEX_COLOR_BLACK 39662) (1-arg self, non-triggered,
-- ungated) — event 23. MovementInform emote + DoAction(
-- ACTION_START_ROLEPLAY) stay out.
local function turnedChampionReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: DoCast(VERTEX_COLOR_BLACK)
    -- (boss_yogg_saron.cpp:1911). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_VERTEX_COLOR_BLACK)
    end
end

-- C++ npc_observation_ring_keeperAI::Reset (:1543-1548): DoCast(
-- SPELL_SIMPLE_TELEPORT_KEEPERS 12980) + DoCast(
-- SPELL_KEEPER_ACTIVE 62647) (1-arg self, non-triggered, ungated)
-- — event 23 for all four entries (33213/33241/33242/33244,
-- ulduar.h:199-202). OnGossipSelect summon + Talk legs stay out.
local function observationKeeperReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset: DoCast(SIMPLE_TELEPORT_KEEPERS);
    -- DoCast(KEEPER_ACTIVE) (boss_yogg_saron.cpp:1546-1547).
    -- Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SIMPLE_TELEPORT_KEEPERS)
        creature:CastSpell(creature, SPELL_KEEPER_ACTIVE)
    end
end

-- C++ npc_yogg_saron_keeperAI::Reset (:1605-1612): DoCast(
-- SPELL_KEEPER_ACTIVE 62647) (1-arg self, non-triggered, ungated)
-- — event 23. The _events Reset/SetPhase legs stay out; the Freya
-- sanity-well GetCreatureList cleanup PORTED in yoggKeeperReset below; the
-- SetReactState(REACT_PASSIVE) leg (ungated, :1607) IS ported below
-- (react-state bridge retraction; RemoveAllAuras ported 2026-10-19,
-- passive ported this run).
local function yoggKeeperReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_yogg_saron_keeperAI::Reset (boss_yogg_saron.cpp:1607):
    -- me->SetReactState(REACT_PASSIVE), ungated, before RemoveAllAuras.
    creature:SetReactState(REACT_PASSIVE)
    creature:RemoveAllAuras()
    -- C++ Reset: DoCast(KEEPER_ACTIVE)
    -- (boss_yogg_saron.cpp:1610). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_KEEPER_ACTIVE)
    end
    -- C++ Reset (:1615-1620), Freya only, ungated: strip both sanity
    -- well auras from every sanity well within 200.0f.
    if creature:GetEntry() == ENTRY_KEEPER_FREYA then
        for _, well in ipairs(creature:GetCreaturesInGrid(ENTRY_SANITY_WELL, 200)) do
            well:RemoveAura(SPELL_SANITY_WELL)
            well:RemoveAura(SPELL_SANITY_WELL_VISUAL)
        end
    end
end

-- C++ npc_yogg_saron_keeperAI::JustEnteredCombat (:1614-1639):
-- entry-gated 1-arg non-triggered one-shots: Freya (33410)
-- SPELL_RESILIENCE_OF_NATURE 62670 + SPELL_SANITY_WELL_SUMMON 64170;
-- Hodir (33411) SPELL_FORTITUDE_OF_FROST 62650 +
-- SPELL_HODIRS_PROTECTIVE_GAZE 64174; Thorim (33413)
-- SPELL_FURY_OF_THE_STORM 62702; Mimiron (33412)
-- SPELL_SPEED_OF_INVENTION 62671 — the auriaya engage-port
-- precedent. The voice-driven SetInCombatWith combat-entry leg
-- (:518-519) has no bridge — documented deviation (assembly_of_iron
-- precedent); keeper IsSummonedBy SIMPLE_TELEPORT_KEEPERS (:1600)
-- stays out.
local function keeperEnterCombat(event, creature, target)
    -- C++ JustEnteredCombat: entry-gated one-shots
    -- (boss_yogg_saron.cpp:1618-1637). Alive-gated.
    if creature:IsAlive() then
        local entry = creature:GetEntry()
        if entry == ENTRY_KEEPER_FREYA then
            creature:CastSpell(creature, SPELL_RESILIENCE_OF_NATURE)
            creature:CastSpell(creature, SPELL_SANITY_WELL_SUMMON)
        elseif entry == ENTRY_KEEPER_HODIR then
            creature:CastSpell(creature, SPELL_FORTITUDE_OF_FROST)
            creature:CastSpell(creature, SPELL_HODIRS_PROTECTIVE_GAZE)
        elseif entry == ENTRY_KEEPER_THORIM then
            creature:CastSpell(creature, SPELL_FURY_OF_THE_STORM)
        elseif entry == ENTRY_KEEPER_MIMIRON then
            creature:CastSpell(creature, SPELL_SPEED_OF_INVENTION)
        end
    end
end

RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 1, voiceEnterCombat)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_VOICE_OF_YOGG_SARON, 19, voiceJustSummoned)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_CONSTRICTOR_TENTACLE, 23, constrictorReset)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 2, cancelAll)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 4, cancelAll)
RegisterCreatureEvent(ENTRY_INFLUENCE_TENTACLE, 23, influenceReset)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 4, cancelAll)
RegisterCreatureEvent(ENTRY_SUIT_OF_ARMOR, 23, influenceReset)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OMINOUS_CLOUD, 23, ominousCloudReset)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_BRAIN_OF_YOGG_SARON, 23, brainReset)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 2, cancelAll)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 4, cancelAll)
RegisterCreatureEvent(ENTRY_TURNED_CHAMPION, 23, turnedChampionReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 23, observationKeeperReset)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 23, observationKeeperReset)

-- C++ npc_observation_ring_keeperAI::OnGossipSelect (:1551-1576):
-- menuId 10333 gate (sender == 0 DB-menu proxy — the Lua select
-- hook passes sender/action, never menuId) -> RemoveFlag gossip +
-- 2s despawn + DoCast TELEPORT + Talk 0/1 to player + entry-switched
-- keeper summon at YSKeepersPos, C++ order. C++ returns false
-- (native arms proceed); Lua mirrors it.
local function observationKeeperGossipSelect(event, player, creature, sender, action)
    if sender ~= 0 then
        return false
    end
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    local guid = creature:GetGUID()
    schedule(guid, "keeper_despawn", 2000, function()
        creature:Despawn()
    end)
    creature:CastSpell(creature, SPELL_KEEPER_TELEPORT)
    creature:Talk(SAY_KEEPER_CHOSEN_1, player)
    creature:Talk(SAY_KEEPER_CHOSEN_2, player)
    local entry = creature:GetEntry()
    if entry == ENTRY_OBSERVATION_RING_KEEPER_FREYA then
        creature:SummonCreature(ENTRY_KEEPER_FREYA, 2036.873, 25.42513, 338.4984, 3.909538)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_HODIR then
        creature:SummonCreature(ENTRY_KEEPER_HODIR, 1939.045, -90.87457, 338.5426, 0.994837)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_THORIM then
        creature:SummonCreature(ENTRY_KEEPER_THORIM, 1939.148, 42.49035, 338.5427, 5.235988)
    elseif entry == ENTRY_OBSERVATION_RING_KEEPER_MIMIRON then
        creature:SummonCreature(ENTRY_KEEPER_MIMIRON, 2036.658, -73.58822, 338.4985, 2.460914)
    end
    return false
end
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_FREYA, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_HODIR, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_THORIM, 2, observationKeeperGossipSelect)
RegisterCreatureGossipEvent(ENTRY_OBSERVATION_RING_KEEPER_MIMIRON, 2, observationKeeperGossipSelect)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_FREYA, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_HODIR, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_MIMIRON, 23, yoggKeeperReset)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 1, keeperEnterCombat)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 2, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 4, cancelAll)
RegisterCreatureEvent(ENTRY_KEEPER_THORIM, 23, yoggKeeperReset)

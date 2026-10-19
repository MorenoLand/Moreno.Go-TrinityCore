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
-- Sara DamageTaken — lethal (damage >= health) is NOT a
-- death rewrite: damage = health - 1 (survive with 1 hp)
-- and Talk(SAY_SARA_TRANSFORM_1) fires only in PHASE_ONE
-- alongside cross-creature DoAction(ACTION_PHASE_TRANSFORM)
-- — no phase / DoAction bridges; the event-9 bridge
-- cannot model it — bridge-blocked.
-- Sara SpellHitTarget — Talk(SAY_SARA_FERVOR_HIT /
-- SAY_SARA_BLESSING_HIT / SAY_SARA_PSYCHOSIS_HIT) gated on
-- roll_chance_i(30) and !PHASE_TRANSFORM — stays out on trigger-path
-- grounds: the EVENT_SARAS_FERVOR / BLESSING / ANGER casts are unported
-- and the phase gate has no bridge, so event 15 never fires for them in
-- Go (event 15 itself is live — boss_ai.go:1904).
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
-- PORTED via event 14 (yoggSaronSpellHit below); the Reset
-- ResetLootMode leg (:939) rides the unbridged scheduler.
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
-- / SPELL_INSANE) gated on PHASE_ONE — no wipe / aura /
-- phase bridges; timer Talks (cross-creature
-- EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE on
-- EVENT_EXTINGUISH_ALL_LIFE; EMOTE_YOGG_SARON_MADNESS +
-- SAY_YOGG_SARON_MADNESS on EVENT_ILLUSION) ride the
-- unbridgeable voice event machine (the mimiron
-- cross-creature precedent).
-- Brain of Yogg-Saron (33890, ulduar.h line 218) — Reset
-- DoCast(me, SPELL_MATCH_HEALTH 64066) PORTED below (event 23);
-- DamageTaken has no Talk arm (aura/flag/DoAction legs
-- only); DoAction ACTION_INDUCE_MADNESS /
-- ACTION_TENTACLE_KILLED have no Talk arms.
-- npc_observation_ring_keeper — Talk(SAY_KEEPER_CHOSEN_1/2,
-- player) gated on OnGossipSelect menuId 10333 — the gossip bridge
-- IS live (RegisterCreatureGossipEvent,
-- engine/world/gossip.go:129/369) but the Lua select hook never
-- receives menuId (sender/action only — gossip.go:351), so the gate
-- is unexpressible; Reset one-shots PORTED below (event
-- 23); (observation-ring
-- keeper entries 33213 / 33241 / 33242 / 33244 entry-verifiable,
-- ulduar.h lines 199-202).
-- npc_yogg_saron_keeper (33410 / 33411 / 33412 / 33413,
-- ulduar.h lines 205-208) — no Talk arms — Reset + JustEnteredCombat
-- one-shots PORTED below (events 23 / 1).
-- npc_yogg_saron_illusions / npc_garona /
-- npc_turned_champion / npc_laughing_skull — roleplay
-- Talks (SAY_CHAMBER_ROLEPLAY_1-5 / SAY_ICECROWN_ROLEPLAY_1-6 /
-- SAY_STORMWIND_ROLEPLAY_1-7) fire from IsSummonedBy /
-- the illusion UpdateAI timer machine on instance
-- DATA_ILLUSION — no bridges; no registrations. (Turned
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
local SAY_YOGG_SARON_DEATH = 6

local SPELL_SHADOWY_BARRIER_YOGG = 63894
local SPELL_KNOCK_AWAY = 64022
local SPELL_IN_THE_MAWS_OF_THE_OLD_GOD = 64184

-- C++ JustEngagedWith (boss_saraAI, ScriptedAI):
-- Talk(SAY_SARA_AGGRO) + three ScheduleEvent legs — only
-- the Talk arm is bridgeable; the auriaya engage-port
-- precedent.
local function saraEnterCombat(event, creature, target)
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

-- C++ JustDied (boss_yogg_saronAI, PassiveAI):
-- Talk(SAY_YOGG_SARON_DEATH), then cross-creature kill /
-- disappear / evade legs and player sanity-aura removal —
-- only the Talk arm is bridgeable; the sjonnir
-- JustDied-Talk precedent.
local function yoggSaronDied(event, creature, killer)
    creature:Talk(SAY_YOGG_SARON_DEATH)
end

-- C++ boss_yogg_saronAI::Reset :931-945 — DoCast(me,
-- SPELL_SHADOWY_BARRIER_YOGG) + DoCast(me, SPELL_KNOCK_AWAY), both
-- non-triggered and ungated (the urom evocate-reset precedent); the
-- _events Reset/SetPhase(PHASE_TWO)/ScheduleEvent(EVENT_YELL_BOW_DOWN)
-- and ResetLootMode/loot-mode legs ride unbridged phase/timer/loot
-- machines (the gothik phase-gating precedent).
local function yoggSaronReset(event, creature)
    -- C++ boss_yogg_saronAI::Reset(): DoCast(me, SHADOWY_BARRIER_YOGG);
    -- DoCast(me, KNOCK_AWAY) (boss_yogg_saron.cpp:931-937). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHADOWY_BARRIER_YOGG)
        creature:CastSpell(creature, SPELL_KNOCK_AWAY)
    end
end

RegisterCreatureEvent(ENTRY_SARA, 1, saraEnterCombat)
RegisterCreatureEvent(ENTRY_SARA, 3, saraKilledUnit)
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
-- the same map+instance is eligible (:1264, :1284-1288 verified);
-- threat-list-vs-player-pool caveat (the_beast/opera-Julianne
-- precedent): C++ picks from the threat list, the pool is map-wide
-- players. The
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
-- leg has no bridge.
local function crusherReset(event, creature)
    cancelTimers(creature:GetGUID())
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
local ENTRY_TURNED_CHAMPION = 33962
local ENTRY_OBSERVATION_RING_KEEPER_HODIR = 33213
local ENTRY_OBSERVATION_RING_KEEPER_FREYA = 33241
local ENTRY_OBSERVATION_RING_KEEPER_THORIM = 33242
local ENTRY_OBSERVATION_RING_KEEPER_MIMIRON = 33244
local ENTRY_KEEPER_FREYA = 33410
local ENTRY_KEEPER_HODIR = 33411
local ENTRY_KEEPER_MIMIRON = 33412
local ENTRY_KEEPER_THORIM = 33413

local SPELL_SANITY_PERIODIC = 63786
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
-- ungated, :525) + events.ScheduleEvent(EVENT_LOCK_DOOR, 15s) (no
-- phase arg, :525). The SetInCombatWith keeper machine (:518-519),
-- the DoCastAOE(SPELL_SUMMON_GUARDIAN_2, spellvalue) leg, and the
-- EVENT_EXTINGUISH_ALL_LIFE / EVENT_SUMMON_GUARDIAN_OF_YOGG_SARON
-- schedules ride unbridged instance/summon/phase machines
-- (assembly_of_iron / gothik precedents) — documented-only.
local function voiceEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ JustEngagedWith: DoCast(me, SPELL_SANITY_PERIODIC)
    -- (boss_yogg_saron.cpp:525). Alive-gated.
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

-- C++ npc_constrictor_tentacleAI::Reset (:1317-1320): non-triggered
-- one-shots DoCast(me, SPELL_TENTACLE_VOID_ZONE_2 64384) +
-- DoCastAOE(SPELL_ERUPT 64144) — self-cast form per the
-- DoCastAOE≡self-cast convention (boss_ai.go lowering). Event 23
-- (the halion/corruptor/crusher Reset precedent). The JustSummoned
-- LUNGE leg (triggered, voice-side :663) stays out.
local function constrictorReset(event, creature)
    cancelTimers(creature:GetGUID())
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

-- C++ boss_brain_of_yogg_saronAI::Reset (:1061-1066): DoCast(me,
-- SPELL_MATCH_HEALTH 64066) (2-arg, non-triggered, ungated) —
-- event 23. The RemoveFlag/SetImmuneToPC/_summons.DespawnAll legs
-- and DamageTaken legs (:1070-1080, triggered casts + DoAction
-- machine) stay out.
local function brainReset(event, creature)
    cancelTimers(creature:GetGUID())
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
-- — event 23. The _events Reset/SetPhase, SetReactState legs and
-- the Freya sanity-well GetCreatureList cleanup ride unbridged
-- machines (RemoveAllAuras ported 2026-10-19).
local function yoggKeeperReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveAllAuras()
    -- C++ Reset: DoCast(KEEPER_ACTIVE)
    -- (boss_yogg_saron.cpp:1610). Alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_KEEPER_ACTIVE)
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

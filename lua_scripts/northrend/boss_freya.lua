-- Freya (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_freya.cpp
-- (boss_freya (CreatureScript) via GetUlduarAI<boss_freyaAI>
-- (BossAI), BOSS_FREYA = 9; boss_elder_brightleaf /
-- boss_elder_ironbranch / boss_elder_stonebark (CreatureScript
-- via GetUlduarAI<...AI> (BossAI), BOSS_BRIGHTLEAF = 14 /
-- BOSS_IRONBRANCH = 15 / BOSS_STONEBARK = 16 — all registered
-- from inside AddSC_boss_freya(); loader decl 120 / call 315
-- (call side verified this run — the header's earlier "call 316"
-- was wrong; 316 is AddSC_boss_thorim()) per
-- northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_hodir() (decl 119 / call 314); the call
-- after it is AddSC_boss_thorim() — loader order confirmed
-- this run; the checkpoint sequence (boss_hodir ->
-- boss_freya) is followed).
-- Entry: 32906 Freya (ulduar.h NPC_FREYA, line 80),
-- 32913 Ironbranch (NPC_IRONBRANCH, line 136),
-- 32915 Brightleaf (NPC_BRIGHTLEAF, line 137),
-- 32914 Stonebark (NPC_STONEBARK, line 138) — all
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the twenty-one script names ("boss_freya",
-- "boss_elder_brightleaf", "boss_elder_ironbranch",
-- "boss_elder_stonebark", "npc_ancient_conservator",
-- "npc_snaplasher", "npc_storm_lasher",
-- "npc_ancient_water_spirit", "npc_detonating_lasher",
-- "npc_sun_beam", "npc_nature_bomb", "npc_eonars_gift",
-- "npc_healthy_spore", "npc_unstable_sun_beam",
-- "npc_iron_roots",
-- "spell_freya_attuned_to_nature_dose_reduction",
-- "spell_freya_iron_roots", "achievement_getting_back_to_nature",
-- "achievement_knock_on_wood",
-- "achievement_knock_knock_on_wood",
-- "achievement_knock_knock_knock_on_wood") hits
-- boss_freya.cpp (+ the loader decl/call lines for
-- AddSC_boss_freya) only; zero sql/ hits for all twenty-one.
-- Ported 2026-10-02 09:30 from a prior-session audit; this run
-- re-verified every claim below against the C++ (the xt002 /
-- vezax / assembly_of_iron / kologarn / hodir stale-wording
-- fix precedent).
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied, 9 OnDamageTaken.
-- Ported arms (C++-exact for all modeled arms):
-- Freya JustEngagedWith — Talk(SAY_AGGRO 0) /
-- Talk(SAY_AGGRO_WITH_ELDER 1) is conditional on elderCount
-- (instance->GetGuidData(BOSS_BRIGHTLEAF + n) alive checks) —
-- no instance GuidData bridge; no registration.
-- Freya KilledUnit — player-gated Talk(SAY_SLAY 2) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the earlier "thirtieth"
-- claim was wrong: at least the forty-first player-gated
-- variant ported (40 pattern-carrying lua ports have mtimes
-- older than this file's 2026-10-02 09:31 birth; 60 are born
-- before it by git birth-order count — the hodir 23:04
-- ordinal-repair precedent).
-- Freya DamageTaken — lethal (damage >= health) ->
-- Talk(SAY_DEATH 3) + damage = 0 (event 9; in C++ the
-- lethal branch sets damage = 0 and manually calls
-- JustDied(who), whose only bridgeable arm is the
-- Talk(SAY_DEATH) — the numeric second return performs the
-- C++-exact damage=0 rewrite (the moroes damage-rewrite
-- precedent); the companion legs have no bridges — see
-- DOCUMENTED-ONLY; the hodir consequence applies: without
-- the despawn leg, subsequent hits keep satisfying
-- damage >= health, so the death Talk re-fires on each hit
-- — verbatim C++ behavior, bounded in C++ by the 7.5s
-- despawn).
-- Elder KilledUnit (all three elders, identical) —
-- player-gated Talk(SAY_ELDER_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the earlier
-- "thirty-first / thirty-second / thirty-third" claim was
-- wrong: at least the forty-second through forty-fourth
-- player-gated variants ported (same count as above,
-- file order: freya then the three elders).
-- Elder JustDied (all three elders, identical) —
-- Talk(SAY_ELDER_DEATH 2) (event 4; the _JustDied()
-- passthrough has no bridge — the sjonnir JustDied-Talk
-- precedent).
-- Elder JustEngagedWith (all three elders, identical) —
-- Talk(SAY_ELDER_AGGRO 0) gated by
-- !me->HasAura(SPELL_DRAINED_OF_POWER 62467) (event 1; the
-- BossAI::JustEngagedWith passthrough has no bridge — the
-- auriaya engage-port precedent).
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 32906 / 32915 / 32914 / 32913):
-- Freya Reset — _Reset() + Initialize() — no bridges.
-- Freya JustEngagedWith companion legs — DoZoneInCombat;
-- elder scan (instance GetGuidData + alive checks) ->
-- applies SPELL_DRAINED_OF_POWER 62467 to the elder, essence
-- casts, RemoveLootMode, elder AttackStart + AddThreat; the
-- Talk choice above; CastSpell SPELL_ATTUNED_TO_NATURE 62519
-- (150 stacks); six ScheduleEvent legs — no instance
-- GuidData / aura / threat / loot-mode / cast / timer
-- bridges.
-- Freya JustDied companion legs — chest summon
-- (summonSpell[2][4]: 62950/62952/62953/62954 10-man,
-- 62955/62956/62957/62958 25-man, indexed by
-- difficulty/elderCount),
-- SetReactState REACT_PASSIVE, InterruptNonMeleeSpells,
-- RemoveAllAttackers, AttackStop, SetFaction FRIENDLY,
-- DespawnOrUnsummon(7.5s), CastSpell
-- SPELL_KNOCK_ON_WOOD_CREDIT, _JustDied(), live-elder
-- shutdown sweep (RemoveAllAuras / AttackStop / CombatStop
-- / EngagementOver / DoAction(ACTION_ELDER_FREYA_KILLED))
-- — no cast / react-state / faction / despawn / DoAction
-- bridges; Talk(SAY_DEATH) itself is ported via event 9.
-- Freya UpdateAI event machine — EVENT_WAVE (summon wave
-- adds); EVENT_EONAR_GIFT (Talk(EMOTE_LIFEBINDERS_GIFT 8));
-- EVENT_NATURE_BOMB / EVENT_UNSTABLE_ENERGY / EVENT_STRENGTHENED_IRON_ROOTS
-- (Talk(EMOTE_IRON_ROOTS 11)); EVENT_GROUND_TREMOR
-- (Talk(EMOTE_GROUND_TREMOR 10)); EVENT_ENRAGE
-- (Talk(SAY_BERSERK 4)); wave summon Talks
-- (SAY_SUMMON_LASHERS 7 / SAY_SUMMON_TRIO 6 /
-- SAY_SUMMON_CONSERVATOR 5); Talk(EMOTE_ALLIES_OF_NATURE
-- 9) — no timer-event / cast / summon bridges; all timer-leg
-- yells ride the unbridgeable event machine (every one of
-- these numbers re-verified against the yells enum this run;
-- the earlier header had them all off).
-- Freya GetData(DATA_GETTING_BACK_TO_NATURE /
-- DATA_KNOCK_ON_WOOD / DATA_KNOCK_KNOCK_ON_WOOD /
-- DATA_KNOCK_KNOCK_KNOCK_ON_WOOD) + DoAction — no GetData /
-- DoAction bridges (consumers: the four achievements below).
-- Elder DamageTaken (stonebark/brightleaf:
-- SPELL_DRAINED_OF_POWER self-cast on lethal; ironbranch:
-- lethal branch) — no aura bridges.
-- npc_ancient_conservator / npc_snaplasher /
-- npc_storm_lasher / npc_ancient_water_spirit /
-- npc_detonating_lasher / npc_sun_beam / npc_nature_bomb /
-- npc_eonars_gift / npc_healthy_spore /
-- npc_unstable_sun_beam / npc_iron_roots — wave/summon
-- creatures — no spawn / cast / movement bridges; no
-- registrations.
-- spell_freya_attuned_to_nature_dose_reduction /
-- spell_freya_iron_roots — no SpellScript bridges; both join
-- the no-SpellScript-bridge queue.
-- achievement_getting_back_to_nature /
-- achievement_knock_on_wood / achievement_knock_knock_on_wood /
-- achievement_knock_knock_knock_on_wood (OnCheck: GetData
-- legs) — no GetData / achievement bridges; all four join
-- the unmodeled-achievement queue.

local ENTRY_FREYA = 32906
local ENTRY_BRIGHTLEAF = 32915
local ENTRY_STONEBARK = 32914
local ENTRY_IRONBRANCH = 32913

local SPELL_DRAINED_OF_POWER = 62467

local SAY_SLAY = 2
local SAY_DEATH = 3

local SAY_ELDER_AGGRO = 0
local SAY_ELDER_SLAY = 1
local SAY_ELDER_DEATH = 2

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function freyaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken: damage >= me->GetHealth() -> damage = 0
-- + manual JustDied(who) call; JustDied's only bridgeable
-- arm is Talk(SAY_DEATH) — the numeric return performs the
-- C++-exact damage=0 rewrite (the moroes damage-rewrite
-- precedent; the hodir consequence applies).
local function freyaDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:Talk(SAY_DEATH)
        return 0
    end
end

-- C++ JustEngagedWith: BossAI passthrough, then
-- Talk(SAY_ELDER_AGGRO) unless the elder still carries
-- SPELL_DRAINED_OF_POWER — the auriaya engage-port
-- precedent with the C++-exact aura gate.
local function elderEnterCombat(event, creature, target)
    if not creature:HasAura(SPELL_DRAINED_OF_POWER) then
        creature:Talk(SAY_ELDER_AGGRO)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_ELDER_SLAY) — the razuvious player-gated variant
-- precedent.
local function elderTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_ELDER_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough, then
-- Talk(SAY_ELDER_DEATH) — the sjonnir JustDied-Talk
-- precedent.
local function elderDied(event, creature, killer)
    creature:Talk(SAY_ELDER_DEATH)
end

RegisterCreatureEvent(ENTRY_FREYA, 3, freyaTargetDied)
RegisterCreatureEvent(ENTRY_FREYA, 9, freyaDamageTaken)

RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 1, elderEnterCombat)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 4, elderDied)

RegisterCreatureEvent(ENTRY_STONEBARK, 1, elderEnterCombat)
RegisterCreatureEvent(ENTRY_STONEBARK, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_STONEBARK, 4, elderDied)

RegisterCreatureEvent(ENTRY_IRONBRANCH, 1, elderEnterCombat)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 4, elderDied)

-- Valithria Dreamwalker (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_valithria_dreamwalker.cpp
-- (1269 lines incl. license; 10 CreatureScripts
-- (boss_valithria_dreamwalker (ScriptedAI, DATA_VALITHRIA_DREAMWALKER = 10),
-- npc_green_dragon_combat_trigger (BossAI),
-- npc_the_lich_king_controller (ScriptedAI),
-- npc_risen_archmage / npc_blazing_skeleton / npc_suppresser /
-- npc_blistering_zombie / npc_gluttonous_abomination (ScriptedAI),
-- npc_dream_portal (CreatureAI), npc_dream_cloud (ScriptedAI)) +
-- 9 SpellScripts (spell_dreamwalker_mana_void,
-- spell_dreamwalker_decay_periodic_timer, spell_dreamwalker_summoner,
-- spell_dreamwalker_summon_suppresser,
-- spell_dreamwalker_summon_suppresser_effect,
-- spell_dreamwalker_summon_dream_portal,
-- spell_dreamwalker_summon_nightmare_portal,
-- spell_dreamwalker_nightmare_cloud,
-- spell_dreamwalker_twisted_nightmares) + 1 AchievementCriteriaScript
-- (achievement_portal_jockey); all registered from inside
-- AddSC_boss_valithria_dreamwalker(); loader decl 181 / call 376 per
-- northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (sister_svalna ->
-- valithria_dreamwalker) is followed).
-- Entries: 36789 Valithria Dreamwalker (icecrown_citadel.h
-- NPC_VALITHRIA_DREAMWALKER, line 287; instance_icecrown_citadel.cpp
-- OnCreatureCreate binds case NPC_VALITHRIA_DREAMWALKER, line 291),
-- 16980 the lich king controller (icecrown_citadel.h
-- NPC_THE_LICH_KING_VALITHRIA, line 297; OnCreatureCreate binds case
-- NPC_THE_LICH_KING_VALITHRIA, line 294) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent); the
-- RegisterIcecrownCitadelCreatureAI ScriptName bindings are DB-side as
-- usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_valithria_dreamwalker" hits boss_valithria_dreamwalker.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction. No valithria lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat.
-- Ported arms (C++-exact for all modeled arms):
-- npc_the_lich_king_controller JustEngagedWith — Talk(SAY_LICH_KING_INTRO 0)
-- (event 1 — the auriaya engage-port precedent; the me->setActive(true)
-- leg has no bridge).
-- DOCUMENTED-ONLY (in this header; only entry 16980 registered):
-- boss_valithria_dreamwalker exposes zero bridgeable arms — NOT registered
-- (the bronjahm npc_corrupted_soul_fragment precedent), 8 Talk() calls
-- all on unbridged legs:
-- Talk(SAY_VALITHRIA_SUCCESS 7) rides the HealReceived 100%-heal leg with
-- the _done flag (no HealReceived bridge; the DoCastSelf/DoCastAOE /
-- instance->SendEncounterUnit / RemoveAuras legs have no bridges);
-- Talk(SAY_VALITHRIA_75_PERCENT 2) rides the HealReceived 75%-threshold leg
-- with the _over75PercentTalkDone flag (no HealReceived bridge);
-- PORTED (2026-10-13): Talk(SAY_VALITHRIA_25_PERCENT 3) — the
-- DamageTaken HealthBelowPctDamaged(25, damage) leg with the
-- _under25PercentTalkDone latch (event 9; ironaya floored-integer
-- convention — algalon/moroes precedent). The death-negation leg
-- (damage = 0, _justDied flag) has no bridge — a lone damage=0 would
-- strand the encounter (algalon precedent);
-- Talk(SAY_VALITHRIA_DEATH 4) rides the DamageTaken death-negated leg
-- (damage = 0, _justDied flag — she never actually dies; the trigger
-- DoAction(ACTION_DEATH) cross-AI leg has no bridge);
-- Talk(SAY_VALITHRIA_ENTER_COMBAT 0) rides UpdateAI EVENT_INTRO_TALK
-- (scheduled from the DoAction ACTION_ENTER_COMBAT leg — no-DoAction and
-- no-timer bridges);
-- Talk(SAY_VALITHRIA_BERSERK 6) rides UpdateAI EVENT_BERSERK (heroic-only
-- scheduler leg — no-timer-bridge);
-- Talk(SAY_VALITHRIA_DREAM_PORTAL 1) rides UpdateAI EVENT_DREAM_PORTAL
-- (non-heroic scheduler leg; the SUMMON_PORTAL DoCast legs have no-cast
-- bridge).
-- SAY_VALITHRIA_PLAYER_DEATH 5 is declared in the text enum but never
-- referenced by a Talk() call — dead text evidence (no KilledUnit arm
-- exists in the file).
-- npc_the_lich_king_controller Reset scheduler legs (5 summon-timer
-- schedules), JustSummoned phase-mask / DoZoneInCombat legs, UpdateAI
-- summoner machine and the 5 DoCastSelf SPELL_TIMER_* legs (no-timer /
-- no-cast / no-movement bridges); JustReachedHome me->setActive(false)
-- (no bridge).
-- npc_green_dragon_combat_trigger (38752) — no Talk; the DoAction
-- ACTION_ENTER_COMBAT / ACTION_SETUP_ARCHMAGES / ACTION_DEATH machine
-- (no-DoAction bridge) — NOT registered.
-- npc_risen_archmage (37868) / npc_blazing_skeleton (36791) /
-- npc_suppresser (37863) — zero Talk lines; their UpdateAI DoCastSelf
-- legs, the suppresser MovementInform CHASE_MOTION_TYPE leg (no
-- MovementInform bridge), summoner SpellHitTarget knockback legs (event
-- 15 never fires) — NOT registered.
-- PORTED (2026-10-13): npc_blistering_zombie (37934) JustDied —
-- DoCastSelf(SPELL_ACID_BURST 70744, triggered); npc_gluttonous_abomination
-- (37886) JustDied — DoCastSelf(SPELL_ROT_WORM_SPAWNER 70675, triggered)
-- unless killed by Valithria (event 4; the_black_morass killer-gate precedent).
-- npc_dream_portal (37945) / npc_dream_cloud (entry DB-side, no header
-- constant) — zero Talk lines; portal JustSummoned delayed-cast event
-- legs (ValithriaDelayedCastEvent / ValithriaAuraRemoveEvent — no
-- event-machine bridge), MISSED_PORTALS GetData leg (no bridge) — NOT
-- registered.
-- The 9 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent); achievement_portal_jockey joins the
-- no-achievement-criteria bridge queue (the lana'thel achievement
-- precedent).

local ENTRY_LICH_KING_VALITHRIA = 16980
local ENTRY_VALITHRIA_DREAMWALKER = 36789
local ENTRY_BLISTERING_ZOMBIE = 37934
local ENTRY_GLUTTONOUS_ABOMINATION = 37886

local SAY_LICH_KING_INTRO = 0
local SAY_VALITHRIA_25_PERCENT = 3
local SAY_VALITHRIA_DEATH = 4

local SPELL_ACID_BURST = 70744
local SPELL_ROT_WORM_SPAWNER = 70675

-- C++ npc_the_lich_king_controller::JustEngagedWith:
-- Talk(SAY_LICH_KING_INTRO) — the auriaya engage-port precedent.
local function lichKingJustEngagedWith(event, creature, target)
    creature:Talk(SAY_LICH_KING_INTRO)
end

local valithriaState = {}

-- C++ boss_valithria_dreamwalker::DamageTaken — HealthBelowPctDamaged(
-- 25, damage) = (health - damage) < floor(max*25/100) (Unit.h:873;
-- ironaya floored-integer convention) + the _under25PercentTalkDone
-- latch — Talk(SAY_VALITHRIA_25_PERCENT) (:348-356); the lethal leg
-- (damage >= health) negates damage to 0 + the _justDied latch —
-- Talk(SAY_VALITHRIA_DEATH) (:357-366). In C++ she never dies; the
-- encounter completion rides the cross-AI ACTION_DEATH leg (no
-- bridge), but the no-death + Talk behavior is exactly bridgeable
-- (return false, 0 — julianne/curator damage-negation precedent).
local function valithriaDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    local st = valithriaState[guid] or {}
    valithriaState[guid] = st
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        if not st.talk25Done then
            st.talk25Done = true
            creature:Talk(SAY_VALITHRIA_25_PERCENT)
        end
        if damage >= creature:GetHealth() then
            if not st.justDied then
                st.justDied = true
                creature:Talk(SAY_VALITHRIA_DEATH)
            end
            return false, 0
        end
    end
    return false, damage
end

local function valithriaReset(event, creature)
    valithriaState[creature:GetGUID()] = nil
end

-- C++ npc_blistering_zombie::JustDied — unconditional
-- DoCastSelf(SPELL_ACID_BURST, true) (triggered, :851-853).
local function blisteringZombieJustDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ACID_BURST, true)
end

-- C++ npc_gluttonous_abomination::JustDied —
-- DoCastSelf(SPELL_ROT_WORM_SPAWNER, true) (triggered) unless killed
-- by Valithria herself (:873-878).
local function gluttonousAbominationJustDied(event, creature, killer)
    if killer and killer:GetEntry() == ENTRY_VALITHRIA_DREAMWALKER then
        return
    end
    creature:CastSpell(creature, SPELL_ROT_WORM_SPAWNER, true)
end

RegisterCreatureEvent(ENTRY_LICH_KING_VALITHRIA, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 9, valithriaDamageTaken)
RegisterCreatureEvent(ENTRY_VALITHRIA_DREAMWALKER, 23, valithriaReset)
RegisterCreatureEvent(ENTRY_BLISTERING_ZOMBIE, 4, blisteringZombieJustDied)
RegisterCreatureEvent(ENTRY_GLUTTONOUS_ABOMINATION, 4, gluttonousAbominationJustDied)

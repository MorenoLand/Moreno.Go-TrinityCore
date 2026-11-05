-- Naxxramas Follower / Follower Worshipper (Faerlina's adds) — Lua port of
-- npc_faerlina_add in src/server/scripts/Northrend/Naxxramas/boss_faerlina.cpp
-- (ScriptedAI; AddSC_boss_faerlina registers it; loader decl 70 / call 265 per
-- northrend_script_loader.cpp, under the "// Naxxramas" marker).
-- Entries: NPC_NAXXRAMAS_FOLLOWER 16505 / NPC_FOLLOWER_WORSHIPPER 16506
-- (naxxramas.h:105-106) — the summon-group rows' targets; both entries run
-- this AI in C++ (the ScriptName binding is creature_template DB-side).
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 7 OnAIUpdate,
-- 23 OnReset. Melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady (jedoga bar — engagement-only leg, documented).
-- Ported arms (C++-exact for all modeled arms):
-- Reset (event 23; boss_faerlina.cpp:214-221) — 10-man only
-- (!Is25ManRaid): ApplySpellImmune(0, IMMUNITY_EFFECT 0,
-- SPELL_EFFECT_BIND 11, true) + ApplySpellImmune(0, IMMUNITY_MECHANIC 5,
-- MECHANIC_CHARM 1, true) (SharedDefines.h:1358/1424-1431; the bridge
-- is creature:ApplySpellImmune, lua_creature_events.go:5191; the gate
-- is creature:Is25ManRaid — IsHeroic is heroic-mode, not raid size,
-- and was never a substitute).
-- JustEngagedWith (event 1; :222-226) — GetInstanceCreature(
-- DATA_FAERLINA 1) -> DoZoneInCombat (the boss's ev5 SetGuidData latch,
-- razuvious/understudy precedent; nil-guarded like the C++ site).
-- JustDied (event 4; :228-233) — 10-man only: GetInstanceCreature(
-- DATA_FAERLINA 1) -> DoCast(WIDOWS_EMBRACE 28732) on Faerlina
-- (non-triggered; the ev4 cast from a dead creature rides the
-- razuvious hopeless precedent — Go carries no alive-gate on the
-- OnDied cast). The cast is what fires the boss's SpellHit arm
-- (Talk EMOTE_WIDOW_EMBRACE + the _frenzyDispels counter, whose only
-- consumer is the unbridged achievement).
-- UpdateAI (event 7; :235-243) — DoCastVictim(ADD_FIREBALL 54095)
-- as a per-tick pump (understudy precedent): victim nil-guard +
-- IsCasting gate (the C++ pre-loop :239 gate), victim-cast via
-- CastSpell(nil, ...) (the moroes/razuvious victim-cast idiom).
-- FIDELITY NOTE: the C++ hard-codes 54095 on both difficulties — the
-- "// 25-man: 54096" enum comment is stale vs the C++ text; this port
-- does NOT raid-mode it.
-- Unmodeled: nothing — all four C++ hooks are ported. The adds arrive
-- via the boss's SummonCreatureGroup (no DB rows in the repo world.db,
-- so the port is latent until the live TDB provides
-- creature_summon_groups rows — same caveat as the tick-238 group ports).

local ENTRY_NAXXRAMAS_FOLLOWER = 16505
local ENTRY_FOLLOWER_WORSHIPPER = 16506

local DATA_FAERLINA = 1 -- naxxramas.h NAXData64 (ANUBREKHAN 0, FAERLINA 1)

local SPELL_WIDOWS_EMBRACE = 28732
local SPELL_ADD_FIREBALL = 54095

local IMMUNITY_EFFECT = 0 -- SharedDefines.h:1426
local IMMUNITY_MECHANIC = 5 -- SharedDefines.h:1431
local SPELL_EFFECT_BIND = 11 -- SpellEffects.cpp:80
local MECHANIC_CHARM = 1 -- SharedDefines.h:1358

-- C++ Reset (boss_faerlina.cpp:214-221): 10-man only, BIND + CHARM
-- immunities. No 25-man leg (the if body is the whole arm).
local function addReset(event, creature)
    if not creature:Is25ManRaid() then
        creature:ApplySpellImmune(0, IMMUNITY_EFFECT, SPELL_EFFECT_BIND, true)
        creature:ApplySpellImmune(0, IMMUNITY_MECHANIC, MECHANIC_CHARM, true)
    end
end

-- C++ JustEngagedWith (boss_faerlina.cpp:222-226): pull Faerlina into
-- combat with the zone (the add-pull-boss leg).
local function addEnterCombat(event, creature, target)
    local faerlina = creature:GetInstanceCreature(DATA_FAERLINA)
    if faerlina then
        faerlina:DoZoneInCombat()
    end
end

-- C++ JustDied (boss_faerlina.cpp:228-233): 10-man only, cast Widow's
-- Embrace on Faerlina (the dispel that fires her SpellHit arm).
local function addDied(event, creature, killer)
    if not creature:Is25ManRaid() then
        local faerlina = creature:GetInstanceCreature(DATA_FAERLINA)
        if faerlina then
            creature:CastSpell(faerlina, SPELL_WIDOWS_EMBRACE)
        end
    end
end

-- C++ UpdateAI (boss_faerlina.cpp:235-243): DoCastVictim(ADD_FIREBALL
-- 54095) every tick, gated on the pre-loop CASTING check.
local function addAIUpdate(event, creature, diff)
    local victim = creature:GetVictim()
    if victim == nil or creature:IsCasting() then
        return
    end
    creature:CastSpell(nil, SPELL_ADD_FIREBALL)
end

for _, entry in ipairs({ENTRY_NAXXRAMAS_FOLLOWER, ENTRY_FOLLOWER_WORSHIPPER}) do
    RegisterCreatureEvent(entry, 1, addEnterCombat)
    RegisterCreatureEvent(entry, 4, addDied)
    RegisterCreatureEvent(entry, 7, addAIUpdate)
    RegisterCreatureEvent(entry, 23, addReset)
end

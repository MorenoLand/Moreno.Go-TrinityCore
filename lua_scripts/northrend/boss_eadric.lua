-- Eadric the Pure (Trial of the Champion) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheChampion/boss_argent_challenge.cpp
-- (boss_eadric, boss_paletress, npc_memory, npc_argent_soldier,
-- spell_eadric_radiance (SpellScript), spell_paletress_summon_memory
-- (SpellScript); AddSC_boss_argent_challenge at end registers all
-- via GetTrialOfTheChampionAI / new). The first Trial of the
-- Champion group in northrend_script_loader.cpp order (decl 45 /
-- call 240, immediately after AddSC_instance_drak_tharon_keep();
-- Drak'Tharon Keep block now CLOSED).
-- Entry: 35119 Eadric the Pure (trial_of_the_champion.h NPC_EADRIC
-- — kalecgos pass; the GetTrialOfTheChampionAI ScriptName binding
-- is instance-shimmed, the creature_template binding DB-side as
-- usual).
-- Sole-source verified: whole-server-tree grep for "boss_eadric",
-- "boss_paletress", "npc_memory", "npc_argent_soldier",
-- "spell_eadric_radiance" and "spell_paletress_summon_memory" hits
-- boss_argent_challenge.cpp only (loader carries only the
-- AddSC decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(creature, spell)
-- = DoCastSelf (phase_hunter / apothecary_hanes precedent — the
-- trailing true is the triggered port).
-- The C++ Yells enum is commented out entirely and no Talk call
-- exists anywhere in the file — no Talk arms to port.
-- Ported arms (C++-exact for all modeled arms):
-- boss_eadric: uiVenganceTimer — DoCastSelf(Vengeance 66865),
-- 10s init, 10s repeat, scheduled from combat start and
-- independent of every unbridged leg (the bDone MovePoint block
-- only runs after the unbridged DamageTaken evade; the timers all
-- run under UpdateVictim).
-- Unmodeled (no bridges — documented, not wired):
-- boss_eadric constructor — SetReactState(REACT_PASSIVE) +
-- SetFlag(UNIT_FLAG_NON_ATTACKABLE) — no react-state / flag
-- bridges (drakkari_colossus precedent).
-- boss_eadric DamageTaken — damage >= health -> damage = 0 +
-- EnterEvadeMode + SetFaction(FACTION_FRIENDLY) + bDone latch —
-- no DamageTaken hook (npc_unkor_the_ruthless precedent) + no
-- faction / evade bridges.
-- boss_eadric MovementInform (POINT_MOTION_TYPE) ->
-- instance->SetBossState(BOSS_ARGENT_CHALLENGE_E, DONE) +
-- DisappearAndDie — no motion-master bridge + instance-script
-- model absent (standing) + no despawn bridge (terestian
-- precedent).
-- boss_eadric bDone-reset MovePoint(0, 746.87, 665.87, 411.75)
-- leg — no motion-master bridge.
-- boss_eadric uiHammerJusticeTimer — InterruptNonMeleeSpells +
-- SelectTarget(Random, 0, 250, true) -> DoCast(Hammer of Justice
-- 66863) + DoCast(Hammer of the Righteous 66867), 25s repeat —
-- random-target SelectTarget bridge absent (cairne/kazzak
-- precedent).
-- boss_eadric uiRadianceTimer — DoCastAOE(Radiance 66935), 16s
-- repeat — no DoCastAOE bridge (terestian/shazzrah precedent).
-- boss_paletress — zero registration. Constructor legs
-- (SetReactState(REACT_PASSIVE) + SetFlag(NON_ATTACKABLE) +
-- RestoreFaction) — no react/flag/faction bridges; Reset's
-- RemoveAllAuras + ObjectAccessor::GetCreature(MemoryGUID)
-- RemoveFromWorld — no aura-removal (aeranas precedent) +
-- cross-AI ObjectAccessor + despawn bridges; SetData(1) ->
-- RemoveAura(SPELL_SHIELD 66515) — no SetData / aura-removal
-- bridges; DamageTaken / MovementInform — same bridges absent as
-- boss_eadric; uiHolyFireTimer — SelectTarget(Random, 0, 250,
-- true) -> DoCast(Holy Fire 66538), 9-12s init, 13s shielded /
-- 9-12s unshielded repeat — random-target SelectTarget bridge
-- absent; uiHolySmiteTimer — SelectTarget(Random, 0, 250, true) ->
-- DoCast(Smite 66536), 5-7s init, 9s shielded / 5-7s unshielded
-- repeat — random-target SelectTarget bridge absent; Renew
-- machine (HasAura(SPELL_SHIELD) gate -> urand(0,1) self-cast
-- Renew 66537 / cross-AI DoCast(pMemory, Renew)) — no aura
-- bridge + cross-AI ObjectAccessor bridge absent; the 25%
-- HealthAbovePct one-shot (InterruptNonMeleeSpells +
-- DoCastAOE(Holy Nova 66546, false) + DoCastSelf(Shield 66515) +
-- DoCastAOE(Summon Memory 66545, false) + DoCastAOE(Confess
-- 66680, false)) — no health-pct bridge (doomwalker precedent)
-- + no DoCastAOE bridge; ungated emulation of the Shield
-- self-cast leg would over-cast vs C++ (jedoga precedent);
-- JustSummoned MemoryGUID latch — summon STRAND absent
-- (standing). Entry 34928 verifiable (trial_of_the_champion.h
-- NPC_PALETRESS — kalecgos pass) but every combat arm rides an
-- absent bridge — bridgeable-but-entry-blocked does not apply;
-- nothing wired.
-- npc_memory — ENTRY UNVERIFIABLE (no NPC constant in
-- trial_of_the_champion.h or anywhere in the C++ sources; the
-- memory creatures materialize through the DB-side summon spells
-- (SPELL_MEMORY_HOGGER 66543 et al.) — belnistrasz/willix
-- precedent, no invented identifiers): zero registration; joins
-- the bridgeable-but-entry-blocked queue. Port-pattern-ready
-- rotation arms: uiWakingNightmare — DoCastSelf(Waking Nightmare
-- 66552), 7s init, 7s repeat; uiOldWoundsTimer —
-- SelectTarget(Random, 0) -> DoCast(Old Wounds 66620), 12s
-- repeat; uiShadowPastTimer — SelectTarget(Random, 1) ->
-- DoCast(Shadows Past 66619), 5s repeat — random-target
-- SelectTarget bridge absent. JustDied cross-AI leg
-- (ToTempSummon()->GetSummonerUnit()->GetAI()->SetData(1, 0)) —
-- no cross-AI SetData bridge (drakkari_colossus precedent).
-- npc_argent_soldier — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (35309 NPC_ARGENT_LIGHWIELDER / 35305 NPC_ARGENT_MONK / 35307
-- NPC_PRIESTESS — trial_of_the_champion.h kalecgos pass): zero
-- registration. Whole AI is the EscortAI waypoint machine
-- (SetData(uiType) -> per-entry AddWaypoint triplets +
-- Start(false, true); WaypointReached(0) -> SetFacingTo
-- (5.81f/4.60f/2.79f per uiWaypoint)) — escort / motion-master /
-- facing bridges absent (escort queue precedent). JustDied leg
-- (instance->SetData(DATA_ARGENT_SOLDIER_DEFEATED, +1)) —
-- instance-script model absent (standing). Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_eadric_radiance — SpellScript (66935 Radiance:
-- OnObjectAreaTargetSelect filter removes targets not in front
-- within 2.5f / beyond 40y) — no SpellScript binding bridge
-- (razelikh precedent) — documented-only.
-- spell_paletress_summon_memory — SpellScript (66545 Summon
-- Memory: random single-target filter + EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> target CastSpell(memorySpellId[
-- urand(0, 24)], caster GUID)) — no SpellScript binding bridge
-- (razelikh precedent) — documented-only.
-- C++ "Known Issues" note carried: SD%Complete is 50 % —
-- ScriptData comment states AI for Argent Soldiers is not
-- implemented and boss AI needs improvements.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler
-- (gargolmar precedent).

local ENTRY_EADRIC = 35119

local SPELL_VENGEANCE = 66865

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

-- C++ uiVenganceTimer: DoCastSelf(Vengeance), 10s init, 10s
-- repeat; independent of every unbridged leg.
local function vengeanceTick(creature, guid)
    creature:CastSpell(creature, SPELL_VENGEANCE)
    schedule(guid, "vengeance", 10000, function()
        vengeanceTick(creature, guid)
    end)
end

local function eadricEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "vengeance", 10000, function()
        vengeanceTick(creature, guid)
    end)
end

local function eadricLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function eadricDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function eadricReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EADRIC, 1, eadricEnterCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 2, eadricLeaveCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 4, eadricDied)
RegisterCreatureEvent(ENTRY_EADRIC, 23, eadricReset)

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
-- OnDied, 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
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
-- uiRadianceTimer — DoCastAOE(Radiance 66935) self-cast, 16s init,
-- 16s repeat (ungated UpdateVictim arm); the
-- spell_eadric_radiance target filter (front 2.5f / 40y cone) has
-- no SpellScript bridge — modeled as the plain self-cast AOE.
-- uiHammerJusticeTimer — InterruptNonMeleeSpells (unwired, no bridge)
-- + SelectTarget(Random, 0, 250, true) -> DoCast(target, Hammer of
-- Justice 66863) + DoCast(target, Hammer of the Righteous 66867),
-- 25s init / 25s repeat — PORTED (the "random-target SelectTarget
-- bridge absent / cairne-kazzak" verdict is retracted: randomPlayer
-- is the in-tree bridge — anomalus spark port).
-- Unmodeled (no bridges — documented, not wired):
-- boss_eadric constructor — SetReactState(REACT_PASSIVE) +
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) (SetReactState
-- IS bridged — lua_creature_events.go:524, frozen_core precedent;
-- SetFlag(59) IS bridged — lua_creature_events.go:968) — but the ctor
-- legs ride the unported announcer intro machine: the un-gating
-- (RemoveFlag(NON_ATTACKABLE) + REACT_AGGRESSIVE + AddThreat,
-- trial_of_the_champion.cpp:385-386) has no bearer, so a one-sided
-- port would leave the boss permanently passive/non-attackable.
-- boss_eadric DamageTaken — PORTED: damage >= health -> damage =
-- 0 + EnterEvadeMode (AttackStop + ClearThreatList approximation —
-- no evade bridge; the MotionMaster MovePoint home leg stays out)
-- + SetFaction(FACTION_FRIENDLY 35, bridged) — the stale "no
-- DamageTaken hook / no faction bridge" note is retracted: event 9
-- is bridged and fires pre-damage (the alar return-0,0 convention);
-- the UpdateAI bDone-reset MovePoint(0, 746.87, 665.87, 411.75) leg
-- stays out (no motion-master bridge), so no per-GUID bDone latch
-- is kept (C++ reads bDone only in UpdateAI).
-- boss_eadric MovementInform (POINT_MOTION_TYPE) ->
-- instance->SetBossState(BOSS_ARGENT_CHALLENGE_E, DONE) +
-- DisappearAndDie — no motion-master bridge + instance-script
-- model absent (standing; the old "no despawn bridge" parenthetical
-- was stale — creature:Despawn() is bridged,
-- lua_creature_events.go:214).
-- boss_eadric bDone-reset MovePoint(0, 746.87, 665.87, 411.75)
-- leg — no motion-master bridge.
-- boss_paletress — zero registration. Constructor legs
-- (SetReactState(REACT_PASSIVE) + SetFlag(NON_ATTACKABLE) +
-- RestoreFaction) — SetReactState + SetFlag(59) + SetFaction ALL
-- IS bridged (lua_creature_events.go:524/:968; eadric:51) — same
-- intro-machine grounding as boss_eadric above: the announcer
-- un-gating has no bearer; Reset's
-- RemoveAllAuras + ObjectAccessor::GetCreature(MemoryGUID)
-- RemoveFromWorld — the "no aura-removal (aeranas precedent)"
-- wording is stale (creature:RemoveAllAuras() is bridged,
-- lua_creature_events.go:169), but the paired leg is cross-AI
-- ObjectAccessor -> RemoveFromWorld with no creature bridge
-- (gameobject-only) on an unregistered npc_memory bearer;
-- SetData(1) ->
-- RemoveAura(SPELL_SHIELD 66515) — the "no aura-removal" wording
-- is stale (creature:RemoveAura is bridged), but the arm is
-- triggered cross-AI via SetData (no bridge) from unregistered
-- npc_memory JustDied; DamageTaken / MovementInform — same bridges
-- absent as
-- boss_eadric; uiHolyFireTimer — SelectTarget(Random, 0, 250,
-- true) -> DoCast(Holy Fire 66538), 9-12s init, 13s shielded /
-- 9-12s unshielded repeat; uiHolySmiteTimer — SelectTarget(Random,
-- 0, 250, true) -> DoCast(Smite 66536), 5-7s init, 9s shielded /
-- 5-7s unshielded repeat — the random-target bridge now EXISTS
-- (randomPlayer, anomalus precedent), but both timers'
-- shielded/unshielded re-arms ride the 25%-health one-shot's Shield
-- product (unported: SpellScript riders + InterruptNonMeleeSpells +
-- no instance model), so porting the casts without the machine would
-- mis-model the repeat branches; Renew
-- machine (HasAura(SPELL_SHIELD) gate -> urand(0,1) self-cast
-- Renew 66537 / cross-AI DoCast(pMemory, Renew)) — the HasAura leg IS
-- bridged (lua_objects.go:228) but the cross-AI ObjectAccessor leg is
-- absent and Shield itself is unported (one-shot machine); the 25%
-- HealthAbovePct one-shot (InterruptNonMeleeSpells +
-- DoCastAOE(Holy Nova 66546, false) + DoCastSelf(Shield 66515) +
-- DoCastAOE(Summon Memory 66545, false) + DoCastAOE(Confess
-- 66680, false)) — the whole one-shot rides the unbridged
-- HealthAbovePct(25) latch (doomwalker precedent); ungated
-- emulation of the Shield self-cast leg would over-cast vs C++
-- (jedoga precedent);
-- JustSummoned MemoryGUID latch — summon STRAND absent
-- (standing). Entry 34928 verifiable (trial_of_the_champion.h
-- NPC_PALETRESS — kalecgos pass) but every combat arm rides an
-- absent bridge — bridgeable-but-entry-blocked does not apply;
-- nothing wired.
-- boss_paletress DamageTaken — PORTED (same arm as boss_eadric,
-- boss_argent_challenge.cpp:320-328): damage >= health -> damage =
-- 0 + EnterEvadeMode (AttackStop + ClearThreatList) +
-- SetFaction(FACTION_FRIENDLY 35). Entry 34928 verifiable
-- (trial_of_the_champion.h NPC_PALETRESS — kalecgos pass), so this
-- arm registers here on its own entry even though the rest of
-- paletress stays out (constructor/Reset/UpdateAI legs on absent
-- bridges — see below).
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
-- DoCast(Shadows Past 66619), 5s repeat — the random-target bridge
-- now EXISTS (randomPlayer) but npc_memory has zero registration
-- (entry-unverifiable). JustDied cross-AI leg
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
local ENTRY_PALETRESS = 34928 -- trial_of_the_champion.h:71

local FACTION_FRIENDLY = 35 -- SharedDefines.h:231

local SPELL_VENGEANCE = 66865
local SPELL_RADIANCE = 66935
local SPELL_HAMMER_JUSTICE = 66863
local SPELL_HAMMER_RIGHTEOUS = 66867

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

-- C++ uiRadianceTimer: DoCastAOE(Radiance), 16s init, 16s repeat;
-- ungated (plain UpdateVictim). The spell_eadric_radiance target
-- filter (front 2.5f / 40y cone) has no Lua bridge — ported as the
-- plain self-cast AOE.
local function radianceTick(creature, guid)
    creature:CastSpell(creature, SPELL_RADIANCE)
    schedule(guid, "radiance", 16000, function()
        radianceTick(creature, guid)
    end)
end

-- C++ SelectTarget(Random, 0, 250, true) — random alive player on the
-- same map/instance within 250y (keleseth / thorim / archavon
-- GetPlayersInWorld precedent; the C++ target->IsAlive() re-check is
-- subsumed by the alive filter). Only threat-listed units qualify in
-- practice.
local function randomPlayer(creature, maxDist)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:GetDistance(plr) <= maxDist then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ uiHammerJusticeTimer: InterruptNonMeleeSpells (no interrupt
-- bridge — maiden precedent, documented unwired) + SelectTarget ->
-- DoCast(target, Hammer of Justice 66863) + DoCast(target, Hammer of
-- the Righteous 66867), non-triggered; 25s init, 25s repeat. No cast
-- when no candidate (C++ SelectTarget returns null on an empty
-- threat list).
local function hammerJusticeTick(creature, guid)
    local target = randomPlayer(creature, 250)
    if target then
        creature:CastSpell(target, SPELL_HAMMER_JUSTICE)
        creature:CastSpell(target, SPELL_HAMMER_RIGHTEOUS)
    end
    schedule(guid, "hammer", 25000, function()
        hammerJusticeTick(creature, guid)
    end)
end

local function eadricEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "vengeance", 10000, function()
        vengeanceTick(creature, guid)
    end)
    schedule(guid, "radiance", 16000, function()
        radianceTick(creature, guid)
    end)
    schedule(guid, "hammer", 25000, function()
        hammerJusticeTick(creature, guid)
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

-- C++ boss_eadricAI::DamageTaken (boss_argent_challenge.cpp:192-200)
-- and boss_paletressAI::DamageTaken (:320-328) — identical arms: a
-- lethal hit is absorbed (damage = 0) and the champion evades as a
-- friendly (EnterEvadeMode -> AttackStop + ClearThreatList, the
-- MovePoint home leg unbridged; SetFaction bridged). Incoming-damage
-- check, so no postDamage guard.
local function argentDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:AttackStop()
        creature:ClearThreatList()
        creature:SetFaction(FACTION_FRIENDLY)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_EADRIC, 1, eadricEnterCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 2, eadricLeaveCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 4, eadricDied)
RegisterCreatureEvent(ENTRY_EADRIC, 9, argentDamageTaken)
RegisterCreatureEvent(ENTRY_EADRIC, 23, eadricReset)
RegisterCreatureEvent(ENTRY_PALETRESS, 9, argentDamageTaken)

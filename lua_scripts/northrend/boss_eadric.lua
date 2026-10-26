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
-- uiHammerJusticeTimer — InterruptNonMeleeSpells(true) PORTED below
-- (no-op twin, unconditional per C++)
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
-- boss_paletress — combat arms PORTED below; constructor legs
-- (SetReactState(REACT_PASSIVE) + SetFlag(NON_ATTACKABLE) +
-- RestoreFaction) — SetReactState + SetFlag(59) + SetFaction ALL
-- IS bridged (lua_creature_events.go:524/:968; eadric:51) — same
-- intro-machine grounding as boss_eadric above: the announcer
-- un-gating has no bearer; Reset's RemoveAllAuras is PORTED below
-- (paletressReset, ev23) and Initialize re-inits the timers/latches;
-- the paired ObjectAccessor::GetCreature(MemoryGUID) RemoveFromWorld
-- leg stays out (cross-AI, no creature bridge, unregistered
-- npc_memory bearer);
-- SetData(1) ->
-- RemoveAura(SPELL_SHIELD 66515) — the "no aura-removal" wording
-- is stale (creature:RemoveAura is bridged), but the arm is
-- triggered cross-AI via SetData (no bridge) from unregistered
-- npc_memory JustDied; DamageTaken / MovementInform — same bridges
-- absent as
-- boss_eadric; uiHolyFireTimer — PORTED below: SelectTarget(Random,
-- 0, 250, true) via threat-list randomPlayer -> DoCast(Holy Fire
-- 66538), 9-12s init, 13s shielded / 9-12s unshielded repeat
-- (HasAura-gated re-arm reads the modeled Shield state, so the
-- unshielded branch is C++-exact while Shield is absent);
-- uiHolySmiteTimer — PORTED below: same pick -> DoCast(Smite 66536),
-- 5-7s init, 9s shielded / 5-7s unshielded repeat; Renew
-- machine — PORTED below (HasAura(SPELL_SHIELD) gate, bridged
-- lua_objects.go:228; urand(0,1): self-cast Renew 66537 on 0, the
-- cross-AI DoCast(pMemory, Renew) leg on 1 stays out — no memory
-- bearer — while the re-arm stays unconditional per C++; the
-- shielded-only decrement is modeled by a 1s poll + remainder
-- counter, since the timer freezes while Shield is absent); the 25%
-- HealthAbovePct one-shot — PORTED below as an ev9 postDamage latch
-- (UpdateAI !HealthAbovePct(25) form, Unit.h:874: fires when
-- postDamage <= floor(max*25/100) with the postDamage > 0 guard, so
-- it never fires on the absorbed lethal hit): InterruptNonMeleeSpells
-- + Holy Nova 66546 + Shield 66515 + Confess 66680 as self-casts
-- (DoCastAOE convention); Summon Memory 66545 stays out (DB-side
-- summon effect, no Go producer — Army of the Dead precedent);
-- JustSummoned MemoryGUID latch — summon STRAND absent
-- (standing). Entry 34928 verifiable (trial_of_the_champion.h
-- NPC_PALETRESS — kalecgos pass); the remaining stay-outs are the
-- constructor/intro-machine legs, MovementInform, and the
-- memory/soldier/SpellScript machines above.
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

local SPELL_HOLY_FIRE = 66538 -- boss_argent_challenge.cpp:70
local SPELL_SMITE = 66536 -- boss_argent_challenge.cpp:68
local SPELL_RENEW = 66537 -- boss_argent_challenge.cpp:72
local SPELL_SHIELD = 66515 -- boss_argent_challenge.cpp:75
local SPELL_HOLY_NOVA = 66546 -- boss_argent_challenge.cpp:74
local SPELL_CONFESS = 66680 -- boss_argent_challenge.cpp:76

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

-- C++ SelectTarget(Random, 0, 250, true) — threat-list players alive
-- within 250y (UnitAI.h: SelectTarget draws the threat list; the
-- GetPlayersInWorld instance pool used here before was the stale-pool
-- class fixed across the 2026-10-25 strips — instance players outside
-- combat are not threat-listed). Bridged via creature:GetAITargets()
-- (threat-ordered; lua_creature_events.go:345); the C++ target->IsAlive()
-- re-check is subsumed by the alive filter.
local function randomPlayer(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ uiHammerJusticeTimer: InterruptNonMeleeSpells(true) (no-op
-- twin, fired unconditionally even with no candidate) + SelectTarget ->
-- DoCast(target, Hammer of Justice 66863) + DoCast(target, Hammer of
-- the Righteous 66867), non-triggered; 25s init, 25s repeat. No cast
-- when no candidate (C++ SelectTarget returns null on an empty
-- threat list).
local function hammerJusticeTick(creature, guid)
    creature:InterruptNonMeleeSpells(true)
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
    -- C++ boss_eadricAI::Reset is Initialize() only (timers re-init);
    -- the :305 RemoveAllAuras belongs to boss_paletressAI::Reset, not
    -- eadric — a prior tick misattributed it here, fixed this tick
    -- (paletress ev23 now carries it).
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

-- Paletress combat arms (boss_argent_challenge.cpp, boss_paletressAI).
local paletressState = {}

local function getPaletressState(guid)
    local st = paletressState[guid]
    if not st then
        st = { shieldHealth = false, renewRemaining = 0 }
        paletressState[guid] = st
    end
    return st
end

-- C++ uiHolyFireTimer: SelectTarget(Random, 0, 250, true) -> Holy Fire,
-- urand(9000,12000) init; re-arm 13000 if shielded else
-- urand(9000,12000). Re-arm unconditional (the SelectTarget if only
-- wraps the cast).
local function holyFireTick(creature, guid)
    local target = randomPlayer(creature, 250)
    if target then
        creature:CastSpell(target, SPELL_HOLY_FIRE)
    end
    if creature:HasAura(SPELL_SHIELD) then
        schedule(guid, "holy_fire", 13000, function()
            holyFireTick(creature, guid)
        end)
    else
        schedule(guid, "holy_fire", math.random(9000, 12000), function()
            holyFireTick(creature, guid)
        end)
    end
end

-- C++ uiHolySmiteTimer: same pick -> Smite, urand(5000,7000) init;
-- re-arm 9000 if shielded else urand(5000,7000).
local function holySmiteTick(creature, guid)
    local target = randomPlayer(creature, 250)
    if target then
        creature:CastSpell(target, SPELL_SMITE)
    end
    if creature:HasAura(SPELL_SHIELD) then
        schedule(guid, "holy_smite", 9000, function()
            holySmiteTick(creature, guid)
        end)
    else
        schedule(guid, "holy_smite", math.random(5000, 7000), function()
            holySmiteTick(creature, guid)
        end)
    end
end

-- C++ uiRenewTimer: decrements ONLY while HasAura(SHIELD); on fire,
-- InterruptNonMeleeSpells(true) + urand(0,1): self Renew on 0, memory
-- Renew on 1 (cross-AI leg stays out, re-arm unconditional). The freeze
-- is modeled by a 1s poll holding the remainder counter (cthun poll
-- precedent); re-arm urand(15000,17000).
local function renewTick(creature, guid)
    local st = getPaletressState(guid)
    if not creature:HasAura(SPELL_SHIELD) then
        schedule(guid, "renew", 1000, function()
            renewTick(creature, guid)
        end)
        return
    end
    st.renewRemaining = st.renewRemaining - 1000
    if st.renewRemaining > 0 then
        schedule(guid, "renew", 1000, function()
            renewTick(creature, guid)
        end)
        return
    end
    creature:InterruptNonMeleeSpells(true)
    if math.random(0, 1) == 0 then
        creature:CastSpell(creature, SPELL_RENEW)
    end
    st.renewRemaining = math.random(15000, 17000)
    schedule(guid, "renew", 1000, function()
        renewTick(creature, guid)
    end)
end

local function paletressEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local st = getPaletressState(guid)
    st.shieldHealth = false
    st.renewRemaining = math.random(2000, 5000)
    schedule(guid, "holy_fire", math.random(9000, 12000), function()
        holyFireTick(creature, guid)
    end)
    schedule(guid, "holy_smite", math.random(5000, 7000), function()
        holySmiteTick(creature, guid)
    end)
    schedule(guid, "renew", 1000, function()
        renewTick(creature, guid)
    end)
end

local function paletressLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function paletressDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_paletressAI::Reset: RemoveAllAuras + Initialize (timers /
-- latches re-init); the MemoryGUID RemoveFromWorld leg stays out.
local function paletressReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    getPaletressState(guid).shieldHealth = false
    creature:RemoveAllAuras()
end

-- C++ UpdateAI 25% one-shot (!bHealth && !HealthAbovePct(25)):
-- InterruptNonMeleeSpells + Holy Nova + Shield + Confess (Summon
-- Memory stays out). Ev9 postDamage form: fires when the hit leaves
-- health at or below floor(max*25/100) without killing (postDamage >
-- 0 guard — the lethal hit is absorbed by argentDamageTaken, which
-- runs first and chains damage to 0).
local function paletressHealthLatch(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = getPaletressState(guid)
    if st.shieldHealth then
        return
    end
    local postDamage = creature:GetHealth() - damage
    local threshold = math.floor(creature:GetMaxHealth() * 25 / 100)
    if postDamage > 0 and postDamage <= threshold then
        st.shieldHealth = true
        creature:InterruptNonMeleeSpells(true)
        creature:CastSpell(creature, SPELL_HOLY_NOVA)
        creature:CastSpell(creature, SPELL_SHIELD)
        creature:CastSpell(creature, SPELL_CONFESS)
    end
end

RegisterCreatureEvent(ENTRY_EADRIC, 1, eadricEnterCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 2, eadricLeaveCombat)
RegisterCreatureEvent(ENTRY_EADRIC, 4, eadricDied)
RegisterCreatureEvent(ENTRY_EADRIC, 9, argentDamageTaken)
RegisterCreatureEvent(ENTRY_EADRIC, 23, eadricReset)
RegisterCreatureEvent(ENTRY_PALETRESS, 1, paletressEnterCombat)
RegisterCreatureEvent(ENTRY_PALETRESS, 2, paletressLeaveCombat)
RegisterCreatureEvent(ENTRY_PALETRESS, 4, paletressDied)
RegisterCreatureEvent(ENTRY_PALETRESS, 9, argentDamageTaken)
RegisterCreatureEvent(ENTRY_PALETRESS, 9, paletressHealthLatch)
RegisterCreatureEvent(ENTRY_PALETRESS, 23, paletressReset)

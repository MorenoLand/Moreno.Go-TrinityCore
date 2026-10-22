-- Icecrown Gunship Battle (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_icecrown_gunship_battle.cpp
-- (2255 lines incl. license; 10 CreatureScripts (npc_gunship,
-- npc_high_overlord_saurfang_igb, npc_muradin_bronzebeard_igb,
-- npc_zafod_boombox, npc_gunship_boarding_leader,
-- npc_gunship_boarding_add, npc_gunship_gunner, npc_gunship_rocketeer,
-- npc_gunship_mage, npc_gunship_cannon) + 17 spell_igb_* SpellScripts +
-- achievement_im_on_a_boat; all registered from inside
-- AddSC_boss_icecrown_gunship_battle(); loader decl 173 / call 368 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_lady_deathwhisper() (call 367)).
-- Entries (icecrown_citadel.h): 36939 NPC_IGB_HIGH_OVERLORD_SAURFANG
-- (:189) / 36948 NPC_IGB_MURADIN_BRONZEBEARD (:190) — captains; 36960
-- NPC_KOR_KRON_SERGEANT (:204) / 36961 NPC_SKYBREAKER_SERGEANT (:200) —
-- boarding leaders; 36957 NPC_KOR_KRON_REAVER (:204) / 36950
-- NPC_SKYBREAKER_MARINE (:199) — boarding adds; 36978
-- NPC_SKYBREAKER_MORTAR_SOLDIER (:198) / 36982 NPC_KOR_KRON_ROCKETEER
-- (:203) — rocketeers. Entry-verifiable; the CreatureScript ScriptName
-- bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_icecrown_gunship_battle" hits
-- boss_icecrown_gunship_battle.cpp only (+ the loader decl/call lines).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 9
-- OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent (GUID-keyed,
-- cancelled on events 2/23); melee is engine-driven in Go.
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- npc_high_overlord_saurfang_igb / npc_muradin_bronzebeard_igb
-- JustEngagedWith (:867-872 / :1121-1126): DoCast(me, SPELL_BATTLE_FURY
-- 69637, true) TRIGGERED — the moroes-shadowform engage precedent; the
-- team-gated damage-mod sibling (:869/:1123, instance GetData
-- DATA_TEAM_IN_INSTANCE) has no bridge. EVENT_CLEAVE (:1062-1064 /
-- :1318-1321): DoCastVictim(SPELL_CLEAVE 15284) non-triggered,
-- {2s,10s} / {2s,10s}.
-- Captains DamageTaken (:969-976 / :1223-1229): SPELL_TASTE_OF_BLOOD
-- 69634 at 65% — DoCast(me, 69634, true) TRIGGERED. UPSTREAM ASYMMETRY:
-- saurfang gates on !HasAura(69634) (:971), muradin on HasAura(69634)
-- (:1225, missing !) — replicated exactly via the HasAura bridge. The
-- never-die damage cap (damage = health-1, :975-976 / :1228-1229) rides
-- the same event-9 handler — return true, health-1 per the
-- fireCreatureDamageTaken convention (the midnight precedent).
-- npc_gunship_boarding_leader JustEngagedWith (:1516-1521):
-- EVENT_BLADESTORM {13s,18s} — DoCastAOE(SPELL_BLADESTORM 69652)
-- non-triggered, re-arm {25s,30s} (:1544-1546);
-- EVENT_WOUNDING_STRIKE {8s,10s} —
-- DoCastVictim(SPELL_WOUNDING_STRIKE 69651) non-triggered, re-arm
-- {9s,13s} (:1548-1550). The :1533 skip-while-casting /
-- already-bladestorming gate: the UNIT_STATE_CASTING leg has no bridge
-- (maiden precedent); the self-aura gate IS bridged — the callback
-- delays 1s instead of firing while HasAura(69652).
-- npc_gunship_boarding_add DamageTaken (:1455-1464):
-- SPELL_DESPERATE_RESOLVE 69647 at 25% — DoCast(me, 69647, true)
-- TRIGGERED, one-shot _usedDesperateResolve latch (per-GUID table,
-- per-creature-lifetime: C++ sets it only in the AI constructor, so it
-- survives evade — the moroes-frenzy pattern).
-- npc_gunship_rocketeer UpdateAI (:1619-1632): SPELL_ROCKET_ARTILLERY —
-- DoCastAOE(entry == 36978 and 70609 or 69678, true) TRIGGERED on a 9s
-- SpellHistory cooldown loop. Lua emulation: engage-immediate cast +
-- repeating 9s timer (C++ fires on the first UpdateAI tick once a victim
-- exists; the SpellHistory cooldown has no bridge — documented
-- deviation).
--
-- DOCUMENTED-ONLY (no registration beyond the 8 entries above):
-- npc_gunship (the ship itself): JustDied / DamageTaken victory legs,
-- cross-creature DoAction mage summon (:698), instance machine — no
-- instance bridge (the instance_ulduar precedent).
-- Captains' intro Talk chains (:989-1009 / :1243-1266) + OnGossipSelect
-- (:956-967) — intro Talk machine, out. EVENT_KEEP_PLAYER_IN_COMBAT
-- (instance DoCastSpellOnPlayers — no instance bridge),
-- EVENT_SUMMON_MAGE / EVENT_ADDS / EVENT_ADDS_BOARD_YELL /
-- EVENT_CHECK_RIFLEMAN / EVENT_CHECK_MORTAR (summon-slot machines +
-- Talk), SPELL_RENDING_THROW (:1075/:1332, attack-timer-gated — no
-- bridge), MovementInform casts (Battle Experience :603,
-- teleport/experience/melee-targeting :1404-1410, mage
-- BELOW_ZERO/SHADOW_CHANNELING :1657-1661), TriggerBurningPitch
-- conditional (:653), gunner DoSpellAttackIfReady / zafod gossip
-- (:1370-1377, no cast arms). MovementInform SetControlled(UNIT_STATE_
-- ROOT) legs — boarding_add (:1590), rocketeer (:1616), mage (:1667),
-- all POINT_MOTION_TYPE EVENT_CHARGE_PREPATH — no MovementInform
-- bridge; the slot-machine react arms ride the same unbridged
-- hooks — gunship_npc_AI::SetData ACTION_SET_SLOT
-- SetReactState(REACT_PASSIVE) (:560), boarding_addAI::SetData
-- (:1390) PASSIVE, MovementInform EVENT_CHARGE_PREPATH
-- SetReactState(REACT_AGGRESSIVE) (:604 / :1450) — no SetData /
-- MovementInform bridges (SetReactState itself IS bridged).
-- cannon (:1712-1713) PassengerBoarded !apply -> SetControlled
-- (false) then SetControlled(true) (the "cannon root flags") — no
-- PassengerBoarded bridge: all stay out.
-- The 17 spell_igb_* SpellScripts join the no-SpellScript-bridge queue;
-- achievement_im_on_a_boat has no achievement bridge.

local ENTRY_SAURFANG = 36939
local ENTRY_MURADIN = 36948
local ENTRY_KOR_KRON_SERGEANT = 36960
local ENTRY_SKYBREAKER_SERGEANT = 36961
local ENTRY_KOR_KRON_REAVER = 36957
local ENTRY_SKYBREAKER_MARINE = 36950
local ENTRY_SKYBREAKER_MORTAR = 36978
local ENTRY_KOR_KRON_ROCKETEER = 36982

local SPELL_BATTLE_FURY = 69637
local SPELL_CLEAVE = 15284
local SPELL_TASTE_OF_BLOOD = 69634
local SPELL_BLADESTORM = 69652
local SPELL_WOUNDING_STRIKE = 69651
local SPELL_DESPERATE_RESOLVE = 69647
local SPELL_ROCKET_ARTILLERY_A = 70609
local SPELL_ROCKET_ARTILLERY_H = 69678

local timers = {}
local desperateUsed = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- bronjahm-style: re-scheduling a key replaces the pending timer, so each
-- named arm has exactly one live timer at a time.
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

-- C++ :1062-1064 / :1318-1321 EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE)
-- non-triggered, {2s,10s} / {2s,10s}.
local function onCleave(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {2000, 10000}, function() onCleave(creature, guid) end)
end

-- C++ captains JustEngagedWith (:867-872 / :1121-1126): DoCast(me,
-- SPELL_BATTLE_FURY, true) TRIGGERED — the moroes-shadowform engage
-- precedent; the team-gated damage-mod sibling (:869/:1123, instance
-- GetData DATA_TEAM_IN_INSTANCE) has no bridge. The SetPhase(PHASE_COMBAT)
-- leg has no phase bridge; EVENT_CLEAVE is not phase-gated.
local function captainEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_BATTLE_FURY, true)
    schedule(guid, "cleave", {2000, 10000}, function() onCleave(creature, guid) end)
end

-- C++ captains DamageTaken (:969-976 / :1223-1229).
local function captainDamageTaken(event, creature, attacker, damage)
    -- SPELL_TASTE_OF_BLOOD at 65%: DoCast(me, 69634, true) TRIGGERED.
    -- UPSTREAM ASYMMETRY: saurfang gates on !HasAura(69634) (:971),
    -- muradin on HasAura(69634) (:1225, missing !) — replicated exactly
    -- via the HasAura bridge.
    local maxHealth = creature:GetMaxHealth()
    if maxHealth ~= 0 then
        local postHealth = creature:GetHealth() - damage
        if postHealth < math.floor(maxHealth * 65 / 100) then
            local entry = creature:GetEntry()
            local has = creature:HasAura(SPELL_TASTE_OF_BLOOD)
            if (entry == ENTRY_SAURFANG and not has)
                    or (entry == ENTRY_MURADIN and has) then
                creature:CastSpell(creature, SPELL_TASTE_OF_BLOOD, true)
            end
        end
    end
    -- Never-die damage cap (:975-976 / :1228-1229): the captains cannot
    -- be killed — the encounter ends via the gunship machine. Return the
    -- rewritten damage per the fireCreatureDamageTaken convention (the
    -- midnight precedent).
    local health = creature:GetHealth()
    if damage >= health then
        return true, health - 1
    end
end

local function captainLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function captainReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ :1544-1546 EVENT_BLADESTORM: DoCastAOE(SPELL_BLADESTORM)
-- non-triggered, re-arm {25s,30s}.
local function onBladestorm(creature, guid)
    -- C++ :1533 skips while casting or already bladestorming; the
    -- UNIT_STATE_CASTING leg has no bridge (maiden precedent) but the
    -- self-aura gate is bridged — delay 1s instead of firing into it.
    if creature:HasAura(SPELL_BLADESTORM) then
        schedule(guid, "bladestorm", 1000, function() onBladestorm(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_BLADESTORM)
    schedule(guid, "bladestorm", {25000, 30000}, function() onBladestorm(creature, guid) end)
end

-- C++ :1548-1550 EVENT_WOUNDING_STRIKE: DoCastVictim(SPELL_WOUNDING_STRIKE)
-- non-triggered, re-arm {9s,13s}.
local function onWoundingStrike(creature, guid)
    creature:CastSpell(nil, SPELL_WOUNDING_STRIKE)
    schedule(guid, "wounding", {9000, 13000}, function() onWoundingStrike(creature, guid) end)
end

-- C++ npc_gunship_boarding_leader JustEngagedWith (:1516-1521): the base
-- npc_gunship_boarding_addAI::JustEngagedWith call resolves to
-- ScriptedAI's (no cast arms); the two ScheduleEvent seeds port below.
local function leaderEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bladestorm", {13000, 18000}, function() onBladestorm(creature, guid) end)
    schedule(guid, "wounding", {8000, 10000}, function() onWoundingStrike(creature, guid) end)
end

local function leaderLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function leaderReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_gunship_boarding_add DamageTaken (:1455-1464):
-- SPELL_DESPERATE_RESOLVE at 25%, one-shot _usedDesperateResolve latch
-- (the moroes-frenzy pattern).
local function boardingAddDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if desperateUsed[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        desperateUsed[guid] = true
        creature:CastSpell(creature, SPELL_DESPERATE_RESOLVE, true)
    end
end

-- C++ _usedDesperateResolve is set only in the
-- npc_gunship_boarding_addAI constructor (:1377-1381); no Reset()
-- override exists in the gunship_npc_AI -> npc_gunship_boarding_addAI
-- chain, so the spent latch SURVIVES evade in C++. The per-GUID table
-- is keyed per creature spawn (fresh GUID per respawn), so it is never
-- cleared here — event 2/23 only cancel timers.
local function boardingAddReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_gunship_rocketeer UpdateAI (:1619-1632): DoCastAOE(entry == 36978
-- and 70609 or 69678, true) TRIGGERED on a 9s SpellHistory cooldown loop.
local function onRocketArtillery(creature, guid)
    local spell = SPELL_ROCKET_ARTILLERY_H
    if creature:GetEntry() == ENTRY_SKYBREAKER_MORTAR then
        spell = SPELL_ROCKET_ARTILLERY_A
    end
    creature:CastSpell(creature, spell, true)
    schedule(guid, "rockets", 9000, function() onRocketArtillery(creature, guid) end)
end

local function rocketeerEnterCombat(event, creature, target)
    -- C++ fires on the first UpdateAI tick once a victim exists, then every
    -- 9s; the SpellHistory cooldown has no bridge. Emulated as
    -- engage-immediate + 9s repeat — documented deviation (cooldown vs
    -- timer).
    local guid = creature:GetGUID()
    cancelTimers(guid)
    onRocketArtillery(creature, guid)
end

local function rocketeerLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function rocketeerReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SAURFANG, 1, captainEnterCombat)
RegisterCreatureEvent(ENTRY_SAURFANG, 2, captainLeaveCombat)
RegisterCreatureEvent(ENTRY_SAURFANG, 9, captainDamageTaken)
RegisterCreatureEvent(ENTRY_SAURFANG, 23, captainReset)
RegisterCreatureEvent(ENTRY_MURADIN, 1, captainEnterCombat)
RegisterCreatureEvent(ENTRY_MURADIN, 2, captainLeaveCombat)
RegisterCreatureEvent(ENTRY_MURADIN, 9, captainDamageTaken)
RegisterCreatureEvent(ENTRY_MURADIN, 23, captainReset)
RegisterCreatureEvent(ENTRY_KOR_KRON_SERGEANT, 1, leaderEnterCombat)
RegisterCreatureEvent(ENTRY_KOR_KRON_SERGEANT, 2, leaderLeaveCombat)
RegisterCreatureEvent(ENTRY_KOR_KRON_SERGEANT, 9, boardingAddDamageTaken)
RegisterCreatureEvent(ENTRY_KOR_KRON_SERGEANT, 23, leaderReset)
RegisterCreatureEvent(ENTRY_SKYBREAKER_SERGEANT, 1, leaderEnterCombat)
RegisterCreatureEvent(ENTRY_SKYBREAKER_SERGEANT, 2, leaderLeaveCombat)
RegisterCreatureEvent(ENTRY_SKYBREAKER_SERGEANT, 9, boardingAddDamageTaken)
RegisterCreatureEvent(ENTRY_SKYBREAKER_SERGEANT, 23, leaderReset)
RegisterCreatureEvent(ENTRY_KOR_KRON_REAVER, 2, boardingAddReset)
RegisterCreatureEvent(ENTRY_KOR_KRON_REAVER, 9, boardingAddDamageTaken)
RegisterCreatureEvent(ENTRY_KOR_KRON_REAVER, 23, boardingAddReset)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MARINE, 2, boardingAddReset)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MARINE, 9, boardingAddDamageTaken)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MARINE, 23, boardingAddReset)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MORTAR, 1, rocketeerEnterCombat)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MORTAR, 2, rocketeerLeaveCombat)
RegisterCreatureEvent(ENTRY_SKYBREAKER_MORTAR, 23, rocketeerReset)
RegisterCreatureEvent(ENTRY_KOR_KRON_ROCKETEER, 1, rocketeerEnterCombat)
RegisterCreatureEvent(ENTRY_KOR_KRON_ROCKETEER, 2, rocketeerLeaveCombat)
RegisterCreatureEvent(ENTRY_KOR_KRON_ROCKETEER, 23, rocketeerReset)

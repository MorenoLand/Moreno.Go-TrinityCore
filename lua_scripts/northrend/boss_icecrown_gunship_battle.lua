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
-- already-bladestorming gate: PORTED 2026-11-04 — 500ms IsCasting
-- defer (mal_ganis idiom); the self-aura gate IS bridged — the callback
-- delays 1s instead of firing while HasAura(69652).
-- npc_gunship_boarding_add DamageTaken (:1455-1464):
-- SPELL_DESPERATE_RESOLVE 69647 at 25% — DoCast(me, 69647, true)
-- TRIGGERED, one-shot _usedDesperateResolve latch (per-GUID table,
-- per-creature-lifetime: C++ sets it only in the AI constructor, so it
-- survives evade — the moroes-frenzy pattern).
-- npc_gunship_rocketeer UpdateAI (:1619-1632): SPELL_ROCKET_ARTILLERY —
-- DoCastAOE(entry == 36978 and 70609 or 69678, true) TRIGGERED on a 9s
-- SpellHistory cooldown loop; the :1624 while-casting gate is PORTED
-- 2026-11-04 as a 500ms IsCasting defer (mal_ganis idiom).
-- Lua emulation: engage-immediate cast +
-- repeating 9s timer (C++ fires on the first UpdateAI tick once a victim
-- exists; the SpellHistory cooldown has no bridge — documented
-- deviation).
--
-- DOCUMENTED-ONLY (no registration beyond the 8 entries above):
-- npc_gunship (the ship itself): JustDied / DamageTaken victory legs,
-- cross-creature DoAction mage summon (:698), instance machine — the
-- instance GetData (DATA_TEAM_IN_INSTANCE) / transport legs have no
-- bridge (the instance_ulduar precedent); the SetBossState DONE/FAIL
-- leg itself IS bridged (creature:SetBossState) but stays out on its
-- trigger + isVictory (see FRAME TRIAGE below). The :780
-- FindNearestCreature enemy-ship leg (HORDE ?
-- NPC_ORGRIMS_HAMMER : NPC_THE_SKYBREAKER, 200.0f ->
-- SPELL_TELEPORT_PLAYERS_ON_VICTORY + achievement/reputation
-- cross-creature casts) rides the same instance/transport machine,
-- and the ship entries have no Lua registration at all —
-- documented-only.
-- FRAME TRIAGE (tick 222, re-verified tick 267): the :713/:717 JustDied
-- DISENGAGE legs (other ship via FindNearestCreature 200yd + me) and the
-- :902/:906/:1155/:1159 DoAction(ACTION_ENEMY_GUNSHIP_TALK) ENGAGE legs
-- stay out as live code. The SetBossState call itself IS bridged now
-- (creature:SetBossState, lua_creature_events.go:5917, the tick-266
-- falric/marwyn precedent) — the blockers are the senders, not the
-- frame call. ENGAGE (:899/:1152 IN_PROGRESS) sender is
-- instance_icecrown_citadel.cpp:1433 ProcessEvent(
-- EVENT_ENEMY_GUNSHIP_COMBAT) — no instance-script bridge — so an ev38
-- receipt on the captains would never fire. DONE/FAIL (:710, npc_gunship
-- JustDied, ship entries NPC_THE_SKYBREAKER 37540 / NPC_ORGRIMS_HAMMER
-- 37215) rides the ship pseudo-death (DamageTaken calls JustDied at
-- lethal damage, then caps damage at health-1 so the ship never actually
-- dies), driven by cannon SpellScript damage (no-SpellScript bridge);
-- isVictory needs GetTransport entry (no bridge — verified grep of the
-- creature surface this tick), so even the DONE/FAIL leg is undecidable.
-- FindNearestCreature itself IS bridged. Captains carry no state legs:
-- ScriptedAI (not BossAI), no Reset override, no base call in
-- JustEngagedWith (:867/:1121) — no engage/reset legs exist to port.
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
-- gunship_npc_AI::JustDied (:587-591): Slot-gated
-- FindNearestCreature captain (HORDE ? 36948 Muradin : 36939
-- Saurfang, 200.0f) -> captain->AI()->SetData(
-- ACTION_CLEAR_SLOT, Index) — cross-creature AI SetData has no
-- bridge, and Slot only ever comes from the unbridged
-- ACTION_SET_SLOT machine above; none of the 6 derived entries
-- registers event 4 (OnDied) either — documented-only.
-- cannon (:1712-1713) PassengerBoarded !apply -> SetControlled
-- (false) then SetControlled(true) (the "cannon root flags") — no
-- PassengerBoarded bridge: all stay out.
-- The 17 spell_igb_* SpellScripts join the no-SpellScript-bridge queue;
-- achievement_im_on_a_boat — PORTED 2026-11-08 (tick 416): the deck-aura
-- visit counter rides deckAuraApply below; Go case in
-- engine/world/achievement_criteria_scripts.go.

local ENTRY_SAURFANG = 36939
local ENTRY_MURADIN = 36948
local ENTRY_KOR_KRON_SERGEANT = 36960
local ENTRY_SKYBREAKER_SERGEANT = 36961
local ENTRY_KOR_KRON_REAVER = 36957
local ENTRY_SKYBREAKER_MARINE = 36950
local ENTRY_SKYBREAKER_MORTAR = 36978
local ENTRY_KOR_KRON_ROCKETEER = 36982

-- spell_igb_on_gunship_deck (boss_icecrown_gunship_battle.cpp:181-182,
-- :1808-1817): deck-aura ids + the ship each aura's visits count on.
-- C++-verified mapping (NOT the inverted one in the tick-416 triage
-- draft): the Skybreaker-deck aura (70120, Horde raid) counts on
-- Orgrim's Hammer (37215); the Orgrim's Hammer-deck aura (70121,
-- Alliance raid) counts on the Skybreaker (37540). The visits live on
-- the raid's OWN ship AI — the criteria target is that ship.
local SPELL_ON_SKYBREAKER_DECK = 70120
local SPELL_ON_ORGRIMS_HAMMER_DECK = 70121
local ENTRY_THE_SKYBREAKER = 37540 -- icecrown_citadel.h:191
local ENTRY_ORGRIMS_HAMMER = 37215 -- icecrown_citadel.h:192
local ACTION_SHIP_VISITS = 5 -- boss_icecrown_gunship_battle.cpp:212

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
    local guid = creature:GetGUIDLow()
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
    cancelTimers(creature:GetGUIDLow())
    -- C++ npc_high_overlord_saurfang_igb / npc_muradin_bronzebeard_igb
    -- EnterEvadeMode (:875-885 / :1128-1138): IsAlive gate + CombatStop(true)
    -- + EngagementOver() + MoveTargetedHome() + Reset(). The Reset() leg
    -- rides ev23 (captainReset below; Go's evade funnel fires On_Reset
    -- before On_LeaveCombat); EngagementOver() has no bridge. PORTED
    -- 2026-11-08 (tick 493): CombatStop + MoveHome are live
    -- (lua_creature_events.go:2887 / :4989). Not extended to the boarding
    -- adds/gunners/rocketeers: the base gunship_npc_AI::EnterEvadeMode
    -- (:577-585) is gated on alive AND in-combat, undecidable at ev2 time.
    if creature:IsAlive() then
        creature:CombatStop(true)
        creature:MoveHome()
    end
end

local function captainReset(event, creature)
    creature:SetRegenerateHealth(false)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ :1544-1546 EVENT_BLADESTORM: DoCastAOE(SPELL_BLADESTORM)
-- non-triggered, re-arm {25s,30s}.
local function onBladestorm(creature, guid)
    -- C++ :1533 pre-loop gate (before ExecuteEvent): skip while casting
    -- or already bladestorming — 500ms IsCasting defer, no re-arm
    -- consumed (the mal_ganis idiom); the self-aura leg delays 1s.
    if creature:IsCasting() then
        schedule(guid, "bladestorm", 500, function() onBladestorm(creature, guid) end)
        return
    end
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
    -- C++ :1533 UpdateAI early-returns while casting or bladestorming,
    -- so the wounding arm stalls with bladestorm too (not just the
    -- bladestorm arm); the casting leg is a 500ms IsCasting defer
    -- (mal_ganis idiom), the aura leg delays 1s, mirroring onBladestorm.
    if creature:IsCasting() then
        schedule(guid, "wounding", 500, function() onWoundingStrike(creature, guid) end)
        return
    end
    if creature:HasAura(SPELL_BLADESTORM) then
        schedule(guid, "wounding", 1000, function() onWoundingStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_WOUNDING_STRIKE)
    schedule(guid, "wounding", {9000, 13000}, function() onWoundingStrike(creature, guid) end)
end

-- C++ npc_gunship_boarding_leader JustEngagedWith (:1516-1521): the base
-- npc_gunship_boarding_addAI::JustEngagedWith call resolves to
-- ScriptedAI's (no cast arms); the two ScheduleEvent seeds port below.
local function leaderEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "bladestorm", {13000, 18000}, function() onBladestorm(creature, guid) end)
    schedule(guid, "wounding", {8000, 10000}, function() onWoundingStrike(creature, guid) end)
end

local function leaderLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function leaderReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_gunship_boarding_add DamageTaken (:1455-1464):
-- SPELL_DESPERATE_RESOLVE at 25%, one-shot _usedDesperateResolve latch
-- (the moroes-frenzy pattern).
local function boardingAddDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
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
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_gunship_rocketeer UpdateAI (:1619-1632): DoCastAOE(entry == 36978
-- and 70609 or 69678, true) TRIGGERED on a 9s SpellHistory cooldown loop.
local function onRocketArtillery(creature, guid)
    -- C++ :1624 UpdateAI gate: return while casting — 500ms IsCasting
    -- defer, no 9s re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "rockets", 500, function() onRocketArtillery(creature, guid) end)
        return
    end
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
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    onRocketArtillery(creature, guid)
end

local function rocketeerLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function rocketeerReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
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

-- C++ spell_igb_on_gunship_deck::HandleApply
-- (boss_icecrown_gunship_battle.cpp:1808-1817): when the enemy-deck aura
-- applies to a player, the nearest gunship's AI counts one visit for
-- that player's GUID (SetGUID, ACTION_SHIP_VISITS); GetData returns the
-- max over players (:815-827). No creature:SetGUID bridge exists, so the
-- per-player counts live in this Lua table (keyed [gunshipGUID][playerGUID])
-- and the running max is written into the gunship's ScriptData store —
-- exactly the store bossLatchData reads for the Go case. Dormant until
-- the deck auras (70120/70121) are applied by the engine.
local shipVisits = {}

local function deckAuraApply(event, aura, target)
    if not target or target:GetObjectType() ~= "Player" then
        return
    end
    -- The aura hook passes (event, aura, target) only — the spell id
    -- rides the aura object (lua_aura.go GetAuraId).
    local spellId = aura:GetAuraId()
    local shipEntry = nil
    if spellId == SPELL_ON_SKYBREAKER_DECK then
        shipEntry = ENTRY_ORGRIMS_HAMMER
    elseif spellId == SPELL_ON_ORGRIMS_HAMMER_DECK then
        shipEntry = ENTRY_THE_SKYBREAKER
    else
        return
    end
    local ship = target:FindNearestCreature(shipEntry, 200.0)
    if not ship then
        return
    end
    local shipGuid = ship:GetGUIDLow()
    local per = shipVisits[shipGuid]
    if not per then
        per = {}
        shipVisits[shipGuid] = per
    end
    local playerGuid = target:GetGUIDLow()
    local visits = (per[playerGuid] or 0) + 1
    per[playerGuid] = visits
    if visits > ship:GetData(ACTION_SHIP_VISITS) then
        ship:SetData(ACTION_SHIP_VISITS, visits)
    end
end

RegisterAuraEvent(SPELL_ON_SKYBREAKER_DECK, 2, deckAuraApply)
RegisterAuraEvent(SPELL_ON_ORGRIMS_HAMMER_DECK, 2, deckAuraApply)

-- Halls of Reflection gauntlet-trash AIs — Lua port of the five
-- npc_gauntlet_trash CreatureScripts in
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (2916 lines; part of the 25 scripts registered from inside
-- AddSC_halls_of_reflection(); loader decl 167 / call 362 per
-- northrend_script_loader.cpp — the SECOND group of the "// Halls of
-- Reflection" block in AddNorthrendScripts(), immediately after
-- AddSC_instance_halls_of_reflection() (call 361), before
-- AddSC_boss_falric() (call 363)).
-- Entries (halls_of_reflection.h lines 73-77; the CreatureScript
-- ScriptName bindings are DB-side as usual):
--   38175 ghostly priest (npc_ghostly_priest, C++ :1503-1572)
--   38172 phantom mage (npc_phantom_mage, C++ :1574-1647)
--   38177 shadowy mercenary (npc_shadowy_mercenary, C++ :1681-1740)
--   38173 spectral footman (npc_spectral_footman, C++ :1742-1794)
--   38176 tortured rifleman (npc_tortured_rifleman, C++ :1796-1878)
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent ({min,max} table = randtime
-- range), GUID-keyed, cancelled on events 2/4/23 (the bronjahm /
-- devourer_of_souls convention); creature:CastSpell(nil, spellId) =
-- victim-cast (bridge falls back to TargetGUID);
-- creature:CastSpell(creature, spellId) = self-cast (the DoCastAOE
-- convention); all casts here are non-triggered. Random targets use
-- the threat-list helper (GetAITargets) filtered to the C++ SelectTarget
-- distance/playerOnly; melee is
-- engine-driven in Go, like C++ DoMeleeAttackIfReady.
--
-- PORTED ARMS (C++ line refs; C++-exact for the modeled arms):
-- npc_ghostly_priest JustEngagedWith (:1512-1518) — SHADOW_WORD_PAIN
--   72318 at {6000,15000}ms (:1514), CIRCLE_OF_DESTRUCTION 72320 at
--   12000ms (:1515), COWER_IN_FEAR 72321 at 10000ms (:1516).
-- EVENT_SHADOW_WORD_PAIN (:1532-1536) — random player within 40 yd
--   (:1533), cast 72318 (:1534), re-arm {6000,15000}ms (:1535).
-- EVENT_CIRCLE_OF_DESTRUCTION (:1537-1541) — random player within
--   10 yd (:1538), cast 72320 (:1539), re-arm 12000ms (:1540).
-- EVENT_COWER_IN_FEAR (:1542-1546) — random player within 20 yd
--   (:1543), cast 72321 (:1544), re-arm 10000ms (:1545).
-- npc_phantom_mage JustEngagedWith (:1588-1595) — FIREBALL 72163 at
--   3000ms (:1589), FLAMESTRIKE 72169 at 6000ms (:1590), FROSTBOLT
--   72166 at 9000ms (:1591), CHAINS_OF_ICE 72121 at 12000ms (:1592),
--   HALLUCINATION 72342 at 40000ms one-shot (:1593).
-- EVENT_FIREBALL (:1610-1614) — random player within 40 yd, cast
--   72163 (:1612), re-arm 15000ms.
-- EVENT_FLAMESTRIKE (:1604-1608) — random player within 40 yd, cast
--   72169 (:1617), re-arm 15000ms.
-- EVENT_FROSTBOLT (:1609-1613) — random player within 40 yd, cast
--   72166 (:1622), re-arm 15000ms.
-- EVENT_CHAINS_OF_ICE (:1614-1618) — random threat-list unit, no distance
--   (playerOnly=false, unlike every other arm here)
--   (:1626), cast 72121 (:1627), re-arm 15000ms.
-- EVENT_HALLUCINATION (:1630-1634) — non-triggered self-cast 72342
--   (:1633), no re-arm (one-shot, like C++).
-- npc_shadowy_mercenary JustEngagedWith (:1690-1696) — SHADOW_STEP
--   72326 at 23000ms (:1691), DEADLY_POISON 72329 at 5000ms (:1692),
--   ENVENOMED_DAGGER_THROW 72333 at 10000ms (:1693), KIDNEY_SHOT
--   72335 at 12000ms (:1694).
-- EVENT_SHADOW_STEP (:1710-1714) — random player within 100 yd
--   (:1711), cast 72326 (:1712), re-arm 8000ms (:1713).
-- EVENT_DEADLY_POISON (:1707-1710) — victim-cast 72329 (:1716),
--   re-arm 10000ms (:1717).
-- EVENT_ENVENOMED_DAGGER_THROW (:1711-1715) — random player within
--   40 yd (:1720), cast 72333 (:1721), re-arm 10000ms (:1722).
-- EVENT_KIDNEY_SHOT (:1716-1719) — victim-cast 72335 (:1725), re-arm
--   10000ms (:1726).
-- npc_spectral_footman JustEngagedWith (:1751-1756) —
--   SPECTRAL_STRIKE 72198 at 14000ms (:1752), SHIELD_BASH 72194 at
--   10000ms (:1753), TORTURED_ENRAGE 72203 at 15000ms (:1754).
-- EVENT_SPECTRAL_STRIKE (:1762-1765) — victim-cast 72198 (:1771),
--   re-arm 5000ms (:1772).
-- EVENT_SHIELD_BASH (:1766-1769) — victim-cast 72194 (:1775), re-arm
--   5000ms (:1776).
-- EVENT_TORTURED_ENRAGE (:1770-1773) — self-cast 72203 (:1779),
--   re-arm 15000ms (:1780).
-- npc_tortured_rifleman JustEngagedWith (:1805-1811) — SHOOT 72208 at
--   1ms (:1806), CURSED_ARROW 72222 at 7000ms (:1807), FROST_TRAP
--   72215 at 10000ms (:1808), ICE_SHOT 72268 at 15000ms (:1809).
-- EVENT_SHOOT (:1825-1829) — random player within 40 yd, cast 72208
--   (:1827), re-arm 2000ms (:1828).
-- EVENT_CURSED_ARROW (:1822-1826) — random player within 40 yd, cast
--   72222 (:1832), re-arm 10000ms (:1833).
-- EVENT_FROST_TRAP (:1827-1830) — self-cast 72215 (:1836), re-arm
--   30000ms (:1837).
-- EVENT_ICE_SHOT (:1831-1835) — random player within 40 yd, cast
--   72268 (:1841), re-arm 15000ms (:1842).
--
-- PORTED 2026-11-13 (round 58, R58-1): npc_phantom_hallucination
-- (C++ :1649-1679) — entry 38567 TDB-verified ('Phantom Hallucination',
-- ScriptName 'npc_phantom_hallucination', 0 spawns, summon-only via
-- 72342). Reset DoZoneInCombat (:1655-1658) rides ev23
-- (hallucinationReset below, no Well of Souls cast — the override
-- replaces the base Reset); JustDied DoCastAOE 72344 (:1667-1671)
-- rides ev4 (hallucinationDied); the owner-aura EnterEvadeMode guard
-- (:1660-1665) rides ev2 (hallucinationEvade, via GetSummonerUnit —
-- GetOwner resolves players only); the mage rotation rides ev1
-- (mageEngaged). The old "no bridges / nothing is registered" note
-- is retracted: DoZoneInCombat is bridged (lua_creature_events.go:2268)
-- and the TDB row supplies the entry.
-- npc_gauntlet_trash::Reset (C++ :1470-1474) — PORTED 2026-11-08
--   (tick 492): the triggered self-cast SPELL_WELL_OF_SOULS 72630
--   (halls_of_reflection.h :157) rides ev23 (trashReset below). The
--   old "Go fires 23/5 only on the respawn path" claim is STALE —
--   fireCreatureSpawned fires ev23+ev5 on first spawn too
--   (creaturemotion.go:1240-1262, Eluna initial-spawn leg). Go ev23
--   also fires on evade (triggerCreatureEvade, the Eluna
--   EnterEvadeMode On_Reset-first order), where the re-cast is a
--   harmless re-application of the spawn visual.
-- npc_gauntlet_trash::EnterEvadeMode (C++ :1476-1480) — PORTED
--   2026-11-08 (tick 492): the instance->SetData(DATA_WAVE_COUNT,
--   NOT_STARTED) leg rides ev2 (trashEvade below) as an unconditional
--   write — the C++ GetData gate has no Lua bridge (no instance
--   GetData), and the instance event-8 handler carries the
--   _waveCount != 0 guard (tick 491). The _events.Reset() half is
--   covered by the event-2 timer cancel. phantom_mage's
--   HasAura(AURA_HALLUCINATION 72343) evade guard (:1583-1587) IS
--   bridged — creature:HasAura (engine/world/lua_objects.go) — so
--   mageEvade skips the evade (event 2 returns true to veto the Go
--   base evade, event 23 keeps the timers) while the hallucination
--   aura is up.
-- ghostly_priest EVENT_DARK_MENDING (:1547-1559) —
--   DoSelectLowestHpFriendly(40, ...) (:1550) targets a missing-HP
--   friendly CREATURE ally; Lua exposes only GetPlayersInWorld, no
--   creature enumeration, so the 20s/5s timer is dropped, not
--   re-targeted at a player.
-- phantom_mage EVENT_HALLUCINATION (:1630-1634) —
--   me->RemoveAllAuras() (:1632) now ported via creature:RemoveAllAuras()
--   (bridged lua_creature_events.go:166, Freywinn precedent); the old
--   "no RemoveAllAuras bridge" claim was stale and is retracted.
--   The non-triggered self-cast 72342 ports after the strip, so the
--   invisibility no longer breaks early on dotted mages.
--
-- Cast-state gates PORTED 2026-11-04: all 19 timer arms defer 500ms on
-- creature:IsCasting() (live, lua_creature_events.go:1950), no re-arm
-- consumed (the mal_ganis idiom), under the five trash UpdateAI pre-loop
-- CASTING returns (:1527/:1605/:1705/:1765/:1820); the stale "no
-- UNIT_STATE_CASTING model, timers fire unconditionally (maiden
-- precedent)" claim is retracted. Deviations from C++: SelectTarget's
-- "valid attackable target" nuance is
-- approximated by alive threat-list entries within the C++ distance.

local ENTRY_GHOSTLY_PRIEST    = 38175
local ENTRY_PHANTOM_MAGE      = 38172
local ENTRY_SHADOWY_MERCENARY = 38177
local ENTRY_SPECTRAL_FOOTMAN  = 38173
local ENTRY_TORTURED_RIFLEMAN = 38176

local SPELL_WELL_OF_SOULS         = 72630
local DATA_WAVE_COUNT             = 6 -- halls_of_reflection.h:45
local BOSS_STATE_NOT_STARTED      = 0 -- InstanceScript.h:68-76

local ENTRY_PHANTOM_HALLUCINATION = 38567
local SPELL_HALLUCINATION_2       = 72344
local SPELL_CIRCLE_OF_DESTRUCTION = 72320
local SPELL_COWER_IN_FEAR         = 72321
local SPELL_FIREBALL              = 72163
local SPELL_FLAMESTRIKE           = 72169
local SPELL_FROSTBOLT             = 72166
local SPELL_CHAINS_OF_ICE         = 72121
local SPELL_HALLUCINATION         = 72342
local AURA_HALLUCINATION          = 72343
local SPELL_SHADOW_STEP           = 72326
local SPELL_DEADLY_POISON         = 72329
local SPELL_ENVENOMED_DAGGER_THROW = 72333
local SPELL_KIDNEY_SHOT           = 72335
local SPELL_SPECTRAL_STRIKE       = 72198
local SPELL_SHIELD_BASH           = 72194
local SPELL_TORTURED_ENRAGE       = 72203
local SPELL_SHOOT                 = 72208
local SPELL_CURSED_ARROW          = 72222
local SPELL_FROST_TRAP            = 72215
local SPELL_ICE_SHOT              = 72268

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, dist, playerOnly):
-- the threat list (GetAITargets) filtered to alive entries within dist
-- yards; playersOnly=true restricts to players, playersOnly=false =
-- any threat-list unit. maxDist nil = no distance filter (CHAINS_OF_ICE,
-- SelectTarget(Random) defaults, playerOnly=false).
local function randomPlayer(creature, maxDist, playersOnly)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if p:GetObjectType() ~= "Player" then
            targetReach = p:GetCombatReach()
        end
        if not p:IsDead()
                and (playersOnly == false or p:GetObjectType() == "Player")
                and (maxDist == nil or creature:GetDistance(p) - creature:GetCombatReach() - targetReach < maxDist) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function alive(guid)
    return timers[guid] ~= nil
end

-- C++ npc_gauntlet_trash::EnterEvadeMode :1476-1480: the wave-count
-- wipe write; the instance event-8 handler runs DO_WIPE only when
-- the stored count is non-zero (tick 491).
local function setWaveCountWipe(creature)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data:SetData(DATA_WAVE_COUNT, BOSS_STATE_NOT_STARTED)
        end
    end
end

-- C++ npc_gauntlet_trash::Reset :1470-1474: triggered self-cast
-- SPELL_WELL_OF_SOULS + _events.Reset() (the timer cancel).
local function trashReset(event, creature)
    creature:CastSpell(creature, SPELL_WELL_OF_SOULS, true)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_gauntlet_trash::EnterEvadeMode :1476-1480: _events.Reset()
-- (the timer cancel) + the wave-count wipe write. Death (ev4) does
-- NOT run this — C++ JustDied never calls EnterEvadeMode.
local function trashEvade(event, creature)
    cancelTimers(creature:GetGUIDLow())
    setWaveCountWipe(creature)
end

-- Ghostly priest -------------------------------------------------------
-- C++ :1532-1536: random player 40 yd, cast 72318, re-arm {6000,15000}.
local function priestShadowWordPain(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "swp", 500, function() priestShadowWordPain(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_WORD_PAIN)
    end
    schedule(guid, "swp", {6000, 15000}, function() priestShadowWordPain(creature, guid) end)
end

-- C++ :1537-1541: random player 10 yd, cast 72320, re-arm 12000.
local function priestCircleOfDestruction(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "cod", 500, function() priestCircleOfDestruction(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 10, true)
    if target then
        creature:CastSpell(target, SPELL_CIRCLE_OF_DESTRUCTION)
    end
    schedule(guid, "cod", 12000, function() priestCircleOfDestruction(creature, guid) end)
end

-- C++ :1542-1546: random player 20 yd, cast 72321, re-arm 10000.
local function priestCowerInFear(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "cif", 500, function() priestCowerInFear(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 20, true)
    if target then
        creature:CastSpell(target, SPELL_COWER_IN_FEAR)
    end
    schedule(guid, "cif", 10000, function() priestCowerInFear(creature, guid) end)
end

-- C++ JustEngagedWith :1512-1518.
local function priestEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "swp", {6000, 15000}, function() priestShadowWordPain(creature, guid) end)
    schedule(guid, "cod", 12000, function() priestCircleOfDestruction(creature, guid) end)
    schedule(guid, "cif", 10000, function() priestCowerInFear(creature, guid) end)
end

-- Phantom mage ---------------------------------------------------------
-- C++ :1610-1614: random player 40 yd, cast 72163, re-arm 15000.
local function mageFireball(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() mageFireball(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 15000, function() mageFireball(creature, guid) end)
end

-- C++ :1604-1608: random player 40 yd, cast 72169, re-arm 15000.
local function mageFlamestrike(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "flamestrike", 500, function() mageFlamestrike(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_FLAMESTRIKE)
    end
    schedule(guid, "flamestrike", 15000, function() mageFlamestrike(creature, guid) end)
end

-- C++ :1609-1613: random player 40 yd, cast 72166, re-arm 15000.
local function mageFrostbolt(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "frostbolt", 500, function() mageFrostbolt(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", 15000, function() mageFrostbolt(creature, guid) end)
end

-- C++ :1614-1618: random player (no distance), cast 72121, re-arm 15000.
local function mageChainsOfIce(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "chains", 500, function() mageChainsOfIce(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, nil, false)
    if target then
        creature:CastSpell(target, SPELL_CHAINS_OF_ICE)
    end
    schedule(guid, "chains", 15000, function() mageChainsOfIce(creature, guid) end)
end

-- C++ :1630-1634: me->RemoveAllAuras() (bridged at
-- lua_creature_events.go:166, Freywinn precedent) then one-shot
-- non-triggered self-cast 72342 — C++ order.
local function mageHallucination(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "hallucination", 500, function() mageHallucination(creature, guid) end)
        return
    end
    creature:RemoveAllAuras()
    creature:CastSpell(creature, SPELL_HALLUCINATION)
end

-- C++ JustEngagedWith :1588-1595.
local function mageEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "fireball", 3000, function() mageFireball(creature, guid) end)
    schedule(guid, "flamestrike", 6000, function() mageFlamestrike(creature, guid) end)
    schedule(guid, "frostbolt", 9000, function() mageFrostbolt(creature, guid) end)
    schedule(guid, "chains", 12000, function() mageChainsOfIce(creature, guid) end)
    schedule(guid, "hallucination", 40000, function() mageHallucination(creature, guid) end)
end

-- C++ npc_phantom_mageAI::EnterEvadeMode (:1583-1587): while the mage
-- has the hallucination aura up, the whole base evade is skipped —
-- returning true from the event-2 handler vetoes the Go base evade
-- (threat clear, move home, reset; engine/world/creaturemotion.go
-- triggerCreatureEvade), and the event-23 timer cancel is skipped too
-- so the mage's timers (the port's _events) keep running, like C++.
-- creature:HasAura is bridged (engine/world/lua_objects.go:228).
local function mageEvade(event, creature)
    if creature:HasAura(AURA_HALLUCINATION) then
        if event == 2 then
            return true
        end
        return
    end
    cancelTimers(creature:GetGUIDLow())
    -- C++ :1583-1587: without the aura, the base EnterEvadeMode runs.
    setWaveCountWipe(creature)
end

-- C++ npc_phantom_mageAI::Reset is the base Reset (:1470-1474): the
-- hallucination-aura guard (:1583-1587) covers EnterEvadeMode only,
-- so on spawn the cast always runs; on evade the C++ override skips
-- the whole evade (Reset included) while the aura is up, so mageReset
-- returns early with the aura up and casts otherwise.
local function mageReset(event, creature)
    if creature:HasAura(AURA_HALLUCINATION) then
        return
    end
    trashReset(event, creature)
end

-- Phantom hallucination ------------------------------------------------
-- C++ npc_phantom_hallucinationAI :1649-1679 inherits the mage AI:
-- the mage rotation rides ev1 (mageEngaged above); the mage evade
-- guard rides ev2 through hallucinationEvade below. Entry
-- TDB-verified: 38567 'Phantom Hallucination', ScriptName
-- 'npc_phantom_hallucination', 0 static spawns (summon-only, via
-- SPELL_HALLUCINATION 72342). Caveat: the 72342 summon-effect target
-- was not re-read from DBC here (not in the 63-record seed
-- Spell.dbc); the registration fires only if the live DBC's 72342
-- summons 38567.
--
-- C++ Reset :1655-1658: DoZoneInCombat(me) ONLY — the hallucination
-- overrides the base gauntlet_trash Reset, so no Well of Souls cast.
-- Go fires ev23 on evade too (On_Reset-first order); C++ base
-- EnterEvadeMode calls Reset() (CreatureAI.cpp:214) but the
-- gauntlet_trash override does not, so on evade this re-engages the
-- zone where C++ would not — same accepted pattern as the mage's
-- Well-of-Souls re-cast on evade, noted here because the effect is
-- combat, not a visual.
local function hallucinationReset(event, creature)
    creature:DoZoneInCombat()
end

-- C++ EnterEvadeMode :1660-1665: if the owner (the summoning mage)
-- exists and lacks AURA_HALLUCINATION, run the mage evade; otherwise
-- skip the whole evade. creature:GetOwner() cannot serve here — it
-- resolves player sessions only — so the summoner comes from
-- creature:GetSummonerUnit() (TempSummon::GetSummonerUnit,
-- lua_creature_events.go:5940).
local function hallucinationEvade(event, creature)
    local summoner = creature:GetSummonerUnit()
    if summoner and not summoner:HasAura(AURA_HALLUCINATION) then
        return mageEvade(event, creature)
    end
    if event == 2 then
        return true
    end
end

-- C++ JustDied :1667-1671: DoCastAOE(SPELL_HALLUCINATION_2 72344) =
-- self-cast; the timer cancel is the port's ev4 bookkeeping, as in
-- onLeaveOrDiedOrReset for the other five entries.
local function hallucinationDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:CastSpell(creature, SPELL_HALLUCINATION_2)
end

-- Shadowy mercenary ----------------------------------------------------
-- C++ :1710-1714: random player 100 yd, cast 72326, re-arm 8000.
local function mercenaryShadowStep(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "shadowstep", 500, function() mercenaryShadowStep(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 100, true)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_STEP)
    end
    schedule(guid, "shadowstep", 8000, function() mercenaryShadowStep(creature, guid) end)
end

-- C++ :1707-1710: victim-cast 72329, re-arm 10000.
local function mercenaryDeadlyPoison(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "deadlypoison", 500, function() mercenaryDeadlyPoison(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_DEADLY_POISON)
    schedule(guid, "deadlypoison", 10000, function() mercenaryDeadlyPoison(creature, guid) end)
end

-- C++ :1711-1715: random player 40 yd, cast 72333, re-arm 10000.
local function mercenaryDaggerThrow(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "daggerthrow", 500, function() mercenaryDaggerThrow(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_ENVENOMED_DAGGER_THROW)
    end
    schedule(guid, "daggerthrow", 10000, function() mercenaryDaggerThrow(creature, guid) end)
end

-- C++ :1716-1719: victim-cast 72335, re-arm 10000.
local function mercenaryKidneyShot(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "kidneyshot", 500, function() mercenaryKidneyShot(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_KIDNEY_SHOT)
    schedule(guid, "kidneyshot", 10000, function() mercenaryKidneyShot(creature, guid) end)
end

-- C++ JustEngagedWith :1690-1696.
local function mercenaryEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shadowstep", 23000, function() mercenaryShadowStep(creature, guid) end)
    schedule(guid, "deadlypoison", 5000, function() mercenaryDeadlyPoison(creature, guid) end)
    schedule(guid, "daggerthrow", 10000, function() mercenaryDaggerThrow(creature, guid) end)
    schedule(guid, "kidneyshot", 12000, function() mercenaryKidneyShot(creature, guid) end)
end

-- Spectral footman -----------------------------------------------------
-- C++ :1762-1765: victim-cast 72198, re-arm 5000.
local function footmanSpectralStrike(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "strike", 500, function() footmanSpectralStrike(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_SPECTRAL_STRIKE)
    schedule(guid, "strike", 5000, function() footmanSpectralStrike(creature, guid) end)
end

-- C++ :1766-1769: victim-cast 72194, re-arm 5000.
local function footmanShieldBash(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "bash", 500, function() footmanShieldBash(creature, guid) end)
        return
    end
    creature:CastSpell(nil, SPELL_SHIELD_BASH)
    schedule(guid, "bash", 5000, function() footmanShieldBash(creature, guid) end)
end

-- C++ :1770-1773: self-cast 72203, re-arm 15000.
local function footmanEnrage(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function() footmanEnrage(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_TORTURED_ENRAGE)
    schedule(guid, "enrage", 15000, function() footmanEnrage(creature, guid) end)
end

-- C++ JustEngagedWith :1751-1756.
local function footmanEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "strike", 14000, function() footmanSpectralStrike(creature, guid) end)
    schedule(guid, "bash", 10000, function() footmanShieldBash(creature, guid) end)
    schedule(guid, "enrage", 15000, function() footmanEnrage(creature, guid) end)
end

-- Tortured rifleman ----------------------------------------------------
-- C++ :1825-1829: random player 40 yd, cast 72208, re-arm 2000.
local function riflemanShoot(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "shoot", 500, function() riflemanShoot(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_SHOOT)
    end
    schedule(guid, "shoot", 2000, function() riflemanShoot(creature, guid) end)
end

-- C++ :1822-1826: random player 40 yd, cast 72222, re-arm 10000.
local function riflemanCursedArrow(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "cursedarrow", 500, function() riflemanCursedArrow(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_CURSED_ARROW)
    end
    schedule(guid, "cursedarrow", 10000, function() riflemanCursedArrow(creature, guid) end)
end

-- C++ :1827-1830: self-cast 72215, re-arm 30000.
local function riflemanFrostTrap(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "frosttrap", 500, function() riflemanFrostTrap(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FROST_TRAP)
    schedule(guid, "frosttrap", 30000, function() riflemanFrostTrap(creature, guid) end)
end

-- C++ :1831-1835: random player 40 yd, cast 72268, re-arm 15000.
local function riflemanIceShot(creature, guid)
    if not alive(guid) then return end
    if creature:IsCasting() then
        schedule(guid, "iceshot", 500, function() riflemanIceShot(creature, guid) end)
        return
    end
    local target = randomPlayer(creature, 40, true)
    if target then
        creature:CastSpell(target, SPELL_ICE_SHOT)
    end
    schedule(guid, "iceshot", 15000, function() riflemanIceShot(creature, guid) end)
end

-- C++ JustEngagedWith :1805-1811.
local function riflemanEngaged(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shoot", 1, function() riflemanShoot(creature, guid) end)
    schedule(guid, "cursedarrow", 7000, function() riflemanCursedArrow(creature, guid) end)
    schedule(guid, "frosttrap", 10000, function() riflemanFrostTrap(creature, guid) end)
    schedule(guid, "iceshot", 15000, function() riflemanIceShot(creature, guid) end)
end

local function onLeaveOrDiedOrReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_GHOSTLY_PRIEST, 1, priestEngaged)
RegisterCreatureEvent(ENTRY_GHOSTLY_PRIEST, 2, trashEvade)
RegisterCreatureEvent(ENTRY_GHOSTLY_PRIEST, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_GHOSTLY_PRIEST, 23, trashReset)

RegisterCreatureEvent(ENTRY_PHANTOM_MAGE, 1, mageEngaged)
RegisterCreatureEvent(ENTRY_PHANTOM_MAGE, 2, mageEvade)
RegisterCreatureEvent(ENTRY_PHANTOM_MAGE, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_PHANTOM_MAGE, 23, mageReset)

RegisterCreatureEvent(ENTRY_SHADOWY_MERCENARY, 1, mercenaryEngaged)
RegisterCreatureEvent(ENTRY_SHADOWY_MERCENARY, 2, trashEvade)
RegisterCreatureEvent(ENTRY_SHADOWY_MERCENARY, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_SHADOWY_MERCENARY, 23, trashReset)

RegisterCreatureEvent(ENTRY_SPECTRAL_FOOTMAN, 1, footmanEngaged)
RegisterCreatureEvent(ENTRY_SPECTRAL_FOOTMAN, 2, trashEvade)
RegisterCreatureEvent(ENTRY_SPECTRAL_FOOTMAN, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_SPECTRAL_FOOTMAN, 23, trashReset)

RegisterCreatureEvent(ENTRY_TORTURED_RIFLEMAN, 1, riflemanEngaged)
RegisterCreatureEvent(ENTRY_TORTURED_RIFLEMAN, 2, trashEvade)
RegisterCreatureEvent(ENTRY_TORTURED_RIFLEMAN, 4, onLeaveOrDiedOrReset)
RegisterCreatureEvent(ENTRY_TORTURED_RIFLEMAN, 23, trashReset)

RegisterCreatureEvent(ENTRY_PHANTOM_HALLUCINATION, 1, mageEngaged)
RegisterCreatureEvent(ENTRY_PHANTOM_HALLUCINATION, 2, hallucinationEvade)
RegisterCreatureEvent(ENTRY_PHANTOM_HALLUCINATION, 4, hallucinationDied)
RegisterCreatureEvent(ENTRY_PHANTOM_HALLUCINATION, 23, hallucinationReset)

-- Faction Champions (Trial of the Crusader) — defensive self-buff timer arms
-- ported from
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- Each champion's CreatureScript struct (npc_toc_druid, npc_toc_paladin,
-- npc_toc_shadow_priest, npc_toc_mage, npc_toc_hunter, npc_toc_boomkin,
-- npc_toc_warrior, npc_toc_dk, npc_toc_rogue, npc_toc_retro_paladin) runs
-- an EventMap; ONLY the defensive self-buff case arm is modeled here per
-- champion — the rest of each champion's rotation is unported (single-arm
-- territory). Creature entries from trial_of_the_crusader.h NPC_ALLIANCE_*
-- / NPC_HORDE_* (lines 180-208).
-- C++ arm shape (e.g. npc_toc_druid EVENT_HEAL_BARKSKIN :817):
--   case EVENT_HEAL_BARKSKIN:
--       if (HealthBelowPct(30))
--       { DoCast(me, SPELL_BARKSKIN); events.RescheduleEvent(EVENT_HEAL_BARKSKIN, 60s); }
--       else events.RescheduleEvent(EVENT_HEAL_BARKSKIN, 3s);
--       return;
-- The check runs at timer-fire time on CURRENT health (post-damage);
-- HealthBelowPct is integer strict-< (Unit.h: HealthBelowPct(N) =
-- GetHealth() < CountPctFromMaxHealth(N), floor). Timers are armed at
-- C++ Reset with an initial randtime and only pump while the EventMap
-- ticks in combat — modeled by arming on enter combat (event 1) and
-- canceling on events 2/4/23 (gargolmar precedent). C++ reschedules are
-- modeled by re-arming the same timer key with the cast-cooldown delay
-- after a cast, or the short check delay otherwise.
-- All casts are DoCast default (triggered=false) self-casts.
-- Unmodeled (no bridges or out of single-arm scope — documented, not wired):
-- - The full rotation of each champion (DoSelectLowestHpFriendly heals,
--   SelectTarget random-player DPS arms, EnemiesInRange checks) is
--   unported; only the HealthBelowPct defensive arm per site fires.
-- - boss_faction_championsAI base: EVENT_THREAT 5s + heroic
--   EVENT_REMOVE_CC 5s (instance-gated), UpdateThreat/UpdatePower,
--   RemoveCC, JustEngagedWith: DoCast(me, SPELL_ANTI_AOE 68595, true)
--   (:646) + SetCombatPulseDelay(5) (:647) + DoZoneInCombat (:649) are
--   PORTED 2026-10-23 in C++ arm order (the old "no bridges" claim was
--   stale — triggered CastSpell and the SetCombatPulseDelay bridge at
--   engine/world/lua_creature_events.go:902 are both live); :648
--   setActive(true) has no bridge; the :650-651 SetData(2, IN_PROGRESS)
--   -> summons.DoZoneInCombat() leg stays out (summons-container relay +
--   instance-script model absent). AttackStart model, boundary —
--   instance/threat-motion model absent.
-- - EVENT_HEAL_HAND_OF_PROTECTION / EVENT_DPS_HAND_OF_PROTECTION
--   (cross-target DoCast(target, ...) friendly-select arms — random-friend
--   bridge; out of the single defensive-arm scope).
-- - npc_toc_shaman / npc_toc_priest / npc_toc_warlock /
--   npc_toc_enh_shaman / pets (npc_toc_pet_warlock, npc_toc_pet_hunter)
--   have no HealthBelowPct arms — not modeled.
-- - The ToC instance controller (DATA_FACTION_CRUSADERS / champion
--   spawning, SetData 2 arms) — instance-script model absent.

local SPELL_DIVINE_SHIELD = 66010
local SPELL_FORBEARANCE = 25771
local SPELL_DISPERSION = 65544
local SPELL_ICE_BLOCK = 65802
local SPELL_DETERRENCE = 65871
local SPELL_RETALIATION = 65932
local SPELL_ICEBOUND_FORTITUDE = 66023
local SPELL_CLOAK = 65961
local SPELL_ANTI_AOE = 68595 -- boss_faction_champions.cpp:42 (JustEngagedWith :646 DoCast(me, ..., true))

-- cfg: pct threshold, spell self-cast id, forbearance flag (optional,
-- blocks the cast and falls back to the short check like C++),
-- initMin/initMax (ms, C++ Reset initial schedule), checkDelay (ms,
-- reschedule when NOT below threshold), castDelay (ms, cooldown
-- reschedule after a cast).
local function cfg(pct, spell, initMin, initMax, checkDelay, castDelay, forbearance)
    return {
        pct = pct, spell = spell,
        initMin = initMin, initMax = initMax,
        checkDelay = checkDelay, castDelay = castDelay,
        forbearance = forbearance,
    }
end

local PALADIN_SHIELD = cfg(30, SPELL_DIVINE_SHIELD, 20000, 20000, 5000, 300000, SPELL_FORBEARANCE)
local SHADOW_DISPERSION = cfg(40, SPELL_DISPERSION, 20000, 40000, 5000, 180000)
local MAGE_ICE_BLOCK = cfg(30, SPELL_ICE_BLOCK, 10000, 20000, 5000, 300000)
local HUNTER_DETERRENCE = cfg(30, SPELL_DETERRENCE, 10000, 20000, 10000, 150000)
local WARRIOR_RETALIATION = cfg(50, SPELL_RETALIATION, 5000, 20000, 5000, 300000)
local DK_ICEBOUND = cfg(50, SPELL_ICEBOUND_FORTITUDE, 25000, 35000, 5000, 60000)
local ROGUE_CLOAK = cfg(50, SPELL_CLOAK, 20000, 30000, 5000, 90000)
local RETRO_PALADIN_SHIELD = cfg(30, SPELL_DIVINE_SHIELD, 20000, 30000, 5000, 300000, SPELL_FORBEARANCE)

-- trial_of_the_crusader.h:180-208 NPC_ALLIANCE_* / NPC_HORDE_* entries.
-- NOTE: npc_toc_druid (restoration 34469/34459) and npc_toc_boomkin
-- (balance 34460/34451) are ported separately in
-- lua_scripts/northrend/npc_toc_druid.lua and npc_toc_boomkin.lua —
-- not registered here.
-- npc_toc_paladin (holy): 34465 / 34445.
-- npc_toc_shadow_priest: 34473 / 34441.
-- npc_toc_mage: 34468 / 34449.
-- npc_toc_hunter: 34467 / 34448.
-- npc_toc_warrior: 34475 / 34453.
-- npc_toc_dk: 34461 / 34458.
-- npc_toc_rogue: 34472 / 34454.
-- npc_toc_retro_paladin (retribution): 34471 / 34456.
local CHAMPIONS = {
    [34465] = PALADIN_SHIELD,
    [34445] = PALADIN_SHIELD,
    [34473] = SHADOW_DISPERSION,
    [34441] = SHADOW_DISPERSION,
    [34468] = MAGE_ICE_BLOCK,
    [34449] = MAGE_ICE_BLOCK,
    [34467] = HUNTER_DETERRENCE,
    [34448] = HUNTER_DETERRENCE,
    [34475] = WARRIOR_RETALIATION,
    [34453] = WARRIOR_RETALIATION,
    [34461] = DK_ICEBOUND,
    [34458] = DK_ICEBOUND,
    [34472] = ROGUE_CLOAK,
    [34454] = ROGUE_CLOAK,
    [34471] = RETRO_PALADIN_SHIELD,
    [34456] = RETRO_PALADIN_SHIELD,
}

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

-- C++ defensive case arm at timer-fire time: HealthBelowPct(pct) on
-- current health (integer strict-<, Unit.h); below threshold (and no
-- forbearance where C++ gates on it) -> self-cast + long cooldown
-- reschedule; else short check reschedule.
local function defensiveCheck(creature, guid, championCfg)
    local maxHealth = creature:GetMaxHealth()
    local fire = maxHealth > 0
        and creature:GetHealth() < math.floor(maxHealth * championCfg.pct / 100)
        and (championCfg.forbearance == nil or not creature:HasAura(championCfg.forbearance))
    if fire then
        creature:CastSpell(creature, championCfg.spell)
        schedule(guid, "def", championCfg.castDelay, function()
            defensiveCheck(creature, guid, championCfg)
        end)
    else
        schedule(guid, "def", championCfg.checkDelay, function()
            defensiveCheck(creature, guid, championCfg)
        end)
    end
end

local function championEnterCombat(event, creature, target)
    local championCfg = CHAMPIONS[creature:GetEntry()]
    if championCfg == nil then
        return
    end
    -- C++ boss_faction_championsAI::JustEngagedWith (:644-651), arm order:
    -- :646 DoCast(me, SPELL_ANTI_AOE, true) — triggered self-cast, PORTED
    -- 2026-10-23 (the old "no bridge" claim was stale — CastSpell triggered
    -- is live); :647 SetCombatPulseDelay(5) — PORTED 2026-10-23 (bridge live
    -- at engine/world/lua_creature_events.go:902, same twin bpc uses; the
    -- old "no bridge" claim was stale); :648 setActive(true) has no bridge;
    -- :649 DoZoneInCombat() PORTED 2026-10-23 (creature:DoZoneInCombat);
    -- the :650-651 SetData(2, IN_PROGRESS) -> summons.DoZoneInCombat()
    -- machine leg stays out (summons-container relay + instance-script
    -- model absent).
    creature:CastSpell(creature, SPELL_ANTI_AOE, true) -- C++ :646
    creature:SetCombatPulseDelay(5) -- C++ :647 (live bridge; 2026-10-23)
    creature:DoZoneInCombat()
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "def", math.random(championCfg.initMin, championCfg.initMax), function()
        defensiveCheck(creature, guid, championCfg)
    end)
end

local function championLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function championDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function championReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local CHAMPION_ENTRIES = {
    34465, 34445, 34473, 34441, 34468, 34449,
    34467, 34448, 34475, 34453, 34461, 34458,
    34472, 34454, 34471, 34456,
}

for _, entry in ipairs(CHAMPION_ENTRIES) do
    RegisterCreatureEvent(entry, 1, championEnterCombat)
    RegisterCreatureEvent(entry, 2, championLeaveCombat)
    RegisterCreatureEvent(entry, 4, championDied)
    RegisterCreatureEvent(entry, 23, championReset)
end

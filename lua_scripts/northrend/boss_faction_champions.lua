-- Faction Champions (Trial of the Crusader) — defensive self-buff timer arms
-- ported from
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- Each champion's CreatureScript struct (npc_toc_druid, npc_toc_paladin,
-- npc_toc_shadow_priest, npc_toc_mage, npc_toc_hunter, npc_toc_boomkin,
-- npc_toc_warrior, npc_toc_dk, npc_toc_rogue, npc_toc_retro_paladin) runs
-- an EventMap; the defensive self-buff case arm is modeled here per
-- champion, plus the victim-gated and EnemiesInRange count-gated
-- rotation arms for hunter / dk / rogue (see below) — the rest of
-- each champion's rotation is unported (single-arm territory). Creature entries from trial_of_the_crusader.h NPC_ALLIANCE_*
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
-- - The rest of each champion's rotation (DoSelectLowestHpFriendly
--   heals, SelectTarget random-player DPS arms, the remaining
--   EnemiesInRange call sites) is unported. npc_toc_warrior
--   EVENT_SHATTERING_THROW (:1676, init 20-40s) stays out: its gate
--   is victim HasAuraWithMechanic(MECHANIC_IMMUNE_SHIELD), which has
--   no Lua surface method. boss_faction_championsAI::UpdateThreat
--   (:611, per-ref ScaleThreat(0) + AddThreat(calculated)) stays out:
--   GetAITargets yields victims, not mutable threat refs, so the
--   per-ref rewrite has no bridge.
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
local SPELL_WING_CLIP = 66207 -- boss_faction_champions.cpp:128 (npc_toc_hunter)
local SPELL_FROST_TRAP = 65880 -- boss_faction_champions.cpp:125 (npc_toc_hunter)
local SPELL_DISENGAGE = 65869 -- boss_faction_champions.cpp:123 (npc_toc_hunter)
local SPELL_DEATH_GRIP = 66017 -- boss_faction_champions.cpp:156 (npc_toc_dk)
local SPELL_BLADE_FLURRY = 65956 -- boss_faction_champions.cpp:167 (npc_toc_rogue)
local SPELL_SHADOWSTEP = 66178 -- boss_faction_champions.cpp:168 (npc_toc_rogue)

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

-- Victim-gated rotation arms (per-struct Reset initial schedules, C++
-- case arms): victim arm cfg holds the spell, init range, IsInRange
-- bounds (minRange nil = plain GetDistance2d < maxRange), and the
-- cast/miss reschedule ranges.
local function victimCfg(spell, initMin, initMax, minRange, maxRange, hitMin, hitMax, missMin, missMax)
    return {
        spell = spell, initMin = initMin, initMax = initMax,
        minRange = minRange, maxRange = maxRange,
        hitMin = hitMin, hitMax = hitMax, missMin = missMin, missMax = missMax,
    }
end

-- npc_toc_hunter EVENT_WING_CLIP :1487 (init 10-20s, reschedule
-- 15-25s either way); npc_toc_dk EVENT_DEATH_GRIP :1755 (init
-- 15-25s, cast -> 35s, gate fails -> 3s); npc_toc_rogue
-- EVENT_SHADOWSTEP :1874 (init 20-30s, cast -> 30s, gate fails -> 5s).
local VICTIM_ARMS = {
    [34467] = victimCfg(SPELL_WING_CLIP, 10000, 20000, nil, 6.0, 15000, 25000, 15000, 25000),
    [34448] = victimCfg(SPELL_WING_CLIP, 10000, 20000, nil, 6.0, 15000, 25000, 15000, 25000),
    [34461] = victimCfg(SPELL_DEATH_GRIP, 15000, 25000, 5.0, 30.0, 35000, 35000, 3000, 3000),
    [34458] = victimCfg(SPELL_DEATH_GRIP, 15000, 25000, 5.0, 30.0, 35000, 35000, 3000, 3000),
    [34472] = victimCfg(SPELL_SHADOWSTEP, 20000, 30000, 10.0, 40.0, 30000, 30000, 5000, 5000),
    [34454] = victimCfg(SPELL_SHADOWSTEP, 20000, 30000, 10.0, 40.0, 30000, 30000, 5000, 5000),
}

local function victimCheck(creature, guid, arm)
    local victim = creature:GetVictim()
    local fire = false
    if victim ~= nil then
        local dist = creature:GetDistance2d(victim)
        if arm.minRange == nil then
            fire = dist < arm.maxRange
        else
            -- WorldObject::IsInRange (Object.cpp:1329, is3D=false): 2D
            -- distance against min/max plus both combat reaches, dist
            -- >= min side and dist < max side; ToC victims are players
            -- (1.5 default reach; player surface has no GetCombatReach,
            -- kologarn precedent).
            local sizefactor = creature:GetCombatReach() + 1.5
            fire = dist >= arm.minRange + sizefactor and dist < arm.maxRange + sizefactor
        end
    end
    if fire then
        creature:CastSpell(victim, arm.spell)
        schedule(guid, "victim", math.random(arm.hitMin, arm.hitMax), function()
            victimCheck(creature, guid, arm)
        end)
    else
        schedule(guid, "victim", math.random(arm.missMin, arm.missMax), function()
            victimCheck(creature, guid, arm)
        end)
    end
end

-- EnemiesInRange count arms: C++ iterates the threat list counting
-- victims within 10.0 yd (GetDistance2d, strict <); GetAITargets is
-- the bridged threat list (sorted, same set C++ iterates unsorted),
-- so the count matches unless a dead ref lingers on one side only.
-- npc_toc_hunter EVENT_FROST_TRAP :1479 (init 10-20s, DoCastAOE =
-- self-cast, 30s either way) and EVENT_DISENGAGE :1470 (init
-- 10-20s, self-cast, 30s either way); npc_toc_rogue
-- EVENT_BLADE_FLURRY :1875 (init 10-20s, cast -> 120s, gate fails
-- -> 5s).
local function countCfg(spell, initMin, initMax, hitMin, hitMax, missMin, missMax)
    return {
        spell = spell, initMin = initMin, initMax = initMax,
        hitMin = hitMin, hitMax = hitMax, missMin = missMin, missMax = missMax,
    }
end

local HUNTER_COUNTS = {
    countCfg(SPELL_FROST_TRAP, 10000, 20000, 30000, 30000, 30000, 30000),
    countCfg(SPELL_DISENGAGE, 10000, 20000, 30000, 30000, 30000, 30000),
}
local ROGUE_COUNTS = {
    countCfg(SPELL_BLADE_FLURRY, 10000, 20000, 120000, 120000, 5000, 5000),
}

local COUNT_ARMS = {
    [34467] = HUNTER_COUNTS,
    [34448] = HUNTER_COUNTS,
    [34472] = ROGUE_COUNTS,
    [34454] = ROGUE_COUNTS,
}

local function countCheck(creature, guid, arm)
    local count = 0
    for _, target in ipairs(creature:GetAITargets()) do
        if creature:GetDistance2d(target) < 10.0 then
            count = count + 1
        end
    end
    if count >= 2 then
        creature:CastSpell(creature, arm.spell)
        schedule(guid, "count" .. arm.spell, math.random(arm.hitMin, arm.hitMax), function()
            countCheck(creature, guid, arm)
        end)
    else
        schedule(guid, "count" .. arm.spell, math.random(arm.missMin, arm.missMax), function()
            countCheck(creature, guid, arm)
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
    local vArm = VICTIM_ARMS[creature:GetEntry()]
    if vArm ~= nil then
        schedule(guid, "victim", math.random(vArm.initMin, vArm.initMax), function()
            victimCheck(creature, guid, vArm)
        end)
    end
    local cArms = COUNT_ARMS[creature:GetEntry()]
    if cArms ~= nil then
        for _, cArm in ipairs(cArms) do
            schedule(guid, "count" .. cArm.spell, math.random(cArm.initMin, cArm.initMax), function()
                countCheck(creature, guid, cArm)
            end)
        end
    end
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

-- C++ boss_faction_championsAI::KilledUnit (boss_faction_champions.cpp
-- :655-666): a player victim relays via instance GetCreature(DATA_VARIAN
-- /DATA_GARROSH) to varian/garrosh DoAction(ACTION_SAY_KILLED_PLAYER),
-- whose only leg is a bare Talk (trial_of_the_crusader.cpp:764/833,
-- GARROSH_SAY_KILLED/VARIAN_SAY_KILLED 6). No DoAction/instance bridge
-- exists, but the trigger (event 3 OnTargetDied, kalecgos precedent)
-- and the receiver lookup (FindNearestCreature, maulgar precedent) are
-- both bridged, so the relay ports: instance DATA_TEAM is the first
-- entering player's team (instance_trial_of_the_crusader.cpp:135-136)
-- and champions spawn opposite-faction, so an Alliance-entry champion
-- means Horde players -> Garrosh talks, and a Horde-entry champion
-- means Alliance players -> Varian talks. Nil leader (out of the
-- 150.0 grid) means no relay, matching the C++ live-creature gate.
local ENTRY_VARIAN = 34990 -- trial_of_the_crusader.h:161 NPC_VARIAN
local ENTRY_GARROSH = 34995 -- trial_of_the_crusader.h:160 NPC_GARROSH
local SAY_KILLED_PLAYER = 6 -- trial_of_the_crusader.cpp:61/70
local ALLIANCE_CHAMPIONS = {
    [34465] = true, [34473] = true, [34468] = true, [34467] = true,
    [34475] = true, [34461] = true, [34472] = true, [34471] = true,
}

local function championTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_VARIAN
    if ALLIANCE_CHAMPIONS[creature:GetEntry()] then
        leaderEntry = ENTRY_GARROSH
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

local CHAMPION_ENTRIES = {
    34465, 34445, 34473, 34441, 34468, 34449,
    34467, 34448, 34475, 34453, 34461, 34458,
    34472, 34454, 34471, 34456,
}

for _, entry in ipairs(CHAMPION_ENTRIES) do
    RegisterCreatureEvent(entry, 1, championEnterCombat)
    RegisterCreatureEvent(entry, 2, championLeaveCombat)
    RegisterCreatureEvent(entry, 3, championTargetDied)
    RegisterCreatureEvent(entry, 4, championDied)
    RegisterCreatureEvent(entry, 23, championReset)
end

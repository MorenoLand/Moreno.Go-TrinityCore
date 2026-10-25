-- Battle for Mount Hyjal: Azgalor — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/
-- boss_azgalor.cpp (281 lines; boss_azgalorAI : public hyjal_trashAI :
-- public EscortAI; npc_lesser_doomguard : public CreatureScript,
-- npc_lesser_doomguardAI : public hyjal_trashAI; AddSC_boss_azgalor at
-- end registers both; kalimdor loader decl 33 / call 146 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_azgalor" and "npc_lesser_doomguard" (loader lines only
-- otherwise).
-- Entry: hyjal.h:81 HYCreaturesIds names AZGALOR = 17842 under the
-- "Bosses summoned after every 8 waves" comment AND
-- instance_hyjal.cpp:126 cases it in OnCreatureCreate (Azgalor GUID
-- capture, :157 the DATA_AZGALOR GetGuidData leg) — the name-to-entry
-- tie is C++-verified (ramstein strength); the creature_template
-- ScriptName binding stays DB-side.
-- GetAI uses GetHyjalAI<boss_azgalorAI>, same as jaina/thrall.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; the IsEvent escort
-- machine needs an EscortAI + instance bridge and has none — see below):
-- - Engage (C++ JustEngagedWith, non-instance leg): Talk SAY_ONAGGRO (3)
--   + arm each timer at its C++ constructor-init cooldown (hyjal.lua
--   convention). SAY_DOOM (2) is marked "Not used?" in C++ and has no
--   Talk arm — no C++-side Talk to port.
-- - Rain of Fire 31340 on a random alive player within 30 (C++
--   SelectTargetMethod::Random, 0, 30, true — threat-list player
--   pool, ras_frostwhisper precedent), non-triggered, init 20s -> {20s,35s} (C++ 20000 +
--   rand32()%15000); nil target leaves the C++ timer expired so the next
--   tick retries — modeled as cast-if-target-alive, then re-arm.
-- - Doom 31347 on SelectTargetMethod::Random, 1, 100, true — position 1
--   (the C++ comment says "never on tank"); threat-list pick via
--   GetAITargets (offset 1 pops the victim, players within 100).
--   Non-triggered, init 50s -> {45s,50s}.
-- - Howl of Azgalor 31344 self-cast (C++ DoCast(me)), non-triggered,
--   init 30s -> 30s.
-- - Cleave 31345 on the victim (C++ DoCastVictim — jeklik GetVictim +
--   CastSpell convention), non-triggered, init 10s -> {10s,15s}
--   (C++ 10000 + rand32()%5000).
-- - Enrage: one-shot at 600s (C++ EnrageTimer < diff && !enraged, then
--   EnrageTimer = 600000 with enraged = true so it never refires):
--   triggered self-cast Berserk 26662 (aku_mai triggered-form
--   convention). me->InterruptNonMeleeSpells(false) is bridged on the
--   event-creature surface as the no-op twin (svala precedent) —
--   PORTED 2026-10-24.
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_ONSLAY (1); no TYPEID gate in C++ (shade_of_aran precedent).
-- - Death (C++ JustDied, non-instance leg): hyjal_trashAI::JustDied is
--   instance bookkeeping (blocked, standing); Talk SAY_ONDEATH (0).
-- Unmodeled (documented-only, no bridges):
-- - The IsEvent escort machine: the go-flagged first-UpdateAI block adds
--   8 C++-verbatim waypoints (5492.91,-2404.61,1462.63 /
--   5531.76,-2460.87,1469.55 / 5554.58,-2514.66,1476.12 /
--   5554.16,-2567.23,1479.90 / 5540.67,-2625.99,1480.89 /
--   5508.16,-2659.20,1480.15 / 5489.62,-2704.05,1482.18 /
--   5457.04,-2726.26,1485.10), Start(false, true),
--   SetDespawnAtEnd(false); WaypointReached(7) AddThreat(0) on the
--   instance DATA_THRALL GUID — EscortAI movement plus instance-script
--   model, both blocked (standing).
-- - Reset/engage/death instance legs: SetData(DATA_AZGALOREVENT,
--   NOT_STARTED / IN_PROGRESS / DONE) — instance-data bridge blocked
--   (standing); the constructor Initialize() timer reset collapses into
--   the engage re-arm (hyjal.lua convention).
-- - hyjal_trashAI::JustDied: instance->SetData(DATA_TRASH, 0) wave signal
--   plus the MINRAIDDAMAGE lootable-flag gate — blocked (standing).
-- npc_lesser_doomguard: IS registered in C++ (new npc_lesser_doomguard()
-- in AddSC_boss_azgalor) but unportable with zero C++ entry evidence (no
-- hyjal.h constant, zero hits outside boss_azgalor.cpp + loader):
-- registering would invent an identifier (gelihast precedent). Its
-- bridgeable arms, documented here for when an entry verifies:
-- - Reset: self-cast Thrash 12787 (non-triggered), timers Cripple 50000 /
--   Warstomp 10000 / Check 5000.
-- - War Stomp 31408 self-cast, non-triggered, init 10s -> {10s,15s}.
-- - Cripple 31406 on a random alive player within 100 (C++
--   SelectTargetMethod::Random, 0, 100, true), non-triggered, init 50s
--   -> {25s,30s}.
-- - CheckTimer 5s: DespawnOrUnsummon when the instance DATA_AZGALOR GUID
--   is missing/dead (instance GUID bridge blocked); MoveInLineOfSight
--   AttackStart within 50 while not in combat — event 27, IsWithinDist
--   and IsInCombat are all bridged, but the AttackStart relay has no
--   bridge, so the arm stays out (2026-10-23 re-triage).

local ENTRY = 17842

local SAY_ONDEATH = 0
local SAY_ONSLAY  = 1
local SAY_ONAGGRO = 3

local SPELL_RAIN_OF_FIRE    = 31340
local SPELL_DOOM            = 31347
local SPELL_HOWL_OF_AZGALOR = 31344
local SPELL_CLEAVE          = 31345
local SPELL_BERSERK         = 26662

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ SelectTargetMethod::Random, 0, maxDist, true — threat-list
-- player pool (UnitAI.h: SelectTarget draws from the threat list;
-- offset 0, no skip; ras_frostwhisper precedent). Only Rain of Fire
-- still uses this helper (Doom uses randomThreatPlayerInRange below);
-- excludeVictim is vestigial here since Rain passes false.
local function randomPlayerInRange(creature, maxDist, excludeVictim)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) <= maxDist then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ SelectTarget(Random, 1, maxDist, true) threat-list pick
-- (UnitAI.h:163-174): offset 1 pops the first MaxThreat entry
-- (the current victim, first in the C++-constructed list);
-- the Go list is threat-descending so the victim entry is
-- dropped by GUID instead. Dead members are offline in C++
-- (skipped at list build) so they are filtered here.
local function randomThreatPlayerInRange(creature, maxDist)
    local victim = creature:GetVictim()
    local victimGUID = victim and victim:GetGUID() or 0
    local pool = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetGUID() ~= victimGUID and t:GetObjectType() == "Player"
                and not t:IsDead() and creature:GetDistance(t) <= maxDist then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ RainTimer: DoCast(SelectTarget(Random, 0, 30, true), 31340),
-- non-triggered, init 20s -> {20s,35s}; nil-target no-op arm still
-- re-arms (kazrogal precedent).
-- C++ re-arms use X + rand32() % N -> [X, X+N-1] (exclusive top edge),
-- NOT urand(X, X+N) -> [X, X+N]; the -1 is C++-exact, not a typo.
local function onRain(creature, guid)
    local target = randomPlayerInRange(creature, 30, false)
    if target and not target:IsDead() then
        creature:CastSpell(target, SPELL_RAIN_OF_FIRE)
    end
    schedule(guid, "rain", math.random(20000, 34999), function()
        onRain(creature, guid)
    end)
end

-- C++ DoomTimer: DoCast(SelectTarget(Random, 1, 100, true), 31347),
-- non-triggered, init 50s -> {45s,50s}; position 1 never hits the tank
-- (threat-list pick, victim entry dropped, see above).
local function onDoom(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target and not target:IsDead() then
        creature:CastSpell(target, SPELL_DOOM)
    end
    schedule(guid, "doom", math.random(45000, 49999), function()
        onDoom(creature, guid)
    end)
end

-- C++ HowlTimer: DoCast(me, 31344), non-triggered, init 30s -> 30s.
local function onHowl(creature, guid)
    creature:CastSpell(creature, SPELL_HOWL_OF_AZGALOR)
    schedule(guid, "howl", 30000, function()
        onHowl(creature, guid)
    end)
end

-- C++ CleaveTimer: DoCastVictim(31345), non-triggered, init 10s ->
-- {10s,15s} (jeklik GetVictim + CastSpell convention).
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim and not victim:IsDead() then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", math.random(10000, 14999), function()
        onCleave(creature, guid)
    end)
end

-- C++ EnrageTimer: one-shot at 600s (InterruptNonMeleeSpells(false)
-- before the cast, C++ order), then triggered self-cast Berserk 26662.
local function onEnrage(creature, guid)
    creature:InterruptNonMeleeSpells(false)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_ONAGGRO)
    schedule(guid, "rain", 20000, function()
        onRain(creature, guid)
    end)
    schedule(guid, "doom", 50000, function()
        onDoom(creature, guid)
    end)
    schedule(guid, "howl", 30000, function()
        onHowl(creature, guid)
    end)
    schedule(guid, "cleave", 10000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_ONDEATH)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_ONSLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
